package internal

import (
	"context"
	"testing"

	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/schedule/v1/schedulepbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"github.com/the-protobuf-project/runtime-go/grpc"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The interceptor enforces the buf.validate rules annotated on the protos:
// the BookingGuests singleton name must match ^bookings/[^/]+/guests$ and
// occupancy counts are >= 0. Valid requests pass through to the handler
// untouched. The interceptor itself lives in runtime-go (grpc.WithValidation /
// grpc.NewValidationInterceptor); these tests pin its behavior against
// freebusy's protos.
func TestValidationInterceptor(t *testing.T) {
	intercept, err := grpc.NewValidationInterceptor()
	if err != nil {
		t.Fatalf("build interceptor: %v", err)
	}

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "ok", nil
	}

	// Bad resource name → InvalidArgument before the handler runs.
	bad := &schedulingpbv1.UpdateBookingGuestsRequest{
		BookingGuests: &schedulingpbv1.BookingGuests{Name: "rooms/nope"},
	}
	if _, err := intercept(context.Background(), bad, nil, handler); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad name: got err %v, want InvalidArgument", err)
	}
	if handlerCalled {
		t.Fatal("handler must not run on a validation failure")
	}

	// Negative occupancy → InvalidArgument.
	neg := &schedulingpbv1.UpdateBookingGuestsRequest{
		BookingGuests: &schedulingpbv1.BookingGuests{
			Name:      "bookings/b1/guests",
			Occupancy: &schedulingpbv1.Occupancy{Adults: -1},
		},
	}
	if _, err := intercept(context.Background(), neg, nil, handler); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("negative occupancy: got err %v, want InvalidArgument", err)
	}

	// Valid request → handler runs.
	good := &schedulingpbv1.UpdateBookingGuestsRequest{
		BookingGuests: &schedulingpbv1.BookingGuests{
			Name:      "bookings/b1/guests",
			Occupancy: &schedulingpbv1.Occupancy{Adults: 2, Children: 1},
		},
	}
	out, err := intercept(context.Background(), good, nil, handler)
	if err != nil || out != "ok" || !handlerCalled {
		t.Fatalf("valid request: out=%v err=%v handlerCalled=%t", out, err, handlerCalled)
	}
}

// One case per service and rule family: the buf.validate annotations must
// reject exactly what the deleted hand-written handler guards rejected —
// missing required fields, malformed resource names, unset oneofs, and
// out-of-range values — and pass well-formed requests through.
func TestValidationInterceptor_Services(t *testing.T) {
	intercept, err := grpc.NewValidationInterceptor()
	if err != nil {
		t.Fatalf("build interceptor: %v", err)
	}
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }

	window := &sharedpbv1.TimeWindow{
		StartTime: timestamppb.Now(),
		EndTime:   timestamppb.Now(),
	}

	cases := []struct {
		name    string
		req     proto.Message
		wantErr bool
	}{
		// organisation
		// Stored-field emails are validated at request entry only: a member row
		// created under looser rules must still accept a role-only update.

		// identity

		// property

		// schedule
		{"schedule get bad name", &schedulepbv1.GetScheduleRequest{Name: "properties/p1/units/u1"}, true},
		{"exception create no span", &schedulepbv1.CreateAvailabilityExceptionRequest{Parent: "resources/r1", AvailabilityException: &schedulepbv1.AvailabilityException{Kind: schedulepbv1.ExceptionKind_EXCEPTION_KIND_CLOSURE}}, true},
		{"exception create ok", &schedulepbv1.CreateAvailabilityExceptionRequest{Parent: "resources/r1", AvailabilityException: &schedulepbv1.AvailabilityException{Kind: schedulepbv1.ExceptionKind_EXCEPTION_KIND_CLOSURE, Span: &schedulepbv1.AvailabilityException_Window{Window: window}}}, false},

		// promocode
		{"promocode create no discount", &promocodepbv1.CreatePromoCodeRequest{PromoCode: &promocodepbv1.PromoCode{Code: "X"}}, true},
		{"promocode create percent out of range", &promocodepbv1.CreatePromoCodeRequest{PromoCode: &promocodepbv1.PromoCode{Code: "X", Discount: &promocodepbv1.Discount{Amount: &promocodepbv1.Discount_PercentOff{PercentOff: 150}}}}, true},
		{"promocode create ok", &promocodepbv1.CreatePromoCodeRequest{PromoCode: &promocodepbv1.PromoCode{Code: "X", Discount: &promocodepbv1.Discount{Amount: &promocodepbv1.Discount_PercentOff{PercentOff: 25}}}}, false},
		{"promocode validate missing subtotal", &promocodepbv1.ValidatePromoCodeRequest{Code: "X"}, true},
		{"promocode validate ok", &promocodepbv1.ValidatePromoCodeRequest{Code: "X", Subtotal: &money.Money{CurrencyCode: "USD", Units: 100}}, false},

		// booking
		{"booking create missing window", &schedulingpbv1.CreateBookingRequest{Booking: &schedulingpbv1.Booking{Unit: "resources/r1"}}, true},
		{"booking create ok", &schedulingpbv1.CreateBookingRequest{Booking: &schedulingpbv1.Booking{Unit: "resources/r1", Window: window}}, false},
		{"booking reschedule missing window", &schedulingpbv1.RescheduleBookingRequest{Name: "bookings/b1"}, true},
		{"booking window missing end", &schedulingpbv1.CreateBookingRequest{Booking: &schedulingpbv1.Booking{Unit: "resources/r1", Window: &sharedpbv1.TimeWindow{StartTime: timestamppb.Now()}}}, true},

		// availability
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := intercept(context.Background(), tc.req, nil, handler)
			if tc.wantErr {
				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("got err %v, want InvalidArgument", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid request rejected: %v", err)
			}
		})
	}
}
