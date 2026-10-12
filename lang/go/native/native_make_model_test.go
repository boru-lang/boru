package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The check-mode make model executes Ideal.Instantiate over concrete
// data ONLY for kinds that declare PureInstantiate (PR #346 review):
// the Ideal contract puts no purity requirement on Instantiate in
// general, so a host-registered constructor — one that may touch
// services, mutate registry state, or be nondeterministic — must never
// run at analysis time. Both directions of the gate are pinned here
// with the same claiming kind, differing only in the marker.

func scalarClaimingIdeal(name string, called *bool, pure bool) *core.Ideal {
	return &core.Ideal{
		Name:    name,
		Enabled: true,
		Accepts: func(base core.Value) bool {
			// A bare type node IS its lattice node (NewTypeLiteral
			// returns *t), so claim the target by node identity.
			n := core.TypeNodeOf(base)
			return core.IsBareTypeNode(&base) && n != nil && n.Equal(core.TScalar)
		},
		Instantiate: func(_, data core.Value, _ *core.Registry) ([]core.Value, error) {
			*called = true
			return []core.Value{data}, nil
		},
		PureInstantiate: pure,
	}
}

func TestMakeScalarModelPurityGate(t *testing.T) {
	rf := makeScalarReturns()
	target := core.NewTypeLiteral(core.TScalar)

	// UNMARKED kind: the model must not execute it — the residual is
	// the fresh instance carrier, exactly as if no Ideal claimed the
	// target.
	r := mirrorReg(t)
	called := false
	r.Ideals.Register(scalarClaimingIdeal("HostScalarKind", &called, false))
	if id := r.Ideals.For(target); id == nil || id.Name != "HostScalarKind" {
		t.Fatalf("fixture kind did not claim the bare Scalar target (got %v)", id)
	}
	out := rf([]core.Value{target, core.NewString("x")}, r)
	if called {
		t.Fatal("unmarked Instantiate executed at analysis time")
	}
	if len(out) != 1 || core.IsConcrete(&out[0]) {
		t.Fatalf("unmarked kind's residual = %v, want one fresh carrier", out)
	}

	// The SAME kind marked PureInstantiate: the model runs it and
	// surfaces the concrete result.
	r2 := mirrorReg(t)
	called2 := false
	r2.Ideals.Register(scalarClaimingIdeal("PureScalarKind", &called2, true))
	out2 := rf([]core.Value{target, core.NewString("x")}, r2)
	if !called2 {
		t.Fatal("marked Instantiate did not run at analysis time")
	}
	if len(out2) != 1 || !core.IsConcrete(&out2[0]) {
		t.Fatalf("marked kind's residual = %v, want the concrete construction", out2)
	}
	if s, err := out2[0].AsConcreteString(); err != nil || s != "x" {
		t.Fatalf("surfaced construction = (%q, %v), want the constructor's value", s, err)
	}
}
