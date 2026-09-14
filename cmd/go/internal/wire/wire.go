// Package wire holds every identifier this project writes into a place it
// does not own: magic bytes inside files on disk, and namespaces inside the
// host operating system's credential stores.
//
// # The rule
//
// NOTHING HERE MAY BE DERIVED FROM THE PRODUCT NAME.
//
// The product name is a codename and will change again. These identifiers
// cannot: they are already inside files and keychains on machines this
// project will never see. Renaming one does not "rename" that data — it
// makes it unreadable, and the failure surfaces as a correct passphrase
// being rejected, or a built executable no longer recognising itself.
// That has already happened twice (AQL -> BORU broke the keyring magic,
// the export magic, and the executable trailer at once); brand-free
// constants are how it stops happening.
//
// To change the product name, edit package brand. Nothing in this file
// should need to move.
//
// # Adding a format
//
// Bump the format byte that follows the magic — do not mint a new magic.
// A magic identifies the container; the format byte versions its layout.
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
	// follows it; the trailing digit is part of the fixed magic, not a
	// version to bump.
	KeyringMagic = "VLTK1"

	// ExportMagic prefixes a portable, passphrase-encrypted export bundle.
	ExportMagic = "VLTX1"

	// ExecMagic marks the trailer of a self-embedding executable. Unlike
	// the others it is matched at the END of a file, and it carries its own
	// trailing version byte rather than a separate format byte.
	ExecMagic = "VLTEXEC\x01"

	// KeychainService is the namespace for entries in the host credential
	// store (macOS Keychain, libsecret, Windows Credential Manager). It is
	// a shared, OS-global namespace, so it is deliberately more specific
	// than the other identifiers to avoid colliding with another vendor.
	KeychainService = "vlt.secrets"
)

// The legacy sets: identifiers previous releases wrote, still accepted on
// READ so their data keeps opening. Newest first. Never write these.
var (
	keyringMagicLegacy    = []string{"BORUK", "AQLK"}
	exportMagicLegacy     = []string{"BORUX", "AQLX"}
	execMagicLegacy       = []string{"BORUEXEC\x01", "AQLEXEC\x01"}
	keychainServiceLegacy = []string{"boru", "aql"}
)

// KeyringMagics returns every keyring magic this binary can read, current
// first. Callers MUST take their field offsets from the length of the magic
// that actually matched: the entries differ in length ("VLTK1" is 5 bytes,
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
