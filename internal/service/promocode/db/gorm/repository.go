// Package gorm provides the GORM-backed implementation of the promocode
// persistence contract (internal/service/promocode/db.PromoCodeRepository). It
// adapts the generated per-entity stores under
// protobuf/generated/gorm/freebusy/... to that contract, converting between
// protobuf domain types and the relational storage models.
package gorm

import (
	"context"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/filterx"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/promocode"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"gorm.io/gorm"
)

// PromoCodeRepository is the GORM-backed promo code repository. Each write
// persists the promo code together with its normalized child rows (discount,
// redemption window, usage limits, scope), their Money value-objects (common
// schema), and the scope's applicable-resources / -offerings join rows inside a
// single transaction; each read re-hydrates them via preloads.
type PromoCodeRepository struct {
	db *gorm.DB
}

// NewPromoCodeRepository returns a GORM-backed PromoCodeRepository bound to db.
// The parent db package asserts it satisfies db.PromoCodeRepository.
func NewPromoCodeRepository(db *gorm.DB) *PromoCodeRepository {
	return &PromoCodeRepository{db: db}
}

// preloadGraph eager-loads a promo code's full association graph onto db so a
// single fetch hydrates the discount, window, limits, and scope (with its Money
// value-objects and applicable join rows) without per-row lookups. Taking a base
// *gorm.DB lets both the repository and an in-flight transaction share it.
func preloadGraph(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Discount.AmountOff").
		Preload("Window").
		Preload("Limits").
		Preload("Scope.MinSubtotal").
		Preload("Scope.ApplicableProperties").
		Preload("Scope.ApplicableUnits")
}

// persistChildren inserts the Money rows and belongs-to children in foreign-key
// order (referenced rows before the resource that references them), then the
// scope's join rows. The resource row itself is created (Create) or updated
// (Update) by the caller, which owns its identity.
func (g *promoGraph) persistChildren(ctx context.Context, tx *gorm.DB) error {
	moneys := common.NewMoneyStore(tx)
	for _, m := range g.moneys {
		if e := moneys.Create(ctx, m); e != nil {
			return e
		}
	}
	if e := promocode.NewDiscountStore(tx).Create(ctx, g.discount); e != nil {
		return e
	}
	if g.window != nil {
		if e := promocode.NewRedemptionWindowStore(tx).Create(ctx, g.window); e != nil {
			return e
		}
	}
	if g.limits != nil {
		if e := promocode.NewUsageLimitsStore(tx).Create(ctx, g.limits); e != nil {
			return e
		}
	}
	if g.scope != nil {
		// The applicable-name arrays are columns on the scope row itself, so
		// this single insert carries them; the two join-table inserts it
		// replaced went away with the join tables.
		if e := promocode.NewScopeStore(tx).Create(ctx, g.scope); e != nil {
			return e
		}
	}
	return nil
}

// persist inserts the full graph: the children and then the resource row that
// references them.
func (g *promoGraph) persist(ctx context.Context, tx *gorm.DB) error {
	if err := g.persistChildren(ctx, tx); err != nil {
		return err
	}
	return promocode.NewPromoCodeStore(tx).Create(ctx, g.promo)
}

// Create persists pc and returns the stored record. The resource name is taken
// from pc.Name when present, otherwise a fresh ULID id is assigned.
func (r *PromoCodeRepository) Create(ctx context.Context, pc *promocodepbv1.PromoCode) (*promocodepbv1.PromoCode, error) {
	id, name, err := types.ResolvePromoCodeName(pc.GetName())
	if err != nil {
		return nil, err
	}

	g := buildGraph(pc)
	g.promo.ID = id
	g.promo.Name = name
	g.promo.Etag = repox.Ptr(ulid.GenerateString())

	if err := r.db.Transaction(func(tx *gorm.DB) error {
		return g.persist(ctx, tx)
	}); err != nil {
		return nil, repox.MapGormErr(err)
	}
	return r.Get(ctx, name)
}

// Get returns the promo code addressed by its resource name.
func (r *PromoCodeRepository) Get(ctx context.Context, name string) (*promocodepbv1.PromoCode, error) {
	id, err := types.PromoCodeID(name)
	if err != nil {
		return nil, err
	}
	var m promocode.PromoCode
	if err := preloadGraph(r.db.WithContext(ctx)).First(&m, "id = ?", id).Error; err != nil {
		return nil, repox.MapGormErr(err)
	}
	return fromModel(&m), nil
}

// FindByCode returns the promo code with the given human-entered code.
func (r *PromoCodeRepository) FindByCode(ctx context.Context, code string) (*promocodepbv1.PromoCode, error) {
	var m promocode.PromoCode
	if err := preloadGraph(r.db.WithContext(ctx)).First(&m, "code = ?", code).Error; err != nil {
		return nil, repox.MapGormErr(err)
	}
	return fromModel(&m), nil
}

// List returns a page of promo codes ordered by params.OrderBy. It fetches one
// extra row to decide whether a further page exists; the whole association graph
// is preloaded in batch queries to avoid an N+1.
func (r *PromoCodeRepository) List(ctx context.Context, in repox.ListInput) ([]*promocodepbv1.PromoCode, string, error) {
	conds, err := filterx.Parse(in.Filter)
	if err != nil {
		return nil, "", repox.MapFilterxErr(err)
	}
	models, next, err := filterx.Gorm[promocode.PromoCode](promocode.PromoCodeFilterSpec).
		Override("state", stateHandler(time.Now().UTC())).
		List(ctx, preloadGraph(r.db), filterx.ListInput{
			PageSize:  in.PageSize,
			PageToken: in.PageToken,
			OrderBy:   in.OrderBy,
			Filter:    conds,
		})
	if err != nil {
		return nil, "", repox.MapGormErr(repox.MapFilterxErr(err))
	}

	items := make([]*promocodepbv1.PromoCode, 0, len(models))
	for i := range models {
		items = append(items, fromModel(&models[i]))
	}
	return items, next, nil
}
