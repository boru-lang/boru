package compiler

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// Code-body closure compilation (plan P2): a higher-order word whose body is a
// quoted code list compiles that body to its own fn unit, and the dispatch
// records the body operand as a closure (OpPushClosure) instead of a Stage-5
// interpreter island. At run time the word's native handler invokes the
// closure through the VM's re-entrant runner (vmContext.invokeClosure) via the
// InvokeBody seam — no interpreter sub-engine.

// The closure-eligible set is NOT a name-keyed table in eng. Each such word
// DECLARES its closure shape on its NativeFunc as a *CallableSpec (BodyPos /
// BodyOut / Inputs), which RegisterNativeFunc copies onto every signature; the
// recorder reads it back via the resolved sig.Callable. eng therefore names no
// specific word — the core transforms (each / fold / scan / do / filter /
// with-decimal) declare in lang/native, and module words (the boru:test case /
// describe bodies) declare in their own module, with no eng↔module coupling.
// See eng/go/value.go::CallableSpec and review §4.5.

// compileClosureBody compiles a code body (bodyToks) consuming the given input
// carriers into its own fn unit, returning (unitIndex, ok). The body is
// recorded into the CURRENT EmitState (r.Check.Emit) — callers run it once in
// a throwaway probe state to test compilability, then once in the real state.
// Mirrors the fn-def compile path (core_helpers.go): StartFnCompile arms the
// unit, AnalyseFnBody records the body under it, finish closes it. ok is false
// when the body refuses (StartFnCompile declined, or the analysis marked the
// state uncompilable).
// paramNames is the per-input name table: a NAMED slot (a lambda param,
// `([p] => …)`) binds the body's `p` to that input carrier in AnalyseFnBody;
// an empty name (the token-quotation form, `[body]`) leaves the input on the
// stack for the body to consume positionally. nil means all-unnamed.
func compileClosureBody(r *core.Registry, word string, bodyOut int, emptyBodyOK bool, bodyToks, inputs []core.Value, paramNames []string, paramPatterns []*core.Value, captures []core.CapturedBinding, shape core.ClosureInShape, pos core.SrcPos) (int, bool) {
	// Closure compilation is emit-cluster machinery: it writes recording
	// internals (fnRecs), so it needs the CONCRETE EmitState. A pass without
	// one (the inactive recorder) declines exactly as the nil field did —
	// the typed-nil receiver's StartFnCompile returns !ok below.
	es, _ := r.Check.Recorder().(*EmitState)
	// A side-effect body word (bodyOut 0 — a test case body) declares NO returns,
	// so its 0-value residual is taken as-is rather than count-refused against the
	// default single [TAny]. The fn-VALUE factory path (word "fnval") passes
	// bodyOut 1, so it keeps the single return. An EmptyBodyErrors word
	// (each/fold/scan) also declares no returns even at bodyOut 1: a 0-net body is
	// the handler's OWN runtime error, raised faithfully at invoke time, so the
	// closure is left count-agnostic rather than count-refused (which would
	// island).
	declared := []*core.Type{core.TAny}
	if bodyOut == 0 || bodyOut == core.BodyOutResidual || emptyBodyOK {
		// A side-effect body (0), a whole-residual body (do — the handler
		// returns everything the body nets, so the count is per-body), and an
		// EmptyBodyErrors body are all count-AGNOSTIC: declared nil skips the
		// closure count refusal and the unit RETs its actual residual.
		declared = nil
	}
	if paramNames == nil {
		paramNames = make([]string, len(inputs)) // all unnamed: body reads inputs off the stack
	}
	name := word + "$body"
	key := check.FnAnalysisKey(r.AnalysisScopeID(), name, inputs, captures, bodyToks)
	unit, finish, ok := es.StartFnCompile(key, name, r, inputs, declared, paramNames, captures, false, pos)
	if !ok {
		return -1, false
	}
	// Record the closure's input convention on the unit (consistent across a
	// memo hit: the key includes name+input types, which determine the shape).
	es.fnRecs[unit].inShape = shape
	es.fnRecs[unit].closure = true
	es.fnRecs[unit].lambdaUnit = word == "fnval"
	// A lambda VALUE's declared param PATTERNS ride on the record from the
	// open, not only from the contract seated after the compile
	// (SetUnitParamTypes): the unit's finish reads them to decide whether
	// the lambda may take the fn path's residual replay (plainLambda).
	if paramPatterns != nil {
		es.fnRecs[unit].paramPatterns = paramPatterns
	}
	// The two stored-ref compile paths use these eng-internal synthetic
	// names; their rebind safety is the per-ref poisoning, so the frozen-
	// read discipline skips them (see fnUnitRec.storedRefUnit).
	es.fnRecs[unit].storedRefUnit = word == "storedfn" || word == "spawnbody"
	// A code body a native runs inside the caller's frame (each, do, fold,
	// for, …) seats its tokens for a per-read deopt (planDeopts, NUR123).
	// A LAMBDA VALUE seats them too: the frame it resumes in is its OWN —
	// live for the whole apply, with its captures in slots — not the
	// factory's, which is gone by then (planDeopts routes it through
	// lambdaNamesSelfBound instead of seedParentDeopt for exactly that
	// reason). A stored fn or a spawned body is invoked by the host outside
	// any such call and seats none.
	if word != "storedfn" && word != "spawnbody" {
		es.SetUnitBody(unit, bodyToks)
	}
	if finish == nil {
		// Memo hit: the unit is already compiled in this state.
		return unit, es.Active()
	}
	// Drop any summary a suspended (non-recording) analysis cached so the
	// body re-runs under the armed unit and records.
	delete(r.Check.FnSummaries, key)
	// true preserves this callback-body path's original recording gate
	// (!anonymous is neutralised): the fn-context change flows through the
	// def/dispatch paths, not this closure-body compile.
	stk := check.AnalyseFnBody(r, name, paramNames, bodyToks, inputs, captures, declared, true)
	if len(bodyToks) == 0 && len(stk) == 0 {
		// An EMPTY body's residual is its pushed inputs, verbatim: the runtime
		// InvokeBody pushes the per-call inputs and runs no tokens, so the frame
		// nets exactly them — `error []` leaves the caught error (pass-through),
		// `each []` leaves the element (identity map). AnalyseFnBody early-
		// returns nil for an empty body, which compiled a RET-nothing unit that
		// DIVERGED from the interpreter (each_error "body produced no result"
		// against the interpreter's identity map). Reconstruct the residual so
		// the unit resolves each input to its param slot (PUSH_LOCAL i … RET).
		stk = append(stk, inputs...)
	}
	finish(stk)
	return unit, es.Compilable
}

// mutableInstanceRef reports whether v is (or stands in for) a MUTABLE
// instance — a concrete flex node / class instance, or a check-mode CARRIER
// typed at one (the binding a `def acc (flex […])` holds during analysis is a
// TFlexList carrier, not the concrete store). Bare type nodes are excluded:
// a class/flex TYPE literal is not an instance.
func mutableInstanceRef(v core.Value) bool {
	if core.IsBareTypeNode(v) {
		return false
	}
	if core.IsFlexNode(v) || core.IsClassInstance(v) {
		return true
	}
	p := v.Parent
	if p == nil {
		return false
	}
	if p.ConformsTo(core.TFlexMap) || p.ConformsTo(core.TFlexList) || p.ConformsTo(core.TFlexXml) {
		return true
	}
	return v.Carrier && p.ConformsTo(core.TClass)
}

// moduleScopeMutableCaptures extends a closure body's lexical captures with
// MODULE-SCOPE bindings the body reads whose values are MUTABLE INSTANCES
// (flex nodes, class instances). Per the language these references are
// DYNAMIC — module scope sits below the fn baseline, so ComputeCaptures
// excludes them — and for const-bakeable data the compiled body's const bake
// IS the dynamic read. A mutable instance cannot bake (materialise refuses;
// the body then refused "code-body word … (Stage 2)"), but its IDENTITY is
// fixed for the whole dispatch: a compiled body cannot rebind a module-scope
// name (body defs are body-local and module-mutating meta words refuse
// compilation), so the value passed at OpPushClosure equals every per-run
// lookup the interpreter makes — a capture is exact. This is the accumulator
// shape: `def acc (flex [0])  … each [ var [[x] (acc set 0 …)] ]`.
// Scoped to mutable instances ONLY — fn values, module exports, and bakeable
// consts keep their existing paths.
func moduleScopeMutableCaptures(r *core.Registry, bodyToks []core.Value, existing []core.CapturedBinding) []core.CapturedBinding {
	have := map[string]bool{}
	for _, cb := range existing {
		have[cb.Name] = true
	}
	bodyLocals := map[string]bool{}
	core.CollectBodyLocalDefs(bodyToks, bodyLocals)
	out := existing
	seen := map[string]bool{}
	core.WalkBodyWords(bodyToks, func(w core.WordInfo, _ core.Value) {
		name := w.Name
		if name == "" || have[name] || bodyLocals[name] || seen[name] {
			return
		}
		seen[name] = true
		v, ok := r.Defs.Top(name)
		if !ok {
			return
		}
		if !mutableInstanceRef(v) && !moduleScopeInstanceCarrier(v) {
			return
		}
		out = append(out, core.CapturedBinding{Name: name, Value: v})
	})
	return out
}

// moduleScopeInstanceCarrier reports whether v is an IMMUTABLE module-scope
// INSTANCE carrier — a non-concrete carrier that a module-scope `def` binds (a
// persistent TrieMap/TstMap, a built collection, any instance a module
// constructor returns; its declared type may be Map/List or an under-annotated
// Any). Module scope sits below the fn baseline, so ComputeCaptures excludes
// it, and it is NOT const-bakeable (materialise refuses a non-concrete carrier)
// — yet it IS produced by a module-scope event, so it resolves at the call site
// (the closure's construction, at module scope) while its event is unreachable
// inside the body. Riding it as a capture slot is exact for the same reason the
// mutable-accumulator capture is: a compiled body cannot rebind a module-scope
// name (body defs are body-local; module-mutating meta words refuse), so the
// value threaded at OpPushClosure equals every per-run lookup the interpreter
// makes. A carrier with no producing event in the emit tables declines later in
// recordClosureDispatch (capOps resolveOperand) — sound fallback, and the
// ordinary path for a FOREIGN body's captures, whose events live elsewhere.
//
// A concrete value still const-bakes (excluded here); a bare type node is a
// type, never a carried instance (excluded). A Dynamic (gradual-Any) carrier is
// INCLUDED: an under-annotated module constructor (TstMap.make) binds its
// instance as a Dynamic Any, yet the binding is still fixed for the program.
// The capture only makes the value reachable in the body; any downstream
// dispatch on it (an `each` over a gradual collection) still refuses at its own
// ambiguity gate, so admitting the capture never introduces an unsound dispatch
// — it just resolves the operand.
func moduleScopeInstanceCarrier(v core.Value) bool {
	return !core.IsBareTypeNode(v) && !core.IsConcrete(v) && v.Carrier
}

// tryRecordClosure attempts to compile a code-body higher-order word's body to
// a closure unit and record a normal dispatch (the body operand lowering to
// OpPushClosure). Returns true on success. A body that does not compile leaves
// the REAL emit state untouched — the probe runs in a throwaway state — so the
// caller falls through to the island path.
func tryRecordClosure(r *core.Registry, word string, sig *core.Signature, args, outs []core.Value, pos core.SrcPos) bool {
	if sig == nil || sig.Callable == nil {
		return false
	}
	spec := *sig.Callable
	// Concrete EmitState needed (extraNoEvalHookSlotsOK reads recording
	// internals); the inactive recorder falls to the !active() decline.
	es, _ := r.Check.Recorder().(*EmitState)
	// 0 or 1 outputs (a side-effect case body nets 0; a map/transform body nets
	// 1) — RecordClosureCall lowers both. A whole-residual word (BodyOutResidual:
	// `do` returns the body's ENTIRE residual) admits any statically-exact count:
	// the closure compiles count-agnostic, the VM's frameless RET returns the full
	// residual, and the dispatch seats all N results. Other multi-output words
	// stay beyond this path.
	if !es.Active() || (len(outs) > 1 && spec.BodyOut != core.BodyOutResidual) || spec.BodyPos >= len(args) {
		return false
	}
	body := args[spec.BodyPos]

	// A SECOND NoEvalArgs hook slot (walk's optional ASCEND hook — the only
	// such slot on a Callable word today) rides as a plain VALUE operand next
	// to the compiled body closure, and the driving handler runs it as a token
	// QUOTATION through a sub-engine at run time. That is sound only for the
	// one provably-name-free shape extraNoEvalHookSlotsOK admits, plus — the
	// Stage M2d extension — a LAMBDA hook on a LambdaSharesTokenShape word,
	// which compiles to its OWN closure unit inside recordClosureDispatch
	// (walkClassifyHook already classifies a compiled closure per hook slot).
	// Anything else declines here so the Stage-2 refusal (the sound
	// interpreter fallback) stands.
	extraLamSlots, extrasOK := es.extraNoEvalHookSlots(sig, spec, args)
	if !extrasOK {
		return false
	}

	// A gradual-Any (Dynamic) DATA arg to a higher-order word whose overloads
	// differ on that arg's type (each/fold/scan: TList vs TMap) is an AMBIGUOUS
	// dispatch: the checker optimistically committed to ONE overload, but the
	// runtime value could need the SIBLING — a gradual collection bound to each's
	// Map overload errors ("expected concrete map") where the interpreter iterates
	// the runtime List. The body arg is never Dynamic, so anyDynamicCarrier here
	// means a DATA arg is. The ≥2-reachable gate is INHERENTLY token-form-specific:
	// a TList token body matches BOTH the {…,TList} and {…,TMap} overloads (count
	// 2), while a TFunction lambda body matches only the single {TFunction,TMap}
	// overload (count 1) — so a lambda never reaches here.
	//
	// For a CrossCollectionTokenShape word the token body is shape-generic (both
	// overloads present the bare value), so we DON'T refuse: fall through to record
	// the first-reachable (List) overload's closure ONCE, relying on the committed
	// handler being runtime-robust to the sibling collection type (it delegates to
	// the map/list iteration by the value's concrete type), so the same closure
	// drives either shape == the interpreter. A non-robust word (no flag) still
	// refuses → sound interpreter fallback.
	_, bodyIsLambda := body.Data.(core.FnDefInfo)
	tokenShapeGeneric := spec.CrossCollectionTokenShape && core.IsConcrete(body) && !bodyIsLambda
	if check.AnyDynamicCarrier(args) && check.DynamicReachableOverloadCount(r, word, args) >= 2 && !tokenShapeGeneric {
		// A CompileDynBody word DECLINES instead: tryRecordDynBody records a
		// POLY re-match over the word's own sigs — the runtime value picks
		// the overload exactly as the interpreter's dispatch does.
		if sig.CompileEffect.Has(core.CompileDynBody) {
			return false
		}
		es.MarkUncompilable("higher-order `" + word + "` over a gradual-Any collection: ambiguous overload (List vs Map), no static commit and no poly re-match")
		return true
	}

	// A lambda VALUE body (`filter ([p:Any] => …) data`): the afn's named
	// param binds to the WORD'S callback shape ({key,value} pair for list
	// filter, KeyVal for the map forms), not to its declared `Any`. Compile
	// the lambda body against that representative shape so `p.value`/`kv.v`
	// typechecks, then record the dispatch with the body as a closure the
	// handler drives through InvokeBody.
	if fd, isFn := body.Data.(core.FnDefInfo); isFn {
		return tryRecordLambdaClosure(r, word, spec, sig, args, &fd, body.Pos(), extraLamSlots, outs, pos)
	}

	// A token-list body (`filter [body] data`): the body consumes its inputs
	// positionally off the stack.
	if !core.IsConcrete(body) {
		return false
	}
	bodyList, err := core.AsList(body)
	if err != nil || bodyList.IsNil() {
		return false
	}
	// A body with a flow-control sentinel (break/continue/return) targets an
	// enclosing loop the VM can't reach across the call boundary — keep it on
	// the whole-program fallback path. The scan is DEEP: a body word resolving
	// to a user fn whose body transitively holds a bare break/continue leaks
	// the signal through the call (`do [f 1]` with a breaking f unwinds the
	// caller's loop in the interpreter; the closure unit starts a fresh loop
	// stack and surfaces internal flow-signal errors — a miscompile). The
	// decline is conservative-but-parity: callback words that do NOT thread
	// the signal (each/filter — the interpreter raises `break outside loop`)
	// fall back to the interpreter, which raises identically.
	if check.BodyHasSentinelDeep(r, body) {
		return false
	}
	inputs := spec.Inputs(args)
	if inputs == nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return false
	}
	bodyToks := bodyList.Slice()

	// Lexical captures: body words resolving to an ENCLOSING fn's binding
	// (a param or body-local of a fn currently being compiled) ride as the
	// closure's captures — resolved here in the enclosing scope, bound into
	// the body unit's trailing slots at invocation. A module/global ref is
	// not a capture (it bakes as a const in the body, or refuses the probe).
	captures := moduleScopeMutableCaptures(r, bodyToks, core.ComputeCaptures(r, &core.FnSig{Impl: core.Boru(bodyToks)}))
	// A multi-run defs-keeping body (`each`) opens the arm-resident
	// bracket around its compiles: `_`-named body defs get their event
	// seats (RecordDynBind's gate), and the probe fork inherits the
	// bracket so probe and real admit the same population.
	if spec.BodyMultiRunKeepsDefs && es != nil {
		es.armResidentDepth++
		defer func() { es.armResidentDepth-- }()
	}
	// The body's compile re-run runs in the environment its analysis run
	// STARTED from (unit_memo.go): claimed here, applied around every
	// compile inside recordClosureDispatch.
	env := es.takeBodyEnv(body, spec)
	if !recordClosureDispatch(r, word, spec, sig, args, bodyToks, inputs, nil, captures, ClosureInValue, extraLamSlots, outs, nil, nil, pos, env) {
		return false
	}
	// A once-run defs-keeping body (`do`) compiled to a closure unit makes
	// its defs frame-local, so the leak the interpreter delivers needs its
	// twins PLACED after the call — the regime's replay account
	// (AdoptBodyTwins; a no-op for the flag-less multi-run words).
	if spec.BodyOnceKeepsDefs {
		es.AdoptBodyTwins(body)
	}
	// A multi-run body's twins instead bridge INTO the unit — per-element
	// installs with runtime values (AdoptResidentTwins; every fence
	// declines to the standing refusal).
	if spec.BodyMultiRunKeepsDefs {
		es.AdoptResidentTwins(body)
	}
	return true
}

// tryRecordLambdaClosure compiles a higher-order word's LAMBDA argument
// (`([p] => …)`, an anonymous FnDefInfo) to a closure unit. The lambda's body
// is compiled with the word's per-callback input shape (lambdaCallbackInputs)
// bound to the lambda's NAMED params, so a body that destructures the entry
// (`p.value`, `kv.v`, `acc`+`kv.v`) typechecks. Returns false — leaving the
// refusal to stand — for a shape the word has no lambda convention for, an
// arity mismatch, or a body that does not compile.
func tryRecordLambdaClosure(r *core.Registry, word string, spec core.CallableSpec, sig *core.Signature, args []core.Value, fd *core.FnDefInfo, fnPos core.SrcPos, extraLamSlots []int, outs []core.Value, pos core.SrcPos) bool {
	inputs, shape, ok := lambdaCallbackInputs(r, word, spec, args)
	if !ok {
		return false
	}
	// The BODY lambda admits LEXICAL captures (allowCaptures): the mini-redis
	// KEYS shape — `def kv state.kv  filter ([e:Any] => [… kv …]) (keys kv)` —
	// captures an enclosing-fn local. Each capture resolves to a compiled home
	// via resolveOperand in recordClosureDispatch (an unresolvable one declines
	// cleanly), rides the stack at OpPushClosure, and binds a trailing unit
	// slot in invokeClosureOn — value-identical to the interpreter's
	// construction-time snapshot, taken at the same program point per dispatch.
	lam, ok := lambdaHookCompatible(r, fd, inputs, shape, true, true)
	if !ok {
		return false
	}
	names := make([]string, len(lam.Params))
	for i := range lam.Params {
		names[i] = lam.Params[i].Name
	}
	// Module-scope MUTABLE-INSTANCE reads in the lambda body (the flex
	// accumulator `def acc (flex [])  walk … (m => [acc (m.path) append])`)
	// ride as closure captures, exactly as the token-body path admits them:
	// the binding's identity is fixed for the dispatch (a compiled body cannot
	// rebind a module-scope name), so the value threaded at OpPushClosure
	// equals every per-run lookup the interpreter's CallBoru makes, and the
	// pointer-backed instance shares mutations across the boundary. See
	// moduleScopeMutableCaptures. The lambda's LEXICAL captures come first
	// (already name-sorted per the CapturedBinding contract); the module-scope
	// additions append after, and the unit binds trailing slots in this same
	// merged order.
	// MODULE-SCOPE mutable captures resolve where the BODY was written, so a
	// foreign body's are looked up in its own registry — see the foreignFnHome
	// arm below for why the two registries split.
	capReg := r
	if foreignFnHome(r, fd) {
		capReg = fd.Registry
	}
	captures := moduleScopeMutableCaptures(capReg, lam.Body(), fd.Captured)
	// A fn value DEFINED in another module resolves its free words THERE
	// (design/FUNCTION-VALUE-SCOPE.0.md). Compile its body against that
	// registry, with the CALLER's CheckState shared onto it: the split is
	// exactly two roles the registry plays, and they go different ways.
	//
	//   BINDINGS  the foreign registry — `lim` inside module A's predicate
	//             must be A's `lim`, not the importer's. StartFnCompile takes
	//             the same registry, so rec.reg is foreign and the unit gets
	//             CompiledFn.Reg stamped; the VM's curReg swap then resolves
	//             the body's runtime dispatches there too.
	//   ANALYSIS  the caller's CheckState — params, carriers and above all the
	//             RECORDER. The unit must land in the CALLER's program, since
	//             that is where OpPushClosure references it.
	//
	// Measured: without the share, the body compiles to `PUSH_CONST false;
	// RET` — the predicate const-folds away, `filter A.big [1 2 3 4]` answers
	// [] against the interpreter's [3 4]. With it, the body compiles to the
	// real predicate and A's `lim` compiles as its own unit returning 2
	// (§6.3, design/FULL-COMPILATION.0.md).
	//
	// ShareCheckStateFrom is the same mechanism execFnDefLiteral already uses
	// to run a module fn's body on the importer's engine, restore contract and
	// nesting idempotence included.
	if foreignFnHome(r, fd) {
		restore := check.ShareCheckStateFrom(fd.Registry, r)
		defer restore()
		return recordClosureDispatch(fd.Registry, word, spec, sig, args, lam.Body(), inputs, names, captures, shape, extraLamSlots, outs, fnValueRetSpec(fd, lam, fnPos), lamParamContract(lam), pos, nil)
	}
	// A lambda body is a fn body: its defs are frame-locals and nothing
	// leaks, so it needs no re-run environment.
	return recordClosureDispatch(r, word, spec, sig, args, lam.Body(), inputs, names, captures, shape, extraLamSlots, outs, fnValueRetSpec(fd, lam, fnPos), lamParamContract(lam), pos, nil)
}

// lamParamContract is a lambda's declared PARAM contract — the types and
// patterns MatchSignature reads — recorded on its closure unit
// (CompiledFn.Params / ParamPatterns, SetUnitParamTypes) so a runtime that
// must dispatch the closure BY NAME can declare the same signature the
// interpreter's frame binding carries (the VM's closureAsWord bridge,
// NUR123): a `z:Integer` lambda read as the word `g` and handed a String
// must no-match there exactly as it does on the interpreter. A param with
// no type (a pattern-only param) declares Any with its pattern. Nil for a
// nil signature.
func lamParamContract(lam *core.Signature) *ClosureParamSpec {
	if lam == nil {
		return nil
	}
	spec := &ClosureParamSpec{Types: make([]*core.Type, len(lam.Params)), Patterns: make([]*core.Value, len(lam.Params))}
	for i, p := range lam.Params {
		spec.Types[i] = p.Type
		if spec.Types[i] == nil {
			spec.Types[i] = core.TAny
		}
		spec.Patterns[i] = p.Pattern
	}
	return spec
}

// ClosureParamSpec is a closure unit's declared param contract, seated by
// recordClosureDispatch / tryReturnedClosure (lamParamContract).
type ClosureParamSpec struct {
	Types    []*core.Type
	Patterns []*core.Value
}

// paramSpecPatterns is a nil-safe read of a contract's patterns.
func paramSpecPatterns(ps *ClosureParamSpec) []*core.Value {
	if ps == nil {
		return nil
	}
	return ps.Patterns
}

// foreignFnHome reports whether fd is a fn VALUE that was DEFINED in another
// module — its `Registry` is set and is not the registry being compiled.
//
// It is a QUESTION about where the body's free words resolve, not a verdict.
// Such a body resolves them in the DEFINING module
// (design/FUNCTION-VALUE-SCOPE.0.md): that is what `execFnDefLiteral` does at
// runtime and what the CallBoruFn / InvokeCallbackFn seams do for every native
// callback. Compile the body against `r` — the module doing the CALLING — and
// it bakes in whatever `r` happens to bind for those names, so `filter P.big
// xs` returns one answer interpreted and another compiled, with no diagnostic
// either way. That was measured, not assumed: the caller's `lim` (100) baked in
// and the row answered [] where the interpreter answers [3 4].
//
// So every use of this predicate must ANSWER the question, one of two ways:
//
//   - COMPILE THE BODY IN ITS OWN HOME. tryRecordLambdaClosure does this for
//     the BODY lambda: recordClosureDispatch takes fd.Registry, and the
//     caller's CheckState is shared onto it so the recorder still writes into
//     the calling program. See the comment at that call site for the split.
//   - DECLINE. The refusal falls through to the runtime callback path, which
//     runs the body on fd.Registry — the same place the interpreter runs it —
//     so the answers match. This is what the extras/hook path does: its
//     shared-token-shape hooks have no per-hook registry to swap to.
//
// The same question reaches MODULE-SCOPE MUTABLE CAPTURES, and it has the same
// two answers. tryRecordLambdaClosure looks them up in fd.Registry for a
// foreign body; a foreign cell then has no producing event in the CALLER's
// emit tables, so resolveOperand declines and the call falls back. Looking
// them up in the caller instead compiles a closure over the WRONG cell
// whenever the two modules share a name — measured as a silent wrong answer,
// pinned by lang/go TestForeignClosureCaptureResolvesInItsOwnRegistry.
//
// What is NOT allowed is to ignore the answer and compile against `r` anyway.
// Both callers take fd from `args[i].Data.(core.FnDefInfo)` and pass its
// address, so fd is never nil here.
func foreignFnHome(r *core.Registry, fd *core.FnDefInfo) bool {
	return fd.Registry != nil && fd.Registry != r
}

// lambdaHookCompatible reports whether a LAMBDA hook value can compile to a
// closure unit against the word's callback `inputs`, returning its single own
// signature. The constraints are the landed lambda-body ones, shared by the
// BODY lambda and the extra hook-slot lambdas (Stage M2d):
//
//   - exactly ONE own signature: an OVERLOADED fn value is dispatched by
//     MatchFnSig at runtime — FirstOwnSig is not necessarily the matched
//     overload, so compiling its body could run the wrong one;
//   - LEXICAL captures only where the caller allows them (allowCaptures):
//     the BODY lambda threads them onto the closure (resolved to compiled
//     homes in the enclosing scope, bound to trailing unit slots); the
//     extras/hook path keeps refusing them — its shared-token-shape hooks
//     have no per-hook capture layout. MODULE-SCOPE mutable-instance reads
//     are not lexical captures — the callers admit those via
//     moduleScopeMutableCaptures;
//   - no flow-control sentinel in the body;
//   - param count matches the callback inputs, and every declared param TYPE
//     accepts its input — the same membership the runtime MatchFnSig checks
//     at dispatch. A param whose type rejects the shape (`[p:String]` against
//     filter's {key,value} pair, or `[kv:KeyVal]` against a list's plain
//     pair) makes the interpreter raise a callback error; compiling the body
//     anyway would silently keep the element. A map-iteration ENTRY input is
//     a KeyVal (a Map subtype) the carrier conservatively under-types as a
//     plain Map, so any Map-family param is accepted there and only a
//     provably-incompatible param (a scalar, a sibling container) refuses.
func lambdaHookCompatible(r *core.Registry, fd *core.FnDefInfo, inputs []core.Value, shape core.ClosureInShape, allowCaptures, foreignOK bool) (*core.Signature, bool) {
	// A fn value DEFINED in another module resolves its free words THERE, so a
	// caller that cannot compile the body in that home must decline it here
	// rather than lower it against r (foreignFnHome's header has the split).
	// `foreignOK` is that caller's answer: the BODY-lambda path passes true
	// because it goes on to compile against fd.Registry; the extras/hook path
	// passes false because it has no registry to swap to, and the runtime
	// callback seam (core.CallBoruFn) then runs the body in its own home,
	// which is where the interpreter runs it too.
	//
	// The gate lives here rather than at the call sites so there is ONE branch
	// to reach and to cover, and so a THIRD caller cannot be added without
	// being asked the question.
	if foreignFnHome(r, fd) && !foreignOK {
		return nil, false
	}
	lam, ok := fd.FirstOwnSig()
	if !ok || len(lam.Body()) == 0 {
		return nil, false
	}
	own := 0
	for i := range fd.Signatures {
		if !fd.Signatures[i].Fallback {
			own++
		}
	}
	if own > 1 {
		return nil, false
	}
	if !allowCaptures && len(fd.Captured) > 0 {
		return nil, false
	}
	if bodyToksHaveSentinel(lam.Body()) {
		return nil, false
	}
	if len(lam.Params) != len(inputs) {
		return nil, false
	}
	for i := range lam.Params {
		pt := lam.Params[i].Type
		if pt == nil {
			continue
		}
		// Quote-polarity screen: an Atom-typed param IS a /q quote-capture
		// slot (fn_params.go — `k:Atom` parses to FnParam.Quote), which binds
		// only a bare Word collected forward at the runtime pointer — never
		// the stack value a callback delivers. The interpreter therefore
		// leaves such a lambda as DATA in every HOF word (each/fold/filter
		// over `(keys m)` → a list of fn values). Admitting it here compiled
		// an APPLYING callback — a live compile≠interpret divergence
		// (checker-compiler-completeness-review.0.md §2.2). Declining keeps
		// the refusal → interpreter fallback → parity.
		if lam.QuoteArgs[i] || pt.ConformsTo(core.TAtom) {
			return nil, false
		}
		if shape == ClosureInKeyVal && inputs[i].Parent.ConformsTo(core.TMap) {
			if !pt.ConformsTo(core.TMap) && !core.TMap.ConformsTo(pt) {
				return nil, false
			}
			continue
		}
		if !core.SigTypeMatches(inputs[i], pt) {
			return nil, false
		}
	}
	return lam, true
}

// fnValueRetSpec is the callback fn value's return contract, or nil when there
// is none to carry.
//
// An ANONYMOUS lambda carries a COUNT-ONLY contract. FnDefInfo.Anonymous
// marks a deliberately conservative static Returns=[Any] placeholder
// (lang/go/CLAUDE.md, "Lambda Syntax") rather than a user-written
// declaration, and the analyser infers the real result TYPE instead — but the
// interpreter's callback seam enforces the placeholder's COUNT (`each
// (x:Integer => [x 1]) [1 2]` raises `each: element 0: … expected 1 return
// value(s), got 2`), so the value must carry it or the compiled callback
// answers `[1 1]` (NUR120, measured 2026-09-05). No type, no declaration
// span beyond the sig's own (a lambda has none): check.LambdaCountContract.
func fnValueRetSpec(fd *core.FnDefInfo, lam *core.Signature, fnPos core.SrcPos) *ClosureRetSpec {
	if fd == nil || lam == nil || len(lam.Returns) == 0 {
		return nil
	}
	if fd.Anonymous {
		return &ClosureRetSpec{
			Types: check.LambdaCountContract(len(lam.Returns)),
			Decl:  lam.Decl,
			Name:  fd.Name,
			Pos:   fnPos,
		}
	}
	return &ClosureRetSpec{
		Types:    lam.Returns,
		Patterns: lam.ReturnPatterns,
		Decl:     lam.Decl,
		Name:     fd.Name,
		Pos:      fnPos,
	}
}

// recordClosureDispatch is the shared tail of the token and lambda closure
// paths: it resolves the lexical captures, probe-compiles the body in a
// throwaway state (a refusal leaves the real program untouched), then
// real-compiles it and records the dispatch (the body operand lowering to
// OpPushClosure). paramNames is nil for the token form (stack-consumed inputs)
// and the lambda's param names for the lambda form. extraLamSlots are the
// NON-body NoEvalArgs slots holding a LAMBDA hook (walk's ASCEND slot, Stage
// M2d): each compiles to its OWN closure unit under the SAME shared token
// shape (extraNoEvalHookSlots only nominates them on a LambdaSharesTokenShape
// word) and rides as a second opClosure operand.
func recordClosureDispatch(r *core.Registry, word string, spec core.CallableSpec, sig *core.Signature, args, bodyToks, inputs []core.Value, paramNames []string, captures []core.CapturedBinding, shape core.ClosureInShape, extraLamSlots []int, outs []core.Value, retSpec *ClosureRetSpec, paramSpec *ClosureParamSpec, pos core.SrcPos, env *bodyRunEnv) bool {
	// The probe fork below needs the CONCRETE EmitState; both callers only
	// reach here through an active recording state, so a non-EmitState
	// recorder (the inactive no-op) declining is the unreachable belt.
	real, isReal := r.Check.Recorder().(*EmitState)
	if !isReal || real == nil {
		return false
	}
	// A capture with no producing event in THIS EmitState has no operand home,
	// so the closure path declines and the island runs the body instead. This
	// was carried as an unreachable defensive arm until the cross-registry
	// closure landed (Stage 3, §6.3): a FOREIGN body's module-scope mutable
	// captures are looked up in fd.Registry, and a foreign flex cell is
	// produced by the foreign module's events, never the caller's — so it
	// reaches here on the ordinary path. Declining is the RIGHT answer, not a
	// belt: resolving that name in the caller instead compiles a closure over
	// the caller's cell, which is a silent wrong answer whenever the two
	// modules share the name (lang/go
	// TestForeignClosureCaptureResolvesInItsOwnRegistry).
	capOps := make([]EmitOperand, len(captures))
	for i, cb := range captures {
		op, ok := real.resolveOperand(cb.Value)
		if !ok {
			return false
		}
		capOps[i] = op
	}

	// Extra LAMBDA hooks (walk's ASCEND slot, Stage M2d): validate each against
	// the SAME shared token-shape inputs the body uses (the nominating gate is
	// LambdaSharesTokenShape-only) and resolve its module-scope captures, so
	// the probe/real passes below compile them alongside the body. Any
	// incompatibility declines the whole closure path — the refusal stands.
	type extraHook struct {
		slot  int
		toks  []core.Value
		names []string
		caps  []core.CapturedBinding
		ops   []EmitOperand
	}
	extras := make([]extraHook, 0, len(extraLamSlots))
	for _, slot := range extraLamSlots {
		fd, isFn := args[slot].Data.(core.FnDefInfo)
		if !isFn { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return false
		}
		hookIns, hookShape, insOK := lambdaCallbackInputs(r, word, spec, args)
		if !insOK { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return false
		}
		lam, lamOK := lambdaHookCompatible(r, &fd, hookIns, hookShape, false, false)
		if !lamOK {
			return false
		}
		names := make([]string, len(lam.Params))
		for i := range lam.Params {
			names[i] = lam.Params[i].Name
		}
		hookCaps := moduleScopeMutableCaptures(r, lam.Body(), nil)
		hookCapOps := make([]EmitOperand, len(hookCaps))
		for i, cb := range hookCaps {
			op, ok := real.resolveOperand(cb.Value)
			if !ok { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				return false
			}
			hookCapOps[i] = op
		}
		extras = append(extras, extraHook{slot: slot, toks: lam.Body(), names: names, caps: hookCaps, ops: hookCapOps})
	}

	// The body compile re-runs the body (the ReturnsFn pass already emitted
	// its diagnostics); drop any it re-emits so counts do not double.
	diagBase := len(r.Check.Diagnostics)
	defer r.Check.TruncateDiagnostics(diagBase)

	// PROBE: compile the body in a throwaway state so a refusal leaves the
	// real program untouched (graceful fall-through to the island). The throwaway
	// is SEEDED with the enclosing fn-unit tables (forkForProbe) so a self-
	// recursive call inside the body — `… msd-go` in an `each` body — resolves to
	// the enclosing in-progress unit instead of re-compiling it in the throwaway
	// (which re-hits the same closure and fails its own residual). The REAL
	// compile below resolves the recursion naturally (the enclosing unit's key is
	// already in the real state).
	// A strip-input word compiles count-agnostic (the runtime nets one value
	// from either admitted shape — stripResidualShapeOK screens the rest).
	countAgnostic := spec.EmptyBodyErrors || spec.StripsUnconsumedInput
	// Every compile below — probe, real, each extra hook — runs the body in
	// the environment its analysis run started from (unit_memo.go), entered
	// afresh per compile and exited back to the leaked table after. An
	// environment that cannot be built declines the closure.
	compile := func(toks []core.Value, names []string, caps []core.CapturedBinding) (int, bool) {
		prev, ok := env.enter(r)
		if !ok {
			return -1, false
		}
		defer env.exit(r, prev)
		return compileClosureBody(r, word, spec.BodyOut, countAgnostic, toks, inputs, names, paramSpecPatterns(paramSpec), caps, shape, pos)
	}
	probe := real.forkForProbe()
	r.Check.Emit = probe
	probeUnit, probeOk := compile(bodyToks, paramNames, captures)
	for _, ex := range extras {
		if !probeOk { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			break
		}
		_, exOk := compile(ex.toks, ex.names, ex.caps)
		probeOk = probeOk && exOk
	}
	r.Check.Emit = real
	if !probeOk {
		return false
	}
	// Probe-terminal environment mode → real pass (see tryReturnedClosure):
	// a dyn-body dispatch inside the closure body armed the probe's dynEnv;
	// the real compile must run widened from the start so units it finishes
	// before that dispatch plan their dyn-bind promotions.
	if probe.dynEnv {
		real.dynEnv = true
	}
	// Whole-residual multi-out exactness (BodyOutResidual, N > 1): the dispatch
	// seats exactly len(outs) results, so the compiled unit's residual count
	// must be statically EXACT and equal — a variadic merge, a dynamic-apply
	// tail, or any count mismatch declines to the refusal path. 0/1-out bodies
	// keep today's shapes untouched (a diverging body's single Error out, a
	// dynTrail apply netting one value).
	if spec.BodyOut == core.BodyOutResidual && len(outs) > 1 && !closureResidualExact(probe, probeUnit, len(outs)) {
		return false
	}
	// Strip-input shape screen (`error`): admit the two residual shapes the
	// runtime nets ONE value from, or — when the dispatch recorded ZERO
	// outputs (the proven-raise zero-netting handler, completeness-review
	// §8.2(6)) — the empty residual the runtime nets nothing from.
	// Everything else declines to the refusal path, exactly as before.
	if spec.StripsUnconsumedInput && !stripResidualShapeOK(probe, probeUnit, len(outs)) {
		return false
	}

	// REAL: compile the body into the program (deterministic success after a
	// clean probe), then record the dispatch with the body as a closure.
	recsBefore := len(real.fnRecs)
	unit, realOk := compile(bodyToks, paramNames, captures)
	// REACHABLE since Stage 4b (unit_memo.go), not a defensive arm: the
	// probe carries no producedBy, so an enclosing binding read whose value
	// an EVENT produced (`k` after a leaking `do` rebound it) bakes as a
	// const in the probe and routes LIVE in the real compile — and the
	// residual-order hazard refuses the live read where it admitted the
	// const. The real state is already marked with the hazard's reason;
	// this decline hands the dispatch to its own refusal path (first reason
	// wins). `def k 5  do [ k  def k 9  k ]  do [ k  def k 12  k ]` pins it
	// (lang/go/analysis_order_test.go).
	if !realOk || unit < 0 {
		return false
	}
	// The closure latch for the arm-residency bridge: THIS unit, and
	// whether it is fresh (a memo hit reuses a unit another dispatch's
	// twins already own — the bridge declines on stale). Set before the
	// extra-hook compiles below so a walk hook's unit never masquerades as
	// the body's.
	real.lastClosure = closureLatch{unit: unit, fresh: unit == recsBefore && len(real.fnRecs) > recsBefore}
	// A lambda's declared param contract rides on its unit (lamParamContract).
	if paramSpec != nil {
		real.SetUnitParamTypes(unit, paramSpec.Types, paramSpec.Patterns)
	}
	var extraOps map[int]EmitOperand
	for _, ex := range extras {
		exUnit, exOk := compile(ex.toks, ex.names, ex.caps)
		if !exOk || exUnit < 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return false
		}
		if extraOps == nil {
			extraOps = map[int]EmitOperand{}
		}
		extraOps[ex.slot] = EmitOperand{kind: opClosure, closureUnit: exUnit, closureCaps: ex.ops}
	}
	return real.RecordClosureCall(word, sig, args, spec.BodyPos, unit, capOps, extraOps, outs, retSpec, pos)
}

// closureResidualExact reports whether a probe-compiled closure unit's residual
// is statically EXACT with exactly want outputs: no variadic merge (`if c []
// [a b]`), no dynamic-apply tail (dynTrailArity), no dyn-frame window
// (dynFrameW), and want resolved residual operands. A whole-residual dispatch
// (`do`, BodyOutResidual) seats want results at the call site, so a unit whose
// runtime count could differ from the check run's must decline to the refusal
// path rather than mis-seat the simulated stack.
func closureResidualExact(es *EmitState, unit, want int) bool {
	if es == nil || unit < 0 || unit >= len(es.fnRecs) {
		return false
	}
	rec := es.fnRecs[unit]
	return !rec.variadic && rec.dynTrailArity == 0 && rec.dynFrameW == 0 && len(rec.outOps) == want
}

// stripResidualShapeOK reports whether a strip-input word's probe-compiled
// closure unit leaves a residual its handler's runtime netting agrees with
// the recorded dispatch about. want is the dispatch's recorded output count:
//   - want 1 (the historical contract): a single-value residual (the body
//     consumed or replaced the input — including the empty `error []` body,
//     whose residual is the input itself), or a 2-value residual whose
//     BOTTOM is the unconsumed input (param local 0) that the handler's
//     identity probe strips (errorHandler's stack-neutrality rule);
//   - want 0 (the §8.2(6) zero-netting graduation): an EMPTY residual — the
//     body consumed the seeded input and produced nothing, so the handler
//     nets nothing and the 0-output dispatch seats nothing; the ReturnsFn
//     measured the same body, so the counts agree by construction.
//
// A diverging body never RETs (the re-raise propagates out of InvokeBody in
// both engines) and is exempt. Anything else — variadic, dynamic-apply
// tails, deeper residuals, a count the dispatch did not record — declines.
func stripResidualShapeOK(es *EmitState, unit, want int) bool {
	if es == nil || unit < 0 || unit >= len(es.fnRecs) {
		return false
	}
	rec := es.fnRecs[unit]
	if rec.frag != nil && fragDiverges(rec.frag) {
		return true
	}
	if rec.variadic || rec.dynTrailArity > 0 || rec.dynFrameW > 0 {
		return false
	}
	switch len(rec.outOps) {
	case 0:
		return want == 0
	case 1:
		return want == 1
	case 2:
		op := rec.outOps[0]
		return want == 1 && op.kind == opLocal && op.idx == 0
	}
	return false
}

// lambdaCallbackInputs returns the representative input carriers a higher-order
// word presents to a LAMBDA callback — the word's callback shape, which differs
// from the token-quotation form (spec.Inputs) — plus the runtime ClosureInShape
// the driving handler reads to present each entry:
//
//   - filter over a LIST: one {key, value} pair Map (the element via `.value`).
//   - filter/each over a MAP: one KeyVal {k v i n} (the value via `.v`).
//   - fold (init form) / scan over a MAP: (accumulator, KeyVal).
//
// The carriers are GENERALISED (field types, not one call's values) so the body
// is compiled once for every entry. ok is false for a shape with no lambda
// convention (the caller then leaves the refusal to stand): a list each/fold,
// a no-init map fold, and for-each (whose check-mode output count does not match
// its 0-result runtime) all stay on the refusal path.
func lambdaCallbackInputs(r *core.Registry, word string, spec core.CallableSpec, args []core.Value) ([]core.Value, core.ClosureInShape, bool) {
	// A word whose lambda callback sees the SAME inputs as its token form
	// (walk's payload map) declares LambdaSharesTokenShape: Inputs(args) IS the
	// lambda convention, no per-word shape below — and no data-follows-body
	// operand layout is assumed (walk's data PRECEDES its hooks).
	if spec.LambdaSharesTokenShape {
		if spec.Inputs == nil {
			return nil, ClosureInValue, false
		}
		if ins := spec.Inputs(args); ins != nil {
			return ins, ClosureInValue, true
		}
		return nil, ClosureInValue, false
	}
	if spec.BodyPos+1 >= len(args) {
		return nil, ClosureInValue, false
	}
	data := args[spec.BodyPos+1] // the data operand follows the body operand
	if !core.IsConcrete(data) {
		// A COMPUTED collection (`filter (…lambda) (keys kv)`) arrives as a
		// typed CARRIER. A typed, NON-dynamic List/Map carrier is admitted:
		// its element type reads off the carrier exactly as off a concrete
		// value (DataListElemTypeFromValue), and the closure compiles once
		// against it. A Dynamic (gradual) carrier still refuses — the
		// collection's family is unknown, so the InShape (pair vs KeyVal),
		// and with it the runtime callback convention, is ambiguous. Bare
		// type literals and non-container carriers refuse as before.
		if data.Dynamic || !data.Carrier || data.Parent == nil ||
			!(data.Parent.ConformsTo(core.TList) || data.Parent.ConformsTo(core.TMap)) {
			return nil, ClosureInValue, false
		}
	}
	elem := check.DataListElemTypeFromValue(data)
	isMap := data.Parent.ConformsTo(core.TMap)
	isList := data.Parent.ConformsTo(core.TList)
	switch word {
	case "filter":
		switch {
		case isMap:
			return []core.Value{keyValCarrier(r, elem)}, ClosureInKeyVal, true
		case isList:
			return []core.Value{pairCarrier(elem)}, ClosureInValue, true
		}
	case "each":
		if isMap {
			return []core.Value{keyValCarrier(r, elem)}, ClosureInKeyVal, true
		}
		// A LIST each hands the callback the bare ELEMENT (NUR086's list
		// Function form: "a per-container form hands the container's natural
		// unit"). Measured against the interpreter, not inferred from the map
		// twin: `def show fn [[e:Any][Any][typeof e]] each show/v [1 2 3]`
		// answers [Integer Integer Integer], so one input, passed through
		// unchanged. filter is the documented exception in the other
		// direction — its single cross-container form hands a {key,value}
		// position descriptor even over a list, which is why the two cases
		// here differ rather than sharing a branch.
		if isList {
			return []core.Value{check.NewElementCarrier(elem)}, ClosureInValue, true
		}
	case "fold":
		// NOT an arity rule. `fold` declares TWO signatures — one taking a seed
		// operand, one not — and the operand test below asks WHICH SIGNATURE
		// this call site matched, exactly as any recorder reads its operands.
		// The argument-binding rule itself is the kernel's single one and holds
		// at every arity; nothing here keys off how many params the CALLBACK
		// declares.
		//
		// Seeded map form (`init fold (lambda) {m}` → args [lambda, map, init]):
		// the accumulator carries the seed's type, the entry rides as a KeyVal.
		if isMap && len(args) > spec.BodyPos+2 {
			acc := args[spec.BodyPos+2]
			return []core.Value{core.NewCarrier(acc.Parent), keyValCarrier(r, elem)}, ClosureInKeyVal, true
		}
		// A LIST fold's lambda declares (element, accumulator) — the
		// interpreter's top-down assignment over the stack InvokeBody hands it
		// — while the handler pushes (accumulator, element). Carriers go in
		// DECLARED order and ClosureInStackPair reverses at the bind; see its
		// doc for why the two containers differ.
		if isList {
			// The seeded signature carries the accumulator at the seed's type;
			// the unseeded one seeds from the first ELEMENT, so both slots carry
			// the element type — the scan case exactly. Same signature question
			// as the map branch above, same answer shape.
			accT := elem
			if len(args) > spec.BodyPos+2 {
				accT = args[spec.BodyPos+2].Parent
			}
			return []core.Value{check.NewElementCarrier(elem), core.NewCarrier(accT)}, ClosureInStackPair, true
		}
	case "scan":
		// scan seeds the accumulator from the first value (no init operand): the
		// accumulator carries the value type, the entry rides as a KeyVal.
		if isMap {
			return []core.Value{core.NewCarrier(elem), keyValCarrier(r, elem)}, ClosureInKeyVal, true
		}
		// A LIST scan seeds the accumulator from the first ELEMENT, so both
		// slots carry the element type; the order and the permutation are the
		// list fold's.
		if isList {
			return []core.Value{check.NewElementCarrier(elem), core.NewCarrier(elem)}, ClosureInStackPair, true
		}
	}
	return nil, ClosureInValue, false
}

// THE LIST CALLBACK CONVENTION — why the list fold/scan cases above carry
// ClosureInStackPair while every other case is positional. Measured 2026-08-27,
// admitted 2026-08-28; three readings, and the wrong two are the useful part.
//
// A compiled closure binds its inputs POSITIONALLY: invokeClosureOn fills the
// unit's leading param slots from the handler's `inputs` slice in order
// (eng/go/vm.go). The interpreter presents them in the CALLBACK CONVENTION for
// the container, and the two conventions are NOT the same:
//
//	MAP  fold/scan   sig[0] = ACCUMULATOR, sig[1] = entry
//	LIST fold/scan   sig[0] = ELEMENT,     sig[1] = accumulator
//
// That asymmetry is measured, not inferred, and it holds with AMBIGUOUS param
// types — `fold ([x:Any y:Any] => [x]) {a:1} 0` answers the seed while
// `fold ([x:Any y:Any] => [x]) [7] 0` answers the element — so it is a real
// ordering convention and not a by-type assignment that merely looks like one.
//
// WHY they differ is the reading that took longest, and it is not in the
// matcher. BOTH handlers hand `(accumulator, element)`. The MAP path calls the
// lambda POSITIONALLY — mapBody.callLambda → CallBoruFn, which binds args in
// sig order — so sig[0] is the accumulator. The LIST path goes through
// InvokeBody, whose interpreter arm runs the inputs as a STACK (RunResolved),
// and MatchSignature fills from the TOP DOWN — so sig[0] is the element. Same
// order in, opposite assignment out, because one path is positional and the
// other is a stack. Nothing about arity or about the accumulator's type is
// involved.
//
// So the compiled binding needs ONE per-word permutation, not a modelled
// callback frame: ClosureInStackPair reverses at the bind, and the list rows
// compile native. Before that they did not REFUSE — they ISLANDED, answering
// correctly through the interpreter inside a compiled program, which is the
// outcome the mission rules out and which no value-parity test can see.
//
// THREE PROBE LESSONS, each of which cost a wrong turn:
//
//   - A TYPED probe cannot establish positional order, because the matcher
//     reassigns by type. `fn [[a:Integer b:String]…] fold f/v [1 2 3] 'seed'`
//     matching, and its swap not matching, reads like "a is the element" and
//     proves nothing of the kind. Use SAME-TYPED or Any params, or make no
//     positional claim.
//   - Correcting that, an earlier version of this note over-shot the other way
//     — asserting the accumulator is sig[0] for BOTH containers, i.e. no
//     asymmetry at all — which the ambiguous-type probe above disproves. Both
//     errors came from reasoning about the matcher instead of running a body
//     that reports which argument it got.
//   - The accumulator's runtime type after step 1 is the BODY'S RETURN, not the
//     seed's, so a single static carrier looked like it needed a stability
//     guard. It does not: the body binds the accumulator at its DECLARED PARAM
//     type, not the carrier's tag — `fold ([acc:Scalar kv:KeyVal] => [if (acc
//     is Integer) ['I'] ['S']]) {a:1 b:2 c:3}` answers 'S' on BOTH lanes, and
//     `typeof acc` answers Scalar on both. A step whose accumulator fails the
//     declared param no-matches identically in both lanes, and an OVERLOADED
//     callback is already refused (lambdaHookCompatible wants exactly one own
//     signature). That guard would have refused valid programs for a hazard
//     that is not there.
//
// And one about the fix itself: supplying the carriers in the other order does
// NOT permute anything. Carriers only TYPE the body; the unit's param slots
// come from the LAMBDA's own declared order, and what lands in each is the
// handler's push order. Both carrier spellings produced 60.
//
// pairCarrier builds a representative {key, value} pair Map carrier — the shape
// filter's list Function form hands its callback (key = the index, value = the
// element). Field VALUES are carriers (Integer key, elem value) so the compiled
// body reads field TYPES, never one call's concrete values.
func pairCarrier(elem *core.Type) core.Value {
	om := core.NewOrderedMap()
	om.Set("key", core.NewCarrier(core.TInteger))
	om.Set("value", core.NewCarrier(elem))
	return core.NewValueRaw(core.TMap, core.MapPayload{M: om})
}

// keyValCarrier builds a representative KeyVal {k v i n} carrier — the shape the
// map Function forms (filter/each/fold/scan over a map) hand their callback. The
// value field carries the map's common value type; k/i/n carry String/Integer/
// Integer. Tagged Node/Map/KeyVal directly — the type is kernel-declared
// (keyval.go), so the former registered-or-plain-Map fallback probe is gone.
func keyValCarrier(_ *core.Registry, elem *core.Type) core.Value {
	om := core.NewOrderedMap()
	om.Set(core.KeyValK, core.NewCarrier(core.TString))
	om.Set(core.KeyValV, core.NewCarrier(elem))
	om.Set(core.KeyValI, core.NewCarrier(core.TInteger))
	om.Set(core.KeyValN, core.NewCarrier(core.TInteger))
	return core.NewValueRaw(core.TKeyVal, core.MapPayload{M: om})
}

// extraNoEvalHookSlots classifies every NON-body NoEvalArgs operand of a
// Callable word (walk's optional ASCEND hook — the only such slot today) for
// riding next to the compiled body closure. The driving handler classifies
// such a hook at run time and, for a concrete list, runs its element snapshot
// as a token QUOTATION in a sub-engine (walkClassifyHook → runQuotationBody),
// where a NAME resolves against the REGISTRY — but the compiled program holds
// top-level value defs as VM frame locals, so a name-bearing hook would
// diverge (the same registry asymmetry execBodyRefsNames defends the body
// slot against). Two shapes are admitted:
//
//   - a flex reference whose runtime tokens are PROVABLY EMPTY
//     (emptyFlexHookOperand) — it rides as a plain value operand;
//   - on a LambdaSharesTokenShape word, a LAMBDA (a concrete FnDefInfo) —
//     returned in `lamSlots` for recordClosureDispatch to compile to its OWN
//     closure unit (Stage M2d, the two-hook walk row): the handler classifies
//     a compiled closure per hook slot, so a second closure operand runs
//     byte-identically to the interpreter's per-node lambda call. The full
//     lambda constraints (single own sig, capture-free, param-compatible) are
//     enforced at compile time there; a decline leaves the refusal standing.
//
// Anything else declines — the caller then leaves the Stage-2 refusal
// standing (an INERT token hook needs no admission here: when the closure
// path declines, the existing noEvalBodiesInertScoped const bake still
// applies).
func (es *EmitState) extraNoEvalHookSlots(sig *core.Signature, spec core.CallableSpec, args []core.Value) (lamSlots []int, ok bool) {
	for i := range args {
		if i == spec.BodyPos || !sig.NoEvalArgs[i] {
			continue
		}
		if es.emptyFlexHookOperand(args[i]) {
			continue
		}
		if _, isFn := args[i].Data.(core.FnDefInfo); isFn && spec.LambdaSharesTokenShape {
			lamSlots = append(lamSlots, i)
			continue
		}
		return nil, false
	}
	return lamSlots, true
}

// emptyFlexHookOperand reports whether v is a flex reference that is PROVABLY
// EMPTY when the word being recorded dispatches, so its classify-time hook
// snapshot (`bl.Slice()` at handler entry, before any traversal) is a
// zero-length token list — and stays zero-length for the whole call: the
// descend closure's in-traversal appends land beyond the snapshot's length.
// Every hook invocation over it then runs ZERO tokens, byte-identical to the
// interpreter classifying the SAME (empty) flex through the same handler —
// this is the `def acc (flex [])  walk … (m => [acc … append])  acc` corpus
// shape, where the trailing accumulator reference is collected as the ascend
// hook. The proof obligations, all conservative:
//
//   - flex-typed operand (list or map family; an empty flex MAP hook raises
//     the same handler-side walk_error in both engines);
//   - produced by the MAIN-registry `flex` native over EMPTY const containers
//     (sig pointer identity, so a module/user shadow never matches);
//   - recorded at TOP-LEVEL straight-line scope (one unit, one frame):
//     recording order IS runtime order and the dispatch runs exactly once —
//     inside a loop/branch/fn a later-recorded (or unrecorded-iteration)
//     mutator could precede this dispatch at run time;
//   - NO event recorded since the construction (pr.seq == es.seq): every
//     runtime effect between the two — a mutator call, a user call, a branch,
//     an island — records an event or refuses, so the flex still holds its
//     constructed (empty) contents when the word dispatches.
func (es *EmitState) emptyFlexHookOperand(v core.Value) bool {
	if len(es.units) != 1 || len(es.frames) != 1 {
		return false
	}
	p := v.Parent
	if p == nil || (!p.ConformsTo(core.TFlexList) && !p.ConformsTo(core.TFlexMap)) {
		return false
	}
	pr, ok := es.producedBy[v.ID]
	if !ok || pr.idx != 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return false
	}
	evs := es.frames[0]
	// Trailing evDynBind and evBindTwin events are def-site BOOKKEEPING (the
	// dynamic-scope binder trail, and the bind-twin position marker — inert
	// today, and at the flip its only runtime effect is a registry install):
	// neither can touch the flex contents — skip them when locating the last
	// EFFECTFUL event, which must still be this flex's construction (the
	// "no event recorded since" proof, bookkeeping-tolerant).
	i := len(evs) - 1
	for i >= 0 && (evs[i].kind == evDynBind || evs[i].kind == evBindTwin) {
		i--
	}
	if i < 0 {
		return false
	}
	last := evs[i]
	if last.seq != pr.seq || last.kind != evCall || last.call.word != "flex" {
		return false
	}
	// The matched sig must be the MAIN registry's `flex` binding (pointer
	// identity into its sig backing array) — a same-named module inner native
	// or user fn must never satisfy this proof.
	if es.reg == nil {
		return false
	}
	fn := es.reg.Lookup("flex")
	if fn == nil {
		return false
	}
	sigOK := false
	for i := range fn.Signatures {
		if &fn.Signatures[i] == last.call.sig {
			sigOK = true
			break
		}
	}
	if !sigOK || len(last.call.ops) == 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return false
	}
	for _, op := range last.call.ops {
		if op.kind != opConst || op.idx < 0 || op.idx >= len(es.consts) { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return false
		}
		if !emptyContainerConst(es.consts[op.idx]) {
			return false
		}
	}
	return true
}

// emptyContainerConst reports whether v is a concrete container literal with
// ZERO members ([] or {}) — the only construction input emptyFlexHookOperand
// accepts, so the flex provably starts with no elements.
func emptyContainerConst(v core.Value) bool {
	if !core.IsConcrete(v) {
		return false
	}
	if lst, err := core.AsList(v); err == nil && !lst.IsNil() {
		return lst.Len() == 0
	}
	if m, err := core.AsMap(v); err == nil && m != nil {
		return m.Len() == 0
	}
	return false
}

// bodyToksHaveSentinel reports whether a lambda body's token slice contains a
// flow-control sentinel (break/continue/return) — the token-slice form of
// bodyHasSentinel, for a lambda whose Body is already []Value.
func bodyToksHaveSentinel(toks []core.Value) bool {
	found := false
	core.WalkBodyWords(toks, func(w core.WordInfo, _ core.Value) {
		switch w.Name {
		case "break", "continue", "return":
			found = true
		}
	})
	return found
}
