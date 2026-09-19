package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// fnValueWithRef builds a fn VALUE whose single sig carries ref as its
// compiled unit — the shape a value has after a compile-time bake, a
// module-load stamp, or the lazy stamp — with the given params and returns.
func fnValueWithRef(ref *compiler.CompiledFnRef, params []core.FnParam, returns []*core.Type) core.Value {
	return core.Value{Parent: core.TFunction, Data: core.FnDefInfo{
		Name: "fv",
		Signatures: []core.Signature{{
			Params:  params,
			Returns: returns,
			Impl:    core.NewBoruImplCompiled([]core.Value{core.NewInteger(7)}, ref),
		}},
	}}
}

// The seam declines — hands the value to the stepping path — for a body that
// is not a fn value, a QUOTED fn value (data on the tape, never a callee),
// and a value none of whose own sigs match the inputs: the interpreter's
// data-versus-uncalled_function fork is the stepping path's to make.
func TestFnValueSeamDeclinesNonCalleeShapes(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	ref := &compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0}
	tokens := core.Value{Parent: core.TList, Data: core.NewList([]core.Value{core.NewInteger(1)}).Data}
	if _, _, ran := vc.invokeFnValue(r, tokens, nil); ran {
		t.Error("a token-list body is not the seam's")
	}
	quoted := fnValueWithRef(ref, nil, nil)
	quoted.Quoted = true
	if _, _, ran := vc.invokeFnValue(r, quoted, nil); ran {
		t.Error("a quoted fn value is data, not a callee")
	}
	intParam := []core.FnParam{{Name: "x", Type: core.TInteger}}
	if _, _, ran := vc.invokeFnValue(r, fnValueWithRef(ref, intParam, nil), []core.Value{core.NewString("s")}); ran {
		t.Error("a value whose sigs do not match the inputs declines")
	}
}

// A value with a unit is hosted with the TOKEN seam's discipline: the unit's
// result comes back, the value's own return contract is enforced over it —
// the type with the value's name in the error, the count likewise.
func TestFnValueSeamHostsUnitWithTokenDiscipline(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	ref := &compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0}
	res, err, ran := vc.invokeFnValue(r, fnValueWithRef(ref, nil, []*core.Type{core.TInteger}), nil)
	if !ran || err != nil || len(res) != 1 {
		t.Fatalf("hosted unit: ran=%v err=%v res=%v", ran, err, res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 42 {
		t.Fatalf("the value's unit answers 42, got %v", res[0])
	}
	// No declared returns: the residual passes through untouched.
	if res, err, ran := vc.invokeFnValue(r, fnValueWithRef(ref, nil, nil), nil); !ran || err != nil || len(res) != 1 {
		t.Fatalf("no-contract passthrough: ran=%v err=%v res=%v", ran, err, res)
	}
	// The declared type is enforced, naming the value.
	_, err, ran = vc.invokeFnValue(r, fnValueWithRef(ref, nil, []*core.Type{core.TString}), nil)
	if !ran || err == nil || !strings.Contains(err.Error(), "fv: return value 1: expected String, got Integer") {
		t.Fatalf("return type: ran=%v err=%v", ran, err)
	}
	// The declared count is enforced (__RC's rule, not CallBoru's trim).
	_, err, ran = vc.invokeFnValue(r, fnValueWithRef(ref, nil, []*core.Type{core.TInteger, core.TInteger}), nil)
	if !ran || err == nil || !strings.Contains(err.Error(), "fv: expected 2 return value(s), got 1") {
		t.Fatalf("return count: ran=%v err=%v", ran, err)
	}
}

// A ref the seam cannot enter declines to the stepping path: a unit index
// past the program's table, a unit whose param count is not what the match
// produced (a compile/run drift), and a stale detached ref with no re-stamp
// box (a compile-time ref whose dep was rebound).
func TestFnValueSeamDeclinesUnenterableRefs(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	if _, _, ran := vc.invokeFnValue(r, fnValueWithRef(&compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 5}, nil, nil), nil); ran {
		t.Error("a unit index past the table declines")
	}
	intParam := []core.FnParam{{Name: "x", Type: core.TInteger}}
	ref := &compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0}
	if _, _, ran := vc.invokeFnValue(r, fnValueWithRef(ref, intParam, nil), []core.Value{core.NewInteger(1)}); ran {
		t.Error("a unit with no param slot for the matched input declines")
	}
	stale := &compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0, DepSnap: map[string]compiler.DepSnapEntry{
		"dep": {Depth: 99, Gen: -1}, // never matches → stale, and no Restamp box
	}}
	if _, _, ran := vc.invokeFnValue(r, fnValueWithRef(stale, nil, nil), nil); ran {
		t.Error("a stale ref with no re-stamp box declines")
	}
}

// A value with NO unit declines when nothing can stamp one: runtime stamping
// is not armed on the registry (the lazy stamp's first gate).
func TestFnValueSeamDeclinesUnstampedWhenStampingDisarmed(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	plain := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "plain", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(7)})}}}}
	if _, _, ran := vc.invokeFnValue(r, plain, nil); ran {
		t.Error("an unstamped value with stamping disarmed declines")
	}
}

// A value whose home is another module runs through the interpreter's own
// foreign arm — InvokeCallback at the home, which takes the unit there under
// the CallBoru discipline — not this seam's hosting.
func TestFnValueSeamForeignHomeTakesInvokeCallback(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	home := stampReg(t)
	ref := &compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0}
	fv := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{
		Name:       "abroad",
		Registry:   home,
		Signatures: []core.Signature{{Impl: core.NewBoruImplCompiled([]core.Value{core.NewInteger(7)}, ref)}},
	}}
	res, err, ran := vc.invokeFnValue(r, fv, nil)
	if !ran || err != nil || len(res) != 1 {
		t.Fatalf("foreign arm: ran=%v err=%v res=%v", ran, err, res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 42 {
		t.Fatalf("the foreign value's unit answers 42 at its home, got %v", res[0])
	}
	if r.Invoker != nil || r.NestedRunner != nil {
		t.Error("the calling registry's body seams must be untouched by a foreign run")
	}
}

// An internal error inside the hosted unit — a corrupted program, the panic
// contained — PROPAGATES. It used to degrade to the stepping path when no
// observable effect had escaped, so the interpreter answered and the
// soundness bug left no mark; the effect fence was there to stop a re-run
// repeating an effect that had. Nothing re-runs now, so both shapes report
// the same thing: a compiler defect, with ran=true because the call was
// hosted and it is the host that failed.
func TestFnValueSeamInternalErrorPropagates(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	broken := oneConstProg(42)
	broken.Fns[0].Code[0].Arg = 99 // const index past the table → index panic
	_, err, ran := vc.invokeFnValue(r, fnValueWithRef(&compiler.CompiledFnRef{Prog: broken, Unit: 0}, nil, nil), nil)
	if !ran || !core.IsInternalErr(err) {
		t.Fatalf("a contained internal error propagates: ran=%v err=%v", ran, err)
	}
	if r.Invoker != nil || r.NestedRunner != nil {
		t.Error("the hosted run must restore the enclosing body seams")
	}
}
func TestFnValueSeamBracketsRootArgsForDynEnv(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	p := oneConstProg(42)
	p.DynEnv = true
	before := r.Args.Depth()
	res, err, ran := vc.invokeFnValue(r, fnValueWithRef(&compiler.CompiledFnRef{Prog: p, Unit: 0}, nil, nil), nil)
	if !ran || err != nil || len(res) != 1 {
		t.Fatalf("DynEnv unit: ran=%v err=%v res=%v", ran, err, res)
	}
	if r.Args.Depth() != before {
		t.Fatalf("args depth %d after, want %d (the bracket restores)", r.Args.Depth(), before)
	}
	// The no-op twin: outside a DynEnv program the bracket pushes nothing.
	pop := pushRootArgs(r, oneConstProg(1), nil)
	if r.Args.Depth() != before {
		t.Fatal("a non-DynEnv program pushes no args list")
	}
	pop()
}
