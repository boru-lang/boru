package lang

import "testing"

// TestNUR276RunIsNoHazardAcrossAStatementEnd pins NUR276: a later
// statement's stack-collecting dispatch marked a computed `do` body's run a
// collection hazard across the `end` between them, and the compile declined
// (NUR121's "fn-value lead's argument was collected by a later dispatch")
// where the run's re-step could never have taken the value. The hazard scan
// stops at the statement end now: each program compiles with the
// interpreter's answer, the scan's statement-end stop holding for a proven
// run.
func TestNUR276RunIsNoHazardAcrossAStatementEnd(t *testing.T) {
	const sv = `def s 'a' end `
	for _, src := range []string{
		`def n 3 end def mk fn [[][List][quote [7]]] end do (mk) end for n [1]`,
		sv + `def mk fn [[][List][quote [7]]] end do (mk) end s size`,
	} {
		requireEngineParity(t, src, true)
	}
	// A binding body reads live (NUR282), and a run leaving a fn value takes
	// the do's count island (NUR348; it was the plain-run check's loud
	// defer, NUR213), end or no end.
	requireEngineParity(t, sv+`def mk fn [[][List][quote [def s 'b']]] end do (mk) end s size`, true)
	agreeOnBothLanes(t, sv+`def mk fn [[][List][quote [([x:String] => [x size])]]] end do (mk) end s size`, "[fn (String) 1]")
	agreeOnBothLanes(t, sv+`def mk fn [[][List][quote [([x:String] => [x size])]]] end do (mk) s size`, "[1]")
}
