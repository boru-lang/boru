// The engine-entry census — the T2 half of the compile-or-fallback gate.
//
// design/FULL-COMPILATION.0.md section 9 names this ratchet. The point it
// exists to make: `islandCeiling` counts only OpFallback spans, so a
// compiled program that re-enters the tree-walker by ANY other route —
// a value-window island (callDynamic's user-body arm, callDynFrame), the
// drift window, a raw-token InvokeBody, a predicate body through CallBoru,
// a busy-registry callback — passes that ceiling while still running the
// interpreter mid-program. Those entries were invisible. This counts them.
//
// What counts: an interpreter entry with an EMPTY attribution, observed
// while the compiled path runs a corpus row. What does not: entries the
// C4 carve-outs tag — "check-mode" (the compiler front end),
// "fallback:refusal" and "fallback:runtime-bail" (the sanctioned
// whole-program re-runs), "module-load". Those are declared interpreter
// use; an unattributed entry is the undeclared kind the end state forbids.
//
// The census rides the compile-or-fallback walk (compiled_fullcorpus_test.go)
// rather than opening a second corpus pass — same rule as compiled_census_test.go:
// one walk, one place the numbers come from. Only the COMPILED instance is
// armed; its interpreter twin is a reference oracle and its entries are not
// the subject.
package langspec

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// engineEntryCeiling is the maximum number of UNATTRIBUTED interpreter
// entries the compiled corpus walk may produce. Monotone DOWN only — it
// reaches 0 at Stage 9, when the last escape valve retires and the
// compiled lane executes without the tree-walker. Never raise it silently:
// a rise means a compiled program started re-entering the interpreter
// somewhere new, which is the regression this ratchet exists to catch.
//
// It counts ENTRIES, not rows, and that is deliberate: interpEntryRowCeiling
// is the row-normalised twin, and the two answer different questions (how
// many programs interpret, versus how much interpreting they do). The cost
// of the entry count is that it moves with corpus growth on a seam that is
// ALREADY ledgered — a new Test.property row runs its raw-quotation body
// about a hundred times, so one such row is about a hundred entries. That
// is not a new route, and a change adding such a row moves this ceiling
// WITH the row, naming the row and the seam in the history below, exactly
// as diagnosticParityCeiling's history names each corpus row that moved
// it. What stays forbidden is an unnamed rise, or a rise whose cause is a
// new seam rather than a new row on a known one.
const engineEntryCeiling = 366 // the REGRESSION ceiling (lanes_test.go; end state 0): 379 on 2026-09-17 — the corpus expansion (Engine.Run×379 with CallBoru×253 beneath it; the new fn-value islands, raw-token code bodies and boru:test quotation rows). History: 277 // 505 (2026-08-25, Stage-1 baseline) -> 281 (2026-09-14, the first lowering: measured three times at exactly 281 on c34a2fb, the tree that closed increment 59 — Engine.Run×281 with CallBoru×240 beneath it, most of those Test.property's ~100 invocations per row, so the ENTRY count is concentrated in two or three module-test.tsv rows while the interp-entry ROW census stands at 28. The ceiling had sat 80% above the live value for seventeen days, which is a ceiling that cannot catch a regression; it is now the live value, as a ratchet must be) -> 277 (2026-09-16, the seventy-first increment: a stored handler reads its module-scope deps live, so module-io.tsv's mount handlers stamp instead of falling to CallBoru — Engine.Run×277 with CallBoru×238 beneath it, measured on 65e9372) -> 366 (2026-09-18, NUR153 closed — one residual rule at every seam: a stored `=>` value applied through a native seam used to have its residual evaluated in the live frame, which is an Engine.Run entry this census counts; core.ResidualEvalsInFrame gives the tape rule everywhere and CallBoruNamed sweeps the deferred residual after teardown. Thirteen entries go, Engine.Run×366 with CallBoru×250 beneath it. Attributed by measurement across three heads, not inference: the same gate reports 379 at e04fa21, 366 at 3e15778, 366 at 5618223 — so the fall is NUR153's and the quoted-class commit moved none of it) -> 0 (Stage 9)

// deferCeiling is the maximum number of runtime bails (vmDefer activations)
// the compiled corpus walk may produce. A bail is the VM meeting a runtime
// surprise it has no compiled answer for — a dyn-scope miss, a poly no-match,
// an Impl-identity drift, a dyn-frame count mismatch — and resolving it by
// asking the caller to re-run the whole program on the interpreter. Monotone
// DOWN only; 0 at Stage 9, when every site has a native answer
// (design/FULL-COMPILATION.0.md section 6.10) and the mechanism deletes.
//
// vmDefer (eng/go/vm_defer.go) is the single chokepoint every reachable
// designed-defer site routes through, so each defer is seen exactly once
// with a stable site tag.
const deferCeiling = 8 // the REGRESSION ceiling (lanes_test.go; end state 0): 8 on 2026-09-17 — vm:poly-nout-drift×3, vm:rematch-matched×3 (the corpus expansion), vm:poly-no-match×2. History: 5 // 5 (2026-08-25, Stage-1 baseline) -> 0 (Stage 9)

// deferLocalCeiling is the second, weaker kind of bail — and it exists because
// a change made the difference measurable rather than theoretical.
//
// deferCeiling's own prose says what it counts: a bail "resolving it by asking
// the caller to re-run the whole program on the interpreter". Measured, all
// five of its bails do exactly that — the row comes back wasCompiled=false.
// The lens units (core/go/reach_unit.go) produce one that does not: a lens
// applied to a receiver its first segment cannot read reaches CALL_NATIVE_POLY
// with no match, and ApplyReach — which has a complete fallback of its own —
// catches the internal_error, runs the interpreted chain for that ONE
// application, and the program finishes COMPILED (wasCompiled=true).
//
// Those are not the same event, and one ratchet cannot hold both: counting
// them together would either forbid a change that removed 16 rows of
// interpretation, or quietly relax the number that guards whole-program
// re-runs. So the walk sorts each bail by what actually happened to its row,
// deferCeiling keeps its exact meaning and its exact number, and the local
// kind ratchets separately from 1.
//
// A row that produced BOTH kinds attributes all of its bails to the STRICTER
// census (wasCompiled is per row, not per bail) — the safe direction, and no
// corpus row does it today.
//
// Monotone DOWN, same as its sibling: 0 at Stage 9, when the poly no-match
// site can raise natively instead of deferring. It cannot today for a lens —
// PolyNoMatchSpec is a faithfulness proof the check pass records at a FAILED
// dispatch, and a lens body analysed with an `Any` receiver never fails one.
const deferLocalCeiling = 1 // 1 (2026-08-28, the lens units) -> 0 (Stage 9)

// deferCensus tallies runtime bails by site tag.
type deferCensus struct {
	mu     sync.Mutex
	bySite map[string]int
	total  int
}

func newDeferCensus() *deferCensus {
	return &deferCensus{bySite: map[string]int{}}
}

func (c *deferCensus) add(e lang.BailEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bySite[e.Site]++
	c.total++
}

func (c *deferCensus) report() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return renderTally(c.bySite)
}

func (c *deferCensus) assertCeiling(t *testing.T) {
	t.Helper()
	c.mu.Lock()
	total := c.total
	c.mu.Unlock()
	t.Logf("defer census: %d runtime bails on the compiled path (sites: %s)", total, c.report())
	gate(t, "runtime defers", total, 0, deferCeiling, false,
		"vmDefer activations on the corpus walk — the VM bailing to the whole-program interpreter re-run, by site: "+c.report())
}

// assertLocalCeiling is assertCeiling for the locally-resolved kind: the VM
// bailed, but the caller had its own fallback and the program stayed compiled.
func (c *deferCensus) assertLocalCeiling(t *testing.T) {
	t.Helper()
	c.mu.Lock()
	total := c.total
	c.mu.Unlock()
	t.Logf("defer census (locally resolved, program stayed compiled): %d (sites: %s)", total, c.report())
	gate(t, "locally-resolved defers", total, 0, deferLocalCeiling, false,
		"VM bails a caller's own fallback absorbed, the program staying compiled: "+c.report())
}

// renderTally renders a count map most-frequent-first, ties broken by name.
func renderTally(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s×%d", k, m[k])
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

// engineEntryCensus tallies unattributed interpreter entries by seam. The
// hook fires on the emitting goroutine and the corpus walk builds a fresh
// instance per row, so the accumulator guards itself (the -race lanes run
// this walk).
type engineEntryCensus struct {
	mu     sync.Mutex
	bySeam map[string]int
	total  int
}

func newEngineEntryCensus() *engineEntryCensus {
	return &engineEntryCensus{bySeam: map[string]int{}}
}

// add records one observation, keeping only the unattributed ones.
//
// The ceiling counts "Engine.Run" alone, because that seam is emitted once
// by every tree-walk (core/go/engine.go, first statement of Engine.Run) and
// exactly once — it is the ground truth for "the interpreter ran". The
// other seams are ROUTES to it (RunResolved, CallBoru and the two island
// seams each emit their own label and then reach Engine.Run), so counting
// them all would multiply-count one re-entry. They are kept for the
// breakdown, which is the diagnostic: it names which mechanism re-entered.
func (c *engineEntryCensus) add(e lang.InterpEntry) {
	if e.Attribution != "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bySeam[e.Seam]++
	if e.Seam == "Engine.Run" {
		c.total++
	}
}

// report renders the seam breakdown, most frequent first — the diagnostic
// that says WHICH mechanism re-entered, which is why the island chokepoints
// carry their own seams ("vm:island", "vm:island-resolved") instead of
// reporting only the generic "Engine.Run".
func (c *engineEntryCensus) report() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return renderTally(c.bySeam)
}

// assertCeiling reports the census and fails when it exceeds the ceiling.
func (c *engineEntryCensus) assertCeiling(t *testing.T) {
	t.Helper()
	c.mu.Lock()
	total := c.total
	c.mu.Unlock()
	t.Logf("engine-entry census: %d unattributed interpreter runs on the compiled path (routes: %s)", total, c.report())
	gate(t, "engine entries", total, 0, engineEntryCeiling, false,
		"unattributed interpreter runs on the compiled path, by seam: "+c.report())
}
