// Booking row assembly and night math.
package hasura

import (
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/sharedql/contactsql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/sharedql/timewindowsql"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"time"

	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"google.golang.org/genproto/googleapis/type/money"
)

// bookingParts is a booking row plus its hydrated value-objects and the unit's
// full resource name.
type bookingParts struct {
	res      *resourceql.BookingResource
	unitName string
	contact  *contactsql.SharedContacts
	window   *timewindowsql.SharedTimeWindows
	price    *money.Money
	discount *money.Money
	total    *money.Money
	refund   *money.Money
}

func bookingFromParts(p bookingParts) *bookingpbv1.Booking {
	r := p.res
	return &bookingpbv1.Booking{
		Name:           r.Name,
		Unit:           p.unitName,
		Customer:       userNameOrEmpty(r.Customer),
		Contact:        contactFromSchema(p.contact),
		Units:          repox.Deref(r.Units),
		Window:         windowFromSchema(p.window),
		AssignedUnit:   repox.Deref(r.AssignedUnit),
		State:          stateFromStr(r.State),
		HoldExpireTime: strToTS(repox.Deref(r.HoldExpireTime)),
		Price:          p.price,
		PromoCode:      promoCodeNameOrEmpty(r.PromoCode),
		Discount:       p.discount,
		Total:          p.total,
		Notes:          repox.Deref(r.Notes),
		Attributes:     jsonToStruct(repox.Deref(r.Attributes)),
		CancelReason:   cancelReasonFromStr(r.CancelReason),
		CreateTime:     strToTS(r.CreateTime),
		UpdateTime:     strToTS(r.UpdateTime),
		ConfirmTime:    strToTS(repox.Deref(r.ConfirmTime)),
		CancelTime:     strToTS(repox.Deref(r.CancelTime)),
		RefundAmount:   p.refund,
		RefundPercent:  repox.Deref(r.RefundPercent),
		HoldTtl:        durationFromStr(repox.Deref(r.HoldTtl)),
		Etag:           repox.Deref(r.Etag),
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
