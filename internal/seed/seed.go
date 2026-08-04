// Package seed plants dev-only baseline data at startup. It is gated by the
// [seed] config block (off in the release defaults), so it never runs in
// production. Its whole reason to exist is that a freshly rebuilt database —
// the schema rebuilds that come with a proto change — starts empty, and every
// property and unit needs an organisation to belong to. Rather than recreate
// that organisation by hand after each rebuild, the server plants a known one.
package seed

import (
	"context"
	"errors"

	"github.com/oh-tarnished/freebusy/config"
	"github.com/oh-tarnished/freebusy/internal/database"
	orgdb "github.com/oh-tarnished/freebusy/internal/service/organisation/db"
	"github.com/oh-tarnished/freebusy/internal/types"
	orgpbv1 "github.com/oh-tarnished/freebusy/protobuf/generated/go/organisation/v1/orgpbv1"
	"github.com/oh-tarnished/freebusy/shared"
)

// Run plants the configured seed data on conn, following whichever provider the
// connection was opened for. It is a no-op unless [seed] is enabled, and it is
// idempotent: the organisation is addressed by a pinned resource name, so a
// second start finds the existing one and leaves it untouched rather than
// creating a duplicate. Seeding failures are logged and returned, but the caller
// treats them as non-fatal — a dev convenience should not stop the server from
// serving.
func Run(ctx context.Context, conn *database.Connection, cfg config.SeedConfig) error {
	if !cfg.Enabled {
		return nil
	}
	return seedOrganisation(ctx, orgdb.New(conn), cfg.Organisation)
}

// seedOrganisation ensures the configured organisation exists, keyed on its
// stable ID so restarts are idempotent.
func seedOrganisation(ctx context.Context, repo orgdb.OrganisationRepository, o config.SeedOrganisation) error {
	if o.ID == "" || o.DisplayName == "" {
		return errors.New("seed: organisation id and display_name are required when seeding is enabled")
	}
	name, err := types.OrganisationName(o.ID)
	if err != nil {
		return err
	}

	switch _, err := repo.GetOrganisation(ctx, name); {
	case err == nil:
		shared.Telemetry.Logger.Info("seed: organisation already present, skipping", "name", name)
		return nil
	case !errors.Is(err, types.ErrNotFound):
		// A real read failure (not simply "absent"); surface it.
		return err
	}

	created, err := repo.CreateOrganisation(ctx, &orgpbv1.Organisation{
		Name:         name,
		DisplayName:  o.DisplayName,
		Slug:         o.Slug,
		BillingEmail: o.BillingEmail,
	})
	if err != nil {
		return err
	}
	shared.Telemetry.Logger.Info("seed: organisation created",
		"name", created.GetName(), "display_name", created.GetDisplayName())
	return nil
}
