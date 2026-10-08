// Package specfix is the shared scaffolding for the .tsv spec-suite
// test runners — `eng/go/spec_test.go` (kernel) and
// `lang/go/test/spec_runner_test.go` (production language). Both walk a
// directory of `.tsv` files and, for each non-blank/non-comment row,
// parse the `<input><TAB><expected>[<TAB><note>]` columns, evaluate the
// input through a caller-supplied engine, and compare the result stack
// rendered through `core.Canon` to `<expected>` (with a `ERROR:<wantSubstring>`
// form for expected-error rows).
//
// The caller supplies a Run function that does the parse-and-evaluate
// step. Rendering lives in `core.Canon`, which emits canonical boru source
// — a form that re-parses to the same stack.
package specfix

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// ErrSkipRow is the sentinel a Run / RenderRun returns to SKIP a row
// rather than evaluate it. eng's standalone corpus run uses it for the
// handful of rows that need the content layer (basic) — the Emailon /
// Urlon constructions — which the full harness in test/go/engspec
// still executes with basic installed.
var ErrSkipRow = errors.New("specfix: row skipped by the runner")

// Run executes one spec row's input and returns the resulting stack.
// Returning an error is the row's way of signalling that the input
// errored; a row marked `ERROR:<text>` in the .tsv passes when the
// returned error's message contains `<text>` (empty `<text>` matches
// any error).
type Run func(input string) ([]core.Value, error)

// RunDir runs every `.tsv` file in dir as a subtest named after the
// file's basename (minus the `.tsv` suffix). Each row inside the file
// becomes its own subtest (`L<line>_<input-snippet>`). Fails the parent
// test if dir has no `.tsv` files.
func RunDir(t *testing.T, dir string, run Run) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	ran := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") || !SpecFileSelected(e.Name()) {
			continue
		}
		ran++
		t.Run(strings.TrimSuffix(e.Name(), ".tsv"), func(t *testing.T) {
			RunFile(t, filepath.Join(dir, e.Name()), run)
		})
	}
	if ran == 0 {
		t.Errorf("no .tsv specs found under %s", dir)
	}
}

// SpecFileSelected reports whether a spec file takes part in this run.
// BORU_SPEC_FILES (comma-separated basenames or globs, e.g.
// `callbacks.tsv,fold-*.tsv`) restricts every corpus walk that reads the
// directory through RunDir to the named files, so one family can be
// iterated on in seconds; unset, every file is selected. The langspec
// gates apply the same variable through their own specEntries.
func SpecFileSelected(name string) bool {
	raw := strings.TrimSpace(os.Getenv("BORU_SPEC_FILES"))
	if raw == "" {
		return true
	}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if ok, _ := filepath.Match(p, name); ok || p == name {
			return true
		}
	}
	return false
}

// RunFile runs every data row of a single `.tsv` file against run.
func RunFile(t *testing.T, path string, run Run) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		line := strings.TrimRight(raw, " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			t.Errorf("%s:L%d: malformed row, want at least input<TAB>expected, got %q", path, lineNum, line)
			continue
		}
		input := strings.TrimSpace(parts[0])
		expected := strings.TrimSpace(parts[1])

		name := fmt.Sprintf("L%d_%s", lineNum, sanitiseSpecName(input))
		t.Run(name, func(t *testing.T) {
			out, runErr := run(input)
			if errors.Is(runErr, ErrSkipRow) {
				t.Skip("row needs the content layer — covered by the full harness")
			}

			if strings.HasPrefix(expected, "ERROR:") {
				want := expected[len("ERROR:"):]
				if runErr == nil {
					t.Fatalf("expected error containing %q, got result %v", want, core.Canon(out))
				}
				if want != "" && !strings.Contains(runErr.Error(), want) {
					t.Errorf("error %q does not contain %q", runErr.Error(), want)
				}
				return
			}

			if runErr != nil {
				t.Fatalf("unexpected error: %v", runErr)
			}
			got := core.Canon(out)
			if got != expected {
				t.Errorf("got %q, want %q", got, expected)
			}
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error in %s: %v", path, err)
	}
}

// RenderRun executes one spec row's input and returns an already-rendered
// result string (e.g. the check-mode carrier stack + diagnostics). As with
// Run, a non-nil error signals that the row errored — matched against an
// `ERROR:<text>` expected column.
type RenderRun func(input string) (string, error)

// RunDirRendered is RunDir for runners that render their own result
// string rather than producing an core.Value stack compared via Canon.
func RunDirRendered(t *testing.T, dir string, run RenderRun) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	ran := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		ran++
		t.Run(strings.TrimSuffix(e.Name(), ".tsv"), func(t *testing.T) {
			RunFileRendered(t, filepath.Join(dir, e.Name()), run)
		})
	}
	if ran == 0 {
		t.Errorf("no .tsv specs found under %s", dir)
	}
}

// RunFileRendered runs every data row of a single `.tsv` file against a
// RenderRun, comparing the rendered string to the expected column.
func RunFileRendered(t *testing.T, path string, run RenderRun) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		line := strings.TrimRight(raw, " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			t.Errorf("%s:L%d: malformed row, want at least input<TAB>expected, got %q", path, lineNum, line)
			continue
		}
		input := strings.TrimSpace(parts[0])
		expected := strings.TrimSpace(parts[1])

		name := fmt.Sprintf("L%d_%s", lineNum, sanitiseSpecName(input))
		t.Run(name, func(t *testing.T) {
			got, runErr := run(input)
			if errors.Is(runErr, ErrSkipRow) {
				t.Skip("row needs the content layer — covered by the full harness")
			}

			if strings.HasPrefix(expected, "ERROR:") {
				want := expected[len("ERROR:"):]
				if runErr == nil {
					t.Fatalf("expected error containing %q, got result %q", want, got)
				}
				if want != "" && !strings.Contains(runErr.Error(), want) {
					t.Errorf("error %q does not contain %q", runErr.Error(), want)
				}
				return
			}

			if runErr != nil {
				t.Fatalf("unexpected error: %v", runErr)
			}
			if got != expected {
				t.Errorf("got %q, want %q", got, expected)
			}
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error in %s: %v", path, err)
	}
}

func sanitiseSpecName(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
