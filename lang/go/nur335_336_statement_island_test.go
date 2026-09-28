package lang

import "testing"

// nur335_336_statement_island_test.go pins the root paren apply over a
// container member the pass could not type, `(m.f 7)`: the interpreter's
// paren applies the member when it is a fn and places it with its args when
// it is data. The compiled apply takes the data arm through the statement's
// island (compiler landing_restart.go), which seats the values beneath the
// statement and runs it again on the interpreter.

const (
	nurDataMk = `def mk fn [[] [Map] [{f: 5}]] end def m (mk) end `
	nurFnMk   = `def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def m (mk) end `
)

// TestNUR335IslandSeatsReadsWhereTheyWereRead: a def-bound value read by the
// statement itself is the statement's, not a value beneath it — the island
// used to seat it (the value carries the def's earlier position) and then
// push it again, `[3 3 5 7]` for `[3 5 7]`. A read before the statement is
// beneath it, and one after is a later statement's.
func TestNUR335IslandSeatsReadsWhereTheyWereRead(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{nurDataMk + `def k 3 end k (m.f 7)`, "[3 5 7]"},
		{nurDataMk + `m (m.f 7)`, "[{f:5} 5 7]"},
		{nurDataMk + `def k 3 end k [(m.f 7)]`, "[3 [5 7]]"},
		{nurDataMk + `def k 3 end k end k (m.f 7)`, "[3 3 5 7]"},
		{nurDataMk + `def k 3 end k k end k (m.f 7)`, "[3 3 3 5 7]"},
		{nurDataMk + `def k 3 end k drop k (m.f 7)`, "[3 5 7]"},
		// The receiver of the statement's member read is the reach's own.
		{nurDataMk + `m end m (m.f 7)`, "[{f:5} {f:5} 5 7]"},
		{nurDataMk + `m end m (m.f 7) m`, "[{f:5} {f:5} 5 7 {f:5}]"},
		{nurDataMk + `def k 3 end k (m.f 7) k`, "[3 5 7 3]"},
		{nurDataMk + `def k 3 end k (m.f 7) end k`, "[3 5 7 3]"},
		{nurDataMk + `def k 3 end k (m.f 7) end k (m.f 8)`, "[3 5 7 3 5 8]"},
		{nurDataMk + `3 end def k 3 end k (m.f 7)`, "[3 3 5 7]"},
		{nurDataMk + `def k {a:1} end k end k (m.f 7)`, "[{a:1} {a:1} 5 7]"},
		{nurDataMk + `def k (3 dup) end k (m.f 7)`, "[3 3 5 7]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def m ((mk) dup) end m (m.f 7)`, "[{f:5} {f:5} 5 7]"},
		// A second paren apply in the statement: the island writes the
		// first's value in its place (the first is a fn's, or its own
		// island ran).
		{nurDataMk + `def k 3 end k (m.f 7) (m.f 8)`, "[3 5 7 5 8]"},
		// The same programs over a fn member apply it.
		{nurFnMk + `def k 3 end k (m.f 7)`, "[3 8]"},
		{nurFnMk + `m (m.f 7)`, "[{f:fn (Integer)} 8]"},
		{nurFnMk + `def k 3 end k [(m.f 7)]`, "[3 [8]]"},
		{nurFnMk + `def k 3 end k (m.f 7) (m.f 8)`, "[3 8 9]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336DataMemberParenApply: the paren apply over a member that holds
// data at run time answers the interpreter's placement — a leading map
// literal, a list holding one and a def-bound type are placed beneath the
// statement now (the island used to be refused over them, and the compiled
// apply then failed inside the runtime).
func TestNUR336DataMemberParenApply(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{nurDataMk + `{a:1} (m.f 7)`, "[{a:1} 5 7]"},
		{nurDataMk + `[{a:1}] (m.f 7)`, "[[{a:1}] 5 7]"},
		{nurDataMk + `def k Integer end k (m.f 7)`, "[Integer 5 7]"},
		{nurDataMk + `def k {a:1} end k (m.f 7)`, "[{a:1} 5 7]"},
		{nurDataMk + `def k true end k (m.f 7)`, "[true 5 7]"},
		// The statement opens with a def of a literal (no `end`): the
		// island takes over after it.
		{nurDataMk + `def k 3 k (m.f 7)`, "[3 5 7]"},
		{nurDataMk + `def k 3 def j 4 k j (m.f 7)`, "[3 4 5 7]"},
		{nurFnMk + `def k 3 k (m.f 7)`, "[3 8]"},
		{nurFnMk + `{a:1} (m.f 7)`, "[{a:1} 8]"},
		{nurFnMk + `def k Integer end k (m.f 7)`, "[Integer 8]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336UnitIslandReadsTheReceiver: inside a fn body the statement's
// island reads the unit's names, the receiver of a reach among them — `q`
// in `(q.f 7)` was never bound for it, and the island raised "undefined
// word: q" where the interpreter placed the member.
func TestNUR336UnitIslandReadsTheReceiver(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[q:Map] [Any] [(q.f 7) drop]] end g (mk)`, "[5]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[q:Map] [Any] [(q.f 7) add]] end g (mk)`, "[12]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[q:Map] [Any] [def k 3 end k (q.f 7) drop drop]] end g (mk)`, "[3]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[] [Any] [def m (mk) end (m.f 7) drop]] end (g)`, "[5]"},
		{`def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def g fn [[q:Map] [Any] [def k 3 end k (q.f 7) add]] end g (mk)`, "[11]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[q:Map] [Any] [(q.f 7)]] end g (mk)`, "ERROR:expected 1 return value(s), got 2"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
