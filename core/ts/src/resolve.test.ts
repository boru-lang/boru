// Unit campaign for resolve.ts and capability.ts — three exported functions
// between them, none of which any test called (both files reported 0.00%
// function coverage).
//
// resolve.ts is ADR-012 rule 4's canonical cascade, and its file header warns
// that re-implementing either arm inline is how the Go per-site cascades
// drifted apart (NUR059). These tests pin the cascade ORDER, which is the
// part a re-implementation gets wrong: keywords, then the registry's binding
// store, then the builtin name table, then Atom.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { cap } from "./capability.ts";
import { Registry } from "./registry.ts";
import { resolveWordValue, resolveWordsDeep } from "./resolve.ts";
import { TEnum, TInteger, TString } from "./type.ts";
import {
  OrderedMap,
  newAtom,
  newDisjunct,
  newEnum,
  newInteger,
  newList,
  newMap,
  newString,
  newTypeLiteral,
  newTypedList,
  newWord,
} from "./value.ts";

describe("resolveWordValue", () => {
  it("passes a non-word through untouched", () => {
    const v = newInteger(1n);
    assert.equal(resolveWordValue(v), v);
  });

  it("resolves the two keywords", () => {
    assert.equal(resolveWordValue(newWord("true")).asBoolean(), true);
    assert.equal(resolveWordValue(newWord("false")).asBoolean(), false);
  });

  it("leaves `none` an Atom — the step loop and the parser own that keyword", () => {
    // core/go's resolveWordValue has `true` and `false` and nothing else:
    // the word `none` is the none VALUE only where it is STEPPED (stepWord)
    // or parsed. Nested in data it is a name like any other, so
    // `d( none foo )` is the two atoms in text order (`foo tor none`) and
    // `[q None ]` does not unify with `[q none ]`. This port resolved it
    // here too, and answered both differently.
    assert.equal(resolveWordValue(newWord("none")).asAtom(), "none");
  });

  it("resolves a builtin type PATH as well as a leaf name", () => {
    // One resolver for the builtin arm — the one stepWord consults.
    assert.equal(
      resolveWordValue(newWord("Scalar/String")).vType.toString(),
      "Scalar/String",
    );
    assert.equal(resolveWordValue(newWord("Scalar/Nope")).asAtom(), "Scalar/Nope");
  });

  it("resolves a builtin type name to its type literal", () => {
    assert.equal(resolveWordValue(newWord("Integer")).toString(), "Integer");
    assert.equal(resolveWordValue(newWord("String")).toString(), "String");
  });

  it("falls through to an Atom for an unknown name", () => {
    assert.equal(resolveWordValue(newWord("nope")).asAtom(), "nope");
  });

  it("lets a registry binding win over the builtin table", () => {
    // Arm order is the contract: the binding store is consulted BEFORE the
    // builtin names, so a user type shadows a builtin of the same name.
    const r = new Registry();
    r.pushDef("Integer", newString("shadowed"));
    assert.equal(
      resolveWordValue(newWord("Integer"), r).asString(),
      "shadowed",
    );
    // ...but the keywords are decided before the registry is consulted.
    r.pushDef("true", newString("nope"));
    assert.equal(resolveWordValue(newWord("true"), r).asBoolean(), true);
  });

  it("still reaches the builtin table for a name the registry does not bind", () => {
    const r = new Registry();
    assert.equal(resolveWordValue(newWord("Integer"), r).toString(), "Integer");
    assert.equal(resolveWordValue(newWord("nope"), r).asAtom(), "nope");
  });
});

describe("resolveWordsDeep", () => {
  it("resolves list elements", () => {
    const out = resolveWordsDeep(
      newList([newWord("true"), newWord("Integer")]),
    );
    assert.equal(out.toString(), "[true Integer]");
  });

  it("resolves map values and keeps insertion order", () => {
    const m = new OrderedMap();
    m.set("a", newWord("true"));
    m.set("b", newWord("nope"));
    assert.equal(resolveWordsDeep(newMap(m)).toString(), "{a:true b:nope}");
  });

  it("resolves a typed container child and its elements", () => {
    const tl = newTypedList(newWord("Integer"), [newWord("true")]);
    assert.equal(resolveWordsDeep(tl).toString(), "[:Integer true]");
  });

  it("preserves list eval/quoted flags through the walk", () => {
    const q = newList([newWord("true")], { quoted: true });
    assert.equal(resolveWordsDeep(q).quoted, true);
    const e = newList([newWord("true")], { eval: true });
    assert.equal(resolveWordsDeep(e).eval, true);
  });

  it("keeps a nested disjunct as ONE alternative, canonical in itself", () => {
    // Go does not flatten: `d( d( String Integer ) Integer )` is the two
    // alternatives `Integer` and `Integer tor String`, in the total order.
    // This port flattened, because its flat membership predicate compared
    // an alternative by equality and a nested union rejected every member;
    // the unifier walks a nested disjunct like any other alternative.
    const inner = newDisjunct([
      newTypeLiteral(TString),
      newTypeLiteral(TInteger),
    ]);
    const outer = newDisjunct([inner, newTypeLiteral(TInteger)]);
    const out = resolveWordsDeep(outer);
    assert.equal(out.asDisjunct().alternatives.length, 2);
    assert.equal(out.toString(), "Integer tor Integer tor String");
  });

  it("reduces a plain disjunct to its canonical alternatives", () => {
    // Repeats dropped, the rest in the total order — whether or not
    // resolution rebuilt an alternative (NUR385).
    const d = newDisjunct([
      newInteger(5n),
      newInteger(5n),
      newTypeLiteral(TInteger),
    ]);
    assert.equal(resolveWordsDeep(d).toString(), "Integer tor 5");
  });

  it("hands back a disjunct that is already canonical, untouched", () => {
    const d = newDisjunct([newTypeLiteral(TInteger), newInteger(5n)]);
    assert.equal(resolveWordsDeep(d), d);
  });

  it("keeps an Enum's declared order unless an alternative changed", () => {
    // An enum's member order is significant: resolution leaves it exactly
    // as declared — out of order, repeats and all.
    const declared = newEnum([newAtom("red"), newAtom("blue"), newAtom("red")]);
    assert.equal(resolveWordsDeep(declared), declared);
    // A word alternative RESOLVES (to an atom), which rebuilds the enum —
    // and a rebuilt one is reduced like any disjunct. It stays an Enum.
    const written = newEnum([newWord("red"), newAtom("blue")]);
    const out = resolveWordsDeep(written);
    assert.equal(out.vType.equal(TEnum), true);
    assert.equal(out.toString(), "blue tor red");
    // A list alternative always counts as changed: resolution rebuilds a
    // container, and a container is itself only by identity.
    const withList = newEnum([newList([newInteger(2n)]), newInteger(1n)]);
    assert.equal(resolveWordsDeep(withList).toString(), "1 tor [2]");
    // A typed-container alternative is rebuilt too, but to an EQUAL value:
    // that is not a change, and the enum is left alone.
    const withTyped = newEnum([
      newTypedList(newTypeLiteral(TString)),
      newInteger(1n),
    ]);
    assert.equal(resolveWordsDeep(withTyped), withTyped);
  });

  it("keeps the name and the position of a resolved named disjunct", () => {
    const named = newDisjunct([newInteger(2n), newInteger(1n)]).named("Small");
    named.pos = { row: 3, col: 4, src: "t" };
    const out = resolveWordsDeep(named);
    assert.equal(out.typeName, "Small");
    assert.deepEqual(out.pos, { row: 3, col: 4, src: "t" });
    assert.deepEqual(
      out.asDisjunct().alternatives.map((a) => a.toString()),
      ["1", "2"],
    );
  });

  it("leaves an unresolvable value alone", () => {
    const v = newInteger(1n);
    assert.equal(resolveWordsDeep(v), v);
  });
});

describe("cap", () => {
  it("returns the stored capability and true", () => {
    const r = new Registry();
    r.setCapability("fileops", { read: true });
    const [v, ok] = cap<{ read: boolean }>(r, "fileops");
    assert.equal(ok, true);
    assert.deepEqual(v, { read: true });
  });

  it("returns undefined and false when the capability is missing", () => {
    const [v, ok] = cap(new Registry(), "absent");
    assert.equal(ok, false);
    assert.equal(v, undefined);
  });
});
