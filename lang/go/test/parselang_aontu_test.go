package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// The aontu PARSER ships built-in with boru:parselang. aontu
// (github.com/rjrodger/aontu) is a CUE-inspired unification config dialect;
// AontuParse wraps the upstream Go port (the repo's go/ module), wired in as
// a built-in `parse` kind. Like the tabnas family, no host registration is
// needed — importing the module is enough for `parse aontu <text>` to parse,
// unify and generate a Node of Maps and Lists.

const aontuImp = `import "boru:parselang"  `

// aStr runs src and renders the single result to a string, so an Integer
// result (returned as an int64 by lang.Run) compares cleanly against the
// expected text.
func aStr(t *testing.T, a *lang.Boru, src string) string {
	t.Helper()
	return fmt.Sprintf("%v", runLast(t, a, src))
}

// TestParseLangAontuScalars pins the scalar decodes and the colon-chain
// nesting that build a Map.
func TestParseLangAontuScalars(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	cases := []struct{ src, expr, want string }{
		{`a:1 b:2`, `get 'b'`, "2"},
		{`x:hello`, `get 'x'`, "hello"},
		{`x:"hi there"`, `get 'x'`, "hi there"},
		{`x:true`, `get 'x'`, "true"},
		{`x:-5`, `get 'x'`, "-5"},
		{`x:1.5`, `get 'x'`, "1.5"},
		{`a:b:c:1`, `(((parse aontu 'a:b:c:1') get 'a') get 'b') get 'c'`, "1"},
	}
	for _, c := range cases {
		prog := fmt.Sprintf("%s(parse aontu '%s') %s", aontuImp, c.src, c.expr)
		if c.src == `a:b:c:1` {
			prog = aontuImp + c.expr // already fully grouped
		}
		if got := aStr(t, a, prog); got != c.want {
			t.Errorf("parse aontu '%s' %s = %v, want %v", c.src, c.expr, got, c.want)
		}
	}
}

// TestParseLangAontuLists pins list decoding (comma- and space-separated)
// and mixed element types.
func TestParseLangAontuLists(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if got := aStr(t, a, aontuImp+`((parse aontu 'a:[1,2,3]') get 'a') get 1`); got != "2" {
		t.Errorf("comma list: got %v, want 2", got)
	}
	if got := aStr(t, a, aontuImp+`((parse aontu 'a:[1 2 3]') get 'a') get 2`); got != "3" {
		t.Errorf("space list: got %v, want 3", got)
	}
	if got := aStr(t, a, aontuImp+`((parse aontu 'a:[1,"two",true]') get 'a') get 1`); got != "two" {
		t.Errorf("mixed list: got %v, want two", got)
	}
}

// TestParseLangAontuUnification pins duplicate-key deep merge and the `&`
// operator — the heart of aontu.
func TestParseLangAontuUnification(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	// duplicate-key merge of two maps
	if got := aStr(t, a, aontuImp+`((parse aontu 'a:{b:1} a:{c:2}') get 'a') get 'c'`); got != "2" {
		t.Errorf("dup-key merge: got %v, want 2", got)
	}
	// `&` unification of two maps (parenthesised so & scopes to the value)
	if got := aStr(t, a, aontuImp+`((parse aontu 'm:({b:2} & {c:3})') get 'm') get 'c'`); got != "3" {
		t.Errorf("& merge: got %v, want 3", got)
	}
	// equal scalars collapse under unification
	if got := aStr(t, a, aontuImp+`(parse aontu 'v:(5 & 5)') get 'v'`); got != "5" {
		t.Errorf("scalar collapse: got %v, want 5", got)
	}
}

// TestParseLangAontuKinds pins kind constraints: a kind unifies with a
// matching scalar to yield the scalar.
func TestParseLangAontuKinds(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if got := aStr(t, a, aontuImp+`(parse aontu 'a:number a:5') get 'a'`); got != "5" {
		t.Errorf("number kind: got %v, want 5", got)
	}
	if got := aStr(t, a, aontuImp+`(parse aontu 'a:string a:"text"') get 'a'`); got != "text" {
		t.Errorf("string kind: got %v, want text", got)
	}
	if got := aStr(t, a, aontuImp+`typeof ((parse aontu 'a:integer a:5') get 'a')`); got != "Integer" {
		t.Errorf("integer kind type: got %v, want Integer", got)
	}
}

// TestParseLangAontuReferences pins absolute and relative references,
// including list indexing.
func TestParseLangAontuReferences(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if got := aStr(t, a, aontuImp+`(parse aontu 'a:1 b:$.a') get 'b'`); got != "1" {
		t.Errorf("absolute ref: got %v, want 1", got)
	}
	if got := aStr(t, a, aontuImp+`(parse aontu 'a:[10,20,30] b:$.a.1') get 'b'`); got != "20" {
		t.Errorf("list-index ref: got %v, want 20", got)
	}
	// relative ref resolves a sibling key at the current map level
	if got := aStr(t, a, aontuImp+`(parse aontu 'a:1 b:.a') get 'b'`); got != "1" {
		t.Errorf("relative ref: got %v, want 1", got)
	}
}

// TestParseLangAontuDesugar proves `parse aontu` is sugar for the standard
// ParseLang.parse_aontu call.
func TestParseLangAontuDesugar(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	sugar := aStr(t, a, aontuImp+`(parse aontu 'a:1') get 'a'`)
	desugared := aStr(t, a, aontuImp+`(ParseLang.parse_aontu 'a:1' {} end) get 'a'`)
	if sugar != desugared || sugar != "1" {
		t.Fatalf("sugar=%v desugared=%v: parse must desugar to the standard call", sugar, desugared)
	}
}

// TestParseLangAontuInKinds confirms aontu is listed by ParseLang.kinds with
// no host registration — it is built in.
func TestParseLangAontuInKinds(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := runReference(t, a, aontuImp+`ParseLang.kinds`)
	if err != nil {
		t.Fatalf("kinds: %v", err)
	}
	if kinds := fmt.Sprintf("%v", res[0]); !strings.Contains(kinds, "aontu") {
		t.Errorf("kinds %v should contain aontu", kinds)
	}
}

// TestParseLangAontuErrors pins the loud failures: a unification conflict, an
// unresolved reference, an incomplete kind, and a malformed source.
func TestParseLangAontuErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"scalar conflict", aontuImp + `parse aontu 'a:1 a:2'`, "parse_syntax_error"},
		{"kind conflict", aontuImp + `parse aontu 'a:number a:"x"'`, "parse_syntax_error"},
		{"unresolved ref", aontuImp + `parse aontu 'b:$.nope'`, "parse_syntax_error"},
		{"incomplete kind", aontuImp + `parse aontu 'a:number'`, "parse_syntax_error"},
		{"unresolved sibling", aontuImp + `parse aontu 'a:{x:1 y:.z}'`, "parse_syntax_error"},
		{"file deferred", aontuImp + `parse aontu {file:'x.aontu'}`, "parse_file_unsupported"},
		{"bad source map", aontuImp + `parse aontu {nope:1}`, "parse_bad_source"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if _, err := a.Run(c.src); err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}

// TestParseLangAontuKindNotShadowable confirms aontu occupies its kind slot:
// the built-in kind wins over a same-named value binding, so a host parser
// bound as `aontu` never intercepts `parse aontu`. (The calc parser would
// reject 'a:1' — only the built-in decodes it.)
func TestParseLangAontuKindNotShadowable(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	v, err := lang.NewParseLangFn(lang.ParseLangSpec{
		Name:    "aontu",
		Returns: []*lang.Type{lang.TMap},
		Handler: calcParserSpec().Handler,
	})
	if err != nil {
		t.Fatalf("NewParseLangFn: %v", err)
	}
	if err := a.DefineValue("aontu", v); err != nil {
		t.Fatalf("DefineValue: %v", err)
	}
	if got := aStr(t, a, aontuImp+`(parse aontu 'a:1') get 'a'`); got != "1" {
		t.Fatalf("built-in aontu should win over the binding: got %v, want 1", got)
	}
}
