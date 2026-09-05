package hasura

// Live-DDN integration tests: the first real verification of the Hasura
// provider (everything else is compile-only). They exercise the full booking
// lifecycle against a running engine — guest-graph inserts, the native
// delete_identity_guests_by_booking_id predicate delete, the etag/state CAS
// guards, party validation, and hydration.
//
// Skipped unless FREEBUSY_TEST_GRAPHQL_URL points at a live engine:
//
//	FREEBUSY_TEST_GRAPHQL_URL=http://localhost:3280/graphql \
//	  go test ./internal/service/booking/db/hasura/ -run Live -v
//
// Each run seeds a fresh organisation → property → unit chain under new ULIDs,
// so runs never collide; rows are left behind (local dev database).

import (
	"context"
	"errors"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/organisationql/resourceql"
	propertiesql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/propertiesql"
	unitsql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/unitsql"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// liveService connects to the engine named by FREEBUSY_TEST_GRAPHQL_URL, or
// skips the test when unset.
func liveService(t *testing.T) *freebusyql.Service {
	t.Helper()
	raw := os.Getenv("FREEBUSY_TEST_GRAPHQL_URL")
	if raw == "" {
		t.Skip("FREEBUSY_TEST_GRAPHQL_URL not set — live DDN integration tests skipped")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	svc, err := freebusyql.Connect(u)
	if err != nil {
		t.Fatalf("connect %s: %v", raw, err)
	}
	return svc
}

// seedUnit inserts a fresh organisation → property → unit chain and returns the
// unit's resource name. The unit takes 1 bookable unit of capacity with a max
// occupancy of 2 guests, in UTC, with no price (pricing is exercised by the
// gorm unit tests; here the lifecycle is the subject).
func seedUnit(t *testing.T, svc *freebusyql.Service) string {
	t.Helper()
	ctx := context.Background()
	now := dbutil.TsToStr(timestamppb.New(time.Now().UTC()))
	orgID, propID, unitID := ulid.GenerateString(), ulid.GenerateString(), ulid.GenerateString()

	if _, err := svc.Mutation.Organisation.Resource.Create(ctx, resourceql.CreateInput{
		Id:          orgID,
		Name:        "organisations/" + orgID,
		DisplayName: "it-org",
		CreateTime:  now,
		UpdateTime:  now,
	}); err != nil {
		t.Fatalf("seed organisation: %v", err)
	}
	if _, err := svc.Mutation.Property.Properties.Create(ctx, propertiesql.CreateInput{
		Id:           propID,
		Name:         "properties/" + propID,
		DisplayName:  "it-property",
		Organisation: orgID,
		TimeZone:     "UTC",
		CreateTime:   now,
		UpdateTime:   now,
	}); err != nil {
		t.Fatalf("seed property: %v", err)
	}
	unitName := "properties/" + propID + "/units/" + unitID
	if _, err := svc.Mutation.Property.Units.Create(ctx, unitsql.CreateInput{
		Id:           unitID,
		Name:         unitName,
		DisplayName:  "it-room",
		PropertyId:   propID,
		TimeZone:     "UTC",
		Capacity:     1,
		MaxOccupancy: 2,
		BookingMode:  "NIGHTLY",
		CreateTime:   now,
		UpdateTime:   now,
	}); err != nil {
		t.Fatalf("seed unit: %v", err)
	}
	return unitName
}

func guest(name string, age identitypbv1.AgeGroup) *identitypbv1.Guest {
	return &identitypbv1.Guest{DisplayName: name, AgeGroup: age}
}

// TestBookingLifecycleLive walks a booking through create → replace party →
// confirm → cancel on a live engine, asserting the CAS and party guards bite.
func TestBookingLifecycleLive(t *testing.T) {
	svc := liveService(t)
	repo := NewBookingRepository(svc)
	ctx := context.Background()
	unitName := seedUnit(t, svc)

	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Hour)
	window := &sharedpbv1.TimeWindow{
		StartTime: timestamppb.New(start),
		EndTime:   timestamppb.New(start.Add(48 * time.Hour)),
	}

	// Create with a one-adult party.
	created, err := repo.CreateBooking(ctx, &bookingpbv1.Booking{
		Unit:      unitName,
		Window:    window,
		Guests:    []*identitypbv1.Guest{guest("Asha", identitypbv1.AgeGroup_AGE_GROUP_ADULT)},
		Occupancy: &bookingpbv1.Occupancy{Adults: 1},
	})
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}
	if created.GetState() != bookingpbv1.BookingState_BOOKING_STATE_PENDING_HOLD || len(created.GetGuests()) != 1 {
		t.Fatalf("created state=%v guests=%d, want PENDING_HOLD with 1 guest", created.GetState(), len(created.GetGuests()))
	}
	name := created.GetName()

	// Replace the party: 2 adults. Exactly 2 guests afterwards proves the
	// native delete-by-booking_id removed the old party (no doubling).
	updated, err := repo.UpdateBookingGuests(ctx, name,
		[]*identitypbv1.Guest{
			guest("Asha", identitypbv1.AgeGroup_AGE_GROUP_ADULT),
			guest("Ravi", identitypbv1.AgeGroup_AGE_GROUP_ADULT),
		},
		&bookingpbv1.Occupancy{Adults: 2},
	)
	if err != nil {
		t.Fatalf("UpdateBookingGuests: %v", err)
	}
	if len(updated.GetGuests()) != 2 || updated.GetOccupancy().GetAdults() != 2 {
		t.Fatalf("party after replace: guests=%d occupancy=%+v, want 2/2", len(updated.GetGuests()), updated.GetOccupancy())
	}
	if updated.GetEtag() == created.GetEtag() {
		t.Fatal("etag must change on a party replace")
	}

	// Overflow: 5 adults on a max-occupancy-2 unit is rejected.
	if _, err := repo.UpdateBookingGuests(ctx, name, nil, &bookingpbv1.Occupancy{Adults: 5}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("overflow party: err=%v, want ErrInvalidArgument", err)
	}

	// Confirm; a second confirm loses the state/etag CAS.
	confirmed, err := repo.ConfirmBooking(ctx, name)
	if err != nil {
		t.Fatalf("ConfirmBooking: %v", err)
	}
	if confirmed.GetState() != bookingpbv1.BookingState_BOOKING_STATE_CONFIRMED {
		t.Fatalf("state=%v, want CONFIRMED", confirmed.GetState())
	}
	if _, err := repo.ConfirmBooking(ctx, name); !errors.Is(err, types.ErrInvalidState) {
		t.Fatalf("double confirm: err=%v, want ErrInvalidState", err)
	}

	// Party edits stay legal while CONFIRMED.
	if _, err := repo.UpdateBookingGuests(ctx, name,
		[]*identitypbv1.Guest{guest("Asha", identitypbv1.AgeGroup_AGE_GROUP_ADULT)},
		&bookingpbv1.Occupancy{Adults: 1},
	); err != nil {
		t.Fatalf("UpdateBookingGuests on CONFIRMED: %v", err)
	}

	// Cancel; then the party is frozen.
	cancelled, err := repo.CancelBooking(ctx, name, bookingpbv1.CancelReason_CANCEL_REASON_REQUESTED_BY_CUSTOMER)
	if err != nil {
		t.Fatalf("CancelBooking: %v", err)
	}
	if cancelled.GetState() != bookingpbv1.BookingState_BOOKING_STATE_CANCELLED {
		t.Fatalf("state=%v, want CANCELLED", cancelled.GetState())
	}
	if _, err := repo.UpdateBookingGuests(ctx, name, nil, nil); !errors.Is(err, types.ErrInvalidState) {
		t.Fatalf("party edit on CANCELLED: err=%v, want ErrInvalidState", err)
	}

	// Re-cancelling is idempotent: the caller's intent is already satisfied, so it
	// returns the cancelled booking rather than an error a client cannot tell
	// apart from someone taking the last room.
	recancelled, err := repo.CancelBooking(ctx, name, bookingpbv1.CancelReason_CANCEL_REASON_REQUESTED_BY_CUSTOMER)
	if err != nil {
		t.Fatalf("re-cancel should be idempotent: %v", err)
	}
	if recancelled.GetState() != bookingpbv1.BookingState_BOOKING_STATE_CANCELLED {
		t.Fatalf("re-cancel state=%v, want CANCELLED", recancelled.GetState())
	}
	if recancelled.GetCancelTime().AsTime() != cancelled.GetCancelTime().AsTime() {
		t.Fatalf("re-cancel moved cancel_time: %v -> %v; a retry must return the first cancel verbatim",
			cancelled.GetCancelTime().AsTime(), recancelled.GetCancelTime().AsTime())
	}
}
