package lang

import (
	"fmt"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	eng "github.com/boru-lang/boru/eng/go"
)

// Call-site specialisation (check's specialiseCallSite): a call passing a
// constant fn to a named Function param of a single-signature fn records a
// unit compiled with the param bound to that fn, guarded on fn identity at
// CALL_USER entry. These tests pin what it compiles to, that it answers what
// the interpreter answers, where it must NOT apply, and the guard's fallback.

const specInc = `def inc fn [[x:Integer][Integer][x add 1]] end  `

// specUnits returns the disassembly's specialised-unit headers.
func specUnits(dis string) []string {
	var out []string
	for _, line := range strings.Split(dis, "\n") {
		if strings.HasPrefix(line, "fn f") && strings.Contains(line, " spec [") {
			out = append(out, line)
		}
	}
	return out
}

// unitBody returns the instructions of the unit whose header contains marker.
func unitBody(dis, marker string) string {
	var sb strings.Builder
	in := false
	for _, line := range strings.Split(dis, "\n") {
		if strings.HasPrefix(line, "fn f") || strings.HasPrefix(line, ";") {
			in = strings.Contains(line, marker)
			continue
		}
		if in {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// mustNewNoSpec is mustNew with call-site specialisation off — for the tests
// that pin the GENERIC compile path over a constant fn arg.
func mustNewNoSpec(t *testing.T) *Boru {
	t.Helper()
	a := mustNew(t)
	a.SetCallSiteSpecialisation(false)
	return a
}

// runBothEnginesNoSpec is runBothEngines with the compiled lane's call-site
// specialisation off.
func runBothEnginesNoSpec(t *testing.T, src string) (gotC []any, compiled bool, errC error, gotI []any, errI error) {
	t.Helper()
	gotC, compiled, errC = mustNewNoSpec(t).RunCompiled(src)
	gotI, errI = mustNew(t).RunInterp(src)
	return
}

// compileDisasmNoSpec is compileDisasm with call-site specialisation off.
func compileDisasmNoSpec(t *testing.T, src string) string {
	t.Helper()
	prog, reason, _, cerr := mustNewNoSpec(t).CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: expected to compile; reason=%q err=%v", src, reason, cerr)
	}
	return prog.Disassemble()
}

// The specialised body calls the passed fn's own compiled body directly —
// re-installed under the PARAM's name, as the interpreter's frame binding
// installs it, so it dispatches, errors and renders as `g` — and types its
// result by the fn's declared return, so the `add` after it is a fixed
// native call instead of a runtime re-match (CALL_NATIVE_POLY).
func TestCallSiteSpecialisationCallsTheFnDirectly(t *testing.T) {
	src := specInc + `def h fn [[g:Function][Integer][(g 2) add 10]] end  h inc/v`
	dis := compileDisasm(t, src)
	specs := specUnits(dis)
	if len(specs) != 1 || !strings.Contains(specs[0], "[l0=fn inc(Integer)]") {
		t.Fatalf("want one unit specialised on inc, got %v:\n%s", specs, dis)
	}
	body := unitBody(dis, " spec [")
	if !strings.Contains(body, "CALL_USER") || !strings.Contains(body, "g/1") {
		t.Errorf("the specialised body must call the fn's unit directly, under the param's name:\n%s", body)
	}
	if strings.Contains(body, "CALL_NATIVE_POLY") || strings.Contains(body, "CALL_DYN") {
		t.Errorf("the specialised body must not re-match at run time:\n%s", body)
	}
	// No generic unit is compiled beside it: the call site records only the
	// specialised one (an unexecuted generic body is not free).
	if n := strings.Count(dis, " h/1 "); n != 1 {
		t.Errorf("want only the specialised unit of h, got %d h units:\n%s", n, dis)
	}
	requireEngineParity(t, src, true)
}

// A code body naming the specialised param (`fold [drop g]`) compiles to a
// closure that tail-calls the fn, where the generic unit re-runs the body
// dynamically: the param is placed registry-visible for the frame
// (placeFnParam), so every path the body could take resolves it.
func TestCallSiteSpecialisationCodeBodyNamesParam(t *testing.T) {
	src := specInc + `def h fn [[g:Function n:Integer][Integer][0 fold [drop g] (range 0 n)]] end  h inc/v 20`
	dis := compileDisasm(t, src)
	if len(specUnits(dis)) != 1 {
		t.Fatalf("want a specialised unit:\n%s", dis)
	}
	body := unitBody(dis, " spec [")
	if !strings.Contains(body, "BIND_DYN_SCOPE") || !strings.Contains(body, "PUSH_CLOSURE") {
		t.Errorf("want the param placed and the fold body compiled as a closure:\n%s", body)
	}
	if !strings.Contains(unitBody(dis, "fold$body"), "g/1") {
		t.Errorf("the fold body must call the fn's unit directly:\n%s", dis)
	}
	requireEngineParity(t, src, true)
}

// Every shape answers what the interpreter answers — values and errors.
func TestCallSiteSpecialisationParity(t *testing.T) {
	for _, src := range []string{
		specInc + `def h fn [[g:Function][Integer][(g 2) add 10]] end  h inc/v`,
		specInc + `def h fn [[g:Function n:Integer][Integer][0 fold [drop g] (range 0 n)]] end  h inc/v 20`,
		`def h fn [[g:Function n:Integer][Integer][0 fold [drop g] (range 0 n)]] end  h ([x:Integer] => [x add 1]) 20`,
		// Two call sites, two fns, and a repeat that reuses its unit.
		specInc + `def dbl fn [[x:Integer][Integer][x mul 2]] end  def h fn [[g:Function][Integer][(g 5) add 1]] end  [(h inc/v) (h dbl/v) (h inc/v)]`,
		// The body rebinds the param: the rebinding wins, as interpreted.
		`def h fn [[g:Function][Any][def g ([x:Integer] => [x mul 2])  g 5]] end  h ([x:Integer] => [x add 1])`,
		// A 0-arg fn fires where it is read.
		`def h fn [[g:Function][Any][g]] end  h ([] => [42])`,
		// A /v read of the param is the fn as data.
		`def h fn [[g:Function][Any][g/v]] end  h ([] => [42])`,
		// Recursion passing the param along reuses the in-flight unit.
		specInc + `def h fn [[g:Function n:Integer][Integer][if (n lte 0) [0] [h g/v (n sub 1)]]] end  h inc/v 3`,
		// The fn raises inside the specialised body.
		`def bad fn [[x:Integer][Integer][x div 0]] end  def h fn [[g:Function][Integer][(g 2) add 10]] end  h bad/v`,
	} {
		requireEngineParity(t, src, true)
	}
}

// Where specialisation must not apply, the program compiles exactly as it
// does without it: no specialised unit, the same answer.
func TestCallSiteSpecialisationDeclines(t *testing.T) {
	for _, src := range []string{
		// An Any param: the declared type did not lose a Function's facts.
		specInc + `def h fn [[g:Any][Any][g 2]] end  h inc/v`,
		// A multi-signature callee: a failed guard has no one signature to
		// apply.
		specInc + `def h fn [[g:Function][Integer][(g 2) add 10] [g:Function x:Integer][Integer][(g x) add 10]] end  h inc/v`,
		// A fn read out of a container is not a constant at the call site.
		specInc + `def h fn [[g:Function][Integer][(g 2) add 1]] end  def m {f: inc/v}  h (m get "f")`,
		// A generic callee instantiates per call already.
		specInc + `def h gen [T] fn [[g:Function x:T][Any][g x]] end  h inc/v 2`,
		// A module fn arg (re-minted per import instance) and a module callee.
		`import "boru:math-util"  def f fn [[g:Function][Function][g/v]]  (f MathUtil.sqrt/v) 16.0`,
		`import module [def app fn [[g:Function][Integer][(g 3)]] export "M" {app: app/v}] end  def inc fn [[n:Integer][Integer][n add 1]] end  M.app inc/v`,
	} {
		dis := compileDisasm(t, src)
		if s := specUnits(dis); len(s) != 0 {
			t.Errorf("%q: must not specialise, got %v:\n%s", src, s, dis)
		}
		// A decline compiles exactly what specialisation off compiles — the
		// module wrapper row's open render divergence included (NUR119's
		// closure-render family, TestClosureCaptureOpenShapes).
		gotOff, _, errOff := mustNewNoSpec(t).RunCompiled(src)
		gotOn, _, errOn := mustNew(t).RunCompiled(src)
		if fmt.Sprint(errOff) != fmt.Sprint(errOn) || fmt.Sprint(gotOff) != fmt.Sprint(gotOn) {
			t.Errorf("%q: specialisation on %v [%v] / off %v [%v]", src, gotOn, errOn, gotOff, errOff)
		}
	}
}

// A source that never names the Function type cannot declare a Function
// param, so it compiles in one pass with specialisation off.
func TestCallSiteSpecialisationNeedsFunctionInSource(t *testing.T) {
	src := specInc + `def h fn [[g:Any][Any][g 2]] end  h inc/v`
	if s := specUnits(compileDisasm(t, src)); len(s) != 0 {
		t.Errorf("must not specialise, got %v", s)
	}
	requireEngineParity(t, src, true)
}

// A specialised pass that fails is re-run without specialisation, from the
// registry as it was: the program compiles exactly as it does without it.
// `g "x"` dispatches inc on a String — no match on the specialised body, a
// runtime signature_error on the generic one, which is what the interpreter
// raises.
func TestCallSiteSpecialisationRetriesWithout(t *testing.T) {
	src := specInc + `def h fn [[g:Function][Any][g "x"]] end  h inc/v`
	dis := compileDisasm(t, src)
	if s := specUnits(dis); len(s) != 0 {
		t.Errorf("the retry must record no specialisation, got %v", s)
	}
	requireEngineParity(t, src, true)
	// The retried pass starts from the registry the first found: a def the
	// program makes is installed once, not stacked on the failed pass's.
	a := mustNew(t)
	prog, reason, _, err := a.CompileCheck(`def z 1 end ` + src)
	if prog == nil || err != nil {
		t.Fatalf("compile: reason=%q err=%v", reason, err)
	}
	if d := a.NativeRegistry().Defs.Depth("z"); d > 1 {
		t.Errorf("z depth after the retried compile = %d: the failed pass's install leaked", d)
	}
}

// A recursion that passes a freshly constructed fn at every level mints at
// most FnSpecQuota specialisations of the fn.
func TestCallSiteSpecialisationQuota(t *testing.T) {
	src := `def h fn [[g:Function n:Integer][Any][if (n lte 0) [0] [h ([x:Integer] => [x]) (n sub 1)]]] end  h ([x:Integer] => [x]) 9`
	dis := compileDisasm(t, src)
	if s := specUnits(dis); len(s) == 0 || len(s) > 4 {
		t.Errorf("want between 1 and 4 specialisations, got %d:\n%s", len(s), dis)
	}
	requireEngineParity(t, src, true)
}

// failSpecGuards compiles src and points every specialised unit's guard at a
// different fn, so each CALL_USER into one takes the fallback.
func failSpecGuards(t *testing.T, a *Boru, src string) *compiler.Program {
	t.Helper()
	other, err := a.RunInterpValues(`([y:Integer] => [y])`)
	if err != nil || len(other) != 1 {
		t.Fatalf("build the other fn: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if prog == nil || cerr != nil {
		t.Fatalf("compile %q: reason=%q err=%v", src, reason, cerr)
	}
	n := 0
	for i := range prog.Fns {
		for j := range prog.Fns[i].SpecGuards {
			prog.Fns[i].SpecGuards[j].Fn = other[0]
			n++
		}
	}
	if n == 0 {
		t.Fatalf("%q: no specialised unit to fail", src)
	}
	return prog
}

// A guard that fails applies the fn itself to the call's args — in call
// position the result lands on the stack; in tail position the frame then
// returns exactly as the tail-called unit's RET would; and an error the fn
// raises surfaces as-is.
func TestCallSiteSpecialisationGuardFallback(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{specInc + `def h fn [[g:Function][Integer][(g 2) add 10]] end  h inc/v`, "[13]"},
		{specInc + `def h fn [[g:Function][Integer][(g 2) add 10]] end  def k fn [[][Integer][h inc/v]] end  k`, "[13]"},
		{specInc + `def h fn [[g:Function][Integer][(g 2) add 10]] end  (h inc/v) mul 2`, "[26]"},
	} {
		a := mustNew(t)
		prog := failSpecGuards(t, a, tc.src)
		got, err := eng.RunProgram(prog, a.NativeRegistry())
		if err != nil || fmt.Sprint(got) != tc.want {
			t.Errorf("%q: fallback gave %v / %v, want %s", tc.src, got, err, tc.want)
		}
	}
	src := `def bad fn [[x:Integer][Integer][x div 0]] end  def h fn [[g:Function][Integer][(g 2) add 10]] end  h bad/v`
	a := mustNew(t)
	prog := failSpecGuards(t, a, src)
	if _, err := eng.RunProgram(prog, a.NativeRegistry()); err == nil || !strings.Contains(err.Error(), "arith_error") {
		t.Errorf("the fallback's own error must surface, got %v", err)
	}
}

// A call THROUGH the specialised param over an argument the callee's
// contract may refuse at run time (a String reaching `inc`'s Integer) declines the
// specialisation: its refusal would be CALL_USER's contract check — the
// compiled notes, the argument VALUES — where the generic unit's run-time
// apply raises the interpreter's own (NUR172's class). So does a fn the body
// constructs capturing the param: its body compiles outside the specialised
// analysis. Both compile exactly as with specialisation off.
func TestCallSiteSpecialisationDeclinesRefusableCalls(t *testing.T) {
	for _, src := range []string{
		specInc + `def h fn [[g:Function x:Any][Any][g x]] end  h inc/v "s"`,
		`def app fn [[g:Function][Function][( fn [[x:Integer][Integer][g x]] )]]  def h (app (z:String => [z]))  (h 5)`,
	} {
		prog, _, _, _ := mustNew(t).CompileCheck(src)
		if prog != nil && len(specUnits(prog.Disassemble())) != 0 {
			t.Errorf("%q: must not specialise", src)
		}
		gotOff, _, errOff := mustNewNoSpec(t).RunCompiled(src)
		gotOn, _, errOn := mustNew(t).RunCompiled(src)
		if fmt.Sprint(errOff) != fmt.Sprint(errOn) || fmt.Sprint(gotOff) != fmt.Sprint(gotOn) {
			t.Errorf("%q: specialisation on %v [%v] / off %v [%v]", src, gotOn, errOn, gotOff, errOff)
		}
	}
}

// A specialised body whose residual misses the declared returns declines:
// the specialised unit would raise the return error at its own RET, where
// the generic unit's run-time apply raises it at the call. The retry
// compiles the call site generically, so the program behaves exactly as it
// does with specialisation off.
func TestCallSiteSpecialisationResidualMissDeclines(t *testing.T) {
	for _, src := range []string{
		// count: two applies of a two-result fn leave three values
		`def apply-twice fn [[f:Function x:Integer][Integer][x f/v apply f/v apply]]  def two fn [[n:Integer][Integer Integer][n n]]  apply-twice two/v 5`,
		// count: a 0-arg lead leaves its argument beside its result
		`def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Integer] [(k 5)]] end h z/v`,
		// type: a strict String where Integer is declared
		`def s fn [[x:Integer][String]['a']] end  def h fn [[g:Function][Integer][g 1]] end  h s/v`,
	} {
		prog, _, _, _ := mustNew(t).CompileCheck(src)
		if prog != nil && len(specUnits(prog.Disassemble())) != 0 {
			t.Errorf("%q: must not specialise", src)
		}
		gotOff, _, errOff := mustNewNoSpec(t).RunCompiled(src)
		gotOn, _, errOn := mustNew(t).RunCompiled(src)
		if fmt.Sprint(errOff) != fmt.Sprint(errOn) || fmt.Sprint(gotOff) != fmt.Sprint(gotOn) {
			t.Errorf("%q: specialisation on %v [%v] / off %v [%v]", src, gotOn, errOn, gotOff, errOff)
		}
	}
}

// The shapes the GENERIC path pins as declines (or as loud errors) compile
// through a call-site specialised unit when the fn arg is a constant, and
// answer what the interpreter answers — value and error code. Each row's
// generic pin stays in its own test, run with specialisation off.
func TestCallSiteSpecialisationGraduatedShapes(t *testing.T) {
	for _, src := range []string{
		// apply_shapes_test.go — a bare fn word at the lead's argument position
		`def inc fn [[n:Integer] [Integer] [n add 1]] end def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k inc)]] end h app/v`,
		`def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function g:Function] [Integer] [(k g)]] end def inc fn [[n:Integer] [Integer] [n add 1]] end h app/v inc/v`,
		// apply_shapes_test.go — NUR176's 0-arg lead: the interpreter's 8
		`def z fn [[] [Integer] [7]] end def inc fn [[n:Integer] [Integer] [n add 1]] end def h fn [[k:Function] [Integer] [(k inc/v)]] end h z/v`,
		// arm_tail_apply_test.go
		`def w fn [[n:Integer k:Function][Any][if (n gt 0) [k/v apply] [0]]] end (w 5 (v:Integer => [add v 1]))`,
		// bytecode_fnvalue_m2_test.go — the mid-body apply
		`def h fn [[comp:Function v:Integer] [Integer] [v comp/v apply drop 42]] h (([x:Integer] => [x add 1])/v) 5`,
		// collection_hazard_test.go — the hazard lead dispatches as interpreted
		`def f fn [[g:Function x:Integer][Integer][g x add 1]]  f (z:Integer => [mul 3 z]) 5`,
		`def f fn [[g:Function x:Integer][Integer][(g x add 1)]]  f (z:Integer => [mul 3 z]) 5`,
		`def f fn [[g:Function x:Integer][Integer][g x drop 9]]  f (z:Integer => [mul 3 z]) 5`,
		// word_read_dispatch_test.go — the read IS the word dispatch
		`def f fn [[g:Function][Any][[g]]]  f ([] => [42])`,
		`def f fn [[g:Function][Any][{a: g}]]  f ([] => [42])`,
		`def f fn [[g:Function][Any][if true [g] [0]]]  f ([] => [42])`,
		`def f fn [[g:Function][Any][g drop  g/v]]  f ([] => [42])`,
		`def f fn [[g:Function][Any][g typeof]]  f ([] => [42])`,
		// an Any param keyed on the call's Integer arg: the call through the
		// specialised param conforms, so it compiles
		specInc + `def h fn [[g:Function x:Any][Any][g x]] end  h inc/v 5`,
		// recorder_edge_paths_test.go
		`def f fn [[comp:Function][Any][(([x:Integer] => [x add 1]) comp)]]  def h fn [[g:Function][Integer][5 g/v apply]]  f h/v`,
		// returned_closure_park_test.go — a handed fn returned under the param's name
		`def app fn [[g:Function][Function][g/v]]  app (z:Integer => [mul 3 z])`,
		`def app fn [[g:Function][Function][g/v]]  app (z:Integer => [mul 3 z]) 7`,
		`def sq (z:Integer => [mul z z])  def app fn [[g:Function][Function][g/v]]  app sq/v`,
		// s2_declared_bodies_test.go — the apply chain, and a written operand
		`def apply-twice fn [[f:Function x:Integer][Integer][x f/v apply f/v apply]]  def inc fn [[n:Integer][Integer][n add 1]]  apply-twice inc/v 5`,
		`def add2 fn [[a:Integer b:Integer][Integer][a add b]] end def ar fn [[a:Integer b:Integer][List][args]] end def h fn [[f:Function g:Function x:Integer y:Integer z:Integer][Any][x y f/v apply z g/v apply]] end h add2/v ar/v 3 4 10`,
	} {
		prog, reason, _, cerr := mustNew(t).CompileCheck(src)
		if prog == nil || cerr != nil {
			t.Errorf("%q: want a compile, got reason=%q err=%v", src, reason, cerr)
			continue
		}
		if len(specUnits(prog.Disassemble())) == 0 {
			t.Errorf("%q: want a specialised unit", src)
		}
		gotI, errI := mustNew(t).RunInterp(src)
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		if !compiled || codeOf(errI) != codeOf(errC) || fmt.Sprint(gotI) != fmt.Sprint(gotC) {
			t.Errorf("%q:\n  interp   %v err=[%s]\n  compiled %v err=[%s]", src, gotI, codeOf(errI), gotC, codeOf(errC))
		}
	}
}
