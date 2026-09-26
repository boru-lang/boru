package lang

import "testing"

// TestNUR267ComputedBodyUndefTakesEffect pins NUR267: a computed body's
// `undef` unbinds in the body's caller's scope, as the interpreter's `do`
// runs it there. The run-time token-body stamp compiled the body as a
// detached fn unit, which did not model the unbind, so a read after it
// answered the value the name held before (`[undef x x]` over `def x 99`
// was 99 for the interpreter's undefined word). Such a body stays the
// interpreter's (compiler bodyUndefs).
func TestNUR267ComputedBodyUndefTakesEffect(t *testing.T) {
	for _, body := range []string{
		`undef x x`,         // undefined word, caught by do: an Error value
		`def x 5 undef x x`, // the body's own def popped: 99
		`undef x def x 7 x`, // [7]
		`undef x 3`,         // [3]
		`x undef x`,         // [99]
		`[undef x] do x`,    // a nested list: [99]
		`def x 5 x`,         // no undef: stamped as before, [5]
	} {
		requireEngineParity(t, `def mk fn [[][List][quote [`+body+`]]] end def x 99 end do (mk)`, true)
	}
}
