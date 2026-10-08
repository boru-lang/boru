package lang

import (
	"fmt"
	"strings"
	"testing"
)

// requireNoWrongAnswer pins a shape the compiled lane may decline or defer
// but must never answer differently: a compiled run agrees with the
// interpreter, or the program declines to compile, or the run defers loudly
// (a bail defect the host hands to the interpreter).
func requireNoWrongAnswer(t *testing.T, src string) {
	t.Helper()
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if !compiled || isBailDefect(errC) {
		return
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("%q: compiled %v / %v, interpreter %v / %v", src, gotC, errC, gotI, errI)
	}
}

// TestSeededGeneratorThroughAMember pins the NUR331 compile refusal's fix. A
// seeded generator read through a map member answered the check pass's
// SHAPE MODEL of the instance (a generator seeded 0, Rand.with-seed's
// ReturnsFn) — [5 52] for [61 84] — and after main e8702ac refused to compile
// ("undefined word: rand-int"). Three mechanisms, one rule — a check-time
// model is not the run's value:
//   - the model's methods are no constant (Registry.ShapeModel), so no const
//     gate bakes them — a map or list holding the generator assembles per run
//     over the generator's event;
//   - a container fold reading a binding an event produced declines
//     (bindingProduced), so the literal records over that event;
//   - a folded constant's fn values queue for the end-of-pass body check in
//     their home registry (FnHome), where their delegates exist.
func TestSeededGeneratorThroughAMember(t *testing.T) {
	const s3 = `import "boru:rand" def s (Rand.with-seed 3) end `
	const m = s3 + `def m {r: s} `
	for _, c := range []struct{ src, want string }{
		{m + `m.r.int 0 100`, "[61]"},
		{m + `[(m.r.int 0 100) (m.r.int 0 100)]`, "[[61 84]]"},
		{m + `m.r.int 0 100 m.r.int 0 100`, "[61 84]"},
		{m + `[(s.int 0 100) (m.r.int 0 100)]`, "[[61 84]]"},
		{`import "boru:rand" def s (Rand.with-seed 7) end def m {r: s} [(m.r.int 0 100) (m.r.int 0 100) (m.r.int 0 100)]`, "[[55 24 42]]"},
		{`import "boru:rand" def s (Rand.with-seed 42) end def m {r: s} [(m.r.int 0 1000) (m.r.int 0 1000)]`, "[[675 411]]"},
		{m + `[(m.r.float) (m.r.float)]`, "[[0.7199826688373036 0.6526308027999123]]"},
		{m + `[(m.r.bool) (m.r.bool) (m.r.bool)]`, "[[false true false]]"},
		{m + `[(m.r.one-of [1 2 3]) (m.r.one-of [1 2 3])]`, "[[2 3]]"},
		{m + `[(m.r.string "abc" 5)]`, "[['bcaac']]"},
		{m + `[(m.r.list-of [s.int 0 10] 3)]`, "[[[1 4 9]]]"},
		{m + `m.r size`, "[7]"},
		{m + `def t m.r end [(t.int 0 100) (t.int 0 100)]`, "[[61 84]]"},
		{m + `def f fn [[][Any][m.r.int 0 100]] end [(f) (f)]`, "[[61 84]]"},
		{m + `def n {q: m} [(n.q.r.int 0 100) (n.q.r.int 0 100)]`, "[[61 84]]"},
		{m + `[((m get "r").int 0 100) ((m get "r").int 0 100)]`, "[[61 84]]"},
		// The instance in a paren member, a nested map, a list member, and a
		// map built in a fn body.
		{`import "boru:rand" def m {r: (Rand.with-seed 3)} [(m.r.int 0 100) (m.r.int 0 100)]`, "[[61 84]]"},
		{s3 + `def m {a: {r: s}} [(m.a.r.int 0 100) (m.a.r.int 0 100)]`, "[[61 84]]"},
		{s3 + `def m {r: {q: s}} [(m.r.q.int 0 100) (m.r.q.int 0 100)]`, "[[61 84]]"},
		{s3 + `def m {r: [s]} [((m.r get 0).int 0 100) ((m.r get 0).int 0 100)]`, "[[61 84]]"},
		{s3 + `def m [{r: s}] [((m get 0).r.int 0 100) ((m get 0).r.int 0 100)]`, "[[61 84]]"},
		{s3 + `def f fn [[][Any][{r: s}]] end def m (f) [(m.r.int 0 100) (m.r.int 0 100)]`, "[[61 84]]"},
		// A logger read through a member logs under its own name.
		{`import "boru:log" ; def l (Log.with "http" {svc:"api"}) ; def m {l: l} ; Log.add-sink memory/q ; Log.remove-sink console/q ; m.l.info "req" ; Log.dump 0 get "logger" get`, "[http]"},
		// A deterministic member keeps folding into the map as before.
		{`def s (range 0 3) end def m {r: s} m.r size`, "[3]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// The method applied inside a `do` over a runtime-variable body stays a
	// sound decline (TestVariadicShapedMethodDeclines), and a logger in a
	// list member never answers another logger's record.
	for _, src := range []string{
		m + `do [m.r.int 0 100]`,
		`import "boru:log" ; def l (Log.with "http" {svc:"api"}) ; def m {l: [l]} ; Log.add-sink memory/q ; Log.remove-sink console/q ; (m.l get 0).info "req" ; Log.dump 0 get "logger" get`,
	} {
		requireNoWrongAnswer(t, src)
	}
	if prog, reason, _, err := mustNew(t).CompileCheck(m + `do [m.r.int 0 100]`); prog != nil || err != nil {
		t.Errorf("do [m.r.int 0 100]: want a sound decline, got prog=%v reason=%q err=%v", prog != nil, reason, err)
	}
	// The negative twin of the bake: the map is ASSEMBLED over the
	// generator's event, never a pooled const holding the model's methods.
	dis := compileDisasm(t, m+`m.r.int 0 100`)
	if !strings.Contains(dis, "MAKE_MAP") || strings.Contains(dis, "[rand-int]") {
		t.Errorf("the member map must assemble per run, not bake the model:\n%s", dis)
	}
}

// TestUserDropExtensionDoBody pins the do-body compile refusal's fix: a
// user extension of `drop` that binds its argument and does nothing else
// (`def drop fn [[x:P] [] []]` over `def P (refine Integer)`) leaves a
// `do [1 drop]` body as raise-free as the core drop does — the registered
// all-Any native stays the total fallback, and whichever overload takes the
// value it is consumed — so the body nets nothing and `depth` folds as it
// does over the core word (shuffleDispatchSafe). An extension that can raise
// or leave values keeps the latched count.
func TestUserDropExtensionDoBody(t *testing.T) {
	const p = `def P (refine Integer) `
	const drop = p + `def drop fn [[x:P] [] []] end `
	for _, c := range []struct{ src, want string }{
		{drop + `do [1 drop] depth`, "[0]"},
		{drop + `do [1 drop] 5`, "[5]"},
		{drop + `1 do [1 drop] depth`, "[1 1]"},
		{drop + `do [1 drop]`, "[]"},
		{drop + `do ["a" drop] depth`, "[0]"},
		{drop + `do [1 drop] end depth`, "[0]"},
		{drop + `[(do [1 drop] depth)]`, "[[0]]"},
		{drop + `def f fn [[][Any][do [1 drop] depth]] end f`, "[0]"},
		{p + `def drop2 fn [[x:P y:P] [] []] end do [1 2 drop2] depth`, "[0]"},
		{`do [1 drop] depth`, "[0]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// Not proven raise-free: the extension has a body, a declared return, an
	// unnamed param, another arity or another effect, or collects forward
	// past its own position. Each keeps the latch — never a wrong answer.
	for _, src := range []string{
		p + `def drop fn [[x:P] [] [x]] end do [1 drop] depth`,
		p + `def drop fn [[x:P] [] [raise bad "boom"]] end do [1 drop] depth`,
		p + `def drop fn [[x:P] [Integer] []] end do [1 drop] depth`,
		p + `def drop fn [[:P] [] []] end do [1 drop] depth`,
		p + `def drop fn [[x:P y:P] [] []] end do [1 2 drop] depth`,
		p + `def dup fn [[x:P] [] []] end do [1 dup] depth`,
		drop + `do [1 2 drop drop] depth`,
		drop + `do [1 drop 2] depth`,
		drop + `do [drop] depth`,
	} {
		requireNoWrongAnswer(t, src)
	}
}

// TestRootGradualReadBeneathACallResult pins the residual-shape compile
// refusal's fix: `def j (mk) end j 5 j typeof` over an Any-returning factory
// lays its residual [j 5 Integer] out through the rebuild (a call result
// above the literal), which the CALLABLE screen refused. A root read of the
// gradual value is its own test's (an island or guard, NUR207) and a call
// result's arrival was modelled at its dispatch, so with no pick/roll fold
// in the program neither is screened (residualCallableExempt). A fn at run
// time is still the read's test: the interpreter's answer or a loud defer.
func TestRootGradualReadBeneathACallResult(t *testing.T) {
	const mk7 = `def mk fn [[][Any][7]] end def j (mk) end `
	for _, c := range []struct{ src, want string }{
		{mk7 + `j 5 j typeof`, "[7 5 Integer]"},
		{mk7 + `j 5 [j]`, "[7 5 [7]]"},
		{mk7 + `j (range 0 3)`, "[7 [0 1 2]]"},
		{mk7 + `j 5 (1 add 2)`, "[7 5 3]"},
		{mk7 + `1 j (range 0 3)`, "[1 7 [0 1 2]]"},
		{mk7 + `j j (range 0 3)`, "[7 7 [0 1 2]]"},
		{mk7 + `(j) 5 j typeof`, "[7 5 Integer]"},
		{mk7 + `j 5 j typeof end`, "[7 5 Integer]"},
		{mk7 + `def k (mk) end j 5 k (range 0 2)`, "[7 5 7 [0 1]]"},
		{`def mk fn [[][Any]['s']] end def j (mk) end j 5 j typeof`, "[s 5 ProperString]"},
		{`def mk fn [[][Any][[1 2]]] end def j (mk) end j (range 0 3)`, "[[1 2] [0 1 2]]"},
		// A data member's read result above a literal.
		{`def inc fn [[n:Integer][Integer][n add 1]] end def m {f: inc/v} 7 (m get f/q) ; 3`, "[7 fn inc(Integer) 3]"},
		{`def inc fn [[n:Integer][Integer][n add 1]] end def mkinc fn [[][Any][inc/v]] end 7 mkinc (range 0 2)`, "[7 fn inc(Integer) [0 1]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A fn at run time: the read's test defers loudly where it cannot resume.
	requireLoudDefer(t, `def lam ([] => [9]) end def mk fn [[][Any][lam/v]] end def j (mk) end j (range 0 3)`,
		"could not re-step (NUR123)", "[9 [0 1 2]]")
	// A pick/roll fold's copy keeps the whole screen: still declined.
	for _, src := range []string{
		mk7 + `j 5 1 pick (range 0 2)`,
		mk7 + `j 5 1 roll (range 0 2)`,
		`def inc fn [[n:Integer][Integer][n add 1]] end def mkinc fn [[][Any][inc/v]] end 7 (mkinc) 0 pick (range 0 2)`,
	} {
		if prog, _, _, err := mustNew(t).CompileCheck(src); prog != nil || err != nil {
			t.Errorf("%s: want a decline, got prog=%v err=%v", src, prog != nil, err)
		}
		requireNoWrongAnswer(t, src)
	}
	for _, src := range []string{
		`def lam ([x:Integer] => [x add 1]) end def mk fn [[][Any][lam/v]] end def j (mk) end j 5 j typeof`,
		`def lam ([x:Integer] => [x add 1]) end def mk fn [[][Any][lam/v]] end def j (mk) end j 5 [j]`,
		`def lam ([x:Integer] => [x add 1]) end def mk fn [[][Any][lam/v]] end def j (mk) end 1 j (range 0 3)`,
		`def lam ([s:String] => [s]) end def mk fn [[][Any][lam/v]] end def j (mk) end j (range 0 3)`,
	} {
		requireNoWrongAnswer(t, src)
	}
}
