// piecetool — the four-piece split's inventory/rename/facade generator
// (design/legacy/ENG-FOUR-PIECE.0.ignore). Its OWN module, deliberately absent from
// the root Makefile's MODULES list: it is a developer tool, so its
// statements stay out of the repo-wide ADR-008 coverage universe that
// every shipped module must satisfy. Build it with `make facades`.
module github.com/boru-lang/boru/tools/piecetool

go 1.24.7

require golang.org/x/tools v0.38.0

require (
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
)
