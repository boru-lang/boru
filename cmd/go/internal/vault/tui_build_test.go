package vault

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// pushed runs a command and returns the pushed screen's title, or "".
func pushedTitle(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	if pm, ok := cmd().(pushMsg); ok {
		return pm.s.Title()
	}
	return ""
}

func TestSecretsAddAuthGating(t *testing.T) {
	// Authenticated file vault: 'a' opens the add form even on an empty table
	// (the old empty-table guard used to block this).
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	secrets := m.buildSecrets()
	_, cmd := secrets.Update(keyMsg("a"))
	if got := pushedTitle(cmd); got != "add" {
		t.Errorf("authed add should open the add form, pushed %q", got)
	}
}

func TestSecretsAddRequiresUnlock(t *testing.T) {
	// Unauthenticated file vault: 'a' must prompt to unlock first, NOT fail at
	// submit time with "no input".
	home := testHome(t)
	mustInit(t)
	ctl := newTUIController(home)
	ctl.setActiveVault(vaultFolder(home), "") // not authenticated
	m := newRootModel(ctl)
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	secrets := m.buildSecrets()
	_, cmd := secrets.Update(keyMsg("a"))
	if got := pushedTitle(cmd); got != "unlock" {
		t.Errorf("unauthed add should prompt to unlock, pushed %q", got)
	}
}

func TestAddCommandPreview(t *testing.T) {
	got := addCommandPreview("proj:k", "github", "proj", "90d")
	for _, want := range []string{"boru vault add", "--provider=github", "--namespace=proj", "--expiry=90d", "proj:k"} {
		if !strings.Contains(got, want) {
			t.Errorf("preview %q missing %q", got, want)
		}
	}
	empty := addCommandPreview("", "", "", "")
	if strings.Contains(empty, "--provider") {
		t.Errorf("empty preview should carry no flags: %q", empty)
	}
	if !strings.Contains(empty, "<alias>") {
		t.Errorf("empty preview should show the <alias> placeholder: %q", empty)
	}
}

func TestAddFormValidators(t *testing.T) {
	// alias
	if validateAliasField("") == nil {
		t.Error("empty alias should be invalid")
	}
	if validateAliasField("good_name") != nil || validateAliasField("ns:name") != nil {
		t.Error("valid aliases rejected")
	}
	if validateAliasField("bad name") == nil {
		t.Error("alias with a space should be invalid")
	}
	// expiry (optional)
	if validateExpiryField("") != nil || validateExpiryField("90d") != nil {
		t.Error("empty/duration expiry should be valid")
	}
	if validateExpiryField("not-a-date") == nil {
		t.Error("garbage expiry should be invalid")
	}
	// namespace (optional)
	if validateNamespaceField("") != nil || validateNamespaceField("proj") != nil {
		t.Error("empty/valid namespace rejected")
	}
	if validateNamespaceField("bad ns") == nil {
		t.Error("namespace with a space should be invalid")
	}
}

func TestFormCommandPreviews(t *testing.T) {
	if got := rotateCommandPreview("k", "90d", true); !strings.Contains(got, "rotate") ||
		!strings.Contains(got, "--revoke-caps") || !strings.Contains(got, "--expiry=90d") ||
		!strings.Contains(got, "--from-stdin") || !strings.HasSuffix(got, " k") {
		t.Errorf("rotate preview: %q", got)
	}
	got := grantCommandPreview("k", "ci", "api.x", "GET", "2h", "5", "0", true)
	for _, want := range []string{"grant", "--agent=ci", "--hosts=api.x", "--methods=GET", "--ttl=2h", "--max-calls=5", "--require-approval", " k"} {
		if !strings.Contains(got, want) {
			t.Errorf("grant preview %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "max-cost") { // 0 should be omitted
		t.Errorf("grant preview should omit a zero max-cost: %q", got)
	}
	if got := grantCommandPreview("", "", "", "", "", "", "", false); !strings.Contains(got, "<alias>") {
		t.Errorf("grant placeholder missing: %q", got)
	}
	if got := expiryCommandPreview("k", ""); !strings.Contains(got, "expiry clear k") {
		t.Errorf("expiry clear: %q", got)
	}
	if got := expiryCommandPreview("k", "90d"); !strings.Contains(got, "expiry set k 90d") {
		t.Errorf("expiry set: %q", got)
	}
	if got := renameCommandPreview("a", "b", false); !strings.Contains(got, "mv a b") {
		t.Errorf("rename: %q", got)
	}
	if got := passwordAddCommandPreview("ci", "read", "*", ""); !strings.Contains(got, "password add") ||
		!strings.Contains(got, "--scope=read") || !strings.Contains(got, "--namespaces=*") {
		t.Errorf("password add: %q", got)
	}
	if got := createVaultCommandPreview("/v", "work", "file"); !strings.Contains(got, "--folder=/v") ||
		!strings.Contains(got, "--suffix=work") || !strings.Contains(got, "init --backend=file") {
		t.Errorf("create vault: %q", got)
	}
}

func TestFormScreenCliCommandIsLive(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	// The rotate form mirrors its alias into the command bar.
	fs, ok := m.buildRotateForm("github_token").(*formScreen)
	if !ok {
		t.Fatalf("rotate form is %T", m.buildRotateForm("github_token"))
	}
	if cmd := fs.cliCommand(); !strings.Contains(cmd, "boru vault rotate") || !strings.HasSuffix(cmd, "github_token") {
		t.Errorf("rotate form command bar: %q", cmd)
	}
}

func TestSecretsEnterOpensDetail(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	m := newRootModel(ctl)
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	secrets := m.buildSecrets()
	_, cmd := secrets.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := pushedTitle(cmd); got != "k" {
		t.Errorf("enter on a secret should open its detail page, pushed %q", got)
	}
}

func TestSecretDetailMasksRevealsAndInjects(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("github_token", "ghp-xyz", "github", "", ""); err != nil {
		t.Fatal(err)
	}
	m := newRootModel(ctl)
	cmds := injectCommands("boru vault", "github_token", "github")

	masked := m.secretDetailBody("github_token", false, "", cmds, 0)
	if strings.Contains(masked, "ghp-xyz") {
		t.Error("masked detail must not show the value")
	}
	// The --for recipe matches the secret's provider (github, not npm).
	for _, want := range []string{"hidden", "github", "boru vault exec github_token", "--for=github"} {
		if !strings.Contains(masked, want) {
			t.Errorf("detail missing %q", want)
		}
	}
	if strings.Contains(masked, "--for=npm") {
		t.Error("--for should match the provider (github), not be hardcoded npm")
	}

	shown := m.secretDetailBody("github_token", true, "ghp-xyz", cmds, 0)
	if !strings.Contains(shown, "ghp-xyz") {
		t.Error("revealed detail should show the value")
	}
}

func TestInjectCommandsMatchProvider(t *testing.T) {
	// A provider that is a known recipe -> --for matches it.
	got := injectCommands("boru vault", "vxg:github", "github")
	if len(got) != 3 || !strings.Contains(got[2].cmd, "--for=github ") {
		t.Errorf("github provider should yield --for=github: %q", got[2].cmd)
	}
	if c := injectCommands("boru vault", "k", "cargo")[2].cmd; !strings.Contains(c, "--for=cargo ") || !strings.Contains(c, "cargo publish") {
		t.Errorf("cargo: %q", c)
	}
	// A non-recipe provider (or none) -> generic placeholder.
	if c := injectCommands("boru vault", "k", "openai")[2].cmd; !strings.Contains(c, "--for=<tool>") {
		t.Errorf("non-recipe provider should be generic: %q", c)
	}
	if c := injectCommands("boru vault", "k", "")[2].cmd; !strings.Contains(c, "--for=<tool>") {
		t.Errorf("no provider should be generic: %q", c)
	}
	// The exec prefix carries the active vault's location flags.
	loc := injectCommands("boru vault --folder=~/.vxgboru01 --suffix=sdk01", "vxg:github", "github")
	for _, ic := range loc {
		if !strings.HasPrefix(ic.cmd, "boru vault --folder=~/.vxgboru01 --suffix=sdk01 exec ") {
			t.Errorf("sample command should carry the vault location: %q", ic.cmd)
		}
	}
}

func TestVaultLocationFlags(t *testing.T) {
	ctl, _ := newTestController(t)
	m := newRootModel(ctl)
	// The default vault (~/.boru, no suffix) adds no flags.
	ctl.folder = homeBoruDir(ctl.homeDir)
	ctl.suffix = ""
	if got := m.vaultLocationFlags(); got != "" {
		t.Errorf("default vault should add no flags, got %q", got)
	}
	if got := m.execPrefix(); got != "boru vault" {
		t.Errorf("default execPrefix = %q", got)
	}
	if got := m.withVaultLocation("boru vault list"); got != "boru vault list" {
		t.Errorf("default withVaultLocation = %q", got)
	}
	// A non-default vault contributes --folder/--suffix.
	ctl.folder = filepath.Join(ctl.homeDir, ".vxgboru01")
	ctl.suffix = "sdk01"
	folderFlag := "--folder=" + filepath.Join("~", ".vxgboru01") // native separator
	if got := m.vaultLocationFlags(); !strings.Contains(got, folderFlag) || !strings.Contains(got, "--suffix=sdk01") {
		t.Errorf("non-default flags = %q", got)
	}
	if got := m.withVaultLocation("boru vault list"); !strings.HasPrefix(got, "boru vault "+folderFlag+" --suffix=sdk01 list") {
		t.Errorf("withVaultLocation = %q", got)
	}
}

func TestSecretDetailSelectRevealCopy(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	m := newRootModel(ctl)
	s := m.buildSecretDetail("k").(*secretDetailScreen)

	// down arrow selects the next command.
	ns, _ := s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s = ns.(*secretDetailScreen)
	if s.cursor != 1 {
		t.Errorf("down -> cursor %d, want 1", s.cursor)
	}
	// a reveal message sets the shown value.
	ns, _ = s.Update(secretRevealedMsg{value: "v"})
	s = ns.(*secretDetailScreen)
	if !s.shown || s.value != "v" {
		t.Error("secretRevealedMsg should set shown+value")
	}
	if !strings.Contains(s.View(80, 24), "▸ ") {
		t.Error("the selected inject command should carry the ▸ cursor")
	}
}

func TestProviderOptionsUnion(t *testing.T) {
	custom := []Provider{
		{Name: "corp", BaseURL: "https://api.corp.example", AuthStyle: "bearer"},
		{Name: "github", BaseURL: "https://evil.example", AuthStyle: "bearer"}, // dup of a built-in
	}
	vals := map[string]int{}
	for _, o := range providerOptions(custom) {
		vals[o.Value]++
	}
	// HTTP providers, custom presets, AND publish-recipe tools are all
	// selectable.
	for _, want := range []string{"", "openai", "anthropic", "github", "generic", "corp", "npm", "cargo", "pypi", "yarn", "pnpm", "terraform", "hex", "cocoapods"} {
		if vals[want] == 0 {
			t.Errorf("provider options missing %q", want)
		}
	}
	// github is a provider preset, a recipe, and a (bogus) custom — must
	// still appear once.
	if vals["github"] != 1 {
		t.Errorf("github should appear once, got %d", vals["github"])
	}
	// A nil custom list degrades to the built-in union.
	if len(providerOptions(nil)) == 0 {
		t.Error("providerOptions(nil) should still list built-ins")
	}
}

func TestExpiryCountdown(t *testing.T) {
	future := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	if cd := expiryCountdown(future); !strings.HasPrefix(cd, "in ") {
		t.Errorf("future expiry countdown = %q, want 'in …'", cd)
	}
	past := time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)
	if cd := expiryCountdown(past); !strings.Contains(cd, "expired") {
		t.Errorf("past expiry countdown = %q, want 'expired …'", cd)
	}
	if cd := expiryCountdown(""); cd != "" {
		t.Errorf("empty expiry should have no countdown, got %q", cd)
	}
	if expiryCell("") != "-" {
		t.Error("empty expiry cell should be '-'")
	}
}

func TestClipboardCopyArgvWired(t *testing.T) {
	env := clipEnv{
		goos:     "darwin",
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "/usr/bin/x", nil },
	}
	c, err := detectClipboard(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.copyArgv) == 0 || c.copyArgv[0] != "pbcopy" {
		t.Errorf("darwin copyArgv = %v, want [pbcopy]", c.copyArgv)
	}
}

func TestCommandBarAndStatusBar(t *testing.T) {
	ctl, _ := newTestController(t)
	if err := ctl.addSecret("k", "v", "", "", ""); err != nil {
		t.Fatal(err)
	}
	m := newRootModel(ctl)
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = feed(t, m, pushMsg{m.buildSecrets()})

	if cb := m.commandBar(); !strings.Contains(cb, "boru vault list") {
		t.Errorf("command bar should show the CLI equivalent: %q", cb)
	}
	sb := m.statusBar()
	if !strings.Contains(sb, "secrets") || !strings.Contains(sb, "1 item") {
		t.Errorf("status bar should show screen + count: %q", sb)
	}
}
