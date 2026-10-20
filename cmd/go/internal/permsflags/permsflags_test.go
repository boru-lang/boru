package permsflags

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/policy"
)

// parseFlags registers the shared flag set, parses argv, and returns
// the populated Flags.
func parseFlags(t *testing.T, argv ...string) *Flags {
	t.Helper()
	var f Flags
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	Register(fs, &f)
	if err := fs.Parse(argv); err != nil {
		t.Fatalf("parse %v: %v", argv, err)
	}
	return &f
}

// clearPolicyEnv guards the env-fallback path against ambient state.
func clearPolicyEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BORU_POLICY", "")
	t.Setenv("BORU_POLICY_FILE", "")
}

// writePolicy drops a minimal named profile file and returns its path.
func writePolicy(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".jsonic")
	body := `{
  version: 1
  name: "` + name + `"
  scopes: { fileops: { words: { default: "allow" } } }
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveNoFlagsNoEnvIsNil(t *testing.T) {
	clearPolicyEnv(t)
	pol, err := parseFlags(t).Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol != nil {
		t.Errorf("policy = %v, want nil (default allow-everything)", pol)
	}
}

func TestResolveMutuallyExclusive(t *testing.T) {
	clearPolicyEnv(t)
	cases := [][]string{
		{"--perms", "full", "--perms-file", "/x"},
		{"--perms", "full", "--perms-inline", "{}"},
		{"--perms-file", "/x", "--perms-inline", "{}"},
		{"--perms", "full", "--perms-file", "/x", "--perms-inline", "{}"},
	}
	for _, argv := range cases {
		_, err := parseFlags(t, argv...).Resolve()
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("%v: err = %v, want mutually-exclusive error", argv, err)
		}
	}
}

func TestResolvePermsProfileName(t *testing.T) {
	clearPolicyEnv(t)
	pol, err := parseFlags(t, "--perms", "sandbox").Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol == nil || pol.Name() != "sandbox" {
		t.Errorf("policy = %v, want the sandbox profile", pol)
	}
}

func TestResolvePermsFile(t *testing.T) {
	clearPolicyEnv(t)
	path := writePolicy(t, "fromfile")
	pol, err := parseFlags(t, "--perms-file", path).Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol == nil || pol.Name() != "fromfile" {
		t.Errorf("policy = %v, want fromfile", pol)
	}
}

func TestResolvePermsFileTildeExpansion(t *testing.T) {
	clearPolicyEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	body := `{ version: 1, name: "tilded", scopes: { fileops: { words: { default: "allow" } } } }`
	if err := os.WriteFile(filepath.Join(home, "p.jsonic"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	pol, err := parseFlags(t, "--perms-file", "~/p.jsonic").Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol == nil || pol.Name() != "tilded" {
		t.Errorf("policy = %v, want tilded", pol)
	}
}

func TestResolvePermsFileMissing(t *testing.T) {
	clearPolicyEnv(t)
	_, err := parseFlags(t, "--perms-file", filepath.Join(t.TempDir(), "nope.jsonic")).Resolve()
	if err == nil {
		t.Error("expected an error for a missing policy file")
	}
}

func TestResolvePermsInline(t *testing.T) {
	clearPolicyEnv(t)
	inline := `{ version: 1, name: "inl", scopes: { fileops: { words: { default: "deny" } } } }`
	pol, err := parseFlags(t, "--perms-inline", inline).Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol == nil || pol.Name() != "inl" {
		t.Errorf("policy = %v, want inl", pol)
	}
	if err := pol.Check("fileops", "write", policy.Args{"path": "/x"}); err == nil {
		t.Error("inline default=deny should deny fileops.write")
	}
}

func TestResolveEnvPolicyFileFallback(t *testing.T) {
	clearPolicyEnv(t)
	path := writePolicy(t, "envfile")
	t.Setenv("BORU_POLICY_FILE", path)
	pol, err := parseFlags(t).Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol == nil || pol.Name() != "envfile" {
		t.Errorf("policy = %v, want envfile", pol)
	}
}

func TestResolveEnvPolicyFallback(t *testing.T) {
	clearPolicyEnv(t)
	t.Setenv("BORU_POLICY", "sandbox")
	pol, err := parseFlags(t).Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol == nil || pol.Name() != "sandbox" {
		t.Errorf("policy = %v, want sandbox", pol)
	}
}

func TestResolveExplicitFlagBeatsEnv(t *testing.T) {
	clearPolicyEnv(t)
	t.Setenv("BORU_POLICY", "sandbox")
	pol, err := parseFlags(t, "--perms", "full").Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol == nil || pol.Name() != "full" {
		t.Errorf("policy = %v, want full (flag beats env)", pol)
	}
}

func TestResolveModsWithoutBaseStartFromFull(t *testing.T) {
	clearPolicyEnv(t)
	pol, err := parseFlags(t, "--deny", "fileops.write").Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol == nil {
		t.Fatal("policy should be non-nil when mods are supplied")
	}
	if pol.Name() != "full+mods" {
		t.Errorf("name = %q, want full+mods", pol.Name())
	}
	if err := pol.Check("fileops", "write", policy.Args{"path": "/x"}); err == nil {
		t.Error("--deny fileops.write should deny the op")
	}
	// Other fileops remain allowed (full base).
	if err := pol.Check("fileops", "read", policy.Args{"path": "/x"}); err != nil {
		t.Errorf("fileops.read should stay allowed: %v", err)
	}
}

func TestResolveAllowAndDenyRulesLand(t *testing.T) {
	clearPolicyEnv(t)
	pol, err := parseFlags(t,
		"--perms", "full",
		"--allow", "fileops.frob",
		"--deny", "network.dial",
	).Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	fileops := pol.Scope("fileops")
	if !ruleExists(fileops.Words.Rules, "frob", true) {
		t.Errorf("fileops rules missing allow frob: %+v", fileops.Words.Rules)
	}
	network := pol.Scope("network")
	if !ruleExists(network.Words.Rules, "dial", false) {
		t.Errorf("network rules missing deny dial: %+v", network.Words.Rules)
	}
	if err := pol.Check("network", "dial", policy.Args{}); err == nil {
		t.Error("--deny network.dial should deny the op")
	}
}

func TestResolveGlobalMods(t *testing.T) {
	clearPolicyEnv(t)
	pol, err := parseFlags(t,
		"--perms", "full",
		"--allow-global", "disk.read",
		"--deny-global", "network",
	).Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	global := pol.Scope("global")
	if !ruleExists(global.Words.Rules, "disk.read", true) {
		t.Errorf("global rules missing allow disk.read: %+v", global.Words.Rules)
	}
	if !ruleExists(global.Words.Rules, "network", false) {
		t.Errorf("global rules missing deny network: %+v", global.Words.Rules)
	}
	if err := pol.CheckGlobal("network"); err == nil {
		t.Error("--deny-global network should lower the hard cap")
	}
}

func TestResolveInstallToggles(t *testing.T) {
	clearPolicyEnv(t)
	pol, err := parseFlags(t, "--perms", "full", "--no-install", "sqlite").Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol.Installed("sqlite") {
		t.Error("--no-install sqlite should uninstall the scope")
	}

	pol, err = parseFlags(t, "--perms", "full", "--no-install", "sqlite", "--install", "sqlite").Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !pol.Installed("sqlite") {
		t.Error("--install sqlite should override --no-install")
	}
}

func TestResolveNoInstallSubscope(t *testing.T) {
	clearPolicyEnv(t)
	pol, err := parseFlags(t, "--perms", "full", "--no-install", "modules.boru:math").Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	mods := pol.Scope("modules")
	sub, ok := mods.Scopes["boru:math"]
	if !ok {
		t.Fatalf("modules subscopes missing boru:math: %+v", mods.Scopes)
	}
	if sub.Installed() {
		t.Error("--no-install modules.boru:math should uninstall the subscope")
	}
}

func TestResolveBadScopeOpForms(t *testing.T) {
	clearPolicyEnv(t)
	for _, argv := range [][]string{
		{"--allow", "nodot"},
		{"--deny", ".op"},
		{"--allow", "scope."},
	} {
		_, err := parseFlags(t, argv...).Resolve()
		if err == nil || !strings.Contains(err.Error(), "scope.op") {
			t.Errorf("%v: err = %v, want scope.op form error", argv, err)
		}
	}
}

func TestResolveModsOnFileBase(t *testing.T) {
	clearPolicyEnv(t)
	path := writePolicy(t, "filebase")
	pol, err := parseFlags(t, "--perms-file", path, "--deny", "fileops.write").Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pol.Name() != "filebase+mods" {
		t.Errorf("name = %q, want filebase+mods", pol.Name())
	}
	if err := pol.Check("fileops", "write", policy.Args{"path": "/x"}); err == nil {
		t.Error("deny mod on file base should deny fileops.write")
	}
}

func TestLoadByNameResolvesBaseAndFallsBack(t *testing.T) {
	clearPolicyEnv(t)
	base, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	resolver := loadByName(base)

	prof, err := resolver("sandbox")
	if err != nil {
		t.Fatalf("resolver(base): %v", err)
	}
	if prof.Name != "sandbox" {
		t.Errorf("base profile name = %q", prof.Name)
	}

	prof, err = resolver("full")
	if err != nil {
		t.Fatalf("resolver(full): %v", err)
	}
	if prof.Name != "full" {
		t.Errorf("fallback profile name = %q", prof.Name)
	}

	if _, err := resolver("no-such-profile"); err == nil {
		t.Error("resolver should fail for an unknown name")
	}
}

func TestStringListFlag(t *testing.T) {
	var s stringList
	if got := s.String(); got != "" {
		t.Errorf("empty String() = %q", got)
	}
	if err := s.Set("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("b"); err != nil {
		t.Fatal(err)
	}
	if got := s.String(); got != "a,b" {
		t.Errorf("String() = %q, want a,b", got)
	}
}

// ruleExists reports whether rules contains op with the given effect.
func ruleExists(rules []policy.Rule, op string, allow bool) bool {
	for _, r := range rules {
		names := r.Deny
		if allow {
			names = r.Allow
		}
		for _, n := range names {
			if n == op {
				return true
			}
		}
	}
	return false
}
