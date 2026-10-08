package core

// Per-export module policy gating — the stamping half (NUR045).
//
// A policy profile may carry per-module subscopes with per-export rules
// (`modules.scopes."boru:time-util".words: deny ["sleep"]`). Enforcing
// them at dispatch requires every dispatch site — on BOTH engines — to
// know which module export it is running. Rather than threading that
// identity through every record/lower/dispatch path, it is stamped ONCE,
// at module-resolution time, onto the signatures a module's exports
// dispatch through (FnSig.ModuleCall):
//
//   - the export map's own wrapper/fn sigs — read by the fn-value
//     dispatch paths (execFnDefLiteral own-sig matching,
//     ExecFnDefSigStackMatch, VM tryNativeFnApply), and copied into any
//     `def w Pkg.word` rebinding (installDef's module-wrapper branch
//     copies the INNER sigs — see below — so the laundering path
//     `def s TimeUtil.sleep/v  s 300` stays gated);
//   - the module sub-registry's STORED sigs for each export's dispatch
//     target (the trivial-delegation inner native, or the preamble fn
//     itself) — the sigs reg.Lookup aggregates, matchSignature returns,
//     execMatch dispatches, and the compiled SigRef/PolyRef/UserPolyRef
//     re-matches resolve.
//
// Because Signature values are COPIED everywhere (aggregates, rebinds,
// usurp/force-arity synthetics, compiled sig interning), the stamp rides
// every descendant for free; a non-module signature keeps a nil
// ModuleCall and every gate is one pointer test.
//
// The gates themselves live at the dispatch chokepoints:
// engine.go (execMatch / ExecFnDefSigStackMatch / policyGateModuleCall),
// registry.go (CallBoru), and vm.go (CALL_NATIVE, CALL_USER, the poly
// re-matches, tryNativeFnApply) — one checker (LookupModuleCallChecker,
// policy_hook.go), identical error on either engine.

// StampModuleCallGates stamps the per-export policy identity onto a
// resolved module's dispatchable signatures. exports is the module
// descriptor's export map (namespace → export key → value); moduleRef
// is the resolved import id ("boru:time-util", "./lib.boru"). An empty
// moduleRef (an inline `module […]`, which has no stable policy-
// addressable id) stamps nothing. Idempotent: an already-stamped
// signature keeps its first identity (an export published under two
// keys gates under the first key stamped).
//
// Word-extension clones (`TimeUtil.add`'s temporal overloads) are
// SKIPPED: their signatures transplant onto the IMPORTER's own word and
// carry copies of the base word's core signatures — stamping them would
// gate plain core dispatches (`1 add 2`) as module calls.
func StampModuleCallGates(exports map[string]*OrderedMap, moduleRef string) {
	if moduleRef == "" {
		return
	}
	for _, om := range exports {
		if om == nil {
			continue
		}
		for _, key := range om.Keys() {
			v, _ := om.Get(key)
			fd, ok := v.Data.(FnDefInfo)
			if !ok {
				continue // type literals / constants: data reads, not calls
			}
			if _, isExt := IsWordExtension(v); isExt {
				continue
			}
			id := &ModuleCallID{Module: moduleRef, Export: key}
			stampSigsModuleCall(fd.Signatures, id)
			if !fd.HasHome() {
				continue
			}
			if !fd.Registry.IsModule() {
				fd.Registry.ModuleRef = moduleRef
			}
			// The stored dispatch targets in the module's sub-registry:
			// the fn's own name (preamble fns — reg.Lookup(fnDef.Name) is
			// what execFnDefLiteral matches against) and every trivial-
			// delegation inner native the wrapper's sigs point at.
			targets := map[string]bool{}
			if fd.Name != "" {
				targets[fd.Name] = true
			}
			own := fd.OwnSigs()
			for i := range own {
				if t, isDeleg := trivialDelegationTarget(&own[i]); isDeleg {
					targets[t] = true
				}
			}
			for name := range targets {
				stampStoredModuleCall(fd.Registry, name, id)
			}
		}
	}
}

// stampSigsModuleCall stamps id onto each unstamped, non-fallback
// signature in place. The slice's backing array is shared with the
// stored binding / export value the caller read it from, so the stamp
// lands on the canonical sigs every later copy descends from.
func stampSigsModuleCall(sigs []Signature, id *ModuleCallID) {
	for i := range sigs {
		if sigs[i].ModuleCall == nil && !sigs[i].Fallback {
			sigs[i].ModuleCall = id
		}
	}
}

// stampStoredModuleCall stamps id onto every FnDefInfo binding stacked
// under name in the module sub-registry. A target name that shadows a
// core word registered in the sub-registry stamps those core sigs TOO —
// correct, because the sub-registry's sigs are reachable only through
// this module's dispatch (`StructUtil.clone` resolving to a core
// overload IS a module call); the importer's own core words live in the
// importer's registry and are never touched.
func stampStoredModuleCall(r *Registry, name string, id *ModuleCallID) {
	for _, entry := range r.Defs.Stack(name) {
		if fd, ok := entry.Data.(FnDefInfo); ok {
			stampSigsModuleCall(fd.Signatures, id)
		}
	}
}

// StampedModuleCall returns the per-export policy identity stamped onto
// name's stored signatures in a module sub-registry, or nil when the
// name carries none. The compiled CALL_USER arm uses it rather than
// reconstructing an identity from the unit's name: a module-preamble
// fn's INNER name (`cli-w-usage`) is not the EXPORT key the policy
// addresses (`usage`), and only the stamp knows the mapping.
func StampedModuleCall(r *Registry, name string) *ModuleCallID {
	if r == nil {
		return nil
	}
	for _, entry := range r.Defs.Stack(name) {
		fd, ok := entry.Data.(FnDefInfo)
		if !ok {
			continue
		}
		for i := range fd.Signatures {
			if id := fd.Signatures[i].ModuleCall; id != nil {
				return id
			}
		}
	}
	return nil
}
