package gorm

import (
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/promocode"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/property"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/booking/pricing"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"google.golang.org/genproto/googleapis/type/money"
)

// This file adapts the GORM unit/promo models into the provider-neutral
// internal/service/booking/pricing engine, so the GORM and Hasura repositories
// share one pricing implementation.

// pricingResult mirrors pricing.Result with the field names the repository uses.
type pricingResult struct {
	base       *money.Money
	discount   *money.Money
	total      *money.Money
	components []*sharedpbv1.PriceComponent
}

// computePricing builds the price breakdown for a booking of `nights` nights and
// `units` units on `unit`, applying LOS + promo discounts, fees, then taxes.
// `unit` must have Price, LosDiscounts, Fees, and Taxes preloaded; `promo` (with
// Discount + Scope preloaded) is optional.
func computePricing(unit *property.Unit, nights, units int64, promo *promocode.PromoCode) pricingResult {
	in := pricing.Inputs{
		Price:       common.MoneyToProto(unit.Price),
		BookingMode: string(unit.BookingMode),
		Nights:      nights,
		Units:       units,
	}
	for i := range unit.LosDiscounts {
		d := &unit.LosDiscounts[i]
		in.LosDiscounts = append(in.LosDiscounts, pricing.LosDiscount{
			MinNights:  d.MinNights,
			PercentOff: d.PercentOff,
			AmountOff:  common.MoneyToProto(d.AmountOff),
		})
	}
	for i := range unit.Fees {
		f := &unit.Fees[i]
		pu := ""
		if f.PricingUnit != nil {
			pu = string(*f.PricingUnit)
		}
		in.Fees = append(in.Fees, pricing.Fee{
			Code:        f.Code,
			DisplayName: repox.Deref(f.DisplayName),
			PricingUnit: pu,
			Percent:     f.Percent,
			Amount:      common.MoneyToProto(f.Amount),
			Taxable:     repox.Deref(f.Taxable),
		})
	}
	for i := range unit.Taxes {
		t := &unit.Taxes[i]
		in.Taxes = append(in.Taxes, pricing.Tax{Code: t.Code, DisplayName: repox.Deref(t.DisplayName), Percent: t.Percent})
	}
	if promo != nil {
		p := &pricing.Promo{Code: promo.Code, DisplayName: repox.Deref(promo.DisplayName)}
		if promo.Discount != nil {
			p.PercentOff = promo.Discount.PercentOff
			p.AmountOff = common.MoneyToProto(promo.Discount.AmountOff)
		}
		if promo.Scope != nil {
			p.MinSubtotal = common.MoneyToProto(promo.Scope.MinSubtotal)
			for i := range promo.Scope.ScopeApplicableUnits {
				p.ApplicableUnitIDs = append(p.ApplicableUnitIDs, promo.Scope.ScopeApplicableUnits[i].UnitID)
			}
		}
		in.Promo = p
	}

	r := pricing.Compute(in, unit.ID)
	return pricingResult{base: r.Base, discount: r.Discount, total: r.Total, components: r.Components}
}

func isZeroMoney(m *money.Money) bool { return pricing.IsZero(m) }
