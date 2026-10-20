package vault

// ADR-008 coverage for borubridge.go — the modules.VaultSpec adapter and
// the `boru vault -i --boru` launcher. Ops run against REAL temp vaults
// (file backend, the newTestController harness), so the bridge is
// exercised over the same command layer the bubbletea TUI drives;
// positives are paired with the error arms per lang/go/CLAUDE.md.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

// probeBackendsRegistered proves registerBoruTuiBackends ran before the
// program: a second registration of either backend must be refused as
// already registered.
func probeBackendsRegistered(a *lang.Boru) error {
	vErr := modules.RegisterHostVault(a.NativeRegistry(), modules.VaultSpec{
		Name: "probe", Do: func(string, map[string]any) (any, error) { return nil, nil },
	})
	if vErr == nil || !strings.Contains(vErr.Error(), "already registered") {
		return fmt.Errorf("vault backend not registered before run: %v", vErr)
	}
	tErr := modules.RegisterHostTui(a.NativeRegistry(), modules.TuiSpec{
		Name: "probe",
		Open: func() (tuikit.Backend, error) { return tuikit.NewVirtualBackend(1, 1), nil },
	})
	if tErr == nil || !strings.Contains(tErr.Error(), "already registered") {
		return fmt.Errorf("tui backend not registered before run: %v", tErr)
	}
	return nil
}

func newTestBridge(t *testing.T) (*boruBridge, string) {
	t.Helper()
	ctl, home := newTestController(t)
	return newBoruBridge(ctl), home
}

// mustDo runs one op and fails the test on error.
func mustDo(t *testing.T, b *boruBridge, op string, params map[string]any) any {
	t.Helper()
	res, err := b.do(op, params)
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
	return res
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("not a map: %#v", v)
	}
	return m
}

// The param readers accept every wire shape the lang side can send and
// zero-default the rest.
func TestBridgeParamReaders(t *testing.T) {
	p := map[string]any{
		"s": "x", "b": true, "i": 1, "i64": int64(2), "f": 3.0,
		"list": []any{"a", 5, "b"},
	}
	if bpStr(p, "s") != "x" || bpStr(p, "missing") != "" {
		t.Error("bpStr")
	}
	if !bpBool(p, "b") || bpBool(p, "missing") {
		t.Error("bpBool")
	}
	if bpInt(p, "i") != 1 || bpInt(p, "i64") != 2 || bpInt(p, "f") != 3 || bpInt(p, "s") != 0 {
		t.Error("bpInt")
	}
	if got := bpStrList(p, "list"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("bpStrList = %v", got)
	}
}

// Session ops: status carries the header fields and the exec prefix;
// authentication round-trips; the text panels capture CLI output.
func TestBridgeSessionOps(t *testing.T) {
	b, _ := newTestBridge(t)
	st := asMap(t, mustDo(t, b, "status", nil))
	if st["ok"] != true || st["backend"] != "file" || st["aliases"] != 0 {
		t.Fatalf("status = %#v", st)
	}
	if st["exec-prefix"] != "boru vault" {
		t.Fatalf("exec-prefix = %v", st["exec-prefix"])
	}
	if need := mustDo(t, b, "needs-passphrase", nil); need != true {
		t.Fatalf("needs-passphrase = %v", need)
	}
	if authed := mustDo(t, b, "authed", nil); authed != true {
		t.Fatalf("authed = %v", authed)
	}
	if _, err := b.do("authenticate", map[string]any{"pass": "wrong"}); err == nil {
		t.Error("authenticate(wrong) should error")
	}
	if ok := mustDo(t, b, "authenticate", map[string]any{"pass": "test-pass"}); ok != true {
		t.Fatalf("authenticate = %v", ok)
	}
	for _, op := range []string{"status-text", "providers-text", "config-text", "history"} {
		if s, _ := mustDo(t, b, op, nil).(string); s == "" {
			t.Errorf("%s returned empty text", op)
		}
	}
	// non-default location flags reach the exec prefix
	b.ctl.folder = "/elsewhere/vaults"
	b.ctl.suffix = "work"
	if got := b.execPrefix(); got != "boru vault --folder=/elsewhere/vaults --suffix=work" {
		t.Fatalf("execPrefix = %q", got)
	}
}

// Secrets ops end to end: add → list/find → recipes → reveal → rotate →
// rename → expiry → remove, plus the miss/error arms.
func TestBridgeSecretOps(t *testing.T) {
	b, _ := newTestBridge(t)
	mustDo(t, b, "add", map[string]any{"alias": "gh_token", "value": "v1", "provider": "github"})
	rows, _ := mustDo(t, b, "secrets", nil).([]any)
	if len(rows) != 1 || asMap(t, rows[0])["name"] != "gh_token" ||
		asMap(t, rows[0])["provider"] != "github" {
		t.Fatalf("secrets = %#v", rows)
	}
	if m := asMap(t, mustDo(t, b, "secret", map[string]any{"alias": "gh_token"})); m["name"] != "gh_token" {
		t.Fatalf("secret = %#v", m)
	}
	if missing := mustDo(t, b, "secret", map[string]any{"alias": "nope"}); missing != nil {
		t.Fatalf("secret(missing) = %#v", missing)
	}
	recipes, _ := mustDo(t, b, "recipes", map[string]any{"alias": "gh_token"}).([]any)
	if len(recipes) < 3 || !strings.Contains(asMap(t, recipes[0])["cmd"].(string), "boru vault exec gh_token") {
		t.Fatalf("recipes = %#v", recipes)
	}
	if got := mustDo(t, b, "reveal", map[string]any{"alias": "gh_token"}); got != "v1" {
		t.Fatalf("reveal = %v", got)
	}
	if _, err := b.do("reveal", map[string]any{"alias": "nope"}); err == nil {
		t.Error("reveal(missing) should error")
	}
	mustDo(t, b, "rotate", map[string]any{"alias": "gh_token", "value": "v2"})
	if got := mustDo(t, b, "reveal", map[string]any{"alias": "gh_token"}); got != "v2" {
		t.Fatalf("after rotate reveal = %v", got)
	}
	mustDo(t, b, "rename", map[string]any{"from": "gh_token", "to": "gh2"})
	mustDo(t, b, "set-expiry", map[string]any{"alias": "gh2", "when": "2030-01-01"})
	if m := asMap(t, mustDo(t, b, "secret", map[string]any{"alias": "gh2"})); m["expires-at"] == "" {
		t.Fatalf("expiry not set: %#v", m)
	}
	mustDo(t, b, "clear-expiry", map[string]any{"alias": "gh2"})
	mustDo(t, b, "remove", map[string]any{"alias": "gh2"})
	if rows, _ := mustDo(t, b, "secrets", nil).([]any); len(rows) != 0 {
		t.Fatalf("secrets after remove = %#v", rows)
	}
}

// Access ops: grant returns the one-time token output, the capability
// lists as active, revoke deactivates it.
func TestBridgeAccessOps(t *testing.T) {
	b, _ := newTestBridge(t)
	mustDo(t, b, "add", map[string]any{"alias": "k", "value": "v"})
	out, _ := mustDo(t, b, "grant", map[string]any{
		"alias": "k", "agent": "agent1", "ttl": "2h", "max-calls": 3,
	}).(string)
	if !strings.Contains(out, "token:") {
		t.Fatalf("grant output = %q", out)
	}
	// a second grant: two active IDs exercise the deterministic sort
	mustDo(t, b, "grant", map[string]any{"alias": "k", "agent": "agent2"})
	capsRes := asMap(t, mustDo(t, b, "capabilities", nil))
	caps, _ := capsRes["caps"].([]any)
	active, _ := capsRes["active"].([]any)
	if len(caps) != 2 || len(active) != 2 {
		t.Fatalf("capabilities = %#v", capsRes)
	}
	if active[0].(string) > active[1].(string) {
		t.Fatalf("active IDs not sorted: %#v", active)
	}
	row := asMap(t, caps[0])
	if row["alias"] != "k" || row["agent"] != "agent1" || row["active"] != true ||
		row["max-calls"] != 3 {
		t.Fatalf("capability row = %#v", row)
	}
	mustDo(t, b, "revoke", map[string]any{"id": row["id"].(string)})
	capsRes = asMap(t, mustDo(t, b, "capabilities", nil))
	if active, _ := capsRes["active"].([]any); len(active) != 1 {
		t.Fatalf("active after revoke = %#v", active)
	}
	if _, err := b.do("revoke", map[string]any{"id": "nope"}); err == nil {
		t.Error("revoke(missing) should error")
	}
}

// Password-slot ops: add a temp slot, count it, list it (metadata only),
// bulk-revoke it, and remove the base slot.
func TestBridgePasswordOps(t *testing.T) {
	b, _ := newTestBridge(t)
	mustDo(t, b, "password-add", map[string]any{"name": "ci", "scope": "read", "pass": "ci-pass"})
	mustDo(t, b, "password-add", map[string]any{"name": "tmp", "scope": "read", "pass": "tmp-pass", "ttl": "1h"})
	// the first password add also mints the admin slot: admin + ci + tmp
	rows, _ := mustDo(t, b, "passwords", nil).([]any)
	if len(rows) != 3 {
		t.Fatalf("passwords = %#v", rows)
	}
	names := map[string]string{}
	for _, r := range rows {
		m := asMap(t, r)
		names[m["name"].(string)] = m["scope"].(string)
		for _, forbidden := range []string{"salt", "verifier", "public_key", "enc_priv_key"} {
			if _, ok := m[forbidden]; ok {
				t.Fatalf("crypto field %q leaked into the bridge row: %#v", forbidden, m)
			}
		}
	}
	if names["ci"] != "read" || names["tmp"] != "read" || names["admin"] != "admin" {
		t.Fatalf("slot scopes = %#v", names)
	}
	if n := mustDo(t, b, "temp-password-count", nil); n != 1 {
		t.Fatalf("temp-password-count = %v", n)
	}
	mustDo(t, b, "password-revoke-temp", nil)
	if n := mustDo(t, b, "temp-password-count", nil); n != 0 {
		t.Fatalf("temp count after revoke = %v", n)
	}
	mustDo(t, b, "password-remove", map[string]any{"name": "ci"})
	if _, err := b.do("password-remove", map[string]any{"name": "nope"}); err == nil {
		t.Error("password-remove(missing) should error")
	}
}

// Maintenance + settings ops: verify/scan/audit capture text, restore
// rolls back a generation, config and lock round-trip, prefs persist.
func TestBridgeMaintenanceAndSettings(t *testing.T) {
	b, home := newTestBridge(t)
	v := asMap(t, mustDo(t, b, "verify", map[string]any{}))
	if v["ok"] != true || v["text"] == "" {
		t.Fatalf("verify = %#v", v)
	}
	if s, _ := mustDo(t, b, "scan", map[string]any{"paths": []any{home}}).(string); s == "" {
		t.Error("scan returned empty text")
	}
	if s, _ := mustDo(t, b, "scan", map[string]any{}).(string); s == "" {
		t.Error("scan with default paths returned empty text")
	}
	mustDo(t, b, "add", map[string]any{"alias": "a", "value": "v"})
	if s, _ := mustDo(t, b, "audit", map[string]any{}).(string); !strings.Contains(s, "vault.add") {
		t.Errorf("audit text = %q", s)
	}
	if s, _ := mustDo(t, b, "audit", map[string]any{"limit": "1"}).(string); s == "" {
		t.Error("filtered audit returned empty text")
	}
	// restore's success path is pinned at the controller layer
	// (TestControllerRestore); the bridge arm forwards the generation and
	// surfaces the CLI's refusal
	if _, err := b.do("restore", map[string]any{"generation": 999999}); err == nil {
		t.Error("restore(bad generation) should error")
	}

	mustDo(t, b, "config-set", map[string]any{"key": "namespace.default", "value": "team"})
	if s, _ := mustDo(t, b, "config-text", nil).(string); !strings.Contains(s, "team") {
		t.Errorf("config-text = %q", s)
	}
	mustDo(t, b, "config-unset", map[string]any{"key": "namespace.default"})
	mustDo(t, b, "lock", nil)
	if locked := mustDo(t, b, "locked", nil); locked != true {
		t.Fatalf("locked = %v", locked)
	}
	mustDo(t, b, "unlock", nil)
	if locked := mustDo(t, b, "locked", nil); locked != false {
		t.Fatalf("locked after unlock = %v", locked)
	}

	mustDo(t, b, "set-prefs", map[string]any{"theme": "dark"})
	if p := asMap(t, mustDo(t, b, "prefs", nil)); p["theme"] != "dark" {
		t.Fatalf("prefs = %#v", p)
	}
}

// Multi-vault ops: the index lists the active vault, create/switch move
// the controller, set-default and prune-index maintain the index.
func TestBridgeMultiVaultOps(t *testing.T) {
	b, home := newTestBridge(t)
	rows, _ := mustDo(t, b, "vaults", nil).([]any)
	foundActive := false
	for _, r := range rows {
		if asMap(t, r)["active"] == true {
			foundActive = true
		}
	}
	if !foundActive {
		t.Fatalf("no active vault in index = %#v", rows)
	}
	second := filepath.Join(home, "second")
	mustDo(t, b, "create", map[string]any{"folder": second, "backend": "file", "pass": "p2"})
	if b.ctl.folder != second {
		t.Fatalf("create did not switch: %q", b.ctl.folder)
	}
	mustDo(t, b, "set-default", nil)
	mustDo(t, b, "switch", map[string]any{"folder": vaultFolder(home)})
	if _, err := b.do("switch", map[string]any{"folder": home + "/missing"}); err == nil {
		t.Error("switch(missing) should error")
	}
	mustDo(t, b, "prune-index", map[string]any{"folder": second})
	if _, err := b.do("create", map[string]any{"folder": second, "backend": "bogus"}); err == nil {
		t.Error("create(bogus backend) should error")
	}
}

// The clipboard op rides the t7detectClipboard seam; unknown ops error.
func TestBridgeCopyAndUnknown(t *testing.T) {
	b, _ := newTestBridge(t)
	orig := t7detectClipboard
	t.Cleanup(func() { t7detectClipboard = orig })
	t7detectClipboard = func(clipEnv) (*clipboardCmd, error) {
		return &clipboardCmd{label: "t7-fake", copyArgv: []string{"cat"}}, nil
	}
	if label := mustDo(t, b, "copy", map[string]any{"text": "hi"}); label != "t7-fake" {
		t.Fatalf("copy = %v", label)
	}
	t7detectClipboard = func(clipEnv) (*clipboardCmd, error) {
		return nil, fmt.Errorf("no clipboard")
	}
	if _, err := b.do("copy", map[string]any{"text": "hi"}); err == nil {
		t.Error("copy without a clipboard should error")
	}
	if _, err := b.do("bogus-op", nil); err == nil ||
		!strings.Contains(err.Error(), `unknown vault op "bogus-op"`) {
		t.Errorf("unknown op = %v", err)
	}
}

// Read-op error arms: a corrupt store file propagates from every
// store-loading op.
func TestBridgeCorruptStoreErrors(t *testing.T) {
	b, home := newTestBridge(t)
	if err := os.WriteFile(StorePath(home), []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"status", "needs-passphrase", "secrets", "capabilities", "passwords", "locked"} {
		if _, err := b.do(op, nil); err == nil {
			t.Errorf("%s over a corrupt store should error", op)
		}
	}
	if _, err := b.do("authenticate", map[string]any{"pass": "test-pass"}); err == nil {
		t.Error("authenticate over a corrupt store should error")
	}
}

// Ops are serialized: concurrent bridge calls (the spawn-and-send
// idiom) never race the env promotion.
func TestBridgeConcurrentOps(t *testing.T) {
	b, _ := newTestBridge(t)
	mustDo(t, b, "add", map[string]any{"alias": "k", "value": "v"})
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := b.do("reveal", map[string]any{"alias": "k"}); err != nil {
				errs <- err
			}
			if _, err := b.do("status", nil); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent op: %v", err)
	}
}

// The launcher: --boru routes into runInteractiveBoru; a non-terminal is
// refused; the init-error and run-error arms surface through the seams;
// the success arm registers both backends and runs the program source.
func TestRunInteractiveBoruArms(t *testing.T) {
	_, home := newTestBridge(t)

	// --boru flag routes here; non-TTY stdin (not an *os.File) refuses
	var errb strings.Builder
	if code := runInteractive([]string{"--boru"}, home, strings.NewReader(""), &errb, &errb); code != 1 {
		t.Fatalf("--boru without a terminal = %d", code)
	}
	if !strings.Contains(errb.String(), "requires a terminal") {
		t.Fatalf("stderr = %q", errb.String())
	}

	// a fake terminal via the t7isTerminal seam + /dev/null files
	origTerm := t7isTerminal
	t7isTerminal = func(int) bool { return true }
	t.Cleanup(func() { t7isTerminal = origTerm })
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	devnullW, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnullW.Close()

	// lang.New failure arm
	origNew := b8langNew
	b8langNew = func(...lang.Options) (*lang.Boru, error) { return nil, fmt.Errorf("b8-boom-init") }
	var errInit strings.Builder
	if code := runInteractiveBoru(home, devnull, devnullW, &errInit); code != 1 ||
		!strings.Contains(errInit.String(), "b8-boom-init") {
		t.Fatalf("init-error arm = %d, %q", code, errInit.String())
	}
	b8langNew = origNew
	t.Cleanup(func() { b8langNew = origNew })

	// run failure arm + success arm, with the program run seamed out (a
	// real run would take over a terminal); the seam still proves both
	// backends were registered and the source carries the dark flag
	origRun := b8runSource
	t.Cleanup(func() { b8runSource = origRun })
	var gotSrc string
	b8runSource = func(a *lang.Boru, src string) error {
		gotSrc = src
		return probeBackendsRegistered(a)
	}
	var out strings.Builder
	if code := runInteractiveBoru(home, devnull, devnullW, &out); code != 0 {
		t.Fatalf("success arm = %d, %q", code, out.String())
	}
	if !strings.Contains(gotSrc, `import "boru:vault-tui"`) ||
		!strings.Contains(gotSrc, "VaultTui.run {dark:") ||
		!strings.Contains(gotSrc, ") drop") {
		t.Fatalf("program source = %q", gotSrc)
	}
	b8runSource = func(*lang.Boru, string) error { return fmt.Errorf("b8-boom-run") }
	var errRun strings.Builder
	if code := runInteractiveBoru(home, devnull, devnullW, &errRun); code != 1 ||
		!strings.Contains(errRun.String(), "b8-boom-run") {
		t.Fatalf("run-error arm = %d, %q", code, errRun.String())
	}

	// no launch vault at all: the picker-pending arm defaults the active
	// location to the home ~/.boru
	b8runSource = func(a *lang.Boru, src string) error { return nil }
	emptyHome := t.TempDir()
	t.Setenv(EnvFolder, "")
	t.Setenv(EnvSuffix, "")
	var out2 strings.Builder
	if code := runInteractiveBoru(emptyHome, devnull, devnullW, &out2); code != 0 {
		t.Fatalf("empty-home arm = %d, %q", code, out2.String())
	}

	// the default run seam really executes source on the interpreter
	aReal, err := lang.New(lang.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rErr := origRun(aReal, "1 drop"); rErr != nil {
		t.Fatalf("default run seam: %v", rErr)
	}
	if rErr := origRun(aReal, "no-such-word-b8"); rErr == nil {
		t.Fatal("default run seam swallowed an error")
	}
}
