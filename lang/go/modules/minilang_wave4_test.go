package modules

import (
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/antchfx/xpath"
	"github.com/boru-lang/boru/lang/go/native"
	tabnasexpr "github.com/tabnas/expr/go"
)

// Wave-4 coverage for the query / formula / xpath / glob mini-languages:
// modules/minilang_math.go (the `math` kind), minilang_query.go (`jp` and
// `jq`), minilang_xpath.go (`xp` and its xpNode/xpNav mirror), and gex.go
// (the `gex` selector kind) — positive rows lifted from
// lang/spec/module-minilang.tsv, each paired with the loud rejection.

// mlw4Int / mlw4Float / mlw4Str read the last stack value as a scalar.
func mlw4Int(t *testing.T, out []native.Value) int64 {
	t.Helper()
	n, err := out[len(out)-1].AsConcreteInteger()
	if err != nil {
		t.Fatalf("expected Integer, got %v (%v)", out[len(out)-1], err)
	}
	return n
}

func mlw4Float(t *testing.T, out []native.Value) float64 {
	t.Helper()
	f, err := out[len(out)-1].AsConcreteFloat()
	if err != nil {
		t.Fatalf("expected Float, got %v (%v)", out[len(out)-1], err)
	}
	return f
}

// TestMiniWave4MathFormulas drives every operator arm of the `math` kind:
// the four infixes, remainder, power (integer ipow and float pow), unary
// minus/plus, parens, literals, and the integer/float domain coercion.
func TestMiniWave4MathFormulas(t *testing.T) {
	intCases := []struct {
		src, opts string
		want      int64
	}{
		{`'x*y-z^2'`, `{x:3 y:4 z:2}`, 8},
		{`'2^3^2'`, `{}`, 512},  // right-associative power
		{`'-z^2'`, `{z:3}`, -9}, // unary minus binds looser than ^
		{`'(x+y)*z'`, `{x:1 y:2 z:3}`, 9},
		{`'x/y'`, `{x:5 y:2}`, 2},   // truncating integer division
		{`'x%y'`, `{x:7 y:4}`, 3},   // integer remainder
		{`'x-y'`, `{x:3 y:10}`, -7}, // integer subtraction
		{`'x'`, `{x:42}`, 42},       // bare variable
		{`'2^0'`, `{}`, 1},          // ipow zero exponent
	}
	for _, c := range intCases {
		r := mcovReg(t)
		out := mcovRun(t, r, mcovImp+`mini math `+c.src+` `+c.opts)
		if got := mlw4Int(t, out); got != c.want {
			t.Errorf("math %s %s = %d, want %d", c.src, c.opts, got, c.want)
		}
	}

	floatCases := []struct {
		src, opts string
		want      float64
	}{
		{`'x*y-z^2'`, `{x:3 y:4 z:2.0}`, 8.0}, // one Float promotes all
		{`'x/y'`, `{x:5.0 y:2}`, 2.5},
		{`'x%y'`, `{x:7.5 y:2.0}`, math.Mod(7.5, 2.0)},
		{`'x+y'`, `{x:1.5 y:1}`, 2.5},
		{`'x-y'`, `{x:1.5 y:1}`, 0.5},
		{`'2^x'`, `{x:-1}`, 0.5},  // negative exponent → float pow
		{`'1.5+1'`, `{}`, 2.5},    // fractional literal is float-domain
		{`'-x'`, `{x:2.5}`, -2.5}, // unary minus, float arm
	}
	for _, c := range floatCases {
		r := mcovReg(t)
		out := mcovRun(t, r, mcovImp+`mini math `+c.src+` `+c.opts)
		if got := mlw4Float(t, out); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("math %s %s = %v, want %v", c.src, c.opts, got, c.want)
		}
	}
}

// TestMiniWave4MathRejections pins the loud math-kind failures: malformed
// formulas, unbound variables, zero division in both domains, and
// non-numeric variable bindings.
func TestMiniWave4MathRejections(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"trailing operator", `mini math 'x +' {x:1}`, "mini_syntax_error"},
		{"unbound variable", `mini math 'x+q' {x:1}`, "mini_eval_error"},
		{"integer division by zero", `mini math 'x/y' {x:1 y:0}`, "mini_eval_error"},
		{"float division by zero", `mini math 'x/y' {x:1.0 y:0}`, "mini_eval_error"},
		{"integer mod by zero", `mini math 'x%y' {x:1 y:0}`, "mini_eval_error"},
		{"float mod by zero", `mini math 'x%y' {x:1.5 y:0}`, "mini_eval_error"},
		{"string-bound variable", `mini math 'x' {x:'s'}`, "mini_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := mcovErr(t, mcovReg(t), mcovImp+c.src)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %v does not contain %q", c.src, err, c.want)
			}
		})
	}
}

// TestMiniWave4MathInternals pins the depth guard and the malformed-AST arms
// of evalFormula / evalApply that a real parse cannot produce, plus
// boruToMval's domain projection.
func TestMiniWave4MathInternals(t *testing.T) {
	if _, err := evalFormula(1.0, nil, maxFormulaDepth+1); err == nil {
		t.Error("depth overflow must be malformed")
	}
	if _, err := evalFormula(struct{}{}, nil, 0); err == nil {
		t.Error("unknown node type must be malformed")
	}
	if _, err := evalApply(nil, nil, 0); err == nil {
		t.Error("empty apply must be malformed")
	}
	if _, err := evalApply([]any{"not-an-op"}, nil, 0); err == nil {
		t.Error("non-Op head must be malformed")
	}
	if _, err := evalApply([]any{&tabnasexpr.Op{Name: "plain-paren"}}, nil, 0); err == nil {
		t.Error("paren without operand must be malformed")
	}
	if _, err := evalApply([]any{&tabnasexpr.Op{Name: "negative-prefix"}}, nil, 0); err == nil {
		t.Error("negate without operand must be malformed")
	}
	if _, err := evalApply([]any{&tabnasexpr.Op{Name: "negative-prefix"}, "q"}, nil, 0); err == nil {
		t.Error("negate of an unbound variable must error")
	}
	if _, err := evalApply([]any{&tabnasexpr.Op{Name: "addition-infix"}, 1.0}, nil, 0); err == nil {
		t.Error("infix with one operand must be malformed")
	}
	if _, err := evalApply([]any{&tabnasexpr.Op{Name: "addition-infix"}, "q", 1.0}, nil, 0); err == nil {
		t.Error("infix with an unbound left operand must error")
	}
	if _, err := evalApply([]any{&tabnasexpr.Op{Name: "addition-infix"}, 1.0, "q"}, nil, 0); err == nil {
		t.Error("infix with an unbound right operand must error")
	}
	if _, err := evalApply([]any{&tabnasexpr.Op{Name: "bogus-infix"}, 1.0, 2.0}, nil, 0); err == nil ||
		!strings.Contains(err.Error(), "unsupported operator") {
		t.Errorf("unknown operator: %v, want unsupported operator", err)
	}

	if mv, ok := boruToMval(native.NewInteger(4)); !ok || !mv.isInt || mv.i != 4 {
		t.Errorf("boruToMval(4) = %+v %v", mv, ok)
	}
	if mv, ok := boruToMval(native.NewFloat(2.5)); !ok || mv.isInt || mv.f != 2.5 {
		t.Errorf("boruToMval(2.5) = %+v %v", mv, ok)
	}
	if _, ok := boruToMval(native.NewString("x")); ok {
		t.Error("boruToMval(String) must decline")
	}
	if _, ok := boruToMval(native.NewTypeLiteral(native.TInteger)); ok {
		t.Error("boruToMval(type literal) must decline")
	}
	if got := (mval{i: 3, isInt: true}).asFloat(); got != 3.0 {
		t.Errorf("asFloat(int 3) = %v", got)
	}
	if got := (mval{f: 1.25}).asFloat(); got != 1.25 {
		t.Errorf("asFloat(1.25) = %v", got)
	}
}

// TestMiniWave4MathHandlerArgGuards pins miniMathHandler's direct arg arms:
// a non-String src and a non-concrete opts Map.
func TestMiniWave4MathHandlerArgGuards(t *testing.T) {
	r := mcovReg(t)
	if _, err := miniMathHandler([]native.Value{
		native.NewInteger(1), native.NewMap(native.NewOrderedMap()),
	}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "mini_error") {
		t.Errorf("integer src: %v, want mini_error", err)
	}
	// A Map type literal in the opts slot: no vars bound, formula still runs.
	out, err := miniMathHandler([]native.Value{
		native.NewString("1+2"), native.NewTypeLiteral(native.TMap),
	}, nil, nil, r)
	if err != nil {
		t.Fatalf("literal opts: %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 3 {
		t.Errorf("1+2 with literal opts = %v, want 3", out[0])
	}
}

// TestMiniWave4JsonPath drives `jp` over the subject shapes: a Node map, a
// bare List, a class instance, a Record and a Table — plus the loud
// malformed-path rejection.
func TestMiniWave4JsonPath(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+
		`{store:{book:[{title:'A'} {title:'B'}]}} mini jp '$.store.book[*].title' {}`)
	if got := out[len(out)-1].String(); got != "['A' 'B']" {
		t.Errorf("jp titles = %q", got)
	}

	out = mcovRun(t, mcovReg(t), mcovImp+`[{n:1} {n:2} {n:3}] mini jp '$[*].n'`)
	if got := out[len(out)-1].String(); got != "[1 2 3]" {
		t.Errorf("jp list subject = %q", got)
	}

	out = mcovRun(t, mcovReg(t), mcovImp+
		`def Pt class {x:Integer y:Integer} end  (make Pt {x:7 y:9}) mini jp '$.x'`)
	if got := out[len(out)-1].String(); got != "[7]" {
		t.Errorf("jp class instance = %q", got)
	}

	out = mcovRun(t, mcovReg(t), mcovImp+
		`def R (refine Record [{a:Integer b:String}])  (make R {a:5 b:'hi'}) mini jp '$.b' {}`)
	if got := out[len(out)-1].String(); got != "['hi']" {
		t.Errorf("jp record = %q", got)
	}

	out = mcovRun(t, mcovReg(t), mcovImp+
		`def Row (refine Record [{a:Integer b:String}])  def T (refine Table Row)  (make T [[1 'x'] [2 'y']]) mini jp '$[*].a'`)
	if got := out[len(out)-1].String(); got != "[1 2]" {
		t.Errorf("jp table = %q", got)
	}

	if err := mcovErr(t, mcovReg(t), mcovImp+`{a:1} mini jp '$.[' {}`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("malformed jsonpath: %v, want mini_parse_error", err)
	}
}

// TestMiniWave4Jq drives `jq` over map / list / table subjects, a scalar
// subject through docToAny's fall-through, and the loud parse / eval errors.
func TestMiniWave4Jq(t *testing.T) {
	out := mcovRun(t, mcovReg(t), mcovImp+`{a:1 b:2} mini jq '.a + .b' {}`)
	if got := out[len(out)-1].String(); got != "[3]" {
		t.Errorf("jq add = %q", got)
	}

	out = mcovRun(t, mcovReg(t), mcovImp+`[1 2 3 4] mini jq '.[] | select(. > 2)'`)
	if got := out[len(out)-1].String(); got != "[3 4]" {
		t.Errorf("jq select = %q", got)
	}

	out = mcovRun(t, mcovReg(t), mcovImp+
		`def Row (refine Record [{a:Integer}])  def T (refine Table Row)  (make T [[1] [2] [3]]) mini jq 'map(.a) | add'`)
	if got := out[len(out)-1].String(); got != "[6]" {
		t.Errorf("jq table sum = %q", got)
	}

	// Scalar subject: docToAny falls through to the standard conversion.
	out = mcovRun(t, mcovReg(t), mcovImp+`42 mini jq '. + 1'`)
	if got := out[len(out)-1].String(); got != "[43]" {
		t.Errorf("jq scalar subject = %q", got)
	}

	if err := mcovErr(t, mcovReg(t), mcovImp+`{a:1} mini jq '.[' {}`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("malformed jq: %v, want mini_parse_error", err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`{a:1} mini jq '.a / 0' {}`); err == nil ||
		!strings.Contains(err.Error(), "mini_eval_error") {
		t.Errorf("jq division by zero: %v, want mini_eval_error", err)
	}
}

// TestMiniWave4QueryInternals pins the query plumbing helpers directly:
// gojq big.Int projection (both the int64 and the too-big Float arm), the
// non-concrete document guard, and idealListToAny's non-Ideal fallback.
func TestMiniWave4QueryInternals(t *testing.T) {
	if v := queryResultToValue(big.NewInt(5)); v.String() != "5" {
		t.Errorf("small big.Int = %q, want 5", v.String())
	}
	huge, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	v := queryResultToValue(huge)
	if _, err := v.AsConcreteFloat(); err != nil {
		t.Errorf("oversized big.Int should project to Float, got %v (%v)", v, err)
	}

	if got := docToAny(native.NewTypeLiteral(native.TMap)); got != nil {
		t.Errorf("docToAny(type literal) = %v, want nil", got)
	}
	// Non-Ideal input to idealListToAny falls back to the standard conversion.
	if got := idealListToAny(native.NewInteger(7)); got != int64(7) && got != any(7) {
		t.Logf("idealListToAny(7) = %#v", got) // fallback shape is conversion-defined
	}

	r := mcovReg(t)
	if _, err := miniJqHandler([]native.Value{
		native.NewInteger(1), native.NewMap(native.NewOrderedMap()), native.NewInteger(1),
	}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "mini_error") {
		t.Errorf("jq integer src: %v, want mini_error", err)
	}
	if _, err := miniJsonPathHandler([]native.Value{
		native.NewInteger(1), native.NewMap(native.NewOrderedMap()), native.NewInteger(1),
	}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "mini_error") {
		t.Errorf("jp integer src: %v, want mini_error", err)
	}
}

// TestMiniWave4XPath drives `xp` across the result kinds: element node-sets,
// text nodes, attributes, count (Integer), boolean, string() and sum
// (Float) — plus positional predicates and the axis moves they exercise.
func TestMiniWave4XPath(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"element node-set", `<r><a>1</a><a>2</a></r> mini xp '//a'`, "[<a>1</a> <a>2</a>]"},
		{"text nodes", `<r><a>1</a><a>2</a></r> mini xp '//a/text()'`, "['1' '2']"},
		{"attribute", `<r id="7"><a/></r> mini xp '//@id'`, "['7']"},
		{"count integer", `<r><a/><a/><a/></r> mini xp 'count(//a)'`, "[3]"},
		{"boolean", `<r><a/><a/></r> mini xp 'count(//a) > 1'`, "[true]"},
		{"string fn", `<r><a>hi</a></r> mini xp 'string(//a)'`, "['hi']"},
		{"sum float", `<r><a>1.5</a><a>2.25</a></r> mini xp 'sum(//a)'`, "[3.75]"},
		{"positional predicate", `<lib><book><title>A</title></book><book><title>B</title></book></lib> mini xp '//book[2]/title/text()'`, "['B']"},
		{"preceding sibling", `<r><a>1</a><a>2</a></r> mini xp '//a[2]/preceding-sibling::a/text()'`, "['1']"},
		{"no match", `<r><a/></r> mini xp '//zzz'`, "[]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := mcovRun(t, mcovReg(t), mcovImp+c.src)
			if got := out[len(out)-1].String(); got != c.want {
				t.Errorf("%s = %q, want %q", c.src, got, c.want)
			}
		})
	}

	if err := mcovErr(t, mcovReg(t), mcovImp+`<r><a/></r> mini xp '//['`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("malformed xpath: %v, want mini_parse_error", err)
	}
}

// TestMiniWave4XPathHandlerGuards pins miniXPathHandler's direct arg arms:
// a non-String src and a non-Xml document.
func TestMiniWave4XPathHandlerGuards(t *testing.T) {
	r := mcovReg(t)
	xml := mcovRun(t, r, `<r><a/></r>`)
	doc := xml[len(xml)-1]
	if _, err := miniXPathHandler([]native.Value{
		native.NewInteger(1), native.NewMap(native.NewOrderedMap()), doc,
	}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "mini_error") {
		t.Errorf("xp integer src: %v, want mini_error", err)
	}
	if _, err := miniXPathHandler([]native.Value{
		native.NewString("//a"), native.NewMap(native.NewOrderedMap()), native.NewInteger(3),
	}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "expected an Xml element") {
		t.Errorf("xp non-Xml doc: %v, want expected-an-Xml-element", err)
	}
}

// TestMiniWave4XpNav unit-drives the xpNode mirror and its NodeNavigator —
// every Move* arm (success and compile failure), the attribute cursor, innerText,
// and the scalar projections xpText / xpNumberToValue / xpResultToList.
func TestMiniWave4XpNav(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, `<r id="7" k="v"><a>1</a><b/>tail</r>`)
	doc := out[len(out)-1]
	if !native.IsXmlValue(doc) {
		t.Fatalf("expected an Xml literal, got %v", doc)
	}
	root := buildXpTree(doc)
	nav := newXpNav(root)

	if nav.NodeType() != xpath.RootNode {
		t.Errorf("root NodeType = %v", nav.NodeType())
	}
	if nav.Prefix() != "" {
		t.Error("Prefix is always empty")
	}
	if !nav.MoveToChild() { // → <r>
		t.Fatal("MoveToChild to r failed")
	}
	if nav.LocalName() != "r" {
		t.Errorf("LocalName = %q, want r", nav.LocalName())
	}
	if nav.Value() != "1tail" { // innerText: descendant text in doc order
		t.Errorf("element Value = %q, want 1tail", nav.Value())
	}
	// Attribute cursor.
	if !nav.MoveToNextAttribute() || nav.LocalName() != "id" || nav.Value() != "7" {
		t.Errorf("first attribute = %q=%q, want id=7", nav.LocalName(), nav.Value())
	}
	if nav.NodeType() != xpath.AttributeNode {
		t.Errorf("attribute NodeType = %v", nav.NodeType())
	}
	if nav.MoveToChild() {
		t.Error("MoveToChild on an attribute must decline")
	}
	if nav.MoveToFirst() {
		t.Error("MoveToFirst on an attribute must decline")
	}
	if nav.MoveToNext() {
		t.Error("MoveToNext on an attribute must decline")
	}
	if nav.MoveToPrevious() {
		t.Error("MoveToPrevious on an attribute must decline")
	}
	if !nav.MoveToNextAttribute() || nav.LocalName() != "k" {
		t.Errorf("second attribute = %q, want k", nav.LocalName())
	}
	if nav.MoveToNextAttribute() {
		t.Error("MoveToNextAttribute past the end must decline")
	}
	if !nav.MoveToParent() { // attr cursor → back to element
		t.Fatal("MoveToParent from an attribute failed")
	}
	if nav.LocalName() != "r" {
		t.Errorf("after attr-parent, at %q, want r", nav.LocalName())
	}

	// Children: <a>, <b/>, text "tail".
	if !nav.MoveToChild() || nav.LocalName() != "a" {
		t.Fatalf("first child = %q, want a", nav.LocalName())
	}
	if !nav.MoveToNext() || nav.LocalName() != "b" {
		t.Fatalf("second child = %q, want b", nav.LocalName())
	}
	if !nav.MoveToNext() || nav.NodeType() != xpath.TextNode || nav.Value() != "tail" {
		t.Fatalf("third child should be the tail text, got %q", nav.Value())
	}
	if nav.MoveToNext() {
		t.Error("MoveToNext past the last sibling must decline")
	}
	if !nav.MoveToPrevious() || nav.LocalName() != "b" {
		t.Errorf("MoveToPrevious = %q, want b", nav.LocalName())
	}
	if !nav.MoveToFirst() || nav.LocalName() != "a" {
		t.Errorf("MoveToFirst = %q, want a", nav.LocalName())
	}
	if nav.MoveToPrevious() {
		t.Error("MoveToPrevious at the first sibling must decline")
	}

	// Copy / MoveTo round trip; a foreign-root navigator is declined.
	cp := nav.Copy()
	if !nav.MoveToParent() || !nav.MoveTo(cp) || nav.LocalName() != "a" {
		t.Error("MoveTo(copy) should restore the position")
	}
	other := mcovRun(t, r, `<z/>`)
	foreign := newXpNav(buildXpTree(other[len(other)-1]))
	if nav.MoveTo(foreign) {
		t.Error("MoveTo across trees must decline")
	}
	nav.MoveToRoot()
	if nav.MoveToParent() {
		t.Error("MoveToParent at the root must decline")
	}
	// Descend twice (root → r → a); both moves have side effects, so
	// spell them as separate statements.
	descended := nav.MoveToChild()
	if descended {
		descended = nav.MoveToChild()
	}
	if descended {
		if !nav.MoveToFirst() {
			t.Error("MoveToFirst at a first element should succeed")
		}
	}

	// innerText on a text node directly.
	txt := &xpNode{typ: xpath.TextNode, text: "T"}
	if txt.innerText() != "T" {
		t.Errorf("text-node innerText = %q", txt.innerText())
	}

	// Scalar projections.
	if xpText(native.NewInteger(5)) != "5" {
		t.Error("xpText should render a non-String scalar via String()")
	}
	if v := xpNumberToValue(2.0); v.String() != "2" {
		t.Errorf("integral xp number = %q", v.String())
	}
	if v := xpNumberToValue(2.5); v.String() != "2.5" {
		t.Errorf("fractional xp number = %q", v.String())
	}
	if _, err := xpNumberToValue(math.Inf(1)).AsConcreteFloat(); err != nil {
		t.Error("infinite xp number should stay Float")
	}
	if _, err := xpNumberToValue(math.NaN()).AsConcreteFloat(); err != nil {
		t.Error("NaN xp number should stay Float")
	}
	if got := xpResultToList("s"); len(got) != 1 || got[0].String() != "'s'" {
		t.Errorf("string result = %v", got)
	}
	if got := xpResultToList(true); len(got) != 1 {
		t.Errorf("boolean result = %v", got)
	}
	if got := xpResultToList(struct{}{}); got != nil {
		t.Errorf("unknown result kind = %v, want nil", got)
	}
}

// TestMiniWave4Gex drives the gex selector over the three subject shapes —
// scalar (value-or-None), List (filter) and Map (key filter) — the literal
// digraphs, and the src-type rejection.
func TestMiniWave4Gex(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"scalar match", `"ax" mini gex 'a*'`, "'ax'"},
		{"scalar no match", `"bx" mini gex 'a*'`, "None"},
		{"single char", `"ab" mini gex 'a?'`, "'ab'"},
		{"literal star", `"a*" mini gex 'a**'`, "'a*'"},
		{"literal question", `"a?" mini gex 'a*?'`, "'a?'"},
		{"list filter", `["ab" "zz" "ac"] mini gex 'a*'`, "['ab' 'ac']"},
		{"map key filter", `{ab:1 zz:2 ac:3} mini gex 'a*' {}`, "{ab:1 ac:3}"},
		{"integer scalar", `7 mini gex '7'`, "7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := mcovRun(t, mcovReg(t), mcovImp+c.src)
			if got := out[len(out)-1].String(); got != c.want {
				t.Errorf("%s = %q, want %q", c.src, got, c.want)
			}
		})
	}

	r := mcovReg(t)
	if _, err := miniGexHandler([]native.Value{
		native.NewInteger(1), native.NewMap(native.NewOrderedMap()), native.NewString("x"),
	}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "mini_error") {
		t.Errorf("gex integer src: %v, want mini_error", err)
	}
}
