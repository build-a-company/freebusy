// Package gorm provides the GORM-backed implementation of the booking
// persistence contract (internal/service/booking/db.BookingRepository). It owns
// the hold lifecycle, the capacity/overlap check that prevents overbooking, and a
// base-price computation from the unit's price (evaluated in the unit timezone
// for nightly stays).
package gorm

import (
	"context"
	"errors"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/filterx"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/booking"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/property"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
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
func (r *BookingRepository) GetBooking(ctx context.Context, name string) (*bookingpbv1.Booking, error) {
	id, err := types.BookingID(name)
	if err != nil {
		return nil, err
	}
	var m booking.Booking
	if err := preloadBooking(r.db.WithContext(ctx)).First(&m, "id = ?", id).Error; err != nil {
		return nil, repox.MapGormErr(err)
	}
	unitName, err := r.unitName(ctx, m.UnitID)
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
func (r *BookingRepository) ListBookings(ctx context.Context, in repox.ListInput) ([]*bookingpbv1.Booking, string, error) {
	fin, err := types.FilterxFromRaw(in)
	if err != nil {
		return nil, "", err
	}
	models, next, err := filterx.Gorm[booking.Booking](booking.BookingFilterSpec).
		List(ctx, preloadBooking(r.db), fin)
	if err != nil {
		return nil, "", repox.MapGormErr(repox.MapFilterxErr(err))
	}
	unitNames, err := r.unitNames(ctx, models)
	if err != nil {
		return nil, "", err
	}
	items := make([]*bookingpbv1.Booking, 0, len(models))
	for i := range models {
		out := bookingFromModel(&models[i], unitNames[models[i].UnitID])
		guests, err := r.loadGuests(ctx, models[i].ID)
		if err != nil {
			return nil, "", err
		}
		out.Guests = guests
		items = append(items, out)
	}
	return items, next, nil
}

// unitName resolves a bare unit id to its full resource name (the booking row
// stores only the id, since its FK targets property.units.id).
func (r *BookingRepository) unitName(ctx context.Context, unitID string) (string, error) {
	var u property.Unit
	if err := r.db.WithContext(ctx).Select("id", "name").First(&u, "id = ?", unitID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", repox.MapGormErr(err)
	}
	return u.Name, nil
}

// unitNames batches the id→name resolution for a page of bookings.
func (r *BookingRepository) unitNames(ctx context.Context, bookings []booking.Booking) (map[string]string, error) {
	ids := make([]string, 0, len(bookings))
	seen := map[string]bool{}
	for i := range bookings {
		if id := bookings[i].UnitID; id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var units []property.Unit
	if err := r.db.WithContext(ctx).Select("id", "name").Where("id IN ?", ids).Find(&units).Error; err != nil {
		return nil, repox.MapGormErr(err)
	}
	for i := range units {
		out[units[i].ID] = units[i].Name
	}
	return out, nil
}
