// Stage-5 compile-or-fallback gate (design/legacy/boru-bytecode-plan.0.ignore
// §Stage 5: "every program either compiles or falls back, so the whole
// suite must pass in compiled mode"). Where the span-level differential
// gate (compiled_differential_test.go) checks ONLY the rows the emitter
// accepts, this gate runs EVERY row through RunCompiled — which compiles
// what it can and SILENTLY falls back to the interpreter for the rest —
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
	"bufio"
	"errors"
	"fmt"

	lang "github.com/boru-lang/boru/lang/go"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
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

func TestSpecCompiledOrFallback(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := specEntries(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var rows, compiledPath, mismatches, refusedRows int
	entryCensus := newEngineEntryCensus()
	bailCensus := newDeferCensus()
	localBailCensus := newDeferCensus()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		path := filepath.Join(specDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])
			rows++

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
				if wasCompiled {
					localBailCensus.add(ev)
				} else {
					bailCensus.add(ev)
				}
			}
			if wasCompiled {
				compiledPath++
			}
			ai := newDifferentialInstance(t)
			gotI, errI := ai.RunInterp(input)

			key := e.Name() + ":L" + itoa(lineNum)
			// Error taxonomy parity: same presence AND same code.
			if cdC, cdI := errCode(errC), errCode(errI); cdC != cdI {
				if !wasCompiled && cdC == "compile_refused" {
					// A REFUSAL, not a divergence: the compile gate in
					// TestCompiledCoverage owns it (every one an open defect).
					refusedRows++
					continue
				}
				if !divergence(t, "compile-or-fallback", key, fmt.Sprintf("(wasCompiled=%v): %s\n  error divergence: compiled=[%s]%v interpreted=[%s]%v",
					wasCompiled, input, cdC, errC, cdI, errI)) {
					mismatches++
				}
				continue
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
						if !divergence(t, "compile-or-fallback", key, fmt.Sprintf("(wasCompiled=%v): %s\n  error detail divergence:\n  compiled=%q\n  interpreted=%q",
							wasCompiled, input, aeC.Detail, aeI.Detail)) {
							mismatches++
						}
						continue
					}
					if aeI.Row > 0 && aeC.Row == 0 {
						if !divergence(t, "compile-or-fallback", key, fmt.Sprintf("(wasCompiled=%v): %s\n  error position lost in compiled mode: interpreter at %d:%d, compiled has no position\n  detail=%q",
							wasCompiled, input, aeI.Row, aeI.Col, aeC.Detail)) {
							mismatches++
						}
						continue
					}
					// Phase-7 rich-diagnostic parity: the compiled error must carry
					// the SAME notes, suggestions, and secondary spans as the
					// interpreter, not just the same Detail.
					if diff := diagPayloadMismatch(aeC, aeI); diff != "" {
						if !divergence(t, "compile-or-fallback", key, fmt.Sprintf("(wasCompiled=%v): %s\n  diagnostic payload divergence — %s",
							wasCompiled, input, diff)) {
							mismatches++
						}
						continue
					}
				}
				continue
			}
			if renderAny(gotC) != renderAny(gotI) {
				if !divergence(t, "compile-or-fallback", key, fmt.Sprintf("(wasCompiled=%v): %s\n  compiled=%q interpreted=%q",
					wasCompiled, input, renderAny(gotC), renderAny(gotI))) {
					mismatches++
				}
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("compile-or-fallback: %d rows, %d compiled, %d refused (the compile gate's), %d unledgered divergences (values + error taxonomy)", rows, compiledPath, refusedRows, mismatches)
	entryCensus.assertCeiling(t)
	bailCensus.assertCeiling(t)
	localBailCensus.assertLocalCeiling(t)
	checkLedgerRetired(t, "compile-or-fallback")
	if mismatches != 0 {
		t.Errorf("%d compile-or-fallback divergences the ledger does not know — every program must compile to an identical result and error taxonomy", mismatches)
	}
}
