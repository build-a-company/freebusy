// CreateBooking places a hold transactionally: availability check, pricing, promo redemption, and the guest/occupancy graph in one transaction.
package gorm

import (
	"context"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/promocode"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/scheduling"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/shared"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/scheduling/party"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"gorm.io/gorm"
)

// CreateBooking places a PENDING_HOLD on a unit for a window. It loads the unit
// (for capacity, price, booking mode, timezone), verifies capacity against
// overlapping active bookings, computes a base price, and persists the booking
// with its window / contact / price value-objects in one transaction.
func (r *BookingRepository) CreateBooking(ctx context.Context, b *schedulingpbv1.Booking) (*schedulingpbv1.Booking, error) {
	id, name, err := types.ResolveBookingName(b.GetName())
	if err != nil {
		return nil, err
	}
	// Validate the name, then keep it whole: the column stores the resource
	// name, so parsing to a bare id here would store something the API never
	// echoes back and the overlap query never matches.
	if _, err := types.ParseResource(b.GetUnit()); err != nil {
		return nil, types.ErrInvalidArgument
	}
	unitID := b.GetUnit()
	if b.GetWindow() == nil {
		return nil, types.ErrInvalidArgument
	}

	plan, err := loadRatePlan(ctx, r.db, b.GetUnit())
	if err != nil {
		return nil, err
	}
	prof := r.resolveResourceProfile(ctx, b.GetUnit())

	// Load the promo code (with its discount and scope) when one is applied, so the
	// pricing engine can evaluate its scope and discount.
	var promo *promocode.PromoCode
	if pid := repox.LastSegment(b.GetPromoCode()); pid != "" {
		var p promocode.PromoCode
		if err := r.db.WithContext(ctx).
			Preload("Discount").Preload("Discount.AmountOff").
			Preload("Scope").Preload("Scope.MinSubtotal").Preload("Scope.ApplicableUnits").
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
	if !party.Fits(prof.MaxOccupancy, requested, b.GetOccupancy(), b.GetGuests()) {
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
	if plan != nil && plan.Price != nil {
		nights := nightsBetween(b.GetWindow(), prof.TimeZone)
		p := computePricing(plan, nights, int64(requested), promo)
		priceModel = moneyToModel(p.base)
		totalModel = moneyToModel(p.total)
		if !isZeroMoney(p.discount) {
			discountModel = moneyToModel(p.discount)
		}
		components = p.components
	}

	state := scheduling.BookingStatePendingHold
	ttl := defaultHoldTTL
	if d := b.GetHoldTtl(); d != nil && d.AsDuration() > 0 {
		ttl = d.AsDuration()
	}
	holdExpire := time.Now().UTC().Add(ttl)

	m := &scheduling.Booking{
		ID:             id,
		Name:           name,
		Unit:           unitID,
		Customer:       strOrNil(b.GetCustomer()),
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
		capacity := prof.Capacity
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
			if e := scheduling.NewOccupancyStore(tx).Create(ctx, occupancy); e != nil {
				return e
			}
		}
		if e := scheduling.NewBookingStore(tx).Create(ctx, m); e != nil {
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
