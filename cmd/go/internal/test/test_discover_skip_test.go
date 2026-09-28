package test

import (
	"os"
	"path/filepath"
	"testing"
)

// NUR082: `boru test`'s discovery shares the tree walk with fmt and check,
// so the `.boru/` package directory (the build and install cache) is never
// a source of test files.
func TestDiscoverSkipsPackageDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".boru", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "a_test.boru")
	for _, p := range []string{keep, filepath.Join(dir, ".boru", "pkg", "b_test.boru")} {
		if err := os.WriteFile(p, []byte("1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != keep {
		t.Fatalf("discover = %v, want only %s (the .boru/ tree skipped)", got, keep)
	}
}
