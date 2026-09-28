package basic

import (
	"fmt"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/parser/go"
)

// merge519Registry is a kernel registry with the basic layer installed and
// ready to run source — the eng-only embedder shape TestRegisterStandalone
// drives.
func merge519Registry(t *testing.T) *Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("core.NewRegistry: %v", err)
	}
	if err := Register(r); err != nil {
		t.Fatalf("basic.Register: %v", err)
	}
	r.SetParseFunc(parser.Parse)
	r.InitRootContext()
	r.MarkReady()
	return r
}

func merge519Parse(t *testing.T, src string) []Value {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return vals
}

// TestMerge519ShuffleOnlyBodyNeedsTheRegisteredAllAnyNative pins
// shuffleOnlyBody's registry screen: a shuffle word counts as raise-free only
// while its binding is the ONE all-Any native signature basic registers. A
// registry where the word is unbound, carries a second overload, is a boru
// body rather than a Go native, or narrows its argument may raise at run
// time, so the body is not proven raise-free. The positive half is the stock
// basic registration, and its negative twin is a body whose shuffle takes a
// value the body never pushed.
func TestMerge519ShuffleOnlyBodyNeedsTheRegisteredAllAnyNative(t *testing.T) {
	body := []Value{core.NewInteger(1), core.NewWord("drop")}

	// The stock registration: `[1 drop]` is raise-free; `[drop]` is not.
	stock := merge519Registry(t)
	if !shuffleOnlyBody(body, stock) {
		t.Error("[1 drop] over the registered all-Any native drop must be shuffle-only")
	}
	if shuffleOnlyBody([]Value{core.NewWord("drop")}, stock) {
		t.Error("[drop] takes a value the body never pushed: it can raise")
	}

	fresh := func() *Registry {
		t.Helper()
		r, err := core.NewRegistry()
		if err != nil {
			t.Fatalf("core.NewRegistry: %v", err)
		}
		return r
	}
	anySig := Signature{Args: []*Type{TAny}, Impl: Go(dropHandler), Returns: []*Type{}, BarrierPos: 0}

	// Unbound: a kernel registry without the stack vocabulary.
	unbound := fresh()
	if unbound.Lookup("drop") != nil {
		t.Fatal("a bare kernel registry must not bind drop")
	}
	if shuffleOnlyBody(body, unbound) {
		t.Error("an unbound drop must not be proven raise-free")
	}

	// Two overloads: another signature may take the value and raise.
	multi := fresh()
	multi.Register("drop", anySig,
		Signature{Args: []*Type{TAny, TAny}, Impl: Go(dropHandler), Returns: []*Type{}, BarrierPos: 0})
	if fd := multi.Lookup("drop"); fd == nil || len(fd.Signatures) != 2 {
		t.Fatalf("want a two-signature drop, got %+v", fd)
	}
	if shuffleOnlyBody(body, multi) {
		t.Error("a multi-overload drop must not be proven raise-free")
	}

	// A boru body in the one slot: it runs user code, which may raise.
	boru := fresh()
	boru.Register("drop", Signature{Args: []*Type{TAny}, Impl: &core.BoruImpl{}, Returns: []*Type{}, BarrierPos: 0})
	if shuffleOnlyBody(body, boru) {
		t.Error("a non-native drop must not be proven raise-free")
	}

	// A narrowed argument: a value of another type fails the dispatch.
	typed := fresh()
	typed.Register("drop", Signature{Args: []*Type{TString}, Impl: Go(dropHandler), Returns: []*Type{}, BarrierPos: 0})
	if shuffleOnlyBody(body, typed) {
		t.Error("a drop over String must not be proven raise-free for an Integer")
	}

	// The same one all-Any native on a fresh registry is proven again.
	plain := fresh()
	plain.Register("drop", anySig)
	if !shuffleOnlyBody(body, plain) {
		t.Error("a lone all-Any native drop must be shuffle-only")
	}
}

// TestMerge519ArmSpliceHandlerValueArms pins __arm's value half: a typed
// list reaching the List slot is the arm's one value, returned
// as is — the interpreter's spliceArg reads it as data — while a plain
// concrete list is a code body and runs.
func TestMerge519ArmSpliceHandlerValueArms(t *testing.T) {
	r := merge519Registry(t)

	typed := merge519Parse(t, "[:Integer 1 2]")
	if len(typed) != 1 || !IsTypedList(typed[0]) {
		t.Fatalf("want one typed list, got %v", typed)
	}
	out, err := ArmSpliceHandler(typed, nil, nil, r)
	if err != nil {
		t.Fatalf("typed-list arm: %v", err)
	}
	if len(out) != 1 || !IsTypedList(out[0]) || fmt.Sprint(out[0]) != fmt.Sprint(typed[0]) {
		t.Errorf("a typed-list arm is its own value: got %v", out)
	}

	// Negative twin: a plain list is a code body — its values land.
	plain := merge519Parse(t, "[1 2]")
	out, err = ArmSpliceHandler(plain, nil, nil, r)
	if err != nil {
		t.Fatalf("code-body arm: %v", err)
	}
	if fmt.Sprint(out) != "[1 2]" {
		t.Errorf("a code-body arm runs: got %v, want [1 2]", out)
	}
}

// TestMerge519FnUndefReparentTarget pins which named FnUndef constraint a
// typed def reparents to: a user-declared fn-shape type is the target; an
// inline shape (no name), a name that binds no type, a builtin name and a
// non-FnUndef constraint are not.
func TestMerge519FnUndefReparentTarget(t *testing.T) {
	r := merge519Registry(t)
	if _, err := core.NewTop(r).Run(merge519Parse(t, "def M fnsig [[Integer] [Integer]]")); err != nil {
		t.Fatalf("def M fnsig: %v", err)
	}
	shape, ok := r.TopTypeBody("M")
	if !ok || !shape.Parent.Equal(TFnUndef) {
		t.Fatalf("M must bind an FnUndef type body, got %v (ok=%v)", shape, ok)
	}

	want := r.LookupTypeName("M")
	if want == nil || want.Origin == core.OriginBuiltin {
		t.Fatalf("M must be a user type, got %v", want)
	}
	if got := fnUndefReparentTarget(r, shape, "M"); got != want {
		t.Errorf("a user fn-shape type reparents to itself: got %v, want %v", got, want)
	}

	if got := fnUndefReparentTarget(r, shape, ""); got != nil {
		t.Errorf("an inline shape has no reparent target, got %v", got)
	}
	if got := fnUndefReparentTarget(r, shape, "nosuchtype"); got != nil {
		t.Errorf("a name binding no type has no reparent target, got %v", got)
	}
	if got := fnUndefReparentTarget(r, shape, "Integer"); got != nil {
		t.Errorf("a builtin type name has no reparent target, got %v", got)
	}
	if got := fnUndefReparentTarget(r, core.NewTypeLiteral(TInteger), "M"); got != nil {
		t.Errorf("a non-FnUndef constraint has no reparent target, got %v", got)
	}
}
