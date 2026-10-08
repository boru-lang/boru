package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// fnshape_typed_window_test.go pins NUR096's plain-check model
// (tryFnShapeTypedWindow): a carrier typed by a fn SHAPE, re-stepped over
// its argument window, is the run's apply of the stored fn, so the window
// collapses to one carrier per DECLARED return of the shape.

// ftwShape is the one-signature spec `[[Integer] rets…]`.
func ftwShape(rets ...*core.Type) core.FnUndefInfo {
	return core.FnUndefInfo{Sigs: []core.FnSigSpec{{
		Params:  []core.FnParam{{Type: core.TInteger}},
		Returns: rets,
	}}}
}

// ftwNamed installs a named shape type under name on a plain pass and
// returns a carrier typed by it.
func ftwNamed(t *testing.T, r *core.Registry, name string, info core.FnUndefInfo) core.Value {
	t.Helper()
	if err := core.InstallType(r, name, core.NewFnUndef(info)); err != nil {
		t.Fatalf("install %s: %v", name, err)
	}
	def := r.LookupTypeName(name)
	if def == nil || !core.TypeIsFnShape(def) {
		t.Fatalf("%s must be a fn-shape type, got %v", name, def)
	}
	return core.NewCarrier(def)
}

func TestFnShapeTypedWindowNamedShape(t *testing.T) {
	r := zzmsReg(t)
	done := r.Check.Begin()
	defer done()
	carrier := ftwNamed(t, r, "Ftw2", ftwShape(core.TInteger, core.TString))
	e := zzmsEngine(r, []core.Value{carrier, core.NewInteger(10), core.NewInteger(9), core.NewEnd()})
	if !tryDynamicFnValueDispatch(e, 0) {
		t.Fatal("the plain check applies the shape over its window")
	}
	if e.Tape.Len() != 4 {
		t.Fatalf("the window becomes the shape's two returns, tape len = %d", e.Tape.Len())
	}
	for i, want := range []*core.Type{core.TInteger, core.TString} {
		if out := e.Tape.At(i); !out.Carrier || out.Dynamic || !out.Parent.Equal(want) {
			t.Errorf("return %d = %v, want a %s carrier", i, out, want.Name())
		}
	}
	if n, _ := e.Tape.At(2).AsConcreteInteger(); n != 9 {
		t.Error("the token past the arity stays on the tape")
	}
}

func TestFnShapeTypedWindowNotedClaim(t *testing.T) {
	r := zzmsReg(t)
	done := r.Check.Begin()
	defer done()
	// An anonymous shape's carrier is typed by the bare FunctionSignature
	// node; the claim noted at the read is what it stands for. An Any
	// return is a dynamic value; a shape returning nothing consumes the
	// window and leaves nothing.
	claim, ok := FnShapeOfSpec(ftwShape(core.TAny))
	if !ok {
		t.Fatal("a one-signature spec of plain params is a claim")
	}
	carrier := core.NewCarrier(core.TFnUndef)
	r.Check.NoteFnShape(carrier, claim)
	e := zzmsEngine(r, []core.Value{carrier, core.NewInteger(1), core.NewEnd()})
	if !tryDynamicFnValueDispatch(e, 0) {
		t.Fatal("the noted claim is applied")
	}
	if out := e.Tape.At(0); !out.Carrier || !out.Dynamic {
		t.Errorf("an Any return is a dynamic value, got %v", out)
	}
	void, _ := FnShapeOfSpec(ftwShape())
	quiet := core.NewCarrier(core.TFnUndef)
	r.Check.NoteFnShape(quiet, void)
	e = zzmsEngine(r, []core.Value{quiet, core.NewInteger(1), core.NewEnd()})
	if !tryDynamicFnValueDispatch(e, 0) || e.Tape.Len() != 1 || !core.IsEnd(e.Tape.At(0)) {
		t.Errorf("a shape returning nothing consumes the window, tape len = %d", e.Tape.Len())
	}
}

func TestFnShapeTypedWindowDeclines(t *testing.T) {
	r := zzmsReg(t)
	done := r.Check.Begin()
	defer done()
	carrier := ftwNamed(t, r, "Ftw1", ftwShape(core.TInteger))
	quoted := carrier
	quoted.Quoted = true
	dynamic := carrier
	dynamic.Dynamic = true
	zero := ftwNamed(t, r, "Ftw0", core.FnUndefInfo{Sigs: []core.FnSigSpec{{Returns: []*core.Type{core.TInteger}}}})
	arityOnly := core.NewCarrier(core.TFnUndef) // claimed, but not its returns
	r.Check.NoteFnShape(arityOnly, core.FnShape{Arity: 1})
	bare := core.NewCarrier(core.TFnUndef) // no claim, and the bare node has no shape
	odd := r.Types.MintType("FtwOdd", core.TFnUndef)
	odd.SetTypeBody(core.NewInteger(1)) // a shape node whose content is no signature
	cases := map[string][]core.Value{
		"quoted":          {quoted, core.NewInteger(1), core.NewEnd()},
		"dynamic":         {dynamic, core.NewInteger(1), core.NewEnd()},
		"not fn-shaped":   {core.NewCarrier(core.TInteger), core.NewInteger(1), core.NewEnd()},
		"arity 0":         {zero, core.NewInteger(1), core.NewEnd()},
		"returns unknown": {arityOnly, core.NewInteger(1), core.NewEnd()},
		"no shape":        {bare, core.NewInteger(1), core.NewEnd()},
		"no signature":    {core.NewCarrier(odd), core.NewInteger(1), core.NewEnd()},
		"unfit argument":  {carrier, core.NewString("s"), core.NewEnd()},
		"short window":    {carrier, core.NewEnd()},
	}
	for name, tape := range cases {
		e := zzmsEngine(r, tape)
		if tryFnShapeTypedWindow(e, 0) || e.Tape.Len() != len(tape) {
			t.Errorf("%s: the plain check must leave the tape as it is", name)
		}
	}
}

// FnShapeOfSpec claims only a shape the types alone decide: one signature,
// plain parameters.
func TestFnShapeOfSpecDeclines(t *testing.T) {
	two := ftwShape(core.TInteger)
	two.Sigs = append(two.Sigs, two.Sigs[0])
	pattern := core.NewInteger(1)
	for name, info := range map[string]core.FnUndefInfo{
		"two signatures": two,
		"optional":       {Sigs: []core.FnSigSpec{{Params: []core.FnParam{{Type: core.TInteger, Optional: true}}}}},
		"patterned":      {Sigs: []core.FnSigSpec{{Params: []core.FnParam{{Type: core.TInteger, Pattern: &pattern}}}}},
		"quoted":         {Sigs: []core.FnSigSpec{{Params: []core.FnParam{{Type: core.TAtom, Quote: true}}}}},
	} {
		if _, ok := FnShapeOfSpec(info); ok {
			t.Errorf("%s: no claim", name)
		}
	}
}
