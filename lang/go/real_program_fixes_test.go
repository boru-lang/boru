package lang

import (
	"fmt"
	"strings"
	"testing"
)

// A routed dispatch (DISPATCH_GENERIC — a module-scope word read inside a fn
// body) whose forward slot is an interpolation: the kernel evaluated it in
// place, the recorder links the value to its token, and the claim covers the
// slot, so the VM reads the value off the stack instead of asking its host to
// evaluate the template (kg/gomod.boru's ev call over the "use block"
// interpolation — the knowledge-graph pipeline died there).
func TestRoutedInterpolationSlot(t *testing.T) {
	const ev = `def ev fn [[a:String b:String c:Any] [Map] [{a:a b:b c:c}]]  def sid "s"  `
	src := ev + "def f fn m:Map Map [ ev sid \"u\" `./${m.path}` ]  f {path:\"x\"}"
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog == nil || err != nil {
		t.Fatalf("compile: %q %v", reason, err)
	}
	if !strings.Contains(prog.Disassemble(), "DISPATCH_GENERIC") {
		t.Fatalf("the site must route (it is the shape under test):\n%s", prog.Disassemble())
	}
	for _, s := range []string{
		src,
		// a list-bound call, as in kg (`def in-work-ev [(ev …)]`)
		ev + "def f fn m:Map List [ def a [(ev sid \"use block\" `./${m.path}`)] a ]  f {path:\"x\"}",
		// the same site over several values
		ev + "def f fn m:Map Map [ ev sid \"u\" `./${m.path}` ]  [(f {path:\"x\"}) (f {path:\"y\"})]",
		// the module-scope word rebound between calls: the routed op reads it live
		ev + "def f fn m:Map Map [ ev sid \"u\" `./${m.path}` ]  def a (f {path:\"x\"})  def sid \"t\"  [a (f {path:\"y\"})]",
		// an XML literal in the slot
		ev + "def f fn m:Map Map [ ev sid \"u\" <p>${m.path}</p> ]  f {path:\"x\"}",
		// an interpolation hole that raises: the interpreter's error
		ev + "def f fn m:Map Map [ ev sid \"u\" `./${m.path getr \"no\"}` ]  f {path:{}}",
	} {
		requireCompiledParity(t, s)
	}
}

// A fn body whose every path tail-calls still sizes its frame for the spill
// temps its lowering allocated (a paren operand reordered beneath a constant
// before the tail call) — kg/tests/digest_test.boru's `blk`, which wrote its
// temp past the frame ("index out of range [1] with length 1").
func TestTailCallingBodySizesItsSpills(t *testing.T) {
	for _, src := range []string{
		`def am fn m:Map Map [m]  def blk fn x:Integer Map [ am {a:1 n:(x add 1) c:x} ]  blk 1`,
		`def am fn m:Map Map [m]  def blk fn x:Integer Map [ am {a:1 n:(x add 1) c:(x mul 2) d:x} ]  [(blk 1) (blk 5)]`,
	} {
		requireCompiledParity(t, src)
	}
}

// Two results of ONE runtime-variable call (a fallible multi-value `do`
// body) are settled where they sit: no fn-value apply is laid over them, so
// the caught path's single Error value no longer underflows it, and the happy
// path answers as before.
func TestVariadicDoResultsAreNotApplied(t *testing.T) {
	for _, src := range []string{
		`import "boru:struct-util" def g StructUtil.parse/v do [(g "") 2]`,
		`import "boru:struct-util" def g StructUtil.parse/v do [(g "1") 2]`,
		`def h fn [[s:String][Any][raise bad "x"]] do [h "1" 2]`,
		`def h fn [[s:String][Any][s]] def g h/v do [(g "1") 2]`,
		`def h fn [[s:String][Any][raise bad "x"]] def g h/v do [(g "1") 2]`,
		// a do result that IS a fn still applies over a later operand
		`def k fn [[x:Integer][Integer][x mul 10]] do [k/v] 2`,
	} {
		requireCompiledParity(t, src)
	}
}

// A nested body the VM enters (a callback a handler invokes per element) runs
// on its own step budget, as the interpreter's sub-engine does: its steps are
// not charged to the run that invoked it. The VM charged every body to one
// program counter, so kg/main.boru's folds inside an `each` hit
// evaluation_limit compiled where the interpreter finished the pipeline. A
// runaway in the program's own run still trips the limit on both lanes.
func TestNestedBodyStepBudget(t *testing.T) {
	src := `each [x:Integer => [(x add 1) mul 2]] (range 0 200)`
	for _, n := range []int{100, 300} {
		gotC, compiled, errC := mustNewOpts(t, Options{Steps: n}).RunCompiled(src)
		gotI, errI := mustNewOpts(t, Options{Steps: n}).RunInterp(src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("steps %d: compiled %v [%v] (compiled=%v), interp [%v]", n, gotC, errC, compiled, errI)
		}
	}
	runaway := `def n 0  for 1000 [def n (n add 1)]  n`
	_, _, errC := mustNewOpts(t, Options{Steps: 100}).RunCompiled(runaway)
	_, errI := mustNewOpts(t, Options{Steps: 100}).RunInterp(runaway)
	for lane, err := range map[string]error{"compiled": errC, "interp": errI} {
		if err == nil || !strings.Contains(err.Error(), "evaluation_limit") {
			t.Errorf("%s: a runaway top-level loop must hit the limit, got %v", lane, err)
		}
	}
}

func mustNewOpts(t *testing.T, o Options) *Boru {
	t.Helper()
	a, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
