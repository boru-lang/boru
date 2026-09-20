package lang

import "testing"

// Stage-2 module-scope IMMUTABLE-instance capture pins (voxgig zero-compile failures
// plan, "fn call operand of unknown provenance" leaf under the trie/burst/tst
// _unit_test test-test bodies).
//
// A module-scope `def fixture (TrieMap.make …)` binds a persistent instance
// whose check-mode value is a NON-CONCRETE carrier (a Map, or an
// under-annotated Any). Read inside a code-body closure (a Test.test body, an
// each/fold body), the reference is module-scope — below the fn baseline, so
// ComputeCaptures excludes it — and it is NOT const-bakeable (materialise
// declines a non-concrete carrier). The body then declined "fn call operand of
// unknown provenance" (the instance reached a user call as an unresolved
// operand) or "code-body word each/fold (Stage 2)".
//
// The fix widens moduleScopeMutableCaptures via moduleScopeInstanceCarrier: an
// immutable non-concrete instance carrier rides as a closure capture exactly
// like the mutable accumulator. It IS produced by a module-scope event, so it
// resolves at the call site (the closure's construction, at module scope) while
// its event is unreachable inside the body — the capture slot bridges that. A
// compiled body cannot rebind a module-scope name, so the value threaded at
// OpPushClosure equals every per-run lookup the interpreter makes.
//
// The langspec differential is blind to these off-corpus shapes, so pin them
// (the memory-noted RunCompiledStrict discipline). The mutable-accumulator
// sibling is pinned in TestFlexCaptureInEachBodyCompiles; these cover the
// IMMUTABLE (Map/List/Any) instance carrier the earlier pass did not reach.

// A module-scope Map instance (a user fn's declared-[Map] return is a
// non-concrete carrier) read inside an each body. Compiles + parity.
func TestModuleScopeMapInstanceEachCompiles(t *testing.T) {
	stage1aCompiles(t, `def build fn [[] [Map] [{a: 10}]]
def fix (build)
def ys ([1 2 3] each [var [[x] x add (fix get "a")]])
ys`)
}

// The same instance carrier threaded through a fold body's accumulator merge.
func TestModuleScopeMapInstanceFoldCompiles(t *testing.T) {
	stage1aCompiles(t, `def build fn [[] [Map] [{a: 5}]]
def fix (build)
def s (0 [1 2 3] [var [[x acc] acc add (x add (fix get "a"))]] fold)
s`)
}

// A module-scope LIST instance carrier read (indexed) inside an each body —
// the List-family arm of the widened predicate.
func TestModuleScopeListInstanceEachCompiles(t *testing.T) {
	stage1aCompiles(t, `def build fn [[] [List] [[100 200]]]
def fix (build)
def ys ([0 1] each [var [[i] (fix get i)]])
ys`)
}

// The instance returned AS the body residual (identity), then consumed after
// the loop: the capture must carry the exact same instance on both surfaces —
// parity, byte-identical.
func TestModuleScopeInstanceIdentityBodySound(t *testing.T) {
	stage1aSound(t, `def build fn [[] [Map] [{a: 3}]]
def fix (build)
def firsts ([1 2] each [var [[x] fix]])
(firsts size)`)
}

// Control: a plain each body with NO module-scope instance reference must stay
// sound and unchanged (the widened predicate never fires here — const-bakeable
// and frame-local operands keep their existing paths).
func TestPlainEachBodyNoInstanceSound(t *testing.T) {
	stage1aSound(t, `def ys ([1 2 3] each [var [[x] x mul 2]])
ys`)
}
