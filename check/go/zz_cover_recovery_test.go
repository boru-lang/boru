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

// --- dispatch_hooks.go: private forwarders over the inactive defaults -------

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

// A run that changes a binding is no pure computation: the fold declines
// (NUR330 — the interpreter keeps the binding a map member's `[def k 1 k]`
// makes, the folded constant would not), and the table is restored EXACTLY,
// a popped entry included, so the expression's recorded run starts where the
// interpreter's does.
func TestZZCoverConcreteEvalOnceDeclinesABindingRun(t *testing.T) {
	r := covRegistry(t, func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{Name: "zzbind", Signatures: []core.Signature{{
			Returns: []*core.Type{core.TInteger},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
				r.Defs.Push("zzfresh", core.NewInteger(1))
				return []core.Value{core.NewInteger(7)}, nil
			}),
		}}})
		r.RegisterNativeFunc(core.NativeFunc{Name: "zzunbind", Signatures: []core.Signature{{
			Returns: []*core.Type{core.TInteger},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
				r.Defs.Pop("zzkeep")
				return []core.Value{core.NewInteger(8)}, nil
			}),
		}}})
	})
	done := r.Check.Begin()
	defer done()
	e := core.NewTop(r)
	r.Defs.Push("zzkeep", core.NewInteger(42))

	if _, ok := concreteEvalOnce(e, []core.Value{core.NewWord("zzbind")}); ok {
		t.Error("a run that binds a name must decline")
	}
	if r.Defs.Has("zzfresh") {
		t.Error("the run's binding must be rolled back")
	}
	if _, ok := concreteEvalOnce(e, []core.Value{core.NewWord("zzunbind")}); ok {
		t.Error("a run that unbinds a name must decline")
	}
	if v, bound := r.Defs.Top("zzkeep"); !bound || v.String() != "42" {
		t.Errorf("the popped binding must be restored, got %v (bound=%v)", v, bound)
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
	if !out.Carrier() || !core.IsDynamic(&out) || !out.Parent().Equal(core.TAny) || core.IDOf(&out) == "" {
		t.Errorf("collapsed window must seat one dynamic Any carrier with an ID, got %#v", out)
	}
	if !core.IsMark(e.Tape.Ref(1)) {
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
	if !out.Carrier() || !core.IsDynamic(&out) || !out.Parent().Equal(core.TAny) {
		t.Errorf("leading apply must collapse to one dynamic Any carrier, got %#v", out)
	}
}

func TestZZCoverParenFnCollapseLeadingFnValueArg(t *testing.T) {
	// `( g inc/v )` — a leading fn carrier over an INERT fn value collapses
	// (S1b's apply shapes, 2026-09-22): the lead collects the value, one
	// call on both engines. A GRADUAL fn-typed argument does not — it may
	// be a bare fn word at run time.
	fnVal := core.ValueOf(&core.Datum{Parent: core.TFunction, Data: &core.FnDefInfo{Name: "inc"}})
	e, fin := zzCollapseEng(t, true,
		core.NewOpenParen(), zzFnCarrier(), fnVal, core.NewCloseParen())
	defer fin()
	if got := checkModeParenFnCollapse(e, 0, 3); got != 2 {
		t.Fatalf("closeIdx = %d, want 2 (an inert fn value collapses under the lead)", got)
	}
	if out := e.Tape.At(1); !out.Carrier() || !core.IsDynamic(&out) || !out.Parent().Equal(core.TAny) {
		t.Errorf("leading apply over a fn value must collapse to one dynamic Any carrier, got %#v", out)
	}
	gradual := core.NewCarrier(core.TFunction)
	core.SetDynamic(&gradual, true)
	e2, fin2 := zzCollapseEng(t, true,
		core.NewOpenParen(), zzFnCarrier(), gradual, core.NewCloseParen())
	defer fin2()
	if got := checkModeParenFnCollapse(e2, 0, 3); got != 3 {
		t.Errorf("closeIdx = %d, want 3 (a gradual fn-typed argument stays un-collapsed)", got)
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
	if !out.Carrier() || !out.Parent().Equal(core.TInteger) {
		t.Errorf("declared-return fn value must splice an Integer carrier, got %#v", out)
	}
	if e.Pointer != 0 {
		t.Errorf("pointer = %d, want 0", e.Pointer)
	}
}

// --- DeclineForwardStackDrift -------------------------------------------------

// --- declineStrandedMemberFn --------------------------------------------------

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
	if !out.Carrier() || !out.Parent().Equal(core.TInteger) {
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
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier() {
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
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier() {
		t.Fatalf("partition join must be spliced as the result, tape len = %d", e.Tape.Len())
	}
	if zzHasDiag(r, "no_signature") {
		t.Error("a partitioned disjunct must not raise the blanket no_signature")
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
	if !out.Carrier() || !out.Parent().Equal(core.TInteger) {
		t.Errorf("the sole sig's ReturnsFn result must be spliced, got %#v", out)
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
	if !out.Carrier() || !out.Parent().Equal(core.TInteger) {
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
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier() {
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
	if !out.Carrier() || !out.Parent().Equal(core.TInteger) {
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

// --- checkModeFallbackPositionsFor -------------------------------------------

// TestZZCoverFallbackPositionsForwardFirst pins NUR180's fix: the recovery's
// window for ONE signature fills the forward-eligible leading positions from
// the written tokens FIRST, as the interpreter's matcher does, and only the
// remainder from the stack — two stack values beneath a typed word with one
// written argument take [top, written], never [deeper, top]; the stack run
// comes back ascending with its length, a non-matching written token stops
// the forward run, `/s` collects nothing forward, and a short stack fills
// from the tokens after the run as the plain gatherer does.
func TestZZCoverFallbackPositionsForwardFirst(t *testing.T) {
	two := &core.Signature{Args: []*core.Type{core.TNumber, core.TNumber}, BarrierPos: core.BarrierAllForward}
	// tape: [1 2 zzw 10 "s"], pointer on zzw
	e := engWithTape(t, []core.Value{core.NewInteger(1), core.NewInteger(2), core.NewWord("zzw"), core.NewInteger(10), core.NewString("s")}, 2)
	pos, nStack := checkModeFallbackPositionsFor(e, two, core.WordInfo{Name: "zzw"})
	if len(pos) != 2 || pos[0] != 1 || pos[1] != 3 || nStack != 1 {
		t.Errorf("forward first: positions = %v nStack = %d, want [1 3] 1 (the written 10 fills sig[0], the stack top sig[1])", pos, nStack)
	}
	// /s: nothing collects forward — both from the stack, ascending.
	pos, nStack = checkModeFallbackPositionsFor(e, two, core.WordInfo{Name: "zzw", ForceStack: true})
	if len(pos) != 2 || pos[0] != 0 || pos[1] != 1 || nStack != 2 {
		t.Errorf("/s: positions = %v nStack = %d, want [0 1] 2", pos, nStack)
	}
	// A written token the position rejects stops the forward run: [1 2 zzw "s"]
	e = engWithTape(t, []core.Value{core.NewInteger(1), core.NewInteger(2), core.NewWord("zzw"), core.NewString("s")}, 2)
	pos, nStack = checkModeFallbackPositionsFor(e, two, core.WordInfo{Name: "zzw"})
	if len(pos) != 2 || pos[0] != 0 || pos[1] != 1 || nStack != 2 {
		t.Errorf("a rejected token stops the run: positions = %v nStack = %d, want [0 1] 2", pos, nStack)
	}
	// A wildcard (an Any carrier, a raw word) is taken: [1 zzw x 10]
	x := core.NewCarrier(core.TAny)
	e = engWithTape(t, []core.Value{core.NewInteger(1), core.NewWord("zzw"), x, core.NewInteger(10)}, 1)
	pos, nStack = checkModeFallbackPositionsFor(e, two, core.WordInfo{Name: "zzw"})
	if len(pos) != 2 || pos[0] != 2 || pos[1] != 3 || nStack != 0 {
		t.Errorf("wildcards are taken: positions = %v nStack = %d, want [2 3] 0", pos, nStack)
	}
	// A short stack fills from the tokens after the run: [zzw "s" 7] with a
	// stack-only signature — nothing forward-eligible, nothing beneath, the
	// shortfall walks the tape after the pointer.
	stackOnly := &core.Signature{Args: []*core.Type{core.TNumber, core.TNumber}, BarrierPos: 0}
	e = engWithTape(t, []core.Value{core.NewWord("zzw"), core.NewString("s"), core.NewInteger(7)}, 0)
	pos, nStack = checkModeFallbackPositionsFor(e, stackOnly, core.WordInfo{Name: "zzw"})
	if len(pos) != 2 || pos[0] != 1 || pos[1] != 2 || nStack != 0 {
		t.Errorf("shortfall: positions = %v nStack = %d, want [1 2] 0", pos, nStack)
	}
	if !fallbackTokenCompatible(two, 0, core.NewWord("w")) || !fallbackTokenCompatible(two, 0, x) || fallbackTokenCompatible(two, 0, core.NewString("s")) {
		t.Error("fallbackTokenCompatible: a raw word and an Any carrier are wildcards, a String against Number is not")
	}
}

// TestZZCoverFallbackPositionsForwardCrossesGroupsAndMarkers: the forward
// run walks exactly as the plain gatherer does (pinned for that walk by
// TestZZCoverFallbackPositionsSkipNestedGroup) — a NESTED group after the
// word is entered and left without its close stopping the run, and the
// engine's tape markers (a mark, a frame's def-cleanup / return-check tail)
// are stepped over rather than taken as operands — so a call whose written
// arguments straddle a raw group or a marker is recovered over the same
// tokens the interpreter's forward phase would reach.
func TestZZCoverFallbackPositionsForwardCrossesGroupsAndMarkers(t *testing.T) {
	three := &core.Signature{Args: []*core.Type{core.TNumber, core.TNumber, core.TNumber}, BarrierPos: core.BarrierAllForward}
	// tape: [zzw ( 1 ) <mark> 2 <def-cleanup> <return-check> 3 4], pointer on zzw
	e := engWithTape(t, []core.Value{
		core.NewWord("zzw"),
		core.NewOpenParen(), core.NewInteger(1), core.NewCloseParen(),
		core.NewMark("zzm"),
		core.NewInteger(2),
		core.NewDefCleanup(core.DefCleanupInfo{SkipCleanup: true}),
		core.NewReturnCheck(core.ReturnCheckInfo{FuncName: "zzf"}),
		core.NewInteger(3), core.NewInteger(4),
	}, 0)
	pos, nStack := checkModeFallbackPositionsFor(e, three, core.WordInfo{Name: "zzw"})
	if len(pos) != 3 || pos[0] != 2 || pos[1] != 5 || pos[2] != 8 || nStack != 0 {
		t.Errorf("positions = %v nStack = %d, want [2 5 8] 0 (the group is crossed, the markers skipped, the fourth token left)", pos, nStack)
	}
	// A forward run that stops SHORT (a String the second position rejects)
	// is a prefix of what the plain gatherer takes past the empty stack, so
	// the shortfall fill skips the taken token and appends exactly the
	// missing one — the window is n, never more.
	two := &core.Signature{Args: []*core.Type{core.TNumber, core.TNumber}, BarrierPos: core.BarrierAllForward}
	e = engWithTape(t, []core.Value{core.NewWord("zzw"), core.NewInteger(1), core.NewString("s"), core.NewInteger(9)}, 0)
	pos, nStack = checkModeFallbackPositionsFor(e, two, core.WordInfo{Name: "zzw"})
	if len(pos) != 2 || pos[0] != 1 || pos[1] != 2 || nStack != 0 {
		t.Errorf("shortfall after a partial run: positions = %v nStack = %d, want [1 2] 0", pos, nStack)
	}
}
