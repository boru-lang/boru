package native

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// TestTabnasKindsOrder pins the tabnas decoder family order. boru:parselang's
// ParseLang.kinds is derived from this slice and pinned by
// lang/spec/module-parselang.tsv §3, so a reorder is a breaking change.
func TestTabnasKindsOrder(t *testing.T) {
	want := []string{"ini", "json", "jsonic", "json5", "jsonc", "csv",
		"toml", "yaml", "xml", "zon", "markdown", "feed"}
	kinds := TabnasKinds()
	if len(kinds) != len(want) {
		t.Fatalf("TabnasKinds count = %d, want %d", len(kinds), len(want))
	}
	for i, w := range want {
		if kinds[i].Name != w {
			t.Errorf("TabnasKinds[%d] = %q, want %q", i, kinds[i].Name, w)
		}
		if kinds[i].Returns == nil {
			t.Errorf("TabnasKinds[%d] (%s) has nil Returns", i, w)
		}
		if kinds[i].Parse == nil || kinds[i].Convert == nil {
			t.Errorf("TabnasKinds[%d] (%s) has nil Parse/Convert", i, w)
		}
	}
}

// TestDefaultFormatsTabnasFamily asserts the parser family is folded into the
// read registry WITHOUT overriding read's own encodable decoders.
func TestDefaultFormatsTabnasFamily(t *testing.T) {
	m := DefaultFormats()
	// Parser-backed additions read otherwise lacked.
	for _, name := range []string{"ini", "json5", "jsonc", "toml", "yaml", "xml", "zon", "markdown", "feed"} {
		f, ok := m[name]
		if !ok {
			t.Errorf("DefaultFormats missing %q", name)
			continue
		}
		if _, isTab := f.(*TabnasFormat); !isTab {
			t.Errorf("%q = %T, want *TabnasFormat", name, f)
		}
	}
	// read keeps its own csv/json/jsonic — they must NOT be replaced.
	if _, ok := m["csv"].(*CSVFormat); !ok {
		t.Errorf("csv = %T, want *CSVFormat (read's table decoder must win)", m["csv"])
	}
	if _, ok := m["json"].(*JSONFormat); !ok {
		t.Errorf("json = %T, want *JSONFormat", m["json"])
	}
	if _, ok := m["jsonic"].(*JsonicFormat); !ok {
		t.Errorf("jsonic = %T, want *JsonicFormat", m["jsonic"])
	}
}

// TestTabnasFormatReadOnly: a parser-backed format decodes but its Encode
// raises the read-only sentinel.
func TestTabnasFormatReadOnly(t *testing.T) {
	var ini *TabnasFormat
	for _, k := range TabnasKinds() {
		if k.Name == "ini" {
			ini = &TabnasFormat{Kind: k}
		}
	}
	if ini == nil {
		t.Fatal("no ini kind")
	}
	vals, err := ini.Decode("a = 1")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(vals) != 1 || !ParentOf(&vals[0]).Equal(TMap) {
		t.Fatalf("Decode = %v, want one Map", vals)
	}
	_, err = ini.Encode(NewString("x"))
	if !IsReadOnlyFormatErr(err) {
		t.Errorf("Encode err = %v, want read-only sentinel", err)
	}
}

// TestTabnasFormatDecodeOpts: the opts-aware path forwards options to the
// decoder (csv field separation), and Decode == DecodeOpts(nil).
func TestTabnasFormatDecodeOpts(t *testing.T) {
	var csv *TabnasFormat
	for _, k := range TabnasKinds() {
		if k.Name == "csv" {
			csv = &TabnasFormat{Kind: k}
		}
	}
	if csv == nil {
		t.Fatal("no csv kind")
	}
	vals, err := csv.DecodeOpts("a;b\n1;2", map[string]any{"field": map[string]any{"separation": ";"}})
	if err != nil {
		t.Fatalf("DecodeOpts: %v", err)
	}
	// csv → a List of rows; with ';' separation, row 0 has two fields.
	if len(vals) != 1 || !ParentOf(&vals[0]).ConformsTo(TList) {
		t.Fatalf("DecodeOpts csv = %v, want a List", vals)
	}
	rows, err := AsMutableList(vals[0])
	if err != nil || len(rows) == 0 {
		t.Fatalf("csv rows = %v (err %v)", rows, err)
	}
	r0, err := AsMutableList(rows[0])
	if err != nil || len(r0) != 2 {
		t.Fatalf("row 0 = %v (err %v), want 2 fields from ';' separation", r0, err)
	}
}

// readMem runs `read path [opts]` against an in-memory FS seeded with one file.
func readMem(t *testing.T, path string, content string, opts ...Value) []Value {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files[path] = []byte(content)
	SetHostFileOps(r, mem)
	tokens := []Value{NewWord("read"), pathV(path)}
	tokens = append(tokens, opts...)
	return runBoru(t, r, tokens)
}

func TestEngineReadINIByExtension(t *testing.T) {
	res := readMem(t, "conf.ini", "a = 1\n[sec]\nx = true")
	if len(res) != 1 || !ParentOf(&res[0]).Equal(TMap) {
		t.Fatalf("read .ini = %v, want a Map", res)
	}
	m, _ := AsMap(res[0])
	secV, ok := m.Get("sec")
	if !ok {
		t.Fatal("missing [sec] section")
	}
	sec, _ := AsMap(secV)
	xv, ok := sec.Get("x")
	if !ok {
		t.Fatal("missing sec.x")
	}
	if b, _ := AsBoolean(xv); !ParentOf(&xv).ConformsTo(TBoolean) || !b {
		t.Errorf("sec.x = %v, want Boolean true", xv)
	}
}

func TestEngineReadYAMLAndTOML(t *testing.T) {
	for _, tc := range []struct{ path, content string }{
		{"d.yaml", "b: hi"},
		{"d.yml", "b: hi"},
		{"e.toml", `b = "hi"`},
	} {
		res := readMem(t, tc.path, tc.content)
		if len(res) != 1 || !ParentOf(&res[0]).Equal(TMap) {
			t.Fatalf("read %s = %v, want a Map", tc.path, res)
		}
		m, _ := AsMap(res[0])
		bv, ok := m.Get("b")
		if !ok {
			t.Fatalf("%s missing key b", tc.path)
		}
		if s, _ := AsString(bv); s != "hi" {
			t.Errorf("%s b = %q, want hi", tc.path, s)
		}
	}
}

func TestEngineReadXMLByExtension(t *testing.T) {
	res := readMem(t, "doc.xml", "<r><a>1</a></r>")
	if len(res) != 1 {
		t.Fatalf("read .xml = %v", res)
	}
	if got := res[0].String(); got != "<r><a>1</a></r>" {
		t.Errorf("read .xml rendered %q, want <r><a>1</a></r>", got)
	}
}

func TestEngineReadFmtOverride(t *testing.T) {
	// {fmt:'text'} forces text even though the extension maps to ini.
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	res := readMem(t, "x.ini", "b = hello", NewMap(opts))
	if len(res) != 1 {
		t.Fatalf("read = %v", res)
	}
	if s, _ := AsString(res[0]); s != "b = hello" {
		t.Errorf("fmt:text override = %q, want raw 'b = hello'", s)
	}
}

func TestEngineReadUnknownFmt(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["x.dat"] = []byte("x")
	SetHostFileOps(r, mem)
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("nonesuch"))
	err = runBoruError(t, r, []Value{NewWord("read"), pathV("x.dat"), NewMap(opts)})
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("err = %v, want 'unknown format'", err)
	}
}

func TestEngineWriteReadOnlyFormat(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("xml"))
	err = runBoruError(t, r, []Value{NewWord("write"), pathV("out.xml"), NewString("<r/>"), NewMap(opts)})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("write {fmt:xml} err = %v, want read-only", err)
	}
	if _, wrote := mem.Files["out.xml"]; wrote {
		t.Error("read-only write should not have created the file")
	}
}

// panicEncodeFormat is a read-only Format whose Encode panics. write must
// reject an explicit {fmt:…} that resolves to it via the ReadOnly marker —
// WITHOUT ever executing Encode (PR #169 review: validation must not run
// arbitrary host encoders).
type panicEncodeFormat struct{}

func (panicEncodeFormat) Decode(string) ([]Value, error) { return nil, nil }
func (panicEncodeFormat) Encode(Value) (string, error) {
	panic("Encode must not be called during write validation")
}
func (panicEncodeFormat) ReadOnly() bool { return true }

// TestWriteReadOnlyDoesNotInvokeEncode: an explicit read-only {fmt:…} is
// rejected via the marker, and the format's Encode is never executed (no
// panic), nor is the file written.
func TestWriteReadOnlyDoesNotInvokeEncode(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("write validation invoked Encode (panic: %v)", rec)
		}
	}()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)
	HostFormats(r)["spy"] = panicEncodeFormat{}
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("spy"))
	err = runBoruError(t, r, []Value{NewWord("write"), pathV("out.spy"), NewString("x"), NewMap(opts)})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("write {fmt:spy} err = %v, want read-only", err)
	}
	if _, wrote := mem.Files["out.spy"]; wrote {
		t.Error("read-only write should not have created the file")
	}
}

// TestFormatFromExtHostOverride: the extension map is host-overridable in
// place, and many-to-one defaults resolve.
func TestFormatFromExtHostOverride(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Many-to-one defaults.
	for _, tc := range []struct{ path, want string }{
		{"a.ini", "ini"}, {"a.cfg", "ini"}, {"a.conf", "ini"},
		{"a.yml", "yaml"}, {"a.yaml", "yaml"}, {"a.bogus", ""},
	} {
		if got := formatFromExt(r, tc.path); got != tc.want {
			t.Errorf("formatFromExt(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
	// Host override: map .bogus → text in place.
	HostExtensions(r)["bogus"] = "text"
	if got := formatFromExt(r, "a.bogus"); got != "text" {
		t.Errorf("after override formatFromExt(a.bogus) = %q, want text", got)
	}
}

// TestRegisterFormatBothRegistries: RegisterFormat installs the format under
// its name AND maps each extension to it.
func TestRegisterFormatBothRegistries(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	f := &TextFormat{}
	if err := RegisterFormat(r, "myfmt", f, ".mine", "MY2"); err != nil {
		t.Fatalf("RegisterFormat: %v", err)
	}
	if _, ok := HostFormats(r)["myfmt"]; !ok {
		t.Error("format not installed under name")
	}
	if got := formatFromExt(r, "x.mine"); got != "myfmt" {
		t.Errorf("ext .mine = %q, want myfmt", got)
	}
	if got := formatFromExt(r, "x.my2"); got != "myfmt" {
		t.Errorf("ext .MY2 (lowercased) = %q, want myfmt", got)
	}
}

// TestTabnasFormatNoPanic feeds type literals / nil opts through the decode
// and encode paths and asserts no panic occurs (Panic-Prevention discipline).
func TestTabnasFormatNoPanic(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("panic: %v", rec)
		}
	}()
	for _, k := range TabnasKinds() {
		f := &TabnasFormat{Kind: k}
		_, _ = f.Decode("")                   // empty source
		_, _ = f.DecodeOpts("", nil)          // nil opts
		_, _ = f.Encode(NewTypeLiteral(TMap)) // type literal to read-only encode
		_, _ = f.Encode(NewTypeLiteral(TList))
	}
	// OptsToMap must tolerate type-literal / non-map values.
	if OptsToMap(NewTypeLiteral(TMap)) != nil {
		t.Error("OptsToMap(type literal) should be nil")
	}
	if OptsToMap(NewInteger(0)) != nil {
		t.Error("OptsToMap(non-map) should be nil")
	}
}
