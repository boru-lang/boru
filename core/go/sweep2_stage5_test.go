package core

// Final-sweep coverage tests (stage 5), part 2: registry/engine-flavored
// arms — capability misconfiguration, the compile sandbox snapshot maps,
// coverage/observability hooks, the TCO eligibility gates, generic class
// schemas, the invoke seams, and the word-extension merge policy.

import (
	"errors"
	"strings"
	"testing"
)

// --- capability.go ---------------------------------------------------------

func TestSweepCapNilCapabilityRegistry(t *testing.T) {
	// A zero Registry (never built via NewRegistry) surfaces the
	// misconfiguration error from the nil capability store.
	if _, ok, err := Cap[int](&Registry{}, "x"); ok || err == nil {
		t.Errorf("Cap on a zero registry = ok=%v err=%v", ok, err)
	}
}

// --- compile_sandbox.go ----------------------------------------------------

func TestSweepCompileSandboxSnapshotsBuiltinsAndCaps(t *testing.T) {
	r := covRegistry(t, nil) // native registrations populate builtinWords
	if err := r.Capabilities.Set("sweep.cap", 41); err != nil {
		t.Fatal(err)
	}
	s := r.SnapshotForCompile()
	if len(s.builtins) == 0 {
		t.Fatal("snapshot captured no builtin words")
	}
	if _, ok := s.caps["sweep.cap"]; !ok {
		t.Fatal("snapshot missed the capability slot")
	}
	// Mutate, then roll back: the snapshot's copies reinstall.
	r.builtinWords["sweep_added"] = true
	if err := r.Capabilities.Set("sweep.added", 1); err != nil {
		t.Fatal(err)
	}
	r.RestoreForCompile(s)
	if r.builtinWords["sweep_added"] {
		t.Error("restore kept a post-snapshot builtin word")
	}
	if _, ok, _ := Cap[int](r, "sweep.added"); ok {
		t.Error("restore kept a post-snapshot capability")
	}
	if v, ok, _ := Cap[int](r, "sweep.cap"); !ok || v != 41 {
		t.Errorf("restore lost the pre-snapshot capability: %v %v", v, ok)
	}
}

// --- coverage.go -----------------------------------------------------------

func TestSweepCoverageArmedAndVMEmit(t *testing.T) {
	r := newTestRegistry(t)
	if r.CoverageArmed() {
		t.Error("a fresh registry must report the hook unarmed")
	}
	// Untagged registry: NoteVMCoverage short-circuits.
	r.NoteVMCoverage([]SrcPos{{}}, 0)
	// Tagged: the emit path runs (the unarmed hook drops the event).
	r.SetCoverID("sweep-unit")
	r.NoteVMCoverage([]SrcPos{{}}, 0)
	// Armed and tagged: the observation arrives.
	var rows int
	disarm := r.ArmCoverageHook(func(string, int) { rows++ })
	defer disarm()
	if !r.CoverageArmed() {
		t.Error("armed registry must report armed")
	}
	r.NoteVMCoverage([]SrcPos{{Row: 3}}, 0)
	if rows != 1 {
		t.Errorf("armed VM emit count = %d, want 1", rows)
	}
}

// --- diag_msg.go -----------------------------------------------------------

func TestSweepRuntimeNoMatch(t *testing.T) {
	r := newTestRegistry(t)
	be := RuntimeNoMatch(r, "sweep_nomatch_word", []Value{NewInteger(1)})
	if be == nil || be.Code != "signature_error" {
		t.Errorf("RuntimeNoMatch = %+v", be)
	}
}

// --- fn_frame_elide.go / fn_frame_probe.go ---------------------------------

func TestSweepTCOEligibleParenEvalDecline(t *testing.T) {
	f := newProbeFixture(t)
	e := NewTop(f.r)
	e.parenEvalDepth = 1
	if e.tcoEligible(frameTailScan{Meta: f.meta, TailStart: 0}, &Signature{}, 0) {
		t.Error("a dispatch inside a forward paren evaluation must decline TCO")
	}
}

func TestSweepTCOEligibleEvalResidualPendingBelow(t *testing.T) {
	f := newProbeFixture(t)
	e := NewTop(f.r)
	snap := f.r.Defs.Snapshot()
	e.Tape = NewTape([]Value{NewDefCleanup(DefCleanupInfo{
		Snapshot: snap, Registry: f.r, EvalResidual: true,
	})}, 4)
	scan := frameTailScan{Meta: f.meta, TailStart: 0, PendingEvalBelow: true}
	sig := &Signature{Impl: &BoruImpl{FnFrame: f.meta}}
	if e.tcoEligible(scan, sig, f.r.Defs.Mutations()) {
		t.Error("an EvalResidual frame with a pending container below must decline")
	}
	// Sanity: the same frame without the pending container is eligible.
	clean := frameTailScan{Meta: f.meta, TailStart: 0}
	if !e.tcoEligible(clean, sig, f.r.Defs.Mutations()) {
		t.Error("the residual-free twin should be eligible")
	}
}

func TestSweepProbeTailCallPendingResidualBelow(t *testing.T) {
	f := newProbeFixture(t)
	// (ₘ [pending] f __DC __pa )  — the pending plain list below the call
	// marks both ValuesBelow and PendingEvalBelow.
	tokens := []Value{
		NewFrameOpen(f.meta), pendingList(NewInteger(1)), NewWord("f"),
		f.dc, NewWord("__pa"), NewCloseParen(),
	}
	e := NewTop(f.r)
	e.Tape = NewTape(tokens, 8)
	e.Pointer = 2
	scan, ok := e.probeTailCall(nil, 0)
	if !ok {
		t.Fatal("probe declined the values-below shape")
	}
	if !scan.ValuesBelow || !scan.PendingEvalBelow {
		t.Errorf("scan flags = %+v, want ValuesBelow and PendingEvalBelow", scan)
	}
}

// --- fork.go ---------------------------------------------------------------

func TestSweepForkCopiesSugarWords(t *testing.T) {
	r := newTestRegistry(t)
	r.BindSugarWord(SugarLambda, "sweeplam")
	fork := r.ForkConcurrent()
	if w, ok := fork.SugarWord(SugarLambda); !ok || w != "sweeplam" {
		t.Errorf("fork sugar binding = %q, %v", w, ok)
	}
}

// --- generics.go / generics_instantiate.go ---------------------------------

func TestSweepBoundNodeSurface(t *testing.T) {
	sv := NewSurfaceType(TSurface, &SurfaceInfo{Required: NewOrderedMap()})
	if got := boundNode(sv); got != TSurface {
		t.Errorf("boundNode(surface) = %v", got)
	}
}

func TestSweepSchemaUnifierFormatFallthrough(t *testing.T) {
	s := &schemaUnifier{
		behaviorWrapper: behaviorWrapper{prev: DefaultBehavior},
		info:            &TypeSchemaInfo{Name: "Class/SweepBox"},
	}
	if got := s.Format(NewInteger(3)); got != "3" {
		t.Errorf("non-schema value render = %q", got)
	}
}

func TestSweepInstantiateClassSchema(t *testing.T) {
	r := newTestRegistry(t)
	spec := &GenSpecInfo{Params: []GenParam{{Name: "T"}}}
	PushGenBindings(r, spec)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(spec.Params[0].Node))
	body := NewClassType(TClass, ClassTypeInfo{Fields: fields})
	PopGenBindings(r, spec)
	info := &TypeSchemaInfo{Params: spec.Params, Body: body, Kind: SchemaClass}
	if err := InstallType(r, "SweepBox", NewTypeSchema(TType, info)); err != nil {
		t.Fatalf("InstallType SweepBox: %v", err)
	}
	inst, err := InstantiateSchema(r, info, []Value{NewTypeLiteral(TInteger)})
	if err != nil {
		t.Fatalf("InstantiateSchema: %v", err)
	}
	oi, aerr := AsClassType(inst)
	if aerr != nil {
		t.Fatalf("instantiation is not a class type: %v", aerr)
	}
	if oi.Type == nil || !strings.Contains(oi.Name, "of [Integer]") {
		t.Errorf("instantiation identity = %+v", oi)
	}
	fv, ok := oi.Fields.Get("x")
	if !ok {
		t.Fatal("substituted field missing")
	}
	if !IsBareTypeNode(fv) || (&fv).Leaf() != "Integer" {
		t.Errorf("substituted field type = %v", fv)
	}
}

// --- interp_entry.go -------------------------------------------------------

func TestSweepRuntimeBailHookAndAttribution(t *testing.T) {
	r := newTestRegistry(t)
	// Unarmed: the emit drops silently.
	r.NoteBail("vm:sweep", "unarmed")
	var got []BailEvent
	disarm := r.ArmRuntimeBailHook(func(ev BailEvent) { got = append(got, ev) })
	r.NoteBail("vm:sweep", "armed")
	disarm()
	r.NoteBail("vm:sweep", "after-disarm")
	if len(got) != 1 || got[0].Site != "vm:sweep" || got[0].Reason != "armed" {
		t.Errorf("bail events = %+v", got)
	}

	restore := r.SetInterpAttribution("sweep:fallback")
	if r.interpAttribution != "sweep:fallback" {
		t.Error("attribution tag not installed")
	}
	restore()
	if r.interpAttribution != "" {
		t.Error("attribution tag not restored")
	}
}

// --- invoke.go -------------------------------------------------------------

type sweepCompiledRuntime struct{}

func (sweepCompiledRuntime) InvokeCompiled(*Registry, *Signature, []Value) ([]Value, error, bool) {
	return []Value{NewInteger(99)}, nil, true
}
func (sweepCompiledRuntime) StampDetached(*Registry, FnDefInfo, SrcPos) {}
func (sweepCompiledRuntime) ClosureAsFnDef(_ *Registry, v Value) (Value, bool) {
	return v, false
}

func TestSweepInvokeBodyRouting(t *testing.T) {
	// Invoker installed: the VM seam owns body execution.
	r := newTestRegistry(t)
	r.Invoker = func(_ *Registry, _ Value, _ []Value) ([]Value, error) {
		return []Value{NewInteger(7)}, nil
	}
	out, err := InvokeBody(r, NewList(nil), nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("invoker path = %v, %v", out, err)
	}
	if got, _ := AsInteger(out[0]); got != 7 {
		t.Errorf("invoker result = %v", out)
	}
	// No Invoker: the pooled interpreter path runs the tokens.
	r2 := newTestRegistry(t)
	out, err = InvokeBody(r2, NewList([]Value{NewInteger(3)}), nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("interpreter path = %v, %v", out, err)
	}
	if got, _ := AsInteger(out[0]); got != 3 {
		t.Errorf("interpreter result = %v", out)
	}
}

func TestSweepInvokeCallbackCompiledFastPath(t *testing.T) {
	prev := InstallCompiledRuntime(sweepCompiledRuntime{})
	defer InstallCompiledRuntime(prev)
	r := newTestRegistry(t)
	out, err := InvokeCallback(r, &Signature{}, nil, nil)
	if err != nil {
		t.Fatalf("compiled callback path: %v", err)
	}
	if got, _ := AsInteger(out[0]); got != 99 {
		t.Errorf("compiled callback result = %v", out)
	}
}

func TestSweepIsInternalError(t *testing.T) {
	if IsInternalError(errors.New("plain")) {
		t.Error("a plain error is not internal_error-class")
	}
	if !IsInternalError(&BoruError{Code: "internal_error"}) {
		t.Error("an internal_error BoruError must classify")
	}
}

// --- macro_expand.go -------------------------------------------------------

func TestSweepExpandTemplateNestedError(t *testing.T) {
	r := newTestRegistry(t)
	// A nested list whose expansion errors (splice with no operand)
	// propagates through expandNested.
	toks := []Value{NewList([]Value{NewWord("splice")})}
	if _, err := expandTemplate(r, toks, map[string]Value{}, map[string]string{}); err == nil {
		t.Error("a nested template error must propagate")
	}
}

// --- module_export_growth.go -----------------------------------------------

func TestSweepModuleExportGrowthResetAndAbsence(t *testing.T) {
	// Reset with no table: the nil-table early return.
	r := newTestRegistry(t)
	ResetModuleExportGrowth(r)
	// Registered ledger with a note: reset clears the per-pass state.
	fields := NewOrderedMap()
	fields.Set("f", NewInteger(1))
	RegisterModuleExportGrowth(r, fields)
	NoteModuleExportAdd(r, fields, "grown")
	PoisonModuleExportGrowth(r, fields)
	ResetModuleExportGrowth(r)
	tbl := exportGrowthTableFor(r, false)
	if tbl == nil {
		t.Fatal("registration lost by reset")
	}
	l := tbl.byFields[fields]
	if l == nil || l.poisoned || len(l.adds) != 0 {
		t.Errorf("reset left per-pass notes: %+v", l)
	}

	// Absence probe with NO growth table on the registry: declines.
	r2 := newTestRegistry(t)
	ns := WithModuleNS(mapOf("f", NewInteger(1)), "SweepMod", NewModuleInstance(ModuleDesc{Ref: "sweep:mod"}))
	if ModuleExportAbsenceStable(r2, []Value{ns, NewAtom("missing")}) {
		t.Error("a table-less registry must decline the absence fold")
	}
}

// --- word_extend.go --------------------------------------------------------

func TestSweepSigTupleEqualArityMismatch(t *testing.T) {
	a := &Signature{Args: []*Type{TInteger}, BarrierPos: 0}
	b := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 0}
	NormalizeSig(a)
	NormalizeSig(b)
	if sigTupleEqual(a, b) {
		t.Error("differing arities must not be tuple-equal")
	}
}

func sweepGoSig(types ...*Type) Signature {
	return Signature{
		Args: types,
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return args, nil
		}),
		Returns: []*Type{TAny}, BarrierPos: -1,
	}
}

func TestSweepMergeExtensionSigsTransplantCollisions(t *testing.T) {
	r := newTestRegistry(t)
	// The same unlocked tuple from a DIFFERENT module: a loud conflict.
	held := sweepGoSig(TInteger)
	NormalizeSig(&held)
	held.Origin = "sweep:m1"
	base := &FnDefInfo{Name: "sweepx", Signatures: []Signature{held}}
	incoming := sweepGoSig(TInteger)
	NormalizeSig(&incoming)
	if _, _, err := mergeExtensionSigs(r, "sweepx", base, []Signature{incoming}, "sweep:m2"); err == nil || !strings.Contains(err.Error(), "extend_conflict") {
		t.Errorf("cross-module collision: err = %v", err)
	}
	// A direct user def (Origin "") is shadowed freely by a transplant.
	direct := sweepGoSig(TInteger)
	NormalizeSig(&direct)
	base2 := &FnDefInfo{Name: "sweepx", Signatures: []Signature{direct}}
	replacement := sweepGoSig(TInteger)
	NormalizeSig(&replacement)
	merged, changed, err := mergeExtensionSigs(r, "sweepx", base2, []Signature{replacement}, "sweep:m2")
	if err != nil || !changed {
		t.Fatalf("replace over a direct def: changed=%v err=%v", changed, err)
	}
	if len(merged) != 1 || merged[0].Origin != "sweep:m2" {
		t.Errorf("replacement provenance = %+v", merged)
	}
}

// sweepPushLockedWord binds name to a hand-built locked-bearing word (a
// NON-builtin base, so the R1 anchor admission does not apply and the
// merge arms are reachable directly).
func sweepPushLockedWord(r *Registry, name string) {
	locked := sweepGoSig(TInteger)
	locked.Locked = true
	NormalizeSig(&locked)
	r.Defs.Push(name, NewFunction(FnDefInfo{Name: name, Signatures: []Signature{locked}}))
}

func TestSweepInstallWordExtensionMergeError(t *testing.T) {
	r := newTestRegistry(t)
	sweepPushLockedWord(r, "sweeplockw")
	ext := FnDefInfo{Name: "sweeplockw", Signatures: []Signature{sweepGoSig(TInteger)}}
	if err := InstallWordExtension(r, "sweeplockw", ext); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Errorf("locked-tuple def-merge: err = %v", err)
	}
}

func TestSweepInstallWordExtensionRegisterHook(t *testing.T) {
	r := newTestRegistry(t)
	sweepPushLockedWord(r, "sweephookw")
	var hooked []string
	r.OnRegisterHook = func(name string) { hooked = append(hooked, name) }
	r.MarkReady()
	ext := FnDefInfo{Name: "sweephookw", Signatures: []Signature{sweepGoSig(TString)}}
	if err := InstallWordExtension(r, "sweephookw", ext); err != nil {
		t.Fatalf("InstallWordExtension: %v", err)
	}
	if len(hooked) != 1 || hooked[0] != "sweephookw" {
		t.Errorf("register hook calls = %v", hooked)
	}
}

func TestSweepTransplantSourceCloneAuthorDefault(t *testing.T) {
	// A SOURCE clone (no ExtOwner) onto a BUILTIN word: the author
	// defaults to the exporting module's owner, and the unanchored
	// kernel-typed tuple is refused.
	r := covRegistry(t, nil)
	ext := NewWordExtension("", "cadd", []Signature{sweepGoSig(TBoolean, TBoolean)})
	if ext.ExtOwner != "" {
		t.Fatalf("source clone ExtOwner = %q", ext.ExtOwner)
	}
	err := TransplantExtension(r, ext, "sweep:mod", "sweep:owner")
	if err == nil || !strings.Contains(err.Error(), "extend_owner") {
		t.Errorf("unanchored source transplant: err = %v", err)
	}
}

func TestSweepTransplantMergeErrorAndHook(t *testing.T) {
	r := newTestRegistry(t)
	// Merge error: the incoming tuple equals the base's locked signature.
	sweepPushLockedWord(r, "sweeptw")
	ext := NewWordExtension("", "sweeptw", []Signature{sweepGoSig(TInteger)})
	if err := TransplantExtension(r, ext, "sweep:m1", ""); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Errorf("locked-tuple transplant: err = %v", err)
	}
	// Successful transplant fires the register hook on a ready registry.
	sweepPushLockedWord(r, "sweeptw2")
	var hooked []string
	r.OnRegisterHook = func(name string) { hooked = append(hooked, name) }
	r.MarkReady()
	ext2 := NewWordExtension("", "sweeptw2", []Signature{sweepGoSig(TString)})
	if err := TransplantExtension(r, ext2, "sweep:m1", ""); err != nil {
		t.Fatalf("transplant: %v", err)
	}
	if len(hooked) != 1 || hooked[0] != "sweeptw2" {
		t.Errorf("register hook calls = %v", hooked)
	}
}
