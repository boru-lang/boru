package lang

import (
	"testing"
)

// uncalled_dispatch_trap_test.go is the whole-program half of the
// forty-ninth increment: a NAMED fn value reached as a call whose match
// failed — the `uncalled_function` finding of design/legacy/FN-VALUE-DISPATCH.0.ignore.
//
// The site's own note used to read "NOT a RuntimeMirror … there is no call
// here to compile". Every clause of that is true; the conclusion is not,
// because a mirror needs something that RAISES IDENTICALLY, not a call. On
// the uncaught top line, over operands the runtime match examines unchanged,
// that something is a terminal OpTrap carrying the interpreter's own error.

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
