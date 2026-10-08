package lang

import (
	"strings"
	"testing"
)

// A binding word reached through an ALIAS — `def mydef def/v` — installs
// into the calling frame exactly as the word itself would, so the leaf
// rule (core.bodyNeedsFrameState) must see it: the def a fn body makes
// through the alias is body-local and gone after the call on every fn
// path, as a literal `def` in the body would be. Before the rule resolved
// aliases, the named path leaked such a def (since the speed plan's leaf
// fast path) and the CallBoru path started to once it shared the rule
// (Codex P1 on #532). The compiled lane refuses these programs at the
// check pass (`y` undefined after the call), which is the same reading.
func TestAliasedBindingWordIsBodyLocalOnEveryPath(t *testing.T) {
	cases := []struct {
		name, src string
	}{
		{"named fn", `def mydef def/v  def f fn [[n:Integer][Integer][mydef y 2 n]]  f 1  y`},
		{"def-bound lambda applied by each", `def mydef def/v  def g ([n:Integer] => [mydef y 2 n])  each g/v [1]  y`},
		{"module fn through CallBoru", `import module [def mydef def/v def f fn [[n:Integer][Integer][mydef y 2 n]] def g fn [[][Integer][y]] export "M" {f: f/v g: g/v}] end M.f 1 M.g`},
	}
	for _, tc := range cases {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		_, err = a.RunInterp(tc.src)
		if err == nil || !strings.Contains(err.Error(), "undefined word: y") {
			t.Errorf("%s: the def made through the alias leaked past the call: err=%v", tc.name, err)
		}
	}
}
