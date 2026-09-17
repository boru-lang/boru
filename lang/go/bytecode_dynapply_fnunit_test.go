package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// bytecode_dynapply_fnunit_test.go pins the dynamic apply INSIDE fn units
// (module-fnvalue-boundary rows 24/25/31/32/33, the I7 close-out):
//
//   - a fn body's unapplied fn-value residual compiles to the whole-frame
//     replay (OpCallDynFrame) — the paren apply `(args.0 args.1)`, the bare
//     `args.0` read returned as data, and an erroring applied body;
//   - the replay is faithful for a callee that collects BELOW the window
//     (the frame's unnamed-param re-pushes), and a runtime residual count
//     the callers' static model cannot absorb DEFERS to the interpreter
//     (CompiledFn.RetReplay) instead of diverging;
//   - a for-body per-iteration apply of a Function param compiles to
//     OpCallDynamic (apply-last hoist and apply-FIRST source orders), and a
//     break/continue raised inside the applied body crosses the island to
//     the enclosing compiled loop (escapedFlow), byte-identical.

func runBothEngines(t *testing.T, src string) (gotC []any, compiled bool, errC error, gotI []any, errI error) {
	t.Helper()
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotC, compiled, errC = b.RunCompiled(src)
	d, _ := New()
	gotI, errI = d.RunInterp(src)
	return
}

func requireParity(t *testing.T, src string, gotC []any, errC error, gotI []any, errI error) {
	t.Helper()
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("%q: parity: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
	}
}

func TestFnUnitDynFrameApplyCompiles(t *testing.T) {
	rows := []struct{ src, want string }{
		// row 24: the bare read — the fn value is DATA both sides (a 1-arg
		// callee cannot fire on the Function param below it).
		{`import module [def keep fn [[Function] [Function] [args.0]] export "L" {keep: keep/v, mk: (fn [[x:Integer] [Integer] [x mul 3]])}] end typeof (L.keep L.mk)`, "[Function]"},
		// row 25: the paren apply fires exactly once.
		{`import module [def useanon fn [[Function Integer] [Integer] [(args.0 args.1)]] export "L" {useanon: useanon/v, mk: (fn [[x:Integer] [Integer] [x mul 3]])}] end L.useanon L.mk 14`, "[42]"},
		// off-corpus: a 2-arg callee collects the frame's OTHER unnamed param
		// from BELOW the apply window (14+14) — the replay's resolved prefix.
		{`import module [def useanon fn [[Function Integer] [Integer] [(args.0 args.1)]] export "L" {useanon: useanon/v, m2: (fn [[a:Integer b:Integer] [Integer] [a add b]])}] end L.useanon L.m2 14`, "[28]"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Fatalf("%q: refused: %q err=%v", c.src, reason, cerr)
		}
		if !strings.Contains(prog.Disassemble(), "CALL_DYN_FRAME") {
			t.Errorf("%q: compiled without the whole-frame replay:\n%s", c.src, prog.Disassemble())
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || errC != nil {
			t.Fatalf("%q: compiled run: compiled=%v err=%v", c.src, compiled, errC)
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q = %v, want %s", c.src, gotC, c.want)
		}
	}
}

// row 31: a raise inside the value-dispatched body propagates out of the
// replay as the same error.
func TestFnUnitDynFrameApplyErrorPropagates(t *testing.T) {
	src := `import module [def useanon fn [[Function Integer] [Integer] [(args.0 args.1)]] export "L" {useanon: useanon/v, bm: (fn [[x:Integer] [Integer] [raise oops_e "bang"]])}] end L.useanon L.bm 1`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled {
		t.Fatalf("expected the compiled path, got fallback (errC=%v)", errC)
	}
	if errC == nil || !strings.Contains(errC.Error(), "bang") {
		t.Fatalf("compiled error = %v, want the raised bang", errC)
	}
	requireParity(t, src, gotC, errC, gotI, errI)
}

// A runtime residual the callers' static model cannot absorb — the callee
// stays data (type mismatch) or returns two values — DEFERS to the
// interpreter (RetReplay count enforcement) with full parity.
func TestFnUnitDynFrameRuntimeCountDefers(t *testing.T) {
	rows := []string{
		`import module [def useanon fn [[Function Integer] [Integer] [(args.0 args.1)]] export "L" {useanon: useanon/v, ms: (fn [[s:String] [Integer] [5]])}] end L.useanon L.ms 14`,
		`import module [def useanon fn [[Function Integer] [Integer] [(args.0 args.1)]] export "L" {useanon: useanon/v, mp: (fn [[x:Integer] [Integer Integer] [x x]])}] end L.useanon L.mp 14`,
	}
	for _, src := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if compiled {
			t.Errorf("%q: expected the runtime deferral (refused, then interpreted), got a compiled run", src)
		}
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}

func TestFnUnitLoopApplyFlowCrossesIsland(t *testing.T) {
	rows := []struct{ src, want string }{
		// row 32: apply-LAST hoist; break in the applied body terminates the
		// enclosing compiled loop (acc stops at 1).
		{`import module [def looper fn [[Function] [Integer] [def acc 0 for 5 [def acc (acc add 1) (args.0 1)] acc]] export "L" {looper: looper/v, brk: (fn [[x:Integer] [Any] [break]])}] end L.looper L.brk`, "[1]"},
		// row 33: apply-FIRST source order; continue skips the statements
		// after the apply on every iteration (acc stays 0).
		{`import module [def looper fn [[Function] [Integer] [def acc 0 for 5 [(args.0 1) def acc (acc add 1)] acc]] export "L" {looper: looper/v, skp: (fn [[x:Integer] [Any] [continue]])}] end L.looper L.skp`, "[0]"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Fatalf("%q: refused: %q err=%v", c.src, reason, cerr)
		}
		if !strings.Contains(prog.Disassemble(), "CALL_DYNAMIC") {
			t.Errorf("%q: compiled without the per-iteration apply:\n%s", c.src, prog.Disassemble())
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || errC != nil {
			t.Fatalf("%q: compiled run: compiled=%v err=%v", c.src, compiled, errC)
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q = %v, want %s", c.src, gotC, c.want)
		}
	}
}

// A VALUE-producing callee through the same looper shape accumulates one
// value per iteration — a runtime residual the static model cannot absorb,
// deferring.
func TestFnUnitLoopApplyValueCalleeDefers(t *testing.T) {
	src := `import module [def looper fn [[Function] [Integer] [def acc 0 for 3 [def acc (acc add 1) (args.0 1)] acc]] export "L" {looper: looper/v, mk: (fn [[x:Integer] [Integer] [x mul 3]])}] end L.looper L.mk`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if compiled {
		t.Errorf("expected the runtime deferral (refused, then interpreted), got a compiled run")
	}
	requireParity(t, src, gotC, errC, gotI, errI)
}

// A break raised in the applied body with NO enclosing loop anywhere: the VM
// translates the escaped signal, finds no open loop, and defers to the
// interpreter, which raises the canonical flow_error — parity on the error.
func TestFnUnitDynFrameBreakWithoutLoopDefers(t *testing.T) {
	src := `import module [def useanon fn [[Function Integer] [Integer] [(args.0 args.1)]] export "L" {useanon: useanon/v, brk: (fn [[x:Integer] [Any] [break]])}] end L.useanon L.brk 1`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if compiled {
		t.Errorf("expected the runtime deferral (no enclosing loop), got a compiled run")
	}
	if errI == nil || !strings.Contains(fmt.Sprint(errI), "outside loop") {
		t.Fatalf("interpreter error = %v, want the flow_error", errI)
	}
	requireParity(t, src, gotC, errC, gotI, errI)
}

// The replay executes ONLY the deferred apply — never the body's other
// statements (they ran as compiled ops) — and it fires at the RET, so it is
// gated to bodies where the apply is the LAST statement (replayIsBodyTail).
// Pins both sides of that contract:
//   - an effectful CALLEE fires exactly once, with identical output;
//   - a body with an effectful statement AFTER the apply refuses (compiling
//     it would run the print before the callee's output — the observed
//     inversion this gate closes), falling back with identical output.
func TestFnUnitDynFrameEffectDiscipline(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	runOut := func(src string, compiled bool) (out []any, printed string, took bool, err error) {
		a, e := New()
		if e != nil {
			t.Fatal(e)
		}
		var buf bytes.Buffer
		a.SetOutput(&buf)
		if compiled {
			out, took, err = a.RunCompiled(src)
		} else {
			out, err = a.RunInterp(src)
		}
		return out, buf.String(), took, err
	}

	// Effectful callee over the pure corpus body: compiles, fires once.
	once := `import module [def useanon fn [[Function Integer] [Integer] [(args.0 args.1)]] export "L" {useanon: useanon/v, pk: (fn [[x:Integer] [Integer] [print "callee" x mul 2]])}] end L.useanon L.pk 14`
	gotC, printedC, took, errC := runOut(once, true)
	gotI, printedI, _, errI := runOut(once, false)
	if !took || errC != nil {
		t.Fatalf("effectful callee: compiled=%v err=%v", took, errC)
	}
	if strings.Count(printedC, "callee") != 1 || printedC != printedI ||
		fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("effectful callee parity: C=%v %q I=%v %q", gotC, printedC, gotI, printedI)
	}

	// A statement AFTER the apply: the replay would reorder its effect ahead
	// of the callee's — must refuse and fall back with identical output.
	after := `import module [def useanon fn [[Function Integer] [Integer] [(args.0 args.1) print "after" drop]] export "L" {useanon: useanon/v, pk: (fn [[x:Integer] [Integer] [print "callee" x]])}] end L.useanon L.pk 14`
	a, _ := New()
	if prog, reason, _, _ := a.CompileCheck(after); prog != nil {
		t.Errorf("a post-apply statement must refuse the replay (reason=%q):\n%s", reason, prog.Disassemble())
	}
	gotC, printedC, took, errC = runOut(after, true)
	gotI, printedI, _, errI = runOut(after, false)
	if took {
		t.Error("the post-apply shape must take the interpreter fallback")
	}
	if printedC != printedI || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("fallback parity: C=%v %q/%v I=%v %q/%v", gotC, printedC, errC, gotI, printedI, errI)
	}
}

// A multi-overload fn DEF'D INSIDE an enclosing fn body, called over a
// gradual (Dynamic) arg — GRADUATED 2026-07-16 (REFUSAL-CLOSURE.0 §6b): the
// body-local binding is popped before the VM runs, so instead of a live name
// Lookup the plan FREEZES the dispatch table at record time
// (UserPolyRef.Sigs) and the VM's runtime re-match runs over the stored
// subset — the same MatchSignature first-match the interpreter takes over
// its per-call construction (source-determined, so the tables agree). Both
// arms dispatch by runtime value through ONE compiled program; a no-match
// defers to the interpreter's canonical signature_error; and the one shape
// whose live table CAN drift from the freeze — a callee whose own body
// rebinds the same name (the dynamic-scope in-place overlap-replace, which
// survives the callee's teardown) — keeps a refusal via the FnBinders
// gate.
func TestBodyLocalMultiOverloadPolyStored(t *testing.T) {
	src := `def wrapfn fn [[m:Map] [Integer] [def helper fn [[a:Integer] [Integer] [a mul 2] [b:String] [Integer] [7]] helper (m get k/q)]] wrapfn {k:3}`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("§6b: expected a stored-sig poly compile, refused: reason=%q err=%v", reason, cerr)
	}
	if !strings.Contains(prog.Disassemble(), "CALL_USER_POLY") {
		t.Errorf("expected a CALL_USER_POLY lowering:\n%s", prog.Disassemble())
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled {
		t.Errorf("the stored-sig poly program must run compiled (errC=%v)", errC)
	}
	requireParity(t, src, gotC, errC, gotI, errI)
	if fmt.Sprint(gotC) != "[6]" {
		t.Errorf("result = %v, want [6]", gotC)
	}

	// BOTH arms dispatch by runtime value through one compiled program: the
	// Integer arm doubles, the String arm returns 7.
	both := `def wrapfn fn [[m:Map] [Integer] [def helper fn [[a:Integer] [Integer] [a mul 2] [b:String] [Integer] [7]] helper (m get k/q)]] [(wrapfn {k:3}) (wrapfn {k:'s'})]`
	gotC, compiled, errC, gotI, errI = runBothEngines(t, both)
	if !compiled || errC != nil {
		t.Errorf("both-arms run: compiled=%v err=%v", compiled, errC)
	}
	requireParity(t, both, gotC, errC, gotI, errI)
	if fmt.Sprint(gotC) != "[[6 7]]" {
		t.Errorf("both-arms result = %v, want [[6 7]]", gotC)
	}

	// A runtime value NO arm matches defers to the interpreter, which raises
	// the canonical signature_error — code parity, never a stored-mode raise
	// of its own.
	nomatch := `def wrapfn fn [[m:Map] [Integer] [def helper fn [[a:Integer] [Integer] [a mul 2] [b:String] [Integer] [7]] helper (m get k/q)]] wrapfn {k:[1 2]}`
	_, compiled, errC, _, errI = runBothEngines(t, nomatch)
	if compiled {
		t.Error("the no-match run must defer to the interpreter")
	}
	if codeOf(errC) != "signature_error" || codeOf(errC) != codeOf(errI) {
		t.Errorf("no-match parity: compiled=[%s]%v interp=[%s]%v", codeOf(errC), errC, codeOf(errI), errI)
	}

	// NEGATIVE (the freeze's soundness fence): another fn whose body rebinds
	// the same local name can mutate the live table between the def and the
	// call — the in-place overlap-replace survives its teardown, so the
	// frozen table would diverge. The FnBinders gate keeps the refusal and
	// the interpreter owns it (parity via fallback).
	mutator := `def h fn [[x:Integer][Integer][ def g fn [[a:Integer][Integer][a add 100] [a:String][Integer][88]] x ]]
def f2 fn [[m:Any] [Integer] [
	def g fn [[a:Integer][Integer][a add 1] [a:String][Integer][99]]
	def u (h 5)
	g (m get "k")
]]
f2 (flex {k:41})`
	if prog, reason, _, _ := mustNew(t).CompileCheck(mutator); prog != nil {
		t.Errorf("the dynamic-scope mutator shape must keep its refusal (reason=%q):\n%s", reason, prog.Disassemble())
	}
	_, compiled, errC, gotI, errI = runBothEngines(t, mutator)
	if compiled {
		t.Error("the mutator shape must not run compiled")
	}
	if codeOf(errC) != "compile_refused" {
		t.Errorf("mutator: RunCompiled err=[%s]%v, want compile_refused (Stage J)", codeOf(errC), errC)
	}
	if errI != nil || fmt.Sprint(gotI) != "[141]" {
		t.Errorf("mutator interp = %v (err=%v), want [141] — h's g wins the dispatch, which the frozen table (42) could not model", gotI, errI)
	}
}

// PR #233 review fixes, pinned end-to-end:
//
//   - a runtime-dispatched multi-overload user call (OpCallUserPoly) records
//     the caller's dynamic-binding depth on its frame, so its RET unwinds only
//     its OWN binds — without dynBase it truncated the whole run's bindings
//     and the caller's later dynamic-scope read deferred spuriously;
//   - a module-preamble fn's minted type (a module-private refine) resolves
//     through the ACTIVE unit's registry at OpPushType, exactly where the
//     interpreter's CallBoru resolves it — previously the importer-registry
//     lookup missed it and the whole program deferred.
func TestUserPolyFramePreservesCallerDynBinds(t *testing.T) {
	src := `def p fn [[a:Integer] [Integer] [a add 1] [s:String] [Integer] [0]] def f fn [[m:Map n:Integer] [Integer] [if (n lte 0) [acc2] [def acc2 n p (m get k/q) drop f m (n sub 1)]]] f {k:3} 2`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("refused: %q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	for _, op := range []string{"CALL_USER_POLY", "BIND_DYN_SCOPE", "LOOKUP_DYN_SCOPE"} {
		if !strings.Contains(dis, op) {
			t.Fatalf("missing %s — the shape no longer exercises the poly-under-dyn-binds path:\n%s", op, dis)
		}
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled || errC != nil {
		t.Fatalf("the poly RET wiped the caller's dyn binds (spurious deferral): compiled=%v err=%v", compiled, errC)
	}
	requireParity(t, src, gotC, errC, gotI, errI)
	if fmt.Sprint(gotC) != "[1]" {
		t.Errorf("result = %v, want [1]", gotC)
	}
}

func TestModulePrivateTypeResolvesInUnitRegistry(t *testing.T) {
	src := `import module [def Pos (refine Integer) def f fn [[x:Integer] [Boolean] [x is Pos]] export "L" {f: f/v}] end L.f 3`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("refused: %q err=%v", reason, cerr)
	}
	if !strings.Contains(prog.Disassemble(), "PUSH_TYPE") {
		t.Fatalf("no PUSH_TYPE — the shape no longer exercises the module-type operand:\n%s", prog.Disassemble())
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled || errC != nil {
		t.Fatalf("the module-private type operand deferred at run time: compiled=%v err=%v", compiled, errC)
	}
	requireParity(t, src, gotC, errC, gotI, errI)
	if fmt.Sprint(gotC) != "[false]" {
		t.Errorf("result = %v, want [false] (a plain Integer is not the nominal newtype)", gotC)
	}
}
