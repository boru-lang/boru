package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestEmptyBodyFnLowers — a user fn declaring no returns and an empty body
// (`def v fn [[x:Integer] [] []]`) produces ZERO values at run time. Stage 3's
// check-mode ReturnsFn used to return a bogus single Any approximation and skip
// recording, so any call refused "user fn call (Stage 3)". It now records a
// 0-output CALL_USER and returns no carriers, so the residual matches runtime:
// the call runs for its effects and the next token is the residual.
func TestEmptyBodyFnLowers(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`def v fn [[x:Integer] [] []] v 1 99`, "[99]"},        // void call, 99 residual
		{`def v fn [[x:Integer] [] []] v 1`, "[]"},             // void call, empty residual
		{`def v fn [[x:Integer] [] []]  v 1  v 2  77`, "[77]"}, // two void calls
	}
	for _, c := range cases {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q did not compile: reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q islanded; expected a native CALL_USER", c.src)
		}
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		d, _ := New()
		gotI, _ := d.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: parity broke: compiled=%v gotC=%v errC=%v gotI=%v want=%s", c.src, compiled, gotC, errC, gotI, c.want)
		}
	}
}

// TestEmptyBodyFnVoidConsumed — a void fn's (absent) result reaching a consumer
// that REQUIRES a value errors identically in both modes. The 0-value return now
// surfaces the diagnostic at check time (the program falls back and the
// interpreter raises the same error), so the error taxonomy stays in parity.
func TestEmptyBodyFnVoidConsumed(t *testing.T) {
	for _, c := range []struct {
		src  string
		code string
	}{
		{`def f fn [[x:Integer] [] []] def r (f 1)`, "def_error"},
		{`def f fn [[x:Integer] [] []]  add 5 (f 1)`, "no_value_error"},
	} {
		b, _ := New()
		_, _, errC := b.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, nil, errC) {
			continue
		}
		d, _ := New()
		_, errI := d.RunInterp(c.src)
		if errC == nil || errI == nil {
			t.Errorf("%q: expected an error in both modes: compiled=%v interp=%v", c.src, errC, errI)
			continue
		}
		if !strings.Contains(fmt.Sprint(errC), c.code) || !strings.Contains(fmt.Sprint(errI), c.code) {
			t.Errorf("%q: error taxonomy diverged: compiled=%v interp=%v want %s", c.src, errC, errI, c.code)
		}
	}
}
