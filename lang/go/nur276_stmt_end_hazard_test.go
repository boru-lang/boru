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
// interpreter's answer, a run that leaves a lambda included (it stands
// aside across the end).
func TestNUR276RunIsNoHazardAcrossAStatementEnd(t *testing.T) {
	const sv = `def s 'a' end `
	for _, src := range []string{
		`def n 3 end def mk fn [[][List][quote [7]]] end do (mk) end for n [1]`,
		sv + `def mk fn [[][List][quote [7]]] end do (mk) end s size`,
		sv + `def mk fn [[][List][quote [def s 'b']]] end do (mk) end s size`,
		sv + `def mk fn [[][List][quote [([x:String] => [x size])]]] end do (mk) end s size`,
	} {
		requireEngineParity(t, src, true)
	}
	// Negative: with no end between them the run's lambda DOES take the
	// value at its re-step, and the hazard stands — a sound decline.
	src := sv + `def mk fn [[][List][quote [([x:String] => [x size])]]] end do (mk) s size`
	a := mustNew(t)
	prog, reason, _, _ := a.CompileCheck(src)
	if prog != nil || !strings.Contains(reason, "NUR121") {
		t.Errorf("%q: the run's argument, collected by a later dispatch in its own statement, declines (NUR121); got prog=%v reason=%q", src, prog != nil, reason)
	}
}
