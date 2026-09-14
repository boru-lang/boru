package vault

// golden_formats_test.go — end-to-end proof that every keyring and export
// envelope this project has ever shipped still opens with the correct
// passphrase. The fixtures are the ones package wire pins; they live there
// as the single source of truth so a rename cannot be "fixed" by quietly
// regenerating a copy that only this package reads.

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

const goldenPass = "golden-fixture-passphrase"

func wireGolden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "wire", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestEveryShippedKeyringStillDecrypts is the regression that the AQL->BORU
// rename needed and did not have. Each fixture is a real keyring blob
// written under one of the magics this project has shipped; all of them must
// open, because on somebody's disk they still exist.
func TestEveryShippedKeyringStillDecrypts(t *testing.T) {
	want := []byte("golden\t" + base64.StdEncoding.EncodeToString([]byte("golden-secret")) + "\n")
	for _, f := range []string{"keyring.vltk1.golden", "keyring.boruk.golden", "keyring.aqlk.golden"} {
		got, err := decryptBlob(wireGolden(t, f), goldenPass)
		if err != nil {
			t.Errorf("%s rejected the correct passphrase: %s", f, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s decrypted to %q, want %q", f, got, want)
		}
		// A wrong passphrase must still be refused on every vintage.
		if _, err := decryptBlob(wireGolden(t, f), "wrong"); err == nil {
			t.Errorf("%s accepted a wrong passphrase", f)
		}
	}
}

// TestEveryShippedExportBundleStillOpens is the same contract for bundles,
// which are worse to lose: they are the backups people keep precisely so a
// vault can be rebuilt.
func TestEveryShippedExportBundleStillOpens(t *testing.T) {
	want := []byte(`{"version":2,"aliases":[]}`)
	for _, f := range []string{"export.vltx1.golden", "export.borux.golden", "export.aqlx.golden"} {
		blob := wireGolden(t, f)
		if !isExportBundle(blob) {
			t.Errorf("%s not recognised as an export bundle", f)
			continue
		}
		got, err := openExport(blob, goldenPass)
		if err != nil {
			t.Errorf("%s rejected the correct passphrase: %s", f, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s opened to %q, want %q", f, got, want)
		}
	}
}
