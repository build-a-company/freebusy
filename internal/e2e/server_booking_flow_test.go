// Promo-code and booking flows of the server e2e suite.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/pricing/v1/pricingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// promoFlow: create with derived ACTIVE state and the redeemability math.
func promoFlow(t *testing.T, c *e2eClients, _ database.Provider) {
	t.Helper()
	ctx := context.Background()

	code := "E2E" + c.suffix
	promo, err := c.promos.CreatePromoCode(ctx, &promocodepbv1.CreatePromoCodeRequest{
		PromoCode: &promocodepbv1.PromoCode{
			Code:     code,
			Discount: &promocodepbv1.Discount{Amount: &promocodepbv1.Discount_PercentOff{PercentOff: 20}},
		},
	})
	if err != nil {
		t.Fatalf("CreatePromoCode: %v", err)
	}
	t.Cleanup(func() {
		if _, err := c.promos.DeletePromoCode(ctx, &promocodepbv1.DeletePromoCodeRequest{Name: promo.GetName(), Force: true}); err != nil {
			t.Logf("DeletePromoCode: %v", err)
		}
	})
	if promo.GetState() != promocodepbv1.PromoCodeState_PROMO_CODE_STATE_ACTIVE {
		t.Fatalf("promo state = %v, want derived ACTIVE", promo.GetState())
	}

	if _, err := c.promos.ValidatePromoCode(ctx, &promocodepbv1.ValidatePromoCodeRequest{
		Code:     code,
		Subtotal: &money.Money{CurrencyCode: "USD", Units: 100},
	}); err != nil {
		t.Fatalf("ValidatePromoCode: %v", err)
	}
}

// bookingFlow drives a booking from hold through confirmation to cancellation
// against a resource freebusy has never heard of, priced by a rate plan it has.
//
// The uncatalogued resource is the assertion, not an oversight. Since the
// catalogue moved to the RFC services, a booking names an RFC 9073 resource that
// freebusy does not store, and the engine must book it on defaults rather than
// refuse it. Pricing is the other half: the plan is created through the real
// RatePlanService, so a non-zero total proves the write and the booking path's
// read of it are both wired.
func bookingFlow(t *testing.T, c *e2eClients) {
	t.Helper()
	ctx := context.Background()

	resource := c.resourceName()

	// Price the resource through the real service. A resource with no plan books
	// free by design (loadRatePlan returns nil, nil), so creating one here is
	// what makes the total below meaningful.
	perNight := pricingpbv1.PricingUnit_PRICING_UNIT_PER_NIGHT
	plan, err := c.ratePlans.CreateRatePlan(ctx, &pricingpbv1.CreateRatePlanRequest{
		RatePlan: &pricingpbv1.RatePlan{
			Resource:    resource,
			DisplayName: "E2E Flexible",
			Price:       &money.Money{CurrencyCode: "INR", Units: 5000},
			PricingUnit: perNight,
		},
	})
	if err != nil {
		t.Fatalf("CreateRatePlan: %v", err)
	}
	t.Cleanup(func() {
		if _, err := c.ratePlans.DeleteRatePlan(ctx, &pricingpbv1.DeleteRatePlanRequest{Name: plan.GetName()}); err != nil {
			t.Logf("DeleteRatePlan: %v", err)
		}
	})

	start := time.Now().UTC().AddDate(0, 0, 30).Truncate(24 * time.Hour)
	window := &sharedpbv1.TimeWindow{
		StartTime: timestamppb.New(start),
		EndTime:   timestamppb.New(start.AddDate(0, 0, 2)),
	}

	booking, err := c.bookings.CreateBooking(ctx, &schedulingpbv1.CreateBookingRequest{
		Booking: &schedulingpbv1.Booking{
			Unit:    resource,
			Window:  window,
			Contact: &sharedpbv1.Contact{DisplayName: "E2E Guest", Email: "guest-" + c.suffix + "@example.com"},
		},
	})
	if err != nil {
		t.Fatalf("CreateBooking against an uncatalogued, unpriced resource: %v", err)
	}
	t.Cleanup(func() {
		if err := c.bookRepos.Bookings.Delete(ctx, booking.GetName()); err != nil {
			t.Logf("delete booking row: %v", err)
		}
	})
	if booking.GetState() != schedulingpbv1.BookingState_BOOKING_STATE_PENDING_HOLD {
		t.Fatalf("booking state = %v, want PENDING_HOLD", booking.GetState())
	}
	if booking.GetUnit() != resource {
		t.Fatalf("booking unit = %q, want the resource name %q round-tripped", booking.GetUnit(), resource)
	}
	// Two nights at 5000/night: the booking was priced from the plan created
	// above, which is the whole point of this assertion.
	if got := booking.GetTotal().GetUnits(); got != 10000 {
		t.Fatalf("booking total = %d, want 10000 (2 nights x 5000 from the rate plan)", got)
	}

	confirmed, err := c.bookings.ConfirmBooking(ctx, &schedulingpbv1.ConfirmBookingRequest{Name: booking.GetName()})
	if err != nil || confirmed.GetState() != schedulingpbv1.BookingState_BOOKING_STATE_CONFIRMED {
		t.Fatalf("ConfirmBooking: %v (state %v)", err, confirmed.GetState())
	}

	// Capacity is one per resource under the per-resource-calendar model, so the
	// confirmed stay exhausts it: a second booking of the same span must fail
	// rather than double-sell.
	_, err = c.bookings.CreateBooking(ctx, &schedulingpbv1.CreateBookingRequest{
		Booking: &schedulingpbv1.Booking{
			Unit:    resource,
			Window:  window,
			Contact: &sharedpbv1.Contact{DisplayName: "Second Guest", Email: "guest2-" + c.suffix + "@example.com"},
		},
	})
	if err == nil {
		t.Fatal("CreateBooking: an overlapping second booking succeeded — capacity is one per resource")
	}

	if _, err := c.bookings.PreviewCancellation(ctx, &schedulingpbv1.PreviewCancellationRequest{Name: booking.GetName()}); err != nil {
		t.Fatalf("PreviewCancellation: %v", err)
	}
	cancelled, err := c.bookings.CancelBooking(ctx, &schedulingpbv1.CancelBookingRequest{Name: booking.GetName()})
	if err != nil || cancelled.GetState() != schedulingpbv1.BookingState_BOOKING_STATE_CANCELLED {
		t.Fatalf("CancelBooking: %v (state %v)", err, cancelled.GetState())
	}
}
