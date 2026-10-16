package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// trimUnnamedArgs (NUR255): the frame keeps nret values off the top and
// drops up to the unnamed count of extras from the bottom; a residual the
// unnamed args cannot account for, and a 0-return frame, are left as they
// are.
func TestTrimUnnamedArgs(t *testing.T) {
	a, b, c := core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)
	if got := trimUnnamedArgs([]core.Value{a, b}, 1, 1); len(got) != 1 || got[0].String() != "2" {
		t.Errorf("one unnamed arg beneath one result: %v", got)
	}
	if got := trimUnnamedArgs([]core.Value{a, b, c}, 1, 1); len(got) != 3 {
		t.Errorf("more extras than unnamed args stay for the count error: %v", got)
	}
	if got := trimUnnamedArgs([]core.Value{a}, 0, 1); len(got) != 1 {
		t.Errorf("a 0-return frame is left as it is: %v", got)
	}
	if got := trimUnnamedArgs([]core.Value{a}, 1, 1); len(got) != 1 {
		t.Errorf("no extra, no trim: %v", got)
	}
}

// emptyBodyResidual (NUR258): an empty body's frame is its unnamed args, in
// order; named params contribute nothing.
func TestEmptyBodyResidual(t *testing.T) {
	a, b, c := core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)
	got := emptyBodyResidual([]string{"", "x", ""}, []core.Value{a, b, c})
	if len(got) != 2 || got[0].String() != "1" || got[1].String() != "3" {
		t.Errorf("the unnamed args in order: %v", got)
	}
	if got := emptyBodyResidual([]string{"x"}, []core.Value{a, b}); len(got) != 1 || got[0].String() != "2" {
		t.Errorf("an arg past the named params is unnamed: %v", got)
	}
	if got := emptyBodyResidual([]string{"x"}, []core.Value{a}); got != nil {
		t.Errorf("every param named: nothing, got %v", got)
	}
}
