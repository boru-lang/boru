package modules

import (
	"sort"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/policy"
)

// profile_module_ids_test.go — every module id a SHIPPED POLICY PROFILE names
// must be a module that exists.
//
// A profile gates imports with `modules.words` rules carrying a
// `where: { module: [...] }` predicate, and gates individual exports with
// per-module subscopes keyed by the same id. Both are plain strings, and
// Profile.Validate checks scope names and global op names but never module
// ids — so `boru policy validate` passes a typo silently, and a typo is
// double-edged: it permits a module that does not exist AND leaves the real
// one denied.
//
// That is not hypothetical. sandbox.jsonic shipped `"boru:math"` and
// `"boru:time"`, neither of which is a module id (the real ones carry the
// `-util` suffix — lang/go/CLAUDE.md's naming rule), so its allowlist had
// exactly one live entry out of three while its `deny: ["sleep"]` subscope
// rule guarded a module nobody could import. compute.jsonic and gen.jsonic
// carried the same `boru:math` typo.
//
// This test lives in `modules` rather than `policy` because it needs both
// halves: policy owns the profiles, modules owns the registry, and policy
// must not depend on modules.

// externalModules are ids a profile may legitimately name that this repository
// does NOT build in: a host can register its own native modules, and a profile
// shipped here may anticipate one. Each entry needs a reason, because the
// alternative reading of an unknown id is a typo.
var externalModules = map[string]string{
	// Moved out of this repository (commit a7882da, "remove the boru:decision
	// module (moved to a separate repo)"). gen.jsonic still names it because
	// the generator workflow runs against that external module; see
	// design/legacy/module-fn-checkstate-ownership.1.ignore.
	"boru:decision": "moved to a separate repository; gen.jsonic names it for hosts that register it",
}

func TestProfileModuleIDsExist(t *testing.T) {
	known := map[string]bool{}
	for name := range modules {
		known["boru:"+name] = true
	}
	if len(known) == 0 {
		t.Fatal("no modules registered — the registry lookup is wrong, not the profiles")
	}

	var problems []string
	for _, prof := range policy.BuiltinNames() {
		p, err := policy.BuiltinProfile(prof)
		if err != nil {
			t.Fatalf("load builtin profile %q: %v", prof, err)
		}
		mods := p.Scopes["modules"]
		if mods == nil {
			continue
		}
		check := func(id, where string) {
			if known[id] || externalModules[id] != "" {
				return
			}
			problems = append(problems, prof+": "+where+" names "+id+", which is not a module")
		}
		// The import allowlist: `where: { module: [...] }` on each rule.
		for i, rule := range mods.Words.Rules {
			for _, v := range rule.Where["module"] {
				s, ok := v.(string)
				if !ok {
					problems = append(problems, prof+": modules rule #"+itoa(i)+" has a non-string module predicate")
					continue
				}
				check(s, "modules rule #"+itoa(i))
			}
		}
		// The per-module subscopes, keyed by module id.
		var keys []string
		for id := range mods.Scopes {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			check(id, "modules.scopes")
		}
	}
	if len(problems) > 0 {
		t.Errorf("%d module id(s) in shipped profiles do not exist:\n  %s\n"+
			"Fix the id, or — if the module is deliberately out of tree — add it to externalModules with a reason.",
			len(problems), strings.Join(problems, "\n  "))
	}
}

// A profile that allows importing a module must also be able to CALL its
// exports: an absent per-module subscope means allow-all, so the pairing is
// not required — but where a subscope IS declared, it must name a module the
// same profile can import, or the rules it carries are dead.
func TestProfileModuleSubscopesAreImportable(t *testing.T) {
	for _, prof := range policy.BuiltinNames() {
		p, err := policy.BuiltinProfile(prof)
		if err != nil {
			t.Fatalf("load builtin profile %q: %v", prof, err)
		}
		mods := p.Scopes["modules"]
		if mods == nil || len(mods.Scopes) == 0 {
			continue
		}
		// Collect the ids this profile's own rules allow importing. A profile
		// that extends another inherits its parent's rules, so an empty set
		// here is not a finding — only a subscope for an id that appears in
		// NEITHER this profile's rules nor its parents' is.
		allowed := map[string]bool{}
		for cur := p; cur != nil; {
			if m := cur.Scopes["modules"]; m != nil {
				for _, rule := range m.Words.Rules {
					for _, v := range rule.Where["module"] {
						if s, ok := v.(string); ok {
							allowed[s] = true
						}
					}
				}
			}
			if cur.Extends == "" {
				break
			}
			next, err := policy.BuiltinProfile(cur.Extends)
			if err != nil {
				break
			}
			cur = next
		}
		if len(allowed) == 0 {
			continue
		}
		for id := range mods.Scopes {
			if !allowed[id] {
				t.Errorf("%s: modules.scopes has rules for %q, which no import rule in the profile or its parents allows — the rules are dead",
					prof, id)
			}
		}
	}
}
