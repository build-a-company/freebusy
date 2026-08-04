package db

import (
	"context"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/graphqlx"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/internal/service/property/db/hasura"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/property/v1/propertypbv1"
)

// The "table" attributes below match the GORM side's per-entity stores
// (internal/database/gorm/freebusy/property/*_store.go), so
// freebusy_orm_store_ops_total / freebusy_orm_store_duration_ms group the
// same logical entity across providers.
const (
	propertyResource = "property.properties"
	unitResource     = "property.units"
	licenceResource  = "property.licences"
)

// instrumentedPropertyRepository wraps a Hasura-backed PropertyRepository so
// every call emits the same ops-counter + duration-histogram + trace span
// that GORM's generated stores already emit via ormtelemetry (see
// internal/database/hasura/graphqlx). GORM is not wrapped here — it is
// instrumented at the SQL layer by freebusy.Default.Instrument
// (internal/database/open.go), and wrapping it again would double-count.
type instrumentedPropertyRepository struct {
	repo *hasura.PropertyRepository
	t    graphqlx.Telemetry
}

// instrumentHasuraProperty wraps repo with the process-wide telemetry client.
func instrumentHasuraProperty(repo *hasura.PropertyRepository) PropertyRepository {
	return &instrumentedPropertyRepository{repo: repo, t: graphqlx.Default()}
}

func (i *instrumentedPropertyRepository) CreateProperty(ctx context.Context, p *propertypbv1.Property) (out *propertypbv1.Property, err error) {
	err = graphqlx.Wrap(ctx, i.t, propertyResource, "create", func(ctx context.Context) error {
		out, err = i.repo.CreateProperty(ctx, p)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) GetProperty(ctx context.Context, name string) (out *propertypbv1.Property, err error) {
	err = graphqlx.Wrap(ctx, i.t, propertyResource, "get", func(ctx context.Context) error {
		out, err = i.repo.GetProperty(ctx, name)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) ListProperties(ctx context.Context, in repox.ListInput) (items []*propertypbv1.Property, nextPageToken string, err error) {
	err = graphqlx.Wrap(ctx, i.t, propertyResource, "list", func(ctx context.Context) error {
		items, nextPageToken, err = i.repo.ListProperties(ctx, in)
		return err
	})
	return items, nextPageToken, err
}

func (i *instrumentedPropertyRepository) UpdateProperty(ctx context.Context, p *propertypbv1.Property, paths []string) (out *propertypbv1.Property, err error) {
	err = graphqlx.Wrap(ctx, i.t, propertyResource, "update", func(ctx context.Context) error {
		out, err = i.repo.UpdateProperty(ctx, p, paths)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) ArchiveProperty(ctx context.Context, name string) (out *propertypbv1.Property, err error) {
	err = graphqlx.Wrap(ctx, i.t, propertyResource, "archive", func(ctx context.Context) error {
		out, err = i.repo.ArchiveProperty(ctx, name)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) UnarchiveProperty(ctx context.Context, name string) (out *propertypbv1.Property, err error) {
	err = graphqlx.Wrap(ctx, i.t, propertyResource, "unarchive", func(ctx context.Context) error {
		out, err = i.repo.UnarchiveProperty(ctx, name)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) CreateUnit(ctx context.Context, parent string, u *propertypbv1.Unit) (out *propertypbv1.Unit, err error) {
	err = graphqlx.Wrap(ctx, i.t, unitResource, "create", func(ctx context.Context) error {
		out, err = i.repo.CreateUnit(ctx, parent, u)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) GetUnit(ctx context.Context, name string) (out *propertypbv1.Unit, err error) {
	err = graphqlx.Wrap(ctx, i.t, unitResource, "get", func(ctx context.Context) error {
		out, err = i.repo.GetUnit(ctx, name)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) ListUnits(ctx context.Context, parent string, in repox.ListInput) (items []*propertypbv1.Unit, nextPageToken string, err error) {
	err = graphqlx.Wrap(ctx, i.t, unitResource, "list", func(ctx context.Context) error {
		items, nextPageToken, err = i.repo.ListUnits(ctx, parent, in)
		return err
	})
	return items, nextPageToken, err
}

func (i *instrumentedPropertyRepository) UpdateUnit(ctx context.Context, u *propertypbv1.Unit, paths []string) (out *propertypbv1.Unit, err error) {
	err = graphqlx.Wrap(ctx, i.t, unitResource, "update", func(ctx context.Context) error {
		out, err = i.repo.UpdateUnit(ctx, u, paths)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) DeleteUnit(ctx context.Context, name string, force bool) (err error) {
	return graphqlx.Wrap(ctx, i.t, unitResource, "delete", func(ctx context.Context) error {
		return i.repo.DeleteUnit(ctx, name, force)
	})
}

func (i *instrumentedPropertyRepository) CreateLicence(ctx context.Context, parent string, l *propertypbv1.Licence) (out *propertypbv1.Licence, err error) {
	err = graphqlx.Wrap(ctx, i.t, licenceResource, "create", func(ctx context.Context) error {
		out, err = i.repo.CreateLicence(ctx, parent, l)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) GetLicence(ctx context.Context, name string) (out *propertypbv1.Licence, err error) {
	err = graphqlx.Wrap(ctx, i.t, licenceResource, "get", func(ctx context.Context) error {
		out, err = i.repo.GetLicence(ctx, name)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) ListLicences(ctx context.Context, parent string, in repox.ListInput) (items []*propertypbv1.Licence, nextPageToken string, err error) {
	err = graphqlx.Wrap(ctx, i.t, licenceResource, "list", func(ctx context.Context) error {
		items, nextPageToken, err = i.repo.ListLicences(ctx, parent, in)
		return err
	})
	return items, nextPageToken, err
}

func (i *instrumentedPropertyRepository) UpdateLicence(ctx context.Context, l *propertypbv1.Licence, paths []string) (out *propertypbv1.Licence, err error) {
	err = graphqlx.Wrap(ctx, i.t, licenceResource, "update", func(ctx context.Context) error {
		out, err = i.repo.UpdateLicence(ctx, l, paths)
		return err
	})
	return out, err
}

func (i *instrumentedPropertyRepository) DeleteLicence(ctx context.Context, name string) (err error) {
	return graphqlx.Wrap(ctx, i.t, licenceResource, "delete", func(ctx context.Context) error {
		return i.repo.DeleteLicence(ctx, name)
	})
}
