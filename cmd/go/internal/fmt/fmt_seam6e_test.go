package fmt

// fmt_seam6e_test.go — covers Run's walk-error arm (the walkDir seam: an
// injected failure, since deleting the working directory is not portable)
// and the write-failure arm (osWriteFile seam; permissions cannot force it
// as root).

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWalkError(t *testing.T) {
	boom := errors.New("directory unavailable")
	orig := walkDir
	t.Cleanup(func() { walkDir = orig })
	walkDir = func(root string, visit fs.WalkDirFunc) error { return visit(root, nil, boom) }
	var stdout, stderr bytes.Buffer
	if code := Run(nil, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), boom.Error()) {
		t.Errorf("stderr = %q, want walk error", stderr.String())
	}
}

func TestRunWriteFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.boru")
	if err := os.WriteFile(path, []byte("  add   1   2"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := osWriteFile
	osWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	t.Cleanup(func() { osWriteFile = orig })
	var stdout, stderr bytes.Buffer
	if code := Run([]string{path}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "write boom") {
		t.Errorf("stderr = %q, want write boom", stderr.String())
	}
}
