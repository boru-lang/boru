package lang

// The const pool keys on the value's TYPE as well as its rendering.
//
// core.CanonValue renders a value through its nearest BASE-type arm
// (core/go/canon.go — the `v.Parent.ConformsTo(TInteger)` case returns the
// digits and nothing else), which is right for canon's own jobs: ordering,
// `deq`, rendering. It is wrong as a const-pool key, because two values whose
// only difference is their nominal type render identically and so collided in
// one Consts slot. Whichever was interned FIRST donated its Parent to the
// other site:
//
//	def Pos (refine Integer)  def x:Pos 42  typeof 42  typeof x
//	  interpreted -> Integer Pos      compiled (before the fix) -> Integer Integer
//
// A silent miscompile on the DEFAULT path, not merely under -force-compile,
// since `-compile` falls back only on a compile failure and this program never
// declined. Found while verifying an unrelated claim about whether a const
// index could serve as a baked-site identity — it cannot, and this is the
// sharper reason why.

import (
	"fmt"
	"strings"
	"testing"
)

// TestConstPoolSeparatesNominalTypes — the miscompile pin, BOTH orderings.
// Order matters because the collision is first-interned-wins: with the refined
// value first the base literal inherited the refinement, and with the literal
// first the refined binding lost its type. One ordering alone would leave half
// the defect unpinned.
func TestConstPoolSeparatesNominalTypes(t *testing.T) {
	cases := []struct{ src, want string }{
		{`def Pos (refine Integer)  def x:Pos 42  typeof 42  typeof x`, "[Integer Pos]"},
		{`def Pos (refine Integer)  def x:Pos 42  typeof x  typeof 42`, "[Pos Integer]"},
		// The same collision one level down: two DISTINCT refinements of one
		// base, which canon renders identically to each other as well as to
		// the base.
		{`def Pos (refine Integer)  def Neg (refine Integer)  def a:Pos 7  def b:Neg 7  typeof a  typeof b`, "[Pos Neg]"},
	}
	for _, c := range cases {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: expected the compiled lane, not a fallback", c.src)
		}
		if errC != nil || errI != nil {
			t.Errorf("%q: unexpected error: compiled=%v interp=%v", c.src, errC, errI)
			continue
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: engine divergence: compiled=%v interp=%v", c.src, gotC, gotI)
		}
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: want %s, got %s", c.src, c.want, fmt.Sprint(gotC))
		}
	}
}

// TestConstPoolStillPoolsWithinAType — the paired negative, and the one that
// makes the fix a SEPARATION rather than a disabling. Keying the pool by type
// would be trivially "correct" if it simply stopped pooling; this asserts that
// repeated literals of the SAME type still share one slot, so a future change
// that keys on something per-site (a position, a counter) fails here rather
// than silently growing every program's const table.
func TestConstPoolStillPoolsWithinAType(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(`42 drop  42 drop  42 drop  42`)
	if cerr != nil || prog == nil {
		t.Fatalf("must compile; reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if !strings.Contains(dis, "consts=1 ") {
		t.Errorf("four occurrences of one Integer literal must share ONE const slot; got:\n%s", dis)
	}
}
