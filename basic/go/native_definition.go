package basic

import (
	"fmt"
	"strings"

	core "github.com/boru-lang/boru/core/go"
)

// DefinitionNatives covers the binding / function-definition words:
// def, undef, var, fn, args, __pa.
//
// Pure helpers used by these handlers (parseFnDef, parseFnParams,
// MatchFnSig, DefName, DefStackOnly, etc.) live alongside their
// callers in native_definition_fn.go and native_definition_helpers.go.
var DefinitionNatives = []NativeFunc{
	{
		Name: "def",

		Signatures: []Signature{
			{
				// Typed-name binding: def name:*Type body. Sorts first
				// because TMap is more specific than TString / TAtom
				// at the same depth (higher inherent score).
				// The body is auto-evaluated like any value argument: a
				// list binds like a map (`def xs [1 add 2]` → `[3]`). For a
				// raw / spliced body use `def name word value`.
				Args:          []*Type{TMap, TAny},
				NoEvalMapArgs: map[int]bool{0: true},
				Impl:          Go(DefTypedHandler, RunInCheck()),
				Returns:       []*Type{},
				BarrierPos:    -1,
			},
			// The constructor FORMS — `def name fn [...]`, `def name
			// class {…}`, `def name gen [T] fn [...]`, … — are NOT
			// listed here: they are synthesized mechanically from the
			// blessed constructors' own signature tables at the end of
			// Register (RegisterDefKeywordForms), so a constructor
			// overload added later propagates to def automatically.
			{
				Args:       []*Type{TString, TAny},
				Impl:       Go(DefHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom, TAny},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(DefHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
		},
	},
	{
		Name: "undef",

		Signatures: []Signature{
			{
				Args:       []*Type{TString},
				Impl:       Go(undefHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(undefHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TString, TFnUndef},
				Impl:       Go(UndefFnHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom, TFnUndef},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(UndefFnHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
		},
	},
	{
		// __varundef is the cleanup unbind the `var` splice emits — semantically
		// ALWAYS the single-name form (`undef name`). It exists separately from
		// `undef` precisely because `undef` is OVERLOADED with a 2-arg fn-overload
		// form (`undef name fnUndefSpec`): the var splice runs the cleanup with the
		// body's RESIDUAL still on the stack, and in check mode that residual is a
		// dynamic-Any carrier which gradually matches the 2-arg form's TFnUndef
		// slot — so `undef name` mis-dispatched to UndefFnHandler and errored
		// ("expected fn undef spec"), leaking the loop binding and declining the
		// closure. A dedicated 1-arg-only word can never mis-match the residual, so
		// it dispatches identically (1-arg unbind) in check mode and at runtime —
		// the property the compiled `each`/`fold`/… var-body closure needs. Reuses
		// undefHandler so the unbind behaviour is byte-identical to `undef name`.
		Name: "__varundef",
		Signatures: []Signature{
			{
				Args:       []*Type{TString},
				Impl:       Go(varUndefHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(varUndefHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
		},
	},
	{
		Name: "var",

		// var SPLICES its body (def/body/undef tokens) onto the tape for the
		// engine to re-step. RunInCheckMode lets the recorder follow that splice
		// so the inline let lowers as the body's events with the bound names as
		// promoted value-def locals (the def/body/undef tokens record exactly as a
		// hand-written `def NAME val end … undef NAME` would). A body word the
		// recorder cannot lower marks the program uncompilable through the same
		// path it does anywhere else, so a declining body DECLINES rather than
		// producing a silent empty unit.
		Signatures: []Signature{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(VarHandler, RunInCheck()),
			Returns:    []*Type{TAny}, BarrierPos: -1,
		}},
	},
	{
		Name: "fn",

		// Two forms, matched longest-first: the 3-arg single-triple
		// form `fn input output body`, then the 1-arg spec-list form
		// `fn [[input] [output] [body] …]`. The triple form's input
		// slot carries a `tnot List` Pattern: a list input ALWAYS
		// means the spec-list form, so a list following `fn` fails the
		// triple candidate at patternsOk and dispatch falls through to
		// the 1-arg sig — existing spec-list calls keep their meaning
		// even with extra values on the stack. The body slot is
		// List-typed (not Any): a triple-form body is always a `[…]`
		// code list, which keeps the greedy 3-slot window from
		// claiming a non-list value as a body in mixed/stack
		// arrangements and makes a truncated `fn input output` fail
		// loudly instead of absorbing a following value. NoEvalArgs on
		// all three slots (the input pair, the output types, and the
		// body are spec/code, not data).
		Signatures: []Signature{
			{
				Args:       []*Type{TAny, TAny, TList},
				Patterns:   map[int]Value{0: NewNegation(NewTypeLiteral(TList))},
				NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
				// NoEvalMapArgs as well, because NoEvalArgs is LIST-only
				// and the shorthand input `x:IS` IS the map: without this
				// autoEvalMap dispatches the type name and pushes its
				// BODY (a Disjunct value, not a lattice node), erasing the
				// declared type before ParseFnParams ever sees it. `def`'s
				// typed-name sig carries the same suppression for the same
				// reason — see its comment above.
				NoEvalMapArgs: map[int]bool{0: true},
				// Slot 1 (the OUTPUT sig) has the mirror-image problem and
				// is deliberately NOT fixed here: a bare Word there is
				// resolved by forward collection itself, which no NoEval
				// flag gates, so a declared union return still arrives as a
				// Disjunct VALUE, resolves to TAny + a pattern that
				// ParseFnReturns has nowhere to put, and goes unenforced.
				// QuoteArgs is NOT the answer — it changes fn's dispatch
				// barrier and breaks every synthesized `def … fn …` form
				// (measured). Closing it needs return patterns on FnSig;
				// see design/verse-report-defects-investigation.0.md §F.
				Impl:       Go(FnTripleHandler, RunInCheck()),
				Returns:    []*Type{TFunction},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(FnHandler, RunInCheck()),
				Returns:    []*Type{TFunction},
				BarrierPos: -1,
			},
			{
				// The 0-argument spelling is never a construction: `fn` with
				// nothing it can take is a declaration the two forms both
				// rejected — `def f fn List Any [1]` used to fall to the
				// synthesized 0-arg fallback and strand its operands silently
				// (`[1] Any List` left behind, nothing bound, exit 0), where
				// `fn List [Integer] [size]` only failed loudly by accident,
				// the body list running as code (NUR091). It raises instead,
				// naming the rule.
				Args:          []*Type{},
				Impl:          Go(FnNoArgsHandler, RunInCheck()),
				Returns:       []*Type{TFunction},
				BarrierPos:    -1,
				CompileEffect: CompileDiverges, // the handler always raises
			},
		},
	},
	{
		// `afn` is the canonical anonymous-fn constructor. The parser
		// folds `A => B` into the group `(A afn B)`. Sig is [Any Any |]
		// — both args forward-eligible, both typed Any. NoEvalArgs on
		// both so the input sig isn't auto-evaluated and the body's
		// words aren't dispatched before construction. RawParens on the
		// body slot (position 0): a function body is CODE, so a paren
		// body is captured RAW and evaluates per CALL with the params
		// bound — this is what makes the chained arrow curry,
		// `x:Integer => y:Integer => [x add y]` ≡
		// (x ⇒ (y ⇒ body)): the inner lambda constructs inside the
		// outer body, capturing x, instead of eagerly at outer-
		// construction time when x doesn't exist yet. It also lets a
		// paren body reference params at all: `x:Integer => (x mul 2)`.
		Name: "afn",

		Signatures: []Signature{{
			Args:       []*Type{TAny, TAny},
			NoEvalArgs: map[int]bool{0: true, 1: true},
			// Slot 1 is the INPUT sig (afn's canonical call is the swap
			// `input afn body`), so it needs the same map-eval suppression
			// `fn`'s triple slot 0 carries: `x:IS => …` would otherwise
			// lose IS to autoEvalMap. See fn above.
			NoEvalMapArgs: map[int]bool{1: true},
			RawParens:     map[int]bool{0: true},
			Impl:          Go(AfnHandler, RunInCheck()),
			Returns:       []*Type{TFunction},
			BarrierPos:    -1,
		}},
	},
	{
		Name: "fnsig",

		// Two forms, matched longest-first, exactly mirroring `fn` one
		// slot shorter — a function TYPE is a function minus its body:
		//
		//	fn    input output body   →  fnsig input output
		//	fn    [[in] [out] [body]] →  fnsig [[in] [out]]
		//
		// The 2-arg pair form's input slot carries the same `tnot List`
		// Pattern as fn's triple: a list input ALWAYS means the
		// spec-list form, so `fnsig [[Integer] [String]]` keeps its
		// meaning even with extra values on the stack. NoEvalArgs /
		// NoEvalMapArgs on both slots for the reasons fn documents —
		// these are spec, not data.
		Signatures: []Signature{
			{
				Args:          []*Type{TAny, TAny},
				Patterns:      map[int]Value{0: NewNegation(NewTypeLiteral(TList))},
				NoEvalArgs:    map[int]bool{0: true, 1: true},
				NoEvalMapArgs: map[int]bool{0: true},
				Impl:          Go(FnsigPairHandler, RunInCheck()),
				Returns:       []*Type{TFnUndef}, BarrierPos: -1,
			},
			{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(FnsigHandler, RunInCheck()),
				// Pure construction — runs in check mode too, so surface
				// schemas carry REAL shapes statically and `exposes` is
				// fully static-checkable (design/legacy/SURFACES.10.ignore S2). A
				// pending gen spec turns the result into a generic
				// fn-shape schema (see the handler).
				Returns: []*Type{TFnUndef}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "fnpred",

		// `fn` one slot shorter, the OTHER way from fnsig. A function
		// TYPE is a function minus its body; a predicate TYPE is a
		// function minus its OUTPUT, because a predicate's output is not
		// a choice — it is membership:
		//
		//	fn      input output body   →  the whole function
		//	fnsig   input output        →  minus the body   (a function TYPE)
		//	fnpred  input        body   →  minus the output (a predicate TYPE)
		//
		// Two forms, matched longest-first, exactly as fnsig has: the
		// 2-arg pair form `fnpred n:Integer [eq 0 (mod 2 n)]`, and the
		// spec-list form `fnpred [[n:Integer] [eq 0 (mod 2 n)]]` for an
		// overload set. The pair form's input slot carries fn's `tnot
		// List` Pattern so a list input ALWAYS selects the spec-list
		// form. NoEvalArgs on both slots for the reasons fn documents —
		// these are spec and code, not data.
		//
		// It exists so a membership test can SAY it is one. Without it,
		// the only route to an arbitrary predicate type is a capitalised
		// `def` over a fn body, which makes the CASE of a name decide
		// whether a body is a callable function or a membership test, and
		// makes the parameter COUNT decide whether the binding is a
		// predicate at all — an arity-keyed exception ADR-016 forbids.
		// NUR099.
		Signatures: []Signature{
			{
				Args:          []*Type{TAny, TAny},
				Patterns:      map[int]Value{0: NewNegation(NewTypeLiteral(TList))},
				NoEvalArgs:    map[int]bool{0: true, 1: true},
				NoEvalMapArgs: map[int]bool{0: true},
				Impl:          Go(FnpredPairHandler, RunInCheck()),
				Returns:       []*Type{TFunction}, BarrierPos: -1,
			},
			{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(FnpredHandler, RunInCheck()),
				// Pure construction, like fnsig: it runs in check mode so a
				// predicate type declared in a body is a REAL type
				// statically, not an Any carrier.
				Returns: []*Type{TFunction}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "args",

		Signatures: []Signature{{
			Impl:    Go(ArgsHandler),
			Returns: []*Type{TList}, BarrierPos: -1,
		}},
	},
	{
		Name: "__pa",

		Signatures: []Signature{{
			Impl:    Go(PopArgsHandler),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	},
}

// ---- def ----

// InstallAndRecordDef binds name→value and records the def for check-mode
// unused-def analysis at pos (the def-name token's position). Every
// successful def / typed-def branch ends with this exact pair followed by a
// `return nil, nil`, so it is consolidated here. The optional stackOnly flag
// is forwarded to InstallDef (only the plain `def` path sets it).
func InstallAndRecordDef(r *Registry, name string, value Value, pos SrcPos, stackOnly ...bool) ([]Value, error) {
	// Stage the def SITE for the bind ledger: this is the only frame that knows
	// it (§6.5). SAVE/RESTORE rather than set — a fn's construction body
	// analysis installs its own body-locals inside this call, and each must see
	// its own site without disturbing this one.
	if r != nil && r.Check != nil {
		prevBindPos := r.Check.PendingBindPos
		r.Check.PendingBindPos = pos
		defer func() { r.Check.PendingBindPos = prevBindPos }()
	}
	// A def's own INSTALL must not count as a USE of the name. Installing a
	// fn runs its construction-time body pass, which resolves the fn's own
	// name (recursion support) and records a spurious self-use; an uncalled
	// fn would then never be flagged unused now that RecordDef no longer
	// resets uses on rebind. Snapshot this name's use flag across
	// install+record and restore it, so only EXTERNAL references count. A
	// prior legitimate use (e.g. a loop-carried read before a rebind) is
	// preserved (prevUsed stays true); a fresh def's self-use is undone.
	checking := r.Check.IsActive()
	prevUsed := checking && r.Check.DefsUsed != nil && r.Check.DefsUsed[name]
	// S5 (COMPILE FAILURE-CLOSURE): a top-level def of a STATICALLY-COUNTED variadic
	// loop region binds the region's FIRST value — the interpreter's pending
	// forward collects the first-arrived value and the rest spill. The
	// binding takes the element carrier; the region carrier itself returns
	// to the check stack as the N-1 REST residual (still produced by the
	// loop event, so the variadic disposition owns it as today).
	var outs []Value
	if elem, split := r.Check.Recorder().SplitLoopRegionBind(name, value); split {
		outs = []Value{value}
		value = elem
	} else if elem, split := r.Check.Recorder().SplitEventRegionBind(name, value); split {
		// The S9.1 static-region twin: a def binding the FIRST result of a
		// fallible do-catch's multi-value region. The event's rest results
		// stay on the check stack by construction (only idx 0 binds), so no
		// outs push — the splice lowering removes the bound value at depth.
		value = elem
	}
	InstallDef(r, name, value, stackOnly...)
	r.Check.RecordDef(name, pos)
	if checking {
		if r.Check.DefsUsed == nil {
			r.Check.DefsUsed = map[string]bool{}
		}
		if prevUsed {
			r.Check.DefsUsed[name] = true
		} else {
			delete(r.Check.DefsUsed, name)
		}
	}
	// Mark a computed binding for value-def-local promotion in the bytecode
	// lowerer: a named value may be referenced in any order, so its producing
	// event is stored to a frame local rather than left on the simulated stack.
	r.Check.Recorder().MarkValueDef(value)
	// A rebind of a LOOP-CARRIED def (a pre-loop binding an armed for-body
	// rebinds — the checker's AnalyseLoopBody registered it via NoteLoopCarried) stores
	// the new value into its frame slot at THIS site, so a conditional rebind
	// updates the cell exactly when its arm runs. No-op for every other def.
	r.Check.Recorder().RecordDefRebind(name, value, pos)
	// Record the def site for the dynamic-scope binder pass: if some fn body
	// READS this name with no lexical home (OpLookupDynScope), the lowering
	// installs a registry-visible OpBindDynScope twin here so the runtime
	// lookup finds the binding the interpreter's def stack would hold.
	r.Check.Recorder().RecordDynBind(name, value, pos)
	// A Function-FAMILY value with no FnDefInfo payload — a computed parser
	// / transducer / emitter under analysis (`def op (Parse.parser g)`) —
	// installs NO Defs binding (installDef's fn arm declines so the compiled
	// closure machinery keeps sole ownership of the name). The parse / mini /
	// emit value-form macros still need the NAME to resolve as
	// "a function is bound here", so record the carrier in the per-pass
	// fn-carrier side table they consult.
	if checking && !IsConcrete(value) &&
		value.Parent.ConformsTo(TFunction) {
		// SHADOWING a live binding. The computed fn is not installed in
		// Defs (the arm below), so the name now denotes different things
		// in the two stores: the carrier here, and whatever `def` bound
		// earlier in Defs. The compiled program binds only the latter, so
		// once anything pops or reads through the shadow the lanes part —
		// `def f 1 ; def f (mk 1) ; undef f ; (f 2)` compiled `1 2` where
		// the interpreter answers 3, because the interpreter's `def` bound
		// the closure over the 1 and its `undef` left fn bindings alone.
		// Decline rather than model a name with two meanings.
		// The same two-store disagreement when the live binding is ITSELF
		// a computed fn (the side table's, not Defs') and this def sits in
		// a FN BODY: the interpreter's install drops the overlapping outer
		// closure and pushes the new one at the same depth, so the frame's
		// def-cleanup pops nothing and the redefinition OUTLIVES the call
		// (`def a5 (mk 1)  def g fn [[xs:List][List][def a5 (mk 5)  each
		// [a5] xs]]  g [1 2 3]  each [a5] [1 2 3]` is `[[6 7 8] [6 7 8]]`
		// interpreted), where the compiled frame's bind is popped with the
		// frame and the outer closure answers again (NUR192, 2026-09-24). A
		// redefinition in the SAME frame (the body re-analysed by a unit
		// compile, a second `def` in one body) is bound at this depth and
		// stays: the frame's cleanup pops both on both lanes.
		_, shadowed := r.Defs.Top(name)
		if !shadowed && r.Check.FnBodyDepth > 0 {
			if depth, bound := CheckFnCarrierBindDepth(r, name); bound && depth < r.Check.FnBodyDepth {
				shadowed = true
			}
		}
		if shadowed {
			r.Check.Recorder().MarkUncompilable(
				"computed fn shadows a live binding of the same name (two binding stores disagree — Stage 1)")
		}
		// A DROPPED APPLY: this def binds the very carrier another name
		// already denotes, which means the body's apply was not modeled —
		// the analysis returned the callee unchanged. `def f2 (f1 2)` over
		// a curried factory is the shape; compiled, both names take one
		// slot and the unconsumed argument leaks into the residual
		// (`2 fn (Integer) 3` where the interpreter answers `6`). Decline
		// rather than leak: the program is then silently interpreted, which
		// hides this failure instead of fixing it.
		if prev, dup := CheckFnCarrierBoundName(r, value.ID); dup && prev != name {
			r.Check.Recorder().MarkUncompilable(
				"def of a computed fn whose apply the analysis dropped (curried chain — Stage 1)")
		}
		NoteCheckFnCarrierBind(r, name, value)
	}
	return outs, nil
}

// The per-check-pass fn-carrier side table (NoteCheckFnCarrierBind /
// CheckFnCarrierBind / ResetCheckFnCarrierBinds) moved DOWN to
// core/go/check_fncarrier.go: the engine's compile-pass undefined-word
// branches (stepWord / stepWordVal) consult it alongside the parse/mini/
// emit value-form macros, and core cannot reach a basic symbol. The
// aliases in aliases.go keep this package's historical spellings.

// defKeywordConstructors is the CLOSED SET of constructor words whose
// bare form after a def name is a declared def signature — the KEYWORD
// form `def name <ctor> …` (keyword slots: a /q position with a
// concrete Atom pattern admits exactly one literal word; see
// patternsOk, eng/go/match.go, and design/STRICT-FORWARD-BARRIER.0.md).
// Each constructor's OWN signatures are mirrored mechanically, shifted
// past the [name/q ctor/q] prefix, so the form is ordinary structural
// dispatch: every operand resolves at plan time, nothing parks, and
// the strict barrier rule needs no wait-through for these idioms.
// `make` is deliberately ABSENT: it is the one INSTANCE constructor
// (per-call fresh identity — ReturnsFreshInstance, OpMakeMap events),
// and routing it through the composite form hides the inner dispatch
// from the bytecode recorder, losing the operand provenance compiled
// programs depend on (`def p0 make Pointer.Point {…} … (p0 add p1)`
// fails to compile). Its bare def form keeps today's wait-through
// path; a keyword form needs recorder plumbing first.
var defKeywordConstructors = []string{
	"fn", "fnsig", "fnpred", "refine", "class", "surface", "enum", "quote", "word",
}

// defGenChainTails are the constructors that consume a pending `gen`
// spec — `def name gen [params] <tail> …` mirrors as a chain form with
// TWO keyword slots ([name/q gen/q List tail/q …]). The `def Name<T>`
// angle sugar desugars to exactly this token shape (parser/parse.go).
var defGenChainTails = []string{"fn", "class", "refine", "fnsig"}

// DefFormVia returns the run implementation for a synthesized def
// keyword overload: construct the bound value by dispatching the
// captured constructor's own signature over the operands after the
// keyword — the capture is binding-agnostic like every /q slot, so
// the builtin constructor semantics apply regardless of shadowing —
// then delegate binding to the ordinary DefHandler path (capitalised-
// name / extension / value-bind branches behave identically to a
// value that arrived at a parked def). genChain forms run gen's
// handler over the params list first, so the tail constructor picks
// up the pending gen spec exactly as in the wait-through sequence.
func DefFormVia(base *Signature, offset int, genChain bool) func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
	return func(args []Value, named map[string]Value, stack []Value, r *Registry) ([]Value, error) {
		ctorArgs := args[offset:]
		if genChain {
			if _, err := GenHandler([]Value{args[2]}, nil, nil, r); err != nil {
				return nil, err
			}
			// The chain signature DEFERRED evaluation of the tail's
			// operands (a schema map / list may reference the gen
			// placeholders — `gen [T] class {value:T}`). Re-apply the
			// tail constructor's own evaluation policy now that the
			// placeholder bindings are installed, exactly as execMatch
			// would have at the tail's own dispatch in the wait-through
			// sequence — including execMatch's pending-gen suspension,
			// so a constructor nested INSIDE an operand (a record
			// field's `(fnsig […])`) cannot steal the spec destined
			// for the tail constructor.
			restore := r.SuspendPendingGen()
			defer restore()
			ctorArgs = append([]Value(nil), ctorArgs...)
			for i := range ctorArgs {
				if base.NoEvalArgs[i] || base.FormArgs[i] || base.NoEvalMapArgs[i] {
					continue
				}
				v := ctorArgs[i]
				// Mirror execMatch's auto-eval admission: unquoted concrete
				// operands, and only REAL OrderedMap maps — Record/Options/
				// typed-map payloads share Parent=TMap but are not
				// evaluatable data (the documented AsMap-nil trap). The
				// parser's Eval flag is NOT consulted: it is consumed on
				// the deferred-collection path before the handler runs,
				// and these positions are by construction parser operands
				// the tail's own execMatch would have evaluated.
				if v.Quoted || !IsConcrete(v) {
					continue
				}
				var err error
				switch {
				case v.Parent.Equal(TMap) && !IsTypedMap(v) && !IsRecordType(v) && !IsOptionsType(v):
					ctorArgs[i], err = core.AutoEvalConsumedMap(r, v, false)
				case v.Parent.Equal(TList):
					ctorArgs[i], err = core.AutoEvalConsumedList(r, v)
				}
				if err != nil {
					return nil, err
				}
			}
			// The tail constructor is the intended consumer — restore
			// the spec before its handler runs (restore-once: the
			// deferred call above becomes a no-op).
			restore()
		}
		vals, err := core.DispatchSig(base, ctorArgs, r)
		if err != nil {
			return nil, err
		}
		// Every blessed constructor signature returns exactly one value.
		return DefHandler([]Value{args[0], vals[0]}, named, stack, r)
	}
}

// synthDefKeywordSig mirrors one constructor signature as a def
// keyword overload: [name/q ctor/q …ctor-args], or the gen chain
// [name/q gen/q params:List tail/q …tail-args]. The constructor's
// per-position dispatch modifiers travel with their slots.
// synthDefKeywordSigNamed synthesizes one def keyword overload, parameterised
// by the NAME
// slot's type. def accepts a word OR a string as the name everywhere else
// (`def "x" 5` binds the same as `def x 5`), so the keyword forms mirror
// both: nameType=TAtom captures a bare-word name via /q, nameType=TString
// takes a string-literal name as a plain value (no quote). Without the
// String variant, `def "double" word […]` (a string-named splice/fn form,
// as host code builds it) fell through to the general value sig and — under
// the strict forward-barrier — stranded on the constructor word.
func synthDefKeywordSigNamed(ctor string, base *Signature, genChain bool, nameType *Type) Signature {
	nameQuote := nameType == TAtom
	offset := 2
	args := []*Type{nameType, TAtom}
	quote := map[int]bool{0: nameQuote, 1: true}
	patterns := map[int]Value{1: NewAtom(ctor)}
	noEval := map[int]bool{}
	if genChain {
		offset = 4
		args = []*Type{nameType, TAtom, TList, TAtom}
		quote[3] = true
		patterns = map[int]Value{1: NewAtom("gen"), 3: NewAtom(ctor)}
		noEval[2] = true // gen's params list is code, not data
	}
	if !nameQuote {
		delete(quote, 0)
	}
	args = append(args, base.Args...)
	// Mirror the base's per-position VALUE patterns, shifted by offset.
	// STRUCTURAL capture slots (/q, raw paren, form, type-literal) are
	// excluded up front (HasStructuralSlots), but a value Pattern is a
	// dispatch CONSTRAINT, not a capture — it must travel with its slot
	// so the keyword form dispatches on the same predicate the bare
	// constructor does. This is load-bearing for fn's two forms: the
	// 3-arg `fn input output body` sig pins slot 0 with `(tnot List)`,
	// so `def g fn [body]` (a List input) fails the 3-arg keyword form
	// and falls through to the 1-list form (`fn [triples]`) instead of
	// over-collecting the following `output`/`body` operands, while
	// `def d fn (2 add 3) [String] ["five"]` (a non-list input) still
	// selects the 3-arg form. QuoteArgs is never mirrored — a /q slot
	// is structural and already gates the sig out above.
	for p, pat := range base.Patterns {
		patterns[p+offset] = pat
	}
	if genChain {
		// DEFER every tail operand's evaluation: a schema map/list may
		// reference the gen placeholders, which are only bound once the
		// form handler has run gen (DefFormVia re-applies the tail's
		// own evaluation policy afterwards).
		for p := range base.Args {
			noEval[p+offset] = true
		}
	}
	for p, on := range base.NoEvalArgs {
		if on {
			noEval[p+offset] = true
		}
	}
	sig := Signature{
		Args:       args,
		QuoteArgs:  quote,
		Patterns:   patterns,
		Impl:       Go(DefFormVia(base, offset, genChain), RunInCheck()),
		Returns:    []*Type{},
		BarrierPos: -1,
		// A constructor that always raises (fn's 0-argument refusal,
		// NUR091) raises the same way through its keyword form: the one
		// compile fact the base declares that the form inherits.
		CompileEffect: base.CompileEffect & CompileDiverges,
	}
	if len(noEval) > 0 {
		sig.NoEvalArgs = noEval
	}
	sig.NoEvalMapArgs = ShiftPosFlags(base.NoEvalMapArgs, offset)
	if genChain {
		// Deferral covers map operands too (NoEvalArgs does not gate
		// map auto-evaluation; NoEvalMapArgs does).
		if sig.NoEvalMapArgs == nil {
			sig.NoEvalMapArgs = make(map[int]bool, len(base.Args))
		}
		for p := range base.Args {
			sig.NoEvalMapArgs[p+offset] = true
		}
	}
	sig.RawParens = ShiftPosFlags(base.RawParens, offset)
	sig.FormArgs = ShiftPosFlags(base.FormArgs, offset)
	sig.TypeArgs = ShiftPosFlags(base.TypeArgs, offset)
	return sig
}

// HasStructuralSlots reports whether a signature captures any position
// structurally — /q word capture, raw paren, macro form, or type-
// literal slot. Such signatures are excluded from the def keyword
// mirror (see RegisterDefKeywordForms).
func HasStructuralSlots(sig *Signature) bool {
	if len(sig.QuoteArgs) > 0 || len(sig.RawParens) > 0 ||
		len(sig.FormArgs) > 0 || len(sig.TypeArgs) > 0 {
		return true
	}
	return false
}

// ShiftPosFlags returns src's per-position flags shifted by `by`
// positions (nil/empty in → nil out).
func ShiftPosFlags(src map[int]bool, by int) map[int]bool {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[int]bool, len(src))
	for p, on := range src {
		dst[p+by] = on
	}
	return dst
}

// RegisterDefKeywordForms synthesizes def's keyword overloads from the
// blessed constructors' live signature tables. Called at the end of
// Register, after every constructor slice has registered.
func RegisterDefKeywordForms(r *Registry) {
	synth := func(ctor string, genChain bool) {
		fnDef := r.Lookup(ctor)
		if fnDef == nil {
			return
		}
		for i := range fnDef.Signatures {
			base := fnDef.Signatures[i]
			if base.Fallback {
				continue
			}
			// A base sig carrying STRUCTURAL capture slots (/q, raw
			// paren, form, type-literal) cannot be mirrored: the
			// capture gate (capturesForwardToken) is per-word across
			// ALL of def's signatures, so an unpatterned structural
			// slot at a shifted tail position would capture ANY word
			// there for EVERY def statement — disabling the function-
			// word barrier two-plus positions out (`def x (…)
			// macroexpand (…)` must keep macroexpand as a barrier).
			// Among the blessed constructors this only excludes
			// quote's word-capture sig; `def a quote foo` spells as
			// `def a foo/q`.
			if HasStructuralSlots(&base) {
				continue
			}
			r.Register("def", synthDefKeywordSigNamed(ctor, &base, genChain, TAtom))
			r.Register("def", synthDefKeywordSigNamed(ctor, &base, genChain, TString))
		}
	}
	for _, ctor := range defKeywordConstructors {
		synth(ctor, false)
	}
	for _, tail := range defGenChainTails {
		synth(tail, true)
	}
}

func DefHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := DefName(args[0])
	stackOnly := DefStackOnly(args[0])
	body := args[1]
	if IsCapitalisedName(name) {
		// `def` is the universal binder (design/legacy/TYPE-UNIFORM.10.ignore
		// Phase 2): a capitalised name is a TYPE binding. Delegate to
		// the kernel type installer — the same path the `type` word
		// uses — so object/predicate lattice-minting and all
		// type-installation validation happen in exactly one place.
		// The rebind notification is InstallType's own now
		// (core/go/rebind_notify.go) — this arm returning the installer's
		// result directly is exactly how a type rebind reached no
		// notification at all before the funnel existed.
		return nil, core.InstallType(r, name, body)
	}
	if err := ValidateWordName(name); err != nil {
		return nil, fmt.Errorf("def %s: %w", name, err)
	}
	if handled, err := DefWordExtension(r, name, body, args[0].Pos()); handled {
		return nil, err
	}
	if r.IsBuiltinWord(name) {
		return nil, reservedWordError(r, "def", name)
	}
	if r.Defs.IsType(name) {
		return nil, r.BoruError("def_error", fmt.Sprintf("def %s: name clash — already a type", name), "def")
	}
	return InstallAndRecordDef(r, name, body, args[0].Pos(), stackOnly)
}

// DefWordExtension routes `def <word> fn […]` on a word that carries
// LOCKED signatures — a built-in, or a module-wrapper rebinding that
// inherited locked inner sigs — to the open-words merge
// (design/OPEN-WORDS.0.md): the fn's signatures merge into a word
// clone bound through the ordinary def shadow stack, scoped like every
// other def. Returns handled=false when the target is not extendable
// this way: plain user fns keep today's whole-replacement shadowing
// (the REPL/iterate idiom — §4.1), and a non-fn body on a built-in
// keeps the reserved_word compile failure. Sealed words (`def`, `make`,
// `word`) decline inside InstallWordExtension.
func DefWordExtension(r *Registry, name string, body Value, pos SrcPos) (bool, error) {
	if !body.Parent.Equal(TFunction) {
		return false, nil
	}
	// A FailedDispatch fn value is here because a CALL matched no signature —
	// a genuine dispatch failure (a concrete type mismatch, e.g.
	// `def y (Net.recv-until nl nl)` feeding Bytes to the Socket slot). At
	// RUNTIME that raises at the call now (design/legacy/FN-VALUE-DISPATCH.0.ignore), so
	// this arm is CHECK MODE, where analysis continues past the finding and
	// `def` still sees the wreckage as a plain value binding. It is never a
	// deliberate `def <word> fn […]` extension, yet it carries the dispatched
	// native's LOCKED signatures; without this guard a re-analysis (a def
	// inside a `for` loop) finds the name already bound to it and misfires the
	// open-words merge as a spurious `locked_signature` compile failure instead of the
	// real dispatch diagnostic. Fall through to the ordinary value binding.
	if body.FailedDispatch {
		return false, nil
	}
	fnDef, ok := body.Data.(FnDefInfo)
	if !ok {
		return false, nil
	}
	existing := r.Lookup(name)
	if existing == nil || !core.HasLockedSigs(existing) {
		return false, nil
	}
	// Mirror InstallAndRecordDef's use-snapshot: the merge's
	// construction-time body pass resolves the word's own name and
	// would otherwise record a spurious self-use.
	checking := r.Check.IsActive()
	prevUsed := checking && r.Check.DefsUsed != nil && r.Check.DefsUsed[name]
	if err := core.InstallWordExtension(r, name, fnDef); err != nil {
		return true, err
	}
	r.Check.RecordDef(name, pos)
	if checking {
		if r.Check.DefsUsed == nil {
			r.Check.DefsUsed = map[string]bool{}
		}
		if prevUsed {
			r.Check.DefsUsed[name] = true
		} else {
			delete(r.Check.DefsUsed, name)
		}
	}
	return true, nil
}

// reservedWordError is the error raised when def / undef targets a core
// word (a native / kernel / host-registered word, or a reserved literal
// true/false/none). Core words are frozen — extend the language by
// defining a NEW word, not by shadowing a built-in.
func reservedWordError(r *Registry, op, name string) error {
	return r.BoruError("reserved_word",
		fmt.Sprintf("%s %s: '%s' is a built-in word and cannot be redefined", op, name, name), op)
}

// markRefineDefUncompilable declines bytecode compilation of a typed-def whose
// refinement constraint could not be recorded as a typed-bind event — the
// fallback behind RecordTypedBindOrDecline. The COMMON dynamic refinement
// shapes (predicate type / bare-refine newtype / DepScalar subset over a
// param or computed carrier) now compile: RecordTypedBind emits an OpBindTyped
// that runs the interpreter's own validate/reparent at run time (RunTypedBind,
// eng/go/typed_bind.go — the closure of miscompile B's compile failure). This
// mark remains only for the residual shape RecordTypedBind declines: a dynamic
// body whose operand has no resolvable provenance, where compiling would have
// to guess the stack layout. Object / alias / schema typed-defs do not route
// here.
// MarkTypedContainerDefUncompilable declines compilation of a typed-container
// def (`def m:{:T} …` / `def xs:[:T] …`) whose body is NON-concrete (a flex /
// carrier body). Such a body's element validation and tag-minting happen at
// runtime only (Unify over the concrete value); the compiled path has no
// typed-container bind to mirror that, so it would silently drop the tag and
// skip enforcement. Falling back to the interpreter keeps the two surfaces in
// agreement. A concrete body validates statically and compiles faithfully.
func MarkTypedContainerDefUncompilable(r *Registry, name string, body, constraint Value) {
	if IsConcrete(body) || (!IsTypedMap(constraint) && !IsTypedList(constraint)) {
		return
	}
	if es := r.Check.Recorder(); es.Active() {
		es.MarkUncompilable("typed-def `" + name + "`: a {:T}/[:T] constraint over a non-concrete (flex) body validates + tags at runtime only")
	}
}

func markRefineDefUncompilable(r *Registry, name string, body Value) {
	// A STATIC (concrete) refinement value's reparent rides the const pool and
	// compiles faithfully — `def p:Pt 5` (a const) folds to a Pt-tagged const, so
	// `p is Pt` holds; RecordTypedBind declines those to keep the proven path,
	// and they must not decline here either.
	if IsConcrete(body) {
		return
	}
	if es := r.Check.Recorder(); es.Active() {
		es.MarkUncompilable("typed-def `" + name + "`: dynamic refinement reparent/validate is interpreter-only (no compiled store-with-reparent)")
	}
}

// defFnPredicateBind is DefTypedHandler's fn-PREDICATE branch (extracted for
// gocyclo): validate/transform via the predicate, reparent per the
// interpreter's decision, and record the runtime bind — for CONCRETE bodies
// too, since the predicate is a runtime evaluation (the 2026-07-15 flip
// finding: a check-lenient bake bound the raw value where the interpreter
// runs the transform). In a COMPILE pass the check-mode run is ANALYSIS ONLY
// (recording suspended so the predicate body's dispatches are not emitted
// inline ahead of the bind); a declined record declines regardless of
// concreteness — no wrong bake, and no compile either.
func defFnPredicateBind(r *Registry, name, typeName string, constraint, body Value, describeType func() string, pos SrcPos) ([]Value, error) {
	resumePred := func() {}
	if es := r.Check.Recorder(); es.Active() && IsConcrete(body) {
		resumePred = es.Suspend()
	}
	out, matched, err := r.RunPredicate(constraint, body)
	resumePred()
	if err != nil {
		return nil, fmt.Errorf("def %s: predicate type %s: %w", name, describeType(), err)
	}
	if !matched {
		return nil, fmt.Errorf("def %s: value %s does not satisfy predicate type %s",
			name, body.String(), describeType())
	}

	// Rewrap with the predicate's *Type so dispatch keys off
	// the nominal name. The underlying Data is unchanged —
	// accessors (AsInteger, AsString, …) read the payload the
	// same way — but the Parent change lets the LCA walk find
	// behaviors installed via `behave compare/q (fn
	// [[Positive Positive] …])` etc.
	//
	// Only fires when the predicate declares a concrete input
	// type (e.g. `fn [n:Integer …]`). Predicates with `Any`
	// input — the historical `fn [x:Any Any […]]` shape — are
	// pure validation gates: their *Type is parented at
	// TFunction and rewrapping would break rendering and
	// downstream type tests (the value would print as
	// `Type/Function/Bbd({…})` rather than its underlying
	// scalar). The PredicateInputType check below mirrors the
	// InstallType decision so the two paths stay aligned.
	var reparentTo *Type
	if typeName != "" && core.PredicateInputType(constraint) != nil {
		if def := r.LookupTypeName(typeName); def != nil && def.Origin != core.OriginBuiltin {
			out = ReparentValue(out, def)
			reparentTo = def
		}
	}
	// A DYNAMIC body records a typed-bind event (OpBindTyped runs the
	// predicate + reparent over the runtime value); RunPredicate above
	// short-circuited on the carrier in check mode, so the runtime bind is
	// the first real evaluation. reparentTo carries the SAME reparent
	// decision the interpreter just took, so the two engines agree.
	out = RecordTypedBindOrDeclineConcrete(r, func() core.TypedBindSpec {
		predCons := constraint
		return core.TypedBindSpec{
			Kind: core.TypedBindPredicate, Name: name, Describe: describeType(),
			Def: reparentTo, Cons: &predCons,
		}
	}, body, out, pos, func() { MarkFnPredicateBindUncompilable(r, name) })
	return InstallAndRecordDef(r, name, out, pos)
}

// MarkFnPredicateBindUncompilable declines compilation when a fn-predicate
// typed-def's bind record declined: the predicate is a runtime evaluation
// for every body shape, so there is no faithful bake to emit. The compile failure
// keeps a wrong bake out; compiling it is still owed.
func MarkFnPredicateBindUncompilable(r *Registry, name string) {
	if es := r.Check.Recorder(); es.Active() {
		es.MarkUncompilable("typed-def `" + name + "`: fn-predicate bind is runtime-evaluated (no compiled bind at this site)")
	}
}

// RecordTypedBindOrDecline threads a refinement typed-def through the bytecode
// recorder: on success the returned binding carries a fresh provenance ID
// registered against a typed-bind event (OpBindTyped re-runs the SAME
// validate/reparent over the runtime value — RunTypedBind), and the program
// keeps compiling. When the recorder declines — emit inactive, a CONCRETE body
// (the static const-pool path stays untouched), or a body operand with no
// resolvable provenance — the site's compile failure mark runs instead, preserving the
// prior fallback taxonomy (and itself no-oping for the inactive/concrete
// cases). bound is the CHECK-mode value the def is about to install; body is
// the raw operand the runtime bind consumes. mkSpec is a THUNK so the spec
// (its Describe renders the constraint) is only built when a bind is actually
// recorded — a plain interpreter run pays nothing it did not pay before.
func RecordTypedBindOrDecline(r *Registry, mkSpec func() core.TypedBindSpec, body, bound Value, pos SrcPos, decline func()) Value {
	if es := r.Check.Recorder(); es.Active() && !IsConcrete(body) {
		if out, ok := es.RecordTypedBind(mkSpec(), body, bound, pos); ok {
			return out
		}
	}
	decline()
	return bound
}

// RecordTypedBindOrDeclineConcrete is RecordTypedBindOrDecline WITHOUT the
// concrete-body decline: the fn-PREDICATE bind is a runtime evaluation for
// every body shape (the predicate can transform, raise, or read live state),
// so a concrete operand records the bind rather than riding the const pool.
func RecordTypedBindOrDeclineConcrete(r *Registry, mkSpec func() core.TypedBindSpec, body, bound Value, pos SrcPos, decline func()) Value {
	if es := r.Check.Recorder(); es.Active() {
		if out, ok := es.RecordTypedBind(mkSpec(), body, bound, pos); ok {
			return out
		}
	}
	decline()
	return bound
}

// ResolveResourceTypeInfo returns the ResourceTypeInfo a typed-def
// annotation denotes, and true, for a Resource/Entity constraint. A
// word-resolved annotation already carries the ResourceTypeInfo body;
// a bare Resource-family type literal (the shape `def e:Entity {…}`
// produces, since builtin type names parse to literals in data context)
// is resolved from the type binding by its lattice-leaf name.
func ResolveResourceTypeInfo(r *Registry, constraint Value) (ResourceTypeInfo, bool) {
	if IsResourceType(constraint) {
		info, _ := AsResourceType(constraint)
		return info, true
	}
	// A bare Resource-family type literal (what `def e:Entity {…}`
	// produces — builtin type names parse to literals in data context)
	// carries no ResourceTypeInfo; resolve it from the def binding by
	// the rendered type name. The lookup itself is the guard: only the
	// Resource/Entity def bindings resolve to a ResourceType, so this
	// never hijacks a non-Resource annotation.
	if r != nil && IsBareTypeNode(constraint) {
		if info, ok := LookupResourceTypeByName(r, constraint.String()); ok {
			return info, true
		}
	}
	return ResourceTypeInfo{}, false
}

// LookupResourceTypeByName resolves a Resource/Entity ResourceTypeInfo
// from a type name. Resource and Entity are installed as def bindings
// (InstallResourceTypes), so — unlike a user class, which is a type
// binding reachable via TopTypeBody — their schema is found through the
// def store.
func LookupResourceTypeByName(r *Registry, name string) (ResourceTypeInfo, bool) {
	if r == nil || name == "" {
		return ResourceTypeInfo{}, false
	}
	v, ok := r.Defs.Top(ResourceDefKey(name))
	if !ok {
		return ResourceTypeInfo{}, false
	}
	info, err := AsResourceType(v)
	if err != nil {
		return ResourceTypeInfo{}, false
	}
	return info, true
}

// typedDefUnifyMirror emits the failed-unify check diagnostic for a typed
// def (NUR058). An INERT-CONST body is an exactly-known operand — the
// check-time value IS the runtime value, byte for byte — so the finding is
// a GUARANTEED runtime error mirror whose detail text cannot drift. The
// stamp is sound only when the COMPILED program raises identically: the
// check arm installs a carrier and continues, so on a recording pass the
// mirror is completed by a terminal RecordTrap (the macroexpand
// discipline), and a declined trap (nested occurrence) keeps the
// unstamped, compile-declining emission. Deep inertness, not shallow
// concreteness, is the gate: a concrete LIST holding a check-mode
// abstract class instance renders differently at check time than the
// runtime value does (`[Box of [String]]` vs `[Box of [String]{value:…}]`
// — the generics container rows), so such a body keeps the unstamped
// emission and its runtime BIND_TYPED raise, which renders the live
// value. A carrier body likewise stays unstamped — the static compile failure is
// an approximation the runtime value could still satisfy.
func typedDefUnifyMirror(r *Registry, name, detail string, body Value, pos SrcPos) {
	// pos is always set now, and the fallback chain that used to stand here
	// is deleted rather than allowlisted. It read: the name token's Pos can
	// be unset (a synthesized pair), so fall back to the body's position, then
	// to the body's first element's.
	//
	// That was NUR108's second half, diagnosed correctly and rescued in the
	// wrong place. The rescue DID produce a position — the BODY's — which is
	// precisely why the compiled trap blamed the value at 1:23 where the
	// interpreter blames the `def` at 1:1. A local rescue can only choose
	// among the positions it can see, and the right one was never one of them.
	//
	// DefTypedHandler now supplies the `def` token's own position
	// (CheckState.CurWordPos) at the source, so every caller here passes a set
	// pos and the chain is unreachable — measured, all three statements, over
	// the whole corpus and every unit suite.
	if core.IsInertConst(body) && (!r.Check.Compiling ||
		r.Check.Recorder().RecordTrap("type_error", detail, name, "", pos)) {
		CheckAddUniqueDiagnostic(r, "type_error", detail, name, pos)
		return
	}
	r.Check.AddDiagnostic(CheckDiagnostic{
		Code:   "type_error",
		Detail: detail,
		Word:   name,
		Row:    pos.Row,
		Col:    pos.Col,
	})
}

func DefTypedHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	nameMap, _ := AsMap(args[0])
	if nameMap == nil || nameMap.Len() == 0 {
		return nil, r.BoruError("def_error", "def: typed-name map must have exactly one key, got empty/non-concrete map", "def")
	}
	if nameMap.Len() != 1 {
		return nil, fmt.Errorf("def: typed-name map must have exactly one key, got %d", nameMap.Len())
	}
	name := nameMap.Keys()[0]
	if IsCapitalisedName(name) {
		return nil, r.BoruError("def_error", fmt.Sprintf("def %s: def names must not start with a capital letter (capitalised names are reserved for types)", name), "def")
	}
	if err := ValidateWordName(name); err != nil {
		return nil, fmt.Errorf("def %s: %w", name, err)
	}
	if r.IsBuiltinWord(name) {
		return nil, reservedWordError(r, "def", name)
	}
	if r.Defs.IsType(name) {
		return nil, r.BoruError("def_error", fmt.Sprintf("def %s: name clash — already a type", name), "def")
	}
	constraint, _ := nameMap.Get(name)

	// The typed-name map is SYNTHESISED by the `name:Type` desugar and carries
	// no position — measured 0:0 at every site below — so `def`'s own token is
	// the position of this definition. It is the one the interpreter reports:
	// stampErrPos stamps an unpositioned handler error with e.currentPos(),
	// which is exactly what CurWordPos publishes.
	//
	// A FALLBACK, not an override: a map that does carry a position keeps it.
	// This is what closes NUR108's first half — the compiled lane's BIND_TYPED
	// raise had no position to stamp because the RECORD had none, and rendered
	// "source position unknown" where the interpreter underlines the `def`.
	defPos := args[0].Pos()
	if defPos.Row == 0 {
		defPos = r.Check.CurWordPos
	}
	// An angle/type-bound annotation — `def b:Box<Integer> {…}` —
	// arrives as a sugar marker; lower it to its paren form so the
	// ParenExpr arm below evaluates it like the spelt-out
	// `def b:(Box of [Integer])`.
	if sinfo, sok := core.AsSugar(constraint); sok {
		if exp, serr := core.SugarExpansion(r, sinfo, constraint, false); serr == nil && len(exp) == 1 {
			constraint = exp[0]
		}
	}
	// A parenthesised annotation — `def b:(Box of [Integer]) {…}` —
	// evaluates inline (def's NoEvalMapArgs keeps the typed-name map
	// raw, so the ParenExpr arrives unevaluated). Generic
	// instantiations are the main client; any expression producing a
	// single type value works.
	if IsParenExpr(constraint) {
		toks, _ := AsParenExpr(constraint)
		body := make([]Value, len(toks))
		copy(body, toks)
		sub := New(r)
		out, err := sub.Run(body)
		if err != nil {
			return nil, fmt.Errorf("def %s: type annotation: %w", name, err)
		}
		if len(out) != 1 {
			return nil, fmt.Errorf("def %s: type annotation must produce one type, got %d values", name, len(out))
		}
		constraint = out[0]
	}
	// A typed-list/map annotation whose CHILD is a paren expression —
	// `def xs:[:(Pair of [String Integer])] […]` — needs the child
	// evaluated the same way a top-level paren annotation is (the
	// parser leaves it as a raw ParenExpr payload).
	if evaluated, cerr := core.ResolveChildTypeExpr(r, constraint); cerr != nil {
		return nil, fmt.Errorf("def %s: type annotation: %w", name, cerr)
	} else {
		constraint = evaluated
	}
	var typeName string
	constraint, typeName, _ = r.ResolveTypedNameValue(constraint)
	if !IsTypeBody(constraint) {
		err := r.BoruError("def_error",
			fmt.Sprintf("def %s: type annotation must be a type value, got %s", name, constraint.String()), "def")
		// An unresolved WORD annotation is almost always a misspelt type
		// name — suggest the near-miss (diagnostics phase 4).
		if ae, ok := err.(*core.BoruError); ok && core.IsWord(constraint) {
			if w, werr := core.AsWord(constraint); werr == nil {
				if s := core.DidYouMean(w.Name, r.SuggestionCandidates()); s != "" {
					ae.Suggestions = append(ae.Suggestions, core.DiagSuggestion{Message: s})
				}
			}
		}
		return nil, err
	}
	describeType := func() string {
		if typeName != "" {
			return typeName
		}
		return constraint.String()
	}
	body := args[1]
	var depScalarCons Value
	constraint, depScalarCons = resolveTypedDefConstraint(constraint)
	// A generic SCHEMA annotation — `def b:Box {value:42}` — infers
	// its type arguments from the body and instantiates (Phase 7 /
	// D12); the instantiation then flows through the ordinary typed-def
	// branches below (the ObjectType branch constructs class
	// instances, etc.). Uninferable, undefaulted parameters error.
	if IsTypeSchema(constraint) {
		inst, ierr := core.InferAndInstantiateSchema(r, constraint, body)
		if ierr != nil {
			return nil, fmt.Errorf("def %s: %w", name, ierr)
		}
		constraint = inst
	}
	if constraint.Parent.Equal(TFnUndef) && IsAtom(body) {
		atomName, _ := AsAtom(body)
		if top, ok := r.Defs.Top(atomName); ok {
			if top.Parent.Equal(TFunction) {
				body = top
			}
		}
	}
	// A PREDICATE constraint: an inline fn value (Parent Function), or —
	// after the Stage 2 flip — the NAME of a predicate type, which
	// evaluates to its minted node carrying a PredicateUnifier. The
	// predicate BODY to run is the node's recorded content
	// (design/legacy/TYPE-REPRESENTATION.1.ignore §N2); defFnPredicateBind keeps
	// its historical run-then-reparent semantics (typeof x → Pos for an
	// input-typed predicate) in both spellings.
	if constraint.Parent.Equal(TFunction) || core.IsPredicateTypeNode(constraint) {
		pred := constraint
		if pb, ok := core.TypeContentOf(constraint); ok {
			pred = pb
		}
		if pred.Data != nil {
			return defFnPredicateBind(r, name, typeName, pred, body, describeType, defPos)
		}
	}

	// ObjectType constraint (`def x:Person {map}` where Person is
	// `type Person object {…}`): build a Person-typed ObjectInstance
	// from the body map via make-style construction. This closes the
	// "structural for validation, nominal for dispatch" gap for
	// object types — without this branch the value would have
	// Parent=TMap and Person's registered behaviors would never
	// dispatch. The result carries Parent=Person, satisfies the
	// `behave compare/q (fn [[Person Person] …])` dispatch path, and
	// supports `get`/`set` via the ObjectInstance signatures.
	//
	// Accepts both a raw Map (built via make) and an already-typed
	// ObjectInstance (passed through). Other body shapes fall
	// through to Unify and either succeed or surface a type error.
	if IsClassType(constraint) {
		info, _ := AsClassType(constraint)
		if body.Parent.Equal(TMap) {
			// `def b:Type {map}` is `def b (make Type map)`. In emit mode record
			// the make event the direct MakeObject call would skip, so the bound
			// instance has the same provenance an explicit make gives it (a
			// downstream `b typeof` then compiles). Outside emit mode this is a
			// no-op and the concrete instance is bound.
			if carrier, ok := core.RecordTypedDefMake(r, constraint, body, defPos); ok {
				return InstallAndRecordDef(r, name, carrier, defPos)
			}
			result, err := core.MakeObject(info, body, r)
			if err != nil {
				return nil, fmt.Errorf("def %s: %w", name, err)
			}
			return InstallAndRecordDef(r, name, result[0], defPos)
		}
		if IsClassInstance(body) {
			oi, _ := AsClassInstance(body)
			// Accept if the instance's nominal type matches the
			// declared one (covers `def x:Person make Person {…}`).
			if oi.TypeRef != nil && oi.TypeRef.ID == info.ID {
				return InstallAndRecordDef(r, name, body, defPos)
			}
		}
	}
	// Resource/Entity annotation: `def e:Entity {map}` is `def e (make
	// Entity map)` — the same construction path a class annotation takes,
	// routed through MakeResource (Resource/Entity have their own flat
	// instance struct). make's `[Ideal Map]` sig serves both kinds, so
	// the emit-mode RecordTypedDefMake carrier is recorded identically.
	// Unlike user class names (which parse as Words and resolve to their
	// ClassTypeInfo), the builtin Resource/Entity names parse to BARE
	// type literals in data context, so ResolveResourceTypeInfo also
	// looks the schema up by name when the constraint carries no body.
	if resInfo, isRes := ResolveResourceTypeInfo(r, constraint); isRes {
		if body.Parent.Equal(TMap) {
			if carrier, ok := core.RecordTypedDefMake(r, constraint, body, defPos); ok {
				return InstallAndRecordDef(r, name, carrier, defPos)
			}
			provided, merr := AsMutableMap(body)
			if merr != nil {
				return nil, fmt.Errorf("def %s: %w", name, merr)
			}
			result, err := MakeResource(resInfo, provided, r)
			if err != nil {
				return nil, fmt.Errorf("def %s: %w", name, err)
			}
			return InstallAndRecordDef(r, name, result[0], defPos)
		}
		if IsResourceInstance(body) {
			ri, _ := AsResourceInstance(body)
			// Accept a pre-made instance whose nominal type matches.
			if ri.TypeRef != nil && ri.TypeRef.ID == resInfo.ID {
				return InstallAndRecordDef(r, name, body, defPos)
			}
		}
	}
	if r.Check.IsActive() && depScalarCons.IsDepScalar() && !IsConcrete(body) {
		if body.Parent.ConformsTo(depScalarCons.Parent) {
			// An ABSTRACT (carrier) body admits on base conformance only —
			// the predicate is value-level and the value is unknown here, so
			// validation stays at RUNTIME via v.Is. A compiled `def
			// x:(Integer gt 10) (f 1)` would bind whatever f returns where
			// the interpreter may raise, and the compiler can't run the
			// inline predicate (no canonical node carrying the DepScalar
			// Behavior) — decline abstract DepScalar typed-defs → fall back.
			//
			// A CONCRETE body deliberately falls THROUGH to the Unify below:
			// unifyDepScalar runs the self-contained predicate on the real
			// value, so `def x:(Integer gt 10) 5` is flagged at check time
			// with the byte-identical runtime message (and a passing literal
			// binds the same value in both engines, so it stays compilable).
			//
			// The dynamic case records a typed-bind event: OpBindTyped runs the
			// SAME Unify(value, constraint) — the self-contained DepScalar
			// predicate, no registry — over the runtime value, raising the
			// byte-identical unify error on failure and binding Unify's result
			// (base tag kept, no reparent) on success.
			bound := RecordTypedBindOrDecline(r, func() core.TypedBindSpec {
				depCons := depScalarCons
				return core.TypedBindSpec{
					Kind: core.TypedBindDepScalar, Name: name, Describe: describeType(), Cons: &depCons,
				}
			}, body, body, defPos, func() {
				if es := r.Check.Recorder(); es.Active() {
					es.MarkUncompilable("typed-def `" + name + "`: DepScalar predicate validation is interpreter-only")
				}
			})
			return InstallAndRecordDef(r, name, bound, defPos)
		}
	}
	// User-minted bare-refine subtype (`def Foo refine Integer`): the
	// constraint is the Foo type literal whose lattice Parent is the
	// type Foo refines. Check the body satisfies the parent type
	// (since values of the parent type are the inhabitants Foo can
	// accept), then reparent a COPY of the body to Foo. Mutating the
	// Unify result would store its by-value type literal (Unify swaps
	// when one side is bare and subtype-ordered) instead of the
	// body's payload — `def x:Foo 1` would silently bind x to the
	// Foo-tagged type literal, not the integer 1.
	if IsBareTypeNode(constraint) && constraint.Origin == core.OriginUserDef &&
		typeName != "" && constraint.Parent != nil {
		// Behavior-keyed guard: a bare user node whose kind enforces
		// membership through a kernel constraint Unifier (dependent
		// scalar, predicate, binding-body — core.HasConstraintUnify)
		// must NOT take this nominal reparent arm: the arm unifies
		// against the BUILTIN ancestor only, so it would bind without
		// ever running the constraint (`def x:Big 5` succeeding once
		// evaluation yields nodes — design/legacy/TYPE-REPRESENTATION.1.ignore
		// §N3). Such constraints fall through to the general UnifyR
		// below, where dispatchUnifier finds the kind's Unify. A user
		// `behave unify/q` wrapper on a nominal refine is NOT a
		// constraint (Unifier but not ContentMembership), so newtypes
		// keep this arm.
		if def := r.LookupTypeName(typeName); def != nil && def.Origin == core.OriginUserDef &&
			!core.HasConstraintUnify(def) {
			// Walk up the lattice past any intervening user refines
			// (e.g. `Foo refine Item refine String`) to the nearest
			// builtin ancestor and unify against THAT. A sibling-of-
			// constraint kernel subtype (ProperString satisfying a
			// Foo whose parent Item branches off String) wouldn't
			// match the immediate parent literal but does match the
			// shared kernel base.
			root := def.Parent
			for root != nil && root.Origin == core.OriginUserDef {
				root = root.Parent
			}
			if root == nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
				return nil, fmt.Errorf("def %s: refine subtype %s has no builtin ancestor",
					name, describeType())
			}
			parentLit := NewTypeLiteral(root)
			if _, ok := Unify(body, parentLit); ok {
				// A DYNAMIC body records a typed-bind event: OpBindTyped re-runs
				// this exact Unify-against-builtin-ancestor + reparent over the
				// runtime value, so compiled typeof/sig-dispatch see the newtype.
				bound := RecordTypedBindOrDecline(r, func() core.TypedBindSpec {
					return core.TypedBindSpec{
						Kind: core.TypedBindRefine, Name: name, Describe: describeType(), Def: def,
					}
				}, body, ReparentValue(body, def), defPos,
					func() { markRefineDefUncompilable(r, name, body) })
				return InstallAndRecordDef(r, name, bound, defPos)
			}
			if r.Check.IsActive() {
				// A CONCRETE body is an exactly-known operand, so the failed
				// unify is a GUARANTEED runtime error mirror — the non-check
				// branch below raises the identical text — and rides the
				// stamping helper (RuntimeMirror + dedupe, NUR058). A carrier
				// body keeps the unstamped emission: the static compile failure is an
				// approximation the runtime value could still satisfy, so the
				// compile pipeline must keep declining on it.
				// Mirror discipline lives in typedDefUnifyMirror (NUR058).
				typedDefUnifyMirror(r, name,
					fmt.Sprintf("def %s: value %s does not unify with declared type %s",
						name, body.String(), describeType()), body, defPos)
				return InstallAndRecordDef(r, name, NewCarrier(def), defPos)
			}
			return nil, r.BoruError("type_error",
				fmt.Sprintf("def %s: value %s does not unify with declared type %s",
					name, body.String(), describeType()), name)
		}
	}
	// Registry-armed: a container constraint may carry a bare
	// type-name child (`def xs:[:Foo]`) that only the registry can
	// resolve (NUR060) — the registry-free Unify degraded it to an
	// Atom and declined every value.
	unified, ok := UnifyR(body, constraint, r)
	if !ok {
		if r.Check.IsActive() {
			// Same mirror discipline as the refine-ancestor arm above
			// (NUR058): exactly-known operands stamp RuntimeMirror; a
			// carrier body keeps the unstamped, compile-declining emission.
			typedDefUnifyMirror(r, name,
				fmt.Sprintf("def %s: value %s does not unify with declared type %s",
					name, body.String(), describeType()), body, defPos)
			return InstallAndRecordDef(r, name, NewCarrier(constraint.Parent), defPos)
		}
		return nil, r.BoruError("type_error",
			fmt.Sprintf("def %s: value %s does not unify with declared type %s",
				name, body.String(), describeType()), name)
	}
	// A typed-container constraint ({:T}/[:T]) over a NON-CONCRETE body — a flex
	// carrier whose concrete elements are unknown at check time — validates its
	// elements and mints the element tag only at RUNTIME (Unify over the concrete
	// flex body). There is no compiled typed-container bind that re-runs that
	// element check + preserves flex-ness, so the compiled path would drop the
	// tag and skip enforcement. Decline compilation → the def falls back to the
	// interpreter, which enforces. A CONCRETE body is validated statically above
	// and compiles faithfully (the plain {:T} map case is unaffected).
	MarkTypedContainerDefUncompilable(r, name, body, constraint)
	// FnUndef constraint (`def f:Mapper fn […]`): after Unify
	// confirms the function shape matches Mapper, rewrap the
	// Parent so dispatch keys off Mapper rather than the generic
	// the generic TFunction. Behaviors installed via
	// `behave compare/q (fn [[Mapper Mapper] …])` then dispatch on
	// f. Same rewrap pattern as predicate types — the payload
	// shape (FnDefInfo) is unchanged, accessors keep working, just
	// the dispatch identity flips.
	if constraint.Parent.Equal(TFnUndef) && typeName != "" {
		if def := r.LookupTypeName(typeName); def != nil && def.Origin != core.OriginBuiltin {
			unified = ReparentValue(unified, def)
		}
	}
	return InstallAndRecordDef(r, name, unified, defPos)
}

// ---- undef ----

// varUndefHandler is __varundef's own thin front over undefHandler: it
// notes the teardown on the recorder (RecordDynUndef — the event seat
// the twin regime's arm-residency bridge pairs with a var param's
// BindUndef twin; a no-op everywhere the recorder has no arm-resident
// bracket open) and then unbinds byte-identically to `undef name`.
func varUndefHandler(args []Value, named map[string]Value, future []Value, r *Registry) ([]Value, error) {
	if name := DefName(args[0]); name != "" {
		r.Check.Recorder().RecordDynUndef(name, args[0].Pos())
	}
	return undefHandler(args, named, future, r)
}

func undefHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := DefName(args[0])
	// A speculative check region (a rolled-back nested body, a fn-body
	// analysis) must not commit the deletion of an ENCLOSING binding: the
	// region may never run, so popping here leaked `undefined_word` onto
	// clean programs (`def x 1 do [7] error [undef x 9] x` — the
	// wrapped-undef FP class; core.Registry.SpecUndefBlocked). The binding
	// stays in the model — lenient in the one direction check mode is
	// allowed to be. In-region bindings still pop (teardown untouched);
	// top-level and `do`-body undefs still commit (leak fidelity).
	if r.SpecUndefBlocked(name) {
		// The model keeps the binding, but from here its VALUE is unknown:
		// GeneraliseSpecUndef puts a carrier in the binding's place, so no
		// later read folds or bakes the value the region may have popped,
		// and the recorder places the pop at this site and reads the name
		// live (the sixty-eighth increment). A binding the model declines
		// to generalise — a type, a fn-family value, a frame binding of an
		// enclosing fn — declines instead, and the interpreter owns the
		// shape. Only for a binding that EXISTS before the region: for a
		// never-bound name the gate merely silences the speculative
		// diagnostic, and there is nothing to pop on either lane (review of
		// #463). The handler passes the fact — this registry's — rather
		// than the recorder re-deriving it from a registry a module call
		// may have left bound.
		if r.Defs.Depth(name) > 0 {
			if core.GeneraliseSpecUndef(r, name) {
				r.Check.Recorder().RecordSpeculativeUndef(name, args[0].Pos())
			} else {
				r.Check.Recorder().DeclineSpeculativeUndef(name)
			}
		}
		return nil, nil
	}
	if r.IsBuiltinWord(name) {
		// A word-extension clone on top of a built-in name is an
		// ordinary def — `undef` pops it, restoring the pre-merge
		// state (design/OPEN-WORDS.0.md §2.1). The base native binding
		// itself stays frozen.
		if entry, ok := r.Defs.TopEntry(name); ok {
			if _, isExt := core.IsWordExtension(entry.Body); isExt {
				UninstallDef(r, name)
				return nil, nil
			}
		}
		return nil, reservedWordError(r, "undef", name)
	}
	if IsCapitalisedName(name) {
		// `undef` is the universal unbinder (the symmetric completion
		// of Phase 2's universal `def` — design/legacy/TYPE-UNIFORM.10.ignore):
		// a capitalised name is a TYPE binding, so pop it from the single
		// binding store and retire the minted lattice type.
		// The pop, the mint retirement, the ledger note and the rebind
		// notification are all core.UninstallType's — it is a BINDING
		// OPERATION, and it was inline here until 2026-09-04, which is how it
		// came to be the one unbinder that notified nothing.
		if !core.UninstallType(r, name) {
			return nil, r.BoruError("undef_error",
				fmt.Sprintf("undef %s: no such type binding", name), "undef")
		}
		return nil, nil
	}
	// An undef of a LOOP-CARRIED def exposes the previous binding while the
	// carried frame slot still holds the rebound value — compiled reads would
	// diverge; decline and let the interpreter own the shape.
	r.Check.Recorder().DeclineCarriedUndef(name)
	// The fn-carrier side table is a SECOND binding store for this name
	// (installDef declines a computed fn, so the name lives only there).
	// Drop it in step with the Defs pop, or the table outlives the binding
	// and a later read resolves a stale carrier — see
	// core.DropCheckFnCarrierBind for the two shapes that diverged.
	DropCheckFnCarrierBind(r, name)
	// An undef of a fn a conditional body defined in THIS region: the
	// placed install has a placed pop (RecordSpecFnUndef — review of #466:
	// the binding leaked past the arm into the next request).
	r.Check.Recorder().RecordSpecFnUndef(name, args[0].Pos())
	UninstallDef(r, name)
	return nil, nil
}

func UndefFnHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := DefName(args[0])
	undefInfo, ok := args[1].Data.(FnUndefInfo)
	if !ok {
		return nil, fmt.Errorf("undef: expected fn undef spec, got %s", args[1].String())
	}
	UninstallFnSigs(r, name, undefInfo)
	return nil, nil
}

// ---- var ----

func VarHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list := args[0]
	if !list.Parent.Equal(TList) {
		return nil, r.BoruError("var_error", "var: argument must be a list", "var")
	}
	if !IsConcrete(list) {
		return nil, r.BoruError("var_error", "var: argument must be a concrete list, got type literal", "var")
	}
	elems, _ := AsList(list)
	if elems.Len() == 0 {
		return nil, r.BoruError("var_error", "var: empty list", "var")
	}

	declVal := elems.Get(0)
	if !declVal.Parent.Equal(TList) || !IsConcrete(declVal) {
		return nil, r.BoruError("var_error", "var: first element must be a list of variable declarations", "var")
	}
	decls, _ := AsList(declVal)
	body := elems.Slice()[1:]

	var result []Value
	var varNames []string
	// varSites holds each declaration NAME token, the site the synthesized
	// def and __varundef tokens carry (WithPos): a token without a position
	// gives its bind transition no site — the compile pass keys the twins a
	// root do body leaves for adoption on the body's token sites, and a
	// site-less undef twin stayed unplaced, declining every root
	// `do [var [[[k 1]] …]]` (code-bodies.tsv L197, measured 2026-09-24).
	var varSites []Value

	for _, decl := range decls.Slice() {
		switch {
		case IsWord(decl):
			_as0, _ := AsWord(decl)
			name := _as0.Name
			varNames = append(varNames, name)
			varSites = append(varSites, decl)
			result = append(result, WithPos(NewWord("def"), decl), WithPos(NewWord(name), decl), NewEnd())

		case decl.Parent.Equal(TList) && decl.Data != nil:
			declElems, _ := AsList(decl)
			if declElems.Len() < 2 {
				return nil, r.BoruError("var_error", "var: declaration list must have name and value", "var")
			}
			var name string
			if IsWord(declElems.Get(0)) {
				_as1, _ := AsWord(declElems.Get(0))
				name = _as1.Name
			} else if declElems.Get(0).Parent.ConformsTo(TString) {
				name, _ = AsString(declElems.Get(0))
			} else {
				return nil, r.BoruError("var_error", "var: declaration name must be a word or string", "var")
			}
			varNames = append(varNames, name)
			varSites = append(varSites, declElems.Get(0))
			result = append(result, WithPos(NewWord("def"), declElems.Get(0)), WithPos(NewWord(name), declElems.Get(0)))
			result = append(result, declElems.Slice()[1:]...)
			result = append(result, NewEnd())

		case decl.Parent.ConformsTo(TString):
			name, _ := AsString(decl)
			varNames = append(varNames, name)
			varSites = append(varSites, decl)
			result = append(result, WithPos(NewWord("def"), decl), WithPos(NewWord(name), decl), NewEnd())

		default:
			return nil, fmt.Errorf("var: invalid declaration: %s", decl.String())
		}
	}

	result = append(result, body...)

	// Cleanup via __varundef (not `undef`): the body residual is still on the
	// stack here, and `undef`'s 2-arg fn-overload form would mis-match it in
	// check mode (a dynamic-Any residual gradually satisfies TFnUndef). The
	// dedicated 1-arg word dispatches identically in check and at runtime, which
	// is what lets a var-body compile to a closure unit.
	for i := len(varNames) - 1; i >= 0; i-- {
		result = append(result, WithPos(NewWord("__varundef"), varSites[i]), WithPos(NewWord(varNames[i]), varSites[i]))
	}

	return result, nil
}

// ---- fn ----

// FnHandler always produces a Function value. The list must be a
// non-zero multiple of 3 (input/output/body triples). For the
// type-only / shape form (input/output pairs, no body) use the
// separate `fnsig` word — registered via eng.RegisterCoreFnSig
// from register.go.
func FnHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// A pending gen spec (`def identity gen [T] fn [[x:T] [T] [x]]`)
	// makes this a GENERIC fn: the placeholders stay bound while
	// ParseFnParams resolves the sigs (so `x:T` types against the
	// placeholder node, whose Behavior handles dispatch admission),
	// then pop; the spec rides FnDefInfo.Gen so each call installs
	// the inferred body-scoped type bindings.
	genSpec := r.TakePendingGen()
	list := args[0]
	if !list.Parent.Equal(TList) {
		return failGenErr(r, genSpec, r.BoruError("fn_error", "fn: argument must be a list", "fn"))
	}
	if !IsConcrete(list) {
		return failGenErr(r, genSpec, r.BoruError("fn_error", "fn: argument must be a concrete list, got type literal", "fn"))
	}
	_lst, _ := AsList(list)
	elems := _lst.Slice()
	if len(elems) == 0 || len(elems)%3 != 0 {
		return failGenErr(r, genSpec, r.BoruError("fn_error", "fn: list length must be a non-zero multiple of 3 (input output body triples); use `fnsig` for the type-only form, or the 3-arg form `fn input output body` for a single triple with a non-list input", "fn"))
	}
	return FnConstruct(r, elems, genSpec)
}

// FnNoArgsHandler — the loud refusal of a `fn` that took nothing (NUR091):
// neither the spec-list form nor the triple matched what was written after
// it, and a declaration both forms reject is reported at the declaration,
// whatever sat in its slots. A bare `List` input is the shape that lands
// here — the `(tnot List)` rule means a single List-typed param must be
// written in the spec-list form. A pending gen spec is consumed like every
// other fn failure.
func FnNoArgsHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	genSpec := r.TakePendingGen()
	return failGenErr(r, genSpec, r.BoruErrorHint("signature_error",
		"fn: expected a spec list or an input/output/body triple after it — a bare List input is rejected by (tnot List); a single List-typed param needs the spec-list form", "fn",
		"hint: write the triple as a list: fn [[xs:List] [Output] [body]]"))
}

// FnTripleHandler — the 3-arg single-triple form `fn input output body`.
// The three args are one signature triple without the wrapping list:
// `fn x:Integer [Integer] [x mul 2]` ≡ `fn [[x:Integer] [Integer] [x mul 2]]`.
// The input must NOT be a list (the sig's `tnot List` Pattern enforces
// this at dispatch): a list input always selects the 1-arg spec-list
// form, so the two forms stay disjoint. Non-list input/output/body
// follow ParseFnDef's abbreviation rule (auto-wrap into a one-element
// list), the same convention afn applies — so a bare type, an implicit
// pair (x:Integer), an explicit map, or a literal pattern all work as
// the single param. Multi-param triples need the spec-list form.
func FnTripleHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	genSpec := r.TakePendingGen()
	// Defensive twin of the sig's `tnot List` Pattern: dispatch never
	// routes a list input here, but check-mode fallbacks and direct
	// handler calls can. A list input means the spec-list form.
	if args[0].Parent.Equal(TList) {
		return failGenErr(r, genSpec, r.BoruError("fn_error", "fn: the 3-arg triple form takes a non-list input; wrap the whole triple as a list instead: fn [[input] [output] [body]]", "fn"))
	}
	// Unlike the spec-list form — whose operands are always literals
	// inside the NoEvalArgs list — a triple-form operand can be COMPUTED
	// (`fn (2 add 3) [String] ['five']`). In check mode such an operand
	// arrives as a carrier, so the Function constructed here carries the
	// carrier where the runtime value belongs (an Integer carrier as the
	// input pattern instead of 5); a compiled unit would intern that
	// check-time Function as a constant and diverge from the interpreter,
	// which re-constructs with the live value. Analysis proceeds with the
	// carrier; the program itself is interpreter-only.
	for i := range args[:3] {
		if args[i].Carrier {
			if es := r.Check.Recorder(); es.Active() {
				es.MarkUncompilable("fn: triple-form construction over a computed (carrier) operand is interpreter-only — the compiled unit would bake the check-time placeholder")
			}
			break
		}
	}
	return FnConstruct(r, args[:3], genSpec)
}

// failGenErr unwinds a pending gen spec's placeholder bindings before
// surfacing err — the shared error tail of every fn-construction path
// (`gen [T] fn …` must not leak T on failure).
func failGenErr(r *Registry, genSpec *GenSpecInfo, err error) ([]Value, error) {
	if genSpec != nil {
		PopGenBindings(r, genSpec)
	}
	return nil, err
}

// FnConstruct is the shared construction core behind both fn forms:
// elems is the flat triple stream ([input output body …]) already
// validated to a non-zero multiple of 3. Parses the FnDefInfo, attaches
// a pending gen spec, computes lexical captures, and runs the
// check-mode analyses (declaration-time generic body check, dead
// overload detection).
func FnConstruct(r *Registry, elems []Value, genSpec *GenSpecInfo) ([]Value, error) {
	failGen := func(err error) ([]Value, error) {
		return failGenErr(r, genSpec, err)
	}
	fnDef, err := parseFnDef(r, elems)
	if err != nil {
		return failGen(err)
	}
	// The fn's HOME is the registry that minted it: its free words resolve
	// there wherever the value later travels (design/FUNCTION-VALUE-SCOPE.0.md
	// rule 1). Stamped at construction, not at module-export resolution, so
	// a main-file fn handed INTO a module keeps main's bindings exactly as a
	// module fn handed out keeps the module's. A nil Registry is left only to
	// bodiless Go-native values, which have no free words to resolve.
	fnDef.Registry = r
	if genSpec != nil {
		PopGenBindings(r, genSpec)
		fnDef.Gen = genSpec
	}
	// Compute lexical captures: per-sig walks merged into one list.
	// Nil at top-level (no enclosing fn) — natural no-op via
	// ComputeCaptures' baseline check.
	perSig := make([][]CapturedBinding, len(fnDef.Signatures))
	for i := range fnDef.Signatures {
		perSig[i] = core.ComputeCaptures(r, &fnDef.Signatures[i])
	}
	fnDef.Captured = core.MergeCaptures(perSig)

	// Check mode, generic fns: declaration-time ABSTRACT check
	// (Phase 5). Analyse each body once with carrier args of the
	// declared param types — for a `x:T` param that is a carrier of
	// the PLACEHOLDER node, so operations on it are admitted exactly
	// when the parameter's bound justifies them (§9.4: a
	// `(T extends Number)` carrier reaches Number ops through the
	// placeholder's lattice parent; a bare T admits nothing it can't
	// prove). Body diagnostics (undefined words, unjustified ops)
	// surface at the definition instead of waiting for a first call.
	// The placeholder bindings are re-pushed around the analysis so
	// body-internal `of [T]` resolves (to a deferred GenInstRef).
	// Non-generic fns get an equivalent construction-time analysis
	// via the dynamic-help example generator; generic params have no
	// synthesizable example values, hence this explicit path.
	if r.Check.IsActive() && genSpec != nil {
		// Names of UNCONSTRAINED type parameters (`gen [T]`, no `extends`): a
		// value of such a type is statically unknown — like an explicit `Any` —
		// so a body word over the param must match gradually, not fail
		// no_signature against a strict abstract carrier.
		unconstrained := map[string]bool{}
		for _, gp := range genSpec.Params {
			if !gp.HasBound {
				unconstrained[gp.Name] = true
			}
		}
		PushGenBindings(r, genSpec)
		for i := range fnDef.Signatures {
			s := &fnDef.Signatures[i]
			if len(s.Body()) == 0 {
				continue
			}
			paramNames := make([]string, len(s.Params))
			carrierArgs := make([]Value, len(s.Params))
			for j, p := range s.Params {
				paramNames[j] = p.Name
				t := p.Type
				if t == nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
					t = TAny
				}
				// A plain-Any param, or one typed by an unconstrained type
				// parameter, binds a DYNAMIC (gradual) carrier so a body word
				// over it (`b dot value`) matches optimistically instead of
				// failing no_signature — mirroring the gradual treatment
				// ParamInputCarrier gives an `Any` param, extended to the
				// type-parameter case this construction-time generic check hits.
				// A CONCRETE or BOUNDED (`T extends C`) param keeps a strict
				// carrier so its real shape is still checked against the bound;
				// a genuine misuse or undefined word in the body still surfaces.
				if t.Equal(TAny) || core.IsUnconstrainedTypeParam(t) || unconstrained[t.Leaf()] {
					// A value of an unconstrained type parameter (or Any) is
					// statically ANY type, so bind dynamic(Any) — a body word
					// then matches gradually. dynamic(T) is NOT enough: the
					// gradual receiver match keys on an Any bound, so a `dot` over
					// a dynamic(T) carrier still misses every concrete slot.
					carrierArgs[j] = NewDynamicCarrier(TAny)
				} else {
					carrierArgs[j] = NewCarrier(t)
				}
			}
			core.RunFnBodyAnalysis(r, "", paramNames, s.Body(), carrierArgs, fnDef.Captured, s.Returns, fnDef.Anonymous)
		}
		PopGenBindings(r, genSpec)
	}

	// NON-generic fns: QUEUE the body for the end-of-pass check. An
	// anonymous fn value never reaches InstallFnDef, so until this its body
	// was analysed by nobody — `each ([e:Any] => [nosuchw e]) [1 2 3]`
	// checked clean and then raised `undefined_word` at run time, while the
	// identical body one line up as a `def` was reported (NUR105). POSITION
	// decided it, not spelling.
	//
	// Queued, not analysed here: the construction site is too early, because
	// every forward reference is still unbound (see NoteFnBodyPending). A fn
	// that reaches InstallFnDef before the drain is analysed there instead,
	// under its name. The generic branch above stays separate because a type
	// PARAMETER has no synthesizable example value; it binds placeholder
	// carriers instead.
	if genSpec == nil {
		core.NoteFnBodyPending(r, fnDef)
	}

	// Check mode: flag overloads that an earlier, higher-priority
	// signature already subsumes — under first-match-wins dispatch they
	// can never fire (the dead-clause analogue). A static property of the
	// sig list, emitted once at fn construction.
	if r.Check.IsActive() && len(fnDef.Signatures) > 1 {
		for _, d := range core.DeadSignatures(fnDef.Signatures) {
			r.Check.AddDiagnostic(core.CheckDiagnostic{
				Code:   "unreachable_signature",
				Detail: "fn overload " + FnSigArgList(d.Sig) + " is unreachable — the earlier signature " + FnSigArgList(d.ShadowedBy) + " already accepts every call it would match",
				Word:   "fn",
			})
		}
	}

	return []Value{NewFunction(fnDef)}, nil
}

// FnSigArgList renders a signature's argument types as a short
// `[Integer String]` list for the unreachable_signature diagnostic.
func FnSigArgList(s core.Signature) string {
	ts := s.ArgTypes()
	parts := make([]string, len(ts))
	for i, t := range ts {
		if t == nil {
			parts[i] = "Any"
			continue
		}
		parts[i] = t.Leaf()
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// AfnHandler — `afn input body` constructs an anonymous Function value
// with a single signature. Mirrors the per-triple shape of ParseFnDef
// (eng/go/fn_def.go) for one triple: auto-wraps non-list input and body
// into single-element lists, parses params via the shared
// core.ParseFnParams, and constructs the FnSig with Returns=[TAny] and
// Anonymous=true. Static Returns is conservative so call sites that
// inspect the type without invoking see `Any`; check-mode dispatch
// reads the Anonymous flag and runs AnalyseFnBody for real propagation.
func AfnHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// `afn` is normally encountered as the infix form `input afn body`
	// (because `input => body` desugars to this), which makes args[1]
	// the source-left operand (input sig) and args[0] the source-right
	// operand (body). Mirrors the boru `args[1] op args[0]` convention.
	inputSig := args[1]
	body := args[0]

	// Same rule as FnTripleHandler: a COMPUTED operand (a check-mode
	// carrier — e.g. `(2 add 3) afn [body]`) makes the construction
	// interpreter-only, or the compiled unit would intern the check-time
	// Function with the carrier baked in where the live value belongs.
	for i := range args[:2] {
		if args[i].Carrier {
			if es := r.Check.Recorder(); es.Active() {
				es.MarkUncompilable("afn: construction over a computed (carrier) operand is interpreter-only — the compiled unit would bake the check-time placeholder")
			}
			break
		}
	}

	if !inputSig.Parent.Equal(TList) {
		inputSig = NewList([]Value{inputSig})
	}

	params, barrierPos, err := parseFnParams(r, inputSig)
	if err != nil {
		return nil, r.BoruError("afn_error", err.Error(), "afn")
	}

	var bodyElems []Value
	if body.Parent.Equal(TList) && body.Data != nil {
		lst, _ := AsList(body)
		bodyElems = lst.Slice()
	} else {
		bodyElems = []Value{body}
	}

	sig := FnSig{
		Params:     params,
		Returns:    []*Type{TAny},
		Impl:       Boru(bodyElems),
		BarrierPos: barrierPos,
		QuoteArgs:  core.QuoteArgsFromParams(params),
		// The lambda dispatches from this authored FnSig directly (no
		// normalizeSig), so carry value/keyword patterns onto it too —
		// otherwise a keyword param (`in/q`) would capture any word
		// instead of only its literal. See core.PatternsFromParams.
		Patterns: core.PatternsFromParams(params),
	}
	fnDef := FnDefInfo{
		Signatures: []FnSig{sig},
		Anonymous:  true,
		Captured:   core.ComputeCaptures(r, &sig),
		// Home registry, as FnConstruct stamps it: a lambda's free words
		// resolve where it was written, whichever module applies it.
		Registry: r,
	}
	// Queue the body for the end-of-pass check, exactly as FnConstruct does
	// and for the same reason: a lambda passed straight to a word (`each
	// ([e:Any] => [typo e]) xs`) is never installed under a name, so this is
	// its only chance to be analysed at all (NUR105).
	core.NoteFnBodyPending(r, fnDef)
	return []Value{NewFunction(fnDef)}, nil
}

// FnsigHandler — `fnsig [input output …]` produces a function-SHAPE
// type literal (FnUndef) from input/output sig pairs. The type-only
// counterpart to `fn` — same grammar, no body. The list length must
// be a non-zero multiple of 2 (each pair is one signature). The
// result is an FnUndef value usable as a type constraint, e.g.
// `def f:fnsig [[Integer] [String]] impl` asserts that `impl` is a
// function whose signatures cover the shape `Integer → String`.
//
// FnUndef is structural: any function value whose registered
// signatures satisfy every pair in the FnUndef matches. See
// eng/go/fnsig.go::FnUndefMatchesFnDef.
// FnsigPairHandler is the 2-arg sugar `fnsig input output`, the exact
// analogue of fn's 3-arg triple form with the body slot removed — a
// function TYPE is a function minus its body. It wraps the pair into
// the one-triple spec list the list form already parses and delegates,
// so the two spellings build an identical FnUndef and the generic
// (`gen`) path is shared rather than duplicated. Non-list input/output
// follow the same auto-wrap abbreviation rule as fn.
func FnsigPairHandler(args []Value, names map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	wrap := func(v Value) Value {
		if _, err := AsList(v); err == nil {
			return v
		}
		return NewList([]Value{v})
	}
	spec := NewList([]Value{wrap(args[0]), wrap(args[1])})
	return FnsigHandler([]Value{spec}, names, stack, r)
}

// FnpredPairHandler is the 2-arg form `fnpred <input> <body>`: wrap each
// operand as a one-element list and delegate to the spec-list form, exactly
// as FnsigPairHandler does for fnsig.
func FnpredPairHandler(args []Value, names map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	wrap := func(v Value) Value {
		if _, err := AsList(v); err == nil {
			return v
		}
		return NewList([]Value{v})
	}
	spec := NewList([]Value{wrap(args[0]), wrap(args[1])})
	return FnpredHandler([]Value{spec}, names, stack, r)
}

// FnpredHandler builds a fn from input/body PAIRS and marks it a declared
// predicate. The output slot fn requires is supplied implicitly as `Any`,
// which is the honest declaration: boru admits two membership conventions
// and they disagree on the return type — Boolean-returning
// (`fnpred n:Integer [eq 0 (mod 2 n)]`), and None-on-failure, where the body
// yields the value for a member and None for a non-member
// (`lang/spec/record.tsv` §177). Pinning the output to Boolean would decline
// the second; `Any` admits both and RunPredicate decides membership.
func FnpredHandler(args []Value, names map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("fnpred_invalid_spec", "fnpred: argument must be a concrete list", "fnpred")
	}
	_lst, _ := AsList(args[0])
	spec := _lst.Slice()
	if len(spec) == 0 || len(spec)%2 != 0 {
		return nil, r.BoruError("fnpred_invalid_spec",
			"fnpred: list length must be a non-zero multiple of 2 (input body pairs); "+
				"a predicate declares no output — use `fn` for the input/output/body form",
			"fnpred")
	}
	// Expand each [input body] pair into fn's [input output body] triple
	// with an implicit Any output.
	triples := make([]Value, 0, len(spec)/2*3)
	for i := 0; i < len(spec); i += 2 {
		triples = append(triples, spec[i], NewList([]Value{NewTypeLiteral(TAny)}), spec[i+1])
	}
	out, err := FnConstruct(r, triples, r.TakePendingGen())
	if err != nil {
		return nil, err
	}
	// FnConstruct's success path is exactly one NewFunction, and
	// MarkPredicateFn is total over the payload shape, so there is nothing
	// to guard here.
	return []Value{MarkPredicateFn(out[0])}, nil
}

func FnsigHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, &BoruError{
			Code:   "fnsig_invalid_spec",
			Detail: "fnsig: argument must be a concrete list",
		}
	}
	_lst, _ := AsList(args[0])
	spec := _lst.Slice()
	if len(spec) == 0 || len(spec)%2 != 0 {
		return nil, &BoruError{
			Code:   "fnsig_invalid_spec",
			Detail: "fnsig: list length must be a non-zero multiple of 2 (input output pairs); use `fn` for the with-body form",
		}
	}
	info, err := parseFnUndefSpec(r, spec)
	if err != nil {
		if g := r.TakePendingGen(); g != nil {
			PopGenBindings(r, g)
		}
		return nil, err
	}
	// A pending gen spec turns the shape into a generic fn-shape
	// schema (`def Mapper gen [T U] fnsig [[T] [U]]`): the
	// placeholders were live while ParseFnParams resolved T/U above.
	if g := r.TakePendingGen(); g != nil {
		return GenWrapSchema(r, g, NewFnUndef(info), SchemaFnSig)
	}
	return []Value{NewFnUndef(info)}, nil
}

// ---- args / __pa ----

func ArgsHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	top, ok, err := r.Args.Top()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, r.BoruError("args_error", "args: not inside a function", "args")
	}
	return []Value{top}, nil
}

func PopArgsHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// The Args pop and the FnBaseline pop must move together (closure-
	// capture detection on subsequent fn constructions reads the
	// baseline). core.PopFrameArgs is the single home of that pairing,
	// shared with any eager frame teardown, so the two cannot drift.
	if err := core.PopFrameArgs(r); err != nil {
		return nil, err
	}
	return nil, nil
}

// resolveTypedDefConstraint applies the name→node recoveries a
// typed-def constraint needs ahead of branch dispatch (the Stage 2
// flip, design/legacy/TYPE-REPRESENTATION.1.ignore §N2): a SCHEMA-kind NAME
// (generic schema / class / record / table / options / typed-map /
// Micron) evaluates to its minted node, and the branches dispatch on
// the declared structural content the node records; kinds that enforce
// membership through a kernel constraint Unifier (HasConstraintUnify —
// predicate / DepScalar / disjunct / negation / FnUndef) keep the
// node, whose Behavior the predicate arm and the general UnifyR
// consult directly. The second result recovers a DepScalar's bounds —
// spelt inline (`(Integer gt 10)`) or behind a name — for the
// check-mode typed-bind arm, which re-runs them at OpBindTyped.
func resolveTypedDefConstraint(constraint Value) (Value, Value) {
	if IsBareTypeNode(constraint) && !core.HasConstraintUnify(&constraint) {
		if content, ok := core.TypeContentOf(constraint); ok {
			constraint = content
		}
	}
	dep := constraint
	if !dep.IsDepScalar() {
		if content, ok := core.TypeContentOf(constraint); ok && content.IsDepScalar() {
			dep = content
		}
	}
	return constraint, dep
}
