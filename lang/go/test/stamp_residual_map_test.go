package test

import "testing"

// TestStampTrailingComputedMapNested pins the widened in-frame residual
// recording (mini-s3's s3-parse-range wall): a MULTI-TOKEN fn body ending
// in a computed container (`{from: from upto: upto}` over body-locals)
// records its OpMakeMap assembly, so a CALLER fn's unit that compiles the
// callee nested no longer declines "body result of unknown provenance".
// Sound because the interpreter evaluates a multi-token body's trailing
// computed container IN-frame on every dispatch path (CallBoru-class and
// same-registry spliced, consumed and unconsumed alike).
func TestStampTrailingComputedMapNested(t *testing.T) {
	evs := stampEventsFor(t, `
module [
  def pr fn [[rv:String total:Integer] [Any] [
    def from (convert Integer rv)
    def upto (if (from lt total) [ total sub 1 ] [ from ])
    {from: from upto: upto}
  ]]
  def use-pr fn [[rv:String] [Any] [
    def r (pr rv 10)
    r.from add r.upto
  ]]
  export "PrT" { pr: pr/v use: use-pr/v }
] import
`)
	for _, name := range []string{"pr", "use-pr"} {
		if reason, ok := stampOutcome(evs, name); !ok {
			t.Errorf("%s did not stamp: %s", name, reason)
		}
	}
}

// TestStampTrailingMapParity runs the nested shape end-to-end: the stamped
// caller must produce the interpreter's in-frame values.
func TestStampTrailingMapParity(t *testing.T) {
	out := runStampedModule(t, `
module [
  def pr fn [[rv:String total:Integer] [Any] [
    def from (convert Integer rv)
    def upto (if (from lt total) [ total sub 1 ] [ from ])
    {from: from upto: upto}
  ]]
  def use-pr fn [[rv:String] [Any] [
    def r (pr rv 10)
    r.from add r.upto
  ]]
  export "PrT" { use: use-pr/v }
] import
`, `PrT.use "3"`)
	if len(out) != 1 {
		t.Fatalf("use-pr result stack = %v, want one value", out)
	}
	n, err := asIntegerVal(out[0])
	if err != nil || n != 12 {
		t.Fatalf("use-pr = %v (%v), want 12 (from 3 + upto 9)", out[0], err)
	}
}

// TestSingleLiteralBodyTransparencyHolds pins the single-literal boundary
// of the in-frame unification: a fn whose WHOLE body is one literal
// container keeps its PRE-EXISTING context semantics, untouched by the
// multi-token change. The top-level spliced direction (the no-closures
// transparency — the returned container resolves the MODULE binding) is
// spec-pinned in def-node-binding.tsv §3; here the CallBoru-class module
// call is pinned: the container evaluates at the callee sub-run's end,
// IN-frame, so the PARAM wins. (The whole-program compiler declines this
// shape — "body result of unknown provenance" — because a single unit
// cannot serve both directions; the detached runtime stamp may serve the
// CallBoru-only module path, where in-frame matches.)
func TestSingleLiteralBodyTransparencyHolds(t *testing.T) {
	out := runStampedModule(t, `
module [
  def c1 1
  def mk fn [[c1:Integer] [Map] [{a: c1}]]
  export "TrT" { mk: mk/v }
] import
`, `def r (TrT.mk 9) r.a`)
	if len(out) != 1 {
		t.Fatalf("mk result stack = %v, want one value", out)
	}
	n, err := asIntegerVal(out[0])
	if err != nil || n != 9 {
		t.Fatalf("module-path mk 9 → .a = %v (%v), want the in-frame param 9 (CallBoru direction)", out[0], err)
	}
}
