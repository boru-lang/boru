package lang

import (
	"strings"
	"testing"
)

// sweep_cells_behave_fnsig_test.go pins the two sweep seed cells that
// graduated on 2026-09-26 after the rest of the sweep's last cells —
// `behave` × container and `fnsig` × module-export — each as a both-lane
// parity pin beside the refusals that fence it, and the three miscompiles
// the behave work found in cells that already passed.

// requireDeclines asserts src does not compile, naming why. It reads the
// compile pass alone (CompileCheck): these fences pin WHERE the recorder
// stops, and the interpreter's answer is asserted beside each so the decline
// is shown to be the compiler's, never the program's.
func requireDeclines(t *testing.T, src, wantReason string) {
	t.Helper()
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog != nil {
		t.Errorf("%q: compiled; want a decline (%s)", src, wantReason)
		return
	}
	if err == nil && !strings.Contains(reason, wantReason) {
		t.Errorf("%q: decline reason %q; want substring %q", src, reason, wantReason)
	}
	if _, errI := mustNew(t).RunInterp(src); errI != nil {
		t.Errorf("%q: the interpreter must answer (the decline is the compiler's): %v", src, errI)
	}
}

const behaveTemp = `def Temp refine Integer end `

// TestBehaveOverContainerMemberCompiles pins `behave` × container: a fn read
// from a container member (`m.c`) is a gradual dynamic(Any) carrier, which
// the generic record declined "dynamic input at behave". behave declares
// CompileDynBody now, and the dyn-body backstop records its dispatch as a
// poly re-match (recordStoredFnDyn) when the member is PROVEN to arrive as
// an interpreter fn value — a read over a const container whose member is a
// concrete fn (compiler strictFnOperandProven). A stored body that names
// something runs under DynEnv, so the behaviour's deferred run resolves its
// names in the interpreter's dynamic scope.
func TestBehaveOverContainerMemberCompiles(t *testing.T) {
	for _, src := range []string{
		// The seed.
		behaveTemp + `def m {c: (fn [[t:Temp][String]['T']])} end behave canon/q m.c end canon (make Temp 5)`,
		// The string-named form, a list member, a second install.
		behaveTemp + `def m {c: (fn [[t:Temp][String]['T']])} end behave 'canon' m.c end canon (make Temp 5)`,
		behaveTemp + `def m [(fn [[t:Temp][String]['L']])] end behave canon/q m.0 end canon (make Temp 5)`,
		behaveTemp + `def m {c: (fn [[t:Temp][String]['T']])} end behave canon/q m.c end def m {c: (fn [[t:Temp][String]['U']])} end behave canon/q m.c end canon (make Temp 5)`,
		behaveTemp + `def m {c: (fn [[t:Temp][String]['T']])} end for 2 [behave canon/q m.c] canon (make Temp 5)`,
		behaveTemp + `def m {c: (fn [[a:Temp b:Temp][Integer][0]])} end behave compare/q m.c end (make Temp 5) eq (make Temp 6)`,
		// A capture-free factory's fn stored in the container.
		behaveTemp + `def mk fn [[][Function][(fn [[t:Temp][String]['T']])]] end def m {c: (mk)} end behave canon/q m.c end canon (make Temp 5)`,
		// A stored body that NAMES something: a program def, and a fn-local
		// def the interpreter's dynamic scope resolves for a behaviour
		// dispatched inside that fn (the DynEnv mirror).
		behaveTemp + `def k 'K' end def m {c: (fn [[t:Temp][String][k]])} end behave canon/q m.c end canon (make Temp 5)`,
		behaveTemp + `def m {c: (fn [[t:Temp][String][k]])} end def g fn [[][String] [def k 'K' canon (make Temp 5)]] end behave canon/q m.c end g`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, behaveTemp+`def m {c: (fn [[t:Temp][String]['T']])} end behave canon/q m.c end canon (make Temp 5)`)
	if !strings.Contains(dis, "behave/2 (poly)") || strings.Contains(dis, "BIND_DYN_SCOPE k") {
		t.Errorf("the member's behave must re-match at run time, and a pure-data body arms no DynEnv:\n%s", dis)
	}
	// Unproven payloads keep the decline: a flex member (replaced at run
	// time), a factory's CAPTURING closure stored in the container (a
	// compiled closure carries no body tokens for behave to install), a
	// member of a param map.
	requireDeclines(t, behaveTemp+`def m (flex {c: (fn [[t:Temp][String]['T']])}) end m set c (fn [[t:Temp][String]['U']]) behave canon/q m.c end canon (make Temp 5)`,
		"dynamic input at behave")
	requireDeclines(t, behaveTemp+`def mk fn [[k:String][Function][(fn [[t:Temp][String][k]])]] end def k 'Z' end def m {c: (mk 'K')} end behave canon/q m.c end canon (make Temp 5)`,
		"dynamic input at behave")
	requireDeclines(t, behaveTemp+`def g fn [[x:Map][] [behave canon/q x.c]] end g {c: (fn [[t:Temp][String]['T']])} canon (make Temp 5)`,
		"dynamic input at behave")
}

// TestStrictStoreSlotRefusesACompiledClosure pins the three miscompiles the
// behave work measured in cells that already compiled. A CompileFnHandler-
// Strict slot (behave, the fn-util combinators, service `add`) validates an
// interpreter FnDefInfo; a factory's CAPTURING closure arrived there as a
// ClosurePayload and raised (`behave canon: fn arg has invalid payload`,
// `FnUtil.compose: argument must be a function value`) where the interpreter
// answered. The slot admits only a proven fn value now. And behave's literal
// fn inside a fn that def-shadows a name its body reads answered the
// program's binding for the interpreter's fn-local one: behave's record arms
// DynEnv when the stored body names something.
func TestStrictStoreSlotRefusesACompiledClosure(t *testing.T) {
	requireDeclines(t, behaveTemp+`def mk fn [[k:String][Function][(fn [[t:Temp][String][k]])]] end def k 'Z' end behave canon/q (mk 'K') end canon (make Temp 5)`,
		"not proven an interpreter fn value")
	requireDeclines(t, behaveTemp+`def mk fn [[k:String][Function][(fn [[t:Temp][String][k]])]] end def k 'Z' end behave 'canon' (mk 'K') end canon (make Temp 5)`,
		"not proven an interpreter fn value")
	requireDeclines(t, `import "boru:fn-util"  def mk fn [[k:Integer][Function][(x:Integer => [add k x])]] end def h (FnUtil.compose (mk 1) (mk 2)) end (h 5)`,
		"not proven an interpreter fn value")
	for _, src := range []string{
		// A capture-free factory's fn is a baked const: proven, and compiles.
		behaveTemp + `def mk fn [[][Function][(fn [[t:Temp][String]['T']])]] end behave canon/q (mk) end canon (make Temp 5)`,
		`import "boru:fn-util"  def mk fn [[][Function][(x:Integer => [add 1 x])]] end def h (FnUtil.compose (mk) (mk)) end (h 5)`,
		// The literal fn's body reads the fn-local `k` under DynEnv.
		behaveTemp + `def g fn [[][String] [def k 'K' canon (make Temp 5)]] end def k 'Z' end behave canon/q (fn [[t:Temp][String][k]]) end g`,
	} {
		requireEngineParity(t, src, true)
	}
}

const fnsigModule = `import module [def sg fn [[][List][[Integer String]]] export "M" {sg: sg/v}] end `

// TestFnsigRuntimeSpecListCompiles pins `fnsig` × module-export: `def T
// fnsig M.sg` mints its type from a list a module fn returns, which exists
// only at run time. The check pass mints NOTHING (binding the carrier would
// bake a type the run never builds, and `is T` would test the wrong node):
// the def form binds nothing on the check engine and its dispatch is emitted
// as the call it is (NoteRuntimeDefDispatch → RecordRuntimeBindDispatch), so
// the compiled run constructs and installs T exactly as the interpreter does.
// A later read of T is the pass's undefined-word finding, so the program
// declines rather than compile a guess.
func TestFnsigRuntimeSpecListCompiles(t *testing.T) {
	for _, src := range []string{
		// The seed, and a local fn's list.
		fnsigModule + `def T fnsig M.sg end 1`,
		`def sg fn [[][List][[Integer String]]] end def T fnsig (sg) end 1`,
		fnsigModule + `def T fnsig M.sg end def U fnsig M.sg end 2`,
		fnsigModule + `for 2 [def T fnsig M.sg end] 1`,
		fnsigModule + `if true [def T fnsig M.sg end] [0] 1`,
		// A run-time read of T (the interpolation reads the live registry).
		fnsigModule + `def T fnsig M.sg end '${T}'`,
		// The run-time list's own error is the interpreter's, byte for byte.
		`import module [def sg fn [[][List][[Integer]]] export "M" {sg: sg/v}] end def T fnsig M.sg end 1`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, fnsigModule+`def T fnsig M.sg end 1`)
	if !strings.Contains(dis, "; def (Atom, Atom, List)") || strings.Contains(dis, "BIND_TWIN") && strings.Contains(dis, "def T") {
		t.Errorf("the def form must run at run time as a native call:\n%s", dis)
	}
	// The fences. A static read of T: the pass never bound it.
	requireDeclines(t, fnsigModule+`def T fnsig M.sg end def f fn x:Integer String [convert String x] end f/v is T`,
		"check diagnostics")
	// A name the pass already holds keeps fnsig's own verdict on the carrier
	// (the check pass would read the OLD T after the run replaced it).
	requireDeclines(t, fnsigModule+`def T refine Integer end def T fnsig M.sg end def f fn x:Integer String [convert String x] end f/v is T`,
		"")
	// A later type of the same name part: the run-time install would meet a
	// part the pass registered AFTER it (the replay rolls parts back never).
	requireDeclines(t, fnsigModule+`def T fnsig M.sg end def T refine Integer end 1`,
		"name part")
	// Inside a compiled unit the install would outlive the frame: the form
	// declines as the compile-time word it is.
	requireDeclines(t, fnsigModule+`def g fn [[][] [def T fnsig M.sg]] end g 1`,
		"compile-time word def")
}
