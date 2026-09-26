package core

import "testing"

// Core-suite pins for NUR218's three analysis notes (the core cover gate
// counts only this suite, and the lang differential is what drives them
// there): the peek that consumes a group's `/v` marker notes the DELIVERED
// value, the standalone marker drop notes the carrier it quotes, and a bare
// read of a quoted fn-possible binding is read as the word. Positive and
// negative paired; each note is asserted through a recording stand-in.

// TestNUR218PeekNotesTheDelivery: a fn value followed by the marker is
// delivered — the marker consumed, the pointer past the value, the value
// unquoted — and under a compile pass the delivery is noted as a value read.
func TestNUR218PeekNotesTheDelivery(t *testing.T) {
	es := &wordReadEmit{EmitRecorder: TheInactiveEmit}
	r := compileCheckRegistry(t)
	r.Check.Emit = es
	fnv := anonFnVal([]FnParam{{Name: "a", Type: TInteger}}, nil, parenBody(NewWord("a")))
	fnv.ID = "T_peek_fn"
	e := NewTop(r)
	e.Tape = NewTape([]Value{fnv, NewDispatchMod(DispatchModInfo{Val: true})}, StackHeadroom)
	if err := e.execFnDefLiteral(0); err != nil {
		t.Fatalf("peek errored: %v", err)
	}
	if e.Tape.Len() != 1 || e.Pointer != 1 || e.Tape.At(0).Quoted {
		t.Errorf("the marker must be consumed and the value delivered unquoted: len=%d ptr=%d quoted=%v", e.Tape.Len(), e.Pointer, e.Tape.At(0).Quoted)
	}
	if len(es.vals) != 1 || es.vals[0] != "T_peek_fn" {
		t.Errorf("the delivery must be noted as a value read: %v", es.vals)
	}
}

// TestNUR218MarkerDropNotesTheCarrier: a standalone marker after a pass
// CARRIER (a member read the pass models, which the peek never reaches)
// quotes the carrier and notes the delivery; after a concrete value it is
// dropped with nothing noted.
func TestNUR218MarkerDropNotesTheCarrier(t *testing.T) {
	es := &wordReadEmit{EmitRecorder: TheInactiveEmit}
	r := compileCheckRegistry(t)
	r.Check.Emit = es
	c := NewDynamicCarrier(TAny)
	c.ID = "T_member_read"
	e := NewTop(r)
	e.Tape = NewTape([]Value{c, NewDispatchMod(DispatchModInfo{Val: true})}, StackHeadroom)
	e.Pointer = 1
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("marker drop errored: %v", err)
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Quoted {
		t.Errorf("the marker must be dropped and the carrier quoted: len=%d", e.Tape.Len())
	}
	if len(es.vals) != 1 || es.vals[0] != "T_member_read" {
		t.Errorf("the carrier's delivery must be noted: %v", es.vals)
	}
	// Negative: a concrete value before the marker is not a carrier — the
	// marker drops, nothing is quoted or noted.
	e.Tape = NewTape([]Value{NewInteger(7), NewDispatchMod(DispatchModInfo{Val: true})}, StackHeadroom)
	e.Pointer = 1
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("marker drop errored: %v", err)
	}
	if e.Tape.Len() != 1 || e.Tape.At(0).Quoted || len(es.vals) != 1 {
		t.Errorf("a concrete value is left alone: quoted=%v notes=%v", e.Tape.At(0).Quoted, es.vals)
	}
}

// TestNUR218QuotedFnBindingReadsAsTheWord: under a compile pass a bare read
// of a binding whose stand-in is a QUOTED fn-possible carrier (a def bound
// what a `/v` marker drop left) is read as the word — noted as a word read —
// while a quoted carrier that cannot hold a fn stays the quoted data it is.
func TestNUR218QuotedFnBindingReadsAsTheWord(t *testing.T) {
	es := &wordReadEmit{EmitRecorder: TheInactiveEmit}
	r := compileCheckRegistry(t)
	r.Check.Emit = es
	g := NewDynamicCarrier(TAny)
	g.ID = "T_bound_member"
	g.Quoted = true
	r.Defs.Push("g", g)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("g")}, StackHeadroom)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("bare read errored: %v", err)
	}
	if len(es.noted) != 1 || es.noted[0] != "g" {
		t.Errorf("a quoted fn-possible binding must be read as the word: %v", es.noted)
	}
	// Negative: a quoted Integer carrier is data — not noted.
	n := NewCarrier(TInteger)
	n.ID = "T_bound_int"
	n.Quoted = true
	r.Defs.Push("n", n)
	e.Tape = NewTape([]Value{NewWord("n")}, StackHeadroom)
	e.Pointer = 0
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("bare read errored: %v", err)
	}
	if len(es.noted) != 1 {
		t.Errorf("a quoted non-fn carrier must not be read as a word: %v", es.noted)
	}
}
