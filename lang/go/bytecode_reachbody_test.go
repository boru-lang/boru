package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestReachBodyInertCompiles: the property-based-testing words Test.prop /
// Test.check-prop / Test.skip take TWO code bodies (a generator and a property,
// e.g. `[r.int 0 100]` and `[0 gte]`). These bodies are inert at the call —
// `prop` stores them in a PropertySpec map, `skip` discards them, `check-prop`
// CallBorus them inside its native handler — so the dispatch should bake them as
// const operands (a plain CALL_NATIVE). It declined only because a dot-access
// reach (`r.int`) inside a body was not admitted as an inert const MEMBER; with
// inertReachMember it now is, so all three compile natively (no FALLBACK island)
// and match the interpreter, RNG draws included.
//
// Negative: a STANDALONE dot-access (`m.a` at the statement pointer, not inside
// an inert body) must keep its ordinary get-lowering — the const-member
// admission must not freeze a reach that the engine expands in place. Pinned by
// asserting it still runs with parity (and would diverge if the reach were
// wrongly baked as frozen data).
func TestReachBodyInertCompiles(t *testing.T) {
	cases := []struct{ src, want string }{
		{`import "boru:test"  (Test.check-prop "nonneg" [r.int 0 100] [0 gte] 20 1 0) get "ok"`, "[true]"},
		{`import "boru:test"  (Test.skip "wip" [r.int 0 9] [false] 10 1 0) get "skipped"`, "[true]"},
		{`import "boru:test"  def p (Test.prop "nonneg" [r.int 0 100] [0 gte]) end p get "runs"`, "[100]"},
		// (Rand.map-from {a:[Rand.int 0 10]} also clears via this fix — its schema
		// map holds a Rand.int reach — but its result is a raw RNG draw that only
		// agrees compiled-vs-interpreter under the differential gate's fixed seed,
		// so it is pinned there, not here.)
	}
	for _, c := range cases {
		a, _ := New()
		prog, reason, _, _ := a.CompileCheck(c.src)
		if prog == nil {
			t.Errorf("%q: must compile (inert reach body), but declined: %q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile native (const body), not island:\n%s", c.src, prog.Disassemble())
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, errI := b.RunInterp(c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: parity broke: compiled=%v gotC=%v errC=%v gotI=%v want=%s",
				c.src, compiled, gotC, errC, gotI, c.want)
		}
	}

	// NEGATIVE — a standalone dot-access (`m.a`) at the statement pointer is NOT
	// a member of an inert compound; it lowers to a normal get and must keep
	// matching the interpreter. The const-member admission only fires inside a
	// never-evaluated body, so this path is unchanged — assert parity holds.
	const standalone = `def m {a:5}  m.a`
	d, _ := New()
	gotC, compiled, errC := d.RunCompiled(standalone)
	if noteCompileDefect(t, standalone, gotC, errC) {
		return
	}
	e, _ := New()
	gotI, errI := e.RunInterp(standalone)
	if errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[5]" {
		t.Errorf("%q: standalone dot-access parity broke (compiled=%v): gotC=%v errC=%v gotI=%v errI=%v",
			standalone, compiled, gotC, errC, gotI, errI)
	}
}
