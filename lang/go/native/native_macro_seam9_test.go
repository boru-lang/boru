package native

import (
	"testing"
)

// W9_nativeB — coverage for native_macro.go handlers and the namespace
// resolution helpers (mini / parse / emit). The namespace helpers each do
// an independent Defs.Top + namespace-facet lookup; the guard arms are
// driven directly by binding the namespace to a plain value or leaving it
// unbound. See design/TEST-SEAMS.10.md.

// --- macroHandler arg guards ---

func TestW9MacroHandlerNonConcreteList(t *testing.T) {
	r := seam5Reg(t)
	if _, err := macroHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r); err == nil {
		t.Fatal("macro: non-concrete list must error")
	}
}

func TestW9MacroHandlerNonWordParam(t *testing.T) {
	r := seam5Reg(t)
	// [[5] [body]] — the single param is a literal, not a bare name.
	spec := NewList([]Value{
		NewList([]Value{NewInteger(5)}),
		NewList([]Value{NewWord("body")}),
	})
	if _, err := macroHandler([]Value{spec}, nil, nil, r); err == nil {
		t.Fatal("macro: non-word param must error")
	}
}

func TestW9MacroHandlerBareValueBody(t *testing.T) {
	r := seam5Reg(t)
	// [[a] 5] — a bare (non-list) body is wrapped into a single-element body.
	spec := NewList([]Value{
		NewList([]Value{NewWord("a")}),
		NewInteger(5),
	})
	out, err := macroHandler([]Value{spec}, nil, nil, r)
	if err != nil {
		t.Fatalf("macro bare body: %v", err)
	}
	if len(out) != 1 || !ParentOf(&out[0]).Equal(TFunction) {
		t.Fatalf("macro should yield a Function, got %v", Canon(out))
	}
}

// --- miniHandler / parseHandler: kind must be a literal ---

func TestW9MiniHandlerKindNotLiteral(t *testing.T) {
	r := seam5Reg(t)
	if _, err := miniHandler([]Value{NewInteger(5), NewString("x"), NewMap(NewOrderedMap())}, nil, nil, r); err == nil {
		t.Fatal("mini: non-literal kind must error")
	}
}

func TestW9ParseHandlerKindNotLiteral(t *testing.T) {
	r := seam5Reg(t)
	if _, err := parseHandler([]Value{NewInteger(5), NewString("x")}, nil, nil, r); err == nil {
		t.Fatal("parse: non-literal kind must error")
	}
}

// --- mini namespace helpers ---

func TestW9MiniKindExportUnbound(t *testing.T) {
	r := seam5Reg(t) // MiniLang not imported
	if _, ok := miniKindExport(r, "lang_re"); ok {
		t.Fatal("miniKindExport must fail when MiniLang is unbound")
	}
}

func TestW9MiniKindExportNonModuleExport(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("MiniLang", NewInteger(5)) // bound, but not a module namespace
	if _, ok := miniKindExport(r, "lang_re"); ok {
		t.Fatal("miniKindExport must fail when MiniLang is not a module namespace")
	}
}

func TestW9MiniPartialFnWrapperMissing(t *testing.T) {
	r := seam5Reg(t) // MiniLang unbound → miniKindExport fails
	if _, ok := miniPartialFn(r, "re", "lang_re", []Value{NewEnd()}); ok {
		t.Fatal("miniPartialFn must decline when the wrapper is missing")
	}
}

func TestW9MiniPartialFnWrapperNotFnDef(t *testing.T) {
	r := seam5Reg(t)
	fields := NewOrderedMap()
	fields.Set("lang_re", NewInteger(5)) // export is not an FnDef
	r.Defs.Push("MiniLang", NewModuleNamespace("MiniLang", fields, Value{}))
	if _, ok := miniPartialFn(r, "re", "lang_re", []Value{NewEnd()}); ok {
		t.Fatal("miniPartialFn must decline when the export is not an FnDef")
	}
}

// (The miniCompileExport / miniInvokeBoruCompile seams died with the boru
// compile-hook surface — hooks are Go-only builtin machinery now.)

func TestW9MiniKindRegisteredNonModuleExport(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("MiniLang", NewInteger(5))
	if miniKindRegistered(r, "lang_re") {
		t.Fatal("miniKindRegistered must be false when MiniLang is not a module namespace")
	}
}

// --- parse namespace helpers + deferred-kind tracking ---

func TestW9ParseKindRegisteredNonModuleExport(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("ParseLang", NewInteger(5))
	if parseKindRegistered(r, "parse_calc") {
		t.Fatal("parseKindRegistered must be false when ParseLang is not a module namespace")
	}
}

func TestW9EmitKindRegisteredNonModuleExport(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("EmitLang", NewInteger(5))
	if emitKindRegistered(r, "emit_json") {
		t.Fatal("emitKindRegistered must be false when EmitLang is not a module namespace")
	}
}

// (The deferred parse-KIND machinery — MarkParseKindDeferred and the
// parselang-deferred-dispatch recorder — was deleted with Parse.register:
// grammar parsers are now Parse.parser Function VALUES dispatched through
// parselang-fn-dispatch; see native_macro_parsefn_test.go.)
