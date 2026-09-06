// Package schedule is the gRPC/protobuf layer for the ScheduleService: it
// implements schedulepbv1.ScheduleServiceServer, owning observability and the
// mapping of repository errors to gRPC status codes. Request validation is
// enforced by the buf.validate rules on the protos via the server-wide
// protovalidate interceptor. Persistence stays behind the provider-agnostic
// db.ScheduleRepository.
package schedule

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/database"

	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/runtime/rpc"
	"github.com/oh-tarnished/freebusy/internal/service/schedule/db"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/schedule/v1/schedulepbv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Server implements schedulepbv1.ScheduleServiceServer on top of a
// provider-agnostic db.ScheduleRepository.
type Server struct {
	schedulepbv1.UnimplementedScheduleServiceServer
	repo db.ScheduleRepository
}

// New builds the schedule service on conn: the provider-selected repository
// wrapped in the gRPC server implementation.
func New(conn *database.Connection) *Server {
	return NewServer(db.New(conn))
}

// NewServer returns a Server backed by repo.
func NewServer(repo db.ScheduleRepository) *Server {
	return &Server{repo: repo}
}

// --- Schedule ----------------------------------------------------------------

// GetSchedule returns a unit's full availability configuration.
func (s *Server) GetSchedule(ctx context.Context, req *schedulepbv1.GetScheduleRequest) (*schedulepbv1.Schedule, error) {
	var out *schedulepbv1.Schedule
	err := rpc.Traced(ctx, "ScheduleService", "GetSchedule", func(ctx context.Context) error {
		sc, err := s.repo.GetSchedule(ctx, req.GetName())
		if err != nil {
			return rpc.ToStatusErr(err)
		}
		out = sc
		return nil
	})
	return out, err
}

// UpdateSchedule upserts the masked sections of a unit's schedule.
func (s *Server) UpdateSchedule(ctx context.Context, req *schedulepbv1.UpdateScheduleRequest) (*schedulepbv1.Schedule, error) {
	sc := req.GetSchedule()
	var out *schedulepbv1.Schedule
	err := rpc.Traced(ctx, "ScheduleService", "UpdateSchedule", func(ctx context.Context) error {
		updated, err := s.repo.UpdateSchedule(ctx, sc, req.GetUpdateMask().GetPaths())
		if err != nil {
			return rpc.ToStatusErr(err)
		}
		out = updated
		return nil
	})
	return out, err
}

// --- AvailabilityException ---------------------------------------------------

// ListAvailabilityExceptions returns a page of a unit's exceptions.
func (s *Server) ListAvailabilityExceptions(ctx context.Context, req *schedulepbv1.ListAvailabilityExceptionsRequest) (*schedulepbv1.ListAvailabilityExceptionsResponse, error) {
	var out *schedulepbv1.ListAvailabilityExceptionsResponse
	err := rpc.Traced(ctx, "ScheduleService", "ListAvailabilityExceptions", func(ctx context.Context) error {
		items, next, err := s.repo.ListAvailabilityExceptions(ctx, req.GetParent(), repox.ListInput{
			PageSize:  req.GetPageSize(),
			PageToken: req.GetPageToken(),
			OrderBy:   req.GetOrderBy(),
			Filter:    req.GetFilter(),
		})
		if err != nil {
			return rpc.ToStatusErr(err)
		}
		out = &schedulepbv1.ListAvailabilityExceptionsResponse{AvailabilityExceptions: items, NextPageToken: next}
		return nil
	})
	return out, err
}

// GetAvailabilityException returns an exception by resource name.
func (s *Server) GetAvailabilityException(ctx context.Context, req *schedulepbv1.GetAvailabilityExceptionRequest) (*schedulepbv1.AvailabilityException, error) {
	var out *schedulepbv1.AvailabilityException
	err := rpc.Traced(ctx, "ScheduleService", "GetAvailabilityException", func(ctx context.Context) error {
		e, err := s.repo.GetAvailabilityException(ctx, req.GetName())
		if err != nil {
			return rpc.ToStatusErr(err)
		}
		out = e
		return nil
	})
	return out, err
}

// CreateAvailabilityException adds a closure or extra-hours exception to a unit.
func (s *Server) CreateAvailabilityException(ctx context.Context, req *schedulepbv1.CreateAvailabilityExceptionRequest) (*schedulepbv1.AvailabilityException, error) {
	e := proto.Clone(req.GetAvailabilityException()).(*schedulepbv1.AvailabilityException)
	if id := req.GetAvailabilityExceptionId(); id != "" {
		resourceID, uerr := types.ParseResource(req.GetParent())
		if uerr != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid parent")
		}
		name, nerr := types.AvailabilityExceptionName(resourceID, id)
		if nerr != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid availability_exception_id")
		}
		e.Name = name
	}
	var out *schedulepbv1.AvailabilityException
	err := rpc.Traced(ctx, "ScheduleService", "CreateAvailabilityException", func(ctx context.Context) error {
		created, err := s.repo.CreateAvailabilityException(ctx, req.GetParent(), e)
		if err != nil {
			return rpc.ToStatusErr(err)
		}
		out = created
		return nil
	})
	return out, err
}

// DeleteAvailabilityException removes an exception by resource name.
func (s *Server) DeleteAvailabilityException(ctx context.Context, req *schedulepbv1.DeleteAvailabilityExceptionRequest) (*emptypb.Empty, error) {
	err := rpc.Traced(ctx, "ScheduleService", "DeleteAvailabilityException", func(ctx context.Context) error {
		return rpc.ToStatusErr(s.repo.DeleteAvailabilityException(ctx, req.GetName()))
	})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
