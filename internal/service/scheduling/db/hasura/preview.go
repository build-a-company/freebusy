// PreviewBooking is CreateBooking's dry run: the same unit load, occupancy
// check, pricing, and capacity check, stopping short of the mutation batch.
package hasura

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/booking/party"
	"github.com/oh-tarnished/freebusy/internal/service/booking/pricing"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
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
func (r *BookingRepository) PreviewBooking(ctx context.Context, b *bookingpbv1.Booking) (*bookingpbv1.Booking, error) {
	_, unitID, err := types.ParseUnitParent(b.GetUnit())
	if err != nil {
		return nil, err
	}
	if b.GetWindow() == nil {
		return nil, types.ErrInvalidArgument
	}
	unit, err := r.svc.Query.Property.Units.Get(ctx, unitID)
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	if unit == nil {
		return nil, types.ErrNotFound
	}

	requested := b.GetUnits()
	if requested < 1 {
		requested = 1
	}
	if !party.Fits(repox.Deref(unit.MaxOccupancy), requested, b.GetOccupancy(), b.GetGuests()) {
		return nil, types.ErrInvalidArgument
	}

	// Capacity: the same overlap query the real create runs, so a dry run that
	// says "yes" and a create that says "no capacity" can only disagree because
	// someone booked in between — never because the two paths check differently.
	reserved, err := r.reservedUnits(ctx, unitID, b.GetWindow(), "")
	if err != nil {
		return nil, err
	}
	capacity := int64(1)
	if unit.Capacity != nil && *unit.Capacity > 0 {
		capacity = int64(*unit.Capacity)
	}
	if reserved+int64(requested) > capacity {
		return nil, types.ErrCapacityExhausted
	}

	out := proto.Clone(b).(*bookingpbv1.Booking)
	in, err := r.pricingInputs(ctx, unit, repox.LastSegment(b.GetPromoCode()))
	if err != nil {
		return nil, err
	}
	if in.Price != nil {
		in.Nights = nightsBetween(b.GetWindow(), unit.TimeZone)
		in.Units = int64(requested)
		res := pricing.Compute(in, unitID)
		out.Price = res.Base
		out.Total = res.Total
		if !pricing.IsZero(res.Discount) {
			out.Discount = res.Discount
		}
		out.PriceComponents = res.Components
	}
	out.Units = requested
	return out, nil
}
