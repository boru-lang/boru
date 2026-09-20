package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestCatchFrame: a closure body that ALWAYS raises (`raise`, marked
// CompileDiverges) is DIVERGENT — it never returns normally, so it compiles
// with no RET; at run time the error propagates out of the VM and the catching
// word (`do` / `error`) turns it into the Error value via InvokeBody, exactly as
// the interpreter does. This is the bytecode side of structured exception
// handling: a trapping closure body is catchable rather than uncompilable.
//
// `error` was also collapsed to ONE (List, Any) sig with a runtime IsError
// branch — a static TError-vs-TAny pick was unsound for the compiled path (the
// checker types `do [raise …]` as Any and would bake the pass-through even when
// the runtime value is an Error). The mono dispatch lets the handler body
// compile as a closure while staying correct on both the error and value paths.
func TestCatchFrame(t *testing.T) {
	// POSITIVE — a raising do-body compiles native (was an island) and `error`
	// handlers that CONSUME the error compile native, all parity-matched.
	pos := []struct{ src, want string }{
		{`do [raise "boom"]`, "[error(boom)]"},                  // caught → Error value
		{`do [21 mul 2]`, "[42]"},                               // non-raising body still nets its value
		{`do [raise "boom"] error [dot message] end`, "[boom]"}, // handler reads the message
		{`do [raise {code: too_big/q, message: "out of range", limit: 99}] error [case [[dot code eq bad_input/q] [dot message] [dot code eq too_big/q] [dot limit] "unexpected"]]`, "[99]"}, // predicate-clause case handler
		// A caught DYNAMIC error (a getr-miss raises not_found, statically Any):
		// the error reaches `error [handler]` as a dynamic value, but its closure
		// input is a FIXED TError carrier independent of that value, so the handler
		// runs faithfully — RecordClosureCall resolves the dynamic input as a stack
		// operand rather than declining it outright.
		{`do [{x:1} !. y] error [dot code]`, "[not_found]"},
	}
	for _, c := range pos {
		a, _ := New()
		prog, reason, _, _ := a.CompileCheck(c.src)
		if prog == nil {
			t.Errorf("%q: must compile (catch frame), but declined: %q", c.src, reason)
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

	// NEGATIVE — shapes that DON'T take the closure path must still fall back
	// faithfully (parity), never miscompile:
	//   - an IGNORE-the-error handler (`["fallback"]`) leaves error+result (nets
	//     2 ≠ BodyOut 1), so the closure declines and the CompileFallbackBody
	//     island owns it (the strip runs there).
	//   - a `get code case [value-clauses]` handler has a COMPUTED scrutinee the
	//     fn-unit value-def-locals promotion cannot seat yet, so `error` declines
	//     and the whole statement falls back.
	neg := []struct{ src, want string }{
		{`do [21 mul 2] error ["fallback"]`, "[42]"},
		{`do [raise bad_input "nope"] error [dot code case [bad_input/q "rejected" io_error/q "retry" "unexpected"]]`, "[rejected]"},
	}
	for _, c := range neg {
		ar, _ := New()
		gotC, _, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, errI := b.RunInterp(c.src)
		if (errC == nil) != (errI == nil) || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: fallback parity broke: gotC=%v errC=%v gotI=%v errI=%v want=%s", c.src, gotC, errC, gotI, errI, c.want)
		}
	}
}
