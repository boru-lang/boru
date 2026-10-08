package compiler

import (
	"strings"
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

func TestS5bEReturnsFnArmedRootCarrierArg(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	defer done()

	sig := core.FnSig{
		Params: []core.FnParam{{Name: "s5bez", Type: core.TAny}},
		Impl:   core.Boru([]core.Value{core.NewInteger(1)}),
	}
	rf := check.BuildFnBodyReturnsFn(r, "s5beh", sig, core.FnDefInfo{})
	// A root-node arg (nil Parent): the armed generalisation must keep it
	// as a carrier rather than calling NewCarrier(nil).
	anyLit := core.NewTypeLiteral(core.TAny)
	if anyLit.Parent != nil {
		t.Fatalf("test premise: the Any literal is its own root (nil Parent), got %v", anyLit.Parent)
	}
	out := rf([]core.Value{anyLit}, r)
	if out == nil {
		t.Log("armed ReturnsFn returned nil residual (acceptable; path exercised)")
	}
}

// The guard-owned decline: an annotated genuine-0-arg member whose landing
// the window model cannot claim (a raw Word in the statement window) DECLINES
// the program — the read guard was skipped for the annotated read, so the
// landing owns the miscompile-E compile failure.
func TestShapedMethodGuardOwnedDecline(t *testing.T) {
	r := seam7Reg(t)
	impl := core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(7)}, nil
	})
	r.RegisterNativeFunc(core.NativeFunc{Name: "z9g", Signatures: []core.Signature{
		{Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: impl},
	}})
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	member := z9Member("z9g", r, core.Signature{
		Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: core.Boru([]core.Value{core.NewWord("z9g")}),
	})
	done := r.Check.BeginCompilePass()
	defer done()
	carrier := core.NewDynamicCarrier(core.TAny)
	r.Check.NoteMethodShape(carrier, member)
	if _, ok := r.Check.MethodShapeMember(carrier.ID); !ok {
		t.Fatal("genuine-0-arg delegation member must be annotated")
	}
	e := z9Engine(t, r, []core.Value{carrier, core.NewWord("z9unmodelable")})
	// WIDENED 2026-07-17 (the S9.4 probe sweep): an ALL-0-ARG member skips
	// the statement-window scan — it auto-fires with no operands, so the
	// following token belongs to the NEXT dispatch and cannot decline this
	// one. The MODEL is attempted regardless of the follower; this synthetic
	// carrier has NO compiled home, so RecordDynMethod declines and the
	// guard-owned compile failure moves to the operand-provenance arm — still sound,
	// still interpreter-owned.
	check.TryShapedMethodDispatch(e, 0)
	es, ok := r.Check.Emit.(*EmitState)
	if !ok {
		t.Fatal("compile pass recorder missing")
	}
	if es.Compilable || !strings.Contains(es.Reason, "operand of unknown provenance at z9g") {
		t.Errorf("want the operand-provenance compile failure, got compilable=%v reason=%q", es.Compilable, es.Reason)
	}

	// A 0-arg member whose MODEL screens out (a NoEvalArgs sig — the arity-0
	// apply cannot bake it) keeps the guard-owned 213 compile failure: the model is
	// attempted (the window scan is skipped for all-0-arg members) but the
	// signature screen declines every sig, so the landing declines rather
	// than auto-dispatching as data (the miscompile-E belt, re-homed).
	r2 := seam7Reg(t)
	impl2 := core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(7)}, nil
	})
	r2.RegisterNativeFunc(core.NativeFunc{Name: "z9h", Signatures: []core.Signature{
		{Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: impl2, NoEvalArgs: map[int]bool{0: true}},
	}})
	if err := r2.Err(); err != nil {
		t.Fatalf("register z9h: %v", err)
	}
	member2 := z9Member("z9h", r2, core.Signature{
		Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: core.Boru([]core.Value{core.NewWord("z9h")}),
	})
	done2 := r2.Check.BeginCompilePass()
	defer done2()
	carrier2 := core.NewDynamicCarrier(core.TAny)
	r2.Check.NoteMethodShape(carrier2, member2)
	e2 := z9Engine(t, r2, []core.Value{carrier2, core.NewWord("z9unmodelable")})
	check.TryShapedMethodDispatch(e2, 0)
	es2, ok2 := r2.Check.Emit.(*EmitState)
	if !ok2 {
		t.Fatal("compile pass recorder missing (z9h)")
	}
	if es2.Compilable || !strings.Contains(es2.Reason, "shaped 0-arg method landing not modelable at z9h") {
		t.Errorf("want the 0-arg guard compile failure, got compilable=%v reason=%q", es2.Compilable, es2.Reason)
	}
}

func TestS6aRunFnBodyOnceErrorMarksUncompilable(t *testing.T) {
	r := newTestRegistry(t)
	es := armEmit(r)
	got := check.RunFnBodyOnce(r, "s6afn", nil, []core.Value{core.NewWord("s6a_undef_body_word")}, nil, nil, false)
	if got != nil {
		t.Errorf("an erroring body returns nil, got %v", got)
	}
	if es.Compilable {
		t.Error("an erroring body under an armed recorder must mark the program uncompilable")
	}
}

func TestW8FailToCompileForwardStackDriftOutOfRange(t *testing.T) {
	// An out-of-range matched position bails the drift check early.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(1)}, core.StackHeadroom)
	sig := &core.Signature{Args: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: 1}
	check.DeclineForwardStackDrift(e, sig, []int{0, 999})
}

func TestW8SpliceFnValueCheckResultEmptyDeclaredReturns(t *testing.T) {
	// A body producing no carrier with declared returns degrades to one
	// carrier per declared return.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	fnDef := core.FnDefInfo{Name: "w8e", Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TInteger}}, Returns: []*core.Type{core.TInteger, core.TString},
		Impl: core.Boru(parenBody()), BarrierPos: 0,
	}}}
	sig := &fnDef.Signatures[0]
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(3), core.NewFunction(fnDef)}, core.StackHeadroom)
	e.Pointer = 1
	check.SpliceFnValueCheckResult(e, 1, 1, fnDef, sig, []core.Value{core.NewInteger(3)})
}

// TestCarrierResultsPartitionCountMismatchKeepsRecorded pins check.CarrierResults'
// partitioned-user-fn tail: when the partition-joined row count differs from
// the recorded dispatch's result count, the RECORDED results are returned
// verbatim (sound, just wider) instead of re-IDing a mismatched tuple. The
// crafted ReturnsFn answers TWO carriers for the whole-disjunct args and ONE
// per concrete alternative combo.
func TestCarrierResultsPartitionCountMismatchKeepsRecorded(t *testing.T) {
	r := covRegistry(t, nil)
	impl := &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}, FnFrame: &core.FnFrameMeta{}}
	r.RegisterNativeFunc(core.NativeFunc{Name: "ljmis", Signatures: []core.Signature{{
		Args: []*core.Type{core.TAny}, BarrierPos: -1, Impl: impl,
		ReturnsFn: func(args []core.Value, _ *core.Registry) []core.Value {
			if len(args) == 1 && core.IsDisjunct(args[0]) {
				return []core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TString)}
			}
			return []core.Value{core.NewCarrier(core.TInteger)}
		},
	}}})
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	done := w8ArmCompile(t, r)
	defer done()
	fn := r.Lookup("ljmis")
	if fn == nil || len(fn.Signatures) == 0 {
		t.Fatal("ljmis not registered")
	}
	d := strictDisjunct(core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString))
	out := check.CarrierResults(r, "ljmis", &fn.Signatures[0], []core.Value{d}, core.SrcPos{}, nil, false)
	if len(out) != 2 {
		t.Fatalf("the count mismatch must keep the RECORDED results (2), got %d: %v", len(out), out)
	}
}

func TestPlainCheckDisjunctPartition(t *testing.T) {
	// A branch-join disjunct (Integer|String) into a poly word partitions
	// the overloads per alternative (disjunctPartitionReturns).
	setup := func(r *core.Registry) {
		registerBranchWords(r)
	}
	diags := plainCheckRun(t, setup, []core.Value{
		core.NewWord("cdub"),
		core.NewOpenParen(),
		core.NewWord("cif"),
		core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(1), core.NewInteger(2), core.NewCloseParen(),
		core.NewInteger(1),
		core.NewString("s"),
		core.NewCloseParen(),
	})
	for _, d := range diags {
		if d.Severity == core.SeverityError {
			t.Errorf("disjunct poly dispatch errored: %v", d)
		}
	}
}
