package check

// Check-piece ordering mirror extracted from compare.go (Stage 2c of the
// four-piece split): the check-mode ReturnsFn for the family-restricted
// ordering words and its static-determinacy probe.

import (
	"errors"

	core "github.com/boru-lang/boru/core/go"
)

// OrderingReturnsFn builds the check-mode ReturnsFn for the family-
// restricted ordering words (lt / gt / lte / gte / cmp). When BOTH
// operands are statically determinate it runs the (pure) handler to
// detect a cross-family pair statically — Integer vs String can never be
// ordered, for any values — and emits an [boru/incomparable] error
// diagnostic, so the mismatch is caught at check time instead of slipping
// through as a runtime error. With a non-determinate operand the runtime
// family is unknown, so nothing is flagged. The result carrier (Boolean
// for the predicates, Integer for cmp) is returned either way so analysis
// continues.
func OrderingReturnsFn(handler core.Handler, result *core.Type) core.ReturnsFunc {
	return func(args []core.Value, r *core.Registry) []core.Value {
		if r != nil && len(args) == 2 && orderingDeterminate(args[0]) && orderingDeterminate(args[1]) {
			if _, err := handler(args, nil, nil, r); err != nil {
				var ae *core.BoruError
				if errors.As(err, &ae) && ae.Code == "incomparable" {
					// Routed through the unique-diagnostic helper for the
					// caught-body gate: inside `do [...]` the runtime error
					// is trapped, so the static mirror stays silent there.
					core.CheckAddUniqueDiagnostic(r, "incomparable", ae.Detail, "", args[0].Pos())
				}
			}
		}
		return []core.Value{core.NewCarrier(result)}
	}
}

// orderingDeterminate reports whether v's ordering family is fixed at
// check time, so the ordering handler may be run on it to detect a
// cross-family pair soundly. A concrete value qualifies (its family is
// its own). `none` also qualifies even though it is non-concrete
// (Data==nil): None is a SINGLETON type, so a non-dynamic none-shaped
// value is determinately `none` at runtime — running the handler on it
// yields exactly the runtime result (no false positive). A bare type
// NODE qualifies for the same reason — the literal IS the runtime
// operand (`List lt Map`, `5 cmp Scalar`), so the handler's verdict
// (litVsLit Rank order within a family, incomparable across) is exactly
// the runtime's. A dynamic / gradual carrier never qualifies: its
// family is genuinely unknown.
func orderingDeterminate(v core.Value) bool {
	if core.IsDynamic(&v) {
		return false
	}
	return core.IsConcrete(&v) || core.IsNoneShape(v) || core.IsBareTypeNode(&v)
}
