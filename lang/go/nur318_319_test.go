package lang

import "testing"

// TestNUR318MemberReadModifiedAsData pins NUR318. A `/v` (or `/q`) after a
// member read says data: the interpreter's peek leaves the member it read as
// a value. The shaped-method and member-fn arrival models fired a 0-arg
// member the moment it landed, whatever followed its group: `def m {f: g/v}
// m.f/v` answered 7 for `fn g`. They stand aside for a marker now, the
// marker quotes the carrier, a word that re-steps its results over it is the
// designed decline a concrete fn value takes, and a fn body's count replay
// leaves a `/v` value as it stands.
func TestNUR318MemberReadModifiedAsData(t *testing.T) {
	const m = `def m {f: g/v} `
	for _, c := range []struct{ src, want string }{
		{m + `m.f/v`, "[fn g]"},
		{m + `m.f/v 5`, "[fn g 5]"},
		{m + `[m.f/v]`, "[[fn g]]"},
		{m + `[m.f/v 5]`, "[[fn g 5]]"},
		{m + `m.f/v typeof`, "[Function]"},
		{m + `m.f/q`, "[fn g]"},
		{m + `if true [m.f/v] [0]`, "[fn g]"},
		{m + `[1] each [drop m.f/v]`, "[[fn g]]"},
		{m + `def f fn [[][Any][m.f/v]] end f`, "[fn g]"},
		{m + `def f fn [[][Any][m.f/v 5]] end f`, "ERROR:expected 1 return value(s), got 2 — [fn g 5]"},
		{`def f fn [[][Any][g/v 5]] end f`, "ERROR:expected 1 return value(s), got 2 — [fn g 5]"},
		// The bare read still dispatches, and a def bound to the /v read
		// is called by its name.
		{m + `m.f`, "[7]"},
		{m + `m.f 5`, "[7 5]"},
		{m + `def h m.f/v h`, "[7]"},
		{`def m {f: l/v} m.f 5`, "[6]"},
		{`def m {f: l/v} m.f/v 5`, "[fn l(Integer) 5]"},
		// The data member above a literal: the program residual's rebuild
		// seats the member's read result where the in-place seating cannot.
		{m + `5 m.f/v`, "[5 fn g]"},
	} {
		agreeOnBothLanes(t, nur312Pre+c.src, c.want)
	}
	requireLoudDecline(t, nur312Pre+m+`m.f/v dup`, "function value reaches dup", "[7 7]")
}

// TestNUR319DefReadOfBranchUnion pins NUR319. A bare read of a def bound to a
// placed branch's union is its word's dispatch (ADR-011), which fires the fn
// alternative over what follows: `def r (if c [g/v] [0]) r 5` is `7 5`, and
// the compiled lane left the read as data before the 5.
func TestNUR319DefReadOfBranchUnion(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def c true def r (if c [g/v] [0]) r 5`, "[7 5]"},
		{`def c false def r (if c [g/v] [0]) r 5`, "[0 5]"},
		{`def c true def r (if c [g/v] [0]) 3 r 5`, "[3 7 5]"},
		{`def c true def r (if c [g/v] [0]) 3 r`, "[3 7]"},
		{`def c true def r (if c [g/v] [0]) r`, "[7]"},
		{`def c true def r (if c [(mkf)] [0]) r 5`, "[7 5]"},
		// A `/v` read of it is data.
		{`def c true def r (if c [g/v] [0]) r/v 5`, "[fn r 5]"},
	} {
		agreeOnBothLanes(t, nur312Pre+c.src, c.want)
	}
	requireLoudDecline(t, nur312Pre+`def c true def r (if c [l/v] [0]) r 5`, "bound to a branch result an arm of which leaves a fn value", "[6]")
}
