package core

import (
	"strings"
	"testing"
)

// macro_expand_seam8_test.go covers the residual error-propagation and
// edge arms of the macro expander (macro_expand.go) that a well-formed
// macro program never reaches: the fewer-operands-than-params continue,
// the non-list template result, nil-backed map containers, escapes that
// produce no value, and the error arms threaded through expandNested /
// expandAllMacros / expandAllNested. Per the fleet brief these are driven
// by direct calls to the in-package helpers with hand-built inputs (the
// same seam vm_seam7_test / emit_seam7_test use).

// w8nilMap is a map-typed concrete value whose payload is NOT a MapPayload:
// IsConcrete is true (a payload is present) yet AsMap returns a nil ReadMap
// (the "not a map payload" error path), tripping every `if m == nil` guard.
// A MapPayload{M:nil} would instead wrap a typed-nil *OrderedMap in a
// non-nil interface and panic on Keys(), so it can't reach the guard.
func w8nilMap() Value { return MakeValue(TMap, IntPayload{N: 1}) }

// w8macroReg builds a covRegistry plus a `w8void` native that returns no
// value (for the escape-produced-no-value arm) and a set of macros.
func w8macroReg(t *testing.T) *Registry {
	t.Helper()
	return covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "w8void",
			Signatures: []Signature{{
				Args:    nil,
				Returns: []*Type{}, BarrierPos: -1,
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return nil, nil
				}),
			}},
		})
		// merr: a macro whose template is `[unquote]` — expanding it always
		// fails with "missing operand", the shape every error arm needs.
		installMacro(r, "merr", []string{"a"}, NewList([]Value{NewWord("unquote")}))
		// mscalar: template body runs to a single NON-list value (an Integer),
		// so tmpl takes the else arm (macro_expand.go:149).
		InstallFnDef(r, "mscalar", FnDefInfo{
			Macro: true,
			Signatures: []Signature{{
				Params:     []FnParam{{Name: "a"}},
				Returns:    []*Type{TAny},
				Impl:       Boru([]Value{NewInteger(42)}),
				BarrierPos: BarrierAllForward,
			}},
		})
		// mtwo: two params, so expanding with a single operand trips the
		// i >= len(operands) continue (macro_expand.go:119).
		installMacro(r, "mtwo", []string{"a", "b"}, NewList([]Value{NewInteger(1)}))
	})
}

func w8macroFd(t *testing.T, r *Registry, name string) *FnDefInfo {
	t.Helper()
	fd, ok := macroByName(r, name)
	if !ok {
		t.Fatalf("%s not registered as a macro", name)
	}
	return &fd
}

// --- expandMacroWith: fewer operands than params, non-list template ------

func TestW8ExpandFewerOperandsThanParams(t *testing.T) {
	r := w8macroReg(t)
	// mtwo has 2 params; give a single operand so the second iteration hits
	// the i >= len(operands) continue.
	toks, err := ExpandMacroWith(r, w8macroFd(t, r, "mtwo"), []Value{NewInteger(7)})
	if err != nil {
		t.Fatalf("mtwo expand: %v", err)
	}
	if len(toks) != 1 {
		t.Fatalf("mtwo expansion = %v, want [1]", toks)
	}
}

func TestW8ExpandNonListTemplate(t *testing.T) {
	r := w8macroReg(t)
	// mscalar's body runs to a bare Integer, exercising the else arm that
	// wraps a non-list template result in a single-element token list.
	toks, err := ExpandMacroWith(r, w8macroFd(t, r, "mscalar"), []Value{NewInteger(1)})
	if err != nil {
		t.Fatalf("mscalar expand: %v", err)
	}
	if len(toks) != 1 {
		t.Fatalf("mscalar expansion = %v, want a single node", toks)
	}
	if n, _ := AsInteger(toks[0]); n != 42 {
		t.Errorf("mscalar expansion node = %v, want 42", toks[0])
	}
}

// --- collectBindersNested / expandNested nil-backed map ------------------

func TestW8CollectBindersNestedNilMap(t *testing.T) {
	set := map[string]bool{}
	collectBindersNested(w8nilMap(), set) // nil-map guard: returns without touching set
	if len(set) != 0 {
		t.Fatalf("nil map contributed binders: %v", set)
	}
}

func TestW8ExpandNestedNilMap(t *testing.T) {
	r := w8macroReg(t)
	got, err := expandNested(r, w8nilMap(), map[string]Value{}, map[string]string{})
	if err != nil {
		t.Fatalf("expandNested(nil map): %v", err)
	}
	if !ParentOf(&got).Equal(TMap) {
		t.Fatalf("nil map should pass through unchanged, got %v", got)
	}
}

// --- resolveTemplateEscape: operand produces no value --------------------

func TestW8ResolveEscapeNoValue(t *testing.T) {
	r := w8macroReg(t)
	// A non-param word bound to a 0-result native: the fast path misses
	// (empty bindings), the operand runs and yields nothing.
	_, err := resolveTemplateEscape(r, NewWord("w8void"), map[string]Value{})
	if err == nil || !strings.Contains(err.Error(), "produced no value") {
		t.Fatalf("void escape: err = %v, want produced-no-value", err)
	}
}

// --- expandNested: error arms threaded through paren / map ---------------

func TestW8ExpandNestedParenError(t *testing.T) {
	r := w8macroReg(t)
	// A paren template value containing a truncated `unquote` errors as the
	// nested walk recurses (macro_expand.go:324).
	bad := NewParenExpr([]Value{NewWord("unquote")})
	if _, err := expandNested(r, bad, map[string]Value{}, map[string]string{}); err == nil ||
		!strings.Contains(err.Error(), "missing operand") {
		t.Fatalf("paren escape error: err = %v", err)
	}
}

func TestW8ExpandNestedMapError(t *testing.T) {
	r := w8macroReg(t)
	// A map whose value is a list carrying a truncated `unquote`: the map
	// loop propagates the inner error (macro_expand.go:337).
	om := NewOrderedMap()
	om.Set("k", NewList([]Value{NewWord("unquote")}))
	if _, err := expandNested(r, NewMap(om), map[string]Value{}, map[string]string{}); err == nil ||
		!strings.Contains(err.Error(), "missing operand") {
		t.Fatalf("map escape error: err = %v", err)
	}
}

// --- ExpandMacroForm: expansion error surfaces ---------------------------

func TestW8ExpandMacroFormExpansionError(t *testing.T) {
	r := w8macroReg(t)
	// merr expands to a truncated unquote; ExpandMacroForm must surface it.
	if _, err := ExpandMacroForm(r, NewList([]Value{NewWord("merr"), NewInteger(1)})); err == nil ||
		!strings.Contains(err.Error(), "missing operand") {
		t.Fatalf("ExpandMacroForm(merr): err = %v", err)
	}
}

// --- expandAllMacros / expandAllNested: error arms -----------------------

func TestW8ExpandAllMacrosHeadError(t *testing.T) {
	r := w8macroReg(t)
	// merr at the head of the token list errors during expandMacroWith
	// (macro_expand.go:425).
	if _, err := expandAllMacros(r, []Value{NewWord("merr"), NewInteger(1)}, 0); err == nil ||
		!strings.Contains(err.Error(), "missing operand") {
		t.Fatalf("expandAllMacros head error: err = %v", err)
	}
}

func TestW8ExpandAllMacrosNestedListError(t *testing.T) {
	r := w8macroReg(t)
	// A list token (not a macro head) whose contents call merr: the error
	// threads back through expandAllNested's list arm (454) and the
	// expandAllMacros nested arm (440).
	tok := NewList([]Value{NewWord("merr"), NewInteger(1)})
	if _, err := expandAllMacros(r, []Value{tok}, 0); err == nil ||
		!strings.Contains(err.Error(), "missing operand") {
		t.Fatalf("expandAllMacros nested list error: err = %v", err)
	}
}

func TestW8ExpandAllNestedParenError(t *testing.T) {
	r := w8macroReg(t)
	// A paren token whose contents call merr (expandAllNested:463).
	tok := NewParenExpr([]Value{NewWord("merr"), NewInteger(1)})
	if _, err := expandAllNested(r, tok, 0); err == nil ||
		!strings.Contains(err.Error(), "missing operand") {
		t.Fatalf("expandAllNested paren error: err = %v", err)
	}
}

func TestW8ExpandAllNestedMapArms(t *testing.T) {
	r := w8macroReg(t)
	// nil-backed map passes through (expandAllNested:469).
	got, err := expandAllNested(r, w8nilMap(), 0)
	if err != nil || !ParentOf(&got).Equal(TMap) {
		t.Fatalf("expandAllNested(nil map) = %v, %v", got, err)
	}
	// A map whose value is a list calling merr surfaces the inner error
	// (expandAllNested:476).
	om := NewOrderedMap()
	om.Set("k", NewList([]Value{NewWord("merr"), NewInteger(1)}))
	if _, err := expandAllNested(r, NewMap(om), 0); err == nil ||
		!strings.Contains(err.Error(), "missing operand") {
		t.Fatalf("expandAllNested map inner error: err = %v", err)
	}
}
