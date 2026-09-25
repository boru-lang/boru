package langspec

import "testing"

// NUR092: a ledger entry the default-breadth sample no longer exercises is
// not stale while the rest of the corpus still does — the verdict consults
// the full breadth before instructing the author to delete the entry, and
// consults it only when the sample came up empty.
func TestLedgerStaleConsultsFullBreadth(t *testing.T) {
	t.Parallel()
	consulted := 0
	rest := func(n int) func() int { return func() int { consulted++; return n } }
	if ledgerStale(0, rest(3)) {
		t.Fatal("a bucket live in the unsampled rest of the corpus was called stale")
	}
	if !ledgerStale(0, rest(0)) {
		t.Fatal("a bucket empty at full breadth was not called stale")
	}
	before := consulted
	if ledgerStale(2, rest(0)) {
		t.Fatal("a bucket observed in the sample was called stale")
	}
	if consulted != before {
		t.Fatal("the rest of the corpus was swept for a bucket the sample already observed")
	}
}
