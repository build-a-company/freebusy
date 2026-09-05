package internal

import (
	"context"
	"testing"

	"github.com/oh-tarnished/freebusy/protobuf/generated/go/availability/v1/availabilitypbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/organisation/v1/orgpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/property/v1/propertypbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/schedule/v1/schedulepbv1"
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
		{"org get bad name", &orgpbv1.GetOrganisationRequest{Name: "orgs/x"}, true},
		{"org get ok", &orgpbv1.GetOrganisationRequest{Name: "organisations/o1"}, false},
		{"org create missing organisation", &orgpbv1.CreateOrganisationRequest{}, true},
		{"org create missing display_name", &orgpbv1.CreateOrganisationRequest{Organisation: &orgpbv1.Organisation{}}, true},
		{"org create ok", &orgpbv1.CreateOrganisationRequest{Organisation: &orgpbv1.Organisation{DisplayName: "Acme"}}, false},
		{"org update missing name", &orgpbv1.UpdateOrganisationRequest{Organisation: &orgpbv1.Organisation{DisplayName: "Acme"}}, true},
		{"org invite bad email", &orgpbv1.InviteMemberRequest{Parent: "organisations/o1", Email: "nope", Role: orgpbv1.OrganisationRole_ORGANISATION_ROLE_ADMIN}, true},
		// Stored-field emails are validated at request entry only: a member row
		// created under looser rules must still accept a role-only update.
		{"member update with legacy email ok", &orgpbv1.UpdateMemberRequest{Member: &orgpbv1.Member{Name: "organisations/o1/members/m1", Email: "ops@intranet", Role: orgpbv1.OrganisationRole_ORGANISATION_ROLE_ADMIN}}, false},
		{"org invite unset role", &orgpbv1.InviteMemberRequest{Parent: "organisations/o1", Email: "a@b.co"}, true},
		{"org invite ok", &orgpbv1.InviteMemberRequest{Parent: "organisations/o1", Email: "a@b.co", Role: orgpbv1.OrganisationRole_ORGANISATION_ROLE_ADMIN}, false},
		{"org list page_size too big", &orgpbv1.ListOrganisationsRequest{PageSize: 5000}, true},

		// identity
		{"user get empty name", &identitypbv1.GetUserRequest{}, true},
		{"user get me ok", &identitypbv1.GetUserRequest{Name: "users/me"}, false},
		{"user update missing user", &identitypbv1.UpdateUserRequest{}, true},
		{"user update bad name", &identitypbv1.UpdateUserRequest{User: &identitypbv1.User{Name: "people/1"}}, true},

		// property
		{"property create missing time_zone", &propertypbv1.CreatePropertyRequest{Property: &propertypbv1.Property{Organisation: "organisations/o1", DisplayName: "Grand"}}, true},
		{"property create ok", &propertypbv1.CreatePropertyRequest{Property: &propertypbv1.Property{Organisation: "organisations/o1", DisplayName: "Grand", TimeZone: "UTC"}}, false},
		{"unit create unset type", &propertypbv1.CreateUnitRequest{Parent: "properties/p1", Unit: &propertypbv1.Unit{DisplayName: "Room", BookingMode: 1, TimeZone: "UTC"}}, true},
		{"unit create ok", &propertypbv1.CreateUnitRequest{Parent: "properties/p1", Unit: &propertypbv1.Unit{DisplayName: "Room", Type: propertypbv1.UnitType_UNIT_TYPE_ROOM, BookingMode: 1, TimeZone: "UTC"}}, false},
		{"licence create foreign unit", &propertypbv1.CreateLicenceRequest{Parent: "properties/p1", Licence: &propertypbv1.Licence{Type: propertypbv1.LicenceType_LICENCE_TYPE_FIRE_SAFETY, Unit: "properties/p2/units/u1"}}, true},
		{"licence create own unit ok", &propertypbv1.CreateLicenceRequest{Parent: "properties/p1", Licence: &propertypbv1.Licence{Type: propertypbv1.LicenceType_LICENCE_TYPE_FIRE_SAFETY, Unit: "properties/p1/units/u1"}}, false},
		{"unit delete bad name", &propertypbv1.DeleteUnitRequest{Name: "units/u1"}, true},

		// schedule
		{"schedule get bad name", &schedulepbv1.GetScheduleRequest{Name: "properties/p1/units/u1"}, true},
		{"exception create no span", &schedulepbv1.CreateAvailabilityExceptionRequest{Parent: "properties/p1/units/u1", AvailabilityException: &schedulepbv1.AvailabilityException{Kind: schedulepbv1.ExceptionKind_EXCEPTION_KIND_CLOSURE}}, true},
		{"exception create ok", &schedulepbv1.CreateAvailabilityExceptionRequest{Parent: "properties/p1/units/u1", AvailabilityException: &schedulepbv1.AvailabilityException{Kind: schedulepbv1.ExceptionKind_EXCEPTION_KIND_CLOSURE, Span: &schedulepbv1.AvailabilityException_Window{Window: window}}}, false},

		// promocode
		{"promocode create no discount", &promocodepbv1.CreatePromoCodeRequest{PromoCode: &promocodepbv1.PromoCode{Code: "X"}}, true},
		{"promocode create percent out of range", &promocodepbv1.CreatePromoCodeRequest{PromoCode: &promocodepbv1.PromoCode{Code: "X", Discount: &promocodepbv1.Discount{Amount: &promocodepbv1.Discount_PercentOff{PercentOff: 150}}}}, true},
		{"promocode create ok", &promocodepbv1.CreatePromoCodeRequest{PromoCode: &promocodepbv1.PromoCode{Code: "X", Discount: &promocodepbv1.Discount{Amount: &promocodepbv1.Discount_PercentOff{PercentOff: 25}}}}, false},
		{"promocode validate missing subtotal", &promocodepbv1.ValidatePromoCodeRequest{Code: "X"}, true},
		{"promocode validate ok", &promocodepbv1.ValidatePromoCodeRequest{Code: "X", Subtotal: &money.Money{CurrencyCode: "USD", Units: 100}}, false},

		// booking
		{"booking create missing window", &schedulingpbv1.CreateBookingRequest{Booking: &schedulingpbv1.Booking{Unit: "properties/p1/units/u1"}}, true},
		{"booking create ok", &schedulingpbv1.CreateBookingRequest{Booking: &schedulingpbv1.Booking{Unit: "properties/p1/units/u1", Window: window}}, false},
		{"booking reschedule missing window", &schedulingpbv1.RescheduleBookingRequest{Name: "bookings/b1"}, true},
		{"booking window missing end", &schedulingpbv1.CreateBookingRequest{Booking: &schedulingpbv1.Booking{Unit: "properties/p1/units/u1", Window: &sharedpbv1.TimeWindow{StartTime: timestamppb.Now()}}}, true},

		// availability
		{"availability compute no period", &availabilitypbv1.ComputeAvailabilityRequest{Unit: "properties/p1/units/u1"}, true},
		{"availability compute ok", &availabilitypbv1.ComputeAvailabilityRequest{Unit: "properties/p1/units/u1", Period: &availabilitypbv1.ComputeAvailabilityRequest_Window{Window: window}}, false},
		{"availability check bad unit", &availabilitypbv1.CheckAvailabilityRequest{Unit: "units/u1", Period: &availabilitypbv1.CheckAvailabilityRequest_Window{Window: window}}, true},
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
