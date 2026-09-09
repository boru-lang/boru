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
		if len(stk) == 0 && argsConcrete && CheckAtUncaughtTopLevel(r) &&
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
	// error (emit.go — TestEmitP5MultiResult pins it), so the refusal
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
			!got.Dynamic && got.Parent != nil && !core.IsBareTypeNode(got) && !got.Parent.Equal(core.TNone) {
			if _, ok := core.Unify(*patterns[k], got); !ok {
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
		if got.Dynamic || got.Parent == nil || core.IsBareTypeNode(got) || got.Parent.Equal(core.TNone) {
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
		if !pat.Parent.Equal(core.TMap) || !val.Parent.Equal(core.TMap) ||
			!core.IsConcrete(*pat) || !core.IsConcrete(val) {
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
			if core.IsBareTypeNode(pv) && !av.Parent.ConformsTo(pv.Parent) && !av.Parent.Equal(core.TAny) {
				r.Check.AddDiagnostic(core.CheckDiagnostic{
					Code:   "record_shape_mismatch",
					Detail: "argument to " + name + ": field " + key + " expected " + pv.Parent.String() + ", got " + av.Parent.String(),
					Word:   name,
				})
			}
		}
	}
}

// LambdaCountContract is the run-time RETURN contract of an ANONYMOUS fn
// (`=>` / `afn`). Its `Returns=[Any]` is a placeholder the analyser infers
// past — BuildFnBodyReturnsFn nils it for AnalyseFnBody so the residual
// carries the body's real type — but the interpreter enforces the
// placeholder's COUNT at every dispatch boundary: the named call (`def f
// (x:Integer => [x 1])  f 5`), the paren apply, the `apply` word and the
// callback seam (`each (x:Integer => [x 1]) [1 2]`) all raise `expected 1
// return value(s), got 2`, exactly as the verbose `fn [[x][Any][x 1]]` twin
// does. A compiled unit therefore keeps a COUNT-ONLY contract — n `Any`
// slots, which checkReturnContract enforces by count and never by type.
// Measured 2026-09-05 (NUR120): with the placeholder dropped outright the
// default lane answered `5 1` and `[1 1]` where the interpreter raises.
func LambdaCountContract(n int) []*core.Type {
	if n <= 0 {
		return nil
	}
	out := make([]*core.Type, n)
	for i := range out {
		out[i] = core.TAny
	}
	return out
}

func BuildFnBodyReturnsFn(r *core.Registry, name string, s core.FnSig, fnDef core.FnDefInfo) core.ReturnsFunc {
	paramNames := make([]string, len(s.Params))
	paramPatterns := make([]*core.Value, len(s.Params))
	for i, p := range s.Params {
		paramNames[i] = p.Name
		paramPatterns[i] = p.Pattern
	}
	declaredReturns := append([]*core.Type(nil), s.Returns...)
	declaredReturnPatterns := append([]*core.Value(nil), s.ReturnPatterns...)
	// The compiled unit's RET contract: a named fn's declaration as written;
	// an anonymous lambda's placeholder count (LambdaCountContract), which
	// stands at run time even though the ANALYSER below infers past it.
	compileReturns := declaredReturns
	if fnDef.Anonymous {
		compileReturns = LambdaCountContract(len(s.Returns))
		declaredReturns = nil
		declaredReturnPatterns = nil
	}
	declSite := s.Decl
	bodyCopy := append([]core.Value(nil), s.Body()...)
	// The sig's own body slice, uncopied: its backing array identifies THIS
	// construction of the lambda to the recorder's pending-apply lookup
	// (PendingClosureApply) — the applied value's sig holds the same array.
	bodyRef := s.Body()
	nameCopy := name
	capturesCopy := fnDef.Captured
	genSpec := fnDef.Gen
	sigParams := append([]core.FnParam(nil), s.Params...)
	return func(args []core.Value, caller *core.Registry) []core.Value {
		// The MERGED-WORD seam (Stage M1, design/STAGE3-INLINING-DESIGN-ROUND.0.md
		// §2.4a/§5): a transplanted word-extension sig (open words — a module-
		// defined `add` merged into the importer's dispatch table) dispatches as a
		// BARE word on the importer's engine, so no execFnDefLiteral wrapper
		// shares the check state with this closure's captured registry r. Without
		// sharing, `es` below read the module's own INACTIVE CheckState:
		// StartFnCompile declined, RecordUserCall never ran, every merged dispatch
		// refused "user fn call … (Stage 3)", and the body was ANALYSED
		// CONCRETELY with its diagnostics stranded on the invisible module Check.
		// Sharing caller → r at the ReturnsFn boundary itself converts the WHOLE
		// user-fn seam (every dispatch path that reaches a foreign-registry fn
		// body records identically — the §2.2 one-recording-path rule): body
		// diagnostics, the FnSummaries/FnInflight memos (scopeID-disjoint keys)
		// and the compiled unit all land on the dispatching pass, while name
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
		// (`mk 9` → `[1]`, the module `c1`, not the arg `9`). Compiling it as a
		// fn UNIT cannot model this: a unit's result is fixed at CALL time and
		// cannot defer to a later module rebind. So make the fn TRANSPARENT —
		// hand the raw deferred list back as the call's residual (no unit) and let
		// the check pass fold it in module scope EXACTLY as a top-level
		// `def c1 1 [[c1]]` already does: at end-of-run, at a `def`-bind, or at a
		// downstream consumer (each resolves `c1` against the binding live at that
		// point). The folded const is what the program bakes; no VM change. Still
		// analyse the body first so its diagnostics propagate.
		// Only an ANONYMOUS fn (afn / =>) keeps this no-closures transparency.
		// A NAMED fn always creates a function context — its params are in scope
		// for the whole body, including a returned bare list — so it falls
		// through to the normal analysis + unit compile, where the list is
		// assembled in-frame against the param (fixed at call time, which the
		// unit models exactly).
		if raw, ok := DeferredParamListResidual(bodyCopy, paramNames); ok && fnDef.Anonymous {
			// An afn is never generic (afnHandler declares no type params, so
			// fnDef.Gen is nil whenever fnDef.Anonymous), hence genNames is empty
			// here — no gen-binding cleanup is needed before the early return. A
			// generic NAMED fn with a deferred-list body is not anonymous, so it
			// falls through to the normal path, which pops genNames below.
			AnalyseFnBody(r, nameCopy, paramNames, bodyCopy, args, capturesCopy, declaredReturns, fnDef.Anonymous)
			return []core.Value{raw}
		}
		// Always analyse the body so diagnostics emitted by stepWord
		// (undefined_word, no_signature, …) inside the body propagate
		// up to the parent registry. When the fn declares an explicit
		// return type, we use that for the carrier result and drop
		// the analyser's residual stack — the analyser is run purely
		// for its side-effecting diagnostic collection. Memoisation
		// inside AnalyseFnBody keeps recursive / repeated calls cheap.
		// Bytecode (Stage 3): compile this overload's body as its own
		// code unit (params as frame locals) and record the call site.
		// The memo key mirrors AnalyseFnBody's so the unit is compiled
		// exactly when the body is analysed.
		es := r.Check.Recorder()
		fnUnit := -1
		var finishFn func([]core.Value)
		polyPlan, polyBarred := dispatchPlanUserPoly(r, es, nameCopy, args, declaredReturns)
		// A /q (quote-capture) param binds only a bare Word collected forward
		// at the runtime pointer — a plain stack value never matches it
		// (`def f fn [[k:Atom] [Atom] [k]] f 'meta'` raises no_signature on
		// every engine). Check-mode matching is looser: a STRICT Atom-typed
		// carrier (a lambda callback's element input) satisfies the slot, so
		// a closure-body probe records an application the runtime rejects —
		// compiled `each [[k:Atom] => […]] (keys m)` APPLIED the lambda where
		// the interpreter leaves it data (checker-compiler-completeness-review
		// §2.2). Latch the probe uncompilable: the closure admission declines,
		// the word's Stage-2 refusal stands, and the interpreter's own rule
		// owns the program. Outside closure units every probed /q arrival
		// already models faithfully (top-level no-match parity, the dynamic-
		// carrier recovery refusal), so the guard stays scoped.
		if es.InClosureUnit() && quoteParamCarrierBind(sigParams, args) {
			es.MarkUncompilable("fn " + nameCopy + ": Atom-typed param bound to a computed value in a closure body (quote capture is forward-only)")
		}
		// A FOREIGN-registry fn whose body constructs a fn value USED to
		// refuse wholesale (the compiled unit executed against the
		// dispatching registry, losing module scope for the constructed
		// lambda). Units now carry their owning registry (CompiledFn.Reg):
		// the VM dispatches the unit's natives against it — exactly the
		// registry the interpreter's foreign-wrapper CallBoru runs the body
		// in — so a constructed lambda's downstream capability state (a
		// listener's per-connection registries) forks module scope on both
		// engines, and the refusal is retired (boru:repl rows 12/16/18
		// compile; the remaining statement-position recovery strand refuses
		// through the ordinary provenance paths).
		if es.Armed() && !polyBarred {
			// The body unit must be compiled against GENERALISED args
			// — pure carriers of the call's arg types. The call's
			// kept-concrete values would constant-fold inside the body
			// (`n sub 1` with n=10 folds to 9), baking one call's
			// constants into the shared unit. Same Parents → same memo
			// key, so the generalised analysis is the one that caches.
			genArgs := make([]core.Value, len(args))
			for i, a := range args {
				if i < len(sigParams) {
					if rc, ok := recordSchemaCarrier(sigParams[i], a); ok {
						genArgs[i] = rc // preserve record schema into the armed compile + closure capture
						continue
					}
					if tc, ok := typedContainerCarrier(sigParams[i], a); ok {
						genArgs[i] = tc // D2 Part A: preserve {:T} element type into the body carrier
						continue
					}
					// A RECOVERED call: an Any arg flowed into a CONCRETELY-typed
					// param (matchSignature couldn't statically commit, so dispatch
					// recovered — tryRecordRecoveredUserFn). Compile the body against
					// the DECLARED param type, not strict-Any: the body's words then
					// dispatch against the real type (e.g. `convert Float` over an
					// Integer param inside a chain whose source was an Options `get`),
					// instead of refusing "unmatched dispatch" on strict-Any. Sound by
					// the param contract — SetUnitParamTypes installs a CALL_USER guard
					// that raises == the interpreter when a runtime arg misses the
					// declared type, so assuming it here can only narrow, never admit a
					// value the interpreter would reject.
					if pt := sigParams[i].Type; pt != nil && !pt.Equal(core.TAny) &&
						a.Parent != nil && a.Parent.Equal(core.TAny) && !core.IsBareTypeNode(a) {
						genArgs[i] = core.NewCarrier(pt)
						continue
					}
					// A DISJUNCT carrier — the checker's abstraction of an
					// unresolved overload's returns (a recovered `slice` over a
					// dynamic input yields Bytes|List|String) — narrows to a typed
					// param by the same contract: the entry guard raises exactly
					// where the interpreter's dispatch would, so the body compiles
					// against the declared type instead of a payload-stripped
					// Disjunct carrier no sig accepts (mini-s3: s3-handle-get's
					// `part` into s3-send-resp's `body:Bytes`, whose chunk loop
					// then dispatched `slice` over a Disjunct and refused
					// "for: body nets multiple values"). The dynamic flag is
					// preserved: a gradual disjunct keeps matching optimistically.
					if pt := sigParams[i].Type; pt != nil && !pt.Equal(core.TAny) &&
						a.Carrier && core.IsDisjunct(a) {
						nc := core.NewCarrier(pt)
						nc.Dynamic = a.Dynamic
						genArgs[i] = nc
						continue
					}
					// The complementary case, INSIDE a stored-handler / spawn-
					// body unit compile only (es.storedGradualDepth): an Any arg
					// flowing into an Any (or untyped) param generalises GRADUAL
					// (ParamInputCarrier), not strict — the callee body's
					// dispatch over the param keeps poly-matching optimistically,
					// exactly as the unit's own params do via fnValueInputs; a
					// strict Any refuses "unmatched dispatch recovered" on the
					// first field access (a stored service handler calling an
					// `st:Any` helper that reads `st.kv`). Sound by the same
					// contract as the narrowing above: the VM's per-word poly
					// re-match raises exactly where the interpreter does. Scoped
					// to the stored context because there the probe-then-real
					// discipline makes a gradual-caused refusal per-body (one
					// unstamped handler); applied globally it flipped whole-
					// program main-pass rows (module-repl). Args with CONCRETE
					// parents keep the precise strict generalisation below.
					if pt := sigParams[i].Type; es.StoredGradualActive() &&
						(pt == nil || pt.Equal(core.TAny)) &&
						a.Parent != nil && a.Parent.Equal(core.TAny) && !core.IsBareTypeNode(a) {
						genArgs[i] = ParamInputCarrier(core.TAny)
						continue
					}
				}
				if a.Parent == nil {
					// A root-node carrier (None / Any / Never) has a nil Parent
					// because it IS its own lattice node — it is already an
					// abstract, constant-free generalisation, so keep it (with the
					// Carrier flag set) rather than calling NewCarrier(nil), which
					// would propagate a nil-typed value into the body analysis.
					g := a
					g.Carrier = true
					g.Data = nil
					genArgs[i] = g
					continue
				}
				genArgs[i] = core.NewCarrier(a.Parent)
			}
			// Key the compiled unit on the GENERALISED args, matching the body
			// analysis (AnalyseFnBody runs on genArgs) and the FnSummaries memo. A
			// RECURSIVE call reaches this fn with DIFFERENT concrete args (a sub-spec
			// vs the top spec) but the SAME generalisation, so an args-keyed unit
			// missed the in-flight unit and allocated a SECOND unit that the
			// in-flight bail left EMPTY (the recursive `subspec run-spec` called an
			// empty RET → sub-specs silently skipped). Keying on genArgs makes the
			// recursive call REUSE the in-flight unit (the generalised body that
			// handles any arg shape at run time).
			key := FnAnalysisKey(r.AnalysisScopeID(), nameCopy, genArgs, capturesCopy, bodyCopy)
			paramNames := make([]string, len(sigParams))
			for i, p := range sigParams {
				paramNames[i] = p.Name
			}
			var okFn bool
			// The body's first-token position locates the compiled unit for
			// a return-type error stamped at the VM's RET. It cannot equal
			// the interpreter's call-site column (one unit serves every call
			// site), but it keeps the compiled error from reporting an
			// unknown position. Empty body falls back to a zero pos.
			var fnPos core.SrcPos
			if len(bodyCopy) > 0 {
				fnPos = bodyCopy[0].Pos()
			}
			fnUnit, finishFn, okFn = es.StartFnCompile(key, nameCopy, r, genArgs, compileReturns, paramNames, capturesCopy, genSpec != nil, fnPos)
			if !okFn {
				fnUnit = -1
			}
			if fnUnit >= 0 {
				// Record the declared PARAM types so the VM enforces them at
				// CALL_USER entry (the gradual-Any param-guard, mirroring the RET
				// return-check). A gradual (Dynamic) arg optimistically matched a
				// concrete param at check time; the compiled call must re-check the
				// runtime value, or a laundered mismatch silently runs the body.
				pts := make([]*core.Type, len(sigParams))
				pats := make([]*core.Value, len(sigParams))
				for i := range sigParams {
					pts[i] = sigParams[i].Type
					pats[i] = sigParams[i].Pattern
				}
				es.SetUnitParamTypes(fnUnit, pts, pats)
				// The body tokens: what a per-read deopt hands to the
				// interpreter (compiler planDeopts, NUR123).
				es.SetUnitBody(fnUnit, bodyCopy)
				// The RET-side twin: a declared union return degrades its
				// *Type to Any, so without the pattern the compiled path —
				// the DEFAULT path — enforces nothing while the interpreter
				// and the check pass both reject.
				es.SetUnitReturnPatterns(fnUnit, declaredReturnPatterns)
				// The return-contract declaration site, so a compiled RET
				// return error labels the declaration exactly as the
				// interpreter's ReturnCheck does.
				es.SetUnitDecl(fnUnit, declSite)
			}
			if finishFn != nil {
				// A fresh compilation must RECORD the body into THIS unit — drop any
				// summary cached by a prior analysis (the install-time synthetic
				// example eval, or a DISCARDED closure PROBE compile that shares
				// r.Check.FnSummaries) so AnalyseFnBody re-runs and re-records the
				// body's events instead of returning the cached residual with an
				// empty fragment. The cache (and the AnalyseFnBody call below) is
				// keyed on genArgs, NOT args — deleting the args key left the
				// genArgs-keyed probe summary live, so a fn dispatched under a
				// recursive closure probe (boru:test run-case) compiled to an EMPTY
				// stub unit in the real pass (silent 0-cases miscompile).
				delete(r.Check.FnSummaries, FnAnalysisKey(r.AnalysisScopeID(), nameCopy, genArgs, capturesCopy, bodyCopy))
				stkGen := AnalyseFnBody(r, nameCopy, paramNames, bodyCopy, genArgs, capturesCopy, declaredReturns, fnDef.Anonymous)
				finishFn(stkGen)
			}
		}
		stk := AnalyseFnBody(r, nameCopy, paramNames, bodyCopy, narrowArgsToParams(args, sigParams), capturesCopy, declaredReturns, fnDef.Anonymous)
		stk = collapseTailApply(es, fnUnit, stk)
		stk = collapseElidedTailApply(bodyCopy, stk, fnDef.Anonymous || len(declaredReturns) == 1)
		for i := len(genNames) - 1; i >= 0; i-- {
			r.Defs.Pop(genNames[i])
		}
		if genSpec == nil {
			checkBodyReturnConformance(r, nameCopy, declaredReturns, declaredReturnPatterns,
				unnamedParamCount(sigParams), allConcreteArgs(args), stk,
				bodyStartPos(bodyCopy), bodySpanEnd(bodyCopy))
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
						c.Dynamic = true
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
				// Cloned for a fresh ID (operand-provenance, like the concrete
				// list/map fold in getNodeReturns). Only for a concrete fn
				// residual aligned to this return slot; anything else keeps the
				// carrier below.
				//
				// PLAIN-CHECK ONLY (!Compiling): during a REAL compile pass the
				// emitter models a returned closure through its own PUSH_CLOSURE
				// machinery, and surfacing the concrete FnDefInfo here instead
				// reorders the recorded call residual and detaches the capture
				// from its construction site — the factory-apply / returned-
				// capturing-closure units then refuse ("capture … unreachable",
				// "call results reordered"). The checker's precision win (a
				// bindable, dispatchable closure) is only needed on the plain
				// check pass; the compile pass keeps the carrier and compiles the
				// closure as before.
				if !r.Check.Compiling &&
					t.ConformsTo(core.TFunction) && len(stk) >= len(declaredReturns) {
					if bv := stk[len(stk)-len(declaredReturns)+i]; core.IsConcrete(bv) &&
						bv.Parent.ConformsTo(core.TFunction) {
						out[i] = core.CloneValue(bv)
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
				if dv, ok := core.UnionCarrierForType(core.CanonicalType(r, t)); ok {
					out[i] = dv
					continue
				}
				// NUR068: a declared record return keeps its schema, and a bare
				// Map return surfaces a record-schema body residual — see
				// nur068ReturnCarrier.
				if rc, ok := nur068ReturnCarrier(r, t, declaredReturnPatterns, i, len(declaredReturns), stk); ok {
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
					c.Dynamic = true
				}
				out[i] = c
			}
			if fnUnit >= 0 {
				out = recordUserCallOrApply(es, r, nameCopy, capturesCopy, bodyRef, fnUnit, args, out)
			} else if polyPlan != nil {
				// Ambiguous multi-overload dispatch with every arm baked: record
				// the runtime-re-matched poly call (OpCallUserPoly). Positions
				// where the arms' declared returns AGREE keep the committed
				// sig's carriers; positions where they DIFFER take the plan's
				// return-join carrier (§8.2(3) — the runtime value is whichever
				// arm the VM selects, a branch join), so downstream typing never
				// rides one arm's unproven commitment.
				polyPlan.SubstituteJoinedOuts(out)
				pos := core.SrcPos{}
				if len(args) > 0 {
					pos = args[0].Pos()
				}
				es.RecordUserPolyCall(nameCopy, r, polyPlan.SigIdx(), polyPlan.Units(), polyPlan.Impls(), polyPlan.Sigs(), args, out, pos)
			}
			return out
		}
		if len(stk) == 0 {
			// No declared returns and an empty body residual: the fn produces
			// ZERO values at run time (`def v fn [[x:Integer] [] []]  v 1 99`
			// → [99]; `def r (f 1)` → def_error: nothing to bind). When the body
			// unit compiled, record a 0-output CALL_USER so the residual matches
			// runtime exactly (the call runs for its effects; the next token is
			// the residual) and return zero values. When the unit did NOT compile
			// (fnUnit < 0, plain check mode), keep the lenient Any approximation
			// so downstream provenance refuses and the program falls back.
			if fnUnit >= 0 {
				pos := core.SrcPos{}
				if len(args) > 0 {
					pos = args[0].Pos()
				}
				// nameCopy, not name, for the same reason every sibling in this
				// closure uses it: the snapshot at the top of
				// BuildFnBodyReturnsFn is what the closure is entitled to read.
				noteBakedCallTarget(es, r, nameCopy)
				es.RecordUserCall(fnUnit, args, nil, pos)
				return nil
			}
			// A ZERO-declared-return POLY set (REFUSAL-CLOSURE.0 §6a): every
			// same-arity arm compiled and nets zero (tryCompileUserPolyArms'
			// unitNetsZero gate), so record the runtime-re-matched 0-output
			// poly call — the residual matches runtime exactly whichever arm
			// the VM's MatchSignature selects, and there is no downstream
			// value to type.
			if polyPlan != nil {
				pos := core.SrcPos{}
				if len(args) > 0 {
					pos = args[0].Pos()
				}
				es.RecordUserPolyCall(nameCopy, r, polyPlan.SigIdx(), polyPlan.Units(), polyPlan.Impls(), polyPlan.Sigs(), args, nil, pos)
				return nil
			}
			// The 0-net / undeclared call whose body unit declined leaves a
			// lenient STRICT Any approximation so a downstream consumer that reads
			// the value refuses on unknown provenance and the program falls back.
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
		// residual: the body's residual IS the result. Record the call with
		// those N carriers so downstream resolves them to this dispatch — or,
		// for a construction-scope-capture unit, the fn-VALUE apply fallback
		// (the anonymous-lambda factory result is exactly this arm's shape).
		if fnUnit >= 0 {
			stk = recordUserCallOrApply(es, r, nameCopy, capturesCopy, bodyRef, fnUnit, args, stk)
		}
		return stk
	}
}

// collapseTailApply mirrors, on the call site's analysed residual, the
// fn-value apply a compiled unit's finish lowered at its body tail
// (EmitRecorder.UnitTailApply): the check engine elides the apply word over
// a fn-typed carrier and the identity result flows to the residual, so the
// call site sees [args…, fn] where the unit nets ONE value. The WHOLE
// residual must be the window — the finish lowers the apply word's form
// only over the entire residual, and a lambda leaving a value beneath the
// applied result (`[7 x f/v apply]`) is the interpreter's count error at
// the RET, which a collapsed two-value residual would hide (measured: the
// compiled unit answered [7 8] for the interpreter's "expected 1 return
// value(s), got 2"). The window collapses to one GRADUAL result — the
// apply's own type is unknown. Declines when the residual is not the
// window (another shape, or a memoised residual of one).
func collapseTailApply(es core.EmitRecorder, fnUnit int, stk []core.Value) []core.Value {
	n, ok := es.UnitTailApply(fnUnit)
	if !ok || len(stk) != n+1 {
		return stk
	}
	top := stk[len(stk)-1]
	if !core.IsFnTypedCarrier(top) && !core.IsFnValueResidual(top) {
		return stk
	}
	out := core.NewCarrier(core.TAny)
	out.Dynamic = true
	return []core.Value{out}
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
	if w, err := core.AsWord(body[len(body)-1]); err != nil || w.Name != "apply" {
		return stk
	}
	if !core.IsFnTypedCarrier(stk[len(stk)-1]) {
		return stk
	}
	out := core.NewCarrier(core.TAny)
	out.Dynamic = true
	return []core.Value{out}
}

// checkFnBodyAtConstruction runs a static body pass for each boru-bodied overload
// of a freshly-installed fn, against generalised (carrier) args, so an UNCALLED
// fn's body is still checked (a called fn is additionally checked per call site
// via BuildFnBodyReturnsFn). Check-mode only; bytecode recording suspended; the
// fn name must already be bound (recursion). Generic and Body-less (native /
// handler) overloads are skipped — a generic body needs per-call type bindings,
// and a native handler has no boru body to analyse.
// noteBakedCallTarget records that a compiled unit is about to bake a CALL
// TARGET for module-scope binding `name`. `RecordUserCall(fnUnit, …)` names a
// SPECIFIC compiled unit and the lowering emits CALL_USER / TAIL_CALL_USER to
// it, so a later module-scope rebind of the name leaves the caller invoking
// the old body while the interpreter dispatches the new one. Measured before
// this note existed, on the default lane with no flags:
//
//	def helper fn [[x:Integer] [Integer] [x add 1]]
//	def use fn [[x:Integer] [Integer] [helper x]]
//	use 1  def helper fn [[x:Integer] [Integer] [x add 100]]  use 1
//	  interpreted -> 2 101      compiled -> 2 2
//
// It is a FUNCTION, and every RecordUserCall site calls it, because the bug
// this closes was caused by exactly the opposite arrangement one layer down:
// `NotifyNameRebound` used to be invoked from individual def/undef HANDLERS,
// so each binder that returned early skipped it silently. (Both halves are
// funnelled now — core/go/rebind_notify.go, gated by
// `TestBindingOpsNotifyRebind`.) The first cut of this note repeated that
// mistake before the funnels existed: it sat inline at recordUserCallOrApply
// and missed the ZERO-OUTPUT site below, which kept
// `def g fn [[] [] [print 1]]  def f fn [[] [] [g]]  f
// def g fn [[] [] [print 99]]  f` printing `1 1` against the interpreter's
// `1 99`. If a third RecordUserCall site appears, it calls this.
//
// The registry is the DISPATCH's own (r), never the recorder's es.reg: es.reg
// is the last registry BindRegistry saw, which after a call into a
// boru-implemented module is that module's sub-registry, whose baselines are
// not this call's (region_record.go carries a dispatch registry for the same
// reason). NoteFrozenRead self-guards on an open unit, so a TOP-LEVEL call
// records nothing — there analysis order is program order and the interpreter
// makes the same dispatch.
func noteBakedCallTarget(es core.EmitRecorder, r *core.Registry, name string) {
	if r == nil || name == "" {
		return
	}
	if core.ModuleScopeBinding(r, name) {
		es.NoteFrozenRead(name, core.FrozenBakeCall, r.Defs.Gen(name))
	}
}

// recordUserCallOrApply records a compiled-unit dispatch at a ReturnsFunc
// record site: the §4.3 fn-value apply fallback where the call qualifies
// (the outs slice is then COPIED with the freshened carrier in slot 0),
// else the ordinary RecordUserCall. Returns the outs to hand downstream.
func recordUserCallOrApply(es core.EmitRecorder, r *core.Registry, name string, captures []core.CapturedBinding, body []core.Value, fnUnit int, args, outs []core.Value) []core.Value {
	pos := core.SrcPos{}
	if len(args) > 0 {
		pos = args[0].Pos()
	}
	// The pending closure apply first: a `/v` read of a def-bound produced
	// closure (`3 p/v apply`, the thirty-first increment) is both a pending
	// entry (its read resolves to the closure's producer) and a name the
	// fallback could re-dispatch by; the pending route runs the closure
	// VM-native where the fallback's name lookup islands.
	if fresh, ok := recordPendingClosureApply(es, body, args, outs, pos); ok {
		outs = append([]core.Value(nil), outs...)
		outs[0] = fresh
		return outs
	}
	if fresh, ok := recordFnValueApplyFallback(es, r, name, captures, args, outs, pos); ok {
		outs = append([]core.Value(nil), outs...)
		outs[0] = fresh
		return outs
	}
	noteBakedCallTarget(es, r, name)
	es.RecordUserCall(fnUnit, args, outs, pos)
	return outs
}

// recordPendingClosureApply routes the re-step dispatch of a closure this
// pass PRODUCED, handed back by the `apply` word (`99 (kk 7) apply` — the
// twenty-eighth increment), through the fn-VALUE apply: the recorder holds
// the applied value under a PENDING entry registered at apply's own
// dispatch (PendingClosureApply, matched by the sig body's backing array —
// the value's own sig, not a copy: a body whose only token is a `=>` group
// carries no position to match by), and RecordDynApply
// over that value resolves the fn to the closure's producer operand — the
// runtime payload with the captures it carries — where RecordUserCall could
// only refuse the construction-scope captures unreachable at this call
// site. Single out, freshened as the §4.3 fallback freshens (the memoised
// residual is shared across calls of one shape); a concrete out takes a
// carrier of its type — the runtime value is the apply's, sound at the cost
// of a fold — except a fn VALUE, which keeps its payload under a fresh id
// (see below). The args arrive in signature order (sig[0] the stack top);
// RecordDynApply takes the window deepest-first with sig[0] on top, so they
// are reversed. A 0-arg closure declines (nothing beneath to lay out — the
// entry stays for the finish to refuse), as does whatever RecordDynApply's
// own guards decline.
func recordPendingClosureApply(es core.EmitRecorder, body, args, outs []core.Value, pos core.SrcPos) (core.Value, bool) {
	if len(outs) != 1 || len(args) == 0 {
		return core.Value{}, false
	}
	fn, ok := es.PendingClosureApply(body)
	if !ok {
		return core.Value{}, false
	}
	var fresh core.Value
	if _, isFn := outs[0].Data.(core.FnDefInfo); isFn {
		// A fn VALUE result (the next closure of a staged combinator) keeps
		// its analysed payload under a fresh identity: a later `apply` over
		// it re-steps the value and records through this same arm, resolving
		// the fn to THIS apply's event.
		fresh = outs[0]
		fresh.ID = core.GenerateID(core.IDPrefixForType(core.TFunction))
	} else {
		parent := outs[0].Parent
		if parent == nil {
			parent = core.TAny
		}
		fresh = core.NewCarrier(parent)
		fresh.Dynamic = outs[0].Dynamic
	}
	window := make([]core.Value, len(args))
	for i, a := range args {
		window[len(args)-1-i] = a
	}
	if _, ok := es.RecordDynApply(window, fn, fresh, pos); !ok {
		return core.Value{}, false
	}
	return fresh, true
}

// recordFnValueApplyFallback routes a call whose compiled unit carries
// CONSTRUCTION-SCOPE (non-concrete) captures through the fn-VALUE apply
// (RecordDynApply) instead of the unit call. Such a unit's captures are
// analysis carriers resolvable only in the factory body's own scope, so
// RecordUserCall refuses "capture X unreachable at a call site" at every
// OUTER call site (the audit's §4.3 capture family) — while the STORED
// runtime value carries its own concrete captures, which the dynamic
// apply's interpreter-dispatch island installs faithfully (CallBoru's
// Captured install). Deliberately narrow, each guard load-bearing:
//
//   - single arg, single out — RecordDynApply's op nets exactly one
//     value, and a 1-arg window cannot be reordered by the op's
//     trailing/forward normalisation;
//   - the binding's value is an ANONYMOUS, single-own-sig, unquoted fn —
//     the island's execFnDefLiteral provably APPLIES that class (a named
//     or quoted value can stay data there where the word dispatch
//     applies — ADR-016's divergence);
//   - at least one capture non-concrete — a fully-concrete-capture unit
//     keeps the established unit call.
//
// RecordDynApply's own guards (operand provenance, the event-lead
// quote-state refusal, fnConcreteSingleValuedOrCarrier) still apply; a
// decline or refusal there leaves the program on the sound fallback.
// Pinned end-to-end by frontier-hof-audit.tsv §9's mkap row.
func recordFnValueApplyFallback(es core.EmitRecorder, r *core.Registry, name string, captures []core.CapturedBinding, args, outs []core.Value, pos core.SrcPos) (core.Value, bool) {
	if len(args) != 1 || len(outs) != 1 {
		return core.Value{}, false
	}
	// The memoised body analysis returns the SAME residual value (same ID)
	// for every call of one shape, so recording each apply against the
	// shared out would overwrite producedBy and resolve every residual
	// slot to the LAST apply (probe: `(h 5) (h 10)` compiled [17 17]).
	// Freshen a CARRIER out per call site — the caller substitutes it —
	// and decline a concrete out (freshening would erase its value).
	if !outs[0].Carrier || outs[0].Parent == nil {
		return core.Value{}, false
	}
	nonConcrete := false
	for _, cb := range captures {
		if !core.IsConcrete(cb.Value) {
			nonConcrete = true
			break
		}
	}
	if !nonConcrete {
		return core.Value{}, false
	}
	top, ok := r.Defs.Top(name)
	if !ok {
		return core.Value{}, false
	}
	fd, isFn := top.Data.(core.FnDefInfo)
	if !isFn || !fd.Anonymous || top.Quoted {
		return core.Value{}, false
	}
	if own := fd.OwnSigs(); len(own) != 1 {
		return core.Value{}, false
	}
	fresh := core.NewCarrier(outs[0].Parent)
	fresh.Dynamic = outs[0].Dynamic
	if !es.RecordDynApplyName(name, args, top, fresh, pos) {
		return core.Value{}, false
	}
	return fresh, true
}

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
		// ISOLATE the analysis in a throwaway EmitState (not just Suspend): the
		// body is analysed against the DECLARED param types, which for an abstract
		// param (a surface like `s:Shape`) takes the surface-shape dispatch path
		// and calls MarkUncompilable. Suspend stops recording but does NOT prevent
		// MarkUncompilable from latching the program's EmitState.Compilable — so a
		// construction-time check of a fn with a surface param would wrongly mark
		// the WHOLE program uncompilable (surface.tsv:32 refused for exactly this).
		// IsolateEmit swaps a fresh EmitState for the analysis (discarded on
		// restore), so MarkUncompilable / recording land on the throwaway; the
		// emitted DIAGNOSTICS (undefined_word for an uncalled body typo, the strand
		// advisory) live on r.Check.Diagnostics and are unaffected — exactly the
		// diagnostics-only contract this pass needs.
		restore := r.Check.IsolateEmit()
		before := len(r.Check.Diagnostics)
		stk := AnalyseFnBody(r, name, paramNames, s.Body(), genArgs, fnDef.Captured, declared, fnDef.Anonymous)
		restore()
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
		// failed look is silence — the compile path still refuses these
		// programs, and loudly.
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
