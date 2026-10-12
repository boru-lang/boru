package native

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Seam-9 coverage (W9_nativeC) for native_control.go: for/do/case/if
// handler error and check-mode arms, driven directly and through crafted
// boru programs.

func TestW9ForRangeTypeLiteral(t *testing.T) {
	r := seam5Reg(t)
	// A type-literal range is rejected (855.26,857.3).
	if _, err := forRangeHandler([]Value{NewTypeLiteral(TList), NewList(nil)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "concrete list") {
		t.Fatalf("forRangeHandler(type literal) = %v", err)
	}
}

func TestW9AsInt64OrDefault(t *testing.T) {
	// A non-Integer value returns the supplied default (1017.2,1017.12).
	if got := asInt64Or(NewString("x"), 42); got != 42 {
		t.Fatalf("asInt64Or(String, 42) = %d, want 42", got)
	}
	// Sanity: an Integer returns its value.
	if got := asInt64Or(NewInteger(7), 42); got != 7 {
		t.Fatalf("asInt64Or(7, 42) = %d, want 7", got)
	}
}

func TestW9DoMapEvalErrors(t *testing.T) {
	r := seam5Reg(t)
	// A map whose value is a nested list containing an unresolved Word
	// errors through doMapHandler → doEvalMapValue (map + list branches):
	// 290.16,292.3 / 343.18,345.5 / 329.17,331.4. Driven directly because
	// the source form would auto-eval the map before the handler runs.
	inner := NewList([]Value{NewWord("undefinedxyzcontrol")})
	om := NewOrderedMap()
	om.Set("x", inner)
	if _, err := doMapHandler([]Value{NewMap(om)}, nil, nil, r); err == nil {
		t.Fatalf("doMapHandler over a nested undefined word must error")
	}
}

func TestW9IfConstantFalseUnreachable(t *testing.T) {
	r := seam5Reg(t)
	// A 2-arg if with a constant-false condition flags the then-branch
	// unreachable in check mode (reduceStaticIf → emitUnreachableBranch).
	if _, err := seam5Check(r, `if false [1]`); err != nil {
		t.Fatalf("seam5Check(if false): %v", err)
	}
	found := false
	for _, d := range r.Check.Diagnostics {
		if strings.Contains(d.Detail, "unreachable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an unreachable-branch diagnostic, got %v", r.Check.Diagnostics)
	}
}

func TestW9IfClauseListJoins(t *testing.T) {
	r := seam5Reg(t)
	// The clause-list if form with multiple clauses joins their carrier
	// stacks in check mode (815.9,817.4, ifListReturnsFn).
	if _, err := seam5Check(r, `if [true [1] false [2]]`); err != nil {
		t.Fatalf("seam5Check(if clause-list): %v", err)
	}
}

func TestW9CaseBranchBodyError(t *testing.T) {
	r := seam5Reg(t)
	// A matched value expression (code body) that errors at runtime
	// propagates through caseHandler's sub.Run (781.17,783.4). Driven
	// directly with an unresolved Word in the value body.
	valBody := NewList([]Value{NewWord("undefinedxyzcase")})
	clauses := NewList([]Value{NewInteger(1), NewString("x")})
	if _, err := caseHandler([]Value{valBody, clauses}, nil, nil, r); err == nil {
		t.Fatalf("caseHandler with an erroring value body must error")
	}
}

// errorReturnsFn's payload-level defense: a sig-matched TList whose payload
// is not a list (constructible only through the raw seam) declines to the
// wide dynamic(Any) instead of panicking in AsList. The narrowing arms are
// pinned end-to-end in lang/go/bytecode_edge_findings_test.go §1.
func TestW9ErrorReturnsFnNonListPayload(t *testing.T) {
	r := seam5Reg(t)
	r.Check.Mode = true
	r.Check.Compiling = true
	bad := NewValueRaw(TList, core.IntPayload{N: 1})
	out := errorReturnsFn([]Value{bad, NewInteger(7)}, r)
	if len(out) != 1 || !IsDynamic(&out[0]) || !out[0].Parent.Equal(TAny) {
		t.Fatalf("non-list payload must stay dynamic(Any), got %v", out)
	}
}
