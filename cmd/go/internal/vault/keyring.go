// Package vault implements `boru vault <mode>` — operation and
// configuration of a local key vault that stores third-party API
// keys and registry tokens via the host OS keychain (macOS Keychain,
// Linux Secret Service, Windows Credential Manager) or an
// AES-256-GCM encrypted file fallback.
//
// Real secret values never appear in ~/.boru/vault.jsonic, in
// process environment variables (unless explicitly requested with
// `vault get --reveal`), in command echoes, or in stored logs.
// The metadata file holds only aliases, policies, and short-lived
// capability token records.
//
// Secret values are also kept off the argv of the helper processes
// each backend shells out to — a same-user `ps` must not be able to
// read them. The file backend stays in-process; secret-tool, the
// macOS `security -i` command stream, and the Windows CredWrite
// helper all receive the secret on stdin; and 1Password is fed a
// template on stdin. Only alias names (a restricted character set)
// travel on argv or in the environment.
package vault

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/crypto/scrypt"

	"github.com/boru-lang/boru/cmd/go/internal/wire"
)

const (
	// keyringService is the namespace used for entries in the host
	// OS keychain. Per-alias keys are stored as "boru:<alias>".
	keyringService = wire.KeychainService

	// BackendAuto picks the best available host keychain and falls
	// back to the file backend.
	BackendAuto = "auto"
	// BackendKeychain uses the macOS Keychain via /usr/bin/security.
	BackendKeychain = "keychain"
	// BackendSecretService uses the freedesktop Secret Service API
	// via the secret-tool command from libsecret-tools.
	BackendSecretService = "secret-service"
	// BackendWinCred uses the Windows Credential Manager via a PowerShell
	// CredWrite/CredRead helper.
	BackendWinCred = "wincred"
	// BackendFile uses an AES-256-GCM encrypted file at
	// ~/.boru/vault.keyring. Used when no host keychain is available
	// or when explicitly requested for offline portability.
	BackendFile = "file"
	// BackendOnePassword stores secrets in 1Password via the `op`
	// CLI. The user must have `op` installed and signed in to a
	// 1Password account; entries are written to the configured
	// vault as items of category "API Credential" with the alias
	// as the item title.
	BackendOnePassword = "1password"
)

// ErrNotFound is returned by keyring.Get when no entry exists for
// the requested alias.
var ErrNotFound = errors.New("vault: secret not found")

// keyring is the storage interface for raw secret values. The
// vault metadata store (store.go) holds aliases and policies; the
// keyring holds only the secret bytes keyed by alias.
type keyring interface {
	// Name reports which backend identifier is in use (one of the
	// Backend* constants).
	Name() string
	// Set stores value under alias. Replaces any existing entry.
	Set(alias, value string) error
	// Get returns the value for alias, or ErrNotFound.
	Get(alias string) (string, error)
	// Delete removes alias if present. Returns nil if absent.
	Delete(alias string) error
}

// selectKeyring resolves a Backend* identifier to a concrete
// keyring implementation. BackendAuto picks the first available
// host backend, falling back to the file backend.
//
// The file backend requires passphrase to be non-empty unless its
// underlying file does not yet exist (in which case Set will
// initialize it with the given passphrase, even if empty — empty
// passphrases are permitted but produce a clear warning at init).
func selectKeyring(backend string, folder, passphrase string) (keyring, error) {
	if backend == "" || backend == BackendAuto {
		backend = autoBackend()
	}
	switch backend {
	case BackendKeychain:
		if p7goosName != "darwin" {
			return nil, fmt.Errorf("vault: keychain backend requires macOS, got %s", runtime.GOOS)
		}
		if _, err := exec.LookPath("security"); err != nil {
			return nil, fmt.Errorf("vault: /usr/bin/security not found")
		}
		return &macKeychain{}, nil
	case BackendSecretService:
		if _, err := exec.LookPath("secret-tool"); err != nil {
			return nil, fmt.Errorf("vault: secret-tool not found (install libsecret-tools)")
		}
		return &secretService{}, nil
	case BackendWinCred:
		if p7goosName != "windows" {
			return nil, fmt.Errorf("vault: wincred backend requires windows, got %s", runtime.GOOS)
		}
		if _, err := exec.LookPath("powershell"); err != nil {
			return nil, fmt.Errorf("vault: powershell not found (needed for the Windows Credential Manager backend)")
		}
		return &winCred{}, nil
	case BackendFile:
		return &fileKeyring{folder: folder, pass: passphrase}, nil
	case BackendOnePassword:
		if _, err := exec.LookPath("op"); err != nil {
			return nil, fmt.Errorf("vault: 1password CLI `op` not found")
		}
		return &onePasswordKeyring{vault: os.Getenv("BORU_VAULT_1PASSWORD_VAULT")}, nil
	default:
		return nil, fmt.Errorf("vault: unknown backend %q", backend)
	}
}

// autoBackend returns the preferred host backend for the current
// platform, or BackendFile if no host backend is available.
func autoBackend() string {
	switch p7goosName {
	case "darwin":
		if _, err := exec.LookPath("security"); err == nil {
			return BackendKeychain
		}
	case "linux":
		if _, err := exec.LookPath("secret-tool"); err == nil {
			return BackendSecretService
		}
	case "windows":
		if _, err := exec.LookPath("powershell"); err == nil {
			return BackendWinCred
		}
	}
	return BackendFile
}

// The host credential stores are shared, OS-global namespaces, and this
// project has written under more than one of them across renames. Writes
// always go to wire.KeychainService; reads and deletes sweep every
// namespace in wire.KeychainServices() so secrets stored by an
// earlier-named release stay reachable instead of silently vanishing.

// firstFound returns the first namespace's hit, treating ErrNotFound as
// "keep looking" and any other error as fatal (a broken keychain must not
// be reported as a missing secret).
func firstFound(services []string, probe func(service string) (string, error)) (string, error) {
	for _, svc := range services {
		v, err := probe(svc)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return "", err
		}
	}
	return "", ErrNotFound
}

// deleteEach removes the alias from every namespace, so a rename does not
// strand a stale copy that a later lookup could resurrect. It reports the
// first real failure but always attempts them all.
func deleteEach(services []string, del func(service string) error) error {
	var firstErr error
	for _, svc := range services {
		if err := del(svc); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// --- macOS Keychain via /usr/bin/security -----------------------------------

type macKeychain struct{}

func (*macKeychain) Name() string { return BackendKeychain }

func (k *macKeychain) Set(alias, value string) error {
	// The secret is NOT passed on argv (where a same-user `ps` could read
	// it). `security -i` reads its *command stream* from stdin, so the
	// add-generic-password line — secret included — travels on the
	// tool's stdin. `security`'s -w password prompt is no use here: it
	// reads /dev/tty, not stdin, whenever a terminal is present.
	//
	// A newline would terminate the single-line interactive command (and
	// could inject a second one), so it is rejected; every other byte of
	// the secret is backslash-escaped so nothing splits the token.
	if strings.ContainsAny(value, "\r\n") {
		return errors.New("vault: keychain backend cannot store a secret containing a newline; use --backend=file")
	}
	cmd := macSetCmd(alias, value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("security -i add-generic-password: %s: %s", err, strings.TrimSpace(string(out)))
	}
	// `security -i` exits 0 even when a sub-command fails, so confirm the
	// value round-trips. If a macOS build parses the escaping differently
	// this fails loudly (and cleans up) instead of leaving a corrupted
	// credential.
	got, err := k.Get(alias)
	if err != nil {
		return fmt.Errorf("security: stored %q but could not read it back: %w", alias, err)
	}
	if got != value {
		_ = k.Delete(alias)
		return fmt.Errorf("security: stored value for %q did not round-trip; refusing to keep a corrupted entry (use --backend=file)", alias)
	}
	return nil
}

// macSetCmd builds the `security -i` invocation that stores value under
// alias with the secret on the tool's stdin command stream rather than
// on argv. Exposed for testing the off-argv invariant.
func macSetCmd(alias, value string) *exec.Cmd {
	line := fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s\n",
		keyringService, alias, backslashEscapeAll(value))
	cmd := exec.Command("security", "-i")
	cmd.Stdin = strings.NewReader(line)
	return cmd
}

// backslashEscapeAll prefixes every byte of s with a backslash. Escaping
// uniformly means no space or metacharacter survives unescaped to split
// the argument or be misread by security's interactive parser. In the
// worst case (a build that does not treat backslash as an escape) the
// value simply fails Set's round-trip check rather than corrupting data
// or injecting a command.
func backslashEscapeAll(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	for i := 0; i < len(s); i++ {
		b.WriteByte('\\')
		b.WriteByte(s[i])
	}
	return b.String()
}

func (*macKeychain) Get(alias string) (string, error) {
	return firstFound(wire.KeychainServices(), func(service string) (string, error) {
		return macGet(service, alias)
	})
}

func macGet(service, alias string) (string, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", service, "-a", alias, "-w")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			if strings.Contains(string(ee.Stderr), "could not be found") {
				return "", ErrNotFound
			}
		}
		return "", fmt.Errorf("security find-generic-password: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (*macKeychain) Delete(alias string) error {
	return deleteEach(wire.KeychainServices(), func(service string) error {
		return macDelete(service, alias)
	})
}

func macDelete(service, alias string) error {
	cmd := exec.Command("security", "delete-generic-password",
		"-s", service, "-a", alias)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "could not be found") {
			return nil
		}
		return fmt.Errorf("security delete-generic-password: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// --- Linux Secret Service via secret-tool -----------------------------------

type secretService struct{}

func (*secretService) Name() string { return BackendSecretService }

func (*secretService) Set(alias, value string) error {
	cmd := exec.Command("secret-tool", "store", "--label", "boru:"+alias,
		"service", keyringService, "account", alias)
	cmd.Stdin = strings.NewReader(value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("secret-tool store: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (*secretService) Get(alias string) (string, error) {
	return firstFound(wire.KeychainServices(), func(service string) (string, error) {
		return secretServiceGet(service, alias)
	})
}

func secretServiceGet(service, alias string) (string, error) {
	cmd := exec.Command("secret-tool", "lookup",
		"service", service, "account", alias)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 && len(ee.Stderr) == 0 {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("secret-tool lookup: %w", err)
	}
	v := string(out)
	if v == "" {
		return "", ErrNotFound
	}
	return strings.TrimRight(v, "\n"), nil
}

func (*secretService) Delete(alias string) error {
	return deleteEach(wire.KeychainServices(), func(service string) error {
		return secretServiceDelete(service, alias)
	})
}

func secretServiceDelete(service, alias string) error {
	cmd := exec.Command("secret-tool", "clear",
		"service", service, "account", alias)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("secret-tool clear: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// --- Windows Credential Manager via PowerShell CredWrite/CredRead -----------

// winCred stores secrets in the Windows Credential Manager through a
// small PowerShell helper that P/Invokes CredWrite / CredRead /
// CredDelete. Unlike the previous cmdkey wrapper, the secret is handed
// to the helper on stdin (never on argv, where a same-user process
// could read it), and Get can actually return the value — cmdkey could
// not read passwords back.
type winCred struct{}

func (*winCred) Name() string { return BackendWinCred }

func (k *winCred) Set(alias, value string) error {
	cmd := winCredCmd("set", alias)
	cmd.Stdin = strings.NewReader(value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("credential write: %s: %s", err, strings.TrimSpace(string(out)))
	}
	// Confirm the value round-trips; otherwise fail loudly (and clean
	// up) rather than leave a silently-wrong credential.
	got, err := k.Get(alias)
	if err != nil {
		return fmt.Errorf("credential stored %q but could not read it back: %w", alias, err)
	}
	if got != value {
		_ = k.Delete(alias)
		return fmt.Errorf("credential value for %q did not round-trip; refusing to keep a corrupted entry (use --backend=file)", alias)
	}
	return nil
}

func (*winCred) Get(alias string) (string, error) {
	return firstFound(wire.KeychainServices(), func(service string) (string, error) {
		return winCredGet(service, alias)
	})
}

func winCredGet(service, alias string) (string, error) {
	cmd := winCredCmdIn(service, "get", alias)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 2 {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("credential read: %s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (*winCred) Delete(alias string) error {
	return deleteEach(wire.KeychainServices(), func(service string) error {
		return winCredDelete(service, alias)
	})
}

func winCredDelete(service, alias string) error {
	cmd := winCredCmdIn(service, "delete", alias)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("credential delete: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// winCredCmd builds the PowerShell helper invocation for op (set | get |
// delete) on alias. The operation and the credential target/user travel
// in environment variables, and for set the secret is supplied on stdin
// by the caller; the script itself carries no secret. So nothing
// sensitive reaches any process's argv. Exposed for testing.
func winCredCmd(op, alias string) *exec.Cmd {
	return winCredCmdIn(keyringService, op, alias)
}

// winCredCmdIn is winCredCmd against an explicit credential-store
// namespace, so Get and Delete can sweep the legacy ones.
func winCredCmdIn(service, op, alias string) *exec.Cmd {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", winCredScript)
	cmd.Env = append(os.Environ(),
		"BORU_KR_OP="+op,
		"BORU_KR_TARGET="+service+":"+alias,
		"BORU_KR_USER="+alias,
	)
	return cmd
}

// winCredScript is the PowerShell helper. It defines a C# type that
// P/Invokes the Win32 Credential Manager API and dispatches on
// $env:BORU_KR_OP. For "set" the secret is read from stdin; for "get" the
// value is written to stdout (exit code 2 means "not found"); "delete"
// removes the entry (a missing entry is success).
const winCredScript = `
$ErrorActionPreference = 'Stop'
try { [Console]::InputEncoding  = New-Object System.Text.UTF8Encoding $false } catch {}
try { [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding $false } catch {}
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class BoruCred {
    [StructLayout(LayoutKind.Sequential)]
    struct CREDENTIAL {
        public uint Flags;
        public uint Type;
        public IntPtr TargetName;
        public IntPtr Comment;
        public System.Runtime.InteropServices.ComTypes.FILETIME LastWritten;
        public uint CredentialBlobSize;
        public IntPtr CredentialBlob;
        public uint Persist;
        public uint AttributeCount;
        public IntPtr Attributes;
        public IntPtr TargetAlias;
        public IntPtr UserName;
    }
    [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    static extern bool CredWriteW(ref CREDENTIAL cred, uint flags);
    [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true, EntryPoint = "CredReadW")]
    static extern bool CredReadW(string target, uint type, uint flags, out IntPtr cred);
    [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true, EntryPoint = "CredDeleteW")]
    static extern bool CredDeleteW(string target, uint type, uint flags);
    [DllImport("advapi32.dll")]
    static extern void CredFree(IntPtr buf);
    const uint GENERIC = 1;
    const uint PERSIST_LOCAL_MACHINE = 2;
    const int ERROR_NOT_FOUND = 1168;
    public static void Write(string target, string user, string secret) {
        byte[] blob = Encoding.Unicode.GetBytes(secret);
        CREDENTIAL c = new CREDENTIAL();
        c.Type = GENERIC;
        c.TargetName = Marshal.StringToCoTaskMemUni(target);
        c.UserName = Marshal.StringToCoTaskMemUni(user);
        c.CredentialBlobSize = (uint)blob.Length;
        c.CredentialBlob = Marshal.AllocCoTaskMem(blob.Length == 0 ? 1 : blob.Length);
        Marshal.Copy(blob, 0, c.CredentialBlob, blob.Length);
        c.Persist = PERSIST_LOCAL_MACHINE;
        bool ok = CredWriteW(ref c, 0);
        int err = Marshal.GetLastWin32Error();
        Marshal.FreeCoTaskMem(c.TargetName);
        Marshal.FreeCoTaskMem(c.UserName);
        Marshal.FreeCoTaskMem(c.CredentialBlob);
        if (!ok) throw new Exception("CredWrite failed: " + err);
    }
    public static int Read(string target, out string value) {
        value = null;
        IntPtr p;
        if (!CredReadW(target, GENERIC, 0, out p)) {
            int err = Marshal.GetLastWin32Error();
            if (err == ERROR_NOT_FOUND) return 2;
            throw new Exception("CredRead failed: " + err);
        }
        try {
            CREDENTIAL c = (CREDENTIAL)Marshal.PtrToStructure(p, typeof(CREDENTIAL));
            if (c.CredentialBlobSize == 0) { value = ""; return 0; }
            byte[] blob = new byte[c.CredentialBlobSize];
            Marshal.Copy(c.CredentialBlob, blob, 0, (int)c.CredentialBlobSize);
            value = Encoding.Unicode.GetString(blob);
            return 0;
        } finally {
            CredFree(p);
        }
    }
    public static void Delete(string target) {
        if (!CredDeleteW(target, GENERIC, 0)) {
            int err = Marshal.GetLastWin32Error();
            if (err == ERROR_NOT_FOUND) return;
            throw new Exception("CredDelete failed: " + err);
        }
    }
}
'@
$op = $env:BORU_KR_OP
$target = $env:BORU_KR_TARGET
$user = $env:BORU_KR_USER
switch ($op) {
    'set' {
        $secret = [Console]::In.ReadToEnd()
        [BoruCred]::Write($target, $user, $secret)
    }
    'get' {
        $value = $null
        $rc = [BoruCred]::Read($target, [ref]$value)
        if ($rc -eq 2) { exit 2 }
        [Console]::Out.Write($value)
    }
    'delete' {
        [BoruCred]::Delete($target)
    }
    default {
        [Console]::Error.Write("unknown op")
        exit 3
    }
}
`

// --- File-backed AES-256-GCM fallback ---------------------------------------

// fileKeyring stores aliases in a single AES-256-GCM encrypted JSON
// blob at {folder}/vault.keyring (or vault.<suffix>.keyring). Each
// Set/Delete reads the file, mutates the map, and rewrites it; this is
// acceptable for the expected ~tens of secrets and avoids partial
// writes leaving readable plaintext on disk.
type fileKeyring struct {
	folder string
	pass   string
}

func (f *fileKeyring) Name() string { return BackendFile }

func (f *fileKeyring) path() string { return filepath.Join(f.folder, vaultFileName("keyring")) }

func (f *fileKeyring) load() (map[string]string, error) {
	data, err := os.ReadFile(f.path())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[string]string{}, nil
	}
	plain, err := decryptBlob(data, f.pass)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if len(plain) == 0 {
		return out, nil
	}
	for _, line := range strings.Split(string(plain), "\n") {
		if line == "" {
			continue
		}
		idx := strings.IndexByte(line, '\t')
		if idx < 0 {
			continue
		}
		k := line[:idx]
		v, err := base64.StdEncoding.DecodeString(line[idx+1:])
		if err != nil {
			return nil, fmt.Errorf("vault: corrupt keyring entry %q", k)
		}
		out[k] = string(v)
	}
	return out, nil
}

func (f *fileKeyring) save(m map[string]string) error {
	if err := os.MkdirAll(f.folder, 0700); err != nil {
		return err
	}
	var buf bytes.Buffer
	for k, v := range m {
		buf.WriteString(k)
		buf.WriteByte('\t')
		buf.WriteString(base64.StdEncoding.EncodeToString([]byte(v)))
		buf.WriteByte('\n')
	}
	enc, err := encryptBlob(buf.Bytes(), f.pass)
	if err != nil {
		return err
	}
	return writeFileAtomic(f.path(), enc, 0600)
}

func (f *fileKeyring) Set(alias, value string) error {
	m, err := f.load()
	if err != nil {
		return err
	}
	m[alias] = value
	return f.save(m)
}

func (f *fileKeyring) Get(alias string) (string, error) {
	m, err := f.load()
	if err != nil {
		return "", err
	}
	v, ok := m[alias]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *fileKeyring) Delete(alias string) error {
	m, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := m[alias]; !ok {
		return nil
	}
	delete(m, alias)
	return f.save(m)
}

// list returns every stored key, satisfying keyringLister so `vault
// verify` can detect orphaned entries. Only the file backend can
// enumerate; the OS-keychain and 1Password backends do not implement
// keyringLister, and verify degrades to dangling-metadata detection
// for them.
func (f *fileKeyring) list() ([]string, error) {
	m, err := f.load()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out, nil
}

// --- Envelope-mode file keyring (format 2) ----------------------------------

// Reserved keyring keyspace. Vault-internal material (the HMAC integrity
// key and the anti-rollback anchor) is stored under names beginning with
// '@', a byte validAlias rejects, so a reserved key can never collide
// with a user alias. add/import/mv refuse reserved names, `verify
// --prune` skips them, and export omits them.
const reservedKeyPrefix = "@"

const (
	// integrityNamespace is the reserved namespace whose data key IS the
	// store-integrity HMAC key. Like any NDK it is sealed into every
	// slot's WrappedKeys, so any authenticated session can verify/maintain
	// the keyed integrity layer.
	integrityNamespace = "@integrity"
	// anchorKeyName is a reserved keyring entry holding the anti-rollback
	// high-water-mark generation (a plain integer, not sealed).
	anchorKeyName = "@anchor"
)

// isReservedKeyringKey reports whether k is in the reserved keyspace.
func isReservedKeyringKey(k string) bool { return strings.HasPrefix(k, reservedKeyPrefix) }

// envelopeKeyringFormat is the on-disk format byte for the plaintext
// envelope container, distinct from the legacy encrypted blob's
// keyringFormat=1. Values inside are already BORUE-sealed under namespace
// data keys, so the container itself needs no encryption — there is no
// single passphrase to encrypt it under once a vault has scoped slots.
const envelopeKeyringFormat = 2

// envelopeKeyringFile is the JSON body of a format-2 keyring. KeyringID
// and Generation let restore detect store<->keyring divergence; Entries
// maps each alias (and any reserved name) to its opaque sealed value.
type envelopeKeyringFile struct {
	KeyringID  string            `json:"keyring_id"`
	Generation int64             `json:"generation"`
	Entries    map[string]string `json:"entries"`
}

// envelopeFileKeyring is the file backend for a vault with password
// slots: a plaintext (mode 0600) container of opaque BORUE ciphertext.
// It needs no passphrase of its own — confidentiality comes from the
// per-namespace envelope applied above it (keyslot.go).
type envelopeFileKeyring struct {
	folder string
}

func (f *envelopeFileKeyring) Name() string { return BackendFile }
func (f *envelopeFileKeyring) path() string { return filepath.Join(f.folder, vaultFileName("keyring")) }

func (f *envelopeFileKeyring) loadFile() (*envelopeKeyringFile, error) {
	data, err := os.ReadFile(f.path())
	if err != nil {
		if os.IsNotExist(err) {
			return &envelopeKeyringFile{Entries: map[string]string{}}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return &envelopeKeyringFile{Entries: map[string]string{}}, nil
	}
	magicLen, ok := keyringMagicLen(data)
	if !ok {
		return nil, errors.New("vault: keyring file has no recognizable header")
	}
	switch format := data[magicLen]; format {
	case envelopeKeyringFormat:
		var ekf envelopeKeyringFile
		if err := json.Unmarshal(data[magicLen+1:], &ekf); err != nil {
			return nil, fmt.Errorf("vault: parsing keyring: %w", err)
		}
		if ekf.Entries == nil {
			ekf.Entries = map[string]string{}
		}
		return &ekf, nil
	case keyringFormat:
		return nil, errors.New("vault: keyring is in legacy single-passphrase format but the vault has password slots; run `boru vault verify`")
	default:
		return nil, fmt.Errorf("vault: keyring is format %d but this boru understands up to %d; upgrade boru", format, envelopeKeyringFormat)
	}
}

func (f *envelopeFileKeyring) saveFile(ekf *envelopeKeyringFile) error {
	if err := os.MkdirAll(f.folder, 0700); err != nil {
		return err
	}
	if ekf.Entries == nil {
		ekf.Entries = map[string]string{}
	}
	if ekf.KeyringID == "" {
		id, err := p7randomID()
		if err != nil {
			return err
		}
		ekf.KeyringID = id
	}
	ekf.Generation++
	body, err := jsonMarshal(ekf)
	if err != nil {
		return err
	}
	out := make([]byte, 0, len(keyringMagic)+1+len(body))
	out = append(out, keyringMagic...)
	out = append(out, byte(envelopeKeyringFormat))
	out = append(out, body...)
	return writeFileAtomic(f.path(), out, 0600)
}

func (f *envelopeFileKeyring) Set(alias, value string) error {
	ekf, err := f.loadFile()
	if err != nil {
		return err
	}
	ekf.Entries[alias] = value
	return f.saveFile(ekf)
}

func (f *envelopeFileKeyring) Get(alias string) (string, error) {
	ekf, err := f.loadFile()
	if err != nil {
		return "", err
	}
	v, ok := ekf.Entries[alias]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *envelopeFileKeyring) Delete(alias string) error {
	ekf, err := f.loadFile()
	if err != nil {
		return err
	}
	if _, ok := ekf.Entries[alias]; !ok {
		return nil
	}
	delete(ekf.Entries, alias)
	return f.saveFile(ekf)
}

// list returns user-visible aliases only, excluding the reserved
// keyspace so `vault verify --prune` never treats the integrity key or
// anchor as an orphan to delete.
func (f *envelopeFileKeyring) list() ([]string, error) {
	ekf, err := f.loadFile()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ekf.Entries))
	for k := range ekf.Entries {
		if isReservedKeyringKey(k) {
			continue
		}
		out = append(out, k)
	}
	return out, nil
}

// keyringIdentity returns the keyring's stable id and current
// generation, or ("", 0) if the keyring file does not yet exist. Restore
// uses these to detect store<->keyring divergence.
func (f *envelopeFileKeyring) keyringIdentity() (id string, generation int64, err error) {
	ekf, err := f.loadFile()
	if err != nil {
		return "", 0, err
	}
	return ekf.KeyringID, ekf.Generation, nil
}

// keyringFileFormat reports the on-disk format of the keyring file in
// folder: 0 if absent/empty, 1 for the legacy encrypted blob (headered
// or pre-header), 2 for the envelope container. Used to choose the
// backend and to drive migration.
func keyringFileFormat(folder string) (int, error) {
	data, err := os.ReadFile(filepath.Join(folder, vaultFileName("keyring")))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}
	magicLen, ok := keyringMagicLen(data)
	if !ok {
		return 1, nil // legacy headerless blob
	}
	return int(data[magicLen]), nil
}

// scryptKey derives a 32-byte AES-256 key from passphrase + salt.
// The N=2^15 cost is conservative for an interactive vault on a
// developer machine; it dominates each Set/Get on the file backend
// by ~tens of milliseconds, which is acceptable.
func scryptKey(passphrase string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(passphrase), salt, 1<<15, 8, 1, 32)
}

const (
	// keyringMagic prefixes a self-describing keyring file, and is the
	// only magic this binary WRITES. Every magic it can read — including
	// the ones earlier, differently-named releases wrote — comes from
	// wire.KeyringMagics(). Older files still exist without any magic (a
	// raw salt|nonce|ciphertext blob) and remain readable too; all of them
	// are re-stamped in the current format on the next save.
	keyringMagic = wire.KeyringMagic
	// keyringFormat is the keyring layout version this binary writes.
	// Bump it (and add a branch in decryptHeadered) when the KDF,
	// cipher, or byte layout changes.
	//
	//	1 — scrypt(N=2^15,r=8,p=1) + AES-256-GCM, header-bound AAD.
	keyringFormat   = 1
	keyringSaltLen  = 16
	keyringNonceLen = 12
)

// keyringMagicLen matches blob's leading magic against every spelling this
// binary can read, returning the MATCHED magic's length so callers can find
// the format byte that follows it. The spellings differ in length, so every
// reader must take its offset from here rather than assume
// len(keyringMagic).
func keyringMagicLen(blob []byte) (int, bool) {
	return wire.MatchPrefix(blob, wire.KeyringMagics(), 1)
}

// Headered layout: "BORUK" | format(1 byte) | salt(16) | nonce(12) | ciphertext|tag.
// The GCM additional data is the header bytes + salt, so the format
// byte is authenticated and cannot be downgraded by tampering.
func encryptBlob(plain []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, keyringSaltLen)
	if _, err := randRead(salt); err != nil {
		return nil, err
	}
	key, err := p7scryptKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := gcmFromBlock(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := randRead(nonce); err != nil {
		return nil, err
	}
	header := append([]byte(keyringMagic), byte(keyringFormat))
	ct := gcm.Seal(nil, nonce, plain, keyringAAD(header, salt))
	out := make([]byte, 0, len(header)+len(salt)+len(nonce)+len(ct))
	out = append(out, header...)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// keyringAAD binds the header and salt into the AEAD additional data.
func keyringAAD(header, salt []byte) []byte {
	aad := make([]byte, 0, len(header)+len(salt))
	aad = append(aad, header...)
	aad = append(aad, salt...)
	return aad
}

func decryptBlob(blob []byte, passphrase string) ([]byte, error) {
	if n, ok := keyringMagicLen(blob); ok {
		return decryptHeadered(blob, n, passphrase)
	}
	return decryptLegacy(blob, passphrase)
}

// decryptHeadered reads the self-describing format, dispatching on the
// format byte. A newer format than this binary understands is reported
// clearly rather than surfaced as a generic "corrupt keyring".
func decryptHeadered(blob []byte, magicLen int, passphrase string) ([]byte, error) {
	off := magicLen + 1 // magic + format byte
	format := int(blob[magicLen])
	if format > keyringFormat {
		return nil, fmt.Errorf("vault: keyring is format %d but this boru understands up to %d; upgrade boru", format, keyringFormat)
	}
	if format != 1 {
		return nil, fmt.Errorf("vault: unknown keyring format %d", format)
	}
	if len(blob) < off+keyringSaltLen+keyringNonceLen+16 {
		return nil, errors.New("vault: keyring file is truncated")
	}
	salt := blob[off : off+keyringSaltLen]
	nonce := blob[off+keyringSaltLen : off+keyringSaltLen+keyringNonceLen]
	ct := blob[off+keyringSaltLen+keyringNonceLen:]
	plain, err := aesGCMOpen(passphrase, salt, nonce, ct, keyringAAD(blob[:off], salt))
	if err != nil {
		return nil, err
	}
	return plain, nil
}

// decryptLegacy reads the original headerless salt|nonce|ciphertext
// layout (AAD = salt). Retained so vaults written before the format
// header remain readable; they are upgraded on the next save.
func decryptLegacy(blob []byte, passphrase string) ([]byte, error) {
	if len(blob) < keyringSaltLen+keyringNonceLen+16 {
		return nil, errors.New("vault: keyring file is truncated")
	}
	salt := blob[:keyringSaltLen]
	nonce := blob[keyringSaltLen : keyringSaltLen+keyringNonceLen]
	ct := blob[keyringSaltLen+keyringNonceLen:]
	return aesGCMOpen(passphrase, salt, nonce, ct, salt)
}

// aesGCMOpen derives the scrypt key and opens the GCM ciphertext,
// mapping any authentication failure to a single non-revealing error.
func aesGCMOpen(passphrase string, salt, nonce, ct, aad []byte) ([]byte, error) {
	key, err := p7scryptKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := gcmFromBlock(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, errors.New("vault: wrong passphrase or corrupt keyring")
	}
	return plain, nil
}

// --- 1Password via `op` CLI -------------------------------------------------

// onePasswordKeyring stores secrets in the 1Password vault named
// by BORU_VAULT_1PASSWORD_VAULT (default "Private"). Each alias
// becomes an "API Credential" item titled "boru:<alias>". The user
// must have an active `op signin` session for the `op` CLI calls
// to succeed; this is the responsibility of the host.
type onePasswordKeyring struct {
	vault string
}

func (k *onePasswordKeyring) Name() string { return BackendOnePassword }

func (k *onePasswordKeyring) itemTitle(alias string) string {
	return "boru:" + alias
}

func (k *onePasswordKeyring) opVault() string {
	if k.vault == "" {
		return "Private"
	}
	return k.vault
}

// opItemTemplate / opField are the minimal subset of the 1Password
// item-create JSON template needed to store a single concealed
// credential. The value rides in this JSON (delivered on stdin), never
// on argv.
type opItemTemplate struct {
	Title    string    `json:"title"`
	Category string    `json:"category"`
	Fields   []opField `json:"fields"`
}

type opField struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Label string `json:"label"`
	Value string `json:"value"`
}

func (k *onePasswordKeyring) Set(alias, value string) error {
	// Best-effort delete of any prior item first. This uses the item
	// title, not the secret, so nothing sensitive reaches argv here.
	_ = exec.Command("op", "item", "delete", k.itemTitle(alias),
		"--vault", k.opVault()).Run()

	// Create from a JSON template piped on stdin (`op item create -`)
	// so the credential value never appears on this process's — or
	// op's — argv, where a same-user `ps` could read it. The label
	// "credential" matches what Get reads back.
	blob, err := jsonMarshal(opItemTemplate{
		Title:    k.itemTitle(alias),
		Category: "API_CREDENTIAL",
		Fields:   []opField{{ID: "credential", Type: "CONCEALED", Label: "credential", Value: value}},
	})
	if err != nil {
		return err
	}
	cmd := exec.Command("op", "item", "create", "--vault", k.opVault(), "-")
	cmd.Stdin = bytes.NewReader(blob)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("op item create: %s: %s", err, strings.TrimSpace(out.String()))
	}
	return nil
}

func (k *onePasswordKeyring) Get(alias string) (string, error) {
	cmd := exec.Command("op", "item", "get", k.itemTitle(alias),
		"--vault", k.opVault(), "--fields", "label=credential", "--reveal")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && strings.Contains(string(ee.Stderr), "isn't an item") {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("op item get: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (k *onePasswordKeyring) Delete(alias string) error {
	cmd := exec.Command("op", "item", "delete", k.itemTitle(alias),
		"--vault", k.opVault())
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "isn't an item") {
			return nil
		}
		return fmt.Errorf("op item delete: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
