package langspec

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// newDifferentialInstance builds a fresh production instance under the spec
// clock — the isolated instance every per-row gate runs a program on.
func newDifferentialInstance(t testing.TB) *lang.Boru {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	a.SetClock(specClock)
	return a
}
