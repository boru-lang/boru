package core

import (
	"sort"
	"sync"
	"sync/atomic"
)

// boru-source line coverage — the interpreter seam that powers boru:test's
// coverage feature (design intent: boru:test measures which source lines of a
// boru module-under-test its tests exercise, so a pure-Boru module like boru:sift
// can be driven to 100% line coverage).
//
// The mechanism mirrors the ObserveHooks pattern (interp_entry.go): a
// registry-level, atomic-pointer hook, OFF by default (one atomic load on the
// unarmed path), shared into module sub-registries by InheritObserveHooks. The
// step loop (engine.go) emits the executing token's SrcPos row once per step.
//
// Attribution is by COVER ID, not by row alone: SrcPos carries no file tag, so
// the harness source and the module-under-test both start at row 1 and their
// rows collide. Instead, only a registry that carries a non-empty CoverID emits
// — and a module-under-test runs its fn bodies on its OWN sub-registry (boru:sift
// runs on siftModReg), which the loader tags with a stable id ("boru:sift"). The
// harness registry has no CoverID, so its tokens are never recorded; every
// recorded (id, row) is unambiguously the tagged module's. This also keeps the
// unarmed hot path to a single atomic load and the ARMED path to the tagged
// module's tokens only.

// coverHook holds the armed line-coverage callback. Same atomic-pointer
// discipline as interpEntryHook: race-free arming, single-load unarmed path.
type coverHook struct {
	fn atomic.Pointer[func(coverID string, row int)]
}

// coverSources maps a cover id to the module-under-test's SOURCE text. A module
// loader registers its source so the coverage reporter can build the denominator
// (all executable rows) without re-reading a file. Shared (pointer) into forks
// and module sub-registries by InheritObserveHooks, like the hook holders.
type coverSources struct {
	mu sync.Mutex
	m  map[string]string
}

// RegisterCoverSource records src as the source text for cover id. A later
// coverage report parses it to enumerate the denominator. Nil-safe; last write
// wins (idempotent module re-import).
func (r *Registry) RegisterCoverSource(id, src string) {
	if r == nil || r.coverSources == nil || id == "" {
		return
	}
	r.coverSources.mu.Lock()
	defer r.coverSources.mu.Unlock()
	if r.coverSources.m == nil {
		r.coverSources.m = map[string]string{}
	}
	r.coverSources.m[id] = src
}

// CoverSource returns the registered source text for cover id, or ("", false).
func (r *Registry) CoverSource(id string) (string, bool) {
	if r == nil || r.coverSources == nil {
		return "", false
	}
	r.coverSources.mu.Lock()
	defer r.coverSources.mu.Unlock()
	src, ok := r.coverSources.m[id]
	return src, ok
}

// CoverSourceIDs returns every registered cover source id, sorted. The
// `boru test --coverage` runner enumerates them after a run to report each
// imported module's line coverage. Nil-safe; empty when nothing registered.
func (r *Registry) CoverSourceIDs() []string {
	if r == nil || r.coverSources == nil {
		return nil
	}
	r.coverSources.mu.Lock()
	defer r.coverSources.mu.Unlock()
	ids := make([]string, 0, len(r.coverSources.m))
	for id := range r.coverSources.m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ArmCoverageHook installs fn as the line-coverage observer and returns the
// disarm func (test seam — pair with defer/t.Cleanup). A registry assembled
// without NewRegistry has no holder and arms nothing (no-op disarm).
func (r *Registry) ArmCoverageHook(fn func(coverID string, row int)) func() {
	if r == nil || r.coverHook == nil {
		return func() {}
	}
	r.coverHook.fn.Store(&fn)
	return func() { r.coverHook.fn.Store(nil) }
}

// CoverageArmed reports whether a coverage run has the hook armed right now. A
// module loader consults it at import time so a module-under-test tags itself
// as a coverage target (SetCoverID) ONLY inside a `Test.cover [ import … ]`
// body — an ordinary run leaves it untagged, so the compiled hot path stays a
// single field compare with no atomic load.
func (r *Registry) CoverageArmed() bool {
	return r != nil && r.coverHook != nil && r.coverHook.fn.Load() != nil
}

// SetCoverID tags this registry with a stable coverage source id. A module
// loader sets it on the module's sub-registry so the module's executed tokens
// are attributed to it; the empty default means "not a coverage target" and
// suppresses emission entirely.
func (r *Registry) SetCoverID(id string) {
	if r == nil {
		return
	}
	r.coverID = id
}

// CoverID reports this registry's coverage source id ("" when untagged).
func (r *Registry) CoverID() string {
	if r == nil {
		return ""
	}
	return r.coverID
}

// noteVMCoverage is the compiled (VM) step-site coverage emit: it records
// debug[pc]'s row when this registry is a coverage target. The coverID field
// compare short-circuits an untagged unit BEFORE noteCoverage's atomic load, so
// an ordinary compiled run pays one branch per instruction. Kept tiny (the Go
// inliner folds it into the VM run loop) so vm.go's hot loop is unchanged.
func (r *Registry) NoteVMCoverage(debug []SrcPos, pc int) {
	if r == nil || r.coverID == "" {
		return
	}
	r.noteCoverage(debug[pc])
}

// noteCoverage emits one line-coverage observation when the hook is armed AND
// this registry is a tagged coverage target. Nil-safe on both the registry and
// the holder; a check-mode step is never counted (static analysis re-runs the
// same tokens — only real execution is coverage). The unarmed path is a single
// atomic load.
func (r *Registry) noteCoverage(pos SrcPos) {
	if r == nil || r.coverHook == nil {
		return
	}
	fp := r.coverHook.fn.Load()
	if fp == nil {
		return
	}
	if r.coverID == "" || pos.Row == 0 || r.analysisActive() {
		return
	}
	(*fp)(r.coverID, pos.Row)
}

// InheritObserveHooks shares the parent's line-coverage hook and cover-source
// table into r — a module sub-registry — so a coverage run armed on the
// parent records the module's rows.
func (r *Registry) InheritObserveHooks(parent *Registry) {
	r.coverHook = parent.coverHook
	r.coverSources = parent.coverSources
}
