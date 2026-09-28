package lang

import (
	"strings"
	"testing"
)

// keptDefsLatchReason is the substring of compiler kept_defs.go
// poisonKeptDefs, the kept-defs latch's decline of a read after a computed
// body that may have changed the binding.
const keptDefsLatchReason = "a computed body keeps its defs and undefs in the enclosing scope"

// requireDeclineReason asserts src declines at compile time with a reason
// holding wantReason, the compiled lane fails loudly as compile_failed, and
// the interpreter runs it (to a value or an error of its own).
func requireDeclineReason(t *testing.T, src, wantReason string) {
	t.Helper()
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog != nil || err != nil || !strings.Contains(reason, wantReason) {
		t.Errorf("%q: want a compile decline naming %q, got prog=%v reason=%q err=%v", src, wantReason, prog != nil, reason, err)
		return
	}
	if _, _, errC := mustNew(t).RunCompiled(src); codeOf(errC) != "compile_failed" {
		t.Errorf("%q: the compiled lane must fail loudly as compile_failed, got %v", src, errC)
	}
}
