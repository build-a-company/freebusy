package gorm

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/booking"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/identity"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"gorm.io/gorm"
)

// This file wires a booking's guest party and occupancy into storage. The
// field mapping itself is generated (identity.GuestFromProto/ToProto and
// friends in the models packages); what lives here is only what the schema
// cannot know: fresh ULIDs, the belongs-to FK wiring between a guest and its
// sub-rows, and FK-ordered persistence. Guests are stored in the
// identity.guests table (has-many by booking_id) with belongs-to sub-rows for
// the ID document, foreigner-registration details, preferences, and the
// permanent/local addresses. Occupancy is a booking-local belongs-to value.

// occupancyToModel maps the proto occupancy onto a fresh row.
func occupancyToModel(o *bookingpbv1.Occupancy) *booking.Occupancy {
	m := booking.OccupancyFromProto(o)
	if m != nil {
		m.ID = ulid.GenerateString()
	}
	return m
}

// --- guest graph -------------------------------------------------------------

// guestGraph is the set of rows one Guest materializes into: the guest row plus
// its belongs-to sub-rows (created before it, since the guest holds their FKs).
type guestGraph struct {
	guest       *identity.Guest
	idDocument  *identity.IdDocument
	foreigner   *identity.ForeignerDetails
	preferences *identity.GuestPreferences
	permanent   *common.PostalAddress
	local       *common.PostalAddress
}

// buildGuestGraph turns a proto Guest into its row graph under bookingID. The
// generated converters map the fields; this wires the graph itself — fresh
// ULIDs and the belongs-to FKs the converters deliberately leave to the caller.
func buildGuestGraph(g *identitypbv1.Guest, bookingID string) guestGraph {
	graph := guestGraph{guest: identity.GuestFromProto(g)}
	graph.guest.ID = ulid.GenerateString()
	graph.guest.BookingID = bookingID
	if d := identity.IdDocumentFromProto(g.GetIdDocument()); d != nil {
		d.ID = ulid.GenerateString()
		graph.idDocument, graph.guest.IDDocumentID = d, &d.ID
	}
	if f := identity.ForeignerDetailsFromProto(g.GetForeigner()); f != nil {
		f.ID = ulid.GenerateString()
		graph.foreigner, graph.guest.ForeignerID = f, &f.ID
	}
	if p := identity.GuestPreferencesFromProto(g.GetPreferences()); p != nil {
		p.ID = ulid.GenerateString()
		graph.preferences, graph.guest.PreferencesID = p, &p.ID
	}
	if a := common.PostalAddressFromProto(g.GetPermanentAddress()); a != nil {
		a.ID = ulid.GenerateString()
		graph.permanent, graph.guest.PermanentAddressID = a, &a.ID
	}
	if a := common.PostalAddressFromProto(g.GetLocalAddress()); a != nil {
		a.ID = ulid.GenerateString()
		graph.local, graph.guest.LocalAddressID = a, &a.ID
	}
	return graph
}

// buildGuestGraphs turns a proto guest party into its row graphs under bookingID.
func buildGuestGraphs(guests []*identitypbv1.Guest, bookingID string) []guestGraph {
	graphs := make([]guestGraph, 0, len(guests))
	for _, g := range guests {
		graphs = append(graphs, buildGuestGraph(g, bookingID))
	}
	return graphs
}

// --- persistence -------------------------------------------------------------

// persistGuests inserts each guest graph in foreign-key order: the belongs-to
// sub-rows (ID document, foreigner details, preferences, addresses) first, then
// the guest row that references them and the booking.
func persistGuests(ctx context.Context, tx *gorm.DB, graphs []guestGraph) error {
	addrs := common.NewPostalAddressStore(tx)
	for i := range graphs {
		g := &graphs[i]
		if g.idDocument != nil {
			if e := identity.NewIdDocumentStore(tx).Create(ctx, g.idDocument); e != nil {
				return e
			}
		}
		if g.foreigner != nil {
			if e := identity.NewForeignerDetailsStore(tx).Create(ctx, g.foreigner); e != nil {
				return e
			}
		}
		if g.preferences != nil {
			if e := identity.NewGuestPreferencesStore(tx).Create(ctx, g.preferences); e != nil {
				return e
			}
		}
		if g.permanent != nil {
			if e := addrs.Create(ctx, g.permanent); e != nil {
				return e
			}
		}
		if g.local != nil {
			if e := addrs.Create(ctx, g.local); e != nil {
				return e
			}
		}
		if e := identity.NewGuestStore(tx).Create(ctx, g.guest); e != nil {
			return e
		}
	}
	return nil
}

// deleteBookingGuests removes a booking's guest party and the belongs-to sub-rows
// (ID documents, foreigner details, preferences, addresses) those guests owned.
// The guest rows are deleted first (they hold the FKs), then the orphaned
// sub-rows by id.
func deleteBookingGuests(ctx context.Context, tx *gorm.DB, bookingID string) error {
	var guests []identity.Guest
	if err := tx.WithContext(ctx).Where("booking_id = ?", bookingID).Find(&guests).Error; err != nil {
		return err
	}
	if len(guests) == 0 {
		return nil
	}
	var docIDs, forIDs, prefIDs, addrIDs []string
	for i := range guests {
		g := &guests[i]
		if g.IDDocumentID != nil {
			docIDs = append(docIDs, *g.IDDocumentID)
		}
		if g.ForeignerID != nil {
			forIDs = append(forIDs, *g.ForeignerID)
		}
		if g.PreferencesID != nil {
			prefIDs = append(prefIDs, *g.PreferencesID)
		}
		if g.PermanentAddressID != nil {
			addrIDs = append(addrIDs, *g.PermanentAddressID)
		}
		if g.LocalAddressID != nil {
			addrIDs = append(addrIDs, *g.LocalAddressID)
		}
	}
	if err := tx.WithContext(ctx).Where("booking_id = ?", bookingID).Delete(&identity.Guest{}).Error; err != nil {
		return err
	}
	for _, d := range []struct {
		ids   []string
		model any
	}{
		{docIDs, &identity.IdDocument{}},
		{forIDs, &identity.ForeignerDetails{}},
		{prefIDs, &identity.GuestPreferences{}},
		{addrIDs, &common.PostalAddress{}},
	} {
		if len(d.ids) > 0 {
			if err := tx.WithContext(ctx).Where("id IN ?", d.ids).Delete(d.model).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// loadGuests returns a booking's guest party, with each guest's sub-rows
// preloaded, ordered by id (ULIDs preserve insertion order).
func (r *BookingRepository) loadGuests(ctx context.Context, bookingID string) ([]*identitypbv1.Guest, error) {
	var models []identity.Guest
	if err := r.db.WithContext(ctx).
		Preload("IDDocument").
		Preload("Foreigner").
		Preload("Preferences").
		Preload("PermanentAddress").
		Preload("LocalAddress").
		Where("booking_id = ?", bookingID).
		Order("id").
		Find(&models).Error; err != nil {
		return nil, repox.MapGormErr(err)
	}
	out := make([]*identitypbv1.Guest, 0, len(models))
	for i := range models {
		out = append(out, identity.GuestToProto(&models[i]))
	}
	return out, nil
}
