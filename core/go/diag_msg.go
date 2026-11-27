package core

import (
	"strconv"
	"strings"
)

// This file is the single text source for the DISPATCH error family
// (design/DIAGNOSTICS.0.md, phase 3) — the message builders every
// surface that reports an unmatched dispatch shares: the interpreter
// (sigError / the swap-probe interception), the VM's compiled param
// contracts (checkParamContract / checkNativeParamContract), the
// check-pass trap (tryRecordUnmatchedDispatchTrap), the registry's
// 0-arg fallback (fnFallbackSig), and the checker's no_signature
// diagnostics (checkModeAssumeSig). One text source is what keeps the
// compiled-vs-interpreted differential gate (Detail equality) holding
// by construction — the return_check_msg.go pattern, generalized.

// noMatchDetail is the Detail line for an unmatched dispatch.
//
// It is a function of the word name ALONE. The interpreter, the VM's
// runtime param-contract guards, the check pass's OpTrap, and the
// checker all raise this same head, so the compiled_fullcorpus
// Detail-equality gate holds by construction.
func NoMatchDetail(name string) string {
	return "cannot call `" + name + "` — no signature matches the arguments"
}

// noMatchDiag builds the COMPLETE unmatched-dispatch diagnostic —
// Detail, the received-arguments note, per-candidate verdicts, and the
// fix suggestions — from data alone (no Engine, no tape), so the
// interpreter (sigError) and the compiled VM's runtime guards produce
// a byte-identical error over the same failing tuple. `written` is the
// failing arguments in sig order (sig[i] ↔ written[i]); `reorder` is
// the precomputed swap-probe message ("" when none applies).
//
// Design note (compiled-mode parity): the diagnostic deliberately
// carries NO tape-snapshot note. The interpreter's old "stack: …" note
// dumped engine-internal tape state (markers, the >>>word<<< pointer)
// that the compiled VM has no equivalent for, so it could never match
// across engines; the received-arguments note conveys the same "what
// values were here" information in plain language instead.
func NoMatchDiag(source, name string, fn *FnDefInfo, written []Value, pos SrcPos, reorder string) *BoruError {
	ae := makeBoruErrorAt("signature_error", NoMatchDetail(name), name, source, "", pos)

	if n := callArgsNote(written); n != "" {
		ae.Notes = append(ae.Notes, n)
	}
	totalSigs := 0
	if fn != nil {
		for i := range fn.Signatures {
			if !fn.Signatures[i].Fallback {
				totalSigs++
			}
		}
	}
	fails := explainCandidates(fn, written)
	notes := candidateNotes(name, fails, totalSigs, len(written))
	if len(notes) == 0 && totalSigs > 0 {
		// The probe could not blame any overload (or had no tuple to
		// probe) — fall back to the compact signature overview.
		notes = []string{"expected: " + name + " " + describeAllSigs(fn)}
	}
	ae.Notes = append(ae.Notes, notes...)

	switch {
	case reorder != "":
		ae.Suggestions = append(ae.Suggestions, DiagSuggestion{Message: reorder})
	case fn != nil && fn.HasForwardSigs():
		// Forward-precedence fix: when the word has forward-collecting
		// signatures, the most common cause of this error is that forward
		// collection ran into a following word before it could gather
		// enough arguments — see forwardParensSuggestion.
		ae.Suggestions = append(ae.Suggestions, DiagSuggestion{Message: forwardParensSuggestion(name)})
	}
	if totalSigs >= 4 || len(fails) > diagMaxCandidates {
		ae.Suggestions = append(ae.Suggestions, DiagSuggestion{Message: describeSuggestion(name)})
	}
	return ae
}

// insufficientArgsDetail is the Detail line for a forward collection
// that could not gather the arguments its matched signature planned
// for (the word's strict forward window ran out of usable tokens).
func insufficientArgsDetail(name string, expected int) string {
	return "cannot call `" + name + "` — it expects " +
		strconv.Itoa(expected) + " forward " + pluralArg(expected) +
		" but fewer were supplied"
}

// forwardParensSuggestion is the shared "group the call in parens" fix
// for a forward-collecting word starved by the next word — raised both
// by the interpreter's sigError and the registry's 0-arg fallback.
// The caveat sentence keeps the fix honest for barrier-receiver words
// (`dot` and the accessor family): a paren group seals the enclosing
// stack, so a word whose receiver slot reads from the stack may never
// match inside one (NUR049) — the sequential spelling is the fallback.
func forwardParensSuggestion(name string) string {
	return "forward args for " + name +
		" may have run into the next word; group the call in parens so its " +
		"RESULT becomes the argument — (" + name + " …). `end` / `;` only ends " +
		"the statement — it does NOT turn a following word into a nested call. " +
		"If " + name + " reads a value from the enclosing stack (a paren seals " +
		"the stack off), run it first and use its result in sequence instead."
}

// describeSuggestion points a dispatch failure at the word's full
// reference entry. Only offered when the candidate list was truncated
// or the word carries many overloads — for a one-signature word the
// candidate verdict already says everything describe would.
func describeSuggestion(name string) string {
	return "see `boru describe " + name + "` for its signatures and examples"
}

// callArgsNote describes the tuple the failed dispatch saw:
// "the argument was 99 (an Integer)" /
// "the arguments were 'x' (a String) and 2 (an Integer)".
// Empty when no operands were in view.
func callArgsNote(written []Value) string {
	if len(written) == 0 {
		return ""
	}
	parts := make([]string, len(written))
	for i, v := range written {
		parts[i] = describeArgValue(v)
	}
	if len(parts) == 1 {
		return "the argument was " + parts[0]
	}
	last := parts[len(parts)-1]
	return "the arguments were " + strings.Join(parts[:len(parts)-1], ", ") +
		" and " + last
}

// describeArgValue renders one operand as “value (a Type)” — the
// expected-vs-found voice used across the dispatch notes.
func describeArgValue(v Value) string {
	leaf := "?"
	if t := ValueType(v); t != nil {
		leaf = t.Leaf()
	}
	return diagValue(v) + " (" + typeArticle(leaf) + " " + leaf + ")"
}

// typeArticle picks the indefinite article for a type name.
func typeArticle(name string) string {
	if name == "" {
		return "a"
	}
	switch name[0] {
	case 'A', 'E', 'I', 'O', 'U', 'a', 'e', 'i', 'o', 'u':
		return "an"
	}
	return "a"
}

// diagMaxCandidates caps the per-candidate verdict notes so a heavily
// overloaded word (add: numeric + concat + patrun forms) does not bury
// the error; the remainder collapses into an “…and N more” line and
// the describe suggestion.
const diagMaxCandidates = 3

// candidateNotes renders explainCandidates' verdicts as notes, ranked
// nearest-first (highest Score), capped at diagMaxCandidates.
// totalSigs is the word's non-fallback overload count; nWritten the
// probed tuple length (for the arity verdict's "but N were supplied").
func candidateNotes(name string, fails []CandidateFailure, totalSigs, nWritten int) []string {
	if len(fails) == 0 {
		return nil
	}
	shown := len(fails)
	if shown > diagMaxCandidates {
		shown = diagMaxCandidates
	}
	notes := make([]string, 0, shown+1)
	for _, f := range fails[:shown] {
		label := "candidate `" + name + " " + describeSigArgs(f.Sig) + "`"
		switch f.Kind {
		case candArity:
			n := f.Sig.TotalArgs()
			supplied := "none were"
			if nWritten == 1 {
				supplied = "1 was"
			} else if nWritten > 1 {
				supplied = strconv.Itoa(nWritten) + " were"
			}
			notes = append(notes, label+" takes "+strconv.Itoa(n)+" "+
				pluralArg(n)+", but "+supplied+" supplied")
		case candSlotType:
			notes = append(notes, label+" — argument "+strconv.Itoa(f.SlotIdx+1)+
				": expected "+f.Expected.String()+", got "+describeArgValue(f.Got))
		default: // candPattern
			notes = append(notes, label+" — argument "+strconv.Itoa(f.SlotIdx+1)+
				": "+diagValue(f.Got)+" does not satisfy its declared pattern "+
				diagValue(f.Pattern))
		}
	}
	if hidden := totalSigs - shown; hidden > 0 {
		notes = append(notes, "…and "+strconv.Itoa(hidden)+" more "+
			pluralSig(hidden))
	}
	return notes
}

// undefinedWordDetail is the Detail line for an unbound word — the
// grep-friendly head every undefined_word surface shares (86 spec rows
// pin it); the Elm voice lives in the did-you-mean suggestion.
func UndefinedWordDetail(name string) string {
	return "undefined word: " + name
}

// DidYouMean renders a near-miss suggestion for name over candidates —
// "did you mean `length`?" — or "" when nothing plausible is close
// enough (SuggestNames' distance cap). Exported for the language layer
// (strict-read key misses suggest the container's actual keys).
func DidYouMean(name string, candidates []string) string {
	return didYouMeanMessage(SuggestNames(name, candidates))
}

// didYouMeanMessage renders an already-ranked near-miss list.
func didYouMeanMessage(matches []string) string {
	if len(matches) == 0 {
		return ""
	}
	quoted := make([]string, len(matches))
	for i, m := range matches {
		quoted[i] = "`" + m + "`"
	}
	switch len(quoted) {
	case 1:
		return "did you mean " + quoted[0] + "?"
	case 2:
		return "did you mean " + quoted[0] + " or " + quoted[1] + "?"
	default:
		return "did you mean " + quoted[0] + ", " + quoted[1] + ", or " + quoted[2] + "?"
	}
}

// pluralArg / pluralSig — grammatical number for the two nouns the
// dispatch notes count.
func pluralArg(n int) string {
	if n == 1 {
		return "argument"
	}
	return "arguments"
}

func pluralSig(n int) string {
	if n == 1 {
		return "signature"
	}
	return "signatures"
}
