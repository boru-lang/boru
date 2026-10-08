package vault

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// --- predicates & labels ----------------------------------------------------

func TestSlotExpired(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name    string
		exp     string
		expired bool
	}{
		{"empty never expires", "", false},
		{"future", now.Add(time.Hour).UTC().Format(time.RFC3339), false},
		{"past", now.Add(-time.Hour).UTC().Format(time.RFC3339), true},
		{"unparseable fails closed", "not-a-timestamp", true},
	}
	for _, c := range cases {
		if got := slotExpired(&PasswordSlot{ExpiresAt: c.exp}, now); got != c.expired {
			t.Errorf("%s: slotExpired(%q) = %v, want %v", c.name, c.exp, got, c.expired)
		}
	}
}

func TestExpiryLabel(t *testing.T) {
	if got := expiryLabel(""); got != "-" {
		t.Errorf("empty => %q, want -", got)
	}
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if got := expiryLabel(future); got != future {
		t.Errorf("future => %q, want raw %q", got, future)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if got := expiryLabel(past); !strings.Contains(got, "(expired)") {
		t.Errorf("past => %q, want (expired) marker", got)
	}
}

// --- store helpers ----------------------------------------------------------

func TestTemporaryPasswordSlotHelpers(t *testing.T) {
	s := &Store{Passwords: []PasswordSlot{
		{Name: "admin", Scope: ScopeAdmin},
		{Name: "perm", Scope: ScopeRead},
		{Name: "temp1", Scope: ScopeRead, ExpiresAt: "2099-01-01T00:00:00Z"},
		{Name: "temp2", Scope: ScopeWrite, ExpiresAt: "2000-01-01T00:00:00Z"},
	}}
	temps := s.TemporaryPasswordSlots()
	if len(temps) != 2 || temps[0].Name != "temp1" || temps[1].Name != "temp2" {
		t.Fatalf("TemporaryPasswordSlots = %+v", temps)
	}
	removed := s.RemoveTemporaryPasswordSlots()
	if strings.Join(removed, ",") != "temp1,temp2" {
		t.Errorf("removed = %v, want temp1,temp2", removed)
	}
	if len(s.Passwords) != 2 || s.Passwords[0].Name != "admin" || s.Passwords[1].Name != "perm" {
		t.Errorf("survivors wrong: %+v", s.Passwords)
	}
	// Idempotent: a store with no temporary slots removes nothing.
	if got := s.RemoveTemporaryPasswordSlots(); len(got) != 0 {
		t.Errorf("second removal = %v, want none", got)
	}
}

// --- CLI: mint a temporary password + enforcement ---------------------------

// tempReaderVault inits a vault, seeds a secret, and mints a temporary read
// password "agent" (password "temp-pass") with the given ttl. Returns home.
func tempReaderVault(t *testing.T, ttl string) string {
	t.Helper()
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "sk-ci\n", "add", "--from-stdin", "ci:token"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	mustAddPassword(t, "test-pass", "temp-pass", "agent", "--scope=read", "--namespaces=*", "--ttl="+ttl)
	return home
}

func TestTempPasswordMigrateAddAndExpiry(t *testing.T) {
	home := tempReaderVault(t, "1h")

	// The slot carries an expiry roughly ttl in the future.
	s, _ := LoadStore(home)
	slot, _ := s.FindPasswordSlot("agent")
	if slot == nil || slot.ExpiresAt == "" {
		t.Fatalf("agent slot missing expiry: %+v", slot)
	}
	exp, err := time.Parse(time.RFC3339, slot.ExpiresAt)
	if err != nil || time.Until(exp) < 50*time.Minute || time.Until(exp) > 70*time.Minute {
		t.Fatalf("expiry %q not ~1h out (%v)", slot.ExpiresAt, err)
	}

	// It authenticates before expiry.
	setPass(t, "temp-pass")
	if code, out, e := runVault(t, "", "get", "--reveal", "ci:token"); code != 0 || strings.TrimSpace(out) != "sk-ci" {
		t.Fatalf("temp get before expiry: code=%d out=%q err=%s", code, out, e)
	}

	// Force it past its expiry: authentication is refused with a clear message.
	forceSlotExpiry(t, home, "agent", time.Now().Add(-time.Minute))
	setPass(t, "temp-pass")
	if code, _, e := runVault(t, "", "get", "--reveal", "ci:token"); code == 0 {
		t.Fatal("expired temp password should be refused")
	} else if !strings.Contains(e, "expired") {
		t.Errorf("expected 'expired' error, got %q", e)
	}

	// The admin is unaffected.
	setPass(t, "test-pass")
	if code, out, _ := runVault(t, "", "get", "--reveal", "ci:token"); code != 0 || strings.TrimSpace(out) != "sk-ci" {
		t.Errorf("admin get after temp expiry: code=%d out=%q", code, out)
	}
}

// forceSlotExpiry rewrites a slot's ExpiresAt in place (a stand-in for the
// clock advancing past the expiry, without re-sealing the private key).
func forceSlotExpiry(t *testing.T, home, name string, when time.Time) {
	t.Helper()
	if err := mutateStore(home, func(st *Store) error {
		sl, _ := st.FindPasswordSlot(name)
		sl.ExpiresAt = when.UTC().Format(time.RFC3339)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestTempPasswordSecondAddAndNaiveExtend covers the addSlotToEnvelope path
// (adding a temp password to an already-envelope vault) and the privAAD
// binding: naively editing ExpiresAt to the future does NOT extend access.
func TestTempPasswordSecondAddAndNaiveExtend(t *testing.T) {
	home := tempReaderVault(t, "1h") // migrates + "agent"

	// Second temp password → the addSlotToEnvelope path.
	mustAddPassword(t, "test-pass", "temp2-pass", "agent2", "--scope=read", "--namespaces=*", "--ttl=2h")
	setPass(t, "temp2-pass")
	if code, out, e := runVault(t, "", "get", "--reveal", "ci:token"); code != 0 || strings.TrimSpace(out) != "sk-ci" {
		t.Fatalf("agent2 get: code=%d out=%q err=%s", code, out, e)
	}

	// Naive extend: push ExpiresAt far into the future WITHOUT re-sealing the
	// private key. slotExpired passes, but the EncPrivKey AAD (bound to the
	// original expiry) no longer verifies, so authentication is refused —
	// tampering the plaintext date grants nothing.
	forceSlotExpiry(t, home, "agent2", time.Now().Add(1000*time.Hour))
	setPass(t, "temp2-pass")
	if code, _, _ := runVault(t, "", "get", "--reveal", "ci:token"); code == 0 {
		t.Error("naively extending ExpiresAt must not re-grant access (privAAD binding)")
	}
}

func TestTempPasswordAddValidation(t *testing.T) {
	testHome(t)
	mustInit(t)
	setPass(t, "test-pass")

	// Negative TTL is rejected.
	if code, _, e := runVault(t, "", "password", "add", "--ttl=-1h", "x"); code == 0 || !strings.Contains(e, "--ttl must be positive") {
		t.Errorf("negative ttl: code=%d err=%q", code, e)
	}
	// A temporary admin password is refused (footgun).
	if code, _, e := runVault(t, "", "password", "add", "--scope=admin", "--ttl=1h", "x"); code == 0 || !strings.Contains(e, "cannot be admin-scoped") {
		t.Errorf("temp admin: code=%d err=%q", code, e)
	}
}

func TestTempPasswordGenerate(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "sk-ci\n", "add", "--from-stdin", "ci:token"); code != 0 {
		t.Fatalf("seed: %s", e)
	}
	setPass(t, "test-pass")
	code, out, e := runVault(t, "", "password", "add", "--scope=read", "--namespaces=*", "--ttl=30m", "--generate", "agent")
	if code != 0 {
		t.Fatalf("generate add: %s", e)
	}
	if !strings.Contains(out, "generated password") || !strings.Contains(out, "expires ") {
		t.Fatalf("generate output missing password/expiry block:\n%s", out)
	}
	// Extract the printed password (the single indented token line).
	var gen string
	for _, ln := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(ln); s != "" && !strings.Contains(s, " ") && s != ")" && len(s) >= 20 {
			gen = s
		}
	}
	if gen == "" {
		t.Fatalf("could not find generated password in:\n%s", out)
	}
	// The generated password authenticates.
	setPass(t, gen)
	if code, out2, _ := runVault(t, "", "get", "--reveal", "ci:token"); code != 0 || strings.TrimSpace(out2) != "sk-ci" {
		t.Errorf("generated password get: code=%d out=%q", code, out2)
	}
}

func TestPasswordListShowsExpiry(t *testing.T) {
	home := tempReaderVault(t, "1h")
	setPass(t, "test-pass")
	code, out, e := runVault(t, "", "password", "list")
	if code != 0 {
		t.Fatalf("list: %s", e)
	}
	if !strings.Contains(out, "EXPIRES") || !strings.Contains(out, "agent") {
		t.Errorf("list missing EXPIRES/agent:\n%s", out)
	}
	// admin (permanent) shows "-"; agent shows a timestamp.
	if !strings.Contains(out, " - ") && !strings.Contains(out, "-\n") {
		t.Errorf("permanent slot should show '-':\n%s", out)
	}
	// Once expired, the marker appears.
	forceSlotExpiry(t, home, "agent", time.Now().Add(-time.Minute))
	setPass(t, "test-pass")
	if _, out, _ := runVault(t, "", "password", "list"); !strings.Contains(out, "(expired)") {
		t.Errorf("expired slot should be marked:\n%s", out)
	}
}

// --- CLI: revoke temporary passwords ----------------------------------------

func TestPasswordRmTempSuccess(t *testing.T) {
	home := tempReaderVault(t, "1h")
	// Add a permanent reader too, to prove only temporaries are pulled.
	mustAddPassword(t, "test-pass", "perm-pass", "keeper", "--scope=read", "--namespaces=*")

	setPass(t, "test-pass")
	code, out, e := runVault(t, "", "password", "rm", "--temp", "--yes")
	if code != 0 {
		t.Fatalf("rm --temp: %s", e)
	}
	if !strings.Contains(out, "revoked 1 temporary password") || !strings.Contains(out, "agent") {
		t.Errorf("rm --temp output: %q", out)
	}
	s, _ := LoadStore(home)
	if sl, _ := s.FindPasswordSlot("agent"); sl != nil {
		t.Error("temporary slot survived rm --temp")
	}
	if sl, _ := s.FindPasswordSlot("keeper"); sl == nil {
		t.Error("permanent slot must survive rm --temp")
	}
	if sl, _ := s.FindPasswordSlot("admin"); sl == nil {
		t.Error("admin must survive rm --temp")
	}
}

func TestPasswordRmTempEdgeCases(t *testing.T) {
	// Not initialized → requireStore error.
	testHome(t)
	setPass(t, "test-pass")
	if code, _, _ := runVault(t, "", "password", "rm", "--temp"); code == 0 {
		t.Error("rm --temp before init should fail")
	}

	// Legacy vault (no slots).
	testHome(t)
	mustInit(t)
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "", "password", "rm", "--temp"); code == 0 || !strings.Contains(e, "no password slots") {
		t.Errorf("rm --temp legacy: code=%d err=%q", code, e)
	}

	// Naming a slot together with --temp is rejected.
	home := tempReaderVault(t, "1h")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "", "password", "rm", "--temp", "agent"); code == 0 || !strings.Contains(e, "do not also name one") {
		t.Errorf("rm --temp <name>: code=%d err=%q", code, e)
	}

	// No temporary passwords present (permanent slots only): a clean no-op.
	mustAddPassword(t, "test-pass", "perm-pass", "keeper", "--scope=read", "--namespaces=*")
	setPass(t, "test-pass")
	// First revoke the one temp so only permanents remain.
	if code, _, e := runVault(t, "", "password", "rm", "--temp", "--yes"); code != 0 {
		t.Fatalf("setup revoke: %s", e)
	}
	if code, out, _ := runVault(t, "", "password", "rm", "--temp"); code != 0 || !strings.Contains(out, "no temporary passwords") {
		t.Errorf("rm --temp none: code=%d out=%q", code, out)
	}

	// Missing --yes on a batch with temporaries: refused.
	mustAddPassword(t, "test-pass", "temp-pass", "agent", "--scope=read", "--namespaces=*", "--ttl=1h")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "", "password", "rm", "--temp"); code == 0 || !strings.Contains(e, "confirmation") {
		t.Errorf("rm --temp without --yes: code=%d err=%q", code, e)
	}
	// Wrong passphrase: authentication fails.
	setPass(t, "definitely-wrong")
	if code, _, _ := runVault(t, "", "password", "rm", "--temp", "--yes"); code == 0 {
		t.Error("rm --temp with wrong passphrase should fail")
	}
	// A non-admin (the temp reader itself) may not revoke.
	setPass(t, "temp-pass")
	if code, _, e := runVault(t, "", "password", "rm", "--temp", "--yes"); code == 0 || !strings.Contains(e, "only an admin") {
		t.Errorf("rm --temp as non-admin: code=%d err=%q", code, e)
	}
	_ = home
}

// TestPasswordAddGenerateRandFailure covers generatePassword surfacing a
// randomness failure (and runPasswordAdd handling it). Set up an envelope
// vault first so authentication uses no randomness, then fail the RNG so the
// only affected step is the password generation.
func TestPasswordAddGenerateRandFailure(t *testing.T) {
	testHome(t)
	mustInit(t)
	mustAddPassword(t, "test-pass", "seed-pass", "seed", "--scope=read", "--namespaces=*")
	setPass(t, "test-pass")
	p7swapRandRead(t, p7failRead)
	if code, _, _ := runVault(t, "", "password", "add", "--generate", "x"); code == 0 {
		t.Error("generate add with a rand failure should fail")
	}
	// The predicate helper directly, too.
	if _, err := generatePassword(); err == nil {
		t.Error("generatePassword should surface a rand failure")
	}
}

// TestPasswordRmTempMutateError drives the store-mutate guard in
// runPasswordRmTemp via the w8mutateStore seam.
func TestPasswordRmTempMutateError(t *testing.T) {
	tempReaderVault(t, "1h")
	old := w8mutateStore
	t.Cleanup(func() { w8mutateStore = old })
	w8mutateStore = func(string, func(*Store) error) error { return errW8Boom }
	setPass(t, "test-pass")
	if code, _, _ := runVault(t, "", "password", "rm", "--temp", "--yes"); code == 0 {
		t.Error("rm --temp should surface a store-mutate error")
	}
}

// TestExpiredSlotDoesNotShadowReusedPassphrase covers the openSession fix: an
// expired temporary slot must not lock out a permanent slot that happens to
// share the same passphrase (password reuse is not forbidden).
func TestExpiredSlotDoesNotShadowReusedPassphrase(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "sk\n", "add", "--from-stdin", "ci:token"); code != 0 {
		t.Fatalf("seed: %s", e)
	}
	// A temporary slot and a permanent slot, both with passphrase "shared".
	mustAddPassword(t, "test-pass", "shared", "agent", "--scope=read", "--namespaces=*", "--ttl=1h")
	mustAddPassword(t, "test-pass", "shared", "keeper", "--scope=read", "--namespaces=*")

	// Expire the temporary one; the shared passphrase must still open the
	// permanent slot rather than reporting "expired".
	forceSlotExpiry(t, home, "agent", time.Now().Add(-time.Minute))
	setPass(t, "shared")
	if code, out, e := runVault(t, "", "get", "--reveal", "ci:token"); code != 0 || strings.TrimSpace(out) != "sk" {
		t.Fatalf("reused passphrase should open the permanent slot: code=%d out=%q err=%s", code, out, e)
	}

	// And if ONLY the expired temporary slot matched, it is still reported
	// expired (remove the permanent twin, keep the expired temp).
	if err := mutateStore(home, func(st *Store) error { st.RemovePasswordSlot("keeper"); return nil }); err != nil {
		t.Fatal(err)
	}
	setPass(t, "shared")
	if code, _, e := runVault(t, "", "get", "--reveal", "ci:token"); code == 0 || !strings.Contains(e, "expired") {
		t.Errorf("only-expired match should report expired: code=%d err=%q", code, e)
	}
}

// TestPasswordRmTempRejectsRekey covers the --rekey + --temp guard.
func TestPasswordRmTempRejectsRekey(t *testing.T) {
	tempReaderVault(t, "1h")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "", "password", "rm", "--temp", "--rekey", "--yes"); code == 0 || !strings.Contains(e, "not supported with --temp") {
		t.Errorf("rm --temp --rekey: code=%d err=%q", code, e)
	}
}

// --- TUI --------------------------------------------------------------------

func TestTUITempPasswordRevoke(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	mustAddPassword(t, "test-pass", "temp-pass", "agent", "--scope=read", "--namespaces=*", "--ttl=1h")

	ctl := newTUIController(home)
	ctl.setActiveVault(vaultFolder(home), "")
	if err := ctl.authenticate("test-pass"); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	// Controller: count and the passwords table's EXPIRES column.
	if n := ctl.temporaryPasswordCount(); n != 1 {
		t.Fatalf("temporaryPasswordCount = %d, want 1", n)
	}
	m := newRootModel(ctl)
	m.Init()
	_, rows := m.passwordsTable()
	sawExpiry := false
	for _, r := range rows {
		if r[0] == "agent" && strings.HasPrefix(r[4], "2") { // EXPIRES column is a timestamp
			sawExpiry = true
		}
	}
	if !sawExpiry {
		t.Errorf("passwords table missing agent's expiry: %+v", rows)
	}

	// The `T` action pushes the revoke-all form when temporaries exist.
	pw := m.buildPasswords().(*tableScreen)
	pw.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if _, cmd := pw.Update(keyMsg("T")); pushedTitle(cmd) != "password rm --temp" {
		t.Errorf("T pushed %q, want password rm --temp", pushedTitle(cmd))
	}

	// Cancel path: completing without choosing the affirmative pops and does
	// not revoke.
	fs := m.buildPasswordRevokeTempForm().(*formScreen)
	if fs.cliCommand() != "boru vault password rm --temp" {
		t.Errorf("revoke-temp preview: %q", fs.cliCommand())
	}
	if msgs := completeForm(t, fs); !msgOfType(msgs, popMsg{}) {
		t.Errorf("cancel should pop, got %v", msgs)
	}
	if n := ctl.temporaryPasswordCount(); n != 1 {
		t.Errorf("cancel must not revoke; count = %d", n)
	}

	// Confirm path: 'y' drives the affirmative → the controller revokes.
	fs2 := m.buildPasswordRevokeTempForm()
	pumpScreen(t, fs2, tea.WindowSizeMsg{Width: 120, Height: 30}, keyMsg("y"))
	if n := ctl.temporaryPasswordCount(); n != 0 {
		t.Errorf("after confirm, count = %d, want 0", n)
	}

	// With no temporaries left, `T` is a status no-op, not a push.
	pw2 := m.buildPasswords().(*tableScreen)
	pw2.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if _, cmd := pw2.Update(keyMsg("T")); !msgOfType(collectMsgs(cmd), setStatusMsg{}) {
		t.Error("T with no temporaries should set a status")
	}
}

// TestTUITempControllerErrorPaths covers the controller's error handling:
// revoking on a slot-less vault fails, and counting on an uninitialized
// location returns zero rather than panicking.
func TestTUITempControllerErrorPaths(t *testing.T) {
	ctl, home := newTestController(t) // legacy vault, no password slots
	if err := ctl.passwordRevokeTemp(); err == nil {
		t.Error("passwordRevokeTemp on a slot-less vault should error")
	}
	ctl.setActiveVault(filepath.Join(home, "empty"), "")
	if n := ctl.temporaryPasswordCount(); n != 0 {
		t.Errorf("temporaryPasswordCount on an empty vault = %d, want 0", n)
	}
}

// TestTUIPasswordAddTTL covers the TUI creating a temporary password (the
// --ttl plumbing in passwordAdd + passwordAddCommandPreview).
func TestTUIPasswordAddTTL(t *testing.T) {
	ctl, home := newTestController(t)
	if err := ctl.passwordAdd("agent", ScopeRead, "*", "temp-pass", "1h"); err != nil {
		t.Fatalf("passwordAdd temp: %v", err)
	}
	s, _ := LoadStore(home)
	if slot, _ := s.FindPasswordSlot("agent"); slot == nil || slot.ExpiresAt == "" {
		t.Errorf("TUI-created temporary password missing expiry")
	}
	if p := passwordAddCommandPreview("agent", "read", "*", "1h"); !strings.Contains(p, "--ttl=1h") {
		t.Errorf("preview missing --ttl: %q", p)
	}
}

// tempBrokerVault builds an envelope vault whose ACTIVE passphrase is a
// temporary read password — for driving the proxy/mcp "don't cache a
// temporary session" guards.
func tempBrokerVault(t *testing.T) string {
	t.Helper()
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "sk\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("seed: %s", e)
	}
	mustAddPassword(t, "test-pass", "temp-pass", "agent", "--scope=read", "--namespaces=*", "--ttl=1h")
	setPass(t, "temp-pass") // the broker authenticates as the temporary agent
	return home
}

// TestMCPRefusesToCacheTemporaryPassword drives runMcp under a temporary
// password: it must warn and not cache the session.
func TestMCPRefusesToCacheTemporaryPassword(t *testing.T) {
	tempBrokerVault(t)
	var out, errb bytes.Buffer
	Run([]string{"mcp"}, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), &out, &errb)
	if !strings.Contains(errb.String(), "TEMPORARY password") {
		t.Errorf("missing mcp temporary-password warning: %q", errb.String())
	}
}
