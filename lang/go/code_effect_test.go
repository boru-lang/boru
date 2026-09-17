package lang

// Typed code values, stage 1 (design/legacy/checker-precision-fronts.0.ignore §1 —
// the `do` escape hatch): a QUOTED code list stored in data carries its
// analysed stack effect (compiler.CodeEffectInfo) on the carrier the checker
// holds for it, so `do` over the NON-literal body types the effect's
// outs — as BOUNDED dynamic(T) carriers, never a strict commitment —
// instead of the dynamic(Any) hatch. Producer: getIntKeyReturns (the
// `get`/`dot` integer-index element read) via compiler.AnalyseCodeEffectCarrier.
// Consumer: doListReturnsFn. Both are gated to plain (non-Compiling)
// check passes: checker precision must NOT imply compile coverage.

import (
	"fmt"
	"strings"
	"testing"
)

// checkStack runs a plain Check and returns the residual stack strings
// plus whether any error-severity diagnostic was reported.
func checkStack(t *testing.T, src string) ([]string, bool) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, cerr := a.Check(src)
	if cerr != nil {
		t.Fatalf("Check(%q): %v", src, cerr)
	}
	return res.Stack, res.Summary.Errors > 0
}

// TestCodeEffectDoTypes — the stage-1 positive: a dispatch-table entry
// read by literal index and run via `do` types as the body's residual
// bound instead of dynamic(Any). The result is dynamic(T) (a bound, not
// a proof — the body's free words re-resolve at runtime `do` time), so
// the assertion is "typed Integer, no longer Any".
func TestCodeEffectDoTypes(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		// get-form and per-element precision across a heterogeneous table
		// (each element carries its OWN effect — no join needed at stage 1).
		{`def ops [quote [1 add 2] quote [3.0 mul 4.0]] do (ops get 0)`, "dynamic(Integer)"},
		{`def ops [quote [1 add 2] quote [3.0 mul 4.0]] do (ops get 1)`, "dynamic(Float)"},
		// dot-sugar integer index shares the same sig/ReturnsFn.
		{`def ops [quote [1 add 2]] do (ops.0)`, "dynamic(Integer)"},
		// a String-producing entry.
		{`def ops [quote ['a' add 'b']] do (ops get 0)`, "dynamic(String)"},
		// a multi-value body nets its FULL residual (mirroring doListHandler).
		{`def ops [quote [1 add 2 10 mul 4]] do (ops get 0)`, "dynamic(Integer) dynamic(Integer)"},
		// the effect-typed result participates in downstream dispatch.
		{`def ops [quote [1 add 2]] do (ops get 0) add 5`, "dynamic(Integer)"},
	}
	for _, c := range cases {
		stack, flagged := checkStack(t, c.src)
		if flagged {
			t.Errorf("%q: checker flagged an error; want clean", c.src)
			continue
		}
		if got := strings.Join(stack, " "); got != c.want {
			t.Errorf("%q: residual %q, want %q", c.src, got, c.want)
		}
	}
}

// TestCodeEffectStaysDynamic — the negatives: shapes whose effect the
// stage-1 analysis cannot justify keep the pre-existing dynamic(Any)
// hatch (the gradual contract — never a guess), and READING an
// unanalysable body stays quiet (a read site describes the body, it
// does not run it; at runtime `do` catches a failing body as an Error
// VALUE).
func TestCodeEffectStaysDynamic(t *testing.T) {
	cases := []struct {
		src  string
		want string
		why  string
	}{
		// A body that needs stack inputs fails its no-input analysis (do
		// runs bodies against an empty sub-stack): unknown effect. At
		// runtime do catches the dispatch failure as an Error value.
		{`def ops [quote [1 add]] do (ops get 0)`, "dynamic(Any)",
			"input-consuming body: analysis fails, effect unknown"},
		// A diverging body (raise) leaves no residual to describe.
		{`def ops [quote [raise oops/q 'x']] do (ops get 0)`, "dynamic(Any)",
			"diverging body: no residual, effect unknown"},
		// A list op that cannot preserve the effect decays to the plain
		// element carrier (the spec's decay clause): the runtime-
		// CONSTRUCTED list reaching do carries no effect.
		{`def ops [quote [2 mul 3]] do (reverse (ops get 0))`, "dynamic(Any)",
			"effect not preserved through list ops: runtime-constructed code stays dynamic"},
		// A pure DATA element (no Word tokens) is not a code value; the
		// producer leaves it alone and do keeps the hatch.
		{`def ops [[7 9]] do (ops get 0)`, "dynamic(Any)",
			"word-free element: not code, no effect attached"},
	}
	for _, c := range cases {
		stack, flagged := checkStack(t, c.src)
		if flagged {
			t.Errorf("%q (%s): checker flagged an error; the read/do must stay quiet", c.src, c.why)
			continue
		}
		if got := strings.Join(stack, " "); got != c.want {
			t.Errorf("%q (%s): residual %q, want %q", c.src, c.why, got, c.want)
		}
	}
}

// TestCodeEffectReadSiteQuiet — reading a stored code list NEVER
// surfaces its body's diagnostics (positive twin: the same read then
// consumed by size types normally). Pinned separately because the
// producer's dry analysis truncates what it raises — a leak here would
// be a false positive on every program that stores partial bodies.
func TestCodeEffectReadSiteQuiet(t *testing.T) {
	for _, src := range []string{
		`def ops [quote [1 add]] (ops get 0)`,       // failing body, read only
		`def ops [quote [1 add]] size (ops get 0)`,  // ...consumed as data
		`def ops [quote [zzz-undef 1]] (ops get 0)`, // undefined word in stored body
	} {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		res, cerr := a.Check(src)
		if cerr != nil {
			t.Fatalf("Check(%q): %v", src, cerr)
		}
		if res.Summary.Errors > 0 || res.Summary.Warnings > 0 {
			t.Errorf("%q: read of a stored code list raised %d error(s) %d warning(s); a read must describe, not run",
				src, res.Summary.Errors, res.Summary.Warnings)
		}
	}
}

// TestCodeEffectInterpreterUnchanged — the plain interpreter's behavior
// is untouched by the check-mode effect machinery (check-mode only, by
// construction): the table programs run to the same values as before.
func TestCodeEffectInterpreterUnchanged(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`def ops [quote [1 add 2] quote [3 mul 4]] do (ops get 0)`, "[3]"},
		{`def ops [quote [1 add 2] quote [3 mul 4]] do (ops get 1)`, "[12]"},
		{`def ops [quote [1 add 2 10 mul 4]] do (ops get 0)`, "[1 48]"},
	}
	for _, c := range cases {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, rerr := a.RunInterp(c.src)
		if rerr != nil {
			t.Errorf("%q: run error %v", c.src, rerr)
			continue
		}
		if fmt.Sprint(got) != c.want {
			t.Errorf("%q: interpreter result %v, want %s", c.src, got, c.want)
		}
	}
	// A failing stored body is CAUGHT by do as an Error value (the escape-
	// hatch semantics) — not a program error.
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, rerr := a.RunInterp(`def ops [quote [1 add]] do (ops get 0)`)
	if rerr != nil || len(got) != 1 || !strings.Contains(fmt.Sprint(got), "error(") {
		t.Errorf("failing stored body: got %v err=%v, want a single caught Error value", got, rerr)
	}
}

// TestCodeEffectCompileDiscipline — checker precision must NOT imply
// compile coverage (the spec's explicit warning). The producer and
// consumer are gated to !Check.Compiling, so the compile pass sees
// today's exact surface:
//
//   - a `do` whose computed body the emitter can concrete-fold (a
//     literal index into a concrete table — tryFoldStaticIndex) keeps
//     compiling NATIVELY with runtime parity;
//   - every other do-over-carrier REFUSES to lower (whole-program
//     interpreter fallback) rather than islanding or mis-lowering.
func TestCodeEffectCompileDiscipline(t *testing.T) {
	// Foldable: compiles natively (no FALLBACK island) and matches the
	// interpreter.
	src := `def ops [quote [1 add 2] quote [3 mul 4]] do (ops get 0)`
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: expected the emit-side static-index fold to compile; reason=%q err=%v", src, reason, cerr)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("%q: compiled with a FALLBACK island; expected a native do-closure", src)
	}
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(src)
	c, _ := New()
	gotI, errI := c.RunInterp(src)
	if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: compiled/interpreted parity broke: compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
			src, compiled, gotC, errC, gotI, errI)
	}

	// Non-foldable carrier bodies: the compile pass REFUSES (prog nil,
	// named reason) and the program still runs correctly on the
	// interpreter fallback.
	// The dyn-body backstop (CompileDynBody, the always-compile goal) now
	// COMPILES a computed body whose operand types strictly as a List: the
	// CALL_NATIVE's runtime sub-run over the concrete tokens IS the
	// interpreter's own execution under the DynEnv environment mirror.
	compiles := []struct {
		src  string
		want string
	}{
		{`def ops [quote [2 mul 3]] do (reverse (ops get 0))`, "[6]"},
	}
	for _, cc := range compiles {
		d, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := d.CompileCheck(cc.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: the dyn-body backstop must compile a typed computed body; reason=%q err=%v", cc.src, reason, cerr)
			continue
		}
		e, _ := New()
		got, compiled, rerr := e.RunCompiled(cc.src)
		if !compiled || rerr != nil || fmt.Sprint(got) != cc.want {
			t.Errorf("%q: want %s compiled; got %v compiled=%v err=%v", cc.src, cc.want, got, compiled, rerr)
		}
	}
	// A GRADUAL-Any collection operand compiles as a dyn-body POLY: the
	// runtime re-match over do's own List/Map sigs picks the overload exactly
	// as the interpreter's dispatch does (tryRecordDynBody's body.Dynamic
	// branch), so no static commit is needed.
	graduals := []struct {
		src  string
		want string
	}{
		{`def i 0 def ops [quote [1 add 2]] do (ops get (i add 0))`, "[3]"},
	}
	for _, rc := range graduals {
		d, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := d.CompileCheck(rc.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: the dyn-body poly must compile a gradual do operand; reason=%q err=%v", rc.src, reason, cerr)
			continue
		}
		e, _ := New()
		got, compiled, rerr := e.RunCompiled(rc.src)
		if !compiled || rerr != nil || fmt.Sprint(got) != rc.want {
			t.Errorf("%q: want %s compiled; got %v compiled=%v err=%v", rc.src, rc.want, got, compiled, rerr)
		}
	}
}
