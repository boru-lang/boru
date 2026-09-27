package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// handlerBody is `w end do [raise] error (mk) drop`: the statement begins at
// token 2, the do word is token 2 (column 5), its literal body token 3, the
// error word token 4 (column 15) and its computed handler the paren at
// token 5 (mk at column 22).
func handlerBody() []core.Value {
	list := gtok(core.NewEvalList([]core.Value{gtok(core.NewWord("raise"), 8)}), 7)
	paren := gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 22)}), 21)
	return []core.Value{
		gtok(core.NewWord("w"), 1), gtok(core.NewEnd(), 3), gtok(core.NewWord("do"), 5), list,
		gtok(core.NewWord("error"), 15), paren, gtok(core.NewWord("drop"), 27),
	}
}

// handlerTree is the do (seq 10) over its body's closure, the handler's
// factory call (seq 11) and the error call (seq 12) that takes both.
func handlerTree() (map[int]treeEvent, *EmitEvent) {
	do := EmitEvent{kind: evCall, seq: 10, call: emitCall{word: "do", pos: gpos(5), ops: []EmitOperand{{kind: opClosure}}}}
	mk := EmitEvent{kind: evCall, seq: 11, call: emitCall{word: "mk", pos: gpos(22)}}
	errc := EmitEvent{kind: evCall, seq: 12, call: emitCall{word: "error", pos: gpos(15), ops: []EmitOperand{EventOperand(11, 0), EventOperand(10, 0)}}}
	return map[int]treeEvent{10: {ev: &do}, 11: {ev: &mk}, 12: {ev: &errc}}, &errc
}

// TestErrorCountPoint pins NUR300's count island: a computed handler's run
// is written over the do, its body, the word and the handler — four tokens —
// and the events inside them (the do, the factory call) plan nothing.
func TestErrorCountPoint(t *testing.T) {
	es := NewEmitState()
	es.argSites = map[int][]argSite{12: {{seq: 11}, {seq: 10}}}
	tree, _ := handlerTree()
	tok, substs, ok := es.countPoint(tree, 12, handlerBody())
	if !ok || tok != 2 || len(substs) != 1 || !substs[0].results || substs[0].span != 4 || substs[0].path[0] != 2 {
		t.Errorf("the do, its body, the word and the handler are the run: %d %+v %v", tok, substs, ok)
	}
	for _, c := range []struct {
		why  string
		edit func(tree map[int]treeEvent, errc *EmitEvent, body []core.Value)
	}{
		{"a handler that is not the token after the word", func(_ map[int]treeEvent, _ *EmitEvent, _ []core.Value) {
			es.argSites[12] = []argSite{{pos: gpos(27), seq: -1}, {seq: 10}}
		}},
		{"a handler site whose producer is not in the statement", func(_ map[int]treeEvent, _ *EmitEvent, _ []core.Value) {
			es.argSites[12] = []argSite{{seq: 40}, {seq: 10}}
		}},
		{"an error call with one site", func(_ map[int]treeEvent, _ *EmitEvent, _ []core.Value) {
			es.argSites[12] = []argSite{{seq: 11}}
		}},
		{"a caught value that is no event", func(_ map[int]treeEvent, errc *EmitEvent, _ []core.Value) {
			errc.call.ops[1] = EmitOperand{kind: opLocal}
		}},
		{"a caught value no do produced", func(_ map[int]treeEvent, errc *EmitEvent, _ []core.Value) {
			errc.call.ops[1] = EventOperand(11, 0)
		}},
		{"a do over a computed body", func(tree map[int]treeEvent, _ *EmitEvent, _ []core.Value) {
			tree[10].ev.call.ops[0] = EventOperand(11, 0)
		}},
		{"a do not written before the word", func(_ map[int]treeEvent, _ *EmitEvent, body []core.Value) {
			body[2] = gtok(core.NewInteger(9), 5)
		}},
		{"a quoted body", func(_ map[int]treeEvent, _ *EmitEvent, body []core.Value) {
			body[3].Quoted = true
		}},
	} {
		es.argSites = map[int][]argSite{12: {{seq: 11}, {seq: 10}}}
		tree, errc := handlerTree()
		body := handlerBody()
		c.edit(tree, errc, body)
		if _, _, ok := es.countPoint(tree, 12, body); ok {
			t.Errorf("%s: no count island", c.why)
		}
	}
	// An error word with no room for a do before it.
	es.argSites = map[int][]argSite{12: {{seq: 11}, {seq: 10}}}
	tree, _ = handlerTree()
	short := handlerBody()[3:]
	if _, _, ok := es.countPoint(tree, 12, short); ok {
		t.Error("an error word at its statement's second token has no do before it")
	}
}

// TestCountPointNeedsARun: a do word last on its level has no body token
// after it, and a call positioned outside the body stands in no statement.
func TestCountPointNeedsARun(t *testing.T) {
	es := NewEmitState()
	do := countDo(7)
	tree := map[int]treeEvent{10: {ev: &do}}
	if _, _, ok := es.countPoint(tree, 10, countBody()[:4]); ok {
		t.Error("a do word last on its level has no run to write")
	}
	away := countDo(7)
	away.call.pos = core.SrcPos{}
	if _, _, ok := es.countPoint(map[int]treeEvent{10: {ev: &away}}, 10, countBody()); ok {
		t.Error("a call outside the body has no statement")
	}
}

func TestInSpan(t *testing.T) {
	body := []core.Value{
		gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("a"), 2), gtok(core.NewWord("b"), 4)}), 1),
		gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("c"), 8), gtok(core.NewWord("d"), 10)}), 7),
	}
	if !inSpan(body, gpos(10), []int{1, 0}, 2) || inSpan(body, gpos(10), []int{1, 0}, 1) {
		t.Error("d is the second token of the second paren")
	}
	if inSpan(body, gpos(4), []int{1, 0}, 2) {
		t.Error("b stands in the first paren, not the second")
	}
	if inSpan(body, gpos(1), []int{1, 0}, 2) {
		t.Error("the first paren's own position is not inside the second")
	}
}

// islandBody is `w end do [raise] error [drop 5 6] drop`: the literal
// handler at token 5 (column 21) where handlerBody writes a paren.
func islandBody() []core.Value {
	body := handlerBody()
	body[5] = gtok(core.NewEvalList([]core.Value{gtok(core.NewWord("drop"), 22)}), 21)
	return body
}

// islandTree is the do (seq 10) over its body's closure and the strip
// island (seq 12) threading the do's value.
func islandTree() (map[int]treeEvent, *EmitEvent) {
	do := EmitEvent{kind: evCall, seq: 10, call: emitCall{word: "do", pos: gpos(5), ops: []EmitOperand{{kind: opClosure}}}}
	isl := EmitEvent{kind: evFallback, seq: 12, fb: emitFallback{pos: gpos(15), ins: []EmitOperand{EventOperand(10, 0)}}}
	return map[int]treeEvent{10: {ev: &do}, 12: {ev: &isl}}, &isl
}

// TestIslandCountPoint pins NUR301's count island over a literal handler's
// interpreter island: the do, its body, the word and the handler are the
// run, as a computed handler's are.
func TestIslandCountPoint(t *testing.T) {
	es := NewEmitState()
	es.eventInfo = map[int]eventFlags{12: {stripIsland: true}}
	tree, _ := islandTree()
	tok, substs, ok := es.countPoint(tree, 12, islandBody())
	if !ok || tok != 2 || len(substs) != 1 || substs[0].span != 4 || substs[0].path[0] != 2 {
		t.Errorf("the do, its body, the word and the handler are the run: %d %+v %v", tok, substs, ok)
	}
	for _, c := range []struct {
		why  string
		edit func(tree map[int]treeEvent, isl *EmitEvent, body []core.Value)
	}{
		{"an island threading no value", func(_ map[int]treeEvent, isl *EmitEvent, _ []core.Value) { isl.fb.ins = nil }},
		{"a threaded value that is no event", func(_ map[int]treeEvent, isl *EmitEvent, _ []core.Value) {
			isl.fb.ins = []EmitOperand{{kind: opLocal}}
		}},
		{"a threaded value no do left", func(tree map[int]treeEvent, _ *EmitEvent, _ []core.Value) { tree[10].ev.call.word = "dup" }},
		{"a do over a computed body", func(tree map[int]treeEvent, _ *EmitEvent, _ []core.Value) {
			tree[10].ev.call.ops[0] = EventOperand(11, 0)
		}},
		{"a do outside the statement", func(tree map[int]treeEvent, _ *EmitEvent, _ []core.Value) { delete(tree, 10) }},
		{"a handler that is no literal list", func(_ map[int]treeEvent, _ *EmitEvent, body []core.Value) {
			body[5] = gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 22)}), 21)
		}},
		{"a quoted body", func(_ map[int]treeEvent, _ *EmitEvent, body []core.Value) { body[3].Quoted = true }},
		{"a do not written before the word", func(_ map[int]treeEvent, _ *EmitEvent, body []core.Value) {
			body[2] = gtok(core.NewInteger(9), 5)
		}},
		{"an island that is no strip word's", func(_ map[int]treeEvent, _ *EmitEvent, _ []core.Value) {
			es.eventInfo[12] = eventFlags{}
		}},
	} {
		es.eventInfo = map[int]eventFlags{12: {stripIsland: true}}
		tree, isl := islandTree()
		body := islandBody()
		c.edit(tree, isl, body)
		if _, _, ok := es.countPoint(tree, 12, body); ok {
			t.Errorf("%s: no count island", c.why)
		}
	}
	// An island word at its statement's second token has no do before it.
	es.eventInfo = map[int]eventFlags{12: {stripIsland: true}}
	tree, _ = islandTree()
	if _, _, ok := es.countPoint(tree, 12, islandBody()[3:]); ok {
		t.Error("an island word at its statement's second token has no do before it")
	}
}

// TestDoBeforeComputedBody pins the do before a handler over a computed
// body: its body token is its one argument site (doBodyAfter), so the do,
// the read, the word and the handler are one run; a site elsewhere is none.
func TestDoBeforeComputedBody(t *testing.T) {
	es := NewEmitState()
	body := islandBody()
	body[3] = gtok(core.NewWord("b"), 8)
	tree, _ := islandTree()
	tree[10].ev.call.ops[0] = EventOperand(9, 0)
	es.eventInfo = map[int]eventFlags{12: {stripIsland: true}}
	es.argSites = map[int][]argSite{10: {{pos: gpos(8), seq: -1}}}
	if _, substs, ok := es.countPoint(tree, 12, body); !ok || len(substs) != 1 || substs[0].span != 4 {
		t.Errorf("a read body is the do's run: %+v %v", substs, ok)
	}
	es.argSites[10] = []argSite{{pos: gpos(27), seq: -1}}
	if _, _, ok := es.countPoint(tree, 12, body); ok {
		t.Error("a body site elsewhere is no run")
	}
}
