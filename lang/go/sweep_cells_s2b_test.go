package lang

import (
	"strings"
	"testing"
)

// sweep_cells_s2b_test.go pins the mechanisms that took the generated
// sweep's last failing seed cells (test/go/sweep/seeds.tsv) to compiled
// parity on 2026-09-26 — each as a both-lane parity pin beside the refusal
// that fences it.

// TestModifierOverFactoryClosureCompiles pins `force-arity` / `forward-args`
// / `usurp` / `stack-args` × factory: a dispatch-modifier word over a user
// fn's returned closure. The gradual poly record declined every typed
// Function carrier at these CompileFnHandlerStrict slots, because the VM
// delivered a capturing closure to the native as a ClosurePayload its
// FnDefInfo assertion refused (NUR158). The natives now wrap a compiled
// closure over its bridged shape and store the closure itself
// (wrapCompiledClosure / core.UsurpClosure …), and the VM's dynamic apply
// unwraps the modifier chain to the closure and applies it natively.
func TestModifierOverFactoryClosureCompiles(t *testing.T) {
	const mk = `def mk fn [[][Function][([a:Integer b:Integer] => [a sub b])]] end `
	const mkK = `def mk fn [[k:Integer][Function][([a:Integer b:Integer] => [(a sub b) add k])]] end `
	for _, src := range []string{
		// The four seeds.
		mk + `force-arity 2 (mk) 1 2`,
		mk + `forward-args (mk) 10 3`,
		mk + `usurp (mk) 10 3`,
		mk + `10 3 stack-args (mk)`,
		// A CAPTURING factory closure — the ClosurePayload NUR158 measured.
		mkK + `usurp (mk 100) 10 3`,
		mkK + `forward-args (mk 100) 10 3`,
		mkK + `force-arity 2 (mk 100) 10 3`,
		mkK + `10 3 stack-args (mk 100)`,
		mkK + `def r (usurp (mk 100)) end r 10 3`,
		mkK + `def r (force-arity 2 (usurp (mk 100))) end r 10 3`,
		mkK + `def r (usurp (mk 100)) end r/v typeof`,
		// NUR158's own witness: the dynamic-Any member read of a closure.
		mkK + `def mk2 fn [[][Map][{a:(mk 100)}]] end def m (mk2) end m.a/u 10 3`,
		mkK + `def mk2 fn [[][Map][{a:(mk 100)}]] end def m (mk2) end usurp (m.a) 10 3`,
		// A window the wrapped closure does not take parks it, as written.
		mkK + `usurp (mk 100) 'x' 3`,
		mk + `force-arity 3 (mk) 1 2 3`,
		// The lambda-body twin: the frame replays the wrapper's apply, and
		// the call's seat is the replay's count, not the un-applied residual.
		`def zzvlam ([] => [` + mk + `force-arity 2 (mk) 1 2]) zzvlam`,
		`def zzvlam ([] => [def m {s: ([a:Integer b:Integer] => [a sub b])} end usurp (m.s) 10 3]) zzvlam`,
		`def zzvlam ([] => [def m {f: ([n:Integer] => [n add 1])} end def f m.f end f 5]) zzvlam`,
	} {
		requireEngineParity(t, src, true)
	}
	// A def-bound wrapper over a claimed shape reads like any produced
	// closure: a written argument it does not take is the interpreter's
	// `cannot call r`, which the flattened dynamic apply parked — so the read
	// declines (the reversed param types ride the claim).
	fnValueM2CompileFailure(t, "def-bound usurp wrapper, unfit window",
		mkK+`def r (usurp (mk 100)) end r 'x' 3`, "does not fit the wrapper's parameter")
	fnValueM2CompileFailure(t, "def-bound forward-args wrapper, unfit window",
		mkK+`def r (forward-args (mk 100)) end r 'x' 3`, "does not fit the wrapper's parameter")
	// A replayed call's seat beside other residual values is unknowable.
	fnValueM2CompileFailure(t, "replayed lambda call under a consumer",
		`def zzvlam ([] => [def m {s: ([a:Integer b:Integer] => [a sub b])} end usurp (m.s) 10 3]) zzvlam add 1`,
		"frame replays a dynamic apply")
}

// TestBranchOfLambdasCompiles pins `if` × lambda: a branch whose arms are both
// capture-free lambdas (or compiled closures carrying their render) renders
// its merged value from the payload the interpreter parks
// (branchResultRenderKnown) — the landing already parks a 0-arg lambda and
// stands an arg-taking one aside.
func TestBranchOfLambdasCompiles(t *testing.T) {
	for _, src := range []string{
		`if true ([] => [1]) ([] => [2])`,
		`if false ([] => [1]) ([] => [2])`,
		`def c true end if c ([] => [1]) ([] => [2])`,
		`def c false end if c ([] => [1]) ([x:Integer] => [2])`,
		`def c false end 5 if c ([] => [1]) ([x:Integer] => [x add 2])`,
		`if true (fn [[][Integer][1]]) (fn [[][Integer][2]])`,
		`def c true end [if c ([] => [1]) ([] => [2])] size`,
	} {
		requireEngineParity(t, src, true)
	}
	// A NAMED fn arm fires at the landing: the render claim is the anonymous
	// const's alone, so the mixed branch keeps its decline — and so does a
	// def-bound branch value, whose NAME read dispatches.
	fnValueM2CompileFailure(t, "named fn arm beside a lambda",
		`def one fn [[][Integer][1]] end def c true end if c one/v ([] => [2])`, "closure render")
	fnValueM2CompileFailure(t, "def-bound branch of lambdas",
		`def c true end def g (if c ([] => [1]) ([] => [2])) end g`, "closure render")
	// `if` × container stays declined (the member's 0-arg landing): the read
	// is data at the landing, but the carrier it leaves hides the fn from a
	// later NAME read, `apply` or param read, which the interpreter
	// dispatches — standing the model aside was measured to miscompile
	// `def j (m get "f")  j` (NUR207's family), so it keeps its decline.
	fnValueM2CompileFailure(t, "container member 0-arg lambda in a branch",
		`def m {f: ([] => [1])} end if true m.f [2]`, "0-arg landing not modelable")
}

// TestMacroGradualLeadDispatchesAtRunTime pins `parse` × container, `mini` ×
// factory / container and `emit` × factory / container: a macro word's
// leading operand the pass cannot see concretely (a factory's returned fn, a
// container member) no longer raises the literal-name check error or declines
// on an unrecorded carrier. The compile pass records the module's runtime
// fn-dispatch native over the macro's own operands, which takes the
// interpreter's route for whatever the value is — the value form for a fn
// (a compiled closure bridged), the word re-run for anything else. `emit` ×
// container was NUR170's miscompile: the auto form took the member as the
// OPTIONS map.
func TestMacroGradualLeadDispatchesAtRunTime(t *testing.T) {
	for _, src := range []string{
		`import "boru:parselang" end def m {p: ([source:String opts:Map] => [source])} end parse m.p 'x'`,
		`import "boru:parselang" end def m {p: ([source:String opts:Map] => [source add '!'])} end parse m.p {} 'x'`,
		`import "boru:parselang" end def m {p: ini/q} end (parse m.p 'b = hello') get 'b'`,
		`import "boru:parselang" end def mk fn [[k:String][Function][([source:String opts:Map] => [source add k])]] end parse (mk '!') 'x'`,
		`import "boru:minilang" end def mk fn [[][Function][([src:String opts:Map] => [src add src])]] end mini (mk) 'ab'`,
		`import "boru:minilang" end def m {d: ([src:String opts:Map] => [src add src])} end mini m.d 'ab'`,
		`import "boru:minilang" end def mk fn [[k:String][Function][([src:String opts:Map] => [src add k])]] end mini (mk '!') 'ab'`,
		`import "boru:minilang" end def m {k: bf/q} end mini m.k '++++++++[>++++++++<-]>+.'`,
		`import "boru:emitlang" end def mk fn [[][Function][(fn [[value:Any opts:Map] [String] ['UP']])]] end emit (mk) {a:1}`,
		`import "boru:emitlang" end def m {up: (fn [[value:Any opts:Map] [String] ['UP']])} end emit m.up {a:1}`,
		`import "boru:emitlang" end def mk fn [[k:String][Function][(fn [[value:Any opts:Map] [String] [k]])]] end emit (mk 'K') {a:1}`,
		`import "boru:emitlang" end def mk fn [[][Function][(fn [[value:Any opts:Map] [String String] ['A' 'B']])]] end emit (mk) {a:1}`,
		`import "boru:emitlang" end def m {k: json/q} end emit m.k {a:1}`,
		`import "boru:emitlang" end def m {o: {}} end emit m.o {a:1}`,
		// The contract errors are the interpreter's, value and taxonomy.
		`import "boru:parselang" end def m {p: ([source:String] => [source])} end parse m.p 'x'`,
		`import "boru:parselang" end def m {p: 5} end parse m.p 'x'`,
		`import "boru:minilang" end def m {d: ([src:String] => [src])} end mini m.d 'ab'`,
		`import "boru:emitlang" end def mk fn [[][Function][(fn [[value:Any] [String] ['A']])]] end emit (mk) {a:1}`,
	} {
		requireEngineParity(t, src, true)
	}
	// The result count is the applied fn's own, so the dispatch records as a
	// variadic region: a position needing a static count declines.
	fnValueM2CompileFailure(t, "variadic macro dispatch under a fixed consumer",
		`import "boru:minilang" end def mk fn [[][Function][([src:String opts:Map] => [src add src])]] end mini (mk) 'ab' size`, "")
	dis := compileDisasm(t, `import "boru:emitlang" end def m {up: (fn [[value:Any opts:Map] [String] ['UP']])} end emit m.up {a:1}`)
	if !strings.Contains(dis, "emitlang-fn-dispatch") || strings.Contains(dis, "emitlang-auto") {
		t.Errorf("a gradual emit lead must dispatch at run time, never bake the auto form:\n%s", dis)
	}
}

// TestCodequoteTypeofAndMemberSpliceRun pins `codequote` × literal / lambda /
// factory and `word` × container: two compiled runs that BAILED. typeof of a
// quoted paren is the __PE lattice node, a bare type node the result screen
// took for a live Word token; and a `word` splice over a member fn read
// spreads the one fn the trailing apply after it applies (the spread's
// claimed single-value Arg).
func TestCodequoteTypeofAndMemberSpliceRun(t *testing.T) {
	for _, src := range []string{
		`typeof (codequote (1 add 2))`,
		`typeof (codequote ([] => [1]))`,
		`def mk fn [[][Function][([n:Integer] => [n add 1])]] end typeof (codequote (mk))`,
		`def q (codequote (1 add 2)) end typeof q`,
		`def m {f: ([n:Integer] => [n add 1])} end def dbl word m.f end 5 dbl`,
		`7 def m {f: ([n:Integer] => [n add 1])} end def dbl word m.f end 5 dbl`,
		`def mk fn [[k:Integer][Any][([n:Integer] => [n add k])]] end def dbl word (mk 3) end 5 dbl`,
		`def m {f: [1 2]} end def dbl word m.f end 5 dbl`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, `def m {f: ([n:Integer] => [n add 1])} end def dbl word m.f end 5 dbl`)
	if !strings.Contains(dis, "SPLICE_DYN") || !strings.Contains(dis, "CALL_DYNAMIC_TRAILING") {
		t.Errorf("the member splice must spread then apply at run time:\n%s", dis)
	}
}

// TestComputedReceiveClauseListCompiles pins `receive` × module-export: a
// COMPUTED clause list (a module fn's returned list) takes the dyn-body
// backstop — receive declares CompileDynBody with no CallableSpec, and
// tryRecordDynBody admits the sole NoEvalArgs slot for a non-concrete list
// only — so the handler parses and runs it at run time under DynEnv.
func TestComputedReceiveClauseListCompiles(t *testing.T) {
	const m = `import module [def cl fn [[][List][(quote [{} [1] after 0 [BODY]])]] export "M" {cl: cl/v}] end `
	for _, src := range []string{
		strings.Replace(m, "BODY", "0", 1) + `receive M.cl`,
		strings.Replace(m, "BODY", "7 8", 1) + `receive M.cl`,
		`def x 5 end ` + strings.Replace(m, "BODY", "x", 1) + `receive M.cl`,
		strings.Replace(m, "BODY", "0", 1) + `def f fn [[][Any][receive M.cl]] end f`,
		`def cl fn [[][List][(quote [{} [1] after 0 [0]])]] end receive (cl)`,
		// The literal list keeps its own bake, unchanged.
		`receive [{} [1] after 0 [0]]`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, `receive [{} [1] after 0 [0]]`)
	if strings.Contains(dis, "BIND_DYN") {
		t.Errorf("a literal clause list must not arm the dyn-body environment:\n%s", dis)
	}
}
