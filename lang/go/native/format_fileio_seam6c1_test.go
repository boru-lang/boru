package native

import (
	"errors"
	"strings"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// Wave-6 coverage for format.go (decoder error arms via the csvConfigure /
// jsonicDecodeValue seams, the extracted convertDelimitedRecords /
// delimitedCellValue helpers, RegisterFormat guards) and fileio.go
// (jsonicToValue wrapper cases, doRead / doWrite error arms).

func swapJsonicDecode(t *testing.T, fn func(any) (Value, error)) {
	t.Helper()
	old := jsonicDecodeValue
	jsonicDecodeValue = fn
	t.Cleanup(func() { jsonicDecodeValue = old })
}

func TestJSONFormatDecodeConversionError(t *testing.T) {
	swapJsonicDecode(t, func(any) (Value, error) {
		return Value{}, errors.New("conversion exploded")
	})
	if _, err := (&JSONFormat{}).Decode("1"); err == nil ||
		!strings.Contains(err.Error(), "conversion exploded") {
		t.Fatalf("expected conversion error, got %v", err)
	}
}

func TestJsonicFormatDecodeConversionError(t *testing.T) {
	swapJsonicDecode(t, func(any) (Value, error) {
		return Value{}, errors.New("conversion exploded")
	})
	if _, err := (&JsonicFormat{}).Decode("1"); err == nil ||
		!strings.Contains(err.Error(), "conversion exploded") {
		t.Fatalf("expected conversion error, got %v", err)
	}
}

func TestDecodeDelimitedConfigureError(t *testing.T) {
	old := csvConfigure
	csvConfigure = func(*jsonic.Jsonic, map[string]any) error {
		return errors.New("plugin declined")
	}
	t.Cleanup(func() { csvConfigure = old })
	if _, err := decodeDelimited("a,b\n1,2", ","); err == nil ||
		!strings.Contains(err.Error(), "plugin declined") {
		t.Fatalf("expected configure error, got %v", err)
	}
}

func TestDecodeDelimitedParseError(t *testing.T) {
	if _, err := decodeDelimited("\"unclosed", ","); err == nil ||
		!strings.Contains(err.Error(), "csv:") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestConvertDelimitedRecordsShapeGuards(t *testing.T) {
	// Non-slice header row → empty list.
	v := convertDelimitedRecords([]any{"not-a-row"})
	if lst, _ := AsList(v); lst.Len() != 0 {
		t.Fatalf("non-slice header should yield empty list, got %v", v)
	}
	// Empty header row → empty list.
	v = convertDelimitedRecords([]any{[]any{}})
	if lst, _ := AsList(v); lst.Len() != 0 {
		t.Fatalf("empty header should yield empty list, got %v", v)
	}
	// A non-slice data row is skipped; a short row pads with "".
	v = convertDelimitedRecords([]any{
		[]any{"a", "b"},
		"stray",
		[]any{"only-a"},
	})
	lst, _ := AsList(v)
	if lst.Len() != 1 {
		t.Fatalf("expected one surviving row, got %d", lst.Len())
	}
	row, _ := AsMap(lst.Slice()[0])
	bVal, _ := row.Get("b")
	if s, _ := AsString(bVal); s != "" {
		t.Fatalf("short row must pad b with empty string, got %v", bVal)
	}
}

func TestDelimitedCellValueArms(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"x", "'x'"},
		{2.0, "2"},
		{1.5, "1.5"},
		{true, "true"},
		{map[string]any{"k": "v"}, "'map[k:v]'"},
	}
	for _, tc := range cases {
		if got := delimitedCellValue(tc.in).String(); got != tc.want {
			t.Errorf("delimitedCellValue(%v) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestRegisterFormatGuardArms(t *testing.T) {
	// Empty name declined.
	r := seam5Reg(t)
	if err := RegisterFormat(r, "", &TextFormat{}); err == nil ||
		!strings.Contains(err.Error(), "name must not be empty") {
		t.Fatalf("expected empty-name error, got %v", err)
	}

	// No formats capability installed (bare registry).
	bare, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterFormat(bare, "foo", &TextFormat{}); err == nil ||
		!strings.Contains(err.Error(), "formats capability not installed") {
		t.Fatalf("expected capability error, got %v", err)
	}

	// Extensions slot empty → RegisterFormat lazily installs defaults.
	if _, err := r.Capabilities.Delete(CapExtensions); err != nil {
		t.Fatal(err)
	}
	if err := RegisterFormat(r, "foo", &TextFormat{}, ".Foo"); err != nil {
		t.Fatal(err)
	}
	em := HostExtensions(r)
	if em == nil || em["foo"] != "foo" {
		t.Fatalf("expected lazily-installed extension map with foo, got %v", em)
	}
}

func TestJsonicToValueWrapperAndErrorArms(t *testing.T) {
	// Nested unsupported type inside a list.
	if _, err := jsonicToValue([]any{make(chan int)}); err == nil ||
		!strings.Contains(err.Error(), "unsupported jsonic type") {
		t.Fatalf("expected nested list conversion error, got %v", err)
	}
	// Nested unsupported type inside a map.
	if _, err := jsonicToValue(map[string]any{"x": make(chan int)}); err == nil ||
		!strings.Contains(err.Error(), "unsupported jsonic type") {
		t.Fatalf("expected nested map conversion error, got %v", err)
	}
	// Nested unsupported type inside an OrderedMap (the insertion-order path).
	if _, err := jsonicToValue(&jsonic.OrderedMap{Keys: []string{"x"}, Vals: map[string]any{"x": make(chan int)}}); err == nil ||
		!strings.Contains(err.Error(), "unsupported jsonic type") {
		t.Fatalf("expected nested ordered-map conversion error, got %v", err)
	}
	// jsonic.Text wrapper.
	v, err := jsonicToValue(jsonic.Text{Str: "hi", Quote: `"`})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := AsString(v); s != "hi" {
		t.Fatalf("Text wrapper should convert to its string, got %v", v)
	}
	// jsonic.ListRef wrapper.
	v, err = jsonicToValue(jsonic.ListRef{Val: []any{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if lst, _ := AsList(v); lst.Len() != 1 {
		t.Fatalf("ListRef wrapper should convert its Val, got %v", v)
	}
	// jsonic.MapRef wrapper.
	v, err = jsonicToValue(jsonic.MapRef{Val: map[string]any{"k": "v"}})
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := AsMap(v); m == nil || m.Len() != 1 {
		t.Fatalf("MapRef wrapper should convert its Val, got %v", v)
	}
}

// failingReader always errors.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("stdin broke") }

// failingWriter always errors.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, errors.New("stdout broke") }

func TestDoReadErrorAndOptsArms(t *testing.T) {
	r := seam5Reg(t)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	// stdin read failure.
	r.Input = failingReader{}
	if _, err := doRead(r, pathStdin, "utf8", "text", "lf", nil, SrcPos{}); err == nil ||
		!strings.Contains(err.Error(), "stdin broke") {
		t.Fatalf("expected stdin error, got %v", err)
	}

	// DecodeOpts path: an opts-aware format (tabnas ini) with parser opts.
	if err := mem.WriteFile("conf.ini", []byte("[a]\nb=1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := doRead(r, "conf.ini", "utf8", "ini", "lf", map[string]any{"bogus": true}, SrcPos{})
	if err != nil {
		t.Fatalf("ini DecodeOpts: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one decoded value, got %d", len(out))
	}

	// Decoder failure: invalid json content.
	if err := mem.WriteFile("bad.json", []byte("{bad"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := doRead(r, "bad.json", "utf8", "json", "lf", nil, SrcPos{}); err == nil ||
		!strings.Contains(err.Error(), "read:") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestDoReadSQLiteArms(t *testing.T) {
	r := seam5Reg(t)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	// Backslash in the path: the table name derives from the basename.
	if err := mem.WriteFile(`C:\data\planets.csv`, []byte("a,b\n1,2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := doRead(r, `C:\data\planets.csv`, "utf8", "csv", "lf", nil, SrcPos{})
	if err != nil {
		t.Fatal(err)
	}
	td, ok := out[0].Data.(TableData)
	if !ok || td.TableName != "planets" {
		t.Fatalf("expected sqlite-backed table 'planets', got %#v", out[0].Data)
	}

	// StoreTable failure: a closed store must surface the error.
	if err := HostSQLite(r).Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := doRead(r, `C:\data\planets.csv`, "utf8", "csv", "lf", nil, SrcPos{}); err == nil ||
		!strings.Contains(err.Error(), "sqlite store") {
		t.Fatalf("expected sqlite store error, got %v", err)
	}
}

func TestDoWriteStdoutError(t *testing.T) {
	r := seam5Reg(t)
	r.Output = failingWriter{}
	if _, err := doWrite(r, pathStdout, "content", "utf8", "text", "write", "lf", false, false, SrcPos{}); err == nil ||
		!strings.Contains(err.Error(), "stdout broke") {
		t.Fatalf("expected stdout write error, got %v", err)
	}
	// Positive pair: stderr writes still succeed with a healthy writer.
	out, err := doWrite(r, pathStderr, "content", "utf8", "text", "write", "lf", false, false, SrcPos{})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := AsString(out[0]); s != pathStderr {
		t.Fatalf("expected path echo, got %v", out[0])
	}
}
