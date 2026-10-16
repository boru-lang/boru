package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestS6aGradualContagionZeroOutSingleValueSibling(t *testing.T) {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "s6aset1",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewInteger(1)}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	done := r.Check.Begin()
	defer done()
	out := applyGradualContagion(r, "s6aset1",
		[]core.Value{core.NewDynamicCarrier(core.TAny)}, nil, core.SrcPos{}, false)
	if len(out) != 1 || !core.IsDynamic(&out[0]) || !out[0].Parent().Equal(core.TInteger) {
		t.Errorf("0-out mutator over dynamic receiver should model one dynamic Integer: %+v", out)
	}
}

// --- tryFoldModuleConst -------------------------------------------------------
