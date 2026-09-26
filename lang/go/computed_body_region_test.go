package lang

import (
	"strings"
	"testing"
)

// computed_body_region_test.go pins NUR210 and NUR211's close (2026-09-26).
//
// NUR210 — a COMPUTED `do` body (the dyn-body backstop) is a variadic
// REGION: its run leaves 0-or-more values where the check pass models one
// dynamic(Any) out. Recorded as one (compiler recordDynBodyCall), a value
// beneath it declines instead of seating after the run (`9 do (mk)` over
// `[1 2]` was `[1 9 2]` for the interpreter's `[9 1 2]`, silent), a
// fixed-count consumer declines, and a run that may leave a callable seats
// only last (the interpreter re-steps a fn value over what follows it). A
// keep-defs word (`do`, `each`, …) over a computed body arms the kept-defs
// latch (compiler kept_defs.go): the body may define or undefine any name,
// so the first later observer of a binding declines — unless the body's
// tokens are proven (a factory returning a const quoted list) and bind
// nothing.
//
// NUR211 — a stack-form count over a computed `for` body (`3 for (mk)`): the
// forward `(mk)` fills for's count slot and the interpreter raises
// signature_error; the runtime rematch's own flexible match would accept the
// window, so the record declines (core rematchWindowMatches) and the program
// fails to compile loudly instead of raising an internal error at run time.

// requireLoudDeclineErr is requireLoudDecline for a program the interpreter
// answers with an error: the compile declines with the reason, the compiled
// lane fails as compile_failed, and the interpreter raises wantCode.
func requireLoudDeclineErr(t *testing.T, src, wantReason, wantCode string) {
	t.Helper()
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog != nil || err != nil {
		t.Errorf("%q: want a compile decline, got prog=%v err=%v", src, prog != nil, err)
		return
	}
	if !strings.Contains(reason, wantReason) {
		t.Errorf("%q: decline reason %q, want substring %q", src, reason, wantReason)
	}
	if _, _, errC := mustNew(t).RunCompiled(src); codeOf(errC) != "compile_failed" {
		t.Errorf("%q: the compiled lane must fail loudly as compile_failed, got %v", src, errC)
	}
	if _, errI := mustNew(t).RunInterp(src); codeOf(errI) != wantCode {
		t.Errorf("%q: interpreter error %v, want code %s", src, errI, wantCode)
	}
}

func TestComputedDoBodyRegionDeclines(t *testing.T) {
	const mk12 = `def mk fn [[][List][quote [1 2]]] end `
	const above = "call result above a literal"
	const region = "consumes loop results"
	const notLast = "seat only as the residual's last entries"
	const kept = "a computed body keeps its defs and undefs in the enclosing scope"
	for _, c := range []struct{ src, reason, want string }{
		// NUR210's first witness and its neighbours: a value beneath the run.
		{mk12 + `9 do (mk)`, above, "[9 1 2]"},
		{mk12 + `def y 9 end y do (mk)`, above, "[9 1 2]"},
		{mk12 + `9 do (mk) end 8`, above, "[9 1 2 8]"},
		{`def mk fn [[][List][quote []]] end 9 do (mk)`, above, "[9]"},
		{`def mk fn [[][List][quote [5]]] end 9 do (mk)`, above, "[9 5]"},
		{mk12 + `9 8 do (mk)`, "variadic region promoted to a frame slot", "[9 8 1 2]"},
		// A fixed-count consumer of the run.
		{mk12 + `[9 do (mk)]`, region, "[[9 1 2]]"},
		{mk12 + `9 do (mk) drop`, region, "[9 1]"},
		{mk12 + `do (mk) add 9`, region, "[1 11]"},
		// A run that may leave a callable, with values after it.
		{`def g fn [[n:Integer][Integer][n add 1]] end def mk fn [[][List][quote [g/v]]] end do (mk) 5`, notLast, "[6]"},
		{`def g fn [[n:Integer][Integer][n add 1]] end def mk fn [[][List][quote [g/v]]] end 9 do (mk)`, above, "[10]"},
		// NUR210's second witness: a rebinding the run leaks, read after it.
		{`def x 99 end def mk fn [[][List][quote [def x 5]]] end do (mk) end x`, "(NUR210)", "[5]"},
		{`def h fn [[][Integer][1]] end def mk fn [[][List][quote [def h fn [[][Integer][7]] end]]] end do (mk) end h`, "(NUR210)", "[7]"},
		{`def x 99 end def mk fn [[][List][quote [def x 5 1]]] end [1 2] each (mk) end x`, kept + ", and the read of `x`", "[[1 1] 5]"},
		{`def x 99 end def mk fn [[][List][quote [def x 5 1]]] end def b (mk) end [1 2] each b end x`, kept + ", and the read of `x`", "[[1 1] 5]"},
		{`def x 99 end def f fn [[b:List][Integer][do b x]] end f (quote [def x 5])`, "(NUR210)", "[5]"},
		{`def f fn [[b:List n:Integer][Integer][do b n]] end f (quote [def n 5]) 1`, "(NUR210)", "[5]"},
		{`def x 99 end def mk fn [[][List][quote [def x 5]]] end def g fn [[][Integer][x]] end do (mk) end g`, "(NUR210)", "[5]"},
		// A run whose tokens are not plain data may leave a callable, so
		// even a literal after it declines (the interpreter would re-step
		// one over it).
		{`def x 99 end def mk fn [[][List][quote [def x 5]]] end do (mk) end 7`, notLast, "[7]"},
	} {
		requireLoudDecline(t, c.src, c.reason, c.want)
	}
	// The undef twins: the interpreter raises where the model still binds.
	for _, src := range []string{
		`def x 99 end def mk fn [[][List][quote [undef x]]] end do (mk) end x`,
		`def x 99 end def f fn [[b:List][][do b]] end f (quote [undef x]) end x`,
	} {
		requireLoudDeclineErr(t, src, "(NUR210)", "undefined_word")
	}
	// An empty run under a consumer: the interpreter's own no-match.
	requireLoudDeclineErr(t, `def mk fn [[][List][quote []]] end do (mk) add 9`, region, "signature_error")
}

func TestComputedDoBodyRegionCompiles(t *testing.T) {
	for _, src := range []string{
		// The run alone, or last, or with a proven plain-data body under
		// the values after it.
		`def mk fn [[][List][quote [1 2]]] end do (mk)`,
		`def mk fn [[][List][quote []]] end do (mk)`,
		`def mk fn [[][List][quote [1 2]]] end (1 add 8) do (mk)`,
		`def mk fn [[][List][quote [1 2]]] end do (mk) 9`,
		`def f fn [[b:List][][do b]] end 9 f (quote [1 2])`,
		`def f fn [[b:List][Any][do b]] end f (quote [1 add 2])`,
		// Nothing read after a keep-defs run that binds.
		`def x 99 end def mk fn [[][List][quote [def x 5]]] end do (mk)`,
		`def x 99 end def mk fn [[][List][quote [def x 5 1]]] end [1 2] each (mk)`,
		// A proven body that binds nothing leaves the later reads compiled.
		`def x 99 end def mk fn [[][List][quote [1 2]]] end do (mk) end x`,
		`def x 99 end def mk fn [[][List][quote [add 1]]] end [1 2] each (mk) end x`,
		`def x 99 end def mk fn [[][List][quote [add 1]]] end def b (mk) end [1 2] each b end x`,
		`def inc fn [[n:Integer][Integer][n add 1]] end def mk fn [[][List][quote [inc]]] end each (mk) [1] each (mk) [2]`,
		// A proven fn callback keeps nothing.
		`def acc (flex []) end def m {f: ([e:Integer] => [acc push e])} end for-each m.f [1 2 3] end size acc`,
	} {
		requireEngineParity(t, src, true)
	}
}

func TestStackCountComputedForBodyDeclines(t *testing.T) {
	for _, src := range []string{
		`def mk fn [[][List][quote [i]]] end 3 for (mk)`,
		`def mk fn [[][List][quote [i]]] end (1 add 2) for (mk)`,
	} {
		requireLoudDeclineErr(t, src, "unmatched dispatch recovered at for", "signature_error")
	}
	// The forward forms keep compiling.
	requireEngineParity(t, `for 3 [i]`, true)
	requireEngineParity(t, `def mk fn [[][List][quote [i]]] end for 3 (mk)`, true)
}
