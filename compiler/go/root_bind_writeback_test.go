package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// rootBindWritesBack is the ONE rule both the lowering (lowerDynBind's
// needGlobal) and the bind-consumes planner (collectRootBindConsumes) read:
// a root def writes its runtime value back over the kept check-pass binding
// exactly when that binding is not the runtime value. The table is the
// rule's own doc, case for case — the twin-carrier class the COLLECT
// oracle found (a computed compound, IsConcrete or not) writes back; a
// scalar fold, a literal, a bare node do not.
func TestRootBindWritesBackByProvenance(t *testing.T) {
	carrier := core.NewCarrier(core.TInteger)
	listOfNode := core.NewList([]core.Value{core.NewTypeLiteral(core.TInteger)})
	listOfInts := core.NewList([]core.Value{core.NewInteger(3)})
	m := core.NewMap(nil)
	cases := []struct {
		name   string
		d      emitDynBind
		writes bool
	}{
		{"not a root def", emitDynBind{root: false, srcSeq: 1, val: carrier}, false},
		{"bare type node, literal", emitDynBind{root: true, srcSeq: -1, val: core.NewTypeLiteral(core.TNone)}, false},
		{"bare type node, computed", emitDynBind{root: true, srcSeq: 1, val: core.NewTypeLiteral(core.TInteger)}, false},
		{"literal scalar", emitDynBind{root: true, srcSeq: -1, val: core.NewInteger(41)}, false},
		{"literal list", emitDynBind{root: true, srcSeq: -1, val: listOfInts}, false},
		{"literal carrier (a stripped literal)", emitDynBind{root: true, srcSeq: -1, val: carrier}, true},
		{"computed scalar fold", emitDynBind{root: true, srcSeq: 1, val: core.NewInteger(3)}, false},
		{"computed string fold", emitDynBind{root: true, srcSeq: 1, val: core.NewString("ab")}, false},
		{"computed carrier", emitDynBind{root: true, srcSeq: 1, val: carrier}, true},
		{"computed list of a type node — the twin-carrier class", emitDynBind{root: true, srcSeq: 1, val: listOfNode}, true},
		{"computed list, concrete to the bottom", emitDynBind{root: true, srcSeq: 1, val: listOfInts}, true},
		{"computed map — a module prototype's shape", emitDynBind{root: true, srcSeq: 1, val: m}, true},
	}
	for _, c := range cases {
		d := c.d
		if got := rootBindWritesBack(&d); got != c.writes {
			t.Errorf("%s: rootBindWritesBack = %v, want %v", c.name, got, c.writes)
		}
	}
}
