// Package stackform defines a canonical strict-stack representation
// of a boru program. It is the form the property-based-testing
// reducer operates on (design/legacy/PBT-PLAN.10.ignore) and the first half of
// the work the bytecode-emission proposal in
// design/legacy/boru-bytecode-report.0.ignore needs.
//
// A StackForm is a flat sequence of Ops. Each Op corresponds to one
// observable engine action — either pushing a literal value onto
// the stack, dispatching a word (handler invocation), or pushing a
// quoted sub-program for later evaluation. Surface syntax (forward
// argument collection, dotted-access expansion, paren groupings)
// has been resolved by the time a value reaches StackForm — every
// call is in strict-stack order with the arity it was matched at.
//
// Build a StackForm with Compile(); evaluate it with Eval();
// render it back to readable boru with Pretty(). Equivalent
// programs produce equal StackForms (Equal); cost-driven shrinking
// uses Cost.
//
// NOT THE BYTECODE PIPELINE. Despite the historical reference above,
// this package shares no code with the production compiler/VM
// (eng/go/emit.go, vm.go, bytecode.go — the Program / Instr / CALL_NATIVE
// path that RunCompiled drives). StackForm is an independent strict-stack
// IR used only by the property-based-testing reducer: its Quote / DoEval
// ops are reserved and not emitted today. Reach for emit.go / vm.go for
// the real bytecode path; reach here for PBT shrinking.
package stackform

import core "github.com/boru-lang/boru/core/go"

// Op is one element of a StackForm. The concrete variants are
// PushLit, Call, Quote, DoEval (see below).
type Op interface {
	opMarker()
}

// PushLit pushes a literal value onto the engine's stack.
//
// Literal here means anything that arrives at the engine pointer
// pre-resolved: integers, strings, decimals, booleans, atoms, type
// literals, concrete maps, and concrete lists with Eval=false.
// Lists with Eval=true that were consumed as code bodies live as
// Quote, not PushLit.
type PushLit struct {
	V core.Value
}

func (PushLit) opMarker() {}

// Call dispatches a word handler with `Arity` stack-resolved
// arguments. Arity matches the matched Signature.Args count
// (post-forward-resolution); the Recorder fires after sig matching
// completes so the arity is the runtime-observed arity, not a
// surface-syntax artifact.
type Call struct {
	Name  string
	Arity int
	// ReStep marks the `apply` WORD's dispatch of a fn value: the handler
	// hands the value back and the engine re-steps it, so the fn's own
	// dispatch is recorded as the NEXT op too — the same application
	// twice (NUR077's "Hole 2"). The form is faithful to what the engine
	// did and cannot be replayed as written; Replayable declines it.
	ReStep bool
}

func (Call) opMarker() {}

// Apply applies the function VALUE the stack holds beneath `Arity`
// arguments — a fn read out of a container, an inline lambda, a param —
// consuming the value as the interpreter's application does (NUR077: a
// `Call` re-invokes by NAME and consumes no receiver, so it could never
// stand for an application). Recorded where the engine applies a fn value
// (execFnDefSig's splice); replayed through the `apply` word over the
// value brought to the top — Eval supports arities 0, 1 and 2 (the stack
// rotations the vocabulary has), and Replayable declines a wider one.
type Apply struct {
	Arity int
}

func (Apply) opMarker() {}

// Quote pushes a quoted sub-program as a list-shaped value. The
// Body is the StackForm of the inner tokens (recursively Compiled).
// Quote appears when a code-body argument (NoEvalArgs position,
// like def/fn/if branches) is consumed.
//
// Implementation note: in the current Recorder model, Quote is not
// emitted by the engine's primary recorder — quoted bodies stay as
// PushLit of the raw list. A future enhancement can promote those
// to Quote when the body is later executed via call/do. See
// design/legacy/PBT-PLAN.10.ignore "Out of scope".
type Quote struct {
	Body *StackForm
}

func (Quote) opMarker() {}

// DoEval represents an explicit `do` on the top-of-stack quotation.
// Reserved for future work; not emitted by Compile today.
type DoEval struct{}

func (DoEval) opMarker() {}

// StackForm is the canonical strict-stack representation. It is an
// ordered sequence of Ops.
type StackForm struct {
	Ops []Op
}

// Len returns the number of Ops in the form.
func (f *StackForm) Len() int {
	if f == nil {
		return 0
	}
	return len(f.Ops)
}

// Append adds an Op to the form. Used by the Compile-time recorder
// and by reducer rewrite passes.
func (f *StackForm) Append(op Op) {
	f.Ops = append(f.Ops, op)
}
