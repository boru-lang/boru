package capabilities

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// The differential parity harness: every scenario runs the SAME operation
// sequence against the real filesystem (OSFileOps under t.TempDir()) and
// MemFileOps, comparing each step's result and error CLASS, then walking
// both trees and asserting they are structurally identical (kind, content,
// permission bits, symlink targets).
//
// Documented fidelity exceptions the harness deliberately does not compare:
//   - umask: creation modes are chosen umask-stable (0644/0755/0600);
//   - directory sizes (filesystem-dependent on the real side);
//   - mtimes are compared within a coarse tolerance (both sides stamp
//     "now" during a scenario) except where Chtimes sets them exactly;
//   - permission-denial (EACCES) scenarios only run where the kernel
//     enforces them (euid != 0) — the root case is covered by the
//     mem-side seam tests in fileops_ext_perm_test.go.

// diffPair yields the two backends addressing the SAME absolute paths:
// the real side under a fresh TempDir, the mem side treating those
// absolute paths as its own keys.
func diffPair(t *testing.T) (string, FileOps, *MemFileOps) {
	t.Helper()
	root := t.TempDir()
	return root, &OSFileOps{}, NewMem()
}

// classify maps an error to a comparable class label, bridging the OS
// errno constants and MemFileOps' errMem* sentinels (whose messages mirror
// the errno strings precisely so the string fallbacks agree).
func classify(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, os.ErrNotExist):
		return "not-exist"
	case errors.Is(err, os.ErrExist):
		return "exist"
	case errors.Is(err, os.ErrPermission):
		return "permission"
	case errors.Is(err, os.ErrInvalid):
		return "invalid"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "read-only"):
		return "read-only"
	case strings.Contains(msg, "not empty"):
		return "not-empty"
	case strings.Contains(msg, "not a directory"):
		return "not-a-directory"
	case strings.Contains(msg, "is a directory"):
		return "is-a-directory"
	case strings.Contains(msg, "too many levels"):
		return "too-many-links"
	case strings.Contains(msg, "invalid argument"):
		return "invalid" // syscall.EINVAL does not errors.Is-match os.ErrInvalid
	}
	return "other:" + msg
}

// collisionClasses are the error classes POSIX permits interchangeably for
// a rename/rmdir DESTINATION collision — rename(2) and rmdir(2) explicitly
// allow EEXIST or ENOTEMPTY, and the kernel's choice further varies by
// filesystem (overlayfs/tmpfs return EEXIST where ext4 returns EISDIR /
// ENOTEMPTY). stepCollision therefore accepts any pairing WITHIN this set;
// everything outside it must still match exactly.
var collisionClasses = map[string]bool{
	"exist": true, "not-empty": true, "is-a-directory": true, "not-a-directory": true,
}

// stepCollision is step() with the destination-collision equivalence.
func stepCollision(t *testing.T, name string, osErr, memErr error) {
	t.Helper()
	co, cm := classify(osErr), classify(memErr)
	if co == cm || (collisionClasses[co] && collisionClasses[cm]) {
		return
	}
	t.Fatalf("%s: error class diverged — os=%q (%v) mem=%q (%v)", name, co, osErr, cm, memErr)
}

// step applies one named operation to both backends and asserts the error
// classes agree. Returns the two errors for scenarios that also compare
// success payloads.
func step(t *testing.T, name string, osErr, memErr error) {
	t.Helper()
	if co, cm := classify(osErr), classify(memErr); co != cm {
		t.Fatalf("%s: error class diverged — os=%q (%v) mem=%q (%v)", name, co, osErr, cm, memErr)
	}
}

// nodeSummary is one tree entry in the final structural comparison.
type nodeSummary struct {
	Kind    string // "file" | "dir" | "symlink"
	Content string // file bytes ("" for dir/symlink)
	Perm    os.FileMode
	Target  string // symlink target
}

// walkTree builds the full relative-path → summary map for a backend,
// using only FileOps methods (lstat view, no symlink following) so both
// sides are observed through the identical API.
func walkTree(t *testing.T, ops FileOps, root string) map[string]nodeSummary {
	t.Helper()
	out := map[string]nodeSummary{}
	var walk func(dir string)
	walk = func(dir string) {
		infos, err := ops.ReadDir(dir)
		if err != nil {
			t.Fatalf("walk ReadDir %s: %v", dir, err)
		}
		for _, fi := range infos {
			p := filepath.Join(dir, fi.Name)
			rel, _ := filepath.Rel(root, p)
			switch {
			case fi.Symlink:
				// ReadDir entries do not carry the target on the OS side
				// (only Stat(follow=false) runs Readlink) — fetch it
				// through the same API on both backends.
				lfi, lerr := ops.Stat(p, false)
				if lerr != nil {
					t.Fatalf("walk lstat %s: %v", p, lerr)
				}
				out[rel] = nodeSummary{Kind: "symlink", Perm: 0o777, Target: lfi.Target}
			case fi.IsDir:
				out[rel] = nodeSummary{Kind: "dir", Perm: fi.Mode.Perm()}
				walk(p)
			default:
				data, rerr := ops.ReadFile(p)
				if rerr != nil {
					t.Fatalf("walk ReadFile %s: %v", p, rerr)
				}
				out[rel] = nodeSummary{Kind: "file", Content: string(data), Perm: fi.Mode.Perm()}
			}
		}
	}
	walk(root)
	return out
}

// compareTrees asserts the two walks are identical.
func compareTrees(t *testing.T, osOps FileOps, mem *MemFileOps, root string) {
	t.Helper()
	osTree := walkTree(t, osOps, root)
	memTree := walkTree(t, mem, root)
	keys := map[string]bool{}
	for k := range osTree {
		keys[k] = true
	}
	for k := range memTree {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		o, oOK := osTree[k]
		m, mOK := memTree[k]
		if oOK != mOK {
			t.Errorf("tree diverged at %q: os-present=%v mem-present=%v", k, oOK, mOK)
			continue
		}
		if o != m {
			t.Errorf("tree diverged at %q:\n  os =%+v\n  mem=%+v", k, o, m)
		}
	}
}

func TestDifferentialWriteReadStat(t *testing.T) {
	skipWithoutUnixMetadata(t, "file modes")
	root, osOps, mem := diffPair(t)
	p := filepath.Join(root, "d", "a.txt")

	step(t, "write", osOps.WriteFile(p, []byte("hello"), 0o644), mem.WriteFile(p, []byte("hello"), 0o644))
	ob, oe := osOps.ReadFile(p)
	mb, me := mem.ReadFile(p)
	step(t, "read", oe, me)
	if !bytes.Equal(ob, mb) {
		t.Errorf("content diverged: os=%q mem=%q", ob, mb)
	}
	ofi, oe := osOps.Stat(p, true)
	mfi, me := mem.Stat(p, true)
	step(t, "stat", oe, me)
	if ofi.Name != mfi.Name || ofi.Size != mfi.Size || ofi.Mode.Perm() != mfi.Mode.Perm() || ofi.IsDir != mfi.IsDir {
		t.Errorf("stat diverged: os=%+v mem=%+v", ofi, mfi)
	}
	// A second write keeps the original mode (open(2) semantics) and moves
	// mtime — on BOTH sides.
	step(t, "rewrite", osOps.WriteFile(p, []byte("hi2"), 0o600), mem.WriteFile(p, []byte("hi2"), 0o600))
	ofi, _ = osOps.Stat(p, true)
	mfi, _ = mem.Stat(p, true)
	if ofi.Mode.Perm() != 0o644 || mfi.Mode.Perm() != 0o644 {
		t.Errorf("rewrite mode: os=%v mem=%v, want both 0644 (mode kept)", ofi.Mode.Perm(), mfi.Mode.Perm())
	}
	// Reading / writing / stat-ing an absent path and reading a directory.
	ghost := filepath.Join(root, "ghost")
	_, oe = osOps.ReadFile(ghost)
	_, me = mem.ReadFile(ghost)
	step(t, "read-absent", oe, me)
	_, oe = osOps.Stat(ghost, true)
	_, me = mem.Stat(ghost, true)
	step(t, "stat-absent", oe, me)
	dir := filepath.Join(root, "d")
	_, oe = osOps.ReadFile(dir)
	_, me = mem.ReadFile(dir)
	step(t, "read-dir", oe, me)
	step(t, "write-over-dir", osOps.WriteFile(dir, []byte("x"), 0o644), mem.WriteFile(dir, []byte("x"), 0o644))
	// A path THROUGH a regular file is ENOTDIR on both sides.
	under := filepath.Join(p, "below")
	_, oe = osOps.ReadFile(under)
	_, me = mem.ReadFile(under)
	step(t, "read-under-file", oe, me)
	compareTrees(t, osOps, mem, root)
}

func TestDifferentialMkdirReadDir(t *testing.T) {
	skipWithoutUnixMetadata(t, "directory modes")
	root, osOps, mem := diffPair(t)
	sub := filepath.Join(root, "a", "b")
	step(t, "mkdir", osOps.MkdirAll(sub, 0o755), mem.MkdirAll(sub, 0o755))
	step(t, "mkdir-again", osOps.MkdirAll(sub, 0o700), mem.MkdirAll(sub, 0o700))
	// Idempotent re-mkdir keeps the original mode on both sides.
	ofi, _ := osOps.Stat(sub, true)
	mfi, _ := mem.Stat(sub, true)
	if ofi.Mode.Perm() != 0o755 || mfi.Mode.Perm() != 0o755 {
		t.Errorf("re-mkdir mode: os=%v mem=%v, want both 0755", ofi.Mode.Perm(), mfi.Mode.Perm())
	}
	step(t, "mkdir-over-file",
		func() error {
			p := filepath.Join(root, "f.txt")
			if err := osOps.WriteFile(p, []byte("x"), 0o644); err != nil {
				return err
			}
			return osOps.MkdirAll(filepath.Join(p, "sub"), 0o755)
		}(),
		func() error {
			p := filepath.Join(root, "f.txt")
			if err := mem.WriteFile(p, []byte("x"), 0o644); err != nil {
				return err
			}
			return mem.MkdirAll(filepath.Join(p, "sub"), 0o755)
		}())

	for _, name := range []string{"z.txt", "a.txt", "m.txt"} {
		p := filepath.Join(root, "a", name)
		step(t, "seed "+name, osOps.WriteFile(p, []byte(name), 0o644), mem.WriteFile(p, []byte(name), 0o644))
	}
	oinfos, oe := osOps.ReadDir(filepath.Join(root, "a"))
	minfos, me := mem.ReadDir(filepath.Join(root, "a"))
	step(t, "readdir", oe, me)
	if len(oinfos) != len(minfos) {
		t.Fatalf("readdir length: os=%d mem=%d", len(oinfos), len(minfos))
	}
	for i := range oinfos {
		if oinfos[i].Name != minfos[i].Name || oinfos[i].IsDir != minfos[i].IsDir {
			t.Errorf("readdir[%d]: os=%+v mem=%+v", i, oinfos[i], minfos[i])
		}
	}
	_, oe = osOps.ReadDir(filepath.Join(root, "nope"))
	_, me = mem.ReadDir(filepath.Join(root, "nope"))
	step(t, "readdir-absent", oe, me)
	_, oe = osOps.ReadDir(filepath.Join(root, "f.txt"))
	_, me = mem.ReadDir(filepath.Join(root, "f.txt"))
	step(t, "readdir-file", oe, me)
	compareTrees(t, osOps, mem, root)
}

func TestDifferentialRemove(t *testing.T) {
	root, osOps, mem := diffPair(t)
	seed := func(rel, content string) {
		p := filepath.Join(root, rel)
		step(t, "seed "+rel, osOps.WriteFile(p, []byte(content), 0o644), mem.WriteFile(p, []byte(content), 0o644))
	}
	seed("f.txt", "x")
	seed("d/a.txt", "1")
	seed("d/sub/b.txt", "2")

	step(t, "remove-file", osOps.Remove(filepath.Join(root, "f.txt"), false), mem.Remove(filepath.Join(root, "f.txt"), false))
	step(t, "remove-absent", osOps.Remove(filepath.Join(root, "f.txt"), false), mem.Remove(filepath.Join(root, "f.txt"), false))
	step(t, "removeall-absent", osOps.Remove(filepath.Join(root, "f.txt"), true), mem.Remove(filepath.Join(root, "f.txt"), true))
	stepCollision(t, "remove-nonempty", osOps.Remove(filepath.Join(root, "d"), false), mem.Remove(filepath.Join(root, "d"), false))
	step(t, "remove-tree", osOps.Remove(filepath.Join(root, "d"), true), mem.Remove(filepath.Join(root, "d"), true))
	compareTrees(t, osOps, mem, root)
}

func TestDifferentialRename(t *testing.T) {
	skipWithoutUnixMetadata(t, "rename-over-directory errors")
	root, osOps, mem := diffPair(t)
	both := func(name string, f func(FileOps) error) {
		step(t, name, f(osOps), f(mem))
	}
	collide := func(name string, f func(FileOps) error) {
		stepCollision(t, name, f(osOps), f(mem))
	}
	j := func(rel string) string { return filepath.Join(root, rel) }
	both("seed src", func(o FileOps) error { return o.WriteFile(j("src.txt"), []byte("s"), 0o644) })
	both("seed dstf", func(o FileOps) error { return o.WriteFile(j("dstf.txt"), []byte("old"), 0o644) })
	both("seed tree", func(o FileOps) error { return o.WriteFile(j("tree/x/y.txt"), []byte("y"), 0o644) })
	both("seed full", func(o FileOps) error { return o.WriteFile(j("full/q.txt"), []byte("q"), 0o644) })
	both("mkdir empty", func(o FileOps) error { return o.MkdirAll(j("empty"), 0o755) })

	both("rename file->file", func(o FileOps) error { return o.Rename(j("src.txt"), j("dstf.txt")) })
	both("rename absent", func(o FileOps) error { return o.Rename(j("ghost"), j("z")) })
	collide("rename file->dir", func(o FileOps) error { return o.Rename(j("dstf.txt"), j("empty")) })
	collide("rename dir->file", func(o FileOps) error { return o.Rename(j("tree"), j("dstf.txt")) })
	collide("rename dir->nonempty", func(o FileOps) error { return o.Rename(j("tree"), j("full")) })
	both("rename tree", func(o FileOps) error { return o.Rename(j("tree"), j("moved")) })
	compareTrees(t, osOps, mem, root)
}

// TestDifferentialRenameDirOverEmpty covers rename(2)'s replace-an-empty-
// directory rule. POSIX specifies success, and MemFileOps implements it —
// but some real filesystems refuse (overlayfs returns EEXIST), so the
// real side is PROBED first and the comparison skips on a host that
// cannot express the rule (mem stays POSIX-correct either way).
func TestDifferentialRenameDirOverEmpty(t *testing.T) {
	root, osOps, mem := diffPair(t)
	j := func(rel string) string { return filepath.Join(root, rel) }
	step(t, "seed tree", osOps.WriteFile(j("tree/x.txt"), []byte("x"), 0o644), mem.WriteFile(j("tree/x.txt"), []byte("x"), 0o644))
	step(t, "seed empty", osOps.MkdirAll(j("empty"), 0o755), mem.MkdirAll(j("empty"), 0o755))
	if err := osOps.Rename(j("tree"), j("empty")); err != nil {
		t.Skipf("host filesystem cannot replace an empty directory via rename (%v) — POSIX rule asserted mem-side only", err)
	}
	if err := mem.Rename(j("tree"), j("empty")); err != nil {
		t.Fatalf("mem refused the POSIX empty-dir replace: %v", err)
	}
	compareTrees(t, osOps, mem, root)
}

func TestDifferentialSymlink(t *testing.T) {
	skipWithoutUnixMetadata(t, "symlink modes")
	root, osOps, mem := diffPair(t)
	both := func(name string, f func(FileOps) error) {
		step(t, name, f(osOps), f(mem))
	}
	j := func(rel string) string { return filepath.Join(root, rel) }
	both("seed", func(o FileOps) error { return o.WriteFile(j("t.txt"), []byte("yo"), 0o644) })
	both("symlink", func(o FileOps) error { return o.Symlink("t.txt", j("ln")) })
	both("symlink-exists", func(o FileOps) error { return o.Symlink("t.txt", j("ln")) })
	both("dangling", func(o FileOps) error { return o.Symlink("nope.txt", j("dang")) })
	// Read/stat through the link; lstat the link itself.
	for _, ops := range []FileOps{osOps, mem} {
		b, err := ops.ReadFile(j("ln"))
		if err != nil || string(b) != "yo" {
			t.Errorf("read through symlink (%T) = %q, %v", ops, b, err)
		}
		fi, err := ops.Stat(j("ln"), false)
		if err != nil || !fi.Symlink || fi.Target != "t.txt" || fi.Size != int64(len("t.txt")) {
			t.Errorf("lstat symlink (%T) = %+v, %v", ops, fi, err)
		}
		fi, err = ops.Stat(j("ln"), true)
		if err != nil || fi.Symlink || fi.Size != 2 {
			t.Errorf("stat through symlink (%T) = %+v, %v", ops, fi, err)
		}
	}
	// Writing through a symlink lands on the target (both sides).
	both("write-through", func(o FileOps) error { return o.WriteFile(j("ln"), []byte("new"), 0o644) })
	for _, ops := range []FileOps{osOps, mem} {
		if b, _ := ops.ReadFile(j("t.txt")); string(b) != "new" {
			t.Errorf("write-through target (%T) = %q", ops, b)
		}
	}
	// Dangling / cycle behaviour.
	_, oe := osOps.Stat(j("dang"), true)
	_, me := mem.Stat(j("dang"), true)
	step(t, "stat-dangling", oe, me)
	both("cycle", func(o FileOps) error { return o.Symlink("cyc", j("cyc")) })
	_, oe = osOps.Stat(j("cyc"), true)
	_, me = mem.Stat(j("cyc"), true)
	step(t, "stat-cycle", oe, me)
	// A symlinked DIRECTORY: ReadDir follows on both sides.
	both("seed dir", func(o FileOps) error { return o.WriteFile(j("dir/in.txt"), []byte("z"), 0o644) })
	both("dirlink", func(o FileOps) error { return o.Symlink("dir", j("dl")) })
	oinfos, oe := osOps.ReadDir(j("dl"))
	minfos, me := mem.ReadDir(j("dl"))
	step(t, "readdir-through-link", oe, me)
	if len(oinfos) != 1 || len(minfos) != 1 || oinfos[0].Name != minfos[0].Name {
		t.Errorf("readdir through symlink: os=%v mem=%v", oinfos, minfos)
	}
	// Path THROUGH the dir symlink resolves on both sides.
	for _, ops := range []FileOps{osOps, mem} {
		if b, err := ops.ReadFile(j("dl/in.txt")); err != nil || string(b) != "z" {
			t.Errorf("read through dir symlink (%T) = %q, %v", ops, b, err)
		}
	}
	// Removing the symlink removes the LINK, not the target.
	both("remove-link", func(o FileOps) error { return o.Remove(j("ln"), false) })
	for _, ops := range []FileOps{osOps, mem} {
		if _, err := ops.Stat(j("t.txt"), true); err != nil {
			t.Errorf("target vanished with its symlink (%T): %v", ops, err)
		}
	}
	compareTrees(t, osOps, mem, root)
}

func TestDifferentialHardLink(t *testing.T) {
	skipWithoutUnixMetadata(t, "shared permission bits")
	root, osOps, mem := diffPair(t)
	both := func(name string, f func(FileOps) error) {
		step(t, name, f(osOps), f(mem))
	}
	j := func(rel string) string { return filepath.Join(root, rel) }
	both("seed", func(o FileOps) error { return o.WriteFile(j("t.txt"), []byte("body"), 0o644) })
	both("link", func(o FileOps) error { return o.Link(j("t.txt"), j("hl")) })
	both("link-exists", func(o FileOps) error { return o.Link(j("t.txt"), j("hl")) })
	both("link-absent", func(o FileOps) error { return o.Link(j("ghost"), j("hl2")) })
	// THE inode contract: writing through either name is visible via the other.
	both("write-canonical", func(o FileOps) error { return o.WriteFile(j("t.txt"), []byte("v2"), 0o644) })
	for _, ops := range []FileOps{osOps, mem} {
		if b, _ := ops.ReadFile(j("hl")); string(b) != "v2" {
			t.Errorf("hard link stale after canonical write (%T): %q", ops, b)
		}
	}
	both("write-alias", func(o FileOps) error { return o.WriteFile(j("hl"), []byte("v3"), 0o644) })
	for _, ops := range []FileOps{osOps, mem} {
		if b, _ := ops.ReadFile(j("t.txt")); string(b) != "v3" {
			t.Errorf("canonical stale after alias write (%T): %q", ops, b)
		}
	}
	// Chmod through one name shows through the other (shared inode metadata).
	both("chmod-alias", func(o FileOps) error { return o.Chmod(j("hl"), 0o600) })
	for _, ops := range []FileOps{osOps, mem} {
		if fi, _ := ops.Stat(j("t.txt"), true); fi.Mode.Perm() != 0o600 {
			t.Errorf("chmod via alias not shared (%T): %v", ops, fi.Mode.Perm())
		}
	}
	// Removing the ORIGINAL name keeps the content alive under the alias.
	both("remove-canonical", func(o FileOps) error { return o.Remove(j("t.txt"), false) })
	for _, ops := range []FileOps{osOps, mem} {
		if b, err := ops.ReadFile(j("hl")); err != nil || string(b) != "v3" {
			t.Errorf("content died with canonical name (%T): %q, %v", ops, b, err)
		}
	}
	// Hard-linking a DIRECTORY is refused; hard-linking a SYMLINK links the
	// link object itself (linkat no-follow).
	both("seed dir", func(o FileOps) error { return o.MkdirAll(j("d"), 0o755) })
	both("link-dir", func(o FileOps) error { return o.Link(j("d"), j("dhl")) })
	both("seed sym", func(o FileOps) error { return o.Symlink("hl", j("sym")) })
	both("link-sym", func(o FileOps) error { return o.Link(j("sym"), j("symhl")) })
	ofi, oe := osOps.Stat(j("symhl"), false)
	mfi, me := mem.Stat(j("symhl"), false)
	step(t, "lstat-linked-sym", oe, me)
	if !ofi.Symlink || !mfi.Symlink || ofi.Target != mfi.Target {
		t.Errorf("hard-linked symlink: os=%+v mem=%+v", ofi, mfi)
	}
	compareTrees(t, osOps, mem, root)
}

func TestDifferentialChmodChtimesTruncate(t *testing.T) {
	skipWithoutUnixMetadata(t, "permission bits")
	root, osOps, mem := diffPair(t)
	both := func(name string, f func(FileOps) error) {
		step(t, name, f(osOps), f(mem))
	}
	j := func(rel string) string { return filepath.Join(root, rel) }
	both("seed", func(o FileOps) error { return o.WriteFile(j("f"), []byte("hello"), 0o644) })

	both("chmod", func(o FileOps) error { return o.Chmod(j("f"), 0o600) })
	both("chmod-absent", func(o FileOps) error { return o.Chmod(j("ghost"), 0o600) })
	mt := time.Unix(1234567890, 0)
	both("chtimes", func(o FileOps) error { return o.Chtimes(j("f"), mt, mt) })
	both("chtimes-absent", func(o FileOps) error { return o.Chtimes(j("ghost"), mt, mt) })
	for _, ops := range []FileOps{osOps, mem} {
		fi, _ := ops.Stat(j("f"), true)
		if !fi.ModTime.Equal(mt) {
			t.Errorf("chtimes mtime (%T) = %v, want %v", ops, fi.ModTime, mt)
		}
		if fi.Mode.Perm() != 0o600 {
			t.Errorf("chmod perm (%T) = %v", ops, fi.Mode.Perm())
		}
	}
	both("shrink", func(o FileOps) error { return o.Truncate(j("f"), 2) })
	both("grow", func(o FileOps) error { return o.Truncate(j("f"), 4) })
	for _, ops := range []FileOps{osOps, mem} {
		if b, _ := ops.ReadFile(j("f")); !bytes.Equal(b, []byte{'h', 'e', 0, 0}) {
			t.Errorf("truncate content (%T) = %v", ops, b)
		}
		// truncate(2) moves mtime forward past the explicit chtimes stamp.
		if fi, _ := ops.Stat(j("f"), true); fi.ModTime.Equal(mt) {
			t.Errorf("truncate did not update mtime (%T)", ops)
		}
	}
	both("negative", func(o FileOps) error { return o.Truncate(j("f"), -1) })
	both("truncate-absent", func(o FileOps) error { return o.Truncate(j("ghost"), 1) })
	both("truncate-dir", func(o FileOps) error { return o.Truncate(root, 1) })
	compareTrees(t, osOps, mem, root)
}

// TestDifferentialPermissionDenied exercises EACCES parity. The kernel
// bypasses permission checks for root, and MemFileOps mirrors that via
// euid — so the denial arm only runs where the kernel enforces it.
func TestDifferentialPermissionDenied(t *testing.T) {
	skipWithoutUnixMetadata(t, "mode-enforced access")
	if os.Geteuid() == 0 {
		t.Skip("running as root: the kernel does not enforce permission bits (mem's non-root arm is covered by the euid-seam tests)")
	}
	root, osOps, mem := diffPair(t)
	j := func(rel string) string { return filepath.Join(root, rel) }
	step(t, "seed", osOps.WriteFile(j("f"), []byte("x"), 0o644), mem.WriteFile(j("f"), []byte("x"), 0o644))
	step(t, "chmod-0", osOps.Chmod(j("f"), 0), mem.Chmod(j("f"), 0))
	_, oe := osOps.ReadFile(j("f"))
	_, me := mem.ReadFile(j("f"))
	step(t, "read-denied", oe, me)
	if classify(oe) != "permission" {
		t.Errorf("expected EACCES, got %v", oe)
	}
	step(t, "write-denied", osOps.WriteFile(j("f"), []byte("y"), 0o644), mem.WriteFile(j("f"), []byte("y"), 0o644))
}

// TestDifferentialErrClassTable pins classify() itself: each label must be
// reachable and distinct, and an unknown error degrades to its message.
func TestDifferentialErrClassTable(t *testing.T) {
	cases := map[string]error{
		"ok":              nil,
		"not-exist":       os.ErrNotExist,
		"exist":           os.ErrExist,
		"permission":      os.ErrPermission,
		"invalid":         os.ErrInvalid,
		"not-empty":       errMemNotEmpty,
		"not-a-directory": errMemNotDir,
		"is-a-directory":  errMemIsDir,
		"too-many-links":  errMemTooManyLinks,
		"read-only":       errReadOnlyFS,
		"other:boom":      errors.New("boom"),
	}
	for want, err := range cases {
		if got := classify(err); got != want {
			t.Errorf("classify(%v) = %q, want %q", err, got, want)
		}
	}
	// A wrapped PathError classifies by its underlying sentinel.
	wrapped := &os.PathError{Op: "open", Path: "x", Err: os.ErrNotExist}
	if got := classify(wrapped); got != "not-exist" {
		t.Errorf("classify(wrapped) = %q", got)
	}
	_ = fmt.Sprintf // keep fmt imported alongside future scenario labels
}
