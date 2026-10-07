package lang

import "testing"

// TestComputedBodyRebindsTheTypeBareRead: a BARE read after a computed
// keep-defs body that rebinds the name to a value of another type. The read
// was seated live (the lookup reads the body's binding) but as a carrier of
// the pre-body type, so the consumer committed to that type's overload:
// `x add 1` ran Integer add over "s" and answered 1 for the interpreter's
// "s1". The seat is now gradual (compiler computedLeakGradual) and the
// consumer's dispatch re-matches at run time; the live read is still data
// to the frame replay (liveDataIDs), so a count error over two such reads
// stays the interpreter's type_error rather than a decline.
func TestComputedBodyRebindsTheTypeBareRead(t *testing.T) {
	const mk = `def x 0 end def mk fn [[][List][quote [def x "s"]]] end `
	const f = `def f fn [[b:List][Any][def t 0 do b drop `
	for _, c := range []struct{ src, want string }{
		// The witnesses: the root and the fn-body twin.
		{mk + `do (mk) end x add 1`, "[s1]"},
		{f + `t add 1]] end f (quote [def t "s" 1])`, "[s1]"},
		// Operand order, a paren, a multi-run body, no `end`, a read after.
		{f + `1 add t]] end f (quote [def t "s" 1])`, "[1s]"},
		{f + `(t add 1)]] end f (quote [def t "s" 1])`, "[s1]"},
		{`def f fn [[b:List][Any][def t 0 [1 2] each b drop t add 1]] end f (quote [def t "s"])`, "[1]"},
		{mk + `do (mk) x add 1`, "[s1]"},
		{mk + `do (mk) end x add 1 end x`, "[s1 s]"},
		{mk + `[1 2] each (mk) end x add 1`, "[[1 2] 1]"},
		{`def x 0 end def mk fn [[][List][quote [def x 2.5]]] end do (mk) end x add 1`, "[3.5]"},
		{f + `t mul 2]] end f (quote [def t 2.5 1])`, "[5.0]"},
		// The same type, and no rebinding: unchanged answers.
		{`def x 0 end def mk fn [[][List][quote [def x 5]]] end do (mk) end x add 1`, "[6]"},
		{f + `t add 1]] end f (quote [def t 5 1])`, "[6]"},
		{f + `t add 1]] end f (quote [1])`, "[1]"},
		// Other consumers of the rebound value.
		{mk + `do (mk) end size x`, "[1]"},
		{mk + `do (mk) end [x]`, "[['s']]"},
		{mk + `do (mk) end {a:x}`, "[{a:'s'}]"},
		{mk + `do (mk) end x typeof`, "[ProperString]"},
		{mk + `do (mk) end if (x eq "s") [1] [2]`, "[1]"},
		// Two reads left in a one-result frame: the interpreter's count
		// error over both, on both lanes (the replay counts no call).
		{f + `t t]] end f (quote [def t 5 1])`, "ERROR:got 2 — [5 5]"},
		{f + `t/v t]] end f (quote [def t 5 1])`, "ERROR:got 2 — [5 5]"},
		{f + `t t/v]] end f (quote [def t 5 1])`, "ERROR:got 2 — [5 5]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestComputedBodyRebindsTheTypeRejects is the negative half: a rebound
// value no overload takes raises the interpreter's own error on both lanes —
// the gradual operand's poly re-match finds no overload, and the record's
// published layout (PolyRef.Split, core optimisticLayout's gradual arm)
// raises the interpreter's signature_error where the run used to defer.
func TestComputedBodyRebindsTheTypeRejects(t *testing.T) {
	for _, src := range []string{
		`def f fn [[b:List][Any][def t 0 do b drop t lt 3]] end f (quote [def t "s" 1])`,
		`def f fn [[b:List][Integer][def t 0 do b drop t]] end f (quote [def t "abc" 1])`,
		`def f fn [[b:List][Any][def t 0 do b drop t add 1]] end f (quote [def t [1] 1])`,
		`def x 0 end def mk fn [[][List][quote [def x "s"]]] end do (mk) end x sub 1`,
	} {
		agreeOnBothLanes(t, src, "ERROR:")
	}
}

// TestUndefinedWordHintAfterComputedUndef: a live read's miss raises the
// interpreter's undefined_word with its did-you-mean line, each candidate
// once — the compiled frame's local `b` is also a registry binding (its
// dynamic-scope install), and was listed twice ("`b`, `b`, or `f`").
func TestUndefinedWordHintAfterComputedUndef(t *testing.T) {
	for _, src := range []string{
		`def f fn [[b:List][Any][def t 0 do b drop t]] end f (quote [undef t 1])`,
		`def f fn [[b:List][Any][def t 0 do b drop t/v]] end f (quote [undef t 1])`,
	} {
		agreeOnBothLanes(t, src, "ERROR:undefined word: t")
	}
}
