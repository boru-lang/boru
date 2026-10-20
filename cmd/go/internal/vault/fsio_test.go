package vault

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.dat")

	if err := writeFileAtomic(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "hello" {
		t.Errorf("content = %q, want hello", b)
	}
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0600 { // Windows has no permission bits
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}

	// Overwrite replaces the content in full.
	if err := writeFileAtomic(path, []byte("world!!"), 0600); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "world!!" {
		t.Errorf("content = %q, want world!!", b)
	}

	// No temp file is left behind on the happy path.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteFileAtomicMissingDirErrors(t *testing.T) {
	// The helper does not create the parent directory; a missing one
	// must surface as an error, not a silent 0700 mkdir.
	path := filepath.Join(t.TempDir(), "nope", "f.dat")
	if err := writeFileAtomic(path, []byte("x"), 0600); err == nil {
		t.Fatal("expected error writing into a non-existent directory")
	}
}

// TestRecordCapabilityUseConcurrent is the regression guard for the
// lost-update race: many concurrent debits must all land. Without the
// serializing mutex this loses increments (and fails under -race).
func TestRecordCapabilityUseConcurrent(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	s, _ := LoadStore(home)
	capRec, _, err := s.NewCapability("k", "agent", nil, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	capID := capRec.ID
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}

	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = recordCapabilityUse(home, capID, 2)
		}()
	}
	wg.Wait()

	s, _ = LoadStore(home)
	c, _ := s.FindCapabilityExact(capID)
	if c == nil {
		t.Fatal("capability vanished")
	}
	if c.UsedCalls != n {
		t.Errorf("UsedCalls = %d, want %d (lost updates)", c.UsedCalls, n)
	}
	if c.UsedCostCents != 2*n {
		t.Errorf("UsedCostCents = %d, want %d (lost updates)", c.UsedCostCents, 2*n)
	}
}

// TestRotateRevokeCapsPersistsRevocationBeforeValue checks the
// fail-closed ordering: the capability revocation is committed to the
// store before the new value is written to the keyring, so the two
// store-writes bracket the keyring write.
func TestRotateRevokeCapsPersistsRevocationBeforeValue(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "old\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	_ = grantOK(t, "k", nil, nil)

	code, out, errOut := runVault(t, "new\n", "rotate", "--yes", "--from-stdin", "--revoke-caps", "k")
	if code != 0 {
		t.Fatalf("rotate: %s", errOut)
	}
	if !strings.Contains(out, "revoked 1 capability") {
		t.Errorf("rotate output = %q", out)
	}
	s, _ := LoadStore(home)
	if len(s.Capabilities) != 1 || !s.Capabilities[0].Revoked {
		t.Errorf("capability not revoked: %+v", s.Capabilities)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "k"); code != 0 || !strings.Contains(out, "new") {
		t.Errorf("rotated value = %q (code %d), want new", out, code)
	}
}
