// CreateBooking places a hold transactionally: availability check, pricing, promo redemption, and the guest/occupancy graph in one transaction.
package gorm

import (
	"context"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/booking"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/promocode"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/property"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/shared"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/booking/party"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"gorm.io/gorm"
)

// CreateBooking places a PENDING_HOLD on a unit for a window. It loads the unit
// (for capacity, price, booking mode, timezone), verifies capacity against
// overlapping active bookings, computes a base price, and persists the booking
// with its window / contact / price value-objects in one transaction.
func (r *BookingRepository) CreateBooking(ctx context.Context, b *bookingpbv1.Booking) (*bookingpbv1.Booking, error) {
	id, name, err := types.ResolveBookingName(b.GetName())
	if err != nil {
		return nil, err
	}
	_, unitID, err := types.ParseUnitParent(b.GetUnit())
	if err != nil {
		return nil, err
	}
	if b.GetWindow() == nil {
		return nil, types.ErrInvalidArgument
	}

	var unit property.Unit
	if err := r.db.WithContext(ctx).
		Preload("Price").
		Preload("Fees").Preload("Fees.Amount").
		Preload("Taxes").
		Preload("LosDiscounts").Preload("LosDiscounts.AmountOff").
		First(&unit, "id = ?", unitID).Error; err != nil {
		return nil, repox.MapGormErr(err)
	}

	// Load the promo code (with its discount and scope) when one is applied, so the
	// pricing engine can evaluate its scope and discount.
	var promo *promocode.PromoCode
	if pid := repox.LastSegment(b.GetPromoCode()); pid != "" {
		var p promocode.PromoCode
		if err := r.db.WithContext(ctx).
			Preload("Discount").Preload("Discount.AmountOff").
			Preload("Scope").Preload("Scope.MinSubtotal").Preload("Scope.ScopeApplicableUnits").
			First(&p, "id = ?", pid).Error; err != nil {
			return nil, repox.MapGormErr(err)
		}
		promo = &p
	}

	requested := b.GetUnits()
	if requested < 1 {
		requested = 1
	}

	// Occupancy: the staying party must fit the unit's max occupancy across the
	// reserved units (guests × max_occupancy). Zero max_occupancy means unbounded.
	if !party.Fits(repox.Deref(unit.MaxOccupancy), requested, b.GetOccupancy(), b.GetGuests()) {
		return nil, types.ErrInvalidArgument
	}
	occupancy := occupancyToModel(b.GetOccupancy())
	guestGraphs := buildGuestGraphs(b.GetGuests(), id)

	window := timeWindowToModel(b.GetWindow())
	contact := contactToModel(b.GetContact())

	// Full price breakdown: base × nights, then LOS + promo discounts, fees, taxes.
	// Nights are counted in the unit's timezone; the itemized components ride along
	// on the create response (they are not persisted).
	var priceModel, discountModel, totalModel *common.Money
	var components []*sharedpbv1.PriceComponent
	if unit.Price != nil {
		nights := nightsBetween(b.GetWindow(), unit.TimeZone)
		p := computePricing(&unit, nights, int64(requested), promo)
		priceModel = moneyToModel(p.base)
		totalModel = moneyToModel(p.total)
		if !isZeroMoney(p.discount) {
			discountModel = moneyToModel(p.discount)
		}
		components = p.components
	}

	state := booking.BookingStatePendingHold
	ttl := defaultHoldTTL
	if d := b.GetHoldTtl(); d != nil && d.AsDuration() > 0 {
		ttl = d.AsDuration()
	}
	holdExpire := time.Now().UTC().Add(ttl)

	m := &booking.Booking{
		ID:             id,
		Name:           name,
		UnitID:         unitID,
		CustomerID:     strOrNil(repox.LastSegment(b.GetCustomer())),
		Units:          repox.Ptr(requested),
		State:          &state,
		HoldExpireTime: &holdExpire,
		PromoCodeID:    strOrNil(repox.LastSegment(b.GetPromoCode())),
		Notes:          strOrNil(b.GetNotes()),
		Attributes:     structToJSON(b.GetAttributes()),
		HoldTtl:        durationToStr(b.GetHoldTtl()),
		Etag:           repox.Ptr(ulid.GenerateString()),
		WindowID:       window.ID,
	}
	if contact != nil {
		m.ContactID = &contact.ID
	}
	if priceModel != nil {
		m.PriceID = &priceModel.ID
	}
	if discountModel != nil {
		m.DiscountID = &discountModel.ID
	}
	if totalModel != nil {
		m.TotalID = &totalModel.ID
	}
	if occupancy != nil {
		m.OccupancyID = &occupancy.ID
	}

	err = r.db.Transaction(func(tx *gorm.DB) error {
		var reserved int64
		if e := tx.WithContext(ctx).Raw(overlapSQL, unitID, window.EndTime, window.StartTime).Scan(&reserved).Error; e != nil {
			return e
		}
		capacity := int64(1)
		if unit.Capacity != nil && *unit.Capacity > 0 {
			capacity = int64(*unit.Capacity)
		}
		if reserved+int64(requested) > capacity {
			return types.ErrCapacityExhausted
		}
		if e := shared.NewTimeWindowStore(tx).Create(ctx, window); e != nil {
			return e
		}
		if contact != nil {
			if e := shared.NewContactStore(tx).Create(ctx, contact); e != nil {
				return e
			}
		}
		moneys := common.NewMoneyStore(tx)
		if priceModel != nil {
			if e := moneys.Create(ctx, priceModel); e != nil {
				return e
			}
		}
		if discountModel != nil {
			if e := moneys.Create(ctx, discountModel); e != nil {
				return e
			}
		}
		if totalModel != nil {
			if e := moneys.Create(ctx, totalModel); e != nil {
				return e
			}
		}
		// Occupancy is belongs-to (created before the booking); guests are has-many
		// (created after, carrying the booking_id FK).
		if occupancy != nil {
			if e := booking.NewOccupancyStore(tx).Create(ctx, occupancy); e != nil {
				return e
			}
		}
		if e := booking.NewBookingStore(tx).Create(ctx, m); e != nil {
			return e
		}
		return persistGuests(ctx, tx, guestGraphs)
	})
	if err != nil {
		return nil, repox.MapGormErr(err)
	}
	out, err := r.GetBooking(ctx, name)
	if err != nil {
		return nil, err
	}
	// price_components are computed, not persisted; ride them along on the response.
	out.PriceComponents = components
	return out, nil
}
