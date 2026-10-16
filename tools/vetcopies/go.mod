// vetcopies — the vet tool behind the no-copy ratchet (design/VALUE-NO-COPY.0.md,
// phase 0; `make vet-copies`). Its OWN module, deliberately absent from the
// root Makefile's MODULES list like tools/piecetool: it is a developer tool,
// so its statements stay out of the repo-wide ADR-008 coverage universe.
module github.com/boru-lang/boru/tools/vetcopies

go 1.24.7

require golang.org/x/tools v0.38.0

require (
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
)
