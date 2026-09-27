package lang

import "testing"

// TestNUR290TypedDefOverADynamicBody pins NUR290's close. A typed def whose
// body the check pass holds as a carrier — `def x:Integer (mk)` over mk's
// Any — unified the carrier with the annotation, and Unify takes the
// narrower side: the annotation's own type content, which the def bound. The
// compiled run bound the TYPE (`x` answered `[Integer]` where the interpreter
// answers 42, and `[String]` where it refuses "s"); a membership unifier that
// cannot inspect a carrier (a fn shape, a negation) admits it instead, and
// the run bound its value unchecked. Unless the carrier's own type proves
// membership, the run decides it now: the pass binds a carrier and records
// OpBindTyped over TypedBindRunMembership, which binds what the
// interpreter's unify binds or raises its refusal, byte-identically. An Any
// annotation is the untyped def.
func TestNUR290TypedDefOverADynamicBody(t *testing.T) {
	const pre = `def T (Integer tor String) end def P {a:Integer} end def Big (Integer gt 10) end `
	const mk = `def mk fn [[][Any][42]] end `
	const ms = `def ms fn [[][Any]["s"]] end `
	for _, c := range []struct{ src, want string }{
		{mk + `def x:Integer (mk) end x`, "[42]"},
		{mk + `def x:Integer (mk) end x add 1`, "[43]"},
		{mk + `def x:Number (mk) end x typeof`, "[Integer]"},
		{mk + `def x:Scalar (mk) end [x x]`, "[[42 42]]"},
		{ms + `def x:Integer (ms) end x`, "ERROR:def x: value 's' does not unify with declared type Integer"},
		{mk + `def x:String (mk) end x`, "ERROR:def x: value 42 does not unify with declared type String"},
		{mk + `def x:Any (mk) end x`, "[42]"},
		{ms + `def x:Any (ms) end x`, "[s]"},
		{pre + mk + `def x:T (mk) end x`, "[42]"},
		{pre + `def ml fn [[][Any][[1 2]]] end def x:T (ml) end x`, "ERROR:def x: value [1 2] does not unify with declared type T"},
		{pre + mk + `def x:(Integer tor String) (mk) end x`, "[42]"},
		{pre + mk + `def x:P (mk) end x`, "ERROR:def x: value 42 does not unify with declared type P"},
		{pre + `def mm fn [[][Any][{a:1}]] end def x:P (mm) end x`, "[{a:1}]"},
		{pre + mk + `def x:Big (mk) end x`, "[42]"},
		{pre + `def m5 fn [[][Any][5]] end def x:Big (m5) end x`, "ERROR:def x: value 5 does not unify with declared type Big"},
		{pre + `def g fn [[][Integer][42]] end def x:T (g) end x`, "[42]"},
		{`def g fn [[][Integer][42]] end def x:Number (g) end x`, "[42]"},
		{`def g fn [[][Number][2.5]] end def x:Integer (g) end x`, "ERROR:def x: value 2.5 does not unify with declared type Integer"},
		{`def g fn [[][Scalar]["s"]] end def x:String (g) end x`, "[s]"},
		{`def f fn [[p:Any][Any][def x:Integer p end x]] end f 5`, "[5]"},
		{`def f fn [[m:Map][Any][def x:List m.k end x]] end f {k:[1 2]}`, "[[1 2]]"},
		{`def f fn [[m:Map][Any][def x:List m.k end x]] end f {k:5}`, "ERROR:def x: value 5 does not unify with declared type List"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// The check pass alone binds the same carrier: no false refusal
	// downstream of the def, which bound the type before.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{
		mk + `def x:Integer (mk) end x add 1`,
		mk + `def x:Scalar (mk) end x`,
		pre + mk + `def x:T (mk) end x`,
	} {
		if errs := errorDiags(t, a, src); len(errs) != 0 {
			t.Errorf("%s: no error diagnostic, got %v", src, errs)
		}
	}
	// A value that may be a fn, under an annotation a fn may inhabit,
	// declines: the binding's read applies the fn, which the read models see
	// only through the body's own carrier.
	for _, src := range []string{
		`def mk fn [[][Any][([y:Integer] => [y])]] end def x:Function (mk) end x typeof`,
		`def M (fnsig [[Integer] [Integer]]) end def mk fn [[][Any][42]] end def x:M (mk) end x typeof`,
		`def mk fn [[][Any][42]] end def x:Type (mk) end x`,
		`def mk fn [[][Any][42]] end def x:(tnot String) (mk) end x`,
	} {
		if gotC, compiled, errC, _, _ := runBothEngines(t, src); compiled {
			t.Errorf("%s: declines, got %v %v", src, gotC, errC)
		}
	}
}
