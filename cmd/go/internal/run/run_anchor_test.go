package run

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NUR083: `boru run sub/m.boru` resolves the script's relative imports
// against the SCRIPT's directory, as `check` and `build` do — not against
// the process cwd (the test process runs in the package directory, which
// holds no lib.boru).
func TestRunAnchorsRelativeImportsAtTheScript(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "lib.boru"), []byte("export \"Lib\" {x: 41}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := filepath.Join(sub, "m.boru")
	if err := os.WriteFile(m, []byte("import \"./lib.boru\"\nLib.x add 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Execute([]string{m}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, errb.String())
	}
	if got := strings.TrimSpace(out.String()); got != "42" {
		t.Fatalf("stdout = %q, want 42", got)
	}
}
