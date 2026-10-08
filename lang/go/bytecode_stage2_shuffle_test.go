package lang

import "testing"

// Stage-2 shuffle-elision pins (voxgig zero-compile failures plan): the
// mutual-recursion-through-a-token-each shape (decision.boru eval-pred-all/any
// — `(children each [input swap eval-pred]) all`) declined as "dynamic input at
// swap": the each-body element of an untyped List is a DYNAMIC carrier, and
// the pure stack shuffles (single all-Any signature, identity returns) fell
// through every dynamic recovery (poly is single-result-only) into the blanket
// anyDynamicCarrier compile failure. Two mechanisms fix it (emit.go):
//
//  1. recordShuffleElided — a pure ID-preserving permutation (swap/rot/swap2)
//     over dynamic operands records NOTHING: the outputs ARE the inputs, so
//     consumers resolve the same value IDs to their original producers and the
//     lowerer re-derives stack motion from dataflow. (Baking a CALL_NATIVE
//     instead strands the pass-through copy of a frame-local operand on the
//     simulated stack — "body leaves extra values".)
//  2. dynamicStackShuffleOK — the remaining shuffle words (dup/over/drop/…)
//     may bake a plain CALL_NATIVE despite dynamic operands: their ONE
//     signature is all-Any (no sibling overload to mis-commit) and the outputs
//     keep their dynamic modality, so downstream gating is unchanged.
//
// The differential corpus never sees this off-corpus shape family, so pin it
// (the memory-noted RunCompiledStrict discipline).

// The decision.boru eval-pred cascade in miniature: ep-all/ep-any/ep-not are
// MUTUALLY RECURSIVE with ep THROUGH a token-body each closure whose `input`
// is the enclosing fn's param (a lexical capture). All node kinds — and-group,
// or-group, not-group, bare leaf, empty children — must compile and match the
// interpreter exactly.
const mutualRecursionEachSrc = `def ec fn [[c:Map input:Map] [Boolean] [(input get (c get "f")) (c get "v") eq]]
def ep-all fn [[children:List input:Map] [Boolean] [(children each [input swap ep]) all]]
def ep-any fn [[children:List input:Map] [Boolean] [(children each [input swap ep]) any]]
def ep-not fn [[children:Map input:Map] [Boolean] [input children ec not]]
def ep-not fn [[children:List input:Map] [Boolean] [input (children 0 get) ep not]]
def ep fn [[pred:Map input:Map] [Boolean] [if ((pred get "kind") "group" eq) [(def group-op (pred get "op") def children quote (pred get "children") if (group-op "all" eq) [input children ep-all] [if (group-op "any" eq) [input children ep-any] [input children ep-not]])] [input pred ec]]]
`

func TestMutualRecursionEachAndGroupCompiles(t *testing.T) {
	stage1aCompiles(t, mutualRecursionEachSrc+
		`[(ep {kind:"group" op:"all" children:[{f:"a" v:1} {f:"b" v:2}]} {a:1 b:2})
  (ep {kind:"group" op:"all" children:[{f:"a" v:1} {f:"b" v:9}]} {a:1 b:2})]`)
}

func TestMutualRecursionEachOrGroupCompiles(t *testing.T) {
	stage1aCompiles(t, mutualRecursionEachSrc+
		`[(ep {kind:"group" op:"any" children:[{f:"a" v:9} {f:"b" v:2}]} {a:1 b:2})
  (ep {kind:"group" op:"any" children:[{f:"a" v:9} {f:"b" v:8}]} {a:1 b:2})]`)
}

func TestMutualRecursionEachEmptyChildrenCompiles(t *testing.T) {
	// Interpreter semantics: all [] → true, any [] → false. The compiled
	// closure loop must reproduce both without invoking the body.
	stage1aCompiles(t, mutualRecursionEachSrc+
		`[(ep {kind:"group" op:"all" children:[]} {a:1})
  (ep {kind:"group" op:"any" children:[]} {a:1})]`)
}

func TestMutualRecursionEachNestedGroupsCompile(t *testing.T) {
	// A group NESTED inside a group: the each closure re-enters ep, which
	// re-enters ep-all/ep-any — the run-spec↔test-describe recursion class,
	// this time through the token-body closure.
	stage1aCompiles(t, mutualRecursionEachSrc+
		`(ep {kind:"group" op:"all" children:[
  {kind:"group" op:"any" children:[{f:"a" v:9} {f:"b" v:2}]}
  {f:"a" v:1}
]} {a:1 b:2})`)
}

func TestMutualRecursionEachNotGroupSound(t *testing.T) {
	// The not-group routes through the 2-overload ep-not (OpCallUserPoly
	// territory); compile or fall back, but never diverge.
	stage1aSound(t, mutualRecursionEachSrc+
		`[(ep {kind:"group" op:"not" children:[{f:"a" v:9}]} {a:1 b:2})
  (ep {kind:"group" op:"not" children:[{f:"a" v:1}]} {a:1 b:2})]`)
}

// The bare shuffle-elision shape without recursion: a dynamic element swapped
// under a captured param and consumed by a single-overload user fn.
func TestDynamicSwapInEachBodyCompiles(t *testing.T) {
	stage1aCompiles(t, `def pick-b fn [[m:Map n:Map] [Integer] [(m get "b")]]
def apply-all fn [[xs:List env0:Map] [List] [(xs each [env0 swap pick-b])]]
(apply-all [{b:1} {b:2} {b:3}] {z:0})`)
}

// A DUPLICATING shuffle over a dynamic element (fresh output IDs — the
// CALL_NATIVE bake path, not the elision): `dup` feeding a poly-re-matched
// native must either compile or decline — never diverge — and produce
// interpreter-identical results either way.
func TestDynamicDupInEachBodySound(t *testing.T) {
	stage1aSound(t, `def sq-all fn [[xs:List] [List] [(xs each [dup mul])]]
(sq-all [2 3 4])`)
}

// NEGATIVE: a shuffle whose runtime shape breaks the each contract (a 2-net
// body via swap of the element under a pushed const) is the interpreter's own
// each_error — compiled mode must decline or raise identically, never silently
// keep one value.
func TestDynamicSwapTwoNetBodyStaysSound(t *testing.T) {
	stage1aSound(t, `def two fn [[xs:List] [List] [(xs each [7 swap])]]
(two [1 2 3])`)
}

// The loop-carried def rebind (decision.boru eval-table-first's `def found
// true` inside a for-arm, read as `found not` next iteration) COMPILES since
// the loop-persistent frame slots landed (NoteLoopCarried / RecordDefRebind /
// evStore — see bytecode_stage2_loopcarried_test.go for the full battery).
// Flipped from the sound-fallback pin to a compile+parity pin.
func TestLoopCarriedDefRebindStaysSound(t *testing.T) {
	stage1aCompiles(t, `def first-big fn [[xs:List] [Integer] [def found false def result 0 for (xs size) [def idx i def x (xs idx get) if (found not) [if (x 10 gt) [def result x def found true] []] []] end result]]
(first-big [3 12 40])`)
}
