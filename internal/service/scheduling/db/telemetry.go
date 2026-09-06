package db

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/graphqlx"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/scheduling/db/hasura"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/guest/v1/guestpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"google.golang.org/genproto/googleapis/type/money"
)

// bookingResource is the "table" attribute every wrapped operation below
// carries (see graphqlx.Wrap). It matches the GORM side's booking_store.go
// (RecordOp(ctx, "booking.resource", ...)), so freebusy_orm_store_ops_total /
// freebusy_orm_store_duration_ms group the same logical entity across
// providers.
const bookingResource = "booking.resource"

// instrumentedBookingRepository wraps a Hasura-backed BookingRepository so
// every call emits the same ops-counter + duration-histogram + trace span
// that GORM's generated stores already emit via ormtelemetry (see
// internal/database/hasura/graphqlx). GORM is not wrapped here — it is
// instrumented at the SQL layer by freebusy.Default.Instrument
// (internal/database/open.go), and wrapping it again would double-count.
type instrumentedBookingRepository struct {
	repo *hasura.BookingRepository
	t    graphqlx.Telemetry
}

// instrumentHasuraBooking wraps repo with the process-wide telemetry client.
func instrumentHasuraBooking(repo *hasura.BookingRepository) BookingRepository {
	return &instrumentedBookingRepository{repo: repo, t: graphqlx.Default()}
}

func (i *instrumentedBookingRepository) CreateBooking(ctx context.Context, b *schedulingpbv1.Booking) (out *schedulingpbv1.Booking, err error) {
	err = graphqlx.Wrap(ctx, i.t, bookingResource, "create", func(ctx context.Context) error {
		out, err = i.repo.CreateBooking(ctx, b)
		return err
	})
	return out, err
}

func (i *instrumentedBookingRepository) PreviewBooking(ctx context.Context, b *schedulingpbv1.Booking) (out *schedulingpbv1.Booking, err error) {
	err = graphqlx.Wrap(ctx, i.t, bookingResource, "preview_create", func(ctx context.Context) error {
		out, err = i.repo.PreviewBooking(ctx, b)
		return err
	})
	return out, err
}

func (i *instrumentedBookingRepository) GetBooking(ctx context.Context, name string) (out *schedulingpbv1.Booking, err error) {
	err = graphqlx.Wrap(ctx, i.t, bookingResource, "get", func(ctx context.Context) error {
		out, err = i.repo.GetBooking(ctx, name)
		return err
	})
	return out, err
}

func (i *instrumentedBookingRepository) ListBookings(ctx context.Context, in repox.ListInput) (items []*schedulingpbv1.Booking, nextPageToken string, err error) {
	err = graphqlx.Wrap(ctx, i.t, bookingResource, "list", func(ctx context.Context) error {
		items, nextPageToken, err = i.repo.ListBookings(ctx, in)
		return err
	})
	return items, nextPageToken, err
}

func (i *instrumentedBookingRepository) ConfirmBooking(ctx context.Context, name string) (out *schedulingpbv1.Booking, err error) {
	err = graphqlx.Wrap(ctx, i.t, bookingResource, "confirm", func(ctx context.Context) error {
		out, err = i.repo.ConfirmBooking(ctx, name)
		return err
	})
	return out, err
}

func (i *instrumentedBookingRepository) CancelBooking(ctx context.Context, name string, reason schedulingpbv1.CancelReason) (out *schedulingpbv1.Booking, err error) {
	err = graphqlx.Wrap(ctx, i.t, bookingResource, "cancel", func(ctx context.Context) error {
		out, err = i.repo.CancelBooking(ctx, name, reason)
		return err
	})
	return out, err
}

func (i *instrumentedBookingRepository) PreviewCancellation(ctx context.Context, name string) (refundable bool, percent int32, amount, nonRefundable *money.Money, summary string, err error) {
	err = graphqlx.Wrap(ctx, i.t, bookingResource, "preview_cancel", func(ctx context.Context) error {
		refundable, percent, amount, nonRefundable, summary, err = i.repo.PreviewCancellation(ctx, name)
		return err
	})
	return refundable, percent, amount, nonRefundable, summary, err
}

func (i *instrumentedBookingRepository) RescheduleBooking(ctx context.Context, name string, b *schedulingpbv1.Booking, newUnit string) (out *schedulingpbv1.Booking, err error) {
	err = graphqlx.Wrap(ctx, i.t, bookingResource, "reschedule", func(ctx context.Context) error {
		out, err = i.repo.RescheduleBooking(ctx, name, b, newUnit)
		return err
	})
	return out, err
}

func (i *instrumentedBookingRepository) ExpireHolds(ctx context.Context) (n int64, err error) {
	err = graphqlx.Wrap(ctx, i.t, bookingResource, "expire_holds", func(ctx context.Context) error {
		n, err = i.repo.ExpireHolds(ctx)
		return err
	})
	return n, err
}

func (i *instrumentedBookingRepository) UpdateBookingGuests(ctx context.Context, name string, guests []*guestpbv1.Guest, occupancy *schedulingpbv1.Occupancy) (out *schedulingpbv1.Booking, err error) {
	err = graphqlx.Wrap(ctx, i.t, bookingResource, "update_guests", func(ctx context.Context) error {
		out, err = i.repo.UpdateBookingGuests(ctx, name, guests, occupancy)
		return err
	})
	return out, err
}
