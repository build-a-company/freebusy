// Rescheduling moves a booking to a new span/unit atomically: availability re-check, price recomputation, and the refund math it shares with cancellation.
package gorm

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/booking"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/promocode"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/property"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/shared"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"gorm.io/gorm"
)

// RescheduleBooking atomically moves a booking to a new window (and optionally a
// new unit), re-checking capacity and recomputing the base price.
func (r *BookingRepository) RescheduleBooking(ctx context.Context, name string, w *bookingpbv1.Booking, newUnit string) (*bookingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	if w.GetWindow() == nil {
		return nil, types.ErrInvalidArgument
	}
	var components []*sharedpbv1.PriceComponent
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var m booking.Booking
		if e := preloadBooking(tx.WithContext(ctx)).First(&m, "id = ?", id).Error; e != nil {
			return e
		}
		unitID := m.UnitID
		if newUnit != "" {
			_, uid, perr := types.ParseUnitParent(newUnit)
			if perr != nil {
				return perr
			}
			unitID = uid
		}
		var unit property.Unit
		if e := tx.WithContext(ctx).
			Preload("Price").
			Preload("Fees").Preload("Fees.Amount").
			Preload("Taxes").
			Preload("LosDiscounts").Preload("LosDiscounts.AmountOff").
			First(&unit, "id = ?", unitID).Error; e != nil {
			return e
		}
		var promo *promocode.PromoCode
		if pid := repox.Deref(m.PromoCodeID); pid != "" {
			var p promocode.PromoCode
			if e := tx.WithContext(ctx).
				Preload("Discount").Preload("Discount.AmountOff").
				Preload("Scope").Preload("Scope.MinSubtotal").Preload("Scope.ScopeApplicableUnits").
				First(&p, "id = ?", pid).Error; e != nil {
				return e
			}
			promo = &p
		}
		window := timeWindowToModel(w.GetWindow())

		// Capacity check on the new window/unit, excluding this booking.
		var reserved int64
		if e := tx.WithContext(ctx).Raw(overlapSQL+" AND b.id <> ?", unitID, window.EndTime, window.StartTime, id).Scan(&reserved).Error; e != nil {
			return e
		}
		capacity := int64(1)
		if unit.Capacity != nil && *unit.Capacity > 0 {
			capacity = int64(*unit.Capacity)
		}
		requested := repox.Deref(m.Units)
		if requested < 1 {
			requested = 1
		}
		if reserved+int64(requested) > capacity {
			return types.ErrCapacityExhausted
		}

		if e := shared.NewTimeWindowStore(tx).Create(ctx, window); e != nil {
			return e
		}
		oldWindowID := m.WindowID
		oldPriceID, oldDiscountID, oldTotalID := m.PriceID, m.DiscountID, m.TotalID

		// Recompute the full price breakdown for the new window/unit (base, LOS +
		// promo discounts, fees, taxes), carrying the booking's promo code.
		var priceID, discountID, totalID *string
		if unit.Price != nil {
			nights := nightsBetween(w.GetWindow(), unit.TimeZone)
			p := computePricing(&unit, nights, int64(requested), promo)
			price := moneyToModel(p.base)
			total := moneyToModel(p.total)
			moneys := common.NewMoneyStore(tx)
			if e := moneys.Create(ctx, price); e != nil {
				return e
			}
			if e := moneys.Create(ctx, total); e != nil {
				return e
			}
			priceID, totalID = &price.ID, &total.ID
			if !isZeroMoney(p.discount) {
				discount := moneyToModel(p.discount)
				if e := moneys.Create(ctx, discount); e != nil {
					return e
				}
				discountID = &discount.ID
			}
			components = p.components
		}

		m.UnitID = unitID
		m.WindowID = window.ID
		m.PriceID = priceID
		m.DiscountID = discountID
		m.TotalID = totalID
		m.Etag = repox.Ptr(ulid.GenerateString())
		m.Contact, m.Window, m.Price, m.Discount, m.Total, m.RefundAmount = nil, nil, nil, nil, nil, nil
		if e := booking.NewBookingStore(tx).Update(ctx, &m); e != nil {
			return e
		}

		// Drop the superseded window (cascade would remove the booking, so delete
		// only after repointing) and the old price/discount/total Money rows.
		if e := shared.NewTimeWindowStore(tx).DeleteByID(ctx, oldWindowID); e != nil {
			return e
		}
		moneys := common.NewMoneyStore(tx)
		for _, mid := range []*string{oldPriceID, oldDiscountID, oldTotalID} {
			if mid != nil {
				if e := moneys.DeleteByID(ctx, *mid); e != nil {
					return e
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, repox.MapGormErr(err)
	}
	out, err := r.GetBooking(ctx, name)
	if err != nil {
		return nil, err
	}
	out.PriceComponents = components
	return out, nil
}
