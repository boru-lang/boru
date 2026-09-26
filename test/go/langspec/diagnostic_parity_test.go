// Diagnostic parity between the two analysis passes — the gate NUR103
// showed was missing.
//
// design/FULL-COMPILATION.0.md section 6.9(4) states the alignment
// property the whole design rests on, and its clause (b) is that
// diagnostics stay identical whether or not a program is being compiled.
// The static pass is one abstracted interpreter over carriers; its verdict
// on a program is a property of the program, not of what the caller
// intends to do with the verdict.
//
// It is not currently true. NUR103 records a program that `boru check`
// reports clean and the compile pass declines with `undefined_word` — a
// divergence a user cannot diagnose, because the tool they would reach for
// says their program is fine. Nothing measured how widespread that is,
// because both passes were only ever compared against the RUNTIME, never
// against each other.
//
// This walks the corpus running both passes over each row and counts the
// rows whose diagnostic sets differ. Like the other censuses it ratchets:
// the count may only fall, and it reaches zero when section 6.9(3)'s
// !Compiling forks are collapsed or proven neutral.
package langspec

import (
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// diagnosticParityCeiling is the number of corpus rows whose diagnostics
// differ between a plain check and a compile-armed check. Monotone DOWN
// only; 0 when the two passes cannot disagree (Stage 8).
//
// 318 of 7568 corpus rows at the baseline, 317 of 7569 after the first row
// was fixed, 318 of 7572 once NUR104's spec rows landed, 319 of 7574 with
// the Stage-4 forward-barrier pair, counting FINDINGS
// only; a
// further 260 rows differ on informational advisories, tracked separately
// because some of those are pass-specific BY DESIGN (see diagSet). The shapes are structured, not
// noise, and they run in BOTH directions:
//   - plain-only findings: no_signature suppressed while compiling (a
//     documented fork, check_recovery.go), unreachable_branch, and the
//     module_body_executed_in_check info;
//   - armed-only findings: redundant_guard, case_not_exhaustive, and —
//     the NUR103 shape — undefined_word on programs plain check calls
//     clean;
//   - armed DUPLICATES: 57 rows where one plain diagnostic becomes two.
//
// Some are deliberate; the design requires each to be collapsed or proven
// diagnostic-neutral (section 6.9(3)), which is what drives this to zero.
// 317 -> 318, same day: NUR104 added three spec rows to
// `edge-dispatch-3.tsv` and one of them diverges. Measured, not guessed —
// `def f fn [[o:{a:(Integer tor String)}][Any][o.a]] f {a:true}` reports
// `no_signature/f` on the plain pass and NOTHING on the armed one, while
// the program compiles. That is the documented no_signature suppression,
// the class already carrying 41 rows, and it is invisible to the user:
// both lanes surface the identical error through the CLI pre-flight, and
// both decline the call. A ratchet that only ever falls would forbid adding
// ERROR rows to the corpus, which is the wrong incentive; what it must
// forbid is a divergence nobody accounted for. This one is accounted for.
//
// 318 -> 319, same day, and accounted for the same way. The Stage-4 region
// probe added two rows to `forward-barrier.tsv` §9 to tell the two
// stranded-forward texts apart — both are signature_error, so every prior
// row matched only `ERROR:signature` and NOTHING distinguished them. Of the
// two, exactly one diverges, and it is the `end` row:
// `def g fn [[a:Any b:Any] [Any] [add a b]] g 1 end def x 5 x` reports
// `no_signature/g` on the plain pass and nothing on the armed one. That is
// the documented no_signature suppression again — the single largest shape
// in this ledger, already carrying 29 `plain=no_signature/g armed=` rows —
// so the checker's behaviour is unchanged and the count moved only because
// the corpus grew. Its sibling (the same program WITHOUT the `end`) does
// not diverge: it RAISES on the check pass rather than reporting a finding,
// and a raise is not a finding on either side.
//
// 319 -> 320, and accounted for the same way a third time. The Apply kernel's
// return-contract fix added two rows to `fn-value.tsv` §12, and exactly one
// diverges:
// `def bad fn [[n:Any][Integer][n]] end def mk fn [[][Function][bad/v]] end ((mk) 'str')`
// reports `type_error/bad` on the plain pass and nothing on the armed one,
// while the program compiles and raises the identical error at run time. That
// is the LOST-UNDER-COMPILATION class — the checker stricter than the compiler,
// the largest shape in this ledger — and it is the benign direction: the
// finding is real, `boru check` surfaces it, and the compiled program accepts
// nothing silently. The whole point of the row is that it now RAISES where it
// used to answer 'str'. Its sibling, the conforming `okr` row, is clean on
// both passes.
const diagnosticParityCeiling = 354 // the REGRESSION ceiling (lanes_test.go; end state 0): 352 -> 354 on 2026-09-26 (the merge of main's #511 with the reverse-order NUR run; BORU_LOG_PARITY_ROWS=1 on the merged tree and on the run's head 3cd4362, row for row): +2 — main's two rows, edge-quote-1.tsv:L28 `quote [add 1 2] get 0` and edge-quote-3.tsv:L56 `… macroexpand (tw2 7) get 1`, which join the documented no_signature-suppression shape (the plain check now re-steps the word node `get` hands back and reports add's genuine no-match; the armed pass suppresses no_signature while compiling and bakes the same no-match as the trap both lanes raise). Nothing else moved. Main's #511: 348 -> 349 (measured 347 at 89dd499 and 349 after, those same two rows). Before: 350 -> 352 on 2026-09-26 (NUR244 closed; BORU_LOG_PARITY_ROWS=1 against d493ef4 and its parent 0663474, row for row): +2 — control.tsv:L207 (`def g fn […] if false [def g fn […]] g 1`, graduated from frontier-conditional-fn-shadow.tsv:15, which this walk never read) and :L208 (its def-bound `def c false … if c` twin) report `plain=unreachable_branch/if armed=` for their CONSTANT condition — the single largest shape here, the one control.tsv:L144 and every constant-condition row already contribute; the condition IS the rows' point (the arm a decided condition skips). Nothing else moved. Before: 349 -> 350 on 2026-09-26 (NUR100 closed; BORU_LOG_PARITY_ROWS=1 against the PR head 7fbd2a4, row for row): +1 — fnpred.tsv:L113 (`def f fn [[q:P] [Any] [q]]  f 0` over a two-overload fnpred), NUR100's new negative pin, reports `plain=no_signature/f armed=` — the documented no_signature suppression on the compiling pass, apply.tsv:L64/L65's class; nothing else moved. (Between 129e591 and 7ffbd14 NUR064's service `add` check half panicked over a short recovery window and masked three rows of the same class the ledger always carried — edge-forward-1.tsv:L144, error.tsv:L33, modifiers.tsv:L63 — which its length guard restored.) Before: 348 -> 349 on 2026-09-26 (NUR078 closed; BORU_LOG_PARITY_ROWS=1 on 54200b3, the committed head and this tree): +3 — the migrated bare-spelling ERROR rows each-variants.tsv:L198 (`each f [1 2 3]`), fn-value.tsv:L256 (`typeof (hold dbl)`) and path-modifier.tsv:L67 (`wa {x:1} sf`) report `plain=no_signature armed=` — a bare fn name CALLS now, so the plain check reports the no-match and the compiling pass suppresses it (the documented no_signature suppression, the apply.tsv:L64/L65 class); +3 — path-modifier.tsv:L77, :L85 and :L94 (`def m {d:div/v} end m.d 0 10` and its `/s` / `/u` twins) report `plain=arith_error armed=` since NUR112 (the plain check applies a stored fn member, so it folds the division by zero the compiling pass leaves to the run); -2 — callbacks.tsv:L147 and module-composition.tsv:L144 stop diverging with NUR089 (an analysed body binds a fn-valued param as the run does, so the plain check's false `no_signature/f` is gone). The committed head measured 346 (345 at 54200b3), under the ceiling. Before: 349 -> 348 on 2026-09-23 (the recovery's window, NUR180): generics-fn.tsv:L55's compile-armed check no longer stops at a FALSE `undefined word: value` — the recovery's stack-first window had left the written key unconsumed — so the two passes agree on it. Before: the REGRESSION ceiling (lanes_test.go; end state 0): 349 = 349 on 2026-09-22 (S1b's apply shapes) — for one measurement callbacks.tsv:L139 (`def hof2 fn [[f:Function][Integer][(f ([n:Integer] => [n add 1]))]] …`) diverged (plain=type_error/hof2, armed clean): the compile-armed pass records the lead window over a lambda literal (RecordDynApplyLead) while the plain surface's checkModeParenFnCollapse still excluded a fn-valued argument and left the window un-collapsed, flagging a false return-type error — fixed the same day on the plain surface (an inert fn value collapses under the lead there too), and the row list is identical to the previous head's, row for row (BORU_LOG_PARITY_ROWS=1 on both). Before: 353 -> 349 on 2026-09-22 — the branch-carried def (compiler/go/branch_carried.go; design/FULL-COMPILATION-HANDOFF.0.md "S5 — the branch-carried def"): fn-locals-scope.tsv:L178/L179 (`r add 10` after a rebinding branch) and :L180/L181 (`a mul 10`) compile, and the `type_error/f` the plain check reported for an operand the compiling pass could not seat is gone with the seat — exactly the four rows the 2026-09-21 note below said would leave together. Before: 351 -> 353 on 2026-09-21 — the nine paired FALSE-PATH witnesses added to the fn-locals-scope §6 arm-binding cluster. DEBT WRITTEN DOWN, NOT DEBT ADDED: all eight of that cluster's rows ran the THEN path, so it could not tell a real arm-binding join from a lowering that always picks the then-arm, and the pairs make it able to prove what it claims. Exactly TWO of the nine diverge, and both were MEASURED rather than assumed (BORU_SPEC_FILES=fn-locals-scope.tsv, the per-row log): fn-locals-scope.tsv:L179 (`type_error/f`) and :L181 (`type_error/f|unreachable_branch/if`) — the two OPERAND-spelling rows, whose decline is `operand of unknown provenance … at add`/`at mul`. Each mirrors a pre-existing armed-only twin in this same ledger, L178 and L180, diagnostic for diagnostic: the pair of an armed-only row is armed-only too. The other seven new rows (L168, L170, L172, L174, L176, L177, L183) do not diverge at all — including L172 and L174, which carry the same literal-`false` condition as L181, so the constant-condition `unreachable_branch` is NOT what moved this: `type_error/f` is. Falls by two, and takes L178/L180 with it, when the arm-binding JOIN lands (design/SESSION-HANDOVER.0.md, "Where the join seats"). Before: 351 on 2026-09-19, S1b-2 (a computed fn value def-bound at the top level resolves at a forward slot and at a `/v` read: the collection seat and stepWordVal consult the fn-carrier side table, so `each f/v xs` over a factory's result dispatches instead of declining "unmatched dispatch recovered") — the seven rows it compiles are callbacks.tsv:L82/L154, each-variants.tsv:L203, fold-map-filter.tsv:L73/L227/L229 and module-composition.tsv:L95 — each stops diverging because both passes now type it the same way. Before: 358 on 2026-09-17 — the corpus expansion added 38 diverging rows in the shapes this ledger already carries in bulk (unused_def and fn_body_error on the armed pass; unreachable_branch and no_signature on the plain one), named by BORU_LOG_PARITY_ROWS=1. History: 320 // 318 (2026-08-26, Stage-1 baseline) -> 317 (NUR103 record-field fix) -> 318 (+3 NUR104 spec rows, one diverging) -> 319 (+2 Stage-4 forward-barrier rows, one diverging) -> 320 (+2 fn-value §12 rows, one diverging) -> 321 (+1 twenty-seventh-increment row, the graduated apply-over-a-gradual-lead negative twin, diverging: the plain check flags its runtime no-match at the call over the concrete rule map, the compiled unit's carrier analysis does not repeat it — the "lost under compilation" class) -> 317 (the unreachable_branch attribution fix: a constant-condition dead branch is a claim about the CODE, so it is no longer emitted from a body analysis SPECIALISED to one call shape — CheckState.CallShapeDepth. Four corpus rows stop diverging because they stop being flagged at all, each proven a false positive by execution: bytecode-migrated.tsv:84, generics-fn.tsv:48, and recursion.tsv:78/:79, whose "dead" arms are the base cases `MR.fac 10 1` and `MR.aev 9` actually return through. The ratchet only falls, and this is the fall) -> 0 (Stage 8) -> 320 (2026-09-10, the forty-sixth and fiftieth increments' corpus rows: THREE more rows diverge and the checker's behaviour is unchanged — each falls into a shape this ledger already carries in bulk. apply.tsv:L64 `p apply $.name` and :L65 `[10 20 30] apply $.1` (the forty-sixth increment's graduated negatives) report `plain=no_signature/apply armed=` — the documented no_signature suppression on the compiling pass, which L37/L38 of the same file already carry; control.tsv:L144 `while [true] [ (7 add 2) if true [break] [5] end ] end 'x'` (the fiftieth increment's break-trims-the-round witness) reports `plain=unreachable_branch/if armed=` for its CONSTANT condition, the single largest shape here at 86 rows and one every constant-condition row in control.tsv §1/§6 already contributes. Named rather than counted because a ratchet that moves without naming what moved it stops being evidence)

// armedOnlyCeiling is the sharpest of the three classes: rows the plain
// check calls clean and the compile-armed pass finds fault with. It is the
// one a user CANNOT diagnose, because the tool they would reach for
// reports the program fine — NUR103's shape. Monotone DOWN only.
//
// The other two classes are less severe and tracked in the log rather than
// gated: 41 rows where the armed pass drops a diagnostic but DECLINES, so
// the finding still reaches the user through the compile failure reason (the
// documented no_signature suppression), and 272 where a check finding
// vanishes and the program compiles anyway — the checker being stricter
// than the compiler, which is a false-positive surface rather than a
// silent-acceptance one.
// 5 -> 4, 2026-08-26: `edge-dispatch-3.tsv:L56` — a field read from a
// STRUCTURAL-RECORD parameter — is fixed. The inline record pattern's field
// type words now resolve at sig install (ResolveSigRecordFields), so the
// schema-bearing param carrier no longer narrows the read to dynamic(Word),
// and the step loop no longer dispatches a word-typed CARRIER as a nameless
// token. NUR103 has the full trace; its `h2` half is a different defect and
// is not among these four.
const armedOnlyCeiling = 8 // the REGRESSION ceiling (lanes_test.go; end state 0): 9 -> 8 on 2026-09-23 (the recovery's window, NUR180): generics-fn.tsv:L55 — `boru check` called it clean and compiling stopped at a false `undefined word: value`; it compiles now. Before: the REGRESSION ceiling (lanes_test.go; end state 0): 13 -> 9 on 2026-09-22 — the branch-carried def: the four fn-locals-scope §6 operand-spelling rows (L178–L181) compile, so `boru check` calling them clean is no longer a call the compiler contradicts. Before: 11 -> 13 on 2026-09-21 — the nine paired FALSE-PATH witnesses added to the fn-locals-scope §6 arm-binding cluster. DEBT WRITTEN DOWN, NOT DEBT ADDED: all eight of that cluster's rows ran the THEN path, so it could not tell a real arm-binding join from a lowering that always picks the then-arm, and the pairs make it able to prove what it claims. Exactly TWO of the nine diverge, and both were MEASURED rather than assumed (BORU_SPEC_FILES=fn-locals-scope.tsv, the per-row log): fn-locals-scope.tsv:L179 (`type_error/f`) and :L181 (`type_error/f|unreachable_branch/if`) — the two OPERAND-spelling rows, whose decline is `operand of unknown provenance … at add`/`at mul`. Each mirrors a pre-existing armed-only twin in this same ledger, L178 and L180, diagnostic for diagnostic: the pair of an armed-only row is armed-only too. The other seven new rows (L168, L170, L172, L174, L176, L177, L183) do not diverge at all — including L172 and L174, which carry the same literal-`false` condition as L181, so the constant-condition `unreachable_branch` is NOT what moved this: `type_error/f` is. Falls by two, and takes L178/L180 with it, when the arm-binding JOIN lands (design/SESSION-HANDOVER.0.md, "Where the join seats"). Before: 11 on 2026-09-19, S1b-2 (a computed fn value def-bound at the top level resolves at a forward slot and at a `/v` read: the collection seat and stepWordVal consult the fn-carrier side table, so `each f/v xs` over a factory's result dispatches instead of declining "unmatched dispatch recovered") — the seven rows it compiles are callbacks.tsv:L82/L154, each-variants.tsv:L203, fold-map-filter.tsv:L73/L227/L229 and module-composition.tsv:L95. Five of the seven were armed-only rows — `boru check` called them clean while compiling declined — and they are exactly the five the diagnostic-surface ledger named under its now-graduated `unused_def` class (diag_surface_test.go): the dispatch declined before the read could credit its def. Before: 16 on 2026-09-17 — the corpus expansion added 12 armed-only rows (fn-locals-scope ×7, fold-map-filter ×2, callbacks, each-variants, module-composition, generics-fn, edge-quote-1, case ×2 — the test names each); checker debt the new idioms exposed. History: 4 // 5 (2026-08-26) -> 4 (NUR103 record-field fix) -> 0 (Stage 8)

// diagKey renders a diagnostic's identity for set comparison: the code and
// the word it is about. Detail text is deliberately excluded — it embeds
// positions and inferred types that legitimately differ in phrasing
// between passes; what must not differ is WHICH findings exist.
func diagKey(d lang.CheckDiagnostic) string {
	return d.Code + "/" + d.Word
}

// diagSet collects the FINDINGS — errors and warnings. Informational
// entries are excluded on purpose, because some of them are advisories
// about the ANALYSIS rather than findings about the program, and those
// legitimately differ between passes. The load-bearing example is
// `module_body_executed_in_check`, which warns that `boru check` executed
// a module body the user did not ask it to run; under compilation that
// execution IS the program's own, so there is nothing to advise about and
// the emitter is explicitly scoped to a pure check pass
// (lang/go/native/native_module_module.go). That is section 6.9(3)'s
// "proven diagnostic-neutral" disposition, not a defect, and a gate that
// counted it would be demanding the wrong thing.
//
// Info-only divergence is still counted and reported separately, so a NEW
// informational fork cannot hide behind this exclusion.
func diagSet(ds []lang.CheckDiagnostic) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		if d.Severity == "info" || d.Severity == "" {
			continue
		}
		out = append(out, diagKey(d))
	}
	sort.Strings(out)
	return out
}

// infoSet is the excluded half, tracked so it cannot drift unwatched.
func infoSet(ds []lang.CheckDiagnostic) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		if d.Severity == "info" || d.Severity == "" {
			out = append(out, diagKey(d))
		}
	}
	sort.Strings(out)
	return out
}

func TestDiagnosticParityAcrossPasses(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var rows, diverged, infoDiverged int
	var armedOnly, carriedByCompileFailure, lostUnderCompile int
	var armedOnlyRows []string
	byShape := map[string]int{}
	var examples []string

	specWalk(t, func(t testing.TB, r specRow) {
		if len(r.Cells) < 2 {
			return
		}
		input := r.Input
		mu.Lock()
		rows++
		mu.Unlock()

		ap := newDifferentialInstance(t)
		plain, perr := ap.Check(input)
		if perr != nil {
			return // a pass that cannot run is not a parity question
		}
		ac := newDifferentialInstance(t)
		prog, _, armed, cerr := ac.CompileCheck(input)
		if cerr != nil {
			return
		}
		declined := prog == nil

		infoDiffers := strings.Join(infoSet(plain.Diagnostics), ",") != strings.Join(infoSet(armed.Diagnostics), ",")
		p, c := diagSet(plain.Diagnostics), diagSet(armed.Diagnostics)
		findingsAgree := strings.Join(p, ",") == strings.Join(c, ",")

		mu.Lock()
		defer mu.Unlock()
		if infoDiffers {
			infoDiverged++
		}
		if findingsAgree {
			return
		}
		diverged++
		// Classify by what the USER sees, which is the property that
		// matters. A finding the armed pass drops is not lost if that
		// pass DECLINED — the compile failure reason carries it, which is
		// exactly why no_signature is suppressed while compiling
		// (check/go/check_recovery.go: emitting it there would mask the
		// specific reason as the generic sentinel). What is serious is a
		// finding only ONE lane surfaces at all.
		switch {
		case len(c) > len(p):
			armedOnly++ // the NUR103 class: clean to `boru check`, declined by the compiler
			// Few enough to name. Listing them is the difference between
			// a ratchet and a worklist.
			armedOnlyRows = append(armedOnlyRows,
				r.Key()+"  "+strings.Join(c, "|")+"  "+firstNRunes(input, 70))
		case declined:
			carriedByCompileFailure++ // dropped as a diagnostic, still reported as a compile failure
		default:
			lostUnderCompile++ // `boru check` errors that vanish AND the program compiles
		}
		// BORU_LOG_PARITY_ROWS=1 names every diverged row and both
		// passes' findings. The ceiling is a ratchet whose every past
		// move was justified by naming the exact row that moved it, and
		// re-deriving that row by hand across a 7,700-row corpus is the
		// step this switch removes. Mirrors BORU_LOG_CENSUS_ROWS
		// (interp_entry_census_test.go) and BORU_LOG_UNFLAGGED
		// (check_accuracy_test.go).
		if os.Getenv("BORU_LOG_PARITY_ROWS") != "" {
			t.Logf("PARITY ROW %s plain=%s armed=%s: %s",
				r.Key(), strings.Join(p, "|"), strings.Join(c, "|"),
				firstNRunes(input, 90))
		}
		shape := "plain=" + strings.Join(p, "|") + " armed=" + strings.Join(c, "|")
		byShape[shape]++
		if len(examples) < 5 {
			examples = append(examples, r.Key()+"  "+firstNRunes(input, 60))
		}
	})
	sort.Strings(armedOnlyRows) // the worklist reads the same whichever worker saw a row first

	t.Logf("diagnostic parity: %d rows, %d diverged on FINDINGS, %d on info-only advisories", rows, diverged, infoDiverged)
	t.Logf("  by user impact: %d armed-only (clean to check, declined compiling — the NUR103 class), %d carried by the compile failure reason instead, %d lost under compilation (check errors that vanish while the program compiles)",
		armedOnly, carriedByCompileFailure, lostUnderCompile)
	for _, ex := range examples {
		t.Logf("  e.g. %s", ex)
	}
	for _, r := range armedOnlyRows {
		t.Logf("  armed-only: %s", r)
	}
	gate(t, "armed-only diagnostics", armedOnly, 0, armedOnlyCeiling, false,
		"programs `boru check` calls clean and compiling FAILS — a user cannot diagnose them (NUR103)")
	gate(t, "diagnostic parity divergences", diverged, 0, diagnosticParityCeiling, false,
		"rows whose findings differ between the plain and the compile-armed check — the checker's verdict depends on who is asking (NUR103); top shapes: "+strings.ReplaceAll(topShapes(byShape, 3), "\n", "; "))
}

// topShapes renders the n most frequent divergence shapes. The full map
// is hundreds of entries; a failure needs the pattern, not the census.
func topShapes(m map[string]int, n int) string {
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
	if len(keys) > n {
		keys = keys[:n]
	}
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = "  " + itoa(m[k]) + "x  " + k
	}
	return strings.Join(lines, "\n")
}

// firstNRunes truncates for log lines without splitting a rune.
func firstNRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
