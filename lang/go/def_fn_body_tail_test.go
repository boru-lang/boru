package lang

import (
	"fmt"
	"testing"
)

// def_fn_body_tail_test.go pins the def-bound computed fn read at a closure
// body's tail over the body's values (2026-09-24, the interp-entry census's
// `each [a5] xs` rows): the interpreter dispatches the WORD `a5` over the
// element beneath (its stack phase, top-down), and the body now lowers that
// as the body-tail trailing apply (OpCallDynTrailTop at the read's claimed
// arity, the name seated for the no-match) where the native used to be
// handed the raw list and step it on the interpreter.

const dfMk = `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def a5 (mk 5) end `

var defFnBodyTailRows = []struct {
	label, src, want string
	native           bool
}{
	{"a def-bound closure read as an each body", dfMk + `each [a5] [1 2 3]`, "[[6 7 8]]", true},
	{"the stack form", dfMk + `[1 2 3] each [a5]`, "[[6 7 8]]", true},
	{"under fold's collect", `def counter fn [[start:Integer][Function][([n:Integer] => [start add n])]]  def c (counter 100)  fold [add] (each [c] [1 2 3]) 0`, "[306]", true},
	{"a two-argument closure as a fold body", `def mk2 fn [[k:Integer][Function][([a:Integer e:Integer] => [a add e add k])]] end def s (mk2 10) end 0 fold [s] [1 2 3]`, "[36]", true},
	{"a filter body", `def mkgt fn [[k:Integer][Function][([n:Integer] => [n gt k])]] end def g1 (mkgt 1) end filter [g1] [1 2 3]`, "[[2 3]]", true},
	// A body with tokens after the read is the read's own statement window
	// (a5 over the element, then add): parity through the native's raw body.
	{"the read under a later word (open)", dfMk + `each [a5 add 1] [1 2 3]`, "[[7 8 9]]", false},
	// A fn-util wrapper's product is a fn DEFINITION, not a closure: the
	// read lowers through the data-position lookup so the binding pushes
	// for the trailing apply, which calls it natively (the wrapper's inner
	// fns are stamped).
	{"a fn-util wrapper as the body", `import "boru:fn-util"  def inc x:Integer => [add 1 x] end def dbl x:Integer => [mul 2 x] end def h (FnUtil.compose inc/v dbl/v) end each [h] [1 2]`, "[[3 5]]", true},
}

func TestDefFnBodyTailParityAndNoEntry(t *testing.T) {
	for _, row := range defFnBodyTailRows {
		a, _ := New()
		gotI, errI := a.RunInterp(row.src)
		b, _ := New()
		var entries []string
		disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
			if ev.Attribution == "" {
				entries = append(entries, ev.Seam)
			}
		})
		gotC, compiled, errC := b.RunCompiled(row.src)
		disarm()
		if noteCompileDefect(t, row.src, gotC, errC) {
			t.Errorf("%s: must compile\n  %s", row.label, row.src)
			continue
		}
		if !compiled {
			t.Errorf("%s: must take the compiled lane\n  %s", row.label, row.src)
		}
		if errI != nil || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != row.want {
			t.Errorf("%s: compiled %v/%v interp %v/%v, want %s\n  %s", row.label, gotC, errC, gotI, errI, row.want, row.src)
		}
		if row.native && len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
		if !row.native && len(entries) == 0 {
			t.Errorf("%s: measured open — the row now runs natively; move it to the native rows\n  %s", row.label, row.src)
		}
	}
}

// TestDefFnBodyTailNoMatchRaisesAlike: the read is a WORD dispatch, so an
// element the closure's contract does not take raises the interpreter's
// `cannot call `a5“, named and positioned at the read, on both lanes — the
// paren-bounded value apply would have parked the closure as data.
func TestDefFnBodyTailNoMatchRaisesAlike(t *testing.T) {
	for _, src := range []string{
		dfMk + `each [a5] ["s"]`,
		dfMk + `each [a5] [1 "s"]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled {
			t.Errorf("%q: not compiled: %v", src, errC)
			continue
		}
		if codeOf(errI) != "signature_error" || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) || len(gotC) != 0 || len(gotI) != 0 {
			t.Errorf("%q: compiled %v [%s] %s, interp %v [%s] %s", src, gotC, codeOf(errC), detailOf(errC), gotI, codeOf(errI), detailOf(errI))
		}
	}
}

// TestDefFnBodyTailNestedDefSoundCompileFailure: the same read of a
// fn-body-LOCAL computed def from a nested closure body — `def a5 (mk 5)`
// inside g, then `each [a5] xs` — is a loud compile failure, not the raw
// body the native used to run on the interpreter, which raised a false
// `undefined word: a5` where the interpreter answers (NUR192, present on
// main): the frame's dynamic-scope bind installs nothing for a closure
// value, so the read cannot reach the live binding yet, and the
// unapplied-fn guard declines the body where the tail rule cannot fire
// (the read resolves to the parent's event, not the live lookup). The
// interpreter's answer is the program's, through the whole-program
// fallback.
func TestDefFnBodyTailNestedDefSoundCompileFailure(t *testing.T) {
	src := dfMk[:len(dfMk)-len("def a5 (mk 5) end ")] + `def g fn [[xs:List][List][def a5 (mk 5)  each [a5] xs]] end g [1 2 3]`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[[6 7 8]]" {
		t.Fatalf("%q: interp %v / %v", src, gotI, errI)
	}
	if compiled {
		t.Errorf("%q: the nested read must not compile yet (NUR192): compiled %v / %v", src, gotC, errC)
	}
	requireCompileDefect(t, src, gotC, errC)
}
