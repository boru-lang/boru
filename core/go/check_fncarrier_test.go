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

// TestDropCheckFnCarrierBind: an undef drops the name's carrier from the
// pass-scoped table and leaves every other name bound; dropping a name the
// table does not hold, or with no table at all, changes nothing.
func TestDropCheckFnCarrierBind(t *testing.T) {
	r := covRegistry(t, nil)
	DropCheckFnCarrierBind(r, "a") // no table yet: a no-op
	NoteCheckFnCarrierBind(r, "a", NewCarrier(TFunction))
	NoteCheckFnCarrierBind(r, "b", NewCarrier(TFunction))
	DropCheckFnCarrierBind(r, "never-bound")
	if _, bound := CheckFnCarrierBind(r, "a"); !bound {
		t.Fatal("dropping another name must leave a bound")
	}
	DropCheckFnCarrierBind(r, "a")
	if _, bound := CheckFnCarrierBind(r, "a"); bound {
		t.Error("undef drops the bind")
	}
	if _, bound := CheckFnCarrierBind(r, "b"); !bound {
		t.Error("the drop reaches the named bind only")
	}
}
