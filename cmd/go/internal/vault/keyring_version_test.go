package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

// legacyEncrypt reproduces the original headerless keyring layout
// (salt|nonce|ciphertext with AAD = salt) so tests can prove the
// current binary still reads vaults written before the format header.
func legacyEncrypt(t *testing.T, plain []byte, pass string) []byte {
	t.Helper()
	salt := make([]byte, keyringSaltLen)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	key, err := scryptKey(pass, salt)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	ct := gcm.Seal(nil, nonce, plain, salt)
	out := append([]byte{}, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out
}

// keyringEntry renders one stored line in the on-disk keyring format,
// "key\tbase64(value)\n".
func keyringEntry(key, value string) string {
	return key + "\t" + base64.StdEncoding.EncodeToString([]byte(value)) + "\n"
}

func TestKeyringHeaderRoundTrip(t *testing.T) {
	plain := []byte(keyringEntry("k", "secret"))
	blob, err := encryptBlob(plain, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(blob, []byte(keyringMagic)) {
		t.Errorf("encrypted blob missing magic header")
	}
	if int(blob[len(keyringMagic)]) != keyringFormat {
		t.Errorf("format byte = %d, want %d", blob[len(keyringMagic)], keyringFormat)
	}
	got, err := decryptBlob(blob, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("round-trip mismatch: %q != %q", got, plain)
	}
	if _, err := decryptBlob(blob, "wrong"); err == nil {
		t.Errorf("wrong passphrase should fail")
	}
}

func TestKeyringReadsLegacyBlob(t *testing.T) {
	plain := []byte(keyringEntry("legacy", "value"))
	legacy := legacyEncrypt(t, plain, "pw")
	if bytes.HasPrefix(legacy, []byte(keyringMagic)) {
		t.Fatal("legacy fixture unexpectedly carries the magic header")
	}
	got, err := decryptBlob(legacy, "pw")
	if err != nil {
		t.Fatalf("decrypting legacy blob: %s", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("legacy round-trip mismatch: %q != %q", got, plain)
	}
}

func TestKeyringRejectsNewerFormat(t *testing.T) {
	blob, err := encryptBlob([]byte("x"), "pw")
	if err != nil {
		t.Fatal(err)
	}
	// Bump the format byte beyond what this binary understands. The
	// version check precedes decryption, so this is reported as an
	// upgrade hint rather than a generic corruption error.
	blob[len(keyringMagic)] = byte(keyringFormat + 1)
	_, err = decryptBlob(blob, "pw")
	if err == nil {
		t.Fatal("expected error for newer keyring format")
	}
	if !strings.Contains(err.Error(), "upgrade boru") {
		t.Errorf("error should advise upgrading boru, got %q", err.Error())
	}
}

// TestFileKeyringUpgradesLegacyOnSave proves a vault written in the
// legacy layout is read transparently and rewritten with the format
// header on the next mutation.
func TestFileKeyringUpgradesLegacyOnSave(t *testing.T) {
	dir := t.TempDir()
	kr := &fileKeyring{folder: dir, pass: "pw"}

	// Seed the on-disk file with a legacy-format single entry.
	legacy := legacyEncrypt(t, []byte(keyringEntry("a", "first")), "pw")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kr.path(), legacy, 0600); err != nil {
		t.Fatal(err)
	}

	// Reading the legacy file works.
	got, err := kr.Get("a")
	if err != nil {
		t.Fatalf("Get on legacy file: %s", err)
	}
	if got != "first" {
		t.Errorf("Get = %q, want first", got)
	}

	// A mutation rewrites the file; it is now headered.
	if err := kr.Set("b", "second"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(kr.path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte(keyringMagic)) {
		t.Errorf("file not upgraded to headered format after save")
	}
	// Both entries survive the upgrade.
	for k, want := range map[string]string{"a": "first", "b": "second"} {
		v, err := kr.Get(k)
		if err != nil || v != want {
			t.Errorf("Get(%q) = %q, %v; want %q", k, v, err, want)
		}
	}
}

// preRenameEncrypt reproduces the headered layout exactly as it was
// written before the project was renamed from AQL to BORU: the magic was
// the 4-byte "AQLK" rather than the 5-byte "BORUK", and — as now — the
// header bytes plus salt were bound in as the AEAD additional data.
func preRenameEncrypt(t *testing.T, plain []byte, pass string) []byte {
	t.Helper()
	salt := make([]byte, keyringSaltLen)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	key, err := p7scryptKey(pass, salt)
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
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	header := append([]byte("AQLK"), byte(keyringFormat))
	ct := gcm.Seal(nil, nonce, plain, keyringAAD(header, salt))
	out := append([]byte{}, header...)
	out = append(out, salt...)
	out = append(out, nonce...)
	return append(out, ct...)
}

// TestKeyringReadsPreRenameBlob pins the AQL->BORU compatibility path.
// The rename changed keyringMagic from "AQLK" to "BORUK"; because the two
// differ in LENGTH, a reader that assumed len(keyringMagic) would slice the
// salt and nonce at the wrong offsets and fail the GCM tag — reporting a
// correct passphrase as wrong on every vault created before the rename.
func TestKeyringReadsPreRenameBlob(t *testing.T) {
	plain := []byte(keyringEntry("vxg:npm", "npm-token"))
	blob := preRenameEncrypt(t, plain, "pw")
	if !bytes.HasPrefix(blob, []byte("AQLK")) {
		t.Fatal("fixture does not carry the pre-rename magic")
	}
	got, err := decryptBlob(blob, "pw")
	if err != nil {
		t.Fatalf("pre-rename keyring rejected with the correct passphrase: %s", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("pre-rename round-trip mismatch: %q != %q", got, plain)
	}
	// A wrong passphrase must still be refused, not silently accepted.
	if _, err := decryptBlob(blob, "wrong"); err == nil {
		t.Error("wrong passphrase should fail on a pre-rename keyring too")
	}
}

// TestKeyringUpgradesPreRenameOnSave proves the compatibility path is
// transitional: the next write re-stamps the file with the current magic.
func TestKeyringUpgradesPreRenameOnSave(t *testing.T) {
	home := testHome(t)
	kr := &fileKeyring{folder: vaultFolder(home), pass: "pw"}
	if err := os.MkdirAll(kr.folder, 0o700); err != nil {
		t.Fatal(err)
	}
	blob := preRenameEncrypt(t, []byte(keyringEntry("vxg:npm", "npm-token")), "pw")
	if err := os.WriteFile(kr.path(), blob, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := kr.Get("vxg:npm"); err != nil || got != "npm-token" {
		t.Fatalf("Get on a pre-rename keyring = %q, %v", got, err)
	}
	if err := kr.Set("vxg:pypi", "pypi-token"); err != nil {
		t.Fatalf("Set: %s", err)
	}
	raw, err := os.ReadFile(kr.path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte(keyringMagic)) {
		t.Errorf("keyring not re-stamped with the current magic: %q", raw[:len("AQLK")])
	}
	for alias, want := range map[string]string{"vxg:npm": "npm-token", "vxg:pypi": "pypi-token"} {
		if got, err := kr.Get(alias); err != nil || got != want {
			t.Errorf("after upgrade %s = %q (%v), want %q", alias, got, err, want)
		}
	}
}

// TestKeyringFileFormatReadsPreRenameMagic covers the format probe, which
// takes its offset from the matched magic rather than the current one.
func TestKeyringFileFormatReadsPreRenameMagic(t *testing.T) {
	home := testHome(t)
	folder := vaultFolder(home)
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	blob := preRenameEncrypt(t, []byte(keyringEntry("k", "v")), "pw")
	kr := &fileKeyring{folder: folder, pass: "pw"}
	if err := os.WriteFile(kr.path(), blob, 0o600); err != nil {
		t.Fatal(err)
	}
	format, err := keyringFileFormat(folder)
	if err != nil {
		t.Fatal(err)
	}
	if format != keyringFormat {
		t.Errorf("format probe on a pre-rename keyring = %d, want %d", format, keyringFormat)
	}
}
