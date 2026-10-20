package vault

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// integHome creates a temp home with a RAW store file at the given
// generation (no sidecar/journal — those are SaveStore side effects we
// want these tests to control explicitly) and returns (home, storeBytes).
func integHome(t *testing.T, gen int64) (string, []byte) {
	t.Helper()
	t.Setenv(EnvSuffix, "")
	t.Setenv(EnvFolder, "")
	dir := t.TempDir()
	if err := os.MkdirAll(vaultFolder(dir), 0700); err != nil {
		t.Fatal(err)
	}
	s := &Store{Version: storeVersion, Backend: BackendFile, Generation: gen}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StorePath(dir), data, 0600); err != nil {
		t.Fatal(err)
	}
	return dir, data
}

func TestSidecarRoundTrip(t *testing.T) {
	dir, data := integHome(t, 7)
	ik := mustRandom(t, 32)
	if err := writeSidecar(dir, 7, data, ik); err != nil {
		t.Fatal(err)
	}
	sc, err := readSidecar(dir)
	if err != nil || sc == nil {
		t.Fatalf("readSidecar: %v", err)
	}
	if sc.Generation != 7 || sc.SHA256 != storeSHA256(data) {
		t.Errorf("sidecar = %+v, want gen 7 + matching sha", sc)
	}
	if sc.HMAC != storeHMAC(ik, 7, sc.SHA256) {
		t.Error("sidecar HMAC mismatch")
	}
	info, _ := os.Stat(sidecarPath(dir))
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 { // Windows has no permission bits
		t.Errorf("sidecar mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestCheckStoreIntegrityStates(t *testing.T) {
	ik := mustRandom(t, 32)

	t.Run("no-baseline", func(t *testing.T) {
		dir, _ := integHome(t, 1)
		got, _ := checkStoreIntegrity(dir, ik, 0)
		if got != IntegrityNoBaseline {
			t.Errorf("got %v, want no-baseline", got)
		}
	})

	t.Run("ok-keyed", func(t *testing.T) {
		dir, data := integHome(t, 2)
		_ = writeSidecar(dir, 2, data, ik)
		if got, _ := checkStoreIntegrity(dir, ik, 0); got != IntegrityOK {
			t.Errorf("got %v, want ok", got)
		}
	})

	t.Run("ok-legacy-keyless", func(t *testing.T) {
		dir, data := integHome(t, 2)
		_ = writeSidecar(dir, 2, data, nil) // no hmac
		if got, _ := checkStoreIntegrity(dir, nil, 0); got != IntegrityOK {
			t.Errorf("got %v, want ok", got)
		}
	})

	t.Run("unverified-no-key", func(t *testing.T) {
		dir, data := integHome(t, 2)
		_ = writeSidecar(dir, 2, data, ik) // keyed sidecar
		if got, _ := checkStoreIntegrity(dir, nil, 0); got != IntegrityUnverified {
			t.Errorf("got %v, want unverified", got)
		}
	})

	t.Run("hmac-invalid", func(t *testing.T) {
		dir, data := integHome(t, 2)
		_ = writeSidecar(dir, 2, data, mustRandom(t, 32)) // signed with a DIFFERENT key
		if got, _ := checkStoreIntegrity(dir, ik, 0); got != IntegrityHMACInvalid {
			t.Errorf("got %v, want hmac-invalid", got)
		}
	})

	t.Run("external-change", func(t *testing.T) {
		dir, data := integHome(t, 2)
		_ = writeSidecar(dir, 2, data, ik)
		// Tamper the store content after signing.
		if err := os.WriteFile(StorePath(dir), append(data, ' '), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, _ := checkStoreIntegrity(dir, ik, 0); got != IntegrityExternalChange {
			t.Errorf("got %v, want external-change", got)
		}
	})

	t.Run("rollback", func(t *testing.T) {
		dir, data := integHome(t, 3)
		_ = writeSidecar(dir, 3, data, ik)
		// Anchor says we have committed up to generation 9.
		if got, _ := checkStoreIntegrity(dir, ik, 9); got != IntegrityRollback {
			t.Errorf("got %v, want rollback-detected", got)
		}
	})

	t.Run("corrupt", func(t *testing.T) {
		dir, _ := integHome(t, 1)
		if err := os.WriteFile(StorePath(dir), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, _ := checkStoreIntegrity(dir, ik, 0); got != IntegrityCorrupt {
			t.Errorf("got %v, want corrupt", got)
		}
	})
}

// TestSidecarDeletionCannotLaunder: deleting the sidecar after tampering
// is caught by reconciling against the journal head (L1).
func TestSidecarDeletionCannotLaunder(t *testing.T) {
	ik := mustRandom(t, 32)
	dir, data := integHome(t, 4)
	sha := storeSHA256(data)
	if err := appendJournal(dir, ik, 4, "user", "vault.add", sha, &Store{Version: storeVersion}); err != nil {
		t.Fatal(err)
	}
	// No sidecar, but the live store still matches the journal head -> ok.
	if got, _ := checkStoreIntegrity(dir, ik, 0); got != IntegrityOK {
		t.Errorf("matching journal head got %v, want ok", got)
	}
	// Tamper the store; no sidecar; journal head no longer matches -> change.
	if err := os.WriteFile(StorePath(dir), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := checkStoreIntegrity(dir, ik, 0); got != IntegrityExternalChange {
		t.Errorf("tampered + no sidecar got %v, want external-change", got)
	}
}

func TestQuarantineDedupeAndExists(t *testing.T) {
	dir, _ := integHome(t, 1)
	p1, err := quarantineStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p1); err != nil {
		t.Fatalf("quarantine snapshot missing: %v", err)
	}
	p2, err := quarantineStore(dir) // identical content -> same path
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 {
		t.Errorf("dedupe failed: %q vs %q", p1, p2)
	}
	if filepath.Dir(p1) != filepath.Dir(StorePath(dir)) {
		t.Errorf("quarantine not beside the store: %q", p1)
	}
}

func TestAnchorReadAdvance(t *testing.T) {
	t.Setenv(EnvSuffix, "")
	kr := &envelopeFileKeyring{folder: t.TempDir()}
	if got, _ := readAnchor(kr); got != 0 {
		t.Errorf("initial anchor = %d, want 0", got)
	}
	if err := advanceAnchor(kr, 5); err != nil {
		t.Fatal(err)
	}
	if got, _ := readAnchor(kr); got != 5 {
		t.Errorf("anchor = %d, want 5", got)
	}
	// Lower value never lowers the anchor.
	if err := advanceAnchor(kr, 3); err != nil {
		t.Fatal(err)
	}
	if got, _ := readAnchor(kr); got != 5 {
		t.Errorf("anchor regressed to %d, want 5", got)
	}
}
