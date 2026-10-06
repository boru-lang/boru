package core

// ApplyResidentBind — the runtime half of an ARM-RESIDENT twin (§6.5's
// each-body recovery; the placement half lives in the compiler's
// OpBindResident). Where ApplyBindTwin REPLAYS a captured entry once at
// its root-stream position, a resident bind executes INSIDE a compiled
// per-invocation unit, once per invocation, with the RUNTIME value —
// because a multi-run body's ledger entry is one generalized
// carrier-valued capture that cannot represent N per-element installs
// (measured: `[10 20] each [var [[r] def x r x]]` leaks x = [20, 10]
// top-down where the ledger holds one dynamic carrier).
//
// The install arm goes through InstallDef — the interpreter's OWN
// installer — so per-element repeats stack exactly as the interpreter's
// leak does (a plain value pushes a fresh level each iteration; a
// Function-valued body runs the same overlap filter). The undef arm
// mirrors ApplyBindTwin's BindUndef: pop whatever is live, retire a
// minted node only when this binding minted it (a var param never does,
// but the arms must not drift). Neither arm records on any unwind trail
// — leak persistence is the semantics; a mid-iteration raise leaves
// earlier elements' installs in place, interpreter-identical.
func ApplyResidentBind(r *Registry, name string, undef bool, v Value) {
	if r == nil {
		return
	}
	if undef {
		PopLiveBinding(r, name)
		return
	}
	InstallDef(r, name, v)
}

// ApplyResidentAssign is the var ASSIGNMENT arm of the same op (a
// BindDefReplace twin, the var word's `var NAME v` inside the body): the
// name's cell is replaced in place with the per-element runtime value, as
// the interpreter's AssignVar replaces it — the depth is unchanged, and
// the table's generation bump invalidates every read's cached plan. A
// missing binding (the cell was torn down by a frame the body ran in)
// installs instead, as the interpreter's var would declare.
func ApplyResidentAssign(r *Registry, name string, v Value) {
	if r == nil {
		return
	}
	if _, isVar := IsVarBinding(r, name); !isVar {
		InstallVar(r, name, v, nil)
		return
	}
	r.Defs.Replace(name, v)
	noteRebind(r, name)
}

// ApplyResidentTypeBind is the TYPE arm of the same op. A type binding has
// no runtime value to install — the node was minted by the check pass — so
// the obvious move is to REPLAY the captured entry the way a top-level
// OpBindTwin does. That is wrong, and the cross-request parity oracle
// measured it: replaying one node twice puts the SAME *Type in two def-stack
// levels, both flagged Minted, so the first `undef` retires the node out of
// the type table and the level BELOW it becomes unresolvable ("unresolvable
// type operand Big") where the interpreter still answers. The interpreter
// mints a distinct node per element, and that distinctness is observable.
//
// So the arm re-runs the interpreter's OWN installer with the captured BODY,
// exactly as the install arm re-runs InstallDef: the type EXPRESSION is
// evaluated once, at compile time, which is sound because the bridge proved
// it element-independent (typeInstallElementIndependent); the NODE is minted
// per element, which is what makes the def stack — and every retirement that
// walks it — interpreter-identical.
//
// It enters at InstallTypeBody rather than InstallType because the NAME was
// validated by the check pass for this program already, and the rollback
// that precedes the run restores bindings but not registered name parts —
// so the front door rejects the replay on the check pass's own leftovers.
// That doc states the measurement.
//
// An error can only mean a captured body the installer itself declines, which
// the check pass's run of the same body did not; it is returned rather than
// swallowed so the VM raises instead of installing nothing.
func ApplyResidentTypeBind(r *Registry, name string, entry DefEntry) error {
	if r == nil {
		return nil
	}
	return InstallTypeBody(r, name, entry.Body)
}
