// Package db is the property persistence layer. It defines the
// provider-agnostic PropertyRepository contract (spoken in protobuf domain
// types) and a factory that, given a database.Connection, builds the
// implementation for the configured backend. The provider-specific
// implementations live in the gorm (and, in a follow-up, hasura) sub-packages;
// each owns its conversion between protobuf types and its storage model. Shared,
// provider-neutral vocabulary (errors, list params, names, field masks) lives in
// internal/types.
package db

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/database"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/property/db/gorm"
	"github.com/oh-tarnished/freebusy/internal/service/property/db/hasura"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/property/v1/propertypbv1"
)

// PropertyRepository provides CRUD persistence for hotels (properties) and their
// bookable units. All methods accept and return the protobuf domain types;
// implementations own the conversion to and from their storage models, including
// the normalized Money/DateRange/PostalAddress value-objects, the media
// galleries, and a unit's pricing children. Errors are the sentinels in
// internal/types (types.ErrNotFound, etc.).
type PropertyRepository interface {
	// CreateProperty persists p and returns the stored record with server-set
	// fields (name, timestamps, etag, state) populated.
	CreateProperty(ctx context.Context, p *propertypbv1.Property) (*propertypbv1.Property, error)

	// GetProperty returns the property identified by its resource name
	// ("properties/{property}"), or types.ErrNotFound.
	GetProperty(ctx context.Context, name string) (*propertypbv1.Property, error)

	// ListProperties returns a page of properties and an opaque next-page token.
	// in.Filter is the raw AIP-160 expression; providers parse and dispatch it.
	ListProperties(ctx context.Context, in repox.ListInput) (items []*propertypbv1.Property, nextPageToken string, err error)

	// UpdateProperty persists the fields named by paths (an AIP-134 field mask);
	// an empty paths slice means a full replace. p.Etag guards against concurrent
	// writes (types.ErrConflict on mismatch).
	UpdateProperty(ctx context.Context, p *propertypbv1.Property, paths []string) (*propertypbv1.Property, error)

	// ArchiveProperty / UnarchiveProperty flip a property's lifecycle state.
	ArchiveProperty(ctx context.Context, name string) (*propertypbv1.Property, error)
	UnarchiveProperty(ctx context.Context, name string) (*propertypbv1.Property, error)

	// CreateUnit persists u under parent ("properties/{property}") and returns the
	// stored record.
	CreateUnit(ctx context.Context, parent string, u *propertypbv1.Unit) (*propertypbv1.Unit, error)

	// GetUnit returns the unit identified by its resource name
	// ("properties/{property}/units/{unit}"), or types.ErrNotFound.
	GetUnit(ctx context.Context, name string) (*propertypbv1.Unit, error)

	// ListUnits returns a page of units under parent ("properties/{property}").
	ListUnits(ctx context.Context, parent string, in repox.ListInput) (items []*propertypbv1.Unit, nextPageToken string, err error)

	// UpdateUnit persists the masked fields of u; an empty mask replaces every
	// mutable field. u.Etag guards against concurrent writes.
	UpdateUnit(ctx context.Context, u *propertypbv1.Unit, paths []string) (*propertypbv1.Unit, error)

	// DeleteUnit removes the unit identified by its resource name, returning
	// types.ErrNotFound when it does not exist. Child licences block the delete
	// unless force is set (AIP-135), in which case they are deleted too.
	DeleteUnit(ctx context.Context, name string, force bool) error

	// CreateLicence persists l under parent ("properties/{property}") and
	// returns the stored record. A licence covering a single unit carries the
	// unit's resource name in l.Unit; the caller validates it belongs to the
	// parent property.
	CreateLicence(ctx context.Context, parent string, l *propertypbv1.Licence) (*propertypbv1.Licence, error)

	// GetLicence returns the licence identified by its resource name
	// ("properties/{property}/licences/{licence}"), or types.ErrNotFound.
	GetLicence(ctx context.Context, name string) (*propertypbv1.Licence, error)

	// ListLicences returns a page of licences under parent
	// ("properties/{property}") — property-wide and per-unit ones alike. The
	// filter narrows by target, unit, type, or state, and supports expiry_date
	// bounds (`expiry_date <= 2026-08-01`) for renewal-reminder queries.
	ListLicences(ctx context.Context, parent string, in repox.ListInput) (items []*propertypbv1.Licence, nextPageToken string, err error)

	// UpdateLicence persists the masked fields of l; an empty mask replaces
	// every mutable field (target and unit are immutable). l.Etag guards
	// against concurrent writes.
	UpdateLicence(ctx context.Context, l *propertypbv1.Licence, paths []string) (*propertypbv1.Licence, error)

	// DeleteLicence removes the licence identified by its resource name.
	DeleteLicence(ctx context.Context, name string) error
}

// Assert the provider implementations satisfy the contract here, so the
// sub-packages don't need to import this one (which would form an import cycle).
var (
	_ PropertyRepository = (*gorm.PropertyRepository)(nil)
	_ PropertyRepository = (*hasura.PropertyRepository)(nil)
)

// New returns the PropertyRepository for the configured provider, built over the
// matching handle on conn (conn.Provider).
func New(conn *database.Connection) PropertyRepository {
	if conn.Provider == database.ProviderHasura {
		return instrumentHasuraProperty(hasura.NewPropertyRepository(conn.Hasura))
	}
	return gorm.NewPropertyRepository(conn.PgSQLConn)
}
