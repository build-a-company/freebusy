// CreateBooking places a hold as one atomic DDN mutation batch.
package hasura

import (
	"context"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/occupanciesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/sharedql/contactsql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/sharedql/timewindowsql"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/commonql/moneysql"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/booking/party"
	"github.com/oh-tarnished/freebusy/internal/service/booking/pricing"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/booking/v1/bookingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"github.com/the-protobuf-project/runtime-go/network/runtime"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CreateBooking places a PENDING_HOLD on a unit for a window. It loads the unit
// (for capacity, price, booking mode, timezone), computes the full price
// breakdown, verifies capacity against overlapping active bookings, and persists
// the booking with its window / contact / money value-objects in one batch.
func (r *BookingRepository) CreateBooking(ctx context.Context, b *bookingpbv1.Booking) (*bookingpbv1.Booking, error) {
	id, name, err := types.ResolveBookingName(b.GetName())
	if err != nil {
		return nil, err
	}
	_, unitID, err := types.ParseUnitParent(b.GetUnit())
	if err != nil {
		return nil, err
	}
	if b.GetWindow() == nil {
		return nil, types.ErrInvalidArgument
	}
	unit, err := r.svc.Query.Property.Units.Get(ctx, unitID)
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	if unit == nil {
		return nil, types.ErrNotFound
	}

	requested := b.GetUnits()
	if requested < 1 {
		requested = 1
	}

	// Occupancy: the staying party must fit the unit's max occupancy across the
	// reserved units. Zero max_occupancy means unbounded.
	if !party.Fits(repox.Deref(unit.MaxOccupancy), requested, b.GetOccupancy(), b.GetGuests()) {
		return nil, types.ErrInvalidArgument
	}
	occupancy := occupancyInput(b.GetOccupancy())
	guestGraphs := buildGuestGraphs(b.GetGuests(), id)

	promoID := repox.LastSegment(b.GetPromoCode())

	// Full price breakdown (base × nights, LOS + promo discounts, fees, taxes).
	in, err := r.pricingInputs(ctx, unit, promoID)
	if err != nil {
		return nil, err
	}
	in.Nights = nightsBetween(b.GetWindow(), unit.TimeZone)
	in.Units = int64(requested)

	var priceIn, discountIn, totalIn *moneysql.CreateInput
	var components []*sharedpbv1.PriceComponent
	if in.Price != nil {
		res := pricing.Compute(in, unitID)
		pi := moneyInput(res.Base)
		priceIn = &pi
		ti := moneyInput(res.Total)
		totalIn = &ti
		if !pricing.IsZero(res.Discount) {
			di := moneyInput(res.Discount)
			discountIn = &di
		}
		components = res.Components
	}

	// Capacity check against overlapping active bookings on the unit.
	reserved, err := r.reservedUnits(ctx, unitID, b.GetWindow(), "")
	if err != nil {
		return nil, err
	}
	capacity := int64(1)
	if unit.Capacity != nil && *unit.Capacity > 0 {
		capacity = int64(*unit.Capacity)
	}
	if reserved+int64(requested) > capacity {
		return nil, types.ErrCapacityExhausted
	}

	now := time.Now().UTC()
	ttl := defaultHoldTTL
	if d := b.GetHoldTtl(); d != nil && d.AsDuration() > 0 {
		ttl = d.AsDuration()
	}
	window := windowInput(b.GetWindow())
	contact := contactInput(b.GetContact())

	bi := resourceql.CreateInput{
		Id:             id,
		Name:           name,
		Unit:           unitID,
		Customer:       repox.LastSegment(b.GetCustomer()),
		Units:          requested,
		State:          "PENDING_HOLD",
		HoldExpireTime: dbutil.TsToStr(timestamppb.New(now.Add(ttl))),
		PromoCode:      promoID,
		Notes:          b.GetNotes(),
		Attributes:     structToJSON(b.GetAttributes()),
		HoldTtl:        durationToStr(b.GetHoldTtl()),
		Etag:           ulid.GenerateString(),
		WindowId:       window.Id,
		CreateTime:     dbutil.TsToStr(timestamppb.New(now)),
		UpdateTime:     dbutil.TsToStr(timestamppb.New(now)),
	}
	if contact != nil {
		bi.ContactId = contact.Id
	}
	if priceIn != nil {
		bi.PriceId = priceIn.Id
	}
	if discountIn != nil {
		bi.DiscountId = discountIn.Id
	}
	if totalIn != nil {
		bi.TotalId = totalIn.Id
	}
	if occupancy != nil {
		bi.OccupancyId = occupancy.Id
	}

	tx := r.svc.Mutation.Tx()
	var winRes timewindowsql.InsertSharedTimeWindowsResponse
	tx.Add(r.svc.Mutation.Shared.TimeWindows.CreateOp(window, &winRes))
	if contact != nil {
		var cRes contactsql.InsertSharedContactsResponse
		tx.Add(r.svc.Mutation.Shared.Contacts.CreateOp(*contact, &cRes))
	}
	queueMoneyInserts(tx, r, priceIn, discountIn, totalIn)
	// Occupancy is belongs-to (before the booking); guests are has-many (after,
	// carrying the booking_id FK).
	if occupancy != nil {
		var oRes occupanciesql.InsertBookingOccupanciesResponse
		tx.Add(r.svc.Mutation.Booking.Occupancies.CreateOp(*occupancy, &oRes))
	}
	var bRes resourceql.InsertBookingResourceResponse
	tx.Add(r.svc.Mutation.Booking.Resource.CreateOp(bi, &bRes))
	queueGuestInserts(tx, r, guestGraphs)
	if err := tx.Commit(ctx); err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}

	out, err := r.GetBooking(ctx, name)
	if err != nil {
		return nil, err
	}
	// price_components are computed, not persisted; ride them along on the response.
	out.PriceComponents = components
	return out, nil
}

// queueMoneyInserts appends inserts for the non-nil Money value-objects.
func queueMoneyInserts(tx *runtime.Tx, r *BookingRepository, moneys ...*moneysql.CreateInput) {
	for _, mi := range moneys {
		if mi == nil {
			continue
		}
		var res moneysql.InsertCommonMoneysResponse
		tx.Add(r.svc.Mutation.Common.Moneys.CreateOp(*mi, &res))
	}
}
