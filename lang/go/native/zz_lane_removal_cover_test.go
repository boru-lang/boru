package native

import (
	"strings"
	"testing"
)

// Direct coverage for the defensive arms the compiler lane's suites used to
// reach and nothing in the interpreter delivers (design/COMPILER-LANE-
// REMOVAL.0.md §4): handler guards for operands the matcher never hands
// over, ReturnsFns asked without a registry, and the mini-compile state of
// a registry that never registered a hook. Every negative row has its
// positive twin beside it, per lang/go/CLAUDE.md.

// The cross-collection redirects (collIsConcreteList): a map overload of
// fold / scan handed a concrete LIST delegates to the list handler, so the
// one handler drives either shape. The matcher never routes a List to a Map
// signature, so the redirect is driven directly, as its each twin is.
func TestMapIterHandlersRedirectAConcreteList(t *testing.T) {
	r := w9Reg(t)
	drive := w9Driven(r)
	xs := w9List(NewInteger(1), NewInteger(2), NewInteger(3))
	add := w9List(NewWord("add"))

	out, err := drive(foldMapInitHandler([]Value{add, xs, NewInteger(10)}, nil, nil, r))
	if err != nil || len(out) != 1 || out[0].String() != "16" {
		t.Fatalf("fold with a seed over a list: %v / %v, want 16", out, err)
	}
	out, err = drive(foldMapNoInitHandler([]Value{add, xs}, nil, nil, r))
	if err != nil || len(out) != 1 || out[0].String() != "6" {
		t.Fatalf("fold over a list: %v / %v, want 6", out, err)
	}
	out, err = drive(scanMapHandler([]Value{add, xs}, nil, nil, r))
	if err != nil || len(out) != 1 || out[0].String() != "[1 3 6]" {
		t.Fatalf("scan over a list: %v / %v, want [1 3 6]", out, err)
	}
	// The map shape stays on the map path.
	m := w9Map("a", NewInteger(1), "b", NewInteger(2))
	out, err = drive(foldMapInitHandler([]Value{add, m, NewInteger(10)}, nil, nil, r))
	if err != nil || len(out) != 1 || out[0].String() != "13" {
		t.Fatalf("fold with a seed over a map: %v / %v, want 13", out, err)
	}
}

// requireConcreteMap refuses a concrete value that is neither collection
// with the dispatcher's own signature_error; a carrier keeps the word's
// error, and a real map passes.
func TestRequireConcreteMapRefusesAConcreteScalar(t *testing.T) {
	r := w9Reg(t)
	_, err := requireConcreteMap(r, NewInteger(5), "keys")
	if err == nil || !strings.Contains(err.Error(), "signature_error") {
		t.Fatalf("a concrete Integer: %v, want signature_error", err)
	}
	if _, err := requireConcreteMap(r, NewCarrier(TMap), "keys"); err == nil ||
		!strings.Contains(err.Error(), "keys_error") {
		t.Fatalf("a Map carrier: %v, want keys_error", err)
	}
	if m, err := requireConcreteMap(r, w9Map("a", NewInteger(1)), "keys"); err != nil || m.Len() != 1 {
		t.Fatalf("a concrete map: %v / %v", m, err)
	}
}

// parse over a Function-family value with no FnDefInfo payload: under
// analysis a carrier degrades to dynamic(Any); a concrete one — only a
// crafted value — is refused.
func TestParseFnExpandRefusesACraftedFunction(t *testing.T) {
	r := w9Reg(t)
	crafted := NewValueRaw(TFunction, IntPayload{N: 1})
	src := NewString("x")
	if _, err := parseFnExpand(crafted, []Value{crafted, src}, r); err == nil ||
		!strings.Contains(err.Error(), "not a usable function value") {
		t.Fatalf("a crafted fn value: %v, want the parse_error refusal", err)
	}
	defer r.Check.Begin()()
	out, err := parseFnExpand(NewCarrier(TFunction), []Value{NewCarrier(TFunction), src}, r)
	if err != nil || len(out) != 1 || !IsDynamic(&out[0]) {
		t.Fatalf("a fn carrier under analysis: %v / %v, want dynamic(Any)", out, err)
	}
}

// A registry no hook was registered on has no mini-compile state, so
// nothing is discoverable; registering creates the state and the hook.
func TestMiniCompileNoStateArms(t *testing.T) {
	r := b2Reg(t)
	if s := miniCompileStateFor(r, false); s != nil {
		t.Fatal("miniCompileStateFor(create=false) with no state should be nil")
	}
	if h, ok := miniGoHook(r, "re"); ok || h != nil {
		t.Fatalf("miniGoHook with no state = %v/%v, want nil/false", h, ok)
	}
	called := false
	RegisterMiniCompileGoHook(r, "re", func(string, Value, *Registry) ([]Value, error) {
		called = true
		return nil, nil
	})
	h, ok := miniGoHook(r, "re")
	if !ok || h == nil {
		t.Fatal("registered hook should be discoverable")
	}
	if _, err := h("x", NewTypeLiteral(TNone), r); err != nil || !called {
		t.Fatalf("hook call = %v, called=%v", err, called)
	}
	if _, ok := miniGoHook(r, "bf"); ok {
		t.Fatal("a kind with no registered hook has none")
	}
}

// ReturnsFns asked without a registry answer the word's declared carrier;
// with one they model the receiver.
func TestReturnsFnsWithoutARegistry(t *testing.T) {
	if out := patrunNewReturns(nil, nil); len(out) != 1 || !out[0].Carrier() || !out[0].Parent().Equal(TPatrun) {
		t.Fatalf("patrunNewReturns(nil) = %v, want a Patrun carrier", out)
	}
	if out := contextReturns(nil, nil); len(out) != 1 || !out[0].Carrier() || !out[0].Parent().Equal(TStore) {
		t.Fatalf("contextReturns(nil) = %v, want a Store carrier", out)
	}
	if out := delFlexMapReturns(nil, nil); len(out) != 1 || !out[0].Carrier() || !out[0].Parent().Equal(TFlexMap) {
		t.Fatalf("delFlexMapReturns(nil) = %v, want a FlexMap carrier", out)
	}
	r := w9Reg(t)
	if out := delFlexMapReturns([]Value{NewString("a"), NewCarrier(TFlexMap)}, r); len(out) != 1 || !out[0].Parent().Equal(TFlexMap) {
		t.Fatalf("delFlexMapReturns over an unshaped receiver = %v, want a fresh FlexMap carrier", out)
	}
}

// A dispatch modifier over a non-concrete fn value under analysis: a
// dynamic Any or a Function carrier is plausibly a function at run time and
// rides a gradual Function carrier; a concrete-typed carrier is provably not
// one and is left to the strict handler.
func TestCheckModeGradualFn(t *testing.T) {
	r := seam5Reg(t)
	if _, ok := checkModeGradualFn(r, NewCarrier(TInteger)); ok {
		t.Fatal("inactive check mode must not gradual-wrap")
	}
	defer r.Check.Begin()()
	if _, ok := checkModeGradualFn(r, NewCarrier(TInteger)); ok {
		t.Fatal("Integer carrier must not gradual-wrap")
	}
	if out, ok := checkModeGradualFn(r, NewDynamicCarrier(TAny)); !ok ||
		len(out) != 1 || !out[0].Parent().ConformsTo(TFunction) {
		t.Fatalf("dynamic Any carrier must gradual-wrap, got %v / %v", out, ok)
	}
	if out, ok := checkModeGradualFn(r, NewCarrier(TFunction)); !ok || len(out) != 1 {
		t.Fatalf("Function carrier must gradual-wrap, got %v / %v", out, ok)
	}
}
