package lang

import (
	"strings"
	"testing"
)

// A check-pass narrowing push (narrowDynamicUses — a dynamic binding
// tightened at a typed use) is an analysis artifact: the same runtime value
// under a tighter static bound. Before the pass-end cleanup it stayed in the
// registry after the pass, SHADOWING the real binding for every later Run on
// the same instance: this program's `IO.unlock l` narrowed `l`, and a later
// read of `l` answered the leaked `dynamic(Ideal)` carrier instead of the
// bound Lock. The live-depth oracle surfaced it as its one non-macro
// mismatch (`module-io.tsv:223` — ledger 1, live 2).
func TestNarrowingPushDoesNotLeakAcrossRuns(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	src := `import "boru:io"  context dot __sys dot fs set mem true  ` +
		`IO.write (make Pathon "mem://f") "d" drop  ` +
		`def l (IO.lock (make Pathon "mem://f"))  IO.unlock l`
	// The binding state this test is about is what the RUN leaves behind, so
	// it is driven on the reference engine: the program does not compile
	// today, and that failure is booked as the defect it is.
	gotC, cErr := a.Run(src)
	noteCompileDefect(t, src, gotC, cErr)
	if _, err := a.RunInterp(src); err != nil {
		t.Fatal(err)
	}
	if d := a.NativeRegistry().Defs.Depth("l"); d != 1 {
		t.Fatalf("`l` must hold exactly the def's binding after the run: depth = %d, want 1", d)
	}
	out, err := a.RunInterp(`typeof l`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("typeof l returned %v", out)
	}
	got, ok := out[0].(string)
	if !ok || !strings.Contains(got, "Lock") {
		t.Fatalf("a later read of `l` must resolve the bound Lock, not a leaked "+
			"analysis carrier: typeof l = %v", out[0])
	}
}
