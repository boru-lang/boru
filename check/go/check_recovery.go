package check

// The check-mode recovery and advisory surface of the step loop —
// dispatch-failure modeling (surface shapes, assumed signatures,
// fallback positions), check-result splicing, mixed-form advisories,
// stranded-operand refusals, and the check-state sharing brackets.
// Extracted from engine.go in Stage 0c of the four-piece split
// (design/ENG-FOUR-PIECE.0.md): this file is the CHECK piece's half of
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
// so this is a no-op). Each carrier remembers which NAME the atom carried:
// an analysis-time-undefined read inside a fn body can still be a
// DYNAMIC-SCOPE reference bound by a live frame at run time (a recursive
// base case reading the previous frame's body-local — recursion.tsv:71);
// resolveOperand's dynScopeRescue lowers it to a runtime lookup iff a
// binder fn reaches the reader.
func drainUndefinedAtoms(e *core.Engine) {
	for i := 0; i < e.Tape.Len(); i++ {
		und := e.Tape.At(i)
		if !und.Undefined || !e.Registry.Check.IsActive() {
			continue
		}
		c := core.NewCarrier(core.TAny)
		if a, aerr := core.AsAtom(und); aerr == nil && a != "" {
			e.Registry.Check.Recorder().NoteDefRead(c.ID, a)
		}
		e.Tape.Set(i, c)
	}
}

// tagCheckModeDefRead records provenance on a def-word substitution so a later
// compile pass can lower an un-ID-able read to a runtime dyn-scope lookup. Two
// cases tag DynFrom (consumed ONLY by resolveOperand's dynScopeRescue; narrowing
// is Dynamic-gated, so a non-dynamic tag is inert there):
//
//   - a DYNAMIC (gradual) value: the tag lets a typed downstream use narrow the
//     binding (narrowing-through-use);
//   - a CONCRETE MODULE-SCOPE mutable reference (a `flex` map/list/xml, a Store
//     bound outside the current fn frame): its reads lower to a live dyn-scope
//     lookup so every reference re-resolves the ONE shared instance. A DETACHED
//     stamp forks the RUNTIME def table where the value's ID was ELIDED, so
//     defReads[ID] cannot name it — the tag makes the read nameable regardless of
//     ID. Scoped to MODULE scope (ModuleScopeBinding): such a binding lives in the
//     registry Defs for the whole run, so OpLookupDynScope resolves it byte-
//     identically to the interpreter. A BODY-LOCAL flex is a frame local, not a
//     registry binding, so dyn-scoping it would miss — it keeps its local-slot
//     lowering (or a sound refusal), left untagged.
//   - a MODULE-FAMILY value bound at module scope — an `import`-bound namespace
//     (`IO`, `StringUtil`) or a Module descriptor (`def m StringUtil.$module`):
//     the same shape as the mutable reference, read as a VALUE (`IO deq IO`, a
//     residual, an `eq` operand). A namespace is a pointer-shared map the
//     const gate deliberately refuses (fn exports; ConstBakeable is closed to
//     module instances), and its identity IS the binding's — so the read
//     lowers to the same live lookup, which is also what honours a re-import.
//
// Extracted from stepWord so the hot dispatch path stays under the cyclomatic-
// complexity gate.
func tagCheckModeDefRead(e *core.Engine, top *core.Value, name string) {
	switch {
	case top.Dynamic:
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
		if core.IsOpenParen(v) || core.IsCloseParen(v) || core.IsForward(v) || core.IsDefCleanup(v) ||
			v.Parent.ConformsTo(core.TMark) || v.Parent.ConformsTo(core.TMove) ||
			v.Parent.ConformsTo(core.TInternal) || v.Parent.ConformsTo(core.TReturnCheck) {
			return // scope boundary: nothing stranded in this scope
		}
		if !core.IsConcrete(v) && !core.IsBareTypeNode(v) {
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

// RefuseForwardStackDrift refuses (compile mode only) a dispatch whose
// check-mode operand match would DIVERGE from the interpreter's runtime
// forward collection — the reified-error / island residual accounting of
// design/EDGE-SPEC-FINDINGS.0.md §1. Preconditions (checked by the caller):
// the dispatch matched ALL-STACK (fwdCount==0). It refuses when ALL of:
//
//   - the recorder is active (a real compile pass — never plain check / run);
//   - the sig is forward-eligible (BarrierPos != 0, not a full-stack word), so
//     the trailing token is a forward CANDIDATE, not a separate residual;
//   - the TOP-OF-STACK matched arg (sig[0], the highest-position operand) is
//     DYNAMIC. That is the operand whose unknown static type BLOCKED the
//     narrower forward overload — forcing check mode to a catch-all all-stack
//     match — while at run time its concrete value would let the word
//     forward-collect instead. If a concrete value is on top, the all-stack
//     match is what the interpreter takes too (`get key dyn`, `force-arity 2
//     dynfn`), so there is no drift;
//   - a DEEPER matched arg is NON-dynamic — the leading residual the all-stack
//     match reached PAST (the `5` of `5 do … error … add 1`). Its runtime
//     value stays put under the word's real result;
//   - the token immediately after the word is a single ATOMIC LITERAL forward
//     operand (a concrete scalar/atom or a bare type node) — the operand the
//     interpreter forward-collects once the dynamic value is concrete. A
//     CloseParen / ParenExpr / structural token is NOT a forward operand (the
//     `eq`/`and` comparison residuals that end on `)` compile faithfully
//     all-stack), so those are excluded.
//
// Without the top-is-dynamic and trailing-token gates a genuine all-stack
// dynamic dispatch (`get key dyn`, `dyn 5 add`) would be refused although it
// compiles faithfully, so both gates are load-bearing.
func RefuseForwardStackDrift(e *core.Engine, sig *core.Signature, positions []int) {
	es := e.Registry.Check.Recorder()
	if !es.Active() || sig == nil || sig.BarrierPos == 0 || sig.FullStack() || len(positions) < 2 {
		return
	}
	// A word with NoEvalArgs (code-body / quoted) positions — `if`, `for`, the
	// higher-order words — forward-collects THOSE body/quote tokens, never a
	// trailing SCALAR after them: the drift this guard models is the `add`-style
	// re-collection of a bare scalar operand (`island add 1`), where a concrete
	// top would pull the next literal into a value slot. For `if ok [t] [e] 0`
	// the check-mode all-stack match is an ANALYSIS-ORDER artifact — the recorded
	// branch event still binds the correct operands (cond=ok, the two arm bodies)
	// and the trailing `0` is a separate statement, so compiled == interpreter.
	// Firing here is a false positive that refuses a faithfully-compilable `if`.
	if len(sig.NoEvalArgs) > 0 {
		return
	}
	// Find the top-of-stack matched arg (highest tape position) and whether any
	// deeper matched arg is non-dynamic.
	topPos, deeperConcrete := -1, false
	for _, p := range positions {
		if p < 0 || p >= e.Tape.Len() {
			return
		}
		if p > topPos {
			topPos = p
		}
	}
	for _, p := range positions {
		if p != topPos && !e.Tape.At(p).Dynamic {
			deeperConcrete = true
		}
	}
	if !e.Tape.At(topPos).Dynamic || !deeperConcrete {
		return
	}
	nxt := e.Pointer + 1
	if nxt >= e.Tape.Len() {
		return
	}
	if core.ForwardLiteralOperand(e.Tape.At(nxt)) {
		es.MarkUncompilable("forward operand accounting across a dynamic/island residual (Stage 3)")
	}
}

// refuseStrandedMemberFn refuses (compile mode only) a dispatch that consumes a
// stack operand while a parked FUNCTION VALUE sits directly beneath it — the
// mid-expression member-fn-apply divergence of design/EDGE-SPEC-FINDINGS.0.md
// §2. The interpreter auto-applies a surfaced member fn (`m.double`) to the
// value that lands on it, so `m.double 21 eq 42` runs `(m.double 21)` → 42
// BEFORE `eq`; the compiler instead lets `eq` consume `21` and applies the
// stranded fn at the residual tail (to the wrong value). The bare statement-tail
// apply `m.double 21` never reaches here — nothing dispatches above the fn — so
// it keeps compiling. No-op outside a compile pass (recorder inactive).
func refuseStrandedMemberFn(e *core.Engine, positions []int) {
	es := e.Registry.Check.Recorder()
	if !es.Active() {
		return
	}
	// Deepest stack operand this dispatch consumes.
	minStack := -1
	for _, p := range positions {
		if p >= 0 && p < e.Pointer && (minStack == -1 || p < minStack) {
			minStack = p
		}
	}
	if minStack <= 0 {
		return
	}
	// Walk to the FIRST data value directly below the consumed operands (skipping
	// pure structural markers), stopping at a scope / statement boundary. Only an
	// IMMEDIATELY-adjacent parked fn is the stolen-arg hazard: a fn buried under
	// other residual values (a previous statement's leftover) is not stranded
	// against THIS dispatch's operand, so the scan stops at the first value
	// regardless of what it is.
	for i := minStack - 1; i >= 0; i-- {
		v := e.Tape.At(i)
		if core.IsOpenParen(v) || core.IsCloseParen(v) || core.IsForward(v) || core.IsEnd(v) || core.IsDefCleanup(v) {
			return // scope / statement boundary
		}
		if v.Parent.ConformsTo(core.TMark) || v.Parent.ConformsTo(core.TMove) ||
			v.Parent.ConformsTo(core.TInternal) || v.Parent.ConformsTo(core.TReturnCheck) {
			continue // pure structural marker between the operand and its neighbour
		}
		// First data value directly below the operand. Only a CONTAINER MEMBER
		// read is the hazard this guard owns (design/EDGE-SPEC-FINDINGS.0.md §2):
		// its result is a checker-typed dynamic(Any) whose PROVENANCE (memberFnRead)
		// marks it as a fn-valued member surfaced by a get-family read. A bare
		// Function value here (a `c/v` param ref, a factory closure) is a
		// DIFFERENT boundary (M2a `apply`, the residual leading/trailing apply) with
		// its own handling — do NOT claim it, or those refuse with the wrong reason.
		if es.MemberFnRead(v.ID) {
			es.MarkUncompilable("member fn value auto-applies mid-expression (fn-value-call boundary, Stage 3)")
		}
		return
	}
}

// valueCarriesCarrier reports whether v is a check-mode carrier or a
// container holding one at any depth — the shape a hole's auto-eval
// leaves when a list/map literal embeds a computed element. Used by the
// interpolation bake probe above; at run time there are no carriers, so
// the walk is check-mode-only work.
func valueCarriesCarrier(v core.Value) bool {
	if v.Carrier {
		return true
	}
	switch d := v.Data.(type) {
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
			if core.IsWord(v) {
				if w, err := core.AsWord(v); err == nil {
					if bound, ok := r.Defs.Top(w.Name); ok && bound.Carrier {
						found = true
						return
					}
				}
			}
			if core.IsParenExpr(v) {
				if toks, err := core.AsParenExpr(v); err == nil {
					walk(toks)
				}
				continue
			}
			// A Reach can hide a carrier in its receiver or a computed key
			// (`m.(k)` with a def-local carrier `k`): recurse into both so the
			// fold declines rather than baking the carrier's check-time value.
			if core.IsReach(v) {
				if ri, err := core.AsReach(v); err == nil {
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
				walk(lst.Slice())
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
// the result is a real value, not a carrier, and nothing is recorded into the
// parent's emit state) and returns the single concrete residual. The def stack
// is snapshotted and restored so a stray binding cannot leak into the compile.
func concreteEvalOnce(e *core.Engine, items []core.Value) (core.Value, bool) {
	r := e.Registry
	snap := r.Defs.Snapshot()
	prev := r.Check.Mode
	r.Check.Mode = false
	// C4 attribution: this concrete sub-run IS the check pass (the const
	// fold needs a real value, so Mode is off for its duration) — without
	// the explicit tag its interpreter entries would report unattributed.
	restoreAtt := r.SetInterpAttribution("check:const-fold")
	res, err := core.RunPooledSub(r, append([]core.Value(nil), items...), false)
	restoreAtt()
	r.Check.Mode = prev
	r.Defs.Restore(snap)
	if err != nil || len(res) != 1 || !core.IsConcrete(res[0]) {
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
	if len(result) == 0 {
		result = []core.Value{core.NewCarrier(core.TAny)}
	}
	spliceFnCheckTail(e, valIdx, nArgs, result)
	return nil
}

// SpliceFnValueCheckResult is the check-mode dispatch for a NON-anonymous
// body-bearing fn VALUE (a `fn` literal resolved from a map/module export and
// then CALLED, e.g. `ParseLang.parse_json 'x' {}`). Unlike a NAMED user fn
// (which dispatches through stepWord → its registered ReturnsFn) and unlike an
// anonymous lambda (spliceAnonCheckResult, analysis-only), a called fn value
// previously fell through to execFnDefSig, whose inline body splice leaks the
// per-call `__pa` (Args/FnBaseline pop) token into the TOP-LEVEL residual —
// refused by the emitter as "context-dependent word __pa". Routing through
// BuildFnBodyReturnsFn ARMS the body analysis via StartFnCompile, so the body
// (with its `__pa` tail) is captured INSIDE its own CALL_USER unit and the
// call site records a CALL_USER — identical to the named-fn path. See
// design/boru-bytecode-stage3-inlining-plan.0.md "THE shared crux:
// body-bearing fn-VALUE dispatch (__pa)".
func SpliceFnValueCheckResult(e *core.Engine, valIdx, nArgs int, fnDef core.FnDefInfo, sig *core.FnSig, args []core.Value) error {
	returns := BuildFnBodyReturnsFn(e.Registry, fnDef.Name, *sig, fnDef)
	result := returns(args, e.Registry)
	if len(result) == 0 && len(sig.Returns) > 0 { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		// A declared-return fn that produced no carrier (the body unit
		// declined to compile) degrades to one carrier per declared return so
		// downstream provenance refuses and the program falls back faithfully.
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
// design/module-fn-checkstate-ownership.1.md §5b.
func shareCheckState(e *core.Engine, capturedReg *core.Registry) func() {
	return shareCheckStateFrom(capturedReg, e.Registry)
}

// shareCheckStateFrom is the registry-level mechanism behind shareCheckState:
// it points owner's Check at caller's for the duration (restore via the
// returned func), no-op when the registries coincide, either is nil, or the
// caller is not in check mode. Split out so the MERGED-WORD seam can share at
// the ReturnsFn boundary itself (BuildFnBodyReturnsFn — Stage M1,
// design/STAGE3-INLINING-DESIGN-ROUND.0.md §5): a transplanted word-extension
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
		if bw, err := core.AsWord(e.Tape.At(e.Pointer)); err == nil {
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
// `boru run` refuse a program both engines run correctly. A dynamic
// argument makes the RESULT no less knowable than a static one — the
// window collapses to dynamic(Any) either way — so the restriction bought
// no soundness. `IsFnValueResidual` still excludes a trailing fn VALUE
// (a curried chain, where the window is not one call).
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
		if count == 0 && !v.Dynamic && !v.Quoted && core.IsFnTypedCarrier(v) {
			leadIdx = i
		}
		count++
		lastIdx = i
	}
	if count < 2 {
		return closeIdx
	}
	last := e.Tape.At(lastIdx)
	trailing := !last.Dynamic && !last.Quoted && core.IsFnTypedCarrier(last)
	leading := leadIdx >= 0 && count == 2 && !core.IsFnValueResidual(last)
	if !trailing && !leading {
		return closeIdx
	}
	anchor := lastIdx
	if leading {
		anchor = leadIdx
	}
	out := core.NewCarrier(core.TAny)
	out.Dynamic = true
	out.ID = core.GenerateID(core.IDPrefixForType(core.TAny))
	out = core.WithPos(out, e.Tape.At(anchor))
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
		if core.IsCloseParen(v) {
			if depth == 0 {
				break
			}
			depth--
			continue
		}
		if core.IsOpenParen(v) {
			depth++
			continue
		}
		if core.IsForward(v) || core.IsMark(v) || core.IsMove(v) ||
			core.IsReturnCheck(v) || core.IsDefCleanup(v) {
			continue
		}
		positions = append(positions, i)
		remaining--
	}
	return positions
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
		// AnalyseFnBody, design/GENERICS.10.md Phase 5) is visible to
		// the S2 scan.
		if core.IsWord(v) {
			if wv, werr := core.AsWord(v); werr == nil {
				if top, ok := e.Registry.Defs.Top(wv.Name); ok {
					v = top
				}
			}
		}
		if v.Parent == nil {
			continue
		}
		info, ok := core.SurfaceInfoOf(v.Parent)
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
	undef, ok := shape.Data.(core.FnUndefInfo)
	if !ok || len(undef.Sigs) == 0 {
		return false, nil
	}
	spec := core.SubstituteSelf(undef.Sigs[0], sinfo.Type)
	e.Registry.Check.Recorder().MarkUncompilable("surface-shape typed dispatch at " + w.Name)
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

func checkModeAssumeSig(e *core.Engine, w core.WordInfo, fn *core.FnDefInfo, fallback *core.Signature, pos core.SrcPos) error {
	// Publish the dispatching word token for every ReturnsFn this recovery
	// can invoke — the disjunct-partition combos, TryRecordRecoveredUserFn,
	// the poly arms. The matched path publishes it in declaredReturnCarriers;
	// this is the unmatched twin, reached with pos = val.Pos(), the word's
	// own position. Without it a recovered user call's ReturnsFn read the
	// PREVIOUS dispatch's cursor and keyed its region claim by that.
	e.Registry.Check.CurCallWord, e.Registry.Check.CurCallPos = w.Name, pos
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
		pos := checkModeFallbackPositions(e, n)
		if len(pos) != n {
			continue
		}
		score := 0
		compatible := true
		for j, p := range pos {
			av := e.Tape.At(p)
			if av.Parent.Equal(core.TAny) {
				continue
			}
			if core.SigArgMatches(s, j, av) {
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
		bestSat := len(checkModeFallbackPositions(e, fbn)) == fbn
		if !bestHasFn || !bestSat {
			for i := range fn.Signatures {
				s := &fn.Signatures[i]
				if s.Fallback || s.ReturnsFn == nil {
					continue
				}
				n := s.TotalArgs()
				if len(checkModeFallbackPositions(e, n)) != n {
					continue
				}
				best = s
				break
			}
		}
	}
	sig := best
	n := sig.TotalArgs()
	positions := checkModeFallbackPositions(e, n)
	// nStack is how many of the gathered positions are STACK args (before
	// the pointer, ascending); the remainder are FORWARD args (after the
	// pointer, source order). checkModeFallbackPositions lays them out in
	// that tape order — stack-before then forward-after — which is NOT
	// signature order. Recorded below for the poly-recovery operand rebuild.
	nStack := len(e.ResolvedIndicesBefore(n))
	if nStack > len(positions) { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		nStack = len(positions)
	}
	// Snapshot the failed-dispatch tape state for the poly no-match spec
	// BEFORE the operand-resolution loop below mutates it in place (the
	// eval-map tape.Set) — the runtime interpreter's sigError reads exactly
	// this state (plan 3c).
	noMatchProbe := e.PolyNoMatchProbe(w.Name, pos)
	args := make([]core.Value, len(positions))
	for i, p := range positions {
		av := e.Tape.At(p)
		// Resolve simple word references to their def bindings — the
		// tape still holds raw Words for forward operands at this
		// recovery point, and both the partition probe below and the
		// assumed sig's ReturnsFn want values, not names.
		if core.IsWord(av) {
			if wi, werr := core.AsWord(av); werr == nil {
				if top, ok := e.Registry.Defs.Top(wi.Name); ok {
					av = top
					e.Registry.Check.RecordUse(wi.Name)
				}
			}
		}
		// Auto-evaluate a raw eval-map operand exactly as the runtime match's
		// execMatch would (word members resolve against the live frame, the
		// recorder assembles a per-run OpMakeMap): the recovery otherwise
		// hands the RAW source map to the poly record, which either baked a
		// live word member as a frozen const (the repl-eval `{line: src}`
		// request map) or refuses. Errors leave the raw operand — the
		// assumed-sig model stays as before.
		if core.IsConcrete(av) && av.Parent != nil && av.Parent.ConformsTo(core.TMap) && core.BearsActiveTokens(av) {
			if ev, everr := e.AutoEvalMap(av, false, true); everr == nil {
				e.Tape.Set(p, ev)
				av = ev
			}
		}
		args[i] = av
	}
	// Strict disjunct rescue (design/checker-accuracy-review.10.md A1):
	// the whole disjunct matched no signature, but individual
	// alternatives may dispatch fine. If at least one does, splice the
	// per-alternative join — the failing alternatives have already
	// been flagged with partial_dispatch warnings, which name the
	// exact path that would fail; the blanket no_signature error
	// would be wrong for the paths that DO dispatch.
	if out, ok := disjunctPartitionReturns(e.Registry, w.Name, args, pos); ok {
		// A strict-disjunct straddle is a runtime-dispatch case, not an
		// inherent refusal: when the word is a safe poly candidate (core
		// builtin, single result, no meta/fn-value/code-body sig), record
		// OpCallNativePoly so the VM re-matches the one concrete alternative
		// at run time — e.g. `(3 and "x") add 1` → `'x1'`, mirroring the
		// normal-path handling in CarrierResults. The poly call needs its
		// operands in SIGNATURE order (sig[0] = top of stack): forward args
		// fill the leading positions in source order, then the stack args
		// fill the rest top-down (the deepest-last ascending run reversed).
		// Feeding the raw tape order here was the prior `[1x]`-vs-`[x1]`
		// operand-order divergence. Only refuse when poly isn't safe.
		if sw := core.SigOrderArgs(args, nStack); dispatchTryRecordPoly(e.Registry, w.Name, sig, sw, out, pos, true, nil, false, noMatchProbe.Spec(fn, sw)) {
			spliceCheckResults(e, positions, out)
			return nil
		}
		// A single-overload user fn over a disjunct-typed operand recovers here
		// (e.g. the boru:test framework's run-cases inside test-describe's body);
		// record a guarded CALL_USER instead of refusing (it splices its own
		// returns). Reached from the eng harness since the partitioned-dispatch
		// recording landed (carrier_ljoin_test.go drives the recovery arm).
		if e.TryRecordRecoveredUserFn(sig, fn, args, nStack, positions) {
			return nil
		}
		// A MULTI-overload user fn over a strict-disjunct operand (`g (h true)`
		// where h returns `(Integer tor String)` and g has an Integer and a
		// String arm): the operand is a runtime disjunct the checker cannot
		// pin to one arm, but every same-arity arm sharing the committed
		// return bakes to OpCallUserPoly, and the VM re-matches the concrete
		// alternative at run time — the same §6b machinery the gradual-Any
		// clusterC path uses, here reached through the disjunct partition
		// (REFUSAL-CLOSURE §9.4 union-return poly). tryCompileUserPolyArms
		// declines (keeping the refusal) for divergent-return or
		// non-plain-param arm sets.
		if es := e.Registry.Check.Recorder(); es.Active() {
			sw := core.SigOrderArgs(args, nStack)
			if plan := dispatchCompileUserPolyArms(e.Registry, es, w.Name, sw, sig.Returns); plan != nil {
				plan.SubstituteJoinedOuts(out)
				es.RecordUserPolyCall(w.Name, e.Registry, plan.SigIdx(), plan.Units(), plan.Impls(), plan.Sigs(), sw, out, pos)
				spliceCheckResults(e, positions, out)
				return nil
			}
		}
		e.Registry.Check.Recorder().MarkUncompilable("unmatched dispatch recovered at " + w.Name)
		spliceCheckResults(e, positions, out)
		return nil
	}
	// No disjunct partition. When an operand is an Any-typed carrier — a value
	// of statically-unknown type, e.g. `get` over a List/Map element
	// (`(cases _i get) get "in"`) — matchSignature could not commit to an
	// overload, but at run time the value is concrete and the SAME first-match
	// the interpreter takes dispatches it. For a SAFE pure core builtin, record
	// a runtime-re-matching OpCallNativePoly instead of refusing: tryRecordPoly's
	// gates keep meta / fn-value / mutating (set) / code-body / multi-result
	// words out, so only words whose runtime re-match is faithful poly. Results
	// are computed with recording suspended so the program records ONLY the poly
	// call, never a duplicate CALL_NATIVE for the same dispatch. Concrete (non-
	// Any) operands that reach here are a genuine type error and still refuse.
	es := e.Registry.Check.Recorder()
	if es.Active() && (AnyAnyCarrier(args) || anyDisjunctCarrier(args)) {
		resume := es.Suspend()
		results := CarrierResults(e.Registry, w.Name, sig, args, pos, nil, false)
		resume()
		// Re-match over the dispatching registry: a module sub-registry word
		// (the test framework's `test-record`, run via CallBoru in the module's
		// sub-registry) must re-match over THAT registry at run time, not the
		// main one, or callPoly's Lookup misses it. e.registry is the same
		// sub-registry pointer the compiled run reaches (it lives on the module
		// export). For a core builtin e.registry is the main registry, so
		// PolyRef.Reg then equals the VM's own registry — the no-op the
		// get/add path already relied on.
		if sw := core.SigOrderArgs(args, nStack); dispatchTryRecordPoly(e.Registry, w.Name, sig, sw, results, pos, false, e.Registry, true, noMatchProbe.Spec(fn, sw)) {
			spliceCheckResults(e, positions, results)
			return nil
		}
		// A SINGLE-overload user/module fn recovered over an Any-typed operand:
		// matchSignature could not statically commit (the operand's type is unknown),
		// but with exactly ONE overload the runtime dispatch is unambiguous — the
		// concrete value either matches the sole sig (dispatch == interpreter) or
		// fails the VM's CALL_USER param contract (raise == the interpreter's
		// no_signature). tryRecordPoly can't take it (user fns have an FnFrame; only
		// sub-registry builtins pass), so drive its ReturnsFn NON-suspended to compile
		// the body unit and record a GUARDED CALL_USER — BuildFnBodyReturnsFn's
		// SetUnitParamTypes installs the param contract the VM enforces at entry, so a
		// runtime arg that misses the sole sig raises exactly as the interpreter does.
		// This is what unblocks the boru:test framework (run-cases) and the trie/
		// decision walkers (find-kid / mk-tnode / lex-mustache). A MULTI-overload fn
		// stays refused below (Cluster C): one baked overload would raise where the
		// interpreter runtime-dispatches a sibling.
		if e.TryRecordRecoveredUserFn(sig, fn, args, nStack, positions) {
			return nil
		}
		// A statically-failed dispatch that no recovery owns can still
		// compile to a terminal trap / runtime rematch — the same attempt
		// the no-partition fall-through below makes. The disjunct-carrier
		// window routes to the rematch gate, whose offset-form render
		// bound and match-defers arm keep it sound (a runtime match
		// defers to the interpreter; a no-match raises the shared rich
		// diagnostic over the bounded slice).
		if e.TryRecordUnmatchedDispatchTrap(w, fn, pos) {
			spliceCheckResults(e, positions, results)
			return nil
		}
		// On a REAL compile pass (Compiling) the MarkUncompilable already refuses
		// and Finalize surfaces THIS reason, so an error-severity no_signature
		// diagnostic here would only mask it as the generic "check diagnostics"
		// (boru.go:297). On a plain check pass this branch is still reachable —
		// IsolateEmit arms a fresh ACTIVE Emit while analysing each fn body — and
		// there the diagnostic IS the genuine static report, so gate it on
		// !Compiling, matching the fall-through path below.
		es.MarkUncompilable("unmatched dispatch recovered at " + w.Name)
		if !e.Registry.Check.Compiling && bestMatch < 0 {
			e.Registry.Check.AddDiagnostic(core.CheckDiagnostic{
				Code:   "no_signature",
				Detail: core.NoMatchDetail(w.Name) + "; assuming best-fit candidate for analysis",
				Word:   w.Name,
				Row:    pos.Row,
				Col:    pos.Col,
			})
		}
		spliceCheckResults(e, positions, results)
		return nil
	}
	// A no-signature dispatch reached here UNDER A SUSPENDED outer recovery
	// (es.suspended > 0) is being ANALYSED to read an enclosing dispatch's result
	// type — CarrierResults suspends recording and re-runs the body purely to
	// inspect its residual — NOT compiled. Its real compile decision happens on
	// the non-suspended recording pass (or it is subsumed by the enclosing poly's
	// runtime re-match). MarkUncompilable here PREMATURELY latches the whole
	// program refusal: the trie find-kid `(nd "kids" get) get (ch)` shape refuses
	// because the inner get's result-type probe analyses the outer get against a
	// transient String-carrier alternative. Skip the latch (and its diagnostic)
	// under suspend; still splice the analysis result so the enclosing probe
	// reads a residual.
	if !es.SuspendedNow() {
		// A STATICALLY-DEFINITE unmatched dispatch — every value the failed
		// match examined is identical at run time — compiles to a terminal
		// OpTrap raising the interpreter's byte-identical error instead of
		// refusing the whole program (the error-row doctrine: a spec ERROR row
		// yields a Program that raises the same taxonomy at the same point).
		// Ineligible shapes (a carrier operand whose runtime tag could match, a
		// nested frame/unit, a plain check pass) keep the blanket refusal.
		if !e.TryRecordUnmatchedDispatchTrap(w, fn, pos) {
			// The trap DECLINED: the mismatch is not statically definite — a
			// carrier operand's runtime tag could still match. An IMPRECISE
			// carrier (a scalar tag a multi-branch narrowing settled on, the
			// mini-redis `join " " reply` where reply IS a list at run time) is a
			// checker stand-in, not a concrete value, so recover via the runtime-
			// re-matching poly instead of refusing — the DEFINITE mismatches (a
			// disjoint Box<String> vs Box<Integer> param) already trapped above,
			// so only genuinely could-match carriers reach here. tryRecordPoly /
			// the single-overload user-fn recovery decline (leaving es untouched)
			// for anything their own gates reject, and the refusal below stands.
			// ONLY the native-poly recovery (OpCallNativePoly), never the
			// single-overload USER-fn recovery: a user fn's guarded CALL_USER
			// enforces the param's NOMINAL type at entry, not a value-sensitive
			// PREDICATE / refinement (`def Big (Integer gt 10) def g […n:Big…]`),
			// so a nominally-typed but predicate-failing arg would run the body
			// compiled where the interpreter raises. tryRecordPoly re-runs the
			// native's own matchSignature over the concrete runtime value (the
			// redis `join` re-match), which is faithful; a user fn stays refused
			// and falls back.
			recovered := false
			if es.Active() && anyImpreciseCarrier(args) {
				resume := es.Suspend()
				results := CarrierResults(e.Registry, w.Name, sig, args, pos, nil, false)
				resume()
				if sw := core.SigOrderArgs(args, nStack); dispatchTryRecordPoly(e.Registry, w.Name, sig, sw, results, pos, false, e.Registry, true, noMatchProbe.Spec(fn, sw)) {
					spliceCheckResults(e, positions, results)
					recovered = true
				}
			}
			if recovered {
				return nil
			}
			e.Registry.Check.Recorder().MarkUncompilable("unmatched dispatch recovered at " + w.Name)
		}
	}
	// Emit the error-severity no_signature diagnostic ONLY off a REAL compile pass
	// (!Compiling), where it is the genuine static report of an unmatched dispatch.
	// This gate is INDEPENDENT of the suspend skip above: a plain check reports a
	// genuine unmatched dispatch even when it is reached under a suspended
	// sub-probe (the over-suppression that dropping it inside the suspend branch
	// caused), while a compile pass never adds it (Finalize surfaces the
	// MarkUncompilable reason; a diagnostic would only mask it as the generic
	// "check diagnostics", boru.go:297). Do NOT additionally gate on
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
	// `!Compiling` guard alone is the correct condition.
	// EXCEPTION to the "!Compiling alone" rule: a SINGLE-overload user fn
	// dispatched over an Any/disjunct-CARRIER arg (a value of statically-unknown
	// type, not a concrete mismatch) is NOT a genuine unmatched dispatch — it is
	// the exact shape the armed (compile) pass RECOVERS as a guarded CALL_USER
	// above (singleOverloadRecoverable), where the VM's param contract raises ==
	// the interpreter iff the runtime value misses the sole sig. A plain check
	// pass reaches HERE (Emit inactive, so the recovery block was skipped) and
	// would otherwise flag what compile silently recovers — the `engine-known
	// engine` false positive (`is String`-guarded Options-`get`). Suppressing is
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
	if !e.Registry.Check.Compiling && !recoverableUnknownType {
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
	core.CheckBraid.RefuseForwardStackDrift = RefuseForwardStackDrift
	core.CheckBraid.RefuseStrandedMemberFn = refuseStrandedMemberFn
	core.CheckBraid.ShareCheckState = shareCheckState
	core.CheckBraid.SpliceAnonCheckResult = spliceAnonCheckResult
	core.CheckBraid.SpliceCheckResults = spliceCheckResults
	core.CheckBraid.SpliceFnValueCheckResult = SpliceFnValueCheckResult
	core.CheckBraid.TagCheckModeDefRead = tagCheckModeDefRead
	core.CheckBraid.TryDynamicFnValueDispatch = tryDynamicFnValueDispatch
	core.CheckBraid.TryMemberFnArrivalDispatch = tryMemberFnArrivalDispatch
	core.CheckBraid.ParenPlacedFnCarrier = parenPlacedFnCarrier
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
	core.AnalysisImpl.ZeroOutResiduals = stripZeroOutResiduals
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
// never calls. `def I x:Integer => [add 1 x] end I 5` prints `I 5` and
// exits 0 — the minted lattice node is placed, the 5 is never consumed,
// and nothing anywhere says so. The combinator literature is all capitals
// (S, K, I, B, C, W, Y), so a reader transcribing it lands here first
// (design/HIGHER-ORDER-FUNCTIONS.0.md §5.1, recommendation 2).
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
		if !fnBodiedTypeNode(v) || core.IsBareTypeNode(residual[i+1]) {
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
				"a def whose name is capitalised and whose body is a fn mints a TYPE " +
					"(`4 is " + name + "` is the intended use); the fn body survives only as that type's content",
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
	if !core.IsBareTypeNode(v) {
		return false
	}
	content, ok := core.TypeContentOf(v)
	return ok && core.IsFnValueResidual(content)
}

// parenPlacedFnCarrier is fnReturnPark's check-side twin (NUR073's BROAD
// park): a pinpointed member-fn read carries its fn identity in the
// recorder's side table, not in the carrier's type, so core cannot see
// that a user paren just collapsed to a FUNCTION. Without this the lanes
// disagree — `(m dot f) 5` parked interpreted and applied compiled, which
// TestCompiledCombinationParity caught. Dot SUGAR is unaffected: its group
// is reach-lowered, excluded before the park asks.
func parenPlacedFnCarrier(e *core.Engine, idx int) bool {
	r := e.Registry
	if r == nil || r.Check == nil {
		return false
	}
	es := r.Check.Recorder()
	if es == nil || !es.Active() {
		return false
	}
	v := e.Tape.At(idx)
	if v.ID == "" {
		return false
	}
	// FOUR shapes place, and they are one decision with one record.
	//
	// A GENUINE fn-typed carrier is visible to core, so core does not need
	// this predicate to decide the park — but the COMPILER still needs the
	// placement recorded, and this is the only place that knows a USER paren
	// (not a reach group) did the placing. Same for a DYNAMIC value the
	// checker cannot prove non-callable: the park treats it as placed, so the
	// compiler has to learn the same fact or the two ends disagree about the
	// shape. A CONCRETE fn value joins them — an inert `/v` reference, a
	// `valof`, an inline literal — because a user paren places one exactly as
	// it places a carrier, and the residual layout needs the record to know
	// that nothing will re-step it (`(inc/v) 7` is `fn inc(Integer) 7`, and
	// refusing it was reading the absence of a record as evidence). It reaches
	// only the LAYOUT reader: the two apply arms gate on Carrier and Dynamic
	// respectively, so neither sees a concrete value. Only when NONE of the
	// three holds does the original member-read gate decide, and a member read
	// that reaches it admits on the same terms.
	//
	// Both wide arms used to fall through to that gate and decline, which
	// combined with an `||` short-circuit at the call site meant a genuine
	// carrier was never recorded at all: `((mk 1) 2)` placed interpreted and
	// applied compiled, because the residual lowering had no way to learn the
	// lead was placed data (NUR101).
	if !core.IsFnTypedCarrier(v) && !(v.Dynamic && core.SigTypeMatches(v, core.TFunction)) &&
		!core.IsFnValueResidual(v) {
		if _, ok := es.MemberFnReadValue(v.ID); !ok {
			return false
		}
	}
	// Core asks only AFTER excluding reach groups, so reaching here proves a
	// USER paren: record the id for the compiler's residual lowering, which
	// must not lower a placed lead as an apply (ParenPlacedFnIDs — see its
	// doc in core/go/check_state.go).
	cs := r.Check
	if cs.ParenPlacedFnIDs == nil {
		cs.ParenPlacedFnIDs = map[string]bool{}
	}
	cs.ParenPlacedFnIDs[v.ID] = true
	return true
}
