package lang

import (
	"strings"
	"testing"
)

// hasLockedSigMisfire reports whether the compile pass emitted the spurious
// `locked_signature` compile failure — the bug both fixes below target.
func hasLockedSigMisfire(res CheckResult) bool {
	for _, d := range res.Diagnostics {
		if d.Severity == "error" && strings.Contains(d.Detail, "locked_signature") {
			return true
		}
	}
	return false
}

// FIX #2 (root): an explicitly-`Any` handler param binds a GRADUAL carrier on
// the stored-handler compile path (fnValueInputs → ParamInputCarrier), so a
// module socket-word dispatch over it resolves optimistically — the connection
// value IS a Socket at runtime. The echo handler with `[sock:Any]` therefore
// compiles its body to a VM unit exactly as the `[sock:Socket]` form does, with
// no `locked_signature` compile failure. Before the fix the strict-`Any` carrier failed
// the Socket slot, left `Net.recv-until` as a FailedDispatch partial, and
// (inside the `for` loop) misfired the open-words merge.
func TestAnyHandlerParamCompilesLikeSocket(t *testing.T) {
	body := `[
	  def nl (convert Bytes "\n")
	  for 3 [
	    def line (Net.recv-until sock nl)
	    Net.send-bytes (convert Bytes (join "" [(convert String line) "\n"])) sock
	  ]
	]`
	for _, ty := range []string{"Any", "Socket"} {
		a, _ := New()
		src := `import "boru:net"
def ln (Net.serve-raw {tcp: 0} (fn [[sock:` + ty + `] [Any] ` + body + `]))`
		prog, reason, res, err := a.CompileCheck(src)
		if err != nil {
			t.Fatalf("[%s] compile error: %v", ty, err)
		}
		if hasLockedSigMisfire(res) {
			t.Fatalf("[%s] spurious locked_signature compile failure", ty)
		}
		if prog == nil {
			t.Fatalf("[%s] whole-program compilation declined (reason=%q)", ty, reason)
		}
		if firstStampedHandler(prog) == nil {
			t.Fatalf("[%s] handler body did not compile to a VM unit", ty)
		}
	}
}

// FIX #1 (defense-in-depth): a value whose PRODUCING call genuinely fails to
// dispatch — a concrete type mismatch the gradual path does not rescue, here a
// `Bytes` fed to `Net.recv-until`'s `Socket` slot — is left on the stack as a
// FailedDispatch Function value that `def` consumes. Re-analysed inside a `for`
// loop it must NOT be misread as a locked-signature word extension: a failed
// dispatch left as data is never a deliberate `def <word> fn […]`, so
// defWordExtension falls through to a plain value binding rather than declining
// with `locked_signature`. (The program still declines to compile for the real
// reason — the dispatch didn't resolve — but never with the spurious merge
// error.)
func TestDefBoundFailedDispatchNotWordExtension(t *testing.T) {
	a, _ := New()
	src := `import "boru:net"
def nl (convert Bytes "\n")
for 3 [ def y (Net.recv-until nl nl) y ]`
	_, _, res, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	if hasLockedSigMisfire(res) {
		t.Fatal("spurious locked_signature on a def-bound failed dispatch")
	}
}
