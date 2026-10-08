// Wave-4 coverage for the specgen generator helpers: the runtime
// failure whose engine error carries no [boru/<code>] tag (the bare
// "ERROR:" class, which classifyFrontier maps to the generic "error"
// detail), and the front-coded reader's remaining edge rows.
package main

import (
	"os"
	"path/filepath"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// bareErrorProgram is a program that checks clean, compiles, and then
// fails at run with an error carrying NO [boru/<code>] tag. Both tests
// below exist to pin that arm, so they need a specimen that genuinely
// has no code — and the specimen keeps moving, because each round of
// error-code work removes one:
//
//   - top-level `1 0 div` stopped qualifying when the checker's arith
//     mirror began flagging it statically (design/legacy/CHECKER-COMPLETION.0.ignore),
//     which classifies classCheck;
//   - an out-of-bounds `getr` stopped qualifying when that condition was
//     given the `index_out_of_range` code its own check-mode mirror had
//     always emitted (design/verse-report-defects-investigation.0.md §E).
//
// `convert` of a non-numeric String is the current one. When it too gets
// a code, these tests will fail with a clear message rather than silently
// stop testing anything — replace the specimen, or, if no code-less
// runtime error is left anywhere, delete both tests and the classifier
// arm they cover.
const bareErrorProgram = `"x" convert Integer`

// TestW4ClassifyFrontierBareErrorDetail pins the classifyFrontier arm
// for a runtime error whose message has no [boru/<code>] prefix: the
// program checks clean and compiles, yet fails at run — errorClass
// yields the generic "ERROR:", and the classifier substitutes the
// "error" detail.
func TestW4ClassifyFrontierBareErrorDetail(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	a.SetClock(specClock)

	cl, detail, compiled := classifyFrontier(a, bareErrorProgram)
	if cl != classRuntime {
		t.Fatalf("class = %d, want classRuntime — %s no longer fails at RUN; "+
			"pick a new code-less specimen", cl, bareErrorProgram)
	}
	if detail != "error" {
		t.Errorf("detail = %q, want the generic \"error\" — %s now carries a "+
			"code, so it can no longer stand in for an UNCODED failure; pick a "+
			"new specimen (see bareErrorProgram)", detail, bareErrorProgram)
	}
	if !compiled {
		t.Errorf("expected %s to compile", bareErrorProgram)
	}
}

// TestW4ErrorClassBareMessage pins errorClass on an engine error with
// no bracketed code at all.
func TestW4ErrorClassBareMessage(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, runErr := a.RunInterp(bareErrorProgram); runErr == nil {
		t.Fatalf("%s ran clean, want a runtime error", bareErrorProgram)
	} else if got := errorClass(runErr); got != "ERROR:" {
		t.Errorf("errorClass = %q, want the bare ERROR: sentinel — %s now "+
			"carries a code; pick a new specimen (see bareErrorProgram)",
			got, bareErrorProgram)
	}
}

// TestW4ForEachDataRowFrontCodedNoExtra covers a front-coded row with
// no third field: extra comes back empty.
func TestW4ForEachDataRowFrontCodedNoExtra(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fc.tsv")
	content := "# header\n" + frontCodeMarker + "\n" +
		"0\tabc\n" +
		"2\txy\textra1\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	var inputs, extras []string
	err := forEachDataRow(path, func(input, expected, note string, line int) {
		inputs = append(inputs, input)
		extras = append(extras, expected)
	})
	if err != nil {
		t.Fatalf("forEachDataRow: %v", err)
	}
	if len(inputs) != 2 || inputs[0] != "abc" || inputs[1] != "abxy" {
		t.Errorf("inputs = %v, want [abc abxy]", inputs)
	}
	if extras[0] != "" || extras[1] != "extra1" {
		t.Errorf("extras = %v, want [\"\" extra1]", extras)
	}
}
