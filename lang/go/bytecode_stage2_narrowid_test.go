package lang

import "testing"

// Stage-2 narrowing-through-use provenance pins (voxgig zero-compile failures plan,
// "quote of a computed get" leaf). The decision.boru eval-tree walker —
// `def nodes quote (tree get "nodes")` … `(nodes next-id find-node)` — used
// to decline "fn call operand of unknown provenance". The quote was
// incidental (its evaluated-paren sig is a RunInCheck identity, ID
// preserved): the real leaf was narrowDynamicUses. A def-bound dynamic Any
// (a computed get result) consumed at a typed slot rebinds the NAME to a
// narrowed bound, but TandValues assembled that bound as a fresh Value —
// fresh ID, no producedBy home — so every LATER read of the name declined.
// Two-part fix:
//
//  1. narrowDynamicUses re-stamps the narrowed rebind with the current
//     binding's ID (the narrowing refines the STATIC bound of the SAME
//     runtime value — no new value exists at run time), so producedBy still
//     resolves the name to its original producing event.
//  2. resolveOperand's type-operand ID-collision guard exempts DYNAMIC
//     carriers: the narrowed bound can be a typed-list shape ([:Any]) that
//     trips IsTypeBody, but a dynamic carrier is a gradual VALUE stand-in,
//     never a type body — its ID-matched event genuinely produced it.
//
// The differential corpus is blind to these off-corpus shapes, so pin them
// (the memory-noted RunCompiledStrict discipline).

// The leaf shape in miniature: quote of a computed get, def-bound, feeding a
// user fn TWICE (the second use reads the narrowed rebind — that read used to
// decline). Compiles + parity.
func TestQuoteComputedGetTwoUsesCompiles(t *testing.T) {
	stage1aCompiles(t, `def measure fn [[xs:List] [Integer] [xs size]]
def treewalk fn [[tree:Map] [Integer] [def nodes quote (tree get "nodes") def a (nodes measure) def b (nodes measure) a add b]]
{nodes: [10 20 30], root: "a"} treewalk end`)
}

// The same leaf WITHOUT quote — proves the fix targets the narrowing rebind,
// not the quote: a plain computed-get def read at two typed slots.
func TestNarrowedDefTwoUsesCompiles(t *testing.T) {
	stage1aCompiles(t, `def measure fn [[xs:List] [Integer] [xs size]]
def treewalk fn [[tree:Map] [Integer] [def nodes (tree get "nodes") def a (nodes measure) def b (nodes measure) a add b]]
{nodes: [10 20 30], root: "a"} treewalk end`)
}

// The eval-tree control-flow shape: the narrowed name read inside an if arm
// inside a for body (a cross-fragment read of the rebound binding) AND again
// after the loop. Compiles + parity.
func TestNarrowedQuoteDefReadInLoopArmCompiles(t *testing.T) {
	stage1aCompiles(t, `def measure fn [[xs:List] [Integer] [xs size]]
def treewalk fn [[tree:Map] [Integer] [def nodes quote (tree get "nodes") def acc 0 for 3 [def _i i if (acc 0 eq) [def acc (nodes measure)] []] end (acc add (nodes measure))]]
{nodes: [10 20 30], root: "a"} treewalk end`)
}

// quote of a WORD (the /q'd-Atom sig — a genuinely non-identity quote mode:
// the word becomes an inert quoted symbol, not the bound value). Must stay
// sound and identical on both surfaces.
func TestQuoteWordAtomModeSound(t *testing.T) {
	stage1aSound(t, `def hd 7
(quote hd) end`)
}

// quote of a LITERAL list: Quoted=true suppresses the end-of-run auto-eval
// (`quote [1 add 2]` residual stays [1 add 2], NOT [3]). The narrowing fix
// must not disturb the Quoted residual gate on either surface.
func TestQuoteLiteralListResidualSound(t *testing.T) {
	stage1aSound(t, `quote [1 add 2] end`)
}

// The unquoted control for the pin above: a parser-created Eval list residual
// DOES auto-eval at end of run ([1 add 2] → [3]) — both surfaces agree.
func TestUnquotedLiteralListResidualSound(t *testing.T) {
	stage1aSound(t, `[1 add 2] end`)
}

// End-of-run residual of the NARROWED value itself: the quoted computed-get
// list is returned from the fn and left as the program residual. A runtime-
// created list is never auto-evaluated regardless of Quoted, and the
// identity-preserving rebind must not change that — parity, byte-identical.
func TestNarrowedQuotedValueResidualCompiles(t *testing.T) {
	stage1aCompiles(t, `def measure fn [[xs:List] [Integer] [xs size]]
def keep fn [[tree:Map] [List] [def nodes quote (tree get "nodes") def n (nodes measure) nodes]]
{nodes: [10 20 30]} keep end`)
}
