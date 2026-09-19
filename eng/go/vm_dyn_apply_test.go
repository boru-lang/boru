package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// dynApplyFn builds a fn VALUE whose single sig carries a compiled ref to
// prog.Fns[unit] — the shape stampFnConst produces for a fn-value const.
func dynApplyFn(params []core.FnParam, prog *compiler.Program, unit int) core.Value {
	impl := core.Boru([]core.Value{core.NewInteger(0)})
	impl.SetCompiled(&compiler.CompiledFnRef{Prog: prog, Unit: unit})
	fd := core.FnDefInfo{Signatures: []core.Signature{{
		Params: params, Returns: []*core.Type{core.TAny}, BarrierPos: core.BarrierAllForward,
		Impl: impl,
	}}}
	return core.Value{Parent: core.TFunction, Data: fd}
}

// The declines: dynApplyEnter answers nil for anything it cannot enter as a
// frame of THIS program, and the island then owns the apply exactly as before.
// Each arm below is a different reason, and every one of them keeps the answer
// the interpreter's.
func TestDynApplyEnterDeclines(t *testing.T) {
	vc, _ := foreignVC(t, oneConstProg(1))
	prog := oneConstProg(7)

	t.Run("not a fn value", func(t *testing.T) {
		if ent := vc.dynApplyEnter(core.NewInteger(5), nil); ent != nil {
			t.Fatalf("a non-fn value is not a callee, got %v", ent)
		}
	})

	t.Run("quoted fn is data", func(t *testing.T) {
		fn := dynApplyFn(nil, prog, 0)
		fn.Quoted = true
		if ent := vc.dynApplyEnter(fn, nil); ent != nil {
			t.Fatal("a QUOTED fn is data — entering it turns `((mk) 2)` from " +
				"[fn (Integer) 2] into [3], a silent wrong answer")
		}
	})

	t.Run("unit belongs to another program", func(t *testing.T) {
		// A DETACHED ref: its unit index means nothing against vc.p, and
		// hosting it nested would reintroduce the per-body context bracket a
		// fn application must not have.
		fn := dynApplyFn(nil, oneConstProg(9), 0)
		if ent := vc.dynApplyEnter(fn, nil); ent != nil {
			t.Fatal("a ref from another program cannot be a frame of this one")
		}
	})

	t.Run("unit index outside the table", func(t *testing.T) {
		fn := dynApplyFn(nil, vc.p, 99)
		if ent := vc.dynApplyEnter(fn, nil); ent != nil {
			t.Fatal("an out-of-range unit is compile/run drift, not a callee")
		}
	})

	t.Run("unit shape disagrees with the match", func(t *testing.T) {
		// The sig matched one arg; the unit declares none. Entering on a
		// mismatch would bind the wrong locals silently.
		fn := dynApplyFn([]core.FnParam{{Name: "n", Type: core.TAny}}, vc.p, 0)
		if ent := vc.dynApplyEnter(fn, []core.Value{core.NewInteger(1)}); ent != nil {
			t.Fatalf("NParams %d vs 1 arg must decline, got %v", vc.p.Fns[0].NParams, ent)
		}
	})
}

// A LIST argument arrives QUOTED, so body references read it as data — the
// compiled mirror of the interpreter's binding rule, and the same delivery
// OpCallUserPoly performs. Without it a body that names its list param would
// re-step the list's contents.
func TestDynApplyEnterQuotesListParams(t *testing.T) {
	vc, r := foreignVC(t, &compiler.Program{
		Fns: []compiler.CompiledFn{{
			Name: "takesList", NParams: 1, NLocals: 1, NArgs: 1,
			Code:  []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug: make([]core.SrcPos, 2),
		}},
	})
	_ = r
	fn := dynApplyFn([]core.FnParam{{Name: "l", Type: core.TList}}, vc.p, 0)
	lst := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	if lst.Quoted {
		t.Fatal("fixture list arrived already quoted — the assertion below would be vacuous")
	}
	ent := vc.dynApplyEnter(fn, []core.Value{lst})
	if ent == nil {
		t.Fatal("a matching list param must enter")
	}
	if !ent.locals[0].Quoted {
		t.Error("a LIST param must be delivered QUOTED, as OpCallUserPoly delivers it")
	}
}

// The param-contract re-check at the frame push. MatchFnSig selects on the
// SIGNATURE's types; the unit records its own declared Params, and a drift
// between the two must raise the interpreter's no-match rather than bind a
// value the body's declaration rejects.
func TestDynApplyFramePushChecksParamContract(t *testing.T) {
	r := stampReg(t)
	prog := &compiler.Program{
		Consts: []core.Value{core.NewString("nope")},
		Fns: []compiler.CompiledFn{{
			Name: "wantsInteger", NParams: 1, NLocals: 1, NArgs: 1,
			Params: []*core.Type{core.TInteger}, // the unit demands Integer …
			Code:   []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug:  make([]core.SrcPos, 2),
		}},
	}
	// … while the SIG matched Any, so a String reaches the frame push.
	fn := dynApplyFn([]core.FnParam{{Name: "n", Type: core.TAny}}, prog, 0)
	prog.Consts = append(prog.Consts, fn)
	prog.Code = []compiler.Instr{
		{Op: compiler.OpPushConst, Arg: 1}, // the fn
		{Op: compiler.OpPushConst, Arg: 0}, // "nope"
		{Op: compiler.OpCallDynamic, Arg: 1},
	}
	prog.Debug = make([]core.SrcPos, 3)

	_, err := RunProgram(prog, r)
	if err == nil {
		t.Fatal("a String at an Integer-declared unit param must raise, not bind")
	}
	if !strings.Contains(err.Error(), "wantsInteger") {
		t.Fatalf("err = %v, want the callee's no-match", err)
	}
}

// dynFrameSimpleWindow decides whether a replay token region is the one shape
// the Apply kernel can enter: a fn followed by plain data. Every rejection
// below is a region the INTERPRETER would re-step and a frame push cannot, so
// each one keeps the island rather than answering differently.
func TestDynFrameSimpleWindow(t *testing.T) {
	prog := oneConstProg(1)
	fn := dynApplyFn(nil, prog, 0)
	quoted := fn
	quoted.Quoted = true

	for _, tc := range []struct {
		name string
		toks []core.Value
		want bool
	}{
		{"fn then data", []core.Value{fn, core.NewInteger(1), core.NewString("x")}, true},
		{"fn alone", []core.Value{fn}, true},
		{"lead is not a fn", []core.Value{core.NewInteger(1), fn}, false},
		{"lead is quoted", []core.Value{quoted, core.NewInteger(1)}, false},
		// A SECOND fn in the region collects its own neighbours when the
		// interpreter re-steps the window; the frame push would hand it to the
		// first fn as an argument instead.
		{"a second fn follows", []core.Value{fn, core.NewInteger(1), fn}, false},
		// A tape-coupled token is re-stepped by definition.
		{"a word follows", []core.Value{fn, core.NewWord("add")}, false},
	} {
		if got := dynFrameSimpleWindow(tc.toks); got != tc.want {
			t.Errorf("%s: dynFrameSimpleWindow = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The island is still the fallback, and this row keeps it exercised.
//
// Before the Apply kernel every apply-top of a user fn reached the island; now
// a fn carrying a compiled unit is ENTERED instead, and the island's success
// return stopped being reached by any corpus row — fourteen probed source
// shapes (usurp-built values, runtime-returned fns, bodies whose stamp should
// decline) all took the compiled path. That is worth a row rather than an
// allowlist entry: "no program I could write reaches this" is the same
// reasoning that hid a silent miscompile earlier in this work, and it is not a
// proof of unreachability.
//
// The decline this row builds is the honest one: an appliable, NON-delegation
// fn value whose signature carries no compiled ref. A Go-impl signature is used
// because it dispatches inside the island without the language layer's frame
// tokens, which a bare kernel registry does not have.
func TestDynApplyTopIslandStillRuns(t *testing.T) {
	r := stampReg(t)
	sig := core.Signature{
		Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
		Returns: []*core.Type{core.TInteger}, BarrierPos: core.BarrierAllForward,
		Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			n, _ := core.AsInteger(a[0])
			return []core.Value{core.NewInteger(n * 3)}, nil
		}),
	}
	fd := core.FnDefInfo{Name: "zztriple", Signatures: []core.Signature{sig}}
	fnVal := core.Value{Parent: core.TFunction, Data: fd}
	if core.IsDelegationFnDef(fd) {
		t.Fatal("fixture reads as a delegation — it would take tryNativeFnApply, not the island")
	}

	vc := &vmContext{p: oneConstProg(1), r: r, ceiling: vmStackCeiling(r), stepLimit: core.DefaultStepLimit}
	// apply-top layout: the fn sits ON TOP of its single arg.
	got, ent, err := vc.callDynApplyTop(r, 1, []core.Value{core.NewInteger(5), fnVal}, make([]core.SrcPos, 4), 0)
	if err != nil {
		t.Fatalf("callDynApplyTop: %v", err)
	}
	if ent != nil {
		t.Fatal("a fn with no compiled unit has nothing to enter — it must take the island")
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if n, _ := got[0].AsConcreteInteger(); n != 15 {
		t.Fatalf("island apply zztriple(5) = %v, want 15", got[0])
	}
}
