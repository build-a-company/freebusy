package database

import (
	"fmt"
	"net/url"

	"github.com/oh-tarnished/freebusy/config"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql"
	"github.com/oh-tarnished/freebusy/shared"
	"github.com/the-protobuf-project/runtime-go/network/runtime"
	"go.opentelemetry.io/otel/propagation"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open opens the database backend selected by config ([database].provider) and
// returns a Connection carrying the live handle for that provider. The caller
// passes the Connection to NewFactory.
func Open() (*Connection, error) {
	switch providerFromConfig() {
	case ProviderHasura:
		return openHasura()
	default:
		return openGorm()
	}
}

// openGorm dials Postgres with the libpq DSN rendered from config and bounds
// the connection pool. The pool cap is the process's backpressure point: excess
// concurrent queries queue on the pool instead of piling onto Postgres. It also
// installs the generated first-party opentelementry GORM plugin, so every
// query the application runs through this handle emits a span and metric
// through shared.Telemetry.
func openGorm() (*Connection, error) {
	pg := config.Get().Database.Postgres
	db, err := gorm.Open(postgres.Open(pg.DSN()), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := freebusy.Default.Instrument(db, shared.Telemetry); err != nil {
		return nil, fmt.Errorf("instrument postgres: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres pool handle: %w", err)
	}
	pool := pg.Pool()
	sqlDB.SetMaxOpenConns(pool.MaxOpen)
	sqlDB.SetMaxIdleConns(pool.MaxIdle)
	sqlDB.SetConnMaxLifetime(pool.MaxLifetime)
	sqlDB.SetConnMaxIdleTime(pool.MaxIdleTime)
	return &Connection{PgSQLConn: db, Provider: ProviderGorm}, nil
}

// openHasura connects the typed GraphQL client to the configured endpoint,
// sending the admin secret as the x-hasura-admin-secret header when set. The
// W3C traceparent propagator injects the calling context's active span into
// every request, so Hasura's own ddn-engine spans nest as children of ours in
// the same trace.
func openHasura() (*Connection, error) {
	h := config.Get().Database.Hasura
	u, err := url.Parse(h.URL)
	if err != nil {
		return nil, fmt.Errorf("parse hasura url %q: %w", h.URL, err)
	}
	var headers map[string]string
	if h.AdminSecret != "" {
		headers = map[string]string{"x-hasura-admin-secret": h.AdminSecret}
	}
	svc, err := freebusyql.New(runtime.ConnectionOptions{
		URL:             runtime.URLFromStd(u),
		Headers:         headers,
		TracePropagator: propagation.TraceContext{},
	})
	if err != nil {
		return nil, fmt.Errorf("connect hasura: %w", err)
	}
	return &Connection{Hasura: svc, Provider: ProviderHasura}, nil
}
