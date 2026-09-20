package native

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// Module namespaces — what `import` binds — and the Ideal/Module
// descriptor type.
//
// When boru code runs `import "boru:math-util"`, the bound name `MathUtil`
// is a PLAIN MAP of the raw exports (one per `export "Name" {…}`
// declaration) carrying the kernel's module-namespace FACET
// (core.ModuleNSInfo — NUR038: provenance is a facet on a plain value,
// never a wrapper type that masks the value). Exported values are exactly
// what the module exported — a fn stays a Function, a constant stays that
// constant, a type stays a plain type — while the facet answers the
// synthetic names `$name` and `$module`:
//
//	MathUtil.sqrt          → the exported sqrt function (raw, callable)
//	MathUtil.$name         → 'MathUtil'               (the export name)
//	MathUtil.$module       → the Module instance      (Ideal/Module)
//	MathUtil.$module.name  → 'boru:math-util'         (the module reference)
//
// A Module instance (Ideal/Module) is the descriptor shared by all of a
// module's namespaces via $module. Its normal fields are name, kind,
// file, folder, and exports (the list of export names).
//
// (The former Ideal/ModuleExport wrapper type — FixedID 5001 — is
// RETIRED: the FixedID is never recycled. See the fixedid stability
// test's retired table.)

// The Ideal/Module type identity, constructor, Behavior, and descriptor
// unwrappers are kernel-declared (eng/go/moduletype.go — ADR-012 stage 2);
// the aliases below keep this package's historical spellings for the
// namespace glue that stays here.
var (
	asModuleDesc      = core.AsModuleDesc
	moduleKind        = core.ModuleKind
	moduleExportNames = core.ModuleExportNames
)

// moduleExportFieldModule and …Name are the synthetic keys a module
// namespace answers in addition to its exported fields.
const (
	moduleExportFieldModule = "$module"
	moduleExportFieldName   = "$name"
)

// NewModuleNamespace builds the value `import` binds for one export: a
// PLAIN Map over the export fields, carrying the module-namespace facet
// (export name + owning Module descriptor). The map payload is the
// module's own export *OrderedMap — shared, not copied — so runtime
// export growth (the DSL `register` family) is visible through the
// binding, and the kernel's growth ledger (module_export_growth.go) keys
// per-map state on the same pointer.
func NewModuleNamespace(name string, fields *OrderedMap, module Value) Value {
	if fields == nil {
		fields = NewOrderedMap()
	}
	return core.WithModuleNS(NewMap(fields), name, module)
}

// moduleNSFields returns the export-fields map behind a bound module
// namespace (a concrete Map carrying the module-namespace facet), or nil
// for anything else.
func moduleNSFields(v Value) *OrderedMap {
	if core.ModuleNSOf(v) == nil || !IsConcrete(v) {
		return nil
	}
	if mp, ok := v.Data.(core.MapPayload); ok {
		return mp.M
	}
	return nil
}

// moduleNamespaceBound reports whether name is bound to a module
// namespace (a facet-carrying Map) in the current scope.
func moduleNamespaceBound(r *Registry, name string) bool {
	top, ok := r.Defs.Top(name)
	return ok && moduleNSFields(top) != nil
}

// moduleNamespaceExport returns the target export from the module
// namespace bound as name. ok=false when the name is unbound, not a
// namespace, or lacks the export.
func moduleNamespaceExport(r *Registry, name, target string) (Value, bool) {
	top, ok := r.Defs.Top(name)
	if !ok {
		return Value{}, false
	}
	fields := moduleNSFields(top)
	if fields == nil {
		return Value{}, false
	}
	return fields.Get(target)
}

// moduleExportGet resolves a key against a module namespace: the synthetic
// $module / $name from the facet, otherwise an exported field from the
// map. ok=false when v carries no facet or the key is neither synthetic
// nor an export.
func moduleExportGet(v Value, key string) (Value, bool) {
	ns := core.ModuleNSOf(v)
	if ns == nil {
		return Value{}, false
	}
	switch key {
	case moduleExportFieldName:
		return NewString(ns.Name), true
	case moduleExportFieldModule:
		return ns.Module, true
	}
	fields := moduleNSFields(v)
	if fields == nil {
		return Value{}, false
	}
	return fields.Get(key)
}

// moduleGet resolves a key against a Module descriptor's normal fields.
func moduleGet(v Value, key string) (Value, bool) {
	desc, ok := asModuleDesc(v)
	if !ok {
		return Value{}, false
	}
	switch key {
	case "name":
		return NewString(desc.Ref), true
	case "kind":
		return NewString(moduleKind(desc)), true
	case "file":
		return NewString(desc.File), true
	case "folder":
		return NewString(desc.Folder), true
	case "exports":
		names := moduleExportNames(desc)
		elems := make([]Value, len(names))
		for i, e := range names {
			elems[i] = NewString(e)
		}
		return NewList(elems), true
	}
	return Value{}, false
}

// moduleInstGetReturns is the check-mode return computer for a Module
// descriptor read (get/getr/dot/dotr on `X.$module`): the field set is
// CLOSED (moduleGet) — name/kind/file/folder are always Strings, exports
// is always a List of export-name Strings. An unknown or non-concrete key
// stays dynamic(Any) (get reads none, getr raises — the runtime's job).
func moduleInstGetReturns(args []Value, _ *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if len(args) != 2 || !IsConcrete(args[0]) {
		return dyn
	}
	switch getKey(args[0]) {
	case "name", "kind", "file", "folder":
		return []Value{NewCarrier(TString)}
	case "exports":
		return []Value{NewCarrier(TList)}
	}
	return dyn
}

// ---- module-namespace hooks for the shared map accessor family ----
//
// A namespace dispatches the ORDINARY map get/getr/has/dot sigs (it IS a
// Map); these helpers add the facet-only behaviours the retired
// Ideal/ModuleExport sigs used to own: the $name/$module synthetics, the
// module-flavored strict-miss error, and the check-mode raw-export
// resolution.

// moduleNSGetSynthetic answers the $name/$module synthetics for a facet
// map. ok=false for a non-namespace receiver or a plain key (which then
// takes the normal map read).
func moduleNSGetSynthetic(container Value, key string) (Value, bool) {
	if core.ModuleNSOf(container) == nil {
		return Value{}, false
	}
	switch key {
	case moduleExportFieldName, moduleExportFieldModule:
		return moduleExportGet(container, key)
	}
	return Value{}, false
}

// moduleNSGetrMiss builds the strict-read miss error for a module
// namespace — "export … not found in module", with the did-you-mean
// suggestion over the export keys — replacing the generic map wording so
// a typo'd export points at the module contract, not at a mutable map.
func moduleNSGetrMiss(r *Registry, container Value, k string, keyPos core.SrcPos) error {
	var keys []string
	if fields := moduleNSFields(container); fields != nil {
		keys = fields.Keys()
	}
	return notFoundKeyError(r, fmt.Sprintf("getr: export %q not found in module", k), k, keyPos, keys)
}

// moduleNSGetReturns is the check-mode read over a module namespace for
// `get`/`dot`. Without it `Pkg.fn` would take the plain-map narrowing —
// which deliberately degrades a Function field to dynamic(Any) and reads
// a missing key as None. A namespace needs the opposite on both counts:
// the raw export value rides out (a function export keeps its FnDefInfo,
// so it stays dispatchable — toCarrier preserves it), and a MISSING key
// stays a strict Any because a module's keyspace can GROW at run time
// (the DSL `register` family; the growth ledger governs which absences
// may fold — module_export_growth.go). ok=false for a non-namespace
// receiver.
func moduleNSGetReturns(args []Value) ([]Value, bool) {
	if len(args) != 2 || core.ModuleNSOf(args[1]) == nil || !IsConcrete(args[1]) {
		return nil, false
	}
	if IsConcrete(args[0]) {
		if val, ok := moduleExportGet(args[1], getKey(args[0])); ok {
			return []Value{val}, true
		}
	}
	// A missing / unresolved export stays a strict Any: emitting
	// dynamic(Any) here would *admit* downstream typed uses and so mask
	// a likely typo worse than the strict fallback (which fails loudly
	// at the next typed slot). Dynamic escape hatches are for genuinely
	// unknown VALUES, not unresolved names.
	return []Value{NewCarrier(TAny)}, true
}

// moduleNSGetrReturns is the getr-specific check-mode read over a module
// namespace. It mirrors moduleNSGetReturns for a resolvable export, but
// for a MISSING concrete key it records a TERMINAL not_found trap
// (top-level only): getr raises not_found at runtime for a missing
// export, so the compiled program raises the byte-identical error here
// via OpTrap instead of declining downstream on the unmaterialisable Any
// residual. RecordTrap truncates the program to the trap, dropping the
// residual. A nested occurrence declines the trap (off the top frame)
// and keeps the lenient Any fallback. ok=false for a non-namespace
// receiver.
func moduleNSGetrReturns(args []Value, r *Registry) ([]Value, bool) {
	if len(args) != 2 || core.ModuleNSOf(args[1]) == nil || !IsConcrete(args[1]) {
		return nil, false
	}
	if IsConcrete(args[0]) {
		k := getKey(args[0])
		if val, ok := moduleExportGet(args[1], k); ok {
			return []Value{val}, true
		}
		// A module's export map is sealed at import (boru-level; runtime
		// growers are ledger-modelled), so a concrete-key miss is ALSO a
		// check diagnostic (the strict-read contract, same text). A
		// RuntimeMirror: the compile failure loop skips it, so the trap below
		// keeps compiling (TestModuleExportGetrNotFoundTrapCompiles pins
		// it).
		if r.Check.IsActive() {
			core.CheckAddUniqueDiagnostic(r, "not_found",
				fmt.Sprintf("getr: export %q not found in module", k), "getr", args[0].Pos())
		}
		// Record the SAME rich error the runtime path raises (with the
		// did-you-mean suggestion over the export keys) so the compiled trap
		// is byte-identical to the interpreter's miss.
		var keys []string
		if fields := moduleNSFields(args[1]); fields != nil {
			keys = fields.Keys()
		}
		ferr := buildNotFoundKeyError(r,
			fmt.Sprintf("getr: export %q not found in module", k), k, args[0].Pos(), keys)
		r.Check.Recorder().RecordTrapErr(ferr, args[0].Pos())
	}
	return []Value{NewCarrier(TAny)}, true
}

// ---- get / getr handlers for the Module descriptor ----

func getModuleInstHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, r.BoruError("get_error", "get: cannot access property on type literal", "get")
	}
	if val, ok := moduleGet(args[1], getKey(args[0])); ok {
		return []Value{val}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

func getrModuleInstHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return nil, r.BoruError("getr_error", "getr: cannot access property on type literal", "getr")
	}
	k := getKey(args[0])
	if val, ok := moduleGet(args[1], k); ok {
		return []Value{val}, nil
	}
	return nil, r.BoruError("not_found", fmt.Sprintf("getr: field %q not found in Module", k), "getr")
}
