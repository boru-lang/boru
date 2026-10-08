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

// bsInstall installs a boru-bodied fn on r so Lookup aggregates it —
// the registry-installed callee shape the transitive scan resolves.
func bsInstall(t *testing.T, r *core.Registry, name string, body ...core.Value) {
	t.Helper()
	core.InstallFnDef(r, name, core.FnDefInfo{Signatures: []core.FnSig{{
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru(body),
	}}})
}

func bsRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return r
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
	q.Quoted = true
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

func TestBodyHasSentinelDeepDirectAndNilRegistry(t *testing.T) {
	r := bsRegistry(t)
	// A syntactic sentinel short-circuits before any callee scan.
	if !BodyHasSentinelDeep(r, core.NewList([]core.Value{core.NewWord("break")})) {
		t.Error("a direct break is found by the syntactic scan")
	}
	// A nil registry cannot resolve callees: the deep scan degrades to
	// the syntactic answer.
	if BodyHasSentinelDeep(nil, core.NewList([]core.Value{core.NewWord("anything")})) {
		t.Error("nil registry: no callee resolution, no leak")
	}
}

func TestBodyHasSentinelDeepTransitiveCallee(t *testing.T) {
	r := bsRegistry(t)
	bsInstall(t, r, "zzbsleak", core.NewWord("break"), core.NewInteger(7))
	bsInstall(t, r, "zzbsmid", core.NewWord("zzbsleak"))

	// One hop: the body calls a fn whose body holds a bare break.
	body := core.NewList([]core.Value{core.NewWord("zzbsleak"), core.NewInteger(1)})
	if BodyHasSentinel(body) {
		t.Fatal("precondition: the syntactic scan must NOT see the callee's break")
	}
	if !BodyHasSentinelDeep(r, body) {
		t.Error("a callee's bare break leaks through the call into the caller's loop")
	}
	// Two hops: transitively through further calls.
	if !BodyHasSentinelDeep(r, core.NewList([]core.Value{core.NewWord("zzbsmid")})) {
		t.Error("the callee scan is transitive through intermediate fns")
	}
	// A continue leaks the same way.
	bsInstall(t, r, "zzbscont", core.NewWord("continue"))
	if !BodyHasSentinelDeep(r, core.NewList([]core.Value{core.NewWord("zzbscont")})) {
		t.Error("a callee's bare continue leaks too")
	}
	// An unknown word resolves to no fn def and contributes nothing.
	if BodyHasSentinelDeep(r, core.NewList([]core.Value{core.NewWord("zz_no_such_callee")})) {
		t.Error("an unresolvable word is not a leak")
	}
}

func TestBodyHasSentinelDeepReturnNotCounted(t *testing.T) {
	// Doc: `return` is deliberately NOT counted in callees — the fn
	// boundary consumes it.
	r := bsRegistry(t)
	bsInstall(t, r, "zzbsret", core.NewWord("return"), core.NewInteger(1))
	if BodyHasSentinelDeep(r, core.NewList([]core.Value{core.NewWord("zzbsret")})) {
		t.Error("a callee's return is consumed by its own fn boundary")
	}
}

func TestBodyHasSentinelDeepCycleGuard(t *testing.T) {
	r := bsRegistry(t)
	bsInstall(t, r, "zzbscyc", core.NewWord("zzbscyc"), core.NewInteger(1))
	if BodyHasSentinelDeep(r, core.NewList([]core.Value{core.NewWord("zzbscyc")})) {
		t.Error("a recursive callee with no sentinel terminates false (seen guard)")
	}
}

func TestBodyHasSentinelDeepConstructedFnPayload(t *testing.T) {
	// Doc (calleeValueLeaksFlow): a constructed FnDefInfo in a body is
	// treated as potentially applied — its break escapes to the caller.
	r := bsRegistry(t)
	leaky := bsFn(core.NewWord("break"))
	body := core.NewList([]core.Value{leaky})
	if BodyHasSentinel(body) {
		t.Fatal("precondition: the syntactic scan skips fn payloads")
	}
	if !BodyHasSentinelDeep(r, body) {
		t.Error("an applied fn value's break escapes to the caller")
	}
	if !calleeValueLeaksFlow(r, leaky, map[string]bool{}) {
		t.Error("the walker scans constructed fn payloads directly")
	}
}

func TestBodyHasSentinelDeepCalleeInsideContainer(t *testing.T) {
	// The transitive walker recurses through containers: a leaky callee
	// referenced inside a paren-expr in the body is still found.
	r := bsRegistry(t)
	bsInstall(t, r, "zzbspleak", core.NewWord("break"))
	body := core.NewList([]core.Value{
		core.NewParenExpr([]core.Value{core.NewWord("zzbspleak"), core.NewInteger(1)}),
	})
	if !BodyHasSentinelDeep(r, body) {
		t.Error("containers recurse in the callee scan")
	}
}

func TestCalleeLeaksFlowGuards(t *testing.T) {
	r := bsRegistry(t)
	bsInstall(t, r, "zzbsleak2", core.NewWord("break"))
	// An empty name resolves nothing.
	if calleeLeaksFlow(r, "", map[string]bool{}) {
		t.Error("empty name: nothing to resolve")
	}
	// A name already on the seen set is not re-scanned (the cycle guard),
	// even when its body genuinely leaks.
	if calleeLeaksFlow(r, "zzbsleak2", map[string]bool{"zzbsleak2": true}) {
		t.Error("a seen name must not be re-scanned")
	}
	if !calleeLeaksFlow(r, "zzbsleak2", map[string]bool{}) {
		t.Error("an unseen leaky name is found")
	}
}

func TestCalleeLeaksFlowNativeSigsContributeNothing(t *testing.T) {
	// Doc: native (Go-impl) sigs have no boru body and contribute nothing.
	r := bsRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zzbsnative",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{args[0]}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
	if calleeLeaksFlow(r, "zzbsnative", map[string]bool{}) {
		t.Error("a native word has no boru body to leak from")
	}
}

func TestFnDefLeaksFlowOwnRegistry(t *testing.T) {
	// Doc: a module fn's body words resolve in its OWN registry.
	rOuter := bsRegistry(t)
	rModule := bsRegistry(t)
	bsInstall(t, rModule, "zzbsmodleak", core.NewWord("break"))
	fd := core.FnDefInfo{
		Registry: rModule,
		Signatures: []core.Signature{{
			Returns: []*core.Type{core.TInteger},
			Impl:    core.Boru([]core.Value{core.NewWord("zzbsmodleak")}),
		}},
	}
	// The callee name is NOT defined in the outer registry...
	if calleeLeaksFlow(rOuter, "zzbsmodleak", map[string]bool{}) {
		t.Fatal("precondition: the module word must not resolve in the outer registry")
	}
	// ...yet the module fn's body still leaks, resolved via fd.Registry.
	if !fnDefLeaksFlow(rOuter, &fd, map[string]bool{}) {
		t.Error("a module fn's body words resolve in its own registry")
	}
}

// --- BodyRefsFnLocalFn -----------------------------------------------------

// bsBodySig builds the body-consuming dispatch signature shape the gate
// admits: one NoEvalArgs code-body position plus the given family marks.
func bsBodySig(effect core.CompileEffect, callable *core.CallableSpec) *core.Signature {
	return &core.Signature{
		NoEvalArgs:    map[int]bool{0: true},
		CompileEffect: effect,
		Callable:      callable,
	}
}

func TestBodyRefsFnLocalFnFamilyGate(t *testing.T) {
	r := bsRegistry(t)
	body := core.NewList([]core.Value{core.NewWord("zzanything")})
	// nil sig.
	if _, ok := BodyRefsFnLocalFn(r, nil, []core.Value{body}); ok {
		t.Error("nil sig: not a body-consuming dispatch")
	}
	// No NoEvalArgs positions.
	if _, ok := BodyRefsFnLocalFn(r, &core.Signature{CompileEffect: core.CompileFallbackBody}, []core.Value{body}); ok {
		t.Error("no NoEvalArgs: not a code-body word")
	}
	// Doc: structured-lowering words (if / for — NoEvalArgs but neither
	// CompileFallbackBody nor Callable) are excluded by the family gate.
	if _, ok := BodyRefsFnLocalFn(r, bsBodySig(core.CompileDefault, nil), []core.Value{body}); ok {
		t.Error("NoEvalArgs alone (if/for shape) must not match")
	}
}

func TestBodyRefsFnLocalFnModuleScope(t *testing.T) {
	// Doc: TopFnBaseline nil (module scope) keeps compiling.
	r := bsRegistry(t)
	r.Defs.Push("zzbstep", bsFn(core.NewInteger(1)))
	defer r.Defs.Pop("zzbstep")
	body := core.NewList([]core.Value{core.NewWord("zzbstep")})
	sig := bsBodySig(core.CompileFallbackBody, nil)
	if _, ok := BodyRefsFnLocalFn(r, sig, []core.Value{body}); ok {
		t.Error("no enclosing fn: nothing is fn-local")
	}
}

func TestBodyRefsFnLocalFnFindsFnLocalFn(t *testing.T) {
	r := bsRegistry(t)
	baseline := r.Defs.Snapshot() // zzbstep unbound: baseline depth 0
	r.PushFnBaseline(baseline)
	defer r.PopFnBaseline()
	r.Defs.Push("zzbstep", bsFn(core.NewInteger(1)))
	defer r.Defs.Pop("zzbstep")

	body := core.NewList([]core.Value{core.NewWord("zzbstep"), core.NewInteger(1)})
	sig := bsBodySig(core.CompileFallbackBody, nil)
	name, ok := BodyRefsFnLocalFn(r, sig, []core.Value{body})
	if !ok || name != "zzbstep" {
		t.Errorf("a fn-local fn named in the body must be reported, got (%q, %v)", name, ok)
	}
	// The Callable half of the family gate admits the same shape.
	name, ok = BodyRefsFnLocalFn(r, bsBodySig(core.CompileDefault, &core.CallableSpec{}), []core.Value{body})
	if !ok || name != "zzbstep" {
		t.Errorf("a CallableSpec word takes the same gate, got (%q, %v)", name, ok)
	}
}

func TestBodyRefsFnLocalFnFirstNameWins(t *testing.T) {
	r := bsRegistry(t)
	r.PushFnBaseline(r.Defs.Snapshot())
	defer r.PopFnBaseline()
	r.Defs.Push("zzbstepA", bsFn(core.NewInteger(1)))
	defer r.Defs.Pop("zzbstepA")
	r.Defs.Push("zzbstepB", bsFn(core.NewInteger(2)))
	defer r.Defs.Pop("zzbstepB")

	// Doc: returns the FIRST such name; the walk latches it and ignores
	// the later fn-local reference.
	body := core.NewList([]core.Value{core.NewWord("zzbstepB"), core.NewWord("zzbstepA")})
	sig := bsBodySig(core.CompileFallbackBody, nil)
	name, ok := BodyRefsFnLocalFn(r, sig, []core.Value{body})
	if !ok || name != "zzbstepB" {
		t.Errorf("the first fn-local fn wins, got (%q, %v)", name, ok)
	}
}

func TestBodyRefsFnLocalFnScopeAndValueExclusions(t *testing.T) {
	r := bsRegistry(t)
	// A module-scope fn: bound BEFORE the baseline snapshot, so
	// Depth == baseline — the utils-corpus house-rule shape keeps compiling.
	r.Defs.Push("zzbutil", bsFn(core.NewInteger(1)))
	defer r.Defs.Pop("zzbutil")
	r.PushFnBaseline(r.Defs.Snapshot())
	defer r.PopFnBaseline()
	// A fn-local VALUE def (non-Function binding): the mini-redis
	// accumulator shape, legitimately handled by lexical captures.
	r.Defs.Push("zzbacc", core.NewInteger(0))
	defer r.Defs.Pop("zzbacc")

	sig := bsBodySig(core.CompileFallbackBody, nil)
	for _, tc := range []struct {
		label string
		word  string
	}{
		{"module-scope fn", "zzbutil"},
		{"fn-local value def", "zzbacc"},
		{"unbound word", "zzb_unbound"},
	} {
		body := core.NewList([]core.Value{core.NewWord(tc.word)})
		if name, ok := BodyRefsFnLocalFn(r, sig, []core.Value{body}); ok {
			t.Errorf("%s must not match, got %q", tc.label, name)
		}
	}
}

func TestBodyRefsFnLocalFnSkipsEvaluatedPositions(t *testing.T) {
	r := bsRegistry(t)
	r.PushFnBaseline(r.Defs.Snapshot())
	defer r.PopFnBaseline()
	r.Defs.Push("zzbstep", bsFn(core.NewInteger(1)))
	defer r.Defs.Pop("zzbstep")

	// Only NoEvalArgs positions are body code: position 0 references the
	// fn-local fn but is an EVALUATED arg, position 1 is the scanned body
	// and holds nothing fn-local.
	sig := &core.Signature{
		NoEvalArgs:    map[int]bool{1: true},
		CompileEffect: core.CompileFallbackBody,
	}
	args := []core.Value{
		core.NewList([]core.Value{core.NewWord("zzbstep")}),
		core.NewList([]core.Value{core.NewInteger(3)}),
	}
	if name, ok := BodyRefsFnLocalFn(r, sig, args); ok {
		t.Errorf("an evaluated position must be skipped, got %q", name)
	}
}
