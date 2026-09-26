package core

// ApplyBindTwin — the runtime half of §6.5's rollback-and-replay regime
// (design/FULL-COMPILATION.0.md; the rollback half is binding_sandbox.go).
//
// One OpBindTwin executes here: re-perform ONE recorded bind-ledger
// transition against the rolled-back registry, at the twin's own stream
// position. Replay, never re-execution — a push kind re-installs the
// IDENTICAL binding object the check pass captured at its note (the same
// FnDefInfo, the same module namespace instance, the same minted node),
// so nothing runs twice; the removal kinds re-remove against whatever the
// replay has built so far, which — by the corpus-proven sandbox contract
// (test/go/langspec/bind_replay_sandbox_test.go) — is exactly the stack
// the check pass saw at that moment.
//
// THE WRITE-BACK PAIRING is the one deliberate divergence from a verbatim
// replay, and it is a PAIRING, not an omission. A top-level def whose kept
// binding is not the runtime value — a carrier, and since the sixty-third
// increment any computed compound (rootBindWritesBack, compiler/go/lower.go)
// — also emitted an OpBindGlobal, which under the regime runs in Push mode
// (GlobalBindSpec.Push) and installs the RUNTIME value where the
// interpreter's `def` would. Twin-then-bind is the stream order (InstallDef
// notes before RecordDynBind stamps), so the skip leaves the push to the op
// that has the real value — replaying the capture AND pushing the runtime
// value would double-install (measured in review of #459: `def b [add 1 2]`
// left two levels, and `undef b` then resolved `b` to the replayed
// `[Integer]`), and replaying the capture alone would resurrect the very
// keep-the-installs staleness the flip exists to remove. The pairing is
// carried on the twin itself (BindTransition.WrittenBack, set by the
// lowering that emitted the write-back), so the compiler's ONE decision
// drives both sides where a write-back exists; a predicate re-derived here
// from the captured entry's shape was how the two drifted apart.
//
// THE CARRIER SKIP stands beside it, and it is a different fact: a captured
// CARRIER is not a value at all — the check pass's placeholder for one —
// and it is never what the run binds, whether the real install is a
// write-back, a BIND_DYN_SCOPE inside a loop body (`for 2 [ f  def k 9 ]`,
// whose body twin captures a carrier and is placed before the loop), or
// nothing. Replaying it would bind the placeholder; the arm that binds the
// real value is elsewhere by construction.
func ApplyBindTwin(r *Registry, tr BindTransition, entry DefEntry) {
	if r == nil {
		return
	}
	switch tr.Kind {
	case BindUndef:
		// The twin pops whatever is live at ITS position — the capture is
		// deliberately zero (check_state.go). Retirement mirrors basic's
		// undef: only a node THIS binding minted; an adopted alias node
		// stays in the lattice.
		PopLiveBinding(r, tr.Name)
	case BindSigUndef:
		applyTwinSigUndef(r, tr.Name, entry.Body)
	case BindDefReplace:
		// InstallDef's same-scope overlap filter: drop the standing entry,
		// then install the replacement — net zero, exactly the delta the
		// ledger records for this kind. The push half pairs with the def's
		// own Push-mode OpBindGlobal via the carrier-class skip above, so
		// a computed replacement still nets zero: twin pops, bind pushes.
		r.Defs.PopEntry(tr.Name)
		applyTwinPush(r, tr, entry)
	default: // BindDef, BindTypeInstall
		applyTwinPush(r, tr, entry)
	}
}

// applyTwinPush re-installs one captured push entry, honouring the
// write-back pairing documented on ApplyBindTwin. The three install arms
// are the sandbox harness's proven replay verbatim: a plain value pushes,
// a minted type binding re-pushes its node (the mint itself was retained
// through the rollback — a compile-time product), an adopted alias
// re-adopts the canonical node.
func applyTwinPush(r *Registry, tr BindTransition, entry DefEntry) {
	if entry.TypeDef == nil && (tr.WrittenBack || (!IsConcrete(entry.Body) && !IsBareTypeNode(entry.Body))) {
		return
	}
	switch {
	case entry.TypeDef == nil:
		r.Defs.Push(tr.Name, entry.Body)
	case entry.Minted:
		// The node must be LIVE in the ID index again, not only bound. The
		// mint normally survives the rollback (readmitRetired leaves mints
		// in place), but when the check pass ALSO retired it — `def Point
		// class {…} … undef Point` — the rollback's snapshot predates the
		// mint, so nothing re-admits it, and every OpPushType between this
		// twin and the undef twin met an unresolvable type operand where the
		// interpreter's `make Point` resolved it (class.tsv L98–L101,
		// 2026-09-26). Adopt is idempotent and keeps the canonical pointer;
		// the undef twin retires it again at its own position.
		r.Types.Adopt(entry.TypeDef)
		r.Defs.PushType(tr.Name, entry.TypeDef, entry.Body)
	default:
		r.Defs.PushTypeAdopted(tr.Name, entry.TypeDef, entry.Body)
	}
}

// applyTwinSigUndef re-removes the captured entry a check-time signature
// undef took out (UninstallFnSigs — possibly MID-stack, which is why the
// note carries the removed entry rather than letting the twin pop the
// top). The removed value is located by identity, most-recent first: the
// carrier ID when the capture has one, else value equality (a fn value
// constructed outside the recorder carries no ID). A twin that cannot
// find its entry removes NOTHING — a twin never guesses by position.
func applyTwinSigUndef(r *Registry, name string, removed Value) {
	stack := r.Defs.Stack(name)
	for j := len(stack) - 1; j >= 0; j-- {
		if stack[j].ID != removed.ID {
			continue
		}
		if removed.ID == "" && !ValuesEqual(stack[j], removed) {
			continue
		}
		rest := append([]Value(nil), stack[:j]...)
		r.Defs.Set(name, append(rest, stack[j+1:]...))
		return
	}
}
