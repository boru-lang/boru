package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/test/go/vary"
)

// TestVarySweepDivergedReport — the diverged-classification file + the
// MISCOMPILES note are drivable only through the varySweep seam: a healthy
// build has no reachable divergence (the do-unit registry-replay class is
// fixed), and this arm must stay exercised so a future real divergence is
// reported, not dropped.
func TestVarySweepDivergedReport(t *testing.T) {
	dir := t.TempDir()
	seedDir := filepath.Join(dir, "seeds")
	outDir := filepath.Join(dir, "out")
	for _, d := range []string{seedDir, outDir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(seedDir, "s.tsv"), []byte("1 add 2\t3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := varySweep
	t.Cleanup(func() { varySweep = prev })
	varySweep = func(seeds []vary.Seed, report func(done, total int)) []vary.Variant {
		report(1, 1)
		sd := seeds[0]
		return []vary.Variant{
			{Seed: sd, Transform: "seed", Src: sd.Input, Res: vary.Result{Outcome: vary.Pass}},
			{Seed: sd, Transform: "do-body", Src: "do [" + sd.Input + "]",
				Res: vary.Result{Outcome: vary.Diverged, Detail: "value divergence: compiled [9] vs interp [3]"}},
		}
	}
	if got := exitCode(t, func() { runVarySweep(seedDir, outDir, 1) }); got != -1 {
		t.Fatalf("seam-driven sweep exited %d, want normal return", got)
	}
	div, err := os.ReadFile(filepath.Join(outDir, "vary-diverged.tsv"))
	if err != nil {
		t.Fatalf("vary-diverged.tsv: %v", err)
	}
	if !strings.Contains(string(div), "value divergence") {
		t.Errorf("vary-diverged.tsv missing the divergence detail:\n%s", div)
	}
}

// TestVaryUsageExits — -vary without -vary-out exits 2.
func TestVaryUsageExits(t *testing.T) {
	if got := runMain(t, "-vary"); got != 2 {
		t.Errorf("-vary without -vary-out exited %d, want 2", got)
	}
}

// TestVarySweepIOFailures — a missing seed dir and an uncreatable output
// directory both exit 1.
func TestVarySweepIOFailures(t *testing.T) {
	dir := t.TempDir()
	if got := exitCode(t, func() { runVarySweep(filepath.Join(dir, "absent"), dir, 1) }); got != 1 {
		t.Errorf("missing seed dir exited %d, want 1", got)
	}

	seedDir := filepath.Join(dir, "seeds")
	if err := os.Mkdir(seedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "s.tsv"), []byte("1 add 2\t3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	badOut := filepath.Join(dir, "no-such-dir")
	if got := exitCode(t, func() { runVarySweep(seedDir, badOut, 1) }); got != 1 {
		t.Errorf("bad -vary-out exited %d, want 1", got)
	}
}

// TestVarySweepEndToEnd — a tiny corpus (one passing seed, one declining
// seed, one interp-rejected seed) classified through the real pipeline via
// the main() dispatch, producing the expected classification files.
func TestVarySweepEndToEnd(t *testing.T) {
	dir := t.TempDir()
	seedDir := filepath.Join(dir, "seeds")
	outDir := filepath.Join(dir, "out")
	for _, d := range []string{seedDir, outDir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	corpus := "1 add 2\t3\n" + // passing base: fans out into every transform
		// declined base (a def-PROMOTED do-catch read — the result leaves the
		// stack for a frame slot, so neither the mark window nor the paren
		// apply can reproduce it): skipped, no variants. (`for 3 [1 2]`
		// graduated when net drivers landed; the bare do-catch region
		// `do [(zf 5) 2] error [dot code]` graduated when the mark-window
		// island landed — neither can serve as the declining seed anymore.)
		"def zf fn [[x:Any] [Any] [raise bad_input 'no']]  def msg (do [(zf 5) 2] error [dot code])  msg\tbad_input/q\n" +
		// A passing base whose FOR-BODY variant declines (a typed def
		// re-embedded in a loop body — the conditional-body rollback keeps
		// this a wrapped-context compile failure; the DO-body wrap compiles natively
		// since do-def leak fidelity landed 2026-07-14).
		"def Big Integer 15 is Big\ttrue\n" +
		"# comment\nbroken zz\tERROR:undefined_word\n" // error row: not a seed
	if err := os.WriteFile(filepath.Join(seedDir, "s.tsv"), []byte(corpus), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := runMain(t, "-vary", "-seed-dir", seedDir, "-vary-out", outDir); got != -1 {
		t.Fatalf("vary sweep exited %d, want normal return", got)
	}

	pass, err := os.ReadFile(filepath.Join(outDir, "vary-pass.tsv"))
	if err != nil {
		t.Fatalf("vary-pass.tsv: %v", err)
	}
	if !strings.Contains(string(pass), "1 add 2") {
		t.Errorf("vary-pass.tsv has no variant of the passing seed:\n%s", pass)
	}
	ref, err := os.ReadFile(filepath.Join(outDir, "vary-declined.tsv"))
	if err != nil {
		t.Fatalf("vary-declined.tsv (the registry-replay variant must decline): %v", err)
	}
	if !strings.Contains(string(ref), "for 2 [def Big Integer 15 is Big]") {
		t.Errorf("vary-declined.tsv missing the wrapped typed-def compile failure:\n%s", ref)
	}
	if _, err := os.Stat(filepath.Join(outDir, "vary-diverged.tsv")); !os.IsNotExist(err) {
		t.Errorf("vary-diverged.tsv exists — a healthy build must produce no divergence (stat err=%v)", err)
	}
	// The declining seed contributes NO variant rows (its base is skipped) —
	// no row of any output file may reference s.tsv:2.
	for _, fn := range []string{"vary-pass.tsv", "vary-declined.tsv", "vary-diverged.tsv", "vary-interp-reject.tsv"} {
		data, err := os.ReadFile(filepath.Join(outDir, fn))
		if os.IsNotExist(err) {
			continue // a bucket with no rows writes no file
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "s.tsv:2") {
			t.Errorf("%s references the declined base's variants:\n%s", fn, data)
		}
	}
}
