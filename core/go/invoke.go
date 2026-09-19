package core

import "errors"

// InvokeBody executes a code BODY against per-call inputs and returns the
// residual value stack (bottom→top) — exactly as the body-running native
// handlers did when they spun up `New(r).Run([inputs… bodyTokens…])`.
//
// This is the single seam through which every higher-order / code-body word
// (each/fold/scan/do/filter/case/where/group/having/order/outer/inner/select)
// runs its body. It exists so the bytecode VM can drive body execution
// WITHOUT re-entering the interpreter: when r.Invoker is set (the VM is
// running), the body is a compiled closure and execution re-enters the VM;
// when it is nil (a plain interpreter run) a fresh sub-engine runs the
// reconstructed token stream, so behaviour is byte-identical to the pre-seam
// handlers (design plan P1).
//
// inputs are spliced BEFORE the body tokens, matching the handlers' historical
// `input[0..k-1]=inputs; input[k..]=bodyTokens` layout, so the body sees its
// per-call values exactly where the original code placed them.
func InvokeBody(r *Registry, body Value, inputs []Value) ([]Value, error) {
	if r.Invoker != nil {
		return r.Invoker(r, body, inputs)
	}
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

// InvokeCallbackBody is the compiled-closure twin of InvokeCallbackFn — the
// fn-VALUE seam. A handler that hands a FnDefInfo to InvokeCallbackFn (the
// CallBoru discipline: the declared TYPES are checked over the aligned
// residual, the COUNT is trimmed, never raised — enforceCallBoruReturns)
// hands a compiled closure here, so the VM applies that same discipline
// (ClosurePayload.RetTrim → checkClosureReturn). InvokeBody is the TOKEN
// seam — each over a list, apply, a paren call — where the fn value is
// stepped and __RC enforces the count; a closure must cross the seam its
// handler's FnDefInfo path uses, or the lanes disagree: `walk … (m:Any =>
// [m.path print])` runs clean interpreted and raised `expected 1 return
// value(s), got 0` compiled once the lambda's count contract landed
// (NUR120). Without the VM (no Invoker) a closure never reaches a handler,
// so the fallback is InvokeBody's own.
func InvokeCallbackBody(r *Registry, body Value, inputs []Value) ([]Value, error) {
	if cl, ok := body.Data.(ClosurePayload); ok && !cl.RetTrim {
		cl.RetTrim = true
		body = Value{Parent: body.Parent, Data: cl, Quoted: body.Quoted}
	}
	return InvokeBody(r, body, inputs)
}

// ClosureSigMatched is the SigMatched mark on a closure VALUE (see
// ClosurePayload.SigMatched): the seam that hands the args has matched the
// closure's own signature, so the VM's invoker applies the unit positionally
// instead of matching again in the token seam's order. A non-closure value
// is returned unchanged.
func ClosureSigMatched(v Value) Value {
	cl, ok := v.Data.(ClosurePayload)
	if !ok || cl.SigMatched {
		return v
	}
	cl.SigMatched = true
	// A whole-value copy with the payload replaced, never a rebuild from
	// selected fields: the mark must not quietly drop the value's id,
	// position or any flag a later seam reads.
	out := v
	out.Data = cl
	return out
}

// ClosureAsFnDef is the compiled runtime's closure bridge, reached from a
// native seam (CompiledRuntime.ClosureAsFnDef): the FnDefInfo-shaped value a
// compiled closure stands in for on the interpreter — one signature over the
// unit's declared param contract — which is what a callback seam matches
// the closure's args against (MatchFnSig) before it runs the unit. ok=false
// outside a VM run, or for a unit with no contract of its own.
func ClosureAsFnDef(r *Registry, v Value) (Value, bool) {
	return compiledRuntime.ClosureAsFnDef(r, v)
}

// InvokeCallback runs a runtime fn VALUE (given its matched signature and the
// per-call args) against the VM when the sig carries a compiled unit whose
// program is stamped AND r can host a fresh run, else falling back to CallBoru —
// the tree-walking interpreter — with the fn's captures. It is the single seam
// every native callback word (serve-raw, spawn, service/codec endpoints)
// dispatches through, so retiring the interpreter for reducible callback bodies
// is one routing decision rather than an edit per word.
//
// Correctness is fail-safe: a nil CompiledRef, or an un-stamped ref (a body the
// compiler refused, or a run that never reached Finalize), falls to CallBoru,
// whose values and error taxonomy are unchanged. When the VM path IS taken it
// executes the exact unit the differential gates prove equivalent to the
// interpreter.
//
// A BUSY registry used to fall to CallBoru as well, and that was the seam's
// quiet hole rather than a safety property: a runtime-stamped body reached from
// inside a live compiled run — every predicate type in the corpus — recorded
// Stamped:true in the stamp ledger and then interpreted, because the nested
// runner declined any ref whose program was not the running one, which a
// detached ref never is. It now hosts the foreign unit instead
// (eng/go/vm_foreign_unit.go).
//
// A callback fires AFTER the enclosing RunProgram returned (serve-raw handling a
// connection, a spawned process), so — unlike an in-program island — there is no
// outer RunCompiled to catch a VM bailout and re-run on the interpreter. This seam
// therefore applies RunCompiled's own discipline itself: if the compiled unit
// raises an internal_error (a VM/lowering soundness assertion or a recovered
// handler panic, the class RunCompiled resolves by falling back), retry on
// CallBoru so the peer sees the interpreter's canonical result/error instead of a
// raw internal_error leaking to the network. Genuine boru runtime errors
// (signature_error, type_error, …) are the interpreter's answer too and are
// returned as-is.
func InvokeCallback(r *Registry, sig *Signature, args []Value, captures []CapturedBinding) ([]Value, error) {
	// sig is the matched signature (callers dispatch it via MatchFnSig and check
	// it non-nil first — serve-raw's handler dispatch is the canonical caller).
	// depsFresh guards RUNTIME-stamped refs (StampDetachedFn): a module dep
	// rebound or shadowed since the stamp means the frozen unit would diverge
	// from live resolution. A stale DETACHED ref first tries the JIT re-stamp
	// (REFUSAL-CLOSURE.0 §7c) — recompile against the live bindings, bounded
	// by the ref's try budget — and only a declined re-stamp takes the
	// interpreter, where the seam previously degraded permanently.
	// The compiled fast path lives behind the CompiledRuntime seam
	// (compiled_runtime.go, Stage 1 of the four-piece split): the VM
	// piece owns ref freshness, the JIT re-stamp, the C1 effect fence,
	// and the internal-error degrade decision. ran=false — including a
	// bailed unit with no observable effect — leaves the interpreter
	// path to the code below.
	res, err, ran := compiledRuntime.InvokeCompiled(r, sig, args)
	if ran {
		return res, err
	}
	// A unit that RAN and deferred is a designed bail, not an island: the
	// replay below is the second sanctioned interpreter entry, already in the
	// bail ledger (bailReplayAttribution). A ref that was never there declines
	// with a nil error, attributes nothing, and stays visible as the island it
	// is.
	defer bailReplayAttribution(r, err)()
	// Observability seam (interp_entry.go): the callback seam's interpreter
	// fallback — its own name so the C4 decline tag can attach later without
	// conflating it with a direct CallBoru.
	r.noteInterp("InvokeCallback:callboru")
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
// Routing through the defining registry also re-anchors the compiled path's
// dep-freshness test: InvokeCompiled evaluates CompiledFnRef.depsFresh against
// whichever registry it is handed, so handing it the caller's registry made the
// check answer a question nobody asked.
//
// FnHome picks the registry and the captures; see its comment for the two nil
// cases.
func InvokeCallbackFn(r *Registry, fnDef *FnDefInfo, sig *Signature, args []Value) ([]Value, error) {
	target, caps := FnHome(r, fnDef)
	// The detached stamp at first application (S1b): a value with no unit for
	// this sig obtains one now, at its home, memoised on the value — so the
	// seam below takes the VM instead of CallBoru. Declined stamps are
	// remembered on the value; the interpreter path stays byte-identical. A
	// nil fnDef is a synthesized carrier sig with no fn value behind it (see
	// FnHome): nothing to stamp.
	if fnDef != nil {
		compiledRuntime.LazyStamp(target, *fnDef, sig, SrcPos{})
	}
	return InvokeCallback(target, sig, args, caps)
}

// (CallBoruFn stood here — InvokeCallbackFn's interpreter-only sibling, same
// defining-registry routing but never offering the body to the VM. It existed
// for one reason, stated in its own comment: "fixing WHERE free words resolve
// must not also change WHICH engine resolves them", the right scope fence for
// design/FUNCTION-VALUE-SCOPE.0.md's change. Full compilation reverses that
// fence — the seven words it served (`filter`'s Function form, the map-lambda
// each/fold bodies, core walk / StructUtil.walk, IO.mount's fileops handlers,
// boru:parse's matcher and action callbacks, and the fn-util words) now call
// InvokeCallbackFn, so the body is offered to the VM with the CallBoru
// fallback intact. A SECOND dispatch path is a source of divergence in its own
// right — design/FN-VALUE-OPEN-WORK.0.md §Origin records two it caused — so
// with its last caller gone it is deleted rather than left as a hatch.)

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
	return fd != nil && !fd.Anonymous && fd.Name != "" //sentinel:home the one compound reading of Name and Anonymous together
}

// FnHomeLookup resolves the fn value's NAME in its home registry — the
// definition a module wrapper delegates to, or a module-preamble fn's own
// stored binding — and is nil for a value with no home, no name, or a home
// that does not bind the name. It is the one way to reach a value's inner
// definition: the dispatch seams that used to spell it as
// `fd.Registry == nil || fd.Name == ""` followed by `fd.Registry.Lookup` all
// ask this instead, so the nil home is read in exactly one place.
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
// on this: a defer must propagate to complete the whole-program fallback, a
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
	// Observability seam (interp_entry.go): the per-element sub-evaluation path.
	r.noteInterp("runPooledSub")
	sub := r.TakeSubEngine()
	sub.ElemEvalRecordable = elemEvalRecordable
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

// bodyTokens returns the executable token sequence for a code body: a concrete
// list's elements (the common case — `[mul 2]`), or the value itself wrapped
// as a singleton for a non-list body. Mirrors what the handlers extracted via
// `AsList(body).Slice()` before splicing.
func BodyTokens(body Value) []Value {
	if lst, err := AsList(body); err == nil && !lst.IsNil() {
		return lst.Slice()
	}
	return []Value{body}
}
