// Wave-4 coverage for the install subcommand: transport and archive
// error arms driven through a hostile in-test registry (invalid zip,
// zip-slip, directory entries, blocked write targets), the post-
// extract failure arms (updateDeps, prep), the flag error, and the
// updateDeps rewrite branches.
package install

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/prep"
)

// zipBytes builds an in-memory zip from name→content pairs. A name
// ending in "/" becomes a directory entry.
func zipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		if strings.HasSuffix(name, "/") {
			if _, err := zw.Create(name); err != nil {
				t.Fatal(err)
			}
			continue
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// serveBody starts a registry stub that answers every module download
// with the given body.
func serveBody(t *testing.T, body []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// setupModuleDir builds a prepped module folder and chdirs into it.
func setupModuleDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "boru.jsonic"), []byte("name: host\nmajor: 0\nminor: 1\npatch: 0\nfiles: [index.boru]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.boru"), []byte("1 add 2"), 0644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	var out, errOut bytes.Buffer
	if code := prep.Run(nil, &out, &errOut); code != 0 {
		t.Fatalf("prep: %s", errOut.String())
	}
	return dir
}

func TestW4CommandMethods(t *testing.T) {
	c := New()
	if c.Name() != "install" {
		t.Errorf("Name() = %q, want install", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
	var out, errOut bytes.Buffer
	if code := c.Run(nil, nil, &out, &errOut); code != 1 {
		t.Errorf("Run(no args) = %d, want 1", code)
	}
}

func TestW4InstallBadFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"-zz-bad-flag"}, &out, &errOut); code != 1 {
		t.Errorf("Run(bad flag) = %d, want 1", code)
	}
}

func TestW4InstallConnectionError(t *testing.T) {
	setupModuleDir(t)
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", "http://127.0.0.1:1", "mod-0.1.0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "error:") {
		t.Errorf("stderr = %q, want a transport error", errOut.String())
	}
}

func TestW4InstallInvalidZip(t *testing.T) {
	setupModuleDir(t)
	url := serveBody(t, []byte("this is not a zip"))
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "invalid zip") {
		t.Errorf("stderr = %q, want invalid zip", errOut.String())
	}
}

func TestW4InstallZipSlipRejected(t *testing.T) {
	setupModuleDir(t)
	url := serveBody(t, zipBytes(t, map[string]string{"../evil.txt": "boom"}))
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "unsafe path") {
		t.Errorf("stderr = %q, want unsafe-path compile failure", errOut.String())
	}
}

func TestW4InstallDirEntriesAndNestedFiles(t *testing.T) {
	dir := setupModuleDir(t)
	url := serveBody(t, zipBytes(t, map[string]string{
		"docs/":          "",
		"docs/notes.txt": "hi",
		"boru.jsonic":    "name: mod\n",
	}))
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".boru", "mod", "docs", "notes.txt")); err != nil {
		t.Errorf("nested file missing: %s", err)
	}
	if !strings.Contains(out.String(), "installed mod@0.1.0") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestW4InstallDestDirBlocked(t *testing.T) {
	dir := setupModuleDir(t)
	// .boru/mod exists as a FILE, so MkdirAll(destDir) fails.
	if err := os.WriteFile(filepath.Join(dir, ".boru", "mod"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	url := serveBody(t, zipBytes(t, map[string]string{"a.txt": "x"}))
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
}

func TestW4InstallWriteFileBlocked(t *testing.T) {
	dir := setupModuleDir(t)
	// destDir/a.txt pre-exists as a DIRECTORY, so WriteFile fails.
	if err := os.MkdirAll(filepath.Join(dir, ".boru", "mod", "a.txt"), 0755); err != nil {
		t.Fatal(err)
	}
	url := serveBody(t, zipBytes(t, map[string]string{"a.txt": "x"}))
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
}

func TestW4InstallUpdateDepsFails(t *testing.T) {
	dir := setupModuleDir(t)
	// Remove boru.jsonic after prep so updateDeps cannot read it.
	if err := os.Remove(filepath.Join(dir, "boru.jsonic")); err != nil {
		t.Fatal(err)
	}
	url := serveBody(t, zipBytes(t, map[string]string{"a.txt": "x"}))
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "updating boru.jsonic") {
		t.Errorf("stderr = %q, want updateDeps error", errOut.String())
	}
}

func TestW4InstallPrepFails(t *testing.T) {
	dir := setupModuleDir(t)
	// Corrupt boru.jsonic so the re-prep after install fails, while
	// updateDeps (a plain text rewrite) still succeeds.
	if err := os.WriteFile(filepath.Join(dir, "boru.jsonic"), []byte("[1 2 3]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	url := serveBody(t, zipBytes(t, map[string]string{"a.txt": "x"}))
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "prep") {
		t.Errorf("stderr = %q, want prep error", errOut.String())
	}
}

func TestW4InstallBodyReadError(t *testing.T) {
	setupModuleDir(t)
	// Content-Length promises more than the handler delivers, so the
	// client's ReadAll fails with an unexpected EOF.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("short"))
	}))
	t.Cleanup(srv.Close)

	var out, errOut bytes.Buffer
	code := Run([]string{"-r", srv.URL, "mod-0.1.0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
}

func TestW4InstallDownloadTooLarge(t *testing.T) {
	setupModuleDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 1<<20) // 1 MiB
		for written := int64(0); written <= maxDownloadBytes; written += int64(len(chunk)) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	var out, errOut bytes.Buffer
	code := Run([]string{"-r", srv.URL, "mod-0.1.0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "exceeds") {
		t.Errorf("stderr = %q, want size-cap error", errOut.String())
	}
}

func TestW4InstallDirEntryBlocked(t *testing.T) {
	dir := setupModuleDir(t)
	// .boru/mod/docs pre-exists as a FILE, so the dir entry's MkdirAll fails.
	if err := os.MkdirAll(filepath.Join(dir, ".boru", "mod"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".boru", "mod", "docs"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	url := serveBody(t, zipBytes(t, map[string]string{"docs/": ""}))
	var out, errOut bytes.Buffer
	if code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut); code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
}

func TestW4InstallParentDirBlocked(t *testing.T) {
	dir := setupModuleDir(t)
	// .boru/mod/sub pre-exists as a FILE, so the file entry's parent
	// MkdirAll fails.
	if err := os.MkdirAll(filepath.Join(dir, ".boru", "mod"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".boru", "mod", "sub"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	url := serveBody(t, zipBytes(t, map[string]string{"sub/a.txt": "x"}))
	var out, errOut bytes.Buffer
	if code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut); code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
}

// zipWithMethod builds a single-entry zip whose entry uses the given
// (non-standard) compression method. Pair with zip.RegisterCompressor
// / RegisterDecompressor to simulate entry open/read failures.
func zipWithMethod(t *testing.T, method uint16, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// passthroughCompressor stores bytes verbatim under a custom method.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func TestW4InstallEntryOpenError(t *testing.T) {
	// Method 60000 has a registered COMPRESSOR (so the archive can be
	// built) but no decompressor, so f.Open fails at extraction.
	zip.RegisterCompressor(60000, func(w io.Writer) (io.WriteCloser, error) {
		return nopWriteCloser{w}, nil
	})
	setupModuleDir(t)
	url := serveBody(t, zipWithMethod(t, 60000, "a.txt", []byte("payload")))
	var out, errOut bytes.Buffer
	if code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut); code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
}

// failAfterReader yields n zero bytes and then an error.
type failAfterReader struct{ n int }

func (r *failAfterReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if len(p) > r.n {
		p = p[:r.n]
	}
	for i := range p {
		p[i] = 0
	}
	r.n -= len(p)
	return len(p), nil
}

func (r *failAfterReader) Close() error { return nil }

func TestW4InstallEntryReadError(t *testing.T) {
	// Method 60001 decompresses into a stream that errors mid-read.
	zip.RegisterCompressor(60001, func(w io.Writer) (io.WriteCloser, error) {
		return nopWriteCloser{w}, nil
	})
	zip.RegisterDecompressor(60001, func(io.Reader) io.ReadCloser {
		return &failAfterReader{n: 16}
	})
	setupModuleDir(t)
	url := serveBody(t, zipWithMethod(t, 60001, "a.txt", []byte("payload")))
	var out, errOut bytes.Buffer
	if code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut); code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
}

func TestW4InstallEntryTooLarge(t *testing.T) {
	// A deflated entry of 64MiB+2 zero bytes: tiny on the wire, but the
	// per-entry extraction cap rejects it as a zip bomb.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "a.txt", Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 1<<20)
	for written := int64(0); written < maxEntryBytes+2; written += int64(len(chunk)) {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	setupModuleDir(t)
	url := serveBody(t, buf.Bytes())
	var out, errOut bytes.Buffer
	if code := Run([]string{"-r", url, "mod-0.1.0"}, &out, &errOut); code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "exceeds") {
		t.Errorf("stderr = %q, want entry-cap error", errOut.String())
	}
}

// --- updateDeps rewrite branches -------------------------------------------

// withJsonic writes content to boru.jsonic in a fresh cwd and returns a
// reader for the rewritten file.
func withJsonic(t *testing.T, content string) func() string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "boru.jsonic"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	return func() string {
		data, err := os.ReadFile(filepath.Join(dir, "boru.jsonic"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
}

func TestW4UpdateDepsInlineReplaceExisting(t *testing.T) {
	read := withJsonic(t, "name: host\ndeps: {color: 0.0.9}\n")
	if err := updateDeps("color", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	got := read()
	if !strings.Contains(got, "color: 0.1.0") || strings.Contains(got, "0.0.9") {
		t.Errorf("inline replace failed: %q", got)
	}
}

func TestW4UpdateDepsInlineAppendNew(t *testing.T) {
	read := withJsonic(t, "name: host\ndeps: {other: 1.0.0}\n")
	if err := updateDeps("color", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	got := read()
	if !strings.Contains(got, "color: 0.1.0") || !strings.Contains(got, "other: 1.0.0") {
		t.Errorf("inline append failed: %q", got)
	}
}

func TestW4UpdateDepsBlockReplaceExisting(t *testing.T) {
	read := withJsonic(t, "name: host\ndeps: \n{\ncolor: 0.0.9\nother: 1.0.0\n}\n")
	if err := updateDeps("color", "0.2.0"); err != nil {
		t.Fatal(err)
	}
	got := read()
	if !strings.Contains(got, "color: 0.2.0") || strings.Contains(got, "0.0.9") {
		t.Errorf("block replace failed: %q", got)
	}
	if !strings.Contains(got, "other: 1.0.0") {
		t.Errorf("unrelated dep lost: %q", got)
	}
}

func TestW4UpdateDepsDepsWordButNoBrace(t *testing.T) {
	// "deps:" present but never found/insertable in-line: falls to the
	// not-found replace of "deps: {".
	read := withJsonic(t, "name: host\ndeps: {}\n")
	if err := updateDeps("color", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	got := read()
	if !strings.Contains(got, "color: 0.1.0") {
		t.Errorf("empty-braces insert failed: %q", got)
	}
}

func TestW4UpdateDepsNoDepsSection(t *testing.T) {
	read := withJsonic(t, "name: host\n")
	if err := updateDeps("color", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	got := read()
	if !strings.Contains(got, "deps: {color: 0.1.0}") {
		t.Errorf("deps section not appended: %q", got)
	}
}

func TestW4UpdateDepsBlockWithoutName(t *testing.T) {
	// A multi-line deps block that never names the module and has no
	// inline "deps: {" — the not-found fallback replace runs (and is a
	// no-op on this shape).
	read := withJsonic(t, "name: host\ndeps:\nother: 1.0.0\n}\n")
	if err := updateDeps("color", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	got := read()
	if !strings.Contains(got, "other: 1.0.0") {
		t.Errorf("existing content lost: %q", got)
	}
}

func TestW4UpdateDepsMissingFile(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)
	if err := updateDeps("color", "0.1.0"); err == nil {
		t.Error("updateDeps without boru.jsonic succeeded, want error")
	}
}
