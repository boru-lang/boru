package lang

import (
	"fmt"
	"strings"
	"testing"
)

// closure_identity_test.go pins the two review findings on the closure VALUE
// bridge (Codex on PR #444, the twenty-fifth increment's ClosureAsFnDef):
// a compiled closure carries the identity token its push minted, and the
// FnDefInfo the interpreter's re-step bridges it to adopts that token — so
// `dup eq` over a produced closure is true on both lanes (each bridge minted
// its own identity before, and the compiled lane answered false); and the
// bridge stands in for one dispatch only — the tape keeps the closure, so a
// parked copy escapes as the payload, never as a handler bound to the
// finished run's VM context (pinned at the seam in core's
// engine_closure_bridge_test.go; here the park's VALUE agrees).
func TestClosureIdentityParity(t *testing.T) {
	const mk = `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  `
	rows := []struct{ src, want, note string }{
		{mk + `[(mk 3)] each [dup eq]`, "true", "two copies of one closure are one function"},
		{mk + `[(mk 3) (mk 3)] fold [eq] 0`, "false", "two constructions are two functions"},
		{mk + `[(mk 3)] each [dup eq/v drop]`, "fn (Integer)", "the copies render as the source lambda"},
		{mk + `[(mk 3)] each ["x" swap]`, "fn (Integer)", "a no-match parks the closure itself"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if s := fmt.Sprint(gotC); !strings.Contains(s, c.want) {
			t.Errorf("%q: %s, want %s (%s)", c.src, s, c.want, c.note)
		}
	}
}
