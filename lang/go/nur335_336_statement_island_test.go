package lang

import (
	"fmt"
	"testing"
)

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

// TestNUR335IslandSeatsTheStackAtTheStatement: the island seats the stack the
// pass stepped into the statement with (the `end` before it tells it), not the
// program residual's leading entries — a value the statement consumes is on
// the interpreter's stack there and on no residual: `m end drop (m.f 7)` ran
// the island's `drop` over an empty stack (a signature_error for `[5 7]`).
// A value the compiled stack holds beneath the statement must still be there
// at the stop, or the island is not taken.
func TestNUR335IslandSeatsTheStackAtTheStatement(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{nurDataMk + `m end drop (m.f 7)`, "[5 7]"},
		{nurDataMk + `m end m drop (m.f 7)`, "[{f:5} 5 7]"},
		{nurDataMk + `m end drop m (m.f 7)`, "[{f:5} 5 7]"},
		{nurDataMk + `def k 3 end k end drop (m.f 7)`, "[5 7]"},
		{nurDataMk + `def k 3 end k end k drop k (m.f 7)`, "[3 3 5 7]"},
		{nurDataMk + `def k 3 end def j k/v end j (m.f 7)`, "[3 5 7]"},
		{nurDataMk + `3 end drop (m.f 7)`, "[5 7]"},
		{nurDataMk + `{a:1} end drop (m.f 7)`, "[5 7]"},
		{nurDataMk + `{a:1} end {a:1} (m.f 7)`, "[{a:1} {a:1} 5 7]"},
		{nurDataMk + `[m] end m (m.f 7)`, "[[{f:5}] {f:5} 5 7]"},
		{nurDataMk + `def k 3 end k end 1 add (m.f 7)`, "[3 1 12]"},
		{nurDataMk + `(mk) end drop (m.f 7)`, "[5 7]"},
		{nurDataMk + `(mk) end 4 swap drop (m.f 7)`, "[4 5 7]"},
		{nurDataMk + `(mk) end (mk) end swap (m.f 7)`, "[{f:5} {f:5} 5 7]"},
		{nurFnMk + `m end drop (m.f 7)`, "[8]"},
		{nurFnMk + `m end m drop (m.f 7)`, "[{f:fn (Integer)} 8]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336PlacedParenApply: a paren apply whose lead is data where no
// island can be planned — a call written after the lead (`(m.f y)`) — is
// placed by the VM itself where nothing after it reads beneath what it
// leaves (the paren's own placement: the lead, then the values after it), and
// an apply there claims no result count, so a fn member taking fewer values
// than the paren holds leaves the rest as the paren does. A lead the landing
// before it parked is applied as the interpreter's paren applies it.
func TestNUR336PlacedParenApply(t *testing.T) {
	const y = `def y fn [[] [Integer] [42]] end `
	for _, c := range []struct{ src, want string }{
		{nurDataMk + y + `(m.f y)`, "[5 42]"},
		{nurDataMk + y + `(m.f y) 9`, "[5 42 9]"},
		{nurDataMk + y + `(m.f y 8)`, "[5 42 8]"},
		{nurDataMk + y + `(m.f 8 y)`, "[5 8 42]"},
		{nurDataMk + y + `3 end (m.f y)`, "[3 5 42]"},
		{nurDataMk + y + `(m.f y) (m.f y)`, "[5 42 5 42]"},
		{nurDataMk + y + `(m.f y) add 1`, "[5 43]"},
		{nurDataMk + y + `(m.f y) size`, "[5 42]"},
		{nurDataMk + y + `[(m.f y) 1]`, "[[5 42 1]]"},
		{nurDataMk + y + `if true [(m.f y)] [0]`, "[5 42]"},
		{nurDataMk + y + `def g fn [[] [Any] [(m.f y) drop]] end (g)`, "[5]"},
		{nurDataMk + `3 end (m.f "a") end`, "[3 5 a]"},
		{`def reg (flex {}) end reg set 'cb' 3 drop end ((reg.cb) 5)`, "[3 5]"},
		{nurFnMk + y + `(m.f y)`, "[43]"},
		{nurFnMk + y + `(m.f y) 9`, "[43 9]"},
		{nurFnMk + y + `(m.f y 8)`, "[43 8]"},
		{nurFnMk + y + `[(m.f y)]`, "[[43]]"},
		{nurFnMk + y + `(m.f y) (m.f y)`, "[43 43]"},
		{nurFnMk + y + `if true [(m.f y)] [0]`, "[43]"},
		{nurFnMk + y + `def g fn [[] [Any] [(m.f y)]] end (g)`, "[43]"},
		{nurFnMk + `(m.f/v 7)`, "[8]"},
		{`def mk fn [[] [Map] [{f: ([] => [9])}]] end def m (mk) end ` + y + `(m.f y)`, "[fn 42]"},
		{`def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end ` + y + `(m.f y)`, "[fn (String) 42]"},
		// A named fn member the landing fires before the paren: the paren's
		// lead is its result, which the island writes as it is.
		{`def h fn [[] [Integer] [42]] end def h fn [[n:Integer] [Integer] [n add 1]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end ` + y + `3 end (m.f y)`, "[3 42 42]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336IslandNotPlanned: statements whose island the root planner
// refuses (compiler planLandingRestarts). A statement opening with a bare
// pair (`a:1`), which the parser mints without a position, has no start the
// planner can place; a stack told at the statement's start that holds a type
// cannot be seated (seatStack). No island is planned for either: a fn member
// applies, a data member the VM places where nothing after the apply reads
// beneath it, and elsewhere a data member stays the loud defer it was —
// never a wrong answer.
func TestNUR336IslandNotPlanned(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{nurFnMk + `a:1 drop (m.f 7)`, "[8]"},
		{nurFnMk + `x:Integer drop (m.f 7)`, "[8]"},
		{nurFnMk + `a:1 (m.f 7)`, "[{a:1} 8]"},
		{nurDataMk + `a:1 drop (m.f 7)`, "[5 7]"},
		{nurFnMk + `Integer end (m.f 7)`, "[Integer 8]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, c := range []struct{ src, want string }{
		{nurDataMk + `a:1 (m.f 7)`, "[{a:1} 5 7]"},
		{nurDataMk + `Integer end (m.f 7)`, "[Integer 5 7]"},
	} {
		requireLoudDefer(t, c.src, "not an appliable function", c.want)
	}
}

// TestNUR336PlacingFnLeadDefers: a paren apply over a fn member that takes
// none of the values after it — a lambda of no argument, a signature the
// values miss — PLACES its lead and values, as the interpreter's paren does.
// Where the program reads beneath what it leaves (no placement claim) the
// apply takes the statement's island (eng callDynMethod's placedAsIs arm),
// whose write of the lead is a fn value the interpreter would dispatch where
// it lands: the designed NUR297 defer, loud.
func TestNUR336PlacingFnLeadDefers(t *testing.T) {
	const zero = `def mk fn [[] [Map] [{f: ([] => [9])}]] end def m (mk) end `
	const str = `def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{zero + `(m.f 7) drop`, "[fn]"},
		{zero + `def k 3 end k (m.f 7) drop`, "[3 fn]"},
		{str + `def k 3 end k (m.f 7) drop`, "[3 fn (String)]"},
	} {
		requireLoudDefer(t, c.src, "NUR297", c.want)
	}
}

// TestNUR336PlacementNeedsWrittenAfterPushes: a paren apply is a PLACING one
// (DynMethodSpec.Place) only when every push after it is a value the program
// WROTE after the paren. A folded shuffle that re-seats an operand written
// beneath the paren (`3 (m.f 7) swap` pushes the 3 after the apply) placed a
// data lead's values beneath it — [5 7 3] for the interpreter's [3 7 5] —
// a silent wrong answer the first NUR336 round introduced; the written-order
// pushes after a paren (`(m.f y) 9`) keep the placement.
func TestNUR336PlacementNeedsWrittenAfterPushes(t *testing.T) {
	const d = `def mk fn [[] [Map] [{f: 5}]] end def m (mk) end `
	const f = `def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def m (mk) end def y 42 end `
	for _, src := range []string{
		d + `3 (m.f 7) swap`,
		d + `def k 3 end k (m.f 7) swap`,
		d + `[1] (m.f 7) swap`,
		d + `3 end (m.f 7) swap`,
		f + `3 (m.f y) swap`,
		// the placement the rule keeps
		d + `def y 42 end (m.f y) 9`,
		d + `def y 42 end (m.f y 8)`,
		f + `(m.f y 8)`,
		f + `(m.f y) 9`,
		`def reg (flex {}) end reg set 'cb' 3 drop end ((reg.cb) 5)`,
	} {
		requireCompiledParity(t, src)
	}
	// Without the statement's `end` the island cannot be planned: loud,
	// never the placed wrong answer.
	src := `def mk fn [[] [Map] [{f: 5}]] def m (mk) 3 (m.f 7) swap`
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if compiled && errC == nil && fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: compiled %v silently, interpreter %v [%v]", src, gotC, gotI, errI)
	}
}
