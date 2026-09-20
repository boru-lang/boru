package core

import (
	"fmt"
	"sort"
)

// Ideal/Module — the module descriptor type. KERNEL-RESIDENT (ADR-012
// rule 1): modules and namespaces are engine mechanics — the kernel owns
// the module gate, export growth, the compiled module-const fold
// (carrier.go::isModuleFamilyValue), and the ModuleDesc payload — so the
// type identity, constructor, and type-local Behavior live here too.
// Moved in from lang/go/native/native_module_types.go (FixedID 5000
// preserved); the module NAMESPACE facet glue (the $name/$module
// synthetics wired into the map accessor words) stays in the language
// layer, which is where the accessor words live.

// TModule is Ideal/Module — the module descriptor instance type. It is
// also the value `module […]` produces and that `import` consumes (it
// carries the full ModuleDesc), so there is no separate internal carrier
// type.
var TModule = mustType("Ideal/Module")

// The Behavior attaches at init like the other builtin-decl behaviors
// (core_xml.go / coretype_list_map_behaviors.go pattern).
func init() {
	TModule.ensureTMeta().Behavior = moduleTypeBehavior{path: "Ideal/Module"}
}

// NewModuleInstance builds an Ideal/Module value wrapping a ModuleDesc. The
// descriptor metadata (Ref/Kind/File/Folder) and the export names are
// surfaced as the name/kind/file/folder/exports fields; the carried Exports
// map is what `import` installs.
//
// The payload BOXES the descriptor (ExtensionPayload{Body: *ModuleDesc}):
// the pointer is the descriptor's IDENTITY under eq/deq (NUR031 — the
// kernel's opaqueIdealExactEqual/DeepEqual arms compare it, exactly like
// the *TimeoutInfo/*IntervalInfo handles), so every Value copy of one
// instance stays eq to itself while each NewModuleInstance call mints a
// distinct instance (per-import-instance identity). The ExtensionPayload
// wrapper itself is load-bearing — several kernel arms key on it (const
// interning, resolution elision, ConstBakeable compile failure, ID minting) — so
// the payload must stay an ExtensionPayload, never a bare *ModuleDesc.
func NewModuleInstance(desc ModuleDesc) Value {
	return Value{Parent: TModule, Data: ExtensionPayload{Body: &desc}}
}

// asModuleDesc unwraps the ExtensionPayload behind an Ideal/Module value
// (a boxed *ModuleDesc — see NewModuleInstance).
func asModuleDesc(v Value) (ModuleDesc, bool) {
	if ep, ok := v.Data.(ExtensionPayload); ok {
		if d, ok := ep.Body.(*ModuleDesc); ok && d != nil {
			return *d, true
		}
	}
	return ModuleDesc{}, false
}

// ModuleKind returns a Module's kind, defaulting "" to "inline".
func ModuleKind(desc ModuleDesc) string {
	if desc.Kind == "" {
		return "inline"
	}
	return desc.Kind
}

// ModuleExportNames returns a Module's export names, sorted for determinism.
func ModuleExportNames(desc ModuleDesc) []string {
	names := make([]string, 0, len(desc.Exports))
	for name := range desc.Exports {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// moduleTypeBehavior renders Module descriptor values and matches
// nominally (DefaultBehavior semantics for Match/Equal).
type moduleTypeBehavior struct {
	path string
}

func (moduleTypeBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (moduleTypeBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }

func (b moduleTypeBehavior) Format(v Value) string {
	if desc, ok := asModuleDesc(v); ok {
		ref := desc.Ref
		if ref == "" {
			ref = ModuleKind(desc)
		}
		return fmt.Sprintf("Module(%s)", ref)
	}
	return b.path
}

// AsModuleDesc unwraps the ModuleDesc carried by an Ideal/Module value
// (what `module […]` produces and `import` consumes). Exported for hosts
// and integration tests; ok=false for non-Module values.
func AsModuleDesc(v Value) (ModuleDesc, bool) { return asModuleDesc(v) }

// ToMap / ToList implement IdealConverter for the Module descriptor:
// it projects to its descriptor fields (name/kind/file/folder/exports).
// (A module NAMESPACE needs no converter — it already IS a Map.)
func (moduleTypeBehavior) ToMap(v Value) (Value, error) {
	if desc, ok := asModuleDesc(v); ok {
		out := NewOrderedMap()
		out.Set("name", NewString(desc.Ref))
		out.Set("kind", NewString(ModuleKind(desc)))
		out.Set("file", NewString(desc.File))
		out.Set("folder", NewString(desc.Folder))
		names := ModuleExportNames(desc)
		elems := make([]Value, len(names))
		for i, n := range names {
			elems[i] = NewString(n)
		}
		out.Set("exports", NewList(elems))
		return NewMap(out), nil
	}
	return NewMap(NewOrderedMap()), nil
}

func (moduleTypeBehavior) ToList(v Value) (Value, error) {
	if desc, ok := asModuleDesc(v); ok {
		names := ModuleExportNames(desc)
		elems := make([]Value, len(names))
		for i, n := range names {
			elems[i] = NewString(n)
		}
		return NewList(elems), nil
	}
	return NewList(nil), nil
}
