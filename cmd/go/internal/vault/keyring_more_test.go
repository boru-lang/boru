package vault

// keyring_more_test.go — backend selection, the file/envelope keyring
// containers, and blob-crypto edges. The exec-based host backends
// (security / secret-tool / powershell / op) are exercised only for their
// failure paths, and only when the host binary is ABSENT, so no real OS
// keychain is ever touched. Tests skip when a binary is present.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireNoBinary skips the test when bin is installed, so the error-path
// assertions can never hit a live host keychain.
func requireNoBinary(t *testing.T, bin string) {
	t.Helper()
	if _, err := exec.LookPath(bin); err == nil {
		t.Skipf("%s is installed; skipping to avoid touching a real keychain", bin)
	}
}

func TestSelectKeyringBackends(t *testing.T) {
	p7swapGoos(t, "linux")
	p7cleanPath(t)
	dir := t.TempDir()
	// The file backend always resolves.
	kr, err := selectKeyring(BackendFile, dir, "p")
	if err != nil || kr.Name() != BackendFile {
		t.Fatalf("file backend: %v", err)
	}
	// Unknown backends are rejected by name.
	if _, err := selectKeyring("floppy", dir, ""); err == nil ||
		!strings.Contains(err.Error(), "unknown backend") {
		t.Errorf("unknown backend: %v", err)
	}
	if p7goosName != "darwin" {
		if _, err := selectKeyring(BackendKeychain, dir, ""); err == nil ||
			!strings.Contains(err.Error(), "requires macOS") {
			t.Errorf("keychain off-macOS: %v", err)
		}
	}
	if p7goosName != "windows" {
		if _, err := selectKeyring(BackendWinCred, dir, ""); err == nil ||
			!strings.Contains(err.Error(), "requires windows") {
			t.Errorf("wincred off-windows: %v", err)
		}
	}
	requireNoBinary(t, "secret-tool")
	if _, err := selectKeyring(BackendSecretService, dir, ""); err == nil ||
		!strings.Contains(err.Error(), "secret-tool not found") {
		t.Errorf("secret-service without tool: %v", err)
	}
	requireNoBinary(t, "op")
	if _, err := selectKeyring(BackendOnePassword, dir, ""); err == nil ||
		!strings.Contains(err.Error(), "`op` not found") {
		t.Errorf("1password without op: %v", err)
	}
	// auto falls back to file when no host backend is available.
	if p7goosName == "linux" {
		if got := autoBackend(); got != BackendFile {
			t.Errorf("autoBackend = %q, want file (no secret-tool)", got)
		}
		kr, err := selectKeyring(BackendAuto, dir, "p")
		if err != nil || kr.Name() != BackendFile {
			t.Errorf("auto -> file: %v", err)
		}
		if _, err := selectKeyring("", dir, "p"); err != nil {
			t.Errorf("empty backend should auto-resolve: %v", err)
		}
	}
}

func TestMacKeychainErrorPaths(t *testing.T) {
	p7cleanPath(t)
	var k macKeychain
	if k.Name() != BackendKeychain {
		t.Error("name")
	}
	// A newline in the value is rejected before any exec.
	if err := k.Set("a", "line1\nline2"); err == nil ||
		!strings.Contains(err.Error(), "newline") {
		t.Errorf("newline value: %v", err)
	}
	requireNoBinary(t, "security")
	if err := k.Set("a", "v"); err == nil {
		t.Error("Set without the security binary should error")
	}
	if _, err := k.Get("a"); err == nil {
		t.Error("Get without the security binary should error")
	}
	if err := k.Delete("a"); err == nil {
		t.Error("Delete without the security binary should error")
	}
}

func TestSecretServiceErrorPaths(t *testing.T) {
	requireNoBinary(t, "secret-tool")
	var k secretService
	if k.Name() != BackendSecretService {
		t.Error("name")
	}
	if err := k.Set("a", "v"); err == nil {
		t.Error("Set without secret-tool should error")
	}
	if _, err := k.Get("a"); err == nil {
		t.Error("Get without secret-tool should error")
	}
	if err := k.Delete("a"); err == nil {
		t.Error("Delete without secret-tool should error")
	}
}

func TestWinCredErrorPaths(t *testing.T) {
	requireNoBinary(t, "powershell")
	var k winCred
	if k.Name() != BackendWinCred {
		t.Error("name")
	}
	if err := k.Set("a", "v"); err == nil {
		t.Error("Set without powershell should error")
	}
	if _, err := k.Get("a"); err == nil {
		t.Error("Get without powershell should error")
	}
	if err := k.Delete("a"); err == nil {
		t.Error("Delete without powershell should error")
	}
}

func TestOnePasswordKeyring(t *testing.T) {
	k := &onePasswordKeyring{}
	if k.Name() != BackendOnePassword {
		t.Error("name")
	}
	if k.itemTitle("x") != "boru:x" {
		t.Errorf("itemTitle = %q", k.itemTitle("x"))
	}
	if k.opVault() != "Private" {
		t.Errorf("default vault = %q", k.opVault())
	}
	if (&onePasswordKeyring{vault: "Work"}).opVault() != "Work" {
		t.Error("explicit vault ignored")
	}
	requireNoBinary(t, "op")
	if err := k.Set("a", "v"); err == nil {
		t.Error("Set without op should error")
	}
	if _, err := k.Get("a"); err == nil {
		t.Error("Get without op should error")
	}
	if err := k.Delete("a"); err == nil {
		t.Error("Delete without op should error")
	}
}

func TestFileKeyringListAndDelete(t *testing.T) {
	dir := t.TempDir()
	kr := &fileKeyring{folder: dir, pass: "p"}
	// Deleting from an absent file is a no-op.
	if err := kr.Delete("ghost"); err != nil {
		t.Errorf("delete on empty: %v", err)
	}
	if err := kr.Set("a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := kr.Set("b", "2"); err != nil {
		t.Fatal(err)
	}
	names, err := kr.list()
	if err != nil || len(names) != 2 {
		t.Fatalf("list = %v, %v", names, err)
	}
	if err := kr.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Get("a"); err != ErrNotFound {
		t.Errorf("deleted key Get = %v, want ErrNotFound", err)
	}
	// Deleting a missing key stays a no-op.
	if err := kr.Delete("a"); err != nil {
		t.Errorf("double delete: %v", err)
	}
	// A wrong passphrase cannot list.
	bad := &fileKeyring{folder: dir, pass: "wrong"}
	if _, err := bad.list(); err == nil {
		t.Error("list with the wrong passphrase should error")
	}
}

func TestFileKeyringCorruptEntry(t *testing.T) {
	dir := t.TempDir()
	kr := &fileKeyring{folder: dir, pass: "p"}
	// Hand-craft a blob whose entry value is not valid base64.
	enc, err := encryptBlob([]byte("alias\t!!!not-base64!!!\n"), "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kr.path(), enc, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Get("alias"); err == nil ||
		!strings.Contains(err.Error(), "corrupt keyring entry") {
		t.Errorf("corrupt entry: %v", err)
	}
	// An empty file reads as an empty keyring.
	if err := os.WriteFile(kr.path(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Get("alias"); err != ErrNotFound {
		t.Errorf("empty file Get = %v, want ErrNotFound", err)
	}
}

func TestEnvelopeFileKeyringContainer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvSuffix, "")
	kr := &envelopeFileKeyring{folder: dir}
	if kr.Name() != BackendFile {
		t.Error("name")
	}
	// Fresh: empty identity, no error.
	id, gen, err := kr.keyringIdentity()
	if err != nil || id != "" || gen != 0 {
		t.Errorf("fresh identity = %q %d %v", id, gen, err)
	}
	if err := kr.Set("a", "sealed-a"); err != nil {
		t.Fatal(err)
	}
	if err := kr.Set("@anchor", "9"); err != nil {
		t.Fatal(err)
	}
	// Saving assigned a stable id and bumped the generation.
	id, gen, err = kr.keyringIdentity()
	if err != nil || id == "" || gen != 2 {
		t.Errorf("identity after 2 saves = %q %d %v", id, gen, err)
	}
	// list excludes the reserved keyspace.
	names, _ := kr.list()
	if len(names) != 1 || names[0] != "a" {
		t.Errorf("list = %v, want [a] (reserved names hidden)", names)
	}
	if v, err := kr.Get("a"); err != nil || v != "sealed-a" {
		t.Errorf("Get = %q, %v", v, err)
	}
	if _, err := kr.Get("nope"); err != ErrNotFound {
		t.Errorf("missing Get = %v", err)
	}
	if err := kr.Delete("nope"); err != nil {
		t.Errorf("delete missing: %v", err)
	}
	if err := kr.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Get("a"); err != ErrNotFound {
		t.Error("a should be gone")
	}

	// Format sniffing on disk.
	if f, err := keyringFileFormat(dir); err != nil || f != envelopeKeyringFormat {
		t.Errorf("format = %d, %v", f, err)
	}
	if f, err := keyringFileFormat(t.TempDir()); err != nil || f != 0 {
		t.Errorf("absent format = %d, %v", f, err)
	}

	// A header-less file cannot be an envelope keyring.
	if err := os.WriteFile(kr.path(), []byte("garbage-blob"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.loadFile(); err == nil ||
		!strings.Contains(err.Error(), "no recognizable header") {
		t.Errorf("headerless: %v", err)
	}
	if f, _ := keyringFileFormat(dir); f != 1 {
		t.Errorf("headerless sniffs as legacy, got %d", f)
	}
	// A legacy format-1 blob under an envelope vault is called out.
	legacy := append([]byte(keyringMagic), byte(keyringFormat))
	if err := os.WriteFile(kr.path(), append(legacy, []byte("rest")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.loadFile(); err == nil ||
		!strings.Contains(err.Error(), "legacy single-passphrase format") {
		t.Errorf("legacy under envelope: %v", err)
	}
	// A future container format asks for an upgrade.
	futuristic := append([]byte(keyringMagic), byte(9))
	if err := os.WriteFile(kr.path(), append(futuristic, []byte("{}")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.loadFile(); err == nil || !strings.Contains(err.Error(), "upgrade boru") {
		t.Errorf("future container: %v", err)
	}
	// Corrupt JSON body.
	envHdr := append([]byte(keyringMagic), byte(envelopeKeyringFormat))
	if err := os.WriteFile(kr.path(), append(envHdr, []byte("{bad")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.loadFile(); err == nil || !strings.Contains(err.Error(), "parsing keyring") {
		t.Errorf("corrupt body: %v", err)
	}
	// An empty file loads as an empty container.
	if err := os.WriteFile(kr.path(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if ekf, err := kr.loadFile(); err != nil || len(ekf.Entries) != 0 {
		t.Errorf("empty file load = %+v, %v", ekf, err)
	}
}

func TestDecryptBlobLegacyAndHeaderedEdges(t *testing.T) {
	// Legacy headerless layout: salt|nonce|ct with AAD=salt.
	salt := make([]byte, keyringSaltLen)
	nonce := make([]byte, keyringNonceLen)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	key, err := scryptKey("p", salt)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	ct := gcm.Seal(nil, nonce, []byte("legacy-plain"), salt)
	blob := append(append(append([]byte{}, salt...), nonce...), ct...)
	plain, err := decryptBlob(blob, "p")
	if err != nil || string(plain) != "legacy-plain" {
		t.Errorf("legacy decrypt = %q, %v", plain, err)
	}
	if _, err := decryptBlob(blob, "wrong"); err == nil ||
		!strings.Contains(err.Error(), "wrong passphrase") {
		t.Errorf("legacy wrong pass: %v", err)
	}
	// Truncated legacy blob.
	if _, err := decryptBlob([]byte("tiny"), "p"); err == nil ||
		!strings.Contains(err.Error(), "truncated") {
		t.Errorf("legacy truncated: %v", err)
	}
	// Headered: future format, unknown format, truncated.
	future := append([]byte(keyringMagic), byte(keyringFormat+1))
	if _, err := decryptBlob(append(future, make([]byte, 64)...), "p"); err == nil ||
		!strings.Contains(err.Error(), "upgrade boru") {
		t.Errorf("headered future: %v", err)
	}
	zero := append([]byte(keyringMagic), byte(0))
	if _, err := decryptBlob(append(zero, make([]byte, 64)...), "p"); err == nil ||
		!strings.Contains(err.Error(), "unknown keyring format") {
		t.Errorf("headered zero: %v", err)
	}
	shortHdr := append([]byte(keyringMagic), byte(keyringFormat))
	if _, err := decryptBlob(append(shortHdr, []byte("x")...), "p"); err == nil ||
		!strings.Contains(err.Error(), "truncated") {
		t.Errorf("headered truncated: %v", err)
	}
	// Round-trip through the current writer, wrong passphrase rejected.
	enc, err := encryptBlob([]byte("hello"), "p")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := decryptBlob(enc, "p"); err != nil || string(got) != "hello" {
		t.Errorf("headered round-trip = %q, %v", got, err)
	}
	if _, err := decryptBlob(enc, "wrong"); err == nil {
		t.Error("headered wrong passphrase should fail")
	}
}

func TestIsReservedKeyringKey(t *testing.T) {
	if !isReservedKeyringKey("@integrity") || !isReservedKeyringKey("@anchor") {
		t.Error("reserved names not detected")
	}
	if isReservedKeyringKey("plain") {
		t.Error("plain names are not reserved")
	}
}

func TestFileKeyringSuffixPath(t *testing.T) {
	t.Setenv(EnvSuffix, "work")
	dir := t.TempDir()
	kr := &fileKeyring{folder: dir, pass: "p"}
	if err := kr.Set("a", "v"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vault.work.keyring")); err != nil {
		t.Errorf("suffixed keyring not written: %v", err)
	}
}
