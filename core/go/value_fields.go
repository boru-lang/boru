package core

// The five public fields of a Value — its payload, its parent, and the
// carrier, origin and internal-marker flags — are read and written here
// (no-copy phase 4, design/VALUE-NO-COPY.0.md §6 "Phase 4 as built"): the
// fields move behind the shared content pointer, and every module reaches
// them through these functions. A setter gives the one holder it is called
// on the new field, never a value another holder shares.

// DataOf is v's payload: nil for a bare type node, a carrier with no
// payload, or the zero Value.
func DataOf(v *Value) Payload { return v.Data }

// ParentOf is the node directly above v in the lattice — an ordinary
// value's type, a type node's supertype — and nil for a lattice root or the
// zero Value.
func ParentOf(v *Value) *Type { return v.Parent }

// IsCarrier reports whether v is a static-typecheck carrier: a type-only
// stand-in the check pass holds where a run would hold a value.
func IsCarrier(v *Value) bool { return v.Carrier }

// OriginOf is how v's type node came to be: builtin or user-defined.
func OriginOf(v *Value) OriginKind { return v.Origin }

// IsInternal reports whether v is a runtime marker (Word/__XX) rather than
// a user-facing value.
func IsInternal(v *Value) bool { return v.IsInternal }

// SetData gives this holder of v the payload d.
func SetData(v *Value, d Payload) { v.Data = d }

// SetParent gives this holder of v the parent t.
func SetParent(v *Value, t *Type) { v.Parent = t }

// SetCarrier marks this holder of v a carrier, or not.
func SetCarrier(v *Value, b bool) { v.Carrier = b }

// SetOrigin gives this holder of v the origin o.
func SetOrigin(v *Value, o OriginKind) { v.Origin = o }

// SetInternal marks this holder of v a runtime marker, or not.
func SetInternal(v *Value, b bool) { v.IsInternal = b }

// MakeValue is the Value with parent t and payload d and nothing else: no
// identity and no occurrence state (NewValueRaw mints an identity where a
// pass needs one).
func MakeValue(t *Type, d Payload) Value { return Value{Parent: t, Data: d} }

// MakeCarrierValue is MakeValue as a carrier.
func MakeCarrierValue(t *Type, d Payload) Value { return Value{Parent: t, Data: d, Carrier: true} }
