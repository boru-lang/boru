package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// plain_lambda_test.go pins the twenty-sixth increment's gate: a lambda VALUE
// unit takes the fn path's residual replay (fnResidualReplayReason) only
// when its params carry no value PATTERN — a pattern is matched by the
// interpreter's frame binding at the apply, which the closure apply ops do
// not enforce, so a pattern lambda keeps the closure count refusal.
func TestPlainLambda(t *testing.T) {
	zero := core.NewInteger(0)
	for _, c := range []struct {
		name string
		rec  fnUnitRec
		want bool
	}{
		{"a code body is not a lambda", fnUnitRec{closure: true}, false},
		{"a lambda with typed params", fnUnitRec{closure: true, lambdaUnit: true, paramPatterns: []*core.Value{nil, nil}}, true},
		{"a lambda with no contract seated", fnUnitRec{closure: true, lambdaUnit: true}, true},
		{"a lambda with a value-pattern param", fnUnitRec{closure: true, lambdaUnit: true, paramPatterns: []*core.Value{&zero}}, false},
	} {
		if got := c.rec.plainLambda(); got != c.want {
			t.Errorf("%s: plainLambda = %v", c.name, got)
		}
	}
	if paramSpecPatterns(nil) != nil {
		t.Error("a nil contract has no patterns")
	}
	if ps := (&ClosureParamSpec{Patterns: []*core.Value{&zero}}); len(paramSpecPatterns(ps)) != 1 {
		t.Error("a contract's patterns ride through")
	}
	// The count refusal stays for a pattern lambda and lifts for a plain one
	// (fnResidualReplayReason's gate): a closure that is no lambda returns
	// no reason at all.
	if reason := (&EmitState{}).fnResidualReplayReason(nil, &fnUnitRec{closure: true}, nil, nil, 0); reason != "" {
		t.Errorf("a code-body closure keeps its own count discipline: %q", reason)
	}
}
