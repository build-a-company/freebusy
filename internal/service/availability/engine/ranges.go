// Contiguous bookable ranges, lead pricing, and calendar math.
package engine

import (
	"time"

	"github.com/oh-tarnished/freebusy/internal/service/scheduling/pricing"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/availability/v1/availabilitypbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NightRanges coalesces consecutive open, sufficiently-free nights into bookable
// ranges (for NIGHTLY ComputeBookableRanges).
func NightRanges(nights []*availabilitypbv1.NightAvailability, unitsReq int32, loc *time.Location) []*availabilitypbv1.BookableRange {
	if unitsReq < 1 {
		unitsReq = 1
	}
	var out []*availabilitypbv1.BookableRange
	var runStart *date.Date
	var runEnd time.Time
	flush := func(end time.Time) {
		if runStart != nil {
			out = append(out, &availabilitypbv1.BookableRange{
				Window: &sharedpbv1.TimeWindow{
					StartTime: timestamppb.New(dateToTime(runStart, loc).UTC()),
					EndTime:   timestamppb.New(end.UTC()),
				},
				Bookable: true,
			})
			runStart = nil
		}
	}
	for _, n := range nights {
		open := !n.GetClosed() && n.GetFreeUnits() >= unitsReq
		nightStart := dateToTime(n.GetNight(), loc)
		if open {
			if runStart == nil {
				runStart = n.GetNight()
			}
			runEnd = nightStart.AddDate(0, 0, 1)
		} else {
			flush(runEnd)
		}
	}
	flush(runEnd)
	return out
}

// SlotRanges coalesces consecutive bookable slots into contiguous ranges.
func SlotRanges(slots []*availabilitypbv1.Slot) []*availabilitypbv1.BookableRange {
	var out []*availabilitypbv1.BookableRange
	var start, end *timestamppb.Timestamp
	flush := func() {
		if start != nil {
			out = append(out, &availabilitypbv1.BookableRange{
				Window:   &sharedpbv1.TimeWindow{StartTime: start, EndTime: end},
				Bookable: true,
			})
			start = nil
		}
	}
	for _, s := range slots {
		if s.GetBookable() {
			if start == nil {
				start = s.GetStartTime()
			}
			end = s.GetEndTime()
		} else {
			flush()
		}
	}
	flush()
	return out
}

// LeadPrice is the full price used for search sorting/display, computed by the
// shared pricing engine: the stay total for NIGHTLY (base × nights, less LOS
// discounts, plus fees and taxes), or the single slot total for TIME_SLOT (base
// plus fees and taxes).
func LeadPrice(u *UnitInfo, nights int32) *money.Money {
	if u.Price == nil {
		return nil
	}
	n := nights
	if n < 1 {
		n = 1
	}
	return pricing.Compute(pricing.Inputs{
		Price:        u.Price,
		BookingMode:  u.Mode,
		Nights:       int64(n),
		Units:        1,
		Fees:         u.Fees,
		Taxes:        u.Taxes,
		LosDiscounts: u.LosDiscounts,
	}, u.ID).Total
}

func dateToTime(d *date.Date, loc *time.Location) time.Time {
	if d == nil {
		return time.Time{}
	}
	return time.Date(int(d.GetYear()), time.Month(d.GetMonth()), int(d.GetDay()), 0, 0, 0, 0, loc)
}

func timeToDate(t time.Time) *date.Date {
	return &date.Date{Year: int32(t.Year()), Month: int32(t.Month()), Day: int32(t.Day())}
}

func cloneMoney(m *money.Money) *money.Money {
	if m == nil {
		return nil
	}
	return &money.Money{CurrencyCode: m.GetCurrencyCode(), Units: m.GetUnits(), Nanos: m.GetNanos()}
}

// NightsBetween counts calendar nights of a date range in loc.
func NightsBetween(start, end *date.Date, loc *time.Location) int32 {
	s := dateToTime(start, loc)
	e := dateToTime(end, loc)
	n := int32(e.Sub(s).Hours() / 24)
	if n < 1 {
		return 1
	}
	return n
}
