package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Pins for the fn-internal `do {…}` computed-map body (decision.boru's
// evaluator idiom — `{ok:false error:…}` result maps): the dyn-do body
// residual is runtime-counted, which used to mark the whole fn
// VARIADIC-returning, so any fixed-arity consumer of its call declined
// "consumes loop results". A DECLARED return tuple now overrides the
// marking — the VM RET enforces the declared count at runtime exactly
// where the interpreter raises — so the call seats the declared shape.
func TestFnDoMapBodyCompiles(t *testing.T) {
	// The call result feeds a FIXED-ARITY consumer (`get`) — the shape
	// that used to decline (a program-residual consumer absorbs variadics
	// and never tripped it).
	mustCompileWithParity(t, `
def make-point fn [[x:Integer y:Integer] [Map] [
  do {x:[x] y:[y] dist2:[(x x mul) (y y mul) add]}
]]
((make-point 3 4) get "dist2")`, "[25]")

	mustCompileWithParity(t, `
def no-match fn [[reason:String] [Map] [
  do {ok:[false] error:[reason]}
]]
((no-match "no-rule") get "error")`, "[no-rule]")
}

// TestFnDoMapCountMismatchParity pins the negative that makes the
// override sound: a declared-[Map] fn whose dyn-do body nets TWO values
// at runtime raises the identical return-count type_error on the VM and
// the interpreter — the declared contract is enforced, not assumed.
func TestFnDoMapCountMismatchParity(t *testing.T) {
	const src = `def bad fn [[b:Any] [Map] [ do b ]] (bad [1 2])`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("expected a native compile, declined: reason=%q err=%v", reason, cerr)
	}
	b, _ := New()
	_, compiled, errC := b.RunCompiled(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	if !compiled {
		t.Fatal("expected a compiled run")
	}
	c, _ := New()
	_, errI := c.RunInterp(src)
	if errC == nil || errI == nil {
		t.Fatalf("both runs must raise: compiled=%v interp=%v", errC, errI)
	}
	const want = "expected 1 return value(s), got 2"
	if !strings.Contains(fmt.Sprint(errC), want) || !strings.Contains(fmt.Sprint(errI), want) {
		t.Errorf("return-count error parity: compiled=%v interp=%v, both must say %q", errC, errI, want)
	}
}

// TestDoMapValueEvalInBranchArm pins the value-eval `do {map}` fix: a
// CONCRETE, non-dynamic Map value-eval body always yields exactly one value
// (the assembled map), so it must NOT be marked a variadic loop result. Before
// the fix, a do-map in a BRANCH ARM (`if c [do {a:1}] [do {b:2}]`) propagated a
// spurious variadic mark up through the branch merge to the enclosing fn, so any
// fixed-arity consumer of the fn's result declined "consumes loop results" — even
// though the VM runs the branch correctly. The decision library's eval-table-*
// helpers (`if … [do {ok:false error:"no-match"}] …`) are exactly this shape.
func TestDoMapValueEvalInBranchArm(t *testing.T) {
	// A do-map arm result consumed by a FIXED-ARITY word (get) — the shape
	// that used to decline.
	mustCompileWithParity(t,
		`def pickmap fn [[c:Boolean] [Map] [if c [do {a:1}] [do {b:2}]]] ((pickmap true) get "a")`, "[1]")
	// One do-arm, one literal-map arm.
	mustCompileWithParity(t,
		`def pickmap fn [[c:Boolean] [Map] [if c [do {a:1}] [{b:2}]]] ((pickmap false) get "b")`, "[2]")
	// The decision eval-table-unique tail shape: nested `if` whose arms are
	// do-map error results, consumed by a fixed-arity word.
	mustCompileWithParity(t,
		`def f fn [[n:Integer] [Map] [if (n gt 0) [do {ok:[true]}] [if (n eq 0) [do {ok:[false] error:["z"]}] [do {ok:[false] error:["neg"]}]]]] ((f 0) get "error")`,
		"[z]")
}

// TestDoMapValueEvalNoDynEnv pins that a value-eval `do {map}` does NOT arm the
// program-wide DynEnv mirror (its handler runs no dynamic-name-resolving code
// body). Before the fix, arming DynEnv forced every unrelated `def` in the
// program to a registry-visible OpBindDynScope twin, declining a sibling fn whose
// binding has no compiled home (`def found None` → "dynamic-scope def `found` of
// unknown provenance") — the decision eval-tree/find-node shape.
func TestDoMapValueEvalNoDynEnv(t *testing.T) {
	// A `do {map}` and a separate loop-with-`def x None` in the same program:
	// the loop def must not be forced dyn-scope by the do-map.
	mustCompileWithParity(t, `
def findn fn [[k:Integer ns:List] [Any] [
  def found None
  for (ns size) [ def i2 i def nd (ns i2 get) if ((nd get "id") k eq) [def found nd] [] ] end
  found
]]
def mk fn [[] [Map] [do {ok:[true]}]]
def _ (mk)
((findn 2 [{id:1} {id:2}]) get "id")`, "[2]")
}
