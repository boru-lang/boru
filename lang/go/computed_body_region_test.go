package lang

import (
	"fmt"
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
// signature_error. Main's #514 declined the record (core
// rematchWindowMatches); at the merge the branch's split-aware rematch
// replaced that decline — the runtime rematch plans the window with the
// interpreter's forward / stack split and raises the same error.

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
	for _, c := range []struct{ src, reason, want string }{
		// NUR210's first witness and its neighbours — a value beneath the
		// run, a list literal collecting it — compile through the branch's
		// prefix island (TestComputedDoBodyIslandCompiles); a value above
		// the run still declines.
		{mk12 + `9 do (mk) end 8`, above, "[9 1 2 8]"},
		// A fixed-count consumer of the run.
		{mk12 + `9 do (mk) drop`, region, "[9 1]"},
		{mk12 + `do (mk) add 9`, region, "[1 11]"},
		// A fn the run may rebind, called after it: a call is no live read
		// (NUR210's second witness and its value-read neighbours compile —
		// TestKeptDefsLiveReadsCompile).
		{`def h fn [[][Integer][1]] end def mk fn [[][List][quote [def h fn [[][Integer][7]] end]]] end do (mk) end h`, "(NUR210)", "[7]"},
		// Inside a fn body the unit's residual declines first: the run may
		// leave a fn value the unit would return unapplied.
		{`def x 99 end def f fn [[b:List][Integer][do b x]] end f (quote [def x 5])`, "unapplied fn-value in body residual", "[5]"},
		{`def f fn [[b:List n:Integer][Integer][do b n]] end f (quote [def n 5]) 1`, "unapplied fn-value in body residual", "[5]"},
		{`def x 99 end def mk fn [[][List][quote [def x 5]]] end def g fn [[][Integer][x]] end do (mk) end g`, "(NUR210)", "[5]"},
	} {
		requireLoudDecline(t, c.src, c.reason, c.want)
	}
	// The unit form: the latch re-arms at the call, where the root's value
	// bindings are generalised (NUR281), so the read after it has no
	// compiled home and the residual's provenance decline is met first.
	requireLoudDeclineErr(t, `def x 99 end def f fn [[b:List][][do b]] end f (quote [undef x]) end x`, "residual value of unknown provenance", "undefined_word")
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
		// A run PROVEN to be one plain value is no region: it seats as the
		// one value the check pass models, under a value or a consumer.
		`def mk fn [[][List][quote [5]]] end 9 do (mk)`,
		`def mk fn [[][List][quote [5]]] end [9 do (mk)]`,
		`def mk fn [[][List][quote [5]]] end (do (mk)) add 1`,
		`def mk fn [[][List][quote [5]]] end def ok (do (mk)) ok`,
		`def mk fn [[][List][quote [5]]] end 9 do (mk) drop`,
	} {
		requireEngineParity(t, src, true)
	}
}

// TestComputedDoBodyGradualRegionCompiles pins NUR282's wrong-count seat for
// a GRADUAL body: `do` over a declared-Any result records a poly re-match
// (either overload, List or Map, is the run's), and the run is a variadic
// region like the List-typed one's — so the op commits no result-count
// claim (PolyNOutRegion) and the region's seat rules own the count. It
// deferred on every run but a single value (`poly dispatch do: result count
// 2 differs from the recorded claim 1`). A fixed seat still declines, and a
// value the run matches no overload for is the interpreter's no-match.
func TestComputedDoBodyGradualRegionCompiles(t *testing.T) {
	mk := func(v string) string { return `def mk fn [[][Any][` + v + `]] end ` }
	for _, src := range []string{
		mk(`[1 2]`) + `do (mk)`,
		mk(`[]`) + `do (mk)`,
		mk(`[1 2 3]`) + `do (mk)`,
		mk(`{a:1}`) + `do (mk)`,
		mk(`[1 2]`) + `9 do (mk)`,
		mk(`[1 2]`) + `[do (mk)]`,
		mk(`[5]`) + `(do (mk)) add 1`,
		mk(`[1 2]`) + `def f fn [[][Any][do (mk)]] end f`,
		mk(`42`) + `do (mk)`,
	} {
		requireEngineParity(t, src, true)
	}
	requireLoudDeclineErr(t, mk(`[1 2]`)+`(do (mk)) add 1`, "consumes loop results", "")
}

// TestComputedDoBodyIslandCompiles: a run with inert values beneath it, or
// collected by a list literal at the program level, is the prefix island's
// (the branch's NUR210 close, compiler prefix_island.go): the interpreter's
// own re-step over the window, so a fn value the run leaves applies as it
// does interpreted (`9 do (mk)` over `[g/v]` is 10), where the region rules
// declined these. Composed at the merge of main's #514.
func TestComputedDoBodyIslandCompiles(t *testing.T) {
	const mk12 = `def mk fn [[][List][quote [1 2]]] end `
	const g1 = `def g fn [[n:Integer][Integer][n add 1]] end def mk fn [[][List][quote [g/v]]] end `
	for _, c := range []struct{ src, want string }{
		{mk12 + `9 do (mk)`, "[9 1 2]"},
		{mk12 + `def y 9 end y do (mk)`, "[9 1 2]"},
		{`def mk fn [[][List][quote []]] end 9 do (mk)`, "[9]"},
		{mk12 + `9 8 do (mk)`, "[9 8 1 2]"},
		{mk12 + `[9 do (mk)]`, "[[9 1 2]]"},
		{g1 + `9 do (mk)`, "[10]"},
		// A list literal over a run a single-value seat would have demoted
		// to the runtime count check: the island seats the run whole.
		{`def mk fn [[n:Integer][List][[n n]]] end [9 do (mk 5)]`, "[[9 5 5]]"},
		{g1 + `[9 do (mk)]`, "[[10]]"},
	} {
		requireEngineParity(t, c.src, true)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: the interpreter answers %s, got %v / %v", c.src, c.want, got, err)
		}
	}
}

// TestComputedDoBodyCheckedPlain pins NUR213's close (the reverse-order NUR
// run, after the merge of main's #514): a computed run that may leave a
// callable, seated where the prefix island does not re-step it — values
// beneath it, entries after it, a fn's result — compiles under a runtime
// check that it left no value the interpreter re-steps (SigRef/PolyRef
// .DynBodyPlain, the VM's vm:dyn-body-plain). A plain run answers the
// interpreter's result; a run holding a fn value is the loud defer, where the
// seated run was data (`(1 add 8) do (mk)` over `[g/v]` answered `[9 fn g]`
// for 10, silent). A run ALONE at the program's end is the island's, exact.
// A run with entries after it takes the do's count island where one is
// seated (NUR348): `do (mk) 5` re-steps g over the 5 as the interpreter
// does, and so does a run beneath a paren the island writes back as its
// value (`(1 add 8) do (mk)`, wholeParen). A fn's run keeps the check's
// defer: its unit's island ends the frame on its own (SigRef.CountFrame).
func TestComputedDoBodyCheckedPlain(t *testing.T) {
	const g1 = `def g fn [[n:Integer][Integer][n add 1]] end def mk fn [[][List][quote [g/v]]] end `
	const g0 = `def g fn [[][Integer][7]] end `
	agreeOnBothLanes(t, g1+`do (mk) 5`, "[6]")
	agreeOnBothLanes(t, g1+`(1 add 8) do (mk)`, "[10]")
	for _, c := range []struct{ src, wantI string }{
		{g1 + `(8 dup drop) do (mk)`, "[9]"},
		{g0 + `def f fn [[b:List][Any][do b]] end f (quote [g/v])`, "[7]"},
		// A gradual body re-matches do's overloads (CALL_NATIVE_POLY) under
		// the same check.
		{g0 + `def f fn [[m:Map][Any][do m.k]] end f {k: (quote [g/v])}`, "[7]"},
	} {
		requireCheckedPlainDefer(t, c.src, c.wantI)
	}
	if dis := compileDisasm(t, g1+`(8 dup drop) do (mk)`); !strings.Contains(dis, "[plain values, checked]") {
		t.Errorf("the seated run's call carries the plain check; got:\n%s", dis)
	}
	// The generic unit's gradual body re-matches do; the call specialises on
	// the map's shape (main's #517), whose unit calls do over the member's
	// List directly — under the same check either way.
	const mapDo = `def f fn [[m:Map][Any][do m.k]] end f {k: (quote [g/v])}`
	if dis := compileDisasmNoSpec(t, g0+mapDo); !strings.Contains(dis, "(poly) [plain values, checked]") {
		t.Errorf("the gradual body's poly call carries the plain check; got:\n%s", dis)
	}
	if dis := compileDisasm(t, g0+mapDo); !strings.Contains(dis, "do (List) [plain values, checked]") {
		t.Errorf("the shape-specialised body's call carries the plain check; got:\n%s", dis)
	}
	for _, c := range []struct{ src, want string }{
		// Plain runs beside their neighbours.
		{`def mk fn [[][List][quote [1 2]]] end (1 add 8) do (mk)`, "[9 1 2]"},
		{`def x 99 end def mk fn [[][List][quote [def x 5]]] end do (mk) end 7`, "[7]"},
		{`def f fn [[b:List][Any][do b]] end f (quote [1 add 2])`, "[3]"},
		// A run alone: the island re-steps it (a named 0-arg fn fires, an
		// arg-taking lambda takes the value beside it in the run).
		{g0 + `def mk fn [[][List][quote [g/v]]] end do (mk)`, "[7]"},
		{`def g fn [[n:Integer][Integer][n add 1]] end def mk fn [[][List][quote [5 g/v]]] end do (mk)`, "[6]"},
		{`def mk fn [[][List][quote [([n:Integer] => [n add 1]) 5]]] end do (mk)`, "[6]"},
	} {
		requireEngineParity(t, c.src, true)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: the interpreter answers %s, got %v / %v", c.src, c.want, got, err)
		}
	}
}

// requireCheckedPlainDefer asserts src COMPILES and its run dies at the
// plain-run check — a compiler-defect bail naming the seated run — where
// the interpreter answers wantI.
func requireCheckedPlainDefer(t *testing.T, src, wantI string) {
	t.Helper()
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog == nil || err != nil {
		t.Errorf("%q: want a compiled program, got decline %q / %v", src, reason, err)
		return
	}
	gotC, _, errC := mustNew(t).RunCompiled(src)
	if !isBailDefect(errC) || !strings.Contains(errC.Error(), "where the run is seated as data") || len(gotC) != 0 {
		t.Errorf("%q: want the loud dyn-body-plain defer, got %v / %v", src, gotC, errC)
	}
	if gotI, errI := mustNew(t).RunInterp(src); errI != nil || fmt.Sprint(gotI) != wantI {
		t.Errorf("%q: interpreter answered %v / %v, want %s", src, gotI, errI, wantI)
	}
}

// NUR210's follow-up (2026-09-26): a computed `do` run that may leave a
// callable, consumed by a SINGLE-VALUE seat — a call operand, a list
// element, a promoted value-def, the mini-s3 handler's `def ok (do b error
// […])` — compiles as one value under a RUNTIME COUNT CHECK (compiler
// dyn_body_one.go, the VM's vm:dyn-body-one) instead of declining as a
// region. A run that leaves exactly one plain value answers the
// interpreter's result; any other run is a loud designed defer the compiled
// lane owns — never a value seated where the interpreter placed another.
// The kept-defs latch lets a read of a binding a def made AFTER the run
// through (`if ok …`), and nothing else.

// requireCheckedOneDefer asserts src COMPILES, and that its run dies at the
// runtime count check — a compiler-defect bail naming the computed body —
// where the interpreter answers wantI (a value, or an error code prefixed
// "error:").
func requireCheckedOneDefer(t *testing.T, src, wantI string) {
	t.Helper()
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog == nil || err != nil {
		t.Errorf("%q: want a compiled program, got decline %q / %v", src, reason, err)
		return
	}
	gotC, _, errC := mustNew(t).RunCompiled(src)
	if !isBailDefect(errC) || !strings.Contains(errC.Error(), "over a computed body left") || len(gotC) != 0 {
		t.Errorf("%q: want the loud dyn-body-one defer, got %v / %v", src, gotC, errC)
	}
	gotI, errI := mustNew(t).RunInterp(src)
	if errI != nil {
		if "error:"+codeOf(errI) != wantI {
			t.Errorf("%q: interpreter error %v, want %s", src, errI, wantI)
		}
	} else if fmt.Sprint(gotI) != wantI {
		t.Errorf("%q: interpreter answered %v, want %s", src, gotI, wantI)
	}
}

func TestComputedDoBodyCheckedOneCompiles(t *testing.T) {
	const risky = `def risky fn [[b:Any][Any][def ok (do b error [ drop false ]) if ok [ 1 ] [ 0 ]]] end `
	for _, src := range []string{
		// The mini-s3 handler shape (TestStampDynEnvLateArmDrift), on the
		// value path, the handled-error path and a raise the handler passes.
		risky + `risky [true]`,
		risky + `risky [false]`,
		risky + `risky [raise 'x']`,
		// A List param's run at each single-value seat.
		`def f fn [[b:List][Any][def ok (do b) ok]] end f (quote [5])`,
		`def f fn [[b:List][Any][def ok (do b) ok add 1]] end f (quote [5])`,
		`def f fn [[b:List][Any][(do b) add 1]] end f (quote [5])`,
		`def f fn [[b:List][List][[9 do b]]] end f (quote [5])`,
		`def f fn [[b:List][Any][do b drop 3]] end f (quote [5])`,
		// A factory whose list is computed, not a const: unproven.
		`def mk fn [[n:Integer][List][[n]]] end (do (mk 5)) add 1`,
		`def mk fn [[n:Integer][List][[n]]] end [9 do (mk 5)]`,
		`def mk fn [[n:Integer][List][[n]]] end def ok (do (mk 5)) ok`,
		// A gradual body (a map member) re-matches do's overloads at run
		// time (CALL_NATIVE_POLY), under the same check.
		`def f fn [[m:Map][Any][(do m.k) add 1]] end f {k: (quote [5])}`,
		// A def bound after a binding run reads its own fresh value.
		`def x 99 end def mk fn [[][List][quote [def x 5 1]]] end def y (do (mk)) y`,
	} {
		requireEngineParity(t, src, true)
	}
}

func TestComputedDoBodyCheckedOneDefers(t *testing.T) {
	const risky = `def risky fn [[b:Any][Any][def ok (do b error [ drop false ]) if ok [ 1 ] [ 0 ]]] end `
	for _, c := range []struct{ src, wantI string }{
		// A run of the wrong count.
		{risky + `risky [true true]`, "error:type_error"},
		{risky + `risky []`, "error:signature_error"},
		// A def's group whose name a later island reads from the compiled
		// frame: a gradual read of it after the group resumes there, and the
		// run's checked value has no re-pushable home for its bind.
		{`def f fn [[b:List][Any][def ok 1 end def ok (do b) ok]] end f (quote [5 6])`, "error:type_error"},
		{`def f fn [[b:List][Any][def ok (do b) ok add 1]] end f (quote [5 6])`, "error:type_error"},
	} {
		requireCheckedOneDefer(t, c.src, c.wantI)
	}
	// A gradual body (a map member) is the same defer in the generic unit;
	// the call specialises on the map's shape (main's #517), and that unit,
	// reading the member as a List, raises f's own return-count error with
	// the interpreter.
	const mapRun = `def f fn [[m:Map][Any][(do m.k) add 1]] end f {k: (quote [5 6])}`
	if gotC, _, errC := mustNewNoSpec(t).RunCompiled(mapRun); !isBailDefect(errC) || !strings.Contains(errC.Error(), "over a computed body left") || len(gotC) != 0 {
		t.Errorf("%q: want the generic unit's loud dyn-body-one defer, got %v / %v", mapRun, gotC, errC)
	}
	agreeOnBothLanes(t, mapRun, "ERROR:expected 1 return value(s), got 2")
	// A seat whose statement the do's count island can re-run (NUR282):
	// the run is written in the do's place, and the interpreter's own
	// answer stands on both lanes. A def's group is such a seat: the island
	// makes the def itself (markIslandMadeDefs), so a fn value the run
	// leaves re-steps as the interpreter steps it.
	for _, c := range []struct{ src, want string }{
		{`def f fn [[b:List][Any][(do b) add 1]] end f (quote [5 6])`, "ERROR:expected 1 return value(s), got 2"},
		{`def f fn [[b:List][Any][(do b) add 1]] end f (quote [])`, "ERROR:cannot call `add`"},
		{`def f fn [[b:List][Any][def ok (do b) ok]] end f (quote [5 6])`, "ERROR:expected 1 return value(s), got 2"},
		{`def f fn [[b:List][Any][def ok (do b) ok]] end f (quote [])`, "ERROR:undefined word: ok"},
		{`def f fn [[b:List][Any][def ok (do b) end ok]] end f (quote [5 6])`, "ERROR:expected 1 return value(s), got 2"},
		{`def g fn [[][Integer][7]] end def f fn [[b:List][Any][def h (do b) h]] end f (quote [g/v])`, "[7]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestKeptDefsFreshReadStaysNarrow: the fresh-read exemption covers a read
// of the very binding a def made after the run at the run's own depth —
// nothing the run could have bound under the model's nose.
func TestKeptDefsFreshReadStaysNarrow(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// A conditional def after the run: the read resolves the join.
		{`def f fn [[b:List c:Boolean][Any][def ok 1 do b drop if c [def ok 2] [] ok]] end f (quote [def ok 7 0]) false`, "[7]"},
		// A second run between the def and the read.
		{`def f fn [[b:List][Any][def ok (do b) do b drop ok]] end f (quote [def ok 3 4])`, "[3]"},
	} {
		requireLoudDecline(t, c.src, "(NUR210)", c.want)
	}
	// A binding made before the run, read bare at the root, is seated live
	// (TestKeptDefsLiveReadsCompile): it reads what the run left.
	requireEngineParity(t, `def x 99 end def mk fn [[][List][quote [def x 5 1]]] end def y (do (mk)) x`, true)
}

// TestKeptDefsLiveReadsCompile pins NUR282's latch half (the reverse-order
// NUR run, after the merge of main's #514): a read the recorder seats LIVE —
// a root read after a root computed keep-defs body (rootDynLeak), a unit's
// own def after one in the unit (NUR203's leak) — observes nothing stale:
// the lookup reads the registry the body installed into, raises the
// interpreter's undefined_word on a miss and bails on a fn value. So the
// kept-defs latch lets it through (compiler noteKeptDefsRead /
// keptReadSeatedLive), and the read's value becomes a carrier of its type,
// so no literal or fold downstream bakes the pre-body value (`… drop [t]`
// is [[3]], where a concrete read folded [[0]]). A call of a fn the body may
// rebind, and a parameter's read, are still observers.
func TestKeptDefsLiveReadsCompile(t *testing.T) {
	const mk = `def x 99 end def mk fn [[][List][quote [def x 5 1]]] end `
	for _, c := range []struct{ src, want string }{
		{`def x 99 end def mk fn [[][List][quote [def x 5]]] end do (mk) end x`, "[5]"},
		{mk + `[1 2] each (mk) end x`, "[[1 1] 5]"},
		{mk + `def b (mk) end [1 2] each b end x`, "[[1 1] 5]"},
		{mk + `[1 2] each (mk) end [x]`, "[[1 1] [5]]"},
		{mk + `[1 2] each (mk) end {a: x}`, "[[1 1] {a:5}]"},
		{mk + `[1 2] each (mk) end def y x end y`, "[[1 1] 5]"},
		{mk + `[1 2] each (mk) end x add 1`, "[[1 1] 6]"},
		{`def f fn [[b:List][Any][def t 0 do b drop [t]]] end f (quote [def t 5 1])`, "[[5]]"},
		{`def f fn [[b:List][Any][def t 0 do b drop t add 1]] end f (quote [def t 5 1])`, "[6]"},
	} {
		requireEngineParity(t, c.src, true)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: the interpreter answers %s, got %v / %v", c.src, c.want, got, err)
		}
	}
	// An unbinding body: the live lookup raises the interpreter's error.
	for _, src := range []string{
		`def x 99 end def mk fn [[][List][quote [undef x]]] end do (mk) end x`,
		`def x 99 end def mk fn [[][List][quote [undef x 1]]] end [1 2] each (mk) end x`,
		`def f fn [[b:List][Any][def t 0 do b drop t]] end f (quote [undef t 1])`,
	} {
		_, _, errC, _, errI := runBothEngines(t, src)
		if codeOf(errI) != "undefined_word" || codeOf(errC) != "undefined_word" {
			t.Errorf("%s: undefined_word on both lanes, got compiled %v, interp %v", src, errC, errI)
		}
	}
	// Negative: a parameter the body may rebind, and a fn it may redefine,
	// are no live reads, and still decline.
	for _, src := range []string{
		`def f fn [[b:List xs:List][Any][each b xs drop xs]] end f (quote [def xs 5 0]) [1 2 3]`,
		`def f fn [[b:List][Any][do b drop do b]] end f (quote [def b (quote [7]) 0])`,
	} {
		prog, why, _, err := mustNew(t).CompileCheck(src)
		if prog != nil || err != nil || !strings.Contains(why, "(NUR210)") {
			t.Errorf("%q: want the latch's decline, got prog=%v reason=%q err=%v", src, prog != nil, why, err)
		}
	}
}

// TestStackCountComputedForBodyAgrees: main's #514 declined these through
// core rematchWindowMatches; composed with the branch's split-aware rematch
// (DispatchSpec.NFwd, lang nur211_test.go) the runtime rematch plans the
// window as the interpreter does, and both lanes raise signature_error.
func TestStackCountComputedForBodyAgrees(t *testing.T) {
	for _, src := range []string{
		`def mk fn [[][List][quote [i]]] end 3 for (mk)`,
		`def mk fn [[][List][quote [i]]] end (1 add 2) for (mk)`,
	} {
		requireEngineParity(t, src, true)
		if _, err := mustNew(t).RunInterp(src); codeOf(err) != "signature_error" {
			t.Errorf("%s: the interpreter raises signature_error, got %v", src, err)
		}
	}
	// The forward forms keep compiling.
	requireEngineParity(t, `for 3 [i]`, true)
	requireEngineParity(t, `def mk fn [[][List][quote [i]]] end for 3 (mk)`, true)
}
