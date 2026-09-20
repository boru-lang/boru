package check

// The final mop-up: branches the corpus and the family-focused test
// files cannot reach — value-shape arms of the const-fold carrier scan,
// the def-read tagging's dynamic and flex arms, the make mirror's
// typed-map decline, and CheckAtIndices' unknown-length silence.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestValueCarriesCarrierShapes(t *testing.T) {
	// A carrier is a carrier wherever it hides.
	if !valueCarriesCarrier(core.NewCarrier(core.TInteger)) {
		t.Error("a bare carrier carries")
	}
	if !valueCarriesCarrier(core.NewList([]core.Value{core.NewInteger(1), core.NewCarrier(core.TInteger)})) {
		t.Error("a carrier inside a list carries")
	}
	if valueCarriesCarrier(core.NewList([]core.Value{core.NewInteger(1)})) {
		t.Error("a concrete list of concretes does not carry")
	}
	if valueCarriesCarrier(core.NewInteger(1)) {
		t.Error("a concrete scalar does not carry")
	}
}

func TestExprRefsCarrierShapes(t *testing.T) {
	r := newTestRegistry(t)
	e := core.NewTop(r)
	done := r.Check.Begin()
	defer done()

	// A word bound to a carrier on the def stack refs a carrier.
	r.Defs.Push("cw", core.NewCarrier(core.TInteger))
	if !exprRefsCarrier(e, []core.Value{core.NewWord("cw")}) {
		t.Error("a def-bound carrier word refs a carrier")
	}
	// A word bound to a concrete does not.
	r.Defs.Push("kw", core.NewInteger(4))
	if exprRefsCarrier(e, []core.Value{core.NewWord("kw")}) {
		t.Error("a def-bound concrete word does not ref a carrier")
	}
	// The scan recurses into nested lists.
	if !exprRefsCarrier(e, []core.Value{core.NewList([]core.Value{core.NewWord("cw")})}) {
		t.Error("a carrier word nested in a list refs a carrier")
	}
	// An unbound word and a plain literal do not.
	if exprRefsCarrier(e, []core.Value{core.NewWord("nope"), core.NewInteger(1)}) {
		t.Error("unbound words and literals do not ref carriers")
	}
}

func TestTagCheckModeDefRead(t *testing.T) {
	r := newTestRegistry(t)
	e := core.NewTop(r)
	done := r.Check.Begin()
	defer done()

	// A dynamic bound value tags the def it came from, so a later
	// diagnostic can name its origin.
	dyn := core.NewDynamicCarrier(core.TAny)
	tagCheckModeDefRead(e, &dyn, "srcdef", core.SrcPos{})
	if dyn.DynFrom() != "srcdef" {
		t.Errorf("a dynamic def read must tag its origin, got %q", dyn.DynFrom())
	}
	// A concrete read is left untagged.
	conc := core.NewInteger(3)
	tagCheckModeDefRead(e, &conc, "srcdef", core.SrcPos{})
	if conc.DynFrom() != "" {
		t.Error("a concrete def read must stay untagged")
	}
}

func TestCheckMakeConstructionTypedMapDeclines(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	fields := core.NewOrderedMap()
	fields.Set("n", core.NewTypeLiteral(core.TInteger))
	ct := r.Types.MintType("Class/TM", core.TClass)
	cls := core.NewClassType(ct, core.ClassTypeInfo{Fields: fields, Name: "TM", Type: ct})

	// A typed-map literal conforms to TMap but AsMap declines it — the
	// provided==nil arm. Value-dependent, so the mirror stays silent.
	tm := core.NewTypedMap(core.NewTypeLiteral(core.TInteger))
	CheckMakeConstruction(r, cls, tm, core.SrcPos{})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("typed-map source must decline silently, got %+v", r.Check.Diagnostics)
	}

	// A carrier field value is value-dependent and skipped, while a
	// concrete wrong-typed sibling still flags.
	src := mapOf("n", core.NewCarrier(core.TString))
	CheckMakeConstruction(r, cls, src, core.SrcPos{Row: 3, Col: 1})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("carrier field value must be skipped, got %+v", r.Check.Diagnostics)
	}
}

func TestCheckAtIndicesUnknownLength(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	idxs := core.NewList([]core.Value{core.NewInteger(99)})
	// Unknown data length: silent regardless of the index.
	CheckAtIndices(r, idxs, core.NewCarrier(core.TList), "atq")
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("unknown length must stay silent, got %+v", r.Check.Diagnostics)
	}
	// Non-concrete indices over a known length: silent too.
	data := core.NewList([]core.Value{core.NewInteger(1)})
	CheckAtIndices(r, core.NewCarrier(core.TList), data, "atq")
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("non-concrete indices must stay silent, got %+v", r.Check.Diagnostics)
	}
}

// zcmSig builds a two-slot signature (stack Integer + forward Any) for
// the strand-advisory scans.
func zcmSig(slot0 *core.Type) *core.Signature {
	return &core.Signature{Args: []*core.Type{slot0, core.TAny}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1}
}

func TestCheckForwardStrandsOperandArms(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	w := core.WordInfo{Name: "sw"}
	pos := core.SrcPos{Row: 1, Col: 1}

	mk := func(tokens []core.Value, ptr int) *core.Engine {
		e := core.NewTop(r)
		e.Tape = core.NewTape(tokens, core.StackHeadroom)
		e.Pointer = ptr
		return e
	}

	// An Any-typed consumed slot is too weak a signal: no advisory.
	e := mk([]core.Value{core.NewInteger(7), core.NewInteger(8)}, 2)
	checkForwardStrandsOperand(e, w, zcmSig(core.TAny), []int{1, 3}, pos)
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("Any slot must stay quiet, got %+v", r.Check.Diagnostics)
	}

	// A scope boundary below the consumed args ends the scan: no advisory.
	e = mk([]core.Value{core.NewOpenParen(), core.NewInteger(8)}, 2)
	checkForwardStrandsOperand(e, w, zcmSig(core.TInteger), []int{1, 3}, pos)
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("scope boundary must stay quiet, got %+v", r.Check.Diagnostics)
	}

	// Structural residue is scanned past; a same-typed operand below it
	// is the stranded sibling and flags.
	e = mk([]core.Value{core.NewInteger(6), core.NewCarrier(core.TInteger), core.NewInteger(8)}, 3)
	checkForwardStrandsOperand(e, w, zcmSig(core.TInteger), []int{2, 4}, pos)
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("stranded sibling under residue must flag once, got %+v", r.Check.Diagnostics)
	}
}

func TestExprRefsCarrierParenAndReach(t *testing.T) {
	r := newTestRegistry(t)
	e := core.NewTop(r)
	done := r.Check.Begin()
	defer done()
	r.Defs.Push("pcw", core.NewCarrier(core.TInteger))

	// A paren expression's tokens are walked.
	pe := core.NewParenExpr([]core.Value{core.NewWord("pcw")})
	if !exprRefsCarrier(e, []core.Value{pe}) {
		t.Error("a carrier word inside a paren expr refs a carrier")
	}
	// A Reach hides carriers in its receiver and computed keys.
	reach := core.NewReach(core.ReachInfo{
		Receiver: []core.Value{core.NewWord("pcw")},
	})
	if !exprRefsCarrier(e, []core.Value{reach}) {
		t.Error("a carrier in a Reach receiver refs a carrier")
	}
	reachKey := core.NewReach(core.ReachInfo{
		Receiver: []core.Value{core.NewInteger(1)},
		Segments: []core.ReachSeg{{Computed: true, KeyExpr: []core.Value{core.NewWord("pcw")}}},
	})
	if !exprRefsCarrier(e, []core.Value{reachKey}) {
		t.Error("a carrier in a computed Reach key refs a carrier")
	}
	// A hit latches across recursion: a carrier found inside a NESTED
	// list returns from the inner walk, and the outer loop's next element
	// sees the latch and stops instead of rescanning.
	nested := core.NewList([]core.Value{core.NewWord("pcw")})
	if !exprRefsCarrier(e, []core.Value{nested, core.NewInteger(2)}) {
		t.Error("the latched walk still reports the carrier")
	}
	reachClean := core.NewReach(core.ReachInfo{
		Receiver: []core.Value{core.NewInteger(1)},
		Segments: []core.ReachSeg{{Computed: false}},
	})
	if exprRefsCarrier(e, []core.Value{reachClean}) {
		t.Error("a carrier-free Reach does not ref a carrier")
	}
}

func TestTagCheckModeDefReadFlexArm(t *testing.T) {
	r := newTestRegistry(t)
	e := core.NewTop(r)
	done := r.Check.Begin()
	defer done()

	// A module-scope flex-map binding read back is tagged dynamic-from:
	// the container's runtime state is not check-visible.
	m := core.NewOrderedMap()
	m.Set("k", core.NewInteger(1))
	flex := core.NewValueRaw(core.TFlexMap, core.MapPayload{M: m})
	if !core.IsFlexMap(flex) {
		t.Fatal("construction must satisfy IsFlexMap")
	}
	tagCheckModeDefRead(e, &flex, "fm", core.SrcPos{})
	if flex.DynFrom() != "fm" {
		t.Errorf("a module-scope flex read must tag its def, got %q", flex.DynFrom())
	}
}

func TestAdoptShapeValueXml(t *testing.T) {
	x := core.NewXmlElement("div", core.NewOrderedMap(), nil)
	got := AdoptShapeValue(x, 0)
	if !got.Carrier || !got.Parent.ConformsTo(core.TFlexXml) {
		t.Errorf("a concrete xml adopts to a FlexXml carrier, got %s", got.Parent.Leaf())
	}
}

func TestAllConcreteArgsBareTypeNode(t *testing.T) {
	// A bare type node is evaluation-fixed — the checked literal IS the
	// runtime operand — so it does not block the dry pass.
	if !allConcreteArgs([]core.Value{core.NewTypeLiteral(core.TInteger), core.NewInteger(3)}) {
		t.Error("a type-literal arg must not block the dry pass")
	}
	// A carrier still does.
	if allConcreteArgs([]core.Value{core.NewTypeLiteral(core.TInteger), core.NewCarrier(core.TInteger)}) {
		t.Error("a carrier arg must still block the dry pass")
	}
	// A strict None CARRIER still passes (the singleton rule): a concrete
	// none takes the IsConcrete arm, so the carrier shape is what
	// exercises the singleton exception.
	if !allConcreteArgs([]core.Value{core.NewCarrier(core.TNone)}) {
		t.Error("a strict None carrier must not block the dry pass")
	}
	dynNone := core.NewDynamicCarrier(core.TNone)
	if allConcreteArgs([]core.Value{dynNone}) {
		t.Error("a DYNAMIC none must still block the dry pass")
	}
}

func TestReturnsDynUnion(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	f := ReturnsDynUnion(core.TString, core.TNone)
	out := f(nil, r)
	if len(out) != 1 {
		t.Fatalf("want one result, got %d", len(out))
	}
	v := out[0]
	if !v.Dynamic {
		t.Error("a declared union result must stay DYNAMIC — a strict union declines gradual call sites")
	}
	if !core.IsDisjunct(v) {
		t.Errorf("the bound must be the alternative set, got %s", v.Parent.Leaf())
	}
	// Each invocation mints a FRESH identity. A shared bound identity
	// aliases every call site's modelled result — the def binder then
	// collapses separate effectful evaluations into one slot, a proven
	// miscompile (TestReadLineStdinAdvances, lang/go/test).
	w := f(nil, r)[0]
	if v.ID != "" && v.ID == w.ID {
		t.Error("two invocations must not share a value ID")
	}
	if w.ID == "" {
		t.Error("inside a check pass the modelled result must carry a minted ID")
	}
}
