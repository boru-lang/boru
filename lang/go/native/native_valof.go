package native

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// valofNatives registers the two words that complete boru's first-class
// function-value pipeline:
//
//   - `val name`  — resolves a name to its bound VALUE, disabling any
//     call the name would otherwise induce; companion to
//     the `/v` word suffix that lives in the
//     parser+stepWord path. It is total over every binding
//     kind: for a fn binding it suppresses the call, and
//     for any other binding it is the identity. That
//     totality is the point — it is the one spelling that
//     reads a slot whose kind is not known statically
//     (NUR085).
//   - `apply fn`  — invokes a captured function value against the
//     preceding stack args. The opposite-direction
//     complement of `valof`: val converts a call site
//     into a value, apply converts a value back into a
//     call site.
//
// Both words sit in lang because every other built-in does (eng
// ships only kernel-level shapes and parser features). The actual
// name-resolution algorithm lives in core.ResolveRef so that
// stepWord's `/v` short-circuit and this `valof` handler share one
// definition.
var valofNatives = []NativeFunc{
	{
		Name: "valof",

		Signatures: []Signature{{
			// /q on the name slot lets the parser capture the upcoming
			// Word as an Atom rather than executing it. `valof add` then
			// arrives here with args[0] = Atom(add).
			Args:      []*Type{TAtom},
			QuoteArgs: map[int]bool{0: true},
			Impl:      Go(valofHandler, RunInCheck(), Park()),
			Returns:   []*Type{TAny},
			// ParkResult: leave the resolved Function value as inert data at
			// the call site instead of re-stepping it — so `valof f` behaves
			// exactly like `f/v`, never auto-invoking (not even a 0-arg fn).
			// The value still dispatches when re-stepped elsewhere (from a
			// map, a paren), matching `(f/v)` / `ops.f a b`.
			BarrierPos: -1,
		}},
	},
	{
		Name: "apply",
		// Stack-only — across BOTH overloads, with no per-overload
		// exception (NUR098's fix, 2026-08-24): `args... fn apply` reads
		// as "take the function off the stack and apply it to the
		// preceding values," and the lens form is `receiver lens apply`.
		// Forward collection would force callers to put fn-args after
		// the fn, which fights boru's left-to-right stack flow.

		Signatures: []Signature{
			{
				Args: []*Type{TFunction},
				Impl: Go(applyHandler),
				// Check mode: return the applied fn VALUE concrete (identity,
				// not widened to Any) so the check engine re-steps it exactly as
				// runtime — the fn then dispatches against its preceding stack
				// args and records as an ordinary CALL_USER, letting the bytecode
				// compiler lower `…args fn apply`. Runtime is unchanged
				// (applyHandler still re-steps the fn).
				ReturnsFn: applyReturns,
				Returns:   []*Type{TAny}, BarrierPos: 0,
			},
			// Apply a Reach (a lens) to a receiver: `person $.name apply`
			// rebinds the reach's receiver to `person` and evaluates it —
			// the lens "get". Stack-only like the Function overload
			// (NUR098's fix): the lens on top, its receiver beneath.
			{
				Args:    []*Type{TReach, TAny},
				Impl:    Go(applyReachHandler),
				Returns: []*Type{TAny}, BarrierPos: 0,
			},
		},
	},
	{
		Name: "rebind",
		// Compose, don't evaluate: `rebind $.name person` returns a NEW
		// Reach with `person` as the receiver (inert data — a bound lens).
		// Apply it / read it later with `apply` or `getpath`. For an
		// already-bound reach it swaps the receiver. Forward-eligible.
		Signatures: []Signature{{
			Args:    []*Type{TReach, TAny},
			Impl:    Go(rebindHandler),
			Returns: []*Type{TReach}, BarrierPos: -1,
		}},
	},
	{
		Name: "usurp",
		// Forward-eligible: `usurp fn` reads as "wrap this fn". Returns a
		// Function value, so it dispatches immediately when args follow
		// (`usurp (valof f) a b`) and stays inert under quote.
		//
		// Two overloads, mirroring how `valof` and `/u` accept a name:
		//   - [Function]      `usurp (valof f)` / `usurp (f/v)` — a value.
		//   - [Atom] (/q)     `usurp f` — capture the word as a name and
		//                     resolve it to its bound function (the
		//                     function-form companion of the `/u` suffix).
		// When the next token is a Word, matchSignature prefers the /q
		// Atom sig; a parenthesised Function value takes the [Function] sig.
		Signatures: []Signature{
			{
				Args:       []*Type{TFunction},
				Impl:       Go(usurpHandler, RunInCheck()),
				Returns:    []*Type{TFunction},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(usurpAtomHandler, RunInCheck()),
				Returns:    []*Type{TFunction},
				BarrierPos: -1,
			},
		},
	},
	{
		Name: "stack-args",
		// The function-form companion of the `/s` modifier: wrap a function
		// so it dispatches in STACK form. Like `usurp`, returns a new
		// Function and accepts a value ([Function]) or a name ([Atom] /q).
		Signatures: []Signature{
			{
				Args:       []*Type{TFunction},
				Impl:       Go(stackArgsHandler, RunInCheck()),
				Returns:    []*Type{TFunction},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(stackArgsAtomHandler, RunInCheck()),
				Returns:    []*Type{TFunction},
				BarrierPos: -1,
			},
		},
	},
	{
		Name: "force-arity",
		// The function-form companion of the `/N` modifier: wrap a function
		// so it dispatches with exactly N args. `force-arity N fn` returns a
		// new Function. Like usurp, accepts a function value or a name:
		//   - [Integer, Function]  `force-arity 2 (f/v)`
		//   - [Integer, Atom] (/q) `force-arity 2 f`
		Signatures: []Signature{
			{
				Args:       []*Type{TInteger, TFunction},
				Impl:       Go(forceArityHandler, RunInCheck()),
				Returns:    []*Type{TFunction},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TInteger, TAtom},
				QuoteArgs:  map[int]bool{1: true},
				Impl:       Go(forceArityAtomHandler, RunInCheck()),
				Returns:    []*Type{TFunction},
				BarrierPos: -1,
			},
		},
	},
	{
		Name: "forward-args",
		// The function-form companion of the `/f` modifier: wrap a function
		// so it dispatches in FORWARD form.
		Signatures: []Signature{
			{
				Args:       []*Type{TFunction},
				Impl:       Go(forwardArgsHandler, RunInCheck()),
				Returns:    []*Type{TFunction},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(forwardArgsAtomHandler, RunInCheck()),
				Returns:    []*Type{TFunction},
				BarrierPos: -1,
			},
		},
	},
}

// stack-args / forward-args mirror usurp: a value form (wrap a Function) and
// a by-name form (resolve a function word, then wrap).
func stackArgsHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	return rebarrierResult(core.ForceStackFunction, args[0], "stack-args", reg)
}
func forwardArgsHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	return rebarrierResult(core.ForceForwardFunction, args[0], "forward-args", reg)
}
func stackArgsAtomHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	return rebarrierAtom(core.ForceStackFunction, args, "stack-args", reg)
}
func forwardArgsAtomHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	return rebarrierAtom(core.ForceForwardFunction, args, "forward-args", reg)
}

func forceArityHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	n, _ := args[0].AsConcreteInteger()
	wrapped, ok := core.ForceArityFunction(args[1], int(n))
	if !ok {
		if out, gradual := checkModeGradualFn(reg, args[1]); gradual {
			recordGradualWrap(reg, "force-arity", args, out)
			return out, nil
		}
		detail := "force-arity requires a non-negative arity and a function value"
		if reg != nil {
			return nil, reg.BoruError("illegal_ref", detail, "force-arity")
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return []Value{wrapped}, nil
}

// forceArityAtomHandler is the by-name form `force-arity N foo`: it
// resolves the captured atom to its bound function (sharing usurp's rules)
// and wraps it. The function-form companion of the `/N` suffix.
// unboundNameError raises undefined_word for a by-name reference whose
// atom resolves to nothing, with the same did-you-mean the bare-word
// path offers (diagnostics phase 4).
func unboundNameError(reg *Registry, detail, name string) error {
	err := reg.BoruError("undefined_word", detail, name)
	if ae, ok := err.(*core.BoruError); ok {
		if s := core.DidYouMean(name, reg.SuggestionCandidates()); s != "" {
			ae.Suggestions = append(ae.Suggestions, core.DiagSuggestion{Message: s})
		}
	}
	return err
}

func forceArityAtomHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	name, err := AsAtom(args[1])
	if err != nil {
		return nil, fmt.Errorf("force-arity: expected an atom name, got %s", args[1].Parent.String())
	}
	v, ok := core.ResolveRef(reg, name)
	if !ok {
		if reg != nil {
			return nil, unboundNameError(reg, "force-arity: name "+name+" is not bound", name)
		}
		return nil, fmt.Errorf("force-arity: name %s is not bound", name)
	}
	if !core.IsFunctionRef(v) {
		// reg is provably non-nil here: ResolveRef only returns ok for a
		// non-nil registry, so a resolved-but-non-fn binding cannot occur
		// with reg == nil (design/TEST-SEAMS.10.md dead-guard removal).
		detail := "force-arity requires a function word: " + name + " is bound to " + v.Parent.String()
		return nil, reg.BoruError("illegal_ref", detail, name)
	}
	return forceArityHandler([]Value{args[0], v}, nil, nil, reg)
}

func rebarrierResult(wrap func(Value) (Value, bool), v Value, word string, reg *Registry) ([]Value, error) {
	wrapped, ok := wrap(v)
	if !ok {
		if out, gradual := checkModeGradualFn(reg, v); gradual {
			// Sound from the by-name (rebarrierAtom) entry too: the recorded
			// operand is the RESOLVED value v (a local / event / const), never
			// the atom, so the compiled program re-supplies the same value the
			// interpreter wrapped — no runtime name resolution is implied.
			recordGradualWrap(reg, word, []Value{v}, out)
			return out, nil
		}
		detail := word + " requires a function value, got " + v.Parent.String()
		if reg != nil {
			return nil, reg.BoruError("illegal_ref", detail, word)
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return []Value{wrapped}, nil
}

func rebarrierAtom(wrap func(Value) (Value, bool), args []Value, word string, reg *Registry) ([]Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("%s: missing name", word)
	}
	name, err := AsAtom(args[0])
	if err != nil {
		return nil, fmt.Errorf("%s: expected an atom name, got %s", word, args[0].Parent.String())
	}
	v, ok := core.ResolveRef(reg, name)
	if !ok {
		if reg != nil {
			return nil, unboundNameError(reg, word+": name "+name+" is not bound", name)
		}
		return nil, fmt.Errorf("%s: name %s is not bound", word, name)
	}
	if !core.IsFunctionRef(v) {
		// reg is provably non-nil here (ResolveRef only returns ok for a
		// non-nil registry); the former reg==nil arm was dead.
		detail := word + " requires a function word: " + name + " is bound to " + v.Parent.String()
		return nil, reg.BoruError("illegal_ref", detail, name)
	}
	return rebarrierResult(wrap, v, word, reg)
}

// usurpAtomHandler is the by-name form `usurp foo`: it resolves the
// captured atom to its bound function value. Unlike `valof`, which is
// total over binding kinds, usurp still gates on fn-ness: an unbound name
// raises undefined_word and a non-fn binding raises illegal_ref (there is
// no reversed wrapper to build over a non-fn value) and then returns the argument-reversed wrapper. It is the
// function-form companion of the `/u` word suffix; `usurp (valof foo)` /
// `usurp (foo/v)` pass the value directly via the [Function] overload.
func usurpAtomHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usurp: missing name")
	}
	name, err := AsAtom(args[0])
	if err != nil {
		return nil, fmt.Errorf("usurp: expected an atom name, got %s", args[0].Parent.String())
	}
	v, ok := core.ResolveRef(reg, name)
	if !ok {
		if reg != nil {
			return nil, unboundNameError(reg, "usurp: name "+name+" is not bound", name)
		}
		return nil, fmt.Errorf("usurp: name %s is not bound", name)
	}
	if !core.IsFunctionRef(v) {
		// reg is provably non-nil here (ResolveRef only returns ok for a
		// non-nil registry); the former reg==nil arm was dead.
		detail := "usurp requires a function word: " + name + " is bound to " + v.Parent.String()
		return nil, reg.BoruError("illegal_ref", detail, name)
	}
	return usurpHandler([]Value{v}, nil, nil, reg)
}

// refHandler resolves the captured atom name to its bound value and
// returns it. Failure to bind raises an undefined_word error, the
// same code stepWord raises for an unbound bare word — so the two
// surfaces report identical errors.
func valofHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("valof: missing name")
	}
	name, err := AsAtom(args[0])
	if err != nil {
		return nil, fmt.Errorf("valof: expected an atom name, got %s", args[0].Parent.String())
	}
	v, ok := core.ResolveRef(reg, name)
	if !ok {
		if reg != nil {
			return nil, unboundNameError(reg, "valof: name "+name+" is not bound", name)
		}
		return nil, fmt.Errorf("valof: name %s is not bound", name)
	}
	// `valof` is the function-form companion of the `/v` suffix and shares
	// its rule: it yields the binding's VALUE whatever kind it is. For a
	// fn binding that suppresses the call; for any other binding it is
	// the identity. No kind gate — that is what makes one spelling able
	// to read a slot whose kind is not known statically (NUR085).
	return []Value{v}, nil
}

// applyHandler unquotes the captured Function value and returns it.
// The engine's stepLiteral check then fires execFnDefLiteral, which
// dispatches the function against whatever stack args precede it.
//
// For boru-defined fns the dispatch uses the captured FnDef's own
// Sigs table, so the call is stable even when the original binding
// has been redefined or undef'd.
//
// For native fns the captured payload has Signatures but no Sigs,
// and execFnDefLiteral's pure-stack path is FnSig-based — those will
// reach apply, unquote, but fall back to passing through. Native fn
// captures still serve as TFunction-slot args to higher-order words
// (filter, walk, behave) where the consumer's handler calls into the
// engine directly via CallBoru.
// applyReturns is `apply`'s check-mode model: ReturnsIdentity(0), so the
// check engine re-steps the fn value exactly as runtime does and an
// arg-taking callee records as an ordinary call and lowers.
//
// The 0-arg shape needs no special case here, and that is the point of
// marking rather than calling (applyHandler below). Because the mark is
// consumed by the re-step, the check engine walks the SAME gate the
// interpreter does: it dispatches the applied fn and carries its result
// forward, so a downstream consumer — `f/v apply add 1` — type-checks
// against the RESULT rather than against the Function. An earlier revision
// applied at the handler and had to declare the shape uncompilable to stay
// honest; re-stepping models it exactly instead, so there is nothing left
// to refuse.
func applyReturns(args []Value, r *Registry) []Value {
	out := ReturnsIdentity(0)(args, r)
	if len(out) == 1 {
		out[0] = markApplied(out[0])
	}
	return out
}

// markApplied stamps `apply`'s one-shot application signal on a fn value
// whose only signatures are 0-arg. The decision lives in core.MarkApplied,
// the SINGLE source the runtime handler, the check-mode model and the VM's
// apply-word op share, so no two engines can disagree about which values
// the re-step's inert-lambda gate must yield for. That is the whole reason
// the mark is a value stamp rather than a call — a handler runs only at
// runtime, and the check pass would have had to guess.
func markApplied(v Value) Value { return core.MarkApplied(v) }

func applyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	v := args[0]
	if !v.Parent.Equal(TFunction) {
		return nil, fmt.Errorf("apply: expected Function, got %s", v.Parent.String())
	}
	if _, ok := v.Data.(FnDefInfo); !ok {
		return nil, fmt.Errorf("apply: function value carries no FnDefInfo (got %T)", v.Data)
	}
	v.Quoted = false
	// ADR-016 (NUR077 §5 Hole 1): a fn whose ONLY signatures are 0-arg
	// must apply the same way whatever its ORIGIN. The re-step reaches
	// execFnDefLiteral's data gate, which parks a 0-arg ANONYMOUS value —
	// and that gate is load-bearing (it is what makes `def f ([] => [42])`
	// bind f to the FUNCTION), so it stays. But letting it also answer
	// "was a call asked for?" made origin decide the outcome:
	//
	//	def z fn [[] [Integer] [42]]   z/v apply  ->  42
	//	def f ([] => [42])             f/v apply  ->  fn f   (was)
	//
	// So state the application instead of performing it: mark the value
	// Applied and hand it back. The gate reads the mark and yields, and
	// the call runs down the ONE dispatch path every other arity already
	// uses. Calling here instead — through the since-retired CallBoruFn — was a
	// SECOND path,
	// and it diverged from the first in three ways the re-step gets right
	// by construction: a native 0-arg fn (`valof context apply`) kept its
	// Go handler instead of running an empty body; a body mutating the
	// context (`context set x/q 2`) landed those mutations in the caller's
	// frame instead of a sub-engine's; and the check pass modelled the
	// RESULT, because ReturnsIdentity(0) re-steps exactly as runtime does
	// — so `f/v apply add 1` type-checks as `43` rather than failing
	// no_signature over (Function, Integer).
	//
	// Restricted to fns with no arg-taking overload, so a nullary
	// signature can never eclipse an arg-taking sibling the stack was
	// about to satisfy (the NUR035 hazard). Macros are excluded by the
	// gate itself — applying a macro is never a stack-value dispatch
	// (design/MACROS-PHASE1.10.md §5, D4).
	return []Value{markApplied(v)}, nil
}

// applyReachHandler applies a Reach (a lens) to a receiver value: it rebinds
// the reach's receiver to args[1] and evaluates the segment walk (the lens
// "get"). getr strictness and computed keys behave exactly as bare m.a.b.
func applyReachHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	info, err := AsReach(args[0])
	if err != nil {
		return nil, fmt.Errorf("apply: %w", err)
	}
	out, err := ApplyReach(r, info, args[1])
	if err != nil {
		return nil, err
	}
	return []Value{out}, nil
}

// rebindHandler returns a new inert Reach with its receiver swapped to
// args[1] — composition, not evaluation. The result is a bound lens (data);
// `apply` / `getpath` read through it.
func rebindHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	info, err := AsReach(args[0])
	if err != nil {
		return nil, fmt.Errorf("rebind: %w", err)
	}
	bound := ReachInfo{
		Receiver: []Value{args[1]},
		Segments: info.Segments,
		Eval:     false,
	}
	return []Value{NewReach(bound)}, nil
}

// usurpHandler wraps a Function value so its signature argument order is
// reversed: the wrapper called `usurped a b c` dispatches the original as
// `f c b a`. Mirrors the /u modifier (core.UsurpFunction). The wrapper is
// returned unquoted, so — like a bare function word — it dispatches
// immediately when args follow, and stays inert when captured with quote.
func usurpHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	wrapped, ok := core.UsurpFunction(args[0])
	if !ok {
		if out, gradual := checkModeGradualFn(reg, args[0]); gradual {
			recordGradualWrap(reg, "usurp", args, out)
			return out, nil
		}
		detail := "usurp requires a function value, got " + args[0].Parent.String()
		if reg != nil {
			return nil, reg.BoruError("illegal_ref", detail, "usurp")
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return []Value{wrapped}, nil
}

// recordGradualWrap records a dispatch-modifier word's GRADUAL dispatch (a
// dynamic fn operand the check-mode handler could not wrap — checkModeGradualFn)
// as an OpCallNativePoly event, so the wrapper is constructed at RUN time by the
// real handler over the real fn value (`m.a/u 1 2` lowers get → poly usurp →
// OpCallDynamic; Stage M2b, design/STAGE3-INLINING-DESIGN-ROUND.0.md). The VM's
// callPoly re-matches the word's own signatures with the kernel's own
// MatchSignature — the exact dispatch the interpreter takes — so a runtime
// non-fn value raises the identical illegal_ref. Only the VALUE-form
// ([Function]-sig) sites record: the by-name Atom forms resolve a registry
// binding the compiled program does not maintain, so they stay unrecorded
// (downstream provenance refuses and the program falls back, the status quo).
// RecordPolyCall declining (an unresolvable operand, inactive recorder) leaves
// the recorder untouched — the residual then refuses, never miscompiles.
func recordGradualWrap(reg *Registry, word string, args, outs []Value) {
	if reg == nil {
		return
	}
	pos := core.SrcPos{}
	if len(args) > 0 {
		pos = args[len(args)-1].Pos()
	}
	reg.Check.Recorder().RecordPolyCall(word, args, outs, pos, nil, nil)
}

// checkModeGradualFn handles a dispatch-modifier word (usurp / stack-args /
// forward-args / force-arity) applied to a NON-CONCRETE function-value carrier
// in check mode. A stored fn-ref read via dot-access (`m.a` where
// `m = {a:add/v}`) is statically dynamic(Any) — getNodeReturns deliberately
// cannot narrow a dispatch-bearing field — but a real Function at run time.
// Rather than the strict handler rejecting the Any carrier (a false no_signature
// / illegal_ref, path-modifier.tsv), return a gradual Function carrier so the
// downstream arg dispatch types optimistically. Only fires for a carrier the
// real wrapper already declined (a concrete Function still wraps normally).
func checkModeGradualFn(reg *Registry, v Value) ([]Value, bool) {
	if reg == nil || !reg.Check.IsActive() || IsConcrete(v) || v.Parent == nil {
		return nil, false
	}
	// Only a STATICALLY-UNKNOWN carrier (a dynamic Any from a dispatch-bearing
	// field read like `m.a`) or an actual Function-typed carrier is plausibly a
	// function at run time. A concrete-TYPED carrier (an Integer / String
	// binding, e.g. `def x 5 x/u`) is provably NOT a function — let the strict
	// handler flag it (a real illegal_ref), so the fallback never masks a
	// genuine non-fn modifier error.
	if (v.Dynamic && v.Parent.Equal(TAny)) || v.Parent.ConformsTo(TFunction) {
		return []Value{NewDynamicCarrier(TFunction)}, true
	}
	return nil, false
}
