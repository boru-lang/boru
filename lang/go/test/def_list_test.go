package test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// registerIOWords installs the words moved OUT of core into loadable modules
// (boru:io, boru:struct, boru:net, boru:bin bitwise, boru:type-util's tpartial, …)
// under their bare names into a test registry. The handlers are unchanged by
// the move; this harness helper lets the behaviour suites keep exercising them
// without an explicit import. Production code must `import "boru:<mod>"` and use
// the namespace (IO.read, BinUtil.band, …) — proved by the module-*.tsv specs.
// Idempotent (guards on `read`).
func registerIOWords(reg *native.Registry) {
	if reg.Lookup("read") != nil {
		return
	}
	moved := [][]native.NativeFunc{
		native.IOModuleNativeFuncs(native.IOModuleTypes{StreamKind: native.MintStreamKind(reg), FileType: native.MintFileType(reg), Watcher: native.MintWatcherType(reg), File: native.MintFileHandleType(reg), Lock: native.MintLockType(reg), Mmap: native.MintMmapType(reg)}),
		native.StructModuleNatives,
		native.NetModuleNatives(native.MintFetchTypes(reg)),
		native.BitwiseModuleNatives,
		native.TPartialModuleNatives,
		native.TimeAsyncModuleNatives(native.MintTemporalModuleTypes(reg)),
		native.LogicModuleNatives,
		native.StringModuleNatives,
	}
	for _, slice := range moved {
		for _, n := range slice {
			reg.RegisterNativeFunc(n)
		}
	}
}

func runNativeSteps(t *testing.T, files map[string]string, steps []string) ([]native.Value, error) {
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
	native.Register(reg)
	modules.InstallMathExports(reg)
	// Struct words (merge/walk/clone/…) moved to boru:struct; install the
	// Struct namespace so tests can call StructUtil.merge etc. without wiring
	// the full module resolver.
	if err := modules.InstallStructExports(reg); err != nil {
		t.Fatal(err)
	}
	// I/O words (read/write/stdin/stdout/stderr/printstr/trace) moved out of
	// core into boru:io. The internal behaviour suite exercises the unchanged
	// handlers under their bare names via this harness helper; production
	// requires `import "boru:io"` + IO.read (proved by module-io.tsv).
	registerIOWords(reg)

	eng := native.NewTop(reg)
	var result []native.Value
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			return nil, err
		}
		result, err = eng.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// def foo (read "data.csv")  list foo — lists all rows via forward
func TestDefListAll(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read (make Pathon "data.csv"))`,
		`list foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

// def foo (read "data.csv")  foo list {age:"30" city:"London"} — prefix form with filter
func TestDefListFilterPrefix(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read (make Pathon "data.csv"))`,
		`foo list {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		m, _ := native.AsMap(row)
		v, _ := m.Get("name")
		ns, _ := native.AsString(v)
		names[i] = ns
	}
	got := strings.Join(names, ",")
	if got != "Alice,Charlie" {
		t.Errorf("got %s, want Alice,Charlie", got)
	}
}

// (read "data.csv") list {age:"30" city:"London"} — parens force evaluation
func TestDefListFilterParens(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`(read (make Pathon "data.csv")) list {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		m, _ := native.AsMap(row)
		v, _ := m.Get("name")
		ns, _ := native.AsString(v)
		names[i] = ns
	}
	got := strings.Join(names, ",")
	if got != "Alice,Charlie" {
		t.Errorf("got %s, want Alice,Charlie", got)
	}
}

// def foo (read "data.csv")  (foo) list {age:"30" city:"London"} — parens around def'd word
func TestDefListFilterParensDef(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read (make Pathon "data.csv"))`,
		`(foo) list {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
}

// def foo (read "data.csv")  foo list {age:"30" city:"London"} — parens in def evaluate eagerly
func TestDefParensListFilter(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read (make Pathon "data.csv"))`,
		`foo list {age:"30" city:"London"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(rows))
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		m, _ := native.AsMap(row)
		v, _ := m.Get("name")
		ns, _ := native.AsString(v)
		names[i] = ns
	}
	got := strings.Join(names, ",")
	if got != "Alice,Charlie" {
		t.Errorf("got %s, want Alice,Charlie", got)
	}
}

// def foo (read "data.csv")  list foo — parens in def, list all rows
func TestDefParensListAll(t *testing.T) {
	csv := "name,age,city\nAlice,30,London\nBob,30,Paris\nCharlie,30,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read (make Pathon "data.csv"))`,
		`list foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

// def foo (read "data.csv")  foo create {id:"4" name:"Dave" city:"Berlin"}
func TestDefParensCreate(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n3,Charlie,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read (make Pathon "data.csv"))`,
		`foo create {id:"4" name:"Dave" city:"Berlin"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	m, _ := native.AsMap(rows[3])
	v, _ := m.Get("name")
	vs1, _ := native.AsString(v)
	if vs1 != "Dave" {
		t.Errorf("expected Dave, got %s", vs1)
	}
}

// def foo (read "data.csv")  foo load {id:"2"}
func TestDefParensLoad(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n3,Charlie,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read (make Pathon "data.csv"))`,
		`foo load {id:"2"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := native.AsMap(result[0])
	v, _ := m.Get("name")
	vs2, _ := native.AsString(v)
	if vs2 != "Bob" {
		t.Errorf("expected Bob, got %s", vs2)
	}
}

// def foo (read "data.csv")  foo update {id:"1" city:"Berlin"}
func TestDefParensUpdate(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read (make Pathon "data.csv"))`,
		`foo update {id:"1" city:"Berlin"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	m, _ := native.AsMap(rows[0])
	city, _ := m.Get("city")
	cityS, _ := native.AsString(city)
	if cityS != "Berlin" {
		t.Errorf("expected Berlin, got %s", cityS)
	}
	name, _ := m.Get("name")
	nameS, _ := native.AsString(name)
	if nameS != "Alice" {
		t.Errorf("expected Alice preserved, got %s", nameS)
	}
}

// def foo (read "data.csv")  foo remove {id:"2"}
func TestDefParensRemove(t *testing.T) {
	csv := "id,name,city\n1,Alice,London\n2,Bob,Paris\n3,Charlie,London\n"
	result, err := runNativeSteps(t, map[string]string{"data.csv": csv}, []string{
		`def foo (read (make Pathon "data.csv"))`,
		`foo remove {id:"2"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for _, row := range rows {
		m, _ := native.AsMap(row)
		v, _ := m.Get("name")
		vs3, _ := native.AsString(v)
		if vs3 == "Bob" {
			t.Error("Bob should have been removed")
		}
	}
}
