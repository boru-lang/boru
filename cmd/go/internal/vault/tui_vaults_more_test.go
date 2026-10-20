package vault

// tui_vaults_more_test.go — the vault index CLI (`boru vault folder` list /
// rm), index default handling, discovery ordering, and launch selection.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVaultsListOutput(t *testing.T) {
	home := testHome(t)
	// No vaults at all.
	code, out, _ := runVault(t, "", "folder")
	if code != 0 || !strings.Contains(out, "no vaults found") {
		t.Errorf("empty list = %d %q", code, out)
	}

	// One real (locked) vault, one default-marked, one stale index entry.
	mustInit(t)
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if err := setDefaultVault(home, vaultFolder(home), ""); err != nil {
		t.Fatal(err)
	}
	if err := recordVaultInit(home, filepath.Join(home, "gone"), "old", "file"); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runVault(t, "", "folder")
	if code != 0 {
		t.Fatalf("list: %s", errOut)
	}
	if !strings.Contains(out, "*") {
		t.Errorf("default marker missing:\n%s", out)
	}
	if !strings.Contains(out, "locked") {
		t.Errorf("locked status missing:\n%s", out)
	}
	if !strings.Contains(out, "missing") {
		t.Errorf("missing status for the stale entry:\n%s", out)
	}
	if !strings.Contains(out, "(default)") {
		t.Errorf("default vault display name missing:\n%s", out)
	}
}

func TestRunVaultsRemoveForgetsEntry(t *testing.T) {
	home := testHome(t)
	custom := filepath.Join(home, "external")
	writeMeta(t, custom, "sdk", "file")
	if code, _, e := runVault(t, "", "folder", "add", custom); code != 0 {
		t.Fatalf("add: %s", e)
	}

	// Usage error without a directory.
	if code, _, errOut := runVault(t, "", "folder", "rm"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("rm without args should print usage: %q", errOut)
	}
	// Removing an entry that is not indexed errors.
	if code, _, errOut := runVault(t, "", "folder", "rm", filepath.Join(home, "nope")); code == 0 ||
		!strings.Contains(errOut, "no index entry") {
		t.Errorf("rm of unindexed folder: %q", errOut)
	}
	// The suffix must match: rm without --suffix targets suffix "".
	if code, _, _ := runVault(t, "", "folder", "rm", custom); code == 0 {
		t.Error("rm without the matching --suffix should fail")
	}
	// With the suffix it forgets the entry but leaves the files.
	code, out, errOut := runVault(t, "", "--suffix=sdk", "folder", "rm", custom)
	if code != 0 {
		t.Fatalf("rm --suffix=sdk: %s", errOut)
	}
	if !strings.Contains(out, "forgot vault") {
		t.Errorf("rm output: %q", out)
	}
	idx, _ := loadVaultIndex(home)
	if ref, _ := idx.find(custom, "sdk"); ref != nil {
		t.Error("entry still indexed after rm")
	}
	if _, err := os.Stat(filepath.Join(custom, metaFileName("sdk"))); err != nil {
		t.Errorf("rm must not delete the vault files: %v", err)
	}
}

func TestRunVaultsAddRejectsMissingMetadata(t *testing.T) {
	home := testHome(t)
	// --suffix names a vault that does not exist in the folder.
	folder := filepath.Join(home, "somefolder")
	writeMeta(t, folder, "real", "file")
	code, _, errOut := runVault(t, "", "--suffix=fake", "folder", "add", folder)
	if code == 0 || !strings.Contains(errOut, "no vault metadata") {
		t.Errorf("add with wrong --suffix: %q", errOut)
	}
	// No argument at all is a usage error.
	if code, _, errOut := runVault(t, "", "folder", "add"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("add without args: %q", errOut)
	}
}

func TestClearDefaultsKeepsSingleDefault(t *testing.T) {
	home := testHome(t)
	if err := setDefaultVault(home, "/a", ""); err != nil {
		t.Fatal(err)
	}
	if err := setDefaultVault(home, "/b", "x"); err != nil {
		t.Fatal(err)
	}
	idx, _ := loadVaultIndex(home)
	defaults := 0
	for _, ref := range idx.Vaults {
		if ref.Default {
			defaults++
			if ref.Folder != filepath.FromSlash("/b") || ref.Suffix != "x" {
				t.Errorf("wrong default: %+v", ref)
			}
		}
	}
	if defaults != 1 {
		t.Errorf("defaults = %d, want exactly 1", defaults)
	}
}

func TestSortDiscoveredVaultsOrdering(t *testing.T) {
	vs := []discoveredVault{
		{vaultRef: vaultRef{Folder: "/z", Suffix: "b"}},
		{vaultRef: vaultRef{Folder: "/z", Suffix: "a"}},
		{vaultRef: vaultRef{Folder: "/m", LastOpenedAt: "2026-01-02T00:00:00Z"}},
		{vaultRef: vaultRef{Folder: "/n", LastOpenedAt: "2026-01-03T00:00:00Z"}},
		{vaultRef: vaultRef{Folder: "/d", Default: true}},
	}
	sortDiscoveredVaults(vs)
	if vs[0].Folder != "/d" {
		t.Errorf("default should sort first: %+v", vs[0])
	}
	if vs[1].Folder != "/n" || vs[2].Folder != "/m" {
		t.Errorf("most recently opened should follow: %+v", vs[:3])
	}
	if vs[3].Suffix != "a" || vs[4].Suffix != "b" {
		t.Errorf("folder/suffix tie-break wrong: %+v", vs[3:])
	}
}

func TestDiscoveredVaultDisplayName(t *testing.T) {
	if (discoveredVault{vaultRef: vaultRef{Label: "Work"}}).displayName() != "Work" {
		t.Error("label should win")
	}
	if (discoveredVault{vaultRef: vaultRef{Suffix: "ci"}}).displayName() != "ci" {
		t.Error("suffix fallback")
	}
	if (discoveredVault{}).displayName() != "(default)" {
		t.Error("default fallback")
	}
	if labelOrDefault("") != "(default)" || labelOrDefault("x") != "x" {
		t.Error("labelOrDefault broken")
	}
	got := quoteSuffixes([]string{"", "work"})
	if got[0] != "(default)" || got[1] != "work" {
		t.Errorf("quoteSuffixes = %v", got)
	}
}

func TestLaunchVaultSelection(t *testing.T) {
	home := testHome(t)
	// Nothing discoverable.
	if _, _, ok := launchVault(home); ok {
		t.Error("no vaults should mean nothing to launch")
	}
	// A stale (missing) index entry alone is not launchable.
	if err := recordVaultInit(home, filepath.Join(home, "gone"), "x", "file"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := launchVault(home); ok {
		t.Error("a stale-only index should mean nothing to launch")
	}
	// A real vault becomes the launch target.
	writeMeta(t, filepath.Join(home, ".boru"), "", "file")
	folder, suffix, ok := launchVault(home)
	if !ok || suffix != "" || folder != filepath.Join(home, ".boru") {
		t.Errorf("launchVault = %q %q %v", folder, suffix, ok)
	}
	// A default-marked second vault wins over the plain one.
	alt := filepath.Join(home, "alt")
	writeMeta(t, alt, "work", "file")
	if err := setDefaultVault(home, alt, "work"); err != nil {
		t.Fatal(err)
	}
	folder, suffix, ok = launchVault(home)
	if !ok || folder != alt || suffix != "work" {
		t.Errorf("default vault should win: %q %q %v", folder, suffix, ok)
	}
}

func TestLoadVaultIndexCorrupt(t *testing.T) {
	home := testHome(t)
	if err := os.MkdirAll(homeBoruDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vaultIndexPath(home), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadVaultIndex(home); err == nil {
		t.Error("corrupt index should be reported")
	}
	// Discovery treats it as empty rather than failing.
	if _, err := enumerateVaults(home); err != nil {
		t.Errorf("enumerateVaults should tolerate a corrupt index: %v", err)
	}
	// recordVaultOpened / setDefaultVault start fresh rather than failing.
	if err := recordVaultOpened(home, "/v", ""); err != nil {
		t.Errorf("recordVaultOpened over corrupt index: %v", err)
	}
	// pruneVaultEntry, however, refuses to guess.
	if err := os.WriteFile(vaultIndexPath(home), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pruneVaultEntry(home, "/v", ""); err == nil {
		t.Error("pruneVaultEntry on a corrupt index should error")
	}
}

func TestSaveVaultIndexNoHome(t *testing.T) {
	// An empty home silently skips the write instead of inventing a path.
	if err := saveVaultIndex("", &vaultIndex{}); err != nil {
		t.Errorf("saveVaultIndex(\"\") = %v, want nil", err)
	}
}

func TestPeekStoreUnreadable(t *testing.T) {
	dir := t.TempDir()
	// Absent file.
	if _, _, ok := peekStore(dir, ""); ok {
		t.Error("absent metadata should not peek")
	}
	// Unparseable file.
	if err := os.WriteFile(filepath.Join(dir, "vault.jsonic"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := peekStore(dir, ""); ok {
		t.Error("corrupt metadata should not peek")
	}
}
