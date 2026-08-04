package db

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/graphqlx"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/promocode/db/hasura"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
)

// promoCodeResource is the "table" attribute every wrapped operation below
// carries (see graphqlx.Wrap). It matches the GORM side's promo_code_store.go
// (RecordOp(ctx, "promocode.resource", ...)), so freebusy_orm_store_ops_total
// / freebusy_orm_store_duration_ms group the same logical entity across
// providers.
const promoCodeResource = "promocode.resource"

// instrumentedPromoCodeRepository wraps a Hasura-backed PromoCodeRepository
// so every call emits the same ops-counter + duration-histogram + trace span
// that GORM's generated stores already emit via ormtelemetry (see
// internal/database/hasura/graphqlx). GORM is not wrapped here — it is
// instrumented at the SQL layer by freebusy.Default.Instrument
// (internal/database/open.go), and wrapping it again would double-count.
type instrumentedPromoCodeRepository struct {
	repo *hasura.PromoCodeRepository
	t    graphqlx.Telemetry
}

// instrumentHasuraPromoCode wraps repo with the process-wide telemetry client.
func instrumentHasuraPromoCode(repo *hasura.PromoCodeRepository) PromoCodeRepository {
	return &instrumentedPromoCodeRepository{repo: repo, t: graphqlx.Default()}
}

func (i *instrumentedPromoCodeRepository) Create(ctx context.Context, pc *promocodepbv1.PromoCode) (out *promocodepbv1.PromoCode, err error) {
	err = graphqlx.Wrap(ctx, i.t, promoCodeResource, "create", func(ctx context.Context) error {
		out, err = i.repo.Create(ctx, pc)
		return err
	})
	return out, err
}

func (i *instrumentedPromoCodeRepository) Get(ctx context.Context, name string) (out *promocodepbv1.PromoCode, err error) {
	err = graphqlx.Wrap(ctx, i.t, promoCodeResource, "get", func(ctx context.Context) error {
		out, err = i.repo.Get(ctx, name)
		return err
	})
	return out, err
}

func (i *instrumentedPromoCodeRepository) FindByCode(ctx context.Context, code string) (out *promocodepbv1.PromoCode, err error) {
	err = graphqlx.Wrap(ctx, i.t, promoCodeResource, "find_by_code", func(ctx context.Context) error {
		out, err = i.repo.FindByCode(ctx, code)
		return err
	})
	return out, err
}

func (i *instrumentedPromoCodeRepository) List(ctx context.Context, in repox.ListInput) (items []*promocodepbv1.PromoCode, nextPageToken string, err error) {
	err = graphqlx.Wrap(ctx, i.t, promoCodeResource, "list", func(ctx context.Context) error {
		items, nextPageToken, err = i.repo.List(ctx, in)
		return err
	})
	return items, nextPageToken, err
}

func (i *instrumentedPromoCodeRepository) Update(ctx context.Context, pc *promocodepbv1.PromoCode, paths []string) (out *promocodepbv1.PromoCode, err error) {
	err = graphqlx.Wrap(ctx, i.t, promoCodeResource, "update", func(ctx context.Context) error {
		out, err = i.repo.Update(ctx, pc, paths)
		return err
	})
	return out, err
}

func (i *instrumentedPromoCodeRepository) Delete(ctx context.Context, name string) (err error) {
	return graphqlx.Wrap(ctx, i.t, promoCodeResource, "delete", func(ctx context.Context) error {
		return i.repo.Delete(ctx, name)
	})
}
