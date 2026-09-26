package lang

import "testing"

// TestNUR238TrailingValueNoMatch pins NUR238's close: a VALUE applied as a
// trailing window it does not fit — a lambda literal, a `/v` delivery, at
// the top level or in a fn — is re-stepped as the interpreter re-steps it:
// an anonymous value parks as data, a named one raises uncalled_function.
// A bare read under a frame binding stays the word dispatch's
// signature_error (NUR107), and a window the value fits applies it.
func TestNUR238TrailingValueNoMatch(t *testing.T) {
	const g = `def g fn [[s:String] [String] [s]] end `
	for _, r := range []struct{ src, want string }{
		{`(5 ([s:String] => [s]))`, "[5 fn (String)]"},
		{`def lam ([s:String] => [s]) end (5 lam/v)`, "[5 fn lam(String)]"},
		// A CAPTURING lambda is a compiled closure: the same park — at the top
		// level through the quote-keeping trail, in a fn through the closure
		// arm, where the window stays as written at the frame's RET. (A word
		// consuming the park in place, `(5 f/v) size`, is NUR246's: the park's
		// count is runtime-variable, and a fixed layout declines.)
		{`def mk fn [[k:Integer][Function][([s:String] => [s k])]] end def lam (mk 1) end (5 lam/v)`, "[5 fn lam(String)]"},
		{`def mk fn [[k:Integer][Function][([s:String] => [s k])]] end def h fn [[f:Function] [Any] [(5 f/v)]] end h (mk 1)`, "ERROR:expected 1 return value(s), got 2"},
		{g + `(5 g/v)`, "ERROR:uncalled_function"},
		{g + `def h fn [[f:Function] [Any] [(5 f/v)]] end h g/v`, "ERROR:uncalled_function"},
		{g + `(g/v 5)`, "ERROR:uncalled_function"},
		{`(5 ([s:Integer] => [s 1 add]))`, "[6]"},
		{g + `("a" g/v)`, "[a]"},
		{`def ld fn [[g:Function x:Integer] [Function Integer] [(g x)]] end ld ([k:String] => [k]) 14`, "ERROR:signature_error"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
