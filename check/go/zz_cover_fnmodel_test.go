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
	if !rc.Carrier() || !core.IsDynamic(&rc) || !rc.Parent().Equal(core.TMap) {
		t.Errorf("schema carrier shape = %+v, want a dynamic Map carrier", rc)
	}
	ri, isRec := rc.Data().(core.RecordTypeInfo)
	if !isRec || ri.Fields == nil {
		t.Fatalf("carrier Data = %T, want RecordTypeInfo carrying the pattern schema", rc.Data())
	}
	if _, has := ri.Fields.Get("f"); !has {
		t.Error("schema carrier lost the pattern's field")
	}
	// A fresh, DISTINCT ID per carrier (the frame-local slot key).
	rc2, _ := recordSchemaCarrier(p, core.NewCarrier(core.TMap))
	if core.IDOf(&rc) == "" || core.IDOf(&rc) == core.IDOf(&rc2) {
		t.Errorf("carrier IDs must be fresh and distinct, got %q and %q", core.IDOf(&rc), core.IDOf(&rc2))
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
	if !core.IsDisjunct(&v) || !v.Carrier() {
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
	if _, isRec := out[0].Data().(core.RecordTypeInfo); !isRec {
		t.Errorf("a record-shape param arg must narrow to the schema carrier, got %+v", out[0])
	}
}

func TestZzFmNarrowArgsStrictDisjunct(t *testing.T) {
	dj := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TBoolean), core.NewTypeLiteral(core.TString),
	})
	dj.SetCarrier(true)
	params := []core.FnParam{{Name: "b", Type: core.TString}}
	out := narrowArgsToParams([]core.Value{dj}, params)
	if len(out) != 1 || out[0].Parent() == nil || !out[0].Parent().Equal(core.TString) {
		t.Fatalf("a strict disjunct carrier must narrow to the declared type, got %v", out)
	}
	if core.IsDynamic(&out[0]) {
		t.Error("the strict-disjunct narrowing stays strict")
	}
}

func TestZzFmNarrowArgsStrictAnyToAnyParam(t *testing.T) {
	params := []core.FnParam{{Name: "nd", Type: core.TAny}}
	out := narrowArgsToParams([]core.Value{core.NewCarrier(core.TAny)}, params)
	if len(out) != 1 || !core.IsDynamic(&out[0]) || out[0].Parent() == nil || !out[0].Parent().Equal(core.TAny) {
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
	if len(out) != 1 || out[0].Parent() == nil || !out[0].Parent().ConformsTo(core.TInteger) {
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
	core.SetEval(&inner, true)
	sig := core.FnSig{
		Params: []core.FnParam{{Name: "zzfmp1", Type: core.TInteger}},
		Impl:   core.Boru([]core.Value{inner}),
	}
	rf := BuildFnBodyReturnsFn(r, "zzfmdefer", sig, core.FnDefInfo{Anonymous: true})
	out := rf([]core.Value{core.NewInteger(3)}, r)
	if len(out) != 1 || out[0].Parent() == nil || !out[0].Parent().Equal(core.TList) {
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
	if len(out) != 1 || !out[0].Carrier() || !core.IsDynamic(&out[0]) {
		t.Fatalf("record-return result = %#v, want one dynamic schema carrier", out)
	}
	rt, ok := out[0].Data().(core.RecordTypeInfo)
	if !ok || rt.Fields == nil {
		t.Fatalf("record-return Data = %#v, want the RecordTypeInfo schema", out[0].Data())
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
	if len(out) != 1 || out[0].Parent() == nil || !out[0].Parent().Equal(core.TInteger) {
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
	fd, isFn := core.FnDefOf(out[0].Data())
	if !isFn || fd.Name != "zzfminner" {
		t.Fatalf("out[0] = %+v, want the concrete closure value surfaced", out[0])
	}
	if !core.IsConcrete(&out[0]) || !out[0].Parent().ConformsTo(core.TFunction) {
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
	if len(out) != 1 || !core.IsDisjunct(&out[0]) || !out[0].Carrier() {
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
	if len(out) != 1 || !core.IsDynamic(&out[0]) || !out[0].Parent().Equal(core.TAny) {
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
	if len(out) != 1 || !out[0].Carrier() || core.IsDynamic(&out[0]) || !out[0].Parent().Equal(core.TAny) {
		t.Fatalf("zero-net residual = %+v, want one STRICT Any carrier", out)
	}
}
