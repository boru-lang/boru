package lang

import (
	"fmt"
	"testing"
)

// TestTypedDefLiteralKeepsItsBrandInBothOrders pins NUR080: a typed def over
// an Integer literal (`def b:UserId 9`) brands the binding and leaves the
// bare literal an Integer, whichever is read first, on both lanes — the
// const-pool reparenting the record traced (a literal's identity changing
// with a typed def elsewhere in the program) no longer happens.
func TestTypedDefLiteralKeepsItsBrandInBothOrders(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"def UserId (refine Integer)  def b:UserId 9  typeof 9  typeof b  b is UserId", "[Integer UserId true]"},
		{"def UserId (refine Integer)  def b:UserId 9  typeof b  typeof 9", "[UserId Integer]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
		requireEngineParity(t, c.src, true)
	}
}
