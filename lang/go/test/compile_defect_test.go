package test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// runReference / runReferenceErr run src on the interpreter — the language's
// reference engine. They date from the bytecode lane, when this package's
// tests reached the reference answer through a compile-defect ledger that
// booked every program the compiler declined; the lane is gone and `Run` IS
// the reference engine, so the two are plain runs kept under the names the
// tests use.
func runReference(t *testing.T, a *lang.Boru, src string) ([]any, error) {
	t.Helper()
	return a.Run(src)
}

// runReferenceErr is runReference for a test that expects the program to
// FAIL with a particular error.
func runReferenceErr(t *testing.T, a *lang.Boru, src string) error {
	t.Helper()
	_, err := a.Run(src)
	return err
}
