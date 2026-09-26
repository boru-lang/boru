package core

// Standalone-suite coverage for the carrier lattice that came down from
// the check piece at the 2026-08-08 ADR-013 amendment: the constructors
// (carrier_new.go), the spread carrier (carrier_spread.go), the join
// family (carrier_join.go), and dead-overload detection (deadsig.go).
//
// These paths were previously proved only through check/go's suite or
// end-to-end through lang; core/go carries a 100% standalone gate, so
// they are proved here against core's own vocabulary.

import (
	"math"
	"testing"
)

// --- carrier_new.go --------------------------------------------------

func TestCarrierConstructors(t *testing.T) {
	// NewDynamicCarrierValue promotes an EXISTING value, keeping its
	// Parent/Data bound rather than replacing it with a bare carrier.
	bound := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	dyn := NewDynamicCarrierValue(bound)
	if !dyn.Carrier || !dyn.Dynamic {
		t.Error("NewDynamicCarrierValue must set both Carrier and Dynamic")
	}
	if !IsDisjunct(dyn) {
		t.Error("the disjunct bound must survive the promotion")
	}

	// NewCarrierTypedList: element type known, container still abstract.
	tl := NewCarrierTypedList(TInteger)
	if !tl.Carrier || !tl.Parent.Equal(TList) {
		t.Errorf("NewCarrierTypedList = %v, want an abstract List", tl)
	}
	ct, ok := tl.Data.(ChildTypeInfo)
	if !ok || !ct.Child.Parent.Equal(TInteger) {
		t.Errorf("element carrier = %+v, want Integer", tl.Data)
	}

	// NewCarrierTypedListValue takes an arbitrary child — here a nested
	// typed list, the shape a bare Parent cannot express.
	nested := NewCarrierTypedListValue(NewCarrierTypedList(TString))
	if !nested.Carrier || !nested.Parent.Equal(TList) {
		t.Errorf("NewCarrierTypedListValue = %v, want an abstract List", nested)
	}
	inner, ok := nested.Data.(ChildTypeInfo)
	if !ok || !inner.Child.Parent.Equal(TList) {
		t.Errorf("nested element = %+v, want a List carrier", nested.Data)
	}
}

func TestUnionCarrierForType(t *testing.T) {
	// A nil type, and a type with no disjunctUnifier Behavior, both decline.
	if _, ok := UnionCarrierForType(nil); ok {
		t.Error("nil type must decline")
	}
	if _, ok := UnionCarrierForType(TInteger); ok {
		t.Error("a plain scalar type must decline")
	}

	// A union type — a node whose Behavior is a DisjunctUnifier — yields
	// the DISTRIBUTING carrier: a strict Disjunct of its alternatives.
	u := MintTestType("Ideal/UnionCarrierProbe")
	alts := []Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)}
	u.SetBehavior(&DisjunctUnifier{Alternatives: alts})
	dv, ok := UnionCarrierForType(u)
	if !ok {
		t.Fatal("a union type must yield its distributing carrier")
	}
	if !dv.Carrier || !IsDisjunct(dv) {
		t.Fatalf("union carrier = %v, want a strict disjunct carrier", dv)
	}
	di, err := AsDisjunct(dv)
	if err != nil || len(di.Alternatives) != 2 {
		t.Errorf("alternatives = %+v (%v), want the union's two members", di, err)
	}

	// An EMPTY alternative list is not a union to distribute over.
	e := MintTestType("Ideal/UnionCarrierEmpty")
	e.SetBehavior(&DisjunctUnifier{})
	if _, ok := UnionCarrierForType(e); ok {
		t.Error("a DisjunctUnifier with no alternatives must decline")
	}
}

func TestReturnsIdentity(t *testing.T) {
	a, b := NewInteger(1), NewString("x")

	// A plain permutation hands back the same Values, identity intact —
	// the narrowing/provenance property the compiler's operand layout
	// depends on.
	swap := ReturnsIdentity(1, 0)
	out := swap([]Value{a, b}, nil)
	if len(out) != 2 || !ValuesEqual(out[0], b) || !ValuesEqual(out[1], a) {
		t.Fatalf("swap = %v, want [x 1]", out)
	}
	if out[1].ID != a.ID {
		t.Error("an unrepeated source must keep its identity")
	}

	// A DUPLICATED source index must mint a fresh ID per copy, or the
	// emitter resolves both operands of `dup add` onto one output.
	dup := ReturnsIdentity(0, 0)
	out = dup([]Value{a}, nil)
	if len(out) != 2 {
		t.Fatalf("dup arity = %d, want 2", len(out))
	}
	if out[0].ID == out[1].ID {
		t.Error("repeated sources must get distinct identities")
	}
	if out[0].ID == a.ID && out[1].ID == a.ID {
		t.Error("neither copy may keep the source's own provenance")
	}

	// An out-of-range mapping (negative, or past the args) degrades to an
	// Any carrier rather than panicking.
	oob := ReturnsIdentity(-1, 5)
	out = oob([]Value{a}, nil)
	if len(out) != 2 {
		t.Fatalf("oob arity = %d, want 2", len(out))
	}
	for i, v := range out {
		if !v.Carrier || !v.Parent.Equal(TAny) {
			t.Errorf("out-of-range slot %d = %v, want an Any carrier", i, v)
		}
	}
}

// --- carrier_spread.go -----------------------------------------------

func TestVariadicSpreadCarrier(t *testing.T) {
	elem := NewTypeLiteral(TInteger)
	sp := NewVariadicCarrier(elem)
	if !sp.Carrier || !sp.Dynamic || !sp.Parent.Equal(TAny) {
		t.Errorf("variadic carrier = %v, want a dynamic Any carrier", sp)
	}
	got, ok := IsVariadicSpread(sp)
	if !ok || !ValuesEqual(got, elem) {
		t.Errorf("IsVariadicSpread = %v/%v, want the element back", got, ok)
	}
	if _, ok := IsVariadicSpread(NewInteger(1)); ok {
		t.Error("an ordinary value is not a variadic spread")
	}
}

func TestFoldVariadicArms(t *testing.T) {
	// Neither arm carries a spread: decline, so the ordinary if-join applies.
	if _, ok := FoldVariadicArms([]Value{NewCarrier(TInteger)}, []Value{NewCarrier(TString)}); ok {
		t.Error("no variadic arm: FoldVariadicArms must decline")
	}

	// One variadic arm plus a fixed lead in the other: the fold joins the
	// per-frame LEAKED types — the spread's own element and the concrete
	// slot — dropping None padding.
	seed := NewVariadicCarrier(NewTypeLiteral(TNever))
	then := []Value{seed, NewCarrier(TNone)}
	els := []Value{NewCarrier(TInteger)}
	out, ok := FoldVariadicArms(then, els)
	if !ok {
		t.Fatal("a variadic arm must fold")
	}
	elem, isSpread := IsVariadicSpread(out)
	if !isSpread {
		t.Fatalf("fold result = %v, want a variadic spread", out)
	}
	// Never is the seed and is dropped; Integer is the only real leak.
	if !ValueType(elem).Equal(TInteger) {
		t.Errorf("folded element = %v, want Integer", ValueType(elem))
	}

	// Two arms whose leaks differ produce a union element.
	out, ok = FoldVariadicArms(
		[]Value{NewVariadicCarrier(NewTypeLiteral(TString))},
		[]Value{NewCarrier(TInteger)})
	if !ok {
		t.Fatal("a variadic arm must fold")
	}
	elem, _ = IsVariadicSpread(out)
	if !IsDisjunct(elem) {
		t.Errorf("mixed leaks = %v, want a disjunct element", elem)
	}

	// All-Never / all-padding leaves the Never seed in place.
	out, ok = FoldVariadicArms([]Value{seed}, []Value{{}})
	if !ok {
		t.Fatal("a variadic arm must fold")
	}
	elem, _ = IsVariadicSpread(out)
	if !ValueType(elem).Equal(TNever) {
		t.Errorf("empty fold element = %v, want Never", ValueType(elem))
	}
}

// --- carrier_join.go -------------------------------------------------

func TestCommonAncestorType(t *testing.T) {
	if got := CommonAncestorType(nil, TInteger); !got.Equal(TAny) {
		t.Errorf("nil left = %v, want Any", got)
	}
	if got := CommonAncestorType(TInteger, nil); !got.Equal(TAny) {
		t.Errorf("nil right = %v, want Any", got)
	}
	// Two value-tagged Integer literals share Integer.
	a := NewInteger(42).Parent
	b := NewInteger(99).Parent
	if got := CommonAncestorType(a, b); !got.Equal(TInteger) {
		t.Errorf("Integer/42 vs Integer/99 = %v, want Integer", got)
	}
	// Integer and Float share Number.
	if got := CommonAncestorType(TInteger, TFloat); !got.Equal(TNumber) {
		t.Errorf("Integer vs Float = %v, want Number", got)
	}
	// A type with no lattice parent shares nothing.
	if got := CommonAncestorType(TNone, TInteger); !got.Equal(TAny) {
		t.Errorf("None vs Integer = %v, want Any", got)
	}
}

func TestFlattenAlternatives(t *testing.T) {
	// A bare type literal IS its own node — returned as-is.
	lit := NewTypeLiteral(TInteger)
	got := FlattenAlternatives(lit)
	if len(got) != 1 || !ValueType(got[0]).Equal(TInteger) {
		t.Errorf("literal flatten = %v, want [Integer]", got)
	}
	// A non-literal carrier contributes a literal of its Parent.
	got = FlattenAlternatives(NewCarrier(TString))
	if len(got) != 1 || !ValueType(got[0]).Equal(TString) {
		t.Errorf("carrier flatten = %v, want [String]", got)
	}
	// A disjunct recurses into its alternatives.
	dj := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	got = FlattenAlternatives(dj)
	if len(got) != 2 {
		t.Fatalf("disjunct flatten = %v, want two alternatives", got)
	}
}

func TestJoinCarriersDynamicContagion(t *testing.T) {
	strict := JoinCarriers(NewCarrier(TInteger), NewCarrier(TInteger))
	if strict.Dynamic {
		t.Error("two strict arms must join strict")
	}
	// EITHER arm dynamic makes the merge dynamic — the gradual contagion
	// rule (looser, never tighter).
	left := JoinCarriers(NewDynamicCarrier(TInteger), NewCarrier(TString))
	if !left.Dynamic || !left.Carrier {
		t.Error("a dynamic left arm must make the merge dynamic")
	}
	right := JoinCarriers(NewCarrier(TString), NewDynamicCarrier(TInteger))
	if !right.Dynamic {
		t.Error("a dynamic right arm must make the merge dynamic")
	}
}

func TestJoinCarriersInner(t *testing.T) {
	none := NewTypeLiteral(TNone)

	// Two None arms join to a None carrier — the parent-math shortcuts are
	// unsafe against None's nil Parent, so this arm is explicit.
	if got := JoinCarriersInner(none, none); !got.Carrier || !got.Parent.Equal(TNone) {
		t.Errorf("None+None = %v, want a None carrier", got)
	}

	// Same Parent, neither a disjunct: collapse, but MINT A FRESH ID so the
	// merged value cannot collide with an arm's live local.
	a := NewCarrier(TInteger)
	b := NewCarrier(TInteger)
	got := JoinCarriersInner(a, b)
	if !got.Parent.Equal(TInteger) || got.Data != nil {
		t.Errorf("Integer+Integer = %v, want a bare Integer carrier", got)
	}
	if got.ID == a.ID || got.ID == b.ID {
		t.Error("the merged carrier must mint a fresh identity")
	}

	// Subtype on either side widens to the supertype.
	if got := JoinCarriersInner(NewCarrier(TInteger), NewCarrier(TNumber)); !got.Parent.Equal(TNumber) {
		t.Errorf("Integer+Number = %v, want Number", got)
	}
	if got := JoinCarriersInner(NewCarrier(TNumber), NewCarrier(TInteger)); !got.Parent.Equal(TNumber) {
		t.Errorf("Number+Integer = %v, want Number", got)
	}

	// DIRECT siblings collapse to their shared parent.
	if got := JoinCarriersInner(NewCarrier(TInteger), NewCarrier(TFloat)); !got.Parent.Equal(TNumber) {
		t.Errorf("Integer+Float = %v, want Number", got)
	}

	// DISTANT cousins must NOT collapse — widening would change
	// first-match dispatch, so they stay a disjunct with the real
	// alternatives.
	got = JoinCarriersInner(NewCarrier(TInteger), NewCarrier(TString))
	if !IsDisjunct(got) || !got.Carrier {
		t.Errorf("Integer+String = %v, want a disjunct carrier", got)
	}

	// A mixed None/value pair falls through to the alternatives union.
	got = JoinCarriersInner(none, NewCarrier(TInteger))
	if !IsDisjunct(got) {
		t.Errorf("None+Integer = %v, want a disjunct", got)
	}

	// Alternatives that simplify to ONE become a carrier of that literal.
	got = JoinCarriersInner(
		NewDisjunct([]Value{NewTypeLiteral(TInteger)}),
		NewDisjunct([]Value{NewTypeLiteral(TInteger)}))
	if IsDisjunct(got) {
		t.Errorf("a one-alternative union = %v, want a plain carrier", got)
	}

	// Past the width cap the union widens to the common ancestor of all
	// alternatives rather than carrying an unbounded disjunct.
	var wide []Value
	for i := 0; i < CarrierDisjunctCap+2; i++ {
		wide = append(wide, NewInteger(int64(i)))
	}
	got = JoinCarriersInner(NewDisjunct(wide), NewCarrier(TInteger))
	if IsDisjunct(got) {
		t.Errorf("over-cap union = %v, want the widened ancestor", got)
	}
	if !got.Parent.ConformsTo(TInteger) {
		t.Errorf("widened ancestor = %v, want Integer or above", got.Parent)
	}
}

func TestIsNoneArm(t *testing.T) {
	if !isNoneArm(NewCarrier(TNone)) {
		t.Error("a None-bound carrier is a None arm")
	}
	if isNoneArm(NewCarrier(TInteger)) {
		t.Error("an Integer carrier is not a None arm")
	}
	// A Parent-less value falls through to the none-SHAPE test.
	if isNoneArm(Value{}) {
		t.Error("the zero Value is not a None arm")
	}
	if !isNoneArm(NewNone()) {
		t.Error("a none value is a None arm")
	}
}

func TestJoinCarrierStacks(t *testing.T) {
	// Equal length: per-position join.
	joined := JoinCarrierStacks(
		[]Value{NewCarrier(TInteger), NewCarrier(TInteger)},
		[]Value{NewCarrier(TInteger), NewCarrier(TFloat)})
	if len(joined) != 2 {
		t.Fatalf("joined depth = %d, want 2", len(joined))
	}
	if !joined[0].Parent.Equal(TInteger) || !joined[1].Parent.Equal(TNumber) {
		t.Errorf("joined = %v, want [Integer Number]", joined)
	}

	// The SHORTER stack pads with None carriers — a padded None keeps the
	// merge strict (a variadic 0-or-1 arm), so no gradual flip here.
	padded := JoinCarrierStacks([]Value{NewCarrier(TInteger), NewCarrier(TInteger)}, nil)
	if len(padded) != 2 {
		t.Fatalf("padded depth = %d, want 2", len(padded))
	}
	for i, v := range padded {
		if v.Dynamic {
			t.Errorf("padded slot %d went gradual; padding must stay strict", i)
		}
	}
	// Padding on the other side takes the mirror branch.
	padded = JoinCarrierStacks(nil, []Value{NewCarrier(TString)})
	if len(padded) != 1 || padded[0].Dynamic {
		t.Errorf("mirror padding = %v, want one strict slot", padded)
	}

	// BOTH arms real and exactly one is None: the optional / sentinel
	// pattern, which merges GRADUAL so a downstream consumer of the real
	// type still matches.
	sentinel := JoinCarrierStacks(
		[]Value{NewCarrier(TNone)},
		[]Value{NewCarrier(TMap)})
	if len(sentinel) != 1 || !sentinel[0].Dynamic {
		t.Errorf("None-vs-value merge = %v, want a gradual result", sentinel)
	}
	// Neither None: stays strict.
	strict := JoinCarrierStacks([]Value{NewCarrier(TInteger)}, []Value{NewCarrier(TString)})
	if strict[0].Dynamic {
		t.Error("a value-vs-value merge must stay strict")
	}
	// BOTH None: also strict (the != test is false).
	bothNone := JoinCarrierStacks([]Value{NewCarrier(TNone)}, []Value{NewCarrier(TNone)})
	if bothNone[0].Dynamic {
		t.Error("a None-vs-None merge must stay strict")
	}
}

func TestJoinBranchDefAndInstallJoinedDefs(t *testing.T) {
	// joinBranchDef: SAME identity on both sides means the arms only
	// NARROWED an enclosing binding, so the merged value keeps that ID —
	// a fresh one would strand every later reference.
	a := NewCarrier(TInteger)
	b := a
	if got := joinBranchDef(a, b); got.ID != a.ID {
		t.Errorf("narrow-only merge ID = %q, want the preserved %q", got.ID, a.ID)
	}
	// DIFFERING identities are a genuine reassignment: fresh ID.
	c := NewCarrier(TInteger)
	if got := joinBranchDef(a, c); got.ID == a.ID || got.ID == c.ID {
		t.Error("a genuine rebind must mint a fresh identity")
	}

	r := w8reg(t)

	// Both branches bound the name: push the join of the two arms.
	InstallJoinedDefs(r,
		map[string]Value{"both": NewCarrier(TInteger)},
		map[string]Value{"both": NewCarrier(TFloat)})
	if v, ok := r.Defs.Top("both"); !ok || !v.Parent.Equal(TNumber) {
		t.Errorf("both-branch join = %v/%v, want a Number carrier", v, ok)
	}

	// then-only with NO pre-branch binding: the def is pushed as-is.
	InstallJoinedDefs(r, map[string]Value{"thenFresh": NewCarrier(TString)}, nil)
	if v, ok := r.Defs.Top("thenFresh"); !ok || !v.Parent.Equal(TString) {
		t.Errorf("then-only fresh = %v/%v, want a String carrier", v, ok)
	}

	// then-only WITH a pre-branch binding: joined with it, because the
	// else path kept the original.
	r.Defs.Push("thenPre", NewCarrier(TFloat))
	InstallJoinedDefs(r, map[string]Value{"thenPre": NewCarrier(TInteger)}, nil)
	if v, ok := r.Defs.Top("thenPre"); !ok || !v.Parent.Equal(TNumber) {
		t.Errorf("then-only over pre = %v/%v, want a Number carrier", v, ok)
	}

	// else-only, both with and without a pre-branch binding.
	InstallJoinedDefs(r, nil, map[string]Value{"elseFresh": NewCarrier(TString)})
	if v, ok := r.Defs.Top("elseFresh"); !ok || !v.Parent.Equal(TString) {
		t.Errorf("else-only fresh = %v/%v, want a String carrier", v, ok)
	}
	r.Defs.Push("elsePre", NewCarrier(TFloat))
	InstallJoinedDefs(r, nil, map[string]Value{"elsePre": NewCarrier(TInteger)})
	if v, ok := r.Defs.Top("elsePre"); !ok || !v.Parent.Equal(TNumber) {
		t.Errorf("else-only over pre = %v/%v, want a Number carrier", v, ok)
	}

	// A name in BOTH maps is handled once — the else loop skips what the
	// then loop already saw.
	InstallJoinedDefs(r,
		map[string]Value{"seen": NewCarrier(TInteger)},
		map[string]Value{"seen": NewCarrier(TInteger)})
	if got := r.Defs.Depth("seen"); got != 1 {
		t.Errorf("a both-branch name was pushed %d times, want 1", got)
	}
}

// --- deadsig.go ------------------------------------------------------

func TestDeadSignatures(t *testing.T) {
	sig := func(args ...*Type) Signature {
		return Signature{Args: args, Returns: []*Type{TAny}, BarrierPos: -1}
	}
	// Fewer than two signatures can have no shadowing.
	if got := DeadSignatures(nil); got != nil {
		t.Errorf("nil sigs = %v, want nil", got)
	}
	if got := DeadSignatures([]Signature{sig(TInteger)}); got != nil {
		t.Errorf("single sig = %v, want nil", got)
	}

	// The canonical dead pair is a DUPLICATE overload: SortSignatures is
	// most-specific-first, so a genuinely more general sig always sorts
	// AFTER the specific one it would subsume — which is exactly why
	// `[Number]` before `[Integer]` is NOT reportable (checked below).
	// Two identical sigs keep their registration order, and the second
	// can never win first-match dispatch.
	dead := DeadSignatures([]Signature{sig(TInteger), sig(TInteger)})
	if len(dead) != 1 {
		t.Fatalf("dead = %+v, want exactly one unreachable overload", dead)
	}
	if !SigArgType(&dead[0].Sig, 0).Equal(TInteger) ||
		!SigArgType(&dead[0].ShadowedBy, 0).Equal(TInteger) {
		t.Errorf("dead pair = %+v, want the duplicate Integer overloads", dead[0])
	}
	if dead[0].Index == 0 {
		t.Error("a dead sig is never first in dispatch order")
	}

	// A supertype overload registered FIRST is still live: the sort puts
	// [Integer] ahead of it, so [Number] keeps every non-Integer call.
	if got := DeadSignatures([]Signature{sig(TNumber), sig(TInteger)}); got != nil {
		t.Errorf("Number/Integer = %+v, want nil — the sort keeps both live", got)
	}

	// Disjoint slot types shadow nothing.
	if got := DeadSignatures([]Signature{sig(TInteger), sig(TString)}); got != nil {
		t.Errorf("disjoint sigs = %+v, want nil", got)
	}

	// A Fallback catch-all is intentional and never reported, on either
	// side of the pairing.
	fb := sig()
	fb.Fallback = true
	if got := DeadSignatures([]Signature{fb, sig(TInteger), sig(TInteger)}); len(got) != 1 {
		t.Errorf("fallback handling = %+v, want just the duplicate overload", got)
	}
}

func TestSigSubsumesDeclines(t *testing.T) {
	sig := func(args ...*Type) Signature {
		return Signature{Args: args, Returns: []*Type{TAny}, BarrierPos: -1}
	}
	// Differing arity.
	a, b := sig(TNumber), sig(TNumber, TNumber)
	if sigSubsumes(&a, &b) {
		t.Error("different arities compete for different call shapes")
	}
	// Differing forward/stack boundary.
	c, d := sig(TNumber), sig(TInteger)
	d.BarrierPos = 0
	if sigSubsumes(&c, &d) {
		t.Error("a differing BarrierPos must decline")
	}
	// Differing TypeArgs kind — a type-literal slot and a value slot
	// admit disjoint things.
	e, f := sig(TNumber), sig(TInteger)
	f.TypeArgs = map[int]bool{0: true}
	if sigSubsumes(&e, &f) {
		t.Error("a differing TypeArgs kind must decline")
	}
	// Differing /q capture.
	g, h := sig(TNumber), sig(TInteger)
	h.QuoteArgs = map[int]bool{0: true}
	if sigSubsumes(&g, &h) {
		t.Error("a differing QuoteArgs kind must decline")
	}
	// A structural PATTERN on either side is value-dependent, so the
	// relation conservatively declines rather than risk a false "dead".
	i, j := sig(TNumber), sig(TInteger)
	i.Patterns = map[int]Value{0: NewInteger(1)}
	if sigSubsumes(&i, &j) {
		t.Error("a pattern on the shadower must decline")
	}
	k, l := sig(TNumber), sig(TInteger)
	l.Patterns = map[int]Value{0: NewInteger(1)}
	if sigSubsumes(&k, &l) {
		t.Error("a pattern on the shadowed must decline")
	}
	// A nil declared type is unusable for the containment test.
	m, n := sig(nil), sig(TInteger)
	if sigSubsumes(&m, &n) {
		t.Error("a nil slot type must decline")
	}
	// The positive case, for contrast.
	o, p := sig(TNumber), sig(TInteger)
	if !sigSubsumes(&o, &p) {
		t.Error("[Number] must subsume [Integer]")
	}
}

// --- check_state.go: the deduping emitters ---------------------------

func TestCheckAddUniqueDedupe(t *testing.T) {
	r := w8reg(t)
	r.Check.Mode = true

	pos := SrcPos{Row: 3, Col: 7}
	CheckAddUniqueDiagnostic(r, "probe", "detail", "w", pos)
	if n := len(r.Check.Diagnostics); n != 1 {
		t.Fatalf("first emit recorded %d diagnostics, want 1", n)
	}
	if !r.Check.Diagnostics[0].RuntimeMirror {
		t.Error("CheckAddUniqueDiagnostic must stamp RuntimeMirror")
	}
	// An identical finding at the same site is dropped — ReturnsFns run
	// once per analysed call shape.
	CheckAddUniqueDiagnostic(r, "probe", "detail", "w", pos)
	if n := len(r.Check.Diagnostics); n != 1 {
		t.Errorf("duplicate emit recorded %d diagnostics, want 1", n)
	}
	// A different position is a different finding.
	CheckAddUniqueDiagnostic(r, "probe", "detail", "w", SrcPos{Row: 9, Col: 1})
	if n := len(r.Check.Diagnostics); n != 2 {
		t.Errorf("distinct site recorded %d diagnostics, want 2", n)
	}

	// A CAUGHT (downgraded) entry never blocks a later REAL emission of
	// the same finding, so the dedupe skips it.
	r2 := w8reg(t)
	r2.Check.Mode = true
	r2.Check.AddDiagnostic(CheckDiagnostic{
		Code: "probe", Detail: "detail", Row: 3, Col: 7, CaughtAtRuntime: true,
	})
	CheckAddUnique(r2, CheckDiagnostic{Code: "probe", Detail: "detail", Row: 3, Col: 7})
	if n := len(r2.Check.Diagnostics); n != 2 {
		t.Errorf("caught entry blocked a real emission (%d diagnostics, want 2)", n)
	}
}

// guard against an unused-import trip if the math cases above are edited.
var _ = math.MaxInt64

// TestJoinCarriersInnerOverCap drives the width-cap arm: past
// CarrierDisjunctCap the union is widened to the common ancestor of all
// alternatives rather than carrying an unbounded disjunct. Needs
// alternatives that SURVIVE SimplifyDisjunctAlts, so they are distinct
// lattice nodes rather than same-family values.
func TestJoinCarriersInnerOverCap(t *testing.T) {
	nodes := []*Type{TInteger, TString, TBoolean, TAtom, TMap, TList, TWord, TFloat, TError}
	if len(nodes) <= CarrierDisjunctCap {
		t.Fatalf("fixture must exceed the cap of %d, has %d", CarrierDisjunctCap, len(nodes))
	}
	alts := make([]Value, len(nodes))
	for i, n := range nodes {
		alts[i] = NewTypeLiteral(n)
	}
	got := JoinCarriersInner(NewDisjunct(alts), NewCarrier(TInteger))
	if IsDisjunct(got) {
		t.Fatalf("over-cap union = %v, want the widened ancestor", got)
	}
	if !got.Carrier {
		t.Errorf("widened result = %v, want a carrier", got)
	}
}

// TestDeadSignaturesSkipsFallbackShadower pins the INNER fallback guard.
// The usual case never reaches it — a fallback is 0-arg, so the
// arity-first sort sinks it past every other overload. But when EVERY
// signature is 0-arg the arity tie leaves a fallback able to sort ahead
// of a plain sig, and sigSubsumes is vacuously true at arity 0, so
// without the guard the fallback would be reported as shadowing.
func TestDeadSignaturesSkipsFallbackShadower(t *testing.T) {
	zero := func(fallback bool) Signature {
		return Signature{Returns: []*Type{TAny}, BarrierPos: -1, Fallback: fallback}
	}
	dead := DeadSignatures([]Signature{zero(true), zero(false), zero(false)})
	for _, d := range dead {
		if d.ShadowedBy.Fallback {
			t.Errorf("a fallback must never be reported as the shadower: %+v", d)
		}
	}
	// The two plain 0-arg sigs still shadow each other, so the pass is
	// not vacuous.
	if len(dead) == 0 {
		t.Error("two identical plain overloads must still report one dead")
	}
}

// The narrowing-only skip (Codex P2, PR #418): an arm add whose ID equals the
// pre-branch binding's — narrowDynamicUses preserves the value's ID — is the
// pass's own refinement of the SAME runtime value and must be neither pushed
// nor noted, at every arm shape. A both-arms GENUINE join, conversely, must
// now be NOTED (its push previously carried no ledger entry at all).
func TestInstallJoinedDefsNarrowingSkipAndBothArmsNote(t *testing.T) {
	r := w8reg(t)
	r.Check.Mode = true

	pre := NewDynamicCarrier(TAny)
	r.Defs.Push("njq", pre)
	narrowed := NewDynamicCarrierValue(NewCarrier(TList))
	narrowed.ID = pre.ID

	base := len(r.Check.BindLedger)
	// then-only, else-only, and both-arms narrowing: no push, no note.
	InstallJoinedDefs(r, map[string]Value{"njq": narrowed}, nil)
	InstallJoinedDefs(r, nil, map[string]Value{"njq": narrowed})
	InstallJoinedDefs(r, map[string]Value{"njq": narrowed}, map[string]Value{"njq": narrowed})
	if d := r.Defs.Depth("njq"); d != 1 {
		t.Fatalf("a narrowing-only add must not be re-pushed at the join: depth = %d, want 1", d)
	}
	if len(r.Check.BindLedger) != base {
		t.Fatalf("a narrowing-only add must not be ledgered: %+v", r.Check.BindLedger[base:])
	}

	// A genuine both-arms binding pushes AND notes.
	InstallJoinedDefs(r,
		map[string]Value{"bjq": NewCarrier(TInteger)},
		map[string]Value{"bjq": NewCarrier(TFloat)})
	if d := r.Defs.Depth("bjq"); d != 1 {
		t.Fatalf("a genuine both-arms join must push: depth = %d", d)
	}
	led := r.Check.BindLedger[base:]
	if len(led) != 1 || led[0].Kind != BindDef || led[0].Name != "bjq" || led[0].Depth != 1 {
		t.Fatalf("the both-arms join must note its push (this note was missing); got %+v", led)
	}
}

// armedJoinRegistry is a registry whose check state answers Armed() — the
// compile-pass configuration condBoundCarrier keys on — restored on cleanup.
func armedJoinRegistry(t *testing.T) *Registry {
	t.Helper()
	r := w8reg(t)
	prev := r.Check.Emit
	r.Check.Emit = &s5aEmit{EmitRecorder: TheInactiveEmit, active: true, armed: true}
	t.Cleanup(func() { r.Check.Emit = prev })
	return r
}

// TestCondBoundCarrierUnderArmedRecorder pins the binding InstallJoinedDefs
// pushes for a name bound in ONE arm of an undecided branch with no
// pre-branch binding under a COMPILE pass: a payload-less carrier of the
// arm's value with a FRESH identity, so a later read resolves through the
// compiler's frame slot (the branch-carried def) rather than baking the
// arm's value. A capitalised name, a fn value and a Function-typed value
// keep the arm's own value — their consumers read the payload itself.
func TestCondBoundCarrierUnderArmedRecorder(t *testing.T) {
	r := armedJoinRegistry(t)

	then := NewInteger(5)
	InstallJoinedDefs(r, map[string]Value{"cbx": then}, nil)
	got, ok := r.Defs.Top("cbx")
	if !ok || IsConcrete(got) || !got.Carrier || !got.Parent.Equal(TInteger) {
		t.Fatalf("a then-only fresh binding under an armed recorder is a payload-less Integer carrier: %v/%v", got, ok)
	}
	if got.ID == then.ID {
		t.Error("the carrier takes a FRESH identity, not the arm's value's")
	}

	els := NewString("s")
	InstallJoinedDefs(r, nil, map[string]Value{"cby": els})
	if got, ok := r.Defs.Top("cby"); !ok || IsConcrete(got) || !got.Parent.ConformsTo(TString) || got.ID == els.ID {
		t.Errorf("an else-only fresh binding takes the same carrier: %v/%v", got, ok)
	}

	capName := NewInteger(7)
	fnv := NewFunction(FnDefInfo{Name: "cbg"})
	fnCarrier := NewCarrier(TFunction)
	InstallJoinedDefs(r, map[string]Value{"Cbz": capName, "cbf": fnv, "cbc": fnCarrier}, nil)
	for name, want := range map[string]Value{"Cbz": capName, "cbf": fnv, "cbc": fnCarrier} {
		if got, ok := r.Defs.Top(name); !ok || got.ID != want.ID {
			t.Errorf("%s: a capitalised name, a fn value and a Function-typed value keep the arm's own value: %v/%v", name, got, ok)
		}
	}

	// Off the compile pass the arm's own value is kept exactly as before.
	plain := w8reg(t)
	InstallJoinedDefs(plain, map[string]Value{"cbx": then}, nil)
	if got, ok := plain.Defs.Top("cbx"); !ok || got.ID != then.ID {
		t.Errorf("a plain check keeps the arm's own value: %v/%v", got, ok)
	}
}

// TestInstallTakenArmDefs pins the CONSTANT-condition twin: the one analysed
// arm always runs, so its binding is unconditionally the post-branch one —
// the arm's own value even under an armed recorder — and every join it
// reports is flagged Taken.
func TestInstallTakenArmDefs(t *testing.T) {
	r := armedJoinRegistry(t)
	tv := NewInteger(1)
	joins := InstallTakenArmDefs(r, map[string]Value{"tka": tv}, nil)
	if len(joins) != 1 || !joins[0].Taken || !joins[0].ThenBinds || joins[0].ElseBinds || joins[0].Name != "tka" {
		t.Fatalf("a then-arm taken binding reports one Taken then-join: %+v", joins)
	}
	if got, ok := r.Defs.Top("tka"); !ok || got.ID != tv.ID {
		t.Errorf("the taken arm's binding is the arm's own value, not a carrier: %v/%v", got, ok)
	}
	ev := NewString("e")
	joins = InstallTakenArmDefs(r, nil, map[string]Value{"tkb": ev})
	if len(joins) != 1 || !joins[0].Taken || joins[0].ThenBinds || !joins[0].ElseBinds {
		t.Fatalf("an else-arm taken binding reports one Taken else-join: %+v", joins)
	}
	if got, ok := r.Defs.Top("tkb"); !ok || got.ID != ev.ID {
		t.Errorf("the taken else arm's binding is the arm's own value: %v/%v", got, ok)
	}
}
