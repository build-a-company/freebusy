package gorm

import (
	"context"
	"fmt"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/scheduling"
	"github.com/oh-tarnished/freebusy/internal/service/scheduling/party"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/guest/v1/guestpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"google.golang.org/genproto/googleapis/type/money"
	"gorm.io/gorm"
)

// ExpireHolds flips every PENDING_HOLD booking whose hold has lapsed to EXPIRED,
// freeing the capacity it reserved. Returns the number of holds expired. Intended
// to be called periodically by the hold sweeper.
func (r *BookingRepository) ExpireHolds(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&scheduling.Booking{}).
		Where("state = ? AND hold_expire_time IS NOT NULL AND hold_expire_time < ?", scheduling.BookingStatePendingHold, now).
		Updates(map[string]any{
			"state":       scheduling.BookingStateExpired,
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
func (r *BookingRepository) UpdateBookingGuests(ctx context.Context, name string, guests []*guestpbv1.Guest, occupancy *schedulingpbv1.Occupancy) (*schedulingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var m scheduling.Booking
		if e := tx.WithContext(ctx).First(&m, "id = ?", id).Error; e != nil {
			return e
		}
		if m.State == nil || (*m.State != scheduling.BookingStatePendingHold && *m.State != scheduling.BookingStateConfirmed) {
			return fmt.Errorf("%w: the party can only be edited while the booking is on hold or confirmed", types.ErrInvalidState)
		}

		// Re-validate the party against the resource's max occupancy.
		prof := r.resolveResourceProfile(ctx, m.Unit)
		if !party.Fits(prof.MaxOccupancy, repox.Deref(m.Units), occupancy, guests) {
			return types.ErrInvalidArgument
		}

		// Drop the old party, then repoint the occupancy and insert the new party.
		if e := deleteBookingGuests(ctx, tx, id); e != nil {
			return e
		}
		oldOccID := m.OccupancyID
		newOcc := occupancyToModel(occupancy)
		if newOcc != nil {
			if e := scheduling.NewOccupancyStore(tx).Create(ctx, newOcc); e != nil {
				return e
			}
			m.OccupancyID = &newOcc.ID
		} else {
			m.OccupancyID = nil
		}
		m.Etag = repox.Ptr(ulid.GenerateString())
		if e := scheduling.NewBookingStore(tx).Update(ctx, &m); e != nil {
			return e
		}
		if oldOccID != nil {
			if e := scheduling.NewOccupancyStore(tx).DeleteByID(ctx, *oldOccID); e != nil {
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
func (r *BookingRepository) ConfirmBooking(ctx context.Context, name string) (*schedulingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var m scheduling.Booking
		if e := tx.WithContext(ctx).First(&m, "id = ?", id).Error; e != nil {
			return e
		}
		if m.State == nil || *m.State != scheduling.BookingStatePendingHold {
			return fmt.Errorf("%w: only a booking on hold can be confirmed", types.ErrInvalidState)
		}
		now := time.Now().UTC()
		state := scheduling.BookingStateConfirmed
		m.State = &state
		m.ConfirmTime = &now
		m.HoldExpireTime = nil
		m.Etag = repox.Ptr(ulid.GenerateString())
		return scheduling.NewBookingStore(tx).Update(ctx, &m)
	})
	if err != nil {
		return nil, repox.MapGormErr(err)
	}
	return r.GetBooking(ctx, name)
}

// CancelBooking flips a held or confirmed booking to CANCELLED, computing the
// refund from the unit's cancellation policy.
func (r *BookingRepository) CancelBooking(ctx context.Context, name string, reason schedulingpbv1.CancelReason) (*schedulingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var m scheduling.Booking
		if e := preloadBooking(tx.WithContext(ctx)).First(&m, "id = ?", id).Error; e != nil {
			return e
		}
		// Cancelling a cancelled booking is what the caller already asked for, so
		// it succeeds and returns the booking as it stands. Nothing is recomputed:
		// re-running the refund would move cancel_time and could land a different
		// refund percent as the policy's clock advances, which would make a retry
		// visibly different from the call it retries.
		if m.State != nil && *m.State == scheduling.BookingStateCancelled {
			return nil
		}
		// An expired hold is not a cancellable thing — the inventory was released
		// when it lapsed. This is a state error, not an inventory conflict.
		if m.State != nil && *m.State == scheduling.BookingStateExpired {
			return fmt.Errorf("%w: the hold expired and cannot be cancelled", types.ErrInvalidState)
		}
		pct, amount, _, e := r.computeRefund(ctx, tx, &m)
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		state := scheduling.BookingStateCancelled
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
		return scheduling.NewBookingStore(tx).Update(ctx, &m)
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
	var m scheduling.Booking
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
