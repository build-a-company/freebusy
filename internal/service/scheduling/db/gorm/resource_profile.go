package gorm

import "context"

// resourceProfile carries the three facts the booking path needs about a
// bookable resource that are not pricing: how many of it there are, how many
// people fit in one, and which timezone its nights are counted in.
//
// These used to be columns on freebusy's own Unit. Unit is now an RFC 9073
// VRESOURCE owned by another service, so freebusy stores none of them:
//
//   - Capacity is a pool count. No RFC models inventory, and under the
//     per-resource-calendar model it is derived by counting the resources in a
//     pool rather than stored on one.
//   - MaxOccupancy is a vertical fact (a hotel room sleeps four; a parking bay
//     does not sleep). It belongs in RFC 9073 section 6.6 STRUCTURED-DATA on the
//     resource, which is the RFC's own extension point.
//   - TimeZone lives on the resource's RFC 5545 Calendar, as a
//     google.type.TimeZone.
//
// This struct is the seam where that lookup will land. Keeping it one type with
// one resolver means the RFC client, when it exists, is a change to a single
// function rather than to five call sites.
type resourceProfile struct {
	// Capacity is how many interchangeable units the pool holds. Never zero:
	// see resolveResourceProfile for why unknown means one.
	Capacity int64

	// MaxOccupancy is the most guests one unit takes. Zero means unbounded,
	// which is party.Fits's existing contract for an unspecified limit.
	MaxOccupancy int32

	// TimeZone is the IANA zone night boundaries are drawn in. Empty means UTC.
	TimeZone string
}

// resolveResourceProfile returns the booking-relevant facts for a resource.
//
// Until the RFC Resources/Calendars client exists it returns conservative
// defaults, and the choice of default is the point:
//
//   - Capacity 1 is fail-closed. It is also exactly what this code already did
//     when a Unit carried no capacity, so nothing is newly permitted: an unknown
//     pool is treated as a single unit and the second concurrent booking is
//     refused. Defaulting to "unlimited" would turn a missing lookup into a
//     double-booking, which is the one outcome worth being paranoid about.
//   - MaxOccupancy 0 is unbounded, matching party.Fits's documented behaviour
//     for a unit that never declared a limit. This is the one relaxation here,
//     and it is bounded: occupancy is a comfort constraint, not an inventory
//     one, so getting it wrong oversells a room's beds rather than the room.
//   - TimeZone empty resolves to UTC in nightsBetween. A wrong zone shifts a
//     night boundary by hours, never the number of resources sold.
//
// The signature already takes the context and the resource name so the RFC
// lookup drops in without touching a caller.
func resolveResourceProfile(_ context.Context, _ string) resourceProfile {
	return resourceProfile{Capacity: 1, MaxOccupancy: 0, TimeZone: ""}
}
