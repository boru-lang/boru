package lang

import (
	"fmt"
	"testing"
)

// fn_body_embed_keep_test.go pins the selective (spine-only) freshen
// (2026-09-24, PR #225 P1's open item closed): a fn-unit body literal that
// EMBEDS an enclosing binding's container — `[c]` with `def c [9]` outside
// the fn — used to decline, because the interpreter constructs the OUTER
// literal fresh per call while the member stays the binding's one instance,
// which neither a deep-clone freshen nor a shared const models. The fresh
// push (OpPushConstFresh, and the multi-read OpPushConstFreshLocal) now
// clones the spine and keeps the embedded members (Program.ConstKeep,
// core.CloneValueKeeping): `((mk) get 0) eq c` is true and `(mk) eq (mk)`
// false on both lanes, at any depth of the literal, in a map literal, and
// inside a module fn (kg/main.boru's `[[repo-entity] …]` is the real-program
// row, graduated in the langspec ledger).

var fnBodyEmbedKeepRows = []struct{ label, src, want string }{
	{"the member is the binding's instance", `def c [9] def mk fn [[] [List] [[c]]] ((mk) get 0) eq c`, "[true]"},
	{"the spine is fresh per call", `def c [9] def mk fn [[] [List] [[c]]] (mk) eq (mk)`, "[false]"},
	{"a map literal's member", `def m {a:1} def mk fn [[] [Map] [{x:m}]] ((mk) get "x") eq m`, "[true]"},
	{"a member beside a nested literal", `def c [9] def mk fn [[][List][[[c] c]]] ((mk) get 1) eq c`, "[true]"},
	{"the member inside the nested literal", `def c [9] def mk fn [[][List][[[c] c]]] (((mk) get 0) get 0) eq c`, "[true]"},
	{"the nested literal is fresh per call", `def c [9] def mk fn [[][List][[[c] c]]] ((mk) get 0) eq ((mk) get 0)`, "[false]"},
	{"a multi-read literal keeps the member", `def c [9] def mk fn [[][List][def t [c]  t drop  t]] ((mk) get 0) eq c`, "[true]"},
	{"a multi-read literal is fresh per call", `def c [9] def mk fn [[][List][def t [c]  t drop  t]] (mk) eq (mk)`, "[false]"},
	{"a module fn's literal over a module-level binding", `import module [def re [1 2] end def cb fn [[][List][[re]]] end export "G" {cb: cb/v}] end (G.cb) eq (G.cb)`, "[false]"},
	{"kg's shape: a flattened literal over a module-level map", `import module [def re {key:'repo'} end def cb fn [[xs:List][List][flatten [[re] xs]]] end export "G" {cb: cb/v}] end G.cb [1 2]`, "[[{key:'repo'} 1 2]]"},
}

func TestFnBodyEmbedKeepParity(t *testing.T) {
	for _, row := range fnBodyEmbedKeepRows {
		a, _ := New()
		gotI, errI := a.RunInterp(row.src)
		b, _ := New()
		var entries []string
		disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
			if ev.Attribution == "" {
				entries = append(entries, ev.Seam)
			}
		})
		gotC, compiled, errC := b.RunCompiled(row.src)
		disarm()
		if errI != nil || errC != nil || !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != row.want {
			t.Errorf("%s: compiled %v/%v (%v) interp %v/%v, want %s\n  %s", row.label, gotC, errC, compiled, gotI, errI, row.want, row.src)
		}
		if len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
	}
}
