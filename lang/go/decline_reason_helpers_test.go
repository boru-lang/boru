package lang

import (
	"strings"
	"testing"
)

// dynRegionNotLastReason is the substring of compiler lower.go
// dynRegionNotLast, the region rule's decline for entries above a computed
// run that may leave a callable.
const dynRegionNotLastReason = "seat only as the residual's last entries"

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
