package lang

import (
	"fmt"
	"strings"
	"testing"
)

// A `var`-body inside a higher-order code body (`each [var [[i] … ]]`) compiles
// to its closure unit even when the body REFERENCES AN ENCLOSING BINDING — a fn
// param/local (a real capture) or a module-global. Both forms used to decline
// "code-body word each (Stage 2)": the `var` splice's cleanup `undef i` ran with
// the body's residual still on the stack, and in check mode that dynamic-Any
// residual gradually matched the 2-arg `undef name fnUndefSpec` overload's
// TFnUndef slot — so the cleanup mis-dispatched, errored, and (a) declined the
// body and (b) leaked `i` so the next analysis pass wrongly captured it. Routing
// the cleanup through the dedicated 1-arg-only `__varundef` removes the ambiguity.
func TestVarBodyCaptureCompiles(t *testing.T) {
	positives := []struct {
		name, src string
	}{
		// Decline point 1: a var-body referencing a MODULE-GLOBAL.
		{"global ref in var body", `def xs [10 20 30] ([0 1 2] each [var [[i] xs i dot end]])`},
		// Decline point 2: a var-body CAPTURING an enclosing fn local (the bloom shape).
		{"capture in var body", `def f fn [[xs:List] [List] [ ([0 1 2] each [var [[i] xs i dot end]]) ]] ([10 20 30] f)`},
		{"capture, reach into element", `def data [{a:1} {a:2}] (data each [var [[s] (s dot a/q)]])`},
		// Decline point 3: a loop body capturing an enclosing COMPUTED def (not a
		// param). The captured value carries a producedBy entry from the parent
		// frame; the body must read its own capture SLOT (resolveOperand capID),
		// and the parent must PROMOTE the computed def to a frame local so the
		// closureCaps operand is a re-pushable local carrying the right VALUE
		// (forEachOperand / promoteOperand closureCaps handling). With only the
		// read-side this miscompiled (`i mul a` read the wrong slot) — so this is
		// the explicit compiled==interpreter guard the langspec differential is
		// blind to (the shape isn't in the corpus). The Bloom.indices-for / merge
		// shape: a derived local read inside the per-element loop.
		{"capture COMPUTED enclosing def in loop body", `def f fn [[k:Integer h:Integer] [List] [ def a (h add 1)  iota k each [var [[i] i mul a]] ]] (3 5 f)`},
		{"capture two computed defs in loop body", `def f fn [[k:Integer h:Integer] [List] [ def a (h add 1)  def b (h mul 2)  iota k each [var [[i] (i mul a) add b]] ]] (3 5 f)`},
	}
	for _, c := range positives {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("expected the var-body to compile, got compile failure: %v", err)
			}
			b, _ := New()
			want, werr := b.RunInterp(c.src)
			if werr != nil {
				t.Fatalf("interpreter errored on a positive case: %v", werr)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("compiled %v != interpreter %v", got, want)
			}
		})
	}

	// NEGATIVE: the cleanup actually UNBINDS the loop variable — a reference to
	// `i` after the each errors as an undefined word (the __varundef cleanup must
	// still unbind exactly like `undef i`; the binding must not leak). This is the
	// behaviour that, when it silently failed, leaked `i` into the capture set.
	t.Run("loop var is unbound after the body (no leak)", func(t *testing.T) {
		a, _ := New()
		if _, err := a.RunInterp(`([0 1 2] each [var [[i] i end]]) i print`); err == nil {
			t.Fatal("expected `i` to be undefined after the var body (cleanup must unbind)")
		} else if !strings.Contains(fmt.Sprint(err), "undefined") {
			t.Errorf("expected an undefined-word error for the unbound loop var, got %v", err)
		}
	})
}
