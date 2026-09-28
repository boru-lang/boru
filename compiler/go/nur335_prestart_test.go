package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur335_prestart_test.go pins how a root statement island finds the values
// beneath its statement (landing_restart.go rootPreStart, NUR335/NUR336):
// each residual entry by where it was PUSHED — an event's result by the
// event, a literal by its token, a def-bound value's read by the reads — and
// where it may take a statement over (past its opening defs of a literal).
// The lang suite drives them end to end
// (TestNUR335IslandSeatsReadsWhereTheyWereRead, TestNUR336*).

func n335Val(id string, row, col int) core.Value {
	v := core.NewInteger(1)
	v.ID = id
	v.SetPos(core.SrcPos{Row: row, Col: col})
	return v
}

func n335Map(row, col int) core.Value {
	om := core.NewOrderedMap()
	om.Set("a", core.NewInteger(1))
	m := core.NewMap(om)
	m.ID = "map"
	m.SetPos(core.SrcPos{Row: row, Col: col})
	return m
}

func n335Call(seq, row, col int) treeEvent {
	return treeEvent{ev: &EmitEvent{kind: evCall, seq: seq, call: emitCall{word: "t", pos: core.SrcPos{Row: row, Col: col}}}}
}

// statementPushed tells an entry's statement by its push: an event's result
// by the event's position, or its seq when the event has none (or is not the
// tree's); a literal by its token; a compound with no position by the
// top-level tokens spelling it, when they stand on one side of start.
func TestStatementPushed(t *testing.T) {
	es := NewEmitState()
	start := core.SrcPos{Row: 2, Col: 1}
	tree := map[int]treeEvent{3: n335Call(3, 1, 4), 5: n335Call(5, 2, 6), 6: n335Call(6, 0, 0)}
	es.producedBy["early"] = producer{seq: 3}
	es.producedBy["mine"] = producer{seq: 5}
	es.producedBy["nopos"] = producer{seq: 6}
	es.producedBy["gone"] = producer{seq: 2}
	for _, c := range []struct {
		name       string
		rv         core.Value
		own, known bool
	}{
		{"an event written before the statement", n335Val("early", 0, 0), false, true},
		{"an event written in it, however late recorded", n335Val("mine", 0, 0), true, true},
		{"an event with no position, recorded at its first event", n335Val("nopos", 0, 0), true, true},
		{"an event the tree does not hold, recorded before it", n335Val("gone", 0, 0), false, true},
		{"a literal written before it", n335Val("lit", 1, 2), false, true},
		{"a literal written in it", n335Val("lit", 2, 3), true, true},
		{"a scalar with no position", n335Val("lit", 0, 0), false, false},
	} {
		own, known := es.statementPushed(tree, c.rv, start, 6)
		if own != c.own || known != c.known {
			t.Errorf("%s: own=%v known=%v, want %v %v", c.name, own, known, c.own, c.known)
		}
	}
	folded := n335Map(0, 0)
	for _, c := range []struct {
		name       string
		body       []core.Value
		own, known bool
	}{
		{"spelled before the statement only", []core.Value{n335Map(1, 1), restartTok(1, 5)}, false, true},
		{"spelled in it only", []core.Value{restartTok(1, 1), n335Map(2, 3)}, true, true},
		{"spelled on both sides", []core.Value{n335Map(1, 1), n335Map(2, 3)}, true, false},
		{"spelled nowhere", []core.Value{restartTok(1, 1), n335Map(0, 0)}, false, false},
	} {
		es.rootBody = c.body
		own, known := es.statementPushed(tree, folded, start, 6)
		if own != c.own || known != c.known {
			t.Errorf("a folded map %s: own=%v known=%v, want %v %v", c.name, own, known, c.own, c.known)
		}
	}
}

// boundReadsBefore finds the reads of a def-bound value on the residual —
// the def's own value, and for a literal at the def's own position — and how
// many of them the stack holds beneath the statement: by the reads'
// positions when those decide it, or refuses.
func TestBoundReadsBefore(t *testing.T) {
	start := core.SrcPos{Row: 2, Col: 1}
	def := func(v core.Value) map[int]treeEvent {
		return map[int]treeEvent{1: {ev: &EmitEvent{kind: evDynBind, seq: 1, dyn: &emitDynBind{name: "k", val: v, srcSeq: -1}}}}
	}
	k := n335Val("k", 1, 3)
	pre, in := core.SrcPos{Row: 1, Col: 9}, core.SrcPos{Row: 2, Col: 2}
	for _, c := range []struct {
		name   string
		reads  []core.SrcPos
		n      int
		before int
		ok     bool
	}{
		{"read before the statement only", []core.SrcPos{pre}, 1, 1, true},
		{"read in it only", []core.SrcPos{in}, 1, 0, true},
		{"read on both sides, none consumed", []core.SrcPos{pre, in}, 2, 1, true},
		{"read on both sides, one consumed", []core.SrcPos{pre, in}, 1, 0, false},
		{"more copies than reads", []core.SrcPos{pre}, 2, 0, false},
	} {
		es := NewEmitState()
		es.rootLocalReads = map[string][]core.SrcPos{"k": c.reads}
		residual := make([]core.Value, c.n)
		for i := range residual {
			residual[i] = k
		}
		before, reads, ok := es.boundReadsBefore(def(k), residual, start)
		if ok != c.ok || (ok && (before["k"] != c.before || !reads[0])) {
			t.Errorf("%s: before=%v reads=%v ok=%v, want %d %v", c.name, before, reads, ok, c.before, c.ok)
		}
	}
	// A `/v` read's position is not kept: it refuses.
	es := NewEmitState()
	es.rootLocalReads = map[string][]core.SrcPos{"k": {pre}}
	es.valReadNoted = map[string]bool{"k": true}
	if _, _, ok := es.boundReadsBefore(def(k), []core.Value{k}, start); ok {
		t.Error("a value read by `/v` cannot be placed")
	}
	// A shared value spelled elsewhere (a type literal) is its token's own
	// push, not a read; an event's result the def bound is a read wherever
	// its value says it came from.
	es = NewEmitState()
	if _, reads, ok := es.boundReadsBefore(def(k), []core.Value{n335Val("k", 1, 1)}, start); !ok || reads[0] {
		t.Errorf("the same value at another token is no read: reads=%v ok=%v", reads, ok)
	}
	es.producedBy["k"] = producer{seq: 0}
	es.rootLocalReads = map[string][]core.SrcPos{"k": {in}}
	if before, reads, ok := es.boundReadsBefore(def(k), []core.Value{n335Val("k", 0, 0)}, start); !ok || !reads[0] || before["k"] != 0 {
		t.Errorf("a def-bound event result is read where its reads are: before=%v reads=%v ok=%v", before, reads, ok)
	}
	if !containsPos([]core.SrcPos{pre, in}, in) || containsPos([]core.SrcPos{pre}, in) {
		t.Error("containsPos")
	}
}

// rootPreStart seats a read made before the statement and stops at one the
// statement makes; an entry it cannot tell, or a read it cannot count, plans
// no island.
func TestRootPreStartReads(t *testing.T) {
	start := core.SrcPos{Row: 2, Col: 1}
	k := n335Val("k", 1, 3)
	tree := map[int]treeEvent{1: {ev: &EmitEvent{kind: evDynBind, seq: 1, dyn: &emitDynBind{name: "k", val: k, srcSeq: -1}}}}
	es := NewEmitState()
	lw := &lowerer{es: es, promoted: map[int]int{}}
	es.rootLocalReads = map[string][]core.SrcPos{"k": {{Row: 1, Col: 9}, {Row: 2, Col: 2}}}
	srcs, held, _, ok := es.rootPreStart(lw, tree, []core.Value{k, k, n335Val("after", 2, 5)}, start, 8)
	if !ok || held != 0 || len(srcs) != 1 || srcs[0].Kind != RestartConst {
		t.Errorf("the earlier read is seated, the statement's own is not: %+v held=%d ok=%v", srcs, held, ok)
	}
	if _, _, _, ok := es.rootPreStart(lw, tree, []core.Value{k}, start, 8); ok {
		t.Error("reads on both sides, one consumed, cannot be counted")
	}
	if _, _, _, ok := es.rootPreStart(lw, nil, []core.Value{n335Map(0, 0)}, start, 8); ok {
		t.Error("a folded compound spelled nowhere cannot be placed")
	}
	carrier := n335Val("c", 1, 1)
	carrier.Carrier = true
	if _, _, _, ok := es.rootPreStart(lw, nil, []core.Value{carrier}, start, 8); ok {
		t.Error("a carrier beneath the statement has no value to seat")
	}
}

// parenOf writes a paren's shaped apply as its value: its method is the
// paren's first token, so the apply takes one operand more than the paren's
// other tokens.
func TestParenOfShapedApply(t *testing.T) {
	head, arg := restartTok(1, 2), restartTok(1, 4)
	paren := core.NewParenExpr([]core.Value{head, arg})
	paren.SetPos(core.SrcPos{Row: 1, Col: 1})
	body := []core.Value{paren}
	apply := EmitEvent{kind: evCall, call: emitCall{word: "(paren apply)", nout: 1, pos: head.Pos(),
		ops: []EmitOperand{EventOperand(1, 0), EventOperand(2, 0)}, dynMethod: &DynMethodSpec{NArgs: 1, NOut: 1}}}
	if path, ok := parenOf(&apply, body, 0); !ok || len(path) != 1 || path[0] != 0 {
		t.Errorf("a paren's shaped apply is the paren's value: path=%v ok=%v", path, ok)
	}
	call := apply
	call.call.dynMethod = nil
	if _, ok := parenOf(&call, body, 0); ok {
		t.Error("a call taking both tokens as operands is not the paren's first token's call")
	}
}

// collectWordNames reads a reach's receiver and its computed keys: the
// island stepping `(q.f 7)` resolves q.
func TestCollectWordNamesReach(t *testing.T) {
	reach := core.NewReach(core.ReachInfo{
		Receiver: []core.Value{core.NewWord("q")},
		Segments: []core.ReachSeg{{KeyLit: core.NewString("f")}, {Computed: true, KeyExpr: []core.Value{core.NewWord("key")}}},
	})
	names := map[string]bool{}
	collectWordNames([]core.Value{reach}, names)
	if !names["q"] || !names["key"] || len(names) != 2 {
		t.Errorf("a reach's names: %v", names)
	}
}

// literalDefsBefore moves a statement island past the defs of a literal its
// statement opens with — each `def`, a name and a scalar the pass recorded a
// literal def at — never onto or past the token holding the stop.
func TestLiteralDefsBefore(t *testing.T) {
	word := func(name string, col int) core.Value {
		w := core.NewWord(name)
		w.SetPos(core.SrcPos{Row: 1, Col: col})
		return w
	}
	lit := func(col int) core.Value { return n335Val("", 1, col) }
	body := []core.Value{word("def", 1), word("k", 5), lit(7), word("def", 9), word("j", 13), lit(15), word("k", 17), word("x", 19)}
	bind := func(col int) treeEvent {
		return treeEvent{ev: &EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "n", srcSeq: -1, pos: core.SrcPos{Row: 1, Col: col}}}}
	}
	stop := core.SrcPos{Row: 1, Col: 19}
	for _, c := range []struct {
		name string
		tree map[int]treeEvent
		stop core.SrcPos
		want int
	}{
		{"two literal defs", map[int]treeEvent{1: bind(5), 2: bind(13)}, stop, 6},
		{"a def the pass recorded no literal bind for", map[int]treeEvent{1: bind(5)}, stop, 3},
		{"no recorded def", nil, stop, 0},
		{"the stop inside the second def", map[int]treeEvent{1: bind(5), 2: bind(13)}, core.SrcPos{Row: 1, Col: 13}, 3},
		{"the stop inside the first def", map[int]treeEvent{1: bind(5), 2: bind(13)}, core.SrcPos{Row: 1, Col: 7}, 0},
	} {
		if got := literalDefsBefore(c.tree, body, 0, c.stop); got != c.want {
			t.Errorf("%s: token %d, want %d", c.name, got, c.want)
		}
	}
	computed := map[int]treeEvent{1: {ev: &EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "k", srcSeq: 4, pos: core.SrcPos{Row: 1, Col: 5}}}}}
	if got := literalDefsBefore(computed, body, 0, stop); got != 0 {
		t.Errorf("a def of a computed value is no literal def: token %d", got)
	}
	notDef := []core.Value{word("let", 1), word("k", 5), lit(7), word("x", 19)}
	if got := literalDefsBefore(map[int]treeEvent{1: bind(5)}, notDef, 0, stop); got != 0 {
		t.Errorf("another word opens no def: token %d", got)
	}
	wordVal := []core.Value{word("def", 1), word("k", 5), word("v", 7), word("x", 19)}
	if got := literalDefsBefore(map[int]treeEvent{1: bind(5)}, wordVal, 0, stop); got != 0 {
		t.Errorf("a def of a word is no literal def: token %d", got)
	}
}

// reachReceivers finds a reach's receiver words through parens and list
// literals, and a read there is the reach's own: boundReadsBefore does not
// count it (`m end m (m.f 7)`).
func TestReachReceivers(t *testing.T) {
	recv := core.NewWord("m")
	recv.SetPos(core.SrcPos{Row: 1, Col: 4})
	reach := core.NewReach(core.ReachInfo{Receiver: []core.Value{recv}, Segments: []core.ReachSeg{{KeyLit: core.NewString("f")}}})
	nested := core.NewReach(core.ReachInfo{Receiver: []core.Value{reach}, Segments: []core.ReachSeg{{KeyLit: core.NewString("g")}}})
	list := core.NewList([]core.Value{nested})
	list.Eval = true
	at := map[core.SrcPos]bool{}
	reachReceivers([]core.Value{core.NewParenExpr([]core.Value{list}), restartTok(1, 9)}, at)
	if !at[core.SrcPos{Row: 1, Col: 4}] || len(at) != 1 {
		t.Errorf("the receiver's word, through a paren, a list and a nested reach: %v", at)
	}
	es := NewEmitState()
	es.rootBody = []core.Value{core.NewParenExpr([]core.Value{reach})}
	k := n335Val("k", 1, 1)
	tree := map[int]treeEvent{1: {ev: &EmitEvent{kind: evDynBind, seq: 1, dyn: &emitDynBind{name: "k", val: k, srcSeq: -1}}}}
	es.rootLocalReads = map[string][]core.SrcPos{"k": {{Row: 0, Col: 9}, {Row: 1, Col: 4}, {Row: 2, Col: 2}}}
	before, _, ok := es.boundReadsBefore(tree, []core.Value{k, k}, core.SrcPos{Row: 2, Col: 1})
	if !ok || before["k"] != 1 {
		t.Errorf("the receiver's read is the reach's, not the stack's: before=%v ok=%v", before, ok)
	}
}
