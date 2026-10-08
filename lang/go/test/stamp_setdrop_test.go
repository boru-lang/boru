package test

import (
	"strings"
	"testing"
)

// TestStampSetDropOverDynamicReceiver pins the statement-position
// mixed-arity modeling (execMatch's consumed-tail lookahead +
// applyGradualContagion): `hs set (k) v` newline `drop` over a DYNAMIC
// receiver stamps without source grouping — the shape every mini-s3 /
// mini-redis store handler writes. The modeled optimistic value is
// consumed by the plain `drop` (a dynStackShuffleWords consumer that
// never forward-collects); a runtime overload returning 0 values is
// owned by callPoly's result-count claim check (interpreter fallback,
// slow-not-wrong).
func TestStampSetDropOverDynamicReceiver(t *testing.T) {
	evs := stampEventsFor(t, `
module [
  def w-any fn [[hs:Any k:String v:String] [Any] [ hs set (k) v drop 0 ]]
  def w-grouped fn [[hs:Any k:String v:String] [Any] [ (hs set (k) v) drop 0 ]]
  def w-typed fn [[hs:Map k:String v:String] [Any] [ hs set (k) v drop 0 ]]
  export "M" { w-any: w-any/v w-grouped: w-grouped/v w-typed: w-typed/v }
] import
`)
	for _, name := range []string{"w-any", "w-grouped", "w-typed"} {
		if reason, ok := stampOutcome(evs, name); !ok {
			t.Fatalf("%s did not stamp: %s", name, reason)
		}
	}
}

// TestStampSetDropNegativeShapes pins what must NOT get the optimistic
// modeling: a MODIFIED shuffle word (drop/s changes the collection shape)
// keeps the faithful 0-arity, so the unit still declines — the modeled
// phantom may only feed a PLAIN dynStackShuffleWords consumer.
func TestStampSetDropNegativeShapes(t *testing.T) {
	evs := stampEventsFor(t, `
module [
  def w-mod fn [[hs:Any k:String v:String] [Any] [ hs set (k) v drop/s 0 ]]
  export "M" { w-mod: w-mod/v }
] import
`)
	reason, ok := stampOutcome(evs, "w-mod")
	if ok {
		t.Fatal("w-mod stamped; the optimistic gradual arity must not fire for a modified shuffle word")
	}
	if strings.Contains(reason, "no stamp event") {
		t.Fatalf("w-mod missing stamp event: %s", reason)
	}
}

// TestSetDropClaimMismatchFallsBack pins the soundness owner of the
// statement-position modeling for the OTHER follower class: a
// forward-collecting word (`print`) after the set keeps the faithful
// 0-arity claim, the unit still stamps, and a value-returning runtime
// receiver (flex map: set returns the node) trips callPoly's
// result-count claim check → the invocation falls back to the
// interpreter and produces the interpreter's exact residual (the
// updated node survives, `print` consumed the trailing literal).
func TestSetDropClaimMismatchFallsBack(t *testing.T) {
	out := runStampedModule(t, `
module [
  def w-fwd fn [[hs:Any k:String v:String] [Any] [ hs set (k) v print 0 ]]
  export "M" { w-fwd: w-fwd/v }
] import
`, `def hs (flex {}) M.w-fwd hs "greeting" "hello" drop hs get "greeting"`)
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %v", out)
	}
	if s, err := asStringVal(out[0]); err != nil || s != "hello" {
		t.Fatalf("claim-mismatch fallback parity: got %v (%v), want 'hello'", out[0], err)
	}
}

// TestSetDropCompiledRuntimeParity pins end-to-end behaviour of the newly
// stamped shape: the module fn runs on the VM (stamped) with a flex-map
// receiver and produces the interpreter's exact result — the set lands,
// the receiver residual is dropped, the sentinel returns.
func TestSetDropCompiledRuntimeParity(t *testing.T) {
	out := runStampedModule(t, `
module [
  def w-any fn [[hs:Any k:String v:String] [Any] [ hs set (k) v drop 0 ]]
  export "M" { w-any: w-any/v }
] import
`, `def hs (flex {}) M.w-any hs "greeting" "hello" drop hs get "greeting"`)
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %v", out)
	}
	if s, err := asStringVal(out[0]); err != nil || s != "hello" {
		t.Fatalf("compiled set/drop parity: got %v (%v), want 'hello'", out[0], err)
	}
}
