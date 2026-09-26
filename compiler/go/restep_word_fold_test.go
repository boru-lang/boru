package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// restep_word_fold_test.go pins tryFoldReStepWord (edge-quote-1.tsv L28,
// edge-quote-3.tsv L56): a get-family read whose check-mode result is a LIVE
// word token — a quoted list's word node at a static index — emits nothing,
// because the check pass re-steps the token and records the call it makes;
// every other shape keeps its ordinary recording.

func restepGetArgs() []core.Value {
	lst := core.NewList([]core.Value{core.NewWord("add"), core.NewInteger(1), core.NewInteger(2)})
	lst.Quoted = true
	return []core.Value{core.NewInteger(0), lst}
}

func TestTryFoldReStepWordFoldsALiveWordRead(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.BeginCompilePass()
	defer done()
	es, _ := r.Check.Recorder().(*EmitState)
	before := len(es.frames[0])
	outs := []core.Value{core.NewWord("add")}
	if !tryFoldReStepWord(r, "get", restepGetArgs(), outs) {
		t.Fatal("a live word read from a concrete list at a static index must fold")
	}
	// The seam folds it too: recordDispatchOutcome records no event.
	recordDispatchOutcome(r, "get", nil, restepGetArgs(), outs, core.SrcPos{}, nil)
	if len(es.frames[0]) != before || !es.Compilable {
		t.Fatalf("the read must emit nothing and keep the program compilable (events %d -> %d, compilable %v)",
			before, len(es.frames[0]), es.Compilable)
	}
	// dot / getr are the same family.
	if !tryFoldReStepWord(r, "dot", restepGetArgs(), outs) || !tryFoldReStepWord(r, "getr", restepGetArgs(), outs) {
		t.Fatal("the get family folds alike")
	}
}

func TestTryFoldReStepWordDeclines(t *testing.T) {
	r := newTestRegistry(t)
	word := []core.Value{core.NewWord("add")}
	// No armed recorder: nothing to fold.
	if tryFoldReStepWord(r, "get", restepGetArgs(), word) {
		t.Fatal("an inactive recorder must decline")
	}
	done := r.Check.BeginCompilePass()
	defer done()
	carrier := core.NewCarrier(core.TList)
	intCarrier := core.NewCarrier(core.TInteger)
	cases := []struct {
		name string
		word string
		args []core.Value
		outs []core.Value
	}{
		{"not a get-family word", "size", restepGetArgs(), word},
		{"a one-operand window", "get", restepGetArgs()[:1], word},
		{"no result", "get", restepGetArgs(), nil},
		{"a data result", "get", restepGetArgs(), []core.Value{core.NewInteger(1)}},
		{"a bare type node is data", "get", restepGetArgs(), []core.Value{core.NewTypeLiteral(core.TInteger)}},
		{"a computed receiver", "get", []core.Value{core.NewInteger(0), carrier}, word},
		{"a map receiver", "get", []core.Value{core.NewInteger(0), core.NewMap(core.NewOrderedMap())}, word},
		{"a computed key", "get", []core.Value{intCarrier, restepGetArgs()[1]}, word},
		{"a string key", "get", []core.Value{core.NewString("a"), restepGetArgs()[1]}, word},
	}
	for _, c := range cases {
		if tryFoldReStepWord(r, c.word, c.args, c.outs) {
			t.Errorf("%s: must decline", c.name)
		}
	}
}
