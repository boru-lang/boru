package vault

// tui_controller_more_test.go — the controller operations not covered by
// tui_controller_test.go: index maintenance, text panels, expiry/rename,
// password slots, restore, and the error translations.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdErr(t *testing.T) {
	if got := cmdErr("error: boom\nsecond line", ""); got.Error() != "boom" {
		t.Errorf("stderr with prefix + extra lines: %q", got)
	}
	if got := cmdErr("", "from stdout"); got.Error() != "from stdout" {
		t.Errorf("stdout fallback: %q", got)
	}
	if got := cmdErr("", ""); got.Error() != "command failed" {
		t.Errorf("empty fallback: %q", got)
	}
}

func TestControllerSetDefaultAndPrune(t *testing.T) {
	ctl, home := newTestController(t)
	if err := ctl.setDefault(); err != nil {
		t.Fatalf("setDefault: %v", err)
	}
	idx, _ := loadVaultIndex(home)
	if ref, _ := idx.find(ctl.folder, ""); ref == nil || !ref.Default {
		t.Fatalf("default not recorded: %+v", idx.Vaults)
	}
	if err := ctl.pruneVault(ctl.folder, ""); err != nil {
		t.Fatalf("pruneVault: %v", err)
	}
	idx, _ = loadVaultIndex(home)
	if ref, _ := idx.find(ctl.folder, ""); ref != nil {
		t.Errorf("entry survived prune: %+v", idx.Vaults)
	}
}

func TestControllerNeedsPassphraseAndAlias(t *testing.T) {
	ctl, home := newTestController(t)
	// A file-backend vault needs a passphrase.
	need, err := ctl.needsPassphrase()
	if err != nil || !need {
		t.Errorf("needsPassphrase = %v,%v; want true", need, err)
	}
	// Unknown alias reports (zero, false).
	if _, ok := ctl.alias("missing"); ok {
		t.Error("alias(missing) should be false")
	}
	if err := ctl.addSecret("k", "v", "openai", "", ""); err != nil {
		t.Fatal(err)
	}
	if a, ok := ctl.alias("k"); !ok || a.Provider != "openai" {
		t.Errorf("alias(k) = %+v,%v", a, ok)
	}
	// Pointing at a folder with no vault: needsPassphrase is false, not an error.
	ctl.setActiveVault(filepath.Join(home, "empty"), "")
	if need, err := ctl.needsPassphrase(); err != nil || need {
		t.Errorf("missing vault needsPassphrase = %v,%v; want false,nil", need, err)
	}
	// status reports ok=false for the missing vault.
	if _, ok, err := ctl.status(); ok || err != nil {
		t.Errorf("status of missing vault = ok:%v err:%v", ok, err)
	}
}

func TestControllerTextPanels(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if out := ctl.statusText(); !strings.Contains(out, "backend:") {
		t.Errorf("statusText: %q", out)
	}
	if out := ctl.providersText(); !strings.Contains(out, "openai") {
		t.Errorf("providersText: %q", out)
	}
	if out := ctl.configText(); !strings.Contains(out, "(no config)") {
		t.Errorf("configText: %q", out)
	}
	// A legacy vault has no journal: history reports that cleanly.
	if out := ctl.historyText(); !strings.Contains(out, "no history") {
		t.Errorf("historyText: %q", out)
	}
	if out := ctl.auditText(); !strings.Contains(out, "vault.add") {
		t.Errorf("auditText should include the add event: %q", out)
	}
	out, ok := ctl.verifyText(false)
	if !ok || !strings.Contains(out, "OK") {
		t.Errorf("verifyText = %q, ok=%v", out, ok)
	}
	// scanText over an empty directory finds nothing alarming.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clean.txt"), []byte("nothing here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := ctl.scanText(dir); strings.Contains(strings.ToLower(out), "panic") {
		t.Errorf("scanText: %q", out)
	}
}

func TestControllerExpiryAndRename(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := ctl.setExpiry("k", "90d"); err != nil {
		t.Fatalf("setExpiry: %v", err)
	}
	if a, _ := ctl.alias("k"); a.ExpiresAt == "" {
		t.Error("expiry not set")
	}
	if err := ctl.clearExpiry("k"); err != nil {
		t.Fatalf("clearExpiry: %v", err)
	}
	if a, _ := ctl.alias("k"); a.ExpiresAt != "" {
		t.Error("expiry not cleared")
	}
	// Negatives: what must be rejected.
	if err := ctl.setExpiry("k", "not-a-date"); err == nil {
		t.Error("bad expiry should error")
	}
	if err := ctl.setExpiry("missing", "90d"); err == nil {
		t.Error("expiry on a missing alias should error")
	}
	if err := ctl.clearExpiry("missing"); err == nil {
		t.Error("clearExpiry on a missing alias should error")
	}

	if err := ctl.renameSecret("k", "k2", false); err != nil {
		t.Fatalf("renameSecret: %v", err)
	}
	if _, ok := ctl.alias("k2"); !ok {
		t.Error("rename target missing")
	}
	if err := ctl.renameSecret("k", "k3", false); err == nil {
		t.Error("renaming a missing alias should error")
	}
	// The revoke-caps variant also drives cleanly.
	if _, err := ctl.grant("k2", "a", "", "", "1h", 0, 0, false); err != nil {
		t.Fatal(err)
	}
	if err := ctl.renameSecret("k2", "k4", true); err != nil {
		t.Fatalf("renameSecret --revoke-caps: %v", err)
	}
	_, active, _ := ctl.listCapabilities()
	for id, on := range active {
		if on {
			t.Errorf("capability %s should be revoked after rename --revoke-caps", id)
		}
	}
}

func TestControllerPasswordSlotLifecycle(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("proj:k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	// No slots yet.
	slots, err := ctl.listPasswordSlots()
	if err != nil || len(slots) != 0 {
		t.Fatalf("fresh vault slots = %v,%v", slots, err)
	}
	// Adding the first password migrates the vault to envelope format.
	if err := ctl.passwordAdd("reader", ScopeRead, "proj", "reader-pass", ""); err != nil {
		t.Fatalf("passwordAdd: %v", err)
	}
	slots, _ = ctl.listPasswordSlots()
	names := map[string]string{}
	for _, p := range slots {
		names[p.Name] = p.Scope
	}
	if names["admin"] != ScopeAdmin || names["reader"] != ScopeRead {
		t.Fatalf("slots after migrate = %+v", slots)
	}
	// The migrated vault still reveals through the controller.
	if got, err := ctl.revealSecret("proj:k"); err != nil || got != "v" {
		t.Fatalf("reveal after migrate = %q, %v", got, err)
	}
	// Duplicate add is rejected.
	if err := ctl.passwordAdd("reader", ScopeRead, "proj", "x", ""); err == nil {
		t.Error("duplicate password add should error")
	}
	// passwordRemove passes --yes (the TUI form runs its own typed
	// confirmation first, like the rm/rotate/restore mutations), so the
	// removal goes through.
	if err := ctl.passwordRemove("reader"); err != nil {
		t.Errorf("passwordRemove: %v", err)
	}
	slots, _ = ctl.listPasswordSlots()
	if len(slots) != 1 {
		t.Errorf("slot count after rm = %d, want 1 (admin)", len(slots))
	}
	// Removing a nonexistent slot is refused.
	if err := ctl.passwordRemove("reader"); err == nil {
		t.Error("passwordRemove of a missing slot should error")
	}
}

func TestControllerRestore(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	// Envelope mode activates journaling; a later generation is restorable.
	if err := ctl.passwordAdd("reader", ScopeRead, "*", "reader-pass", ""); err != nil {
		t.Fatal(err)
	}
	if err := ctl.addSecret("k2", "v2", "", "", ""); err != nil {
		t.Fatal(err)
	}
	st, ok, _ := ctl.status()
	if !ok || st.Generation < 1 {
		t.Fatalf("status = %+v ok=%v", st, ok)
	}
	if !st.HasSlots || st.Slots != 2 || st.AdminSlots != 1 {
		t.Errorf("slot summary wrong: %+v", st)
	}
	if err := ctl.restore(st.Generation); err != nil {
		t.Fatalf("restore: %v", err)
	}
	// Restoring a bogus generation errors.
	if err := ctl.restore(9999); err == nil {
		t.Error("restore(9999) should error")
	}
}

func TestControllerCopyToClipboardWithoutTool(t *testing.T) {
	for _, tool := range []string{"wl-copy", "xclip", "xsel", "pbcopy", "pwsh", "powershell"} {
		if _, err := exec.LookPath(tool); err == nil {
			t.Skipf("clipboard tool %s present; not exercising the error path", tool)
		}
	}
	ctl, _ := newTestController(t)
	if _, err := ctl.copyToClipboard("text"); err == nil {
		t.Error("copyToClipboard without any tool should error")
	}
}

func TestControllerRunPropagatesPassphrase(t *testing.T) {
	ctl, _ := newTestController(t)
	// Wipe the ambient env passphrase: the controller must supply its own.
	t.Setenv(EnvPassphrase, "")
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatalf("addSecret with controller-held passphrase: %v", err)
	}
	if got, err := ctl.revealSecret("k"); err != nil || got != "v" {
		t.Fatalf("reveal = %q, %v", got, err)
	}
	// After the run, the passphrase is scrubbed from the environment.
	if os.Getenv(EnvPassphrase) != "" {
		t.Error("controller leaked the passphrase into the environment")
	}
}
