package eng

import (
	"strings"
	"testing"

	check "github.com/boru-lang/boru/check/go"
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// Coverage for the mark/move interpreter machinery (engine.go: stepMark,
// stepMove, stepMoveCont, stepMoveIf, handleFlowCtrl, handleLoopBreak,
// handleLoopContinue, exitWithFlowCtrl) and the compiled loop pipeline
// (emit.go RecordLoop / ArmLoopCapture / ConsumeLoopArm /
// BeginLoopCarried / EndLoopCarried / Checkpoint, lower.go lowerLoop /
// lowerBreak / lowerContinue / allocLocal, vm.go opForSetup / OpForNext
// / flowSignal). A minimal `cfor` counted loop is registered from
// inside the kernel, mirroring lang's `for` (native_control.go /
// forloop.go): the runtime handler splices mark+body+moveCont tokens,
// the check-mode ReturnsFn analyses the body via AnalyseLoopBody and
// records the loop for the bytecode plan.

// cforRun is the runtime handler: the interpreter's counted loop via
// mark/body/moveCont splicing (the runForLoop shape).
func cforRun(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
	n, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, r.BoruError("for_error", "cfor: count must be a concrete Integer", "cfor")
	}
	if n <= 0 {
		return nil, nil
	}
	body := args[1]
	if !core.IsConcrete(body) {
		return nil, r.BoruError("for_error", "cfor: body must be a concrete list", "cfor")
	}
	lst, _ := core.AsList(body)
	bodySlice := lst.Slice()

	core.InstallDef(r, "i", core.NewInteger(0))
	bodyCopy := make([]core.Value, len(bodySlice))
	copy(bodyCopy, bodySlice)
	cont := &core.ForCont{Registry: r, IterName: "i", Current: 0, End: n, Step: 1, Body: bodyCopy}

	id := core.NextMarkID()
	tokens := make([]core.Value, 0, len(bodySlice)+2)
	tokens = append(tokens, core.NewMark(id, bodySlice...))
	iter := make([]core.Value, len(bodySlice))
	copy(iter, bodySlice)
	tokens = append(tokens, iter...)
	tokens = append(tokens, core.NewMoveCont(id, "cfor loop", cont))
	return tokens, nil
}

// cforReturns is the check-mode analysis + loop recording (the
// forCarrierAnalyse shape, counted form only).
func cforReturns(args []core.Value, r *core.Registry) []core.Value {
	es, _ := r.Check.Recorder().(*compiler.EmitState)
	body := args[len(args)-1]
	iter := core.NewCarrier(core.TInteger)
	cv := args[0]
	lowerable := core.IsConcrete(cv) && cv.Parent.ConformsTo(core.TInteger)
	var startV, endV, stepV core.Value
	if lowerable {
		startV, endV, stepV = core.NewInteger(0), cv, core.NewInteger(1)
		es.ArmLoopCapture()
	}
	stk := check.AnalyseLoopBody(r, body, []string{"i"}, []core.Value{iter}, false)
	out := core.NewCarrier(core.TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		if core.IsDisjunct(top) {
			out = core.NewCarrierTypedListValue(top)
		} else {
			out = core.NewCarrierTypedList(top.Parent)
		}
	}
	if lowerable {
		frag := es.TakeFragment()
		es.RecordLoop(startV, endV, stepV, frag, stk, iter.ID, "i", out, 0, args[0].Pos())
	}
	return []core.Value{out}
}

// registerLoopWords adds `cfor`, `break`, and `continue` to a
// covRegistry. break/continue mirror the lang registrations exactly:
// runtime handlers raise the FlowCtrl signal; the recorder's call
// classifier turns the check-mode dispatch into evBreak/evContinue by
// NAME (the words must be called break/continue for that).
func registerLoopWords(r *core.Registry) {
	registerBranchWords(r)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cfor",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TInteger, core.TList},
			NoEvalArgs: map[int]bool{1: true},
			Impl:       core.Go(cforRun),
			ReturnsFn:  cforReturns,
			BarrierPos: -1,
		}},
	})
	for name, sig := range map[string]core.FlowCtrl{"break": core.FlowBreak, "continue": core.FlowContinue} {
		ctrl := sig
		r.RegisterNativeFunc(core.NativeFunc{
			Name: name,
			Signatures: []core.Signature{{
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
					r.FlowCtrl = ctrl
					return nil, nil
				}),
				Returns: []*core.Type{}, BarrierPos: 0,
			}},
		})
	}
}

// --- interpreter loop paths ------------------------------------------------

func TestInterpretedForLoop(t *testing.T) {
	r := covRegistry(t, registerLoopWords)
	out, err := core.NewTop(r).Run([]core.Value{
		core.NewWord("cfor"), core.NewInteger(3),
		codeBody(core.NewWord("cadd"), core.NewWord("i"), core.NewInteger(10)),
	})
	if err != nil {
		t.Fatalf("cfor: %v", err)
	}
	if got := renderAll(out); got != "10 | 11 | 12" {
		t.Errorf("cfor results = %q", got)
	}
	// Zero-count loop runs zero times.
	out, err = core.NewTop(r).Run([]core.Value{core.NewWord("cfor"), core.NewInteger(0), codeBody(core.NewInteger(1))})
	if err != nil || len(out) != 0 {
		t.Errorf("cfor 0: out=%v err=%v", out, err)
	}
}

func TestInterpretedLoopBreakAndContinue(t *testing.T) {
	r := covRegistry(t, registerLoopWords)
	// break at i>2: only 0 1 2 accumulate.
	out, err := core.NewTop(r).Run([]core.Value{
		core.NewWord("cfor"), core.NewInteger(5),
		codeBody(
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewWord("i"), core.NewInteger(2), core.NewCloseParen(),
			codeBody(core.NewWord("break")),
			codeBody(core.NewWord("i")),
		),
	})
	if err != nil {
		t.Fatalf("break loop: %v", err)
	}
	if got := renderAll(out); got != "0 | 1 | 2" {
		t.Errorf("break loop = %q", got)
	}

	// continue at i>2: partial results of later iterations are discarded.
	out, err = core.NewTop(r).Run([]core.Value{
		core.NewWord("cfor"), core.NewInteger(5),
		codeBody(
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewWord("i"), core.NewInteger(2), core.NewCloseParen(),
			codeBody(core.NewWord("continue")),
			codeBody(core.NewWord("i")),
		),
	})
	if err != nil {
		t.Fatalf("continue loop: %v", err)
	}
	if got := renderAll(out); got != "0 | 1 | 2" {
		t.Errorf("continue loop = %q", got)
	}
}

func TestBreakOutsideLoopErrors(t *testing.T) {
	r := covRegistry(t, registerLoopWords)
	_, err := core.NewTop(r).Run([]core.Value{core.NewWord("break")})
	if err == nil || !strings.Contains(err.Error(), "outside loop") {
		t.Errorf("bare break: err = %v", err)
	}
	_, err = core.NewTop(r).Run([]core.Value{core.NewWord("continue")})
	if err == nil || !strings.Contains(err.Error(), "outside loop") {
		t.Errorf("bare continue: err = %v", err)
	}
}

func TestMarkMoveReplay(t *testing.T) {
	r := covRegistry(t, nil)
	// A plain move replays the mark's saved body once (both mark and
	// move are consumed by the jump).
	id := core.NextMarkID()
	out, err := core.NewTop(r).Run([]core.Value{
		core.NewMark(id, core.NewInteger(7)),
		core.NewInteger(7),
		core.NewMove(id, "wave3 replay"),
	})
	if err != nil {
		t.Fatalf("mark/move replay: %v", err)
	}
	if got := renderAll(out); got != "7" {
		t.Errorf("replayed = %q", got)
	}
}

func TestMoveIfSplicesBranches(t *testing.T) {
	r := covRegistry(t, nil)
	run := func(cond core.Value) string {
		t.Helper()
		id := core.NextMarkID()
		out, err := core.NewTop(r).Run([]core.Value{
			core.NewMark(id),
			cond,
			core.NewMoveIf(id, "wave3 if", &core.IfCont{
				Then: []core.Value{core.NewInteger(1)},
				Else: []core.Value{core.NewInteger(2)},
			}),
		})
		if err != nil {
			t.Fatalf("moveIf: %v", err)
		}
		return renderAll(out)
	}
	if got := run(core.NewBoolean(true)); got != "1" {
		t.Errorf("then branch = %q", got)
	}
	if got := run(core.NewBoolean(false)); got != "2" {
		t.Errorf("else branch = %q", got)
	}

	// Condition produced no value → runtime_error.
	id := core.NextMarkID()
	_, err := core.NewTop(r).Run([]core.Value{
		core.NewMark(id),
		core.NewMoveIf(id, "wave3 empty if", &core.IfCont{Then: []core.Value{core.NewInteger(1)}}),
	})
	if err == nil || !strings.Contains(err.Error(), "condition produced no value") {
		t.Errorf("empty condition: err = %v", err)
	}
}

// --- compiled loop pipeline -------------------------------------------------

func TestCompiledCountedLoop(t *testing.T) {
	got := runDifferential(t, registerLoopWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cfor"), core.NewInteger(3),
			codeBody(core.NewWord("cadd"), core.NewWord("i"), core.NewInteger(10)),
		}
	})
	if got != "10 | 11 | 12" {
		t.Errorf("compiled loop = %q", got)
	}
}

func TestCompiledLoopBreak(t *testing.T) {
	got := runDifferential(t, registerLoopWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cfor"), core.NewInteger(5),
			codeBody(
				core.NewWord("cif"),
				core.NewOpenParen(), core.NewWord("cgt"), core.NewWord("i"), core.NewInteger(2), core.NewCloseParen(),
				codeBody(core.NewWord("break")),
				codeBody(core.NewWord("i")),
			),
		}
	})
	if got != "0 | 1 | 2" {
		t.Errorf("compiled break loop = %q", got)
	}
}

func TestCompiledLoopContinue(t *testing.T) {
	got := runDifferential(t, registerLoopWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cfor"), core.NewInteger(5),
			codeBody(
				core.NewWord("cif"),
				core.NewOpenParen(), core.NewWord("cgt"), core.NewWord("i"), core.NewInteger(2), core.NewCloseParen(),
				codeBody(core.NewWord("continue")),
				codeBody(core.NewWord("i")),
			),
		}
	})
	if got != "0 | 1 | 2" {
		t.Errorf("compiled continue loop = %q", got)
	}
}

func TestCompiledLoopFeedsIterator(t *testing.T) {
	// The loop result is variadic — only the program residual may
	// absorb it; the iterator local drives OpPushLocal per iteration.
	got := runDifferential(t, registerLoopWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cfor"), core.NewInteger(4),
			codeBody(core.NewWord("cmul"), core.NewWord("i"), core.NewWord("i")),
		}
	})
	if got != "0 | 1 | 4 | 9" {
		t.Errorf("compiled square loop = %q", got)
	}
}

func TestCompiledLoopBranchValueArms(t *testing.T) {
	// A computed condition over the iterator with plain value arms.
	got := runDifferential(t, registerLoopWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cfor"), core.NewInteger(5),
			codeBody(
				core.NewWord("cif"),
				core.NewOpenParen(), core.NewWord("cgt"), core.NewWord("i"), core.NewInteger(2), core.NewCloseParen(),
				core.NewInteger(1),
				core.NewInteger(0),
			),
		}
	})
	if got != "0 | 0 | 0 | 1 | 1" {
		t.Errorf("compiled guard loop = %q", got)
	}
}

func TestCompiledLoopSideEffectBody(t *testing.T) {
	// A body netting zero values per iteration is a side-effect loop
	// (zeroOut); a following literal is the only program result.
	setup := func(r *core.Registry) {
		registerLoopWords(r)
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "csink",
			Signatures: []core.Signature{{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return nil, nil
				}),
				Returns: []*core.Type{}, BarrierPos: -1,
			}},
		})
	}
	got := runDifferential(t, setup, func() []core.Value {
		return []core.Value{
			core.NewWord("cfor"), core.NewInteger(3),
			codeBody(core.NewWord("csink"), core.NewWord("i")),
			core.NewEnd(),
			core.NewInteger(42),
		}
	})
	if got != "42" {
		t.Errorf("side-effect loop = %q", got)
	}
}

func TestNestedLoopsInterpretedAndFailedToCompile(t *testing.T) {
	// The interpreter runs nested mark/move loops; the Stage-2 compiler
	// declines a loop whose result is another loop's body result — the
	// compile failure reason is pinned so a silent miscompile can't replace it.
	tokens := func() []core.Value {
		return []core.Value{
			core.NewWord("cfor"), core.NewInteger(2),
			codeBody(
				core.NewWord("cfor"), core.NewInteger(2),
				codeBody(core.NewWord("cadd"), core.NewWord("i"), core.NewInteger(100)),
			),
		}
	}
	ri := covRegistry(t, registerLoopWords)
	out, err := core.NewTop(ri).Run(tokens())
	if err != nil {
		t.Fatalf("nested loops interpreted: %v", err)
	}
	if got := renderAll(out); got != "100 | 101 | 100 | 101" {
		t.Errorf("nested loops = %q", got)
	}
	rc := covRegistry(t, registerLoopWords)
	prog, reason := compileTokens(t, rc, tokens())
	if prog != nil {
		// If a future stage compiles this, it must agree with the interpreter.
		cOut, cErr := RunProgram(prog, rc)
		if cErr != nil || renderAll(cOut) != "100 | 101 | 100 | 101" {
			t.Errorf("compiled nested loops diverge: %v %v", cOut, cErr)
		}
	} else if !strings.Contains(reason, "loop results as a branch/body result") {
		t.Errorf("unexpected compile failure reason: %s", reason)
	}
}

func TestLoopSignatureErrorMentionsShape(t *testing.T) {
	// A dispatch that cannot match (type literal in a concrete slot)
	// raises the interpreter's detailed signature_error, including the
	// expected-signature summary and nearby stack types.
	r := covRegistry(t, registerLoopWords)
	_, err := core.NewTop(r).Run([]core.Value{core.NewWord("cfor"), core.NewTypeLiteral(core.TInteger), codeBody(core.NewInteger(1))})
	if err == nil || !strings.Contains(err.Error(), "signature_error") {
		t.Fatalf("literal count: err = %v", err)
	}
	if !strings.Contains(err.Error(), "cfor (Integer, List)") {
		t.Errorf("error does not summarise the expected signature: %v", err)
	}
}

// --- emit checkpoint/rollback ------------------------------------------------
