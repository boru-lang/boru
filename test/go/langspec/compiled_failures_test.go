// Compilation compile failures are test FAILURES. Full native compilation of every boru
// program is the goal (design/COMPILABLE-SUBSET.md, design/legacy/P7-ENDGAME.10.ignore): a
// whole-program compile failure silently runs on the interpreter — nothing in the run
// says the compile failed — and it keeps the compiler tied to the tree-walker. So this gate treats every spec-row compile failure as a failure UNLESS
// the row is on knownCompileFailures — the small, documented set where compiling the
// guess available TODAY would ship a WRONG answer, so the shape declines.
// Nothing absorbs it: the row does not compile and says so. Being on the
// allowlist does not make a row
// acceptable: each entry is an OPEN DEFECT owed a faithful lowering, and the
// list ratchets to zero (design/legacy/boru-bytecode-final-two-refusals.0.ignore withdrew
// the earlier "correct-by-design" reading of these rows).
//
// This is the per-ROW companion to TestCompiledCoverage's count/root-cause gate:
// the count gate catches a compile failure-count regression, this gate pins the EXACT
// rows, so swapping one compile failure for another (count unchanged) still fails, and a
// row that starts compiling is flagged as a stale allowlist entry to ratchet the
// list DOWN toward zero. Both read the single corpus census (compiled_census_test).
package langspec

import "testing"

// knownCompileFailures is the allowlist of spec rows the bytecode compiler currently
// declines to lower, keyed by EXACT source. Every entry is a proven SOUNDNESS
// compile failure — and an open defect: dispatch does not statically resolve — an "unmatched
// dispatch recovered" best-guess (the checker recovers an unmatched call the
// interpreter resolves or raises at runtime), or a fn value reaching a consuming
// word whose identity/capture state cannot bake — so a compiled guess could
// diverge from the interpreter. These are the ONLY compile failures the suite tolerates.
//
// To lower this list (the goal), widen the compilable subset so the row compiles,
// then delete its entry. Do NOT add a row here to silence a real gap — an entry
// is a promise that compiling the row would be UNSOUND, not merely unimplemented.
var knownCompileFailures = map[string]string{
	// GRADUATED 2026-07-14 (OpDispatchRematch, plan Phase 3): the apply.tsv
	// pair and the four generics rows compile to a terminal runtime rematch —
	// the failed window (a single event-carrier in every one) re-matches over
	// the live values at run time and raises the shared rich diagnostic
	// byte-identical to the interpreter (or defers when it unexpectedly
	// matches). The map is EMPTY: every corpus compile failure graduated (the last —
	// the each variadic-if row — on 2026-07-15; see the dated notes below).
	// An entry added here must carry a soundness proof, and the goal is for
	// the map to stay empty.

	// GRADUATED 2026-07-15 (the LAST corpus compile failure — compile failures reached 0):
	// the each variadic-if row. Its dispatch half records an offset-form
	// rematch (DispatchSpec.WrittenOff); the branch merge seats the 1-vs-2
	// arm residual via the all-inert re-push (captureInertArmResidual — the
	// loop-side capture mirrored to branch arms) and the variadic merge; the
	// terminal rematch seats its const operand UNDER the live region top
	// (push + swap — the raise only reads the window), so both polarities
	// compile and raise byte-identical to the interpreter.

	// GRADUATED 2026-07-15 (render bound, plan 3a): the local-add row — a
	// locally-redefined `add` overload whose merge is invisible after fn
	// exit — compiles to a runtime rematch. Its match probed a WIDER window
	// (3 positions) than the tuple its error renders (the single stack
	// value: the forward walk breaks at the `true`/`false` WORD tokens), so
	// DispatchSpec carries NWritten, proven a leading prefix of the window
	// by ID at the record gate; the VM re-matches the full window and
	// renders over window[:NWritten], byte-identical to the interpreter.

	// GRADUATED 2026-07-14 (word-splice): a PARKED `__SP` marker (def-bound,
	// collected by value — never stepped before the dispatch) is identical at
	// run time, so the definiteness screen no longer lists IsSplice and the
	// row compiles to a serialized terminal trap raising the interpreter's
	// byte-identical no-match. The cascade fn_body_error of the post-trap
	// assumed-dispatch body analysis joined the SuppressBodyErrors codes. A
	// splice AT the pointer fires before any dispatch on both engines, so no
	// window ever holds a would-have-fired marker.
}

// TestCompileFailuresAreBugs fails on any spec-row compilation compile failure that is not
// a documented allowlisted compile failure, and on any allowlist entry that no
// longer declines (stale — remove it). See knownCompileFailures.
func TestCompileFailuresAreBugs(t *testing.T) {
	t.Parallel()
	c := gatherCensus(t)

	seen := make(map[string]bool, len(knownCompileFailures))
	for _, r := range c.refusedRows {
		if _, ok := knownCompileFailures[r.input]; ok {
			seen[r.input] = true
			continue
		}
		// Open debt on the direction lane; on the regression lane the COUNT
		// gate in TestCompiledCoverage owns these rows (lanes_test.go).
		directionFailure(t, "compilation compile failure is a failure (%s:%d): %q\n  reason: %s\n  full native compilation is the goal: widen the compilable subset so this row compiles, or — only if compiling it would be UNSOUND — add it to knownCompileFailures with a soundness justification.",
			r.file, r.line, r.input, r.reason)
	}

	for input, why := range knownCompileFailures {
		if !seen[input] {
			t.Errorf("stale allowlist entry — this row no longer fails to compile; delete it from knownCompileFailures to ratchet the list down:\n  %q\n  (was: %s)", input, why)
		}
	}

	t.Logf("compile failures: %d rows, all %d documented in knownCompileFailures", len(c.refusedRows), len(knownCompileFailures))
}
