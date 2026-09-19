package lang

import (
	"fmt"
	"testing"
)

// Off-corpus regressions for record-typed-param schema recovery — the foundation
// for force-compiling record-data-driven code (design plan: plan-the-boru-test-
// work). A record-typed param (`c:SomeRecord`) is reparented at the call boundary
// to a carrier that carries the record's RecordTypeInfo schema, so a body
// `c get "field"` recovers the field's declared type (gradually) instead of
// degrading to Any. The langspec differential is blind to these shapes, so they
// are pinned here as RunCompiled==Run (compile MUST equal interpret).

func recSound(t *testing.T, src string) {
	t.Helper()
	a, _ := New()
	want, werr := a.RunInterp(src)
	b, _ := New()
	got, _, gerr := b.RunCompiled(src)
	if noteCompileDefect(t, src, got, gerr) {
		return
	}
	if (werr == nil) != (gerr == nil) {
		t.Fatalf("error disagreement (compile != interpret):\n  src: %s\n  interp: %v\n  compiled: %v", src, werr, gerr)
	}
	if werr == nil && fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("SILENT MISCOMPILE (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, want, got)
	}
}

// A record-typed param whose body reads fields: the field types are recovered
// from the schema; compile == interpret.
func TestRecordParam_FieldRecoverySound(t *testing.T) {
	const src = `def C refine Record [x:Integer label:String]
		def f fn [[c:C] [Scalar] [ (c get "x") add 1 ]]
		def ok {x:5 label:"a"}
		ok f`
	recSound(t, src)
}

// The record param is CAPTURED into a closure body (the boru:test `test-describe`
// shape): the schema carrier must survive capture so the captured field read
// still recovers its type. compile == interpret either way.
func TestRecordParam_CapturedFieldRecoverySound(t *testing.T) {
	const src = `def C refine Record [x:Integer]
		def f fn [[c:C] [List] [ iota 2 each [ var [[i] (c get "x") add i ] ] ]]
		def ok {x:10}
		ok f`
	recSound(t, src)
}

// NEGATIVE: a value whose field type does not conform is rejected at the param
// match and never reaches the body — interpret and compile agree (both error or
// both fall back). A regression that wrongly admitted it would diverge.
func TestRecordParam_NonConformingStaysSound(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			name: "wrong-field-type",
			src: `def C refine Record [x:Integer]
				def f fn [[c:C] [Scalar] [ (c get "x") add 1 ]]
				def bad {x:"nope"}
				bad f`,
		},
		{
			// a plain (untyped) Map arg flowing into the record param
			name: "plain-map-arg",
			src: `def C refine Record [x:Integer]
				def f fn [[c:C] [Scalar] [ (c get "x") add 1 ]]
				def m {x:7}
				def m2 (m set "x" 8)
				m2 f`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { recSound(t, c.src) })
	}
}

// A non-record map-shape param (no record def) must behave as before — the
// reparent only applies where a schema pattern exists; untyped Map params keep
// gradual Any. Pinned as soundness.
func TestRecordParam_UntypedMapUnchangedSound(t *testing.T) {
	const src = `def f fn [[m:Map] [Scalar] [ (m get "x") add 1 ]]
		def v {x:3}
		v f`
	recSound(t, src)
}
