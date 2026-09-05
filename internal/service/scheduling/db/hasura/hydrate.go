// Read-side hydration: assembling a Booking proto from its rows, guests, and unit names.
package hasura

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/commonql/postaladdressql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/identityql/foreignerdetailsql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/identityql/guestpreferencesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/identityql/iddocumentsql"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/identityql/guestsql"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
)

// hydrateBooking loads a booking row's value-objects and resolves its unit name.
func (r *BookingRepository) hydrateBooking(ctx context.Context, res *resourceql.BookingResource) (*bookingpbv1.Booking, error) {
	parts := bookingParts{res: res}

	unitName, err := r.unitName(ctx, res.Unit)
	if err != nil {
		return nil, err
	}
	parts.unitName = unitName

	if res.ContactId != nil {
		c, err := r.svc.Query.Shared.Contacts.Get(ctx, *res.ContactId)
		if err != nil {
			return nil, dbutil.MapHasuraErr(err)
		}
		parts.contact = c
	}
	if res.WindowId != "" {
		w, err := r.svc.Query.Shared.TimeWindows.Get(ctx, res.WindowId)
		if err != nil {
			return nil, dbutil.MapHasuraErr(err)
		}
		parts.window = w
	}
	if res.PriceId != nil {
		if parts.price, err = r.money(ctx, *res.PriceId); err != nil {
			return nil, err
		}
	}
	if res.DiscountId != nil {
		if parts.discount, err = r.money(ctx, *res.DiscountId); err != nil {
			return nil, err
		}
	}
	if res.TotalId != nil {
		if parts.total, err = r.money(ctx, *res.TotalId); err != nil {
			return nil, err
		}
	}
	if res.RefundAmountId != nil {
		if parts.refund, err = r.money(ctx, *res.RefundAmountId); err != nil {
			return nil, err
		}
	}
	out := bookingFromParts(parts)

	if res.OccupancyId != nil {
		occ, err := r.svc.Query.Booking.Occupancies.Get(ctx, *res.OccupancyId)
		if err != nil {
			return nil, dbutil.MapHasuraErr(err)
		}
		out.Occupancy = occupancyFromSchema(occ)
	}
	guests, err := r.loadGuests(ctx, res.Id)
	if err != nil {
		return nil, err
	}
	out.Guests = guests
	return out, nil
}

// loadGuests returns a booking's guest party, each with its sub-rows hydrated,
// ordered by id (ULIDs preserve insertion order).
func (r *BookingRepository) loadGuests(ctx context.Context, bookingID string) ([]*identitypbv1.Guest, error) {
	rows, err := r.svc.Query.Identity.Guests.List(ctx, guestsql.List().Where(guestsql.BookingId.Eq(bookingID)).OrderBy(guestsql.Id.Asc()))
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	out := make([]*identitypbv1.Guest, 0, len(rows))
	for i := range rows {
		g := &rows[i]
		var doc *iddocumentsql.IdentityIdDocuments
		var foreigner *foreignerdetailsql.IdentityForeignerDetails
		var prefs *guestpreferencesql.IdentityGuestPreferences
		var perm, loc *postaladdressql.CommonPostalAddress
		if g.IdDocumentId != nil {
			if doc, err = r.svc.Query.Identity.IdDocuments.Get(ctx, *g.IdDocumentId); err != nil {
				return nil, dbutil.MapHasuraErr(err)
			}
		}
		if g.ForeignerId != nil {
			if foreigner, err = r.svc.Query.Identity.ForeignerDetails.Get(ctx, *g.ForeignerId); err != nil {
				return nil, dbutil.MapHasuraErr(err)
			}
		}
		if g.PreferencesId != nil {
			if prefs, err = r.svc.Query.Identity.GuestPreferences.Get(ctx, *g.PreferencesId); err != nil {
				return nil, dbutil.MapHasuraErr(err)
			}
		}
		if g.PermanentAddressId != nil {
			if perm, err = r.svc.Query.Common.PostalAddress.Get(ctx, *g.PermanentAddressId); err != nil {
				return nil, dbutil.MapHasuraErr(err)
			}
		}
		if g.LocalAddressId != nil {
			if loc, err = r.svc.Query.Common.PostalAddress.Get(ctx, *g.LocalAddressId); err != nil {
				return nil, dbutil.MapHasuraErr(err)
			}
		}
		out = append(out, guestFromSchema(g, doc, foreigner, prefs, perm, loc))
	}
	return out, nil
}

// unitName resolves a bare unit id to its full resource name (the booking row
// stores only the id, since its FK targets property.units.id).
func (r *BookingRepository) unitName(ctx context.Context, unitID string) (string, error) {
	if unitID == "" {
		return "", nil
	}
	u, err := r.svc.Query.Property.Units.Get(ctx, unitID)
	if err != nil {
		return "", dbutil.MapHasuraErr(err)
	}
	if u == nil {
		return "", nil
	}
	return u.Name, nil
}
