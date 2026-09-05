package hasura

import (
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"

	occupanciesql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/occupanciesql"
	postaladdressql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/commonql/postaladdressql"
	foreignerdetailsql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/identityql/foreignerdetailsql"
	guestpreferencesql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/identityql/guestpreferencesql"
	guestsql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/identityql/guestsql"
	iddocumentsql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/identityql/iddocumentsql"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
)

const dateLayout = "2006-01-02"

// --- occupancy ---------------------------------------------------------------

func occupancyInput(o *bookingpbv1.Occupancy) *occupanciesql.CreateInput {
	if o == nil {
		return nil
	}
	return &occupanciesql.CreateInput{
		Id:       ulid.GenerateString(),
		Adults:   o.GetAdults(),
		Children: o.GetChildren(),
		Infants:  o.GetInfants(),
	}
}

func occupancyFromSchema(o *occupanciesql.BookingOccupancies) *bookingpbv1.Occupancy {
	if o == nil {
		return nil
	}
	return &bookingpbv1.Occupancy{Adults: repox.Deref(o.Adults), Children: repox.Deref(o.Children), Infants: repox.Deref(o.Infants)}
}

// --- guest graph -------------------------------------------------------------

// guestGraph is the set of GraphQL insert inputs one Guest materializes into.
type guestGraph struct {
	guest     guestsql.CreateInput
	idDoc     *iddocumentsql.CreateInput
	foreigner *foreignerdetailsql.CreateInput
	prefs     *guestpreferencesql.CreateInput
	permanent *postaladdressql.CreateInput
	local     *postaladdressql.CreateInput
}

func buildGuestGraph(g *identitypbv1.Guest, bookingID string) guestGraph {
	graph := guestGraph{
		guest: guestsql.CreateInput{
			Id:          ulid.GenerateString(),
			BookingId:   bookingID,
			DisplayName: g.GetDisplayName(),
			Primary:     g.GetPrimary(),
			Gender:      bare(g.GetGender().String(), "GENDER_"),
			BirthDate:   dateToStr(g.GetBirthDate()),
			AgeGroup:    bare(g.GetAgeGroup().String(), "AGE_GROUP_"),
			Nationality: g.GetNationality(),
			Email:       g.GetEmail(),
			PhoneNumber: g.GetPhoneNumber(),
		},
	}
	if d := g.GetIdDocument(); d != nil {
		id := ulid.GenerateString()
		graph.idDoc = &iddocumentsql.CreateInput{
			Id:             id,
			Type:           bare(d.GetType().String(), "ID_DOCUMENT_TYPE_"),
			Number:         d.GetNumber(),
			IssuingCountry: d.GetIssuingCountry(),
			IssuePlace:     d.GetIssuePlace(),
			IssueDate:      dateToStr(d.GetIssueDate()),
			ExpiryDate:     dateToStr(d.GetExpiryDate()),
		}
		graph.guest.IdDocumentId = id
	}
	if f := g.GetForeigner(); f != nil {
		id := ulid.GenerateString()
		graph.foreigner = &foreignerdetailsql.CreateInput{
			Id:              id,
			VisaNumber:      f.GetVisaNumber(),
			VisaType:        f.GetVisaType(),
			VisaIssuePlace:  f.GetVisaIssuePlace(),
			VisaIssueDate:   dateToStr(f.GetVisaIssueDate()),
			VisaExpiryDate:  dateToStr(f.GetVisaExpiryDate()),
			ArrivalDate:     dateToStr(f.GetArrivalDate()),
			EntryPort:       f.GetEntryPort(),
			Origin:          f.GetOrigin(),
			NextDestination: f.GetNextDestination(),
			VisitPurpose:    f.GetVisitPurpose(),
		}
		graph.guest.ForeignerId = id
	}
	if p := g.GetPreferences(); p != nil {
		id := ulid.GenerateString()
		graph.prefs = &guestpreferencesql.CreateInput{
			Id:              id,
			Smoking:         bare(p.GetSmoking().String(), "SMOKING_PREFERENCE_"),
			Bed:             bare(p.GetBed().String(), "BED_PREFERENCE_"),
			Dietary:         toStrPtrs(p.GetDietary()),
			Accessibility:   toStrPtrs(p.GetAccessibility()),
			FloorPreference: p.GetFloorPreference(),
			LoyaltyNumber:   p.GetLoyaltyNumber(),
			SpecialRequests: toStrPtrs(p.GetSpecialRequests()),
			Notes:           p.GetNotes(),
		}
		graph.guest.PreferencesId = id
	}
	if a := addressInput(g.GetPermanentAddress()); a != nil {
		graph.permanent = a
		graph.guest.PermanentAddressId = a.Id
	}
	if a := addressInput(g.GetLocalAddress()); a != nil {
		graph.local = a
		graph.guest.LocalAddressId = a.Id
	}
	return graph
}

// buildGuestGraphs turns a proto guest party into its insert graphs under bookingID.
func buildGuestGraphs(guests []*identitypbv1.Guest, bookingID string) []guestGraph {
	graphs := make([]guestGraph, 0, len(guests))
	for _, g := range guests {
		graphs = append(graphs, buildGuestGraph(g, bookingID))
	}
	return graphs
}

// guestFromSchema hydrates a protobuf Guest from its stored rows.
func guestFromSchema(g *guestsql.IdentityGuests, doc *iddocumentsql.IdentityIdDocuments, f *foreignerdetailsql.IdentityForeignerDetails, p *guestpreferencesql.IdentityGuestPreferences, perm, loc *postaladdressql.CommonPostalAddress) *identitypbv1.Guest {
	out := &identitypbv1.Guest{
		DisplayName:      g.DisplayName,
		Primary:          repox.Deref(g.Primary),
		Gender:           genderFromStr(g.Gender),
		BirthDate:        strToDate(repox.Deref(g.BirthDate)),
		AgeGroup:         ageGroupFromStr(g.AgeGroup),
		Nationality:      repox.Deref(g.Nationality),
		Email:            repox.Deref(g.Email),
		PhoneNumber:      repox.Deref(g.PhoneNumber),
		PermanentAddress: addressFromSchema(perm),
		LocalAddress:     addressFromSchema(loc),
	}
	if doc != nil {
		out.IdDocument = &identitypbv1.IdDocument{
			Type:           idDocTypeFromStr(doc.Type),
			Number:         doc.Number,
			IssuingCountry: repox.Deref(doc.IssuingCountry),
			IssuePlace:     repox.Deref(doc.IssuePlace),
			IssueDate:      strToDate(repox.Deref(doc.IssueDate)),
			ExpiryDate:     strToDate(repox.Deref(doc.ExpiryDate)),
		}
	}
	if f != nil {
		out.Foreigner = &identitypbv1.ForeignerDetails{
			VisaNumber:      repox.Deref(f.VisaNumber),
			VisaType:        repox.Deref(f.VisaType),
			VisaIssuePlace:  repox.Deref(f.VisaIssuePlace),
			VisaIssueDate:   strToDate(repox.Deref(f.VisaIssueDate)),
			VisaExpiryDate:  strToDate(repox.Deref(f.VisaExpiryDate)),
			ArrivalDate:     strToDate(repox.Deref(f.ArrivalDate)),
			EntryPort:       repox.Deref(f.EntryPort),
			Origin:          repox.Deref(f.Origin),
			NextDestination: repox.Deref(f.NextDestination),
			VisitPurpose:    repox.Deref(f.VisitPurpose),
		}
	}
	if p != nil {
		out.Preferences = &identitypbv1.GuestPreferences{
			Smoking:         smokingFromStr(p.Smoking),
			Bed:             bedFromStr(p.Bed),
			Dietary:         fromStrPtrs(p.Dietary),
			Accessibility:   fromStrPtrs(p.Accessibility),
			FloorPreference: repox.Deref(p.FloorPreference),
			LoyaltyNumber:   repox.Deref(p.LoyaltyNumber),
			SpecialRequests: fromStrPtrs(p.SpecialRequests),
			Notes:           repox.Deref(p.Notes),
		}
	}
	return out
}

// --- postal address ----------------------------------------------------------

// --- date / list / enum helpers ----------------------------------------------

// --- persistence -------------------------------------------------------------
