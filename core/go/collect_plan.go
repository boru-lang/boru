package core

// The plan-level matcher, extracted from the Engine onto the collection
// seam so the VM's routed dispatch reads the same rule (see PlanMatch).

// PlanMatch is the unified signature matching function — the PLAN-level
// matcher, seated on the collection seam (design/FULL-COMPILATION.0.md
// §6.2, Stage 4): the interpreter calls it through Engine.MatchSignature
// over its tape, and the VM's routed dispatch (eng, OpDispatchGeneric)
// over a descriptor window laid out the same way — the resolved stack
// values, the word at `pointer`, the forward tokens after it. Moved here
// from Engine.MatchSignature textually, receiver fields becoming
// parameters (h for the per-candidate scan's host, win for the window,
// reg for the check-mode notes, pointer for the word's index, and the
// three per-dispatch facts the method computed), so both hosts read ONE
// implementation of the rule that decides which signature a call takes.
//
// Algorithm:
//
//	0.1 Using the ordered signatures, attempt to match in order,
//	    stopping at the first match.
//	1.1 If stack-only (or /s) and not /f: skip forward, go to step 2.
//	    If stack-only but /f: override → do forward scan.
//	1.2 Match each parameter in order against future tokens.
//	1.3 Stop if all params matched, or if /N params reached.
//	1.4 Move to step 2 if you hit a boundary condition:
//	    a function word, a pipe barrier, or "end".
//	1.5 If you hit an open paren, treat as boundary (pre-evaluated).
//	2.1 Match the remaining parameters against the stack, working
//	    backwards (top of stack first).
//	2.2 Stop once all or /N params reached.
//
// This is implemented as one outer loop over signatures and one inner
// loop over parameters. No separate functions are called for matching.
//
// Returns: matched signature, arg positions (absolute stack indices
// in signature order), and the speculation marker. Positions > pointer
// are forward args that need deferred collection. Positions < pointer
// are stack args. Returns nil sig if no signature matches.
//
// The third return is the sig-order index of the FIRST forward slot
// the plan filled with a WORD bound to a dispatching definition (an
// FnDefInfo binding accepted as an operand — typically through an
// Any-typed slot), or -1 when no slot was filled that way. Such a
// token is planned as an operand but DISPATCHES at runtime — the
// plan-time stop condition insertForward records on ForwardInfo so
// the arrival side can observe it. See
// design/FORWARD-COLLECTION-PHASES.10.md.
//
// COMPLEXITY: this function once carried //nolint:gocyclo,gocognit at
// 87/211 — a reading of the pre-carve eng/go/match.go version. Stage 2's
// re-seat 2 (54d8830) extracted the per-candidate scan to
// CollectCandidateScan and the live measure is 68/131, UNDER the repo
// caps (70/200), so the exemption is deleted and the caps now bind here
// for real. Headroom is thin by design (2 gocyclo points): a change that
// would breach a cap extracts the block it grew — textually identically,
// in its own commit, per the §10 triggers in design/FULL-COMPILATION.0.md
// — rather than re-earning an exemption.
func PlanMatch(h CollectHost, win *Tape, reg *Registry, fn *FnDefInfo, w WordInfo, resolved []Value, pointer int, insideForward, checkActive, compiling bool) (*Signature, []int, int) {

	// Unified dispatch (post §1.4 fix): no more stackOnly/forward-prec
	// dichotomy at the word level. Each sig declares its own boundary
	// via BarrierPos — the count of leading args that may be collected
	// from forward tokens. Args at sig[BarrierPos..N-1] always come
	// from the stack, top-down. The /s and /f modifiers override
	// BarrierPos at the call site:
	//   - /s (ForceStack)   → boundary at 0, all stack
	//   - /f (ForceForward) → boundary at N, all forward

	// Forward/stack split ambiguity (check mode only, compile-time advisory).
	// If a more-specific overload is rejected because the stack-top operand
	// is a genuinely MIXED gradual carrier (carrierMixedConform), and the
	// overload finally SELECTED forward-collects instead — leaving that
	// carrier on the stack — the static split diverges from the runtime one
	// (a concrete value would have matched the more-specific overload and
	// been grabbed). noteSplit flags it so the compiler declines; dispatch
	// itself is unchanged. See CheckState.AmbiguousGradualSplit.
	// Only ever read under checkActive (the scan's gradual-Any arm), so the
	// runtime hot path never asks; hoisted here so the per-candidate loop
	// asks once per dispatch rather than once per candidate.
	mixedCarrierRejectIdx := -1
	noteSplit := func(positions []int, fwd int) {
		if !checkActive || mixedCarrierRejectIdx < 0 || fwd == 0 {
			return
		}
		for _, p := range positions {
			if p == mixedCarrierRejectIdx { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
				return // the carrier was consumed after all — not skipped
			}
		}
		reg.noteAmbiguousGradualSplit()
	}

	// When the next forward token is a Word, prefer signatures with
	// /q at position 0 (inspect-style name capture). The user wrote a
	// Word, not a String — the /q sig captures the user's intent that
	// the name is data, not a call site. The non-/q TString sister
	// sig is for callers who pass a string literal. This also covers
	// untype Foo (Foo in r.Types), `m.Color` after import (Color is a
	// key in the imported map), and inspect-style name capture.
	preferWordSig := false
	if pointer+1 < win.Len() {
		next := win.At(pointer + 1)
		if IsWord(next) {
			preferWordSig = true
		}
	}

	// Track the best non-preferred match so that if no preferred sig
	// matches, we can fall back to it without a second pass.
	type matchResult struct {
		sig       *Signature
		positions []int
		specAt    int
	}
	var bestDeferred *matchResult

	// ONE int buffer per matchSignature INVOCATION backs both the
	// per-candidate positions (first maxSigArgs cells, re-sliced and
	// re-zeroed per candidate — the previous per-candidate make was ~17%
	// of all interpreter allocations) and the resolved-index map (tail
	// cells). Per-invocation (NOT engine-level) is load-bearing: a
	// predicate-typed param runs boru during sigTypeMatches (RunPredicate),
	// so matchSignature can nest — each nested call owns its own buffer.
	// A success return hands the positions slice to the caller (ownership
	// transfers; this call never touches the buffer again and the
	// resolvedIdx region is dead after return); bestDeferred keeps its
	// explicit copy since later candidates overwrite the buffer.
	maxSigArgs := 0
	for si := range fn.Signatures {
		if n := fn.Signatures[si].TotalArgs(); n > maxSigArgs {
			maxSigArgs = n
		}
	}
	intBuf := make([]int, maxSigArgs, maxSigArgs+len(resolved))
	posBuf := intBuf[:maxSigArgs]

	// Absolute stack indices of the resolved values — the positions of
	// stack-matched args. Filled into the tail region of intBuf.
	resolvedIdx := resolvedIndicesBeforeInto(win, pointer, intBuf[maxSigArgs:maxSigArgs], len(resolved))

	// ── 0.1: one outer loop over sorted signatures ───────────────
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]

		if sig.Fallback {
			continue
		}
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}

		nArgs := sig.TotalArgs()

		// 0-arg sigs are deferred to the fallback section at the bottom.
		if nArgs == 0 {
			continue
		}

		// Check if this is a preferred (/q at arg[0]) signature.
		isPreferred := preferWordSig && nArgs > 0 &&
			sig.QuoteArgs != nil && sig.QuoteArgs[0]

		// Effective forward limit for this match attempt — the same
		// predicate the phase-1 plan walk uses (see effectiveForwardLimit).
		forwardLimit := effectiveForwardLimit(sig, w)

		// ── Step 1: forward matching ─────────────────────────────

		positions := posBuf[:nArgs]
		for i := range positions {
			positions[i] = 0
		}
		// The per-candidate scan runs on the collection kernel
		// (collect_kernel.go): once per candidate signature, over THIS
		// signature's own forward limit. If forwardLimit is 0 the walk
		// simply does not execute and every arg comes from the stack below.
		// fwd is how many params the forward tokens filled; specAt the
		// first slot a dispatching word filled, -1 for none.
		fwd, specAt := CollectCandidateScan(h, sig, forwardLimit, positions, pointer+1, checkActive, compiling)

		// 1.3: all params matched by forward?
		if fwd == nArgs {
			// Pattern check (post §1.1 fix): scalar literals route
			// through Signature.Patterns instead of value-tagged
			// type paths, so the pattern check has to run for
			// forward-matched positions too. The previous code
			// short-circuited here without consulting Patterns,
			// which made `def fact[0] (1)` fire for any integer.
			if !patternsOk(sig, positions, win, fwd, reg) {
				continue
			}
			if preferWordSig && !isPreferred {
				if bestDeferred == nil {
					bestDeferred = &matchResult{sig, append([]int(nil), positions...), specAt}
				}
				continue
			}
			noteSplit(positions, fwd)
			return sig, positions, specAt
		}

		// Inside a pending forward scope: all args must come from
		// forward. Accept only if forward+stack would satisfy the sig.
		if insideForward && fwd > 0 {
			remaining := nArgs - fwd
			if len(resolved) >= remaining {
				canStack := true
				for j := 0; j < remaining; j++ {
					stackVal := resolved[len(resolved)-1-j]
					if !SigArgMatches(sig, fwd+j, stackVal) {
						canStack = false
						break
					}
				}
				if canStack {
					// Fill remaining positions from stack (nearest first).
					for j := 0; j < remaining; j++ {
						ri := len(resolvedIdx) - 1 - j
						positions[fwd+j] = resolvedIdx[ri]
					}
					// Pattern gate — this selection point must enforce
					// Patterns exactly like the full-forward (above) and
					// normal-stack (below) returns, or a sig whose
					// pattern rejects a filled position is selected
					// anyway (fn's tnot-List triple sig would claim a
					// spec-list call made inside an enclosing pending
					// forward with stack values available).
					if !patternsOk(sig, positions, win, fwd, reg) {
						continue
					}
					if preferWordSig && !isPreferred {
						if bestDeferred == nil {
							bestDeferred = &matchResult{sig, append([]int(nil), positions...), specAt}
						}
						continue
					}
					noteSplit(positions, fwd)
					return sig, positions, specAt
				}
			}
			continue
		}

		// /f means all args must come from forward — if any args
		// remain unmatched after the forward scan, this sig fails.
		if w.ForceForward && fwd < nArgs {
			continue
		}

		// ── Step 2: stack matching ───────────────────────────────

		remaining := nArgs - fwd
		if len(resolved) < remaining {
			continue // not enough stack values
		}

		// 2.1: match remaining sig positions against the stack,
		// top-down. sig[fwd] = top of stack, sig[fwd+1] = next deeper,
		// etc. This is the "stack in reverse order" half of the
		// unified rule — same for stack-only sigs (BarrierPos=0)
		// as for partial-boundary sigs.

		allMatch := true
		// gradualStack notes a stack operand this candidate took on an
		// UNPROVEN match — a carrier whose static type does not conform to
		// the slot, so the runtime value may miss it (NUR228, below).
		gradualStack := false
		for j := 0; j < remaining; j++ {
			ri := len(resolvedIdx) - 1 - j
			stackVal := resolved[ri]
			sigIdx := fwd + j

			// /q is a forward-only rule (see Signature.QuoteArgs doc).
			// stackVal cannot be a Word in normal execution: stepWord
			// has already resolved any Word at the pointer to a function
			// call, defined value, or Atom, and quote produces Atoms.
			// The branch below is defensive only — a stack Atom matches
			// an [Atom/q, ...] sig via the regular sigTypeMatches path
			// just below, no /q involvement required.
			if sig.QuoteArgs != nil && sig.QuoteArgs[sigIdx] && stackVal.Parent.Equal(TWord) {
				if !TAtom.ConformsTo(SigArgType(sig, sigIdx)) {
					allMatch = false
					break
				}
				positions[sigIdx] = resolvedIdx[ri]
				continue
			}
			// A /q position captures a literal word/atom; a NON-CONCRETE carrier
			// (a computed check-mode value) must not fill it via the
			// Any-conforms-to-everything rule — it belongs to a value overload.
			// Mirrors the forward-scan and positionalMatch /q guards. Inert at
			// runtime (operands are concrete there).
			if sig.QuoteArgs != nil && sig.QuoteArgs[sigIdx] && stackVal.Carrier && !IsConcrete(stackVal) && !stackVal.Parent.ConformsTo(TAtom) {
				allMatch = false
				break
			}
			if !SigArgMatches(sig, sigIdx, stackVal) {
				// A more-specific overload rejected because the stack-top
				// operand is a mixed gradual carrier: a concrete value drawn
				// from it might have matched here (and been grabbed). Remember
				// it; if a forward-collecting overload is then selected, the
				// split is ambiguous (noteSplit at the return points).
				if checkActive && j == 0 && reg.analysisMixedConform(stackVal, SigArgType(sig, sigIdx)) {
					mixedCarrierRejectIdx = resolvedIdx[ri]
				}
				allMatch = false
				break
			}
			isTypeArg := sig.TypeArgs != nil && sig.TypeArgs[sigIdx]
			if !isTypeArg && rejectsTypeLiteral(stackVal, SigArgType(sig, sigIdx)) {
				allMatch = false
				break
			}
			if unprovenStackOperand(stackVal, SigArgType(sig, sigIdx)) {
				gradualStack = true
			}
			positions[sigIdx] = resolvedIdx[ri]
		}
		if !allMatch {
			continue
		}

		// Check structural patterns on every matched position. Post
		// §1.1 fix: scalar literals route through Patterns regardless
		// of whether they came from forward or stack matching, so the
		// pattern check no longer skips forward positions.
		if !patternsOk(sig, positions, win, fwd, reg) {
			continue
		}

		// Full match found.
		if preferWordSig && !isPreferred {
			if bestDeferred == nil {
				bestDeferred = &matchResult{sig, append([]int(nil), positions...), specAt}
			}
			continue
		}
		// The window this candidate claims — fwd tokens, the rest from the
		// stack — hangs on a stack operand the runtime value may miss, while
		// a LATER candidate forward-collects past the token this one's scan
		// stopped at: the interpreter, seeing that value, takes the other
		// window (`v send {a: 1} "nobody"` sends to "nobody" when v is None
		// and to v when v is a Pid). No static window is faithful, so the
		// compile declines — the mirror of noteSplit's case, where the
		// static choice forward-collects and the runtime one grabs the
		// carrier (NUR228). An all-stack match (fwd 0) is the forward-drift
		// guard's (DeclineForwardStackDrift and its drift window).
		if compiling && gradualWindowAmbiguous(h, fn, si, w, pointer, fwd, gradualStack, checkActive) {
			reg.noteAmbiguousGradualSplit()
		}
		return sig, positions, specAt
	}

	// Return deferred non-preferred match if one was found.
	if bestDeferred != nil {
		return bestDeferred.sig, bestDeferred.positions, bestDeferred.specAt
	}

	// Try fallback (0-arg or Fallback handler).
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}
		if sig.TotalArgs() == 0 || sig.Fallback {
			return sig, nil, -1
		}
	}

	return nil, nil, -1
}

// laterCandidateCollectsPast reports whether a signature sorted AFTER
// fn.Signatures[si] would forward-collect more than fwd tokens at this word:
// its own scan (the kernel's, over its own forward limit) claims the token the
// selected candidate's scan stopped at. That candidate is the interpreter's
// dispatch whenever the selected one's gradual stack operand misses its slot
// at run time, and it binds a different window (NUR228).
// unprovenStackOperand reports a stack operand matched on no proof — a
// carrier whose static type does not conform to the slot, so the runtime
// value may miss it (NUR228).
func unprovenStackOperand(v Value, slot *Type) bool {
	return !IsConcrete(v) && !v.Parent.ConformsTo(slot)
}

// gradualWindowAmbiguous reports a compile-pass window that took fwd forward
// tokens and hangs on an unproven stack operand while a LATER candidate
// forward-collects past the token this one's scan stopped at (NUR228).
func gradualWindowAmbiguous(h CollectHost, fn *FnDefInfo, si int, w WordInfo, pointer, fwd int, gradualStack, checkActive bool) bool {
	return fwd > 0 && gradualStack && laterCandidateCollectsPast(h, fn, si, w, pointer, fwd, checkActive, true)
}

func laterCandidateCollectsPast(h CollectHost, fn *FnDefInfo, si int, w WordInfo, pointer, fwd int, checkActive, compiling bool) bool {
	for k := si + 1; k < len(fn.Signatures); k++ {
		alt := &fn.Signatures[k]
		if alt.Fallback || (w.ArgCount >= 0 && alt.TotalArgs() != w.ArgCount) {
			continue
		}
		limit := effectiveForwardLimit(alt, w)
		if limit <= fwd {
			continue
		}
		// A claim of the stop token by a DISPATCHING word (specAt == fwd — a
		// function word the plan admits speculatively at an Any slot, its
		// result to complete the slot) is no wider window of values.
		if n, specAt := CollectCandidateScan(h, alt, limit, make([]int, alt.TotalArgs()), pointer+1, checkActive, compiling); n > fwd && specAt != fwd {
			return true
		}
	}
	return false
}
