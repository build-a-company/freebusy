package hasura

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/availabilityexceptionsql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/recurringrulesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/refundtiersql"
	resourceql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/schedule/v1/schedulepbv1"
)

// ScheduleRepository is the Hasura-backed schedule + availability-exception
// repository. A Schedule is a per-unit singleton whose child graph is read with
// follow-up queries and written as one atomic mutation batch (svc.Mutation.Tx());
// an AvailabilityException is a separate resource under the unit.
type ScheduleRepository struct {
	svc *freebusyql.Service
}

// NewScheduleRepository returns a Hasura-backed ScheduleRepository bound to svc.
func NewScheduleRepository(svc *freebusyql.Service) *ScheduleRepository {
	return &ScheduleRepository{svc: svc}
}

// --- Schedule ----------------------------------------------------------------

// GetSchedule returns a unit's schedule configuration. When none is stored yet the
// singleton is reported as an empty Schedule (name only). The exceptions list is
// always derived from the unit's AvailabilityException rows.
func (r *ScheduleRepository) GetSchedule(ctx context.Context, name string) (*schedulepbv1.Schedule, error) {
	unitID, err := types.ParseScheduleName(name)
	if err != nil {
		return nil, err
	}
	res, err := r.svc.Query.Schedule.Resource.Find(ctx, resourceql.List().Where(resourceql.Name.Eq(name)))
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	var out *schedulepbv1.Schedule
	if res == nil {
		out = &schedulepbv1.Schedule{Name: name}
	} else {
		parts, _, err := r.hydrateSchedule(ctx, res)
		if err != nil {
			return nil, err
		}
		out = scheduleFromParts(parts)
	}
	names, err := r.exceptionNames(ctx, unitID)
	if err != nil {
		return nil, err
	}
	out.Exceptions = names
	return out, nil
}

// hydrateSchedule loads a schedule row's belongs-to children (buffers, stay
// constraints, cancellation policy + refund tiers) and its recurring rules, and
// returns the ids of the replaceable child rows.
func (r *ScheduleRepository) hydrateSchedule(ctx context.Context, res *resourceql.ScheduleResource) (scheduleParts, scheduleRefs, error) {
	p := scheduleParts{res: res}
	refs := scheduleRefs{buffersID: res.BuffersId, stayID: res.StayConstraintsId, cancelID: res.CancellationPolicyId}

	if res.BuffersId != nil {
		b, err := r.svc.Query.Schedule.BufferSettings.Get(ctx, *res.BuffersId)
		if err != nil {
			return scheduleParts{}, scheduleRefs{}, dbutil.MapHasuraErr(err)
		}
		p.buffers = b
	}
	if res.StayConstraintsId != nil {
		s, err := r.svc.Query.Schedule.StayConstraints.Get(ctx, *res.StayConstraintsId)
		if err != nil {
			return scheduleParts{}, scheduleRefs{}, dbutil.MapHasuraErr(err)
		}
		p.stay = s
	}
	if res.CancellationPolicyId != nil {
		p.hasPolicy = true
		tiers, err := r.svc.Query.Schedule.RefundTiers.List(ctx,
			refundtiersql.List().Where(refundtiersql.CancellationPolicyId.Eq(*res.CancellationPolicyId)))
		if err != nil {
			return scheduleParts{}, scheduleRefs{}, dbutil.MapHasuraErr(err)
		}
		p.refundTiers = tiers
	}
	rules, err := r.svc.Query.Schedule.RecurringRules.List(ctx,
		recurringrulesql.List().Where(recurringrulesql.ScheduleId.Eq(res.Id)))
	if err != nil {
		return scheduleParts{}, scheduleRefs{}, dbutil.MapHasuraErr(err)
	}
	p.recurring = rules
	for i := range rules {
		refs.recurringIDs = append(refs.recurringIDs, rules[i].Id)
	}
	return p, refs, nil
}

// exceptionNames returns the resource names of a unit's availability exceptions.
func (r *ScheduleRepository) exceptionNames(ctx context.Context, unitID string) ([]string, error) {
	rows, err := r.svc.Query.Schedule.AvailabilityExceptions.List(ctx,
		availabilityexceptionsql.List().Where(availabilityexceptionsql.UnitId.Eq(unitID)).OrderBy(availabilityexceptionsql.CreateTime.Asc()))
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	names := make([]string, 0, len(rows))
	for i := range rows {
		names = append(names, rows[i].Name)
	}
	return names, nil
}

// --- AvailabilityException ---------------------------------------------------
