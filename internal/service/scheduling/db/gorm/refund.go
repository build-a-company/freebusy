// Refund math shared by cancellation and preview.
package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/booking"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/common"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/property"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/schedule"
	"github.com/oh-tarnished/freebusy/internal/types"
	"google.golang.org/genproto/googleapis/type/money"
	"gorm.io/gorm"
)

// computeRefund resolves the unit's cancellation policy (from its schedule) and
// returns the refund percent, amount, and a human summary for the booking's lead
// time. No matching tier (or no policy) means non-refundable.
func (r *BookingRepository) computeRefund(ctx context.Context, tx *gorm.DB, m *booking.Booking) (int32, *money.Money, string, error) {
	total := common.MoneyToProto(m.Total)
	if total == nil {
		return 0, nil, "non-refundable", nil
	}
	var unit property.Unit
	if err := tx.WithContext(ctx).Select("id", "property_id").First(&unit, "id = ?", m.UnitID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil, "non-refundable", nil
		}
		return 0, nil, "", err
	}
	scheduleName, err := types.ScheduleName(unit.PropertyID, m.UnitID)
	if err != nil {
		return 0, nil, "", err
	}
	var sched schedule.Schedule
	switch err := tx.WithContext(ctx).Preload("CancellationPolicy.RefundTiers").First(&sched, "name = ?", scheduleName).Error; {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return 0, nil, "non-refundable (no cancellation policy)", nil
	case err != nil:
		return 0, nil, "", err
	}
	if sched.CancellationPolicy == nil || len(sched.CancellationPolicy.RefundTiers) == 0 {
		return 0, nil, "non-refundable", nil
	}

	var lead time.Duration
	if m.Window != nil {
		lead = time.Until(m.Window.StartTime)
	}
	// The satisfied tier with the largest cutoff wins (cancelled at least cutoff
	// before the booking start).
	var bestPct int32
	bestCutoff := time.Duration(-1)
	for i := range sched.CancellationPolicy.RefundTiers {
		cutoff, perr := time.ParseDuration(sched.CancellationPolicy.RefundTiers[i].Cutoff)
		if perr != nil {
			continue
		}
		if lead >= cutoff && cutoff > bestCutoff {
			bestCutoff = cutoff
			bestPct = sched.CancellationPolicy.RefundTiers[i].RefundPercent
		}
	}
	return bestPct, moneyPct(total, bestPct), fmt.Sprintf("%d%% refund for the applicable tier", bestPct), nil
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
