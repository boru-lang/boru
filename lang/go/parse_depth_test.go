package lang_test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// End-to-end guard: deeply nested source reaching the public API must surface a
// clean error, never crash the host process with a Go stack overflow (Finding 1
// of the bytecode-compiler resource review). Before the parser depth guard, the
// `if`-nested input below killed the test binary with `fatal error: stack
// overflow`, which recover() cannot catch.
func TestDeeplyNestedSourceDoesNotCrash(t *testing.T) {
	src := strings.Repeat("if true [", 200000) + "1" + strings.Repeat("]", 200000)

	a, err := lang.New(lang.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RunCompiled panicked on deeply nested source: %v", r)
		}
	}()

	out, compiled, rerr := a.RunCompiled(src)
	if lang.NoteCompileDefect(t, src, out, rerr) {
		return
	}
	if rerr == nil {
		t.Fatalf("deeply nested source returned no error (out=%v compiled=%v)", out, compiled)
	}
	if !strings.Contains(rerr.Error(), "evaluation_limit") &&
		!strings.Contains(rerr.Error(), "nesting") {
		t.Fatalf("error %q is not the nesting-depth diagnostic", rerr)
	}
}

// Negative control: a normally-nested program still runs and agrees on both
// engines, so the depth guards (parser + lowerer) do not regress legitimate
// programs. 40 nested ifs is far under both ceilings.
func TestModeratelyNestedProgramStillRuns(t *testing.T) {
	src := strings.Repeat("if true [", 40) + "7" + strings.Repeat("]", 40)

	a, err := lang.New(lang.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.RunInterp(src)
	if err != nil {
		t.Fatalf("interpreter rejected a 40-deep nested program: %v", err)
	}
	if len(got) != 1 || got[0] != int64(7) {
		t.Fatalf("nested if = %v, want [7]", got)
	}

	b, err := lang.New(lang.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cgot, _, cerr := b.RunCompiled(src)
	if lang.NoteCompileDefect(t, src, cgot, cerr) {
		return
	}
	if cerr != nil {
		t.Fatalf("compiled path rejected a 40-deep nested program: %v", cerr)
	}
	if len(cgot) != 1 || cgot[0] != int64(7) {
		t.Fatalf("compiled nested if = %v, want [7]", cgot)
	}
}
