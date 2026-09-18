package core

// patternsOk runs Signature.Patterns against the matched arg
// positions. `fwd` is the count of positions filled from forward
// tokens; positions [0..fwd) are forward args and [fwd..) are stack
// args.
//
// Forward vs stack handling:
//
//   - Scalar-literal patterns (Integer, Float, String, Boolean,
//     Atom — concrete `Data != nil` payloads) are checked on EVERY
//     position. This is the §1.1 entry point: a sig with
//     `Patterns[0] = NewInteger(0)` must reject any non-zero arg
//     regardless of which side it came from.
//   - Structural patterns (record/map shapes, `OpenUnifyMap`
//     candidates) are checked ONLY on stack-matched positions.
//     The legacy semantics — that handlers may further constrain
//     forward args inside the handler body — depends on this skip.
//     Tightening it would break callers like `create` whose 1-arg
//     `(Map) Patterns={kind:"api"}` sig was previously matched on
//     non-api maps when the handler then routed by stack contents.
func patternsOk(sig *Signature, positions []int, tape *Tape, fwd int, r *Registry) bool {
	for idx := 0; idx < sig.TotalArgs(); idx++ {
		pattern, ok := SigPattern(sig, idx)
		if !ok {
			continue
		}
		// A CARRIER pattern is a check-mode placeholder for a COMPUTED
		// pattern value (`fn (2 add 3) [String] ['five']` constructs its
		// input pattern from a carrier during the check pass): the real
		// value is unknown statically, so enforcement is skipped —
		// gradual, like every other unknowable check-mode operand. At
		// runtime patterns are always concrete (the construction ran
		// with live values), so this branch is check-mode-only.
		if pattern.Carrier {
			continue
		}
		if idx >= len(positions) {
			continue
		}
		isForward := idx < fwd
		val := tape.At(positions[idx])
		// KEYWORD slot: a /q position whose pattern is a concrete Atom
		// matches the raw token NAME. A /q capture is binding-agnostic
		// ("capture trumps binding" — FnParam.Quote), so the pattern
		// must NOT unify against a resolved binding: a Defs-bound
		// native like `fn` would never match its own name (its binding
		// is an FnDefInfo, not an Atom). Word and Atom tokens compare
		// by name; anything else fails the slot. This is what lets a
		// signature admit one specific literal word — `def`'s
		// `[name/q fn/q sigs]` form, and the boru-authorable equivalent
		// for macros/binders (syntax-rules-style keyword literals).
		if sig.QuoteArgs != nil && sig.QuoteArgs[idx] && IsAtom(pattern) {
			name := ""
			if w, werr := AsWord(val); werr == nil {
				name = w.Name
			} else if a, aerr := AsAtom(val); aerr == nil {
				name = a
			} else {
				return false
			}
			if pn, perr := AsAtom(pattern); perr != nil || pn != name {
				return false
			}
			continue
		}
		// A forward position may still hold the unresolved Word token
		// for a def-bound value (the forward scan plans against
		// Defs.Top but does not rewrite the tape). Resolve it the same
		// way before unifying — otherwise a typed-list/map pattern
		// (`xs:[:Integer]`) rejects `f zs` while accepting the literal
		// `f [1 2]`, purely by spelling.
		if isForward && r != nil && IsWord(val) {
			if w, werr := AsWord(val); werr == nil {
				if top, ok := r.Defs.Top(w.Name); ok {
					val = top
				}
			}
		}
		if pattern.Parent.Equal(TMap) && val.Parent.Equal(TMap) &&
			pattern.Data != nil && val.Data != nil &&
			!IsOptionsType(pattern) &&
			!IsRecordType(val) && !IsTypedMap(val) && !IsOptionsType(val) {
			if isForward {
				// Legacy: structural map patterns only enforced on
				// stack positions. See doc comment.
				continue
			}
			if !OpenUnifyMap(pattern, val) {
				return false
			}
			continue
		}
		// Concrete scalar pattern? Always check.
		// *Type-literal / non-concrete pattern on a forward position?
		// Skip — handlers may further constrain inside the body.
		if isForward && !IsConcrete(pattern) {
			continue
		}
		// A negation pattern takes the direct unifyNegation path —
		// skipping Unify's ResolveWordsDeep prepass, which deep-resolves
		// (and rebuilds) every list operand per call and dominated
		// fn-construction dispatch: each spec-list `fn [[…]]` call tests
		// the triple sig's tnot-List pattern here and at plan time. A
		// negation-of-type verdict depends only on the operand's
		// top-level shape, never on nested word resolution, so the
		// direct path is verdict-identical.
		if ni, err := AsNegation(pattern); err == nil {
			if _, uerr := unifyNegation(ni, val, nil); uerr != nil {
				return false
			}
			continue
		}
		if _, uOk := Unify(val, pattern); !uOk {
			return false
		}
	}
	return true
}

// forwardPatternRejects reports whether a concrete forward value at sig
// position pos DEFINITELY fails the position's declared Pattern — the
// same verdict patternsOk reaches for a forward-matched position (the
// isForward branch above). resolveForwardArgs consults it to prune an
// overload from the paren-pre-evaluation viable set: a sig whose
// concrete pattern rejects the token can never be selected with that
// token at that position (matchSignature's forward scan cannot skip a
// type-matching token, and patternsOk then rejects the fill), so
// pruning keeps evaluation gating in lockstep with dispatch — without
// it, fn's `tnot List` triple sig kept a spec-list call's 3-token
// window open and pre-evaluated the NEXT statement's paren group
// (`def sub2 fn [list] (sub2/v) …` failed on the not-yet-installed
// name). Keep the skip rules here mirrored with patternsOk: structural
// map patterns and non-concrete patterns are NOT enforced on forward
// positions.
func forwardPatternRejects(sig *Signature, pos int, val Value) bool {
	pattern, ok := SigPattern(sig, pos)
	if !ok {
		return false
	}
	if pattern.Parent.Equal(TMap) && val.Parent.Equal(TMap) &&
		pattern.Data != nil && val.Data != nil &&
		!IsOptionsType(pattern) &&
		!IsRecordType(val) && !IsTypedMap(val) && !IsOptionsType(val) {
		return false // structural map pattern: stack-only enforcement
	}
	if !IsConcrete(pattern) {
		return false // type-literal / non-concrete pattern: not forward-enforced
	}
	// Direct unifyNegation path for negation patterns — see the twin
	// fast path in patternsOk: skips ResolveWordsDeep, verdict-identical.
	if ni, err := AsNegation(pattern); err == nil {
		_, uerr := unifyNegation(ni, val, nil)
		return uerr != nil
	}
	_, uOk := Unify(val, pattern)
	return !uOk
}

// OpenUnifyMap checks whether candidate contains at least the key-value pairs
// of pattern. Extra keys in candidate are allowed (open/subset matching).
//
// This is an asymmetric subset match, not a unifier — it returns only
// ok/!ok and never produces a unified value. Lives next to patternsOk
// because both are matching primitives used by signature dispatch.
//
// Missing-key rule: when a pattern key is absent on the candidate,
// synthesise an Absent value and unify the pattern's value against it.
// A pattern value containing `Absent` as a disjunct alternative (the
// `?:T` desugaring) accepts it; any other shape rejects it. This
// implements the "? means None or absent" rule via the type system —
// no out-of-band optional-key metadata required.
func OpenUnifyMap(pattern, candidate Value) bool {
	return openUnifyMap(pattern, candidate, nil)
}

// openUnifyMap is OpenUnifyMap with the enclosing unify chain's
// registry: the disjunct walk (unifyDisjunct) runs the subset match on
// a concrete map alternative from INSIDE a unify, so each per-key
// unify keeps the chain armed. The public entry passes nil.
func openUnifyMap(pattern, candidate Value, r *Registry) bool {
	pMap, _ := AsMap(pattern)
	cMap, _ := AsMap(candidate)

	// Non-concrete map shapes (a typed map's ChildTypeInfo, record /
	// options bodies) have no OrderedMap to walk — AsMap returns nil
	// for them. Route those pairs through the full unifier, whose map
	// family owns typed-vs-concrete and record matching, rather than
	// panicking on pMap.Keys(). Callers' guards vary; this is the
	// single defensive boundary.
	if pMap == nil || cMap == nil {
		_, uerr := unifyWithin(pattern, candidate, r)
		return uerr == nil
	}

	absentVal := NewTypeLiteral(TAbsent)
	for _, key := range pMap.Keys() {
		pVal, _ := pMap.Get(key)
		cVal, ok := cMap.Get(key)
		if !ok {
			if _, uerr := unifyWithin(pVal, absentVal, r); uerr != nil {
				return false
			}
			continue
		}
		if _, uerr := unifyWithin(pVal, cVal, r); uerr != nil {
			return false
		}
	}
	return true
}
