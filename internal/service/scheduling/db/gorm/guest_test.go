package gorm

import (
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"testing"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/booking"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/identity"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
	"google.golang.org/genproto/googleapis/type/date"
)

func TestOccupancyRoundTrip(t *testing.T) {
	in := &bookingpbv1.Occupancy{Adults: 2, Children: 1, Infants: 0}
	out := booking.OccupancyToProto(occupancyToModel(in))
	if out.GetAdults() != 2 || out.GetChildren() != 1 || out.GetInfants() != 0 {
		t.Fatalf("occupancy round-trip = %+v", out)
	}
}

// A foreign guest with passport + Form C details + preferences survives the
// graph build and the model→proto conversion intact.
func TestGuestGraphRoundTrip(t *testing.T) {
	in := &identitypbv1.Guest{
		DisplayName: "Asha Kumar",
		Primary:     true,
		Gender:      identitypbv1.Gender_GENDER_FEMALE,
		BirthDate:   &date.Date{Year: 1990, Month: 5, Day: 12},
		AgeGroup:    identitypbv1.AgeGroup_AGE_GROUP_ADULT,
		Nationality: "GB",
		IdDocument: &identitypbv1.IdDocument{
			Type:           identitypbv1.IdDocumentType_ID_DOCUMENT_TYPE_PASSPORT,
			Number:         "P1234567",
			IssuingCountry: "GB",
			IssuePlace:     "London",
		},
		Foreigner: &identitypbv1.ForeignerDetails{
			VisaNumber:   "V999",
			VisaType:     "Tourist",
			ArrivalDate:  &date.Date{Year: 2026, Month: 12, Day: 24},
			EntryPort:    "Goa",
			VisitPurpose: "Tourism",
		},
		Preferences: &identitypbv1.GuestPreferences{
			Smoking:         identitypbv1.SmokingPreference_SMOKING_PREFERENCE_NON_SMOKING,
			Bed:             identitypbv1.BedPreference_BED_PREFERENCE_KING,
			Dietary:         []string{"vegetarian"},
			SpecialRequests: []string{"late check-in"},
		},
	}

	g := buildGuestGraph(in, "booking-1")
	if g.guest.BookingID != "booking-1" || g.guest.DisplayName != "Asha Kumar" || !repox.Deref(g.guest.Primary) {
		t.Fatalf("guest row wrong: %+v", g.guest)
	}
	if g.idDocument == nil || g.foreigner == nil || g.preferences == nil {
		t.Fatal("sub-rows should be built")
	}
	// The guest row must reference its sub-rows.
	if g.guest.IDDocumentID == nil || g.guest.ForeignerID == nil || g.guest.PreferencesID == nil {
		t.Fatal("guest FKs not wired to sub-rows")
	}

	// Reattach preloaded associations (as GORM would on read) and convert back
	// through the generated converter.
	g.guest.IDDocument = g.idDocument
	g.guest.Foreigner = g.foreigner
	g.guest.Preferences = g.preferences
	out := identity.GuestToProto(g.guest)

	if out.GetNationality() != "GB" || out.GetGender() != identitypbv1.Gender_GENDER_FEMALE {
		t.Fatalf("guest scalars lost: %+v", out)
	}
	if out.GetIdDocument().GetType() != identitypbv1.IdDocumentType_ID_DOCUMENT_TYPE_PASSPORT || out.GetIdDocument().GetNumber() != "P1234567" {
		t.Fatalf("id document lost: %+v", out.GetIdDocument())
	}
	if out.GetForeigner().GetEntryPort() != "Goa" || out.GetForeigner().GetVisitPurpose() != "Tourism" {
		t.Fatalf("foreigner details lost: %+v", out.GetForeigner())
	}
	if out.GetPreferences().GetBed() != identitypbv1.BedPreference_BED_PREFERENCE_KING || len(out.GetPreferences().GetDietary()) != 1 {
		t.Fatalf("preferences lost: %+v", out.GetPreferences())
	}
	if out.GetBirthDate().GetYear() != 1990 {
		t.Fatalf("birth_date lost: %+v", out.GetBirthDate())
	}
}
