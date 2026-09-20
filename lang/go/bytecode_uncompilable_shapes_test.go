package lang

import "testing"

// Pins for the historical `uncompilable.boru` trio (authored against
// 407feda, where `boru --force-compile` aborted on each): three shapes
// that interpreted green while the strict compiler declined.
// All three compile natively today; these pins keep them compiling.
// The compile failure MESSAGES each shape used to produce still guard genuinely
// unsound siblings (e.g. the single-literal-body provenance compile failure,
// pinned in lang/go/test/stamp_residual_map_test.go and the langspec
// knownCompileFailures ratchet) — the classes narrowed, they did not vanish.
func TestUncompilableShapesNowCompile(t *testing.T) {
	// A. `do {…}` over a dynamically-typed value (a class instance's
	// field reads) — was "unannotated or opaque word do".
	mustCompileWithParity(t,
		`def box class { n:Integer } def b (make box { n: 42 })`+
			` def snapshot (do {v: [b.n], double: [b.n mul 2]})`+
			` snapshot.v add snapshot.double`,
		"[126]")

	// B. A computed result parked in a literal container consumed by
	// `make` — was "operand of unknown provenance or not statically
	// materialisable at make". (The original used the since-retired
	// `Array` type; `List` is its successor.)
	mustCompileWithParity(t,
		`def inc fn [ [x:Integer] [Integer] [ x add 1 ] ]`+
			` def pair (make List [ (inc 5) 99 ])`+
			` (pair get 0) add (pair get 1)`,
		"[105]")

	// C. A code-body word whose body `var`-binds the element and
	// mutates a captured container — was "code-body word each (Stage 2)".
	mustCompileWithParity(t,
		`def cells (flex {})`+
			` iota 3 each [ var [[i] cells set (convert String i) (i mul 10) drop 0 ] ]`+
			` drop (cells get "2")`,
		"[20]")

	// C'. The verbatim 407feda spelling of C (Array→List): the each body
	// `var`-binds the element and `set`s a captured LIST by integer index.
	// List set is copy-on-write, so the fresh list is discarded and the
	// binding stays [1 2 3] — the pin asserts BOTH engines agree on that
	// AND on the per-iteration body results, through the list-set path
	// the store-based C above never touches.
	mustCompileWithParity(t,
		`def cells (make List [1 2 3])`+
			` def _fill (iota 3 each [ var [[i] cells 0 i set end 0 ] ])`+
			` [_fill cells]`,
		"[[[0 0 0] [1 2 3]]]")
}
