package lang

import (
	"strings"
	"testing"
)

// TestNUR281RootMapLiteralReadDeclines pins NUR281's close: a MAP literal's
// value read after a computed keep-defs body at the program level folded to
// a constant off the emit path (core AutoEvalMap's check-mode fold), so the
// kept-defs latch never saw the read and the map held the pre-body binding —
// `[1 2] each (mk) end {a: x}` over `[def x 5 1]` answered `{a:99}` for the
// interpreter's `{a:5}`, silent. When the latch arms at the root the pass
// now generalises root value bindings (compiler generaliseRootValues, the
// speculative undef's transition `do`'s check half already makes), so the
// fold stands aside and the read reaches the latch, which declines.
func TestNUR281RootMapLiteralReadDeclines(t *testing.T) {
	const mk = `def x 99 end def mk fn [[][List][quote [def x 5 1]]] end `
	const reason = "a computed body keeps its defs and undefs in the enclosing scope"
	for _, src := range []string{
		mk + `[1 2] each (mk) end {a: x}`,
		mk + `def b (mk) end [1 2] each b end {a: x}`,
		mk + `[1 2] each (mk) end {a: x b: 1}`,
		mk + `[1 2] each (mk) end [{a: x}]`,
		mk + `[1 2] each (mk) end {a: (x)}`,
	} {
		prog, why, _, err := mustNew(t).CompileCheck(src)
		if prog != nil || err != nil || !strings.Contains(why, reason) {
			t.Errorf("%q: want the latch's decline, got prog=%v reason=%q err=%v", src, prog != nil, why, err)
		}
	}
	// Negative: a body proven to bind nothing arms no latch, and the map
	// keeps its folded value, which is the interpreter's.
	requireEngineParity(t, `def x 99 end def mk fn [[][List][quote [add 1]]] end [1 2] each (mk) end {a: x}`, true)
}
