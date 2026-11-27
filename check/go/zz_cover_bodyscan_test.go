package check

// White-box tests for the body-scan family in carrier.go: the sentinel
// scanners (valueHasSentinel / BodyHasSentinel / scanBodyContainers /
// xmlTmplScan), the transitive callee scan (BodyHasSentinelDeep /
// calleeValueLeaksFlow / calleeLeaksFlow / fnDefLeaksFlow), the fn-local
// fn body gate (BodyRefsFnLocalFn), and the callback-body-name predicate
// (isCallbackBodyName). Each test asserts the behaviour the function's
// doc comment states.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// bsFn builds a Function value whose single boru-bodied overload holds
// the given body tokens — the constructed-fn-payload shape the scanners
// classify.
func bsFn(body ...core.Value) core.Value {
	return core.NewFunction(core.FnDefInfo{Signatures: []core.FnSig{{
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru(body),
	}}})
}

// --- valueHasSentinel / BodyHasSentinel ------------------------------------

func TestValueHasSentinelWordsAndLists(t *testing.T) {
	for _, w := range []string{"break", "continue", "return"} {
		if !valueHasSentinel(core.NewWord(w)) {
			t.Errorf("a bare %s is a live sentinel", w)
		}
	}
	if valueHasSentinel(core.NewWord("add")) {
		t.Error("an ordinary word is not a sentinel")
	}
	// A sentinel nested in an inner list is found (the recursive list
	// element hit), and a clean list scans to false.
	nested := core.NewList([]core.Value{
		core.NewInteger(1),
		core.NewList([]core.Value{core.NewWord("continue")}),
	})
	if !BodyHasSentinel(nested) {
		t.Error("a sentinel inside a nested list must be found")
	}
	if BodyHasSentinel(core.NewList([]core.Value{core.NewInteger(1), core.NewString("s")})) {
		t.Error("an inert list has no sentinel")
	}
}

func TestValueHasSentinelIgnoresQuotedFlag(t *testing.T) {
	// Doc: quote marks the LIST as data, not its tokens as
	// non-executable — a quoted `break` handed to do/each is live.
	q := core.NewList([]core.Value{core.NewWord("break")})
	core.SetQuoted(&q, true)
	if !BodyHasSentinel(q) {
		t.Error("a quoted list's break is a live sentinel — .Quoted must not be honoured")
	}
}

func TestValueHasSentinelSkipsNestedFnBodies(t *testing.T) {
	// Doc: nested fn bodies are skipped — their sentinel targets their
	// own scope.
	fnv := bsFn(core.NewWord("break"))
	if valueHasSentinel(fnv) {
		t.Error("a nested FnDefInfo payload's break targets its own scope")
	}
	if BodyHasSentinel(core.NewList([]core.Value{fnv})) {
		t.Error("a fn value inside a body list must not trip the syntactic scan")
	}
}

// --- scanBodyContainers ----------------------------------------------------

func TestScanBodyContainersParenExpr(t *testing.T) {
	hit := core.NewParenExpr([]core.Value{core.NewWord("add"), core.NewWord("break")})
	if !valueHasSentinel(hit) {
		t.Error("a paren-expr token runs as code: its break is live")
	}
	clean := core.NewParenExpr([]core.Value{core.NewWord("add"), core.NewInteger(1)})
	if valueHasSentinel(clean) {
		t.Error("a paren-expr with no sentinel scans to false")
	}
}

func TestScanBodyContainersInterpString(t *testing.T) {
	hit := core.NewInterpString([]core.InterpPart{
		{Lit: "x="},
		{Expr: []core.Value{core.NewWord("break")}},
	})
	if !valueHasSentinel(hit) {
		t.Error("an interp-string ${break} runs as code: it is live")
	}
	clean := core.NewInterpString([]core.InterpPart{
		{Lit: "x="},
		{Expr: []core.Value{core.NewWord("n")}},
	})
	if valueHasSentinel(clean) {
		t.Error("an interp-string with no sentinel scans to false")
	}
}

func TestScanBodyContainersMap(t *testing.T) {
	om := core.NewOrderedMap()
	om.Set("k", core.NewInteger(1))
	om.Set("b", core.NewWord("break"))
	if !valueHasSentinel(core.NewMap(om)) {
		t.Error("a map VALUE runs as code: {b: break} is live")
	}
	clean := core.NewOrderedMap()
	clean.Set("k", core.NewInteger(1))
	if valueHasSentinel(core.NewMap(clean)) {
		t.Error("a map with inert values has no sentinel")
	}
	// A typed map literal ({:Integer}) carries a ChildTypeInfo payload
	// with no concrete entries: AsMap yields no readable map (m == nil)
	// and the scan reports false rather than exploding.
	typed := core.NewTypedMap(core.NewTypeLiteral(core.TInteger))
	if valueHasSentinel(typed) {
		t.Error("a typed map with no entries has nothing to scan")
	}
}

func TestScanBodyContainersDirectListAndLeaf(t *testing.T) {
	// Called directly (as the transitive scan does) the list arm is
	// scanBodyContainers' own.
	hit := core.NewList([]core.Value{core.NewInteger(1), core.NewWord("break")})
	if !scanBodyContainers(hit, valueHasSentinel) {
		t.Error("a list element hit must be reported")
	}
	clean := core.NewList([]core.Value{core.NewInteger(1)})
	if scanBodyContainers(clean, valueHasSentinel) {
		t.Error("a clean list scans to false")
	}
	// Doc: a non-container value returns false — leaf classification
	// belongs to the caller's scan.
	if scanBodyContainers(core.NewInteger(5), valueHasSentinel) {
		t.Error("a scalar leaf is not a container")
	}
	if scanBodyContainers(core.NewWord("break"), valueHasSentinel) {
		t.Error("even a sentinel WORD is the caller's leaf, not a container")
	}
}

// --- xmlTmplScan -----------------------------------------------------------

func TestXmlTmplScanAttributeHole(t *testing.T) {
	tmpl := core.XmlTmpl{Tag: "p", Attr: []core.XmlAttrTmpl{{
		Name: "class",
		Parts: []core.InterpPart{
			{Lit: "c-"},
			{Expr: []core.Value{core.NewWord("break")}},
		},
	}}}
	if !valueHasSentinel(core.NewXmlInterp(tmpl)) {
		t.Error("an attribute ${break} hole runs as code: it is live")
	}
}

func TestXmlTmplScanExprChild(t *testing.T) {
	tmpl := core.XmlTmpl{Tag: "p", Cren: []core.XmlCren{
		{Kind: core.XmlCrenLit, Lit: "text"},
		{Kind: core.XmlCrenExpr, Expr: []core.Value{core.NewWord("continue")}},
	}}
	if !valueHasSentinel(core.NewXmlInterp(tmpl)) {
		t.Error("a child ${continue} hole runs as code: it is live")
	}
}

func TestXmlTmplScanNestedChildTemplate(t *testing.T) {
	inner := core.XmlTmpl{Tag: "i", Cren: []core.XmlCren{
		{Kind: core.XmlCrenExpr, Expr: []core.Value{core.NewWord("break")}},
	}}
	tmpl := core.XmlTmpl{Tag: "p", Cren: []core.XmlCren{
		{Kind: core.XmlCrenChild, Child: &inner},
	}}
	if !valueHasSentinel(core.NewXmlInterp(tmpl)) {
		t.Error("the scan must recurse into nested child templates")
	}
}

func TestXmlTmplScanCleanTemplate(t *testing.T) {
	// Every hole kind present and none holding a sentinel: attribute
	// parts (literal and expr), a literal child, a clean expr hole, a
	// nil child, and a clean nested child template.
	inner := core.XmlTmpl{Tag: "i", Cren: []core.XmlCren{
		{Kind: core.XmlCrenExpr, Expr: []core.Value{core.NewWord("y")}},
	}}
	tmpl := core.XmlTmpl{Tag: "p",
		Attr: []core.XmlAttrTmpl{{
			Name: "class",
			Parts: []core.InterpPart{
				{Lit: "lit"},
				{Expr: []core.Value{core.NewWord("x")}},
			},
		}},
		Cren: []core.XmlCren{
			{Kind: core.XmlCrenLit, Lit: "text"},
			{Kind: core.XmlCrenExpr, Expr: []core.Value{core.NewWord("z")}},
			{Kind: core.XmlCrenChild, Child: nil},
			{Kind: core.XmlCrenChild, Child: &inner},
		},
	}
	if valueHasSentinel(core.NewXmlInterp(tmpl)) {
		t.Error("a template with no sentinel in any hole scans to false")
	}
}

// --- BodyHasSentinelDeep and the transitive callee scan --------------------

// --- BodyRefsFnLocalFn -----------------------------------------------------
