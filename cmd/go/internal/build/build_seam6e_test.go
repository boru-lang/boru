package build

// build_seam6e_test.go — seam-arm tests (design/TEST-SEAMS.10.md) for the
// blocks unreachable through the public surface: the --options parse-error
// arm, buildSelfEmbed's locate/read/nested/encode arms, buildConfig's
// Abs-failure arm, collectImports' seen/recursion arms, and native.go's
// toolchain/moduledir/tempdir/marshal/write/go-build failure arms.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/buildrt"
)

// fakeModuleTree builds a directory shaped like a repo checkout containing
// the cmd/go module (just its go.mod), suitable as a BORU_SRC target so
// native-build tests never write inside the real module tree.
func fakeModuleTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "cmd", "go")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+cmdGoModulePath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func testCfg(t *testing.T) buildrt.Config {
	t.Helper()
	dir := t.TempDir()
	src := write(t, dir, "p.boru", "add 1 2")
	cfg, err := buildConfig(src, "", 0, buildrt.CompileOff, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// --- Run: --options jsonic parse error (ParseOptions arm) ---

func TestRunOptionsParseError(t *testing.T) {
	dir := t.TempDir()
	src := write(t, dir, "p.boru", "add 1 2")
	code, _, stderr := runBuild(t, "-options", "]", src)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "error:") {
		t.Errorf("stderr = %q, want parse error", stderr)
	}
}

// --- Run: -native error and success arms ---

func TestRunNativeError(t *testing.T) {
	t.Setenv("BORU_SRC", filepath.Join(t.TempDir(), "nowhere"))
	dir := t.TempDir()
	src := write(t, dir, "p.boru", "add 1 2")
	code, _, stderr := runBuild(t, "-native", "-o", filepath.Join(dir, "p"), src)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "BORU_SRC") {
		t.Errorf("stderr = %q, want BORU_SRC error", stderr)
	}
}

func TestRunNativeSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping native build in -short mode")
	}
	if _, err := findCmdGoModuleDir(); err != nil {
		t.Skipf("no cmd/go module context: %v", err)
	}
	dir := t.TempDir()
	src := write(t, dir, "p.boru", "add 1 2")
	out := filepath.Join(dir, "p")
	code, stdout, stderr := runBuild(t, "-native", "-o", out, src)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "wrote "+out) {
		t.Errorf("stdout = %q, want wrote line", stdout)
	}
}

// --- buildConfig: filepath.Abs failure (injected at resolution) ---

func TestBuildConfigAbsError(t *testing.T) {
	boom := errors.New("absolute path unavailable")
	resolve := func(string) (string, error) { return "", boom }
	if _, err := buildConfigWithAbs("rel.boru", "", 0, buildrt.CompileOff, "", nil, resolve); !errors.Is(err, boom) {
		t.Fatalf("buildConfig must propagate resolution failure: %v", err)
	}
}

// --- collectImports: seen-skip and recursive-error arms ---

func TestCollectImportsCycleAndNestedError(t *testing.T) {
	dir := t.TempDir()
	// a imports b, b imports a (cycle -> seen-skip arm).
	a := write(t, dir, "a.boru", `import "./b.boru"`)
	write(t, dir, "b.boru", `import "./a.boru"`)
	cfg, err := buildConfig(a, "", 0, buildrt.CompileOff, "", nil)
	if err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(cfg.Files) != 2 {
		t.Errorf("cycle: bundled %d files, want 2", len(cfg.Files))
	}

	// c imports d, d imports a missing file (recursive-error arm).
	c := write(t, dir, "c.boru", `import "./d.boru"`)
	write(t, dir, "d.boru", `import "./missing.boru"`)
	if _, err := buildConfig(c, "", 0, buildrt.CompileOff, "", nil); err == nil {
		t.Fatal("nested missing import: want error, got nil")
	}
}

// --- buildSelfEmbed: locate/read/nested/encode arms ---

func TestSelfEmbedLocateSelfError(t *testing.T) {
	orig := osExecutable
	osExecutable = func() (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { osExecutable = orig })
	err := buildSelfEmbed(testCfg(t), filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "locate self") {
		t.Fatalf("err = %v, want locate self error", err)
	}
}

func TestSelfEmbedReadSelfError(t *testing.T) {
	orig := osExecutable
	missing := filepath.Join(t.TempDir(), "no-such-binary")
	osExecutable = func() (string, error) { return missing, nil }
	t.Cleanup(func() { osExecutable = orig })
	err := buildSelfEmbed(testCfg(t), filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "read self") {
		t.Fatalf("err = %v, want read self error", err)
	}
}

func TestSelfEmbedNestedPayloadRejected(t *testing.T) {
	// Craft a fake "already built" binary: any image with a payload.
	payload, err := buildrt.EncodePayload(buildrt.Config{Source: "add 1 2"})
	if err != nil {
		t.Fatal(err)
	}
	built := filepath.Join(t.TempDir(), "built")
	if err := os.WriteFile(built, append([]byte("#!image"), payload...), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := osExecutable
	osExecutable = func() (string, error) { return built, nil }
	t.Cleanup(func() { osExecutable = orig })
	err = buildSelfEmbed(testCfg(t), filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "itself a built executable") {
		t.Fatalf("err = %v, want nested-payload error", err)
	}
}

func TestSelfEmbedEncodeError(t *testing.T) {
	orig := encodePayload
	encodePayload = func(buildrt.Config) ([]byte, error) { return nil, errors.New("encode boom") }
	t.Cleanup(func() { encodePayload = orig })
	err := buildSelfEmbed(testCfg(t), filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("err = %v, want encode boom", err)
	}
}

// --- buildNative failure arms ---

func TestNativeNoGoToolchain(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir: no `go` binary
	var stdout, stderr bytes.Buffer
	err := buildNative(buildrt.Config{}, "out", false, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "Go toolchain") {
		t.Fatalf("err = %v, want toolchain error", err)
	}
}

func TestNativeModuleDirError(t *testing.T) {
	t.Setenv("BORU_SRC", filepath.Join(t.TempDir(), "nowhere"))
	var stdout, stderr bytes.Buffer
	err := buildNative(buildrt.Config{}, "out", false, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "BORU_SRC") {
		t.Fatalf("err = %v, want BORU_SRC error", err)
	}
}

func TestNativeAbsError(t *testing.T) {
	t.Setenv("BORU_SRC", fakeModuleTree(t))
	boom := errors.New("absolute path unavailable")
	resolve := func(string) (string, error) { return "", boom }
	var stdout, stderr bytes.Buffer
	if err := buildNativeWithAbs(buildrt.Config{}, "relative-out", false, &stdout, &stderr, resolve); !errors.Is(err, boom) {
		t.Fatalf("buildNative must propagate resolution failure: %v", err)
	}
}

func TestNativeMkdirTempError(t *testing.T) {
	t.Setenv("BORU_SRC", fakeModuleTree(t))
	orig := osMkdirTemp
	osMkdirTemp = func(dir, pattern string) (string, error) { return "", errors.New("mkdirtemp boom") }
	t.Cleanup(func() { osMkdirTemp = orig })
	var stdout, stderr bytes.Buffer
	err := buildNative(buildrt.Config{}, "out", false, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "create build dir") {
		t.Fatalf("err = %v, want create build dir error", err)
	}
}

func TestNativeKeepPrintsDirAndMarshalError(t *testing.T) {
	t.Setenv("BORU_SRC", fakeModuleTree(t))
	orig := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }
	t.Cleanup(func() { jsonMarshal = orig })
	var stdout, stderr bytes.Buffer
	err := buildNative(buildrt.Config{}, "out", true, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "encode config") {
		t.Fatalf("err = %v, want encode config error", err)
	}
	if !strings.Contains(stdout.String(), "build dir:") {
		t.Errorf("stdout = %q, want keep-mode build dir line", stdout.String())
	}
}

func TestNativeWriteConfigError(t *testing.T) {
	t.Setenv("BORU_SRC", fakeModuleTree(t))
	orig := osWriteFile
	osWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	t.Cleanup(func() { osWriteFile = orig })
	var stdout, stderr bytes.Buffer
	err := buildNative(buildrt.Config{}, "out", false, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "write boom") {
		t.Fatalf("err = %v, want write boom", err)
	}
}

func TestNativeWriteMainError(t *testing.T) {
	t.Setenv("BORU_SRC", fakeModuleTree(t))
	orig := osWriteFile
	osWriteFile = func(path string, data []byte, perm os.FileMode) error {
		if strings.HasSuffix(path, "main.go") {
			return errors.New("main write boom")
		}
		return orig(path, data, perm)
	}
	t.Cleanup(func() { osWriteFile = orig })
	var stdout, stderr bytes.Buffer
	err := buildNative(buildrt.Config{}, "out", false, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "main write boom") {
		t.Fatalf("err = %v, want main write boom", err)
	}
}

func TestNativeGoBuildFails(t *testing.T) {
	t.Setenv("BORU_SRC", fakeModuleTree(t))
	t.Setenv("GOOS", "bogus-goos") // rejected before any compilation
	var stdout, stderr bytes.Buffer
	err := buildNative(buildrt.Config{}, filepath.Join(t.TempDir(), "out"), false, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "go build failed") {
		t.Fatalf("err = %v, want go build failed", err)
	}
}

// --- findCmdGoModuleDir / isCmdGoModule arms ---

func TestFindCmdGoModuleDirViaBoruSrc(t *testing.T) {
	root := fakeModuleTree(t)
	t.Setenv("BORU_SRC", root)
	dir, err := findCmdGoModuleDir()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if want := filepath.Join(root, "cmd", "go"); dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
}

func TestFindCmdGoModuleDirNoContext(t *testing.T) {
	t.Setenv("BORU_SRC", "")
	t.Chdir(t.TempDir()) // outside any module: `go env GOMOD` -> /dev/null
	if _, err := findCmdGoModuleDir(); err == nil {
		t.Fatal("want error outside module context, got nil")
	}
}

func TestIsCmdGoModule(t *testing.T) {
	if isCmdGoModule(filepath.Join(t.TempDir(), "absent", "go.mod")) {
		t.Error("missing go.mod: want false")
	}
	noModule := write(t, t.TempDir(), "go.mod", "// no module line\n")
	if isCmdGoModule(noModule) {
		t.Error("go.mod without module line: want false")
	}
	other := write(t, t.TempDir(), "go.mod", "module example.com/other\n")
	if isCmdGoModule(other) {
		t.Error("other module: want false")
	}
}
