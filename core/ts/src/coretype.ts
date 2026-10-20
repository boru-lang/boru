// Type-introspection words ported from eng/go/core_type.go:
// typeof, pathof, and the IsValueOfType conformance check behind `is`.
//
// PARITY NOTE: this is the subset the eng/spec corpus reaches through
// the current (no-parser) TS runner — bare type literals and concrete
// scalars/lists. The typed-list / typed-map / record-shape branches of
// IsValueOfType (which need ChildType payloads) and the Disjunct/Enum
// membership branches are added by their owning port increments.
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
 * IsValueOfType reports whether v satisfies type t. Subset of
 * core_type.go::IsValueOfType:
 *   - t is the bare metatype Type: v is itself a type;
 *   - t is any other bare type literal: lattice subtyping
 *     (v.VType conforms to t.VType);
 *   - otherwise: structural identity on the carried types (best effort).
 */
/**
 * The symmetric unifier's PREDICATE half: do these two values unify?
 *
 * EXPORTED so core/spec can reach it. The corpus had no expression that
 * touched unification at all — its fixture registry calls it from nowhere,
 * and unify's real clients (`is`, `case`, typed defs, record shapes) are
 * all basic/lang-layer words — so the most fundamental thing core does
 * after dispatch was invisible to the very instrument built to compare the
 * two ports. `unifyq` in the three corpus runners is the door; this is
 * what it opens.
 *
 * Go's twin is `UnifyR(a, b, r) (Value, bool)` and returns the unified
 * VALUE as well. This one cannot: core/ts has a predicate, not a meet, so
 * there is no intersection to hand back. That asymmetry is deliberate and
 * recorded rather than papered over with a fake meet — the corpus asks
 * only the question both engines can currently answer, and the value half
 * lands with the real port.
 *
 * unifiesValue mirrors Go's symmetric Unify over the fixture's value
 * subset — the path IsValueOfType falls to for a concrete RHS and for
 * disjunct alternatives (unify.go::unifyInner). A bare type literal on
 * either side admits a conforming partner (`Integer is 5`, `[3] is
 * [Integer]`); lists unify element-wise; concrete maps unify with
 * EXACT key sets (the open subset pattern applies only at
 * IsValueOfType's record-shape arm and unifyDisjunct's immediate map
 * alternative, never to nested maps); scalar leaves need compatible
 * types and equal payloads.
 */
export function unifiesValue(a: Value, b: Value): boolean {
  if (a.data === null || b.data === null) {
    return a.vType.matches(b.vType) || b.vType.matches(a.vType);
  }
  const ae = listElements(a);
  const be = listElements(b);
  if (ae !== null && be !== null) {
    return (
      ae.length === be.length && ae.every((x, i) => unifiesValue(x, be[i]!))
    );
  }
  if (ae !== null || be !== null) return false;
  // `isMap()` tests the lattice node only; a TYPED map carries a ChildType
  // payload, not an OrderedMap, so `asMap()` on one threw a raw
  // `AsMap: not a map value` — an uncoded host exception escaping
  // `Engine.run` with no `boru/` code at all. Worse than a wrong answer,
  // and the class the crossdiff treats most leniently: a GAP is logged and
  // permitted, so nine such rows would have gone by unreported. Two
  // independent sweeps found it — `unifyq { a: 1 } {: Integer }` in the
  // core corpus and nine `is` shapes at the engine level.
  //
  // Gating on the PAYLOAD rather than the type keeps the arm honest: a
  // typed map falls through to the leaf comparison below rather than
  // pretending to be a plain one. Unifying a typed map properly needs the
  // child-constraint arm core/ts does not have yet, which is tracked as
  // debt rather than faked here.
  if (a.data instanceof OrderedMap && b.data instanceof OrderedMap) {
    const am = a.asMap();
    const bm = b.asMap();
    const ak = am.sortedKeys();
    const bk = bm.sortedKeys();
    return (
      ak.length === bk.length &&
      ak.every((k, i) => k === bk[i] && unifiesValue(am.get(k)!, bm.get(k)!))
    );
  }
  // Scalars / atoms: compatible leaves and equal payload.
  if (!(a.vType.matches(b.vType) || b.vType.matches(a.vType))) return false;
  if (typeof a.data === "bigint" && typeof b.data === "bigint")
    return a.data === b.data;
  return a.data === b.data;
}

/** Extract a list's elements whether it is a plain or a typed list. */
function listElements(v: Value): Value[] | null {
  if (Array.isArray(v.data)) return v.data as Value[];
  if (v.isTypedList()) return v.asChildType().elements;
  return null;
}

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
    if (!v.isMap()) return false;
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
  // Disjunct / Enum membership, mirroring unify_disjunct.go: v
  // satisfies the union if some alternative unifies with it — a
  // type-literal alternative is a subtype check; a concrete map
  // alternative is an OPEN pattern (subset matching); any other
  // concrete alternative admits an equal value OR a bare-literal
  // candidate the alternative's type conforms to (`Integer is E`
  // holds for `enum [1 2]` because 1 unifies with Integer).
  if (t.isDisjunct()) {
    if (v.data === null && !v.carrier && v.vType.equal(TAny)) return true;
    for (const alt of t.asDisjunct().alternatives) {
      if (alt.data === null) {
        if (isValueOfType(v, alt)) return true;
        continue;
      }
      if (alt.data instanceof OrderedMap && v.data instanceof OrderedMap) {
        const am = alt.asMap();
        const vm = v.asMap();
        if (
          am
            .keys()
            .every((k) => vm.has(k) && unifiesValue(vm.get(k)!, am.get(k)!))
        )
          return true;
        continue;
      }
      if (unifiesValue(v, alt)) return true;
    }
    return false;
  }
  if (isBareTypeNode(t)) {
    if (t.vType.equal(TType)) {
      if (v.carrier) return false;
      return isBareTypeNode(v) || isTypeBody(v);
    }
    return v.vType.matches(t.vType);
  }
  // Concrete RHS that is not a type — including a concrete list
  // template `[A B …]` — routes through the symmetric unify walk, like
  // Go's terminal `Unify(v, t)`: `5 is 5` → true, `Integer is 5` →
  // true, `[3 2] is [Integer 2]` → true.
  return unifiesValue(v, t);
}
