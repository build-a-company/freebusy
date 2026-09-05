// Refund math shared by cancellation and preview.
package hasura

import (
	"context"
	"fmt"
	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/bookingql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/service/dbutil"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/refundtiersql"
	schedresourceql "github.com/oh-tarnished/freebusy/internal/database/hasura/freebusyql/scheduleql/resourceql"
	"github.com/oh-tarnished/freebusy/internal/types"
	"google.golang.org/genproto/googleapis/type/money"
)

// computeRefund resolves the unit's cancellation policy (from its schedule) and
// returns the refund percent, amount, and a human summary for the booking's lead
// time. No matching tier (or no policy) means non-refundable.
func (r *BookingRepository) computeRefund(ctx context.Context, res *resourceql.BookingResource) (int32, *money.Money, string, error) {
	if res.TotalId == nil {
		return 0, nil, "non-refundable", nil
	}
	total, err := r.money(ctx, *res.TotalId)
	if err != nil {
		return 0, nil, "", err
	}
	unit, err := r.svc.Query.Property.Units.Get(ctx, res.Unit)
	if err != nil {
		return 0, nil, "", dbutil.MapHasuraErr(err)
	}
	if unit == nil {
		return 0, nil, "non-refundable", nil
	}
	scheduleName, err := types.ScheduleName(unit.PropertyId, res.Unit)
	if err != nil {
		return 0, nil, "", err
	}
	sched, err := r.svc.Query.Schedule.Resource.Find(ctx, schedresourceql.List().Where(schedresourceql.Name.Eq(scheduleName)))
	if err != nil {
		return 0, nil, "", dbutil.MapHasuraErr(err)
	}
	if sched == nil || sched.CancellationPolicyId == nil {
		return 0, nil, "non-refundable (no cancellation policy)", nil
	}
	tiers, err := r.svc.Query.Schedule.RefundTiers.List(ctx,
		refundtiersql.List().Where(refundtiersql.CancellationPolicyId.Eq(*sched.CancellationPolicyId)))
	if err != nil {
		return 0, nil, "", dbutil.MapHasuraErr(err)
	}
	if len(tiers) == 0 {
		return 0, nil, "non-refundable", nil
	}

	var lead time.Duration
	if res.WindowId != "" {
		if w, werr := r.svc.Query.Shared.TimeWindows.Get(ctx, res.WindowId); werr == nil && w != nil {
			if st := strToTS(w.StartTime); st != nil {
				lead = time.Until(st.AsTime())
			}
		}
	}
	// The satisfied tier with the largest cutoff wins (cancelled at least cutoff
	// before the booking start).
	var bestPct int32
	bestCutoff := time.Duration(-1)
	for i := range tiers {
		cutoff, perr := time.ParseDuration(tiers[i].Cutoff)
		if perr != nil {
			continue
		}
		if lead >= cutoff && cutoff > bestCutoff {
			bestCutoff = cutoff
			bestPct = tiers[i].RefundPercent
		}
	}
	return bestPct, moneyPct(total, bestPct), fmt.Sprintf("%d%% refund for the applicable tier", bestPct), nil
}

// moneyPct returns pct percent of m.
func moneyPct(m *money.Money, pct int32) *money.Money {
	if m == nil {
		return nil
	}
	total := (m.GetUnits()*1_000_000_000 + int64(m.GetNanos())) * int64(pct) / 100
	return &money.Money{CurrencyCode: m.GetCurrencyCode(), Units: total / 1_000_000_000, Nanos: int32(total % 1_000_000_000)}
}

// moneySub returns a − b (used to split a total into refundable / retained).
func moneySub(a, b *money.Money) *money.Money {
	if a == nil {
		return nil
	}
	if b == nil {
		return a
	}
	total := (a.GetUnits()*1_000_000_000 + int64(a.GetNanos())) - (b.GetUnits()*1_000_000_000 + int64(b.GetNanos()))
	return &money.Money{CurrencyCode: a.GetCurrencyCode(), Units: total / 1_000_000_000, Nanos: int32(total % 1_000_000_000)}
}
