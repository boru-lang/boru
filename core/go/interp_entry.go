package core

import "sync/atomic"

// Observability seams for the runtime-independence program
// (design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md, C4 + Phase 10): two
// arm-able hooks that make silent engine decisions observable WITHOUT
// changing behaviour, the way the stamp log does for stamping decisions
// (stamp_report.go). Both are TEST SEAMS (design/TEST-SEAMS.10.md), not API:
// production code never arms them, an unarmed hook costs one atomic load at
// each emit point, and tests restore via the returned disarm func.
//
//   - The INTERPRETER-ENTRY hook observes every entry into tree-walking
//     machinery (Engine.Run, RunResolved, CallBoru, runPooledSub, and
//     InvokeCallback's CallBoru fallback). The frontier suite uses it to
//     assert "this program ran with no unattributed interpreter execution" —
//     the C4 end-state invariant — at entry points (REPL, exec), callback
//     seams, and post-Stage-J the public Run itself.
//   - The RUNTIME-BAIL hook observes every DESIGNED VM defer-to-interpreter
//     (the internal_error class RunCompiled resolves by re-running): the
//     compile-time census is structurally blind to these, so the executed
//     bail census (TestRuntimeBailCensus, plan Phase 10) needs its own
//     instrument.
//
// The hook holders live on the Registry as pointers, so ForkConcurrent's
// shallow copy shares them into concurrent forks for free; module
// sub-registries inherit them explicitly (InheritObserveHooks) alongside the
// writers and the effect ledger.

// InterpEntry is one observed entry into interpreter machinery.
type InterpEntry struct {
	// Seam names the entry point: "Engine.Run", "RunResolved", "CallBoru",
	// "runPooledSub", or "InvokeCallback:callboru" (the callback seam's
	// interpreter fallback — distinguished so its C4 decline tag can attach
	// when attribution lands).
	Seam string
	// Attribution is the C4 carve-out tag this entry belongs to ("check-mode"
	// today; decline/oracle tags land with plan Phase 10). "" = unattributed —
	// interpreter execution the end-state invariant does not permit.
	Attribution string
	// CheckMode reports whether the registry was in check mode at entry (the
	// compiler front-end executing RunInCheckMode words — always attributed).
	CheckMode bool
}

// BailEvent is one designed VM defer-to-interpreter (a runtime bail): the VM
// raised the internal_error that makes RunCompiled re-run the source on the
// interpreter (or, post effect-fence, propagate).
type BailEvent struct {
	// Site names the defer site class: "vm:poly-no-match",
	// "vm:poly-nout-drift", "vm:user-poly-unresolved", "vm:user-poly-drift",
	// "vm:user-poly-no-match", "vm:rematch-matched",
	// "vm:shaped-method-not-appliable", "vm:splice-active-payload",
	// "vm:dyn-scope-miss", "vm:dyn-scope-dispatching",
	// "vm:dyn-scope-active-token", "vm:dyn-scope-data-miss",
	// "vm:dyn-scope-data-class", "vm:dyn-scope-data-active-token",
	// "vm:dyn-frame-replay".
	Site string
	// Reason is the defer's message text (what vmErrAt wraps).
	Reason string
}

// interpEntryHook holds the armed interpreter-entry callback. The atomic
// pointer keeps the unarmed fast path to a single load with no lock — the
// emit points sit on the interpreter's hottest paths (per-element
// runPooledSub) — while staying race-free against arming from another
// goroutine (the -race concurrency gates run forks against a parent-armed
// hook).
type interpEntryHook struct {
	fn atomic.Pointer[func(InterpEntry)]
}

// bailHook holds the armed runtime-bail callback (same discipline).
type bailHook struct {
	fn atomic.Pointer[func(BailEvent)]
}

// RegionOracleEvent is one execution of the compiled lane's COLLECT oracle
// (compiler.RegionOracle, eng/go/region_oracle.go): a region descriptor
// walked LIVE by the VM against the registry the dispatch sees, and checked
// against the claim the static lowering made for the same dispatch.
// Observability only — the VM continues to the ordinary call whatever the
// outcome — so the lane that arms it can count how often the live walk
// reproduces the recorded claim before any dispatch is routed through one.
type RegionOracleEvent struct {
	// Word is the descriptor's lead word; Pos its position; NFwd the claim.
	Word string
	Pos  SrcPos
	NFwd int
	// Outcome is "reproduced" (a live signature claims exactly NFwd slots and
	// every live word slot presents the value the lowering pushed),
	// "declined" (the walk needed an evaluation the descriptor host cannot
	// perform, so nothing is known), "unbound" (the lead has no live
	// binding), "under-claimed" (the live walk would claim MORE than the
	// record — the record stopped early, the safe direction),
	// "over-claimed" (every live signature claims LESS than the record —
	// the miscompile direction) or "diverged-value" (a live word slot's
	// binding is not what the lowering pushed). Detail names the
	// disagreement.
	Outcome string
	Detail  string
}

// regionOracleHook holds the armed region-oracle callback (same discipline).
type regionOracleHook struct {
	fn atomic.Pointer[func(RegionOracleEvent)]
}

// ArmInterpEntryHook installs fn as the interpreter-entry observer and
// returns the disarm func (test seam — pair with defer/t.Cleanup). A
// registry assembled without NewRegistry has no holder and arms nothing
// (no-op disarm).
func (r *Registry) ArmInterpEntryHook(fn func(InterpEntry)) func() {
	if r.interpHook == nil {
		return func() {}
	}
	r.interpHook.fn.Store(&fn)
	return func() { r.interpHook.fn.Store(nil) }
}

// ArmRuntimeBailHook installs fn as the runtime-bail observer and returns
// the disarm func (test seam). Same holder discipline as ArmInterpEntryHook.
func (r *Registry) ArmRuntimeBailHook(fn func(BailEvent)) func() {
	if r.bailHook == nil {
		return func() {}
	}
	r.bailHook.fn.Store(&fn)
	return func() { r.bailHook.fn.Store(nil) }
}

// ArmRegionOracleHook installs fn as the region-oracle observer and returns
// the disarm func (test seam). Same holder discipline as ArmInterpEntryHook.
func (r *Registry) ArmRegionOracleHook(fn func(RegionOracleEvent)) func() {
	if r.regionOracleHook == nil {
		return func() {}
	}
	r.regionOracleHook.fn.Store(&fn)
	return func() { r.regionOracleHook.fn.Store(nil) }
}

// NoteRegionOracle emits one region-oracle observation when the hook is
// armed. Nil-safe on both the registry and the holder; the emit point is
// the VM's OpCollect (eng/go), outside this module.
func (r *Registry) NoteRegionOracle(ev RegionOracleEvent) {
	if r == nil || r.regionOracleHook == nil {
		return
	}
	if fp := r.regionOracleHook.fn.Load(); fp != nil {
		(*fp)(ev)
	}
}

// InheritObserveHooks shares the parent's hook holders (and nothing else)
// into a module sub-registry, alongside the writer/effect-ledger threading in
// RunModuleBody — a module body's interpreter entries and runtime bails must
// report to the SAME observers the enclosing request armed.
func (r *Registry) InheritObserveHooks(parent *Registry) {
	r.interpHook = parent.interpHook
	r.bailHook = parent.bailHook
	r.regionOracleHook = parent.regionOracleHook
	r.coverHook = parent.coverHook
	r.coverSources = parent.coverSources
}

// noteInterp emits one interpreter-entry observation when the hook is armed.
// Nil-safe on both the registry and the holder.
func (r *Registry) noteInterp(seam string) {
	if r == nil || r.interpHook == nil {
		return
	}
	fp := r.interpHook.fn.Load()
	if fp == nil {
		return
	}
	att := r.interpAttribution
	check := r.analysisActive()
	if check {
		att = "check-mode"
	}
	(*fp)(InterpEntry{Seam: seam, Attribution: att, CheckMode: check})
}

// NoteInterp emits one interpreter-entry observation for an emit point
// OUTSIDE this module — the VM's island seams (eng/go), which reach
// Engine.Run through their own pooled run and would otherwise report only
// the generic "Engine.Run" seam, indistinguishable from a sanctioned
// interpreter run. The engine-entry census
// (design/FULL-COMPILATION.0.md section 9) has to name WHICH mechanism
// re-entered, so the SEAM carries the label.
//
// Deliberately does NOT touch Attribution: an island is exactly the
// unattributed interpreter execution the C4 end-state invariant forbids,
// and tagging one would make the frontier's unattributed-entry assertions
// (lang/go/frontier_cases_test.go fcNoUnattributedInterp) pass vacuously.
func (r *Registry) NoteInterp(seam string) {
	r.noteInterp(seam)
}

// SetInterpAttribution installs tag as the C4 attribution context for
// interpreter entries on this registry and returns the restore func — the
// compiled-mode entry points bracket their SANCTIONED fallback re-runs with
// it ("fallback:refusal", "fallback:runtime-bail") so the interp-entry hook
// reports the re-run's entries as attributed. Pair with defer.
func (r *Registry) SetInterpAttribution(tag string) func() {
	prev := r.interpAttribution
	r.interpAttribution = tag
	return func() { r.interpAttribution = prev }
}

// noteBail emits one runtime-bail observation when the hook is armed.
func (r *Registry) NoteBail(site, reason string) {
	if r == nil || r.bailHook == nil {
		return
	}
	fp := r.bailHook.fn.Load()
	if fp == nil {
		return
	}
	(*fp)(BailEvent{Site: site, Reason: reason})
}

// bailReplayAttribution brackets the interpreter replay that follows a stamped
// unit's decline — and only when that decline was a DESIGNED VM defer.
//
// CompiledRuntime.InvokeCompiled declines with ran=false for two different
// reasons, and only the error tells them apart (see its contract): a nil error
// means the unit was never hosted, a non-nil one means it RAN and deferred —
// the C1 internal-error degrade, which vmDefer has already written to the bail
// ledger. The replay of that defer belongs to the same category RunCompiled's
// top-level arm names, "fallback:runtime-bail".
//
// Leaving it unattributed counted one defer TWICE — once in the bail census as
// the defer it is, and again in the interp-entry census as an island. The nil
// case attributes nothing, so a lane that never reached the VM at all still
// shows up as the island it is.
func bailReplayAttribution(r *Registry, declined error) func() {
	if declined == nil {
		return func() {}
	}
	return r.SetInterpAttribution("fallback:runtime-bail")
}
