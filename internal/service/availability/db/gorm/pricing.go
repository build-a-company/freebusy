package gorm

import (
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"strings"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/property"
	"github.com/oh-tarnished/freebusy/internal/service/scheduling/pricing"
)

// feesOf / taxesOf / losOf convert a unit's preloaded pricing children into the
// neutral pricing-engine inputs, so the availability lead price matches what the
// booking service would actually charge.

func feesOf(u *property.Unit) []pricing.Fee {
	out := make([]pricing.Fee, 0, len(u.Fees))
	for i := range u.Fees {
		f := &u.Fees[i]
		pu := ""
		if f.PricingUnit != nil {
			pu = string(*f.PricingUnit)
		}
		out = append(out, pricing.Fee{
			Code:        f.Code,
			DisplayName: repox.Deref(f.DisplayName),
			PricingUnit: pu,
			Percent:     f.Percent,
			Amount:      moneyFromModel(f.Amount),
			Taxable:     repox.Deref(f.Taxable),
		})
	}
	return out
}

func taxesOf(u *property.Unit) []pricing.Tax {
	out := make([]pricing.Tax, 0, len(u.Taxes))
	for i := range u.Taxes {
		out = append(out, pricing.Tax{Code: u.Taxes[i].Code, DisplayName: repox.Deref(u.Taxes[i].DisplayName), Percent: u.Taxes[i].Percent})
	}
	return out
}

func losOf(u *property.Unit) []pricing.LosDiscount {
	out := make([]pricing.LosDiscount, 0, len(u.LosDiscounts))
	for i := range u.LosDiscounts {
		d := &u.LosDiscounts[i]
		out = append(out, pricing.LosDiscount{MinNights: d.MinNights, PercentOff: d.PercentOff, AmountOff: moneyFromModel(d.AmountOff)})
	}
	return out
}

// protoWeekday maps the stored proto weekday name (as persisted by the schedule
// adapter, e.g. "WEEKDAY_MONDAY") to a Go weekday.
var protoWeekday = map[string]time.Weekday{
	"WEEKDAY_SUNDAY":    time.Sunday,
	"WEEKDAY_MONDAY":    time.Monday,
	"WEEKDAY_TUESDAY":   time.Tuesday,
	"WEEKDAY_WEDNESDAY": time.Wednesday,
	"WEEKDAY_THURSDAY":  time.Thursday,
	"WEEKDAY_FRIDAY":    time.Friday,
	"WEEKDAY_SATURDAY":  time.Saturday,
}

// weekdaysFromStr parses the comma-joined weekday-name column into Go weekdays.
func weekdaysFromStr(s *string) []time.Weekday {
	if s == nil || *s == "" {
		return nil
	}
	var out []time.Weekday
	for _, name := range strings.Split(*s, ",") {
		if wd, ok := protoWeekday[strings.TrimSpace(name)]; ok {
			out = append(out, wd)
		}
	}
	return out
}
