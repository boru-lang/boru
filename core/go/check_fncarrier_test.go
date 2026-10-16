package core

import (
	"testing"
)

// TestCheckFnCarrierBindTable drives the side table's arms directly: the
// nil-registry reset, the absent-table reset, the create and reuse write
// arms, a miss, and the pass-scoped clearing. (The lang-level twin
// exercises the shim aliases; this one keeps core's own coverage whole
// after the move down from basic.)
func TestCheckFnCarrierBindTable(t *testing.T) {
	ResetCheckFnCarrierBinds(nil) // nil-registry arm
	r := covRegistry(t, nil)
	ResetCheckFnCarrierBinds(r) // absent table: no-op
	if _, hit := CheckFnCarrierBind(r, "x"); hit {
		t.Error("empty table must miss")
	}
	NoteCheckFnCarrierBind(r, "a", NewCarrier(TFunction)) // create arm
	NoteCheckFnCarrierBind(r, "b", NewCarrier(TFunction)) // reuse arm
	if _, hit := CheckFnCarrierBind(r, "a"); !hit {
		t.Error("noted name must resolve")
	}
	if _, hit := CheckFnCarrierBind(r, "zz"); hit {
		t.Error("unnoted name must miss")
	}
	ResetCheckFnCarrierBinds(r)
	if _, hit := CheckFnCarrierBind(r, "a"); hit {
		t.Error("reset must clear the pass-scoped table")
	}
}

// TestCheckFnCarrierBindDepth: the table keeps the OUTERMOST fn-body depth
// a name was bound at (NUR192's shadow test) — a deeper rebind does not
// hide the enclosing bind, a shallower one lowers it — and the depth goes
// with the bind on undef and on the pass-scoped reset.
func TestCheckFnCarrierBindDepth(t *testing.T) {
	r := covRegistry(t, nil)
	if _, bound := CheckFnCarrierBindDepth(r, "a"); bound {
		t.Error("an unbound name has no depth")
	}
	r.Check.FnBodyDepth = 0
	NoteCheckFnCarrierBind(r, "a", NewCarrier(TFunction))
	r.Check.FnBodyDepth = 1
	NoteCheckFnCarrierBind(r, "a", NewCarrier(TFunction))
	NoteCheckFnCarrierBind(r, "b", NewCarrier(TFunction))
	if d, bound := CheckFnCarrierBindDepth(r, "a"); !bound || d != 0 {
		t.Errorf("the outermost depth stays: got %d %v, want 0", d, bound)
	}
	if d, bound := CheckFnCarrierBindDepth(r, "b"); !bound || d != 1 {
		t.Errorf("a name first bound in a fn body records that depth: got %d %v, want 1", d, bound)
	}
	r.Check.FnBodyDepth = 0
	NoteCheckFnCarrierBind(r, "b", NewCarrier(TFunction))
	if d, _ := CheckFnCarrierBindDepth(r, "b"); d != 0 {
		t.Errorf("a shallower bind lowers the depth: got %d, want 0", d)
	}
	DropCheckFnCarrierBind(r, "a")
	if _, bound := CheckFnCarrierBindDepth(r, "a"); bound {
		t.Error("undef drops the depth with the bind")
	}
	ResetCheckFnCarrierBinds(r)
	if _, bound := CheckFnCarrierBindDepth(r, "b"); bound {
		t.Error("the reset clears the depths with the table")
	}
	// A registry without a check state records depth 0.
	r2 := covRegistry(t, nil)
	r2.Check = nil
	NoteCheckFnCarrierBind(r2, "c", NewCarrier(TFunction))
	if d, bound := CheckFnCarrierBindDepth(r2, "c"); !bound || d != 0 {
		t.Errorf("no check state: depth 0, got %d %v", d, bound)
	}
}

// TestStepWordCarrierIsData — a word-TYPED CARRIER at the pointer is DATA,
// not a token. `IsWord` classifies on the parent type alone, so a carrier of
// type Word used to be dispatched as a word; it has no WordInfo, so the name
// was "" and every name arm fell through to the undefined-word tail, emitting
// a diagnostic that named no word and carried no position (NUR103). stepWord
// now collects it like any other value: there is nothing to look up and no
// arguments to collect.
func TestStepWordCarrierIsData(t *testing.T) {
	r := covRegistry(t, nil)
	end := r.Check.Begin()
	defer end()

	carrier := NewCarrier(TWord)
	e := NewTop(r)
	e.Tape = NewTape([]Value{carrier}, StackHeadroom)
	if err := e.stepWord(e.Tape.Ref(0)); err != nil {
		t.Fatalf("stepWord over a word carrier errored: %v", err)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("a word carrier raised diagnostics: %v", r.Check.Diagnostics)
	}
	if got := e.Tape.At(0); IsUndefined(&got) || !got.Carrier() || !got.Parent().Equal(TWord) {
		t.Errorf("word carrier not collected as data: %v", got)
	}
}

// TestCheckFnCarrierBindDepthWithoutDepthTable pins the depth read's second
// guard: a name the BINDS table holds while its depth twin is absent (the
// two are separate registry capabilities — a writer that sets one alone, or
// a partial reset, leaves exactly this state) reads as bound at no depth
// rather than panicking on a nil map or inventing depth 0.
func TestCheckFnCarrierBindDepthWithoutDepthTable(t *testing.T) {
	r := covRegistry(t, nil)
	if err := r.Capabilities.Set(capCheckFnCarrierBinds, map[string]Value{"solo": NewCarrier(TFunction)}); err != nil {
		t.Fatalf("Capabilities.Set: %v", err)
	}
	if _, bound := CheckFnCarrierBind(r, "solo"); !bound {
		t.Fatal("the binds table alone resolves the bind")
	}
	if d, bound := CheckFnCarrierBindDepth(r, "solo"); bound || d != 0 {
		t.Errorf("no depth table: the name has no bind depth, got %d %v", d, bound)
	}
}
