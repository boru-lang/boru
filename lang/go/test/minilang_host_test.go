package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// The calc mini-language exercises the Go host value constructor
// (lang.NewMiniLangFn) bound with DefineValue. `iop` is an integer binary
// operation: src is "<a> <op> <b>", a and b name keys in the opts map, and
// op is one of + - * / %. It is a generator shape (no stack input —
// operands come from opts), so its standard call is
// [src:String opts:Map] -> [Integer].

// calcSpec builds the iop mini-language. Implemented entirely in Go — the
// point of the host API is that a mini-language needs no boru source and no
// fork of the core minilang module.
func calcSpec() lang.MiniLangSpec {
	return lang.MiniLangSpec{
		Name:    "iop",
		Returns: []*lang.Type{lang.TInteger},
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
			src, err := args[0].AsConcreteString()
			if err != nil {
				return nil, r.BoruError("mini_error", "iop: src: "+err.Error(), "lang_iop")
			}
			m, merr := native.AsMap(args[1])
			if merr != nil || m == nil {
				return nil, r.BoruError("mini_error", "iop: opts must be a map", "lang_iop")
			}
			parts := strings.Fields(src)
			if len(parts) != 3 {
				return nil, r.BoruErrorHint("mini_parse_error",
					"iop: expected '<a> <op> <b>', got "+src, "lang_iop",
					"write a binary expression like 'x + y'")
			}
			a, err := calcOperand(r, m, parts[0])
			if err != nil {
				return nil, err
			}
			b, err := calcOperand(r, m, parts[2])
			if err != nil {
				return nil, err
			}
			var out int64
			switch parts[1] {
			case "+":
				out = a + b
			case "-":
				out = a - b
			case "*":
				out = a * b
			case "/", "%":
				if b == 0 {
					return nil, r.BoruError("mini_eval_error", "iop: division by zero", "lang_iop")
				}
				if parts[1] == "/" {
					out = a / b
				} else {
					out = a % b
				}
			default:
				return nil, r.BoruErrorHint("mini_parse_error",
					"iop: unknown operator "+parts[1], "lang_iop", "use one of + - * / %")
			}
			return []native.Value{native.NewInteger(out)}, nil
		},
	}
}

// calcOperand resolves a variable name to its integer value in opts.
func calcOperand(r *native.Registry, m native.ReadMap, name string) (int64, error) {
	v, ok := m.Get(name)
	if !ok {
		return 0, r.BoruError("mini_eval_error", "iop: undefined variable "+name, "lang_iop")
	}
	n, err := v.AsConcreteInteger()
	if err != nil {
		return 0, r.BoruError("mini_error", "iop: "+name+": "+err.Error(), "lang_iop")
	}
	return n, nil
}

// newCalcInstance builds a boru instance with the iop value bound.
func newCalcInstance(t *testing.T) *lang.Boru {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	v, err := lang.NewMiniLangFn(calcSpec())
	if err != nil {
		t.Fatalf("NewMiniLangFn: %v", err)
	}
	if err := a.DefineValue("iop", v); err != nil {
		t.Fatalf("DefineValue: %v", err)
	}
	return a
}

// runLast runs src and returns the single residual value, failing on error
// or on an unexpected stack shape.
func runLast(t *testing.T, a *lang.Boru, src string) any {
	t.Helper()
	res, err := runReference(t, a, src)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("Run(%q): expected 1 result, got %d: %v", src, len(res), res)
	}
	return res[0]
}

// TestMiniLangHostCalcOperators pins every operator's result through a
// `mini iop` call site — binding BEFORE import.
func TestMiniLangHostCalcOperators(t *testing.T) {
	a := newCalcInstance(t)
	cases := []struct {
		expr string
		opts string
		want int64
	}{
		{"x + y", "{x:10, y:2}", 12},
		{"x - y", "{x:10, y:2}", 8},
		{"x * y", "{x:10, y:2}", 20},
		{"x / y", "{x:10, y:2}", 5},
		{"x % y", "{x:10, y:3}", 1},
		// operand names are arbitrary opts keys, not fixed to x/y
		{"a - b", "{a:3, b:9}", -6},
	}
	for _, c := range cases {
		src := `import "boru:minilang"  mini iop '` + c.expr + `' ` + c.opts
		got := runLast(t, a, src)
		if got != c.want {
			t.Errorf("mini iop '%s' %s = %v, want %d", c.expr, c.opts, got, c.want)
		}
	}
}

// TestMiniLangHostDesugarEquivalence proves `mini iop` on a bound value is
// pure sugar for the direct call `iop <src> <opts> end` — both must agree.
func TestMiniLangHostDesugarEquivalence(t *testing.T) {
	a := newCalcInstance(t)
	sugar := runLast(t, a, `import "boru:minilang"  mini iop 'x * y' {x:6, y:7}`)
	desugared := runLast(t, a, `import "boru:minilang"  iop 'x * y' {x:6, y:7} end`)
	if sugar != desugared {
		t.Fatalf("sugar=%v desugared=%v: mini must desugar to the direct call", sugar, desugared)
	}
	if sugar != int64(42) {
		t.Fatalf("got %v, want 42", sugar)
	}
}

// TestMiniLangHostRegisterAfterImport covers binding AFTER the import — the
// module is already built and cached; a def binding needs nothing from it.
func TestMiniLangHostRegisterAfterImport(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	// Import first — this builds and caches the minilang module.
	if _, err := runReference(t, a, `import "boru:minilang"`); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Bind AFTER the import.
	v, err := lang.NewMiniLangFn(calcSpec())
	if err != nil {
		t.Fatalf("NewMiniLangFn: %v", err)
	}
	if err := a.DefineValue("iop", v); err != nil {
		t.Fatalf("DefineValue after import: %v", err)
	}
	got := runLast(t, a, `mini iop 'x + y' {x:40, y:2}`)
	if got != int64(42) {
		t.Fatalf("post-import binding: got %v, want 42", got)
	}
}

// TestMiniLangHostNoImportValueForm pins that a BOUND mini-language value
// needs no boru:minilang import at all — the value form is import-free (only
// the built-in kind sugar and the MiniLang namespace need the module).
func TestMiniLangHostNoImportValueForm(t *testing.T) {
	a := newCalcInstance(t)
	if got := runLast(t, a, `mini iop 'x + y' {x:40, y:2}`); got != int64(42) {
		t.Fatalf("no-import value form: got %v, want 42", got)
	}
}

// TestMiniLangHostIsolation confirms a kind registered on one instance does
// not leak into another — the registration is per-registry, not global.
func TestMiniLangHostIsolation(t *testing.T) {
	a := newCalcInstance(t) // has iop
	b, err := lang.New()    // does NOT
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	// a resolves iop.
	if got := runLast(t, a, `import "boru:minilang"  mini iop 'x + y' {x:1, y:1}`); got != int64(2) {
		t.Fatalf("instance a: got %v, want 2", got)
	}
	// b must NOT — unknown kind is an expansion-time error.
	if _, err := b.Run(`import "boru:minilang"  mini iop 'x + y' {x:1, y:1}`); err == nil {
		t.Fatal("instance b: expected mini_unknown_lang, got nil error")
	} else if !strings.Contains(err.Error(), "iop") {
		t.Fatalf("instance b: error should name the unknown kind, got: %v", err)
	}
}

// --- negative coverage: the contract is what gets declined -----------------

// TestMiniLangHostRuntimeErrors pins the handler's loud failures.
func TestMiniLangHostRuntimeErrors(t *testing.T) {
	a := newCalcInstance(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"unknown operator", `import "boru:minilang"  mini iop 'x ^ y' {x:1, y:2}`, "unknown operator"},
		{"division by zero", `import "boru:minilang"  mini iop 'x / y' {x:1, y:0}`, "division by zero"},
		{"modulo by zero", `import "boru:minilang"  mini iop 'x % y' {x:1, y:0}`, "division by zero"},
		{"undefined variable", `import "boru:minilang"  mini iop 'x + z' {x:1, y:2}`, "undefined variable"},
		{"malformed source", `import "boru:minilang"  mini iop 'x +' {x:1}`, "expected"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
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

// TestMiniLangHostRegistrationContract pins what NewMiniLangFn declines, and
// that a binding never shadows a built-in kind.
func TestMiniLangHostRegistrationContract(t *testing.T) {
	t.Run("empty name", func(t *testing.T) {
		if _, err := lang.NewMiniLangFn(lang.MiniLangSpec{Handler: calcSpec().Handler}); err == nil {
			t.Fatal("expected error for empty name")
		}
	})
	t.Run("nil handler rejected", func(t *testing.T) {
		if _, err := lang.NewMiniLangFn(lang.MiniLangSpec{Name: "iop"}); err == nil {
			t.Fatal("expected error for nil handler")
		}
	})
	t.Run("DefineValue rejects capitalised names", func(t *testing.T) {
		a, _ := lang.New()
		v, err := lang.NewMiniLangFn(calcSpec())
		if err != nil {
			t.Fatal(err)
		}
		if err := a.DefineValue("Iop", v); err == nil {
			t.Fatal("expected error for a capitalised value name")
		}
	})
	t.Run("built-in kind wins over a binding", func(t *testing.T) {
		// `re` is a built-in kind; a same-named binding never intercepts
		// `mini re` (the iop handler would reject the regex source).
		a, _ := lang.New()
		spec := calcSpec()
		spec.Name = "re"
		v, err := lang.NewMiniLangFn(spec)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.DefineValue("re", v); err != nil {
			t.Fatal(err)
		}
		got := runLast(t, a, `import "boru:minilang"  def m ("AbcD" mini re '[a-z]+')  m.fst.m`)
		if got != "bc" {
			t.Fatalf("built-in re should win over the binding: got %v, want bc", got)
		}
	})
	// (The lang_ prefix, duplicate-kind and collision rules died with the
	// namespace: a value binding is def-scoped — rebinding shadows, like
	// any def.)
}
