package party

import (
	"testing"

	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
)

func TestSize(t *testing.T) {
	// Explicit occupancy wins: 2 adults + 1 child = 3 (infants excluded).
	occ := &bookingpbv1.Occupancy{Adults: 2, Children: 1, Infants: 1}
	if n := Size(occ, nil); n != 3 {
		t.Fatalf("Size(occupancy) = %d, want 3", n)
	}
	// Derived from guests: 2 adults + 1 child counted, 1 infant excluded.
	guests := []*identitypbv1.Guest{
		{DisplayName: "A", AgeGroup: identitypbv1.AgeGroup_AGE_GROUP_ADULT},
		{DisplayName: "B", AgeGroup: identitypbv1.AgeGroup_AGE_GROUP_ADULT},
		{DisplayName: "C", AgeGroup: identitypbv1.AgeGroup_AGE_GROUP_CHILD},
		{DisplayName: "D", AgeGroup: identitypbv1.AgeGroup_AGE_GROUP_INFANT},
	}
	if n := Size(nil, guests); n != 3 {
		t.Fatalf("Size(guests) = %d, want 3", n)
	}
}

func TestFits(t *testing.T) {
	occ := &bookingpbv1.Occupancy{Adults: 3, Children: 1}
	for _, tc := range []struct {
		name          string
		maxOcc, units int32
		want          bool
	}{
		{"party of 4 fits 2/unit across 2 units", 2, 2, true},
		{"party of 4 overflows 2/unit on 1 unit", 2, 1, false},
		{"zero max means unbounded", 0, 1, true},
		{"units floored to 1", 2, 0, false},
		{"exact fit", 4, 1, true},
	} {
		if got := Fits(tc.maxOcc, tc.units, occ, nil); got != tc.want {
			t.Errorf("%s: Fits(%d, %d) = %v, want %v", tc.name, tc.maxOcc, tc.units, got, tc.want)
		}
	}
}
