// Bare-name type resolution — the canonical cascade of ADR-012 rule 4,
// mirroring eng/go/resolve.go. A capitalised name arriving as a Word
// resolves in exactly two arms, in order: (1) the registry's binding
// store (user types / dynamic bindings), then (2) the live builtin
// name table. Sites with a Registry run both arms; registry-free sites
// run arm 2 only. Do NOT re-implement either arm inline — that is how
// the Go per-site cascades drifted apart (NUR059).

import {
  disjunctAltsCanonical,
  simplifyDisjunctAlts,
  valuesEqual,
} from "./compare.ts";
import type { Registry } from "./registry.ts";
import { resolveBuiltinTypeName, TDisjunct, TList, TMap } from "./type.ts";
import {
  ChildType,
  type DisjunctInfo,
  OrderedMap,
  Value,
  newAtom,
  newBoolean,
  newList,
  newMap,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
} from "./value.ts";

/**
 * Resolve a scalar Word to its semantic value: true/false/none
 * keywords, a bound def, a builtin type literal, else an Atom of the
 * same name. Mirrors Go resolveWordValue.
 */
export function resolveWordValue(v: Value, registry?: Registry): Value {
  if (!v.isWord()) return v;
  const name = v.asWord().name;
  if (name === "true") return newBoolean(true);
  if (name === "false") return newBoolean(false);
  if (registry) {
    const top = registry.topOfDefStack(name);
    if (top !== undefined) return top;
  }
  // The builtin arm of the cascade: the leaf-name table, then the slash
  // path (`Scalar/String`) — one resolver, the one stepWord consults.
  const t = resolveBuiltinTypeName(name);
  if (t !== undefined) return newTypeLiteral(t);
  return newAtom(name);
}

/**
 * Recursively resolve word values to their semantic form: lists
 * element-wise, map values, typed-container children, disjunct
 * alternatives. Mirrors Go ResolveWordsDeep / ResolveWordsDeepR.
 */
export function resolveWordsDeep(v: Value, registry?: Registry): Value {
  if (v.isWord()) return resolveWordValue(v, registry);
  if (v.data instanceof ChildType) {
    const ct = v.data;
    const child = resolveWordsDeep(ct.child, registry);
    if (v.vType.equal(TMap)) {
      return newTypedMap(
        child,
        ct.entries.map((e) => ({
          key: e.key,
          value: resolveWordsDeep(e.value, registry),
        })),
      );
    }
    return newTypedList(
      child,
      ct.elements.map((e) => resolveWordsDeep(e, registry)),
    );
  }
  if (v.vType.equal(TList) && Array.isArray(v.data)) {
    const resolved = (v.data as Value[]).map((e) =>
      resolveWordsDeep(e, registry),
    );
    return newList(resolved, { eval: v.eval, quoted: v.quoted });
  }
  if (v.vType.equal(TMap) && v.data instanceof OrderedMap) {
    const m = v.data;
    const out = new OrderedMap();
    for (const k of m.keys()) {
      out.set(k, resolveWordsDeep(m.get(k)!, registry));
    }
    return newMap(out);
  }
  if (v.isDisjunct()) {
    const before = v.asDisjunct().alternatives;
    const alts = before.map((a) => resolveWordsDeep(a, registry));
    // A plain disjunct is a SET: it is reduced whenever it is not already
    // in canonical form, not only when resolution happened to rebuild an
    // alternative. An Enum keeps the order it was declared in, so it is
    // re-simplified only when an alternative changed. Go's rule exactly
    // (core/go/resolve.go, NUR385).
    const changed = alts.some((a, i) => !sameAlternative(before[i]!, a));
    if (
      !changed &&
      (!v.vType.equal(TDisjunct) || disjunctAltsCanonical(before))
    ) {
      return v;
    }
    // Only the payload moves: the type (an Enum stays an Enum), the name a
    // `def` gave it and its flags are the value's own.
    const out = new Value(
      v.vType,
      { alternatives: simplifyDisjunctAlts(alts) } satisfies DisjunctInfo,
      { quoted: v.quoted, carrier: v.carrier, dynamic: v.dynamic },
    );
    out.pos = v.pos;
    out.typeName = v.typeName;
    return out;
  }
  return v;
}

/**
 * Did resolution leave this alternative as it was? Go asks ExactEqual
 * (core/go/compare.go): a scalar or a type is itself by VALUE, a list or a
 * map only by IDENTITY — and resolution always rebuilds a container, so a
 * map or a non-empty list alternative always counts as changed. The EMPTY
 * list is the exception: it has no backing store to be identical by, so
 * every empty list is the one empty list.
 */
function sameAlternative(before: Value, after: Value): boolean {
  if (before === after) return true;
  if (after.data instanceof OrderedMap) return false;
  if (Array.isArray(after.data)) return after.data.length === 0;
  return before.vType.equal(after.vType) && valuesEqual(before, after);
}
