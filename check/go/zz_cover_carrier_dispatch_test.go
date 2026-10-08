package check

// zz_cover_carrier_dispatch_test.go — white-box coverage for the
// check-mode dispatch result model in carrier.go: CarrierResults and
// its three-way return resolution (declaredReturnCarriers), the
// special-word short-circuits (specialWordResults), gradual contagion
// (applyGradualContagion), the strict-disjunct partition family
// (disjunctPartitionReturns / alternativeCarriers / joinReturnRows /
// firstMatchingSig) and the dynamic-reachability return unions
// (dynamicReachableValueReturns / dynamicReachableReturns).
//
// Every test drives the DOCUMENTED behaviour from the function's doc
// comment; fixture words follow the corpus convention of a `q` suffix.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// covCDImpl is a trivial native implementation for fixture signatures —
// check-mode analysis never invokes it.
func covCDImpl() *core.GoImpl {
	return core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return args, nil
	})
}

// covCDEmit is a minimal EmitRecorder test double: every method
// delegates to the shared inactive no-op recorder, with the activity
// probes overridable and RecordTrap captured — the two behaviours the
// dispatch model consults (CarrierResults' armed-recording gate,
// specialWordResults' closure-unit decline and macroexpand trap).
type covCDEmit struct {
	core.EmitRecorder
	active    bool
	inClosure bool
	traps     []string
}

func (e *covCDEmit) Active() bool        { return e.active }
func (e *covCDEmit) InClosureUnit() bool { return e.inClosure }
func (e *covCDEmit) RecordTrap(code, _, word, _ string, _ core.SrcPos) bool {
	e.traps = append(e.traps, code+"@"+word)
	return true
}

// covCDDisjunct builds a STRICT disjunct carrier (Carrier=true,
// Dynamic=false) over the given alternative types; declared marks it a
// declared-union param binding (DisjunctInfo.Declared).
func covCDDisjunct(declared bool, types ...*core.Type) core.Value {
	alts := make([]core.Value, len(types))
	for i, tp := range types {
		alts[i] = core.NewTypeLiteral(tp)
	}
	v := core.NewValueRaw(core.TDisjunct, core.DisjunctInfo{Alternatives: alts, Declared: declared})
	v.Carrier = true
	return v
}

// covCDMacro installs a macro fixture: a zero-operand macro whose
// template body is the given token list (run per expansion; its last
// result is the template).
func covCDMacro(r *core.Registry, name string, body []core.Value) {
	core.InstallDef(r, name, core.NewFunction(core.FnDefInfo{
		Name:       name,
		Macro:      true,
		Signatures: []core.Signature{{Impl: core.Boru(body)}},
	}))
}

// covCDDiagCodes projects the diagnostics down to their codes.
func covCDDiagCodes(r *core.Registry) []string {
	out := make([]string, 0, len(r.Check.Diagnostics))
	for _, d := range r.Check.Diagnostics {
		out = append(out, d.Code)
	}
	return out
}

// --- CarrierResults --------------------------------------------------------

func TestCarrierResultsSpecialWordShortCircuit(t *testing.T) {
	r := newTestRegistry(t)
	// `word [body]` is resolved by specialWordResults before ordinary
	// return resolution: the __SP splice marker comes back as the one
	// result, bypassing declared-return resolution entirely (no sig
	// needed — the short-circuit returns first).
	out := CarrierResults(r, "word", nil, []core.Value{core.NewList([]core.Value{core.NewWord("x")})},
		core.SrcPos{Row: 1, Col: 1}, r, false)
	if len(out) != 1 || !core.IsSplice(out[0]) {
		t.Fatalf("word splice: got %d results (splice=%v), want 1 splice marker",
			len(out), len(out) == 1 && core.IsSplice(out[0]))
	}
}

func TestCarrierResultsDisjunctStraddlePolyLowering(t *testing.T) {
	r := newTestRegistry(t)
	// Integer and String take DIFFERENT overloads: the strict-disjunct
	// straddle CarrierResults resolves per alternative.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "pstrq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString}, BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	done := r.Check.Begin()
	defer done()
	fn := r.Lookup("pstrq")
	sig := &fn.Signatures[0]
	args := []core.Value{covCDDisjunct(false, core.TInteger, core.TString)}
	// No armed recording (plain check): the partition result is returned
	// directly — the per-alternative join, not the single matched sig's
	// declared return.
	out := CarrierResults(r, "pstrq", sig, args, core.SrcPos{Row: 2, Col: 3}, r, false)
	if len(out) != 1 || !core.IsDisjunct(out[0]) || !out[0].Carrier {
		t.Fatalf("straddle result = %#v, want one disjunct carrier (the per-alternative join)", out)
	}
	if got := len(core.FlattenAlternatives(out[0])); got != 2 {
		t.Errorf("joined alternatives = %d, want 2 (Integer and String)", got)
	}
}

func TestCarrierResultsPartitionedUserFnPrecision(t *testing.T) {
	r := newTestRegistry(t)
	// A USER FN (BoruImpl with a frame) whose every alternative
	// first-matches a sig with the SAME run implementation: under an
	// ARMED recording the dispatch falls through to the ordinary path
	// (one recorded call) and the partition-joined carriers are re-IDed
	// onto the recorded results at the tail.
	shared := &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}, FnFrame: &core.FnFrameMeta{Name: "ucolq"}}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "ucolq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: shared},
			{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString}, BarrierPos: 1, Impl: shared},
		},
	})
	done := r.Check.Begin()
	defer done()
	r.Check.Emit = &covCDEmit{EmitRecorder: core.TheInactiveEmit, active: true}
	fn := r.Lookup("ucolq")
	sig := &fn.Signatures[0]
	args := []core.Value{covCDDisjunct(false, core.TInteger, core.TString)}
	out := CarrierResults(r, "ucolq", sig, args, core.SrcPos{Row: 3, Col: 1}, r, false)
	// The committed sig declares a single scalar return; the partition
	// join is the per-alternative Integer|String union. Getting the
	// disjunct back proves the partition carriers (not the recorded
	// declared return) were handed downstream.
	if len(out) != 1 || !core.IsDisjunct(out[0]) {
		t.Fatalf("partitioned user-fn result = %#v, want the per-alternative disjunct", out)
	}
}

func TestCarrierResultsPartitionedUserFnCountMismatch(t *testing.T) {
	r := newTestRegistry(t)
	shared := &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}, FnFrame: &core.FnFrameMeta{Name: "ucol2q"}}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "ucol2q",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: shared},
			{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString}, BarrierPos: 1, Impl: shared},
		},
	})
	done := r.Check.Begin()
	defer done()
	r.Check.Emit = &covCDEmit{EmitRecorder: core.TheInactiveEmit, active: true}
	// The committed sig declares TWO returns while the partition joins
	// ONE per row: "a count mismatch keeps the recorded results verbatim
	// — sound, just wider."
	committed := &core.Signature{
		Args:       []*core.Type{core.TScalar},
		Returns:    []*core.Type{core.TInteger, core.TInteger},
		BarrierPos: 1,
		Impl:       shared,
	}
	args := []core.Value{covCDDisjunct(false, core.TInteger, core.TString)}
	out := CarrierResults(r, "ucol2q", committed, args, core.SrcPos{Row: 4, Col: 1}, r, false)
	if len(out) != 2 {
		t.Fatalf("count mismatch must keep the recorded results: got %d, want 2", len(out))
	}
	for i, v := range out {
		if core.IsDisjunct(v) || v.Parent == nil || !v.Parent.Equal(core.TInteger) {
			t.Errorf("result %d = %#v, want the committed sig's declared Integer carrier", i, v)
		}
	}
}

func TestCarrierResultsScalarConstFold(t *testing.T) {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "feqq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TBoolean}, BarrierPos: 2, Impl: covCDImpl()},
		},
	})
	done := r.Check.Begin()
	defer done()
	// Arm the S3 fold slot the way the compiler piece does at init: the
	// comparison "ran for real" over compile-time-known scalars, so the
	// concrete result must replace the declared-type carrier.
	orig := DispatchBraid.TryFoldScalarConst
	DispatchBraid.TryFoldScalarConst = func(_ *core.Registry, _ *core.Signature, _ []core.Value) (core.Value, bool) {
		return core.NewBoolean(false), true
	}
	t.Cleanup(func() { DispatchBraid.TryFoldScalarConst = orig })
	fn := r.Lookup("feqq")
	sig := &fn.Signatures[0]
	out := CarrierResults(r, "feqq", sig,
		[]core.Value{core.NewInteger(0), core.NewInteger(0)}, core.SrcPos{Row: 5, Col: 1}, r, false)
	if len(out) != 1 || !core.IsConcrete(out[0]) {
		t.Fatalf("folded dispatch = %#v, want one CONCRETE result replacing the Boolean carrier", out)
	}
	if b, err := core.AsBoolean(out[0]); err != nil || b {
		t.Errorf("folded value = (%v, %v), want the fold's concrete false", b, err)
	}
}

// --- specialWordResults ----------------------------------------------------

func TestSpecialWordResultsArgsFrameProjection(t *testing.T) {
	r := newTestRegistry(t)
	pos := core.SrcPos{Row: 1, Col: 1}

	// Top level: the args stack is empty, so `args` falls through to the
	// normal path (RecordCall's compile failure in a compile; nothing here).
	if out, ok := specialWordResults(r, "args", nil, pos); ok || out != nil {
		t.Fatalf("empty args stack must fall through, got (%v, %v)", out, ok)
	}

	// Inside a compiled fn body the frame's params project as a list
	// value with no recorded event.
	top := core.NewList([]core.Value{core.NewInteger(4)})
	if err := r.Args.Push(top); err != nil {
		t.Fatalf("Args.Push: %v", err)
	}
	out, ok := specialWordResults(r, "args", nil, pos)
	if !ok || len(out) != 1 {
		t.Fatalf("concrete frame top must project, got (%v, %v)", out, ok)
	}
	l, err := core.AsList(out[0])
	if err != nil || len(l.Slice()) != 1 {
		t.Fatalf("projection = %#v, want the pushed one-param list", out[0])
	}
	if n, err := core.AsInteger(l.Slice()[0]); err != nil || n != 4 {
		t.Errorf("projected param = (%d, %v), want 4", n, err)
	}

	// An ARMED recording outside a closure unit keeps the projection.
	r.Check.Emit = &covCDEmit{EmitRecorder: core.TheInactiveEmit, active: true}
	if _, ok := specialWordResults(r, "args", nil, pos); !ok {
		t.Error("armed non-closure recording must keep the args projection")
	}
	// Inside a CLOSURE body compile the projection must DECLINE: the
	// analysis frame holds the CallableSpec inputs while the runtime
	// reads the ENCLOSING fn's per-call list.
	r.Check.Emit = &covCDEmit{EmitRecorder: core.TheInactiveEmit, active: true, inClosure: true}
	if out, ok := specialWordResults(r, "args", nil, pos); ok || out != nil {
		t.Fatalf("closure-unit compile must decline the projection, got (%v, %v)", out, ok)
	}
	r.Check.Emit = nil

	// A non-concrete frame top (a carrier) has nothing to project.
	if err := r.Args.Push(core.NewCarrier(core.TList)); err != nil {
		t.Fatalf("Args.Push: %v", err)
	}
	if out, ok := specialWordResults(r, "args", nil, pos); ok || out != nil {
		t.Fatalf("carrier frame top must fall through, got (%v, %v)", out, ok)
	}
}

func TestSpecialWordResultsWordSpliceMarker(t *testing.T) {
	r := newTestRegistry(t)
	pos := core.SrcPos{Row: 1, Col: 1}
	body := core.NewList([]core.Value{core.NewWord("x")})
	out, ok := specialWordResults(r, "word", []core.Value{body}, pos)
	if !ok || len(out) != 1 || !core.IsSplice(out[0]) {
		t.Fatalf("word [body] = (%v, %v), want one __SP splice marker", out, ok)
	}
	// Only the one-arg shape is the splice form.
	if out, ok := specialWordResults(r, "word", []core.Value{body, body}, pos); ok || out != nil {
		t.Errorf("two-arg word must fall through, got (%v, %v)", out, ok)
	}
	// An unrelated word is not special at all.
	if out, ok := specialWordResults(r, "addq", []core.Value{body}, pos); ok || out != nil {
		t.Errorf("ordinary word must fall through, got (%v, %v)", out, ok)
	}
}

func TestSpecialWordResultsMacroexpandFold(t *testing.T) {
	r := newTestRegistry(t)
	// A deterministic macro: the template is the literal token list [7],
	// so both probe expansions agree and the expansion bakes as a const.
	covCDMacro(r, "cmacq", []core.Value{core.NewList([]core.Value{core.NewInteger(7)})})
	form := core.NewList([]core.Value{core.NewWord("cmacq")})
	out, ok := specialWordResults(r, "macroexpand", []core.Value{form}, core.SrcPos{Row: 1, Col: 1})
	if !ok || len(out) != 1 {
		t.Fatalf("macroexpand fold = (%v, %v), want the baked expansion", out, ok)
	}
	if !core.IsInertConst(out[0]) {
		t.Fatalf("baked expansion %#v must be an inert const", out[0])
	}
	l, err := core.AsList(out[0])
	if err != nil || len(l.Slice()) != 1 {
		t.Fatalf("expansion = %#v, want the one-token list [7]", out[0])
	}
	if n, err := core.AsInteger(l.Slice()[0]); err != nil || n != 7 {
		t.Errorf("expanded token = (%d, %v), want 7", n, err)
	}
}

func TestSpecialWordResultsMacroexpandRunawayTrap(t *testing.T) {
	r := newTestRegistry(t)
	// A macro that expands to (a call to) itself runs into the depth
	// guard's macroexpand_error — the interpreter raises exactly this at
	// run time, so the checker records the byte-identical terminal trap
	// and still falls through to decline the dispatch.
	covCDMacro(r, "rmacq", []core.Value{core.NewList([]core.Value{core.NewWord("rmacq")})})
	em := &covCDEmit{EmitRecorder: core.TheInactiveEmit}
	r.Check.Emit = em
	form := core.NewList([]core.Value{core.NewWord("rmacq")})
	out, ok := specialWordResults(r, "macroexpand", []core.Value{form}, core.SrcPos{Row: 1, Col: 1})
	if ok || out != nil {
		t.Fatalf("runaway macro must fall through to decline, got (%v, %v)", out, ok)
	}
	if len(em.traps) != 1 || em.traps[0] != "macroexpand_error@macroexpand" {
		t.Errorf("recorded traps = %v, want the one macroexpand_error trap", em.traps)
	}
}

func TestSpecialWordResultsMacroexpandOtherErrorFallsThrough(t *testing.T) {
	r := newTestRegistry(t)
	// A macro whose template produces no value fails with macro_error —
	// NOT a runtime error to reproduce, so no trap is recorded and the
	// dispatch falls through to decline.
	covCDMacro(r, "emacq", nil)
	em := &covCDEmit{EmitRecorder: core.TheInactiveEmit}
	r.Check.Emit = em
	form := core.NewList([]core.Value{core.NewWord("emacq")})
	out, ok := specialWordResults(r, "macroexpand", []core.Value{form}, core.SrcPos{Row: 1, Col: 1})
	if ok || out != nil {
		t.Fatalf("failing expansion must fall through, got (%v, %v)", out, ok)
	}
	if len(em.traps) != 0 {
		t.Errorf("recorded traps = %v, want none for a non-macroexpand_error failure", em.traps)
	}
}

// --- declaredReturnCarriers ------------------------------------------------

func TestDeclaredReturnCarriersMissingReturns(t *testing.T) {
	r := newTestRegistry(t)
	// Explicit nil Returns (no annotation) triggers the missing_returns
	// fallback: one DYNAMIC Any carrier so the unannotated word does not
	// cascade false no_signature errors downstream.
	sig := &core.Signature{Impl: covCDImpl()}
	out := declaredReturnCarriers(r, "annq", sig, nil, core.SrcPos{Row: 2, Col: 5})
	if len(out) != 1 || out[0].Parent == nil || !out[0].Parent.Equal(core.TAny) {
		t.Fatalf("fallback = %#v, want one Any carrier", out)
	}
	if !out[0].Dynamic || !out[0].Carrier {
		t.Errorf("fallback carrier Dynamic=%v Carrier=%v, want a DYNAMIC Any carrier", out[0].Dynamic, out[0].Carrier)
	}
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %v, want the one missing_returns", covCDDiagCodes(r))
	}
	d := r.Check.Diagnostics[0]
	if d.Code != "missing_returns" || d.Word != "annq" || d.Row != 2 || d.Col != 5 {
		t.Errorf("diagnostic = %+v, want missing_returns at annq 2:5", d)
	}

	// An empty but NON-nil slice is a valid "returns nothing"
	// declaration — no fallback, no diagnostic.
	empty := &core.Signature{Returns: []*core.Type{}, Impl: covCDImpl()}
	if out := declaredReturnCarriers(r, "annq", empty, nil, core.SrcPos{Row: 3, Col: 1}); len(out) != 0 {
		t.Errorf("empty non-nil Returns = %#v, want no results", out)
	}
	if len(r.Check.Diagnostics) != 1 {
		t.Errorf("returns-nothing declaration must not add a diagnostic, got %v", covCDDiagCodes(r))
	}

	// A declared Any return means "statically unknown": the fresh
	// carrier rides dynamic so typed consumers downstream still match.
	anySig := &core.Signature{Returns: []*core.Type{core.TAny, core.TInteger}, Impl: covCDImpl()}
	out = declaredReturnCarriers(r, "getq", anySig, nil, core.SrcPos{Row: 4, Col: 1})
	if len(out) != 2 || !out[0].Dynamic || out[1].Dynamic {
		t.Errorf("declared [Any Integer] = %#v, want dynamic Any + strict Integer", out)
	}
}

// TestDeclaredReturnCarriersRecordSchema pins NUR068's declared-return
// resolution: a record return (TMap + field-bag ReturnPattern) builds
// the schema-bearing dynamic(Map)+RecordTypeInfo carrier instead of a
// bare Map carrier, so field reads through the word narrow. A pattern
// that is not a field-bag map (Options) keeps the plain Map carrier.
func TestDeclaredReturnCarriersRecordSchema(t *testing.T) {
	r := newTestRegistry(t)
	fields := core.NewOrderedMap()
	fields.Set("name", core.NewTypeLiteral(core.TString))
	pat := core.NewMap(fields)
	sig := &core.Signature{
		Returns:        []*core.Type{core.TMap},
		ReturnPatterns: []*core.Value{&pat},
		Impl:           covCDImpl(),
	}
	out := declaredReturnCarriers(r, "mkq", sig, nil, core.SrcPos{Row: 5, Col: 1})
	if len(out) != 1 || !out[0].Carrier || !out[0].Dynamic {
		t.Fatalf("record return = %#v, want one dynamic carrier", out)
	}
	rt, ok := out[0].Data.(core.RecordTypeInfo)
	if !ok || rt.Fields == nil {
		t.Fatalf("record return Data = %#v, want the RecordTypeInfo schema", out[0].Data)
	}
	if ft, ok := rt.Fields.Get("name"); !ok || !core.ValueType(ft).Equal(core.TString) {
		t.Fatalf("schema field name = %#v, want the String constraint", rt.Fields)
	}
	if out[0].ID == "" {
		t.Error("record return carrier must mint a fresh identity")
	}

	// An Options pattern is NOT a record schema: the plain Map carrier
	// (ChildTypeInfo payload) stands.
	opat := core.NewOptionsType(fields)
	osig := &core.Signature{
		Returns:        []*core.Type{core.TMap},
		ReturnPatterns: []*core.Value{&opat},
		Impl:           covCDImpl(),
	}
	oout := declaredReturnCarriers(r, "mkq", osig, nil, core.SrcPos{Row: 6, Col: 1})
	if len(oout) != 1 {
		t.Fatalf("options return = %#v, want one carrier", oout)
	}
	if _, isRec := oout[0].Data.(core.RecordTypeInfo); isRec {
		t.Fatal("an Options pattern must not build a record-schema carrier")
	}
}

// --- recordReturnCarrier / returnPatternAt (NUR068 unit pins) --------------

func TestRecordReturnCarrierGates(t *testing.T) {
	fields := core.NewOrderedMap()
	fields.Set("n", core.NewTypeLiteral(core.TInteger))
	pat := core.NewMap(fields)

	if _, ok := recordReturnCarrier(nil, &pat); ok {
		t.Error("nil type must decline")
	}
	if _, ok := recordReturnCarrier(core.TInteger, &pat); ok {
		t.Error("a non-Map declared type must decline")
	}
	if _, ok := recordReturnCarrier(core.TMap, nil); ok {
		t.Error("a patternless return must decline")
	}
	opat := core.NewOptionsType(fields)
	if _, ok := recordReturnCarrier(core.TMap, &opat); ok {
		t.Error("an Options pattern must decline")
	}
	rc, ok := recordReturnCarrier(core.TMap, &pat)
	if !ok || !rc.Carrier || !rc.Dynamic || rc.ID == "" {
		t.Fatalf("field-bag pattern = (%#v, %v), want a fresh dynamic schema carrier", rc, ok)
	}
	if rt, isRec := rc.Data.(core.RecordTypeInfo); !isRec || rt.Fields == nil {
		t.Fatalf("carrier Data = %#v, want RecordTypeInfo", rc.Data)
	}
}

func TestReturnPatternAtLeniency(t *testing.T) {
	pat := core.NewInteger(1)
	pats := []*core.Value{nil, &pat}
	if got := returnPatternAt(pats, 0); got != nil {
		t.Errorf("position 0 = %v, want nil", got)
	}
	if got := returnPatternAt(pats, 1); got != &pat {
		t.Errorf("position 1 = %v, want the stored pattern", got)
	}
	if got := returnPatternAt(pats, 2); got != nil {
		t.Errorf("out-of-range = %v, want nil", got)
	}
	if got := returnPatternAt(nil, 0); got != nil {
		t.Errorf("nil slice = %v, want nil", got)
	}
}

// --- applyGradualContagion -------------------------------------------------

func TestApplyGradualContagionStrictAdvisory(t *testing.T) {
	r := newTestRegistry(t)
	r.Check.Strict = true
	pos := core.SrcPos{Row: 7, Col: 2}
	args := []core.Value{core.NewDynamicCarrier(core.TInteger)}

	out := applyGradualContagion(r, "strq", args, []core.Value{core.NewCarrier(core.TInteger)}, pos, false)
	if len(out) != 1 || !out[0].Dynamic || !out[0].Carrier {
		t.Fatalf("contagion result = %#v, want the declared return flipped dynamic", out)
	}
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Code != "dynamic_dispatch" {
		t.Fatalf("diagnostics = %v, want the one strict dynamic_dispatch advisory", covCDDiagCodes(r))
	}
	// Deduped by word+position: re-analysing the same dispatch under
	// another call shape must not duplicate the advisory.
	applyGradualContagion(r, "strq", args, []core.Value{core.NewCarrier(core.TInteger)}, pos, false)
	if len(r.Check.Diagnostics) != 1 {
		t.Errorf("re-analysis duplicated the advisory: %v", covCDDiagCodes(r))
	}
}

func TestApplyGradualContagionMixedArityMutator(t *testing.T) {
	r := newTestRegistry(t)
	// `set`-shaped mixed arity: value-returning Map/List overloads next
	// to a returns-nothing String sibling.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "mutq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TMap, core.TInteger}, Returns: []*core.Type{core.TMap}, BarrierPos: 2, Impl: covCDImpl()},
			{Args: []*core.Type{core.TList, core.TInteger}, Returns: []*core.Type{core.TList}, BarrierPos: 2, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString, core.TInteger}, Returns: []*core.Type{}, BarrierPos: 2, Impl: covCDImpl()},
		},
	})
	pos := core.SrcPos{Row: 8, Col: 1}
	args := []core.Value{core.NewDynamicCarrier(core.TAny), core.NewCarrier(core.TInteger)}

	// The matched overload returns NOTHING but a dynamic receiver also
	// reaches the value-returning siblings: model ONE dynamic value — the
	// union of the reachable value returns.
	out := applyGradualContagion(r, "mutq", args, nil, pos, false)
	if len(out) != 1 || !core.IsDisjunct(out[0]) || !out[0].Dynamic {
		t.Fatalf("mixed-arity model = %#v, want one dynamic Map|List disjunct", out)
	}
	if got := len(core.FlattenAlternatives(out[0])); got != 2 {
		t.Errorf("modeled alternatives = %d, want 2 (Map and List)", got)
	}

	// A single reachable value return models that one type directly.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "mut1q",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TMap, core.TInteger}, Returns: []*core.Type{core.TMap}, BarrierPos: 2, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString, core.TInteger}, Returns: []*core.Type{}, BarrierPos: 2, Impl: covCDImpl()},
		},
	})
	out = applyGradualContagion(r, "mut1q", args, nil, pos, false)
	if len(out) != 1 || out[0].Parent == nil || !out[0].Parent.Equal(core.TMap) || !out[0].Dynamic {
		t.Fatalf("single-sibling model = %#v, want one dynamic Map carrier", out)
	}

	// Under a REAL compile the optimistic model fires ONLY in paren-tail
	// position: a free statement-position residual would be collected by
	// the NEXT word and corrupt its arity.
	r.Check.Compiling = true
	if out := applyGradualContagion(r, "mutq", args, nil, pos, false); len(out) != 0 {
		t.Errorf("statement-position compile must keep the true 0-arity, got %#v", out)
	}
	if out := applyGradualContagion(r, "mutq", args, nil, pos, true); len(out) != 1 {
		t.Errorf("paren-tail compile must model the consumed value, got %#v", out)
	}
	r.Check.Compiling = false
}

// --- disjunctPartitionReturns ----------------------------------------------

func TestDisjunctPartitionReturnsJoinAndDiagnostics(t *testing.T) {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "pcolq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString}, BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "pdq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	done := r.Check.Begin()
	defer done()
	pos := core.SrcPos{Row: 9, Col: 4}

	// Both alternatives dispatch: the per-alternative returns join
	// position-wise into the Integer|String union.
	out, ok := disjunctPartitionReturns(r, "pcolq", []core.Value{covCDDisjunct(false, core.TInteger, core.TString)}, pos)
	if !ok || len(out) != 1 || !core.IsDisjunct(out[0]) {
		t.Fatalf("partition = (%#v, %v), want the joined Integer|String carrier", out, ok)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("full partition must not diagnose, got %v", covCDDiagCodes(r))
	}

	// An ANALYSIS-join disjunct alternative that reaches no overload gets
	// a partial_dispatch WARNING and drops out of the join.
	out, ok = disjunctPartitionReturns(r, "pdq", []core.Value{covCDDisjunct(false, core.TInteger, core.TString)}, pos)
	if !ok || len(out) != 1 || out[0].Parent == nil || !out[0].Parent.Equal(core.TInteger) {
		t.Fatalf("partial partition = (%#v, %v), want the surviving Integer row", out, ok)
	}
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %v, want one partial_dispatch", covCDDiagCodes(r))
	}
	d := r.Check.Diagnostics[0]
	if d.Code != "partial_dispatch" || d.Severity != core.SeverityWarning ||
		!strings.Contains(d.Detail, "disjunct input") || !strings.Contains(d.Detail, "String") {
		t.Errorf("analysis-join diagnostic = %+v, want a warning naming the String alternative", d)
	}

	// A DECLARED union param's failing alternative is an ERROR: the body
	// is broken for part of its declared domain.
	out, ok = disjunctPartitionReturns(r, "pdq", []core.Value{covCDDisjunct(true, core.TInteger, core.TString)}, pos)
	if !ok || len(out) != 1 {
		t.Fatalf("declared-domain partition = (%#v, %v), want the surviving row", out, ok)
	}
	if len(r.Check.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %v, want a second partial_dispatch", covCDDiagCodes(r))
	}
	d = r.Check.Diagnostics[1]
	if d.Code != "partial_dispatch" || d.Severity != core.SeverityError ||
		!strings.Contains(d.Detail, "declared union parameter") {
		t.Errorf("declared-domain diagnostic = %+v, want an error naming the declared union", d)
	}

	// No alternative matches anything: the partition declines and the
	// engine's no_signature handling stands (each combo still warns).
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "pdnq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TBoolean}, Returns: []*core.Type{core.TBoolean}, BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	out, ok = disjunctPartitionReturns(r, "pdnq", []core.Value{covCDDisjunct(false, core.TInteger, core.TString)}, pos)
	if ok || out != nil {
		t.Fatalf("no-match partition = (%#v, %v), want a decline", out, ok)
	}
	if len(r.Check.Diagnostics) != 4 {
		t.Errorf("diagnostics = %v, want one warning per failing alternative", covCDDiagCodes(r))
	}
}

func TestDisjunctPartitionReturnsReturnsFnArm(t *testing.T) {
	r := newTestRegistry(t)
	// A ReturnsFn overload runs per alternative: identity over the
	// alternative carrier, so the join recovers Integer|String.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "prfq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TScalar},
				ReturnsFn:  func(args []core.Value, _ *core.Registry) []core.Value { return []core.Value{args[0]} },
				BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	done := r.Check.Begin()
	defer done()
	out, ok := disjunctPartitionReturns(r, "prfq",
		[]core.Value{covCDDisjunct(false, core.TInteger, core.TString)}, core.SrcPos{Row: 10, Col: 1})
	if !ok || len(out) != 1 || !core.IsDisjunct(out[0]) {
		t.Fatalf("ReturnsFn partition = (%#v, %v), want the joined Integer|String carrier", out, ok)
	}
}

func TestDisjunctPartitionReturnsDeclines(t *testing.T) {
	// A nil registry / inactive check pass declines outright.
	if out, ok := disjunctPartitionReturns(nil, "pcolq", nil, core.SrcPos{}); ok || out != nil {
		t.Error("nil registry must decline")
	}
	inactive := newTestRegistry(t)
	if out, ok := disjunctPartitionReturns(inactive, "pcolq",
		[]core.Value{covCDDisjunct(false, core.TInteger, core.TString)}, core.SrcPos{}); ok || out != nil {
		t.Error("inactive check pass must decline")
	}

	r := newTestRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "pcolq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString}, BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	// An unannotated sibling: a combo landing on it declines the whole
	// partition (missing_returns + dynamic Any beats a partial join).
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "punq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString}, BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	// Divergent return arity across alternatives: the join declines.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "pmisq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	done := r.Check.Begin()
	defer done()
	pos := core.SrcPos{Row: 11, Col: 1}
	strict := covCDDisjunct(false, core.TInteger, core.TString)

	// No strict disjunct arg: a plain carrier and a DYNAMIC disjunct both
	// take the whole-disjunct path.
	if out, ok := disjunctPartitionReturns(r, "pcolq", []core.Value{core.NewCarrier(core.TInteger)}, pos); ok || out != nil {
		t.Error("no strict disjunct arg must decline")
	}
	dyn := covCDDisjunct(false, core.TInteger, core.TString)
	dyn.Dynamic = true
	if out, ok := disjunctPartitionReturns(r, "pcolq", []core.Value{dyn}, pos); ok || out != nil {
		t.Error("a dynamic disjunct is not a strict partition operand")
	}
	// A concrete list operand: body-running ReturnsFns must not be
	// re-entered per alternative.
	if out, ok := disjunctPartitionReturns(r, "pcolq",
		[]core.Value{strict, core.NewList([]core.Value{core.NewInteger(1)})}, pos); ok || out != nil {
		t.Error("a concrete list operand must decline the partition")
	}
	// Unknown word.
	if out, ok := disjunctPartitionReturns(r, "no-such-q", []core.Value{strict}, pos); ok || out != nil {
		t.Error("an unknown word must decline")
	}
	// Cross product over the cap (17 > disjunctPartitionCap).
	wide := make([]*core.Type, 0, disjunctPartitionCap+1)
	for i := 0; i <= disjunctPartitionCap; i++ {
		if i%2 == 0 {
			wide = append(wide, core.TInteger)
		} else {
			wide = append(wide, core.TString)
		}
	}
	if out, ok := disjunctPartitionReturns(r, "pcolq", []core.Value{covCDDisjunct(false, wide...)}, pos); ok || out != nil {
		t.Error("a cross product over the cap must decline")
	}
	// Unannotated combo overload.
	if out, ok := disjunctPartitionReturns(r, "punq", []core.Value{strict}, pos); ok || out != nil {
		t.Error("an unannotated combo overload must decline the whole partition")
	}
	// Mismatched return arity across alternatives.
	if out, ok := disjunctPartitionReturns(r, "pmisq", []core.Value{strict}, pos); ok || out != nil {
		t.Error("mismatched return arity across alternatives must decline")
	}
}

// --- alternativeCarriers ---------------------------------------------------

func TestAlternativeCarriersExpansion(t *testing.T) {
	// A strict disjunct expands to one carrier per flattened alternative.
	alts := alternativeCarriers(covCDDisjunct(false, core.TInteger, core.TString))
	if len(alts) != 2 {
		t.Fatalf("expansion = %#v, want 2 per-alternative carriers", alts)
	}
	names := []string{core.TypeNameOf(alts[0]), core.TypeNameOf(alts[1])}
	if names[0] != "Integer" || names[1] != "String" {
		t.Errorf("alternative types = %v, want [Integer String]", names)
	}
	for i, a := range alts {
		if !a.Carrier || a.Dynamic {
			t.Errorf("alternative %d must be a strict carrier, got %#v", i, a)
		}
	}

	// Any other value yields itself.
	plain := core.NewCarrier(core.TInteger)
	if got := alternativeCarriers(plain); len(got) != 1 || got[0].ID != plain.ID {
		t.Errorf("a plain carrier must yield itself, got %#v", got)
	}
	dyn := covCDDisjunct(false, core.TInteger, core.TString)
	dyn.Dynamic = true
	if got := alternativeCarriers(dyn); len(got) != 1 || !core.IsDisjunct(got[0]) {
		t.Errorf("a dynamic disjunct must yield itself, got %#v", got)
	}
	conc := core.NewInteger(3)
	if got := alternativeCarriers(conc); len(got) != 1 || got[0].ID != conc.ID {
		t.Errorf("a concrete value must yield itself, got %#v", got)
	}
}

// --- joinReturnRows --------------------------------------------------------

func TestJoinReturnRowsLattice(t *testing.T) {
	// No surviving row: the partition declines.
	if out, ok := joinReturnRows(nil); ok || out != nil {
		t.Error("no rows must decline")
	}
	// Position-wise join of divergent rows: the Integer|String union.
	out, ok := joinReturnRows([][]core.Value{
		{core.NewCarrier(core.TInteger)},
		{core.NewCarrier(core.TString)},
	})
	if !ok || len(out) != 1 || !core.IsDisjunct(out[0]) || !out[0].Carrier {
		t.Fatalf("divergent join = (%#v, %v), want one Integer|String carrier", out, ok)
	}
	// Agreeing rows collapse to the shared type.
	out, ok = joinReturnRows([][]core.Value{
		{core.NewCarrier(core.TInteger)},
		{core.NewCarrier(core.TInteger)},
	})
	if !ok || len(out) != 1 || core.IsDisjunct(out[0]) || !out[0].Parent.Equal(core.TInteger) {
		t.Fatalf("agreeing join = (%#v, %v), want the one Integer carrier", out, ok)
	}
	// Rows disagreeing on return arity decline.
	if out, ok := joinReturnRows([][]core.Value{
		{core.NewCarrier(core.TInteger)},
		{core.NewCarrier(core.TInteger), core.NewCarrier(core.TInteger)},
	}); ok || out != nil {
		t.Error("arity disagreement must decline")
	}
	// All-zero-arity rows yield an empty slice with ok=true.
	if out, ok := joinReturnRows([][]core.Value{{}, {}}); !ok || len(out) != 0 {
		t.Errorf("zero-arity rows = (%#v, %v), want an empty ok row", out, ok)
	}
}

// --- firstMatchingSig ------------------------------------------------------

func TestFirstMatchingSigOrder(t *testing.T) {
	fn := &core.FnDefInfo{Signatures: []core.Signature{
		{Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TInteger}, Impl: covCDImpl()},
		{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString}, Impl: covCDImpl()},
	}}
	// The different-arity overload is skipped, not matched.
	s := firstMatchingSig(fn, []core.Value{core.NewCarrier(core.TString)})
	if s == nil || len(s.Returns) != 1 || !s.Returns[0].Equal(core.TString) {
		t.Fatalf("firstMatchingSig = %#v, want the one-arg String sig", s)
	}
	// First match in signature order for the two-arg shape.
	s = firstMatchingSig(fn, []core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TInteger)})
	if s == nil || !s.Returns[0].Equal(core.TInteger) {
		t.Fatalf("firstMatchingSig = %#v, want the two-arg Integer sig", s)
	}
	// A same-arity type mismatch reaches no signature.
	if s := firstMatchingSig(fn, []core.Value{core.NewCarrier(core.TBoolean)}); s != nil {
		t.Errorf("firstMatchingSig = %#v, want nil for an unmatched arg type", s)
	}
}

// --- dynamicReachableValueReturns ------------------------------------------

func TestDynamicReachableValueReturnsMixedArity(t *testing.T) {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "dvrq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TMap, core.TInteger}, Returns: []*core.Type{core.TMap}, BarrierPos: 2, Impl: covCDImpl()},
			{Args: []*core.Type{core.TList, core.TInteger}, Returns: []*core.Type{core.TList}, BarrierPos: 2, Impl: covCDImpl()},
			// The in-place mutator sibling: returns nothing, never a
			// VALUE return candidate.
			{Args: []*core.Type{core.TString, core.TInteger}, Returns: []*core.Type{}, BarrierPos: 2, Impl: covCDImpl()},
			// Arity filler: skipped for every two-arg call shape.
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	// A dynamic receiver reaches BOTH value-returning twins.
	rets := dynamicReachableValueReturns(r, "dvrq",
		[]core.Value{core.NewDynamicCarrier(core.TAny), core.NewCarrier(core.TInteger)})
	if len(rets) != 2 {
		t.Fatalf("dynamic receiver value returns = %v, want [Map List]", rets)
	}
	names := map[string]bool{rets[0].Leaf(): true, rets[1].Leaf(): true}
	if !names["Map"] || !names["List"] {
		t.Errorf("value-return types = %v, want Map and List", names)
	}
	// A CONCRETE receiver reaches only its own overload.
	rets = dynamicReachableValueReturns(r, "dvrq",
		[]core.Value{core.NewCarrier(core.TMap), core.NewCarrier(core.TInteger)})
	if len(rets) != 1 || rets[0].Leaf() != "Map" {
		t.Errorf("concrete receiver value returns = %v, want [Map] only", rets)
	}
	// An unknown word has no candidates.
	if rets := dynamicReachableValueReturns(r, "no-such-q", nil); rets != nil {
		t.Errorf("unknown word value returns = %v, want nil", rets)
	}
}

// --- dynamicReachableReturns -----------------------------------------------

func TestDynamicReachableReturnsUnion(t *testing.T) {
	r := newTestRegistry(t)
	// Divergent single-return overloads plus an unknown-return
	// (ReturnsFn) sibling.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "drrq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString}, BarrierPos: 1, Impl: covCDImpl()},
			{Args: []*core.Type{core.TScalar},
				ReturnsFn:  func(args []core.Value, _ *core.Registry) []core.Value { return []core.Value{args[0]} },
				BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	// All operands fully unknown (dynamic Any): the unknown-return
	// overload folds in as the Any alternative — gradual-permissive.
	rets := dynamicReachableReturns(r, "drrq", []core.Value{core.NewDynamicCarrier(core.TAny)})
	if len(rets) != 3 {
		t.Fatalf("all-unknown reachable returns = %v, want Integer, String and the folded Any", rets)
	}
	seen := map[string]bool{}
	for _, tp := range rets {
		seen[tp.Leaf()] = true
	}
	if !seen["Integer"] || !seen["String"] || !seen["Any"] {
		t.Errorf("reachable set = %v, want {Integer String Any}", seen)
	}
	// A PARTIALLY-known call must not be blanket-widened: a reachable
	// unknown-return overload suppresses refinement entirely.
	if rets := dynamicReachableReturns(r, "drrq", []core.Value{core.NewDynamicCarrier(core.TNumber)}); rets != nil {
		t.Errorf("partially-known reachable returns = %v, want nil (refinement suppressed)", rets)
	}

	// Without the unknown-return sibling the distinct concrete returns
	// refine as usual — but only when TWO OR MORE remain.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "drr2q",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString}, BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	if rets := dynamicReachableReturns(r, "drr2q", []core.Value{core.NewDynamicCarrier(core.TAny)}); len(rets) != 2 {
		t.Errorf("two-overload reachable returns = %v, want [Integer String]", rets)
	}
	if rets := dynamicReachableReturns(r, "drr2q", []core.Value{core.NewDynamicCarrier(core.TInteger)}); rets != nil {
		t.Errorf("single reachable return = %v, want nil (the matched sig is already correct)", rets)
	}
	// Unknown and single-signature words never refine.
	if rets := dynamicReachableReturns(r, "no-such-q", []core.Value{core.NewDynamicCarrier(core.TAny)}); rets != nil {
		t.Errorf("unknown word = %v, want nil", rets)
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "drrmonoq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: covCDImpl()},
		},
	})
	if rets := dynamicReachableReturns(r, "drrmonoq", []core.Value{core.NewDynamicCarrier(core.TAny)}); rets != nil {
		t.Errorf("single-signature word = %v, want nil", rets)
	}
}
