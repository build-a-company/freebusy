// Package party is the provider-neutral headcount arithmetic for a booking's
// staying party: how many people count against a unit's occupancy, and whether
// a party fits the capacity a booking reserves. Both persistence providers
// (gorm, hasura) share these rules so create and update paths cannot drift.
package party

import (
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
)

// Size is the headcount charged against occupancy: the adults+children of the
// explicit Occupancy when given, else the non-infant guests. Infants are not
// counted.
func Size(o *bookingpbv1.Occupancy, guests []*identitypbv1.Guest) int32 {
	if o != nil && o.GetAdults()+o.GetChildren() > 0 {
		return o.GetAdults() + o.GetChildren()
	}
	var n int32
	for _, g := range guests {
		if g.GetAgeGroup() != identitypbv1.AgeGroup_AGE_GROUP_INFANT {
			n++
		}
	}
	return n
}

// Fits reports whether the party fits maxOcc guests per unit across the number
// of units the booking reserves. A non-positive maxOcc means the unit declares
// no occupancy limit; units is floored to 1 (a booking always occupies at least
// one unit).
func Fits(maxOcc, units int32, o *bookingpbv1.Occupancy, guests []*identitypbv1.Guest) bool {
	if maxOcc <= 0 {
		return true
	}
	if units < 1 {
		units = 1
	}
	return Size(o, guests) <= maxOcc*units
}
