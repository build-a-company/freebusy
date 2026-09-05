package gorm

import (
	"errors"
	"testing"

	filterx "github.com/oh-tarnished/freebusy/internal/database/gorm/filterx"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/promocode"
)

// The generated PromoCodeFilterSpec + filterx.Gorm engine replace the
// hand-written order allowlist; these tests pin the same guarantees against
// the spec.
//
// Since protoc-gen-store 1.5.x the engine also appends `id ASC` as a
// tiebreaker. That is a total-order guarantee, not a leak of the allowlist:
// a caller still cannot sort by id (TestOrderClauseRejectsInjection covers
// that), but every ORDER BY the engine emits ends in a unique column, so a
// page boundary falling inside a run of equal sort keys cannot repeat or skip
// a row. The two tests below pin the tiebreaker so it cannot be dropped
// silently by a future regeneration.

func TestOrderClauseAllowlisted(t *testing.T) {
	got, err := filterx.Gorm[promocode.PromoCode](promocode.PromoCodeFilterSpec).OrderClause("create_time desc, code")
	if err != nil {
		t.Fatalf("OrderClause: %v", err)
	}
	if want := "create_time DESC, code ASC, id ASC"; got != want {
		t.Fatalf("OrderClause = %q, want %q", got, want)
	}
}

func TestOrderClauseRejectsInjection(t *testing.T) {
	// Fields outside the allowlist (including SQL-injection attempts) must be
	// rejected, never passed through to the ORDER BY clause.
	eng := filterx.Gorm[promocode.PromoCode](promocode.PromoCodeFilterSpec)
	for _, in := range []string{
		"id",                 // real column, but not in the sort allowlist
		"code; DROP TABLE x", // injection via extra tokens
		"(CASE WHEN 1=1 THEN name END)",
	} {
		if _, err := eng.OrderClause(in); !errors.Is(err, filterx.ErrInvalid) {
			t.Fatalf("OrderClause(%q) err = %v, want filterx.ErrInvalid", in, err)
		}
	}
}

func TestOrderClauseEmpty(t *testing.T) {
	// No order_by still yields the tiebreaker rather than an empty clause: an
	// unordered List is exactly the case where pagination is least stable.
	got, err := filterx.Gorm[promocode.PromoCode](promocode.PromoCodeFilterSpec).OrderClause("")
	if err != nil || got != "id ASC" {
		t.Fatalf("OrderClause(\"\") = (%q, %v), want (\"id ASC\", nil)", got, err)
	}
}
