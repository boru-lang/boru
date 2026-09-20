package lang

import (
	"fmt"
	"testing"
)

// Cross-module element-typing leaf pins (voxgig stage-2 slice): the
// `dynamic input at each` flip.
//
// ROOT CAUSE: applyGradualContagion (eng/go/carrier.go) marked EVERY output
// of a call with any dynamic input as Dynamic("type unknown, matched
// optimistically"). But a word with a CONCRETE, input-INDEPENDENT declared
// return type — `StructUtil.items` always returns a `List`, regardless of
// which runtime value flowed in — has a statically-KNOWN result type. The
// dynamic input only decides whether the call SUCCEEDS or RAISES, and the
// call itself bakes as a runtime-faithful guarded dispatch (CALL_NATIVE /
// poly) that raises exactly as the interpreter on a bad receiver, so a
// compiled program never observes a wrongly-typed result. Marking that
// output dynamic was therefore over-conservative: `(nd "kids" get)
// StructUtil.items` — items over the dynamic Any result of a Map field read
// — typed as a Dynamic List, and the downstream `each` declined "dynamic
// input at each" (a dynamic carrier could be List OR Map — ambiguous).
//
// FIX: keep a SINGLE-reachable-return, concrete (non-Any) declared return
// STRICT under gradual contagion, so `each` commits on the known `List`
// container and derives its element carrier. Multi-overload divergent
// returns still ride the dynamic union (the runtime could take a sibling
// overload's return); a genuinely input-dependent return (`get` → Any)
// stays dynamic and its downstream `each` still declines.

func crossmodSound(t *testing.T, src string) {
	t.Helper()
	a, _ := New()
	want, werr := a.RunInterp(src)
	b, _ := New()
	got, _, gerr := b.RunCompiled(src)
	if noteCompileDefect(t, src, got, gerr) {
		return
	}
	if (werr == nil) != (gerr == nil) {
		t.Fatalf("error disagreement (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, werr, gerr)
	}
	if werr == nil && fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("SILENT MISCOMPILE (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, want, got)
	}
}

func crossmodCompiles(t *testing.T, src string) {
	t.Helper()
	crossmodSound(t, src)
	a, _ := New()
	if _, reason, _, err := a.CompileCheck(src); err != nil || reason != "" {
		t.Fatalf("expected the shape to force-compile, got compile failure: %v / %q\n  src: %s", err, reason, src)
	}
}

// The exact voxgig radix `edge-items` shape: `StructUtil.items` over the
// DYNAMIC result of a Map field read, feeding `each` with a reach-lens body.
// Previously declined "dynamic input at each"; now compiles + parity.
func TestItemsOverDynamicReceiverEachCompiles(t *testing.T) {
	crossmodCompiles(t, `import "boru:struct-util"
def edge-cols fn [[nd:Map] [List] [ ((nd "kids" get) StructUtil.items) each $.1 ]]
(edge-cols {kids: {a:{x:1} b:{x:2}}})`)
}

// The each RESULT (a strict List, not a dynamic one) must flow on to a
// downstream typed consumer (`size` → Integer). If the fix left the each
// result dynamic, `size` would still commit but the each itself would have
// declined; this pins the whole chain compiling end-to-end.
func TestItemsEachResultFeedsTypedConsumer(t *testing.T) {
	crossmodCompiles(t, `import "boru:struct-util"
def n-cols fn [[nd:Map] [Integer] [ (((nd "kids" get) StructUtil.items) each $.1) size ]]
(n-cols {kids: {a:{x:1} b:{x:2} c:{x:3}}})`)
}

// The trie `drop-kid` shape: `StructUtil.items` over a dynamic receiver
// feeding a `fold` (the same declared-List return, consumed by the
// fold-collection overload). A bonus advance in the same family.
func TestItemsOverDynamicReceiverFoldCompiles(t *testing.T) {
	crossmodCompiles(t, `import "boru:struct-util"
def drop-first fn [[nd:Map] [Map] [
  ({} ((nd "kids" get) StructUtil.items) [
    var [[pair acc] acc set ((pair get 0)) (pair get 1) ]
  ] fold)
]]
(drop-first {kids: {a:1 b:2}})`)
}

// All runtime paths of the items-each shape stay sound (empty kids → empty
// List; populated → the plucked column). Empty and populated pin the
// element-carrier derivation under both branches.
func TestItemsEachAllPathsSound(t *testing.T) {
	for _, call := range []string{
		`(edge-cols {kids: {}})`,
		`(edge-cols {kids: {a:{x:1}}})`,
		`(edge-cols {kids: {a:{x:1} b:{x:2}}})`,
	} {
		crossmodSound(t, `import "boru:struct-util"
def edge-cols fn [[nd:Map] [List] [ ((nd "kids" get) StructUtil.items) each $.1 ]]
`+call)
	}
}

// An `each` over a GENUINELY input-dependent Any (a `get` result, whose
// declared return IS Any) stays DYNAMIC — the fix keeps only concrete non-Any
// declared returns strict. Until S1a that meant each declined and the program
// was interpreted; since S1a (2026-09-19, design/FULL-COMPILATION-REPLAN.0.md)
// each declares CompileDynBody, so the dispatch lowers to a poly re-match over
// its own overloads and the live value picks the List form: compiled, with
// parity. The dynamic carrier is still not narrowed — that is the point.
func TestEachOverDynamicAnyCompiles(t *testing.T) {
	crossmodCompiles(t, `def f fn [[m:Map] [List] [ (m "xs" get) each $.0 ]]
(f {xs: [[1 2] [3 4]]})`)
}

// NEGATIVE: a multi-overload word whose reachable overloads return DIVERGENT
// types over a dynamic receiver must NOT be forced strict to the single
// matched sig's return — it must stay a dynamic union. `slice` over a
// dynamic (String|List) receiver, whose sliced result feeds a String-only
// consumer, must remain sound whichever runtime type the receiver holds.
func TestDivergentReturnStaysDynamicUnion(t *testing.T) {
	for _, call := range []string{
		`(chop "hello")`,
		`(chop [1 2 3 4])`,
	} {
		crossmodSound(t, `def chop fn [[v:Any] [Any] [ (v slice 1) ]]
`+call)
	}
}
