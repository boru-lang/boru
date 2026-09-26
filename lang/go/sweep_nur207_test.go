package lang

import "testing"

// sweep_nur207_test.go pins NUR207 (2026-09-26) — a name DEF-BOUND to a fn
// value that arrives through a dynamic or gradual carrier was read as data —
// and the `if` × container cell it graduated. Every row is a both-lane
// parity pin that must run compiled; the recorder-level declines are pinned
// in compiler/go (nur207_gradual_fn_test.go) and check/go
// (fn_read_arrival_gradual_test.go), and the one program-level decline below
// is NUR207's own second witness.

// TestParkedMemberReadIsTheLambda pins the fold: a get / dot read of a
// PARKING member — an anonymous, capture-free, unapplied, only-0-arg fn,
// which every value landing parks as data — over a concrete container and
// key is the member value itself on the compiled lane (the map twin of the
// list read's static-index fold), so a NAME read of a def of it and `apply`
// dispatch it. The fold's escape fence declines every other consumer — a
// user fn call, a native that stores or shuffles it, a container literal, a
// code body's result — since a lambda literal handed there can come back as
// a gradual carrier read as data (NUR216–NUR218), and a program the member's
// 0-arg landing declined must never compile into that.
func TestParkedMemberReadIsTheLambda(t *testing.T) {
	const m = `def m {f: ([] => [42])} end `
	for _, src := range []string{
		m + `m.f`,
		m + `5 m.f`,
		m + `def j m.f end j`,
		m + `def j (m get "f") end j`,
		m + `def j (m.f) end j add 1`,
		m + `m.f apply`,
		m + `(m.f) apply`,
		m + `m.f/v apply`,
		m + `m.f typeof`,
		m + `typeof m.f`,
		m + `def k {a: m.f} end k.a`,
		m + `def g fn [[][Any][def j m.f j]] end g`,
		m + `if true [def j m.f j] [0]`,
		m + `def m {f: 5} end m.f`,
		`def m {f: {g: ([] => [42])}} end def j m.f.g end j`,
		`def m {f: (fn [[][Integer][42]])} end def j m.f end j`,
		// The members the fold leaves alone keep their own models.
		`def one fn [[][Integer][1]] end def m {f: one/v} end m.f`,
		`def m {f: ([n:Integer] => [n add 1])} end m.f 5`,
		`def m (flex {f: ([] => [42])}) end m.f`,
	} {
		requireEngineParity(t, src, true)
	}
	// The escape fence: the folded lambda handed to a store — read back out
	// of it, the interpreter's def dispatches the fn (42) where the compiled
	// lane would carry the flex member as data.
	fnValueM2CompileFailure(t, "folded member stored into a flex",
		`def m {f: ([] => [42])} end def s (flex {}) end s set f m.f end def j (s get "f") end j`,
		"reaches `set`")
}

// TestGradualFnCarrierNameReads pins NUR207's own witnesses and the claims
// that answer them: an `Any`-returning factory's result and a pinpointed
// arg-taking member read claim their shape at the def, so the read
// dispatches the name over its whole window; `apply` over a gradual lead
// is the pending apply (a fn applies, data raises apply's no-match).
func TestGradualFnCarrierNameReads(t *testing.T) {
	const mk0 = `def mk fn [[][Any][([] => [42])]] end `
	const mk1 = `def mk fn [[][Any][([n:Integer] => [n add 1])]] end `
	const ms = `def m {s: ([a:Integer b:Integer] => [a sub b])} end `
	for _, src := range []string{
		mk0 + `def j (mk) end j`,
		mk0 + `def j ((mk)) end j`,
		mk0 + `def j (mk) end j add 1`,
		mk0 + `def j (mk) end [j]`,
		mk0 + `def j (mk) end if true [j] [0]`,
		mk0 + `(mk) apply`,
		`def mk fn [[][Any][5]] end (mk) apply`,
		mk0 + `def g fn [[h:Function][Any][h]] end g (mk)`,
		mk1 + `def j (mk) end j 5`,
		mk1 + `def j (mk) end j 5 add 1`,
		mk1 + `5 (mk) apply`,
		`def mk fn [[][Any][5]] end def j (mk) end j`,
		ms + `def r (m.s) end r 10 3`,
		ms + `def r m.s end r 10 3 add 1`,
		ms + `def r m.s end 10 3 r`,
		ms + `def r m.s end 10 r 3`,
		`def m {s: 5} end def r (m.s) end r add 1`,
		// The def-bound branch whose arm is a fn that FIRES at the landing
		// stays data past it (NUR159's settled arm).
		`def one fn [[][Integer][1]] end def g (if true one/v [2]) end g`,
	} {
		requireEngineParity(t, src, true)
	}
	// NUR207's second witness: a written argument the member does not take
	// is the interpreter's `cannot call r`, which the flattened dynamic
	// apply parked (`[fn (Integer, Integer) x 3]`) — the read declines.
	fnValueM2CompileFailure(t, "def-bound member lambda, unfit window",
		ms+`def r (m.s) end r 'x' 3`, "does not fit the wrapper's parameter")
}
