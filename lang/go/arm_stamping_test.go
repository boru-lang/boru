package lang

import (
	"fmt"
	"testing"
)

// storeSiteSrc constructs a service handler at runtime — the canonical
// detached-stamp trigger (compiler.StampDetachedFn fires at the store site).
const storeSiteSrc = `def svc (service {}) add {cmd:"X"} ([req:Map state:Any] => [ 42 ]) svc (call {cmd:"X"} svc)`

// ArmRuntimeStamping is the caller-side half of the Stage-J explicit
// fallback: an explicit RunInterp run stamps stored callbacks ONLY while
// armed, the disarm func restores the prior state, and arming an
// already-armed instance is a no-op whose disarm must not disarm the
// outer arming.
func TestArmRuntimeStamping(t *testing.T) {
	a := mustNew(t)

	// Unarmed baseline: a plain interpreter run records no stamp attempts.
	if _, err := a.RunInterp(storeSiteSrc); err != nil {
		t.Fatalf("baseline run: %v", err)
	}
	if got := a.StampReport(); len(got) != 0 {
		t.Fatalf("unarmed RunInterp must not stamp, got %+v", got)
	}

	// Armed: the same run stamps the handler at its store site.
	disarm := a.ArmRuntimeStamping()
	// Nested arming is a no-op; its disarm must leave the outer arming live.
	innerDisarm := a.ArmRuntimeStamping()
	innerDisarm()
	if _, err := a.RunInterp(storeSiteSrc); err != nil {
		t.Fatalf("armed run: %v", err)
	}
	armed := len(a.StampReport())
	if armed == 0 {
		t.Fatalf("armed RunInterp recorded no stamp attempts")
	}
	disarm()

	// Disarmed again: no further attempts accumulate.
	if _, err := a.RunInterp(storeSiteSrc); err != nil {
		t.Fatalf("post-disarm run: %v", err)
	}
	if got := len(a.StampReport()); got != armed {
		t.Fatalf("disarm did not restore: %d attempts, want %d", got, armed)
	}
}

// RunInterpValues is the raw-Value twin of RunInterp (the REPL's fallback
// renderer needs Value.String()); it must agree with RunInterp on results
// and propagate errors identically.
func TestRunInterpValues(t *testing.T) {
	a := mustNew(t)
	vals, err := a.RunInterpValues(`1 add 2 "hi"`)
	if err != nil {
		t.Fatalf("RunInterpValues: %v", err)
	}
	b := mustNew(t)
	out, err := b.RunInterp(`1 add 2 "hi"`)
	if err != nil {
		t.Fatalf("RunInterp: %v", err)
	}
	if fmt.Sprint(convertResults(vals)) != fmt.Sprint(out) {
		t.Fatalf("parity: values=%v interp=%v", vals, out)
	}
	if len(vals) != 2 || vals[0].String() != "3" || vals[1].String() != "'hi'" {
		t.Fatalf("raw values = %v", vals)
	}

	if _, err := a.RunInterpValues(`zz-missing-word-xyz`); err == nil {
		t.Fatalf("undefined word must error")
	} else if codeOf(err) == "compile_failed" {
		t.Fatalf("RunInterpValues must never report compile_failed: %v", err)
	}
}
