package eng

import (
	"fmt"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// vm_seat_below_mark_test.go pins OpSeatBelowMark — the op that seats a
// residual's INERT PREFIX beneath a runtime-variadic REGION (NUR067's
// consuming half).
//
// The op's whole point is that it never names the region's length, so what is
// worth pinning here is exactly that: the same call, the same prefix, three
// different region lengths — many, one, and (the direction the compile-time
// refusal used to stand in for) ZERO — all producing the one layout rule,
// prefix at the mark with the run above it and both orders preserved.
//
// lang/go/region_prefix_test.go is the whole-program half (the parity rows
// and the emitted stream); this is the seam half, where the count is dialled
// directly instead of being produced by a loop.

// sbmStack builds an operand stack of Integers.
func sbmStack(vals ...int64) []core.Value {
	out := make([]core.Value, len(vals))
	for i, v := range vals {
		out[i] = core.NewInteger(v)
	}
	return out
}

// sbmShow renders a stack as "1,2,3" so a layout mismatch reads directly.
func sbmShow(t *testing.T, stack []core.Value) string {
	t.Helper()
	s := ""
	for i, v := range stack {
		n, err := core.AsInteger(v)
		if err != nil {
			t.Fatalf("stack[%d] = %v, want an Integer", i, v)
		}
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf("%d", n)
	}
	return s
}

func TestVmSeatBelowMarkLiftsARegionOfAnyLength(t *testing.T) {
	// Each case is one mark depth, one stack ALREADY holding
	// [below… | region… , prefix…], and the layout the op must produce.
	// `n` is the prefix length; everything between the mark and the prefix
	// is the region, whatever its length.
	for _, c := range []struct {
		name  string
		mark  int
		stack []core.Value
		n     int
		want  string
	}{
		// The frontier row's shape: `99 for 3 [i]` — a three-value run under
		// a one-value prefix, with nothing beneath the mark.
		{"many", 0, sbmStack(0, 1, 2, 99), 1, "99,0,1,2"},
		// One value: the run and the prefix are the same size, the case a
		// fixed-count lowering would also have got right.
		{"one", 0, sbmStack(7, 99), 1, "99,7"},
		// ZERO — `99 await {mode:'first'} [[]]`. The prefix is the whole
		// region-side stack and the op is a no-op on the values, which is
		// exactly why it must not be written as "move the top n past k".
		{"zero", 0, sbmStack(99), 1, "99"},
		// A multi-value prefix keeps ITS order too: `1 2 for 3 [i]`.
		{"two-deep prefix", 0, sbmStack(0, 1, 2, 1, 2), 2, "1,2,0,1,2"},
		// Values BELOW the mark are untouched — the mark is not the stack
		// bottom in a program with anything already seated under it.
		{"below the mark", 2, sbmStack(8, 9, 0, 1, 99), 1, "8,9,99,0,1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			marks, stack, err := vmSeatBelowMark(c.n, []int{c.mark}, c.stack, seam7Dbg, 0)
			if err != nil {
				t.Fatalf("SEAT_BELOW_MARK: %v", err)
			}
			if len(marks) != 0 {
				t.Errorf("marks = %v, want the mark consumed", marks)
			}
			if got := sbmShow(t, stack); got != c.want {
				t.Errorf("layout = %s, want %s", got, c.want)
			}
		})
	}
}

func TestVmSeatBelowMarkErrors(t *testing.T) {
	if _, _, err := vmSeatBelowMark(1, nil, sbmStack(99), seam7Dbg, 0); err == nil {
		t.Error("SEAT_BELOW_MARK with no mark did not error")
	} else {
		wantInternal(t, err, "SEAT_BELOW_MARK with no open mark")
	}
	// The prefix reaches BENEATH the mark: the lowering would have to have
	// pushed fewer values than it claims, so the region boundary is not
	// recoverable and the op refuses rather than shuffling live values.
	if _, _, err := vmSeatBelowMark(3, []int{2}, sbmStack(8, 9, 99), seam7Dbg, 0); err == nil {
		t.Error("SEAT_BELOW_MARK past the mark did not error")
	} else {
		wantInternal(t, err, "SEAT_BELOW_MARK prefix reaches past the mark")
	}
	// A negative prefix length is the same refusal from the other side (a
	// malformed Arg), and it must not slice with a negative index.
	if _, _, err := vmSeatBelowMark(-1, []int{0}, sbmStack(99), seam7Dbg, 0); err == nil {
		t.Error("SEAT_BELOW_MARK with a negative prefix did not error")
	} else {
		wantInternal(t, err, "SEAT_BELOW_MARK prefix reaches past the mark")
	}
}
