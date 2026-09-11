package lang

import (
	"fmt"
	"strings"
	"testing"
)

// uncalled_dispatch_trap_test.go is the whole-program half of the
// forty-ninth increment: a NAMED fn value reached as a call whose match
// failed — the `uncalled_function` finding of design/FN-VALUE-DISPATCH.0.md.
//
// The site's own note used to read "NOT a RuntimeMirror … there is no call
// here to compile". Every clause of that is true; the conclusion is not,
// because a mirror needs something that RAISES IDENTICALLY, not a call. On
// the uncaught top line, over operands the runtime match examines unchanged,
// that something is a terminal OpTrap carrying the interpreter's own error.

// udRun runs a source on both lanes and reports whether the compiled lane
// ran natively and whether the two agree byte for byte — values AND error
// text, which for these rows is the entire claim.
func udRun(t *testing.T, src string) (ran bool, agree bool, cerr error) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotC, ran, cerr := a.RunCompiled(src)
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotI, ierr := b.RunInterp(src)
	agree = fmt.Sprint(gotC) == fmt.Sprint(gotI) && fmt.Sprint(cerr) == fmt.Sprint(ierr)
	return ran, agree, cerr
}

// TestUncalledDispatchTrapRaisesByteIdentically — the two ledger rows
// (`FnUtil.flip 5`, `FnUtil.curry 5`) and the shape behind them. Each used
// to refuse the WHOLE program with the generic "check diagnostics"; each now
// compiles to a trap that raises the interpreter's own rich error — caret,
// span and `/v` hint included.
func TestUncalledDispatchTrapRaisesByteIdentically(t *testing.T) {
	for _, tc := range []struct{ src, word string }{
		{`import "boru:fn-util"  FnUtil.flip 5`, "flip"},
		{`import "boru:fn-util"  FnUtil.curry 5`, "curry"},
		// A module native over a disjoint concrete argument, and the same
		// row with a prefix before it: the trap is terminal, but it is
		// terminal AT THE CALL, not at the top of the program.
		{`import "boru:math-util"  MathUtil.cbrt 'x'`, "cbrt"},
		{`import "boru:math-util"  def zq 7 end MathUtil.cbrt 'x'`, "cbrt"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			ran, agree, cerr := udRun(t, tc.src)
			if !ran {
				t.Fatalf("the definite uncalled dispatch must compile to a trap, not refuse: %v", cerr)
			}
			if cerr == nil {
				t.Fatal("the dispatch must still raise")
			}
			if !strings.Contains(cerr.Error(), "call to '"+tc.word+"' matched no signature") {
				t.Errorf("error drifted: %v", cerr)
			}
			if !strings.Contains(cerr.Error(), tc.word+"/v to push the function as a value") {
				t.Errorf("the interpreter's hint must ride on the trap: %v", cerr)
			}
			if !agree {
				t.Error("the compiled raise must be byte-identical to the interpreter's")
			}
		})
	}
}

// TestUncalledDispatchTrapEmitsTheTerminalTrap — the emitted shape, so a
// future lowering change cannot quietly turn the trap into an island or a
// fallback and keep this file green on answers alone.
func TestUncalledDispatchTrapEmitsTheTerminalTrap(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(`import "boru:math-util"  MathUtil.cbrt 'x'`)
	if cerr != nil {
		t.Fatal(cerr)
	}
	if prog == nil {
		t.Fatalf("must compile: %q", reason)
	}
	dis := prog.Disassemble()
	if !strings.Contains(dis, "TRAP") {
		t.Errorf("no terminal trap emitted:\n%s", dis)
	}
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("the trap must not carry an interpreter island:\n%s", dis)
	}
}

// TestUncalledDispatchTrapDeclinesInexactOperands — the negatives, and the
// reason the screen is narrow. Each row's failed match examined something
// the check pass does not hold exactly, so the trap declines and the
// whole-program refusal stands; both lanes still agree, through the
// interpreter.
func TestUncalledDispatchTrapDeclinesInexactOperands(t *testing.T) {
	for _, src := range []string{
		// A raw WORD token stands for whatever the binding holds at run time.
		`import "boru:math-util"  def zs 'x' end MathUtil.cbrt zs`,
		// A DYNAMIC flex read has a static tag, not a value.
		`import "boru:math-util"  def zf (flex {n:'x'}) end MathUtil.cbrt (zf get 'n')`,
	} {
		t.Run(src, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			prog, reason, _, cerr := a.CompileCheck(src)
			if cerr != nil {
				t.Fatal(cerr)
			}
			if prog != nil {
				t.Fatalf("an inexact operand must keep the refusal; it compiled:\n%s", prog.Disassemble())
			}
			if reason != "check diagnostics" {
				t.Errorf("refusal reason drifted: %q", reason)
			}
			ran, agree, _ := udRun(t, src)
			if ran {
				t.Error("the refused program must run on the interpreter")
			}
			if !agree {
				t.Error("the fallback answer must match the interpreter's")
			}
		})
	}
}

// TestUncalledDispatchPlainCheckStillReportsTheError — the trap is a COMPILE
// decision. `boru check` never compiles, so it still reports the finding at
// error severity: the diagnostic's content is unchanged in every lane, only
// its RuntimeMirror stamp moves, and only on a compile pass.
func TestUncalledDispatchPlainCheckStillReportsTheError(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	diags, err := a.Check(`import "boru:math-util"  MathUtil.cbrt 'x'`)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range diags.Diagnostics {
		if d.Code == "uncalled_function" && d.Severity == SeverityError {
			found = true
		}
	}
	if !found {
		t.Errorf("the plain check must still report uncalled_function as an error: %+v", diags.Diagnostics)
	}
}
