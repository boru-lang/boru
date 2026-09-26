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
// "fallback:compile failure" and "fallback:runtime-bail" (the sanctioned
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
const engineEntryCeiling = 185 // the REGRESSION ceiling (lanes_test.go; end state 0): 183 -> 185 on 2026-09-26 (NUR190 closed): fn-value.tsv L317/L318 — `m.f z`, `m get 'f' z` — each run one landing ISLAND (Engine.Run 1, vm:island-resolved 1: the `/q` capture of the word and the program after it, on the interpreter) where the run used to BAIL at the landing (the defer census falls 7 -> 5 by the same two rows); a known seam carrying the two rows, the trade of a failing run for an answering one. Nothing else moved (CallBoru 132, RunResolved 32, vm:island 6, InvokeCallback:callboru 5, runPooledSub 3); measured against 7fbd2a4. Before: 165 -> 183 on 2026-09-25 (the merge with the reverse-order NUR run): eighteen entries, every one on a ROW THAT RUN ADDED and on a known seam (Engine.Run, with RunResolved or vm:island beside it) — no existing row moved: callbacks.tsv L167–L171 (NUR166: a fn value a higher-order word applies opens its own frame, `do [args]` in its body) and L178, L180, L181 (NUR211: a named value's no-match past step 0 is the word's raise), fn-value.tsv L330, L334, L336 (NUR168: a def names the fn value it binds) and L349 (NUR158: a wrap word over a compiled closure), and user-types.tsv L366 (NUR167: a fn body's per-call type install under each) — all of them pinned for their verdict on both lanes, the entry being the seam their callback already takes; measured with BORU_LOG_CENSUS_ROWS=1 against main's merge base 00ec530 (interpEntryRowCeiling's entry of the same date names the same rows). Before: 164 -> 165 on 2026-09-25 (behave's quoted behaviour name declared CompileQuoteInert): code-bodies.tsv:L228 compiles now and its installed behaviour body runs once through the userBehavior wrapper on the registry (Engine.Run 1) — a known seam, named in interpEntryRowCeiling's entry of the same date; before: 166 -> 164 on 2026-09-24 (the keep-defs token body, NUR202 closed): a run-time token body that rebinds a name it reads no longer re-stamps past its budget and runs the rest on the interpreter; before: 172 -> 166 on 2026-09-24 (the flex-member map literal): the two canon.tsv rows (L39/L40, `Vm.run (canon v)` over a flex tree) ran their sub-program — `(flex {a:(flex [1])})`, a map literal whose paren member mints a flex — on the interpreter, three Engine.Run and two runPooledSub each (the sub-engine's run and its map members' pooled evaluations); the folded flex member now carries its recorded region result's identity, so the map assembles from its event (RecordMakeMap) and the sub-program compiles — Engine.Run 172 -> 166, runPooledSub 9 -> 5, CallBoru 132, RunResolved 16, vm:island-resolved 7, InvokeCallback:callboru 5 and vm:island 4 unchanged; measured with BORU_LOG_CENSUS_ROWS=1 against the previous head 6ccf827. Before: 281 -> 172 on 2026-09-24 (the boru:test bodies reach the VM): `Test.invoke` through the InvokeBody seam (its subject a hosted token body, not a fresh engine per invoke and per `run-spec` case), check-prop's raw bodies stamped at run time over the handler's own params typed by the inputs (native.StampBodySig — the property that leaves its input beneath its residual, the bodies read raw from a `Test.prop` spec; twenty iterations of a hosted property used to be twenty CallBoru frames and twenty Engine.Run), both shrinkers dispatching the property through the same carrier, the value-level shrinker's literal candidates answered without an engine run and the gen-program recorder's run attributed "stackform-record" — Engine.Run 281 -> 172, CallBoru 235 -> 132, RunResolved 14 -> 16 (`Test.invoke`'s undefined subjects, corpus-modules.tsv L165/L166, decline the stamp and take the seam's interpreter path: RunResolved where a fresh engine's Run was), runPooledSub 9, vm:island-resolved 7, InvokeCallback:callboru 5 and vm:island 4 unchanged; measured with BORU_LOG_CENSUS_ROWS=1 against the previous head 9b441a6. Before: 335 -> 281 on 2026-09-24 (S3's first slice, the run-time token body at the seam): the twenty-four rows that leave the interp-entry census (see interpEntryRowCeiling) each paid one Engine.Run and one RunResolved per application of their token body — Engine.Run 335 -> 281, RunResolved 67 -> 14, runPooledSub 10 -> 9 (code-bodies.tsv:L166's do body), CallBoru 235, vm:island-resolved 7, InvokeCallback:callboru 5 and vm:island 4 unchanged; measured with BORU_LOG_CENSUS_ROWS=1 against the previous head d75dd75. Before: 344 -> 335 on 2026-09-24 (the foreign-home fn value at the apply seam): the four rows that leave the interp-entry census (see interpEntryRowCeiling) — Engine.Run 344 -> 335 (each island and each CallBoru is one), CallBoru 239 -> 235 (the three module-fn dispatches of L100, L51 and L75's two elements), vm:island-resolved 10 -> 7 (L146, L100, L51), vm:island 6 -> 4 (L75's two elements), RunResolved 67, runPooledSub 10 and InvokeCallback:callboru 5 unchanged; measured with BORU_LOG_CENSUS_ROWS=1 against the previous head 017ea83. Before: 375 -> 344 on 2026-09-24 (the fn-util wrapper at the token seam, and the recorder's registry after a module body): the five rows that leave the interp-entry census (see interpEntryRowCeiling) — Engine.Run 375 -> 344, RunResolved 73 -> 67 (callbacks.tsv:L154's and module-composition.tsv:L94's three per element), runPooledSub 26 -> 10 (the two `$module` comparison rows, eight each), CallBoru 239 unchanged, nothing else moved (vm:island-resolved 10, vm:island 6, InvokeCallback:callboru 5); measured with BORU_LOG_CENSUS_ROWS=1 against the previous head be610d7, and per file with BORU_SPEC_FILES over the five files on both heads. Before: 392 -> 375 on 2026-09-24 (the def-bound computed fn read at a closure body's tail): the six rows that leave the interp-entry census (see interpEntryRowCeiling) each ran their body through the native's RunResolved seam once per element — Engine.Run 392 -> 375, RunResolved 90 -> 73, CallBoru 239 unchanged, nothing else moved (runPooledSub 26, vm:island-resolved 10, vm:island 6, InvokeCallback:callboru 5); measured with BORU_LOG_CENSUS_ROWS=1 against the previous head 2ffd07f. Before: 408 -> 392 on 2026-09-23 (the placed value inside a code body): the seven rows that leave the interp-entry census (see interpEntryRowCeiling) each ran their body through the native's RunResolved seam once per element — Engine.Run 408 -> 392, RunResolved 105 -> 90, CallBoru 240 -> 239 (bytecode-migrated.tsv:L113's module fn), nothing else moved; measured with BORU_LOG_CENSUS_ROWS=1 against the previous head 1ca30b1. Before: 406 -> 408 on 2026-09-23 (the recovery's window, NUR180): generics-fn.tsv:L55 enters the census as a graduated row (see interpEntryRowCeiling) — one Engine.Run and one RunResolved, its fold body a raw token body at the callback seam. Before: the REGRESSION ceiling (lanes_test.go; end state 0): 415 -> 406 on 2026-09-22 — the quotation-body container reads: the three rows that leave the interp-entry census (callbacks.tsv:L103, callbacks.tsv:L55, module-composition.tsv:L76 — see interpEntryRowCeiling) each ran their capture-free fn value through the stepping island once per element, an orphaned probe stamp having left the value's unit without a Program; with the stamp undone the fn-value seams run the unit VM-native (Engine.Run×406, CallBoru×240, RunResolved×103, vm:island 13 -> 6, InvokeCallback:callboru 7 -> 5; measured against a worktree at 1ae0a21). The three rows the increment compiles add no entry. 415 was: 418 -> 415 on 2026-09-22 — the curried chain: callbacks.tsv:L85 leaves the interp-entry census (see interpEntryRowCeiling), and its three elements were one Engine.Run + one RunResolved each (Engine.Run×415, CallBoru×242, RunResolved×103; measured against a worktree at 917ecf0: Engine.Run×418, RunResolved×106, callbacks.tsv 11 rows -> 10). Nothing else moved. 418 was: 416 -> 418 on 2026-09-22 — the dynamic-lead group: module-composition.tsv:L100 compiles (see interpEntryRowCeiling) and its apply of a fetched MODULE export runs the export's body through the re-step island, Engine.Run 1 + CallBoru 1; nothing else moved (Engine.Run×418, CallBoru×242, RunResolved×106). 416 was: 422 -> 416 on 2026-09-22 — S1b's apply shapes: the three `each ([f:Function] => [(f n)]) fs` rows that left the interp-entry census (interpEntryRowCeiling: callbacks.tsv:L54/L105, fold-map-filter.tsv:L200) each cost Engine.Run 1 + RunResolved 1, six entries, and their callbacks now run as closure units; no entry added (Engine.Run×416, CallBoru×241, RunResolved×106). Before: 422 on 2026-09-21 — NUR174/NUR175 move this ceiling in BOTH directions and it lands where it started. DOWN 2: the re-step landing gained the interpreter's ANONYMOUS-0-ARG PARK, so a fn-VALUE closure stays data instead of being invoked, and bytecode-migrated.tsv:L285 and callbacks.tsv:L150 stop entering — curried chains whose 1-param wrapper the landing invoked, had invokeFnValueClosure decline, and paid a RunResolved entry to step the body to the same 'stays data'. UP 2: fn-value.tsv:L319/L320, the two NUR175 0-RETURN witnesses (`def h fn [[] [] []] end … 5 m.f` and its `get` twin), which the landing now stands aside from because its one-result claim cannot express a member applied for its EFFECT. They take today's residual apply, which islands — vm:island 11 -> 13, a KNOWN seam carrying a new row, not a new route. Both directions measured row by row with BORU_LOG_CENSUS_ROWS=1 against b39be40. Before: the REGRESSION ceiling (lanes_test.go; end state 0): 422 on 2026-09-19 — S1b's SECOND increment (a computed fn value resolves at a forward slot: the collection seat and the `/v` read consult the fn-carrier side table). Seven corpus rows that FAILED TO COMPILE now compile, so the walk reaches them for the first time, and one of the seven still enters the interpreter: callbacks.tsv:L154, `FnUtil.compose`'s wrapper over a three-element list — Engine.Run +3, RunResolved +3, one pair per element (measured directly: the row makes exactly three of each; the other six rows are fully native, `each a5/v [1 2 3]` making none). That is a compile FAILURE turning into a compiled row with an attributed seam, the S1a trade in miniature, and the wrapper is on S1b's owed list. 419 was: 419 on 2026-09-19 — S1b's first increment (the fn-VALUE seam made native, the lazy detached stamp): the twenty-five fn-value callback rows S1a had landed on the RunResolved seam run their units on the VM, so Engine.Run×419 with CallBoru×244 and RunResolved×109 beneath it (from Engine.Run×489, CallBoru×244, RunResolved×179 — seventy entries, the same rows interpEntryRowCeiling names). Before: 489 on 2026-09-19 — S1a of design/FULL-COMPILATION-REPLAN.0.md (each/fold/scan/filter declare CompileDynBody): the fifty corpus rows the ambiguous-overload gate used to DECLINE now compile as dyn-body CALL_NATIVE dispatches whose handler runs the callback through the RunResolved seam once per element, so Engine.Run×489 with CallBoru×244 and RunResolved×179 beneath it (from Engine.Run×366, CallBoru×250, RunResolved×56). Same rows as interpEntryRowCeiling's 52 -> 102 move (interp_entry_census_test.go names them by file and line); S1b retires the seam by lowering the re-matched overload's body natively. Before S1a: 379 on 2026-09-17 — the corpus expansion (Engine.Run×379 with CallBoru×253 beneath it; the new fn-value islands, raw-token code bodies and boru:test quotation rows). History: 277 // 505 (2026-08-25, Stage-1 baseline) -> 281 (2026-09-14, the first lowering: measured three times at exactly 281 on c34a2fb, the tree that closed increment 59 — Engine.Run×281 with CallBoru×240 beneath it, most of those Test.property's ~100 invocations per row, so the ENTRY count is concentrated in two or three module-test.tsv rows while the interp-entry ROW census stands at 28. The ceiling had sat 80% above the live value for seventeen days, which is a ceiling that cannot catch a regression; it is now the live value, as a ratchet must be) -> 277 (2026-09-16, the seventy-first increment: a stored handler reads its module-scope deps live, so module-io.tsv's mount handlers stamp instead of falling to CallBoru — Engine.Run×277 with CallBoru×238 beneath it, measured on 65e9372) -> 366 (2026-09-18, NUR153 closed — one residual rule at every seam: a stored `=>` value applied through a native seam used to have its residual evaluated in the live frame, which is an Engine.Run entry this census counts; core.ResidualEvalsInFrame gives the tape rule everywhere and CallBoruNamed sweeps the deferred residual after teardown. Thirteen entries go, Engine.Run×366 with CallBoru×250 beneath it. Attributed by measurement across three heads, not inference: the same gate reports 379 at e04fa21, 366 at 3e15778, 366 at 5618223 — so the fall is NUR153's and the quoted-class commit moved none of it) -> 489 (2026-09-19, S1a: the gradual-Any overload commitment lands the fifty declined callback rows on the dyn-body seat — Engine.Run×489, RunResolved×179 — the G-lane-first landing the re-plan names, to be retired by S1b) -> 419 (2026-09-19, S1b-1: the fn-value seam made native — RunResolved×109; the fn-value rows leave the seam, the token-body rows stay for S3) -> 0 (Stage 9)

// deferCeiling is the maximum number of runtime bails (vmDefer activations)
// the compiled corpus walk may produce. A bail is the VM meeting a runtime
// surprise it has no compiled answer for — a dyn-scope miss, a poly no-match,
// an Impl-identity drift, a dyn-frame count mismatch. It used to resolve by
// asking the caller to re-run the whole program on the interpreter; since
// 2026-09-19 nothing re-runs and it FAILS the program instead. Same event,
// same count, worse consequence — which is the honest one. Monotone DOWN
// only; 0 at Stage 9, when every site has a native answer
// (design/FULL-COMPILATION.0.md section 6.10) and the mechanism deletes.
//
// vmDefer (eng/go/vm_defer.go) is the single chokepoint every reachable
// designed-defer site routes through, so each defer is seen exactly once
// with a stable site tag.
const deferCeiling = 5 // the REGRESSION ceiling (lanes_test.go; end state 0): 7 -> 5 on 2026-09-26 (NUR190 closed): fn-value.tsv L317/L318 — `m.f z`, `m get 'f' z` — no longer bail at the landing (vm:landing-quote-claim): the `/q` capture takes the landing's island. Before: 10 -> 7 on 2026-09-25 (the strict-Any dyn-body recovery): fold-map-filter.tsv L246–L248, fold/each/scan over a class member's Any-typed list, run natively through the dyn-body poly re-match the recovery records instead of bailing at the rematch trap (vm:rematch-matched×3 gone); before: 8 -> 10 on 2026-09-24 (NUR190's open halves deferred, the maintainer's call): fn-value.tsv:L317/L318 (`m.f z`, `m get 'f' z` — a `/q` slot CAPTURES the following word) BAIL at the landing's walk (vm:landing-quote-claim×2) where they passed by coincidence (z's result is its own atom) and the same shape answered wrong off-corpus (`m.f y` was `[42 42]` for `[y]`); a compile-time decline would have been over-wide (no static model tells a `/q` slot from a typed slot's barrier), and the deferral is kept on the per-file runtime-defers ledger (runtime_defers.tsv, runtime_defer_ledger_test.go) as the maintainer asked. Before: 8 on 2026-09-17 — vm:poly-nout-drift×3, vm:rematch-matched×3 (the corpus expansion), vm:poly-no-match×2. History: 5 // 5 (2026-08-25, Stage-1 baseline) -> 0 (Stage 9)

// deferLocalCeiling is the second, weaker kind of bail — and it exists because
// a change made the difference measurable rather than theoretical.
//
// deferCeiling counts a bail that becomes the RUN's failure. The lens units
// (core/go/reach_unit.go) produce one that does not: a lens applied to a
// receiver its first segment cannot read reaches CALL_NATIVE_POLY with no
// match, and ApplyReach — which has a complete fallback of its own — catches
// the internal_error, runs the interpreted chain for that ONE application,
// and the program finishes and ANSWERS.
//
// The discriminator used to be wasCompiled, because a bail the caller could
// not absorb re-ran the whole program and came back wasCompiled=false. That
// stopped being true on 2026-09-19: a bailed program now comes back
// wasCompiled=TRUE with an error, so the walk sorts on whether the row
// produced a RESULT. Same two kinds, same question — "did the program
// survive it" — asked of something that still answers it.
//
// Those are not the same event, and one ratchet cannot hold both: counting
// them together would either forbid a change that removed 16 rows of
// interpretation, or quietly relax the number that guards whole-program
// re-runs. So the walk sorts each bail by what actually happened to its row,
// deferCeiling keeps its exact meaning and its exact number, and the local
// kind ratchets separately from 1.
//
// A row that produced BOTH kinds attributes all of its bails to the STRICTER
// census (the verdict is per row, not per bail) — the safe direction, and no
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
		"vmDefer activations on the corpus walk — the VM abandoning the run, by site: "+c.report())
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
