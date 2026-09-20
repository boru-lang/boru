package modules

import (
	"fmt"
	mathrand "math/rand"
	"sync"

	"github.com/boru-lang/boru/lang/go/native"
)

// randState holds a PRNG instance. Each instance is independent —
// successive calls on the same state share the stream, but two
// distinct states (e.g. top-level default vs `Rand.with-seed N`) are
// fully isolated.
type randState struct {
	mu  sync.Mutex
	rng *mathrand.Rand
}

// BuildRandModule creates the "boru:rand" native module.
//
// The top-level `rand` namespace is **non-deterministic by default**:
// at module-build time we seed once from the host clock so a fresh
// `import "boru:rand"` produces genuinely random values.
//
// For deterministic / reproducible sequences (property tests, demo
// fixtures, replayable simulations) use `Rand.with-seed N` — it
// returns a fresh isolated instance (an OrderedMap) carrying the same
// methods as the top-level (`int`, `bool`, `float`, `string`,
// `one-of`). The instance has its own PRNG sourced from `N` and does
// not affect the top-level rand or any other instance.
//
//	import "boru:rand"
//	Rand.int 0 100              # random, [0, 100)
//	def r (Rand.with-seed 42)   # isolated, seeded with 42
//	r.int 0 100                 # deterministic at seed 42
func BuildRandModule(parent *native.Registry) (native.ModuleDesc, error) {
	// Seed the top-level instance from the clock so default usage is
	// non-deterministic — what most developers expect.
	defaultState := newRandState(native.EffectiveClock(parent).Now().UnixNano())
	exports, err := buildRandExportsForState(defaultState)
	if err != nil {
		return native.ModuleDesc{}, err
	}

	// `Rand.with-seed` lives only at the top level. Its handler
	// constructs a new randState seeded with N, builds a separate
	// exports map with all the standard methods (int, bool, float,
	// string, one-of), and returns that map as an OrderedMap. Each
	// call mints a fresh instance — no global mutation.
	withSeedSubReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	withSeedSubReg.RegisterNativeFunc(native.NativeFunc{
		Name: "rand-with-seed",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TInteger},
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
			// Check-mode shape: a Rand instance's METHOD TABLE is static (the
			// same wrapper FnDefs for every seed -- only the captured RNG state
			// differs). Surface a concrete Map of those wrappers so a downstream
			// `r get list-of` resolves the closure-bearing wrapper and dispatches
			// the SAME closure-word path as the `Rand.list-of` module-export form
			// (getNodeReturns -> isClosureBearingWrapper). The wrappers ride a
			// throwaway shape-only sub-registry; the only handler the compiler
			// can reach through them (rand-list-of) is a closure DRIVER that does
			// not itself draw from the RNG, so the resolved shape is seed-agnostic
			// and the runtime answer is unchanged. RNG-bound methods (int/bool/...)
			// stay dynamic at the field read, so no seed-specific draw is baked.
			ReturnsFn: randWithSeedReturns,
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				seed, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				state := newRandState(seed)
				instance, err := buildRandExportsForState(state)
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewMap(instance)}, nil
			}),
		}},
	})
	exports.Set("with-seed", wrapRandFnDef("rand-with-seed",
		[]native.FnParam{{Type: native.TInteger}},
		[]*native.Type{native.TMap}, withSeedSubReg))

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"Rand": exports},
	}, nil
}

// randWithSeedReturns is the check-mode shape for `Rand.with-seed N`. The
// handler does not run during static analysis, so the result would otherwise be
// a shapeless Map carrier and a downstream `r get list-of` could not resolve the
// closure-bearing wrapper. This returns a CONCRETE Map of the instance's method
// wrappers (built from a fixed shape-only state) so the field read resolves the
// wrapper and the dispatch records the closure-word path -- identical to the
// `Rand.list-of` module-export form. The state here is used ONLY for its method
// SHAPE; the sole compiler-reachable wrapper (list-of) is an RNG-independent
// closure driver, so no seed-specific RNG behaviour is baked.
func randWithSeedReturns(_ []native.Value, _ *native.Registry) []native.Value {
	instance, err := buildRandExportsForState(newRandState(0))
	if err != nil {
		// Fall back to the bare Map carrier on the (registration-time
		// impossible) build error; the row simply declines as before.
		return []native.Value{native.NewCarrier(native.TMap)}
	}
	return []native.Value{native.NewMap(instance)}
}

// newRandState builds a fresh PRNG seeded with the given int64.
func newRandState(seed int64) *randState {
	return &randState{rng: mathrand.New(mathrand.NewSource(seed))}
}

// BuildSeededRandInstance constructs a fresh rand instance with the
// given seed. Returns an OrderedMap with the same methods as the
// top-level `rand` namespace (`int`, `bool`, `float`, `string`,
// `one-of`), each closing over a private PRNG seeded with `seed`.
//
// Exposed so other modules (notably boru:test's PBT framework) can
// build deterministic rand instances without going through boru-level
// `Rand.with-seed N` invocation. The returned Map is functionally
// identical to what `Rand.with-seed N` produces from Boru.
func BuildSeededRandInstance(seed int64) (*native.OrderedMap, error) {
	state := newRandState(seed)
	return buildRandExportsForState(state)
}

// BuildSeededRandRegistry returns a fresh registry whose top-level
// word table includes every rand-* native bound to a private PRNG
// seeded with `seed`. Used by the PBT shrinker to evaluate
// generator-program StackForms — those forms contain Call ops by
// the inner native names (`rand-int`, `rand-bool`, etc.), which a
// caller-side registry without rand installed wouldn't dispatch.
//
// The returned registry also carries the default kernel native set
// (math / comparison / stack ops / etc.) since it's built from
// native.DefaultRegistry; complex gen bodies that mix rand with
// arithmetic replay cleanly.
func BuildSeededRandRegistry(seed int64) (*native.Registry, error) {
	r, err := newDefaultRegistry()
	if err != nil {
		return nil, err
	}
	state := newRandState(seed)
	for _, n := range randNativesForState(state) {
		r.RegisterNativeFunc(n)
	}
	return r, nil
}

// buildRandExportsForState builds the OrderedMap of dotted methods
// (`int`, `bool`, `float`, `string`, `one-of`) bound to the given
// state. Used for both the top-level default and for each
// `Rand.with-seed` instance — each gets its own sub-registry of
// natives closing over its own randState.
func buildRandExportsForState(state *randState) (*native.OrderedMap, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return nil, err
	}
	for _, n := range randNativesForState(state) {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	// Wrapper FnSig Params match the inner Signature.Args order
	// (top-first per SIG-ORDER-REFACTOR.10.md). Aligned with the
	// FORWARD canonical surface — sig[0] is the first arg written
	// after the word: `Rand.int LO HI`, `Rand.string CHARSET LEN`.
	exports.Set("int", wrapRandFnDef("rand-int",
		[]native.FnParam{{Type: native.TInteger}, {Type: native.TInteger}},
		[]*native.Type{native.TInteger}, subReg))
	exports.Set("bool", wrapRandFnDef("rand-bool",
		nil,
		[]*native.Type{native.TBoolean}, subReg))
	exports.Set("float", wrapRandFnDef("rand-float",
		nil,
		[]*native.Type{native.TFloat}, subReg))
	exports.Set("string", wrapRandFnDef("rand-string",
		[]native.FnParam{{Type: native.TString}, {Type: native.TInteger}},
		[]*native.Type{native.TString}, subReg))
	exports.Set("one-of", wrapRandFnDef("rand-one-of",
		[]native.FnParam{{Type: native.TList}},
		[]*native.Type{native.TAny}, subReg))
	// list-of takes a quoted generator body — NoEvalArgs[0]=true on
	// both the wrapper FnSig and the inner native sig so the body
	// reaches the handler intact rather than being auto-evaluated
	// at either boundary.
	exports.Set("list-of", wrapRandFnDefNoEval("rand-list-of",
		[]native.FnParam{{Type: native.TList}, {Type: native.TInteger}},
		[]*native.Type{native.TList}, map[int]bool{0: true}, nil, subReg))
	// map-from takes a schema map whose values are quoted generators.
	// NoEvalMapArgs[0]=true so the map structure (and inner gen lists)
	// survive unchanged.
	exports.Set("map-from", wrapRandFnDefNoEval("rand-map-from",
		[]native.FnParam{{Type: native.TMap}},
		[]*native.Type{native.TMap}, nil, map[int]bool{0: true}, subReg))
	return exports, nil
}

// wrapRandFnDef builds the FnDef wrapper that dispatches a dotted
// rand.<word> call into the sub-registry's native handler (the shared
// makeWrapFnDef trivial-delegation shape).
func wrapRandFnDef(wordName string, params []native.FnParam, returns []*native.Type, subReg *native.Registry) native.Value {
	return wrapRandFnDefNoEval(wordName, params, returns, nil, nil, subReg)
}

// wrapRandFnDefNoEval is wrapRandFnDef plus NoEvalArgs / NoEvalMapArgs
// passthrough for wrappers whose params are quoted code bodies
// (Rand.list-of, Rand.map-from). Without these, execFnDefSig's
// auto-eval would silently sub-Run the bodies before the inner
// handler sees them.
func wrapRandFnDefNoEval(
	wordName string,
	params []native.FnParam,
	returns []*native.Type,
	noEval map[int]bool,
	noEvalMap map[int]bool,
	subReg *native.Registry,
) native.Value {
	return makeWrapFnDef(wordName, subReg, wrapSig{
		params:    params,
		returns:   returns,
		noEval:    noEval,
		noEvalMap: noEvalMap,
	})
}

// randNativesForState builds the Go-implemented rand primitives.
// Every handler closes over `state` directly, so each call mints a
// new set of natives bound to a specific PRNG instance. No global
// capability lookup — the state pointer is captured at construction.
func randNativesForState(state *randState) []native.NativeFunc {
	return []native.NativeFunc{
		{
			Name: "rand-int",
			Signatures: []native.Signature{{
				// Canonical surface (forward form): `Rand.int LO HI`.
				// sig[0]=lo, sig[1]=hi. Returns a uniform integer in
				// the HALF-OPEN range [lo, hi) — inclusive lower,
				// exclusive upper. Matches Python's random.randrange,
				// Rust's gen_range, Go's rand.Intn.
				Args:       []*native.Type{native.TInteger, native.TInteger},
				Returns:    []*native.Type{native.TInteger},
				BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					lo, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					hi, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					if hi <= lo {
						return nil, r.BoruError("rand_error",
							fmt.Sprintf("Rand.int: hi (%d) <= lo (%d); range must be non-empty", hi, lo),
							"Rand.int")
					}
					state.mu.Lock()
					n := lo + state.rng.Int63n(hi-lo)
					state.mu.Unlock()
					return []native.Value{native.NewInteger(n)}, nil
				}),
			}},
		},
		{
			Name: "rand-bool",
			Signatures: []native.Signature{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TBoolean},
				BarrierPos: -1,
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					state.mu.Lock()
					b := state.rng.Intn(2) == 1
					state.mu.Unlock()
					return []native.Value{native.NewBoolean(b)}, nil
				}),
			}},
		},
		{
			Name: "rand-float",
			Signatures: []native.Signature{{
				// Returns a uniform decimal in [0.0, 1.0).
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TFloat},
				BarrierPos: -1,
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					state.mu.Lock()
					f := state.rng.Float64()
					state.mu.Unlock()
					return []native.Value{native.NewFloat(f)}, nil
				}),
			}},
		},
		{
			Name: "rand-string",
			Signatures: []native.Signature{{
				// Canonical surface (forward form):
				// `Rand.string CHARSET LENGTH`. sig[0]=charset (String),
				// sig[1]=length (Integer).
				Args:       []*native.Type{native.TString, native.TInteger},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					charset, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					length, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					if length < 0 {
						return nil, r.BoruError("rand_error",
							fmt.Sprintf("Rand.string: length (%d) < 0", length), "Rand.string")
					}
					runes := []rune(charset)
					if len(runes) == 0 {
						if length == 0 {
							return []native.Value{native.NewString("")}, nil
						}
						return nil, r.BoruError("rand_error",
							"Rand.string: empty charset", "Rand.string")
					}
					out := make([]rune, length)
					state.mu.Lock()
					for i := range out {
						out[i] = runes[state.rng.Intn(len(runes))]
					}
					state.mu.Unlock()
					return []native.Value{native.NewString(string(out))}, nil
				}),
			}},
		},
		{
			// Run `body` `n` times, collecting each iteration's
			// top-of-stack into a List. body is a quoted code block
			// (NoEvalArgs[0]=true) — typically uses `r` or rand.*
			// to produce a single value per iteration.
			Name: "rand-list-of",
			// A 0-input generator body run n times — the same closure shape as
			// `do`, so the recorder compiles `[body]` to a closure unit and the
			// handler runs it via the VM seam instead of a sub-engine (the body's
			// RNG draws advance the same module generator either way).
			Callable: &native.CallableSpec{BodyPos: 0, BodyOut: 1, BodyResultTop: true, Inputs: func(_ []native.Value) []native.Value {
				return []native.Value{}
			}},
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TList, native.TInteger},
				Returns:    []*native.Type{native.TList},
				NoEvalArgs: map[int]bool{0: true},
				BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					n, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					if n < 0 {
						return nil, r.BoruError("rand_error",
							fmt.Sprintf("Rand.list-of: length (%d) < 0", n), "Rand.list-of")
					}
					out := make([]native.Value, 0, n)
					// Compiled path: the body arrived as a compiled CLOSURE; run it
					// n times through the VM's re-entrant runner (InvokeBody).
					if native.IsCompiledClosure(args[0]) {
						for i := int64(0); i < n; i++ {
							res, err := native.InvokeBody(r, args[0], nil)
							if err != nil {
								return nil, fmt.Errorf("evaluating Rand.list-of[%d]: %w", i, err)
							}
							if len(res) == 0 {
								return nil, r.BoruError("rand_error",
									fmt.Sprintf("Rand.list-of[%d]: body produced no value", i), "Rand.list-of")
							}
							out = append(out, res[len(res)-1])
						}
						return []native.Value{native.NewList(out)}, nil
					}
					// Interpreter path: run the quoted token body in a sub-engine.
					body, err := native.RequireConcreteList(args[0], "Rand.list-of body")
					if err != nil {
						return nil, err
					}
					bodyTokens := body.Slice()
					// A STEPLESS body is its own residual — the engine would place
					// its values and hand them straight back — so N iterations of
					// `Rand.list-of ['a','b'] 2` do not need N sub-engines to
					// discover that. Same rule as `do {key:[body]}` and the mixed
					// apply's window, at the site that owns this loop.
					if native.IsSteplessWindow(bodyTokens) {
						// n == 0 runs the body ZERO times, so an empty body is not
						// an error there — the loop below never reaches its
						// no-value check either. Measured: guarding on the body
						// alone made `Rand.list-of [] 0` raise where both engines
						// answer [].
						if n > 0 && len(bodyTokens) == 0 {
							return nil, r.BoruError("rand_error",
								"Rand.list-of[0]: body produced no value", "Rand.list-of")
						}
						for i := int64(0); i < n; i++ {
							out = append(out, bodyTokens[len(bodyTokens)-1])
						}
						return []native.Value{native.NewList(out)}, nil
					}
					for i := int64(0); i < n; i++ {
						res, err := native.RunPooled(r, append([]native.Value(nil), bodyTokens...))
						if err != nil {
							return nil, fmt.Errorf("evaluating Rand.list-of[%d]: %w", i, err)
						}
						if len(res) == 0 {
							return nil, r.BoruError("rand_error",
								fmt.Sprintf("Rand.list-of[%d]: body produced no value", i),
								"Rand.list-of")
						}
						out = append(out, res[len(res)-1])
					}
					return []native.Value{native.NewList(out)}, nil
				}),
			}},
		},
		{
			// Build a Map by running each key's quoted body. Schema is
			// a Map whose values are quoted code blocks; the result
			// has the same keys with each body's top-of-stack as the
			// corresponding value.
			Name: "rand-map-from",
			Signatures: []native.Signature{{
				Args:          []*native.Type{native.TMap},
				Returns:       []*native.Type{native.TMap},
				NoEvalMapArgs: map[int]bool{0: true},
				BarrierPos:    -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					schema, err := native.RequireConcreteMap(args[0], "Rand.map-from schema")
					if err != nil {
						return nil, err
					}
					out := native.NewOrderedMap()
					for _, key := range schema.Keys() {
						bodyVal, _ := schema.Get(key)
						body, err := native.RequireConcreteList(bodyVal, "Rand.map-from value")
						if err != nil {
							return nil, fmt.Errorf("evaluating Rand.map-from[%s]: %w", key, err)
						}
						res, err := native.RunPooled(r, append([]native.Value(nil), body.Slice()...))
						if err != nil {
							return nil, fmt.Errorf("evaluating Rand.map-from[%s]: %w", key, err)
						}
						if len(res) == 0 {
							return nil, r.BoruError("rand_error",
								fmt.Sprintf("Rand.map-from[%s]: body produced no value", key),
								"Rand.map-from")
						}
						out.Set(key, res[len(res)-1])
					}
					return []native.Value{native.NewMap(out)}, nil
				}),
			}},
		},
		{
			Name: "rand-one-of",
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TList},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					lst, err := native.RequireConcreteList(args[0], "Rand.one-of")
					if err != nil {
						return nil, err
					}
					n := lst.Len()
					if n == 0 {
						return nil, r.BoruError("rand_error",
							"Rand.one-of: empty list", "Rand.one-of")
					}
					state.mu.Lock()
					idx := state.rng.Intn(n)
					state.mu.Unlock()
					return []native.Value{lst.Get(idx)}, nil
				}),
			}},
		},
	}
}
