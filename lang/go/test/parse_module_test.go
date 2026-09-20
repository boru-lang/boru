package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// The boru:parse module lets a boru author DEFINE a custom parser — from an
// ABNF grammar, a declarative rule map, custom lex matchers, and semantic
// actions that build custom data types — and finalize it into a ParseLang
// Function VALUE (Parse.parser), run via the ordinary `parse` macro's value
// form. These tests pin each capability and its loud failures.

const parseImports = `import "boru:parse"  import "boru:parselang"  `

// runParse runs src on a fresh instance and returns the single result, failing
// on error.
func runParseLast(t *testing.T, src string) any {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := runReference(t, a, src)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("Run(%q): expected 1 result, got %d: %v", src, len(res), res)
	}
	return res[0]
}

// runParseErr runs src on a fresh instance and returns the error (nil if none).
func runParseErr(t *testing.T, src string) error {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	return runReferenceErr(t, a, src)
}

// TestParseABNFActionCustomType is the headline: an ABNF grammar plus a
// semantic action whose boru fn REPLACES the node with a custom boru value —
// proving the action bridge carries a user-built value to the parse result.
func TestParseABNFActionCustomType(t *testing.T) {
	build := parseImports + `
def g Parse.grammar
Parse.action g '@op:o:INC' ([nd:Any] => [{kind:'inc' delta:1}])
Parse.action g '@op:o:DEC' ([nd:Any] => [{kind:'dec' delta:-1}])
Parse.abnf g "op = \"inc\" / \"dec\"" {start:'op'}
def op (Parse.parser g)
end  `

	if got := runParseLast(t, build+`(parse op 'inc').kind`); got != "inc" {
		t.Errorf("(parse op 'inc').kind = %v, want inc", got)
	}
	if got := runParseLast(t, build+`(parse op 'inc').delta`); got != int64(1) {
		t.Errorf("(parse op 'inc').delta = %v, want 1", got)
	}
	if got := runParseLast(t, build+`(parse op 'dec').delta`); got != int64(-1) {
		t.Errorf("(parse op 'dec').delta = %v, want -1", got)
	}
}

// TestParseABNFScalarResult pins that an action can set the node to a plain
// scalar boru value (the simplest custom result) and it survives.
func TestParseABNFScalarResult(t *testing.T) {
	build := parseImports + `
def g Parse.grammar
Parse.action g '@op:o:INC' ([nd:Any] => [42])
Parse.abnf g "op = \"inc\" / \"dec\"" {start:'op'}
def opv (Parse.parser g)
end  `
	if got := runParseLast(t, build+`parse opv 'inc'`); got != int64(42) {
		t.Errorf("parse opv 'inc' = %v, want 42", got)
	}
}

// TestParseABNFBaseline pins that an ABNF grammar with NO actions still parses
// (the node is the converter's default tree) and that a malformed input is a
// loud parse error.
func TestParseABNFBaseline(t *testing.T) {
	build := parseImports + `
def g Parse.grammar
Parse.abnf g "op = \"inc\" / \"dec\"" {start:'op'}
def opb (Parse.parser g)
end  `
	// A well-formed input parses without error (the exact node shape is the
	// converter's; we only assert it succeeds and is non-error).
	if err := runParseErr(t, build+`parse opb 'inc'`); err != nil {
		t.Errorf("parse opb 'inc' should succeed, got %v", err)
	}
	// A token the grammar does not accept is a loud syntax error.
	if err := runParseErr(t, build+`parse opb 'zzz'`); err == nil {
		t.Error("parse opb 'zzz' should be a syntax error")
	} else if !strings.Contains(err.Error(), "parse_syntax_error") {
		t.Errorf("parse opb 'zzz' error = %v, want parse_syntax_error", err)
	}
}

// TestParseDesugarsToParseMacro proves a Parse-built parser is reachable both
// via the `parse` macro's value form and by invoking the bound fn directly —
// the value IS the standard-call target (`opd 'inc' {} end`).
func TestParseDesugarsToParseMacro(t *testing.T) {
	build := parseImports + `
def g Parse.grammar
Parse.action g '@op:o:INC' ([nd:Any] => [7])
Parse.abnf g "op = \"inc\" / \"dec\"" {start:'op'}
def opd (Parse.parser g)
end  `
	sugar := runParseLast(t, build+`parse opd 'inc'`)
	desugared := runParseLast(t, build+`opd 'inc' {} end`)
	if sugar != desugared || sugar != int64(7) {
		t.Fatalf("sugar=%v desugared=%v: must agree (=7)", sugar, desugared)
	}
}

// TestParseMatcherCoexists confirms a custom lex matcher coexists with an ABNF
// grammar: a matcher that declines (returns None) leaves normal lexing intact,
// so the grammar still parses.
func TestParseMatcherCoexists(t *testing.T) {
	build := parseImports + `
def g Parse.grammar
Parse.matcher g decline 1000000 ([rest:String] => [None])
Parse.abnf g "op = \"inc\" / \"dec\"" {start:'op'}
def opm (Parse.parser g)
end  `
	if err := runParseErr(t, build+`parse opm 'inc'`); err != nil {
		t.Errorf("declining matcher should not disturb the parse, got %v", err)
	}
}

// TestParseMatcherBadShape pins the matcher contract: a return that is neither
// None nor a {src:String …} map is a loud parse error (and never a panic).
func TestParseMatcherBadShape(t *testing.T) {
	build := parseImports + `
def g Parse.grammar
Parse.matcher g bad 1000000 ([rest:String] => [999])
Parse.abnf g "op = \"inc\" / \"dec\"" {start:'op'}
def opx (Parse.parser g)
end  `
	// Parse a token the grammar's own fixed literals do not claim, so the
	// custom matcher is consulted (and returns its bad shape).
	if err := runParseErr(t, build+`parse opx 'zzz'`); err == nil {
		t.Error("a bad-shape matcher result should be a loud error")
	} else if !strings.Contains(err.Error(), "matcher") {
		t.Errorf("error %q should mention the matcher contract", err.Error())
	}
}

// TestParseDeclarativeRuleInstalls pins that the declarative grammar form
// (Parse.rule with a {open:[…]} map) converts and installs without error — the
// "grammar as Map subtypes" path — and that a malformed spec is rejected.
func TestParseDeclarativeRuleInstalls(t *testing.T) {
	// A well-formed rule map converts and registers.
	ok := parseImports + `
def g Parse.grammar
Parse.rule g val {open:[{s:'#NR'}]}
def decl (Parse.parser g)
end  parse decl '1'`
	if err := runParseErr(t, ok); err != nil {
		t.Errorf("declarative rule should install and parse, got %v", err)
	}
	// A non-list `open` is rejected at conversion time.
	bad := parseImports + `
def g Parse.grammar
Parse.rule g val {open:'nope'}
def decl2 (Parse.parser g)
end`
	if err := runParseErr(t, bad); err == nil {
		t.Error("Parse.rule with a non-list open should error")
	} else if !strings.Contains(err.Error(), "parse_bad_rule") {
		t.Errorf("error %q should be parse_bad_rule", err.Error())
	}
}

// TestParseGrammarSubtypeExports pins that Parse.AltSpec / Parse.RuleSpec are
// exported as Map-subtype structural types ("grammar as Map subtypes") that
// document the declarative GrammarAltSpec / GrammarRuleSpec field schema.
func TestParseGrammarSubtypeExports(t *testing.T) {
	alt := runParseLast(t, parseImports+`Parse.AltSpec`)
	altStr, _ := alt.(string)
	for _, field := range []string{"s:String", "p:String", "a:Any"} {
		if !strings.Contains(altStr, field) {
			t.Errorf("Parse.AltSpec %q missing field %q", altStr, field)
		}
	}
	if got := runParseLast(t, parseImports+`typeof Parse.AltSpec`); got != "Map" {
		t.Errorf("typeof Parse.AltSpec = %v, want Map", got)
	}
	rule := runParseLast(t, parseImports+`Parse.RuleSpec`)
	ruleStr, _ := rule.(string)
	if !strings.Contains(ruleStr, "open:List") || !strings.Contains(ruleStr, "close:List") {
		t.Errorf("Parse.RuleSpec %q missing open/close fields", ruleStr)
	}
}

// TestParseParserContract pins the finalizer failures — there is no kind
// name any more (the parse kind namespace is fixed; a finalized parser is a
// ParseLang Function VALUE), so the contract is grammar-shaped: single-use
// finalization and loud non-grammar compile failures.
func TestParseParserContract(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			// The finalized parser closes over the grammar — a second
			// finalize must decline rather than mutate a live parser.
			"double finalize rejected",
			parseImports + `def g Parse.grammar  Parse.abnf g "x = \"a\"" {start:'x'}  def p (Parse.parser g)  Parse.parser g`,
			"parse_grammar_done",
		},
		{
			// An Integer where a Grammar is required is rejected at dispatch
			// (no signature matches) — a loud error, never a silent no-op.
			"non-grammar arg rejected",
			parseImports + `Parse.abnf 5 "x = \"a\""`,
			"no signature",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := runParseErr(t, c.src)
			if err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}

// TestParseMalformedABNF pins that a malformed ABNF grammar is a loud error at
// finalize time, not a panic.
func TestParseMalformedABNF(t *testing.T) {
	src := parseImports + `def g Parse.grammar  Parse.abnf g "= = = ="  def broke (Parse.parser g)`
	err := runParseErr(t, src)
	if err == nil {
		t.Fatal("malformed ABNF should error at finalize")
	}
	if !strings.Contains(err.Error(), "parse_bad_grammar") {
		t.Errorf("error %q should be parse_bad_grammar", err.Error())
	}
}

// TestParseDeclarativeStringRefAction pins that a declarative rule may
// reference an action registered via Parse.action by its string ref (a:'@hit')
// — the ref is wired into the grammar's Ref map, so Parse.parser succeeds
// rather than failing with an unresolved-ref error.
func TestParseDeclarativeStringRefAction(t *testing.T) {
	src := parseImports + `
def g Parse.grammar
Parse.action g '@hit' ([nd:Any] => [nd])
Parse.rule g doc {open:[{s:'#TX' a:'@hit'}]}
def srefkind (Parse.parser g)
end`
	if err := runParseErr(t, src); err != nil {
		t.Fatalf("string-ref action should register without error, got %v", err)
	}
}

// TestParseGrammarSingleUse pins the single-use contract: once a grammar is
// registered, a second register or any further builder word on the SAME
// grammar is a loud error (the finalized parser closes over the shared
// builder, so reuse must not silently mutate it).
func TestParseGrammarSingleUse(t *testing.T) {
	base := parseImports + `
def g Parse.grammar
Parse.abnf g 'op = "inc" / "dec"' {start:'op'}
def one (Parse.parser g)
end  `
	cases := []struct{ name, tail string }{
		{"second register", `def two (Parse.parser g)`},
		{"builder word after register", `Parse.abnf g 'x = "a"' {start:'x'}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := runParseErr(t, base+c.tail)
			if err == nil {
				t.Fatalf("%s: expected error reusing a registered grammar, got nil", c.name)
			}
			if !strings.Contains(err.Error(), "parse_grammar_done") {
				t.Fatalf("%s: error %q should be parse_grammar_done", c.name, err.Error())
			}
		})
	}
	// The first registered parser still works after the rejected reuse.
	if err := runParseErr(t, base+`parse one 'inc'`); err != nil {
		t.Errorf("the first registered kind should still parse, got %v", err)
	}
}
