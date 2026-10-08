package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR076BehaveMakeIsVisibleToCheck pins NUR076's close: a `behave make`
// call is seen by the check pass, so a type whose own constructor ignores its
// source is not validated against a schema that constructor never runs. `make
// P {bogus: 1}` after the call builds Class/P{a:42} on both lanes and checks
// clean — it used to fail `boru check` with two schema errors, which the
// default pre-flight turned into a refusal of a program that runs, and the
// compiled lane declined it at the same check. The pass notes the slot; it
// installs nothing, so no user body runs during analysis.
func TestNUR076BehaveMakeIsVisibleToCheck(t *testing.T) {
	const cls = `def P class {a: Integer} end `
	const mk = `behave make/q (fn Any P [make P {a: 42}]) end `
	src := cls + mk + `make P {bogus: 1}`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	cr, err := a.Check(src)
	if err != nil || cr.Summary.Errors != 0 {
		t.Errorf("%s: check %v, %d error(s): %v", src, err, cr.Summary.Errors, cr.Diagnostics)
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[Class/P{a:42}]" {
		t.Errorf("%s: interpreter %v / %v", src, gotI, errI)
	}
	if !compiled || errC != nil || fmt.Sprint(gotC) != "[Class/P{a:42}]" {
		t.Errorf("%s: compiled %v / %v (compiled=%v)", src, gotC, errC, compiled)
	}
	// Negatives: a construction BEFORE the call validates, as the run —
	// which raises there — has it; a `behave` of another slot gives the type
	// no constructor; a type without one still validates.
	for _, neg := range []string{
		cls + `make P {bogus: 1} ` + mk,
		cls + `behave compare/q (fn [[P P] [Integer] [0]]) end make P {bogus: 1}`,
		cls + `make P {bogus: 1}`,
		// A fn the pass cannot see through (a parameter's carrier) notes
		// nothing — and the body that would install it never runs here.
		cls + `def inst fn [[g:Function] [Any] [behave make/q g]] end make P {bogus: 1}`,
		// A call the handler refuses notes nothing either.
		cls + `behave make/q (fn Any Integer [1]) end make P {bogus: 1}`,
	} {
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		cr, err := b.Check(neg)
		if err != nil || cr.Summary.Errors == 0 {
			t.Errorf("%s: check must still flag the schema, got %v / %v", neg, err, cr.Diagnostics)
		}
		_, _, _, _, errI := runBothEngines(t, neg)
		if errI == nil || (!strings.Contains(errI.Error(), `unknown field "bogus"`) && !strings.Contains(errI.Error(), "cannot install on builtin type")) {
			t.Errorf("%s: the run raises, got %v", neg, errI)
		}
	}
}
