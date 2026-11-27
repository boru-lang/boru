// Type-introspection words ported from eng/go/core_type.go:
// typeof, pathof, and the IsValueOfType conformance check behind `is`.
//
// PARITY NOTE: IsValueOfType carries the shape arms whose subset rule is
// its own — typed list, typed map, record shape, the bare metatype — and
// hands everything else (a disjunct or enum, a concrete value, a list
// template) to the unifier in unify.ts, as Go's does to `Unify`. The
// symmetric predicate `unifiesValue` lived here as a flat function with
// its own disjunct arm; it is the unifier's predicate now.
import {
  BoruType,
  TAny,
  TBoolean,
  TInteger,
  TList,
  TMap,
  TNone,
  TNumber,
  TString,
  TType,
} from "./type.ts";
import { valToString } from "./canon.ts";
import { unifiesValue } from "./unify.ts";
import { newList, newTypeLiteral, OrderedMap, Value } from "./value.ts";

/**
 * coerceBoolean reduces any value to a truthiness, mirroring
 * eng/go/core_helpers.go::CoerceBoolean: none/empty-string/empty-
 * container/zero are falsy; everything else is truthy.
 */
export function coerceBoolean(v: Value): boolean {
  if (v.isNone()) return false;
  // A bare type literal (or other non-concrete value) is falsy.
  if (v.data === null) return false;
  const t = v.vType;
  if (t.matches(TBoolean)) return v.asBoolean();
  if (t.matches(TInteger)) return v.asInteger() !== 0n;
  if (t.matches(TNumber)) return v.asFloat() !== 0;
  if (t.equal(TList)) {
    if (Array.isArray(v.data)) return v.data.length > 0;
    if (v.isTypedList()) return v.asChildType().elements.length > 0;
    return false;
  }
  if (t.equal(TMap)) {
    if (v.data instanceof OrderedMap) return v.data.size > 0;
    return false;
  }
  if (t.matches(TString)) return v.asString().length > 0;
  // A non-String that RENDERS as "false" is an unresolved boolean literal
  // reaching truthiness as a Word or Atom — a bare `false` condition in
  // `if [false …]`, a quoted `false` atom. It keeps its boolean reading;
  // everything else is truthy unless it renders empty. Mirrors the tail of
  // Go's CoerceBoolean, and its absence here is what made a clause-list
  // `if [ false [1] [2] ]` take the THEN branch.
  const text = valToString(v);
  return text !== "false" && text !== "";
}

// Degenerate roots saturate under typeof (they are their own parent).
const SATURATING_ROOTS = new Set(["Any", "None", "Never", "Absent"]);

/**
 * parentType returns the lattice parent of a type, mirroring the
 * Go Type.Parent chain:
 *   - a multi-part path drops its last segment (Scalar/Number/Integer →
 *     Scalar/Number);
 *   - a branch root (Scalar/Node/Ideal/Word/Type) has parent Any;
 *   - a degenerate root (Any/None/Never/Absent) has no parent (null).
 */
export function parentType(t: BoruType): BoruType | null {
  if (t.parts.length > 1) {
    return new BoruType(t.parts.slice(0, -1));
  }
  if (SATURATING_ROOTS.has(t.parts[0]!)) return null;
  return TAny;
}

/**
 * TypeOf returns the type of v as a type-literal Value — a single
 * parent hop up the unified lattice. A concrete value yields its own
 * VType (`typeof 5 → Integer`); a type literal yields the lattice
 * parent of its VType (`typeof Integer → Number`), with the degenerate
 * roots saturating (`typeof None → None`). Mirrors core_type.go::TypeOf
 * where, because Type = Value, a type literal's Parent IS its lattice
 * parent.
 */
export function typeOf(v: Value): Value {
  // The none value's type is the None type literal.
  if (v.isNone()) return newTypeLiteral(TNone);
  if (v.data === null) {
    const p = parentType(v.vType);
    return newTypeLiteral(p ?? v.vType);
  }
  return newTypeLiteral(v.vType);
}

/**
 * PathOf returns a type's ancestry as a List of type literals, root
 * first, leaf last: `pathof Integer → [Scalar Number Integer]`,
 * `pathof Type → [Type]`, `pathof None → [None]`. Mirrors
 * core_type.go::PathOf (Any is never an explicit ancestor segment
 * because the lattice paths here are already rooted at the branch
 * root, not at Any).
 */
export function pathOf(t: Value): Value {
  const parts = t.vType.parts;
  const elems: Value[] = [];
  for (let i = 0; i < parts.length; i++) {
    elems.push(newTypeLiteral(new BoruType(parts.slice(0, i + 1))));
  }
  return newList(elems, { eval: false });
}

/** A bare type node: a type literal with no concrete payload (incl. None/Any/Never). */
function isBareTypeNode(v: Value): boolean {
  return v.data === null && !v.carrier;
}

/**
 * isRecordShape: a non-empty concrete map whose every value is a type
 * body (type literal or nested record shape). Mirrors
 * core_type.go::IsRecordShape.
 */
export function isRecordShape(v: Value): boolean {
  if (!v.isMap()) return false;
  const m = v.asMap();
  if (m.size === 0) return false;
  return m.keys().every((k) => {
    const fv = m.get(k)!;
    return fv.data === null || isRecordShape(fv);
  });
}

/** isTypeBody: a structural type value (typed container, disjunct, record shape, fn). */
export function isTypeBody(v: Value): boolean {
  if (v.isTypedList() || v.isTypedMap() || v.isDisjunct()) return true;
  if (v.data !== null && v.vType.matches(TType)) return true;
  return isRecordShape(v);
}

/**
 * IsValueOfType reports whether v satisfies type t — the membership
 * question `is` asks. Mirrors core_type.go::IsValueOfType:
 *   - t is a typed list / typed map: v is a concrete container whose every
 *     member satisfies the child;
 *   - t is a record shape (a map): v is a concrete map carrying every
 *     declared key with a satisfying value — extra keys are allowed;
 *   - t is the bare metatype Type: v is itself a type;
 *   - t is any other bare type literal: lattice subtyping;
 *   - otherwise — a disjunct, a concrete value, a list template — v and t
 *     UNIFY (unify.ts). There is one unifier; this predicate adds only the
 *     shape arms above, whose subset rule is asymmetric where unification
 *     is not.
 */
export function isValueOfType(v: Value, t: Value): boolean {
  // A named type IS a type (Value.typeName) whatever its body's kind:
  // `def N 5  N is Type` holds — the singleton's members are still just 5.
  if (v.typeName !== undefined && t.isTypeLiteral() && t.vType.equal(TType)) return true;
  // Typed list `[:T]`: v must be a CONCRETE list whose every element
  // satisfies T. A plain list (even empty) is concrete; a typed list is
  // concrete only when it carries retained elements — a bare typed-list
  // carrier (`[:Integer]`, no elements) is a type, not a value, so it
  // does not conform. Mirrors the Go IsConcrete guard.
  if (t.isTypedList()) {
    let elems: Value[] | null = null;
    if (Array.isArray(v.data)) elems = v.data as Value[];
    else if (v.isTypedList() && v.asChildType().elements.length > 0)
      elems = v.asChildType().elements;
    if (elems === null || !v.vType.equal(t.vType)) return false;
    const child = t.asChildType().child;
    return elems.every((e) => isValueOfType(e, child));
  }
  // Typed map `{:T}`: v must be a concrete map whose every value
  // satisfies T. A typed-map VALUE (`{:T a:1 b:2}` — ChildType data
  // with inline entries) validates its entries the same way.
  if (t.isTypedMap()) {
    if (!v.isMap() || !v.isConcrete()) return false;
    const child = t.asChildType().child;
    if (v.isTypedMap()) {
      return v
        .asChildType()
        .entries.every((e) => isValueOfType(e.value, child));
    }
    return v
      .asMap()
      .keys()
      .every((k) => isValueOfType(v.asMap().get(k)!, child));
  }
  // Record-shape / structural map type: every key declared in t must
  // be present in v with a conforming value (extra v keys are allowed).
  if (t.data instanceof OrderedMap) {
    if (!(v.data instanceof OrderedMap)) return false;
    const tm = t.asMap();
    const vm = v.asMap();
    return tm
      .keys()
      .every((k) => vm.has(k) && isValueOfType(vm.get(k)!, tm.get(k)!));
  }
  if (isBareTypeNode(t)) {
    if (t.vType.equal(TType)) {
      if (v.carrier) return false;
      return isBareTypeNode(v) || isTypeBody(v);
    }
    return v.vType.matches(t.vType);
  }
  // Everything else — a disjunct or enum, a concrete value, a list
  // template `[A B …]` — is Go's terminal `Unify(v, t)`: `5 is 5`,
  // `Integer is 5`, `[3 2] is [Integer 2]`, `5 is (Integer tor String)`.
  return unifiesValue(v, t);
}
