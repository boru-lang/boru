package modules

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
	tabnasabnf "github.com/tabnas/abnf/go"
	tabnas "github.com/tabnas/parser/go"
)

// pcovReg builds a registry with the module resolver wired, mirroring
// production import resolution (log_test.go pattern).
func pcovReg(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)
	return r
}

// pcovRun parses and runs src, failing the test on error.
func pcovRun(t *testing.T, r *native.Registry, src string) []native.Value {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := native.NewTop(r).Run(vals)
	if err != nil {
		t.Fatalf("run: %v\n--- src ---\n%s", err, src)
	}
	return out
}

// pcovErr parses and runs src, returning the run error (nil if none).
func pcovErr(t *testing.T, r *native.Registry, src string) error {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, rerr := native.NewTop(r).Run(vals)
	return rerr
}

const pcovImports = `import "boru:parse"  import "boru:parselang"  `

// TestParseCovSpecWholeGrammar drives Parse.spec's happy path: options
// (fixed token + v), a ref action, and a rule referencing it — the whole
// grammar as one declarative map — then registers and parses.
func TestParseCovSpecWholeGrammar(t *testing.T) {
	r := pcovReg(t)
	out := pcovRun(t, r, pcovImports+`
def g Parse.grammar
Parse.spec g {
  v: 1
  options: {fixed:{token:{'#PL':'+'}}}
  ref: {'@hit': ([nd:Any] => [7])}
  rule: {val:{open:[{s:'#PL' a:'@hit'}]}}
}
def sfull (Parse.parser g)
end
parse sfull '+'`)
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %v", out)
	}
	n, err := out[0].AsConcreteInteger()
	if err != nil || n != 7 {
		t.Errorf("parse sfull '+' = %v, want 7 (err %v)", out[0], err)
	}
}

// TestParseCovSpecRefListAndAbnfForms covers the ref list-of-fns form
// (composeAltActions) and every abnf-section spelling: bare String, a
// {src:…} map, and a list of entries.
func TestParseCovSpecRefListAndAbnfForms(t *testing.T) {
	r := pcovReg(t)
	// Two actions on one ref run in order — the second wins the node.
	// (/v keeps the lambdas inert inside the evaluated list literal.)
	out := pcovRun(t, r, pcovImports+`
def g Parse.grammar
Parse.spec g {
  ref: {'@two': [([nd:Any] => [1])/v ([nd:Any] => [2])/v]}
  rule: {val:{open:[{s:'#NR' a:'@two'}]}}
}
def stwo (Parse.parser g)
end
parse stwo '5'`)
	if n, err := out[0].AsConcreteInteger(); err != nil || n != 2 {
		t.Errorf("composed ref actions: got %v, want 2 (err %v)", out[0], err)
	}

	// abnf as a {src:…} map entry.
	r2 := pcovReg(t)
	if err := pcovErr(t, r2, pcovImports+`
def g Parse.grammar
Parse.spec g {abnf:{src:'op = "inc" / "dec"' start:'op'}}
def smap (Parse.parser g)
end
parse smap 'inc'`); err != nil {
		t.Errorf("abnf map entry should register and parse: %v", err)
	}

	// abnf as a LIST of entries.
	r3 := pcovReg(t)
	if err := pcovErr(t, r3, pcovImports+`
def g Parse.grammar
Parse.spec g {abnf:[{src:'op = "inc" / "dec"' start:'op'}]}
def slist (Parse.parser g)
end
parse slist 'dec'`); err != nil {
		t.Errorf("abnf list entry should register and parse: %v", err)
	}
}

// TestParseCovSpecMatcherSection covers the matcher section of
// Parse.spec — collected pending matchers are applied at register.
func TestParseCovSpecMatcherSection(t *testing.T) {
	r := pcovReg(t)
	if err := pcovErr(t, r, pcovImports+`
def g Parse.grammar
Parse.spec g {matcher:{skip:{priority:1000000 fn:([rest:String] => [None])}}}
def smtch (Parse.parser g)
end
parse smtch '1'`); err != nil {
		t.Errorf("a declining spec matcher should leave normal lexing intact: %v", err)
	}
}

// TestParseCovSpecNegatives pins the loud shape failures of the whole-
// grammar map, one section at a time.
func TestParseCovSpecNegatives(t *testing.T) {
	cases := []struct{ name, spec, want string }{
		{"unknown section", `{bogus:1}`, "unknown section"},
		{"options non-map", `{options:'x'}`, "must be a map of"},
		{"v non-integer", `{v:'x'}`, "v must be an Integer"},
		{"v out of range", `{v:99}`, "schema version"},
		{"ref without @", `{ref:{hit:([nd:Any] => [1])}}`, "must start with '@'"},
		{"ref non-function", `{ref:{'@x':42}}`, "must be a Function"},
		{"abnf map missing src", `{abnf:{start:'op'}}`, "src:String"},
		{"abnf wrong type", `{abnf:42}`, "must be a String"},
		{"matcher entry non-map", `{matcher:{m:42}}`, "{priority fn} map"},
		{"matcher missing priority", `{matcher:{m:{fn:([x:String] => [None])}}}`, "priority must be an Integer"},
		{"matcher missing fn", `{matcher:{m:{priority:1}}}`, "fn must be a Function"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := pcovReg(t)
			err := pcovErr(t, r, pcovImports+`def g Parse.grammar  Parse.spec g `+c.spec)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// TestParseCovRuleAltFields drives altMapToSpec's field arms: b (Integer),
// g, n counters, c/u/k data maps with nested values, plus the
// list-of-actions form (an inline /v-parked fn followed by a '@ref').
//
// The `c` condition key is a DOTTED PATH onto a rule property, so a counter
// condition is `c:{'n.k1':0}`, not `c:{k1:0}`. The bare-name form parsed
// here until tabnas/parser v0.8.0 and did nothing at all: an unrooted path
// can never resolve, so the guard failed open forever. v0.8.0 rejects it
// while the grammar is built, which is what exposed this row as vacuous.
func TestParseCovRuleAltFields(t *testing.T) {
	r := pcovReg(t)
	out := pcovRun(t, r, pcovImports+`
def g Parse.grammar
Parse.action g '@hit' ([nd:Any] => [nd])
Parse.rule g val {open:[
  {s:'#NR' b:1 g:'gg' n:{k1:1} c:{'n.k1':0} u:{ux:1 deep:{d:2} lx:[1 'a' 2.5]} k:{kx:'s'}}
  {s:'#TX' a:[([nd:Any] => [5])/v '@hit']}
]}
def ralt (Parse.parser g)
end
parse ralt 'xy'`)
	if n, err := out[0].AsConcreteInteger(); err != nil || n != 5 {
		t.Errorf("parse ralt 'xy' = %v, want 5 (err %v)", out[0], err)
	}
}

// TestParseCovRuleBacktrackString covers the b:String arm — a backtrack
// function reference; an unknown ref is a loud register-time failure.
func TestParseCovRuleBacktrackString(t *testing.T) {
	r := pcovReg(t)
	err := pcovErr(t, r, pcovImports+`
def g Parse.grammar
Parse.rule g val {open:[{s:'#TX' b:'nosuch'}]}
def pbk (Parse.parser g)`)
	if err == nil || !strings.Contains(err.Error(), "unknown backtrack") {
		t.Errorf("unknown backtrack ref should fail register loudly, got %v", err)
	}
}

// TestParseCovRuleNegatives pins ruleMapToSpec / altMapToSpec compile failures.
func TestParseCovRuleNegatives(t *testing.T) {
	cases := []struct{ name, rule, want string }{
		{"alt non-map", `{open:[42]}`, "each alternate must be a map"},
		{"c non-map", `{open:[{s:'#NR' c:5}]}`, "condition must be a declarative map"},
		{"u non-map", `{open:[{s:'#NR' u:5}]}`, "custom props must be a map"},
		{"k non-map", `{open:[{s:'#NR' k:5}]}`, "propagated props must be a map"},
		{"a list bad element", `{open:[{s:'#NR' a:[42]}]}`, "each action must be a Function"},
		{"open non-list", `{open:'nope'}`, "must be a list of alternates"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := pcovReg(t)
			err := pcovErr(t, r, pcovImports+`def g Parse.grammar  Parse.rule g val `+c.rule)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// TestParseCovTokenAndInlineAction covers Parse.token (a fixed lexer
// token) plus an inline fn action on a declarative alternate.
func TestParseCovTokenAndInlineAction(t *testing.T) {
	r := pcovReg(t)
	out := pcovRun(t, r, pcovImports+`
def g Parse.grammar
Parse.token g '#ZZ' 'zz'
Parse.rule g val {open:[{s:'#ZZ' a:([nd:Any] => [3])}]}
def tk (Parse.parser g)
end
parse tk 'zz'`)
	if n, err := out[0].AsConcreteInteger(); err != nil || n != 3 {
		t.Errorf("parse tk 'zz' = %v, want 3 (err %v)", out[0], err)
	}
}

// TestParseCovMatcherMatches covers the full wrapMatcher happy path: a
// custom matcher that CLAIMS a prefix. The custom-tin variant registers
// a fresh token and gates a rule alternate on it; the default variant
// leaves tin at #TX.
func TestParseCovMatcherMatches(t *testing.T) {
	r := pcovReg(t)
	out := pcovRun(t, r, pcovImports+`
def g Parse.grammar
Parse.matcher g zed 1000000 ([rest:String] => [if (rest eq 'zz') [{src:'zz' tin:'#ZED' val:9}] [None]])
Parse.rule g val {open:[{s:'#ZED' a:([nd:Any] => ['ZED-HIT'])}]}
def zk (Parse.parser g)
end
parse zk 'zz'`)
	s, err := out[0].AsConcreteString()
	if err != nil || s != "ZED-HIT" {
		t.Errorf("parse zk 'zz' = %v, want ZED-HIT (err %v)", out[0], err)
	}

	// Default token (#TX), value riding the token.
	r2 := pcovReg(t)
	out2 := pcovRun(t, r2, pcovImports+`
def g Parse.grammar
Parse.matcher g zed 1000000 ([rest:String] => [if (rest eq 'zz') [{src:'zz' val:'ZED'}] [None]])
Parse.rule g val {open:[{s:'#TX' a:([nd:Any] => ['TX-HIT'])}]}
def zt (Parse.parser g)
end
parse zt 'zz'`)
	s2, err2 := out2[0].AsConcreteString()
	if err2 != nil || s2 != "TX-HIT" {
		t.Errorf("parse zt 'zz' = %v, want TX-HIT (err %v)", out2[0], err2)
	}
}

// TestParseCovMatcherContract pins wrapMatcher's compile failures: a map without
// src, a src that is not a prefix, and a callback that raises.
func TestParseCovMatcherContract(t *testing.T) {
	cases := []struct{ name, matcher, want string }{
		{"missing src", `([rest:String] => [{tin:'#TX'}])`, "non-empty 'src'"},
		{"src not a prefix", `([rest:String] => [{src:'qq'}])`, "not a prefix"},
		{"non-map result", `([rest:String] => [999])`, "must return None or a"},
		{"callback raises", `([rest:String] => [raise "boom"])`, "parse_syntax_error"},
		{"wrong arity callback", `([a:String b:String] => [None])`, "no matching callback signature"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := pcovReg(t)
			err := pcovErr(t, r, pcovImports+`
def g Parse.grammar
Parse.matcher g m 1000000 `+c.matcher+`
Parse.abnf g "op = \"inc\"" {start:'op'}
def mk (Parse.parser g)
end
parse mk 'zzz'`)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// TestParseCovActionRaises pins wrapAction's error capture: an action
// callback that raises surfaces as a loud parse error.
func TestParseCovActionRaises(t *testing.T) {
	r := pcovReg(t)
	err := pcovErr(t, r, pcovImports+`
def g Parse.grammar
Parse.action g '@op:o:INC' ([nd:Any] => [raise "kaput"])
Parse.abnf g "op = \"inc\" / \"dec\"" {start:'op'}
def ar (Parse.parser g)
end
parse ar 'inc'`)
	if err == nil {
		t.Fatal("expected the raising action to fail the parse")
	}
	if !strings.Contains(err.Error(), "parse_syntax_error") {
		t.Errorf("error %q should be parse_syntax_error", err.Error())
	}
}

// TestParseCovActionRefContract pins Parse.action's '@' prefix rule.
func TestParseCovActionRefContract(t *testing.T) {
	r := pcovReg(t)
	err := pcovErr(t, r, pcovImports+`
def g Parse.grammar
Parse.action g 'hit' ([nd:Any] => [nd])`)
	if err == nil || !strings.Contains(err.Error(), "must start with '@'") {
		t.Errorf("a ref without @ should be declined, got %v", err)
	}
}

// TestParseCovMultipleActionsCompose covers composeAltActions via two
// Parse.action registrations on the same mark ref — both run, in order.
func TestParseCovMultipleActionsCompose(t *testing.T) {
	r := pcovReg(t)
	out := pcovRun(t, r, pcovImports+`
def g Parse.grammar
Parse.action g '@op:o:INC' ([nd:Any] => [1])
Parse.action g '@op:o:INC' ([nd:Any] => [nd 10 add])
Parse.abnf g "op = \"inc\"" {start:'op'}
def comp (Parse.parser g)
end
parse comp 'inc'`)
	if n, err := out[0].AsConcreteInteger(); err != nil || n != 11 {
		t.Errorf("composed actions = %v, want 11 (first sets 1, second adds 10; err %v)", out[0], err)
	}
}

// TestParseCovAbnfOpts covers abnfOptsFrom's full option set.
func TestParseCovAbnfOpts(t *testing.T) {
	r := pcovReg(t)
	if err := pcovErr(t, r, pcovImports+`
def g Parse.grammar
Parse.abnf g "op = \"inc\" / \"dec\"" {start:'op' tag:'t1' builtins:true marks:false}
def ao (Parse.parser g)
end
parse ao 'inc'`); err != nil {
		t.Errorf("abnf with the full opts map should register and parse: %v", err)
	}
}

// TestParseCovSingleUsePerWord pins ensureOpen on every builder word: a
// registered grammar declines all further mutation.
func TestParseCovSingleUsePerWord(t *testing.T) {
	base := pcovImports + `
def g Parse.grammar
Parse.abnf g 'op = "inc"' {start:'op'}
def su (Parse.parser g)
end
`
	tails := []struct{ name, tail string }{
		{"abnf", `Parse.abnf g 'x = "a"' {start:'x'}`},
		{"spec", `Parse.spec g {v:1}`},
		{"rule", `Parse.rule g val {open:[{s:'#NR'}]}`},
		{"token", `Parse.token g '#ZZ' 'zz'`},
		{"matcher", `Parse.matcher g m 1000000 ([x:String] => [None])`},
		{"action", `Parse.action g '@a' ([nd:Any] => [nd])`},
		{"second register", `def su2 (Parse.parser g)`},
	}
	for _, c := range tails {
		t.Run(c.name, func(t *testing.T) {
			r := pcovReg(t)
			err := pcovErr(t, r, base+c.tail)
			if err == nil {
				t.Fatalf("%s after register should error", c.name)
			}
			if !strings.Contains(err.Error(), "parse_grammar_done") {
				t.Fatalf("error %q should be parse_grammar_done", err.Error())
			}
		})
	}
}

// TestParseCovRegisterFailures pins register-time compile failures: a broken
// deferred grammar step and a collision with an existing kind.
func TestParseCovParserFailures(t *testing.T) {
	r := pcovReg(t)
	err := pcovErr(t, r, pcovImports+`def g Parse.grammar  Parse.abnf g "= = ="  def broke (Parse.parser g)`)
	if err == nil || !strings.Contains(err.Error(), "parse_bad_grammar") {
		t.Errorf("malformed ABNF should fail the finalize with parse_bad_grammar, got %v", err)
	}

	// A Parse.spec options step whose token regexp cannot compile fails at
	// the same replay (the deferred Grammar(gs) call's error arm).
	rBad := pcovReg(t)
	err = pcovErr(t, rBad, pcovImports+`def g Parse.grammar  Parse.spec g {options:{match:{token:{'#BX':'@/[[/'}}}}  def broke (Parse.parser g)`)
	if err == nil || !strings.Contains(err.Error(), "parse_bad_grammar") {
		t.Errorf("an invalid spec token regexp should fail the finalize with parse_bad_grammar, got %v", err)
	}

	// There is no kind-name collision any more — the parser is a def-scoped
	// VALUE, and a built-in kind simply WINS the `parse <name>` lookup: the
	// binding is legal, the sugar still reaches the built-in json.
	r2 := pcovReg(t)
	out := pcovRun(t, r2, pcovImports+`def g Parse.grammar  Parse.abnf g 'x = "a"' {start:'x'}  def json (Parse.parser g)  end  (parse json '{"a":1}') get 'a'`)
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %v", out)
	}
	if n, err := out[0].AsConcreteInteger(); err != nil || n != 1 {
		t.Errorf("a def named json must not shadow the built-in kind: got %v (err %v)", out[0], err)
	}
}

// TestParseCovSyntaxError pins the registered parser's loud rejection of
// malformed input (parseHandler's perr arm).
func TestParseCovSyntaxError(t *testing.T) {
	r := pcovReg(t)
	err := pcovErr(t, r, pcovImports+`
def g Parse.grammar
Parse.abnf g 'op = "inc"' {start:'op'}
def se (Parse.parser g)
end
parse se 'nope'`)
	if err == nil || !strings.Contains(err.Error(), "parse_syntax_error") {
		t.Errorf("malformed input should be parse_syntax_error, got %v", err)
	}
}

// TestParseCovSpecReturnsCheckMode drives Parse.spec's check-mode hook
// directly: a concrete-but-bad spec map surfaces a diagnostic; a good
// map surfaces none.
func TestParseCovSpecReturnsCheckMode(t *testing.T) {
	r := pcovReg(t)
	done := r.Check.Begin()
	defer done()

	bad := native.NewOrderedMap()
	bad.Set("bogus", native.NewInteger(1))
	out := parseSpecReturns([]native.Value{native.NewInteger(0), native.NewMap(bad)}, r)
	if len(out) != 0 {
		t.Errorf("parseSpecReturns should return no values, got %v", out)
	}
	if len(r.Check.Diagnostics) == 0 {
		t.Error("a bad spec map should add a check diagnostic")
	}
	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "parse_bad_spec" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a parse_bad_spec diagnostic, got %+v", r.Check.Diagnostics)
	}

	// A well-formed spec adds nothing new.
	before := len(r.Check.Diagnostics)
	good := native.NewOrderedMap()
	good.Set("v", native.NewInteger(1))
	parseSpecReturns([]native.Value{native.NewInteger(0), native.NewMap(good)}, r)
	if len(r.Check.Diagnostics) != before {
		t.Errorf("a valid spec map should add no diagnostics, got %+v", r.Check.Diagnostics[before:])
	}
}

// TestParseCovSpecReturnsLenient covers applySpecMap's lenient arms: a
// spec whose sections are all non-concrete carriers is silently skipped
// by the check-mode dry pass — no diagnostics.
func TestParseCovSpecReturnsLenient(t *testing.T) {
	r := pcovReg(t)
	done := r.Check.Begin()
	defer done()

	spec := native.NewOrderedMap()
	spec.Set("options", native.NewTypeLiteral(native.TMap))
	spec.Set("v", native.NewTypeLiteral(native.TInteger))
	spec.Set("abnf", native.NewTypeLiteral(native.TString))
	spec.Set("matcher", native.NewTypeLiteral(native.TMap))
	// Concrete sections whose ENTRIES are non-concrete are also skipped.
	refs := native.NewOrderedMap()
	refs.Set("@x", native.NewTypeLiteral(native.TFunction))
	spec.Set("ref", native.NewMap(refs))
	rules := native.NewOrderedMap()
	rules.Set("r1", native.NewTypeLiteral(native.TMap))
	spec.Set("rule", native.NewMap(rules))

	parseSpecReturns([]native.Value{native.NewInteger(0), native.NewMap(spec)}, r)
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("non-concrete sections must be skipped leniently, got %+v", r.Check.Diagnostics)
	}
}

// TestParseCovParserReturns covers parse-parser's check hook: the grammar
// carrier is never concrete under analysis, so the value is an abstract —
// PLAIN, not dynamic (the declared return type is exact) — Function carrier.
func TestParseCovParserReturns(t *testing.T) {
	r := pcovReg(t)
	done := r.Check.Begin()
	defer done()
	out := parseParserReturns([]native.Value{native.NewInteger(0)}, r)
	if len(out) != 1 || out[0].Dynamic || native.IsConcrete(out[0]) ||
		!out[0].Parent.ConformsTo(native.TFunction) {
		t.Errorf("expected one plain Function carrier, got %v", out)
	}
}

// TestParseCovAsParseGrammarDirect pins the unwrapper's compile failure of a
// non-grammar value with the guiding hint.
func TestParseCovAsParseGrammarDirect(t *testing.T) {
	r := pcovReg(t)
	_, err := asParseGrammar(native.NewInteger(5), "Parse.abnf", r)
	if err == nil {
		t.Fatal("expected an error for a non-grammar value")
	}
	if !strings.Contains(err.Error(), "expected a Parse grammar") {
		t.Errorf("error %q should explain the grammar contract", err.Error())
	}
}

// TestParseCovSetErrFirstWins pins setErr's first-error-wins contract.
func TestParseCovSetErrFirstWins(t *testing.T) {
	g := &parseGrammar{}
	e1, e2 := errors.New("first"), errors.New("second")
	g.setErr(e1)
	g.setErr(e2)
	if g.firstErr != e1 {
		t.Errorf("firstErr = %v, want the first recorded error", g.firstErr)
	}
}

// TestParseCovComposeAltActionsDirect covers both arms of
// composeAltActions: single-action passthrough and the ordered loop.
func TestParseCovComposeAltActionsDirect(t *testing.T) {
	var order []int
	mk := func(n int) tabnasabnf.ActionFn {
		return func(_ *tabnas.Rule, _ *tabnas.Context) { order = append(order, n) }
	}
	one := composeAltActions([]tabnasabnf.ActionFn{mk(1)})
	one(nil, nil)
	two := composeAltActions([]tabnasabnf.ActionFn{mk(2), mk(3)})
	two(nil, nil)
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("actions ran out of order: %v", order)
	}
}

// TestParseCovSpecAnyValueDirect covers specAnyValue / specAnyMap /
// specDataMap conversions at every depth.
func TestParseCovSpecAnyValueDirect(t *testing.T) {
	inner := native.NewOrderedMap()
	inner.Set("i", native.NewInteger(3))
	m := native.NewOrderedMap()
	m.Set("n", native.NewInteger(1))
	m.Set("s", native.NewString("x"))
	m.Set("sub", native.NewMap(inner))
	m.Set("lst", native.NewList([]native.Value{native.NewInteger(2), native.NewString("y")}))

	out, ok := specDataMap(native.NewMap(m))
	if !ok {
		t.Fatal("specDataMap should accept a concrete map")
	}
	if out["n"] != 1 {
		t.Errorf("n = %#v, want int 1", out["n"])
	}
	if out["s"] != "x" {
		t.Errorf("s = %#v, want \"x\"", out["s"])
	}
	sub, _ := out["sub"].(map[string]any)
	if sub == nil || sub["i"] != 3 {
		t.Errorf("sub = %#v, want nested map with i=3", out["sub"])
	}
	lst, _ := out["lst"].([]any)
	if len(lst) != 2 || lst[0] != 2 || lst[1] != "y" {
		t.Errorf("lst = %#v, want [2 y]", out["lst"])
	}

	if _, ok := specDataMap(native.NewInteger(9)); ok {
		t.Error("specDataMap should decline a non-map")
	}
}

// TestParseCovIsCallableValueDirect pins the callable probe on both
// function shapes and a plain value.
func TestParseCovIsCallableValueDirect(t *testing.T) {
	if isCallableValue(native.NewInteger(1)) {
		t.Error("an Integer is not callable")
	}
	fn := native.NewFunction(native.FnDefInfo{Name: "f"})
	if !isCallableValue(fn) {
		t.Error("a Function value is callable")
	}
	fd := native.NewFunction(native.FnDefInfo{Name: "g"})
	if !isCallableValue(fd) {
		t.Error("an FnDef value is callable")
	}
}

// TestParseCovTmKeysDirect pins the nil-tolerant key walk.
func TestParseCovTmKeysDirect(t *testing.T) {
	if got := tmKeys(nil); got != nil {
		t.Errorf("tmKeys(nil) = %v, want nil", got)
	}
	m := native.NewOrderedMap()
	m.Set("a", native.NewInteger(1))
	if got := tmKeys(m); len(got) != 1 || got[0] != "a" {
		t.Errorf("tmKeys = %v, want [a]", got)
	}
}
