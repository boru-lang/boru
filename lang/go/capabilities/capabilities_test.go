package capabilities

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemFileOpsReadWrite(t *testing.T) {
	m := NewMem()
	data := []byte("hello world")

	err := m.WriteFile("test.txt", data, 0644)
	if err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	got, err := m.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestMemFileOpsReadNotFound(t *testing.T) {
	m := NewMem()
	_, err := m.ReadFile("nope.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected IsNotExist error, got: %v", err)
	}
}

func TestMemFileOpsResolvePath(t *testing.T) {
	m := NewMem()
	got, err := m.ResolvePath("foo/bar.txt")
	if err != nil {
		t.Fatalf("ResolvePath error: %v", err)
	}
	want := filepath.Clean("foo/bar.txt")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMemFileOpsWriteCopiesData(t *testing.T) {
	m := NewMem()
	data := []byte("original")
	if err := m.WriteFile("f.txt", data, 0644); err != nil {
		t.Fatal(err)
	}
	// Mutate original slice
	data[0] = 'X'
	got, _ := m.ReadFile("f.txt")
	if string(got) != "original" {
		t.Errorf("data was not copied: got %q", got)
	}
}

func TestOSFileOpsResolvePath(t *testing.T) {
	o := &OSFileOps{}

	// Absolute path returns as-is (t.TempDir is absolute on every host;
	// `/tmp/…` is drive-relative on Windows).
	abs := filepath.Join(t.TempDir(), "test.txt")
	got, err := o.ResolvePath(abs)
	if err != nil {
		t.Fatalf("ResolvePath error: %v", err)
	}
	if got != abs {
		t.Errorf("got %q, want %q", got, abs)
	}

	// Relative path resolved against cwd
	got, err = o.ResolvePath("rel.txt")
	if err != nil {
		t.Fatalf("ResolvePath error: %v", err)
	}
	wd, _ := os.Getwd()
	want := filepath.Join(wd, "rel.txt")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOSFileOpsReadWrite(t *testing.T) {
	o := &OSFileOps{}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	err := o.WriteFile(path, []byte("hello"), 0644)
	if err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	got, err := o.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestOSFileOpsWriteCreatesDir(t *testing.T) {
	o := &OSFileOps{}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sub", "dir", "file.txt")

	err := o.WriteFile(path, []byte("nested"), 0644)
	if err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	got, err := o.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "nested" {
		t.Errorf("got %q, want %q", got, "nested")
	}
}

func TestOSFileOpsReadNotFound(t *testing.T) {
	o := &OSFileOps{}
	_, err := o.ReadFile("/tmp/nonexistent_boru_test_file.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewDefault(t *testing.T) {
	ops := NewDefault()
	if ops == nil {
		t.Fatal("NewDefault returned nil")
	}
	if _, ok := ops.(*OSFileOps); !ok {
		t.Errorf("expected *OSFileOps, got %T", ops)
	}
}
