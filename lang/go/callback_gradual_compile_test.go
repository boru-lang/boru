package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Lambda callbacks over GRADUAL collections compile and agree with the
// interpreter. kg's `var [[…]]` bodies became lambdas (design/IMMUTABLE-DEF.1.md
// §5 phase 1) and two compiler defects surfaced on the strict lane:
//
//   - a CAPTURING lambda literal whose closure path declines (the collection
//     is gradual, so lambdaCallbackInputs has no element type) takes the
//     dyn-body backstop as the fn VALUE it is (recordDynBodyCall →
//     tryReturnedClosure), where it declined "dynamic input at each" and its
//     quotation twin compiled;
//   - a COUNT-AMBIGUOUS poly (`set` over a gradual accumulator: a Store's 0
//     results against a Map's 1) is recorded as a runtime-variable call with no
//     count claim (check.PolyNOutAmbiguous), where the VM deferred on the drift
//     (vm:poly-nout-drift) — an internal error on the strict lane.
func TestLambdaOverGradualCollectionCompiles(t *testing.T) {
	const ga = `def ga fn [[m:Map][Any][m get "a"]] end `
	for _, c := range []struct{ src, want string }{
		{ga + `def f fn [[raw:Map][List][def row 5 each ([c] => [def k c k add row]) (ga raw)]] end f {a:[1]}`, "[[6]]"},
		{ga + `def f fn [[raw:Map][List][def row 5 each ([c] => [each ([d] => [d add row]) (ga c)]) (ga raw)]] end f {a:[{a:[1]}]}`, "[[[6]]]"},
		{ga + `def g ([c acc] => [acc set (c) 1]) end def f fn [[raw:Map][Map][fold g/v (ga raw) {}]] end f {a:["x"]}`, "[{x:1}]"},
		{ga + `def f fn [[raw:Map][Map][def row 5 fold ([c acc] => [acc set (c) row]) (ga raw) {}]] end f {a:["x"]}`, "[{x:5}]"},
		// kg's `sindex`: a lambda-bodied fold building a map, read afterwards.
		{`def sindex (fold ([s acc] => [acc set (s.id) s]) [{id:"a" n:1}] {}) end (sindex get "a") get "n"`, "[1]"},
	} {
		gotC, errC := mustNew(t).RunCompiledStrict(c.src)
		if errC != nil {
			t.Errorf("%s\n  strict compiled run: %v", c.src, errC)
			continue
		}
		gotI, errI := mustNew(t).RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s\n  compiled %v, interp %v / %v, want %s", c.src, gotC, gotI, errI, c.want)
		}
	}
	// The count the committed arm did not claim: a Store receiver's `set`
	// returns nothing, and the lambda's own RET raises the interpreter's
	// count error on both lanes.
	bad := ga + `def g ([c acc] => [acc set (c) 1]) end def f fn [[raw:Map][Any][fold g/v (ga raw) (context)]] end f {a:["x"]}`
	_, _, errC := mustNew(t).RunCompiled(bad)
	_, errI := mustNew(t).RunInterp(bad)
	for lane, err := range map[string]error{"compiled": errC, "interp": errI} {
		if err == nil || !strings.Contains(err.Error(), "expected 1 return value(s), got 0") {
			t.Errorf("%s lane: err = %v, want the lambda's return-count error", lane, err)
		}
	}
	// A fixed-arity consumer of the ambiguous result keeps parity however the
	// dispatch lowers (the variadic mark declines the fixed seat).
	fixed := ga + `def g ([c acc] => [(acc set (c) 1) size]) end def f fn [[raw:Map][Any][fold g/v (ga raw) {}]] end f {a:["x"]}`
	gotC, _, errC := mustNew(t).RunCompiled(fixed)
	gotI, errI := mustNew(t).RunInterp(fixed)
	if errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[1]" {
		t.Errorf("fixed consumer: compiled %v / %v, interp %v / %v, want [1]", gotC, errC, gotI, errI)
	}
}
