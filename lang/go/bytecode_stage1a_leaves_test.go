package lang

import (
	"fmt"
	"testing"
)

// Stage-1a leaf pins (voxgig zero-refusals plan): two closure/materialization
// fixes whose exact shapes the langspec differential does not cover.
//
//  1. Mutable-instance closure captures (callable_words.go
//     moduleScopeMutableCaptures): a module-scope `def acc (flex […])` (or a
//     class instance) read inside an each/fold body cannot const-bake, and per
//     the language the reference is dynamic (not a lexical capture) — the body
//     refused. It now rides as a capture: the identity is fixed for the whole
//     dispatch (a compiled body cannot rebind a module-scope name), so the
//     value pushed at OpPushClosure equals every per-run lookup.
//  2. Nested-container element materialization (engine.go elemEvalRecordable):
//     a map literal AS A LIST ELEMENT is residual-evaluated in a sub-engine
//     (consumed=false), so its OpMakeMap never recorded and a nested spec
//     literal (`[ {cases:[{in:[(make …)]}…]} … ]`) broke at the first
//     map-in-list layer. Recordability now propagates through container
//     element evals anchored at a recordable (top-level/consumed) site.

func stage1aSound(t *testing.T, src string) {
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

func stage1aCompiles(t *testing.T, src string) {
	t.Helper()
	stage1aSound(t, src)
	a, _ := New()
	if _, reason, _, err := a.CompileCheck(src); err != nil || reason != "" {
		t.Fatalf("expected the shape to force-compile, got refusal: %v / %q\n  src: %s", err, reason, src)
	}
}

// A module-scope flex accumulator mutated inside an each body — the bloom/
// stats accumulator idiom. Must compile and match the interpreter.
func TestFlexCaptureInEachBodyCompiles(t *testing.T) {
	stage1aCompiles(t, `def acc (flex [0])
def _ ([1 2 3] each [ var [[x] (acc set 0 ((acc get 0) add x)) drop 0 ] ])
(acc get 0)`)
}

// The capture must observe mutations ACROSS iterations (a const bake would
// freeze the first value) — the accumulated sum pins the shared identity.
func TestFlexCaptureSharedAcrossIterations(t *testing.T) {
	stage1aSound(t, `def acc (flex [1])
def _ ([2 3 4] each [ var [[x] (acc set 0 ((acc get 0) mul x)) drop 0 ] ])
(acc get 0)`)
}

// A nested spec-style literal: a def-bound LIST of MAPS whose inner lists
// carry class instances (the voxgig bloom_unit_spec shape, 4 levels deep).
// The whole chain must materialise (list → map → list → map → instance
// event) so the literal resolves as a higher-order word's data operand.
func TestNestedSpecLiteralMaterialises(t *testing.T) {
	stage1aCompiles(t, `def Box class { v: 0 }
def specs [
  { name: "a"
    cases: [ { in: [ (make Box {v:1}) ] out: 1 } ] }
  { name: "b"
    cases: [ { in: [ (make Box {v:2}) ] out: 2 } ] }
]
def outs (specs each [ var [[s] (((s get "cases") get 0) get "out") ] ])
outs`)
}

// NEGATIVE: a deferred residual list (a fn body RETURNING a bare-word list)
// must still refuse/fall back — the recordability propagation is scoped to
// container evals anchored at a recordable site, not end-of-run residuals
// whose frame has popped (the undefined_word divergence the consumed gate
// guards).
func TestDeferredResidualListStaysSound(t *testing.T) {
	stage1aSound(t, `def f fn [[y:Integer] [] [ [y y] ]] (f 5)`)
}

// The cross-NESTED-fragment reference shape (trie-insert recompiled under the
// unit-spec cascade): `def kid (user-call …)` produced inside the OUTER if's
// else-arm, referenced as the INNER if's arm-out. The boolean fragInternal
// conflated "same fragment" with "same-or-ancestor", so the producer was never
// promoted and the inner arm refused "branch leaves extra values" (out=opEvent,
// vm=0, fragEvents=0). crossFragRef (fragment-identity tracking in
// collectPromotableEvents) now promotes it to a unit-frame local.
func TestCrossNestedFragmentUserCallPromotes(t *testing.T) {
	stage1aCompiles(t, `def seek9 fn [[k:String m:Map] [Any] [ (m get k) ]]
def pick9 fn [
  [k:String m:Map] [Any] [
    if ((k size) eq 0) [
      "empty"
    ] [
      def kid (seek9 k m)
      def kidnode (if (kid eq none) ["fresh"] [kid])
      kidnode
    ]
  ]
]
(pick9 "a" {a: 42})`)
}

// All three runtime paths of the nested guard (found / missing / empty key).
func TestCrossNestedFragmentAllPathsSound(t *testing.T) {
	for _, call := range []string{`(pick9 "a" {a: 42})`, `(pick9 "z" {a: 42})`, `(pick9 "" {a: 42})`} {
		stage1aSound(t, `def seek9 fn [[k:String m:Map] [Any] [ (m get k) ]]
def pick9 fn [
  [k:String m:Map] [Any] [
    if ((k size) eq 0) [ "empty" ] [
      def kid (seek9 k m)
      def kidnode (if (kid eq none) ["fresh"] [kid])
      kidnode
    ]
  ]
]
`+call)
	}
}
