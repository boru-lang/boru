package core

import (
	"errors"
)

// ErrNoConverter lets a Behavior decline the IdealConverter capability so
// the parent-chain walk continues — mirrors ErrNoComparer. A wrapper
// Behavior that structurally satisfies IdealConverter but has no body
// installed returns this from ToMap/ToList.
var ErrNoConverter = errors.New("eng: no converter in this Behavior")

// IdealConverter is the optional capability that lets an Ideal type
// convert its values to a Map or a List. Implement it on a TypeBehavior.
//
// The base Ideal type (idealRootBehavior, installed below) provides the
// fallback: any Ideal that does not override converts to an empty Map ({})
// or empty List ([]). Concrete Ideals — built-in or user-defined, in Go or
// boru — may attach a Behavior implementing this to give a meaningful
// projection. Dispatch walks the value's Parent chain (ConvertIdealToMap /
// ConvertIdealToList) exactly like the Comparer walk, so the nearest
// override wins and the Ideal root guarantees a terminal result.
type IdealConverter interface {
	ToMap(v Value) (Value, error)
	ToList(v Value) (Value, error)
}

// ConvertIdealToMap converts an Ideal value to a Map by walking its
// Parent chain for the nearest IdealConverter. Falls back to an empty Map
// when nothing in the chain converts (the Ideal root always does).
func ConvertIdealToMap(v Value) (Value, error) {
	for t := ValueType(v); t != nil; t = t.Parent() {
		c, ok := t.Behavior().(IdealConverter)
		if !ok {
			continue
		}
		m, err := c.ToMap(v)
		if errors.Is(err, ErrNoConverter) {
			continue
		}
		return m, err
	}
	return NewMap(NewOrderedMap()), nil
}

// ConvertIdealToList converts an Ideal value to a List by the same walk.
func ConvertIdealToList(v Value) (Value, error) {
	for t := ValueType(v); t != nil; t = t.Parent() {
		c, ok := t.Behavior().(IdealConverter)
		if !ok {
			continue
		}
		l, err := c.ToList(v)
		if errors.Is(err, ErrNoConverter) {
			continue
		}
		return l, err
	}
	return NewList(nil), nil
}

// idealRootBehavior is THE Ideal-root Behavior: one struct composing
// every root capability, assigned exactly once (below). It supplies the
// empty-{}/[] IdealConverter fallback for every Ideal that does not
// override, and the payload-switch Sizer (its Size method lives in
// size.go with the other kernel Sizers); Match/Format/Equal stay at the
// kernel default via the embedded defaultBehavior + formatDelegate.
//
// One struct is load-bearing (the NUR017 fix): the root previously had
// TWO mutually-exclusive behaviors — a converter-only struct assigned
// here and a sizer-only struct assigned in size.go's init — so lexical
// filename order decided which survived, the loser's assignment was
// dead, and renaming a file would silently drop `size` (or `convert`)
// on every Ideal kind without its own override.
// ideal_root_behavior_test.go pins the resolved composition.
type idealRootBehavior struct{ defaultBehavior }

func (idealRootBehavior) formatDelegate() {}

func (idealRootBehavior) ToMap(Value) (Value, error)  { return NewMap(NewOrderedMap()), nil }
func (idealRootBehavior) ToList(Value) (Value, error) { return NewList(nil), nil }

func init() {
	// The SINGLE Ideal-root Behavior install. Package vars (incl.
	// TIdeal) are initialised before init() runs — see the
	// compare_scalar_behaviors.go init for the same ordering rationale.
	TIdeal.ensureTMeta().Behavior = idealRootBehavior{}
}

// ---- concrete converters for the built-in Ideal kinds ----
//
// Each behavior adds the IdealConverter capability to a kernel Ideal
// without changing its rendering: Format delegates to kernelFormatDefault
// (Value.String calls that Format, so
// error(…)/array/etc. output is unchanged). Match/Equal stay default.

// objectConvertBehavior: an Object instance → its fields (own + inherited).
type objectConvertBehavior struct{}

func (objectConvertBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (objectConvertBehavior) matchesAsDefault()           {}
func (objectConvertBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (objectConvertBehavior) Format(v Value) string       { return kernelFormatDefault(v) }
func (objectConvertBehavior) ToMap(v Value) (Value, error) {
	fm, ok := FlatInstanceFields(v)
	if !ok {
		return NewMap(NewOrderedMap()), nil
	}
	return NewMap(fm), nil
}
func (objectConvertBehavior) ToList(v Value) (Value, error) {
	fm, ok := FlatInstanceFields(v)
	if !ok {
		return NewList(nil), nil
	}
	return NewList(orderedMapValues(fm)), nil
}

// ClassFields is the exported view of objectFieldMap — the flattened
// field map of an object or class instance (prototype chain base-first
// for legacy object instances; class instances are already flat). Used
// by the lang layer for items / transform / serialization projections.
func ClassFields(oi *ClassInstanceInfo) *OrderedMap {
	return objectFieldMap(oi)
}

// objectFieldMap copies an instance's (flat) fields into a fresh
// OrderedMap.
func objectFieldMap(oi *ClassInstanceInfo) *OrderedMap {
	out := NewOrderedMap()
	if oi != nil && oi.Fields != nil {
		for _, k := range oi.Fields.Keys() {
			val, _ := oi.Fields.Get(k)
			out.Set(k, val)
		}
	}
	return out
}

func orderedMapValues(m *OrderedMap) []Value {
	out := make([]Value, 0, m.Len())
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		out = append(out, v)
	}
	return out
}

// storeConvertBehavior: a Store → its own key/value entries (sorted keys
// for determinism); ToList is the values in that order.
type storeConvertBehavior struct{}

func (storeConvertBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (storeConvertBehavior) matchesAsDefault()           {}
func (storeConvertBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (storeConvertBehavior) Format(v Value) string       { return kernelFormatDefault(v) }
func (storeConvertBehavior) ToMap(v Value) (Value, error) {
	return NewMap(storeEntryMap(v)), nil
}
func (storeConvertBehavior) ToList(v Value) (Value, error) {
	return NewList(orderedMapValues(storeEntryMap(v))), nil
}

func storeEntryMap(v Value) *OrderedMap {
	out := NewOrderedMap()
	si, err := AsStore(v)
	if err != nil || si == nil {
		return out
	}
	// The VISIBLE keyset, not this layer's own — enumeration walks the
	// prototype chain with masking and tombstones exactly as Get does, so
	// a Store shows the keys it answers for (NUR052). Reading si.Data
	// directly under-reported every key inherited from a parent layer.
	for _, e := range si.VisibleEntries() {
		out.Set(e.Key, e.Value)
	}
	return out
}

// errorConvertBehavior: an Error → {message:…}; ToList is [message].
type errorConvertBehavior struct{}

func (errorConvertBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (errorConvertBehavior) matchesAsDefault()           {}
func (errorConvertBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (errorConvertBehavior) Format(v Value) string       { return kernelFormatDefault(v) }
func (errorConvertBehavior) ToMap(v Value) (Value, error) {
	m := NewOrderedMap()
	if ei, err := AsError(v); err == nil {
		// Code first when present (a BoruError-backed or `raise`d
		// error); a plain Go error stays {message:…} as before.
		if ei.Code != "" {
			m.Set("code", NewAtom(ei.Code))
		}
		m.Set("message", NewString(ei.Message))
		if ei.Data != nil {
			for _, k := range ei.Data.Keys() {
				dv, _ := ei.Data.Get(k)
				m.Set(k, dv)
			}
		}
	}
	return NewMap(m), nil
}
func (errorConvertBehavior) ToList(v Value) (Value, error) {
	if ei, err := AsError(v); err == nil {
		return NewList([]Value{NewString(ei.Message)}), nil
	}
	return NewList(nil), nil
}

// reachConvertBehavior: an Ideal/Reach → an inspectable map describing its
// receiver and segments, and a list of its segment keys. Format renders the
// dotted surface (m.a.b) so Value.String() matches canon. See REACH.10.md §7.
type reachConvertBehavior struct{}

func (reachConvertBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (reachConvertBehavior) matchesAsDefault()           {}
func (reachConvertBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (reachConvertBehavior) Format(v Value) string       { return canonReach(v) }

func (reachConvertBehavior) ToMap(v Value) (Value, error) {
	info, err := AsReach(&v)
	if err != nil {
		return NewMap(NewOrderedMap()), nil
	}
	out := NewOrderedMap()
	if len(info.Receiver) == 1 {
		out.Set("receiver", info.Receiver[0])
	} else {
		out.Set("receiver", NewList(append([]Value(nil), info.Receiver...)))
	}
	segs := make([]Value, len(info.Segments))
	for i, s := range info.Segments {
		sm := NewOrderedMap()
		op := "get"
		if s.Getr {
			op = "getr"
		}
		sm.Set("op", NewAtom(op))
		sm.Set("computed", NewBoolean(s.Computed))
		if s.Computed {
			sm.Set("key", NewList(append([]Value(nil), s.KeyExpr...)))
		} else {
			sm.Set("key", s.KeyLit)
		}
		segs[i] = NewMap(sm)
	}
	out.Set("segments", NewList(segs))
	return NewMap(out), nil
}

func (reachConvertBehavior) ToList(v Value) (Value, error) {
	info, err := AsReach(&v)
	if err != nil {
		return NewList(nil), nil
	}
	out := make([]Value, 0, len(info.Segments))
	for _, s := range info.Segments {
		if s.Computed {
			out = append(out, NewList(append([]Value(nil), s.KeyExpr...)))
		} else {
			out = append(out, s.KeyLit)
		}
	}
	return NewList(out), nil
}

func init() {
	// Class instances project to maps/lists as flat field maps (class
	// instances have no prototype chain, so the flatten is a single
	// pass). The behavior carries no Sizer, so the SizeOf walk continues
	// past Class to the Ideal root's payload-switch Sizer.
	TClass.ensureTMeta().Behavior = objectConvertBehavior{}
	// Resource/Entity instances share the flat field-map shape, so they
	// reuse the same converter (registered on the TResource root, so
	// Entity inherits it via the Parent-chain Behavior walk).
	TResource.ensureTMeta().Behavior = objectConvertBehavior{}
	TStore.ensureTMeta().Behavior = storeConvertBehavior{}
	TError.ensureTMeta().Behavior = errorConvertBehavior{}
	TReach.ensureTMeta().Behavior = reachConvertBehavior{}
}

// tableConvertBehavior: a Table → its rows (List); ToMap is columnar —
// {field: [value per row]} using the table's record field order.
type tableConvertBehavior struct{}

func (tableConvertBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (tableConvertBehavior) matchesAsDefault()           {}
func (tableConvertBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (tableConvertBehavior) Format(v Value) string       { return kernelFormatDefault(v) }
func (tableConvertBehavior) ToList(v Value) (Value, error) {
	td, ok := v.Data().(TableData)
	if !ok {
		return NewList(nil), nil
	}
	return NewList(append([]Value(nil), td.Rows...)), nil
}
func (tableConvertBehavior) ToMap(v Value) (Value, error) {
	out := NewOrderedMap()
	td, ok := v.Data().(TableData)
	if !ok || td.Record.Fields == nil {
		return NewMap(out), nil
	}
	for _, col := range td.Record.Fields.Keys() {
		vals := make([]Value, 0, len(td.Rows))
		for _, row := range td.Rows {
			rm, err := AsMap(row)
			if err != nil {
				continue
			}
			if cv, ok := rm.Get(col); ok {
				vals = append(vals, cv)
			}
		}
		out.Set(col, NewList(vals))
	}
	return NewMap(out), nil
}

func init() { TTable.ensureTMeta().Behavior = tableConvertBehavior{} }
