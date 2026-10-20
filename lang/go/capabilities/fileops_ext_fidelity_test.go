package capabilities

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Unit tests for the strict-fidelity MemFileOps behaviours that the
// differential harness cannot pin on every host: the euid permission
// seam (both arms, regardless of the uid the suite runs as), the POSIX
// rename empty-dir replace (overlayfs hosts skip it), hard-link alias
// promotion internals, and the symlink resolution error taxonomy.

func memAsUser(uid int) *MemFileOps {
	m := NewMem()
	m.euidFn = func() int { return uid }
	return m
}

func TestMemPermEnforcementNonRoot(t *testing.T) {
	m := memAsUser(1000)
	if err := m.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Chmod("f", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadFile("f"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("read of 0000 file = %v, want EACCES", err)
	}
	if err := m.WriteFile("f", []byte("y"), 0o644); !errors.Is(err, os.ErrPermission) {
		t.Errorf("write of 0000 file = %v, want EACCES", err)
	}
	if err := m.Truncate("f", 0); !errors.Is(err, os.ErrPermission) {
		t.Errorf("truncate of 0000 file = %v, want EACCES", err)
	}
	// A write-only file reads denied but writes fine; a read-only inverse.
	if err := m.Chmod("f", 0o200); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("f", []byte("z"), 0o644); err != nil {
		t.Errorf("write of 0200 file: %v", err)
	}
	// An unreadable DIRECTORY refuses ReadDir.
	if err := m.MkdirAll("dark", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("dark/a", []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Chmod("dark", 0o300); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadDir("dark"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("readdir of unreadable dir = %v, want EACCES", err)
	}
	// A read-only PARENT refuses namespace mutations: create, remove,
	// rename, symlink, and hard link.
	if err := m.MkdirAll("ro", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("ro/keep", []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Chmod("ro", 0o555); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("ro/new", []byte("n"), 0o644); !errors.Is(err, os.ErrPermission) {
		t.Errorf("create in read-only dir = %v, want EACCES", err)
	}
	if err := m.Remove("ro/keep", false); !errors.Is(err, os.ErrPermission) {
		t.Errorf("remove in read-only dir = %v, want EACCES", err)
	}
	if err := m.Rename("ro/keep", "elsewhere"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("rename out of read-only dir = %v, want EACCES", err)
	}
	if err := m.WriteFile("src", []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename("src", "ro/in"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("rename into read-only dir = %v, want EACCES", err)
	}
	if err := m.Symlink("src", "ro/sl"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("symlink in read-only dir = %v, want EACCES", err)
	}
	if err := m.Link("src", "ro/hl"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("hard link in read-only dir = %v, want EACCES", err)
	}
	if err := m.MkdirAll("ro/sub", 0o755); !errors.Is(err, os.ErrPermission) {
		t.Errorf("mkdir in read-only dir = %v, want EACCES", err)
	}
}

func TestMemPermRootBypasses(t *testing.T) {
	m := memAsUser(0) // explicit root arm, independent of the suite's uid
	if err := m.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Chmod("f", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadFile("f"); err != nil {
		t.Errorf("root read of 0000 file: %v", err)
	}
	if err := m.WriteFile("f", []byte("y"), 0o644); err != nil {
		t.Errorf("root write of 0000 file: %v", err)
	}
	if err := m.MkdirAll("ro", 0o555); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("ro/new", []byte("n"), 0o644); err != nil {
		t.Errorf("root create in read-only dir: %v", err)
	}
}

func TestMemPermDefaultUsesProcessEuid(t *testing.T) {
	// With no seam installed, euid() consults the real process uid — the
	// live default every production registry gets.
	m := NewMem()
	if got := m.euid(); got != os.Geteuid() {
		t.Errorf("euid() = %d, want process euid %d", got, os.Geteuid())
	}
	if m.now().IsZero() {
		t.Error("now() default should be the live clock")
	}
}

func TestMemRenameDirOverEmptyPOSIX(t *testing.T) {
	// POSIX rename replaces an EMPTY destination directory. (The
	// differential twin probes the host and skips on overlayfs — this
	// pins mem's side unconditionally.)
	m := NewMem()
	if err := m.WriteFile("tree/x.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.MkdirAll("empty", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename("tree", "empty"); err != nil {
		t.Fatalf("rename dir over empty dir: %v", err)
	}
	if b, err := m.ReadFile("empty/x.txt"); err != nil || string(b) != "x" {
		t.Errorf("moved tree content = %q (%v)", b, err)
	}
	if _, err := m.Stat("tree", false); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("source dir remains: %v", err)
	}
}

func TestMemHardLinkAliasPromotion(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("t", []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{"l1", "l2"} {
		if err := m.Link("t", l); err != nil {
			t.Fatal(err)
		}
	}
	// Removing an ALIAS is a plain unlink; content stays.
	if err := m.Remove("l2", false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadFile("t"); err != nil {
		t.Errorf("canonical died with an alias: %v", err)
	}
	// Re-link two aliases, then remove the CANONICAL: first alias (sorted)
	// is promoted and the other repointed.
	if err := m.Link("t", "l2"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("t", false); err != nil {
		t.Fatal(err)
	}
	if b, err := m.ReadFile("l1"); err != nil || string(b) != "body" {
		t.Errorf("promoted alias l1 = %q (%v)", b, err)
	}
	if b, err := m.ReadFile("l2"); err != nil || string(b) != "body" {
		t.Errorf("repointed alias l2 = %q (%v)", b, err)
	}
	if _, err := m.ReadFile("t"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("removed canonical still readable: %v", err)
	}
	// Writing through the repointed alias reaches the promoted canonical.
	if err := m.WriteFile("l2", []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.ReadFile("l1"); string(b) != "v2" {
		t.Errorf("post-promotion write not shared: %q", b)
	}
	// Linking an ALIAS canonicalises: the new name points at the root.
	if err := m.Link("l2", "l3"); err != nil {
		t.Fatal(err)
	}
	if m.Links["l3"] != "l1" {
		t.Errorf("link-of-alias canonical = %q, want l1", m.Links["l3"])
	}
}

func TestMemRemoveTreePromotesOutsideAlias(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("d/in.txt", []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Link("d/in.txt", "outside"); err != nil {
		t.Fatal(err)
	}
	// An alias INSIDE the removed tree dies with it; the outside alias
	// keeps the content alive.
	if err := m.Link("d/in.txt", "d/inside-alias"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("d", true); err != nil {
		t.Fatal(err)
	}
	if b, err := m.ReadFile("outside"); err != nil || string(b) != "keep" {
		t.Errorf("outside alias = %q (%v)", b, err)
	}
	if _, err := m.ReadFile("d/inside-alias"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("inside alias survived tree removal: %v", err)
	}
}

func TestMemRenameRemapsAliasTargets(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("d/f.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Link("d/f.txt", "al"); err != nil {
		t.Fatal(err)
	}
	// Moving the tree that holds the CANONICAL repoints the outside alias.
	if err := m.Rename("d", "moved"); err != nil {
		t.Fatal(err)
	}
	if m.Links["al"] != filepath.FromSlash("moved/f.txt") {
		t.Errorf("alias target after rename = %q", m.Links["al"])
	}
	if b, err := m.ReadFile("al"); err != nil || string(b) != "x" {
		t.Errorf("alias read after rename = %q (%v)", b, err)
	}
	// Renaming the ALIAS name moves only the name.
	if err := m.Rename("al", "al2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Links["al"]; ok {
		t.Error("old alias name survived its rename")
	}
	if b, _ := m.ReadFile("al2"); string(b) != "x" {
		t.Errorf("renamed alias = %q", b)
	}
	// Renaming a file OVER an existing alias unlinks that name only.
	if err := m.WriteFile("solo", []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename("solo", "al2"); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.ReadFile("al2"); string(b) != "s" {
		t.Errorf("rename-over-alias = %q", b)
	}
	if b, _ := m.ReadFile("moved/f.txt"); string(b) != "x" {
		t.Errorf("canonical harmed by rename-over-alias: %q", b)
	}
}

func TestMemLinkKinds(t *testing.T) {
	m := NewMem()
	// Hard-linking a DIRECTORY is EPERM, like link(2).
	if err := m.MkdirAll("d", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Link("d", "dl"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("hard link of a directory = %v, want EPERM", err)
	}
	// Hard-linking a SYMLINK clones the link object (no follow).
	if err := m.Symlink("somewhere", "sl"); err != nil {
		t.Fatal(err)
	}
	if err := m.Link("sl", "slhl"); err != nil {
		t.Fatal(err)
	}
	fi, err := m.Stat("slhl", false)
	if err != nil || !fi.Symlink || fi.Target != "somewhere" {
		t.Errorf("hard-linked symlink = %+v (%v)", fi, err)
	}
}

func TestMemResolveErrorTaxonomy(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("plain", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A path THROUGH a regular file is ENOTDIR.
	if _, err := m.ReadFile("plain/under"); err == nil || !errors.Is(err, errMemNotDir) {
		t.Errorf("path through a file = %v, want ENOTDIR", err)
	}
	if err := m.WriteFile("plain/under", []byte("y"), 0o644); !errors.Is(err, errMemNotDir) {
		t.Errorf("write through a file = %v, want ENOTDIR", err)
	}
	if err := m.MkdirAll("plain/sub", 0o755); !errors.Is(err, errMemNotDir) {
		t.Errorf("mkdir through a file = %v, want ENOTDIR", err)
	}
	if err := m.MkdirAll("plain", 0o755); !errors.Is(err, errMemNotDir) {
		t.Errorf("mkdir OVER a file = %v, want ENOTDIR", err)
	}
	// Mutually-recursive symlinks exhaust the depth budget (ELOOP).
	if err := m.Symlink("b", "a"); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("a", "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadFile("a"); !errors.Is(err, errMemTooManyLinks) {
		t.Errorf("mutual symlink cycle = %v, want ELOOP", err)
	}
	// An ABSOLUTE symlink target resolves from the root.
	if err := m.WriteFile("/abs/t.txt", []byte("abs"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("/abs/t.txt", "absl"); err != nil {
		t.Fatal(err)
	}
	if b, err := m.ReadFile("absl"); err != nil || string(b) != "abs" {
		t.Errorf("absolute symlink target = %q (%v)", b, err)
	}
	// Reading a DIRECTORY is EISDIR; truncating one too; writing over one.
	if err := m.MkdirAll("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadFile("dir"); !errors.Is(err, errMemIsDir) {
		t.Errorf("read of a dir = %v, want EISDIR", err)
	}
	if err := m.Truncate("dir", 0); !errors.Is(err, errMemIsDir) {
		t.Errorf("truncate of a dir = %v, want EISDIR", err)
	}
	if err := m.WriteFile("dir", []byte("x"), 0o644); !errors.Is(err, errMemIsDir) {
		t.Errorf("write over a dir = %v, want EISDIR", err)
	}
}

func TestMemChtimesStoresATime(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	at, mt := time.Unix(111, 0), time.Unix(222, 0)
	if err := m.Chtimes("f", at, mt); err != nil {
		t.Fatal(err)
	}
	meta := m.Meta["f"]
	if meta == nil || !meta.ATime.Equal(at) || !meta.MTime.Equal(mt) {
		t.Errorf("stored times = %+v, want atime %v mtime %v", meta, at, mt)
	}
}

func TestMemClockSeam(t *testing.T) {
	m := NewMem()
	fixed := time.Unix(5000, 0)
	m.nowFn = func() time.Time { return fixed }
	if err := m.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, _ := m.Stat("f", false); !fi.ModTime.Equal(fixed) {
		t.Errorf("write mtime = %v, want the seam clock %v", fi.ModTime, fixed)
	}
	// A directly-poked file (no Meta) still reports the zero mtime —
	// the direct-poke tolerance the model documents.
	m.Files["poked"] = []byte("p")
	if fi, _ := m.Stat("poked", false); !fi.ModTime.IsZero() {
		t.Errorf("direct-poke mtime = %v, want zero", fi.ModTime)
	}
	// WriteFile over the poked (meta-less) file synthesises metadata.
	if err := m.WriteFile("poked", []byte("p2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := m.Stat("poked", false); fi.Mode.Perm() != 0o644 || !fi.ModTime.Equal(fixed) {
		t.Errorf("meta-less rewrite = %+v, want default 0644 mode + seam mtime", fi)
	}
}

func TestMemAliasListingAndStat(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("d/t.txt", []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Link("d/t.txt", "d/hl"); err != nil {
		t.Fatal(err)
	}
	// The alias appears in its directory listing under its OWN name, with
	// the shared size and mode.
	infos, err := m.ReadDir("d")
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 || infos[0].Name != "hl" || infos[0].Size != 3 || infos[0].Mode.Perm() != 0o600 {
		t.Errorf("alias listing = %+v", infos)
	}
	// lstat of the alias is a regular file (hard links are not symlinks).
	fi, err := m.Stat("d/hl", false)
	if err != nil || fi.Symlink || fi.IsDir || fi.Name != "hl" || fi.Size != 3 {
		t.Errorf("alias lstat = %+v (%v)", fi, err)
	}
	// Chtimes through the alias lands on the shared metadata.
	mt := time.Unix(777, 0)
	if err := m.Chtimes("d/hl", mt, mt); err != nil {
		t.Fatal(err)
	}
	if cfi, _ := m.Stat("d/t.txt", false); !cfi.ModTime.Equal(mt) {
		t.Errorf("chtimes via alias not shared: %v", cfi.ModTime)
	}
}

// TestMemResolveErrPropagation drives every op's resolve-failure arm: a
// path THROUGH a regular file (ENOTDIR) must propagate from resolution,
// for each method that resolves before acting.
func TestMemResolveErrPropagation(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("plain", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	under := "plain/under"
	if _, err := m.ReadDir(under); !errors.Is(err, errMemNotDir) {
		t.Errorf("ReadDir = %v, want ENOTDIR", err)
	}
	if err := m.Remove(under, false); !errors.Is(err, errMemNotDir) {
		t.Errorf("Remove = %v, want ENOTDIR", err)
	}
	if err := m.Rename(under, "z"); !errors.Is(err, errMemNotDir) {
		t.Errorf("Rename src = %v, want ENOTDIR", err)
	}
	if err := m.Rename("plain", under); !errors.Is(err, errMemNotDir) {
		t.Errorf("Rename dst = %v, want ENOTDIR", err)
	}
	if err := m.Symlink("t", under); !errors.Is(err, errMemNotDir) {
		t.Errorf("Symlink = %v, want ENOTDIR", err)
	}
	if err := m.Link(under, "hl"); !errors.Is(err, errMemNotDir) {
		t.Errorf("Link target = %v, want ENOTDIR", err)
	}
	if err := m.Link("plain", under); !errors.Is(err, errMemNotDir) {
		t.Errorf("Link linkPath = %v, want ENOTDIR", err)
	}
	if err := m.Chmod(under, 0o600); !errors.Is(err, errMemNotDir) {
		t.Errorf("Chmod = %v, want ENOTDIR", err)
	}
	if err := m.Chtimes(under, time.Unix(1, 0), time.Unix(1, 0)); !errors.Is(err, errMemNotDir) {
		t.Errorf("Chtimes = %v, want ENOTDIR", err)
	}
	if err := m.Truncate(under, 0); !errors.Is(err, errMemNotDir) {
		t.Errorf("Truncate = %v, want ENOTDIR", err)
	}
	if _, err := m.Stat(under, false); !errors.Is(err, errMemNotDir) {
		t.Errorf("Stat = %v, want ENOTDIR", err)
	}
}

// TestMemPermImplicitAncestors covers the model's implicit-dir tolerance
// under a non-root euid: unrecorded ancestors impose nothing (creation at
// depth succeeds), and a poked implicit directory lists without a
// permission check target.
func TestMemPermImplicitAncestors(t *testing.T) {
	m := memAsUser(1000)
	// Deep create with NO recorded ancestors: the parent walk crosses
	// unrecorded dirs and permits.
	m.Files["seed"] = []byte("s") // poke, recording nothing
	if err := m.WriteFile("a/b/c.txt", []byte("x"), 0o644); err != nil {
		t.Errorf("deep create with implicit ancestors: %v", err)
	}
	// A poked implicit dir (children but no Dirs entry) lists fine — the
	// checkPerm target is absent so no bit is enforced.
	m.Files["imp/x"] = []byte("1")
	if _, err := m.ReadDir("imp"); err != nil {
		t.Errorf("readdir of implicit dir: %v", err)
	}
}
