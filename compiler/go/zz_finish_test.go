package compiler

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

func (c *countingRecorder) Active() bool {
	c.activeCalls++
	return c.EmitRecorder.Active()
}

func (c *countingRecorder) Armed() bool {
	c.armedCalls++
	return c.EmitRecorder.Armed()
}

func (c *countingRecorder) RecordCall(word string, sig *core.Signature, args, outs []core.Value, pos core.SrcPos, forceDynOut, quoteInertOK bool) {
	c.recordCalls++
	c.EmitRecorder.RecordCall(word, sig, args, outs, pos, forceDynOut, quoteInertOK)
}

func TestS6aRecordTypedDefMakeGuards(t *testing.T) {
	if _, ok := core.RecordTypedDefMake(nil, "", core.Value{}, core.Value{}, core.SrcPos{}); ok {
		t.Error("nil registry must decline")
	}
	r := newTestRegistry(t)
	armEmit(r)
	// eng registries have no `make` word: objectMakeSig returns nil.
	if _, ok := core.RecordTypedDefMake(r, "", core.NewTypeLiteral(core.TInteger), core.NewMap(core.NewOrderedMap()), core.SrcPos{}); ok {
		t.Error("a registry without `make` must decline")
	}
}

func TestW8ExecFnDefStackMatchCheckFnValueUnnamed(t *testing.T) {
	// A non-anonymous body-bearing fn value with UNNAMED params in compile
	// mode routes through check.SpliceFnValueCheckResult (unnamed branch).
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	fnDef := core.FnDefInfo{Name: "w8fv", Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TInteger}}, Returns: []*core.Type{core.TInteger},
		Impl: core.Boru(parenBody(core.NewInteger(42))), BarrierPos: 0,
	}}}
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(3), core.NewFunction(fnDef)}, core.StackHeadroom)
	e.Pointer = 1
	if err := e.ExecFnDefSigStackMatch(1, fnDef, []core.Value{core.NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

// registerBranchWords adds `cif` (3-arg conditional) and `cgt`
// (integer greater-than) to a covRegistry.
func registerBranchWords(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cif",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny, core.TAny, core.TAny},
			NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				condVal := args[0]
				// A single-element list condition ([true] — the form
				// LiteralCondValue folds statically) unwraps at runtime.
				if lst, err := core.AsList(condVal); err == nil && lst.Len() == 1 {
					condVal = lst.Get(0)
				}
				cond, _ := core.AsBoolean(condVal)
				if cond {
					return cifSplice(args[1]), nil
				}
				return cifSplice(args[2]), nil
			}),
			ReturnsFn: cifReturns, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cgt",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				// Convention: args[1] OP args[0] for natural forward reading.
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				return []core.Value{core.NewBoolean(a > b)}, nil
			}),
			Returns: []*core.Type{core.TBoolean}, BarrierPos: -1,
		}},
	})
}

func TestS6aAnalyseFnBodyInflightBailArmedRecorder(t *testing.T) {
	r := newTestRegistry(t)
	es := armEmit(r)
	es.fnArm = true // keep recording live through the fn-body guard
	body := []core.Value{core.NewWord("s6a_rec_body")}
	key := check.FnAnalysisKey(r.AnalysisScopeID(), "s6arec", nil, nil, body)
	r.Check.FnInflight = map[string]bool{key: true}
	out := check.AnalyseFnBody(r, "s6arec", nil, body, nil, nil, nil, false)
	if len(out) != 1 || !out[0].Carrier || !out[0].Parent.Equal(core.TAny) || out[0].Dynamic {
		t.Errorf("armed in-flight bail must be a strict Any carrier, got %+v", out)
	}
}

// --- isDeferredWordList ------------------------------------------------------------------------
