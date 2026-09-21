package check

// White-box coverage tests for the check-mode recovery surface
// (check_recovery.go) and the S3 dispatch braid's private forwarders
// (dispatch_hooks.go). Each test drives a documented behaviour:
//
//   - the inactive DispatchBraid defaults DECLINE (no-op / false /
//     nil-interface), reached through the private forwarders;
//   - the recovery helpers (concreteEvalOnce, SpliceFnValueCheckResult,
//     the check-state sharing brackets, the speculative-commit advisory,
//     the paren fn-carrier collapse, the fallback-position walk, the
//     surface-shape typing, the assume-sig recovery) model exactly what
//     their doc comments state;
//   - the two compile-mode compile failures (DeclineForwardStackDrift,
//     declineStrandedMemberFn) mark uncompilable only in their documented
//     hazard shapes, observed through a test EmitRecorder stub embedding
//     the inactive recorder (the same pattern core's own emit-stub tests
//     use).
//
// Where an ARMED answer is needed the tests install a minimal double
// into the DispatchBraid slot for the call's duration, restoring the
// inactive default with defer so TestInactiveDispatchBraid keeps
// pinning the compiler-less configuration.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// --- test doubles ------------------------------------------------------------

// zzRecEmit is an EmitRecorder stub: the inactive recorder with an
// activity switch, a suspend counter, and probes for the calls the
// recovery paths make (MarkUncompilable, MemberFnRead,
// RecordDispatchRematchValues).
type zzRecEmit struct {
	core.EmitRecorder
	activeOn  bool
	suspended int
	uncomp    []string
	member    map[string]bool
	rematchOK bool
	rematched bool
}

func zzNewEmit() *zzRecEmit {
	return &zzRecEmit{EmitRecorder: core.TheInactiveEmit, activeOn: true, member: map[string]bool{}}
}

func (s *zzRecEmit) Active() bool { return s.activeOn && s.suspended == 0 }
func (s *zzRecEmit) Suspend() func() {
	s.suspended++
	return func() { s.suspended-- }
}
func (s *zzRecEmit) SuspendedNow() bool             { return s.suspended > 0 }
func (s *zzRecEmit) MarkUncompilable(reason string) { s.uncomp = append(s.uncomp, reason) }
func (s *zzRecEmit) MemberFnRead(id string) bool    { return s.member[id] }
func (s *zzRecEmit) RecordDispatchRematchValues(string, []core.Value, int, int, core.SrcPos) bool {
	s.rematched = true
	return s.rematchOK
}

func (s *zzRecEmit) hasUncomp(sub string) bool {
	for _, r := range s.uncomp {
		if strings.Contains(r, sub) {
			return true
		}
	}
	return false
}

// zzArmEmit installs the stub as the pass recorder (after Begin, which
// resets Emit to the inactive instance).
func zzArmEmit(r *core.Registry) *zzRecEmit {
	s := zzNewEmit()
	r.Check.Emit = s
	return s
}

// zzPolyPlan is a minimal UserPolyPlan double for the
// CompileUserPolyArms seam.
type zzPolyPlan struct {
	subbed []core.Value
}

func (p *zzPolyPlan) SubstituteJoinedOuts(out []core.Value) { p.subbed = out }
func (p *zzPolyPlan) SigIdx() []int                         { return []int{0} }
func (p *zzPolyPlan) Units() []int                          { return []int{0} }
func (p *zzPolyPlan) Impls() []core.SigImpl                 { return nil }
func (p *zzPolyPlan) Sigs() []core.Signature                { return nil }

// --- dispatch_hooks.go: private forwarders over the inactive defaults -------

// TestZZCoverDispatchForwardersDecline drives the two private
// forwarders TestInactiveDispatchBraid does not reach. With no compiler
// linked the slots hold the named inactive defaults, so each call is
// the documented decline: TryRecordPoly=false, CompileUserPolyArms=nil
// interface. The empty-bodied RecordOutcome default is exercised too
// (it has no statements; the call pins the no-op contract).
func TestZZCoverDispatchForwardersDecline(t *testing.T) {
	if dispatchTryRecordPoly(nil, "zznope", nil, nil, nil, core.SrcPos{}, false, nil, false, nil) {
		t.Error("unarmed TryRecordPoly forwarder must decline (false)")
	}
	if dispatchCompileUserPolyArms(nil, core.TheInactiveEmit, "zznope", nil, nil) != nil {
		t.Error("unarmed CompileUserPolyArms forwarder must decline (nil interface)")
	}
	// The no-op default and its forwarder: nothing to observe beyond
	// "does not panic, records nothing" — the documented inactive decline.
	inactiveDispatchRecordOutcome(nil, "", nil, nil, nil, core.SrcPos{}, nil)
	dispatchRecordOutcome(nil, "", nil, nil, nil, core.SrcPos{}, nil)
}

// --- concreteEvalOnce --------------------------------------------------------

func TestZZCoverConcreteEvalOnce(t *testing.T) {
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	e := core.NewTop(r)
	r.Defs.Push("zzceo", core.NewInteger(42))

	// Success: one concrete residual, computed with check mode OFF.
	got, ok := concreteEvalOnce(e, []core.Value{
		core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2),
	})
	if !ok {
		t.Fatal("cadd 1 2 must concrete-eval to a single residual")
	}
	if n, err := core.AsInteger(got); err != nil || n != 3 {
		t.Errorf("residual = %v (%v), want 3", got, err)
	}
	if !r.Check.Mode {
		t.Error("check mode must be restored after the sub-run")
	}
	if _, bound := r.Defs.Top("zzceo"); !bound {
		t.Error("the def snapshot must be restored")
	}

	// Two residuals: not a single value, the fold declines.
	if _, ok := concreteEvalOnce(e, []core.Value{core.NewInteger(1), core.NewInteger(2)}); ok {
		t.Error("a two-value residual must decline")
	}
	// An erroring run declines too.
	if _, ok := concreteEvalOnce(e, []core.Value{core.NewWord("zz-cover-no-such-word")}); ok {
		t.Error("an erroring sub-run must decline")
	}
}

// --- shareCheckState / shareCheckStateFrom ----------------------------------

func TestZZCoverShareCheckState(t *testing.T) {
	caller := covRegistry(t, nil)
	done := caller.Check.Begin()
	defer done()
	owner := covRegistry(t, nil)
	orig := owner.Check

	e := core.NewTop(caller)
	restore := shareCheckState(e, owner)
	if owner.Check != caller.Check {
		t.Error("sharing must point the module registry's Check at the caller's")
	}
	restore()
	if owner.Check != orig {
		t.Error("restore must put back the owner's own Check state")
	}
}

// --- noteSpeculativeBarrierCommit -------------------------------------------

func TestZZCoverNoteSpeculativeBarrierCommit(t *testing.T) {
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewWord("zzelse")}, core.StackHeadroom)
	e.Pointer = 0

	// Non-speculative commit: advisory stays silent.
	noteSpeculativeBarrierCommit(e, core.ForwardInfo{FuncName: "zzif"})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("a non-speculative commit must emit nothing, got %v", r.Check.Diagnostics)
	}

	// Speculative commit at a statement boundary: info advisory naming
	// the word at the pointer and the planned slot.
	noteSpeculativeBarrierCommit(e, core.ForwardInfo{
		FuncName: "zzif", CollectedArgs: 1, StackArgs: 1,
		Speculative: true, SpeculativeAt: 2,
		Pos: core.SrcPos{Row: 7, Col: 3},
	})
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("want exactly one advisory, got %v", r.Check.Diagnostics)
	}
	d := r.Check.Diagnostics[0]
	if d.Code != "speculative_forward_commit" || d.Word != "zzif" || d.Row != 7 || d.Col != 3 {
		t.Errorf("advisory = %+v", d)
	}
	if !strings.Contains(d.Detail, "`zzelse`") {
		t.Errorf("detail must name the boundary word, got %q", d.Detail)
	}
	if !strings.Contains(d.Detail, "2 argument(s)") || !strings.Contains(d.Detail, "slot 2") {
		t.Errorf("detail must carry the arg count and planned slot, got %q", d.Detail)
	}
}

// --- checkModeParenFnCollapse ------------------------------------------------

func zzCollapseEng(t *testing.T, begin bool, vals ...core.Value) (*core.Engine, func()) {
	t.Helper()
	r := covRegistry(t, nil)
	fin := func() {}
	if begin {
		fin = r.Check.Begin()
	}
	e := core.NewTop(r)
	e.Tape = core.NewTape(vals, core.StackHeadroom)
	return e, fin
}

func zzFnCarrier() core.Value { return core.NewCarrier(core.TFunction) }

func TestZZCoverParenFnCollapseModeOff(t *testing.T) {
	// Outside check mode the collapse is a no-op returning closeIdx.
	e, fin := zzCollapseEng(t, false,
		core.NewOpenParen(), core.NewInteger(1), zzFnCarrier(), core.NewCloseParen())
	defer fin()
	if got := checkModeParenFnCollapse(e, 0, 3); got != 3 {
		t.Errorf("closeIdx = %d, want 3 (untouched outside check mode)", got)
	}
	if e.Tape.Len() != 4 {
		t.Errorf("tape must be untouched, len = %d", e.Tape.Len())
	}
}

func TestZZCoverParenFnCollapseTrailing(t *testing.T) {
	// `( arg comp )` with a trailing fn-typed carrier nets ONE dynamic
	// Any carrier; a non-recordable Mark inside the window is skipped.
	e, fin := zzCollapseEng(t, true,
		core.NewOpenParen(), core.NewMark("zzskip"), core.NewInteger(1),
		zzFnCarrier(), core.NewCloseParen())
	defer fin()
	got := checkModeParenFnCollapse(e, 0, 4)
	if got != 3 {
		t.Fatalf("closeIdx = %d, want 3 (one literal spliced out)", got)
	}
	if e.Tape.Len() != 4 {
		t.Fatalf("tape len = %d, want 4", e.Tape.Len())
	}
	out := e.Tape.At(2)
	if !out.Carrier || !out.Dynamic || !out.Parent.Equal(core.TAny) || out.ID == "" {
		t.Errorf("collapsed window must seat one dynamic Any carrier with an ID, got %#v", out)
	}
	if !core.IsMark(e.Tape.At(1)) {
		t.Error("the non-recordable Mark must survive the collapse")
	}
}

func TestZZCoverParenFnCollapseLeadingOneArg(t *testing.T) {
	// `( g x )` — leading fn carrier over exactly one argument.
	e, fin := zzCollapseEng(t, true,
		core.NewOpenParen(), zzFnCarrier(), core.NewInteger(2), core.NewCloseParen())
	defer fin()
	got := checkModeParenFnCollapse(e, 0, 3)
	if got != 2 {
		t.Fatalf("closeIdx = %d, want 2", got)
	}
	out := e.Tape.At(1)
	if !out.Carrier || !out.Dynamic || !out.Parent.Equal(core.TAny) {
		t.Errorf("leading apply must collapse to one dynamic Any carrier, got %#v", out)
	}
}

func TestZZCoverParenFnCollapseNeitherShape(t *testing.T) {
	// Two plain values: no fn carrier in either admission position, the
	// window stays as the interpreter leaves it.
	e, fin := zzCollapseEng(t, true,
		core.NewOpenParen(), core.NewInteger(1), core.NewInteger(2), core.NewCloseParen())
	defer fin()
	if got := checkModeParenFnCollapse(e, 0, 3); got != 3 {
		t.Errorf("closeIdx = %d, want 3 (no collapse)", got)
	}
	if e.Tape.Len() != 4 {
		t.Errorf("tape must be untouched, len = %d", e.Tape.Len())
	}
}

// --- checkModeFallbackPositions ---------------------------------------------

func TestZZCoverFallbackPositionsSkipNestedGroup(t *testing.T) {
	// A nested forward group after the pointer is entered (depth++) and
	// left (depth--) without its close breaking the walk; only the
	// enclosing close at depth 0 would stop it.
	e := engWithTape(t, []core.Value{
		core.NewWord("zzw"),
		core.NewOpenParen(), core.NewInteger(1), core.NewCloseParen(),
		core.NewInteger(2),
	}, 0)
	got := checkModeFallbackPositions(e, 3)
	if len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Errorf("positions = %v, want [2 4] (walk crosses the nested group)", got)
	}
}

// --- SpliceFnValueCheckResult ------------------------------------------------

func TestZZCoverSpliceFnValueCheckResult(t *testing.T) {
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()

	body := parenBody(core.NewWord("cadd"), core.NewWord("n"), core.NewWord("n"))
	sig := core.FnSig{
		Params:     []core.FnParam{{Name: "n", Type: core.TInteger}},
		Returns:    []*core.Type{core.TInteger},
		Impl:       core.Boru(body),
		BarrierPos: core.BarrierAllForward,
	}
	fnDef := core.FnDefInfo{Name: "zzfnval", Signatures: []core.Signature{sig}}

	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(7), core.NewFunction(fnDef)}, core.StackHeadroom)
	e.Pointer = 1

	if err := SpliceFnValueCheckResult(e, 1, 1, fnDef, &sig, []core.Value{core.NewInteger(7)}); err != nil {
		t.Fatalf("SpliceFnValueCheckResult: %v", err)
	}
	if e.Tape.Len() != 1 {
		t.Fatalf("tape len = %d, want 1 (args + fn literal replaced by the result)", e.Tape.Len())
	}
	out := e.Tape.At(0)
	if !out.Carrier || !out.Parent.Equal(core.TInteger) {
		t.Errorf("declared-return fn value must splice an Integer carrier, got %#v", out)
	}
	if e.Pointer != 0 {
		t.Errorf("pointer = %d, want 0", e.Pointer)
	}
}

// --- DeclineForwardStackDrift -------------------------------------------------

func zzDriftSig() *core.Signature {
	return &core.Signature{Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 1}
}

func zzDriftEng(t *testing.T, vals []core.Value, pointer int) (*core.Engine, *zzRecEmit, func()) {
	t.Helper()
	r := covRegistry(t, nil)
	fin := r.Check.Begin()
	es := zzArmEmit(r)
	e := core.NewTop(r)
	e.Tape = core.NewTape(vals, core.StackHeadroom)
	e.Pointer = pointer
	return e, es, fin
}

func TestZZCoverForwardStackDriftFailsToCompile(t *testing.T) {
	// The documented hazard: dynamic on top, concrete beneath, an atomic
	// literal right after the word — the all-stack match drifts from the
	// interpreter's forward collection, so the compile declines.
	e, es, fin := zzDriftEng(t, []core.Value{
		core.NewInteger(5), core.NewDynamicCarrier(core.TAny),
		core.NewWord("zzadd"), core.NewInteger(1),
	}, 2)
	defer fin()
	DeclineForwardStackDrift(e, zzDriftSig(), []int{0, 1})
	if !es.hasUncomp("forward operand accounting") {
		t.Errorf("drift shape must decline, marks = %v", es.uncomp)
	}
}

func TestZZCoverForwardStackDriftGates(t *testing.T) {
	dyn := func() core.Value { return core.NewDynamicCarrier(core.TAny) }

	// NoEvalArgs (code-body word): never this guard's drift; no compile failure.
	e, es, fin := zzDriftEng(t, []core.Value{
		core.NewInteger(5), dyn(), core.NewWord("zzif"), core.NewInteger(0),
	}, 2)
	defer fin()
	sig := zzDriftSig()
	sig.NoEvalArgs = map[int]bool{1: true}
	DeclineForwardStackDrift(e, sig, []int{0, 1})
	if len(es.uncomp) != 0 {
		t.Errorf("a NoEvalArgs word must not decline, marks = %v", es.uncomp)
	}

	// An out-of-range matched position aborts silently.
	e2, es2, fin2 := zzDriftEng(t, []core.Value{
		core.NewInteger(5), dyn(), core.NewWord("zzadd"), core.NewInteger(1),
	}, 2)
	defer fin2()
	DeclineForwardStackDrift(e2, zzDriftSig(), []int{0, 99})
	if len(es2.uncomp) != 0 {
		t.Errorf("bad position must not decline, marks = %v", es2.uncomp)
	}

	// Concrete on top: the interpreter takes the all-stack match too.
	e3, es3, fin3 := zzDriftEng(t, []core.Value{
		dyn(), core.NewInteger(5), core.NewWord("zzadd"), core.NewInteger(1),
	}, 2)
	defer fin3()
	DeclineForwardStackDrift(e3, zzDriftSig(), []int{0, 1})
	if len(es3.uncomp) != 0 {
		t.Errorf("concrete top must not decline, marks = %v", es3.uncomp)
	}

	// No trailing token at all: nothing to forward-collect.
	e4, es4, fin4 := zzDriftEng(t, []core.Value{
		core.NewInteger(5), dyn(), core.NewWord("zzadd"),
	}, 2)
	defer fin4()
	DeclineForwardStackDrift(e4, zzDriftSig(), []int{0, 1})
	if len(es4.uncomp) != 0 {
		t.Errorf("no trailing token must not decline, marks = %v", es4.uncomp)
	}

	// A structural trailing token (`)`) is NOT a forward operand.
	e5, es5, fin5 := zzDriftEng(t, []core.Value{
		core.NewInteger(5), dyn(), core.NewWord("zzadd"), core.NewCloseParen(),
	}, 2)
	defer fin5()
	DeclineForwardStackDrift(e5, zzDriftSig(), []int{0, 1})
	if len(es5.uncomp) != 0 {
		t.Errorf("a close paren is not a forward operand, marks = %v", es5.uncomp)
	}
}

// --- declineStrandedMemberFn --------------------------------------------------

func TestZZCoverStrandedMemberFn(t *testing.T) {
	memfn := core.NewDynamicCarrier(core.TAny)
	memfn.ID = "zzmemfn1"

	// The hazard: a member-fn read directly beneath the consumed operand
	// (a structural Mark between them is skipped) — decline.
	e, es, fin := zzDriftEng(t, []core.Value{
		memfn, core.NewMark("zzm"), core.NewInteger(21), core.NewWord("zzeq"),
	}, 3)
	defer fin()
	es.member["zzmemfn1"] = true
	declineStrandedMemberFn(e, []int{2})
	if !es.hasUncomp("member fn value auto-applies mid-expression") {
		t.Errorf("stranded member fn must decline, marks = %v", es.uncomp)
	}
}

func TestZZCoverStrandedMemberFnGates(t *testing.T) {
	// Deepest operand at the stack bottom: nothing beneath it.
	e, es, fin := zzDriftEng(t, []core.Value{
		core.NewInteger(21), core.NewWord("zzeq"),
	}, 1)
	defer fin()
	declineStrandedMemberFn(e, []int{0})
	if len(es.uncomp) != 0 {
		t.Errorf("bottom operand must not decline, marks = %v", es.uncomp)
	}

	// A scope boundary (open paren) directly beneath: stop, no compile failure.
	e2, es2, fin2 := zzDriftEng(t, []core.Value{
		core.NewOpenParen(), core.NewInteger(21), core.NewWord("zzeq"),
	}, 2)
	defer fin2()
	declineStrandedMemberFn(e2, []int{1})
	if len(es2.uncomp) != 0 {
		t.Errorf("scope boundary must not decline, marks = %v", es2.uncomp)
	}

	// First data value beneath is NOT a member-fn read: keep compiling.
	e3, es3, fin3 := zzDriftEng(t, []core.Value{
		core.NewInteger(1), core.NewInteger(21), core.NewWord("zzeq"),
	}, 2)
	defer fin3()
	declineStrandedMemberFn(e3, []int{1})
	if len(es3.uncomp) != 0 {
		t.Errorf("a plain value beneath is not the hazard, marks = %v", es3.uncomp)
	}
}

// --- checkModeSurfaceShape ---------------------------------------------------

// zzSurface mints a surface type requiring the named ops, each with the
// contract shape [self:Self -> Integer], and returns its lattice node.
func zzSurface(t *testing.T, r *core.Registry, name string, ops ...string) *core.Type {
	t.Helper()
	info := &core.SurfaceInfo{Required: core.NewOrderedMap(), Conform: map[string]bool{}}
	for _, op := range ops {
		info.Required.Set(op, core.NewFnUndef(core.FnUndefInfo{Sigs: []core.FnSigSpec{{
			Params:  []core.FnParam{{Name: "self", Type: core.TSelf}},
			Returns: []*core.Type{core.TInteger},
		}}}))
	}
	if err := core.InstallType(r, name, core.NewSurfaceType(core.TSurface, info)); err != nil {
		t.Fatalf("InstallType %s: %v", name, err)
	}
	node := r.LookupTypeName(name)
	if node == nil {
		t.Fatalf("minted surface %s not found", name)
	}
	return node
}

func TestZZCoverSurfaceShapeTypesRequiredOp(t *testing.T) {
	r := covRegistry(t, nil)
	node := zzSurface(t, r, "Zzshapea", "zzr-area")
	done := r.Check.Begin()
	defer done()

	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewCarrier(node), core.NewWord("zzr-area")}, core.StackHeadroom)
	e.Pointer = 1

	handled, err := checkModeSurfaceShape(e, core.WordInfo{Name: "zzr-area", ArgCount: -1}, core.SrcPos{Row: 2, Col: 5})
	if err != nil || !handled {
		t.Fatalf("surface-required op must be handled: (%v, %v)", handled, err)
	}
	if e.Tape.Len() != 1 {
		t.Fatalf("tape len = %d, want 1 (word + arg replaced by the shape's return)", e.Tape.Len())
	}
	out := e.Tape.At(0)
	if !out.Carrier || !out.Parent.Equal(core.TInteger) {
		t.Errorf("call must type as the contract's declared return, got %#v", out)
	}
}

func TestZZCoverSurfaceShapeResolvesDefBoundWord(t *testing.T) {
	// A forward position still holding a raw Word resolves through the
	// def stack, so a def-bound surface carrier is visible to the scan.
	r := covRegistry(t, nil)
	node := zzSurface(t, r, "Zzshapeb", "zzr-area")
	done := r.Check.Begin()
	defer done()
	r.Defs.Push("zzsv", core.NewCarrier(node))

	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewWord("zzr-area"), core.NewWord("zzsv")}, core.StackHeadroom)
	e.Pointer = 0

	handled, err := checkModeSurfaceShape(e, core.WordInfo{Name: "zzr-area", ArgCount: -1}, core.SrcPos{})
	if err != nil || !handled {
		t.Fatalf("def-bound surface carrier must be handled: (%v, %v)", handled, err)
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier {
		t.Errorf("tape = %d values, want the single shape-return carrier", e.Tape.Len())
	}
}

func TestZZCoverSurfaceShapeDeclinesWhenOpNotRequired(t *testing.T) {
	// A surface carrier whose contract does NOT require the dispatched
	// word declines: handled=false, nothing spliced.
	r := covRegistry(t, nil)
	node := zzSurface(t, r, "Zzshapec", "zzr-other")
	done := r.Check.Begin()
	defer done()

	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewCarrier(node), core.NewWord("zzr-area")}, core.StackHeadroom)
	e.Pointer = 1

	handled, err := checkModeSurfaceShape(e, core.WordInfo{Name: "zzr-area", ArgCount: -1}, core.SrcPos{})
	if handled || err != nil {
		t.Errorf("op outside the contract must decline: (%v, %v)", handled, err)
	}
	if e.Tape.Len() != 2 {
		t.Errorf("declining must leave the tape untouched, len = %d", e.Tape.Len())
	}
}

// --- checkModeAssumeSig ------------------------------------------------------

// zzGoInt is a trivial Go handler returning one Integer.
func zzGoInt() core.SigImpl {
	return core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(0)}, nil
	})
}

// zzIntCarrierFn is a len-guarded ReturnsFn producing one Integer carrier.
func zzIntCarrierFn(_ []core.Value, _ *core.Registry) []core.Value {
	return []core.Value{core.NewCarrier(core.TInteger)}
}

// zzAssumeReg registers the assume-sig fixture words:
//
//	zzr-poly2  — two Go overloads (Integer→Integer, String→String);
//	zzr-user1  — ONE boru-bodied overload [x:Any -> Integer] with a
//	             ReturnsFn (the single-overload-recoverable shape; its
//	             aggregate gains the synthetic 0-arg Fallback sig);
//	zzr-two    — one Go overload (Integer Integer → Integer), no ReturnsFn;
//	zzr-one    — one Go overload (Integer → Integer) WITH a ReturnsFn;
//	zzr-map1   — one Go overload (Map → Integer);
//	zzr-arity  — a 5-arg and a 1-arg ReturnsFn overload plus a 3-arg
//	             plain overload (the second-pass fallback shapes).
func zzAssumeReg(t *testing.T) *core.Registry {
	t.Helper()
	return covRegistry(t, func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{Name: "zzr-poly2", Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Impl: zzGoInt(), Returns: []*core.Type{core.TInteger}, BarrierPos: -1},
			{Args: []*core.Type{core.TString}, Impl: zzGoInt(), Returns: []*core.Type{core.TString}, BarrierPos: -1},
		}})
		r.RegisterNativeFunc(core.NativeFunc{Name: "zzr-user1", Signatures: []core.Signature{{
			Params:     []core.FnParam{{Name: "x", Type: core.TAny}},
			Returns:    []*core.Type{core.TInteger},
			ReturnsFn:  zzIntCarrierFn,
			Impl:       core.Boru(parenBody(core.NewWord("cadd"), core.NewWord("x"), core.NewWord("x"))),
			BarrierPos: -1,
		}}})
		r.RegisterNativeFunc(core.NativeFunc{Name: "zzr-two", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger}, Impl: zzGoInt(),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}}})
		r.RegisterNativeFunc(core.NativeFunc{Name: "zzr-one", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Impl: zzGoInt(),
			Returns: []*core.Type{core.TInteger}, ReturnsFn: zzIntCarrierFn, BarrierPos: -1,
		}}})
		r.RegisterNativeFunc(core.NativeFunc{Name: "zzr-map1", Signatures: []core.Signature{{
			Args: []*core.Type{core.TMap}, Impl: zzGoInt(),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}}})
		r.RegisterNativeFunc(core.NativeFunc{Name: "zzr-arity", Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger, core.TInteger, core.TInteger, core.TInteger, core.TInteger},
				Impl: zzGoInt(), Returns: []*core.Type{core.TInteger}, ReturnsFn: zzIntCarrierFn, BarrierPos: -1},
			{Args: []*core.Type{core.TInteger, core.TInteger, core.TInteger},
				Impl: zzGoInt(), Returns: []*core.Type{core.TInteger}, BarrierPos: -1},
			{Args: []*core.Type{core.TInteger},
				Impl: zzGoInt(), Returns: []*core.Type{core.TInteger}, ReturnsFn: zzIntCarrierFn, BarrierPos: -1},
		}})
	})
}

// zzAssumeCall drives checkModeAssumeSig for word over the given tape,
// with the word token at `pointer`. The fallback is selected from the
// aggregate by pick (nil = Signatures[0]).
func zzAssumeCall(t *testing.T, r *core.Registry, word string, vals []core.Value, pointer int,
	pick func(*core.Signature) bool) (*core.Engine, *core.FnDefInfo) {
	t.Helper()
	fn := r.Lookup(word)
	if fn == nil {
		t.Fatalf("fixture word %s not registered", word)
	}
	fallback := &fn.Signatures[0]
	if pick != nil {
		found := false
		for i := range fn.Signatures {
			if pick(&fn.Signatures[i]) {
				fallback = &fn.Signatures[i]
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("no fallback sig matched the pick for %s", word)
		}
	}
	e := core.NewTop(r)
	e.Tape = core.NewTape(vals, core.StackHeadroom)
	e.Pointer = pointer
	if err := checkModeAssumeSig(e, core.WordInfo{Name: word, ArgCount: -1}, fn, fallback, core.SrcPos{Row: 1, Col: 1}); err != nil {
		t.Fatalf("checkModeAssumeSig(%s): %v", word, err)
	}
	return e, fn
}

func zzDisjunctIS() core.Value {
	return strictDisjunct(core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString))
}

// hasDiag reports whether the pass gathered a diagnostic with the code.
func zzHasDiag(r *core.Registry, code string) bool {
	for _, d := range r.Check.Diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}

// TestZZCoverAssumeSigDisjunctPartitionFallthrough: a strict-disjunct
// operand partitions per alternative; with nothing armed the straddle
// is declined (MarkUncompilable is the inactive no-op here) and the
// per-alternative join is spliced — never a blanket no_signature.
func TestZZCoverAssumeSigDisjunctPartitionFallthrough(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()
	e, _ := zzAssumeCall(t, r, "zzr-poly2",
		[]core.Value{zzDisjunctIS(), core.NewWord("zzr-poly2")}, 1, nil)
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier {
		t.Fatalf("partition join must be spliced as the result, tape len = %d", e.Tape.Len())
	}
	if zzHasDiag(r, "no_signature") {
		t.Error("a partitioned disjunct must not raise the blanket no_signature")
	}
}

// TestZZCoverAssumeSigDisjunctPolyRecorded: with an armed TryRecordPoly
// (test double) the straddle records a runtime re-match and splices the
// join — the sig-order operands and the straddle flag cross the seam.
func TestZZCoverAssumeSigDisjunctPolyRecorded(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()

	prev := DispatchBraid.TryRecordPoly
	var gotStraddle, gotDynRecovery bool
	var gotWord string
	DispatchBraid.TryRecordPoly = func(_ *core.Registry, word string, _ *core.Signature, _, _ []core.Value, _ core.SrcPos, straddle bool, _ *core.Registry, dynRecovery bool, _ *core.PolyNoMatchSpec) bool {
		gotWord, gotStraddle, gotDynRecovery = word, straddle, dynRecovery
		return true
	}
	defer func() { DispatchBraid.TryRecordPoly = prev }()

	e, _ := zzAssumeCall(t, r, "zzr-poly2",
		[]core.Value{zzDisjunctIS(), core.NewWord("zzr-poly2")}, 1, nil)
	if gotWord != "zzr-poly2" || !gotStraddle || gotDynRecovery {
		t.Errorf("straddle poly record: word=%q straddle=%v dynRecovery=%v", gotWord, gotStraddle, gotDynRecovery)
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier {
		t.Errorf("recorded straddle must splice the join, tape len = %d", e.Tape.Len())
	}
}

// TestZZCoverAssumeSigDisjunctSingleUserFn: a SINGLE-overload boru user
// fn over the disjunct records a guarded CALL_USER (its ReturnsFn's
// results are spliced) instead of declining.
func TestZZCoverAssumeSigDisjunctSingleUserFn(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()
	e, _ := zzAssumeCall(t, r, "zzr-user1",
		[]core.Value{zzDisjunctIS(), core.NewWord("zzr-user1")}, 1,
		func(s *core.Signature) bool { return s.ReturnsFn != nil && !s.Fallback })
	if e.Tape.Len() != 1 {
		t.Fatalf("tape len = %d, want 1", e.Tape.Len())
	}
	out := e.Tape.At(0)
	if !out.Carrier || !out.Parent.Equal(core.TInteger) {
		t.Errorf("the sole sig's ReturnsFn result must be spliced, got %#v", out)
	}
}

// TestZZCoverAssumeSigDisjunctUserPolyPlan: a MULTI-overload straddle
// under an armed recorder bakes the compiler's arm plan (test double):
// the join carriers are substituted into the plan and spliced.
func TestZZCoverAssumeSigDisjunctUserPolyPlan(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()
	zzArmEmit(r)

	plan := &zzPolyPlan{}
	prev := DispatchBraid.CompileUserPolyArms
	DispatchBraid.CompileUserPolyArms = func(_ *core.Registry, _ core.EmitRecorder, _ string, _ []core.Value, _ []*core.Type) UserPolyPlan {
		return plan
	}
	defer func() { DispatchBraid.CompileUserPolyArms = prev }()

	e, _ := zzAssumeCall(t, r, "zzr-poly2",
		[]core.Value{zzDisjunctIS(), core.NewWord("zzr-poly2")}, 1, nil)
	if len(plan.subbed) == 0 {
		t.Error("the record site must substitute the joined outs into the plan")
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier {
		t.Errorf("user-poly record must splice the join, tape len = %d", e.Tape.Len())
	}
}

// TestZZCoverAssumeSigAnyCarrierPolyRecovery: an Any-typed carrier
// operand under an armed recorder computes results suspended (nothing
// double-records) and hands the dispatch to the runtime-re-matching
// poly (test double) with dynamicRecovery set and the dispatching
// registry as owner.
func TestZZCoverAssumeSigAnyCarrierPolyRecovery(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()
	es := zzArmEmit(r)

	prev := DispatchBraid.TryRecordPoly
	var gotDynRecovery, gotStraddle, wasActiveDuringResults bool
	var gotOwner *core.Registry
	DispatchBraid.TryRecordPoly = func(_ *core.Registry, _ string, _ *core.Signature, _, _ []core.Value, _ core.SrcPos, straddle bool, owner *core.Registry, dynRecovery bool, _ *core.PolyNoMatchSpec) bool {
		gotStraddle, gotOwner, gotDynRecovery = straddle, owner, dynRecovery
		wasActiveDuringResults = es.Active()
		return true
	}
	defer func() { DispatchBraid.TryRecordPoly = prev }()

	e, _ := zzAssumeCall(t, r, "zzr-poly2",
		[]core.Value{core.NewCarrier(core.TAny), core.NewWord("zzr-poly2")}, 1, nil)
	if !gotDynRecovery || gotStraddle || gotOwner != r {
		t.Errorf("poly recovery: dynRecovery=%v straddle=%v owner==r:%v", gotDynRecovery, gotStraddle, gotOwner == r)
	}
	if !wasActiveDuringResults {
		t.Error("recording must be resumed by the time the poly record runs")
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier {
		t.Errorf("recorded poly must splice the results, tape len = %d", e.Tape.Len())
	}
}

// TestZZCoverAssumeSigAnyCarrierDoesNotLowerAndDiagnoses: everything
// declines (unarmed braid, plain — non-Compiling — pass): the recovery
// latches uncompilable and, with no best-fit candidate at all, reports
// the genuine no_signature.
func TestZZCoverAssumeSigAnyCarrierDoesNotLowerAndDiagnoses(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()
	es := zzArmEmit(r)

	// String breaks the sole {Integer,Integer} sig's compatibility, so
	// bestMatch stays -1; the Any carrier makes the dispatch deferrable.
	e, _ := zzAssumeCall(t, r, "zzr-two",
		[]core.Value{core.NewString("x"), core.NewCarrier(core.TAny), core.NewWord("zzr-two")}, 2, nil)
	if !es.hasUncomp("unmatched dispatch recovered at zzr-two") {
		t.Errorf("declined recovery must latch uncompilable, marks = %v", es.uncomp)
	}
	if !zzHasDiag(r, "no_signature") {
		t.Error("a plain pass with no best fit must report no_signature")
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier {
		t.Errorf("assumed results must still be spliced, tape len = %d", e.Tape.Len())
	}
}

// TestZZCoverAssumeSigAnyCarrierRematchTrap: on a COMPILE pass
// (Compiling, armed recorder) the declined Any-carrier dispatch
// compiles to the runtime rematch — the trap owns the tail and no
// compile failure is latched.
func TestZZCoverAssumeSigAnyCarrierRematchTrap(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()
	es := zzArmEmit(r)
	es.rematchOK = true
	r.Check.Compiling = true

	e, _ := zzAssumeCall(t, r, "zzr-poly2",
		[]core.Value{core.NewCarrier(core.TAny), core.NewWord("zzr-poly2")}, 1, nil)
	if !es.rematched {
		t.Error("the compile pass must record the dispatch rematch")
	}
	if len(es.uncomp) != 0 {
		t.Errorf("a recorded rematch must not latch a compile failure, marks = %v", es.uncomp)
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier {
		t.Errorf("the rematch splices the modelled results, tape len = %d", e.Tape.Len())
	}
}

// TestZZCoverAssumeSigAnyCarrierSingleUserFn: the single-overload user
// fn over an Any carrier records the guarded CALL_USER (splicing its
// ReturnsFn results) — the L4 leaf — instead of declining.
func TestZZCoverAssumeSigAnyCarrierSingleUserFn(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()
	es := zzArmEmit(r)

	e, _ := zzAssumeCall(t, r, "zzr-user1",
		[]core.Value{core.NewCarrier(core.TAny), core.NewWord("zzr-user1")}, 1,
		func(s *core.Signature) bool { return s.ReturnsFn != nil && !s.Fallback })
	if len(es.uncomp) != 0 {
		t.Errorf("the recovered user fn must not decline, marks = %v", es.uncomp)
	}
	out := e.Tape.At(0)
	if e.Tape.Len() != 1 || !out.Carrier || !out.Parent.Equal(core.TInteger) {
		t.Errorf("the sole sig's returns must be spliced, got %#v", out)
	}
	if zzHasDiag(r, "no_signature") {
		t.Error("the recoverable single-overload shape must not be flagged")
	}
}

// TestZZCoverAssumeSigImpreciseCarrierPolyRecovery: a concrete-but-
// imprecise carrier (a String carrier against an Integer slot) is not a
// definite mismatch — the armed pass recovers via the runtime
// re-matching poly (test double) instead of declining.
func TestZZCoverAssumeSigImpreciseCarrierPolyRecovery(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()
	es := zzArmEmit(r)

	prev := DispatchBraid.TryRecordPoly
	var gotDynRecovery bool
	DispatchBraid.TryRecordPoly = func(_ *core.Registry, _ string, _ *core.Signature, _, _ []core.Value, _ core.SrcPos, _ bool, _ *core.Registry, dynRecovery bool, _ *core.PolyNoMatchSpec) bool {
		gotDynRecovery = dynRecovery
		return true
	}
	defer func() { DispatchBraid.TryRecordPoly = prev }()

	e, _ := zzAssumeCall(t, r, "zzr-two",
		[]core.Value{core.NewInteger(1), core.NewCarrier(core.TString), core.NewWord("zzr-two")}, 2, nil)
	if !gotDynRecovery {
		t.Error("the imprecise-carrier recovery must offer a dynamic-recovery poly")
	}
	if len(es.uncomp) != 0 {
		t.Errorf("a recorded poly must not decline, marks = %v", es.uncomp)
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier {
		t.Errorf("the recovered dispatch splices its results, tape len = %d", e.Tape.Len())
	}
}

// TestZZCoverAssumeSigResolvesForwardWordArg: a gathered position after
// the pointer still holding a raw Word resolves through the def stack
// before the assumed sig's ReturnsFn sees it.
func TestZZCoverAssumeSigResolvesForwardWordArg(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()
	r.Defs.Push("zzrbound", core.NewInteger(5))

	e, _ := zzAssumeCall(t, r, "zzr-one",
		[]core.Value{core.NewWord("zzr-one"), core.NewWord("zzrbound")}, 0, nil)
	if e.Tape.Len() != 1 {
		t.Fatalf("tape len = %d, want 1", e.Tape.Len())
	}
	out := e.Tape.At(0)
	if !out.Carrier || !out.Parent.Equal(core.TInteger) {
		t.Errorf("the ReturnsFn result must be spliced, got %#v", out)
	}
}

// TestZZCoverAssumeSigAutoEvalsRawMapOperand: a raw eval-map operand
// (word-valued member) is auto-evaluated exactly as execMatch would, so
// the recovery models the evaluated map, not the raw source tokens.
func TestZZCoverAssumeSigAutoEvalsRawMapOperand(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()
	r.Defs.Push("zzrmw", core.NewInteger(9))

	rawMap := mapOf("line", core.NewWord("zzrmw"))
	e, _ := zzAssumeCall(t, r, "zzr-map1",
		[]core.Value{rawMap, core.NewWord("zzr-map1")}, 1, nil)
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier {
		t.Errorf("the assumed map dispatch splices its declared return, tape len = %d", e.Tape.Len())
	}
}

// TestZZCoverAssumeSigArityFallbackPrefersSatisfiableReturnsFn: with no
// compatible sig at all and an Fn-less first-ranked candidate, the
// recovery moves to the first ReturnsFn-bearing sig whose full window
// EXISTS — the 5-arg ReturnsFn overload is skipped (unsatisfiable), the
// 1-arg one wins.
func TestZZCoverAssumeSigArityFallbackPrefersSatisfiableReturnsFn(t *testing.T) {
	r := zzAssumeReg(t)
	done := r.Check.Begin()
	defer done()

	// One String operand: incompatible with every Integer slot, so the
	// scored scan finds nothing; only arity-1 windows exist.
	e, _ := zzAssumeCall(t, r, "zzr-arity",
		[]core.Value{core.NewString("s"), core.NewWord("zzr-arity")}, 1,
		func(s *core.Signature) bool { return s.TotalArgs() == 3 })
	if !zzHasDiag(r, "no_signature") {
		t.Error("the assumed dispatch still reports no_signature on a plain pass")
	}
	if e.Tape.Len() != 1 {
		t.Fatalf("tape len = %d, want 1 (the 1-arg window spliced)", e.Tape.Len())
	}
	out := e.Tape.At(0)
	if !out.Carrier || !out.Parent.Equal(core.TInteger) {
		t.Errorf("the satisfiable ReturnsFn sig's carrier must be spliced, got %#v", out)
	}
}

func TestZZCoverSurfaceShapeDeclinesNonFnsigShape(t *testing.T) {
	// A Required entry that is not an fnsig shape (no FnUndefInfo
	// payload) cannot type the call: the S2 path declines rather than
	// synthesising a signature from a malformed contract.
	r := covRegistry(t, nil)
	info := &core.SurfaceInfo{Required: core.NewOrderedMap(), Conform: map[string]bool{}}
	info.Required.Set("zzr-area", core.NewInteger(1))
	if err := core.InstallType(r, "Zzshaped", core.NewSurfaceType(core.TSurface, info)); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	node := r.LookupTypeName("Zzshaped")
	done := r.Check.Begin()
	defer done()

	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewCarrier(node), core.NewWord("zzr-area")}, core.StackHeadroom)
	e.Pointer = 1

	handled, err := checkModeSurfaceShape(e, core.WordInfo{Name: "zzr-area", ArgCount: -1}, core.SrcPos{})
	if handled || err != nil {
		t.Errorf("a non-fnsig Required shape must decline: (%v, %v)", handled, err)
	}
	if e.Tape.Len() != 2 {
		t.Errorf("declining must leave the tape untouched, len = %d", e.Tape.Len())
	}
}

// --- noteReStepLanding (NUR173, widened by NUR174) --------------------------

// The landing NOTES and nothing more — it consumes nothing, splices nothing
// and declines nothing, which is what lets it sit last in stepLiteral's model
// chain without disturbing the three above it. The gates each leave it
// unrecorded: a suspended recorder, a quoted / id-less / concrete value, a
// NON-CALLABLE carrier the re-step could never apply, a collectable token
// written after the survivor (which the alone-island could not have taken), a
// DISPATCH MODIFIER stating data intent, and a value still sitting alone
// inside a LIVE reach group, where execFnDefLiteral defers rather than calls.
func TestZZCoverReStepLandingGates(t *testing.T) {
	v := core.NewDynamicCarrier(core.TAny)
	v.ID = "zzland2"
	quoted := v
	quoted.Quoted = true
	noID := core.NewDynamicCarrier(core.TAny)
	noID.ID = ""
	reachOpen := core.NewOpenParen()
	reachOpen.ReachGroup = true
	for _, tc := range []struct {
		name    string
		tape    []core.Value
		at      int
		suspend bool
	}{
		{"suspended", []core.Value{v}, 0, true},
		{"quoted", []core.Value{quoted}, 0, false},
		{"no id", []core.Value{noID}, 0, false},
		{"concrete", []core.Value{core.NewInteger(7)}, 0, false},
		{"a non-callable carrier", []core.Value{core.NewCarrier(core.TInteger)}, 0, false},
		{"a collectable token follows", []core.Value{v, core.NewInteger(1)}, 0, false},
		{"a word follows — a barrier, so it lands", []core.Value{v, core.NewWord("zzeq")}, 0, false},
		{"a boundary follows — it lands", []core.Value{v, core.NewCloseParen()}, 0, false},
		{"the tape ends — it lands", []core.Value{v}, 0, false},
		{"alone in a LIVE reach group", []core.Value{reachOpen, v, core.NewCloseParen()}, 1, false},
	} {
		e, es, fin := zzDriftEng(t, tc.tape, 0)
		if tc.suspend {
			es.Suspend()
		}
		noteReStepLanding(e, tc.at)
		if len(es.uncomp) != 0 {
			t.Errorf("%s must never decline, marks = %v", tc.name, es.uncomp)
		}
		fin()
	}
}

// A DISPATCH MODIFIER after the value is DATA intent, and execFnDefLiteral
// honours it by quoting rather than calling. It is a Word by kind, so without
// its own rung nothingToCollectAfter reads it as "nothing to collect" and the
// landing calls the one read written specifically not to be one.
func TestZZCoverReStepLandingDeclinesADispatchModifier(t *testing.T) {
	v := core.NewDynamicCarrier(core.TAny)
	v.ID = "zzland4"
	mod := core.Value{Parent: core.TDispatchMod, Data: core.DispatchModInfo{}}
	e, _, fin := zzDriftEng(t, []core.Value{v, mod}, 0)
	defer fin()
	if nothingToCollectAfter(e, 0) {
		t.Error("a dispatch modifier must stop the landing: it states DATA intent")
	}
}

// aloneInLiveReachGroup is the O(1) shape test execFnDefLiteral makes at the
// same index: only a REACH-written group's markers either side count, so a
// user's own paren — whose call IS the user's — still lands.
func TestZZCoverAloneInLiveReachGroup(t *testing.T) {
	v := core.NewDynamicCarrier(core.TAny)
	v.ID = "zzland5"
	reachOpen := core.NewOpenParen()
	reachOpen.ReachGroup = true
	for _, tc := range []struct {
		name string
		tape []core.Value
		at   int
		want bool
	}{
		{"a reach group's lone token", []core.Value{reachOpen, v, core.NewCloseParen()}, 1, true},
		{"a USER paren's lone token", []core.Value{core.NewOpenParen(), v, core.NewCloseParen()}, 1, false},
		{"a reach group with more to come", []core.Value{reachOpen, v, core.NewInteger(1)}, 1, false},
		{"at index 0 — nothing before it", []core.Value{v, core.NewCloseParen()}, 0, false},
		{"at the tape end — nothing after it", []core.Value{reachOpen, v}, 1, false},
	} {
		e, _, fin := zzDriftEng(t, tc.tape, 0)
		if got := aloneInLiveReachGroup(e, tc.at); got != tc.want {
			t.Errorf("%s: aloneInLiveReachGroup = %v, want %v", tc.name, got, tc.want)
		}
		fin()
	}
}
