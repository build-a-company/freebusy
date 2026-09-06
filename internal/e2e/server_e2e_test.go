// This file end-to-end tests the ASSEMBLED server: the real *internal.Service
// (every domain server on its provider-selected repository) behind the real
// protovalidate interceptor chain, served over an in-memory bufconn listener
// and driven through the generated gRPC clients — exactly the stack a
// production client talks to, minus the TCP socket.
//
// Rebuilt after the catalogue moved to the RFC services. The suite no longer
// creates a Property and a Unit to book against, because freebusy no longer owns
// either: a booking names an RFC 9073 resource, and freebusy does not require
// that resource to exist. The catalogue only *enriches* a booking — occupancy
// limit and timezone — so an end-to-end booking flow needs no RFC server, which
// is why these tests can run against Postgres alone.
//
// Two env-gated matrices mirror the live-suite conventions:
//
//	FREEBUSY_TEST_POSTGRES_DSN="host=... dbname=freebusydb ..." go test ./internal/e2e/ -run TestE2E_Gorm -v
//	FREEBUSY_TEST_GRAPHQL_URL="http://localhost:3280/graphql"  go test ./internal/e2e/ -run TestE2E_Hasura -v
//
// (run `just migrate` / `just hasura regen` first so the backends are ready).
package e2e

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/oh-tarnished/freebusy/internal"
	"github.com/oh-tarnished/freebusy/internal/database"
	"github.com/oh-tarnished/freebusy/internal/database/repository/freebusy/scheduling"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/pricing/v1/pricingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestE2E_Gorm(t *testing.T) {
	gdb := openTestGorm(t)
	conn := &database.Connection{PgSQLConn: gdb, Provider: database.ProviderGorm}
	cc := dialServer(t, conn)
	serverLifecycle(t, cc, conn, scheduling.NewGorm(gdb))
}

// TestE2E_Hasura runs the identical client-visible lifecycle with the server
// assembled on the Hasura DDN backend — one behavior, two providers.
func TestE2E_Hasura(t *testing.T) {
	svc := connectTestGraphQL(t)
	conn := &database.Connection{Hasura: svc, Provider: database.ProviderHasura}
	cc := dialServer(t, conn)
	serverLifecycle(t, cc, conn, scheduling.NewGraphQL(svc))
}

// e2eClients bundles the per-service gRPC clients plus the generated repository
// set used only to clean up rows the API deliberately never deletes (bookings
// only cancel).
type e2eClients struct {
	suffix    string
	bookings  schedulingpbv1.SchedulingServiceClient
	promos    promocodepbv1.PromoCodeServiceClient
	ratePlans pricingpbv1.RatePlanServiceClient
	bookRepos scheduling.Repositories
}

// resourceName is the RFC 9073 resource a booking names.
//
// Deliberately not created anywhere: freebusy stores no catalogue, and the
// booking path treats an unknown resource as one with no profile — capacity one,
// unbounded occupancy, UTC. Asserting a booking succeeds against a name nothing
// registered is the point, because that is the contract the catalogue split
// introduced.
func (c *e2eClients) resourceName() string { return "resources/e2e-" + c.suffix }

// serverLifecycle drives the services through the wire: interceptor rejections,
// the booking hold flow, and promo validation. Each flow registers its own
// t.Cleanup, so teardown runs LIFO in dependency order.
func serverLifecycle(t *testing.T, cc *grpc.ClientConn, conn *database.Connection, bookRepos scheduling.Repositories) {
	t.Helper()
	c := &e2eClients{
		suffix:    fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000),
		bookings:  schedulingpbv1.NewSchedulingServiceClient(cc),
		promos:    promocodepbv1.NewPromoCodeServiceClient(cc),
		ratePlans: pricingpbv1.NewRatePlanServiceClient(cc),
		bookRepos: bookRepos,
	}

	rejectionFlow(t, c)
	promoFlow(t, c, conn.Provider)
	bookingFlow(t, c)
}

// rejectionFlow pins that the protovalidate interceptor runs in the assembled
// server, not just in unit tests: a malformed resource name is refused before
// any repository is touched.
func rejectionFlow(t *testing.T, c *e2eClients) {
	t.Helper()
	ctx := context.Background()

	_, err := c.bookings.GetBooking(ctx, &schedulingpbv1.GetBookingRequest{Name: "not-a-booking-name"})
	wantCode(t, err, codes.InvalidArgument, "malformed booking name")

	_, err = c.bookings.CreateBooking(ctx, &schedulingpbv1.CreateBookingRequest{
		Booking: &schedulingpbv1.Booking{Unit: "properties/p1/units/u1"},
	})
	wantCode(t, err, codes.InvalidArgument, "pre-RFC unit name is no longer a valid resource name")
}

// wantCode asserts err carries the expected gRPC status code.
func wantCode(t *testing.T, err error, want codes.Code, what string) {
	t.Helper()
	if status.Code(err) != want {
		t.Fatalf("%s: code = %v (err %v), want %v", what, status.Code(err), err, want)
	}
}

// dialServer assembles the full server on conn, serves it over bufconn, and
// returns a connected client conn. Everything is torn down with the test.
func dialServer(t *testing.T, conn *database.Connection) *grpc.ClientConn {
	t.Helper()
	srv, _, err := internal.NewGRPCServer(conn)
	if err != nil {
		t.Fatalf("assemble server: %v", err)
	}
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	cc, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	return cc
}
