package check

// Check-piece fn-body machinery: the construction-time static body pass, the
// return-conformance mirror, and the analysis ReturnsFunc builder. Extracted
// from core_helpers.go (Stage 2b of the four-piece split) — everything here
// runs only under an active CheckState.

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// checkBodyReturnConformance is the check-mode mirror of the runtime RET
// check (validateReturnTypes): the analysed body residual's top K values are
// compared slot-wise against the K declared return types, and a mismatch is
// reported as the SAME type_error the runtime raises (returnTypeErrorText, so
// the two surfaces are byte-identical). Unlike the runtime's membership check
// (`got.Is(exp)`), only a PROVABLY IMPOSSIBLE return flags here: the residual
// type neither conforms to the declaration nor the declaration to it, and
// their tand meet is Never — the same disjointness proof the gradual param
// match uses (sigTypeMatches). A subtype/refine declaration stays
// value-dependent (`[n add 1]` against a declared `Big (Integer gt 10)` may
// pass at runtime) and is left to the RET check. Dynamic residuals are the
// gradual contract; a short residual (recursion bail, zero-net body shape),
// a None placeholder, and a bare type node are left to the runtime arity /
// membership errors; a disjunct flags only when EVERY alternative is
// impossible. Deduped by detail — the ReturnsFn runs once per analysed call
// shape, but shapes repeat across call sites.
//
// The COUNT mirror of the same boundary: a residual provably LONGER than the
// declaration tolerates is the runtime's "expected N return value(s), got M"
// (returnCountErrorText — the __RC arity rule, which discards bottom extras
// only up to unnamedCount, the fn's unconsumed unnamed-arg allowance). Only a
// count the analysis knows exactly flags: a variadic spread models 0-or-more
// values, and a Function in the residual may be an unapplied fn-value
// call the static model over-counts (the emit.go cluster-E shape) — both
// skip. The short side (len < declared) stays with the runtime arity error —
// EXCEPT the all-concrete-call EMPTY residual on the top-level straight
// line: a declared fn whose per-call analysis (real argument values, no
// generalisation) nets NOTHING either diverged (a taken `raise` branch —
// the divergence model leaves no carrier) or under-returns, and the runtime
// errors EITHER way (the raise, or "expected N…, got 0"), so the call is a
// guaranteed program error. argsConcrete gates it to real concrete-arg
// calls — the install-time synthetic example eval and generalised analyses
// use carriers and never fire it.
func checkBodyReturnConformance(r *core.Registry, name string, declared []*core.Type, patterns []*core.Value, unnamedCount int, argsConcrete bool, stk []core.Value, pos, bodyEnd core.SrcPos) {
	if len(declared) == 0 || !r.Check.IsActive() {
		return
	}
	if len(stk) < len(declared) {
		// Not under SuppressBodyErrors: there the call is an ASSUMED dispatch
		// over args the real match already rejected (checkModeAssumeSig) or a
		// narrowed recursive re-entry, and "the call always errors" is the
		// cascade of the one honest finding at the call site — the
		// no_signature. Measured: `def p word [1 add 2] def f fn
		// [[x:Integer][Integer][x mul 10]] f p` (word-splice.tsv:L115) drew
		// this on the compile-armed pass alone, the fn-memo drop there
		// re-running the body the plain pass served from its summary.
		if len(stk) == 0 && argsConcrete && r.Check.SuppressBodyErrors == 0 && CheckAtUncaughtTopLevel(r) &&
			!fnBodyUndefinedWordShield(r, name, pos, bodyEnd) {
			detail := fmt.Sprintf(
				"%s: the body produces no return value for this call (declared %d) — the call always errors",
				name, len(declared))
			if !hasCheckDiagnostic(r, "type_error", detail) {
				r.Check.AddDiagnostic(core.CheckDiagnostic{
					Code:          "type_error",
					Detail:        detail,
					Word:          name,
					Row:           pos.Row,
					Col:           pos.Col,
					RuntimeMirror: true,
				})
			}
		}
		return
	}
	// A residual computed while this body contained an UNDEFINED word is not
	// evidence: a mutually-recursive sibling defined later (`def isod … isev
	// … def isev …`) makes the install-time analysis see an incomplete world,
	// and the FnSummaries memo then serves that broken residual to every
	// later call. The undefined_word diagnostic tagged with this fn's name is
	// still in the list here (RescueForwardRefDiagnostics drops it only at
	// end of pass), so its presence is the reliable "analysis ran early"
	// signal — skip; the runtime RET check still guards these bodies.
	//
	// Scoped to THIS body's source span [pos, bodyEnd]: an undefined forward
	// ref in a DIFFERENT overload/redefinition of the same name must not
	// shield this body's conformance check (`def f fn [… [g a]] def f fn
	// [… [String] [42]] …` — the second f's Integer return is provably
	// wrong regardless of the first f's rescued `g`). An unattributed
	// diagnostic (Row 0) or an unpositioned body keeps the name-wide skip —
	// conservative, never a new false positive.
	if fnBodyUndefinedWordShield(r, name, pos, bodyEnd) {
		return
	}
	extra := len(stk) - len(declared)
	// The count mirror flags only an exactly-known count: a DYNAMIC value
	// in the residual marks a modelling seam (a mid-body `apply`, a
	// gradual branch join) where the static count can diverge from the
	// runtime one — skip, like the variadic/fn-value shapes. A bare strict
	// Any carrier is the same kind of seam: it is the lenient approximation
	// left for a 0-net / undeclared call whose body unit declined (a
	// cross-module fn calling a private 0-return helper before its own
	// return — sort.boru's counting-sort → ensure-ints), a phantom that
	// inflates the residual by an unknown count, so skip it too. It is a
	// RuntimeMirror: the compile pass deliberately COMPILES the
	// count-mismatched body and lets the VM RET raise the byte-identical
	// error (emit.go — TestEmitP5MultiResult pins it), so the compile failure
	// loop skips mirror diagnostics.
	if extra > unnamedCount &&
		!stackHasVariadic(stk) && !stackHasFnValue(stk) && !stackHasDynamic(stk) &&
		!stackHasApproxAny(stk) {
		detail := core.ReturnCountErrorText(name, len(declared), len(stk)-unnamedCount,
			stk[unnamedCount:])
		if !hasCheckDiagnostic(r, "type_error", detail) {
			r.Check.AddDiagnostic(core.CheckDiagnostic{
				Code:          "type_error",
				Detail:        detail,
				Word:          name,
				Row:           pos.Row,
				Col:           pos.Col,
				RuntimeMirror: true,
			})
		}
	}
	for k, exp := range declared {
		got := stk[extra+k]
		// The PATTERN check comes first and is not gated on exp: a declared
		// union return degrades its *Type to Any precisely because the union
		// has no lattice node to name, so the `exp.Equal(TAny)` skip below
		// would drop the only contract there is. `Type` aliases `Value`, so
		// the pattern doubles as the "expected" rendering.
		if k < len(patterns) && patterns[k] != nil &&
			!core.IsDynamic(&got) && got.Parent() != nil && !core.IsBareTypeNode(&got) && !got.Parent().Equal(core.TNone) {
			if _, ok := core.Unify(*patterns[k], got); !ok && !refinementUndecided(*patterns[k], got) {
				detail, _ := core.ReturnTypeErrorText(name, k+1, patterns[k], got)
				if !hasCheckDiagnostic(r, "type_error", detail) {
					r.Check.AddDiagnostic(core.CheckDiagnostic{
						Code:          "type_error",
						Detail:        detail,
						Word:          name,
						Row:           pos.Row,
						Col:           pos.Col,
						RuntimeMirror: true,
					})
				}
				continue
			}
		}
		if exp == nil || exp.Equal(core.TAny) {
			continue
		}
		if core.IsDynamic(&got) || got.Parent() == nil || core.IsBareTypeNode(&got) || got.Parent().Equal(core.TNone) {
			continue
		}
		if !residualProvablyDisjoint(got, exp) {
			// Value-level membership for a compile-time-known scalar residual:
			// the runtime RET boundary asks got.Is(exp) on this SAME value, so
			// a concrete scalar that fails membership NOW is a guaranteed
			// runtime type_error — flag it statically. This closes the
			// smart-constructor holes the disjointness test cannot see:
			//   - predicate returns: `def mkbad fn [[] [Big] [5]]` (Big =
			//     Integer gt 10; 5 fails the predicate) — previously
			//     check-clean, runtime-only;
			//   - nominal newtype returns: `def mk fn [[] [UserId] [42]]`
			//     (a raw Integer is not a UserId; a reparented `def x:UserId
			//     42  x` residual carries the UserId tag and passes).
			// Restricted to scalarFoldOperand shapes (concrete scalar
			// payloads — literals and folded comparisons), where the checked
			// value provably equals the runtime value; abstract carriers keep
			// the runtime check (value-dependent returns bail by design).
			if !ScalarFoldOperand(got) || got.Is(exp) {
				continue
			}
		}
		detail, _ := core.ReturnTypeErrorText(name, k+1, exp, got)
		if !hasCheckDiagnostic(r, "type_error", detail) {
			// A RuntimeMirror like the count path: the VM RET re-checks the
			// runtime value and raises the byte-identical type_error, so the
			// compiled error path is exact.
			r.Check.AddDiagnostic(core.CheckDiagnostic{
				Code:          "type_error",
				Detail:        detail,
				Word:          name,
				Row:           pos.Row,
				Col:           pos.Col,
				RuntimeMirror: true,
			})
		}
	}
}

// fnBodyUndefinedWordShield reports whether an undefined_word diagnostic is
// attributed to THIS fn body's source span — the "analysis ran early"
// signal (a mutually-recursive sibling defined later makes the install-time
// analysis see an incomplete world, and FnSummaries then serves that broken
// residual to every later call). Scoped to [pos, bodyEnd]: an undefined
// forward ref in a DIFFERENT overload/redefinition of the same name must
// not shield this body's conformance check. An unattributed diagnostic
// (Row 0) or an unpositioned body keeps the name-wide shield —
// conservative, never a new false positive.
func fnBodyUndefinedWordShield(r *core.Registry, name string, pos, bodyEnd core.SrcPos) bool {
	for _, d := range r.Check.Diagnostics {
		if d.Code != "undefined_word" || d.FnName != name {
			continue
		}
		if d.Row != 0 && (bodyEnd.Row != 0 || bodyEnd.Col != 0) &&
			(posBefore(d.Row, d.Col, pos) || posBefore(bodyEnd.Row, bodyEnd.Col, core.SrcPos{Row: d.Row, Col: d.Col})) {
			continue // attributed outside this body — not this body's signal
		}
		return true
	}
	return false
}

// hasCheckDiagnostic reports whether a diagnostic with the given code and
// exact detail is already recorded — the dedupe the per-call-shape ReturnsFn
// paths use (one analysed shape can repeat across call sites). A caught
// (downgraded) entry does not count: it must never mask a later REAL
// emission of the same finding outside the trapping region.
func hasCheckDiagnostic(r *core.Registry, code, detail string) bool {
	for _, d := range r.Check.Diagnostics {
		if d.Code == code && d.Detail == detail && !d.CaughtAtRuntime {
			return true
		}
	}
	return false
}

// checkRecordShapeArgs is the pattern / record-shape check for one analysed
// call: for each declared record-typed param, verify the arg map carries each
// declared field key with a conforming type. Skips calls whose arg is empty
// or whose key set doesn't overlap the pattern at all (that shape is
// typically the synthetic/default arg map used during fn body analysis, not
// a real user call).
func checkRecordShapeArgs(r *core.Registry, name string, paramPatterns []*core.Value, args []core.Value) {
	for i, pat := range paramPatterns {
		if pat == nil || i >= len(args) {
			continue
		}
		val := args[i]
		if !pat.Parent().Equal(core.TMap) || !val.Parent().Equal(core.TMap) ||
			!core.IsConcrete(pat) || !core.IsConcrete(&val) {
			continue
		}
		pMap, _ := core.AsMap(*pat)
		vMap, _ := core.AsMap(val)
		if pMap == nil || vMap == nil || vMap.Len() == 0 {
			continue
		}
		overlap := 0
		for _, k := range pMap.Keys() {
			if _, ok := vMap.Get(k); ok {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}
		for _, key := range pMap.Keys() {
			pv, _ := pMap.Get(key)
			av, hasKey := vMap.Get(key)
			if !hasKey {
				r.Check.AddDiagnostic(core.CheckDiagnostic{
					Code:   "record_shape_mismatch",
					Detail: "argument to " + name + " missing field: " + key,
					Word:   name,
				})
				continue
			}
			if core.IsBareTypeNode(&pv) && !av.Parent().ConformsTo(pv.Parent()) && !av.Parent().Equal(core.TAny) {
				r.Check.AddDiagnostic(core.CheckDiagnostic{
					Code:   "record_shape_mismatch",
					Detail: "argument to " + name + ": field " + key + " expected " + pv.Parent().String() + ", got " + av.Parent().String(),
					Word:   name,
				})
			}
		}
	}
}

// BuildFnBodyReturnsFn is the analysis half of a fn sig's dispatch — the
// sig's handler.
func BuildFnBodyReturnsFn(r *core.Registry, name string, s core.FnSig, fnDef core.FnDefInfo) core.ReturnsFunc {
	paramNames := make([]string, len(s.Params))
	paramPatterns := make([]*core.Value, len(s.Params))
	for i, p := range s.Params {
		paramNames[i] = p.Name
		paramPatterns[i] = p.Pattern
	}
	declaredReturns := append([]*core.Type(nil), s.Returns...)
	declaredReturnPatterns := append([]*core.Value(nil), s.ReturnPatterns...)
	bodyTraps := bodyTrapsErrors(s.Body())
	// An anonymous lambda's `Returns=[Any]` is a placeholder the analyser
	// infers past (nil'd for AnalyseFnBody so the residual carries the body's
	// real type); the interpreter enforces the placeholder's COUNT at every
	// dispatch boundary, and trimUnnamedArgs below keeps that count.
	lambdaReturns := len(s.Returns)
	if fnDef.Anonymous {
		declaredReturns = nil
		declaredReturnPatterns = nil
	}
	// The sig's own body: a fn body's array is never written, and every
	// analysis and run copies what it steps. Clamped, so an append
	// reallocates.
	sigBody := s.Body()
	bodyToks := sigBody[:len(sigBody):len(sigBody)]
	nameCopy := name
	capturesCopy := fnDef.Captured
	genSpec := fnDef.Gen
	sigParams := append([]core.FnParam(nil), s.Params...)
	return func(args []core.Value, caller *core.Registry) []core.Value {
		// The MERGED-WORD seam (Stage M1, design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore
		// §2.4a/§5): a transplanted word-extension sig (open words — a module-
		// defined `add` merged into the importer's dispatch table) dispatches as a
		// BARE word on the importer's engine, so no execFnDefLiteral wrapper
		// shares the check state with this closure's captured registry r. Without
		// sharing, the analysis read the module's own INACTIVE CheckState and
		// the body was ANALYSED CONCRETELY with its diagnostics stranded on the
		// invisible module Check. Sharing caller → r at the ReturnsFn boundary
		// itself converts the WHOLE user-fn seam (every dispatch path that
		// reaches a foreign-registry fn body analyses identically — the §2.2
		// one-path rule): body diagnostics and the FnSummaries/FnInflight memos
		// (scopeID-disjoint keys) land on the dispatching pass, while name
		// resolution stays in the module's own Defs/Types. No-op when caller == r
		// (ordinary same-registry fns), when an enclosing execFnDefLiteral /
		// execFnDefSig dispatch already shared (pointer-equal swap), and outside
		// check mode — interpretation is untouched.
		restoreCheck := shareCheckStateFrom(r, caller)
		defer restoreCheck()
		checkRecordShapeArgs(r, nameCopy, paramPatterns, args)
		// Generic fns (Phase 5): infer the parameter bindings from the
		// call's arg carriers and install them around the body
		// analysis, so body-internal `of [T]` / `make (Box of [T])`
		// resolve per call site — the FnSummaries memo key already
		// includes the arg types, so each distinct instantiation is
		// analysed once (monomorphization for free). The bindings are
		// popped explicitly (Defs.Pop is non-retiring); there is no
		// DefCleanup frame on this path.
		var genBindings map[string]core.Value
		var genNames []string
		if genSpec != nil {
			genBindings = core.InferGenBindings(genSpec, sigParams, args)
			genNames = core.InstallGenBindingMap(r, genSpec, genBindings)
		}
		// Deferred-list body (def-node-binding.tsv:54) — `def mk fn
		// [[c1:Integer] [List] [[c1]]]`. The body is a single list literal that
		// references a parameter, but a returned list NEVER closes over the param
		// (only a `=>` lambda captures); the interpreter returns it RAW and
		// auto-evaluates it later, in MODULE scope, against the live binding
		// (`mk 9` → `[1]`, the module `c1`, not the arg `9`). So make the fn
		// TRANSPARENT — hand the raw deferred list back as the call's residual and let
		// the check pass fold it in module scope EXACTLY as a top-level
		// `def c1 1 [[c1]]` already does: at end-of-run, at a `def`-bind, or at a
		// downstream consumer (each resolves `c1` against the binding live at that
		// point). Still analyse the body first so its diagnostics propagate.
		// Only an ANONYMOUS fn (afn / =>) keeps this no-closures transparency.
		// A NAMED fn always creates a function context — its params are in scope
		// for the whole body, including a returned bare list — so it falls
		// through to the normal analysis, where the list is assembled in-frame
		// against the param (fixed at call time).
		if raw, ok := DeferredParamListResidual(bodyToks, paramNames); ok && fnDef.Anonymous {
			// An afn is never generic (afnHandler declares no type params, so
			// fnDef.Gen is nil whenever fnDef.Anonymous), hence genNames is empty
			// here — no gen-binding cleanup is needed before the early return. A
			// generic NAMED fn with a deferred-list body is not anonymous, so it
			// falls through to the normal path, which pops genNames below.
			AnalyseFnBody(r, nameCopy, paramNames, bodyToks, args, capturesCopy, declaredReturns, fnDef.Anonymous)
			return []core.Value{raw}
		}
		// Always analyse the body so diagnostics emitted by stepWord
		// (undefined_word, no_signature, …) inside the body propagate
		// up to the parent registry. When the fn declares an explicit
		// return type, we use that for the carrier result and drop
		// the analyser's residual stack — the analyser is run purely
		// for its side-effecting diagnostic collection. Memoisation
		// inside AnalyseFnBody keeps recursive / repeated calls cheap.
		// A shaped flex container passed to a user fn may be written through
		// the parameter's alias, which the body analysis (over a plain
		// carrier) does not see: its claims are stale from here (NUR315).
		for _, a := range args {
			PoisonFlexShapes(a)
		}
		stk := AnalyseFnBody(r, nameCopy, paramNames, bodyToks, narrowArgsToParams(args, sigParams), capturesCopy, declaredReturns, fnDef.Anonymous)
		stk = collapseElidedTailApply(bodyToks, stk, fnDef.Anonymous || len(declaredReturns) == 1)
		for i := len(genNames) - 1; i >= 0; i-- {
			r.Defs.Pop(genNames[i])
		}
		if genSpec == nil {
			checkBodyReturnConformance(r, nameCopy, declaredReturns, declaredReturnPatterns,
				unnamedParamCount(sigParams), allConcreteArgs(args), stk,
				bodyStartPos(bodyToks), bodySpanEnd(bodyToks))
		}
		if len(declaredReturns) > 0 {
			out := make([]core.Value, len(declaredReturns))
			for i, t := range declaredReturns {
				// A return slot naming a type parameter refines to the
				// call's inferred binding. An uninferable parameter is
				// reported (unbound_param) and degrades to dynamic(Any)
				// — never a silent strict Any.
				if genSpec != nil {
					if pname := core.TypeParamName(t); pname != "" {
						if b, ok := genBindings[pname]; ok {
							out[i] = core.GenBindingCarrier(r, b)
							continue
						}
						// Dedupe identical emissions: the ReturnsFn runs
						// once per analysed call shape AND once for the
						// dynamic-help example generator's synthetic
						// invocation at install — the same text twice
						// helps nobody (the FnSummaries memo dedupes the
						// body analysis but not this substitution).
						detail := fmt.Sprintf(
							"%s: type parameter %s cannot be inferred from this call's arguments — the declared return type is unknown here",
							nameCopy, pname)
						dup := false
						for _, d := range r.Check.Diagnostics {
							if d.Code == "unbound_param" && d.Detail == detail {
								dup = true
								break
							}
						}
						if !dup {
							r.Check.AddDiagnostic(core.CheckDiagnostic{
								Code:   "unbound_param",
								Detail: detail,
								Word:   nameCopy,
								// A FN whose return names an uninferable type
								// parameter still RUNS (it returns whatever the
								// body produces; the result degrades to
								// dynamic(Any) below) — unlike an uninferable
								// `make` parameter, which cannot construct the
								// instance and is a hard error. So this is a
								// non-gating precision REPORT, not a defect
								// (spec: generics-fn.tsv §8 `loose` "the checker
								// reports the precision loss"). Warning severity
								// overrides the code map's default Error.
								Severity: core.SeverityWarning,
							})
						}
						c := core.NewCarrier(core.TAny)
						core.SetDynamic(&c, true)
						out[i] = c
						continue
					}
				}
				// A fn DECLARED to return a Function whose body produced a
				// CONCRETE closure value (a real FnDefInfo — e.g. a returned
				// lambda `([] => [c1])`) surfaces that value rather than an
				// abstract Function carrier. The carrier has no FnDefInfo, so
				// binding it (`def g (mkfn)`) and later referencing `g` would
				// fail to dispatch (undefined_word) — the closure-render gap.
				// The value itself, not a clone: CloneValue hands a Function
				// back unchanged (a fn payload is immutable), so the clone this
				// once made bought no fresh ID. Only for a concrete fn residual
				// aligned to this return slot; anything else keeps the carrier
				// below.
				if t.ConformsTo(core.TFunction) && len(stk) >= len(declaredReturns) {
					if bv := stk[len(stk)-len(declaredReturns)+i]; core.IsConcrete(&bv) &&
						bv.Parent().ConformsTo(core.TFunction) {
						out[i] = bv
						continue
					}
				}
				// A declared USER-UNION return (`def id fn [[x:T] [T] [x]]`
				// with `def T (Integer tor String)`) must DISTRIBUTE over
				// downstream dispatch: a bare carrier TAGGED T carries no
				// DisjunctInfo, so sigTypeMatches' strict-disjunct branch
				// never fires and `(id 1) add 1` failed no_signature against
				// add's Number slot — the THIRD multi-denotation carrier shape
				// after dynamic and payload-bearing disjuncts (the distribute-
				// over-dispatch invariant, checker-accuracy-review.10.md §3).
				// Surface the alternatives so disjunctPartitionReturns joins
				// the per-alternative dispatches, like an inline `tor` result.
				if dv, ok := core.UnionCarrierForType(core.NodeOf(t)); ok {
					out[i] = dv
					continue
				}
				// NUR068: a declared record return keeps its schema, and a bare
				// Map return surfaces a record-schema body residual — see
				// nur068ReturnCarrier.
				//
				// Or the body PROVED an exact leaf scalar under a declared `Any`
				// (provenNarrowerReturn): the caller sees the proven type, so its
				// dispatches over the result commit instead of re-matching at
				// run time. The TYPE only — never the residual's value.
				if rc, ok := refinedDeclaredReturn(r, t, sigParams, declaredReturnPatterns, i, len(declaredReturns), stk, bodyTraps); ok {
					out[i] = rc
					continue
				}
				c := core.NewCarrier(t)
				// A declared `Any` return is "statically unknown", not "the Any
				// root": a STRICT Any conforms to no typed slot, so a user fn
				// declaring `[Any]` poisoned every typed consumer downstream with
				// false no_signature errors (a `[Any]`-returning node lookup fed
				// into another fn's `nd:Map` param — the trie/decision walkers).
				// Mark it dynamic for optimistic matching, mirroring the native
				// `[Any]`-return handling in CarrierResults.
				if t.Equal(core.TAny) {
					core.SetDynamic(&c, true)
				}
				out[i] = c
			}
			return out
		}
		if len(stk) == 0 {
			// No declared returns and an empty body residual: the fn produces
			// ZERO values at run time (`def v fn [[x:Integer] [] []]  v 1 99`
			// → [99]; `def r (f 1)` → def_error: nothing to bind). The 0-net /
			// undeclared call leaves a lenient STRICT Any approximation.
			// It is kept strict (not dynamic): a dynamic Any would match a
			// concrete consumer's slot optimistically and hide a real error —
			// `3 add (f 1)` where f returns nothing must still flag. But it is a
			// PHANTOM from the analysis's point of view (the call nets zero at
			// runtime), so checkBodyReturnConformance's count mirror excludes a
			// bare strict-Any carrier from the residual count (stackHasApproxAny),
			// which cleared the false "expected N return value(s), got N+1" on a
			// fn calling a private 0-return helper before its own return
			// (sort.boru's counting-sort → ensure-ints, reached cross-module).
			return []core.Value{core.NewCarrier(core.TAny)}
		}
		// Undeclared fn (anonymous lambda, 0-return fn) with a non-empty body
		// residual: the body's residual IS the result. An anonymous lambda's
		// frame keeps its placeholder count off the top and drops the unnamed
		// args pushed beneath the body (trimUnnamedArgs, NUR255).
		if fnDef.Anonymous {
			stk = trimUnnamedArgs(stk, lambdaReturns, unnamedParamCount(sigParams))
		}
		return stk
	}
}

// collapseElidedTailApply is collapseTailApply's twin for the PLAIN check
// pass, which has no unit to ask: a body whose last token is the `apply`
// word and whose analysed residual ends in a fn-typed CARRIER is the apply
// the check engine could not re-step (applyReturns hands the carrier back
// as the identity), and at run time it nets ONE value — the applied fn
// consumes what it matches, and every other count is the interpreter's own
// return error, which ends the run. So the residual collapses to one
// gradual result, exactly as the compiled unit's call site does, and the
// program's checked types stay sound over what actually runs (the
// type-soundness gate caught the two B rows at [Integer Function] for an
// actual [Integer]). Only where ONE value is the contract — a lambda's
// count contract, a declared single return: a declared tuple keeps its
// residual, since the applied fn may under-apply into exactly that count.
// A residual the recorder already collapsed ends in a gradual carrier, not
// a fn-typed one, and passes through.
func collapseElidedTailApply(body, stk []core.Value, oneValued bool) []core.Value {
	if !oneValued || len(body) == 0 || len(stk) < 2 {
		return stk
	}
	if w, err := core.AsWord(&body[len(body)-1]); err != nil || w.Name != "apply" {
		return stk
	}
	if !core.IsFnTypedCarrier(&stk[len(stk)-1]) {
		return stk
	}
	out := core.NewCarrier(core.TAny)
	core.SetDynamic(&out, true)
	return []core.Value{out}
}

// checkFnBodyAtConstruction runs a static body pass for each boru-bodied overload
// of a freshly-installed fn, against generalised (carrier) args, so an UNCALLED
// fn's body is still checked (a called fn is additionally checked per call site
// via BuildFnBodyReturnsFn). Check-mode only; the fn name must already be
// bound (recursion). Generic and Body-less (native /
// handler) overloads are skipped — a generic body needs per-call type bindings,
// and a native handler has no boru body to analyse.
func checkFnBodyAtConstruction(r *core.Registry, name string, fnDef core.FnDefInfo) {
	if r == nil || !r.Check.IsActive() || fnDef.Gen != nil {
		return
	}
	for i := range fnDef.Signatures {
		s := &fnDef.Signatures[i]
		if s.Fallback || len(s.Body()) == 0 {
			continue
		}
		// One analysis per BODY per pass. A fn reaches this check twice —
		// once where the value is constructed, once where it is installed
		// under a name — and the two differ only in the name, which
		// FnSummaries' key does not collapse. Without this the body's
		// diagnostics come out twice, byte-identically. A body with no
		// source position is not memoised: the zero SrcPos is shared by
		// every synthesized body, so keying on it would silence real
		// diagnostics from a different one.
		if pos := s.Body()[0].Pos(); pos != (core.SrcPos{}) {
			if r.Check.FnBodyChecked[pos] {
				continue
			}
			if r.Check.FnBodyChecked == nil {
				r.Check.FnBodyChecked = map[core.SrcPos]bool{}
			}
			r.Check.FnBodyChecked[pos] = true
		}
		paramNames := make([]string, len(s.Params))
		genArgs := make([]core.Value, len(s.Params))
		for j, p := range s.Params {
			paramNames[j] = p.Name
			genArgs[j] = ParamBodyCarrier(p)
		}
		var declared []*core.Type
		if !fnDef.Anonymous {
			declared = append([]*core.Type(nil), s.Returns...)
		}
		before := len(r.Check.Diagnostics)
		stk := AnalyseFnBody(r, name, paramNames, s.Body(), genArgs, fnDef.Captured, declared, fnDef.Anonymous)
		// NUR111: hold the analysed body to its OWN declaration. The dispatch
		// path runs this same mirror when it analyses a CALL, so a fn that is
		// called is covered; a fn VALUE handed to a higher-order word
		// (`[1 2] each cbad/v`) never dispatches during the pass, and without
		// this its declared return is enforced by nobody statically while both
		// engines raise. One analysis, one answer: the body reached here has
		// already been analysed, so this adds an obligation, not a pass.
		//
		// argsConcrete is FALSE by construction: genArgs are the declared
		// params' carriers, not a call's real values. That keeps the
		// empty-residual "the call always errors" arm — which is sound only
		// for a concrete-argument call — with the dispatch path, and leaves
		// this seam reporting exactly the provably-impossible returns.
		//
		// The gate is what makes that sound: a residual from CARRIER args is
		// not a return model whenever the analysis could not perform an
		// application, because the operands it already pushed stay behind.
		// generalisedResidualModelsReturn spells out the two shapes and the
		// three corpus rows that proved them.
		if generalisedResidualModelsReturn(stk, declared, s.ReturnPatterns, bodyIsInertLiteral(s.Body())) {
			checkBodyReturnConformance(r, name, declared, s.ReturnPatterns,
				unnamedParamCount(s.Params), false, stk, bodyStartPos(s.Body()), bodySpanEnd(s.Body()))
		}
		// A SPECULATIVE analysis that cannot run reports nothing. The
		// end-of-pass drain analyses bodies nobody asked about — fn values
		// nothing ever named (NUR105) — and it analyses them in ISOLATION,
		// with no enclosing stack. A body that reads the caller's stack
		// therefore cannot be run at all: the Church-numeral row in
		// frontier-hof-audit.tsv raises the strict-forward-barrier error
		// whose own text says why ("`apply` reads its receiver from the
		// enclosing stack"), on a program that runs correctly and answers 5.
		//
		// `fn_body_error` is the analyser reporting that IT could not
		// proceed, which is a fact about the analysis and not about the
		// code. For a NAMED fn it stays: defining a fn is asking for its
		// body to be analysed, and the existing surface is pinned. For a
		// body the checker volunteered to look at, the honest output of a
		// failed look is silence.
		if name == "" {
			kept := r.Check.Diagnostics[:before]
			for _, d := range r.Check.Diagnostics[before:] {
				if d.Code == "fn_body_error" {
					continue
				}
				kept = append(kept, d)
			}
			r.Check.Diagnostics = kept
		}
	}
}

// refinementUndecided reports whether a return pattern's failed Unify is a
// value-level refinement's — a DepScalar, `[(Integer gt 3)]`, or a union
// carrying one — over a residual the pass does not know: membership is the
// VALUE's, which only the run has, and the RET check asks it there. The
// named spelling (`[Big]`) defers exactly this case already (the scalar-fold
// gate below), so an inline refinement return refused an abstract Integer at
// check time that both lanes then returned (NUR309). A residual provably
// outside the refinement's base, or a compile-time-known scalar, still
// decides.
func refinementUndecided(pattern, got core.Value) bool {
	if ScalarFoldOperand(got) {
		return false
	}
	if pattern.IsDepScalar() {
		return !residualProvablyDisjoint(got, pattern.Parent())
	}
	if core.IsDisjunct(&pattern) {
		di, _ := core.AsDisjunct(pattern) // IsDisjunct: the payload is a DisjunctInfo
		for _, alt := range di.Alternatives {
			if refinementUndecided(alt, got) {
				return true
			}
		}
	}
	return false
}

// provenNarrowerReturn is the carrier for return slot i of a fn declaring n
// returns of which slot i is `Any` and whose params are all typed
// (allParamsTyped), when the analysed body residual stk
// proves an exact leaf scalar there: exactly n residual values, slot i a
// STRICT CARRIER of Integer, Float or Boolean. Exact leaves only: a join
// widens to an upper bound (List ∪ Map is Node, Integer ∪ Float is Number)
// that matches FEWER overloads than the runtime value would, and a String
// carrier holds a ProperString at run time. Carriers only: a concrete
// residual value is one call's constant.
func provenNarrowerReturn(t *core.Type, params []core.FnParam, stk []core.Value, n, i int) (core.Value, bool) {
	if t == nil || !t.Equal(core.TAny) || len(stk) != n || !allParamsTyped(params) {
		return core.Value{}, false
	}
	bv := stk[i]
	if !bv.Carrier() || core.IsDynamic(&bv) || bv.Parent() == nil || core.IsDisjunct(&bv) || bv.AscribedType() != nil {
		return core.Value{}, false
	}
	if !bv.Parent().Equal(core.TInteger) && !bv.Parent().Equal(core.TFloat) && !bv.Parent().Equal(core.TBoolean) {
		return core.Value{}, false
	}
	return core.NewCarrier(bv.Parent()), true
}

// allParamsTyped reports whether every param declares a type other than
// Any: an Any-in, Any-out fn (`def id fn [[x:Any][Any][x]]`) is how a
// program launders a value gradual on purpose, and its result stays so.
func allParamsTyped(params []core.FnParam) bool {
	for _, p := range params {
		if p.Type == nil || p.Type.Equal(core.TAny) {
			return false
		}
	}
	return true
}

// refinedDeclaredReturn is the carrier a declared return slot refines to
// from what the body shows: a record return's schema (nur068ReturnCarrier),
// else an exact leaf scalar proven under `Any` (provenNarrowerReturn).
func refinedDeclaredReturn(r *core.Registry, t *core.Type, params []core.FnParam, patterns []*core.Value, i, n int, stk []core.Value, bodyTraps bool) (core.Value, bool) {
	if rc, ok := nur068ReturnCarrier(r, t, patterns, i, n, stk); ok {
		return rc, true
	}
	if bodyTraps {
		return core.Value{}, false
	}
	return provenNarrowerReturn(t, params, stk, n, i)
}

// bodyTrapsErrors reports whether a fn body holds a `do`, which traps its
// body's raise into an Error value (DoListHandler) that the modelled residual
// does not carry: such a residual proves no exact leaf for
// provenNarrowerReturn. `def rpt fn [[] [Any] [do [def Big Integer 15 is
// Big]]]` answers true on its first call and a trapped conflict Error on its
// second, where the narrowed return claimed Boolean for both.
func bodyTrapsErrors(body []core.Value) bool {
	traps := false
	core.WalkBodyWords(body, func(w core.WordInfo, _ core.Value) {
		if w.Name == "do" {
			traps = true
		}
	})
	return traps
}
