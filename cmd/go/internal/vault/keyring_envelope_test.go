package vault

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// envKeyringDir returns a temp folder with the suffix env cleared so
// vaultFileName resolves to vault.keyring.
func envKeyringDir(t *testing.T) string {
	t.Helper()
	t.Setenv(EnvSuffix, "")
	t.Setenv(EnvFolder, "")
	return t.TempDir()
}

func TestEnvelopeFileKeyringRoundTrip(t *testing.T) {
	dir := envKeyringDir(t)
	kr := &envelopeFileKeyring{folder: dir}

	if _, err := kr.Get("missing"); err != ErrNotFound {
		t.Errorf("Get(missing) = %v, want ErrNotFound", err)
	}
	if err := kr.Set("proj:apikey", "BORUE-sealed-blob"); err != nil {
		t.Fatal(err)
	}
	got, err := kr.Get("proj:apikey")
	if err != nil || got != "BORUE-sealed-blob" {
		t.Fatalf("Get = %q,%v want the sealed blob", got, err)
	}
	if err := kr.Delete("proj:apikey"); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Get("proj:apikey"); err != ErrNotFound {
		t.Errorf("after delete Get = %v, want ErrNotFound", err)
	}
}

// TestEnvelopeKeyringListExcludesReserved confirms the reserved keyspace
// (integrity key, anchor) is invisible to list(), so verify --prune
// can't treat it as an orphan.
func TestEnvelopeKeyringListExcludesReserved(t *testing.T) {
	dir := envKeyringDir(t)
	kr := &envelopeFileKeyring{folder: dir}
	if err := kr.Set("root:a", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := kr.Set("proj:b", "v2"); err != nil {
		t.Fatal(err)
	}
	if err := kr.Set("@integrity#aa", "ik"); err != nil {
		t.Fatal(err)
	}
	if err := kr.Set(anchorKeyName, "5"); err != nil {
		t.Fatal(err)
	}
	keys, err := kr.list()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("list returned %v, want exactly the two user aliases", keys)
	}
	for _, k := range keys {
		if isReservedKeyringKey(k) {
			t.Errorf("list leaked reserved key %q", k)
		}
	}
	// Reserved keys remain retrievable by exact name.
	if v, err := kr.Get("@integrity#aa"); err != nil || v != "ik" {
		t.Errorf("reserved Get = %q,%v want ik", v, err)
	}
}

// TestEnvelopeKeyringHeaderAndFormatDetection checks the on-disk header
// and the keyringFileFormat probe.
func TestEnvelopeKeyringHeaderAndFormatDetection(t *testing.T) {
	dir := envKeyringDir(t)
	if f, err := keyringFileFormat(dir); err != nil || f != 0 {
		t.Fatalf("format of absent keyring = %d,%v want 0", f, err)
	}

	kr := &envelopeFileKeyring{folder: dir}
	if err := kr.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "vault.keyring"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw[:len(keyringMagic)]) != keyringMagic || raw[len(keyringMagic)] != envelopeKeyringFormat {
		t.Errorf("keyring header = %q/%d, want %q/%d", raw[:len(keyringMagic)], raw[len(keyringMagic)], keyringMagic, envelopeKeyringFormat)
	}
	if f, err := keyringFileFormat(dir); err != nil || f != 2 {
		t.Errorf("format of envelope keyring = %d,%v want 2", f, err)
	}

	// mode 0600
	info, err := os.Stat(filepath.Join(dir, "vault.keyring"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 { // Windows has no permission bits
		t.Errorf("keyring mode = %v, want 0600", info.Mode().Perm())
	}
}

// TestEnvelopeKeyringRejectsLegacyFormat ensures the envelope backend
// refuses a legacy (format-1) keyring rather than misreading it.
func TestEnvelopeKeyringRejectsLegacyFormat(t *testing.T) {
	dir := envKeyringDir(t)
	// Write a legacy encrypted blob via the old fileKeyring.
	legacy := &fileKeyring{folder: dir, pass: "pw"}
	if err := legacy.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	if f, _ := keyringFileFormat(dir); f != 1 {
		t.Fatalf("legacy keyring format = %d, want 1", f)
	}
	env := &envelopeFileKeyring{folder: dir}
	if _, err := env.Get("k"); err == nil {
		t.Error("envelope backend should refuse a legacy-format keyring")
	}
}

// TestEnvelopeKeyringIdentity confirms a stable id and a monotonic
// generation across writes (used by restore divergence detection).
func TestEnvelopeKeyringIdentity(t *testing.T) {
	dir := envKeyringDir(t)
	kr := &envelopeFileKeyring{folder: dir}
	if err := kr.Set("a", "1"); err != nil {
		t.Fatal(err)
	}
	id1, gen1, err := kr.keyringIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if id1 == "" {
		t.Fatal("keyring id should be assigned on first save")
	}
	if err := kr.Set("b", "2"); err != nil {
		t.Fatal(err)
	}
	id2, gen2, err := kr.keyringIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id1 {
		t.Errorf("keyring id changed: %q -> %q", id1, id2)
	}
	if gen2 <= gen1 {
		t.Errorf("generation not monotonic: %d -> %d", gen1, gen2)
	}
}
