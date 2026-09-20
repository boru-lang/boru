package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestCaseInClosure: value-def-locals now run for EVERY compiled unit, not just
// the top-level program — so a COMPUTED scrutinee read across a desugared `case`
// if-chain (`get code case […]`) seats as a frame local inside a fn unit too.
// This is what lets the `error [get code case […]]` handlers (the bulk of the
// error cluster) compile as closures: previously CanSeatAcrossFragment bailed in
// any fn unit, so the case islanded and `error` fell back.
//
// Positive: the four error-code dispatch handlers compile native (no FALLBACK)
// and match the interpreter. No-regression: a plain closure body (filter/each)
// AND a case-in-closure over a computed scrutinee both stay parity-correct — the
// promotion must not perturb the common single-use closure body.
func TestCaseInClosure(t *testing.T) {
	const clause = ` error [dot code case [bad_input/q "rejected" io_error/q "retry" "unexpected"]]`
	pos := []struct{ src, want string }{
		{`do [raise bad_input "nope"]` + clause, "[rejected]"},
		{`do [raise io_error "disk"]` + clause, "[retry]"},
		{`do [raise weird "?"]` + clause, "[unexpected]"},
		{`do [21 mul 2] error [dot code case [bad_input/q "rejected" "unexpected"]]`, "[42]"}, // success → pass-through
		// a computed scrutinee inside an each-body closure (not via error):
		{`[2 4] each [dup add case [4 "four" 8 "eight" "other"]]`, "[['four' 'eight']]"},
	}
	for _, c := range pos {
		a, _ := New()
		prog, reason, _, _ := a.CompileCheck(c.src)
		if prog == nil {
			t.Errorf("%q: must compile (case-in-closure), but declined: %q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile native (no island):\n%s", c.src, prog.Disassemble())
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, errI := b.RunInterp(c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: parity broke: compiled=%v gotC=%v errC=%v gotI=%v want=%s", c.src, compiled, gotC, errC, gotI, c.want)
		}
	}

	// NO-REGRESSION — the common single-use closure body (no cross-fragment read)
	// must be unaffected by the per-unit promotion pass.
	for _, c := range []struct{ src, want string }{
		{`[1 2 3] filter [0 gte]`, "[[1 2 3]]"},
		{`[1 2 3] each [dup add]`, "[[2 4 6]]"},
	} {
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, errI := b.RunInterp(c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: closure regression: compiled=%v gotC=%v errC=%v gotI=%v want=%s", c.src, compiled, gotC, errC, gotI, c.want)
		}
	}
}
