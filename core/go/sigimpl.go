package core

import "sync/atomic"

// Signature implementation — a sealed sum type.
//
// A signature's run IMPLEMENTATION is one of two mutually-exclusive kinds:
//
//   - GoImpl — a Go `Handler` plus its dispatch knobs: a native word, a
//     usurp/force wrapper, the synthetic fallback, or a captured-native-fn
//     bound to a name.
//   - BoruImpl — a boru `Body` of tokens: an installed `def fn` body, an
//     anonymous lambda, or a module-ref trivial-delegation (`Body=[Word(inner)]`).
//
// The discriminator is `dispatchHandler() == nil` — nil selects the
// body-splice path (a module-ref / un-installed lambda). An INSTALLED boru fn
// is still a `BoruImpl`, but carries a derived body-splicing handler
// (buildFnBodyHandler) in `dispatch`, so its `dispatchHandler()` is non-nil.
//
// `normalizeSig` synthesizes `Impl` from the flat authoring fields at the
// install boundary — the SAME install-boundary demotion `Params` gets from the
// legacy `Args`. The flat `Handler`/`Body`/`FnFrame`/knob fields survive only
// as those authoring INPUTS; every reader consults `Impl` through the
// `Signature` accessors below (all reads were routed through them in Stage 3a).

// SigImpl is the sealed sum of a signature's run implementation. Only the two
// package-local variants satisfy it (the unexported `sigImpl()` seals the set,
// so the compiler enforces exhaustiveness at every type switch below).
type SigImpl interface {
	DispatchHandler() Handler
	sigImpl()
}

// GoImpl is the implementation of a native word, a usurp/force wrapper, the
// synthetic fallback, or a captured-native-fn bound to a name: a Go Handler
// plus the dispatch knobs that only a Go handler carries.
type GoImpl struct {
	Handler          Handler
	FullStack        bool
	CheckFullStackFn CheckFullStackFunc
	ParkResult       bool
	RunInCheckMode   bool
}

func (g *GoImpl) DispatchHandler() Handler { return g.Handler }
func (g *GoImpl) sigImpl()                 {}

// BoruImpl is the implementation of an installed boru fn body, an anonymous
// lambda, or a module-ref delegation. `dispatch` is the derived body-splicer
// (buildFnBodyHandler) for an installed fn, or nil for a module-ref /
// un-installed lambda — in which case execFnDefLiteral splices `Body` directly.
// A nil `dispatch` reproduces today's `Handler==nil` splice discriminator.
type BoruImpl struct {
	Body     []Value
	FnFrame  *FnFrameMeta
	dispatch Handler
	// compiled is the durable reference to this body's compiled unit, when a
	// program compiled it (a store-fn handler bake, a module-load stamp) or a
	// runtime seam stamped it at the value's first application (the lazy
	// detached stamp, S1b). It rides ALONGSIDE Body: the value keeps both
	// representations, so a callback runs the compiled unit when the registry
	// can host a VM run and falls back to splicing Body on the interpreter
	// otherwise. Nil for a body the compiler never armed (a plain interpreter
	// run, or a refused body). The slot is ATOMIC because a fn value is shared
	// by every copy of the Value that carries it — a container field, a
	// module export, a value handed to a forked process — and the lazy stamp
	// writes it from whichever seam applies the value first while another
	// fork may be reading it. Read through Compiled, written through
	// SetCompiled; the compiler piece owns the payload (an opaque
	// *CompiledFnRef, or its own declined marker — S4 opaque handle).
	compiled atomic.Value // compiledSlot
}

// compiledSlot boxes the compiled ref so a nil ref is storable (atomic.Value
// refuses a bare nil) and so a later Store never changes the stored type.
type compiledSlot struct{ ref any }

func (a *BoruImpl) DispatchHandler() Handler { return a.dispatch }
func (a *BoruImpl) sigImpl()                 {}

// Compiled returns the body's compiled-unit reference (opaque; nil when none
// was ever stamped or the last stamp was dropped).
func (a *BoruImpl) Compiled() any {
	if a == nil {
		return nil
	}
	slot, _ := a.compiled.Load().(compiledSlot)
	return slot.ref
}

// SetCompiled stores the body's compiled-unit reference; nil drops it.
func (a *BoruImpl) SetCompiled(ref any) { a.compiled.Store(compiledSlot{ref: ref}) }

// NewBoruImplCompiled builds a bare body impl that already carries its
// compiled-unit reference — the synthetic fn-value carriers the compiler mints
// for a stored code body (spawn's process body, a compiled param body), whose
// unit is known at construction.
func NewBoruImplCompiled(body []Value, ref any) *BoruImpl {
	a := &BoruImpl{Body: body}
	a.SetCompiled(ref)
	return a
}

// Clone returns a fresh impl carrying the same body, frame meta, dispatch
// handler and compiled reference — the pre-publication copy a value-level
// stamp mutates instead of the shared original (StampFnValue). A struct copy
// would copy the atomic slot by value, which is what the accessors exist to
// prevent.
func (a *BoruImpl) Clone() *BoruImpl {
	c := &BoruImpl{Body: a.Body, FnFrame: a.FnFrame, dispatch: a.dispatch}
	if ref := a.Compiled(); ref != nil {
		c.SetCompiled(ref)
	}
	return c
}

// --- Authoring constructors. Native words and internal Go-handler sites write
// `Impl: Go(handler, opts...)`; module-refs / un-installed bodies write
// `Impl: boru(body)`. These are the ONLY way an implementation is spelled once
// the flat authoring fields are retired. ---

// GoOpt sets an optional dispatch knob on a GoImpl built by Go.
type GoOpt func(*GoImpl)

// Go builds the implementation of a native word / internal Go handler.
func Go(h Handler, opts ...GoOpt) *GoImpl {
	g := &GoImpl{Handler: h}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// RunInCheck marks the handler to run even under check mode (usurp/force
// re-dispatch wrappers, def/undef, refine, depscalar predicates).
func RunInCheck() GoOpt { return func(g *GoImpl) { g.RunInCheckMode = true } }

// Park makes execMatch advance past the spliced result instead of re-stepping
// it (the `ref` word).
func Park() GoOpt { return func(g *GoImpl) { g.ParkResult = true } }

// FullStack passes the full resolved stack to the handler (depth/pick/roll/…).
func FullStack() GoOpt { return func(g *GoImpl) { g.FullStack = true } }

// CheckFullStack sets the check-mode full-stack replacement handler (paired
// with FullStack on the stack-shuffling words).
func CheckFullStack(fn CheckFullStackFunc) GoOpt {
	return func(g *GoImpl) { g.CheckFullStackFn = fn }
}

// boru builds the implementation of a module-ref trivial delegation or an
// un-installed lambda body — a token Body spliced by execFnDefLiteral (dispatch
// nil, no frame). InstallFnDef / compileFnDefLiteral build `&BoruImpl{…}`
// directly instead, so they can attach the derived body-splicer and frame meta.
func Boru(body []Value) *BoruImpl { return &BoruImpl{Body: body} }

// --- Signature accessors: the SINGLE way every reader reaches the run
// implementation, all reading the authoritative Impl. A nil Impl means the
// Signature has no implementation (a check-mode shape synth, a match-only test
// fixture) — dispatchHandler() is nil and the rest return their zero. ---

// DeclaresCheckReturns reports whether this signature tells the checker
// what it produces: a declared Returns slice (an empty non-nil slice is a
// valid "produces nothing"), a check-mode ReturnsFn, a full-stack
// CheckFullStack shape function, or a boru body (whose returns the
// analyser derives itself via AnalyseFnBody / the installed ReturnsFn). A
// signature declaring NONE of these hits the missing_returns fallback at
// every call site — the coverage gate
// (test/go/langspec/check_returns_gate_test.go) enumerates every
// registered native and fails on such a sig unless it is explicitly
// allowlisted with a justification.
func (s *Signature) DeclaresCheckReturns() bool {
	return s.Returns != nil || s.ReturnsFn != nil ||
		s.checkFullStackFn() != nil || len(s.Body()) > 0
}

func (s *Signature) DispatchHandler() Handler {
	if s.Impl == nil {
		return nil
	}
	return s.Impl.DispatchHandler()
}

// DispatchSig invokes sig's run implementation directly over
// already-resolved args (sig order). It is the invocation seam for
// synthesized COMPOSITE forms — def's keyword overloads (`def name fn
// […]`, `def name class {…}`, …) dispatch the captured constructor's
// own signature through it, keeping the constructor's handler the
// single implementation. Errors when the signature carries no
// runnable implementation (a match-only shape).
func DispatchSig(sig *Signature, args []Value, r *Registry) ([]Value, error) {
	h := sig.DispatchHandler()
	if h == nil {
		return nil, r.BoruError("signature_error",
			"signature has no run implementation", "")
	}
	return h(args, nil, nil, r)
}

// body returns the sig's boru token body (nil for a Go sig / no impl).
func (s *Signature) Body() []Value {
	if a, ok := s.Impl.(*BoruImpl); ok {
		return a.Body
	}
	return nil
}

// fnFrame returns the boru fn-frame metadata stamped on an installed body /
// anonymous lambda (nil for Go sigs and module refs).
func (s *Signature) FnFrame() *FnFrameMeta {
	if a, ok := s.Impl.(*BoruImpl); ok {
		return a.FnFrame
	}
	return nil
}

// fullStack reports whether the handler receives the full resolved stack.
func (s *Signature) FullStack() bool {
	g, ok := s.Impl.(*GoImpl)
	return ok && g.FullStack
}

// checkFullStackFn returns the check-mode full-stack handler, if any.
func (s *Signature) checkFullStackFn() CheckFullStackFunc {
	if g, ok := s.Impl.(*GoImpl); ok {
		return g.CheckFullStackFn
	}
	return nil
}

// parkResult reports whether the pointer advances past the spliced result
// instead of re-stepping it.
func (s *Signature) ParkResult() bool {
	g, ok := s.Impl.(*GoImpl)
	return ok && g.ParkResult
}

// runInCheckMode reports whether the handler runs even under check mode.
func (s *Signature) RunInCheckMode() bool {
	g, ok := s.Impl.(*GoImpl)
	return ok && g.RunInCheckMode
}
