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
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(core.TInteger) {
		t.Errorf("0-out mutator over dynamic receiver should model one dynamic Integer: %+v", out)
	}
}

// --- tryFoldModuleConst -------------------------------------------------------

// TestInactiveDispatchBraid pins the compiler-less defaults behind the
// S3 slot table: each is the exact decline an unarmed recording pass
// produces. The live table is compiler-installed at init
// (installDispatchBraid); these bodies are the post-cut check-only
// configuration.
func TestInactiveDispatchBraid(t *testing.T) {
	inactiveDispatchRecordOutcome(nil, "", nil, nil, nil, core.SrcPos{}, nil)
	if v, ok := inactiveDispatchTryFoldScalarConst(nil, nil, nil); ok || core.IsConcrete(v) {
		t.Fatal("inactive scalar fold must decline")
	}
	if inactiveDispatchTryRecordPoly(nil, "", nil, nil, nil, core.SrcPos{}, false, nil, false, nil) {
		t.Fatal("inactive poly record must decline")
	}
	if inactiveDispatchTryRecordDynBody(nil, "", nil, nil, nil, core.SrcPos{}) {
		t.Fatal("inactive dyn-body record must decline")
	}
	if inactiveDispatchCompileUserPolyArms(nil, core.TheInactiveEmit, "", nil, nil) != nil {
		t.Fatal("inactive poly-arm compile must decline")
	}
	if p, barred := inactiveDispatchPlanUserPoly(nil, core.TheInactiveEmit, "", nil, nil); p != nil || barred {
		t.Fatal("inactive poly plan must decline")
	}
}
