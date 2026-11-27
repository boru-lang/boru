package basic

import (
	core "github.com/boru-lang/boru/core/go"
)

// The predefined Resource / Entity object types (Ideal/Resource,
// Ideal/Resource/Entity). The lattice nodes themselves are kernel
// builtins (core/go/typetable.go); this file owns their language-facing
// definitions — the field schemas installed into every registry.

// InstallResourceTypes pushes the builtin Resource and Entity object
// types onto the type stack. Called once during engine.Register.
//
//   - Object/Resource has field kind:String
//   - Object/Resource/Entity inherits kind from Resource and adds
//     spec:String, entity:String
//
// These are registered via InstallDef so they get proper handler
// resolution and can be referenced by name in boru code (e.g. make
// Entity {...}).
func InstallResourceTypes(r *Registry) {
	resourceFields := NewOrderedMap()
	resourceFields.Set("kind", NewTypeLiteral(TString))

	resourceInfo := ResourceTypeInfo{
		Fields: resourceFields,
		Parent: nil,
		ID:     BuiltinIDForPath("Ideal/Resource"),
		Name:   TResource.String(),
	}

	InstallDef(r, ResourceDefKey("Resource"), NewResourceType(TResource, resourceInfo))

	resourceVal, _ := r.Defs.Top(ResourceDefKey("Resource"))
	installedResource, _ := AsResourceType(resourceVal)

	entityFields := NewOrderedMap()
	entityFields.Set("spec", NewTypeLiteral(TString))
	entityFields.Set("entity", NewTypeLiteral(TString))

	entityInfo := ResourceTypeInfo{
		Fields: entityFields,
		Parent: &installedResource,
		ID:     BuiltinIDForPath("Ideal/Resource/Entity"),
		Name:   TResourceEntity.String(),
	}

	InstallDef(r, ResourceDefKey("Entity"), NewResourceType(TResourceEntity, entityInfo))
}

// ResourceDefKey re-exports the eng hidden-key convention
// (core.ResourceDefKey) — see that helper for the rationale.
func ResourceDefKey(name string) string { return core.ResourceDefKey(name) }
