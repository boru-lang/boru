// Package wire pins the keyring, export-bundle and embedded-executable
// markers and OS credential-store namespaces independently of product naming.
//
// # The rule
//
// SHIPPED IDENTIFIERS NEVER FOLLOW A PRODUCT RENAME.
//
// The product name is a codename and will change again. These identifiers
// cannot: they are already inside files and keychains on machines this
// project will never see. Renaming one does not "rename" that data — it
// makes it unreadable, and the failure surfaces as a correct passphrase
// being rejected, or a built executable no longer recognising itself.
// The AQL -> BORU rename changed these identifiers. Readers accept both
// spellings, while writers retain the shipped BORU spelling so existing
// BORU binaries can still read data produced by this version. A historical
// product name inside a frozen literal is not a reason to change its bytes.
//
// Cosmetic text is still distributed throughout the repository. See
// scripts/brand-audit.sh and design/WIRE-IDENTITY.0.md for the inventory.
//
// # Adding a format
//
// A magic identifies the container; the format byte versions its layout.
// Any layout change needs an explicit compatibility plan, even if its
// marker remains unchanged.
//
// # Retiring an identifier
//
// Never delete a legacy entry. Each one is the only thing keeping data
// written by an older binary readable. The lists only grow, and they grow
// only when someone renames something they should not have.
//
// Every constant here is pinned byte-for-byte by golden fixtures in
// testdata/. Those fixtures are binary on purpose: a find-and-replace over
// the repository can rewrite a Go constant and its assertion in the same
// sweep, leaving the suite green while every file on disk becomes
// unreadable. It cannot rewrite the fixtures.
package wire

const (
	// KeyringMagic prefixes the encrypted keyring blob. A format byte
	// follows it. Retain the shipped spelling for older BORU readers.
	KeyringMagic = "BORUK"

	// ExportMagic prefixes a portable, passphrase-encrypted export bundle.
	ExportMagic = "BORUX"

	// ExecMagic marks the trailer of a self-embedding executable. Unlike
	// the others it is matched at the END of a file, and it carries its own
	// trailing version byte rather than a separate format byte.
	ExecMagic = "BORUEXEC\x01"

	// KeychainService is the namespace for entries in the host credential
	// store (macOS Keychain, libsecret, Windows Credential Manager). It is
	// a shared namespace: keep the shipped value so old and new binaries
	// update the same credential rather than shadowing each other's writes.
	KeychainService = "boru"
)

// Additional read identifiers. AQL spellings came from earlier releases;
// VLT spellings were proposed in development and remain readable for any
// development artifacts. Shipped namespaces take priority. Never write these.
var (
	keyringMagicLegacy    = []string{"AQLK", "VLTK1"}
	exportMagicLegacy     = []string{"AQLX", "VLTX1"}
	execMagicLegacy       = []string{"AQLEXEC\x01", "VLTEXEC\x01"}
	keychainServiceLegacy = []string{"aql", "vlt.secrets"}
)

// KeyringMagics returns every keyring magic this binary can read, current
// first. Callers MUST take their field offsets from the length of the magic
// that actually matched: the entries differ in length ("BORUK" is 5 bytes,
// "AQLK" is 4), and assuming len(KeyringMagic) is precisely the bug this
// package exists to prevent.
func KeyringMagics() []string { return prepend(KeyringMagic, keyringMagicLegacy) }

// ExportMagics returns every export-bundle magic this binary can read.
func ExportMagics() []string { return prepend(ExportMagic, exportMagicLegacy) }

// ExecMagics returns every executable-trailer magic this binary can read.
func ExecMagics() []string { return prepend(ExecMagic, execMagicLegacy) }

// KeychainServices returns every credential-store namespace to look in,
// current first. Writes always go to KeychainService.
func KeychainServices() []string { return prepend(KeychainService, keychainServiceLegacy) }

// prepend returns current followed by legacy, as a fresh slice so a caller
// cannot mutate the package's tables.
func prepend(current string, legacy []string) []string {
	out := make([]string, 0, len(legacy)+1)
	out = append(out, current)
	return append(out, legacy...)
}

// MatchPrefix reports the length of the first id in ids that prefixes data,
// requiring at least extra further bytes after it (1 where a format byte
// must follow, 0 where the magic alone is enough to classify the file).
// Negative extra values do not match.
// The returned length — never len(ids[0]) — is what callers must use to
// locate everything that follows.
func MatchPrefix(data []byte, ids []string, extra int) (int, bool) {
	if extra < 0 || extra > len(data) {
		return 0, false
	}
	for _, id := range ids {
		if len(id) <= len(data)-extra && string(data[:len(id)]) == id {
			return len(id), true
		}
	}
	return 0, false
}

// MatchSuffix reports the length of the first id in ids that appears at
// off bytes from the end of data (the executable trailer sits before a
// fixed-width length field, not flush with the end). As with MatchPrefix,
// the matched length is authoritative for the caller's arithmetic.
// Offsets outside data do not match.
func MatchSuffix(data []byte, ids []string, off int) (int, bool) {
	if off < 0 || off > len(data) {
		return 0, false
	}
	for _, id := range ids {
		end := len(data) - off
		start := end - len(id)
		if start >= 0 && string(data[start:end]) == id {
			return len(id), true
		}
	}
	return 0, false
}
