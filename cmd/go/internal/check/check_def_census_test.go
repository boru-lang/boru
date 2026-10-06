package check

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `boru check --def-census` prints the pass's def census after each target's
// diagnostics — one line per finding — and leaves the exit code to the
// check itself (core/go/def_census.go, design/IMMUTABLE-DEF.1.md §5 phase 0).
func TestRunCLIDefCensus(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--def-census", "-e", `def a 1 def a 2 if true [def w 1] [def w 2] w`}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (the census is report-only): %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"1:13  rebind  a  (module)  <- 1:5  value", "1:45  leak-read  w  (module)  <- 1:32  value"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	// Without the flag nothing is printed; a clean program prints nothing
	// with it.
	stdout.Reset()
	RunCLI([]string{"-e", `def a 1 def a 2`}, &stdout, &stderr)
	if strings.Contains(stdout.String(), "rebind") {
		t.Error("the census printed without --def-census")
	}
	stdout.Reset()
	RunCLI([]string{"--def-census", "-e", `def a 1 a`}, &stdout, &stderr)
	if strings.Contains(stdout.String(), "rebind") || strings.Contains(stdout.String(), "(module)") {
		t.Errorf("a clean program has an empty census: %q", stdout.String())
	}
	// A file target prefixes each line with its path; a finding with no
	// standing site (undef) prints none.
	dir := t.TempDir()
	path := filepath.Join(dir, "c.boru")
	if err := os.WriteFile(path, []byte("def a 1\nundef a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if code := RunCLI([]string{"--def-census", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr.String())
	}
	if want := path + ":2:7  undef  a  (module)\n"; !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout lacks %q:\n%s", want, stdout.String())
	}
	// A finding inside a fn body names the fn: its positions are the body's.
	stdout.Reset()
	RunCLI([]string{"--def-census", "-e", `def k fn [[n:Integer] [Integer] [def n 5 n]] k 1`}, &stdout, &stderr)
	if want := "1:38  rebind  n  (frame)  param  in k\n"; !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout lacks %q:\n%s", want, stdout.String())
	}
}
