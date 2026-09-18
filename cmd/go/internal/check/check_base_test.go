package check

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// --base anchors a target's relative imports where `boru` (run) anchors
// them: the cwd the caller names, not the file's own directory. The kg
// pipeline's tests are the witness — tests/x_test.boru imports
// "./util.boru" from its parent, runs from kg/, and checked from the same
// place found nothing and reported every namespace word undefined.
func TestRunCLIBaseAnchorsRelativeImports(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	util := filepath.Join(root, "util.boru")
	if err := os.WriteFile(util, []byte("def k 1\nexport \"U\" {k: k}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "tests", "t.boru")
	if err := os.WriteFile(target, []byte("import \"./util.boru\"\nU.k\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--base", root, target}, &stdout, &stderr); code != 0 {
		t.Fatalf("--base %s: exit %d, want 0\nstderr: %s", root, code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	// Without the flag the import anchors on tests/, where util.boru is not,
	// and the checker reports the namespace undefined.
	if code := RunCLI([]string{target}, &stdout, &stderr); code != 1 {
		t.Fatalf("no --base: exit %d, want 1 (the import anchors on the file's directory)\nstderr: %s", code, stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("undefined_word")) {
		t.Errorf("no --base: want an undefined_word diagnostic for the unresolved namespace, got:\n%s", stderr.String())
	}
}
