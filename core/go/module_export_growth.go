package core

// Module export-map growth ledger (Phase 6 M3, minilang:320 —
// design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore §6 Stage M3).
//
// tryFoldModuleConst refuses to fold a MISSING-key get over a module export
// to None because a module's keyspace can GROW at run time: the DSL `register`
// family installs new exports (`lang_<kind>`, a minted member type, a
// `compile_<kind>` hook) AFTER the check pass folded the program, so a baked
// absence could go stale. That blanket decline is the sound default — but it
// also refuses provably-stable absences: `MiniLang.register gen <2-param fn>`
// installs ONLY `lang_gen` (a non-filter kind mints no member type), so
// `MiniLang.Gen` is None on every run, and the interpreter returns exactly
// that.
//
// The ledger makes the stable case provable without weakening the default:
//
//   - a module whose PROGRAM-REACHABLE runtime growers are all check-modelled
//     REGISTERS its export map here at build time (RegisterModuleExportGrowth)
//     — an unregistered map keeps the blanket decline forever;
//   - each grower's check-mode twin NOTES the keys its runtime execution may
//     install (NoteModuleExportAdd), or POISONS the ledger when the key set
//     is not statically decidable (a non-concrete kind name);
//   - the fold's absence check (moduleExportAbsenceStable) then admits the
//     None fold exactly when the receiver's ledger exists, is unpoisoned, and
//     the requested key is not among the noted adds.
//
// Notes are PER-PASS state: ResetModuleExportGrowth clears adds/poison (but
// keeps the registration — a static property of the module implementation)
// at every check/compile entry point. With the kind namespaces frozen the
// shipping registrants note NO adds (custom languages are lexically-bound
// fn values, never export-map keys), so registration alone makes every
// missing-key read provably stable; the note/poison machinery remains for
// any future module whose exports can grow mid-program.
//
// The ledger is keyed by the export map POINTER (the §4 discipline: aliasing
// is pointer-payload, not ID-graph — the StoreShapeInfo precedent). A bound
// module namespace IS a plain Map over the module's export *OrderedMap
// (carrying the module-namespace facet — NUR038 retired the wrapper type),
// so the kernel reads the pointer straight off the receiver with AsMap; no
// lang-layer payload type is involved.

// capModuleExportGrowth is the capability slot holding the per-registry
// growth table.
const capModuleExportGrowth = "engine.module.export-growth"

// exportGrowthLedger is one export map's per-pass growth record.
type exportGrowthLedger struct {
	poisoned bool
	adds     map[string]bool
}

// exportGrowthTable maps registered export maps to their ledgers.
type exportGrowthTable struct {
	byFields map[*OrderedMap]*exportGrowthLedger
}

func exportGrowthTableFor(r *Registry, create bool) *exportGrowthTable {
	if t, ok, _ := Cap[*exportGrowthTable](r, capModuleExportGrowth); ok && t != nil {
		return t
	}
	if !create || r == nil || r.Capabilities == nil {
		return nil
	}
	t := &exportGrowthTable{byFields: map[*OrderedMap]*exportGrowthLedger{}}
	_ = r.Capabilities.Set(capModuleExportGrowth, t)
	return t
}

// RegisterModuleExportGrowth marks fields as growth-MODELLED: every word this
// program can run that installs new keys into fields has a check-mode twin
// that calls NoteModuleExportAdd / PoisonModuleExportGrowth. Called once at
// module build time; registering is an implementation-level soundness claim,
// so only add a call for a module whose grower inventory is audited.
func RegisterModuleExportGrowth(r *Registry, fields *OrderedMap) {
	if fields == nil {
		return
	}
	t := exportGrowthTableFor(r, true)
	if t == nil {
		return
	}
	if _, ok := t.byFields[fields]; !ok {
		t.byFields[fields] = &exportGrowthLedger{adds: map[string]bool{}}
	}
}

// NoteModuleExportAdd records that a runtime grower analysed this pass MAY
// install keys into fields. Conservative direction only: a noted key makes
// the absence fold DECLINE for that key; it never enables a fold.
func NoteModuleExportAdd(r *Registry, fields *OrderedMap, keys ...string) {
	t := exportGrowthTableFor(r, false)
	if t == nil || fields == nil {
		return
	}
	l := t.byFields[fields]
	if l == nil {
		return
	}
	for _, k := range keys {
		l.adds[k] = true
	}
}

// PoisonModuleExportGrowth records that a grower with a statically
// undecidable key set was analysed: ANY key may appear at run time, so every
// absence fold on fields declines for the rest of the pass.
func PoisonModuleExportGrowth(r *Registry, fields *OrderedMap) {
	t := exportGrowthTableFor(r, false)
	if t == nil || fields == nil {
		return
	}
	if l := t.byFields[fields]; l != nil {
		l.poisoned = true
	}
}

// ResetModuleExportGrowth clears every ledger's per-pass notes (adds +
// poison) while keeping the registrations. Called at the start of each
// check/compile pass so a reused instance never carries a prior program's
// registration set (the ResetParseDeferredKinds discipline).
func ResetModuleExportGrowth(r *Registry) {
	t := exportGrowthTableFor(r, false)
	if t == nil {
		return
	}
	for _, l := range t.byFields {
		l.poisoned = false
		l.adds = map[string]bool{}
	}
}

// moduleExportAbsenceStable reports whether a MISSING-key get over a module
// namespace receiver is provably still missing at run time: the receiver's
// export map has a growth ledger (all program-reachable growers are
// check-modelled), the ledger is unpoisoned, and the requested key is not
// among the pass's noted adds. args are the dispatch operands (any order —
// the receiver is the facet-carrying namespace Map, the key the concrete
// Atom/String operand); an unrecognisable shape declines.
func ModuleExportAbsenceStable(r *Registry, args []Value) bool {
	var fields *OrderedMap
	key := ""
	haveKey := false
	for _, a := range args {
		if IsModuleFamilyValue(a) {
			if ModuleNSOf(a) != nil && fields == nil {
				if mp, ok := a.Data.(MapPayload); ok && mp.M != nil {
					fields = mp.M
					continue
				}
			}
			return false // a Module descriptor / second namespace — not modelled
		}
		if haveKey {
			return false // two key candidates — not the 2-operand get shape
		}
		if s, err := a.AsConcreteAtom(); err == nil {
			key, haveKey = s, true
			continue
		}
		if s, err := a.AsConcreteString(); err == nil {
			key, haveKey = s, true
			continue
		}
		return false
	}
	if fields == nil || !haveKey {
		return false
	}
	t := exportGrowthTableFor(r, false)
	if t == nil {
		return false
	}
	l := t.byFields[fields]
	return l != nil && !l.poisoned && !l.adds[key]
}
