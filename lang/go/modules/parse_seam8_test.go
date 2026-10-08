package modules

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	tabnasabnf "github.com/tabnas/abnf/go"
	tabnas "github.com/tabnas/parser/go"
)

// w8ParseGrammar mints a fresh, un-registered Parse grammar carrier plus the
// underlying *parseGrammar, so a white-box test can drive the builder-word
// handlers directly (the carrier satisfies asParseGrammar).
func w8ParseGrammar() (native.Value, *parseGrammar) {
	g := &parseGrammar{j: tabnas.Make(), markActions: tabnasabnf.ActionsMap{}}
	return core.NewExtension(native.TIdeal, g), g
}

// w8LitS is a non-concrete String type literal (passes a TString sig, declined
// by AsConcreteString).
func w8LitS() native.Value { return native.NewTypeLiteral(native.TString) }

// TestW8ParseBuilderBadGrammar drives every builder handler's asParseGrammar
// decline arm: a non-grammar args[0].
func TestW8ParseBuilderBadGrammar(t *testing.T) {
	r := mcovReg(t)
	bad := native.NewInteger(0)
	if _, err := parseAbnfHandler([]native.Value{bad, native.NewString("x")}, nil, nil, r); err == nil {
		t.Error("Parse.abnf: non-grammar should error")
	}
	if _, err := parseSpecHandler([]native.Value{bad, s7bMap()}, nil, nil, r); err == nil {
		t.Error("Parse.spec: non-grammar should error")
	}
	if _, err := parseRuleHandler([]native.Value{bad, native.NewAtom("v"), s7bMap()}, nil, nil, r); err == nil {
		t.Error("Parse.rule: non-grammar should error")
	}
	if _, err := parseTokenHandler([]native.Value{bad, native.NewString("#Z"), native.NewString("z")}, nil, nil, r); err == nil {
		t.Error("Parse.token: non-grammar should error")
	}
	if _, err := parseMatcherHandler([]native.Value{bad, native.NewAtom("m"), native.NewInteger(1), mcovFilterFn()}, nil, nil, r); err == nil {
		t.Error("Parse.matcher: non-grammar should error")
	}
	if _, err := parseActionHandler([]native.Value{bad, native.NewString("@a"), mcovFilterFn()}, nil, nil, r); err == nil {
		t.Error("Parse.action: non-grammar should error")
	}
}

// TestW8ParseBuilderBadArgs drives the AsConcrete* decline arms of the builder
// handlers (a type-literal passes the sig but the handler declines it).
func TestW8ParseBuilderBadArgs(t *testing.T) {
	r := mcovReg(t)
	gv, _ := w8ParseGrammar()

	// Parse.abnf src (372)
	if _, err := parseAbnfHandler([]native.Value{gv, w8LitS()}, nil, nil, r); err == nil {
		t.Error("Parse.abnf: non-concrete src should error")
	}
	// Parse.rule name (667)
	if _, err := parseRuleHandler([]native.Value{gv, native.NewTypeLiteral(native.TAtom), s7bMap()}, nil, nil, r); err == nil {
		t.Error("Parse.rule: non-concrete name should error")
	}
	// Parse.token name (693) + literal (697)
	if _, err := parseTokenHandler([]native.Value{gv, w8LitS(), native.NewString("z")}, nil, nil, r); err == nil {
		t.Error("Parse.token: non-concrete name should error")
	}
	if _, err := parseTokenHandler([]native.Value{gv, native.NewString("#Z"), w8LitS()}, nil, nil, r); err == nil {
		t.Error("Parse.token: non-concrete literal should error")
	}
	// Parse.matcher name (717) + priority (721)
	if _, err := parseMatcherHandler([]native.Value{gv, native.NewTypeLiteral(native.TAtom), native.NewInteger(1), mcovFilterFn()}, nil, nil, r); err == nil {
		t.Error("Parse.matcher: non-concrete name should error")
	}
	if _, err := parseMatcherHandler([]native.Value{gv, native.NewAtom("m"), native.NewTypeLiteral(native.TInteger), mcovFilterFn()}, nil, nil, r); err == nil {
		t.Error("Parse.matcher: non-concrete priority should error")
	}
	// Parse.action ref (738)
	if _, err := parseActionHandler([]native.Value{gv, w8LitS(), mcovFilterFn()}, nil, nil, r); err == nil {
		t.Error("Parse.action: non-concrete ref should error")
	}
}

// TestW8ParseParserHandlerArgs drives the parse-parser handler's grammar
// decline arm directly.
func TestW8ParseParserHandlerArgs(t *testing.T) {
	r := mcovReg(t)
	h := parseParserHandlerFor(r)
	if _, err := h([]native.Value{native.NewInteger(5)}, nil, nil, r); err == nil {
		t.Error("Parse.parser: non-grammar should error")
	}
}

// TestW8ParseHandlerSrcError drives parseHandler's src decline arm (817).
func TestW8ParseHandlerSrcError(t *testing.T) {
	r := mcovReg(t)
	_, g := w8ParseGrammar()
	h := g.parseHandler("k")
	if _, err := h([]native.Value{w8LitS()}, nil, nil, r); err == nil {
		t.Error("parseHandler: non-concrete src should error")
	}
}

// TestW8ApplySpecMapArms drives applySpecMap's error / string arms that the
// boru-run cov tests don't reach.
func TestW8ApplySpecMapArms(t *testing.T) {
	r := mcovReg(t)

	// 461: RequireConcreteMap compile failure (a non-concrete spec value).
	if _, g := w8ParseGrammar(); applySpecMap(g, native.NewTypeLiteral(native.TMap), r, false) == nil {
		t.Error("applySpecMap: non-concrete spec should error")
	}

	// 535: ref section not a map.
	refBad := native.NewOrderedMap()
	refBad.Set("ref", native.NewInteger(5))
	if _, g := w8ParseGrammar(); applySpecMap(g, native.NewMap(refBad), r, false) == nil {
		t.Error("applySpecMap: non-map ref section should error")
	}

	// 564: rule section not a map.
	ruleBad := native.NewOrderedMap()
	ruleBad.Set("rule", native.NewInteger(5))
	if _, g := w8ParseGrammar(); applySpecMap(g, native.NewMap(ruleBad), r, false) == nil {
		t.Error("applySpecMap: non-map rule section should error")
	}

	// 621: matcher section not a map.
	mBad := native.NewOrderedMap()
	mBad.Set("matcher", native.NewInteger(5))
	if _, g := w8ParseGrammar(); applySpecMap(g, native.NewMap(mBad), r, false) == nil {
		t.Error("applySpecMap: non-map matcher section should error")
	}

	// 599: abnf as a bare String entry.
	abnfStr := native.NewOrderedMap()
	abnfStr.Set("abnf", native.NewString(`op = "inc"`))
	_, g := w8ParseGrammar()
	if err := applySpecMap(g, native.NewMap(abnfStr), r, false); err != nil {
		t.Errorf("applySpecMap: bare-String abnf entry should apply, got %v", err)
	}
	if len(g.steps) == 0 {
		t.Error("applySpecMap: bare-String abnf entry should defer a step")
	}
}

// TestW8ApplySpecMapLenientEntrySkips drives the per-entry lenient skips (593,
// 626): a concrete section whose ELEMENTS are non-concrete.
func TestW8ApplySpecMapLenientEntrySkips(t *testing.T) {
	r := mcovReg(t)
	spec := native.NewOrderedMap()
	// abnf: a concrete LIST holding a non-concrete element → 593 continue.
	spec.Set("abnf", native.NewList([]native.Value{native.NewTypeLiteral(native.TString)}))
	// matcher: a concrete MAP whose entry value is non-concrete → 626 continue.
	mm := native.NewOrderedMap()
	mm.Set("m", native.NewTypeLiteral(native.TMap))
	spec.Set("matcher", native.NewMap(mm))

	_, g := w8ParseGrammar()
	if err := applySpecMap(g, native.NewMap(spec), r, true); err != nil {
		t.Errorf("lenient per-entry non-concrete should be skipped, got %v", err)
	}
}

// TestW8ApplySpecMapRuleStepError drives the deferred rule step's build-error
// arm (578): a spec rule with an unknown backtrack ref fails at replay.
func TestW8ApplySpecMapRuleStepError(t *testing.T) {
	r := mcovReg(t)
	spec := native.NewOrderedMap()
	rule := native.NewOrderedMap()
	val := native.NewOrderedMap()
	alt := native.NewOrderedMap()
	alt.Set("s", native.NewString("#TX"))
	alt.Set("b", native.NewString("nosuch"))
	val.Set("open", native.NewList([]native.Value{native.NewMap(alt)}))
	rule.Set("val", native.NewMap(val))
	spec.Set("rule", native.NewMap(rule))

	_, g := w8ParseGrammar()
	if err := applySpecMap(g, native.NewMap(spec), r, false); err != nil {
		t.Fatalf("applySpecMap defers, should not error yet: %v", err)
	}
	var stepErr error
	for _, s := range g.steps {
		if e := s(); e != nil {
			stepErr = e
		}
	}
	if stepErr == nil {
		t.Error("a rule with an unknown backtrack ref should fail at step replay")
	}
}

// TestW8RuleMapToSpecArms drives ruleMapToSpec's close-build error (1002) and
// altMapToSpec's p / r field arms (1022 / 1025).
func TestW8RuleMapToSpecArms(t *testing.T) {
	r := mcovReg(t)
	_, g := w8ParseGrammar()

	// 1002: close is not a list.
	closeBad := native.NewOrderedMap()
	closeBad.Set("close", native.NewString("nope"))
	if _, err := ruleMapToSpec(g, "val", native.NewMap(closeBad), r); err == nil {
		t.Error("ruleMapToSpec: non-list close should error")
	}

	// 1022 / 1025: p and r fields on an alternate.
	alt := native.NewOrderedMap()
	alt.Set("s", native.NewString("#TX"))
	alt.Set("p", native.NewString("child"))
	alt.Set("r", native.NewString("child"))
	rule := native.NewOrderedMap()
	rule.Set("open", native.NewList([]native.Value{native.NewMap(alt)}))
	if _, err := ruleMapToSpec(g, "val", native.NewMap(rule), r); err != nil {
		t.Errorf("ruleMapToSpec: p/v fields should be accepted, got %v", err)
	}
}

// TestW8WrapActionGuard drives wrapAction's inert guard (845): with no live
// registry the action no-ops.
func TestW8WrapActionGuard(t *testing.T) {
	_, g := w8ParseGrammar() // g.r == nil
	act := g.wrapAction(mcovFilterFn())
	act(nil, nil) // must not panic; the guard returns immediately
}

// TestW8WrapMatcherGuards drives wrapMatcher's inert guard (867) and the
// cursor-at-end arm (871).
func TestW8WrapMatcherGuards(t *testing.T) {
	r := mcovReg(t)
	_, g := w8ParseGrammar()

	// 867: no live registry → returns nil.
	mt := g.wrapMatcher(mcovFilterFn())
	if tok := mt(nil, nil); tok != nil {
		t.Error("wrapMatcher: nil registry should return nil")
	}

	// 871: cursor at end of an empty source → returns nil.
	g.r = r
	mtEmpty := g.wrapMatcher(mcovFilterFn())
	lexEmpty := tabnas.NewLex("", g.j.Config())
	if tok := mtEmpty(lexEmpty, nil); tok != nil {
		t.Error("wrapMatcher: cursor at end should return nil")
	}
}

// TestW8WrapMatcherResults drives wrapMatcher's no-values (880) and
// None-result (884) arms via a live registry and a non-empty source.
func TestW8WrapMatcherResults(t *testing.T) {
	r := mcovReg(t)

	// 880: a callback that leaves the stack empty → no match.
	emptyFn := mcovRun(t, r, `([rest:String] => [])`)[0]
	_, ge := w8ParseGrammar()
	ge.r = r
	mtEmpty := ge.wrapMatcher(emptyFn)
	if tok := mtEmpty(tabnas.NewLex("abc", ge.j.Config()), nil); tok != nil {
		t.Error("wrapMatcher: an empty callback result should not match")
	}
	if ge.firstErr != nil {
		t.Errorf("wrapMatcher: an empty result is not an error, got %v", ge.firstErr)
	}

	// 884: a callback that returns None → explicit no-match.
	noneFn := mcovRun(t, r, `([rest:String] => [None])`)[0]
	_, gn := w8ParseGrammar()
	gn.r = r
	mtNone := gn.wrapMatcher(noneFn)
	if tok := mtNone(tabnas.NewLex("abc", gn.j.Config()), nil); tok != nil {
		t.Error("wrapMatcher: a None result should not match")
	}
}
