package eng

import (
	"strings"
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

	if got, ok := substToken(toks, nil, 1, v); !ok || len(got) != 3 || &got[0] != &toks[0] {
		t.Error("an empty path is the tokens themselves")
	}
	got, ok := substToken(toks, []int{1}, 1, v)
	if !ok || got[1].String() != "7" || !core.IsParenExpr(toks[1]) {
		t.Errorf("a top-level paren replaced in a copy: %v %v", got, ok)
	}
	got, ok = substToken(toks, []int{2, 1}, 1, v)
	l, _ := core.AsList(got[2])
	orig, _ := core.AsList(toks[2])
	if !ok || !got[2].Eval || l.Get(1).String() != "7" || !core.IsParenExpr(orig.Get(1)) {
		t.Errorf("a paren inside a list literal, the literal copied with its flags: %v %v", got, ok)
	}
	nested := []core.Value{core.NewParenExpr([]core.Value{core.NewInteger(1), core.NewParenExpr(nil)})}
	got, ok = substToken(nested, []int{0, 1}, 1, v)
	inner, _ := core.AsParenExpr(got[0])
	if !ok || len(inner) != 2 || inner[1].String() != "7" {
		t.Errorf("a paren inside a paren: %v %v", got, ok)
	}
	// A run of two — a do and its body list — is one token now.
	got, ok = substToken(toks, []int{1}, 2, v)
	if !ok || len(got) != 2 || got[0].String() != "5" || got[1].String() != "7" || len(toks) != 3 {
		t.Errorf("a run of two replaced by one token in a copy: %v %v", got, ok)
	}
	got, ok = substToken(toks, []int{2, 0}, 2, v)
	l, _ = core.AsList(got[2])
	if !ok || l.Len() != 1 || l.Get(0).String() != "7" {
		t.Errorf("a run of two inside a list literal: %v %v", got, ok)
	}
	for _, c := range []struct {
		name string
		path []int
		span int
	}{
		{"an index past the tokens", []int{3}, 1},
		{"a negative index", []int{-1}, 1},
		{"a path into a scalar", []int{0, 0}, 1},
		{"a path past a nested token's end", []int{2, 5}, 1},
		{"a run past the tokens", []int{2}, 2},
		{"an empty run", []int{1}, 0},
	} {
		if _, ok := substToken(toks, c.path, c.span, v); ok {
			t.Errorf("%s: substToken reports the bad path", c.name)
		}
	}

	is := &compiler.StmtIsland{Island: []core.Value{core.NewInteger(9), core.NewParenExpr([]core.Value{core.NewWord("zz-no-such-word")})}, RetPC: 3, Root: true,
		Substs: []compiler.RestartSubst{{Path: []int{1}, Span: 1, Src: compiler.RestartSrc{Kind: compiler.RestartGuard}}}}
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
		return compiler.RestartSubst{Path: []int{i}, Span: 1, Src: compiler.RestartSrc{Kind: kind, Idx: idx}}
	}
	got, err := vc.substIsland(island, []compiler.RestartSubst{sub(0, compiler.RestartGuard, 0), sub(1, compiler.RestartLocal, 0), sub(2, compiler.RestartStack, 0)}, []core.Value{g}, 1, stack, seam7Dbg, 0)
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
		{"an empty run", compiler.RestartSubst{Path: []int{0}, Src: compiler.RestartSrc{Kind: compiler.RestartStack}}},
	} {
		if _, err := vc.substIsland(island, []compiler.RestartSubst{c.sb}, nil, 1, stack, seam7Dbg, 0); err == nil || !core.IsVMDefer(err) {
			t.Errorf("%s: substIsland raises the compiler's fault, got %v", c.name, err)
		}
	}
	// The runs are written last to first: a do's run of two before a paren
	// leaves the paren's path where the plan put it.
	runs := []compiler.RestartSubst{{Path: []int{0}, Span: 2, Src: compiler.RestartSrc{Kind: compiler.RestartStack}}, sub(2, compiler.RestartLocal, 0)}
	got, err = vc.substIsland(island, runs, nil, 1, stack, seam7Dbg, 0)
	if err != nil || len(got) != 2 || got[0].String() != "3" || got[1].String() != "2" {
		t.Errorf("a run of two, then a paren after it: %v %v", got, err)
	}
	// A paren's value that would dispatch as a token is a designed defer —
	// the interpreter parks it — while a do's result, which the interpreter
	// steps in the do's place, is written.
	fn := core.NewFunction(core.FnDefInfo{Name: "g", Signatures: []core.Signature{{Impl: core.Boru(nil)}}})
	fstack := []core.Value{core.NewInteger(0), fn}
	_, err = vc.substIsland(island, []compiler.RestartSubst{sub(0, compiler.RestartStack, 0)}, nil, 1, fstack, seam7Dbg, 0)
	if err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "parks it") {
		t.Errorf("a paren's fn value defers: %v", err)
	}
	got, err = vc.substIsland(island, []compiler.RestartSubst{{Path: []int{0}, Span: 2, Src: compiler.RestartSrc{Kind: compiler.RestartStack}}}, nil, 1, fstack, seam7Dbg, 0)
	if err != nil || len(got) != 2 || !core.FnValueDispatchesAtPointer(got[0]) {
		t.Errorf("a do's fn result is written: %v %v", got, err)
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

// TestLandedPos pins where the landing's walk raises a strand (NUR289's
// caret): the landed value's own token, else where the recording pass saw
// it land (LandingWord.ValPos), else the word after it.
func TestLandedPos(t *testing.T) {
	at := func(col int) core.SrcPos { return core.SrcPos{Row: 1, Col: col} }
	own := core.WithPosAt(core.NewInteger(1), at(3))
	lword := compiler.LandingWord{Pos: at(9), ValPos: at(5)}
	if got := landedPos(own, lword); got != at(3) {
		t.Errorf("a value with a token raises there: %v", got)
	}
	if got := landedPos(core.NewInteger(1), lword); got != at(5) {
		t.Errorf("a value without one raises where it landed: %v", got)
	}
	if got := landedPos(core.NewInteger(1), compiler.LandingWord{Pos: at(9)}); got != at(9) {
		t.Errorf("with neither, at the word: %v", got)
	}
}

// TestFirstIteration pins the loops' first-iteration check a statement island
// takes before it runs (compiler.RestartFirst, NUR296): every enclosing
// loop's index slot holds its start. Past the first iteration — or a slot the
// frame lacks, or holding no integer — the island is a designed defer.
func TestFirstIteration(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: &compiler.Program{}, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	vc.restartLocals = []core.Value{core.NewInteger(0), core.NewString("s")}
	for _, c := range []struct {
		name  string
		first []compiler.RestartFirst
		want  bool
	}{
		{"no loop", nil, true},
		{"the first iteration", []compiler.RestartFirst{{Slot: 0, Val: 0}}, true},
		{"a later iteration", []compiler.RestartFirst{{Slot: 0, Val: 1}}, false},
		{"a slot holding no integer", []compiler.RestartFirst{{Slot: 1, Val: 0}}, false},
		{"a slot past the frame", []compiler.RestartFirst{{Slot: 5, Val: 0}}, false},
		{"a negative slot", []compiler.RestartFirst{{Slot: -1, Val: 0}}, false},
	} {
		if got := vc.firstIteration(c.first); got != c.want {
			t.Errorf("%s: firstIteration = %v, want %v", c.name, got, c.want)
		}
	}
	later := []compiler.RestartFirst{{Slot: 0, Val: 1}}
	lword := compiler.LandingWord{Island: []core.Value{core.NewInteger(9)}, RetPC: 2, Root: true, FirstIter: later}
	if _, _, err := vc.landingRestart(r, lword, 0, nil, seam7Dbg, 0); err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "first iteration") {
		t.Errorf("a landing's island past the first iteration defers: %v", err)
	}
	spec := &compiler.DynMethodSpec{Word: "f", NOut: 1, Restart: true, Root: true, RetPC: 2, Island: lword.Island, FirstIter: later}
	if _, _, err := vc.callDynMethod(r, spec, 0, []core.Value{core.NewInteger(5)}, seam7Dbg, 0); err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "first iteration") {
		t.Errorf("a shaped apply's island past the first iteration defers: %v", err)
	}
}

// TestLandingCollectsVM pins the VM's collecting landing
// (compiler.LandingCollects, NUR298): unguarded, the value is the residual
// arms' as it always was; guarded, the statement island runs the re-step,
// and with no island an argument-taking fn is a designed defer while a
// nullary one passes.
func TestLandingCollectsVM(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	unary := core.NewFunction(core.FnDefInfo{Name: "g", Signatures: []core.Signature{{Args: []*core.Type{core.TInteger}}}})
	nullary := core.NewFunction(core.FnDefInfo{Name: "h", Signatures: []core.Signature{{}}})
	collect, guarded := compiler.LandingCollects, compiler.LandingCollects|compiler.LandingBeneathGuard
	if got, ent, err := vc.reStepLanding(r, collect, 0, []core.Value{unary}, seam7Dbg, 0, compiler.LandingWord{}); err != nil || ent != nil || len(got) != 1 {
		t.Errorf("unguarded: the arms' value: %v %v %v", got, ent, err)
	}
	if _, _, err := vc.reStepLanding(r, guarded, 0, []core.Value{unary}, seam7Dbg, 0, compiler.LandingWord{}); err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "NUR298") {
		t.Errorf("guarded with no island: a designed defer: %v", err)
	}
	if got, _, err := vc.reStepLanding(r, guarded, 0, []core.Value{nullary}, seam7Dbg, 0, compiler.LandingWord{}); err != nil || len(got) != 1 {
		t.Errorf("guarded, a nullary fn passes: %v %v", got, err)
	}
	lword := compiler.LandingWord{Restart: true, Root: true, RetPC: 4, Island: []core.Value{core.NewInteger(9)}}
	if got, ent, err := vc.reStepLanding(r, guarded, 0, []core.Value{unary}, seam7Dbg, 0, lword); err != nil || ent == nil || ent.jumpPC != 4 || len(got) != 1 || got[0].String() != "9" {
		t.Errorf("guarded with an island: the statement runs again: %v %+v %v", got, ent, err)
	}
}

// TestStopRestartResults pins the count island's substitution (NUR222): the
// run a do's call left is written whole in place of the do word and its
// body (RestartResults) — none at all, or several — and the stop's values
// must be there to write.
func TestStopRestartResults(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: &compiler.Program{}, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	island := []core.Value{core.NewInteger(1), core.NewWord("do"), core.NewEvalList(nil), core.NewWord("drop")}
	run := compiler.RestartSubst{Path: []int{1}, Span: 2, Src: compiler.RestartSrc{Kind: compiler.RestartResults}}
	got, err := vc.substIsland(island, []compiler.RestartSubst{run}, []core.Value{}, 0, nil, seam7Dbg, 0)
	if err != nil || len(got) != 2 || got[0].String() != "1" {
		t.Errorf("an empty run removes the do: %v %v", got, err)
	}
	got, err = vc.substIsland(island, []compiler.RestartSubst{run}, []core.Value{core.NewInteger(7), core.NewInteger(8)}, 0, nil, seam7Dbg, 0)
	if err != nil || len(got) != 4 || got[1].String() != "7" || got[2].String() != "8" {
		t.Errorf("a run of two is written whole: %v %v", got, err)
	}
	if _, err := vc.substIsland(island, []compiler.RestartSubst{run}, nil, 0, nil, seam7Dbg, 0); err == nil {
		t.Error("no run to write is the compiler's fault")
	}
	guard := compiler.RestartSubst{Path: []int{1}, Span: 1, Src: compiler.RestartSrc{Kind: compiler.RestartGuard}}
	if _, err := vc.substIsland(island, []compiler.RestartSubst{guard}, []core.Value{core.NewInteger(1), core.NewInteger(2)}, 0, nil, seam7Dbg, 0); err == nil {
		t.Error("a guard's value is one value")
	}
	is := &compiler.StmtIsland{Island: island[:3], RetPC: 5, Root: true, Substs: []compiler.RestartSubst{run}}
	res, ent, err := vc.stopRestart(r, is, []core.Value{core.NewInteger(7)}, 0, nil, seam7Dbg, 0)
	if err != nil || ent == nil || ent.jumpPC != 5 || len(res) != 2 || res[1].String() != "7" {
		t.Errorf("the statement runs again over the run: `1 7`: %v %+v %v", res, ent, err)
	}
}

// TestSubstIslandNone pins the effect run (NUR296): a call run before the
// stop that left nothing is written as no token — the word and its
// arguments gone from the island, so the island never runs it again — and
// a zero-argument one as well; nothing is written, so there is no fn value
// to park.
func TestSubstIslandNone(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: &compiler.Program{}, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	island := []core.Value{core.NewWord("print"), core.NewString("a"), core.NewInteger(5), core.NewWord("nl")}
	none := compiler.RestartSrc{Kind: compiler.RestartNone}
	got, err := vc.substIsland(island, []compiler.RestartSubst{{Path: []int{0}, Span: 2, Src: none}, {Path: []int{3}, Span: 1, Src: none}}, nil, 0, nil, seam7Dbg, 0)
	if err != nil || len(got) != 1 || got[0].String() != "5" || len(island) != 4 {
		t.Errorf("the effects' runs are gone from a copy of the island: %v %v", got, err)
	}
	// A call run's value is placed: a fn value that would dispatch where the
	// island writes it defers, whatever the run's span (NUR297's rule).
	vc.restartLocals = []core.Value{core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{}}})}
	defer func() { vc.restartLocals = nil }()
	placed := compiler.RestartSubst{Path: []int{0}, Span: 2, Src: compiler.RestartSrc{Kind: compiler.RestartLocal, Idx: 0}, Placed: true}
	if _, err := vc.substIsland(island, []compiler.RestartSubst{placed}, nil, 0, nil, seam7Dbg, 0); err == nil || !core.IsVMDefer(err) {
		t.Errorf("a placed fn value defers: %v", err)
	}
	placed.Placed = false
	if _, err := vc.substIsland(island, []compiler.RestartSubst{placed}, nil, 0, nil, seam7Dbg, 0); err != nil {
		t.Errorf("a do span's value is stepped, as the interpreter steps it: %v", err)
	}
}
