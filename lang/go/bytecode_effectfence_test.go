package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// These were the C1 effect fence's pins. The fence permitted a silent
// interpreter re-run only while no observable effect had escaped since before
// the check pass, because RestoreForCompile rolls back registry scopes but
// cannot un-print: a re-run after an effect DUPLICATED it (the L-DUP class,
// design/legacy/VOXGIG-COMPILE-LEAVES.2.ignore — the whole trie smoke suite
// printed twice).
//
// Nothing re-runs the source any more, so there is no arm left to fence and
// the whole duplicate-effect class is gone by construction. What these tests
// pin now is the contract that replaced it: whatever the compiled lane hits —
// a compile failure, a runtime bail, a foreign error, before or after an
// effect — it reports it and stops. The effect fires exactly once because
// there is only ever one run.

// --- a runtime bail ---------------------------------------------------------

// A compiled run that hits a runtime internal_error (the zz-inst shape-claim
// violation from bytecode_methodshape_test.go) propagates the annotated
// internal_error. It did so only when an effect had already escaped; with the
// re-run gone it is unconditional, and the effect — when there is one — fires
// exactly once because there is only one run.
func TestRuntimeBailPropagatesAsADefect(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"after an effect", `print "once" ; def i (zz-inst) ; i.m 5 ; 42`, "once\n"},
		{"with no effect", `def i (zz-inst) ; i.m 5 ; 42`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := zzShapedInstance(t)
			var out bytes.Buffer
			a.SetOutput(&out)

			got, compiled, err := a.RunCompiled(c.src)
			if noteCompileDefect(t, c.src, got, err) {
				return
			}
			if codeOf(err) != "internal_error" {
				t.Fatalf("bail: err=[%s] %v (got=%v compiled=%v); want the propagated internal_error", codeOf(err), err, got, compiled)
			}
			if !strings.Contains(err.Error(), "compiler defect") {
				t.Errorf("bail: error should name itself a compiler defect, got: %v", err)
			}
			if out.String() != c.want {
				t.Errorf("bail: output = %q, want exactly %q", out.String(), c.want)
			}
		})
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

// A genuine REFUSAL returns the compile_failed error (Stage J). The code is
// a GUARANTEE: no observable effect escaped the check pass, so a caller's
// explicit whole-source re-run (Run's own fallback, the CLI surfaces') is
// sound.
func TestRefusalReturnsCompileRefused(t *testing.T) {
	a := mustNew(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(zzRefusingRow)
	if noteCompileDefect(t, zzRefusingRow, got, err) {
		return
	}
	if codeOf(err) != "compile_failed" {
		t.Fatalf("refusal: err=[%s] %v (got=%v compiled=%v); want compile_failed (Stage J: no silent re-run)", codeOf(err), err, got, compiled)
	}
	if !strings.Contains(err.Error(), "consumes loop results") {
		t.Errorf("refusal error should carry the reason, got: %v", err)
	}
	if out.String() != "" {
		t.Errorf("refusal: output = %q, want none (no re-run)", out.String())
	}
}

// A compile failure whose CHECK PASS already emitted an observable effect is
// still a compile failure. It used to be reported as a fence-blocked
// internal_error, because compile_failed carried a promise that a caller's
// whole-source re-run was sound and that promise would have been a lie. No
// caller re-runs anything now, so the code makes no promise and the honest
// report is the defect itself — with the effect emitted exactly once.
func TestCompileFailureAfterCheckEffectStillReportsTheDefect(t *testing.T) {
	a := mustNew(t)
	zzCheckEmit(a)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(`zz-emit ; ` + zzRefusingRow)
	if noteCompileDefect(t, `zz-emit ; `+zzRefusingRow, got, err) {
		return
	}
	if codeOf(err) != "compile_failed" {
		t.Fatalf("effect-escaped compile failure: err=[%s] %v (got=%v compiled=%v); want compile_failed", codeOf(err), err, got, compiled)
	}
	if out.String() != "E" {
		t.Errorf("effect-escaped compile failure: output = %q, want exactly one %q", out.String(), "E")
	}
	// Run is the same entry point with the same one outcome.
	b := mustNew(t)
	zzCheckEmit(b)
	var outB bytes.Buffer
	b.SetOutput(&outB)
	if _, rerr := b.Run(`zz-emit ; ` + zzRefusingRow); codeOf(rerr) != "compile_failed" {
		t.Fatalf("Run over an effect-escaped compile failure: err=[%s] %v, want compile_failed", codeOf(rerr), rerr)
	}
	if outB.String() != "E" {
		t.Errorf("Run over an effect-escaped compile failure: output = %q, want exactly one %q", outB.String(), "E")
	}
}

// A program whose only blocking diagnostic is a CAUGHT one (a do-body
// failure the error handler recovers — severity info, CaughtAtRuntime) is
// VALID: the interpreter continues with the handler's result. It bought a
// silent interpreter run for exactly that reason. It does not compile, so it
// is now a compile defect and says so — with or without a check-pass effect.
// The defect is real and owed a fix; what it is not is a second outcome.
func TestCaughtDiagnosticIsACompileDefect(t *testing.T) {
	const src = `do [zz-missing-word-xyz] error ['caught']`
	for _, c := range []struct{ name, pre, want string }{
		{"after a check-pass effect", `zz-emit ; `, "E"},
		{"with no effect", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := mustNew(t)
			zzCheckEmit(a)
			var out bytes.Buffer
			a.SetOutput(&out)

			got, compiled, err := a.RunCompiled(c.pre + src)
			if noteCompileDefect(t, c.pre+src, got, err) {
				return
			}
			if codeOf(err) != "compile_failed" {
				t.Fatalf("caught diagnostic: err=[%s] %v (got=%v compiled=%v); want compile_failed", codeOf(err), err, got, compiled)
			}
			if out.String() != c.want {
				t.Errorf("caught diagnostic: output = %q, want %q", out.String(), c.want)
			}
			// The interpreter still answers it — the program is valid, which
			// is what makes the compile failure a defect rather than a verdict.
			b := mustNew(t)
			if gotI, errI := b.RunInterp(c.pre + src); errI != nil || fmt.Sprint(gotI) != "[caught]" {
				t.Errorf("interpreted: got %v / %v, want [caught] — the program is valid", gotI, errI)
			}
		})
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
	if noteCompileDefect(t, `zz-emit ; zz-no-such-word-xyz`, nil, err) {
		return
	}
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
	if noteCompileDefect(t, `zz-emit-fail`, nil, err) {
		return
	}
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

// A FOREIGN error out of a native handler is the same class: not a boru
// error, so not the program's own verdict — it is wrapped in an
// internal_error carrying the foreign text and the defect note, before or
// after an effect, and the effect fires exactly once.
func TestForeignErrorBailPropagatesAsADefect(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"after an effect", `print "once" ; zz-boom`, "once\n"},
		{"with no effect", `zz-boom`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := zzForeignBoom(t)
			var out bytes.Buffer
			a.SetOutput(&out)

			_, compiled, err := a.RunCompiled(c.src)
			if noteCompileDefect(t, c.src, nil, err) {
				return
			}
			if codeOf(err) != "internal_error" {
				t.Fatalf("foreign bail: err=[%s] %v compiled=%v; want the wrapped internal_error", codeOf(err), err, compiled)
			}
			if !strings.Contains(err.Error(), "zz-boom: foreign failure") || !strings.Contains(err.Error(), "compiler defect") {
				t.Errorf("foreign bail: error should carry the foreign text and the defect note, got: %v", err)
			}
			if out.String() != c.want {
				t.Errorf("foreign bail: output = %q, want exactly %q", out.String(), c.want)
			}
		})
	}
}

// --- check-pass effect-freedom (obligation O1) ------------------------------

// The check pass over ordinary programs — value words, defs and calls, a
// module import, and a print in RUN position (check mode analyses it, never
// executes it) — must emit NO observable effect. It is no longer what keeps a
// re-run sound, because nothing re-runs; it is the plain obligation that
// ANALYSING a program does not run it, and a program's output is emitted by
// its run and only by its run.
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
