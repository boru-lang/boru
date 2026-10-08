package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// A minimal `cif` conditional word registered from inside the kernel,
// mirroring lang's if3 (native_control.go): the runtime handler splices
// the chosen body, the check-mode ReturnsFn runs both arms through the
// carrier machinery and records a BranchRecord so the emit → lower → VM
// pipeline exercises its branch/join paths.

func cifSplice(v core.Value) []core.Value {
	if v.Parent.Equal(core.TList) && v.Data != nil && !core.IsTypedList(v) && !core.IsTableType(v) {
		elems, _ := core.AsList(v)
		out := make([]core.Value, 0, elems.Len()+2)
		out = append(out, core.NewOpenParen())
		out = append(out, elems.Slice()...)
		out = append(out, core.NewCloseParen())
		return out
	}
	return []core.Value{v}
}

func cifReturns(args []core.Value, r *core.Registry) []core.Value {
	es := r.Check
	if lit, ok := core.LiteralCondValue(args[0]); ok {
		// Statically-known condition: capture only the taken arm.
		var stk []core.Value
		var defs map[string]core.Value
		if lit {
			restore := core.ApplyGuardNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = core.RunCarrierBodyWithDefs(r, args[1])
			restore()
			core.InstallJoinedDefs(r, defs, nil)
		} else {
			restore := core.ApplyComplementNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = core.RunCarrierBodyWithDefs(r, args[2])
			restore()
			core.InstallJoinedDefs(r, nil, defs)
		}
		frag := testRecorderState(es).TakeFragment()
		if len(stk) == 0 {
			es.Recorder().MarkUncompilable("cif: branch produces no value")
			return nil
		}
		out := stk[len(stk)-1]
		taken := lit
		testRecorderState(es).RecordBranch(core.BranchRecord{
			ConstCond: &taken, HasElse: true,
			Then: frag, ThenStk: stk, Out: out, Pos: args[0].Pos(),
		})
		return []core.Value{out}
	}

	armOf := func(arg core.Value, then bool) (core.EmitFragmentRef, []core.Value, map[string]core.Value, *core.Value) {
		if core.IsConcrete(arg) && arg.Parent.ConformsTo(core.TList) {
			var restore func()
			if then {
				restore = core.ApplyGuardNarrowing(r, args[0])
			} else {
				restore = core.ApplyComplementNarrowing(r, args[0])
			}
			es.Recorder().ArmBranchCapture()
			stk, defs := core.RunCarrierBodyWithDefs(r, arg)
			frag := testRecorderState(es).TakeFragment()
			restore()
			return frag, stk, defs, nil
		}
		v := arg
		return nil, []core.Value{v}, nil, &v
	}
	thenFrag, thenStk, thenDefs, thenValue := armOf(args[1], true)
	elseFrag, elseStk, elseDefs, elseValue := armOf(args[2], false)
	core.InstallJoinedDefs(r, thenDefs, elseDefs)
	joined := core.JoinCarrierStacks(thenStk, elseStk)
	if len(joined) == 0 {
		out := core.NewCarrier(core.TNone)
		testRecorderState(es).RecordBranch(core.BranchRecord{
			Cond: args[0], HasElse: true,
			Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
			ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
		})
		if !es.Recorder().Active() {
			return nil
		}
		return []core.Value{out}
	}
	if !es.Recorder().Active() {
		if v, ok := core.FoldVariadicArms(thenStk, elseStk); ok {
			return []core.Value{v}
		}
	}
	out := joined[len(joined)-1]
	testRecorderState(es).RecordBranch(core.BranchRecord{
		Cond: args[0], HasElse: true,
		Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
		ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
	})
	return []core.Value{out}
}

// registerBranchWords adds `cif` (3-arg conditional) and `cgt`
// (integer greater-than) to a covRegistry.
func registerBranchWords(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cif",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny, core.TAny, core.TAny},
			NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				condVal := args[0]
				// A single-element list condition ([true] — the form
				// LiteralCondValue folds statically) unwraps at runtime.
				if lst, err := core.AsList(condVal); err == nil && lst.Len() == 1 {
					condVal = lst.Get(0)
				}
				cond, _ := core.AsBoolean(condVal)
				if cond {
					return cifSplice(args[1]), nil
				}
				return cifSplice(args[2]), nil
			}),
			ReturnsFn: cifReturns, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cgt",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				// Convention: args[1] OP args[0] for natural forward reading.
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				return []core.Value{core.NewBoolean(a > b)}, nil
			}),
			Returns: []*core.Type{core.TBoolean}, BarrierPos: -1,
		}},
	})
}

func codeBody(tokens ...core.Value) core.Value {
	return core.NewList(tokens)
}

func TestCompiledBranchTakesThen(t *testing.T) {
	// cif (cgt 5 3) [cadd 1 2] [cadd 30 40] → 3
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(5), core.NewInteger(3), core.NewCloseParen(),
			codeBody(core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2)),
			codeBody(core.NewWord("cadd"), core.NewInteger(30), core.NewInteger(40)),
		}
	})
	if got != "3" {
		t.Errorf("got %q, want 3", got)
	}
}

func TestCompiledBranchTakesElse(t *testing.T) {
	// cif (cgt 3 5) [cadd 1 2] [cadd 30 40] → 70
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(3), core.NewInteger(5), core.NewCloseParen(),
			codeBody(core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2)),
			codeBody(core.NewWord("cadd"), core.NewInteger(30), core.NewInteger(40)),
		}
	})
	if got != "70" {
		t.Errorf("got %q, want 70", got)
	}
}

func TestCompiledBranchValueArms(t *testing.T) {
	// Value (non-body) arms: cif (cgt 1 2) 10 20 → 20.
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(1), core.NewInteger(2), core.NewCloseParen(),
			core.NewInteger(10),
			core.NewInteger(20),
		}
	})
	if got != "20" {
		t.Errorf("got %q, want 20", got)
	}
}

func TestCompiledBranchMixedArms(t *testing.T) {
	// Then is a body, else a plain value.
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(9), core.NewInteger(2), core.NewCloseParen(),
			codeBody(core.NewWord("cmul"), core.NewInteger(6), core.NewInteger(7)),
			core.NewInteger(0),
		}
	})
	if got != "42" {
		t.Errorf("got %q, want 42", got)
	}
}

func TestCompiledBranchConstCondFolds(t *testing.T) {
	// A literal list-form condition ([true]) folds statically: only the
	// taken arm is captured (BranchRecord.ConstCond).
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			codeBody(core.NewBoolean(true)),
			codeBody(core.NewWord("cadd"), core.NewInteger(20), core.NewInteger(22)),
			codeBody(core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(1)),
		}
	})
	if got != "42" {
		t.Errorf("got %q, want 42", got)
	}
	got = runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			codeBody(core.NewBoolean(false)),
			codeBody(core.NewWord("cadd"), core.NewInteger(20), core.NewInteger(22)),
			codeBody(core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(1)),
		}
	})
	if got != "2" {
		t.Errorf("got %q, want 2", got)
	}
}

func TestCompiledNestedBranches(t *testing.T) {
	inner := codeBody(
		core.NewWord("cif"),
		core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(1), core.NewInteger(2), core.NewCloseParen(),
		codeBody(core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(1)),
		codeBody(core.NewWord("cadd"), core.NewInteger(2), core.NewInteger(2)),
	)
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(5), core.NewInteger(3), core.NewCloseParen(),
			inner,
			codeBody(core.NewWord("cadd"), core.NewInteger(9), core.NewInteger(9)),
		}
	})
	if got != "4" {
		t.Errorf("got %q, want 4 (inner else)", got)
	}
}

func TestCompiledBranchFeedsCall(t *testing.T) {
	// The branch result becomes an argument of a following call:
	// cadd (cif (cgt 5 3) [cadd 1 2] [cadd 3 4]) 10 → 13
	got := runDifferential(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cadd"),
			core.NewOpenParen(),
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(5), core.NewInteger(3), core.NewCloseParen(),
			codeBody(core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2)),
			codeBody(core.NewWord("cadd"), core.NewInteger(3), core.NewInteger(4)),
			core.NewCloseParen(),
			core.NewInteger(10),
		}
	})
	if got != "13" {
		t.Errorf("got %q, want 13", got)
	}
}

func TestCompiledBranchZeroValueStatement(t *testing.T) {
	// Both arms run a 0-return word: the if is a 0-value statement
	// (zeroOut phantom); a following literal is the program's only
	// result in both engines.
	setup := func(r *core.Registry) {
		registerBranchWords(r)
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "cnoop",
			Signatures: []core.Signature{{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return nil, nil
				}),
				Returns: []*core.Type{}, BarrierPos: -1,
			}},
		})
	}
	got := runDifferential(t, setup, func() []core.Value {
		return []core.Value{
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(5), core.NewInteger(3), core.NewCloseParen(),
			codeBody(core.NewWord("cnoop"), core.NewInteger(1)),
			codeBody(core.NewWord("cnoop"), core.NewInteger(2)),
			core.NewEnd(),
			core.NewInteger(77),
		}
	})
	if got != "77" {
		t.Errorf("got %q, want 77", got)
	}
}

func TestCarrierJoinHelpers(t *testing.T) {
	// JoinCarrierStacks joins same-length stacks element-wise.
	a := []core.Value{core.NewCarrier(core.TInteger)}
	b := []core.Value{core.NewCarrier(core.TString)}
	joined := core.JoinCarrierStacks(a, b)
	if len(joined) != 1 {
		t.Fatalf("joined len = %d", len(joined))
	}
	// The join of Integer and String carriers is a wider carrier —
	// it must still admit both leaf kinds via unification.
	j := joined[0]
	if core.IsConcrete(j) {
		t.Errorf("join of two carriers is concrete: %v", j)
	}
	// Mismatched lengths: the shorter side wins (variadic absorption
	// is the caller's job); just assert no panic and sane length.
	joined = core.JoinCarrierStacks([]core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TInteger)}, nil)
	if len(joined) > 2 {
		t.Errorf("mismatched join len = %d", len(joined))
	}

	// JoinCarriers on values.
	jc := core.JoinCarriers(core.NewCarrier(core.TInteger), core.NewCarrier(core.TInteger))
	if !strings.Contains(jc.String(), "Integer") {
		t.Errorf("JoinCarriers(Integer,Integer) = %v", jc)
	}

	// BoolWord renders the source literal.
	if core.BoolWord(true) != "true" || core.BoolWord(false) != "false" {
		t.Error("BoolWord wrong")
	}

	// LiteralCondValue folds a single-element condition list holding a
	// concrete boolean (or a true/false word); anything else declines.
	if v, ok := core.LiteralCondValue(core.NewList([]core.Value{core.NewBoolean(true)})); !ok || !v {
		t.Error("LiteralCondValue([true]) failed")
	}
	if v, ok := core.LiteralCondValue(core.NewList([]core.Value{core.NewWord("false")})); !ok || v {
		t.Error("LiteralCondValue([word false]) failed")
	}
	if _, ok := core.LiteralCondValue(core.NewList([]core.Value{core.NewCarrier(core.TBoolean)})); ok {
		t.Error("Boolean carrier misread as a literal condition")
	}
	if _, ok := core.LiteralCondValue(core.NewBoolean(true)); ok {
		t.Error("bare boolean (non-list) misread as a condition list")
	}
	if _, ok := core.LiteralCondValue(core.NewList([]core.Value{core.NewBoolean(true), core.NewBoolean(false)})); ok {
		t.Error("two-element condition list misread as literal")
	}
}

// testRecorderState unwraps the concrete backend for the branch-pipeline
// fixtures; under a plain-check pass (inactive recorder) it returns a nil
// *EmitState whose methods no-op via the kernel's nil-receiver guards —
// the same decline the pre-S2 interface no-ops provided.
func testRecorderState(c *core.CheckState) *compiler.EmitState {
	es, _ := c.Recorder().(*compiler.EmitState)
	return es
}
