// vetcopies: the vet tool behind the no-copy ratchet (design/VALUE-NO-COPY.0.md,
// phase 0). It runs two analyzers under `go vet -vettool`:
//
//   - copylocks, go vet's own: core.Value carries a zero-size noCopy marker
//     with Lock and Unlock methods, so the analyzer reports every by-value
//     copy of a single Value — each argument, parameter, receiver,
//     assignment, return, range variable and composite literal.
//   - bulkcopy (bulkcopy.go): the element copies of a []core.Value that
//     copylocks cannot see, through the built-in copy and an append whose
//     source is spread with "...".
//   - fnvaralias (fnvaralias.go): a function variable aliasing a function
//     that takes a *core.Value. Calls through it are indirect, so every
//     &v handed to them escapes to the heap; the ratchet allows none.
//
// scripts/vet-copies.sh builds it, runs it in every go.work module, and holds
// each module's non-test counts to the ceilings in ceilings.tsv.
package main

import (
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/unitchecker"
)

func main() { unitchecker.Main(copylock.Analyzer, BulkCopy, FnVarAlias) }
