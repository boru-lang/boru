package capabilities

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Wave-4 coverage: the two Clock implementations, OSFileOps.MkdirAll (the
// resolve-failure, mkdir-failure and happy arms) and MemFileOps.MkdirAll
// (parent-dir recording) — positives paired with the failing seams.

// TestWave4Clocks pins WallClock (a live reading) and FixedClock (frozen).
func TestWave4Clocks(t *testing.T) {
	before := time.Now().Add(-time.Minute)
	if got := (WallClock{}).Now(); got.Before(before) {
		t.Errorf("WallClock.Now = %v, want a current reading", got)
	}
	frozen := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)
	if got := (FixedClock{T: frozen}).Now(); !got.Equal(frozen) {
		t.Errorf("FixedClock.Now = %v, want %v", got, frozen)
	}
}

// TestWave4OSMkdirAll pins OSFileOps.MkdirAll: a real nested create, the
// unresolvable-relative-path rejection, and a failing mkdir seam.
func TestWave4OSMkdirAll(t *testing.T) {
	dir := t.TempDir()
	ops := &OSFileOps{}
	target := filepath.Join(dir, "a", "b")
	if err := ops.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		t.Errorf("MkdirAll did not create %s: %v", target, err)
	}

	// A relative path with no resolvable working directory is refused.
	noWD := &OSFileOps{getwd: func() (string, error) { return "", errors.New("no wd") }}
	if err := noWD.MkdirAll("relative/dir", 0o755); err == nil {
		t.Error("MkdirAll with a failing getwd must error")
	}

	// The mkdir seam itself failing propagates.
	failing := &OSFileOps{mkdirAll: func(string, os.FileMode) error { return errors.New("mkdir refused") }}
	if err := failing.MkdirAll(filepath.Join(dir, "c"), 0o755); err == nil ||
		err.Error() != "mkdir refused" {
		t.Errorf("MkdirAll with a failing seam: %v, want mkdir refused", err)
	}
}

// TestWave4MemMkdirAll pins MemFileOps.MkdirAll: the directory and all its
// parents are recorded, idempotently.
func TestWave4MemMkdirAll(t *testing.T) {
	m := NewMem()
	if err := m.MkdirAll("/a/b/c", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, d := range []string{"/a/b/c", "/a/b", "/a"} {
		if !m.Dirs[filepath.FromSlash(d)] {
			t.Errorf("dir %s not recorded: %v", d, m.Dirs)
		}
	}
	if err := m.MkdirAll("/a/b/c", 0o755); err != nil {
		t.Errorf("MkdirAll must be idempotent: %v", err)
	}
	// A relative path resolves against the simulated working directory.
	m.Cwd = "/wd"
	if err := m.MkdirAll("rel", 0o755); err != nil || !m.Dirs[filepath.FromSlash("/wd/rel")] {
		t.Errorf("relative MkdirAll = %v, dirs %v", err, m.Dirs)
	}
}
