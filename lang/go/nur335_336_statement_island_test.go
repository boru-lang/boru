package lang

import (
	"bytes"
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

// TestNUR336IslandPastAPositionlessPairAndTypes: a statement opening with a
// bare pair (`a:1`), which the parser mints without a position, starts just
// past the `end` before it (compiler statementStart), and a stack told at a
// statement's start that holds a type seats the canonical node by its ID
// (RestartType) — both used to plan no island, and the data member's apply
// failed inside the compiled runtime.
func TestNUR336IslandPastAPositionlessPairAndTypes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{nurFnMk + `a:1 drop (m.f 7)`, "[8]"},
		{nurFnMk + `x:Integer drop (m.f 7)`, "[8]"},
		{nurFnMk + `a:1 (m.f 7)`, "[{a:1} 8]"},
		{nurDataMk + `a:1 drop (m.f 7)`, "[5 7]"},
		{nurFnMk + `Integer end (m.f 7)`, "[Integer 8]"},
		{nurDataMk + `a:1 (m.f 7)`, "[{a:1} 5 7]"},
		{nurDataMk + `a:1 (m.f 7) drop`, "[{a:1} 5]"},
		{nurDataMk + `a:1 b:2 (m.f 7)`, "[{a:1} {b:2} 5 7]"},
		{nurDataMk + `x:Integer (m.f 7)`, "[{x:Integer} 5 7]"},
		{nurDataMk + `3 end a:1 (m.f 7)`, "[3 {a:1} 5 7]"},
		{nurDataMk + `Integer end (m.f 7)`, "[Integer 5 7]"},
		{nurDataMk + `Integer end String end (m.f 7)`, "[Integer String 5 7]"},
		{nurDataMk + `Integer end (m.f 7) swap`, "[Integer 7 5]"},
		{nurDataMk + `Integer end drop (m.f 7)`, "[5 7]"},
		{nurDataMk + `def T Integer end T end (m.f 7)`, "[Integer 5 7]"},
		{nurDataMk + `def Foo (refine Integer) end Foo end (m.f 7)`, "[Foo 5 7]"},
		{nurDataMk + `[Integer] end (m.f 7)`, "[[Integer] 5 7]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336LeadTakingNoneRunsItsIsland: a paren apply over a fn member no
// signature of which takes the paren's values — a lambda of none, one of a
// String over an Integer, one of two Integers over one — is not applied over
// them by the interpreter's paren: it places a lambda that takes none, and
// re-steps any other where it reaches beneath the paren (`3 (m.f 7)` over
// two Integers is 4). The VM tells so from the value before it runs it
// (parenMissesWindow) and takes the statement's island, which writes the
// lead as the reach group its member read lowers to (RestartSubst.Reach):
// the lead never ran, so nothing runs twice (the Codex review of #520).
func TestNUR336LeadTakingNoneRunsItsIsland(t *testing.T) {
	const zero = `def mk fn [[] [Map] [{f: ([] => [9])}]] end def m (mk) end `
	const str = `def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end `
	const two = `def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def m (mk) end `
	const named = `def h fn [[] [Integer] [42]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{zero + `(m.f 7) drop`, "[fn]"},
		{zero + `def k 3 end k (m.f 7) drop`, "[3 fn]"},
		{str + `def k 3 end k (m.f 7) drop`, "[3 fn (String)]"},
		{zero + `3 end (m.f 7)`, "[3 fn 7]"},
		{zero + `3 end (m.f 7) swap`, "[3 7 fn]"},
		{zero + `3 end (m.f 7 8) drop`, "[3 fn 7]"},
		{zero + `for 2 [(m.f 7) drop]`, "[fn fn]"},
		{zero + `[(m.f 7) 8]`, "[[fn 7 8]]"},
		{zero + `if true [(m.f 7)] [0]`, "[fn 7]"},
		{zero + `def g fn [[] [Any] [def k 3 end k (m.f 7) drop drop]] end (g)`, "[3]"},
		{str + `"s" end (m.f 7)`, "[s 7]"},
		{str + `"s" (m.f 7)`, "[s 7]"},
		{str + `"s" end (m.f 7) drop`, "[s]"},
		{str + `def g fn [[] [Any] [def k 3 end k (m.f 7) drop drop]] end (g)`, "[3]"},
		{two + `3 (m.f 7)`, "[4]"},
		{two + `3 end (m.f 7)`, "[4]"},
		{two + `3 end (m.f 7) drop`, "[]"},
		{two + `(m.f 7) 9`, "[-2]"},
		{named + `3 end (m.f 7)`, "[3 42 7]"},
		{named + `3 end (m.f 7) drop`, "[3 42]"},
		{named + `3 end (m.f 7) add`, "[3 49]"},
		{`def h fn [[x:String] [String] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end "s" end (m.f 7)`, "ERROR:matched no signature"},
		// A lead a signature of which takes the values applies as before.
		{nurFnMk + `3 end (m.f 7) add`, "[11]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336PlacedLeadNeverReachesPastItsWindow: a placing apply
// (DynMethodSpec.Place) ran a lead that takes none of the paren's values over
// them alone and took whatever it left, where the interpreter's paren
// re-steps it over the stack beneath — `("s" dup drop) end (m.f y)` over a
// lambda of one String took the "s" (`[s 42]`), and the placed run answered
// `[s fn (String) 42]`, a silent wrong answer on main (found on the way,
// NUR336). The run is the paren's only where nothing stands beneath the
// window and nothing follows (placesAlone); elsewhere such a lead used to
// defer, unrun, loud — its statement's island now writes the paren as the
// survivors its close re-steps (survivorIsland), the interpreter's answer.
func TestNUR336PlacedLeadNeverReachesPastItsWindow(t *testing.T) {
	const str = `def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end def y fn [[] [Integer] [42]] end `
	const two = `def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def m (mk) end def y fn [[] [Integer] [42]] end `
	agreeOnBothLanes(t, str+`("s" dup drop) end (m.f y)`, "[s 42]")
	agreeOnBothLanes(t, two+`(3 dup drop) end (m.f y)`, "[39]")
	// Alone, the run is the paren's.
	agreeOnBothLanes(t, str+`(m.f y)`, "[fn (String) 42]")
	agreeOnBothLanes(t, two+`(m.f y)`, "[fn (Integer, Integer) 42]")
}

// TestNUR336DefBoundWordLead: a paren whose lead is the word of a def-bound
// lambda (`def g m.f/v end … (g 7)`) is the interpreter's dispatch of the
// NAME — which always calls: a lambda of none runs before the 7, and one of a
// String raises the word's no-match — where the compiled apply held only the
// value, whose position is the def's: the statement island was planned in
// the def's statement, or not at all, and the placing apply stepped the value
// as a lambda, which stays data (`[fn g 7]` for `[9 7]`, a silent wrong
// answer on main). The recorder finds the lead's own token by the paren's
// argument (compiler parenLeadToken, DynMethodSpec.LeadName); the island
// writes the value as the name's call runs it (RestartSubst.Named), and a
// miss raises the name's no-match on a fork holding the name as the
// interpreter's def binds it (namedMissRaise).
func TestNUR336DefBoundWordLead(t *testing.T) {
	const zero = `def mk fn [[] [Map] [{f: ([] => [9])}]] end def m (mk) end `
	const str = `def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{zero + `def g m.f/v end def k 3 end k (g 7) drop`, "[3 9]"},
		{zero + `def g m.f/v end (g 7)`, "[9 7]"},
		{zero + `def g m.f/v end def k 3 end k (g 7 8) drop drop`, "[3 9]"},
		{str + `def g (m.f/v) end def k 3 end k (g 7)`, "ERROR:cannot call `g`"},
		{str + `def g m.f/v end (g 7)`, "ERROR:cannot call `g`"},
		{str + `def g m.f/v end "s" end (g 7)`, "ERROR:cannot call `g`"},
		{nurFnMk + `def g (m.f/v) end def k 3 end k (g 7)`, "[3 8]"},
		{nurFnMk + `def g m.f/v end def k 3 end k (g 7) add`, "[11]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// An island that dispatches the word again after the stop met the name
	// bound by the compiled bind, which the interpreter's word path cannot
	// run (its def compiles the lambda's signatures; the bind keeps the
	// value): "no runnable implementation for `g`", loud. The island now
	// runs over the name installed as the interpreter's def installs it
	// (bindRootRead, as a root deopt island does) — NUR336's remainder.
	for _, c := range []struct{ src, want string }{
		{zero + `def g m.f/v end def k 3 end k (g 7) (g 8) drop`, "[3 9 7 9]"},
		{zero + `def g m.f/v end (g 7) (g 8)`, "[9 7 9 8]"},
		{zero + `def g m.f/v end def k 3 end k (g 7) g`, "[3 9 7 9]"},
		{zero + `def g m.f/v end def k 3 end k (g 7) [g]`, "[3 9 7 [9]]"},
		{zero + `def g m.f/v end def k 3 end k (g 7) (g 8) drop typeof`, "[3 9 7 Integer]"},
		// The negative half: the island's own reads of the name as a value,
		// and its own rebinding of it, are the interpreter's.
		{zero + `def g m.f/v end def k 3 end k (g 7) g/v`, "[3 9 7 fn g]"},
		{zero + `def g m.f/v end def k 3 end k (g 7) def g 4 g`, "[3 9 7 4]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336ComputedDefOpener: a statement opening with a def of a COMPUTED
// value (`def k (3 dup) k (m.f 7)`) planned no island — the island may not
// run the def's paren again, and the stack after it is not the one the `end`
// before the statement told. The engine tells the stack it steps the token
// after a completed def over (Engine.noteDefStack), and the island takes the
// statement over from the last such token before its stop (compiler
// toldAfter), the def's leftovers seated as told. A fn body has no told
// stack: its island takes over past the defs its statement opens with, each
// of a value the def took whole (defsBefore), over the frame the compiled
// code holds there.
func TestNUR336ComputedDefOpener(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{nurDataMk + `def k (3 dup) k (m.f 7)`, "[3 3 5 7]"},
		{nurDataMk + `def k (3 dup) end k (m.f 7)`, "[3 3 5 7]"},
		{nurDataMk + `def k (3 dup) def j (4 dup) k j (m.f 7)`, "[3 4 3 4 5 7]"},
		{nurDataMk + `3 dup def k (4) k (m.f 7)`, "[3 3 4 5 7]"},
		{nurDataMk + `def k (3 dup) k (m.f 7) add`, "[3 3 12]"},
		{nurDataMk + `def k (3 dup) k (m.f 7) swap`, "[3 3 7 5]"},
		{nurFnMk + `def k (3 dup) k (m.f 7)`, "[3 3 8]"},
		{nurFnMk + `def k (3 dup) k (m.f 7) add`, "[3 11]"},
		{`def mk fn [[] [Map] [{f: ([] => [9])}]] end def m (mk) end def k (3 dup) k (m.f 7) drop`, "[3 3 fn]"},
		{`def k 3 def mk fn [[] [Map] [{f: 5}]] def m (mk) k/v (m.f 7) add`, "[3 12]"},
		{`def k 3 def mk fn [[] [Map] [{f: 5}]] def m (mk) k (m.f 7) add`, "[3 12]"},
		{`def mk fn [[] [Map] [{f: 5}]] def m (mk) 3 (m.f 7) add`, "[3 12]"},
		{`def mk fn [[] [Map] [{f: 5}]] def m (mk) def k (m.f 7) k`, "[7 5]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[][Any][def m (mk) def k 3 k (m.f 7)]] end (g)`, "ERROR:expected 1 return value(s), got 3"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[][Any][def m (mk) def k 3 k (m.f 7) drop drop]] end (g)`, "[3]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[][Any][def m (mk) k (m.f 7)]] end def k 3 end (g)`, "ERROR:expected 1 return value(s), got 3"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336ComputedDefOpenerParen pins NUR336's first remainder: the paren
// right after a computed def. The def's operand group expands the paren
// after it to its markers before the engine steps it, and a positionless
// open paren's statement stack went untold — the island was never planned,
// and a data member's apply deferred, loud. The engine keys the note by the
// paren's first inner token (statementStackPos) and the recorder reads it
// for the paren's own token (toldAt). In a fn body the def's call leaves the
// value it did not bind in a promoted slot, or on the frame beside the one it
// did: the island seats the leftovers where the compiled code holds them
// (defLeftoverSrcs), where it used to seat the frame as it stood — `drop
// drop` answered "expected 1 return value(s), got 0" for the interpreter's
// [3], a wrong error on main — and the pass counted a leftover produced
// before the statement as consumed with its def-bound sibling
// (deoptDeferred's first-result rule).
func TestNUR336ComputedDefOpenerParen(t *testing.T) {
	const zero = `def mk fn [[] [Map] [{f: ([] => [9])}]] end def m (mk) end `
	const str = `def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end `
	const two = `def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{nurDataMk + `def k (3 dup) (m.f 7)`, "[3 5 7]"},
		{nurDataMk + `def k (3 dup) (m.f 7) add`, "[3 12]"},
		{nurDataMk + `def k (3 dup) (m.f 7) swap`, "[3 7 5]"},
		{nurDataMk + `def k (3 dup) (m.f 7) k`, "[3 5 7 3]"},
		{nurDataMk + `def k (3 dup) (m.f 7) (m.f 8)`, "[3 5 7 5 8]"},
		{nurDataMk + `def k (3 dup) def j (4 dup) (m.f 7)`, "[3 4 5 7]"},
		{nurDataMk + `3 dup def k (4) (m.f 7)`, "[3 3 5 7]"},
		{nurDataMk + `def k (3 dup) ((m.f 7))`, "[3 5 7]"},
		{nurDataMk + `def k (3 dup) (m.f 7 8)`, "[3 5 7 8]"},
		{nurDataMk + `def k (3 dup) [(m.f 7)]`, "[3 [5 7]]"},
		{nurDataMk + `def k (3 dup) (1 add 2) (m.f 7)`, "[3 3 5 7]"},
		{`def mk fn [[] [Map] [{f: 5}]] def m (mk) def k (3 dup) (m.f 7)`, "[3 5 7]"},
		{nurFnMk + `def k (3 dup) (m.f 7)`, "[3 8]"},
		{nurFnMk + `def k (3 dup) (m.f 7) add`, "[11]"},
		{zero + `def k (3 dup) (m.f 7) drop`, "[3 fn]"},
		{str + `def k (3 dup) (m.f 7)`, "[3 fn (String) 7]"},
		{two + `def k (3 dup) (m.f 7)`, "[4]"},
		// The fn-body twin.
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[][Any][def m (mk) def k (3 dup) (m.f 7) drop drop]] end (g)`, "[3]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[][Any][def m (mk) def k (3 dup) (m.f 7)]] end (g)`, "ERROR:expected 1 return value(s), got 3"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[q:Map][Any][def k (3 dup) (q.f 7) drop drop]] end g (mk)`, "[3]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[q:Map Integer][Any][def k (3 dup) (q.f 7) drop drop add]] end g (mk) 1`, "[4]"},
		{`def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def g fn [[q:Map][Any][def k (3 dup) (q.f 7) drop]] end g (mk)`, "[3]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def g fn [[q:Map][Any][def k (3 dup) 4 (q.f 7) drop drop drop]] end g (mk)`, "[3]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A value an EARLIER statement left is no def's leftover, but where the
	// compiled frame holds it beneath the statement, intact at the stop, the
	// island seats it with the frame (deoptPoint.onFrame, NUR336 — pinned in
	// TestNUR336UnitIslandOverAnEarlierStatement).
	// A user fn's multi-result call bound by a def: the def takes its first
	// result, which the island reads by name, so the call's results go to
	// consecutive slots and the bind re-pushes the first
	// (planValueDefLocals' dyn-bind source arm, deoptDefsBindable, NUR336);
	// its second is the frame's leftover, seated from its slot.
	const twoRes = `def mk fn [[] [Map] [{f: 5}]] end def two fn [[][Integer Integer][1 2]] end `
	for _, c := range []struct{ src, want string }{
		{twoRes + `def g fn [[q:Map][Any][def k (two) (q.f 7) drop drop add k]] end g (mk)`, "[3]"}, // the register's witness
		{twoRes + `def g fn [[q:Map][Any][def k (two) (q.f 7)]] end g (mk)`, "ERROR:expected 1 return value(s), got 3"},
		{twoRes + `def g fn [[q:Map][Any][def k (two) k add]] end g (mk)`, "[3]"},
		{twoRes + `def g fn [[q:Map][Any][def k (two) print k (q.f 7) drop drop add k]] end g (mk)`, "[3]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def three fn [[][Integer Integer Integer][1 2 3]] end def g fn [[q:Map][Any][def k (three) (q.f 7) drop drop add k add]] end g (mk)`, "[6]"},
		{`def two fn [[][Integer Integer][1 2]] end def g fn [[b:List][Any][def k (two) do b drop k add]] end g (quote [def k 10 1])`, "[12]"},
		{`def two fn [[][Integer Integer][1 2]] end def g fn [[][Any][def k (two) k add]] end (g)`, "[3]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336RematchTakesItsIsland: the pass holds a paren apply's result as
// any value, and a nested forward call over it (`k (m.f 7) add add`) fails
// the pass's match — the compiled program ends in a runtime rematch trap,
// which the run matches (DISPATCH_REMATCH's internal_error). Where the trap's
// statement has an island (compiler planRematchRestart) the interpreter runs
// the statement and the program after it, the apply written as the value it
// left.
func TestNUR336RematchTakesItsIsland(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{nurFnMk + `def y 42 end 3 end def k 1 k (m.f 7) add add`, "[12]"},
		{nurFnMk + `3 end def k 1 end k (m.f 7) add add`, "[12]"},
		{nurFnMk + `3 end 1 (m.f 7) add add`, "[12]"},
		{nurFnMk + `3 1 (m.f 7) add add`, "[12]"},
		{nurFnMk + `3 1 (m.f 7) add add 1`, "[3 10]"},
		{nurFnMk + `3 1 (m.f 7) add add end 5`, "[12 5]"},
		{nurFnMk + `1 (m.f 7) add add`, "ERROR:cannot call `add`"},
		{nurDataMk + `3 1 (m.f 7) add add`, "[3 13]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336RematchWithoutIslandDefers: a rematch trap whose statement no
// island can take over — a loop ran in it before the trap, which no island
// may run again (restartSubsts) — keeps the designed defer where the run
// matches: loud, never the truncated compiled run.
func TestNUR336RematchWithoutIslandDefers(t *testing.T) {
	requireLoudDefer(t, nurFnMk+`for 2 [0 drop] 3 1 (m.f 7) add add`, "matched at run time", "[12]")
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

// TestNUR336LandingLeadSurvivors pins NUR336's second remainder: a paren
// apply whose lead went through a landing — `(m.f y) 9` over a lambda of two
// Integers, the landing's walk parking it where the word y stopped it — and
// no signature of which takes the paren's values. The apply found a value
// the landing may have run, so the statement's island (which writes the lead
// as the value found, where the interpreter parks it) could not take it, and
// the apply ran it over its values alone: "result count 2 violates the
// host-registered shape claim 1", loud. The paren has run everything it
// holds by then, and the interpreter's close lands its rewind on the lead of
// more than one survivor and steps each: the island writes the paren as
// those survivors (survivorIsland) — nothing the paren ran runs again, its
// values written as they are.
func TestNUR336LandingLeadSurvivors(t *testing.T) {
	const y = `def y fn [[] [Integer] [42]] end `
	const two = `def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def m (mk) end `
	const zero = `def mk fn [[] [Map] [{f: ([] => [5])}]] end def m (mk) end `
	const str = `def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{two + y + `(m.f y) 9`, "[33]"},
		{two + y + `3 (m.f y) 9`, "[3 33]"},
		{two + y + `3 end (m.f y) 9`, "[3 33]"},
		{two + y + `def k 3 end k (m.f y) 9`, "[3 33]"},
		{two + y + `3 end (m.f y)`, "[39]"},
		{two + y + `(m.f y) 9 1`, "[33 1]"},
		{two + y + `(m.f y) 9 add 1`, "[34]"},
		{two + y + `[(m.f y) 9]`, "[[33]]"},
		{two + y + `def g fn [[] [Any] [(m.f y) 9]] end (g)`, "[33]"},
		{two + `def y 42 end (m.f y) 9`, "[33]"},
		{two + `def y 42 end 3 end (m.f y) 9`, "[3 33]"},
		{zero + y + `(m.f y) 9`, "[fn 42 9]"},
		{str + y + `(m.f y) 9`, "[fn (String) 42 9]"},
		{str + y + `"s" end (m.f y) 9`, "[s 42 9]"},
		// A lead that takes the paren's values applies as before.
		{`def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def m (mk) end ` + y + `(m.f y) 9`, "[43 9]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// Effects run once: the lead's own and the paren's call alike.
	for _, src := range []string{
		`def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [print x x sub y])}]] end def m (mk) end ` + y + `(m.f y) 9`,
		two + `def y fn [[] [Integer] [print "y" 42]] end (m.f y) 9`,
	} {
		var outC, outI bytes.Buffer
		bc, bi := mustNew(t), mustNew(t)
		bc.SetOutput(&outC)
		bi.SetOutput(&outI)
		gotC, compiled, errC := bc.RunCompiled(src)
		gotI, errI := bi.RunInterp(src)
		if !compiled || fmt.Sprint(gotC, errC) != fmt.Sprint(gotI, errI) || outC.String() != outI.String() || outI.Len() == 0 {
			t.Errorf("%s: compiled %v / %v %q, interpreter %v / %v %q", src, gotC, errC, outC.String(), gotI, errI, outI.String())
		}
	}
	// A loop's statement takes its per-iteration continuation (loopContPlan,
	// TestNUR336LoopContinuation), and a `/v` lead its island (leadRun,
	// TestNUR336SlashVLead).
	agreeOnBothLanes(t, two+y+`for 2 [(m.f y) 9 drop]`, "[]")
	agreeOnBothLanes(t, two+y+`(m.f/v y) 9`, "[33]")
}
