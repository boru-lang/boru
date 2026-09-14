// Package brand holds the product name, for display only.
//
// The name is a codename and is expected to change. Everything here is
// safe to change with it: help text, banners, editor-facing labels — text
// a human reads once and nothing ever parses back.
//
// Identifiers that OUTLIVE a rename — magic bytes in files, credential
// store namespaces — do not belong here. They live in package wire, which
// is deliberately brand-free. The test for which package a string belongs
// in: if an older release wrote it somewhere this binary must later read,
// it is wire; if it only ever reaches a human's eyes, it is brand.
package brand

const (
	// Name is the product name as written in prose and banners.
	Name = "boru"

	// CLI is the executable name as typed at a shell prompt.
	CLI = "boru"
)
