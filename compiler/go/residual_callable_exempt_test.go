package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestResidualCallableExempt pins which program residual entries the
// rebuild's CALLABLE screen need not hold against it. A root read of a
// gradual def-bound value is its own test's (NUR207) while the residual
// holds it no more often than the program reads it; a call result held once
// was modelled at its dispatch. A value held twice (a copy), an entry nothing
// produced, an ID-less entry, and every entry once a pick/roll fold has run
// keep the screen.
func TestResidualCallableExempt(t *testing.T) {
	es := NewEmitState()
	read := core.NewDynamicCarrier(core.TAny)
	copied := core.NewDynamicCarrier(core.TAny)
	result := core.NewDynamicCarrier(core.TType)
	twice := core.NewDynamicCarrier(core.TAny)
	loose := core.NewDynamicCarrier(core.TAny)
	idless := core.NewDynamicCarrier(core.TAny)
	idless.ID = ""
	for i, v := range []core.Value{read, copied, result, twice} {
		es.producedBy[v.ID] = producer{seq: i + 1}
	}
	reads := map[string]rootWordRead{
		read.ID:   {name: "j", reads: []core.SrcPos{{Row: 1, Col: 1}}},
		copied.ID: {name: "k", reads: []core.SrcPos{{Row: 1, Col: 5}}},
	}
	residual := []core.Value{read, copied, copied, result, twice, twice, loose, idless}
	exempt := es.residualCallableExempt(residual, reads)
	for _, c := range []struct {
		v    core.Value
		want bool
		what string
	}{
		{read, true, "a root read held once"},
		{copied, false, "a root read held more often than it is read"},
		{result, true, "a call result held once"},
		{twice, false, "a call result held twice"},
		{loose, false, "an entry no event produced"},
	} {
		if exempt[c.v.ID] != c.want {
			t.Errorf("%s: exempt %v, want %v", c.what, exempt[c.v.ID], c.want)
		}
	}
	if exempt[""] {
		t.Error("an ID-less entry is never exempt")
	}
	if !residualMayBeCallable(residual, exempt) {
		t.Error("the unexempt dynamic entries keep the screen")
	}
	if residualMayBeCallable([]core.Value{read, result, core.NewInteger(1)}, exempt) {
		t.Error("exempt entries and data pass the screen")
	}

	// A pick/roll fold's copies: nothing is exempt.
	es.shuffleFolded = true
	if got := es.residualCallableExempt(residual, reads); got != nil {
		t.Errorf("after a shuffle fold nothing is exempt, got %v", got)
	}
}

// TestFoldFullStackLatchesShuffle pins the latch the exemption reads: a
// pick or roll fold sets it, a depth fold does not.
func TestFoldFullStackLatchesShuffle(t *testing.T) {
	a, b := core.NewInteger(1), core.NewInteger(2)
	a.ID, b.ID = core.GenerateID("S_"), core.GenerateID("S_")
	es := NewEmitState()
	if _, ok := es.FoldFullStack("depth", nil, []core.Value{a, b}); !ok || es.shuffleFolded {
		t.Fatalf("depth folds without latching (ok=%v latched=%v)", ok, es.shuffleFolded)
	}
	for _, word := range []string{"pick", "roll"} {
		es = NewEmitState()
		if _, ok := es.FoldFullStack(word, []core.Value{core.NewInteger(1)}, []core.Value{a, b}); !ok || !es.shuffleFolded {
			t.Errorf("%s folds and latches (ok=%v latched=%v)", word, ok, es.shuffleFolded)
		}
	}
}
