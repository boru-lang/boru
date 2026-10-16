package native

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	runBoru(t, r, []Value{NewWord("print"), NewString("hello")})

	got := strings.TrimSpace(buf.String())
	if got != "hello" {
		t.Errorf("print string: got %q, want %q", got, "hello")
	}
}

func TestPrintInteger(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	runBoru(t, r, []Value{NewWord("print"), NewInteger(42)})

	got := strings.TrimSpace(buf.String())
	if got != "42" {
		t.Errorf("print integer: got %q, want %q", got, "42")
	}
}

func TestPrintBoolean(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	runBoru(t, r, []Value{NewWord("print"), NewBoolean(true)})

	got := strings.TrimSpace(buf.String())
	if got != "true" {
		t.Errorf("print boolean: got %q, want %q", got, "true")
	}
}

func TestPrintMap(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	om := NewOrderedMap()
	om.Set("a", NewBoolean(true))
	runBoru(t, r, []Value{NewWord("print"), NewMap(om)})

	got := strings.TrimSpace(buf.String())
	want := `{"a": true}`
	if got != want {
		t.Errorf("print map: got %q, want %q", got, want)
	}
}

func TestPrintMapMultiKey(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	om := NewOrderedMap()
	om.Set("x", NewInteger(1))
	om.Set("y", NewString("hello"))
	runBoru(t, r, []Value{NewWord("print"), NewMap(om)})

	got := strings.TrimSpace(buf.String())
	want := `{"x": 1, "y": "hello"}`
	if got != want {
		t.Errorf("print map: got %q, want %q", got, want)
	}
}

func TestPrintList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	list := NewList([]Value{NewInteger(1), NewString("two"), NewBoolean(false)})
	runBoru(t, r, []Value{NewWord("print"), list})

	got := strings.TrimSpace(buf.String())
	want := `[1, "two", false]`
	if got != want {
		t.Errorf("print list: got %q, want %q", got, want)
	}
}

func TestPrintTable(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	fields.Set("y", NewTypeLiteral(TString))
	recType := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("x", NewInteger(1))
	row1.Set("y", NewString("a"))
	row2 := NewOrderedMap()
	row2.Set("x", NewInteger(2))
	row2.Set("y", NewString("b"))

	table := ValueOf(&Datum{Parent: TList, Data: TableData{
		Record: recType,
		Rows:   []Value{NewMap(row1), NewMap(row2)},
	}})

	runBoru(t, r, []Value{NewWord("print"), table})

	got := strings.TrimSpace(buf.String())
	// Expected aligned table output:
	// x | y
	// --+--
	// 1 | a
	// 2 | b
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %q", len(lines), got)
	}
	if !strings.Contains(lines[0], "x") || !strings.Contains(lines[0], "y") {
		t.Errorf("header missing columns: %q", lines[0])
	}
	if !strings.Contains(lines[1], "-") {
		t.Errorf("expected separator line: %q", lines[1])
	}
	if !strings.Contains(lines[2], "1") || !strings.Contains(lines[2], "a") {
		t.Errorf("row 1 missing data: %q", lines[2])
	}
	if !strings.Contains(lines[3], "2") || !strings.Contains(lines[3], "b") {
		t.Errorf("row 2 missing data: %q", lines[3])
	}
}

func TestPrintConsumesValue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	// print should consume the value, leaving empty stack
	result := runBoru(t, r, []Value{NewWord("print"), NewString("gone")})
	if len(result) != 0 {
		t.Errorf("print should consume value, stack has %d items", len(result))
	}
}

func TestPrintEmptyTable(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	fields := NewOrderedMap()
	recType := RecordTypeInfo{Fields: fields}
	table := ValueOf(&Datum{Parent: TList, Data: TableData{
		Record: recType,
		Rows:   nil,
	}})

	runBoru(t, r, []Value{NewWord("print"), table})

	got := strings.TrimSpace(buf.String())
	if got != "(empty table)" {
		t.Errorf("empty table: got %q, want %q", got, "(empty table)")
	}
}

func TestPrintstrString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	runBoru(t, r, []Value{NewWord("printstr"), NewString("hello")})

	got := buf.String()
	if got != "hello" {
		t.Errorf("printstr string: got %q, want %q", got, "hello")
	}
}

func TestPrintstrNoNewline(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	runBoru(t, r, []Value{NewWord("printstr"), NewString("abc")})

	got := buf.String()
	if strings.Contains(got, "\n") {
		t.Errorf("printstr should not emit newline, got %q", got)
	}
}

func TestPrintstrInteger(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	runBoru(t, r, []Value{NewWord("printstr"), NewInteger(42)})

	got := buf.String()
	if got != "42" {
		t.Errorf("printstr integer: got %q, want %q", got, "42")
	}
}

func TestPrintstrConsumesValue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	result := runBoru(t, r, []Value{NewWord("printstr"), NewString("gone")})
	if len(result) != 0 {
		t.Errorf("printstr should consume value, stack has %d items", len(result))
	}
}

func TestFormatValueJSON(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"string", NewString("hi"), `"hi"`},
		{"integer", NewInteger(7), "7"},
		{"bool_true", NewBoolean(true), "true"},
		{"bool_false", NewBoolean(false), "false"},
		{"none", ValueOf(&Datum{Parent: TNone}), "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatValueJSON(tt.val)
			if got != tt.want {
				t.Errorf("FormatValueJSON = %q, want %q", got, tt.want)
			}
		})
	}
}
