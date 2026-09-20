package core

// capCheckFnCarrierBinds is the per-check-pass side table of names def-bound
// to a Function-family CARRIER (a computed fn the analysis cannot see).
// installDef deliberately installs no Defs binding for those (the compiled
// closure machinery owns the name — see the fn arm in core_helpers.go), so
// readers resolve the name here instead: the parse/mini/emit value-form
// macros (lang), and the engine's compile-pass undefined-word branches
// (stepWord / stepWordVal), which substitute the carrier where a plain pass
// would report undefined_word. Reset at the start of every check pass
// (ResetCheckFnCarrierBinds) — like the module-export growth ledger.
const capCheckFnCarrierBinds = "engine.check.fn-carrier-binds"

// NoteCheckFnCarrierBind records name → carrier in the per-pass table.
func NoteCheckFnCarrierBind(r *Registry, name string, v Value) {
	if m, ok, _ := Cap[map[string]Value](r, capCheckFnCarrierBinds); ok && m != nil {
		m[name] = v
		return
	}
	_ = r.Capabilities.Set(capCheckFnCarrierBinds, map[string]Value{name: v})
}

// CheckFnCarrierBind returns the fn carrier def-bound to name during this
// check pass, if any.
func CheckFnCarrierBind(r *Registry, name string) (Value, bool) {
	m, ok, _ := Cap[map[string]Value](r, capCheckFnCarrierBinds)
	if !ok || m == nil {
		return Value{}, false
	}
	v, hit := m[name]
	return v, hit
}

// CheckFnCarrierBoundName is CheckFnCarrierBind's reverse: the name this
// pass already bound to the carrier VALUE id, if any. The def site uses it
// to catch a DROPPED APPLY. `def f2 (f1 2)` over a curried factory binds
// f2 to the very carrier f1 denotes — the analysis could not model the
// apply, so it returned the callee unchanged — and the compiled program
// then binds both names to one slot and leaks the unconsumed argument
// into the residual (`2 fn (Integer) 3` for the interpreter's `6`). A
// bind whose value is already table-bound under ANOTHER name is exactly
// that shape, and nothing else: a legitimate alias cannot reach here
// (`def g f1` is a strict-barrier syntax error, and `def g f1/v` resolves
// through Defs without consulting this table).
func CheckFnCarrierBoundName(r *Registry, id string) (string, bool) {
	if id == "" {
		return "", false
	}
	m, ok, _ := Cap[map[string]Value](r, capCheckFnCarrierBinds)
	if !ok || m == nil {
		return "", false
	}
	for name, v := range m {
		if v.ID == id {
			return name, true
		}
	}
	return "", false
}

// DropCheckFnCarrierBind removes name from the fn-carrier side table — the
// `undef` half of NoteCheckFnCarrierBind. Without it the table outlives the
// binding it stands for, and the two stores disagree about what `undef` did:
// `installDef` DECLINES to bind a computed fn (the compiled closure
// machinery owns the name), so `undef` pops whatever Defs entry a previous
// `def` left, while the table still answers with the computed carrier. Both
// spellings then diverge —
//
//	def f (mk 1) ;  undef f ;  (f 2)              the table's stale carrier
//	def f 1 ;  def f (mk 1) ;  undef f ;  (f 2)   compiled `1 2`, interp 3
//
// — the second because the pop exposed the SHADOWED `f = 1` in Defs while
// the table kept the carrier. Dropping the entry makes the read miss, which
// raises the ordinary undefined_word diagnostic and declines the program to
// the interpreter, where `undef` of a fn binding is the interpreter's own
// business. Correct in both spellings, and correct whichever way that
// separate question is eventually settled.
func DropCheckFnCarrierBind(r *Registry, name string) {
	if m, ok, _ := Cap[map[string]Value](r, capCheckFnCarrierBinds); ok && m != nil {
		delete(m, name)
	}
}

// ResetCheckFnCarrierBinds clears the fn-carrier side table so it is scoped
// to a single check pass (a reused instance must not resolve a stale name).
// Called at the start of every check pass alongside ResetModuleExportGrowth.
func ResetCheckFnCarrierBinds(r *Registry) {
	if r == nil || r.Capabilities == nil {
		return
	}
	_, _ = r.Capabilities.Delete(capCheckFnCarrierBinds)
}
