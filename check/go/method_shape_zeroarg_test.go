package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// method_shape_zeroarg_test.go pins the shaped 0-arg landing model's decline
// arms and helpers (the guard-owned re-home of the miscompile-E auto-dispatch
// guard): allZeroArgSigs / fnDefName, the 0-arg window path's per-signature
// gates in shapedMethodApplyWindow, NoteMethodShape's non-delegation decline,
// and the NUnnamed arms of the VM's return contract.

// z9Member builds a named trivial-delegation fn VALUE over inner natives
// registered in r (the shaped-instance-method class).
func z9Member(name string, r *core.Registry, sigs ...core.Signature) core.Value {
	return core.NewFunction(core.FnDefInfo{Name: name, Registry: r, Signatures: sigs})
}

func z9Engine(t *testing.T, r *core.Registry, tape []core.Value) *core.Engine {
	t.Helper()
	e := core.NewTop(r)
	e.Tape = core.NewTape(tape, 8)
	e.Pointer = 0
	return e
}

func TestAllZeroArgSigsTable(t *testing.T) {
	zero := core.Signature{Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(1)}, nil
	})}
	one := zero
	one.Args = []*core.Type{core.TInteger}
	fb := zero
	fb.Fallback = true
	for _, c := range []struct {
		name string
		fn   *core.FnDefInfo
		want bool
	}{
		{"all-zero", &core.FnDefInfo{Signatures: []core.Signature{zero}}, true},
		{"zero-plus-fallback", &core.FnDefInfo{Signatures: []core.Signature{zero, fb}}, true},
		{"mixed-arity", &core.FnDefInfo{Signatures: []core.Signature{zero, one}}, false},
		{"only-fallback", &core.FnDefInfo{Signatures: []core.Signature{fb}}, false},
		{"arg-taking", &core.FnDefInfo{Signatures: []core.Signature{one}}, false},
	} {
		if got := allZeroArgSigs(c.fn); got != c.want {
			t.Errorf("%s: allZeroArgSigs = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFnDefNameFallbacks(t *testing.T) {
	r := seam7Reg(t)
	named := z9Member("z9named", r, core.Signature{Returns: []*core.Type{core.TAny}, BarrierPos: -1})
	if got := fnDefName(named); got != "z9named" {
		t.Errorf("fnDefName(named) = %q, want z9named", got)
	}
	if got := fnDefName(core.NewInteger(3)); got != "fn value" {
		t.Errorf("fnDefName(non-fn) = %q, want the generic label", got)
	}
	if got := fnDefName(core.NewFunction(core.FnDefInfo{})); got != "fn value" {
		t.Errorf("fnDefName(anonymous) = %q, want the generic label", got)
	}
}

// The 0-arg window path: a bare landed member (no forward window) models only
// a GENUINE 0-arg overload whose inner signature is a plain Go handler; a
// gate-failing 0-arg sig, an inner without any 0-arg sig, and a non-0-arg
// member all decline.
func TestShapedMethodApplyWindowZeroArgGates(t *testing.T) {
	impl := core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(7)}, nil
	})
	memberSig := core.Signature{Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: core.Boru([]core.Value{core.NewWord("z9x")})}

	// Inner 0-arg sig behind a NON-0-arg sibling: the loop skips the sibling
	// (continue) and models the 0-arg one.
	r1 := seam7Reg(t)
	r1.RegisterNativeFunc(core.NativeFunc{Name: "z9x", Signatures: []core.Signature{
		{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: impl},
		{Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: impl},
	}})
	if err := r1.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	m1 := z9Member("z9x", r1, memberSig)
	e1 := z9Engine(t, r1, []core.Value{m1})
	sg, positions, ok := shapedMethodApplyWindow(e1, 0, m1)
	if !ok || sg == nil || sg.TotalArgs() != 0 || positions != nil {
		t.Errorf("mixed-sibling inner: want the 0-arg sig with no positions, got sig=%v pos=%v ok=%v", sg, positions, ok)
	}

	// Inner 0-arg sig failing the plain-Go-handler gate (NoEvalArgs): decline.
	r2 := seam7Reg(t)
	r2.RegisterNativeFunc(core.NativeFunc{Name: "z9x", Signatures: []core.Signature{
		{Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: impl, NoEvalArgs: map[int]bool{0: true}},
	}})
	if err := r2.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	m2 := z9Member("z9x", r2, memberSig)
	e2 := z9Engine(t, r2, []core.Value{m2})
	if _, _, ok := shapedMethodApplyWindow(e2, 0, m2); ok {
		t.Error("NoEvalArgs 0-arg inner sig must not model")
	}

	// Inner with NO 0-arg sig at all (the member's 0-arg claim has no inner
	// twin): the loop finds nothing — terminal decline.
	r3 := seam7Reg(t)
	r3.RegisterNativeFunc(core.NativeFunc{Name: "z9x", Signatures: []core.Signature{
		{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: impl},
	}})
	if err := r3.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	m3 := z9Member("z9x", r3, memberSig)
	e3 := z9Engine(t, r3, []core.Value{m3})
	if _, _, ok := shapedMethodApplyWindow(e3, 0, m3); ok {
		t.Error("inner without a 0-arg sig must not model")
	}

	// A member WITHOUT a genuine 0-arg overload stays data (the
	// !fnValueZeroArg decline).
	r4 := seam7Reg(t)
	r4.RegisterNativeFunc(core.NativeFunc{Name: "z9y", Signatures: []core.Signature{
		{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: impl},
	}})
	if err := r4.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	m4Sig := memberSig
	m4Sig.Args = []*core.Type{core.TInteger}
	m4Sig.Impl = core.Boru([]core.Value{core.NewWord("z9y")})
	m4 := z9Member("z9y", r4, m4Sig, m4Sig) // two 1-arg sigs: zeros < 2
	e4 := z9Engine(t, r4, []core.Value{m4})
	if _, _, ok := shapedMethodApplyWindow(e4, 0, m4); ok {
		t.Error("a member without a genuine 0-arg overload must not take the 0-arg path")
	}
}

// NoteMethodShape declines a plain (non-delegation) fn member — a real body
// keeps today's compile failure paths.
func TestNoteMethodShapeDeclinesNonDelegation(t *testing.T) {
	r := seam7Reg(t)
	r.Check.Mode = true
	realBody := core.NewFunction(core.FnDefInfo{Name: "z9real", Registry: r, Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "n", Type: core.TInteger}}, Returns: []*core.Type{core.TAny},
		BarrierPos: -1, Impl: core.Boru([]core.Value{core.NewWord("n"), core.NewWord("n")}),
	}}})
	out := core.NewDynamicCarrier(core.TAny)
	r.Check.NoteMethodShape(out, realBody)
	if _, ok := r.Check.MethodShapeMember(out.ID); ok {
		t.Error("a non-delegation member must not be annotated")
	}
}
