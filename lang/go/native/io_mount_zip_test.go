package native

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// testZipBytes builds a deflate archive (name→content), deterministically.
func testZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fw, err := w.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(files[n])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// zipMountReg builds a registry with the io words and a runner over it.
func zipMountReg(t *testing.T) (*Registry, func(string) error) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	return r, func(src string) error {
		vals, perr := parser.Parse(src)
		if perr != nil {
			t.Fatalf("parse %q: %v", src, perr)
		}
		_, rerr := NewTop(r).Run(vals)
		return rerr
	}
}

func TestMountZipArchive(t *testing.T) {
	dir := t.TempDir()
	zpath := filepath.Join(dir, "a.zip")
	if err := os.WriteFile(zpath, testZipBytes(t, map[string]string{
		"hello.txt": "world", "sub/x.txt": "deep",
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	r, run := zipMountReg(t)

	// Mount by the ".zip" extension sniff: a read-only archive FS.
	if err := run(fmt.Sprintf(`mount (make Pathon %q)`, zpath)); err != nil {
		t.Fatalf("zip mount: %v", err)
	}
	ops := HostFileOps(r)
	if b, err := ops.ReadFile("hello.txt"); err != nil || string(b) != "world" {
		t.Errorf("zip read = %q (%v)", b, err)
	}
	if b, err := ops.ReadFile("sub/x.txt"); err != nil || string(b) != "deep" {
		t.Errorf("nested zip read = %q (%v)", b, err)
	}
	if err := ops.WriteFile("hello.txt", []byte("no"), 0o644); err == nil {
		t.Error("a read-only zip mount must decline writes")
	}
	if err := run(`unmount`); err != nil {
		t.Fatalf("unmount: %v", err)
	}

	// {writable:true}: a copy-on-write overlay over the archive — the zip is
	// the read-only lower, edits land in a fresh mem upper.
	if err := run(fmt.Sprintf(`mount (make Pathon %q) {writable:true}`, zpath)); err != nil {
		t.Fatalf("writable zip mount: %v", err)
	}
	wops := HostFileOps(r)
	if b, err := wops.ReadFile("hello.txt"); err != nil || string(b) != "world" {
		t.Errorf("overlay reads the zip lower = %q (%v)", b, err)
	}
	if err := wops.WriteFile("new.txt", []byte("edit"), 0o644); err != nil {
		t.Errorf("writable overlay write: %v", err)
	}
	if b, err := wops.ReadFile("new.txt"); err != nil || string(b) != "edit" {
		t.Errorf("overlay upper edit = %q (%v)", b, err)
	}
	if err := run(`unmount`); err != nil {
		t.Fatalf("unmount: %v", err)
	}

	// {zip:true} forces archive mounting regardless of extension.
	apath := filepath.Join(dir, "archive.dat")
	if err := os.WriteFile(apath, testZipBytes(t, map[string]string{"z.txt": "zed"}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(fmt.Sprintf(`mount (make Pathon %q) {zip:true}`, apath)); err != nil {
		t.Fatalf("{zip:true} mount: %v", err)
	}
	if b, err := HostFileOps(r).ReadFile("z.txt"); err != nil || string(b) != "zed" {
		t.Errorf("{zip:true} read = %q (%v)", b, err)
	}
	if err := run(`unmount`); err != nil {
		t.Fatalf("unmount: %v", err)
	}
}

func TestMountZipErrors(t *testing.T) {
	_, run := zipMountReg(t)
	// A Pathon with no zip intent (no ".zip", no {zip:true}) is declined.
	if err := run(`mount (make Pathon "plain.txt")`); err == nil {
		t.Error("a non-zip Pathon mount must error")
	}
	// A ".zip" path that does not exist → the read error surfaces.
	if err := run(`mount (make Pathon "/no/such/archive.zip")`); err == nil {
		t.Error("a missing archive must error")
	}
	// A ".zip" path whose bytes are not a valid archive → the parse error.
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.zip")
	if err := os.WriteFile(bad, []byte("not a zip at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(fmt.Sprintf(`mount (make Pathon %q)`, bad)); err == nil {
		t.Error("invalid archive bytes must error")
	}
}
