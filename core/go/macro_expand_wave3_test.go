package core

import (
	"strings"
	"testing"
)

// Coverage for the macro definer/expander (macro_expand.go): execMacro
// dispatch through the engine, operand scanning, template expansion
// (unquote / splice / hygiene renames / nested containers), the
// macroexpand introspection entry (ExpandMacroForm + expandAllMacros),
// and every reachable error path. Macros are installed by hand via
// InstallFnDef with Macro=true — the same shape the lang-side `macro`
// definer produces.

// installMacro binds name to a macro whose single overload takes the
// given param names and whose template body is the (inert) template
// list — the last value the body run produces.
func installMacro(r *Registry, name string, params []string, template Value, captured ...CapturedBinding) {
	ps := make([]FnParam, len(params))
	for i, p := range params {
		ps[i] = FnParam{Name: p}
	}
	InstallFnDef(r, name, FnDefInfo{
		Macro:    true,
		Captured: captured,
		Signatures: []Signature{{
			Params:     ps,
			Returns:    []*Type{TAny},
			Impl:       Boru([]Value{template}),
			BarrierPos: BarrierAllForward,
		}},
	})
}

func TestMacroExpandUnquote(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		// (m1 x) → cadd x x
		installMacro(r, "m1", []string{"a"}, NewList([]Value{
			NewWord("cadd"), NewWord("unquote"), NewWord("a"), NewWord("unquote"), NewWord("a"),
		}))
	})
	out, err := NewTop(r).Run([]Value{NewWord("m1"), NewInteger(5)})
	if err != nil {
		t.Fatalf("m1 5: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("m1 5 results = %v", out)
	}
	if n, _ := AsInteger(out[0]); n != 10 {
		t.Errorf("m1 5 = %v, want 10", out[0])
	}
}

func TestMacroExpansionCacheReused(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		installMacro(r, "m1", []string{"a"}, NewList([]Value{
			NewWord("cadd"), NewWord("unquote"), NewWord("a"), NewInteger(1),
		}))
	})
	// Same macro applied twice to the same operand form: the second
	// application hits the registry-level expansion cache.
	out, err := NewTop(r).Run([]Value{
		NewWord("m1"), NewInteger(4), NewEnd(),
		NewWord("m1"), NewInteger(4),
	})
	if err != nil {
		t.Fatalf("double m1: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("double m1 results = %v", out)
	}
	for i, v := range out {
		if n, _ := AsInteger(v); n != 5 {
			t.Errorf("result %d = %v, want 5", i, v)
		}
	}
}

func TestMacroSplice(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		// (m2 xs) → cadd <elements of xs>
		installMacro(r, "m2", []string{"xs"}, NewList([]Value{
			NewWord("cadd"), NewWord("splice"), NewWord("xs"),
		}))
	})
	out, err := NewTop(r).Run([]Value{
		NewWord("m2"), NewList([]Value{NewInteger(4), NewInteger(5)}),
	})
	if err != nil {
		t.Fatalf("m2 [4 5]: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 9 {
		t.Errorf("m2 [4 5] = %v, want 9", out[0])
	}

	// Negative: splice of a non-list operand.
	r2 := covRegistry(t, func(r *Registry) {
		installMacro(r, "m2", []string{"xs"}, NewList([]Value{
			NewWord("splice"), NewWord("xs"),
		}))
	})
	_, err = NewTop(r2).Run([]Value{NewWord("m2"), NewInteger(3)})
	if err == nil || !strings.Contains(err.Error(), "did not evaluate to a list") {
		t.Errorf("splice of integer: err = %v", err)
	}
}

func TestMacroUnquoteEvaluatesNonParamOperand(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		// unquote (cadd 1 2) → 3 inserted into the expansion.
		installMacro(r, "m3", []string{"a"}, NewList([]Value{
			NewWord("cadd"),
			NewWord("unquote"), NewWord("a"),
			NewWord("unquote"), NewParenExpr([]Value{NewWord("cadd"), NewInteger(1), NewInteger(2)}),
		}))
	})
	out, err := NewTop(r).Run([]Value{NewWord("m3"), NewInteger(10)})
	if err != nil {
		t.Fatalf("m3 10: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 13 {
		t.Errorf("m3 10 = %v, want 13", out[0])
	}
}

func TestMacroAtomEscapeBecomesWord(t *testing.T) {
	// An escape resolving to an Atom is inserted as a WORD, so a name
	// value (a gensym) acts as an identifier in the expansion.
	r := covRegistry(t, func(r *Registry) {
		installMacro(r, "m4", []string{"op"}, NewList([]Value{
			NewWord("unquote"), NewWord("op"), NewInteger(2), NewInteger(3),
		}))
	})
	out, err := NewTop(r).Run([]Value{NewWord("m4"), NewAtom("cmul")})
	if err != nil {
		t.Fatalf("m4 :cmul: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 6 {
		t.Errorf("m4 :cmul = %v, want 6", out[0])
	}
}

func TestMacroCapturedBindingVisibleToTemplate(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		installMacro(r, "m5", []string{"a"}, NewList([]Value{
			NewWord("cadd"),
			NewWord("unquote"), NewWord("a"),
			// `cap` is not a param: the escape evaluates it in the live
			// macro scope, where the captured binding is installed.
			NewWord("unquote"), NewWord("cap"),
		}), CapturedBinding{Name: "cap", Value: NewInteger(7)})
	})
	out, err := NewTop(r).Run([]Value{NewWord("m5"), NewInteger(1)})
	if err != nil {
		t.Fatalf("m5 1: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 8 {
		t.Errorf("m5 1 = %v, want 8", out[0])
	}
}

func TestMacroNestedContainers(t *testing.T) {
	// unquote/splice resolve inside nested List, ParenExpr, and Map
	// template values (expandNested / collectBindersNested).
	r := covRegistry(t, func(r *Registry) {
		inner := NewOrderedMap()
		inner.Set("k", NewList([]Value{NewWord("unquote"), NewWord("a")}))
		installMacro(r, "m6", []string{"a"}, NewList([]Value{
			NewWord("clen"),
			NewList([]Value{
				NewWord("unquote"), NewWord("a"),
				NewParenExpr([]Value{NewWord("unquote"), NewWord("a")}),
				NewMap(inner),
			}),
		}))
	})
	out, err := NewTop(r).Run([]Value{NewWord("m6"), NewInteger(9)})
	if err != nil {
		t.Fatalf("m6 9: %v", err)
	}
	// The expansion is `clen [9 (9) {k:[9]}]` — a 3-element list.
	if n, _ := AsInteger(out[0]); n != 3 {
		t.Errorf("m6 9 = %v, want 3", out[0])
	}
}

func TestMacroOperandErrors(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		installMacro(r, "m1", []string{"a"}, NewList([]Value{NewWord("unquote"), NewWord("a")}))
	})
	// No operands at all.
	_, err := NewTop(r).Run([]Value{NewWord("m1")})
	if err == nil || !strings.Contains(err.Error(), "expects 1 operand") {
		t.Errorf("bare macro word: err = %v", err)
	}
	// A structural boundary (End) stops the scan short.
	_, err = NewTop(r).Run([]Value{NewWord("m1"), NewEnd(), NewInteger(1)})
	if err == nil || !strings.Contains(err.Error(), "expects 1 operand") {
		t.Errorf("macro before End: err = %v", err)
	}
}

func TestMacroTemplateErrors(t *testing.T) {
	// Template body produces no value.
	r := covRegistry(t, func(r *Registry) {
		InstallFnDef(r, "mempty", FnDefInfo{
			Macro: true,
			Signatures: []Signature{{
				Params:     []FnParam{{Name: "a"}},
				Impl:       Boru([]Value{}),
				BarrierPos: BarrierAllForward,
			}},
		})
	})
	_, err := NewTop(r).Run([]Value{NewWord("mempty"), NewInteger(1)})
	if err == nil || !strings.Contains(err.Error(), "template produced no value") {
		t.Errorf("empty template: err = %v", err)
	}

	// Template body itself errors.
	r2 := covRegistry(t, func(r *Registry) {
		InstallFnDef(r, "mbad", FnDefInfo{
			Macro: true,
			Signatures: []Signature{{
				Params:     []FnParam{{Name: "a"}},
				Impl:       Boru([]Value{NewWord("definitely_not_a_word")}),
				BarrierPos: BarrierAllForward,
			}},
		})
	})
	_, err = NewTop(r2).Run([]Value{NewWord("mbad"), NewInteger(1)})
	if err == nil || !strings.Contains(err.Error(), "template") {
		t.Errorf("erroring template: err = %v", err)
	}

	// unquote with no operand in the template.
	r3 := covRegistry(t, func(r *Registry) {
		installMacro(r, "mtrunc", []string{"a"}, NewList([]Value{NewWord("unquote")}))
	})
	_, err = NewTop(r3).Run([]Value{NewWord("mtrunc"), NewInteger(1)})
	if err == nil || !strings.Contains(err.Error(), "missing operand") {
		t.Errorf("truncated unquote: err = %v", err)
	}

	// An escape operand that errors when evaluated.
	r4 := covRegistry(t, func(r *Registry) {
		installMacro(r, "mesc", []string{"a"}, NewList([]Value{
			NewWord("unquote"), NewParenExpr([]Value{NewWord("nope_not_defined")}),
		}))
	})
	_, err = NewTop(r4).Run([]Value{NewWord("mesc"), NewInteger(1)})
	if err == nil {
		t.Error("erroring escape operand succeeded")
	}

	// A macro with no signatures at all.
	r5 := covRegistry(t, nil)
	r5.Defs.Push("msigless", NewFunction(FnDefInfo{Name: "msigless", Macro: true}))
	_, err = NewTop(r5).Run([]Value{NewWord("msigless"), NewInteger(1)})
	if err == nil || !strings.Contains(err.Error(), "no signature") {
		t.Errorf("sig-less macro: err = %v", err)
	}
}

func TestMacroHygieneRenamesTemplateBinders(t *testing.T) {
	// A template-origin `def tmp …` binder is renamed to a fresh gensym
	// consistently; a `def unquote <name>` binder is user-origin and kept.
	r := covRegistry(t, func(r *Registry) {
		installMacro(r, "mh", []string{"a"}, NewList([]Value{
			NewWord("def"), NewWord("tmp"), NewWord("unquote"), NewWord("a"),
			NewWord("tmp"),
			NewList([]Value{NewWord("def"), NewWord("nested")}),
		}))
	})
	fd, ok := macroByName(r, "mh")
	if !ok {
		t.Fatal("mh not registered as a macro")
	}
	toks, err := ExpandMacroWith(r, &fd, []Value{NewInteger(3)})
	if err != nil {
		t.Fatalf("ExpandMacroWith: %v", err)
	}
	if len(toks) < 5 {
		t.Fatalf("expansion too short: %v", toks)
	}
	// toks: def <g> 3 <g> [def <g2>]
	w1, _ := AsWord(&toks[1])
	if w1.Name == "tmp" {
		t.Error("template binder tmp was not renamed")
	}
	w3, _ := AsWord(&toks[3])
	if w3.Name != w1.Name {
		t.Errorf("binder rename inconsistent: %q vs %q", w1.Name, w3.Name)
	}
	// The nested list's binder was renamed too (collectBindersNested).
	nested, _ := AsList(toks[4])
	nw, _ := AsWord(addrOf(nested.Get(1)))
	if nw.Name == "nested" {
		t.Error("nested template binder was not renamed")
	}
}

func TestExpandMacroFormAndAll(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		installMacro(r, "minner", []string{"b"}, NewList([]Value{
			NewWord("cadd"), NewWord("unquote"), NewWord("b"), NewInteger(1),
		}))
		// mouter expands to a call of minner — macroexpand-all must
		// expand that too.
		installMacro(r, "mouter", []string{"a"}, NewList([]Value{
			NewWord("minner"), NewWord("unquote"), NewWord("a"),
		}))
		// A macro left with too few following tokens stays as-is.
		installMacro(r, "mbare", []string{"a"}, NewList([]Value{NewWord("minner")}))
		InstallFnDef(r, "plainfn", FnDefInfo{
			Signatures: []Signature{{
				Params:     []FnParam{{Name: "n", Type: TInteger}},
				Returns:    []*Type{TInteger},
				Impl:       Boru([]Value{NewWord("n")}),
				BarrierPos: BarrierAllForward,
			}},
		})
	})

	// ParenExpr form.
	toks, err := ExpandMacroForm(r, NewParenExpr([]Value{NewWord("mouter"), NewInteger(5)}))
	if err != nil {
		t.Fatalf("ExpandMacroForm paren: %v", err)
	}
	if len(toks) != 3 {
		t.Fatalf("mouter expansion = %v", toks)
	}
	if w, _ := AsWord(&toks[0]); w.Name != "cadd" {
		t.Errorf("nested macro not expanded: %v", toks)
	}

	// List form.
	toks, err = ExpandMacroForm(r, NewList([]Value{NewWord("minner"), NewInteger(2)}))
	if err != nil {
		t.Fatalf("ExpandMacroForm list: %v", err)
	}
	if len(toks) != 3 {
		t.Fatalf("minner expansion = %v", toks)
	}

	// A macro word with too few following tokens is left unexpanded.
	toks, err = ExpandMacroForm(r, NewList([]Value{NewWord("mbare"), NewInteger(1)}))
	if err != nil {
		t.Fatalf("ExpandMacroForm mbare: %v", err)
	}
	if len(toks) != 1 || !IsWord(&toks[0]) {
		t.Errorf("bare inner macro should stay as-is: %v", toks)
	}

	// Error taxonomy.
	if _, err := ExpandMacroForm(r, NewInteger(1)); err == nil ||
		!strings.Contains(err.Error(), "expected a (macro operand") {
		t.Errorf("scalar form: err = %v", err)
	}
	if _, err := ExpandMacroForm(r, NewList(nil)); err == nil ||
		!strings.Contains(err.Error(), "must start with a macro word") {
		t.Errorf("empty form: err = %v", err)
	}
	if _, err := ExpandMacroForm(r, NewList([]Value{NewWord("no_such_macro"), NewInteger(1)})); err == nil ||
		!strings.Contains(err.Error(), "undefined macro") {
		t.Errorf("undefined macro: err = %v", err)
	}
	if _, err := ExpandMacroForm(r, NewList([]Value{NewWord("plainfn"), NewInteger(1)})); err == nil ||
		!strings.Contains(err.Error(), "is not a macro") {
		t.Errorf("non-macro word: err = %v", err)
	}
}

func TestExpandAllMacrosNestedContainers(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		installMacro(r, "minner", []string{"b"}, NewList([]Value{
			NewWord("cadd"), NewWord("unquote"), NewWord("b"), NewInteger(1),
		}))
		// Expansion embeds macro calls inside a list, a paren, and a map
		// value — expandAllNested recurses into each.
		inner := NewOrderedMap()
		inner.Set("k", NewList([]Value{NewWord("minner"), NewInteger(3)}))
		installMacro(r, "mwrap", []string{"a"}, NewList([]Value{
			NewList([]Value{NewWord("minner"), NewWord("unquote"), NewWord("a")}),
			NewParenExpr([]Value{NewWord("minner"), NewInteger(2)}),
			NewMap(inner),
		}))
	})
	toks, err := ExpandMacroForm(r, NewList([]Value{NewWord("mwrap"), NewInteger(1)}))
	if err != nil {
		t.Fatalf("ExpandMacroForm mwrap: %v", err)
	}
	if len(toks) != 3 {
		t.Fatalf("mwrap expansion = %v", toks)
	}
	lst, err := AsList(toks[0])
	if err != nil || lst.Len() != 3 {
		t.Errorf("nested list not macro-expanded: %v", toks[0])
	}
	pe, err := AsParenExpr(&toks[1])
	if err != nil || len(pe) != 3 {
		t.Errorf("nested paren not macro-expanded: %v", toks[1])
	}
	m, err := AsMap(toks[2])
	if err != nil {
		t.Fatalf("map value lost: %v", err)
	}
	kv, _ := m.Get("k")
	klst, err := AsList(kv)
	if err != nil || klst.Len() != 3 {
		t.Errorf("map-value list not macro-expanded: %v", kv)
	}
}

func TestRecursiveMacroExpansionDepthBound(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		installMacro(r, "mrec", []string{"a"}, NewList([]Value{
			NewWord("mrec"), NewWord("unquote"), NewWord("a"),
		}))
	})
	_, err := ExpandMacroForm(r, NewList([]Value{NewWord("mrec"), NewInteger(1)}))
	if err == nil || !strings.Contains(err.Error(), "too deep") {
		t.Errorf("recursive macro: err = %v", err)
	}
}

func TestMacroByNameDiscriminates(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		installMacro(r, "misa", []string{"a"}, NewList([]Value{NewInteger(1)}))
	})
	if _, ok := macroByName(r, "misa"); !ok {
		t.Error("macroByName missed a macro")
	}
	if _, ok := macroByName(r, "unbound_name"); ok {
		t.Error("macroByName found an unbound name")
	}
	// A plain value binding is not a macro.
	InstallDef(r, "plainval", NewInteger(3))
	if _, ok := macroByName(r, "plainval"); ok {
		t.Error("macroByName treated a value binding as a macro")
	}
}
