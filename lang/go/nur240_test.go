package lang

import "testing"

// TestNUR240ArmMemberNoMatchCode pins NUR240's close: a definite no-match of
// a module member's fn value, caught by `do … error`, is `uncalled_function`
// on both lanes wherever the call sits — at the body's root, or inside a
// branch arm, where the unit trap does not reach. NUR238's value-trail
// no-match gives the arm's call the interpreter's code (a named value that
// matches nothing raises uncalled_function); it raised signature_error.
func TestNUR240ArmMemberNoMatchCode(t *testing.T) {
	const m = `import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] export "M" {dec: dec/v} ] end `
	for _, r := range []struct{ src, want string }{
		{m + `def msg (do [if true [(true 5 M.dec)] [1] "no-raise"] error [dot code]) msg`, "[uncalled_function]"},
		{m + `def c true def msg (do [if c [(true 5 M.dec)] [1] "no-raise"] error [dot code]) msg`, "[uncalled_function]"},
		{m + `def msg (do [(true 5 M.dec) "no-raise"] error [dot code]) msg`, "[uncalled_function]"},
		{m + `def msg (do [if true [M.dec false 5] [1]] error [dot code]) msg`, "[5]"},
		{m + `def msg (do [if true [M.dec true 5] [1]] error [dot code]) msg`, "[bad_input]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
