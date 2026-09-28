package lang

import (
	"fmt"
	"testing"
)

// TestNUR281RootMapLiteralReadAgrees pins NUR281's close: a MAP literal's
// value read after a computed keep-defs body at the program level folded to
// a constant off the emit path (core AutoEvalMap's check-mode fold), so the
// kept-defs latch never saw the read and the map held the pre-body binding —
// `[1 2] each (mk) end {a: x}` over `[def x 5 1]` answered `{a:99}` for the
// interpreter's `{a:5}`, silent. When the latch arms at the root the pass
// generalises root value bindings (compiler generaliseRootValues, the
// speculative undef's transition `do`'s check half already makes), so the
// fold stands aside; the read is then seated live (NUR282) and the map is
// assembled from the lookup, the interpreter's answer.
func TestNUR281RootMapLiteralReadAgrees(t *testing.T) {
	const mk = `def x 99 end def mk fn [[][List][quote [def x 5 1]]] end `
	for _, c := range []struct{ src, want string }{
		{mk + `[1 2] each (mk) end {a: x}`, "[[1 1] {a:5}]"},
		{mk + `def b (mk) end [1 2] each b end {a: x}`, "[[1 1] {a:5}]"},
		{mk + `[1 2] each (mk) end {a: x b: 1}`, "[[1 1] {a:5 b:1}]"},
		{mk + `[1 2] each (mk) end [{a: x}]`, "[[1 1] [{a:5}]]"},
		{mk + `[1 2] each (mk) end {a: (x)}`, "[[1 1] {a:5}]"},
		// A body proven to bind nothing arms no latch, and the map keeps its
		// folded value, which is the interpreter's.
		{`def x 99 end def mk fn [[][List][quote [add 1]]] end [1 2] each (mk) end {a: x}`, "[[2 3] {a:99}]"},
	} {
		requireEngineParity(t, c.src, true)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: the interpreter answers %s, got %v / %v", c.src, c.want, got, err)
		}
	}
}
