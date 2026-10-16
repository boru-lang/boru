package check

// Check-piece make mirror: the check-mode model of class / Resource
// construction. Extracted from core_make.go (Stage 2b of the four-piece
// split).

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// CheckMakeConstruction is the CHECK-MODE mirror of the class / Resource
// construction validation (makeClassInstance / makeResource): when the make
// target resolves to a class or Resource/Entity schema and the construction
// map is CONCRETE, the schema-decidable rules run right here — an unknown
// field, a missing non-defaulted field, and (for a concrete field value) the
// field-type check via MakeClassFieldValue — each emitted as a type_error
// diagnostic carrying the byte-identical runtime message. Value-dependent
// parts stay with the runtime constructor: a carrier construction map or a
// carrier field value is skipped. Deduped by detail+position — the ReturnsFn
// runs once per analysed call shape, but a body can be analysed under
// several shapes. No-op outside check mode.
func CheckMakeConstruction(r *core.Registry, target, src core.Value, pos core.SrcPos) {
	if r == nil || !r.Check.IsActive() {
		return
	}
	target = core.ResolveTypeLiteralDef(target, r)
	if !core.IsConcrete(&src) || core.ParentOf(&src) == nil || !core.ParentOf(&src).ConformsTo(core.TMap) {
		return
	}
	provided, _ := core.AsMap(src)
	if provided == nil {
		return
	}
	var allFields *core.OrderedMap
	var label string
	switch {
	case core.IsClassType(&target):
		ot, err := core.AsClassType(target)
		if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return
		}
		allFields, label = ot.AllFields(), "class "+ot.Name
	case core.IsResourceType(target):
		rt, err := core.AsResourceType(target)
		if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return
		}
		allFields, label = rt.AllFields(), rt.Name
	default:
		return
	}
	diag := func(detail string) {
		for _, d := range r.Check.Diagnostics {
			if d.Code == "type_error" && d.Detail == detail && d.Row == pos.Row && d.Col == pos.Col {
				return
			}
		}
		r.Check.AddDiagnostic(core.CheckDiagnostic{
			Code:   "type_error",
			Detail: detail,
			Word:   "make",
			Row:    pos.Row,
			Col:    pos.Col,
		})
	}
	for _, key := range provided.Keys() {
		if _, ok := allFields.Get(key); !ok {
			diag(fmt.Sprintf("make: unknown field %q for %s", key, label))
		}
	}
	for _, key := range allFields.Keys() {
		constraint, _ := allFields.Get(key)
		val, hasVal := provided.Get(key)
		if !hasVal {
			// Same has-default probe the runtime constructors use: a
			// schema slot with a payload is a concrete default.
			if core.DataOf(&constraint) != nil {
				continue
			}
			diag(fmt.Sprintf("make: missing field %q for %s", key, label))
			continue
		}
		if !core.IsConcrete(&val) {
			continue
		}
		if _, err := core.MakeClassFieldValue(val, constraint, r); err != nil {
			diag(fmt.Sprintf("make: field %q: %v", key, err))
		}
	}
}
