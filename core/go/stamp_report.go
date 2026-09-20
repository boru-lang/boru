package core

import "sync"

// Stamp attribution (design/legacy/RUNTIME-STAMPING.0.ignore Phase 5). Runtime stamping
// is silent by design — a compile failure must never change behaviour — which
// historically made "did this callback compile?" unobservable from outside a
// Go test (the gap that let a stale "the handlers compile" claim survive in
// the design notes). When stamping is armed, every detached-stamp ATTEMPT
// records a StampEvent on a per-arming log; the lang layer exposes the log
// (Boru.StampReport) and the CLI prints it under -compile-report.
//
// Only genuine attempts record: values the primitive was never going to
// compile (non-fns, Go-backed wrappers, already-stamped values — the bulk of
// a module's def table) are skipped so the report stays readable.

// StampEvent is one detached-stamp attempt: the fn's name (empty for an
// anonymous handler lambda — Pos then locates its construction site), the
// outcome, and the compile failure reason when the compile declined.
type StampEvent struct {
	Name    string
	Pos     SrcPos
	Stamped bool
	Reason  string // "" when Stamped
}

// stampLog collects StampEvents across every registry that shares one
// arming: ForkConcurrent's shallow copy and RunModuleBody's inheritance both
// propagate the POINTER, so per-connection forks and module sub-registries
// feed one report. Mutex'd — stamps run on whichever goroutine owns the
// triggering registry.
type stampLog struct {
	mu     sync.Mutex
	events []StampEvent
}

func (l *stampLog) record(ev StampEvent) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.events = append(l.events, ev)
	l.mu.Unlock()
}

func (l *stampLog) reset() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.events = nil
	l.mu.Unlock()
}

// StampEvents returns a copy of the stamp attempts recorded on this
// registry's arming (nil when runtime stamping was never armed).
func (r *Registry) StampEvents() []StampEvent {
	if r == nil || r.stampLog == nil {
		return nil
	}
	r.stampLog.mu.Lock()
	defer r.stampLog.mu.Unlock()
	out := make([]StampEvent, len(r.stampLog.events))
	copy(out, r.stampLog.events)
	return out
}

// recordStampEvent appends to the shared log; inert when unarmed.
func (r *Registry) RecordStampEvent(ev StampEvent) {
	if r == nil {
		return
	}
	// An anonymous fn (an afn lambda handler) has no Name; record the
	// canonical display name so report consumers match events by name the
	// same way PrintStampReport renders them — capturing lambdas are the
	// COMMON detached-stamp shape post the capture-slot landing.
	if ev.Name == "" {
		ev.Name = "(anonymous fn)"
	}
	r.stampLog.record(ev)
}

// ResetStampLog drops the recorded stamp attempts while leaving stamping armed
// (a no-op when unarmed). RunCompiled's interpreter-fallback path calls it
// after RestoreForCompile rolls back the check pass's in-place module-load
// stamps, so only the fallback re-run's authoritative stamps reach the report
// — otherwise -compile-report prints each rolled-back stamp twice.
func (r *Registry) ResetStampLog() {
	if r != nil {
		r.stampLog.reset()
	}
}
