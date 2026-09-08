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
	"strings"
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
	if _, ok := r.Check.MethodShapeMember(carrier.ID); !ok {
		done()
		t.Fatal("test premise: the delegation member must be annotated")
	}
	return carrier, done
}

// zzmsRecorder is the minimal armed-recorder double: core.TheInactiveEmit
// supplies every no-op; only the methods the method-shape models consult
// are overridden. Suspend honours nesting the way the models use it
// (tryMemberFnArrivalDispatch runs its CarrierResults probe suspended).
type zzmsDynCall struct {
	fn   core.Value
	args []core.Value
	outs []core.Value
	word string
}

type zzmsRecorder struct {
	core.EmitRecorder
	active       bool
	suspended    bool
	suspendCalls int
	reasons      []string
	dynOK        bool
	dynCalls     []zzmsDynCall
	members      map[string]core.Value
	defReads     map[string]string
}

func newZZMSRecorder() *zzmsRecorder {
	return &zzmsRecorder{EmitRecorder: core.TheInactiveEmit, active: true, dynOK: true}
}

func (z *zzmsRecorder) Active() bool       { return z.active && !z.suspended }
func (z *zzmsRecorder) SuspendedNow() bool { return z.suspended }
func (z *zzmsRecorder) Suspend() func() {
	z.suspendCalls++
	prev := z.suspended
	z.suspended = true
	return func() { z.suspended = prev }
}
func (z *zzmsRecorder) MarkUncompilable(reason string) { z.reasons = append(z.reasons, reason) }
func (z *zzmsRecorder) RecordDynMethod(fn core.Value, args, outs []core.Value, word string, _ core.SrcPos) bool {
	z.dynCalls = append(z.dynCalls, zzmsDynCall{fn: fn, args: args, outs: outs, word: word})
	return z.dynOK
}
func (z *zzmsRecorder) MemberFnReadValue(id string) (core.Value, bool) {
	v, ok := z.members[id]
	return v, ok
}
func (z *zzmsRecorder) DefReadName(id string) (string, bool) {
	n, ok := z.defReads[id]
	return n, ok
}

// zzmsSeamRecordOutcome routes the S3 RecordOutcome slot to
// TryRecordMethodApply — byte-for-byte what the compiler's installed
// recordDispatchOutcome does first (compiler_dispatch_record.go), so the
// PendingMethodApply seam consummates in this compiler-less build. The
// returned func restores the inactive default.
func zzmsSeamRecordOutcome() func() {
	saved := DispatchBraid.RecordOutcome
	DispatchBraid.RecordOutcome = func(r *core.Registry, word string, _ *core.Signature, args, out []core.Value, pos core.SrcPos, _ *core.Registry) {
		TryRecordMethodApply(r, word, args, out, pos)
	}
	return func() { DispatchBraid.RecordOutcome = saved }
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
	if !out.Dynamic || !out.Carrier || !out.Parent.Equal(core.TAny) {
		t.Errorf("the collapse result must be dynamic(Any), got %v", out)
	}
	if !core.IsEnd(e.Tape.At(1)) {
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
	if e.Tape.Len() != 1 || !core.IsEnd(e.Tape.At(0)) {
		t.Fatalf("a 0-return method must collapse to NO values, tape len = %d", e.Tape.Len())
	}
	if len(rfArgs) != 1 {
		t.Fatalf("the arity resolver must hand the window args to ReturnsFn, got %v", rfArgs)
	}
	if s, err := core.AsString(rfArgs[0]); err != nil || s != "req" {
		t.Errorf("ReturnsFn arg = %v, want the window's string", rfArgs[0])
	}
}

// The collapse declines — carrier and window left untouched — for a
// non-inert window, for a window whose args match NO overload, and on a
// (suspended-recorder) compile pass.
func TestZZMSPlainCheckCollapseDeclines(t *testing.T) {
	newFix := func(t *testing.T) (*core.Registry, core.Value, func()) {
		r := zzmsReg(t)
		zzmsRegisterInner(t, r, "zzmsadd", core.Signature{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
			BarrierPos: core.BarrierAllForward, Impl: zzmsGoImpl(),
		})
		member := zzmsMember("zzmsadd", r, core.TInteger)
		carrier, done := zzmsAnnotate(t, r, member)
		return r, carrier, done
	}

	// A word inside the statement window: the interpreter could dispatch
	// through it, so the whole model declines.
	r1, c1, done1 := newFix(t)
	defer done1()
	e1 := zzmsEngine(r1, []core.Value{c1, core.NewWord("zzmsw"), core.NewEnd()})
	if TryShapedMethodDispatch(e1, 0) {
		t.Error("a non-inert window must decline the collapse")
	}
	if e1.Tape.Len() != 3 || e1.Tape.At(0).ID != c1.ID {
		t.Error("a declined collapse must leave the tape untouched")
	}

	// An inert window the member's overloads cannot claim (String arg
	// against the Integer sig): the engine's own matcher declines.
	r2, c2, done2 := newFix(t)
	defer done2()
	e2 := zzmsEngine(r2, []core.Value{c2, core.NewString("nope"), core.NewEnd()})
	if TryShapedMethodDispatch(e2, 0) {
		t.Error("an unmatchable window must decline the collapse")
	}

	// Compiling (suspended compile pass, recorder inactive): the collapse
	// is gated OFF so the [dyn, args] residual stays byte-identical.
	r3, c3, done3 := newFix(t)
	defer done3()
	r3.Check.Compiling = true
	e3 := zzmsEngine(r3, []core.Value{c3, core.NewInteger(5), core.NewEnd()})
	if TryShapedMethodDispatch(e3, 0) {
		t.Error("the collapse must not run while Compiling")
	}
	if e3.Tape.Len() != 3 {
		t.Error("a compiling-pass decline must leave the tape untouched")
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

// The full three-step consummation: the model dispatches through
// CarrierResults, the outcome seam consumes the PendingMethodApply into
// RecordDynMethod, and the tape splices to the modelled results.
func TestZZMSCompilePassModelRecordsDynMethod(t *testing.T) {
	restore := zzmsSeamRecordOutcome()
	defer restore()

	r := zzmsReg(t)
	zzmsRegisterInner(t, r, "zzmsadd", core.Signature{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
		BarrierPos: core.BarrierAllForward, Impl: zzmsGoImpl(),
	})
	member := zzmsMember("zzmsadd", r, core.TInteger)
	carrier, done := zzmsAnnotate(t, r, member)
	defer done()
	rec := newZZMSRecorder()
	r.Check.Emit = rec
	r.Check.Compiling = true

	e := zzmsEngine(r, []core.Value{carrier, core.NewInteger(5), core.NewEnd()})
	if !TryShapedMethodDispatch(e, 0) {
		t.Fatal("the compile-pass model must consume a modelable apply")
	}
	if r.Check.PendingMethodApply != nil {
		t.Error("the outcome seam must consume the pending apply")
	}
	if len(rec.dynCalls) != 1 {
		t.Fatalf("exactly one RecordDynMethod event, got %d", len(rec.dynCalls))
	}
	call := rec.dynCalls[0]
	if call.word != "zzmsadd" {
		t.Errorf("recorded word = %q, want the member's inner name", call.word)
	}
	if call.fn.ID != carrier.ID {
		t.Error("the event's fn operand must be the dot-read carrier (the runtime value)")
	}
	if len(call.args) != 1 || len(call.outs) != 1 {
		t.Errorf("recorded arity = %d args / %d outs, want 1/1", len(call.args), len(call.outs))
	}
	if e.Tape.Len() != 2 {
		t.Fatalf("the model must splice carrier+window to the results, tape len = %d", e.Tape.Len())
	}
	out := e.Tape.At(0)
	if !out.Carrier || !out.Parent.Equal(core.TInteger) {
		t.Errorf("the modelled result must be the declared Integer carrier, got %v", out)
	}
	if len(rec.reasons) != 0 {
		t.Errorf("a clean model must not refuse, got %v", rec.reasons)
	}
}

// The seam's operand-provenance refusal: RecordDynMethod declines (the
// synthetic carrier has no compiled home) and TryRecordMethodApply marks
// the program uncompilable — still consuming the pending apply.
func TestZZMSCompilePassRecordDeclineMarksUncompilable(t *testing.T) {
	restore := zzmsSeamRecordOutcome()
	defer restore()

	r := zzmsReg(t)
	zzmsRegisterInner(t, r, "zzmsadd", core.Signature{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
		BarrierPos: core.BarrierAllForward, Impl: zzmsGoImpl(),
	})
	member := zzmsMember("zzmsadd", r, core.TInteger)
	carrier, done := zzmsAnnotate(t, r, member)
	defer done()
	rec := newZZMSRecorder()
	rec.dynOK = false
	r.Check.Emit = rec
	r.Check.Compiling = true

	e := zzmsEngine(r, []core.Value{carrier, core.NewInteger(5), core.NewEnd()})
	if !TryShapedMethodDispatch(e, 0) {
		t.Fatal("the model still consumes the dispatch; only the recording refuses")
	}
	if len(rec.reasons) != 1 ||
		!strings.Contains(rec.reasons[0], "operand of unknown provenance at zzmsadd") {
		t.Errorf("want the operand-provenance refusal, got %v", rec.reasons)
	}
	if r.Check.PendingMethodApply != nil {
		t.Error("the pending apply must be consumed even when recording refuses")
	}
}

// Without the compiler's seam installed (the inactive RecordOutcome slot)
// the pending apply survives CarrierResults, and the model declines
// wholesale — the carrier keeps today's paths, the tape is untouched.
func TestZZMSCompilePassUnconsumedPendingDeclines(t *testing.T) {
	r := zzmsReg(t)
	zzmsRegisterInner(t, r, "zzmsadd", core.Signature{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
		BarrierPos: core.BarrierAllForward, Impl: zzmsGoImpl(),
	})
	member := zzmsMember("zzmsadd", r, core.TInteger)
	carrier, done := zzmsAnnotate(t, r, member)
	defer done()
	rec := newZZMSRecorder()
	r.Check.Emit = rec

	e := zzmsEngine(r, []core.Value{carrier, core.NewInteger(5), core.NewEnd()})
	if TryShapedMethodDispatch(e, 0) {
		t.Error("an unconsumed pending apply must decline the model")
	}
	if r.Check.PendingMethodApply != nil {
		t.Error("the decline must clear the pending apply")
	}
	if e.Tape.Len() != 3 || e.Tape.At(0).ID != carrier.ID {
		t.Error("the declined model must leave the tape untouched")
	}
}

// The guard-owned decline: an annotated genuine-0-arg member whose landing
// the window model cannot claim (a NoEvalArgs inner 0-arg sig) REFUSES —
// the read guard was skipped for the annotated read, so the landing owns
// the miscompile-E refusal.
func TestZZMSCompilePassZeroArgGuardOwnedDecline(t *testing.T) {
	r := zzmsReg(t)
	zzmsRegisterInner(t, r, "zzms0g", core.Signature{
		Returns: []*core.Type{core.TAny}, BarrierPos: core.BarrierAllForward,
		Impl: zzmsGoImpl(), NoEvalArgs: map[int]bool{0: true},
	})
	member := zzmsMember("zzms0g", r) // genuine 0-arg delegation wrapper
	carrier, done := zzmsAnnotate(t, r, member)
	defer done()
	rec := newZZMSRecorder()
	r.Check.Emit = rec

	e := zzmsEngine(r, []core.Value{carrier, core.NewEnd()})
	if TryShapedMethodDispatch(e, 0) {
		t.Error("an unmodelable 0-arg landing must not be consumed")
	}
	if len(rec.reasons) != 1 ||
		!strings.Contains(rec.reasons[0], "shaped 0-arg method landing not modelable at zzms0g") {
		t.Errorf("want the guard-owned 0-arg refusal, got %v", rec.reasons)
	}
}

// A window decline on a NON-0-arg member under the armed recorder is a
// silent per-carrier decline — no refusal, today's paths keep the carrier.
func TestZZMSCompilePassWindowDeclineNonZeroArgSilent(t *testing.T) {
	r := zzmsReg(t)
	zzmsRegisterInner(t, r, "zzmsadd", core.Signature{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
		BarrierPos: core.BarrierAllForward, Impl: zzmsGoImpl(),
	})
	member := zzmsMember("zzmsadd", r, core.TInteger)
	carrier, done := zzmsAnnotate(t, r, member)
	defer done()
	rec := newZZMSRecorder()
	r.Check.Emit = rec

	e := zzmsEngine(r, []core.Value{carrier, core.NewWord("zzmsw"), core.NewEnd()})
	if TryShapedMethodDispatch(e, 0) {
		t.Error("a non-inert window must decline the compile-pass model")
	}
	if len(rec.reasons) != 0 {
		t.Errorf("a 1-arg member's window decline must not refuse, got %v", rec.reasons)
	}
}

// ─── TryRecordMethodApply (direct seam arms) ─────────────────────────

func TestZZMSTryRecordMethodApplyNoPending(t *testing.T) {
	r := zzmsReg(t)
	if TryRecordMethodApply(r, "zzmsany", nil, nil, core.SrcPos{}) {
		t.Error("no pending apply: the specialist must fall through")
	}
}

// ─── tryDynamicFnValueDispatch ───────────────────────────────────────

func TestZZMSDynamicFnValueDispatch(t *testing.T) {
	// Success: a dynamic Function-bound carrier with an inert forward
	// window collapses window and all to a single dynamic(Any).
	r := zzmsReg(t)
	e := zzmsEngine(r, []core.Value{
		core.NewDynamicCarrier(core.TFunction),
		core.NewInteger(3), core.NewString("x"), core.NewEnd(),
	})
	if !tryDynamicFnValueDispatch(e, 0) {
		t.Fatal("the dynamic-fn model must consume an inert forward window")
	}
	if e.Tape.Len() != 2 {
		t.Fatalf("the collapse must consume the whole window, tape len = %d", e.Tape.Len())
	}
	out := e.Tape.At(0)
	if !out.Dynamic || !out.Carrier || !out.Parent.Equal(core.TAny) {
		t.Errorf("the collapse result must be dynamic(Any), got %v", out)
	}

	// No args: a bare dynamic fn value stays data in both engines.
	e2 := zzmsEngine(r, []core.Value{core.NewDynamicCarrier(core.TFunction), core.NewEnd()})
	if tryDynamicFnValueDispatch(e2, 0) {
		t.Error("an empty window must decline (the value stays data)")
	}

	// Compile pass: the model is plain-check ONLY — the residual must stay
	// for resolveDynamicApply.
	r3 := zzmsReg(t)
	r3.Check.Compiling = true
	e3 := zzmsEngine(r3, []core.Value{
		core.NewDynamicCarrier(core.TFunction), core.NewInteger(3), core.NewEnd(),
	})
	if tryDynamicFnValueDispatch(e3, 0) {
		t.Error("the dynamic-fn collapse must not run while Compiling")
	}
	if e3.Tape.Len() != 3 {
		t.Error("the compile-pass decline must leave the tape untouched")
	}
}

// ─── tryMemberFnArrivalDispatch ──────────────────────────────────────

// zzmsArrivalSig is the modelable member-fn class: one plain-param,
// single-return, boru-bodied signature.
func zzmsArrivalSig() core.Signature {
	return core.Signature{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
		BarrierPos: core.BarrierAllForward,
		Impl:       core.Boru([]core.Value{core.NewWord("zzmsbody")}),
	}
}

// zzmsArrivalFix arms a pass with the recorder double and a member-read
// annotation for a fresh dynamic carrier, returning both.
func zzmsArrivalFix(t *testing.T, member core.Value) (*core.Registry, *zzmsRecorder, core.Value, func()) {
	t.Helper()
	r := zzmsReg(t)
	done := r.Check.Begin()
	rec := newZZMSRecorder()
	r.Check.Emit = rec
	carrier := core.NewDynamicCarrier(core.TAny)
	rec.members = map[string]core.Value{carrier.ID: member}
	return r, rec, carrier, done
}

// The arrival-apply success: the single signature's whole arity of inert
// tokens follows the pinpointed member-read carrier — the model probes
// SUSPENDED, records the guarded dyn-method event, and splices the arity
// window (NOT the statement: the trailing word survives).
func TestZZMSMemberFnArrivalDispatchSuccess(t *testing.T) {
	member := core.NewFunction(core.FnDefInfo{Name: "zzmsarr", Signatures: []core.Signature{zzmsArrivalSig()}})
	r, rec, carrier, done := zzmsArrivalFix(t, member)
	defer done()

	e := zzmsEngine(r, []core.Value{
		carrier, core.NewInteger(21), core.NewWord("zzmseq"), core.NewInteger(42), core.NewEnd(),
	})
	if !tryMemberFnArrivalDispatch(e, 0) {
		t.Fatal("the arrival model must consume a filled arity window")
	}
	if rec.suspendCalls != 1 || rec.suspended {
		t.Errorf("the model must probe suspended and resume (suspends=%d, suspended=%v)",
			rec.suspendCalls, rec.suspended)
	}
	if len(rec.dynCalls) != 1 {
		t.Fatalf("exactly one RecordDynMethod event, got %d", len(rec.dynCalls))
	}
	call := rec.dynCalls[0]
	if call.word != "zzmsarr" || call.fn.ID != carrier.ID {
		t.Errorf("event = %q over %q, want the member name over the read carrier", call.word, call.fn.ID)
	}
	if len(call.args) != 1 || len(call.outs) != 1 {
		t.Errorf("recorded arity = %d args / %d outs, want 1/1", len(call.args), len(call.outs))
	}
	if e.Tape.Len() != 4 {
		t.Fatalf("the splice must claim the ARITY window only, tape len = %d", e.Tape.Len())
	}
	out := e.Tape.At(0)
	if !out.Carrier || out.Dynamic || !out.Parent.Equal(core.TInteger) {
		t.Errorf("the modelled out must be the declared Integer carrier, got %v", out)
	}
	if !core.IsWord(e.Tape.At(1)) {
		t.Error("the token after the arity window belongs to the NEXT dispatch")
	}
}

// Every screening decline leaves the tape untouched and records nothing.
func TestZZMSMemberFnArrivalDispatchDeclines(t *testing.T) {
	plain := zzmsArrivalSig()
	inertTail := func() []core.Value {
		return []core.Value{core.NewInteger(21), core.NewEnd()}
	}
	cases := []struct {
		name   string
		member core.Value
		tape   func(carrier core.Value) []core.Value
	}{
		{"unnamed member", core.NewFunction(core.FnDefInfo{Signatures: []core.Signature{plain}}),
			func(c core.Value) []core.Value { return append([]core.Value{c}, inertTail()...) }},
		{"anonymous member", core.NewFunction(core.FnDefInfo{Name: "zzmsan", Anonymous: true, Signatures: []core.Signature{plain}}),
			func(c core.Value) []core.Value { return append([]core.Value{c}, inertTail()...) }},
		{"multi-overload", core.NewFunction(core.FnDefInfo{Name: "zzmsmo", Signatures: []core.Signature{
			{Fallback: true}, plain, plain,
		}}), func(c core.Value) []core.Value { return append([]core.Value{c}, inertTail()...) }},
		{"only fallback", core.NewFunction(core.FnDefInfo{Name: "zzmsfb", Signatures: []core.Signature{
			{Fallback: true},
		}}), func(c core.Value) []core.Value { return append([]core.Value{c}, inertTail()...) }},
		{"no boru body", core.NewFunction(core.FnDefInfo{Name: "zzmsgo", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
			BarrierPos: core.BarrierAllForward, Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return nil, nil
			}),
		}}}), func(c core.Value) []core.Value { return append([]core.Value{c}, inertTail()...) }},
		{"quote args", core.NewFunction(core.FnDefInfo{Name: "zzmsqa", Signatures: []core.Signature{{
			Args: []*core.Type{core.TAtom}, QuoteArgs: map[int]bool{0: true},
			Returns:    []*core.Type{core.TInteger},
			BarrierPos: core.BarrierAllForward,
			Impl:       core.Boru([]core.Value{core.NewWord("zzmsbody")}),
		}}}), func(c core.Value) []core.Value { return append([]core.Value{c}, inertTail()...) }},
		{"two declared returns", core.NewFunction(core.FnDefInfo{Name: "zzmsr2", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger, core.TInteger},
			BarrierPos: core.BarrierAllForward,
			Impl:       core.Boru([]core.Value{core.NewWord("zzmsbody")}),
		}}}), func(c core.Value) []core.Value { return append([]core.Value{c}, inertTail()...) }},
		{"window past tape end", core.NewFunction(core.FnDefInfo{Name: "zzmste", Signatures: []core.Signature{plain}}),
			func(c core.Value) []core.Value { return []core.Value{c} }},
		{"boundary inside window", core.NewFunction(core.FnDefInfo{Name: "zzmsbd", Signatures: []core.Signature{plain}}),
			func(c core.Value) []core.Value { return []core.Value{c, core.NewEnd()} }},
		{"non-fixed window token", core.NewFunction(core.FnDefInfo{Name: "zzmsnf", Signatures: []core.Signature{plain}}),
			func(c core.Value) []core.Value { return []core.Value{c, core.NewWord("zzmsw"), core.NewEnd()} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, rec, carrier, done := zzmsArrivalFix(t, c.member)
			defer done()
			tape := c.tape(carrier)
			e := zzmsEngine(r, tape)
			if tryMemberFnArrivalDispatch(e, 0) {
				t.Error("the screen must decline this member/window shape")
			}
			if e.Tape.Len() != len(tape) {
				t.Error("a declined arrival must leave the tape untouched")
			}
			if len(rec.dynCalls) != 0 {
				t.Error("a declined arrival must record nothing")
			}
		})
	}

	// A non-dynamic value at the pointer declines before the member lookup.
	r, rec, _, done := zzmsArrivalFix(t, core.NewFunction(core.FnDefInfo{Name: "zzmsnd", Signatures: []core.Signature{plain}}))
	defer done()
	e := zzmsEngine(r, []core.Value{core.NewInteger(1), core.NewEnd()})
	if tryMemberFnArrivalDispatch(e, 0) {
		t.Error("a non-dynamic pointer value must decline")
	}

	// A dynamic carrier with NO member-read annotation declines.
	rec.members = map[string]core.Value{}
	c2 := core.NewDynamicCarrier(core.TAny)
	e2 := zzmsEngine(r, []core.Value{c2, core.NewInteger(21), core.NewEnd()})
	if tryMemberFnArrivalDispatch(e2, 0) {
		t.Error("an unannotated carrier must decline")
	}
}

// Fault injection on the two recorder-facing tails: a probe producing a
// non-single result, and a RecordDynMethod refusal — both decline after
// resuming, leaving the tape untouched.
func TestZZMSMemberFnArrivalDispatchRecorderFaults(t *testing.T) {
	// CarrierResults produces TWO outs (a crafted ReturnsFn against the
	// declared single return): the model declines.
	twoOut := zzmsArrivalSig()
	twoOut.ReturnsFn = func(_ []core.Value, _ *core.Registry) []core.Value {
		return []core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TString)}
	}
	m1 := core.NewFunction(core.FnDefInfo{Name: "zzmsf2", Signatures: []core.Signature{twoOut}})
	r1, rec1, c1, done1 := zzmsArrivalFix(t, m1)
	defer done1()
	e1 := zzmsEngine(r1, []core.Value{c1, core.NewInteger(21), core.NewEnd()})
	if tryMemberFnArrivalDispatch(e1, 0) {
		t.Error("a non-single probe result must decline the model")
	}
	if e1.Tape.Len() != 3 || rec1.suspended {
		t.Error("the decline must resume the recorder and leave the tape untouched")
	}

	// RecordDynMethod refuses: the model declines without splicing.
	m2 := core.NewFunction(core.FnDefInfo{Name: "zzmsfr", Signatures: []core.Signature{zzmsArrivalSig()}})
	r2, rec2, c2, done2 := zzmsArrivalFix(t, m2)
	defer done2()
	rec2.dynOK = false
	e2 := zzmsEngine(r2, []core.Value{c2, core.NewInteger(21), core.NewEnd()})
	if tryMemberFnArrivalDispatch(e2, 0) {
		t.Error("a refused recording must decline the model")
	}
	if e2.Tape.Len() != 3 {
		t.Error("a refused recording must leave the tape untouched")
	}
}
