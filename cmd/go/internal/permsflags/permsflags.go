// Package permsflags provides a shared flag set for boru CLI
// commands that build a lang.Boru instance and want to optionally
// apply a permissions policy.
//
// Usage:
//
//	var pf permsflags.Flags
//	permsflags.Register(fs, &pf)
//	if err := fs.Parse(args); err != nil { … }
//	pol, err := pf.Resolve()
//	a, _ := lang.New(lang.Options{Policy: pol})
//
// Resolution layers:
//
//  1. A base policy from --perms / --perms-file / --perms-inline
//     (auto-detected when --perms is used).
//  2. Incremental modifications via --allow / --deny /
//     --allow-global / --deny-global / --no-install / --install.
//  3. BORU_POLICY / BORU_POLICY_FILE env vars as fallback when no
//     explicit --perms flag was set.
//
// Returns nil Policy when no flags or env vars are set — preserving
// the default allow-everything behaviour.
package permsflags

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
	"github.com/boru-lang/boru/lang/go/policy"
)

// Flags holds the parsed permission flag values. A zero-value
// Flags represents "no policy" — Resolve returns nil.
type Flags struct {
	Perms       string
	PermsFile   string
	PermsInline string
	Allow       stringList
	Deny        stringList
	AllowGlobal stringList
	DenyGlobal  stringList
	NoInstall   stringList
	Install     stringList
}

// Register attaches every flag to fs. Pass a *Flags whose lifetime
// outlives the parse call.
func Register(fs *flag.FlagSet, f *Flags) {
	fs.StringVar(&f.Perms, "perms", "",
		"policy: profile name, file path, or inline jsonic (auto-detected)")
	fs.StringVar(&f.PermsFile, "perms-file", "",
		"policy file path (explicit)")
	fs.StringVar(&f.PermsInline, "perms-inline", "",
		"inline jsonic policy (explicit; @- = stdin, @path = file)")
	fs.Var(&f.Allow, "allow",
		"add an allow rule (repeatable; form: scope.op)")
	fs.Var(&f.Deny, "deny",
		"add a deny rule (repeatable; form: scope.op)")
	fs.Var(&f.AllowGlobal, "allow-global",
		"raise a global hard cap (repeatable; form: disk.read / network / …)")
	fs.Var(&f.DenyGlobal, "deny-global",
		"lower a global hard cap (repeatable)")
	fs.Var(&f.NoInstall, "no-install",
		"set scope.install=false (repeatable; form: scope or scope.subscope)")
	fs.Var(&f.Install, "install",
		"set scope.install=true (repeatable; overrides extends)")
}

// EnvPolicy is the policy the environment names — BORU_POLICY_FILE, then
// BORU_POLICY, exactly as Resolve reads them when no flag is given — for the
// surfaces that take no permission flags yet run a program's analysis, which
// executes an imported module's body (describe, the language server; NUR079).
// (nil, nil) when neither is set.
func EnvPolicy() (policy.Policy, error) {
	var none Flags
	return none.Resolve()
}

// Resolve produces the final policy from all flag inputs. Returns
// (nil, nil) when the user did not configure any policy. Returns
// an error when the inputs are inconsistent (e.g. --perms and
// --perms-file both set) or any policy fails to parse.
func (f *Flags) Resolve() (policy.Policy, error) {
	specified := 0
	if f.Perms != "" {
		specified++
	}
	if f.PermsFile != "" {
		specified++
	}
	if f.PermsInline != "" {
		specified++
	}
	if specified > 1 {
		return nil, fmt.Errorf("--perms, --perms-file, --perms-inline are mutually exclusive")
	}

	var base policy.Policy
	var err error
	// A leading ~ in a policy path that the shell left verbatim (e.g.
	// --perms-file=~/p.jsonic, a quoted path, or a BORU_POLICY_FILE env
	// value) must resolve under the home folder. LoadAuto only treats a
	// value as a file when it can stat it, so expanding first is also what
	// makes ~ paths detectable; a leading ~ never names a profile or
	// inline policy, so expansion is a no-op for those.
	switch {
	case f.Perms != "":
		base, err = policy.LoadAuto(pathutil.Expand(f.Perms))
	case f.PermsFile != "":
		base, err = policy.LoadFile(pathutil.Expand(f.PermsFile))
	case f.PermsInline != "":
		base, err = policy.LoadInline(f.PermsInline)
	default:
		// No explicit flag — try env fallbacks.
		if v := os.Getenv("BORU_POLICY_FILE"); v != "" {
			base, err = policy.LoadFile(pathutil.Expand(v))
		} else if v := os.Getenv("BORU_POLICY"); v != "" {
			base, err = policy.LoadAuto(pathutil.Expand(v))
		}
	}
	if err != nil {
		return nil, err
	}

	// If no incremental flags are set and no base was resolved,
	// return nil so lang.New runs unmodified.
	hasMods := len(f.Allow) > 0 || len(f.Deny) > 0 ||
		len(f.AllowGlobal) > 0 || len(f.DenyGlobal) > 0 ||
		len(f.NoInstall) > 0 || len(f.Install) > 0
	if base == nil && !hasMods {
		return nil, nil
	}
	if base == nil {
		// Mods supplied with no base: start from "full" as the
		// most-permissive base so they're additive subtractions.
		base, err = policyLoadFull()
		if err != nil {
			return nil, err
		}
	}

	if !hasMods {
		return base, nil
	}
	return applyMods(base, f)
}

// applyMods merges incremental rule additions onto base via a
// generated Profile-on-top-of-base, then recompiles.
func applyMods(base policy.Policy, f *Flags) (policy.Policy, error) {
	prof := &policy.Profile{
		Name:    base.Name() + "+mods",
		Extends: base.Name(),
		Scopes:  map[string]*policy.Scope{},
	}
	for _, raw := range f.Allow {
		scope, op, err := splitScopeOp(raw)
		if err != nil {
			return nil, err
		}
		addRule(prof, scope, policy.Rule{Allow: []string{op}})
	}
	for _, raw := range f.Deny {
		scope, op, err := splitScopeOp(raw)
		if err != nil {
			return nil, err
		}
		addRule(prof, scope, policy.Rule{Deny: []string{op}})
	}
	for _, raw := range f.AllowGlobal {
		addRule(prof, "global", policy.Rule{Allow: []string{raw}})
	}
	for _, raw := range f.DenyGlobal {
		addRule(prof, "global", policy.Rule{Deny: []string{raw}})
	}
	for _, raw := range f.NoInstall {
		setInstall(prof, raw, false)
	}
	for _, raw := range f.Install {
		setInstall(prof, raw, true)
	}
	return prof.Compile(loadByName(base))
}

// loadByName returns a resolver that looks up base by exact name
// and falls back to policy.loadProfileByName for any other.
func loadByName(base policy.Policy) func(string) (*policy.Profile, error) {
	return func(name string) (*policy.Profile, error) {
		if name == base.Name() {
			// Reflect the resolved base back as a Profile so the
			// chain works. Build a fresh Profile that re-imports
			// every scope. This is one round-trip cost per CLI
			// invocation.
			return profileFromPolicy(base), nil
		}
		// Fall back to the package's normal loader.
		p, err := policy.LoadAuto(name)
		if err != nil {
			return nil, err
		}
		return profileFromPolicy(p), nil
	}
}

// ProfileFromPolicy flattens a compiled Policy back into a serialisable
// Profile. Exported so `boru build` can BAKE the resolved policy into the
// executable: Policy is an interface and cannot be marshalled, Profile is
// fully json-tagged and can.
func ProfileFromPolicy(p policy.Policy) *policy.Profile { return profileFromPolicy(p) }

func profileFromPolicy(p policy.Policy) *policy.Profile {
	prof := &policy.Profile{
		Name:   p.Name(),
		Limits: p.Limits(),
		Scopes: map[string]*policy.Scope{},
	}
	for _, name := range policy.KnownScopes {
		s := p.Scope(name)
		// Clone into a heap copy so we don't share the live Compiled's
		// internal pointers.
		sCopy := s
		prof.Scopes[name] = &sCopy
	}
	return prof
}

// addRule appends a rule to the named scope, creating the scope
// if absent.
func addRule(p *policy.Profile, scopeName string, r policy.Rule) {
	if p.Scopes == nil {
		p.Scopes = map[string]*policy.Scope{}
	}
	s := p.Scopes[scopeName]
	if s == nil {
		s = &policy.Scope{}
		p.Scopes[scopeName] = s
	}
	s.Words.Rules = append(s.Words.Rules, r)
}

// setInstall sets the install field on the named scope (top-level
// or scope.subscope path).
func setInstall(p *policy.Profile, raw string, v bool) {
	if p.Scopes == nil {
		p.Scopes = map[string]*policy.Scope{}
	}
	parts := strings.SplitN(raw, ".", 2)
	scope := p.Scopes[parts[0]]
	if scope == nil {
		scope = &policy.Scope{}
		p.Scopes[parts[0]] = scope
	}
	if len(parts) == 1 {
		install := v
		scope.Install = &install
		return
	}
	// Subscope path.
	if scope.Scopes == nil {
		scope.Scopes = map[string]*policy.Scope{}
	}
	sub := scope.Scopes[parts[1]]
	if sub == nil {
		sub = &policy.Scope{}
		scope.Scopes[parts[1]] = sub
	}
	install := v
	sub.Install = &install
}

// splitScopeOp parses "scope.op" or "scope.subscope.op" into the
// scope and op fields the rule wants. For module subscope addressing
// the caller passes "modules.boru:math.sin" — the first dot splits
// scope from the rest; the rest is the op (for module exports,
// callers should use --perms-inline since the where-predicate form
// is required).
func splitScopeOp(raw string) (scope, op string, err error) {
	parts := strings.SplitN(raw, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("--allow/--deny: expected scope.op form, got %q", raw)
	}
	return parts[0], parts[1], nil
}

// stringList is a flag.Value that accumulates repeated --flag=value
// invocations into a slice.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}
