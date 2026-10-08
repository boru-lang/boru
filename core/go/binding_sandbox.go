package core

// The NARROW binding sandbox — the rollback half of the bind-twin regime
// (design/FULL-COMPILATION.0.md §6.5, Stage 4).
//
// WHY A SECOND SANDBOX EXISTS. Today the compiled path deliberately KEEPS the
// check pass's RunInCheckMode installs and runs the Program against them
// (lang/go/boru.go), which is why `OpDispatchGeneric` cannot simply look a name
// up live: the registry already holds the FINAL binding for every name the
// front end walked, so a live lookup at instruction i would resolve what the
// source binds at the END rather than at i. The twin regime fixes that by
// rolling the runtime-visible binding transitions back and letting a twin op
// re-install each one at its own source position — replay, never re-execution:
// each twin re-installs the IDENTICAL binding object the check pass produced,
// so nothing runs twice and a module in particular is imported exactly once.
//
// THE EXISTING PRIMITIVE IS TOO COARSE TO REUSE, and that is a measured fact
// about SnapshotForCompile rather than a preference. It restores r.Types
// WHOLESALE, which would discard the minted type IDs the twins are specifically
// NOT supposed to replay: type minting, macro expansion and const folding are
// compile-time products, baked under the front-end carve-out, and a rollback
// that unmints them leaves OpPushType resolving IDs that no longer exist.
//
// BUT r.Types CANNOT BE EXCLUDED WHOLESALE EITHER, and this is the case the
// partition below exists for. A capitalised `undef` executing on the check pass
// RETIRES a lattice node (basic's definition words: PopEntry, then
// r.Types.Retire when this binding minted it). Roll r.Defs back alone and the
// type BINDING returns while its ID stays retired — a live binding pointing at
// a dead node, in a registry the VM has not yet reached the twin for. So the
// TypeTable is PARTITIONED rather than kept or restored:
//
//	minted IDs    RETAINED  — compile-time product, baked
//	retirements   ROLLED BACK — a runtime-visible transition, replayed by a twin
//
// Concretely, restore re-admits every ID the snapshot held that the live table
// has since lost, and touches nothing else. The result is the UNION of the two
// id sets, which is exactly "mints kept, retirements undone".
//
// WHAT IS DELIBERATELY OUT OF SCOPE. r.Check (the analysis state is the front
// end's own and has no runtime twin), the context stack (ctx-set/ctx-get have
// no twin op yet — §6.5 does not specify one, and silently rolling them back
// would change today's behaviour without a replay to restore it), capabilities,
// the builtin-word set, FlowCtrl and pendingGen. Each is either a compile-time
// product or has no twin to re-apply it; capturing it here would roll back
// state nothing would put back.
//
// TWO CLIENTS, in arrival order. The FIRST is the corpus-scale harness
// (test/go/langspec/bind_replay_sandbox_test.go): snapshot → compile pass →
// RestoreBindings → replay the pass's ledger — proven to land exactly the
// registry the pass left (depths per transition, entry identity per name)
// over every push-only corpus row, before any VM semantics changed. The
// SECOND is the runtime regime itself — since the flip, the ONLY runtime
// regime: lang's compiled entry points snapshot before the check pass and
// roll back through RestoreBindingsForReplay below at the one safe point — BETWEEN the pass and the run, where no enclosing
// pass holds references into the swapped table — before the placed
// OpBindTwin ops (core.ApplyBindTwin) replay the transitions in stream
// order.

// BindingSandbox captures the RUNTIME-VISIBLE binding state a check pass can
// move: the def table, the module ledger, and enough of the type table to undo
// a retirement without undoing a mint. Read-only infrastructure and every
// compile-time product are shared and not captured — see the partition above.
type BindingSandbox struct {
	defs    *DefTable
	typeIDs map[string]*Type
	// typeParts is the type-part reservation set at capture (TypePartsSnapshot):
	// a reservation the pass made for a binding the restore rolls back comes
	// off with the binding (ForgetTypePartsSince), and the binding's twin
	// reserves it again at its own stream position (applyTwinPush) — the
	// part is a runtime-visible transition exactly as the binding is, and a
	// reservation left standing from the pass made a RUN-time mint of the
	// same name, ahead of the twin's position, the conflicting one (NUR167's
	// root-def sibling: `def f fn [[n:Integer] [Integer] [def T (class {})
	// n]]  each f/v [1]  def T (class {})` raised at each's element 0 for
	// the interpreter's raise at the root def). The minted NODE is still
	// retained (the partition above); only its name comes free.
	typeParts map[string]bool
	modLoaded map[string]ModuleDesc
	modSeq    int
	// valid distinguishes a real capture from the zero value, which must never
	// read as an empty-but-usable snapshot (eng/go/CLAUDE.md, "No Zero-Value
	// Overload"): restoring a zero BindingSandbox would install a nil DefTable.
	valid bool
}

// SnapshotBindings captures the registry's runtime-visible bindings for a later
// RestoreBindings. Cheap relative to SnapshotForCompile: no CheckState clone, no
// context-stack walk, and the type table contributes an id map rather than a
// deep clone.
func (r *Registry) SnapshotBindings() BindingSandbox {
	if r == nil {
		return BindingSandbox{}
	}
	s := BindingSandbox{
		defs:      r.Defs.Clone(),
		typeIDs:   r.Types.idSnapshot(),
		typeParts: r.TypePartsSnapshot(),
		valid:     true,
	}
	if r.Modules != nil {
		s.modSeq = r.Modules.seq
		s.modLoaded = make(map[string]ModuleDesc, len(r.Modules.Loaded))
		for k, v := range r.Modules.Loaded {
			s.modLoaded[k] = v
		}
	}
	return s
}

// RestoreBindings rolls the runtime-visible bindings back to a
// SnapshotBindings capture, leaving every compile-time product in place.
//
// The dispatch-cache reset is part of the regime, not an artifact of the wide
// snapshot it was first written for: DefTable.Clone continues the parent's
// generation timeline, but the twins will re-install bindings under names the
// cache may already hold, so an entry cached before the rollback could be
// served for a name whose restored binding differs. Drop them all and let the
// aggregates rebuild.
func (r *Registry) RestoreBindings(s BindingSandbox) {
	if r == nil || !s.valid {
		return
	}
	r.Defs = s.defs
	r.dispatchCache.reset()
	r.Types.readmitRetired(s.typeIDs)
	r.ForgetTypePartsSince(s.typeParts)
	if r.Modules != nil {
		r.Modules.seq = s.modSeq
		r.Modules.Loaded = s.modLoaded
	}
}

// RestoreBindingsForReplay is the twin regime's runtime rollback: identical
// to RestoreBindings EXCEPT that the module ledger stays PASS-FINAL. The
// full restore exists for abandonment — a declined compile whose interpreter
// fallback re-runs the source, imports included, so the ledger must forget
// them. Here nothing is abandoned: the run that follows replays each module
// namespace BINDING through its def twin (the identical namespace instance
// — §6.5's imported-exactly-once), while the import itself already ran on
// the check pass and must never run again, on this request or the next one
// on the same instance. Rolling the ledger back would claim otherwise and
// re-import (re-running module-body effects) on the next request. The type
// partition and the cache reset are unchanged: mints retained, retirements
// readmitted for the undef twins to re-apply, cached dispatch dropped.
//
// It restores from a CLONE, so the snapshot stays pristine: the snapshot is
// a Program's rollback base (compiler.Program.ReplayBase, restored by
// RunProgram), a program may run more than once, and every run must roll
// back to the SAME base — not to the table the previous run mutated after
// the restore installed it live.
func (r *Registry) RestoreBindingsForReplay(s BindingSandbox) {
	if r == nil || !s.valid {
		return
	}
	r.Defs = s.defs.Clone()
	r.dispatchCache.reset()
	r.Types.readmitRetired(s.typeIDs)
	r.ForgetTypePartsSince(s.typeParts)
}

// SwapDefs installs dt as the registry's live binding table and returns the
// table it replaced, resetting the dispatch cache exactly as RestoreBindings
// does — the cache is keyed by per-name generation, and a table swapped in
// from a clone shares the parent's generation timeline, so an entry cached
// against the old table could otherwise be served for a name whose binding
// the new table holds differently.
//
// It exists for the compile pass's body re-run (Stage 4b): a leaking body —
// a `do` body, a higher-order body the runtime re-runs per element — is
// analysed once with recording suspended and then RE-RUN to compile, and the
// re-run must begin from the binding state the interpreter's single run
// begins from, not from the state the analysis run leaked. The recorder
// clones the table at the guard's open, swaps the clone in around each
// compile, and swaps the leaked table back afterwards so the enclosing
// analysis continues where the interpreter's run would have left it. A nil
// table is declined: the registry always has a live table.
func (r *Registry) SwapDefs(dt *DefTable) *DefTable {
	if r == nil || dt == nil {
		return nil
	}
	prev := r.Defs
	r.Defs = dt
	r.dispatchCache.reset()
	return prev
}
