// Package gorm provides the GORM-backed implementation of the schedule
// persistence contract (internal/service/schedule/db.ScheduleRepository). A
// Schedule is a per-unit singleton with a normalized child graph; an
// AvailabilityException is a separate resource under the unit.
package gorm

import (
	"context"
	"errors"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/schedule"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/schedule/v1/schedulepbv1"
	"gorm.io/gorm"
)

// ScheduleRepository is the GORM-backed schedule + availability-exception
// repository.
type ScheduleRepository struct {
	db *gorm.DB
}

// NewScheduleRepository returns a GORM-backed ScheduleRepository bound to db.
func NewScheduleRepository(db *gorm.DB) *ScheduleRepository {
	return &ScheduleRepository{db: db}
}

func preloadSchedule(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Buffers").
		Preload("StayConstraints").
		Preload("CancellationPolicy.RefundTiers").
		Preload("RecurringRules")
}

// persist inserts a schedule graph: belongs-to children (buffers, stay
// constraints, cancellation policy) first, then the schedule row, then the
// has-many refund tiers (referencing the policy) and recurring rules
// (referencing the schedule).
func (g *scheduleGraph) persist(ctx context.Context, tx *gorm.DB) error {
	if g.buffers != nil {
		if e := schedule.NewBufferSettingsStore(tx).Create(ctx, g.buffers); e != nil {
			return e
		}
	}
	if g.stayConstraints != nil {
		if e := schedule.NewStayConstraintsStore(tx).Create(ctx, g.stayConstraints); e != nil {
			return e
		}
	}
	if g.cancellationPolicy != nil {
		if e := schedule.NewCancellationPolicyStore(tx).Create(ctx, g.cancellationPolicy); e != nil {
			return e
		}
	}
	if e := schedule.NewScheduleStore(tx).Create(ctx, g.schedule); e != nil {
		return e
	}
	return g.persistChildren(ctx, tx)
}

// persistChildren inserts the refund tiers and recurring rules; the schedule row
// and its belongs-to children must already exist. Used by both create and update.
func (g *scheduleGraph) persistChildren(ctx context.Context, tx *gorm.DB) error {
	tiers := schedule.NewRefundTierStore(tx)
	for _, t := range g.refundTiers {
		t.CancellationPolicyID = g.cancellationPolicy.ID
		if e := tiers.Create(ctx, t); e != nil {
			return e
		}
	}
	rules := schedule.NewRecurringRuleStore(tx)
	for _, r := range g.recurringRules {
		r.ScheduleID = g.schedule.ID
		if e := rules.Create(ctx, r); e != nil {
			return e
		}
	}
	return nil
}

// --- Schedule ----------------------------------------------------------------

// GetSchedule returns a unit's schedule configuration. When none is stored yet
// the singleton is reported as an empty Schedule (name only). The exceptions list
// is always derived from the unit's AvailabilityException rows.
func (r *ScheduleRepository) GetSchedule(ctx context.Context, name string) (*schedulepbv1.Schedule, error) {
	unitID, err := types.ParseScheduleName(name)
	if err != nil {
		return nil, err
	}
	var out *schedulepbv1.Schedule
	var m schedule.Schedule
	switch err := preloadSchedule(r.db.WithContext(ctx)).First(&m, "name = ?", name).Error; {
	case err == nil:
		out = scheduleFromModel(&m)
	case errors.Is(err, gorm.ErrRecordNotFound):
		out = &schedulepbv1.Schedule{Name: name}
	default:
		return nil, repox.MapGormErr(err)
	}
	names, err := r.exceptionNames(ctx, unitID)
	if err != nil {
		return nil, err
	}
	out.Exceptions = names
	return out, nil
}

// exceptionNames returns the resource names of a unit's availability exceptions.
func (r *ScheduleRepository) exceptionNames(ctx context.Context, unitID string) ([]string, error) {
	var rows []schedule.AvailabilityException
	if err := r.db.WithContext(ctx).Model(&schedule.AvailabilityException{}).
		Where("unit_id = ?", unitID).Order("create_time").Find(&rows).Error; err != nil {
		return nil, repox.MapGormErr(err)
	}
	names := make([]string, 0, len(rows))
	for i := range rows {
		names = append(names, rows[i].Name)
	}
	return names, nil
}
