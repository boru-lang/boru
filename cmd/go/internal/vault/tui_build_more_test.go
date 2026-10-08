package vault

// tui_build_more_test.go — drives the concrete screen constructors in
// tui_build.go: menu actions, form submit closures, the palette, and the
// passphrase gate. Screens are exercised synchronously with collectMsgs /
// pumpScreen; no real terminal is involved.

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// completeForm marks a form screen submitted with its current values and
// returns the messages produced by onSubmit.
func completeForm(t *testing.T, s screen) []tea.Msg {
	t.Helper()
	fs, ok := s.(*formScreen)
	if !ok {
		t.Fatalf("screen is %T, want *formScreen", s)
	}
	fs.form.State = huh.StateCompleted
	_, cmd := fs.Update(keyMsg("x"))
	return collectMsgs(cmd)
}

// listActs returns the act closures of a listScreen's items keyed by name.
func listActs(t *testing.T, s screen) map[string]func() tea.Cmd {
	t.Helper()
	ls, ok := s.(*listScreen)
	if !ok {
		t.Fatalf("screen is %T, want *listScreen", s)
	}
	out := map[string]func() tea.Cmd{}
	for _, it := range ls.list.Items() {
		li := it.(listItem)
		out[li.name] = li.act
	}
	return out
}

func TestVaultKeyRoundTrip(t *testing.T) {
	f, s := splitVaultKey(vaultKey("/home/u/.boru", "work"))
	if f != "/home/u/.boru" || s != "work" {
		t.Errorf("splitVaultKey = %q,%q", f, s)
	}
	if f, s := splitVaultKey("plain"); f != "plain" || s != "" {
		t.Errorf("separator-less key = %q,%q", f, s)
	}
	if vaultLabel(listItem{name: "● work"}) != "work" || vaultLabel(listItem{name: "★ prod"}) != "prod" {
		t.Error("vaultLabel should strip status markers")
	}
}

func TestHomeMenuOpensEveryCategory(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.Init()
	acts := listActs(t, m.buildHome())
	for name, wantTitle := range map[string]string{
		"Secrets": "secrets", "Access": "access", "Passwords": "passwords",
		"Maintenance": "maintenance", "Settings": "settings",
	} {
		act := acts[name]
		if act == nil {
			t.Fatalf("home has no %q item", name)
		}
		if got := pushedTitle(act()); got != wantTitle {
			t.Errorf("%s opened %q, want %q", name, got, wantTitle)
		}
	}
}

func TestVaultPickerActions(t *testing.T) {
	ctl, home := newTestController(t)
	m := newRootModel(ctl)
	m.Init()
	picker := m.buildVaultPicker().(*listScreen)
	picker.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// n opens the create-vault form.
	_, cmd := picker.Update(keyMsg("n"))
	if got := pushedTitle(cmd); got != "new vault" {
		t.Errorf("n pushed %q, want new vault", got)
	}
	// d marks the selected vault as default and records it in the index.
	_, cmd = picker.Update(keyMsg("d"))
	if op, ok := firstOpResult(collectMsgs(cmd)); !ok || op.err != nil {
		t.Fatalf("set-default failed: %+v", op)
	}
	idx, _ := loadVaultIndex(home)
	if ref, _ := idx.find(vaultFolder(home), ""); ref == nil || !ref.Default {
		t.Errorf("default not recorded in the index: %+v", idx.Vaults)
	}
	// D prunes the selected entry from the index (files untouched).
	_, cmd = picker.Update(keyMsg("D"))
	if op, ok := firstOpResult(collectMsgs(cmd)); !ok || op.err != nil {
		t.Fatalf("prune failed: %+v", op)
	}
	idx, _ = loadVaultIndex(home)
	if ref, _ := idx.find(vaultFolder(home), ""); ref != nil {
		t.Errorf("entry not pruned: %+v", idx.Vaults)
	}
	if _, err := os.Stat(StorePath(home)); err != nil {
		t.Errorf("prune must not touch the vault files: %v", err)
	}
}

func TestCreateVaultFormDrivenCreatesSuffixedVault(t *testing.T) {
	ctl, home := newTestController(t)
	m := newRootModel(ctl)
	fs := m.buildCreateVaultForm().(*formScreen)
	if !strings.Contains(fs.cliCommand(), "init --backend=auto") {
		t.Errorf("create-vault preview: %q", fs.cliCommand())
	}
	// Drive the real form: suffix "alt2", folder blank, backend select
	// default (auto), passphrase "newpass".
	msgs := []tea.Msg{tea.Msg(tea.WindowSizeMsg{Width: 80, Height: 24})}
	for _, r := range "alt2" {
		msgs = append(msgs, keyMsg(string(r)))
	}
	msgs = append(msgs, tea.KeyMsg{Type: tea.KeyEnter}) // suffix -> folder
	msgs = append(msgs, tea.KeyMsg{Type: tea.KeyEnter}) // folder -> backend
	msgs = append(msgs, tea.KeyMsg{Type: tea.KeyEnter}) // backend -> passphrase
	for _, r := range "newpass" {
		msgs = append(msgs, keyMsg(string(r)))
	}
	msgs = append(msgs, tea.KeyMsg{Type: tea.KeyEnter}) // submit
	_, seen := pumpScreen(t, fs, msgs...)
	found := false
	for _, msg := range seen {
		switch sw := msg.(type) {
		case switchedToVaultMsg:
			found = true
			if sw.name != "alt2" {
				t.Errorf("switched to %q, want alt2", sw.name)
			}
		case opResultMsg:
			if sw.err != nil {
				t.Fatalf("create vault failed: %v", sw.err)
			}
		}
	}
	if !found {
		t.Fatalf("no switchedToVaultMsg seen: %+v", seen)
	}
	if ctl.suffix != "alt2" {
		t.Errorf("active suffix = %q, want alt2", ctl.suffix)
	}
	if _, err := os.Stat(vaultIndexPath(home)); err != nil {
		t.Errorf("index not written: %v", err)
	}
}

func TestCreateVaultFormSubmitExistingVaultErrors(t *testing.T) {
	// Submitting with all defaults re-inits the existing default vault,
	// which must surface as an error result, not silently succeed.
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	msgs := completeForm(t, m.buildCreateVaultForm())
	if op, ok := firstOpResult(msgs); !ok || op.err == nil {
		t.Errorf("re-init of an existing vault should error: %+v", msgs)
	}
}

func TestSecretDetailScreenActions(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("tok", "sekrit", "github", "", ""); err != nil {
		t.Fatal(err)
	}
	m := newRootModel(ctl)
	m.Init()
	s := m.buildSecretDetail("tok").(*secretDetailScreen)

	// Metadata glue.
	if s.capturesInput() || s.reload() != nil || s.Init() != nil {
		t.Error("detail metadata wrong")
	}
	if s.cliCommand() != "boru vault get tok" || s.Title() != "tok" {
		t.Errorf("detail cmd/title: %q %q", s.cliCommand(), s.Title())
	}
	if !strings.Contains(s.helpInfo(), "Rotate") {
		t.Error("detail helpInfo should explain the actions")
	}
	if len(s.shortHelp()) == 0 || len(s.fullHelp()) != 1 {
		t.Error("detail help bindings wrong")
	}

	// Cursor moves with j/k and arrows, clamped at the ends.
	s.Update(keyMsg("k")) // already at 0
	if s.cursor != 0 {
		t.Error("cursor should clamp at 0")
	}
	s.Update(keyMsg("j"))
	if s.cursor != 1 {
		t.Errorf("j should move down, cursor=%d", s.cursor)
	}
	s.Update(tea.KeyMsg{Type: tea.KeyUp})
	if s.cursor != 0 {
		t.Errorf("up should move back, cursor=%d", s.cursor)
	}

	// r reveals through the (authed) controller.
	_, cmd := s.Update(keyMsg("r"))
	msgs := collectMsgs(cmd)
	var rev secretRevealedMsg
	for _, msg := range msgs {
		if rm, ok := msg.(secretRevealedMsg); ok {
			rev = rm
		}
	}
	if rev.value != "sekrit" {
		t.Fatalf("reveal produced %+v", msgs)
	}
	s.Update(rev)
	if !s.shown || s.value != "sekrit" {
		t.Error("secretRevealedMsg should set the value")
	}
	if v := s.View(80, 24); !strings.Contains(v, "sekrit") {
		t.Error("revealed view should show the value")
	}
	// r again hides.
	s.Update(keyMsg("r"))
	if s.shown || s.value != "" {
		t.Error("second r should hide and clear the value")
	}

	// The per-key actions push their forms (controller is authenticated).
	for _, tc := range []struct{ key, want string }{
		{"R", "rotate"}, {"m", "rename"}, {"e", "expiry"}, {"g", "grant"}, {"D", "delete"},
	} {
		_, cmd := s.Update(keyMsg(tc.key))
		if got := pushedTitle(cmd); got != tc.want {
			t.Errorf("%q pushed %q, want %q", tc.key, got, tc.want)
		}
	}

	// Exercise copy failure without consulting or changing the OS clipboard.
	origDetect := t7detectClipboard
	t.Cleanup(func() { t7detectClipboard = origDetect })
	t7detectClipboard = func(clipEnv) (*clipboardCmd, error) {
		return nil, os.ErrNotExist
	}

	_, cmd = s.Update(keyMsg("c"))
	msgs = collectMsgs(cmd)
	if len(msgs) != 1 {
		t.Fatalf("copy should produce one status, got %+v", msgs)
	}
	if st, ok := msgs[0].(setStatusMsg); !ok || !st.err {
		t.Errorf("copy without a clipboard should be an error status: %+v", msgs[0])
	}
}

func TestRecipeExampleCmdKnownTools(t *testing.T) {
	for name, want := range map[string]string{
		"cargo":     "cargo publish",
		"gem":       "gem push",
		"pypi":      "twine upload",
		"hex":       "mix hex.publish",
		"swift":     "swift package-registry publish",
		"cocoapods": "pod trunk push",
		"composer":  "composer install",
		"github":    "gh release",
		"gitlab":    "glab release",
		"terraform": "terraform apply",
		"npm":       "npm publish",
	} {
		if got := recipeExampleCmd(name); !strings.Contains(got, want) {
			t.Errorf("recipeExampleCmd(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAddFormSubmitStoresAndRemembersContext(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.lastAdd = addDefaults{provider: "github", namespace: "", expiry: ""}
	fs := m.buildAddForm().(*formScreen)
	if !strings.Contains(fs.cliCommand(), "--provider=github") {
		t.Errorf("add preview should carry the remembered provider: %q", fs.cliCommand())
	}
	// Submitting with an empty alias fails through the real CLI path.
	msgs := completeForm(t, fs)
	if op, ok := firstOpResult(msgs); !ok || op.err == nil {
		t.Errorf("empty-alias add should error: %+v", msgs)
	}
	// The non-secret context was remembered for the next add.
	if m.lastAdd.provider != "github" {
		t.Errorf("lastAdd not preserved: %+v", m.lastAdd)
	}
}

func TestRotateRenameExpiryFormSubmits(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	m := newRootModel(ctl)

	// Rotate with an empty new value must fail via the real CLI.
	fs := m.buildRotateForm("k").(*formScreen)
	if !strings.Contains(fs.cliCommand(), "rotate") {
		t.Errorf("rotate preview: %q", fs.cliCommand())
	}
	if op, ok := firstOpResult(completeForm(t, fs)); !ok || op.err == nil {
		t.Error("rotate with empty value should error")
	}

	// Rename to the same name errors (mv refuses).
	fs = m.buildRenameForm("k").(*formScreen)
	if !strings.Contains(fs.cliCommand(), "mv k k") {
		t.Errorf("rename preview: %q", fs.cliCommand())
	}
	if op, ok := firstOpResult(completeForm(t, fs)); !ok || op.err == nil {
		t.Error("rename to the same name should error")
	}

	// Expiry with a blank input clears the reminder (ok path).
	fs = m.buildExpiryForm("k").(*formScreen)
	if !strings.Contains(fs.cliCommand(), "expiry clear k") {
		t.Errorf("expiry preview: %q", fs.cliCommand())
	}
	if op, ok := firstOpResult(completeForm(t, fs)); !ok || op.err != nil {
		t.Errorf("expiry clear should succeed: %+v", op)
	}

	// Expiry with a duration sets it (driven through the input field).
	fs = m.buildRestoreFormExpiryHelper(t, "k")
	_, seen := pumpScreen(t, fs,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		keyMsg("9"), keyMsg("0"), keyMsg("d"), tea.KeyMsg{Type: tea.KeyEnter})
	if op, ok := firstOpResult(seen); !ok || op.err != nil {
		t.Fatalf("expiry set 90d failed: %+v", seen)
	}
	if a, ok := ctl.alias("k"); !ok || a.ExpiresAt == "" {
		t.Errorf("expiry not persisted: %+v", a)
	}
}

// buildRestoreFormExpiryHelper builds a fresh expiry form (helper keeps
// the test above readable).
func (m *rootModel) buildRestoreFormExpiryHelper(t *testing.T, alias string) *formScreen {
	t.Helper()
	return m.buildExpiryForm(alias).(*formScreen)
}

func TestRemoveFormConfirmAndCancel(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("gone", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	m := newRootModel(ctl)

	// Default (ok=false) is the cancel path: pops with a status, keeps data.
	fs := m.buildRemoveForm("gone").(*formScreen)
	if fs.cliCommand() != "boru vault rm --yes gone" {
		t.Errorf("remove preview: %q", fs.cliCommand())
	}
	msgs := completeForm(t, fs)
	if !msgOfType(msgs, popMsg{}) || !msgOfType(msgs, setStatusMsg{}) {
		t.Errorf("cancel should pop + status: %+v", msgs)
	}
	if got, err := ctl.revealSecret("gone"); err != nil || got != "v" {
		t.Fatal("cancel must not delete the secret")
	}

	// Driving "y" confirms and actually deletes.
	fs = m.buildRemoveForm("gone").(*formScreen)
	_, seen := pumpScreen(t, fs, tea.WindowSizeMsg{Width: 80, Height: 24}, keyMsg("y"))
	if op, ok := firstOpResult(seen); !ok || op.err != nil {
		t.Fatalf("confirmed delete failed: %+v", seen)
	}
	if _, err := ctl.revealSecret("gone"); err == nil {
		t.Error("secret should be gone after the confirmed delete")
	}
}

func TestAccessScreenGrantAndRevoke(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.grant("k", "agent1", "", "", "2h", 0, 0, false); err != nil {
		t.Fatal(err)
	}
	m := newRootModel(ctl)
	m.Init()

	access := m.buildAccess().(*tableScreen)
	access.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if access.count != 1 {
		t.Fatalf("access rows = %d, want 1", access.count)
	}
	// a opens the grant form (unprefilled).
	_, cmd := access.Update(keyMsg("a"))
	if got := pushedTitle(cmd); got != "grant" {
		t.Errorf("a pushed %q, want grant", got)
	}
	// D on the row opens the revoke form for that id.
	_, cmd = access.Update(keyMsg("D"))
	if got := pushedTitle(cmd); got != "revoke" {
		t.Errorf("D pushed %q, want revoke", got)
	}

	// The grant form submit path mints a capability (prefilled alias).
	fs := m.buildGrantForm("k").(*formScreen)
	if !strings.Contains(fs.cliCommand(), "--ttl=2h") || !strings.HasSuffix(fs.cliCommand(), " k") {
		t.Errorf("grant preview: %q", fs.cliCommand())
	}
	msgs := completeForm(t, fs)
	var granted bool
	for _, msg := range msgs {
		if gm, ok := msg.(grantedMsg); ok {
			granted = true
			if !strings.Contains(gm.output, "token:") {
				t.Errorf("grant output missing token: %q", gm.output)
			}
		}
	}
	if !granted {
		t.Fatalf("grant submit produced %+v", msgs)
	}

	// The revoke form: cancel keeps it active, y revokes it.
	caps, active, _ := ctl.listCapabilities()
	id := caps[0].ID
	fs = m.buildRevokeForm(id).(*formScreen)
	if !strings.Contains(fs.cliCommand(), "revoke "+id) {
		t.Errorf("revoke preview: %q", fs.cliCommand())
	}
	if msgs := completeForm(t, fs); !msgOfType(msgs, popMsg{}) {
		t.Errorf("revoke cancel should pop: %+v", msgs)
	}
	if !active[id] {
		t.Fatal("capability should still be active after cancel")
	}
	fs = m.buildRevokeForm(id).(*formScreen)
	_, seen := pumpScreen(t, fs, tea.WindowSizeMsg{Width: 80, Height: 24}, keyMsg("y"))
	if op, ok := firstOpResult(seen); !ok || op.err != nil {
		t.Fatalf("confirmed revoke failed: %+v", seen)
	}
	_, active, _ = ctl.listCapabilities()
	if active[id] {
		t.Error("capability still active after confirmed revoke")
	}
}

func TestPasswordsScreenAndForms(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.Init()

	pw := m.buildPasswords().(*tableScreen)
	pw.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if pw.count != 0 {
		t.Fatalf("fresh vault should list no password slots")
	}
	// a opens the add form even with no rows; D with no rows is a no-op.
	_, cmd := pw.Update(keyMsg("a"))
	if got := pushedTitle(cmd); got != "password add" {
		t.Errorf("a pushed %q", got)
	}
	if _, cmd = pw.Update(keyMsg("D")); cmd != nil {
		t.Error("D with no rows should do nothing")
	}

	// Add-form submit with an empty name errors through the CLI.
	fs := m.buildPasswordAddForm().(*formScreen)
	if !strings.Contains(fs.cliCommand(), "password add") {
		t.Errorf("password add preview: %q", fs.cliCommand())
	}
	if op, ok := firstOpResult(completeForm(t, fs)); !ok || op.err == nil {
		t.Error("password add with empty name should error")
	}

	// Remove-form: cancel path, then confirmed removal against a vault
	// with no slots errors cleanly.
	fs = m.buildPasswordRemoveForm("reader").(*formScreen)
	if fs.cliCommand() != "boru vault password rm reader" {
		t.Errorf("password rm preview: %q", fs.cliCommand())
	}
	if msgs := completeForm(t, fs); !msgOfType(msgs, popMsg{}) {
		t.Errorf("cancel should pop: %+v", msgs)
	}
	fs = m.buildPasswordRemoveForm("reader").(*formScreen)
	_, seen := pumpScreen(t, fs, tea.WindowSizeMsg{Width: 80, Height: 24}, keyMsg("y"))
	if op, ok := firstOpResult(seen); !ok || op.err == nil {
		t.Errorf("rm on a slot-less vault should error: %+v", seen)
	}
}

func TestMaintenanceMenuAndPagers(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.Init()

	acts := listActs(t, m.buildMaintenance())
	for name, want := range map[string]string{
		"Verify": "verify", "Scan": "scan", "History": "history", "Audit": "audit",
	} {
		act := acts[name]
		if act == nil {
			t.Fatalf("maintenance missing %q", name)
		}
		if got := pushedTitle(act()); got != want {
			t.Errorf("%s pushed %q, want %q", name, got, want)
		}
	}

	// The verify pager reports and exposes the prune action.
	vp := m.buildVerifyPager().(*pagerScreen)
	_, cmd := vp.Update(keyMsg("p"))
	if got := pushedTitle(cmd); got != "prune" {
		t.Errorf("p pushed %q, want prune", got)
	}
	// Prune form: cancel, then confirmed repair (a clean vault repairs to ok).
	fs := m.buildVerifyPruneForm().(*formScreen)
	if fs.cliCommand() != "boru vault verify --prune" {
		t.Errorf("prune preview: %q", fs.cliCommand())
	}
	if msgs := completeForm(t, fs); !msgOfType(msgs, popMsg{}) {
		t.Errorf("prune cancel should pop: %+v", msgs)
	}
	fs = m.buildVerifyPruneForm().(*formScreen)
	_, seen := pumpScreen(t, fs, tea.WindowSizeMsg{Width: 80, Height: 24}, keyMsg("y"))
	if op, ok := firstOpResult(seen); !ok || op.err != nil {
		t.Fatalf("verify --prune failed: %+v", seen)
	}

	// The history pager exposes restore.
	hp := m.buildHistoryPager().(*pagerScreen)
	_, cmd = hp.Update(keyMsg("R"))
	if got := pushedTitle(cmd); got != "restore" {
		t.Errorf("R pushed %q, want restore", got)
	}
	// The restore form validates + confirms the generation, then fails
	// cleanly when that generation has no snapshot (legacy vault: no log).
	fs = m.buildRestoreForm().(*formScreen)
	if !strings.Contains(fs.cliCommand(), "--generation=<generation>") {
		t.Errorf("restore preview placeholder: %q", fs.cliCommand())
	}
	_, seen = pumpScreen(t, fs,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		keyMsg("7"), tea.KeyMsg{Type: tea.KeyEnter},
		keyMsg("7"), tea.KeyMsg{Type: tea.KeyEnter})
	if op, ok := firstOpResult(seen); !ok || op.err == nil {
		t.Errorf("restore to a nonexistent generation should error: %+v", seen)
	}

	// The scan form submit produces a scan pager.
	fs = m.buildScanForm().(*formScreen)
	if !strings.Contains(fs.cliCommand(), "scan .") {
		t.Errorf("scan preview: %q", fs.cliCommand())
	}
}

func TestSettingsMenuAndConfigForms(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.Init()

	acts := listActs(t, m.buildSettings())
	if got := pushedTitle(acts["Config"]()); got != "config" {
		t.Errorf("Config pushed %q", got)
	}
	if got := pushedTitle(acts["Providers"]()); got != "providers" {
		t.Errorf("Providers pushed %q", got)
	}
	if got := pushedTitle(acts["CLI-only commands"]()); got != "cli-only" {
		t.Errorf("CLI-only pushed %q", got)
	}

	// Lock/Unlock toggles through the real CLI.
	if op, ok := firstOpResult(collectMsgs(m.toggleLock())); !ok || op.err != nil || op.okText != "vault locked" {
		t.Fatalf("toggleLock lock: %+v", op)
	}
	if locked, _ := ctl.locked(); !locked {
		t.Fatal("vault should be locked")
	}
	if op, ok := firstOpResult(collectMsgs(m.toggleLock())); !ok || op.okText != "vault unlocked" {
		t.Fatalf("toggleLock unlock: %+v", op)
	}

	// Config pager actions.
	cp := m.buildConfigPager().(*pagerScreen)
	_, cmd := cp.Update(keyMsg("s"))
	if got := pushedTitle(cmd); got != "config set" {
		t.Errorf("s pushed %q", got)
	}
	_, cmd = cp.Update(keyMsg("x"))
	if got := pushedTitle(cmd); got != "config unset" {
		t.Errorf("x pushed %q", got)
	}

	// config set / unset driven with a real key+value.
	fs := m.buildConfigSetForm().(*formScreen)
	if !strings.Contains(fs.cliCommand(), "--set <key>=") {
		t.Errorf("config set preview: %q", fs.cliCommand())
	}
	_, seen := pumpScreen(t, fs,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		keyMsg("n"), keyMsg("s"), tea.KeyMsg{Type: tea.KeyEnter}, // key: "ns"
		keyMsg("p"), tea.KeyMsg{Type: tea.KeyEnter}) // value: "p"
	if op, ok := firstOpResult(seen); !ok || op.err != nil {
		t.Fatalf("config set failed: %+v", seen)
	}
	if !strings.Contains(ctl.configText(), "ns=p") {
		t.Errorf("config not set: %q", ctl.configText())
	}
	fs = m.buildConfigUnsetForm().(*formScreen)
	_, seen = pumpScreen(t, fs,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		keyMsg("n"), keyMsg("s"), tea.KeyMsg{Type: tea.KeyEnter})
	if op, ok := firstOpResult(seen); !ok || op.err != nil {
		t.Fatalf("config unset failed: %+v", seen)
	}
	if strings.Contains(ctl.configText(), "ns=p") {
		t.Errorf("config not unset: %q", ctl.configText())
	}
}

func TestTextPagerEmptyContent(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	p := m.textPager("t", "  \n ")
	if v := p.View(40, 5); !strings.Contains(v, "(empty)") {
		t.Errorf("blank content should render (empty): %q", v)
	}
}

func TestPaletteCommandRouting(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.Init()

	if m.runPalette("") != nil {
		t.Error("empty palette command should be a no-op")
	}
	for cmd, want := range map[string]string{
		"secrets": "secrets", "s": "secrets",
		"access": "access", "caps": "access",
		"pw": "passwords", "maint": "maintenance",
		"set": "settings", "switch": "vaults", "folder": "vaults",
		"status": "status", "audit": "audit", "history": "history",
		"providers": "providers", "config": "config",
		"add": "add", "grant": "grant",
	} {
		got := pushedTitle(m.runPalette(cmd))
		if got != want {
			t.Errorf("palette %q pushed %q, want %q", cmd, got, want)
		}
	}
	// lock / unlock run the operation directly.
	if op, ok := firstOpResult(collectMsgs(m.runPalette("lock"))); !ok || op.okText != "vault locked" {
		t.Errorf("palette lock: %+v", op)
	}
	if op, ok := firstOpResult(collectMsgs(m.runPalette("unlock"))); !ok || op.okText != "vault unlocked" {
		t.Errorf("palette unlock: %+v", op)
	}
	// home pops to root; quit quits.
	if msgs := collectMsgs(m.runPalette("home")); !msgOfType(msgs, popToRootMsg{}) {
		t.Errorf("palette home: %+v", msgs)
	}
	if msgs := collectMsgs(m.runPalette("quit")); len(msgs) != 1 {
		t.Errorf("palette quit should produce the quit msg: %+v", msgs)
	}
}

func TestRequireAuthPathways(t *testing.T) {
	// Authenticated: runs the action immediately.
	ctl, home := newTestController(t)
	m := newRootModel(ctl)
	ran := false
	cmd := m.requireAuth(func() tea.Cmd { ran = true; return nil })
	if !ran || cmd != nil {
		t.Error("authed requireAuth should run the action inline")
	}

	// Unauthenticated file vault: pushes the unlock form; a wrong
	// passphrase is rejected inline, the right one runs the action.
	ctl.setActiveVault(vaultFolder(home), "")
	ran = false
	cmd = m.requireAuth(func() tea.Cmd { ran = true; return status("acted") })
	pm, ok := collectMsgs(cmd)[0].(pushMsg)
	if !ok || pm.s.Title() != "unlock" {
		t.Fatalf("unauthed requireAuth should push the unlock form, got %T", collectMsgs(cmd)[0])
	}
	if ran {
		t.Fatal("action must not run before authentication")
	}
	fs := pm.s.(*formScreen)
	// An empty passphrase is rejected inline; the form stays, no action.
	// (A legacy file vault defers WRONG-passphrase detection to the first
	// value read, matching the CLI, so only emptiness fails validation here.)
	var msgs []tea.Msg
	_, msgs = pumpScreen(t, fs,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter})
	if ran {
		t.Fatal("an empty passphrase must not run the gated action")
	}
	_ = msgs
	// A (non-empty) passphrase validates, pops, and runs the action.
	right := []tea.Msg{}
	for _, r := range "test-pass" {
		right = append(right, keyMsg(string(r)))
	}
	right = append(right, tea.KeyMsg{Type: tea.KeyEnter})
	_, msgs = pumpScreen(t, fs, right...)
	if !ran {
		t.Fatalf("correct passphrase should run the gated action: %+v", msgs)
	}
	if !ctl.authed {
		t.Error("controller should remember the session auth")
	}

	// A corrupt store surfaces as an error status.
	if err := os.WriteFile(StorePath(home), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	msgs = collectMsgs(m.requireAuth(func() tea.Cmd { return nil }))
	if st, ok := msgs[0].(setStatusMsg); !ok || !st.err {
		t.Errorf("corrupt store should produce an error status: %+v", msgs)
	}
}

func TestOrPlaceholderAndValidators(t *testing.T) {
	if orPlaceholder("  ", "<x>") != "<x>" || orPlaceholder("v", "<x>") != "v" {
		t.Error("orPlaceholder broken")
	}
	if optionalInt("") != nil || optionalInt(" 42 ") != nil {
		t.Error("optionalInt should accept blank and numbers")
	}
	if optionalInt("abc") == nil {
		t.Error("optionalInt should reject non-numbers")
	}
	if nonEmpty("field")("") == nil || nonEmpty("field")("x") != nil {
		t.Error("nonEmpty broken")
	}
	if atoiOr0(" 7 ") != 7 || atoiOr0("junk") != 0 {
		t.Error("atoiOr0 broken")
	}
}
