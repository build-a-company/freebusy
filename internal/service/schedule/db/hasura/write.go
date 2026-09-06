package hasura

import (
	"context"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/buffersettingsql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/cancellationpoliciesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/recurringrulesql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/refundtiersql"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/stayconstraintsql"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"

	resourceql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/schedule/v1/schedulepbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
	"github.com/the-protobuf-project/runtime-go/network/graphql"
	"github.com/the-protobuf-project/runtime-go/network/runtime"
)

// UpdateSchedule upserts a unit's schedule configuration as one atomic mutation
// batch. The schedule is a singleton (created on first update); an empty paths
// slice replaces every mutable section, and s.Etag guards against concurrent
// writes on an existing schedule. The merged proto is re-materialized into a fresh
// child graph and the superseded rows are deleted in the same batch.
func (r *ScheduleRepository) UpdateSchedule(ctx context.Context, s *schedulepbv1.Schedule, paths []string) (*schedulepbv1.Schedule, error) {
	resourceID, err := types.ParseScheduleName(s.GetName())
	if err != nil {
		return nil, err
	}
	existing, err := r.svc.Query.Schedule.Resource.Find(ctx, resourceql.List().Where(resourceql.Name.Eq(s.GetName())))
	if err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	exists := existing != nil
	if exists && s.GetEtag() != "" && existing.Etag != nil && s.GetEtag() != *existing.Etag {
		return nil, types.ErrConflict
	}

	merged := &schedulepbv1.Schedule{Name: s.GetName()}
	var oldRefs scheduleRefs
	if exists {
		parts, refs, err := r.hydrateSchedule(ctx, existing)
		if err != nil {
			return nil, err
		}
		merged = scheduleFromParts(parts)
		oldRefs = refs
	}
	applyScheduleMask(merged, s, paths)
	g := buildScheduleGraph(merged, resourceID)

	tx := r.svc.Mutation.Tx()
	if g.buffers != nil {
		var res buffersettingsql.InsertScheduleBufferSettingsResponse
		tx.Add(r.svc.Mutation.Schedule.BufferSettings.CreateOp(*g.buffers, &res))
	}
	if g.stayConstraints != nil {
		var res stayconstraintsql.InsertScheduleStayConstraintsResponse
		tx.Add(r.svc.Mutation.Schedule.StayConstraints.CreateOp(*g.stayConstraints, &res))
	}
	if g.cancellationPolicy != nil {
		var res cancellationpoliciesql.InsertScheduleCancellationPoliciesResponse
		tx.Add(r.svc.Mutation.Schedule.CancellationPolicies.CreateOp(*g.cancellationPolicy, &res))
	}

	if exists {
		patch := resourceql.UpdateInput{
			BuffersId:            dbutil.NullableStr(g.schedule.BuffersId),
			StayConstraintsId:    dbutil.NullableStr(g.schedule.StayConstraintsId),
			CancellationPolicyId: dbutil.NullableStr(g.schedule.CancellationPolicyId),
			Etag:                 graphql.Value(ulid.GenerateString()),
		}
		var updRes resourceql.UpdateScheduleResourceByIdResponse
		tx.Add(r.svc.Mutation.Schedule.Resource.UpdateOp(existing.Id, patch, &updRes))
		r.queueScheduleChildren(tx, g, existing.Id)
		queueScheduleChildDeletes(tx, r, oldRefs)
	} else {
		g.schedule.Id = ulid.GenerateString()
		g.schedule.Name = s.GetName()
		g.schedule.Etag = ulid.GenerateString()
		var res resourceql.InsertScheduleResourceResponse
		tx.Add(r.svc.Mutation.Schedule.Resource.CreateOp(g.schedule, &res))
		r.queueScheduleChildren(tx, g, g.schedule.Id)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, dbutil.MapHasuraErr(err)
	}
	return r.GetSchedule(ctx, s.GetName())
}

// queueScheduleChildren appends the has-many inserts (refund tiers referencing the
// new cancellation policy, recurring rules referencing scheduleID).
func (r *ScheduleRepository) queueScheduleChildren(tx *runtime.Tx, g *scheduleGraph, scheduleID string) {
	for i := range g.refundTiers {
		var res refundtiersql.InsertScheduleRefundTiersResponse
		tx.Add(r.svc.Mutation.Schedule.RefundTiers.CreateOp(g.refundTiers[i], &res))
	}
	for i := range g.recurringRules {
		g.recurringRules[i].ScheduleId = scheduleID
		var res recurringrulesql.InsertScheduleRecurringRulesResponse
		tx.Add(r.svc.Mutation.Schedule.RecurringRules.CreateOp(g.recurringRules[i], &res))
	}
}

// queueScheduleChildDeletes appends deletes for a schedule's superseded recurring
// rules and its now-unreferenced belongs-to rows. The old policy's refund tiers
// cascade in the DB when the policy is deleted.
func queueScheduleChildDeletes(tx *runtime.Tx, r *ScheduleRepository, refs scheduleRefs) {
	for _, id := range refs.recurringIDs {
		var res recurringrulesql.DeleteScheduleRecurringRulesByIdResponse
		tx.Add(r.svc.Mutation.Schedule.RecurringRules.DeleteOp(id, &res))
	}
	if refs.cancelID != nil {
		var res cancellationpoliciesql.DeleteScheduleCancellationPoliciesByIdResponse
		tx.Add(r.svc.Mutation.Schedule.CancellationPolicies.DeleteOp(*refs.cancelID, &res))
	}
	if refs.buffersID != nil {
		var res buffersettingsql.DeleteScheduleBufferSettingsByIdResponse
		tx.Add(r.svc.Mutation.Schedule.BufferSettings.DeleteOp(*refs.buffersID, &res))
	}
	if refs.stayID != nil {
		var res stayconstraintsql.DeleteScheduleStayConstraintsByIdResponse
		tx.Add(r.svc.Mutation.Schedule.StayConstraints.DeleteOp(*refs.stayID, &res))
	}
}
