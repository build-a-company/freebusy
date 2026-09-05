package hasura

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/filterx"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/scheduling"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
)

const defaultHoldTTL = 15 * time.Minute

// BookingRepository is the Hasura-backed booking repository.
type BookingRepository struct {
	svc *freebusyql.Service
}

// NewBookingRepository returns a Hasura-backed BookingRepository bound to svc.
func NewBookingRepository(svc *freebusyql.Service) *BookingRepository {
	return &BookingRepository{svc: svc}
}

// GetBooking returns the booking addressed by its resource name.
func (r *BookingRepository) GetBooking(ctx context.Context, name string) (*schedulingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	res, err := r.svc.Query.Scheduling.Bookings.Get(ctx, id)
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	if res == nil {
		return nil, types.ErrNotFound
	}
	return r.hydrateBooking(ctx, res)
}

// ListBookings returns a page of bookings ordered by params.OrderBy.
func (r *BookingRepository) ListBookings(ctx context.Context, in repox.ListInput) ([]*schedulingpbv1.Booking, string, error) {
	fin, err := types.FilterxFromRaw(in)
	if err != nil {
		return nil, "", err
	}
	rows, next, err := filterx.Hasura(scheduling.BookingFilterSpec, r.svc.Query.Scheduling.Bookings).
		List(ctx, fin)
	if err != nil {
		return nil, "", dbutil.MapHasuraErr(repox.MapFilterxErr(err))
	}
	items := make([]*schedulingpbv1.Booking, 0, len(rows))
	for i := range rows {
		out, err := r.hydrateBooking(ctx, &rows[i])
		if err != nil {
			return nil, "", err
		}
		items = append(items, out)
	}
	return items, next, nil
}
