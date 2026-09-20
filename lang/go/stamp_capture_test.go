package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Capture-bearing detached stamps (plan Phase 6.3): StampDetachedFn compiles
// a CAPTURING body — fd.Captured rides compileClosureBody's capture slots
// (the OpPushClosure layout) and the ref carries the captured VALUES so
// RunUnit/runUnitNested bind them at every invoke. The pins: the handler
// stamps (under the "(anonymous fn)" display name), the stamped VM run
// returns the CAPTURED value, and two fn values from the same factory carry
// their OWN captures (per-value refs — StampFnValue clones per value).
func TestCapturingHandlerStampsAndRunsWithCaptures(t *testing.T) {
	const factory = `def mk (fn [[n:Integer] [Any] [ def svc (service {}) add {cmd:"N"} ([req:Map state:Any] => [ n ]) svc svc ]]) `

	a := mustNew(t)
	disarm := a.ArmRuntimeStamping()
	out, err := a.RunInterp(factory + `def s7 (mk 7) def s9 (mk 9) [(call {cmd:"N"} s7) (call {cmd:"N"} s9)]`)
	disarm()
	if err != nil {
		t.Fatalf("armed run: %v", err)
	}
	if fmt.Sprint(out) != "[[7 9]]" {
		t.Errorf("per-value captures: got %v, want [[7 9]] (each service returns ITS n)", out)
	}
	stamped := 0
	for _, ev := range a.StampReport() {
		if ev.Stamped && strings.Contains(ev.Name, "anonymous fn") {
			stamped++
		}
	}
	if stamped < 2 {
		t.Errorf("want both capturing handlers stamped under the anonymous display name, got %d (report %+v)", stamped, a.StampReport())
	}

	// Parity negative: the unarmed interpreter run computes the identical
	// values with zero stamp attempts.
	b := mustNew(t)
	outI, errI := b.RunInterp(factory + `def s7 (mk 7) def s9 (mk 9) [(call {cmd:"N"} s7) (call {cmd:"N"} s9)]`)
	if errI != nil || fmt.Sprint(outI) != fmt.Sprint(out) {
		t.Errorf("parity: armed=%v unarmed=%v (err=%v)", out, outI, errI)
	}
	if got := b.StampReport(); len(got) != 0 {
		t.Errorf("unarmed run must not stamp, got %+v", got)
	}
}

// The §7a landing (COMPILE FAILURE-CLOSURE.0, 2026-07-16): a handler capturing a
// runtime-COMPUTED value — `(n add 1)` evaluated during a plain interpreter
// run, so the value carries NO compile identity under the mode-gated ID
// elision — STAMPS too: the detached compile mints a fresh identity on a
// clone of the frozen captured slice, so the capture keys its slot like any
// ID-bearing one. Previously this declined with "closure captures a
// runtime-minted value (no compile identity)". The pins: the handler stamps,
// the stamped run returns the CAPTURED computed value, per-value refs stay
// independent, and the unarmed interpreter run is byte-identical.
func TestComputedCaptureStampsAndRunsWithCaptures(t *testing.T) {
	const factory = `def mk (fn [[n:Integer] [Any] [ def m (n add 1) def svc (service {}) add {cmd:"N"} ([req:Map state:Any] => [ m ]) svc svc ]]) `
	const calls = `def s7 (mk 7) def s9 (mk 9) [(call {cmd:"N"} s7) (call {cmd:"N"} s9)]`

	a := mustNew(t)
	disarm := a.ArmRuntimeStamping()
	out, err := a.RunInterp(factory + calls)
	disarm()
	if err != nil {
		t.Fatalf("armed run: %v", err)
	}
	if fmt.Sprint(out) != "[[8 10]]" {
		t.Errorf("computed per-value captures: got %v, want [[8 10]] (each service returns ITS m=n+1)", out)
	}
	stamped, minted := 0, 0
	for _, ev := range a.StampReport() {
		if ev.Stamped && strings.Contains(ev.Name, "anonymous fn") {
			stamped++
		}
		if strings.Contains(ev.Reason, "runtime-minted value") {
			minted++
		}
	}
	if stamped < 2 {
		t.Errorf("want both computed-capture handlers stamped, got %d (report %+v)", stamped, a.StampReport())
	}
	if minted != 0 {
		t.Errorf("the runtime-minted-capture decline must be gone (§7a), got %d in %+v", minted, a.StampReport())
	}

	// Parity negative: the unarmed interpreter run computes the identical
	// values with zero stamp attempts.
	b := mustNew(t)
	outI, errI := b.RunInterp(factory + calls)
	if errI != nil || fmt.Sprint(outI) != fmt.Sprint(out) {
		t.Errorf("parity: armed=%v unarmed=%v (err=%v)", out, outI, errI)
	}
	if got := b.StampReport(); len(got) != 0 {
		t.Errorf("unarmed run must not stamp, got %+v", got)
	}
}

// The §7b landing (COMPILE FAILURE-CLOSURE.0, 2026-07-16): a MULTI-OVERLOAD handler
// stamps EVERY own sig to its own unit — the invoke seam dispatches through
// MatchFnSig, so the matched sig's own Impl ref is the "sig table". The
// two-arity service-handler shape (a `[req st prior]` overload beside a
// `[req st]` one) was the standing decline: under the old single-own-sig
// gate such a value never stamped and permanently interpreted.
func TestMultiOverloadHandlerStampsPerSig(t *testing.T) {
	const src = `def svc (service {}) add {cmd:"G"} (fn [[req:Map st:Map p:Function] [Any] [41 add 1] [req:Map st:Map] [Any] [99]]) svc [(call {cmd:"G"} svc)]`

	a := mustNew(t)
	disarm := a.ArmRuntimeStamping()
	out, err := a.RunInterp(src)
	disarm()
	if err != nil || fmt.Sprint(out) != "[[42]]" {
		t.Fatalf("armed run: out=%v err=%v, want [[42]] (the matched 3-arity unit)", out, err)
	}
	stamped := 0
	for _, ev := range a.StampReport() {
		if ev.Stamped {
			stamped++
		}
	}
	if stamped != 2 {
		t.Errorf("both overloads must stamp (§7b), got %d: %+v", stamped, a.StampReport())
	}
	// Parity: the unarmed interpreter run is byte-identical.
	b := mustNew(t)
	outI, errI := b.RunInterp(src)
	if errI != nil || fmt.Sprint(outI) != fmt.Sprint(out) {
		t.Errorf("parity: armed=%v unarmed=%v (err=%v)", out, outI, errI)
	}
	if got := b.StampReport(); len(got) != 0 {
		t.Errorf("unarmed run must not stamp, got %+v", got)
	}
}

// Per-sig fail-safe: an overload whose body DECLINES the unit compile (the
// context-dependent `args`) stays plain while its sibling stamps; the seam's
// matched sig runs its own ref — or CallBoru when it has none — so a partial
// stamp can never change behaviour.
func TestMultiOverloadHandlerPartialStamp(t *testing.T) {
	const src = `def svc (service {}) add {cmd:"P"} (fn [[req:Map st:Map p:Function] [Any] [req get "v"] [req:Map st:Map] [Any] [args]]) svc [(call {v:7 cmd:"P"} svc)]`

	a := mustNew(t)
	disarm := a.ArmRuntimeStamping()
	out, err := a.RunInterp(src)
	disarm()
	if err != nil || fmt.Sprint(out) != "[[7]]" {
		t.Fatalf("armed run: out=%v err=%v, want [[7]]", out, err)
	}
	stamped, declined := 0, 0
	for _, ev := range a.StampReport() {
		if ev.Stamped {
			stamped++
		} else if ev.Reason != "" {
			declined++
		}
	}
	if stamped != 1 || declined != 1 {
		t.Errorf("want exactly one stamped + one reasoned compile failure, got stamped=%d declined=%d: %+v",
			stamped, declined, a.StampReport())
	}
	b := mustNew(t)
	outI, errI := b.RunInterp(src)
	if errI != nil || fmt.Sprint(outI) != fmt.Sprint(out) {
		t.Errorf("parity: armed=%v unarmed=%v (err=%v)", out, outI, errI)
	}
}
