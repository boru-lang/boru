package lang

import (
	"fmt"
	"strings"
	"testing"
)

// A fn unit whose value-def promotion is planned BEFORE a later dispatch arms
// DynEnv left its dyn-bound COMPUTED defs on the single-consume sim stack, and
// Finalize then lowered the unit widened and declined "dynamic-scope def `x` of
// unpromoted computed value" — the blocker across the voxgig bloom-filter
// unit/smoke suites, where `Bloom.make`'s make-bloom (`def k-val (derive-k …)`)
// is compiled at its call site and a later `Bloom.params` `do {n:[bf.n] …}`
// map body — opaque-typed, so it takes the dynamic backstop — arms DynEnv.
// promoteLateDynBind seats those sources late.
//
// Each case pairs a computed-def-bearing fn (compiled FIRST) with a later
// `do {…}` over a MIXED-type class instance (Integer + Float fields → an
// opaque map result that arms DynEnv via tryRecordDynBody). The differential
// is BLIND to this arming order, so each is pinned RunCompiledStrict == Run and
// asserted to compile with no FALLBACK island.
func TestLateDynEnvArmingPromotesComputedDefs(t *testing.T) {
	strict := []struct {
		name, src, want string
	}{
		// The reduced make-bloom shape: mk binds a USER-call computed def `kv`
		// used as a make field (like `k-val (derive-k …)`), is CALLED first
		// (plan-time DynEnv false), THEN `params`' mixed-type `do {…}` arms
		// DynEnv, so mk is lowered widened and its `kv` must be seated late.
		{"user-call computed def as make-field, mixed do-map arms dynenv after",
			`def Box class { n:Integer p:Float }
def g fn [[x:Integer] [Integer] [x mul 3]]
def mk fn [[x:Integer] [Box] [ def kv (g x) make Box {n: kv p: 0.5} ]]
def a (mk 5)
def params fn [[b:Box] [Map] [ do {n: [b.n], p: [b.p]} ]]
(a params)`, `[{n:15 p:0.5}]`},
		// Two chained computed defs both feeding the make (like m-val→k-val):
		// the first is multi-referenced (promoted at plan time), the second
		// single-use (seated late).
		{"chained computed defs into make, second seated late",
			`def Box class { n:Integer p:Float }
def g fn [[x:Integer] [Integer] [x mul 2]]
def h fn [[x:Integer] [Integer] [x add 1]]
def mk fn [[x:Integer] [Box] [ def m (g x) def k (h m) make Box {n: (m add k) p: 0.5} ]]
def a (mk 5)
def params fn [[b:Box] [Map] [ do {n: [b.n], p: [b.p]} ]]
(a params)`, `[{n:21 p:0.5}]`},
		// A computed def inside an if-arm (a fragment-internal dyn-bound source,
		// as in make-bloom's bad-input guards), seated to a unit frame local
		// with its store emitted inside the arm.
		{"computed def inside an if-arm, mixed do-map arms dynenv after",
			`def Box class { n:Integer p:Float }
def g fn [[x:Integer] [Integer] [x mul 3]]
def mk fn [[x:Integer] [Integer] [
  if (x lt 0) [ def lo (g x) lo ] [ def kv (g x) kv add 1 ]
]]
def a (mk 5)
def bx (make Box {n: 1 p: 0.5})
def params fn [[b:Box] [Map] [ do {n: [b.n], p: [b.p]} ]]
def _ (bx params)
a`, `[16]`},
		// A BRANCH-valued dyn-bound source (`def v (if … [g x] [0])`) whose two
		// arms EACH net one value: planBranchPromotion seats the branch merge to a
		// unit frame local and the late OpBindDynScope re-pushes it, byte-identical
		// to the interpreter. (Contrast the VARIADIC source in the negatives below,
		// whose empty arm nets 0 at runtime and genuinely must decline.)
		{"branch-valued dyn-bind source, both arms single-value, seated late",
			`def Box class { n:Integer p:Float }
def g fn [[x:Integer] [Integer] [x mul 3]]
def mk fn [[x:Integer] [Box] [ def v (if (x gt 0) [ g x ] [ 0 ]) make Box {n: v p: 0.5} ]]
def a (mk 5)
def params fn [[b:Box] [Map] [ do {n: [b.n], p: [b.p]} ]]
(a params)`, `[{n:15 p:0.5}]`},
	}
	for _, c := range strict {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile natively, declined: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("%s must compile native (no island)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, werr := b.RunInterp(c.src)
			if werr != nil {
				t.Fatalf("interpreter errored: %v", werr)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}

	// A dyn-bound source the late-arming promotion CANNOT seat is left untouched
	// and declines at lowerDynBind, a compile failure (never a wrong
	// store). RunCompiledStrict must surface the compile failure, and the interpreter
	// must still produce the value.
	negatives := []struct{ name, src, want string }{
		// A VARIADIC-returning source (`maybe = if … [x] []`) has static nout==1
		// but leaves 0 values at runtime for the empty arm. Promoting it emitted
		// a lone STORE_LOCAL that underflowed the VM (compile != interpret — the
		// `mk -1` crash Codex flagged on boru-lang/boru#261). It must decline and
		// fall back; the value-producing call still matches the interpreter.
		{"variadic-returning dyn-bind source declines, interp runs",
			`def Box class { n:Integer p:Float }
def maybe fn [[x:Integer] [] [ if (x gt 0) [ x ] [ ] ]]
def mk fn [[x:Integer] [Box] [ def v (maybe x) make Box {n: v p: 0.5} ]]
def a (mk 5)
def params fn [[b:Box] [Map] [ do {n: [b.n], p: [b.p]} ]]
(a params)`, `[{n:5 p:0.5}]`},
	}
	for _, c := range negatives {
		t.Run("declines/"+c.name, func(t *testing.T) {
			a, _ := New()
			if _, err := a.RunCompiledStrict(c.src); err == nil {
				t.Errorf("%s: expected a force-compile failure, but it compiled", c.name)
			}
			b, _ := New()
			want, werr := b.RunInterp(c.src)
			if werr != nil {
				t.Fatalf("interpreter must run the fallback: %v", werr)
			}
			if fmt.Sprint(want) != c.want {
				t.Errorf("interpreter got %v, want %s", want, c.want)
			}
		})
	}
}
