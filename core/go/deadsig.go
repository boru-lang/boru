package core

// Dead-overload (unreachable-signature) detection. boru dispatch is
// first-match-wins over SortSignatures' most-specific-first order, so a
// signature S is unreachable when an earlier, higher-priority signature
// S' accepts every call S accepts — S' always wins and S never fires.
// See design/legacy/checker-loud-diagnostics-report.10.ignore Phase 2.

// DeadSig records an unreachable signature and the earlier one that
// shadows it.
type DeadSig struct {
	Index      int       // position of the dead sig in dispatch (sorted) order
	Sig        Signature // the unreachable signature
	ShadowedBy Signature // the earlier signature that subsumes it
}

// sigIsTypeArg reports whether position i of s expects a type literal
// (TypeArgs slot) rather than an ordinary value.
func sigIsTypeArg(s *Signature, i int) bool {
	return s.TypeArgs != nil && s.TypeArgs[i]
}

// sigIsQuoteArg reports whether position i of s captures the upcoming
// word as an atom (a /q slot) rather than matching an existing value.
// A /q slot and a plain slot of the same type admit different inputs.
func sigIsQuoteArg(s *Signature, i int) bool {
	return s.QuoteArgs != nil && s.QuoteArgs[i]
}

// sigSubsumes reports whether s1 accepts every call that s2 accepts.
// Requirements, all of which must hold for the relation to be sound:
//
//   - same arity and same forward/stack boundary (BarrierPos) — sigs
//     that differ here compete for different call shapes;
//   - the same per-position TypeArgs kind — a type-literal slot and a
//     value slot admit disjoint things;
//   - no structural pattern on either side at any position — patterns
//     are value-dependent, so we conservatively decline to reason about
//     them rather than risk a false "dead" verdict;
//   - at every position, s2's declared type is s1's type or a subtype
//     (t2 ⊆ t1), so any value satisfying s2 also satisfies s1.
//
// When this holds and s1 sorts before s2, s2 can never win dispatch.
func sigSubsumes(s1, s2 *Signature) bool {
	if s1.TotalArgs() != s2.TotalArgs() {
		return false
	}
	if s1.BarrierPos != s2.BarrierPos {
		return false
	}
	n := s2.TotalArgs()
	for i := 0; i < n; i++ {
		if sigIsTypeArg(s1, i) != sigIsTypeArg(s2, i) {
			return false
		}
		if sigIsQuoteArg(s1, i) != sigIsQuoteArg(s2, i) {
			return false
		}
		if _, ok := SigPattern(s1, i); ok {
			return false
		}
		if _, ok := SigPattern(s2, i); ok {
			return false
		}
		t1 := SigArgType(s1, i)
		t2 := SigArgType(s2, i)
		if t1 == nil || t2 == nil {
			return false
		}
		// s1 admits every value of type t2 iff t2 is t1 or a descendant.
		if !t2.ConformsTo(t1) {
			return false
		}
	}
	return true
}

// DeadSignatures returns the unreachable signatures in sigs, in dispatch
// order. The input is copied and sorted via SortSignatures (idempotent
// on already-sorted input) so the result reflects the order the matcher
// actually uses. Fallback signatures are never reported — they are
// intentional 0-arg catch-alls that sort last.
func DeadSignatures(sigs []Signature) []DeadSig {
	if len(sigs) < 2 {
		return nil
	}
	sorted := make([]Signature, len(sigs))
	copy(sorted, sigs)
	SortSignatures(sorted)

	var dead []DeadSig
	for j := 1; j < len(sorted); j++ {
		if sorted[j].Fallback {
			continue
		}
		for i := 0; i < j; i++ {
			if sorted[i].Fallback {
				continue
			}
			if sigSubsumes(&sorted[i], &sorted[j]) {
				dead = append(dead, DeadSig{Index: j, Sig: sorted[j], ShadowedBy: sorted[i]})
				break
			}
		}
	}
	return dead
}
