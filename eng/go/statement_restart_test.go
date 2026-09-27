package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestStatementRestart pins the statement island (NUR242, NUR219 —
// compiler's landing_restart.go): the prefix is the frame region beneath the
// statement, or at the root each earlier residual entry where the root keeps
// it — a constant, a slot, a stack entry — and the island's residual
// replaces the frame region, the run continuing at RetPC. A malformed entry
// is the compiler's own fault, raised as such.
func TestStatementRestart(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: &compiler.Program{}, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	island := []core.Value{core.NewInteger(9)}
	x, v := core.NewString("x"), core.NewInteger(0)
	vc.restartLocals = []core.Value{core.NewInteger(7)}
	srcs := []compiler.RestartSrc{
		{Kind: compiler.RestartConst, Val: core.NewInteger(1)},
		{Kind: compiler.RestartLocal, Idx: 0},
		{Kind: compiler.RestartStack, Idx: 0},
	}
	got, ent, err := vc.statementRestart(r, srcs, island, 1, 5, true, 0, []core.Value{x, v}, seam7Dbg, 0)
	if err != nil || ent == nil || !ent.jump || ent.jumpPC != 5 || len(got) != 4 || got[1].String() != "7" || got[2].String() != x.String() || got[3].String() != "9" {
		t.Fatalf("the prefix from its sources, then the island: got %v %+v %v", got, ent, err)
	}
	// A unit's island: the frame region beneath the statement is the prefix.
	got, _, err = vc.landingRestart(r, compiler.LandingWord{Island: island, Depth: 1, RetPC: 2}, 0, []core.Value{x, v}, seam7Dbg, 0)
	if err != nil || len(got) != 2 || got[0].String() != x.String() {
		t.Fatalf("a unit's prefix is its frame region: got %v %v", got, err)
	}
	for _, c := range []struct {
		name   string
		island []core.Value
		depth  int
		retPC  int
		srcs   []compiler.RestartSrc
	}{
		{"no tokens", nil, 0, 1, nil},
		{"no return pc", island, 0, -1, nil},
		{"a region deeper than the frame", island, 3, 1, nil},
		{"a slot the frame lacks", island, 0, 1, []compiler.RestartSrc{{Kind: compiler.RestartLocal, Idx: 4}}},
		{"a stack entry past the region", island, 0, 1, []compiler.RestartSrc{{Kind: compiler.RestartStack, Idx: 0}}},
	} {
		if _, _, err := vc.statementRestart(r, c.srcs, c.island, c.depth, c.retPC, false, 0, []core.Value{x}, seam7Dbg, 0); err == nil {
			t.Errorf("%s: a malformed island entry raises", c.name)
		}
	}
	// The island's own error is the program's.
	if _, _, err := vc.statementRestart(r, nil, []core.Value{core.NewWord("zz-no-such-word")}, 0, 1, true, 0, nil, seam7Dbg, 0); err == nil {
		t.Error("an island that raises raises")
	}
}
