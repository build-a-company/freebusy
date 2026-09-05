// Promo-code and booking flows of the server e2e suite.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/availability/v1/availabilitypbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// promoFlow: create with derived ACTIVE state, redeemability math, and the one
// deliberate provider divergence — derived-state filtering works on gorm and
// is rejected by hasura.
func promoFlow(t *testing.T, c *e2eClients, provider database.Provider) {
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

	verdict, err := c.promos.ValidatePromoCode(ctx, &promocodepbv1.ValidatePromoCodeRequest{
		Code:     code,
		Subtotal: &money.Money{CurrencyCode: "USD", Units: 100},
	})
	if err != nil {
		t.Fatalf("ValidatePromoCode: %v", err)
	}
	if !verdict.GetValid() || verdict.GetDiscountAmount().GetUnits() != 20 {
		t.Fatalf("ValidatePromoCode: valid=%t discount=%v, want valid with 20 USD off", verdict.GetValid(), verdict.GetDiscountAmount())
	}

	// Derived-state filtering: the gorm engine answers `state = ACTIVE` through
	// a hand-written override; the hasura engine rejects the non-stored column.
	_, err = c.promos.ListPromoCodes(ctx, &promocodepbv1.ListPromoCodesRequest{Filter: "state = ACTIVE"})
	if provider == database.ProviderGorm {
		if err != nil {
			t.Fatalf("ListPromoCodes state filter (gorm): %v", err)
		}
	} else {
		wantCode(t, err, codes.InvalidArgument, "ListPromoCodes state filter (hasura)")
	}
}

// bookingFlow: the hold lifecycle — create → PENDING_HOLD → confirm → the
// confirmed span reports not bookable → preview → cancel — plus the
// missing-window rejection. Cleanup drops the cancelled row (bookings have no
// delete RPC) so the unit teardown isn't blocked by its RESTRICT reference.
func bookingFlow(t *testing.T, c *e2eClients, unit string) {
	t.Helper()
	ctx := context.Background()

	_, err := c.bookings.RescheduleBooking(ctx, &schedulingpbv1.RescheduleBookingRequest{Name: "bookings/nope"})
	wantCode(t, err, codes.InvalidArgument, "reschedule without window")

	start := time.Now().UTC().AddDate(0, 0, 30).Truncate(24 * time.Hour)
	window := &sharedpbv1.TimeWindow{
		StartTime: timestamppb.New(start),
		EndTime:   timestamppb.New(start.AddDate(0, 0, 2)),
	}
	booking, err := c.bookings.CreateBooking(ctx, &schedulingpbv1.CreateBookingRequest{
		Booking: &schedulingpbv1.Booking{
			Unit:    unit,
			Window:  window,
			Contact: &sharedpbv1.Contact{DisplayName: "E2E Guest", Email: "guest-" + c.suffix + "@example.com"},
		},
	})
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}
	t.Cleanup(func() {
		if err := c.bookRepos.Bookings.Delete(ctx, booking.GetName()); err != nil {
			t.Logf("delete booking row: %v", err)
		}
	})
	if booking.GetState() != schedulingpbv1.BookingState_BOOKING_STATE_PENDING_HOLD {
		t.Fatalf("booking state = %v, want PENDING_HOLD", booking.GetState())
	}

	confirmed, err := c.bookings.ConfirmBooking(ctx, &schedulingpbv1.ConfirmBookingRequest{Name: booking.GetName()})
	if err != nil || confirmed.GetState() != schedulingpbv1.BookingState_BOOKING_STATE_CONFIRMED {
		t.Fatalf("ConfirmBooking: %v (state %v)", err, confirmed.GetState())
	}

	// The confirmed stay occupies the span: the same window is no longer bookable.
	check, err := c.avail.CheckAvailability(ctx, &availabilitypbv1.CheckAvailabilityRequest{
		Unit:   unit,
		Period: &availabilitypbv1.CheckAvailabilityRequest_Window{Window: window},
	})
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if check.GetBookable() {
		t.Fatal("CheckAvailability: confirmed span still reports bookable")
	}

	if _, err := c.bookings.PreviewCancellation(ctx, &schedulingpbv1.PreviewCancellationRequest{Name: booking.GetName()}); err != nil {
		t.Fatalf("PreviewCancellation: %v", err)
	}
	cancelled, err := c.bookings.CancelBooking(ctx, &schedulingpbv1.CancelBookingRequest{Name: booking.GetName()})
	if err != nil || cancelled.GetState() != schedulingpbv1.BookingState_BOOKING_STATE_CANCELLED {
		t.Fatalf("CancelBooking: %v (state %v)", err, cancelled.GetState())
	}
}
