package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// vm_make_list_to_mark_test.go pins OpMakeListToMark — the op that collects a
// runtime-variadic REGION into one List (NUR067's consuming half).
//
// It is OpMakeList with the element count taken from the mark instead of from
// Arg, so what is worth pinning is the counts OpMakeList could not express:
// a run of three, a run of one, and an EMPTY run — which must build the empty
// List rather than underflow or collect what lies beneath the mark.
//
// lang/go/region_collect_test.go is the whole-program half.

func TestVmMakeListToMarkCollectsARunOfAnyLength(t *testing.T) {
	for _, c := range []struct {
		name  string
		mark  int
		stack []core.Value
		want  string
	}{
		// `[(for 3 [i])]` — the run is the whole stack above the mark.
		{"many", 0, sbmStack(0, 1, 2), "[0 1 2]"},
		{"one", 0, sbmStack(7), "[7]"},
		// ZERO — `[(for 0 [i])]`. The empty List, not an underflow.
		{"zero", 0, nil, "[]"},
		// Values BELOW the mark stay on the stack, beneath the new List.
		{"below the mark", 2, sbmStack(8, 9, 0, 1), "8 9 [0 1]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			marks, stack, err := vmMakeListToMark([]int{c.mark}, c.stack, seam7Dbg, 0)
			if err != nil {
				t.Fatalf("MAKE_LIST_TO_MARK: %v", err)
			}
			if len(marks) != 0 {
				t.Errorf("marks = %v, want the mark consumed", marks)
			}
			// Canon renders the whole stack, so a collected List (`[0 1 2]`)
			// reads differently from three loose values (`0 1 2`) — the very
			// distinction this op exists to make.
			if got := core.Canon(stack); got != c.want {
				t.Errorf("stack = %s, want %s", got, c.want)
			}
		})
	}
}

func TestVmMakeListToMarkErrors(t *testing.T) {
	if _, _, err := vmMakeListToMark(nil, sbmStack(1), seam7Dbg, 0); err == nil {
		t.Error("MAKE_LIST_TO_MARK with no mark did not error")
	} else {
		wantInternal(t, err, "MAKE_LIST_TO_MARK with no open mark")
	}
	if _, _, err := vmMakeListToMark([]int{5}, sbmStack(1), seam7Dbg, 0); err == nil {
		t.Error("MAKE_LIST_TO_MARK above depth did not error")
	} else {
		wantInternal(t, err, "MAKE_LIST_TO_MARK above current depth")
	}
}
