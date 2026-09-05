package hasura

import (
	"context"
	"fmt"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/commonql/moneysql"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"

	resourceql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"github.com/the-protobuf-project/runtime-go/network/graphql"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ExpireHolds flips every PENDING_HOLD booking whose hold has lapsed to EXPIRED,
// freeing the capacity it reserved. Returns the number of holds expired. Timestamps
// are stored as fixed-width RFC 3339 UTC strings, so a lexical `<` matches
// chronological order. Intended to be called periodically by the hold sweeper.
func (r *BookingRepository) ExpireHolds(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	nowStr := dbutil.TsToStr(timestamppb.New(now))
	rows, err := r.svc.Query.Booking.Resource.List(ctx, resourceql.List().Where(resourceql.And(
		resourceql.State.Eq("PENDING_HOLD"),
		resourceql.HoldExpireTime.Lt(nowStr),
	)))
	if err != nil {
		return 0, dbutil.MapHasuraErr(err)
	}
	var expired int64
	for i := range rows {
		patch := resourceql.UpdateInput{
			State:      graphql.Value("EXPIRED"),
			Etag:       graphql.Value(ulid.GenerateString()),
			UpdateTime: graphql.Value(nowStr),
		}
		if _, err := r.svc.Mutation.Booking.Resource.Update(ctx, rows[i].Id, patch); err != nil {
			return expired, dbutil.MapHasuraErr(err)
		}
		expired++
	}
	return expired, nil
}

// ConfirmBooking flips a PENDING_HOLD booking to CONFIRMED.
func (r *BookingRepository) ConfirmBooking(ctx context.Context, name string) (*bookingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	res, err := r.svc.Query.Booking.Resource.Get(ctx, id)
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	if res == nil {
		return nil, types.ErrNotFound
	}
	if res.State == nil || *res.State != "PENDING_HOLD" {
		return nil, fmt.Errorf("%w: only a booking on hold can be confirmed", types.ErrInvalidState)
	}
	now := time.Now().UTC()
	patch := resourceql.UpdateInput{
		State:          graphql.Value("CONFIRMED"),
		ConfirmTime:    graphql.Value(dbutil.TsToStr(timestamppb.New(now))),
		HoldExpireTime: graphql.Null[string](),
		Etag:           graphql.Value(ulid.GenerateString()),
		UpdateTime:     graphql.Value(dbutil.TsToStr(timestamppb.New(now))),
	}
	// CAS: only a still-held, unchanged booking confirms; a concurrent cancel,
	// expiry, or double-confirm loses the race and gets Conflict instead of
	// silently overwriting the state that won.
	match := resourceql.State.Eq("PENDING_HOLD")
	if res.Etag != nil {
		match = resourceql.And(match, resourceql.Etag.Eq(*res.Etag))
	}
	if _, err := r.svc.Mutation.Booking.Resource.UpdateIfMatch(ctx, id, patch, match); err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	return r.GetBooking(ctx, name)
}

// CancelBooking flips a held or confirmed booking to CANCELLED, computing the
// refund from the unit's cancellation policy, in one batch.
func (r *BookingRepository) CancelBooking(ctx context.Context, name string, reason bookingpbv1.CancelReason) (*bookingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	res, err := r.svc.Query.Booking.Resource.Get(ctx, id)
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	if res == nil {
		return nil, types.ErrNotFound
	}
	// Already cancelled: the caller's intent is satisfied. Return the booking as
	// it stands rather than recomputing a refund whose percent could have drifted
	// with the policy clock since the original cancel.
	if res.State != nil && *res.State == "CANCELLED" {
		return r.GetBooking(ctx, name)
	}
	// An expired hold released its inventory when it lapsed; there is nothing to
	// cancel. A state error, not an inventory conflict.
	if res.State != nil && *res.State == "EXPIRED" {
		return nil, fmt.Errorf("%w: the hold expired and cannot be cancelled", types.ErrInvalidState)
	}
	pct, amount, _, err := r.computeRefund(ctx, res)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	patch := resourceql.UpdateInput{
		State:         graphql.Value("CANCELLED"),
		CancelTime:    graphql.Value(dbutil.TsToStr(timestamppb.New(now))),
		CancelReason:  dbutil.NullableStr(cancelReasonToStr(reason)),
		RefundPercent: graphql.Value(pct),
		Etag:          graphql.Value(ulid.GenerateString()),
		UpdateTime:    graphql.Value(dbutil.TsToStr(timestamppb.New(now))),
	}
	// CAS: the cancel only lands if the booking hasn't reached a terminal state
	// (or been otherwise modified) since the read above. The refund Money insert
	// rides in the same batch; if the guarded update matches no row, the batch
	// still commits the insert, so the zero-affected-rows path reaps it.
	match := resourceql.Not(resourceql.State.In("CANCELLED", "EXPIRED"))
	if res.Etag != nil {
		match = resourceql.And(match, resourceql.Etag.Eq(*res.Etag))
	}
	tx := r.svc.Mutation.Tx()
	refundID := ""
	if amount != nil {
		mi := moneyInput(amount)
		var mRes moneysql.InsertCommonMoneysResponse
		tx.Add(r.svc.Mutation.Common.Moneys.CreateOp(mi, &mRes))
		patch.RefundAmountId = graphql.Value(mi.Id)
		refundID = mi.Id
	}
	var updRes resourceql.UpdateBookingResourceByIdResponse
	tx.Add(r.svc.Mutation.Booking.Resource.UpdateOp(id, patch, &updRes, resourceql.Update().PreCheck(match)))
	if err := tx.Commit(ctx); err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	if updRes.AffectedRows == 0 {
		if refundID != "" {
			_, _ = r.svc.Mutation.Common.Moneys.Delete(ctx, refundID) // reap the orphan
		}
		// The guarded update matched nothing: the booking reached a terminal state
		// between our read and our write. If that state is CANCELLED, a concurrent
		// cancel beat us to it and the caller's intent is still satisfied — same
		// answer as the already-cancelled path above. Anything else is a real race.
		current, err := r.svc.Query.Booking.Resource.Get(ctx, id)
		if err != nil {
			return nil, dbutil.MapHasuraErr(err)
		}
		if current != nil && current.State != nil && *current.State == "CANCELLED" {
			return r.GetBooking(ctx, name)
		}
		return nil, types.ErrConflict
	}
	return r.GetBooking(ctx, name)
}

// PreviewCancellation computes the refund a cancellation would yield now, without
// cancelling.
func (r *BookingRepository) PreviewCancellation(ctx context.Context, name string) (refundable bool, percent int32, amount, nonRefundable *money.Money, summary string, err error) {
	id, err := types.BookingID(name)
	if err != nil {
		return false, 0, nil, nil, "", err
	}
	res, err := r.svc.Query.Booking.Resource.Get(ctx, id)
	if err != nil {
		return false, 0, nil, nil, "", dbutil.MapHasuraErr(err)
	}
	if res == nil {
		return false, 0, nil, nil, "", types.ErrNotFound
	}
	percent, amount, summary, err = r.computeRefund(ctx, res)
	if err != nil {
		return false, 0, nil, nil, "", err
	}
	var total *money.Money
	if res.TotalId != nil {
		if total, err = r.money(ctx, *res.TotalId); err != nil {
			return false, 0, nil, nil, "", err
		}
	}
	nonRefundable = moneySub(total, amount)
	return percent > 0, percent, amount, nonRefundable, summary, nil
}
