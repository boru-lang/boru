package test

import (
	"github.com/boru-lang/boru/lang/go/native"
	"strings"
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// runWithOSFiles creates a registry using real OS file ops and runs Boru.
// Files are resolved relative to the test package directory (lang/go/test/).
func runWithOSFiles(t *testing.T, expr string) ([]native.Value, error) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	registerIOWords(reg)
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)

	values, err := parser.Parse(expr)
	if err != nil {
		return nil, err
	}

	eng := native.NewTop(reg)
	return eng.Run(values)
}

// --- CSV file loading ---

func TestFileReadCSV(t *testing.T) {
	result, err := runWithOSFiles(t, `(make Pathon "file/people.csv") read`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	v := result[0]
	if !native.IsTableType(v) {
		t.Fatalf("expected table type, got %s", v.Parent())
	}

	_lst, _ := native.AsList(v)
	rows := _lst.Slice()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Check first row: Alice,30,London
	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "name", "Alice")
	assertField(t, r0, "age", "30")
	assertField(t, r0, "city", "London")

	// Check last row: Charlie,35,Tokyo
	r2, _ := native.AsMap(rows[2])
	assertField(t, r2, "name", "Charlie")
	assertField(t, r2, "city", "Tokyo")
}

func TestFileReadCSVSchema(t *testing.T) {
	result, err := runWithOSFiles(t, `(make Pathon "file/people.csv") read`)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	ti, _ := native.AsTableType(v)
	keys := ti.Record.Fields.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 columns, got %d: %v", len(keys), keys)
	}
	// Column order should match the CSV header order.
	if keys[0] != "name" || keys[1] != "age" || keys[2] != "city" {
		t.Errorf("columns = %v, want [name age city]", keys)
	}
}

func TestFileReadSimpleCSV(t *testing.T) {
	result, err := runWithOSFiles(t, `(make Pathon "file/simple.csv") read`)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	_lst, _ := native.AsList(v)
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "x", "1")
	assertField(t, r0, "y", "a")
}

func TestFileReadQuotedCSV(t *testing.T) {
	result, err := runWithOSFiles(t, `(make Pathon "file/quoted.csv") read`)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	_lst, _ := native.AsList(v)
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "name", "Smith, John")
	assertField(t, r0, "description", "Has a comma, in name")
}

func TestFileReadEmptyCSV(t *testing.T) {
	result, err := runWithOSFiles(t, `(make Pathon "file/empty.csv") read`)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	if !native.IsTableType(v) {
		// Empty CSV with only headers may return empty list
		if !v.Parent().Equal(native.TList) {
			t.Fatalf("expected list/table type, got %s", v.Parent())
		}
	}
	_lst, _ := native.AsList(v)
	rows := _lst.Slice()
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

// --- TSV file loading ---

func TestFileReadTSV(t *testing.T) {
	result, err := runWithOSFiles(t, `(make Pathon "file/items.tsv") read`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	v := result[0]
	if !native.IsTableType(v) {
		t.Fatalf("expected table type, got %s", v.Parent())
	}

	_lst, _ := native.AsList(v)
	rows := _lst.Slice()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Check first row: 1, Widget, 9.99
	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "id", "1")
	assertField(t, r0, "name", "Widget")
	assertField(t, r0, "price", "9.99")

	// Check third row: 3, Gizmo, 14.75
	r2, _ := native.AsMap(rows[2])
	assertField(t, r2, "id", "3")
	assertField(t, r2, "name", "Gizmo")
	assertField(t, r2, "price", "14.75")
}

func TestFileReadTSVSchema(t *testing.T) {
	result, err := runWithOSFiles(t, `(make Pathon "file/items.tsv") read`)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	ti, _ := native.AsTableType(v)
	keys := ti.Record.Fields.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 columns, got %d: %v", len(keys), keys)
	}
	if keys[0] != "id" || keys[1] != "name" || keys[2] != "price" {
		t.Errorf("columns = %v, want [id name price]", keys)
	}
}

// --- Extension override ---

func TestFileReadCSVWithTextOverride(t *testing.T) {
	// Read a CSV file but force text format — should get raw string.
	result, err := runWithOSFiles(t, `(make Pathon "file/simple.csv") {fmt:"text"} read`)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	if native.IsTableType(v) {
		t.Fatal("expected plain string with text override, got table")
	}
	s, _ := native.AsString(v)
	if !strings.Contains(s, "x,y") {
		t.Errorf("expected raw CSV content, got %q", s)
	}
}

func TestFileReadCSVExplicitFmt(t *testing.T) {
	// Read a CSV file with explicit csv format — should work same as auto.
	result, err := runWithOSFiles(t, `(make Pathon "file/people.csv") {fmt:"csv"} read`)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	if !native.IsTableType(v) {
		t.Fatalf("expected table type, got %s", v.Parent())
	}
	_lst, _ := native.AsList(v)
	rows := _lst.Slice()
	if len(rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(rows))
	}
}

// --- Print with file-loaded tables ---

func TestFileReadCSVPrint(t *testing.T) {
	reg, err := native.DefaultRegistry()
	registerIOWords(reg)
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	var buf strings.Builder
	reg.Output = &buf

	values, err := parser.Parse(`(make Pathon "file/people.csv") read print`)
	if err != nil {
		t.Fatal(err)
	}

	eng := native.NewTop(reg)
	result, err := eng.Run(values)
	if err != nil {
		t.Fatal(err)
	}

	// print consumes the value
	if len(result) != 0 {
		t.Errorf("expected empty stack after print, got %d values", len(result))
	}

	out := buf.String()
	// Should contain column headers
	if !strings.Contains(out, "name") || !strings.Contains(out, "age") || !strings.Contains(out, "city") {
		t.Errorf("table output missing headers: %q", out)
	}
	// Should contain data
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "London") {
		t.Errorf("table output missing data: %q", out)
	}
	// Should have separator line
	if !strings.Contains(out, "---") {
		t.Errorf("table output missing separator: %q", out)
	}
}

func TestFileReadTSVPrint(t *testing.T) {
	reg, err := native.DefaultRegistry()
	registerIOWords(reg)
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	var buf strings.Builder
	reg.Output = &buf

	values, err := parser.Parse(`(make Pathon "file/items.tsv") read print`)
	if err != nil {
		t.Fatal(err)
	}

	eng := native.NewTop(reg)
	_, err = eng.Run(values)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "Widget") || !strings.Contains(out, "Gadget") {
		t.Errorf("table output missing data: %q", out)
	}
}

// assertField checks that an OrderedMap has a field with the expected string value.
func assertField(t *testing.T, om native.ReadMap, key, want string) {
	t.Helper()
	val, ok := om.Get(key)
	if !ok {
		t.Errorf("missing field %q", key)
		return
	}
	got, _ := native.AsString(val)
	if got != want {
		t.Errorf("field %q = %q, want %q", key, got, want)
	}
}
