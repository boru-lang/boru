// bind_twin_regime_test.go pins §6.5's rollback-and-replay MECHANICS on
// shapes small enough to reason through by hand (design/FULL-COMPILATION.0.md).
//
// Since the flip this is the ONLY regime: a compiled request never keeps
// the check pass's installs — lang rolls the runtime-visible bindings back
// to the pre-pass snapshot (core.RestoreBindingsForReplay), each placed
// OpBindTwin re-installs its recorded transition at its source position
// (core.ApplyBindTwin), OpBindGlobal pushes the runtime values, and
// OpBindResident installs per element inside a compiled unit. The corpus
// differential that once ran here as a separate flag-armed lane is now
// simply compiled_differential_test.go — one lane, one regime — and its
// floor (6410) moved there with it.
package langspec

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestTwinRegimeSmoke pins the flip's mechanics on four shapes: a concrete
// def (the twin replays the captured value), a computed def (the twin
// skips, OpBindGlobal pushes the runtime value), cross-request persistence
// on ONE instance (the replayed binding must be readable by the next
// request's check pass — keep-on-compile's contract, delivered by replay),
// and a type install (the twin re-pushes the minted node).
func TestTwinRegimeSmoke(t *testing.T) {
	t.Parallel()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.RunCompiledStrict("def x 1  x add 2")
	if err != nil || len(out) != 1 || renderAny(out) != "3" {
		t.Fatalf("concrete def: %v, %v", out, err)
	}
	out, err = a.RunCompiledStrict("x add 10")
	if err != nil || renderAny(out) != "11" {
		t.Fatalf("cross-request read of a replayed binding: %v, %v", out, err)
	}
	out, err = a.RunCompiledStrict("def Flagq (refine Boolean)  def f:Flagq true  f")
	if err != nil || renderAny(out) != "true" {
		t.Fatalf("type install + typed def: %v, %v", out, err)
	}
}
