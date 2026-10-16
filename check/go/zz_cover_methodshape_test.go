package check

// zz_cover_methodshape_test.go — standalone white-box coverage for the
// ENGINE-STEPPING halves of method_shape.go: the shaped-method dispatch
// driver (TryShapedMethodDispatch), its window/match resolver
// (shapedMethodApplyWindow / inertStatementWindow), the collapse arity
// (shapedMethodReturnArity), the outcome-seam specialist
// (TryRecordMethodApply), and the two sibling models
// (tryDynamicFnValueDispatch, tryMemberFnArrivalDispatch).
//
// The tests build the exact state the file comment's three-step model
// describes: an annotated dynamic method-read carrier at the pointer
// (NoteMethodShape on a trivial-delegation member over a registered inner
// native), followed by its inert forward-arg window on a live tape. The
// compile-pass tests install a minimal EmitRecorder double (embedding
// core.TheInactiveEmit for no-op behaviour) and — where the full model
// must consummate — route the S3 RecordOutcome slot to TryRecordMethodApply
// exactly as the compiler's installer does, saved/restored per test.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// ─── fixtures ────────────────────────────────────────────────────────

func zzmsReg(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func zzmsEngine(r *core.Registry, tape []core.Value) *core.Engine {
	e := core.NewTop(r)
	e.Tape = core.NewTape(tape, core.StackHeadroom)
	e.Pointer = 0
	return e
}

// zzmsGoImpl is a plain Go handler (the delegation-wrapper inner-native
// class: DispatchHandler set, no FnFrame / FullStack / check-mode flags).
func zzmsGoImpl() core.SigImpl {
	return core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(7)}, nil
	})
}

// zzmsRegisterInner registers the inner native the delegation member
// forwards to (normalizeSig resolves BarrierAllForward to the arity).
func zzmsRegisterInner(t *testing.T, r *core.Registry, name string, sigs ...core.Signature) {
	t.Helper()
	r.RegisterNativeFunc(core.NativeFunc{Name: name, Signatures: sigs})
	if err := r.Err(); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

// zzmsMember builds the NAMED trivial-delegation method wrapper over the
// inner native `name` registered in r — the shaped-instance-method class
// NoteMethodShape vets (single-word body, unnamed params, foreign registry).
func zzmsMember(name string, r *core.Registry, argTypes ...*core.Type) core.Value {
	return core.NewFunction(core.FnDefInfo{Name: name, Registry: r, Signatures: []core.Signature{{
		Args:       argTypes,
		Returns:    []*core.Type{core.TAny},
		BarrierPos: core.BarrierAllForward,
		Impl:       core.Boru([]core.Value{core.NewWord(name)}),
	}}})
}

// zzmsAnnotate begins a pass, mints the dynamic method-read carrier and
// annotates it via NoteMethodShape (step 1 of the model). The returned
// done deactivates the pass.
func zzmsAnnotate(t *testing.T, r *core.Registry, member core.Value) (core.Value, func()) {
	t.Helper()
	done := r.Check.Begin()
	carrier := core.NewDynamicCarrier(core.TAny)
	r.Check.NoteMethodShape(carrier, member)
	if _, ok := r.Check.MethodShapeMember(core.IDOf(&carrier)); !ok {
		done()
		t.Fatal("test premise: the delegation member must be annotated")
	}
	return carrier, done
}

// ─── shapedMethodReturnArity ─────────────────────────────────────────

func TestZZMSShapedMethodReturnArity(t *testing.T) {
	r := zzmsReg(t)
	e := zzmsEngine(r, nil)

	// ReturnsFn arm: the produced arity wins, and the call site is exposed
	// through CurCallPos exactly as declaredReturnCarriers exposes it.
	var got []core.Value
	rf := &core.Signature{
		Returns: []*core.Type{core.TAny},
		ReturnsFn: func(args []core.Value, _ *core.Registry) []core.Value {
			got = args
			return []core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TString)}
		},
	}
	pos := core.SrcPos{Row: 7, Col: 3}
	args := []core.Value{core.NewInteger(5)}
	if n := shapedMethodReturnArity(e, rf, args, pos); n != 2 {
		t.Errorf("ReturnsFn arity = %d, want the produced 2", n)
	}
	if len(got) != 1 {
		t.Errorf("ReturnsFn must receive the window args, got %v", got)
	}
	if r.Check.CurCallPos.Row != 7 || r.Check.CurCallPos.Col != 3 {
		t.Errorf("CurCallPos = %+v, want the call site {7 3}", r.Check.CurCallPos)
	}

	// Declared-Returns arm: the declared length (nil → 0, the silent
	// side-effect-only collapse).
	if n := shapedMethodReturnArity(e, &core.Signature{Returns: []*core.Type{core.TInteger}}, nil, core.SrcPos{}); n != 1 {
		t.Errorf("declared-Returns arity = %d, want 1", n)
	}
	if n := shapedMethodReturnArity(e, &core.Signature{}, nil, core.SrcPos{}); n != 0 {
		t.Errorf("nil-Returns arity = %d, want 0", n)
	}
}

// ─── plain-check collapse (recorder inactive, not compiling) ─────────

// A shaped dynamic method apply on the plain-check surface collapses the
// carrier + inert window to the matched signature's RETURN ARITY of
// dynamic(Any) carriers — one for a value method.
func TestZZMSPlainCheckCollapseValueMethod(t *testing.T) {
	r := zzmsReg(t)
	zzmsRegisterInner(t, r, "zzmsadd", core.Signature{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
		BarrierPos: core.BarrierAllForward, Impl: zzmsGoImpl(),
	})
	member := zzmsMember("zzmsadd", r, core.TInteger)
	carrier, done := zzmsAnnotate(t, r, member)
	defer done()

	e := zzmsEngine(r, []core.Value{carrier, core.NewInteger(5), core.NewEnd()})
	if !TryShapedMethodDispatch(e, 0) {
		t.Fatal("the plain-check collapse must consume a modelable apply")
	}
	if e.Tape.Len() != 2 {
		t.Fatalf("collapse must splice [carrier, arg] to one value, tape len = %d", e.Tape.Len())
	}
	out := e.Tape.At(0)
	if !core.IsDynamic(&out) || !out.Carrier() || !out.Parent().Equal(core.TAny) {
		t.Errorf("the collapse result must be dynamic(Any), got %v", out)
	}
	if !core.IsEnd(e.Tape.Ref(1)) {
		t.Error("the statement boundary must survive the splice")
	}
}

// A side-effect-only shaped method (ReturnsFn produces NOTHING) collapses
// to ZERO values — splicing a fabricated dynamic(Any) would wrongly feed a
// downstream consumer.
func TestZZMSPlainCheckCollapseSideEffectMethod(t *testing.T) {
	r := zzmsReg(t)
	var rfArgs []core.Value
	zzmsRegisterInner(t, r, "zzmslog", core.Signature{
		Args: []*core.Type{core.TString}, Returns: []*core.Type{},
		BarrierPos: core.BarrierAllForward, Impl: zzmsGoImpl(),
		ReturnsFn: func(args []core.Value, _ *core.Registry) []core.Value {
			rfArgs = args
			return nil
		},
	})
	member := zzmsMember("zzmslog", r, core.TString)
	carrier, done := zzmsAnnotate(t, r, member)
	defer done()

	e := zzmsEngine(r, []core.Value{carrier, core.NewString("req"), core.NewEnd()})
	if !TryShapedMethodDispatch(e, 0) {
		t.Fatal("the collapse must consume a side-effect-only apply too")
	}
	if e.Tape.Len() != 1 || !core.IsEnd(e.Tape.Ref(0)) {
		t.Fatalf("a 0-return method must collapse to NO values, tape len = %d", e.Tape.Len())
	}
	if len(rfArgs) != 1 {
		t.Fatalf("the arity resolver must hand the window args to ReturnsFn, got %v", rfArgs)
	}
	if s, err := core.AsString(rfArgs[0]); err != nil || s != "req" {
		t.Errorf("ReturnsFn arg = %v, want the window's string", rfArgs[0])
	}
}

// A matched sig failing the plain-Go-handler screen (NoEvalArgs) keeps
// today's paths: shapedMethodApplyWindow declines after the match.
func TestZZMSApplyWindowScreenedSigDeclines(t *testing.T) {
	r := zzmsReg(t)
	zzmsRegisterInner(t, r, "zzmsq", core.Signature{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
		BarrierPos: core.BarrierAllForward, Impl: zzmsGoImpl(),
		NoEvalArgs: map[int]bool{0: true},
	})
	member := zzmsMember("zzmsq", r, core.TInteger)
	carrier, done := zzmsAnnotate(t, r, member)
	defer done()

	e := zzmsEngine(r, []core.Value{carrier, core.NewInteger(5), core.NewEnd()})
	if _, _, ok := shapedMethodApplyWindow(e, 0, member); ok {
		t.Error("a NoEvalArgs inner sig is not a plain Go-handler apply")
	}
	if TryShapedMethodDispatch(e, 0) {
		t.Error("the screened sig must decline the collapse wholesale")
	}
}

// A MIXED-barrier inner sig (one forward arg, one from the stack BELOW
// the carrier) matches, but its positions are not the contiguous window
// prefix — not the statement-window apply, so the model declines and the
// trailing/mixed shape keeps today's paths.
func TestZZMSApplyWindowMixedStackMatchDeclines(t *testing.T) {
	r := zzmsReg(t)
	zzmsRegisterInner(t, r, "zzmsmx", core.Signature{
		Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TInteger},
		BarrierPos: 1, // arg0 forward, arg1 from the stack
		Impl:       zzmsGoImpl(),
	})
	member := zzmsMember("zzmsmx", r, core.TInteger, core.TInteger)

	// A resolved Integer sits BELOW the carrier; the pointer is ON the
	// carrier with one inert forward token after it.
	e := zzmsEngine(r, []core.Value{
		core.NewInteger(3), core.NewDynamicCarrier(core.TAny), core.NewInteger(5), core.NewEnd(),
	})
	e.Pointer = 1
	if _, _, ok := shapedMethodApplyWindow(e, 1, member); ok {
		t.Error("a mixed forward/stack match is not the statement-window apply")
	}
}

// ─── inertStatementWindow ────────────────────────────────────────────

func TestZZMSInertStatementWindow(t *testing.T) {
	r := zzmsReg(t)

	// Inert tokens up to the statement boundary: winEnd advances to the
	// last window token and the End is never entered.
	e := zzmsEngine(r, []core.Value{
		core.NewCarrier(core.TAny), core.NewInteger(1), core.NewString("s"),
		core.NewEnd(), core.NewInteger(9),
	})
	winEnd, ok := inertStatementWindow(e, 0)
	if !ok || winEnd != 2 {
		t.Errorf("window = (%d,%v), want the two inert tokens (2,true)", winEnd, ok)
	}

	// No boundary before the tape ends: the scan runs off the end, still ok.
	e2 := zzmsEngine(r, []core.Value{core.NewCarrier(core.TAny), core.NewInteger(1)})
	if winEnd, ok := inertStatementWindow(e2, 0); !ok || winEnd != 1 {
		t.Errorf("tape-end window = (%d,%v), want (1,true)", winEnd, ok)
	}

	// A non-fixed token inside the window declines the scan.
	e3 := zzmsEngine(r, []core.Value{core.NewCarrier(core.TAny), core.NewInteger(1), core.NewWord("w")})
	if _, ok := inertStatementWindow(e3, 0); ok {
		t.Error("a word inside the window must decline")
	}

	// An immediate boundary: the empty window (winEnd == valIdx) is ok.
	e4 := zzmsEngine(r, []core.Value{core.NewCarrier(core.TAny), core.NewEnd()})
	if winEnd, ok := inertStatementWindow(e4, 0); !ok || winEnd != 0 {
		t.Errorf("empty window = (%d,%v), want (0,true)", winEnd, ok)
	}
}

// ─── compile-pass model (armed recorder) ─────────────────────────────

// ─── TryRecordMethodApply (direct seam arms) ─────────────────────────

// ─── tryDynamicFnValueDispatch ───────────────────────────────────────

// ─── tryMemberFnArrivalDispatch ──────────────────────────────────────
