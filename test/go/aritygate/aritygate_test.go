// aritygate_test.go gates the tree against arity-keyed exceptions.
//
// ADR-016: "Every function behaves the same way whatever its arity and
// wherever it came from … this record forbids exceptions keyed on arity or
// origin." Accepted 2026-08-15, and ruled ABSOLUTE by the maintainer on
// 2026-08-25 — "everything everywhere every time and always".
//
// An absolute rule with no gate is a rule nobody can enforce. NUR100 recorded
// two live sites that contradicted the ADR (both closed 2026-09-26: the
// predicate role keys on a one-value APPLICATION through the matcher, the
// poly decline on an overload's declared re-step); a THIRD was found on
// 2026-08-28 —
// the compiler's ARITY-1 BOUNDARY, which decided that a one-input callback
// could compile and a two-input one could not — and it was found while fixing
// something else, not by looking. It had survived unrecorded because it read
// as a compile-coverage limit rather than a semantic exception. A site nobody
// is counting is a site nobody finds.
//
// WHAT THIS GATE COUNTS, and what it deliberately does not. It flags a
// comparison one of whose sides is the ARITY OF A FUNCTION BEING INSPECTED —
// len(sig.Params), len(fd.Signatures), x.Arity, sig.TotalArgs(). It does NOT
// flag a handler bounds-checking the args IT received (`len(args) < 2`): that
// is a function reading its own declared positions, which every handler does
// and which is not an exception to anything. The distinction is the whole
// design of the predicate — an earlier draft that counted bare `len(args)`
// found 298 sites across 100 files and would have been noise.
//
// A PIN IS NOT AN ACCUSATION. Most entries below are the matcher and its
// machinery — engine.go, signature.go, match.go, carrier.go — reading arities
// in order to MATCH a signature. That IS the one argument rule; implementing
// it is not an exception to it. The pins exist so that a CHANGE in the count
// forces someone to look and say which kind it is. The two entries that
// were known divergences (NUR100) are closed; their files' remaining pins are
// the argument rule's own reads.
//
// If this test failed because a count ROSE: say which kind the new site is. If
// it implements the argument rule (matching, dispatch, canon), raise the pin
// and note why. If it decides behaviour BY arity — whether a function may act
// as a predicate, whether a body may compile, which overload is admitted —
// that is what ADR-016 forbids, and the fix is to key on the thing you
// actually mean (a role, a declaration, a container) rather than on a count.
//
// If it failed because a count FELL: that is the ratchet tightening. Lower it.
package aritygate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// countSel names the SELECTORS whose length is a function's arity. A selector
// is required — a bare `len(args)` is a handler reading its own positions, not
// an inspection of some other function's shape.
var countSel = map[string]bool{
	"Params": true, "Signatures": true, "OwnSigs": true, "ArgTypes": true, "Args": true,
}

// arityNames are the direct arity accessors — no len() involved.
var arityNames = map[string]bool{
	"Arity": true, "NParams": true, "NArgs": true, "TotalArgs": true, "ArgCount": true,
}

// pinnedAritySites is the exhaustive census, repo-relative file → number of
// arity comparisons. See the header for what a pin means.
var pinnedAritySites = map[string]int{
	// ── The matcher and its machinery: reading arities to MATCH a signature
	//    is the one argument rule (eng/go/CLAUDE.md, "Signature Ordering"),
	//    not an exception to it.
	// 33 -> 28: the plan-level matcher moved to core/go/collect_plan.go
	// (PlanMatch, the sixty-fourth increment) with its five comparisons —
	// the per-candidate arity reads that fill positions from the forward
	// scan and the stack, the argument rule itself. The sites did not
	// change; the file did.
	// 28 -> 27 (the sixty-sixth increment): barrierReceiverWord's one
	// comparison (`s.BarrierPos < s.TotalArgs()` — does any overload read a
	// slot from the enclosing stack, for the strict-barrier diagnostic's
	// sequential-spelling note) moved to core/go/region_diag.go as
	// BarrierReceiverWord, where the routed dispatch raises the same
	// diagnostic from its window. The site did not change; the file did.
	// 27 -> 29 (2026-09-23, the trailing value's re-step, NUR184): two
	// reads of the argument rule at the paren collapse —
	// trailingFnCollectsPastClose skips a signature with no position to
	// collect into (`sig.TotalArgs() == 0`) before asking whether the
	// token after the close matches its first forward position, and
	// parenFeedsPendingForward asks whether a parked Forward is still
	// collecting (`fwd.CollectedArgs < fwd.Sig.TotalArgs()`, the same test
	// hasPendingForwardCollecting makes). Both decide where a value's
	// arguments COME FROM, never what a fn may do by its count.
	// 29 -> 27 (2026-09-26, NUR078 closed): the retired clause-2 sites took
	// their two reads with them — sigWantsFunctionAt's bounds check on the
	// slot it asked about (`pos >= sig.TotalArgs()`) and
	// hasPendingForwardExpectingFunction's still-collecting test
	// (`nextIdx < fwd.Sig.TotalArgs()`). No site was added: a bare fn name
	// now calls at every slot, so nothing asks which slot is open.
	// 27 -> 28 (2026-09-26, the merge of main's #509): TryRecordRecoveredUserFn
	// refuses a recovered window shorter than the sole sig (`len(window) <
	// sig.TotalArgs()`) — the matcher's own arity rule, mirrored so the
	// guarded CALL_USER never binds a partial window the interpreter's
	// signature_error refuses; it decides whether the arguments are THERE,
	// not what the fn does by their count (main's 29 -> 30).
	// 28 -> 30 (2026-09-26, the merge of main's #510): main's two sites
	// (its 30 -> 32, the last real programs, the fn-value recovery) —
	// narrowOverloadShadowed's window-arity candidate filter
	// (`s.TotalArgs() != len(window)`), which proves which overload the
	// interpreter's first match takes over a fixed operand window — the
	// matcher's own rule — and fnValueNoMatchRecovers' empty-table guard
	// (`len(fn.Signatures) == 0`, no overload to recover into). No function
	// behaves differently by its count.
	// 30 -> 32 (2026-09-26, NUR241 closed as a sound decline):
	// noteWordLedArrival skips a slot past the deferred signature's own
	// arity (`slot >= fwd.Sig.TotalArgs()`) — whether the window HAS that
	// slot — and narrowerWindowFits skips a 0-arg signature
	// (`TotalArgs() == 0`), which takes no window and so cannot be the
	// narrower one that fits the stack beneath the word. Both ask which
	// window the matcher's own rule collects, never what a fn does by its
	// count.
	"core/go/engine.go":      32,
	"core/go/region_diag.go": 1,
	// NUR242 (2026-09-26): the exact layout is published only for a PLAIN
	// dispatching word (`w.ArgCount != -1` rules out an `/N` modifier), since
	// a modifier overrides the forward limit the plan reads — the token's
	// syntax, not a function's parameter count.
	"core/go/dispatch_layout.go": 1,
	// NUR264/NUR263 (2026-09-27): an optimistic dispatch's window and layout
	// are published only when the match's positions cover its operands
	// (`len(indices) != len(match.Args)`, an empty match) and, for the
	// layout, for a PLAIN dispatching word (`w.ArgCount != -1`, the `/N`
	// modifier's syntax, as NUR242's layout) — reading the window the
	// matcher's own rule collected, never deciding what a fn does by its
	// count.
	"core/go/optimistic_match.go": 3,
	"core/go/collect_plan.go":     8, // 5 -> 8 (NUR305): laterCandidateCollectsPast compares FORWARD-WINDOW counts (a later candidate's limit and scan against the selected fill) — the argument rule over two candidates, not behaviour by arity
	// 12 -> 13 on 2026-09-27 (the poly inline cache): TagDeterminedSigs
	// considers only the n-argument overloads (`s.TotalArgs() != n`) — the
	// ones MatchSignature's own ArgCount filter considers for an n-operand
	// window — to prove the first match is decided by the operands' tags.
	// The argument rule, not behaviour by arity: a cache miss takes the
	// same full match every arity takes.
	"core/go/signature.go":   13,
	"core/go/match.go":       1,
	"core/go/fnsig.go":       3,
	"core/go/word_extend.go": 6,
	// 4 -> 2 (2026-09-25, NUR099 closed): PredicateInputType lost the
	// parameter-COUNT route (`!info.Predicate && len(sig.Params) != 1`) —
	// ADR-016's arity-keyed exception, a fn body read as a membership test
	// because it took one parameter — and its first-signature guard now
	// reads the declared predicate's params once. Only `fnpred` declares a
	// predicate; what remains guards the declared signature's first slot.
	"core/go/core_helpers.go": 2,
	"core/go/core_ref.go":     3,
	// 3 -> 2 (2026-09-25, NUR099 closed): isPredicateFnValue — "looks like
	// a predicate because it takes one parameter", the deprecated route —
	// is deleted; IsDeclaredPredicateFn reads the `fnpred` mark instead.
	"core/go/unify.go":   2,
	"core/go/deadsig.go": 1,
	"core/go/canon.go":   1,
	"core/go/value.go":   1,
	// NoteFnShape rejects a NEGATIVE claim (`FnShape.Arity < 0`) — a shape a
	// producing word could not build (`partial` over a 0-param fn raises) —
	// a validity guard on the claim itself, not a decision keyed on a
	// function's parameter count (the thirty-fifth increment).
	"core/go/check_state.go":  1,
	"core/go/boru_error.go":   2,
	"core/go/macro_expand.go": 1,
	// sameSigShape compares two signatures' arity (`a.TotalArgs() ==
	// b.TotalArgs()`) and walks their positions (`i < a.TotalArgs()`):
	// whether one branch arm's fn can stand for the other's at a call's
	// record (NUR245) — the shape the call's claim fixes, never behaviour
	// keyed on a function's parameter count.
	// 2 -> 5 (2026-09-26, NUR245's differing shapes): claimCompatibleSigs
	// walks the same positions (`i < a.TotalArgs()`) to widen each one's
	// type, and widenedSig copies the declared params only when there are
	// some (`len(a.Params) > 0`) and re-aligns the legacy Args only when
	// they are the joined window's own (`len(a.Args) == len(joined)`) —
	// the widened model's positions, one per slot of the call's window.
	"core/go/spec_fn.go": 5,

	// ── The checker's and VM's mirrors of that same matching.
	// 13 -> 15 on 2026-09-26 (the last real programs):
	// reachableUnknownReturnSibling's same-arity candidate filter
	// (`s.TotalArgs() != len(args)`) and its single-overload early out
	// (`len(fn.Signatures) < 2`) — dynamicReachableReturns' own
	// reachability rule, read to know whether a dynamic operand reaches an
	// overload whose return the union cannot name. Matching, not
	// arity-keyed behaviour.
	// 15 -> 16 (2026-09-28, the stranded `if` — formerly NUR332):
	// declaredReturnCarriers never calls a ReturnsFn with fewer operands
	// than its signature declares (`len(args) < sig.TotalArgs()`) — the
	// recovery's best-fit overload over a SHORT window answers its declared
	// Returns instead. A bounds check against the signature the argument
	// rule matched, never behaviour keyed on a function's count.
	"check/go/carrier.go": 16,
	// 1 -> 2 (2026-09-24, the written argument's fit, NUR194):
	// shapedFnReadWindow guards `i-1 < len(shape.Params)` — a BOUNDS check
	// on the claim's parameter-type slice, which may be shorter than the
	// arity (nil where only the arity is known) — before asking whether a
	// written token fits that parameter. It reads where the types END,
	// never what a function of a given arity may do: every arity takes the
	// same path, and the matching itself is SigTypeMatches, the argument
	// rule's own arm.
	// 2 -> 3 (2026-09-25, NUR096 closed): tryFnShapeTypedWindow stands
	// aside for an ARITY-0 shape (`shape.Arity == 0`). The model it guards
	// consumes a shape's window by the argument rule; a 0-parameter shape
	// has no window, and whether a stored 0-arg fn FIRES is the runtime's
	// anonymous-0-arg park (ADR-016's one kept gate, decided by the stored
	// fn, not the shape) — so the pass leaves the carrier as it was rather
	// than guess. It decides nothing by count: it declines to model.
	"check/go/method_shape.go": 3,
	// 1 -> 2 (2026-09-25, the strict-Any dyn-body recovery):
	// widestSatisfiableOverload compares `n > best.TotalArgs()` to pick,
	// among a word's overloads whose FULL operand window exists at the
	// call site, the one of greatest arity — the matching rule's own
	// choice (the interpreter's first match consumes the widest window it
	// can: fold's seeded form over its seedless one), never a decision
	// about what a function of a given arity may do; the run-time poly
	// re-match over that window then matches by type exactly as
	// CALL_NATIVE_POLY does.
	"check/go/check_recovery.go": 2,
	// A bounds check on a signature INDEX, not a decision about a function's
	// shape (CompileFnSigUnit guarding fnDef.Signatures[sigIdx] before it
	// compiles that one signature's body as a dispatch of it would — the
	// seventieth increment's replaced outer).
	"check/go/spec_fn_unit.go": 1,
	// Call-site specialisation (2026-09-27) is attempted only for a fn with
	// exactly one signature (`len(fnDef.OwnSigs()) != 1`): an OVERLOAD-LIST
	// presence test choosing the specialised unit's fallback — the one
	// signature a failed guard applies. It never decides whether a body
	// compiles or how a call behaves: every fn, one signature or many,
	// compiles its generic unit exactly as before, and a specialised call
	// answers what the generic call answers. Not behaviour by arity.
	"check/go/call_site_spec.go": 1,
	// The seeded poly pick (2026-09-27) proves the checker's pick for an
	// n-operand window: the pick must take n args (`sig.TotalArgs() != n`)
	// and, for the tag-free guard, be the word's ONLY overload MatchSignature
	// would consider for n operands (`sigs[i].TotalArgs() == n`) — the
	// matcher's own ArgCount filter. A failed guard re-matches exactly as
	// before; no function behaves differently by its count.
	"compiler/go/poly_seed.go": 2,
	// The seed's revalidation against the LIVE aggregate (2026-09-27, Codex
	// review of #517) finds the live overload standing for the seed among
	// the window's-arity overloads (`sigs[i].TotalArgs() != len(window)`) —
	// MatchSignature's own ArgCount filter, the argument rule. A seed that
	// does not validate takes the same full match every arity takes.
	"eng/go/vm_poly_seed.go": 1,
	// 10 -> 11: nameFrameFns bounds its loop by `i < fn.NParams` to visit the
	// NAMED PARAM slots of a frame — which slots are params, so a fn value
	// bound for one takes the binding's name as the interpreter's frame
	// binding gives it (installDef). It reads WHERE the params sit, never
	// what a function of a given arity may do; every arity takes the path.
	// 11 -> 12: callDynApply's `fn.NParams == n` picks the VM-native fast
	// path for a compiled closure whose unit takes exactly the window's
	// values; every other arity takes the interpreter's own apply re-step,
	// which applies the closure by its signature like any fn value. A path
	// choice between two implementations of ONE dispatch rule, never a
	// behaviour a function of a given arity gets — the twenty-seventh
	// increment (the same shape dynApplyEnter's drift check pins above).
	// 12 -> 11: that same test now reads `fn.NParams-fn.NCaptures == n` —
	// the unit's PARAM slots alone, since NParams counts the trailing
	// capture slots too and reading it whole sent every capturing closure
	// to the island (the twenty-eighth increment). The census's pattern no
	// longer sees the comparison; the site and its meaning are unchanged.
	// 11 -> 12: the OpDispatchGeneric arm enters the committed unit exactly
	// as the OpCallUserPoly arm above it does — the same `i < fn.NParams`
	// loop over the frame's PARAM slots (the sixty-fourth increment).
	// 12 -> 13: reStepLanding's `len(fnDef.OwnSigs()) > 0` (NUR173) is a
	// PRESENCE test, not an arity one — the same three-ways-to-have-no-own-
	// signatures question noMatchIfSigged asks four lines below, and it
	// gates the same next step: whether MatchFnSig has anything to match
	// against. The decision that follows is MatchFnSig's, the argument rule
	// itself, asked with an empty window because the landing is where the
	// interpreter re-steps a value with nothing written after it. A fn of
	// any arity reaches it and is answered by its signatures, never by a
	// count: one that matches at zero runs, one that does not stays data,
	// which is what the interpreter's own re-step does there.
	// 13 -> 15 (2026-09-23, the typed callback's contract, NUR155):
	// unmatchedLambdaBody asks whether a callback BODY unit recorded a
	// contract of its own — one Params entry per real arg (`len(fn.Params)
	// == 0`, `len(fn.Params) != fn.NArgs`: closureSigParams's own
	// precondition) — before matching that contract over the element with
	// MatchFnSig's rule. A unit with no contract (a quotation body) stands
	// aside; the decision that follows is the signature's, never the count's.
	// 15 -> 17 on 2026-09-23 (the landing's overload walk, NUR190): both
	// MATCH the argument rule — `sig.TotalArgs() == 0` reads the plan the
	// interpreter's own matcher returned (its zero-argument fallback, the
	// signature no forward token or stack value fills) to fire that
	// overload, and `ArgCount: -1` is the unmodified call the landed value
	// plans as (no `/N` at a value). Neither decides behaviour by arity.
	// 17 -> 18 (2026-09-24, the fn-util wrapper at the token seam):
	// applyNativeFnValueTopDown asks each own signature's TotalArgs for how
	// many inputs the value takes from the seam's stack window — the
	// argument rule's own stack fill (positions filled from the top down),
	// mirrored for a Go-implemented fn value handed to a native's body
	// seam; every arity takes the same path and the match is
	// tryNativeFnApply's.
	// 18 -> 22 (2026-09-25, the no-match window and the fn-value body's
	// own args): the closure-invocation seam brackets a fn value's body
	// with the value's OWN call args, slicing the inputs the unit declared
	// out of the seam's window (`fn.NArgs <= len(args)`: the argument
	// rule's own count, NUR166), and polyHasArity asks whether some
	// non-fallback overload declares exactly the k inputs the poly seat is
	// retrying at — overload selection by declared signature (NUR147).
	// Neither decides behaviour by arity.
	// 22 -> 23 (NUR238): valueTrailNoMatch asks whether a trailing apply's
	// fn value carries ANY own signature (`len(fd.OwnSigs()) == 0`) before
	// asking whether the window fits one — a value with no contract to
	// consult is not the no-match rule's. The count of params never enters;
	// every arity takes the same path.
	// 23 -> 24 (NUR282's `j j`): parksResult asks whether a fn value
	// carries ANY signature (`len(fd.Signatures) == 0`) before asking
	// whether every one runs a boru body, whose result the interpreter
	// parks — a value with no signature has no body to park. The same
	// presence test as NUR238's; the count of params never enters.
	// 24 -> 26 (NUR336's remainders, 2026-09-29): parenMissesWindow and
	// namedMissRaise ask whether a signature takes as many values as the
	// paren's window holds (a closure unit: its params less its captures)
	// before MatchFnSig admits them — the argument rule's own first test —
	// and placesAlone reads the forward split the rule makes (a signature of
	// k values, all forward-eligible, takes the window's first k). Each
	// MATCHES a signature against the window; none decides what a function
	// of a given arity may do.
	"eng/go/vm.go": 26,
	// The Apply kernel's runtime entry: `fn.NParams != len(args)` checks that
	// the compiled unit AGREES with the signature MatchFnSig already selected
	// (compile/run drift detection — entering on a mismatch would bind the
	// wrong locals silently), and the delivery loop bounds itself on the unit's
	// own param count, verbatim from OpCallUserPoly. A function of any arity
	// takes the same path. NOTE: this entry was added because the gate caught
	// it — on the first change after the gate landed, which is the whole point.
	// 2 -> 3: allForwardSig compares a matched signature's BARRIER against its
	// param count to answer where the call's arguments come from — all forward,
	// or forward up to the barrier and the rest from the stack. That IS the
	// argument rule, not a decision about what a function of a given arity may
	// do: the replay window reads it to know whether the callee can reach the
	// resolved prefix below the tokens, and a callee of any arity that cannot
	// takes the same path.
	"eng/go/vm_dyn_apply.go": 4, // 3 -> 4 (2026-09-24, the foreign-home fn value at the apply seam): dynApplyForeign requires the matched overload's parameter count to equal the window — the argument rule (a unit binds exactly its params; dynApplyEnter's NParams == len(args) shape rule for a detached unit), never behaviour by arity
	"eng/go/vm_rematch.go":   2,
	// 2026-09-27, the fn-value no-match park: fnValueNoMatchVerdict declines
	// a value with a real 0-parameter signature because the plan's fallback
	// section PICKS one (core.PlanMatch) and the step applies it — the
	// argument rule's own selection, mirrored so the park is never claimed
	// where the interpreter dispatches; the second site counts the window's
	// CANDIDATE operands (the interpreter's uncalled_function guard), not a
	// parameter count.
	"eng/go/vm_fnvalue_park.go": 2,
	"eng/go/vm_poly_nomatch.go": 3,
	// closureAsWord bridges a compiled closure to a handler-bearing FnDefInfo
	// so the interpreter's WORD dispatch can match it (NUR123): the bridge
	// DECLARES the callee's signature from the unit's own param count and
	// types — the argument rule's input, not a decision about what a closure
	// of a given arity may do; a closure of any arity takes the same path.
	// 1 -> 2: landedFnTakesArgs (NUR286) asks the same of a FnDefInfo — does
	// any real signature take an argument — so the landing's guard knows
	// whether the interpreter's re-step could match the fn over the values
	// beneath it: the argument rule over that supply, never a decision about
	// what a fn of a given arity may do.
	"eng/go/vm_dyn_words.go": 2,
	// The COLLECT oracle's candidate table (oracleCandidates, the
	// sixty-second increment): a FN-LOCAL fn is a frame binding the run-time
	// registry never holds, so the oracle DECLARES the callee's signature
	// from the unit's own param count — `fn.Params[:fn.NArgs]`, all of them
	// forward — exactly as closureAsWord does above, and the two comparisons
	// are the bounds guards on that slice (`NArgs > 0`, `len(Params) >=
	// NArgs`). The argument rule's input, not a decision about what a fn of
	// a given arity may do; a fn-local fn of any arity is scanned the same
	// way against the synthetic signature.
	"eng/go/region_oracle.go": 2,
	// The routed dispatch (vm_generic.go, the sixty-fourth increment):
	// unitMatchesSig asks whether the LIVE matched signature is of the
	// committed unit's shape — the same arity (`sig.TotalArgs() !=
	// fn.NArgs`), the guard on the slice it then walks (`len(fn.Params) <
	// fn.NArgs`) and the walk's own bound (`i < fn.NArgs`), comparing the
	// declared types position by position. Reading a matched signature's
	// shape is the argument rule's own output (the same reading
	// callDynApply and dynApplyEnter make above); a fn of any arity takes
	// the same path. 3 -> 4 (review of #460): the claim guard compares the
	// live plan's arity with the record's (`len(positions) != gs.NArgs`) —
	// the argument rule's output on both sides, and a plan of another
	// extent defers whatever the arity.
	"eng/go/vm_generic.go": 4,
	// The region capture (review of #460): plainWord reads the lead's `/N`
	// modifier (`w.ArgCount == -1`) to know whether the tape wrote ANY
	// modifier, so the WordInfo rides the descriptor and the live walk
	// honours it exactly as the record did. Syntax carried, not a decision
	// by arity: a modified lead of any arity routes the same way.
	"compiler/go/region_record.go": 1,

	// ── NUR100 §1, CLOSED 2026-09-26. 3 -> 2: RunPredicate no longer decides
	//    whether a function may act as a predicate by counting its
	//    parameters ("predicate must take exactly one argument"). Membership
	//    is a one-value APPLICATION — the candidate is matched against the
	//    predicate's signatures by MatchFnSig, the one matcher every call
	//    takes, and a candidate no signature takes is not a member. What
	//    remains is the argument rule's own: Register's MaxArgs bound and the
	//    0-arg courtesy dispatch of a call that collected nothing.
	"core/go/registry.go": 2,

	// ── NUR100 §2, CLOSED 2026-09-26. 2 -> 1: tryRecordPoly's decline no
	//    longer counts a SMALLER-arity overload (smallerArityOverload, gone);
	//    it keys on what the count stood in for — a reachable overload that
	//    DECLARES CompileResteps, a dispatch whose result re-steps on the
	//    tape, which a poly re-match cannot reproduce at any arity
	//    (restepOverloadReachable). What remains is the barrier clamp reading
	//    a signature's forward positions, the argument rule itself.
	// 1 -> 2 (NUR265, 2026-09-26): concreteHandlerEval runs a handler only
	//    over its signature's full arity (`len(args) != sig.TotalArgs()`) —
	//    the argument rule, which every dispatch honours when it hands a
	//    handler its window; a recovery's short window reached `gt`'s
	//    handler and the check pass panicked.
	// 2 -> 3 (NUR225, 2026-09-27, from main): tryFoldScalarConst's
	//    `len(args) != sig.TotalArgs()` refuses to CALL a handler over a
	//    window that is not its declared positions (the no-match recovery
	//    handed it gt's two-slot constructor over one value, and the
	//    handler's unguarded read panicked out of RunCompiled). The same
	//    contract as NUR265's — a handler reads the params it declares — at
	//    the fold's call; a sig of any arity takes the same path, and only
	//    the fold is skipped, the dispatch records as it would.
	"compiler/go/compiler_dispatch_record.go": 3,

	// ── Compiler: recording and lowering against declared signatures.
	// 3 -> 4: the `apply` word's two overloads differ in arity — [Function]
	// takes the lead alone, [Reach Any] the lead and a receiver — and the
	// recorder reads WHICH the check matched (sig.TotalArgs of 2) to record
	// a gradual-lead apply as the one-arg event (recordGradualApplyEvent).
	// That reads the matched signature's shape, the argument rule's own
	// output; a fn of any arity on top at run time takes the same op — the
	// twenty-seventh increment.
	// 4 -> 6: slotDeclaresFunction reads whether a signature's slot i is
	// DECLARED `Function` — two bounds checks on a signature INDEX
	// (`i < len(sig.Params)`, `i < len(sig.Args)`) choosing which side of
	// the signature holds the slot, not a decision about a function's
	// shape; the declared slot type is the argument rule's own input —
	// the thirtieth increment.
	// 6 -> 7: fnSigsDeclared reads whether a fn value carries ANY signature
	// (`len(fd.Signatures) == 0`) before asking each for a boru body with a
	// declaration site — the identity a speculative family's routed op
	// locates its unit by. A lambda or a Go alias declares none, so the
	// placement declines it; the count of PARAMS never enters — the
	// seventieth increment.
	// branchArmMayTakeArgs (7 -> 8, the named fn value's candidates,
	// 2026-09-23) reads a factory closure's RECOVERED arity for a branch
	// arm's value: zero is the anonymous park the landing itself applies
	// (a 0-arg lambda is data wherever it is held, ADR-016's parking rule),
	// positive means the value takes the values beneath it. It classifies
	// the arm as settled or unsettled for the residual arms — the same
	// rule the interpreter's execFnDefLiteral applies to the value at run
	// time — never what a fn of a given arity may do.
	// 8 -> 10 (2026-09-26, NUR246): applyWindowFits compares a produced
	// closure's claimed param count with the apply's window (`len(s.Params)
	// != len(sigArgs)`) before matching each position — whether the window
	// the op binds provably FITS, the argument rule; raisesTrailNoMatch asks
	// whether a named value carries any own signature (`len(fd.OwnSigs()) >
	// 0`), valueTrailNoMatch's own guard mirrored so the recorder knows
	// which values raise their no-match. Neither decides behaviour by a
	// function's arity.
	"compiler/go/emit.go": 10,
	// sameFnDecls compares two fn VALUES for declaration identity — the
	// same signature list: the same count, then each position's declaration
	// site (Signature.Decl). It decides whether a unit's recorded def event
	// IS the current binding (a closed body's redefinition is not), never
	// what a function of a given arity may do; every arity takes the path —
	// the seventy-second increment (review of #468).
	"compiler/go/fn_local.go":  1,
	"compiler/go/user_poly.go": 1,
	// The second is a bounds check: a closure's argument window shorter
	// than the signature it was matched under is no call at all (a failed
	// dispatch's recovery assumed it, and spec.Inputs reads the operands
	// positionally) — the argument rule's own read, never a decision BY
	// arity; every arity takes the path (NUR340).
	"compiler/go/callable_words.go": 2,
	// A bounds check on a signature INDEX, not a decision about a function's
	// shape (StampDetachedSig guarding fd.Signatures[sigIdx]).
	"compiler/go/stamp_runtime.go": 1,
	// UnitIsFnValue asks whether a closure unit RECORDS ITS OWN SIGNATURE —
	// one declared Params entry per real arg — which is the precondition for
	// matching it at all, and the same test closureSigParams makes before it
	// builds the contract (eng/go/vm_dyn_words.go). A unit that records no
	// contract is a callback BODY compiled at a call site, which has no
	// signature to match; one that does is a fn VALUE, and every arity of
	// one takes the same path. Matching machinery, not a decision by arity —
	// S1b-2 of design/FULL-COMPILATION-REPLAN.0.md.
	// 1 -> 2: ClosureCallsAtLanding (NUR321) asks whether a NAMED fn value
	// landing where nothing supplies an argument matches at all — the
	// argument rule over an EMPTY supply. A signature that needs no operand
	// matches there, so a nullary named value calls, exactly as
	// MatchSignature admits it over no values; one that needs any fails the
	// match and stays data, as the interpreter leaves it. The count decides
	// only whether the match can succeed over nothing, never what a fn of a
	// given arity may do.
	// 2 -> 3: ClosureTakesArgs (NUR286) asks whether a fn-value closure's
	// unit takes an argument at all — the argument rule over the values
	// beneath a landing: a unit that takes none cannot match any of them,
	// exactly as MatchSignature over a supply admits only a signature that
	// consumes from it. The count decides only whether a match over those
	// values is possible, never what a fn of a given arity may do.
	"compiler/go/bytecode.go": 3,

	// ── Generics: instantiation matches a declaration's shape.
	"core/go/generics_unify.go":       1,
	"core/go/generics_instantiate.go": 1,
	"core/go/guard_predicate.go":      1,
	// objectMakeSig selects `make`'s [Ideal Map] overload by ARG SHAPE — the
	// arity is one component of the shape, alongside both arg types. That is
	// overload selection by declared signature, which is the argument rule.
	"core/go/record_typed_def.go": 1,

	// ── Native words and modules reading a user fn's declared shape to
	//    present it (help text, macro expansion, behaviour install, codecs).
	"lang/go/native/native_behave.go": 8,
	"lang/go/native/help/help.go":     8,
	// 5 -> 4 (2026-09-25): the runtime `parse <fn>` dispatch's overload-list
	// presence test now reads the value's own signature list through a
	// local (`len(own) == 0`, the list a fn-shaped value resolves to) — the
	// same OVERLOAD-LIST presence test as before, which the walker no
	// longer sees as a selector; no site changed meaning.
	// 4 -> 5 (2026-09-26, the merge of main's #510; main's 5 -> 6, the
	// sweep's last cells): recordMacroFnDispatch picks the emit / mini
	// fn-dispatch native's signature whose arg count is the macro call's
	// SURFACE operand count — overload selection by the declared
	// signature, the argument rule, never behaviour by arity.
	"lang/go/native/native_macro.go": 5,
	"lang/go/native/native_help.go":  3,
	// 1 -> 2: the runtime `parse <fn>` dispatch reads whether a registry
	// binding carries any overloads AT ALL before matching against them
	// (parseFnNativeApply, mirroring eng/go/vm.go::tryNativeFnApply). It is an
	// OVERLOAD-LIST presence test, not a parameter count, and it decides which
	// signature TABLE to match — the value's own, or the one its name resolves
	// to in the registry — never whether the parser may act. That is matching
	// machinery, the argument rule's own, not a behaviour-by-arity exception.
	// 2 -> 3 (2026-09-26, the sweep's last cells): the module build installs
	// parselang-lead-dispatch only when the looked-up native carries exactly
	// its one registered signature (`len(fn.Signatures) == 1`) — the same
	// installation guard the fn dispatch beside it has: an OVERLOAD-LIST
	// presence test on a native the module itself registers, never a
	// decision about a user fn's shape.
	"lang/go/modules/parselang.go": 3,
	"lang/go/modules/net_codec.go": 1,
	"lang/go/modules/test.go":      1,
	// StampBodySig binds a handler's run-time inputs to its throwaway
	// signature's params POSITIONALLY before typing each param by the input
	// it binds — the frame's argument rule (CallBoru binds the same inputs
	// to the same params); an input count that is not the params' stands
	// aside so CallBoru raises what it raised. Matching machinery, not a
	// decision by arity.
	"lang/go/native/body_sig_stamp.go": 1,
	// A StackForm replays a RECORDED call: its Arity is how many operands
	// the recorder saw the call take, the argument rule as it ran, never a
	// function's parameter count. walk.go's structural equality compares
	// it for a named Call and (1 -> 2, 2026-09-25, NUR077) for the new
	// Apply of a fn value; eval.go's Replayable declines an Apply over
	// more than two recorded operands, which no stack shuffle (swap, rot)
	// can lift the applied value above — how the replay DELIVERS the
	// arguments, not what a fn may do by its count.
	"lang/go/stackform/walk.go": 2,
	"lang/go/stackform/eval.go": 1,
	// 1 -> 2 (2026-09-27, NUR222's shuffle-only `do` body): shuffleOnlyBody
	// admits a stack-shuffle word only while its registered native carries
	// exactly its one signature (`len(fd.Signatures) != 1`) — a user overload
	// appended to `drop` may raise. An OVERLOAD-LIST presence test on a
	// native basic itself registers, never a decision about a user fn's
	// shape; the shuffle's stack effect is the word's own table.
	// 2 -> 3 (2026-09-28, the user `drop` in a `do` body): shuffleDispatchSafe
	// admits a user overload that only CONSUMES its arguments with the
	// shuffle's own stack effect — its param count must equal the effect's
	// input count (`len(s.Params) != eff[0]`). Matching the word's stack
	// effect against the parameters it binds, the argument rule's own
	// question, not a choice of behaviour by arity.
	"basic/go/native_control.go":    3,
	"basic/go/native_definition.go": 1,
	// 2026-09-28 (NUR342, a `case` clause block that is a bare function
	// word): callReachesContext asks whether any signature takes an operand
	// or reads the whole stack (`TotalArgs() > 0`) — whether the block's
	// re-step at the case can COLLECT from the values around it, which is
	// how the argument rule reaches past the word. A block that can declines
	// the sealed-arm desugar; every arity that collects is treated alike.
	// The second (2026-09-29, NUR342 compiled) is no arity at all: a guard
	// word the check pass may decide statically must declare NO code-body
	// position (`len(NoEvalArgs) > 0` sizes the map of code-body slots) —
	// it asks whether the word runs a body, and every arity is treated alike.
	"basic/go/conditional.go": 2,
	// The list re-step asks whether a fn value's first declared slot QUOTES
	// (a `/q` slot captures the next word — NUR219, NUR295): the params
	// presence test guards the index it reads. How the argument rule
	// collects that slot, never behaviour decided by the count.
	"eng/go/vm_list_restep.go": 2,
	// A `do`'s re-step asks whether a dispatching fn value's every signature
	// takes no parameter (NUR317): such a value collects nothing — the
	// argument rule reaches no value beneath or after it — so the island's
	// step of the results alone is the interpreter's. How the argument rule
	// collects, never behaviour decided by the count.
	"eng/go/vm_do_restep.go": 1,

	// ── Tooling and fixtures.
	"tools/piecetool/demethod.go": 1,
	"test/specfix/control.go":     1,
}

// selectorCount reports whether e is `<something>.Params`-shaped — a selector,
// never a bare identifier. See the header for why the distinction carries the
// whole design.
func selectorCount(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		return countSel[x.Sel.Name]
	case *ast.CallExpr:
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			return countSel[sel.Sel.Name]
		}
	case *ast.IndexExpr:
		return selectorCount(x.X)
	}
	return false
}

func isArityExpr(e ast.Expr) bool {
	if c, ok := e.(*ast.CallExpr); ok {
		if id, isIdent := c.Fun.(*ast.Ident); isIdent && id.Name == "len" && len(c.Args) == 1 {
			return selectorCount(c.Args[0])
		}
		if sel, isSel := c.Fun.(*ast.SelectorExpr); isSel {
			return arityNames[sel.Sel.Name]
		}
	}
	if s, ok := e.(*ast.SelectorExpr); ok {
		return arityNames[s.Sel.Name]
	}
	return false
}

func TestArityKeyedSitesArePinned(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found := map[string]int{}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "coverage":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			// A file the Go toolchain itself would reject is someone else's
			// failure; the gate only counts what compiles.
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			switch be.Op {
			case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ:
			default:
				return true
			}
			if isArityExpr(be.X) || isArityExpr(be.Y) {
				found[rel]++
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking repo: %v", walkErr)
	}

	for _, rel := range sortedKeys(found) {
		n := found[rel]
		pinned, ok := pinnedAritySites[rel]
		if !ok {
			t.Errorf("arity comparison in UNPINNED file %s (%d site(s)): ADR-016 forbids deciding "+
				"behaviour by a function's parameter count. If this site MATCHES a signature it "+
				"implements the argument rule and belongs in the table with that note; if it "+
				"decides whether a function may act as a predicate, whether a body may compile, "+
				"or which overload is admitted, it is the exception the ADR forbids — key on the "+
				"role or declaration you actually mean", rel, n)
			continue
		}
		if n != pinned {
			t.Errorf("%s has %d arity comparison(s), pinned %d: say which kind the change is "+
				"(matching the argument rule, or deciding behaviour BY arity — ADR-016 / NUR100) "+
				"before moving the number", rel, n, pinned)
		}
	}
	for rel, pinned := range pinnedAritySites {
		if _, ok := found[rel]; !ok && pinned != 0 {
			t.Errorf("pinned site %s (%d) no longer compares an arity — if the special case was "+
				"retired, tighten the table (that is the ratchet working)", rel, pinned)
		}
	}
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
