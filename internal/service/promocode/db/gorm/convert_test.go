package gorm

import (
	"testing"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// roundTrip mirrors what the repository does around the pure converters: build
// the normalized row graph from the proto, set the identity the repository owns,
// wire the associations in memory the way a preloaded read would, then convert
// the model back to a proto.
func roundTrip(in *promocodepbv1.PromoCode) *promocodepbv1.PromoCode {
	g := buildGraph(in)
	g.promo.ID = "ID123"
	g.promo.Name = in.GetName()

	moneyByID := map[string]*common.Money{}
	for _, m := range g.moneys {
		moneyByID[m.ID] = m
	}

	g.promo.Discount = g.discount
	if g.discount.AmountOffID != nil {
		g.discount.AmountOff = moneyByID[*g.discount.AmountOffID]
	}
	g.promo.Window = g.window
	g.promo.Limits = g.limits
	if g.scope != nil {
		g.promo.Scope = g.scope
		if g.scope.MinSubtotalID != nil {
			g.scope.MinSubtotal = moneyByID[*g.scope.MinSubtotalID]
		}
	}
	return fromModel(g.promo)
}

func TestPromoConvertPercentageRoundTrip(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	in := &promocodepbv1.PromoCode{
		Name:        "promoCodes/ID123",
		Code:        "SUMMER25",
		DisplayName: "Summer Sale",
		Description: "25% off everything",
		Discount:    &promocodepbv1.Discount{Amount: &promocodepbv1.Discount_PercentOff{PercentOff: 25}},
		Window:      &promocodepbv1.RedemptionWindow{StartTime: timestamppb.New(start)},
		Limits: &promocodepbv1.UsageLimits{
			MaxRedemptions:   wrapperspb.Int64(100),
			PerCustomerLimit: wrapperspb.Int32(2),
		},
		Scope: &promocodepbv1.Scope{
			MinSubtotal:          &money.Money{CurrencyCode: "USD", Units: 50},
			ApplicableProperties: []string{"properties/room-1"},
			ApplicableUnits:      []string{"properties/room-1/units/night"},
		},
	}

	out := roundTrip(in)

	if out.GetCode() != "SUMMER25" || out.GetDisplayName() != "Summer Sale" || out.GetDescription() != "25% off everything" {
		t.Fatalf("scalar fields not preserved: %+v", out)
	}
	if out.GetDiscount().GetAmountOff() != nil || out.GetDiscount().GetPercentOff() != 25 {
		t.Fatalf("discount not preserved as 25%% percentage: %+v", out.GetDiscount())
	}
	if out.GetLimits().GetMaxRedemptions().GetValue() != 100 || out.GetLimits().GetPerCustomerLimit().GetValue() != 2 {
		t.Fatalf("usage limits not preserved: %+v", out.GetLimits())
	}
	if out.GetScope().GetMinSubtotal().GetUnits() != 50 || out.GetScope().GetMinSubtotal().GetCurrencyCode() != "USD" {
		t.Fatalf("min subtotal not preserved: %+v", out.GetScope().GetMinSubtotal())
	}
	if got := out.GetScope().GetApplicableProperties(); len(got) != 1 || got[0] != "properties/room-1" {
		t.Fatalf("applicable properties = %v", got)
	}
	if got := out.GetScope().GetApplicableUnits(); len(got) != 1 || got[0] != "properties/room-1/units/night" {
		t.Fatalf("applicable units = %v", got)
	}
	if !out.GetWindow().GetStartTime().AsTime().Equal(start) {
		t.Fatalf("window start = %v, want %v", out.GetWindow().GetStartTime().AsTime(), start)
	}
	if out.GetState() != promocodepbv1.PromoCodeState_PROMO_CODE_STATE_ACTIVE {
		t.Fatalf("state = %v, want ACTIVE", out.GetState())
	}
	if out.GetName() != "promoCodes/ID123" {
		t.Fatalf("name = %q", out.GetName())
	}
}

func TestPromoConvertFixedAmountDisabled(t *testing.T) {
	in := &promocodepbv1.PromoCode{
		Code:     "FLAT10",
		Discount: &promocodepbv1.Discount{Amount: &promocodepbv1.Discount_AmountOff{AmountOff: &money.Money{CurrencyCode: "EUR", Units: 10, Nanos: 990000000}}},
		Disabled: true,
	}

	out := roundTrip(in)

	amt := out.GetDiscount().GetAmountOff()
	if amt == nil {
		t.Fatalf("expected fixed-amount discount, got %+v", out.GetDiscount())
	}
	if amt.GetUnits() != 10 || amt.GetNanos() != 990000000 || amt.GetCurrencyCode() != "EUR" {
		t.Fatalf("amount off not preserved: %+v", amt)
	}
	if !out.GetDisabled() {
		t.Fatal("disabled flag not preserved")
	}
	if out.GetState() != promocodepbv1.PromoCodeState_PROMO_CODE_STATE_DISABLED {
		t.Fatalf("state = %v, want DISABLED", out.GetState())
	}
	// No scope was set, so there should be no min subtotal.
	if out.GetScope().GetMinSubtotal() != nil {
		t.Fatalf("expected nil min subtotal, got %+v", out.GetScope().GetMinSubtotal())
	}
}
