package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// The tabnas parser family (github.com/tabnas/*) all ships built-in with
// boru:parselang, reached the same way as ini: import the module, then
// `parse <kind> '<text>'`. These tests cover the user-facing sugar for each
// kind plus the kinds listing and a couple of failure modes.

// TestParseLangTabnasFamily pins one decode per built-in parser kind through
// the `parse <kind>` sugar, across the three top-level shapes (object, list,
// scalar field).
func TestParseLangTabnasFamily(t *testing.T) {
	cases := []struct {
		kind, expr string
		want       any
	}{
		{"json", `(parse json '{"a":1,"b":"hi"}') get 'b'`, "hi"},
		{"jsonic", `(parse jsonic '{a:1, b:"hi"}') get 'b'`, "hi"},
		{"json5", `(parse json5 '{a:1, b:0x10}') get 'b'`, int64(16)},
		{"jsonc", `(parse jsonc '{"a":1, "b":2}') get 'b'`, int64(2)},
		{"csv", `((parse csv 'a,b\n1,2') get 0) get 1`, "b"},
		{"toml", `(parse toml 'b = "hi"') get 'b'`, "hi"},
		{"yaml", `(parse yaml 'b: hi') get 'b'`, "hi"},
		// XML now decodes to a Node/Xml element (not a generic Map), so it
		// renders back to well-formed XML — the same value a literal yields.
		{"xml", `parse xml '<r><a>1</a></r>'`, "<r><a>1</a></r>"},
		{"zon", `(parse zon '.{ .a = 1, .b = "hi" }') get 'b'`, "hi"},
		// Markdown decodes to the mdast-adjacent AST document, so the path
		// walks node keys: document children -> the paragraph -> its text.
		{"markdown", `(((((parse markdown '# T\n\nhello world') get 'children') get 1) get 'children') get 0) get 'value'`, "hello world"},
		{"feed", `(parse feed '<rss version="2.0"><channel><title>T</title></channel></rss>') get 'format'`, "atom"},
	}
	imp := `import "boru:parselang"  `
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if got := runLast(t, a, imp+c.expr); got != c.want {
				t.Errorf("%s: got %v (%T), want %v", c.kind, got, got, c.want)
			}
		})
	}
}

// TestParseLangTabnasDesugar proves the sugar matches the desugared standard
// call for a representative kind from each shape family.
func TestParseLangTabnasDesugar(t *testing.T) {
	imp := `import "boru:parselang"  `
	cases := []struct{ kind, sugar, desugared string }{
		{"json", `(parse json '{"a":1}') get 'a'`, `(ParseLang.parse_json '{"a":1}' {} end) get 'a'`},
		{"yaml", `(parse yaml 'b: hi') get 'b'`, `(ParseLang.parse_yaml 'b: hi' {} end) get 'b'`},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			sugar := runLast(t, a, imp+c.sugar)
			desugared := runLast(t, a, imp+c.desugared)
			if sugar != desugared {
				t.Fatalf("%s: sugar=%v desugared=%v must agree", c.kind, sugar, desugared)
			}
		})
	}
}

// TestParseLangTabnasOpts proves the middle `parse <kind> <opts> <source>`
// map is forwarded to a jsonic-plugin kind: a custom CSV field separator
// makes ';' split columns where the default ',' would not.
func TestParseLangTabnasOpts(t *testing.T) {
	imp := `import "boru:parselang"  `
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	// With separation ';', the header row 'a;b' becomes two fields, so
	// row 0 field 1 is "b". Without the opt the whole 'a;b' is one field.
	got := runLast(t, a, imp+`((parse csv {field:{separation:';'}} 'a;b\n1;2') get 0) get 1`)
	if got != "b" {
		t.Errorf("csv opts: got %v (%T), want \"b\"", got, got)
	}
	// Negative: the same source under the default ',' separator keeps 'a;b'
	// as a single field, so field 1 is out of range / the row has one cell.
	def := runLast(t, a, imp+`(parse csv 'a;b\n1;2') get 0`)
	if s := fmt.Sprintf("%v", def); !strings.Contains(s, "a;b") {
		t.Errorf("csv default sep: row 0 = %v, want a single 'a;b' field", def)
	}
}

// TestParseLangTabnasKinds confirms every built-in kind is listed by
// ParseLang.kinds with no host registration.
func TestParseLangTabnasKinds(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := runReference(t, a, `import "boru:parselang"  ParseLang.kinds`)
	if err != nil {
		t.Fatalf("kinds: %v", err)
	}
	listed := fmt.Sprintf("%v", res[0])
	for _, kind := range []string{
		"ini", "json", "jsonic", "json5", "jsonc", "csv",
		"toml", "yaml", "xml", "zon", "markdown", "feed",
	} {
		if !strings.Contains(listed, kind) {
			t.Errorf("ParseLang.kinds %v should contain %q", listed, kind)
		}
	}
}

// TestParseLangTabnasSyntaxErrors pins the loud failure: a malformed source
// raises parse_syntax_error rather than returning a silent none.
func TestParseLangTabnasSyntaxErrors(t *testing.T) {
	imp := `import "boru:parselang"  `
	cases := []struct{ name, src string }{
		{"json", imp + `parse json '{not valid'`},
		{"toml", imp + `parse toml '= = ='`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if _, err := a.Run(c.src); err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			} else if !strings.Contains(err.Error(), "parse_syntax_error") {
				t.Fatalf("%s: error %q should be parse_syntax_error", c.name, err.Error())
			}
		})
	}
}
