package lang

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/policy"
)

// TestNUR079FileModuleImportPolicy pins NUR079's second half. A FILE module
// is imported under the same three checks as a native one — the modules
// scope installed, the `import` op allowed for the module, the module's own
// subscope not install:false — keyed on the ref the import names and
// carrying kind "file", so the restrictive built-in profiles admit source
// modules as a class (a body runs under the importer's policy) while a
// profile can refuse one module, or every file module, by name. A refusal
// is CODED, and catchable by `do`.
func TestNUR079FileModuleImportPolicy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.boru"),
		[]byte("def twice fn [[n:Integer] [Integer] [n mul 2]]\nexport \"Lib\" {twice: twice/v}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const prog = `import "./lib.boru" Lib.twice 21`
	load := func(src string) policy.Policy {
		t.Helper()
		pol, err := policy.LoadAuto(src)
		if err != nil {
			t.Fatalf("policy %s: %v", src, err)
		}
		return pol
	}
	pinned := `{version:1 name:"pin" extends:"read-only" scopes:{modules:{scopes:{"./lib.boru":{install:false}}}}}`
	noFiles := `{version:1 name:"nofile" extends:"read-only" scopes:{modules:{words:{rules:[{deny:["import"] where:{kind:["file"]}}]}}}}`
	for _, c := range []struct {
		name, src string
		pol       policy.Policy
		want      string // a value, or the refusal's code
	}{
		{"no policy", prog, nil, "[42]"},
		{"read-only admits a file module", prog, load("read-only"), "[42]"},
		{"client admits a file module", prog, load("client"), "[42]"},
		{"a subscope refuses one module", `import "./lib.boru" 1`, load(pinned), "capability_not_installed"},
		{"a rule refuses every file module", `import "./lib.boru" 1`, load(noFiles), "permission_denied"},
		{"read-only still refuses boru:net", `import "boru:net" 1`, load("read-only"), "permission_denied"},
		{"the refusal is dispatchable", `do [import "boru:net"] error [dot code]`, load("read-only"), "[permission_denied]"},
		{"a refused module's names are never reached", prog, load(pinned), "capability_not_installed"},
	} {
		a, err := New(Options{Policy: c.pol, BaseDir: dir})
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		out, err := a.Run(c.src)
		if err != nil {
			var be *core.BoruError
			if errors.As(err, &be) {
				got = be.Code
			} else {
				got = fmt.Sprintf("uncoded %v", err)
			}
		} else {
			got = fmt.Sprint(out)
		}
		if got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// TestNUR079CheckReportsRefusedImport: the check pass reports a refused
// top-level import as the error the run raises, under the same code — not
// an opaque module whose names then read as undefined.
func TestNUR079CheckReportsRefusedImport(t *testing.T) {
	pol, err := policy.Load("read-only")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Check(`import "boru:net" 1`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range res.Diagnostics {
		found = found || d.Code == "permission_denied"
	}
	if !found {
		t.Errorf("want a permission_denied diagnostic, got %v", res.Diagnostics)
	}
	// A refusal under `do` is caught at run time, so the check mirrors no
	// error there: the run raises nothing a caller did not trap.
	c, _ := New(Options{Policy: pol})
	if res, err := c.Check(`do [import "boru:net"] error [dot code]`); err != nil {
		t.Fatal(err)
	} else {
		for _, d := range res.Diagnostics {
			if d.Code == "permission_denied" && d.Severity == SeverityError {
				t.Errorf("a caught refusal must not be mirrored as an error, got %v", res.Diagnostics)
			}
		}
	}
	// Negative: an allowed import reports nothing.
	b, _ := New(Options{Policy: pol})
	if res, err := b.Check(`import "boru:math-util" 1`); err != nil || len(res.Diagnostics) != 0 {
		t.Errorf("an admitted import must check clean, got %v / %v", res.Diagnostics, err)
	}
}
