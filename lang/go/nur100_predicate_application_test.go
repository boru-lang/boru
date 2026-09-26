package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR100PredicateIsAOneValueApplication pins NUR100 §1's close: whether a
// function may act as a predicate is no longer decided by counting its
// parameters. Membership is a ONE-VALUE APPLICATION — the candidate is
// matched against the predicate's signatures by the one matcher every call
// takes (MatchFnSig), and the signature that takes it runs. So the whole
// overload set is consulted (a later overload takes what the first cannot), a
// value pattern selects as it does for any call, and a predicate none of
// whose signatures can take one value is a type no value inhabits — every
// membership question answers so, on both lanes, where the use used to raise
// "RunPredicate: predicate must take exactly one argument".
func TestNUR100PredicateIsAOneValueApplication(t *testing.T) {
	const p = `def P fnpred [[n:Integer] [n gt 0] [s:String] [(size s) gt 0]] end `
	const k = `def K fnpred [[a:Any b:Any] [a]] end `
	const k0 = `def K0 fnpred [[] [true]] end `
	const z = `def Z fnpred [[0] [true]] end `
	for _, c := range []struct{ src, want string }{
		{p + `[(5 is P) ("ab" is P) ("" is P) (0 is P)]`, "[[true true false false]]"},
		{p + `def v:P "ab" v`, "[ab]"},
		{p + `def f fn [[q:P] [Any] [q]] end f "ab"`, "[ab]"},
		{k + `4 is K`, "[false]"},
		{k0 + `4 is K0`, "[false]"},
		{z + `[(0 is Z) (1 is Z)]`, "[[true false]]"},
		{z + `def v:Z 0 v`, "[0]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
	// Negatives: what no signature takes is refused as a MEMBERSHIP failure —
	// a typed def raises "does not satisfy", a typed parameter declines the
	// dispatch — and never as an arity error, on either lane.
	for _, c := range []struct{ src, want string }{
		{k + `def v:K 5 v`, "does not satisfy predicate type K"},
		{p + `def v:P "" v`, "does not satisfy predicate type P"},
		{z + `def v:Z 1 v`, "does not satisfy predicate type Z"},
		{p + `def f fn [[q:P] [Any] [q]] end f 0`, "signature_error"},
	} {
		_, _, errC, _, errI := runBothEngines(t, c.src)
		if errI == nil || !strings.Contains(errI.Error(), c.want) {
			t.Errorf("%s: interpreter error %v, want %q", c.src, errI, c.want)
		}
		if errC == nil || !strings.Contains(errC.Error(), c.want) {
			t.Errorf("%s: compiled error %v, want %q", c.src, errC, c.want)
		}
		for _, err := range []error{errI, errC} {
			if err != nil && strings.Contains(err.Error(), "exactly one argument") {
				t.Errorf("%s: the arity gate answered: %v", c.src, err)
			}
		}
	}
}

// TestNUR223CallbackSeamDiscardsUnconsumedUnnamed pins NUR223, found closing
// NUR100: the callback seam hands its caller exactly the values the
// interpreter's CallBoru hands — residuals beyond the signature's declared
// return count that are UNCONSUMED unnamed params are discarded, up to the
// unnamed-param count. A stored fn's unit is compiled count-agnostic, so the
// compiled seam returned the param beneath the verdict and the predicate
// protocol refused the pair: `0 is Z` over `fnpred [[Integer] [true]]` was
// true interpreted and false compiled, and a typed def raised "predicate must
// return exactly one value, got 2" compiled only.
func TestNUR223CallbackSeamDiscardsUnconsumedUnnamed(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def Z fnpred [[Integer] [true]] end 0 is Z`, "[true]"},
		{`def Z fnpred [[Integer] [dup 0 eq]] end [(0 is Z) (1 is Z)]`, "[[true false]]"},
		{`def Z fnpred [[Integer] [dup 0 eq]] end def v:Z 0 v`, "[0]"},
		// Negative: a body that CONSUMES its unnamed param leaves nothing to
		// discard — the verdict is the whole residual and stays the answer.
		{`def Z fnpred [[Integer] [0 eq]] end [(0 is Z) (1 is Z)]`, "[[true false]]"},
		// …and a named param is a binding, never a residual.
		{`def Z fnpred [[n:Integer] [n 0 eq]] end [(0 is Z) (1 is Z)]`, "[[true false]]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
	// Negative: a residual SHORTER than the declared count is not trimmed into
	// a verdict it does not hold — the protocol still refuses it alike.
	src := `def Z fnpred [[Integer] [drop]] end 0 is Z`
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	requireParity(t, src, gotC, errC, gotI, errI)
}

// TestNUR224PredicateRefusalIsATypeError pins NUR224, found closing NUR100: a
// typed def's predicate refusal is a type_error on both lanes, as the typed
// def's other refusals are (`def q:T "x"` — does not unify with declared type
// T). It was a PLAIN error, which the interpreter surfaced bare while the
// compiled run, raising the same refusal from its typed-bind op over a
// runtime value, wrapped it as an internal_error annotated a compiler defect.
func TestNUR224PredicateRefusalIsATypeError(t *testing.T) {
	const big = `def Big fnpred n:Integer [n gt 10] end `
	for _, src := range []string{
		big + `def f fn [[x:Any] [Any] [def q:Big x q]] end f 5`,
		`def P fnpred [[n:Integer] [true]] end def v:P Integer v`,
	} {
		_, _, errC, _, errI := runBothEngines(t, src)
		for lane, err := range map[string]error{"interpreter": errI, "compiled": errC} {
			if err == nil || !strings.Contains(err.Error(), "[boru/type_error]") ||
				!strings.Contains(err.Error(), "does not satisfy predicate type") {
				t.Errorf("%s: %s error %v, want the type_error refusal", src, lane, err)
			}
		}
	}
	// Negative: a member binds on both lanes — the refusal is the refusal
	// alone.
	src := big + `def f fn [[x:Any] [Any] [def q:Big x q]] end f 50`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[50]" || !compiled || errC != nil || fmt.Sprint(gotC) != "[50]" {
		t.Errorf("%s: interpreter %v / %v, compiled %v / %v (compiled=%v), want [50]", src, gotI, errI, gotC, errC, compiled)
	}
}
