package core

import (
	"fmt"
	"sort"
	"strings"
)

// InstallDef registers a new word as a literal substitution or a typed
// function definition. Multiple defs for the same name stack; undef pops
// the top.
//
// When body is a FnDefInfo value (produced by the fn word), InstallDef
// registers typed signatures. Otherwise, body is stored directly as a
// literal substitution.
//
// This is the REDEFINITION install: a fn-valued body whose signatures
// overlap an existing binding for the SAME name drops the colliding
// overload (so the stale one doesn't race the new one). For a per-call
// fn-frame param/capture — which enters a NEW lexical scope and must
// SHADOW, not replace, an outer same-named binding — use
// InstallFrameBinding instead.
func InstallDef(r *Registry, name string, body Value, stackOnly ...bool) {
	installDef(r, name, body, false, stackOnly...)
}

// InstallFrameBinding installs a per-call fn-frame binding (a named
// param or a lexical capture). Unlike InstallDef it is a lexical
// SHADOW: a Function-valued binding pushes a fresh dispatch entry
// that shadows — never removes — an outer same-named binding, so the
// caller's binding is restored intact when the frame's teardown pops
// this entry. InstallDef's overlap-removal models top-level
// REDEFINITION (it must drop the colliding overload); a param entering
// a new scope must not, or it destroys the caller's binding (e.g. a
// fn-valued arg whose param name collides with a live caller param —
// design/ACCESSOR-SPLIT-AND-CLEANUP-BUG.md).
func InstallFrameBinding(r *Registry, name string, body Value) {
	installDef(r, name, body, true)
}

func installDef(r *Registry, name string, body Value, shadow bool, stackOnly ...bool) {
	// The rebind notification, seated with the operation rather than with the
	// `def` word (core/go/rebind_notify.go). `!shadow` is the same test every
	// twin note below makes: a SHADOWING install is InstallFrameBinding's —
	// a param or a capture, scoped to one call — not a rebind of the
	// module-scope name a unit could have baked.
	if !shadow {
		noteRebind(r, name)
	}
	isStackOnly := len(stackOnly) > 0 && stackOnly[0]

	// Attribute a body-local def to its enclosing fn for the dynamic-scope
	// undefined-word rescue (check mode only; no-op at the top level or outside
	// check). A name bound as a local inside fn F is visible — via boru's
	// dynamic scoping — to any fn F reaches on the call stack.
	r.noteAnalysisFnBinder(name)

	// FnDefInfo body (from fn word): install typed signatures.
	// Only fn-based defs register functions; simple value defs just use
	// DefStacks. A Function-FAMILY body with no FnDefInfo payload — a
	// CARRIER under analysis — installs NOTHING here: the compiled closure
	// machinery (RecordDynBind / MarkValueDef / closure render) owns such
	// names, and a visible Defs binding diverts its provenance modeling.
	// The def word's handler records the carrier in the check-scoped
	// fn-carrier side table instead (native installAndRecordDef), which the
	// parse/mini/emit value-form macros consult for name resolution.
	if body.Parent.Equal(TFunction) {
		fnDef, ok := body.Data.(FnDefInfo)
		if !ok {
			return
		}
		fnDef.Name = name

		// Module-wrapper rebinding: a trivial-delegation wrapper
		// (`Body=[Word(inner)]`, all-unnamed params, carrying its own
		// sub-registry) is what `import` produces for each export and
		// what `unpack` extracts. Dot-access (`pkg.word`) dispatches it
		// via execFnDefLiteral's short-circuit, which looks up the
		// INNER native's real Signatures (carrying QuoteArgs /
		// NoEvalArgs / the Go handler) in the sub-registry. When the
		// same value is rebound to a bare name (`def w pkg.word` or
		// `unpack [word] pkg`), the body-splice path below would instead
		// re-dispatch `Word(inner)` in THIS registry — where the inner
		// native isn't registered — and drop the wrapper's special
		// arg-handling (FnSig has no QuoteArgs field). Mirror dot-access
		// instead: bind the inner native's Signatures verbatim under the
		// new name so bare-word dispatch behaves exactly like pkg.word.
		if reg := fnDef.Registry; reg != nil && reg != r {
			own := fnDef.OwnSigs()
			// EVERY own sig must be a trivial delegation to the SAME
			// inner native — a multi-overload wrapper (e.g. IO.write)
			// carries one delegation FnSig per overload. Requiring only
			// a single sig here used to drop multi-sig wrappers onto the
			// body-splice path below, where the wrapper's own UNLOCKED
			// FnSigs were installed — so a later overlapping `def` could
			// silently replace a module word instead of raising
			// locked_signature (the inner native's sigs are locked).
			innerName := ""
			allTrivial := len(own) > 0
			for i := range own {
				target, ok := trivialDelegationTarget(&own[i])
				if !ok || (innerName != "" && target != innerName) {
					allTrivial = false
					break
				}
				innerName = target
			}
			if allTrivial {
				if inner := reg.Lookup(innerName); inner != nil && len(inner.Signatures) > 0 {
					rebound := FnDefInfo{
						Name:           name,
						Signatures:     append([]Signature(nil), inner.Signatures...),
						MaxForwardArgs: inner.MaxForwardArgs,
						Registry:       reg,
						// A trivial-delegation rebind is the inner word under
						// another name — the record's own case — so it inherits
						// the inner word's identity token (NUR031).
						ident: inner.ident,
					}
					r.Defs.Push(name, NewFunction(rebound))
					if !shadow {
						r.NoteBindTransition(BindDef, name, body.Pos())
					}
					if !shadow && r.ready && r.OnRegisterHook != nil {
						r.OnRegisterHook(name)
					}
					return
				}
			}
		}

		// Remove any previous DefStack entries whose signatures overlap
		// with the new definition. Each entry holds its own overloads and
		// the dispatch aggregate (Registry.Lookup) unions them, so an
		// overlapping redefinition must drop the old entry — otherwise the
		// stale overload races the new one (equal scores, first match wins).
		//
		// SKIPPED for a shadowing frame-binding install: a fn param/capture
		// that collides with an outer same-named binding must SHADOW it (a
		// fresh entry the teardown pops to restore the outer), not drop it.
		// Dropping the outer entry here is the per-call cleanup over-pop bug
		// (design/ACCESSOR-SPLIT-AND-CLEANUP-BUG.md): the colliding outer
		// param vanishes, then the frame's undef tail pops the wrong level.
		// Entries carrying LOCKED signatures (native registrations, module-
		// wrapper rebindings) are never dropped: locked sigs can never be
		// replaced or removed (design/OPEN-WORDS.0.md §2.3). The def-merge
		// path (BuildWordExtension) intercepts fn defs over locked-bearing
		// words before InstallDef, so this guard is defence in depth for
		// direct InstallDef callers.
		replaced := false
		if stack := r.Defs.Stack(name); !shadow && len(stack) > 0 {
			filtered := stack[:0:0]
			changed := false
			for _, entry := range stack {
				oldFn, ok := entry.Data.(FnDefInfo)
				if ok && !hasLockedSig(oldFn.Signatures) && FnDefsOverlap(oldFn, fnDef) {
					changed = true
					continue
				}
				filtered = append(filtered, entry)
			}
			if changed {
				// A fn REDEFINITION whose overlap-removal drops an existing
				// overload inside a CONDITIONALLY-reached body (an if/case arm
				// or a loop body — CondBodyDepth) clobbers the enclosing
				// binding in-place: the drop-then-push leaves the def depth
				// UNCHANGED, so the branch/loop def rollback (which restores by
				// depth growth) cannot revert it, and compiled resolution bakes
				// the conditional shadow. The interpreter keeps the outer fn
				// when the branch is not taken (or the loop runs zero times), so
				// the two diverge. Refuse — MarkUncompilable is a no-op off the
				// compile pass, so plain check and the interpreter are unaffected
				// and the program runs correctly (slow, not wrong). An
				// UNCONDITIONAL redefinition (top level or inside `do`) is sound
				// and keeps compiling: CondBodyDepth is 0 there.
				//
				// A redefinition inside a FN BODY by a CAPTURING fn value — a
				// factory's returned closure (`def p (kk 9)` over an outer
				// `def p (kk 7)`) — is the same divergence on the interpreter's
				// own frame teardown: the drop-then-push leaves the frame's def
				// depth unchanged, so DefCleanup pops nothing and the replacement
				// OUTLIVES the call, where the analysis restores its snapshot and
				// the compiled program keeps the outer closure's bake (`g (p 3)`
				// answered 10 for the interpreter's 12 — the thirty-first
				// increment). A capture-free literal takes the compiled twin and
				// agrees; the closure's payload has no twin, so it refuses.
				refusal := ""
				if r.analysisInCondBody() {
					refusal = "fn '" + name + "' redefined inside a conditional body (branch/loop) shadows an outer overload"
				} else if r.Check.FnBodyDepth > 0 && len(fnDef.Captured) > 0 {
					refusal = "fn '" + name + "' redefined inside a fn body by a capturing fn value replaces an outer overload past the call"
				}
				if refusal != "" {
					r.analysisRecorder().MarkUncompilable(refusal)
				}
				r.Defs.Set(name, filtered)
				replaced = true
			}
		}

		// Compile this definition's own overloads and push a single
		// DefStack entry. The 0-arg fallback and cross-stack overloading
		// are synthesised on demand by Registry.Lookup → aggregateDispatch.
		installFnDef(r, name, fnDef, !shadow, isStackOnly)
		if !shadow {
			// A REDEFINITION whose overlap filter dropped the colliding entry
			// is a drop-then-push: the net depth is unchanged, so a twin that
			// replays it as a plain push lands one level too deep and a later
			// `undef` exposes the wrong binding — the hazard §6.5 names and the
			// one family L's refusal exists for. Recorded as its own kind so
			// the twin can reproduce the replace rather than infer it.
			kind := BindDef
			if replaced {
				kind = BindDefReplace
			}
			r.NoteBindTransition(kind, name, body.Pos())
		}
		return
	}

	// FnUndefInfo body (from fn word in pair mode): remove targeted signatures.
	if body.Parent.Equal(TFnUndef) {
		undefInfo, ok := body.Data.(FnUndefInfo)
		if !ok {
			return
		}
		UninstallFnSigs(r, name, undefInfo)
		return
	}

	// ClassTypeInfo body: set the proper name in the type hierarchy.
	if IsClassType(body) {
		info, _ := AsClassType(body)
		if info.Parent != nil {
			// Child type: full name is Parent/Name (e.g. Ideal/Foo/Bar)
			info.Name = info.Parent.Name + "/" + name
		} else {
			// Direct child of the Object kind: Ideal/Name
			info.Name = "Ideal/" + name
		}
		// Register the name parts as known type parts.
		for _, p := range strings.Split(info.Name, "/") {
			r.RegisterPart(p)
		}
		// Preserve the body's *Type identity (set by the caller via
		// NewClassType). InstallDef rewrites info.Name based on the
		// def name and parent, then re-wraps the value — but the def
		// itself stays the caller's choice. For builtin object types
		// (Resource, Entity) the caller passes the canonical builtin
		// *Type; for user-defined object types installed as defs the
		// caller is responsible for minting first.
		def := body.Parent
		if def == nil {
			def = TClass
		}
		body = NewClassType(def, info)
		r.Defs.Push(name, body)
		if !shadow {
			r.NoteTypeInstall(name, body.Pos())
		}
		return
	}

	r.Defs.Push(name, body)
	if !shadow {
		r.NoteBindTransition(BindDef, name, body.Pos())
	}
}

// UninstallDef removes the most recent def for a word, exposing whatever
// binding it shadowed. For an fn def this drops the top DefStack entry's own
// overloads; the dispatch table is rebuilt on demand from the remaining
// entries by Registry.Lookup → aggregateDispatch, so no explicit re-register
// is needed.
func UninstallDef(r *Registry, name string) {
	noteRebind(r, name)
	r.Defs.Pop(name)
	// Recorded AFTER the pop so Depth is the post-transition depth, which is
	// what a twin has to reproduce (§6.5).
	r.NoteBindTransition(BindUndef, name, SrcPos{})
}

// buildFnBodyHandler produces the dispatch Handler for one boru fn
// signature. Rather than computing a final result, the handler returns
// a PAREN-WRAPPED TOKEN SEQUENCE — `( unnamed-args… body DefCleanup __pa
// undef-tail… ReturnCheck )` — that execMatch splices back onto the
// stack to be RE-STEPPED inline. Named params and lexical captures are
// installed as defs up front (captures first so params shadow them);
// the paren keeps the whole expansion atomic so an outer forward can't
// grab an intermediate body value (e.g. recursive factorial).
//
// The handler closes over the install-time registry `r`: an FnDef
// registered into a name dispatches in the registry it was installed in.
// (The registry passed to the Handler at call time is intentionally
// ignored here — this mirrors the historical InstallFnDef closure.)
//
// Extracted verbatim from InstallFnDef so the same body-runner can be
// shared with the Function-value dispatch path (function-model
// consolidation). `s` is the signature; `fnDefCopy` supplies Captured;
// `meta` is the overload identity stamped on each spliced frame's open
// paren (the same pointer the compile boundary stores in
// Signature.FnFrame — see eng/go/fn_frame.go).
func buildFnBodyHandler(r *Registry, name string, s FnSig, fnDefCopy FnDefInfo, meta *FnFrameMeta) Handler {
	// Computed ONCE at construction: does this body install any body-local
	// def or construct an inner fn? A generic fn always does (it installs
	// inferred type-param bindings per call). When false — the common
	// leaf-fn / recursion case (fib, a tail-accumulator) — every call skips
	// BOTH per-call DefTable.Snapshot maps (each O(all bound names)) and the
	// def-cleanup name scan, the dominant term behind the ~340 allocs/frame
	// (design/INTERPRETER-SPEED-PLAN.10.md #5). Stack balance is preserved:
	// the fn baseline still pushes/pops (a nil entry) and the DefCleanup
	// marker still rides the tape (carrying SkipCleanup).
	needsFrameState := fnDefCopy.Gen != nil || bodyNeedsFrameState(r, s.Body())
	// For a leaf body (!needsFrameState) the ENTIRE token expansion is
	// constant per signature except the unnamed-arg cells: frame open,
	// body, and the cleanup tail (nil-snapshot DefCleanup, __pa, undef
	// pairs, zero-Pos ReturnCheck) never vary between calls. Build that
	// skeleton ONCE here and per call only copy it with the arg values
	// patched in — the old per-call rebuild minted ~7 ID-stamped tokens
	// per frame (design/INTERPRETER-SPEED-PLAN.10.md #5). The per-call
	// COPY is mandatory: execMatch's stampResultPos mutates the returned
	// slice (ReturnCheck Pos, fn-value pos), and ForkConcurrent engines
	// share this handler. Nothing keys on the shared tokens' Value.IDs —
	// the tail probe and unwindFrameTail match structurally.
	var (
		skeleton   []Value
		unnamedIdx []int // param positions whose args splice into the frame head
		emptyArgs  Value
		refsArgs   bool
	)
	if !needsFrameState {
		for i, p := range s.Params {
			if p.Name == "" {
				unnamedIdx = append(unnamedIdx, i)
			}
		}
		u := len(unnamedIdx)
		if u > 0 {
			skeleton = append(skeleton, NewFrameOpenSpan(meta, u))
		} else {
			skeleton = append(skeleton, NewFrameOpen(meta))
		}
		skeleton = append(skeleton, make([]Value, u)...) // arg placeholder cells
		skeleton = append(skeleton, s.Body()...)
		skeleton = AppendFrameTail(skeleton, FrameTailSpec{
			Registry:       r,
			SkipCleanup:    true,
			Names:          fnInstallNames(s, fnDefCopy.Captured),
			Returns:        s.Returns,
			ReturnPatterns: s.ReturnPatterns,
			Decl:           s.Decl,
			UnnamedCount:   u,
			FuncName:       name,
			EvalResidual:   !fnDefCopy.Anonymous || BodyEvalsResidual(s.Body()),
		})
		skeleton = append(skeleton, NewCloseParen())
		// When the body provably never reads `args` (sound under the
		// !needsFrameState gate — see bodyReferencesArgs), push a shared
		// empty list per call instead of copying the args into a fresh
		// one; __pa only needs an entry to pop, and nothing else reads
		// the list contents.
		refsArgs = bodyReferencesArgs(r, s.Body())
		emptyArgs = NewList(nil)
	}
	return func(args []Value, _ map[string]Value, _ []Value, callReg *Registry) ([]Value, error) {
		// Reached from a FOREIGN registry (callReg != the install registry r) — a
		// module fn dispatched through the unified execMatch path from an outer
		// engine. The inline-re-step token expansion below binds params and resolves
		// the body in r, but execMatch re-steps the returned tokens in the CALLER's
		// registry (callReg), where r's params + module-private words are absent
		// ("undefined word: x"). So run the body via CallBoru and return the result —
		// the SAME execution the old execFnDefSig path did, now behind ONE dispatch
		// path. A same-registry fn (callReg == r, the common top-level case) keeps the
		// inline re-step (recursion / forward-ref intact).
		//
		// Pointer inequality alone is too broad: a registry FORK (await/timer/model
		// background branch — ForkConcurrent) is a NEW *Registry yet shares r's whole
		// binding lineage PLUS the branch's own local defs. Discriminate by scope id:
		// a fork inherits r's regID (AnalysisScopeID), a true module sub-registry has
		// its own. For a fork, run in callReg (the fork) so a branch-local binding
		// (`TimeUtil.await [[def x 1 read] …]`) the body reads resolves — r lacks it;
		// for a genuine module sub-registry, run in r where its private words live.
		if callReg != nil && callReg != r {
			target := r
			if callReg.AnalysisScopeID() == r.AnalysisScopeID() {
				target = callReg
			}
			return target.CallBoruNamed(&s, args, fnDefCopy.Captured, fnDefCopy.Name)
		}
		// Retag typed-container args up front so EVERY access path in the body —
		// named binding, the args stack (args.N), and unnamed body-token pushes —
		// sees the tagged value and enforces the {:T}/[:T] param contract (a write
		// via args.N must not bypass what a write via the named param enforces).
		args = RetagTypedContainerArgs(s.Params, args)
		// Leaf fast path: per-call work is registry installs plus ONE
		// slice copy of the memoized skeleton with the arg cells patched
		// (see the construction-time comment above).
		if !needsFrameState {
			r.PushFnBaseline(nil)
			argsList := emptyArgs
			if refsArgs {
				argsCopy := make([]Value, len(args))
				copy(argsCopy, args)
				argsList = NewList(argsCopy)
			}
			if err := r.Args.Push(argsList); err != nil {
				r.PopFnBaseline()
				return nil, err
			}
			for _, cb := range fnDefCopy.Captured {
				InstallFrameBinding(r, cb.Name, cb.Value)
			}
			for i, p := range s.Params {
				if p.Name != "" {
					arg := args[i]
					// Quote list params so they're treated as data values
					// when referenced in the body, not expanded as code bodies.
					if arg.Parent.Equal(TList) && !arg.Quoted {
						arg.Quoted = true
					}
					InstallFrameBinding(r, p.Name, RetagTypedContainerParam(p, arg))
				}
			}
			out := make([]Value, len(skeleton))
			copy(out, skeleton)
			for k, i := range unnamedIdx {
				out[1+k] = args[i]
			}
			return out, nil
		}
		var result []Value
		var names []string
		// Wrap the entire expansion (unnamed args + body + undef
		// cleanup) in parens so it evaluates as a single
		// sub-expression. Without this, an outer forward can grab
		// intermediate values from the body before the body
		// finishes executing (e.g. recursive factorial: the outer
		// mul's forward grabs x=1 from the inner body instead of
		// waiting for the full result).
		result = append(result, NewFrameOpen(meta))

		// Push the fn-entry baseline BEFORE installing anything
		// for this call. Closure-capture detection on inner fn
		// constructions (afn/fn) inside this body consults
		// TopFnBaseline to identify enclosing-fn-local bindings:
		// names installed AFTER this snapshot (this call's
		// captures + named params + body-local defs) are
		// capturable; names already present at module/global
		// scope are dynamic. (Only needsFrameState bodies reach
		// here — the leaf fast path above pushes a nil baseline.)
		r.PushFnBaseline(r.Defs.Snapshot())

		// Push args list onto the args stack for access via the
		// "args" word (args.0, args.1, etc.). Paired with __pa
		// at the body tail, which also pops the FnBaseline.
		argsCopy := make([]Value, len(args))
		copy(argsCopy, args)
		argsList := NewList(argsCopy)
		if err := r.Args.Push(argsList); err != nil {
			r.PopFnBaseline()
			return nil, err
		}

		// Install lexical captures BEFORE named params so params
		// shadow captures with the same name (innermost binding
		// wins). Captures are appended to `names` so the
		// synthesized undef tail tears them down alongside params.
		for _, cb := range fnDefCopy.Captured {
			InstallFrameBinding(r, cb.Name, cb.Value)
			names = append(names, cb.Name)
		}

		unnamedCount := 0
		for i, p := range s.Params {
			if p.Name != "" {
				arg := args[i]
				// Quote list params so they're treated as data values
				// when referenced in the body, not expanded as code bodies.
				if arg.Parent.Equal(TList) && !arg.Quoted {
					arg.Quoted = true
				}
				InstallFrameBinding(r, p.Name, RetagTypedContainerParam(p, arg))
				names = append(names, p.Name)
			} else {
				// Unnamed parameter: push value back for the body to use
				result = append(result, args[i])
				unnamedCount++
			}
		}
		// Stamp the resolved-argument span on the frame open so the step
		// loop skips the unnamed args — they enter as stack data and are
		// never re-stepped (arguments are inert; FrameOpenInfo.ArgSpan).
		if unnamedCount > 0 {
			result[0] = NewFrameOpenSpan(meta, unnamedCount)
		}
		// Snapshot DefStacks lengths after installing named params
		// so we can clean up any defs created during body execution
		// (fixes def leakage from fn bodies — DX-REPORT Issue 2).
		defSnapshot := r.Defs.Snapshot()

		// Generic fn: install the inferred type-parameter bindings for
		// the body (`of [T]`, `make (Box of [T])`). AFTER the snapshot,
		// so the existing DefCleanup truncation tears them down — the
		// undef tail's capitalised path would Retire the bound type's
		// canonical node (design/GENERICS.10.md Phase 4).
		if fnDefCopy.Gen != nil {
			InstallGenCallBindings(r, fnDefCopy.Gen, s.Params, args)
		}

		// Append the body tokens directly: append COPIES them into result's
		// backing array and s.Body() (the shared BoruImpl.Body) is never
		// mutated here, so the previous intermediate make+copy was a
		// redundant per-call allocation (design/INTERPRETER-SPEED-PLAN.10.md #5).
		result = append(result, s.Body()...)
		// The canonical cleanup tail: DefCleanup (undoes body-local
		// defs), __pa (pops Args + FnBaseline), the undef pairs for
		// captures+params, and the ReturnCheck when returns are
		// declared (Pos left zero — execMatch stamps the call site).
		result = AppendFrameTail(result, FrameTailSpec{
			Registry:       r,
			Snapshot:       defSnapshot,
			Names:          names,
			Returns:        s.Returns,
			ReturnPatterns: s.ReturnPatterns,
			Decl:           s.Decl,
			UnnamedCount:   unnamedCount,
			FuncName:       name,
			EvalResidual:   !fnDefCopy.Anonymous || BodyEvalsResidual(s.Body()),
		})
		result = append(result, NewCloseParen())
		return result, nil
	}
}

// RetagTypedContainerParam retags a CONCRETE runtime arg bound to a {:T}/[:T]
// param with the param's element constraint, so typed-container PARAM writes are
// enforced in the body. The arg is otherwise bound untagged — a caller's plain
// `{a:1}` for `m:{:Integer}` would let `(m set b/q "bad")` reach the write path
// with no ElemConstraint and bypass enforcement. Unify against the pattern both
// VALIDATES (the sig already admitted the arg, so it conforms) and recursively
// tags; a non-container param or a non-concrete arg passes through. Applied at
// every param-binding site (execFnDefSig / CallBoru / InstallFnDef).
func RetagTypedContainerParam(p FnParam, arg Value) Value {
	if p.Pattern == nil || !IsConcrete(arg) {
		return arg
	}
	return RetagTypedContainerValue(*p.Pattern, arg)
}

// RetagTypedContainerValue is the shared retag core used by BOTH the interpreter
// (RetagTypedContainerParam) and the compiled entry guard (checkParamContract),
// so the two paths agree. It tags a concrete {:T}/[:T] arg with the pattern's
// element constraint; a non-typed-container pattern or a non-flex/non-container
// arg passes through.
//
// A flex container is a REFERENCE type: Unify would deep-copy its backing store
// (NewFlexList/NewFlexMap) and detach the body binding, so a body write would
// mutate a copy and leave the caller's flex untouched. Retag the header in place
// — the FlexListData/FlexMapData pointer stays shared. Dispatch has already
// element-checked the arg (a non-conforming flex is rejected before binding), so
// its elements conform; no re-validate, no rebuild. A plain (immutable) arg is
// re-unified — that validates AND recursively re-tags nested containers.
func RetagTypedContainerValue(pat, arg Value) Value {
	if !IsTypedMap(pat) && !IsTypedList(pat) {
		return arg
	}
	if IsFlexList(arg) || IsFlexMap(arg) {
		ci, err := AsChildType(pat)
		if err != nil { //covergate:allow pat is IsTypedMap/IsTypedList (checked above) so it carries ChildTypeInfo; AsChildType cannot fail
			return arg
		}
		return RetagFlexElem(arg, ci.Child)
	}
	unified, ok := Unify(pat, arg)
	if !ok { //covergate:allow dispatch (matchSignature/checkParamContract) element-checks the concrete arg against pat and rejects any non-conformer with no_signature BEFORE param binding, so an arg reaching here always satisfies pat and Unify cannot fail
		return arg
	}
	unified.Quoted = arg.Quoted // preserve the list-arg no-auto-eval quote
	return unified
}

// RetagFlexElem retags a flex container v so its element type is elemType,
// returning v's header with the tag set (the FlexListData/FlexMapData pointer
// stays shared — identity preserved). When elemType is itself a typed container
// (a nested {:{:T}} / [:[:T]] flex), v's EXISTING children are recursively
// retagged IN PLACE so a nested write enforces the contract at every depth
// (FlexDeepCopy makes the whole tree flex, so every child is a mutable flex
// container). Fixes the nested-container invariant hole (#8, Codex round 8).
func RetagFlexElem(v Value, elemType Value) Value {
	out := v
	out.SetElemConstraint(elemType)
	if IsTypedMap(elemType) || IsTypedList(elemType) {
		if ci, err := AsChildType(elemType); err == nil {
			retagFlexChildrenInPlace(v, ci.Child)
		}
	}
	return out
}

// retagFlexChildrenInPlace retags every existing child of the flex container v so
// each child's element type is grandElem, writing the retagged child header back
// into v's shared store (a mutation of the shared FlexMapData/FlexListData, which
// is exactly what preserves the caller's flex identity).
func retagFlexChildrenInPlace(v Value, grandElem Value) {
	if IsFlexMap(v) {
		if om, _ := AsMutableMap(v); om != nil {
			for _, k := range om.Keys() {
				e, _ := om.Get(k)
				om.Set(k, RetagFlexElem(e, grandElem))
			}
		}
		return
	}
	if fd, _ := AsFlexList(v); fd != nil {
		for i := range fd.Elems {
			fd.Elems[i] = RetagFlexElem(fd.Elems[i], grandElem)
		}
	}
}

// isTypedContainerParam reports whether p's pattern is a {:T} map / [:T] list —
// the only params RetagTypedContainerParam acts on.
func isTypedContainerParam(p FnParam) bool {
	if p.Pattern == nil {
		return false
	}
	pat := *p.Pattern
	return IsTypedMap(pat) || IsTypedList(pat)
}

// RetagTypedContainerArgs retags every concrete {:T}/[:T] argument with its
// param's element constraint, returning a slice where EVERY body access path —
// the named binding, the args stack (`args.N`), and unnamed body-token pushes —
// sees the tagged value. Retagging only the named binding (RetagTypedContainerParam
// at the InstallFrameBinding site) left `args.N` and unnamed params reading the raw
// untagged arg, so a body write through them bypassed enforcement and diverged from
// the compiled path (which retags its locals in checkParamContract). Copy-on-write:
// returns args unchanged (no allocation) when no param is a typed container — the
// common case, keeping the leaf fn-dispatch fast path allocation-free.
func RetagTypedContainerArgs(params []FnParam, args []Value) []Value {
	out := args
	cloned := false
	for i := 0; i < len(args) && i < len(params); i++ {
		if !IsConcrete(args[i]) || !isTypedContainerParam(params[i]) {
			continue
		}
		if !cloned {
			out = make([]Value, len(args))
			copy(out, args)
			cloned = true
		}
		out[i] = RetagTypedContainerParam(params[i], args[i])
	}
	return out
}

// InstallFnDef compiles a function definition's own overloads and pushes a
// single DefStack entry holding them. Each compiled Signature keeps its
// authored shape (Params with names, Returns, Body) and gains a body-splicing
// Handler (binds named params via InstallDef, returns body tokens, appends
// undef cleanup) plus a check-mode ReturnsFn. The cross-stack dispatch table
// (union of stacked entries + sort + synthetic 0-arg fallback) is assembled on
// demand by Registry.Lookup → aggregateDispatch, so this entry stays its own
// authored unit for targeted undef and overlap detection.
func InstallFnDef(r *Registry, name string, fnDef FnDefInfo, stackOnly ...bool) {
	installFnDef(r, name, fnDef, true, stackOnly...)
}

// installFnDef is InstallFnDef with the register hook under the caller's
// control. A SHADOWING frame binding (InstallFrameBinding — a per-call param
// or capture, or the VM's word-read replay installing a fn-valued read for
// one island run) is not a registration: the hook is dynamic help's example
// generator, which RUNS an engine over the new word, so firing it per call
// generated examples for a name that lives one frame and, in a compiled
// program, counted as an unattributed interpreter entry (the interp-entry
// census). A registration proper (`def`, a module export) keeps it.
func installFnDef(r *Registry, name string, fnDef FnDefInfo, hook bool, stackOnly ...bool) {
	isStackOnly := len(stackOnly) > 0 && stackOnly[0]
	entry := fnDef
	entry.Name = name
	entry.Signatures = compileFnSigs(r, name, fnDef, isStackOnly)
	SortSignatures(entry.Signatures)
	entry.MaxForwardArgs = calcMaxForwardArgs(entry.Signatures)
	r.Defs.Push(name, NewFunction(entry))
	// Construction-time body check (first-class, post-binding). Replaces the
	// dynamic-help example eval's accidental side-channel: that eval ran each fn
	// body against SYNTHETIC example args ({a:1,b:2}), producing false positives
	// (decision.boru), and is now hermetic. Here we analyse each boru-bodied
	// overload ONCE against CARRIER args (NewCarrier(param) — an abstract Map/List
	// reads dynamic(Any)), post-binding so recursive self-refs resolve, so a fn
	// that is DEFINED BUT NEVER CALLED still has its body statically checked
	// (an undefined word, an in-body forward strand). Bytecode recording is
	// suspended (a registration is not part of the program's straight line; the
	// armed compile at first dispatch deletes this suspended summary and re-records).
	// Same registry as the body handler above: analysing a foreign fn's body
	// against the INSTALLING registry reports `undefined_word` for every
	// module-private name the body legitimately reaches
	// (design/FUNCTION-VALUE-SCOPE.0.md §7.3 item 3).
	home, _ := FnHome(r, &fnDef)
	home.analysisFnConstructionPass(name, entry)
	if hook && r.ready && r.OnRegisterHook != nil {
		r.OnRegisterHook(name)
	}
}

// compileFnSigs turns a definition's authored overloads into dispatch-
// ready Signatures: optional params expand into extra sigs, boru-bodied
// sigs get the body-splicing runner (buildFnBodyHandler) plus the
// check-mode ReturnsFn, and the BarrierPos sentinel resolves. Shared by
// InstallFnDef (ordinary `def name fn […]`) and BuildWordExtension
// (`def <locked word> fn […]` — the open-words merge), so an added
// signature dispatches byte-identically to an installed fn's.
func compileFnSigs(r *Registry, name string, fnDef FnDefInfo, isStackOnly bool) []Signature {
	// The body runs — and its free words resolve — in the registry that
	// DEFINED the fn, not the one installing the name
	// (design/FUNCTION-VALUE-SCOPE.0.md §4.3). `def g A.pub` and a named
	// `f:Function` parameter both funnel here, and building the handler
	// against the installing registry is what made `A.pub 5` and
	// `def g A.pub  g 5` give DIFFERENT answers: applying a value kept its
	// scope, naming it lost it.
	//
	// Registry is set only at module-export resolution, so a fn defined in
	// the running scope has none and this is r — the common path is byte-
	// identical. Only a value that arrived from another module moves.
	//
	// The rebind is TOTAL and deliberate: every remaining use of r below —
	// buildFnBodyHandler's closed-over registry, its body analyses, the frame
	// tail, analysisReturnsFn — wants the defining one. Read the rest of this
	// function as "r is where this fn lives".
	r, _ = FnHome(r, &fnDef)
	// Expand optional parameters into additional signatures.
	sigs := ExpandOptionalSigs(name, fnDef.OwnSigs())
	compiled := make([]Signature, 0, len(sigs))
	for _, sig := range sigs {
		s := sig           // capture for closure
		fnDefCopy := fnDef // capture for closure (we need Captured at call time)

		// BarrierPos sentinel resolution (No Zero-Value Overload): -1 =
		// default all-forward → len(Params); 0 = explicit all-stack; >0 =
		// explicit barrier. The `/s` modifier on the def name pins it to 0
		// so the fn is stack-only regardless of its FnSig default.
		barrier := s.BarrierPos
		if isStackOnly {
			barrier = 0
		} else if barrier == BarrierAllForward {
			barrier = len(s.Params)
		}

		cs := s // keep Params (names), Returns, NoEval*, Impl
		// boru-bodied sigs get the body-splicing runner in a fresh BoruImpl. A sig
		// that already carries a Go handler with NO boru Body is a pre-compiled /
		// synthetic overload — a `usurp` wrapper (whose handler re-dispatches
		// the wrapped fn through a paren) or a captured native fn value bound
		// to a name. Wrapping the body-runner around its empty Body would
		// emit zero values and fail the return check, so preserve the existing
		// Impl/ReturnsFn instead; the fn it forwards to performs its own
		// arg-binding and return validation. (An already-compiled boru fn
		// re-bound by name still has Body>0, so it correctly gets a fresh
		// body-runner — only Body-less handler sigs are preserved.)
		if s.DispatchHandler() == nil || len(s.Body()) > 0 {
			meta := &FnFrameMeta{
				Name:         name,
				HasGen:       fnDefCopy.Gen != nil,
				InstallNames: fnInstallNames(s, fnDefCopy.Captured),
			}
			cs.Impl = &BoruImpl{
				Body:     s.Body(),
				FnFrame:  meta,
				dispatch: buildFnBodyHandler(r, name, s, fnDefCopy, meta),
			}
			cs.ReturnsFn = r.analysisReturnsFn(name, s, fnDefCopy)
		}
		cs.BarrierPos = barrier
		NormalizeSig(&cs)
		compiled = append(compiled, cs)
	}
	return compiled
}

// UninstallFnSigs removes specific function signatures from a word's DefStack.
// For each spec in the FnUndefInfo, it finds and removes the most recent
// DefStack entry whose own overloads include a matching signature. Removing
// the entry is sufficient — the dispatch table is rebuilt on demand by
// Registry.Lookup → aggregateDispatch from whatever entries remain.
func UninstallFnSigs(r *Registry, name string, specs FnUndefInfo) {
	// Unconditional, and deliberately WIDER than the twin notes below: those
	// fire only when a removal COMMITS, so a sig-undef whose every match is
	// locked would notify nothing while still being a rebind the user wrote.
	noteRebind(r, name)
	stack := r.Defs.Stack(name)
	if len(stack) == 0 {
		return
	}
	stack = append([]Value(nil), stack...)

	// For each spec, find and remove the most recent matching DefStack entry.
	// Each removal is COMMITTED and NOTED individually: a sig-undef never
	// reaches UninstallDef (it rewrites the stack in place, possibly
	// mid-stack), yet every removal is a runtime-visible transition of its
	// own — depth delta -1, Depth recorded after the commit so it is the
	// post-transition depth, and the note carrying the REMOVED entry so a
	// twin can remove that identical entry rather than guess by position.
	// A sig-undef whose every match is locked commits and notes NOTHING — a
	// no-op is not a transition. (Both arms used to record one net-zero
	// BindDefReplace; the conflation hid a live depth incoherence the corpus
	// could not show — a plain two-overload fn's sig-undef took the name
	// from depth 2 to 1 while recording delta 0.)
	for _, spec := range specs.Sigs {
		for j := len(stack) - 1; j >= 0; j-- {
			fnDef, ok := stack[j].Data.(FnDefInfo)
			if !ok {
				continue
			}
			// Locked signatures can never be removed — a targeted undef
			// spec matching a native registration (or a word-extension
			// clone, which carries the locked base sigs) must not drop
			// the entry (design/OPEN-WORDS.0.md §2.3).
			if hasLockedSig(fnDef.Signatures) {
				continue
			}
			matched := false
			for _, sig := range fnDef.OwnSigs() {
				if FnSigMatchesSpec(sig, spec) {
					matched = true
					break
				}
			}
			if matched {
				rm := stack[j]
				stack = append(stack[:j], stack[j+1:]...)
				r.Defs.Set(name, stack)
				r.NoteBindTransitionEntry(BindSigUndef, name, SrcPos{},
					DefEntry{Body: rm})
				break
			}
		}
	}
}

// CoerceBoolean converts any value to a boolean by presence, not by
// parsing content — the same rule `convert Boolean` and `make Boolean`
// apply: booleans pass through, numbers are non-zero, none is false,
// lists/maps are non-empty, and a String is false only when empty (its
// characters are never inspected, so "false" and "true" both coerce to
// true). The sole content check is the tail carve-out below: a non-String
// value that RENDERS as "false" is an unresolved boolean literal reaching
// truthiness as a Word/Atom (a bare `false` clause, a quoted `false`
// atom) and keeps its boolean reading.
//
// A type may OVERRIDE all of that with the Truther capability
// (truthy.go) — the cascade below is the default, not the only answer.
// No kernel type implements Truther, so the walk is inert until
// something opts in.
func CoerceBoolean(v Value) bool {
	if res, ok := truthinessOf(v); ok {
		return res
	}
	switch {
	case ValueType(v).ConformsTo(TBoolean):
		b, _ := AsBoolean(v)
		return b
	case ValueType(v).ConformsTo(TNumber):
		// AsNumber REFUSES the arbitrary-precision leaves rather than
		// projecting them (value.go: "use AsFloatApprox for a lossy
		// float64"), so its error must not be dropped here — the
		// accompanying zero would read as a real magnitude and make
		// EVERY Big value falsy, `0d1` included, while `0d1 eq 1` is
		// true (NUR055). Test the exact value instead; AsFloatApprox
		// would not do, since a small enough BigDecimal underflows.
		if numIsBig(v) {
			return !bigNumIsZero(v)
		}
		n, _ := AsNumber(v)
		return n != 0
	case ValueType(v).Equal(TNone):
		return false
	// nodeFamily folds FlexList / FlexMap into their immutable parents
	// so empty flex containers are falsey exactly like empty plain
	// ones (PR #123 review note).
	case nodeFamily(ValueType(v)).Equal(TList):
		if !IsConcrete(v) {
			return false
		}
		if elems, err := AsMutableList(v); err == nil {
			return len(elems) > 0
		}
		// Non-[]Value list backings (table types, query builders) are truthy.
		return true
	case nodeFamily(ValueType(v)).Equal(TMap):
		if !IsConcrete(v) {
			return false
		}
		if om, err := AsMutableMap(v); err == nil {
			return om.Len() > 0
		}
		// Non-*OrderedMap map backings (record/options/child types) are truthy.
		return true
	}
	text := ValToString(v)
	// A String is falsy only when empty. The former magic-token rule —
	// where the String "false" was the one non-empty string treated as
	// falsy, so "FALSE"/"0"/"no" were truthy but "false" was not — is
	// removed (WAT-AUDIT.5.md §E): string CONTENT is never inspected.
	if ValueType(v).ConformsTo(TString) {
		return text != ""
	}
	// A non-String that renders as "false" is an unresolved boolean
	// literal reaching truthiness as a Word/Atom (a bare `false` clause
	// condition in `if [false …]`, a quoted `false` atom). It keeps its
	// boolean reading; everything else is truthy unless it renders empty.
	switch text {
	case "false", "":
		return false
	default:
		return true
	}
}

// CowSet performs a copy-on-write set on a Store. It creates a new Store
// layer whose prototype is the old Store, sets the key in the new layer,
// and propagates the update up through parent Stores to the ctxStack.
func CowSet(store *StoreInstanceInfo, key string, val Value, r *Registry) {
	// Create new COW layer: only the changed key, prototype = old store.
	newStore := &StoreInstanceInfo{
		TypeName:  store.TypeName,
		Data:      map[string]Value{key: val},
		Prototype: store,
		Parent:    store.Parent,
		ParentKey: store.ParentKey,
	}

	// Track parent for nested Store values.
	if childStore, ok := val.Data.(*StoreInstanceInfo); ok {
		childStore.Parent = newStore
		childStore.ParentKey = key
	}

	// Propagate up the parent chain: each parent Store gets a new COW
	// layer with the updated child reference.
	current := newStore
	parent := store.Parent
	parentKey := store.ParentKey

	for parent != nil {
		newParent := &StoreInstanceInfo{
			TypeName:  parent.TypeName,
			Data:      map[string]Value{parentKey: NewStoreValue(nil, current)},
			Prototype: parent,
			Parent:    parent.Parent,
			ParentKey: parent.ParentKey,
		}
		current.Parent = newParent
		current.ParentKey = parentKey

		current = newParent
		parentKey = parent.ParentKey
		parent = parent.Parent
	}

	// current is the topmost COW'd Store. Update the ctxStack entry that
	// references the original store (either directly or via prototype chain).
	// The topmost COW'd store's prototype is the original root store.
	// Walk each ctxStack entry's prototype chain to see if it passes
	// through the original root, and if so, create a new ctxStack entry
	// that uses the COW'd store.
	// current's prototype is never nil here: current is either the
	// initial COW layer (prototype = the store argument) or the last
	// loop's newParent (prototype = a parent the loop condition proved
	// non-nil).
	r.Contexts.UpdateChain(current.Prototype, current)
}

// CowDel performs a copy-on-write delete on a Store — the `del` twin of
// CowSet, and deliberately the same shape: a new layer over the old store,
// then the same parent-chain propagation and ctxStack rebind.
//
// The one difference is what the new layer carries. CowSet writes the key
// into Data; CowDel writes it into Deleted, because the key may live in a
// prototype layer that other scopes still share and must not be edited.
// Get honours the tombstone, so the key reads as absent from this layer
// down. Deleting a key that was never present is a no-op layer — harmless,
// and it keeps `del` idempotent and total, matching the Map/FlexMap forms.
func CowDel(store *StoreInstanceInfo, key string, r *Registry) {
	newStore := &StoreInstanceInfo{
		TypeName:  store.TypeName,
		Deleted:   map[string]bool{key: true},
		Prototype: store,
		Parent:    store.Parent,
		ParentKey: store.ParentKey,
	}

	current := newStore
	parent := store.Parent
	parentKey := store.ParentKey

	for parent != nil {
		newParent := &StoreInstanceInfo{
			TypeName:  parent.TypeName,
			Data:      map[string]Value{parentKey: NewStoreValue(nil, current)},
			Prototype: parent,
			Parent:    parent.Parent,
			ParentKey: parent.ParentKey,
		}
		current.Parent = newParent
		current.ParentKey = parentKey

		current = newParent
		parentKey = parent.ParentKey
		parent = parent.Parent
	}

	// current.Prototype is non-nil for the same reason it is in CowSet:
	// current is either newStore (prototype = store) or the last newParent
	// (prototype = a parent the loop condition proved non-nil).
	r.Contexts.UpdateChain(current.Prototype, current)
}

// IsHostTypeBody reports whether v is a constructed type produced by a
// host Ideal: an ExtensionPayload whose Body embeds eng.HostTypeBody.
// The kernel recognises such a value as a type without inspecting its
// concrete shape (the payload Body being opaque). See
// design/IDEAL.10.md §6.
func IsHostTypeBody(v Value) bool {
	ep, ok := v.Data.(ExtensionPayload)
	if !ok {
		return false
	}
	_, ok = ep.Body.(interface{ hostTypeBody() })
	return ok
}

// IsTypeBody reports whether a value is a valid type definition body
// in the strict, structural sense: it carries explicit type-shape
// information (a type literal, disjunct, record / table / object /
// options type, typed list/map, dependent scalar, fn-shape, or
// predicate function).
//
// boru also lets every concrete value act as a type — `type Foo 1`
// defines Foo as the singleton type containing only 1 — but that
// "literals as types" admission is checked separately via
// IsLiteralTypeBody at the `type` install site, so paths that need
// to discriminate code-bodies / fn-bodies / data-defs from explicit
// type shapes (e.g. `inspect`) keep using IsTypeBody and stay sharp.
func IsTypeBody(v Value) bool {
	// A bare lattice node IS a type; everything else asks its sealed
	// payload through the one recognition seam (Payload.IsTypeContent,
	// design/TYPE-REPRESENTATION.1.md §N4). The 18-arm shape
	// enumeration this replaced is pinned as the equivalence oracle in
	// TestIsTypeContentMirrorsLegacy.
	if IsBareTypeNode(v) {
		return true
	}
	// The fn-shape and predicate arms are DISPATCH-IDENTITY facts, not
	// payload facts: any value at the FunctionSignature or Function
	// identity is a type body (a fn shape / predicate candidate),
	// Data-bearing or check-mode carrier alike — exactly the legacy
	// enumeration's parent-keyed arms.
	if v.Parent != nil && (v.Parent.Equal(TFnUndef) || v.Parent.Equal(TFunction)) {
		return true
	}
	return v.Data != nil && v.Data.IsTypeContent(&v)
}

// PredicateInputType returns the concrete input type of a
// predicate-shaped fn body (a Function whose first sig
// takes exactly one argument with a declared type other than Any).
// Returns nil if v isn't a predicate type or the input type is Any
// or unset — those bodies stay parented at TFunction, the
// pre-existing behavior.
//
// Used by InstallType to mint user-defined predicate types with the
// declared input type as their parent so values rewrapped by the
// typed-bind path participate in the LCA-walk dispatch alongside
// kernel scalars. Without this, `behave compare/q (fn [[Positive
// Positive] …])` would have no dispatch surface — no value's Parent
// is ever Positive.
func PredicateInputType(v Value) *Type {
	if v.Parent == nil {
		return nil
	}
	if !v.Parent.Equal(TFunction) {
		return nil
	}
	info, ok := v.Data.(FnDefInfo)
	if !ok {
		return nil
	}
	sig, ok := info.FirstOwnSig()
	if !ok || len(sig.Params) == 0 {
		return nil
	}
	// The parameter-COUNT test is the DEPRECATED route (NUR099/NUR100):
	// ADR-016 forbids arity deciding how a function behaves. A `fnpred`
	// declaration carries the fact explicitly and is believed whatever its
	// shape; the count is consulted only for a body that never said so.
	if !info.Predicate && len(sig.Params) != 1 {
		return nil
	}
	t := sig.Params[0].Type
	if t == nil || t.Equal(TAny) {
		return nil
	}
	return t
}

// IsLiteralTypeBody reports whether v can be installed as a "value-
// is-a-type" type body — the singleton-type interpretation. Scalar
// literals (Integer / Float / String / Boolean / Atom / Pathon / the
// `none` value), and concrete lists / maps qualify. Used by
// installType to relax the strict IsTypeBody check in a way that
// doesn't pollute the inspect / fn-shape paths.
func IsLiteralTypeBody(v Value) bool {
	if IsNone(v) {
		return true
	}
	switch {
	case v.Parent.ConformsTo(TInteger),
		v.Parent.ConformsTo(TFloat),
		v.Parent.ConformsTo(TNumber),
		v.Parent.ConformsTo(TString),
		v.Parent.ConformsTo(TBoolean),
		v.Parent.ConformsTo(TAtom),
		v.Parent.ConformsTo(TMicron):
		return v.Data != nil
	}
	if v.Parent.Equal(TList) && v.Data != nil {
		return true
	}
	if v.Parent.Equal(TMap) && v.Data != nil {
		return true
	}
	return false
}

// ResolveWordValue converts a word value to its semantic value.
// Words named "true"/"false" become booleans, known type names become type
// literals, and other words become atoms (bare strings). Registry-free —
// runs only the builtin arm of the canonical cascade (resolve.go); sites
// with a Registry in hand use ResolveWordValueR.
func ResolveWordValue(v Value) Value {
	return resolveWordValue(v, nil)
}

// SimplifyDisjunctAlts filters Never, dedupes structurally identical
// alternatives, and applies subsumption: a strict subtype drops in
// favour of its supertype, and a concrete value drops if some other
// alternative is a covering type literal. Two concrete values of the
// same type are both kept — each one is a distinct piece of
// information that the type literal couldn't replace.
func SimplifyDisjunctAlts(alts []Value) []Value {
	// First pass: drop Never.
	live := make([]Value, 0, len(alts))
	for _, alt := range alts {
		if ValueType(alt).Equal(TNever) {
			continue
		}
		live = append(live, alt)
	}
	// Second pass: keep an alt only if no other live alt subsumes or
	// duplicates it. "Earlier-wins" for duplicates so source order is
	// preserved among survivors.
	out := make([]Value, 0, len(live))
outer:
	for i, cand := range live {
		candType := ValueType(cand)
		// Drop if structurally equal to an earlier kept alt.
		for j := 0; j < i; j++ {
			if ValueType(live[j]).Equal(candType) && ValuesEqual(live[j], cand) {
				continue outer
			}
		}
		// Drop if subsumed by some other alt:
		//   - cand is a type literal whose type is a strict subtype
		//     of another's (Integer subsumed by Number).
		//   - cand is a concrete value covered by another type literal
		//     (5 subsumed by Integer).
		// Strict subtype only: equal types are handled by dedup above.
		for j, other := range live {
			if i == j {
				continue
			}
			otherType := ValueType(other)
			if candType.Equal(otherType) {
				continue
			}
			if !candType.ConformsTo(otherType) {
				continue
			}
			// cand's type is a strict subtype of other's.
			if IsBareTypeNode(cand) && IsBareTypeNode(other) {
				continue outer
			}
			if IsConcrete(cand) && IsBareTypeNode(other) {
				continue outer
			}
		}
		out = append(out, cand)
	}
	// Canonicalise: `tor` is commutative, so a type union's terms are
	// stored in tcmp order. This is the single boundary every user-facing
	// disjunction passes through (TorHandler / unionType / exclude /
	// extract / the check-mode carrier merge), so `None tor 1` and
	// `1 tor None` reduce to the identical value — equal under `tcmp`,
	// identical when printed.
	sort.SliceStable(out, func(i, j int) bool {
		return disjunctAltLess(out[i], out[j])
	})
	return out
}

// FnDefsOverlap returns true if any signature in a has the same parameter
// types as any signature in b (ignoring param names, return types, and body).
func FnDefsOverlap(a, b FnDefInfo) bool {
	for _, sa := range a.OwnSigs() {
		for _, sb := range b.OwnSigs() {
			if len(sa.Params) != len(sb.Params) {
				continue
			}
			match := true
			for i := range sa.Params {
				if !sa.Params[i].Type.Equal(sb.Params[i].Type) {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

// BaseValue returns the zero/default value for a given type, similar to Go's
// zero values. Used by both the "base" word and "make" with base:true option.
func BaseValue(t *Type) (Value, error) {
	switch {
	case t.ConformsTo(TInteger):
		return NewInteger(0), nil
	case t.ConformsTo(TFloat):
		return NewFloat(0), nil
	case t.ConformsTo(TNumber):
		return NewInteger(0), nil
	case t.ConformsTo(TString):
		return NewString(""), nil
	case t.ConformsTo(TBoolean):
		return NewBoolean(false), nil
	case t.ConformsTo(TList):
		return NewList([]Value{}), nil
	case t.ConformsTo(TMap):
		return NewMap(NewOrderedMap()), nil
	case t.ConformsTo(TNone):
		return NewTypeLiteral(TNone), nil
	case t.ConformsTo(TAtom):
		return NewAtom(""), nil
	default:
		return Value{}, fmt.Errorf("base: unsupported type %s", t.String())
	}
}

// BaseValueForConstraint returns the base value for a field constraint.
// For type literals, returns the zero value directly.
// For disjunctions (e.g. string|none), returns the base of the first
// non-none alternative.
func BaseValueForConstraint(constraint Value) (Value, error) {
	if IsDisjunct(constraint) {
		di, _ := AsDisjunct(constraint)
		for _, alt := range di.Alternatives {
			if IsTypeLiteral(alt) {
				return BaseValue(ValueType(alt))
			}
		}
		return NewTypeLiteral(TNone), nil
	}
	if IsBareTypeNode(constraint) {
		return BaseValue(ValueType(constraint))
	}
	return Value{}, fmt.Errorf("base: cannot determine base value for %s", constraint.String())
}

// omittedDefaultValue returns the value substituted for an omitted
// optional FnParam. Options-typed params get a Map populated with
// each Field's concrete default (fields whose value is a type body —
// type literals, disjuncts, nested type definitions — carry no
// default and are skipped). Non-Options params fall back to BaseValue
// of the param's declared Type.
func omittedDefaultValue(p FnParam) (Value, error) {
	if p.Pattern != nil && IsOptionsType(*p.Pattern) {
		oi, err := AsOptionsType(*p.Pattern)
		if err == nil && oi.Fields != nil {
			m := NewOrderedMap()
			for _, k := range oi.Fields.Keys() {
				fv, _ := oi.Fields.Get(k)
				if IsTypeBody(fv) {
					continue
				}
				m.Set(k, fv)
			}
			return NewMap(m), nil
		}
	}
	return BaseValue(p.Type)
}

// ExpandOptionalSigs takes a list of fn signatures and expands those with
// optional params into the full set of overloaded signatures. Each
// optional combination becomes its own signature whose body calls the
// original function with the omitted params filled by their type's
// base value.
func ExpandOptionalSigs(name string, sigs []FnSig) []FnSig {
	var expanded []FnSig
	for _, sig := range sigs {
		expanded = append(expanded, sig)

		var optIndices []int
		for i, p := range sig.Params {
			if p.Optional {
				optIndices = append(optIndices, i)
			}
		}
		if len(optIndices) == 0 {
			continue
		}

		numOpt := len(optIndices)
		for mask := 1; mask < (1 << numOpt); mask++ {
			omitted := make(map[int]bool)
			for bit := 0; bit < numOpt; bit++ {
				if mask&(1<<bit) != 0 {
					omitted[optIndices[bit]] = true
				}
			}

			var reducedParams []FnParam
			for i, p := range sig.Params {
				if !omitted[i] {
					reducedParams = append(reducedParams, FnParam{
						Name:    p.Name,
						Type:    p.Type,
						Pattern: p.Pattern,
					})
				}
			}

			var body []Value
			body = append(body, NewWord(name))
			// invalid latches when an omitted param has no usable
			// default: either none can be synthesized, or the synthesized
			// value fails the param's own pattern — the SAME Unify
			// predicate dispatch applies (patternsOk), so the two can't
			// drift. In either case the omission combination is skipped
			// entirely below. The historical bug here `continue`d on a
			// synthesis error, DROPPING the argument from the body: the
			// 0-arg overload then re-dispatched the word with the arg
			// still missing (or with a default the original sig rejects),
			// which matched the same 0-arg overload again — an infinite
			// self-recursion that ground the tape until exhaustion.
			// Skipping the overload makes the omission fail dispatch
			// honestly instead ("matched no signature").
			invalid := false
			presentIdx := 0
			for i, p := range sig.Params {
				if omitted[i] {
					bv, err := omittedDefaultValue(p)
					if err != nil {
						invalid = true
						break
					}
					if p.Pattern != nil {
						if _, ok := Unify(bv, *p.Pattern); !ok {
							invalid = true
							break
						}
					}
					body = append(body, bv)
				} else {
					if p.Name != "" {
						body = append(body, NewWord(p.Name))
					} else {
						body = append(body,
							NewOpenParen(),
							NewWord("args"),
							NewAtom(fmt.Sprintf("%d", presentIdx)),
							// dot, not get: the key is a literal atom index
							// (`args.N`); get no longer accepts an atom key.
							NewWord("dot"),
							NewCloseParen(),
						)
					}
					presentIdx++
				}
			}

			// An omitted param with no valid default: this omission
			// combination cannot be satisfied — generate NO overload for
			// it, so calls relying on the omission fail dispatch cleanly.
			if invalid {
				continue
			}

			// Propagate the parent sig's BarrierPos so an
			// optional-param expansion inherits the same forward/
			// stack convention. -1 (the boru-source default) stays
			// -1; an explicit barrier carries over but clamps to
			// the reduced param count.
			expandedBarrier := sig.BarrierPos
			if expandedBarrier > len(reducedParams) {
				expandedBarrier = len(reducedParams)
			}
			expanded = append(expanded, FnSig{
				Params:         reducedParams,
				Returns:        sig.Returns,
				ReturnPatterns: sig.ReturnPatterns,
				Impl:           Boru(body),
				BarrierPos:     expandedBarrier,
			})
		}
	}
	return expanded
}

// bigNumIsZero reports whether an arbitrary-precision numeric leaf is
// exactly zero. Split out of CoerceBoolean so the truthiness test never
// routes a Big value through the float64 channel: AsNumber refuses them
// outright, and AsFloatApprox would flatten a sufficiently small
// BigDecimal to 0.0. A value whose accessor fails is not provably zero,
// so it stays truthy — the same direction the non-Big arm takes for an
// unreadable payload.
func bigNumIsZero(v Value) bool {
	if v.Parent.ConformsTo(TBigInteger) {
		bi, err := AsBigInteger(v)
		return err == nil && bi != nil && bi.Sign() == 0
	}
	bd, err := AsBigDecimal(v)
	return err == nil && bd != nil && bd.IsZero()
}
