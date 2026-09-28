package lang

import "testing"

// TestUserPolyResultBoundDeadAtTheRoot pins the lowering of a user poly
// call whose result a root def binds and nothing reads: the root write-back
// consumes it (bindConsumes), so the call pushes no drop, on both lanes.
// Its negative twin reads the binding, so the result is no longer dead.
func TestUserPolyResultBoundDeadAtTheRoot(t *testing.T) {
	const f = `def f fn [[x:Integer][Integer][x add 1] [x:String][String][x]] end def c true end def y (if c [1] ['a']) end `
	requireEngineParity(t, f+`def z (f y) end 7`, true)
	requireEngineParity(t, f+`def z (f y) end z`, true)
}
