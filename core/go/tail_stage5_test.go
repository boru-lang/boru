package core

// Stage-5 coverage for the small-file tail: effects.go (ledger + fence),
// tape.go (bounds, growth ceiling, reload), modules.go (module registry
// state), define_type.go (host-Go installers), exit_error.go, ideal.go
// (kind registry), analysis_hooks.go (accessor layer), argsstack.go,
// boru_error.go, and carrier_new.go.

import (
	"errors"
	"strings"
	"testing"
)

// --- effects.go -----------------------------------------------------------

func TestX5EffectLedgerNoteCount(t *testing.T) {
	var nilLedger *EffectLedger
	nilLedger.Note() // nil-safe
	if nilLedger.Count() != 0 {
		t.Fatal("nil ledger counts 0")
	}
	l := &EffectLedger{}
	l.Note()
	l.Note()
	if l.Count() != 2 {
		t.Fatalf("expected 2 effects, got %d", l.Count())
	}
}

func TestX5RegistryNoteEffect(t *testing.T) {
	var nilReg *Registry
	nilReg.NoteEffect() // nil-safe
	r := x5reg(t)
	before := r.Effects.Count()
	r.NoteEffect()
	if r.Effects.Count() != before+1 {
		t.Fatal("NoteEffect marks the registry ledger")
	}
}

// --- tape.go --------------------------------------------------------------

func x5Vals(n int) []Value {
	vs := make([]Value, n)
	for i := range vs {
		vs[i] = NewInteger(int64(i))
	}
	return vs
}

func TestX5TapeResolveClampsToProgram(t *testing.T) {
	// InitialSize below the program length is raised to hold it.
	tp := NewTapeWith(x5Vals(5), TapeConfig{InitialSize: 2}, nil)
	if tp.Len() != 5 {
		t.Fatalf("expected 5 elements, got %d", tp.Len())
	}
}

func TestX5TapeReloadTooSmall(t *testing.T) {
	tp := NewTapeWith(x5Vals(2), TapeConfig{InitialSize: 2}, nil)
	if tp.Reload(x5Vals(4000)) {
		t.Fatal("a program larger than the backing array must decline Reload")
	}
}

func TestX5TapeBoundsGuards(t *testing.T) {
	tp := NewTape(x5Vals(3), 4)
	if v := tp.At(-1); v.Data != nil {
		t.Fatal("out-of-range At returns the zero Value")
	}
	if v := tp.At(99); v.Data != nil {
		t.Fatal("past-end At returns the zero Value")
	}
	tp.Set(-1, NewInteger(9)) // ignored
	tp.Set(99, NewInteger(9)) // ignored
	if n, _ := AsInteger(tp.At(0)); n != 0 {
		t.Fatal("guarded Set must not write")
	}
	tp.MoveGap(-5)
	tp.MoveGap(1000)
	tp.Insert(-1, NewInteger(7))
	tp.Insert(999, NewInteger(8))
	if tp.Len() != 5 {
		t.Fatalf("clamped inserts landed at the ends, got len %d", tp.Len())
	}
	tp.Remove(-1)
	tp.Remove(999)
	if tp.Len() != 5 {
		t.Fatal("guarded Remove must not delete")
	}
	tp.Splice(-2, 1)
	if tp.Len() != 4 {
		t.Fatalf("clamped splice removes at index 0, got len %d", tp.Len())
	}
	if got := tp.Prefix(-1); len(got) != 0 {
		t.Fatal("negative Prefix end clamps to 0")
	}
	if got := tp.CopyRange(0, 999); len(got) != tp.Len() {
		t.Fatalf("CopyRange clamps j to Len, got %d", len(got))
	}
	if got := tp.CopyRange(3, 2); got != nil {
		t.Fatal("inverted CopyRange returns nil")
	}
}

func TestX5TapeGrowCeilingTooSmallForEdit(t *testing.T) {
	// initial 4, one grow by 2 → ceiling 8; a 10-value payload can never
	// fit, so the tape latches exhausted.
	tp := NewTapeWith(x5Vals(4), TapeConfig{InitialSize: 4, MaxGrows: 1, GrowthFactor: 2}, nil)
	tp.Splice(4, 0, x5Vals(10)...)
	if !tp.Exhausted() {
		t.Fatal("an edit beyond the growth ceiling latches exhausted")
	}
}

func TestX5TapeInsertAtCeiling(t *testing.T) {
	// A full tape with no growth headroom: Insert becomes a no-op and
	// latches exhausted.
	tp := NewTapeWith(x5Vals(2), TapeConfig{InitialSize: 2, MaxGrows: 1, GrowthFactor: 1.01}, nil)
	tp.Insert(2, NewInteger(9))
	if !tp.Exhausted() {
		t.Fatal("insert at the ceiling latches exhausted")
	}
	if tp.Len() != 2 {
		t.Fatal("the failed insert is dropped")
	}
}

// --- modules.go -----------------------------------------------------------

func TestX5ModuleRegistryStates(t *testing.T) {
	var nilMR *ModuleRegistry
	nilMR.InheritConfig(NewModuleRegistry()) // nil-safe
	if nilMR.IsLoaded("m") {
		t.Fatal("nil registry has nothing loaded")
	}
	if _, ok := nilMR.LoadedDesc("m"); ok {
		t.Fatal("nil registry has no descs")
	}
	nilMR.MarkLoaded("m", ModuleDesc{}) // nil-safe
	if got := nilMR.NextID(); got != "mod_0" {
		t.Fatalf("nil registry IDs as mod_0, got %q", got)
	}

	parent := NewModuleRegistry()
	parent.InitFunc = func(*Registry) {}
	parent.Resolver = func(string, *Registry) (ModuleDesc, error) { return ModuleDesc{}, nil }
	m := NewModuleRegistry()
	m.InheritConfig(parent)
	if m.InitFunc == nil || m.Resolver == nil {
		t.Fatal("InheritConfig copies both callbacks")
	}

	// A zero-struct registry lazily allocates its loaded map.
	z := &ModuleRegistry{}
	z.MarkLoaded("x5m", ModuleDesc{ID: "m1"})
	if !z.IsLoaded("x5m") {
		t.Fatal("marked module reports loaded")
	}
	d, ok := z.LoadedDesc("x5m")
	if !ok || d.ID != "m1" {
		t.Fatalf("cached desc round-trips, got %v ok=%v", d, ok)
	}
	if got := z.NextID(); got != "mod_1" {
		t.Fatalf("first NextID is mod_1, got %q", got)
	}
}

// --- define_type.go -------------------------------------------------------

// x5SubNamer rejects every subtype name.
type x5SubNamer struct{ x5StubBehavior }

func (x5SubNamer) ValidateSubtypeName(name string) error {
	return errors.New("x5: bad subtype name " + name)
}

func TestX5DefineTypeAndEnum(t *testing.T) {
	r := x5reg(t)
	def, err := r.DefineType("X5DT", NewTypeLiteral(TInteger))
	if err != nil || def == nil {
		t.Fatalf("DefineType alias: %v", err)
	}
	if _, err := r.DefineType("x5bad", NewTypeLiteral(TInteger)); err == nil {
		t.Fatal("a lowercase type name must be rejected")
	}
	enum, err := r.DefineEnum("X5DE", NewTypeLiteral(TInteger), NewTypeLiteral(TString))
	if err != nil || enum == nil {
		t.Fatalf("DefineEnum: %v", err)
	}
}

func TestX5DefineMemberType(t *testing.T) {
	r := x5reg(t)
	def, err := r.DefineMemberType("X5DM", TInteger, func(v Value) bool {
		n, e := AsInteger(v)
		return e == nil && n > 0
	})
	if err != nil || def == nil {
		t.Fatalf("DefineMemberType: %v", err)
	}
	if r.LookupTypeName("X5DM") == nil {
		t.Fatal("member type binds its name")
	}

	// Name validation failure.
	if _, err := r.DefineMemberType("x5dm", TInteger, func(Value) bool { return true }); err == nil {
		t.Fatal("a lowercase member-type name must be rejected")
	}

	// A family SubtypeNamer rule on the parent chain applies.
	namer := x5UserType("X5Fam", TInteger, x5SubNamer{})
	if _, err := r.DefineMemberType("X5DN", namer, func(Value) bool { return true }); err == nil {
		t.Fatal("the family naming rule must reject the name")
	}
}

// --- exit_error.go --------------------------------------------------------

func TestX5ExitError(t *testing.T) {
	e := NewExitError(3, "src", SrcPos{Row: 1, Col: 1})
	if e.Code != ExitErrorCode {
		t.Fatalf("exit error carries the reserved code, got %q", e.Code)
	}
	code, ok := ExitCode(e)
	if !ok || code != 3 {
		t.Fatalf("expected (3,true), got (%d,%v)", code, ok)
	}
	if _, ok := ExitCode(errors.New("plain")); ok {
		t.Fatal("a plain error is not an exit request")
	}
	// An exit error whose Data lost its code reports 0.
	bare := NewExitError(7, "", SrcPos{})
	bare.Data = nil
	code, ok = ExitCode(bare)
	if !ok || code != 0 {
		t.Fatalf("dataless exit reports (0,true), got (%d,%v)", code, ok)
	}
	// A non-integer code entry also reports 0.
	odd := NewExitError(7, "", SrcPos{})
	odd.Data = NewOrderedMap()
	odd.Data.Set(ExitCodeKey, NewString("x"))
	code, ok = ExitCode(odd)
	if !ok || code != 0 {
		t.Fatalf("non-integer code reports (0,true), got (%d,%v)", code, ok)
	}
}

// --- ideal.go -------------------------------------------------------------

func TestX5IdealRegistry(t *testing.T) {
	ir := NewIdealRegistry()
	ir.Register(nil) // guarded
	ir.Register(&Ideal{Name: ""})
	if len(ir.Names()) != 0 {
		t.Fatal("nil/unnamed ideals are not registered")
	}

	first := &Ideal{Name: "X5K", Enabled: true, Accepts: func(Value) bool { return true }}
	ir.Register(first)
	second := &Ideal{Name: "X5K2", Enabled: true}
	ir.Register(second)
	// Re-registering a name replaces in place, keeping resolver order.
	replacement := &Ideal{Name: "X5K", Enabled: true, Accepts: func(Value) bool { return false }}
	ir.Register(replacement)
	names := ir.Names()
	if len(names) != 2 || names[0] != "X5K" || names[1] != "X5K2" {
		t.Fatalf("replacement keeps position, got %v", names)
	}
	if ir.Get("X5K") != replacement {
		t.Fatal("the name index points at the replacement")
	}

	var nilIR *IdealRegistry
	if nilIR.Get("X5K") != nil {
		t.Fatal("nil registry Gets nil")
	}
	if nilIR.Match(NewInteger(1)) != nil {
		t.Fatal("nil registry Matches nil")
	}
	if nilIR.Names() != nil {
		t.Fatal("nil registry Names nil")
	}
}

// --- analysis_hooks.go ----------------------------------------------------

func TestX5AnalysisAccessors(t *testing.T) {
	r := x5reg(t)
	if r.analysisCompiling() {
		t.Fatal("a fresh registry is not compiling")
	}
	r.noteAnalysisDiagnostic(CheckDiagnostic{Code: "x5", Detail: "d"})
	r.noteSuppressedRuntimeError()
	r.noteAmbiguousGradualSplit()
	if !r.Check.SuppressedRuntimeError || !r.Check.AmbiguousGradualSplit {
		t.Fatal("latches must set their flags")
	}
	// The inactive AnalysisImpl defaults answer conservatively.
	if r.analysisMixedConform(NewInteger(1), TInteger) {
		t.Fatal("inactive MixedConform is false")
	}
	if r.analysisAtUncaughtTopLevel() {
		t.Fatal("inactive AtUncaughtTopLevel is false")
	}
	r.noteAnalysisUniqueDiagnostic(CheckDiagnostic{Code: "x5u", Detail: "d"})
}

func TestX5AnalysisStepMeterBudget(t *testing.T) {
	r := x5reg(t)
	end := r.Check.Begin()
	defer end()
	r.Check.StepBudget = 0
	if !r.analysisStepMeter() {
		t.Fatal("a zero budget trips immediately")
	}
	if !r.Check.BudgetTripped {
		t.Fatal("the trip latches")
	}
	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "step_budget_exceeded" {
			found = true
		}
	}
	if !found {
		t.Fatal("the one budget diagnostic is emitted")
	}
	// A second overshoot does not duplicate the diagnostic.
	if !r.analysisStepMeter() {
		t.Fatal("still exceeded")
	}
}

// --- argsstack.go ---------------------------------------------------------

func TestX5ArgsStackDepthTruncate(t *testing.T) {
	var nilAS *ArgsStack
	if nilAS.Depth() != 0 {
		t.Fatal("nil stack depth is 0")
	}
	nilAS.Truncate(0) // nil-safe

	as := NewArgsStack()
	_ = as.Push(NewList(nil))
	_ = as.Push(NewList(nil))
	if as.Depth() != 2 {
		t.Fatalf("expected depth 2, got %d", as.Depth())
	}
	as.Truncate(-1) // guarded
	as.Truncate(5)  // at/above depth: no-op
	if as.Depth() != 2 {
		t.Fatal("guarded truncates must not change depth")
	}
	as.Truncate(1)
	if as.Depth() != 1 {
		t.Fatalf("truncate to 1, got %d", as.Depth())
	}
}

// --- boru_error.go / carrier_new.go ---------------------------------------

func TestX5MakeBoruErrorAtAndSigArgs(t *testing.T) {
	e := MakeBoruErrorAt("type_error", "detail", "w", "src", "hint", SrcPos{Row: 2, Col: 3})
	if e == nil || e.Code != "type_error" {
		t.Fatalf("positioned error mints, got %v", e)
	}
	if got := describeSigArgs(nil); got != "(no args)" {
		t.Fatalf("nil signature describes as (no args), got %q", got)
	}
}

func TestX5CarrierOfLiteral(t *testing.T) {
	c := CarrierOfLiteral(NewTypeLiteral(TInteger))
	if !c.Carrier {
		t.Fatal("the result is a carrier")
	}
	if c.Parent == nil || !c.Parent.Equal(TInteger) {
		t.Fatalf("the carrier's parent is the literal's node, got %v", c.Parent)
	}
	if !strings.Contains(c.Parent.String(), "Integer") {
		t.Fatalf("unexpected parent render %q", c.Parent.String())
	}
}
