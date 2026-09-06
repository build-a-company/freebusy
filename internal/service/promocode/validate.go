package promocode

import (
	"context"
	"errors"
	"time"

	"github.com/oh-tarnished/freebusy/internal/runtime/rpc"
	"github.com/oh-tarnished/freebusy/internal/service/promocode/discount"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ValidatePromoCode resolves a human-entered code and evaluates it against the
// prospective booking (subtotal, optional resource and offering). A code that is
// absent or not redeemable for this booking is returned as a gRPC status error
// (NOT_FOUND, FAILED_PRECONDITION, RESOURCE_EXHAUSTED, or INVALID_ARGUMENT); only
// a redeemable code yields a response, carrying the discount and final total.
func (s *Server) ValidatePromoCode(ctx context.Context, req *promocodepbv1.ValidatePromoCodeRequest) (*promocodepbv1.ValidatePromoCodeResponse, error) {
	var out *promocodepbv1.ValidatePromoCodeResponse
	err := rpc.Traced(ctx, "PromoCodeService", "ValidatePromoCode", func(ctx context.Context) error {
		pc, err := s.repo.FindByCode(ctx, req.GetCode())
		if err != nil {
			if errors.Is(err, types.ErrNotFound) {
				return status.Error(codes.NotFound, "promo code not found")
			}
			return rpc.ToStatusErr(err)
		}

		result := discount.Evaluate(pc, req.GetSubtotal(), req.GetProperty(), req.GetUnit(), time.Now().UTC())
		if !result.Valid {
			return status.Error(reasonCode(result.Reason), result.Reason.Message())
		}
		out = &promocodepbv1.ValidatePromoCodeResponse{
			Valid:          true,
			PromoCode:      pc.GetName(),
			DiscountAmount: result.Discount,
			FinalTotal:     result.FinalTotal,
		}
		return nil
	})
	return out, err
}

// reasonCode maps a discount non-redeemable reason to the gRPC status code the
// validate endpoint returns for it.
func reasonCode(r discount.Reason) codes.Code {
	switch r {
	case discount.ReasonLimitReached:
		return codes.ResourceExhausted
	case discount.ReasonCurrencyMismatch:
		return codes.InvalidArgument
	default:
		// disabled, expired, not-yet-redeemable, below-minimum, not-applicable.
		return codes.FailedPrecondition
	}
}
