package lang

import (
	"fmt"
	"testing"
)

// OpCallUserPoly pins (voxgig zero-compile failures Stage 2): a gradual-Any arg to a
// MULTI-overload user fn used to decline at compile ("ambiguous dispatch, no
// poly re-match"). Now every same-arity overload's body compiles to its own
// unit and the VM re-runs the interpreter's MatchSignature at entry to pick
// the arm — the sound user-fn mirror of OpCallNativePoly. These pins are
// RunCompiled-differential regressions: the langspec differential is blind to
// off-corpus shapes (see the soundness-gate discipline), so each dispatch
// direction is pinned explicitly.

func userPolySound(t *testing.T, src string) {
	t.Helper()
	a, _ := New()
	want, werr := a.RunInterp(src)
	b, _ := New()
	got, _, gerr := b.RunCompiled(src)
	if noteCompileDefect(t, src, got, gerr) {
		return
	}
	if (werr == nil) != (gerr == nil) {
		t.Fatalf("error disagreement (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, werr, gerr)
	}
	if werr == nil && fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("SILENT MISCOMPILE (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, want, got)
	}
}

func userPolyCompiles(t *testing.T, src string) {
	t.Helper()
	userPolySound(t, src)
	a, _ := New()
	if _, reason, _, err := a.CompileCheck(src); err != nil || reason != "" {
		t.Fatalf("expected the shape to force-compile, got compile failure: %v / %q\n  src: %s", err, reason, src)
	}
}

// The 2-overload fn with a laundered (gradual-Any) arg: the runtime value is
// an Integer, so the re-match must pick the Integer arm — the interpreted
// result — and the shape must force-compile (no compile failure).
func TestUserPolyIntegerArmCompiles(t *testing.T) {
	userPolyCompiles(t, `def g fn [[a:Integer] [String] ["i"] [a:String] [String] ["s"]]
def m (flex {k:5})
def x (m get "k")
(g x)`)
}

// The sibling direction: the SAME fn, a String runtime value → the OTHER arm.
// This is exactly the case the old single-committed-overload guard got wrong
// (the checker committed one arm; the interpreter dispatched the sibling).
func TestUserPolyStringArmCompiles(t *testing.T) {
	userPolyCompiles(t, `def g fn [[a:Integer] [String] ["i"] [a:String] [String] ["s"]]
def m (flex {k:"str"})
def x (m get "k")
(g x)`)
}

// No-match: a List matches neither arm — BOTH surfaces must raise (the VM's
// no-match bails, and the compiled run reports it,
// which raises the canonical signature_error).
func TestUserPolyNoMatchErrorAgreement(t *testing.T) {
	userPolySound(t, `def g fn [[a:Integer] [String] ["i"] [a:String] [String] ["s"]]
def m (flex {k:[1 2]})
def x (m get "k")
(g x)`)
}

// The decision.boru apply-op shape: a GENERIC 2-sig def (a Comparable-bounded
// 3-arg overload plus a 3-arg Any catch-all) merged with a separate 2-arg
// def, called with record-field Any args. All three runtime directions —
// the generic arm, the catch-all arm (which raises), and per-op relational
// dispatch — must match the interpreter.
func TestUserPolyGenericArmsSound(t *testing.T) {
	srcFor := func(rec string) string {
		return `def Comparable surface {cmp: (fnsig [[Self Self] [Integer]])}
Integer exposes Comparable
String exposes Comparable
def apop gen [(T extends Comparable)] fn [[rhs:T op:String lhs:T] [Boolean] [if (op "eq" eq) [lhs rhs eq] [if (op "lt" eq) [lhs rhs lt] [raise unknown_op op]]] [rhs:Any op:String lhs:Any] [Boolean] [if (op "eq" eq) [lhs rhs eq] [raise not_comparable op]]]
def c (flex ` + rec + `)
((c get "l") (c get "o") (c get "v") apop)`
	}
	for _, rec := range []string{
		`{l:3 o:"lt" v:5}`,     // generic Comparable arm, lt → true
		`{l:9 o:"lt" v:5}`,     // generic Comparable arm, lt → false
		`{l:3 o:"eq" v:3}`,     // eq on comparables
		`{l:[1] o:"lt" v:[2]}`, // non-Comparable → catch-all arm → not_comparable raise
	} {
		userPolySound(t, srcFor(rec))
	}
	// The happy relational direction must not just agree — it must
	// force-compile (this is the decision.boru apply-op leaf).
	userPolyCompiles(t, srcFor(`{l:3 o:"lt" v:5}`))
}

// REDEFINITION drift: the first call recorded against the 2-sig table; by
// run time the table carries 4 sigs (the second def shadows), so the VM's
// Impl-identity drift guard fires and the whole program defers to the
// interpreter — results must match the interpreter exactly (the newest def
// wins for the second call).
func TestUserPolyRedefinitionDriftStaysSound(t *testing.T) {
	userPolySound(t, `def g fn [[a:Integer] [String] ["i"] [a:String] [String] ["s"]]
def m (flex {k:5})
def x (m get "k")
def r1 (g x)
def g fn [[a:Integer] [String] ["i2"] [a:String] [String] ["s2"]]
def r2 (g x)
[r1 r2]`)
}

// A BODY-LOCAL multi-overload fn is popped before the VM runs, so the poly
// path declines it (the runtime Lookup could never resolve the word) and the
// interpreter keeps owning the shape — result parity via fallback.
func TestUserPolyBodyLocalFnStaysSound(t *testing.T) {
	userPolySound(t, `def myout fn [[c:Integer] [String] [
  def g fn [[a:Integer] [String] ["i"] [a:String] [String] ["s"]]
  def m (flex {k:5})
  def x (m get "k")
  (g x)
]]
(myout 1)`)
}

// A MIXED-arity word must not confuse the poly set: the 2-arg def is a
// separate overload the arity-3 call never re-matches against, and the 2-arg
// call site is a plain single-reachable dispatch.
func TestUserPolyMixedArityStaysSound(t *testing.T) {
	userPolySound(t, `def h fn [[a:Integer b:Integer] [String] ["ii"] [a:String b:Integer] [String] ["si"]]
def h fn [[a:Integer] [String] ["one"]]
def m (flex {k:7})
def x (m get "k")
(x 1 h)`)
}
