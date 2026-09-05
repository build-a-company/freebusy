// Package dbutil holds the small helpers shared by the hand-written Hasura
// repository code across domains — the pieces with no generated (repox)
// equivalent. Anything that grows a generated counterpart should move there
// and leave this package smaller.
package dbutil

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/oh-tarnished/freebusy/internal/types"
	"github.com/the-protobuf-project/runtime-go/network/graphql"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MapHasuraErr translates the GraphQL client's storage errors onto the shared
// sentinels: an optimistic-concurrency conflict keeps its meaning, and
// unique/duplicate constraint messages surface as AlreadyExists. Everything
// else passes through unchanged.
func MapHasuraErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, graphql.ErrConflict):
		return types.ErrConflict
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate") {
		return types.ErrAlreadyExists
	}
	return err
}

// TsToStr renders a timestamp in the RFC3339 UTC form the Hasura timestamptz
// scalar expects; nil renders empty (callers null it via NullableStr).
func TsToStr(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format(time.RFC3339)
}

// NullableStr wraps a string for a nullable GraphQL input field: empty maps to
// null, anything else to its value.
func NullableStr(s string) graphql.Nullable[string] {
	if s == "" {
		return graphql.Null[string]()
	}
	return graphql.Value(s)
}

// BigdecimalToFloat reads a graphql.Bigdecimal — an arbitrary-precision decimal
// carried as its textual form — into a float64.
//
// The scalar became a string type when the generated client moved to
// protoc-gen-store 1.5.x, so that an engine returning "12.5" keeps full
// precision on the wire instead of being narrowed during JSON decode. The
// values it guards here are tax percentages, which are small and fixed-point,
// so float64 is lossless for them; anything monetary must not round-trip
// through this.
//
// An unparseable or empty value yields 0 rather than an error: a malformed
// percentage should not fail a whole read, and 0 is the safe reading — it
// charges nothing rather than charging an arbitrary amount.
func BigdecimalToFloat(v graphql.Bigdecimal) float64 {
	f, err := strconv.ParseFloat(string(v), 64)
	if err != nil {
		return 0
	}
	return f
}

// FloatToBigdecimal renders a float64 as a graphql.Bigdecimal, the inverse of
// BigdecimalToFloat. 'f' with -1 precision emits the shortest representation
// that round-trips, so 12.5 stays "12.5" rather than "12.500000".
func FloatToBigdecimal(f float64) graphql.Bigdecimal {
	return graphql.Bigdecimal(strconv.FormatFloat(f, 'f', -1, 64))
}
