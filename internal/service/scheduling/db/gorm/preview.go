// PreviewBooking is CreateBooking's dry run: the same unit load, occupancy
// check, pricing, and capacity check, stopping short of the transaction.
package gorm

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/promocode"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/scheduling/party"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"google.golang.org/protobuf/proto"
)

// PreviewBooking validates and prices a draft booking without persisting it: it
// answers "what would this cost, and would it be allowed" — which is what
// validate_only claims to do and, until now, did not (it echoed the draft back
// with no price at all, so callers quoted ₹0).
//
// It runs the same checks CreateBooking runs, in the same order, and returns the
// same failures: ErrInvalidArgument when the party overflows the unit, and
// ErrCapacityExhausted when the window is full. What it does not do is write —
// no hold is placed, so the price it quotes is indicative, not reserved.
func (r *BookingRepository) PreviewBooking(ctx context.Context, b *schedulingpbv1.Booking) (*schedulingpbv1.Booking, error) {
	unitID, err := types.ParseResource(b.GetUnit())
	if err != nil {
		return nil, err
	}
	if b.GetWindow() == nil {
		return nil, types.ErrInvalidArgument
	}

	plan, err := loadRatePlan(ctx, r.db, b.GetUnit())
	if err != nil {
		return nil, err
	}
	prof := resolveResourceProfile(ctx, b.GetUnit())

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
	if !party.Fits(prof.MaxOccupancy, requested, b.GetOccupancy(), b.GetGuests()) {
		return nil, types.ErrInvalidArgument
	}

	// Capacity: the same overlap query the real create runs, so a dry run that
	// says "yes" and a create that says "no capacity" can only disagree because
	// someone booked in between — never because the two paths check differently.
	window := timeWindowToModel(b.GetWindow())
	var reserved int64
	if err := r.db.WithContext(ctx).Raw(overlapSQL, unitID, window.EndTime, window.StartTime).Scan(&reserved).Error; err != nil {
		return nil, repox.MapGormErr(err)
	}
	capacity := prof.Capacity
	if reserved+int64(requested) > capacity {
		return nil, types.ErrCapacityExhausted
	}

	out := proto.Clone(b).(*schedulingpbv1.Booking)
	if plan.Price != nil {
		nights := nightsBetween(b.GetWindow(), prof.TimeZone)
		p := computePricing(plan, nights, int64(requested), promo)
		out.Price = p.base
		out.Total = p.total
		if !isZeroMoney(p.discount) {
			out.Discount = p.discount
		}
		out.PriceComponents = p.components
	}
	out.Units = requested
	return out, nil
}
