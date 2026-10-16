package core

// The content fields of a Value — its payload, its parent, and the
// carrier, origin and internal-marker flags — are read and written here
// (no-copy phase 4, design/VALUE-NO-COPY.0.md §6 "Phase 4 as built"): they
// live in the content a Value's holders share, and every module reaches
// them through these functions. A setter gives the one holder it is called
// on content of its own (ownContent), never writing content another holder
// can see.

// DataOf is v's payload: nil for a bare type node, a carrier with no
// payload, or the zero Value.
func DataOf(v *Value) Payload {
	if v.c == nil {
		return nil
	}
	return v.c.data
}

// ParentOf is the node directly above v in the lattice — an ordinary
// value's type, a type node's supertype — and nil for a lattice root or the
// zero Value.
func ParentOf(v *Value) *Type {
	if v.c == nil {
		return nil
	}
	return v.c.parent
}

// IsCarrier reports whether v is a static-typecheck carrier: a type-only
// stand-in the check pass holds where a run would hold a value.
func IsCarrier(v *Value) bool { return v.c != nil && v.c.carrier }

// OriginOf is how v's type node came to be: builtin or user-defined.
func OriginOf(v *Value) OriginKind {
	if v.c == nil {
		return OriginBuiltin
	}
	return v.c.origin
}

// IsInternal reports whether v is a runtime marker (Word/__XX) rather than
// a user-facing value.
func IsInternal(v *Value) bool { return v.c != nil && v.c.isInternal }

// SetData gives this holder of v the payload d.
func SetData(v *Value, d Payload) { ownContent(v).data = d }

// SetParent gives this holder of v the parent t.
func SetParent(v *Value, t *Type) { ownContent(v).parent = t }

// SetCarrier marks this holder of v a carrier, or not.
func SetCarrier(v *Value, b bool) { ownContent(v).carrier = b }

// SetOrigin gives this holder of v the origin o.
func SetOrigin(v *Value, o OriginKind) { ownContent(v).origin = o }

// SetInternal marks this holder of v a runtime marker, or not.
func SetInternal(v *Value, b bool) { ownContent(v).isInternal = b }

// MakeValue is the Value with parent t and payload d and nothing else: no
// identity and no occurrence state (NewValueRaw mints an identity where a
// pass needs one).
func MakeValue(t *Type, d Payload) Value { return Value{c: &content{parent: t, data: d}} }

// MakeCarrierValue is MakeValue as a carrier.
func MakeCarrierValue(t *Type, d Payload) Value {
	return Value{c: &content{parent: t, data: d, carrier: true}}
}
