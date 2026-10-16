package native

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	parser "github.com/boru-lang/boru/parser/go"
)

// Wave-4 coverage for native_query.go (the boru:query DSL handlers),
// native_module_module.go (file / native / data-file module loading,
// export installation, renames), describe.go (module describe paths),
// and native_unpack.go (module unpack).

// ---------------------------------------------------------------------------
// Query DSL (native_query.go)
// ---------------------------------------------------------------------------

// w4QueryReg builds a registry with the boru:query natives registered
// bare (the same set modules.BuildQueryModule wraps) plus the fixture
// tables from query_cov_test.go in the context store.
func w4QueryReg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	for _, n := range QueryNatives {
		r.RegisterNativeFunc(n)
	}
	r.ContextSet("people", NewValueRaw(TList, covTableNameAge().td))
	r.ContextSet("pets", NewValueRaw(TList, covPetsTable()))
	r.ContextSet("notable", NewInteger(42))
	return r
}

// w4Materialize forces a lazy query Value into concrete TableData.
func w4Materialize(t *testing.T, v Value) TableData {
	t.Helper()
	mp, ok := DataOf(&v).(MaterializerPayload)
	if !ok {
		t.Fatalf("expected a lazy query, got %T", DataOf(&v))
	}
	td, err := mp.M.Materialize()
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	return td
}

// w4QueryRun runs src and returns the single residual value.
func w4QueryRun(t *testing.T, r *Registry, src string) Value {
	t.Helper()
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	if len(out) != 1 {
		t.Fatalf("%q: expected 1 residual, got %d", src, len(out))
	}
	return out[0]
}

func TestW4QuerySelectFromPipeline(t *testing.T) {
	r := w4QueryReg(t)
	// SELECT * — the empty column list.
	td := w4Materialize(t, w4QueryRun(t, r, `select [] from people`))
	if len(td.Rows) != 4 {
		t.Errorf("select * from people = %d rows, want 4", len(td.Rows))
	}
	// Projection + limit + offset + distinct through the language surface.
	td = w4Materialize(t, w4QueryRun(t, r, `select [name] from people limit 2`))
	if len(td.Rows) != 2 || td.Record.Fields.Len() != 1 {
		t.Errorf("projection+limit = %d rows, %d cols", len(td.Rows), td.Record.Fields.Len())
	}
	td = w4Materialize(t, w4QueryRun(t, r, `select [age] from people distinct`))
	if len(td.Rows) != 3 { // ages 30, 20, 25, 20 → 3 distinct
		t.Errorf("distinct ages = %d rows, want 3", len(td.Rows))
	}
	td = w4Materialize(t, w4QueryRun(t, r, `select [name] from people offset 1`))
	if len(td.Rows) != 3 {
		t.Errorf("offset 1 = %d rows, want 3", len(td.Rows))
	}
	// order attaches through the language surface too (collision-free name).
	td = w4Materialize(t, w4QueryRun(t, r, `select [] from people order [age desc] limit 1`))
	if len(td.Rows) != 1 {
		t.Fatalf("order+limit = %d rows", len(td.Rows))
	}
	m, err := AsMap(td.Rows[0])
	if err != nil || m == nil {
		t.Fatal("row is not a map")
	}
	// Integer columns round-trip SQLite as numerics; accept 30 / 30.0.
	if v, _ := m.Get("age"); !strings.HasPrefix(FormatForPrint(v), "30") {
		t.Errorf("top age = %v, want 30", v)
	}
}

func TestW4QueryFromErrors(t *testing.T) {
	r := w4QueryReg(t)
	for _, c := range []struct{ src, substr string }{
		{`select [] from missing`, "unknown table"},
		{`select [] from notable`, "not a table"},
	} {
		toks, err := parser.Parse(c.src)
		if err != nil {
			t.Fatal(err)
		}
		_, err = NewTop(r).Run(toks)
		if err == nil || !strings.Contains(err.Error(), c.substr) {
			t.Errorf("%q: error %v, want %q", c.src, err, c.substr)
		}
	}
}

// w4Seed builds a fresh SELECT * builder value via the entry handler.
func w4Seed(t *testing.T, r *Registry) Value {
	t.Helper()
	out, err := selectColsHandler([]Value{NewList(nil)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("selectColsHandler: %v", err)
	}
	return out[0]
}

// w4From attaches the named context table to a builder.
func w4From(t *testing.T, r *Registry, name string, b Value) Value {
	t.Helper()
	out, err := fromNameHandler([]Value{NewAtom(name), b}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("fromNameHandler(%s): %v", name, err)
	}
	return out[0]
}

func TestW4QueryClauseHandlers(t *testing.T) {
	// The clause words whose names collide with core words (where /
	// group / join and friends) are exercised through their handlers
	// directly — same args, sig order.
	r := w4QueryReg(t)
	b := w4From(t, r, "people", w4Seed(t, r))

	// where
	cond := covCond(NewAtom("age"), NewAtom("gt"), NewInteger(21))
	out, err := queryWhereHandler([]Value{cond, b}, nil, nil, r)
	if err != nil {
		t.Fatalf("where: %v", err)
	}
	if rows := len(w4Materialize(t, out[0]).Rows); rows != 2 { // 30, 25
		t.Errorf("where age>21 = %d rows, want 2", rows)
	}
	if _, err = queryWhereHandler([]Value{cond, NewInteger(9)}, nil, nil, r); err == nil {
		t.Error("where: non-builder should error")
	}
	if _, err = queryWhereHandler([]Value{covCond(NewAtom("age"), NewAtom("gt")), b}, nil, nil, r); err == nil {
		t.Error("where: dangling condition should error")
	}

	// group + having
	out, err = queryGroupHandler([]Value{covCond(NewAtom("age")), b}, nil, nil, r)
	if err != nil {
		t.Fatalf("group: %v", err)
	}
	grouped := out[0]
	if rows := len(w4Materialize(t, grouped).Rows); rows != 3 {
		t.Errorf("group by age = %d rows, want 3", rows)
	}
	out, err = queryHavingHandler([]Value{covCond(NewAtom("age"), NewAtom("gt"), NewInteger(21)), grouped}, nil, nil, r)
	if err != nil {
		t.Fatalf("having: %v", err)
	}
	if rows := len(w4Materialize(t, out[0]).Rows); rows != 2 {
		t.Errorf("group+having = %d rows, want 2", rows)
	}
	if _, err = queryGroupHandler([]Value{covCond(NewAtom("age")), NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("group: non-builder should error")
	}
	if _, err = queryGroupHandler([]Value{covCond(NewInteger(1)), b}, nil, nil, r); err == nil {
		t.Error("group: non-column clause should error")
	}
	if _, err = queryHavingHandler([]Value{covCond(NewAtom("x"), NewAtom("gt")), b}, nil, nil, r); err == nil {
		t.Error("having: dangling condition should error")
	}
	if _, err = queryHavingHandler([]Value{covCond(), NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("having: non-builder should error")
	}

	// order / limit / offset error arms
	if _, err = queryOrderHandler([]Value{covCond(), b}, nil, nil, r); err == nil {
		t.Error("order: empty column list should error")
	}
	if _, err = queryOrderHandler([]Value{covCond(NewAtom("age")), NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("order: non-builder should error")
	}
	if _, err = queryLimitHandler([]Value{NewTypeLiteral(TInteger), b}, nil, nil, r); err == nil {
		t.Error("limit: type-literal count should error")
	}
	if _, err = queryLimitHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil {
		t.Error("limit: non-builder should error")
	}
	if _, err = queryOffsetHandler([]Value{NewTypeLiteral(TInteger), b}, nil, nil, r); err == nil {
		t.Error("offset: type-literal count should error")
	}
	if _, err = queryOffsetHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil {
		t.Error("offset: non-builder should error")
	}
	if _, err = queryDistinctHandler([]Value{NewInteger(3)}, nil, nil, r); err == nil {
		t.Error("distinct: non-builder should error")
	}
}

func TestW4QueryJoinHandlers(t *testing.T) {
	r := w4QueryReg(t)
	b := w4From(t, r, "people", w4Seed(t, r))
	joinFn := queryJoinNative("join", "JOIN")
	h := joinFn.Signatures[0].DispatchHandler()

	out, err := h([]Value{NewAtom("pets"), b}, nil, nil, r)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	joined := out[0]
	// on: attach the join predicate, then materialize the join.
	onCond := covCond(NewAtom("name"), NewAtom("eq"), NewAtom("owner"))
	out, err = queryOnHandler([]Value{onCond, joined}, nil, nil, r)
	if err != nil {
		t.Fatalf("on: %v", err)
	}
	if rows := len(w4Materialize(t, out[0]).Rows); rows != 3 { // alice×2 + bob×1
		t.Errorf("join..on = %d rows, want 3", rows)
	}
	// using: shared-column join spelling — a self-join on the shared
	// `name` column so the SQL actually runs.
	selfJoined, err := h([]Value{NewAtom("people"), b}, nil, nil, r)
	if err != nil {
		t.Fatalf("self join: %v", err)
	}
	out, err = queryUsingHandler([]Value{covCond(NewAtom("name")), selfJoined[0]}, nil, nil, r)
	if err != nil {
		t.Fatalf("using: %v", err)
	}
	if rows := len(w4Materialize(t, out[0]).Rows); rows != 4 { // names are unique
		t.Errorf("self join using(name) = %d rows, want 4", rows)
	}

	// Negative arms.
	if _, err = h([]Value{NewTypeLiteral(TAtom), b}, nil, nil, r); err == nil {
		t.Error("join: non-atom table should error")
	}
	if _, err = h([]Value{NewAtom("pets"), NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("join: non-builder should error")
	}
	if _, err = queryOnHandler([]Value{onCond, b}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "no preceding join") {
		t.Errorf("on without join: %v", err)
	}
	if _, err = queryOnHandler([]Value{covCond(NewAtom("a")), joined}, nil, nil, r); err == nil {
		t.Error("on: malformed condition should error")
	}
	if _, err = queryOnHandler([]Value{onCond, NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("on: non-builder should error")
	}
	if _, err = queryUsingHandler([]Value{covCond(NewAtom("name")), b}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "no preceding join") {
		t.Errorf("using without join: %v", err)
	}
	if _, err = queryUsingHandler([]Value{covCond(NewInteger(1)), joined}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected column name") {
		t.Errorf("using non-column: %v", err)
	}
	if _, err = queryUsingHandler([]Value{NewInteger(1), joined}, nil, nil, r); err == nil {
		t.Error("using: non-list should error")
	}
	if _, err = queryUsingHandler([]Value{covCond(NewAtom("name")), NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("using: non-builder should error")
	}
}

func TestW4QuerySetOps(t *testing.T) {
	r := w4QueryReg(t)
	left := w4From(t, r, "people", w4Seed(t, r))
	right := w4From(t, r, "people", w4Seed(t, r))
	union := querySetOpNative("union", "UNION")
	h := union.Signatures[0].DispatchHandler()

	out, err := h([]Value{right, left}, nil, nil, r)
	if err != nil {
		t.Fatalf("union: %v", err)
	}
	if rows := len(w4Materialize(t, out[0]).Rows); rows != 4 { // UNION dedupes
		t.Errorf("people UNION people = %d rows, want 4", rows)
	}
	all := querySetOpNative("unionall", "UNION ALL").Signatures[0].DispatchHandler()
	out, err = all([]Value{right, left}, nil, nil, r)
	if err != nil {
		t.Fatalf("unionall: %v", err)
	}
	if rows := len(w4Materialize(t, out[0]).Rows); rows != 8 {
		t.Errorf("people UNION ALL people = %d rows, want 8", rows)
	}
	if _, err = h([]Value{NewInteger(1), left}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "right operand") {
		t.Errorf("union bad right: %v", err)
	}
	if _, err = h([]Value{right, NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "left operand") {
		t.Errorf("union bad left: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Modules (native_module_module.go) + describe (describe.go) + unpack
// ---------------------------------------------------------------------------

// w4ModEnv builds a registry with an in-memory filesystem holding a
// small module tree, plus a fake native-module resolver.
func w4ModEnv(t *testing.T) (*Registry, *capabilities.MemFileOps) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	mem := capabilities.NewMem()
	mem.Cwd = "/proj"
	SetHostFileOps(r, mem)

	write := func(path, content string) {
		t.Helper()
		if err := mem.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A plain file module with a fn export, a type export, and a value.
	write("/mods/util/index.boru", `
def twice fn [[x:Integer] [Integer] [x mul 2]]
def Color refine Integer
export "Util" {twice: twice/v Color: Color msg: "hello"}
`)
	// A module whose entry point is declared in .boru/boru.json, with a
	// data resource.
	write("/mods/alt/.boru/boru.json", `{"main": "main.boru", "resource": {"cfg": "cfg.json"}}`)
	write("/mods/alt/main.boru", `export "Alt" {v: 41}`)
	write("/mods/alt/cfg.json", `{"a": 1}`)
	// A module with a malformed resource declaration.
	write("/mods/badres/.boru/boru.json", `{"main": "main.boru", "resource": {"cfg": 7}}`)
	write("/mods/badres/main.boru", `export "BadRes" {v: 1}`)
	// A bare module resolvable by the .boru search walk.
	write("/proj/.boru/mymod/index.boru", `export "My" {v: 5}`)
	// A data file.
	write("/data/d.json", `{"a": 1}`)

	w4InstallFakeResolver(t, r)
	return r, mem
}

// w4InstallFakeResolver wires a native-module resolver that serves two
// synthetic modules and rejects everything else.
func w4InstallFakeResolver(t *testing.T, r *Registry) {
	t.Helper()
	r.Modules.Resolver = func(name string, reg *Registry) (ModuleDesc, error) {
		var src string
		switch name {
		case "fake":
			src = `def fnx fn [[x:Integer] [Integer] [x add 1]]
export "Fk" {v: 7 fnx: fnx/v}`
		case "type-util":
			src = `def tp fn [[x:Any] [Any] [x]]
export "TypeUtil" {tpartial: tp/v}`
		default:
			return ModuleDesc{}, fmt.Errorf("no such native module %q", name)
		}
		toks, err := parser.Parse(src)
		if err != nil {
			return ModuleDesc{}, err
		}
		desc, err := RunModuleBody(reg, toks)
		if err != nil {
			return ModuleDesc{}, err
		}
		desc.Ref = "boru:" + name
		desc.Kind = "native"
		return desc, nil
	}
}

func TestW4FileModuleImport(t *testing.T) {
	r, _ := w4ModEnv(t)
	// Explicit path.
	w4LogWant(t, r, `import "/mods/util/index.boru" ; Util.twice 5`, `10`)
	w4LogWant(t, r, `Util.msg`, `'hello'`)
	// Extension-less path resolves index.boru (no boru.json).
	w4LogWant(t, r, `import "/mods/util" ; Util.twice 3`, `6`)
	// boru.json main resolution + data resource export.
	w4LogWant(t, r, `import "/mods/alt" ; Alt.v`, `41`)
	w4LogWant(t, r, `resource.cfg`, `{a:1}`)
	// Bare module name via the .boru search walk.
	w4LogWant(t, r, `import "mymod" ; My.v`, `5`)
	// Data file import lands the value on the stack.
	w4LogWant(t, r, `import "/data/d.json"`, `{a:1}`)
	// Missing file / missing bare module.
	w4LogErr(t, r, `import "/mods/nothere.boru"`, "import")
	w4LogErr(t, r, `import "nomod"`, "not found")
	// Malformed resource declaration.
	w4LogErr(t, r, `import "/mods/badres"`, "must be a string filename")
}

func TestW4FileModuleRenameImports(t *testing.T) {
	r, _ := w4ModEnv(t)
	// Single rename pair [From To].
	w4LogWant(t, r, `import [Util MyU] "/mods/util/index.boru" ; MyU.twice 4`, `8`)
	// Multiple rename pairs [[From To] …].
	w4LogWant(t, r, `import [[Util U2]] "/mods/util/index.boru" ; U2.msg`, `'hello'`)
	// Errors: unknown export name, malformed pair shapes.
	w4LogErr(t, r, `import [Nope New] "/mods/util/index.boru"`, "not found")
	w4LogErr(t, r, `import [[Nope New]] "/mods/util/index.boru"`, "not found")
	w4LogErr(t, r, `import [OnlyOne] "/mods/util/index.boru"`, "2 elements")
	w4LogErr(t, r, `import [[Util U3 Extra]] "/mods/util/index.boru"`, "2 elements")
}

func TestW4NativeModuleImport(t *testing.T) {
	r, _ := w4ModEnv(t)
	w4LogWant(t, r, `import "boru:fake" ; Fk.v`, `7`)
	w4LogWant(t, r, `Fk.fnx 2`, `3`)
	// Re-import of a loaded module is a quiet no-op…
	w4LogWant(t, r, `import "boru:fake" ; Fk.v`, `7`)
	// …and re-binds a namespace that was undef'd (ensureExportsBound).
	w4LogWant(t, r, `undef Fk ; import "boru:fake" ; Fk.v`, `7`)
	// Unknown module and empty name.
	w4LogErr(t, r, `import "boru:zzz"`, "no such native module")
	w4LogErr(t, r, `import "boru:"`, "empty native module name")
	// A registry with no resolver at all.
	r2, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	w4LogErr(t, r2, `import "boru:math-util"`, "resolver not configured")
}

func TestW4ResolveAnyModule(t *testing.T) {
	r, _ := w4ModEnv(t)
	if _, err := ResolveAnyModule(r, "boru:fake"); err != nil {
		t.Errorf("ResolveAnyModule(boru:fake): %v", err)
	}
	if _, err := ResolveAnyModule(r, "boru:"); err == nil {
		t.Error("empty native name should error")
	}
	if _, err := ResolveAnyModule(r, "/data/d.json"); err == nil ||
		!strings.Contains(err.Error(), "data file") {
		t.Errorf("data file ref: %v", err)
	}
	if _, err := ResolveAnyModule(r, "/mods/util/index.boru"); err != nil {
		t.Errorf("file ref: %v", err)
	}
	if _, err := ResolveAnyModule(r, "mymod"); err != nil {
		t.Errorf("bare ref: %v", err)
	}
	if _, err := ResolveAnyModule(r, "nomod"); err == nil {
		t.Error("unknown bare ref should error")
	}
	r2, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveAnyModule(r2, "boru:x"); err == nil ||
		!strings.Contains(err.Error(), "not configured") {
		t.Errorf("no-resolver ref: %v", err)
	}
}

func TestW4LoadDataFileDirect(t *testing.T) {
	r, _ := w4ModEnv(t)
	if _, err := loadDataFile(r, "/data/missing.json"); err == nil {
		t.Error("missing data file should error")
	}
	if _, err := loadDataFile(r, "/data/x.zzz"); err == nil ||
		!strings.Contains(err.Error(), "unknown format") {
		t.Errorf("unknown format: %v", err)
	}
	out, err := loadDataFile(r, "/data/d.json")
	if err != nil || len(out) != 1 {
		t.Fatalf("loadDataFile: %v", err)
	}
}

func TestW4DescribeModules(t *testing.T) {
	r, _ := w4ModEnv(t)
	run := func(name string) string {
		var buf bytes.Buffer
		DescribeName(r, &buf, name)
		return buf.String()
	}

	// A file module: bare path (fallthrough) and colon-form word lookup.
	if out := run("/mods/util/index.boru"); !strings.Contains(out, "Util.twice") ||
		!strings.Contains(out, "Words (call as <export>.<word>):") {
		t.Errorf("describe file module:\n%s", out)
	}
	if out := run("/mods/util/index.boru:twice"); !strings.Contains(out, "Util.twice — ") &&
		!strings.Contains(out, "Util.twice") {
		t.Errorf("describe module word:\n%s", out)
	}
	if out := run("/mods/util/index.boru:nope"); !strings.Contains(out, `no exported word "nope"`) ||
		!strings.Contains(out, "Exported words:") {
		t.Errorf("describe missing module word:\n%s", out)
	}

	// Native module through the resolver, with and without the boru: prefix.
	if out := run("boru:fake"); !strings.Contains(out, "Fk.v") {
		t.Errorf("describe boru:fake:\n%s", out)
	}
	// A *built-in-catalog* name routes through the canonical boru: id even
	// when spelled bare, and shows the catalog summary line.
	if out := run("type-util:tpartial"); !strings.Contains(out, "TypeUtil.tpartial") {
		t.Errorf("describe type-util:tpartial:\n%s", out)
	}
	if out := run("type-util"); !strings.Contains(out, "boru:type-util — ") {
		t.Errorf("describe bare builtin module name:\n%s", out)
	}
	// Unresolvable module reference.
	if out := run("boru:zzz"); !strings.Contains(out, "cannot load module") {
		t.Errorf("describe unknown module:\n%s", out)
	}
	// Unknown plain name falls to the terminal hint.
	if out := run("w4-no-such-name"); !strings.Contains(out, "no description available") {
		t.Errorf("describe unknown name:\n%s", out)
	}
	// An imported namespace's dotted export renders dynamic help.
	if _, err := w3Run(t, r, `import "boru:fake"`); err != nil {
		t.Fatal(err)
	}
	if out := run("Fk.fnx"); !strings.Contains(out, "Fk.fnx") {
		t.Errorf("describe imported dotted word:\n%s", out)
	}
}

func TestW4WriteModuleDescBuiltinSummary(t *testing.T) {
	// The summary line renders when the ref names a catalogued built-in.
	desc := ModuleDesc{Exports: map[string]*OrderedMap{}}
	em := NewOrderedMap()
	em.Set("info", NewInteger(1))
	desc.Exports["Log"] = em
	var buf bytes.Buffer
	writeModuleDesc(&buf, "boru:log", desc)
	out := buf.String()
	if !strings.Contains(out, "boru:log — ") || !strings.Contains(out, "Log.info") {
		t.Errorf("writeModuleDesc builtin:\n%s", out)
	}
	// exportInfoFromDesc skips non-function exports and nil maps.
	desc.Exports["Nil"] = nil
	if info := exportInfoFromDesc(desc, "info"); info != nil {
		t.Error("a non-fn export should not produce FuncInfo")
	}
	names := exportWordNames(desc)
	if len(names) != 1 || names[0] != "Log.info" {
		t.Errorf("exportWordNames = %v", names)
	}
}

func TestW4UnpackModule(t *testing.T) {
	r, _ := w4ModEnv(t)
	// Whole-module unpack binds every lowercase export word bare.
	w4LogWant(t, r, `unpack "boru:fake" ; fnx 9`, `10`)
	w4LogWant(t, r, `unpack "boru:fake" ; v`, `7`)
	// Named-export unpack.
	w4LogWant(t, r, `unpack Fk "boru:fake" ; fnx 1`, `2`)
	// Negatives: unknown export, unknown module, empty name, no resolver.
	w4LogErr(t, r, `unpack Nope "boru:fake"`, "not found")
	w4LogErr(t, r, `unpack "boru:zzz"`, "no such native module")
	w4LogErr(t, r, `unpack "boru:"`, "empty module name")
	r2, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	w4LogErr(t, r2, `unpack "boru:anything"`, "resolver not configured")
}
