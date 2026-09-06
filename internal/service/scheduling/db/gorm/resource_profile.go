package gorm

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/rfc"
)

// resourceProfile carries the three facts the booking path needs about a
// bookable resource that are not pricing: how many of it there are, how many
// people fit in one, and which timezone its nights are counted in.
//
// These used to be columns on freebusy's own Unit. Unit is now an RFC 9073
// VRESOURCE owned by another service, so freebusy stores none of them:
//
//   - Capacity is not looked up at all, and that is not a shortcut. Under the
//     per-resource-calendar model an RFC 9073 VRESOURCE *is* one bookable thing,
//     so its capacity is one by construction. Pooling — "5 of 12 bays free" —
//     is allocation's job across many resources, not a number stored on one.
//   - MaxOccupancy is a vertical fact (a hotel room sleeps four; a parking bay
//     does not sleep). It belongs in RFC 9073 section 6.6 STRUCTURED-DATA on the
//     resource, which is the RFC's own extension point.
//   - TimeZone lives on the resource's RFC 5545 Calendar, as a
//     google.type.TimeZone.
//
// This struct is the seam that lookup lands on. Keeping it one type with one
// resolver meant wiring the RFC client touched a single function rather than
// five call sites.
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

// resolveResourceProfile returns the booking-relevant facts for a resource,
// reading the RFC catalogue when one is configured.
//
// Degrades rather than fails. A missing, unreachable or silent catalogue yields
// the defaults below, because the alternative — refusing every booking whenever
// the catalogue is down — makes freebusy's availability a function of another
// service's uptime. What each default costs if it is wrong:
//
//   - Capacity 1 is exact, not a fallback: one VRESOURCE is one bookable thing.
//   - MaxOccupancy 0 is unbounded, matching party.Fits for a resource that never
//     declared a limit. Getting it wrong oversells a room's beds, never the
//     room, so it is the one fact worth defaulting permissively.
//   - TimeZone empty resolves to UTC in nightsBetween: a wrong zone shifts a
//     night boundary by hours, never the number of resources sold.
//
// Errors are deliberately swallowed. Every one of them means "the catalogue did
// not tell us", and the defaults are what that resolves to; surfacing them would
// turn an enrichment into a hard dependency.
func (r *BookingRepository) resolveResourceProfile(ctx context.Context, resourceName string) resourceProfile {
	prof := resourceProfile{Capacity: 1}
	if r == nil || r.catalogue == nil || resourceName == "" {
		return prof
	}
	res, err := r.catalogue.Resource(ctx, resourceName)
	if err != nil {
		return prof
	}
	if p, perr := rfc.DecodeProfile(res); perr == nil {
		prof.MaxOccupancy = p.MaxOccupancy
	}
	if tz, terr := r.catalogue.TimeZone(ctx, res); terr == nil {
		prof.TimeZone = tz
	}
	return prof
}
