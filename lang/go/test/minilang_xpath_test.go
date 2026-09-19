package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// The `xp` mini-language (github.com/antchfx/xpath) queries a Node/Xml document
// — the stack subject, an <tag>…</tag> literal — with XPath. It ships built-in
// with boru:minilang and returns a List of results, the same shape as jp / jq.

const xpImp = `import "boru:minilang"  `

// TestMiniXPathResults pins the result projection for each XPath result kind: a
// node-set of elements (→ Node/Xml values), text and attribute nodes (→
// Strings), and the scalar count/string/boolean function results.
func TestMiniXPathResults(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"elements", `<r><a>1</a><a>2</a></r> mini xp '//a'`, "[<a>1</a> <a>2</a>]"},
		{"text nodes", `<r><a>1</a><a>2</a></r> mini xp '//a/text()'`, "['1' '2']"},
		{"attribute node", `<r id="7"><a/></r> mini xp '//@id'`, "['7']"},
		{"string fn", `<r id="7"><a/></r> mini xp 'string(//@id)'`, "['7']"},
		{"count integral", `<r><a/><a/><a/></r> mini xp 'count(//a)'`, "[3]"},
		{"sum fractional", `<r><p>1.5</p><p>2.0</p></r> mini xp 'sum(//p)'`, "[3.5]"},
		{"boolean", `<r><a/><a/></r> mini xp 'count(//a) > 1'`, "[true]"},
		{"positional predicate", `<lib><book><t>A</t></book><book><t>B</t></book></lib> mini xp '//book[2]/t/text()'`, "['B']"},
		{"nested descendant", `<a><b><c>deep</c></b></a> mini xp '//c/text()'`, "['deep']"},
		{"no match empty", `<r><a/></r> mini xp '//z'`, "[]"},
		{"attribute predicate", `<r><a k="x"/><a k="y"/></r> mini xp 'string(//a[@k="y"]/@k)'`, "['y']"},
		// FlexXml (the mutable element) conforms to Xml, so xp accepts it too.
		{"flex subject", `(flex <r><a>1</a></r>) mini xp '//a/text()'`, "['1']"},
		// An element match composes with the xml-* accessor words.
		{"compose xml-text", `def h (<r><a>1</a><a>2</a></r> mini xp '//a') end  xml-text (h.1)`, "2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			res, err := runReference(t, a, xpImp+c.src)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if got := strings.TrimSpace(fmt.Sprintf("%v", res[0])); got != c.want {
				t.Errorf("%s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestMiniXPathDesugar proves `mini xp` is sugar for the standard call.
func TestMiniXPathDesugar(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	sugar := fmt.Sprintf("%v", mustRun(t, a, xpImp+`<r><a/></r> mini xp '//a'`))
	desugared := fmt.Sprintf("%v", mustRun(t, a, xpImp+`<r><a/></r> MiniLang.lang_xp '//a' {} end`))
	if sugar != desugared {
		t.Fatalf("xp sugar=%q desugared=%q must agree", sugar, desugared)
	}
}

// TestMiniXPathErrors pins the loud failures: a malformed XPath (parse error)
// and a non-Xml document (the typed signature refuses to dispatch).
func TestMiniXPathErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"bad xpath", `<r/> mini xp '//['`, "mini_parse_error"},
		{"unterminated fn", `<r/> mini xp 'count('`, "mini_parse_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if _, err := a.Run(xpImp + c.src); err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q should contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}

// TestMiniXPathNonXmlSubject pins the partial-function semantics for a
// mismatched subject: a non-Xml value does not match xp's signature, so
// the expansion's partial stays a VALUE next to the untouched subject
// instead of raising (store it, or feed a Node/Xml). Interpreter-level
// (see minilang_partial_test.go for why the parked shapes are not spec
// rows).
func TestMiniXPathNonXmlSubject(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	got, err := runReference(t, a, xpImp+`{a:1} mini xp '//a' {} typeof`)
	if err != nil {
		t.Fatalf("expected the partial to stay data, got error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected [subject, typeof(partial)] residual, got %v", got)
	}
	if s, _ := got[1].(string); s != "Function" {
		t.Fatalf("expected a Function partial on top, got %v", got[1])
	}
}

// TestMiniXPathInKinds confirms xp is listed by MiniLang.kinds.
func TestMiniXPathInKinds(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	listed := fmt.Sprintf("%v", mustRun(t, a, xpImp+`MiniLang.kinds`))
	if !strings.Contains(listed, "xp") {
		t.Errorf("MiniLang.kinds %q should contain %q", listed, "xp")
	}
}
