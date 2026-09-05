package gorm

import (
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/booking"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/shared"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
)

// This file holds what the generated converters (the models packages'
// protobuf.go) deliberately leave to the repository: resource-name↔id mapping,
// write-side model construction (which must respect OUTPUT_ONLY semantics the
// schema doesn't know), money arithmetic, and night counting. Read-side field
// mapping is the generated BookingToProto and friends.

const defaultHoldTTL = 15 * time.Minute

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func durationToStr(d *durationpb.Duration) *string {
	if d == nil {
		return nil
	}
	return repox.Ptr(d.AsDuration().String())
}

func structToJSON(s *structpb.Struct) []byte {
	if s == nil {
		return nil
	}
	b, err := s.MarshalJSON()
	if err != nil {
		return nil
	}
	return b
}

// --- value-object write-side constructors -------------------------------------

func moneyToModel(m *money.Money) *common.Money {
	if m == nil {
		return nil
	}
	return &common.Money{
		ID:           ulid.GenerateString(),
		CurrencyCode: strOrNil(m.GetCurrencyCode()),
		Units:        repox.Ptr(m.GetUnits()),
		Nanos:        repox.Ptr(m.GetNanos()),
	}
}

// moneyMul multiplies a proto Money by n, normalizing the nanos carry.
func moneyMul(m *money.Money, n int64) *money.Money {
	if m == nil {
		return nil
	}
	total := (m.GetUnits()*1_000_000_000 + int64(m.GetNanos())) * n
	return &money.Money{
		CurrencyCode: m.GetCurrencyCode(),
		Units:        total / 1_000_000_000,
		Nanos:        int32(total % 1_000_000_000),
	}
}

// moneyPct returns pct percent of m (used for refund amounts).
func moneyPct(m *money.Money, pct int32) *money.Money {
	if m == nil {
		return nil
	}
	total := (m.GetUnits()*1_000_000_000 + int64(m.GetNanos())) * int64(pct) / 100
	return &money.Money{
		CurrencyCode: m.GetCurrencyCode(),
		Units:        total / 1_000_000_000,
		Nanos:        int32(total % 1_000_000_000),
	}
}

func contactToModel(c *sharedpbv1.Contact) *shared.Contact {
	if c == nil {
		return nil
	}
	return &shared.Contact{
		ID:          ulid.GenerateString(),
		DisplayName: strOrNil(c.GetDisplayName()),
		Email:       strOrNil(c.GetEmail()),
		PhoneNumber: strOrNil(c.GetPhoneNumber()),
	}
}

func timeWindowToModel(w *sharedpbv1.TimeWindow) *shared.TimeWindow {
	if w == nil {
		return nil
	}
	return &shared.TimeWindow{
		ID:        ulid.GenerateString(),
		StartTime: w.GetStartTime().AsTime().UTC(),
		EndTime:   w.GetEndTime().AsTime().UTC(),
	}
}

// nightsBetween counts calendar nights of a window evaluated in tz (an IANA name
// on the unit). It never returns less than one night.
func nightsBetween(w *sharedpbv1.TimeWindow, tz string) int64 {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	start := w.GetStartTime().AsTime().In(loc)
	end := w.GetEndTime().AsTime().In(loc)
	sd := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	ed := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc)
	nights := int64(ed.Sub(sd).Hours() / 24)
	if nights < 1 {
		return 1
	}
	return nights
}

// --- enum + name conversions -------------------------------------------------

func cancelReasonToModel(r bookingpbv1.CancelReason) *booking.CancelReason {
	if r == bookingpbv1.CancelReason_CANCEL_REASON_UNSPECIFIED {
		return nil
	}
	v := booking.CancelReasonFromProto(r)
	if v == "" {
		return nil
	}
	return &v
}

// userNameOrEmpty rebuilds "users/{id}" from the bare customer FK id.
func userNameOrEmpty(id *string) string {
	if id == nil || *id == "" {
		return ""
	}
	return "users/" + *id
}

// promoCodeNameOrEmpty rebuilds the promo code resource name from the bare id.
func promoCodeNameOrEmpty(id *string) string {
	if id == nil || *id == "" {
		return ""
	}
	name, err := types.PromoCodeName(*id)
	if err != nil {
		return ""
	}
	return name
}

// bookingFromModel assembles the protobuf Booking from a stored row via the
// generated converter, then fills what only the repository can know: the
// resource names rebuilt from bare FK ids (the unit name is resolved by the
// caller — the row stores only the unit id).
func bookingFromModel(m *booking.Booking, unitName string) *bookingpbv1.Booking {
	out := booking.BookingToProto(m)
	out.Unit = unitName
	out.Customer = userNameOrEmpty(m.CustomerID)
	out.PromoCode = promoCodeNameOrEmpty(m.PromoCodeID)
	return out
}
