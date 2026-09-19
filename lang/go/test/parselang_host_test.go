package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// The calc PARSER is the parse-system sibling of the iop calc mini-language:
// same `<a> <op> <b>` surface, but it returns an AST `{op,left,right}` (raw
// token strings) instead of an evaluated integer. It is built through the Go
// host API (lang.NewParseLangFn) and bound with DefineValue; the framework
// resolves the source (String or {src:…}) before the handler runs, so the
// handler sees a String.

func calcParserSpec() lang.ParseLangSpec {
	return lang.ParseLangSpec{
		Name:    "calc",
		Returns: []*lang.Type{lang.TMap},
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
			src, err := args[0].AsConcreteString() // already resolved by the framework
			if err != nil {
				return nil, r.BoruError("parse_error", "calc: src: "+err.Error(), "parse_calc")
			}
			parts := strings.Fields(src)
			if len(parts) != 3 {
				return nil, r.BoruErrorHint("parse_syntax_error",
					"calc: expected '<a> <op> <b>', got "+src, "parse_calc",
					"write a binary expression like 'x + y'")
			}
			switch parts[1] {
			case "+", "-", "*", "/", "%":
			default:
				return nil, r.BoruErrorHint("parse_syntax_error",
					"calc: unknown operator "+parts[1], "parse_calc", "use one of + - * / %")
			}
			ast := native.NewOrderedMap()
			ast.Set("op", native.NewString(parts[1]))
			ast.Set("left", native.NewString(parts[0]))
			ast.Set("right", native.NewString(parts[2]))
			return []native.Value{native.NewMap(ast)}, nil
		},
	}
}

func newCalcParserInstance(t *testing.T) *lang.Boru {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	v, err := lang.NewParseLangFn(calcParserSpec())
	if err != nil {
		t.Fatalf("NewParseLangFn: %v", err)
	}
	if err := a.DefineValue("calc", v); err != nil {
		t.Fatalf("DefineValue: %v", err)
	}
	return a
}

// Compile-time pin: a host parser's Handler carries the exported ParseLang
// type, and the lang alias is identical to the modules definition — this
// assignment fails to compile if either export drifts.
var _ lang.ParseLang = calcParserSpec().Handler

// TestParseLangHostHandlerType pins the exported ParseLang handler type at
// runtime: a ParseLang is directly invocable with the framework's
// post-resolution argument shape (args[0]=source String, args[1]=opts Map).
func TestParseLangHostHandlerType(t *testing.T) {
	p := calcParserSpec().Handler
	if p == nil {
		t.Fatal("calc parser handler: expected a non-nil ParseLang")
	}
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	out, perr := p([]native.Value{
		native.NewString("x + y"), native.NewMap(native.NewOrderedMap()),
	}, nil, nil, r)
	if perr != nil || len(out) != 1 {
		t.Fatalf("ParseLang direct call: out=%v err=%v", out, perr)
	}
	// The same contract still rejects bad input when called directly.
	if _, perr = p([]native.Value{
		native.NewString("nope"), native.NewMap(native.NewOrderedMap()),
	}, nil, nil, r); perr == nil {
		t.Error("ParseLang direct call: a malformed source should error")
	}
}

// TestParseLangValueComputed pins the computed ParseLang-value form of the
// core `parse` word — the parser fn produced by a runtime call: `parse (mk)
// 'hi'`. The module-parselang.tsv §10 twin row additionally pins compiled
// parity (the recorded parselang-fn-dispatch); this is the fast Go-level
// interpreter pin.
func TestParseLangValueComputed(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	got, err := a.Run(`import "boru:parselang"
		def mk fn [[] [Function] [fn [[source:String opts:Map] [Any] [source]]]]
		parse (mk) 'hi'`)
	if err != nil {
		t.Fatalf("computed ParseLang value: %v", err)
	}
	if len(got) != 1 || got[0] != "hi" {
		t.Fatalf("computed ParseLang value = %v, want [hi]", got)
	}
}

// TestParseLangHostCalcAST pins the AST a `parse calc` call produces — every
// operator, and the operand fields — via field access on the returned node.
func TestParseLangHostCalcAST(t *testing.T) {
	a := newCalcParserInstance(t)
	imp := `import "boru:parselang"  `
	for _, op := range []string{"+", "-", "*", "/", "%"} {
		got := runLast(t, a, imp+`(parse calc 'a `+op+` b').op`)
		if got != op {
			t.Errorf("(parse calc 'a %s b').op = %v, want %q", op, got, op)
		}
	}
	if got := runLast(t, a, imp+`(parse calc 'x + y').left`); got != "x" {
		t.Errorf(".left = %v, want x", got)
	}
	if got := runLast(t, a, imp+`(parse calc 'x + y').right`); got != "y" {
		t.Errorf(".right = %v, want y", got)
	}
	// operands are arbitrary tokens, not fixed identifiers
	if got := runLast(t, a, imp+`(parse calc '3 * 4').left`); got != "3" {
		t.Errorf("numeric .left = %v, want 3", got)
	}
}

// TestParseLangHostSourceForms covers the source argument: inline String and
// {src:…} agree; {file:…} is deferred; an empty / bad source map errors.
func TestParseLangHostSourceForms(t *testing.T) {
	a := newCalcParserInstance(t)
	imp := `import "boru:parselang"  `
	inline := runLast(t, a, imp+`(parse calc 'x + y').op`)
	srcMap := runLast(t, a, imp+`(parse calc {src:'x + y'}).op`)
	if inline != srcMap || inline != "+" {
		t.Fatalf("inline=%v {src}=%v: source forms must agree on the AST", inline, srcMap)
	}
	// opts present (3-arg form) — calc ignores opts but the source still parses.
	if got := runLast(t, a, imp+`(parse calc {trace:true} 'x + y').op`); got != "+" {
		t.Fatalf("opts form .op = %v, want +", got)
	}
	neg := []struct {
		name, src, want string
	}{
		{"file deferred", imp + `parse calc {file:'x.calc'}`, "parse_file_unsupported"},
		{"empty source map", imp + `parse calc {}`, "parse_bad_source"},
		{"bad source map", imp + `parse calc {nope:1}`, "parse_bad_source"},
	}
	for _, c := range neg {
		t.Run(c.name, func(t *testing.T) {
			if _, err := a.Run(c.src); err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}

// TestParseLangHostDesugarEquivalence proves `parse calc` on a bound parser
// value is sugar for the direct call `calc <source> <opts> end`.
func TestParseLangHostDesugarEquivalence(t *testing.T) {
	a := newCalcParserInstance(t)
	imp := `import "boru:parselang"  `
	sugar := runLast(t, a, imp+`(parse calc 'x * y').op`)
	desugared := runLast(t, a, imp+`(calc 'x * y' {} end).op`)
	if sugar != desugared || sugar != "*" {
		t.Fatalf("sugar=%v desugared=%v: parse must desugar to the standard call", sugar, desugared)
	}
}

// TestParseLangHostRegisterAfterImport covers the post-import registration
// (live-inject) path — the module is already built and cached.
func TestParseLangHostRegisterAfterImport(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if _, err := runReference(t, a, `import "boru:parselang"`); err != nil {
		t.Fatalf("import: %v", err)
	}
	v, err := lang.NewParseLangFn(calcParserSpec())
	if err != nil {
		t.Fatalf("NewParseLangFn: %v", err)
	}
	if err := a.DefineValue("calc", v); err != nil {
		t.Fatalf("DefineValue after import: %v", err)
	}
	if got := runLast(t, a, `(parse calc 'x - y').op`); got != "-" {
		t.Fatalf("post-import binding: got %v, want -", got)
	}
}

// TestParseLangHostIsolation confirms a parser registered on one instance does
// not leak into another.
func TestParseLangHostIsolation(t *testing.T) {
	a := newCalcParserInstance(t)
	b, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if got := runLast(t, a, `import "boru:parselang"  (parse calc 'x + y').op`); got != "+" {
		t.Fatalf("instance a: got %v, want +", got)
	}
	if _, err := b.Run(`import "boru:parselang"  parse calc 'x + y'`); err == nil {
		t.Fatal("instance b: expected parse_unknown_lang, got nil error")
	} else if !strings.Contains(err.Error(), "calc") {
		t.Fatalf("instance b: error should name the unknown parser, got: %v", err)
	}
}

// TestParseLangHostRuntimeErrors pins the parser's loud failures. Each case
// runs on a FRESH instance — `Run` state persists across calls, so a shared
// instance would leak the import into the "no import" case.
func TestParseLangHostRuntimeErrors(t *testing.T) {
	imp := `import "boru:parselang"  `
	cases := []struct{ name, src, want string }{
		{"unknown operator", imp + `parse calc 'x ^ y'`, "unknown operator"},
		{"malformed source", imp + `parse calc 'x +'`, "expected"},
		{"unknown kind", imp + `parse nope 'x + y'`, "parse_unknown_lang"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := newCalcParserInstance(t)
			_, err := a.Run(c.src)
			if err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}

// TestParseLangHostNoImportValueForm pins that a BOUND parser value needs no
// boru:parselang import at all — the value form is import-free (only the
// kind sugar and the ParseLang namespace need the module).
func TestParseLangHostNoImportValueForm(t *testing.T) {
	a := newCalcParserInstance(t)
	if got := runLast(t, a, `(parse calc 'x + y').op`); got != "+" {
		t.Fatalf("no-import value form: got %v, want +", got)
	}
}

// TestParseLangHostRegistrationContract pins what NewParseLangFn refuses.
func TestParseLangHostRegistrationContract(t *testing.T) {
	t.Run("empty name", func(t *testing.T) {
		if _, err := lang.NewParseLangFn(lang.ParseLangSpec{Handler: calcParserSpec().Handler}); err == nil {
			t.Fatal("expected error for empty name")
		}
	})
	t.Run("nil handler rejected", func(t *testing.T) {
		if _, err := lang.NewParseLangFn(lang.ParseLangSpec{Name: "calc"}); err == nil {
			t.Fatal("expected error for nil handler")
		}
	})
	t.Run("DefineValue rejects capitalised names", func(t *testing.T) {
		a, _ := lang.New()
		v, err := lang.NewParseLangFn(calcParserSpec())
		if err != nil {
			t.Fatal(err)
		}
		if err := a.DefineValue("Calc", v); err == nil {
			t.Fatal("expected error for a capitalised value name")
		}
		if err := a.DefineValue("", v); err == nil {
			t.Fatal("expected error for an empty value name")
		}
		// The full def word-name grammar is enforced — a name source boru
		// cannot spell must be refused, not silently installed.
		if err := a.DefineValue("1parser", v); err == nil {
			t.Fatal("expected error for a name failing ValidateWordName")
		}
	})
	// (The parse_ prefix and duplicate-kind rules died with the namespace:
	// a value binding is def-scoped — rebinding shadows, like any def.)
}
