package fmt

// fmt_seam6e_test.go — covers Run's walk-error arms (injected callback failure) and the
// write-failure arm (osWriteFile seam; permissions cannot force it as root).

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWalkError(t *testing.T) {
	boom := errors.New("directory unavailable")
	walk := func(root string, visit filepath.WalkFunc) error {
		return visit(root, nil, boom)
	}
	var stdout, stderr bytes.Buffer
	if code := runWithWalk(nil, &stdout, &stderr, walk); code != 1 {
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
