package core

// The collection kernel — the shared machine behind boru's forward
// collection (design/FULL-COMPILATION.0.md §6.2, Stage 2).
//
// Forward collection is not one algorithm. It is THREE loops over the same
// tokens with three different stop-condition sets and three different
// position countings: the phase-1 plan walk (CollectForward here, once per
// dispatch, over the UNION of viable barriers — it EVALUATES), the
// per-candidate scan inside MatchSignature (once per candidate signature,
// over that signature's OWN BarrierPos — it CLASSIFIES), and the arrival
// loop in stepLiteral (once per arriving value — it DISPATCHES). They are
// not refinements of one another, and merging them would be a behaviour
// change rather than a tidy-up: phase ORDER is load-bearing. Phase 1
// commits or strands before the scan's weaker per-signature tests are ever
// consulted, and it pre-evaluates forms out of existence before the scan
// would have to classify them.
//
// So the kernel is not a merged loop. It is the SEAM the three loops walk
// through: a mutable, spliceable, live-length token window plus the host
// evaluations only the host can perform. The Engine is re-seated on it one
// loop at a time — the differential cannot say which re-seat broke
// something otherwise.
//
// Two properties the seam exists to preserve, both learned the hard way:
//
//   - THE WINDOW MUTATION IS THE INTERFACE BETWEEN THE PHASES. The
//     per-candidate scan has no arm for an interp-string, an XML literal or
//     a paren expression, and needs none — phase 1 has already replaced the
//     form with a plain value by the time the scan runs. An adapter that
//     ran the classifying scan over un-pre-evaluated slots would meet forms
//     it cannot classify. Every mutation performed through this seam
//     therefore OUTLIVES the walk, which is why each mutating arm below is
//     gated on some still-viable overload consuming the position: a form
//     rewritten in the window of a pruned overload would have been
//     rewritten for a dispatch that never happens.
//   - SLOT INDICES ARE NOT STABLE ACROSS EVALUATION. A zero-value paren
//     collapse slides the next token into the evaluated slot; a multi-value
//     collapse leaves extras to be re-examined as later positions. Which
//     slots a statement owns is an OUTPUT of running it — which is why the
//     window is live-length and the walk re-reads Len() after every
//     mutation instead of trusting a computed extent.
//
// Scope note. The seam is EXPORTED as of Stage 4: CollectHost, its methods,
// the three loop entry points, and the helper types they name (ViableSig,
// FwdKind). The interpreter is one implementation — *Engine, with its *Tape
// as the window — and the VM's region-descriptor adapter is the second.
//
// Why exporting the INTERFACE is the right shape, and a func-struct is not.
// core/go is held to 100% coverage BY ITS OWN SUITE (cover-gate-core), so
// the question is what each option costs in core statements. Exporting the
// interface RENAMES methods that already exist on *Engine and are already
// covered: zero new statements. A struct of function fields would add one
// delegator per method — statements core's own suite would then have to
// cover for a seam only an outside caller uses. An earlier note here
// recommended the func-struct on a coverage argument that was exactly
// backwards; this is the correction.
//
// The constraint that note was really defending still holds and is not this
// one: no arm of the SHARED ROUTINES below may exist only for the VM. A
// branch core's own suite cannot reach is dead code in its profile and
// fails the gate, whoever calls it. Exporting the seam does not create such
// an arm — the adapter lives in eng and is covered by eng's tests driving
// these same entry points.

// CollectWindow is the token window a collection walk reads and mutates: a
// live-length, spliceable sequence, NOT a frozen slot array. The
// interpreter's *Tape satisfies it as written.
type CollectWindow interface {
	// Len is the window's CURRENT length, re-read after every mutation.
	Len() int
	At(i int) Value
	Set(i int, v Value)
	// Splice replaces count tokens at i with repl, changing the length.
	Splice(i, count int, repl ...Value)
	// Remove drops the token at i. Splice(i, 1) says the same thing; the
	// dedicated method is what the arrival decision actually reaches for,
	// and the window is not the place to make a caller spell a deletion as
	// a degenerate splice.
	Remove(i int)
}

// CollectHost is the evaluator half of the seam: what a collection walk
// cannot do for itself. The two groups are deliberately distinct — the
// EVALUATIONS mutate the window and may raise, the CLASSIFICATIONS are pure
// questions about a token against the live binding set — because the second
// implementation builds them from different material.
type CollectHost interface {
	// Window is the token window this host collects over.
	Window() CollectWindow

	// --- evaluations: these mutate the window and may raise ---

	// EvalGroupAt collapses the paren group whose OpenParen sits at i, in
	// place, to its result value(s).
	EvalGroupAt(i int) error
	// EvalInterp evaluates an interpolated template string to its String
	// value. It does not write the window; the caller does.
	EvalInterp(tok Value) (Value, error)
	// EvalXml evaluates an interpolated XML literal to its Node/Xml value.
	// It does not write the window; the caller does.
	EvalXml(tok Value) (Value, error)
	// ExpandSugarAt lowers a sugar marker at i in place, reporting whether
	// it expanded (false means the marker is a boundary). A lowering that
	// is a function of the marker ALONE may commit; one that is a function
	// of the VIABLE SET may not, which is why the viable slice is a
	// parameter rather than host state.
	ExpandSugarAt(tok Value, pos, i int, viable []ViableSig) (bool, error)
	// FlowInterrupted reports whether flow control (break / continue /
	// return) was raised inside an evaluation, which abandons the walk to
	// the enclosing frame.
	FlowInterrupted() bool
	// ScratchParenSpan wraps items in paren markers using the host's
	// reusable span buffer — valid only until the caller splices it.
	ScratchParenSpan(items []Value) []Value

	// --- classifications: pure questions against the live binding set ---

	// DefTop resolves a name to its active binding, if any.
	DefTop(name string) (Value, bool)
	// IsFnWordBarrier reports whether tok is a bare function word acting as
	// a forward-collection barrier.
	IsFnWordBarrier(tok Value) bool
	// IsReachCallHead reports whether a reach-collapsed named fn at i is a
	// CALL head rather than an operand — the fn-word barrier's value twin
	// (NUR038).
	IsReachCallHead(tok Value, viable []ViableSig, pos, i int) bool
	// StaticForwardType classifies a token by what it presents to signature
	// matching WITHOUT evaluation.
	StaticForwardType(tok Value) (Value, FwdKind)
	// LookupWord resolves a name to its function binding, if any.
	LookupWord(name string) *FnDefInfo
	// ReachFnWouldClaim reports whether a reach-read fn at i would collect
	// from the tokens after it — the call-vs-data decision.
	ReachFnWouldClaim(tok Value, i int) bool
	// ExpandSugarTokens lowers a sugar marker to its tokens WITHOUT writing
	// the window; the caller splices. head selects the binder's name form.
	ExpandSugarTokens(sinfo SugarInfo, tok Value, head bool) ([]Value, error)
}

// The interpreter's seat, asserted at compile time: *Engine is the host and
// its *Tape the window. When the second implementation lands these gain a
// sibling; until then this is what makes "the Engine is re-seated on the
// kernel" a fact the compiler checks rather than a claim in a comment.
var (
	_ CollectHost   = (*Engine)(nil)
	_ CollectWindow = (*Tape)(nil)
)

// CollectForward is the PHASE-1 plan walk, seated on the seam: once per
// dispatch, over the union of the viable signatures' barriers, evaluating
// the groups a still-viable overload consumes and pruning the viable set on
// what it finds. start is the first window index after the dispatching word.
//
// It is the first of the three loops to be re-seated; the per-candidate scan
// and the arrival loop follow separately, because a differential failure has
// to name one re-seat to be worth anything.
func CollectForward(h CollectHost, fn *FnDefInfo, w WordInfo, start int) error {
	win := h.Window()
	// Forward-eligible signatures paired with their effective barrier
	// (the /s and /f modifiers override the declared BarrierPos, mirroring
	// matchSignature's forwardLimit computation).
	viable := make([]ViableSig, 0, len(fn.Signatures))
	maxBarrier := 0
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if sig.Fallback {
			continue
		}
		barrier := effectiveForwardLimit(sig, w)
		if barrier > 0 {
			viable = append(viable, ViableSig{Sig: sig, Barrier: barrier})
			if barrier > maxBarrier {
				maxBarrier = barrier
			}
		}
	}
	if maxBarrier <= 0 {
		return nil
	}

	// viableConsumes reports whether any still-viable signature collects a
	// forward argument at position pos (i.e. pos is within its barrier).
	viableConsumes := func(pos int) bool {
		return viableConsumesAt(viable, pos)
	}

	// pruneViable drops every signature that a concrete forward value at
	// position pos definitely rules out (parity with matchSignature's
	// per-position rejection). Raw/Form/TypeArg slots and Any slots are
	// never used to prune (conservative — keep the signature viable);
	// a concrete Pattern on the slot prunes exactly as patternsOk would
	// reject the position (forwardPatternRejects) — fn's `tnot List`
	// triple sig must fall out of the viable set on a spec-list token,
	// or its 3-token window pre-evaluates groups past the call.
	pruneViable := func(pos int, v Value) {
		kept := viable[:0]
		for _, vs := range viable {
			keep := true
			if pos < vs.Barrier && !sigRawSlot(vs.Sig, pos) {
				if et := SigArgType(vs.Sig, pos); !et.Equal(TAny) && !SigArgMatches(vs.Sig, pos, v) {
					keep = false
				} else if forwardPatternRejects(vs.Sig, pos, v) {
					keep = false
				}
			}
			if keep {
				kept = append(kept, vs)
			}
		}
		viable = kept
	}

	// scanHasKeyword: computed once so the per-token keyword prune below
	// is zero-cost for the overwhelming majority of words, which carry
	// no keyword slots.
	scanHasKeyword := sigsHaveKeywordSlot(viable)

	// prunePatterns is the PATTERN-ONLY prune for values whose TYPE must
	// not prune (a collapsed paren result — multi-value accounting — or a
	// def-bound word's binding, whose matchSignature treatment is
	// contextual). The pattern verdict is position-exact either way: the
	// value tested is what matchSignature's patternsOk will test at this
	// position, so a definite concrete-pattern rejection (the same
	// forwardPatternRejects parity pruneViable uses) is sound. Quote slots
	// are exempt — a /q position captures the word's NAME, not its
	// binding.
	prunePatterns := func(pos int, v Value) {
		kept := viable[:0]
		for _, vs := range viable {
			keep := true
			if pos < vs.Barrier && !sigRawSlot(vs.Sig, pos) &&
				!(vs.Sig.QuoteArgs != nil && vs.Sig.QuoteArgs[pos]) &&
				forwardPatternRejects(vs.Sig, pos, v) {
				keep = false
			}
			if keep {
				kept = append(kept, vs)
			}
		}
		viable = kept
	}

	// pruneResolvedPatterns applies prunePatterns to a token, resolving a
	// WORD through Defs.Top first — the same resolution patternsOk applies
	// before unifying — so the pattern is tested against the binding the
	// matcher will actually see. An unbound word never prunes.
	pruneResolvedPatterns := func(pos int, tok Value) {
		if IsWord(tok) {
			if wi, werr := AsWord(tok); werr == nil {
				if top, ok := h.DefTop(wi.Name); ok {
					prunePatterns(pos, top)
				}
			}
			return
		}
		prunePatterns(pos, tok)
	}

	pos := 0
	scanIdx := start
	for pos < maxBarrier && scanIdx < win.Len() {
		tok := win.At(scanIdx)

		// Boundary tokens (engine structurals, end / `)`): stop scanning.
		if scanBoundaryToken(tok) {
			break
		}

		// A sugar marker expands HERE — once per dispatch, before
		// matchSignature's per-candidate scans (which must never mutate
		// the tape per sig). A marker the expansion helper declines is a
		// boundary; a selected-head expansion failure is the user's
		// syntax error, surfaced now.
		if IsSugar(tok) {
			expanded, serr := h.ExpandSugarAt(tok, pos, scanIdx, viable)
			if serr != nil {
				return serr
			}
			if !expanded {
				break
			}
			continue
		}

		// Keyword slots are decided by the raw token at their position —
		// prune before any group evaluation or word expansion below, so a
		// keyword overload's larger arity never widens the scan past the
		// dispatch the non-keyword overloads will actually make: `def g
		// (fn […]) (g 3)` must not pre-evaluate `(g 3)` before the
		// 2-arg def binds g.
		if scanHasKeyword {
			viable = pruneKeywordViable(viable, pos, tok)
		}

		// Open paren: a forward group of unknown type.
		if IsOpenParen(tok) {
			// Structure-first gate: evaluate ONLY if some still-viable
			// overload consumes a forward argument at this position.
			// Otherwise no signature wants a value here — leave the
			// paren raw so matchSignature treats it as a boundary.
			if !viableConsumes(pos) {
				break
			}
			if err := h.EvalGroupAt(scanIdx); err != nil {
				return err
			}
			// Flow-control raised inside the paren: let the outer Run
			// frame resolve it (parity with the former preEvalParens).
			if h.FlowInterrupted() {
				return nil
			}
			// The paren collapsed to its result value(s) at scanIdx; count
			// it as one resolved position and advance, exactly as the
			// former scan did. (The result's runtime type is not used to
			// prune further: a group can collapse to zero or many values,
			// so we keep the conservative one-slot accounting.) A concrete
			// PATTERN mismatch does prune: whatever now sits at scanIdx is
			// exactly what matchSignature will test at this sig position,
			// so a sig whose pattern definitely rejects it can never be
			// selected here — without this, a paren-spelled spec list
			// (`fn (quote [[…]]) …`) left fn's 3-token triple window open
			// and pre-evaluated the NEXT statement's groups. A WORD at
			// scanIdx (the group collapsed to zero values and the next
			// token slid in) prunes only through its resolved binding.
			// A tagged reach-collapsed named fn that WOULD CLAIM its
			// next token is a CALL head — the group resolved a callee,
			// not an operand: stop the scan (the fn-word barrier's
			// value twin, NUR038). A claim-less one is an operand, and
			// so is one filling a FUNCTION slot of the collecting word
			// (`usurp (m dot a)` — the higher-order consumer wants the
			// fn itself; Any slots stay barred).
			res := win.At(scanIdx)
			if h.IsReachCallHead(res, viable, pos, scanIdx) {
				break
			}
			pruneResolvedPatterns(pos, res)
			pos++
			scanIdx++
			continue
		}

		// Interpolated template string: an expression, not a value — its
		// type is only knowable after evaluation (always a String).
		// Treated like a paren group: when a still-viable overload
		// consumes this position, evaluate it in place so a typed slot
		// (`raise` msg:String, `add`'s Scalar overload) sees the String
		// it will actually receive. Left raw, the token's internal
		// InterpString type would prune every typed signature and a
		// `raise `bad: ${x}`` mis-dispatched to the 0-arg fallback.
		if IsInterpString(tok) {
			if !viableConsumes(pos) {
				break
			}
			result, err := h.EvalInterp(tok)
			if err != nil {
				return err
			}
			result.pos = tok.pos
			win.Set(scanIdx, result)
			pruneViable(pos, result)
			pos++
			scanIdx++
			continue
		}

		// Interpolated XML literal: same as InterpString — its type
		// (Node/Xml) is only knowable after evaluation, so evaluate in
		// place when a viable overload consumes this position, then prune.
		if IsXmlInterp(tok) {
			if !viableConsumes(pos) {
				break
			}
			result, err := h.EvalXml(tok)
			if err != nil {
				return err
			}
			result.pos = tok.pos
			win.Set(scanIdx, result)
			pruneViable(pos, result)
			pos++
			scanIdx++
			continue
		}

		// Paren expression value (paren-nesting Step 3): expand it back to
		// its OpenParen … CloseParen marker span in place, then re-process
		// — the IsOpenParen branch above collapses it on THIS engine. See
		// design/legacy/PAREN-REPRESENTATION.9.ignore Step 3.
		if IsParenExpr(tok) {
			// Step 4: a quote-captured ParenExpr (already Quoted) or a
			// raw-capture forward position is left unevaluated so the
			// matched sig captures the paren as code.
			if tok.Quoted || rawParenForward(fn, pos) || rawFormForward(fn, pos) {
				pos++
				scanIdx++
				continue
			}
			peItems, _ := AsParenExpr(tok)
			win.Splice(scanIdx, 1, h.ScratchParenSpan(peItems)...)
			continue
		}

		// A Reach in the forward window evaluates like a ParenExpr (Reach
		// Phase B): expand to its lowered get-chain marker span in place,
		// then re-process. Quoted/raw-capture reaches are left for the
		// matched sig (parity with the ParenExpr branch above).
		if IsReach(tok) {
			if !isEvalReach(tok) || rawParenForward(fn, pos) || rawFormForward(fn, pos) {
				pos++
				scanIdx++
				continue
			}
			// A dot-access chain is a single-value navigation, not a
			// forward-collection barrier (see the statement-branch Reach
			// hook): pre-evaluate it into the collecting word's slot exactly
			// like a paren group — uniformly, strict or not. Only a bare
			// function word that collects its own args stops the scan
			// (below). design/STRICT-FORWARD-BARRIER.0.md.
			info, _ := AsReach(tok)
			win.Splice(scanIdx, 1, expandReach(info)...)
			continue
		}

		// A word def-bound to a DATA __SP splice marker occupies its
		// forward position as the paren group (w) — the `f w ≡ f (w)`
		// equivalence: a plain (non-function) def-bound word expands into
		// the token stream wherever it stands. Rewriting the token to
		// ParenExpr([w]) and reprocessing routes it through the ParenExpr/
		// OpenParen branches above, so evaluation gating, multi-value
		// collapse, and raw-capture handling are byte-identical to a
		// written (w). Exemptions: positions no still-viable overload
		// consumes (viableConsumes — the rewrite is a TAPE MUTATION that
		// outlives this dispatch, so a word in the window of a pruned
		// overload must stay a word for the NEXT word to capture; the
		// paren/interp branches above gate the same way), structural-
		// capture slots (/q takes the word's NAME, a KEYWORD slot takes the
		// matching literal word, form/raw/type slots take the raw token —
		// see capturesForwardToken), code-bearing splices (Forth-style
		// macros that must run against the live stack — see spliceIsData),
		// and binder operands (`def y xs` rebinds the MARKER so y aliases
		// the splice — see bindsReferent).
		if IsWord(tok) && viableConsumes(pos) && !bindsReferent(fn.Name) && !capturesForwardToken(fn, pos, tok) {
			if wi, werr := AsWord(tok); werr == nil {
				if top, ok := h.DefTop(wi.Name); ok && IsSplice(top) {
					if info, serr := AsSplice(top); serr == nil && spliceIsData(info) {
						pe := NewParenExpr([]Value{tok})
						pe.pos = tok.pos
						win.Set(scanIdx, pe)
						continue
					}
				}
			}
		}

		// A registered FUNCTION word in the forward window that the collecting
		// word does NOT capture is the NEXT dispatch — the runtime's "another
		// function word is a barrier" rule (commitBarrierForward) stops forward
		// collection here, and the pre-evaluation scan must stop too. The
		// former scan counted the word as one resolved forward position and
		// kept going, so a LATER group was pre-evaluated ACROSS the barrier
		// once the COLLECTING word's own max arity (maxBarrier) reached past
		// it. With a heterogeneous-arity overload — e.g. a 3-arg `add` —
		// `(g) add (g) add (g)` evaluated the third group before the first add
		// ran; the recorded events then put both later operands on the
		// simulated stack and the operand layout declined "not adjacent on
		// top". Stop so each dispatch pre-evaluates only the groups IT
		// collects, in source order. Lookup mirrors commitBarrierForward's own
		// function-word test. The capturesForwardToken guard preserves a word
		// the collecting sig takes STRUCTURALLY as an operand (a /q name like
		// `undef foo`, a raw/form/type slot, a matching KEYWORD literal like
		// def's `fn`) — there the function word is the argument, not a
		// barrier, and the scan must walk past it.
		if IsWord(tok) && !capturesForwardToken(fn, pos, tok) && h.IsFnWordBarrier(tok) {
			break
		}

		// A tagged reach-collapsed named fn already in the window (a
		// re-plan after the arrival gate closed a statement) that WOULD
		// CLAIM its next token is a CALL head — the fn-word barrier's
		// value twin (NUR038): stop. A claim-less one is an operand, and
		// so is one filling a FUNCTION slot of the collecting word.
		if h.IsReachCallHead(tok, viable, pos, scanIdx) {
			break
		}

		// Non-group token. A concrete literal carries a final type that
		// matchSignature tests identically, so it is sound to prune the
		// viable set on it. Words and other non-concrete tokens are left
		// un-pruned (their matchSignature treatment is contextual) but are
		// still counted as one resolved position — so, exactly like the
		// former scan, groups beyond a NON-FUNCTION word remain reachable.
		if mt, kind := h.StaticForwardType(tok); kind == FwdValue {
			pruneViable(pos, mt)
		} else if IsWord(tok) {
			// A def-bound word's TYPE stays un-pruned (contextual), but
			// its concrete-PATTERN verdict is exact — patternsOk resolves
			// the word through Defs.Top the same way before unifying — so
			// a sig whose pattern rejects the binding can never be
			// selected with this word at this position. Without this, a
			// word-spelled spec list (`def sw quote [[…]]  fn sw …`) left
			// fn's 3-token triple window open past the call.
			pruneResolvedPatterns(pos, tok)
		}
		pos++
		scanIdx++
	}
	return nil
}

// CollectCandidateScan is the PER-CANDIDATE scan, seated on the seam: once
// per candidate signature, over THAT signature's own forward limit, it
// CLASSIFIES the tokens a match would claim and records where each one
// landed. It is the loop that actually matches, and — unlike the phase-1
// plan walk — it must not evaluate: running once per candidate, any
// evaluation it committed would fire for overloads that never dispatch.
//
// It has no arm for an interp-string, an XML literal or a paren expression,
// and needs none: phase 1 has already replaced those forms with plain
// values by the time this runs. That is the window-mutation property this
// file opens with, seen from the consuming side.
//
// The one lowering it does commit is a sugar expansion, and only because
// every kind it admits has a single deterministic lowering — a function of
// the MARKER alone, never of the viable set. The Angle marker, whose
// head/use-site form depends on which overload wins, is a plain boundary
// here and is decided at arrival instead.
//
// positions is written in place; fwd is how many parameters the forward
// tokens filled and specAt the first slot a dispatching word filled (-1 for
// none).
//
// checkActive and compiling are hoisted by the caller rather than asked
// here. Both are per-DISPATCH facts, so the per-signature loop asks them
// once instead of once per candidate — and `compiling` is only ever read
// under `checkActive`, so the caller computes it under the same guard and
// the runtime hot path never asks at all. They are plain bools rather than
// host methods for the same reason: a seam method that only one arm calls,
// under a condition the rest of the walk does not reach, is a method whose
// only proof of life is that one arm.
func CollectCandidateScan(h CollectHost, sig *Signature, forwardLimit int, positions []int, start int, checkActive, compiling bool) (fwd, specAt int) {
	win := h.Window()
	specAt = -1
	scanIdx := start

	// One inner loop over parameters, matching forward tokens.
	for fwd < forwardLimit && scanIdx < win.Len() {

		tok := win.At(scanIdx)
		expectedType := SigArgType(sig, fwd)

		// 1.4: structural boundaries (forward / mark / move /
		// internal / return-check) and `end` / `)`. ONE predicate,
		// shared with the phase-1 plan walk (resolveForwardArgs),
		// which asks the identical question of the same tokens.
		//
		// The two loops legitimately DIFFER on other arms — an open
		// paren is pre-evaluated there and a hard boundary here — so
		// the arms where they agree are exactly the ones that must
		// not be written twice and left to drift apart. Collection
		// is three loops over the same tokens with three stop
		// condition sets (design/FULL-COMPILATION.0.md §6.2); every
		// decision they share belongs in one place.
		if scanBoundaryToken(tok) {
			break
		}

		// 1.5: open parens are pre-evaluated by preEvalParens
		// before matching begins. If one remains, treat as boundary.
		if IsOpenParen(tok) {
			break
		}

		// FormArgs (macro raw capture): accept ANY form at this
		// position — a word stays a Word, a paren/list/literal stays
		// as-is — with no resolution, no dispatch, no Word→Atom
		// coercion, and no function-word boundary. The operand is
		// captured unevaluated. See design/legacy/MACROS-PHASE1.10.ignore §3.
		if sig.FormArgs != nil && sig.FormArgs[fwd] {
			positions[fwd] = scanIdx
			fwd++
			scanIdx++
			continue
		}

		if IsWord(tok) {
			ww, _ := AsWord(tok)
			// /q modifier: capture the upcoming Word as an Atom
			// (the conversion happens at insertForward / stepLiteral
			// time; here we just count it as a match).
			if sig.QuoteArgs != nil && sig.QuoteArgs[fwd] {
				if TAtom.ConformsTo(expectedType) {
					positions[fwd] = scanIdx
					fwd++
					scanIdx++
					continue
				}
				break
			}

			// Defined word: resolves to its def type.
			if top, ok := h.DefTop(ww.Name); ok {
				// Gradual typing: an Any-typed forward operand — a value
				// flowed from a dynamic `get`, or a param bound to Any at
				// a gradual call site — is optimistically accepted for a
				// concrete param in PURE CHECK mode. At runtime the value
				// is concrete and dispatches (or raises) exactly as the
				// interpreter does, so the static analysis stays advisory
				// rather than emitting a spurious no_signature. NOT in
				// compile mode: there the dispatch must remain UNMATCHED so
				// the emitter declines (force-compile) instead of baking a
				// wrong direct call — preserving compile==interpret.
				// A TYPE binding denotes its lattice node (the Stage 2
				// flip — deftable.Top), so the same plan-time guard the
				// builtin-name arm below carries applies here: a type
				// literal is declined at a concrete-payload slot, so the
				// plan never claims what the commit re-match would
				// reject.
				if IsBareTypeNode(top) {
					isTypeArg := sig.TypeArgs != nil && sig.TypeArgs[fwd]
					if !isTypeArg && rejectsTypeLiteral(top, expectedType) {
						break
					}
				}
				gradualAny := checkActive && !compiling &&
					top.Parent != nil && top.Parent.Equal(TAny)
				if SigArgMatches(sig, fwd, top) || expectedType.Equal(TAny) || gradualAny {
					// A dispatching binding (FnDefInfo) planned as an
					// operand is SPECULATIVE: at runtime this token
					// dispatches rather than arriving as a value
					// (the `def name fn […]` idiom relies on exactly
					// that — fn runs and its result completes def).
					// Record the first such slot so the parked
					// ForwardInfo carries the plan's stop condition.
					// A slot that specifically expects a Function
					// gets the word as a resolved REFERENCE at
					// collection time (stepWord's TFunction
					// intercept) — consistent, not speculative.
					if _, isFn := top.Data.(FnDefInfo); isFn &&
						specAt == -1 && !expectedType.Equal(TFunction) {
						specAt = fwd
					}
					positions[fwd] = scanIdx
					fwd++
					scanIdx++
					continue
				}
				if _, ok := top.Data.(FnDefInfo); !ok {
					break // simple def, type mismatch
				}
			}

			// (A def-bound TYPE name is fully handled by the
			// Defs.Top arm above — post the Stage 2 flip the
			// binding denotes its bare node, which that arm
			// either claims or rejects terminally, so no
			// separate TopTypeBody mirror remains.)

			// 1.4: function word — boundary, stop. A `/v`-marked
			// word is NO boundary in principle (it denotes its
			// REFERENCE value, NUR050/G12) — but since the ADR-011
			// collapse its Defs binding IS a Function value, so
			// every slot that can admit the reference (a Function
			// slot, an Any slot) already claimed it in the
			// def-binding branch above; a /v word reaching here
			// faces a slot no Function can fill and stops the scan
			// exactly like its unmarked twin. (Lookup and Defs.Top
			// read the same store, so this arm is only reached on
			// the def-binding branch's typed fall-through.)
			if h.LookupWord(ww.Name) != nil {
				break
			}

			// Known literals: true/false → Boolean, type names → type literal.
			if ww.Name == "true" || ww.Name == "false" {
				if SigArgMatches(sig, fwd, Value{Parent: TBoolean}) || expectedType.Equal(TAny) {
					positions[fwd] = scanIdx
					fwd++
					scanIdx++
					continue
				}
				break
			}
			if tn, isType := ResolveBuiltinTypeName(ww.Name); isType {
				lit := NewTypeLiteral(tn)
				if SigArgMatches(sig, fwd, lit) {
					// Same admission a future LITERAL token gets
					// (the block below): a type literal is declined
					// at a concrete-payload slot, so the plan never
					// claims what the commit re-match would reject.
					isTypeArg := sig.TypeArgs != nil && sig.TypeArgs[fwd]
					if !isTypeArg && rejectsTypeLiteral(lit, expectedType) {
						break
					}
					positions[fwd] = scanIdx
					fwd++
					scanIdx++
					continue
				}
				break
			}

			// Undefined word: always resolves to Atom.
			if SigArgMatches(sig, fwd, Value{Parent: TAtom}) || expectedType.Equal(TAny) {
				positions[fwd] = scanIdx
				fwd++
				scanIdx++
				continue
			}
			break // type mismatch
		}

		// Open paren marker: boundary, stop forward scan.
		if IsOpenParen(tok) { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
			break
		}

		// A reach-collapsed NAMED function value (the transient
		// ReachGroup tag) that WOULD CLAIM its next token is a
		// CALL head — the value twin of the fn-word boundary
		// above (NUR038): it stops the forward scan exactly as
		// its bare-word spelling would. One with no claim is an
		// operand (a branch arm, a reference) and scans on, as
		// does one filling this sig's own Function slot.
		if tok.ReachGroup && !tok.Quoted && isFnDefValue(tok) &&
			!sigWantsFunctionAt(sig, fwd) &&
			h.ReachFnWouldClaim(tok, scanIdx+1) {
			break
		}

		// A /q (QuoteArgs) position captures a literal word/ATOM (the
		// IsWord branch above handles a raw word). A non-concrete carrier
		// whose type is NOT atom-family — a computed check-mode value such
		// as the pre-evaluated result of `quote (s get k)` (an Any/Integer
		// carrier) — is not an atom, so it must not fill the /q slot via
		// the Any-conforms-to-everything rule: that would pick quote's
		// word-capture sig ([TAtom], QuoteArgs) over its value sig ([TAny],
		// ReturnsIdentity), fail to compile, and (since the /q handler is
		// quoteWordHandler) never run the value path. A genuine Atom
		// carrier (e.g. `set (quote name) v`) DOES conform and still
		// matches. Inert at runtime (operands are concrete there). Mirrors
		// the stack-phase and positionalMatch /q guards. See
		// design/legacy/module-fn-checkstate-ownership.2.ignore.
		if sig.QuoteArgs != nil && sig.QuoteArgs[fwd] && tok.Carrier && !IsConcrete(tok) && !tok.Parent.ConformsTo(TAtom) {
			break
		}

		// A sugar marker EXPANDS in place during the scan
		// (sugar.go): the tokens it lowers to — a fn word, a
		// ParenExpr — then get exactly the treatment the
		// pre-marker parser output got. The current slot's
		// QuoteArgs flag selects the Angle marker's head form
		// (the binder's name slot). An unexpandable marker is
		// a boundary; it errors at step time.
		if IsSugar(tok) {
			sinfo, sok := AsSugar(tok)
			if !sok { //covergate:allow IsSugar guarantees a SugarInfo payload
				break
			}
			// The Angle marker's head/use-site choice belongs at
			// ARRIVAL (stepSugar's pending-forward probe): this
			// scan runs once per CANDIDATE sig, so committing a
			// choice here would mutate the tape for the wrong
			// overload. It is a plain boundary.
			if sinfo.Kind == SugarAngle {
				break
			}
			exp, serr := h.ExpandSugarTokens(sinfo, tok, false)
			if serr != nil {
				break
			}
			win.Splice(scanIdx, 1, exp...)
			continue
		}
		// Literal value: direct type check.
		if SigArgMatches(sig, fwd, tok) || expectedType.Equal(TAny) {
			isTypeArg := sig.TypeArgs != nil && sig.TypeArgs[fwd]
			if !isTypeArg && rejectsTypeLiteral(tok, expectedType) {
				break // reject type literal at concrete-payload sig
			}
			positions[fwd] = scanIdx
			fwd++
			scanIdx++
			continue
		}

		// *Type mismatch — stop forward scanning.
		break
	}
	return fwd, specAt
}

// ArrivalVerdict is what the ARRIVAL decision concludes about a value that
// has reached the pointer with a forward collection pending. It is a
// verdict and not an action on purpose: the three non-collect outcomes are
// DISPATCHES — entering a function, committing a parked forward, resolving
// one from the stack — and dispatching is the host's job, not the
// collection kernel's. Keeping the boundary there is what stopped this
// re-seat from dragging the recorder, the tracer, the fn-frame probe and
// the fn-value sealer through the seam with it: an interface wide enough to
// carry those would be the Engine wearing a different name, and the second
// implementation could not satisfy it from its own material.
type ArrivalVerdict int

const (
	// ArrivalCollect — the value fills the pending slot. The host commits
	// it into the window and advances the collection counters.
	ArrivalCollect ArrivalVerdict = iota
	// ArrivalDispatchFn — the value is a reach-read fn with a 0-arg
	// overload, so the dot-read is a PROPERTY call. The host dispatches it
	// in place; it consumes nothing, so no cross-statement swallow is
	// possible, and its RESULT arrives at this still-pending window.
	ArrivalDispatchFn
	// ArrivalBarrierClose — the value is a reach-read fn that WOULD CLAIM
	// the tokens after it, so it is the NEXT dispatch (NUR038, the value
	// twin of the fn-word collection barrier). The host commits the parked
	// forward with what it already holds, or resolves it from the stack
	// when no smaller-arity overload can fire; either way the window closes
	// here and the fn re-steps as its own statement.
	ArrivalBarrierClose
	// ArrivalImplicitEnd — the value does not match the slot. The host
	// resolves the forward from the stack.
	ArrivalImplicitEnd
)

// CollectArrival is the ARRIVAL decision, seated on the seam: once per value
// reaching the pointer with a collection pending, does this value fill the
// next slot?
//
// It is the third of the three collection loops, and the one whose shape the
// design had wrong. `stepLiteral` is not a collection loop end to end — it
// is form expansion, then a standalone-value path (splices, dispatch
// modifiers, shaped-method dispatch, the recorder push), then this decision,
// then the commit-and-maybe-dispatch bookkeeping. Only the decision is
// collection. Re-seating the whole of it would have needed some seventeen
// further host methods — e.recorder, e.trace, e.inFnFrame, e.sealFnValue,
// e.rearrangeForForward, e.pendingForwardIdx and the rest — which is not a
// seam but an Engine with extra steps.
//
// So the extraction takes the decision and leaves the dispatches: the two
// in-place window mutations it DOES own (the /q Word→Atom conversion and
// the `/v` marker consumption) are exactly the ones a match verdict depends
// on, and the host re-reads the window afterwards to see them.
func CollectArrival(h CollectHost, fwd ForwardInfo, valIdx int) ArrivalVerdict {
	if fwd.CollectedArgs >= fwd.ExpectedArgs {
		return ArrivalCollect
	}
	win := h.Window()
	val := win.At(valIdx)
	nextIdx := fwd.CollectedArgs
	matches := SigArgMatches(fwd.Sig, nextIdx, val)
	// A /q-marked TAtom slot accepts a Word: convert it in place so the
	// eventual handler sees a uniform Atom rather than having to extract a
	// name from either shape.
	if !matches && fwd.Sig.QuoteArgs != nil && fwd.Sig.QuoteArgs[nextIdx] &&
		val.Parent.Equal(TWord) && TAtom.ConformsTo(SigArgType(fwd.Sig, nextIdx)) {
		w, _ := AsWord(val)
		atom := NewAtom(w.Name)
		atom.pos = val.pos // preserve source position across /q Word→Atom conversion
		win.Set(valIdx, atom)
		matches = true
	}
	// A named function that a REACH-LOWERED group collapsed to (the
	// transient ReachGroup tag) and that WOULD COLLECT from the tokens after
	// it is a CALL, not data — the value twin of the fn-word collection
	// barrier (NUR038). A bare fn word in the window stops collection; the
	// SAME function reached through a dot-access (`5 m.p m.p 7`,
	// `IO.printstr "A" IO.printstr "B"` — the second callee resolving
	// mid-collection with ITS argument right after it) must stop it too, or
	// the open window swallows the next statement whole. The call-vs-data
	// decision mirrors execFnDefLiteral's own: a reach-read fn with NOTHING
	// to claim stays data (`typeof IO.stdin`, `def sqrt MathUtil.sqrt` — the
	// pinned reference idioms). A slot that SPECIFICALLY expects a Function
	// always admits (the designed reference intercept, e.g. `each`);
	// explicit data intent spells `/v` — either already Quoted, or the
	// group's trailing Word/__DM marker consumed here exactly as
	// execFnDefLiteral's peek does (`def g M.w/v`: the fn arrives
	// mid-collection before that peek can run); user-written reference
	// expressions ((inc/v), (usurp sub2)) carry no tag.
	if matches && val.ReachGroup && !val.Quoted &&
		!SigArgType(fwd.Sig, nextIdx).ConformsTo(TFunction) {
		marked := false
		if valIdx+1 < win.Len() {
			if _, ok := AsDispatchMod(win.At(valIdx + 1)); ok {
				win.Remove(valIdx + 1)
				val.Quoted = true
				win.Set(valIdx, val)
				marked = true
			}
		}
		switch {
		case marked:
			// `/v` data intent — collected as the reference.
		case fnValueHasZeroArgSig(val):
			return ArrivalDispatchFn
		case h.ReachFnWouldClaim(val, valIdx+1):
			return ArrivalBarrierClose
		}
	}
	if !matches {
		return ArrivalImplicitEnd
	}
	return ArrivalCollect
}
