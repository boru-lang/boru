package check

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/policy"
)

// NUR079: the analysis commands accept the permission flags they document.
// A check EXECUTES an imported module's body to learn its exports, so the
// profile that governs the run governs the check — `--perms`, and the
// BORU_POLICY fallback `boru run` honours — and a refused import is
// reported as the error the run would raise.
func TestCheckHonoursPermissionFlags(t *testing.T) {
	const src = `import "boru:net" 1`
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--perms", "read-only", "-e", src}, &stdout, &stderr); code != 1 {
		t.Fatalf("--perms read-only must refuse the import: exit %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	if out := stdout.String() + stderr.String(); !strings.Contains(out, "permission_denied") {
		t.Errorf("the refusal is reported by its code, got:\n%s", out)
	}

	stdout.Reset()
	stderr.Reset()
	t.Setenv("BORU_POLICY", "read-only")
	if code := RunCLI([]string{"-e", src}, &stdout, &stderr); code != 1 {
		t.Errorf("BORU_POLICY must be honoured like --perms: exit %d", code)
	}

	// Negatives: no policy checks the same import clean, and so does a
	// profile that admits it.
	t.Setenv("BORU_POLICY", "")
	for _, args := range [][]string{{"-e", src}, {"--perms", "trusted", "-e", src}} {
		stdout.Reset()
		stderr.Reset()
		if code := RunCLI(args, &stdout, &stderr); code != 0 {
			t.Errorf("%v: want a clean check, exit %d\n%s%s", args, code, stdout.String(), stderr.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := RunCLI([]string{"--perms", "no-such-profile", "-e", src}, &stdout, &stderr); code != 1 {
		t.Errorf("an unresolvable profile is an error, exit %d", code)
	}
}

// The run's pre-flight executes the same analysis, so it runs under the
// policy the run resolves (PreflightPolicyAt), and refuses what the run
// would refuse before the run starts.
func TestPreflightRunsUnderPolicy(t *testing.T) {
	pol, err := policy.Load("read-only")
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if err := PreflightPolicyAt(&stderr, `import "boru:net" 1`, "", 0, false, false, "", pol); err == nil {
		t.Error("the pre-flight under read-only must refuse the import")
	}
	stderr.Reset()
	if err := PreflightPolicyAt(&stderr, `import "boru:net" 1`, "", 0, false, false, "", nil); err != nil {
		t.Errorf("with no policy the pre-flight passes: %v", err)
	}
}
