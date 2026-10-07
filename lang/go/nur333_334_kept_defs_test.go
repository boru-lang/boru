package lang

import (
	"fmt"
	"strings"
	"testing"
)

// nur333_334_kept_defs_test.go pins two holes in the kept-defs machinery
// (compiler kept_defs.go, NUR210), both silent wrong answers after a COMPUTED
// keep-defs body — a `do`/`each` over a factory's quoted list or a List param,
// whose tokens run as a run-time-stamped unit that keeps its defs in the
// enclosing scope.

// TestNUR333ComputedBodyBindsAType: the body's `def x Integer` binds a
// lowercase name to a type NODE. The kept install re-pushes each def's value
// into the registry (a kept OpBindDynScope), but only a value with a home —
// an event, a frame slot, an inert const — was installable, and a type node
// is none of those (its operand is OpPushType, the canonical node by ID), so
// the def was skipped and the live read after the body found the name's
// earlier value: 99 for the interpreter's Integer. The same arm declined a
// fn unit's own `def t Integer` over a computed body ("dynamic-scope def `t`
// of unknown provenance"); both now push the node by type operand.
func TestNUR333ComputedBodyBindsAType(t *testing.T) {
	const mk = `def mk fn [[][List][quote [def x Integer]]] end `
	for _, c := range []struct{ src, want string }{
		// The register's witnesses.
		{`def x 99 end ` + mk + `do (mk) end x`, "[Integer]"},
		{`def x 99 end ` + mk + `do (mk) end [x]`, "[[Integer]]"},
		{`def x Integer end def mk fn [[][List][quote [def x String]]] end do (mk) end x`, "[String]"},
		// Around them: no `end`, a value left by the body, a paren body
		// value, a multi-run body, the node used as a type.
		{`def x 99 end ` + mk + `do (mk) x`, "[Integer]"},
		{`def x 99 end def mk fn [[][List][quote [def x Integer 1]]] end do (mk) end x`, "[1 Integer]"},
		{`def x 99 end def mk fn [[][List][quote [def x (Integer) 1]]] end do (mk) end x`, "[1 Integer]"},
		{`def x 99 end ` + mk + `[1 2] each (mk) end x`, "[[1 2] Integer]"},
		{`def x 99 end ` + mk + `do (mk) end 5 is x`, "[true]"},
		{`def x 99 end ` + mk + `do (mk) end x typeof`, "[Number]"},
		{`def x 99 end def mk fn [[][List][quote [def x None]]] end do (mk) end x`, "[None]"},
		{`def x 99 end def mk fn [[][List][quote [def x Any]]] end do (mk) end x`, "[Any]"},
		// A user type, bound before the body and minted inside it.
		{`def P (refine Integer) end def x 99 end def mk fn [[][List][quote [def x P]]] end do (mk) end x`, "[P]"},
		{`def x 99 end def mk fn [[][List][quote [def P (refine Integer) def x P]]] end do (mk) end x`, "[P]"},
		// Rebinding order inside the body: the last def wins.
		{`def x 99 end def mk fn [[][List][quote [def x 5 def x Integer]]] end do (mk) end x`, "[Integer]"},
		{`def x 99 end def mk fn [[][List][quote [def x Integer def x 5]]] end do (mk) end x`, "[5]"},
		// Inside a loop, and inside a fn unit over a List param.
		{`def x 99 end for 2 [do (quote [def x Integer])] end x`, "[Integer]"},
		{`def f fn [[b:List][Any][def t 0 do b drop t]] end f (quote [def t Integer 1])`, "[Integer]"},
		// The unit's own type def, which declined before (the cover pass's
		// "dynamic-scope def `t` of unknown provenance").
		{`def f fn [[b:List][Any][def t Integer do b drop t]] end f (quote [def t 5 1])`, "[5]"},
		{`def f fn [[b:List][Any][def t Integer do b drop t]] end f (quote [1])`, "[Integer]"},
		// A value def through the same path agreed already.
		{`def x 99 end def mk fn [[][List][quote [def x 5]]] end do (mk) end x`, "[5]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR333TypeDefUndefinedStillRaises is the negative half: a body that
// unbinds the name is still the interpreter's undefined_word on both lanes,
// and a type name that is not bound still fails the check pass.
func TestNUR333TypeDefUndefinedStillRaises(t *testing.T) {
	agreeOnBothLanes(t, `def x 99 end def mk fn [[][List][quote [undef x]]] end do (mk) end x`, "ERROR:undefined word: x")
	_, errI := mustNew(t).RunInterp(`def mk fn [[][List][quote [def x Nope]]] end do (mk) end x`)
	if errI == nil || !strings.Contains(errI.Error(), "Nope") {
		t.Errorf("an unbound type name raises on the interpreter, got %v", errI)
	}
}

// TestNUR333StampKeepsEveryDef: the same hole for a def the kept install
// still cannot make — a literal reparented to a user refinement (`def y:P
// 5`), whose install must follow the type's twin. A compile-time body's
// bind twin replays such a def after the unit; a run-time token-body stamp
// has none, so the def was dropped and the read after the body answered 0
// for the interpreter's 5. The stamp now declines and the interpreter runs
// the body, as for every declined stamp.
func TestNUR333StampKeepsEveryDef(t *testing.T) {
	const p = `def P (refine Integer) end `
	for _, c := range []struct{ src, want string }{
		{p + `def y 0 end def mk fn [[][List][quote [def y:P 5]]] end do (mk) end y`, "[5]"},
		{p + `def y 0 end def mk fn [[][List][quote [def y:P 5]]] end do (mk) end y/v`, "[5]"},
		{p + `def f fn [[b:List][Any][def t 0 do b drop t]] end f (quote [def t:P 5 1])`, "[5]"},
		{p + `def f fn [[b:List][Any][def t 0 do b drop t/v]] end f (quote [def t:P 5 1])`, "[5]"},
		// A literal body replays the def through its twin, as before.
		{p + `def y 0 end do [def y:P 5] end y`, "[5]"},
		{`do [def P (refine Integer) def x P def y:P 5] end y is x`, "[true]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR334ValReadAfterComputedBody: a `/v` read of a name after a computed
// keep-defs body. A bare read reaches the latch through NoteDefRead and is
// seated live by the tag hook; the `/v` read resolved the binding its own
// way (core stepWordVal) and noted neither, so inside a fn unit it baked the
// unit's stale value (0 for the interpreter's 5) and at the root it declined
// ("residual value of unknown provenance"). It now takes the same discipline
// (NoteValReadLive): seated live where the bare read is, an observer where
// the bare read is.
func TestNUR334ValReadAfterComputedBody(t *testing.T) {
	const f = `def f fn [[b:List][Any][def t 0 do b drop `
	const tail = `]] end f (quote [def t 5 1])`
	const g = `def g fn [[][Integer][7]] end `
	for _, c := range []struct{ src, want string }{
		// The register's witness and its `t/v t` sibling.
		{f + `t/v` + tail, "[5]"},
		{f + `t/v t` + tail, "ERROR:got 2 — [5 5]"},
		// The read as an operand, in a list, in a paren, twice.
		{f + `t/v add 1` + tail, "[6]"},
		{f + `[t/v]` + tail, "[[5]]"},
		{f + `(t/v)` + tail, "[5]"},
		{f + `(t/v add 1)` + tail, "[6]"},
		{f + `size [t/v]` + tail, "[1]"},
		// The body binding other kinds of value.
		{`def f fn [[b:List][Any][def t 0 do b drop t/v]] end f (quote [def t "s" 1])`, "[s]"},
		{`def f fn [[b:List][Any][def t 0 do b drop t/v]] end f (quote [def t [1 2] 1])`, "[[1 2]]"},
		{`def f fn [[b:List][Any][def t 0 do b drop t/v]] end f (quote [def t {a:1} 1])`, "[{a:1}]"},
		{`def f fn [[b:List][Any][def t 0 do b drop t/v]] end f (quote [def t Integer 1])`, "[Integer]"},
		{`def f fn [[b:List][Any][def t Integer do b drop t/v]] end f (quote [def t 5 1])`, "[5]"},
		{`def f fn [[b:List][Any][def t Integer do b drop t/v]] end f (quote [def t String 1])`, "[String]"},
		// A body that binds nothing leaves the unit's value.
		{`def f fn [[b:List][Any][def t 0 do b drop t/v]] end f (quote [1])`, "[0]"},
		// A multi-run body.
		{`def f fn [[b:List][Any][def t 0 [1 2] each b drop t/v]] end f (quote [def t 5])`, "[5]"},
		// The root: a /v read after a computed body at the program level.
		{`def x 0 end def mk fn [[][List][quote [def x 5]]] end do (mk) end x/v`, "[5]"},
		{`def x 0 end def mk fn [[][List][quote [def x 5]]] end do (mk) end [x/v]`, "[[5]]"},
		{`def x 0 end def mk fn [[][List][quote [def x 5]]] end do (mk) end x/v add 1`, "[6]"},
		{`def x 0 end def mk fn [[][List][quote [def x 5]]] end do (mk) end {a:x/v}`, "[{a:5}]"},
		{`def x 0 end def mk fn [[][List][quote [def x 5]]] end do (mk) x/v`, "[5]"},
		{`def x 99 end def mk fn [[][List][quote [def x Integer]]] end do (mk) end x/v`, "[Integer]"},
		// A literal multi-run body's leak, read by /v.
		{`[1 2] each [def x 5] end x/v`, "[[1 2] 5]"},
		{`def x 1 end [] each [def x 5] end x/v`, "[[] 1]"},
		// The body rebinds the name to another type: the read is gradual,
		// so the consumer's dispatch is the run's (computedLeakGradual).
		{`def x 0 end def mk fn [[][List][quote [def x "s"]]] end do (mk) end x/v add 1`, "[s1]"},
		{`def x 0 end def mk fn [[][List][quote [def x 2.5]]] end do (mk) end x/v add 1`, "[3.5]"},
		{`def f fn [[b:List][Any][def t 0 do b drop t/v add 1]] end f (quote [def t "s" 1])`, "[s1]"},
		{`def f fn [[b:List][Any][def t 0 do b drop t/v mul 2]] end f (quote [def t 2.5 1])`, "[5.0]"},
		// A fn or a class the body binds: the value spelling pushes it as
		// data (OpLookupDynScopeRef — ResolveRef's value), never dispatched.
		{g + `def f fn [[b:List][Any][def t 0 do b drop t/v]] end f (quote [def t g/v 1])`, "[fn t]"},
		{g + `def f fn [[b:List][Any][def t 0 do b drop [t/v]]] end f (quote [def t g/v 1])`, "[[fn t]]"},
		{g + `def x 0 end def mk fn [[][List][quote [def x g/v]]] end do (mk) end [x/v]`, "[[fn x]]"},
		{`def C class {a:1} end def f fn [[b:List][Any][def t 0 do b drop t/v]] end f (quote [def t C 1])`, "[C]"},
		{`def C class {a:1} end def x 0 end def mk fn [[][List][quote [def x C]]] end do (mk) end [x/v]`, "[[C]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// The fn the body bound, APPLIED with nothing rendered beneath it: the
	// read is a gradual Integer carrier (computedLeakGradual keeps the
	// pre-body tag as the bound), `apply`'s Function slot misses it, and the
	// no-match recovery's rematch cannot render a window that holds the
	// computed `do`'s own result under the read — so the row declines
	// (NUR384; the compile-defect ledger counts it). It compiled until
	// 2026-10-07 only through the recovery scoring its candidates in tape
	// order against signature-order slots, which read the `do` result as the
	// `[Reach Any]` overload's second operand and poly-recorded that
	// two-operand window; scored in signature order no overload is
	// compatible. A value beneath the read (`5 x/v apply`, the rows above)
	// renders, and the rematch defers the dispatch to the interpreter.
	src := g + `def x 0 end def mk fn [[][List][quote [def x g/v]]] end do (mk) end x/v apply`
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	requireCompileDefect(t, src, gotC, errC)
	if errI != nil || fmt.Sprint(gotI) != "[7]" {
		t.Errorf("%s: interpreter %v / %v, want [7]", src, gotI, errI)
	}
}

// TestNUR334ValReadMissRaises is the negative half: a body that unbinds the
// name, or a multi-run body that never ran, leaves the `/v` read the
// interpreter's undefined_word, raised at the read on both lanes.
func TestNUR334ValReadMissRaises(t *testing.T) {
	for _, src := range []string{
		`def x 99 end def mk fn [[][List][quote [undef x]]] end do (mk) end x/v`,
		`[] each [def x 5] end x/v`,
	} {
		agreeOnBothLanes(t, src, "ERROR:undefined word: x")
	}
}

// TestNUR334ValReadObserverDeclines: where the bare read of a name after a
// computed body is an OBSERVER — a name the unit does not bind, read inside
// the unit, which the body may have rebound — the `/v` read declines through
// the same latch (NUR210) rather than bake the check model's binding.
func TestNUR334ValReadObserverDeclines(t *testing.T) {
	const reason = "the read of `t` after it would read the check model's binding, which never saw the body (NUR210)"
	requireLoudDecline(t, `def t 3 end def f fn [[b:List][Any][do b drop t/v]] end f (quote [def t 5 1])`, reason, "[5]")
	requireLoudDecline(t, `def f fn [[b:List][Any][def t 0 for 2 [do b drop] t/v]] end f (quote [def t 5 1])`, reason, "[5]")
	// A read the live lookup cannot stand in for: the value may be a fn at
	// run time (here the Any node — the body may bind any value at all),
	// which the `/v` read pushes as data where the lookup would defer. It
	// is not seated, so it is the latch's observer and declines.
	requireLoudDecline(t, `def f fn [[b:List][Any][def t Any do b drop t/v]] end f (quote [def t 5 1])`, reason, "[5]")
}
