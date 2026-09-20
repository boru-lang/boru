package lang

import (
	"testing"
)

// A COMPUTED map literal (`{k:(expr)}`) consumed in-frame — bound to a value-def
// local and returned as a fn body result, or read by a downstream word — records
// an OpMakeMap assembly so it resolves as a real per-run event. Before this it
// only recorded for make's construction body (dataMap), so the same map in a
// plain fn body declined "body result of unknown provenance" even though the
// interpreter runs it. The recording is gated on in-frame CONSUMPTION: a DEFERRED
// residual (a bare map tail, auto-evaluated after the frame pops) must still
// decline, because compiling it in-frame would diverge from the interpreter.
//
// The POSITIVE (value-parity) halves of this suite — including the do-map
// variadic-arm shapes (`do {…}` in if arms / recursive returns, non-variadic
// single-map results) — are corpus rows now: lang/spec/bytecode-migrated.tsv
// (WS4 migration), where the census owns native-compile + parity. This file
// keeps the NEGATIVES: the deferred-residual divergence guard the corpus
// cannot host (declining rows never enter the main corpus).
func TestComputedMapInFnBodyCompiles(t *testing.T) {
	// Negative: a DEFERRED residual — a bare computed-map fn-body tail whose value
	// references a fn param — errors in the interpreter (the data-context paren is
	// evaluated after the frame pops, so the param is gone). The compiled path must
	// NOT silently succeed where the interpreter errors: it must decline and fall
	// back, so RunCompiledStrict surfaces a force-compile failure rather than a value.
	negatives := []struct {
		name, src string
	}{
		{"paren value", `def f ([a:Integer] => [{x:(a 1 add)}]) (f 5)`},
		{"list value", `def f ([a:Integer] => [{x:[a add 1]}]) (f 5)`},
	}
	for _, c := range negatives {
		t.Run("deferred-residual declines/"+c.name, func(t *testing.T) {
			// The interpreter itself errors on this shape (undefined word a).
			b, _ := New()
			if _, werr := b.RunInterp(c.src); werr == nil {
				t.Fatalf("expected the interpreter to error on the deferred-residual map")
			}
			a, _ := New()
			if _, err := a.RunCompiledStrict(c.src); err == nil {
				t.Fatal("compiled path produced a value where the interpreter errors — divergence")
			} else if codeOf(err) != "compile_failed" {
				t.Errorf("expected a compile failure, got %q", err.Error())
			}
		})
	}
}
