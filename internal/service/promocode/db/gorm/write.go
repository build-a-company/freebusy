package gorm

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/promocode"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"gorm.io/gorm"
)

// Update applies the masked fields of pc to the stored record and returns the
// result. An empty paths slice replaces every mutable field; pc.Etag, when set,
// guards against concurrent writes (types.ErrConflict on mismatch). The whole
// update runs in one transaction: the merged proto is re-materialized into a
// fresh child graph, the resource row is repointed at it, and the superseded
// child / Money rows are deleted only after the resource no longer references them.
func (r *PromoCodeRepository) Update(ctx context.Context, pc *promocodepbv1.PromoCode, paths []string) (*promocodepbv1.PromoCode, error) {
	id, err := types.PromoCodeID(pc.GetName())
	if err != nil {
		return nil, err
	}

	err = r.db.Transaction(func(tx *gorm.DB) error {
		var existing promocode.PromoCode
		if e := preloadGraph(tx.WithContext(ctx)).First(&existing, "id = ?", id).Error; e != nil {
			return e
		}
		if pc.GetEtag() != "" && existing.Etag != nil && pc.GetEtag() != *existing.Etag {
			return types.ErrConflict
		}
		old := collectRefs(&existing)

		// Merge the masked fields onto a proto built from the stored record, then
		// rebuild the whole child graph from the result.
		merged := fromModel(&existing)
		applyMask(merged, pc, paths)
		g := buildGraph(merged)
		if e := g.persistChildren(ctx, tx); e != nil {
			return e
		}

		// Repoint the resource row at the new children and bump the etag. The
		// association pointers are cleared so GORM's Save only rewrites the FK
		// columns instead of re-upserting the old children.
		existing.Code = g.promo.Code
		existing.DisplayName = g.promo.DisplayName
		existing.Description = g.promo.Description
		existing.State = g.promo.State
		existing.Disabled = g.promo.Disabled
		existing.DiscountID = g.promo.DiscountID
		existing.WindowID = g.promo.WindowID
		existing.LimitsID = g.promo.LimitsID
		existing.ScopeID = g.promo.ScopeID
		existing.Etag = repox.Ptr(ulid.GenerateString())
		existing.Discount, existing.Window, existing.Limits, existing.Scope = nil, nil, nil, nil
		existing.Redemptions = nil
		if e := promocode.NewPromoCodeStore(tx).Update(ctx, &existing); e != nil {
			return e
		}
		return deleteChildren(ctx, tx, old)
	})
	if err != nil {
		return nil, repox.MapGormErr(err)
	}
	return r.Get(ctx, pc.GetName())
}

// Delete removes the promo code, its belongs-to children, their Money
// value-objects, and the scope's join rows.
func (r *PromoCodeRepository) Delete(ctx context.Context, name string) error {
	id, err := types.PromoCodeID(name)
	if err != nil {
		return err
	}
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var existing promocode.PromoCode
		if e := preloadGraph(tx.WithContext(ctx)).First(&existing, "id = ?", id).Error; e != nil {
			return e
		}
		refs := collectRefs(&existing)
		// Delete the resource row first (cascading to its redemptions), then the
		// now-unreferenced children and Money rows.
		if e := promocode.NewPromoCodeStore(tx).DeleteByID(ctx, id); e != nil {
			return e
		}
		return deleteChildren(ctx, tx, refs)
	})
	return repox.MapGormErr(err)
}

// promoRefs captures the ids of a promo code's child rows and Money rows so they
// can be deleted once the resource row no longer references them.
type promoRefs struct {
	discountID string
	windowID   *string
	limitsID   *string
	scopeID    *string
	moneyIDs   []string
}

func collectRefs(m *promocode.PromoCode) promoRefs {
	refs := promoRefs{
		discountID: m.DiscountID,
		windowID:   m.WindowID,
		limitsID:   m.LimitsID,
		scopeID:    m.ScopeID,
	}
	if m.Discount != nil && m.Discount.AmountOffID != nil {
		refs.moneyIDs = append(refs.moneyIDs, *m.Discount.AmountOffID)
	}
	if m.Scope != nil && m.Scope.MinSubtotalID != nil {
		refs.moneyIDs = append(refs.moneyIDs, *m.Scope.MinSubtotalID)
	}
	return refs
}

// deleteChildren removes a promo code's child rows in foreign-key order: the
// scope, then the window / limits / discount, then the Money rows those children
// referenced.
//
// The scope's applicable-name arrays need no deletion of their own: they are
// columns on the scope row and go with it. The two join-table deletes this
// replaced were removed with the join tables, when those references became
// cross-service and could no longer point at anything local.
func deleteChildren(ctx context.Context, tx *gorm.DB, refs promoRefs) error {
	if refs.scopeID != nil {
		if e := promocode.NewScopeStore(tx).DeleteByID(ctx, *refs.scopeID); e != nil {
			return e
		}
	}
	if refs.windowID != nil {
		if e := promocode.NewRedemptionWindowStore(tx).DeleteByID(ctx, *refs.windowID); e != nil {
			return e
		}
	}
	if refs.limitsID != nil {
		if e := promocode.NewUsageLimitsStore(tx).DeleteByID(ctx, *refs.limitsID); e != nil {
			return e
		}
	}
	if refs.discountID != "" {
		if e := promocode.NewDiscountStore(tx).DeleteByID(ctx, refs.discountID); e != nil {
			return e
		}
	}
	moneys := common.NewMoneyStore(tx)
	for _, mid := range refs.moneyIDs {
		if e := moneys.DeleteByID(ctx, mid); e != nil {
			return e
		}
	}
	return nil
}
