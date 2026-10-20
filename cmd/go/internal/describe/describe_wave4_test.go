// Wave-4 coverage for the describe subcommand: every dispatch arm of
// Run (index, category, word, dotted export, module path, load
// fallback, unknown), the module-path resolution helpers, and the
// command.Command wrapper methods.
package describe

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestW4CommandMethods(t *testing.T) {
	c := New()
	if c.Name() != "describe" {
		t.Errorf("Name() = %q, want describe", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
	var out bytes.Buffer
	if code := c.Run(nil, nil, &out, nil); code != 0 {
		t.Errorf("Run(nil) = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "boru language reference") {
		t.Errorf("index output missing header: %.120q", out.String())
	}
}

func TestW4IndexListsWordsAndModules(t *testing.T) {
	var out bytes.Buffer
	if code := Run(nil, &out); code != 0 {
		t.Fatalf("Run(nil) = %d, want 0", code)
	}
	s := out.String()
	for _, want := range []string{"Words:", "Modules", "Drill in:", "boru describe <word>"} {
		if !strings.Contains(s, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestW4Category(t *testing.T) {
	var out bytes.Buffer
	if code := Run([]string{"math"}, &out); code != 0 {
		t.Fatalf("Run(math) = %d, want 0", code)
	}
	s := out.String()
	if !strings.Contains(s, "add") {
		t.Errorf("math category missing add: %.200q", s)
	}
	if !strings.Contains(s, "boru describe <word>") {
		t.Errorf("category footer missing: %.200q", s)
	}
}

func TestW4RegisteredWord(t *testing.T) {
	var out bytes.Buffer
	if code := Run([]string{"add"}, &out); code != 0 {
		t.Fatalf("Run(add) = %d, want 0; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "add") {
		t.Errorf("word docs missing add: %.200q", out.String())
	}
}

func TestW4DottedExport(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"ArrayUtil.shape"}, &out)
	if code != 0 {
		t.Fatalf("Run(ArrayUtil.shape) = %d, want 0; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "shape") {
		t.Errorf("dotted export docs missing shape: %.200q", out.String())
	}
}

func TestW4DottedExportUnknownWord(t *testing.T) {
	// Namespace resolves but the word does not: falls through the dotted
	// arm and ends at the not-found message.
	var out bytes.Buffer
	code := Run([]string{"ArrayUtil.zz-no-such-word"}, &out)
	if code != 1 {
		t.Fatalf("Run(ArrayUtil.zz-no-such-word) = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "no description available") {
		t.Errorf("expected not-found message, got %.200q", out.String())
	}
}

func TestW4BareModuleName(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"array-util"}, &out)
	if code != 0 {
		t.Fatalf("Run(array-util) = %d, want 0; out=%s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "boru:array-util") {
		t.Errorf("module render missing boru:array-util: %.200q", s)
	}
	if !strings.Contains(s, "ArrayUtil.shape") {
		t.Errorf("module render missing exported word list: %.300q", s)
	}
}

func TestW4UnknownName(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"zz-definitely-not-a-thing"}, &out)
	if code != 1 {
		t.Fatalf("Run(unknown) = %d, want 1", code)
	}
	s := out.String()
	if !strings.Contains(s, "no description available") {
		t.Errorf("missing not-found line: %.200q", s)
	}
	if !strings.Contains(s, "Run 'boru describe'") {
		t.Errorf("missing hint line: %.200q", s)
	}
}

func TestW4ModulePathWithBoruPrefix(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"boru:array-util"}, &out)
	if code != 0 {
		t.Fatalf("Run(boru:array-util) = %d, want 0; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Load with import") {
		t.Errorf("module render missing import hint: %.300q", out.String())
	}
}

func TestW4ModulePathWord(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"boru:array-util:shape"}, &out)
	if code != 0 {
		t.Fatalf("Run(boru:array-util:shape) = %d, want 0; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "shape") {
		t.Errorf("module word docs missing shape: %.300q", out.String())
	}
}

func TestW4ModulePathWordWithoutPrefix(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"array-util:shape"}, &out)
	if code != 0 {
		t.Fatalf("Run(array-util:shape) = %d, want 0; out=%s", code, out.String())
	}
}

func TestW4ModulePathMissingWord(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"boru:array-util:zz-nope"}, &out)
	if code != 1 {
		t.Fatalf("Run(boru:array-util:zz-nope) = %d, want 1", code)
	}
	s := out.String()
	if !strings.Contains(s, "has no exported word") {
		t.Errorf("missing no-exported-word message: %.300q", s)
	}
	if !strings.Contains(s, "Exported words:") {
		t.Errorf("missing exported-word hint: %.300q", s)
	}
}

func TestW4ModulePathUnknownModule(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"boru:zz-no-such-module"}, &out)
	if code != 1 {
		t.Fatalf("Run(boru:zz-no-such-module) = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "cannot load module") {
		t.Errorf("missing cannot-load message: %.300q", out.String())
	}
}

func TestW4SplitModulePath(t *testing.T) {
	cases := []struct {
		in, mod, word string
	}{
		{"boru:type-util:foo", "boru:type-util", "foo"},
		{"boru:type-util", "boru:type-util", ""},
		{"type-util:foo", "type-util", "foo"},
		{"type-util", "type-util", ""},
		{"boru:m:a:b", "boru:m", "a:b"},
		// A one-letter module is a module, not a Windows drive.
		{"m:a:b", "m", "a:b"},
	}
	if runtime.GOOS == "windows" {
		// The volume's colon is part of the path; the word follows the next.
		cases = append(cases, []struct{ in, mod, word string }{
			{`C:\proj\lib.boru:twice`, `C:\proj\lib.boru`, "twice"},
			{`C:\proj\lib.boru`, `C:\proj\lib.boru`, ""},
		}...)
	}
	for _, c := range cases {
		mod, word := splitModulePath(c.in)
		if mod != c.mod || word != c.word {
			t.Errorf("splitModulePath(%q) = (%q, %q), want (%q, %q)",
				c.in, mod, word, c.mod, c.word)
		}
	}
}

func TestW4IsModule(t *testing.T) {
	if !isModule("array-util") {
		t.Error("isModule(array-util) = false, want true")
	}
	if isModule("zz-nope") {
		t.Error("isModule(zz-nope) = true, want false")
	}
}

func TestW4ExportHelpers(t *testing.T) {
	reg, err := newRegistry()
	if err != nil {
		t.Fatalf("newRegistry: %s", err)
	}
	desc, err := resolveModule(reg, "boru:array-util")
	if err != nil {
		t.Fatalf("resolveModule: %s", err)
	}

	if info := exportInfoFromDesc(desc, "shape"); info == nil {
		t.Error("exportInfoFromDesc(shape) = nil, want info")
	}
	if info := exportInfoFromDesc(desc, "zz-nope"); info != nil {
		t.Errorf("exportInfoFromDesc(zz-nope) = %+v, want nil", info)
	}

	names := exportWordNames(desc)
	if len(names) == 0 {
		t.Fatal("exportWordNames returned nothing")
	}
	found := false
	for _, n := range names {
		if n == "ArrayUtil.shape" {
			found = true
		}
	}
	if !found {
		t.Errorf("exportWordNames missing ArrayUtil.shape: %v", names)
	}
}

func TestW4QualifiedExportInfo(t *testing.T) {
	reg, err := newRegistry()
	if err != nil {
		t.Fatalf("newRegistry: %s", err)
	}
	if info := qualifiedExportInfo(reg, "ArrayUtil.shape"); info == nil {
		t.Error("qualifiedExportInfo(ArrayUtil.shape) = nil, want info")
	}
	// Namespace matches but the word does not.
	if info := qualifiedExportInfo(reg, "ArrayUtil.zz-nope"); info != nil {
		t.Errorf("qualifiedExportInfo(ArrayUtil.zz-nope) = %+v, want nil", info)
	}
	// No namespace matches at all.
	if info := qualifiedExportInfo(reg, "ZzNope.word"); info != nil {
		t.Errorf("qualifiedExportInfo(ZzNope.word) = %+v, want nil", info)
	}
	// Malformed dotted names.
	if info := qualifiedExportInfo(reg, ".leading"); info != nil {
		t.Errorf("qualifiedExportInfo(.leading) = %+v, want nil", info)
	}
	if info := qualifiedExportInfo(reg, "trailing."); info != nil {
		t.Errorf("qualifiedExportInfo(trailing.) = %+v, want nil", info)
	}
}

func TestW4StaticOnlyWord(t *testing.T) {
	// "abs" is documented in the static help registry but is a module
	// word, not a registered word — it renders via the static entry.
	var out bytes.Buffer
	code := Run([]string{"abs"}, &out)
	if code != 0 {
		t.Fatalf("Run(abs) = %d, want 0; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "abs") {
		t.Errorf("static entry missing abs: %.200q", out.String())
	}
}

func TestW4FileModuleFallback(t *testing.T) {
	// A bare name that is not a word, category, or built-in module gets
	// one more chance as a loadable module. A relative file path takes
	// the file-module load path.
	dir := t.TempDir()
	mod := filepath.Join(dir, "w4mod.boru")
	src := "def w4double fn [[n:Integer] [Integer] [(n 2 mul)]]\n\nexport \"W4\" {double:w4double/v}\n"
	if err := os.WriteFile(mod, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	var out bytes.Buffer
	code := Run([]string{"./w4mod.boru"}, &out)
	if code != 0 {
		t.Fatalf("Run(./w4mod.boru) = %d, want 0; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "W4.double") {
		t.Errorf("file module render missing W4.double: %.300q", out.String())
	}
}

func TestW4ResolveModuleFallback(t *testing.T) {
	reg, err := newRegistry()
	if err != nil {
		t.Fatalf("newRegistry: %s", err)
	}
	// Not a built-in: goes through ResolveAnyModule and fails.
	if _, err := resolveModule(reg, "zz-no-such-module"); err == nil {
		t.Error("resolveModule(zz-no-such-module) succeeded, want error")
	}
}
