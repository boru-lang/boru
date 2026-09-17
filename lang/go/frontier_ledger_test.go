package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The frontier ledger — the expected-red harness for the runtime-independence
// program (design/legacy/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.ignore). Each frontier
// CASE asserts the TARGET behavior of a remaining compiler gap (compiles
// natively / runs on the VM / zero runtime bails); the LEDGER pins that the
// case fails today and HOW (the failure-mode substring). The runner is green
// while the frontier is red, and it enforces the test-first contract
// continuously, generalizing knownRefusals' stale-entry ratchet
// (test/go/langspec/compiled_refusals_test.go):
//
//   - a ledgered case that PASSES fails the runner ("graduate": the phase
//     landed — delete the ledger row; the case stays forever as a green pin);
//   - a ledgered case whose failure MODE drifts fails the runner
//     ("re-diagnose": a half-landed phase or a chained-leaf unmasking — the
//     designed L-JOIN→L-NP handoff surfaces here, loudly);
//   - an UNledgered case must pass (graduated targets never regress);
//   - a ledger row with failsWith == "" is the BOOTSTRAP sentinel: the runner
//     fails printing the observed error verbatim, so recording the initial
//     failure signature is enforced inside the landing change itself.
//
// This file is the CANONICAL runner copy; test/go/langspec and
// cmd/go/internal/{repl,exec} carry byte-similar copies (Go test helpers
// cannot cross modules/packages; test files carry no cover-gate cost).

// frontierCase asserts one TARGET behavior. run returns nil when the target
// holds. Cases are data — no *testing.T inside, every assertion through the
// error return — so the harness self-test can drive them synthetically.
type frontierCase struct {
	name string
	run  func() error
}

// frontierEntry pins one expected-red case.
type frontierEntry struct {
	why       string // the plan-phase gap and its mechanism reference
	failsWith string // substring the failure MUST contain ("" = bootstrap)
}

// failer is the *testing.T surface the runner needs — an interface so the
// harness self-test can substitute a recorder.
type failer interface {
	Helper()
	Errorf(format string, args ...any)
}

// runFrontier applies the ledger contract to every case.
func runFrontier(t failer, cases []frontierCase, ledger map[string]frontierEntry) {
	t.Helper()
	seen := make(map[string]bool, len(cases))
	for _, c := range cases {
		seen[c.name] = true
		err := c.run()
		entry, ledgered := ledger[c.name]
		switch {
		case !ledgered && err != nil:
			t.Errorf("frontier case %q must PASS (not ledgered — graduated targets never regress): %v", c.name, err)
		case ledgered && err == nil:
			t.Errorf("stale ledger entry %q — the target behavior now HOLDS; graduate: delete its ledger row (the case stays as a permanent pin).\n  was red because: %s", c.name, entry.why)
		case ledgered && entry.failsWith == "":
			t.Errorf("unpinned ledger row %q — record the failure mode into failsWith. Observed: %v", c.name, err)
		case ledgered && !strings.Contains(err.Error(), entry.failsWith):
			t.Errorf("frontier case %q failure MODE drifted:\n  got:    %v\n  pinned: %q\nre-diagnose (a drift can mean a phase half-landed, or a deeper leaf unmasked) before editing the row", c.name, err, entry.failsWith)
		}
	}
	for name, e := range ledger {
		if !seen[name] {
			t.Errorf("orphan ledger row %q (no such case): %s", name, e.why)
		}
	}
}

// TestFrontierLedger runs the package's frontier inventory (frontier_cases_test.go).
func TestFrontierLedger(t *testing.T) {
	runFrontier(t, frontierCases, frontierLedger)
	t.Logf("frontier: %d cases, %d expected-red", len(frontierCases), len(frontierLedger))
}

// recordingFailer captures runner errors for the harness self-test.
type recordingFailer struct{ errs []string }

func (r *recordingFailer) Helper() {}
func (r *recordingFailer) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

// TestFrontierRunnerHarness drives every runner arm with synthetic cases —
// the expected-red harness's own contract must never rot silently (a runner
// that reports nothing would turn the whole frontier suite into a no-op).
func TestFrontierRunnerHarness(t *testing.T) {
	fails := func(msg string) frontierCase {
		return frontierCase{name: msg, run: func() error { return fmt.Errorf("%s failure text", msg) }}
	}
	passes := func(name string) frontierCase {
		return frontierCase{name: name, run: func() error { return nil }}
	}

	check := func(name string, cases []frontierCase, ledger map[string]frontierEntry, wantErrs int, wantSubstr string) {
		t.Helper()
		rec := &recordingFailer{}
		runFrontier(rec, cases, ledger)
		if len(rec.errs) != wantErrs {
			t.Fatalf("%s: got %d runner errors %v, want %d", name, len(rec.errs), rec.errs, wantErrs)
		}
		if wantSubstr != "" && (len(rec.errs) == 0 || !strings.Contains(rec.errs[0], wantSubstr)) {
			t.Fatalf("%s: runner error %v, want substring %q", name, rec.errs, wantSubstr)
		}
	}

	check("pinned red is quiet",
		[]frontierCase{fails("a")},
		map[string]frontierEntry{"a": {why: "w", failsWith: "a failure text"}}, 0, "")
	check("stale entry demands graduation",
		[]frontierCase{passes("b")},
		map[string]frontierEntry{"b": {why: "w", failsWith: "x"}}, 1, "graduate")
	check("mode drift demands re-diagnosis",
		[]frontierCase{fails("c")},
		map[string]frontierEntry{"c": {why: "w", failsWith: "some other signature"}}, 1, "drifted")
	check("bootstrap sentinel demands recording",
		[]frontierCase{fails("d")},
		map[string]frontierEntry{"d": {why: "w", failsWith: ""}}, 1, "unpinned")
	check("unledgered pass is quiet",
		[]frontierCase{passes("e")}, nil, 0, "")
	check("unledgered failure is a regression",
		[]frontierCase{fails("f")}, nil, 1, "must PASS")
	check("orphan ledger row is an error",
		nil, map[string]frontierEntry{"g": {why: "w", failsWith: "x"}}, 1, "orphan")
}
