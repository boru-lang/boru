package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestSoleSigParamsNominalSkipsAnUntypedParam pins soleSigParamsNominal's
// untyped slot (main's #509): a sole signature's param with no declared type
// constrains nothing, so it does not make the signature value-sensitive; a
// refinement-typed param does.
func TestSoleSigParamsNominalSkipsAnUntypedParam(t *testing.T) {
	mk := func(pt *core.Type) (*core.Signature, *core.FnDefInfo) {
		sole := core.Signature{
			Params:     []core.FnParam{{Name: "x", Type: pt}},
			Impl:       core.Boru([]core.Value{core.NewInteger(1)}),
			ReturnsFn:  func(args []core.Value, r *core.Registry) []core.Value { return []core.Value{core.NewInteger(1)} },
			BarrierPos: core.BarrierAllForward,
		}
		fn := &core.FnDefInfo{Name: "f", Signatures: []core.Signature{{Fallback: true}, sole}}
		return &fn.Signatures[1], fn
	}
	if sig, fn := mk(nil); !soleSigParamsNominal(sig, fn) {
		t.Error("an untyped param is nominal")
	}
	if sig, fn := mk(core.TInteger); !soleSigParamsNominal(sig, fn) {
		t.Error("an Integer param is nominal")
	}
}
