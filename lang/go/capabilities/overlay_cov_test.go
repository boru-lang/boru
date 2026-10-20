package capabilities

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

// overlayFailOps wraps a MemFileOps and fails selected methods, so the
// overlay's error-propagation arms are reachable.
type overlayFailOps struct {
	*MemFileOps
	failResolve  bool
	failReadFile bool
	failWrite    bool
	failReadDir  bool
	failRemove   bool
	failMkdir    bool
	// path-targeted failures, for arms deep in a tree walk
	failStatPath    string
	failReadDirPath string
}

func (f *overlayFailOps) ResolvePath(p string) (string, error) {
	if f.failResolve {
		return "", fmt.Errorf("resolve boom")
	}
	return f.MemFileOps.ResolvePath(p)
}

func (f *overlayFailOps) ReadFile(p string) ([]byte, error) {
	if f.failReadFile {
		return nil, fmt.Errorf("readfile boom")
	}
	return f.MemFileOps.ReadFile(p)
}

func (f *overlayFailOps) WriteFile(p string, d []byte, m os.FileMode) error {
	if f.failWrite {
		return fmt.Errorf("writefile boom")
	}
	return f.MemFileOps.WriteFile(p, d, m)
}

func (f *overlayFailOps) ReadDir(p string) ([]FileInfo, error) {
	if f.failReadDir || (f.failReadDirPath != "" && p == f.failReadDirPath) {
		return nil, fmt.Errorf("readdir boom")
	}
	return f.MemFileOps.ReadDir(p)
}

func (f *overlayFailOps) Remove(p string, r bool) error {
	if f.failRemove {
		return fmt.Errorf("remove boom")
	}
	return f.MemFileOps.Remove(p, r)
}

func (f *overlayFailOps) MkdirAll(p string, m os.FileMode) error {
	if f.failMkdir {
		return fmt.Errorf("mkdir boom")
	}
	return f.MemFileOps.MkdirAll(p, m)
}

func (f *overlayFailOps) Stat(p string, follow bool) (FileInfo, error) {
	if f.failStatPath != "" && p == f.failStatPath {
		return FileInfo{}, fmt.Errorf("stat boom")
	}
	return f.MemFileOps.Stat(p, follow)
}

func TestOverlayDerefELOOP(t *testing.T) {
	o, _, _ := overlayPair(nil)
	if err := o.Symlink("b", "a"); err != nil {
		t.Fatal(err)
	}
	if err := o.Symlink("a", "b"); err != nil {
		t.Fatal(err)
	}
	// Every deref-ing op propagates ELOOP.
	if _, err := o.ReadFile("a"); !errors.Is(err, errMemTooManyLinks) {
		t.Errorf("ReadFile cycle = %v", err)
	}
	if err := o.WriteFile("a", []byte("x"), 0o644); !errors.Is(err, errMemTooManyLinks) {
		t.Errorf("WriteFile cycle = %v", err)
	}
	if _, err := o.Stat("a", true); !errors.Is(err, errMemTooManyLinks) {
		t.Errorf("Stat cycle = %v", err)
	}
	if _, err := o.ReadDir("a"); !errors.Is(err, errMemTooManyLinks) {
		t.Errorf("ReadDir cycle = %v", err)
	}
	if err := o.Chmod("a", 0o600); !errors.Is(err, errMemTooManyLinks) {
		t.Errorf("Chmod cycle = %v", err)
	}
}

func TestOverlayResolveFallback(t *testing.T) {
	// A failing upper ResolvePath degrades the whiteout key to the
	// lexical form — the union still deletes correctly.
	upper := &overlayFailOps{MemFileOps: NewMem(), failResolve: true}
	lower := NewMem()
	if err := lower.WriteFile("low.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewOverlay(upper, lower)
	if err := o.Remove("./low.txt", false); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ReadFile("low.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("whiteout with degraded key ineffective: %v", err)
	}
}

func TestOverlayCopyUpErrors(t *testing.T) {
	// Lower ReadFile fails mid copy-up.
	lower := &overlayFailOps{MemFileOps: NewMem(), failReadFile: true}
	if err := lower.MemFileOps.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewOverlay(NewMem(), lower)
	if err := o.Truncate("f", 0); err == nil {
		t.Error("expected copy-up ReadFile error")
	}
	// Upper WriteFile fails mid copy-up.
	lower2 := NewMem()
	if err := lower2.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o2 := NewOverlay(&overlayFailOps{MemFileOps: NewMem(), failWrite: true}, lower2)
	if err := o2.Truncate("f", 0); err == nil {
		t.Error("expected copy-up WriteFile error")
	}
	// A lower-only DIRECTORY copy-up (chmod on it) materialises as a dir.
	lower3 := NewMem()
	if err := lower3.MkdirAll("d", 0o755); err != nil {
		t.Fatal(err)
	}
	upper3 := NewMem()
	o3 := NewOverlay(upper3, lower3)
	if err := o3.Chmod("d", 0o700); err != nil {
		t.Fatal(err)
	}
	if fi, err := upper3.Stat("d", false); err != nil || !fi.IsDir {
		t.Errorf("dir copy-up = %+v (%v)", fi, err)
	}
}

func TestOverlayRemoveUnionArms(t *testing.T) {
	// A union directory whose remaining children live BELOW refuses a
	// non-recursive remove.
	o, _, _ := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("d/low.txt", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if err := o.MkdirAll("d", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := o.Remove("d", false); !errors.Is(err, errMemNotEmpty) {
		t.Errorf("non-recursive remove of populated union dir = %v", err)
	}
	// The upper layer's own Remove failure propagates.
	upper := &overlayFailOps{MemFileOps: NewMem(), failRemove: true}
	if err := upper.MemFileOps.WriteFile("u.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o2 := NewOverlay(upper, NewMem())
	if err := o2.Remove("u.txt", false); err == nil {
		t.Error("expected the upper Remove failure to propagate")
	}
}

func TestOverlayRenameErrorArms(t *testing.T) {
	// File copy-up failure during rename (lower ReadFile fails).
	lower := &overlayFailOps{MemFileOps: NewMem(), failReadFile: true}
	if err := lower.MemFileOps.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewOverlay(NewMem(), lower)
	if err := o.Rename("f", "g"); err == nil {
		t.Error("expected rename copy-up failure")
	}
	// Tree copy-up failure during rename (lower ReadDir fails).
	lower2 := &overlayFailOps{MemFileOps: NewMem(), failReadDir: true}
	if err := lower2.MemFileOps.WriteFile("d/x.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o2 := NewOverlay(NewMem(), lower2)
	if err := o2.Rename("d", "e"); err == nil {
		t.Error("expected rename tree copy-up failure")
	}
	// The upper rename's own destination rules propagate (dir over file).
	o3, _, _ := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("t/x.txt", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if err := o3.WriteFile("dst", []byte("d"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o3.Rename("t", "dst"); !errors.Is(err, errMemNotDir) {
		t.Errorf("union rename dir over file = %v", err)
	}
	// copyUpTree skips whited-out children and recurses into subdirs.
	o4, upper4, _ := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("big/keep.txt", []byte("k"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := l.WriteFile("big/drop.txt", []byte("d"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := l.WriteFile("big/sub/deep.txt", []byte("s"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if err := o4.Remove("big/drop.txt", false); err != nil {
		t.Fatal(err)
	}
	if err := o4.Rename("big", "moved"); err != nil {
		t.Fatal(err)
	}
	if _, err := upper4.ReadFile("moved/drop.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Error("whited-out child copied up by rename")
	}
	if b, err := o4.ReadFile("moved/sub/deep.txt"); err != nil || string(b) != "s" {
		t.Errorf("deep child lost in tree rename: %q (%v)", b, err)
	}
}

func TestOverlayLinkSymlinkErrorArms(t *testing.T) {
	// The upper's own refusal propagates: creating under an upper FILE.
	o, _, _ := overlayPair(nil)
	if err := o.WriteFile("uf", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.Symlink("t", "uf/sl"); !errors.Is(err, errMemNotDir) {
		t.Errorf("symlink under a file = %v", err)
	}
	if err := o.WriteFile("t", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.Link("t", "uf/hl"); !errors.Is(err, errMemNotDir) {
		t.Errorf("hard link under a file = %v", err)
	}
	if err := o.MkdirAll("uf/sub", 0o755); !errors.Is(err, errMemNotDir) {
		t.Errorf("mkdir under a file = %v", err)
	}
	if err := o.WriteFile("uf/w", []byte("x"), 0o644); !errors.Is(err, errMemNotDir) {
		t.Errorf("write under a file = %v", err)
	}
	// A whited-out lower target refuses a hard link.
	o2, _, _ := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("lt", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if err := o2.Remove("lt", false); err != nil {
		t.Fatal(err)
	}
	if err := o2.Link("lt", "hl"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("link to whited-out target = %v", err)
	}
}

func TestOverlayChtimesCopyUpPreservesStamp(t *testing.T) {
	// The copy-up preserves the lower mtime before the mutation applies —
	// observable when only atime is being changed... Chtimes sets both, so
	// instead assert the pre-mutation copy carries the lower stamp by
	// copy-up via Chmod (which does not touch times).
	lowStamp := time.Unix(4242, 0)
	o, upper, _ := overlayPair(func(l *MemFileOps) {
		if err := l.WriteFile("f", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := l.Chtimes("f", lowStamp, lowStamp); err != nil {
			t.Fatal(err)
		}
	})
	if err := o.Chmod("f", 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, err := upper.Stat("f", false); err != nil || !fi.ModTime.Equal(lowStamp) {
		t.Errorf("copy-up lost the lower mtime: %+v (%v)", fi, err)
	}
}

func TestOverlayCopyUpTreeErrorArms(t *testing.T) {
	skipWithoutUnixMetadata(t, "mode-enforced access")
	// The tree-walk's per-child Stat failure propagates.
	lower := &overlayFailOps{MemFileOps: NewMem(), failStatPath: "d/bad.txt"}
	if err := lower.MemFileOps.WriteFile("d/bad.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewOverlay(NewMem(), lower).Rename("d", "e"); err == nil {
		t.Error("expected child-stat failure to propagate")
	}
	// The upper's MkdirAll failure propagates.
	lower2 := NewMem()
	if err := lower2.WriteFile("d/x.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o2 := NewOverlay(&overlayFailOps{MemFileOps: NewMem(), failMkdir: true}, lower2)
	if err := o2.Rename("d", "e"); err == nil {
		t.Error("expected upper MkdirAll failure to propagate")
	}
	// A NESTED subdir's ReadDir failure propagates through the recursion.
	lower3 := &overlayFailOps{MemFileOps: NewMem(), failReadDirPath: "d/sub"}
	if err := lower3.MemFileOps.WriteFile("d/sub/deep.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewOverlay(NewMem(), lower3).Rename("d", "e"); err == nil {
		t.Error("expected nested readdir failure to propagate")
	}
}
