package publish

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestS7n_ReadZipFailureAfterSuccessfulPack drives Run's
// os.ReadFile(zipPath) failure arm by swapping the packRun seam:
// pack reports success (code 0) but "writes" a path that doesn't
// exist, so the subsequent read fails.
func TestS7n_ReadZipFailureAfterSuccessfulPack(t *testing.T) {
	orig := packRun
	t.Cleanup(func() { packRun = orig })
	bogusZip := filepath.Join(t.TempDir(), "zz-missing.zip")
	packRun = func(args []string, stdout, stderr io.Writer) int {
		stdout.Write([]byte(bogusZip + "\n"))
		return 0
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: tok\nregistry: http://localhost:9\n")

	var out, errOut bytes.Buffer
	code := Run(nil, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "reading zip") {
		t.Errorf("stderr = %q, want a reading-zip error", errOut.String())
	}
	if _, err := os.Stat(bogusZip); err == nil {
		t.Fatal("test bug: the bogus zip path should not actually exist")
	}
}
