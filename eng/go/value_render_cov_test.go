package eng

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/cockroachdb/apd/v3"
)

// Rendering and accessor coverage for value.go and print.go: every
// constructor round-trips through its accessor, every accessor rejects
// a foreign value, and the format paths never panic.

func TestKernelRenderTable(t *testing.T) {
	m := core.NewOrderedMap()
	m.Set("k", core.NewInteger(1))
	cases := []struct {
		name string
		v    core.Value
		want string // substring of v.String()
	}{
		{"word", core.NewWord("foo"), "word(foo)"},
		{"open-paren", core.NewOpenParen(), "("},
		{"close-paren", core.NewCloseParen(), ")"},
		{"end", core.NewEnd(), "end"},
		{"mark", core.NewMark("m1"), "mark(m1)"},
		{"move", core.NewMove("m1", "loop"), "move(m1,loop)"},
		{"paren-expr", core.NewParenExpr([]core.Value{core.NewInteger(1)}), "paren"},
		{"string", core.NewString("hi"), "'hi'"},
		{"atom", core.NewAtom("sym"), "sym"},
		{"integer", core.NewInteger(-3), "-3"},
		{"float", core.NewFloat(2.5), "2.5"},
		{"bool-true", core.NewBoolean(true), "true"},
		{"bool-false", core.NewBoolean(false), "false"},
		{"none-value", core.NewNone(), "none"},
		{"none-literal", core.NewTypeLiteral(core.TNone), "None"},
		{"type-literal", core.NewTypeLiteral(core.TInteger), "Integer"},
		{"list", core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)}), "1"},
		{"map", core.NewMap(m), "k"},
		{"error", core.NewError(errors.New("boom")), "boom"},
		{"pathon", func() core.Value { out, _ := core.MakePathon(core.NewString("a/b"), true); return out[0] }(), "/a/b"},
		{"big-integer", core.NewBigInteger(big.NewInt(12345)), "12345"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.v.String()
			if !strings.Contains(got, c.want) {
				t.Errorf("String() = %q, want substring %q", got, c.want)
			}
		})
	}
}

func TestFormatForPrintTable(t *testing.T) {
	m := core.NewOrderedMap()
	m.Set("a", core.NewInteger(1))
	m.Set("s", core.NewString("x"))
	cases := []struct {
		name string
		v    core.Value
		want string
	}{
		{"none", core.NewNone(), "none"},
		{"none-type", core.NewTypeLiteral(core.TNone), "None"},
		{"type-literal", core.NewTypeLiteral(core.TString), "String"},
		{"string-unquoted", core.NewString("plain"), "plain"},
		{"integer", core.NewInteger(9), "9"},
		{"map-json", core.NewMap(m), `"a": 1`},
		{"list-json", core.NewList([]core.Value{core.NewInteger(1), core.NewString("z")}), `[1, "z"]`},
		{"error", core.NewError(errors.New("oops")), "oops"},
		{"nested", core.NewList([]core.Value{core.NewMap(m)}), `"s": "x"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := core.FormatForPrint(c.v)
			if !strings.Contains(got, c.want) {
				t.Errorf("FormatForPrint = %q, want substring %q", got, c.want)
			}
		})
	}
}

func TestFormatValueJSONQuotesStrings(t *testing.T) {
	if got := core.FormatValueJSON(core.NewString("s")); got != `"s"` {
		t.Errorf("string JSON = %q", got)
	}
	if got := core.FormatValueJSON(core.NewInteger(3)); got != "3" {
		t.Errorf("int JSON = %q", got)
	}
	if got := core.FormatValueJSON(core.NewBoolean(true)); got != "true" {
		t.Errorf("bool JSON = %q", got)
	}
	// Nested compounds recurse.
	inner := core.NewOrderedMap()
	inner.Set("k", core.NewList([]core.Value{core.NewInteger(1)}))
	got := core.FormatValueJSON(core.NewMap(inner))
	if !strings.Contains(got, `"k": [1]`) {
		t.Errorf("nested JSON = %q", got)
	}
}

func TestFormatTableAndPadRight(t *testing.T) {
	if got := core.PadRight("ab", 5); got != "ab   " {
		t.Errorf("PadRight = %q", got)
	}
	if got := core.PadRight("abcdef", 3); got != "abcdef" {
		t.Errorf("PadRight over-width = %q", got)
	}

	rec := intStrRecord()
	row := core.NewOrderedMap()
	row.Set("a", core.NewInteger(1))
	row.Set("b", core.NewString("x"))
	td := core.TableData{Record: rec, Rows: []core.Value{core.NewMap(row)}}
	tbl := core.NewValueRaw(core.TTable, td)
	got := core.FormatForPrint(tbl)
	if !strings.Contains(got, "a") || !strings.Contains(got, "x") {
		t.Errorf("table render = %q, want headers and cells", got)
	}
	// Empty table renders headers only, without panicking.
	empty := core.NewValueRaw(core.TTable, core.TableData{Record: rec})
	if got := core.FormatForPrint(empty); got == "" {
		t.Error("empty table rendered as empty string")
	}
}

func TestPrintHandlers(t *testing.T) {
	var buf strings.Builder
	r := newTestRegistry(t)
	r.Output = &buf
	if _, err := core.PrintHandler([]core.Value{core.NewString("hello")}, nil, nil, r); err != nil {
		t.Fatalf("PrintHandler: %v", err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("print output = %q", buf.String())
	}
	buf.Reset()
	if _, err := core.PrintstrHandler([]core.Value{core.NewString("raw")}, nil, nil, r); err != nil {
		t.Fatalf("PrintstrHandler: %v", err)
	}
	if !strings.Contains(buf.String(), "raw") {
		t.Errorf("printstr output = %q", buf.String())
	}
}

// --- accessor round-trips with negatives -----------------------------------

func TestBigNumberAccessors(t *testing.T) {
	bi := core.NewBigInteger(big.NewInt(99))
	n, err := core.AsBigInteger(bi)
	if err != nil || n.Int64() != 99 {
		t.Errorf("AsBigInteger = %v, %v", n, err)
	}
	if _, err := core.AsBigInteger(core.NewInteger(1)); err == nil {
		t.Error("AsBigInteger(1) should error")
	}

	d := apd.New(25, -1) // 2.5
	bd := core.NewBigDecimal(d)
	dv, err := core.AsBigDecimal(bd)
	if err != nil {
		t.Fatalf("AsBigDecimal: %v", err)
	}
	if dv.String() != d.String() {
		t.Errorf("AsBigDecimal = %v", dv)
	}
	if _, err := core.AsBigDecimal(core.NewFloat(2.5)); err == nil {
		t.Error("AsBigDecimal(2.5 float) should error")
	}

	if s := core.FormatBigInteger(big.NewInt(7)); !strings.Contains(s, "7") {
		t.Errorf("FormatBigInteger = %q", s)
	}
	if s := core.FormatBigDecimal(d); !strings.Contains(s, "2.5") {
		t.Errorf("FormatBigDecimal = %q", s)
	}

	// AsFloatApprox spans int, float, and the big leaves.
	for _, c := range []struct {
		v    core.Value
		want float64
	}{
		{core.NewInteger(2), 2},
		{core.NewFloat(1.5), 1.5},
		{bi, 99},
	} {
		f, err := core.AsFloatApprox(c.v)
		if err != nil {
			t.Errorf("AsFloatApprox(%v): %v", c.v, err)
			continue
		}
		if f != c.want {
			t.Errorf("AsFloatApprox(%v) = %v, want %v", c.v, f, c.want)
		}
	}
	if _, err := core.AsFloatApprox(core.NewString("x")); err == nil {
		t.Error("AsFloatApprox(string) should error")
	}
}

func TestMarkMoveAccessors(t *testing.T) {
	mk := core.NewMark("m9", core.NewInteger(1))
	info, err := core.AsMark(mk)
	if err != nil || info.ID != "m9" || len(info.Body) != 1 {
		t.Errorf("AsMark = %+v, %v", info, err)
	}
	if _, err := core.AsMark(core.NewInteger(1)); err == nil {
		t.Error("AsMark(1) should error")
	}

	mv := core.NewMove("m9", "test")
	mi, err := core.AsMove(mv)
	if err != nil || mi.To != "m9" || mi.Reason != "test" {
		t.Errorf("AsMove = %+v, %v", mi, err)
	}
	if _, err := core.AsMove(core.NewInteger(1)); err == nil {
		t.Error("AsMove(1) should error")
	}

	// Continuation-carrying moves keep their payloads.
	fc := &core.ForCont{}
	mvc := core.NewMoveCont("m9", "iter", fc)
	mic, _ := core.AsMove(mvc)
	if mic.Cont != fc {
		t.Error("NewMoveCont lost the ForCont")
	}
	ic := &core.IfCont{}
	mvi := core.NewMoveIf("m9", "if", ic)
	mii, _ := core.AsMove(mvi)
	if mii.IfCont != ic {
		t.Error("NewMoveIf lost the IfCont")
	}
}

func TestErrorValueAccessors(t *testing.T) {
	// Plain Go error: message only, no code.
	ev := core.NewError(errors.New("plain failure"))
	info, err := core.AsError(ev)
	if err != nil {
		t.Fatalf("AsError: %v", err)
	}
	if info.Message != "plain failure" || info.Code != "" {
		t.Errorf("plain error info = %+v", info)
	}
	// BoruError: code preserved for dispatch.
	ae := core.MakeBoruError("type_error", "typed failure", "w", "", "")
	av := core.NewError(ae)
	ainfo, _ := core.AsError(av)
	if ainfo.Code != "type_error" {
		t.Errorf("BoruError code lost: %+v", ainfo)
	}
	if !strings.Contains(ainfo.Message, "typed failure") {
		t.Errorf("BoruError message lost: %+v", ainfo)
	}
	// Negative.
	if _, err := core.AsError(core.NewInteger(1)); err == nil {
		t.Error("AsError(1) should error")
	}
	if !core.IsError(av) || core.IsError(core.NewInteger(1)) {
		t.Error("IsError misclassifies")
	}
}

func TestSpliceAndDispatchMod(t *testing.T) {
	sp := core.NewSplice(core.NewList([]core.Value{core.NewInteger(1)}))
	if !core.IsSplice(sp) {
		t.Error("NewSplice not recognised by IsSplice")
	}
	if core.IsSplice(core.NewInteger(1)) {
		t.Error("IsSplice(1) = true")
	}

	dm := core.NewDispatchMod(core.DispatchModInfo{Val: true})
	got, ok := core.AsDispatchMod(dm)
	if !ok || !got.Val || got.Quote {
		t.Errorf("AsDispatchMod = %+v, %v", got, ok)
	}
	if _, ok := core.AsDispatchMod(core.NewInteger(1)); ok {
		t.Error("AsDispatchMod(1) should decline")
	}
}

func TestTypedListAndMapConstructors(t *testing.T) {
	child := core.NewTypeLiteral(core.TInteger)

	tl := core.NewTypedList(child)
	if !core.IsTypedList(tl) {
		t.Error("NewTypedList not a typed list")
	}
	tle := core.NewTypedListWithElements(child, []core.Value{core.NewInteger(1), core.NewInteger(2)})
	ci, err := core.AsChildType(tle)
	if err != nil {
		t.Fatalf("AsChildType: %v", err)
	}
	if len(ci.Elements) != 2 {
		t.Errorf("typed list elements = %d, want 2", len(ci.Elements))
	}

	tm := core.NewTypedMap(child)
	if !core.IsTypedMap(tm) {
		t.Error("NewTypedMap not a typed map")
	}
	tme := core.NewTypedMapWithEntries(child, []core.ChildEntry{{Key: "k", Value: core.NewInteger(3)}})
	mi, err := core.AsChildType(tme)
	if err != nil {
		t.Fatalf("AsChildType map: %v", err)
	}
	if len(mi.Entries) != 1 || mi.Entries[0].Key != "k" {
		t.Errorf("typed map entries = %+v", mi.Entries)
	}

	// Negatives.
	if core.IsTypedList(core.NewList(nil)) {
		t.Error("plain list misread as typed list")
	}
	if core.IsTypedMap(core.NewMap(core.NewOrderedMap())) {
		t.Error("plain map misread as typed map")
	}
	if _, err := core.AsChildType(core.NewInteger(1)); err == nil {
		t.Error("AsChildType(1) should error")
	}
}

func TestEvalAndImplicitContainers(t *testing.T) {
	el := core.NewEvalList([]core.Value{core.NewInteger(1)})
	if !el.Eval {
		t.Error("NewEvalList did not set Eval")
	}
	m := core.NewOrderedMap()
	m.Set("a", core.NewInteger(1))
	em := core.NewEvalMap(m)
	if !em.Eval {
		t.Error("NewEvalMap did not set Eval")
	}
	im := core.NewImplicitMap(m)
	if !core.IsImplicitMap(im) {
		t.Error("NewImplicitMap not implicit")
	}
	if core.IsImplicitMap(core.NewMap(core.NewOrderedMap())) {
		t.Error("plain map misread as implicit")
	}
	fm := core.NewFlexMap(m)
	if !core.IsFlexMap(fm) {
		t.Error("NewFlexMap not recognised")
	}
	if core.IsFlexMap(core.NewMap(m)) {
		t.Error("plain map misread as flex map")
	}
}

func TestAtomReferent(t *testing.T) {
	a := core.NewAtom("name")
	if _, ok := core.AtomReferent(a); ok {
		t.Error("fresh atom should have no referent")
	}
	withRef := core.SetAtomReferent(a, core.NewInteger(42))
	ref, ok := core.AtomReferent(withRef)
	if !ok {
		t.Fatal("referent lost")
	}
	if got, _ := core.AsInteger(ref); got != 42 {
		t.Errorf("referent = %v", ref)
	}
	// Non-atoms have no referent.
	if _, ok := core.AtomReferent(core.NewInteger(1)); ok {
		t.Error("integer claims an atom referent")
	}
}

func TestTypeNameAndPathOf(t *testing.T) {
	if got := core.TypeNameOf(core.NewInteger(1)); got != "Integer" {
		t.Errorf("TypeNameOf(1) = %q", got)
	}
	if got := core.TypePathOf(core.NewInteger(1)); !strings.Contains(got, "Integer") {
		t.Errorf("TypePathOf(1) = %q", got)
	}
	// A concrete string's leaf is the ProperString node (String's
	// concrete child), not the String family root.
	if got := core.TypeNameOf(core.NewString("s")); got != "ProperString" {
		t.Errorf("TypeNameOf(\"s\") = %q, want ProperString leaf", got)
	}
}

func TestEnumAndFnUndef(t *testing.T) {
	e := core.NewEnum([]core.Value{core.NewAtom("red"), core.NewAtom("blue")})
	if !core.IsDisjunct(e) {
		t.Error("enum should be a disjunct value")
	}
	di, err := core.AsDisjunct(e)
	if err != nil || len(di.Alternatives) != 2 {
		t.Errorf("AsDisjunct(enum) = %+v, %v", di, err)
	}

	fu := core.NewFnUndef(core.FnUndefInfo{Sigs: []core.FnSigSpec{{}}})
	if !fu.Parent.Equal(core.TFnUndef) {
		t.Errorf("NewFnUndef parent = %v", fu.Parent)
	}
}

func TestClassAndResourceTypeValues(t *testing.T) {
	ct := testClassType()
	cv := core.NewClassType(core.TClass, ct)
	got, err := core.AsClassType(cv)
	if err != nil || got.Name != ct.Name {
		t.Errorf("AsClassType = %+v, %v", got, err)
	}
	if _, err := core.AsClassType(core.NewInteger(1)); err == nil {
		t.Error("AsClassType(1) should error")
	}

	fields := core.NewOrderedMap()
	fields.Set("id", core.NewTypeLiteral(core.TInteger))
	rt := core.ResourceTypeInfo{Fields: fields, Name: "Ideal/Resource/RVal"}
	rv := core.NewResourceType(core.TResource, rt)
	if !core.IsResourceType(rv) {
		t.Error("NewResourceType not recognised")
	}
	back, err := core.AsResourceType(rv)
	if err != nil || back.Name != rt.Name {
		t.Errorf("AsResourceType = %+v, %v", back, err)
	}
}

func TestStoreWithPrototype(t *testing.T) {
	proto := &core.StoreInstanceInfo{Data: map[string]core.Value{"greet": core.NewString("hi")}}
	s := core.NewStoreWithPrototype(core.TStore, proto)
	si, err := core.AsStore(s)
	if err != nil {
		t.Fatalf("AsStore: %v", err)
	}
	if si.Prototype != proto {
		t.Error("prototype not attached")
	}
	if _, err := core.AsStore(core.NewInteger(1)); err == nil {
		t.Error("AsStore(1) should error")
	}
}

func TestTableTypeAccessors(t *testing.T) {
	tt := core.NewTableType(intStrRecord())
	if !core.IsTableType(tt) {
		t.Error("NewTableType not recognised")
	}
	info, err := core.AsTableType(tt)
	if err != nil || info.Record.Fields.Len() != 2 {
		t.Errorf("AsTableType = %+v, %v", info, err)
	}
	if _, err := core.AsTableType(core.NewInteger(1)); err == nil {
		t.Error("AsTableType(1) should error")
	}
	if core.IsTableType(core.NewInteger(1)) {
		t.Error("IsTableType(1) = true")
	}
}

func TestRecordTypeAccessors(t *testing.T) {
	rv := core.NewRecordType(intStrRecord().Fields)
	if !core.IsRecordType(rv) {
		t.Error("NewRecordType not recognised")
	}
	ri, err := core.AsRecordType(rv)
	if err != nil || ri.Fields.Len() != 2 {
		t.Errorf("AsRecordType = %+v, %v", ri, err)
	}
	if _, err := core.AsRecordType(core.NewInteger(1)); err == nil {
		t.Error("AsRecordType(1) should error")
	}
}

func TestWordUsurpConstructor(t *testing.T) {
	w := core.NewWordUsurp("target", true)
	wi, err := core.AsWord(w)
	if err != nil {
		t.Fatalf("AsWord: %v", err)
	}
	if !wi.ForceUsurp || !wi.ForceVal || wi.Name != "target" {
		t.Errorf("usurp word info = %+v", wi)
	}
	plain := core.NewWordUsurp("other", false)
	pi, _ := core.AsWord(plain)
	if pi.ForceVal {
		t.Error("non-ref usurp gained ForceVal")
	}
}

func TestMapFieldHelpers(t *testing.T) {
	m := core.NewOrderedMap()
	m.Set("s", core.NewString("v"))
	m.Set("i", core.NewInteger(3))
	m.Set("b", core.NewBoolean(true))
	m.Set("f", core.NewFloat(1.5))
	mv := core.NewMap(m)
	rm, err := core.AsMap(mv)
	if err != nil {
		t.Fatalf("AsMap: %v", err)
	}
	if s, ok := core.MapFieldString(rm, "s"); !ok || s != "v" {
		t.Errorf("MapFieldString = %q, %v", s, ok)
	}
	if n, ok := core.MapFieldInteger(rm, "i"); !ok || n != 3 {
		t.Errorf("MapFieldInteger = %d, %v", n, ok)
	}
	if b, ok := core.MapFieldBoolean(rm, "b"); !ok || !b {
		t.Errorf("MapFieldBoolean = %v, %v", b, ok)
	}
	if f, ok := core.MapFieldFloat(rm, "f"); !ok || f != 1.5 {
		t.Errorf("MapFieldFloat = %v, %v", f, ok)
	}
	// Negatives: absent key and wrong kind.
	if _, ok := core.MapFieldString(rm, "missing"); ok {
		t.Error("missing key found")
	}
	if _, ok := core.MapFieldInteger(rm, "s"); ok {
		t.Error("string field read as integer")
	}
}

func TestConcreteScalarAccessors(t *testing.T) {
	if s, err := core.NewString("x").AsConcreteString(); err != nil || s != "x" {
		t.Errorf("AsConcreteString = %q, %v", s, err)
	}
	if _, err := core.NewTypeLiteral(core.TString).AsConcreteString(); err == nil {
		t.Error("bare String literal read as concrete string")
	}
	if f, err := core.NewFloat(1.25).AsConcreteFloat(); err != nil || f != 1.25 {
		t.Errorf("AsConcreteFloat = %v, %v", f, err)
	}
	if _, err := core.NewTypeLiteral(core.TFloat).AsConcreteFloat(); err == nil {
		t.Error("bare Float literal read as concrete float")
	}
	if b, err := core.NewBoolean(true).AsConcreteBoolean(); err != nil || !b {
		t.Errorf("AsConcreteBoolean = %v, %v", b, err)
	}
	if _, err := core.NewTypeLiteral(core.TBoolean).AsConcreteBoolean(); err == nil {
		t.Error("bare Boolean literal read as concrete boolean")
	}
}

func TestReadListGetOkAndOrderedMapExtras(t *testing.T) {
	rl := core.NewReadList([]core.Value{core.NewInteger(1)})
	if v, ok := rl.GetOk(0); !ok || v.String() != "1" {
		t.Errorf("GetOk(0) = %v, %v", v, ok)
	}
	if _, ok := rl.GetOk(5); ok {
		t.Error("GetOk out of range returned ok")
	}
	if _, ok := rl.GetOk(-1); ok {
		t.Error("GetOk(-1) returned ok")
	}

	m := core.NewOrderedMap()
	m.Set("b", core.NewInteger(2))
	m.Set("a", core.NewInteger(1))
	keys := m.SortedKeys()
	if len(keys) != 2 || keys[0] != "a" {
		t.Errorf("SortedKeys = %v", keys)
	}
	m.Delete("a")
	if _, ok := m.Get("a"); ok {
		t.Error("Delete left the key behind")
	}
	if m.Len() != 1 {
		t.Errorf("Len after delete = %d", m.Len())
	}
}

func TestIsTypeValue(t *testing.T) {
	if !core.IsTypeValue(core.NewTypeLiteral(core.TInteger)) {
		t.Error("Integer literal should be a type value")
	}
	if !core.IsTypeValue(core.NewRecordType(intStrRecord().Fields)) {
		t.Error("record type should be a type value")
	}
	if core.IsTypeValue(core.NewInteger(1)) {
		t.Error("concrete integer misread as a type value")
	}
}

func TestNextMarkIDAndObjectTypeID(t *testing.T) {
	a, b := core.NextMarkID(), core.NextMarkID()
	if a == b {
		t.Errorf("NextMarkID not unique: %q vs %q", a, b)
	}
	x, y := core.GenerateObjectTypeID(), core.GenerateObjectTypeID()
	if x == y {
		t.Errorf("GenerateObjectTypeID not unique: %q vs %q", x, y)
	}
	if !strings.HasPrefix(x, "T_") {
		t.Errorf("object type ID %q missing T_ prefix", x)
	}
}

func TestFnDefOwnSigsAndFields(t *testing.T) {
	fd := core.FnDefInfo{Name: "f", Signatures: []core.Signature{
		{Params: []core.FnParam{{Name: "n", Type: core.TInteger}}, Returns: []*core.Type{core.TInteger}},
		{Fallback: true},
	}}
	own := fd.OwnSigs()
	for i := range own {
		if own[i].Fallback {
			t.Error("OwnSigs kept the synthesized fallback")
		}
	}
	first, ok := fd.FirstOwnSig()
	if !ok || first == nil || len(first.Params) != 1 {
		t.Errorf("FirstOwnSig = %+v, %v", first, ok)
	}
	empty := core.FnDefInfo{}
	if _, ok := empty.FirstOwnSig(); ok {
		t.Error("FirstOwnSig on empty def should report false")
	}
}
