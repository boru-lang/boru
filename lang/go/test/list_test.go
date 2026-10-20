package test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// runNativeWithFiles creates a registry with native functions and in-memory files.
func runNativeWithFiles(t *testing.T, files map[string]string, expr string) ([]native.Value, error) {
	t.Helper()
	mem := capabilities.NewMem()
	for path, content := range files {
		mem.Files[filepath.Clean(filepath.FromSlash(path))] = []byte(content) // the model keys a file by its resolved, OS-native path
	}

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.SetHostFileOps(reg, mem)
	registerIOWords(reg)
	native.Register(reg)

	values, err := parser.Parse(expr)
	if err != nil {
		return nil, err
	}

	eng := native.NewTop(reg)
	return eng.Run(values)
}

// --- list: load CSV and list all rows ---

func TestListAllFromCSV(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,25,Paris\nCharlie,35,Tokyo\n"

	result, err := runNativeWithFiles(t, map[string]string{
		"people.csv": csv,
	}, `read (make Pathon "people.csv") list`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	names := make(map[string]bool)
	for _, row := range rows {
		m, _ := native.AsMap(row)
		v, _ := m.Get("name")
		vs, _ := native.AsString(v)
		names[vs] = true
	}
	for _, want := range []string{"Alice", "Bob", "Charlie"} {
		if !names[want] {
			t.Errorf("missing name %q in list result", want)
		}
	}
}

// --- list: load CSV and filter by matching fields ---

func TestListFilterFromCSV(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,25,Paris\nCharlie,35,London\n"

	result, err := runNativeWithFiles(t, map[string]string{
		"people.csv": csv,
	}, `read (make Pathon "people.csv") list {city:"London"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 matching rows, got %d", len(rows))
	}

	for _, row := range rows {
		m, _ := native.AsMap(row)
		cityVal, _ := m.Get("city")
		cityStr, _ := native.AsString(cityVal)
		if cityStr != "London" {
			t.Errorf("expected city London, got %s", cityStr)
		}
	}
}

// --- list: filter with no matches returns empty list ---

func TestListFilterNoMatches(t *testing.T) {
	csv := "name,age\nAlice,30\nBob,25\n"

	result, err := runNativeWithFiles(t, map[string]string{
		"data.csv": csv,
	}, `read (make Pathon "data.csv") list {name:"Nobody"}`)
	if err != nil {
		t.Fatal(err)
	}

	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

// --- list: filter on multiple fields ---

func TestListFilterMultipleFields(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"

	result, err := runNativeWithFiles(t, map[string]string{
		"data.csv": csv,
	}, `read (make Pathon "data.csv") list {age:"30" city:"London"}`)
	if err != nil {
		t.Fatal(err)
	}

	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	names := make([]string, len(rows))
	for i, row := range rows {
		m, _ := native.AsMap(row)
		v, _ := m.Get("name")
		vs, _ := native.AsString(v)
		names[i] = vs
	}
	got := strings.Join(names, ",")
	if got != "Alice,Charlie" {
		t.Errorf("got names %s, want Alice,Charlie", got)
	}
}
