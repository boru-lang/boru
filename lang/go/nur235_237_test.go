package lang

import "testing"

// TestNUR235NamedFnValueMemberCalls pins NUR235's close: a fn value built
// from a `fn` literal is NAMED (only `afn` / `=>` make one anonymous), and a
// name always calls — a member read of a nullary one fires on both lanes,
// though its compiled closure shares a unit with anonymous values over the
// same body; the push carries the name. An anonymous value, one that needs
// an argument, and a `/v` read stay data.
func TestNUR235NamedFnValueMemberCalls(t *testing.T) {
	const named = `def mkg fn [[c:Any][Any][def g fn [[][Any][c]] {g: g/v}]] end def m (mkg 5) end `
	for _, r := range []struct{ src, want string }{
		{named + `m.g`, "[5]"},
		{named + `m.g/v`, "[fn g]"},
		{named + `(m.g) 1 add`, "[6]"},
		{`def mk fn [[c:Any][Any][def g ([] => [c]) {g: g/v}]] end def m (mk 5) end m.g`, "[fn g]"},
		{`def mkg fn [[c:Any][Any][def g fn [[n:Integer][Any][n c]] {g: g/v}]] end def m (mkg 5) end m.g`, "[fn g(Integer)]"},
		{`def mkg fn [[c:Any][Any][def g fn [[n:Integer][Any][n c add]] {g: g/v}]] end def m (mkg 5) end m.g 3`, "[8]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}

// TestNUR236SplicedConsumerDeopts pins NUR236's close: a gradual def read
// consumed by a spliced word's expansion (`j tp` over `def tp word
// [typeof]`) carries its deopt — the consumer's position names the word's
// definition, so the point is ordered by the event stream — and a read that
// holds a fn at run time dispatches it, as the interpreter's bare name does.
func TestNUR236SplicedConsumerDeopts(t *testing.T) {
	const h = `def tp word [typeof] def h fn [[m:Map][Any][def j (m get "f") j tp]] `
	const hg = `def tp word [typeof] def h fn [[m:Map][Any][def j (m get "f") j (m get "g") drop tp]] `
	for _, r := range []struct{ src, want string }{
		{h + `h {f: ([] => [42])}`, "[Integer]"},
		{h + `h {f: (fn [[] [Integer] [7]])}`, "[Integer]"},
		{h + `h {f: 5}`, "[Integer]"},
		{h + `h {f: "s"}`, "[ProperString]"},
		{hg + `h {f: ([] => [42]) g: 1}`, "[Integer]"},
		{hg + `h {f: 5 g: 1}`, "[Integer]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}

// TestNUR237LoopSplitRebindInABranch pins NUR237's close: a root def of a
// name an S5 first-value loop bind bound is registry-visible, because a
// top-level read of the name reads the live registry binding — the
// program's residual included, which resolves only after the events lower.
// A taken branch arm's rebind is seen after the merge on both lanes.
func TestNUR237LoopSplitRebindInABranch(t *testing.T) {
	for _, r := range []struct{ src, want string }{
		{`def x (for 2 [5]) def c true if c [def x 1] [] end x`, "[5 1]"},
		{`def x (for 2 [5]) def c true if c [def x 1] [def x 2] end x`, "[5 1]"},
		{`def x (for 2 [5]) def c false if c [def x 1] [] end x`, "[5 5]"},
		{`def x (for 2 [5]) def c true if c [def x 1] [] end undef x x`, "[5 5]"},
		{`def x (for 2 [5]) def f fn [[] [Any] [x]] end def c true if c [def x 1] [] end (f)`, "[5 1]"},
		{`def x (for 2 [5]) def x 7 x`, "[5 7]"},
		{`def x (for 2 [5]) for 2 [def x 3] x`, "[5 3]"},
		{`def x (for 3 [i]) def c true if c [def x 9] [] end x`, "[1 2 9]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
