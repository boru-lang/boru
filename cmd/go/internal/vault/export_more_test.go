package vault

// export_more_test.go — export/import edges: lock state, namespace
// filtering and remapping, malformed envelopes, and prompt mismatches.

import (
	"bytes"
	"crypto/aes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportRefusalPaths(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bp")
	dir := t.TempDir()
	initVaultAt(t, dir, "p")
	bundle := filepath.Join(t.TempDir(), "b.borux")

	// Nothing to export.
	if code, _, errOut := runVault(t, "", "export", "--out="+bundle); code == 0 ||
		!strings.Contains(errOut, "no matching aliases") {
		t.Errorf("empty vault export: %q", errOut)
	}
	if code, _, _ := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatal("add")
	}
	// Bad namespace filter.
	if code, _, errOut := runVault(t, "", "export", "--namespace=bad ns", "--out="+bundle); code == 0 {
		t.Errorf("invalid namespace should be refused: %q", errOut)
	}
	// A namespace filter with no matches.
	if code, _, errOut := runVault(t, "", "export", "--namespace=other", "--out="+bundle); code == 0 ||
		!strings.Contains(errOut, "no matching aliases") {
		t.Errorf("no-match filter: %q", errOut)
	}
	// An unknown referenced alias is an error, not silently skipped.
	if code, _, errOut := runVault(t, "", "export", "--out="+bundle, "nope"); code == 0 ||
		!strings.Contains(errOut, "no alias") {
		t.Errorf("unknown alias export: %q", errOut)
	}
	// A locked vault refuses to export.
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "", "export", "--out="+bundle); code == 0 ||
		!strings.Contains(errOut, "locked") {
		t.Errorf("locked export: %q", errOut)
	}
}

func TestExportNamespaceFilterSelects(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bp")
	dir := t.TempDir()
	initVaultAt(t, dir, "p")
	for _, k := range []string{"rootkey", "proj:one", "proj:two"} {
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", k); code != 0 {
			t.Fatalf("add %s: %s", k, e)
		}
	}
	read := func(path string) map[string]bool {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := openExport(data, "bp")
		if err != nil {
			t.Fatal(err)
		}
		var b exportBundle
		if err := json.Unmarshal(plain, &b); err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, a := range b.Aliases {
			got[a.Name] = true
		}
		return got
	}
	// --namespace=proj exports only the namespace.
	b1 := filepath.Join(t.TempDir(), "proj.borux")
	if code, _, e := runVault(t, "", "export", "--namespace=proj", "--out="+b1); code != 0 {
		t.Fatalf("export proj: %s", e)
	}
	if got := read(b1); !got["proj:one"] || !got["proj:two"] || got["rootkey"] {
		t.Errorf("proj filter selected %v", got)
	}
	// --namespace=: exports the root only.
	b2 := filepath.Join(t.TempDir(), "root.borux")
	if code, _, e := runVault(t, "", "export", "--namespace=:", "--out="+b2); code != 0 {
		t.Fatalf("export root: %s", e)
	}
	if got := read(b2); !got["rootkey"] || got["proj:one"] {
		t.Errorf("root filter selected %v", got)
	}
	// Named aliases combine with the filter.
	b3 := filepath.Join(t.TempDir(), "named.borux")
	if code, _, e := runVault(t, "", "export", "--namespace=proj", "--out="+b3, "proj:one", "rootkey"); code != 0 {
		t.Fatalf("export named: %s", e)
	}
	if got := read(b3); !got["proj:one"] || got["rootkey"] {
		t.Errorf("named+filter selected %v", got)
	}
}

func TestImportBundleRemapAndPrefix(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "bp")
	plain, _ := json.Marshal(exportBundle{Version: exportVersion, Aliases: []exportAlias{
		{Name: "proj:k", Provider: "openai", Value: "v1"},
		{Name: "plain", Value: "v2"},
		{Name: "empty", Value: ""},      // skipped: empty value
		{Name: "bad name!", Value: "v"}, // skipped: invalid alias
	}})
	blob, err := sealExport(plain, "bp")
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(t.TempDir(), "b.borux")
	if err := os.WriteFile(bundle, blob, 0o600); err != nil {
		t.Fatal(err)
	}

	// Remap everything into "team", with a prefix on the base names.
	dir := t.TempDir()
	initVaultAt(t, dir, "p")
	code, out, errOut := runVault(t, "", "import", "--namespace=team", "--prefix=x_", bundle)
	if code != 0 {
		t.Fatalf("import remap: %s", errOut)
	}
	for _, want := range []string{"imported team:x_k", "imported team:x_plain", "imported 2 secret(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("remap output missing %q: %q", want, out)
		}
	}
	for _, warn := range []string{"empty value", "invalid alias"} {
		if !strings.Contains(errOut, warn) {
			t.Errorf("stderr missing %q warning: %q", warn, errOut)
		}
	}
	if _, out, _ := runVault(t, "", "get", "--reveal", "team:x_k"); !strings.Contains(out, "v1") {
		t.Errorf("remapped secret unreadable: %q", out)
	}

	// Remap to the root namespace with ':'.
	dir2 := t.TempDir()
	initVaultAt(t, dir2, "p")
	if code, out, _ := runVault(t, "", "import", "--namespace=:", bundle); code != 0 ||
		!strings.Contains(out, "imported k") {
		t.Errorf("root remap = %d %q", code, out)
	}

	// An invalid --namespace is rejected before anything is written.
	dir3 := t.TempDir()
	initVaultAt(t, dir3, "p")
	if code, _, errOut := runVault(t, "", "import", "--namespace=bad ns", bundle); code == 0 ||
		!strings.Contains(errOut, "invalid namespace") {
		t.Errorf("invalid remap namespace: %q", errOut)
	}

	// A locked destination vault refuses the import.
	dir4 := t.TempDir()
	initVaultAt(t, dir4, "p")
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "", "import", bundle); code == 0 ||
		!strings.Contains(errOut, "locked") {
		t.Errorf("locked import: %q", errOut)
	}
}

func TestOpenExportMalformedEnvelopes(t *testing.T) {
	// Not a bundle at all.
	if _, err := openExport([]byte("PLAINTEXT"), "p"); err == nil ||
		!strings.Contains(err.Error(), "not a boru vault export bundle") {
		t.Errorf("non-bundle: %v", err)
	}
	// A future envelope format asks for an upgrade.
	future := append([]byte(exportMagic), byte(exportEnvelopeFormat+1))
	future = append(future, bytes.Repeat([]byte{0}, 64)...)
	if _, err := openExport(future, "p"); err == nil || !strings.Contains(err.Error(), "upgrade boru") {
		t.Errorf("future format: %v", err)
	}
	// Format zero is unknown.
	zero := append([]byte(exportMagic), byte(0))
	zero = append(zero, bytes.Repeat([]byte{0}, 64)...)
	if _, err := openExport(zero, "p"); err == nil || !strings.Contains(err.Error(), "unknown export bundle format") {
		t.Errorf("format zero: %v", err)
	}
	// Truncated body.
	trunc := append([]byte(exportMagic), byte(exportEnvelopeFormat))
	trunc = append(trunc, []byte("short")...)
	if _, err := openExport(trunc, "p"); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Errorf("truncated: %v", err)
	}
	// Valid envelope, wrong passphrase.
	blob, err := sealExport([]byte("{}"), "right")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openExport(blob, "wrong"); err == nil ||
		!strings.Contains(err.Error(), "wrong export passphrase") {
		t.Errorf("wrong passphrase: %v", err)
	}
}

func TestExportSealPassphraseMismatch(t *testing.T) {
	t.Setenv(EnvExportPassphrase, "")
	_, err := exportSealPassphrase(strings.NewReader("one\ntwo\n"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Errorf("mismatched confirmation: %v", err)
	}
	// The env override wins without prompting.
	t.Setenv(EnvExportPassphrase, "envpass")
	got, err := exportSealPassphrase(nil, nil)
	if err != nil || got != "envpass" {
		t.Errorf("env passphrase = %q, %v", got, err)
	}
}

func TestIsTerminalWriter(t *testing.T) {
	if isTerminalWriter(&bytes.Buffer{}) {
		t.Error("a buffer is not a terminal")
	}
	f, err := os.CreateTemp(t.TempDir(), "w")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminalWriter(f) {
		t.Error("a regular file is not a terminal")
	}
}

func TestImportBundleFutureInnerVersionViaFile(t *testing.T) {
	// Import must not authenticate against the vault before rejecting a
	// future schema (fail fast).
	t.Setenv(EnvExportPassphrase, "bp")
	plain, _ := json.Marshal(exportBundle{Version: exportVersion + 5})
	blob, _ := sealExport(plain, "bp")
	bundle := filepath.Join(t.TempDir(), "b.borux")
	if err := os.WriteFile(bundle, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	initVaultAt(t, dir, "p")
	if code, _, errOut := runVault(t, "", "import", bundle); code == 0 ||
		!strings.Contains(errOut, "upgrade boru") {
		t.Errorf("future inner version: %q", errOut)
	}
}

// preRenameSealExport builds a bundle envelope exactly as a pre-rename
// binary did: the 4-byte "AQLX" magic instead of the 5-byte "BORUX", with
// the header bytes plus salt bound in as the AEAD additional data.
func preRenameSealExport(t *testing.T, plain []byte, passphrase string) []byte {
	t.Helper()
	salt := make([]byte, keyringSaltLen)
	if _, err := randRead(salt); err != nil {
		t.Fatal(err)
	}
	key, err := c7scryptKey(passphrase, salt)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := gcmFromBlock(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := randRead(nonce); err != nil {
		t.Fatal(err)
	}
	header := append([]byte("AQLX"), byte(exportEnvelopeFormat))
	aad := append(append([]byte{}, header...), salt...)
	ct := gcm.Seal(nil, nonce, plain, aad)
	out := append([]byte{}, header...)
	out = append(out, salt...)
	out = append(out, nonce...)
	return append(out, ct...)
}

// TestExportReadsPreRenameBundle pins the AQL->BORU compatibility path for
// export bundles: the magic changed length, so a reader assuming
// len(exportMagic) would slice salt and nonce at the wrong offsets and
// reject the correct passphrase on every bundle exported before the rename.
func TestExportReadsPreRenameBundle(t *testing.T) {
	plain := []byte(`{"version":2,"aliases":[]}`)
	blob := preRenameSealExport(t, plain, "bundle-pw")
	if !isExportBundle(blob) {
		t.Fatal("a pre-rename bundle should be recognised as an export bundle")
	}
	got, err := openExport(blob, "bundle-pw")
	if err != nil {
		t.Fatalf("pre-rename bundle rejected with the correct passphrase: %s", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("pre-rename bundle round-trip mismatch: %q != %q", got, plain)
	}
	if _, err := openExport(blob, "wrong"); err == nil {
		t.Error("wrong passphrase should fail on a pre-rename bundle too")
	}
}
