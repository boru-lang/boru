package test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// `boru check` is documented as "type-check without running", but it runs
// imported module bodies for real. Two deliberate decisions compose to
// produce that:
//
//   - every `import` signature is registered RunInCheck, because the
//     checker cannot type `Mod.v` without the module's actual exports;
//   - check mode is deliberately NOT propagated into the module
//     sub-registry, because carrier-stripping destroys the concrete string
//     literals a body uses as export names and map keys.
//
// So a module body's effects — prints, file writes, network calls —
// execute with full ambient authority during a type check. And because
// file modules are never cached, a default run (which pre-flight checks
// first) executes each body twice per importer.
//
// Neither decision is wrong on its own. The first block of tests below pins
// the info diagnostic that made the behaviour visible; the second pins the
// actual repair — the THIRD MODE (core.CheckState.ModelEffects), which carries
// the handler-substitution half of check mode into the module sub-registry
// while leaving values concrete, so the body still produces real export names
// and its effects go nowhere.
//
// The mode's line is WRITES. That is what keeps it safe rather than merely
// suppressive: reads are untouched, so no module body that loads today can
// start failing under check — which is precisely what sank the cheaper
// "deny-all policy around the check-pass body" option, since denial raises
// and the raise aborts the import and loses the exports check needs.

func countCode(res lang.CheckResult, code string) int {
	n := 0
	for _, d := range res.Diagnostics {
		if d.Code == code {
			n++
		}
	}
	return n
}

func TestCheckReportsModuleBodyExecution(t *testing.T) {
	res := checkDiag(t, `import module [ export "N" {w: 2} ] end
N.w`)
	if got := countCode(res, "module_body_executed_in_check"); got != 1 {
		t.Fatalf("one module body ran, want 1 diagnostic, got %d: %v",
			got, diagCodes(res))
	}
	d, _ := diagWithCode(res, "module_body_executed_in_check")
	if d.Severity != lang.SeverityInfo {
		t.Errorf("severity = %s, want info — the program is not wrong, the "+
			"behaviour was merely invisible", d.Severity)
	}
	// The detail has to say WHY, or a reader will file it as a bug in
	// `import` rather than recognise the two decisions behind it.
	for _, frag := range []string{"import", "check"} {
		if !strings.Contains(d.Detail, frag) {
			t.Errorf("detail %q should mention %q", d.Detail, frag)
		}
	}
}

// TestCheckModuleExecutionCountIsNotDeduped pins the count, which is the
// actual finding: effects multiply by IMPORTERS, not by a fixed 2. A
// deduped diagnostic would report "a module body ran" and hide that it ran
// three times.
func TestCheckModuleExecutionCountIsNotDeduped(t *testing.T) {
	res := checkDiag(t, `import module [ export "A" {v: 1} ] end
import module [ export "B" {v: 2} ] end
import module [ export "C" {v: 3} ] end
A.v`)
	if got := countCode(res, "module_body_executed_in_check"); got != 3 {
		t.Fatalf("three module bodies ran, want 3 diagnostics, got %d: %v",
			got, diagCodes(res))
	}
}

// TestCheckWithoutModulesIsSilent is the negative half: the diagnostic must
// cost nothing for the overwhelming majority of programs, which import no
// module at all. An advisory that fires on everything is noise, and noise
// is how a real finding gets ignored.
func TestCheckWithoutModulesIsSilent(t *testing.T) {
	for _, src := range []string{
		`1 add 2`,
		`def f fn [[x:Integer][Integer][x mul 2]] f 5`,
		`do [1 div 0] error [drop 9]`,
	} {
		res := checkDiag(t, src)
		if got := countCode(res, "module_body_executed_in_check"); got != 0 {
			t.Errorf("%q imports nothing but drew %d module-execution "+
				"diagnostic(s): %v", src, got, diagCodes(res))
		}
	}
}

// ---- the third mode: effects modelled, values concrete ----

// writeModule writes src to <dir>/<name> and returns its absolute path, ready
// to drop into an `import "<path>" end` line. Absolute so resolution does not
// depend on the test process's working directory.
func writeModule(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// checkAndRun runs src through Check and then Run on two fresh engines,
// returning what each wrote to its output writer. Both are required to
// succeed: the whole point of modelling (rather than denying) effects is that
// a body which loads at runtime keeps loading under check.
func checkAndRun(t *testing.T, src string) (checkOut, runOut string) {
	t.Helper()
	var cbuf, rbuf bytes.Buffer

	ca, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ca.SetOutput(&cbuf)
	if _, err := ca.Check(src); err != nil {
		t.Fatalf("check: %v", err)
	}

	ra, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ra.SetOutput(&rbuf)
	if _, err := runReference(t, ra, src); err != nil {
		t.Fatalf("run: %v", err)
	}
	return cbuf.String(), rbuf.String()
}

// TestCheckModelsModuleBodyFileWrites is the headline: `boru check` no longer
// touches the filesystem through an imported module body. The same program run
// for real still writes, so the modelling is scoped to the check pass and not
// a blanket disable.
func TestCheckModelsModuleBodyFileWrites(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "written.txt")
	mod := writeModule(t, dir, "m.boru", `
import "boru:io" end
IO.write (make Pathon '`+target+`') 'from the module body' drop
export "M" {v: 1}
`)
	src := `import '` + mod + `' end
M.v`

	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := a.Check(src); err != nil {
		t.Fatalf("check: %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Errorf("check created %s — a type check must not write to the "+
			"filesystem, and the module body's write is the one path that did",
			target)
	}

	b, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := runReference(t, b, src); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("the RUN must still write %s (modelling is scoped to the "+
			"check pass, not a disable): %v", target, err)
	}
}

// TestCheckModelledWriteIsReadableInSameBody is why the substitution is an
// OVERLAY and not a black hole. A body that writes a file and reads it back —
// a staging idiom — must still see its own bytes, or modelling would break
// bodies that work at runtime. Nothing reaches the real filesystem either
// way.
func TestCheckModelledWriteIsReadableInSameBody(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "roundtrip.txt")
	mod := writeModule(t, dir, "m.boru", `
import "boru:io" end
IO.write (make Pathon '`+target+`') 'staged' drop
def back (IO.read (make Pathon '`+target+`'))
export "M" {v: back}
`)
	src := `import '` + mod + `' end
M.v`

	res := checkDiag(t, src)
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			t.Errorf("check reported %s: %s — the modelled write must still be "+
				"readable inside the body", d.Code, d.Detail)
		}
	}
	if _, err := os.Stat(target); err == nil {
		t.Errorf("%s escaped to the real filesystem", target)
	}
	// And the same program must run.
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := runReference(t, a, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "[staged]"; fmt.Sprint(got) != want {
		t.Errorf("run = %s, want %s", fmt.Sprint(got), want)
	}
}

// TestCheckModelledBodyStillReadsRealFiles pins the constraint that decided
// the design. A module body commonly reads a config or resource at load; a
// mode that hid or denied reads would make check report an error for a program
// that runs fine. Reads fall through to the real filesystem.
func TestCheckModelledBodyStillReadsRealFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := writeModule(t, dir, "cfg.txt", "real-config")
	mod := writeModule(t, dir, "m.boru", `
import "boru:io" end
def cfg (IO.read (make Pathon '`+cfg+`'))
export "M" {v: cfg}
`)
	src := `import '` + mod + `' end
M.v`

	res := checkDiag(t, src)
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			t.Errorf("check reported %s: %s — a module body's READ must still "+
				"resolve against the real filesystem", d.Code, d.Detail)
		}
	}
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := runReference(t, a, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "[real-config]"; fmt.Sprint(got) != want {
		t.Errorf("run = %s, want %s", fmt.Sprint(got), want)
	}
}

// TestCheckModelsModuleBodyOutput covers the other substituted class. A module
// that prints at load printed TWICE under a default run — once for the
// pre-flight check, once for the run. Now the check pass draws nothing.
func TestCheckModelsModuleBodyOutput(t *testing.T) {
	checkOut, runOut := checkAndRun(t, `import module [ print 'loading' export "M" {v: 1} ] end
M.v`)
	if strings.Contains(checkOut, "loading") {
		t.Errorf("check wrote %q — a module body's print is an effect, and "+
			"`check` is documented as type-check WITHOUT running", checkOut)
	}
	if !strings.Contains(runOut, "loading") {
		t.Errorf("run wrote %q, want it to contain \"loading\" — the run must "+
			"still print", runOut)
	}
}

// TestModelledEffectsPropagateThroughNestedImports: the mode is derived from
// the PARENT's state, so a module imported from inside a module body inherits
// it. Without the transitive half, one level of nesting would restore the
// original defect — and nesting is the common shape (a facade module).
func TestModelledEffectsPropagateThroughNestedImports(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested.txt")
	inner := writeModule(t, dir, "inner.boru", `
import "boru:io" end
IO.write (make Pathon '`+target+`') 'inner wrote this' drop
export "Inner" {v: 1}
`)
	outer := writeModule(t, dir, "outer.boru", `
import '`+inner+`' end
export "Outer" {v: Inner.v}
`)
	src := `import '` + outer + `' end
Outer.v`

	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := a.Check(src); err != nil {
		t.Fatalf("check: %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Errorf("check created %s through a NESTED import — ModelEffects did "+
			"not propagate past the first module body", target)
	}
}

// TestModellingDoesNotLeakOutsideCheck is the negative half at the other
// boundary: a top-level program's writes are real. A registry that models
// effects because it once served a check pass would silently swallow a user's
// output — so the flag must be per-sub-registry, not sticky.
func TestModellingDoesNotLeakOutsideCheck(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "toplevel.txt")
	src := `import "boru:io" end
IO.write (make Pathon '` + target + `') 'top level' drop`

	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := runReference(t, a, src); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("a top-level write must reach the real filesystem: %v", err)
	}
	if string(data) != "top level" {
		t.Errorf("wrote %q, want %q", data, "top level")
	}
}

// TestDefaultRunEmitsModuleBodyEffectExactlyOnce is the sharpest test of the
// whole repair, and it guards the subtlety that nearly broke it.
//
// A default run is check-then-run on ONE registry — exactly what the CLI does
// (check.PreflightColor then buildrt.EvalReport). §D measured the module body's
// effect happening TWICE there: once per pass, multiplied by importers.
//
// Suppressing the check-pass copy is only half the story. On the compiled path
// the COMPILE pass is where a module body runs at all — `import` is RunInCheck,
// the exports are baked, and the VM never re-imports — so a mode that modelled
// effects for every check-mode pass would not deduplicate the effect, it would
// DELETE it: measured, this program went from one line of output to none.
// core.CheckState.Compiling is what separates a pure check pass (nobody's
// execution — modelled) from a compile pass (the execution — real).
//
// So: exactly one. Not two, and not zero.
func TestDefaultRunEmitsModuleBodyEffectExactlyOnce(t *testing.T) {
	const src = `import module [ print 'loading' export "M" {v: 1} ] end
M.v`
	var buf bytes.Buffer
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	a.SetOutput(&buf)
	if _, err := a.Check(src); err != nil { // the CLI's quiet pre-flight
		t.Fatalf("check: %v", err)
	}
	if _, err := a.Run(src); err != nil { // then the compiled run
		t.Fatalf("run: %v", err)
	}
	switch n := strings.Count(buf.String(), "loading"); n {
	case 1: // correct
	case 0:
		t.Errorf("the module body's effect vanished from a default run: the "+
			"COMPILE pass is the only pass that runs the body on the compiled "+
			"path, so modelling it there deletes the effect rather than "+
			"deduplicating it — gate modelling on !Check.Compiling. Output: %q",
			buf.String())
	case 2:
		t.Errorf("the module body's effect happened twice — once per pass, the "+
			"original §D doubling. The pure check pass must model it. Output: %q",
			buf.String())
	default:
		t.Errorf("the module body's effect happened %d times: %q", n, buf.String())
	}
}
