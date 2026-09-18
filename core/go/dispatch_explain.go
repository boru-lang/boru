package core

import "sort"

// This file owns the dispatch-failure EXPLAIN pass
// (design/DIAGNOSTICS.0.md, phase 3): after a dispatch has already
// failed, re-probe each overload against the failing tuple and record
// WHY it rejected — the per-candidate verdicts Rust prints under
// "no method found". It runs ONLY on the failure path (sigError and
// the swap-probe interception), never inside matchSignature's hot
// loop, so a successful call pays nothing for it.

// candFailKind classifies why one overload rejected the probed tuple.
type candFailKind int

const (
	// candArity — the overload's arity differs from the tuple's length.
	candArity candFailKind = iota
	// candSlotType — a slot's declared type rejected the written value.
	candSlotType
	// candPattern — the slot's type admitted the value but its declared
	// pattern (a structural map shape, a value literal, a negation)
	// rejected it.
	candPattern
)

// CandidateFailure records why one overload could not accept the
// failing argument tuple.
type CandidateFailure struct {
	Sig      *Signature
	Kind     candFailKind
	SlotIdx  int   // 0-based failing slot; -1 for arity failures
	Expected *Type // declared type at SlotIdx (candSlotType only)
	Got      Value // written value at SlotIdx (slot/pattern failures)
	Pattern  Value // the rejecting pattern (candPattern only)
	Score    int   // leading slots that admitted their value — ranks nearest-first
}

// explainCandidates probes every non-fallback overload of fn against
// the written tuple POSITIONALLY (sig[i] ↔ written[i] — the same
// assignment-order view the swap probe reorderHintFor receives) and
// reports the first reason each overload fails, ranked nearest-first
// by how many leading slots matched.
//
// Positional fidelity is the documented contract: the probe does not
// replay forward-collection re-splits or barrier windows, so the slot
// it blames is the best positional reading, not a re-run of the
// planner. It only ever runs against a tuple the dispatch ALREADY
// rejected, so it can never contradict a successful match. An overload
// whose every slot admits its value positionally (the real failure was
// collection order or context) is SKIPPED rather than given a
// fabricated reason.
func explainCandidates(fn *FnDefInfo, written []Value) []CandidateFailure {
	if fn == nil {
		return nil
	}
	fails := make([]CandidateFailure, 0, len(fn.Signatures))
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if sig.Fallback {
			continue
		}
		n := sig.TotalArgs()
		if n != len(written) {
			score := 0
			for i := 0; i < n && i < len(written); i++ {
				if !slotAdmits(sig, i, written[i]) {
					break
				}
				score++
			}
			fails = append(fails, CandidateFailure{
				Sig: sig, Kind: candArity, SlotIdx: -1, Score: score})
			continue
		}
		if f, failed := probeSlots(sig, written); failed {
			fails = append(fails, f)
		}
	}
	sort.SliceStable(fails, func(i, j int) bool {
		return fails[i].Score > fails[j].Score
	})
	return fails
}

// probeSlots walks sig's positions over an arity-matching tuple and
// returns the first slot-level failure. ok=false means every slot
// admitted its value — the probe cannot explain this overload.
func probeSlots(sig *Signature, written []Value) (CandidateFailure, bool) {
	n := sig.TotalArgs()
	for i := 0; i < n; i++ {
		if sig.QuoteArgs != nil && sig.QuoteArgs[i] {
			// A quoted slot binds a bare name as data without a type
			// match (the interpreter's rule; checkNativeParamContract
			// skips these the same way).
			continue
		}
		if !SigArgMatches(sig, i, written[i]) {
			return CandidateFailure{Sig: sig, Kind: candSlotType, SlotIdx: i,
				Expected: SigArgType(sig, i), Got: written[i], Score: i}, true
		}
		if pat, ok := SigPattern(sig, i); ok && patternRejects(pat, written[i]) {
			return CandidateFailure{Sig: sig, Kind: candPattern, SlotIdx: i,
				Got: written[i], Pattern: pat, Score: i}, true
		}
	}
	return CandidateFailure{}, false
}

// slotAdmits reports whether sig position i admits v — the quoted-slot
// rule plus the ordinary positional type match.
func slotAdmits(sig *Signature, i int, v Value) bool {
	if sig.QuoteArgs != nil && sig.QuoteArgs[i] {
		return true
	}
	return SigArgMatches(sig, i, v)
}

// patternRejects reports whether a declared slot pattern DEFINITELY
// rejects v — the same verdicts patternsOk reaches for a stack-matched
// position, minus the tape/forward machinery (the probe holds resolved
// values, so the forward-leniency skips do not apply).
func patternRejects(pattern Value, v Value) bool {
	if pattern.Carrier {
		// A check-mode placeholder for a COMPUTED pattern — the real
		// value is unknown, so the probe cannot judge it.
		return false
	}
	if pattern.Parent.Equal(TMap) && v.Parent.Equal(TMap) &&
		pattern.Data != nil && v.Data != nil &&
		!IsOptionsType(pattern) &&
		!IsRecordType(v) && !IsTypedMap(v) && !IsOptionsType(v) {
		return !OpenUnifyMap(pattern, v)
	}
	// Negation patterns take the direct path, mirroring patternsOk's
	// fast path (the verdict depends only on the operand's top-level
	// shape).
	if ni, err := AsNegation(pattern); err == nil {
		_, uerr := unifyNegation(ni, v, nil)
		return uerr != nil
	}
	_, ok := Unify(v, pattern)
	return !ok
}
