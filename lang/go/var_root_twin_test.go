package lang

import (
	"strings"
	"testing"
)

// TestVarInRootDoBodyCompiles pins the root `do [var [[[k 1]] k add 1]]`
// (code-bodies.tsv L197): `var` splices `def k 1 … __varundef k` onto the
// tape, and the synthesized tokens carried no source position, so the undef
// twin the do body left for the root's adoption (AdoptBodyTwins keys the
// twins on the body's token sites) had no site, stayed unplaced, and the
// twin regime's full-placement gate declined the program. The synthesized
// tokens carry the declaration NAME's position now, so the twins adopt
// like a hand-written `def k 1 … undef k` (which always compiled).
func TestVarInRootDoBodyCompiles(t *testing.T) {
	for _, src := range []string{
		`do [var [[[k 1]] k add 1]]`,
		`def k 9 end do [var [[[k 1]] k add 1]] end k`,
		`do [var [[[a 1] [b 2]] a add b]] end 5`,
		`do [var [[[k 1]] k add 1]] end do [var [[[k 2]] k mul 2]]`,
		`do [def m 3 var [[[k m]] k add 1]] end m`,
		// The shapes that compiled before the stamping keep compiling.
		`var [[[k 1]] k add 1]`,
		`def k 9 end var [[[k 1]] k add 1] end k`,
		`def a 7 end for 2 [do [var [[[a 1]] a add 1]]] end a`,
		`def f fn [[][Integer][do [var [[[a 1]] a add 1]]]] end f`,
		`do [def k 1 k add 1 undef k]`,
	} {
		requireEngineParity(t, src, true)
	}
	// The twins adopt as placed BIND_TWIN replays; nothing islands.
	dis := compileDisasm(t, `do [var [[[k 1]] k add 1]]`)
	if !strings.Contains(dis, "BIND_TWIN") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("the root do body's var twins must adopt as placed replays:\n%s", dis)
	}
}
