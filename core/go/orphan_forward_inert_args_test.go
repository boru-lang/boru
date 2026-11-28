package core

import "testing"

// A call frame's unnamed arguments are INERT wherever the frame is
// stepped: the open paren carries their span, and the step loop passes
// over it (stepPastOpenParen). Two loops besides the main one step a
// frame's tokens — the retry after a forward left pending at the END OF
// INPUT is resolved (resolveOrphanedForwards), and the re-run of a group's
// tokens at its close (stepCloseParen's implicit end) — and until both
// passed over the span as the main loop does, a Function among the
// arguments was stepped there: `un/u negq/uv`, whose usurped call
// completes at the end of input, ended in `uncalled_function` for the
// `negq` it was only handed. The same rule, and the same fix, as the
// group a waiting word evaluates (NUR397).
func TestInertArgumentsStayInertWhenAForwardResolvesLate(t *testing.T) {
	const un = "def un fn( _ Any Any [ ] ) ; "
	for _, c := range []struct{ expr, want string }{
		// the end of input: the pending forward of the usurped call resolves
		// there, and the frame it splices is stepped by the retry loop
		{"run " + un + "un/u negq/uv", "fn [[a:Integer][Integer][]]"},
		// the close of a group: the same, by the group's own re-run
		{"run " + un + "( un/u negq/uv )", "fn [[a:Integer][Integer][]]"},
		// a function written in place, handed the same way
		{"run " + un + "un/u fn( n Integer Integer [ addq n 1 ] )", "fn [[n:Integer][Integer][addq n 1]]"},
		// and beneath a value the body leaves, where it is dropped unrun
		{"run def un fn( _ Any Any [ 7 ] ) ; un/u negq/uv", "7"},
	} {
		if got := evalCoreSpec(t, coreSpecRegistry(t), c.expr); got != c.want {
			t.Errorf("%s\n   got  %s\n   want %s", c.expr, got, c.want)
		}
	}
}

// Passing over the span passes over ONLY the arguments: what the body
// does with them, late or not, is unchanged — an argument the body calls
// is called, and a call that cannot be made is still refused as itself.
func TestALateForwardStillRunsTheBody(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		// the body reads the argument from the frame's stack
		{"run def un fn( _ Any Any [ negq/s ] ) ; un/u 5", "-5"},
		{"run def un fn( _ Any Any [ negq/s ] ) ; ( un/u 5 )", "-5"},
		// a body that fails over a function argument fails as the body
		{"run def un fn( _ Any Any [ negq/s ] ) ; un/u negq/uv", "ERROR:signature_error"},
		// a named parameter was never in the span
		{"run def un fn( n Any Any [ 7 ] ) ; un/u negq/uv", "7"},
	} {
		if got := evalCoreSpec(t, coreSpecRegistry(t), c.expr); got != c.want {
			t.Errorf("%s\n   got  %s\n   want %s", c.expr, got, c.want)
		}
	}
}
