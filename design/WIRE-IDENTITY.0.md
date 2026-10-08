# Wire identity across product renames

**Status:** Design note for PR #452, not an accepted ADR · **Date:** 2026-09-14

This note covers `cmd/go/internal/wire`: keyring and export-bundle markers,
embedded-executable trailers, and OS credential-store namespaces.

## Preserve shipped bytes

The product name may change; identifiers already stored in files or host
credential stores must remain readable. The AQL to BORU rename changed the
keyring, export and executable markers, including their lengths. A reader
using the current marker's length to locate an older file's fields can
misread its salt, nonce or payload length. For encrypted data this can
report a correct passphrase as invalid.

Keep the shipped BORU identifiers as the write format: `BORUK`, `BORUX`,
`BORUEXEC\x01`, and the credential service `boru`. These values are frozen
protocol identities even though they contain a historical product name.
Do not derive them from a display name or change them during a rename.

The initial proposal used `VLTK1`, `VLTX1`, `VLTEXEC\x01`, and `vlt.secrets`
for new writes. Review rejected that migration: earlier BORU readers would
not recognize new files, and different credential services would let old
and new binaries shadow each other's updates. Those proposed spellings
remain readable for development artifacts, but are not written. Shipped
credential services take precedence over the development namespace.

Readers accept AQL and BORU markers and compute offsets from the marker
that actually matched. OS keychain reads check the current service first,
then the supported older/development services; backend errors stop the
search instead of masquerading as a missing secret. Deletes sweep all
services, treating missing entries as success and reporting real failures.

## Scope of a future rename

`scripts/brand-audit.sh` inventories occurrences; its counts are a starting
point for review, not an exhaustive classification or an automatic renamer.

- **Frozen:** bytes in stored formats and credential namespaces. Keep
  shipped values unchanged. A necessary format change requires an explicit
  compatibility plan for old readers, rollback and mixed-version writes.
- **Migratable:** paths, environment variables, source extensions, import
  namespaces and executable names. Define the compatibility window before
  changing them; readers and launchers may need both spellings.
- **Cosmetic:** human-readable help, banners and labels. These are still
  distributed across CLI code, language diagnostics/help, editor assets,
  web assets and documentation. There is no central brand package that
  controls these surfaces; search and review the actual occurrences.

This package is not yet an inventory of every persisted identifier in the
repository. Other formats, such as the per-secret envelope and 1Password
item naming, need their own compatibility audit before a product rename.

## Regression evidence

`cmd/go/internal/wire/testdata` contains fixed keyring, export and executable
fixtures for the supported markers. Tests read these bytes independently
of the encoders' current constants, so changing a constant and its text
assertion together does not silently rewrite the expected serialized data.
Never regenerate an existing fixture merely to make a changed marker pass.

The tests also pin the current write identifiers, malformed-marker
rejection, wrong-passphrase rejection, namespace lookup precedence and
deletion after a backend failure. A new layout or marker needs a new
fixture and a compatibility decision; it must not replace old evidence.
