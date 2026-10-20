package capabilities

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fileops_meta_test.go — ownership (Chown / FileInfo.UID/GID) and
// extended-attribute coverage: mem-model unit tests plus the
// differential scenarios that keep mem and the real kernel in lockstep.

// xattrSupported probes whether the real filesystem under dir accepts
// user xattrs — CI filesystems (some overlayfs/tmpfs configs) and
// non-unix platforms don't, and every real-side xattr assertion must
// probe-and-skip rather than fail there.
func xattrSupported(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "xattr-probe")
	if err := os.WriteFile(probe, []byte("p"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := osXattrSet(probe, xattrHostName("probe"), []byte("1"))
	// The probe must not survive into the scenario's tree comparison.
	if rerr := os.Remove(probe); rerr != nil {
		t.Fatal(rerr)
	}
	return err == nil
}

// xattrHostName maps a plain attribute name onto the host's namespace —
// the same mapping the io word layer applies (user.<name> on Linux, the
// raw name elsewhere).
func xattrHostName(name string) string {
	if XattrNamespacePrefix != "" {
		return XattrNamespacePrefix + name
	}
	return name
}

func TestDifferentialChownNoOpAndStatOwner(t *testing.T) {
	skipWithoutUnixMetadata(t, "ownership")
	root, osOps, mem := diffPair(t)
	j := func(rel string) string { return filepath.Join(root, rel) }
	step(t, "seed", osOps.WriteFile(j("f"), []byte("x"), 0o644), mem.WriteFile(j("f"), []byte("x"), 0o644))

	// The -1/-1 no-op succeeds on both backends against any owned path.
	step(t, "chown-noop", osOps.Chown(j("f"), -1, -1, true), mem.Chown(j("f"), -1, -1, true))
	step(t, "chown-absent", osOps.Chown(j("ghost"), -1, -1, true), mem.Chown(j("ghost"), -1, -1, true))

	// Stat ownership parity: a file this process created is owned by this
	// process on the real side; mem stamps the same identity.
	ofi, _ := osOps.Stat(j("f"), true)
	mfi, _ := mem.Stat(j("f"), true)
	if ofi.UID != mfi.UID || ofi.GID != mfi.GID {
		t.Errorf("stat ownership diverged: os %d/%d vs mem %d/%d", ofi.UID, ofi.GID, mfi.UID, mfi.GID)
	}
	if ofi.UID != os.Geteuid() || ofi.GID != os.Getegid() {
		t.Errorf("created file not owned by creator: %d/%d vs %d/%d", ofi.UID, ofi.GID, os.Geteuid(), os.Getegid())
	}
	compareTrees(t, osOps, mem, root)
}

func TestDifferentialChownDenied(t *testing.T) {
	skipWithoutUnixMetadata(t, "ownership")
	if os.Geteuid() == 0 {
		t.Skip("running as root: the kernel permits any chown (mem's non-root arm is covered by TestMemChownRules)")
	}
	root, osOps, mem := diffPair(t)
	j := func(rel string) string { return filepath.Join(root, rel) }
	step(t, "seed", osOps.WriteFile(j("f"), []byte("x"), 0o644), mem.WriteFile(j("f"), []byte("x"), 0o644))
	oe := osOps.Chown(j("f"), 0, -1, true)
	me := mem.Chown(j("f"), 0, -1, true)
	step(t, "chown-to-root-denied", oe, me)
	if classify(oe) != "permission" {
		t.Errorf("expected EPERM class, got %v", oe)
	}
}

func TestDifferentialXattr(t *testing.T) {
	root, osOps, mem := diffPair(t)
	if !xattrSupported(t, root) {
		t.Skip("filesystem does not support user xattrs")
	}
	j := func(rel string) string { return filepath.Join(root, rel) }
	n := xattrHostName
	step(t, "seed", osOps.WriteFile(j("f"), []byte("x"), 0o644), mem.WriteFile(j("f"), []byte("x"), 0o644))

	step(t, "set", osOps.XattrSet(j("f"), n("note"), []byte("hi")), mem.XattrSet(j("f"), n("note"), []byte("hi")))
	step(t, "set2", osOps.XattrSet(j("f"), n("tag"), []byte{0xFF, 0x00}), mem.XattrSet(j("f"), n("tag"), []byte{0xFF, 0x00}))

	ov, oe := osOps.XattrGet(j("f"), n("note"))
	mv, me := mem.XattrGet(j("f"), n("note"))
	step(t, "get", oe, me)
	if !bytes.Equal(ov, mv) || string(ov) != "hi" {
		t.Errorf("xattr get diverged: os %q vs mem %q", ov, mv)
	}

	ol, oe := osOps.XattrList(j("f"))
	ml, me := mem.XattrList(j("f"))
	step(t, "list", oe, me)
	// The real side may carry extra system attributes (selinux); require
	// the mem names to be a SUBSET-equal on ours: both contain note+tag.
	has := func(xs []string, want string) bool {
		for _, x := range xs {
			if x == want {
				return true
			}
		}
		return false
	}
	for _, want := range []string{n("note"), n("tag")} {
		if !has(ol, want) || !has(ml, want) {
			t.Errorf("xattr list missing %q: os %v mem %v", want, ol, ml)
		}
	}

	// The absent-attribute error CLASS must agree (ENODATA / ENOATTR).
	_, oe = osOps.XattrGet(j("f"), n("ghost"))
	_, me = mem.XattrGet(j("f"), n("ghost"))
	step(t, "get-absent", oe, me)
	step(t, "remove", osOps.XattrRemove(j("f"), n("note")), mem.XattrRemove(j("f"), n("note")))
	step(t, "remove-absent", osOps.XattrRemove(j("f"), n("note")), mem.XattrRemove(j("f"), n("note")))
	step(t, "get-on-absent-path", ignoreVal(osOps.XattrGet(j("ghost"), n("a"))), ignoreVal(mem.XattrGet(j("ghost"), n("a"))))
	step(t, "list-on-absent-path", ignoreVals(osOps.XattrList(j("ghost"))), ignoreVals(mem.XattrList(j("ghost"))))
	compareTrees(t, osOps, mem, root)
}

func ignoreVal(_ []byte, err error) error    { return err }
func ignoreVals(_ []string, err error) error { return err }

func TestMemChownRules(t *testing.T) {
	m := NewMem()
	m.euidFn = func() int { return 0 }
	m.egidFn = func() int { return 0 }
	if err := m.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Root re-owns freely; a -1 arm leaves that id in place.
	if err := m.Chown("f", 42, -1, true); err != nil {
		t.Fatalf("root chown: %v", err)
	}
	fi, _ := m.Stat("f", true)
	if fi.UID != 42 || fi.GID != 0 {
		t.Errorf("after chown 42/-1: %d/%d", fi.UID, fi.GID)
	}
	if err := m.Chown("f", -1, 7, true); err != nil {
		t.Fatalf("root chgrp: %v", err)
	}
	if fi, _ = m.Stat("f", true); fi.GID != 7 || fi.UID != 42 {
		t.Errorf("after chown -1/7: %d/%d", fi.UID, fi.GID)
	}

	// Non-root: any actual change is EPERM; the -1/-1 no-op succeeds.
	m.euidFn = func() int { return 1000 }
	if err := m.Chown("f", 1000, -1, true); !errors.Is(err, os.ErrPermission) {
		t.Errorf("non-root chown: %v, want EPERM", err)
	}
	if err := m.Chown("f", -1, -1, true); err != nil {
		t.Errorf("non-root -1/-1 no-op: %v", err)
	}
	// Absent path.
	if err := m.Chown("ghost", -1, -1, true); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("chown absent: %v", err)
	}

	// lchown (follow=false) re-owns the SYMLINK, not its target.
	m.euidFn = func() int { return 0 }
	if err := m.Symlink("f", "sl"); err != nil {
		t.Fatal(err)
	}
	if err := m.Chown("sl", 5, 5, false); err != nil {
		t.Fatalf("lchown: %v", err)
	}
	sfi, _ := m.Stat("sl", false)
	tfi, _ := m.Stat("sl", true)
	if sfi.UID != 5 || tfi.UID != 42 {
		t.Errorf("lchown leaked: link %d target %d (want 5 / 42)", sfi.UID, tfi.UID)
	}

	// An unstamped meta (test-poked file) is stamped before the partial
	// update, so a lone -1 arm keeps the current default identity.
	m.Files["poked"] = []byte("y")
	if err := m.Chown("poked", 9, -1, true); err != nil {
		t.Fatalf("chown poked: %v", err)
	}
	if fi, _ = m.Stat("poked", true); fi.UID != 9 || fi.GID != 0 {
		t.Errorf("poked ownership: %d/%d, want 9/0", fi.UID, fi.GID)
	}
}

func TestMemXattrRules(t *testing.T) {
	m := NewMem()
	m.euidFn = func() int { return 1000 }
	m.egidFn = func() int { return 1000 }
	if err := m.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Round trip, list ordering, and value isolation (mutating the caller's
	// slice after set must not leak in).
	payload := []byte("v1")
	if err := m.XattrSet("f", "b", payload); err != nil {
		t.Fatal(err)
	}
	payload[0] = 'X'
	if err := m.XattrSet("f", "a", []byte("v2")); err != nil {
		t.Fatal(err)
	}
	names, err := m.XattrList("f")
	if err != nil || len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("xattr list = %v (%v), want [a b]", names, err)
	}
	v, err := m.XattrGet("f", "b")
	if err != nil || string(v) != "v1" {
		t.Errorf("xattr b = %q (%v), want v1 (isolated from caller mutation)", v, err)
	}
	// The returned slice is a copy too.
	v[0] = 'Z'
	if v2, _ := m.XattrGet("f", "b"); string(v2) != "v1" {
		t.Errorf("get-returned slice aliases storage: %q", v2)
	}

	// Absent attribute / absent path.
	if _, err := m.XattrGet("f", "ghost"); !errors.Is(err, errXattrAbsent) {
		t.Errorf("absent attr: %v, want errXattrAbsent", err)
	}
	if err := m.XattrRemove("f", "ghost"); !errors.Is(err, errXattrAbsent) {
		t.Errorf("remove absent attr: %v", err)
	}
	if _, err := m.XattrGet("ghost", "a"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent path get: %v", err)
	}
	if err := m.XattrSet("ghost", "a", nil); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent path set: %v", err)
	}
	if _, err := m.XattrList("ghost"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent path list: %v", err)
	}
	if err := m.XattrRemove("ghost", "a"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent path remove: %v", err)
	}

	// Permission enforcement: read bit gates get/list, write bit gates
	// set/remove (euid is non-root here).
	if err := m.Chmod("f", 0o200); err != nil { // write-only
		t.Fatal(err)
	}
	if _, err := m.XattrGet("f", "a"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("get on write-only file: %v", err)
	}
	if _, err := m.XattrList("f"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("list on write-only file: %v", err)
	}
	if err := m.Chmod("f", 0o400); err != nil { // read-only
		t.Fatal(err)
	}
	if err := m.XattrSet("f", "c", []byte("x")); !errors.Is(err, os.ErrPermission) {
		t.Errorf("set on read-only file: %v", err)
	}
	if err := m.XattrRemove("f", "a"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("remove on read-only file: %v", err)
	}

	// Hard links share attributes (inode fidelity), and removal through
	// one name is visible through the other.
	if err := m.Chmod("f", 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Link("f", "hl"); err != nil {
		t.Fatal(err)
	}
	if v, err := m.XattrGet("hl", "a"); err != nil || string(v) != "v2" {
		t.Errorf("hard link xattr = %q (%v)", v, err)
	}
	if err := m.XattrRemove("hl", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.XattrGet("f", "a"); !errors.Is(err, errXattrAbsent) {
		t.Errorf("removal through alias not shared: %v", err)
	}
}

// TestMemOwnershipStamping pins creation-time stamping across entry kinds
// and the read-time default for meta-less pokes.
func TestMemOwnershipStamping(t *testing.T) {
	m := NewMem()
	m.euidFn = func() int { return 7 }
	m.egidFn = func() int { return 8 }
	if err := m.WriteFile("d/f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("d/f", "sl"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"d", "d/f"} {
		if fi, _ := m.Stat(p, false); fi.UID != 7 || fi.GID != 8 {
			t.Errorf("%s ownership = %d/%d, want 7/8", p, fi.UID, fi.GID)
		}
	}
	if fi, _ := m.Stat("sl", false); fi.UID != 7 || fi.GID != 8 {
		t.Errorf("symlink ownership = %d/%d", fi.UID, fi.GID)
	}
	// Identity switch: NEW entries take the new identity; existing keep theirs.
	m.euidFn = func() int { return 9 }
	m.egidFn = func() int { return 9 }
	if err := m.WriteFile("d/g", []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := m.Stat("d/g", true); fi.UID != 9 {
		t.Errorf("new file ownership = %d, want 9", fi.UID)
	}
	if fi, _ := m.Stat("d/f", true); fi.UID != 7 {
		t.Errorf("existing file re-owned to %d on identity switch", fi.UID)
	}
	// A meta-less poked file defaults to the CURRENT identity at read.
	m.Files["poked"] = []byte("z")
	if fi, _ := m.Stat("poked", true); fi.UID != 9 || fi.GID != 9 {
		t.Errorf("poked default ownership = %d/%d, want 9/9", fi.UID, fi.GID)
	}
	// Hard links share the canonical ownership.
	if err := m.Link("d/f", "hl"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := m.Stat("hl", true); fi.UID != 7 {
		t.Errorf("hard link ownership = %d, want the canonical 7", fi.UID)
	}
	// Chtimes then egid seam default (egid nil branch elsewhere): keep
	// the egid seam exercised through Chown of gid only.
	if err := m.Chown("hl", -1, 3, true); err == nil && m.euid() != 0 {
		t.Error("non-root gid change should refuse")
	}
	_ = time.Now
}

// TestOSChownXattrErrorPaths covers the OS wrappers' resolve-failure arms
// and the non-unix-style refusals via an injected failing getwd.
func TestOSChownXattrErrorPaths(t *testing.T) {
	skipWithoutUnixMetadata(t, "ownership")
	bad := &OSFileOps{getwd: func() (string, error) { return "", errors.New("no cwd") }}
	if err := bad.Chown("rel", -1, -1, true); err == nil {
		t.Error("Chown with failing getwd should error")
	}
	if _, err := bad.XattrGet("rel", "a"); err == nil {
		t.Error("XattrGet with failing getwd should error")
	}
	if err := bad.XattrSet("rel", "a", nil); err == nil {
		t.Error("XattrSet with failing getwd should error")
	}
	if _, err := bad.XattrList("rel"); err == nil {
		t.Error("XattrList with failing getwd should error")
	}
	if err := bad.XattrRemove("rel", "a"); err == nil {
		t.Error("XattrRemove with failing getwd should error")
	}

	// Lchown arm: re-own a symlink without following (a -1/-1 no-op so it
	// succeeds unprivileged).
	dir := t.TempDir()
	target := filepath.Join(dir, "t")
	link := filepath.Join(dir, "l")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := (&OSFileOps{}).Chown(link, -1, -1, false); err != nil {
		t.Errorf("lchown -1/-1: %v", err)
	}
}

// TestOSFileOwnerFallback pins the -1/-1 sentinel for a FileInfo whose
// Sys() is not a Stat_t (a synthetic double).
func TestOSFileOwnerFallback(t *testing.T) {
	if uid, gid := osFileOwner(fakeFileInfo{}); uid != -1 || gid != -1 {
		t.Errorf("fallback owner = %d/%d, want -1/-1", uid, gid)
	}
}

type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "fake" }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }

// TestMemMetaResolveLoops covers the symlink-loop (ELOOP) resolve arms of
// Chown and the xattr quartet.
func TestMemMetaResolveLoops(t *testing.T) {
	m := NewMem()
	if err := m.Symlink("cyc", "cyc"); err != nil {
		t.Fatal(err)
	}
	if err := m.Chown("cyc", -1, -1, true); err == nil {
		t.Error("chown through a symlink loop should error")
	}
	if _, err := m.XattrGet("cyc", "a"); err == nil {
		t.Error("xattr-get through a symlink loop should error")
	}
	if err := m.XattrSet("cyc", "a", nil); err == nil {
		t.Error("xattr-set through a symlink loop should error")
	}
}
