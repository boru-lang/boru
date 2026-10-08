package build

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/boru-lang/boru/cmd/go/internal/buildrt"
)

// cmdGoModulePath is the import path / module path of the CLI module that owns
// internal/buildrt. A `--native` build must compile inside this module's tree
// so the generated main can import the internal runtime and resolve the
// dependency graph through the module's existing replace directives.
const cmdGoModulePath = "github.com/boru-lang/boru/cmd/go"

// osMkdirTemp is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive buildNative's create-build-dir failure arm, which cannot be
// forced via directory permissions when the suite runs as root.
var osMkdirTemp = os.MkdirTemp

// jsonMarshal is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive buildNative's encode-config failure arm, which is unreachable
// with the plain buildrt.Config shape.
var jsonMarshal = json.Marshal

// osWriteFile is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive buildNative's write-failure arms for config.json and main.go
// (unreachable via permissions as root).
var osWriteFile = os.WriteFile

// buildNative produces the binary via the Go toolchain: write a tiny main.go
// (mainTemplate) plus the baked config.json into a temp package inside the
// cmd/go module tree, then `go build` it. Reuses the module's replace graph so
// we never reconstruct the (unpublished, replace-stitched) dependency set.
func buildNative(cfg buildrt.Config, outPath string, keep bool, stdout, stderr io.Writer) error {
	return buildNativeWithAbs(cfg, outPath, keep, stdout, stderr, filepath.Abs)
}

func buildNativeWithAbs(cfg buildrt.Config, outPath string, keep bool, stdout, stderr io.Writer, abs func(string) (string, error)) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("--native needs the Go toolchain on PATH: %w", err)
	}

	moduleDir, err := findCmdGoModuleDir()
	if err != nil {
		return err
	}

	absOut, err := abs(outPath)
	if err != nil {
		return err
	}

	// The temp package lives inside the module tree (required for the internal
	// import) but is dot-prefixed so `go build ./...` / `go test ./...` over
	// the module skip it.
	tmpDir, err := osMkdirTemp(moduleDir, ".borubuild-")
	if err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}
	if keep {
		fmt.Fprintf(stdout, "build dir: %s\n", tmpDir)
	} else {
		defer os.RemoveAll(tmpDir)
	}

	configJSON, err := jsonMarshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := osWriteFile(filepath.Join(tmpDir, "config.json"), configJSON, 0o644); err != nil {
		return err
	}
	if err := osWriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainTemplate), 0o644); err != nil {
		return err
	}

	// Build the package in place. GOOS/GOARCH from the environment flow through
	// os.Environ(), enabling cross-compilation.
	c := exec.Command("go", "build", "-o", absOut, ".")
	c.Dir = tmpDir
	c.Env = os.Environ()
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("go build failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// findCmdGoModuleDir locates the directory of the cmd/go module so a --native
// build can compile inside it. Resolution order:
//  1. $BORU_SRC/cmd/go (BORU_SRC points at a repo checkout root),
//  2. `go env GOMOD` from the current directory, if it is the cmd/go module.
//
// Returns a clear error when neither is available — a stock installed binary on
// a machine without the source cannot self-build natively.
func findCmdGoModuleDir() (string, error) {
	if src := strings.TrimSpace(os.Getenv("BORU_SRC")); src != "" {
		dir := filepath.Join(src, "cmd", "go")
		if isCmdGoModule(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		return "", fmt.Errorf("BORU_SRC=%q does not contain the cmd/go module (expected %s)", src, filepath.Join(dir, "go.mod"))
	}

	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err == nil {
		goMod := strings.TrimSpace(string(out))
		if goMod != "" && goMod != os.DevNull && isCmdGoModule(goMod) {
			return filepath.Dir(goMod), nil
		}
	}

	return "", fmt.Errorf(
		"--native needs the boru source tree: set BORU_SRC to a repo checkout, or run from within the %s module (the default self-embedding build needs neither)",
		cmdGoModulePath)
}

// isCmdGoModule reports whether goModPath is the cmd/go module's go.mod.
func isCmdGoModule(goModPath string) bool {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")) == cmdGoModulePath
		}
	}
	return false
}
