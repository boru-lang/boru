package lang

// Single-use user-call value-def SEATING pins (COMPILE FAILURE-CLOSURE §9.4 force-
// compile quick-win). A NAMED user-call value-def read exactly ONCE
// (`def a (n g)`) used to be left LOOSE on the simulated stack — the
// "leave a single-use user call on the Stage-3 layout" case. That is fine
// only while nothing is pushed ABOVE it before the read: once two more
// computed operands land on top (`(n g) (n g)`), the buried `a` cannot be
// seated as the next operand and the unit declined
// `def a: value-def result is not adjacent on top`. This is the trie/stats
// leaf in miniature (`def a (nd g) … a b add c add` — the chained op's
// operands are not adjacent).
//
// planValueDefLocals now promotes such a single-use named user-call value-def
// to a frame local (store once, re-push per reference — the interpreter's
// def-evaluates-once semantics), so it seats regardless of what sits above it.
// The promotion EXEMPTS a plain loop-carried store source (an evStore rebind
// like `def acc (nodes measure)`): lowerStore seats that off the sim top, so
// promoting it would strand the store — see TestNarrowedQuoteDefReadInLoopArm
// Compiles (bytecode_stage2_narrowid_test.go), whose lowerStore source-on-top
// coverage is the guard for that exemption.

import "testing"

// A single-use user-call value-def buried under two computed operands: without
// the promotion the buried `a` declines "not adjacent on top"; with it, `a`
// stores to a frame local and re-pushes for the read. compile == interpret.
func TestSingleUseUserCallValueDefSeatsUnderComputedOperands(t *testing.T) {
	stage1aCompiles(t, `def g fn [[x:Integer] [Integer] [x 1 add]]
def h fn [[n:Integer] [Integer] [def a (n g) (n g) (n g) a add add]]
(h 4) end`)
}

// The same shape through a non-commutative chain (`mul mul`) — proves the
// re-push preserves operand order, not just that a value reaches the op.
func TestSingleUseUserCallValueDefSeatsInMulChain(t *testing.T) {
	stage1aCompiles(t, `def g fn [[x:Integer] [Integer] [x 1 add]]
def h fn [[n:Integer] [Integer] [def a (n g) (n g) (n g) a mul mul]]
(h 2) end`)
}

// The loop-carried store-source counterpart the promotion must NOT over-reach:
// `def acc (nodes measure)` rebinds a carried slot from a user call inside a
// for-arm. lowerStore seats the source directly off the sim top (OpStoreLocal),
// so it stays UNpromoted — the narrowing that keeps the single-use trigger from
// stranding the store. Compiles + parity either way; this pins the behaviour,
// the source-on-top block coverage pins the path.
func TestLoopCarriedUserCallRebindStaysOnStorePath(t *testing.T) {
	stage1aCompiles(t, `def measure fn [[xs:List] [Integer] [xs size]]
def treewalk fn [[tree:Map] [Integer] [def nodes quote (tree get "nodes") def acc 0 for 3 [def _i i if (acc 0 eq) [def acc (nodes measure)] []] end acc]]
{nodes: [10 20 30]} treewalk end`)
}
