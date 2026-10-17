package lang

import (
	"fmt"
	"testing"

	"github.com/boru-lang/boru/lang/go/policy"
)

// Per-export module gating (NUR045). A profile's per-module subscope
// rules — `modules.scopes."boru:time-util".words: deny ["sleep"]` — are
// enforced at dispatch, on BOTH engines, through the identity stamped
// onto a module's signatures at resolution time. Before the fix the
// schema was dead: a sandboxed program slept exactly as long as an
// unsandboxed one, and every shipped profile's per-export rule was a
// false guarantee in the security layer.
func denySleepPolicy(t *testing.T) Policy {
	t.Helper()
	pol, err := policy.LoadInline(`{
		name: "no-sleep",
		scopes: {
			modules: {
				words: { default: "allow" },
				scopes: {
					"boru:time-util": {
						words: { default: "allow", rules: [{ deny: ["sleep"] }] }
					}
				}
			}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	return pol
}

// A module whose subscope denies nothing is unaffected, and the denial
// is addressed by the RESOLVED module id — a different module's
// same-named export does not gate.
func TestModuleCallGateIsAddressedByModuleID(t *testing.T) {
	pol := denySleepPolicy(t)
	const src = `import "boru:math-util"  MathUtil.sqrt 16.0`
	a, _ := New(Options{Policy: pol})
	got, err := a.RunInterp(src)
	if err != nil {
		t.Fatalf("another module must be unaffected: %v", err)
	}
	if fmt.Sprint(got) != "[4.0]" {
		t.Errorf("got %v, want [4.0]", got)
	}
}
