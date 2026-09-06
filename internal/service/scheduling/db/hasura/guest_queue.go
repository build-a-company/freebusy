// Mutation-batch helpers inserting and deleting guest graphs.
package hasura

import (
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/commonql/postaladdressql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/guestql/foreignerdetailsql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/guestql/iddocumentsql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/guestql/preferencesql"
	guestresourceql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/guestql/resourceql"
	"github.com/the-protobuf-project/runtime-go/network/runtime"
)

// queueGuestInserts appends each guest graph to tx in foreign-key order: the
// belongs-to sub-rows first, then the guest row that references them and the
// booking.
func queueGuestInserts(tx *runtime.Tx, r *BookingRepository, graphs []guestGraph) {
	for i := range graphs {
		g := &graphs[i]
		if g.idDoc != nil {
			var res iddocumentsql.InsertGuestIdDocumentsResponse
			tx.Add(r.svc.Mutation.Guest.IdDocuments.CreateOp(*g.idDoc, &res))
		}
		if g.foreigner != nil {
			var res foreignerdetailsql.InsertGuestForeignerDetailsResponse
			tx.Add(r.svc.Mutation.Guest.ForeignerDetails.CreateOp(*g.foreigner, &res))
		}
		if g.prefs != nil {
			var res preferencesql.InsertGuestPreferencesResponse
			tx.Add(r.svc.Mutation.Guest.Preferences.CreateOp(*g.prefs, &res))
		}
		if g.permanent != nil {
			var res postaladdressql.InsertCommonPostalAddressResponse
			tx.Add(r.svc.Mutation.Common.PostalAddress.CreateOp(*g.permanent, &res))
		}
		if g.local != nil {
			var res postaladdressql.InsertCommonPostalAddressResponse
			tx.Add(r.svc.Mutation.Common.PostalAddress.CreateOp(*g.local, &res))
		}
		var gRes guestresourceql.InsertGuestResourceResponse
		tx.Add(r.svc.Mutation.Guest.Resource.CreateOp(g.guest, &gRes))
	}
}

// queueGuestDeletes appends deletes for a booking's existing guest party: one
// predicate delete (a native mutation, delete_identity_guests_by_booking_id)
// removes every guest row on the booking — including rows a stale snapshot
// missed — then the snapshot's belongs-to sub-rows (ID documents, foreigner
// details, preferences, addresses) are deleted by id.
func queueGuestDeletes(tx *runtime.Tx, r *BookingRepository, bookingID string, guests []guestresourceql.GuestResource) {
	var delAll guestresourceql.DeleteGuestResourceByBookingIdResponse
	tx.Add(r.svc.Mutation.Guest.Resource.DeleteByBookingIdOp(bookingID, &delAll))
	for i := range guests {
		g := &guests[i]
		if g.IdDocumentId != nil {
			var res iddocumentsql.DeleteGuestIdDocumentsByIdResponse
			tx.Add(r.svc.Mutation.Guest.IdDocuments.DeleteOp(*g.IdDocumentId, &res))
		}
		if g.ForeignerId != nil {
			var res foreignerdetailsql.DeleteGuestForeignerDetailsByIdResponse
			tx.Add(r.svc.Mutation.Guest.ForeignerDetails.DeleteOp(*g.ForeignerId, &res))
		}
		if g.PreferencesId != nil {
			var res preferencesql.DeleteGuestPreferencesByIdResponse
			tx.Add(r.svc.Mutation.Guest.Preferences.DeleteOp(*g.PreferencesId, &res))
		}
		if g.PermanentAddressId != nil {
			var res postaladdressql.DeleteCommonPostalAddressByIdResponse
			tx.Add(r.svc.Mutation.Common.PostalAddress.DeleteOp(*g.PermanentAddressId, &res))
		}
		if g.LocalAddressId != nil {
			var res postaladdressql.DeleteCommonPostalAddressByIdResponse
			tx.Add(r.svc.Mutation.Common.PostalAddress.DeleteOp(*g.LocalAddressId, &res))
		}
	}
}
