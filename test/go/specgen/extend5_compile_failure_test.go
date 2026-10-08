package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtendLayerCompileBucket pins -extend5's compile-compile failure bucket with a
// SYNTHETIC passing root whose one-atom extensions decline: a fn body whose
// result is a FRAME-LOCAL module value (`def m (module […])` inside the fn)
// has no bind operand for the popped-with-the-frame binding, so the unit
// declines "body result of unknown provenance" and `<root> drop` (and
// friends) classify as compile-declined whatever follows the call. (The
// root this test first used, a module value left on the TOP-LEVEL stack,
// graduated 2026-09-05 — module-family values read live — which is why
// the compile failure has to live inside a fn unit now.) The pipeline test's
// generated alphabet corpus stopped producing this bucket when
// OpDispatchRematch compiled its last declining candidates — a
// user-supplied corpus still can, so the arm is pinned here with one.
func TestExtendLayerCompileBucket(t *testing.T) {
	dir := t.TempDir()
	passing := filepath.Join(dir, "passing.tsv")
	// The root is interpreter-clean (its expected value is the module
	// render); the pass1 file is empty so every extension of the root is a
	// candidate.
	if err := os.WriteFile(passing,
		[]byte("def f fn [[] [Any] [def m (module [export \"X\" {a: 1}]) m]] f\tModule(inline)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pass1 := filepath.Join(dir, "pass1.tsv")
	if err := os.WriteFile(pass1, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	pOut := filepath.Join(dir, "p.tsv")
	ckOut := filepath.Join(dir, "ck.tsv")
	coOut := filepath.Join(dir, "co.tsv")
	roOut := filepath.Join(dir, "ro.tsv")
	mmOut := filepath.Join(dir, "mm.tsv")
	runSpecgenMain(t, "-extend5",
		"-passing", passing, "-len123", pass1,
		"-pass-out", pOut, "-check-out", ckOut, "-compile-out", coOut,
		"-runtime-out", roOut, "-mismatch-out", mmOut)

	rows := decodeFile(t, coOut)
	if len(rows) == 0 {
		t.Fatal("the module-residual root must yield compile-declined extensions (the five-way compile bucket)")
	}
	found := false
	for _, r := range rows {
		if r.input == `def f fn [[] [Any] [def m (module [export "X" {a: 1}]) m]] f drop` {
			found = true
			if r.expected == "" {
				t.Error("the compile bucket row must carry the compile failure reason")
			}
		}
	}
	if !found {
		t.Errorf("`... m drop` missing from the compile bucket; rows: %v", rows)
	}
}
