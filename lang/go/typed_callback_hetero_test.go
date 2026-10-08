package lang

import (
	"fmt"
	"testing"
)

// typed_callback_hetero_test.go pins NUR155 (2026-09-17, FIXED 2026-09-23):
// a TYPED lambda callback over a HETEROGENEOUS collection is applied only to
// the elements its signature admits — the interpreter's list handlers hand
// the fn VALUE to InvokeBody, which STEPS it over the inputs, and a step no
// signature admits leaves the value as DATA on top of them (the element's
// result is the fn itself); the map arm's lambda no-match raises its own
// signature_error. A lambda the compiler lowers as the word's own BODY unit
// (each$body, fold$body — the lambda's contract recorded, not a fn VALUE
// unit) was entered for every element regardless: `each ([x:Integer] =>
// [typeof x]) [1 'a' [2] {b:1} true none]` compiled `[Integer ProperString
// List Map Boolean None]` for `[Integer fn fn fn fn fn]`, and the map fold
// `0 fold ([acc:Integer kv:KeyVal] => [acc add kv.v]) {a:"s" b:1}` compiled
// `[0s1]` where the interpreter raises at key b. The VM's token seam now
// matches such a unit's contract per element (unmatchedLambdaBody,
// eng/go/vm.go): a list body's no-match hands back the inputs with the
// closure on top, rendered as the interpreter renders the lambda; a map
// body's raises the map arm's error.

// TestTypedCallbackHeteroParity: every row compiles and agrees with the
// interpreter.
func TestTypedCallbackHeteroParity(t *testing.T) {
	rows := []struct{ src, want, note string }{
		{`each ([x:Integer] => [typeof x]) [1 'a' [2] {b:1} true none]`, "[[Integer fn (Integer) fn (Integer) fn (Integer) fn (Integer) fn (Integer)]]", "each-variants.tsv:L216, the corpus row"},
		{`each ([x:Integer] => [typeof x]) [[1] none]`, "[[fn (Integer) fn (Integer)]]", "no admitted element at all"},
		{`each ([x:Integer] => [typeof x]) [[1] {b:1}]`, "[[fn (Integer) fn (Integer)]]", "a list and a map"},
		{`each ([x:Integer] => [x add 1]) [1 none 3]`, "[[2 fn (Integer) 4]]", "the admitted elements run, the rest carry the fn"},
		{`0 fold ([a:Integer x:Integer] => [a add x]) [1 none]`, "[fn (Integer, Integer)]", "a list fold's no-match leaves the fn as the accumulator"},
		{`0 fold ([a:Integer x:Integer] => [a add x]) [1 [2]]`, "[fn (Integer, Integer)]", "the same over a list element"},
		{`scan ([a:Integer x:Integer] => [a add x]) [1 'a' 2]`, "[[1 fn (Integer, Integer) fn (Integer, Integer)]]", "scan (the stored-fn route, as before)"},
		{`each ([x:Integer] => [typeof x]) [1 'a']`, "[[Integer fn (Integer)]]", "the stored-fn route agreed already"},
		{`each ([x:Integer] => [typeof x]) [1]`, "[[Integer]]", "a homogeneous list runs the unit"},
		{`each ([kv:KeyVal] => [kv.v]) {a:1 b:none}`, "[{a:1 b:none}]", "a map body's KeyVal contract admits every entry"},
		{`0 fold ([acc:Integer kv:KeyVal] => [acc add kv.v]) {a:1 b:2}`, "[3]", "a map fold whose accumulator stays admitted"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
}

// TestTypedCallbackHeteroErrorParity: the map arm's lambda no-match raises
// the same signature_error on both lanes — the accumulator a String after
// the first entry, the lambda's `acc:Integer` rejecting it at the second.
func TestTypedCallbackHeteroErrorParity(t *testing.T) {
	rows := []struct{ src, code, note string }{
		{`0 fold ([acc:Integer kv:KeyVal] => [acc add kv.v]) {a:"s" b:1}`, "signature_error", "a map fold body's no-match (compiled [0s1] on main)"},
		{`each ([x:Integer] => [x add 1]) {a:1 b:"s"}`, "signature_error", "a typed map lambda over a non-KeyVal param (the stored-fn route)"},
		{`filter ([x:Integer] => [x gt 1]) [1 none 3]`, "signature_error", "filter's own no-match (as before)"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if codeOf(errI) != c.code || codeOf(errC) != c.code {
			t.Errorf("%q: interp [%s] compiled [%s], want %s on both (%s)", c.src, codeOf(errI), codeOf(errC), c.code, c.note)
		}
	}
}
