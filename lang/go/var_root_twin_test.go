package lang

import (
	"strings"
	"testing"
)

// TestVarInRootDoBodyCompiles pins the root `do [(([k] => [k add 1]) 1)]`
// (code-bodies.tsv L197). As `do [var [[[k 1]] k add 1]]` the construct
// spliced `def k 1 … __varundef k` onto the tape with no source positions,
// so the undef twin the do body left for the root's adoption had no site
// and the twin regime declined the program; the construct is gone and the
// lambda call binds `k` in its own frame — these shapes compile like a
// hand-written `def k 1 … undef k` always did.
func TestVarInRootDoBodyCompiles(t *testing.T) {
	for _, src := range []string{
		`do [(([k] => [k add 1]) 1)]`,
		`def k 9 end do [(([k] => [k add 1]) 1)] end k`,
		`do [(([a b] => [a add b]) 1 2)] end 5`,
		`do [(([k] => [k add 1]) 1)] end do [(([k] => [k mul 2]) 2)]`,
		`do [def m 3 (([k] => [k add 1]) m)] end m`,
		// The shapes that compiled before the stamping keep compiling.
		`(([k] => [k add 1]) 1)`,
		`def k 9 end (([k] => [k add 1]) 1) end k`,
		`def a 7 end for 2 [do [(([a] => [a add 1]) 1)]] end a`,
		`def f fn [[][Integer][do [(([a] => [a add 1]) 1)]]] end f`,
		`do [def k 1 k add 1 undef k]`,
	} {
		requireEngineParity(t, src, true)
	}
	// The lambda call is the body's own unit; nothing islands.
	dis := compileDisasm(t, `do [(([k] => [k add 1]) 1)]`)
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("the root do body's lambda call must compile natively:\n%s", dis)
	}
}
