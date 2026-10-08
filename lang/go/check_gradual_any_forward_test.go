package lang

import (
	"strings"
	"testing"
)

// TestGradualAnyForwardArgAccepted pins the gradual-Any forward-collection rule
// (engine.go matchSignature, forward Word-match). An Any-typed forward operand — a
// value flowed from a dynamic `get`, or a param bound to Any at a gradual call site —
// is optimistically accepted for a CONCRETE param in PURE CHECK mode. At runtime the
// operand is concrete and dispatches (or raises) exactly as the interpreter does, so
// the static analysis stays advisory instead of emitting a spurious no_signature.
//
// This is the template.boru pipeline cluster: `gen-program`/`lex-mustache`/`engine-known`
// recovered only because their String params were fed Any operands (engine/source/src
// flowed from `opts get` over an Options receiver). Cleared the 5 pipeline errors
// (template 12 -> 7) with compile==interpret intact (verify-bytecode PASSES — the
// emitter still DECLINES the gradual dispatch in compile mode, since the rule is gated
// on !Compiling, so force-compile declines rather than baking a wrong call).
//
// SOUND + NARROW: only an ANY operand is accepted (gradual). A provable CONCRETE
// mismatch (String -> Integer) is not Any and still flags.
func TestGradualAnyForwardArgAccepted(t *testing.T) {
	// POSITIVE: Any forward arg to a concrete (String) param — no false no_signature.
	for _, src := range []string{
		`def lex fn [ [s:String] [List] [ ["x"] ] ]
def lexgen fn [ [src:Any] [List] [ (lex src) ] ]
def _ (lexgen "hi")`,
		`def known fn [ [name:String] [Boolean] [ true ] ]
def caller fn [ [opts:Options] [Boolean] [ def engine (opts get "engine") (known engine) ] ]
def _ (caller {engine:"m"})`,
	} {
		a, _ := New()
		res, _ := a.Check(src)
		for _, d := range res.Diagnostics {
			if d.Code == "no_signature" {
				t.Errorf("Any forward operand to a concrete param must not emit no_signature (gradual): %s\nsrc:\n%s", d.Detail, src)
			}
		}
	}

	// NEGATIVE: a provable concrete mismatch (String -> Integer) MUST still flag — the
	// rule only relaxes the Any case, never a concrete-vs-concrete contradiction.
	const bad = `def need-int fn [ [n:Integer] [Integer] [ n ] ]
def caller fn [ [s:String] [Integer] [ (need-int s) ] ]
def _ (caller "x")`
	b, _ := New()
	res2, _ := b.Check(bad)
	flagged := false
	for _, d := range res2.Diagnostics {
		if d.Code == "no_signature" || strings.Contains(d.Detail, "no signature matches") {
			flagged = true
			break
		}
	}
	if !flagged {
		t.Errorf("a provable concrete String->Integer mismatch must still be flagged")
	}
}
