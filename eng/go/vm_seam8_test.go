package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_seam8_test.go extends vm_seam7_test.go with the residual VM dispatch
// arms it left uncovered: the get/getr poly auto-apply of a 0-arg
// trivial-delegation method (callPoly, vm.go:307-317), the shaped-method
// delegation result screen (callDynMethod guard, vm.go:622), the shaped-
// method island SUCCESS return (vm.go:657), and the typed-bind result
// screen (OpBindTyped, vm.go:1037). All are driven with the same direct-
// call / hand-built-Program seam vm_seam7 uses. The screenResults arms that
// sit only on an ISLAND path are covered organically here where the leak
// travels a DIRECT handler (poly result, delegation apply, typed bind) —
// never through a sub-engine, which re-steps tape tokens and so can never
// return one as a residual (see the residual notes in the fleet report).

// w8Reg is seam7Reg plus a convenience for asserting registration succeeded.
func w8Reg(t *testing.T) *core.Registry {
	t.Helper()
	return seam7Reg(t)
}

// w8reg0 registers a 0-arg inner native `name` with the given handler and
// returns a 0-arg trivial-delegation fn VALUE wrapping it (body [Word(name)],
// no named params) — the `r.bool`-shaped method get/getr auto-applies.
func w8Deleg0(t *testing.T, r *core.Registry, name string, impl core.Handler) core.Value {
	t.Helper()
	r.RegisterNativeFunc(core.NativeFunc{
		Name: name,
		Signatures: []core.Signature{{
			Args: nil, Returns: []*core.Type{core.TAny}, BarrierPos: -1,
			Impl: core.Go(impl),
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	return core.NewFunction(core.FnDefInfo{
		Name: name, Registry: r,
		Signatures: []core.Signature{{
			Returns: []*core.Type{core.TAny}, BarrierPos: -1,
			Impl: core.Boru([]core.Value{core.NewWord(name)}),
		}},
	})
}

// w8registerReturningPoly registers a get-family poly word `word` whose single
// [TAny] overload returns `result` verbatim — the shape callPoly re-dispatches
// and then post-processes (auto-apply / screen).
func w8registerReturningPoly(t *testing.T, r *core.Registry, word string, result core.Value) {
	t.Helper()
	r.RegisterNativeFunc(core.NativeFunc{
		Name: word,
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny}, Returns: []*core.Type{core.TAny}, BarrierPos: -1,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{result}, nil
			}),
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register poly %s: %v", word, err)
	}
}

// --- callPoly get/getr returns a 0-arg delegation method AS DATA ----------
//
// The runtime auto-apply this arm used to pin was removed with the shaped
// 0-arg landing model: the recorder now models the interpreter's instant
// auto-fire as an explicit arity-0 OpCallDynMethod after the poly (or the
// program declines via check.TryShapedMethodDispatch's guard-owned decline), so
// the poly must return the member VALUE for that opcode to consume.

func TestW8CallPolyGetMethodValueStaysData(t *testing.T) {
	r := w8Reg(t)
	deleg := w8Deleg0(t, r, "w8zero", func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(99)}, nil
	})
	// `dot` is a get-word (isGetWord). Its poly result is the delegation
	// method value; callPoly returns it VERBATIM — the arity-0
	// CALL_DYN_METHOD the recorder emits right after is what applies it.
	w8registerReturningPoly(t, r, "dot", deleg)
	vc := seam7VC(r)
	out, err := vc.callPoly(&compiler.PolyRef{Word: "dot", Arity: 1, NOut: 1}, []core.Value{core.NewInteger(1)}, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("callPoly method-value get: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("method-value get residual = %v, want the member value", out)
	}
	fd, ok := out[0].Data.(core.FnDefInfo)
	if !ok || fd.Name != "w8zero" {
		t.Errorf("get result = %v, want the w8zero delegation method value (unapplied)", out[0])
	}
	// The explicit landing opcode applies it — the runtime pair the recorder lays.
	applied, _, err := vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "w8zero", NArgs: 0, NOut: 1}, out, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("arity-0 callDynMethod: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("arity-0 apply residual = %v, want a single value", applied)
	}
	if n, _ := applied[0].AsConcreteInteger(); n != 99 {
		t.Errorf("arity-0 applied method value = %v, want 99", applied[0])
	}
}

func TestW8CallDynMethodZeroArgError(t *testing.T) {
	r := w8Reg(t)
	// The arity-0 applied inner native errors: callDynMethod surfaces it.
	deleg := w8Deleg0(t, r, "w8zfail", func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
		return nil, r.BoruError("value_error", "w8zfail: boom", "w8zfail")
	})
	vc := seam7VC(r)
	_, _, err := vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "w8zfail", NArgs: 0, NOut: 1},
		[]core.Value{deleg}, seam7Dbg, 0)
	wantErr(t, err, "w8zfail: boom")
}

func TestW8CallPolyResultScreened(t *testing.T) {
	r := w8Reg(t)
	// A get-word poly whose result is a bare Word (not an FnDef): the
	// auto-apply branch declines (not an FnDefInfo) and the belt-and-braces
	// screen rejects the tape-coupled result (vm.go:317).
	w8registerReturningPoly(t, r, "dot", core.NewWord("leak"))
	vc := seam7VC(r)
	_, err := vc.callPoly(&compiler.PolyRef{Word: "dot", Arity: 1, NOut: 1}, []core.Value{core.NewInteger(1)}, seam7Dbg, 0)
	wantInternal(t, err, "tape-coupled poly result at dot")
}

// --- callDynMethod: delegation result screen + island success ------------

// w8Deleg1 registers a 1-arg inner native `name` and returns a
// trivial-delegation fn VALUE wrapping it (body [Word(name)]).
func w8Deleg1(t *testing.T, r *core.Registry, name string, impl core.Handler) core.Value {
	t.Helper()
	r.RegisterNativeFunc(core.NativeFunc{
		Name: name,
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1,
			Impl: core.Go(impl),
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	return core.NewFunction(core.FnDefInfo{
		Name: name, Registry: r,
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1,
			Impl: core.Boru([]core.Value{core.NewWord(name)}),
		}},
	})
}

func TestW8CallDynMethodDelegationResultScreened(t *testing.T) {
	r := w8Reg(t)
	// A delegation method whose inner native returns a tape-coupled Word:
	// tryNativeFnApply hands it back DIRECTLY (no island), so guard's
	// screenResults rejects it (vm.go:622).
	deleg := w8Deleg1(t, r, "w8mleak", func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewWord("leak")}, nil
	})
	if !core.IsDelegationFnDef(deleg.Data.(core.FnDefInfo)) {
		t.Fatal("w8mleak wrapper is not a delegation fn")
	}
	vc := seam7VC(r)
	_, _, err := vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "w8mleak", NArgs: 1, NOut: 1},
		[]core.Value{core.NewInteger(5), deleg}, seam7Dbg, 0)
	wantInternal(t, err, "tape-coupled shaped method result at w8mleak")
}

func TestW8CallDynMethodIslandSuccess(t *testing.T) {
	r := w8Reg(t)
	// A non-delegation user fn (named param disqualifies the fast path) that
	// returns its arg cleanly: callDynMethod islands it and the guard SUCCESS
	// return commits the shaped result (vm.go:657).
	fn := core.NewFunction(core.FnDefInfo{
		Name: "w8okmethod", Registry: r,
		Signatures: []core.Signature{{
			Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
			Returns: []*core.Type{core.TAny}, BarrierPos: core.BarrierAllForward,
			Impl: core.Boru([]core.Value{core.NewWord("n")}),
		}},
	})
	if core.IsDelegationFnDef(fn.Data.(core.FnDefInfo)) {
		t.Fatal("named-param fn should NOT be a delegation")
	}
	vc := seam7VC(r)
	out, _, err := vc.callDynMethod(vc.r, &compiler.DynMethodSpec{Word: "w8okmethod", NArgs: 1, NOut: 1},
		[]core.Value{core.NewInteger(5), fn}, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("island method success: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("island method residual = %v, want a single value", out)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 5 {
		t.Errorf("island method returned %v, want 5", out[0])
	}
}

// --- OpBindTyped result screen -------------------------------------------

func TestW8BindTypedResultScreened(t *testing.T) {
	// A malformed program pushes a tape-coupled token (a Forward) as the value
	// a DepScalar typed-bind admits verbatim (Unify against Any returns the
	// value unchanged), so the belt-and-braces screen rejects the bound result
	// (vm.go:1037). No valid emitter ever bakes a tape-coupled const; this is
	// the same compiler-bug shape vm_seam7 feeds the other dispatch guards.
	// (A Word would be re-tagged by Unify and slip the IsWord screen, so a
	// Forward — which survives Unify unchanged — is used.)
	anyLit := core.NewTypeLiteral(core.TAny)
	p := &compiler.Program{
		Consts: []core.Value{core.NewForward(core.ForwardInfo{})},
		Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpBindTyped, Arg: 0}},
		Debug:  []core.SrcPos{{}, {}},
		TypedBinds: []core.TypedBindSpec{{
			Kind: core.TypedBindDepScalar, Name: "tb", Describe: "Any", Cons: &anyLit,
		}},
	}
	_, err := RunProgram(p, seam7Reg(t))
	wantInternal(t, err, "tape-coupled typed-bind result at tb")
}
