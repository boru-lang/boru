package capabilities

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var errNoWD = errors.New("working directory unavailable")

func failGetwd() (string, error) {
	return "", errNoWD
}

// TestOSFileOpsResolvePathGetwdError triggers the os.Getwd error path
// using an injected getwd function (cross-platform).
func TestOSFileOpsResolvePathGetwdError(t *testing.T) {
	o := &OSFileOps{getwd: failGetwd}

	_, err := o.ResolvePath("relative.txt")
	if err == nil {
		t.Fatal("expected error from ResolvePath when getwd fails")
	}
	if !errors.Is(err, errNoWD) {
		t.Fatalf("expected errNoWD, got: %v", err)
	}
}

// TestOSFileOpsReadFileResolveError triggers the ResolvePath error path
// in ReadFile via an injected getwd function.
func TestOSFileOpsReadFileResolveError(t *testing.T) {
	o := &OSFileOps{getwd: failGetwd}

	_, err := o.ReadFile("relative.txt")
	if err == nil {
		t.Fatal("expected error from ReadFile when getwd fails")
	}
}

// TestOSFileOpsWriteFileResolveError triggers the ResolvePath error path
// in WriteFile via an injected getwd function.
func TestOSFileOpsWriteFileResolveError(t *testing.T) {
	o := &OSFileOps{getwd: failGetwd}

	err := o.WriteFile("relative.txt", []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error from WriteFile when getwd fails")
	}
}

// TestOSFileOpsWriteMkdirAllError triggers the MkdirAll error path
// using an injected mkdirAll function (cross-platform, works as root).
func TestOSFileOpsWriteMkdirAllError(t *testing.T) {
	errMkdir := errors.New("mkdir failure")
	o := &OSFileOps{
		mkdirAll: func(string, os.FileMode) error { return errMkdir },
	}

	target := filepath.Join(t.TempDir(), "newsubdir", "file.txt")
	err := o.WriteFile(target, []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error from WriteFile due to MkdirAll failure")
	}
	if !errors.Is(err, errMkdir) {
		t.Fatalf("expected errMkdir, got: %v", err)
	}
}

// TestOSFileOpsWriteFileError triggers the os.WriteFile error
// by writing to a path that is a directory.
func TestOSFileOpsWriteFileError(t *testing.T) {
	o := &OSFileOps{}
	tmpDir := t.TempDir()

	// Create a directory where WriteFile expects a file.
	dirAsFile := filepath.Join(tmpDir, "actuallyADir")
	if err := os.Mkdir(dirAsFile, 0755); err != nil {
		t.Fatal(err)
	}

	err := o.WriteFile(dirAsFile, []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error writing to a directory path")
	}
}

// TestOSFileOpsResolvePathAbsolute verifies absolute paths pass through unchanged.
func TestOSFileOpsResolvePathAbsolute(t *testing.T) {
	o := &OSFileOps{}
	// t.TempDir is absolute on every host; `/absolute/…` is not on Windows,
	// where a rooted path is relative to the current drive.
	abs := filepath.Join(t.TempDir(), "path.txt")
	got, err := o.ResolvePath(abs)
	if err != nil {
		t.Fatalf("ResolvePath error: %v", err)
	}
	if got != abs {
		t.Errorf("got %q, want %q", got, abs)
	}
}

// TestMemFileOpsResolvePathAbsolute tests MemFileOps.ResolvePath with an absolute path.
func TestMemFileOpsResolvePathAbsolute(t *testing.T) {
	m := NewMem()
	got, err := m.ResolvePath("/absolute/path.txt")
	if err != nil {
		t.Fatalf("ResolvePath error: %v", err)
	}
	if want := filepath.FromSlash("/absolute/path.txt"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestMemFileOpsReadWriteAbsolutePath tests read/write with absolute paths.
func TestMemFileOpsReadWriteAbsolutePath(t *testing.T) {
	m := NewMem()
	path := "/some/abs/path.txt"
	data := []byte("absolute data")

	if err := m.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	got, err := m.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "absolute data" {
		t.Errorf("got %q, want %q", got, "absolute data")
	}
}

// TestMemFileOpsWriteEmptyData tests writing empty data.
func TestMemFileOpsWriteEmptyData(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("empty.txt", []byte{}, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	got, err := m.ReadFile("empty.txt")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty data, got %q", got)
	}
}
