package lang

import (
	"strings"
	"testing"
)

// TestNUR276RunIsNoHazardAcrossAStatementEnd pins NUR276: a later
// statement's stack-collecting dispatch marked a computed `do` body's run a
// collection hazard across the `end` between them, and the compile declined
// (NUR121's "fn-value lead's argument was collected by a later dispatch")
// where the run's re-step could never have taken the value. The hazard scan
// stops at the statement end now: each program compiles with the
// interpreter's answer. Since the merge of main's #514 a run whose tokens
// are not proven binding-free arms the kept-defs latch, which declines the
// read after it; the scan's statement-end stop still lets a proven run
// compile.
func TestNUR276RunIsNoHazardAcrossAStatementEnd(t *testing.T) {
	const sv = `def s 'a' end `
	for _, src := range []string{
		`def n 3 end def mk fn [[][List][quote [7]]] end do (mk) end for n [1]`,
		sv + `def mk fn [[][List][quote [7]]] end do (mk) end s size`,
	} {
		requireEngineParity(t, src, true)
	}
	for _, src := range []string{
		sv + `def mk fn [[][List][quote [def s 'b']]] end do (mk) end s size`,
		sv + `def mk fn [[][List][quote [([x:String] => [x size])]]] end do (mk) end s size`,
	} {
		requireDeclineReason(t, src, keptDefsLatchReason)
	}
	// Negative: with no end between them the run's lambda DOES take the
	// value at its re-step — a sound decline, the hazard's (NUR121) or the
	// region's.
	src := sv + `def mk fn [[][List][quote [([x:String] => [x size])]]] end do (mk) s size`
	a := mustNew(t)
	prog, reason, _, _ := a.CompileCheck(src)
	if prog != nil || !(strings.Contains(reason, "NUR121") || strings.Contains(reason, keptDefsLatchReason)) {
		t.Errorf("%q: the run's argument, collected by a later dispatch in its own statement, declines; got prog=%v reason=%q", src, prog != nil, reason)
	}
}
