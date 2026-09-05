package gorm

import (
	"context"
	"fmt"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/booking"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/property"
	"github.com/oh-tarnished/freebusy/internal/service/booking/party"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"google.golang.org/genproto/googleapis/type/money"
	"gorm.io/gorm"
)

// ExpireHolds flips every PENDING_HOLD booking whose hold has lapsed to EXPIRED,
// freeing the capacity it reserved. Returns the number of holds expired. Intended
// to be called periodically by the hold sweeper.
func (r *BookingRepository) ExpireHolds(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&booking.Booking{}).
		Where("state = ? AND hold_expire_time IS NOT NULL AND hold_expire_time < ?", booking.BookingStatePendingHold, now).
		Updates(map[string]any{
			"state":       booking.BookingStateExpired,
			"etag":        ulid.GenerateString(),
			"update_time": now,
		})
	if res.Error != nil {
		return 0, repox.MapGormErr(res.Error)
	}
	return res.RowsAffected, nil
}

// UpdateBookingGuests replaces the whole staying party (guests + occupancy) on a
// booking. It is allowed only while the booking is PENDING_HOLD or CONFIRMED, and
// re-validates the new party against the unit's max occupancy. Old guest rows and
// their sub-rows, and the old occupancy, are removed in the same transaction.
func (r *BookingRepository) UpdateBookingGuests(ctx context.Context, name string, guests []*identitypbv1.Guest, occupancy *bookingpbv1.Occupancy) (*bookingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var m booking.Booking
		if e := tx.WithContext(ctx).First(&m, "id = ?", id).Error; e != nil {
			return e
		}
		if m.State == nil || (*m.State != booking.BookingStatePendingHold && *m.State != booking.BookingStateConfirmed) {
			return fmt.Errorf("%w: the party can only be edited while the booking is on hold or confirmed", types.ErrInvalidState)
		}

		// Re-validate the party against the unit's max occupancy.
		var unit property.Unit
		if e := tx.WithContext(ctx).Select("id", "max_occupancy").First(&unit, "id = ?", m.UnitID).Error; e != nil {
			return e
		}
		if !party.Fits(repox.Deref(unit.MaxOccupancy), repox.Deref(m.Units), occupancy, guests) {
			return types.ErrInvalidArgument
		}

		// Drop the old party, then repoint the occupancy and insert the new party.
		if e := deleteBookingGuests(ctx, tx, id); e != nil {
			return e
		}
		oldOccID := m.OccupancyID
		newOcc := occupancyToModel(occupancy)
		if newOcc != nil {
			if e := booking.NewOccupancyStore(tx).Create(ctx, newOcc); e != nil {
				return e
			}
			m.OccupancyID = &newOcc.ID
		} else {
			m.OccupancyID = nil
		}
		m.Etag = repox.Ptr(ulid.GenerateString())
		if e := booking.NewBookingStore(tx).Update(ctx, &m); e != nil {
			return e
		}
		if oldOccID != nil {
			if e := booking.NewOccupancyStore(tx).DeleteByID(ctx, *oldOccID); e != nil {
				return e
			}
		}
		return persistGuests(ctx, tx, buildGuestGraphs(guests, id))
	})
	if err != nil {
		return nil, repox.MapGormErr(err)
	}
	return r.GetBooking(ctx, name)
}

// ConfirmBooking flips a PENDING_HOLD booking to CONFIRMED.
func (r *BookingRepository) ConfirmBooking(ctx context.Context, name string) (*bookingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var m booking.Booking
		if e := tx.WithContext(ctx).First(&m, "id = ?", id).Error; e != nil {
			return e
		}
		if m.State == nil || *m.State != booking.BookingStatePendingHold {
			return fmt.Errorf("%w: only a booking on hold can be confirmed", types.ErrInvalidState)
		}
		now := time.Now().UTC()
		state := booking.BookingStateConfirmed
		m.State = &state
		m.ConfirmTime = &now
		m.HoldExpireTime = nil
		m.Etag = repox.Ptr(ulid.GenerateString())
		return booking.NewBookingStore(tx).Update(ctx, &m)
	})
	if err != nil {
		return nil, repox.MapGormErr(err)
	}
	return r.GetBooking(ctx, name)
}

// CancelBooking flips a held or confirmed booking to CANCELLED, computing the
// refund from the unit's cancellation policy.
func (r *BookingRepository) CancelBooking(ctx context.Context, name string, reason bookingpbv1.CancelReason) (*bookingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var m booking.Booking
		if e := preloadBooking(tx.WithContext(ctx)).First(&m, "id = ?", id).Error; e != nil {
			return e
		}
		// Cancelling a cancelled booking is what the caller already asked for, so
		// it succeeds and returns the booking as it stands. Nothing is recomputed:
		// re-running the refund would move cancel_time and could land a different
		// refund percent as the policy's clock advances, which would make a retry
		// visibly different from the call it retries.
		if m.State != nil && *m.State == booking.BookingStateCancelled {
			return nil
		}
		// An expired hold is not a cancellable thing — the inventory was released
		// when it lapsed. This is a state error, not an inventory conflict.
		if m.State != nil && *m.State == booking.BookingStateExpired {
			return fmt.Errorf("%w: the hold expired and cannot be cancelled", types.ErrInvalidState)
		}
		pct, amount, _, e := r.computeRefund(ctx, tx, &m)
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		state := booking.BookingStateCancelled
		m.State = &state
		m.CancelTime = &now
		m.CancelReason = cancelReasonToModel(reason)
		m.RefundPercent = repox.Ptr(pct)
		m.Etag = repox.Ptr(ulid.GenerateString())
		m.Contact, m.Window, m.Price, m.Discount, m.Total, m.RefundAmount = nil, nil, nil, nil, nil, nil
		if amount != nil {
			refund := moneyToModel(amount)
			if e := common.NewMoneyStore(tx).Create(ctx, refund); e != nil {
				return e
			}
			m.RefundAmountID = &refund.ID
		}
		return booking.NewBookingStore(tx).Update(ctx, &m)
	})
	if err != nil {
		return nil, repox.MapGormErr(err)
	}
	return r.GetBooking(ctx, name)
}

// PreviewCancellation computes the refund a cancellation would yield now, without
// cancelling.
func (r *BookingRepository) PreviewCancellation(ctx context.Context, name string) (refundable bool, percent int32, amount, nonRefundable *money.Money, summary string, err error) {
	id, err := types.BookingID(name)
	if err != nil {
		return false, 0, nil, nil, "", err
	}
	var m booking.Booking
	if err = preloadBooking(r.db.WithContext(ctx)).First(&m, "id = ?", id).Error; err != nil {
		return false, 0, nil, nil, "", repox.MapGormErr(err)
	}
	percent, amount, summary, err = r.computeRefund(ctx, r.db.WithContext(ctx), &m)
	if err != nil {
		return false, 0, nil, nil, "", repox.MapGormErr(err)
	}
	total := common.MoneyToProto(m.Total)
	nonRefundable = moneySub(total, amount)
	return percent > 0, percent, amount, nonRefundable, summary, nil
}
