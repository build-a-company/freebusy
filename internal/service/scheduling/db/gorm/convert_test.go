package gorm

import (
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"testing"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/booking"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/shared"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNightsBetweenTimezone(t *testing.T) {
	// Dec 24 14:00 IST (08:30 UTC) -> Dec 27 11:00 IST (05:30 UTC) = 3 nights.
	w := &sharedpbv1.TimeWindow{
		StartTime: timestamppb.New(time.Date(2026, 12, 24, 8, 30, 0, 0, time.UTC)),
		EndTime:   timestamppb.New(time.Date(2026, 12, 27, 5, 30, 0, 0, time.UTC)),
	}
	if n := nightsBetween(w, "Asia/Kolkata"); n != 3 {
		t.Fatalf("nights = %d, want 3", n)
	}
	// A sub-day window still counts as one night.
	same := &sharedpbv1.TimeWindow{
		StartTime: timestamppb.New(time.Date(2026, 12, 24, 8, 0, 0, 0, time.UTC)),
		EndTime:   timestamppb.New(time.Date(2026, 12, 24, 10, 0, 0, 0, time.UTC)),
	}
	if n := nightsBetween(same, "Asia/Kolkata"); n != 1 {
		t.Fatalf("nights = %d, want 1", n)
	}
}

func TestMoneyMath(t *testing.T) {
	base := &money.Money{CurrencyCode: "INR", Units: 5000, Nanos: 500_000_000} // 5000.50
	if got := moneyMul(base, 3); got.GetUnits() != 15001 || got.GetNanos() != 500_000_000 {
		t.Fatalf("moneyMul = %d.%09d, want 15001.5", got.GetUnits(), got.GetNanos())
	}
	total := &money.Money{CurrencyCode: "INR", Units: 8000}
	if got := moneyPct(total, 50); got.GetUnits() != 4000 {
		t.Fatalf("moneyPct(50%%) = %d, want 4000", got.GetUnits())
	}
	if got := moneySub(total, moneyPct(total, 50)); got.GetUnits() != 4000 {
		t.Fatalf("moneySub = %d, want 4000", got.GetUnits())
	}
}

func TestBookingFromModel(t *testing.T) {
	state := booking.BookingStatePendingHold
	m := &booking.Booking{
		ID:     "b1",
		Name:   "bookings/b1",
		UnitID: "u1",
		Units:  repox.Ptr(int32(2)),
		State:  &state,
		Window: &shared.TimeWindow{
			StartTime: time.Date(2026, 12, 24, 8, 30, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 27, 5, 30, 0, 0, time.UTC),
		},
		Price: &common.Money{CurrencyCode: repox.Ptr("INR"), Units: repox.Ptr(int64(24000))},
		Total: &common.Money{CurrencyCode: repox.Ptr("INR"), Units: repox.Ptr(int64(24000))},
		Contact: &shared.Contact{
			DisplayName: repox.Ptr("Asha"),
			Email:       repox.Ptr("asha@example.com"),
		},
	}
	out := bookingFromModel(m, "properties/p1/units/u1")

	if out.GetUnit() != "properties/p1/units/u1" || out.GetUnits() != 2 {
		t.Fatalf("unit/units not preserved: %+v", out)
	}
	if out.GetState() != bookingpbv1.BookingState_BOOKING_STATE_PENDING_HOLD {
		t.Fatalf("state not preserved: %v", out.GetState())
	}
	if out.GetPrice().GetUnits() != 24000 || out.GetTotal().GetUnits() != 24000 {
		t.Fatalf("price/total not preserved: %+v", out.GetPrice())
	}
	if out.GetContact().GetDisplayName() != "Asha" || out.GetContact().GetEmail() != "asha@example.com" {
		t.Fatalf("contact not preserved: %+v", out.GetContact())
	}
	if out.GetWindow().GetStartTime().AsTime().Hour() != 8 {
		t.Fatalf("window not preserved: %+v", out.GetWindow())
	}
}
