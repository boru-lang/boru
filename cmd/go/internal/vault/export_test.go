package vault

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// initVaultAt initializes a fresh file-backed vault under dir with the
// given vault passphrase, and points BORU_HOME at it.
func initVaultAt(t *testing.T, dir, pass string) {
	t.Helper()
	t.Setenv(EnvHome, dir)
	t.Setenv(EnvPassphrase, pass)
	if code, _, errOut := runVault(t, "", "init", "--backend=file"); code != 0 {
		t.Fatalf("init at %s: %s", dir, errOut)
	}
}

// TestExportImportRoundTripAcrossVaults proves the portability story:
// secrets exported from one vault import cleanly into a *separate* vault
// (a stand-in for another machine), with metadata preserved.
func TestExportImportRoundTripAcrossVaults(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bundle-pass")
	bundle := filepath.Join(t.TempDir(), "vault.borux")

	// Source vault. --namespace qualifies, so k1 is stored as proj:k1.
	src := t.TempDir()
	initVaultAt(t, src, "src-pass")
	if code, _, e := runVault(t, "value-one\n", "add", "--from-stdin", "--provider=openai", "--namespace=proj", "k1"); code != 0 {
		t.Fatalf("add k1: %s", e)
	}
	if code, _, e := runVault(t, "value-two\n", "add", "--from-stdin", "k2"); code != 0 {
		t.Fatalf("add k2: %s", e)
	}
	if code, _, e := runVault(t, "", "export", "--out="+bundle); code != 0 {
		t.Fatalf("export: %s", e)
	}
	info, err := os.Stat(bundle)
	if err != nil {
		t.Fatalf("bundle not written: %s", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 { // Windows has no permission bits
		t.Errorf("bundle mode = %o, want 600", info.Mode().Perm())
	}

	// Destination vault: a different home and a different vault
	// passphrase, like a move to another machine.
	dst := t.TempDir()
	initVaultAt(t, dst, "dst-pass")
	if code, out, e := runVault(t, "", "import", bundle); code != 0 {
		t.Fatalf("import: %s", e)
	} else if !strings.Contains(out, "imported proj:k1") || !strings.Contains(out, "imported k2") {
		t.Errorf("import output missing aliases: %q", out)
	}

	// Values survived the move, qualified names intact.
	for alias, want := range map[string]string{"proj:k1": "value-one", "k2": "value-two"} {
		code, out, _ := runVault(t, "", "get", "--reveal", alias)
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("get %s = %q (code %d), want %q", alias, out, code, want)
		}
	}
	// Metadata survived too.
	s, _ := LoadStore(dst)
	if a, _ := s.FindAlias("proj:k1"); a == nil || a.Provider != "openai" || a.Namespace != "proj" {
		t.Errorf("proj:k1 metadata not preserved: %+v", a)
	}
}

// TestExportImportPreservesIPWhitelist guards that a proxy IP allowlist —
// enforced metadata — is carried through the export bundle and restored on
// import, so a backup/migration cannot silently drop a key's IP restriction.
func TestExportImportPreservesIPWhitelist(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bundle-pass")
	bundle := filepath.Join(t.TempDir(), "vault.borux")

	src := t.TempDir()
	initVaultAt(t, src, "src-pass")
	if code, _, e := runVault(t, "sekret\n", "add", "--from-stdin", "--ip-whitelist=10.0.0.0/8,203.0.113.7", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	if code, _, e := runVault(t, "", "export", "--out="+bundle); code != 0 {
		t.Fatalf("export: %s", e)
	}

	dst := t.TempDir()
	initVaultAt(t, dst, "dst-pass")
	if code, _, e := runVault(t, "", "import", bundle); code != 0 {
		t.Fatalf("import: %s", e)
	}
	s, _ := LoadStore(dst)
	a, _ := s.FindAlias("k")
	if a == nil || len(a.IPWhitelist) != 2 ||
		a.IPWhitelist[0] != "10.0.0.0/8" || a.IPWhitelist[1] != "203.0.113.7" {
		t.Errorf("ip-whitelist not preserved through export/import: %+v", a)
	}
}

// TestExportImportCarriesCustomProvider proves a custom-backed alias
// still brokers after moving vaults: the referenced preset travels in the
// bundle and is restored on import, rather than the alias landing with a
// dangling provider tag that resolves to the URL-less generic preset.
func TestExportImportCarriesCustomProvider(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bundle-pass")
	bundle := filepath.Join(t.TempDir(), "vault.borux")

	src := t.TempDir()
	initVaultAt(t, src, "src-pass")
	// Two referenced presets (so the bundle sorts more than one), plus one
	// no exported alias references (which must NOT travel).
	if code, _, e := runVault(t, "", "provider", "add",
		"--url=https://api.corp.example/v1", "--auth-style=x-api-key", "corp"); code != 0 {
		t.Fatalf("provider add corp: %s", e)
	}
	if code, _, e := runVault(t, "", "provider", "add", "--url=https://bbb.example", "bbb"); code != 0 {
		t.Fatalf("provider add bbb: %s", e)
	}
	if code, _, e := runVault(t, "", "provider", "add", "--url=https://unused.example", "unused"); code != 0 {
		t.Fatalf("provider add unused: %s", e)
	}
	if code, _, e := runVault(t, "sekret\n", "add", "--from-stdin", "--provider=corp", "k"); code != 0 {
		t.Fatalf("add k: %s", e)
	}
	if code, _, e := runVault(t, "sekret2\n", "add", "--from-stdin", "--provider=bbb", "k2"); code != 0 {
		t.Fatalf("add k2: %s", e)
	}
	if code, _, e := runVault(t, "", "export", "--out="+bundle, "k", "k2"); code != 0 {
		t.Fatalf("export: %s", e)
	}

	dst := t.TempDir()
	initVaultAt(t, dst, "dst-pass")
	code, out, e := runVault(t, "", "import", bundle)
	if code != 0 {
		t.Fatalf("import: %s", e)
	}
	if !strings.Contains(out, "restored 2 custom provider") {
		t.Errorf("import should report the restored presets: %q", out)
	}
	s, _ := LoadStore(dst)
	p, _ := s.FindCustomProvider("corp")
	if p == nil || p.BaseURL != "https://api.corp.example/v1" || p.AuthStyle != "x-api-key" {
		t.Fatalf("custom provider not carried through export/import: %+v", p)
	}
	if b, _ := s.FindCustomProvider("bbb"); b == nil || b.BaseURL != "https://bbb.example" {
		t.Errorf("second referenced preset not carried: %+v", b)
	}
	if _, idx := s.FindCustomProvider("unused"); idx >= 0 {
		t.Errorf("an unreferenced preset should not travel in the bundle")
	}
	// The alias resolves through the restored preset, so it would broker.
	if got := LookupProviderIn(s, "corp"); got.BaseURL != "https://api.corp.example/v1" {
		t.Errorf("imported alias does not resolve to the restored preset: %+v", got)
	}
}

// TestImportSkipsInvalidBundleProvider: a tampered bundle carrying a
// custom provider with an un-mintable name is warned about and dropped,
// never smuggled into the store (defence in depth behind the CLI's
// mint-time name check).
func TestImportSkipsInvalidBundleProvider(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bundle-pass")
	dir := filepath.Join(t.TempDir(), "tampered.borux")

	bundle := exportBundle{
		Version: exportVersion,
		Aliases: []exportAlias{{Name: "k", Provider: "corp", Value: "v"}},
		CustomProviders: []Provider{
			{Name: "corp", BaseURL: "https://api.corp.example", AuthStyle: "bearer"},
			{Name: "", BaseURL: "https://attacker.example", AuthStyle: "bearer"}, // un-mintable
		},
	}
	plain, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := sealExport(plain, "bundle-pass")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, blob, 0600); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	initVaultAt(t, dst, "dst-pass")
	code, out, errOut := runVault(t, "", "import", dir)
	if code != 0 {
		t.Fatalf("import: %s", errOut)
	}
	if !strings.Contains(errOut, "skipping invalid custom provider") {
		t.Errorf("import should warn about the invalid preset: %q", errOut)
	}
	if !strings.Contains(out, "restored 1 custom provider") {
		t.Errorf("only the valid preset should be restored: %q", out)
	}
	s, _ := LoadStore(dst)
	if len(s.CustomProviders) != 1 || s.CustomProviders[0].Name != "corp" {
		t.Errorf("invalid preset leaked into the store: %+v", s.CustomProviders)
	}
}

// TestImportCustomProviderOverwrite: an existing preset is kept unless
// --overwrite, mirroring alias import semantics.
func TestImportCustomProviderOverwrite(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bundle-pass")
	bundle := filepath.Join(t.TempDir(), "vault.borux")

	src := t.TempDir()
	initVaultAt(t, src, "src-pass")
	if code, _, e := runVault(t, "", "provider", "add", "--url=https://new.example", "corp"); code != 0 {
		t.Fatalf("provider add: %s", e)
	}
	if code, _, e := runVault(t, "sekret\n", "add", "--from-stdin", "--provider=corp", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	if code, _, e := runVault(t, "", "export", "--out="+bundle, "k"); code != 0 {
		t.Fatalf("export: %s", e)
	}

	dst := t.TempDir()
	initVaultAt(t, dst, "dst-pass")
	// A preset with the same name already exists on the target.
	if code, _, e := runVault(t, "", "provider", "add", "--url=https://old.example", "corp"); code != 0 {
		t.Fatalf("dst provider add: %s", e)
	}
	// Plain import keeps the existing preset (alias k is imported anew).
	if code, out, _ := runVault(t, "sekret\n", "add", "--from-stdin", "--provider=corp", "seed"); code != 0 {
		t.Fatalf("seed alias: %s", out)
	}
	code, _, errOut := runVault(t, "", "import", bundle)
	if code != 0 {
		t.Fatalf("import: %s", errOut)
	}
	if !strings.Contains(errOut, "skipping existing custom provider") {
		t.Errorf("import should report the kept preset on stderr: %q", errOut)
	}
	s, _ := LoadStore(dst)
	if p, _ := s.FindCustomProvider("corp"); p == nil || p.BaseURL != "https://old.example" {
		t.Errorf("plain import overwrote an existing preset: %+v", p)
	}
	// --overwrite replaces it.
	if code, _, e := runVault(t, "", "import", "--overwrite", bundle); code != 0 {
		t.Fatalf("import --overwrite: %s", e)
	}
	s, _ = LoadStore(dst)
	if p, _ := s.FindCustomProvider("corp"); p == nil || p.BaseURL != "https://new.example" {
		t.Errorf("--overwrite did not replace the preset: %+v", p)
	}
}

// TestExportOutTildeExpanded guards that a leading ~ in --out that the
// shell left verbatim (e.g. --out=~/bundle.borux) resolves under the home
// folder rather than writing to a literal "~" directory in the cwd.
func TestExportOutTildeExpanded(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bundle-pass")
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "sk\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	if code, _, e := runVault(t, "", "export", "--out=~/bundle.borux", "k"); code != 0 {
		t.Fatalf("export: %s", e)
	}
	if _, err := os.Stat(filepath.Join(home, "bundle.borux")); err != nil {
		t.Errorf("bundle not written under expanded home: %v", err)
	}
	if _, err := os.Stat(filepath.Join("~", "bundle.borux")); err == nil {
		t.Errorf("a literal ~ directory was created")
	}
}

// TestExportToStdoutKeepsPromptsOffBundle guards the corruption bug
// where passphrase prompts were written to the same stdout stream as
// the bundle, so `boru vault export > vault.borux` produced an
// unimportable file prefixed with prompt text. With no
// BORU_VAULT_EXPORT_PASSPHRASE set, the export passphrase is typed at
// the prompt (twice, for confirmation); the captured stdout must be a
// clean, importable bundle.
func TestExportToStdoutKeepsPromptsOffBundle(t *testing.T) {
	testHome(t) // sets BORU_VAULT_PASSPHRASE, so only the export prompt fires
	mustInit(t)
	if code, _, e := runVault(t, "secret-val\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}

	// No --out (bundle to stdout), no export-passphrase env: prompt path.
	code, out, errOut := runVault(t, "bundlepass\nbundlepass\n", "export")
	if code != 0 {
		t.Fatalf("export: code %d, stderr=%q", code, errOut)
	}
	if !strings.HasPrefix(out, exportMagic) {
		t.Fatalf("stdout bundle does not start with %q magic — prompt text leaked into it: %.50q", exportMagic, out)
	}
	// The prompts must have gone to stderr instead.
	if !strings.Contains(errOut, "export passphrase") {
		t.Errorf("expected the passphrase prompt on stderr, got %q", errOut)
	}
	// And the captured bytes are a real, decryptable bundle.
	plain, err := openExport([]byte(out), "bundlepass")
	if err != nil {
		t.Fatalf("captured stdout is not a valid bundle: %v", err)
	}
	if !strings.Contains(string(plain), `"k"`) {
		t.Errorf("bundle missing alias k: %s", plain)
	}
}

func TestExportSubsetByName(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bp")
	dir := t.TempDir()
	initVaultAt(t, dir, "p")
	for _, k := range []string{"a", "b", "c"} {
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", k); code != 0 {
			t.Fatalf("add %s: %s", k, e)
		}
	}
	bundle := filepath.Join(t.TempDir(), "b.borux")
	if code, _, e := runVault(t, "", "export", "--out="+bundle, "a", "c"); code != 0 {
		t.Fatalf("export: %s", e)
	}
	data, _ := os.ReadFile(bundle)
	plain, err := openExport(data, "bp")
	if err != nil {
		t.Fatal(err)
	}
	var b exportBundle
	_ = json.Unmarshal(plain, &b)
	got := map[string]bool{}
	for _, a := range b.Aliases {
		got[a.Name] = true
	}
	if !got["a"] || !got["c"] || got["b"] {
		t.Errorf("subset export wrong: %v", got)
	}
}

func TestImportBundleSkipsExistingUnlessOverwrite(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bp")
	bundle := filepath.Join(t.TempDir(), "b.borux")

	src := t.TempDir()
	initVaultAt(t, src, "p")
	if code, _, _ := runVault(t, "fresh\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add")
	}
	if code, _, e := runVault(t, "", "export", "--out="+bundle); code != 0 {
		t.Fatalf("export: %s", e)
	}

	// Destination already has k with a different value.
	dst := t.TempDir()
	initVaultAt(t, dst, "p")
	if code, _, _ := runVault(t, "original\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add dst")
	}

	// Default: skip, value unchanged.
	if code, out, _ := runVault(t, "", "import", bundle); code != 0 || !strings.Contains(out, "skipped 1") {
		t.Errorf("expected skip, code=%d out=%q", code, out)
	}
	if _, out, _ := runVault(t, "", "get", "--reveal", "k"); !strings.Contains(out, "original") {
		t.Errorf("value should be unchanged, got %q", out)
	}

	// --overwrite replaces it.
	if code, _, e := runVault(t, "", "import", "--overwrite", bundle); code != 0 {
		t.Fatalf("import --overwrite: %s", e)
	}
	if _, out, _ := runVault(t, "", "get", "--reveal", "k"); !strings.Contains(out, "fresh") {
		t.Errorf("value should be overwritten, got %q", out)
	}
}

func TestImportBundleWrongPassphrase(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "b.borux")
	src := t.TempDir()
	t.Setenv(EnvExportPassphrase, "right-pass")
	initVaultAt(t, src, "p")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add")
	}
	if code, _, e := runVault(t, "", "export", "--out="+bundle); code != 0 {
		t.Fatalf("export: %s", e)
	}

	dst := t.TempDir()
	initVaultAt(t, dst, "p")
	t.Setenv(EnvExportPassphrase, "wrong-pass")
	code, _, errOut := runVault(t, "", "import", bundle)
	if code == 0 {
		t.Fatal("expected failure with wrong export passphrase")
	}
	if !strings.Contains(errOut, "wrong export passphrase") {
		t.Errorf("unexpected error: %q", errOut)
	}
}

func TestExportRefusesEmptyPassphrase(t *testing.T) {
	dir := t.TempDir()
	initVaultAt(t, dir, "p")
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatal("add")
	}
	// No env passphrase; two empty prompt lines -> empty passphrase.
	t.Setenv(EnvExportPassphrase, "")
	bundle := filepath.Join(t.TempDir(), "b.borux")
	code, _, errOut := runVault(t, "\n\n", "export", "--out="+bundle)
	if code == 0 {
		t.Fatal("expected refusal of empty export passphrase")
	}
	if !strings.Contains(errOut, "must not be empty") {
		t.Errorf("unexpected error: %q", errOut)
	}
}

func TestImportRejectsFutureBundleVersion(t *testing.T) {
	// Craft a bundle whose inner schema version is from the future.
	future := exportBundle{Version: exportVersion + 1, Aliases: []exportAlias{{Name: "k", Value: "v"}}}
	plain, _ := json.Marshal(future)
	blob, err := sealExport(plain, "bp")
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(t.TempDir(), "b.borux")
	if err := os.WriteFile(bundle, blob, 0600); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	t.Setenv(EnvExportPassphrase, "bp")
	initVaultAt(t, dir, "p")
	code, _, errOut := runVault(t, "", "import", bundle)
	if code == 0 {
		t.Fatal("expected refusal of future bundle version")
	}
	if !strings.Contains(errOut, "upgrade boru") {
		t.Errorf("unexpected error: %q", errOut)
	}
}

func TestExportBundleEnvelopeShape(t *testing.T) {
	blob, err := sealExport([]byte(`{"version":1}`), "pw")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(blob, []byte(exportMagic)) {
		t.Errorf("bundle missing magic header")
	}
	if int(blob[len(exportMagic)]) != exportEnvelopeFormat {
		t.Errorf("envelope format byte = %d, want %d", blob[len(exportMagic)], exportEnvelopeFormat)
	}
	// A bundle is NOT mistaken for a keyring file, and vice versa.
	if isExportBundle([]byte("BORUK\x01rest")) {
		t.Errorf("keyring magic misclassified as export bundle")
	}
	got, err := openExport(blob, "pw")
	if err != nil || string(got) != `{"version":1}` {
		t.Errorf("round-trip failed: %q, %v", got, err)
	}
}

func TestImportBundleFromStdinRequiresEnvPassphrase(t *testing.T) {
	// Build a valid bundle.
	plain, _ := json.Marshal(exportBundle{Version: exportVersion, Aliases: []exportAlias{{Name: "k", Value: "v"}}})
	blob, _ := sealExport(plain, "bp")

	dir := t.TempDir()
	initVaultAt(t, dir, "p")
	t.Setenv(EnvExportPassphrase, "") // force prompt path, which stdin can't serve

	// Feed the bundle on stdin with no --in path and no env passphrase.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"import"}, bytes.NewReader(blob), &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected failure: cannot prompt for passphrase while stdin carries the bundle")
	}
	if !strings.Contains(stderr.String(), EnvExportPassphrase) {
		t.Errorf("error should name the env var, got %q", stderr.String())
	}
}
