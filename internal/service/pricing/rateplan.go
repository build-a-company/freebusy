// Package pricing is the gRPC/protobuf layer for the rate-plan service: it
// implements pricingpbv1.RatePlanServiceServer, owning request validation,
// observability, and the mapping of repository errors to gRPC status codes.
// Persistence stays behind the generated, provider-agnostic RatePlanRepository,
// so the database layer knows nothing of protobuf or gRPC.
package pricing

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/database"
	pricingrepo "github.com/oh-tarnished/freebusy/internal/database/repository/freebusy/pricing"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/runtime/rpc"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/pricing/v1/pricingpbv1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Server implements pricingpbv1.RatePlanServiceServer over the generated
// repository, which the pricing factory selects per provider (GORM or Hasura).
//
// Thinner than the promo-code or booking servers on purpose: a rate plan is
// plain CRUD with no derived state and no cross-resource invariant, so there is
// nothing for a hand-written service layer to own. Everything the generated
// repository already does — masks, etags, pagination, filtering — is used as is
// rather than wrapped.
type Server struct {
	pricingpbv1.UnimplementedRatePlanServiceServer
	repo pricingrepo.RatePlanRepository
}

// New builds the rate-plan service on conn.
func New(conn *database.Connection) *Server {
	if conn.Provider == database.ProviderHasura {
		return NewServer(pricingrepo.NewGraphQL(conn.Hasura).RatePlans)
	}
	return NewServer(pricingrepo.NewGorm(conn.PgSQLConn).RatePlans)
}

// NewServer returns a Server backed by repo.
func NewServer(repo pricingrepo.RatePlanRepository) *Server {
	return &Server{repo: repo}
}

// GetRatePlan returns one rate plan by resource name.
func (s *Server) GetRatePlan(ctx context.Context, req *pricingpbv1.GetRatePlanRequest) (*pricingpbv1.RatePlan, error) {
	var out *pricingpbv1.RatePlan
	err := rpc.Traced(ctx, "RatePlanService", "GetRatePlan", func(ctx context.Context) error {
		m, err := s.repo.Get(ctx, req.GetName())
		if err != nil {
			return rpc.ToStatusErr(err)
		}
		out = m
		return nil
	})
	return out, err
}

// ListRatePlans returns a page of rate plans.
//
// The filter matters more here than on most collections: a caller almost always
// wants the plans for one resource, and `resource="resources/x"` is how it asks.
func (s *Server) ListRatePlans(ctx context.Context, req *pricingpbv1.ListRatePlansRequest) (*pricingpbv1.ListRatePlansResponse, error) {
	var out *pricingpbv1.ListRatePlansResponse
	err := rpc.Traced(ctx, "RatePlanService", "ListRatePlans", func(ctx context.Context) error {
		items, next, err := s.repo.List(ctx, repox.ListInput{
			PageSize:  req.GetPageSize(),
			PageToken: req.GetPageToken(),
			Filter:    req.GetFilter(),
		})
		if err != nil {
			return rpc.ToStatusErr(err)
		}
		out = &pricingpbv1.ListRatePlansResponse{RatePlans: items, NextPageToken: next}
		return nil
	})
	return out, err
}

// CreateRatePlan stores a new rate plan.
func (s *Server) CreateRatePlan(ctx context.Context, req *pricingpbv1.CreateRatePlanRequest) (*pricingpbv1.RatePlan, error) {
	var out *pricingpbv1.RatePlan
	err := rpc.Traced(ctx, "RatePlanService", "CreateRatePlan", func(ctx context.Context) error {
		m, err := s.repo.Create(ctx, req.GetRatePlan())
		if err != nil {
			return rpc.ToStatusErr(err)
		}
		out = m
		return nil
	})
	return out, err
}

// UpdateRatePlan applies the masked fields of the supplied plan.
//
// Editing a plan changes what future quotes cost and never reprices a booking
// already taken: a booking carries the components it was quoted under, so the
// two are deliberately not linked.
func (s *Server) UpdateRatePlan(ctx context.Context, req *pricingpbv1.UpdateRatePlanRequest) (*pricingpbv1.RatePlan, error) {
	var out *pricingpbv1.RatePlan
	err := rpc.Traced(ctx, "RatePlanService", "UpdateRatePlan", func(ctx context.Context) error {
		m, err := s.repo.Update(ctx, req.GetRatePlan(), req.GetUpdateMask().GetPaths())
		if err != nil {
			return rpc.ToStatusErr(err)
		}
		out = m
		return nil
	})
	return out, err
}

// DeleteRatePlan removes a rate plan.
//
// Archiving (state ARCHIVED via Update) is preferable and the proto says so: a
// booking priced under a plan should stay explainable after the plan stops being
// offered, and a delete takes that record away. The method exists because
// AIP-135 requires it, not because it is the expected path.
func (s *Server) DeleteRatePlan(ctx context.Context, req *pricingpbv1.DeleteRatePlanRequest) (*emptypb.Empty, error) {
	err := rpc.Traced(ctx, "RatePlanService", "DeleteRatePlan", func(ctx context.Context) error {
		if err := s.repo.Delete(ctx, req.GetName()); err != nil {
			return rpc.ToStatusErr(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
