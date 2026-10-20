package capabilities

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fileops_temp_test.go — TempFile / TempDir / Statfs coverage: the mem
// model, the OS wrappers' error arms, and the differential scenarios
// (SHAPE-compared: temp names carry a random token on the real side and
// a counter on mem; statfs numbers are volume-dependent — the harness
// pins prefix/suffix/containment/modes and error classes, never values).

func TestDifferentialTempShape(t *testing.T) {
	skipWithoutUnixMetadata(t, "temp-file modes")
	root, osOps, mem := diffPair(t)
	j := func(rel string) string { return filepath.Join(root, rel) }
	step(t, "seed-dir", osOps.MkdirAll(j("d"), 0o755), mem.MkdirAll(j("d"), 0o755))

	check := func(name, base string, wantPrefix, wantSuffix string, wantDir bool, ops FileOps, got string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s (%T): %v", name, ops, err)
		}
		if filepath.Dir(got) != base {
			t.Errorf("%s (%T): %q not contained in %q", name, ops, got, base)
		}
		bn := filepath.Base(got)
		if !strings.HasPrefix(bn, wantPrefix) || !strings.HasSuffix(bn, wantSuffix) {
			t.Errorf("%s (%T): name %q missing prefix %q / suffix %q", name, ops, bn, wantPrefix, wantSuffix)
		}
		fi, serr := ops.Stat(got, false)
		if serr != nil || fi.IsDir != wantDir {
			t.Errorf("%s (%T): stat = %+v (%v)", name, ops, fi, serr)
		}
		wantMode := os.FileMode(0o600)
		if wantDir {
			wantMode = 0o700
		}
		if fi.Mode.Perm() != wantMode {
			t.Errorf("%s (%T): mode = %v, want %v", name, ops, fi.Mode.Perm(), wantMode)
		}
	}
	for _, ops := range []FileOps{osOps, mem} {
		got, err := ops.TempFile(j("d"), "log-*.txt")
		check("tempfile", j("d"), "log-", ".txt", false, ops, got, err)
		got, err = ops.TempDir(j("d"), "work-")
		check("tempdir", j("d"), "work-", "", true, ops, got, err)
	}

	// An absent parent dir refuses with the same class on both sides.
	_, oe := osOps.TempFile(j("ghost"), "x-*")
	_, me := mem.TempFile(j("ghost"), "x-*")
	step(t, "temp-absent-dir", oe, me)
	_, oe = osOps.TempDir(j("ghost"), "x-*")
	_, me = mem.TempDir(j("ghost"), "x-*")
	step(t, "tempdir-absent-dir", oe, me)
	// A FILE as the parent refuses too.
	step(t, "seed-file", osOps.WriteFile(j("f"), []byte("x"), 0o644), mem.WriteFile(j("f"), []byte("x"), 0o644))
	_, oe = osOps.TempFile(j("f"), "x-*")
	_, me = mem.TempFile(j("f"), "x-*")
	step(t, "temp-in-file", oe, me)
}

func TestDifferentialStatfsShape(t *testing.T) {
	skipWithoutUnixMetadata(t, "statfs")
	root, osOps, mem := diffPair(t)
	step(t, "seed", osOps.WriteFile(filepath.Join(root, "f"), []byte("data"), 0o644),
		mem.WriteFile(filepath.Join(root, "f"), []byte("data"), 0o644))

	for _, ops := range []FileOps{osOps, mem} {
		fs, err := ops.Statfs(filepath.Join(root, "f"))
		if err != nil {
			t.Fatalf("statfs (%T): %v", ops, err)
		}
		// SHAPE contract only: a real volume and the synthetic mem volume
		// agree on invariants, never on numbers.
		if fs.TotalBytes == 0 {
			t.Errorf("statfs (%T): zero total", ops)
		}
		if fs.AvailBytes > fs.FreeBytes || fs.FreeBytes > fs.TotalBytes {
			t.Errorf("statfs (%T): avail %d ≤ free %d ≤ total %d violated", ops, fs.AvailBytes, fs.FreeBytes, fs.TotalBytes)
		}
		if fs.BlockSize <= 0 || fs.Type == "" {
			t.Errorf("statfs (%T): bsize %d / type %q", ops, fs.BlockSize, fs.Type)
		}
	}
	// Absent path: same error class.
	_, oe := osOps.Statfs(filepath.Join(root, "ghost"))
	_, me := mem.Statfs(filepath.Join(root, "ghost"))
	step(t, "statfs-absent", oe, me)
}

func TestMemTempModel(t *testing.T) {
	m := NewMem()
	// Default root is the implicit /tmp; tokens count deterministically.
	p1, err := m.TempFile("", "a-*")
	if err != nil || p1 != filepath.FromSlash("/tmp/a-1") {
		t.Errorf("first temp = %q (%v), want /tmp/a-1", p1, err)
	}
	// A pattern without "*" appends the token.
	p2, err := m.TempFile("", "plain")
	if err != nil || p2 != filepath.FromSlash("/tmp/plain2") {
		t.Errorf("no-star temp = %q (%v), want /tmp/plain2", p2, err)
	}
	// The uniqueness loop skips an occupied candidate.
	if err := m.WriteFile("/tmp/a-3", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p3, err := m.TempFile("", "a-*")
	if err != nil || p3 != filepath.FromSlash("/tmp/a-4") {
		t.Errorf("collision-skip temp = %q (%v), want /tmp/a-4", p3, err)
	}
	// TempDir creates a 0700 directory.
	d, err := m.TempDir("", "d-*")
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := m.Stat(d, false); !fi.IsDir || fi.Mode.Perm() != 0o700 {
		t.Errorf("temp dir stat = %+v", fi)
	}
}

func TestMemStatfsModel(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("f", []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := m.Statfs("f")
	if err != nil {
		t.Fatal(err)
	}
	if fs.TotalBytes != 1<<30 || fs.Type != "mem" || fs.BlockSize != 4096 {
		t.Errorf("synthetic volume = %+v", fs)
	}
	if fs.FreeBytes != (1<<30)-5 || fs.AvailBytes != fs.FreeBytes {
		t.Errorf("usage accounting = %+v", fs)
	}
	// The mem root always statfs-es (implicit), an absent path refuses,
	// a symlink loop errors through resolve.
	if _, err := m.Statfs("."); err != nil {
		t.Errorf("root statfs: %v", err)
	}
	if _, err := m.Statfs("ghost"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent statfs: %v", err)
	}
	if err := m.Symlink("cyc", "cyc"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Statfs("cyc"); err == nil {
		t.Error("loop statfs should error")
	}
}

func TestOSTempStatfsErrorPaths(t *testing.T) {
	bad := &OSFileOps{getwd: func() (string, error) { return "", errors.New("no cwd") }}
	if _, err := bad.TempFile("rel", "x-*"); err == nil {
		t.Error("TempFile with failing getwd should error")
	}
	if _, err := bad.TempDir("rel", "x-*"); err == nil {
		t.Error("TempDir with failing getwd should error")
	}
	if _, err := bad.Statfs("rel"); err == nil {
		t.Error("Statfs with failing getwd should error")
	}
	// The "" dir arm needs no resolution (os.TempDir root).
	good := &OSFileOps{}
	p, err := good.TempFile("", "boru-cover-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(p)
	if !strings.HasPrefix(filepath.Base(p), "boru-cover-") {
		t.Errorf("default-root temp = %q", p)
	}
}

func TestMemTempCreateErrors(t *testing.T) {
	// tempName skips the parent check for the implicit temp root, so a
	// /tmp shadowed by a FILE lets tempName succeed but the create fail —
	// the only way to reach TempFile's WriteFile / TempDir's MkdirAll arm.
	m := NewMem()
	if err := m.WriteFile(memTempRoot, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.TempFile("", "y-*"); err == nil {
		t.Error("TempFile under a file-shadowed temp root should error")
	}
	if _, err := m.TempDir("", "z-*"); err == nil {
		t.Error("TempDir under a file-shadowed temp root should error")
	}
}

func TestOSTempCloseSeam(t *testing.T) {
	orig := closeTempFile
	defer func() { closeTempFile = orig }()
	closeTempFile = func(f *os.File) error {
		_ = f.Close() // real close so no fd leaks; report the seam error
		return errors.New("close boom")
	}
	if _, err := (&OSFileOps{}).TempFile("", "boru-close-*"); err == nil {
		t.Error("a Close failure should surface from TempFile")
	}
}

func TestOverlayTempErrors(t *testing.T) {
	o, _, _ := metaOverlay(t)
	// An absent (non-root) dir fails in the upper's tempName — the
	// overlay's error arm forwards it before the whiteout delete.
	if _, err := o.TempFile("ghostdir", "q-*"); err == nil {
		t.Error("overlay TempFile in an absent dir should error")
	}
	if _, err := o.TempDir("ghostdir", "q-*"); err == nil {
		t.Error("overlay TempDir in an absent dir should error")
	}
}

func TestHexMagic(t *testing.T) {
	if got := hexMagic(0xEF53); got != "0xef53" {
		t.Errorf("hexMagic = %q", got)
	}
}

func TestOverlayTempStatfs(t *testing.T) {
	o, upper, lower := metaOverlay(t)
	if err := lower.WriteFile("seed", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Temp entries land in the UPPER; a whiteout at the path is cleared.
	p, err := o.TempFile("", "u-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, serr := upper.Stat(p, false); serr != nil {
		t.Error("temp file not in upper")
	}
	if _, serr := lower.Stat(p, false); serr == nil {
		t.Error("temp file leaked into lower")
	}
	d, err := o.TempDir("", "ud-*")
	if err != nil {
		t.Fatal(err)
	}
	if fi, serr := upper.Stat(d, false); serr != nil || !fi.IsDir {
		t.Error("temp dir not in upper")
	}
	// Statfs reports the upper's volume.
	fs, err := o.Statfs(".")
	if err != nil || fs.Type != "mem" {
		t.Errorf("overlay statfs = %+v (%v)", fs, err)
	}
}
