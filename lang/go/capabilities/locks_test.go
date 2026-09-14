package capabilities

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// locks_test.go — advisory locks (Lock) and memory-mapped files (Mmap):
// the OS backend (flock / mmap behind the platform seams), the mem model
// (reader/writer table + cond, copied-and-flushed mmap), the overlay's
// copy-up-then-upper routing, and an OS-vs-mem differential.

func TestOSLock(t *testing.T) {
	root := t.TempDir()
	o := &OSFileOps{}
	p := filepath.Join(root, "f")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Exclusive lock; a non-blocking second lock is busy; releasing frees it.
	l1, err := o.Lock(p, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Lock(p, false, false); !errors.Is(err, ErrLockBusy) {
		t.Errorf("contended non-block lock = %v, want ErrLockBusy", err)
	}
	if err := l1.Close(); err != nil {
		t.Fatal(err)
	}
	l3, err := o.Lock(p, false, false)
	if err != nil {
		t.Fatalf("relock after release: %v", err)
	}
	if err := l3.Close(); err != nil {
		t.Fatal(err)
	}
	// A second Close is a clean no-op (sync.Once).
	if err := l3.Close(); err != nil {
		t.Errorf("double close: %v", err)
	}
	// Locking an absent path errors (open fails).
	if _, err := o.Lock(filepath.Join(root, "ghost"), false, true); err == nil {
		t.Error("lock of an absent path should error")
	}
}

func TestOSLockShared(t *testing.T) {
	root := t.TempDir()
	o := &OSFileOps{}
	p := filepath.Join(root, "f")
	_ = os.WriteFile(p, []byte("x"), 0o644)
	// Two shared locks coexist.
	s1, err := o.Lock(p, true, true)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := o.Lock(p, true, false)
	if err != nil {
		t.Fatalf("second shared lock: %v", err)
	}
	_ = s1.Close()
	_ = s2.Close()
}

func TestOSLockAllowsIO(t *testing.T) {
	o := &OSFileOps{}
	p := filepath.Join(t.TempDir(), "f")
	if err := o.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock, err := o.Lock(p, false, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	if data, err := o.ReadFile(p); err != nil || string(data) != "before" {
		t.Fatalf("advisory lock must allow ordinary reads: %q, %v", data, err)
	}
	if err := o.WriteFile(p, []byte("after"), 0o644); err != nil {
		t.Fatalf("advisory lock must allow ordinary writes: %v", err)
	}
	if other, err := o.Lock(p, false, false); !errors.Is(err, ErrLockBusy) {
		if other != nil {
			_ = other.Close()
		}
		t.Fatalf("file I/O must not release the advisory lock: %v", err)
	}
}

func TestOSLockMmapResolveErrors(t *testing.T) {
	bad := &OSFileOps{getwd: func() (string, error) { return "", errors.New("no wd") }}
	if _, err := bad.Lock("rel", false, true); err == nil {
		t.Error("Lock with a failing getwd should error")
	}
	if _, err := bad.Mmap("rel", 0, 0, false); err == nil {
		t.Error("Mmap with a failing getwd should error")
	}
}

func TestOSMmapStatSeam(t *testing.T) {
	orig := osFileStat
	t.Cleanup(func() { osFileStat = orig })
	osFileStat = func(*os.File) (os.FileInfo, error) { return nil, errors.New("stat boom") }
	root := t.TempDir()
	p := filepath.Join(root, "m")
	_ = os.WriteFile(p, []byte("abc"), 0o644)
	if _, err := (&OSFileOps{}).Mmap(p, 0, 0, false); err == nil {
		t.Error("a Stat failure should surface from Mmap")
	}
}

func TestOSMmap(t *testing.T) {
	root := t.TempDir()
	o := &OSFileOps{}
	p := filepath.Join(root, "m.bin")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Read-only mapping; a window; Flush is a no-op; Close unmaps.
	r, err := o.Mmap(p, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Bytes()) != "hello world" {
		t.Errorf("mmap bytes = %q", r.Bytes())
	}
	if err := r.Flush(); err != nil {
		t.Errorf("read-only flush: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Errorf("double close: %v", err)
	}
	// Windowed mapping.
	w, err := o.Mmap(p, 6, 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(w.Bytes()) != "world" {
		t.Errorf("windowed mmap = %q", w.Bytes())
	}
	_ = w.Close()
	// Writable mapping: mutate in place, flush, and the file changes.
	mw, err := o.Mmap(p, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	copy(mw.Bytes(), []byte("HELLO"))
	if err := mw.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "HELLO world" {
		t.Errorf("writable mmap did not persist: %q", b)
	}
	// A zero-length file cannot be mapped; an out-of-range window errors.
	empty := filepath.Join(root, "empty")
	_ = os.WriteFile(empty, nil, 0o644)
	if _, err := o.Mmap(empty, 0, 0, false); err == nil {
		t.Error("mapping an empty file should error")
	}
	if _, err := o.Mmap(p, 100, 0, false); err == nil {
		t.Error("an out-of-range offset should error")
	}
	if _, err := o.Mmap(p, 0, 999, false); err == nil {
		t.Error("an over-long window should error")
	}
	// An absent path errors.
	if _, err := o.Mmap(filepath.Join(root, "ghost"), 0, 0, false); err == nil {
		t.Error("mapping an absent path should error")
	}
}

func TestMemLockModel(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Exclusive; contended non-block is busy; release then relock.
	l1, err := m.Lock("f", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Lock("f", false, false); !errors.Is(err, ErrLockBusy) {
		t.Errorf("contended exclusive = %v, want ErrLockBusy", err)
	}
	if _, err := m.Lock("f", true, false); !errors.Is(err, ErrLockBusy) {
		t.Errorf("shared over exclusive = %v, want ErrLockBusy", err)
	}
	_ = l1.Close()
	_ = l1.Close() // idempotent
	// Two shared locks coexist; an exclusive over them is busy.
	s1, _ := m.Lock("f", true, true)
	s2, _ := m.Lock("f", true, true)
	if _, err := m.Lock("f", false, false); !errors.Is(err, ErrLockBusy) {
		t.Errorf("exclusive over shared = %v, want ErrLockBusy", err)
	}
	_ = s1.Close()
	_ = s2.Close()
	// Locking an absent path errors.
	if _, err := m.Lock("ghost", false, true); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lock absent = %v", err)
	}
	// A symlink loop errors through resolve.
	if err := m.Symlink("cyc", "cyc"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Lock("cyc", false, true); err == nil {
		t.Error("lock of a symlink loop should error")
	}
}

func TestMemLockBlockingCond(t *testing.T) {
	// A blocking lock waits on the cond until another goroutine releases.
	m := NewMem()
	_ = m.WriteFile("f", []byte("x"), 0o644)
	l1, err := m.Lock("f", false, true)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan io.Closer, 1)
	go func() {
		l2, _ := m.Lock("f", false, true) // blocks (cond.Wait) until l1 releases
		acquired <- l2
	}()
	// Let the goroutine reach cond.Wait (contended, so it cannot return
	// until we release), then confirm it is still blocked and release.
	time.Sleep(20 * time.Millisecond)
	select {
	case <-acquired:
		t.Fatal("second lock acquired before the first released")
	default:
	}
	_ = l1.Close()
	l2 := <-acquired
	_ = l2.Close()
}

func TestMemMmapModel(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("m.bin", []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A read-only region is a copy; Flush/Close are no-ops for content.
	r, err := m.Mmap("m.bin", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Bytes()) != "abcdef" {
		t.Errorf("mem mmap = %q", r.Bytes())
	}
	copy(r.Bytes(), []byte("ZZ")) // mutating a read-only copy...
	_ = r.Flush()                 // ...does not write back
	_ = r.Close()
	if b, _ := m.ReadFile("m.bin"); string(b) != "abcdef" {
		t.Errorf("read-only mmap wrote back: %q", b)
	}
	// A writable window splices back on Flush.
	w, err := m.Mmap("m.bin", 2, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	copy(w.Bytes(), []byte("XYZ"))
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	if b, _ := m.ReadFile("m.bin"); string(b) != "abXYZf" {
		t.Errorf("writable mmap window = %q", b)
	}
	// Errors: a directory, an absent path, an out-of-range window.
	_ = m.MkdirAll("d", 0o755)
	if _, err := m.Mmap("d", 0, 0, false); err == nil {
		t.Error("mmap of a directory should error")
	}
	if _, err := m.Mmap("ghost", 0, 0, false); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("mmap absent = %v", err)
	}
	if _, err := m.Mmap("m.bin", 0, 999, false); err == nil {
		t.Error("over-long mem window should error")
	}
	// A symlink loop errors through resolve.
	if err := m.Symlink("cyc", "cyc"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Mmap("cyc", 0, 0, false); err == nil {
		t.Error("mmap of a symlink loop should error")
	}
}

func TestOverlayLockMmap(t *testing.T) {
	o, upper, lower := metaOverlay(t)
	if err := lower.WriteFile("base.txt", []byte("lower"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Locking a lower-only file copies it up, then locks the upper.
	l, err := o.Lock("base.txt", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, serr := upper.Stat("base.txt", false); serr != nil {
		t.Error("lock did not copy the lower file up")
	}
	_ = l.Close()
	// A read-only mmap reads through to the lower.
	r, err := o.Mmap("other.txt", 0, 0, false)
	if err == nil {
		t.Error("mmap of an absent overlay path should error")
	}
	_ = r
	if err := lower.WriteFile("ro.txt", []byte("LOW"), 0o644); err != nil {
		t.Fatal(err)
	}
	rr, err := o.Mmap("ro.txt", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(rr.Bytes()) != "LOW" {
		t.Errorf("overlay read mmap = %q", rr.Bytes())
	}
	_ = rr.Close()
	// A writable mmap copies up then maps the upper.
	w, err := o.Mmap("ro.txt", 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	copy(w.Bytes(), []byte("UPP"))
	_ = w.Flush()
	_ = w.Close()
	if b, _ := upper.ReadFile("ro.txt"); string(b) != "UPP" {
		t.Errorf("writable overlay mmap landed = %q (upper)", b)
	}
	if b, _ := lower.ReadFile("ro.txt"); string(b) != "LOW" {
		t.Errorf("lower must stay pristine: %q", b)
	}
	// A whited-out path refuses a read-only mmap.
	_ = upper.WriteFile("wh.txt", []byte("x"), 0o644)
	_ = o.Remove("wh.txt", false)
	if _, err := o.Mmap("wh.txt", 0, 0, false); err == nil {
		t.Error("mmap of a whited-out path should error")
	}
	// A read-only mmap of an UPPER file maps the upper directly.
	_ = upper.WriteFile("up.txt", []byte("U"), 0o644)
	ur, err := o.Mmap("up.txt", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(ur.Bytes()) != "U" {
		t.Errorf("upper mmap = %q", ur.Bytes())
	}
	_ = ur.Close()
	// A deref loop errors before locking or mapping.
	_ = upper.Symlink("loop", "loop")
	if _, err := o.Lock("loop", false, true); err == nil {
		t.Error("lock of a deref loop should error")
	}
	if _, err := o.Mmap("loop", 0, 0, false); err == nil {
		t.Error("mmap of a deref loop should error")
	}
	// A read-only mmap of a WHITED-OUT lower file refuses (hidden arm).
	_ = lower.WriteFile("lo.txt", []byte("L"), 0o644)
	if err := o.Remove("lo.txt", false); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Mmap("lo.txt", 0, 0, false); err == nil {
		t.Error("mmap of a whited-out lower file should refuse")
	}
	// A writable lock on an UPPER file needs no copy-up (inUpper arm).
	ul, err := o.Lock("up.txt", false, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = ul.Close()
	// A writable mmap of a path absent in BOTH layers hits the
	// Lower.Stat-fail arm of copyUpIfLowerOnly, then Upper.Mmap refuses.
	if _, err := o.Mmap("nope.txt", 0, 0, true); err == nil {
		t.Error("writable mmap of an absent path should refuse")
	}
}

func TestOverlayLockMmapCopyUpError(t *testing.T) {
	// A copyUp failure (lower read boom) forwards from Lock and a writable
	// Mmap on write intent.
	badLower := NewMem()
	if err := badLower.WriteFile("seed.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewOverlay(NewMem(), failReadLower{badLower})
	if _, err := o.Lock("seed.txt", false, true); err == nil {
		t.Error("copyUp failure should forward from Lock")
	}
	if _, err := o.Mmap("seed.txt", 0, 0, true); err == nil {
		t.Error("copyUp failure should forward from a writable Mmap")
	}
}

func TestDifferentialLockMmap(t *testing.T) {
	root, osOps, mem := diffPair(t)
	j := func(rel string) string { return filepath.Join(root, rel) }
	step(t, "seed", osOps.WriteFile(j("f"), []byte("payload"), 0o644),
		mem.WriteFile(j("f"), []byte("payload"), 0o644))

	for _, ops := range []FileOps{osOps, mem} {
		// Lock/relock and a non-blocking contention both agree in class.
		l, err := ops.Lock(j("f"), false, true)
		if err != nil {
			t.Fatalf("lock (%T): %v", ops, err)
		}
		if _, berr := ops.Lock(j("f"), false, false); !errors.Is(berr, ErrLockBusy) {
			t.Errorf("busy (%T): %v", ops, berr)
		}
		if err := l.Close(); err != nil {
			t.Errorf("unlock (%T): %v", ops, err)
		}
		// Mmap read agrees.
		r, err := ops.Mmap(j("f"), 0, 0, false)
		if err != nil {
			t.Fatalf("mmap (%T): %v", ops, err)
		}
		if string(r.Bytes()) != "payload" {
			t.Errorf("mmap bytes (%T) = %q", ops, r.Bytes())
		}
		_ = r.Close()
	}
	// Absent path: same error class on both.
	_, oe := osOps.Lock(j("ghost"), false, true)
	_, me := mem.Lock(j("ghost"), false, true)
	step(t, "lock-absent", oe, me)
}
