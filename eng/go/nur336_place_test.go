package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur336_place_test.go pins placedAsIs, the test the shaped paren apply
// (callDynMethod) runs over an apply whose results miss the claimed count:
// its results are the lead and the values after it, unchanged and in the
// paren's order, only when the lead took none of them and nothing ran — the
// interpreter's paren placed them, and the statement's island answers that
// shape (NUR336). The lang suite drives the arm end to end
// (TestNUR336PlacedParenApply).
func TestPlacedAsIs(t *testing.T) {
	val := func(id string) core.Value {
		v := core.NewInteger(1)
		v.ID = id
		return v
	}
	fn, a, b := val("fn"), val("a"), val("b")
	for _, c := range []struct {
		name    string
		results []core.Value
		want    bool
	}{
		{"the lead then the values", []core.Value{fn, a, b}, true},
		{"one value fewer", []core.Value{fn, a}, false},
		{"another lead", []core.Value{a, a, b}, false},
		{"the values out of order", []core.Value{fn, b, a}, false},
	} {
		if got := placedAsIs(c.results, fn, []core.Value{a, b}); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
