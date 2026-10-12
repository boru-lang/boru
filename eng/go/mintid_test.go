package eng

import (
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestMintIDNoSiblingCollision pins the fix for the sibling-table ID
// collision: mintID used a strictly per-TypeTable counter, so the Nth
// mint in any two tables forked from the same parent (a module body's
// sub-registry and its importer, two sibling modules, concurrent
// forks) produced the SAME ID — and type identity (teq, the nominal
// `is` walk, dispatch) is ID-based, so a `refine Integer` minted in
// one inline module was teq-identical to a `refine String` minted in
// another. The counter is now shared per registry TREE (AdoptSeqFrom
// for module sub-registries, pointer-shared by CloneDynamic), while
// rollback clones COPY it — see mintID for the full contract.
// Language-level rows pin the same behaviour in
// lang/spec/module-instance.tsv §7.
func TestMintIDNoSiblingCollision(t *testing.T) {
	parent := core.NewDynamicTypeTable()

	// Two module-style sub-registries adopting the importing tree's
	// counter: their mints must be distinct from each other and from
	// the parent's own later mints.
	m1 := core.NewDynamicTypeTable()
	m1.AdoptSeqFrom(parent)
	m2 := core.NewDynamicTypeTable()
	m2.AdoptSeqFrom(parent)
	t1 := m1.MintType("Foo", core.TInteger)
	t2 := m2.MintType("Foo", core.TString)
	tp := parent.MintType("TopLevel", core.TInteger)
	if core.IDOf(t1) == core.IDOf(t2) {
		t.Errorf("sibling module tables minted colliding IDs: %s", core.IDOf(t1))
	}
	if core.IDOf(tp) == core.IDOf(t1) || core.IDOf(tp) == core.IDOf(t2) {
		t.Errorf("parent mint after module mints collided: %s", core.IDOf(tp))
	}

	// Concurrent-fork clones share the counter: same rule.
	cf1 := parent.CloneDynamic()
	cf2 := parent.CloneDynamic()
	f1 := cf1.MintType("Fork1", core.TInteger)
	f2 := cf2.MintType("Fork2", core.TInteger)
	if core.IDOf(f1) == core.IDOf(f2) {
		t.Errorf("sibling CloneDynamic() tables minted colliding IDs: %s", core.IDOf(f1))
	}
	if core.IDOf(f1) == core.IDOf(tp) || core.IDOf(f2) == core.IDOf(tp) {
		t.Errorf("fork mints collided with parent mint: %s / %s", core.IDOf(f1), core.IDOf(f2))
	}
}

// TestMintIDRollbackCloneCopies pins the OTHER half of the contract:
// Clone() (the rollback-sandbox snapshot) COPIES the counter rather
// than sharing it. Sandbox mints are discarded on restore, so the
// parent's later mints must reproduce the same IDs as if the sandbox
// never minted — this is what keeps a check-mode pass and a plain run
// of one program minting identical IDs (the type-soundness ratchet
// compares the two engines' types by identity).
func TestMintIDRollbackCloneCopies(t *testing.T) {
	parent := core.NewDynamicTypeTable()
	parent.MintType("Pre", core.TInteger)

	sandbox := parent.Clone()
	sbMint := sandbox.MintType("Discarded", core.TInteger)

	// The sandbox mint must not advance the parent's counter: the
	// parent's next mint takes the ordinal the sandbox used (the
	// sandbox and its mints are about to be thrown away).
	after := parent.MintType("After", core.TInteger)
	if core.IDOf(after) != core.IDOf(sbMint) {
		t.Errorf("rollback clone advanced the parent counter: parent minted %s, sandbox minted %s (must reuse the discarded ordinal)", core.IDOf(after), core.IDOf(sbMint))
	}
}

// TestMintIDConcurrentUnique asserts mints racing from concurrent
// forks (the await shape) all receive unique IDs — the shared per-tree
// counter must be atomic, not merely shared.
func TestMintIDConcurrentUnique(t *testing.T) {
	const perTable, tables = 50, 8
	parent := core.NewDynamicTypeTable()

	ids := make(chan string, perTable*tables)
	var wg sync.WaitGroup
	for i := 0; i < tables; i++ {
		fork := parent.CloneDynamic()
		wg.Add(1)
		go func(tt *core.TypeTable) {
			defer wg.Done()
			for j := 0; j < perTable; j++ {
				ids <- core.IDOf(tt.MintType("C", core.TInteger))
			}
		}(fork)
	}
	wg.Wait()
	close(ids)

	seen := make(map[string]bool, perTable*tables)
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate minted ID under concurrency: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != perTable*tables {
		t.Fatalf("expected %d unique IDs, got %d", perTable*tables, len(seen))
	}
}
