// Package hasura is the Hasura/GraphQL-backed AvailabilityReader: the same
// read-only queries the availability engine runs on as the GORM reader, expressed
// against the generated freebusyql client. It converts GraphQL rows into the
// provider-neutral engine value types.
package hasura

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/unitsql"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql"
	feesql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/feesql"
	losdiscountsql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/losdiscountsql"
	taxesql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/taxesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/recurringrulesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/service/availability/engine"
	"github.com/oh-tarnished/freebusy/internal/service/scheduling/pricing"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/shared/rrule"
	"google.golang.org/genproto/googleapis/type/money"
)

// AvailabilityReader is the Hasura-backed availability reader.
type AvailabilityReader struct {
	svc *freebusyql.Service
}

// NewAvailabilityReader returns a Hasura-backed AvailabilityReader bound to svc.
func NewAvailabilityReader(svc *freebusyql.Service) *AvailabilityReader {
	return &AvailabilityReader{svc: svc}
}

// GetUnit loads the unit's config, pricing children, and schedule policy.
func (r *AvailabilityReader) GetUnit(ctx context.Context, unitName string) (*engine.UnitInfo, error) {
	unitID, err := types.UnitID(unitName)
	if err != nil {
		return nil, err
	}
	u, err := r.svc.Query.Property.Units.Get(ctx, unitID)
	if err != nil {
		return nil, mapErr(err)
	}
	if u == nil {
		return nil, types.ErrNotFound
	}
	return r.buildUnitInfo(ctx, u)
}

// buildUnitInfo assembles the engine UnitInfo for one unit row.
func (r *AvailabilityReader) buildUnitInfo(ctx context.Context, u *unitsql.PropertyUnits) (*engine.UnitInfo, error) {
	info := &engine.UnitInfo{
		ID:          u.Id,
		Name:        u.Name,
		DisplayName: u.DisplayName,
		Mode:        u.BookingMode,
		Capacity:    1,
		TimeZone:    u.TimeZone,
		Duration:    durationFromStr(repox.Deref(u.Duration)),
		Archived:    u.State != nil && *u.State == "ARCHIVED",
	}
	if u.Capacity != nil && *u.Capacity > 0 {
		info.Capacity = *u.Capacity
	}
	// price + pricing children
	price, fees, taxes, los, err := r.pricing(ctx, u)
	if err != nil {
		return nil, err
	}
	info.Price, info.Fees, info.Taxes, info.LosDiscounts = price, fees, taxes, los

	// schedule policy
	scheduleName, err := types.ScheduleName(u.PropertyId, u.Id)
	if err != nil {
		return nil, err
	}
	sched, err := r.svc.Query.Schedule.Resource.Find(ctx, resourceql.List().Where(resourceql.Name.Eq(scheduleName)))
	if err != nil {
		return nil, mapErr(err)
	}
	if sched != nil {
		if err := r.applySchedulePolicy(ctx, info, sched); err != nil {
			return nil, err
		}
	}
	return info, nil
}

// pricing loads a unit's base price and pricing children as engine inputs.
func (r *AvailabilityReader) pricing(ctx context.Context, u *unitsql.PropertyUnits) (*money.Money, []pricing.Fee, []pricing.Tax, []pricing.LosDiscount, error) {
	var price *money.Money
	if u.PriceId != nil {
		m, err := r.money(ctx, *u.PriceId)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		price = m
	}
	feeRows, err := r.svc.Query.Property.Fees.List(ctx, feesql.List().Where(feesql.UnitId.Eq(u.Id)))
	if err != nil {
		return nil, nil, nil, nil, mapErr(err)
	}
	fees := make([]pricing.Fee, 0, len(feeRows))
	for i := range feeRows {
		f := &feeRows[i]
		var amt *money.Money
		if f.AmountId != nil {
			if amt, err = r.money(ctx, *f.AmountId); err != nil {
				return nil, nil, nil, nil, err
			}
		}
		fees = append(fees, pricing.Fee{Code: f.Code, DisplayName: repox.Deref(f.DisplayName), PricingUnit: repox.Deref(f.PricingUnit), Percent: f.Percent, Amount: amt, Taxable: repox.Deref(f.Taxable)})
	}
	taxRows, err := r.svc.Query.Property.Taxes.List(ctx, taxesql.List().Where(taxesql.UnitId.Eq(u.Id)))
	if err != nil {
		return nil, nil, nil, nil, mapErr(err)
	}
	taxes := make([]pricing.Tax, 0, len(taxRows))
	for i := range taxRows {
		taxes = append(taxes, pricing.Tax{Code: taxRows[i].Code, DisplayName: repox.Deref(taxRows[i].DisplayName), Percent: taxRows[i].Percent})
	}
	losRows, err := r.svc.Query.Property.LosDiscounts.List(ctx, losdiscountsql.List().Where(losdiscountsql.UnitId.Eq(u.Id)))
	if err != nil {
		return nil, nil, nil, nil, mapErr(err)
	}
	los := make([]pricing.LosDiscount, 0, len(losRows))
	for i := range losRows {
		d := &losRows[i]
		var amt *money.Money
		if d.AmountOffId != nil {
			if amt, err = r.money(ctx, *d.AmountOffId); err != nil {
				return nil, nil, nil, nil, err
			}
		}
		los = append(los, pricing.LosDiscount{MinNights: d.MinNights, PercentOff: d.PercentOff, AmountOff: amt})
	}
	return price, fees, taxes, los, nil
}

// applySchedulePolicy fills the stay/notice/buffer/open-hours policy from a
// schedule row.
func (r *AvailabilityReader) applySchedulePolicy(ctx context.Context, info *engine.UnitInfo, sched *resourceql.ScheduleResource) error {
	if sched.StayConstraintsId != nil {
		sc, err := r.svc.Query.Schedule.StayConstraints.Get(ctx, *sched.StayConstraintsId)
		if err != nil {
			return mapErr(err)
		}
		if sc != nil {
			info.MinNights = derefInt32(sc.MinNights)
			info.MaxNights = derefInt32(sc.MaxNights)
			info.CheckinWeekdays = weekdaysFromStr(sc.CheckinWeekdays)
			info.CheckoutWeekdays = weekdaysFromStr(sc.CheckoutWeekdays)
		}
	}
	if sched.BuffersId != nil {
		b, err := r.svc.Query.Schedule.BufferSettings.Get(ctx, *sched.BuffersId)
		if err != nil {
			return mapErr(err)
		}
		if b != nil {
			info.MinNotice = durationFromStr(repox.Deref(b.MinNotice))
			info.MaxAdvance = durationFromStr(repox.Deref(b.MaxAdvance))
			info.Gap = durationFromStr(repox.Deref(b.Gap))
			info.StartDelta = durationFromStr(repox.Deref(b.StartDelta))
			info.EndDelta = durationFromStr(repox.Deref(b.EndDelta))
		}
	}
	rows, err := r.svc.Query.Schedule.RecurringRules.List(ctx, recurringrulesql.List().Where(recurringrulesql.ScheduleId.Eq(sched.Id)))
	if err != nil {
		return mapErr(err)
	}
	rules := make([]rrule.Rule, 0, len(rows))
	for i := range rows {
		rules = append(rules, rrule.Rule{RRule: rows[i].Rrule, Opens: repox.Deref(rows[i].Opens), Closes: repox.Deref(rows[i].Closes)})
	}
	info.Recurring = rrule.Compile(rules)
	return nil
}
