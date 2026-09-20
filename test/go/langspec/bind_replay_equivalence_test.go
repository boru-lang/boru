// bind_replay_equivalence_test.go answers the load-bearing question the bind
// twins rest on, before any twin op is written.
//
// §6.5's regime is "replay, never re-execution": roll the runtime-visible
// binding transitions back to the pre-check snapshot, and let a twin re-install
// each one at its own source position. That is only sound if replaying the
// recorded transitions actually REPRODUCES the state the check pass left. If it
// does not — if the ledger is missing a transition, or records them out of
// order — then every twin op written against it inherits the gap, and the
// differential will report it as a mysterious binding divergence much later.
//
// So this checks the claim directly, with no opcode and no VM change: run a
// program's check pass, capture the per-name def DEPTHS it left, then verify
// that the ledger's own final depth for each name agrees.
//
// DEPTH is the right observable, and §6.5 says why: "a twin that re-installs at
// its source position must leave the same depth the check pass did, or
// shadowing and a later `undef` expose a different binding". A ledger whose
// depths are internally coherent is a ledger whose replay lands each binding at
// the level the interpreter would.
//
// The check is SELF-CONTAINED, and deliberately so. It replays each name's
// transitions symbolically — a def raises that name's depth by one, an undef
// lowers it, a type-install raises it — and asserts the running count matches
// the depth the ledger recorded at every step. That catches both failure modes
// a twin replay would inherit: a MISSING transition (the count drifts below the
// record) and a DOUBLE-recorded one (it drifts above). Neither needs the live
// registry, so the test cannot be fooled by whatever else the pass left there.
//
// It is deliberately NOT a full replay. Re-installing requires the identical
// binding object (§6.5 — a module in particular is imported once and its twin
// re-binds the instance already produced), and that object belongs to a
// Program-level table the twin ops will own, not to this measurement. What can
// be established now is the STRUCTURE: every transition accounted for, in an
// order whose depths compose.
package langspec

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// bindDelta is the depth change a transition makes to its own name. A
// BindDefReplace is a drop-then-push and nets ZERO — that is the whole reason
// it is a kind of its own rather than a BindDef — and a BindSigUndef removes
// one entry (possibly mid-stack), so it nets -1 like an undef.
func bindDelta(k core.BindKind) int {
	switch k {
	case core.BindUndef, core.BindSigUndef:
		return -1
	case core.BindDefReplace:
		return 0
	}
	return +1
}

// syntheticBranchArmSources are TOP-LEVEL branch-arm defs, which the corpus does
// not contain. lang/spec holds exactly three `if …[def …]` rows and all three
// sit inside a fn body, where FnBodyDepth suppresses them — so this gate read a
// clean 0 over 7644 rows while the shape it exists to size, NUR110's, was
// double-recorded: the arm's own speculative install AND the joined binding
// InstallJoinedDefs leaves behind, both at the same depth. A twin replaying
// that would push the binding twice.
//
// These rows make the corpus's blind spot a covered one. They are not a
// substitute for corpus evidence; they are the case corpus evidence cannot
// reach.
var syntheticBranchArmSources = []string{
	`def c false  if c [def op 1] [0] end 1`,
	`def c true  if c [def op 1] [0] end 1`,
	`def c false  def op 0  if c [def op 1] [0] end op`,
	`def c false  if c [def op 1] [def op 2] end op`,
	`def c false  if c [def op 1] [0] end  if c [def op2 2] [0] end 1`,
	// List-form condition — the paren form `(n gt 0)` evaluates to a Boolean
	// and while's (List, List) signature declines it, so the paren spelling
	// never exercised a loop at all: its body list survived to the end-of-run
	// drain instead, and the row "passed" composition without ever measuring
	// the shape it was written for. The live-depth oracle caught it: a REAL
	// while leaves the loop-join pushes (n at 2, acc at 1) that AnalyseLoopBody
	// must ledger.
	`def n 3  while [n gt 0] [def acc n  def n (n sub 1)] end 1`,
	// A SIGNATURE undef of a plain two-overload fn removes the matching
	// entry — depth 2 to 1 — and must compose as its own -1 kind
	// (BindSigUndef). The corpus lacks the shape entirely: while it was
	// conflated into net-zero BindDefReplace, the composition gate read
	// clean while the ledger misstated the depth by one.
	`def f fn [[a:Integer] [Integer] [a]]  def f fn [[s:String] [String] [s]]  ` +
		`undef f (fnsig [[String] [String]])  f 1`,
	// The LOCKED counterpart: a sig-undef aimed at a word-extension clone
	// (which carries the locked base sigs) removes nothing and must note
	// nothing — a no-op is not a transition.
	`def Flag (refine Boolean)  def add fn [[a:Flag b:Flag] [Boolean] [a or b]]  ` +
		`undef add (fnsig [[Flag Flag] [Boolean]])  1`,
	// NARROWING-ONLY bodies (Codex P2, PR #418): a branch arm / loop body
	// that merely CONSUMES a dynamic name through a typed slot must leave NO
	// join push and NO ledger entry beyond the def itself — narrowing
	// preserves the value's ID, and an add whose ID equals the pre-binding's
	// is the pass's own refinement, not a binding the runtime leaves.
	`def m (do {a: 1})  def c false  if c [keys m drop] [0] end 1`,
	`def m (do {a: 1})  while [false] [keys m drop] end 1`,
}

// composeLedger replays one ledger symbolically and reports the incoherent
// transitions, formatted for the caller's log.
//
// Each name starts at whatever depth it had BEFORE its first recorded
// transition, derived from that entry: post-depth minus the delta the
// transition itself made. Builtins and any binding the pass never touched are
// irrelevant — a name with no transition has no twin either. After a drift the
// running count is RE-BASED, so one missing transition does not cascade into a
// count of every later transition for the same name.
func composeLedger(src string, led []core.BindTransition, where string) []string {
	var bad []string
	run := map[string]int{}
	seen := map[string]bool{}
	for _, tr := range led {
		d := bindDelta(tr.Kind)
		if !seen[tr.Name] {
			seen[tr.Name] = true
			run[tr.Name] = tr.Depth - d
		}
		run[tr.Name] += d
		if run[tr.Name] != tr.Depth {
			s := src
			if len(s) > 80 {
				s = s[:80] + "…"
			}
			bad = append(bad, fmt.Sprintf("%s name=%q recorded=%d composed=%d  %s",
				where, tr.Name, tr.Depth, run[tr.Name], s))
			run[tr.Name] = tr.Depth
		}
	}
	return bad
}

// TestBindLedgerBranchArmDepthsCompose is TestBindLedgerDepthsCompose over the
// synthetic rows above — the population the corpus cannot supply.
func TestBindLedgerBranchArmDepthsCompose(t *testing.T) {
	t.Parallel()
	for _, src := range syntheticBranchArmSources {
		a, err := lang.New()
		if err != nil {
			t.Fatal(err)
		}
		_, _, res, _ := a.CompileCheck(src)
		if len(res.BindLedger) == 0 {
			t.Errorf("no ledger entries for %q — the row records nothing, so it gates nothing", src)
			continue
		}
		for _, bad := range composeLedger(src, res.BindLedger, "synthetic") {
			t.Errorf("%s", bad)
		}
	}
}

func TestBindLedgerDepthsCompose(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	checked, incoherent := 0, 0
	var worst []string

	specWalk(t, func(t testing.TB, r specRow) {
		if len(r.Cells) < 2 {
			return
		}
		src := r.Input
		a, aerr := lang.New()
		if aerr != nil {
			return
		}
		_, _, res, _ := a.CompileCheck(src)
		if len(res.BindLedger) == 0 {
			return
		}
		bad := composeLedger(src, res.BindLedger, fmt.Sprintf("%s:%d", r.File, r.Line))

		mu.Lock()
		defer mu.Unlock()
		checked++
		incoherent += len(bad)
		for _, b := range bad {
			if len(worst) < 12 {
				worst = append(worst, b)
			}
		}
	})
	sort.Strings(worst) // the examples read the same whichever worker saw a row first

	t.Logf("bind-ledger depth coherence: %d rows with a ledger, %d incoherent transitions", checked, incoherent)
	for _, w := range worst {
		t.Logf("    %s", w)
	}

	if checked == 0 {
		t.Fatal("no rows produced a ledger — the test is measuring its own wiring")
	}
	// No ratchet and no allowance. An incoherent depth means a transition is
	// missing or double-recorded, and a twin replaying the ledger would land
	// that binding at the wrong level — the exact hazard §6.5 names. Fix the
	// ledger, never the number.
	if incoherent > 0 {
		t.Errorf("%d incoherent transitions: the ledger's depths do not compose, so a twin "+
			"replaying it would land bindings at the wrong level (§6.5)", incoherent)
	}
}
