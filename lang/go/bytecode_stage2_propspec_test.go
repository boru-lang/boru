package lang

import "testing"

// Stage-2 property-test pins (voxgig zero-compile failures plan): the boru:test
// PROPERTY surface — `Test.prop NAME [gen] [property]` builds a PropertySpec as
// DATA, and `Test.run-property` dispatches the Go driver `Test.check-prop`.
// Every `_prop_spec` corpus file declined at `code-body word each (Stage 2)`,
// whose masked inner leaf was `Test.check-prop` itself.
//
// The compile failure chain and the fix:
//   1. `Test.check-prop` is a NoEvalArgs code-body word whose generator/property
//      bodies arrive DYNAMIC on the declarative surface (`p get "gen"` /
//      `p get "property"` inside run-property). The NoEvalArgs code-body compile failure
//      fires because a dynamic body is not an inert const. BUT runCheckProp runs
//      BOTH bodies through parent.CallBoru in fresh ISOLATED frames against the
//      module-captured registry — the SAME Go handler under interpreter and VM,
//      binding only each body's own params (gen: `r`; property: one unnamed).
//      Name resolution inside a body never touches a compiled frame local, so a
//      plain CALL_NATIVE bake is sound. Declared via the new CompileEffect flag
//      CompileRunsBodyIsolated on test-check-prop; the code-body compile failure exempts
//      the inert-scoped disjunct for such a word.
//   2. With the bodies routed, the six operands (name/gen/property/runs/seed/
//      max-shrinks) are still DYNAMIC — record-field reads over run-property's
//      `p:PropertySpec` param (typed via the record schema). Every operand's
//      CONCRETE type conforms to test-check-prop's single sig, so the match is
//      PROVEN, not a gradual-Any widening (which shows as a bare-Any operand).
//      dynInputsProven admits it, and the declared-Map return rides as a dynamic
//      (declared-Any) output the same way dynOutNativeOK admits for concrete-arg
//      builtins.
//
// run-property's param was retyped `p:Map` -> `p:PropertySpec` so the schema
// makes each `get` concrete-typed; the plain map Test.prop builds conforms to
// the PropertySpec refine-Record at the param contract the VM enforces.
//
// The langspec differential is blind to these off-corpus shapes, so pin them.

// The declarative surface end to end: Test.prop builds a spec, `specs each`
// runs it via Test.run-property (the exact corpus shape). The gen body uses the
// seeded rand instance (`r.int`) — which is NEVER compiled: it runs inside
// runCheckProp's CallBoru, so the dot-method dispatch is an interpreter concern,
// not a compile leaf. Must force-compile clean AND match the interpreter.
func TestPropSpecDeclarativeSurfaceCompiles(t *testing.T) {
	stage1aCompiles(t, `import "boru:test" end
def specs [
  (Test.prop "ge1" [ (r.int 1 6) ] [ var [[x] (x gte 1) ] ])
]
def _ (specs each [ var [[s] (s Test.run-property end) 0 ] ])
Test.fail-count end`)
}

// Multiple specs in the list, each running the full 100-iteration budget with a
// seeded PRNG: the per-spec/per-iteration fail bookkeeping must be identical on
// both surfaces (fail-count 0 across all).
func TestPropSpecMultiSpecParity(t *testing.T) {
	stage1aCompiles(t, `import "boru:test" end
def specs [
  (Test.prop "ge1"  [ (r.int 1 6) ]           [ var [[x] (x gte 1) ] ])
  (Test.prop "str"  [ r.string "abc" 4 ]      [ var [[s] ((s size) lte 4) ] ])
  (Test.prop "one"  [ [10 20 30] r.one-of ]   [ var [[n] (n gte 10) ] ])
]
def _ (specs each [ var [[s] (s Test.run-property end) 0 ] ])
Test.fail-count end`)
}

// A property that FAILS on every input: the failing-input record, the ok=false
// flag, and Test.fail-count must be byte-identical on both surfaces (the report
// path the corpus files exercise). Uses a low, deterministic seed so the first
// failing iteration replays identically.
func TestPropSpecFailingPropertyParity(t *testing.T) {
	stage1aCompiles(t, `import "boru:test" end
def specs [
  (Test.prop "always-fails" [ (r.int 1 6) ] [ var [[x] (x gt 100) ] ])
]
def _ (specs each [ var [[s]
  def res (s Test.run-property end)
  (res "ok" get) print
  (res "failing-input" get) print
  0
] ])
Test.fail-count end`)
}

// The IMPERATIVE surface with LITERAL bodies (the `_prop_test` sibling shape):
// `Test.check-prop` called directly with inline gen/property list literals and
// explicit run/seed/shrink counts. The bodies are inert consts here, so this
// path already bakes — pinned to guard it against the new dynamic-body handling.
func TestCheckPropImperativeLiteralCompiles(t *testing.T) {
	stage1aCompiles(t, `import "boru:test" end
def res (Test.check-prop "ge0" [ (r.int 0 9) ] [ var [[x] (x gte 0) ] ] 20 1 0)
(res "ok" get)`)
}

// NEGATIVE: the generator/property arrive as bare-Any operands (a wrapper fn
// with Any params), so the match to test-check-prop's List sig positions is a
// gradual-Any WIDENING, not a proven type. dynInputsProven must REJECT it (a
// bare-Any operand is exactly the unproven case the dynamic-input guard
// defends), so the program falls back — and must stay result-identical on both
// surfaces (no miscompile from the exemption over-broadening).
func TestCheckPropBareAnyBodiesStaySound(t *testing.T) {
	stage1aSound(t, `import "boru:test" end
def wrap fn [[g:Any pr:Any] [Map] [ Test.check-prop "n" g pr 5 1 0 ]]
def res (wrap [ (r.int 1 3) ] [ var [[x] (x gte 1) ] ])
(res "ok" get)`)
}
