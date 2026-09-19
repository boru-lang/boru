// Wave-4 coverage for the do subcommand: evaluating a joined
// expression, the empty-expression and flag errors, permission-flag
// resolution failure, and evaluation errors.
package do

import (
	"bytes"
	"strings"
	"testing"
)

func TestW4CommandMethods(t *testing.T) {
	c := New()
	if c.Name() != "do" {
		t.Errorf("Name() = %q, want do", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
}

func TestW4DoEvaluatesExpression(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"1", "add", "2"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "3") {
		t.Errorf("stdout = %q, want it to contain 3", stdout.String())
	}
}

// `boru do` compiles and runs its expression. The --compile/--no-compile/
// --force-compile family is gone with the fallback it selected between.
func TestW4DoEvaluatesCompiled(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"2", "mul", "3"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run() = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "6") {
		t.Errorf("stdout = %q, want it to contain 6", stdout.String())
	}
}

func TestW4DoEmptyExpression(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := New().Run(nil, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run() = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "requires an expression") {
		t.Errorf("stderr = %q, want requires-an-expression", stderr.String())
	}
}

func TestW4DoBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"--zz-not-a-flag", "1"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(--zz-not-a-flag) = %d, want 1", code)
	}
}

func TestW4DoPermsConflict(t *testing.T) {
	// --perms and --perms-inline are mutually exclusive, so Resolve fails.
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"--perms", "none", "--perms-inline", "{}", "1"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(perms conflict) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a resolve error", stderr.String())
	}
}

func TestW4DoEvalError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"zz-undefined-word"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(undefined word) = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Error("expected an error on stderr")
	}
}
