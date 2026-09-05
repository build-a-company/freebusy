// The BookingGuests singleton sub-resource RPCs.
package booking

import (
	"context"
	"strings"

	"github.com/oh-tarnished/freebusy/internal/runtime/rpc"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetBookingGuests returns the staying party (guests + occupancy) on a booking.
func (s *Server) GetBookingGuests(ctx context.Context, req *bookingpbv1.GetBookingGuestsRequest) (*bookingpbv1.BookingGuests, error) {
	bookingName, ok := bookingNameFromGuestsName(req.GetName())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "name must be of the form bookings/{booking}/guests")
	}
	var out *bookingpbv1.BookingGuests
	err := rpc.Traced(ctx, "BookingService", "GetBookingGuests", func(ctx context.Context) error {
		b, err := s.repo.GetBooking(ctx, bookingName)
		if err != nil {
			return toStatusErr(err)
		}
		out = &bookingpbv1.BookingGuests{Name: req.GetName(), Guests: b.GetGuests(), Occupancy: b.GetOccupancy()}
		return nil
	})
	return out, err
}

// UpdateBookingGuests replaces the staying party (guests + occupancy) on a
// booking. Allowed only while PENDING_HOLD or CONFIRMED.
func (s *Server) UpdateBookingGuests(ctx context.Context, req *bookingpbv1.UpdateBookingGuestsRequest) (*bookingpbv1.BookingGuests, error) {
	bg := req.GetBookingGuests()
	bookingName, ok := bookingNameFromGuestsName(bg.GetName())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "booking_guests.name must be of the form bookings/{booking}/guests")
	}
	var out *bookingpbv1.BookingGuests
	err := rpc.Traced(ctx, "BookingService", "UpdateBookingGuests", func(ctx context.Context) error {
		b, err := s.repo.UpdateBookingGuests(ctx, bookingName, bg.GetGuests(), bg.GetOccupancy())
		if err != nil {
			return toStatusErr(err)
		}
		out = &bookingpbv1.BookingGuests{Name: bg.GetName(), Guests: b.GetGuests(), Occupancy: b.GetOccupancy()}
		return nil
	})
	return out, err
}

// bookingNameFromGuestsName strips the singleton "/guests" suffix from a
// BookingGuests resource name, returning the parent Booking's name.
func bookingNameFromGuestsName(name string) (string, bool) {
	const suffix = "/guests"
	if !strings.HasSuffix(name, suffix) {
		return "", false
	}
	booking := strings.TrimSuffix(name, suffix)
	if booking == "" {
		return "", false
	}
	return booking, true
}
