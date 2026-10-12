package core

import (
	"strings"
	"testing"
)

// Coverage for the XML-interpolation evaluator (EvalXmlInterp /
// BuildXmlFromTmpl), the Reach lowering trio (lowerReach / expandReach /
// ApplyReach), and the container auto-eval arms (autoEvalMap /
// autoEvalList / evalInterpString / evalParenExprResults) in engine.go.
// Reuses covRegistry from compile_pipeline_cov_test.go.

// registerDotWords adds minimal `dot` / `dotr` get-chain words (the
// lang-layer words lowerReach targets). Convention: args[0] = key
// (forward), args[1] = receiver (stack). `dot` is lenient (missing key
// yields no value); `dotr` is strict (missing key errors).
func registerDotWords(r *Registry) {
	get := func(strict bool) Handler {
		return func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
			key := ""
			switch {
			case IsAtom(args[0]):
				key, _ = AsAtom(args[0])
			case args[0].Parent.ConformsTo(TString):
				key, _ = AsString(args[0])
			default:
				key = ValToString(args[0])
			}
			m, err := AsMap(args[1])
			if err != nil {
				return nil, r.BoruError("type_error", "dot: receiver is not a map", "dot")
			}
			v, ok := m.Get(key)
			if !ok {
				if strict {
					return nil, r.BoruError("get_error", "dotr: missing key "+key, "dotr")
				}
				return nil, nil
			}
			return []Value{v}, nil
		}
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: "dot",
		Signatures: []Signature{{
			Args: []*Type{TAny, TMap}, Impl: Go(get(false)),
			Returns: []*Type{TAny}, BarrierPos: 1,
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "dotr",
		Signatures: []Signature{{
			Args: []*Type{TAny, TMap}, Impl: Go(get(true)),
			Returns: []*Type{TAny}, BarrierPos: 1,
		}},
	})
}

// --- Reach lowering ------------------------------------------------------------

func TestEvalReachTopLevel(t *testing.T) {
	r := covRegistry(t, registerDotWords)
	m := mapOf("a", mapOf("b", NewInteger(42)))
	reach := NewReach(ReachInfo{
		Receiver: []Value{m},
		Segments: []ReachSeg{
			{KeyLit: NewString("a")},
			{Getr: true, KeyLit: NewString("b")},
		},
		Eval: true,
	})
	out, err := NewTop(r).Run([]Value{reach})
	if err != nil {
		t.Fatalf("reach eval: %v", err)
	}
	if got := renderAll(out); got != "42" {
		t.Errorf("got %q, want 42", got)
	}
}

func TestEvalReachComputedKey(t *testing.T) {
	r := covRegistry(t, registerDotWords)
	m := mapOf("k3", NewString("hit"))
	// Computed segment: the key expression evaluates before dot consumes it.
	reach := NewReach(ReachInfo{
		Receiver: []Value{m},
		Segments: []ReachSeg{{
			Computed: true,
			KeyExpr:  []Value{NewWord("ccat"), NewString("k"), NewString("3")},
		}},
		Eval: true,
	})
	out, err := NewTop(r).Run([]Value{reach})
	if err != nil {
		t.Fatalf("computed reach: %v", err)
	}
	if got := renderAll(out); !strings.Contains(got, "hit") {
		t.Errorf("got %q, want hit", got)
	}
}

func TestEvalReachNestedInParens(t *testing.T) {
	// A Reach nested inside a collapsing paren span expands via the
	// stepLiteral / stepCloseParen path rather than the main loop.
	r := covRegistry(t, registerDotWords)
	m := mapOf("n", NewInteger(5))
	reach := NewReach(ReachInfo{
		Receiver: []Value{m},
		Segments: []ReachSeg{{KeyLit: NewString("n")}},
		Eval:     true,
	})
	out, err := NewTop(r).Run([]Value{
		NewWord("cadd"),
		NewOpenParen(), reach, NewCloseParen(),
		NewInteger(10),
	})
	if err != nil {
		t.Fatalf("nested reach: %v", err)
	}
	if got := renderAll(out); got != "15" {
		t.Errorf("got %q, want 15", got)
	}
}

func TestQuotedAndInertReachStayData(t *testing.T) {
	r := covRegistry(t, registerDotWords)
	m := mapOf("a", NewInteger(1))
	q := NewReach(ReachInfo{Receiver: []Value{m}, Segments: []ReachSeg{{KeyLit: NewString("a")}}, Eval: true})
	SetQuoted(&q, true)
	inert := NewReachFromKeys(m, []Value{NewString("a")})
	out, err := NewTop(r).Run([]Value{q, inert})
	if err != nil {
		t.Fatalf("inert reach: %v", err)
	}
	if len(out) != 2 || !IsReach(&out[0]) || !IsReach(&out[1]) {
		t.Errorf("reaches evaluated: %s", renderAll(out))
	}
}

func TestApplyReachDirect(t *testing.T) {
	r := covRegistry(t, registerDotWords)
	m := mapOf("x", mapOf("y", NewString("deep")))
	info := ReachInfo{Segments: []ReachSeg{
		{KeyLit: NewString("x")},
		{KeyLit: NewString("y")},
	}}
	v, err := ApplyReach(r, info, m)
	if err != nil {
		t.Fatalf("ApplyReach: %v", err)
	}
	if s, _ := AsString(v); s != "deep" {
		t.Errorf("ApplyReach = %v", v)
	}

	// Missing key on the lenient chain yields no value → apply error.
	miss := ReachInfo{Segments: []ReachSeg{{KeyLit: NewString("zz")}}}
	if _, err := ApplyReach(r, miss, m); err == nil || !strings.Contains(err.Error(), "no value") {
		t.Errorf("missing-key apply: err = %v", err)
	}

	// Strict segment (dotr) errors instead.
	strict := ReachInfo{Segments: []ReachSeg{{Getr: true, KeyLit: NewString("zz")}}}
	if _, err := ApplyReach(r, strict, m); err == nil {
		t.Error("strict apply of missing key did not error")
	}
}

func TestLowerReachShape(t *testing.T) {
	info := ReachInfo{
		Receiver: []Value{NewWord("m")},
		Segments: []ReachSeg{
			{KeyLit: NewString("a")},
			{Getr: true, KeyLit: NewString("b")},
			{Computed: true, KeyExpr: []Value{NewWord("k")}},
		},
	}
	toks := lowerReach(info)
	if len(toks) != 7 {
		t.Fatalf("lowered token count = %d: %v", len(toks), toks)
	}
	w1, _ := AsWord(&toks[1])
	w3, _ := AsWord(&toks[3])
	if w1.Name != "dot" || w3.Name != "dotr" {
		t.Errorf("chain words = %s %s", w1.Name, w3.Name)
	}
	if !IsParenExpr(&toks[6]) {
		t.Errorf("computed key did not lower to a ParenExpr: %v", toks[6])
	}
	span := expandReach(info)
	if !IsOpenParen(&span[0]) || !IsCloseParen(&span[len(span)-1]) {
		t.Error("expandReach did not wrap in paren markers")
	}
}

// --- XML interpolation -----------------------------------------------------------

func TestXmlInterpLiteralSkeleton(t *testing.T) {
	r := covRegistry(t, nil)
	tmpl := XmlTmpl{
		Tag:  "p",
		Attr: []XmlAttrTmpl{{Name: "id", Parts: []InterpPart{{Lit: "x1"}}}},
		Cren: []XmlCren{{Kind: XmlCrenLit, Lit: "hello"}},
	}
	out, err := NewTop(r).Run([]Value{NewXmlInterp(tmpl)})
	if err != nil {
		t.Fatalf("xml interp: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("xml results: %s", renderAll(out))
	}
	s := out[0].String()
	if !strings.Contains(s, "p") || !strings.Contains(s, "hello") {
		t.Errorf("rendered xml = %q", s)
	}
}

func TestXmlInterpAttrAndChildHoles(t *testing.T) {
	r := covRegistry(t, nil)
	tmpl := XmlTmpl{
		Tag: "div",
		Attr: []XmlAttrTmpl{{
			Name:  "n",
			Parts: []InterpPart{{Lit: "v"}, {Expr: []Value{NewWord("cadd"), NewInteger(1), NewInteger(2)}}},
		}},
		Cren: []XmlCren{
			{Kind: XmlCrenLit, Lit: "count: "},
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("cmul"), NewInteger(3), NewInteger(4)}},
			{Kind: XmlCrenLit, Lit: "!"},
		},
	}
	out, err := NewTop(r).Run([]Value{NewXmlInterp(tmpl)})
	if err != nil {
		t.Fatalf("xml holes: %v", err)
	}
	s := out[0].String()
	if !strings.Contains(s, "v3") {
		t.Errorf("attr hole not evaluated: %q", s)
	}
	if !strings.Contains(s, "count: 12!") {
		t.Errorf("child hole / text merge wrong: %q", s)
	}
}

func TestXmlInterpChildSpliceRules(t *testing.T) {
	// A List hole splices each element; an XML value is one child; a
	// nested template recurses; a nil child is skipped.
	r := covRegistry(t, nil)
	innerXml := NewXmlElement("b", NewOrderedMap(), []Value{NewString("bold")})
	InstallDef(r, "xs", NewList([]Value{NewInteger(1), innerXml, NewString("t")}))
	tmpl := XmlTmpl{
		Tag: "ul",
		Cren: []XmlCren{
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("xs")}},
			{Kind: XmlCrenChild, Child: nil},
			{Kind: XmlCrenChild, Child: &XmlTmpl{Tag: "li", Cren: []XmlCren{{Kind: XmlCrenLit, Lit: "item"}}}},
		},
	}
	out, err := NewTop(r).Run([]Value{NewXmlInterp(tmpl)})
	if err != nil {
		t.Fatalf("xml splice: %v", err)
	}
	s := out[0].String()
	for _, want := range []string{"ul>1<b>bold</b>t", "<li>item</li>"} {
		if !strings.Contains(s, want) {
			t.Errorf("splice render %q missing %q", s, want)
		}
	}
}

func TestXmlInterpExprErrorPropagates(t *testing.T) {
	r := covRegistry(t, nil)
	tmpl := XmlTmpl{
		Tag:  "p",
		Cren: []XmlCren{{Kind: XmlCrenExpr, Expr: []Value{NewWord("no_such_word_x")}}},
	}
	if _, err := NewTop(r).Run([]Value{NewXmlInterp(tmpl)}); err == nil {
		t.Error("xml hole error did not propagate")
	}
	// Attribute expression error propagates too.
	tmpl2 := XmlTmpl{
		Tag:  "p",
		Attr: []XmlAttrTmpl{{Name: "a", Parts: []InterpPart{{Expr: []Value{NewWord("no_such_word_x")}}}}},
	}
	if _, err := NewTop(r).Run([]Value{NewXmlInterp(tmpl2)}); err == nil {
		t.Error("xml attr error did not propagate")
	}
}

// --- autoEvalMap arms --------------------------------------------------------------

func evalMapVal(t *testing.T, r *Registry, om *OrderedMap) Value {
	t.Helper()
	mv := NewMap(om)
	SetEval(&mv, true)
	out, err := NewTop(r).Run([]Value{mv})
	if err != nil {
		t.Fatalf("map auto-eval: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("map results: %s", renderAll(out))
	}
	return out[0]
}

func TestAutoEvalMapValueArms(t *testing.T) {
	r := covRegistry(t, registerDotWords)
	InstallDef(r, "rv", NewInteger(10))
	om := NewOrderedMap()
	om.Set("w", NewWord("rv"))                                                                  // word resolves
	om.Set("p", NewParenExpr([]Value{NewWord("cadd"), NewInteger(1), NewInteger(2)}))           // paren evaluates
	om.Set("multi", NewParenExpr([]Value{NewInteger(1), NewEnd(), NewInteger(2)}))              // multi-result → list
	om.Set("i", NewInterpString([]InterpPart{{Lit: "n="}, {Expr: []Value{NewWord("rv")}}}))     // interp string
	om.Set("s", NewString("keep"))                                                              // plain value
	om.Set("x", NewXmlInterp(XmlTmpl{Tag: "q", Cren: []XmlCren{{Kind: XmlCrenLit, Lit: "z"}}})) // xml interp
	rm := mapOf("a", NewInteger(7))
	om.Set("r", NewReach(ReachInfo{Receiver: []Value{rm}, Segments: []ReachSeg{{KeyLit: NewString("a")}}, Eval: true}))

	got := evalMapVal(t, r, om)
	m, err := AsMap(got)
	if err != nil {
		t.Fatalf("result not a map: %v", got)
	}
	check := func(key, want string) {
		t.Helper()
		v, ok := m.Get(key)
		if !ok {
			t.Errorf("key %s missing", key)
			return
		}
		if s := v.String(); !strings.Contains(s, want) {
			t.Errorf("%s = %q, want %q", key, s, want)
		}
	}
	check("w", "10")
	check("p", "3")
	check("multi", "1")
	check("i", "n=10")
	check("s", "keep")
	check("x", "z")
	check("r", "7")
	if v, _ := m.Get("multi"); !v.Parent.Equal(TList) {
		t.Errorf("multi-result paren did not become a list: %v", v)
	}
}

func TestAutoEvalMapComputedKeys(t *testing.T) {
	r := covRegistry(t, nil)
	InstallDef(r, "sk", NewString("skey"))
	InstallDef(r, "ak", NewAtom("akey"))
	InstallDef(r, "ik", NewInteger(9))
	om := NewOrderedMap()
	om.Set("sk", NewInteger(1))
	om.Set("ak", NewInteger(2))
	om.Set("ik", NewInteger(3))
	om.Meta = map[string]any{"ck": map[string]bool{"sk": true, "ak": true, "ik": true}}

	got := evalMapVal(t, r, om)
	m, _ := AsMap(got)
	for _, want := range []string{"skey", "akey", "9"} {
		if _, ok := m.Get(want); !ok {
			t.Errorf("computed key %q missing: %v", want, got)
		}
	}
}

func TestAutoEvalMapComputedKeyError(t *testing.T) {
	r := covRegistry(t, nil)
	om := NewOrderedMap()
	om.Set("no_such_key_word", NewInteger(1))
	om.Meta = map[string]any{"ck": map[string]bool{"no_such_key_word": true}}
	mv := NewMap(om)
	SetEval(&mv, true)
	_, err := NewTop(r).Run([]Value{mv})
	if err == nil || !strings.Contains(err.Error(), "computed key") {
		t.Errorf("computed key error = %v", err)
	}
}

func TestAutoEvalMapValueErrorPropagates(t *testing.T) {
	r := covRegistry(t, nil)
	om := NewOrderedMap()
	om.Set("bad", NewParenExpr([]Value{NewWord("no_such_word_y")}))
	mv := NewMap(om)
	SetEval(&mv, true)
	if _, err := NewTop(r).Run([]Value{mv}); err == nil {
		t.Error("map value error did not propagate")
	}
}

func TestAutoEvalMapConsumedArg(t *testing.T) {
	// A map ARG consumed by a native word auto-evaluates (consumed=true).
	setup := func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "cmsize",
			Signatures: []Signature{{
				Args: []*Type{TMap},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					m, err := AsMap(args[0])
					if err != nil {
						return nil, err
					}
					return []Value{NewInteger(int64(len(m.Keys())))}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	}
	r := covRegistry(t, setup)
	om := NewOrderedMap()
	om.Set("a", NewParenExpr([]Value{NewWord("cadd"), NewInteger(1), NewInteger(1)}))
	om.Set("b", NewInteger(2))
	mv := NewMap(om)
	SetEval(&mv, true)
	out, err := NewTop(r).Run([]Value{NewWord("cmsize"), mv})
	if err != nil {
		t.Fatalf("consumed map: %v", err)
	}
	if got := renderAll(out); got != "2" {
		t.Errorf("got %q, want 2", got)
	}
}

// --- autoEvalList arms ---------------------------------------------------------------

func TestAutoEvalListResidualAndConsumed(t *testing.T) {
	r := covRegistry(t, nil)
	// Residual: a computed list at end of run evaluates in place.
	lv := NewList([]Value{NewWord("cadd"), NewInteger(2), NewInteger(3)})
	SetEval(&lv, true)
	out, err := NewTop(r).Run([]Value{lv})
	if err != nil {
		t.Fatalf("residual list: %v", err)
	}
	if got := renderAll(out); got != "[5]" {
		t.Errorf("residual list = %q", got)
	}

	// Consumed: the same list as a clen arg evaluates before the call.
	lv2 := NewList([]Value{NewWord("cadd"), NewInteger(2), NewInteger(3)})
	SetEval(&lv2, true)
	out, err = NewTop(r).Run([]Value{NewWord("clen"), lv2})
	if err != nil {
		t.Fatalf("consumed list: %v", err)
	}
	if got := renderAll(out); got != "1" {
		t.Errorf("consumed list len = %q", got)
	}

	// Empty list short-circuits.
	le := NewList(nil)
	SetEval(&le, true)
	out, err = NewTop(r).Run([]Value{le})
	if err != nil || len(out) != 1 {
		t.Fatalf("empty list: %v %s", err, renderAll(out))
	}

	// Error inside a residual list propagates.
	lbad := NewList([]Value{NewWord("no_such_word_z")})
	SetEval(&lbad, true)
	if _, err := NewTop(r).Run([]Value{lbad}); err == nil {
		t.Error("list eval error did not propagate")
	}
}
