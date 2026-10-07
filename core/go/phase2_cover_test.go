package core

import (
	"strings"
	"testing"
)

// TestAssignVarUnitGateDeclinesAnEnclosingFramesVar pins the closure-unit
// gate (assignVarUnitGate): a var of an ENCLOSING frame assigned from a unit
// inside it declines the compiled lane; a module var, a var of the unit's
// own frame, a var in a block at module level, and a `do` body's transparent
// baseline (which compiles into the frame around it) do not.
func TestAssignVarUnitGateDeclinesAnEnclosingFramesVar(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	es := newS5BEmit()
	prev := r.Check.Emit
	r.Check.Emit = es
	defer func() { r.Check.Emit = prev }()

	InstallVar(r, "m", NewInteger(0), nil) // a module var assigns from any unit
	AssignVar(r, "m", NewInteger(1), SrcPos{})
	r.PushFnBaseline(nil) // the frame that declares t
	InstallVar(r, "t", NewInteger(0), nil)
	AssignVar(r, "t", NewInteger(1), SrcPos{}) // the unit's own var
	if len(es.uncompilable) != 0 {
		t.Fatalf("a module var and a frame's own var assign: %v", es.uncompilable)
	}
	tScope, _ := r.Defs.TopEntry("t")
	r.PushFnBaseline(nil) // a closure unit inside the frame
	AssignVar(r, "t", NewInteger(2), SrcPos{})
	if len(es.uncompilable) != 1 || !strings.Contains(es.uncompilable[0], "assigns the enclosing frame's var `t`") {
		t.Fatalf("a unit assigning an enclosing frame's var declines: %v", es.uncompilable)
	}
	r.PopFnBaseline()
	// A `do` body's baseline is transparent: the unit is still the frame.
	r.Check.NextBaselineTransparent = true
	r.PushFnBaseline(nil)
	if base, ok := r.UnitBaselineScope(); !ok || base != tScope.Scope {
		t.Errorf("the transparent baseline is skipped: %d %v, want %d", base, ok, tScope.Scope)
	}
	AssignVar(r, "t", NewInteger(3), SrcPos{})
	if len(es.uncompilable) != 1 {
		t.Errorf("a do body assigns the frame's var: %v", es.uncompilable)
	}
	r.PopFnBaseline()
	r.PopFnBaseline()
	// At module level only a transparent baseline: no unit.
	r.Check.NextBaselineTransparent = true
	r.PushFnBaseline(nil)
	if _, ok := r.UnitBaselineScope(); ok {
		t.Error("a transparent baseline alone is no unit")
	}
	r.PopFnBaseline()
	// A var in a block at module level: no unit baseline, nothing to decline.
	blk := EnterBlock(r)
	InstallVar(r, "b", NewInteger(0), nil)
	AssignVar(r, "b", NewInteger(1), SrcPos{})
	LeaveBlock(r, blk)
	if len(es.uncompilable) != 1 {
		t.Errorf("a block's var at module level assigns: %v", es.uncompilable)
	}
	// The helpers' edges.
	if _, ok := (*Registry)(nil).UnitBaselineScope(); ok {
		t.Error("a nil registry has no unit baseline")
	}
	if _, ok := r.UnitBaselineScope(); ok {
		t.Error("the module level has no unit baseline")
	}
	if _, ok := (*DefTable)(nil).ScopeIndex(1); ok {
		t.Error("a nil table indexes nothing")
	}
	if _, ok := r.Defs.ScopeIndex(1 << 20); ok {
		t.Error("a closed or unknown scope has no index")
	}
	if (*DefTable)(nil).ScopeBoundNames(1) != nil {
		t.Error("a nil table binds nothing")
	}
}

// TestDefCensusDiscardName pins the `_` exemption (design/IMMUTABLE-DEF.1.md
// #15): the discard is bound again freely in one scope or a block of it,
// and neither the census nor the block gate notes it.
func TestDefCensusDiscardName(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	es := newS5BEmit()
	prev := r.Check.Emit
	r.Check.Emit = es
	defer func() { r.Check.Emit = prev }()
	siteAt(r, 1, 1)
	InstallDef(r, "_", NewInteger(1))
	siteAt(r, 1, 9)
	InstallDef(r, "_", NewInteger(2)) // a rebind for any other name
	blk := EnterBlock(r)
	siteAt(r, 1, 20)
	InstallDef(r, "_", NewInteger(3)) // a block shadow for any other name
	LeaveBlock(r, blk)
	if got := classesOf(r); len(got) != 0 {
		t.Errorf("the discard is no finding: %v", got)
	}
	if len(es.uncompilable) != 0 {
		t.Errorf("the discard's block shadow compiles: %v", es.uncompilable)
	}
}

// TestBlockTypeGateDeclinesATypeBoundInABlock pins blockTypeGate: a type
// bound inside a block under an active check pass declines the compiled
// lane; one bound at module level does not.
func TestBlockTypeGateDeclinesATypeBoundInABlock(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	es := newS5BEmit()
	prev := r.Check.Emit
	r.Check.Emit = es
	defer func() { r.Check.Emit = prev }()
	if err := InstallType(r, "Zbtm", NewTypeLiteral(TInteger)); err != nil {
		t.Fatal(err)
	}
	if len(es.uncompilable) != 0 {
		t.Fatalf("a module-level type compiles: %v", es.uncompilable)
	}
	blk := EnterBlock(r)
	if err := InstallType(r, "Zbtb", NewTypeLiteral(TInteger)); err != nil {
		t.Fatal(err)
	}
	LeaveBlock(r, blk)
	if len(es.uncompilable) != 1 || !strings.Contains(es.uncompilable[0], "block-local type def `Zbtb`") {
		t.Errorf("a type bound in a block declines: %v", es.uncompilable)
	}
	if r.Defs.Has("Zbtb") {
		t.Error("the block's type ends with the block")
	}
}

// TestBlockEndResidualEdges pins the block-end residual scan's edges: an
// arm that binds nothing leaves its containers for the run's end; a paren
// group collected as an argument steps the arm's block end inside the group
// (evalParenGroupAt); and a break escaping a residual literal the block
// evaluates is the enclosing loop's (NUR358), the block closed by the
// loop's resolution.
func TestBlockEndResidualEdges(t *testing.T) {
	r, _ := blockReg(t)
	// Binds nothing: the list keeps its timing and its value.
	toks := ifTokens(NextMarkID(), NewBoolean(true), &IfCont{Then: blockBody(NewEvalList([]Value{NewInteger(1), NewInteger(2)})), ThenBlock: true})
	res, err := NewTop(r).Run(toks)
	if err != nil || Canon(res) != "[1 2]" {
		t.Fatalf("unbound arm residual = %s, %v, want [1 2]", Canon(res), err)
	}
	assertClosed(t, r, "unbound arm")
	// A paren group as var's argument: `var (if true [var 5 blk] …) blk`.
	id := NextMarkID()
	toks = blockBody(NewWord("var"), NewOpenParen(), NewMark(id), NewBoolean(true), NewMoveIf(id, "if", &IfCont{Then: bind5(), ThenBlock: true}), NewCloseParen(), NewWord("blk"))
	res, err = NewTop(r).Run(toks)
	if err != nil || Canon(res) != "5" {
		t.Fatalf("paren-collected arm = %s, %v, want 5", Canon(res), err)
	}
	if d := r.Defs.ScopeDepth(); d != 0 {
		t.Errorf("every scope closed after the paren group, depth %d", d)
	}
	r.Defs.Pop("blk")
	// A break inside the arm's residual literal, inside a loop: the loop
	// takes it, the iteration's values are dropped, the block is closed.
	// The loop's body binds (the driver's BodyBindsLocals sees the arm's def
	// through the nested list), so each iteration is a block of its own and
	// the arm's block nests inside it: the break leaves the iteration block,
	// and every scope opened inside it with it.
	id = NextMarkID()
	arm := ifTokens(id, NewBoolean(true), &IfCont{Then: blockBody(NewWord("var"), NewInteger(5), NewEvalList([]Value{NewWord("blk"), NewWord("bbrk")})), ThenBlock: true})
	InstallDef(r, "bi", NewInteger(0))
	cont := &ForCont{Registry: r, IterName: "bi", Current: 0, End: 3, Step: 1, Results: []Value{NewInteger(42)}, Body: arm, IterDepth: r.Defs.Depth("bi"), Block: true}
	cont.BlockID = EnterBlock(r)
	prog := append([]Value{NewMark("bL", arm...)}, arm...)
	prog = append(prog, NewMoveCont("bL", "for loop", cont))
	res, err = NewTop(r).Run(prog)
	if err != nil || Canon(res) != "42" {
		t.Fatalf("escaped residual = %s, %v, want 42", Canon(res), err)
	}
	if r.Defs.Has("bi") {
		t.Error("the index ends with the loop")
	}
	assertClosed(t, r, "escaped residual")
}
