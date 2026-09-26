package lsp

import (
	"errors"
	"io"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestS7n_ComputeDiagnosticsLangNewFailure drives computeDiagnostics's
// init-failure arm (diagnostics.go) by swapping the langNew seam so
// lang.New fails; the server must synthesise a single boru/init
// diagnostic carrying the underlying error message.
func TestS7n_ComputeDiagnosticsLangNewFailure(t *testing.T) {
	orig := langNew
	t.Cleanup(func() { langNew = orig })
	langNew = func(opts ...lang.Options) (*lang.Boru, error) {
		return nil, errors.New("boom-init")
	}

	s := newServer(strings.NewReader(""), io.Discard, io.Discard)
	diags := s.computeDiagnostics("1 add 2")
	if len(diags) != 1 {
		t.Fatalf("diags = %+v, want exactly 1", diags)
	}
	d := diags[0]
	if d.Code != "boru/init" {
		t.Errorf("Code = %q, want boru/init", d.Code)
	}
	if d.Severity != severityError {
		t.Errorf("Severity = %v, want severityError", d.Severity)
	}
	if !strings.Contains(d.Message, "boom-init") {
		t.Errorf("Message = %q, want it to contain boom-init", d.Message)
	}
}

// NUR079: the check the language server runs executes an imported module's
// body, so it runs under the documented environment policy; one that cannot
// be resolved is the init failure, not a silently ungated check — and a
// completion's truncate-and-check offers nothing from it.
func TestComputeDiagnosticsEnvPolicyFailure(t *testing.T) {
	t.Setenv("BORU_POLICY", "no-such-profile")
	s := newServer(strings.NewReader(""), io.Discard, io.Discard)
	diags := s.computeDiagnostics("1 add 2")
	if len(diags) != 1 || diags[0].Code != "boru/init" {
		t.Fatalf("want one boru/init diagnostic, got %+v", diags)
	}
	if _, err := newCompletionBoru(); err == nil {
		t.Error("a completion instance must not be built under an unresolvable policy")
	}
	// Negative: a resolvable policy checks as before.
	t.Setenv("BORU_POLICY", "read-only")
	if diags := s.computeDiagnostics("1 add 2"); len(diags) != 0 {
		t.Errorf("a clean program under read-only has no diagnostics, got %+v", diags)
	}
	if _, err := newCompletionBoru(); err != nil {
		t.Errorf("a resolvable policy builds the completion instance: %v", err)
	}
}
