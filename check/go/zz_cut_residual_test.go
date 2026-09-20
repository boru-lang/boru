package check

// Residual pins and helpers assembled at the core cut (triage output).

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

var (
	_ = errors.New
	_ = fmt.Sprint
	_ = math.Pi
	_ = sort.Ints
	_ = strings.TrimSpace
	_ sync.Mutex
	_ = testing.Short
)

func TestOrderingDeterminate(t *testing.T) {
	if !orderingDeterminate(core.NewInteger(1)) {
		t.Error("concrete integer not determinate")
	}
	if !orderingDeterminate(core.NewNone()) {
		t.Error("none (singleton type) not determinate")
	}
	if orderingDeterminate(core.NewCarrier(core.TInteger)) {
		t.Error("bare carrier claims determinate")
	}
	dyn := core.NewInteger(1)
	dyn.Dynamic = true
	if orderingDeterminate(dyn) {
		t.Error("dynamic value claims determinate")
	}
}

func TestS5bENarrowArgsMoreArgsThanParams(t *testing.T) {
	args := []core.Value{core.NewInteger(1), core.NewInteger(2)}
	params := []core.FnParam{{Name: "a", Type: core.TInteger}}
	out := narrowArgsToParams(args, params)
	if out != nil && len(out) != len(args) {
		t.Errorf("unexpected narrowing result: %v", out)
	}
}

func TestS5bEBodySpanEndComputedReach(t *testing.T) {
	seg := core.ReachSeg{Computed: true, KeyExpr: []core.Value{s5beAt(core.NewWord("k"), 3, 7)}}
	reach := core.NewReach(core.ReachInfo{Receiver: []core.Value{s5beAt(core.NewWord("m"), 3, 1)}, Segments: []core.ReachSeg{seg}})
	end := bodySpanEnd([]core.Value{reach})
	if end.Row != 3 || end.Col != 7 {
		t.Errorf("bodySpanEnd = %+v, want row 3 col 7 (the computed key)", end)
	}
}

func TestS5bEResidualProvablyDisjointDisjunct(t *testing.T) {
	// Empty disjunct: nothing is provable.
	if residualProvablyDisjoint(core.NewDisjunct(nil), core.TInteger) {
		t.Error("empty disjunct must not be provably disjoint")
	}
	// Every alternative disjoint from the declared type → true.
	d := core.NewDisjunct([]core.Value{core.NewString("x")})
	if !residualProvablyDisjoint(d, core.TInteger) {
		t.Error("String-only disjunct should be provably disjoint from Integer")
	}
}

func TestS5bEReturnsFnDeferredParamListPopsGenBindings(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	p := core.GenParam{Name: "S5beT"}
	node := core.MintTypeParam(r, p)
	spec := &core.GenSpecInfo{Params: []core.GenParam{p}}

	// Body: a single evaluate-by-default list literal referencing the
	// param — the deferred-list shape.
	inner := core.NewList([]core.Value{core.NewWord("s5bec1")})
	inner.Eval = true
	sig := core.FnSig{
		Params: []core.FnParam{{Name: "s5bec1", Type: node}},
		Impl:   core.Boru([]core.Value{inner}),
	}
	rf := BuildFnBodyReturnsFn(r, "s5bemk", sig, core.FnDefInfo{Gen: spec})
	out := rf([]core.Value{core.NewInteger(9)}, r)
	if len(out) != 1 {
		t.Fatalf("expected the raw deferred list residual, got %v", out)
	}
	// The inferred T binding must have been popped again.
	if _, ok := r.Defs.Top("S5beT"); ok {
		t.Error("gen binding S5beT must be popped after the deferred-list return")
	}
}

func TestS5bEReturnsFnUnboundReturnTypeParamDedupes(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	p := core.GenParam{Name: "S5beU"}
	node := core.MintTypeParam(r, p)
	spec := &core.GenSpecInfo{Params: []core.GenParam{p}}
	// The param types never mention the placeholder, so the return's
	// type parameter is uninferable.
	sig := core.FnSig{
		Params:  []core.FnParam{{Name: "s5bew", Type: core.TInteger}},
		Returns: []*core.Type{node},
		Impl:    core.Boru([]core.Value{core.NewInteger(1)}),
	}
	rf := BuildFnBodyReturnsFn(r, "s5bei", sig, core.FnDefInfo{Gen: spec})

	out := rf([]core.Value{core.NewInteger(1)}, r)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("expected one dynamic(Any) carrier, got %v", out)
	}
	count := func() int {
		n := 0
		for _, d := range r.Check.Diagnostics {
			if d.Code == "unbound_param" {
				n++
			}
		}
		return n
	}
	first := count()
	if first == 0 {
		t.Fatal("expected an unbound_param diagnostic")
	}
	// Second identical call: the dedupe loop must find the duplicate and
	// not add another diagnostic.
	rf([]core.Value{core.NewInteger(1)}, r)
	if got := count(); got != first {
		t.Errorf("unbound_param diagnostics grew from %d to %d; dedupe failed", first, got)
	}
}

func TestS7DeadSignaturesShort(t *testing.T) {
	if got := core.DeadSignatures(nil); got != nil {
		t.Errorf("core.DeadSignatures(nil) = %v, want nil", got)
	}
	if got := core.DeadSignatures([]core.Signature{{Args: []*core.Type{core.TInteger}, BarrierPos: -1}}); got != nil {
		t.Errorf("core.DeadSignatures(single) = %v, want nil", got)
	}
}

func TestS7DeadSignaturesDuplicateAndFallback(t *testing.T) {
	sigA := core.Signature{Args: []*core.Type{core.TInteger}, BarrierPos: -1, Impl: core.Go(nil)}
	sigDup := core.Signature{Args: []*core.Type{core.TInteger}, BarrierPos: -1, Impl: core.Go(nil)}
	// A trailing fallback sig must be skipped, and the duplicate detected.
	fb := core.Signature{Fallback: true, BarrierPos: -1, Impl: core.Go(nil)}
	dead := core.DeadSignatures([]core.Signature{sigA, sigDup, fb})
	if len(dead) == 0 {
		t.Error("a duplicate signature should be flagged dead")
	}
	for _, d := range dead {
		if d.Sig.Fallback {
			t.Error("a fallback signature must never be reported dead")
		}
	}
}

func TestS7DeadSignaturesFallbackFirst(t *testing.T) {
	// A fallback with MORE args sorts BEFORE a normal sig (CompareSignatures
	// ignores Fallback, ranks by arg count) — so the inner loop encounters a
	// fallback at an earlier index than a non-fallback and must skip it.
	fb2 := core.Signature{Fallback: true, Args: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: -1, Impl: core.Go(nil)}
	norm := core.Signature{Args: []*core.Type{core.TInteger}, BarrierPos: -1, Impl: core.Go(nil)}
	if got := core.DeadSignatures([]core.Signature{norm, fb2}); len(got) != 0 {
		t.Errorf("no sig should be dead here, got %v", got)
	}
}

func TestUndefinedWordCheckDiagSuggestions(t *testing.T) {
	e := engWithTape(t, nil, 0)
	e.Registry.Defs.Push("counter", core.NewInteger(1))
	d := undefinedWordCheckDiag(e, "countr", core.SrcPos{Row: 2, Col: 3})
	if d.Code != "undefined_word" || d.Row != 2 || d.Col != 3 {
		t.Fatalf("diag = %+v", d)
	}
	if len(d.Suggestions) == 0 || !strings.Contains(d.Suggestions[0].Message, "`counter`") {
		t.Errorf("check diag must carry the did-you-mean, got %+v", d.Suggestions)
	}
	// No plausible match → no suggestions at all.
	d = undefinedWordCheckDiag(e, "qqqqqqqq", core.SrcPos{})
	if len(d.Suggestions) != 0 {
		t.Errorf("far name must carry no suggestions, got %+v", d.Suggestions)
	}
}

func TestS7CheckModeFallbackPositionsSkipsMarkers(t *testing.T) {
	// A Mark token after the pointer is skipped; the Integer beyond it is
	// gathered.
	e := engWithTape(t, []core.Value{core.NewInteger(0), core.NewMark("m"), core.NewInteger(9)}, 0)
	got := checkModeFallbackPositions(e, 1)
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("positions = %v, want [2] (skipped the Mark)", got)
	}
}

func TestW8SpliceFnCheckTailElseBranch(t *testing.T) {
	// nArgs exceeds the resolved values before valIdx: the else branch
	// clamps argStart and splices.
	e := engWithTape(t, []core.Value{core.NewInteger(1), core.NewInteger(2)}, 1)
	spliceFnCheckTail(e, 1, 2, []core.Value{core.NewCarrier(core.TAny)})
	if e.Pointer != 0 {
		t.Errorf("pointer = %d, want 0 (argStart clamped)", e.Pointer)
	}
}

func TestW8CheckModeSurfaceShapeNilParent(t *testing.T) {
	// A nil-Parent value at a fallback position is skipped; no surface
	// candidate is found → (false, nil).
	e := engWithTape(t, []core.Value{core.NewInteger(0), {}}, 0)
	ok, err := checkModeSurfaceShape(e, core.WordInfo{Name: "w8x", ArgCount: -1}, core.SrcPos{})
	if ok || err != nil {
		t.Errorf("got (%v, %v), want (false, nil)", ok, err)
	}
}

func TestCheckMakeConstruction(t *testing.T) {
	r := newTestRegistry(t)
	fields := core.NewOrderedMap()
	fields.Set("n", core.NewTypeLiteral(core.TInteger))
	fields.Set("d", core.NewInteger(7)) // defaulted field
	ct := r.Types.MintType("Class/Chk", core.TClass)
	cls := core.NewClassType(ct, core.ClassTypeInfo{Fields: fields, Name: "Chk", Type: ct})

	done := r.Check.Begin()
	defer done()

	// Well-formed construction: no diagnostics.
	CheckMakeConstruction(r, cls, mapOf("n", core.NewInteger(1)), core.SrcPos{Row: 1, Col: 1})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("clean construction flagged: %+v", r.Check.Diagnostics)
	}

	// Unknown field, missing field, and a wrong-typed field each flag.
	CheckMakeConstruction(r, cls, mapOf("bogus", core.NewInteger(1)), core.SrcPos{Row: 2, Col: 1})
	CheckMakeConstruction(r, cls, mapOf("n", core.NewString("x")), core.SrcPos{Row: 3, Col: 1})
	var unknown, missing, badType bool
	for _, d := range r.Check.Diagnostics {
		switch {
		case strings.Contains(d.Detail, "unknown field"):
			unknown = true
		case strings.Contains(d.Detail, "missing field"):
			missing = true
		case strings.Contains(d.Detail, `field "n"`):
			badType = true
		}
	}
	if !unknown || !missing || !badType {
		t.Errorf("diagnostics missing (unknown=%v missing=%v badType=%v): %+v",
			unknown, missing, badType, r.Check.Diagnostics)
	}

	// Dedup: repeating the same construction at the same position adds nothing.
	n := len(r.Check.Diagnostics)
	CheckMakeConstruction(r, cls, mapOf("bogus", core.NewInteger(1)), core.SrcPos{Row: 2, Col: 1})
	if len(r.Check.Diagnostics) != n {
		t.Errorf("duplicate diagnostics: %+v", r.Check.Diagnostics)
	}

	// Non-map / carrier sources and non-class targets are skipped.
	CheckMakeConstruction(r, cls, core.NewCarrier(core.TMap), core.SrcPos{})
	CheckMakeConstruction(r, cls, core.NewInteger(1), core.SrcPos{})
	CheckMakeConstruction(r, core.NewTypeLiteral(core.TInteger), mapOf("n", core.NewInteger(1)), core.SrcPos{})
	if len(r.Check.Diagnostics) != n {
		t.Errorf("skipped shapes flagged: %+v", r.Check.Diagnostics)
	}
}

func TestCheckMakeConstructionInactiveNoOp(t *testing.T) {
	r := newTestRegistry(t)
	// Outside check mode nothing happens (and nothing panics).
	CheckMakeConstruction(r, core.NewTypeLiteral(core.TInteger), mapOf("n", core.NewInteger(1)), core.SrcPos{})
	CheckMakeConstruction(nil, core.Value{}, core.Value{}, core.SrcPos{})
}

func TestPayloadMarkersCallable(t *testing.T) {
	// The sealed-payload markers are no-ops; calling them directly pins
	// that the types satisfy eng.Payload.
	mod := core.NewDispatchMod(core.DispatchModInfo{Val: true})
	if !core.IsDispatchMod(mod) {
		t.Error("dispatch-mod marker not recognised")
	}
	if info, ok := core.AsDispatchMod(mod); !ok || !info.Val {
		t.Errorf("AsDispatchMod = %+v, %v", info, ok)
	}
}

func TestMembershipBeyondNominal(t *testing.T) {
	r := newTestRegistry(t)
	member, err := r.DefineMemberType("W3Even", core.TInteger, func(v core.Value) bool {
		n, e := core.AsInteger(v)
		return e == nil && n%2 == 0
	})
	if err != nil {
		t.Fatalf("DefineMemberType: %v", err)
	}
	if !membershipBeyondNominal(member) {
		t.Error("member type read as nominal")
	}
	if membershipBeyondNominal(core.TInteger) {
		t.Error("Integer read as membership-based")
	}
	if membershipBeyondNominal(nil) {
		t.Error("nil read as membership-based")
	}
}

func TestVariadicCarrierHelpers(t *testing.T) {
	elem := core.NewTypeLiteral(core.TInteger)
	vc := core.NewVariadicCarrier(elem)
	got, ok := core.IsVariadicSpread(vc)
	if !ok || !got.Equal(core.TInteger) {
		t.Errorf("core.IsVariadicSpread = %v, %v", got, ok)
	}
	if _, ok := core.IsVariadicSpread(core.NewInteger(1)); ok {
		t.Error("integer read as variadic")
	}
	if !stackHasVariadic([]core.Value{core.NewInteger(1), vc}) {
		t.Error("stackHasVariadic missed the spread")
	}
	if stackHasVariadic([]core.Value{core.NewInteger(1)}) {
		t.Error("stackHasVariadic false positive")
	}

	// FoldVariadicArms: one variadic arm plus a leaked concrete slot →
	// one spread joining both element types.
	folded, ok := core.FoldVariadicArms(
		[]core.Value{core.NewCarrier(core.TInteger), vc},
		[]core.Value{core.NewNone()},
	)
	if !ok {
		t.Fatal("core.FoldVariadicArms declined a variadic arm")
	}
	fe, ok := core.IsVariadicSpread(folded)
	if !ok {
		t.Fatalf("folded arm not variadic: %v", folded)
	}
	if !strings.Contains(fe.String(), "Integer") {
		t.Errorf("folded element = %v", fe)
	}
	// No variadic anywhere: declines.
	if _, ok := core.FoldVariadicArms([]core.Value{core.NewCarrier(core.TInteger)}, nil); ok {
		t.Error("core.FoldVariadicArms accepted plain arms")
	}
}

func TestTypedListCarrierHelpers(t *testing.T) {
	tl := core.NewCarrierTypedListValue(core.NewCarrier(core.TString))
	if !tl.Carrier || !core.IsTypedList(tl) {
		t.Errorf("typed-list carrier shape: %v", tl)
	}
	if elem := DataListElemTypeFromValue(tl); !elem.Equal(core.TString) {
		t.Errorf("elem of typed list = %v", elem)
	}
	// Bare type-literal child: the child IS the element type.
	btl := core.NewTypedList(core.NewTypeLiteral(core.TInteger))
	if elem := DataListElemTypeFromValue(btl); !elem.Equal(core.TInteger) {
		t.Errorf("elem of bare typed list = %v", elem)
	}
	if elem := DataListElemTypeFromValue(core.NewTypeLiteral(core.TList)); !elem.Equal(core.TAny) {
		t.Errorf("elem of bare list literal = %v", elem)
	}

	pres := ReturnsPreserveListAt(0)
	out := pres([]core.Value{core.NewCarrierTypedList(core.TInteger)}, nil)
	if len(out) != 1 || !core.IsTypedList(out[0]) {
		t.Fatalf("ReturnsPreserveListAt = %v", out)
	}
	if elem := DataListElemTypeFromValue(out[0]); !elem.Equal(core.TInteger) {
		t.Errorf("preserved elem = %v", elem)
	}
	// Out-of-range index degrades to a plain List carrier.
	out = ReturnsPreserveListAt(3)([]core.Value{core.NewCarrier(core.TList)}, nil)
	if len(out) != 1 || !out[0].Parent.Equal(core.TList) {
		t.Errorf("out-of-range preserve = %v", out)
	}

	elemFn := ReturnsListElemAt(0)
	out = elemFn([]core.Value{core.NewCarrierTypedList(core.TString)}, nil)
	if len(out) != 1 || !out[0].Parent.Equal(core.TString) {
		t.Errorf("ReturnsListElemAt = %v", out)
	}
	out = ReturnsListElemAt(-1)(nil, nil)
	if len(out) != 1 || !out[0].Parent.Equal(core.TAny) {
		t.Errorf("out-of-range elem = %v", out)
	}
}

func TestElementAndParamCarriers(t *testing.T) {
	// Unknown element type → dynamic carrier.
	ec := NewElementCarrier(core.TAny)
	if !ec.Dynamic {
		t.Error("Any element carrier not dynamic")
	}
	if NewElementCarrier(core.TInteger).Dynamic {
		t.Error("typed element carrier dynamic")
	}

	// Mixed concrete list joins element types.
	mixed := ElementCarrierFromValue(core.NewList([]core.Value{core.NewInteger(1), core.NewString("s")}))
	if !mixed.Carrier {
		t.Errorf("mixed element carrier: %v", mixed)
	}
	// Single-typed list keeps the plain path.
	single := ElementCarrierFromValue(core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)}))
	if !single.Parent.Equal(core.TInteger) {
		t.Errorf("single-typed element = %v", single)
	}
	// A map's values join too.
	mv := ElementCarrierFromValue(mapOf("a", core.NewInteger(1), "b", core.NewString("s")))
	if !mv.Carrier {
		t.Errorf("map value carrier: %v", mv)
	}

	// ParamInputCarrier: Any → dynamic; concrete stays strict.
	if !ParamInputCarrier(core.TAny).Dynamic {
		t.Error("Any param carrier not dynamic")
	}
	if !ParamInputCarrier(nil).Dynamic {
		t.Error("nil param carrier not dynamic")
	}
	pc := ParamInputCarrier(core.TInteger)
	if pc.Dynamic || !pc.Parent.Equal(core.TInteger) {
		t.Errorf("Integer param carrier = %v", pc)
	}
}

func TestW9EvalFixedWindowToken(t *testing.T) {
	// Non-inert modes → false (carrier / dynamic / undefined).
	if evalFixedWindowToken(core.NewCarrier(core.TInteger)) {
		t.Error("carrier is not evaluation-fixed")
	}
	if evalFixedWindowToken(core.NewDynamicCarrier(core.TInteger)) {
		t.Error("dynamic carrier is not evaluation-fixed")
	}
	und := core.NewInteger(1)
	und.Undefined = true
	if evalFixedWindowToken(und) {
		t.Error("undefined value is not evaluation-fixed")
	}

	// A list of all-inert elements → true; a list with a word → false.
	if !evalFixedWindowToken(core.NewList([]core.Value{core.NewInteger(1), core.NewString("x")})) {
		t.Error("a list of inert scalars is evaluation-fixed")
	}
	if evalFixedWindowToken(core.NewList([]core.Value{core.NewWord("w")})) {
		t.Error("a list containing a word is not evaluation-fixed")
	}

	// A nil-backed map payload → false.
	if evalFixedWindowToken(core.NewMap(nil)) {
		t.Error("a nil-backed map is not evaluation-fixed")
	}
	// A map carrying computed keys (Meta "ck") → false.
	ck := core.NewOrderedMap()
	ck.Set("k", core.NewInteger(1))
	ck.Meta = map[string]any{"ck": map[string]bool{"k": true}}
	if evalFixedWindowToken(core.NewMap(ck)) {
		t.Error("a map with computed keys is not evaluation-fixed")
	}
	// A map with a non-inert value → false.
	bad := core.NewOrderedMap()
	bad.Set("k", core.NewWord("w"))
	if evalFixedWindowToken(core.NewMap(bad)) {
		t.Error("a map with a word value is not evaluation-fixed")
	}
	// A fully inert map → true.
	good := core.NewOrderedMap()
	good.Set("k", core.NewInteger(1))
	if !evalFixedWindowToken(core.NewMap(good)) {
		t.Error("an inert map is evaluation-fixed")
	}
}

func TestW9ShapedMethodApplyWindowRejects(t *testing.T) {
	r := newTestRegistry(t)
	e := &core.Engine{Registry: r}
	e.Tape = core.NewTape([]core.Value{core.NewCarrier(core.TAny), core.NewInteger(1)}, core.StackHeadroom)
	// Member payload is not an FnDefInfo → decline.
	if _, _, ok := shapedMethodApplyWindow(e, 0, core.NewInteger(1)); ok {
		t.Error("a non-FnDef member must decline")
	}
	// FnDefInfo whose owning registry does not resolve the name → decline.
	member := core.NewFunction(core.FnDefInfo{Registry: r, Name: "w9missing"})
	if _, _, ok := shapedMethodApplyWindow(e, 0, member); ok {
		t.Error("an unresolvable member fn must decline")
	}
}

func TestW9TryRecordMethodApplyWordMismatch(t *testing.T) {
	r := newTestRegistry(t)
	r.Check.PendingMethodApply = &core.PendingMethodApply{Origin: core.NewCarrier(core.TAny), Word: "owner"}
	if TryRecordMethodApply(r, "other", nil, nil, core.SrcPos{}) {
		t.Error("a word mismatch must leave the pending for its owner")
	}
	if r.Check.PendingMethodApply == nil {
		t.Error("the pending must survive a word mismatch")
	}
}

func TestW9TryDynamicFnValueDispatchNonInertWindow(t *testing.T) {
	r := newTestRegistry(t)
	e := &core.Engine{Registry: r}
	// A dynamic Function-bearing carrier followed by a WORD (non-inert): the
	// window admission declines the model.
	e.Tape = core.NewTape([]core.Value{core.NewDynamicCarrier(core.TFunction), core.NewWord("w")}, core.StackHeadroom)
	if tryDynamicFnValueDispatch(e, 0) {
		t.Error("a word in the forward window must decline the dynamic-fn model")
	}
}

func TestW9ScalarFoldOperand(t *testing.T) {
	// A plain concrete integer is inert-const → the fast path.
	if !ScalarFoldOperand(core.NewInteger(1)) {
		t.Error("a concrete integer is a scalar fold operand")
	}
	// A carrier whose payload is a value-scalar reaches the carrier-tolerant
	// switch arm (not inert-const, not dynamic, scalar payload).
	scalarCarrier := core.Value{Parent: core.TInteger, Data: core.IntPayload{N: 1}, Carrier: true}
	if !ScalarFoldOperand(scalarCarrier) {
		t.Error("a scalar-payload carrier is a fold operand")
	}
	// A compound (ChildTypeInfo) carrier declines.
	if ScalarFoldOperand(core.NewCarrier(core.TInteger)) {
		t.Error("a compound carrier is not a scalar fold operand")
	}
}

func TestW9JoinCarriersInnerOverCap(t *testing.T) {
	// More than core.CarrierDisjunctCap distinct alternatives collapse to a single
	// common-ancestor carrier.
	first := []core.Value{
		core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString), core.NewTypeLiteral(core.TBoolean),
		core.NewTypeLiteral(core.TAtom), core.NewTypeLiteral(core.TList),
	}
	second := []core.Value{
		core.NewTypeLiteral(core.TMap), core.NewTypeLiteral(core.TWord), core.NewTypeLiteral(core.TFunction),
		core.NewTypeLiteral(core.TError), core.NewTypeLiteral(core.TType),
	}
	out := core.JoinCarriersInner(core.NewDisjunct(first), core.NewDisjunct(second))
	if !out.Carrier {
		t.Errorf("an over-cap join should collapse to a carrier, got %v", out)
	}
	if core.IsDisjunct(out) {
		t.Error("an over-cap join must not remain a disjunct")
	}
}

func TestS7StaticListLenNilList(t *testing.T) {
	// A concrete but nil list reports no static length.
	if _, ok := StaticListLen(core.NewList(nil)); ok {
		t.Error("a nil list must not report a static length")
	}
}

func TestS7MinMaxIndexErrorArms(t *testing.T) {
	// DepScalar whose bound value is a non-integer -> no min/max.
	strLo := core.NewDepScalar(core.DepGT, core.NewString("z"))
	if _, ok := minIndex(strLo); ok {
		t.Error("string-bounded lower DepScalar has no integer minimum")
	}
	strHi := core.NewDepScalar(core.DepLT, core.NewString("z"))
	if _, ok := maxIndex(strHi); ok {
		t.Error("string-bounded upper DepScalar has no integer maximum")
	}
	// Concrete Integer-tagged value with a non-int payload -> no min/max.
	badInt := core.Value{Parent: core.TInteger, Data: core.StrPayload{S: "x"}}
	if _, ok := minIndex(badInt); ok {
		t.Error("unreadable integer index has no minimum")
	}
	if _, ok := maxIndex(badInt); ok {
		t.Error("unreadable integer index has no maximum")
	}
}

// toCarrier preserves the two step-time marker families whole: an __SP
// splice and a Word/__SG sugar marker both lose their reason to exist
// if the check-mode strip nulls their payload.
func TestToCarrierKeepsStepMarkers(t *testing.T) {
	sp := core.NewSplice(core.NewList([]core.Value{core.NewInteger(1)}))
	if got := toCarrier(sp); !core.IsSplice(got) {
		t.Errorf("splice must survive the carrier strip, got %v", got)
	} else if _, serr := core.AsSplice(got); serr != nil {
		t.Errorf("splice payload must survive the carrier strip: %v", serr)
	}
	sg := core.NewSugar(core.SugarInfo{Kind: core.SugarLambda})
	if got := toCarrier(sg); !core.IsSugar(got) {
		t.Errorf("sugar marker must survive the carrier strip, got %v", got)
	} else if _, ok := core.AsSugar(got); !ok {
		t.Errorf("sugar marker payload must survive the carrier strip")
	}
}

// typedContainerCarrier (D2 Part A) threads a {:T} param's element type into
// the body carrier. Its guard arms are exercised through the compile path by
// the langspec census; the arms that ordinary dispatch cannot reach (a typed
// pattern whose arg is not the matching container family — matchSignature
// rejects that before genArgs) are pinned by direct call.
func TestTypedContainerCarrier(t *testing.T) {
	mapPat := core.NewTypedMap(core.NewTypeLiteral(core.TInteger))
	listPat := core.NewCarrierTypedListValue(core.NewTypeLiteral(core.TInteger))

	// Typed-map pattern + a conforming Map arg → a typed carrier.
	if v, ok := typedContainerCarrier(core.FnParam{Pattern: &mapPat}, core.NewCarrier(core.TMap)); !ok || !core.IsTypedMap(v) {
		t.Errorf("map pattern + Map arg: got (%v, %v), want a typed-map carrier", v.Parent, ok)
	}
	// Typed-list pattern + a conforming List arg → a typed carrier.
	if v, ok := typedContainerCarrier(core.FnParam{Pattern: &listPat}, core.NewCarrier(core.TList)); !ok || !core.IsTypedList(v) {
		t.Errorf("list pattern + List arg: got (%v, %v), want a typed-list carrier", v.Parent, ok)
	}
	// Nil pattern → no carrier.
	if _, ok := typedContainerCarrier(core.FnParam{}, core.NewCarrier(core.TMap)); ok {
		t.Errorf("nil pattern should not produce a carrier")
	}
	// {:Any} pattern (child Any) → falls through, no tag.
	anyPat := core.NewTypedMap(core.NewTypeLiteral(core.TAny))
	if _, ok := typedContainerCarrier(core.FnParam{Pattern: &anyPat}, core.NewCarrier(core.TMap)); ok {
		t.Errorf("{:Any} pattern should fall through")
	}
	// Typed-map pattern + a NON-container arg (Integer) → the final
	// fallthrough: neither the map nor the list branch admits it.
	if _, ok := typedContainerCarrier(core.FnParam{Pattern: &mapPat}, core.NewInteger(5)); ok {
		t.Errorf("map pattern + Integer arg should fall through")
	}
	// Typed-list pattern + a Map arg (wrong family) → fallthrough too.
	if _, ok := typedContainerCarrier(core.FnParam{Pattern: &listPat}, core.NewCarrier(core.TMap)); ok {
		t.Errorf("list pattern + Map arg should fall through")
	}
}

func newTestRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func parenBody(tokens ...core.Value) []core.Value {
	out := []core.Value{core.NewOpenParen()}
	out = append(out, tokens...)
	out = append(out, core.NewCloseParen())
	return out
}

func mapOf(pairs ...any) core.Value {
	m := core.NewOrderedMap()
	for i := 0; i < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(core.Value))
	}
	return core.NewMap(m)
}

// s5beAt stamps a source position on a value.
func s5beAt(v core.Value, row, col int) core.Value {
	v.SetPos(core.SrcPos{Row: row, Col: col})
	return v
}

func TestS7BuildXmlAttrDynamic(t *testing.T) {
	r := xmlReg(t)
	e := core.NewTop(r)
	tmpl := core.XmlTmpl{Tag: "p", Attr: []core.XmlAttrTmpl{{
		Name: "x", Parts: []core.InterpPart{{Expr: []core.Value{core.NewWord("dynv")}}},
	}}}
	_, dynamic, _, _, err := e.BuildXmlFromTmpl(tmpl)
	if err != nil {
		t.Fatalf("attr dynamic: %v", err)
	}
	if !dynamic {
		t.Error("a carrier-valued attr part should mark the build dynamic")
	}
}

func TestS7BuildXmlNestedChildDynamic(t *testing.T) {
	r := xmlReg(t)
	e := core.NewTop(r)
	child := &core.XmlTmpl{Tag: "span", Attr: []core.XmlAttrTmpl{{
		Name: "x", Parts: []core.InterpPart{{Expr: []core.Value{core.NewWord("dynv")}}},
	}}}
	tmpl := core.XmlTmpl{Tag: "p", Cren: []core.XmlCren{{Kind: core.XmlCrenChild, Child: child}}}
	_, dynamic, _, _, err := e.BuildXmlFromTmpl(tmpl)
	if err != nil {
		t.Fatalf("nested child dynamic: %v", err)
	}
	if !dynamic {
		t.Error("a dynamic nested child should mark the parent build dynamic")
	}
}

// zzAnonDef builds an anonymous FnDefInfo with one signature.
func zzAnonDef(params []core.FnParam, returns []*core.Type, body []core.Value) core.FnDefInfo {
	return core.FnDefInfo{
		Anonymous: true,
		Signatures: []core.Signature{{
			Params:     params,
			Returns:    returns,
			Impl:       core.Boru(body),
			BarrierPos: core.BarrierAllForward,
		}},
	}
}

func TestW8AnonFnCheckModeUnnamedParamsStackActiveSlot(t *testing.T) {
	// An unnamed-param anonymous fn with stack args in check mode →
	// spliceAnonCheckResult(nArgs) via the else (unnamed) branch.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := zzAnonDef(
		[]core.FnParam{{Type: core.TInteger}, {Type: core.TInteger}},
		[]*core.Type{core.TInteger},
		parenBody(core.NewWord("cadd")),
	)
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(2), core.NewInteger(3), core.NewFunction(fnDef)}, core.StackHeadroom)
	e.Pointer = 2
	if err := e.ExecFnDefSigStackMatch(2, fnDef, []core.Value{core.NewInteger(2), core.NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}
