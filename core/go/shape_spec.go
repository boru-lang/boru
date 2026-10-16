package core

import (
	"fmt"
	"strconv"
	"strings"
)

// Shape specialisation (check's call-site specialisation over a Map arg): a
// fn called with a plain Map compiles a unit whose Map param is a STRICT
// record carrier — ShapeCarrier — whose schema is the arg's exact key set
// and each value's tag. Field reads of it are then strict (a record carrier
// is otherwise always gradual), so the body's reads and the dispatches over
// them commit. The unit is valid only for a Map of that exact shape; the VM
// checks ShapeHolds at the unit's entry and runs the fn itself otherwise.
// A plain Map is copy-on-write (`set` returns a new map), so a shape that
// holds at entry holds for the frame.

// ShapeOf returns the shape carrier for v when v is a plain Map (tag Map,
// an ordinary map payload) of 1..maxFields entries, each value concrete and
// tag-keyable (TagKeyable) and not a fn value. A carrier field is refused:
// its tag bounds the runtime value's rather than equalling it (a String
// carrier holds a ProperString at run time), and the guard compares tags
// exactly — a subtype could pick a more specific overload than the one the
// specialised body committed.
func ShapeOf(v Value, maxFields int) (Value, bool) {
	if ParentOf(&v) != TMap || IsDynamic(&v) || v.AscribedType() != nil {
		return Value{}, false
	}
	mp, ok := DataOf(&v).(MapPayload)
	if !ok || mp.M == nil || mp.M.Len() == 0 || mp.M.Len() > maxFields {
		return Value{}, false
	}
	fields := NewOrderedMap()
	for _, k := range mp.M.Keys() {
		fv, _ := mp.M.Get(k)
		if !shapeData(fv) {
			return Value{}, false
		}
		fields.Set(k, NewCarrier(ParentOf(&fv)))
	}
	return Value{c: &content{parent: TMap, carrier: true, data: RecordTypeInfo{Fields: fields}}, occ: idOcc(GenerateID(IDPrefixForType(TMap)))}, true
}

// IsShapeCarrier reports whether v is a shape carrier: a STRICT record
// carrier (every other record carrier is gradual).
func IsShapeCarrier(v Value) bool {
	rt, ok := DataOf(&v).(RecordTypeInfo)
	return ok && rt.Fields != nil && IsCarrier(&v) && !IsDynamic(&v) && ParentOf(&v) == TMap
}

// ShapeHolds reports whether the runtime value v has shape's exact key set,
// each value tag-keyable and carrying the recorded tag.
func ShapeHolds(shape, v Value) bool {
	rt, ok := DataOf(&shape).(RecordTypeInfo)
	if !ok || rt.Fields == nil || ParentOf(&v) != TMap || IsCarrier(&v) {
		return false
	}
	mp, ok := DataOf(&v).(MapPayload)
	if !ok || mp.M == nil || mp.M.Len() != rt.Fields.Len() {
		return false
	}
	for _, k := range rt.Fields.Keys() {
		want, _ := rt.Fields.Get(k)
		got, ok := mp.M.Get(k)
		if !ok || !TagKeyable(got) || ParentOf(&got) != ParentOf(&want) {
			return false
		}
	}
	return true
}

// ShapeKey renders a shape carrier's schema for a specialisation key:
// INJECTIVE, since two shapes with one key share one compiled unit — each
// field name quoted (so `{"a:Integer b": …}` cannot read as two fields)
// and each tag by its node's identity, not its display name (two scopes can
// mint distinct types of one name).
func ShapeKey(shape Value) string {
	rt, ok := DataOf(&shape).(RecordTypeInfo)
	if !ok || rt.Fields == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("{")
	for _, k := range rt.Fields.Keys() {
		fv, _ := rt.Fields.Get(k)
		fmt.Fprintf(&b, "%s:%p ", strconv.Quote(k), ParentOf(&fv))
	}
	b.WriteString("}")
	return b.String()
}

// ListShapeOf is ShapeOf for a plain List (tag List, an ordinary list
// payload) of 1..maxElems concrete, tag-keyable, non-fn elements: the shape
// is the list itself with each element replaced by a strict carrier of its
// tag — a concrete list of carriers, which the check pass already reads
// precisely (`xs get 0` is the element's carrier, `size xs` its length).
// A List is copy-on-write too, so the shape holds for the frame.
func ListShapeOf(v Value, maxElems int) (Value, bool) {
	if ParentOf(&v) != TList || IsDynamic(&v) || IsCarrier(&v) || IsQuoted(&v) || v.AscribedType() != nil {
		return Value{}, false
	}
	lp, ok := DataOf(&v).(ListPayload)
	if !ok || len(lp.Elems) == 0 || len(lp.Elems) > maxElems {
		return Value{}, false
	}
	elems := make([]Value, len(lp.Elems))
	for i, e := range lp.Elems {
		if !shapeData(e) {
			return Value{}, false
		}
		elems[i] = NewCarrier(ParentOf(&e))
	}
	return NewList(elems), true
}

// IsListShapeGuard reports whether v is a list shape (ListShapeOf): a plain,
// non-carrier List whose every element is a strict carrier — a value no
// runtime computation produces (runtime lists hold concrete values).
func IsListShapeGuard(v Value) bool {
	lp, ok := DataOf(&v).(ListPayload)
	if !ok || ParentOf(&v) != TList || IsCarrier(&v) || len(lp.Elems) == 0 {
		return false
	}
	for _, e := range lp.Elems {
		if !IsCarrier(&e) || IsDynamic(&e) {
			return false
		}
	}
	return true
}

// ListShapeHolds reports whether the runtime value v is a plain List of the
// shape's exact length whose elements carry the shape's tags.
func ListShapeHolds(shape, v Value) bool {
	sp, ok := DataOf(&shape).(ListPayload)
	if !ok || ParentOf(&v) != TList || IsCarrier(&v) {
		return false
	}
	lp, ok := DataOf(&v).(ListPayload)
	if !ok || len(lp.Elems) != len(sp.Elems) {
		return false
	}
	for i, e := range lp.Elems {
		if !TagKeyable(e) || ParentOf(&e) != ParentOf(&sp.Elems[i]) {
			return false
		}
	}
	return true
}

// ListShapeKey renders a list shape for a specialisation key, each element
// tag by its node's identity (ShapeKey's rule).
func ListShapeKey(shape Value) string {
	sp, _ := DataOf(&shape).(ListPayload)
	var b strings.Builder
	b.WriteString("[")
	for _, e := range sp.Elems {
		fmt.Fprintf(&b, "%p ", ParentOf(&e))
	}
	b.WriteString("]")
	return b.String()
}

// shapeData reports whether v may key a shape: a tag-keyable plain DATA
// value — a scalar, a plain List or Map, or none. Behaviour-bearing values
// (a fn, a reach `$.a`, a store, a module, any Ideal) are refused: a
// strict carrier of one commits dispatches (`apply` over a reach) the
// interpreter resolves by the value.
func shapeData(v Value) bool {
	if !TagKeyable(v) {
		return false
	}
	return ParentOf(&v).ConformsTo(TScalar) || ParentOf(&v) == TList || ParentOf(&v) == TMap || ParentOf(&v) == TNone
}
