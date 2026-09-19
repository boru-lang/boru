package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// fnValueClosureProg is a program whose unit 0 is a fn VALUE's — a one-param
// `[n:Integer]` lambda body answering its argument — and whose unit 1 is a
// callback body unit with no contract.
func fnValueClosureProg() *compiler.Program {
	return &compiler.Program{
		Fns: []compiler.CompiledFn{{
			Name: "fnval$body", Lambda: true, NArgs: 1, NParams: 1, NLocals: 1,
			Params: []*core.Type{core.TInteger},
			Code:   []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug:  []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}, {
			Name: "each$body", NArgs: 1, NParams: 1, NLocals: 1,
			Code:  []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug: []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
}

// TestFnValueClosureSeamMatchesTopDown pins the token seam's fn-value
// closure arm (S1b-2): a fn-VALUE closure minted by another program is
// matched against its own signature and hosted over the matched args; a
// closure that matches nothing goes to the stepping path, where it stays
// data; a quoted closure, a callback body unit's closure and a seam-matched
// (SigMatched) closure are not the arm's.
func TestFnValueClosureSeamMatchesTopDown(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	fnv := compiler.NewClosure(fnValueClosureProg(), 0, nil)
	cl := fnv.Data.(core.ClosurePayload)

	res, err, ran := vc.invokeFnValueClosure(r, fnv, cl, []core.Value{core.NewInteger(5)})
	if !ran || err != nil || len(res) != 1 {
		t.Fatalf("a matching fn-value closure runs its unit: ran=%v err=%v res=%v", ran, err, res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 5 {
		t.Errorf("the unit answers its argument, got %v", res[0])
	}

	res, err, ran = vc.invokeFnValueClosure(r, fnv, cl, []core.Value{core.NewString("s")})
	if !ran || err != nil {
		t.Fatalf("a no-match declines to the stepping path: ran=%v err=%v", ran, err)
	}
	if len(res) != 2 || !compiler.IsCompiledClosure(res[1]) {
		t.Errorf("the stepped closure stays data over its input: %v", res)
	}

	quoted := fnv
	quoted.Quoted = true
	if _, _, ran := vc.invokeFnValueClosure(r, quoted, cl, nil); ran {
		t.Error("a quoted closure is data, not a callee")
	}
	body := compiler.NewClosure(fnValueClosureProg(), 1, nil)
	if _, _, ran := vc.invokeFnValueClosure(r, body, body.Data.(core.ClosurePayload), nil); ran {
		t.Error("a callback body unit's closure is not the arm's")
	}

	// Through the invoker: the same closure marked SigMatched skips the
	// arm and applies positionally.
	res, err = vc.invokeClosureOn(r, core.ClosureSigMatched(fnv), []core.Value{core.NewInteger(9)})
	if err != nil || len(res) != 1 {
		t.Fatalf("a seam-matched closure applies positionally: err=%v res=%v", err, res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 9 {
		t.Errorf("positional apply answers 9, got %v", res[0])
	}
}
