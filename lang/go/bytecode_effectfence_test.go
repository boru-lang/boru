package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// The C1 effect fence on RunCompiled's two fallback arms (eng effects.go,
// design/legacy/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.ignore): a silent interpreter
// re-run is permitted only while no observable effect escaped since before
// the check pass — RestoreForCompile rolls back registry scopes but cannot
// un-print, so a re-run after an effect DUPLICATES it (the L-DUP class,
// design/legacy/VOXGIG-COMPILE-LEAVES.2.ignore — the whole trie smoke suite printed
// twice). The pure-value differential corpus never exercises
// emit-then-fall-back, so these are the pins.

// --- the runtime-bail arm ---------------------------------------------------

// A compiled run that PRINTS and then hits a runtime internal_error (the
// zz-inst shape-claim violation from bytecode_methodshape_test.go) must
// propagate the annotated internal_error with the output emitted exactly
// once — the pre-fence behaviour re-ran the whole source and printed twice.
func TestRuntimeBailAfterEffectPropagates(t *testing.T) {
	src := `print "once" ; def i (zz-inst) ; i.m 5 ; 42`
	a := zzShapedInstance(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(src)
	if codeOf(err) != "internal_error" {
		t.Fatalf("fenced bail: err=[%s] %v (got=%v compiled=%v); want the propagated internal_error", codeOf(err), err, got, compiled)
	}
	if !strings.Contains(err.Error(), "--no-compile") {
		t.Errorf("fenced bail: error should carry the --no-compile note, got: %v", err)
	}
	if out.String() != "once\n" {
		t.Errorf("fenced bail: output = %q, want exactly one %q (no duplicate from a re-run)", out.String(), "once\n")
	}
}

// The positive twin: the SAME runtime bail with NO effect before it still
// falls back silently to the interpreter with the correct result — the fence
// only blocks a re-run that would duplicate output.
func TestRuntimeBailBeforeEffectStillFallsBack(t *testing.T) {
	src := `def i (zz-inst) ; i.m 5 ; 42`
	a := zzShapedInstance(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(src)
	if err != nil {
		t.Fatalf("effect-free bail: err=%v; want the silent interpreter fallback", err)
	}
	if compiled {
		t.Error("effect-free bail: ran compiled; want the interpreter fallback")
	}
	if fmt.Sprint(got) != "[7 42]" {
		t.Errorf("effect-free bail: got %v, want [7 42] from the fallback", got)
	}
	if out.String() != "" {
		t.Errorf("effect-free bail: unexpected output %q", out.String())
	}
}

// --- the refusal arm --------------------------------------------------------

// zzCheckEmit registers `zz-emit`, a RunInCheckMode word that WRITES to the
// registry output when it executes — so the CHECK pass itself emits an
// observable effect, the way a module body printing at import time does.
func zzCheckEmit(a *Boru) {
	a.Register("zz-emit", native.Signature{
		Args:       []*native.Type{},
		Returns:    []*native.Type{},
		BarrierPos: -1,
		Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, reg *native.Registry) ([]native.Value, error) {
			fmt.Fprint(reg.Output, "E")
			return nil, nil
		}, native.RunInCheck()),
	})
}

// zzRefusingRow is an OFF-CORPUS refusing shape (a `def` consuming a
// variadic loop region whose count is DYNAMIC — the S5 first-value split
// needs the STATIC region size, so a runtime-only count keeps the refusal,
// making this the stable refusing fixture): it compiles to a nil Program
// with no check error, driving the refusal arm, and the interpreter runs it
// fine (binds xs to the region's first value and spills the rest —
// `[1 1 1]`). Every CORPUS refusal has graduated (the each variadic-if row
// was the last, 2026-07-15); the deferred-token class graduated 2026-07-16
// (the §2 rematch), and the statically-counted §5 shape graduated
// 2026-07-17 (the S5 split), so the fence pins ride this dynamic-count
// sibling.
const zzRefusingRow = `def m {n: 3} def xs (for (m get "n") [1]) xs`

// A genuine REFUSAL returns the compile_refused error (Stage J). The code is
// a GUARANTEE: no observable effect escaped the check pass, so a caller's
// explicit whole-source re-run (Run's own fallback, the CLI surfaces') is
// sound.
func TestRefusalReturnsCompileRefused(t *testing.T) {
	a := mustNew(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(zzRefusingRow)
	if codeOf(err) != "compile_refused" {
		t.Fatalf("refusal: err=[%s] %v (got=%v compiled=%v); want compile_refused (Stage J: no silent re-run)", codeOf(err), err, got, compiled)
	}
	if !strings.Contains(err.Error(), "consumes loop results") {
		t.Errorf("refusal error should carry the reason, got: %v", err)
	}
	if out.String() != "" {
		t.Errorf("refusal: output = %q, want none (no re-run)", out.String())
	}
}

// A refusal whose CHECK PASS already emitted an observable effect must NOT
// return compile_refused — the code's re-run guarantee would be a lie (the
// caller's fallback would duplicate the effect). It returns the fence's
// blocked-fallback internal_error, and the effect is emitted exactly once.
func TestRefusalAfterCheckEffectIsFenceBlocked(t *testing.T) {
	a := mustNew(t)
	zzCheckEmit(a)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(`zz-emit ; ` + zzRefusingRow)
	if codeOf(err) != "internal_error" || !strings.Contains(err.Error(), "emitted observable output") {
		t.Fatalf("effect-escaped refusal: err=[%s] %v (got=%v compiled=%v); want the fence-blocked internal_error", codeOf(err), err, got, compiled)
	}
	if codeOf(err) == "compile_refused" {
		t.Fatalf("effect-escaped refusal must never claim the re-run-is-sound code")
	}
	if out.String() != "E" {
		t.Errorf("effect-escaped refusal: output = %q, want exactly one %q", out.String(), "E")
	}
	// The public Run's explicit fallback honours the guarantee: it re-runs
	// only on compile_refused, so the fence-blocked error propagates and
	// the effect still fires exactly once.
	b := mustNew(t)
	zzCheckEmit(b)
	var outB bytes.Buffer
	b.SetOutput(&outB)
	if _, rerr := b.Run(`zz-emit ; ` + zzRefusingRow); codeOf(rerr) != "internal_error" {
		t.Fatalf("Run over an effect-escaped refusal: err=[%s] %v, want internal_error", codeOf(rerr), rerr)
	}
	if outB.String() != "E" {
		t.Errorf("Run over an effect-escaped refusal: output = %q, want exactly one %q", outB.String(), "E")
	}
}

// A refusal whose ONLY sentinel diagnostic is a CAUGHT one (a do-body failure
// the error handler recovers — severity info, CaughtAtRuntime) must NOT
// surface that diagnostic as the program's verdict when the fence blocks the
// fallback: the interpreter CONTINUES with the handler's result (the twin
// below pins [caught]), so the honest report is the blocked-fallback
// internal_error, like any other refusal the fence cannot resolve.
func TestCaughtDiagnosticRefusalReportsBlockedFallback(t *testing.T) {
	a := mustNew(t)
	zzCheckEmit(a)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(`zz-emit ; do [zz-missing-word-xyz] error ['caught']`)
	if codeOf(err) != "internal_error" {
		t.Fatalf("caught-diag refusal: err=[%s] %v (got=%v compiled=%v); want the blocked-fallback internal_error — a caught diagnostic is not the program's verdict", codeOf(err), err, got, compiled)
	}
	if !strings.Contains(err.Error(), "--no-compile") {
		t.Errorf("caught-diag refusal: error should carry the --no-compile note, got: %v", err)
	}
	if out.String() != "E" {
		t.Errorf("caught-diag refusal: output = %q, want exactly one %q", out.String(), "E")
	}
}

// The effect-free twin: the same caught-diagnostic refusal with no check-pass
// effect falls back silently and the program's REAL verdict is the handler's
// value — which is why the fenced arm above must not report the caught
// diagnostic as an error.
func TestCaughtDiagnosticRefusalWithoutEffectFallsBack(t *testing.T) {
	a := mustNew(t)
	got, compiled, err := a.RunCompiled(`do [zz-missing-word-xyz] error ['caught']`)
	if err != nil || compiled {
		t.Fatalf("caught-diag fallback: err=%v compiled=%v; want the silent interpreter fallback", err, compiled)
	}
	if fmt.Sprint(got) != "[caught]" {
		t.Errorf("caught-diag fallback: got %v, want [caught] from the handler", got)
	}
}

// A late effect from a PREVIOUS request's detached work (a ForkConcurrent
// body still printing) must not poison the CURRENT request's fence baseline:
// the ledger is per-request, and a stale worker holds the pointer it captured
// at its own fork/arm time. zz-stale-note stands in for that worker — it
// notes the ledger that was live BEFORE this request began, during the new
// request's check pass.
func TestStaleLedgerEffectDoesNotBlockFallback(t *testing.T) {
	a := mustNew(t)
	stale := a.registry.Effects
	a.Register("zz-stale-note", native.Signature{
		Args:       []*native.Type{},
		Returns:    []*native.Type{},
		BarrierPos: -1,
		Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			stale.Note()
			return nil, nil
		}, native.RunInCheck()),
	})

	// The probe rides the STATIC-ORACLE class (the caught-diag sentinel row
	// succeeds interpreted) — the class that KEEPS the bounded re-run post
	// Stage J, which is exactly the arm a poisoned baseline would block.
	got, compiled, err := a.RunCompiled(`zz-stale-note ; do [zz-missing-word-xyz] error ['caught']`)
	if compiled || err != nil {
		t.Fatalf("stale-note oracle re-run: compiled=%v err=%v; want the silent bounded fallback", compiled, err)
	}
	if fmt.Sprint(got) != "[caught]" {
		t.Errorf("stale-note oracle re-run: got %v, want [caught] — a stale ledger note must not block the bounded fallback", got)
	}
	if a.registry.Effects != stale {
		t.Error("the request must restore the prior ledger on return")
	}
}

// The write word is a production NoteEffect caller: a filesystem write counts
// on the ledger (here through the in-memory FS — it persists across a
// fallback re-run just like a disk file, so it counts identically), while the
// read-back does not.
func TestFileWriteNotesEffectLedger(t *testing.T) {
	a := mustNew(t)
	before := a.registry.Effects.Count()
	got, err := a.RunInterp(`import "boru:io"  context dot __sys dot fs set mem true  IO.write (make Pathon "mem://a.txt") "hi"  IO.read (make Pathon "mem://a.txt")`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || fmt.Sprint(got[len(got)-1]) != "hi" {
		t.Fatalf("write/read round trip = %v, want trailing \"hi\"", got)
	}
	if delta := a.registry.Effects.Count() - before; delta != 1 {
		t.Errorf("file write ledger delta = %d, want exactly 1 (the write counts, the read does not)", delta)
	}
}

// A STATIC check error after a check-pass effect surfaces AS ITSELF (the
// program is invalid in both engines; the interpreter re-run that would
// normally render the canonical error is blocked, but the check error is the
// truthful verdict) — never masked as a blocked-fallback internal_error.
func TestStaticErrorAfterCheckEffectSurfacesItself(t *testing.T) {
	a := mustNew(t)
	zzCheckEmit(a)
	var out bytes.Buffer
	a.SetOutput(&out)

	_, compiled, err := a.RunCompiled(`zz-emit ; zz-no-such-word-xyz`)
	if err == nil || compiled {
		t.Fatalf("fenced static error: err=%v compiled=%v; want the check error surfaced", err, compiled)
	}
	if codeOf(err) == "internal_error" {
		t.Errorf("fenced static error: masked as internal_error, want the genuine static diagnostic: %v", err)
	}
	if out.String() != "E" {
		t.Errorf("fenced static error: output = %q, want exactly one %q", out.String(), "E")
	}
}

// A check-pass ERROR (CompileCheck's err return, not the diagnostics
// sentinel) after a check-pass effect also surfaces AS ITSELF: zz-emit-fail
// is a RunInCheckMode word that writes and then errors, so the check pass
// both emits and fails — the fence must return the genuine error, not a
// blocked-fallback wrapper, and must not re-run the source.
func TestCheckErrorAfterCheckEffectSurfacesItself(t *testing.T) {
	a := mustNew(t)
	a.Register("zz-emit-fail", native.Signature{
		Args:       []*native.Type{},
		Returns:    []*native.Type{},
		BarrierPos: -1,
		Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, reg *native.Registry) ([]native.Value, error) {
			fmt.Fprint(reg.Output, "E")
			return nil, fmt.Errorf("zz-emit-fail: deliberate check-pass failure")
		}, native.RunInCheck()),
	})
	var out bytes.Buffer
	a.SetOutput(&out)

	_, compiled, err := a.RunCompiled(`zz-emit-fail`)
	if err == nil || compiled {
		t.Fatalf("fenced check error: err=%v compiled=%v; want the check error surfaced", err, compiled)
	}
	if !strings.Contains(err.Error(), "deliberate check-pass failure") {
		t.Errorf("fenced check error: want the genuine check error, got: %v", err)
	}
	if out.String() != "E" {
		t.Errorf("fenced check error: output = %q, want exactly one %q", out.String(), "E")
	}
}

// --- the foreign-error (non-BoruError) bail class ----------------------------

// zzForeignBoom registers `zz-boom`, a plain native whose handler returns a
// foreign Go error — the non-BoruError class runtimeShouldFallback also
// resolves by re-running the interpreter.
func zzForeignBoom(t *testing.T) *Boru {
	t.Helper()
	a := mustNew(t)
	a.Register("zz-boom", native.Signature{
		Args:       []*native.Type{},
		Returns:    []*native.Type{},
		BarrierPos: -1,
		Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			return nil, fmt.Errorf("zz-boom: foreign failure")
		}),
	})
	return a
}

// A compiled run that prints and then fails with a FOREIGN error must also be
// fenced: fenceBlockedFallback wraps the foreign text in an internal_error
// carrying the --no-compile note, with the output emitted exactly once.
func TestForeignErrorBailAfterEffectPropagates(t *testing.T) {
	a := zzForeignBoom(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	_, compiled, err := a.RunCompiled(`print "once" ; zz-boom`)
	if codeOf(err) != "internal_error" {
		t.Fatalf("fenced foreign bail: err=[%s] %v compiled=%v; want the wrapped internal_error", codeOf(err), err, compiled)
	}
	if !strings.Contains(err.Error(), "zz-boom: foreign failure") || !strings.Contains(err.Error(), "--no-compile") {
		t.Errorf("fenced foreign bail: error should carry the foreign text and the note, got: %v", err)
	}
	if out.String() != "once\n" {
		t.Errorf("fenced foreign bail: output = %q, want exactly one %q", out.String(), "once\n")
	}
}

// The positive twin: the same foreign failure with no prior effect still
// falls back silently, and the interpreter re-run surfaces the same foreign
// error as its own verdict.
func TestForeignErrorBailWithoutEffectFallsBack(t *testing.T) {
	a := zzForeignBoom(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	_, compiled, err := a.RunCompiled(`zz-boom`)
	if compiled {
		t.Error("effect-free foreign bail: ran compiled; want the interpreter fallback")
	}
	if err == nil || !strings.Contains(err.Error(), "zz-boom: foreign failure") {
		t.Errorf("effect-free foreign bail: err=%v; want the interpreter's own foreign error", err)
	}
	if out.String() != "" {
		t.Errorf("effect-free foreign bail: unexpected output %q", out.String())
	}
}

// The hatch twin: BORU_COMPILE_FALLBACK=1 restores the pre-Stage-J silent
// interpreter fallback for one release — the refused row then runs
// interpreted and yields its canonical interpreter result.
func TestRefusalHatchRestoresFallback(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	a := mustNew(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(zzRefusingRow)
	if compiled {
		t.Error("hatched refusal: ran compiled; want the interpreter fallback")
	}
	if err != nil {
		t.Errorf("hatched refusal: err=%v; want the silent interpreter fallback", err)
	}
	if fmt.Sprint(got) != "[1 1 1]" {
		t.Errorf("hatched refusal: got %v, want [1 1 1] from the fallback", got)
	}
	if out.String() != "" {
		t.Errorf("hatched refusal: unexpected output %q", out.String())
	}
}

// --- check-pass effect-freedom (obligation O1) ------------------------------

// The check pass over ordinary programs — value words, defs and calls, a
// module import, and a print in RUN position (check mode analyses it, never
// executes it) — must emit NO observable effect: this is what keeps the
// refusal arm's silent fallback sound for every corpus row, and what makes
// the C2 error-oracle run single-emission.
func TestCheckPassIsEffectFree(t *testing.T) {
	for _, src := range []string{
		`1 add 2`,
		`print "hello"`,
		`def f fn [[x:Integer] [Integer] [x mul 2]] f 21`,
		`import "boru:math-util" ; MathUtil.sqrt 16.0`,
		`def xs [1 2 3] xs each [dup mul]`,
	} {
		a := mustNew(t)
		var out bytes.Buffer
		a.SetOutput(&out)
		disarm := a.registry.ArmEffectFence()
		before := a.registry.Effects.Count()
		_, _, _, err := a.CompileCheck(src)
		after := a.registry.Effects.Count()
		disarm()
		if err != nil {
			t.Errorf("CompileCheck(%q): %v", src, err)
			continue
		}
		if after != before {
			t.Errorf("check pass over %q emitted %d observable effect(s) (output %q) — the check pass must be effect-free (O1)", src, after-before, out.String())
		}
	}
}
