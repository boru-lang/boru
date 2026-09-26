package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestFnReadRefused pins the refusal the seams ask of a closure unit that
// reads a param bare where it pushes the slot (NUR217's stored fn, NUR219's
// callback body): a fn argument in one of its FnReadParams slots is refused,
// data in the same slot is not, and a slot the unit does not list refuses
// nothing whatever it holds.
func TestFnReadRefused(t *testing.T) {
	fnv := core.NewFunction(core.FnDefInfo{Name: "one", Signatures: []core.Signature{{BarrierPos: 0}}})
	unit := &CompiledFn{FnReadParams: []int{1}}
	if !unit.FnReadRefused([]core.Value{core.NewInteger(1), fnv}) {
		t.Error("a fn in a listed slot must be refused")
	}
	// Negatives: data in the listed slot, a fn in an unlisted one, a listed
	// slot past the arguments, and a nil unit.
	if unit.FnReadRefused([]core.Value{fnv, core.NewInteger(1)}) {
		t.Error("a fn in an unlisted slot must not be refused")
	}
	if unit.FnReadRefused([]core.Value{core.NewInteger(1), core.NewInteger(2)}) {
		t.Error("data in a listed slot must not be refused")
	}
	if unit.FnReadRefused([]core.Value{fnv}) {
		t.Error("a listed slot past the arguments must not be read")
	}
	var none *CompiledFn
	if none.FnReadRefused([]core.Value{fnv}) {
		t.Error("a nil unit refuses nothing")
	}

	// The ref form: the unit its ref names, and nothing for a ref that names
	// no unit.
	prog := &Program{Fns: []CompiledFn{{}, {FnReadParams: []int{0}}}}
	if !(&CompiledFnRef{Prog: prog, Unit: 1}).RefusesArgs([]core.Value{fnv}) {
		t.Error("the ref must answer for the unit it names")
	}
	if (&CompiledFnRef{Prog: prog, Unit: 0}).RefusesArgs([]core.Value{fnv}) {
		t.Error("a unit with no listed slot refuses nothing")
	}
	for _, ref := range []*CompiledFnRef{nil, {}, {Prog: prog, Unit: -1}, {Prog: prog, Unit: 2}} {
		if ref.RefusesArgs([]core.Value{fnv}) {
			t.Errorf("a ref naming no unit refuses nothing: %+v", ref)
		}
	}
}

// TestCallbackSourceSpec pins the callback fn VALUE riding on its push's
// spec (NUR219): the contract fnValueRetSpec built is kept, and a value with
// no declared contract still carries its source — with no contract fields.
func TestCallbackSourceSpec(t *testing.T) {
	src := core.NewFunction(core.FnDefInfo{Name: "g"})
	got := callbackSourceSpec(&ClosureRetSpec{Name: "g", Types: []*core.Type{core.TInteger}}, src)
	if got.Source == nil || got.Name != "g" || len(got.Types) != 1 {
		t.Errorf("the contract must ride with the source: %+v", got)
	}
	bare := callbackSourceSpec(nil, src)
	if bare.Source == nil || bare.Name != "" || len(bare.Types) != 0 {
		t.Errorf("no contract means none added: %+v", bare)
	}
	if fd, ok := bare.Source.Data.(core.FnDefInfo); !ok || fd.Name != "g" {
		t.Errorf("the source must be the value handed in: %v", bare.Source)
	}
}
