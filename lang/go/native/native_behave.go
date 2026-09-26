package native

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	core "github.com/boru-lang/boru/core/go"
)

// behave BEHAVIOR FN
//
// Installs a user-defined capability on a type. The first arg is the
// behavior name (Atom — `compare/q`, `canon/q`, `truthy/q`, …); the
// second is a Function whose sig declares the target type and shape:
//
//	behave compare/q (fn [[Foo Foo] [Integer] [body]])
//	behave canon/q   (fn [[Foo]     [String]  [body]])
//	behave nodify/q  (fn [[Foo]     [Any]     [body]])
//	behave unify/q   (fn [[Foo Foo] [Foo]     [body]])
//	behave truthy/q  (fn [[Foo]     [Boolean] [body]])
//	behave deq/q     (fn [[Foo Foo] [Boolean] [body]])
//	behave size/q    (fn [[Foo]     [Integer] [body]])
//	behave make/q    (fn [[Any]     [Foo]     [body]])
//
// The handler validates the fn's first sig against the behavior's
// declared shape, extracts the target type from the input params,
// looks the type up in the registry, and wraps its TypeBehavior so
// the body runs whenever the kernel dispatches the corresponding
// capability (CompareValues for compare, Value.String for canon,
// NodifyValue for nodify, Unify for unify, CoerceBoolean for truthy,
// DeepEqual for deq, SizeOf for size, TryMake for make).
//
// `make` is the one slot whose target comes from the fn's RETURN type
// rather than its params, because construction has no receiver — only a
// target type and an arbitrary source. See validateMakeSig.
//
// Calling `behave` again on the same type with the same or a
// different behavior is additive — the existing userBehavior wrapper
// accepts new capability slots without losing previously installed
// ones.
var behaveNative = NativeFunc{
	Name: "behave",

	Signatures: []Signature{
		// behave STORES its fn for later invocation through the type's
		// Behavior wrapper (never re-stepped on the VM tape) — the store-fn
		// pattern log/patrun/service already carry, so a capture-free fn
		// operand bakes as an inert const instead of declining "function-
		// valued operand" (probe-verified: `behave "compare" (… /v)`).
		{
			Args:      []*Type{TAtom, TFunction},
			QuoteArgs: map[int]bool{0: true},
			Impl:      Go(behaveHandler),
			ReturnsFn: behaveReturns,
			Returns:   []*Type{}, BarrierPos:

			// String form for the behavior name (`behave "compare" fn […]`).
			-1,
			// The quoted behaviour NAME is inert data the handler reads
			// verbatim (a table key: `canon`, `compare`), so the atom form
			// bakes as a plain CALL_NATIVE once the fn operand is inert too
			// (S2-line, 2026-09-25): the VM runs the same handler over the
			// same baked atom and fn, installing the behaviour on the
			// run-time registry's type exactly as the interpreter does.
			CompileEffect: CompileStoresFn | CompileQuoteInert,
		},

		{
			Args:      []*Type{TString, TFunction},
			Impl:      Go(behaveHandler),
			ReturnsFn: behaveReturns,
			Returns:   []*Type{}, BarrierPos: -1,
			CompileEffect: CompileStoresFn,
		},
	},
}

// nodify VALUE
//
// Project VALUE into a Node or Scalar via its type's Nodifier
// capability — direct access to the data-shape produced by a
// `behave nodify/q` body without the JSON-string serialisation step.
// Useful when the caller wants the structural result for further boru
// processing rather than for wire output. With no nodify behavior
// registered for the type, the value passes through unchanged.
//
// The existing `jsonify` word composes this projection with
// voxgig/struct's JSON encoder; `nodify` exposes just the projection
// so tests and downstream pipelines can observe the Node/Scalar
// directly.
// nodifyHandler exposes voxgig/struct's Node/Scalar projection. `nodify`
// moved to the boru:struct module (see struct_module.go); the handler stays
// here next to its sibling `behave`.
func nodifyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	out, err := core.NodifyValue(args[0])
	if err != nil {
		return nil, err
	}
	return []Value{out}, nil
}

type behaviorEntry struct {
	// validate inspects the fn's first sig and returns the target
	// *Type the body should be attached to, or an error if the shape
	// doesn't match the behavior's contract.
	validate func(sig core.FnSig) (*core.Type, error)
	// install mutates the userBehavior wrapper to record the body
	// under the appropriate capability slot.
	install func(u *userBehavior, body []core.Value)
}

// behaviors is the kernel-known table of behavior names → wiring
// rules. The registry is closed: `behave unknown/q fn […]` errors
// out rather than silently installing a behavior the kernel doesn't
// dispatch. Plugins can extend the table at init time (not in this
// commit).
var behaviors = map[string]behaviorEntry{
	"compare": {
		validate: validateCompareSig,
		install:  func(u *userBehavior, body []core.Value) { u.compareBody = body },
	},
	"canon": {
		validate: validateCanonSig,
		install:  func(u *userBehavior, body []core.Value) { u.canonBody = body },
	},
	"nodify": {
		validate: validateNodifySig,
		install:  func(u *userBehavior, body []core.Value) { u.nodifyBody = body },
	},
	"unify": {
		validate: validateUnifySig,
		install:  func(u *userBehavior, body []core.Value) { u.unifyBody = body },
	},
	// The three slots below close the gap between what the kernel can
	// dispatch and what boru can install. `Sizer` already existed and was
	// reachable only from Go; `Truther` and `DeepEqualer` are new kernel
	// capabilities added for exactly this reason — a type could say how it
	// orders, renders, unifies and projects, but not whether it is empty
	// or when two of its values are the same.
	"truthy": {
		validate: validateTruthySig,
		install:  func(u *userBehavior, body []core.Value) { u.truthyBody = body },
	},
	"deq": {
		validate: validateDeqSig,
		install:  func(u *userBehavior, body []core.Value) { u.deqBody = body },
	},
	"size": {
		validate: validateSizeSig,
		install:  func(u *userBehavior, body []core.Value) { u.sizeBody = body },
	},
	// The eighth and last slot (NUR056). Construction was the one kernel
	// operation with no opt-in: a type could say what it MEANS in every
	// operation that consumed it and nothing about how it was BUILT,
	// while a Go-side Ideal could already customise construction through
	// Ideal.Instantiate — a Go-vs-boru asymmetry, not just a gap.
	"make": {
		validate: validateMakeSig,
		install:  func(u *userBehavior, body []core.Value) { u.makeBody = body },
	},
}

// knownBehaviorNames renders the installable slots for the unknown-name
// error. Derived from the table rather than written out, so a slot added
// to `behaviors` cannot go unmentioned — the four-name literal this
// replaced was already stale the moment the table grew.
func knownBehaviorNames() string {
	names := make([]string, 0, len(behaviors))
	for n := range behaviors {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func behaveHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	be, target, sig, err := behaveTarget(name, args[1], r)
	if err != nil {
		return nil, err
	}

	body := append([]Value{}, sig.Body()...)

	// Reuse an existing userBehavior wrapper if one is already
	// installed — adding a new capability (canon on top of compare)
	// must preserve previously installed slots.
	var ub *userBehavior
	if existing, ok := target.Behavior().(*userBehavior); ok {
		ub = existing
	} else {
		ub = &userBehavior{
			prev:     target.Behavior(),
			registry: r,
			typeName: target.Leaf(),
			target:   target,
		}
		target.SetBehavior(ub)
	}
	be.install(ub, body)
	// Capture the target *Type for capabilities that need to validate
	// body output against the declared type (unify currently — its
	// `[T T] → [T]` body must produce a T-compatible value at run
	// time, since validateUnifySig allows Any in the return slot).
	if name == "unify" {
		ub.unifyTarget = target
	}
	return nil, nil
}

// behaveTarget validates a behave call — the behavior name, the fn's first
// signature against the slot's declared shape — and returns the slot's entry,
// the TARGET type the capability attaches to, and the signature whose body
// it runs. Shared by the handler and its check-mode half (behaveReturns), so
// the analysis notes exactly the capability the run installs.
func behaveTarget(name string, fnVal Value, r *Registry) (behaviorEntry, *core.Type, core.FnSig, error) {
	be, ok := behaviors[name]
	if !ok {
		return be, nil, core.FnSig{}, r.BoruError("behave_error",
			fmt.Sprintf("behave %s: unknown behavior name; known: %s", name, knownBehaviorNames()),
			"behave")
	}
	info, err := extractFnDefInfo(fnVal)
	if err != nil {
		return be, nil, core.FnSig{}, fmt.Errorf("behave %s: %w", name, err)
	}
	firstSig, ok := info.FirstOwnSig()
	if !ok {
		return be, nil, core.FnSig{}, r.BoruError("behave_error", fmt.Sprintf("behave %s: fn has no signatures", name), "behave")
	}
	sig := *firstSig
	target, err := be.validate(sig)
	if err != nil {
		return be, nil, sig, fmt.Errorf("behave %s: %w", name, err)
	}
	if target == nil {
		return be, nil, sig, r.BoruError("behave_error", fmt.Sprintf("behave %s: could not infer target type from fn sig", name), "behave")
	}
	if target.Origin == core.OriginBuiltin {
		return be, nil, sig, fmt.Errorf("behave %s: cannot install on builtin type %s", name, target.Leaf())
	}
	return be, target, sig, nil
}

// behaveReturns is behave's CHECK-MODE half (NUR076). The pass does not run
// the handler, so a behave-installed capability was invisible to analysis,
// and one slot is not invisible in its consequences: `make` VALIDATES a
// construction against the target's declared schema, so a type whose own
// constructor ignores its source (`behave make/q (fn Any P [make P {a:
// 42}])`) was judged against rules that constructor never runs — `make P
// {bogus: 1}` built Class/P{a:42} and failed `boru check` with two schema
// errors, and the default pre-flight refused a program that runs. The half
// validates the call as the handler does and, for `make`, notes the target
// in the pass's own state (CheckState.NoteBehaveMaker), which HasMaker reads.
// It installs NOTHING: a wrapper on the type would put user bodies within
// reach of analysis-time rendering, comparison and construction, and the
// other seven slots change only what a program computes, which analysis does
// not evaluate. A call the pass cannot see through — a fn carrier, a
// computed name — and a call the handler would refuse note nothing; the run
// raises the refusal where it happens.
func behaveReturns(args []Value, r *Registry) []Value {
	if r == nil || !r.Check.IsActive() || !IsConcrete(args[0]) || !IsConcrete(args[1]) {
		return nil
	}
	name := defName(args[0])
	if name != "make" {
		return nil
	}
	if _, target, _, err := behaveTarget(name, args[1], r); err == nil {
		r.Check.NoteBehaveMaker(core.CanonicalType(r, target))
	}
	return nil
}

// extractFnDefInfo unwraps a TFunction value into its
// FnDefInfo payload. Returns an error for anything else.
func extractFnDefInfo(v Value) (core.FnDefInfo, error) {
	if v.Parent == nil {
		return core.FnDefInfo{}, errors.New("fn arg is nil")
	}
	if !v.Parent.Equal(core.TFunction) {
		return core.FnDefInfo{}, fmt.Errorf("fn arg must be a Function (got %s)", v.Parent.String())
	}
	info, ok := v.Data.(core.FnDefInfo)
	if !ok {
		return core.FnDefInfo{}, fmt.Errorf("fn arg has invalid payload (%T)", v.Data)
	}
	return info, nil
}

// validateCompareSig enforces shape `[[T T] [Integer] [body]]` and
// returns T. Both input params must be the same type; the kernel will
// only dispatch the comparator when both operands share an ancestor
// at or below T.
func validateCompareSig(sig core.FnSig) (*core.Type, error) {
	if len(sig.Params) != 2 {
		return nil, fmt.Errorf("compare: fn must take 2 args (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 || !sig.Returns[0].Equal(core.TInteger) {
		return nil, fmt.Errorf("compare: fn must return Integer")
	}
	t0 := sig.Params[0].Type
	t1 := sig.Params[1].Type
	if t0 == nil || t1 == nil {
		return nil, fmt.Errorf("compare: both params must declare a type")
	}
	if !t0.Equal(t1) {
		return nil, fmt.Errorf("compare: both params must be the same type (got %s and %s)", t0, t1)
	}
	return t0, nil
}

// validateCanonSig enforces shape `[[T] [String] [body]]` and returns
// T. The body produces a canonical-source string for values of type T.
func validateCanonSig(sig core.FnSig) (*core.Type, error) {
	if len(sig.Params) != 1 {
		return nil, fmt.Errorf("canon: fn must take 1 arg (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 || !sig.Returns[0].Equal(core.TString) {
		return nil, fmt.Errorf("canon: fn must return String")
	}
	t := sig.Params[0].Type
	if t == nil {
		return nil, fmt.Errorf("canon: param must declare a type")
	}
	return t, nil
}

// validateNodifySig enforces shape `[[T] [Any] [body]]` and returns
// T. The body produces a Node or Scalar projection of the value —
// the output stays in the boru data domain (Integer, String, Map,
// List, …) rather than a serialised JSON string, so callers can
// compose with other data transforms before encoding.
func validateNodifySig(sig core.FnSig) (*core.Type, error) {
	if len(sig.Params) != 1 {
		return nil, fmt.Errorf("nodify: fn must take 1 arg (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 {
		return nil, fmt.Errorf("nodify: fn must declare a single return type")
	}
	t := sig.Params[0].Type
	if t == nil {
		return nil, fmt.Errorf("nodify: param must declare a type")
	}
	return t, nil
}

// validateUnifySig enforces shape `[[T T] [T] [body]]` and returns T.
// Both inputs must be the same type — the body is a closed-over
// operation on T. The kernel dispatches the Unifier when the LCA walk
// reaches T; the body receives the two operands as `a` and `b` and is
// expected to return the unified value (or the type literal `None` to
// signal failure).
//
// The return type slot accepts either T itself or `Any`. ParseFnReturns
// runs without a registry, so user-defined types in the return position
// degrade to Any — accepting Any here keeps the surface usable while
// the kernel still validates at call time that the body returned a
// value compatible with the target type.
func validateUnifySig(sig core.FnSig) (*core.Type, error) {
	if len(sig.Params) != 2 {
		return nil, fmt.Errorf("unify: fn must take 2 args (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 {
		return nil, fmt.Errorf("unify: fn must declare a single return type")
	}
	t0 := sig.Params[0].Type
	t1 := sig.Params[1].Type
	if t0 == nil || t1 == nil {
		return nil, fmt.Errorf("unify: both params must declare a type")
	}
	if !t0.Equal(t1) {
		return nil, fmt.Errorf("unify: both params must be the same type (got %s and %s)", t0, t1)
	}
	if !sig.Returns[0].Equal(t0) && !sig.Returns[0].Equal(core.TAny) {
		return nil, fmt.Errorf("unify: return type must match input type or be Any (got %s, want %s)", sig.Returns[0], t0)
	}
	return t0, nil
}

// validateTruthySig enforces shape `[[T] [Boolean] [body]]` and returns
// T. The body decides what a value of T means in a boolean position —
// `if`, the connectives, loop conditions. Without it a type outside the
// kernel's truthiness cascade is simply truthy whatever it holds.
func validateTruthySig(sig core.FnSig) (*core.Type, error) {
	if len(sig.Params) != 1 {
		return nil, fmt.Errorf("truthy: fn must take 1 arg (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 || !sig.Returns[0].Equal(core.TBoolean) {
		return nil, fmt.Errorf("truthy: fn must return Boolean")
	}
	t := sig.Params[0].Type
	if t == nil {
		return nil, fmt.Errorf("truthy: param must declare a type")
	}
	return t, nil
}

// validateDeqSig enforces shape `[[T T] [Boolean] [body]]` and returns
// T — the same two-same-type shape `compare` requires, since deep
// equality is likewise a closed operation on T.
func validateDeqSig(sig core.FnSig) (*core.Type, error) {
	if len(sig.Params) != 2 {
		return nil, fmt.Errorf("deq: fn must take 2 args (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 || !sig.Returns[0].Equal(core.TBoolean) {
		return nil, fmt.Errorf("deq: fn must return Boolean")
	}
	t0 := sig.Params[0].Type
	t1 := sig.Params[1].Type
	if t0 == nil || t1 == nil {
		return nil, fmt.Errorf("deq: both params must declare a type")
	}
	if !t0.Equal(t1) {
		return nil, fmt.Errorf("deq: both params must be the same type (got %s and %s)", t0, t1)
	}
	return t0, nil
}

// validateSizeSig enforces shape `[[T] [Integer] [body]]` and returns T.
// The body answers `size` for values of T — the kernel rule being "the
// length of the collection the value stands for".
func validateSizeSig(sig core.FnSig) (*core.Type, error) {
	if len(sig.Params) != 1 {
		return nil, fmt.Errorf("size: fn must take 1 arg (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 || !sig.Returns[0].Equal(core.TInteger) {
		return nil, fmt.Errorf("size: fn must return Integer")
	}
	t := sig.Params[0].Type
	if t == nil {
		return nil, fmt.Errorf("size: param must declare a type")
	}
	return t, nil
}

// validateMakeSig enforces shape `[[Any] [T] [body]]` and returns T —
// read from the RETURN type, which is the one place a `behave` slot
// takes its target from anywhere but a parameter (NUR056).
//
// That is inherent, not an inconsistency to tidy away. The other seven
// capabilities answer a question ABOUT a value of T, so T is what they
// receive; construction answers how to PRODUCE one, so T is what it
// yields, and the source it is handed can be any type at all. Requiring
// a declared param type would be requiring the constructor to accept
// exactly one input shape, which is the opposite of what a constructor
// is for.
func validateMakeSig(sig core.FnSig) (*core.Type, error) {
	if len(sig.Params) != 1 {
		return nil, fmt.Errorf("make: fn must take 1 arg (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 {
		return nil, fmt.Errorf("make: fn must return exactly 1 value")
	}
	// One branch for both compile failures: a missing return type and an `Any` one
	// fail for the same reason — neither names a type to construct — and
	// folding them keeps the nil guard (which stops the Equal call below
	// from dereferencing nothing) without a second arm no fn spelling can
	// reach.
	if t := sig.Returns[0]; t == nil || t.Equal(core.TAny) {
		return nil, fmt.Errorf("make: fn must declare a concrete return type — that is the type being constructed, and Any names none")
	}
	return sig.Returns[0], nil
}

// userBehavior is the shared wrapper type that carries one or more
// boru-bodied capability slots on a target *Type. The TypeBehavior
// surface (Match / Format / Equal) delegates to the previous
// Behavior; capability methods (Compare for compare, Format-via-canon
// for canon, Nodify for nodify) run an installed body or hand off
// to prev.
//
// Re-entrancy: a canon body that triggers another render of the same
// Parent (e.g. by calling `inspect` or formatting a nested field of
// the same type) loops through Value.String → Behavior.Format → body
// → Value.String. The per-Parent inRender guard breaks the loop by
// falling back to the previous Behavior's Format on re-entry. A
// parallel inNodify guard handles `tonode`-bodies that recurse into
// the same Parent, and inTruthy/inDeq/inSize/inMake the same for the newer
// slots. The guards assume the kernel's single-goroutine engine model:
// they are SAME-GOROUTINE re-entry breaks, not synchronization.
// ForkConcurrent registries share the *Type node and therefore this
// wrapper, so a concurrent branch dispatching a capability races on
// the flags and runs the body against the INSTALLING registry — the
// known limitation recorded as design/BORU-SHARP-EDGES.0.md G14; the
// fix (fork-local capability execution state) needs its own design
// pass and applies to all eight slots alike.
type userBehavior struct {
	prev        core.TypeBehavior
	registry    *Registry
	typeName    string
	compareBody []Value
	canonBody   []Value
	nodifyBody  []Value
	unifyBody   []Value
	unifyTarget *core.Type // T from `behave unify/q (fn [[T T] [_] [...]])`
	// target is the type node this wrapper is installed on — what
	// Size's keep-walking arm resumes SizeOf's chain walk above.
	target     *core.Type
	truthyBody []Value
	deqBody    []Value
	sizeBody   []Value
	makeBody   []Value
	inRender   bool
	inNodify   bool
	inUnify    bool
	inTruthy   bool
	inDeq      bool
	inSize     bool
	inMake     bool
}

// Match delegates to prev — `behave` does not currently install
// custom Match logic; that's the job of predicate types and external
// builtins.
func (u *userBehavior) Match(v Value, t *Type) bool {
	if u.prev != nil {
		return u.prev.Match(v, t)
	}
	return core.DefaultBehavior.Match(v, t)
}

// DelegatesMatchTo exposes the wrapped behavior for the ownership-anchor
// content check (core.MatchDelegating): because Match delegates to prev,
// this wrapper's membership IS prev's, so a behave'd predicate/surface must
// still be seen as content-based and excluded from anchoring an extension
// (design/OPEN-WORDS.1.md §3.1). Returns prev (nil-terminating the walk;
// a nil prev means Match falls to DefaultBehavior, which is nominal).
func (u *userBehavior) DelegatesMatchTo() core.TypeBehavior { return u.prev }

// Equal also delegates — equality stays structural by default.
func (u *userBehavior) Equal(a, b Value) bool {
	if u.prev != nil {
		return u.prev.Equal(a, b)
	}
	return core.DefaultBehavior.Equal(a, b)
}

// Format runs the installed canon body if any, otherwise delegates
// to prev. On re-entry (a canon body triggering another render of
// the same Parent), falls back to prev to break the loop.
func (u *userBehavior) Format(v Value) string {
	if len(u.canonBody) == 0 || u.inRender {
		if u.prev != nil {
			return u.prev.Format(v)
		}
		return core.DefaultBehavior.Format(v)
	}
	u.inRender = true
	defer func() { u.inRender = false }()
	s, err := u.runCanonBody(v)
	if err != nil {
		return fmt.Sprintf("<%s canon-error: %v>", u.typeName, err)
	}
	return s
}

// Compare runs the installed comparator body if any. If no body is
// installed but prev has its own Comparer, delegate to it. Otherwise
// signal `ErrNoComparer` so CompareValues continues the parent walk.
func (u *userBehavior) Compare(a, b Value) (int, error) {
	if len(u.compareBody) > 0 {
		return u.runCompareBody(a, b)
	}
	if cmp, ok := u.prev.(core.Comparer); ok {
		return cmp.Compare(a, b)
	}
	return 0, core.ErrNoComparer
}

func (u *userBehavior) runCompareBody(a, b Value) (int, error) {
	r := u.registry
	if r == nil {
		return 0, fmt.Errorf("behave compare %s: no registry attached", u.typeName)
	}
	r.Defs.Push("a", a)
	r.Defs.Push("b", b)
	defer r.Defs.Pop("a")
	defer r.Defs.Pop("b")

	tokens := append([]Value{}, u.compareBody...)
	result, err := core.RunPooledTop(r, tokens)
	if err != nil {
		return 0, fmt.Errorf("behave compare %s: %w", u.typeName, err)
	}
	if len(result) == 0 {
		return 0, fmt.Errorf("behave compare %s: body produced no result", u.typeName)
	}
	top := result[len(result)-1]
	if !top.Parent.ConformsTo(core.TInteger) {
		return 0, fmt.Errorf("behave compare %s: body must return Integer, got %s", u.typeName, top.Parent.String())
	}
	n, err := core.AsInteger(top)
	if err != nil {
		return 0, fmt.Errorf("behave compare %s: %w", u.typeName, err)
	}
	switch {
	case n < 0:
		return -1, nil
	case n > 0:
		return 1, nil
	default:
		return 0, nil
	}
}

// Nodify runs the installed projection body if any. Falls back to
// prev's Nodifier when present, or signals ErrNoNodifier so
// NodifyValue continues the parent-chain walk. Re-entrancy is
// guarded the same way as Format — a nodify body that calls
// `tonode` on a nested value of the same type would otherwise loop.
func (u *userBehavior) Nodify(v Value) (Value, error) {
	if len(u.nodifyBody) == 0 {
		if n, ok := u.prev.(core.Nodifier); ok {
			return n.Nodify(v)
		}
		return Value{}, core.ErrNoNodifier
	}
	if u.inNodify {
		// Re-entry on the same type — fall through so the body
		// can use `tonode` on nested fields of the same type
		// without looping. Returning ErrNoNodifier here would
		// just propagate up; falling back to the value itself is
		// the natural "no further transformation" signal.
		return v, nil
	}
	u.inNodify = true
	defer func() { u.inNodify = false }()
	return u.runNodifyBody(v)
}

// Unify runs the installed unifier body if any. Falls back to prev's
// Unifier when present, or signals ErrNoUnifier so the kernel's
// dispatchUnifier walk continues up the parent chain. Re-entrancy is
// guarded the same way Format/Nodify are — a unifier body that
// recursively unifies values of the same type would otherwise loop.
func (u *userBehavior) Unify(a, b Value, r *core.Registry) (Value, *core.UnifyError) {
	if len(u.unifyBody) == 0 {
		if next, ok := u.prev.(core.Unifier); ok {
			return next.Unify(a, b, r)
		}
		return Value{}, core.ErrNoUnifier
	}
	if u.inUnify {
		// Body recurses into a same-type Unify — fall through so
		// recursion terminates. Return the structural narrowing
		// candidate via the prev chain.
		if next, ok := u.prev.(core.Unifier); ok {
			return next.Unify(a, b, r)
		}
		return Value{}, core.ErrNoUnifier
	}
	u.inUnify = true
	defer func() { u.inUnify = false }()
	return u.runUnifyBody(a, b)
}

func (u *userBehavior) runUnifyBody(a, b Value) (Value, *core.UnifyError) {
	r := u.registry
	if r == nil {
		return Value{}, &core.UnifyError{
			Reason: fmt.Sprintf("behave unify %s: no registry attached", u.typeName),
		}
	}
	r.Defs.Push("a", a)
	r.Defs.Push("b", b)
	defer r.Defs.Pop("a")
	defer r.Defs.Pop("b")

	tokens := append([]Value{}, u.unifyBody...)
	result, err := core.RunPooledTop(r, tokens)
	if err != nil {
		return Value{}, &core.UnifyError{
			Reason: fmt.Sprintf("behave unify %s: %v", u.typeName, err),
		}
	}
	if len(result) == 0 {
		return Value{}, &core.UnifyError{
			Reason: fmt.Sprintf("behave unify %s: body produced no result", u.typeName),
		}
	}
	top := result[len(result)-1]
	// A `None` return signals "no unification" — same convention as
	// predicate types use for "doesn't match". Anything else is the
	// unified value.
	if core.IsNoneShape(top) {
		return Value{}, &core.UnifyError{
			Reason: fmt.Sprintf("behave unify %s: body returned None", u.typeName),
			A:      a,
			B:      b,
		}
	}
	// Validate the body's output type. The kernel relies on Unifier
	// implementations to return a value in the target type's lattice
	// family — a buggy body that returns an unrelated type (e.g. a
	// String from a unifier on an Integer refine) would otherwise
	// propagate as an invalid unification result.
	// validateUnifySig allows `Any` in the fn return slot because
	// ParseFnReturns degrades user types without a registry, so this
	// runtime check is the actual enforcement point.
	//
	// "In the family" means: output.Parent and target are on the
	// same lattice chain — either subtype-or-equal-or-supertype.
	// Arithmetic naturally produces base-type results (Integer for
	// an Item-refine), which is a supertype of the target; reparent
	// to the target so the user sees an Item-typed value back.
	if u.unifyTarget != nil {
		tp := top.Parent
		matches := tp.Equal(u.unifyTarget) ||
			tp.IsAncestor(u.unifyTarget) ||
			u.unifyTarget.IsAncestor(tp)
		if !matches {
			return Value{}, &core.UnifyError{
				Reason: fmt.Sprintf("behave unify %s: body returned %s, expected a value in the %s family",
					u.typeName, tp.String(), u.unifyTarget.Leaf()),
				A: a,
				B: b,
			}
		}
		if u.unifyTarget.IsAncestor(tp) && !tp.Equal(u.unifyTarget) {
			top = core.ReparentValue(top, u.unifyTarget)
		}
	}
	return top, nil
}

func (u *userBehavior) runNodifyBody(v Value) (Value, error) {
	r := u.registry
	if r == nil {
		return Value{}, fmt.Errorf("behave nodify %s: no registry attached", u.typeName)
	}
	r.Defs.Push("a", v)
	defer r.Defs.Pop("a")

	tokens := append([]Value{}, u.nodifyBody...)
	result, err := core.RunPooledTop(r, tokens)
	if err != nil {
		return Value{}, fmt.Errorf("behave nodify %s: %w", u.typeName, err)
	}
	if len(result) == 0 {
		return Value{}, fmt.Errorf("behave nodify %s: body produced no result", u.typeName)
	}
	return result[len(result)-1], nil
}

// Truthy runs the installed truthiness body if any, else delegates to
// prev's Truther, else signals ErrNoTruther so CoerceBoolean falls back
// to the kernel's family cascade.
//
// Re-entrancy is guarded like Format's: a body that puts the value back
// into a boolean position (`if a [...]`) would otherwise loop through
// CoerceBoolean → Truthy → body → CoerceBoolean. On re-entry the guard
// declines, so the inner test gets the cascade's reading and the loop
// terminates.
func (u *userBehavior) Truthy(v Value) (bool, error) {
	if len(u.truthyBody) == 0 {
		if tr, ok := u.prev.(core.Truther); ok {
			return tr.Truthy(v)
		}
		return false, core.ErrNoTruther
	}
	if u.inTruthy {
		return false, core.ErrNoTruther
	}
	u.inTruthy = true
	defer func() { u.inTruthy = false }()
	return u.runTruthyBody(v)
}

func (u *userBehavior) runTruthyBody(v Value) (bool, error) {
	top, err := u.runUnaryBody(u.truthyBody, v, "truthy")
	if err != nil {
		return false, err
	}
	if !top.Parent.ConformsTo(core.TBoolean) {
		return false, fmt.Errorf("behave truthy %s: body must return Boolean, got %s",
			u.typeName, top.Parent.String())
	}
	return core.AsBoolean(top)
}

// DeepEqualValues runs the installed deq body if any, else delegates to
// prev's DeepEqualer, else signals ErrNoDeepEqualer so DeepEqual
// continues its walk and reaches its terminal verdict.
func (u *userBehavior) DeepEqualValues(a, b Value) (bool, error) {
	if len(u.deqBody) == 0 {
		if de, ok := u.prev.(core.DeepEqualer); ok {
			return de.DeepEqualValues(a, b)
		}
		return false, core.ErrNoDeepEqualer
	}
	if u.inDeq {
		return false, core.ErrNoDeepEqualer
	}
	u.inDeq = true
	defer func() { u.inDeq = false }()
	return u.runDeqBody(a, b)
}

func (u *userBehavior) runDeqBody(a, b Value) (bool, error) {
	r := u.registry
	if r == nil {
		return false, fmt.Errorf("behave deq %s: no registry attached", u.typeName)
	}
	r.Defs.Push("a", a)
	r.Defs.Push("b", b)
	defer r.Defs.Pop("a")
	defer r.Defs.Pop("b")

	tokens := append([]Value{}, u.deqBody...)
	result, err := core.RunPooledTop(r, tokens)
	if err != nil {
		return false, fmt.Errorf("behave deq %s: %w", u.typeName, err)
	}
	if len(result) == 0 {
		return false, fmt.Errorf("behave deq %s: body produced no result", u.typeName)
	}
	top := result[len(result)-1]
	if !top.Parent.ConformsTo(core.TBoolean) {
		return false, fmt.Errorf("behave deq %s: body must return Boolean, got %s",
			u.typeName, top.Parent.String())
	}
	return core.AsBoolean(top)
}

// Size runs the installed size body if any, else delegates to prev's
// Sizer, else CONTINUES SizeOf's walk above the owning type. That last
// arm is load-bearing: this wrapper satisfies core.Sizer structurally
// whether or not a size body was ever installed, so without it a
// `behave canon` on an Integer refine would silently change `size x`
// from the magnitude rule to 0 — the walk would stop here with nothing
// to say. Sizer has no decline channel (unlike Truther/DeepEqualer),
// so "not mine" is expressed by re-entering the walk via SizeOfAbove.
// A body that errors or returns a non-Integer also falls through to
// the walk, for the same reason erroring Truther bodies do.
func (u *userBehavior) Size(v Value) int {
	sizeAbove := func() int {
		if sz, ok := u.prev.(core.Sizer); ok {
			return sz.Size(v)
		}
		return core.SizeOfAbove(u.target, v)
	}
	if len(u.sizeBody) == 0 || u.inSize {
		return sizeAbove()
	}
	u.inSize = true
	defer func() { u.inSize = false }()
	top, err := u.runUnaryBody(u.sizeBody, v, "size")
	if err != nil || !top.Parent.ConformsTo(core.TInteger) {
		return sizeAbove()
	}
	n, err := core.AsInteger(top)
	if err != nil {
		return sizeAbove()
	}
	return int(n)
}

// runUnaryBody runs a one-argument capability body with the value bound
// to `a` and returns the top of the resulting stack. Shared by the
// truthy and size slots, whose only difference is the return type they
// then demand.
// MakeValue runs the installed make body if any, else delegates to prev's
// Maker, else declines with ErrNoMaker so construction proceeds exactly as
// it did before the slot existed (NUR056).
//
// Unlike every other slot, a body ERROR is returned rather than swallowed.
// `make` raises on failure, so a constructor rejecting its source is a real
// answer — "a C cannot be built from that" — and falling through to the
// kernel's coercion would silently produce a value the type's own
// constructor declined. The result's CONFORMANCE to the target is checked by
// the kernel (makerCapability), not here: it is a rule about `make`, so a
// Go-side Maker is held to it too.
func (u *userBehavior) MakeValue(target *core.Type, src Value) (Value, error) {
	if len(u.makeBody) == 0 {
		if mk, ok := u.prev.(core.Maker); ok {
			return mk.MakeValue(target, src)
		}
		return Value{}, core.ErrNoMaker
	}
	if u.inMake {
		return Value{}, core.ErrNoMaker
	}
	u.inMake = true
	defer func() { u.inMake = false }()
	return u.runUnaryBody(u.makeBody, src, "make")
}

func (u *userBehavior) runUnaryBody(body []Value, v Value, slot string) (Value, error) {
	r := u.registry
	if r == nil {
		return Value{}, fmt.Errorf("behave %s %s: no registry attached", slot, u.typeName)
	}
	r.Defs.Push("a", v)
	defer r.Defs.Pop("a")

	tokens := append([]Value{}, body...)
	result, err := core.RunPooledTop(r, tokens)
	if err != nil {
		return Value{}, fmt.Errorf("behave %s %s: %w", slot, u.typeName, err)
	}
	if len(result) == 0 {
		return Value{}, fmt.Errorf("behave %s %s: body produced no result", slot, u.typeName)
	}
	return result[len(result)-1], nil
}

func (u *userBehavior) runCanonBody(v Value) (string, error) {
	r := u.registry
	if r == nil {
		return "", fmt.Errorf("no registry attached")
	}
	r.Defs.Push("a", v)
	defer r.Defs.Pop("a")

	tokens := append([]Value{}, u.canonBody...)
	result, err := core.RunPooledTop(r, tokens)
	if err != nil {
		return "", err
	}
	if len(result) == 0 {
		return "", fmt.Errorf("body produced no result")
	}
	top := result[len(result)-1]
	if !top.Parent.ConformsTo(core.TString) {
		return "", fmt.Errorf("body must return String, got %s", top.Parent.String())
	}
	s, err := core.AsString(top)
	if err != nil {
		return "", err
	}
	return s, nil
}
