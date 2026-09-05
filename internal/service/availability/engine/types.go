// Package engine is the pure availability compute engine. It has no I/O: given a
// unit's config, its active reservations, and its closures, it computes the
// bookable shape (per-night counts for NIGHTLY units, discrete slots for TIME_SLOT
// units), tests exact spans, coalesces bookable ranges, and produces search
// matches with a lead price. The read side (internal/service/availability/db)
// supplies the inputs; this package owns the booking-availability rules.
package engine

import (
	"time"

	"github.com/oh-tarnished/freebusy/internal/service/scheduling/pricing"
	"github.com/oh-tarnished/freebusy/shared/rrule"
	"google.golang.org/genproto/googleapis/type/money"
)

// Booking modes (matching the proto/DB enum values).
const (
	ModeNightly  = "NIGHTLY"
	ModeTimeSlot = "TIME_SLOT"
)

// UnitInfo is everything the engine needs about a unit: its booking shape, pool
// size, timezone, price + pricing rules, slot length, and the stay/notice/buffer
// policy from its schedule.
type UnitInfo struct {
	ID          string
	Name        string
	DisplayName string
	Mode        string        // ModeNightly or ModeTimeSlot
	Capacity    int32         // pool size; treated as 1 when unset
	TimeZone    string        // IANA name
	Duration    time.Duration // default slot length (TIME_SLOT)
	Price       *money.Money
	Archived    bool

	// Pricing rules, for the search lead price (full stay total, not just base).
	Fees         []pricing.Fee
	Taxes        []pricing.Tax
	LosDiscounts []pricing.LosDiscount

	// Policy pulled from the unit's schedule (zero when unset).
	MinNights        int32
	MaxNights        int32
	MinNotice        time.Duration
	MaxAdvance       time.Duration
	Gap              time.Duration  // required gap between adjacent bookings
	StartDelta       time.Duration  // prep time reserved before each booking
	EndDelta         time.Duration  // turnover time reserved after each booking
	CheckinWeekdays  []time.Weekday // allowed check-in days (empty = any)
	CheckoutWeekdays []time.Weekday // allowed check-out days (empty = any)
	Recurring        rrule.Compiled // open-hours rules, pre-parsed (empty = always open)
}

// Reservation is one active booking's held span and unit count (UTC instants).
type Reservation struct {
	Start time.Time
	End   time.Time
	Units int32
}

// Closure is a CLOSURE exception's span as UTC instants.
type Closure struct {
	Start time.Time
	End   time.Time
}
