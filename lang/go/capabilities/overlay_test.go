package capabilities

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestOverlayWhiteoutConcurrency exercises concurrent whiteout mutation and
// reads so `go test -race` catches a regression of the wmu guard (the
// PR-review fix): IO.watch runs its callback on a fork while the main program
// keeps doing I/O, and without the lock the shared whiteouts map is a fatal
// "concurrent map read and map write".
func TestOverlayWhiteoutConcurrency(t *testing.T) {
	o, _, _ := overlayPair(func(l *MemFileOps) {
		for i := 0; i < 16; i++ {
			if err := l.WriteFile(fmt.Sprintf("f%d.txt", i), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 16; i++ {
				p := fmt.Sprintf("f%d.txt", i)
				_ = o.Remove(p, false)                 // setWhiteout
				_ = o.WriteFile(p, []byte("y"), 0o644) // clearWhiteout
				_, _ = o.ReadDir(".")                  // isWhiteout
			}
		}()
	}
	wg.Wait()
}

// overlayPair builds the canonical unit-test shape: a writable MEM upper
// over a read-only-treated MEM lower seeded by the caller. Returned lower
// is only mutated through seeding, never by the overlay (asserted where
// it matters).
func overlayPair(seed func(lower *MemFileOps)) (*OverlayFileOps, *MemFileOps, *MemFileOps) {
	lower := NewMem()
	if seed != nil {
		seed(lower)
	}
	upper := NewMem()
	return NewOverlay(upper, lower), upper, lower
}

func TestOverlayReadsFallThrough(t *testing.T) {
	o, _, _ := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("base.txt", []byte("lower"), 0o640); err != nil {
			t.Fatal(err)
		}
	})
	// A lower-only file reads and stats through the union.
	b, err := o.ReadFile("base.txt")
	if err != nil || string(b) != "lower" {
		t.Fatalf("union read = %q (%v)", b, err)
	}
	fi, err := o.Stat("base.txt", true)
	if err != nil || fi.Size != 5 || fi.Mode.Perm() != 0o640 {
		t.Errorf("union stat = %+v (%v)", fi, err)
	}
	// An upper write SHADOWS the lower without touching it.
	if err := o.WriteFile("base.txt", []byte("upper"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := o.ReadFile("base.txt"); string(b) != "upper" {
		t.Errorf("shadowed read = %q", b)
	}
	// Absent everywhere is a clean not-exist.
	if _, err := o.ReadFile("ghost"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent union read = %v", err)
	}
	if _, err := o.Stat("ghost", false); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent union stat = %v", err)
	}
}

func TestOverlayLowerStaysPristine(t *testing.T) {
	o, _, lower := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("keep.txt", []byte("original"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if err := o.WriteFile("keep.txt", []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.Remove("keep.txt", false); err != nil {
		t.Fatal(err)
	}
	if err := o.Chmod("keep.txt", 0o600); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("chmod after union delete = %v, want not-exist", err)
	}
	// The LOWER layer never changed.
	if b, err := lower.ReadFile("keep.txt"); err != nil || string(b) != "original" {
		t.Errorf("lower mutated: %q (%v)", b, err)
	}
	if fi, _ := lower.Stat("keep.txt", false); fi.Mode.Perm() != 0o644 {
		t.Errorf("lower mode mutated: %v", fi.Mode.Perm())
	}
}

func TestOverlayWhiteouts(t *testing.T) {
	o, _, _ := overlayPair(func(l *MemFileOps) {
		for _, p := range []string{"d/a.txt", "d/b.txt", "top.txt"} {
			if err := l.WriteFile(p, []byte(p), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})
	// Union-deleting a lower file hides it from read/stat/list.
	if err := o.Remove("d/a.txt", false); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ReadFile("d/a.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("whited-out read = %v", err)
	}
	infos, err := o.ReadDir("d")
	if err != nil || len(infos) != 1 || infos[0].Name != "b.txt" {
		t.Errorf("whited-out listing = %+v (%v)", infos, err)
	}
	if got := o.Whiteouts(); len(got) != 1 || got[0] != filepath.FromSlash("d/a.txt") {
		t.Errorf("Whiteouts() = %v", got)
	}
	// Re-creating the path clears the whiteout.
	if err := o.WriteFile("d/a.txt", []byte("reborn"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := o.ReadFile("d/a.txt"); string(b) != "reborn" {
		t.Errorf("reborn read = %q", b)
	}
	if got := o.Whiteouts(); len(got) != 0 {
		t.Errorf("whiteout survived rebirth: %v", got)
	}
	// A whited-out DIRECTORY hides its whole lower subtree (ancestor rule).
	if err := o.Remove("d", true); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ReadFile("d/b.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("subtree visible under whited-out dir: %v", err)
	}
	if _, err := o.ReadDir("d"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("whited-out dir lists: %v", err)
	}
	// Remove of an absent path: recursive is idempotent, plain errors.
	if err := o.Remove("ghost", true); err != nil {
		t.Errorf("recursive remove of absent = %v", err)
	}
	if err := o.Remove("ghost", false); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("plain remove of absent = %v", err)
	}
}

func TestOverlayMergedListing(t *testing.T) {
	o, _, _ := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("d/lower.txt", []byte("l"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := l.WriteFile("d/both.txt", []byte("lower-version"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if err := o.WriteFile("d/upper.txt", []byte("u"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.WriteFile("d/both.txt", []byte("upper-version"), 0o644); err != nil {
		t.Fatal(err)
	}
	infos, err := o.ReadDir("d")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(infos))
	for i, fi := range infos {
		names[i] = fi.Name
	}
	want := []string{"both.txt", "lower.txt", "upper.txt"}
	if len(names) != len(want) {
		t.Fatalf("merged listing = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("merged listing = %v, want %v", names, want)
		}
	}
	// The collision entry is the UPPER's.
	for _, fi := range infos {
		if fi.Name == "both.txt" && fi.Size != int64(len("upper-version")) {
			t.Errorf("collision entry took the lower size: %d", fi.Size)
		}
	}
	// Listing a dir that only exists below works; absent-everywhere errors.
	if _, err := o.ReadDir("nowhere"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent union readdir = %v", err)
	}
}

func TestOverlayCopyUpMutations(t *testing.T) {
	o, upper, lower := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("f.txt", []byte("12345"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := l.Chtimes("f.txt", time.Unix(100, 0), time.Unix(100, 0)); err != nil {
			t.Fatal(err)
		}
	})
	// Truncate of a lower-only file copies up, preserving mode, then acts.
	if err := o.Truncate("f.txt", 2); err != nil {
		t.Fatal(err)
	}
	if b, err := upper.ReadFile("f.txt"); err != nil || string(b) != "12" {
		t.Errorf("copy-up content = %q (%v)", b, err)
	}
	if fi, _ := o.Stat("f.txt", true); fi.Mode.Perm() != 0o640 {
		t.Errorf("copy-up mode = %v, want the lower's 0640", fi.Mode.Perm())
	}
	if b, _ := lower.ReadFile("f.txt"); string(b) != "12345" {
		t.Errorf("lower content mutated: %q", b)
	}
	// Chmod / Chtimes on another lower-only file copy up too.
	if err := lower.WriteFile("g.txt", []byte("g"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.Chmod("g.txt", 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := o.Stat("g.txt", true); fi.Mode.Perm() != 0o600 {
		t.Errorf("chmod after copy-up = %v", fi.Mode.Perm())
	}
	mt := time.Unix(777, 0)
	if err := o.Chtimes("g.txt", mt, mt); err != nil {
		t.Fatal(err)
	}
	if fi, _ := o.Stat("g.txt", true); !fi.ModTime.Equal(mt) {
		t.Errorf("chtimes after copy-up = %v", fi.ModTime)
	}
	// Mutating an absent path is a clean not-exist per op.
	if err := o.Truncate("ghost", 0); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("truncate absent = %v", err)
	}
	if err := o.Chtimes("ghost", mt, mt); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("chtimes absent = %v", err)
	}
}

func TestOverlayRename(t *testing.T) {
	o, _, lower := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("low.txt", []byte("low"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := l.WriteFile("tree/a/x.txt", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	// Renaming a lower-only FILE copies up, renames, and whites out.
	if err := o.Rename("low.txt", "moved.txt"); err != nil {
		t.Fatal(err)
	}
	if b, err := o.ReadFile("moved.txt"); err != nil || string(b) != "low" {
		t.Errorf("renamed read = %q (%v)", b, err)
	}
	if _, err := o.ReadFile("low.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old union name visible after rename: %v", err)
	}
	if b, _ := lower.ReadFile("low.txt"); string(b) != "low" {
		t.Errorf("lower mutated by union rename: %q", b)
	}
	// Renaming a lower-only TREE copies the whole subtree up.
	if err := o.Rename("tree", "treed"); err != nil {
		t.Fatal(err)
	}
	if b, err := o.ReadFile("treed/a/x.txt"); err != nil || string(b) != "x" {
		t.Errorf("renamed tree read = %q (%v)", b, err)
	}
	if _, err := o.ReadDir("tree"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old tree visible: %v", err)
	}
	// Renaming an upper-only file is a plain upper rename (no whiteout).
	if err := o.WriteFile("up.txt", []byte("u"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.Rename("up.txt", "up2.txt"); err != nil {
		t.Fatal(err)
	}
	for _, w := range o.Whiteouts() {
		if w == "up.txt" {
			t.Error("upper-only rename recorded a whiteout")
		}
	}
	// Renaming an absent / whited-out path errors.
	if err := o.Rename("ghost", "z"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("rename absent = %v", err)
	}
	if err := o.Remove("moved.txt", false); err != nil {
		t.Fatal(err)
	}
	if err := o.Rename("low.txt", "z"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("rename whited-out = %v", err)
	}

	// Renaming a MIXED directory (children in BOTH layers) carries the
	// lower-only children along and keeps upper modifications — the PR-review
	// fix. Without merging the lower subtree up first, the old-name whiteout
	// would drop mix/low.txt.
	o2, _, _ := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("mix/low.txt", []byte("L"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := l.WriteFile("mix/both.txt", []byte("lowver"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	// Upper writes make `mix` exist in both layers: a new child, and one that
	// SHADOWS a lower child (which the merge must not clobber).
	if err := o2.WriteFile("mix/up.txt", []byte("U"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o2.WriteFile("mix/both.txt", []byte("upver"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o2.Rename("mix", "mixed"); err != nil {
		t.Fatal(err)
	}
	if b, err := o2.ReadFile("mixed/low.txt"); err != nil || string(b) != "L" {
		t.Errorf("mixed rename lost the lower-only child: %q (%v)", b, err)
	}
	if b, err := o2.ReadFile("mixed/up.txt"); err != nil || string(b) != "U" {
		t.Errorf("mixed rename lost the upper child: %q (%v)", b, err)
	}
	if b, err := o2.ReadFile("mixed/both.txt"); err != nil || string(b) != "upver" {
		t.Errorf("mixed rename clobbered the upper modification: %q (%v), want upver", b, err)
	}
	if _, err := o2.ReadDir("mix"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old mixed dir still visible after rename: %v", err)
	}
}

func TestOverlayLinks(t *testing.T) {
	o, _, lower := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("t.txt", []byte("target"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	// Symlink over a VISIBLE lower entry collides (union EEXIST).
	if err := o.Symlink("t.txt", "t.txt"); !errors.Is(err, os.ErrExist) {
		t.Errorf("symlink over lower entry = %v, want EEXIST", err)
	}
	if err := o.Symlink("t.txt", "sl"); err != nil {
		t.Fatal(err)
	}
	if b, err := o.ReadFile("sl"); err != nil || string(b) != "target" {
		t.Errorf("read through union symlink = %q (%v)", b, err)
	}
	// A hard link to a lower-only target copies it up, then shares WITHIN
	// the upper: writes through the link do not leak below.
	if err := o.Link("t.txt", "hl"); err != nil {
		t.Fatal(err)
	}
	if err := o.WriteFile("hl", []byte("mutated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := o.ReadFile("t.txt"); string(b) != "mutated" {
		t.Errorf("union hard link not shared: %q", b)
	}
	if b, _ := lower.ReadFile("t.txt"); string(b) != "target" {
		t.Errorf("lower mutated through hard link: %q", b)
	}
	// Hard-linking an absent target errors; linking over an existing name.
	if err := o.Link("ghost", "hl2"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("link absent target = %v", err)
	}
	if err := o.Link("t.txt", "hl"); !errors.Is(err, os.ErrExist) {
		t.Errorf("link over existing = %v", err)
	}
}

// TestOverlayOverReal is the promised use case: a mem upper over the REAL
// filesystem, with the real side never mutated.
func TestOverlayOverReal(t *testing.T) {
	root := t.TempDir()
	seed := filepath.Join(root, "fixture.txt")
	if err := os.WriteFile(seed, []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewOverlay(NewMem(), &OSFileOps{})
	// Read the real fixture through the union.
	if b, err := o.ReadFile(seed); err != nil || string(b) != "real" {
		t.Fatalf("union read of real file = %q (%v)", b, err)
	}
	// Shadow it, delete it — the disk file is untouched.
	if err := o.WriteFile(seed, []byte("shadow"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := o.ReadFile(seed); string(b) != "shadow" {
		t.Error("shadow write not visible")
	}
	if err := o.Remove(seed, false); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ReadFile(seed); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("union delete not effective: %v", err)
	}
	if b, err := os.ReadFile(seed); err != nil || string(b) != "real" {
		t.Errorf("REAL file mutated through the overlay: %q (%v)", b, err)
	}
	// New files land only in the mem upper.
	newFile := filepath.Join(root, "new.txt")
	if err := o.WriteFile(newFile, []byte("mem-only"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("union write reached the real disk: %v", err)
	}
}

// TestOverlayNested pins that an overlay stacks as a layer of another.
func TestOverlayNested(t *testing.T) {
	base := NewMem()
	if err := base.WriteFile("deep.txt", []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	mid := NewOverlay(NewMem(), base)
	top := NewOverlay(NewMem(), mid)
	if b, err := top.ReadFile("deep.txt"); err != nil || string(b) != "base" {
		t.Fatalf("two-deep read = %q (%v)", b, err)
	}
	if err := top.Remove("deep.txt", false); err != nil {
		t.Fatal(err)
	}
	if _, err := top.ReadFile("deep.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("top whiteout ineffective: %v", err)
	}
	if b, _ := mid.ReadFile("deep.txt"); string(b) != "base" {
		t.Errorf("middle layer mutated: %q", b)
	}
}

func TestOverlayResolvePathDelegates(t *testing.T) {
	upper := NewMem()
	upper.Cwd = "/work"
	o := NewOverlay(upper, NewMem())
	got, err := o.ResolvePath("rel.txt")
	if err != nil || got != filepath.FromSlash("/work/rel.txt") {
		t.Errorf("ResolvePath = %q (%v), want the upper's resolution", got, err)
	}
}
