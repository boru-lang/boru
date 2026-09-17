package core

// ResolveRef looks up name in the registry and returns the value-form
// of its current binding without invoking. Resolution mirrors the
// priority used by stepWord:
//
//  1. A type binding (capitalised def, refine-prefab) returns its
//     stored body — typically a type literal.
//  2. A value binding returns the bound value: an FnDef binding is
//     wrapped as a Function value; every other binding is returned
//     as-is.
//
// The returned Function value is UNQUOTED. Under the dispatch rules
// of this engine, an unquoted Function on the stack is a live call
// site — full signature matching (forward + stack) applies the next
// time the engine processes it. To capture as inert data, wrap with
// `quote` at the call site.
//
// The second return is false when the name is not bound at all. The
// caller decides how to report the failure — the `ref` word raises
// an undefined_word error, the /v short-circuit in stepWord does the
// same.
//
// Lives in eng because stepWord's /v path needs it during the run
// loop; the `ref` word itself is registered in the language layer.
func ResolveRef(r *Registry, name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	top, ok := r.Defs.Top(name)
	if !ok {
		return Value{}, false
	}
	// Resolving a name through a reference (`name/v`, the `ref` word, an
	// export-map value) IS a use of that def — record it so unused-def
	// analysis in check mode does not falsely flag reference-exported
	// public words (the canonical `export "X" { f: impl/v }` form).
	// A type binding resolves through Top like every other binding, so it
	// yields what the name DENOTES — the minted lattice node, the same
	// value the bare name evaluates to (design/legacy/TYPE-REPRESENTATION.1.ignore
	// §6); the declared body is the node's CONTENT (`TypeContentOf`), not
	// the binding's value.
	r.noteAnalysisUse(name)
	if _, ok := top.Data.(FnDefInfo); ok {
		// Wrap the aggregate dispatch view so the reference carries every
		// overload of the name, not just the topmost entry's own sigs.
		if fnDef := r.Lookup(name); fnDef != nil {
			return NewFunction(*fnDef), true
		}
	}
	return top, true
}

// IsFunctionRef reports whether a value resolved by ResolveRef is a
// function word — the only binding kind `/v` and `ref` are permitted to
// reference. The reference surfaces exist to break the asymmetry between
// value bindings (a bare name already pushes the value) and fn bindings
// (a bare name invokes); for a non-fn binding there is no asymmetry to
// break, so referencing it is meaningless and rejected. ResolveRef wraps
// every FnDef binding as a Function value (Parent == TFunction), so the
// predicate is a single Parent check; plain values and type bodies come
// back with their own Parent and are illegal ref targets.
func IsFunctionRef(v Value) bool {
	return v.Parent.Equal(TFunction)
}

// UsurpFunction wraps a function value so its signature argument order is
// reversed: a wrapped fn called `usurped a b c` dispatches the original as
// `f c b a`. It returns false when v is not a function value.
//
// The wrapper is a fresh TFunction whose signatures each copy an original
// signature with the per-position Params REVERSED (so the wrapper accepts
// the caller's args in usurped order, with Pattern / Optional / types still
// aligned to each slot) and an all-forward BarrierPos so the natural
// `usurped a b c` forward form matches. Each wrapper sig carries a Go
// handler that re-dispatches the ORIGINAL function with the matched args in
// reverse order (see usurpDispatchHandler). Because the original sig runs
// unchanged on re-dispatch, every original behaviour (barriers, quoting,
// return checks, closures, module scope) is preserved; usurp only reverses
// which caller argument lands in which original slot.
func UsurpFunction(v Value) (Value, bool) {
	if !v.Parent.Equal(TFunction) {
		return Value{}, false
	}
	fnDef, ok := v.Data.(FnDefInfo)
	if !ok {
		return Value{}, false
	}
	orig := NewFunction(fnDef)
	own := fnDef.OwnSigs()
	wrapped := make([]Signature, 0, len(own))
	for i := range own {
		src := own[i]
		rev := src // copy Returns (output shape is unchanged) and other flags
		rev.Params = reverseParams(src.Params)
		rev.BarrierPos = len(rev.Params) // all-forward: caller writes usurped a b c
		// Drop per-position fields that would now be mis-indexed; the
		// original sig re-applies them on re-dispatch. Args/Patterns are
		// rebuilt from Params by normalizeSig at registration time.
		rev.Args = nil
		rev.Patterns = nil
		rev.QuoteArgs = nil
		rev.TypeArgs = nil
		rev.NoEvalArgs = nil
		rev.NoEvalMapArgs = nil
		rev.ReturnsFn = nil
		rev.Fallback = false
		// The re-dispatch layout must respect the ORIGINAL sig's barrier so a
		// stack-only or mixed-barrier original still receives its args in the
		// right place (the wrapper itself stays all-forward). Resolve the
		// sentinel (-1 → all-forward) and clamp to [0, N].
		origBarrier := src.BarrierPos
		if origBarrier < 0 || origBarrier > len(src.Params) {
			origBarrier = len(src.Params)
		}
		// A Go re-dispatch wrapper: it re-dispatches the ORIGINAL (which carries
		// its own frame) via a Go handler, so it is a GoImpl — replacing any
		// inherited boru impl. RunInCheck lets the carrier compiler step the
		// re-dispatch and compile the original call directly — `usurp (valof f) a
		// b` lowers exactly like `f b a` — instead of refusing the opaque
		// wrapper dispatch. Soundness rides the differential (the compiled
		// re-dispatch is byte-identical to the runtime one).
		rev.Impl = Go(usurpDispatchHandler(orig, origBarrier), RunInCheck())
		NormalizeSig(&rev)
		wrapped = append(wrapped, rev)
	}
	SortSignatures(wrapped)
	return NewFunction(FnDefInfo{
		Name:           fnDef.Name,
		Signatures:     wrapped,
		MaxForwardArgs: calcMaxForwardArgs(wrapped),
		Registry:       fnDef.Registry,
		// The reversal above is invisible to any later inspection of the
		// value — same Name, and for a HOMOGENEOUS sig the same param types
		// in the same order (`sub(Number, Number)` reversed is itself). Mark
		// it so a consumer that would otherwise dispatch through the wrapped
		// word's own live binding can decline instead of dropping the swap.
		ArgsReversed: true,
		// …and what it wraps, so a consumer WITHOUT a tape can perform the
		// re-dispatch itself rather than stepping the handler's tokens.
		Wrap:  WrapReverse,
		Wraps: &orig,
	}), true
}

// ForceStackFunction / ForceForwardFunction are the function-form
// companions of the `/s` and `/f` modifiers (emitted by the `stack-args`
// and `forward-args` words), mirroring UsurpFunction. They wrap a function
// value in a fresh TFunction whose signatures copy the originals — params
// UNCHANGED — but with the wrapper's BarrierPos forced all-stack (`/s`) or
// all-forward (`/f`), so the wrapper collects the caller's args in that
// form. Each wrapper sig re-dispatches the ORIGINAL with the args in caller
// order (respecting the original's own barrier), so all original behaviour
// is preserved. Return false when v is not a function value.
func ForceStackFunction(v Value) (Value, bool)   { return rebarrierFunction(v, true) }
func ForceForwardFunction(v Value) (Value, bool) { return rebarrierFunction(v, false) }

func rebarrierFunction(v Value, stack bool) (Value, bool) {
	if !v.Parent.Equal(TFunction) {
		return Value{}, false
	}
	fnDef, ok := v.Data.(FnDefInfo)
	if !ok {
		return Value{}, false
	}
	orig := NewFunction(fnDef)
	own := fnDef.OwnSigs()
	wrapped := make([]Signature, 0, len(own))
	for i := range own {
		src := own[i]
		ws := src // copy Returns and other flags
		ws.Params = append([]FnParam(nil), src.Params...)
		if stack {
			ws.BarrierPos = 0 // all-stack: caller writes  a b c fn
		} else {
			ws.BarrierPos = len(ws.Params) // all-forward: caller writes  fn a b c
		}
		// Drop per-position fields; the original sig re-applies them on
		// re-dispatch. Args/Patterns are rebuilt from Params by normalizeSig.
		ws.Args = nil
		ws.Patterns = nil
		ws.QuoteArgs = nil
		ws.TypeArgs = nil
		ws.NoEvalArgs = nil
		ws.NoEvalMapArgs = nil
		ws.ReturnsFn = nil
		ws.Fallback = false
		origBarrier := src.BarrierPos
		if origBarrier < 0 || origBarrier > len(src.Params) {
			origBarrier = len(src.Params)
		}
		// A Go re-dispatch wrapper (see UsurpFunction): replaces any inherited
		// boru impl with a GoImpl. Like usurp, running it in CHECK mode lets the
		// carrier compiler step the re-dispatch and compile the original call
		// directly — `forward-args f a b` / `a b stack-args f` lower exactly like
		// the plain `f` call — instead of refusing the opaque wrapper. Soundness
		// rides the differential.
		ws.Impl = Go(rebarrierDispatchHandler(orig, origBarrier), RunInCheck())
		NormalizeSig(&ws)
		wrapped = append(wrapped, ws)
	}
	SortSignatures(wrapped)
	return NewFunction(FnDefInfo{
		Name:           fnDef.Name,
		Signatures:     wrapped,
		MaxForwardArgs: calcMaxForwardArgs(wrapped),
		Registry:       fnDef.Registry,
		// A wrapper composes: if the value it wraps already reverses its
		// args (usurp), this one still does, and the marker has to survive
		// or a consumer downstream sees an unmarked reversal. Measured:
		// `force-arity 2 (usurp (m.s)) 10 3` answered -7 against the
		// interpreter's 7 while this line was missing.
		ArgsReversed: fnDef.ArgsReversed,
		// A rebarrier leaves the arg-to-param mapping alone — the barrier is
		// spent once the args are collected — so a tape-less consumer can
		// dispatch Wraps with the args UNCHANGED.
		Wrap:  WrapRebarrier,
		Wraps: &orig,
	}), true
}

// ForceArityFunction is the function-form companion of the `/N` modifier
// (emitted by the `force-arity` word). It wraps a function value in a fresh
// TFunction with a single N-arg all-forward signature that re-dispatches
// the original in forward form — so `force-arity 2 f a b` runs `f a b`,
// selecting the original's 2-arg overload. Returns false when v is not a
// function value or n < 0.
func ForceArityFunction(v Value, n int) (Value, bool) {
	if n < 0 || !v.Parent.Equal(TFunction) {
		return Value{}, false
	}
	fnDef, ok := v.Data.(FnDefInfo)
	if !ok {
		return Value{}, false
	}
	orig := NewFunction(fnDef)
	params := make([]FnParam, n)
	for i := range params {
		params[i] = FnParam{Type: TAny}
	}
	sig := Signature{
		Params:     params,
		BarrierPos: n, // all-forward: caller writes  f a b …
		// Re-dispatch the original respecting ITS barrier (not always forward) so
		// force-arity composes over stack-form wrappers (e.g. force-arity over
		// stack-args). Re-dispatch in check mode so the carrier compiler compiles
		// the original call directly (`force-arity 2 f a b` lowers like `f a b`),
		// mirroring usurp / rebarrier. Soundness rides the differential.
		Impl: Go(rebarrierDispatchHandler(orig, funcSigBarrier(orig, n)), RunInCheck()),
	}
	NormalizeSig(&sig)
	return NewFunction(FnDefInfo{
		Name:           fnDef.Name,
		Signatures:     []Signature{sig},
		MaxForwardArgs: n,
		Registry:       fnDef.Registry,
		// A wrapper composes: if the value it wraps already reverses its
		// args (usurp), this one still does, and the marker has to survive
		// or a consumer downstream sees an unmarked reversal. Measured:
		// `force-arity 2 (usurp (m.s)) 10 3` answered -7 against the
		// interpreter's 7 while this line was missing.
		ArgsReversed: fnDef.ArgsReversed,
		// A rebarrier leaves the arg-to-param mapping alone — the barrier is
		// spent once the args are collected — so a tape-less consumer can
		// dispatch Wraps with the args UNCHANGED.
		Wrap:  WrapRebarrier,
		Wraps: &orig,
	}), true
}

// funcSigBarrier returns the resolved BarrierPos of orig's n-arg signature
// (all-forward when there is none) — used to lay out a re-dispatch.
func funcSigBarrier(orig Value, n int) int {
	fnDef, ok := orig.Data.(FnDefInfo)
	if !ok {
		return n
	}
	for _, s := range fnDef.OwnSigs() {
		if s.TotalArgs() == n {
			b := s.BarrierPos
			if b < 0 || b > n {
				b = n
			}
			return b
		}
	}
	return n
}

// rebarrierDispatchHandler re-emits the original function with the caller's
// args in ORDER (no reversal), laid out around `orig` to satisfy the
// original's barrier B: positions B…N-1 on the stack (bottom→top, deepest
// first) and positions 0…B-1 forward after orig.
func rebarrierDispatchHandler(orig Value, origBarrier int) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		n := len(args)
		b := origBarrier
		if b < 0 || b > n {
			b = n
		}
		out := make([]Value, 0, n+3)
		out = append(out, NewOpenParen())
		for i := n - 1; i >= b; i-- { // stack part: pos n-1 … b
			out = append(out, args[i])
		}
		out = append(out, orig)
		for i := 0; i < b; i++ { // forward part: pos 0 … b-1
			out = append(out, args[i])
		}
		out = append(out, NewCloseParen())
		return out, nil
	}
}

// reverseParams returns a new slice with the params in reverse order.
func reverseParams(params []FnParam) []FnParam {
	out := make([]FnParam, len(params))
	for i, p := range params {
		out[len(params)-1-i] = p
	}
	return out
}

// usurpDispatchHandler builds the handler for one usurped signature. The
// wrapper sig collected the caller's args in usurp order (args[0] is the
// first value the caller wrote). To realise usurped a0…a(N-1) ≡
// orig a(N-1)…a0, it re-emits the ORIGINAL function value with the reversed
// args laid out around it, paren-wrapped so the re-dispatch is atomic.
//
// WHERE the args go depends on the original sig's barrier B (= origBarrier):
// the original collects positions 0…B-1 from forward tokens (after orig) and
// positions B…N-1 from the stack (before orig, top-first). Its natural call
// layout is therefore  [pN-1 … pB] orig [p0 … pB-1].  Substituting the
// reversal pi = a(N-1-i) gives one layout that is correct for EVERY barrier:
//
//		( a0 a1 … a(N-1-B)   orig   a(N-1) a(N-2) … a(N-B) )
//		  └── stack part ──┘        └──── forward part ────┘
//
//	  - B == N (all-forward, the common case): stack part empty → ( orig aN-1…a0 ).
//	  - B == 0 (stack-only): forward part empty → ( a0…aN-1 orig ); orig reads
//	    the stack top-first, yielding the reversal. (Fixes usurp of stack-only
//	    words, which previously left the wrapper inert.)
//	  - 0 < B < N (mixed): both parts populated, args split at the barrier.
//
// The original sig runs unchanged, preserving every original behaviour
// (barriers, return checks, closures, module scope).
func usurpDispatchHandler(orig Value, origBarrier int) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		n := len(args)
		b := origBarrier
		if b < 0 || b > n {
			b = n
		}
		out := make([]Value, 0, n+3)
		out = append(out, NewOpenParen())
		// Stack part: a0 … a(N-1-B), in order (bottom→top).
		for i := 0; i < n-b; i++ {
			out = append(out, args[i])
		}
		out = append(out, orig)
		// Forward part: a(N-1) down to a(N-B).
		for i := n - 1; i >= n-b; i-- {
			out = append(out, args[i])
		}
		out = append(out, NewCloseParen())
		return out, nil
	}
}

// ResolveUsurp resolves name to its bound function value and returns the
// usurped wrapper (see UsurpFunction). The second return is false when the
// name is unbound; an ok-but-non-function binding is returned as-is so the
// caller's IsFunctionRef check raises illegal_ref (mirrors ResolveRef).
func ResolveUsurp(r *Registry, name string) (Value, bool) {
	v, ok := ResolveRef(r, name)
	if !ok {
		return Value{}, false
	}
	if !IsFunctionRef(v) {
		return v, true
	}
	wrapped, _ := UsurpFunction(v)
	return wrapped, true
}
