package core

import (
	"math"
	"unicode/utf8"
)

// SizeOf reports the natural size of a value. It is a total function
// — every value has a size — so unlike CompareValues it never errors.
//
// Dispatch is type-driven: the size logic lives on the type's
// Behavior (the Sizer capability), reached by walking the parent
// chain so a descendant inherits its branch's Sizer. The kernel
// Sizers follow one rule — a value sizes to the length of the
// collection it stands for: a List's elements, a Map's keys, a
// Pathon's segments, an Object's fields, a Table's rows. A number
// sizes to its floored magnitude, a string or atom to its character
// length. A type with no Sizer in its lattice — None, a Date, a
// bare scalar — sizes 0.
func SizeOf(v Value) int {
	return sizeOfFrom(v.Parent(), v)
}

// sizeOfFrom is SizeOf's walk starting at an arbitrary chain position.
// Split out so a wrapper Behavior that satisfies Sizer structurally but
// has no size rule of its own can CONTINUE the walk above its owning
// node — Sizer has no decline channel (it is total, unlike Comparer),
// so "keep walking" must be expressed by re-entering the walk here.
func sizeOfFrom(t *Type, v Value) int {
	for ; t != nil; t = t.Parent() {
		if sz, ok := t.Behavior().(Sizer); ok {
			return sz.Size(v)
		}
	}
	return 0
}

// SizeOfAbove reports what SizeOf would answer if the walk started
// ABOVE t — the escape hatch for a Sizer-shaped wrapper installed on t
// that has nothing to say for this value (see sizeOfFrom).
func SizeOfAbove(t *Type, v Value) int {
	if t == nil {
		return 0
	}
	return sizeOfFrom(t.Parent(), v)
}

// SizeOwner returns the type node whose Sizer answers SizeOf for a
// value of type t — the nearest ancestor (t included) implementing
// Sizer — or nil when no Sizer exists in the chain (SizeOf reports 0).
// The static analyser uses it to decide whether a fold over the
// PHYSICAL element count is faithful: only when the owner is the
// kernel node itself, not a user type that may have installed its own
// size rule.
func SizeOwner(t *Type) *Type {
	for ; t != nil; t = t.Parent() {
		if _, ok := t.Behavior().(Sizer); ok {
			return t
		}
	}
	return nil
}

// The Size methods below attach the Sizer capability to the kernel
// Behaviors declared in compare_scalar_behaviors.go (the scalar
// branch), coretype_list_map_behaviors.go (List / Map), and
// convert_ideal.go (idealRootBehavior, the Ideal family).
// Gathering them keeps the one size rule auditable in one place.

// Size of a Number is its floored magnitude: an Integer floors to
// itself, a Float drops its fraction (7.9 → 7). A non-finite Float
// (NaN/±Inf) has no integer magnitude — int(math.Floor(NaN)) is
// implementation-defined garbage — so it sizes to 0.
func (numberCompareBehavior) Size(v Value) int {
	n, _ := AsNumber(v)
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	return int(math.Floor(n))
}

// Size of a String is its length in characters (Unicode runes) — the
// ONE character unit every string word counts in (NUR007: size/slice/
// indexof previously counted bytes while the occurrence ops mul/pow
// counted runes). Byte-level work belongs to the Bytes type.
func (stringCompareBehavior) Size(v Value) int {
	s, _ := AsString(v)
	return utf8.RuneCountInString(s)
}

// Size of a Boolean is 1 for true, 0 for false.
func (booleanCompareBehavior) Size(v Value) int {
	if b, _ := AsBoolean(v); b {
		return 1
	}
	return 0
}

// Size of an Atom is the length of its name in characters (runes),
// mirroring the String rule.
func (atomCompareBehavior) Size(v Value) int {
	a, _ := AsAtom(v)
	return utf8.RuneCountInString(a)
}

// Size on the Scalar root is reached only by Pathon — its branch has
// no Sizer of its own — so a Pathon sizes to its segment count, the
// length of its dominant list. Any other value that walks here has
// no size rule and sizes 0.
func (scalarCompareBehavior) Size(v Value) int {
	if p, err := AsPathon(v); err == nil {
		return len(p.Parts)
	}
	return 0
}

// Size of a List is its element count.
func (listFormatBehavior) Size(v Value) int {
	lst, err := RequireConcreteList(v, "size")
	if err != nil {
		return 0
	}
	return lst.Len()
}

// Size of a Map is its key count.
func (mapFormatBehavior) Size(v Value) int {
	m, err := RequireConcreteMap(v, "size")
	if err != nil {
		return 0
	}
	return m.Len()
}

// The Ideal family's Sizer rides idealRootBehavior (convert_ideal.go —
// the ONE Ideal-root Behavior; NUR017). The SizeOf walk reaches it for
// any Ideal-family instance whose own type has no Sizer. Each kind
// sizes to its member count: an Object's fields, a Store's entries, a
// Table's rows. Record instances are field-maps and size via the Map
// Sizer instead.
func (idealRootBehavior) Size(v Value) int {
	switch d := v.Data().(type) {
	case ClassInstanceInfo:
		if d.Fields != nil {
			return d.Fields.Len()
		}
	case ResourceInstanceInfo:
		if d.Fields != nil {
			return d.Fields.Len()
		}
	case *StoreInstanceInfo:
		if d != nil {
			// The VISIBLE keyset, walking the prototype chain with masking
			// and tombstones — the same rule Get applies, so `size` counts
			// the keys the store answers for (NUR052). `len(d.Data)` read
			// only the newest copy-on-write layer, so two `context set`s
			// left two keys reachable by get/has and a size of 1.
			return len(d.VisibleKeys())
		}
	case TableData:
		return len(d.Rows)
	}
	return 0
}
