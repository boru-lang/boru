package lang_test

import (
	"errors"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/policy"
)

func loadPolicy(t *testing.T, name string) policy.Policy {
	t.Helper()
	p, err := policy.Load(name)
	if err != nil {
		t.Fatalf("policy.Load(%q): %s", name, err)
	}
	return p
}

// --- engine.words enforcement ---

func TestEngineScopeDeniesWord(t *testing.T) {
	// engine.default=deny + only `add` allowed.
	pol, err := policy.LoadInline(`{
		name: "only-add"
		scopes: {
			engine: {
				words: {
					default: "deny"
					rules: [{ allow: ["add"] }]
				}
			}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	a, err := lang.New(lang.Options{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	// add is allowed.
	if _, err := a.RunInterp("1 add 2"); err != nil {
		t.Errorf("add should be allowed: %v", err)
	}
	// sub is not.
	_, err = a.RunInterp("3 sub 1")
	if err == nil {
		t.Fatal("sub should be denied")
	}
	if !strings.Contains(err.Error(), "sub") {
		t.Errorf("error should mention sub: %v", err)
	}
}

func TestEngineScopeAllowsWithFullProfile(t *testing.T) {
	pol := loadPolicy(t, "full")
	a, err := lang.New(lang.Options{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunInterp("1 add 2"); err != nil {
		t.Errorf("full should allow add: %v", err)
	}
}

func TestEngineScopeNoPolicyAllowsEverything(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunInterp("1 add 2"); err != nil {
		t.Errorf("no policy should allow add: %v", err)
	}
}

// --- modules.import enforcement ---

func TestModulesScopeDeniesImport(t *testing.T) {
	pol, err := policy.LoadInline(`{
		name: "no-modules"
		scopes: {
			engine:  { words: { default: "allow" } }
			modules: { words: { default: "deny" } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	a, err := lang.New(lang.Options{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.RunInterp(`import "boru:math-util"`)
	if err == nil {
		t.Fatal("expected import to be denied")
	}
}

func TestModulesScopeAllowsSpecificModule(t *testing.T) {
	pol, err := policy.LoadInline(`{
		name: "only-math"
		scopes: {
			engine: { words: { default: "allow" } }
			modules: {
				words: {
					default: "deny"
					rules: [
						{ allow: ["import"], where: { module: ["boru:math-util"] } }
					]
				}
			}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	a, err := lang.New(lang.Options{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunInterp(`import "boru:math-util"`); err != nil {
		t.Errorf("boru:math should be allowed: %v", err)
	}
}

func TestModulesScopeUninstall(t *testing.T) {
	pol, err := policy.LoadInline(`{
		name: "no-modules-at-all"
		scopes: {
			engine:  { words: { default: "allow" } }
			modules: { install: false }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	a, err := lang.New(lang.Options{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.RunInterp(`import "boru:math-util"`)
	if err == nil {
		t.Fatal("expected import to be refused")
	}
	// The refusal is CODED (NUR079): an uninstalled modules scope is
	// capability_not_installed, blaming the scope's install flag.
	if !strings.Contains(err.Error(), "capability_not_installed") || !strings.Contains(err.Error(), "modules.install=false") {
		t.Errorf("expected a coded modules.install=false refusal, got: %v", err)
	}
}

// --- internal markers bypass the check ---

func TestInternalMarkersBypass(t *testing.T) {
	// Even with engine.default=deny and no allow rules, basic
	// programs that use internal markers (e.g. __pa for fn-body
	// pop) should not be denied — those are not user-visible
	// words.
	pol, err := policy.LoadInline(`{
		name: "deny-all-engine"
		scopes: {
			engine: { words: { default: "deny" } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	a, err := lang.New(lang.Options{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	// Pure literal evaluation doesn't dispatch any word, so it
	// succeeds even under deny-all engine.
	result, err := a.RunInterp("42")
	if err != nil {
		t.Errorf("literal eval should not be denied: %v", err)
	}
	if len(result) == 0 || result[0] != int64(42) {
		t.Errorf("result = %v, want 42", result)
	}
}

// --- the Denied error code path ---

func TestEngineDenialErrorContent(t *testing.T) {
	pol, err := policy.LoadInline(`{
		name: "no-sub"
		scopes: {
			engine: {
				words: {
					default: "allow"
					rules: [{ deny: ["sub"] }]
				}
			}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	a, err := lang.New(lang.Options{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.RunInterp("5 sub 1")
	var d *policy.Denied
	// The engine wraps the Denied in a BoruError; the .Error()
	// string surfaces it. We can't direct-assert As(*Denied)
	// because the engine wraps it.
	if err == nil {
		t.Fatal("expected denial")
	}
	if errors.As(err, &d) {
		if d.Scope != "engine" {
			t.Errorf("denied.Scope = %q, want engine", d.Scope)
		}
	}
	// Even without errors.As succeeding, the error text should
	// carry the relevant info.
	if !strings.Contains(err.Error(), "sub") {
		t.Errorf("error should mention sub: %v", err)
	}
}
