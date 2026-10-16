package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// stampFnConst's container walk must survive a map const with no backing
// OrderedMap. A MapPayload's M is a pointer, so "an empty map" and "no map at
// all" are different values, and the walk reaches consts it did not build —
// freshenableConst carries the same guard for the same reason. Without it the
// key loop nil-derefs on a shape the compile pass would simply skip.
func TestStampFnConstWalksNilMapPayload(t *testing.T) {
	es := NewEmitState()
	// No registry bound: the walk must not reach a stamp at all here, which is
	// the point — it returns at the nil map.
	es.stampFnConstAt(core.ValueOf(&core.Datum{Parent: core.TMap, Data: core.MapPayload{}}), 0)

	// The list twin, for symmetry: an empty element slice walks to completion.
	es.stampFnConstAt(core.ValueOf(&core.Datum{Parent: core.TList, Data: core.ListPayload{}}), 0)

	// And the depth bound holds: a walk that started past it stops immediately
	// rather than recursing on whatever it was handed.
	es.stampFnConstAt(core.ValueOf(&core.Datum{Parent: core.TList, Data: core.ListPayload{
		Elems: []core.Value{core.NewInteger(1)},
	}}), stampFnConstDepth+1)
}
