// Package gorm provides the GORM-backed implementation of the booking
// persistence contract (internal/service/booking/db.BookingRepository). It owns
// the hold lifecycle, the capacity/overlap check that prevents overbooking, and a
// base-price computation from the unit's price (evaluated in the unit timezone
// for nightly stays).
package gorm

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/rfc"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/filterx"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/scheduling"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"gorm.io/gorm"
)

// overlapSQL sums the reserved units of active bookings (held or confirmed) whose
// window overlaps [start,end) on a unit, for the capacity check. Windows are
// compared as UTC instants, so the check is timezone-safe. A PENDING_HOLD only
// counts while its hold is unexpired: a lapsed hold frees capacity immediately,
// without waiting for the sweeper to flip its stored state.
const overlapSQL = `
SELECT COALESCE(SUM(COALESCE(b.units, 1)), 0)
FROM "booking"."resource" b
JOIN "shared"."time_windows" w ON w.id = b.window_id
WHERE b.unit = ? AND b.state IN ('PENDING_HOLD','CONFIRMED')
  AND (b.state <> 'PENDING_HOLD' OR b.hold_expire_time IS NULL OR b.hold_expire_time > now())
  AND w.start_time < ? AND w.end_time > ?`

// BookingRepository is the GORM-backed booking repository.
type BookingRepository struct {
	db *gorm.DB
	// catalogue reads resource facts from the RFC services. Nil is valid and
	// means "no catalogue configured", which resolveResourceProfile handles by
	// falling back to its defaults rather than failing the booking.
	catalogue *rfc.Client
}

// NewBookingRepository returns a GORM-backed BookingRepository bound to db.
func NewBookingRepository(db *gorm.DB) *BookingRepository {
	return &BookingRepository{db: db}
}

func preloadBooking(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Contact").
		Preload("Window").
		Preload("Price").
		Preload("Discount").
		Preload("Total").
		Preload("RefundAmount").
		Preload("Occupancy")
}

// GetBooking returns the booking addressed by its resource name.
func (r *BookingRepository) GetBooking(ctx context.Context, name string) (*schedulingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	var m scheduling.Booking
	if err := preloadBooking(r.db.WithContext(ctx)).First(&m, "id = ?", id).Error; err != nil {
		return nil, repox.MapGormErr(err)
	}
	unitName := m.Unit
	if err != nil {
		return nil, err
	}
	out := bookingFromModel(&m, unitName)
	guests, err := r.loadGuests(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	out.Guests = guests
	return out, nil
}

// ListBookings returns a page of bookings ordered by params.OrderBy.
func (r *BookingRepository) ListBookings(ctx context.Context, in repox.ListInput) ([]*schedulingpbv1.Booking, string, error) {
	fin, err := types.FilterxFromRaw(in)
	if err != nil {
		return nil, "", err
	}
	models, next, err := filterx.Gorm[scheduling.Booking](scheduling.BookingFilterSpec).
		List(ctx, preloadBooking(r.db), fin)
	if err != nil {
		return nil, "", repox.MapGormErr(repox.MapFilterxErr(err))
	}
	if err != nil {
		return nil, "", err
	}
	items := make([]*schedulingpbv1.Booking, 0, len(models))
	for i := range models {
		out := bookingFromModel(&models[i], models[i].Unit)
		guests, err := r.loadGuests(ctx, models[i].ID)
		if err != nil {
			return nil, "", err
		}
		out.Guests = guests
		items = append(items, out)
	}
	return items, next, nil
}

// WithCatalogue attaches an RFC catalogue client, returning the repository for
// chaining. Mirrors the generated stores' WithTelemetry: optional collaborator,
// set once at construction, nil-safe if never called.
func (r *BookingRepository) WithCatalogue(c *rfc.Client) *BookingRepository {
	r.catalogue = c
	return r
}
