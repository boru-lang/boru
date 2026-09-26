package core

import "errors"

// Maker is the optional capability interface for `make` — the eighth and
// last of the kernel's per-type operations to get one (NUR056). A type
// implementing it decides how its own values are CONSTRUCTED.
//
// Every other kernel operation already worked this way: ordering,
// rendering, membership, unification, projection, truthiness, deep
// equality and size all dispatch through the type's Behavior, and seven
// of them are installable from boru through `behave`. Construction did
// not: `MakeConvert`'s scalar arm is a closed switch over the kernel
// leaves, and the structural arm dispatches through the Ideals REGISTRY
// — a Go-side table. So a Go Ideal could already customise construction
// (`Ideal.Instantiate`) and boru could not, which made this a Go-vs-boru
// asymmetry as much as a missing capability.
//
// The dispatch differs from the other capabilities in one way that is
// inherent rather than incidental: construction has no receiver. There is
// only a TARGET TYPE and an arbitrary source value, so the walk climbs
// the target's parent chain — there is no lowest-common-ancestor to find,
// because only one of the two operands has a type that means anything
// here. That disanalogy was argued at the verdict and accepted; it is why
// `behave make` reads its target from the fn's RETURN type rather than
// from a parameter.
//
// Return ErrNoMaker to decline and let construction continue exactly as
// it does today, the same contract Comparer and DeepEqualer use.
type Maker interface {
	MakeValue(target *Type, src Value) (Value, error)
}

// ErrNoMaker signals that a Maker-shaped Behavior has no body for this
// target, so construction falls through to the kernel's own path.
var ErrNoMaker = errors.New("no maker")

// makerCapability walks the TARGET type's parent chain looking for a
// Maker willing to build src, and returns (value, true) when one does.
//
// A body that ERRORS does NOT decline. This is the opposite of the
// DeepEqualer rule, and deliberately so: `deq` is total and has no error
// channel, so a failed body there can only be reported as a missing
// answer, whereas `make` already raises on failure and a constructor that
// rejects its source is giving a real answer — "this cannot be built from
// that". Swallowing it would fall through to the kernel's coercion and
// silently produce a value the type's own constructor refused. Only
// ErrNoMaker, the explicit decline, continues the walk.
func makerCapability(target *Type, src Value) (Value, bool, error) {
	for t := target; t != nil; t = t.Parent {
		mk, is := t.Behavior().(Maker)
		if !is {
			continue
		}
		out, err := mk.MakeValue(target, src)
		if errors.Is(err, ErrNoMaker) {
			continue
		}
		if err != nil {
			return Value{}, true, err
		}
		out, err = conformMakerResult(target, out)
		return out, true, err
	}
	return Value{}, false, nil
}

// conformMakerResult holds a Maker's result to the target type.
//
// Every other capability has a fixed return type its validator pins at
// install (Boolean, Integer, String); a Maker returns T, and only the
// target knows what T is at the call — so the check has to happen here.
// `make` is the language's narrowest construction boundary, and letting a
// body smuggle an off-type value through it would make the capability a
// hole rather than a hook.
//
// A result carrying the target's builtin BASE EXACTLY is reparented rather
// than refused, which is the same courtesy the kernel's own newtype arm
// extends (MakeScalarHandler): for `def C refine Float`, a body ending in
// `1.0` yields a plain Float, and demanding the author reparent it by hand
// would make the capability strictly harder to use than the default it
// replaces.
//
// BUILTIN is load-bearing. A bare ConformsTo test would also admit any
// SIBLING refinement of that base — `def D refine Float`, whose values
// conform to Float — and silently relabel a D as a C, which is laundering
// rather than construction. Requiring the result's own type to be a
// builtin admits exactly the untagged values ordinary literals and
// arithmetic produce (`1.0` is a Float; `"x"` is a ProperString, a builtin
// LEAF under String) and refuses every nominally-tagged user type. The
// reparent is a courtesy for an untagged value, not a cast.
//
// The result must also be CONCRETE. A body ending in a bare type node
// (`[C]`) has that node's Parent conforming to the base, so without this
// the reparent would mint a malformed type literal whose parent points at
// the target; and a Go Maker returning the zero Value has no Parent at all,
// which the diagnostic below would have dereferenced.
// target is never nil here: the only caller is makerCapability's walk,
// whose loop condition already required a node to read a Behavior from.
func conformMakerResult(target *Type, out Value) (Value, error) {
	if IsConcrete(out) {
		if out.Is(target) {
			return out, nil
		}
		if base := builtinBaseOf(target); base != nil &&
			out.Parent.Origin == OriginBuiltin && out.Parent.ConformsTo(base) {
			return ReparentValue(out, target), nil
		}
	}
	return Value{}, &BoruError{
		Code: "type_error",
		Detail: "make: the " + target.String() + " constructor returned " +
			makerResultDesc(out) + ", which is not a " + target.String(),
	}
}

// makerResultDesc names a rejected constructor result for the diagnostic
// WITHOUT assuming it is a well-formed value. A Maker is a public extension
// interface, so its result may be the zero Value (no Parent to read) or a
// type node rather than an inhabitant; this is the arm responsible for
// refusing those, and it must not panic while saying so.
func makerResultDesc(out Value) string {
	switch {
	case out.Parent == nil:
		return "no value"
	case IsBareTypeNode(out):
		return "the type " + out.Parent.String() + " rather than a value of it"
	default:
		return out.Parent.String()
	}
}

// TryMake gives the target type's Maker capability the first word on a
// `make`, ahead of every kernel construction path (NUR056). ok=false
// means no Maker claimed the target and the caller proceeds unchanged.
//
// It is called at the top of the three make handlers rather than only
// inside MakeConvert. MakeConvert receives a resolved *Type, and the
// scalar handler reaches it having ALREADY rewritten a user refinement's
// target to its builtin base — so a Maker installed on `def C refine
// Float` would never be consulted for `make C x`, which is the record's
// own example. Dispatching from the handlers keeps the capability ahead
// of both that rewrite and the Ideals registry, which is what the verdict
// asked for; MakeConvert keeps its own walk for the callers that reach
// the shared scalar coercion directly (record-field coercion).
func TryMake(reg *Registry, targetVal, srcVal Value) ([]Value, bool, error) {
	out, ok, err := makerCapability(makeTargetType(reg, targetVal), srcVal)
	if !ok || err != nil {
		return nil, ok, err
	}
	return []Value{out}, true, nil
}

// makeTargetType resolves the *Type a make target VALUE stands for, or nil
// when the target is not a type at all. A bare type node IS its type; a
// named def resolves through the registry first, so `make C x` finds the
// type C was bound to rather than the binding's value shape; and the result
// is canonicalised, because `behave` installs the wrapper on the registry's
// canonical node and a by-value type-literal copy would walk a chain of
// stale Behaviors.
//
// The isTypeLike guard is what keeps a CONCRETE value from being read as a
// target. ValueType answers with a node for any value — for a concrete one
// that node is its type — so without the guard `make c "x"`, where `c` is a
// value of a Maker-carrying C, would dispatch C's constructor as though the
// caller had written `make C "x"`. An invalid target must keep reaching the
// ordinary error path; the capability may not make a previously-refused
// spelling work.
func makeTargetType(reg *Registry, targetVal Value) *Type {
	resolved := ResolveTypeLiteralDef(targetVal, reg)
	if !isTypeLike(resolved) {
		return nil
	}
	return CanonicalType(reg, ValueType(resolved))
}

// HasMaker reports whether the make TARGET carries a constructor of its
// own — the question the CHECK pass has to ask before validating a
// construction against the type's declared schema (NUR056).
//
// A type with a Maker does not run the kernel's schema construction at
// all, so the schema's unknown/missing/mistyped-field rules describe code
// that will not execute. Applying them anyway rejects programs that run:
// `def P class {a:Integer}` with a Maker that ignores its source refuses
// `make P {bogus:1}` at check while the constructor builds it happily.
//
// A `behave make` call is seen too, though the check pass does not run it:
// its check-mode half notes the type (CheckState.BehaveMakers, NUR076), so a
// construction AFTER the call skips the validation exactly as a Go-side
// Maker's does, and one before it validates, as the run's order has it.
func HasMaker(reg *Registry, targetVal Value) bool {
	for t := makeTargetType(reg, targetVal); t != nil; t = t.Parent {
		if _, ok := t.Behavior().(Maker); ok {
			return true
		}
		if reg != nil && reg.Check != nil && reg.Check.BehaveMakers[t] {
			return true
		}
	}
	return false
}
