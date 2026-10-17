package lang

import (
	"strings"
	"testing"
)

// Dead-arm suppression: when a guard can never hold (or never fail) for the
// analysed arg shape, the unreachable arm's dispatch errors are suppressed —
// the runtime never runs that arm. A LIVE arm's errors are NOT suppressed.
func TestCheckDeadArmSuppression(t *testing.T) {
	// complement-dead: v is concretely a Map, so the ELSE arm (v is not-Map)
	// is unreachable; its `convert String v` never analysed for errors.
	deadElse := `def js fn [[v:Any] [Any] [
  if (v is Map) ["[object Object]"] [convert String v]
]]
js {a: 1}`
	if hasErr, codes := checkDiags(t, deadElse); hasErr {
		t.Errorf("dead else-arm flagged: %s\n%q", codes, deadElse)
	}
	// guard-dead: v is concretely a flex List, so the THEN arm (v is Map) is
	// unreachable; its `keys v` never analysed.
	deadThen := `def cl fn [[v:Any] [Any] [
  if (v is Map) [keys v] [v]
]]
cl (flex [1 2])`
	if hasErr, codes := checkDiags(t, deadThen); hasErr {
		t.Errorf("dead then-arm flagged: %s\n%q", codes, deadThen)
	}
	// NEGATIVE: a LIVE then-arm's error is still reported. v:Map, the guard
	// holds, so `keys 5` (keys over an Integer) is a real, reachable error.
	live := `def bad fn [[v:Map] [Any] [
  if (v is Map) [keys 5] [v]
]]
bad {a: 1}`
	if hasErr, _ := checkDiags(t, live); !hasErr {
		t.Errorf("live-arm error suppressed: %q", live)
	}
}

// The guard MEET keeps a validate-then-call clean even when nested over a
// dynamic binding: x:Any narrows to Integer, then to Big, so the Big-param
// call checks clean.
func TestGuardNarrowingMeetKeepsValidateThenCall(t *testing.T) {
	src := `def Big (Integer gt 10)
def g fn [[n:Big] [Integer] [n]]
def f fn [[x:Any] [Any] [
  if (x is Integer) [
    if (x is Big) [g x] [0]
  ] [0]
]]
f 50`
	if hasErr, codes := checkDiags(t, src); hasErr {
		t.Errorf("nested-meet validate-then-call flagged: %s\n%q", codes, src)
	}
}

// A forward-referenced fn name (defined later in the source) leaks into a
// per-call-site analysis as an Undefined Atom placeholder; it gradualizes to a
// dynamic Any carrier so the forward-ref call does not cascade a false
// no_signature. A genuine typo still raises undefined_word.
func TestCheckForwardRefPlaceholderIsDynamic(t *testing.T) {
	fwd := `def driver fn [[n:Integer] [Any] [
  def r (make-list n)
  r
]]
def make-list fn [[n:Integer] [List] [flex [n]]]
driver 3`
	if hasErr, codes := checkDiags(t, fwd); hasErr {
		t.Errorf("forward-ref call flagged: %s\n%q", codes, fwd)
	}
	// NEGATIVE: a real typo (never defined) is still an undefined_word error.
	typo := `def bad fn [[n:Integer] [Any] [no-such-word n]]
bad 3`
	if _, codes := checkDiags(t, typo); !strings.Contains(codes, "undefined_word") {
		t.Errorf("typo not flagged undefined_word: %s", codes)
	}
}

// Predicate-refinement guard narrowing via a 2-token `pred x` guard: a user
// predicate whose body is an `x is T` test (shape A: `x is T`; shape B:
// `if (x is T) [_] [false]`) narrows x to T in the THEN branch only.
func TestPredicateFnGuardNarrowing(t *testing.T) {
	// Shape A — `x is T`. The narrowing is clean.
	shapeA := `def my-islist fn [[x:Any] [Boolean] [x is List]]
def go2 fn [[x:Any] [Any] [if (my-islist x) [unshift 9 x] [x]]]
go2 [1 2]`
	if hasErr, codes := checkDiags(t, shapeA); hasErr {
		t.Errorf("shape-A predicate guard flagged: %s\n%q", codes, shapeA)
	}
	// Shape B — `if (x is T) [_] [false]`. Also clean.
	shapeB := `def p-ismap fn [[x:Any] [Boolean] [if (x is Map) [true] [false]]]
def use fn [[a:Any b:Any] [Any] [if (p-ismap a) [keys a] [{}]]]
use {x: 1} 0`
	if hasErr, codes := checkDiags(t, shapeB); hasErr {
		t.Errorf("shape-B predicate guard flagged: %s\n%q", codes, shapeB)
	}
	// Shape A discharges a real refinement: with a concrete Integer arg the
	// valid predicate narrows a to List so `unshift 9 a` checks clean.
	discharge := `def is-list fn [[x:Any] [Boolean] [x is List]]
def use fn [[a:Any b:Any] [Any] [if (is-list a) [unshift 9 a] [a]]]
use 5 [1]`
	if hasErr, codes := checkDiags(t, discharge); hasErr {
		t.Errorf("valid predicate did not narrow (flagged): %s\n%q", codes, discharge)
	}

	// NEGATIVE: a predicate whose else-arm is NOT `false` is not an is-T test,
	// so it narrows nothing — the concrete-Integer `a` reaches `unshift` and
	// the real type error survives.
	nonFalseElse := `def maybe fn [[x:Any] [Boolean] [if (x is List) [true] [true]]]
def use fn [[a:Any b:Any] [Any] [if (maybe a) [unshift 9 a] [a]]]
use 5 [1]`
	if hasErr, _ := checkDiags(t, nonFalseElse); !hasErr {
		t.Errorf("non-false-else predicate wrongly narrowed (error suppressed): %q", nonFalseElse)
	}

	// NEGATIVE: a compound OR condition licenses no per-clause narrowing, so
	// the concrete-Integer `a` reaches `unshift` and the error survives.
	orCond := `def use3 fn [[a:Any b:Any] [Any] [if ((a is List) or (b is List)) [unshift 9 a] [a]]]
use3 5 [1]`
	if hasErr, _ := checkDiags(t, orCond); !hasErr {
		t.Errorf("or-cond wrongly narrowed (error suppressed): %q", orCond)
	}

	// NEGATIVE: the ThenOnly predicate fact does NOT narrow the ELSE branch —
	// the else-branch use of x stays its original type and checks clean.
	thenOnly := `def strict-map fn [[x:Any] [Boolean] [x is Map]]
def use2 fn [[x:Any] [Any] [if (strict-map x) [keys x] [x]]]
use2 {a: 1}`
	if hasErr, codes := checkDiags(t, thenOnly); hasErr {
		t.Errorf("ThenOnly complement flagged: %s\n%q", codes, thenOnly)
	}
}
