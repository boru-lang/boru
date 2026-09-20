package core

// Compile-pass state isolation (design/legacy/boru-bytecode-plan.0.ignore §Stage 5,
// fallback registry-isolation follow-on).
//
// CompileCheck executes the program in check mode, so its
// RunInCheckMode words (def/import/type/macro, the Test harness) leave
// real side effects on the registry: def bindings, minted lattice
// types, the module load set, context-store state. The COMPILED path
// relies on those persisting (OpPushType resolves minted IDs; islands
// re-run via a sub-engine over the same registry). But when a program
// is UNCOMPILABLE nothing re-runs the source, and
// double-applies them — a re-mint, a re-import, a re-run Test spec —
// diverging from a clean interpreter run.
//
// CompileSandbox snapshots the mutable execution scopes before the
// check pass so the fallback path can roll them back and interpret on
// pristine state. It is the in-place counterpart to ForkConcurrent: a
// fork gives a SEPARATE registry (which loses module-import fidelity
// when re-running an import), whereas this restores the SAME registry,
// so the fallback interpreter sees exactly what a fresh run would.

// CompileSandbox holds clones of every registry scope the check pass
// can mutate. Read-only infrastructure (builtin lattice, ideals,
// capabilities, native registrations, parser, host hooks) is shared and
// not captured.
type CompileSandbox struct {
	defs      *DefTable
	types     *TypeTable
	ctx       []*StoreInstanceInfo
	modLoaded map[string]ModuleDesc
	modSeq    int
	builtins  map[string]bool
	caps      map[string]any
	check     *CheckState
	flow      FlowCtrl
	pendGen   *GenSpecInfo
	valid     bool
}

// SnapshotForCompile captures the registry's mutable execution state.
func (r *Registry) SnapshotForCompile() CompileSandbox {
	if r == nil {
		return CompileSandbox{}
	}
	// check is DEEP-cloned (CheckState.Clone) because Check is now a shared
	// *CheckState: a module sub-registry can transiently point its Check at the
	// parent pass's and mutate its maps IN PLACE during module-fn body analysis
	// (design/legacy/module-fn-checkstate-ownership.1.ignore §3.2). The old by-value copy
	// reasoning ("the check pass only ever REPLACES, never mutates in place") no
	// longer holds, so a shallow capture would let in-place mutations survive the
	// rollback. pendGen stays a by-pointer capture (still only reassigned).
	s := CompileSandbox{
		defs:    r.Defs.Clone(),
		types:   r.Types.Clone(),
		ctx:     r.Contexts.Snapshot(),
		check:   r.Check.Clone(),
		flow:    r.FlowCtrl,
		pendGen: r.pendingGen,
		valid:   true,
	}
	if r.Modules != nil {
		s.modSeq = r.Modules.seq
		s.modLoaded = make(map[string]ModuleDesc, len(r.Modules.Loaded))
		for k, v := range r.Modules.Loaded {
			s.modLoaded[k] = v
		}
	}
	if r.builtinWords != nil {
		s.builtins = make(map[string]bool, len(r.builtinWords))
		for k, v := range r.builtinWords {
			s.builtins[k] = v
		}
	}
	// Capability slots are mostly host config (file ops, clock, policy,
	// output) — stored as pointers we copy by reference. The check pass
	// can ADD a per-run accumulator slot (the Test harness's testRun);
	// a shallow map snapshot rolls exactly those additions back while
	// preserving the host-config pointers untouched.
	if r.Capabilities != nil && r.Capabilities.store != nil {
		s.caps = make(map[string]any, len(r.Capabilities.store))
		for k, v := range r.Capabilities.store {
			s.caps[k] = v
		}
	}
	return s
}

// RestoreForCompile rolls the registry back to a SnapshotForCompile
// capture, discarding every check-pass side effect.
func (r *Registry) RestoreForCompile(s CompileSandbox) {
	if r == nil || !s.valid {
		return
	}
	r.Defs = s.defs
	// The restored clone carries a FRESH gen map (DefTable.Clone starts a
	// new generation timeline at 0), but the dispatchCache lives on the
	// registry and is keyed by (name, gen). Reinstalling a reset-gen table
	// under the old cache would let a gen-0 entry cached before this
	// rollback be served for a name whose restored binding differs — the
	// gen counter can no longer distinguish them. Drop every entry so the
	// fallback interpreter rebuilds every aggregate from the restored state
	// (this is the rare uncompilable path; correctness over the cache).
	// reset() empties the holder in place rather than swapping the pointer,
	// preserving the never-reassigned invariant shared registries rely on.
	r.dispatchCache.reset()
	r.Types = s.types
	r.Contexts.Restore(s.ctx)
	// Restore analysis state IN PLACE so a module sub-registry transiently
	// sharing this *CheckState observes the rollback (§3.2).
	if s.check != nil && r.Check != nil {
		*r.Check = *s.check
	} else {
		r.Check = s.check
	}
	r.FlowCtrl = s.flow
	r.pendingGen = s.pendGen
	if r.Modules != nil {
		r.Modules.seq = s.modSeq
		r.Modules.Loaded = s.modLoaded
	}
	// builtins/caps restore only when the snapshot captured a (non-nil) copy.
	// Both maps are constructed in NewRegistry and never reset to nil, so the
	// snapshot always holds a copy in practice; the guard just avoids
	// installing a nil map on the degenerate (never-constructed) path rather
	// than being a meaningful "leave as-is" branch.
	if s.builtins != nil {
		r.builtinWords = s.builtins
	}
	if s.caps != nil && r.Capabilities != nil {
		r.Capabilities.store = s.caps
	}
}
