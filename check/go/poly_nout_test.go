package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// PolyNOutAmbiguous: a poly re-match over a gradual operand may land on an
// overload whose DECLARED result count differs from the committed arm's (set
// over a Store returns nothing, over a Map the container), and the recorder
// then claims no count. Arms typed by a ReturnsFn alone do not count; a
// strict operand reaches only the arms that admit it; an unknown word is
// never ambiguous.
func TestPolyNOutAmbiguous(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "setq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TString, core.TAny, core.TMap}, Returns: []*core.Type{core.TMap}, BarrierPos: 3, Impl: ovImpl()},
			{Args: []*core.Type{core.TString, core.TAny, core.TStore}, Returns: []*core.Type{}, BarrierPos: 3, Impl: ovImpl()},
			{Args: []*core.Type{core.TString, core.TAny, core.TList}, BarrierPos: 3, Impl: ovImpl()}, // a ReturnsFn-only arm: no declared count
		},
	})
	key, val := core.NewCarrier(core.TString), core.NewCarrier(core.TInteger)
	gradual := []core.Value{key, val, core.NewDynamicCarrier(core.TAny)}
	if !PolyNOutAmbiguous(r, "setq", gradual, 1) {
		t.Error("a gradual receiver reaches the Store arm (0 results) beside the committed Map arm (1): ambiguous")
	}
	if !PolyNOutAmbiguous(r, "setq", gradual, 0) {
		t.Error("…and the Map arm beside a committed Store arm")
	}
	strictMap := []core.Value{key, val, core.NewCarrier(core.TMap)}
	if PolyNOutAmbiguous(r, "setq", strictMap, 1) {
		t.Error("a Map receiver reaches the Map arm alone: not ambiguous")
	}
	if PolyNOutAmbiguous(r, "setq", []core.Value{key, val, core.NewCarrier(core.TList)}, 7) {
		t.Error("an arm with no declared Returns does not count against the claim")
	}
	if PolyNOutAmbiguous(r, "no-such-q", gradual, 1) {
		t.Error("an unknown word is never ambiguous")
	}
	// The shared reachability test: arity first, then every slot.
	s := &r.Lookup("setq").Signatures[0]
	if dynamicOverloadReachable(s, gradual[:2]) {
		t.Error("a window of the wrong arity reaches nothing")
	}
	if !dynamicOverloadReachable(s, gradual) || dynamicOverloadReachable(s, []core.Value{key, val, core.NewCarrier(core.TInteger)}) {
		t.Error("a gradual receiver reaches the Map arm; an Integer receiver does not")
	}
}
