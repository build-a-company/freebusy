// Scalar codecs for guest rows: addresses, dates, enums, and pointer-slice plumbing.
package hasura

import (
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"strings"
	"time"

	postaladdressql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/commonql/postaladdressql"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/guest/v1/guestpbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/genproto/googleapis/type/postaladdress"
)

func addressInput(a *postaladdress.PostalAddress) *postaladdressql.CreateInput {
	if a == nil {
		return nil
	}
	return &postaladdressql.CreateInput{
		Id:                 ulid.GenerateString(),
		Revision:           a.GetRevision(),
		RegionCode:         a.GetRegionCode(),
		LanguageCode:       a.GetLanguageCode(),
		PostalCode:         a.GetPostalCode(),
		SortingCode:        a.GetSortingCode(),
		AdministrativeArea: a.GetAdministrativeArea(),
		Locality:           a.GetLocality(),
		Sublocality:        a.GetSublocality(),
		AddressLines:       toStrPtrs(a.GetAddressLines()),
		Recipients:         toStrPtrs(a.GetRecipients()),
		Organization:       a.GetOrganization(),
	}
}

func addressFromSchema(a *postaladdressql.CommonPostalAddress) *postaladdress.PostalAddress {
	if a == nil {
		return nil
	}
	return &postaladdress.PostalAddress{
		Revision:           repox.Deref(a.Revision),
		RegionCode:         repox.Deref(a.RegionCode),
		LanguageCode:       repox.Deref(a.LanguageCode),
		PostalCode:         repox.Deref(a.PostalCode),
		SortingCode:        repox.Deref(a.SortingCode),
		AdministrativeArea: repox.Deref(a.AdministrativeArea),
		Locality:           repox.Deref(a.Locality),
		Sublocality:        repox.Deref(a.Sublocality),
		AddressLines:       fromStrPtrs(a.AddressLines),
		Recipients:         fromStrPtrs(a.Recipients),
		Organization:       repox.Deref(a.Organization),
	}
}

func dateToStr(d *date.Date) string {
	if d == nil || (d.GetYear() == 0 && d.GetMonth() == 0 && d.GetDay() == 0) {
		return ""
	}
	return time.Date(int(d.GetYear()), time.Month(d.GetMonth()), int(d.GetDay()), 0, 0, 0, 0, time.UTC).Format(dateLayout)
}

func strToDate(s string) *date.Date {
	if s == "" {
		return nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil
	}
	return &date.Date{Year: int32(t.Year()), Month: int32(t.Month()), Day: int32(t.Day())}
}

func toStrPtrs(ss []string) []*string {
	if len(ss) == 0 {
		return nil
	}
	out := make([]*string, len(ss))
	for i := range ss {
		v := ss[i]
		out[i] = &v
	}
	return out
}

func fromStrPtrs(ps []*string) []string {
	if len(ps) == 0 {
		return nil
	}
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		if p != nil {
			out = append(out, *p)
		}
	}
	return out
}

// bare strips the enum prefix from a proto enum name, returning "" for the
// unspecified value (so the column is omitted).
func bare(protoName, prefix string) string {
	if protoName == "" || strings.HasSuffix(protoName, "UNSPECIFIED") {
		return ""
	}
	return strings.TrimPrefix(protoName, prefix)
}

func genderFromStr(s *string) guestpbv1.Gender {
	if s == nil || *s == "" {
		return guestpbv1.Gender_GENDER_UNSPECIFIED
	}
	return guestpbv1.Gender(guestpbv1.Gender_value["GENDER_"+*s])
}

func ageGroupFromStr(s *string) guestpbv1.AgeGroup {
	if s == nil || *s == "" {
		return guestpbv1.AgeGroup_AGE_GROUP_UNSPECIFIED
	}
	return guestpbv1.AgeGroup(guestpbv1.AgeGroup_value["AGE_GROUP_"+*s])
}

func idDocTypeFromStr(s string) guestpbv1.IdDocumentType {
	if s == "" {
		return guestpbv1.IdDocumentType_ID_DOCUMENT_TYPE_UNSPECIFIED
	}
	return guestpbv1.IdDocumentType(guestpbv1.IdDocumentType_value["ID_DOCUMENT_TYPE_"+s])
}

func smokingFromStr(s *string) guestpbv1.SmokingPreference {
	if s == nil || *s == "" {
		return guestpbv1.SmokingPreference_SMOKING_PREFERENCE_UNSPECIFIED
	}
	return guestpbv1.SmokingPreference(guestpbv1.SmokingPreference_value["SMOKING_PREFERENCE_"+*s])
}

func bedFromStr(s *string) guestpbv1.BedPreference {
	if s == nil || *s == "" {
		return guestpbv1.BedPreference_BED_PREFERENCE_UNSPECIFIED
	}
	return guestpbv1.BedPreference(guestpbv1.BedPreference_value["BED_PREFERENCE_"+*s])
}
