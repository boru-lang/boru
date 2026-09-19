package compiler

import core "github.com/boru-lang/boru/core/go"

// Runtime (detached) fn-unit stamping — design/legacy/RUNTIME-STAMPING.0.ignore.
//
// The compile-time store-fn bake (RecordCallOperands → compileStoredFnUnit →
// stampCompiledRef) stamps a CompiledFnRef only onto a fn value that is a
// concrete const in a CompileStoresFn slot of the program CURRENTLY being
// compiled. A fn value constructed at runtime — a service handler lambda
// added inside an interpreted module-fn body, a custom codec fn resolved
// from a codec map — is never visible to any compile pass, so its sig
// carries no ref and InvokeCallback permanently falls back to CallBoru.
//
// StampDetachedFn closes that gap: it compiles such a body to a standalone
// one-unit *Program OUTSIDE any whole-program pass, on an isolated fork of
// the live registry, and returns a ref the existing InvokeCallback seam runs
// via RunUnit / runUnitNested with zero changes to its happy path. Refusal
// is silent and per-body — the caller keeps the plain value and the
// interpreter behaviour is byte-identical (slow, never wrong).
//
// Freshness: a detached ref outlives the compile fork, so the compile-time
// NotifyNameRebound poisoning cannot observe later rebinds of the module
// names its body reads. Instead the ref carries a DepSnap — per dep name,
// the DefTable shadow depth and mutation generation at stamp time — which
// InvokeCallback validates on every invoke (CompiledFnRef.depsFresh); any
// mismatch degrades to CallBoru, which resolves the live binding exactly as
// the interpreter would.

// StampDetachedFn compiles a runtime-constructed, capture-free fn VALUE's
// FIRST stampable own-sig body to a standalone one-unit Program and returns
// its CompiledFnRef — the single-sig entry the predicate-type constructor
// and the module-load sweep use (their values are single-overload by
// construction). Multi-overload values stamp EVERY own sig through the
// value-level loops (StampFnValue / StampFnValueInPlace →
// StampDetachedSig, REFUSAL-CLOSURE §7b).
func StampDetachedFn(r *core.Registry, fd core.FnDefInfo, pos core.SrcPos) (*CompiledFnRef, bool) {
	si, ok := firstStampableSig(fd)
	if !ok {
		// Not recorded: native-word bindings swept by the module-load loop
		// land here — a shape that was never a compile candidate is report
		// noise, not an attempt.
		return nil, false
	}
	return StampDetachedSig(r, fd, si, pos)
}

// StampDetachedSig is the per-signature detached stamp: it compiles
// fd.Signatures[sigIdx]'s body to a standalone one-unit Program and returns
// its CompiledFnRef. It runs only when runtime stamping is armed on r
// (EnableRuntimeStamping — the compiled execution entry points). The compile
// is fully isolated: it runs on a ForkConcurrent copy of r, which carries
// its own fresh CheckState (the compile pass must not touch the parent's
// live check state). Refusal returns (nil, false) and leaves r untouched.
//
// The caller contract is ForkConcurrent's: invoke from the goroutine that
// owns r (store words and codec resolution run on the registry executing
// them, so this holds at every trigger site).
func StampDetachedSig(r *core.Registry, fd core.FnDefInfo, sigIdx int, pos core.SrcPos) (*CompiledFnRef, bool) {
	if r == nil || !r.RuntimeStampingEnabled() {
		return nil, false
	}
	if sigIdx < 0 || sigIdx >= len(fd.Signatures) || !storedSigEligible(&fd.Signatures[sigIdx]) {
		return nil, false
	}
	fork := r.ForkConcurrent()
	// The fork carries its OWN fresh check state (ForkConcurrent gives every
	// fork one — a fork's shallow copy used to alias r.Check, and arming a
	// compile pass on the alias would have trashed the parent's live
	// diagnostics and emit state), so the compile pass is armed on it
	// directly.
	defer fork.Check.BeginCompilePass()()
	// BeginCompilePass installs a concrete *EmitState; the two-value cast
	// (never-failing here) keeps this panic-free without an unreachable
	// guard branch — every EmitState method below is nil-receiver-safe and
	// compileStoredFnUnit declines on a nil state.
	es, _ := fork.Check.Recorder().(*EmitState)
	es.BindRegistry(fork)
	if es != nil {
		// Detached compiles run in GRADUAL-Any nesting mode: an Any arg
		// flowing into a nested callee's Any param binds a gradual carrier
		// (see EmitState.storedGradualDepth), so a handler calling an
		// `st:Any` helper that reads `st.kv` compiles instead of refusing
		// on the first field access. Safe here and only here — this fork
		// owns its program and Finalize, so any gradual-caused failure is
		// one silently declined stamp.
		es.storedGradualDepth = 1
		// Re-entrancy fence — see EmitState.inStampCompile. Measured before it
		// existed: lang/go/modules went from 77s to not finishing.
		es.inStampCompile = true
	}
	// An identity-less capture value (minted at pure runtime, where the
	// mode-gated ID elision skips minting) cannot key its positional capture
	// slot, so StartFnCompile's identity gate would refuse the unit
	// (REFUSAL-CLOSURE.0 §7a). For a DETACHED unit the capture is per-ref
	// and FROZEN — ref.Captures carries the construction-time snapshot — so
	// minting a fresh identity on a CLONE of the captured slice is confined
	// to this unit's compile: body reads resolve to the slot by the minted
	// ID exactly like an ID-bearing capture, and no shared published value
	// is mutated (the input's Captured backing array stays untouched). The
	// compile pass armed above keeps GenerateID live.
	for i := range fd.Captured {
		if fd.Captured[i].Value.ID == "" {
			cloned := append([]core.CapturedBinding(nil), fd.Captured...)
			for j := range cloned {
				if cloned[j].Value.ID == "" {
					cloned[j].Value.ID = core.GenerateID(core.IDPrefixForType(cloned[j].Value.Parent))
				}
			}
			fd.Captured = cloned
			break
		}
	}
	// Deps are read off the PRE-analysis def table (the fork clone equals r
	// here), so the snapshot below describes exactly what the body resolved
	// against — the analysis inside compileStoredFnUnit installs and restores
	// its own body-local bindings.
	deps := es.storedHandlerDeps(fd.Signatures[sigIdx].Body())
	unit, ok := es.compileStoredFnUnit(fd, sigIdx, pos)
	if !ok {
		// The probe's latched reason when it gave one; the report printer
		// substitutes a generic text for an empty reason (a refusal path
		// that never reached MarkUncompilable).
		r.RecordStampEvent(core.StampEvent{Name: fd.Name, Pos: pos, Reason: es.storedFnProbeReason})
		return nil, false
	}
	ref := &CompiledFnRef{Unit: unit, depNames: deps}
	// A capturing body compiled with its captures as trailing param slots
	// (compileStoredFnUnit passes fd.Captured to the closure compile — the
	// same slot layout OpPushClosure uses); the ref carries the captured
	// VALUES so RunUnit / runUnitNested bind them at every invoke. The
	// values are the construction-time snapshots the interpreter's dispatch
	// installs — frozen in fd.Captured, name-sorted, so slot order is
	// deterministic.
	for _, cb := range fd.Captured {
		ref.Captures = append(ref.Captures, cb.Value)
	}
	es.storedFnRefs = append(es.storedFnRefs, ref)
	prog, _, ok := es.Finalize(nil)
	if !ok || prog == nil || ref.Prog == nil {
		// REACHABLE, sound per-body fallback. compileStoredFnUnit's probe/real
		// passes only RECORD this body's events (and any nested fn as a
		// sub-unit); they do not LOWER the recorded units. Finalize does — its
		// per-unit lowering loop (emit.go, "fn <name>: " + reason) can refuse a
		// SUB-UNIT that the outer body's compile accepted. A runtime-constructed
		// fn whose body defines a nested fn that consumes a loop result (Stage-2
		// boundary: "consumes loop results") is the concrete case — the outer
		// compileStoredFnUnit returns ok, then Finalize declines on the inner
		// unit. This is the same "the real pass CAN decline after a clean probe"
		// shape compileStoredParamBody documents; the value stays plain and
		// interprets, byte-identically. (Graduated from //covergate:allow: the
		// variation sweep's module-body transform over a sift row reaches it —
		// design/COVERAGE-ALLOWLIST.10.md graduation.)
		r.RecordStampEvent(core.StampEvent{Name: fd.Name, Pos: pos, Reason: "finalize left the unit unstamped"})
		return nil, false
	}
	if len(deps) > 0 {
		ref.DepSnap = make(map[string]DepSnapEntry, len(deps))
		for name := range deps {
			ref.DepSnap[name] = DepSnapEntry{Depth: r.Defs.Depth(name), Gen: r.Defs.Gen(name)}
		}
	} else {
		// A body with no module deps still marks itself runtime-stamped with
		// an empty (non-nil) snapshot: vacuously fresh forever, but the field
		// keeps "detached ref" distinguishable from "compile-time ref".
		ref.DepSnap = map[string]DepSnapEntry{}
	}
	// Arm the JIT re-stamp box (REFUSAL-CLOSURE.0 §7c): the stamp inputs ride
	// the ref so a later dep rebind re-compiles against the live bindings at
	// invoke time (jitRestamp) instead of degrading permanently to CallBoru.
	// fd here carries the §7a identity-minted capture clone, so a re-stamp
	// needs no re-clone.
	ref.Restamp = &RestampBox{fd: fd, sigIdx: sigIdx, pos: pos}
	r.RecordStampEvent(core.StampEvent{Name: fd.Name, Pos: pos, Stamped: true})
	return ref, true
}

// RestampMaxTries bounds the TOTAL re-compiles one detached ref may pay
// across its lifetime: a dep that keeps rebinding between invokes would
// otherwise cost a compile per invoke — after the budget the seam stays on
// CallBoru, which resolves the live binding exactly as the interpreter.
const RestampMaxTries = 3

// jitRestamp is InvokeCallback's stale-ref recovery (REFUSAL-CLOSURE.0 §7c):
// when a detached ref's DepSnap no longer matches the live def table, re-run
// StampDetachedFn against the CURRENT bindings and return the fresh twin —
// each re-stamp snapshots the new generations, so a stable rebind pays one
// compile and then runs on the VM again. Returns nil when the seam should
// take the interpreter instead: a compile-time ref (no box), an exhausted
// try budget, or a declined re-stamp (stamping disarmed, the body now
// refusing). The box mutex serialises concurrent invokers of one shared sig
// — the winner compiles, the rest reuse its twin; StampDetachedFn itself
// runs on the CALLER's registry per its ForkConcurrent contract.
func (ref *CompiledFnRef) JitRestamp(r *core.Registry) *CompiledFnRef {
	box := ref.Restamp
	if box == nil {
		return nil
	}
	box.mu.Lock()
	defer box.mu.Unlock()
	if box.Cur != nil && box.Cur.DepsFresh(r) {
		return box.Cur
	}
	if box.Tries >= RestampMaxTries {
		return nil
	}
	box.Tries++
	nr, ok := StampDetachedSig(r, box.fd, box.sigIdx, box.pos)
	if !ok {
		return nil
	}
	box.Cur = nr
	return nr
}

// StampFnValue is the value-level entry over StampDetachedFn: given a fn
// VALUE (an FnDefInfo payload), it compiles the body and returns a CLONE of
// the value whose own boru sig carries the ref. The clone is deliberate — the
// input may be a published, shared value (a module binding, an interned
// const), and mutating its shared *BoruImpl from a store word would race
// concurrent readers; the compile-time stampCompiledRef mutates only
// pre-publication interned consts. On any decline (not a fn value, already
// stamped, capturing, ineligible shape, refusing body, policy off) it
// returns the input unchanged with ok=false, so callers may use the returned
// value unconditionally.
func StampFnValue(r *core.Registry, v core.Value) (core.Value, bool) {
	fd, ok := v.Data.(core.FnDefInfo)
	if !ok {
		return v, false
	}
	// Refuse an already-stamped value wholesale (first stamp wins: a
	// compile-time stamp or an earlier detached one already carries the VM
	// edge for the sigs it accepted; re-stamping is the §7c box's job).
	//
	// RECORD THE ATTRIBUTION ANYWAY. ok=false here means "this call did not
	// stamp", which is not the same as "this value is not compiled" — and the
	// stamp REPORT answers the second question. Callers that name an anonymous
	// value before stamping it (stampActionFn labels a model action's lambda
	// with the action name) are the reason it matters: once stampFnConst began
	// stamping fn consts at compile time, the model's own stamp found a ref,
	// declined, and `-compile-report` lost the "gen" row the frontier ledger
	// pins — while the body was, in fact, compiled. Reporting the outcome
	// rather than this call's participation in it keeps the report true.
	for i := range fd.Signatures {
		if CompiledRef(&fd.Signatures[i]) != nil {
			r.RecordStampEvent(core.StampEvent{Name: fd.Name, Pos: v.Pos(), Stamped: true})
			return v, false
		}
	}
	// EVERY stampable own sig compiles to its OWN unit and ref
	// (REFUSAL-CLOSURE §7b): the callback seam dispatches through
	// MatchFnSig, so the matched sig's Impl ref is the sig table. A sig
	// whose body declines stays plain and interprets — per-sig, fail-safe.
	// The sig slice clones once (and each stamped impl clones) so the stamp
	// never writes through a shared pointer of a published value.

	// Stamp against the fn's DEFINING registry
	// (design/FUNCTION-VALUE-SCOPE.0.md §7.3 item 6). A stored handler is
	// stamped where it is STORED — `service add`, codec resolution — but run
	// where it was WRITTEN, so compiling the body against the storing registry
	// would bake in that module's bindings for the body's free words and hand
	// the VM a different answer from the interpreter fallback. Same registry
	// for a fn defined in the running scope, so the common path is unchanged.
	home, _ := core.FnHome(r, &fd)
	var sigs []core.Signature
	for i := range fd.Signatures {
		if !storedSigEligible(&fd.Signatures[i]) {
			continue
		}
		ref, ok := StampDetachedSig(home, fd, i, v.Pos())
		if !ok {
			continue
		}
		if sigs == nil {
			sigs = make([]core.Signature, len(fd.Signatures))
			copy(sigs, fd.Signatures)
		}
		na := sigs[i].Impl.(*core.BoruImpl).Clone()
		na.SetCompiled(ref)
		sigs[i].Impl = na
	}
	if sigs == nil {
		// Nothing stamped: a Go-backed / fallback-only value (a built-in
		// codec), or every eligible body declined.
		return v, false
	}
	fd.Signatures = sigs
	out := v
	out.Data = fd
	return out, true
}

// StampFnValueInPlace is StampFnValue for PRE-PUBLICATION values: it stamps
// the ref onto the value's own shared *BoruImpl (stampCompiledRef) instead of
// cloning. Callers must guarantee the value has not escaped to concurrent
// readers — the one sanctioned site is module load (RunModuleBody), where the
// module's def bindings and its export map share impl pointers and both must
// see the stamp, and nothing outside the loading goroutine holds the value
// yet. Returns false on any decline, leaving the value untouched.
func StampFnValueInPlace(r *core.Registry, v core.Value) bool {
	fd, ok := v.Data.(core.FnDefInfo)
	if !ok {
		return false
	}
	for i := range fd.Signatures {
		if CompiledRef(&fd.Signatures[i]) != nil {
			return false
		}
	}
	// Per-sig stamps onto the value's own shared impls (pre-publication —
	// see the doc above): every stampable own sig gets its own ref
	// (REFUSAL-CLOSURE §7b); a declining body leaves that sig plain.
	// The defining registry compiles the body — see StampFnValue.
	homeIP, _ := core.FnHome(r, &fd)
	any := false
	for i := range fd.Signatures {
		if !storedSigEligible(&fd.Signatures[i]) {
			continue
		}
		ref, ok := StampDetachedSig(homeIP, fd, i, v.Pos())
		if !ok {
			continue
		}
		fd.Signatures[i].Impl.(*core.BoruImpl).SetCompiled(ref)
		any = true
	}
	return any
}

// stampDeclined is the marker LazyStampFnSig leaves in a body's compiled slot
// when its detached stamp declined: the next application finds the marker
// and takes the interpreter without paying the compile again. It is not a
// *CompiledFnRef, so CompiledRef reads the slot as "no ref" and a later
// compile-time stamp (which tests CompiledRef, not the raw slot) may still
// replace it.
type stampDeclined struct{}

// LazyStampFnSig is the detached stamp made universal (S1b of
// design/FULL-COMPILATION-REPLAN.0.md, the review's §3.3 "unit half"): a fn
// VALUE applied through a runtime seam — a callback handed to each/fold/scan
// through InvokeBody, a value InvokeCallback is about to fall back on —
// obtains a compiled unit for the sig the application matched, NOW, compiled
// at the value's home, and keeps it on the sig's shared impl so every later
// application of the same value (a container field read again, a module
// export applied in a loop) finds it without a second compile. The memo is
// the value itself: the impl is shared by every copy of the Value that
// carries the fn, and the slot is atomic (core.BoruImpl), so a fork applying
// the same value concurrently reads either nothing or a whole ref.
//
// Returns the ref to run, or nil when the seam keeps the interpreter: the
// sig is not a boru body, runtime stamping is not armed on r, the body is
// not stampable, or the stamp declined (remembered on the slot). A declined
// stamp is per value, not per application — the interpreter behaviour is
// byte-identical either way (slow, never wrong).
func LazyStampFnSig(r *core.Registry, fd core.FnDefInfo, sig *core.Signature, pos core.SrcPos) *CompiledFnRef {
	impl, ok := sig.Impl.(*core.BoruImpl)
	if !ok {
		return nil
	}
	if slot := impl.Compiled(); slot != nil {
		ref, _ := slot.(*CompiledFnRef)
		return ref // stamped already, or declined and remembered
	}
	if r == nil || !r.RuntimeStampingEnabled() {
		return nil
	}
	idx := -1
	for i := range fd.Signatures {
		if fd.Signatures[i].Impl == sig.Impl {
			idx = i
			break
		}
	}
	// A body that mutates the registry when run — a capitalised def, an
	// import — is never stamped lazily: the detached compile pass RUNS the
	// body in check mode, and the type it would mint or the module it would
	// load leaks into the live registry the value is about to be applied on
	// (measured: `def T (class {})` in a callback body raised the name
	// conflict at the FIRST element, one call early). The compile-time seat
	// has the same rule (bodyHasReplayHazard, the dyn-body seat).
	if idx < 0 || !storedSigEligible(sig) || bodyHasReplayHazard(core.NewList(sig.Body())) {
		impl.SetCompiled(stampDeclined{})
		return nil
	}
	// The defining registry compiles the body, as every value-level stamp
	// does (StampFnValue): a module export's free words resolve where it was
	// written.
	home, _ := core.FnHome(r, &fd)
	ref, ok := StampDetachedSig(home, fd, idx, pos)
	if !ok {
		impl.SetCompiled(stampDeclined{})
		return nil
	}
	impl.SetCompiled(ref)
	return ref
}
