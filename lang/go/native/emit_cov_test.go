package native

import (
	"errors"
	"strings"
	"testing"
)

// Coverage tests for emit.go: the format emitters (json/jsonic/yaml/
// csv/tsv/toml/ini/xml), the opts helpers, the kind registry, and the
// emit error plumbing. Unsupported shapes are pinned as rejections.

// covEmitMap builds a concrete map value preserving insertion order.
func covEmitMap(pairs ...any) Value {
	om := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		om.Set(pairs[i].(string), pairs[i+1].(Value))
	}
	return NewMap(om)
}

// ---- opts helpers ----

func TestOptBoolBranches(t *testing.T) {
	if optBool(nil, "pretty") {
		t.Error("nil opts must be false")
	}
	if optBool(map[string]any{}, "pretty") {
		t.Error("missing key must be false")
	}
	if optBool(map[string]any{"pretty": "yes"}, "pretty") {
		t.Error("non-bool value must be false")
	}
	if !optBool(map[string]any{"pretty": true}, "pretty") {
		t.Error("true must be true")
	}
}

func TestOptIndentBranches(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
		want int
		on   bool
	}{
		{"int", map[string]any{"indent": int(3)}, 3, true},
		{"int64", map[string]any{"indent": int64(4)}, 4, true},
		{"float64", map[string]any{"indent": float64(5)}, 5, true},
		{"negative clamps", map[string]any{"indent": -2}, 0, true},
		{"pretty default", map[string]any{"pretty": true}, 2, true},
		{"nil off", nil, 0, false},
		{"wrong type off", map[string]any{"indent": "x"}, 0, false},
	}
	for _, c := range cases {
		got, on := optIndent(c.opts, 2)
		if got != c.want || on != c.on {
			t.Errorf("optIndent %s = (%d,%v), want (%d,%v)", c.name, got, on, c.want, c.on)
		}
	}
}

func TestOptSeparatorBranches(t *testing.T) {
	if got := optSeparator(nil, ","); got != "," {
		t.Errorf("nil opts = %q, want ,", got)
	}
	if got := optSeparator(map[string]any{"separation": ""}, ","); got != "," {
		t.Errorf("empty separation = %q, want default", got)
	}
	if got := optSeparator(map[string]any{"separation": ";"}, ","); got != ";" {
		t.Errorf("separation = %q, want ;", got)
	}
}

// ---- kind registry / schema surface ----

func TestEmitKindsOrderAndNames(t *testing.T) {
	kinds := EmitKinds()
	wantOrder := []string{"json", "jsonic", "yaml", "csv", "tsv", "toml", "ini", "xml"}
	if len(kinds) != len(wantOrder) {
		t.Fatalf("EmitKinds len = %d, want %d", len(kinds), len(wantOrder))
	}
	for i, k := range kinds {
		if k.Name != wantOrder[i] {
			t.Errorf("kind[%d] = %q, want %q", i, k.Name, wantOrder[i])
		}
	}
	names := EmitKindNames()
	if !strings.HasPrefix(names, "json, ") || !strings.HasSuffix(names, ", xml") {
		t.Errorf("EmitKindNames = %q", names)
	}
}

func TestEmitOptsSchemaKinds(t *testing.T) {
	for _, kind := range []string{"json", "jsonic", "yaml", "csv", "tsv", "toml", "ini", "xml"} {
		v, ok := EmitOptsSchema(kind)
		if !ok {
			t.Errorf("EmitOptsSchema(%s) ok = false", kind)
			continue
		}
		if !v.Parent().ConformsTo(TMap) {
			t.Errorf("EmitOptsSchema(%s) is not map-family: %s", kind, v.Parent())
		}
	}
	if _, ok := EmitOptsSchema("bogus"); ok {
		t.Error("EmitOptsSchema(bogus) ok = true, want false")
	}
	auto := EmitAutoOptsSchema()
	if !auto.Parent().ConformsTo(TMap) {
		t.Errorf("EmitAutoOptsSchema not map-family: %s", auto.Parent())
	}
}

func TestNaturalEmitKind(t *testing.T) {
	xml := NewXmlElement("t", nil, nil)
	tdVal := ValueOf(&Datum{Parent: TList, Data: covTableNameAge().td})

	cases := []struct {
		name string
		v    Value
		want string
		ok   bool
	}{
		{"xml", xml, "xml", true},
		{"table payload", tdVal, "csv", true},
		{"list", NewList([]Value{NewInteger(1)}), "json", true},
		{"map", covEmitMap("a", NewInteger(1)), "json", true},
		{"error ideal", NewError(errors.New("boom")), "json", true},
		{"string", NewString("s"), "json", true},
		{"integer", NewInteger(1), "json", true},
		{"float", NewFloat(1.5), "json", true},
		{"boolean", NewBoolean(true), "json", true},
		{"atom", NewAtom("a"), "json", true},
		{"none", ValueOf(&Datum{Parent: TNone}), "json", true},
		{"word has no natural kind", NewWord("w"), "", false},
	}
	for _, c := range cases {
		got, ok := NaturalEmitKind(c.v)
		if got != c.want || ok != c.ok {
			t.Errorf("NaturalEmitKind %s = (%q,%v), want (%q,%v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// ---- json / jsonic ----

func TestEncodeJSONShapes(t *testing.T) {
	v := covEmitMap(
		"s", NewString("x"),
		"n", NewInteger(1),
		"f", NewFloat(2.5),
		"b", NewBoolean(true),
		"b2", NewBoolean(false),
		"none", ValueOf(&Datum{Parent: TNone}),
		"at", NewAtom("tom"),
		"lst", NewList([]Value{NewInteger(1), NewInteger(2)}),
		"em", covEmitMap(),
		"el", NewList([]Value{}),
	)
	got, err := encodeJSON(v, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"s":"x","n":1,"f":2.5,"b":true,"b2":false,"none":null,"at":"tom","lst":[1,2],"em":{},"el":[]}`
	if got != want {
		t.Errorf("compact json = %q, want %q", got, want)
	}

	pretty, err := encodeJSON(v, map[string]any{"pretty": true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pretty, "\n  \"s\": \"x\",") || !strings.Contains(pretty, "\n  \"lst\": [\n    1,") {
		t.Errorf("pretty json shape wrong:\n%s", pretty)
	}
}

func TestEncodeJSONQuoting(t *testing.T) {
	got, err := encodeJSON(NewString("a\"b\\c\nd\re\tf\bg\fh\x01i"), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `"a\"b\\c\nd\re\tf\bg\fh\u0001i"`
	if got != want {
		t.Errorf("jsonQuote = %q, want %q", got, want)
	}
}

func TestEncodeJsonicBareKeys(t *testing.T) {
	v := covEmitMap("abc", NewInteger(1), "a b", NewInteger(2), "", NewInteger(3))
	got, err := encodeJsonic(v, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `{abc:1,"a b":2,"":3}`
	if got != want {
		t.Errorf("jsonic = %q, want %q", got, want)
	}
}

func TestEncodeJSONXmlLeaf(t *testing.T) {
	xml := NewXmlElement("tag", nil, nil)
	got, err := encodeJSON(xml, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, `"`) || !strings.Contains(got, "tag") {
		t.Errorf("xml-in-json = %q, want a quoted string containing the tag", got)
	}
}

func TestClassifyNodeCarriers(t *testing.T) {
	kind, entries := classifyNode(NewTypedList(NewTypeLiteral(TInteger)))
	if kind != emitList || len(entries) != 0 {
		t.Errorf("typed list carrier = (%v,%d entries), want (emitList,0)", kind, len(entries))
	}
	kind, entries = classifyNode(NewCarrier(TMap))
	if kind != emitMap || len(entries) != 0 {
		t.Errorf("map carrier = (%v,%d entries), want (emitMap,0)", kind, len(entries))
	}
	kind, _ = classifyNode(NewError(errors.New("x")))
	if kind != emitMap {
		t.Errorf("error ideal = %v, want emitMap", kind)
	}
	kind, _ = classifyNode(NewInteger(1))
	if kind != emitScalar {
		t.Errorf("integer = %v, want emitScalar", kind)
	}
}

// ---- yaml ----

func TestEncodeYAMLScalarTop(t *testing.T) {
	got, err := encodeYAML(NewInteger(5), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "5" {
		t.Errorf("yaml scalar = %q, want 5", got)
	}
}

func TestEncodeYAMLMapAndList(t *testing.T) {
	v := covEmitMap(
		"s", NewString("x"),
		"n", NewInteger(5),
		"none", ValueOf(&Datum{Parent: TNone}),
		"em", covEmitMap(),
		"el", NewList([]Value{}),
		"sub", covEmitMap("k", NewString("v")),
		"lst", NewList([]Value{NewInteger(1), NewString("two")}),
	)
	got, err := encodeYAML(v, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		"s: x", "n: 5", "none: null", "em: {}", "el: []",
		"sub:\n  k: v", "lst:\n  - 1\n  - two",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("yaml missing %q in:\n%s", frag, got)
		}
	}

	lst := NewList([]Value{
		NewInteger(1),
		covEmitMap("a", NewInteger(1)),
		NewList([]Value{}),
		covEmitMap(),
		NewList([]Value{NewInteger(9)}),
	})
	got, err = encodeYAML(lst, map[string]any{"indent": 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{"- 1", "a: 1", "- []", "- {}", "- 9"} {
		if !strings.Contains(got, frag) {
			t.Errorf("yaml list missing %q in:\n%s", frag, got)
		}
	}

	// Empty containers at the top render their empty forms.
	if got, err := encodeYAML(covEmitMap(), nil); err != nil || got != "{}" {
		t.Errorf("yaml empty map = (%q,%v), want ({},nil)", got, err)
	}
	if got, err := encodeYAML(NewList([]Value{}), nil); err != nil || got != "[]" {
		t.Errorf("yaml empty list = (%q,%v), want ([],nil)", got, err)
	}
}

func TestEncodeYAMLXmlLeaf(t *testing.T) {
	v := covEmitMap("x", NewXmlElement("tag", nil, nil))
	got, err := encodeYAML(v, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "x: ") || !strings.Contains(got, "tag") {
		t.Errorf("yaml xml leaf = %q", got)
	}
	// An Xml element inside a list is also a leaf.
	got, err = encodeYAML(NewList([]Value{NewXmlElement("tag", nil, nil)}), nil)
	if err != nil || !strings.Contains(got, "tag") {
		t.Errorf("yaml xml list leaf = (%q,%v)", got, err)
	}
}

func TestYamlScalarQuoting(t *testing.T) {
	cases := []struct {
		in     string
		quoted bool
	}{
		{"", true},
		{"true", true},
		{"no", true},
		{"~", true},
		{"123", true},
		{"1.5", true},
		{" lead", true},
		{"trail ", true},
		{"a:b", true},
		{"-x", true},
		{"?q", true},
		{"plain", false},
	}
	for _, c := range cases {
		got := yamlScalar(NewString(c.in))
		if c.quoted && !strings.HasPrefix(got, `"`) {
			t.Errorf("yamlScalar(%q) = %q, want quoted", c.in, got)
		}
		if !c.quoted && got != c.in {
			t.Errorf("yamlScalar(%q) = %q, want plain", c.in, got)
		}
	}
	// Non-string values fall back to String().
	if got := yamlScalar(NewInteger(5)); got != "5" {
		t.Errorf("yamlScalar(5) = %q, want 5", got)
	}
	// yamlLeaf falls back to a quoted String() for non-scalars.
	if got := yamlLeaf(NewWord("w")); got == "" {
		t.Error("yamlLeaf(word) must render something")
	}
}

// ---- csv / tsv ----

func TestEncodeTabularTableAndOpts(t *testing.T) {
	tbl := covTableNameAge()
	tdVal := ValueOf(&Datum{Parent: TList, Data: tbl.td})

	kinds := EmitKinds()
	csv, err := kinds[3].Encode(tdVal, map[string]any{"separation": ";"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(csv, "name;age\n") || !strings.Contains(csv, "alice;30") {
		t.Errorf("csv = %q", csv)
	}
	tsv, err := kinds[4].Encode(tdVal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tsv, "name\tage\n") {
		t.Errorf("tsv = %q", tsv)
	}

	// A Table with an empty schema renders empty.
	empty := TableData{Record: RecordTypeInfo{Fields: NewOrderedMap()}}
	got, err := encodeTabular(ValueOf(&Datum{Parent: TList, Data: empty}), ",", "csv")
	if err != nil || got != "" {
		t.Errorf("empty-schema table = (%q,%v), want empty", got, err)
	}
}

func TestEncodeTabularListShapes(t *testing.T) {
	// List of records: header from the first row, missing keys empty,
	// non-map rows skipped, cells quoted per RFC 4180.
	row1 := covEmitMap("a", NewString("x,y"), "b", NewString(`q"z`))
	row2 := covEmitMap("a", NewString("plain"))
	rows := NewList([]Value{row1, row2, NewInteger(7)})
	got, err := encodeTabular(rows, ",", "csv")
	if err != nil {
		t.Fatal(err)
	}
	want := "a,b\n\"x,y\",\"q\"\"z\"\nplain,\n"
	if got != want {
		t.Errorf("list-of-records = %q, want %q", got, want)
	}

	// List of lists: no header, non-list rows degrade to one cell.
	lol := NewList([]Value{
		NewList([]Value{NewInteger(1), NewString("a\nb")}),
		NewString("solo"),
	})
	got, err = encodeTabular(lol, ",", "csv")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1,\"a\nb\"\nsolo\n" {
		t.Errorf("list-of-lists = %q", got)
	}

	// Empty list renders empty.
	got, err = encodeTabular(NewList([]Value{}), ",", "csv")
	if err != nil || got != "" {
		t.Errorf("empty list = (%q,%v)", got, err)
	}

	// Non-list shapes are rejected.
	_, err = encodeTabular(covEmitMap("a", NewInteger(1)), ",", "csv")
	if err == nil || EmitErrorCode(err) != "emit_unsupported" {
		t.Errorf("map: expected emit_unsupported, got %v", err)
	}
}

func TestEncodeTabularMaterializer(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tbl := covTableNameAge()
	qb := NewQueryBuilder(r, tbl.td)
	got, err := encodeTabular(wrapQB(qb), ",", "csv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "name,age\n") || !strings.Contains(got, "bob") {
		t.Errorf("materializer csv = %q", got)
	}

	// A query with no FROM errors through the encode wrapper.
	_, err = encodeTabular(wrapQB(newSelectBuilder(r, nil)), ",", "csv")
	if err == nil || !strings.Contains(err.Error(), "encode:") {
		t.Errorf("no-source query: expected encode error, got %v", err)
	}

	// A non-query extension payload is not materializable.
	bad := ValueOf(&Datum{Parent: TList, Data: ExtensionPayload{Body: 42}})
	_, err = encodeTabular(bad, ",", "csv")
	if err == nil || EmitErrorCode(err) != "emit_unsupported" {
		t.Errorf("non-query payload: expected emit_unsupported, got %v", err)
	}
}

// ---- toml ----

func TestEncodeTOMLShapes(t *testing.T) {
	v := covEmitMap(
		"i", NewInteger(1),
		"f", NewFloat(1.5),
		"b", NewBoolean(true),
		"s", NewString("x"),
		"arr", NewList([]Value{NewInteger(1), NewInteger(2)}),
		"k y", NewString("q"),
		"tbl", covEmitMap(
			"a", NewInteger(1),
			"sub", covEmitMap("b2", NewInteger(2)),
		),
	)
	got, err := encodeTOML(v, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		"i = 1", "f = 1.5", "b = true", `s = "x"`,
		"arr = [1, 2]", `"k y" = "q"`, "[tbl]", "a = 1", "[tbl.sub]", "b2 = 2",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("toml missing %q in:\n%s", frag, got)
		}
	}
}

func TestEncodeTOMLRejections(t *testing.T) {
	// Top-level non-map.
	_, err := encodeTOML(NewList([]Value{NewInteger(1)}), nil)
	if err == nil || EmitErrorCode(err) != "emit_unsupported" {
		t.Errorf("list top: expected emit_unsupported, got %v", err)
	}
	// None has no TOML representation.
	_, err = encodeTOML(covEmitMap("n", ValueOf(&Datum{Parent: TNone})), nil)
	if err == nil {
		t.Error("None value: expected error, got nil")
	}
	// Arrays of tables are unsupported.
	_, err = encodeTOML(covEmitMap("arr", NewList([]Value{covEmitMap("a", NewInteger(1))})), nil)
	if err == nil {
		t.Error("array of tables: expected error, got nil")
	}
	// Nested lists inside arrays are also non-scalar.
	_, err = encodeTOML(covEmitMap("arr", NewList([]Value{NewList([]Value{NewInteger(1)})})), nil)
	if err == nil {
		t.Error("array of arrays: expected error, got nil")
	}
	// Errors propagate out of nested tables.
	_, err = encodeTOML(covEmitMap("t", covEmitMap("bad", ValueOf(&Datum{Parent: TNone}))), nil)
	if err == nil {
		t.Error("nested table with None: expected error, got nil")
	}
}

func TestTomlKeyForms(t *testing.T) {
	if got := tomlKey(""); got != `""` {
		t.Errorf("empty key = %q", got)
	}
	if got := tomlKey("a_-9Z"); got != "a_-9Z" {
		t.Errorf("bare key = %q", got)
	}
	if got := tomlKey("a.b"); got != `"a.b"` {
		t.Errorf("dotted key = %q", got)
	}
}

// ---- ini ----

func TestEncodeINIShapes(t *testing.T) {
	v := covEmitMap(
		"x", NewInteger(1),
		"s", NewString("v"),
		"sec", covEmitMap("k", NewString("v2"), "n", NewInteger(2)),
		"sec2", covEmitMap("q", NewBoolean(true)),
	)
	got, err := encodeINI(v, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "x=1\ns=v\n\n[sec]\nk=v2\nn=2\n\n[sec2]\nq=true"
	if got != want {
		t.Errorf("ini = %q, want %q", got, want)
	}
}

func TestEncodeINIRejections(t *testing.T) {
	_, err := encodeINI(NewInteger(1), nil)
	if err == nil || EmitErrorCode(err) != "emit_unsupported" {
		t.Errorf("scalar top: expected emit_unsupported, got %v", err)
	}
	_, err = encodeINI(covEmitMap("l", NewList([]Value{NewInteger(1)})), nil)
	if err == nil {
		t.Error("list value: expected error, got nil")
	}
	_, err = encodeINI(covEmitMap("sec", covEmitMap("deep", covEmitMap("x", NewInteger(1)))), nil)
	if err == nil {
		t.Error("nested section: expected error, got nil")
	}
	// iniScalar falls back to String() for non-scalar-leaf shapes.
	none := ValueOf(&Datum{Parent: TNone})
	if got := iniScalar(none); got != none.String() {
		t.Errorf("iniScalar(None) = %q, want String() fallback %q", got, none.String())
	}
}

// ---- xml ----

func TestEncodeXMLPrettyAndPlain(t *testing.T) {
	attrs := NewOrderedMap()
	attrs.Set("id", NewString(`a&b<c>"d`))
	xml := NewXmlElement("root", attrs, []Value{
		NewXmlElement("kid", nil, []Value{NewString("hi & <there>")}),
		NewString("   "),
		NewString("tail"),
		NewXmlElement("empty", nil, nil),
	})

	plain, err := encodeXML(xml, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plain != xml.String() {
		t.Errorf("plain xml must be String(): %q vs %q", plain, xml.String())
	}

	pretty, err := encodeXML(xml, map[string]any{"pretty": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		`id="a&amp;b&lt;c&gt;&quot;d"`,
		"<kid>hi &amp; &lt;there&gt;</kid>",
		"<empty/>",
		"tail",
		"</root>",
	} {
		if !strings.Contains(pretty, frag) {
			t.Errorf("pretty xml missing %q in:\n%s", frag, pretty)
		}
	}
	if strings.Contains(pretty, "   \n") {
		t.Errorf("whitespace-only text must be dropped:\n%s", pretty)
	}

	// All-text element stays inline.
	textOnly := NewXmlElement("p", nil, []Value{NewString("hello")})
	pretty, err = encodeXML(textOnly, map[string]any{"pretty": true})
	if err != nil || !strings.Contains(pretty, "<p>hello</p>") {
		t.Errorf("all-text pretty = (%q,%v)", pretty, err)
	}

	// A non-Xml top is rejected.
	_, err = encodeXML(NewInteger(1), nil)
	if err == nil || EmitErrorCode(err) != "emit_unsupported" {
		t.Errorf("non-xml: expected emit_unsupported, got %v", err)
	}
}

func TestXmlPrettyNonXmlFallback(t *testing.T) {
	// A non-Xml node renders as its escaped String() form on its own line.
	var sb strings.Builder
	v := NewString("x<y")
	xmlPretty(&sb, v, 2, 0)
	want := xmlText(v.String()) + "\n"
	if sb.String() != want {
		t.Errorf("fallback = %q, want %q", sb.String(), want)
	}
}

// ---- error plumbing ----

func TestEmitErrorCodePaths(t *testing.T) {
	err := emitUnsupported("toml", "detail")
	if err.Error() != "toml: detail" {
		t.Errorf("emitError message = %q", err.Error())
	}
	if EmitErrorCode(err) != "emit_unsupported" {
		t.Errorf("EmitErrorCode = %q, want emit_unsupported", EmitErrorCode(err))
	}
	if EmitErrorCode(errors.New("plain")) != "" {
		t.Error("plain error must map to empty code")
	}
}

// ---- residual edge branches ----

func TestEmitEdgeBranches(t *testing.T) {
	// A Table-typed value (non-TableData payload) is list-like and its
	// natural kind is csv.
	tv := NewValueRaw(TTable, StrPayload{S: "x"})
	if kind, _ := classifyNode(tv); kind != emitList {
		t.Errorf("Table value classify = %v, want emitList", kind)
	}
	if got, ok := NaturalEmitKind(tv); !ok || got != "csv" {
		t.Errorf("NaturalEmitKind(Table) = (%q,%v), want csv", got, ok)
	}

	// A non-concrete list carrier still classifies as a list.
	if kind, entries := classifyNode(NewCarrier(TList)); kind != emitList || len(entries) != 0 {
		t.Errorf("list carrier classify = (%v,%d), want (emitList,0)", kind, len(entries))
	}

	// A Word leaf falls through to the quoted-String() arm of json.
	got, err := encodeJSON(NewWord("w"), nil)
	if err != nil || !strings.HasPrefix(got, `"`) {
		t.Errorf("json word leaf = (%q,%v), want quoted string", got, err)
	}

	// None inside a TOML array is a scalar with no TOML form.
	if _, err := encodeTOML(covEmitMap("arr", NewList([]Value{ValueOf(&Datum{Parent: TNone})})), nil); err == nil {
		t.Error("None in toml array: expected error, got nil")
	}

	// A non-string text child of an all-text XML element renders via
	// its String() form.
	pretty, err := encodeXML(
		NewXmlElement("p", nil, []Value{NewInteger(5)}),
		map[string]any{"pretty": true})
	if err != nil || !strings.Contains(pretty, "<p>5</p>") {
		t.Errorf("non-string text child = (%q,%v)", pretty, err)
	}
}
