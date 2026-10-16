package core

import (
	"errors"
	"strings"
	"testing"
)

// NUR056: `make` was the one kernel operation with no per-type opt-in.
// These pin the CAPABILITY — the kernel half — with a Go Maker, so the
// contract holds for a Go-side type as well as for a `behave make/q`
// body (whose wiring is pinned in lang/go/native).

// makerBehavior is a Behavior that also implements Maker. build==nil
// declines, which is how a type with other capabilities but no
// constructor must behave.
type makerBehavior struct {
	TypeBehavior
	build func(target *Type, src Value) (Value, error)
}

func (m makerBehavior) MakeValue(target *Type, src Value) (Value, error) {
	if m.build == nil {
		return Value{}, ErrNoMaker
	}
	return m.build(target, src)
}

// makerType mints a refinement of base carrying a Maker.
func makerType(t *testing.T, r *Registry, name string, base *Type, build func(*Type, Value) (Value, error)) *Type {
	t.Helper()
	ty := r.Types.MintType(name, base)
	if ty == nil {
		t.Fatalf("MintType %s", name)
	}
	ty.SetBehavior(makerBehavior{TypeBehavior: ty.Behavior(), build: build})
	return ty
}

func TestNUR056MakerBuildsItsOwnValues(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	ty := makerType(t, r, "NUR056Built", TFloat, func(target *Type, _ Value) (Value, error) {
		return MintRetagged(addrOf(NewFloat(1.5)), target), nil
	})
	// The source is ignored by this constructor, which is the point: a
	// Maker decides how a value is built, not merely how one is coerced.
	out, ok, err := makerCapability(ty, NewString("anything at all"))
	if !ok {
		t.Fatal("a type carrying a Maker must claim its own construction")
	}
	if err != nil {
		t.Fatalf("MakeValue: %v", err)
	}
	if !out.Is(ty) {
		t.Fatalf("result must be a %s, got %s", ty, ParentOf(&out))
	}
}

// A type with no Maker in its chain leaves construction exactly as it
// was — the decline is what makes the capability additive.
func TestNUR056NoMakerDeclines(t *testing.T) {
	if _, ok, _ := makerCapability(TFloat, NewString("2.5")); ok {
		t.Fatal("a plain builtin must not claim construction")
	}
	r, _ := NewRegistry()
	ty := makerType(t, r, "NUR056Silent", TFloat, nil)
	if _, ok, _ := makerCapability(ty, NewString("2.5")); ok {
		t.Fatal("a Maker returning ErrNoMaker must decline, not claim")
	}
}

// The walk climbs the TARGET's chain — there is no receiver to find a
// lowest common ancestor with — so a refinement inherits its base type's
// constructor.
func TestNUR056MakerIsInheritedDownTheChain(t *testing.T) {
	r, _ := NewRegistry()
	base := makerType(t, r, "NUR056Base", TFloat, func(target *Type, _ Value) (Value, error) {
		return MintRetagged(addrOf(NewFloat(7)), target), nil
	})
	sub := r.Types.MintType("NUR056Sub", base)
	out, ok, err := makerCapability(sub, NewString("x"))
	if !ok || err != nil {
		t.Fatalf("a descendant must inherit its branch's constructor: ok=%v err=%v", ok, err)
	}
	// …and the target handed to the body is the type being CONSTRUCTED,
	// not the one that supplied the body.
	if !out.Is(sub) {
		t.Fatalf("result must be a %s, got %s", sub, ParentOf(&out))
	}
}

// A body ERROR is the constructor's answer — "this cannot be built from
// that" — and must reach the caller. This is the opposite of the
// DeepEqualer rule, where an erroring body declines; the difference is
// that `make` has an error channel and `deq` does not.
func TestNUR056MakerErrorIsNotADecline(t *testing.T) {
	r, _ := NewRegistry()
	boom := errors.New("a NUR056Refuse needs a number")
	ty := makerType(t, r, "NUR056Refuse", TFloat, func(*Type, Value) (Value, error) {
		return Value{}, boom
	})
	_, ok, err := makerCapability(ty, NewString("x"))
	if !ok {
		t.Fatal("an erroring constructor still CLAIMED the construction")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("the body's error must reach the caller, got %v", err)
	}
}

// The result is held to the target. Every other capability has a return
// type its validator pins at install; this one returns T, so only the
// call site can check it.
func TestNUR056MakerResultMustConformToTheTarget(t *testing.T) {
	r, _ := NewRegistry()
	ty := makerType(t, r, "NUR056Wrong", TFloat, func(*Type, Value) (Value, error) {
		return NewString("not a float at all"), nil
	})
	_, ok, err := makerCapability(ty, NewString("x"))
	if !ok || err == nil {
		t.Fatal("an off-type result must be refused, not passed through")
	}
	if !strings.Contains(err.Error(), "NUR056Wrong") {
		t.Fatalf("the refusal must name the target type, got %v", err)
	}
}

// …but a result carrying the target's builtin BASE is reparented rather
// than refused, the same courtesy the kernel's own newtype construction
// extends. Without it a body ending in `1.0` — the obvious spelling —
// would be rejected, making the capability harder to use than the
// default it replaces.
func TestNUR056BaseTypedResultIsReparented(t *testing.T) {
	r, _ := NewRegistry()
	ty := makerType(t, r, "NUR056Bare", TFloat, func(*Type, Value) (Value, error) {
		return NewFloat(1), nil // a plain Float, not reparented by the body
	})
	out, ok, err := makerCapability(ty, NewString("x"))
	if !ok || err != nil {
		t.Fatalf("a base-typed result must be accepted: ok=%v err=%v", ok, err)
	}
	if !out.Is(ty) {
		t.Fatalf("…and reparented to the target, got %s", ParentOf(&out))
	}
}

// TryMake is the handlers' entry point: it resolves the target VALUE to
// a type before walking. A non-type target claims nothing.
func TestNUR056TryMakeResolvesItsTarget(t *testing.T) {
	r, _ := NewRegistry()
	ty := makerType(t, r, "NUR056Try", TFloat, func(target *Type, _ Value) (Value, error) {
		return MintRetagged(addrOf(NewFloat(3)), target), nil
	})
	out, ok, err := TryMake(r, NewTypeLiteral(ty), NewString("x"))
	if !ok || err != nil {
		t.Fatalf("TryMake must claim a Maker-carrying target: ok=%v err=%v", ok, err)
	}
	if len(out) != 1 || !out[0].Is(ty) {
		t.Fatalf("TryMake must return the constructed value, got %v", out)
	}
	// A concrete value is not a type, so there is nothing to dispatch on.
	if _, ok, _ := TryMake(r, NewInteger(1), NewString("x")); ok {
		t.Fatal("a non-type target must not claim construction")
	}
	// An erroring constructor propagates through TryMake too.
	bad := makerType(t, r, "NUR056TryBad", TFloat, func(*Type, Value) (Value, error) {
		return NewString("wrong"), nil
	})
	if _, ok, err := TryMake(r, NewTypeLiteral(bad), NewString("x")); !ok || err == nil {
		t.Fatal("TryMake must surface the conformance refusal")
	}
}

// MakeConvert keeps its own walk for the callers that reach the shared
// scalar coercion WITHOUT passing a make handler — record-field coercion
// is the live one. Without this arm a field declared of a Maker-carrying
// type would be coerced by the kernel switch, ignoring its constructor.
func TestNUR056MakeConvertConsultsTheMaker(t *testing.T) {
	r, _ := NewRegistry()
	ty := makerType(t, r, "NUR056Field", TFloat, func(target *Type, _ Value) (Value, error) {
		return MintRetagged(addrOf(NewFloat(9)), target), nil
	})
	out, err := MakeConvert(NewString("ignored"), ty)
	if err != nil {
		t.Fatalf("MakeConvert: %v", err)
	}
	if !out.Is(ty) {
		t.Fatalf("MakeConvert must use the target's constructor, got %s", ParentOf(&out))
	}
	// The unclaimed path is untouched.
	if _, err := MakeConvert(NewString("2.5"), TFloat); err != nil {
		t.Fatalf("an unclaimed target must convert as before: %v", err)
	}
}

// The capability runs at the top of all THREE make handlers, not only the
// two the verdict named. Each arm is its own dispatch entry — a scalar
// target, a structural target, and the generic fallback — and a Maker that
// reached only one of them would be silently absent from the others.
func TestNUR056EveryMakeHandlerConsultsTheMaker(t *testing.T) {
	build := func(target *Type, _ Value) (Value, error) {
		return MintRetagged(addrOf(NewFloat(42)), target), nil
	}
	for _, tc := range []struct {
		name    string
		handler func([]Value, map[string]Value, []Value, *Registry) ([]Value, error)
	}{
		{"scalar", MakeScalarHandler},
		{"obj", MakeObjHandler},
		{"generic", MakeHandler},
	} {
		r, err := NewRegistry()
		if err != nil {
			t.Fatalf("%s: NewRegistry: %v", tc.name, err)
		}
		ty := makerType(t, r, "NUR056Handler"+tc.name, TFloat, build)
		out, err := tc.handler([]Value{NewTypeLiteral(ty), NewString("ignored")}, nil, nil, r)
		if err != nil {
			t.Errorf("%s handler: %v", tc.name, err)
			continue
		}
		if len(out) != 1 || !out[0].Is(ty) {
			t.Errorf("%s handler did not use the target's constructor: %v", tc.name, out)
		}
	}
}

// …and every handler leaves an unclaimed target alone, which is what makes
// the capability additive rather than a new construction path.
func TestNUR056EveryMakeHandlerDeclinesWithoutAMaker(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	out, err := MakeScalarHandler([]Value{NewTypeLiteral(TFloat), NewString("2.5")}, nil, nil, r)
	if err != nil {
		t.Fatalf("an unclaimed scalar target must convert as before: %v", err)
	}
	if f, _ := AsFloat(out[0]); f != 2.5 {
		t.Errorf("make Float '2.5' = %v, want 2.5", out[0])
	}
}

// ─── PR #379 review (Codex) ───────────────────────────────────────────

// A CONCRETE value is not a make target. ValueType answers with a node
// for any value — for a concrete one, its type — so without a type-like
// guard `make c "x"`, where c holds a value of a Maker-carrying C, would
// dispatch C's constructor as though the caller had written `make C "x"`.
// An invalid target must keep reaching the ordinary error path: a new
// capability may not make a previously-refused spelling work.
func TestNUR056ConcreteValueIsNotAMakeTarget(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	ty := makerType(t, r, "NUR056Target", TFloat, func(target *Type, _ Value) (Value, error) {
		return MintRetagged(addrOf(NewFloat(1)), target), nil
	})
	// The positive: the TYPE claims construction.
	if _, ok, _ := TryMake(r, NewTypeLiteral(ty), NewString("x")); !ok {
		t.Fatal("the type itself must claim construction")
	}
	// The negative twin: a VALUE of that very type must not, even though
	// its type carries the Maker.
	inhabitant := MintRetagged(addrOf(NewFloat(2)), ty)
	if _, ok, _ := TryMake(r, inhabitant, NewString("x")); ok {
		t.Fatal("a concrete value must not be read as a make target")
	}
}

// The reparent courtesy is for an UNTAGGED base value, not a cast. A
// ConformsTo test would also admit any sibling refinement of the builtin
// base and silently relabel it as the target — laundering, not
// construction.
func TestNUR056SiblingRefinementIsNotLaundered(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	sibling := r.Types.MintType("NUR056Sibling", TFloat)
	ty := makerType(t, r, "NUR056Launder", TFloat, func(*Type, Value) (Value, error) {
		return MintRetagged(addrOf(NewFloat(5)), sibling), nil
	})
	_, ok, err := makerCapability(ty, NewString("x"))
	if !ok || err == nil {
		t.Fatal("a sibling refinement of the base must be refused, not relabelled")
	}
	if !strings.Contains(err.Error(), "NUR056Sibling") {
		t.Errorf("the refusal must name what was returned, got %v", err)
	}
}

// A Maker is a public extension interface, so its result may not be a
// well-formed value at all. Both shapes are refused, and — the point —
// the diagnostic must not dereference what it is refusing.
func TestNUR056MalformedMakerResultsAreRefusedNotPanics(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	for _, tc := range []struct {
		name string
		out  Value
		want string
	}{
		{"the zero Value has no Parent to read", Value{}, "no value"},
		{"a bare type node is not an inhabitant", NewTypeLiteral(TFloat), "rather than a value"},
	} {
		ty := makerType(t, r, "NUR056Malformed"+tc.name[:4], TFloat,
			func(*Type, Value) (Value, error) { return tc.out, nil })
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("%s: panicked instead of refusing: %v", tc.name, p)
				}
			}()
			_, ok, err := makerCapability(ty, NewString("x"))
			if !ok || err == nil {
				t.Fatalf("%s: must be refused", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s: diagnostic %q must contain %q", tc.name, err, tc.want)
			}
		}()
	}
}

// The OPTIONS form reaches the structural branches directly rather than
// through MakeHandler, so it needs its own dispatch — otherwise
// `make T data` used the custom constructor while `make T data opts`
// silently used the kernel's.
func TestNUR056OptionsFormConsultsTheMaker(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	ty := makerType(t, r, "NUR056Opts", TFloat, func(target *Type, _ Value) (Value, error) {
		return MintRetagged(addrOf(NewFloat(11)), target), nil
	})
	opts := NewOrderedMap()
	opts.Set("base", NewBoolean(true))
	out, err := MakeWithOpts([]Value{NewTypeLiteral(ty), NewString("x"), NewMap(opts)}, nil, nil, r)
	if err != nil {
		t.Fatalf("MakeWithOpts: %v", err)
	}
	if len(out) != 1 || !out[0].Is(ty) {
		t.Fatalf("the options form must use the target's constructor, got %v", out)
	}
}

// HasMaker is the CHECK pass's question: does this target construct
// itself, and therefore is its declared schema not the thing being run?
func TestNUR056HasMakerAnswersForTheCheckPass(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	ty := makerType(t, r, "NUR056Has", TFloat, func(target *Type, _ Value) (Value, error) {
		return MintRetagged(addrOf(NewFloat(1)), target), nil
	})
	if !HasMaker(r, NewTypeLiteral(ty)) {
		t.Error("a Maker-carrying target must be recognised")
	}
	// Inherited down the chain, like the construction walk itself.
	if !HasMaker(r, NewTypeLiteral(r.Types.MintType("NUR056HasSub", ty))) {
		t.Error("a descendant of a Maker-carrying type must be recognised too")
	}
	if HasMaker(r, NewTypeLiteral(TFloat)) {
		t.Error("a plain builtin carries no constructor")
	}
	if HasMaker(r, NewInteger(1)) {
		t.Error("a non-type target carries none either")
	}
}
