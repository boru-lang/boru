package check

// The check-mode recovery and advisory surface of the step loop —
// dispatch-failure modeling (surface shapes, assumed signatures,
// fallback positions), check-result splicing, mixed-form advisories,
// stranded-operand compile failures, and the check-state sharing brackets.
// Extracted from engine.go in Stage 0c of the four-piece split
// (design/legacy/ENG-FOUR-PIECE.0.ignore): this file is the CHECK piece's half of
// the interpreter's dispatch machinery and moves behind seam S1
// (AnalysisHooks) when the packages cut.

import (
	core "github.com/boru-lang/boru/core/go"

	"strconv"
	"strings"
)

// undefinedWordCheckDiag is undefinedWordError's check-mode twin — the
// same Detail and did-you-mean, carried on the CheckDiagnostic wire
// type (the context hint is runtime-shaped and stays off it).
func undefinedWordCheckDiag(e *core.Engine, name string, pos core.SrcPos) core.CheckDiagnostic {
	return core.CheckDiagnostic{
		Code:        "undefined_word",
		Detail:      core.UndefinedWordDetail(name),
		Word:        name,
		Row:         pos.Row,
		Col:         pos.Col,
		Suggestions: e.DidYouMeanSuggestions(name),
	}
}

// drainUndefinedAtoms replaces dangling Undefined atoms with Any carriers
// (check mode only — outside check mode stepWord errors on undefined words,
// so this is a no-op): an analysis-time-undefined read inside a fn body can
// still be a DYNAMIC-SCOPE reference bound by a live frame at run time (a
// recursive base case reading the previous frame's body-local —
// recursion.tsv:71).
func drainUndefinedAtoms(e *core.Engine) {
	for i := 0; i < e.Tape.Len(); i++ {
		und := e.Tape.At(i)
		if !core.IsUndefined(&und) || !e.Registry.Check.IsActive() {
			continue
		}
		e.Tape.Set(i, core.NewCarrier(core.TAny))
	}
}

// tagCheckModeDefRead records the binding's NAME on a def-word substitution
// (DynFrom, read by the narrowing-through-use rule, which is Dynamic-gated,
// so a non-dynamic tag is inert there). Two cases tag:
//
//   - a DYNAMIC (gradual) value: the tag lets a typed downstream use narrow the
//     binding (narrowing-through-use);
//   - a CONCRETE MODULE-SCOPE mutable reference (a `flex` map/list/xml, a Store
//     bound outside the current fn frame) or a MODULE-FAMILY value bound at
//     module scope — an `import`-bound namespace (`IO`, `StringUtil`) or a
//     Module descriptor (`def m StringUtil.$module`) — read as a VALUE: the
//     binding lives in the registry Defs for the whole run, and its identity
//     IS the binding's. A BODY-LOCAL flex is a frame local, not a registry
//     binding, and stays untagged.
//
// Extracted from stepWord so the hot dispatch path stays under the cyclomatic-
// complexity gate.
func tagCheckModeDefRead(e *core.Engine, top *core.Value, name string, _ core.SrcPos) {
	switch {
	case core.IsDynamic(top):
		top.SetDynFrom(name)
	case (core.IsFlexMap(*top) || core.IsFlexList(*top) || core.IsFlexXml(*top) || core.IsStore(*top) ||
		core.IsModuleFamilyValue(*top)) && core.ModuleScopeBinding(e.Registry, name):
		top.SetDynFrom(name)
	}
}

// checkMixedFormAdvisories emits the two check-mode forward-greediness
// advisories when a word forward-collects an argument AND also takes a stack
// argument (a mixed-form dispatch, at any arity): the forward-strand advisory (the
// `1 2 add 3 mul → 5` surprise — `add` grabs the forward `3` and strands the
// `1`) and the mixed-form-call advisory (a 3+-arg call taking operand(s) from a
// PRECEDING expression while forward-collecting binds differently from the
// all-forward reading — `(cond) if [a] [b]`). Both are advisory only (info
// severity), emitted in check mode, never gating. The mixed-form arm fires only
// when the deepest stack-bound slot is Any-typed (the genuine footgun, if3's
// {Any,Any,Any}); a concretely-typed receiver-first idiom (`xs set 0 v`,
// `s slice j e`) binds correctly and stays quiet — without that gate the
// advisory over-fired ~190 false info across the voxgig-boru libraries. Extracted
// from stepWord so the hot dispatch path stays under the cyclomatic-complexity
// gate. See design/FORWARD-STRAND-ADVISORY.10.md, ERRORS.8.md §6.2.
func checkMixedFormAdvisories(e *core.Engine, w core.WordInfo, sig *core.Signature, positions []int, pos core.SrcPos, fwdCount, stkCount int) {
	if !(e.Registry.Check.IsActive() && fwdCount > 0 && stkCount > 0) {
		return
	}
	checkForwardStrandsOperand(e, w, sig, positions, pos)
	if sig.TotalArgs() >= 3 && core.MixedFormStackSlotAny(e, sig, positions) {
		e.Registry.Check.AddDiagnostic(core.CheckDiagnostic{
			Code: "mixed_form_call",
			Detail: w.Name + " takes " + strconv.Itoa(stkCount) + " argument(s) from the stack while forward-collecting " +
				strconv.Itoa(fwdCount) + " — the mixed form binds differently from the all-forward form; " +
				"prefer " + w.Name + " arg1 arg2 … or group explicitly",
			Word: w.Name,
			Row:  pos.Row,
			Col:  pos.Col,
		})
	}
}

// checkForwardStrandsOperand implements the "forward greediness" advisory
// (check mode only). Preconditions (checked by the caller): the dispatch is
// mixed — it forward-collected ≥1 arg AND took ≥1 stack arg.
//
// It flags the case where a SIBLING operand is stranded: a value sitting on
// the stack just below the deepest stack arg the word consumed, in the same
// scope, whose type matches that consumed slot — i.e. an operand the word
// could equally have taken from the stack. The sibling-type test is what
// separates the genuine `1 2 add 3` gotcha (a stranded Number under `add`)
// from a deliberately-kept value of an unrelated type left by an earlier
// statement.
func checkForwardStrandsOperand(e *core.Engine, w core.WordInfo, sig *core.Signature, positions []int, pos core.SrcPos) {
	// Deepest stack position the word consumed, and its sig slot.
	minStack := -1
	minSigPos := -1
	for sp, p := range positions {
		if p < e.Pointer && (minStack == -1 || p < minStack) {
			minStack = p
			minSigPos = sp
		}
	}
	if minStack <= 0 || minSigPos < 0 {
		return
	}
	slotType := core.SigArgType(sig, minSigPos)
	// An Any-typed slot matches everything, so it cannot tell a sibling
	// operand from unrelated residue — too weak to be a reliable signal.
	if slotType == nil || slotType.Equal(core.TAny) {
		return
	}

	// Scan downward from just below the consumed stack args, stopping at the
	// nearest scope boundary. The first data value found is "stranded".
	for i := minStack - 1; i >= 0; i-- {
		v := e.Tape.At(i)
		if core.IsOpenParen(&v) || core.IsCloseParen(&v) || core.IsForward(&v) || core.IsDefCleanup(&v) ||
			v.Parent().ConformsTo(core.TMark) || v.Parent().ConformsTo(core.TMove) ||
			v.Parent().ConformsTo(core.TInternal) || v.Parent().ConformsTo(core.TReturnCheck) {
			return // scope boundary: nothing stranded in this scope
		}
		if !core.IsConcrete(&v) && !core.IsBareTypeNode(&v) {
			continue // structural residue, keep scanning
		}
		// Sibling-operand heuristic: only flag a stranded value the word
		// could itself have consumed (same type as the slot it just took).
		if v.Is(slotType) {
			e.Registry.Check.AddDiagnostic(core.CheckDiagnostic{
				Code: "forward_strands_operand",
				Detail: w.Name + " collected a forward argument while a " +
					slotType.String() + " operand was left unconsumed on the stack — " +
					"it may be stranded; group the intended operands, e.g. (… " + w.Name + " …)",
				Word: w.Name,
				Row:  pos.Row,
				Col:  pos.Col,
			})
		}
		return // only the nearest value below matters
	}
}

// valueCarriesCarrier reports whether v is a check-mode carrier or a
// container holding one at any depth — the shape a hole's auto-eval
// leaves when a list/map literal embeds a computed element. Used by the
// interpolation bake probe above; at run time there are no carriers, so
// the walk is check-mode-only work.
func valueCarriesCarrier(v core.Value) bool {
	if v.Carrier() {
		return true
	}
	switch d := v.Data().(type) {
	case core.ListPayload:
		for _, e := range d.Elems {
			if valueCarriesCarrier(e) {
				return true
			}
		}
	case core.MapPayload:
		if d.M != nil {
			for _, k := range d.M.Keys() {
				if mv, ok := d.M.Get(k); ok && valueCarriesCarrier(mv) {
					return true
				}
			}
		}
	}
	return false
}

// exprRefsCarrier reports whether a folded container expression references a
// def-bound name whose current value is a CARRIER — a computed value or a loop
// iterator, abstract at check time. Folding such an expression runs the handler
// against the carrier, which coerces (e.g. AsInteger -> 0), so the fold freezes
// a wrong constant. A user TYPE binding (a type literal, Carrier=false) or a
// concrete literal binding is NOT a carrier and still folds. Walks nested paren-
// exprs, lists, and map values; builtins (not in Defs) are ignored.
func exprRefsCarrier(e *core.Engine, items []core.Value) bool {
	r := e.Registry
	found := false
	var walk func(vs []core.Value)
	walk = func(vs []core.Value) {
		for _, v := range vs {
			if found {
				return
			}
			if core.IsWord(&v) {
				if w, err := core.AsWord(&v); err == nil {
					if bound, ok := r.Defs.Top(w.Name); ok && bound.Carrier() {
						found = true
						return
					}
				}
			}
			if core.IsParenExpr(&v) {
				if toks, err := core.AsParenExpr(&v); err == nil {
					walk(toks)
				}
				continue
			}
			// A Reach can hide a carrier in its receiver or a computed key
			// (`m.(k)` with a def-local carrier `k`): recurse into both so the
			// fold declines rather than baking the carrier's check-time value.
			if core.IsReach(&v) {
				if ri, err := core.AsReach(&v); err == nil {
					walk(ri.Receiver)
					for i := range ri.Segments {
						if ri.Segments[i].Computed {
							walk(ri.Segments[i].KeyExpr)
						}
					}
				}
				continue
			}
			if lst, err := core.AsList(v); err == nil && !lst.IsNil() {
				walk(lst.Elems())
				continue
			}
			if mp, err := core.AsMap(v); err == nil && mp != nil {
				for _, k := range mp.Keys() {
					mv, _ := mp.Get(k)
					walk([]core.Value{mv})
				}
			}
		}
	}
	walk(items)
	return found
}

// concreteEvalOnce runs items in a throwaway sub-engine with check mode OFF (so
// the result is a real value, not a carrier) and returns the single concrete
// residual. The def stack
// is snapshotted and restored exactly (the content-preserving pair: a pop or a
// same-depth rebind inside the run is undone too) so a stray binding cannot
// leak into the compile.
//
// A run that CHANGED a binding declines: the fold is a stand-in for a pure
// computation, and the interpreter's evaluation of the same tokens keeps
// that change — a map literal's value `[def k 1 k]` evaluated as a word's
// argument binds k for the rest of the program — where a folded constant
// would change nothing (NUR330: `def k 5 size {a:[def k 1 k]} k` is [1 1]).
// Declined, the expression is modelled as it runs, its binding change with
// it.
func concreteEvalOnce(e *core.Engine, items []core.Value) (core.Value, bool) {
	r := e.Registry
	snap := r.Defs.SnapshotEntries()
	prev := r.Check.Mode
	r.Check.Mode = false
	res, err := core.RunContainerSub(r, items, false)
	r.Check.Mode = prev
	// A break/continue escaping the expression (a container member's run,
	// NUR358) is no constant: the fold declines, and the signal stays the
	// run's to raise, not this scratch run's.
	escaped := core.BodyEscaped(r)
	if escaped {
		r.TakeFlow()
	}
	changed := r.Defs.ChangedSince(snap)
	r.Defs.RestoreEntriesSnapshot(snap)
	if escaped || changed || err != nil || len(res) != 1 || !core.IsConcrete(&res[0]) {
		return core.Value{}, false
	}
	return res[0], true
}

// spliceAnonCheckResult runs AnalyseFnBody on an anonymous FnDef in
// check mode and splices the residual carrier stack as the dispatch
// result. This bypasses the body splice + ReturnCheck path that named
// fns use: an anonymous lambda's static Returns is the conservative
// [Any], and AnalyseFnBody recovers the real return type for downstream
// type propagation.
func spliceAnonCheckResult(e *core.Engine, valIdx, nArgs int, sig *core.FnSig, args []core.Value, captures []core.CapturedBinding) error {
	paramNames := make([]string, len(sig.Params))
	for i, p := range sig.Params {
		paramNames[i] = p.Name
	}
	result := AnalyseFnBody(e.Registry, "", paramNames, sig.Body(), args, captures, sig.Returns, true)
	result = trimUnnamedArgs(result, len(sig.Returns), unnamedParamCount(sig.Params))
	if len(result) == 0 {
		result = []core.Value{core.NewCarrier(core.TAny)}
	}
	spliceFnCheckTail(e, valIdx, nArgs, result)
	return nil
}

// trimUnnamedArgs is the frame return's discipline over an analysed
// anonymous body's residual (the interpreter's ReturnCheck): the UNNAMED
// params were pushed beneath the body, and the frame keeps its nret returns
// off the top, discarding up to unnamed unconsumed args from the bottom. So
// `(0 ([0] => [1]))` nets the one value 1 on the interpreter, and the call's
// model must seat one: it seated the pushed 0 beside it, and a list after
// the call underflowed at run time (NUR255). A residual the unnamed args
// cannot account for is left as it is, since the interpreter raises its
// count error there.
func trimUnnamedArgs(result []core.Value, nret, unnamed int) []core.Value {
	if extra := len(result) - nret; nret > 0 && extra > 0 && extra <= unnamed {
		return result[extra:]
	}
	return result
}

// SpliceFnValueCheckResult is the check-mode dispatch for a NON-anonymous
// body-bearing fn VALUE (a `fn` literal resolved from a map/module export and
// then CALLED, e.g. `ParseLang.parse_json 'x' {}`). Unlike a NAMED user fn
// (which dispatches through stepWord → its registered ReturnsFn) and unlike an
// anonymous lambda (spliceAnonCheckResult, analysis-only), a called fn value
// previously fell through to execFnDefSig, whose inline body splice leaks the
// frame's per-call teardown into the TOP-LEVEL residual — at the time a
// `__pa` (Args/FnBaseline pop) token, declined by the emitter as
// "context-dependent word __pa"; since 2026-10-08 the frame's DefCleanup
// marker. Routing through BuildFnBodyReturnsFn ARMS the body analysis via
// StartFnCompile, so the body (with its frame tail) is captured INSIDE its
// own CALL_USER unit and the call site records a CALL_USER — identical to
// the named-fn path. See
// design/legacy/boru-bytecode-stage3-inlining-plan.0.ignore "THE shared crux:
// body-bearing fn-VALUE dispatch (__pa)".
func SpliceFnValueCheckResult(e *core.Engine, valIdx, nArgs int, fnDef core.FnDefInfo, sig *core.FnSig, args []core.Value) error {
	returns := BuildFnBodyReturnsFn(e.Registry, fnDef.Name, *sig, fnDef)
	result := returns(args, e.Registry)
	if len(result) == 0 && len(sig.Returns) > 0 { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		// A declared-return fn that produced no carrier (the body unit
		// declined to compile) degrades to one carrier per declared return so
		// downstream provenance declines and the program falls back faithfully.
		result = make([]core.Value, len(sig.Returns))
		for i, t := range sig.Returns { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
			result[i] = core.NewCarrier(t)
		}
	}
	spliceFnCheckTail(e, valIdx, nArgs, result)
	return nil
}

// spliceFnCheckTail removes the consumed args + the fn-value literal from the
// tape and splices the check-mode result carriers in their place. Shared by
// spliceAnonCheckResult and SpliceFnValueCheckResult so the two check-mode
// fn-value paths cannot diverge in their stack discipline.
func spliceFnCheckTail(e *core.Engine, valIdx, nArgs int, result []core.Value) {
	indices := e.ResolvedIndicesBefore(nArgs)
	if len(indices) == nArgs && nArgs > 0 {
		firstArgIdx := indices[0]
		skipSet := make(map[int]bool, nArgs+1)
		for _, idx := range indices {
			skipSet[idx] = true
		}
		skipSet[valIdx] = true
		dst := firstArgIdx
		for i := firstArgIdx; i <= valIdx; i++ {
			if !skipSet[i] {
				e.Tape.Set(dst, e.Tape.At(i))
				dst++
			}
		}
		e.Tape.Splice(dst, valIdx+1-dst, result...)
		e.Pointer = firstArgIdx
	} else if nArgs == 0 {
		e.Tape.Splice(valIdx, 1, result...)
	} else {
		argStart := valIdx - nArgs
		if argStart < 0 {
			argStart = 0
		}
		e.Tape.Splice(argStart, valIdx+1-argStart, result...)
		e.Pointer = argStart
	}
}

// shareCheckState lets a module sub-registry's fn body run IN CHECK MODE under
// the parent compile pass. When the calling engine is in check mode and the body
// executes in a DIFFERENT (captured module) registry, it points that registry's
// Check at the parent's for the duration of the call, so mode, Emit recording,
// the memo / in-flight maps and the step/budget counters are shared — while word
// resolution still uses the module registry's own Defs/Types (untouched). It
// returns a restore function (a no-op when no sharing applies). The shared memo
// keys stay disjoint across the boundary via the per-registry scopeID prefix
// (§5a), so a module fn and a parent fn of the same name cannot alias. See
// design/legacy/module-fn-checkstate-ownership.1.ignore §5b.
func shareCheckState(e *core.Engine, capturedReg *core.Registry) func() {
	return shareCheckStateFrom(capturedReg, e.Registry)
}

// shareCheckStateFrom is the registry-level mechanism behind shareCheckState:
// it points owner's Check at caller's for the duration (restore via the
// returned func), no-op when the registries coincide, either is nil, or the
// caller is not in check mode. Split out so the MERGED-WORD seam can share at
// the ReturnsFn boundary itself (BuildFnBodyReturnsFn — Stage M1,
// design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore §5): a transplanted word-extension
// sig dispatches as a BARE word on the importer's engine, where no
// execFnDefLiteral wrapper exists to share around the call, and the sig's
// owning registry is known only to the ReturnsFn closure (the transplant
// clone's FnDefInfo.Registry is deliberately nil — see TransplantExtension).
// Idempotent under nesting: when an enclosing dispatch already shared, the
// swap is pointer-equal and the restore puts back the same shared state.
// ShareCheckStateFrom is shareCheckStateFrom exported for the COMPILER's
// cross-registry closure compile (compiler/go/callable_words.go): a foreign
// fn value's body must resolve its free words in the DEFINING registry while
// the analysis it drives — params, carriers, the recorder — stays the
// CALLER's, because the unit it produces has to land in the caller's program.
// Same mechanism, same restore contract, same idempotence under nesting.
func ShareCheckStateFrom(owner, caller *core.Registry) func() {
	return shareCheckStateFrom(owner, caller)
}

func shareCheckStateFrom(owner, caller *core.Registry) func() {
	if owner == nil || caller == nil || owner == caller || !caller.Check.IsActive() {
		return func() {}
	}
	saved := owner.Check
	owner.Check = caller.Check
	return func() { owner.Check = saved }
}

// noteSpeculativeBarrierCommit emits the speculative_forward_commit
// advisory (check mode only, info severity, never gating): a parked
// word committed at a statement boundary AND its plan had filled a
// forward slot with a dispatching word — the else-less-guard shape.
// The commit resolves it correctly; the note exists because the
// source reads ambiguously, and an explicit `[]` else (or `end`)
// states the intent. Structurally silent on the `def name fn […]`
// idiom: there the smaller-arity probe FAILS, no commit happens, and
// this helper is never reached.
func noteSpeculativeBarrierCommit(e *core.Engine, fwd core.ForwardInfo) {
	if e.Registry == nil || !e.Registry.Check.IsActive() || !fwd.Speculative {
		return
	}
	detail := fwd.FuncName + " committed with " +
		strconv.Itoa(fwd.CollectedArgs+fwd.StackArgs) + " argument(s) at a statement boundary"
	if e.Pointer >= 0 && e.Pointer < e.Tape.Len() {
		if bw, err := core.AsWord(e.Tape.Ref(e.Pointer)); err == nil {
			detail += " (`" + bw.Name + "`)"
		}
	}
	detail += " — its trailing slot " + strconv.Itoa(fwd.SpeculativeAt) +
		" was planned from the following word; an explicit `[]` else or `end` makes the intent loud"
	e.Registry.Check.AddDiagnostic(core.CheckDiagnostic{
		Code:   "speculative_forward_commit",
		Detail: detail,
		Word:   fwd.FuncName,
		Row:    fwd.Pos.Row,
		Col:    fwd.Pos.Col,
	})
}

// checkModeParenFnCollapse collapses a fn-CARRIER apply window on the PLAIN
// check surface — the recorder is inactive or suspended (a construction-time
// AnalyseFnBody, a bare `check` run), so nothing records; the tape model
// simply nets what the interpreter nets. Two shapes, mirroring the compile
// pass's RecordDynApply admissions exactly so the two surfaces report the
// same diagnostics (completeness-review §8.4.2):
//
//   - TRAILING carrier apply `(a b comp)` — the runtime applies comp over
//     the whole window (the comparator convention), netting ONE value;
//   - LEADING one-arg carrier apply `(g x)` — the arity where leading and
//     trailing collection converge (§9.6b), netting ONE value or raising
//     identically in both spellings.
//
// The window collapses to ONE dynamic(Any) carrier — the honest gradual
// model of "some single runtime result". Without this the un-collapsed
// [carrier, arg] residual nets TWO values and stalls a pending `def`'s
// collection, flagging `undefined_word` on the def-bound name in a program
// that runs clean — the §9.4 def-split FALSE POSITIVE (the check_run_fp
// +74 class). A CONCRETE fn value in either position is untouched: the
// check step dispatches it for real. A Dynamic lead keeps the
// tryDynamicFnValueDispatch path. Multi-arg leading windows stay
// un-collapsed (the spellings' collection orders diverge beyond one
// argument — same edge as the compile side). Returns the possibly-shrunk
// closeIdx.
//
// The leading case admits a DYNAMIC argument (NUR087's fix, 2026-08-24).
// It previously required `!last.Dynamic`, which left exactly this
// machinery's own false-positive class open one step further out: `def r2
// (b r1)` — a call through a Function PARAM whose argument is an earlier
// dynamic binding — did not collapse, so the pending `def` collected
// nothing and every later read of `r2` raised a false `undefined_word`.
// That is the shape the audit's parser-combinator library is built from
// (`def r1 (a s)` then `def r2 (b (r1.rest))`), and it made plain
// `boru run` decline a program both engines run correctly. A dynamic
// argument makes the RESULT no less knowable than a static one — the
// window collapses to dynamic(Any) either way — so the restriction bought
// no soundness. `IsFnValueResidual` excluded a trailing fn VALUE until
// 2026-09-22 (S1b's apply shapes): an INERT fn value at the argument
// position — a `/v` read, a lambda literal — is a value the lead COLLECTS
// (a Function or Any param binds it, any other no-matches), one call on
// both engines, and leaving the window un-collapsed flagged a false
// `type_error` on `def hof2 fn [[f:Function][Integer][(f ([n:Integer] =>
// [n add 1]))]]` ("return value 1: expected Integer, got Function") that
// the compile-armed pass, which records the window (RecordDynApplyLead),
// did not — the one new diagnostic-parity divergence the landing made. A
// gradual (Dynamic) fn-typed argument is still not collapsed: it may be a
// bare fn WORD at run time, which the lead's collection meets as a barrier.
func checkModeParenFnCollapse(e *core.Engine, openIdx, closeIdx int) int {
	if !e.Registry.Check.Mode {
		return closeIdx
	}
	count, lastIdx, leadIdx := 0, -1, -1
	for i := openIdx + 1; i < closeIdx; i++ {
		v := e.Tape.At(i)
		if !core.IsRecordableLiteral(v) {
			continue
		}
		if count == 0 && !core.IsDynamic(&v) && !core.IsQuoted(&v) && core.IsFnTypedCarrier(&v) {
			leadIdx = i
		}
		count++
		lastIdx = i
	}
	if count < 2 {
		return closeIdx
	}
	last := e.Tape.At(lastIdx)
	trailing := !core.IsDynamic(&last) && !core.IsQuoted(&last) && core.IsFnTypedCarrier(&last)
	leading := leadIdx >= 0 && count == 2 && (!core.IsFnValueResidual(last) || !core.IsDynamic(&last))
	if !trailing && !leading {
		return closeIdx
	}
	anchor := lastIdx
	if leading {
		anchor = leadIdx
	}
	out := core.NewCarrier(core.TAny)
	core.SetDynamic(&out, true)
	core.ThreadPos(&out, e.Tape.Ref(anchor))
	// Seat the collapsed carrier at the window's first recordable literal
	// and splice every later one out.
	seat := -1
	for i := openIdx + 1; i < closeIdx; i++ {
		if core.IsRecordableLiteral(e.Tape.At(i)) {
			seat = i
			break
		}
	}
	e.Tape.Set(seat, out)
	for i := closeIdx - 1; i > seat; i-- {
		if core.IsRecordableLiteral(e.Tape.At(i)) {
			e.Tape.Remove(i)
			closeIdx--
		}
	}
	return closeIdx
}

// checkModeFallbackPositions returns up to n stack indices to use as
// argument positions when a check-mode fallback fires (no signature
// matched, assume first candidate). Values before the pointer are
// preferred (normal stack order); any shortfall is filled from
// values after the pointer, skipping control tokens. Types are not
// verified — this is the "assume" path.
func checkModeFallbackPositions(e *core.Engine, n int) []int {
	positions := e.ResolvedIndicesBefore(n)
	remaining := n - len(positions)
	// depth tracks open forward-groups entered during this walk so the close that
	// returns to depth 0 is the ENCLOSING group's `)` — a forward arg never crosses
	// it. Without this break the recovery could gather positions PAST the group's
	// close when its assumed arity exceeds the real arg count; splicing those out
	// then deletes tokens across the `)` boundary and leaves a phantom "unmatched
	// opening parenthesis" in a later (balanced) fn body — the emergent whole-module
	// paren bleed (template.boru's first-word / after-word / parts errors).
	depth := 0
	for i := e.Pointer + 1; remaining > 0 && i < e.Tape.Len(); i++ {
		v := e.Tape.At(i)
		if core.IsCloseParen(&v) {
			if depth == 0 {
				break
			}
			depth--
			continue
		}
		if core.IsOpenParen(&v) {
			depth++
			continue
		}
		if core.IsForward(&v) || core.IsMark(&v) || core.IsMove(&v) ||
			core.IsReturnCheck(&v) || core.IsDefCleanup(&v) {
			continue
		}
		positions = append(positions, i)
		remaining--
	}
	return positions
}

// checkModeFallbackPositionsFor is checkModeFallbackPositions for ONE
// candidate signature under the dispatching word's modifiers, laid out as
// the interpreter's MatchSignature would lay the call out: the FORWARD-
// eligible leading positions (sig[0..limit), effectiveForwardLimit — the
// declared barrier, `/s` nothing, `/f` everything) are filled from the
// tokens after the pointer FIRST, while each token is compatible with its
// position (a type match, or a wildcard the assume path cannot type: an
// Any carrier, a raw word), and only the remainder from the stack, top-down;
// a shortfall on the stack is then filled from further forward tokens as
// before (the assume path verifies nothing). Returns the positions in TAPE
// order — the stack run ascending, then the forward run in source order —
// and the length of the stack run, which is what SigOrderArgs needs to
// rebuild signature order.
//
// The stack-first order the plain gatherer keeps was the mechanism of
// NUR180: inside an UNNAMED-param frame the frame's input sits on the
// stack beneath a trailing paren apply's strict-Any result, so a typed
// word over that result — `xs each [(2 (mk 1)) mul 10]` — was recovered
// over [input, result] with the written `10` left unconsumed, and each
// body netted 10 for the interpreter's 30; the same body in a named frame
// (one stack value, the shortfall filled forward) agreed. The interpreter's
// forward phase takes the written argument first whatever the stack holds,
// and so does this.
func checkModeFallbackPositionsFor(e *core.Engine, s *core.Signature, w core.WordInfo) (positions []int, nStack int) {
	n := s.TotalArgs()
	limit := core.EffectiveForwardLimit(s, w)
	if limit < 0 || limit > n {
		limit = n
	}
	var forward []int
	depth := 0
	// The forward walk: the plain gatherer's own (markers skipped, a nested
	// group entered, the ENCLOSING group's close a hard stop), taking at
	// most `limit` compatible tokens.
	for i := e.Pointer + 1; len(forward) < limit && i < e.Tape.Len(); i++ {
		v := e.Tape.At(i)
		if core.IsCloseParen(&v) {
			if depth == 0 {
				break
			}
			depth--
			continue
		}
		if core.IsOpenParen(&v) {
			depth++
			continue
		}
		if core.IsForward(&v) || core.IsMark(&v) || core.IsMove(&v) ||
			core.IsReturnCheck(&v) || core.IsDefCleanup(&v) {
			continue
		}
		if !fallbackTokenCompatible(s, len(forward), v) {
			break
		}
		forward = append(forward, i)
	}
	positions = e.ResolvedIndicesBefore(n - len(forward))
	nStack = len(positions)
	positions = append(positions, forward...)
	if len(positions) < n {
		// The stack is short: fill from the tokens after the forward run,
		// exactly as the plain gatherer fills its shortfall (positions past
		// the last taken forward token, the same walk). The fill cannot
		// overrun n: the plain gatherer takes the same stack run (nStack
		// indices, all the stack holds) and then at most n-nStack tokens of
		// the SAME walk, of which the forward run is a prefix — this walk only
		// adds the compatibility stop — so skipping the taken tokens appends
		// at most what is missing, never more.
		rest := checkModeFallbackPositions(e, n)
		for _, p := range rest[nStack:] {
			taken := false
			for _, q := range forward {
				if q == p {
					taken = true
					break
				}
			}
			if !taken && p > e.Pointer {
				positions = append(positions, p)
			}
		}
	}
	return positions, nStack
}

// fallbackTokenCompatible reports whether a forward token can fill sig
// position idx on the assume path: a static type match, or a value the
// path cannot type and the interpreter's forward phase would still take —
// an Any carrier (the scoring's wildcard), a raw word the dispatch has not
// yet resolved.
func fallbackTokenCompatible(s *core.Signature, idx int, v core.Value) bool {
	if core.IsWord(&v) || v.Parent() == nil || v.Parent().Equal(core.TAny) {
		return true
	}
	return core.SigArgMatches(s, idx, &v)
}

// checkModeAssumeSig is the recovery path for unmatched signatures in
// check mode: emit a diagnostic (with pos attached), gather up to N
// adjacent positions as synthetic args, synthesise carrier results
// from the assumed signature, and splice them over the word +
// consumed positions.
//
// This path deliberately bypasses forward collection and type
// matching — both would cascade failures. The trade-off is that the
// checker reports one diagnostic per site and keeps going with the
// assumed signature's declared return types (or Any if unannotated).
// checkModeSurfaceShape is the S2 typing path: when the concrete
// overloads of w reject the candidate args but one of them is a
// surface-typed carrier whose contract REQUIRES w, the call types as
// the surface's fnsig shape with Self := the surface node — the
// contract guarantees the operation exists for every member, so the
// checker uses the declared shape instead of degrading through the
// assume-sig path (which would emit a spurious no_signature and often
// land on Any). Returns handled=false when no nearby candidate arg is
// a surface carrier requiring w.
func checkModeSurfaceShape(e *core.Engine, w core.WordInfo, pos core.SrcPos) (bool, error) {
	// Locate a surface-typed candidate arg requiring w among the
	// nearby positions (same neighbourhood the assume-sig path
	// gathers; MaxArgs would over-collect, 4 covers surface op
	// arities in practice).
	var sinfo *core.SurfaceInfo
	var shape core.Value
	for _, p := range checkModeFallbackPositions(e, 4) {
		v := e.Tape.At(p)
		// A position AFTER the pointer may still hold the raw Word
		// token (forward args resolve during collection, which the
		// fallback path bypasses). Resolve it the way the forward scan
		// would — via the def stack — so a def-bound surface carrier
		// (e.g. a generic fn's surface-bounded `x:T` param inside
		// AnalyseFnBody, design/legacy/GENERICS.10.ignore Phase 5) is visible to
		// the S2 scan.
		if core.IsWord(&v) {
			if wv, werr := core.AsWord(&v); werr == nil {
				if top, ok := e.Registry.Defs.Top(wv.Name); ok {
					v = top
				}
			}
		}
		if v.Parent() == nil {
			continue
		}
		info, ok := core.SurfaceInfoOf(v.Parent())
		if !ok {
			continue
		}
		sv, found := info.Required.Get(w.Name)
		if !found {
			continue
		}
		sinfo, shape = info, sv
		break
	}
	if sinfo == nil {
		return false, nil
	}
	undef, ok := shape.Data().(core.FnUndefInfo)
	if !ok || len(undef.Sigs) == 0 {
		return false, nil
	}
	spec := core.SubstituteSelf(undef.Sigs[0], sinfo.Type)
	synth := &core.Signature{Params: spec.Params, Returns: spec.Returns}
	core.NormalizeSig(synth)

	n := synth.TotalArgs()
	positions := checkModeFallbackPositions(e, n)
	args := make([]core.Value, len(positions))
	for i, p := range positions {
		args[i] = e.Tape.At(p)
	}
	results := CarrierResults(e.Registry, w.Name, synth, args, pos, nil, false)
	spliceCheckResults(e, positions, results)
	return true, nil
}

// checkModeAssumeSig recovers an unmatched dispatch of fn — a registry entry,
// never a renamed or rebound copy of one.
func checkModeAssumeSig(e *core.Engine, w core.WordInfo, fn *core.FnDefInfo, fallback *core.Signature, pos core.SrcPos) error {
	// Publish the dispatching word token for every ReturnsFn this recovery
	// can invoke — the disjunct-partition combos, the sole-overload user-fn
	// recovery. The matched path publishes it in declaredReturnCarriers;
	// this is the unmatched twin, reached with pos = val.Pos(), the word's
	// own position.
	e.Registry.Check.CurCallWord, e.Registry.Check.CurCallPos = w.Name, pos
	defer e.ClearRecoveryRaw()
	// Gather candidate positions once and try to pick a signature
	// whose arity matches and whose declared types are compatible
	// with (or at least not contradicted by) the actual carrier
	// args. TAny carriers are treated as wildcards.
	best := fallback
	bestMatch := -1
	// Scan all signatures and pick the best fit. Scoring:
	//  - compatible concrete-type matches count.
	//  - ties break toward sigs with ReturnsFn (carry custom
	//    check-mode logic) over plain Returns (static list).
	// When nothing is concretely compatible, fall through to
	// scanning by arity alone so we still land on a ReturnsFn-
	// bearing sig when possible rather than a static catch-all.
	bestHasFn := fallback.ReturnsFn != nil
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.Fallback {
			continue
		}
		n := s.TotalArgs()
		pos, _ := checkModeFallbackPositionsFor(e, s, w)
		if len(pos) != n {
			continue
		}
		score := 0
		compatible := true
		for j, p := range pos {
			av := e.Tape.At(p)
			if av.Parent().Equal(core.TAny) {
				continue
			}
			if core.SigArgMatches(s, j, &av) {
				score++
				continue
			}
			compatible = false
			break
		}
		if !compatible {
			continue
		}
		hasFn := s.ReturnsFn != nil
		if score > bestMatch || (score == bestMatch && hasFn && !bestHasFn) {
			bestMatch = score
			best = s
			bestHasFn = hasFn
		}
	}
	// Fallback pass: if no compatible sig was found at all, prefer a
	// sig with a ReturnsFn over one without (all else equal), at a
	// SATISFIABLE arity. The fallback (first-ranked candidate) may be
	// a wider overload than this call site can even supply positions
	// for — specificity ranking puts the 3-arg patrun `add` above the
	// 2-arg math adds — and a fallback that merely CARRIES a ReturnsFn
	// must not win on that alone: assuming the wider sig swallows an
	// unrelated operand into the recovery window (corrupting the
	// disjunct rescue for `add` over two disjuncts) or feeds its
	// ReturnsFn a short args slice (an index-out-of-range panic
	// class). So when the current best is Fn-less OR arity-
	// unsatisfiable, move to the first ReturnsFn-bearing sig whose
	// full window EXISTS; if none does, the fallback stands (its
	// ReturnsFn sees the short window — ReturnsFns are len-guarded).
	if bestMatch < 0 {
		fbn := best.TotalArgs()
		fbPos, _ := checkModeFallbackPositionsFor(e, best, w)
		bestSat := len(fbPos) == fbn
		if !bestHasFn || !bestSat {
			for i := range fn.Signatures {
				s := &fn.Signatures[i]
				if s.Fallback || s.ReturnsFn == nil {
					continue
				}
				n := s.TotalArgs()
				if sp, _ := checkModeFallbackPositionsFor(e, s, w); len(sp) != n {
					continue
				}
				best = s
				break
			}
		}
	}
	sig := best
	// positions are in TAPE order — the stack run (nStack of them, ascending
	// toward the pointer) then the forward run in source order — which is
	// NOT signature order; nStack is kept for the operand rebuild
	// (SigOrderArgs). The forward-eligible leading positions were filled
	// from the written tokens first (checkModeFallbackPositionsFor).
	positions, nStack := checkModeFallbackPositionsFor(e, sig, w)
	args := recoveryArgsAt(e, positions)
	// Strict disjunct rescue (design/legacy/checker-accuracy-review.10.ignore A1):
	// the whole disjunct matched no signature, but individual
	// alternatives may dispatch fine. If at least one does, splice the
	// per-alternative join — the failing alternatives have already
	// been flagged with partial_dispatch warnings, which name the
	// exact path that would fail; the blanket no_signature error
	// would be wrong for the paths that DO dispatch.
	if out, ok := disjunctPartitionReturns(e.Registry, w.Name, args, pos); ok {
		// A single-overload user fn over a disjunct-typed operand recovers here
		// (e.g. the boru:test framework's run-cases inside test-describe's
		// body): its sole signature's ReturnsFn models the call over the
		// window and splices its own returns (carrier_ljoin_test.go drives
		// the recovery arm).
		if recoverUserFn(e, w, pos, sig, fn, args, nStack, positions) {
			return nil
		}
		spliceCheckResults(e, positions, out)
		return nil
	}
	// Emit the error-severity no_signature diagnostic — the genuine static
	// report of an unmatched dispatch. Do NOT gate it on
	// `bestMatch >= 0`: this fall-through is reached only when matchSignature
	// already FAILED to commit, so a positive best-fit score here is a
	// best-effort guess (a bare type-literal or wildcard operand that
	// sigArgMatches accepts but the runtime rejects — `class Map`, `get 'a'
	// Map`, `add/3 2 3`, `fold [add] {}` …). Suppressing the diagnostic when a
	// guess exists is UNSOUND: it dropped 16 genuine error rows the interpreter
	// raises on (TestCheckAccuracyRatchet coverage 208→192). A positive best-fit
	// here does NOT imply the dispatch matches at runtime — matchSignature
	// already failed; the recovery's `bestMatch` is a best-effort score that can
	// "fit" a bare type literal (`class Map`), a wrong-typed value, or a
	// dynamic operand whose runtime value still misses the sole sig (the
	// recursive `f` whose `next` holds a List where the sig wants an Integer —
	// which `signature_error`s at run time, NOT a false positive). The
	// pass gate alone is the correct condition.
	// EXCEPTION to the "pass gate alone" rule: a SINGLE-overload user fn
	// dispatched over an Any/disjunct-CARRIER arg (a value of statically-unknown
	// type, not a concrete mismatch) is NOT a genuine unmatched dispatch
	// (singleOverloadRecoverable): the runtime value either matches the sole
	// sig or raises there. Flagging it would be the `engine-known engine`
	// false positive (`is String`-guarded Options-`get`). Suppressing is
	// SOUND and narrower than the rejected bestMatch>=0 gate: it fires only on an
	// unknown-TYPE carrier (never a concrete/type-literal operand) AND a one-
	// overload user fn, so it cannot drop the concrete-mismatch error rows.
	// anyAnyCarrier ONLY, NOT anyDisjunctCarrier: a DISJUNCT's members are
	// statically known, so a disjunct that misses the param in EVERY member
	// (Integer|String -> Map, String|List -> Integer) is a GENUINE error the
	// interpreter always raises on and must stay flagged (TestJoinCarriersDynamicArm
	// / TestSliceDynamicReceiverRefines). Only the fully-unknown Any carrier is
	// deferrable to the runtime CALL_USER contract.
	recoverableUnknownType := AnyAnyCarrier(args) && core.SingleOverloadRecoverable(sig, fn) && core.ConcreteArgsMatch(sig, args, nStack)
	if !recoverableUnknownType {
		// Expected-vs-actual: name the operand types the dispatch saw and
		// the nearest candidate's declared types, so the user can see the
		// mismatch without reconstructing the stack ("got (Map, Integer);
		// nearest [Number Number]").
		detail := core.NoMatchDetail(w.Name)
		if got := core.ArgTypeSummary(args); got != "" {
			detail += "; got (" + got + ")"
			if near := core.SigTypeSummary(sig); near != "" {
				detail += "; nearest [" + near + "]"
			}
		}
		e.Registry.Check.AddDiagnostic(core.CheckDiagnostic{
			Code:   "no_signature",
			Detail: detail + "; assuming best-fit candidate for analysis",
			Word:   w.Name,
			Row:    pos.Row,
			Col:    pos.Col,
		})
	}
	// The assumed dispatch runs its ReturnsFn against args the REAL
	// match already rejected — a user fn's body analysis under those args
	// produces CASCADE noise (an unbound param surfacing as a spurious
	// `undefined_word: x` from inside the body, a dependent no_signature on
	// a body word). The one honest diagnostic is the no_signature above;
	// suppress the error-level body diagnostics of the consequent analysis
	// (the SuppressBodyErrors discipline recursive re-entry already uses).
	e.Registry.Check.SuppressBodyErrors++
	results := CarrierResults(e.Registry, w.Name, sig, args, pos, nil, false)
	e.Registry.Check.SuppressBodyErrors--
	spliceCheckResults(e, positions, results)
	return nil
}

// spliceCheckResults removes the word at the pointer plus the consumed
// candidate positions and splices the synthesised carrier results in
// at the word's slot — the shared tail of the check-mode recovery
// paths (assume-sig and the S2 surface-shape typing).
func spliceCheckResults(e *core.Engine, positions []int, results []core.Value) {
	indices := append([]int{e.Pointer}, positions...)
	// Insertion sort (small n).
	for i := 1; i < len(indices); i++ {
		for j := i; j > 0 && indices[j] < indices[j-1]; j-- {
			indices[j], indices[j-1] = indices[j-1], indices[j]
		}
	}
	// Deduplicate (defensive).
	uniq := indices[:0]
	prev := -1
	for _, idx := range indices {
		if idx != prev {
			uniq = append(uniq, idx)
			prev = idx
		}
	}
	// Remove from highest to lowest to avoid shifting.
	insertAt := e.Pointer
	for i := len(uniq) - 1; i >= 0; i-- {
		if uniq[i] < insertAt {
			insertAt--
		}
		e.Tape.Remove(uniq[i])
	}
	e.Tape.Splice(insertAt, 0, results...)
	e.Pointer = insertAt
}

// installCheckBraid wires the braid into core's S9 slot table; one
// registration point so the cut moves it wholesale into the check
// package's init.
func installCheckBraid() {
	core.CheckBraid.CheckMixedFormAdvisories = checkMixedFormAdvisories
	core.CheckBraid.CheckModeAssumeSig = checkModeAssumeSig
	core.CheckBraid.CheckModeFallbackPositions = checkModeFallbackPositions
	core.CheckBraid.CheckModeParenFnCollapse = checkModeParenFnCollapse
	core.CheckBraid.CheckModeSurfaceShape = checkModeSurfaceShape
	core.CheckBraid.ConcreteEvalOnce = concreteEvalOnce
	core.CheckBraid.DrainUndefinedAtoms = drainUndefinedAtoms
	core.CheckBraid.ExprRefsCarrier = exprRefsCarrier
	core.CheckBraid.NoteSpeculativeBarrierCommit = noteSpeculativeBarrierCommit
	core.CheckBraid.ShareCheckState = shareCheckState
	core.CheckBraid.ShareCheckStateFrom = shareCheckStateFrom
	core.CheckBraid.SpliceAnonCheckResult = spliceAnonCheckResult
	core.CheckBraid.SpliceCheckResults = spliceCheckResults
	core.CheckBraid.SpliceFnValueCheckResult = SpliceFnValueCheckResult
	core.CheckBraid.TagCheckModeDefRead = tagCheckModeDefRead
	core.CheckBraid.TryDynamicFnValueDispatch = tryDynamicFnValueDispatch
	core.CheckBraid.NoteStrandedTypeCall = noteStrandedTypeCall
	core.CheckBraid.TryShapedMethodDispatch = TryShapedMethodDispatch
	core.CheckBraid.UndefinedWordCheckDiag = undefinedWordCheckDiag
}

func init() { installCheckBraid() }

// installAnalysisImpl wires the check piece's analysis operations into
// core's S1 implementation table; one registration point so the cut
// moves it wholesale into the check package's init.
func installAnalysisImpl() {
	core.AnalysisImpl.FnConstructionPass = checkFnBodyAtConstruction
	core.AnalysisImpl.ReturnsFn = BuildFnBodyReturnsFn
	core.AnalysisImpl.StripToCarriers = StripToCarriers
	core.AnalysisImpl.CarrierResults = CarrierResults
	core.AnalysisImpl.MixedConform = carrierMixedConform
	core.AnalysisImpl.ValueCarriesCarrier = valueCarriesCarrier
	core.AnalysisImpl.AtUncaughtTopLevel = CheckAtUncaughtTopLevel
	core.AnalysisImpl.AnalyseFnBody = AnalyseFnBody
	core.AnalysisImpl.AnalyseLoopBody = AnalyseLoopBody
}

func init() { installAnalysisImpl() }

// noteStrandedTypeCall reports §5.1's silent wrong answer: a capitalised
// name bound to a FUNCTION body is a TYPE, so writing it in call position
// never calls. `def I fnpred x:Integer [add 1 x] end I 5` prints `I 5` and
// exits 0 — the minted lattice node is placed, the 5 is never consumed,
// and nothing anywhere says so. The combinator literature is all capitals
// (S, K, I, B, C, W, Y), so a reader transcribing it lands here first
// (design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore §5.1, recommendation 2).
// Since NUR099 the undeclared spelling (`def I x:Integer => [add 1 x]`, a
// plain `fn` body under a capitalised name) is refused at the declaration
// with def_error, so the one fn-bodied type node left to strand is a
// DECLARED predicate written as a call.
//
// The gate is deliberately narrow, because this is a hint and a false one
// costs more than a missed one. It fires on a bare lattice node whose
// DECLARED CONTENT is a function value, IMMEDIATELY followed by a
// non-type value — the forward-form call `I 5` exactly. It stays quiet on:
//
//   - a node stranded LAST (`4 is Even  Even`, `xs E`): a statement that
//     merely names a type after an earlier one produced a value is legal;
//   - a node whose content is not a fn — a `fnsig` type, a `class`, a plain
//     alias (`def Foo Integer  Foo 5`), a builtin (`Integer 5`): naming a
//     type beside a value is ordinary in `is` / `typeof` code;
//   - two adjacent nodes (`Even Odd`): nothing was being called.
//
// It judges the TOP-LEVEL residual only. That is not as narrow as it
// sounds: a paren group runs on the same tape, and an `if` arm, a `for`
// body, a `do` region and a fn body all surface their own residual into it,
// so the stranded pair is caught wherever it can still reach the program's
// result. What it excludes is the pair landing inside a container VALUE
// (`[I 5]`, `{a:I b:5}`), evaluated by a sub-engine — boru carries types as
// data, so a node beside a value in a list or a map is not a call that
// failed to happen.
//
// KNOWN RECALL LIMIT — a residual scan cannot see a pair something else
// already ate. `I 5 drop`, `I 5 print`, `def r (I 5)` and `size (I 5)` are
// all the same defect and all go unreported: the later word takes the
// operand, or the enclosing call takes the node, and by end-of-run the
// adjacency is gone. Closing it means recording the candidate where the
// node is PLACED (the step site knows the following SOURCE token, which is
// the fact that actually decides it) rather than reading the finished
// stack — a core-seam change, deliberately not folded into the commit that
// introduced this. Pinned as a fact in
// lang/go/test/stranded_type_call_test.go's MissesConsumedPair, and written
// up under §5.1 "What it still misses" so the limit is a known one.
func noteStrandedTypeCall(e *core.Engine, residual []core.Value) {
	if !e.IsTop || !e.Registry.Check.IsActive() {
		return
	}
	for i := 0; i+1 < len(residual); i++ {
		v := residual[i]
		if !fnBodiedTypeNode(v) || core.IsBareTypeNode(&residual[i+1]) {
			continue
		}
		// Prefer the SOURCE token over the node's own name: they agree for
		// `I 5`, but an alias (`def J I  J 5`) writes J where the node is
		// still called I, and the caret points at what was written. A
		// computed placement — `(valof I) 5` — carries no position at all,
		// and there the node's name is the only thing left to say.
		name := v.Pos().Src
		if name == "" {
			name = v.String()
		}
		// CheckAddUnique, not AddDiagnostic: one SOURCE defect must cost one
		// diagnostic. A fn body is analysed once per call shape, so a body
		// that strands the pair (`def g y:Integer => [I y]  g 1  g 2`)
		// surfaces the same tokens into the residual once per analysed call
		// — identical code, detail and position every time.
		core.CheckAddUnique(e.Registry, core.CheckDiagnostic{
			Code: "stranded_type_call",
			Detail: "'" + name + "' names a type, not a function: a capitalised def binds a TYPE, " +
				"so the call never ran and its operands stayed on the stack",
			Word: name,
			Row:  v.Pos().Row,
			Col:  v.Pos().Col,
			Src:  v.Pos().Src,
			Notes: []string{
				"a capitalised def binds a TYPE, and a fnpred body is that type's membership test " +
					"(`4 is " + name + "` is the intended use), never a function to call",
			},
			// No Replacement: the fix is a COORDINATED rename — the
			// declaration and every reference — and this diagnostic points at
			// one use site. A single-token replacement applied there would
			// leave `def ` + name + ` …` standing and turn the call into an
			// undefined_word, which is worse than no code action at all.
			Suggestions: []core.DiagSuggestion{{
				Message: "a capitalised def can only ever bind a type: rename it and its uses " +
					"together (`def " + strings.ToLower(name) + " …` … `" + strings.ToLower(name) + " 5`)",
			}},
		})
	}
}

// fnBodiedTypeNode reports whether v is a bare lattice node whose declared
// content is a FUNCTION value — the node `def <Capitalised> <fn body>`
// mints, whether that lands as a predicate type (a 1-arg Boolean body) or
// a plainly Function-parented one (every other arity). Keying on the
// CONTENT rather than on predicate-ness is what makes the multi-argument
// combinators — `def K fn [[a:Any b:Any][Any][a]] end` — visible too.
func fnBodiedTypeNode(v core.Value) bool {
	if !core.IsBareTypeNode(&v) {
		return false
	}
	content, ok := core.TypeContentOf(v)
	return ok && core.IsFnValueResidual(content)
}

// recoveryArgsAt resolves the operand values at the recovery's tape
// positions: a raw Word forward operand resolves to its def binding (the
// tape still holds Words at this point, and the partition probe and the
// assumed sig's ReturnsFn want values), and a raw eval-map operand is
// auto-evaluated exactly as the runtime match's execMatch would (word
// members resolve against the live frame, the recorder assembles a per-run
// OpMakeMap) — the recovery otherwise hands the RAW source map to the poly
// record, which either baked a live word member as a frozen const (the
// repl-eval `{line: src}` request map) or declines. Errors leave the raw
// operand, so the assumed-sig model stays as before.
func recoveryArgsAt(e *core.Engine, positions []int) []core.Value {
	args := make([]core.Value, len(positions))
	for i, p := range positions {
		av := e.Tape.At(p)
		if core.IsWord(&av) {
			if wi, werr := core.AsWord(&av); werr == nil {
				if top, ok := e.Registry.Defs.Top(wi.Name); ok {
					av = top
					e.Registry.Check.RecordUse(wi.Name)
				}
			}
		}
		if core.IsConcrete(&av) && av.Parent() != nil && av.Parent().ConformsTo(core.TMap) && core.BearsActiveTokens(av) {
			if ev, everr := e.AutoEvalMap(av, false, true); everr == nil {
				// The interpreter's failed match never evaluates the
				// literal: its report renders it as written, so a trap built
				// over this tape must too (NUR235).
				e.NoteRecoveryRaw(p, e.Tape.At(p))
				e.Tape.Set(p, ev)
				av = ev
			}
		}
		args[i] = av
	}
	return args
}

// widestSatisfiableOverload picks, among a word's non-fallback overloads,
// the one of GREATEST arity whose full operand window exists at this call
// site (checkModeFallbackPositionsFor supplies every position), first in
// match order among equals; nil when none is satisfiable. The dyn-body
// recovery re-matches the live values over this window at run time, and
// the interpreter's first match consumes the widest window it can — so a
// narrower window would leave an operand behind.
func widestSatisfiableOverload(e *core.Engine, fn *core.FnDefInfo, w core.WordInfo) (*core.Signature, []int, int) {
	var best *core.Signature
	var bestPos []int
	bestN := 0
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.Fallback {
			continue
		}
		n := s.TotalArgs()
		sp, nStack := checkModeFallbackPositionsFor(e, s, w)
		if len(sp) != n {
			continue
		}
		if best == nil || n > best.TotalArgs() {
			best, bestPos, bestN = s, sp, nStack
		}
	}
	return best, bestPos, bestN
}

// soleSigParamsNominal reports whether fn is a single-overload user fn
// (core.SingleOverloadRecoverable) whose sole signature's param types are all
// NOMINAL — no type carrying a value-sensitive constraint the guarded
// CALL_USER's nominal entry check could not enforce.
func soleSigParamsNominal(sig *core.Signature, fn *core.FnDefInfo) bool {
	if !core.SingleOverloadRecoverable(sig, fn) || fn.Gen != nil {
		// A GENERIC fn (`def unbox gen [T] fn [[b:T] …]`) has no nominal
		// param to guard — T is bound per call by the generic lane, whose
		// evaluating host is not built (generics-fn.tsv L54 compiled through
		// this recovery and ran the call on the interpreter, an interp-entry
		// census row); it keeps the decline.
		return false
	}
	// HasConstraintUnify is nil-safe (an absent type carries no constraint),
	// so every position is asked directly.
	for _, t := range sig.ArgTypes() {
		if core.HasConstraintUnify(t) {
			return false
		}
	}
	return true
}

// recoverUserFn is the single-overload user-fn recovery
// (Engine.TryRecordRecoveredUserFn) with the dispatching word's cursor
// published again right before it: the recovery's earlier probes (the poly
// re-match's CarrierResults) analyse the callee's body, whose own dispatches
// move the cursor, and the recovered call's ReturnsFn reads it as the call
// word and the CALL_USER's position — the caret of a no-match its entry
// check raises (`k m.a` over an Integer inside g anchored at `keys` in k's
// body, NUR360).
func recoverUserFn(e *core.Engine, w core.WordInfo, pos core.SrcPos, sig *core.Signature, fn *core.FnDefInfo, args []core.Value, nStack int, positions []int) bool {
	// A window whose written operands reach past the signature's barrier is
	// the recovery's shortfall fill, not the interpreter's collection, which
	// stops at the barrier and finds the stack short (NUR362: `g (h) 7` over
	// `g [a:Integer | b:Integer]` raises): the call is not modelled.
	if core.BarrierBars([]core.Signature{*sig}, len(args)-nStack) {
		return false
	}
	e.Registry.Check.CurCallWord, e.Registry.Check.CurCallPos = w.Name, pos
	return e.TryRecordRecoveredUserFn(sig, fn, args, nStack, positions)
}
