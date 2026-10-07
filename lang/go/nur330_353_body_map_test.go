package lang

import (
	"fmt"
	"strings"
	"testing"
)

// nur330_353_body_map_test.go pins NUR330's residual and NUR353.
//
// NUR330's recorded repro — `f {a:[def k 1 k]} k` over `def f fn [[m:Map]
// [Any][Rand.map-from m]]` — was not a map-from defect at all: a map literal
// evaluated as a word's argument runs its list and paren members on the
// shared registry, and the interpreter keeps a def they make, but the check
// pass's const fold (concreteEvalOnce) ran the member in a sandbox and
// rolled the binding back, so the compiled program pushed the folded map and
// every later read answered the stale binding. The fold now declines a run
// that changed a binding, and the member records as it runs.
func TestNUR330MapMemberDefsAgree(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def k 5 size {a:[def k 1 k]} k`, "[1 1]"},
		{`def k 5 size {a:(def k 1 k)} k`, "[1 1]"},
		{`def k 5 print {a:[def k 1 k]} k`, "[1]"},
		{`def k 5 def f fn [[m:Map][Any][m]] end f {a:[def k 1 k]} k`, "[{a:[1]} 1]"},
		{`def k 5 def f fn [[m:Any][Any][m]] end f {a:[def k 1 k]} k`, "[{a:[1]} 1]"},
		{`def k 5 def f fn [[m:Map][Any][m]] end f {a:[def k 1]} k`, "[{a:[]} 1]"},
		{`def k 5 def f fn [[m:Map][Any][m]] end f {a:{b:[def k 1 k]}} k`, "[{a:{b:[1]}} 1]"},
		{`def k 5 def f fn [[m:Map][Any][m.a]] end f {a:[def k 1 k]} k`, "[[1] 1]"},
		{`def k 5 def m {a:[def k 1 k]} end [m k]`, "[[{a:[1]} 1]]"},
		{`def k 5 size {a:[def j 1 j]} j`, "[1 1]"},
		{`def k 5 def k 6 size {a:[undef k 1]} k`, "[1 5]"},
		{`def k 5 size {a:[def k (k add 1) k] b:[k]} k`, "[2 6]"},
		// Negative: a member that binds nothing still folds, a fn call's own
		// frame is no binding change, and a residual map evaluated after the
		// read leaves it alone on both lanes.
		{`def k 5 size {a:[k]} k`, "[1 5]"},
		{`def g fn [[n:Integer] [Integer] [n add 1]] end def k 5 size {a:(g 3)} k`, "[1 5]"},
		{`def g fn [[n:Integer] [Integer] [n add 1]] end def n 5 {a:(g 3) b:n}`, "[{a:4 b:5}]"},
		{`def k 5 [{a:[def k 1 k]} k]`, "[[{a:[1]} 5]]"},
		{`def k 5 def f fn [[] [Any] [size {a:[def k 1 k]}]] end f k`, "[1 5]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// requireBodyMapDeclines asserts the program agrees with want on the interpreter
// and declines to compile, with the reason naming why.
func requireBodyMapDeclines(t *testing.T, src, want, why string) {
	t.Helper()
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if got := fmt.Sprint(gotI); errI != nil && want != "ERROR" || errI == nil && got != want {
		t.Errorf("%s: interpreter %v / %v, want %s", src, gotI, errI, want)
	}
	if compiled || errC == nil || !strings.Contains(errC.Error(), "compil") || !strings.Contains(errC.Error(), why) {
		t.Errorf("%s: must decline to compile (%q), got %v / %v (compiled=%v)", src, why, gotC, errC, compiled)
	}
}

// NUR330's residual proper: a Rand.map-from schema the pass holds only as a
// carrier runs bodies the pass never saw, and the interpreter keeps a def
// they make — past the calling fn's frame when the fn has no binding of its
// own. The run now arms the kept-defs latch (compiler noteBodyMapRun), so a
// read after it seats live where the unit rebinds the name first and the
// compile declines otherwise; a program that reads nothing after it compiles.
func TestNUR330CarrierSchemaBodyDefs(t *testing.T) {
	const rnd = `import "boru:rand"  `
	for _, c := range []struct{ src, want string }{
		{rnd + `def k 5 def f fn [[m:Map][Any][Rand.map-from m]] end f {a:(quote [def k 1 k])}`, "[{a:1}]"},
		{rnd + `def f fn [[m:Map][Any][Rand.map-from m]] end f {a:[1]}`, "[{a:1}]"},
		{rnd + `def k 5 def f fn [[m:Map][Any][def k 7 Rand.map-from m]] end f {a:(quote [def k 1 k])} k`, "[{a:1} 5]"},
		{rnd + `def k 5 def f fn [[m:Map][Any][def k 7 Rand.map-from m]] end f {a:(quote [k])}`, "[{a:7}]"},
		{rnd + `def k 5 def f fn [[b:List][Any][def s {a: b} Rand.map-from s]] end f [2]`, "[{a:2}]"},
		{rnd + `def k 5 def f fn [[m:Map][Any][def k 7 Rand.map-from m k]] end f {a:(quote [def k 1 k])}`, "ERROR"},
	} {
		if c.want == "ERROR" {
			gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
			if !compiled || errI == nil || fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
				t.Errorf("%s: compiled %v/%v (%v), interp %v/%v", c.src, gotC, errC, compiled, gotI, errI)
			}
			continue
		}
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, c := range []struct{ src, want string }{
		{rnd + `def k 5 def f fn [[m:Map][Any][Rand.map-from m]] end f {a:[def k 1 k]} k`, "[{a:1} 1]"},
		// A quoted body runs inside map-from as a BLOCK (phase 2): its def is
		// the body's own, and k after the call is the module's 5.
		{rnd + `def k 5 def f fn [[m:Map][Any][Rand.map-from m]] end f {a:(quote [def k 1 k])} k`, "[{a:1} 5]"},
		{rnd + `def k 5 def f fn [[m:Map][Any][Rand.map-from m]] end [(f {a:(quote [def k 1 k])}) k]`, "[[{a:1} 5]]"},
		{rnd + `def k 5 def f fn [[m:Map][Any][Rand.map-from m k]] end f {a:(quote [def k 1 k])}`, "ERROR"},
		{rnd + `def k 5 def f fn [[b:List][Any][def s {a: b} Rand.map-from s]] end f (quote [def k 1 k]) k`, "[{a:1} 5]"},
		// Negative: a body that binds nothing reads the same k on both
		// lanes, but the unit cannot know its argument's bodies — it
		// declines rather than guess.
		{rnd + `def k 5 def f fn [[m:Map][Any][Rand.map-from m]] end f {a:[1]} k`, "[{a:1} 5]"},
	} {
		requireBodyMapDeclines(t, c.src, c.want, "")
	}
}

// NUR353: a map-from body is a run-time token body resolving names on the
// registry, where the interpreter's `for` binds its index; and a
// break/continue it raises outside any loop is the interpreter's flow_error
// where its tape stands. The escape at the root declines at the lowering
// (lowerer.bodyMapReason). The index read declined there too while the
// compiled loop kept its index in a frame slot alone; under the
// dynamic-environment mirror the body's facts arm, the loop publishes its
// index on the registry (NUR354, OpForPublish), and those shapes compile and
// agree.
func TestNUR353BodyMapLoopIndexAndEscape(t *testing.T) {
	const rnd = `import "boru:rand"  `
	for _, c := range []struct{ src, want string }{
		{rnd + `for 2 [Rand.map-from {a:[i]}]`, "[{a:0} {a:1}]"},
		{rnd + `for 2 [Rand.map-from {a:[i add 10] b:[i]}]`, "[{a:10 b:0} {a:11 b:1}]"},
		{rnd + `for 3 [Rand.map-from {a:[i break]}]`, "[]"},
		{rnd + `for 3 [Rand.map-from {a:[i continue]}]`, "[]"},
		{rnd + `def f fn [[m:Map][Any][for 2 [Rand.map-from m]]] end f {a:(quote [i])}`, "ERROR:got 2 — [{a:0} {a:1}]"},
		{rnd + `for 3 [Rand.map-from {a:[1 break]}] 7`, "[7]"},
		{rnd + `for 2 [Rand.map-from {a:[1 add 10]}]`, "[{a:11} {a:11}]"},
		{rnd + `var n 0 for 3 [var n (n add 1) Rand.map-from {a:[n]}] n`, "[{a:1} {a:2} {a:3} 3]"},
		{rnd + `var n 0 for 3 [var n (n add 1) Rand.map-from {a:[continue] b:[9]}] n`, "[3]"},
		{rnd + `def f fn [[] [Any] [break]] end for 3 [Rand.map-from {a:[f]}] 7`, "[7]"},
		{rnd + `def k 5 Rand.map-from {b:[k add 1]} k`, "[{b:6} 5]"},
		{rnd + `def i 9 Rand.map-from {a:[i]}`, "[{a:9}]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, c := range []struct{ src, want, why string }{
		{rnd + `Rand.map-from {a:[1 break]}`, "ERROR", "outside a compiled loop"},
		{rnd + `Rand.map-from {a:[1 continue]} 5`, "ERROR", "outside a compiled loop"},
		// A list literal is not a loop (NUR358): the escape raises.
		{rnd + `[Rand.map-from {a:[1 break]}]`, "ERROR", "outside a compiled loop"},
		{rnd + `if true [Rand.map-from {a:[1 break]}] [0]`, "ERROR", "outside a compiled loop"},
	} {
		requireBodyMapDeclines(t, c.src, c.want, c.why)
	}
}

// The review of #523 (Codex): four holes in the body-map scan, each pinned
// with the shape it let through or refused, and its negative.
//
//   - P1: a word reads its callee's body too, transitively — a user fn a
//     body calls reads the loop index on the interpreter's registry (and,
//     the index published since NUR354, reads it compiled: the run-time
//     stamp's snapshot reads through the callee too, so the stamp that baked
//     one iteration's index goes stale at the next — storedHandlerDepsDeep);
//   - P1: a body is whatever the handler takes as one (a flex list);
//   - P2: a carrier entry no longer stops the scan, so a literal body's
//     escape declines at the root whatever the key order;
//   - P2: quoted data (a quoted list, `quote`'s operand) reads no name
//     unless a word that runs a value takes it.
func TestNUR353BodyMapReviewFixes(t *testing.T) {
	const rnd = `import "boru:rand"  `
	for _, c := range []struct{ src, want string }{
		{rnd + `def f fn [[] [Integer] [7]] end for 2 [Rand.map-from {a:[f]}]`, "[{a:7} {a:7}]"},
		{rnd + `def f fn [[] [Integer] [f]] end def g fn [[] [Integer] [7]] end for 2 [Rand.map-from {a:[g]}]`, "[{a:7} {a:7}]"},
		{rnd + `def f fn [[n:Integer] [Integer] [n]] end for 2 [Rand.map-from {a:[f 3]}]`, "[{a:3} {a:3}]"},
		{rnd + `def s {a:(flex (quote [7]))} end for 2 [Rand.map-from s]`, "[{a:7} {a:7}]"},
		{rnd + `def s {a:(flex (quote [1 break]))} end for 3 [Rand.map-from s] 7`, "[7]"},
		{rnd + `def mk fn [[][List][quote [1]]] end def s {a:(mk) c:(quote [1])} end Rand.map-from s`, "[{a:1 c:1}]"},
		{rnd + `for 2 [Rand.map-from {a:[quote [i]]}]`, "[{a:[word(i)]} {a:[word(i)]}]"},
		{rnd + `for 2 [Rand.map-from {a:[quote i]}]`, "[{a:i} {a:i}]"},
		{rnd + `for 2 [Rand.map-from {a:[(quote [i])]}]`, "[{a:[word(i)]} {a:[word(i)]}]"},
		{rnd + `for 2 [Rand.map-from {a:[quote [i] size]}]`, "[{a:1} {a:1}]"},
		{rnd + `for 2 [Rand.map-from {a:[i/q]}]`, "[{a:i} {a:i}]"},
		{rnd + `def i 9 end def f fn [[] [Integer] [i]] end for 2 [Rand.map-from {a:[f]}]`, "[{a:0} {a:1}]"},
		{rnd + `def i 9 end def g fn [[] [Integer] [i]] end def f fn [[] [Integer] [g]] end for 2 [Rand.map-from {a:[f]}]`, "[{a:0} {a:1}]"},
		{rnd + `def s {a:(flex (quote [i]))} end for 2 [Rand.map-from s]`, "[{a:0} {a:1}]"},
		{rnd + `for 2 [Rand.map-from {a:[do (quote [i])]}]`, "[{a:0} {a:1}]"},
		{rnd + `for 2 [Rand.map-from {a:[quote [i] do]}]`, "[{a:0} {a:1}]"},
		{rnd + `def g fn [[x:Any] [Any] [do x]] end for 2 [Rand.map-from {a:[g (quote [i])]}]`, "[{a:0} {a:1}]"},
		{rnd + `def s {a:(quote [i])} end for 2 [Rand.map-from s]`, "[{a:0} {a:1}]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, c := range []struct{ src, want, why string }{
		{rnd + `def s {a:(flex (quote [1 break]))} end Rand.map-from s`, "ERROR", "outside a compiled loop"},
		{rnd + `def mk fn [[][List][quote [1]]] end def s {a:(mk) c:(quote [1 break])} end Rand.map-from s`, "ERROR", "outside a compiled loop"},
		{rnd + `def mk fn [[][List][quote [1]]] end def s {c:(quote [1 break]) a:(mk)} end Rand.map-from s`, "ERROR", "outside a compiled loop"},
	} {
		requireBodyMapDeclines(t, c.src, c.want, c.why)
	}
}
