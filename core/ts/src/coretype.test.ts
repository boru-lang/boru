// Unit campaign for coretype.ts — the seven exported classifiers, none of
// which any test called before this file existed (0.00% function coverage
// at 39.36% lines: the module body ran, nothing in it did).
//
// Expectations come from the documented contract each function names as its
// Go mirror (core_type.go, coretype.go). coerceBoolean is the one worth
// reading twice: it is a COERCION, not a parse, so the STRING "false" is a
// non-empty string and coerces to TRUE. make.ts's Boolean arm depends on
// that and so does the `if` rule.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import {
  coerceBoolean,
  isRecordShape,
  isTypeBody,
  isValueOfType,
  mintNamedType,
  parentType,
  pathOf,
  typeContent,
  typeContentOf,
  typeOf,
  unifiesValue,
} from "./coretype.ts";
import { BoruType, TAny, TInteger, TNone, TString, TType } from "./type.ts";
import {
  OrderedMap,
  newAtom,
  newBoolean,
  newDisjunct,
  newFloat,
  newInteger,
  newList,
  newMap,
  newNone,
  newString,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
  Value,
} from "./value.ts";

function mapOf(entries: Array<[string, ReturnType<typeof newInteger>]>) {
  const m = new OrderedMap();
  for (const [k, v] of entries) m.set(k, v);
  return newMap(m);
}

describe("coerceBoolean", () => {
  it("is false for none and for a bare type literal", () => {
    assert.equal(coerceBoolean(newNone()), false);
    assert.equal(coerceBoolean(newTypeLiteral(TInteger)), false);
  });

  it("reads a Boolean directly", () => {
    assert.equal(coerceBoolean(newBoolean(true)), true);
    assert.equal(coerceBoolean(newBoolean(false)), false);
  });

  it("is emptiness for numbers", () => {
    assert.equal(coerceBoolean(newInteger(0n)), false);
    assert.equal(coerceBoolean(newInteger(1n)), true);
    assert.equal(coerceBoolean(newInteger(-1n)), true);
    assert.equal(coerceBoolean(newFloat(0)), false);
    assert.equal(coerceBoolean(newFloat(0.5)), true);
  });

  it("is emptiness for strings, lists and maps", () => {
    assert.equal(coerceBoolean(newString("")), false);
    assert.equal(coerceBoolean(newString("x")), true);
    assert.equal(coerceBoolean(newList([])), false);
    assert.equal(coerceBoolean(newList([newInteger(1n)])), true);
    assert.equal(coerceBoolean(mapOf([])), false);
    assert.equal(coerceBoolean(mapOf([["k", newInteger(1n)]])), true);
  });

  it('never inspects string CONTENT — "false" is a non-empty string', () => {
    // The coercion/parse distinction make.ts's Boolean arm relies on.
    assert.equal(coerceBoolean(newString("false")), true);
    assert.equal(coerceBoolean(newString("0")), true);
  });

  it("counts a typed list by its elements", () => {
    const empty = newTypedList(newTypeLiteral(TInteger), []);
    const one = newTypedList(newTypeLiteral(TInteger), [newInteger(1n)]);
    assert.equal(coerceBoolean(empty), false);
    assert.equal(coerceBoolean(one), true);
  });
});

describe("parentType", () => {
  it("drops the last segment of a multi-part path", () => {
    assert.deepEqual(parentType(TInteger)?.parts, ["Scalar", "Number"]);
    assert.deepEqual(parentType(new BoruType(["Scalar", "Number"]))?.parts, [
      "Scalar",
    ]);
  });

  it("gives a branch root the parent Any", () => {
    assert.deepEqual(parentType(new BoruType(["Scalar"]))?.parts, TAny.parts);
    assert.deepEqual(parentType(TType)?.parts, TAny.parts);
  });

  it("saturates the degenerate roots — they have no parent", () => {
    for (const name of ["Any", "None", "Never", "Absent"]) {
      assert.equal(
        parentType(new BoruType([name])),
        null,
        `${name} should saturate`,
      );
    }
  });
});

describe("typeOf", () => {
  it("gives the none value the None type literal", () => {
    assert.equal(typeOf(newNone()).toString(), "None");
  });

  it("gives a concrete value its own type, subtype tag included", () => {
    assert.equal(typeOf(newInteger(1n)).toString(), "Integer");
    // A string carries its String SUBTYPE, exactly as Go's NewString tags it
    // (EmptyString for '', ProperString otherwise) — typeOf reports the tag,
    // not the branch.
    assert.equal(typeOf(newString("x")).toString(), "ProperString");
    assert.equal(typeOf(newString("")).toString(), "EmptyString");
  });
});

describe("pathOf", () => {
  it("expands a type to its ancestor chain, root first", () => {
    const p = pathOf(newTypeLiteral(TInteger));
    assert.equal(p.toString(), "[Scalar Number Integer]");
  });

  it("gives a single-part type a one-element path", () => {
    assert.equal(pathOf(newTypeLiteral(TNone)).toString(), "[None]");
  });
});

describe("isRecordShape", () => {
  it("accepts a non-empty map whose every value is a type literal", () => {
    const m = new OrderedMap();
    m.set("a", newTypeLiteral(TInteger));
    m.set("b", newTypeLiteral(TString));
    assert.equal(isRecordShape(newMap(m)), true);
  });

  it("recurses into nested record shapes", () => {
    const inner = new OrderedMap();
    inner.set("x", newTypeLiteral(TInteger));
    const outer = new OrderedMap();
    outer.set("n", newMap(inner));
    assert.equal(isRecordShape(newMap(outer)), true);
  });

  it("rejects an empty map, a non-map, and a map with a concrete value", () => {
    assert.equal(isRecordShape(mapOf([])), false);
    assert.equal(isRecordShape(newInteger(1n)), false);
    assert.equal(isRecordShape(mapOf([["a", newInteger(1n)]])), false);
  });
});

describe("isTypeBody", () => {
  it("accepts typed containers and disjunctions", () => {
    assert.equal(isTypeBody(newTypedList(newTypeLiteral(TInteger), [])), true);
    assert.equal(isTypeBody(newTypedMap(newTypeLiteral(TInteger), [])), true);
    assert.equal(
      isTypeBody(
        newDisjunct([newTypeLiteral(TInteger), newTypeLiteral(TString)]),
      ),
      true,
    );
  });

  it("accepts a record shape", () => {
    assert.equal(isTypeBody(mapOf([])), false);
    const m = new OrderedMap();
    m.set("a", newTypeLiteral(TInteger));
    assert.equal(isTypeBody(newMap(m)), true);
  });

  it("rejects a plain scalar", () => {
    assert.equal(isTypeBody(newInteger(1n)), false);
    assert.equal(isTypeBody(newString("x")), false);
  });
});

describe("isValueOfType", () => {
  it("accepts a value under a bare type literal by lattice subtyping", () => {
    assert.equal(isValueOfType(newInteger(1n), newTypeLiteral(TInteger)), true);
    assert.equal(isValueOfType(newInteger(1n), newTypeLiteral(TAny)), true);
    assert.equal(
      isValueOfType(newString("x"), newTypeLiteral(TInteger)),
      false,
    );
  });

  it("accepts a type value under the bare metatype Type", () => {
    assert.equal(
      isValueOfType(newTypeLiteral(TInteger), newTypeLiteral(TType)),
      true,
    );
  });

  it("checks a typed list element-wise", () => {
    const t = newTypedList(newTypeLiteral(TInteger), []);
    assert.equal(
      isValueOfType(newList([newInteger(1n), newInteger(2n)]), t),
      true,
    );
    assert.equal(
      isValueOfType(newList([newInteger(1n), newString("x")]), t),
      false,
    );
  });

  it("admits a disjunction alternative", () => {
    const d = newDisjunct([newTypeLiteral(TInteger), newTypeLiteral(TString)]);
    assert.equal(isValueOfType(newInteger(1n), d), true);
    assert.equal(isValueOfType(newString("x"), d), true);
    assert.equal(isValueOfType(newBoolean(true), d), false);
  });
});

// The arms measurement found uncovered. Each expectation below was read
// off core/go first (CoerceBoolean, TypeOf, IsValueOfType on the same
// inputs) rather than off this port, so the row states the shared contract
// and not TypeScript's own behaviour.
describe("coretype arms measured against core/go", () => {
  it("coerces a string by emptiness, not by content", () => {
    assert.equal(coerceBoolean(newString("")), false);
    assert.equal(coerceBoolean(newString("a")), true);
    // "false" is a non-empty STRING, so it coerces to true — the arm's
    // whole point, and what make.ts's Boolean conversion relies on.
    assert.equal(coerceBoolean(newString("false")), true);
  });

  it("coerces a value with no falsy rule of its own to true", () => {
    // The trailing `return true`: an Atom has no emptiness notion.
    assert.equal(coerceBoolean(newAtom("a")), true);
  });

  it("takes one lattice hop for a bare type literal", () => {
    // `typeof Integer` is Number, one Parent hop — eng/spec/typeof.tsv:77.
    // The payload-less arm is what supplies the hop; a CONCRETE value
    // reports its own type instead.
    assert.equal(
      typeOf(newTypeLiteral(TInteger)).vType.toString(),
      "Scalar/Number",
    );
    assert.equal(
      typeOf(newInteger(5n)).vType.toString(),
      "Scalar/Number/Integer",
    );
    assert.equal(typeOf(newNone()).vType.toString(), "None");
  });

  it("reads a concrete map alternative as an OPEN pattern", () => {
    // A map alternative admits a value carrying AT LEAST its keys, with
    // conforming values; a differing value at a declared key rejects.
    const d = newDisjunct([mapOf([["a", newInteger(1n)]]), newInteger(9n)]);
    assert.equal(
      isValueOfType(
        mapOf([
          ["a", newInteger(1n)],
          ["b", newInteger(2n)],
        ]),
        d,
      ),
      true,
    );
    assert.equal(isValueOfType(mapOf([["a", newInteger(2n)]]), d), false);
    assert.equal(isValueOfType(newInteger(9n), d), true);
  });

  it("reads a map NESTED in an alternative as an exact key set", () => {
    // The open-subset rule stops at the immediate alternative: a map
    // reached through a list unifies only on an EQUAL key set, which is
    // the arm that separates unifiesValue's map case from the one above.
    const d = newDisjunct([newList([mapOf([["a", newInteger(1n)]])])]);
    assert.equal(
      isValueOfType(newList([mapOf([["a", newInteger(1n)]])]), d),
      true,
    );
    assert.equal(
      isValueOfType(
        newList([
          mapOf([
            ["a", newInteger(1n)],
            ["b", newInteger(2n)],
          ]),
        ]),
        d,
      ),
      false,
    );
  });
});

describe("mintNamedType (the Stage-2 node)", () => {
  it("mints a node under the body's type that carries the body", () => {
    const one = mintNamedType("One", newInteger(1));
    assert.equal(one.toString(), "Scalar/Number/Integer/One");
    assert.equal(one.leaf(), "One");
    assert.equal(typeContent(one)?.asInteger(), 1n);
    const lit = newTypeLiteral(one);
    assert.equal(typeContentOf(lit).asInteger(), 1n, "the node's literal reads through to its content");
    assert.equal(typeContentOf(newInteger(7)).asInteger(), 7n, "any other value is its own structure");
    assert.equal(typeContent(TInteger), undefined, "a builtin node carries no content");
    // typeof is one parent hop: the content's type.
    assert.equal(typeOf(lit).vType.toString(), TInteger.toString());
  });
  it("is a member of Type and of itself; membership is the content's", () => {
    const one = newTypeLiteral(mintNamedType("One", newInteger(1)));
    assert.equal(isValueOfType(one, newTypeLiteral(TType)), true);
    assert.equal(isValueOfType(one, one), true);
    assert.equal(isValueOfType(newInteger(1), one), true);
    assert.equal(isValueOfType(newInteger(2), one), false);
    assert.equal(isValueOfType(newInteger(1), newTypeLiteral(TInteger)), true, "the value keeps its own type");
    const color = newTypeLiteral(mintNamedType("Color", newDisjunct([newAtom("red"), newAtom("green")])));
    assert.equal(isValueOfType(newAtom("red"), color), true);
    assert.equal(isValueOfType(newAtom("blue"), color), false);
    assert.equal(isValueOfType(newString("red"), color), false);
  });
  it("unifies as its content on either side", () => {
    const one = newTypeLiteral(mintNamedType("One", newInteger(1)));
    assert.equal(unifiesValue(one, newInteger(1)), true);
    assert.equal(unifiesValue(newInteger(1), one), true);
    assert.equal(unifiesValue(one, newInteger(2)), false);
    assert.equal(unifiesValue(one, one), true);
  });
  it("mints a check-mode carrier body under the carrier's own type", () => {
    const c = new Value(TInteger, null, { carrier: true });
    const n = mintNamedType("N", c);
    assert.equal(n.toString(), "Scalar/Number/Integer/N", "the carrier's type is not hopped");
    assert.equal(typeContent(n), c);
    assert.equal(typeContentOf(c), c, "a carrier is its own structure");
  });
});
