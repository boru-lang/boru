package compiler

import (
	"slices"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur336_leftover_test.go pins the helpers of NUR336's and NUR347's last
// remainders: the def leftovers a unit's statement island seats
// (defLeftovers, resultCount, defLeftoverSrcs), the told stack of a paren
// the engine expanded before stepping it (toldAt), the `/v` read positions a
// native call's fn arguments carry (noteFnArgPos, SigRef.FnArgPos) and the
// unexpanded tokens a no-match report names (unexpandedToken). The lang
// suite drives them end to end (TestNUR336ComputedDefOpenerParen,
// TestNUR347ValReadCallbackAnchor, TestNUR329NoMatchNamesTheUnexpandedReach).

func TestNUR336DefLeftovers(t *testing.T) {
	name := func(n string, col int) core.Value {
		w := core.NewWord(n)
		w.SetPos(core.SrcPos{Row: 1, Col: col})
		return w
	}
	// def k (3 dup)  def j 4  def q (two)  — then the statement's stop.
	body := []core.Value{
		core.NewWord("def"), name("k", 5), core.NewParenExpr(nil),
		core.NewWord("def"), name("j", 20), core.NewInteger(4),
		core.NewWord("def"), name("q", 30), core.NewParenExpr(nil),
	}
	dup := EmitEvent{kind: evCall, seq: 1, call: emitCall{word: "dup", nout: 2}}
	two := EmitEvent{kind: evCallUser, seq: 3, uc: emitUserCall{nout: 3}}
	tree := map[int]treeEvent{
		1: {ev: &dup},
		2: {ev: &EmitEvent{kind: evDynBind, seq: 2, dyn: &emitDynBind{name: "k", srcSeq: 1, pos: core.SrcPos{Row: 1, Col: 5}}}},
		3: {ev: &two},
		4: {ev: &EmitEvent{kind: evDynBind, seq: 4, dyn: &emitDynBind{name: "j", srcSeq: -1, pos: core.SrcPos{Row: 1, Col: 20}}}},
		5: {ev: &EmitEvent{kind: evDynBind, seq: 5, dyn: &emitDynBind{name: "q", srcSeq: 3, pos: core.SrcPos{Row: 1, Col: 30}}}},
	}
	left, bound := defLeftovers(tree, body, 0, 9)
	if !slices.Equal(left, []producer{{seq: 1, idx: 1}, {seq: 3, idx: 1}, {seq: 3, idx: 2}}) {
		t.Errorf("leftovers: %v", left)
	}
	if !slices.Equal(bound, []producer{{seq: 1}, {seq: 3}}) {
		t.Errorf("bound: %v", bound)
	}
	// A def the tree does not hold names nothing.
	if left, bound := defLeftovers(map[int]treeEvent{}, body, 0, 3); left != nil || bound != nil {
		t.Errorf("an unknown def: %v %v", left, bound)
	}
	if resultCount(nil) != 1 || resultCount(&EmitEvent{kind: evBranch}) != 1 || resultCount(&dup) != 2 || resultCount(&two) != 3 {
		t.Error("resultCount")
	}
	if !outsProducedBefore([]EmitOperand{{kind: opEvent, idx: 1, resIdx: 0}}, 2, left) || outsProducedBefore([]EmitOperand{{kind: opEvent, idx: 1, resIdx: 1}}, 2, left) {
		t.Error("outsProducedBefore passes a seated leftover and no other result")
	}
}

func TestNUR336DefLeftoverSrcs(t *testing.T) {
	lw := &lowerer{promoted: map[int]int{7: 4}}
	for _, c := range []struct {
		what        string
		vm          []vmSlot
		r           landingRestart
		frame, left []RestartSrc
		ok          bool
	}{
		{"promoted leftovers over an empty frame", nil,
			landingRestart{leftovers: []producer{{seq: 7, idx: 1}}, defBound: []producer{{seq: 7}}},
			nil, []RestartSrc{{Kind: RestartLocal, Idx: 5}}, true},
		{"the bound value left on the frame is the def's", []vmSlot{{seq: 7}, {seq: 7, idx: 1}},
			landingRestart{depth: 2, leftovers: []producer{{seq: 7, idx: 1}}, defBound: []producer{{seq: 7}}},
			[]RestartSrc{{Kind: RestartStack, Idx: 1}}, nil, true},
		{"a promoted leftover above a frame value", []vmSlot{nonEventSlot},
			landingRestart{depth: 1, leftovers: []producer{{seq: 7, idx: 1}}},
			nil, nil, false},
		{"a leftover held nowhere", nil,
			landingRestart{leftovers: []producer{{seq: 8, idx: 1}}},
			nil, nil, false},
	} {
		lw.vm = c.vm
		frame, left, ok := lw.defLeftoverSrcs(&c.r)
		if ok != c.ok || !slices.Equal(frame, c.frame) || !slices.Equal(left, c.left) {
			t.Errorf("%s: %v %v %v", c.what, frame, left, ok)
		}
	}
	// noteRestartDepth seats them: the frame's params, then the frame, then
	// the leftovers; an unseatable leftover marks the island so.
	lw = &lowerer{promoted: map[int]int{7: 4}, unnamedParams: []int{0}}
	r := &landingRestart{depth: -1, start: core.SrcPos{Row: 1, Col: 1}, leftovers: []producer{{seq: 7, idx: 1}}}
	lw.noteRestartDepth(r, core.SrcPos{Row: 1, Col: 2})
	if !slices.Equal(r.srcs, []RestartSrc{{Kind: RestartLocal, Idx: 0}, {Kind: RestartLocal, Idx: 5}}) || r.unseatable {
		t.Errorf("seated: %v %v", r.srcs, r.unseatable)
	}
	lw = &lowerer{vm: []vmSlot{nonEventSlot}}
	r = &landingRestart{depth: -1, start: core.SrcPos{Row: 1, Col: 1}, leftovers: []producer{{seq: 7, idx: 1}}}
	lw.noteRestartDepth(r, core.SrcPos{Row: 1, Col: 2})
	if !r.unseatable {
		t.Error("a leftover held nowhere leaves the island unseatable")
	}
}

func TestNUR336ToldAt(t *testing.T) {
	inner := core.NewWord("m")
	inner.SetPos(core.SrcPos{Row: 1, Col: 65})
	paren := core.NewParenExpr([]core.Value{inner})
	paren.SetPos(core.SrcPos{Row: 1, Col: 64})
	bare := core.NewParenExpr([]core.Value{core.NewWord("m")})
	empty := core.NewParenExpr(nil)
	es := &EmitState{rootBody: []core.Value{restartTok(1, 1), paren, bare, empty}}
	es.rootStmtStacks = map[core.SrcPos][]core.Value{
		{Row: 1, Col: 1}:  {core.NewInteger(1)},
		{Row: 1, Col: 65}: {core.NewInteger(3)},
	}
	if s, ok := es.toldAt(0); !ok || len(s) != 1 {
		t.Error("a token's own note")
	}
	if s, ok := es.toldAt(1); !ok || len(s) != 1 || s[0].String() != "3" {
		t.Errorf("an expanded paren's note, by its first inner token: %v %v", s, ok)
	}
	if _, ok := es.toldAt(2); ok {
		t.Error("a paren whose first token has no position")
	}
	if _, ok := es.toldAt(3); ok {
		t.Error("an empty paren")
	}
	es.rootBody = append(es.rootBody, restartTok(2, 1))
	if _, ok := es.toldAt(4); ok {
		t.Error("an untold token")
	}
	if got := es.toldAfter(0, core.SrcPos{Row: 1, Col: 65}); got != 1 {
		t.Errorf("toldAfter reaches the expanded paren: %d", got)
	}
}

func TestNUR347NoteFnArgPos(t *testing.T) {
	es := NewEmitState()
	fn := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "h"}, ID: "fnid"}
	fn.SetPos(core.SrcPos{Row: 1, Col: 101})
	unread := fn
	unread.ID = "other"
	noID := fn
	noID.ID = ""
	noPos := fn
	noPos.SetPos(core.SrcPos{})
	es.valReadNoted = map[string]bool{"fnid": true, "n": true}
	n := core.NewInteger(5)
	n.ID = "n"
	n.SetPos(core.SrcPos{Row: 1, Col: 3})
	es.noteFnArgPos(1, []core.Value{n, fn})
	if got := es.fnArgPos[1]; len(got) != 2 || got[0].Row != 0 || got[1].Col != 101 {
		t.Errorf("a `/v`-read fn argument keeps its read's position: %v", got)
	}
	es.noteFnArgPos(2, []core.Value{n, unread, noID, noPos})
	if _, kept := es.fnArgPos[2]; kept {
		t.Error("no qualifying argument keeps nothing")
	}
	carrier := core.NewCarrier(core.TFunction)
	carrier.ID = "fnid"
	carrier.SetPos(core.SrcPos{Row: 2, Col: 1})
	es.noteFnArgPos(3, []core.Value{carrier})
	if got := es.fnArgPos[3]; len(got) != 1 || got[0].Row != 2 {
		t.Errorf("a fn-typed carrier: %v", got)
	}
}

func TestNUR329UnexpandedToken(t *testing.T) {
	reach := core.NewReachFromKeys(core.NewWord("m"), []core.Value{core.NewAtom("b")})
	carrier := reach
	carrier.Carrier = true
	dyn := reach
	dyn.Dynamic = true
	interp := core.NewInterpString([]core.InterpPart{{Lit: "s"}})
	for _, c := range []struct {
		v    core.Value
		want bool
	}{
		{reach, true}, {interp, true}, {carrier, false}, {dyn, false}, {core.NewInteger(1), false},
	} {
		if got := unexpandedToken(c.v); got != c.want {
			t.Errorf("%v: %v", c.v, got)
		}
	}
}
