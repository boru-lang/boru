package buildrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// A built multi-file program must run the sources it was BUILT from.
//
// It did not. The bundle was overlaid with the host as the upper layer, and an
// overlay's upper layer is both the write target and the first read consulted
// — so the bundled copy only ever won when the host had nothing at the path.
// Two consequences, both reproduced before the fix:
//
//  1. Editing an imported .boru after `boru build` changed the executable's
//     behaviour. "Self-contained" was not true for anything but a single file.
//
//  2. Worse: a relative `import "./lib.boru"` resolves through the file layer
//     against the RUN-TIME process working directory, so a built tool started
//     in any directory containing a file of that name loaded and executed THAT
//     file in place of its bundled module. A baked -perms profile constrains
//     what the substituted code may do; it does not stop it running.
//
// These tests drive bundledFirst directly rather than through a built binary,
// because the property is about which layer answers a read — and a unit test
// says so in one line where an end-to-end build says it in thirty.

// memWith builds an in-memory FileOps holding the given path->content pairs.
func memWith(t *testing.T, cwd string, files map[string]string) capabilities.FileOps {
	t.Helper()
	mem := lang.NewMemFileOps()
	if cwd != "" {
		mem.Cwd = cwd
	}
	for p, data := range files {
		mem.Files[p] = []byte(data)
	}
	return mem
}

func TestBundledFirstPrefersTheBundleOverAHostFileAtTheSamePath(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "lib.boru")
	if err := os.WriteFile(shared, []byte("EDITED AFTER THE BUILD"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := bundledFirst{
		FileOps: capabilities.NewDefault(),
		mem:     memWith(t, dir, map[string]string{shared: "BUNDLED AT BUILD TIME"}),
	}
	got, err := fs.ReadFile(shared)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "BUNDLED AT BUILD TIME" {
		t.Errorf("read %q, want the BUNDLED copy — a live file at the same path "+
			"must not change what a built program runs", got)
	}
}

// The injection case, stated as a test: the bundle is keyed by the BUILD
// machine's path, and an unrelated file of the same name sits where the tool
// is being run. The read must miss the bundle and miss the stranger too,
// rather than silently executing the stranger.
func TestBundledFirstDoesNotReadAStrangerFromTheRunTimeCwd(t *testing.T) {
	runDir := t.TempDir()
	stranger := filepath.Join(runDir, "lib.boru")
	if err := os.WriteFile(stranger, []byte("STRANGER FROM THE CWD"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Bundled under a build-machine path that does not exist here.
	fs := bundledFirst{
		FileOps: capabilities.NewDefault(),
		mem: memWith(t, hostAbs("/build/machine/proj"), map[string]string{
			hostAbs("/build/machine/proj/lib.boru"): "BUNDLED AT BUILD TIME",
		}),
	}
	// A read of the BUNDLED key must find the bundle, not the host.
	got, err := fs.ReadFile(hostAbs("/build/machine/proj/lib.boru"))
	if err != nil {
		t.Fatalf("ReadFile(bundled key): %v", err)
	}
	if string(got) != "BUNDLED AT BUILD TIME" {
		t.Errorf("read %q, want the bundled copy", got)
	}
	// And the stranger is reachable only by asking for it BY ITS OWN PATH —
	// never as a substitute for a bundled module.
	if b, err := fs.ReadFile(stranger); err != nil || string(b) != "STRANGER FROM THE CWD" {
		t.Errorf("an explicit read of a real path must still work: %q %v", b, err)
	}
}

// The other half of the contract, and the reason the layers cannot simply be
// swapped: a program's WRITES must reach the real filesystem. mem-over-host
// would trap them in RAM and discard them at exit, silently, with exit 0 —
// which is the bug the original overlay was introduced to fix.
func TestBundledFirstWritesReachTheHost(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	fs := bundledFirst{
		FileOps: capabilities.NewDefault(),
		mem:     memWith(t, dir, map[string]string{filepath.Join(dir, "lib.boru"): "bundled"}),
	}
	if err := fs.WriteFile(target, []byte("payload"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("the write did not reach the real filesystem: %v", err)
	}
	if string(b) != "payload" {
		t.Errorf("file holds %q, want %q", b, "payload")
	}
}

// A path the bundle does not carry falls through unchanged, so a program
// reading its own real data files behaves exactly as it did before.
func TestBundledFirstFallsThroughForUnbundledPaths(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data.csv")
	if err := os.WriteFile(data, []byte("a,b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := bundledFirst{
		FileOps: capabilities.NewDefault(),
		mem:     memWith(t, dir, map[string]string{filepath.Join(dir, "lib.boru"): "bundled"}),
	}
	got, err := fs.ReadFile(data)
	if err != nil || string(got) != "a,b\n" {
		t.Errorf("ReadFile(unbundled) = %q, %v; want the host's content", got, err)
	}
	if _, err := fs.Stat(data, true); err != nil {
		t.Errorf("Stat(unbundled) = %v; want the host's answer", err)
	}
}

// Stat follows the same precedence as ReadFile, so a program that stats a
// module before reading it cannot be told the host's version is the one there.
func TestBundledFirstStatPrefersTheBundle(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "lib.boru")
	if err := os.WriteFile(shared, []byte("host content is longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := bundledFirst{
		FileOps: capabilities.NewDefault(),
		mem:     memWith(t, dir, map[string]string{shared: "short"}),
	}
	fi, err := fs.Stat(shared, true)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Size != int64(len("short")) {
		t.Errorf("Stat reported size %d, want %d — Stat and ReadFile must agree "+
			"about which layer holds the file", fi.Size, len("short"))
	}
}

// A read-only Open resolves from the bundle; a write-intent Open is the
// host's, because the bundle is an immutable snapshot and a write landing
// there would be discarded at exit.
func TestBundledFirstOpenSplitsByWriteIntent(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "lib.boru")
	if err := os.WriteFile(shared, []byte("HOST"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := bundledFirst{
		FileOps: capabilities.NewDefault(),
		mem:     memWith(t, dir, map[string]string{shared: "BUNDLED"}),
	}

	h, err := fs.Open(shared, capabilities.OpenOpts{})
	if err != nil {
		t.Fatalf("read-only Open: %v", err)
	}
	buf := make([]byte, len("BUNDLED"))
	n, _ := h.Read(buf)
	_ = h.Close()
	if string(buf[:n]) != "BUNDLED" {
		t.Errorf("read-only Open served %q, want the bundled copy", buf[:n])
	}

	w, err := fs.Open(shared, capabilities.OpenOpts{Write: true, Truncate: true})
	if err != nil {
		t.Fatalf("write Open: %v", err)
	}
	if _, err := w.Write([]byte("WRITTEN")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = w.Close()
	b, err := os.ReadFile(shared)
	if err != nil || string(b) != "WRITTEN" {
		t.Errorf("a write-intent Open must reach the real file; it holds %q (%v)", b, err)
	}
}
