package core

import "testing"

// The pass-end cleanup seam and the bind-ledger suppression bracket are what
// keeps ANALYSIS-ONLY registry state out of both the post-pass registry and
// the ledger. The live-depth oracle (test/go/langspec) proves the corpus-level
// consequence; these pin the seam's own contract, which cover-gate-core
// requires this module's suite to reach on its own.

// Pass-end cleanups run LIFO — chained analysis pushes on one name tear down
// top-first — and exactly once, however many times the closer is invoked
// (closers ride defer AND t.Cleanup in places).
func TestPassEndCleanupsRunLIFOExactlyOnce(t *testing.T) {
	c := &CheckState{}
	done := c.Begin()
	var order []int
	c.AddPassEndCleanup(func() { order = append(order, 1) })
	c.AddPassEndCleanup(func() { order = append(order, 2) })
	done()
	if len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Fatalf("cleanups must run LIFO once: got %v", order)
	}
	done()
	if len(order) != 2 {
		t.Fatalf("a second closer call must not re-run cleanups: got %v", order)
	}
}

// Outside a pass there is no pass end to attach to — the caller's mutation is
// then runtime state, not an analysis artifact — so the add is a no-op. A nil
// CheckState (a registry-less caller) must not panic either.
func TestAddPassEndCleanupOutsidePass(t *testing.T) {
	c := &CheckState{}
	c.AddPassEndCleanup(func() { t.Fatal("must not be registered outside a pass") })
	if len(c.PassEndCleanups) != 0 {
		t.Fatal("a cleanup added outside a pass must be dropped")
	}
	c.Begin()() // immediate end: nothing to run

	var nilState *CheckState
	nilState.AddPassEndCleanup(func() {})
}

// Begin resets the cleanup list: a closure staged in an outer pass must not
// fire when a nested Begin supersedes it, and the outer closer — running
// after the nested pass wiped the list — finds nothing left to run. (Both
// closers still run, keeping the process-wide pass-depth counter balanced.)
func TestBeginResetsPassEndCleanups(t *testing.T) {
	c := &CheckState{}
	outer := c.Begin()
	c.AddPassEndCleanup(func() { t.Fatal("stale cleanup leaked across Begin") })
	c.Begin()()
	outer()
}

// Clone deep-copies the cleanup list so a sandboxed run's appends cannot
// bleed into the snapshot's backing array.
func TestCloneCopiesPassEndCleanups(t *testing.T) {
	c := &CheckState{}
	done := c.Begin()
	ran := 0
	c.AddPassEndCleanup(func() { ran++ })
	cp := c.Clone()
	if len(cp.PassEndCleanups) != 1 {
		t.Fatalf("clone must carry the cleanup list, got %d", len(cp.PassEndCleanups))
	}
	c.AddPassEndCleanup(func() { ran += 10 })
	if len(cp.PassEndCleanups) != 1 {
		t.Fatal("an append on the original must not grow the clone")
	}
	done()
	if ran != 11 {
		t.Fatalf("the original's closer runs both cleanups exactly once, got %d", ran)
	}
}
