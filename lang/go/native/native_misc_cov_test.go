package native

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	parser "github.com/boru-lang/boru/parser/go"
)

// Coverage tests for native_misc.go: help/describe, referent, the inline
// module import forms, file/data/bare imports, the export no-op, await
// options parsing, the stack-first read form, and the Timeout/Interval
// format behaviors. Positive and negative shapes are paired throughout.

// runMiscCovWith parses and runs src on a pre-configured registry, returning
// the canonical stack render, whatever was printed to r.Output, and any error.
func runMiscCovWith(t *testing.T, r *Registry, src string) (string, string, error) {
	t.Helper()
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, runErr := NewTop(r).Run(toks)
	return Canon(out), buf.String(), runErr
}

func runMiscCov(t *testing.T, src string) (string, string, error) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return runMiscCovWith(t, r, src)
}

// ---- help / describe --------------------------------------------------------

func TestMiscCovHelpOverview(t *testing.T) {
	_, printed, err := runMiscCov(t, `help`)
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(printed, "boru") {
		t.Errorf("help output should mention boru, got %q", printed)
	}
}

func TestMiscCovDescribeIndex(t *testing.T) {
	_, printed, err := runMiscCov(t, `describe`)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if !strings.Contains(printed, "boru language reference") {
		t.Errorf("describe index missing header, got %q", printed[:min(len(printed), 120)])
	}
}

func TestMiscCovDescribeWord(t *testing.T) {
	_, printed, err := runMiscCov(t, `describe 'add'`)
	if err != nil {
		t.Fatalf("describe 'add': %v", err)
	}
	if !strings.Contains(printed, "add") {
		t.Errorf("describe 'add' output missing the word, got %q", printed)
	}

	// An unknown name reports, it does not error.
	_, printed, err = runMiscCov(t, `describe 'zz-no-such-word-zz'`)
	if err != nil {
		t.Fatalf("describe unknown: %v", err)
	}
	if !strings.Contains(printed, "no description available") {
		t.Errorf("describe unknown should say so, got %q", printed)
	}
}

// describe on a class-bound name renders the schema view (formatTypeSchema).
func TestMiscCovDescribeClassSchema(t *testing.T) {
	_, printed, err := runMiscCov(t, `def Point class {x:Integer y:0} describe 'Point'`)
	if err != nil {
		t.Fatalf("describe class: %v", err)
	}
	for _, frag := range []string{"class", "Fields:", "(required)", "(default)", "make Point"} {
		if !strings.Contains(printed, frag) {
			t.Errorf("class schema missing %q, got %q", frag, printed)
		}
	}

	// The refine chain marks inherited fields and names the parent.
	_, printed, err = runMiscCov(t,
		`def Point class {x:Integer y:0} def Point3 refine Point {z:3} describe 'Point3'`)
	if err != nil {
		t.Fatalf("describe refined class: %v", err)
	}
	for _, frag := range []string{"Refines:", "(inherited)"} {
		if !strings.Contains(printed, frag) {
			t.Errorf("refined schema missing %q, got %q", frag, printed)
		}
	}
}

// describe on a surface-bound name renders the contract view
// (formatSurfaceSchema), including the fn-shape rendering.
func TestMiscCovDescribeSurfaceSchema(t *testing.T) {
	_, printed, err := runMiscCov(t,
		`def Shape surface {area: (fnsig [[Self] [Float]])} describe 'Shape'`)
	if err != nil {
		t.Fatalf("describe surface: %v", err)
	}
	for _, frag := range []string{"surface", "Required operations", "area", "Conformance"} {
		if !strings.Contains(printed, frag) {
			t.Errorf("surface schema missing %q, got %q", frag, printed)
		}
	}
}

// formatSurfaceSchema declines a non-surface value.
func TestMiscCovFormatSurfaceSchemaNonSurface(t *testing.T) {
	if got := formatSurfaceSchema("X", NewInteger(1)); got != "" {
		t.Errorf("formatSurfaceSchema(non-surface) = %q, want empty", got)
	}
}

// ---- referent ----------------------------------------------------------------

func TestMiscCovReferent(t *testing.T) {
	got, _, err := runMiscCov(t, `def xv 5 referent xv/q`)
	if err != nil {
		t.Fatalf("referent bound: %v", err)
	}
	if got != "5" {
		t.Errorf("referent xv/q = %q, want 5", got)
	}

	_, _, err = runMiscCov(t, `referent zz-unbound-name-zz/q`)
	if err == nil || !strings.Contains(err.Error(), "no referent") {
		t.Errorf("referent of unbound atom: err = %v, want 'no referent'", err)
	}
}

func TestMiscCovReferentTypeLiteral(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("panic: %v", rec)
		}
	}()
	if _, herr := referentHandler([]Value{NewTypeLiteral(TAtom)}, nil, nil, r); herr == nil {
		t.Error("referent of a bare Atom literal must error")
	}
}

// ---- module / inline import forms --------------------------------------------

func TestMiscCovModuleValue(t *testing.T) {
	got, _, err := runMiscCov(t, `def m (module [export "A" {x:1}]) typeof m`)
	if err != nil {
		t.Fatalf("module: %v", err)
	}
	if got != "Module" {
		t.Errorf("typeof module = %q, want Module", got)
	}
}

func TestMiscCovImportInline(t *testing.T) {
	got, _, err := runMiscCov(t, `import module [export "Foo" {a:1}] Foo.a`)
	if err != nil || got != "1" {
		t.Fatalf("inline import = %q, %v; want 1", got, err)
	}

	// Renamed inline import (list + module + body): binds the new name only.
	got, _, err = runMiscCov(t, `import [Foo Bar] module [export "Foo" {a:1}] Bar.a`)
	if err != nil || got != "1" {
		t.Fatalf("inline rename import = %q, %v; want 1", got, err)
	}
	_, _, err = runMiscCov(t, `import [Foo Bar] module [export "Foo" {a:1}] Foo.a`)
	if err == nil || !strings.Contains(err.Error(), "undefined_word") {
		t.Errorf("renamed import must unbind the original: %v", err)
	}

	// Pair-list form renames several exports at once.
	got, _, err = runMiscCov(t,
		`import [[A X] [B Y]] module [export "A" {a:1} export "B" {b:2}] X.a add Y.b`)
	if err != nil || got != "3" {
		t.Fatalf("pair-list rename = %q, %v; want 3", got, err)
	}

	// Single-rename form: atom + module + body.
	got, _, err = runMiscCov(t, `import Bar/q module [export "Foo" {a:1}] Bar.a`)
	if err != nil || got != "1" {
		t.Fatalf("single-rename inline import = %q, %v; want 1", got, err)
	}
}

func TestMiscCovImportInlineErrors(t *testing.T) {
	cases := []struct{ src, frag string }{
		// The inline form only accepts the literal word `module`.
		{`import bogus [export "A" {x:1}]`, "unknown inline form"},
		{`import [A B] bogus [export "A" {x:1}]`, "unknown inline form"},
		{`import Bar/q bogus [export "A" {x:1}]`, "unknown inline form"},
		// A rename must name an actual export.
		{`import [Nope Bar] module [export "Foo" {a:1}]`, "not found in module"},
		// Single rename needs exactly one export.
		{`import Bar/q module [export "A" {x:1} export "B" {y:2}]`, "exactly one export"},
		{`import Bar/q module [def x 1]`, "no exports"},
	}
	for _, tc := range cases {
		_, _, err := runMiscCov(t, tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.frag) {
			t.Errorf("%s: err = %v, want fragment %q", tc.src, err, tc.frag)
		}
	}
}

// Type literals in the module/import slots must error, never panic.
func TestMiscCovModuleImportTypeLiteralNoPanic(t *testing.T) {
	srcs := []string{
		`module List`,
		`import List module [export "A" {x:1}]`,
		`import module List`,
		`import [A B] module List`,
		`import Bar/q module List`,
	}
	for _, src := range srcs {
		t.Run(src, func(t *testing.T) {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("panic on %q: %v", src, rec)
				}
			}()
			r, err := DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			toks, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, _ = NewTop(r).Run(toks)
		})
	}
}

// ---- export (top-level no-op) -------------------------------------------------

func TestMiscCovExportNoopTopLevel(t *testing.T) {
	got, _, err := runMiscCov(t, `export "X" {a:1}`)
	if err != nil {
		t.Fatalf("top-level export must be a no-op, got %v", err)
	}
	if got != "" {
		t.Errorf("top-level export must produce no value, got %q", got)
	}
}

// In check mode the no-op records exported names as uses.
func TestMiscCovExportNoopCheckModeRecordsUses(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A real fn value (FnDefInfo payload) for the reference-export branch.
	toks, err := parser.Parse(`def fcov fn [[x:Integer] [Integer] [x]] fcov/v`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil || len(out) != 1 {
		t.Fatalf("fn setup failed: %v %v", out, err)
	}
	fnVal := out[0]

	done := r.Check.Begin()
	defer done()

	om := NewOrderedMap()
	om.Set("f", fnVal)
	om.Set("w", NewWord("some-word"))
	om.Set("a", NewAtom("some-atom"))
	om.Set("s", NewString("some-name"))
	if _, herr := exportNoopHandler([]Value{NewString("M"), NewMap(om)}, nil, nil, r); herr != nil {
		t.Fatalf("exportNoopHandler: %v", herr)
	}

	// A non-concrete export map is quietly ignored.
	if _, herr := exportNoopHandler([]Value{NewString("M"), NewTypeLiteral(TMap)}, nil, nil, r); herr != nil {
		t.Fatalf("exportNoopHandler(type literal): %v", herr)
	}
}

// ---- file / data / bare / native imports ---------------------------------------

// miscCovImportReg builds a registry with an in-memory FS holding a module
// file and a data file, and the real parser wired for file imports.
func miscCovImportReg(t *testing.T) (*Registry, *capabilities.MemFileOps) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := capabilities.NewMem()
	mem.Files["mod.boru"] = []byte(`export "FM" {x: 7}`)
	mem.Files["d.json"] = []byte(`{"a": 1}`)
	mem.Files["/proj/.boru/barem/index.boru"] = []byte(`export "BM" {y: 2}`)
	SetHostFileOps(r, mem)
	r.SetParseFunc(parser.Parse)
	return r, mem
}

func TestMiscCovImportFile(t *testing.T) {
	r, _ := miscCovImportReg(t)
	got, _, err := runMiscCovWith(t, r, `import "./mod.boru" FM.x`)
	if err != nil || got != "7" {
		t.Fatalf("file import = %q, %v; want 7", got, err)
	}

	// Renamed file import.
	r2, _ := miscCovImportReg(t)
	got, _, err = runMiscCovWith(t, r2, `import [FM N2] "./mod.boru" N2.x`)
	if err != nil || got != "7" {
		t.Fatalf("renamed file import = %q, %v; want 7", got, err)
	}

	// A missing file is an error, not a silent no-op.
	r3, _ := miscCovImportReg(t)
	_, _, err = runMiscCovWith(t, r3, `import "./no-such-file.boru"`)
	if err == nil {
		t.Error("import of a missing file must error")
	}
}

func TestMiscCovImportDataFile(t *testing.T) {
	r, _ := miscCovImportReg(t)
	got, _, err := runMiscCovWith(t, r, `import "./d.json"`)
	if err != nil {
		t.Fatalf("data import: %v", err)
	}
	if got != "{a:1}" {
		t.Errorf("data import = %q, want {a:1}", got)
	}

	// Rename is meaningless for a data file.
	r2, _ := miscCovImportReg(t)
	_, _, err = runMiscCovWith(t, r2, `import [A B] "./d.json"`)
	if err == nil || !strings.Contains(err.Error(), "rename not supported") {
		t.Errorf("data rename err = %v, want 'rename not supported'", err)
	}
}

func TestMiscCovImportBareModule(t *testing.T) {
	r, _ := miscCovImportReg(t)
	r.BaseDir = "/proj"
	got, _, err := runMiscCovWith(t, r, `import "barem" BM.y`)
	if err != nil || got != "2" {
		t.Fatalf("bare import = %q, %v; want 2", got, err)
	}

	r2, _ := miscCovImportReg(t)
	r2.BaseDir = "/proj"
	got, _, err = runMiscCovWith(t, r2, `import [BM Q] "barem" Q.y`)
	if err != nil || got != "2" {
		t.Fatalf("bare rename import = %q, %v; want 2", got, err)
	}

	// An unknown bare module reports the search failure.
	r3, _ := miscCovImportReg(t)
	_, _, err = runMiscCovWith(t, r3, `import "zz-no-such-module-zz"`)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("bare import err = %v, want 'not found'", err)
	}
	r4, _ := miscCovImportReg(t)
	_, _, err = runMiscCovWith(t, r4, `import [A B] "zz-no-such-module-zz"`)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("bare rename import err = %v, want 'not found'", err)
	}
}

// Check mode degrades imports to opaque carriers instead of hard errors.
func TestMiscCovImportCheckMode(t *testing.T) {
	r, _ := miscCovImportReg(t)
	done := r.Check.Begin()
	defer done()

	// A stripped (empty) path yields a Module carrier.
	out, err := importFileHandler([]Value{NewString("")}, nil, nil, r)
	if err != nil || len(out) != 1 || !out[0].Parent.Equal(TModuleInst) {
		t.Fatalf("check import of stripped path = %v, %v; want one Module carrier", out, err)
	}
	if _, err := importFileRenameHandler([]Value{NewList(nil), NewString("")}, nil, nil, r); err != nil {
		t.Fatalf("check rename import of stripped path: %v", err)
	}

	// A loadable file binds its exports for cross-module resolution.
	out, err = importFileHandler([]Value{NewString("./mod.boru")}, nil, nil, r)
	if err != nil || out != nil {
		t.Fatalf("check import of real file = %v, %v; want nil, nil", out, err)
	}
	if !r.Defs.Has("FM") {
		t.Error("check-mode import must bind the export namespace")
	}

	// A data file is skipped (no exports to bind).
	if out, err = importFileHandler([]Value{NewString("./d.json")}, nil, nil, r); err != nil || out != nil {
		t.Fatalf("check import of data file = %v, %v; want nil, nil", out, err)
	}

	// A missing file degrades to a carrier rather than erroring.
	out, err = importFileHandler([]Value{NewString("./no-such.boru")}, nil, nil, r)
	if err != nil || len(out) != 1 || !out[0].Parent.Equal(TModuleInst) {
		t.Fatalf("check import of missing file = %v, %v; want carrier", out, err)
	}

	// So does an unresolvable bare module.
	out, err = importFileHandler([]Value{NewString("zz-no-such-module-zz")}, nil, nil, r)
	if err != nil || len(out) != 1 || !out[0].Parent.Equal(TModuleInst) {
		t.Fatalf("check import of missing bare module = %v, %v; want carrier", out, err)
	}

	// A native module reference with no resolver installed (the bare native
	// package has none — lang wires it) degrades to a carrier too.
	out, err = importFileHandler([]Value{NewString("boru:math-util")}, nil, nil, r)
	if err != nil || len(out) != 1 || !out[0].Parent.Equal(TModuleInst) {
		t.Fatalf("check import of native module = %v, %v; want carrier", out, err)
	}
}

// A native "boru:" import routes through the registry's module resolver; the
// bare native package has none installed, so wiring a fake one exercises both
// the resolve+install path and the resolver-missing error.
func TestMiscCovImportNativeModule(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	om := NewOrderedMap()
	om.Set("x", NewInteger(7))
	r.Modules.Resolver = func(name string, _ *Registry) (ModuleDesc, error) {
		if name != "fakemod" {
			t.Fatalf("resolver got %q, want fakemod", name)
		}
		return ModuleDesc{
			ID:      "fakemod",
			Ref:     "boru:fakemod",
			Kind:    "native",
			Exports: map[string]*OrderedMap{"NM": om},
		}, nil
	}
	got, _, rerr := runMiscCovWith(t, r, `import "boru:fakemod" NM.x`)
	if rerr != nil || got != "7" {
		t.Fatalf("native import = %q, %v; want 7", got, rerr)
	}
	// A second import of the same module takes the already-loaded path.
	got, _, rerr = runMiscCovWith(t, r, `import "boru:fakemod" NM.x`)
	if rerr != nil || got != "7" {
		t.Fatalf("re-import = %q, %v; want 7", got, rerr)
	}

	// Without a resolver the reference is a clean error.
	r2, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	_, _, rerr = runMiscCovWith(t, r2, `import "boru:math-util"`)
	if rerr == nil || !strings.Contains(rerr.Error(), "resolver not configured") {
		t.Errorf("resolver-less native import err = %v, want 'resolver not configured'", rerr)
	}
	// An empty native module name is rejected before resolution.
	if _, herr := importFileHandler([]Value{NewString("boru:")}, nil, nil, r2); herr == nil ||
		!strings.Contains(herr.Error(), "empty native module name") {
		t.Errorf("empty native name err = %v, want 'empty native module name'", herr)
	}
}

// ---- await options parsing -------------------------------------------------

func TestMiscCovAwaitOpts(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	empty := NewList([]Value{})

	// Options value with a String mode.
	fields := NewOrderedMap()
	fields.Set("mode", NewString("all"))
	out, herr := awaitWithOptsHandler([]Value{NewOptionsType(fields), empty}, nil, nil, r)
	if herr != nil || len(out) != 1 {
		t.Fatalf("await opts(String mode) = %v, %v", out, herr)
	}

	// Plain map with an Atom mode.
	om := NewOrderedMap()
	om.Set("mode", NewAtom("first"))
	if _, herr = awaitWithOptsHandler([]Value{NewMap(om), empty}, nil, nil, r); herr != nil {
		t.Fatalf("await opts(map Atom mode): %v", herr)
	}

	// Plain map with a String mode.
	om2 := NewOrderedMap()
	om2.Set("mode", NewString("any"))
	if _, herr = awaitWithOptsHandler([]Value{NewMap(om2), empty}, nil, nil, r); herr != nil {
		t.Fatalf("await opts(map String mode): %v", herr)
	}

	// A map without a mode defaults to "all".
	if _, herr = awaitWithOptsHandler([]Value{NewMap(NewOrderedMap()), empty}, nil, nil, r); herr != nil {
		t.Fatalf("await opts(no mode): %v", herr)
	}

	// An unknown mode with work to do is an error.
	om3 := NewOrderedMap()
	om3.Set("mode", NewString("bogus"))
	one := NewList([]Value{NewInteger(1)})
	if _, herr = awaitWithOptsHandler([]Value{NewMap(om3), one}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "unknown mode") {
		t.Errorf("await bogus mode err = %v, want 'unknown mode'", herr)
	}

	// A type-literal parallels list is rejected.
	if _, herr = doAwait(r, "all", NewTypeLiteral(TList)); herr == nil ||
		!strings.Contains(herr.Error(), "concrete list") {
		t.Errorf("doAwait(type literal) err = %v, want 'concrete list'", herr)
	}
}

// ---- stack-first read form ----------------------------------------------------

func TestMiscCovReadOptsRev(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := capabilities.NewMem()
	mem.Files["f.txt"] = []byte("hello")
	SetHostFileOps(r, mem)
	registerIOWords(r)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	out, rerr := NewTop(r).Run([]Value{pathV("f.txt"), NewMap(opts), NewWord("read")})
	if rerr != nil {
		t.Fatalf("stack-first read: %v", rerr)
	}
	if len(out) != 1 {
		t.Fatalf("stack-first read produced %d values", len(out))
	}
	s, serr := AsString(out[0])
	if serr != nil || s != "hello" {
		t.Errorf("stack-first read = %v, want 'hello'", out[0])
	}
}

// ---- writable-format gate -----------------------------------------------------

func TestMiscCovCheckWritableFormat(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Implicit or unknown formats pass the gate.
	if err := checkWritableFormat(r, "ini", false); err != nil {
		t.Errorf("implicit format must pass: %v", err)
	}
	if err := checkWritableFormat(r, "zz-not-a-format", true); err != nil {
		t.Errorf("unknown format must pass (doWrite's concern): %v", err)
	}
	// An explicit parser-backed (read-only) format is rejected.
	rejected := false
	for name, f := range HostFormats(r) {
		if ro, ok := f.(ReadOnlyFormat); ok && ro.ReadOnly() {
			if err := checkWritableFormat(r, name, true); err == nil ||
				!strings.Contains(err.Error(), "read-only") {
				t.Errorf("read-only format %q: err = %v, want 'read-only'", name, err)
			}
			rejected = true
			break
		}
	}
	if !rejected {
		t.Skip("no read-only format registered in the default host formats")
	}
}

// ---- Timeout / Interval behaviors ----------------------------------------------

func TestMiscCovTimerBehaviors(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tt := MintTemporalModuleTypes(r)
	to := tt.NewTimeout(&TimeoutInfo{ID: "t1", Ms: 5})
	iv := tt.NewInterval(&IntervalInfo{ID: "i1", Ms: 7})

	tb := timeoutFormatBehavior{}
	ib := intervalFormatBehavior{}

	if got := tb.Format(to); got != "Timeout(t1,5ms)" {
		t.Errorf("Timeout format = %q", got)
	}
	if got := tb.Format(NewInteger(1)); got != "Timeout(nil)" {
		t.Errorf("Timeout format fallback = %q", got)
	}
	if got := ib.Format(iv); got != "Interval(i1,7ms)" {
		t.Errorf("Interval format = %q", got)
	}
	if got := ib.Format(NewInteger(1)); got != "Interval(nil)" {
		t.Errorf("Interval format fallback = %q", got)
	}

	// Equal delegates to the kernel default: identity holds, cross-kind fails.
	if !tb.Equal(to, to) {
		t.Error("Timeout must equal itself")
	}
	if tb.Equal(to, NewInteger(1)) {
		t.Error("Timeout must not equal an Integer")
	}
	if !ib.Equal(iv, iv) {
		t.Error("Interval must equal itself")
	}
	if ib.Equal(iv, to) {
		t.Error("Interval must not equal a Timeout")
	}
	if !tb.Match(to, tt.Timeout) {
		t.Error("Timeout must match its own type")
	}
	if !ib.Match(iv, tt.Interval) {
		t.Error("Interval must match its own type")
	}

	// ToMap / ToList conversions, plus the empty fallbacks.
	tm, cerr := tb.ToMap(to)
	if cerr != nil {
		t.Fatalf("Timeout ToMap: %v", cerr)
	}
	m, _ := AsMap(tm)
	if idv, ok := m.Get("id"); !ok || mustCovStr(t, idv) != "t1" {
		t.Errorf("Timeout ToMap id = %v", tm)
	}
	tl, cerr := tb.ToList(to)
	if cerr != nil {
		t.Fatalf("Timeout ToList: %v", cerr)
	}
	lst, _ := AsList(tl)
	if lst.Len() != 2 {
		t.Errorf("Timeout ToList = %v", tl)
	}
	im, cerr := ib.ToMap(iv)
	if cerr != nil {
		t.Fatalf("Interval ToMap: %v", cerr)
	}
	m2, _ := AsMap(im)
	if msv, ok := m2.Get("ms"); !ok || mustCovInt(t, msv) != 7 {
		t.Errorf("Interval ToMap ms = %v", im)
	}
	il, cerr := ib.ToList(iv)
	if cerr != nil {
		t.Fatalf("Interval ToList: %v", cerr)
	}
	lst2, _ := AsList(il)
	if lst2.Len() != 2 {
		t.Errorf("Interval ToList = %v", il)
	}

	// Non-payload inputs fall back to empty containers, never panic.
	if tm, cerr = tb.ToMap(NewInteger(1)); cerr != nil {
		t.Errorf("Timeout ToMap fallback: %v", cerr)
	} else if m3, _ := AsMap(tm); m3.Len() != 0 {
		t.Errorf("Timeout ToMap fallback = %v", tm)
	}
	if tl, cerr = tb.ToList(NewInteger(1)); cerr != nil {
		t.Errorf("Timeout ToList fallback: %v", cerr)
	} else if l3, _ := AsList(tl); l3.Len() != 0 {
		t.Errorf("Timeout ToList fallback = %v", tl)
	}
	if im, cerr = ib.ToMap(NewInteger(1)); cerr != nil {
		t.Errorf("Interval ToMap fallback: %v", cerr)
	} else if m4, _ := AsMap(im); m4.Len() != 0 {
		t.Errorf("Interval ToMap fallback = %v", im)
	}
	if il, cerr = ib.ToList(NewInteger(1)); cerr != nil {
		t.Errorf("Interval ToList fallback: %v", cerr)
	} else if l4, _ := AsList(il); l4.Len() != 0 {
		t.Errorf("Interval ToList fallback = %v", il)
	}
}

func mustCovStr(t *testing.T, v Value) string {
	t.Helper()
	s, err := AsString(v)
	if err != nil {
		t.Fatalf("not a string: %v", v)
	}
	return s
}

func mustCovInt(t *testing.T, v Value) int64 {
	t.Helper()
	n, err := AsInteger(v)
	if err != nil {
		t.Fatalf("not an integer: %v", v)
	}
	return n
}

// ---- second wave: handler branches dispatch cannot deliver ---------------------

// A quoted atom captured via `quote` carries a referent snapshot.
func TestMiscCovReferentQuoteSnapshot(t *testing.T) {
	got, _, err := runMiscCov(t, `def xv 5 referent (quote xv)`)
	if err != nil {
		t.Fatalf("referent (quote xv): %v", err)
	}
	if got != "5" {
		t.Errorf("referent (quote xv) = %q, want 5", got)
	}
}

// A module body that fails to run surfaces the body error.
func TestMiscCovModuleBodyError(t *testing.T) {
	_, _, err := runMiscCov(t, `module [zz-undefined-word-xyz]`)
	if err == nil || !strings.Contains(err.Error(), "module:") {
		t.Errorf("module body error = %v, want 'module:' prefix", err)
	}
	_, _, err = runMiscCov(t, `import module [zz-undefined-word-xyz]`)
	if err == nil || !strings.Contains(err.Error(), "import module:") {
		t.Errorf("inline import body error = %v, want 'import module:' prefix", err)
	}
	_, _, err = runMiscCov(t, `import [A B] module [zz-undefined-word-xyz]`)
	if err == nil || !strings.Contains(err.Error(), "import module:") {
		t.Errorf("inline rename body error = %v, want 'import module:' prefix", err)
	}
	_, _, err = runMiscCov(t, `import N/q module [zz-undefined-word-xyz]`)
	if err == nil || !strings.Contains(err.Error(), "import module:") {
		t.Errorf("inline single-rename body error = %v, want 'import module:' prefix", err)
	}
}

// Type-literal argument guards, called directly (the dispatcher would not
// route a bare List literal into these sigs).
func TestMiscCovHandlerTypeLiteralGuards(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	litL := NewTypeLiteral(TList)
	modAtom := NewAtom("module")
	emptyList := NewList([]Value{})

	if _, herr := moduleHandler([]Value{litL}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "concrete list") {
		t.Errorf("moduleHandler(literal) err = %v", herr)
	}

	om := NewOrderedMap()
	om.Set("x", NewInteger(1))
	desc := ModuleDesc{ID: "m1", Kind: "inline", Exports: map[string]*OrderedMap{"A": om}}
	mod := NewModuleInstance(desc)
	if _, herr := importRenameHandler([]Value{litL, mod}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "concrete list") {
		t.Errorf("importRenameHandler(literal list) err = %v", herr)
	}

	if _, herr := importInlineHandler([]Value{modAtom, litL}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "concrete list") {
		t.Errorf("importInlineHandler(literal body) err = %v", herr)
	}
	if _, herr := importInlineRenameHandler([]Value{litL, modAtom, emptyList}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "concrete list") {
		t.Errorf("importInlineRenameHandler(literal rename) err = %v", herr)
	}
	if _, herr := importInlineRenameHandler([]Value{emptyList, modAtom, litL}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "concrete list") {
		t.Errorf("importInlineRenameHandler(literal body) err = %v", herr)
	}
	if _, herr := importInlineSingleRenameHandler([]Value{NewAtom("N"), modAtom, litL}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "concrete list") {
		t.Errorf("importInlineSingleRenameHandler(literal body) err = %v", herr)
	}
}

// write with an explicitly read-only format is declined; an unknown format
// surfaces doWrite's error.
func TestMiscCovWriteFormatGates(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)
	registerIOWords(r)

	var roName string
	for name, f := range HostFormats(r) {
		if ro, ok := f.(ReadOnlyFormat); ok && ro.ReadOnly() {
			roName = name
			break
		}
	}
	if roName != "" {
		opts := NewOrderedMap()
		opts.Set("fmt", NewString(roName))
		_, herr := writeOptsHandler([]Value{NewString("o.dat"), NewString("data"), NewMap(opts)}, nil, nil, r)
		if herr == nil || !strings.Contains(herr.Error(), "read-only") {
			t.Errorf("writeOptsHandler(read-only fmt %q) err = %v", roName, herr)
		}
		_, herr = writeAnyOptsHandler([]Value{NewString("o.dat"), NewInteger(5), NewMap(opts)}, nil, nil, r)
		if herr == nil || !strings.Contains(herr.Error(), "read-only") {
			t.Errorf("writeAnyOptsHandler(read-only fmt %q) err = %v", roName, herr)
		}
	}

	// The happy path through writeAnyOptsHandler upgrades text to jsonic.
	opts2 := NewOrderedMap()
	out, herr := writeAnyOptsHandler([]Value{pathV("o.j"), NewInteger(5), NewMap(opts2)}, nil, nil, r)
	if herr != nil || len(out) != 1 {
		t.Fatalf("writeAnyOptsHandler happy path = %v, %v", out, herr)
	}
	if _, ok := mem.Files["o.j"]; !ok {
		t.Error("writeAnyOptsHandler must write the file")
	}
}
