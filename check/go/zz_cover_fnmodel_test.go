package check

// Standalone-coverage tests for the fn-analysis models (check_fnmodel.go)
// and the fn-body machinery (check_fnbody.go): the param->carrier builders
// (recordSchemaCarrier / ParamBodyCarrier / narrowArgsToParams), the
// residual-stack probes, the body-span walker, the disjointness proof, the
// return-conformance mirror, the record-shape check, and the ReturnsFn
// builder's arms — the armed-compile generalisation and the poly-plan
// record path observed through an EmitRecorder stub embedding the inactive
// recorder (the zzRecEmit idiom) and a DispatchBraid slot swap.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// fmRec is an EmitRecorder stub: the inactive recorder with the fn-unit
// compile group and the probes BuildFnBodyReturnsFn consults overridden so
// the armed-compile paths run compiler-less and their calls are observable.
type fmRec struct {
	core.EmitRecorder
	armed     bool
	inClosure bool
	storedGr  bool
	startUnit int // unit id StartFnCompile returns; negative = decline

	uncomp    []string
	startArgs []core.Value
	finished  [][]core.Value
	pts       []*core.Type
	retPats   bool
	declSet   bool

	userUnits []int
	userOuts  [][]core.Value
	userPos   []core.SrcPos

	polyWords []string
	polyOuts  [][]core.Value
}

func newFmRec() *fmRec { return &fmRec{EmitRecorder: core.TheInactiveEmit} }

func (f *fmRec) Armed() bool                    { return f.armed }
func (f *fmRec) InClosureUnit() bool            { return f.inClosure }
func (f *fmRec) StoredGradualActive() bool      { return f.storedGr }
func (f *fmRec) MarkUncompilable(reason string) { f.uncomp = append(f.uncomp, reason) }

func (f *fmRec) StartFnCompile(_, _ string, _ *core.Registry, args []core.Value, _ []*core.Type, _ []string, _ []core.CapturedBinding, _ bool, _ core.SrcPos) (int, func([]core.Value), bool) {
	if f.startUnit < 0 {
		return -1, nil, false
	}
	f.startArgs = append([]core.Value(nil), args...)
	return f.startUnit, func(stk []core.Value) { f.finished = append(f.finished, stk) }, true
}

func (f *fmRec) SetUnitParamTypes(_ int, pts []*core.Type, _ []*core.Value) { f.pts = pts }
func (f *fmRec) SetUnitReturnPatterns(int, []*core.Value)                   { f.retPats = true }
func (f *fmRec) SetUnitDecl(int, core.DeclSite)                             { f.declSet = true }

func (f *fmRec) RecordUserCall(unit int, _ string, _, outs []core.Value, pos, _ core.SrcPos) {
	f.userUnits = append(f.userUnits, unit)
	f.userOuts = append(f.userOuts, outs)
	f.userPos = append(f.userPos, pos)
}

func (f *fmRec) RecordUserPolyCall(word string, _ *core.Registry, _, _ []int, _ []core.SigImpl, _ []core.Signature, _ []core.Value, outs []core.Value, _ core.SrcPos, _ string, _ core.SrcPos) {
	f.polyWords = append(f.polyWords, word)
	f.polyOuts = append(f.polyOuts, outs)
}

// fmPolyPlan is a UserPolyPlan stub whose SubstituteJoinedOuts replaces the
// first out with a String carrier — a recognisable "return-join" stand-in.
type fmPolyPlan struct{ subbed bool }

func (p *fmPolyPlan) SubstituteJoinedOuts(out []core.Value) {
	p.subbed = true
	if len(out) > 0 {
		out[0] = core.NewCarrier(core.TString)
	}
}
func (p *fmPolyPlan) SigIdx() []int          { return []int{0} }
func (p *fmPolyPlan) Units() []int           { return []int{0} }
func (p *fmPolyPlan) Impls() []core.SigImpl  { return nil }
func (p *fmPolyPlan) Sigs() []core.Signature { return nil }

// fmRecordPattern builds a record-shape param pattern: a concrete map whose
// field constraints are bare type nodes.
func fmRecordPattern(pairs ...any) core.Value { return mapOf(pairs...) }

// --- recordSchemaCarrier ----------------------------------------------------

func TestZzFmRecordSchemaCarrier(t *testing.T) {
	pat := fmRecordPattern("f", core.NewTypeLiteral(core.TInteger))
	p := core.FnParam{Name: "c", Pattern: &pat}

	rc, ok := recordSchemaCarrier(p, core.NewCarrier(core.TMap))
	if !ok {
		t.Fatal("record-shape pattern + Map carrier arg must produce a schema carrier")
	}
	if !rc.Carrier || !rc.Dynamic || !rc.Parent.Equal(core.TMap) {
		t.Errorf("schema carrier shape = %+v, want a dynamic Map carrier", rc)
	}
	ri, isRec := rc.Data.(core.RecordTypeInfo)
	if !isRec || ri.Fields == nil {
		t.Fatalf("carrier Data = %T, want RecordTypeInfo carrying the pattern schema", rc.Data)
	}
	if _, has := ri.Fields.Get("f"); !has {
		t.Error("schema carrier lost the pattern's field")
	}
	// A fresh, DISTINCT ID per carrier (the frame-local slot key).
	rc2, _ := recordSchemaCarrier(p, core.NewCarrier(core.TMap))
	if rc.ID == "" || rc.ID == rc2.ID {
		t.Errorf("carrier IDs must be fresh and distinct, got %q and %q", rc.ID, rc2.ID)
	}

	// A pattern that is NOT a concrete map (a typed-map ChildTypeInfo
	// pattern) falls through to the final decline.
	tm := core.NewTypedMap(core.NewTypeLiteral(core.TInteger))
	if _, ok := recordSchemaCarrier(core.FnParam{Pattern: &tm}, core.NewCarrier(core.TMap)); ok {
		t.Error("a non-MapPayload pattern must decline")
	}
}

// --- ParamBodyCarrier -------------------------------------------------------

func TestZzFmParamBodyCarrierInlineUnion(t *testing.T) {
	pat := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString),
	})
	v := ParamBodyCarrier(core.FnParam{Name: "x", Pattern: &pat})
	if !core.IsDisjunct(v) || !v.Carrier {
		t.Fatalf("inline-union param carrier = %+v, want a strict disjunct carrier", v)
	}
	di, err := core.AsDisjunct(v)
	if err != nil || len(di.Alternatives) == 0 {
		t.Fatalf("AsDisjunct: %v (%d alts)", err, len(di.Alternatives))
	}
	if !di.Declared {
		t.Error("an inline union is a DECLARED domain — Declared must be set")
	}
}

// --- narrowArgsToParams -----------------------------------------------------

func TestZzFmNarrowArgsRecordSchema(t *testing.T) {
	pat := fmRecordPattern("f", core.NewTypeLiteral(core.TInteger))
	params := []core.FnParam{{Name: "c", Type: core.TMap, Pattern: &pat}}
	args := []core.Value{core.NewCarrier(core.TMap)}
	out := narrowArgsToParams(args, params)
	if len(out) != 1 {
		t.Fatalf("narrowed args = %v", out)
	}
	if _, isRec := out[0].Data.(core.RecordTypeInfo); !isRec {
		t.Errorf("a record-shape param arg must narrow to the schema carrier, got %+v", out[0])
	}
}

func TestZzFmNarrowArgsStrictDisjunct(t *testing.T) {
	dj := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TBoolean), core.NewTypeLiteral(core.TString),
	})
	dj.Carrier = true
	params := []core.FnParam{{Name: "b", Type: core.TString}}
	out := narrowArgsToParams([]core.Value{dj}, params)
	if len(out) != 1 || out[0].Parent == nil || !out[0].Parent.Equal(core.TString) {
		t.Fatalf("a strict disjunct carrier must narrow to the declared type, got %v", out)
	}
	if out[0].Dynamic {
		t.Error("the strict-disjunct narrowing stays strict")
	}
}

func TestZzFmNarrowArgsStrictAnyToAnyParam(t *testing.T) {
	params := []core.FnParam{{Name: "nd", Type: core.TAny}}
	out := narrowArgsToParams([]core.Value{core.NewCarrier(core.TAny)}, params)
	if len(out) != 1 || !out[0].Dynamic || out[0].Parent == nil || !out[0].Parent.Equal(core.TAny) {
		t.Fatalf("a strict Any arg on a declared-Any param must lift to dynamic(Any), got %v", out)
	}
}

// --- stackHasApproxAny ------------------------------------------------------

func TestZzFmStackHasApproxAny(t *testing.T) {
	if !stackHasApproxAny([]core.Value{core.NewInteger(1), core.NewCarrier(core.TAny)}) {
		t.Error("a bare strict Any carrier is the approximation phantom")
	}
	// A DYNAMIC Any carrier is the gradual contract, not the phantom.
	if stackHasApproxAny([]core.Value{core.NewDynamicCarrier(core.TAny)}) {
		t.Error("a dynamic Any carrier must not read as the approximation phantom")
	}
	if stackHasApproxAny([]core.Value{core.NewCarrier(core.TInteger)}) {
		t.Error("a typed carrier must not read as the approximation phantom")
	}
}

// --- bodySpanEnd ------------------------------------------------------------

func TestZzFmBodySpanEndShapes(t *testing.T) {
	pe := core.NewParenExpr([]core.Value{
		s5beAt(core.NewWord("w"), 1, 2), s5beAt(core.NewInteger(1), 1, 9),
	})
	if end := bodySpanEnd([]core.Value{pe}); end.Row != 1 || end.Col != 9 {
		t.Errorf("paren-expr span end = %+v, want row 1 col 9", end)
	}
	lst := core.NewList([]core.Value{s5beAt(core.NewString("s"), 2, 4)})
	if end := bodySpanEnd([]core.Value{lst}); end.Row != 2 || end.Col != 4 {
		t.Errorf("list span end = %+v, want row 2 col 4", end)
	}
	mp := mapOf("k", s5beAt(core.NewInteger(3), 6, 8))
	if end := bodySpanEnd([]core.Value{mp}); end.Row != 6 || end.Col != 8 {
		t.Errorf("map span end = %+v, want row 6 col 8", end)
	}
}

// --- residualProvablyDisjoint ----------------------------------------------

func TestZzFmResidualProvablyDisjointArms(t *testing.T) {
	// A disjunct whose alternative is a BARE TYPE NODE probes via
	// CarrierOfLiteral; String alone is disjoint from Integer.
	dLit := core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TString)})
	if !residualProvablyDisjoint(dLit, core.TInteger) {
		t.Error("String-literal-only disjunct should be provably disjoint from Integer")
	}
	// One alternative NOT disjoint -> the whole disjunct is not.
	dInt := core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TInteger)})
	if residualProvablyDisjoint(dInt, core.TInteger) {
		t.Error("an Integer alternative kills the disjointness proof")
	}
	// A Function residual sits on the fn-value-call frontier: never provable.
	if residualProvablyDisjoint(core.NewCarrier(core.TFunction), core.TInteger) {
		t.Error("a Function residual must stay with the runtime RET check")
	}
	// An OPAQUE union residual (Parent=Disjunct, no readable payload).
	if residualProvablyDisjoint(core.NewCarrier(core.TDisjunct), core.TInteger) {
		t.Error("an opaque union residual proves nothing")
	}
}

func TestZzFmResidualProvablyDisjointMembership(t *testing.T) {
	r := newTestRegistry(t)
	member, err := r.DefineMemberType("ZzFmEven", core.TInteger, func(v core.Value) bool {
		n, e := core.AsInteger(v)
		return e == nil && n%2 == 0
	})
	if err != nil {
		t.Fatalf("DefineMemberType: %v", err)
	}
	// The declared type admits values by VALUE-level membership; the tag
	// proof says nothing — skip (not provably disjoint).
	if residualProvablyDisjoint(core.NewCarrier(core.TString), member) {
		t.Error("a membership-based declared type must not be nominally disproved")
	}
}

// --- checkBodyReturnConformance --------------------------------------------

func TestZzFmReturnConformanceInactiveAndSkips(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	// No declared returns: the mirror has nothing to check.
	checkBodyReturnConformance(r, "zzfmnone", nil, nil, 0, true,
		[]core.Value{core.NewInteger(1)}, core.SrcPos{}, core.SrcPos{})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("no-declared-returns call flagged: %+v", r.Check.Diagnostics)
	}

	// A declared Any return is the gradual contract: no flag.
	checkBodyReturnConformance(r, "zzfmany", []*core.Type{core.TAny}, nil, 0, false,
		[]core.Value{core.NewInteger(1)}, core.SrcPos{}, core.SrcPos{})
	// A dynamic residual is the gradual contract too.
	checkBodyReturnConformance(r, "zzfmdyn", []*core.Type{core.TInteger}, nil, 0, false,
		[]core.Value{core.NewDynamicCarrier(core.TAny)}, core.SrcPos{}, core.SrcPos{})
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("gradual shapes flagged: %+v", r.Check.Diagnostics)
	}
}

func TestZzFmReturnConformancePatternMismatch(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	// A declared union return degrades its *Type to Any; the pattern is the
	// only contract. A Boolean residual fails Unify against (Integer tor
	// String) and flags the SAME type_error the runtime raises.
	pat := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString),
	})
	stk := []core.Value{core.NewBoolean(true)}
	checkBodyReturnConformance(r, "zzfmpat", []*core.Type{core.TAny}, []*core.Value{&pat},
		0, false, stk, core.SrcPos{Row: 2, Col: 1}, core.SrcPos{Row: 3, Col: 1})
	var found int
	for _, d := range r.Check.Diagnostics {
		if d.Code == "type_error" && d.Word == "zzfmpat" {
			found++
			if !d.RuntimeMirror {
				t.Error("the pattern mismatch is a RuntimeMirror diagnostic")
			}
			if d.Row != 2 || d.Col != 1 {
				t.Errorf("diagnostic at %d:%d, want 2:1", d.Row, d.Col)
			}
		}
	}
	if found != 1 {
		t.Fatalf("pattern mismatch flagged %d times, want 1: %+v", found, r.Check.Diagnostics)
	}
	// Deduped by detail: the same analysed shape repeated adds nothing.
	checkBodyReturnConformance(r, "zzfmpat", []*core.Type{core.TAny}, []*core.Value{&pat},
		0, false, stk, core.SrcPos{Row: 2, Col: 1}, core.SrcPos{Row: 3, Col: 1})
	n := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "type_error" && d.Word == "zzfmpat" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("dedupe failed: %d type_error diagnostics", n)
	}
}

// --- fnBodyUndefinedWordShield ---------------------------------------------

func TestZzFmShieldOutOfSpanDiagnostic(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	// An undefined_word attributed OUTSIDE this body's span must not shield
	// this body (a different overload's forward ref).
	r.Check.Diagnostics = append(r.Check.Diagnostics, core.CheckDiagnostic{
		Code: "undefined_word", FnName: "zzfmsh", Row: 30, Col: 1,
	})
	if fnBodyUndefinedWordShield(r, "zzfmsh", core.SrcPos{Row: 1, Col: 1}, core.SrcPos{Row: 2, Col: 10}) {
		t.Error("a diagnostic beyond bodyEnd must not shield this body")
	}
	// One attributed INSIDE the span shields.
	r.Check.Diagnostics = append(r.Check.Diagnostics, core.CheckDiagnostic{
		Code: "undefined_word", FnName: "zzfmsh", Row: 1, Col: 5,
	})
	if !fnBodyUndefinedWordShield(r, "zzfmsh", core.SrcPos{Row: 1, Col: 1}, core.SrcPos{Row: 2, Col: 10}) {
		t.Error("an in-span undefined_word is the analysis-ran-early signal")
	}
}

// --- checkRecordShapeArgs ---------------------------------------------------

func TestZzFmCheckRecordShapeArgs(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	pat := fmRecordPattern(
		"n", core.NewTypeLiteral(core.TInteger),
		"m", core.NewTypeLiteral(core.TString),
	)
	pats := []*core.Value{&pat}

	// Missing field: the arg overlaps (has n) but lacks m.
	checkRecordShapeArgs(r, "zzfmrec", pats, []core.Value{mapOf("n", core.NewInteger(1))})
	var missing bool
	for _, d := range r.Check.Diagnostics {
		if d.Code == "record_shape_mismatch" && strings.Contains(d.Detail, "missing field: m") {
			missing = true
		}
	}
	if !missing {
		t.Fatalf("missing-field diagnostic absent: %+v", r.Check.Diagnostics)
	}

	// Wrong-typed field flags with expected/got.
	checkRecordShapeArgs(r, "zzfmrec", pats, []core.Value{
		mapOf("n", core.NewString("x"), "m", core.NewString("y")),
	})
	var badType bool
	for _, d := range r.Check.Diagnostics {
		if d.Code == "record_shape_mismatch" && strings.Contains(d.Detail, "field n expected") {
			badType = true
		}
	}
	if !badType {
		t.Fatalf("wrong-typed-field diagnostic absent: %+v", r.Check.Diagnostics)
	}

	// Zero key overlap silently skips (the synthetic/default analysis map).
	n := len(r.Check.Diagnostics)
	checkRecordShapeArgs(r, "zzfmrec", pats, []core.Value{mapOf("q", core.NewInteger(1))})
	// An EMPTY arg map skips too.
	checkRecordShapeArgs(r, "zzfmrec", pats, []core.Value{mapOf()})
	// A non-map pattern is not a record shape.
	intPat := core.NewInteger(3)
	checkRecordShapeArgs(r, "zzfmrec", []*core.Value{&intPat}, []core.Value{mapOf("n", core.NewInteger(1))})
	if len(r.Check.Diagnostics) != n {
		t.Errorf("skipped shapes flagged: %+v", r.Check.Diagnostics[n:])
	}
}

// --- BuildFnBodyReturnsFn ---------------------------------------------------

// Anonymous lambdas drop the declared-returns fast path: the analyser's
// inferred residual wins.
func TestZzFmReturnsFnAnonymousDropsDeclared(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	sig := core.FnSig{
		Returns: []*core.Type{core.TString}, // the afn placeholder, dropped
		Impl:    core.Boru([]core.Value{core.NewInteger(5)}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmanon", sig, core.FnDefInfo{Anonymous: true})
	out := rf(nil, r)
	if len(out) != 1 || out[0].Parent == nil || !out[0].Parent.ConformsTo(core.TInteger) {
		t.Fatalf("anonymous fn result = %v, want the inferred Integer residual", out)
	}
}

// A deferred-list body on an ANONYMOUS fn returns the raw list residual
// (no unit; module-scope fold later).
func TestZzFmReturnsFnDeferredListAnonymous(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	inner := core.NewList([]core.Value{core.NewWord("zzfmp1")})
	inner.Eval = true
	sig := core.FnSig{
		Params: []core.FnParam{{Name: "zzfmp1", Type: core.TInteger}},
		Impl:   core.Boru([]core.Value{inner}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmdefer", sig, core.FnDefInfo{Anonymous: true})
	out := rf([]core.Value{core.NewInteger(3)}, r)
	if len(out) != 1 || out[0].Parent == nil || !out[0].Parent.Equal(core.TList) {
		t.Fatalf("deferred-list residual = %v, want the raw list", out)
	}
}

// NUR068: a declared RECORD return (TMap + field-bag ReturnPattern)
// resolves to the schema-bearing dynamic(Map)+RecordTypeInfo carrier —
// the same shape make/param produce — instead of a bare Map carrier.
func TestZzFmReturnsFnRecordSchemaCarrier(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	fields := core.NewOrderedMap()
	fields.Set("name", core.NewTypeLiteral(core.TString))
	pat := core.NewMap(fields)
	sig := core.FnSig{
		Returns:        []*core.Type{core.TMap},
		ReturnPatterns: []*core.Value{&pat},
		Impl:           core.Boru([]core.Value{core.NewInteger(1)}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmrec", sig, core.FnDefInfo{})
	out := rf(nil, r)
	if len(out) != 1 || !out[0].Carrier || !out[0].Dynamic {
		t.Fatalf("record-return result = %#v, want one dynamic schema carrier", out)
	}
	rt, ok := out[0].Data.(core.RecordTypeInfo)
	if !ok || rt.Fields == nil {
		t.Fatalf("record-return Data = %#v, want the RecordTypeInfo schema", out[0].Data)
	}
}

// NUR068's residual-surfacing twin: a fn declared to return a BARE Map
// whose body residual is a record-SCHEMA carrier surfaces the schema on
// the plain pass (cloned, fresh identity) and keeps the shapeless
// declared carrier on the compile pass.
func TestZzFmReturnsFnBareMapSurfacesRecordResidual(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	fields := core.NewOrderedMap()
	fields.Set("name", core.NewTypeLiteral(core.TString))
	res := core.NewDynamicCarrierValue(core.NewRecordType(fields))
	sig := core.FnSig{
		Returns: []*core.Type{core.TMap},
		Impl:    core.Boru([]core.Value{res}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmsurf", sig, core.FnDefInfo{})
	out := rf(nil, r)
	if len(out) != 1 || !out[0].Carrier {
		t.Fatalf("surfaced result = %#v, want one carrier", out)
	}
	rt, ok := out[0].Data.(core.RecordTypeInfo)
	if !ok || rt.Fields == nil {
		t.Fatalf("surfaced Data = %#v, want the residual's RecordTypeInfo", out[0].Data)
	}
	if out[0].ID == "" || out[0].ID == res.ID {
		t.Error("surfaced residual must carry a freshly minted identity")
	}

	// The COMPILE pass keeps the declared bare Map carrier (the recorded
	// call results and the RET contract are untouched).
	r.Check.Compiling = true
	defer func() { r.Check.Compiling = false }()
	out2 := rf(nil, r)
	if len(out2) != 1 {
		t.Fatalf("compile-pass result = %#v, want one carrier", out2)
	}
	if _, isRec := out2[0].Data.(core.RecordTypeInfo); isRec {
		t.Error("the compile pass must keep the shapeless declared Map carrier")
	}
}

// A generic fn whose return names an INFERABLE type parameter refines to
// the call's binding (no unbound_param report).
func TestZzFmReturnsFnGenBindingInferred(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	p := core.GenParam{Name: "ZzFmT"}
	node := core.MintTypeParam(r, p)
	spec := &core.GenSpecInfo{Params: []core.GenParam{p}}
	sig := core.FnSig{
		Params:  []core.FnParam{{Name: "zzfmx", Type: node}},
		Returns: []*core.Type{node},
		Impl:    core.Boru([]core.Value{core.NewWord("zzfmx")}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmid", sig, core.FnDefInfo{Gen: spec})
	out := rf([]core.Value{core.NewInteger(4)}, r)
	if len(out) != 1 || out[0].Parent == nil || !out[0].Parent.Equal(core.TInteger) {
		t.Fatalf("inferred-binding return = %v, want an Integer carrier", out)
	}
	for _, d := range r.Check.Diagnostics {
		if d.Code == "unbound_param" {
			t.Errorf("inferable parameter reported unbound: %s", d.Detail)
		}
	}
}

// A fn declared to return a Function whose body produced a CONCRETE closure
// surfaces that value (cloned, fresh ID) rather than an abstract carrier.
func TestZzFmReturnsFnConcreteClosureSurfaces(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	body := []core.Value{core.NewWord("zzfmunused")}
	sig := core.FnSig{
		Returns: []*core.Type{core.TFunction},
		Impl:    core.Boru(body),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmclose", sig, core.FnDefInfo{})
	// Seed the analysis memo with the concrete closure residual so the body
	// pass hands it back (the returned-lambda shape) without re-analysis.
	inner := core.NewFunction(core.FnDefInfo{Name: "zzfminner"})
	if r.Check.FnSummaries == nil {
		r.Check.FnSummaries = map[string][]core.Value{}
	}
	key := FnAnalysisKey(r.AnalysisScopeID(), "zzfmclose", nil, nil, body)
	r.Check.FnSummaries[key] = []core.Value{inner}

	out := rf(nil, r)
	if len(out) != 1 {
		t.Fatalf("out = %v", out)
	}
	fd, isFn := out[0].Data.(core.FnDefInfo)
	if !isFn || fd.Name != "zzfminner" {
		t.Fatalf("out[0] = %+v, want the concrete closure value surfaced", out[0])
	}
	if !core.IsConcrete(out[0]) || !out[0].Parent.ConformsTo(core.TFunction) {
		t.Errorf("surfaced closure = %+v, want a concrete Function value", out[0])
	}
}

// A declared USER-UNION return distributes: the result is a strict disjunct
// carrier of the union's alternatives.
func TestZzFmReturnsFnUnionReturnDistributes(t *testing.T) {
	r := newTestRegistry(t)
	alts := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString),
	})
	if err := core.InstallType(r, "ZzFmU", alts); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	ut := r.LookupTypeName("ZzFmU")
	if ut == nil {
		t.Fatal("union type not resolvable after install")
	}
	done := r.Check.Begin()
	defer done()

	sig := core.FnSig{
		Params:  []core.FnParam{{Name: "zzfmn", Type: core.TInteger}},
		Returns: []*core.Type{ut},
		Impl:    core.Boru([]core.Value{core.NewInteger(2)}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmuni", sig, core.FnDefInfo{})
	out := rf([]core.Value{core.NewInteger(1)}, r)
	if len(out) != 1 || !core.IsDisjunct(out[0]) || !out[0].Carrier {
		t.Fatalf("union return = %+v, want a strict disjunct carrier", out)
	}
}

// A declared `Any` return is "statically unknown": dynamic for optimistic
// downstream matching.
func TestZzFmReturnsFnDeclaredAnyIsDynamic(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	sig := core.FnSig{
		Returns: []*core.Type{core.TAny},
		Impl:    core.Boru([]core.Value{core.NewInteger(1)}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmanyret", sig, core.FnDefInfo{})
	out := rf(nil, r)
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(core.TAny) {
		t.Fatalf("declared-Any return = %+v, want dynamic(Any)", out)
	}
}

// The 0-net / undeclared call whose body unit declined leaves the lenient
// STRICT Any approximation.
func TestZzFmReturnsFnZeroNetLeavesStrictAny(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	sig := core.FnSig{Impl: core.Boru(nil)} // empty body, no declared returns
	rf := BuildFnBodyReturnsFn(r, "zzfmzero", sig, core.FnDefInfo{})
	out := rf(nil, r)
	if len(out) != 1 || !out[0].Carrier || out[0].Dynamic || !out[0].Parent.Equal(core.TAny) {
		t.Fatalf("zero-net residual = %+v, want one STRICT Any carrier", out)
	}
}

// The closure-unit quote-capture screen: an Atom-typed param bound to a
// computed (carrier) value inside a closure unit latches uncompilable.
func TestZzFmReturnsFnQuoteParamClosureScreen(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	rec := newFmRec()
	rec.inClosure = true
	r.Check.Emit = rec

	sig := core.FnSig{
		Params: []core.FnParam{{Name: "k", Type: core.TAtom}},
		Impl:   core.Boru([]core.Value{core.NewInteger(1)}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmquote", sig, core.FnDefInfo{})
	rf([]core.Value{core.NewCarrier(core.TAtom)}, r)
	found := false
	for _, reason := range rec.uncomp {
		if strings.Contains(reason, "quote capture is forward-only") &&
			strings.Contains(reason, "zzfmquote") {
			found = true
		}
	}
	if !found {
		t.Errorf("closure quote-capture screen did not latch: %v", rec.uncomp)
	}
}

// The armed compile: genArgs generalisation arm by arm, the unit start /
// finish protocol, and the declared-returns RecordUserCall.
func TestZzFmReturnsFnArmedCompile(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	rec := newFmRec()
	rec.armed = true
	rec.storedGr = true
	rec.startUnit = 3
	r.Check.Emit = rec

	recPat := fmRecordPattern("f", core.NewTypeLiteral(core.TInteger))
	tmPat := core.NewTypedMap(core.NewTypeLiteral(core.TInteger))
	sig := core.FnSig{
		Params: []core.FnParam{
			{Name: "a", Type: core.TMap, Pattern: &recPat}, // record schema
			{Name: "b", Type: core.TMap, Pattern: &tmPat},  // typed container
			{Name: "c", Type: core.TInteger},               // recovered Any->declared
			{Name: "d", Type: core.TString},                // disjunct carrier narrow
			{Name: "e", Type: core.TAny},                   // stored-gradual Any->Any
			{Name: "g"},                                    // root-node carrier keep
			{Name: "h", Type: core.TInteger},               // plain generalisation
		},
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru([]core.Value{s5beAt(core.NewInteger(1), 5, 3)}),
	}
	dj := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TBoolean), core.NewTypeLiteral(core.TString),
	})
	dj.Carrier = true
	dj.Dynamic = true
	args := []core.Value{
		s5beAt(core.NewCarrier(core.TMap), 7, 1),
		core.NewCarrier(core.TMap),
		core.NewCarrier(core.TAny),
		dj,
		core.NewCarrier(core.TAny),
		core.NewTypeLiteral(core.TAny), // the Any root node: Parent nil
		core.NewInteger(9),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmarm", sig, core.FnDefInfo{})
	out := rf(args, r)

	if len(rec.startArgs) != 7 {
		t.Fatalf("StartFnCompile saw %d genArgs, want 7", len(rec.startArgs))
	}
	if _, isRec := rec.startArgs[0].Data.(core.RecordTypeInfo); !isRec {
		t.Errorf("genArgs[0] = %+v, want the record schema carrier", rec.startArgs[0])
	}
	if !core.IsTypedMap(rec.startArgs[1]) || !rec.startArgs[1].Dynamic {
		t.Errorf("genArgs[1] = %+v, want a dynamic typed-map carrier", rec.startArgs[1])
	}
	if !rec.startArgs[2].Parent.Equal(core.TInteger) {
		t.Errorf("genArgs[2] = %+v, want the declared Integer carrier (recovered call)", rec.startArgs[2])
	}
	if !rec.startArgs[3].Parent.Equal(core.TString) || !rec.startArgs[3].Dynamic {
		t.Errorf("genArgs[3] = %+v, want dynamic String (disjunct narrowed, dynamic preserved)", rec.startArgs[3])
	}
	if !rec.startArgs[4].Parent.Equal(core.TAny) || !rec.startArgs[4].Dynamic {
		t.Errorf("genArgs[4] = %+v, want gradual Any (stored-gradual generalisation)", rec.startArgs[4])
	}
	if rec.startArgs[5].Parent != nil || !rec.startArgs[5].Carrier || rec.startArgs[5].Data != nil {
		t.Errorf("genArgs[5] = %+v, want the root-node carrier kept (Carrier set, no payload)", rec.startArgs[5])
	}
	if !rec.startArgs[6].Parent.Equal(core.TInteger) || !rec.startArgs[6].Carrier {
		t.Errorf("genArgs[6] = %+v, want a plain Integer carrier", rec.startArgs[6])
	}

	if len(rec.pts) != 7 {
		t.Errorf("SetUnitParamTypes saw %d types, want 7", len(rec.pts))
	}
	if !rec.retPats || !rec.declSet {
		t.Error("unit return patterns / decl site must be installed on a fresh unit")
	}
	if len(rec.finished) != 1 || len(rec.finished[0]) == 0 {
		t.Errorf("finish must receive the generalised residual, got %v", rec.finished)
	}
	if len(rec.userUnits) != 1 || rec.userUnits[0] != 3 {
		t.Fatalf("RecordUserCall units = %v, want [3]", rec.userUnits)
	}
	if len(rec.userOuts[0]) != 1 || !rec.userOuts[0][0].Parent.Equal(core.TInteger) {
		t.Errorf("recorded outs = %v, want the declared Integer carrier", rec.userOuts[0])
	}
	if rec.userPos[0].Row != 7 || rec.userPos[0].Col != 1 {
		t.Errorf("recorded pos = %+v, want the first arg's position 7:1", rec.userPos[0])
	}
	if len(out) != 1 || !out[0].Parent.Equal(core.TInteger) {
		t.Errorf("out = %v, want the declared Integer carrier", out)
	}
}

// StartFnCompile declining (okFn=false) drops to fnUnit=-1: no unit calls.
func TestZzFmReturnsFnArmedCompileDeclined(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	rec := newFmRec()
	rec.armed = true
	rec.startUnit = -1
	r.Check.Emit = rec

	sig := core.FnSig{
		Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru([]core.Value{core.NewInteger(1)}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmdecl", sig, core.FnDefInfo{})
	out := rf([]core.Value{core.NewInteger(2)}, r)
	if len(out) != 1 || !out[0].Parent.Equal(core.TInteger) {
		t.Fatalf("out = %v", out)
	}
	if len(rec.userUnits) != 0 || rec.declSet || len(rec.finished) != 0 {
		t.Error("a declined unit must record nothing")
	}
}

// An armed fn with NO declared returns and an EMPTY residual records a
// 0-output CALL_USER and returns zero values.
func TestZzFmReturnsFnArmedZeroResidual(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	rec := newFmRec()
	rec.armed = true
	rec.startUnit = 5
	r.Check.Emit = rec

	sig := core.FnSig{
		Params: []core.FnParam{{Name: "n", Type: core.TInteger}},
		Impl:   core.Boru(nil),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmz0", sig, core.FnDefInfo{})
	out := rf([]core.Value{s5beAt(core.NewInteger(2), 8, 2)}, r)
	if out != nil {
		t.Fatalf("out = %v, want nil (zero runtime values)", out)
	}
	if len(rec.userUnits) != 1 || rec.userUnits[0] != 5 || rec.userOuts[0] != nil {
		t.Fatalf("RecordUserCall = units %v outs %v, want unit 5 with nil outs", rec.userUnits, rec.userOuts)
	}
	if rec.userPos[0].Row != 8 || rec.userPos[0].Col != 2 {
		t.Errorf("recorded pos = %+v, want 8:2", rec.userPos[0])
	}
}

// An armed UNDECLARED fn with a non-empty residual records the residual as
// the call's outs and returns it.
func TestZzFmReturnsFnArmedUndeclaredResidual(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	rec := newFmRec()
	rec.armed = true
	rec.startUnit = 6
	r.Check.Emit = rec

	sig := core.FnSig{
		Params: []core.FnParam{{Name: "n", Type: core.TInteger}},
		Impl:   core.Boru([]core.Value{core.NewInteger(1)}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmres", sig, core.FnDefInfo{})
	out := rf([]core.Value{s5beAt(core.NewInteger(2), 9, 4)}, r)
	if len(out) == 0 {
		t.Fatal("undeclared fn must return the body residual")
	}
	if len(rec.userUnits) != 1 || rec.userUnits[0] != 6 || len(rec.userOuts[0]) != len(out) {
		t.Errorf("RecordUserCall = units %v outs %v", rec.userUnits, rec.userOuts)
	}
	if rec.userPos[0].Row != 9 || rec.userPos[0].Col != 4 {
		t.Errorf("recorded pos = %+v, want 9:4", rec.userPos[0])
	}
}

// The poly-plan record path (declared returns): the plan substitutes the
// return-join carriers and the runtime-re-matched call is recorded.
func TestZzFmReturnsFnPolyPlanDeclared(t *testing.T) {
	oldPlan := DispatchBraid.PlanUserPoly
	plan := &fmPolyPlan{}
	DispatchBraid.PlanUserPoly = func(*core.Registry, core.EmitRecorder, string, []core.Value, []*core.Type) (UserPolyPlan, bool) {
		return plan, false
	}
	defer func() { DispatchBraid.PlanUserPoly = oldPlan }()

	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	rec := newFmRec() // unarmed: fnUnit stays -1, the plan owns the record
	r.Check.Emit = rec

	sig := core.FnSig{
		Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru([]core.Value{core.NewInteger(1)}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmpoly", sig, core.FnDefInfo{})
	out := rf([]core.Value{core.NewInteger(2)}, r)
	if !plan.subbed {
		t.Fatal("the plan's return-join substitution must run")
	}
	if len(out) != 1 || !out[0].Parent.Equal(core.TString) {
		t.Errorf("out = %v, want the plan's joined String carrier", out)
	}
	if len(rec.polyWords) != 1 || rec.polyWords[0] != "zzfmpoly" {
		t.Errorf("RecordUserPolyCall words = %v", rec.polyWords)
	}
}

// The ZERO-declared-return poly set: a 0-output runtime-re-matched poly
// call is recorded and the call returns zero values.
func TestZzFmReturnsFnPolyPlanZeroDeclared(t *testing.T) {
	oldPlan := DispatchBraid.PlanUserPoly
	plan := &fmPolyPlan{}
	DispatchBraid.PlanUserPoly = func(*core.Registry, core.EmitRecorder, string, []core.Value, []*core.Type) (UserPolyPlan, bool) {
		return plan, false
	}
	defer func() { DispatchBraid.PlanUserPoly = oldPlan }()

	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	rec := newFmRec()
	r.Check.Emit = rec

	sig := core.FnSig{
		Params: []core.FnParam{{Name: "n", Type: core.TInteger}},
		Impl:   core.Boru(nil),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmpoly0", sig, core.FnDefInfo{})
	out := rf([]core.Value{core.NewInteger(2)}, r)
	if out != nil {
		t.Fatalf("out = %v, want nil (zero runtime values)", out)
	}
	if len(rec.polyWords) != 1 || rec.polyWords[0] != "zzfmpoly0" || rec.polyOuts[0] != nil {
		t.Errorf("RecordUserPolyCall = %v outs %v, want zzfmpoly0 with nil outs", rec.polyWords, rec.polyOuts)
	}
}
