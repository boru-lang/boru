package capabilities

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ── OSFileOps ─────────────────────────────────────────────────────────────
// Success paths use absolute t.TempDir() paths; ResolvePath error paths use
// the injected failGetwd (from coverage_test.go) with a relative path.

func TestOSStat(t *testing.T) {
	dir := t.TempDir()
	o := &OSFileOps{}
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	if fi, err := o.Stat(file, true); err != nil || fi.Name != "f.txt" || fi.Size != 5 || fi.IsDir {
		t.Errorf("stat file (follow): %+v %v", fi, err)
	}
	if fi, err := o.Stat(file, false); err != nil || fi.Size != 5 {
		t.Errorf("stat file (lstat): %+v %v", fi, err)
	}
	if _, err := o.Stat(filepath.Join(dir, "nope"), true); err == nil {
		t.Error("expected stat error (follow, nonexistent)")
	}
	if _, err := o.Stat(filepath.Join(dir, "nope"), false); err == nil {
		t.Error("expected stat error (lstat, nonexistent)")
	}

	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if li, err := o.Stat(link, false); err != nil || !li.Symlink || li.Target != file {
		t.Errorf("lstat symlink: %+v %v", li, err)
	}
	if si, err := o.Stat(link, true); err != nil || si.Symlink || si.Size != 5 {
		t.Errorf("stat symlink (follow): %+v %v", si, err)
	}

	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "ghost"), broken); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Stat(broken, true); err == nil {
		t.Error("expected error following a broken symlink")
	}

	of := &OSFileOps{getwd: failGetwd}
	if _, err := of.Stat("rel.txt", true); err == nil {
		t.Error("expected resolve error (follow)")
	}
	if _, err := of.Stat("rel.txt", false); err == nil {
		t.Error("expected resolve error (lstat)")
	}
}

func TestOSReadDir(t *testing.T) {
	dir := t.TempDir()
	o := &OSFileOps{}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	infos, err := o.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 || infos[0].Name != "a.txt" || infos[1].Name != "sub" || !infos[1].IsDir {
		t.Errorf("readdir: %+v", infos)
	}
	if _, err := o.ReadDir(filepath.Join(dir, "nope")); err == nil {
		t.Error("expected readdir error (nonexistent)")
	}
	of := &OSFileOps{getwd: failGetwd}
	if _, err := of.ReadDir("rel"); err == nil {
		t.Error("expected readdir resolve error")
	}
}

func TestOSRemove(t *testing.T) {
	dir := t.TempDir()
	o := &OSFileOps{}
	f := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := o.Remove(f, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("file not removed")
	}
	if err := o.Remove(f, false); err == nil {
		t.Error("expected remove error (nonexistent)")
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "c"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := o.Remove(sub, false); err == nil {
		t.Error("expected error removing non-empty dir non-recursively")
	}
	if err := o.Remove(sub, true); err != nil {
		t.Fatalf("recursive remove: %v", err)
	}
	of := &OSFileOps{getwd: failGetwd}
	if err := of.Remove("rel", false); err == nil {
		t.Error("expected remove resolve error")
	}
}

func TestOSRename(t *testing.T) {
	dir := t.TempDir()
	o := &OSFileOps{}
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.WriteFile(a, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := o.Rename(a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b); err != nil {
		t.Error("rename target missing")
	}
	if err := o.Rename(filepath.Join(dir, "ghost"), b); err == nil {
		t.Error("expected rename error (nonexistent source)")
	}
	of := &OSFileOps{getwd: failGetwd}
	if err := of.Rename("relold", "/abs/new"); err == nil {
		t.Error("expected rename old-resolve error")
	}
	if err := of.Rename("/abs/old", "relnew"); err == nil {
		t.Error("expected rename new-resolve error")
	}
}

func TestOSSymlink(t *testing.T) {
	dir := t.TempDir()
	o := &OSFileOps{}
	link := filepath.Join(dir, "l")
	if err := o.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(link); got != "target" {
		t.Errorf("symlink target = %q", got)
	}
	of := &OSFileOps{getwd: failGetwd}
	if err := of.Symlink("t", "rel"); err == nil {
		t.Error("expected symlink resolve error")
	}
}

func TestOSLink(t *testing.T) {
	dir := t.TempDir()
	o := &OSFileOps{}
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := o.Link(src, filepath.Join(dir, "dst")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "dst")); string(b) != "x" {
		t.Error("hard link content mismatch")
	}
	if err := o.Link(filepath.Join(dir, "ghost"), filepath.Join(dir, "d2")); err == nil {
		t.Error("expected link error (nonexistent target)")
	}
	of := &OSFileOps{getwd: failGetwd}
	if err := of.Link("reltarget", "/abs/link"); err == nil {
		t.Error("expected link target-resolve error")
	}
	if err := of.Link("/abs/target", "rellink"); err == nil {
		t.Error("expected link link-resolve error")
	}
}

func TestOSChmod(t *testing.T) {
	skipWithoutUnixMetadata(t, "permission bits")
	dir := t.TempDir()
	o := &OSFileOps{}
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := o.Chmod(f, 0600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(f); fi.Mode().Perm() != 0600 {
		t.Errorf("chmod mode = %v", fi.Mode())
	}
	if err := o.Chmod(filepath.Join(dir, "ghost"), 0600); err == nil {
		t.Error("expected chmod error (nonexistent)")
	}
	of := &OSFileOps{getwd: failGetwd}
	if err := of.Chmod("rel", 0600); err == nil {
		t.Error("expected chmod resolve error")
	}
}

func TestOSChtimes(t *testing.T) {
	dir := t.TempDir()
	o := &OSFileOps{}
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	mt := time.Unix(1_000_000, 0)
	if err := o.Chtimes(f, mt, mt); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(f); !fi.ModTime().Equal(mt) {
		t.Errorf("chtimes mtime = %v", fi.ModTime())
	}
	if err := o.Chtimes(filepath.Join(dir, "ghost"), mt, mt); err == nil {
		t.Error("expected chtimes error (nonexistent)")
	}
	of := &OSFileOps{getwd: failGetwd}
	if err := of.Chtimes("rel", mt, mt); err == nil {
		t.Error("expected chtimes resolve error")
	}
}

func TestOSTruncate(t *testing.T) {
	dir := t.TempDir()
	o := &OSFileOps{}
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := o.Truncate(f, 3); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); string(b) != "hel" {
		t.Errorf("truncate = %q", b)
	}
	if err := o.Truncate(filepath.Join(dir, "ghost"), 0); err == nil {
		t.Error("expected truncate error (nonexistent)")
	}
	of := &OSFileOps{getwd: failGetwd}
	if err := of.Truncate("rel", 0); err == nil {
		t.Error("expected truncate resolve error")
	}
}

// ── MemFileOps ────────────────────────────────────────────────────────────

func TestMemStat(t *testing.T) {
	m := NewMem()
	mustWrite(t, m, "a/f.txt", "hello")
	if err := m.MkdirAll("a/sub", 0755); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("f.txt", "a/link"); err != nil { // relative target
		t.Fatal(err)
	}

	if fi, err := m.Stat("a/f.txt", false); err != nil || fi.Size != 5 || fi.IsDir || fi.Symlink {
		t.Errorf("stat file: %+v %v", fi, err)
	}
	if di, err := m.Stat("a/sub", false); err != nil || !di.IsDir {
		t.Errorf("stat dir: %+v %v", di, err)
	}
	if li, err := m.Stat("a/link", false); err != nil || !li.Symlink || li.Target != "f.txt" {
		t.Errorf("stat symlink (no follow): %+v %v", li, err)
	}
	if si, err := m.Stat("a/link", true); err != nil || si.Symlink || si.Size != 5 {
		t.Errorf("stat symlink (follow, relative): %+v %v", si, err)
	}
	if _, err := m.Stat("a/ghost", false); err == nil {
		t.Error("expected stat error (nonexistent)")
	}

	mustWrite(t, m, "/abs/t.txt", "yo")
	if err := m.Symlink("/abs/t.txt", "a/abslink"); err != nil {
		t.Fatal(err)
	}
	if ai, err := m.Stat("a/abslink", true); err != nil || ai.Size != 2 {
		t.Errorf("stat symlink (follow, absolute): %+v %v", ai, err)
	}

	if err := m.Symlink("cycle", "a/cycle"); err != nil { // self-referential
		t.Fatal(err)
	}
	if _, err := m.Stat("a/cycle", true); err == nil {
		t.Error("expected too-many-links error on a symlink cycle")
	}
}

func TestMemReadDir(t *testing.T) {
	m := NewMem()
	mustWrite(t, m, "d/a.txt", "x")
	if err := m.MkdirAll("d/sub", 0755); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("a.txt", "d/link"); err != nil {
		t.Fatal(err)
	}
	infos, err := m.ReadDir("d")
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 3 || infos[0].Name != "a.txt" || infos[1].Name != "link" || infos[2].Name != "sub" {
		t.Errorf("readdir d: %+v", infos)
	}
	if _, err := m.ReadDir("d/a.txt"); err == nil {
		t.Error("expected error listing a file")
	}
	if _, err := m.ReadDir("nope"); err == nil {
		t.Error("expected error listing a nonexistent dir")
	}
	if err := m.MkdirAll("empty", 0755); err != nil {
		t.Fatal(err)
	}
	if e, err := m.ReadDir("empty"); err != nil || len(e) != 0 {
		t.Errorf("empty dir: %+v %v", e, err)
	}
	// root ("." and "/") is always a valid, possibly-empty directory.
	fresh := NewMem()
	if ri, err := fresh.ReadDir("."); err != nil || len(ri) != 0 {
		t.Errorf("empty root .: %+v %v", ri, err)
	}
	if ri, err := fresh.ReadDir("/"); err != nil || len(ri) != 0 {
		t.Errorf("empty root /: %+v %v", ri, err)
	}
}

func TestMemRemove(t *testing.T) {
	m := NewMem()
	mustWrite(t, m, "f.txt", "x")
	if err := m.Remove("f.txt", false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stat("f.txt", false); err == nil {
		t.Error("file not removed")
	}
	if err := m.Remove("f.txt", false); err == nil {
		t.Error("expected error removing absent file non-recursively")
	}
	if err := m.Remove("ghost", true); err != nil {
		t.Errorf("recursive remove of absent path should be nil: %v", err)
	}
	mustWrite(t, m, "d/c.txt", "y")
	if err := m.Remove("d", false); err == nil {
		t.Error("expected error removing non-empty dir non-recursively")
	}
	if err := m.Remove("d", true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stat("d/c.txt", false); err == nil {
		t.Error("tree not removed")
	}
	if _, err := m.Stat("d", false); err == nil {
		t.Error("dir not removed")
	}
	if err := m.MkdirAll("e", 0755); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("e", false); err != nil {
		t.Fatalf("empty-dir non-recursive remove: %v", err)
	}
	if err := m.Symlink("x", "sl"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("sl", false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stat("sl", false); err == nil {
		t.Error("symlink not removed")
	}
}

func TestMemRename(t *testing.T) {
	m := NewMem()
	mustWrite(t, m, "a.txt", "x")
	if err := m.Rename("a.txt", "b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stat("b.txt", false); err != nil {
		t.Error("rename target missing")
	}
	if _, err := m.Stat("a.txt", false); err == nil {
		t.Error("rename source remains")
	}
	if err := m.Rename("ghost", "z"); err == nil {
		t.Error("expected rename error (nonexistent)")
	}
	mustWrite(t, m, "d/c.txt", "y")
	if err := m.MkdirAll("d/sub", 0755); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename("d", "e"); err != nil {
		t.Fatal(err)
	}
	if b, err := m.ReadFile("e/c.txt"); err != nil || string(b) != "y" {
		t.Errorf("dir rename child: %q %v", b, err)
	}
	if _, err := m.Stat("e/sub", false); err != nil {
		t.Error("dir rename subdir missing")
	}
	if _, err := m.Stat("d/c.txt", false); err == nil {
		t.Error("old tree remains after rename")
	}
	if err := m.Symlink("x", "sl"); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename("sl", "sl2"); err != nil {
		t.Fatal(err)
	}
	if li, err := m.Stat("sl2", false); err != nil || !li.Symlink {
		t.Errorf("renamed symlink: %+v %v", li, err)
	}
}

func TestMemSymlinkAndLink(t *testing.T) {
	m := NewMem()
	if err := m.Symlink("target", "l"); err != nil {
		t.Fatal(err)
	}
	if li, _ := m.Stat("l", false); !li.Symlink || li.Target != "target" {
		t.Errorf("symlink: %+v", li)
	}
	mustWrite(t, m, "f", "x")
	if err := m.Symlink("t", "f"); err == nil {
		t.Error("expected symlink error over existing path")
	}

	if err := m.Link("f", "dst"); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.ReadFile("dst"); string(b) != "x" {
		t.Error("hard link content mismatch")
	}
	if err := m.Link("ghost", "d2"); err == nil {
		t.Error("expected link error (nonexistent target)")
	}
	if err := m.Link("f", "dst"); err == nil {
		t.Error("expected link error over existing path")
	}
}

func TestMemChmodChtimesTruncate(t *testing.T) {
	m := NewMem()
	mustWrite(t, m, "f", "x")

	if err := m.Chmod("f", 0600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := m.Stat("f", false); fi.Mode.Perm() != 0600 {
		t.Errorf("chmod: %v", fi.Mode)
	}
	if err := m.MkdirAll("d", 0755); err != nil {
		t.Fatal(err)
	}
	if err := m.Chmod("d", 0700); err != nil {
		t.Fatal(err)
	}
	if di, _ := m.Stat("d", false); !di.IsDir || di.Mode.Perm() != 0700 {
		t.Errorf("chmod dir: %v", di.Mode)
	}
	if err := m.Chmod("f", 0); err != nil { // 0000 must be honored (no zero-overload)
		t.Fatal(err)
	}
	if fi, _ := m.Stat("f", false); fi.Mode.Perm() != 0 {
		t.Errorf("chmod 0000: %v", fi.Mode)
	}
	if err := m.Chmod("ghost", 0600); err == nil {
		t.Error("expected chmod error (nonexistent)")
	}

	// A file poked directly into Files (no Meta) exercises the default-mode
	// path (modeOf/mtimeOf nil) and metaForExisting's lazy-create branch.
	m.Files["poked"] = []byte("z")
	delete(m.Meta, "poked")
	if pfi, _ := m.Stat("poked", false); pfi.Mode.Perm() != 0644 {
		t.Errorf("poked default mode: %v", pfi.Mode)
	}
	if err := m.Chmod("poked", 0640); err != nil {
		t.Fatal(err)
	}
	if pfi, _ := m.Stat("poked", false); pfi.Mode.Perm() != 0640 {
		t.Errorf("poked chmod: %v", pfi.Mode)
	}

	mt := time.Unix(5000, 0)
	if err := m.Chtimes("f", mt, mt); err != nil {
		t.Fatal(err)
	}
	if fi, _ := m.Stat("f", false); !fi.ModTime.Equal(mt) {
		t.Errorf("chtimes: %v", fi.ModTime)
	}
	if err := m.Chtimes("ghost", mt, mt); err == nil {
		t.Error("expected chtimes error (nonexistent)")
	}

	mustWrite(t, m, "big", "hello")
	if err := m.Truncate("big", 3); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.ReadFile("big"); string(b) != "hel" {
		t.Errorf("truncate shrink: %q", b)
	}
	if err := m.Truncate("big", 6); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.ReadFile("big"); len(b) != 6 || string(b[:3]) != "hel" {
		t.Errorf("truncate grow: %q", b)
	}
	if err := m.Truncate("ghost", 0); err == nil {
		t.Error("expected truncate error (nonexistent)")
	}
	if err := m.Truncate("big", -1); err == nil {
		t.Error("expected truncate error (negative size)")
	}
}

func mustWrite(t *testing.T, m *MemFileOps, path, content string) {
	t.Helper()
	if err := m.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
