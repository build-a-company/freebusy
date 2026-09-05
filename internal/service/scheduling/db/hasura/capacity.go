// The capacity check backing create/reschedule: how many units active bookings
// still reserve on a unit over a window.
package hasura

import (
	"context"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/sharedql/timewindowsql"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"github.com/the-protobuf-project/runtime-go/network/graphql"
)

// reservedUnits sums the units of active bookings (held or confirmed) on unitID
// whose window overlaps target, excluding excludeID (empty to exclude none).
// Windows are compared as UTC instants, so the check is timezone-safe.
func (r *BookingRepository) reservedUnits(ctx context.Context, unitID string, target *sharedpbv1.TimeWindow, excludeID string) (int64, error) {
	preds := []graphql.Predicate{
		resourceql.Unit.Eq(unitID),
		resourceql.State.In("PENDING_HOLD", "CONFIRMED"),
		notLapsedHold(time.Now()),
	}
	if excludeID != "" {
		preds = append(preds, resourceql.Id.Neq(excludeID))
	}
	rows, err := r.svc.Query.Booking.Resource.List(ctx, resourceql.List().Where(resourceql.And(preds...)))
	if err != nil {
		return 0, dbutil.MapHasuraErr(err)
	}
	windows, err := r.windowsByID(ctx, rows)
	if err != nil {
		return 0, err
	}
	var sum int64
	for i := range rows {
		w, ok := windows[rows[i].WindowId]
		if !ok || !overlaps(&w, target) {
			continue
		}
		u := int64(1)
		if rows[i].Units != nil && *rows[i].Units > 0 {
			u = int64(*rows[i].Units)
		}
		sum += u
	}
	return sum, nil
}

// windowsByID fetches the bookings' time windows in one query, keyed by id.
func (r *BookingRepository) windowsByID(ctx context.Context, rows []resourceql.BookingResource) (map[string]timewindowsql.SharedTimeWindows, error) {
	ids := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].WindowId != "" {
			ids = append(ids, rows[i].WindowId)
		}
	}
	out := make(map[string]timewindowsql.SharedTimeWindows, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	ws, err := r.svc.Query.Shared.TimeWindows.List(ctx, timewindowsql.List().Where(timewindowsql.Id.In(ids...)))
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	for i := range ws {
		out[ws[i].Id] = ws[i]
	}
	return out, nil
}

// notLapsedHold matches bookings that still reserve capacity: any non-hold
// state, or a PENDING_HOLD whose hold has not yet lapsed. A lapsed hold frees
// capacity immediately, without waiting for the sweeper to flip its state.
func notLapsedHold(now time.Time) graphql.Predicate {
	return resourceql.Or(
		resourceql.State.Neq("PENDING_HOLD"),
		resourceql.HoldExpireTime.IsNull(true),
		resourceql.HoldExpireTime.Gt(now.UTC().Format(time.RFC3339)),
	)
}

// overlaps reports whether stored window w overlaps target [start,end) as UTC
// instants (half-open: touching endpoints do not overlap).
func overlaps(w *timewindowsql.SharedTimeWindows, target *sharedpbv1.TimeWindow) bool {
	ws, we := strToTS(w.StartTime), strToTS(w.EndTime)
	if ws == nil || we == nil || target == nil {
		return false
	}
	return ws.AsTime().Before(target.GetEndTime().AsTime()) && we.AsTime().After(target.GetStartTime().AsTime())
}
