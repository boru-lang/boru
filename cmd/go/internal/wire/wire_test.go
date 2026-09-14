package wire

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/brand"
)

// golden reads a checked-in fixture. These files are BINARY ON PURPOSE.
// The bug this package exists to prevent — a rename sweep that rewrites an
// identifier and its own assertion in one pass, leaving the suite green
// while shipped data becomes unreadable — is defeated only by an expectation
// that lives outside the Go source. Never regenerate a fixture to make a
// test pass; a failure means a shipped identifier moved.
func golden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestCurrentIdentifiersStillMatchShippedBytes is the anti-rename guard.
// Each fixture was written by a real encoder using the identifier named in
// its filename; if the corresponding constant is edited, the constant no
// longer prefixes its own fixture and this fails.
func TestCurrentIdentifiersStillMatchShippedBytes(t *testing.T) {
	cases := []struct {
		file, want string
	}{
		{"keyring.vltk1.golden", KeyringMagic},
		{"export.vltx1.golden", ExportMagic},
	}
	for _, c := range cases {
		data := golden(t, c.file)
		if got := string(data[:len(c.want)]); got != c.want {
			t.Errorf("%s starts with %q, but the current identifier is %q — "+
				"an identifier that is already on disk was changed", c.file, got, c.want)
		}
	}
	// The executable magic sits before the 8-byte length field, not at the
	// start of the file.
	img := golden(t, "exec.vltexec.golden")
	end := len(img) - 8
	if got := string(img[end-len(ExecMagic) : end]); got != ExecMagic {
		t.Errorf("exec fixture trailer is %q, want %q", got, ExecMagic)
	}
}

// TestEveryHistoricalIdentifierStillReadable pins the compatibility set: a
// legacy entry may never be deleted, because it is the only thing keeping
// data written by an older release readable.
func TestEveryHistoricalIdentifierStillReadable(t *testing.T) {
	keyrings := map[string]int{"keyring.vltk1.golden": 5, "keyring.boruk.golden": 5, "keyring.aqlk.golden": 4}
	for file, wantLen := range keyrings {
		n, ok := MatchPrefix(golden(t, file), KeyringMagics(), 1)
		if !ok {
			t.Errorf("%s no longer recognised — a legacy keyring magic was dropped", file)
			continue
		}
		if n != wantLen {
			t.Errorf("%s matched length %d, want %d — offsets would be wrong", file, n, wantLen)
		}
	}

	exports := map[string]int{"export.vltx1.golden": 5, "export.borux.golden": 5, "export.aqlx.golden": 4}
	for file, wantLen := range exports {
		n, ok := MatchPrefix(golden(t, file), ExportMagics(), 0)
		if !ok {
			t.Errorf("%s no longer recognised — a legacy export magic was dropped", file)
			continue
		}
		if n != wantLen {
			t.Errorf("%s matched length %d, want %d", file, n, wantLen)
		}
	}

	execs := map[string]int{"exec.vltexec.golden": 8, "exec.boruexec.golden": 9, "exec.aqlexec.golden": 8}
	for file, wantLen := range execs {
		n, ok := MatchSuffix(golden(t, file), ExecMagics(), 8)
		if !ok {
			t.Errorf("%s no longer recognised — a legacy exec magic was dropped", file)
			continue
		}
		if n != wantLen {
			t.Errorf("%s matched length %d, want %d", file, n, wantLen)
		}
	}
}

// TestIdentifiersAreBrandFree is the forward guard: it fails if a future
// rename leaks the new product name into this package. The check is on the
// SHAPE of the value, not on any particular name — it asserts the current
// identifiers are exactly the vetted, brand-free literals. Changing one is
// a deliberate act that must come with a migration, so it must not be
// possible by sweeping a name through the tree.
func TestIdentifiersAreBrandFree(t *testing.T) {
	frozen := map[string]string{
		"KeyringMagic":    "VLTK1",
		"ExportMagic":     "VLTX1",
		"ExecMagic":       "VLTEXEC\x01",
		"KeychainService": "vlt.secrets",
	}
	got := map[string]string{
		"KeyringMagic":    KeyringMagic,
		"ExportMagic":     ExportMagic,
		"ExecMagic":       ExecMagic,
		"KeychainService": KeychainService,
	}
	for name, want := range frozen {
		if got[name] != want {
			t.Errorf("%s = %q, want %q. These identifiers are ON DISK and in OS "+
				"credential stores on machines you cannot reach. Changing one "+
				"orphans that data. If this is deliberate, move the old value "+
				"into the legacy list, add a golden fixture for it, and update "+
				"this test in the same commit.", name, got[name], want)
		}
	}
}

func TestMatchPrefixRequiresTrailingBytes(t *testing.T) {
	// The magic is present but the required format byte is not.
	if _, ok := MatchPrefix([]byte("VLTK1"), KeyringMagics(), 1); ok {
		t.Error("a magic with no format byte after it must not match")
	}
	if n, ok := MatchPrefix([]byte("VLTK1"), ExportMagics(), 0); ok || n != 0 {
		t.Error("a keyring magic must not be read as an export bundle")
	}
	if _, ok := MatchPrefix([]byte("nope"), KeyringMagics(), 1); ok {
		t.Error("unrelated bytes must not match")
	}
}

func TestMatchSuffixBounds(t *testing.T) {
	if _, ok := MatchSuffix([]byte("tiny"), ExecMagics(), 8); ok {
		t.Error("an image shorter than the trailer must not match")
	}
	img := append([]byte("VLTEXEC\x01"), make([]byte, 8)...)
	if n, ok := MatchSuffix(img, ExecMagics(), 8); !ok || n != 8 {
		t.Errorf("MatchSuffix = %d, %v; want 8, true", n, ok)
	}
}

func TestMatchersRejectInvalidOffsets(t *testing.T) {
	data := []byte("VLTK1")
	for _, offset := range []int{-1, len(data) + 1, int(^uint(0) >> 1)} {
		if n, ok := MatchPrefix(data, KeyringMagics(), offset); n != 0 || ok {
			t.Errorf("MatchPrefix with extra %d = %d, %v", offset, n, ok)
		}
		if n, ok := MatchSuffix(data, KeyringMagics(), offset); n != 0 || ok {
			t.Errorf("MatchSuffix with offset %d = %d, %v", offset, n, ok)
		}
	}
	if n, ok := MatchPrefix(data, KeyringMagics(), 0); n != len(data) || !ok {
		t.Errorf("MatchPrefix exact marker = %d, %v", n, ok)
	}
	if n, ok := MatchSuffix(data, KeyringMagics(), 0); n != len(data) || !ok {
		t.Errorf("MatchSuffix exact marker = %d, %v", n, ok)
	}
}

// The read sets must start with the identifier that is WRITTEN, and the
// tables must not be mutable through the accessor.
func TestReadSetsStartWithCurrentAndAreCopies(t *testing.T) {
	for name, pair := range map[string][2]interface{}{
		"keyring":  {KeyringMagics(), KeyringMagic},
		"export":   {ExportMagics(), ExportMagic},
		"exec":     {ExecMagics(), ExecMagic},
		"keychain": {KeychainServices(), KeychainService},
	} {
		set := pair[0].([]string)
		cur := pair[1].(string)
		if len(set) < 2 {
			t.Errorf("%s read set has no legacy entries", name)
		}
		if set[0] != cur {
			t.Errorf("%s read set starts with %q, want the written identifier %q", name, set[0], cur)
		}
	}
	a := KeyringMagics()
	a[0] = "MUTATED"
	if KeyringMagics()[0] != KeyringMagic {
		t.Error("callers must not be able to mutate the package's tables")
	}
}

// TestIdentifiersDoNotContainAnyProductName is the gate that survives the
// NEXT rename. TestIdentifiersAreBrandFree pins today's literals; this one
// states the underlying rule, so it keeps working when the product is
// called something nobody has thought of yet: whatever these identifiers
// are, they must not be spelled after the product.
//
// It reads the current name from package brand, so renaming the product and
// re-spelling a magic to match — the exact move that broke AQL -> BORU —
// fails here without anyone having to remember this file exists.
func TestIdentifiersDoNotContainAnyProductName(t *testing.T) {
	names := []string{brand.Name, brand.CLI, "boru", "aql"} // current + every retired codename

	ids := map[string]string{
		"KeyringMagic":    KeyringMagic,
		"ExportMagic":     ExportMagic,
		"ExecMagic":       ExecMagic,
		"KeychainService": KeychainService,
	}
	for id, val := range ids {
		low := strings.ToLower(val)
		for _, n := range names {
			if n == "" {
				continue
			}
			if strings.Contains(low, strings.ToLower(n)) {
				t.Errorf("%s = %q contains the product name %q.\n"+
					"Identifiers in this package are written into files and OS "+
					"credential stores that outlive the name. Spelling one after "+
					"the product guarantees the next rename orphans that data — "+
					"it has already happened twice. Pick a name-free identifier.",
					id, val, n)
			}
		}
	}
}
