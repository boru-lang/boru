package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestGuardRestart pins a branch guard's statement island (SigRef.Restart,
// NUR292): the guarded value is written in place of the paren that computed
// it — at the top level or down a path through parens and list literals,
// each level a fresh copy — and the island runs as a landing's does. A path
// that leaves the tokens is the compiler's own fault, raised as such.
func TestGuardRestart(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: &compiler.Program{}, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	v := core.NewInteger(7)
	paren := core.NewParenExpr([]core.Value{core.NewWord("mk")})
	lit := core.NewEvalList([]core.Value{core.NewInteger(1), core.NewParenExpr([]core.Value{core.NewWord("mk")})})
	toks := []core.Value{core.NewInteger(5), paren, lit}

	if got, ok := substToken(toks, nil, v); !ok || len(got) != 3 || &got[0] != &toks[0] {
		t.Error("an empty path is the tokens themselves")
	}
	got, ok := substToken(toks, []int{1}, v)
	if !ok || got[1].String() != "7" || !core.IsParenExpr(toks[1]) {
		t.Errorf("a top-level paren replaced in a copy: %v %v", got, ok)
	}
	got, ok = substToken(toks, []int{2, 1}, v)
	l, _ := core.AsList(got[2])
	orig, _ := core.AsList(toks[2])
	if !ok || !got[2].Eval || l.Get(1).String() != "7" || !core.IsParenExpr(orig.Get(1)) {
		t.Errorf("a paren inside a list literal, the literal copied with its flags: %v %v", got, ok)
	}
	nested := []core.Value{core.NewParenExpr([]core.Value{core.NewInteger(1), core.NewParenExpr(nil)})}
	got, ok = substToken(nested, []int{0, 1}, v)
	inner, _ := core.AsParenExpr(got[0])
	if !ok || len(inner) != 2 || inner[1].String() != "7" {
		t.Errorf("a paren inside a paren: %v %v", got, ok)
	}
	for _, c := range []struct {
		name string
		path []int
	}{
		{"an index past the tokens", []int{3}},
		{"a negative index", []int{-1}},
		{"a path into a scalar", []int{0, 0}},
		{"a path past a nested token's end", []int{2, 5}},
	} {
		if _, ok := substToken(toks, c.path, v); ok {
			t.Errorf("%s: substToken reports the bad path", c.name)
		}
	}

	is := &compiler.StmtIsland{Island: []core.Value{core.NewInteger(9), core.NewParenExpr([]core.Value{core.NewWord("zz-no-such-word")})}, RetPC: 3, Root: true,
		Substs: []compiler.RestartSubst{{Path: []int{1}, Src: compiler.RestartSrc{Kind: compiler.RestartGuard}}}}
	res, ent, err := vc.guardRestart(r, is, v, 0, nil, seam7Dbg, 0)
	if err != nil || ent == nil || ent.jumpPC != 3 || len(res) != 2 || res[1].String() != "7" {
		t.Fatalf("the island runs with the value in the paren's place: %v %+v %v", res, ent, err)
	}
	is.Substs[0].Path = []int{5}
	if _, _, err := vc.guardRestart(r, is, v, 0, nil, seam7Dbg, 0); err == nil || !core.IsVMDefer(err) {
		t.Errorf("a substitution past the island is the compiler's fault: %v", err)
	}
}

// TestSubstIsland pins where a statement island reads each substituted
// paren's value (compiler.RestartSubst): the guarded value, a slot, a
// frame-region entry; a source the stop does not hold raises as the
// compiler's own fault, and a landing's or shaped apply's island writes its
// values too.
func TestSubstIsland(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: &compiler.Program{}, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	vc.restartLocals = []core.Value{core.NewInteger(2)}
	g := core.NewInteger(1)
	island := []core.Value{core.NewParenExpr(nil), core.NewParenExpr(nil), core.NewParenExpr(nil)}
	stack := []core.Value{core.NewInteger(0), core.NewInteger(3)}
	sub := func(i int, kind compiler.RestartSrcKind, idx int) compiler.RestartSubst {
		return compiler.RestartSubst{Path: []int{i}, Src: compiler.RestartSrc{Kind: kind, Idx: idx}}
	}
	got, err := vc.substIsland(island, []compiler.RestartSubst{sub(0, compiler.RestartGuard, 0), sub(1, compiler.RestartLocal, 0), sub(2, compiler.RestartStack, 0)}, &g, 1, stack, seam7Dbg, 0)
	if err != nil || got[0].String() != "1" || got[1].String() != "2" || got[2].String() != "3" || !core.IsParenExpr(island[0]) {
		t.Fatalf("each value from its source, the program's tokens untouched: %v %v", got, err)
	}
	for _, c := range []struct {
		name string
		sb   compiler.RestartSubst
	}{
		{"a guard's value with no guard", sub(0, compiler.RestartGuard, 0)},
		{"a slot the frame lacks", sub(0, compiler.RestartLocal, 4)},
		{"a negative slot", sub(0, compiler.RestartLocal, -1)},
		{"an entry past the stack", sub(0, compiler.RestartStack, 1)},
		{"a constant", sub(0, compiler.RestartConst, 0)},
		{"a path past the island", sub(7, compiler.RestartStack, 0)},
	} {
		if _, err := vc.substIsland(island, []compiler.RestartSubst{c.sb}, nil, 1, stack, seam7Dbg, 0); err == nil || !core.IsVMDefer(err) {
			t.Errorf("%s: substIsland raises the compiler's fault, got %v", c.name, err)
		}
	}
	// A landing's island and a shaped apply's write their substitutions.
	lword := compiler.LandingWord{Island: []core.Value{core.NewParenExpr([]core.Value{core.NewWord("zz-no-such-word")})}, RetPC: 4, Root: true,
		Substs: []compiler.RestartSubst{sub(0, compiler.RestartStack, 1)}}
	res, _, err := vc.landingRestart(r, lword, 0, stack, seam7Dbg, 0)
	if err != nil || len(res) != 1 || res[0].String() != "3" {
		t.Errorf("a landing's island writes the value it holds: %v %v", res, err)
	}
	lword.Substs[0].Src.Idx = 9
	if _, _, err := vc.landingRestart(r, lword, 0, stack, seam7Dbg, 0); err == nil {
		t.Error("a landing's bad substitution raises")
	}
	// A shaped apply whose method is data at run time restarts its statement
	// with its substitutions written.
	spec := &compiler.DynMethodSpec{Word: "f", NOut: 1, Restart: true, Root: true, RetPC: 4, Island: lword.Island,
		Substs: []compiler.RestartSubst{sub(0, compiler.RestartStack, 0)}}
	res, _, err = vc.callDynMethod(r, spec, 0, []core.Value{core.NewInteger(5)}, seam7Dbg, 0)
	if err != nil || len(res) != 1 || res[0].String() != "5" {
		t.Errorf("a shaped apply's island writes the value it holds: %v %v", res, err)
	}
	spec.Substs[0].Src.Idx = 9
	if _, _, err := vc.callDynMethod(r, spec, 0, []core.Value{core.NewInteger(5)}, seam7Dbg, 0); err == nil {
		t.Error("a shaped apply's bad substitution raises")
	}
}
