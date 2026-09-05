// Package db is the booking persistence layer. It defines the provider-agnostic
// BookingRepository contract (spoken in protobuf domain types) and a factory that
// builds the implementation for the configured backend. Shared, provider-neutral
// vocabulary (errors, list params, names, field masks) lives in internal/types.
package db

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/database"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/scheduling/db/gorm"
	"github.com/oh-tarnished/freebusy/internal/service/scheduling/db/hasura"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
	"google.golang.org/genproto/googleapis/type/money"
)

// BookingRepository provides the booking lifecycle: placing holds, confirming,
// cancelling (with refund), rescheduling, and reads. Errors are the sentinels in
// internal/types.
type BookingRepository interface {
	// CreateBooking places a PENDING_HOLD on a unit for a window (server-computed
	// price/state/hold-expiry). Rejects with types.ErrCapacityExhausted when the
	// window is full.
	CreateBooking(ctx context.Context, b *schedulingpbv1.Booking) (*schedulingpbv1.Booking, error)

	// PreviewBooking validates and prices a draft without placing a hold: the
	// same occupancy, capacity, and pricing rules CreateBooking applies, minus
	// the write. Backs validate_only.
	PreviewBooking(ctx context.Context, b *schedulingpbv1.Booking) (*schedulingpbv1.Booking, error)

	// GetBooking returns the booking by resource name.
	GetBooking(ctx context.Context, name string) (*schedulingpbv1.Booking, error)

	// ListBookings returns a page of bookings.
	ListBookings(ctx context.Context, in repox.ListInput) (items []*schedulingpbv1.Booking, nextPageToken string, err error)

	// ConfirmBooking flips a held booking to CONFIRMED.
	ConfirmBooking(ctx context.Context, name string) (*schedulingpbv1.Booking, error)

	// CancelBooking cancels a booking, computing the refund from the unit's
	// cancellation policy.
	CancelBooking(ctx context.Context, name string, reason schedulingpbv1.CancelReason) (*schedulingpbv1.Booking, error)

	// PreviewCancellation computes the refund a cancellation would yield now.
	PreviewCancellation(ctx context.Context, name string) (refundable bool, percent int32, amount, nonRefundable *money.Money, summary string, err error)

	// RescheduleBooking moves a booking to a new window (and optionally unit),
	// re-checking capacity and recomputing the price.
	RescheduleBooking(ctx context.Context, name string, b *schedulingpbv1.Booking, newUnit string) (*schedulingpbv1.Booking, error)

	// ExpireHolds flips every PENDING_HOLD booking whose hold has lapsed to
	// EXPIRED, freeing the capacity it reserved, and returns how many it expired.
	// Called periodically by the hold sweeper.
	ExpireHolds(ctx context.Context) (int64, error)

	// UpdateBookingGuests replaces the whole staying party (guests + occupancy) on
	// a booking. Allowed only while PENDING_HOLD or CONFIRMED (types.ErrConflict
	// otherwise); re-validates the party against the unit's max occupancy
	// (types.ErrInvalidArgument when it overflows).
	UpdateBookingGuests(ctx context.Context, name string, guests []*identitypbv1.Guest, occupancy *schedulingpbv1.Occupancy) (*schedulingpbv1.Booking, error)
}

// Assert the provider implementations satisfy the contract here, so the
// sub-packages don't need to import this one (which would form an import cycle).
var (
	_ BookingRepository = (*gorm.BookingRepository)(nil)
	_ BookingRepository = (*hasura.BookingRepository)(nil)
)

// New returns the BookingRepository for the configured provider, built over the
// matching handle on conn (conn.Provider).
func New(conn *database.Connection) BookingRepository {
	if conn.Provider == database.ProviderHasura {
		return instrumentHasuraBooking(hasura.NewBookingRepository(conn.Hasura))
	}
	return gorm.NewBookingRepository(conn.PgSQLConn)
}
