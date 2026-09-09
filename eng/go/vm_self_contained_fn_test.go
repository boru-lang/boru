package eng

import (
	"testing"

	"github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_self_contained_fn_test.go pins the thirty-fifth increment's VM arms: a
// SELF-CONTAINED Go-impl fn value (fn-util's produced wrappers) applies on
// its OWN signatures, never through the live registry under its label, and
// callDynMethod resolves a modifier wrapper to what it wraps.

// scFn builds fn-util's produced-wrapper shape (goFnValue): anonymous, Name a
// LABEL, one all-forward Go-impl signature of n Any params.
func scFn(name string, n int, h core.Handler) core.Value {
	params := make([]core.FnParam, n)
	for i := range params {
		params[i] = core.FnParam{Type: core.TAny}
	}
	sig := core.Signature{Params: params, BarrierPos: n, Impl: core.Go(h)}
	core.NormalizeSig(&sig)
	return core.NewFunction(core.FnDefInfo{Name: name, Anonymous: true, Signatures: []core.Signature{sig}, MaxForwardArgs: n})
}

func scConst(v int64) core.Handler {
	return func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(v)}, nil
	}
}

// A registered word SHARES the wrapper's label and answers 99 — the measured
// miscompile: `((FnUtil.const 7) 99)` compiled to 99 for the interpreter's 7
// while `const` resolved to the singleton-type maker.
func TestSelfContainedGoFnAppliesOnOwnSigs(t *testing.T) {
	r := seam7Reg(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "sclabel",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny}, Returns: []*core.Type{core.TAny}, BarrierPos: -1,
			Impl: core.Go(scConst(99)),
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	seven := scFn("sclabel", 1, scConst(7))
	fd := seven.Data.(core.FnDefInfo)
	if !core.IsSelfContainedGoFnDef(fd) || !vmNativeApplicable(r, fd) {
		t.Fatal("a self-contained Go-impl fn value is VM-native applicable")
	}
	vc := seam7VC(r)
	want7 := func(got []core.Value, err error, where string) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", where, err)
		}
		if n, _ := got[len(got)-1].AsConcreteInteger(); len(got) != 1 || n != 7 {
			t.Errorf("%s = %v, want [7] (the value's OWN handler, not the registered word's 99)", where, got)
		}
	}
	got, _, err := vc.callDynamic(vc.r, 1, false, []core.Value{seven, core.NewInteger(99)}, seam7Dbg, 0)
	want7(got, err, "callDynamic leading")
	got, _, err = vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "k", NArgs: 1, NOut: 1}, []core.Value{core.NewInteger(99), seven}, seam7Dbg, 0)
	want7(got, err, "callDynMethod")
	// An arg count the own signature does not take declines the fast path;
	// the caller islands, where the value collects its own arity.
	if _, done, _ := vc.tryNativeFnApply(seven, []core.Value{core.NewInteger(1), core.NewInteger(2)}); done {
		t.Error("two args over a unary own signature must decline the native apply")
	}
}

// callDynMethod's retry against what a modifier wrapper wraps (`FnUtil.flip`'s
// usurp wrapper under a def-read apply): a usurp over a delegation native
// dispatches the inner native with the args reversed, and its error surfaces.
func TestCallDynMethodUnwrapsModifierWrapper(t *testing.T) {
	r := seam7Reg(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "scsub",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				x, _ := core.AsInteger(a[0])
				y, _ := core.AsInteger(a[1])
				return []core.Value{core.NewInteger(x - y)}, nil
			}),
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "scboom",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
				return nil, r.BoruError("value_error", "scboom: boom", "scboom")
			}),
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	deleg := func(name string) core.Value {
		return core.NewFunction(core.FnDefInfo{Name: name, Registry: r, Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TInteger},
			BarrierPos: core.BarrierAllForward,
			Impl:       core.Boru([]core.Value{core.NewWord(name)}),
		}}})
	}
	flipped, ok := core.UsurpFunction(deleg("scsub"))
	if !ok {
		t.Fatal("usurp over a delegation wrapper")
	}
	vc := seam7VC(r)
	// (fs 3 10): sig order [3, 10] — the fn on top, the first arg at top-1.
	got, _, err := vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "fs", NArgs: 2, NOut: 1}, []core.Value{core.NewInteger(10), core.NewInteger(3), flipped}, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("flipped apply: %v", err)
	}
	if n, _ := got[0].AsConcreteInteger(); len(got) != 1 || n != 7 {
		t.Errorf("flipped scsub(3, 10) = %v, want [7] (scsub 10 3)", got)
	}
	boom, _ := core.UsurpFunction(deleg("scboom"))
	_, _, err = vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "fb", NArgs: 2, NOut: 1}, []core.Value{core.NewInteger(10), core.NewInteger(3), boom}, seam7Dbg, 0)
	wantErr(t, err, "scboom: boom")
}

// A handler error is anchored on the fn VALUE's position, its token text
// included: the caret is the read token's, as the interpreter's stampErrPos
// stamps it, not the word the handler named.
func TestSelfContainedGoFnErrorAnchoredOnValue(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	failing := scFn("scfail", 1, func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
		return nil, r.BoruError("value_error", "scfail: boom", "FnUtil.compose")
	})
	failing = core.WithPosAt(failing, core.SrcPos{Row: 3, Col: 5, Src: "h"})
	_, done, err := vc.tryNativeFnApply(failing, []core.Value{core.NewInteger(1)})
	if !done {
		t.Fatal("the own-sig apply must run the handler")
	}
	wantErr(t, err, "scfail: boom")
	ae, ok := err.(*core.BoruError)
	if !ok || ae.Row != 3 || ae.Col != 5 || ae.Src != "h" {
		t.Errorf("error anchored at %v, want 3:5 with the token text `h`", err)
	}
}
