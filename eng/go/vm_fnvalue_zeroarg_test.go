package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_fnvalue_zeroarg_test.go pins the zero-argument apply (vm_fnvalue_zeroarg.go):
// a NAMED fn value whose every own signature takes no argument runs over
// nothing, its results on top of the window it never touched, at the token
// seam and at CALL_DYNAMIC (trailing: the args beneath; leading: the args
// after) — and every value the interpreter's step would answer differently
// stays the island's. The corpus parity pins are lang's
// (census_items3_test.go).

// zeroArgFn builds a named (or anonymous) fn value over one signature.
func zeroArgFn(name string, anon bool, sig core.Signature) core.Value {
	return core.NewFunction(core.FnDefInfo{Name: name, Anonymous: anon, Signatures: []core.Signature{sig}})
}

// zeroArgUnitSig is a zero-param signature whose body carries ref (a unit
// answering 42) and declares returns.
func zeroArgUnitSig(ref *compiler.CompiledFnRef, returns ...*core.Type) core.Signature {
	return core.Signature{Returns: returns, Impl: core.NewBoruImplCompiled([]core.Value{core.NewInteger(7)}, ref)}
}

func intsOf(t *testing.T, vs []core.Value) []int64 {
	t.Helper()
	out := make([]int64, len(vs))
	for i, v := range vs {
		n, err := v.AsConcreteInteger()
		if err != nil {
			t.Fatalf("value %d is not an Integer: %v", i, v)
		}
		out[i] = n
	}
	return out
}

func sameInts(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The unit runs over nothing and its result lands on top of the window: at
// the token seam the inputs stay beneath it, at CALL_DYNAMIC the trailing
// form keeps its args beneath and the leading form places them after.
func TestZeroArgFnValueRunsOverTheWindow(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	g := zeroArgFn("g", false, zeroArgUnitSig(&compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0}, core.TInteger))
	one, two := core.NewInteger(1), core.NewInteger(2)

	res, err, ran := vc.zeroArgTokenSeam(r, g, []core.Value{one, two})
	if !ran || err != nil || !sameInts(intsOf(t, res), []int64{1, 2, 42}) {
		t.Fatalf("token seam: ran=%v err=%v res=%v", ran, err, res)
	}

	stack := []core.Value{core.NewInteger(9), g, one}
	st, err, ran := vc.zeroArgCallDynamic(r, g, stack[2:], append([]core.Value(nil), stack...), 1, true)
	if !ran || err != nil || !sameInts(intsOf(t, st), []int64{9, 1, 42}) {
		t.Fatalf("trailing: ran=%v err=%v stack=%v", ran, err, st)
	}
	st, err, ran = vc.zeroArgCallDynamic(r, g, stack[2:], append([]core.Value(nil), stack...), 1, false)
	if !ran || err != nil || !sameInts(intsOf(t, st), []int64{9, 42, 1}) {
		t.Fatalf("leading: ran=%v err=%v stack=%v", ran, err, st)
	}
}

// An EMPTY body has no unit and leaves nothing: the window is the answer,
// unless the body declares a return, whose count error is the interpreter's.
func TestZeroArgFnValueEmptyBody(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	empty := zeroArgFn("h", false, core.Signature{Impl: core.Boru(nil)})
	res, err, ran := vc.zeroArgTokenSeam(r, empty, []core.Value{core.NewInteger(5)})
	if !ran || err != nil || !sameInts(intsOf(t, res), []int64{5}) {
		t.Fatalf("empty body: ran=%v err=%v res=%v", ran, err, res)
	}
	declared := zeroArgFn("h", false, core.Signature{Returns: []*core.Type{core.TInteger}, Impl: core.Boru(nil)})
	if _, _, ran := vc.zeroArgTokenSeam(r, declared, []core.Value{core.NewInteger(5)}); ran {
		t.Error("an empty body declaring a return is the interpreter's to raise")
	}
}

// The unit's own return contract is enforced, and its error comes back from
// both arms with ran=true — the call was made.
func TestZeroArgFnValueContractErrorPropagates(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	bad := zeroArgFn("g", false, zeroArgUnitSig(&compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0}, core.TString))
	if _, err, ran := vc.zeroArgTokenSeam(r, bad, nil); !ran || err == nil {
		t.Fatalf("token seam: ran=%v err=%v", ran, err)
	}
	stack := []core.Value{bad, core.NewInteger(1)}
	if _, err, ran := vc.zeroArgCallDynamic(r, bad, stack[1:], stack, 0, false); !ran || err == nil {
		t.Fatalf("call dynamic: ran=%v err=%v", ran, err)
	}
}

// Everything the interpreter's step answers otherwise stays the island's.
func TestZeroArgFnValueDeclines(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	ref := &compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0}
	one := core.NewInteger(1)
	quoted := zeroArgFn("g", false, zeroArgUnitSig(ref))
	quoted.Quoted = true
	pending := core.NewList([]core.Value{core.NewWord("g")})
	pending.Eval = true
	native := core.Signature{Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return nil, nil
	})}
	for _, c := range []struct {
		label  string
		fn     core.Value
		window []core.Value
	}{
		{"not a fn value", one, nil},
		{"an anonymous lambda parks", zeroArgFn("", true, zeroArgUnitSig(ref)), nil},
		{"a value taking an argument", zeroArgFn("g", false, core.Signature{Params: []core.FnParam{{Name: "a", Type: core.TInteger}}, Impl: core.Boru([]core.Value{one})}), nil},
		{"a quoted value is data", quoted, nil},
		{"a window value the island's end would evaluate", zeroArgFn("g", false, zeroArgUnitSig(ref)), []core.Value{pending}},
		{"a native", zeroArgFn("g", false, native), nil},
		{"a body with no unit", zeroArgFn("g", false, core.Signature{Impl: core.Boru([]core.Value{one})}), nil},
	} {
		if _, _, ran := vc.zeroArgTokenSeam(r, c.fn, c.window); ran {
			t.Errorf("token seam: %s must decline", c.label)
		}
	}
	// CALL_DYNAMIC steps every arg on its island: one that is not its own
	// result declines before the value is looked at.
	g := zeroArgFn("g", false, zeroArgUnitSig(ref))
	stack := []core.Value{g, pending}
	if _, _, ran := vc.zeroArgCallDynamic(r, g, stack[1:], stack, 0, true); ran {
		t.Error("call dynamic: an arg the island steps must decline")
	}
	if _, _, ran := vc.zeroArgCallDynamic(r, one, []core.Value{one}, []core.Value{one, one}, 0, true); ran {
		t.Error("call dynamic: a non-fn value must decline")
	}
}
