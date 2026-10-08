package native

import (
	"strings"
	"testing"
)

// Wave-7 coverage for native_map_iter.go: newMapBody's non-body reject,
// requireConcreteMap's two reject arms, callLambda's no-signature and
// body-error arms, and runQuotationBody's error propagation
// (design/TEST-SEAMS.10.md).
//
// The invokeBodyTop error arm (native_map_iter.go:96) fires only for a
// compiled-closure body driven by the bytecode VM (IsCompiledClosure),
// which cannot be constructed from a plain interpreter unit test — left
// as a documented residual.

func b2Lambda(t *testing.T, r *Registry, src string) Value {
	t.Helper()
	out, err := b2Run(r, src)
	if err != nil || len(out) != 1 || !out[0].Parent.ConformsTo(TFunction) {
		t.Fatalf("expected a lambda from %q, got %v / %v", src, out, err)
	}
	return out[0]
}

func TestNewMapBodyRejectsNonBody(t *testing.T) {
	r := b2Reg(t)
	// A bare List type literal is neither closure, lambda, nor concrete list.
	if _, err := newMapBody(r, NewTypeLiteral(TList), "each"); err == nil ||
		!strings.Contains(err.Error(), "quotation list or a lambda") {
		t.Fatalf("newMapBody(type literal): expected reject, got %v", err)
	}
	// Positive pair: a concrete quotation list classifies as tokens.
	mb, err := newMapBody(r, NewList([]Value{NewWord("dup")}), "each")
	if err != nil || mb.lambda || mb.closure || len(mb.tokens) != 1 {
		t.Fatalf("newMapBody(list) = %+v / %v", mb, err)
	}
}

func TestRequireConcreteMapArms(t *testing.T) {
	r := b2Reg(t)
	// !IsConcrete arm: a bare Map type literal.
	if _, err := requireConcreteMap(r, NewTypeLiteral(TMap), "each"); err == nil {
		t.Fatal("requireConcreteMap(type literal): expected error")
	}
	// m == nil arm: a typed-but-empty map `{:String}` is concrete and
	// Map-conforming, but AsMap returns nil (ChildTypeInfo, no entries).
	typed, terr := b2Run(r, "{:String}")
	if terr != nil || len(typed) != 1 {
		t.Fatal("could not build typed empty map")
	}
	if _, err := requireConcreteMap(r, typed[0], "each"); err == nil {
		t.Fatal("requireConcreteMap(typed empty map): expected error")
	}
	// Positive pair: a real map passes.
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	if _, err := requireConcreteMap(r, NewMap(om), "each"); err != nil {
		t.Fatalf("requireConcreteMap(real map): unexpected error %v", err)
	}
}

func TestCallLambdaNoMatchAndBodyError(t *testing.T) {
	r := b2Reg(t)
	// A 2-param lambda handed one entry (a single KeyVal) has no matching
	// signature.
	mb, err := newMapBody(r, b2Lambda(t, r, "([a:Any b:Any] => [a])"), "each")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := mb.value(r, "k", NewInteger(1), 0, 1); err == nil ||
		!strings.Contains(err.Error(), "no matching lambda signature") {
		t.Fatalf("2-param lambda over 1 entry: expected no-sig error, got %v", err)
	}

	// A matching lambda whose body raises propagates the CallBoru error.
	mb2, err := newMapBody(r, b2Lambda(t, r, "([kv:Any] => [zzz_undef_word_xyz])"), "each")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := mb2.value(r, "k", NewInteger(1), 0, 1); err == nil {
		t.Fatal("lambda body error should propagate")
	}
}

func TestRunQuotationBodyError(t *testing.T) {
	r := b2Reg(t)
	drive := w9Driven(r)
	// A token body referencing an undefined word errors when the sub-engine
	// runs it.
	mb, err := newMapBody(r, NewList([]Value{NewWord("zzz_undef_word_xyz")}), "each")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := mb.value(r, "k", NewInteger(1), 0, 1); err == nil {
		t.Fatal("undefined-word quotation body should error")
	}

	// Positive pair: a working quotation body transforms the value.
	out, err := drive(eachMapHandler([]Value{
		NewList([]Value{NewWord("dup"), NewWord("add")}),
		mapVal("a", NewInteger(2)),
	}, nil, nil, r))
	if err != nil || len(out) != 1 {
		t.Fatalf("each over map = %v / %v", out, err)
	}
}

func mapVal(k string, v Value) Value {
	om := NewOrderedMap()
	om.Set(k, v)
	return NewMap(om)
}
