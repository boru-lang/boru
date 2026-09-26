package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestBehaveQuotedNameCompiles — behave's quoted behaviour NAME is inert
// data the handler reads verbatim (CompileQuoteInert, 2026-09-25), so the
// atom form bakes as a plain CALL_NATIVE over the baked atom and the inert
// stored fn; the VM installs the behaviour on the run-time registry's type
// exactly as the interpreter does (code-bodies.tsv L228).
func TestBehaveQuotedNameCompiles(t *testing.T) {
	for _, src := range []string{
		`def Temp refine Integer end behave canon/q (fn [[t:Temp][String]['T']]) end canon (make Temp 5)`,
		`def Temp refine Integer end behave "canon" (fn [[t:Temp][String]['T']]) end canon (make Temp 5)`,
		`def Temp refine Integer end behave size/q (fn [[t:Temp][Integer][99]]) end size (make Temp 5)`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, `def Temp refine Integer end behave canon/q (fn [[t:Temp][String]['T']]) end canon (make Temp 5)`)
	if !strings.Contains(dis, "behave") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("behave must bake as a plain native call, no island:\n%s", dis)
	}
}

// TestErrorComputedHandlerBodyCompiles — `error` declares CompileDynBody
// (2026-09-25): a COMPUTED handler body (a List param, a fn's result, a
// def-bound quoted list) lowers to a plain CALL_NATIVE under DynEnv where
// the closure path declined, as `do`'s does; ErrorHandler runs it through
// the InvokeBody seam (code-bodies.tsv L152).
func TestErrorComputedHandlerBodyCompiles(t *testing.T) {
	for _, src := range []string{
		`def f fn [[b:List][Any][do [raise oops 'x'] error b]] end f (quote ['caught'])`,
		`def f fn [[b:List][Any][do [raise oops 'x'] error b]] end f (quote [get "message"])`,
		`def f fn [[b:List][Any][do [1 add 2] error b]] end f (quote ['caught'])`,
		`def h (quote ['fb']) end do [raise oops 'x'] error h`,
		`def mk fn [[][List][quote ['fb']]] end do [raise oops 'x'] error (mk)`,
		`def h (quote ['fb']) end do [7] error h`,
	} {
		requireEngineParity(t, src, true)
	}
}

// TestContainerReadParenLeadCompiles — a paren lead that is the RESULT of a
// get/dot read the pass could not type (a flex member, a gradual map field)
// is admitted to the guarded paren apply beside the tagged member-fn read
// (ContainerReadResult, 2026-09-25): the op applies the runtime value and
// defers on a non-callable one (callbacks.tsv L61).
func TestContainerReadParenLeadCompiles(t *testing.T) {
	for _, src := range []string{
		`def reg (flex {}) end def h fn [[n:Integer][Integer][n add 1]] end reg set 'cb' h/v drop end ((reg.cb) 5)`,
		`def reg (flex {}) end def h fn [[n:Integer m:Integer][Integer][n add m]] end reg set 'cb' h/v drop end ((reg.cb) 5 6)`,
	} {
		requireEngineParity(t, src, true)
	}
	// A non-callable lead keeps the interpreter's answer (the op defers).
	for _, src := range []string{
		`def reg (flex {}) end reg set 'cb' 3 drop end ((reg.cb) 5)`,
		`def m {cb: 3} end ((m.cb) 5)`,
		`def reg (flex {}) end def h fn [[n:Integer m:Integer][Integer][n add m]] end reg set 'cb' h/v drop end ((reg.cb) 5)`,
	} {
		requireEngineParity(t, src, false)
	}
}

// TestStrictAnyDynBodyRecoveryCompiles — a CompileDynBody word over a STRICT
// Any operand (a declared `xs:Any` param, a class field's Any-typed list)
// recovers to the dyn-body poly re-match over the word's WIDEST satisfiable
// overload (2026-09-25): fold's seeded form keeps its seed in the window,
// where a best-fit 2-operand window left it on the stack (`0 6`).
func TestStrictAnyDynBodyRecoveryCompiles(t *testing.T) {
	for _, src := range []string{
		`def sum2 fn [[xs:Any][Integer][0 fold [add] xs]]  def go fn [[m:Map][Integer][sum2 m.xs]]  go {xs: [1 2 3]}`,
		`def sum2 fn [[xs:Any][Integer][0 fold [add] xs]] end sum2 [1 2 3]`,
		`def sum2 fn [[xs:Any][Integer][fold [add] xs]] end sum2 [1 2 3]`,
		`def sum2 fn [[xs:Any][Integer][0 fold [add] xs]] end sum2 {a:1 b:2}`,
		`def sq fn [[xs:Any][List][each [dup mul] xs]] end sq [1 2 3]`,
		`def ev fn [[xs:Any][List][filter [2 mod 0 eq] xs]] end ev [1 2 3 4]`,
		`def Box class {data: Any} end def b (make Box {data: [1 2 3]}) end 0 fold [add] b.data`,
		`def Box class {data: Any} end def b (make Box {data: [1 2 3]}) end fold [add] b.data`,
		`def Box class {data: Any} end def b (make Box {data: [1 2 3]}) end each [mul 2] b.data`,
		`def Box class {data: Any} end def b (make Box {data: [1 2 3]}) end scan [add] b.data`,
		`def rows [{k:'a' v:1} {k:'b' v:2} {k:'a' v:3}] end import "boru:array-util" end each ([g:Any] => [0 fold [add] g.v]) (ArrayUtil.group (each ([r:Map] => [r.k]) rows) (each ([r:Map] => [r.v]) rows))`,
	} {
		requireEngineParity(t, src, true)
	}
	// A concrete no-match stays the interpreter's own error (loud decline).
	requireEngineParity(t, `def sum2 fn [[xs:Any][Integer][0 fold [add] xs]] end sum2 7`, false)
}

// TestArgsProjectionCompiles — the bare `args` read in a fn unit is
// assembled per call from the param locals (RecordArgsProjection,
// 2026-09-25); an `args.N` fold retracts the assembly and keeps the bare
// local (code-bodies.tsv L174).
func TestArgsProjectionCompiles(t *testing.T) {
	for _, src := range []string{
		`def f fn [[a:Integer b:Integer][List][args]] end f 1 2`,
		`def f fn [[a:Integer b:Integer][Integer][args.0 add args.1]] end f 1 2`,
		`def f fn [[a:Integer b:Integer][Integer][args size]] end f 1 2`,
		`def f fn [[a:Integer b:Integer][List][def a 9 args]] end f 1 2`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, `def f fn [[a:Integer b:Integer][Integer][args.0 add args.1]] end f 1 2`)
	if strings.Contains(dis, "MAKE_LIST") {
		t.Errorf("an args.N fold must retract the projection's assembly:\n%s", dis)
	}
	dis = compileDisasm(t, `def f fn [[a:Integer b:Integer][List][args]] end f 1 2`)
	if !strings.Contains(dis, "MAKE_LIST") {
		t.Errorf("the bare args read must assemble the params:\n%s", dis)
	}
}

// TestUnpackUnprovenSourceCompiles — `unpack [names] src` over a source the
// check pass cannot read (a Map param, a fn's result) binds RUN-TIME names
// (2026-09-25): the handler notes them (NoteRuntimeBind), the dispatch is
// emitted as the plain CALL_NATIVE it is (RecordRuntimeBindDispatch), the
// stub installs record no dyn-scope def and no twin, and every read seats
// live as a gradual value, so a downstream dispatch poly re-matches
// (code-bodies.tsv L173). Before: inside a fn the shape declined ("check-
// mode suppressed a runtime error"), and at the top level it COMPILED to a
// terminal unpack_error trap the interpreter never raised — a silent
// miscompile, closed here (the trap is recorded for a PROVEN source only).
func TestUnpackUnprovenSourceCompiles(t *testing.T) {
	for _, src := range []string{
		`def f fn [[d:Map][Integer][unpack [a b] d end a add b]] end f {a:1 b:2}`,
		`def f fn [[][Map][{a:1 b:2}]] end unpack [a b] (f) end a add b`,
		`def g fn [[d:Map][Integer][unpack [a b] d end a add b]] end g {a:1 b:2} end g {a:5 b:6}`,
		`def g fn [[d:Map][Integer][unpack [a b] d end (a add b) mul 2]] end g {a:1 b:2}`,
		`def f fn [[][Map][{a:1 b:2}]] end unpack [a b] (f) end def a 9 end a add b`,
		// The interpreter's own binding discipline is kept: the fn-body
		// unpack rebinds an outer name past the call, on both lanes.
		`def a 100 end def g fn [[d:Map][Integer][unpack [a b] d end a add b]] end g {a:1 b:2} end a`,
		// A key the run-time source lacks raises the interpreter's own
		// unpack_error from the emitted call, never a compile-time trap.
		`def g fn [[d:Map][Integer][unpack [a b] d end a add b]] end g {a:1}`,
		// A LATER real def of the same name — in another fn unit, at the
		// root, before or after the unpack's fn — records as ever: the stub's
		// suppression is one occurrence, not the name (a Codex review of
		// #507 found the program-wide form answering the stub's 1 for h's 9).
		`def g fn [[d:Map][Integer][unpack [a] d end a]] end def h fn [[][Integer][def a 9 end a]] end g {a:1} end h`,
		`def h fn [[][Integer][def a 9 end a]] end def g fn [[d:Map][Integer][unpack [a] d end a]] end g {a:1} end h`,
		`def g fn [[d:Map][Integer][unpack [a] d end a]] end g {a:1} end def a 9 end a`,
		`def g fn [[d:Map][Integer][unpack [a] d end a]] end g {a:1} end def a 9 end def k fn [[][Integer][a add 1]] end k`,
	} {
		requireEngineParity(t, src, true)
	}
	// A PROVEN source keeps its compile-time binds (no call emitted).
	dis := compileDisasm(t, `def d {a:1 b:2} end unpack [a b] d end a add b`)
	if strings.Contains(dis, "unpack") {
		t.Errorf("a proven unpack must fold its binds, not call:\n%s", dis)
	}
	dis = compileDisasm(t, `def f fn [[d:Map][Integer][unpack [a b] d end a add b]] end f {a:1 b:2}`)
	if !strings.Contains(dis, "unpack") || !strings.Contains(dis, "LOOKUP_DYN_SCOPE") {
		t.Errorf("an unproven unpack must call and read live:\n%s", dis)
	}
}

// TestModuleFnParamPopIsNotARootTransition — a module fn applied inside a
// re-matched `each` body runs through CallBoru at FnBodyDepth 0; its frame
// teardown used to ledger the PARAM's pop as a root-depth BindUndef no op
// could place ("twin regime" decline, each-variants.tsv L206). A frame
// binding's pop notes no transition, as its shadowing push never did
// (core.UninstallFrameBinding, 2026-09-25).
func TestModuleFnParamPopIsNotARootTransition(t *testing.T) {
	for _, src := range []string{
		`import module [export "M" {dbl: (fn [[x:Integer][Integer][x mul 2]])}] end each [M.dbl] [1 2]`,
		`import module [export "M" {dbl: (fn [[x:Integer][Integer][x mul 2]])}] end [1 2] each [M.dbl]`,
		`import module [export "M" {dbl: (fn [[x:Integer][Integer][x mul 2]])}] end def r (each [M.dbl] [1 2]) end r`,
		`import module [export "M" {dbl: (fn [[x:Integer][Integer][x mul 2]])}] end each [3 M.dbl] [1 2]`,
	} {
		requireEngineParity(t, src, true)
	}
}

// TestValofOfMutatedFlexSeatsLive — valof's check-mode read is a def read
// like the bare word's (NoteLiveRead, 2026-09-25): a module-scope flex a
// multi-run body mutated — a homeless carrier under the pass — seats live
// on the registry's cell, so the consuming word compiles
// (module-composition.tsv L93); every other valof read is untouched.
func TestValofOfMutatedFlexSeatsLive(t *testing.T) {
	for _, src := range []string{
		`import "boru:string-util" end def acc (flex []) end for-each [StringUtil.upper acc swap append drop] ['a' 'b'] end join ',' (valof acc)`,
		`def acc (flex []) end for-each [acc swap append drop] [1 2] end size (valof acc)`,
		`def x 5 end (valof x) add 1`,
		`def f fn [[n:Integer][Integer][n add 1]] end typeof (valof f)`,
		`def g fn [[n:Integer][Integer][(valof n) add 1]] end g 4`,
		`def acc (flex []) end def g fn [[][Integer][acc 7 append drop size (valof acc)]] end g`,
	} {
		requireEngineParity(t, src, true)
	}
}

// TestApplyChainInFnBodyCompiles — a CHAIN of `apply`-word applications over
// a Function-typed param inside a fn body (`x f/v apply f/v apply`,
// callbacks.tsv L125, 2026-09-25): every step's window is the whole residual
// beneath its fn, as applyHandler re-steps; the steps lower interleaved with
// their operand pushes (fnUnitRec.applyChain, emitBodyTailApply), each but
// the last committed to ONE result (OpCallDynApplyOne — another count
// defers), the last the whole-residual tail apply the RET counts.
func TestApplyChainInFnBodyCompiles(t *testing.T) {
	for _, src := range []string{
		`def apply-twice fn [[f:Function x:Integer][Integer][x f/v apply f/v apply]]  def inc fn [[n:Integer][Integer][n add 1]]  apply-twice inc/v 5`,
		`def ap3 fn [[f:Function x:Integer][Integer][x f/v apply f/v apply f/v apply]]  def inc fn [[n:Integer][Integer][n add 1]]  ap3 inc/v 5`,
		`def apply-twice fn [[f:Function x:Integer][Integer][x f/v apply f/v apply]]  def dbl fn [[n:Integer][Integer][n mul 2]]  apply-twice dbl/v 5`,
		`def ap fn [[f:Function g:Function x:Integer][Integer][x f/v apply g/v apply]]  def inc fn [[n:Integer][Integer][n add 1]]  def dbl fn [[n:Integer][Integer][n mul 2]]  ap inc/v dbl/v 5`,
	} {
		requireEngineParity(t, src, true)
	}
	// A step netting TWO values is not the one the model committed: the
	// event form defers, and the interpreter's own answer stands.
	requireEngineParity(t, `def apply-twice fn [[f:Function x:Integer][Integer][x f/v apply f/v apply]]  def two fn [[n:Integer][Integer Integer][n n]]  apply-twice two/v 5`, false)
	// A no-match on the second step raises the interpreter's own
	// uncalled_function on both lanes (the taxonomy the corpus compares;
	// the position differs — NUR171's class).
	src := `def ap fn [[f:Function x:Integer y:Integer][Integer][x y f/v apply f/v apply]]  def add2 fn [[a:Integer b:Integer][Integer][a add b]]  ap add2/v 5 6`
	_, _, errC, _, errI := runBothEngines(t, src)
	if codeOf(errC) != "uncalled_function" || codeOf(errI) != "uncalled_function" {
		t.Errorf("%q: want uncalled_function on both lanes, got compiled=%v interp=%v", src, errC, errI)
	}
	dis := compileDisasm(t, `def apply-twice fn [[f:Function x:Integer][Integer][x f/v apply f/v apply]]  def inc fn [[n:Integer][Integer][n add 1]]  apply-twice inc/v 5`)
	if !strings.Contains(dis, "CALL_DYN_APPLY_ONE") || !strings.Contains(dis, "CALL_DYN_APPLY_TOP") {
		t.Errorf("the chain must lower as a one-result step and a tail apply:\n%s", dis)
	}
}

// TestApplyChainWrittenOperandDeclines pins the chain's admission: a later
// step takes ONLY the previous step's result. An operand written between
// two applies (`x y f/v apply z g/v apply`) is a token the interpreter's
// re-stepped fn can forward-collect into the FIRST call, which no static
// partition models (a Codex review of #508: `[4 13]` compiled against
// `[7 10]` interpreted), so the body declines and both lanes agree.
func TestApplyChainWrittenOperandDeclines(t *testing.T) {
	fnValueM2CompileFailure(t, "apply chain with a written operand between steps",
		`def add2 fn [[a:Integer b:Integer][Integer][a add b]] end def ar fn [[a:Integer b:Integer][List][args]] end def h fn [[f:Function g:Function x:Integer y:Integer z:Integer][Any][x y f/v apply z g/v apply]] end h add2/v ar/v 3 4 10`,
		"apply of a dynamic fn value not at the body tail")
}

// TestComputedForBodyCompiles — `for` declares CompileDynBody (2026-09-25):
// a COMPUTED loop body (a fn's result, a quoted list read at run time) lowers
// to the plain CALL_NATIVE under DynEnv where the loop lowering has no tokens
// to capture, and RunForLoop hosts the loop over the InvokeBody seam under a
// compiled run — the index installed per iteration as the interpreter's, an
// escaped body (break / continue) discarding that iteration's values
// (code-bodies.tsv L141). A LITERAL body keeps the native loop lowering.
// TestComputedForBodyDeclines pins that a COMPUTED for body DECLINES at
// compile time, and why. A hosted attempt (2026-09-25, withdrawn the same
// day on a Codex review of #508) ran each iteration through the InvokeBody
// seam and diverged from the interpreter's INLINE SPLICE on four measured
// shapes, the witnesses below: the body reads the caller's stack beneath
// the loop, its defs and undefs leak into the enclosing scope, a caught body
// error leaves the iterator installed, and the zero-out event seated a
// residual beneath the loop after the loop's values. No body activation
// models the splice, so the row declines loud and both lanes agree through
// the fallback (code-bodies.tsv L141).
func TestComputedForBodyDeclines(t *testing.T) {
	for _, src := range []string{
		`def mk fn [[n:Integer][List][quote [i]]] end for 3 (mk 0)`,
		`def mk fn [[][List][quote [i add]]] end 9 for 1 (mk)`,
		`def mk fn [[][List][quote [i]]] end 9 for 2 (mk)`,
		`def x 99 end def mk fn [[][List][quote [def x i]]] end for 3 (mk) end x`,
		`def x 99 end def mk fn [[][List][quote [undef x]]] end for 1 (mk) end x`,
	} {
		fnValueM2CompileFailure(t, "computed for body: "+src, src, "for: body not captured")
	}
	// The fourth divergence the review measured — a caught body error
	// leaving the iterator installed — was the interpreter's own leak,
	// literal bodies included: NUR206, closed at the merge with the
	// reverse-order NUR run (its NUR208 unwinds the live loops on the fault
	// path), pinned by TestLoopIndexUnwoundByCaughtError.
	// A def-bound quoted body is concrete at the check and keeps the native
	// loop; a literal body always did.
	for _, src := range []string{
		`def b (quote [i i mul]) end for [1 4] b`,
		`for 3 [i]`,
	} {
		requireEngineParity(t, src, true)
	}
}

// TestLoopIndexUnwoundByCaughtError pins NUR206's close: a `for` loop's
// index level is uninstalled when an error the enclosing `do` catches
// abandons the loop, so a later read of the same name — or the handler's —
// is the outer binding (99) on both lanes, a literal or a computed body
// alike. The interpreter used to leave the iteration's level installed (0):
// the raise unwound the spliced body before its move cleanup. It closed at
// the merge of main's #508 with the reverse-order NUR run, whose NUR208
// unwinds every live loop on the fault path (Engine.unwindLiveLoops, the
// break/continue twin). An `each` body raising inside the same `do` always
// agreed: its callback runs in a body run of its own.
func TestLoopIndexUnwoundByCaughtError(t *testing.T) {
	for _, src := range []string{
		`def i 99 end do [for 3 [raise oops 'x']] error [drop] end i`,
		`def i 99 end do [for 3 [raise oops 'x']] error [i]`,
		`def i 99 end def mk fn [[][List][quote [raise oops 'x']]] end do [for 3 (mk)] error [drop] end i`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if errI != nil || fmt.Sprint(gotI) != "[99]" {
			t.Errorf("%q: the interpreter must unwind the loop's index: want [99], got %v / %v", src, gotI, errI)
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != "[99]" {
			t.Errorf("%q: the compiled lane reads the outer binding [99], got compiled=%v %v / %v", src, compiled, gotC, errC)
		}
	}
	// Negative: with no outer binding the name is gone after the caught
	// error — the interpreter's undefined_word, not the iteration's value.
	if got, err := mustNew(t).RunInterp(`do [for 3 [raise oops 'x']] error [drop] end i`); err == nil || codeOf(err) != "undefined_word" {
		t.Errorf("no outer binding: the index must not survive the caught error, got %v / %v", got, err)
	}
	requireEngineParity(t, `def i 99 end do [[1 2] each [raise oops 'x']] error [i]`, true)
}
