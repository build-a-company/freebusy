// This file end-to-end tests the ASSEMBLED server: the real *internal.Service
// (every domain server on its provider-selected repository) behind the real
// protovalidate interceptor chain, served over an in-memory bufconn listener
// and driven through the generated gRPC clients — exactly the stack a
// production client talks to, minus the TCP socket. The per-domain flows live
// in the server_*_flow_test.go siblings.
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
	"github.com/oh-tarnished/freebusy/internal/database/repository/freebusy/property"
	"github.com/oh-tarnished/freebusy/internal/service/scheduling/db"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/availability/v1/availabilitypbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/scheduling/v1/schedulingpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/identity/v1/identitypbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/organisation/v1/orgpbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/property/v1/propertypbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/schedule/v1/schedulepbv1"
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
	serverLifecycle(t, cc, conn, property.NewGorm(gdb), scheduling.NewGorm(gdb))
}

// TestE2E_Hasura runs the identical client-visible lifecycle with the server
// assembled on the Hasura DDN backend — one behavior, two providers.
func TestE2E_Hasura(t *testing.T) {
	svc := connectTestGraphQL(t)
	conn := &database.Connection{Hasura: svc, Provider: database.ProviderHasura}
	cc := dialServer(t, conn)
	serverLifecycle(t, cc, conn, property.NewGraphQL(svc), scheduling.NewGraphQL(svc))
}

// e2eClients bundles the per-service gRPC clients plus the generated
// repository sets used only to clean up rows the API deliberately never
// deletes (PropertyService archives; bookings only cancel).
type e2eClients struct {
	suffix    string
	orgs      orgpbv1.OrganisationServiceClient
	props     propertypbv1.PropertyServiceClient
	licences  propertypbv1.LicenceServiceClient
	schedules schedulepbv1.ScheduleServiceClient
	bookings  schedulingpbv1.SchedulingServiceClient
	promos    promocodepbv1.PromoCodeServiceClient
	avail     availabilitypbv1.AvailabilityServiceClient
	identity  identitypbv1.IdentityServiceClient
	propRepos property.Repositories
	bookRepos scheduling.Repositories
}

// serverLifecycle drives every service through the wire: interceptor
// rejections, full CRUD with masks and etags, the booking hold flow, hold
// expiry, promo validation, and the provider-visible filter divergence on
// derived state. Each flow registers its own t.Cleanup, so teardown runs LIFO
// in dependency order.
func serverLifecycle(t *testing.T, cc *grpc.ClientConn, conn *database.Connection, propRepos property.Repositories, bookRepos scheduling.Repositories) {
	t.Helper()
	provider := conn.Provider
	c := &e2eClients{
		suffix:    fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000),
		orgs:      orgpbv1.NewOrganisationServiceClient(cc),
		props:     propertypbv1.NewPropertyServiceClient(cc),
		licences:  propertypbv1.NewLicenceServiceClient(cc),
		schedules: schedulepbv1.NewScheduleServiceClient(cc),
		bookings:  schedulingpbv1.NewSchedulingServiceClient(cc),
		promos:    promocodepbv1.NewPromoCodeServiceClient(cc),
		avail:     availabilitypbv1.NewAvailabilityServiceClient(cc),
		identity:  identitypbv1.NewIdentityServiceClient(cc),
		propRepos: propRepos,
		bookRepos: bookRepos,
	}

	rejectionFlow(t, c)
	org := orgFlow(t, c)
	unit := propertyFlow(t, c, org)
	scheduleFlow(t, c, unit)
	promoFlow(t, c, provider)
	bookingFlow(t, c, unit)
	holdExpiryFlow(t, c, unit, db.New(conn))
	identityFlow(t, c)
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

// wantCode asserts err carries the given gRPC status code.
func wantCode(t *testing.T, err error, want codes.Code, what string) {
	t.Helper()
	if status.Code(err) != want {
		t.Fatalf("%s: got %v (err %v), want %v", what, status.Code(err), err, want)
	}
}
