// aritygate_test.go gates the tree against arity-keyed exceptions.
//
// ADR-016: "Every function behaves the same way whatever its arity and
// wherever it came from … this record forbids exceptions keyed on arity or
// origin." Accepted 2026-08-15, and ruled ABSOLUTE by the maintainer on
// 2026-08-25 — "everything everywhere every time and always".
//
// An absolute rule with no gate is a rule nobody can enforce. NUR100 records
// two live sites that contradict the ADR; a THIRD was found on 2026-08-28 —
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
// forces someone to look and say which kind it is. Two entries are known
// divergences and are marked as such.
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
	"core/go/engine.go":       29,
	"core/go/region_diag.go":  1,
	"core/go/collect_plan.go": 5,
	"core/go/signature.go":    12,
	"core/go/match.go":        1,
	"core/go/fnsig.go":        3,
	"core/go/word_extend.go":  6,
	"core/go/core_helpers.go": 4,
	"core/go/core_ref.go":     3,
	"core/go/unify.go":        3,
	"core/go/deadsig.go":      1,
	"core/go/canon.go":        1,
	"core/go/value.go":        1,
	// NoteFnShape rejects a NEGATIVE claim (`FnShape.Arity < 0`) — a shape a
	// producing word could not build (`partial` over a 0-param fn raises) —
	// a validity guard on the claim itself, not a decision keyed on a
	// function's parameter count (the thirty-fifth increment).
	"core/go/check_state.go":  1,
	"core/go/boru_error.go":   2,
	"core/go/macro_expand.go": 1,

	// ── The checker's and VM's mirrors of that same matching.
	"check/go/carrier.go": 13,
	// 1 -> 2 (2026-09-24, the written argument's fit, NUR194):
	// shapedFnReadWindow guards `i-1 < len(shape.Params)` — a BOUNDS check
	// on the claim's parameter-type slice, which may be shorter than the
	// arity (nil where only the arity is known) — before asking whether a
	// written token fits that parameter. It reads where the types END,
	// never what a function of a given arity may do: every arity takes the
	// same path, and the matching itself is SigTypeMatches, the argument
	// rule's own arm.
	"check/go/method_shape.go":   2,
	"check/go/check_recovery.go": 1,
	// A bounds check on a signature INDEX, not a decision about a function's
	// shape (CompileFnSigUnit guarding fnDef.Signatures[sigIdx] before it
	// compiles that one signature's body as a dispatch of it would — the
	// seventieth increment's replaced outer).
	"check/go/spec_fn_unit.go": 1,
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
	"eng/go/vm.go": 18,
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
	"eng/go/vm_dyn_apply.go":    3,
	"eng/go/vm_rematch.go":      2,
	"eng/go/vm_poly_nomatch.go": 3,
	// closureAsWord bridges a compiled closure to a handler-bearing FnDefInfo
	// so the interpreter's WORD dispatch can match it (NUR123): the bridge
	// DECLARES the callee's signature from the unit's own param count and
	// types — the argument rule's input, not a decision about what a closure
	// of a given arity may do; a closure of any arity takes the same path.
	"eng/go/vm_dyn_words.go": 1,
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

	// ── NUR100 §1, a NAMED DIVERGENCE: RunPredicate decides whether a
	//    function may act as a predicate at all by counting its parameters.
	//    "predicate type K: RunPredicate: predicate must take exactly one
	//    argument" — two functions that both express a membership test are
	//    admitted or declined on arity alone. No verdict yet: the predicate
	//    role does need to test ONE value, so removing the gate needs a
	//    replacement contract, not a deletion.
	"core/go/registry.go": 3,

	// ── NUR100 §2, a NAMED DIVERGENCE: smallerArityOverload declines a poly
	//    window when the word registers an overload consuming FEWER operands.
	//    Lower stakes than §1 (compile-coverage conservatism, not an answer
	//    change — the lane falls back and the results agree), but the same
	//    shape.
	"compiler/go/compiler_dispatch_record.go": 2,

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
	"compiler/go/emit.go": 8,
	// sameFnDecls compares two fn VALUES for declaration identity — the
	// same signature list: the same count, then each position's declaration
	// site (Signature.Decl). It decides whether a unit's recorded def event
	// IS the current binding (a closed body's redefinition is not), never
	// what a function of a given arity may do; every arity takes the path —
	// the seventy-second increment (review of #468).
	"compiler/go/fn_local.go":       1,
	"compiler/go/user_poly.go":      1,
	"compiler/go/callable_words.go": 1,
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
	"compiler/go/bytecode.go": 1,

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
	"lang/go/native/native_macro.go":  5,
	"lang/go/native/native_help.go":   3,
	// 1 -> 2: the runtime `parse <fn>` dispatch reads whether a registry
	// binding carries any overloads AT ALL before matching against them
	// (parseFnNativeApply, mirroring eng/go/vm.go::tryNativeFnApply). It is an
	// OVERLOAD-LIST presence test, not a parameter count, and it decides which
	// signature TABLE to match — the value's own, or the one its name resolves
	// to in the registry — never whether the parser may act. That is matching
	// machinery, the argument rule's own, not a behaviour-by-arity exception.
	"lang/go/modules/parselang.go":  2,
	"lang/go/modules/net_codec.go":  1,
	"lang/go/modules/test.go":       1,
	"lang/go/stackform/walk.go":     1,
	"basic/go/native_control.go":    1,
	"basic/go/native_definition.go": 1,

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
