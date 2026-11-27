// Unit campaign for compare.ts — the total order, value equality and a
// disjunct's canonical alternatives.
//
// core/spec/data.tsv pins the order through every shape the corpus notation
// can write, on all three runners. What it cannot write is a Float, an
// arbitrary-precision number, a check-mode carrier or a flex container, and
// those are the arms here. Every numeric expectation below was read off
// core/go first — `boru do '(1.5 tor 1 tor 0d2 …)'` prints a disjunct in
// exactly this order — so the rows state the shared contract and not this
// port's own behaviour.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { canonValue } from "./canon.ts";
import {
  compareValues,
  disjunctAltsCanonical,
  simplifyDisjunctAlts,
  valuesEqual,
} from "./compare.ts";
import { decimalFromString } from "./decimal.ts";
import {
  newType,
  TFloat,
  TFunction,
  TInteger,
  TList,
  TMap,
  TNever,
  TNumber,
  TString,
} from "./type.ts";
import {
  OrderedMap,
  Value,
  newAtom,
  newBigDecimal,
  newBigInteger,
  newBoolean,
  newCarrier,
  newDisjunct,
  newEnum,
  newErrorValue,
  newFloat,
  newFnDef,
  newInteger,
  newList,
  newMap,
  newNone,
  newPath,
  newString,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
  newWord,
} from "./value.ts";

const int = (n: number | bigint): Value => newInteger(BigInt(n));
const flt = (f: number): Value => newFloat(f);
const big = (s: string): Value => newBigInteger(BigInt(s));
const dec = (s: string): Value => newBigDecimal(decimalFromString(s)!);
const lit = newTypeLiteral;

function mapOf(entries: Array<[string, Value]>): Value {
  const m = new OrderedMap();
  for (const [k, v] of entries) m.set(k, v);
  return newMap(m);
}

/** The canonical disjunct of `alts`, rendered as Go's `tor` prints it. */
function canonical(alts: Value[]): string {
  return simplifyDisjunctAlts(alts).map(canonValue).join(" tor ");
}

describe("the total order: numbers across leaves", () => {
  it("orders every numeric leaf by magnitude, as core/go does", () => {
    assert.equal(
      canonical([
        flt(1.5),
        int(1),
        big("2"),
        dec("1.25"),
        flt(NaN),
        flt(Infinity),
        flt(-Infinity),
        flt(-0),
        int(0),
        flt(0),
        big("0"),
      ]),
      "-inf tor -0.0 tor 0 tor 0.0 tor 0d0 tor 1 tor 0d1.25 tor 1.5 tor 0d2 tor inf tor nan",
    );
  });

  it("separates magnitude-equal leaves by canonical text", () => {
    // `1`, `1.0`, `0d1` and `0d1.0` are EQUAL in the order — the one
    // deliberate value-level equivalence — and four distinct values.
    assert.equal(
      canonical([int(1), flt(1), big("1"), dec("1.0")]),
      "0d1 tor 0d1.0 tor 1 tor 1.0",
    );
    assert.equal(compareValues(int(1), flt(1)), 0);
    assert.equal(compareValues(dec("1.50"), dec("1.5")), 0);
    assert.equal(canonical([dec("1.50"), dec("1.5")]), "0d1.5 tor 0d1.50");
  });

  it("compares an Integer and a Float as floats", () => {
    // 2^53 + 1 has no binary64: read as the nearest float it IS 2^53, so
    // the pair ties and the text decides.
    assert.equal(compareValues(int(9007199254740993n), flt(9007199254740992)), 0);
    assert.equal(
      canonical([int(9007199254740993n), flt(9007199254740992)]),
      "9007199254740992.0 tor 9007199254740993",
    );
    assert.equal(compareValues(int(2), flt(2.5)), -1);
    assert.equal(compareValues(flt(2.5), int(2)), 1);
    assert.equal(compareValues(int(3), int(2)), 1);
  });

  it("compares an arbitrary-precision number EXACTLY", () => {
    // Against a BigInteger the same float is strictly smaller: no rounding.
    assert.equal(
      compareValues(flt(9007199254740992), big("9007199254740993")),
      -1,
    );
    assert.equal(
      canonical([
        big("9007199254740993"),
        flt(9007199254740992),
        int(9007199254740993n),
      ]),
      "9007199254740992.0 tor 0d9007199254740993 tor 9007199254740993",
    );
    // The float 0.1 is a hair ABOVE one tenth; the decimal is exactly it.
    assert.equal(canonical([flt(0.1), dec("0.1")]), "0d0.1 tor 0.1");
    // A float with a positive binary exponent, a subnormal, and a decimal
    // with a negative scale all have exact rationals too.
    assert.equal(compareValues(flt(1e300), big("1")), 1);
    assert.equal(compareValues(flt(5e-324), big("0")), 1);
    assert.equal(compareValues(dec("1e2"), int(100)), 0);
    assert.equal(compareValues(dec("-2.5"), flt(-2.5)), 0);
  });

  it("gives a non-finite Float the extreme slot against a big number", () => {
    assert.equal(canonical([flt(NaN), big("5")]), "0d5 tor nan");
    assert.equal(compareValues(big("5"), flt(NaN)), -1);
    assert.equal(canonical([flt(Infinity), big("5")]), "0d5 tor inf");
    assert.equal(compareValues(big("5"), flt(Infinity)), -1);
    assert.equal(canonical([big("-5"), flt(-Infinity)]), "-inf tor -0d5");
    assert.equal(compareValues(big("-5"), flt(-Infinity)), 1);
  });

  it("sorts negative zero just below positive zero, big numbers included", () => {
    assert.equal(canonical([flt(0), flt(-0), int(0)]), "-0.0 tor 0 tor 0.0");
    assert.equal(canonical([big("0"), flt(-0)]), "-0.0 tor 0d0");
    assert.equal(compareValues(int(0), flt(-0)), 1);
  });

  it("holds NaN above every float and equal to itself", () => {
    assert.equal(compareValues(flt(NaN), flt(NaN)), 0);
    assert.equal(compareValues(flt(NaN), flt(Infinity)), 1);
    assert.equal(compareValues(flt(1), flt(NaN)), -1);
  });
});

describe("the total order: types, carriers and families", () => {
  it("sorts a bare type before any value of its family, types by rank", () => {
    assert.equal(
      canonical([lit(TFloat), flt(1.5), lit(TInteger), int(1)]),
      "Integer tor Float tor 1 tor 1.5",
    );
    // `Number` is strictly wider than both values and absorbs them.
    assert.equal(canonical([lit(TNumber), flt(1.5), int(1)]), "Number");
  });

  it("orders two carriers as their lattice nodes, after a bare type", () => {
    assert.equal(compareValues(newCarrier(TInteger), newCarrier(TFloat)), -1);
    assert.equal(compareValues(lit(TFloat), newCarrier(TInteger)), -1);
    assert.equal(compareValues(newCarrier(TInteger), lit(TFloat)), 1);
    // A carrier has no payload to read against a value: the text decides.
    assert.equal(compareValues(newCarrier(TInteger), int(5)), 1);
  });

  it("orders two scalars of different families by rank", () => {
    assert.equal(compareValues(newBoolean(true), int(0)), -1);
    assert.equal(compareValues(newString("a"), int(9)), 1);
    assert.equal(compareValues(newAtom("z"), newBoolean(false)), -1);
    // Two values of one structured-scalar type tie; the text decides.
    assert.equal(
      canonical([newPath(["b"], false), newPath(["a"], false)]),
      "a tor b",
    );
  });

  it("orders strings and atoms by code point, booleans false first", () => {
    assert.equal(compareValues(newString("B"), newString("a")), -1);
    // An astral character sorts AFTER every BMP one, as in UTF-8.
    assert.equal(compareValues(newString("\u{1F600}"), newString("�")), 1);
    assert.equal(compareValues(newAtom("a"), newAtom("b")), -1);
    assert.equal(compareValues(newBoolean(false), newBoolean(true)), -1);
  });

  it("orders words by their canonical text", () => {
    assert.equal(compareValues(newWord("a"), newWord("b")), -1);
    assert.equal(compareValues(newWord("b"), newWord("a")), 1);
  });

  it("orders a flex container as its family, a flex TYPE as itself", () => {
    const flexList = new Value(newType("Node/List/FlexList"), [int(1)]);
    const weakList = new Value(newType("Node/List/FlexList/WeakFlexList"), [int(1)]);
    assert.equal(compareValues(flexList, newList([int(1)])), 0);
    assert.equal(compareValues(weakList, newList([int(2)])), -1);
    const m = new OrderedMap();
    m.set("a", int(1));
    const flexMap = new Value(newType("Node/Map/FlexMap"), m);
    const weakMap = new Value(newType("Node/Map/FlexMap/WeakFlexMap"), m);
    assert.equal(compareValues(flexMap, mapOf([["a", int(1)]])), 0);
    assert.equal(compareValues(weakMap, mapOf([["a", int(0)]])), 1);
    // The bare literals are two lattice nodes and order by rank.
    assert.equal(compareValues(lit(TList), lit(newType("Node/List/FlexList"))), -1);
    assert.equal(valuesEqual(flexList, newList([int(1)])), true);
  });

  it("orders two maps of one key set by the values under the sorted keys", () => {
    const a = mapOf([["b", int(1)], ["a", int(2)]]);
    const b = mapOf([["a", int(2)], ["b", int(2)]]);
    assert.equal(compareValues(a, b), -1);
    assert.equal(compareValues(b, a), 1);
    assert.equal(compareValues(a, mapOf([["a", int(2)], ["b", int(1)]])), 0);
  });

  it("orders a typed container by what it retains, then by text", () => {
    const one = newTypedList(lit(TInteger), [int(1)]);
    assert.equal(compareValues(newTypedList(lit(TInteger)), one), -1);
    const tm = newTypedMap(lit(TInteger), [{ key: "a", value: int(1) }]);
    assert.equal(compareValues(newTypedMap(lit(TInteger)), tm), -1);
    // At one size a typed and a plain list fall to their text: `[1]`
    // sorts before `[:Integer 1]`.
    assert.equal(compareValues(one, newList([int(1)])), 1);
  });
});

describe("valuesEqual", () => {
  it("compares type literals by node and never with a value", () => {
    assert.equal(valuesEqual(lit(TInteger), lit(TInteger)), true);
    assert.equal(valuesEqual(lit(TInteger), lit(TString)), false);
    assert.equal(valuesEqual(lit(TInteger), int(1)), false);
    assert.equal(valuesEqual(int(1), lit(TInteger)), false);
    // Carriers are abstract: conservatively equal.
    assert.equal(valuesEqual(newCarrier(TInteger), newCarrier(TInteger)), true);
    assert.equal(valuesEqual(newCarrier(TInteger), lit(TInteger)), true);
  });

  it("does not cross types", () => {
    assert.equal(valuesEqual(int(1), flt(1)), false);
    assert.equal(valuesEqual(newAtom("a"), newString("a")), false);
    assert.equal(valuesEqual(newString(""), newString("a")), false);
    assert.equal(valuesEqual(int(1), big("1")), false);
  });

  it("reads a Float by its bits", () => {
    assert.equal(valuesEqual(flt(NaN), flt(NaN)), true);
    assert.equal(valuesEqual(flt(-0), flt(0)), false);
    assert.equal(valuesEqual(flt(1.5), flt(1.5)), true);
  });

  it("compares scalars by payload", () => {
    assert.equal(valuesEqual(int(5), int(5)), true);
    assert.equal(valuesEqual(int(5), int(6)), false);
    assert.equal(valuesEqual(newString("a"), newString("a")), true);
    assert.equal(valuesEqual(newNone(), newNone()), true);
    assert.equal(valuesEqual(big("7"), big("7")), true);
    assert.equal(valuesEqual(big("7"), big("8")), false);
  });

  it("compares lists element by element and maps key by key", () => {
    assert.equal(valuesEqual(newList([int(1), int(2)]), newList([int(1), int(2)])), true);
    assert.equal(valuesEqual(newList([int(1)]), newList([int(1), int(2)])), false);
    assert.equal(valuesEqual(newList([int(1)]), newList([flt(1)])), false);
    const ab = mapOf([["a", int(1)], ["b", int(2)]]);
    const ba = mapOf([["b", int(2)], ["a", int(1)]]);
    assert.equal(valuesEqual(ab, ba), true);
    assert.equal(valuesEqual(ab, mapOf([["a", int(1)]])), false);
    assert.equal(valuesEqual(ab, mapOf([["a", int(1)], ["c", int(2)]])), false);
    assert.equal(valuesEqual(ab, mapOf([["a", int(1)], ["b", int(3)]])), false);
  });

  it("compares typed containers by their child constraint alone", () => {
    const t = (...elems: Value[]): Value => newTypedList(lit(TInteger), elems);
    assert.equal(valuesEqual(t(), t()), true);
    assert.equal(valuesEqual(t(int(1)), t(int(1))), true);
    // What a typed container RETAINS is not part of the type, and not
    // part of this equality either — core/go's rule, edge and all:
    // `d( [: Integer 1 ] [: Integer 2 ] )` keeps only the first.
    assert.equal(valuesEqual(t(int(1)), t(int(2))), true);
    assert.equal(valuesEqual(t(int(1)), t()), true);
    assert.equal(canonical([t(int(1)), t(int(2))]), "[:Integer 1]");
    assert.equal(valuesEqual(t(), newTypedList(lit(TString))), false);
    assert.equal(valuesEqual(t(), newList([])), false);
    const m = (k: string, v: Value): Value =>
      newTypedMap(lit(TInteger), [{ key: k, value: v }]);
    assert.equal(valuesEqual(m("a", int(1)), m("b", int(2))), true);
    assert.equal(valuesEqual(m("a", int(1)), newTypedMap(lit(TString))), false);
    // A typed list and a typed map are two types whatever their child.
    assert.equal(valuesEqual(t(), newTypedMap(lit(TInteger))), false);
    // A typed container's unresolved child is still a bare word, and two
    // words are the value their text spells.
    assert.equal(
      valuesEqual(newTypedList(newWord("Foo")), newTypedList(newWord("Foo"))),
      true,
    );
  });

  it("compares two disjuncts as their alternatives print, in order", () => {
    const d = (...a: Value[]): Value => newDisjunct(a);
    assert.equal(valuesEqual(d(int(1), int(2)), d(int(1), int(2))), true);
    assert.equal(valuesEqual(d(int(1), int(2)), d(int(2), int(1))), false);
    // An Enum and a plain disjunct are two types.
    assert.equal(valuesEqual(d(int(1)), newEnum([int(1)])), false);
    // core/go's rule, edges and all — it compares the two payloads as
    // rendered. A one-alternative nesting prints as its alternative...
    assert.equal(valuesEqual(d(d(int(1))), d(int(1))), true);
    assert.equal(valuesEqual(d(d()), d()), true);
    // ...two maps that differ in key order print differently...
    const ab = mapOf([["a", int(1)], ["b", int(2)]]);
    const ba = mapOf([["b", int(2)], ["a", int(1)]]);
    assert.equal(valuesEqual(ab, ba), true);
    assert.equal(valuesEqual(d(ab), d(ba)), false);
    // ...an unresolved word is not the value it resolves to...
    assert.equal(valuesEqual(d(newWord("false")), d(newBoolean(false))), false);
    // ...and two functions print as their parameter types, though they
    // are two functions.
    const sig = { params: [], returns: [], body: [] };
    const [f, g] = [newFnDef({ sigs: [sig] }), newFnDef({ sigs: [sig] })];
    assert.equal(valuesEqual(f, g), false);
    assert.equal(valuesEqual(d(f), d(g)), true);
    const taking = (t: typeof TInteger): Value =>
      newFnDef({ sigs: [{ params: [{ name: "n", type: t }], returns: [], body: [] }] });
    assert.equal(valuesEqual(d(taking(TInteger)), d(taking(TInteger))), true);
    assert.equal(valuesEqual(d(taking(TInteger)), d(taking(TString))), false);
    // So, as alternatives of an outer disjunct: one, two, one.
    assert.equal(canonical([d(d(int(1))), d(int(1))]), "1");
    assert.equal(canonical([d(ab), d(ba)]), "{a:1 b:2} tor {b:2 a:1}");
    assert.equal(canonical([d(f), d(g)]), "fn [[][][]]");
  });

  it("gives a function, an error and a class of payload identity only", () => {
    const sig = { params: [], returns: [], body: [] };
    const f = newFnDef({ sigs: [sig] });
    assert.equal(valuesEqual(f, f), true);
    assert.equal(valuesEqual(f, newFnDef({ sigs: [sig] })), false);
    assert.equal(valuesEqual(newErrorValue("x"), newErrorValue("x")), false);
    assert.equal(f.vType.equal(TFunction), true);
  });

  it("reads a structured scalar and a word by canonical text", () => {
    assert.equal(valuesEqual(dec("1.5"), dec("1.5")), true);
    assert.equal(valuesEqual(dec("1.5"), dec("1.50")), false);
    assert.equal(valuesEqual(newPath(["a", "b"], false), newPath(["a", "b"], false)), true);
    assert.equal(valuesEqual(newPath(["a"], false), newPath(["a"], true)), false);
    assert.equal(valuesEqual(newWord("w"), newWord("w")), true);
    assert.equal(valuesEqual(newWord("w"), newWord("x")), false);
  });
});

describe("simplifyDisjunctAlts", () => {
  it("drops Never, repeats and absorbed alternatives, and sorts the rest", () => {
    assert.equal(
      canonical([newString("a"), lit(TNever), int(5), int(5), lit(TString)]),
      "5 tor String",
    );
  });

  it("deduplicates by value: NaN once, the two zeros twice", () => {
    assert.equal(canonical([flt(NaN), flt(NaN), flt(1.5), flt(1.5)]), "1.5 tor nan");
    assert.equal(canonical([flt(-0), flt(0)]), "-0.0 tor 0.0");
  });

  it("keeps a carrier: it is neither a repeat of a value nor absorbed", () => {
    const out = simplifyDisjunctAlts([
      newCarrier(TInteger),
      lit(TNumber),
      newCarrier(TInteger),
      int(5),
    ]);
    // The value is absorbed by `Number`; the carrier stands for a value
    // nobody has seen and stays, once.
    assert.deepEqual(
      out.map((v) => [v.vType.leaf(), v.carrier]),
      [
        ["Number", false],
        ["Integer", true],
      ],
    );
  });

  it("does not absorb a value into its OWN type, only a wider one", () => {
    assert.equal(canonical([int(5), lit(TInteger)]), "Integer tor 5");
    assert.equal(canonical([lit(TInteger), lit(TNumber)]), "Number");
    assert.equal(canonical([mapOf([["a", int(1)]]), lit(TMap)]), "Map tor {a:1}");
  });

  it("reports whether a list of alternatives is already canonical", () => {
    const a = [lit(TInteger), int(5)];
    assert.equal(disjunctAltsCanonical(a), true);
    assert.equal(disjunctAltsCanonical([int(5), lit(TInteger)]), false);
    assert.equal(disjunctAltsCanonical([int(5), int(5)]), false);
    assert.equal(disjunctAltsCanonical([lit(TNever)]), false);
    assert.equal(disjunctAltsCanonical([]), true);
  });
});
