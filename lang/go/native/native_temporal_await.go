package native

import (
	"fmt"
	"sort"
	"sync"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	eng "github.com/boru-lang/boru/eng/go"
)

// The "await" word lives in miscNatives (native_misc.go). Below
// are the parallel-branch helpers used by the dispatcher.

// parallelResult holds the outcome of one parallel branch.
type parallelResult struct {
	index  int
	values []Value
	err    bool // true if the branch produced an error value or runtime error
}

// branchSharingViolation reports the binding name and container kind of
// the first STATEFUL MUTABLE container an `await` branch body can reach
// from the parent scope, or ("", "") when every branch is clean.
//
// ForkConcurrent isolates execution SCOPES — a `def` or `context set` on
// a branch stays private — and its header says exactly that. It does not
// isolate bound VALUES: cloning a binding table copies Values, and a
// FlexMap / FlexList / Store / class-instance / Table Value holds a
// POINTER to shared state. Two branches mutating one such container race
// on `OrderedMap.Set`, an unsynchronised map assign plus a slice append,
// which is undefined behaviour and not merely a surprise.
//
// The rule is `send`'s — refuse a mutable container at a concurrency
// boundary — but the PREDICATE has to be its own, and the reason is worth
// stating because it is easy to get wrong by reaching for the obvious
// reuse. `send` asks "can this be COPIED across?" and deep-copies what it
// admits (CloneValue at the boundary), so it only refuses containers a
// copy cannot preserve. `await` asks "can this be SHARED across?" and
// copies nothing, so it must refuse anything mutated in place — a
// strictly broader set.
//
// Concretely, sendableViolation misses FlexMap: a FlexMap is
// `MapPayload{*OrderedMap}` with a TFlexMap tag, payload-identical to an
// immutable Map, so a payload-keyed switch reads it as a plain map. That
// is harmless for `send` (the clone makes it safe either way) and fatal
// here. So this discriminates on the TYPE TAG, which is what actually
// separates in-place mutation from copy-on-write.
//
// LIMITS, stated rather than implied — this is a boundary rule, not a
// proof of non-aliasing. Reachability is what a branch body REFERENCES,
// followed one level into any referenced function (its captures and its
// body), plus a scope-wide fallback when a reference cannot be resolved
// at all (branchRefIsOpaque / scopeSharedMutable).
//
// What it does NOT see: a container reached only through a longer chain
// of calls, and one held in a VM frame slot with nothing mutable visible
// in `r.Defs` to trip the opaque fallback.
//
// The fn-local hole this used to have is CLOSED. Compiled, `def w ([] =>
// [m set a 1])` inside a fn body puts `w` in a frame slot, so the walk
// dead-ended at `w`; the check then refused the harmless shapes (`[m]`,
// `[m get a]`) and permitted the mutating one — a real data race under
// `-race`, on the default path. All four shapes now refuse in both
// engines, pinned by TestAwaitRefusesFnLocalLambdaInBothEngines, with
// TestAwaitOpaqueRefIsNotRefusedWithoutAMutable holding the other side so
// the fallback cannot grow into refusing map keys and branch-locals.
func sharedMutableKind(v Value) string {
	if v.Parent == nil {
		return ""
	}
	switch {
	case v.Parent.ConformsTo(TFlexMap), v.Parent.ConformsTo(TWeakFlexMap):
		return "FlexMap"
	case v.Parent.ConformsTo(TFlexList), v.Parent.ConformsTo(TWeakFlexList):
		return "FlexList"
	case v.Parent.ConformsTo(TStore):
		return "Store"
	case v.Parent.ConformsTo(TTable):
		return "Table"
	}
	switch d := v.Data.(type) {
	case ClassInstanceInfo:
		return "Object"
	case ListPayload:
		for _, e := range d.Elems {
			if k := sharedMutableKind(e); k != "" {
				return k
			}
		}
	case MapPayload:
		if d.M != nil {
			for _, key := range d.M.Keys() {
				e, _ := d.M.Get(key)
				if k := sharedMutableKind(e); k != "" {
					return k
				}
			}
		}
	}
	return ""
}

// branchTokens returns a branch element's body tokens for either shape:
// the raw token list (interpreted) or the BoruImpl body carried by a
// compiled fn-value (the shape runParallelBranch unwraps via RunUnit).
func branchTokens(elem Value) []Value {
	if fd, ok := elem.Data.(core.FnDefInfo); ok {
		for i := range fd.Signatures {
			if a, isBoru := fd.Signatures[i].Impl.(*core.BoruImpl); isBoru {
				return a.Body
			}
		}
		return nil
	}
	body, err := AsList(elem)
	if err != nil || body.IsNil() {
		return nil
	}
	return body.Slice()
}

// branchRefIsOpaque reports whether a branch-body word resolves to
// NOTHING the check can inspect.
//
// It is consulted only for a name the walk already failed to resolve in
// `r.Defs`, and that failure is the whole signal: compiled, a fn-body
// local is exactly what it looks like — `def w ([] => …)` inside a fn
// body lives in a VM frame slot, not in r.Defs, and a native handler has
// no path to frame locals.
//
// The one exception is a bare LITERAL. `true` / `false` / `none` arrive
// as Words and never resolve, but they are values, not bindings — a
// branch body of `[true]` reaches nothing. Without this case an
// `await [[true] [false]]` with any FlexMap in scope would be refused
// outright.
//
// Registered words and type names deliberately have no case here. Both
// were tried, and both are unreachable from this call site: every
// registered word also carries a `r.Defs` binding (and `undef` of a
// builtin is refused as `reserved_word`), while a kernel type name is
// converted by the parser into a type literal and never arrives as a
// Word at all. Measured across the whole Go test corpus, every name
// reaching this function misses both lookups. A guard that cannot fire
// is worse than no guard: it reads as protection that isn't there.
func branchRefIsOpaque(name string) bool {
	switch name {
	case "true", "false", "none":
		return false
	}
	return true
}

// scopeSharedMutable returns the first binding in scope that is an
// in-place-mutable container. It answers the only question that matters
// once a branch reference has gone opaque: is there anything here for it
// to reach?
func scopeSharedMutable(r *Registry) (string, string) {
	names := r.Defs.Names()
	sort.Strings(names) // deterministic: the same program names the same binding
	for _, n := range names {
		if v, ok := r.Defs.Top(n); ok {
			if bad := sharedMutableKind(v); bad != "" {
				return n, bad
			}
		}
	}
	return "", ""
}

func branchSharingViolation(r *Registry, elems []Value) (string, string) {
	seen := map[string]bool{}
	var offender, kind string
	// opaque records a branch reference the check could not resolve. It is
	// not a violation by itself — a map key (`m set a 1` walks `a` as a bare
	// Word) and a branch-local `def` are both opaque and both harmless.
	var opaque string
	inspect := func(name string, v Value) bool {
		if bad := sharedMutableKind(v); bad != "" {
			offender, kind = name, bad
			return true
		}
		return false
	}
	// A branch body arrives in one of two shapes, and checking only the
	// obvious one is how this check first shipped inert: interpreted, the
	// element IS the token list; COMPILED, it is a synthetic fn-value
	// carrier holding the tokens in its BoruImpl body (the
	// CompileStoresBodyList spawn pattern runParallelBranch unwraps). Miss
	// the second and the boundary is unguarded on the default path — the
	// only path most programs take.
	var work [][]Value
	for _, elem := range elems {
		if toks := branchTokens(elem); len(toks) > 0 {
			work = append(work, toks)
		}
	}
	// Referenced FUNCTIONS are followed into, because both ways a lambda
	// can reach a container cross the boundary just as surely as a
	// directly-named one — and they are different mechanisms:
	//
	//   - CAPTURED, when the container is a binding of an enclosing fn.
	//     It travels inside the Function value.
	//   - DYNAMIC, when the container is module-level. Nothing is
	//     captured (CLAUDE.md "What gets captured": a module-level def
	//     stays dynamic), so it is reachable ONLY through the body's
	//     word references — which is exactly the shape `def w ([] => [m
	//     set a 1])` takes at the top level, the one a user is most
	//     likely to write.
	//
	// `seen` bounds the walk, so a recursive or mutually-recursive fn
	// terminates rather than looping.
	for len(work) > 0 && offender == "" {
		tokens := work[0]
		work = work[1:]
		WalkBodyWords(tokens, func(w WordInfo, _ Value) {
			if offender != "" || seen[w.Name] {
				return
			}
			seen[w.Name] = true
			bound, ok := r.Defs.Top(w.Name)
			if !ok {
				if branchRefIsOpaque(w.Name) {
					opaque = w.Name
				}
				return
			}
			if inspect(w.Name, bound) {
				return
			}
			if fd, isFn := bound.Data.(FnDefInfo); isFn {
				for _, c := range fd.Captured {
					if inspect(c.Name, c.Value) {
						return
					}
				}
				for i := range fd.Signatures {
					if a, isBoru := fd.Signatures[i].Impl.(*core.BoruImpl); isBoru {
						work = append(work, a.Body)
					}
				}
			}
		})
	}
	if offender != "" {
		return offender, kind
	}
	// Nothing was reached DIRECTLY, but a reference went opaque. The check
	// cannot bound what that reference reaches, so fall back to the only
	// sound question left: is there a mutable container in scope at all?
	//
	// This is what closes the compiled fn-local hole. `def w ([] => [m set
	// a 1])` inside a fn body compiles `w` into a frame slot, so the walk
	// dead-ends at `w` — while `m` sits in r.Defs the whole time, one step
	// out of reach. Two branches then wrote the same OrderedMap
	// concurrently on the DEFAULT path: a real data race, not a
	// theoretical one.
	//
	// It is deliberately conditional on something mutable existing. An
	// unconditional "opaque reference => refuse" would reject the two
	// shapes the docs promote — `[m set a 1]` over an IMMUTABLE map (the
	// key `a` walks as a bare Word) and `[def a (make FlexMap {}) a set k
	// 1]` (branch-local, private by construction) — because a map key and
	// a branch-local binding are indistinguishable from a fn-local
	// function here.
	//
	// The residual imprecision is stated rather than hidden: a mutable
	// container in scope that the opaque reference could not actually have
	// reached is refused anyway. That is the conservative direction, and
	// it is the direction this boundary already chose over cloning.
	if opaque != "" {
		if name, bad := scopeSharedMutable(r); name != "" {
			return name, bad
		}
	}
	return "", ""
}

// makeBranchForks builds one isolated registry per branch so the
// branches can run concurrently without racing on the shared registry's
// mutable stacks. A single SyncWriter is shared across the forks so
// concurrent prints to the parent's output are serialized rather than
// interleaved or raced. Forking happens here, on the dispatching
// goroutine, while the parent is not yet parked — ForkConcurrent's
// reads of the parent state are therefore race-free.
//
// It also enforces the branch boundary: a reachable mutable container is
// REFUSED rather than shared or silently copied. Refusing is what makes
// the documented guarantee ("writes to mutable objects inside one branch
// do not bleed into the others") true and checkable, and it is the answer
// `send` already gives at the process boundary. Deep-copying instead
// would make the same programs work silently, but it hides the sharing
// the author wrote and pays a per-branch copy for every container in
// scope.
func makeBranchForks(r *Registry, elems []Value) ([]*Registry, error) {
	if name, kind := branchSharingViolation(r, elems); name != "" {
		return nil, r.BoruErrorHint("not_sendable",
			fmt.Sprintf("await: branch reaches `%s`, a mutable %s — a stateful "+
				"container cannot be shared across concurrent branches", name, kind),
			"await",
			"branches run on separate goroutines, so an in-place write to a "+
				"shared container is a data race; pass an immutable List/Map, or "+
				"build the container inside each branch and combine the results")
	}
	out := NewSyncWriter(r.Output)
	errOut := NewSyncWriter(r.ErrOutput)
	forks := make([]*Registry, len(elems))
	for i := range forks {
		f := r.ForkConcurrent()
		f.Output = out
		f.ErrOutput = errOut
		forks[i] = f
	}
	return forks, nil
}

// runParallelBranch executes one element with do semantics on its own
// isolated registry fork. A COMPILED branch body arrives as a synthetic
// fn-value carrier with a CompiledFnRef (CompileStoresBodyList — the spawn
// pattern) and runs via RunUnit on the fork; a raw list runs as an
// interpreter sub-program; anything else is returned as a single value.
func runParallelBranch(reg *Registry, elem Value) parallelResult {
	if fd, ok := elem.Data.(core.FnDefInfo); ok {
		for i := range fd.Signatures {
			ref := compiler.CompiledRef(&fd.Signatures[i])
			if ref == nil || ref.Prog == nil {
				continue
			}
			effectsAt := reg.Effects.Count()
			result, runErr := eng.RunUnit(ref, reg, nil)
			if core.IsInternalError(runErr) && reg.Effects.Count() == effectsAt {
				// A VM soundness bail with NO observable effect: re-run the raw
				// tokens on the interpreter, exactly as the branch would have run
				// without the stamp (the C1 fence — see InvokeCallback).
				if a, isBoru := fd.Signatures[i].Impl.(*core.BoruImpl); isBoru {
					return interpretBranchBody(reg, a.Body)
				}
			}
			return branchOutcome(result, runErr)
		}
	}
	if elem.Parent.ConformsTo(TList) && elem.Data != nil && !IsTypedList(elem) && !IsTableType(elem) {
		_lst, _ := AsList(elem)
		return interpretBranchBody(reg, _lst.Slice())
	}
	// Non-list element: just return it as-is.
	return parallelResult{values: []Value{elem}}
}

// interpretBranchBody runs a branch's raw token list on a fresh interpreter
// sub-engine over the fork — the pre-stamping branch path, byte-identical.
//
// An EMPTY body short-circuits. Zero tokens is zero work: the sub-engine would
// run no step and return an empty stack, so the outcome is empty BY
// CONSTRUCTION and the only thing the run produces is an interpreter entry
// inside an otherwise compiled program, which the interp-entry census counts
// as debt (`await {mode:'first'} [[]]` is the row). The compile side cannot
// remove it either — compileStoredBody declines an empty token list, so an
// empty branch is the one shape that reaches here with nothing to do.
func interpretBranchBody(reg *Registry, body []Value) parallelResult {
	if len(body) == 0 {
		return branchOutcome(nil, nil)
	}
	sub := New(reg)
	input := make([]Value, len(body))
	copy(input, body)
	result, runErr := sub.Run(input)
	return branchOutcome(result, runErr)
}

// branchOutcome maps a branch run's (result, error) to a parallelResult —
// one shared mapping so the compiled and interpreted paths agree exactly.
func branchOutcome(result []Value, runErr error) parallelResult {
	if runErr != nil {
		return parallelResult{values: []Value{NewError(runErr)}, err: true}
	}
	if len(result) == 1 && IsError(result[0]) {
		return parallelResult{values: result, err: true}
	}
	return parallelResult{values: result}
}

// awaitAll waits for all branches to succeed. Returns the first error if any reject.
func awaitAll(r *Registry, elems []Value) ([]Value, error) {
	results := make([]parallelResult, len(elems))
	forks, ferr := makeBranchForks(r, elems)
	if ferr != nil {
		return nil, ferr
	}
	var wg sync.WaitGroup
	wg.Add(len(elems))

	for i, elem := range elems {
		go func(idx int, e Value, reg *Registry) {
			defer wg.Done()
			pr := runParallelBranch(reg, e)
			pr.index = idx
			results[idx] = pr
		}(i, elem, forks[i])
	}
	wg.Wait()

	// If any rejected, return the first error.
	for _, pr := range results {
		if pr.err {
			return pr.values, nil
		}
	}

	// Collect all results into a list. Each branch's result is unwrapped
	// if it produced exactly one value.
	out := make([]Value, len(results))
	for i, pr := range results {
		if len(pr.values) == 1 {
			out[i] = pr.values[0]
		} else {
			out[i] = NewList(pr.values)
		}
	}
	return []Value{NewList(out)}, nil
}

// awaitFull waits for all branches to complete and returns a list of
// {status:'ok, value:...} or {status:'error, value:...} maps.
func awaitFull(r *Registry, elems []Value) ([]Value, error) {
	results := make([]parallelResult, len(elems))
	forks, ferr := makeBranchForks(r, elems)
	if ferr != nil {
		return nil, ferr
	}
	var wg sync.WaitGroup
	wg.Add(len(elems))

	for i, elem := range elems {
		go func(idx int, e Value, reg *Registry) {
			defer wg.Done()
			pr := runParallelBranch(reg, e)
			pr.index = idx
			results[idx] = pr
		}(i, elem, forks[i])
	}
	wg.Wait()

	out := make([]Value, len(results))
	for i, pr := range results {
		m := NewOrderedMap()
		if pr.err {
			m.Set("status", NewAtom("error"))
		} else {
			m.Set("status", NewAtom("ok"))
		}
		if len(pr.values) == 1 {
			m.Set("value", pr.values[0])
		} else {
			m.Set("value", NewList(pr.values))
		}
		out[i] = NewMap(m)
	}
	return []Value{NewList(out)}, nil
}

// awaitFirst returns the result of whichever branch finishes first.
func awaitFirst(r *Registry, elems []Value) ([]Value, error) {
	forks, ferr := makeBranchForks(r, elems)
	if ferr != nil {
		return nil, ferr
	}
	ch := make(chan parallelResult, len(elems))
	for i, elem := range elems {
		go func(idx int, e Value, reg *Registry) {
			pr := runParallelBranch(reg, e)
			pr.index = idx
			ch <- pr
		}(i, elem, forks[i])
	}
	first := <-ch
	return first.values, nil
}

// awaitAny returns the first successful result. If all reject, returns
// the last error.
func awaitAny(r *Registry, elems []Value) ([]Value, error) {
	type indexedResult struct {
		pr parallelResult
	}

	forks, ferr := makeBranchForks(r, elems)
	if ferr != nil {
		return nil, ferr
	}
	ch := make(chan indexedResult, len(elems))
	for i, elem := range elems {
		go func(idx int, e Value, reg *Registry) {
			pr := runParallelBranch(reg, e)
			pr.index = idx
			ch <- indexedResult{pr: pr}
		}(i, elem, forks[i])
	}

	var lastErr parallelResult
	errCount := 0
	for range elems {
		ir := <-ch
		if !ir.pr.err {
			return ir.pr.values, nil
		}
		lastErr = ir.pr
		errCount++
	}
	// All rejected — return the last error.
	return lastErr.values, nil
}

// awaitRunner selects doAwait's per-mode implementation, or nil when the
// mode has no arm. The handler and the check-time mirror share this ONE
// table, so a mode can never be known to one and unknown to the other.
func awaitRunner(mode string) func(*Registry, []Value) ([]Value, error) {
	switch mode {
	case "all":
		return awaitAll
	case "full":
		return awaitFull
	case "first":
		return awaitFirst
	case "any":
		return awaitAny
	}
	return nil
}

// awaitUnknownMode is doAwait's unknown-mode raise. The mirror reports it
// through this same constructor, so the check diagnostic and the runtime
// error cannot drift in code or in detail.
func awaitUnknownMode(r *Registry, mode string) error {
	return r.BoruError("await_error",
		fmt.Sprintf("await: unknown mode %q, expected all, full, first, or any", mode), "await")
}

// awaitParallelsEmpty is doAwait's own emptiness test, and its own leniency:
// doAwait reads `_lst.Slice()` and branches on len alone, so a NIL payload
// counts as empty exactly like a zero-length one.
//
// ONE predicate, shared by the mirror and the result model, because two
// copies of it already drifted: the model was written with an extra
// `!IsNil()` guard, a direct test caught it there, and the identical guard
// three lines above in the mirror survived the fix (PR #351 review, Codex
// P2). In the mirror the drift is worse than imprecision — a nil-backed
// empty list with an unknown mode would be flagged error-severity while
// doAwait returns the empty List and the program runs.
func awaitParallelsEmpty(parallels Value) bool {
	lst, err := AsList(parallels)
	return err == nil && len(lst.Slice()) == 0
}

// awaitModeGate accepts a call only when BOTH operands the validator reads
// are concrete. DeepConcreteOptions' arg0-only contract does not hold here:
// the validator reads the parallels list too, for the empty short-circuit.
// Position 1 is checked SHALLOW on purpose — the validator asks it for
// Len() alone, and the elements are unevaluated code bodies that deep
// concreteness would refuse for no reason.
func awaitModeGate(args []Value) bool {
	return len(args) == 2 && DeepConcreteOptionsAt(0)(args) && IsConcrete(args[1])
}

// awaitModeMirror flags an unknown {mode:} at check time.
//
// The walk is doAwait's, and the ORDER is the whole point: doAwait returns
// the empty List for an empty parallels list BEFORE it ever selects a
// runner, so `await {mode:'bogus'} []` runs clean. A mirror that validated
// the mode first would flag that program — the handler-order rule on a word
// where a short-circuit sits between the entry and the raise being mirrored.
func awaitModeMirror() ReturnsFunc {
	return MirrorReturns("await", awaitModeGate,
		func(args []Value, r *Registry) error {
			// awaitModeGate has already required a concrete operand at 1.
			if awaitParallelsEmpty(args[1]) {
				return nil
			}
			mode := awaitOptsMode(args[0])
			if awaitRunner(mode) == nil {
				return awaitUnknownMode(r, mode)
			}
			return nil
		},
		awaitOptsReturns)
}

// awaitOptsMode reads the `mode` option out of await's options argument.
// awaitWithOptsHandler and the check model share this ONE reader, so the
// model can never resolve a different mode than the run does.
func awaitOptsMode(opts Value) string {
	mode := "all"
	if oi, err := AsOptionsType(opts); err == nil {
		if v, ok := oi.Fields.Get("mode"); ok {
			mode = awaitModeName(v, mode)
		}
		return mode
	}
	if optsMap, _ := AsMap(opts); optsMap != nil {
		if v, ok := optsMap.Get("mode"); ok {
			mode = awaitModeName(v, mode)
		}
	}
	return mode
}

// awaitModeName reads a mode field spelled either way — `{mode:'full'}` or
// `{mode:full/q}` — and keeps def when it is neither String nor Atom, so
// `{mode:5}` runs the DEFAULT mode rather than reaching doAwait's
// unknown-mode raise. Preserved verbatim from the handler: the check model
// has to inherit that quirk, not correct it.
func awaitModeName(v Value, def string) string {
	if s, err := AsString(v); err == nil {
		return s
	}
	if a, err := AsAtom(v); err == nil {
		return a
	}
	return def
}

// awaitOptsReturns models the 2-arg form's residual. The mode has to be
// READ before it can select a shape, so a non-concrete options argument
// resolves nothing knowable — and "nothing knowable" includes the ARITY:
// a runtime `first`/`any` mode is reachable, so the model is the variadic
// spread, not a one-seat Any (NUR067).
func awaitOptsReturns(args []Value, r *Registry) []Value {
	if len(args) < 2 || !IsConcrete(args[0]) {
		return awaitVariadicResult(r)
	}
	return awaitResidual(r, awaitOptsMode(args[0]), args[1])
}

// awaitDefaultReturns models the 1-arg form, which pins mode to "all" —
// arity 1 on every path, so the variadic machinery is never armed here.
func awaitDefaultReturns(args []Value, r *Registry) []Value {
	if len(args) < 1 {
		return []Value{NewCarrier(TAny)}
	}
	return awaitResidual(r, "all", args[0])
}

// awaitVariadicResult models a winner-takes-all residual — `first` / `any`
// hand back the winning branch's WHOLE residual, any count including zero
// (`await {mode:'first'} [[]]` nets nothing; a 3-value branch nets three).
//
// ONE model, both passes: the variadic-spread carrier, so the checked arity
// is honest in both directions (the soundness oracle absorbs 0-or-more
// entries). The element is the deliberate Any: the branches are unevaluated
// code bodies run on isolated forks, so the winning residual's types
// genuinely cannot be bounded here — see the SpreadPayload contract note.
//
// The COMPILE pass used to refuse the whole program here, because the
// runtime count can EXCEED any modeled seat and the emitter's only variadic
// device was the L-DO catch mark, which covers just the SHRINKING direction
// (`do`'s N no-raise vs 1 caught). A 1-seat event that delivers three values
// stranded two: `size [(await {mode:'any'} [[7 8]])]` compiled to a stranded
// 7 and a 1-element list where the interpreter answers 2 (NUR067's
// miscompile).
//
// GRADUATED 2026-09-10: the recorder reads this very carrier out of the
// dispatch's residual (compiler's callVariadicRegion) and records the event
// as a runtime-variadic REGION — one simulated slot for the whole run, the
// representation a value-producing LOOP already uses, marked lw.variadic at
// lowering. So the growing direction has a home, and what refuses is each
// position that genuinely needs a static count: a call/list operand, a frame
// promotion, the dead-result drop. There is nothing pass-specific left to
// say here, which is why this function no longer takes a branch.
func awaitVariadicResult(r *Registry) []Value {
	return []Value{core.NewVariadicCarrier(NewTypeLiteral(TAny))}
}

// awaitResidual walks doAwait's own statement order: a non-concrete
// parallels list raises before any mode is consulted, an EMPTY list returns
// the empty List ahead of the mode switch, and only then does the mode
// select a shape.
//
// Two of the four modes have a knowable residual:
//
//   - full — awaitFull has no error escape. Every branch, rejected or not,
//     becomes a {status value} map in the result list, so the result is a
//     List for every input, and a list of MAPS specifically: awaitFull's
//     only write into the result is `out[i] = NewMap(m)`. Typed, so a
//     `(await {mode:'full'} […]) get 0` reads a Map rather than an Any.
//   - all — awaitAll builds a List, EXCEPT that the first rejecting branch
//     short-circuits to that branch's single Error value (branchOutcome's
//     err arms both carry exactly one). Both arms are reachable, so the
//     residual is genuinely the union.
//
// The `all` union is DYNAMIC, and the reason is worth stating because the
// strict form was tried first and is wrong. A strict disjunct DISTRIBUTES
// over dispatch: every alternative must dispatch or the call is refused, so
// `(await […]) get 0` — a program that runs fine — collects two
// partial_dispatch warnings and a hard no_signature. That is a false
// positive bought with precision nobody asked for. The dynamic form matches
// optimistically (no refusal) while still naming both arms, so a downstream
// Error accessor dispatches its Error overload instead of no_signature-ing
// on a bare List. It is `do`'s scalar-tor-Error shape, for `do`'s reason.
//
// What the dynamic form costs: typeCovered (check_accuracy_test.go) opens
// with `if checked.Dynamic { return true }`, so the soundness gate cannot
// falsify this claim. It is justified by READING awaitAll's two return
// statements, not by the gate agreeing with it — the gate is silent here,
// which is not the same as the gate assenting.
//
// first and any hand back the WINNING BRANCH's whole residual — any type at
// any arity, including none (`await {mode:'first'} [[]]` returns nothing at
// run time, and a 3-value branch body returns three) — so their model is
// the VARIADIC result (awaitVariadicResult): one variadic-spread carrier,
// the same on both passes since NUR067's graduation, which the compile pass
// records as a runtime-variadic region.
//
// The two variadic arms fire for a NON-CONCRETE parallels list too: the
// runtime list could be empty (one List) or not (the winner's residual), so
// only the variadic claim covers both.
func awaitResidual(r *Registry, mode string, parallels Value) []Value {
	if mode == "first" || mode == "any" {
		if IsConcrete(parallels) && awaitParallelsEmpty(parallels) {
			return []Value{NewCarrier(TList)}
		}
		return awaitVariadicResult(r)
	}
	if !IsConcrete(parallels) {
		return []Value{NewCarrier(TAny)}
	}
	if awaitParallelsEmpty(parallels) {
		return []Value{NewCarrier(TList)}
	}
	switch mode {
	case "full":
		return []Value{NewCarrierTypedList(TMap)}
	case "all":
		return []Value{NewDynamicCarrierValue(
			NewDisjunct([]Value{NewTypeLiteral(TList), NewTypeLiteral(TError)}))}
	}
	return []Value{NewCarrier(TAny)}
}
