package lang

import "testing"

// NUR119: a fn value read through a PARAM's `/v` renders under the param's
// name on BOTH lanes — `def app fn [[g:Function][Function][g/v]]` hands back
// `fn g(Integer)` whether the caller placed the result by a paren or by the
// bare spelling, and whether the return slot is Function- or Any-typed. The
// named-value twin (`app sq/v`) declines on the compiled lane under the
// render gate (closure render); the lanes never disagree on a value.
func TestFnValueReadThroughParamRendersAlike(t *testing.T) {
	for _, src := range []string{
		`def app fn [[g:Function][Function][g/v]]  (app (z:Integer => [mul 3 z]))`,
		`def app fn [[g:Function][Any][g/v]]  app (z:Integer => [mul 3 z])`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || errC != nil || errI != nil {
			t.Fatalf("%s: compiled=%v errC=%v errI=%v", src, compiled, errC, errI)
		}
		requireParity(t, src, gotC, errC, gotI, errI)
		if len(gotI) != 1 || gotI[0] != "fn g(Integer)" {
			t.Fatalf("%s: interpreted %v, want the value rendered under the param's name", src, gotI)
		}
	}
	requireEngineParity(t, `def sq (z:Integer => [mul z z])  def app fn [[g:Function][Function][g/v]]  app sq/v`, false)
}
