// Package booking is the gRPC/protobuf layer for the BookingService: it
// implements schedulingpbv1.SchedulingServiceServer, owning request validation,
// observability, and the mapping of repository errors to gRPC status codes.
// Persistence and the hold lifecycle stay behind db.BookingRepository.
package scheduling

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/database"

	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/runtime/rpc"
	"github.com/oh-tarnished/freebusy/internal/service/scheduling/db"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Server implements schedulingpbv1.SchedulingServiceServer on top of a
// provider-agnostic db.BookingRepository.
type Server struct {
	schedulingpbv1.UnimplementedSchedulingServiceServer
	repo db.BookingRepository
}

// New builds the booking service on conn: the provider-selected repository
// wrapped in the gRPC server implementation.
func New(conn *database.Connection) *Server {
	return NewServer(db.New(conn))
}

// NewServer returns a Server backed by repo.
func NewServer(repo db.BookingRepository) *Server {
	return &Server{repo: repo}
}

// CreateBooking places a hold on a unit for a window. validate_only checks the
// request without persisting a hold.
func (s *Server) CreateBooking(ctx context.Context, req *schedulingpbv1.CreateBookingRequest) (*schedulingpbv1.Booking, error) {
	b := proto.Clone(req.GetBooking()).(*schedulingpbv1.Booking)
	if id := req.GetBookingId(); id != "" {
		name, err := types.BookingName(id)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid booking_id")
		}
		b.Name = name
	}
	if req.GetValidateOnly() {
		// Dry run: price it and check availability + occupancy against the real
		// rules, but place no hold. This used to echo the draft straight back,
		// which quoted a total of zero.
		var out *schedulingpbv1.Booking
		err := rpc.Traced(ctx, "BookingService", "CreateBooking", func(ctx context.Context) error {
			previewed, err := s.repo.PreviewBooking(ctx, b)
			if err != nil {
				return toStatusErr(err)
			}
			out = previewed
			return nil
		})
		return out, err
	}
	var out *schedulingpbv1.Booking
	err := rpc.Traced(ctx, "BookingService", "CreateBooking", func(ctx context.Context) error {
		created, err := s.repo.CreateBooking(ctx, b)
		if err != nil {
			return toStatusErr(err)
		}
		out = created
		return nil
	})
	return out, err
}

// GetBooking returns a single booking by resource name.
func (s *Server) GetBooking(ctx context.Context, req *schedulingpbv1.GetBookingRequest) (*schedulingpbv1.Booking, error) {
	var out *schedulingpbv1.Booking
	err := rpc.Traced(ctx, "BookingService", "GetBooking", func(ctx context.Context) error {
		b, err := s.repo.GetBooking(ctx, req.GetName())
		if err != nil {
			return toStatusErr(err)
		}
		out = b
		return nil
	})
	return out, err
}

// ListBookings returns a page of bookings.
func (s *Server) ListBookings(ctx context.Context, req *schedulingpbv1.ListBookingsRequest) (*schedulingpbv1.ListBookingsResponse, error) {
	var out *schedulingpbv1.ListBookingsResponse
	err := rpc.Traced(ctx, "BookingService", "ListBookings", func(ctx context.Context) error {
		items, next, err := s.repo.ListBookings(ctx, repox.ListInput{
			PageSize:  req.GetPageSize(),
			PageToken: req.GetPageToken(),
			OrderBy:   req.GetOrderBy(),
			Filter:    req.GetFilter(),
		})
		if err != nil {
			return toStatusErr(err)
		}
		out = &schedulingpbv1.ListBookingsResponse{Bookings: items, NextPageToken: next}
		return nil
	})
	return out, err
}

// ConfirmBooking confirms a held booking.
func (s *Server) ConfirmBooking(ctx context.Context, req *schedulingpbv1.ConfirmBookingRequest) (*schedulingpbv1.Booking, error) {
	var out *schedulingpbv1.Booking
	err := rpc.Traced(ctx, "BookingService", "ConfirmBooking", func(ctx context.Context) error {
		b, err := s.repo.ConfirmBooking(ctx, req.GetName())
		if err != nil {
			return toStatusErr(err)
		}
		out = b
		return nil
	})
	return out, err
}

// CancelBooking cancels a booking, computing the refund from the cancellation policy.
func (s *Server) CancelBooking(ctx context.Context, req *schedulingpbv1.CancelBookingRequest) (*schedulingpbv1.Booking, error) {
	var out *schedulingpbv1.Booking
	err := rpc.Traced(ctx, "BookingService", "CancelBooking", func(ctx context.Context) error {
		b, err := s.repo.CancelBooking(ctx, req.GetName(), req.GetReason())
		if err != nil {
			return toStatusErr(err)
		}
		out = b
		return nil
	})
	return out, err
}

// PreviewCancellation reports the refund a cancellation would yield now.
func (s *Server) PreviewCancellation(ctx context.Context, req *schedulingpbv1.PreviewCancellationRequest) (*schedulingpbv1.PreviewCancellationResponse, error) {
	var out *schedulingpbv1.PreviewCancellationResponse
	err := rpc.Traced(ctx, "BookingService", "PreviewCancellation", func(ctx context.Context) error {
		refundable, pct, amount, nonRefundable, summary, err := s.repo.PreviewCancellation(ctx, req.GetName())
		if err != nil {
			return toStatusErr(err)
		}
		out = &schedulingpbv1.PreviewCancellationResponse{
			Refundable:          refundable,
			RefundPercent:       pct,
			RefundAmount:        amount,
			NonRefundableAmount: nonRefundable,
			PolicySummary:       summary,
		}
		return nil
	})
	return out, err
}

// toStatusErr maps repository sentinel errors onto gRPC status codes. Booking
// used to override the shared mapping here, because capacity exhaustion and
// every other conflict shared one sentinel and only booking cared about the
// difference. They are separate sentinels now (types.ErrCapacityExhausted,
// types.ErrInvalidState, types.ErrConflict), each with its own status code and
// ErrorInfo reason, so the shared mapping is the whole story.
func toStatusErr(err error) error {
	return rpc.ToStatusErr(err)
}

// RescheduleBooking moves a booking to a new span (and optionally unit).
func (s *Server) RescheduleBooking(ctx context.Context, req *schedulingpbv1.RescheduleBookingRequest) (*schedulingpbv1.Booking, error) {
	var out *schedulingpbv1.Booking
	err := rpc.Traced(ctx, "BookingService", "RescheduleBooking", func(ctx context.Context) error {
		b, err := s.repo.RescheduleBooking(ctx, req.GetName(), &schedulingpbv1.Booking{Window: req.GetWindow()}, req.GetUnit())
		if err != nil {
			return toStatusErr(err)
		}
		out = b
		return nil
	})
	return out, err
}
