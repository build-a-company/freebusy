package gorm

import (
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/schedule"
	"github.com/oh-tarnished/freebusy/internal/database/repository/repox"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/schedule/v1/schedulepbv1"
	"github.com/oh-tarnished/runtime-go/ulid"
)

// scheduleGraph is the set of rows a Schedule materializes into: the schedule
// row, its belongs-to buffers / stay-constraints / cancellation-policy (created
// before it), the policy's has-many refund tiers, and its has-many recurring
// rules (created after it, carrying the schedule_id FK).
type scheduleGraph struct {
	schedule           *schedule.Schedule
	buffers            *schedule.BufferSettings
	stayConstraints    *schedule.StayConstraints
	cancellationPolicy *schedule.CancellationPolicy
	refundTiers        []*schedule.RefundTier
	recurringRules     []*schedule.RecurringRule
}

// buildScheduleGraph turns a proto Schedule into its row graph. The schedule
// row's identity (ID/Name/Etag) and the child FKs (schedule_id on recurring
// rules, cancellation_policy_id on refund tiers) are stamped by the repository.
//
// It takes no parent id: a schedule is named "resources/{resource}/schedule",
// so the resource it belongs to is carried by the name the repository stamps.
// The row had a property_id column when schedules hung off a Property; that
// parent no longer exists.
func buildScheduleGraph(s *schedulepbv1.Schedule) *scheduleGraph {
	g := &scheduleGraph{}
	g.schedule = &schedule.Schedule{}

	if b := s.GetBuffers(); b != nil {
		g.buffers = &schedule.BufferSettings{
			ID:         ulid.GenerateString(),
			StartDelta: durationToStr(b.GetStartDelta()),
			EndDelta:   durationToStr(b.GetEndDelta()),
			MinNotice:  durationToStr(b.GetMinNotice()),
			MaxAdvance: durationToStr(b.GetMaxAdvance()),
			Gap:        durationToStr(b.GetGap()),
		}
		g.schedule.BuffersID = &g.buffers.ID
	}
	if sc := s.GetStayConstraints(); sc != nil {
		g.stayConstraints = &schedule.StayConstraints{
			ID:               ulid.GenerateString(),
			MinNights:        nilIfZeroInt32(sc.GetMinNights()),
			MaxNights:        nilIfZeroInt32(sc.GetMaxNights()),
			CheckinWeekdays:  weekdaysToStr(sc.GetCheckinWeekdays()),
			CheckoutWeekdays: weekdaysToStr(sc.GetCheckoutWeekdays()),
			AdvanceMinDays:   nilIfZeroInt32(sc.GetAdvanceMinDays()),
			AdvanceMaxDays:   nilIfZeroInt32(sc.GetAdvanceMaxDays()),
		}
		g.schedule.StayConstraintsID = &g.stayConstraints.ID
	}
	if cp := s.GetCancellationPolicy(); cp != nil {
		g.cancellationPolicy = &schedule.CancellationPolicy{ID: ulid.GenerateString()}
		g.schedule.CancellationPolicyID = &g.cancellationPolicy.ID
		for _, t := range cp.GetTiers() {
			g.refundTiers = append(g.refundTiers, &schedule.RefundTier{
				ID:            ulid.GenerateString(),
				Cutoff:        repox.Deref(durationToStr(t.GetCutoff())),
				RefundPercent: t.GetRefundPercent(),
			})
		}
	}
	for _, r := range s.GetRecurringRules() {
		g.recurringRules = append(g.recurringRules, &schedule.RecurringRule{
			ID:     ulid.GenerateString(),
			Rrule:  r.GetRrule(),
			Opens:  strOrNil(r.GetOpens()),
			Closes: strOrNil(r.GetCloses()),
		})
	}
	return g
}

// scheduleFromModel assembles the protobuf Schedule from a stored row and its
// preloaded associations. The exceptions list is populated separately by the
// repository (derived from the unit's AvailabilityException rows).
func scheduleFromModel(m *schedule.Schedule) *schedulepbv1.Schedule {
	out := &schedulepbv1.Schedule{
		Name:               m.Name,
		Buffers:            bufferFromModel(m.Buffers),
		StayConstraints:    stayFromModel(m.StayConstraints),
		CancellationPolicy: cancellationFromModel(m.CancellationPolicy),
		Etag:               repox.Deref(m.Etag),
	}
	for i := range m.RecurringRules {
		out.RecurringRules = append(out.RecurringRules, recurringFromModel(&m.RecurringRules[i]))
	}
	return out
}

func recurringFromModel(r *schedule.RecurringRule) *schedulepbv1.RecurringRule {
	return &schedulepbv1.RecurringRule{
		Rrule:  r.Rrule,
		Opens:  repox.Deref(r.Opens),
		Closes: repox.Deref(r.Closes),
	}
}

func bufferFromModel(b *schedule.BufferSettings) *schedulepbv1.BufferSettings {
	if b == nil {
		return nil
	}
	return &schedulepbv1.BufferSettings{
		StartDelta: durationFromStr(b.StartDelta),
		EndDelta:   durationFromStr(b.EndDelta),
		MinNotice:  durationFromStr(b.MinNotice),
		MaxAdvance: durationFromStr(b.MaxAdvance),
		Gap:        durationFromStr(b.Gap),
	}
}

func stayFromModel(s *schedule.StayConstraints) *schedulepbv1.StayConstraints {
	if s == nil {
		return nil
	}
	return &schedulepbv1.StayConstraints{
		MinNights:        repox.Deref(s.MinNights),
		MaxNights:        repox.Deref(s.MaxNights),
		CheckinWeekdays:  weekdaysFromStr(s.CheckinWeekdays),
		CheckoutWeekdays: weekdaysFromStr(s.CheckoutWeekdays),
		AdvanceMinDays:   repox.Deref(s.AdvanceMinDays),
		AdvanceMaxDays:   repox.Deref(s.AdvanceMaxDays),
	}
}

func cancellationFromModel(c *schedule.CancellationPolicy) *schedulepbv1.CancellationPolicy {
	if c == nil {
		return nil
	}
	out := &schedulepbv1.CancellationPolicy{}
	for i := range c.RefundTiers {
		out.Tiers = append(out.Tiers, &schedulepbv1.RefundTier{
			Cutoff:        durationFromStr(&c.RefundTiers[i].Cutoff),
			RefundPercent: c.RefundTiers[i].RefundPercent,
		})
	}
	return out
}

func nilIfZeroInt32(v int32) *int32 {
	if v == 0 {
		return nil
	}
	return &v
}
