// The SPLICE marker (`Word/__SP`) — what `def name word [ … ]` binds.
//
// A binding holds a value, and a list bound to a name is that list
// (core/spec/dispatch.tsv, def BINDINGS). Splicing a list's elements into
// the program is a different thing altogether and has its own value: a
// marker that FIRES where it stands with nothing collecting it. The corpus
// notation cannot build one, so the marker's arms are tested here; the
// engine-level corpus (eng/spec/forth.tsv and friends) runs them from
// source on both engines.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { canon } from "./canon.ts";
import { Engine } from "./engine.ts";
import { Registry } from "./registry.ts";
import { TAny, TInteger, TSplice } from "./type.ts";
import {
  isSplice,
  newInteger,
  newInterpString,
  newList,
  newMap,
  newParenExpr,
  newSplice,
  newTypeLiteral,
  newWord,
  OrderedMap,
  spliceExpand,
  spliceIsData,
  Value,
} from "./value.ts";

function fixture(): Registry {
  const r = new Registry();
  r.registerNativeFunc({
    name: "addq",
    signatures: [
      {
        args: [TInteger, TInteger],
        returns: [TInteger],
        barrierPos: 2,
        handler: (a: Value[]): Value[] => [
          newInteger(a[0]!.asInteger() + a[1]!.asInteger()),
        ],
      },
    ],
  });
  r.registerNativeFunc({
    name: "idq",
    signatures: [
      { args: [TAny], barrierPos: 1, handler: (a: Value[]): Value[] => [a[0]!] },
    ],
  });
  // Two gradual slots, answering the second.
  r.registerNativeFunc({
    name: "secondq",
    signatures: [
      {
        args: [TAny, TAny],
        barrierPos: 2,
        handler: (a: Value[]): Value[] => [a[1]!],
      },
    ],
  });
  // A word that captures the NAME after it, and hands it back.
  r.registerNativeFunc({
    name: "nameq",
    signatures: [
      {
        args: [TAny],
        quoteArgs: new Set([0]),
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [a[0]!],
      },
    ],
  });
  // A binder, by name: its operand is a word's binding, not its expansion.
  r.registerNativeFunc({
    name: "def",
    signatures: [
      {
        args: [TAny, TAny],
        barrierPos: 2,
        handler: (a: Value[]): Value[] => [a[0]!],
      },
    ],
  });
  // An engine-internal word that leaves nothing: it runs without stranding
  // a waiting call, which is how a collection is still parked when the
  // next token is stepped.
  r.registerNativeFunc({
    name: "__nop",
    signatures: [{ args: [], barrierPos: 0, handler: (): Value[] => [] }],
  });
  return r;
}

const run = (input: Value[], r: Registry): string =>
  canon(new Engine(r).run(input));

const ints = (...ns: number[]): Value[] => ns.map((n) => newInteger(BigInt(n)));

/** A registry with `name` bound to a splice over `payload`. */
function bound(name: string, payload: Value): Registry {
  const r = fixture();
  r.pushDef(name, newSplice(payload));
  return r;
}

describe("a splice marker — the value", () => {
  it("is a marker only with a payload: the bare type is not one", () => {
    assert.equal(true, isSplice(newSplice(newInteger(1n))));
    assert.equal(false, isSplice(newTypeLiteral(TSplice)));
    assert.equal(false, isSplice(newInteger(1n)));
  });

  it("expands a plain list to its elements and anything else to itself", () => {
    const elems = ints(1, 2);
    assert.deepEqual(spliceExpand(newSplice(newList(elems))), elems);
    const seven = newInteger(7n);
    assert.deepEqual(spliceExpand(newSplice(seven)), [seven]);
    const m = newMap(new OrderedMap());
    assert.deepEqual(spliceExpand(newSplice(m)), [m]);
  });

  it("is DATA exactly when nothing it splices would run", () => {
    assert.equal(true, spliceIsData(newSplice(newList(ints(1, 2)))));
    assert.equal(true, spliceIsData(newSplice(newInteger(7n))));
    // A nested list is a value at the level the marker splices.
    assert.equal(
      true,
      spliceIsData(newSplice(newList([newList([newWord("addq")])]))),
    );
    for (const code of [
      newWord("addq"),
      newParenExpr(ints(1)),
      newInterpString([{ lit: "a" }]),
      newSplice(newInteger(1n)),
    ]) {
      assert.equal(false, spliceIsData(newSplice(newList([code]))));
    }
  });
});

describe("a splice marker — stepped where nothing is collecting it", () => {
  it("is replaced by a list's elements, which then run", () => {
    // `def three word [ addq 1 2 ]` then `three`.
    const r = bound("three", newList([newWord("addq"), ...ints(1, 2)]));
    assert.equal(run([newWord("three")], r), "3");
  });

  it("runs against the LIVE stack, as a Forth word does", () => {
    // `def incr word [ 1 addq/s ]` then `5 incr`: the spliced tokens see
    // the 5 beneath them.
    const adds = new Value(newWord("addq").vType, {
      name: "addq",
      forceStack: true,
    });
    const r = bound("incr", newList([newInteger(1n), adds]));
    assert.equal(run([newInteger(5n), newWord("incr")], r), "6");
  });

  it("contributes a value that is not a list as itself", () => {
    assert.equal(run([newWord("seven")], bound("seven", newInteger(7n))), "7");
  });

  it("fires again at every reference", () => {
    const r = bound("pair", newList(ints(1, 2)));
    assert.equal(run([newWord("pair"), newWord("pair")], r), "1 2 1 2");
  });
});

describe("a splice-bound word in a forward window — f w is f ( w )", () => {
  it("stands as the group ( w ), so a DATA splice's values are the arguments", () => {
    // Two values fill two slots, exactly as a written `addq ( pair )`.
    const r = bound("pair", newList(ints(1, 2)));
    assert.equal(run([newWord("addq"), newWord("pair")], r), "3");
  });

  it("is grouped the same way when it is stepped at a parked collection", () => {
    // `__nop` keeps secondq parked without filling a slot, so `pair` is
    // STEPPED with the collection waiting — and stands as ( pair ) there.
    const r = bound("pair", newList(ints(1, 2)));
    assert.equal(
      run(
        [newWord("secondq"), newWord("__nop"), newWord("pair"), newInteger(9n)],
        r,
      ),
      "2 9",
    );
  });

  it("is NOT grouped when it is code: the marker is the argument", () => {
    // A code-bearing splice is a macro over the live stack; grouping it
    // would change what it means. idq is handed the marker, hands it back,
    // and it fires where it lands.
    const r = bound("three", newList([newWord("addq"), ...ints(1, 2)]));
    assert.equal(run([newWord("idq"), newWord("three")], r), "3");
  });

  it("is NOT grouped for a BINDER, which rebinds the marker", () => {
    // `def y xs` aliases the splice. Here the binder hands its first
    // operand back, so the marker itself comes out — and fires.
    const r = bound("pair", newList(ints(1, 2)));
    assert.equal(
      run([newWord("def"), newWord("pair"), newInteger(5n)], r),
      "1 2",
    );
  });

  it("is NOT grouped for a binder parked on it either", () => {
    const r = bound("pair", newList(ints(1, 2)));
    assert.equal(
      run(
        [newWord("def"), newWord("__nop"), newWord("pair"), newInteger(5n)],
        r,
      ),
      "1 2",
    );
  });

  it("is NOT grouped where the slot captures the NAME", () => {
    // nameq takes the word itself; handed back, the word is stepped on
    // its own and the splice fires then.
    const r = bound("pair", newList(ints(1, 2)));
    assert.equal(run([newWord("nameq"), newWord("pair")], r), "1 2");
  });
});
