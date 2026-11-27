// The one total order, value equality, and a disjunct's canonical
// alternatives — the TS twin of core/go/compare.go (`CompareValues`),
// core/go/equal.go (`ValuesEqual`) and core/go/core_helpers.go
// (`SimplifyDisjunctAlts`). spec/Boru/Core/Compare.lean states the same
// three rules compactly, and this file follows it arm for arm.
//
// core/ts had none of them. Its only ordering was eng/ts's `tor` handler,
// which sorts the alternatives it builds, and its only reduction was a
// flatten-and-dedupe by canonical text in resolve.ts — so a disjunct the
// engine merely RESOLVED kept the order and the repeats it was written
// with (`5 tor 5 tor Integer`), where Go reduces every plain disjunct to
// one canonical form (`Integer tor 5`).
import { canonValue } from "./canon.ts";
import { Decimal } from "./decimal.ts";
import {
  BoruType,
  compareText,
  compareTypes,
  TAtom,
  TBigInteger,
  TBoolean,
  TList,
  TMap,
  TNever,
  TNumber,
  TScalar,
  TString,
  TWord,
} from "./type.ts";
import { ChildType, OrderedMap, Value } from "./value.ts";

/** A bare lattice node: a type literal — None, Any and Never included. */
function isBareTypeNode(v: Value): boolean {
  return v.data === null && !v.carrier;
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

// ── numbers ────────────────────────────────────────────────────────────────

/** An exact rational: numerator over a positive denominator. */
type Ratio = [bigint, bigint];

/** The exact value of a FINITE binary64. */
function floatRatio(f: number): Ratio {
  const view = new DataView(new ArrayBuffer(8));
  view.setFloat64(0, f);
  const bits = view.getBigUint64(0);
  const exp = Number((bits >> 52n) & 0x7ffn);
  const frac = bits & 0xfffffffffffffn;
  // A subnormal has no implicit leading bit and the minimum exponent.
  const mant = exp === 0 ? frac : frac | (1n << 52n);
  const e = (exp === 0 ? 1 : exp) - 1075;
  const signed = bits >> 63n === 1n ? -mant : mant;
  return e >= 0 ? [signed << BigInt(e), 1n] : [signed, 1n << BigInt(-e)];
}

function decimalRatio(d: Decimal): Ratio {
  return d.scale <= 0
    ? [d.unscaled * 10n ** BigInt(-d.scale), 1n]
    : [d.unscaled, 10n ** BigInt(d.scale)];
}

function sign(n: bigint): number {
  return n < 0n ? -1 : n > 0n ? 1 : 0;
}

type NumKind = "int" | "float" | "bigint" | "bigdec";

/** Which Number leaf a concrete value is, read off its payload. */
function numKind(v: Value): NumKind | undefined {
  if (typeof v.data === "number") return "float";
  if (v.data instanceof Decimal) return "bigdec";
  if (typeof v.data !== "bigint") return undefined;
  return v.vType.matches(TBigInteger) ? "bigint" : "int";
}

function numRatio(v: Value, kind: NumKind): Ratio {
  if (kind === "float") return floatRatio(v.data as number);
  if (kind === "bigdec") return decimalRatio(v.data as Decimal);
  return [v.data as bigint, 1n];
}

/** Negative zero sorts just below positive zero. Only a Float has one. */
function signedZeroOrder(a: Value, b: Value): number {
  const an = Object.is(a.data, -0);
  const bn = Object.is(b.data, -0);
  return an === bn ? 0 : an ? -1 : 1;
}

/** Floats by magnitude: NaN above everything, and equal to itself. */
function compareFloats(x: number, y: number): number {
  if (Number.isNaN(x) || Number.isNaN(y)) {
    return Number.isNaN(x) ? (Number.isNaN(y) ? 0 : 1) : -1;
  }
  return x < y ? -1 : x > y ? 1 : 0;
}

/**
 * Numbers across leaves, by magnitude. `1` and `1.0` are EQUAL here — the
 * one deliberate value-level equivalence in the order. Two Integers compare
 * exactly; an Integer against a Float compares as floats (the Integer read
 * as the nearest one); a pair with an arbitrary-precision side compares as
 * exact rationals, a non-finite Float taking the extreme slot.
 */
function compareNumbers(a: Value, b: Value, ka: NumKind, kb: NumKind): number {
  const big = (k: NumKind): boolean => k === "bigint" || k === "bigdec";
  if (big(ka) || big(kb)) {
    for (const [v, k, flip] of [[a, ka, 1], [b, kb, -1]] as const) {
      if (k !== "float") continue;
      const f = v.data as number;
      if (Number.isNaN(f)) return flip;
      if (!Number.isFinite(f)) return f < 0 ? -flip : flip;
    }
    const [an, ad] = numRatio(a, ka);
    const [bn, bd] = numRatio(b, kb);
    const c = sign(an * bd - bn * ad);
    return c !== 0 ? c : signedZeroOrder(a, b);
  }
  if (ka === "int" && kb === "int") {
    return sign((a.data as bigint) - (b.data as bigint));
  }
  const c = compareFloats(Number(a.data), Number(b.data));
  return c !== 0 ? c : signedZeroOrder(a, b);
}

// ── the order ──────────────────────────────────────────────────────────────

/** The flexible container types order as their base family. */
function nodeFamily(t: BoruType): BoruType {
  const p = t.toString();
  if (p === "Node/Map/FlexMap" || p === "Node/Map/FlexMap/WeakFlexMap") {
    return TMap;
  }
  if (p === "Node/List/FlexList" || p === "Node/List/FlexList/WeakFlexList") {
    return TList;
  }
  return t;
}

/** The type a value is ordered at: its family when concrete, itself else. */
function orderType(v: Value): BoruType {
  return v.isConcrete() ? nodeFamily(v.vType) : v.vType;
}

const FAMILIES: readonly BoruType[] = [TNumber, TString, TBoolean, TAtom];

/**
 * The family that owns the comparison of two types, if any: the nearest of
 * Number, String, Boolean, Atom, Scalar and Word on the walk up from their
 * lowest common ancestor. `TScalar` is the cross-family catch-all — two
 * scalars of different families order by rank.
 */
function comparerFamily(a: BoruType, b: BoruType): BoruType | undefined {
  for (const fam of FAMILIES) {
    if (a.matches(fam) && b.matches(fam)) return fam;
  }
  if (a.pathSubtype(TScalar) && b.pathSubtype(TScalar)) return TScalar;
  if (a.pathSubtype(TWord) && b.pathSubtype(TWord)) return TWord;
  return undefined;
}

/**
 * compareValues is the total order: every pair of values is ordered,
 * whatever their types. The answer comes from a cascade —
 *
 *   1. the nearest common ancestor that owns a FAMILY comparison decides:
 *      numbers by magnitude across leaves, strings and atoms by text,
 *      booleans false-first, two scalars of different families by rank;
 *   2. with no family comparison, the lattice RANK of the two types
 *      decides (then depth, then name);
 *   3. two values of one type compare by size, then structurally — lists
 *      element by element, maps by sorted keys then values, anything else
 *      by canonical text.
 *
 * One rule runs through every family: a bare TYPE sorts before any
 * concrete value of that family, and two bare types sort by rank.
 */
export function compareValues(a: Value, b: Value): number {
  const ta = orderType(a);
  const tb = orderType(b);
  const fam = comparerFamily(ta, tb);
  if (fam === TScalar) return compareTypes(a.vType, b.vType);
  if (fam !== undefined) {
    const al = isBareTypeNode(a);
    const bl = isBareTypeNode(b);
    if (al !== bl) return al ? -1 : 1;
    // Two types — or two carriers, which have no payload to read either —
    // order as their lattice nodes do.
    if (a.data === null && b.data === null) {
      return compareTypes(a.vType, b.vType);
    }
    if (fam === TNumber) {
      const ka = numKind(a);
      const kb = numKind(b);
      if (ka !== undefined && kb !== undefined) {
        return compareNumbers(a, b, ka, kb);
      }
    } else if (typeof a.data === "string" && typeof b.data === "string") {
      // Strings and atoms, by text.
      return compareText(a.data, b.data);
    } else if (typeof a.data === "boolean" && typeof b.data === "boolean") {
      return Number(a.data) - Number(b.data);
    }
    return compareText(canonValue(a), canonValue(b));
  }
  const byType = compareTypes(ta, tb);
  if (byType !== 0) return byType;
  const bySize = sizeOf(a) - sizeOf(b);
  if (bySize !== 0) return bySize < 0 ? -1 : 1;
  const al = plainList(a);
  const bl = plainList(b);
  if (al !== null && bl !== null) {
    for (let i = 0; i < al.length; i++) {
      const c = compareValues(al[i]!, bl[i]!);
      if (c !== 0) return c;
    }
    return 0;
  }
  const am = plainMap(a);
  const bm = plainMap(b);
  if (am !== null && bm !== null) {
    // The key lists, in sorted order, compare first; only two maps with
    // one key set compare by the values under those keys.
    const ak = am.keys().sort(compareText);
    const bk = bm.keys().sort(compareText);
    for (let i = 0; i < ak.length; i++) {
      const c = compareText(ak[i]!, bk[i]!);
      if (c !== 0) return c;
    }
    for (const k of ak) {
      const c = compareValues(am.get(k)!, bm.get(k)!);
      if (c !== 0) return c;
    }
    return 0;
  }
  return compareText(canonValue(a), canonValue(b));
}

/** The size a container is first ordered by; zero for everything else. */
function sizeOf(v: Value): number {
  if (v.data instanceof ChildType) {
    return v.data.elements.length + v.data.entries.length;
  }
  return plainList(v)?.length ?? plainMap(v)?.size ?? 0;
}

// ── equality ───────────────────────────────────────────────────────────────

/**
 * valuesEqual is VALUE equality — what a disjunct's alternatives are
 * deduplicated by, and what two same-typed values must have to unify.
 * Structural: lists element by element, maps key by key whatever their
 * insertion order. A function is equal only to itself: two structurally
 * identical fn literals are two functions.
 *
 * It does NOT cross types — the Integer 1 and the Float 1.0 are two
 * values (the order ties them; equality does not), and so are the atom
 * `a` and the string 'a'.
 *
 * Two TYPED containers are equal when their child constraints are: what
 * each RETAINS is not compared, exactly as it is not part of the type when
 * two of them unify. That is core/go's ValuesEqual, and it has a visible
 * edge — `[:Integer 1]` and `[:Integer 2]` are one alternative, so
 * `d( [: Integer 1 ] [: Integer 2 ] )` reduces to `[:Integer 1]`. The
 * Lean model (spec/Boru/Core/Compare.lean) compares the retained members
 * too and keeps both; no corpus row sits on the difference.
 *
 * Two DISJUNCTS are equal when their alternatives PRINT alike, in order.
 * That too is core/go's rule — it has no arm for a disjunct and compares
 * the two payloads as Value.String renders them (toString here) — and it
 * is coarser than structure in one direction and finer in the other:
 * `d( d( 1 ) )` equals `d( 1 )`, and two functions of the same parameter
 * types are one alternative once each is nested in a disjunct, while two
 * maps that differ only in key order are two, and an unresolved word is
 * not the value it will resolve to. It is reached only for a disjunct
 * held INSIDE another value (a nested alternative, a list element); the
 * Lean model compares those by structure.
 */
export function valuesEqual(a: Value, b: Value): boolean {
  if (a.data === null || b.data === null) {
    if (a.data !== b.data) return false;
    // Two carriers are abstract, and conservatively equal; two type
    // literals are equal iff they are one lattice node.
    return a.carrier || b.carrier || a.vType.equal(b.vType);
  }
  if (!nodeFamily(a.vType).equal(nodeFamily(b.vType))) return false;
  if (typeof a.data === "number") {
    // A Float is its bit pattern: NaN is the same value as itself, and
    // -0.0 and 0.0 are two. (`===` says the opposite of both.)
    return Object.is(a.data, b.data);
  }
  if (a.data === b.data) return true;
  const al = plainList(a);
  const bl = plainList(b);
  if (al !== null && bl !== null) return listsEqual(al, bl);
  const am = plainMap(a);
  const bm = plainMap(b);
  if (am !== null && bm !== null) {
    if (am.size !== bm.size) return false;
    return am.keys().every((k) => {
      const bv = bm.get(k);
      return bv !== undefined && valuesEqual(am.get(k)!, bv);
    });
  }
  if (a.data instanceof ChildType && b.data instanceof ChildType) {
    return valuesEqual(a.data.child, b.data.child);
  }
  if (a.isDisjunct() && b.isDisjunct()) {
    return disjunctText(a) === disjunctText(b);
  }
  // A structured scalar (a BigDecimal, a path) and a word are the value
  // their canonical text spells. Everything else left — a function, a
  // class type, an error — has identity, not structure, and the
  // `a.data === b.data` test above has already said these are two.
  if (typeof a.data !== "object" || typeof b.data !== "object") return false;
  if (!a.vType.pathSubtype(TScalar) && !a.vType.pathSubtype(TWord)) return false;
  return canonValue(a) === canonValue(b);
}

/** A disjunct as its alternatives print, in order — its equality key. */
function disjunctText(v: Value): string {
  return v.asDisjunct().alternatives.map(String).join(" ");
}

function listsEqual(a: Value[], b: Value[]): boolean {
  return a.length === b.length && a.every((x, i) => valuesEqual(x, b[i]!));
}

// ── a disjunct's canonical alternatives ────────────────────────────────────

/** The canonical order of two alternatives: the total order, ties by text. */
function compareAlternatives(a: Value, b: Value): number {
  const c = compareValues(a, b);
  return c !== 0 ? c : compareText(canonValue(a), canonValue(b));
}

/**
 * simplifyDisjunctAlts reduces a disjunct's alternatives to their canonical
 * form — a disjunct is a SET, and this is the one spelling of it:
 *
 *   - a `Never` alternative goes (it adds nothing to a union);
 *   - a repeat of an earlier alternative goes, by value equality — two maps
 *     are one alternative whatever their key order;
 *   - an alternative ABSORBED by a strictly wider bare type goes — `5` and
 *     `Integer` under `Number`, never the reverse;
 *   - what is left is ordered by the total order, ties by canonical text.
 *
 * A nested disjunct stays ONE alternative: it is not flattened here.
 */
export function simplifyDisjunctAlts(alts: Value[]): Value[] {
  const live = alts.filter((v) => !v.vType.equal(TNever));
  const kept = live.filter((cand, i) => {
    const ct = cand.vType;
    for (let j = 0; j < i; j++) {
      if (live[j]!.vType.equal(ct) && valuesEqual(live[j]!, cand)) return false;
    }
    // Only a type or a concrete value is absorbed; a check-mode carrier
    // stands for a value nobody has seen and is kept.
    if (!isBareTypeNode(cand) && !cand.isConcrete()) return true;
    return !live.some(
      (other, j) =>
        j !== i &&
        isBareTypeNode(other) &&
        !ct.equal(other.vType) &&
        ct.matches(other.vType),
    );
  });
  // A stable INSERTION sort, written out rather than Array.prototype.sort:
  // it is the algorithm Go's sort.SliceStable runs below twenty elements.
  // Over a transitive order every stable sort agrees and the choice would
  // not matter — but this order is not quite one. A typed list against a
  // plain list of the same size falls to canonical text while two plain
  // lists compare member by member, and the two can disagree in a cycle:
  // `[foo/q]` < `[(1)]` < `[:Any 0]` < `[foo/q]`. On such alternatives the
  // result depends on the algorithm, so the port runs Go's.
  for (let i = 1; i < kept.length; i++) {
    for (let j = i; j > 0 && compareAlternatives(kept[j]!, kept[j - 1]!) < 0; j--) {
      [kept[j - 1], kept[j]] = [kept[j]!, kept[j - 1]!];
    }
  }
  return kept;
}

/**
 * disjunctAltsCanonical reports whether `alts` is already what
 * simplifyDisjunctAlts would return — nothing to drop, nothing out of
 * order — so a resolver can hand back the value it was given.
 */
export function disjunctAltsCanonical(alts: Value[]): boolean {
  const out = simplifyDisjunctAlts(alts);
  return out.length === alts.length && out.every((v, i) => v === alts[i]);
}
