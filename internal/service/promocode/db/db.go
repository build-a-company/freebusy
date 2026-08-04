// Package db is the promo code persistence layer. It defines the
// provider-agnostic PromoCodeRepository contract (spoken in protobuf domain
// types) and a factory that, given a database.Connection, builds the
// implementation for the configured backend (GORM by default, Hasura opt-in).
// The provider-specific implementations live in the gorm and hasura
// sub-packages; each owns its conversion between protobuf types and its storage
// model. Shared, provider-neutral vocabulary (errors, list params, names, field
// masks) lives in internal/types.
package db

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/database"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/promocode/db/gorm"
	"github.com/oh-tarnished/freebusy/internal/service/promocode/db/hasura"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
)

// PromoCodeRepository provides CRUD persistence for promo codes. All methods
// accept and return the protobuf PromoCode type; implementations own the
// conversion to and from their storage models, including the normalized Money
// value-objects and the applicable-resources / applicable-offerings join rows.
// Errors are the sentinels in internal/types (types.ErrNotFound, etc.).
type PromoCodeRepository interface {
	// Create persists pc and returns the stored record with server-set fields
	// (name, timestamps, etag, state) populated.
	Create(ctx context.Context, pc *promocodepbv1.PromoCode) (*promocodepbv1.PromoCode, error)

	// Get returns the promo code identified by its resource name
	// ("promoCodes/{promo_code}"), or types.ErrNotFound.
	Get(ctx context.Context, name string) (*promocodepbv1.PromoCode, error)

	// FindByCode returns the promo code with the given human-entered code (e.g.
	// "SUMMER25"), or types.ErrNotFound. Used by validation/redemption flows that
	// address a code rather than a resource name.
	FindByCode(ctx context.Context, code string) (*promocodepbv1.PromoCode, error)

	// List returns a page of promo codes and an opaque token for the next page
	// (empty when there are no further results). in.Filter is the raw AIP-160
	// expression; the provider parses and dispatches it (including the derived
	// "state" field, which GORM answers via a handler override and Hasura
	// rejects).
	List(ctx context.Context, in repox.ListInput) (items []*promocodepbv1.PromoCode, nextPageToken string, err error)

	// Update persists the fields named by paths (an AIP-134 field mask); an empty
	// paths slice means a full replace. The record is identified by pc.Name and
	// pc.Etag guards against concurrent writes (types.ErrConflict on mismatch).
	Update(ctx context.Context, pc *promocodepbv1.PromoCode, paths []string) (*promocodepbv1.PromoCode, error)

	// Delete removes the promo code identified by its resource name, returning
	// types.ErrNotFound when it does not exist.
	Delete(ctx context.Context, name string) error
}

// Assert the provider implementations satisfy the contract here, so the
// sub-packages don't need to import this one (which would form an import cycle).
var (
	_ PromoCodeRepository = (*gorm.PromoCodeRepository)(nil)
	_ PromoCodeRepository = (*hasura.PromoCodeRepository)(nil)
)

// New returns the PromoCodeRepository for the configured provider, built over the
// matching handle on conn (conn.Provider).
func New(conn *database.Connection) PromoCodeRepository {
	if conn.Provider == database.ProviderHasura {
		return instrumentHasuraPromoCode(hasura.NewPromoCodeRepository(conn.Hasura))
	}
	return gorm.NewPromoCodeRepository(conn.PgSQLConn)
}
