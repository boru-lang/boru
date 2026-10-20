package capabilities

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// handles_test.go — the stateful file-handle capability: the OpenOpts →
// flag mapping, the OS and mem Open paths, every memFileHandle method arm,
// the overlay's read-vs-write-intent routing, and an OS-vs-mem
// differential of an open/write/seek/read/close thread.

func TestOpenFlagMapping(t *testing.T) {
	cases := []struct {
		opts OpenOpts
		want int
	}{
		{OpenOpts{}, os.O_RDONLY},
		{OpenOpts{Read: true}, os.O_RDONLY},
		{OpenOpts{Write: true}, os.O_WRONLY},
		{OpenOpts{Read: true, Write: true}, os.O_RDWR},
		{OpenOpts{Write: true, Append: true}, os.O_WRONLY | os.O_APPEND},
		{OpenOpts{Write: true, Create: true, Truncate: true}, os.O_WRONLY | os.O_CREATE | os.O_TRUNC},
		{OpenOpts{Write: true, Create: true, Exclusive: true}, os.O_WRONLY | os.O_CREATE | os.O_EXCL},
		// Truncate without a write intent must NOT set O_TRUNC.
		{OpenOpts{Read: true, Truncate: true}, os.O_RDONLY},
	}
	for _, c := range cases {
		if got := openFlag(c.opts); got != c.want {
			t.Errorf("openFlag(%+v) = %#x, want %#x", c.opts, got, c.want)
		}
	}
	if got := openPerm(OpenOpts{}); got != 0o644 {
		t.Errorf("openPerm default = %v, want 0644", got)
	}
	if got := openPerm(OpenOpts{Perm: 0o600}); got != 0o600 {
		t.Errorf("openPerm explicit = %v, want 0600", got)
	}
}

func TestOSOpen(t *testing.T) {
	root := t.TempDir()
	o := &OSFileOps{}
	p := filepath.Join(root, "f.txt")
	// Create + write, then read back through a fresh read-only handle.
	h, err := o.Open(p, OpenOpts{Write: true, Create: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	rh, err := o.Open(p, OpenOpts{Read: true})
	if err != nil {
		t.Fatal(err)
	}
	defer rh.Close()
	got, _ := io.ReadAll(rh)
	if string(got) != "hi" {
		t.Errorf("os handle read = %q", got)
	}
	// A failing getwd surfaces from ResolvePath.
	bad := &OSFileOps{getwd: func() (string, error) { return "", errors.New("no wd") }}
	if _, err := bad.Open("rel.txt", OpenOpts{Read: true}); err == nil {
		t.Error("Open with a failing getwd should error")
	}
	// Opening an absent file read-only errors from os.OpenFile.
	if _, err := o.Open(filepath.Join(root, "ghost"), OpenOpts{Read: true}); err == nil {
		t.Error("Open of an absent file should error")
	}
}

func TestMemOpenModel(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("d/f.txt", []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Read-only open of an existing file.
	h, err := m.Open("d/f.txt", OpenOpts{Read: true})
	if err != nil {
		t.Fatal(err)
	}
	if h.Name() != "d/f.txt" {
		t.Errorf("Name = %q", h.Name())
	}
	got, _ := io.ReadAll(h)
	if string(got) != "hello" {
		t.Errorf("read = %q", got)
	}
	_ = h.Close()

	// Opening a directory errors.
	if err := m.MkdirAll("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Open("dir", OpenOpts{Read: true}); err == nil {
		t.Error("opening a directory should error")
	}
	// Absent + no create → ENOENT.
	if _, err := m.Open("ghost", OpenOpts{Read: true}); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent no-create = %v", err)
	}
	// Create makes the file; Exclusive then refuses it.
	if _, err := m.Open("d/new.txt", OpenOpts{Write: true, Create: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Open("d/new.txt", OpenOpts{Write: true, Create: true, Exclusive: true}); !errors.Is(err, os.ErrExist) {
		t.Errorf("exclusive-exists = %v", err)
	}
	// Truncate clears an existing file's contents.
	th, err := m.Open("d/f.txt", OpenOpts{Write: true, Truncate: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = th.Close()
	if b, _ := m.ReadFile("d/f.txt"); len(b) != 0 {
		t.Errorf("truncate left %q", b)
	}
	// Create under an UNRECORDED parent succeeds: mem treats unrecorded
	// ancestors as implicitly writable (a documented fidelity exception,
	// like WriteFile), recording them on the way.
	if _, err := m.Open("nodir/x.txt", OpenOpts{Write: true, Create: true}); err != nil {
		t.Errorf("create under an unrecorded dir: %v", err)
	}
}

func TestMemOpenPermission(t *testing.T) {
	m := NewMem()
	// A non-root euid is enforced. A 0400 file cannot be opened for write.
	m.SetIdentity(func() int { return 1000 }, func() int { return 1000 })
	if err := m.WriteFile("ro.txt", []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Open("ro.txt", OpenOpts{Write: true}); !errors.Is(err, os.ErrPermission) {
		t.Errorf("write-open of a 0400 file = %v", err)
	}
	// A 0200 file cannot be opened for read.
	if err := m.WriteFile("wo.txt", []byte("x"), 0o200); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Open("wo.txt", OpenOpts{Read: true}); !errors.Is(err, os.ErrPermission) {
		t.Errorf("read-open of a 0200 file = %v", err)
	}
}

func TestMemFileHandleOps(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("f.bin", []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := m.Open("f.bin", OpenOpts{Read: true, Write: true})
	if err != nil {
		t.Fatal(err)
	}

	// Sequential Read advances the cursor; a second Read continues.
	buf := make([]byte, 4)
	if n, _ := h.Read(buf); n != 4 || string(buf) != "0123" {
		t.Errorf("read1 = %d %q", n, buf)
	}
	// ReadAt is positioned and leaves the cursor untouched.
	at := make([]byte, 3)
	if n, _ := h.ReadAt(at, 5); n != 3 || string(at) != "567" {
		t.Errorf("readat = %d %q", n, at)
	}
	// Seek to end and read → EOF.
	if off, _ := h.Seek(0, io.SeekEnd); off != 10 {
		t.Errorf("seek end = %d", off)
	}
	if _, err := h.Read(buf); err != io.EOF {
		t.Errorf("read at EOF = %v", err)
	}
	// Seek current back, then WriteAt splices.
	if off, _ := h.Seek(-4, io.SeekCurrent); off != 6 {
		t.Errorf("seek current = %d", off)
	}
	if _, err := h.WriteAt([]byte("XY"), 2); err != nil {
		t.Fatal(err)
	}
	// Write at the cursor (6) overwrites in place.
	if _, err := h.Write([]byte("Z")); err != nil {
		t.Fatal(err)
	}
	// The handle write is visible to a stateless read (page-cache).
	if b, _ := m.ReadFile("f.bin"); string(b) != "01XY45Z789" {
		t.Errorf("post-write content = %q", b)
	}
	// Truncate shrink then grow (zero-fill).
	if err := h.Truncate(3); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.ReadFile("f.bin"); string(b) != "01X" {
		t.Errorf("shrink = %q", b)
	}
	if err := h.Truncate(5); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.ReadFile("f.bin"); len(b) != 5 || b[3] != 0 || b[4] != 0 {
		t.Errorf("grow = %v", b)
	}
	if err := h.Sync(); err != nil {
		t.Errorf("sync: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	// Double close errors; every op on a closed handle errors.
	if err := h.Close(); err == nil {
		t.Error("double close should error")
	}
	if _, err := h.Read(buf); err == nil {
		t.Error("read on closed")
	}
	if _, err := h.ReadAt(buf, 0); err == nil {
		t.Error("readat on closed")
	}
	if _, err := h.Write(buf); err == nil {
		t.Error("write on closed")
	}
	if _, err := h.WriteAt(buf, 0); err == nil {
		t.Error("writeat on closed")
	}
	if _, err := h.Seek(0, io.SeekStart); err == nil {
		t.Error("seek on closed")
	}
	if err := h.Truncate(0); err == nil {
		t.Error("truncate on closed")
	}
	if err := h.Sync(); err == nil {
		t.Error("sync on closed")
	}
}

func TestMemFileHandleModeGuards(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("f", []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A read-only handle refuses writes and truncation.
	rh, _ := m.Open("f", OpenOpts{Read: true})
	if _, err := rh.Write([]byte("x")); err == nil {
		t.Error("write on a read-only handle")
	}
	if _, err := rh.WriteAt([]byte("x"), 0); err == nil {
		t.Error("writeat on a read-only handle")
	}
	if err := rh.Truncate(0); err == nil {
		t.Error("truncate on a read-only handle")
	}
	_ = rh.Close()
	// A write-only handle refuses reads.
	wh, _ := m.Open("f", OpenOpts{Write: true})
	if _, err := wh.Read(make([]byte, 1)); err == nil {
		t.Error("read on a write-only handle")
	}
	if _, err := wh.ReadAt(make([]byte, 1), 0); err == nil {
		t.Error("readat on a write-only handle")
	}
	_ = wh.Close()
	// Negative offsets and an unknown whence are invalid.
	h, _ := m.Open("f", OpenOpts{Read: true, Write: true})
	if _, err := h.ReadAt(make([]byte, 1), -1); err == nil {
		t.Error("negative readat")
	}
	if _, err := h.WriteAt([]byte("x"), -1); err == nil {
		t.Error("negative writeat")
	}
	if _, err := h.Seek(0, 99); err == nil {
		t.Error("bad whence")
	}
	if _, err := h.Seek(-1, io.SeekStart); err == nil {
		t.Error("seek to a negative offset")
	}
	if err := h.Truncate(-1); err == nil {
		t.Error("negative truncate")
	}
	_ = h.Close()
}

func TestMemFileHandleAppend(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("log", []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, _ := m.Open("log", OpenOpts{Write: true, Append: true})
	// Append writes at end regardless of a prior seek.
	_, _ = h.Seek(0, io.SeekStart)
	_, _ = h.Write([]byte("B"))
	_, _ = h.Write([]byte("C"))
	// A positioned WriteAt on an append handle is refused, like os.File.
	if _, err := h.WriteAt([]byte("X"), 0); err == nil {
		t.Error("WriteAt on an append handle should be refused")
	}
	_ = h.Close()
	if b, _ := m.ReadFile("log"); string(b) != "ABC" {
		t.Errorf("append = %q", b)
	}
}

func TestMemTruncateReadOnlyOpen(t *testing.T) {
	// {Read, Truncate} must NOT clear the file (truncate needs write).
	m := NewMem()
	if err := m.WriteFile("keep", []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := m.Open("keep", OpenOpts{Read: true, Truncate: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = h.Close()
	if b, _ := m.ReadFile("keep"); string(b) != "data" {
		t.Errorf("read+truncate cleared the file: %q", b)
	}
}

func TestMemFileHandleNoMeta(t *testing.T) {
	// A file poked straight into Files (no Meta) still stores through a
	// handle write — the store's meta-absent arm.
	m := NewMem()
	m.Files[filepath.FromSlash("/bare")] = []byte("x")
	h, err := m.Open("/bare", OpenOpts{Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Write([]byte("yy")); err != nil {
		t.Fatal(err)
	}
	_ = h.Close()
	if string(m.Files["/bare"]) != "yy" {
		t.Errorf("no-meta write = %q", m.Files["/bare"])
	}
}

func TestOverlayOpen(t *testing.T) {
	o, upper, lower := metaOverlay(t)
	if err := lower.WriteFile("base.txt", []byte("lower"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Read-only open falls through to the lower.
	rh, err := o.Open("base.txt", OpenOpts{Read: true})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(rh); string(b) != "lower" {
		t.Errorf("overlay read-through = %q", b)
	}
	_ = rh.Close()
	// A write-intent open copies the lower file up, then appends land in
	// the upper only.
	ah, err := o.Open("base.txt", OpenOpts{Write: true, Append: true})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = ah.Write([]byte("+up"))
	_ = ah.Close()
	if b, _ := upper.ReadFile("base.txt"); string(b) != "lower+up" {
		t.Errorf("copy-up append landed = %q (upper)", b)
	}
	if b, _ := lower.ReadFile("base.txt"); string(b) != "lower" {
		t.Errorf("lower must stay pristine, got %q", b)
	}
	// Create a fresh file → upper.
	ch, err := o.Open("fresh.txt", OpenOpts{Write: true, Create: true})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = ch.Write([]byte("new"))
	_ = ch.Close()
	if _, serr := upper.Stat("fresh.txt", false); serr != nil {
		t.Error("fresh create not in upper")
	}
	// Read-only open of an absent path errors; a whiteout hides the lower.
	if _, err := o.Open("absent", OpenOpts{Read: true}); err == nil {
		t.Error("read-open absent should error")
	}
	if err := o.Remove("base.txt", false); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Open("base.txt", OpenOpts{Read: true}); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("read-open whited-out = %v", err)
	}
	// Create over a whiteout re-materialises it (Exclusive succeeds since
	// the union sees it as deleted).
	xh, err := o.Open("base.txt", OpenOpts{Write: true, Create: true, Exclusive: true})
	if err != nil {
		t.Fatalf("exclusive create over whiteout: %v", err)
	}
	_ = xh.Close()
}

// failReadLower is a lower whose ReadFile fails (Stat still succeeds), to
// reach the overlay Open write-intent's copyUp-error arm.
type failReadLower struct{ *MemFileOps }

func (f failReadLower) ReadFile(string) ([]byte, error) { return nil, errors.New("lower read boom") }

func TestMemOpenEdgeArms(t *testing.T) {
	m := NewMem()
	// A symlink loop surfaces from resolve on open.
	if err := m.Symlink("cyc", "cyc"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Open("cyc", OpenOpts{Read: true}); err == nil {
		t.Error("opening a symlink loop should error")
	}
	// A create under a non-writable parent (non-root) refuses.
	m.SetIdentity(func() int { return 1000 }, func() int { return 1000 })
	if err := m.MkdirAll("ro-dir", 0o500); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Open("ro-dir/child.txt", OpenOpts{Write: true, Create: true}); err == nil {
		t.Error("create under a read-only dir should refuse")
	}
	// ReadAt past EOF returns EOF; a buffer larger than the remainder
	// returns a short read + EOF.
	m2 := NewMem()
	if err := m2.WriteFile("f", []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, _ := m2.Open("f", OpenOpts{Read: true})
	if _, err := h.ReadAt(make([]byte, 2), 10); err != io.EOF {
		t.Errorf("readat past EOF = %v", err)
	}
	big := make([]byte, 8)
	if n, err := h.ReadAt(big, 1); n != 2 || err != io.EOF {
		t.Errorf("readat partial = %d %v", n, err)
	}
	_ = h.Close()
}

func TestOverlayOpenEdgeArms(t *testing.T) {
	upper, lower := NewMem(), NewMem()
	o := NewOverlay(upper, lower)
	// Read-only open of a file that lives in the UPPER.
	if err := upper.WriteFile("up.txt", []byte("U"), 0o644); err != nil {
		t.Fatal(err)
	}
	uh, err := o.Open("up.txt", OpenOpts{Read: true})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(uh); string(b) != "U" {
		t.Errorf("upper read = %q", b)
	}
	_ = uh.Close()
	// A write-intent Upper.Open failure forwards: O_EXCL on an
	// already-upper file.
	if _, err := o.Open("up.txt", OpenOpts{Write: true, Create: true, Exclusive: true}); !errors.Is(err, os.ErrExist) {
		t.Errorf("exclusive over an upper file = %v", err)
	}
	// A deref loop errors before the open.
	if err := upper.Symlink("loop", "loop"); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Open("loop", OpenOpts{Read: true}); err == nil {
		t.Error("overlay deref loop should error")
	}
	// A copyUp failure (lower read boom) forwards on write intent.
	badLower := NewMem()
	if err := badLower.WriteFile("seed.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ob := NewOverlay(NewMem(), failReadLower{badLower})
	if _, err := ob.Open("seed.txt", OpenOpts{Write: true}); err == nil {
		t.Error("copyUp failure should forward from Open")
	}

	// An exclusive create over a LOWER-only file fails on union existence
	// BEFORE any copy-up: the refused open must leave the upper untouched.
	up2, low2 := NewMem(), NewMem()
	if err := low2.WriteFile("only-lower.txt", []byte("L"), 0o644); err != nil {
		t.Fatal(err)
	}
	o2 := NewOverlay(up2, low2)
	if _, err := o2.Open("only-lower.txt", OpenOpts{Write: true, Create: true, Exclusive: true}); !errors.Is(err, os.ErrExist) {
		t.Errorf("exclusive over a lower file = %v, want ErrExist", err)
	}
	if _, serr := up2.Stat("only-lower.txt", false); serr == nil {
		t.Error("a refused exclusive open must not copy the file up")
	}

	// A write-intent open whose upper target is a DIRECTORY forwards the
	// Upper.Open error (EISDIR).
	if err := upper.MkdirAll("adir", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Open("adir", OpenOpts{Write: true}); err == nil {
		t.Error("write-open of a directory should forward the upper error")
	}
}

func TestDifferentialOpenThread(t *testing.T) {
	root, osOps, mem := diffPair(t)
	j := func(rel string) string { return filepath.Join(root, rel) }
	step(t, "seed-dir", osOps.MkdirAll(j("d"), 0o755), mem.MkdirAll(j("d"), 0o755))

	for _, ops := range []FileOps{osOps, mem} {
		p := j("d/thread.txt")
		h, err := ops.Open(p, OpenOpts{Read: true, Write: true, Create: true, Truncate: true})
		if err != nil {
			t.Fatalf("open (%T): %v", ops, err)
		}
		if _, err := h.Write([]byte("hello world")); err != nil {
			t.Fatalf("write (%T): %v", ops, err)
		}
		// A positioned WriteAt splices; Seek + read the region back.
		if _, err := h.WriteAt([]byte("HELLO"), 0); err != nil {
			t.Fatalf("writeat (%T): %v", ops, err)
		}
		if off, err := h.Seek(6, io.SeekStart); err != nil || off != 6 {
			t.Fatalf("seek (%T): %d %v", ops, off, err)
		}
		region := make([]byte, 5)
		n, _ := h.Read(region)
		if string(region[:n]) != "world" {
			t.Errorf("thread read (%T) = %q", ops, region[:n])
		}
		if err := h.Sync(); err != nil {
			t.Errorf("sync (%T): %v", ops, err)
		}
		if err := h.Close(); err != nil {
			t.Errorf("close (%T): %v", ops, err)
		}
		// The whole file, read statelessly, agrees on both backends.
		if b, _ := ops.ReadFile(p); string(b) != "HELLO world" {
			t.Errorf("final content (%T) = %q", ops, b)
		}
	}

	// O_EXCL over an existing file errors with the same class on both.
	_, oe := osOps.Open(j("d/thread.txt"), OpenOpts{Write: true, Create: true, Exclusive: true})
	_, me := mem.Open(j("d/thread.txt"), OpenOpts{Write: true, Create: true, Exclusive: true})
	step(t, "exclusive-exists", oe, me)
	// Read-open of an absent file: same class.
	_, oe = osOps.Open(j("d/ghost"), OpenOpts{Read: true})
	_, me = mem.Open(j("d/ghost"), OpenOpts{Read: true})
	step(t, "open-absent", oe, me)
}
