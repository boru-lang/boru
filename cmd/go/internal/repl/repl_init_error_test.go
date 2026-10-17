package repl

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// The session instance's construction-error arm, driven through the
// langNewFromRegistry seam: the production registry is never nil, so
// NewFromRegistry never refuses it here. The REPL reports the failure and
// stops before reading a line; with the real constructor the same line
// evaluates.
func TestStartInstanceInitErrorReported(t *testing.T) {
	prev := langNewFromRegistry
	t.Cleanup(func() { langNewFromRegistry = prev })
	langNewFromRegistry = func(*native.Registry) (*lang.Boru, error) {
		return nil, errors.New("boom")
	}
	out := &bytes.Buffer{}
	Start(strings.NewReader("1 add 2\n"), out, "")
	if !strings.Contains(out.String(), "init error: boom") {
		t.Fatalf("expected the init-error arm, got %q", out.String())
	}

	langNewFromRegistry = prev
	out.Reset()
	Start(strings.NewReader("1 add 2\n"), out, "")
	if strings.Contains(out.String(), "init error") || !strings.Contains(out.String(), "3") {
		t.Fatalf("the real constructor must evaluate the line, got %q", out.String())
	}
}
