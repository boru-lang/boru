// cliexamples_test.go builds the boru binary once and runs each extracted
// shell example inside a per-example temporary directory, comparing the
// command's stdout to the documented output.
//
// Sandboxing note: a Go test cannot fully OS-sandbox a child process.
// Isolation here is best-effort and layered: each example runs with its
// cwd, HOME, and XDG/TMP env pointed at a fresh temp dir, so file and
// project state can't escape into the repo or the user's home. Examples
// whose subcommand needs the network, a long-running service, a missing
// on-disk file, or other non-deterministic input are skipped up front
// (needsSandboxSkip) rather than executed.
package cliexamples

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// docFiles are the docs scanned for shell `boru …` examples.
var docFiles = []string{"CLI.md", "HOWTO.md"}

func docRoot() string { return filepath.Join("..", "..", "..") }

// buildBoru compiles cmd/go/boru into a temp binary once for the test
// binary's lifetime and returns its path.
func buildBoru(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "boru")
	if runtime.GOOS == "windows" {
		bin += ".exe" // Windows runs a program only by its extension
	}
	cmd := exec.Command("go", "build", "-o", bin, "./boru")
	cmd.Dir = filepath.Join(docRoot(), "cmd", "go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build boru: %v\n%s", err, out)
	}
	return bin
}

func TestCLIExamples(t *testing.T) {
	bin := buildBoru(t)

	// Render-parity sanity check: the binary must print the comma-free
	// canonical form Part A established, matching the doc convention.
	if got := runBoru(t, bin, t.TempDir(), []string{"do", "[1 2 3]"}); got != "[1 2 3]" {
		t.Fatalf("CLI render sanity: got %q, want %q", got, "[1 2 3]")
	}

	ran := 0
	for _, name := range docFiles {
		body, err := os.ReadFile(filepath.Join(docRoot(), name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		examples := Extract(name, string(body))

		t.Run(name, func(t *testing.T) {
			for _, ex := range examples {
				ex := ex
				if reason := needsSandboxSkip(ex); reason != "" {
					continue
				}
				ran++
				t.Run(sanitise(ex.Raw, ex.Line), func(t *testing.T) {
					dir := t.TempDir()
					got := runBoru(t, bin, dir, ex.Args)
					if got != ex.Expected {
						t.Errorf("%s\n  got:  %q\n  want: %q", ex.Raw, got, ex.Expected)
					}
				})
			}
		})
	}
	if ran == 0 {
		t.Error("no runnable CLI examples found — extractor or doc regression?")
	}
}

// needsSandboxSkip returns a non-empty reason when an example can't run
// deterministically in the temp-dir sandbox: it names a service /
// network / vault subcommand, or references a file/registry path that
// doesn't exist in a fresh dir.
func needsSandboxSkip(ex CLIExample) string {
	if len(ex.Args) == 0 {
		return "no args"
	}
	sub := ex.Args[0]
	switch sub {
	case "exec", "lsp", "serve", "login", "logout", "publish", "register",
		"registry", "vault", "install", "prep", "clean":
		return "needs service/network/registry"
	}
	// `boru -e EXPR` (legacy eval flag) and `boru do EXPR` are pure-eval.
	// A bare script path (`boru script.boru`) or `-r ./registry` references
	// on-disk state that isn't present in a fresh temp dir.
	for _, a := range ex.Args {
		if strings.HasSuffix(a, ".boru") || a == "-r" || a == "--registry" {
			return "references on-disk file/registry"
		}
		if strings.Contains(a, "...") || strings.Contains(a, "$") {
			return "placeholder / shell-variable example"
		}
	}
	return ""
}

// runBoru executes the binary in dir with HOME/TMP/cwd redirected there,
// and returns trimmed stdout. A non-zero exit (or stderr-only output) is
// surfaced via the returned string so a mismatch shows the diagnostic.
func runBoru(t *testing.T, bin, dir string, args []string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"HOME="+dir,
		"TMPDIR="+dir,
		"XDG_CONFIG_HOME="+filepath.Join(dir, ".config"),
		"XDG_DATA_HOME="+filepath.Join(dir, ".local", "share"),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		// Surface the error text so the comparison fails with context.
		return strings.TrimSpace(stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}

// sanitise builds a short, filesystem-safe subtest name.
func sanitise(s string, line int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	if len(s) > 40 {
		s = s[:40]
	}
	return "L" + itoa(line) + "_" + s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
