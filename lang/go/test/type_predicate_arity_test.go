package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// Integration tests for the predicate-fn invocation path. The
// engine-level util_test.go covers structural error branches in
// isolation; this file pins behaviour for predicate bodies that
// return the wrong number of values — the "must return exactly one
// value" branch in RunPredicate.

// A predicate body that pushes two values returns 2 — the engine
// wrap should report that as a non-match with a clear error.
func TestRunPredicate_TwoReturns(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def Two fnpred x:Any [x x]
def n:Two 1`)
	if err == nil {
		t.Fatalf("expected error for predicate returning 2 values")
	}
	if !strings.Contains(err.Error(), "exactly one value") {
		t.Errorf("error %q does not say 'exactly one value'", err)
	}
}
