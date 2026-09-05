// Replacing the staying party: the UpdateBookingGuests mutation batch.
package hasura

import (
	"context"
	"fmt"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/occupanciesql"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"

	resourceql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/resourceql"
	guestsql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/identityql/guestsql"
	"github.com/oh-tarnished/freebusy/internal/service/booking/party"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"github.com/the-protobuf-project/runtime-go/network/graphql"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UpdateBookingGuests replaces the whole staying party (guests + occupancy) on a
// booking, allowed only while PENDING_HOLD or CONFIRMED and re-validated against
// the unit's max occupancy.
//
// Ordering matters: the booking row is repointed with a CAS guard (state + etag
// preCheck) BEFORE any destructive guest work. The etag bump serializes
// concurrent writers — a racing replace/confirm/cancel loses the CAS and gets
// Conflict before a single guest row is touched — which also makes the
// old-guest snapshot below race-free. A failure between the CAS and the batch
// leaves only orphaned value-object rows (never a corrupted booking); the swap
// batch itself is atomic.
func (r *BookingRepository) UpdateBookingGuests(ctx context.Context, name string, guests []*identitypbv1.Guest, occupancy *bookingpbv1.Occupancy) (*bookingpbv1.Booking, error) {
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
	if res.State == nil || (*res.State != "PENDING_HOLD" && *res.State != "CONFIRMED") {
		return nil, fmt.Errorf("%w: the party can only be edited while the booking is on hold or confirmed", types.ErrInvalidState)
	}

	// Re-validate the party against the unit's max occupancy.
	unit, err := r.svc.Query.Property.Units.Get(ctx, res.Unit)
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	if unit == nil {
		return nil, types.ErrNotFound
	}
	if !party.Fits(repox.Deref(unit.MaxOccupancy), repox.Deref(res.Units), occupancy, guests) {
		return nil, types.ErrInvalidArgument
	}

	// Insert the new occupancy first so the CAS repoint below has an FK target.
	now := time.Now().UTC()
	newOcc := occupancyInput(occupancy)
	occID := ""
	if newOcc != nil {
		if _, e := r.svc.Mutation.Booking.Occupancies.Create(ctx, *newOcc); e != nil {
			return nil, dbutil.MapHasuraErr(e)
		}
		occID = newOcc.Id
	}

	// CAS: repoint the occupancy and bump the etag only if the booking is still
	// editable and unchanged since the read above.
	match := resourceql.State.In("PENDING_HOLD", "CONFIRMED")
	if res.Etag != nil {
		match = resourceql.And(match, resourceql.Etag.Eq(*res.Etag))
	}
	patch := resourceql.UpdateInput{
		OccupancyId: dbutil.NullableStr(occID),
		Etag:        graphql.Value(ulid.GenerateString()),
		UpdateTime:  graphql.Value(dbutil.TsToStr(timestamppb.New(now))),
	}
	if _, e := r.svc.Mutation.Booking.Resource.UpdateIfMatch(ctx, id, patch, match); e != nil {
		if occID != "" {
			_, _ = r.svc.Mutation.Booking.Occupancies.Delete(ctx, occID) // reap the orphan
		}
		return nil, dbutil.MapHasuraErr(e)
	}

	// The etag is bumped — this writer owns the replace section. Swap the party
	// and drop the superseded occupancy in one atomic batch.
	oldGuests, err := r.svc.Query.Identity.Guests.List(ctx, guestsql.List().Where(guestsql.BookingId.Eq(id)))
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	tx := r.svc.Mutation.Tx()
	if res.OccupancyId != nil {
		var delOcc occupanciesql.DeleteBookingOccupanciesByIdResponse
		tx.Add(r.svc.Mutation.Booking.Occupancies.DeleteOp(*res.OccupancyId, &delOcc))
	}
	queueGuestDeletes(tx, r, id, oldGuests)
	queueGuestInserts(tx, r, buildGuestGraphs(guests, id))
	if err := tx.Commit(ctx); err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	return r.GetBooking(ctx, name)
}
