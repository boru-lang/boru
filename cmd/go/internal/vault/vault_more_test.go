package vault

// vault_more_test.go — Run()'s dispatch/location plumbing and the
// per-mode argument and refusal edges in vault.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandWrapper(t *testing.T) {
	testHome(t)
	c := New()
	if c.Name() != "vault" {
		t.Errorf("Name = %q", c.Name())
	}
	if !strings.Contains(c.Synopsis(), "vault") {
		t.Errorf("Synopsis = %q", c.Synopsis())
	}
	var out, errb strings.Builder
	if code := c.Run([]string{"help"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Errorf("help exit = %d", code)
	}
	if !strings.Contains(out.String(), "Usage: boru vault") {
		t.Errorf("help output: %q", out.String())
	}
}

func TestRunLocationHandling(t *testing.T) {
	testHome(t)
	// An invalid suffix (from the env) is rejected up front.
	t.Setenv(EnvSuffix, "bad suffix!")
	if code, _, errOut := runVault(t, "", "status"); code == 0 ||
		!strings.Contains(errOut, "invalid vault suffix") {
		t.Errorf("invalid suffix: %q", errOut)
	}
	t.Setenv(EnvSuffix, "")

	// --folder promotes into the env and localizes the vault files.
	folder := filepath.Join(t.TempDir(), "custom")
	if code, _, errOut := runVault(t, "", "--folder="+folder, "init", "--backend=file"); code != 0 {
		t.Fatalf("init --folder: %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(folder, "vault.jsonic")); err != nil {
		t.Errorf("store not created under --folder: %v", err)
	}
	// --suffix names sibling files.
	if code, _, errOut := runVault(t, "", "--folder="+folder, "--suffix=work", "init", "--backend=file"); code != 0 {
		t.Fatalf("init --suffix: %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(folder, "vault.work.jsonic")); err != nil {
		t.Errorf("suffixed store not created: %v", err)
	}
}

func TestRunWithoutHomeDir(t *testing.T) {
	// No BORU_HOME and no HOME: commands fail...
	t.Setenv(EnvHome, "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "") // os.UserHomeDir reads USERPROFILE on Windows
	t.Setenv(EnvFolder, "")
	t.Setenv(EnvSuffix, "")
	if code, _, errOut := runVault(t, "", "status"); code == 0 || errOut == "" {
		t.Errorf("no home should fail: %q", errOut)
	}
	// ...unless a --folder override makes the home irrelevant.
	folder := t.TempDir()
	t.Setenv(EnvPassphrase, "p")
	if code, _, errOut := runVault(t, "", "--folder="+folder, "init", "--backend=file"); code != 0 {
		t.Errorf("--folder should not need a home dir: %q", errOut)
	}
}

func TestInitEdges(t *testing.T) {
	testHome(t)
	// Mismatched interactive passphrases.
	t.Setenv(EnvPassphrase, "")
	if code, _, errOut := runVault(t, "one\ntwo\n", "init", "--backend=file"); code == 0 ||
		!strings.Contains(errOut, "did not match") {
		t.Errorf("mismatch: %q", errOut)
	}
	// A host backend that cannot be probed fails init loudly. Empty PATH so the
	// keychain tool (`security`) is unresolvable on any host — otherwise a real
	// macOS keychain would probe successfully, initialize the vault, and make
	// the later "unknown backend" and --force arms see an already-init vault.
	t.Setenv("PATH", t.TempDir())
	if code, _, errOut := runVault(t, "", "init", "--backend=keychain"); code == 0 || errOut == "" {
		t.Errorf("keychain init on this host should fail: %q", errOut)
	}
	// Unknown backend name.
	if code, _, errOut := runVault(t, "", "init", "--backend=floppy"); code == 0 ||
		!strings.Contains(errOut, "unknown backend") {
		t.Errorf("unknown backend: %q", errOut)
	}
	// --force reinitializes.
	t.Setenv(EnvPassphrase, "test-pass")
	mustInit(t)
	if code, _, errOut := runVault(t, "", "init", "--backend=file", "--force"); code != 0 {
		t.Errorf("init --force: %q", errOut)
	}
}

func TestAddArgumentEdges(t *testing.T) {
	testHome(t)
	mustInit(t)
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"usage", "", []string{"add"}, "usage"},
		{"bad expiry", "v\n", []string{"add", "--from-stdin", "--expiry=whenever", "k"}, "expiry"},
		{"namespace conflict", "v\n", []string{"add", "--from-stdin", "--namespace=a", "b:k"}, "already carries a namespace"},
		{"bad namespace", "v\n", []string{"add", "--from-stdin", "--namespace=bad ns", "k"}, "invalid namespace"},
		{"two sources", "v\n", []string{"add", "--from-stdin", "--from-env=X", "k"}, "only one of"},
		{"unset env var", "", []string{"add", "--from-env=NO_SUCH_VAR_SET", "k"}, "empty or unset"},
		{"empty value", "\n", []string{"add", "--from-stdin", "k"}, "empty value"},
	}
	for _, tc := range cases {
		code, _, errOut := runVault(t, tc.stdin, tc.args...)
		if code == 0 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: code=%d err=%q (want %q)", tc.name, code, errOut, tc.want)
		}
	}
	// --namespace=: forces the root namespace even with a default set.
	if code, _, _ := runVault(t, "", "config", "--set=namespace.default=proj"); code != 0 {
		t.Fatal("config")
	}
	if code, _, errOut := runVault(t, "v\n", "add", "--from-stdin", "--namespace=:", "rooted"); code != 0 {
		t.Fatalf("root-forced add: %s", errOut)
	}
	s, _ := LoadStore(os.Getenv(EnvHome))
	if a, _ := s.FindAlias("rooted"); a == nil {
		t.Error("rooted alias should be stored at the root, unqualified")
	}
	// The interactive prompt path reads the hidden value from stdin. The
	// bare name follows the active default namespace (proj, set above).
	if code, _, errOut := runVault(t, "prompted-value\n", "add", "prompted"); code != 0 {
		t.Fatalf("prompt add: %s", errOut)
	}
	if _, out, _ := runVault(t, "", "get", "--reveal", "proj:prompted"); !strings.Contains(out, "prompted-value") {
		t.Errorf("prompted value wrong: %q", out)
	}
}

func TestGetListEdges(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, errOut := runVault(t, "", "get"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("get usage: %q", errOut)
	}
	// A locked vault refuses get.
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add")
	}
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "", "get", "k"); code == 0 || !strings.Contains(errOut, "locked") {
		t.Errorf("locked get: %q", errOut)
	}
	if code, _, _ := runVault(t, "", "unlock"); code != 0 {
		t.Fatal("unlock")
	}
	// list filters by namespace, and rejects a bad filter.
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "proj:p1"); code != 0 {
		t.Fatal("add proj:p1")
	}
	code, out, _ := runVault(t, "", "list", "--namespace=proj")
	if code != 0 || !strings.Contains(out, "proj:p1") || strings.Contains(out, "\nk ") {
		t.Errorf("filtered list:\n%s", out)
	}
	code, out, _ = runVault(t, "", "list", "--namespace=:")
	if code != 0 || strings.Contains(out, "proj:p1") {
		t.Errorf("root-only list:\n%s", out)
	}
	if code, _, errOut := runVault(t, "", "list", "--namespace=bad ns"); code == 0 {
		t.Errorf("bad list filter accepted: %q", errOut)
	}
	// An empty vault prints a placeholder.
	dir2 := t.TempDir()
	initVaultAt(t, dir2, "p")
	if _, out, _ := runVault(t, "", "list"); !strings.Contains(out, "(no aliases)") {
		t.Errorf("empty list: %q", out)
	}
}

func TestRedactShapes(t *testing.T) {
	if redact("") != "<empty>" {
		t.Error("empty redact")
	}
	if redact("short") != "********" {
		t.Error("short redact")
	}
	if got := redact("longer-than-eight"); !strings.Contains(got, "(17 bytes)") {
		t.Errorf("long redact = %q", got)
	}
}

func TestRemoveAndRotateEdges(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, errOut := runVault(t, "", "rm"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("rm usage: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "rm", "--yes", "ghost"); code == 0 ||
		!strings.Contains(errOut, "no alias") {
		t.Errorf("rm unknown: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "rotate", "--yes", "--from-stdin"); code == 0 ||
		!strings.Contains(errOut, "usage") {
		t.Errorf("rotate usage: %q", errOut)
	}
	if code, _, errOut := runVault(t, "v\n", "rotate", "--yes", "--from-stdin", "ghost"); code == 0 ||
		!strings.Contains(errOut, "no alias") {
		t.Errorf("rotate unknown: %q", errOut)
	}
	if code, _, _ := runVault(t, "old\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add")
	}
	// Rotate without --yes has no terminal to confirm on.
	if code, _, errOut := runVault(t, "new\n", "rotate", "--from-stdin", "k"); code == 0 ||
		!strings.Contains(errOut, "confirmation") {
		t.Errorf("rotate without --yes: %q", errOut)
	}
	// Empty new value refused.
	if code, _, errOut := runVault(t, "\n", "rotate", "--yes", "--from-stdin", "k"); code == 0 ||
		!strings.Contains(errOut, "empty value") {
		t.Errorf("rotate empty: %q", errOut)
	}
	// Bad --expiry refused before any prompt.
	if code, _, errOut := runVault(t, "new\n", "rotate", "--yes", "--from-stdin", "--expiry=junk", "k"); code == 0 ||
		!strings.Contains(errOut, "expiry") {
		t.Errorf("rotate bad expiry: %q", errOut)
	}
	// A locked vault refuses rotation.
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "new\n", "rotate", "--yes", "--from-stdin", "k"); code == 0 ||
		!strings.Contains(errOut, "locked") {
		t.Errorf("locked rotate: %q", errOut)
	}
	if code, _, _ := runVault(t, "", "unlock"); code != 0 {
		t.Fatal("unlock")
	}
	// rotate --from-env with an expiry update.
	t.Setenv("ROT_VAL", "rotated-env")
	if code, _, errOut := runVault(t, "", "rotate", "--yes", "--from-env=ROT_VAL", "--expiry=90d", "k"); code != 0 {
		t.Fatalf("rotate --from-env: %s", errOut)
	}
	if _, out, _ := runVault(t, "", "get", "--reveal", "k"); !strings.Contains(out, "rotated-env") {
		t.Errorf("rotate value: %q", out)
	}
	s, _ := LoadStore(os.Getenv(EnvHome))
	if a, _ := s.FindAlias("k"); a == nil || a.ExpiresAt == "" || a.Source != "env:ROT_VAL" {
		t.Errorf("rotate metadata: %+v", a)
	}
}

func TestGrantRevokeEdges(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, errOut := runVault(t, "", "grant"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("grant usage: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "grant", "ghost"); code == 0 ||
		!strings.Contains(errOut, "no alias") {
		t.Errorf("grant unknown alias: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "revoke"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("revoke usage: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "revoke", "deadbeef"); code == 0 ||
		!strings.Contains(errOut, "no capability") {
		t.Errorf("revoke unknown: %q", errOut)
	}
	// A locked vault refuses to grant.
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add")
	}
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "", "grant", "k"); code == 0 || !strings.Contains(errOut, "locked") {
		t.Errorf("locked grant: %q", errOut)
	}
}

func TestConfigEdges(t *testing.T) {
	testHome(t)
	// Before init, config listing needs a store.
	if code, _, errOut := runVault(t, "", "config"); code == 0 ||
		!strings.Contains(errOut, "not initialized") {
		t.Errorf("config before init: %q", errOut)
	}
	mustInit(t)
	if code, _, errOut := runVault(t, "", "config", "--set=novalue"); code == 0 ||
		!strings.Contains(errOut, "key=value") {
		t.Errorf("config --set without '=': %q", errOut)
	}
}

func TestImportDotenvEdges(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// No parsable pairs.
	empty := filepath.Join(home, "empty.env")
	if err := os.WriteFile(empty, []byte("# only a comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runVault(t, "", "import", empty); code == 0 ||
		!strings.Contains(errOut, "no key=value pairs") {
		t.Errorf("empty dotenv: %q", errOut)
	}
	// A missing file is an I/O error.
	if code, _, errOut := runVault(t, "", "import", filepath.Join(home, "nope.env")); code == 0 || errOut == "" {
		t.Errorf("missing file: %q", errOut)
	}
	envFile := filepath.Join(home, "some.env")
	if err := os.WriteFile(envFile, []byte("GOOD=1\n1BAD KEY=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// --dry-run reports without writing.
	code, out, _ := runVault(t, "", "import", "--dry-run", "--prefix=pre_", envFile)
	if code != 0 || !strings.Contains(out, "would import pre_GOOD") {
		t.Errorf("dry-run: %q", out)
	}
	s, _ := LoadStore(home)
	if len(s.Aliases) != 0 {
		t.Error("dry-run wrote aliases")
	}
	// An invalid --namespace is rejected.
	if code, _, errOut := runVault(t, "", "import", "--namespace=bad ns", envFile); code == 0 ||
		!strings.Contains(errOut, "invalid namespace") {
		t.Errorf("bad namespace: %q", errOut)
	}
	// A real import skips the invalid key with a warning.
	code, out, errOut := runVault(t, "", "import", envFile)
	if code != 0 {
		t.Fatalf("import: %s", errOut)
	}
	if !strings.Contains(out, "imported GOOD") {
		t.Errorf("import out: %q", out)
	}
	if !strings.Contains(errOut, "skipping invalid alias") {
		t.Errorf("import warning: %q", errOut)
	}
	// Locked vault refuses .env import.
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "", "import", envFile); code == 0 ||
		!strings.Contains(errOut, "locked") {
		t.Errorf("locked import: %q", errOut)
	}
	if code, _, _ := runVault(t, "", "unlock"); code != 0 {
		t.Fatal("unlock")
	}
	// Importing from stdin works with the env passphrase.
	var (
		content = "STDIN_KEY=stdin-value\n"
	)
	code, out, errOut = runVault(t, content, "import")
	if code != 0 {
		t.Fatalf("stdin import: %s", errOut)
	}
	if !strings.Contains(out, "imported STDIN_KEY") {
		t.Errorf("stdin import out: %q", out)
	}
	if _, out, _ = runVault(t, "", "get", "--reveal", "STDIN_KEY"); !strings.Contains(out, "stdin-value") {
		t.Errorf("stdin import value: %q", out)
	}
}

func TestProvidersAndStatusBreakdown(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, out, _ := runVault(t, "", "providers")
	if code != 0 || !strings.Contains(out, "anthropic") || !strings.Contains(out, "x-api-key") {
		t.Errorf("providers: %q", out)
	}
	// The namespace breakdown appears once a non-root namespace exists.
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatal("add")
	}
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "rootk"); code != 0 {
		t.Fatal("add root")
	}
	code, out, _ = runVault(t, "", "status")
	if code != 0 || !strings.Contains(out, "namespaces:") || !strings.Contains(out, "(root)=1") ||
		!strings.Contains(out, "proj=1") {
		t.Errorf("status breakdown: %q", out)
	}
}

func TestSplitCSV(t *testing.T) {
	if got := splitCSV(""); got != nil {
		t.Errorf("empty = %v", got)
	}
	got := splitCSV(" a , ,b,")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("splitCSV = %v", got)
	}
}

func TestLockUnlockBeforeInit(t *testing.T) {
	testHome(t)
	if code, _, errOut := runVault(t, "", "lock"); code == 0 || errOut == "" {
		t.Errorf("lock before init should fail: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "unlock"); code == 0 || errOut == "" {
		t.Errorf("unlock before init should fail: %q", errOut)
	}
}
