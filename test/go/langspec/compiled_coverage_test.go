// Compiled-coverage ratchet (design plan: "Make the boru bytecode VM fully
// independent of the interpreter at runtime", phase P0). The runtime-
// independence work drives every supported program to compile to bytecode
// that runs entirely in the VM, so that BOTH interpreter dependencies — the
// OpFallback island and the whole-program fallback — can be deleted.
//
// This test is the objective measure of that goal. It runs EVERY spec value
// row through CompileCheck and counts the rows that REFUSE (no Program, no
// check error), bucketed by a normalised refusal reason. The total refusal
// count is a downward ratchet: it must never rise above the recorded ceiling,
// and each phase (P2..P6) lowers the ceiling as a refusal category is
// eliminated. P7 (delete the fallback) is gated on this reaching ZERO.
//
// Rows that error during the check pass itself (parse errors, type-error
// rows) are NOT refusals — the program is statically invalid in both engines
// — and are reported separately, not counted against the ceiling.
package langspec

import (
	"sort"
	"strings"
	"testing"
)

// refusalCeiling is the maximum number of spec value rows allowed to refuse
// compilation (CompileCheck returns a nil Program with no check error AND the
// row is not a statically-invalid check-diagnostics row). The runtime-
// independence phases lower it monotonically; it must reach 0 before the
// interpreter fallback can be deleted (plan P7). Never raise it.
const refusalCeiling = 113 // RAISED 0 -> 113 (2026-09-17) by the corpus expansion, and the direction is the honest one: the 0 was an artifact of thin coverage, not an achievement. The corpus tested one-liners whose callback is always written literally at the call site; 707 new rows covering each-variants, callbacks, fold/map/filter, code bodies, fn-locals and multi-construct composition exposed 113 compile ERRORS the old denominator could not see. Every one is an OPEN DEFECT against "all valid code compiles" — see design/COMPILABLE-SUBSET.md §5. Do NOT lower this by deleting rows; lower it by compiling them. The dominant family: the callback is not written at the call site (stored in a map field, picked by a dynamic key, arriving as a f:Function param, built by a factory, composed) — which is exactly the shape real programs use, and why 95.9% on the old corpus did not survive contact with a 10k-character file.

// islandCeiling is the maximum number of compiled programs allowed to embed an
// interpreter island (OpFallback). Islands re-enter the interpreter sub-engine
// at run time, so this is the second downward ratchet toward run-time
// independence (plan): each phase that compiles an island shape natively lowers
// it, and it must reach 0 before the OpFallback machinery can be deleted (P7).
const islandCeiling = 0 // STAYS 0. See islandGate below: islanding is not a thing that gets a budget.

// normaliseReason buckets a refusal reason into a stable category by
// stripping the row-specific tail (word names, counts), so the histogram is
// comparable across rows.
func normaliseReason(reason string) string {
	switch {
	case strings.HasPrefix(reason, "code-body word"):
		return "code-body word (NoEvalArgs)"
	case strings.HasPrefix(reason, "quoted-operand word"):
		return "quoted-operand word"
	case strings.HasPrefix(reason, "compile-time word"):
		return "compile-time word (RunInCheckMode)"
	case strings.HasPrefix(reason, "full-stack word"):
		return "full-stack word (depth/pick/roll)"
	case strings.HasPrefix(reason, "context-dependent word"):
		return "context-dependent word (args/__pa)"
	case strings.HasPrefix(reason, "user fn call"):
		return "user fn call (Stage 3)"
	case strings.HasPrefix(reason, "function-valued operand"):
		return "function-valued operand (Stage 3)"
	case strings.HasPrefix(reason, "function value reaches"):
		return "function value reaches word (Stage 3)"
	case strings.HasPrefix(reason, "anonymous function dispatch"):
		return "anonymous fn dispatch (Stage 3)"
	case strings.HasPrefix(reason, "dynamic input at"):
		return "dynamic input"
	case strings.HasPrefix(reason, "unannotated or opaque word"):
		return "dynamic/opaque output"
	case strings.HasPrefix(reason, "polymorphic dispatch"):
		return "polymorphic dispatch"
	case strings.Contains(reason, "surface-shape typed dispatch"),
		strings.Contains(reason, "unmatched dispatch recovered"):
		return "dispatch recovery (best guess)"
	case strings.Contains(reason, "returns ") && strings.Contains(reason, "values"):
		return "multi-result call"
	case strings.HasPrefix(reason, "dynamic value precedes residual"):
		return "fn-value-call boundary"
	case strings.Contains(reason, "operand of unknown provenance"),
		strings.Contains(reason, "of unknown provenance"):
		return "operand provenance"
	case strings.Contains(reason, "suppressed a runtime error"):
		return "suppressed runtime error"
	case strings.Contains(reason, "without exactly one declared return"),
		strings.Contains(reason, "body value count differs"):
		return "multi-return fn"
	case strings.HasPrefix(reason, "residual shape beyond Stage 1"),
		strings.Contains(reason, "residual value not statically materialisable"):
		return "residual lowering (Stage 1 limit)"
	case strings.HasPrefix(reason, "if:"):
		return "if-branch lowering"
	case strings.HasPrefix(reason, "stack discipline"):
		return "stack discipline (lowering)"
	case strings.HasPrefix(reason, "operand shape at"):
		return "operand shape (Stage 1 limit)"
	case strings.Contains(reason, "redefined inside a conditional body"):
		return "conditional fn shadow (branch/loop)"
	default:
		return "other: " + reason
	}
}

// rootCause maps a normalised refusal bucket to its underlying axis, the second
// dimension of the ratchet (review §6 meta-improvement). It tells a future
// session WHICH kind of investment clears a bucket:
//
//   - correct-error: the row is KNOWN to error; it should compile an error
//     program (OpTrap / a RET count check), never refuse. Must stay 0 — this is
//     the proof of the Trap/Raise work.
//   - soundness:     compiling would (or might) diverge from the interpreter —
//     dynamic/opaque values, the fn-value-call boundary, lost provenance. Needs
//     a soundness story (e.g. richer runtime dispatch), not just more lowering.
//   - scheduling:    the analysis is fine but the lowerer can't arrange the
//     stack — the stack-scheduling / DDCG territory (review §4.1).
//   - opcode:        a missing VM primitive (DUP/ROT/TUCK for deep stack words).
//   - coverage:      a word class / feature the compiler does not model yet
//     (code-body DSL words, quoted-operand meta words, user-fn dispatch).
func rootCause(bucket string) string {
	switch bucket {
	case "suppressed runtime error", "multi-return fn":
		return "correct-error"
	case "dynamic input", "dynamic/opaque output", "fn-value-call boundary",
		"dispatch recovery (best guess)", "operand provenance",
		"function value reaches word (Stage 3)", "context-dependent word (args/__pa)",
		"conditional fn shadow (branch/loop)":
		return "soundness"
	case "residual lowering (Stage 1 limit)", "stack discipline (lowering)",
		"operand shape (Stage 1 limit)", "if-branch lowering", "multi-result call":
		return "scheduling"
	case "full-stack word (depth/pick/roll)":
		return "opcode"
	default:
		return "coverage"
	}
}

// islandCeilingLive is the REGRESSION ceiling of the island count only
// (lanes_test.go) — the END STATE, islandGate below, stays 0 by the
// maintainer's direction. 12 on 2026-09-17: fifteen rows of the corpus
// expansion less the three increment 3 of PR #471 compiled, every one a fn
// VALUE callback (COMPILABLE-SUBSET.md §5). Falls with every island removed;
// never rises.
const islandCeilingLive = 12

// correctErrorCeiling is the REGRESSION ceiling of the correct-error bucket
// (end state 0): code-bodies.tsv:L173 (2026-09-17), an `unpack` inside a fn
// body the checker rejects with `check-mode suppressed a runtime error` on a
// program that has none — a checker defect, pinned on the accuracy side too
// (pinnedFalsePositives).
const correctErrorCeiling = 1

func TestCompiledCoverage(t *testing.T) {
	t.Parallel()
	c := gatherCensus(t)
	rows, compiled, checkErr, refused, islanded := c.rows, c.compiled, c.checkErr, c.refused, c.islanded
	buckets := c.refusalBuckets

	// Histogram, most-frequent first.
	type kv struct {
		reason string
		n      int
	}
	hist := make([]kv, 0, len(buckets))
	for r, n := range buckets {
		hist = append(hist, kv{r, n})
	}
	sort.Slice(hist, func(i, j int) bool {
		if hist[i].n != hist[j].n {
			return hist[i].n > hist[j].n
		}
		return hist[i].reason < hist[j].reason
	})

	t.Logf("compiled coverage: %d rows — %d compiled (%d islanded), %d check-errors, %d refused",
		rows, compiled, islanded, checkErr, refused)
	for _, h := range hist {
		t.Logf("  refusal %4d  %s  [%s]", h.n, h.reason, rootCause(h.reason))
	}

	// Second axis: bucket the refusals by ROOT CAUSE so a future session can see
	// which kind of investment moves the number (soundness vs lowering vs a
	// missing opcode vs a known-error path). The correct-error axis is the proof
	// of the Trap/Raise work — those rows now compile an error program, so it
	// must stay 0.
	byCause := map[string]int{}
	for _, h := range hist {
		byCause[rootCause(h.reason)] += h.n
	}
	for _, cause := range []string{"correct-error", "soundness", "scheduling", "opcode", "coverage"} {
		t.Logf("  root-cause %4d  %s", byCause[cause], cause)
	}
	gate(t, "correct-error refusals", byCause["correct-error"], 0, correctErrorCeiling, false,
		"a known-to-error row must compile an OpTrap / RET error path, not refuse")

	// P7 ENDGAME (design/legacy/P7-ENDGAME.10.ignore): the frontier is GATED at the
	// documented-tier floor. Every one of the refusalGate rows below is owned
	// by a named tier with a written rationale; a NEW refusal (a regression,
	// or an unclassified corpus row) must trip CI and force a conscious
	// classification, never drift in silently. The gate moves DOWN as tiers
	// close; it moves UP only with a new named tier documented in the endgame
	// record. The I1–I5 wave closed the container auto-dispatch tier (I3:
	// guarded 0-arg method landings), the G5 flex path-shape tier (I2: flex
	// shape narrowing in the compile pass), the M6 dynamic-scope tier (I4:
	// OpLookupDynScope/OpBindDynScope), one unnamed fn-value row (I1:
	// unnamed-param frame flow) and three of the repl served-handler rows
	// (I5: CompiledFn.Reg — units dispatch against their owning registry, so
	// an escaped lambda keeps module scope and bodyConstructsFn is retired).
	// I7 then closed the module fn-value-boundary tier (rows 24/25/31/32/33):
	// a fn body's unapplied fn-value residual compiles to the whole-frame
	// replay (OpCallDynFrame — the interpreter's execFnDefLiteral rule decides
	// the apply at run time against the frame, with the RetReplay trim/defer
	// return discipline), and a for-body per-iteration apply of a Function
	// param lowers to OpCallDynamic in source order (apply-first / apply-last),
	// with an escaping break/continue crossing the island to the enclosing
	// compiled loop (escapedFlow). Every tier has since CLOSED: the last
	// "unmatched dispatch recovered" soundness rows graduated to terminal
	// runtime rematches (OpDispatchRematch and the render-bound variants) on
	// 2026-07-15 — the per-row history lives in compiled_refusals_test.go,
	// whose knownRefusals map is EMPTY and whose header states the standing
	// rule: an entry added there must carry a soundness proof, and the goal
	// is for the map to stay empty. The main corpus therefore compiles with
	// ZERO refusals; rows the compiler cannot yet model live in the frontier
	// ledger (frontier_spec_test.go), outside this ratchet, each with a
	// stated graduation criterion.
	const refusalGate = 113 // RAISED 0 -> 113 (2026-09-17). This is the ASSERTION gate; refusalCeiling above is the historical floor. The 0 was true only of the pre-expansion corpus, whose rows are one-liners that always write a callback literally at the call site. 707 rows covering the shapes real code uses exposed 113 compile ERRORS — the compiler did not regress, the measurement got honest. Every one is an open defect (design/COMPILABLE-SUBSET.md §5); lower this by compiling them, never by deleting rows.
	const islandGate = 0    // STAYS 0 — maintainer direction 2026-09-17: "there should be no islanding at all". An island is a region of a COMPILED program that still runs on the interpreter, so it is an uncompiled region inside something we call compiled — the fallback in miniature. It is never ledgered and never raised. The expanded corpus exposes 15 today; they are defects to remove, and this gate stays red until they are.
	// Two lanes (lanes_test.go). The END STATE of both is 0 — no refusal, no
	// island — and the direction lane asserts exactly that; the regression
	// ceilings are the live counts, which only fall, so a change that adds a
	// refusal or an island fails every lane while the open debt stays named.
	gate(t, "compile refusals", refused, 0, refusalGate, false,
		"corpus rows the compiler refuses — every one an open defect (design/COMPILABLE-SUBSET.md §5)")
	gate(t, "interpreter islands", islanded, islandGate, islandCeilingLive, false,
		"compiled programs with an OpFallback span — an uncompiled region inside something called compiled")
	t.Logf("compile refusals=%d (gate %d), islanded=%d (gate %d); historical floor refs %d/%d",
		refused, refusalGate, islanded, islandGate, refusalCeiling, islandCeiling)
}
