package parser

// Seam-5b wave E: unit tests for grammar/parse/xml_literal blocks no suite
// reached. The jsonic grammar actions and lex matchers are ordinary Go
// closures; where a guard arm cannot be provoked through Parse (nil parent
// rules, matcher invocations without a rule), the closure is retrieved from
// the configured grammar (RuleSpec.Actions / AltSpec fields /
// Config.CustomMatchers) and invoked directly with a synthetic Rule — the
// in-package equivalent of calling an unexported function.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	jsonic "github.com/tabnas/jsonic/go"
)

// s5beGrammar builds the same configured jsonic instance Parse builds.
func s5beGrammar() (*jsonic.Jsonic, parserTokens) {
	j := SafeMake(jsonic.Options{
		Info:  &jsonic.InfoOptions{Text: boolPtr(true), List: boolPtr(true), Map: boolPtr(true)},
		List:  &jsonic.ListOptions{Pair: boolPtr(true), Child: boolPtr(true)},
		Map:   &jsonic.MapOptions{Child: boolPtr(true)},
		Value: &jsonic.ValueOptions{Lex: boolPtr(false)},
	})
	t, _ := setupBaseTokens(j, loadDeclGrammar())
	setupTemplateLiteralMatcher(j, t)
	setupBigNumberMatcher(j, t)
	setupDecimalUnderscoreMatcher(j, t)
	setupMiniLitMatcher(j, t)
	setupXmlMatcher(j, t)
	setupValRule(j, t)
	setupPairGrammar(j, t)
	setupParenGrammar(j, t)
	setupAngleGrammar(j, t)
	setupInterpGrammar(j, t)
	setupXmlGrammar(j, t)
	setupNumberSub(j)
	return j, t
}

func s5beMatcher(tb testing.TB, j *jsonic.Jsonic, name string) jsonic.LexMatcher {
	tb.Helper()
	for _, m := range j.Config().CustomMatchers {
		if m.Name == name {
			return m.Match
		}
	}
	tb.Fatalf("matcher %q not registered", name)
	return nil
}

func s5beActions(j *jsonic.Jsonic, rule, phase string) []jsonic.StateAction {
	var out []jsonic.StateAction
	j.Rule(rule, func(rs *jsonic.RuleSpec, _ *jsonic.Parser) { out = rs.Actions(phase) })
	return out
}

// --- lex matchers -----------------------------------------------------------

func TestS5bETemplateMatcherGuards(t *testing.T) {
	j, _ := s5beGrammar()
	m := s5beMatcher(t, j, "template_literal")

	// No rule context: the matcher must decline.
	if tok := m(jsonic.NewLex("`x`", &jsonic.LexConfig{}), nil); tok != nil {
		t.Errorf("nil rule: expected decline, got %v", tok)
	}
	// Armed but at end of source: decline.
	armed := &jsonic.Rule{K: map[string]any{"boru_tpl": true}}
	if tok := m(jsonic.NewLex("", &jsonic.LexConfig{}), armed); tok != nil {
		t.Errorf("empty source: expected decline, got %v", tok)
	}
	// Sanity: armed mid-text it produces a #TL token.
	if tok := m(jsonic.NewLex("ab`", &jsonic.LexConfig{}), armed); tok == nil {
		t.Error("expected a template-literal token for plain text")
	}
}

func TestS5bEDecimalMatcherGuards(t *testing.T) {
	j, _ := s5beGrammar()
	m := s5beMatcher(t, j, "decimal_underscore")
	for _, src := range []string{"", "+"} {
		if tok := m(jsonic.NewLex(src, &jsonic.LexConfig{}), nil); tok != nil {
			t.Errorf("%q: expected decline, got %v", src, tok)
		}
	}
	// A signed leading-dot token with an incomplete exponent keeps its
	// numeric prefix; conversion will reject the leading dot itself.
	if tok := m(jsonic.NewLex("+.1e", &jsonic.LexConfig{}), nil); tok == nil || tok.Src != "+.1" {
		t.Errorf("signed leading-dot prefix: got %v", tok)
	}
	// Separator validation owns this malformed exponent. The matcher still
	// emits #NR (with a placeholder value) so the complete source reaches it.
	if tok := m(jsonic.NewLex("1e_", &jsonic.LexConfig{}), nil); tok == nil || tok.Src != "1e_" {
		t.Errorf("loose exponent separator: got %v", tok)
	}
}

func TestS5bEXmlMatcherGuards(t *testing.T) {
	j, _ := s5beGrammar()
	m := s5beMatcher(t, j, "xml_literal")

	// No rule context: decline.
	if tok := m(jsonic.NewLex("<a/>", &jsonic.LexConfig{}), nil); tok != nil {
		t.Errorf("nil rule: expected decline, got %v", tok)
	}
	armed := &jsonic.Rule{K: map[string]any{"boru_xml": true}}
	// `<` at end of source (nothing after the consumed `<`): no progress,
	// decline so normal lexing reports the character.
	if tok := m(jsonic.NewLex("", &jsonic.LexConfig{}), armed); tok != nil {
		t.Errorf("no progress: expected decline, got %v", tok)
	}
	// Cursor at 0 (the `<` position clamp): la = afterLA-1 clamps to 0.
	tok := m(jsonic.NewLex("a/>", &jsonic.LexConfig{}), armed)
	if tok == nil {
		t.Fatal("expected an #XML token")
	}
	ev, ok := tok.Val.(xmlElemVal)
	if !ok || ev.Err != nil {
		t.Fatalf("expected a clean xmlElemVal, got %#v", tok.Val)
	}
}

func TestS5bEParseLoneAngleIsError(t *testing.T) {
	if _, err := Parse("<"); err == nil {
		t.Error("a lone `<` at end of source must be a parse error")
	}
}

// --- dotchain ---------------------------------------------------------------

func TestS5bEMapValueDotchainLoops(t *testing.T) {
	// A multi-segment chain in map-value position: the second dotchain
	// push sees the parent's parenGroup already seeded.
	vals, err := Parse("{n: bf.a.b}")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected one map value, got %v", vals)
	}
}

func TestS5bEDotchainActionsNilParent(t *testing.T) {
	j, _ := s5beGrammar()
	bo := s5beActions(j, "dotchain", "bo")
	bc := s5beActions(j, "dotchain", "bc")
	if len(bo) == 0 || len(bc) == 0 {
		t.Fatal("dotchain actions not registered")
	}
	// Nil / NoRule parents: the actions must be inert.
	bo[0](&jsonic.Rule{}, nil)
	bo[0](&jsonic.Rule{Parent: jsonic.NoRule}, nil)
	bc[0](&jsonic.Rule{}, nil)
	bc[0](&jsonic.Rule{Parent: jsonic.NoRule}, nil)
	// A looped dotchain seeing an existing parenGroup keeps it as-is.
	par := &jsonic.Rule{Node: parenGroup{jsonic.Text{Str: "bf"}}}
	bo[0](&jsonic.Rule{Parent: par}, nil)
	if pg, ok := par.Node.(parenGroup); !ok || len(pg) != 1 {
		t.Errorf("existing parenGroup must be preserved, got %#v", par.Node)
	}
}

func TestS5bEDotchainSegNilToken(t *testing.T) {
	seg := dotchainSeg(".")
	// No key token and no parent: both guard arms fire without effect.
	seg(&jsonic.Rule{}, nil)
	// With a parent group, the bare marker is appended.
	par := &jsonic.Rule{Node: parenGroup{}}
	seg(&jsonic.Rule{Parent: par}, nil)
	pg, ok := par.Node.(parenGroup)
	if !ok || len(pg) != 1 {
		t.Fatalf("expected the bare marker appended, got %#v", par.Node)
	}
	if txt, ok := pg[0].(jsonic.Text); !ok || txt.Str != "." {
		t.Errorf("appended marker = %#v, want Text(.)", pg[0])
	}
}

// --- arrow folds --------------------------------------------------------------

func TestS5bEArrowAfterDotChainStaysFlat(t *testing.T) {
	// A flat dot-chain receiver before `=>`: arrowFoldable must decline
	// (the fold would capture only the final key as the sig).
	vals, err := Parse("m.k => [1]")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(vals) == 0 {
		t.Fatal("expected values")
	}
	// Sanity: a non-dot previous sibling still folds inside parens.
	if _, err := Parse("(a k => [k])"); err != nil {
		t.Fatalf("Parse paren fold: %v", err)
	}
}

func TestS5bEArrowFoldableCondPrevSibling(t *testing.T) {
	j, _ := s5beGrammar()
	var cond jsonic.AltCond
	j.Rule("val", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		for _, alt := range rs.CloseAlts() {
			if alt.P == "arrowfold" && alt.C != nil {
				cond = alt.C
			}
		}
	})
	if cond == nil {
		t.Fatal("arrowfold close alternate not found on val")
	}
	pelem := &jsonic.Rule{Name: "pelem"}
	// Previous sibling is a sited dot marker: a flat dot-chain must not fold.
	r := &jsonic.Rule{
		Parent: pelem,
		Prev:   &jsonic.Rule{Node: sited{Node: jsonic.Text{Str: "."}}},
	}
	if cond(r, nil) {
		t.Error("a val preceded by a `.` marker must not arrow-fold")
	}
	// Strict-marker sibling declines too.
	r.Prev = &jsonic.Rule{Node: jsonic.Text{Str: "!"}}
	if cond(r, nil) {
		t.Error("a val preceded by a `!` marker must not arrow-fold")
	}
	// An ordinary previous sibling folds.
	r.Prev = &jsonic.Rule{Node: jsonic.Text{Str: "x"}}
	if !cond(r, nil) {
		t.Error("an ordinary pelem val with a plain sibling should fold")
	}
}

func TestS5bEArrowfoldActionsNilParent(t *testing.T) {
	j, _ := s5beGrammar()
	bo := s5beActions(j, "arrowfold", "bo")
	bc := s5beActions(j, "arrowfold", "bc")
	if len(bo) == 0 || len(bc) == 0 {
		t.Fatal("arrowfold actions not registered")
	}
	bo[0](&jsonic.Rule{}, nil)
	bo[0](&jsonic.Rule{Parent: jsonic.NoRule}, nil)
	bc[0](&jsonic.Rule{}, nil)
	bc[0](&jsonic.Rule{Parent: jsonic.NoRule}, nil)
	// An undefined body: inert. The grammar refuses a bodiless arrow
	// before the fold closes (arrowNoBody, NUR060), so only a direct call
	// reaches this guard.
	par := &jsonic.Rule{Node: parenGroup{"sig", arrowTag{}}}
	bc[0](&jsonic.Rule{Parent: par, Child: &jsonic.Rule{Node: jsonic.Undefined}}, nil)
	if pg, ok := par.Node.(parenGroup); !ok || len(pg) != 2 {
		t.Errorf("an undefined body must leave the group untouched, got %#v", par.Node)
	}
}

func TestS5bEArrowfoldElemBOArms(t *testing.T) {
	j, _ := s5beGrammar()
	bo := s5beActions(j, "arrowfoldelem", "bo")
	if len(bo) == 0 {
		t.Fatal("arrowfoldelem BO not registered")
	}
	// Nil parent: inert.
	bo[0](&jsonic.Rule{}, nil)
	// Empty ListRef parent node: nothing to pop.
	r := &jsonic.Rule{U: map[string]any{}, Parent: &jsonic.Rule{Node: jsonic.ListRef{}}}
	bo[0](r, nil)
	if _, ok := r.U["af_sig"]; ok {
		t.Error("empty ListRef must not stash a sig")
	}
	// Empty []any parent node: nothing to pop.
	r = &jsonic.Rule{U: map[string]any{}, Parent: &jsonic.Rule{Node: []any{}}}
	bo[0](r, nil)
	if _, ok := r.U["af_sig"]; ok {
		t.Error("empty []any must not stash a sig")
	}
	// Non-empty []any with a grandparent: the pair pops, both levels are
	// updated with the shortened slice.
	grand := &jsonic.Rule{}
	par := &jsonic.Rule{Node: []any{"pairA", "pairB"}, Parent: grand}
	r = &jsonic.Rule{U: map[string]any{}, Parent: par}
	bo[0](r, nil)
	if got, ok := r.U["af_sig"]; !ok || got != "pairB" {
		t.Errorf("af_sig = %#v, want pairB", got)
	}
	if rest, ok := par.Node.([]any); !ok || len(rest) != 1 {
		t.Errorf("parent node = %#v, want the 1-elem rest", par.Node)
	}
	if rest, ok := grand.Node.([]any); !ok || len(rest) != 1 {
		t.Errorf("grandparent node = %#v, want the propagated rest", grand.Node)
	}
}

func TestS5bEArrowfoldElemBCArms(t *testing.T) {
	j, _ := s5beGrammar()
	bc := s5beActions(j, "arrowfoldelem", "bc")
	if len(bc) == 0 {
		t.Fatal("arrowfoldelem BC not registered")
	}
	// No stashed sig: inert.
	bc[0](&jsonic.Rule{U: map[string]any{}, Parent: &jsonic.Rule{Node: []any{}}}, nil)
	// A stashed sig but an undefined body: inert (only a direct call reaches
	// this guard — the grammar refuses a bodiless arrow first, NUR060).
	undef := &jsonic.Rule{Node: []any{}}
	bc[0](&jsonic.Rule{U: map[string]any{"af_sig": "sig"}, Parent: undef, Child: &jsonic.Rule{Node: jsonic.Undefined}}, nil)
	if n, ok := undef.Node.([]any); !ok || len(n) != 0 {
		t.Errorf("an undefined body must append nothing, got %#v", undef.Node)
	}
	// []any parent with a grandparent: the (SIG afn BODY) group is
	// appended and propagated up.
	grand := &jsonic.Rule{}
	par := &jsonic.Rule{Node: []any{}, Parent: grand}
	r := &jsonic.Rule{
		U:      map[string]any{"af_sig": "sig"},
		Parent: par,
		Child:  &jsonic.Rule{Node: "body"},
	}
	bc[0](r, nil)
	grown, ok := par.Node.([]any)
	if !ok || len(grown) != 1 {
		t.Fatalf("parent node = %#v, want the appended group", par.Node)
	}
	if _, ok := grown[0].(parenGroup); !ok {
		t.Errorf("appended element = %#v, want a parenGroup", grown[0])
	}
	if g, ok := grand.Node.([]any); !ok || len(g) != 1 {
		t.Errorf("grandparent node = %#v, want the propagated slice", grand.Node)
	}
}

// --- interp / angle -----------------------------------------------------------

func TestS5bEIelemUndefinedNode(t *testing.T) {
	j, _ := s5beGrammar()
	bc := s5beActions(j, "ielem", "bc")
	if len(bc) == 0 {
		t.Fatal("ielem BC not registered")
	}
	par := &jsonic.Rule{Node: interpGroup{}}
	r := &jsonic.Rule{Node: jsonic.Undefined, Child: jsonic.NoRule, Parent: par}
	bc[0](r, nil)
	if grp, ok := par.Node.(interpGroup); !ok || len(grp) != 0 {
		t.Errorf("undefined ielem node must not be collected, parent = %#v", par.Node)
	}
}

func TestS5bEAngleActionsNilParent(t *testing.T) {
	j, _ := s5beGrammar()
	bc := s5beActions(j, "angle", "bc")
	ac := s5beActions(j, "angle", "ac")
	if len(bc) == 0 || len(ac) == 0 {
		t.Fatal("angle actions not registered")
	}
	// BC with a nil parent: inert.
	bc[0](&jsonic.Rule{}, nil)
	bc[0](&jsonic.Rule{Parent: jsonic.NoRule}, nil)
	// AC on an unclosed group with a nil parent: inert.
	ac[0](&jsonic.Rule{U: map[string]any{}}, nil)
	ac[0](&jsonic.Rule{U: map[string]any{}, Parent: jsonic.NoRule}, nil)
	// AC on a closed group returns before touching the node.
	closed := &jsonic.Rule{U: map[string]any{"closed": true}, Node: "keep"}
	ac[0](closed, nil)
	if closed.Node != "keep" {
		t.Errorf("closed angle AC must not rewrite the node, got %#v", closed.Node)
	}
}

// --- parse.go conversions -------------------------------------------------------

func TestS5bEConvertFloat64(t *testing.T) {
	v, err := convertTopLevelValueInner(float64(1.5), &parseDepth{})
	if err != nil {
		t.Fatalf("convert float64: %v", err)
	}
	if v.Parent == nil {
		t.Fatal("expected a numeric value")
	}
}

func TestS5bEConvertInterpGroupUnknownPart(t *testing.T) {
	_, err := convertInterpGroup(interpGroup{42}, &parseDepth{})
	if err == nil || !strings.Contains(err.Error(), "unexpected interp part type") {
		t.Errorf("expected the unexpected-part error, got %v", err)
	}
}

func TestS5bENumberValFallbacksAndOverflow(t *testing.T) {
	// A dotted Src that fails strconv re-parsing falls back to the
	// jsonic-lexed float value.
	v, err := numberValToValue(numberVal{Src: "1.2.3", Val: 7.5})
	if err != nil {
		t.Fatalf("dotted fallback: %v", err)
	}
	f, err := core.AsFloat(v)
	if err != nil || f != 7.5 {
		t.Errorf("fallback float = %v (%v), want 7.5", f, err)
	}
	// A base-prefixed literal out of int64 range raises integer_overflow.
	_, err = numberValToValue(numberVal{Src: "0x10000000000000000"})
	if err == nil || !strings.Contains(err.Error(), "integer_overflow") {
		t.Errorf("expected integer_overflow for the huge hex literal, got %v", err)
	}
}

func TestS5bEParseBigNumberEmptyDigits(t *testing.T) {
	if _, err := parseBigNumber("0d"); err == nil {
		t.Error("0d with no digits must error")
	}
}
