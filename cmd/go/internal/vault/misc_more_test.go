package vault

// misc_more_test.go — clipboard command execution (against stub tools),
// the exec-based keyring backends driven by PATH stubs (never a real
// keychain), scope/integrity/store/theme/session helpers, and the root
// model's message plumbing.

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubDir creates an executable shell-script stub named bin in a fresh
// dir (shared per test via dir) so exec.LookPath resolves it first. A
// `#!/bin/sh` stub cannot run on Windows (exec finds executables by
// extension), so a test that stubs a tool skips there; the backends these
// stub — keychain, secret-service, 1Password, the Credential Manager's
// powershell bridge — are exercised against the real tools by their own
// platform's runs.
func stubBin(t *testing.T, dir, bin, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("tool stubs are /bin/sh scripts, which Windows cannot execute")
	}
	if err := os.WriteFile(filepath.Join(dir, bin), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// prependPath puts dir at the front of PATH for the test.
func prependPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// --- clipboard command execution ----------------------------------------

func TestClipboardCmdExecution(t *testing.T) {
	// paste trims surrounding whitespace and trailing newlines.
	c := &clipboardCmd{label: "t", pasteArgv: []string{"echo", " hi "}}
	if got, err := c.paste(); err != nil || got != "hi" {
		t.Errorf("paste = %q, %v", got, err)
	}
	bad := &clipboardCmd{label: "t", pasteArgv: []string{"false"}}
	if _, err := bad.paste(); err == nil {
		t.Error("failing paste tool should error")
	}
	// clear, with and without the empty-stdin convention.
	c = &clipboardCmd{label: "t", clearArgv: []string{"true"}}
	if err := c.clear(); err != nil {
		t.Errorf("clear: %v", err)
	}
	c = &clipboardCmd{label: "t", clearArgv: []string{"cat"}, clearStdin: true}
	if err := c.clear(); err != nil {
		t.Errorf("clear with stdin: %v", err)
	}
	if err := (&clipboardCmd{label: "t", clearArgv: []string{"false"}}).clear(); err == nil {
		t.Error("failing clear tool should error")
	}
	// copy feeds stdin; a missing copy tool is a clean error.
	c = &clipboardCmd{label: "t", copyArgv: []string{"cat"}}
	if err := c.copy("text"); err != nil {
		t.Errorf("copy: %v", err)
	}
	if err := (&clipboardCmd{label: "t", copyArgv: []string{"false"}}).copy("x"); err == nil {
		t.Error("failing copy tool should error")
	}
	if err := (&clipboardCmd{label: "t"}).copy("x"); err == nil ||
		!strings.Contains(err.Error(), "not supported") {
		t.Errorf("copy without argv: %v", err)
	}
}

func TestClearClipboardAfterStore(t *testing.T) {
	var out, errb bytes.Buffer
	// No tools at all: an advisory warning, never a failure.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	clearClipboardAfterStore(&out, &errb)
	if !strings.Contains(errb.String(), "could not clear clipboard") {
		t.Errorf("missing warning: %q", errb.String())
	}
	// With a working stub tool the clipboard is wiped and announced. The stub
	// matches the host's real clipboard toolchain (pbcopy on macOS, xclip on
	// Linux) so this is portable rather than Linux-only.
	dir := t.TempDir()
	label := clipStubScripts(t, dir, "exit 0", "cat > /dev/null; exit 0")
	t.Setenv("PATH", dir)
	out.Reset()
	errb.Reset()
	clearClipboardAfterStore(&out, &errb)
	if !strings.Contains(out.String(), "clipboard cleared ("+label+")") {
		t.Errorf("clear not announced: %q / %q", out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "clipboard manager") {
		t.Errorf("missing the clipboard-manager caveat: %q", out.String())
	}
	// A tool that fails to clear degrades to a warning too.
	clipStubScripts(t, dir, "exit 0", "exit 1")
	out.Reset()
	errb.Reset()
	clearClipboardAfterStore(&out, &errb)
	if !strings.Contains(errb.String(), "could not clear clipboard") {
		t.Errorf("failed clear should warn: %q", errb.String())
	}
}

func TestAddFromClipboardWithoutTool(t *testing.T) {
	testHome(t)
	mustInit(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	if code, _, errOut := runVault(t, "", "add", "--from-clipboard", "k"); code == 0 ||
		!strings.Contains(errOut, "clipboard") {
		t.Errorf("clipboard add without a tool: %q", errOut)
	}
}

// --- exec-based keyring backends against PATH stubs ----------------------

func TestSecretServiceViaStub(t *testing.T) {
	dir := t.TempDir()
	stubBin(t, dir, "secret-tool", `case "$1" in
store) cat > /dev/null; exit 0;;
lookup)
  case "$BORU_STUB_LOOKUP" in
    value) printf 'sv\n'; exit 0;;
    empty) exit 0;;
    *) exit 1;;
  esac;;
clear) exit 0;;
esac
exit 3
`)
	prependPath(t, dir)
	var k secretService
	if err := k.Set("a", "v"); err != nil {
		t.Errorf("stub Set: %v", err)
	}
	t.Setenv("BORU_STUB_LOOKUP", "value")
	if got, err := k.Get("a"); err != nil || got != "sv" {
		t.Errorf("stub Get = %q, %v", got, err)
	}
	t.Setenv("BORU_STUB_LOOKUP", "empty")
	if _, err := k.Get("a"); err != ErrNotFound {
		t.Errorf("empty lookup = %v, want ErrNotFound", err)
	}
	t.Setenv("BORU_STUB_LOOKUP", "missing")
	if _, err := k.Get("a"); err != ErrNotFound {
		t.Errorf("exit-1 lookup = %v, want ErrNotFound", err)
	}
	if err := k.Delete("a"); err != nil {
		t.Errorf("stub Delete: %v", err)
	}
	// With the stub on PATH, selection and auto-detection see the backend.
	if kr, err := selectKeyring(BackendSecretService, dir, ""); err != nil ||
		kr.Name() != BackendSecretService {
		t.Errorf("selectKeyring with stub: %v", err)
	}
	// autoBackend's precedence is GOOS-specific — Linux prefers secret-service
	// when secret-tool is present, macOS prefers its native keychain. Force the
	// Linux dispatch so the stubbed secret-tool is what gets detected (on a
	// real macOS host the `security` keychain would otherwise win).
	p7swapGoos(t, "linux")
	if got := autoBackend(); got != BackendSecretService {
		t.Errorf("autoBackend with stub secret-tool = %q", got)
	}
}

func TestMacKeychainViaStub(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BORU_STUB_DIR", dir)
	// The stub honors macSetCmd's stdin protocol: it de-escapes the -w
	// argument and stores it, so Set's round-trip check passes.
	stubBin(t, dir, "security", `case "$1" in
-i)
  line=$(cat)
  printf '%s' "$line" | sed -e 's/^.* -w //' -e 's/\\\(.\)/\1/g' > "$BORU_STUB_DIR/secval"
  exit 0;;
find-generic-password)
  if [ -f "$BORU_STUB_DIR/secval" ]; then cat "$BORU_STUB_DIR/secval"; echo; exit 0
  else echo "could not be found" >&2; exit 44; fi;;
delete-generic-password)
  echo "security: the item could not be found"; exit 1;;
esac
exit 3
`)
	prependPath(t, dir)
	var k macKeychain
	if _, err := k.Get("a"); err != ErrNotFound {
		t.Errorf("missing entry = %v, want ErrNotFound", err)
	}
	if err := k.Set("a", "sk-value_1"); err != nil {
		t.Errorf("stub Set: %v", err)
	}
	if got, err := k.Get("a"); err != nil || got != "sk-value_1" {
		t.Errorf("stub Get = %q, %v", got, err)
	}
	// Delete of a missing item is a no-op by message.
	if err := k.Delete("a"); err != nil {
		t.Errorf("stub Delete: %v", err)
	}
}

func TestWinCredViaStub(t *testing.T) {
	dir := t.TempDir()
	stubBin(t, dir, "powershell", `case "$BORU_KR_OP" in
set) cat > /dev/null; exit 0;;
get)
  case "$BORU_STUB_WINCRED_GET" in
    value) printf 'wv'; exit 0;;
    *) exit 2;;
  esac;;
delete) exit 0;;
esac
exit 3
`)
	prependPath(t, dir)
	var k winCred
	// Round-trip success: the stub always reads back "wv".
	t.Setenv("BORU_STUB_WINCRED_GET", "value")
	if err := k.Set("a", "wv"); err != nil {
		t.Errorf("stub Set: %v", err)
	}
	if got, err := k.Get("a"); err != nil || got != "wv" {
		t.Errorf("stub Get = %q, %v", got, err)
	}
	// A value that doesn't round-trip is refused and cleaned up.
	if err := k.Set("a", "different"); err == nil ||
		!strings.Contains(err.Error(), "did not round-trip") {
		t.Errorf("round-trip mismatch: %v", err)
	}
	// Read-back failure is also refused.
	t.Setenv("BORU_STUB_WINCRED_GET", "notfound")
	if _, err := k.Get("a"); err != ErrNotFound {
		t.Errorf("exit-2 get = %v, want ErrNotFound", err)
	}
	if err := k.Set("a", "wv"); err == nil ||
		!strings.Contains(err.Error(), "could not read it back") {
		t.Errorf("read-back failure: %v", err)
	}
	if err := k.Delete("a"); err != nil {
		t.Errorf("stub Delete: %v", err)
	}
}

func TestOnePasswordViaStub(t *testing.T) {
	dir := t.TempDir()
	stubBin(t, dir, "op", `verb="$2"
case "$verb" in
create) cat > /dev/null; exit 0;;
get)
  case "$BORU_STUB_OP_GET" in
    value) printf 'opv\n'; exit 0;;
    *) echo "\"boru:a\" isn't an item" >&2; exit 1;;
  esac;;
delete) echo "\"boru:a\" isn't an item" >&2; exit 1;;
esac
exit 3
`)
	prependPath(t, dir)
	k := &onePasswordKeyring{}
	if err := k.Set("a", "opv"); err != nil {
		t.Errorf("stub Set: %v", err)
	}
	t.Setenv("BORU_STUB_OP_GET", "value")
	if got, err := k.Get("a"); err != nil || got != "opv" {
		t.Errorf("stub Get = %q, %v", got, err)
	}
	t.Setenv("BORU_STUB_OP_GET", "notfound")
	if _, err := k.Get("a"); err != ErrNotFound {
		t.Errorf("isn't-an-item get = %v, want ErrNotFound", err)
	}
	// Delete of a missing item is success by message.
	if err := k.Delete("a"); err != nil {
		t.Errorf("stub Delete: %v", err)
	}
	if kr, err := selectKeyring(BackendOnePassword, dir, ""); err != nil ||
		kr.Name() != BackendOnePassword {
		t.Errorf("selectKeyring with op stub: %v", err)
	}
}

// --- scope / integrity / audit helpers -----------------------------------

func TestScopeModel(t *testing.T) {
	if OpRead.String() != "read" || OpWrite.String() != "write" ||
		OpMove.String() != "move" || OpAdmin.String() != "admin" || Op(99).String() != "unknown" {
		t.Error("Op.String broken")
	}
	if !validScope(ScopeAdmin) || !validScope(ScopeRead) || validScope("root") {
		t.Error("validScope broken")
	}
	allow := []struct {
		scope string
		op    Op
		want  bool
	}{
		{ScopeAdmin, OpAdmin, true}, {ScopeAdmin, OpMove, true},
		{ScopeWrite, OpRead, true}, {ScopeWrite, OpWrite, true},
		{ScopeWrite, OpMove, false}, {ScopeWrite, OpAdmin, false},
		{ScopeMove, OpMove, true}, {ScopeMove, OpWrite, false},
		{ScopeRead, OpRead, true}, {ScopeRead, OpWrite, false},
		{"bogus", OpRead, false},
	}
	for _, tc := range allow {
		if got := scopeAllows(tc.scope, tc.op); got != tc.want {
			t.Errorf("scopeAllows(%q,%s) = %v, want %v", tc.scope, tc.op, got, tc.want)
		}
	}
	if !slotCoversNamespace(nil, "any") {
		t.Error("legacy slot covers everything")
	}
	if !slotCoversNamespace(&PasswordSlot{}, "x") {
		t.Error("empty allow-list covers everything")
	}
	if !slotCoversNamespace(&PasswordSlot{Namespaces: []string{"*"}}, "x") {
		t.Error("star covers everything")
	}
	s := &PasswordSlot{Namespaces: []string{"proj"}}
	if !slotCoversNamespace(s, "proj") || slotCoversNamespace(s, "other") {
		t.Error("named list broken")
	}
}

func TestStoreIntegrityString(t *testing.T) {
	want := map[StoreIntegrity]string{
		IntegrityOK:             "ok",
		IntegrityUnverified:     "unverified",
		IntegrityNoBaseline:     "no-baseline",
		IntegrityExternalChange: "external-change",
		IntegrityCorrupt:        "corrupt",
		IntegrityHMACInvalid:    "hmac-invalid",
		IntegrityRollback:       "rollback-detected",
		StoreIntegrity(99):      "unknown",
	}
	for k, v := range want {
		if k.String() != v {
			t.Errorf("StoreIntegrity(%d).String() = %q, want %q", k, k.String(), v)
		}
	}
}

func TestAuditHelpers(t *testing.T) {
	if shortCap("abcdef0123456789") != "abcdef01" {
		t.Error("shortCap should trim to 8")
	}
	if shortCap("abc") != "abc" {
		t.Error("shortCap should keep short ids")
	}
	if defaultActor() == "" {
		t.Error("defaultActor should never be empty")
	}
}

// --- store record helpers -------------------------------------------------

func TestStoreSlotAndAliasHelpers(t *testing.T) {
	s := &Store{}
	// Upsert inserts, stamping CreatedAt.
	s.UpsertPasswordSlot(PasswordSlot{Name: "a", Scope: ScopeRead})
	if len(s.Passwords) != 1 || s.Passwords[0].CreatedAt == "" {
		t.Fatalf("insert: %+v", s.Passwords)
	}
	created := s.Passwords[0].CreatedAt
	// Upsert replaces by name, preserving CreatedAt and stamping UpdatedAt.
	s.UpsertPasswordSlot(PasswordSlot{Name: "a", Scope: ScopeWrite})
	if len(s.Passwords) != 1 || s.Passwords[0].Scope != ScopeWrite {
		t.Fatalf("replace: %+v", s.Passwords)
	}
	if s.Passwords[0].CreatedAt != created || s.Passwords[0].UpdatedAt == "" {
		t.Errorf("timestamps wrong: %+v", s.Passwords[0])
	}
	// A pre-set CreatedAt is honored on insert.
	s.UpsertPasswordSlot(PasswordSlot{Name: "b", CreatedAt: "2020-01-01T00:00:00Z"})
	if s.Passwords[1].CreatedAt != "2020-01-01T00:00:00Z" {
		t.Errorf("explicit CreatedAt overwritten: %+v", s.Passwords[1])
	}
	// Remove of a missing slot reports false.
	if s.RemovePasswordSlot("ghost") {
		t.Error("removing a missing slot should be false")
	}

	// OtherFunctionalAdminExists is identity-aware.
	s = &Store{Passwords: []PasswordSlot{
		{Name: "admin", Scope: ScopeAdmin},
		{Name: "reader", Scope: ScopeRead},
	}}
	if s.OtherFunctionalAdminExists("admin") {
		t.Error("the only admin has no other admin")
	}
	if !s.OtherFunctionalAdminExists("reader") {
		t.Error("removing the reader leaves the admin")
	}
	s.Passwords = append(s.Passwords, PasswordSlot{Name: "admin2", Scope: ScopeAdmin})
	if !s.OtherFunctionalAdminExists("admin") {
		t.Error("a second admin exists")
	}

	// RemoveAlias drops the alias and its capabilities.
	s = &Store{
		Aliases:      []Alias{{Name: "k"}, {Name: "other"}},
		Capabilities: []Capability{{ID: "1", Alias: "k"}, {ID: "2", Alias: "other"}},
	}
	if !s.RemoveAlias("k") {
		t.Fatal("remove existing")
	}
	if len(s.Aliases) != 1 || len(s.Capabilities) != 1 || s.Capabilities[0].Alias != "other" {
		t.Errorf("scoped capability not dropped: %+v %+v", s.Aliases, s.Capabilities)
	}
	if s.RemoveAlias("ghost") {
		t.Error("removing a missing alias should be false")
	}

	// FindCapabilityByToken never matches an empty token or empty hash.
	if c, _ := s.FindCapabilityByToken(""); c != nil {
		t.Error("empty token must not match")
	}
	s.Capabilities = append(s.Capabilities, Capability{ID: "3", Alias: "x"}) // no TokenHash
	if c, _ := s.FindCapabilityByToken("anything"); c != nil {
		t.Error("hashless capability must not match")
	}
}

// --- theme prefs -----------------------------------------------------------

func TestTUIPrefsRoundTrip(t *testing.T) {
	home := t.TempDir()
	// Missing / corrupt files yield defaults.
	if p := loadTUIPrefs(home); p.Theme != "" {
		t.Errorf("missing prefs = %+v", p)
	}
	if err := saveTUIPrefs("", tuiPrefs{Theme: "dark"}); err != nil {
		t.Errorf("empty home save should be a silent no-op: %v", err)
	}
	if err := saveTUIPrefs(home, tuiPrefs{Theme: "dark"}); err != nil {
		t.Fatal(err)
	}
	if p := loadTUIPrefs(home); p.Theme != "dark" {
		t.Errorf("round-trip = %+v", p)
	}
	if err := os.WriteFile(tuiPrefsPath(home), []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := loadTUIPrefs(home); p.Theme != "" {
		t.Errorf("corrupt prefs = %+v", p)
	}
}

// --- session / keyring opening ---------------------------------------------

func TestReadPassphraseSources(t *testing.T) {
	t.Setenv(EnvPassphrase, "from-env")
	if p, err := readPassphrase(nil, nil, ""); err != nil || p != "from-env" {
		t.Errorf("env = %q, %v", p, err)
	}
	t.Setenv(EnvPassphrase, "")
	if _, err := readPassphrase(nil, nil, ""); err == nil ||
		!strings.Contains(err.Error(), EnvPassphrase) {
		t.Errorf("nil stdin should name the env var: %v", err)
	}
	var sink bytes.Buffer
	if p, err := readPassphrase(strings.NewReader("typed\n"), &sink, "P: "); err != nil || p != "typed" {
		t.Errorf("prompt = %q, %v", p, err)
	}
	if _, err := readPassphrase(strings.NewReader("\n"), &sink, "P: "); err == nil ||
		!strings.Contains(err.Error(), "empty") {
		t.Errorf("empty typed: %v", err)
	}
}

func TestAuthenticateWithBackendResolution(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	s, _ := LoadStore(home)
	// Legacy file vault: an empty passphrase is refused, a non-empty one
	// yields an implicit-admin session.
	if _, err := authenticateWith(s, home, ""); err == nil ||
		!strings.Contains(err.Error(), "passphrase") {
		t.Errorf("empty pass: %v", err)
	}
	sess, err := authenticateWith(s, home, "test-pass")
	if err != nil || sess.Scope != ScopeAdmin || !sess.legacy() {
		t.Fatalf("legacy session: %+v, %v", sess, err)
	}
	sess.Close()
	// A host backend that cannot be opened propagates the error. Empty PATH so
	// the keychain tool (`security`) is unresolvable on any host — otherwise a
	// real macOS keychain would open and the arm would not be exercised.
	t.Setenv("PATH", t.TempDir())
	broken := &Store{Backend: BackendKeychain}
	if _, err := authenticateWith(broken, home, ""); err == nil {
		t.Error("unavailable host backend should error")
	}
	if _, err := openEnvelopeKeyring(broken, home); err == nil {
		t.Error("openEnvelopeKeyring on an unavailable backend should error")
	}
	if kr, err := openEnvelopeKeyring(&Store{Backend: BackendFile}, home); err != nil ||
		kr.Name() != BackendFile {
		t.Errorf("file envelope keyring: %v", err)
	}
}

func TestOpenKeyringPromptPath(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	s, _ := LoadStore(home)
	t.Setenv(EnvPassphrase, "")
	var sink bytes.Buffer
	// The passphrase is prompted from stdin when the env is unset.
	kr, err := openKeyring(s, home, strings.NewReader("test-pass\n"), &sink, "Vault passphrase: ")
	if err != nil {
		t.Fatalf("prompted openKeyring: %v", err)
	}
	if kr.Name() != BackendFile {
		t.Errorf("backend = %q", kr.Name())
	}
	if !strings.Contains(sink.String(), "Vault passphrase") {
		t.Errorf("prompt not written: %q", sink.String())
	}
	// No env, no stdin: a clear error.
	if _, err := openKeyring(s, home, nil, &sink, "P: "); err == nil ||
		!strings.Contains(err.Error(), "requires a passphrase") {
		t.Errorf("no source: %v", err)
	}
	// A host backend skips the passphrase entirely (and errors here since none
	// is available). Empty PATH so the keychain tool is unresolvable on any
	// host — otherwise a real macOS keychain would open successfully.
	t.Setenv("PATH", t.TempDir())
	if _, err := openKeyring(&Store{Backend: BackendKeychain}, home, nil, &sink, ""); err == nil {
		t.Error("host backend should fail on this machine")
	}
}

func TestWriteFileAtomicErrors(t *testing.T) {
	if err := writeFileAtomic(filepath.Join(t.TempDir(), "no-such-dir", "f"), []byte("x"), 0o600); err == nil {
		t.Error("missing parent dir should error")
	}
}

// --- root model message plumbing --------------------------------------------

func TestModelMessagePlumbing(t *testing.T) {
	m := newTestModel(t)

	// push / pop / popToRoot.
	m = feed(t, m, pushMsg{m.buildSecrets()})
	m = feed(t, m, pushMsg{m.buildAccess()})
	if len(m.stack) != 3 {
		t.Fatalf("depth = %d", len(m.stack))
	}
	m = feed(t, m, popMsg{})
	if len(m.stack) != 2 {
		t.Fatalf("pop -> %d", len(m.stack))
	}
	m = feed(t, m, popToRootMsg{})
	if len(m.stack) != 1 {
		t.Fatalf("popToRoot -> %d", len(m.stack))
	}

	// Status line messages, both flavors, rendered by statusView.
	if m.statusView() != "" {
		t.Error("no status yet")
	}
	m = feed(t, m, setStatusMsg{text: "did it", err: false})
	if !strings.Contains(m.statusView(), "did it") || m.statusIsErr {
		t.Errorf("ok status: %q", m.statusView())
	}
	m = feed(t, m, opResultMsg{err: errAborted})
	if !m.statusIsErr || !strings.Contains(m.statusView(), "aborted") {
		t.Errorf("err status: %q", m.statusView())
	}
	m = feed(t, m, opResultMsg{okText: "done"})
	if m.statusIsErr {
		t.Error("ok result should clear the error flag")
	}

	// vaultChanged / reloadTop refresh without panicking.
	m = feed(t, m, vaultChangedMsg{})
	m = feed(t, m, reloadTopMsg{})

	// switchedToVault resets the stack to Home and reports the switch via
	// a status command.
	m = feed(t, m, pushMsg{m.buildSecrets()})
	nm0, cmd0 := m.Update(switchedToVaultMsg{name: "default"})
	m = nm0.(*rootModel)
	if len(m.stack) != 1 {
		t.Errorf("switch should reset to home, depth=%d", len(m.stack))
	}
	var switched bool
	for _, msg := range collectMsgs(cmd0) {
		if st, ok := msg.(setStatusMsg); ok && strings.Contains(st.text, "switched to default") {
			switched = true
		}
	}
	if !switched {
		t.Error("switch status command missing")
	}

	// grantedMsg pushes the one-time-token pager.
	nm, cmd := m.Update(grantedMsg{output: "token: xyz"})
	m = nm.(*rootModel)
	msgs := collectMsgs(cmd)
	var pushed bool
	for _, msg := range msgs {
		if pm, ok := msg.(pushMsg); ok && pm.s.Title() == "granted" {
			pushed = true
		}
	}
	if !pushed {
		t.Errorf("grantedMsg should push the granted pager: %+v", msgs)
	}

	// Unhandled messages route to the top screen without changing depth.
	m = feed(t, m, secretRevealedMsg{value: "x"})
	if len(m.stack) != 1 {
		t.Errorf("unhandled message changed the stack: depth=%d", len(m.stack))
	}
}

func TestModelPaletteOpenTypeCancelAndRun(t *testing.T) {
	m := newTestModel(t)
	m = feed(t, m, keyMsg(":"))
	if !m.paletteOpen {
		t.Fatal("palette should open")
	}
	m = feed(t, m, keyMsg("a"))
	if m.palette.Value() != "a" {
		t.Errorf("typed value = %q", m.palette.Value())
	}
	// esc cancels (closePalette).
	m = feed(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.paletteOpen {
		t.Fatal("esc should close the palette")
	}
	// Reopen, type a command, enter runs it.
	m = feed(t, m, keyMsg(":"))
	for _, r := range "secrets" {
		m = feed(t, m, keyMsg(string(r)))
	}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*rootModel)
	if m.paletteOpen {
		t.Error("enter should close the palette")
	}
	if got := pushedTitle(cmd); got != "secrets" {
		t.Errorf("palette enter pushed %q", got)
	}
}

func TestModelThemeKeyAndVaultLine(t *testing.T) {
	m := newTestModel(t)
	before := m.theme
	nm, cmd := m.Update(keyMsg("T"))
	m = nm.(*rootModel)
	if m.theme == before {
		t.Error("T should cycle the theme")
	}
	var themed bool
	for _, msg := range collectMsgs(cmd) {
		if st, ok := msg.(setStatusMsg); ok && strings.Contains(st.text, "theme:") {
			themed = true
		}
	}
	if !themed {
		t.Error("theme status command missing")
	}
	// The no-vault variant of the vault line.
	m.vaultOK = false
	if vl := m.vaultLine(); !strings.Contains(vl, "(no vault)") {
		t.Errorf("no-vault line: %q", vl)
	}
	// shortenHome falls through for foreign paths.
	if shortenHome("/other/place", "/home/u") != "/other/place" {
		t.Error("shortenHome should not rewrite foreign paths")
	}
	if shortenHome("/home/u/x", "") != "/home/u/x" {
		t.Error("empty home should not rewrite")
	}
}

func TestFormScreenHelpAndViewGlue(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	fs := m.buildAddForm().(*formScreen)
	if fs.helpInfo() != "" {
		t.Errorf("add form helpInfo = %q", fs.helpInfo())
	}
	fs.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if v := fs.View(80, 24); !strings.Contains(v, "Alias") {
		t.Errorf("form view missing fields: %.80q", v)
	}
}
