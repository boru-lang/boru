package eng

import (
	"fmt"
	"testing"

	"github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestNUR336SurvivorIsland pins the island a paren apply over a lead that
// went through a landing takes (NUR336's second remainder): the paren is
// written as its survivors — the lead, then the values after it — where the
// interpreter's close lands its rewind on the lead and steps each, the
// island's other substitutions standing, those inside the paren dropped.
func TestNUR336SurvivorIsland(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	guard := compiler.RestartSrc{Kind: compiler.RestartGuard}
	stackSrc := compiler.RestartSrc{Kind: compiler.RestartStack}
	// `(k) (m.f y) 9 (j)`: the first paren written as its value (the
	// frame's entry 0), the lead and y inside the apply's paren, the last
	// paren after it written as the stack's entry too.
	island := []core.Value{
		core.NewParenExpr([]core.Value{core.NewWord("k")}),
		core.NewParenExpr([]core.Value{core.NewWord("m"), core.NewWord("y")}),
		core.NewInteger(9),
		core.NewParenExpr([]core.Value{core.NewWord("j")}),
	}
	spec := &compiler.DynMethodSpec{Island: island, Substs: []compiler.RestartSubst{
		{Path: []int{0}, Span: 1, Src: stackSrc},
		{Path: []int{1, 0}, Span: 1, Src: guard, Placed: true},
		{Path: []int{1, 1}, Span: 1, Src: stackSrc},
		{Path: []int{3}, Span: 1, Src: stackSrc},
	}}
	if got := survivorParen(spec.Substs); fmt.Sprint(got) != "[1]" {
		t.Fatalf("the lead's paren: %v", got)
	}
	lead := n336Fn("f", core.TInteger, core.TInteger)
	stack := []core.Value{core.NewInteger(3)}
	got, err := vc.survivorIsland(spec, []core.Value{lead, core.NewInteger(42)}, 0, stack, seam7Dbg, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != fmt.Sprint([]core.Value{core.NewInteger(3), lead, core.NewInteger(42), core.NewInteger(9), core.NewInteger(3)}) {
		t.Errorf("the survivors in the paren's place: %v", got)
	}
	// A substitution the island cannot read fails as substIsland does, on
	// either side of the paren.
	for _, bad := range [][]compiler.RestartSubst{
		{{Path: []int{0}, Span: 1, Src: compiler.RestartSrc{Kind: compiler.RestartLocal, Idx: 9}}, {Path: []int{1, 0}, Span: 1, Src: guard}},
		{{Path: []int{1, 0}, Span: 1, Src: guard}, {Path: []int{3}, Span: 1, Src: compiler.RestartSrc{Kind: compiler.RestartLocal, Idx: 9}}},
	} {
		if _, err := vc.survivorIsland(&compiler.DynMethodSpec{Island: island, Substs: bad}, []core.Value{lead}, 0, stack, seam7Dbg, 0); err == nil {
			t.Errorf("%v: want the bad substitution", bad)
		}
	}
	// A paren path the island does not hold.
	if _, err := vc.survivorIsland(&compiler.DynMethodSpec{Island: island[:1], Substs: []compiler.RestartSubst{{Path: []int{5, 0}, Span: 1, Src: guard}}}, []core.Value{lead}, 0, stack, seam7Dbg, 0); err == nil {
		t.Error("want the bad substitution for a paren the island lacks")
	}
	// No lead substitution, or one not first in its paren, names no paren.
	for _, substs := range [][]compiler.RestartSubst{
		nil,
		{{Path: []int{1, 1}, Span: 1, Src: guard}},
		{{Path: []int{1}, Span: 1, Src: guard}},
		{{Path: []int{1, 0}, Span: 1, Src: stackSrc}},
	} {
		if got := survivorParen(substs); got != nil {
			t.Errorf("%v: named %v", substs, got)
		}
	}
	if !pathHasPrefix([]int{1, 0}, []int{1}) || pathHasPrefix([]int{2}, []int{1}) || pathHasPrefix(nil, []int{1}) {
		t.Error("pathHasPrefix")
	}
	if !pathBefore([]int{0, 3}, []int{1}) || pathBefore([]int{2}, []int{1, 0}) || pathBefore([]int{1}, []int{1, 0}) {
		t.Error("pathBefore")
	}
}
