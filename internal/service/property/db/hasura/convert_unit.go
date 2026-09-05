// The unit write graph: proto to mutation inputs.
package hasura

import (
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/commonql/moneysql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/feesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/losdiscountsql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/rateoverridesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/taxesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/unitapplicablepromocodesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/unitmediasql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/propertyql/unitsql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/sharedql/daterangesql"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/property/v1/propertypbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type unitGraph struct {
	unit          unitsql.CreateInput
	price         *moneysql.CreateInput
	moneys        []moneysql.CreateInput
	dates         []daterangesql.CreateInput
	rateOverrides []rateoverridesql.CreateInput
	losDiscounts  []losdiscountsql.CreateInput
	fees          []feesql.CreateInput
	taxes         []taxesql.CreateInput
	medias        []unitmediasql.CreateInput
	promoCodes    []unitapplicablepromocodesql.CreateInput
}

func buildUnitGraph(u *propertypbv1.Unit, propertyID string, now time.Time) *unitGraph {
	g := &unitGraph{}
	nowStr := dbutil.TsToStr(timestamppb.New(now))
	g.unit = unitsql.CreateInput{
		DisplayName:  u.GetDisplayName(),
		Description:  u.GetDescription(),
		Type:         unitTypeToStr(u.GetType()),
		BookingMode:  bookingModeToStr(u.GetBookingMode()),
		Capacity:     u.GetCapacity(),
		MaxOccupancy: u.GetMaxOccupancy(),
		TimeZone:     u.GetTimeZone(),
		PricingUnit:  pricingUnitToStr(u.GetPricingUnit()),
		Duration:     durationToStr(u.GetDuration()),
		Tags:         strSliceToPtrs(u.GetTags()),
		Attributes:   structToJSON(u.GetAttributes()),
		State:        stateActive,
		PropertyId:   propertyID,
		CreateTime:   nowStr,
		UpdateTime:   nowStr,
	}
	if p := u.GetPrice(); p != nil {
		mID := ulid.GenerateString()
		ci := moneyInput(mID, p)
		g.price = &ci
		g.unit.PriceId = mID
	}
	for _, ro := range u.GetRateOverrides() {
		row := rateoverridesql.CreateInput{Id: ulid.GenerateString(), Weekdays: weekdaysToStr(ro.GetWeekdays())}
		if dr := ro.GetDateRange(); dr != nil {
			dID := ulid.GenerateString()
			g.dates = append(g.dates, dateRangeInput(dID, dr))
			row.DateRangeId = dID
		}
		if p := ro.GetPrice(); p != nil {
			mID := ulid.GenerateString()
			g.moneys = append(g.moneys, moneyInput(mID, p))
			row.PriceId = mID
		}
		g.rateOverrides = append(g.rateOverrides, row)
	}
	for _, ld := range u.GetLosDiscounts() {
		row := losdiscountsql.CreateInput{Id: ulid.GenerateString(), MinNights: ld.GetMinNights(), PercentOff: ld.GetPercentOff()}
		if amt := ld.GetAmountOff(); amt != nil {
			mID := ulid.GenerateString()
			g.moneys = append(g.moneys, moneyInput(mID, amt))
			row.AmountOffId = mID
		}
		g.losDiscounts = append(g.losDiscounts, row)
	}
	for _, f := range u.GetFees() {
		row := feesql.CreateInput{
			Id:          ulid.GenerateString(),
			Code:        f.GetCode(),
			DisplayName: f.GetDisplayName(),
			Percent:     f.GetPercent(),
			PricingUnit: pricingUnitToStr(f.GetPricingUnit()),
			Taxable:     f.GetTaxable(),
		}
		if amt := f.GetAmount(); amt != nil {
			mID := ulid.GenerateString()
			g.moneys = append(g.moneys, moneyInput(mID, amt))
			row.AmountId = mID
		}
		g.fees = append(g.fees, row)
	}
	for _, t := range u.GetTaxes() {
		g.taxes = append(g.taxes, taxesql.CreateInput{
			Id:          ulid.GenerateString(),
			Code:        t.GetCode(),
			DisplayName: t.GetDisplayName(),
			Percent:     dbutil.FloatToBigdecimal(t.GetPercent()),
		})
	}
	for _, m := range u.GetMedia() {
		g.medias = append(g.medias, unitmediasql.CreateInput{
			Id:          ulid.GenerateString(),
			Uri:         m.GetUri(),
			Type:        mediaTypeToStr(m.GetType()),
			Title:       m.GetTitle(),
			Description: m.GetDescription(),
			MimeType:    m.GetMimeType(),
			SortOrder:   m.GetSortOrder(),
			Primary:     m.GetPrimary(),
		})
	}
	for _, name := range u.GetApplicablePromoCodes() {
		g.promoCodes = append(g.promoCodes, unitapplicablepromocodesql.CreateInput{
			Id:          ulid.GenerateString(),
			PromoCodeId: name,
		})
	}
	return g
}
