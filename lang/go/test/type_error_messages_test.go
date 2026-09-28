package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// --- Typed-def error messages name the type ---
//
// `def n:Bbd "e"` previously errored with "value does not satisfy
// predicate type" — the user had to remember which type was at the
// colon. The handler now captures the source name when the
// constraint resolves through a word lookup and surfaces it in the
// error: "def n: value 'e' does not satisfy predicate refine Bbd".

func runErr(t *testing.T, src string) error {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(src)
	if err == nil {
		t.Fatalf("expected an error from %q, got none", src)
	}
	return err
}

func TestErrorMessage_PredicateNamesType(t *testing.T) {
	err := runErr(t, `def Bbd fnpred x:Any [if ((x is String) and (x gte "b") and (x lte "d")) [x] [None]]
def n:Bbd "e"`)
	msg := err.Error()
	if !strings.Contains(msg, "Bbd") {
		t.Errorf("error %q does not mention type name Bbd", msg)
	}
	if !strings.Contains(msg, "'e'") {
		t.Errorf("error %q does not mention the offending value 'e'", msg)
	}
	if !strings.Contains(msg, "predicate type") {
		t.Errorf("error %q does not say it's a predicate type", msg)
	}
}

func TestErrorMessage_DepScalarNamesType(t *testing.T) {
	err := runErr(t, `def G10 (Integer gt 10)
def n:G10 5`)
	msg := err.Error()
	if !strings.Contains(msg, "G10") {
		t.Errorf("error %q does not mention type name G10", msg)
	}
	if !strings.Contains(msg, "5") {
		t.Errorf("error %q does not mention the offending value 5", msg)
	}
}

// When the constraint is a built-in type used directly (no user
// `type` alias), the error falls back to the rendered type form.
// Still informative — just doesn't have a friendlier alias.
func TestErrorMessage_BuiltinFallback(t *testing.T) {
	err := runErr(t, `def n:Integer "hi"`)
	msg := err.Error()
	if !strings.Contains(msg, "Integer") {
		t.Errorf("error %q does not mention Integer", msg)
	}
	if !strings.Contains(msg, "'hi'") {
		t.Errorf("error %q does not mention the offending value 'hi'", msg)
	}
}
