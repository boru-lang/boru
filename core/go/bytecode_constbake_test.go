package core

import "testing"

// The const pool is the bytecode compiler's single sharpest soundness edge:
// isInertConst (emit.go) decides what may live in Program.Consts, and a pooled
// const is pushed by the SAME pointer-backed Value on every execution —
// including each iteration of a loop. So the whitelist MUST NEVER admit a
// MUTABLE instance type (Array / Object / Store): if one slipped in, an
// in-place mutator (`set` on an Array/Object/Store receiver) would corrupt the
// baked const across iterations with no type error. The invariant is documented
// exhaustively at isInertConst's MUTATION-SAFETY note; this test PINS it so a
// well-meaning "just add this type to the switch" change fails loudly here
// rather than silently in a loop.
//
// Paired positive/negative per the repo's test discipline: the immutable shapes
// the whitelist is FOR stay bakeable, the mutable instances it must exclude do
// not.

func TestIsInertConstRejectsMutableInstances(t *testing.T) {
	// NEGATIVE — mutable instance types must never bake as consts.
	mutable := []struct {
		name string
		v    Value
	}{
		{"store", NewStore(TStore)},
		{"class-instance", NewClassInstance(TClass, ClassInstanceInfo{})},
	}
	for _, m := range mutable {
		if IsInertConst(m.v) {
			t.Errorf("isInertConst(%s) = true; a mutable instance type must NOT be const-bakeable "+
				"(a pooled const is pushed by the same pointer every iteration — an in-place "+
				"mutator would corrupt it). See isInertConst's MUTATION-SAFETY note before "+
				"changing the whitelist.", m.name)
		}
		// And as a MEMBER of a const compound — a mutable instance nested in a
		// list/map must keep the whole compound off the const path.
		if IsInertConstMember(m.v) {
			t.Errorf("isInertConstMember(%s) = true; a mutable instance must not ride inside a const compound", m.name)
		}
		if IsInertConst(NewList([]Value{m.v})) {
			t.Errorf("isInertConst([%s]) = true; a list holding a mutable instance must not bake", m.name)
		}
	}

	// POSITIVE — the immutable scalars/compounds the whitelist exists for stay
	// bakeable, so the negative assertions above are not vacuously passing on a
	// blanket "everything is false".
	inert := []struct {
		name string
		v    Value
	}{
		{"integer", NewInteger(7)},
		{"float", NewFloat(1.5)},
		{"string", NewString("x")},
		{"boolean", NewBoolean(true)},
		{"int-list", NewList([]Value{NewInteger(1), NewInteger(2)})},
	}
	for _, c := range inert {
		if !IsInertConst(c.v) {
			t.Errorf("isInertConst(%s) = false; an immutable scalar/compound must stay const-bakeable", c.name)
		}
	}
}

// An immutable Node/Xml literal (`<a x="1"><b/>hi</a>`) is a constant value at
// parse time (parser emits NewXmlElement; only a ${}-interpolated literal
// becomes the deferred Word/__XI runtime builder). It is value-semantics with
// structural sharing — never mutated in place — so it joins the const whitelist
// like the List / Map cases, gated on its attribute values and child nodes being
// inert members. The MUTABLE twin, FlexXml (*FlexXmlData), must stay OFF the
// const pool: a baked FlexXml shared across loop iterations would be corrupted by
// an in-place `set` / `append`, exactly the hazard the mutable-instance test
// guards. Paired positive/negative pins both edges.
func TestIsInertConstXmlLiteral(t *testing.T) {
	attr := NewOrderedMap()
	attr.Set("x", NewString("1"))
	// <a x="1"><b/>hi</a> — attribute value, a nested immutable element, and a
	// text child, all inert members.
	xml := NewXmlElement("a", attr, []Value{
		NewXmlElement("b", nil, nil),
		NewString("hi"),
	})

	// POSITIVE — an immutable element with inert interior bakes, standalone and
	// as a member of a const compound.
	if !IsInertConst(xml) {
		t.Errorf(`isInertConst(<a x="1"><b/>hi</a>) = false; an immutable Node/Xml literal with an inert interior must be const-bakeable`)
	}
	if !IsInertConst(NewList([]Value{xml})) {
		t.Error("isInertConst([<a/>…]) = false; a list of immutable XML literals must bake")
	}

	// NEGATIVE — the MUTABLE FlexXml twin must never bake, standalone or nested.
	flex := NewFlexXml("a", nil, nil)
	if IsInertConst(flex) {
		t.Error("isInertConst(flex <a/>) = true; the MUTABLE FlexXml must NOT const-bake " +
			"(an in-place set/append would corrupt a pooled const across iterations)")
	}
	if IsInertConstMember(flex) {
		t.Error("isInertConstMember(flex <a/>) = true; a mutable FlexXml must not ride inside a const compound")
	}
	if IsInertConst(NewList([]Value{flex})) {
		t.Error("isInertConst([flex <a/>]) = true; a list holding a mutable FlexXml must not bake")
	}
	// A mutable FlexXml CHILD must keep its containing immutable element off the
	// const path too (the interior member check must catch it).
	if IsInertConst(NewXmlElement("p", nil, []Value{flex})) {
		t.Error("isInertConst(<p>{flex}</p>) = true; an immutable element wrapping a mutable FlexXml child must not bake")
	}
}

// A Table type body is a thin wrapper over its row RecordType (TableTypeInfo
// embeds RecordTypeInfo), so it joins the structural-type-body family
// (Record/Options/Object/Disjunct) the const whitelist already admits: a
// module-exported Table type (`Test.TestSet`) folds to a const so the get over
// the immutable module export bakes rather than declining "unknown provenance".
// Soundness rides the SAME typeBodyConstOK interior check the record uses — a
// carrier/dynamic field in the row schema must still keep the whole type off the
// const path. Paired positive/negative pins both edges.
func TestIsInertConstTableTypeBody(t *testing.T) {
	dataFields := NewOrderedMap()
	dataFields.Set("name", NewTypeLiteral(TString)) // bare type nodes — data schema
	dataFields.Set("value", NewTypeLiteral(TInteger))
	dataTable := NewTableType(RecordTypeInfo{Fields: dataFields})

	// POSITIVE — a Table over a data-only record schema bakes as a const.
	if !IsInertConst(dataTable) {
		t.Errorf("isInertConst(Table{name:String value:Integer}) = false; a Table type over a "+
			"data-only record must be const-bakeable (it is a thin wrapper over RecordTypeInfo, "+
			"which already bakes). Disassemble: %v", dataTable)
	}
	if !typeBodyConstOK(dataTable) {
		t.Error("typeBodyConstOK(data Table) = false; want true")
	}

	// NEGATIVE — a carrier in the row schema (a check-mode artefact that must
	// never be materialised) keeps the Table type off the const path, exactly
	// as it would the bare record.
	carrierFields := NewOrderedMap()
	carrierFields.Set("rows", NewCarrier(TList))
	carrierTable := NewTableType(RecordTypeInfo{Fields: carrierFields})
	if IsInertConst(carrierTable) {
		t.Error("isInertConst(Table over carrier-bearing record) = true; a carrier interior must " +
			"keep the structural type body off the const pool")
	}
	if typeBodyConstOK(carrierTable) {
		t.Error("typeBodyConstOK(carrier Table) = true; want false")
	}
}

// A dot-access reach (`r.int`) inside a NEVER-evaluated code body (a NoEvalArgs
// operand the driving word stores / drops / CallBorus — Test.prop / Test.skip /
// Test.check-prop — or a quoted list) is pure DATA: the VM pushes the baked
// compound verbatim and never expands a reach (in-place expansion is an
// interpreter behaviour). So a receiver reach rides as a const MEMBER even
// though the STANDALONE isInertReach (the detached lens) requires receiverless +
// Eval=false. A COMPUTED segment (a paren to run) is code and still declines.
func TestInertReachMember(t *testing.T) {
	// `r.int` — receiver [Word(r)], one literal-key segment, Eval=true (the
	// dot-access form the parser builds for a member of a code body).
	dotReach := NewReach(ReachInfo{
		Receiver: []Value{NewWord("r")},
		Segments: []ReachSeg{{KeyLit: NewWord("int")}},
		Eval:     true,
	})

	// POSITIVE — a dot-access reach is an inert const MEMBER, so a code body
	// holding it (`[r.int 0 9]`) bakes as a const list.
	if !inertReachMember(dotReach) {
		t.Error("inertReachMember(r.int) = false; a dot-access reach must ride as a const member")
	}
	body := NewList([]Value{dotReach, NewInteger(0), NewInteger(9)})
	if !IsInertConst(body) {
		t.Error("isInertConst([r.int 0 9]) = false; a code body of inert tokens + a reach must bake")
	}
	// …but the SAME reach standalone is NOT an inert const — at the engine
	// pointer it expands in place, so it must reach the runtime as code, never
	// frozen into the const pool.
	if IsInertConst(dotReach) {
		t.Error("isInertConst(r.int) = true; a standalone dot-access reach must stay OUT of the const pool")
	}

	// NEGATIVE — a COMPUTED segment (`r.(k)`, a paren evaluated at run time) is
	// code, not data, so the reach declines as a member and keeps its body off
	// the const path.
	computedReach := NewReach(ReachInfo{
		Receiver: []Value{NewWord("p")},
		Segments: []ReachSeg{{Computed: true, KeyExpr: []Value{NewWord("k")}}},
		Eval:     true,
	})
	if inertReachMember(computedReach) {
		t.Error("inertReachMember(p.(k)) = true; a computed segment is code and must decline")
	}
	if IsInertConst(NewList([]Value{computedReach})) {
		t.Error("isInertConst([p.(k)]) = true; a reach with a computed segment must keep the body off the const path")
	}
}
