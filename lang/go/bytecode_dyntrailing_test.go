package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestEmitTrailingFnValueApply: a fn value that TRAILS its argument (`5 m.f` —
// the fn `m.f` produces auto-applies to the 5 beneath it) compiles to
// CALL_DYNAMIC_TRAILING (no fallback) and matches the interpreter. The leading
// form `(mk2 5) 10` uses plain CALL_DYNAMIC; this is its mirror. The negative
// half pins the one-arg bound: the 2-arg mixed form `3 m.f 2` must NOT take the
// trailing-apply path (forward+stack collection would reorder the args).
func TestEmitTrailingFnValueApply(t *testing.T) {
	const src = `def m {f: (fn [[a:Integer][Integer][a add 1]])}  5 m.f`
	a, _ := New()
	prog, _, _, _ := a.CompileCheck(src)
	if prog == nil {
		t.Fatalf("%q: must compile via trailing apply, but declined", src)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("%q: must compile native (trailing apply), not island:\n%s", src, dis)
	}
	if !strings.Contains(dis, "CALL_DYNAMIC_TRAILING") {
		t.Errorf("%q: expected CALL_DYNAMIC_TRAILING:\n%s", src, dis)
	}
	ar, _ := New()
	gotC, compiled, errC := ar.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	b, _ := New()
	gotI, _ := b.RunInterp(src)
	if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[6]" {
		t.Errorf("%q: trailing-apply parity broke: compiled=%v gotC=%v interp=%v err=%v", src, compiled, gotC, gotI, errC)
	}

	// NEGATIVE: the 2-arg mixed form is beyond the one-arg bound, so it must not
	// compile to a trailing apply — it does not compile faithfully.
	const mixed = `def m {f: (fn [[a:Integer b:Integer][Integer][(a mul 100) add b]])}  3 m.f 2`
	m, _ := New()
	if mp, _, _, _ := m.CompileCheck(mixed); mp != nil && strings.Contains(mp.Disassemble(), "CALL_DYNAMIC_TRAILING") {
		t.Errorf("%q: 2-arg mixed form must not use the trailing apply (out of the one-arg bound)", mixed)
	}
}
