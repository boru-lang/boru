package lang

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// The lang-package frontier inventory (see frontier_ledger_test.go for the
// contract): cases whose TARGET assertions need Go-level observability — the
// stamp report, the interp-entry hook, the runtime-bail hook. Pure
// source→value/error frontier repros live as shared TSV rows instead
// (lang/spec/frontier/, WS2 of the plan) so the TS port runs them too.

// --- error-returning assertion helpers --------------------------------------

// fcNew builds a fresh instance or reports why it could not.
func fcNew() (*Boru, error) {
	a, err := New()
	if err != nil {
		return nil, fmt.Errorf("New: %w", err)
	}
	return a, nil
}

// fcParityCompiledZeroBails asserts the FULL target for a source-level
// frontier gap: the program compiles (no compile failure), runs on the VM with zero
// designed runtime bails, and its values, error taxonomy, AND printed output
// are byte-identical to the interpreter's.
func fcParityCompiledZeroBails(src string) error {
	a, err := fcNew()
	if err != nil {
		return err
	}
	var outC bytes.Buffer
	a.SetOutput(&outC)
	var bails []BailEvent
	defer a.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e) })()
	gotC, compiled, reason, errC := a.RunCompiledReason(src)
	if !compiled {
		if reason != "" {
			return fmt.Errorf("declined: %s", reason)
		}
		return fmt.Errorf("did not run compiled (err=%v)", errC)
	}
	if len(bails) > 0 {
		return fmt.Errorf("runtime bails: %s", bailCensus(bails))
	}

	b, err := fcNew()
	if err != nil {
		return err
	}
	var outI bytes.Buffer
	b.SetOutput(&outI)
	gotI, errI := b.RunInterp(src)
	if codeOf(errC) != codeOf(errI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		return fmt.Errorf("error parity: compiled [%s] %v vs interp [%s] %v", codeOf(errC), errC, codeOf(errI), errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		return fmt.Errorf("value parity: compiled %v vs interp %v", gotC, gotI)
	}
	if outC.String() != outI.String() {
		return fmt.Errorf("output parity: compiled %q vs interp %q", outC.String(), outI.String())
	}
	return nil
}

// fcStampedRun runs src through RunCompiled and asserts the stamp report
// carries a SUCCESSFUL stamp for a binding whose name contains name.
func fcStampedRun(src, name string) error {
	a, err := fcNew()
	if err != nil {
		return err
	}
	a.SetOutput(&bytes.Buffer{})
	if _, _, err := a.RunCompiled(src); err != nil {
		// A declining fixture returns compile_failed under the Stage-J
		// default; this case's contract is the STAMP REPORT, not the
		// compile failure policy, so fall back explicitly (the CLI's own pattern)
		// and assert stamps over the interpreter run.
		if !strings.Contains(fmt.Sprint(err), "compile_failed") {
			return fmt.Errorf("run failed before the stamp assertion: %w", err)
		}
		disarm := a.ArmRuntimeStamping()
		_, ierr := a.RunInterp(src)
		disarm()
		if ierr != nil {
			return fmt.Errorf("interp run failed before the stamp assertion: %w", ierr)
		}
	}
	var attempted *StampEvent
	for i, ev := range a.StampReport() {
		if strings.Contains(ev.Name, name) {
			if ev.Stamped {
				return nil
			}
			attempted = &a.StampReport()[i]
		}
	}
	if attempted != nil {
		return fmt.Errorf("stamp declined for %q: %s", name, attempted.Reason)
	}
	return fmt.Errorf("no stamp attempt recorded for %q", name)
}

// fcNoUnattributedInterp arms the interp-entry hook, runs drive, and fails
// when any interpreter entry lacks a C4 attribution — the end-state invariant
// ("no interpreter execution of an accepted program on a default path").
func fcNoUnattributedInterp(drive func(a *Boru) error) error {
	a, err := fcNew()
	if err != nil {
		return err
	}
	a.SetOutput(&bytes.Buffer{})
	var (
		mu      sync.Mutex
		entries []InterpEntry
	)
	disarm := a.ArmInterpEntryHook(func(e InterpEntry) {
		mu.Lock()
		defer mu.Unlock()
		entries = append(entries, e)
	})
	defer disarm()
	if err := drive(a); err != nil {
		return fmt.Errorf("drive failed before the entry assertion: %w", err)
	}
	mu.Lock()
	defer mu.Unlock()
	counts := map[string]int{}
	for _, e := range entries {
		if e.Attribution == "" {
			counts[e.Seam]++
		}
	}
	if len(counts) > 0 {
		return fmt.Errorf("unattributed interpreter entries: %s", seamCensus(counts))
	}
	return nil
}

func seamCensus(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s×%d", k, counts[k])
	}
	return strings.Join(parts, " ")
}

func bailCensus(bails []BailEvent) string {
	counts := map[string]int{}
	for _, b := range bails {
		counts[b.Site]++
	}
	return seamCensus(counts)
}

// --- shared frontier sources -------------------------------------------------

// lJoinRepro is the L-JOIN minimal repro verbatim from
// design/legacy/VOXGIG-COMPILE-LEAVES.2.ignore — the recursive branch-join accumulator
// whose self-call operand loses provenance across fixpoint iterations. The
// only library-code blocker (tst.boru / radix.boru).
const lJoinRepro = `def rec fn [
  [nd:Any key:Any consumed:Any best:Any] [Any] [
    if (nd eq none) [best] [
      def pc (consumed "x" add)
      def best2 (if (nd "end" get) [pc] [best])
      best2 consumed key (nd "mid" get) rec
    ]
  ]
]
(rec none "hi" "" none) print end`

// --- the inventory ------------------------------------------------------------

var frontierCases = []frontierCase{
	// Phase 4 — GRADUATED 2026-07-14 (permanent pin): the L-JOIN repro
	// compiles (the disjunct-distribution recording fix, carrier.go
	// disjunctPartitionReturns/carrierResults) and runs compiled with ZERO
	// runtime bails and full parity — the staged L-NP handoff never fired.
	{"p4/l-np-no-runtime-bail-after-join", func() error {
		return fcParityCompiledZeroBails(lJoinRepro)
	}},

	// Phase 6 — stamping extensions.
	{"p6/predicate-stamps-and-runs-vm", func() error {
		return fcStampedRun(`def Pos fnpred [[n:Integer] [n gt 0]] def x:Pos 5 x`, "Pos")
	}},
	{"p6/model-action-stamps", func() error {
		return fcStampedRun(`import "boru:model" def m (Model.new {src:'a: 1 b: 2', actions:{gen:([mod:Any] => [true])}}) (Model.run m) get 'ok'`, "gen")
	}},
	// GRADUATED 2026-07-15: StampDetachedFn compiles capturing bodies —
	// fd.Captured rides compileClosureBody's capture slots (the OpPushClosure
	// layout) and the ref carries the captured VALUES for bindUnitLocals at
	// every invoke; anonymous handlers record under "(anonymous fn)". The
	// case stays as a permanent pin.
	{"p6/capturing-handler-stamps", func() error {
		// The capture shape from run_compile_report_test.go (verbatim
		// — the leading map-lambda statement is load-bearing for the parse):
		// the service handler closes over n.
		return fcStampedRun(`def m {f: ([y:Integer] => [y add 1])} add 1 (5 (m get "f") apply) drop def mk (fn [[n:Integer] [Any] [ def svc (service {}) add {cmd:"N"} ([req:Map state:Any] => [ n ]) svc svc ]]) def s (mk 7) (call {cmd:"N"} s)`, "anonymous fn")
	}},
	// UN-GRADUATED 2026-07-18: the gen/property bodies were graduated
	// 2026-07-14 (stored-param-body units on the VM), but a direct rand-call
	// GEN body (`[r.int 1 9]` — a member fn read from the opaque Map param
	// `r` applied to forward args) is the fn-value-call boundary: it CANNOT
	// compile to a correct unit and must DECLINE. The "graduated" unit
	// silently returned the trailing arg (a constant generator — the
	// r.int→9 miscompile that turned the trie/sort PBT suites vacuous). The
	// decline fix (emit.go closure-residual dynMaybeFn compile failure) makes the gen
	// body interpret per-iteration, so this case is now legitimately red —
	// ledgered below. The property body (no member-fn boundary) still
	// compiles; only the direct rand-call gen interprets.
	{"p6/check-prop-body-on-vm", func() error {
		// Module-scope check-prop with a DIRECT rand-call gen body: the gen
		// body's member-fn-arrival dispatch declines (sound), so the
		// per-iteration gen run adds unattributed interpreter entries.
		src := "import \"boru:test\" end\ndef res (Test.check-prop \"x\" [r.int 1 9] [ var [[k] (`v${k}`) eq `v${k}` ] ] 5 1 0)\nres get \"ok\""
		return fcNoUnattributedInterp(func(a *Boru) error {
			_, err := a.RunCompiledStrict(src)
			return err
		})
	}},
	{"p6/concurrent-fork-bodies-on-vm", func() error {
		return fcNoUnattributedInterp(func(a *Boru) error {
			_, _, err := a.RunCompiled(`import "boru:time-util" TimeUtil.await [[1 add 2] [3 add 4]]`)
			return err
		})
	}},
	// GRADUATED 2026-07-15 (the FINAL frontier row — 0 expected-red): the
	// policy-gate lift (user-authorized) let Vm.run's sub-engine run
	// compiled-by-default (modules.CompiledSubRun, injected by lang; the
	// composed sandbox policy is enforced per VM dispatch by gateWord).
	// The case stays as a permanent pin.
	{"p6/vm-run-on-vm", func() error {
		return fcNoUnattributedInterp(func(a *Boru) error {
			_, _, err := a.RunCompiled(`import "boru:vm" Vm.run "1 add 2"`)
			return err
		})
	}},

	// Phase 10 — the executed-census seeds.
	{"p10/no-unattributed-interp-on-islanded-program", func() error {
		// A genuine whole-program compile failure (the each variadic-if knownCompileFailures
		// row): today the silent fallback re-runs the source unattributed.
		// Target: every residual interpreter entry belongs to a named C4 seam.
		return fcNoUnattributedInterp(func(a *Boru) error {
			_, _, _ = a.RunCompiled(zzFailingRow) // the row raises; the entries are the assertion
			return nil
		})
	}},
	{"p10/runtime-bail-census-canary", func() error {
		// The zz-inst shape-claim violation is a real, reachable runtime bail;
		// the executed census must reach zero before Stage J.
		a := zzShapedInstanceE()
		if a == nil {
			return fmt.Errorf("zz-inst fixture unavailable")
		}
		a.SetOutput(&bytes.Buffer{})
		var bails []BailEvent
		defer a.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e) })()
		// The run now FAILS when it bails — nothing re-runs it — so the
		// error is expected and is not what this measures. The census is:
		// a reachable runtime bail is an open defect and must reach zero.
		_, _, _ = a.RunCompiled(`def i (zz-inst) ; i.m 5 ; 42`)
		if len(bails) > 0 {
			return fmt.Errorf("runtime bails: %s", bailCensus(bails))
		}
		return nil
	}},

	// Phase 11 — Stage J.
	// GRADUATED 2026-07-15 (permanent pin): Stage J's Run flip landed —
	// the public Run executes compiled-by-default with zero unattributed
	// interpreter entries. The last two blockers closed the same day: the
	// "concurrent-watch composite" (actually the cross-request def
	// persistence bug — OpBindGlobal) and the cross-instance observability
	// pollution (no longer reproducible after it: 20 plain + 5 -race
	// in-sequence ledger runs clean).
	{"p11/public-run-is-compiled", func() error {
		return fcNoUnattributedInterp(func(a *Boru) error {
			_, err := a.Run(`1 add 2`)
			return err
		})
	}},
	{"p11/no-unbounded-fallback", func() error {
		// GRADUATED 2026-07-15 (permanent pin): a program that does not
		// compile returns the reason as an error and never silently re-runs
		// the source. The BORU_COMPILE_FALLBACK=1 hatch that used to restore
		// the old behaviour retired with every other fallback on 2026-09-19.
		// The probe must be a program that FAILS TO COMPILE yet SUCCEEDS
		// interpreted (a raising one cannot distinguish a re-run's error from
		// a returned compile failure): the paren-bounded fn-value application
		// does not compile and runs to bad_input/q on the interpreter.
		const failingButSucceeds = `def zf fn [[x:Any] [Any] [raise bad_input 'no']]  def msg (do [(zf 5) 2] error [dot code])  msg`
		a, err := fcNew()
		if err != nil {
			return err
		}
		a.SetOutput(&bytes.Buffer{})
		out, compiled, rerr := a.RunCompiled(failingButSucceeds)
		if !compiled && rerr == nil && len(out) > 0 {
			return fmt.Errorf("compile failure resolved by the silent interpreter re-run (post-Stage-J it returns the compile failure error)")
		}
		return nil
	}},
}

// zzShapedInstanceE is zzShapedInstance without the *testing.T (cases are
// data): it returns nil if the fixture cannot be built.
func zzShapedInstanceE() *Boru {
	a, err := New()
	if err != nil {
		return nil
	}
	zzInstallShapedInstance(a)
	return a
}

var frontierLedger = map[string]frontierEntry{
	// GRADUATED 2026-07-14: p4/l-np-no-runtime-bail-after-join — BOTH stages
	// at once: the L-JOIN compile failure was a per-alternative recording leak (the
	// disjunct-distribution combos ran a user fn's ReturnsFn under the armed
	// recording with fresh-ID alternative copies — RecordUserCall then declined
	// "unknown provenance"); the fix suspends the combo probes and records ONE
	// CALL_USER with the original args (partition-joined carriers re-IDed onto
	// the recorded results, gated by disjunctCombosTakeSig). The anticipated
	// L-NP vm:dyn-scope-miss bail never materialised — the repro runs compiled
	// with ZERO runtime bails and full parity (bytecode_ljoin_test.go pins the
	// family). The case above stays as a permanent pin.
	// GRADUATED 2026-07-14: p6/predicate-stamps-and-runs-vm — predicates
	// stamp at construction (InstallType stamps the body in place, naming
	// the stamp event with the type name; RunPredicate's InvokeCallback then
	// runs the unit on the VM). The case above stays as a permanent pin.
	// GRADUATED 2026-07-14: p6/model-action-stamps — buildActions stamps each
	// action at model build (stampActionFn: the model's private copy takes the
	// action name and a detached unit; InvokeCallback runs it on the VM). The
	// case above stays as a permanent pin.
	// GRADUATED 2026-07-14: p6/concurrent-fork-bodies-on-vm — await's
	// parallels compile per element (CompileStoresBodyList → compileStoredBody
	// carriers) and runParallelBranch runs each via RunUnit on its fork.
	// The case above stays as a permanent pin.

	// UN-GRADUATED 2026-07-18: a direct rand-call check-prop GEN body
	// (`[r.int 1 9]`) reads a member fn from the opaque Map param `r` and
	// applies it to forward args — the fn-value-call boundary. A stored-param
	// unit cannot model the auto-dispatch (`r` is Any-typed; no member schema
	// is known), so it must DECLINE and the gen body interprets per iteration.
	// The earlier "graduated" unit silently returned the trailing arg — a
	// constant generator that made the trie/sort PBT suites vacuous or fail
	// (bisected to 853dcaa4). The decline is the sound state; the compilable
	// half (property bodies, non-member gen bodies) still runs on the VM.
	"p6/check-prop-body-on-vm": {
		why:       "member-fn-arrival gen body (r.int LO HI from an opaque Map param) is the fn-value-call boundary; the stored-param unit must decline and the interpreter owns the auto-dispatch",
		failsWith: "unattributed interpreter entries",
	},
}
