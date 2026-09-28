package lang

import "testing"

// TestNUR242StatementIslands pins NUR242's last four programs and the paren
// apply over a data member: a root landing nested in a list literal or a
// branch arm, whose `/q` claim — or whose fire before the paren apply that
// takes the value as its method — has no compiled answer, and a paren
// apply whose method is data at run time. Each ran a member read and
// nothing else before the point, so its statement runs again on the
// interpreter from its first token (compiler's landing_restart.go), and the
// island's residual is the program's.
func TestNUR242StatementIslands(t *testing.T) {
	const q = `def z fn [[] [Atom] [(quote z)]] end def y fn [[] [Integer] [42]] end def h fn [[] [Integer] [42]] end `
	for _, r := range []struct{ src, want string }{
		{q + `def h fn [[x:Atom/q] [Atom Atom] [x x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end [(m.f y)]`, "[[y y]]"},
		{q + `def h fn [[x:Atom/q] [Atom] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end if true [(m.f add 1 2)] [0]`, "[add 1 2]"},
		{q + `def h fn [[x:Atom/q] [Atom] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end def dbl fn [[n:Integer][Integer][n mul 2]] end if true [(m.f y dbl)] [0]`,
			"ERROR:cannot call `dbl`"},
		{`def h fn [[] [Integer] [42]] end def h fn [[n:Integer] [Integer] [n add 1]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end def z fn [[] [Integer] [0]] end def y fn [[] [Integer] [42]] end if true [(m.f y)] [0]`,
			"[42 42]"},
		// The paren places a data member and its operand.
		{`def mk fn [[] [Map] [{f: 5}]] end def m (mk) end (m.f 7)`, "[5 7]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def m (mk) end [(m.f 7)]`, "[[5 7]]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def m (mk) end def g fn [[] [Any] [[(m.f 7)]]] end g`, "[[5 7]]"},
		// In a loop over a loop-invariant read, and in a code body.
		{q + `def h fn [[x:Atom/q] [Atom Atom] [x x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end for 1 [[(m.f y)]]`, "[[y y]]"},
		{q + `def h fn [[x:Atom/q] [Atom Atom] [x x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end do [[(m.f y)]]`, "[[y y]]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def m (mk) end for 1 [(m.f 7)]`, "[5 7]"},
		// A member a paren applies keeps its compiled apply.
		{`def h fn [[x:Integer] [Any] [x add 1]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end [(m.f 5)]`, "[[6]]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
