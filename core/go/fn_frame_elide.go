package core

// Tail-call elimination for direct self-recursion
// (design/legacy/TCO-STAGED.10.ignore Stages 3 and 4a).
//
// When a fn-body dispatch is a direct self-recursive tail call, the
// enclosing frame's cleanup tail — already on the tape, already
// scheduled to run after the callee returns — is executed NOW instead.
// Nothing about the callee's execution changes; the caller's teardown
// just runs before it instead of after, which the gate proves the
// callee cannot observe:
//
//   - the callee's args are already concrete values (auto-eval ran);
//   - captures are construction-time snapshots, reinstalled per call;
//   - the probe proved nothing pending sits below the call inside the
//     frame, so no caller code runs after the callee;
//   - the gate proved arg auto-evaluation touched no bindings, so
//     teardown-now removes exactly what teardown-later would have.
//
// Two tape treatments, chosen by frame-interior cleanliness:
//
// FULL REPLACEMENT (Stage 4a, the clean case — nothing parked below
// the call): after the handler produces the callee's frame tokens,
// the caller's ENTIRE frame region — open paren through close paren,
// ReturnCheck included — is replaced by the callee's frame. Zero
// residue: tape, paren depth, and all three per-call stacks are O(1)
// across any self-recursive tail chain, so tail recursion is a real
// iteration construct and a tail runaway is CPU-bound
// (evaluation_limit), not memory-bound. Dropping the caller's
// ReturnCheck is sound precisely because the callee is the SAME
// compiled overload: its own ReturnCheck arrives with its frame and
// enforces the identical declared returns.
//
// SHELL ELISION (Stage 3, the values-below case): inert values parked
// below the call belong to the caller's result scope, so the frame's
// shell — open paren, ReturnCheck, close paren — stays in place and
// only the marker run is deleted; the callee splices into the shell at
// the call region. Leftover-value and return semantics are
// byte-identical to nesting; the shell still accretes per call, so
// the tape-exhaustion guard bounds a values-below runaway as before
// (such frames accrete real values and are not constant-space-able
// anyway).

// tcoEligible decides whether a detected tail call's enclosing frame
// may be torn down eagerly at all (Stage 4b: ANY fn→fn tail call —
// self or mutual — the teardown's soundness conditions never
// referenced the callee's identity). Which tape treatment applies is
// the caller's next decision: full replacement needs the
// return-conformance check (returnsConform) on top; the shell keeps
// the caller's ReturnCheck so it is sound for any callee. Everything
// here is deny-by-default on top of the probe's own default-deny; a
// declined call simply nests, so correctness never depends on firing.
func (e *Engine) tcoEligible(scan frameTailScan, sig *Signature, defMutsBefore int64) bool {
	if e.Registry.TCO.Disable {
		return false
	}
	// A tail call dispatched while a forward paren group is being
	// evaluated (evalParenGroupAt, e.parenEvalDepth > 0) must nest: that
	// loop finds the group's `)` with a local paren-depth counter, and a
	// frame-region rewrite (full replacement / shell elision) splices the
	// tape underneath it, desyncing the counter so the group never
	// collapses and its bound result vanishes (`def x (f n)` for
	// tail-recursive `f`). Declining nests the call — which the depth
	// counter tracks correctly — at no VALUE cost (TCO is an optimisation;
	// a declined call computes the identical result).
	//
	// The cost is SPACE, and only in the tree-walking interpreter: a deep
	// tail recursion whose result is consumed by a forward paren nests to
	// full depth here instead of running in O(1), so under `--no-compile`
	// it can raise tape_exhausted at ~70-80k frames where the SAME program
	// completes compiled. That is within the interpreter/compiler
	// differential contract (the interpreter may hit a resource ceiling the
	// compiler clears, never the reverse — lang/go/boru.go), the default and
	// `--force-compile` paths lower this recursion to O(1), and it is
	// strictly better than the prior silent undefined_word miscompile. Note
	// the asymmetry: a WORD-CONTEXT paren `(f n)` keeps TCO, because
	// stepCloseParen re-derives the close index after each dispatch
	// (findCloseParenAfter) instead of tracking it across the splice; only
	// this forward-collection eager-eval path declines.
	if e.parenEvalDepth > 0 {
		return false
	}
	if scan.Meta == nil {
		return false
	}
	// Generic fns install per-call type-parameter bindings; their
	// teardown/Retire interaction is not yet proven under elision —
	// decline when either the enclosing frame or the callee is
	// generic.
	if scan.Meta.HasGen || sig.FnFrame().HasGen {
		return false
	}
	// Arg auto-evaluation ran code (evaluated list/map args) between
	// collection and this dispatch. If it touched any binding, the
	// parked teardown would have sequenced around that change
	// differently than an eager one — decline.
	if e.Registry.Defs.Mutations() != defMutsBefore {
		return false
	}
	// The synthesized undef tail must be replicable by UninstallDef
	// alone: a capitalised name takes undef's type-retire path, and a
	// builtin-shadowing name would ERROR at the parked token — both
	// decline so eager teardown matches the token-by-token behaviour
	// exactly. Each name must ALSO be one the callee immediately
	// reinstalls (see coverage below).
	covered := func(name string) bool {
		for _, n := range sig.FnFrame().InstallNames {
			if n == name {
				return true
			}
		}
		return false
	}
	for _, name := range scan.UndefNames {
		if IsCapitalisedName(name) || e.Registry.IsBuiltinWord(name) || !covered(name) {
			return false
		}
	}
	// Teardown-coverage for the DefCleanup truncation: body-local
	// bindings the caller's frame created are visible to the callee
	// chain via dynamic resolution under nesting (innermost binding
	// wins, outer frames' bindings stay live until unwind) — the
	// recursive-local-fn idiom (`def go fn […] go 3`) and loop-carried
	// base-branch reads depend on it. Eager teardown may therefore
	// remove ONLY names the callee rebinds before its body runs;
	// anything else declines and nests.
	dcInfo, err := AsDefCleanup(e.Tape.At(scan.TailStart))
	if err != nil {
		return false
	}
	// The frame's per-call state must live on THIS engine's registry:
	// teardown pops e.registry's Args/baseline and undefs against its
	// def table. A frame spliced by a handler closed over a FOREIGN
	// registry (a compiled fn value carrying a module sub-registry —
	// see compileFnDef's foreign-value path) keeps its state there;
	// decline rather than pop the wrong stacks. The DefCleanup marker
	// carries the frame's registry, so the check is one comparison.
	if dcInfo.Registry != e.Registry {
		return false
	}
	// An EvalResidual frame with a pending Eval container parked below
	// the call would have that residual EVALUATED by the eager teardown
	// — before the callee runs — where the parked marker evaluates it
	// after the callee returns. A side-effectful residual would observe
	// the reorder, so the frame nests: the parked __DC then sequences
	// the residual exactly as under nesting.
	if dcInfo.EvalResidual && scan.PendingEvalBelow {
		return false
	}
	// A SkipCleanup frame installs no body-local defs, so its (nil) Snapshot
	// truncation removes nothing — trivially covered, no need to walk the
	// def table. Without this shortcut the nil Snapshot would read every
	// depth as 0 and spuriously decline eager teardown, regressing tail
	// recursion (design/legacy/INTERPRETER-SPEED-PLAN.10.ignore #5).
	if !dcInfo.SkipCleanup && !e.Registry.Defs.TruncationCoveredBy(dcInfo.Snapshot, covered) {
		return false
	}
	return true
}

// returnsConform reports whether dropping the enclosing frame's
// ReturnCheck in favour of the callee's is sound: every value the
// callee's check admits must also satisfy the caller's. Holds when the
// caller declares no returns at all, or when the callee declares the
// same NUMBER of returns, each conforming (callee[k] ⊑ caller[k] on
// the type lattice). Identical overloads (self-recursion) hold
// trivially. A placeholder-Any callee return against a stricter caller
// declines here (Any ⊄ Integer), as does an unchecked callee against a
// checked caller — those take the shell path, where the caller's
// ReturnCheck stays in place and enforces its contract exactly as
// under nesting.
func (e *Engine) returnsConform(scan frameTailScan, sig *Signature) bool {
	if scan.RCIdx < 0 {
		return true
	}
	rc, err := AsReturnCheck(e.Tape.At(scan.RCIdx))
	if err != nil {
		return false
	}
	if len(sig.Returns) == 0 || len(sig.Returns) != len(rc.Returns) {
		return false
	}
	for k := range sig.Returns {
		if sig.Returns[k] == nil || rc.Returns[k] == nil ||
			!sig.Returns[k].ConformsTo(rc.Returns[k]) {
			return false
		}
	}
	return true
}

// teardownFrameState executes the scanned frame tail's REGISTRY
// effects eagerly — the same operations the parked markers would have
// performed, via the same helpers. Tape edits are the caller's
// business: the shell variant deletes just the marker run, the full
// replacement deletes the whole frame after the callee's tokens are
// in hand.
func (e *Engine) teardownFrameState(scan frameTailScan) error {
	// __DC: truncate body-local defs to the frame's entry snapshot
	// (running the multi-token in-frame residual eval first, exactly as
	// the parked marker would — the eager teardown must not change WHERE
	// a residual container's names resolve).
	if err := e.stepDefCleanup(e.Tape.At(scan.TailStart), scan.TailStart); err != nil {
		return err
	}
	// __pa: pop the per-call Args list and FnBaseline, paired.
	if err := PopFrameArgs(e.Registry); err != nil {
		return err
	}
	// The undef tail: captures+params, already in reverse install
	// order in the scan. The gate proved every name takes undef's
	// plain UninstallDef path.
	for _, name := range scan.UndefNames {
		UninstallDef(e.Registry, name)
	}
	return nil
}

// elideTailFrame is the SHELL variant: run the frame tail's registry
// effects eagerly, then delete the marker run from the tape. The
// frame's ReturnCheck (when present) and close paren are deliberately
// kept; the callee splices into the shell at the call region as usual.
// Used for frames with inert values parked below the call — the shell
// keeps them, so leftover-value semantics are identical to nesting.
//
// All tape edits here sit strictly AHEAD of the pointer (the marker
// run lies beyond the call region), so the pointer, the matched arg
// positions, and every index below them are untouched.
func (e *Engine) elideTailFrame(scan frameTailScan) error {
	if err := e.teardownFrameState(scan); err != nil {
		return err
	}
	// Delete the executed markers; keep the ReturnCheck (when
	// declared) and the frame's close paren — the shell.
	end := scan.RCIdx
	if end < 0 {
		end = scan.CloseIdx
	}
	e.Tape.Splice(scan.TailStart, end-scan.TailStart)
	return nil
}
