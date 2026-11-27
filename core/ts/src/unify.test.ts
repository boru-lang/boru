// Unit campaign for unify.ts — the meet of two values.
//
// core/spec/unify.tsv holds all three runners to the unifier through the
// fixture words `unifyq` and `meetq`, and reaches every arm the corpus
// notation can spell. This file is the API-level half:
//
//   - the TS twin of core/go/disjunct_pair_test.go — the same tables, the
//     same expected values, so the two ports are held to one statement of
//     the disjunct rules at the function they each export;
//   - the shapes the notation cannot write: a check-mode carrier, an
//     Options instance, a named type, a registry-bound name;
//   - what the membership predicate `isValueOfType` now gets from the
//     unifier rather than from arms of its own.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { canonValue } from "./canon.ts";
import { isValueOfType } from "./coretype.ts";
import { Registry } from "./registry.ts";
import {
  TAbsent,
  TAny,
  TEnum,
  TInteger,
  TMap,
  TNever,
  TNode,
  TNone,
  TNumber,
  TScalar,
  TString,
  TType,
} from "./type.ts";
import { unifiesValue, unify } from "./unify.ts";
import {
  OrderedMap,
  Value,
  newAny,
  newAtom,
  newCarrier,
  newDisjunct,
  newDynamicCarrier,
  newEnum,
  newFnDef,
  newInteger,
  newList,
  newMap,
  newNone,
  newOptions,
  newString,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
  newWord,
} from "./value.ts";

const lit = newTypeLiteral;
const int = (n: number): Value => newInteger(BigInt(n));
const d = (...alts: Value[]): Value => newDisjunct(alts);
const l = (...elems: Value[]): Value => newList(elems);

function pairMap(...kv: Array<string | Value>): Value {
  const m = new OrderedMap();
  for (let i = 0; i < kv.length; i += 2) m.set(kv[i] as string, kv[i + 1] as Value);
  return newMap(m);
}

/** The canonical text of the meet, or undefined when there is none. */
function pairCanon(a: Value, b: Value): string | undefined {
  const meet = unify(a, b);
  return meet === undefined ? undefined : canonValue(meet);
}

interface Case {
  name: string;
  a: Value;
  b: Value;
  /** The meet's canonical text; undefined when the two do not unify. */
  want: string | undefined;
}

/** Each case holds its value, and the PREDICATE is symmetric. */
function check(cases: Case[]): void {
  for (const c of cases) {
    assert.equal(pairCanon(c.a, c.b), c.want, c.name);
    assert.equal(unifiesValue(c.b, c.a), c.want !== undefined, `${c.name} (swapped)`);
  }
}

const intT = lit(TInteger);
const strT = lit(TString);
const numT = lit(TNumber);
const [i1, i2, i3, i5, i6, i7] = [1, 2, 3, 5, 6, 7].map(int) as [
  Value,
  Value,
  Value,
  Value,
  Value,
  Value,
];

describe("a disjunct against a disjunct — the pairwise meet (NUR386)", () => {
  it("is the disjunct of the meets of the alternatives", () => {
    check([
      // a disjunct against itself is itself — not its first member
      { name: "types, self", a: d(intT, strT), b: d(intT, strT), want: "Integer tor String" },
      { name: "values, self", a: d(i5, i6), b: d(i5, i6), want: "5 tor 6" },
      // the order the alternatives were written in decides nothing
      { name: "types, reordered", a: d(strT, intT), b: d(intT, strT), want: "Integer tor String" },
      // one surviving meet is that value, not a one-alternative disjunct
      { name: "overlap of one", a: d(i5, i6), b: d(i6, i7), want: "6" },
      // a type against the values it admits narrows to the values
      { name: "type narrows to values", a: d(intT, strT), b: d(i5, i6), want: "5 tor 6" },
      // a meet absorbed by a wider meet is dropped
      { name: "absorbed meet", a: d(numT, strT), b: d(intT, numT), want: "Number" },
      // no pair unifying is a failure
      { name: "disjoint", a: d(i5, i6), b: d(i7, strT), want: undefined },
      // a nested disjunct's meet is flattened into the result
      { name: "nested", a: d(d(i5, i6), strT), b: d(i5, i6), want: "5 tor 6" },
    ]);
  });

  it("merges two map alternatives as open patterns", () => {
    check([
      // two patterns: the maps both admit carry the keys of either
      { name: "subset", a: d(pairMap("a", i1)), b: d(pairMap("a", i1, "b", i2)), want: "{a:1 b:2}" },
      { name: "superset", a: d(pairMap("a", i1, "b", i2)), b: d(pairMap("a", i1)), want: "{a:1 b:2}" },
      { name: "disjoint keys", a: d(pairMap("a", i1)), b: d(pairMap("b", i2)), want: "{a:1 b:2}" },
      // a shared key holds the meet of its two values
      { name: "shared key narrows", a: d(pairMap("a", intT)), b: d(pairMap("a", i3)), want: "{a:3}" },
      // a shared key whose values do not unify admits no map at all
      { name: "shared key clashes", a: d(pairMap("a", i1)), b: d(pairMap("a", i2)), want: undefined },
      // a map alternative and a non-map alternative never meet
      { name: "map against scalar", a: d(pairMap("a", i1)), b: d(i1, i2), want: undefined },
    ]);
  });
});

describe("the empty disjunct is Never", () => {
  it("in every pair", () => {
    const empty = d();
    const never = lit(TNever);
    check([
      { name: "itself", a: empty, b: empty, want: "Never" },
      { name: "Never", a: empty, b: never, want: "Never" },
      { name: "Never, swapped", a: never, b: empty, want: "Never" },
      // as `Never` unifies with nothing else — `Any` included
      { name: "Any", a: empty, b: lit(TAny), want: undefined },
      { name: "a value", a: empty, b: i5, want: undefined },
      { name: "a type", a: empty, b: lit(TScalar), want: undefined },
      { name: "a disjunct", a: empty, b: d(i5), want: undefined },
    ]);
  });
});

describe("a disjunct's meet distributes over its alternatives", () => {
  it("a TYPE narrows every alternative; a VALUE is that value", () => {
    check([
      { name: "type over values", a: d(i5, i6), b: intT, want: "5 tor 6" },
      { name: "type over values, swapped", a: intT, b: d(i5, i6), want: "5 tor 6" },
      { name: "wider type keeps the disjunct", a: d(intT, strT), b: lit(TScalar), want: "Integer tor String" },
      { name: "type admitting one alternative", a: d(intT, strT), b: numT, want: "Integer" },
      { name: "type admitting none", a: d(i5, i6), b: strT, want: undefined },
      // a list holding a type is a shape, not a value
      { name: "list shape", a: d(l(i5), l(i6)), b: l(intT), want: "[5] tor [6]" },
      // a typed container is a type
      { name: "typed list", a: d(l(i5), l(newString("a"))), b: newTypedList(intT), want: "[5]" },
      // a value is admitted by the first alternative that takes it
      { name: "value by type", a: d(intT, strT), b: i5, want: "5" },
      { name: "value by value", a: d(i5, i6), b: i6, want: "6" },
      { name: "list value", a: d(l(intT), l(strT)), b: l(i5), want: "[5]" },
      { name: "value refused", a: d(i5, i6), b: i7, want: undefined },
    ]);
  });

  it("a check-mode carrier is no value: the types that admit it answer", () => {
    // The carrier stands for an Integer nobody has seen. A type over it
    // admits it; a concrete alternative cannot know it is that one.
    check([
      { name: "carrier", a: d(numT, strT), b: newCarrier(TInteger), want: "Integer" },
      { name: "carrier against values", a: d(i5, i6), b: newCarrier(TInteger), want: undefined },
    ]);
    const meet = unify(d(numT, strT), newCarrier(TInteger))!;
    assert.equal(meet.carrier, true);
  });

  it("a map alternative is an open pattern against a value and a shape", () => {
    check([
      // a value: the answer is the value, extra keys and all
      {
        name: "value with extra key",
        a: d(pairMap("a", i1), pairMap("a", i2)),
        b: pairMap("a", i1, "b", i2),
        want: "{a:1 b:2}",
      },
      { name: "value lacking the key", a: d(pairMap("a", i1)), b: pairMap("b", i2), want: undefined },
      // a shape: every alternative that fits narrows it
      {
        name: "shape narrowed by each",
        a: d(pairMap("a", i1), pairMap("a", i2)),
        b: pairMap("a", intT),
        want: "{a:1} tor {a:2}",
      },
      {
        name: "shape keeps its other keys",
        a: d(pairMap("a", i1)),
        b: pairMap("a", intT, "b", strT),
        want: "{a:1 b:String}",
      },
      { name: "shape lacking the key", a: d(pairMap("a", i1)), b: pairMap("b", intT), want: undefined },
      { name: "shape refusing the key", a: d(pairMap("a", i1)), b: pairMap("a", strT), want: undefined },
      // a map holding a list that holds a type is a shape too
      { name: "nested shape", a: d(pairMap("a", l(i1))), b: pairMap("a", l(intT)), want: "{a:[1]}" },
    ]);
  });

  it("treats a record shape and a named type as types, not values", () => {
    // An IMPLICIT map is the pair-syntax record shape: a type body, so a
    // map alternative narrows it rather than testing it as a value.
    const shape = new OrderedMap();
    shape.set("a", i1);
    shape.implicit = true;
    assert.equal(pairCanon(d(pairMap("a", i1)), newMap(shape)), "{a:1}");
    // A named singleton (`def One 1`) is a type whose body is the value.
    const one = i1.named("One");
    assert.equal(unify(d(intT, strT), one), one);
  });
});

describe("the degenerate roots and Any", () => {
  it("a root unifies only with itself, the concrete inhabitant winning", () => {
    const none = newNone();
    assert.equal(unify(lit(TNone), none), none);
    assert.equal(unify(none, lit(TNone)), none);
    assert.equal(pairCanon(lit(TNone), lit(TNone)), "None");
    assert.equal(unify(none, i5), undefined);
    assert.equal(unify(lit(TNever), i5), undefined);
    assert.equal(pairCanon(lit(TNever), lit(TNever)), "Never");
  });

  it("Any yields the other side — the none value included, Never not", () => {
    assert.equal(unify(lit(TAny), i5), i5);
    assert.equal(unify(i5, lit(TAny)), i5);
    const none = newNone();
    assert.equal(unify(lit(TAny), none), none);
    assert.equal(unify(none, lit(TAny)), none);
    assert.equal(unify(lit(TAny), lit(TNever)), undefined);
    // A gradual carrier and an Any-typed value are `Any` too.
    assert.equal(unify(newDynamicCarrier(TAny), i5), i5);
    assert.equal(unify(newAny("opaque"), i5), i5);
  });

  it("against Any a disjunct stands whole — name and kind kept", () => {
    const e = newEnum([newAtom("red"), newAtom("blue")]);
    assert.equal(unify(e, lit(TAny)), e);
    assert.equal(unify(newDynamicCarrier(TAny), e), e);
    const named = d(i1, i2).named("Small");
    assert.equal(unify(lit(TAny), named), named);
    assert.equal(named.vType.equal(TEnum), false);
  });
});

describe("carriers, by the general narrowing", () => {
  it("unify with a type on their chain, and with each other", () => {
    const c = newCarrier(TInteger);
    assert.equal(unify(c, numT), c); // the narrower side
    assert.equal(unify(numT, c), c);
    assert.equal(unify(c, newCarrier(TInteger)), c);
    assert.equal(unify(c, intT), c);
    assert.equal(unify(c, strT), undefined);
    // A wider carrier admits a narrower value; a same-typed one cannot
    // know it is that value.
    assert.equal(unify(newCarrier(TNumber), i5), i5);
    assert.equal(unify(c, i5), undefined);
  });
});

describe("containers", () => {
  it("the Node and Map literals admit an Options instance; it is itself only", () => {
    const m = new OrderedMap();
    m.set("a", i1);
    const opts = newOptions(m);
    assert.equal(unify(lit(TMap), opts), opts);
    assert.equal(unify(opts, lit(TMap)), opts);
    assert.equal(unify(lit(TNode), opts), opts);
    assert.equal(unify(opts, opts), opts);
    assert.equal(unify(opts, newOptions(m)), undefined);
    assert.equal(unify(opts, pairMap("a", i1)), undefined);
    assert.equal(unify(newTypedMap(intT), opts), undefined);
  });

  it("a typed container against a non-container has no meet", () => {
    assert.equal(unify(newTypedList(intT), i5), undefined);
    assert.equal(unify(i5, newTypedMap(intT)), undefined);
    assert.equal(unify(l(i1), pairMap("a", i1)), undefined);
    assert.equal(unify(lit(TNode), i5), undefined);
    assert.equal(unify(lit(TNode), intT), undefined);
  });

  it("unifying a map with ITSELF keeps every key", () => {
    const m = pairMap("a", i1, "b", i2);
    assert.equal(pairCanon(m, m), "{a:1 b:2}");
  });

  it("a function unifies only with itself", () => {
    const sig = { params: [], returns: [], body: [] };
    const f = newFnDef({ sigs: [sig] });
    assert.equal(unify(f, f), f);
    assert.equal(unify(f, newFnDef({ sigs: [sig] })), undefined);
    // The bare metatype admits it, as any type admits a value of it.
    assert.equal(unify(lit(TType), f), f);
  });
});

describe("word resolution ahead of the meet", () => {
  it("resolves the words inside both shapes", () => {
    assert.equal(pairCanon(l(newWord("Integer"), newWord("true")), l(i5, newWord("Boolean"))), "[5 true]");
    // An unbound name is an atom, and an atom is itself only.
    assert.equal(pairCanon(l(newWord("foo")), l(newWord("foo"))), "[foo/q]");
    assert.equal(unify(l(newWord("foo")), l(i5)), undefined);
  });

  it("consults the registry when one is given", () => {
    const r = new Registry();
    r.pushDef("Foo", intT);
    const pattern = l(newWord("Foo"));
    assert.equal(pairCanon(pattern, l(i5)), undefined);
    assert.equal(canonValue(unify(pattern, l(i5), r)!), "[5]");
    assert.equal(unifiesValue(pattern, l(i5), r), true);
    assert.equal(unifiesValue(pattern, l(newString("a")), r), false);
  });
});

describe("isValueOfType, on the unifier", () => {
  it("answers a disjunct by unification", () => {
    const u = d(intT, strT);
    assert.equal(isValueOfType(i5, u), true);
    assert.equal(isValueOfType(l(i5), u), false);
    // The empty disjunct is Never: nothing is a member, `Any` included.
    assert.equal(isValueOfType(lit(TAny), d()), false);
    assert.equal(isValueOfType(lit(TAny), u), true);
    // A nested disjunct alternative is walked.
    assert.equal(isValueOfType(i5, d(d(intT), strT)), true);
    // A missing key satisfies a map alternative only where the pattern's
    // value admits Absent — the `?:T` desugar.
    const optional = d(pairMap("a", d(intT, lit(TNone), lit(TAbsent))));
    assert.equal(isValueOfType(pairMap("b", i1), optional), true);
    assert.equal(isValueOfType(pairMap("b", i1), d(pairMap("a", intT))), false);
  });

  it("refuses a non-concrete map against a typed map instead of throwing", () => {
    // `Map is {:Integer}` reached asMap() on a payload-free literal and
    // died in an uncoded `AsMap: not a map value`.
    assert.equal(isValueOfType(lit(TMap), newTypedMap(intT)), false);
    assert.equal(isValueOfType(newCarrier(TMap), newTypedMap(intT)), false);
    assert.equal(isValueOfType(pairMap("a", i1), newTypedMap(intT)), true);
  });
});
