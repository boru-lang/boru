package core

import "fmt"

// The KERNEL-RESIDENT half of the Scalar/Micron structured-scalar
// family. The family's identity lives in builtinDecls (paths, FixedIDs,
// positional Ranks, interval labels — the Resource/Entity precedent),
// and this file keeps the machinery the kernel itself consumes: the
// sealed payload variants with their accessors, Pathon content
// equality (equal.go), the render bridge, and the two opt-in
// capabilities (ADR-012 rule 5) through which the CONTENT half —
// validators, grammars, Behaviors, the -on naming rule, now in
// basic/go/micron.go — plugs back in without the kernel naming a
// single Micron leaf.

// MicronTypeInfo is the type body produced by `refine Micron {fields}`
// — the kernel analogue of ClassTypeInfo. InstallType's Micron branch
// fills in Name/Type when the body is bound to a capitalised name.
type MicronTypeInfo struct {
	Name   string
	Type   *Type
	Fields *OrderedMap
}

// MicronPayload is the instance payload for Emailon / Urlon / user
// Microns. Fields holds only the PRIMARY fields — derived properties
// (Emailon's address, Urlon's href) are synthesized at read time and
// never stored, so content equality compares primary fields only.
// Pathon keeps its own PathonPayload (io words and the fileinfo path
// consume it).
type MicronPayload struct{ Fields *OrderedMap }

// IsMicronType reports whether v is a Micron type body (the
// `refine Micron {fields}` construction result).
func IsMicronType(v Value) bool {
	_, ok := DataOf(&v).(MicronTypeInfo)
	return ok
}

// AsMicronType returns the MicronTypeInfo payload.
func AsMicronType(v Value) (MicronTypeInfo, error) {
	if info, ok := DataOf(&v).(MicronTypeInfo); ok {
		return info, nil
	}
	return MicronTypeInfo{}, fmt.Errorf("AsMicronType: not a Micron type body (got %T)", DataOf(&v))
}

// IsMicronValue reports whether v carries a MicronPayload (an Emailon /
// Urlon / user-Micron instance). Pathon values carry PathonPayload —
// probe with IsPathon.
func IsMicronValue(v Value) bool {
	_, ok := DataOf(&v).(MicronPayload)
	return ok
}

// AsMicronFields returns the primary-field map of a MicronPayload
// instance.
func AsMicronFields(v Value) (*OrderedMap, error) {
	if p, ok := DataOf(&v).(MicronPayload); ok {
		return p.Fields, nil
	}
	return nil, fmt.Errorf("AsMicronFields: not a Micron instance (got %T)", DataOf(&v))
}

// PathonContentEqual is Pathon's content equality: same segments, same
// absolute/relative flag — exactly the pairs comparePathons orders as
// 0, so eq/deq agree with cmp on paths.
func PathonContentEqual(a, b PathonInfo) bool {
	if a.Abs != b.Abs || a.Volume != b.Volume || len(a.Parts) != len(b.Parts) {
		return false
	}
	for i := range a.Parts {
		if a.Parts[i] != b.Parts[i] {
			return false
		}
	}
	return true
}

// SubtypeNamer is an opt-in TypeBehavior capability: a type whose
// Behavior implements it imposes a naming rule on every subtype minted
// beneath it (typed defs, bare-nominal refines, dependent scalars,
// host member types). The kernel consults it generically via
// validateSubtypeNameFor — it carries no name policy of its own; the
// Micron family's -on rule is the first implementation (basic/go).
type SubtypeNamer interface {
	ValidateSubtypeName(name string) error
}

// validateSubtypeNameFor walks parent's ancestor chain for the nearest
// Behavior implementing SubtypeNamer and applies its rule to name. No
// namer anywhere on the chain means no rule — the nil error.
func validateSubtypeNameFor(parent *Type, name string) error {
	for t := parent; t != nil; t = ParentOf(t) {
		if sn, ok := t.Behavior().(SubtypeNamer); ok {
			return sn.ValidateSubtypeName(name)
		}
	}
	return nil
}

// MicronSubtypeMinter is the second opt-in capability, keyed on the
// kernel MicronTypeInfo payload: when InstallType binds a
// `refine Micron {fields}` body to a capitalised name, the freshly
// minted subtype's Behavior comes from the family root's Behavior via
// this hook (the content layer returns its family Behavior carrying
// the schema). Without a minter on the root — an eng-alone build —
// the mint falls back to DefaultBehavior.
type MicronSubtypeMinter interface {
	MintSubtypeBehavior(def *Type, info *MicronTypeInfo) TypeBehavior
}

// micronRenderBridge is the content layer's canonical MicronPayload
// render, installed by basic at init (the RegisterBytesBridge shape:
// eng owns the mechanism seam, the owning layer supplies the
// conversion). Consulted only by the display-string backstop in
// value.go; nil — an eng-alone build — falls through to the generic
// rendering.
var micronRenderBridge func(Value) string

// RegisterMicronRenderBridge installs the MicronPayload render hook.
// Called once from the content layer's package init (single-threaded).
func RegisterMicronRenderBridge(f func(Value) string) { micronRenderBridge = f }

// MicronFieldsEqual is MicronPayload's primary-field equality — the
// kernel's structural map equality over the payload's field map,
// exported for the content layer's Behavior (basic/go/micron.go).
func MicronFieldsEqual(a, b *OrderedMap) bool {
	return mapsEqual(a, b)
}
