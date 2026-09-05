// The booking read-side of availability: which reservations still hold capacity.
package hasura

import (
	"context"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/schedulingql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/service/availability/engine"
	"github.com/the-protobuf-project/runtime-go/network/graphql"
)

// ActiveBookings returns the overlapping held/confirmed reservations on unitID.
func (r *AvailabilityReader) ActiveBookings(ctx context.Context, unitID string, start, end time.Time) ([]engine.Reservation, error) {
	m, err := r.activeBookings(ctx, []string{unitID}, start, end)
	if err != nil {
		return nil, err
	}
	return m[unitID], nil
}

// ActiveBookingsForUnits batches ActiveBookings across many units.
func (r *AvailabilityReader) ActiveBookingsForUnits(ctx context.Context, unitIDs []string, start, end time.Time) (map[string][]engine.Reservation, error) {
	return r.activeBookings(ctx, unitIDs, start, end)
}

func (r *AvailabilityReader) activeBookings(ctx context.Context, unitIDs []string, start, end time.Time) (map[string][]engine.Reservation, error) {
	out := map[string][]engine.Reservation{}
	if len(unitIDs) == 0 {
		return out, nil
	}
	rows, err := r.svc.Query.Booking.Resource.List(ctx, resourceql.List().Where(resourceql.And(
		resourceql.Unit.In(unitIDs...),
		resourceql.State.In("PENDING_HOLD", "CONFIRMED"),
		notLapsedHold(time.Now()),
	)))
	if err != nil {
		return nil, mapErr(err)
	}
	ids := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].WindowId != "" {
			ids = append(ids, rows[i].WindowId)
		}
	}
	windows, err := r.timeWindowsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		w, ok := windows[rows[i].WindowId]
		if !ok {
			continue
		}
		ws, we := parseTS(w.StartTime), parseTS(w.EndTime)
		if ws.Before(end) && we.After(start) {
			u := int32(1)
			if rows[i].Units != nil && *rows[i].Units > 0 {
				u = *rows[i].Units
			}
			out[rows[i].Unit] = append(out[rows[i].Unit], engine.Reservation{Start: ws, End: we, Units: u})
		}
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
