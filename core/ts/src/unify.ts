// Unification: the MEET of two values.
//
// Since a type is a value (ADR-010), "does this value have this type", "do
// these two shapes agree" and "is this the same value" are ONE question: do
// the two values unify, and to what. The answer is the more specific of
// the two — a concrete value over the type that admits it, a narrower type
// over a wider one — or nothing when they are incompatible.
//
// This is the TS twin of core/go/unify.go (`unifyInner` and its family
// handlers in unify_list.go, unify_map.go, unify_disjunct.go);
// spec/Boru/Core/Unify.lean states the same rules compactly and this file
// follows it arm for arm. THE ORDER OF THE ARMS IS THE RULE: an earlier arm
// rules the pair before a later one sees it.
//
// What stood here before was a flat predicate (`unifiesValue`, 40 lines in
// coretype.ts) with arms for plain lists, plain maps and scalars and
// NOTHING for a disjunct or a typed container, and no meet at all — so the
// corpus could ask only "did they unify", and 45 rows of
// core/spec/divergent.tsv were that one gap. `unifiesValue` is now this
// unifier's predicate, not a second implementation.
import { simplifyDisjunctAlts, valuesEqual } from "./compare.ts";
import type { Registry } from "./registry.ts";
import { resolveWordsDeep } from "./resolve.ts";
import {
  BoruType,
  TAbsent,
  TAny,
  TList,
  TMap,
  TNever,
  TNode,
  TNone,
  TType,
} from "./type.ts";
import {
  ChildType,
  newDisjunct,
  newList,
  newMap,
  newTypedList,
  newTypedMap,
  newTypeLiteral,
  OptionsData,
  OrderedMap,
  Value,
} from "./value.ts";

/**
 * unify is the meet of two values, or undefined when they have none. The
 * words inside both shapes are resolved first (a bare `Integer` in a list
 * is the type, `true` the boolean) — through `registry` when one is given,
 * so a name bound there resolves too. Go's `UnifyR`.
 */
export function unify(
  a: Value,
  b: Value,
  registry?: Registry,
): Value | undefined {
  return unifyInner(
    resolveWordsDeep(a, registry),
    resolveWordsDeep(b, registry),
  );
}

/** The predicate half: do the two values unify at all. */
export function unifiesValue(a: Value, b: Value, registry?: Registry): boolean {
  return unify(a, b, registry) !== undefined;
}

// ── classification ─────────────────────────────────────────────────────────
//
// The kernel never asks "is the payload nil": it asks a named question
// (core/go/CLAUDE.md, "Payload-presence vs value-mode"). These are those
// questions, for the value kinds core/ts holds.

/** A bare lattice node: a type literal — None, Any and Never included. */
function isBareTypeNode(v: Value): boolean {
  return v.data === null && !v.carrier;
}

/** The node itself, as a bare literal: `List`, not a list. */
function isLiteralOf(v: Value, t: BoruType): boolean {
  return isBareTypeNode(v) && v.vType.equal(t);
}

/** The elements of a plain list (not a typed one), else null. */
function plainList(v: Value): Value[] | null {
  return Array.isArray(v.data) && v.vType.matches(TList)
    ? (v.data as Value[])
    : null;
}

/** The entries of a plain map (not a typed one), else null. */
function plainMap(v: Value): OrderedMap | null {
  return v.data instanceof OrderedMap && v.vType.matches(TMap) ? v.data : null;
}

function isListShape(v: Value): boolean {
  return plainList(v) !== null || v.isTypedList();
}

function isMapShape(v: Value): boolean {
  return plainMap(v) !== null || v.isTypedMap() || v.data instanceof OptionsData;
}

/**
 * A VALUE through and through (Go's `isPlainValue`): concrete, not itself
 * a type — a typed container, a disjunct, a function, a named type, a
 * record shape — and, for a list or a map, holding only such values.
 * `[1 2]` is one; `[Integer]`, `[:Integer]` and a check-mode carrier are
 * not.
 */
function isPlainValue(v: Value): boolean {
  if (!v.isConcrete() || v.typeName !== undefined) return false;
  if (v.data instanceof ChildType || v.vType.matches(TType)) return false;
  const elems = plainList(v);
  if (elems !== null) return elems.every(isPlainValue);
  const m = plainMap(v);
  if (m === null) return true;
  return !m.implicit && m.keys().every((k) => isPlainValue(m.get(k)!));
}

// ── the dispatcher ─────────────────────────────────────────────────────────

/**
 * A degenerate root — Never, None, Absent — unifies only with itself. The
 * concrete inhabitant is the answer when exactly one side has one: the
 * value `none` against the `None` literal is `none`.
 */
function foldRoot(root: BoruType, ruling: Value, other: Value): Value | undefined {
  if (!other.vType.equal(root)) return undefined;
  return other.isConcrete() && !ruling.isConcrete() ? other : ruling;
}

const ROOTS: readonly BoruType[] = [TNever, TNone, TAbsent];

/** The meet of two word-resolved values. */
function unifyInner(a: Value, b: Value): Value | undefined {
  // A disjunct rules first, whichever side it is on — before the
  // degenerate roots, so `String tor None` folds before None's self-only
  // rule sees the pair.
  if (a.isDisjunct()) return unifyDisjunct(a, b);
  if (b.isDisjunct()) return unifyDisjunct(b, a);

  for (const root of ROOTS) {
    for (const [ruling, other] of [[a, b], [b, a]] as const) {
      if (!ruling.vType.equal(root)) continue;
      // None is the one root `Any` admits: `Any` is the top of the VALUE
      // domain and `none` is a value (NUR069). Never and Absent keep the
      // self-only rule — an `Any` field is required, not optional.
      if (root === TNone && other.vType.equal(TAny)) return ruling;
      return foldRoot(root, ruling, other);
    }
  }
  // `Any` yields the other, more specific, side.
  if (a.vType.equal(TAny)) return b;
  if (b.vType.equal(TAny)) return a;

  // The bare `Node` literal is the abstract family root over lists and
  // maps: it admits any container, and a narrower node-family literal.
  const aNode = isLiteralOf(a, TNode);
  const bNode = isLiteralOf(b, TNode);
  if (aNode && bNode) return a;
  if (aNode || bNode) {
    const other = bNode ? a : b;
    if (isListShape(other) || isMapShape(other)) return other;
    return isBareTypeNode(other) && other.vType.matches(TNode)
      ? other
      : undefined;
  }

  // Family handlers: any side in the family routes to its owner.
  const aList = isLiteralOf(a, TList);
  const bList = isLiteralOf(b, TList);
  if (isListShape(a) || isListShape(b) || aList || bList) {
    if (aList) return isListShape(b) || bList ? b : undefined;
    if (bList) return isListShape(a) ? a : undefined;
    return unifyLists(a, b);
  }
  const aMap = isLiteralOf(a, TMap);
  const bMap = isLiteralOf(b, TMap);
  if (isMapShape(a) || isMapShape(b) || aMap || bMap) {
    if (aMap) return isMapShape(b) || bMap ? b : undefined;
    if (bMap) return isMapShape(a) ? a : undefined;
    return unifyMaps(a, b);
  }
  return unifySameOrSubtype(a, b);
}

/**
 * The general narrowing: a type literal admits a concrete value whose type
 * conforms to it; two values of one type are equal or have no meet; of two
 * types on one lattice chain the narrower wins.
 */
function unifySameOrSubtype(a: Value, b: Value): Value | undefined {
  const ta = a.vType;
  const tb = b.vType;
  if (isBareTypeNode(a) && b.data !== null && tb.matches(ta)) return b;
  if (isBareTypeNode(b) && a.data !== null && ta.matches(tb)) return a;
  if (ta.equal(tb)) return valuesEqual(a, b) ? a : undefined;
  if (ta.pathSubtype(tb)) return a;
  if (tb.pathSubtype(ta)) return b;
  return undefined;
}

// ── lists ──────────────────────────────────────────────────────────────────

/** Every element against one child constraint. */
function unifyEach(child: Value, elems: Value[]): Value[] | undefined {
  const out: Value[] = [];
  for (const e of elems) {
    const u = unifyInner(child, e);
    if (u === undefined) return undefined;
    out.push(u);
  }
  return out;
}

/**
 * Two list shapes. Two typed lists unify their CHILD constraints — the
 * retained elements are not part of the type. A typed list against a plain
 * one constrains every element. Two plain lists unify element by element,
 * and only at one length.
 */
function unifyLists(a: Value, b: Value): Value | undefined {
  const al = plainList(a);
  const bl = plainList(b);
  if (a.isTypedList() && b.isTypedList()) {
    const child = unifyInner(a.asChildType().child, b.asChildType().child);
    return child === undefined ? undefined : newTypedList(child);
  }
  if (a.isTypedList() || b.isTypedList()) {
    const [typed, elems] = a.isTypedList() ? [a, bl] : [b, al];
    if (elems === null) return undefined;
    const out = unifyEach(typed.asChildType().child, elems);
    return out === undefined ? undefined : newList(out);
  }
  if (al === null || bl === null || al.length !== bl.length) return undefined;
  const out: Value[] = [];
  for (let i = 0; i < al.length; i++) {
    const u = unifyInner(al[i]!, bl[i]!);
    if (u === undefined) return undefined;
    out.push(u);
  }
  return newList(out);
}

// ── maps ───────────────────────────────────────────────────────────────────

const absent = (): Value => newTypeLiteral(TAbsent);

/**
 * Two map shapes. Typed maps mirror typed lists. Two plain maps are
 * CLOSED: every key of either must unify with the other's value for it, a
 * missing key standing as `Absent` — which only a value admitting `Absent`
 * (the `?:T` desugar) unifies with — and a key whose meet is `Absent` is
 * dropped. Key order follows the left map, then the right map's extras.
 */
function unifyMaps(a: Value, b: Value): Value | undefined {
  const am = plainMap(a);
  const bm = plainMap(b);
  if (a.isTypedMap() && b.isTypedMap()) {
    const child = unifyInner(a.asChildType().child, b.asChildType().child);
    return child === undefined ? undefined : newTypedMap(child);
  }
  if (a.isTypedMap() || b.isTypedMap()) {
    const [typed, m] = a.isTypedMap() ? [a, bm] : [b, am];
    if (m === null) return undefined;
    const child = typed.asChildType().child;
    const out = new OrderedMap();
    for (const k of m.keys()) {
      const u = unifyInner(child, m.get(k)!);
      if (u === undefined) return undefined;
      out.set(k, u);
    }
    return newMap(out);
  }
  // An Options instance is not a plain map: it is itself or nothing.
  if (am === null || bm === null) return unifySameOrSubtype(a, b);
  const out = new OrderedMap();
  const settle = (k: string, mine: Value, theirs: Value | undefined): boolean => {
    const u = unifyInner(mine, theirs ?? absent());
    if (u === undefined) return false;
    if (!u.vType.equal(TAbsent)) out.set(k, u);
    return true;
  };
  for (const k of am.keys()) {
    if (!settle(k, am.get(k)!, bm.get(k))) return undefined;
  }
  for (const k of bm.keys()) {
    if (!am.has(k) && !settle(k, bm.get(k)!, undefined)) return undefined;
  }
  return newMap(out);
}

/**
 * Open map unification: every key the PATTERN names must unify, a key the
 * candidate lacks standing as `Absent`; extra candidate keys pass.
 */
function openUnifyMap(pattern: OrderedMap, candidate: OrderedMap): boolean {
  return pattern
    .keys()
    .every(
      (k) => unifyInner(pattern.get(k)!, candidate.get(k) ?? absent()) !== undefined,
    );
}

// ── disjuncts ──────────────────────────────────────────────────────────────

/**
 * A disjunct against a value. ONE rule: the meet is the disjunct of the
 * meets of its alternatives with that value — the set intersection.
 *
 *   - The EMPTY disjunct denotes the empty set, so it IS `Never`: the pair
 *     is settled as `Never` against the other side.
 *   - Against `Any` the disjunct stands whole.
 *   - Against another DISJUNCT, or against a TYPE — anything that is not a
 *     value through and through — every alternative's meet is gathered
 *     (`unifyMeets`): `5 tor 6` against `Integer` is `5 tor 6`.
 *   - Against a VALUE every alternative that admits it answers that same
 *     value, so the first one that does is the answer.
 *
 * A map alternative is an open PATTERN, not a full schema: it admits a map
 * value carrying its keys (`openUnifyMap`), narrows a map shape carrying
 * them (`narrowMapShape`), and meets another pattern as the two merged
 * (`mergeMapPatterns`).
 */
function unifyDisjunct(disj: Value, val: Value): Value | undefined {
  const alts = disj.asDisjunct().alternatives;
  if (alts.length === 0) return unifyInner(newTypeLiteral(TNever), val);
  if (!val.isConcrete() && val.vType.equal(TAny)) return disj;
  if (val.isDisjunct()) {
    return unifyMeets(alts, val.asDisjunct().alternatives, mergeMapPatterns);
  }
  if (!isPlainValue(val)) return unifyMeets(alts, [val], narrowMapShape);
  const vm = plainMap(val);
  for (const alt of alts) {
    const pattern = plainMap(alt);
    if (pattern !== null && vm !== null) {
      if (openUnifyMap(pattern, vm)) return val;
      continue;
    }
    const u = unifyInner(alt, val);
    if (u !== undefined) return u;
  }
  return undefined;
}

type MapMeet = (alt: OrderedMap, other: OrderedMap) => OrderedMap | undefined;

/**
 * The meet of a disjunct's alternatives with a set of others — a second
 * disjunct's alternatives, or the one type standing against the disjunct:
 * every alternative against every other, the meets that exist gathered
 * into one canonical disjunct. No pair unifying is no meet; exactly one
 * surviving meet is that value, not a one-alternative disjunct. A meet
 * that is itself a disjunct contributes its alternatives.
 */
function unifyMeets(
  alts: Value[],
  others: Value[],
  mapMeet: MapMeet,
): Value | undefined {
  const meets: Value[] = [];
  for (const alt of alts) {
    for (const other of others) {
      const am = plainMap(alt);
      const om = plainMap(other);
      if (am !== null && om !== null) {
        const m = mapMeet(am, om);
        if (m !== undefined) meets.push(newMap(m));
        continue;
      }
      const u = unifyInner(alt, other);
      if (u === undefined) continue;
      if (u.isDisjunct()) meets.push(...u.asDisjunct().alternatives);
      else meets.push(u);
    }
  }
  const out = simplifyDisjunctAlts(meets);
  if (out.length === 0) return undefined;
  return out.length === 1 ? out[0] : newDisjunct(out);
}

/**
 * A map alternative against a map SHAPE — a map that is not a value
 * through and through, `{a:Integer}`. The test is the open one a map value
 * takes; what differs is the answer. A value is already as narrow as it
 * can be; a shape is not, so each key the pattern names is narrowed to the
 * meet of the two: `{a:1}` against `{a:Integer b:String}` is
 * `{a:1 b:String}`.
 */
function narrowMapShape(
  pattern: OrderedMap,
  shape: OrderedMap,
): OrderedMap | undefined {
  const narrowed = new OrderedMap();
  for (const k of shape.keys()) narrowed.set(k, shape.get(k)!);
  for (const k of pattern.keys()) {
    const meet = unifyInner(pattern.get(k)!, shape.get(k) ?? absent());
    if (meet === undefined) return undefined;
    if (shape.has(k)) narrowed.set(k, meet);
  }
  return narrowed;
}

/**
 * The meet of two map ALTERNATIVES. Each is an open pattern — it admits
 * any map carrying its keys — so the maps BOTH admit carry the keys of
 * either: every key of the left, a shared key holding the meet of its two
 * values, then the right's extra keys. A shared key whose values do not
 * unify leaves no map both admit.
 */
function mergeMapPatterns(a: OrderedMap, b: OrderedMap): OrderedMap | undefined {
  const merged = new OrderedMap();
  for (const k of a.keys()) {
    const other = b.get(k);
    const meet = other === undefined ? a.get(k)! : unifyInner(a.get(k)!, other);
    if (meet === undefined) return undefined;
    merged.set(k, meet);
  }
  for (const k of b.keys()) {
    if (!a.has(k)) merged.set(k, b.get(k)!);
  }
  return merged;
}
