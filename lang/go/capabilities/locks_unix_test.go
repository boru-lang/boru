//go:build unix

package capabilities

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOSLockFlockSeam(t *testing.T) {
	orig := flockFn
	t.Cleanup(func() { flockFn = orig })
	flockFn = func(int, int) error { return errors.New("flock boom") }
	root := t.TempDir()
	p := filepath.Join(root, "f")
	_ = os.WriteFile(p, []byte("x"), 0o644)
	if _, err := (&OSFileOps{}).Lock(p, false, true); err == nil {
		t.Error("a flock failure should surface from Lock")
	}
}

func TestOSMmapSeams(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "m")
	_ = os.WriteFile(p, []byte("abc"), 0o644)
	o := &OSFileOps{}
	// mmap syscall failure surfaces.
	origM := mmapFn
	mmapFn = func(int, int64, int, int, int) ([]byte, error) { return nil, errors.New("mmap boom") }
	if _, err := o.Mmap(p, 0, 0, false); err == nil {
		t.Error("mmap syscall failure should surface")
	}
	mmapFn = origM
	// munmap / msync failures surface from Close.
	origMs, origMu := msyncFn, munmapFn
	t.Cleanup(func() { msyncFn = origMs; munmapFn = origMu })
	r, err := o.Mmap(p, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	msyncFn = func([]byte, int) error { return errors.New("msync boom") }
	if err := r.Flush(); err == nil {
		t.Error("msync failure should surface from Flush")
	}
	munmapFn = func([]byte) error { return errors.New("munmap boom") }
	if err := r.Close(); err == nil {
		t.Error("munmap/msync failure should surface from Close")
	}
}
