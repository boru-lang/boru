package lang

import (
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
