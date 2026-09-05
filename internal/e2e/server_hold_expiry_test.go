// The hold-expiry flow of the server e2e suite: a lapsed PENDING_HOLD frees its
// inventory the moment it expires — availability and the create-capacity check
// ignore it WITHOUT a sweep — and the sweeper afterwards only converges the
// stored state to EXPIRED. This pins the authoritative-read behavior on both
// providers; regressing it re-locks inventory for a whole sweep interval.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/oh-tarnished/freebusy/internal/service/scheduling/db"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/availability/v1/availabilitypbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// holdExpiryTTL is deliberately tiny so the flow can outwait a real expiry.
const holdExpiryTTL = 2 * time.Second

func holdExpiryFlow(t *testing.T, c *e2eClients, unit string, holds db.BookingRepository) {
	t.Helper()
	ctx := context.Background()

	// A window of its own, far from the other flows' spans.
	start := time.Now().UTC().AddDate(0, 0, 40).Truncate(24 * time.Hour)
	window := &sharedpbv1.TimeWindow{
		StartTime: timestamppb.New(start),
		EndTime:   timestamppb.New(start.AddDate(0, 0, 2)),
	}

	first := createHold(t, c, unit, window)

	// While the hold is live it occupies the unit's capacity.
	if checkWindow(t, c, unit, window).GetBookable() {
		t.Fatal("live hold should make the span not bookable")
	}

	// Let the hold lapse. No sweep runs here: the reads themselves must free it.
	time.Sleep(holdExpiryTTL + time.Second)

	if !checkWindow(t, c, unit, window).GetBookable() {
		t.Fatal("lapsed hold still blocks availability; reads should ignore it without a sweep")
	}
	createHold(t, c, unit, window) // the capacity check must also ignore the lapsed hold

	// The sweeper is bookkeeping only: it converges the stored state to EXPIRED.
	if _, err := holds.ExpireHolds(ctx); err != nil {
		t.Fatalf("ExpireHolds: %v", err)
	}
	got, err := c.bookings.GetBooking(ctx, &schedulingpbv1.GetBookingRequest{Name: first.GetName()})
	if err != nil {
		t.Fatalf("GetBooking after sweep: %v", err)
	}
	if got.GetState() != schedulingpbv1.BookingState_BOOKING_STATE_EXPIRED {
		t.Fatalf("first booking state = %v, want EXPIRED after sweep", got.GetState())
	}
}

// createHold places a short-TTL PENDING_HOLD on unit over window and registers
// the row's cleanup (bookings have no delete RPC).
func createHold(t *testing.T, c *e2eClients, unit string, window *sharedpbv1.TimeWindow) *schedulingpbv1.Booking {
	t.Helper()
	ctx := context.Background()
	b, err := c.bookings.CreateBooking(ctx, &schedulingpbv1.CreateBookingRequest{
		Booking: &schedulingpbv1.Booking{
			Unit:    unit,
			Window:  window,
			Contact: &sharedpbv1.Contact{DisplayName: "Hold Expiry", Email: "hold-" + c.suffix + "@example.com"},
			HoldTtl: durationpb.New(holdExpiryTTL),
		},
	})
	if err != nil {
		t.Fatalf("CreateBooking (short hold): %v", err)
	}
	t.Cleanup(func() {
		if err := c.bookRepos.Bookings.Delete(ctx, b.GetName()); err != nil {
			t.Logf("delete hold row %s: %v", b.GetName(), err)
		}
	})
	if b.GetState() != schedulingpbv1.BookingState_BOOKING_STATE_PENDING_HOLD {
		t.Fatalf("booking state = %v, want PENDING_HOLD", b.GetState())
	}
	return b
}

// checkWindow asks the availability service whether window is bookable on unit.
func checkWindow(t *testing.T, c *e2eClients, unit string, window *sharedpbv1.TimeWindow) *availabilitypbv1.CheckAvailabilityResponse {
	t.Helper()
	check, err := c.avail.CheckAvailability(context.Background(), &availabilitypbv1.CheckAvailabilityRequest{
		Unit:   unit,
		Period: &availabilitypbv1.CheckAvailabilityRequest_Window{Window: window},
	})
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	return check
}
