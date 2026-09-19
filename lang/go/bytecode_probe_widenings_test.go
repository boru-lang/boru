package lang

// Three probe-verified gate widenings (the REFUSAL-CLOSURE §9.4 open-site
// sweep, 2026-07-17), each turning a refusal into a compiled shape:
//
//   - a computed range START/STEP that resolves to a frame LOCAL lowers
//     (computedRangeBounds passes bounds as-is; RecordLoop admits const +
//     local operands and keeps refusing event-produced ones — opForSetup
//     already enforces the runtime Integer/zero-step taxonomy);
//   - an ALL-0-ARG member's shaped landing skips the statement-window scan
//     (the member never forward-collects, so following tokens belong to the
//     next dispatch — `r get "bool" eq false` declined on the `eq`);
//   - `behave` carries CompileStoresFn (it stores its fn for later Behavior
//     dispatch, never re-stepping it on the tape — the log/patrun/service
//     store-fn pattern), so a capture-free comparator bakes as a const.

import "testing"

func TestProbeWideningComputedRangeStartStep(t *testing.T) {
	mustCompileWithParity(t,
		`def f fn [[n:Integer] [Integer] [def acc 0 for [n 5] [def acc (acc add i)] end acc]] (f 1)`, "[10]")
	mustCompileWithParity(t,
		`def f fn [[n:Integer] [Integer] [def acc 0 for [0 6 n] [def acc (acc add i)] end acc]] f 2`, "[6]")
	mustCompileWithParity(t,
		`def s 2 def acc 0 for [s 4] [def acc (acc add i)] end acc`, "[5]")
	// An EVENT-produced step keeps the refusal (no re-pushable home).
	mustRefuseWithParity(t,
		`def f fn [[n:Integer] [Integer] [def acc 0 for [5 1 (0 sub n)] [def acc (acc add i)] end acc]] (f 1)`,
		"computed range start/step")
	// A 4-element range is a runtime for_error in BOTH engines (parseRange
	// arity 1-3; computedRangeBounds falls through the same arity gate).
	{
		src := `for [1 2 3 4] [ 5 drop ] 0`
		a, _ := New()
		_, errI := a.RunInterp(src)
		b, _ := New()
		gotC, _, errC2 := b.RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC2) {
			return
		}
		_ = gotC
		if codeOf(errI) != "for_error" || codeOf(errC2) != "for_error" {
			t.Errorf("4-elem range: want for_error both, got interp [%s] compiled [%s]", codeOf(errI), codeOf(errC2))
		}
	}
}

func TestProbeWideningZeroArgMethodLanding(t *testing.T) {
	mustCompileWithParity(t,
		`import "boru:rand"  def r (Rand.with-seed 7)  r get "bool" eq false`, "[true]")
}

func TestProbeWideningBehaveStoresFn(t *testing.T) {
	mustCompileWithParity(t, `def Person class {age:Integer}
behave "compare" ((fn [[a:Person b:Person] [Integer] [(a get "age") (b get "age") sub]])/v)
def alice (make Person {age:30})
def bob (make Person {age:25})
alice bob lt`, "[false]")
}

// Both-computed `if` over a NON-event condition (the S9.4 probe sweep):
// lowerBothComputedMatCond materialises the condFrag / const / local cond
// above the two eager arm values (the single-computed lowerComputedCond
// shapes) and JMP_IF_FALSE selects — parens evaluate eagerly in BOTH
// engines, so selection-only lowering is the event-cond arm's identical
// semantics. Both branch directions, both cond forms.
func TestProbeWideningBothComputedNonEventCond(t *testing.T) {
	mustCompileWithParity(t,
		`def f fn [[b:Boolean x:Integer] [Integer] [if b (x add 1) (x add 2)]] f true 5`, "[6]")
	mustCompileWithParity(t,
		`def f fn [[b:Boolean x:Integer] [Integer] [if b (x add 1) (x add 2)]] f false 5`, "[7]")
	mustCompileWithParity(t,
		`def f fn [[x:Integer] [Integer] [if [x lt 9] (x add 1) (x add 2)]] f 5`, "[6]")
	mustCompileWithParity(t,
		`def f fn [[x:Integer] [Integer] [if [x lt 9] (x add 1) (x add 2)]] f 50`, "[52]")
	// The event-cond regression control.
	mustCompileWithParity(t,
		`def f fn [[x:Integer] [Integer] [if (x lt 9) (x add 1) (x add 2)]] f 5`, "[6]")
}

// A multi-overload user fn over a STRICT-DISJUNCT operand (the S9.4 probe
// sweep, union-return poly): `g (h true)` where h returns `(Integer tor
// String)` and g has an Integer arm and a String arm — every same-arity arm
// sharing the committed return bakes to OpCallUserPoly and the VM re-matches
// the concrete alternative at run time (the §6b machinery reached through
// the disjunct partition). Divergent-return arm sets keep the refusal.
func TestProbeWideningUnionReturnPoly(t *testing.T) {
	const mod = `def U (Integer tor String)
def g fn [[a:Integer] [Integer] [a add 1] [a:String] [Integer] [0]]
def h fn [[b:Boolean] [U] [if b [3] ['x']]]
`
	mustCompileWithParity(t, mod+`g (h true)`, "[4]")
	mustCompileWithParity(t, mod+`g (h false)`, "[0]")
	// DIVERGENT returns (Integer arm vs String arm) GRADUATED 2026-08-03 —
	// the §8.2(3) return-join: the poly plan records the join of the arms'
	// returns (userPolyPlan.outs) and the recorded identity survives the
	// first-match-partition widening (applyGradualContagion preserves
	// out[0].ID), so the strict-disjunct operand rides OpCallUserPoly with
	// the VM re-matching the concrete alternative at run time.
	mustCompileWithParity(t, `def U (Integer tor String)
def g fn [[a:Integer] [Integer] [a add 1] [a:String] [String] ['s']]
def h fn [[b:Boolean] [U] [if b [3] ['x']]]
g (h true)`, "[4]")
	mustCompileWithParity(t, `def U (Integer tor String)
def g fn [[a:Integer] [Integer] [a add 1] [a:String] [String] ['s']]
def h fn [[b:Boolean] [U] [if b [3] ['x']]]
g (h false)`, "[s]")
}
