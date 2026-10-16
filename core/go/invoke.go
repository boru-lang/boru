package core

import "errors"

// InvokeBody executes a code BODY against per-call inputs and returns the
// residual value stack (bottom→top) — exactly as the body-running native
// handlers did when they spun up `New(r).Run([inputs… bodyTokens…])`.
//
// This is the seam through which a code-body word (do/case/where/group/
// having/order/select) runs its body from Go: a pooled sub-engine runs the
// reconstructed token stream, so behaviour is byte-identical to the
// pre-seam handlers (design plan P1). It was also the bytecode VM's way of
// driving a body without re-entering the interpreter; the compiler lane is
// gone (design/COMPILER-LANE-REMOVAL.0.md) and the seam stays as the
// Go-side body run.
//
// The ITERATING words — each, for-each, fold, scan, outer, inner, eachrank,
// foldaxis, filter's quotation form and the map forms over a quotation —
// no longer reach here on the interpreter: their loop runs on the running
// tape (StartLoop / LoopDriver, loop.go), one sealed region per element,
// and only the VM lane drives the same driver through this seam
// (DriveLoop).
//
// inputs are spliced BEFORE the body tokens, matching the handlers' historical
// `input[0..k-1]=inputs; input[k..]=bodyTokens` layout, so the body sees its
// per-call values exactly where the original code placed them.
func InvokeBody(r *Registry, body Value, inputs []Value) ([]Value, error) {
	// Pooled + resolved: the engine and its tape are reused across
	// invocations (runPooledSub / the registry sub-engine pool), and the
	// inputs enter as RESOLVED stack data rather than being re-stepped —
	// arguments are inert (design/legacy/ARG-SEMANTICS-UNIFICATION.0.ignore, via
	// RunResolved's start offset).
	//
	// A code-body word (each/fold/do/…) does NOT strip a dispatch ascription
	// from its result: it is inline value-routing (design/OPEN-WORDS.1.md
	// §9), transparent like a paren group or an if-branch, so the ascription
	// flows to the consuming dispatch — which matches the compiled path,
	// where `as` folds at compile time and the ascription rides the static
	// value flow to that same dispatch. (Only a fn/lambda/module return, an
	// abstraction boundary with a declared signature, strips.)
	return RunResolved(r, inputs, BodyTokens(body))
}

// BodyEscaped reports whether the body InvokeBody just ran left a
// break/continue unresolved — the registry's FlowCtrl set, the signal an
// enclosing loop or the top of the run will resolve (Engine.exitWithFlowCtrl's
// sub-engine contract; the VM's resolveEscapedFlow after the native returns). An
// ITERATING native — each, fold, scan, filter, for-each, the zip and matrix
// words — ends its iteration on it and returns NO result, instead of reading
// the escaped run's residual as the element's answer: the interpreter's
// sub-engine hands back the unstepped tape (a frame-cleanup marker `__CP`,
// which `filter` then reported as "must produce a Boolean, got __CP"), and
// the VM's island hands back nothing (which `each` then reported as "body
// produced no result", for the interpreter's flow_error — NUR196). Whatever
// the native returns on an escape is discarded by the flow's resolution —
// the loop's iteration is abandoned, or the run raises `outside loop` — so
// returning nothing is the one answer both lanes share. Single-shot bodies
// (`do`, a branch) need no such check: their residual is the run's to keep
// stepping, and no per-element contract judges it.
func BodyEscaped(r *Registry) bool {
	return r.FlowCtrl != FlowNone
}

// InvokeCallback runs a runtime fn VALUE (given its matched signature and the
// per-call args) on the interpreter — CallBoru with the fn's captures. It is
// the single seam every native callback word (serve-raw, spawn, service/codec
// endpoints) dispatches through, so a change to how a callback body runs is
// one routing decision rather than an edit per word.
//
// A callback fires AFTER the enclosing RunProgram returned (serve-raw handling
// a connection, a spawned process), so what the peer sees is CallBoru's own
// result and error taxonomy: genuine boru runtime errors (signature_error,
// type_error, …) are returned as-is.
func InvokeCallback(r *Registry, sig *Signature, args []Value, captures []CapturedBinding) ([]Value, error) {
	return invokeCallback(r, sig, args, captures, false, "", SrcPos{})
}

// InvokeCallbackStrict is InvokeCallback for a NAMED fn call — the module-fn
// dispatch at run time: CallBoruStrict, which enforces the frame's return
// count (NUR191).
func InvokeCallbackStrict(r *Registry, sig *Signature, args []Value, captures []CapturedBinding, label string, pos SrcPos) ([]Value, error) {
	return invokeCallback(r, sig, args, captures, true, label, pos)
}

func invokeCallback(r *Registry, sig *Signature, args []Value, captures []CapturedBinding, strict bool, label string, pos SrcPos) ([]Value, error) {
	// sig is the matched signature (callers dispatch it via MatchFnSig and check
	// it non-nil first — serve-raw's handler dispatch is the canonical caller).
	if strict {
		return r.CallBoruStrict(sig, args, captures, label, pos)
	}
	return r.CallBoru(sig, args, captures)
}

// InvokeCallbackFn is InvokeCallback for a fn VALUE whose definition is in
// hand. It runs the body in the fn's DEFINING registry whenever the value came
// from another module, so a callback's free words resolve where the function
// was WRITTEN rather than where it happens to be invoked
// (design/FUNCTION-VALUE-SCOPE.0.md).
//
// This is the seam fix for the native-callback class. `filter`, service
// handlers, codecs, Tui.run's update/view, refine predicates and the rest all
// reach a user fn through a Go word, and every one of them previously ran the
// body on the INVOKING registry — so a module-private helper the callback
// depends on either failed to resolve (undefined_word) or, when the invoking
// module happened to bind the same name, silently resolved to the WRONG
// definition and returned a plausible wrong answer. The interpreter's own
// foreign-registry branch (execFnDefSig) has always routed the value path this
// way; this makes the native seam agree with it.
//
// FnHome picks the registry and the captures; see its comment for the two nil
// cases.
func InvokeCallbackFn(r *Registry, fnDef *FnDefInfo, sig *Signature, args []Value) ([]Value, error) {
	return InvokeCallbackFnAs(r, fnDef, fnDef.nameOrEmpty(), sig, args)
}

// InvokeCallbackFnAs is InvokeCallbackFn for a fn value its holder names:
// name — the name a binding gave the holder (FnNameOf), passed beside the
// definition — attributes the first-application stamp.
func InvokeCallbackFnAs(r *Registry, fnDef *FnDefInfo, name string, sig *Signature, args []Value) ([]Value, error) {
	target, caps := FnHome(r, fnDef)
	return InvokeCallback(target, sig, args, caps)
}

// (CallBoruFn stood here — InvokeCallbackFn's sibling with the same
// defining-registry routing, kept apart while the compiled lane existed so
// that fixing WHERE free words resolve did not also change WHICH engine
// resolved them. The seven words it served (`filter`'s Function form, the
// map-lambda each/fold bodies, core walk / StructUtil.walk, IO.mount's
// fileops handlers, boru:parse's matcher and action callbacks, and the
// fn-util words) call InvokeCallbackFn. A SECOND dispatch path is a source of
// divergence in its own right — design/FN-VALUE-OPEN-WORK.0.md §Origin
// records two it caused — so with its last caller gone it was deleted rather
// than left as a hatch.)

// FnHome answers the two questions every native callback seam has to ask about
// a fn VALUE before running it: which registry resolves its free words, and
// which captures ride along.
//
// A boru-bodied fn carries the registry that MINTED it (FnConstruct, `=>`,
// `macro`), so its free words resolve where the function was WRITTEN rather
// than where a Go word happens to invoke it — in both directions: a module
// export applied from the main program reads the module's bindings, and a
// main-program fn handed into a module reads main's. The comparison is by
// MODULE (Registry.Home), not by pointer: when the caller is an instance of
// the fn's own module — the module registry itself, or a concurrent fork of
// it carrying a connection's or a service's live state — the fn runs on the
// CALLER, which is what that fork exists for. Only a genuinely foreign call
// moves to the defining registry.
//
// The nil cases fall back to the caller and are correct by construction:
// fnDef == nil is a synthesized carrier sig with no fn value behind it, and
// fnDef.Registry == nil is a Go-built value (a registered native, a wrapper
// minted by Go) with no free words to resolve.
func FnHome(r *Registry, fnDef *FnDefInfo) (*Registry, []CapturedBinding) {
	if fnDef == nil {
		return r, nil
	}
	if FnHomeForeign(r, fnDef) {
		return fnDef.Registry, fnDef.Captured
	}
	return r, fnDef.Captured
}

// FnHomeForeign reports whether applying fnDef from r crosses a module
// boundary: the fn carries a home and that home is not r's module. A Go-built
// value (no home) is never foreign, and neither is a fn invoked on a fork of
// the module that minted it.
func FnHomeForeign(r *Registry, fnDef *FnDefInfo) bool {
	return fnDef != nil && fnDef.HasHome() && !fnDef.Registry.SameHome(r)
}

// ShapeModelHomed reports whether the fn value is a method of a check-mode
// instance MODEL (Registry.ShapeModel): a stand-in for the run's method, never
// a constant to bake.
func (fd *FnDefInfo) ShapeModelHomed() bool {
	return fd.HasHome() && fd.Registry.ShapeModel
}

// MarkShapeModel marks the home registry of every method wrapper in a
// check-mode instance MODEL (Registry.ShapeModel), so no const gate takes the
// model's methods for the run's. A ReturnsFn that surfaces a shape-only
// instance calls it on the map it returns.
func MarkShapeModel(inst *OrderedMap) {
	for _, k := range inst.Keys() {
		mv, _ := inst.Get(k)
		if fd, ok := mv.Data().(*FnDefInfo); ok && fd.HasHome() {
			fd.Registry.ShapeModel = true
		}
	}
}

// HasHome reports whether the fn value carries the registry that minted it.
// Every boru-bodied fn does (FnConstruct, `=>`, `macro`, and a module's own
// exports); a Go-built value — a registered native read as `add/v`, a wrapper
// a fn-util word produces — has none, and nothing to resolve in one. This is
// the ONLY reading of a nil Registry: it never means "defined in the running
// scope" (NUR152 is what that reading cost), and a seam that needs the
// registry a body runs in asks FnHome, never the field.
func (fd *FnDefInfo) HasHome() bool {
	return fd != nil && fd.Registry != nil //sentinel:home the one reading of a nil fn-value Registry: a Go-built value with no home
}

// NamedDef reports whether the fn value is a def-bound, registry-dispatched
// definition: it carries a registered name AND was built by a verbose `fn`
// construction (or is a native), not by the `=>` lambda sugar. The two flags
// are read together because each alone misleads — a def-bound lambda
// (`def f => …`) has a Name but is still Anonymous and still models as a
// closure literal; a nameless verbose `fn` (a curried factory's inner fn)
// is not Anonymous but has no name to dispatch or recurse by. Only the
// conjunction carries the by-name dispatch and recursion semantics the
// closure models decline and the no-match diagnostics report against.
func (fd *FnDefInfo) NamedDef() bool {
	return fd.namedAs(fd.nameOrEmpty())
}

// namedAs is NamedDef under the name a holder presents the fn by
// (FnNameOf): a binding names its holder, never the shared payload.
func (fd *FnDefInfo) namedAs(name string) bool {
	return fd != nil && !fd.Anonymous && name != "" //sentinel:home the one compound reading of a name and Anonymous together
}

// nameOrEmpty is fd's own name, "" for a nil fd.
func (fd *FnDefInfo) nameOrEmpty() string {
	if fd == nil {
		return ""
	}
	return fd.Name
}

func FnHomeLookup(fd *FnDefInfo) *FnDefInfo {
	if !fd.HasHome() || fd.Name == "" {
		return nil
	}
	return fd.Registry.Lookup(fd.Name)
}

// isInternalErr reports whether err is an internal_error-class BoruError — the
// signal runVMEntry stamps on a recovered VM panic / lowering soundness bailout,
// and the exact class RunCompiled resolves by re-running on the interpreter. The
// callback seam mirrors that decision so a post-program bailout degrades to the
// interpreter rather than surfacing a raw compiler bug to a network peer.
func IsInternalErr(err error) bool {
	var ae *BoruError
	return errors.As(err, &ae) && ae.Code == "internal_error"
}

// IsInternalError is the exported face of isInternalErr, for run-side seams
// outside eng that take the same degrade-to-interpreter decision on a stamped
// unit's result (await's per-branch fork runs — native_temporal_await.go).
func IsInternalError(err error) bool { return IsInternalErr(err) }

// IsVMDefer reports whether err is a DESIGNED VM defer-to-interpreter (its
// BoruError carries VMDefer), as opposed to a user `raise internal_error …`
// carrying the same public code. The `do` escape hatch keys catch-or-re-raise
// on this: a defer must propagate so the run reports it, a
// user error stays trapped as an Error value.
func IsVMDefer(err error) bool {
	var ae *BoruError
	return errors.As(err, &ae) && ae.VMDefer
}

// runPooledSub runs input on a pooled reusable sub-engine and returns a
// caller-owned COPY of the results. It is the shared seam behind every
// per-element sub-evaluation (higher-order bodies, list/paren/interp-hole
// auto-evaluation): pooling means a hot loop reloads one tape in place
// instead of allocating a fresh ~DefaultTapeInitialFloor-entry tape per
// invocation. The result copy is mandatory — Engine.Run's result slice
// aliases the engine's tape (Tape.TakeAll), which the pool's next Reload
// overwrites — and it also stops a retained result from pinning the whole
// tape buffer the way the previous spin-up-per-call path did.
//
// elemEvalRecordable configures the sub-engine's container-element
// recording flag (see Engine.elemEvalRecordable); pass false when the
// caller does not evaluate recordable container elements.
func RunPooledSub(r *Registry, input []Value, elemEvalRecordable bool) ([]Value, error) {
	return runPooledSub(r, input, elemEvalRecordable, false)
}

// RunContainerSub is RunPooledSub for a container literal's elements — a
// pending list's contents, a map member, an interpolation hole — on a
// ContainerRun sub-engine: a break/continue escaping the elements tears
// down the frames they spliced and returns nothing, the registry's FlowCtrl
// left set for the literal's evaluator to abandon the value (NUR358).
func RunContainerSub(r *Registry, input []Value, elemEvalRecordable bool) ([]Value, error) {
	return runPooledSub(r, input, elemEvalRecordable, true)
}

func runPooledSub(r *Registry, input []Value, elemEvalRecordable, container bool) ([]Value, error) {
	sub := r.TakeSubEngine()
	sub.ElemEvalRecordable = elemEvalRecordable
	sub.ContainerRun = container
	res, err := sub.Run(input)
	if err != nil {
		r.PutSubEngine(sub)
		return nil, err
	}
	out := make([]Value, len(res))
	copy(out, res)
	r.PutSubEngine(sub)
	return out, nil
}

// BodyTokens returns the executable token sequence for a code body: a
// concrete list's own elements (the common case — `[mul 2]`; ReadList.Elems,
// not a copy: every caller reads them or copies them into the program it
// runs), or the value itself wrapped as a singleton for a non-list body.
func BodyTokens(body Value) []Value {
	if lst, err := AsList(body); err == nil && !lst.IsNil() {
		return lst.Elems()
	}
	return []Value{body}
}
