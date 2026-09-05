// Rescheduling moves a booking to a new span/unit atomically over DDN mutations.
package hasura

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/sharedql/timewindowsql"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"
	"time"

	resourceql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/resourceql"
	moneysql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/commonql/moneysql"
	"github.com/oh-tarnished/freebusy/internal/service/booking/pricing"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"github.com/the-protobuf-project/runtime-go/network/graphql"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RescheduleBooking atomically moves a booking to a new window (and optionally a
// new unit), re-checking capacity and recomputing the full price breakdown.
func (r *BookingRepository) RescheduleBooking(ctx context.Context, name string, w *bookingpbv1.Booking, newUnit string) (*bookingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	if w.GetWindow() == nil {
		return nil, types.ErrInvalidArgument
	}
	res, err := r.svc.Query.Booking.Resource.Get(ctx, id)
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	if res == nil {
		return nil, types.ErrNotFound
	}
	unitID := res.Unit
	if newUnit != "" {
		_, uid, perr := types.ParseUnitParent(newUnit)
		if perr != nil {
			return nil, perr
		}
		unitID = uid
	}
	unit, err := r.svc.Query.Property.Units.Get(ctx, unitID)
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	if unit == nil {
		return nil, types.ErrNotFound
	}

	requested := int64(repox.Deref(res.Units))
	if requested < 1 {
		requested = 1
	}
	reserved, err := r.reservedUnits(ctx, unitID, w.GetWindow(), id)
	if err != nil {
		return nil, err
	}
	capacity := int64(1)
	if unit.Capacity != nil && *unit.Capacity > 0 {
		capacity = int64(*unit.Capacity)
	}
	if reserved+requested > capacity {
		return nil, types.ErrCapacityExhausted
	}

	in, err := r.pricingInputs(ctx, unit, repox.Deref(res.PromoCode))
	if err != nil {
		return nil, err
	}
	in.Nights = nightsBetween(w.GetWindow(), unit.TimeZone)
	in.Units = requested

	var priceIn, discountIn, totalIn *moneysql.CreateInput
	var components []*sharedpbv1.PriceComponent
	priceID, discountID, totalID := "", "", ""
	if in.Price != nil {
		result := pricing.Compute(in, unitID)
		pi := moneyInput(result.Base)
		priceIn = &pi
		priceID = pi.Id
		ti := moneyInput(result.Total)
		totalIn = &ti
		totalID = ti.Id
		if !pricing.IsZero(result.Discount) {
			di := moneyInput(result.Discount)
			discountIn = &di
			discountID = di.Id
		}
		components = result.Components
	}

	now := time.Now().UTC()
	window := windowInput(w.GetWindow())
	patch := resourceql.UpdateInput{
		Unit:       graphql.Value(unitID),
		WindowId:   graphql.Value(window.Id),
		PriceId:    dbutil.NullableStr(priceID),
		DiscountId: dbutil.NullableStr(discountID),
		TotalId:    dbutil.NullableStr(totalID),
		Etag:       graphql.Value(ulid.GenerateString()),
		UpdateTime: graphql.Value(dbutil.TsToStr(timestamppb.New(now))),
	}

	// Stage 1: create the replacement rows (window + price breakdown). Additive
	// only — safe to reap if the CAS below loses.
	ins := r.svc.Mutation.Tx()
	var winRes timewindowsql.InsertSharedTimeWindowsResponse
	ins.Add(r.svc.Mutation.Shared.TimeWindows.CreateOp(window, &winRes))
	queueMoneyInserts(ins, r, priceIn, discountIn, totalIn)
	if err := ins.Commit(ctx); err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}

	// Stage 2: CAS-repoint the booking onto the new rows. A concurrent write
	// (confirm/cancel/another reschedule) loses here — before anything old is
	// deleted — and the freshly inserted rows are reaped best-effort.
	// (Reschedule carries no state gate today, matching the GORM provider, so
	// the guard is the etag alone.)
	var casErr error
	if res.Etag != nil {
		_, casErr = r.svc.Mutation.Booking.Resource.UpdateIfMatch(ctx, id, patch, resourceql.Etag.Eq(*res.Etag))
	} else {
		_, casErr = r.svc.Mutation.Booking.Resource.Update(ctx, id, patch)
	}
	if casErr != nil {
		reap := r.svc.Mutation.Tx()
		var delW timewindowsql.DeleteSharedTimeWindowsByIdResponse
		reap.Add(r.svc.Mutation.Shared.TimeWindows.DeleteOp(window.Id, &delW))
		for _, in := range []*moneysql.CreateInput{priceIn, discountIn, totalIn} {
			if in != nil {
				var delM moneysql.DeleteCommonMoneysByIdResponse
				reap.Add(r.svc.Mutation.Common.Moneys.DeleteOp(in.Id, &delM))
			}
		}
		_ = reap.Commit(ctx) // best-effort: a failed reap only leaves orphans
		return nil, dbutil.MapHasuraErr(casErr)
	}

	// Stage 3: drop the superseded window and Money rows. The booking is already
	// consistent, so a failure here must not fail the reschedule — it only
	// leaves orphaned value-object rows.
	drop := r.svc.Mutation.Tx()
	var delW timewindowsql.DeleteSharedTimeWindowsByIdResponse
	drop.Add(r.svc.Mutation.Shared.TimeWindows.DeleteOp(res.WindowId, &delW))
	for _, mid := range []*string{res.PriceId, res.DiscountId, res.TotalId} {
		if mid != nil {
			var delM moneysql.DeleteCommonMoneysByIdResponse
			drop.Add(r.svc.Mutation.Common.Moneys.DeleteOp(*mid, &delM))
		}
	}
	_ = drop.Commit(ctx)

	out, err := r.GetBooking(ctx, name)
	if err != nil {
		return nil, err
	}
	out.PriceComponents = components
	return out, nil
}
