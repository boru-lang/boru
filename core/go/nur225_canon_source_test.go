package core

import "testing"

// TestNUR225TemplateCanonIsSource pins NUR225's template half: a template
// string canons as the backtick source it came from, literal text carrying
// the template lexer's escapes and each hole rendering its tokens' canon —
// where it used to fall to the debug `interp('a ' ${word(x)})`.
func TestNUR225TemplateCanonIsSource(t *testing.T) {
	v := NewInterpString([]InterpPart{
		{Lit: "a`b\\c ${d}\n\t\r$"},
		{Expr: []Value{NewWord("x"), NewWord("add"), NewInteger(1)}},
		{Lit: "e"},
	})
	want := "`a\\`b\\\\c \\${d}\\n\\t\\r$${x add 1}e`"
	if got := CanonValue(v); got != want {
		t.Errorf("template canon = %q, want %q", got, want)
	}
}

// TestNUR225XmlTmplCanonIsSource pins the XML half: an XML literal with holes
// canons as its source — attribute and text holes over their tokens' canon,
// literal text and attribute text escaped as a plain element's are, a nested
// element recursing — where it used to wear the debug `interp-xml(…)`.
func TestNUR225XmlTmplCanonIsSource(t *testing.T) {
	inner := XmlTmpl{Tag: "b"}
	v := NewXmlInterp(XmlTmpl{
		Tag: "a",
		Attr: []XmlAttrTmpl{{Name: "k", Parts: []InterpPart{
			{Lit: `p"&`},
			{Expr: []Value{NewString("x'y")}},
		}}},
		Cren: []XmlCren{
			{Kind: XmlCrenLit, Lit: "t<&>"},
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("x")}},
			{Kind: XmlCrenChild, Child: &inner},
			{Kind: XmlCrenChild},
		},
	})
	want := `<a k="p&quot;&amp;${"x'y"}">t&lt;&amp;&gt;${x}<b/></a>`
	if got := CanonValue(v); got != want {
		t.Errorf("xml template canon = %q, want %q", got, want)
	}
}

// TestNUR226MapKeysCanonAsTheirKey pins NUR226: a key that would not lex back
// as the same single key is quoted — whitespace, an empty key, a structural
// character — and one that would stays bare, in a map, a typed map, a flex
// map and a weak flex map alike.
func TestNUR226MapKeysCanonAsTheirKey(t *testing.T) {
	for _, c := range []struct{ key, want string }{
		{"a", "a"}, {"q k", "'q k'"}, {"", "''"}, {"@quit", "@quit"},
		{"1.5", "'1.5'"}, {"é-x_$", "é-x_$"}, {"a?", "'a?'"},
	} {
		if got := canonKey(c.key); got != c.want {
			t.Errorf("canonKey(%q) = %q, want %q", c.key, got, c.want)
		}
	}
	om := NewOrderedMap()
	om.Set("q k", NewInteger(2))
	if got := CanonValue(NewMap(om)); got != "{'q k':2}" {
		t.Errorf("map canon = %q", got)
	}
	if got := CanonValue(NewFlexMap(om)); got != "(flex {'q k':2})" {
		t.Errorf("flex map canon = %q", got)
	}
	tm := NewTypedMapWithEntries(NewWord("Integer"), []ChildEntry{{Key: "a b", Value: NewInteger(1)}})
	if got := CanonValue(tm); got != "{:Integer 'a b':1}" {
		t.Errorf("typed map canon = %q", got)
	}
}

// TestNUR227CommaSeparatesAnAngleReceiver pins NUR227: a bare capitalised
// token before a part that opens `<` is joined with a comma — the angle gate
// would fuse them — and nothing else is.
func TestNUR227CommaSeparatesAnAngleReceiver(t *testing.T) {
	for _, c := range []struct {
		parts []string
		want  string
	}{
		{[]string{":A", "<a/>"}, ":A, <a/>"},
		{[]string{"A", "<a/>"}, "A, <a/>"},
		{[]string{"x", "<a/>"}, "x <a/>"},
		{[]string{"(A)", "<a/>"}, "(A) <a/>"},
		{[]string{"A", "b"}, "A b"},
		{[]string{"<a/>"}, "<a/>"},
	} {
		if got := joinCanonParts(c.parts); got != c.want {
			t.Errorf("joinCanonParts(%q) = %q, want %q", c.parts, got, c.want)
		}
	}
	xml := NewXmlInterp(XmlTmpl{Tag: "a"})
	if got := CanonValues([]Value{NewWord("A"), xml}); got != "A, <a/>" {
		t.Errorf("stream canon = %q, want A, <a/>", got)
	}
}

// TestNUR072SequenceRulesSpellMarkersAndFolds pins NUR072's sequence rules in
// core: a group-modifier marker is spelled after the group it precedes, in a
// stream, a list and a paren body; a marker with nothing after it renders
// alone; an arrow fold renders without parens and an explicit paren around it
// keeps its own.
func TestNUR072SequenceRulesSpellMarkersAndFolds(t *testing.T) {
	group := NewParenExpr([]Value{NewInteger(1), NewInteger(2)})
	for _, c := range []struct {
		marker SugarInfo
		want   string
	}{
		{SugarInfo{Kind: SugarUsurp}, "(1 2) /u"},
		{SugarInfo{Kind: SugarStackArgs}, "(1 2) /s"},
		{SugarInfo{Kind: SugarForwardArgs}, "(1 2) /f"},
		{SugarInfo{Kind: SugarForceArity, N: 2}, "(1 2) /2"},
	} {
		if got := CanonValues([]Value{NewSugar(c.marker), group}); got != c.want {
			t.Errorf("stream %v = %q, want %q", c.marker.Kind, got, c.want)
		}
	}
	stack := NewSugar(SugarInfo{Kind: SugarStackArgs})
	if got := CanonValue(NewList([]Value{stack, group})); got != "[(1 2) /s]" {
		t.Errorf("list = %q", got)
	}
	if got := CanonValue(NewParenExpr([]Value{stack, group})); got != "((1 2) /s)" {
		t.Errorf("paren body = %q", got)
	}
	if got := CanonValues([]Value{NewInteger(1), stack}); got != "1 sugar(stack-args)" {
		t.Errorf("a trailing marker renders alone, got %q", got)
	}
	fold := NewParenExpr([]Value{NewWord("x"), NewSugar(SugarInfo{Kind: SugarLambda}), NewList([]Value{NewWord("x")})})
	if got := CanonValue(fold); got != "x => [x]" {
		t.Errorf("fold = %q", got)
	}
	if got := CanonValue(NewParenExpr([]Value{fold})); got != "(x => [x])" {
		t.Errorf("explicit paren around a fold = %q", got)
	}
	notFold := NewParenExpr([]Value{NewWord("x"), NewWord("y"), NewWord("z")})
	if got := CanonValue(notFold); got != "(x y z)" {
		t.Errorf("a three-token group that is no fold keeps its parens, got %q", got)
	}
	if isLambdaFold([]Value{NewWord("x"), NewString("=>"), NewWord("y")}) {
		t.Error("a non-marker middle token is no fold")
	}
}

// TestNUR072DisjunctCanonSpellsItsMembers pins the Go disjunct arm: an atom
// member bare, a type literal by its leaf, anything else by its canon — the
// TS twin's rule.
func TestNUR072DisjunctCanonSpellsItsMembers(t *testing.T) {
	d := NewDisjunct([]Value{NewAtom("a"), NewTypeLiteral(TInteger), NewWord("W"), NewString("s")})
	if got := CanonValue(d); got != "a tor Integer tor W tor 's'" {
		t.Errorf("disjunct canon = %q", got)
	}
}

// TestCanonFallbackIsTheDebugForm pins the canon's last arm: a kind with no
// source spelling yet renders its Value.String form — the ADR-015 residue a
// future record must name, never a silent empty string.
func TestCanonFallbackIsTheDebugForm(t *testing.T) {
	m := NewMark("m1")
	if got := CanonValue(m); got != m.String() || got == "" {
		t.Errorf("fallback canon = %q, want %q", got, m.String())
	}
}

// TestFirstOwnSig covers the single-overload reader core itself no longer
// calls since the predicate stopped reading one overload (NUR100) — a nil
// payload, a fallback-only set, and the first own signature past a fallback.
func TestFirstOwnSig(t *testing.T) {
	var none *FnDefInfo
	if _, ok := none.FirstOwnSig(); ok {
		t.Error("a nil payload has no own signature")
	}
	fb := &FnDefInfo{Signatures: []Signature{{Fallback: true}}}
	if _, ok := fb.FirstOwnSig(); ok {
		t.Error("a fallback-only set has no own signature")
	}
	own := &FnDefInfo{Signatures: []Signature{{Fallback: true}, {BarrierPos: 7}}}
	if s, ok := own.FirstOwnSig(); !ok || s.BarrierPos != 7 {
		t.Errorf("the first own signature past a fallback, got %v %v", s, ok)
	}
}
