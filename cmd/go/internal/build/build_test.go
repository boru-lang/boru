package build

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/buildrt"
)

// --- defaultOutput ---

func TestDefaultOutput(t *testing.T) {
	cases := map[string]string{
		"prog.boru":      "prog",
		"a/b/c.boru":     "c",
		"/x/y/main.boru": "main",
		"noext":          "noext",
	}
	for in, want := range cases {
		if got := defaultOutput(in); got != want {
			t.Errorf("defaultOutput(%q)=%q, want %q", in, got, want)
		}
	}
}

// --- buildConfig + import bundling ---

func TestBuildConfigSingleFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "p.boru")
	if err := os.WriteFile(src, []byte("add 1 2"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := buildConfig(src, "", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source != "add 1 2" {
		t.Fatalf("Source=%q", cfg.Source)
	}
	if cfg.Files != nil {
		t.Fatalf("single-file program should carry no Files map, got %v", cfg.Files)
	}
	if cfg.EntryDir != dir {
		t.Fatalf("EntryDir=%q, want %q", cfg.EntryDir, dir)
	}
}

func TestBuildConfigBundlesTransitiveImports(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "main.boru", `import "./lib.boru"`+"\n"+`Lib.x`)
	write(t, dir, "lib.boru", `import "./dep.boru"`+"\n"+`export "Lib" {x:1}`)
	write(t, dir, "dep.boru", `export "Dep" {y:2}`)

	cfg, err := buildConfig(filepath.Join(dir, "main.boru"), "", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main.boru", "lib.boru", "dep.boru"} {
		key := filepath.Join(dir, name)
		if _, ok := cfg.Files[key]; !ok {
			t.Errorf("expected %s in bundled Files", key)
		}
	}
	if len(cfg.Files) != 3 {
		t.Fatalf("expected 3 bundled files, got %d", len(cfg.Files))
	}
}

func TestBuildConfigSkipsBuiltinImports(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.boru", `import "boru:math-util"`+"\n"+`MathUtil.sqrt 16.0`)
	cfg, err := buildConfig(filepath.Join(dir, "p.boru"), "", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Files != nil {
		t.Fatalf("built-in boru: import must not bundle files, got %v", cfg.Files)
	}
}

func TestBuildConfigRejectsBareImport(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.boru", `import "somepkg"`+"\n"+`1`)
	_, err := buildConfig(filepath.Join(dir, "p.boru"), "", 0, "", nil)
	if err == nil {
		t.Fatal("expected an error for an unbundlable bare-module import")
	}
}

func TestBuildConfigMissingImportFails(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "p.boru", `import "./missing.boru"`+"\n"+`1`)
	_, err := buildConfig(filepath.Join(dir, "p.boru"), "", 0, "", nil)
	if err == nil {
		t.Fatal("expected an error when a bundled import does not exist")
	}
}

func TestBuildConfigMissingEntryFails(t *testing.T) {
	_, err := buildConfig(filepath.Join(t.TempDir(), "nope.boru"), "", 0, "", nil)
	if err == nil {
		t.Fatal("expected an error for a missing entry file")
	}
}

// --- generated template is valid Go ---

func TestMainTemplateParses(t *testing.T) {
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "main.go", mainTemplate, parser.AllErrors); err != nil {
		t.Fatalf("generated main template is not valid Go: %v", err)
	}
}

// --- self-embed round-trip without a binary ---

func TestSelfEmbedPayloadDetectable(t *testing.T) {
	// Build a fake "host binary", append a payload the way buildSelfEmbed
	// does, and confirm the runtime detects and decodes it.
	cfg := buildrt.Config{Source: "add 1 2"}
	payload, err := buildrt.EncodePayload(cfg)
	if err != nil {
		t.Fatal(err)
	}
	image := append([]byte("\x7fELF fake host binary bytes"), payload...)

	out := filepath.Join(t.TempDir(), "fake-built")
	if err := os.WriteFile(out, image, 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok, err := buildrt.ReadEmbeddedPayload(out)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected the appended payload to be detected")
	}
	if got.Source != "add 1 2" {
		t.Fatalf("Source=%q", got.Source)
	}
}

// --- end-to-end self-embed build (shells out: gated) ---

func TestBuildSelfEmbedEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end self-embed build in -short mode")
	}
	// buildSelfEmbed copies os.Executable(); under `go test` that is the test
	// binary, which has no embedded CLI dispatch — so we cannot run the
	// produced file as a program here. We assert the produced file exists, is
	// executable, and carries a decodable payload. The full run-the-binary
	// path is exercised by the manual verification in the plan.
	dir := t.TempDir()
	src := write(t, dir, "p.boru", "add 1 2")
	out := filepath.Join(dir, "p")

	cfg, err := buildConfig(src, "", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := buildSelfEmbed(cfg, out); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("produced file is not executable")
	}
	if _, ok, err := buildrt.ReadEmbeddedPayload(out); err != nil || !ok {
		t.Fatalf("produced file has no detectable payload (ok=%v err=%v)", ok, err)
	}
}

// --- end-to-end native build (shells out to go build: gated) ---

func TestBuildNativeEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping native build in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	moduleDir, err := findCmdGoModuleDir()
	if err != nil {
		t.Skipf("no cmd/go module context: %v", err)
	}
	_ = moduleDir

	dir := t.TempDir()
	src := write(t, dir, "p.boru", "add 1 2")
	out := filepath.Join(dir, "p")

	cfg, err := buildConfig(src, "", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := buildNative(cfg, out, false, &stdout, &stderr); err != nil {
		t.Fatalf("buildNative: %v", err)
	}
	got, err := exec.Command(out).CombinedOutput()
	if err != nil {
		t.Fatalf("running produced binary: %v\n%s", err, got)
	}
	if string(bytes.TrimSpace(got)) != "3" {
		t.Fatalf("produced binary output=%q, want 3", got)
	}
}

// write writes name with content under dir and returns its full path.
func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
