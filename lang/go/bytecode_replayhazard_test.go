package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The do-unit registry-replay fix (design/RUNTIME-INDEPENDENCE-COMPLETION-
// PLAN.0.md, Phase 6 item; found by the variation sweep): a baked code body
// containing a registry-mutating statement used to REPLAY it at VM time
// against the check pass's surviving state. Two halves, pinned here from
// both directions:
//
//   - typed defs REFUSE at the bake decision (bodyHasReplayHazard) and ride
//     the sound interpreter fallback — parity, never a type-conflict Error;
//   - imports compile NATIVELY as closure units (the check-time install is
//     the only install) once ensureExportsBound re-binds a real ModuleExport
//     — parity, never mini/parse_unknown_lang.
func TestReplayHazardTypedDefRefusesWithParity(t *testing.T) {
	// GRADUATED 2026-07-14 (do-def leak fidelity): the typed-def do bodies
	// COMPILE now — the check pass keeps do-body defs (RunCarrierBodyKeepDefs,
	// matching the runtime leak), so the closure re-analysis shadow-rebinds
	// instead of tripping the parts conflict, and the body lowers to a
	// closure unit whose runtime def is registry-visible. The pin's contract
	// moves from "must refuse" to "must compile and run byte-identical" —
	// including the leak-semantics edges: the repeated fn-scoped install
	// (the interpreter's own parts conflict, reproduced via fallback), the
	// post-do redefinition, and undef-after-do.
	for _, src := range []string{
		`do [def Big Integer 15 is Big]`,
		`do [def Big (Integer gt 10) 15 is Big]`,
		`do [def Big Integer 15 is Big] error [dot code]`,
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: refused (%q) — the leak-fidelity compile must lower this body", src, reason)
		}
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		c, _ := New()
		gotI, errI := c.RunInterp(src)
		if !compiled {
			t.Errorf("%q: fell back; want the compiled run", src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: parity: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
		}
	}
	// The leak-semantics edges hold parity whichever path runs them (the
	// repeated fn-scoped install falls back and reproduces the interpreter's
	// own parts-conflict Error value).
	for _, src := range []string{
		`def rpt fn [[][Any][do [def Big Integer 15 is Big]]] (rpt) (rpt)`,
		`do [def Big Integer 15 is Big] end def Big String 'x' is Big`,
		`do [def Big Integer] end undef Big 15 is Big`,
		`for 2 [do [def Big Integer 15 is Big]]`,
	} {
		b, _ := New()
		gotC, _, errC := b.RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		c, _ := New()
		gotI, errI := c.RunInterp(src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: parity: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
		}
	}
}

// The import half compiles natively (closure unit, no token replay) with
// byte-identical results — including VM-time APPLICATION of module fns
// (their bodies compile to their own units, so no name lookup replays).
func TestReplayHazardImportBodyCompilesNative(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`do [import "boru:minilang"  +re/[a-z]+/ typeof]`, "[Function]"},
		{`do [import "boru:string-util" StringUtil.upper 'ab']`, "[AB]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Fatalf("%q: want a native compile, got reason=%q err=%v", c.src, reason, cerr)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: islanded; want native", c.src)
		}
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		d, _ := New()
		gotI, errI := d.RunInterp(c.src)
		if !compiled || errC != nil || errI != nil {
			t.Fatalf("%q: compiled=%v errC=%v errI=%v", c.src, compiled, errC, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}
}

// Value defs inside baked bodies were never hazardous and must KEEP
// compiling (the negative proving the refusal is not blanket).
func TestReplayHazardValueDefStillCompiles(t *testing.T) {
	src := `do [def b 5 b add 1]`
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: want a compile, got reason=%q err=%v", src, reason, cerr)
	}
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled || errC != nil || fmt.Sprint(gotC) != "[6]" {
		t.Errorf("%q: compiled=%v got=%v err=%v, want [6]", src, compiled, gotC, errC)
	}
}

// ensureExportsBound must re-bind a REAL ModuleExport after a namespace
// teardown + re-import — the synthetic $name read distinguishes it from the
// plain Map the old code installed (a Map has no $name). Pure interpreter
// regression: the first import happens inside a fn body (torn down at fn
// exit), the second re-import takes the already-loaded path.
func TestEnsureExportsBoundRebindsModuleExport(t *testing.T) {
	src := `def f fn [[] [Integer] [import "boru:minilang" 0]]  (f)  import "boru:minilang"  MiniLang get "$name"`
	a, _ := New()
	got, err := a.RunInterp(src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if fmt.Sprint(got) != "[0 MiniLang]" {
		t.Errorf("got %v, want [0 MiniLang] (the re-bound namespace must carry the module-namespace facet answering $name)", got)
	}
	// And the re-bound namespace serves kind lookups (the mini matcher
	// dispatches through the namespace facet — the exact read that broke).
	src2 := `def f fn [[] [Integer] [import "boru:minilang" 0]]  (f)  import "boru:minilang"  +re/[a-z]+/ typeof`
	b, _ := New()
	got2, err2 := b.RunInterp(src2)
	if err2 != nil {
		t.Fatalf("mini after re-import: %v", err2)
	}
	if fmt.Sprint(got2) != "[0 Function]" {
		t.Errorf("mini after re-import: got %v, want [0 Function]", got2)
	}
}
