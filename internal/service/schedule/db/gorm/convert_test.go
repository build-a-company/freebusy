package gorm

import (
	"testing"
	"time"

	"github.com/oh-tarnished/freebusy/protobuf/generated/go/schedule/v1/schedulepbv1"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/shared/v1/sharedpbv1"
	"google.golang.org/protobuf/types/known/durationpb"
)

// roundTripSchedule materializes a proto Schedule into its row graph, wires the
// preloaded associations back onto the schedule model, and re-hydrates the proto.
func roundTripSchedule(in *schedulepbv1.Schedule) *schedulepbv1.Schedule {
	g := buildScheduleGraph(in)
	g.schedule.ID = "s1"
	g.schedule.Name = in.GetName()
	g.schedule.Buffers = g.buffers
	g.schedule.StayConstraints = g.stayConstraints
	if g.cancellationPolicy != nil {
		for _, t := range g.refundTiers {
			g.cancellationPolicy.RefundTiers = append(g.cancellationPolicy.RefundTiers, *t)
		}
		g.schedule.CancellationPolicy = g.cancellationPolicy
	}
	for _, r := range g.recurringRules {
		g.schedule.RecurringRules = append(g.schedule.RecurringRules, *r)
	}
	return scheduleFromModel(g.schedule)
}

func TestScheduleRoundTrip(t *testing.T) {
	in := &schedulepbv1.Schedule{
		Name: "properties/p1/units/u1/schedule",
		RecurringRules: []*schedulepbv1.RecurringRule{
			{Rrule: "FREQ=WEEKLY;BYDAY=MO,TU", Opens: "09:00", Closes: "17:00"},
		},
		Buffers: &schedulepbv1.BufferSettings{
			StartDelta: durationpb.New(15 * time.Minute),
			EndDelta:   durationpb.New(30 * time.Minute),
			MinNotice:  durationpb.New(2 * time.Hour),
		},
		StayConstraints: &schedulepbv1.StayConstraints{
			MinNights:       2,
			MaxNights:       14,
			CheckinWeekdays: []sharedpbv1.Weekday{sharedpbv1.Weekday_WEEKDAY_FRIDAY, sharedpbv1.Weekday_WEEKDAY_SATURDAY},
			AdvanceMaxDays:  365,
		},
		CancellationPolicy: &schedulepbv1.CancellationPolicy{
			Tiers: []*schedulepbv1.RefundTier{
				{Cutoff: durationpb.New(48 * time.Hour), RefundPercent: 100},
				{Cutoff: durationpb.New(24 * time.Hour), RefundPercent: 50},
			},
		},
	}
	out := roundTripSchedule(in)

	if rr := out.GetRecurringRules(); len(rr) != 1 || rr[0].GetRrule() != "FREQ=WEEKLY;BYDAY=MO,TU" || rr[0].GetOpens() != "09:00" {
		t.Fatalf("recurring rules not preserved: %+v", rr)
	}
	if b := out.GetBuffers(); b.GetStartDelta().AsDuration() != 15*time.Minute || b.GetMinNotice().AsDuration() != 2*time.Hour {
		t.Fatalf("buffers not preserved: %+v", b)
	}
	if sc := out.GetStayConstraints(); sc.GetMinNights() != 2 || sc.GetMaxNights() != 14 || len(sc.GetCheckinWeekdays()) != 2 || sc.GetAdvanceMaxDays() != 365 {
		t.Fatalf("stay constraints not preserved: %+v", sc)
	}
	if cp := out.GetCancellationPolicy(); len(cp.GetTiers()) != 2 ||
		cp.GetTiers()[0].GetCutoff().AsDuration() != 48*time.Hour || cp.GetTiers()[0].GetRefundPercent() != 100 {
		t.Fatalf("cancellation policy not preserved: %+v", cp)
	}
}
