package eng

import (
	"strconv"
	"strings"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_token_body.go — S3's first slice (2026-09-24): a TOKEN body reaching the
// InvokeBody seam at run time runs as a compiled unit.
//
// A native's body operand that the program could not lower — a quoted list
// read from a flex (`each bodies.inc xs`), returned by a fn (`each (mkb) xs`,
// `do (mk 0)`), selected by a branch, passed as a List param (`def f fn
// [[b:List][List][each b xs]]`) — reaches invokeClosureOn as a raw list, and
// the seam stepped it on a pooled sub-engine once per application
// (core.RunResolved): the interp-entry census's code-bodies.tsv, twenty-one
// rows, every one `Engine.Run 1, RunResolved 1`. The body exists only at run
// time, so it is compiled at run time — the detached stamp every fn value
// already takes at its first application (compiler.StampTokenBody over
// StampDetachedSig: a synthetic anonymous fn whose one signature takes the
// seam's inputs as unnamed Any params over the tokens) — and hosted nested,
// as a foreign closure is (applyClosure's foreign arm), with the token seam's
// count-agnostic residual (rootRetTrim false: the unit's whole frame comes
// back, unconsumed inputs beneath the results, exactly RunResolved's stack).
//
// The synthetic signature's params are TYPED by the runtime inputs (each
// input's concrete Parent), not Any: a body that leaves an input in its
// residual — fold's accumulator under `[add 1]` — would otherwise leave an
// Any the checker cannot rule out as a fn value and decline ("unapplied
// fn-value in body residual"), and a typed input dispatches natively where
// a gradual one poly-matches at run time. The unit is therefore memoised on
// the CALLING registry by the body, the input count AND the input types
// (core.Registry.TokenBodyStamp): the same list applied per element over
// like elements compiles once, a heterogeneous collection compiles once per
// type shape it meets, and a body that declines is remembered as declined
// so it pays the compile once. The body's key is its tokens rendered with
// their positions when every token is identity-free, else its value ID
// (tokenBodyKey): a list stored in a flex or built by `push` at run time
// carries no ID (the mode-gated ID elision), a fn returning a quoted
// literal hands out a fresh clone per call, and two bodies of the same
// text at the same positions are the same program text, so one unit
// answers both, error positions included.
// Freshness is the detached ref's own — DepSnap against the registry's live
// bindings, the JIT re-stamp bounded by its try budget — so a free word
// rebound between applications recompiles, and one rebound past the budget
// takes the interpreter, which resolves it live.
//
// What stays the interpreter's, each byte-identical to before: an empty
// body, a body with a replay hazard or a flow sentinel, a body that
// declines to compile, and a stale ref past its budget.

// tokenBodyDeclined marks a body whose stamp declined, so the next
// application takes the interpreter without paying the compile again.
type tokenBodyDeclined struct{}

// flowEscape is the error a hosted token body's run returns when a
// break/continue found no loop in the hosted frames (vmContext.flowEscapes):
// the signal belongs to the ENCLOSING run, and the seam hands it over as the
// interpreter's sub-engine does — the registry's FlowCtrl set, no values,
// no error (Engine.exitWithFlowCtrl's sub-engine contract), for the VM to
// translate after the native returns (escapedFlow). `for 5 [do b i]` with
// `b` a computed body whose fn breaks ends the `for`, on both lanes.
type flowEscape struct{ op compiler.Opcode }

func (e *flowEscape) Error() string { return "flow signal escaping a hosted token body" }

// flowCtrlOf maps the escaping opcode to the registry's flag.
func flowCtrlOf(op compiler.Opcode) core.FlowCtrl {
	if op == compiler.OpFlowContinue {
		return core.FlowContinue
	}
	return core.FlowBreak
}

// tokenBodyKey names one body under one input shape, and reports ok=false
// for a body it cannot name: the body's own name (core.TokenBodyKey — its
// tokens rendered with their positions when every token is identity-free,
// else its value ID; a reference-bearing body with no ID is not named at
// all and the seam keeps the interpreter) closed by this seam's shape, the
// input count and each input's type.
func tokenBodyKey(body core.Value, tokens, inputs []core.Value) (string, bool) {
	base, ok := core.TokenBodyKey(body, tokens)
	if !ok {
		return "", false
	}
	var b strings.Builder
	b.WriteString(base)
	b.WriteByte('/')
	b.WriteString(strconv.Itoa(len(inputs)))
	for _, in := range inputs {
		b.WriteByte(',')
		b.WriteString(core.TokenBodyInputType(in).Name())
	}
	return b.String(), true
}

// invokeTokenBody runs body — a raw token list at the InvokeBody seam — as a
// run-time-stamped unit over inputs (stack order), reporting ran=false when
// the seam keeps its interpreter path.
func (vc *vmContext) invokeTokenBody(reg *core.Registry, body core.Value, inputs []core.Value) ([]core.Value, error, bool) {
	lst, err := core.AsList(body)
	if err != nil || lst.IsNil() || len(lst.Slice()) == 0 {
		return nil, nil, false
	}
	tokens := lst.Slice()
	key, named := tokenBodyKey(body, tokens, inputs)
	if !named {
		return nil, nil, false
	}
	var ref *compiler.CompiledFnRef
	if slot, seen := reg.TokenBodyStamp(key); seen {
		r, isRef := slot.(*compiler.CompiledFnRef)
		if !isRef {
			return nil, nil, false // declined before: the interpreter's, remembered
		}
		ref = r
	} else {
		types := make([]*core.Type, len(inputs))
		for i, in := range inputs {
			types[i] = core.TokenBodyInputType(in)
		}
		r, ok := compiler.StampTokenBody(reg, tokens, types, body.Pos())
		if !ok {
			reg.SetTokenBodyStamp(key, tokenBodyDeclined{})
			return nil, nil, false
		}
		reg.SetTokenBodyStamp(key, r)
		ref = r
	}
	if !ref.DepsFresh(reg) {
		// A dependency rebound since the stamp: the JIT re-stamp against the
		// live bindings, bounded by the ref's try budget; past it, the
		// interpreter resolves the binding live. The cache keeps the FIRST
		// ref — its box carries the current twin and the tries spent — as
		// the callback seam keeps the sig's (InvokeCompiled); caching the
		// twin would hand every rebind a fresh budget, a compile per
		// application for a dependency that keeps moving.
		if ref = ref.JitRestamp(reg); ref == nil {
			return nil, nil, false
		}
	}
	if ref.Prog == nil || ref.Unit < 0 || ref.Unit >= len(ref.Prog.Fns) { //covergate:allow compiler/VM defensive arm; StampDetachedSig and JitRestamp return only finalized one-unit programs (§compiler)
		return nil, nil, false
	}
	// The token seam's discipline, as applyClosure's foreign arm: the
	// hosted root RET hands back the whole frame.
	prev := vc.rootRetTrim
	vc.rootRetTrim = false
	defer func() { vc.rootRetTrim = prev }()
	// The body's own frame: a DynEnv unit reads `args` as the interpreter's
	// sub-run would (pushRootArgs).
	defer pushRootArgs(reg, ref.Prog, inputs)()
	res, err := vc.hostForeign(ref.Prog, reg, ref.Unit, inputs, nil, true)
	if fe, escaped := err.(*flowEscape); escaped {
		reg.FlowCtrl = flowCtrlOf(fe.op)
		return nil, nil, true
	}
	return res, err, true
}
