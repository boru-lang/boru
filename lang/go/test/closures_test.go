package test

import (
	"strings"
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"

	"github.com/boru-lang/boru/lang/go/native"
)

// boru fns and lambdas use implicit lexical capture: at construction
// time the engine walks the body's bare-Word references, identifies
// which ones resolve to bindings made by an enclosing fn (params or
// local defs), and snapshots their current values into the FnDefInfo.
// At dispatch the captured names are installed as defs alongside per-
// call named params; the body sees its captures regardless of what
// happened to the outer scope. Module-global names and forward refs
// stay dynamic (resolved via registry at call time).
//
// The headline test is the factory pattern — `make-adder N` returning
// an inner closure that captures `N`. Today that pattern depended on
// the outer scope being live; the closure mechanism makes it work
// regardless.

func intResult(t *testing.T, vals []native.Value) int64 {
	t.Helper()
	if len(vals) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(vals), vals)
	}
	n, err := core.AsInteger(vals[0])
	if err != nil {
		t.Fatalf("expected Integer, got %s: %v", vals[0].Parent.String(), err)
	}
	return n
}

// A — Factory pattern via fn.
func TestClosureFactoryViaFn(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def make-adder fn [[x:Integer] [Function] [fn [[y:Integer] [Integer] [x add y]]]]`,
		`def add5 (make-adder 5)`,
		`add5 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 8 {
		t.Errorf("add5 3 → %d, want 8", got)
	}
}

// B — Factory via afn / =>.
func TestClosureFactoryViaArrow(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def make-adder ([x:Integer] => [([y:Integer] => [x add y])])`,
		`def add5 (make-adder 5)`,
		`add5 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 8 {
		t.Errorf("add5 3 → %d, want 8", got)
	}
}

// Independent captures: two `make-adder` calls produce two closures
// that don't share state.
func TestClosureIndependentCaptures(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def make-adder ([x:Integer] => [([y:Integer] => [x add y])])`,
		`def add5 (make-adder 5)`,
		`def add20 (make-adder 20)`,
		`add5 1`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 6 {
		t.Errorf("add5 1 → %d, want 6", got)
	}
	out, err = runNativeSteps(t, nil, []string{
		`def make-adder ([x:Integer] => [([y:Integer] => [x add y])])`,
		`def add20 (make-adder 20)`,
		`add20 1`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 21 {
		t.Errorf("add20 1 → %d, want 21", got)
	}
}

// D — Module-level (top-level) names stay dynamic. `x` is bound at
// module scope; the closure's body references it but it should NOT be
// captured. Reassigning x after closure construction changes what the
// body sees.
func TestClosureModuleLevelStaysDynamic(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def x 1`,
		`def f ([] => [x])`,
		`def x 2`,
		`f`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 2 {
		t.Errorf("f after `def x 2` → %d, want 2 (module x is dynamic)", got)
	}
}

// E — Recursion via forward reference: at construction `fact` isn't
// bound (the `def fact …` completes after the fn handler returns), so
// it isn't captured. Runtime registry lookup finds it.
func TestClosureRecursionForwardRef(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]`,
		`fact 5`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 120 {
		t.Errorf("fact 5 → %d, want 120", got)
	}
}

// F — Capture wins over module-level rebind. The inner def-local `n`
// is captured (enclosing-fn-local). After the outer fn returns, the
// closure's `n` is the captured one — module `n` is shadowed for the
// closure body's lookup.
func TestClosureShadowsModuleLevel(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def n 99`,
		`def f fn [[] [Function] [ def n 5  ([] => [n]) ]]`,
		`def g (f)`,
		`def n 7`,
		`g`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 5 {
		t.Errorf("g → %d, want 5 (captured n=5 shadows module n)", got)
	}
}

// H — Top-level fn captures nothing. Inspect the FnDefInfo's
// Captured field directly.
func TestClosureTopLevelFnHasNoCaptures(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`fn [[x:Integer] [Integer] [x add 1]]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	fnDef, ok := out[0].Data.(core.FnDefInfo)
	if !ok {
		t.Fatalf("payload type = %T, want FnDefInfo", out[0].Data)
	}
	if fnDef.Captured != nil {
		t.Errorf("top-level fn Captured = %v, want nil", fnDef.Captured)
	}
}

// Inner fn populates Captured. Construct the inner fn inside an outer
// fn body and verify the inner FnDefInfo has the outer param captured.
func TestClosureInnerFnHasCaptures(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def mk ([x:Integer] => [([y:Integer] => [x add y])])`,
		`mk 5`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	fnDef, ok := out[0].Data.(core.FnDefInfo)
	if !ok {
		t.Fatalf("payload type = %T, want FnDefInfo", out[0].Data)
	}
	if len(fnDef.Captured) != 1 {
		t.Fatalf("expected 1 capture, got %d: %v", len(fnDef.Captured), fnDef.Captured)
	}
	if fnDef.Captured[0].Name != "x" {
		t.Errorf("captured name = %q, want x", fnDef.Captured[0].Name)
	}
	n, _ := core.AsInteger(fnDef.Captured[0].Value)
	if n != 5 {
		t.Errorf("captured x = %d, want 5", n)
	}
}

// L — Param shadows capture. Inner fn has its own `x` param, distinct
// from outer's `x`. The inner body's `x` resolves to the param, not
// the capture.
func TestClosureParamShadowsCapture(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def mk ([x:Integer] => [([x:String] => [x])])`,
		`def mk5 (mk 5)`,
		`mk5 "hello"`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(out), out)
	}
	s, err := core.AsString(out[0])
	if err != nil {
		t.Fatalf("expected String, got %s: %v", out[0].Parent.String(), err)
	}
	if s != "hello" {
		t.Errorf("got %q, want \"hello\"", s)
	}
}

// I — Captured-def cleanup safety: after `def add5 (make-adder 5)`,
// the outer make-adder's `x` is gone from the module scope (cleaned
// up by DefCleanup); only add5's captured `x` survives. No `x` leak.
func TestClosureNoOuterDefLeak(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.Register(reg)
	e := native.NewTop(reg)
	src := `def make-adder ([x:Integer] => [([y:Integer] => [x add y])])
def add5 (make-adder 5)`
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := e.Run(vals); err != nil {
		t.Fatalf("run: %v", err)
	}
	if reg.Defs.Depth("x") != 0 {
		t.Errorf("outer x leaked to module scope: depth = %d", reg.Defs.Depth("x"))
	}
}

// J0 — End-to-end check-mode inference through factory + def + call.
// make-adder returns an inner closure capturing x:Integer; binding
// that to add5 and calling `add5 3` should produce an Integer carrier
// because the inner body `x add y` resolves x (captured) and y (param)
// to Integer carriers, and `add` propagates Integer.
func TestClosureCheckModeFactoryDefCall(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.Register(reg)
	reg.Check.Mode = true
	defer func() { reg.Check.Mode = false }()

	e := native.NewTop(reg)
	src := `def make-adder ([x:Integer] => [([y:Integer] => [x add y])])
def add5 (make-adder 5)
add5 3`
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := e.Run(vals)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(out), out)
	}
	v := out[0]
	if !v.Carrier {
		t.Errorf("expected carrier in check mode, got %v", v)
	}
	if !v.Parent.Equal(core.TInteger) {
		t.Errorf("carrier Parent = %s, want Integer (capture flowed through factory→def→call)", v.Parent.String())
	}
}

// J — AnalyseFnBody installs captures so a body that references an
// enclosing-fn-local sees its captured type during check-mode
// inference. Direct test of the Phase 5 wire-up: call AnalyseFnBody
// with a synthetic body, an Integer-typed captured `x`, and verify
// the residual carrier infers Integer from `x add 1`.
func TestClosureCheckModeAnalyseFnBodyUsesCaptures(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.Register(reg)
	reg.Check.Mode = true
	defer func() { reg.Check.Mode = false }()

	// Body: x add 1 — references captured x, no params.
	body := []core.Value{
		core.NewWord("x"),
		core.NewWord("add"),
		core.NewInteger(1),
	}
	captures := []core.CapturedBinding{
		{Name: "x", Value: core.NewCarrier(core.TInteger)},
	}

	result := check.AnalyseFnBody(reg, "test", nil, body, nil, captures, nil, false)
	if len(result) != 1 {
		t.Fatalf("got %d residual values, want 1: %v", len(result), result)
	}
	v := result[0]
	if !v.Parent.Equal(core.TInteger) {
		t.Errorf("residual Parent = %s, want Integer (capture flowed through)", v.Parent.String())
	}

	// Confirm captures are NOT leaked: x should not exist at module
	// scope after AnalyseFnBody returns (it restores via Defs.Restore).
	if _, ok := reg.Defs.Top("x"); ok {
		t.Errorf("captured x leaked to outer scope after AnalyseFnBody")
	}
}

// Same test minus captures: body's reference to x stays undefined, so
// AnalyseFnBody emits an undefined_word diagnostic for x. (With captures the
// companion test above sees no such diagnostic and infers Integer.) The
// residual type itself is no longer a reliable discriminator: since `add` was
// tightened to require a Number/String operand, an undefined `x` no longer
// matches `add`'s old `[Scalar Scalar]` catch-all (which used to make the
// residual String), and the analyser's best-fit recovery now assumes the
// numeric overload — so the *diagnostic*, not the residual, is what proves x
// went unresolved.
func TestClosureCheckModeAnalyseFnBodyWithoutCaptures(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.Register(reg)
	reg.Check.Mode = true
	defer func() { reg.Check.Mode = false }()

	body := []core.Value{
		core.NewWord("x"),
		core.NewWord("add"),
		core.NewInteger(1),
	}
	// Same body, no captures — x is undefined in body's scope.
	check.AnalyseFnBody(reg, "test", nil, body, nil, nil, nil, false)

	foundUndefinedX := false
	for _, d := range reg.Check.Diagnostics {
		if d.Code == "undefined_word" && strings.Contains(d.Detail, "x") {
			foundUndefinedX = true
			break
		}
	}
	if !foundUndefinedX {
		t.Errorf("expected an undefined_word diagnostic for x without captures; got %v", reg.Check.Diagnostics)
	}
}

// K — args stays dynamic across the capture boundary. Inside a
// captured fn, `args.0` refers to that fn's own call args, not the
// surrounding scope's.
func TestClosureArgsStaysDynamic(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def outr ([a:Any] => [([] => [args])])`,
		`def captured-lam (outr 42)`,
		`captured-lam`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(out), out)
	}
	// captured-lam was invoked with no args, so its `args` should
	// be an empty list — not outer's [42].
	if !out[0].Parent.Equal(core.TList) {
		t.Fatalf("expected list, got %s", out[0].Parent.String())
	}
	lst, _ := core.AsList(out[0])
	if lst.Len() != 0 {
		t.Errorf("args inside captured fn = %s, want empty (its own call args, not outer's)", out[0].String())
	}
}

// L — Frame param shadows a colliding caller binding (per-call cleanup
// over-pop fix). A callee whose Function parameter name collides with the
// caller's same-named parameter — the classic comparator threaded through a
// `/v`-parked arg — must SHADOW the caller's binding (push depth+1) and tear
// back down to exactly the caller's level on frame exit, not destroy it at
// install time. Pre-fix, installing the callee's `comp` ran the same-scope
// overlap-redefinition filter, which DROPPED the caller's `comp`; the undef
// cleanup tail then over-popped the survivor to depth 0, so the SECOND
// `comp/v h` errored `undefined word: comp`. See
// design/legacy/ACCESSOR-SPLIT-AND-CLEANUP-BUG.ignore and InstallFrameBinding.
func TestFrameParamShadowsCollidingFunctionParam(t *testing.T) {
	// buildFnBodyHandler path: a named-param fn (h) invoked via a helper,
	// whose Function param `comp` collides with caller t's `comp`, reused twice.
	out, err := runNativeSteps(t, nil, []string{
		`def g ([x:Integer] => [x add 1])`,
		`def h fn [[comp:Function v:Integer] [Integer] [v comp/v apply]]`,
		`def t fn [[comp:Function] [Integer] [ def a (5 comp/v h)  def b (7 comp/v h)  a add b ]]`,
		`g/v t`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 14 {
		t.Errorf("g/v t → %d, want 14 (g(5)+g(7)=6+8); pre-fix errored `undefined word: comp` on the 2nd use", got)
	}

	// Direct comp/v apply (execFnDefSig path) reusing the colliding param twice
	// in one frame.
	out, err = runNativeSteps(t, nil, []string{
		`def g ([x:Integer] => [x add 1])`,
		`def t fn [[comp:Function] [Integer] [ (5 comp/v apply) add (7 comp/v apply) ]]`,
		`g/v t`,
	})
	if err != nil {
		t.Fatalf("run (apply path): %v", err)
	}
	if got := intResult(t, out); got != 14 {
		t.Errorf("apply path g/v t → %d, want 14", got)
	}
}

// M — The def-word overlap-redefinition filter is unaffected by the frame-
// binding shadow change: redefining a fn at the SAME scope still REPLACES the
// overlapping overload (it does not stack/union with the old one). Guards that
// InstallFrameBinding's shadow flag only gates frame installs, not `def`.
func TestDefWordOverlapStillRedefines(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def f fn [[x:Integer] [Integer] [x]]`,
		`def f fn [[x:Integer] [Integer] [x add 100]]`,
		`5 f`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 105 {
		t.Errorf("redefined f, 5 f → %d, want 105 (overlapping redefinition must replace, not race the old overload)", got)
	}
}
