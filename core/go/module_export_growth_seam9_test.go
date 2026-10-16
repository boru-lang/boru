package core

import "testing"

// W9 module_export_growth.go coverage: the ledger register/note/poison guard
// arms and moduleExportAbsenceStable's operand-shape declines. All pure.

func TestW9ModuleExportGrowthGuards(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()

	// Note / Poison before any table exists → t==nil arms.
	NoteModuleExportAdd(r, fields, "k")
	PoisonModuleExportGrowth(r, fields)

	// Register guards: nil fields, and a nil registry (table creation fails).
	RegisterModuleExportGrowth(r, nil)
	RegisterModuleExportGrowth(nil, fields)

	// Register a DIFFERENT map so the table now exists, then Note/Poison an
	// UNregistered map → ledger==nil arms.
	other := NewOrderedMap()
	RegisterModuleExportGrowth(r, other)
	NoteModuleExportAdd(r, fields, "k")
	PoisonModuleExportGrowth(r, fields)

	// The unrelated (registered) map's absence is stable for an unnoted key.
	if !ModuleExportAbsenceStable(r, absenceArgsW9(other, "missing")) {
		t.Error("an unpoisoned, unnoted key should fold to a stable absence")
	}
	// After noting the key, the fold declines.
	NoteModuleExportAdd(r, other, "seen")
	if ModuleExportAbsenceStable(r, absenceArgsW9(other, "seen")) {
		t.Error("a noted key must decline the absence fold")
	}

	// Poisoning the registered ledger declines EVERY absence fold.
	PoisonModuleExportGrowth(r, other)
	if ModuleExportAbsenceStable(r, absenceArgsW9(other, "unnoted")) {
		t.Error("a poisoned ledger must decline every absence fold")
	}
}

// absenceArgsW9 builds the [namespaceValue, keyAtom] operand pair. The
// namespace is what `import` binds: a plain Map over the export fields,
// carrying the module-namespace facet.
func absenceArgsW9(fields *OrderedMap, key string) []Value {
	mod := WithModuleNS(NewMap(fields), "E", Value{})
	return []Value{mod, NewAtom(key)}
}

func TestW9ModuleExportAbsenceStableDeclines(t *testing.T) {
	r := newTestRegistry(t)

	// A Module DESCRIPTOR (not a namespace) → decline: only a namespace
	// map's fields are ledger-modelled.
	mt := &Type{tmeta: &typeMeta{Name: "Ideal/Module"}}
	opaque := MakeValue(mt, ExtensionPayload{Body: 42})
	if ModuleExportAbsenceStable(r, []Value{opaque, NewAtom("k")}) {
		t.Error("an opaque module descriptor must decline")
	}

	// A facet-carrying value whose payload is not a MapPayload → decline
	// (defensive: the facet is only ever stamped on maps).
	odd := WithModuleNS(NewInteger(1), "E", Value{})
	if ModuleExportAbsenceStable(r, []Value{odd, NewAtom("k")}) {
		t.Error("a non-map facet carrier must decline")
	}

	// Two namespace operands → not the 2-operand get shape.
	ns := absenceArgsW9(NewOrderedMap(), "")[0]
	if ModuleExportAbsenceStable(r, []Value{ns, ns}) {
		t.Error("two namespace operands must decline")
	}

	// Two key candidates → not the 2-operand get shape.
	if ModuleExportAbsenceStable(r, []Value{NewAtom("a"), NewAtom("b")}) {
		t.Error("two key operands must decline")
	}

	// A String key with no receiver → fields nil → decline (exercises the
	// AsConcreteString branch and the fields==nil guard).
	if ModuleExportAbsenceStable(r, []Value{NewString("k")}) {
		t.Error("a lone string key must decline")
	}

	// An operand that is neither module-family nor a concrete atom/string.
	if ModuleExportAbsenceStable(r, []Value{NewInteger(1)}) {
		t.Error("a non-key non-module operand must decline")
	}
}
