package eng

import (
	"errors"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The VM-side word-policy gate (plan Phase 10): every NAMED compiled
// dispatch — CALL_NATIVE, CALL_USER/TAIL_CALL_USER, CALL_NATIVE_POLY,
// CALL_USER_POLY, CALL_DYN_METHOD — consults the SAME WordChecker the
// interpreter's policyGateWord runs, so a denied word raises the identical
// error on either engine. Programs are hand-built (the seam7 pattern):
// production compiles still decline policy-gated registries, so the gate is
// reached by installing the checker AFTER construction.

var errZzDenied = errors.New("zz-policy: word denied")

// denyChecker denies exactly one word.
type denyChecker struct{ word string }

func (d denyChecker) CheckWord(name string) error {
	if name == d.word {
		return errZzDenied
	}
	return nil
}

func gateReg(t *testing.T, deny string) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	if err := r.Capabilities.Set(core.CapPolicy, denyChecker{word: deny}); err != nil {
		t.Fatalf("install checker: %v", err)
	}
	return r
}

func TestVMWordPolicyGateArms(t *testing.T) {
	sig := core.Signature{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return []core.Value{args[0]}, nil
		})}
	one := core.NewInteger(1)

	cases := []struct {
		name string
		p    *compiler.Program
	}{
		{"call-native", &compiler.Program{
			Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNative, Arg: 0}},
			Consts: []core.Value{one}, Sigs: []compiler.SigRef{{Word: "zz-gated", Sig: &sig}},
		}},
		{"call-user", &compiler.Program{
			Code: []compiler.Instr{{Op: compiler.OpCallUser, Arg: 0}},
			Fns:  []compiler.CompiledFn{{Name: "zz-gated", NParams: 0}},
		}},
		{"call-native-poly", &compiler.Program{
			Code:     []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNativePoly, Arg: 0}},
			Consts:   []core.Value{one},
			PolyRefs: []compiler.PolyRef{{Word: "zz-gated", Arity: 1}},
		}},
		{"call-user-poly", &compiler.Program{
			Code:      []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallUserPoly, Arg: 0}},
			Consts:    []core.Value{one},
			UserPolys: []compiler.UserPolyRef{{Word: "zz-gated", Arity: 1}},
		}},
		{"call-dyn-method", &compiler.Program{
			Code:       []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallDynMethod, Arg: 0}},
			Consts:     []core.Value{one},
			DynMethods: []compiler.DynMethodSpec{{Word: "zz-gated", NArgs: 1, NOut: 1}},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := RunProgram(c.p, gateReg(t, "zz-gated"))
			if err == nil || !strings.Contains(err.Error(), "zz-policy: word denied") {
				t.Fatalf("denied word must raise the checker's error, got %v", err)
			}
			// The negative twin: the same program under a checker denying a
			// DIFFERENT word passes the gate (it may then fail later for
			// structural reasons — the gate must be what stopped it above).
			_, err2 := RunProgram(c.p, gateReg(t, "zz-other"))
			if err2 != nil && strings.Contains(err2.Error(), "zz-policy") {
				t.Fatalf("non-denied word must pass the gate, got %v", err2)
			}
		})
	}

	// Internal markers are exempt, exactly as in policyGateWord.
	marker := &compiler.Program{
		Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNative, Arg: 0}},
		Consts: []core.Value{one}, Sigs: []compiler.SigRef{{Word: "__zz", Sig: &sig}},
	}
	if _, err := RunProgram(marker, gateReg(t, "__zz")); err != nil && strings.Contains(err.Error(), "zz-policy") {
		t.Fatalf("internal marker must bypass the gate, got %v", err)
	}
}
