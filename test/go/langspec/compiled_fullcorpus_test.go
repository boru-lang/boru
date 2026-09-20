// Stage-5 compile-or-fallback gate (design/legacy/boru-bytecode-plan.0.ignore
// §Stage 5: "every program either compiles or falls back, so the whole
// suite must pass in compiled mode"). Where the span-level differential
// gate (compiled_differential_test.go) checks ONLY the rows the emitter
// accepts, this gate runs EVERY row through RunCompiled — which compiles
// what it can and SILENTLY does not compile for the rest —
// and asserts FULL parity with the interpreter: identical values, AND
// identical error taxonomy (presence + code). This is the plan's ground
// rule: "identical results, identical error taxonomy, or the stage
// doesn't ship."
//
// Zero divergences is the bar. Two classes of compiled-mode unsoundness
// were closed to reach it:
//   - the fallback no longer double-executes check-pass side effects
//     (RunCompiled snapshots/rolls back the registry — no re-mint,
//     re-import, re-run Test spec);
//   - the compiled path reproduces the interpreter's runtime guards: the
//     VM enforces declared return types/counts at RET (the __RC mirror),
//     and a check-mode word that suppresses a strict runtime error (an
//     orphan gen, an unpack of a missing key) marks the program
//     uncompilable so it falls back and errors faithfully.
package langspec

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// errCode returns a boru error's taxonomy code (or "" for nil, "non-boru"
// for a foreign error) so the gate compares taxonomy, not message text.
func errCode(e error) string {
	if e == nil {
		return ""
	}
	var ae *core.BoruError
	if errors.As(e, &ae) {
		return ae.Code
	}
	return "non-boru"
}

// asBoruError unwraps e to a *BoruError, or nil for a non-Boru / nil error.
func asBoruError(e error) *core.BoruError {
	var ae *core.BoruError
	if e != nil && errors.As(e, &ae) {
		return ae
	}
	return nil
}

// diagPayloadMismatch compares the RICH diagnostic payload of the compiled and
// interpreted errors — the notes, the suggestion messages, and the secondary
// span labels + positions — and returns a human-readable divergence, or "" when
// they match. This is the phase-7 parity enforcement: compiled-mode errors must
// carry the SAME structured diagnostic as the interpreter, not merely the same
// Detail. The PRIMARY caret position is deliberately excluded (the VM points
// inside the shared fn unit where the interpreter points at the call site — the
// documented return-error difference, gated separately by position PRESENCE).
func diagPayloadMismatch(aeC, aeI *core.BoruError) string {
	if !normSliceEq(aeC.Notes, aeI.Notes) {
		return "notes:\n  compiled=" + strings.Join(aeC.Notes, " | ") +
			"\n  interpreted=" + strings.Join(aeI.Notes, " | ")
	}
	sc, si := suggestionMsgs(aeC), suggestionMsgs(aeI)
	if !normSliceEq(sc, si) {
		return "suggestions:\n  compiled=" + strings.Join(sc, " | ") +
			"\n  interpreted=" + strings.Join(si, " | ")
	}
	lc, li := spanKeys(aeC), spanKeys(aeI)
	if !strSliceEq(lc, li) {
		return "spans:\n  compiled=" + strings.Join(lc, " | ") +
			"\n  interpreted=" + strings.Join(li, " | ")
	}
	return ""
}

// volatileValueRender strips the two incidental value-rendering
// differences the two engines legitimately have when a diagnostic
// EMBEDS a value — the same non-determinism the result comparison
// already tolerates, orthogonal to diagnostic quality:
//   - counter-based provenance IDs (`fn foo#162` vs `#163`): the engines
//     mint IDs at different points, so the numbers differ;
//   - an operand held pre- vs post-evaluation (`{k:word(true)}` vs
//     `{k:true}`): a map literal captured before/after auto-eval.
//
// The diagnostic STRUCTURE (candidate verdicts, suggestions, spans) is
// what the gate enforces; the embedded value's incidental form is not.
var volatileID = regexp.MustCompile(`#\d+`)
var volatileWord = regexp.MustCompile(`word\(([^)]*)\)`)

func volatileValueRender(s string) string {
	s = volatileID.ReplaceAllString(s, "#N")
	s = volatileWord.ReplaceAllString(s, "$1")
	return s
}

// normSliceEq compares two note/suggestion slices with the incidental
// value-rendering differences normalised away.
func normSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if volatileValueRender(a[i]) != volatileValueRender(b[i]) {
			return false
		}
	}
	return true
}

func strSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func suggestionMsgs(ae *core.BoruError) []string {
	out := make([]string, len(ae.Suggestions))
	for i, s := range ae.Suggestions {
		out[i] = s.Message
	}
	return out
}

// spanKeys renders each secondary span as "label@row:col" so both the label
// and the location are compared (the produced-value and declaration spans must
// point at the same place in both engines).
func spanKeys(ae *core.BoruError) []string {
	out := make([]string, len(ae.Spans))
	for i, s := range ae.Spans {
		out[i] = s.Label + "@" + itoa(s.Pos.Row) + ":" + itoa(s.Pos.Col)
	}
	return out
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// fallbackVerdict compares one row's compiled-or-fallback run with the
// interpreter's: error taxonomy first, then error content, then values.
// declined is a compile_failed the compile gate owns (not a divergence);
// unledgered is a divergence knownDivergences does not carry. It runs on a
// walk worker, so it takes testing.TB and touches no shared state —
// divergence is goroutine-safe.
// A pinLedger records rows whose two lanes agree on everything the error
// TAXONOMY names — code and detail — and differ only in how the diagnostic
// is PRESENTED. Each pin carries the NUR that owns the drift.
//
// Its own ledger rather than knownDivergences, because it is its own kind of
// finding. knownDivergences is checked by every gate that walks the corpus
// and its entries must diverge on all of them; a presentation drift is
// visible only where presentation is asserted, which is here. Filing one
// there would fail the differential gate for not seeing a divergence it does
// not look for.
//
// The seen-set is the ledger's other half: a pin that stopped drifting is
// retired with the change that fixed it, never left to rot.
type pinLedger struct {
	name string
	pins map[string]string

	mu   sync.Mutex
	seen map[string]bool
}

// known reports the pin for key, recording that the gate saw it drift.
func (l *pinLedger) known(key string) (string, bool) {
	why, ok := l.pins[key]
	if !ok {
		return "", false
	}
	l.mu.Lock()
	if l.seen == nil {
		l.seen = map[string]bool{}
	}
	l.seen[key] = true
	l.mu.Unlock()
	return why, true
}

// checkRetired fails for every pin the gate did not meet on a full walk.
func (l *pinLedger) checkRetired(t testing.TB) {
	t.Helper()
	if filteredCorpus() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, why := range l.pins {
		if !l.seen[key] {
			t.Errorf("%s entry %s no longer drifts — retire it with the change that fixed it (was: %s)", l.name, key, why)
		}
	}
}

// knownPositionLoss pins rows whose compiled error carries no SOURCE POSITION
// where the interpreter's does.
var knownPositionLoss = &pinLedger{name: "knownPositionLoss", pins: map[string]string{
	"reach.tsv:L52": "NUR171 — the compiled no-match diagnostic carries no source position: the recorder gives PolyNoMatchSpec no dispatch position and the debug table has none at that pc, so the raise has nothing to stamp (the interpreter's re-run used to supply it)",
}}

// knownDiagDrift pins rows whose two lanes describe the SAME failure in
// different words — the notes, suggestions or secondary spans differ while
// code and detail agree.
var knownDiagDrift = &pinLedger{name: "knownDiagDrift", pins: map[string]string{
	"reach.tsv:L52": "NUR172 — the two lanes describe different argument windows at a poly no-match: CALL_NATIVE_POLY holds both the key atom and the receiver, so the compiled notes report a type mismatch on argument 2, while the interpreter never bound the forward atom and reports an arity failure over one argument",
}}

func fallbackVerdict(t testing.TB, key, input string, wasCompiled bool, gotC []any, errC error, gotI []any, errI error) (declined, unledgered bool) {
	t.Helper()
	// A compiled run that BAILED is not a divergence: it is the defect the
	// interpreter re-run used to absorb, and it is counted in its own
	// ledger (compiled_defect_test.go) rather than read as a new miscompile.
	if bookCompiledDefect(key, input, errC) {
		return false, false
	}
	// Error taxonomy parity: same presence AND same code.
	if cdC, cdI := errCode(errC), errCode(errI); cdC != cdI {
		if !wasCompiled && cdC == "compile_failed" {
			// A COMPILE FAILURE, not a divergence: the compile gate in
			// TestCompiledCoverage owns it (every one an open defect).
			return true, false
		}
		return false, !divergence(t, "compile-or-fallback", key, fmt.Sprintf("(wasCompiled=%v): %s\n  error divergence: compiled=[%s]%v interpreted=[%s]%v",
			wasCompiled, input, cdC, errC, cdI, errI))
	}
	if errC != nil {
		// Error CONTENT parity: the compiled VM goes out of its way to
		// reproduce the interpreter's errors byte-for-byte (vmReturnTypeErr,
		// vmReturnCountErr), so detail text must match and the compiled
		// error must carry a source position whenever the interpreter does.
		// Exact Row/Col are NOT asserted: a return-type error is stamped at
		// the call site by the interpreter but inside the shared fn unit by
		// the VM, so the column legitimately differs — only presence is
		// gated, which is what catches a "source position unknown" regression.
		if aeC, aeI := asBoruError(errC), asBoruError(errI); aeC != nil && aeI != nil {
			if aeC.Detail != aeI.Detail {
				return false, !divergence(t, "compile-or-fallback", key, fmt.Sprintf("(wasCompiled=%v): %s\n  error detail divergence:\n  compiled=%q\n  interpreted=%q",
					wasCompiled, input, aeC.Detail, aeI.Detail))
			}
			if aeI.Row > 0 && aeC.Row == 0 {
				if why, known := knownPositionLoss.known(key); known {
					directionFailure(t, "%s: known position loss (%s)", key, why)
				} else {
					return false, !divergence(t, "compile-or-fallback", key, fmt.Sprintf("(wasCompiled=%v): %s\n  error position lost in compiled mode: interpreter at %d:%d, compiled has no position\n  detail=%q",
						wasCompiled, input, aeI.Row, aeI.Col, aeC.Detail))
				}
			}
			// Phase-7 rich-diagnostic parity: the compiled error must carry
			// the SAME notes, suggestions, and secondary spans as the
			// interpreter, not just the same Detail.
			if diff := diagPayloadMismatch(aeC, aeI); diff != "" {
				why, known := knownDiagDrift.known(key)
				if !known {
					return false, !divergence(t, "compile-or-fallback", key, fmt.Sprintf("(wasCompiled=%v): %s\n  diagnostic payload divergence — %s",
						wasCompiled, input, diff))
				}
				directionFailure(t, "%s: known diagnostic drift (%s)", key, why)
			}
		}
		return false, false
	}
	if renderAny(gotC) != renderAny(gotI) {
		return false, !divergence(t, "compile-or-fallback", key, fmt.Sprintf("(wasCompiled=%v): %s\n  compiled=%q interpreted=%q",
			wasCompiled, input, renderAny(gotC), renderAny(gotI)))
	}
	return false, false
}

func TestSpecCompiledOrFallback(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var rows, compiledPath, mismatches, failedRows int
	entryCensus := newEngineEntryCensus()
	bailCensus := newDeferCensus()
	localBailCensus := newDeferCensus()
	specWalk(t, func(t testing.TB, r specRow) {
		if len(r.Cells) < 2 {
			return
		}
		input := r.Input

		ac := newDifferentialInstance(t)
		disarm := ac.ArmInterpEntryHook(entryCensus.add)
		// A bail's COST is not knowable when it fires — it depends on who
		// catches it — so hold the row's bails and sort them by what
		// actually happened (see deferLocalCeiling).
		var rowBails []lang.BailEvent
		disarmBail := ac.ArmRuntimeBailHook(func(ev lang.BailEvent) {
			rowBails = append(rowBails, ev)
		})
		gotC, wasCompiled, errC := ac.RunCompiled(input)
		disarm()
		disarmBail()
		for _, ev := range rowBails {
			// Sorted by whether the bail SURFACED as the run's failure,
			// not by wasCompiled. The two used to coincide: a bail the
			// caller could not absorb re-ran the whole program, so the row
			// came back wasCompiled=false. Nothing re-runs, so a bailed
			// program comes back wasCompiled=TRUE with an error — it
			// compiled, and then it died — and bucketing on the flag would
			// file every whole-program bail as "locally resolved", which is
			// the one thing it is not.
			//
			// "Surfaced" is the DEFECT class specifically, not any error:
			// a row whose bail was resolved locally and which then failed
			// for its own reasons (`5 $.name apply` raises the
			// interpreter's signature_error) was still resolved locally,
			// and so was one that raised the defer's prepared alt — that is
			// the trap disposition working, not a bail escaping.
			if compiledDefect(errC) {
				bailCensus.add(ev)
			} else {
				localBailCensus.add(ev)
			}
		}
		ai := newDifferentialInstance(t)
		gotI, errI := ai.RunInterp(input)

		declined, unledgered := fallbackVerdict(t, r.Key(), input, wasCompiled, gotC, errC, gotI, errI)

		mu.Lock()
		defer mu.Unlock()
		rows++
		if wasCompiled {
			compiledPath++
		}
		if declined {
			failedRows++
		}
		if unledgered {
			mismatches++
		}
	})

	t.Logf("compile-or-fallback: %d rows, %d compiled, %d declined (the compile gate's), %d unledgered divergences (values + error taxonomy)", rows, compiledPath, failedRows, mismatches)
	entryCensus.assertCeiling(t)
	bailCensus.assertCeiling(t)
	localBailCensus.assertLocalCeiling(t)
	checkLedgerRetired(t, "compile-or-fallback")
	knownPositionLoss.checkRetired(t)
	knownDiagDrift.checkRetired(t)
	assertBailDefectLedger(t)
	if mismatches != 0 {
		t.Errorf("%d compile-or-fallback divergences the ledger does not know — every program must compile to an identical result and error taxonomy", mismatches)
	}
}
