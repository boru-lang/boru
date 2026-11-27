// `name/u` — the bound function with its argument order reversed — and
// `name/uv`, that wrapper as a value: the arms the corpus cannot reach.
//
// core/spec/dispatch.tsv pins the rule on the fixture words: `f/u a b c` is
// `f c b a`. Every one of those words is all-forward or all-stack and has
// one signature, so what is built here is a word with a MIXED barrier, a
// word with several signatures, the registry's own answers, and a check
// pass.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import {
  AnalysisImpl,
  installAnalysisImpl,
} from "./analysis-hooks.ts";
import { canon } from "./canon.ts";
import { Engine } from "./engine.ts";
import { BoruError } from "./error.ts";
import { Registry } from "./registry.ts";
import { TAny, TString, TWord } from "./type.ts";
import {
  newInteger,
  newList,
  newString,
  newTypeLiteral,
  Value,
  type WordInfo,
} from "./value.ts";

/** A word written `name/u`, or `name/uv`, as the parser hands it over. */
const usurp = (name: string, val = false): Value =>
  new Value(TWord, {
    name,
    forceUsurp: true,
    ...(val ? { forceVal: true } : {}),
  } satisfies WordInfo);

const ints = (...ns: number[]): Value[] => ns.map((n) => newInteger(BigInt(n)));

function fixture(): Registry {
  const r = new Registry();
  // Three gradual parameters, the FIRST collectable forward and the other
  // two read from the stack: `c b mixq a`. It answers its arguments in
  // signature order, so a test can read back where each one landed.
  r.registerNativeFunc({
    name: "mixq",
    signatures: [
      {
        args: [TAny, TAny, TAny],
        argNames: ["a", "b", "c"],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [newList([...a])],
      },
    ],
  });
  // Two signatures: one argument, or a String and anything.
  r.registerNativeFunc({
    name: "polyq",
    signatures: [
      {
        args: [TAny],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [newList([newString("one"), a[0]!])],
      },
      {
        args: [TString, TAny],
        barrierPos: 2,
        handler: (a: Value[]): Value[] => [
          newList([newString("two"), a[0]!, a[1]!]),
        ],
      },
    ],
  });
  return r;
}

const run = (input: Value[], r: Registry = fixture()): string =>
  canon(new Engine(r).run(input));

describe("name/u — the reversed call", () => {
  it("lays the arguments out for the ORIGINAL's barrier", () => {
    // mixq a b c, called as written, is `c b mixq a`: a forward, b on top
    // of the stack, c beneath.
    assert.equal(run([...ints(3, 2), new Value(TWord, { name: "mixq" }), ...ints(1)]), "[1 2 3]");
    // mixq/u collects all three FORWARD and in reverse: `mixq/u 3 2 1` is
    // mixq with a=1, b=2, c=3 — re-dispatched as `3 2 mixq 1`.
    assert.equal(run([usurp("mixq"), ...ints(3, 2, 1)]), "[1 2 3]");
  });

  it("reverses every signature and still picks by what is written", () => {
    // polyq 'k' 5 is the two-argument signature; reversed, the String is
    // written LAST.
    assert.equal(
      run([usurp("polyq"), newInteger(5n), newString("k")]),
      "['two' 'k' 5]",
    );
    // A lone Integer still reaches the one-argument signature.
    assert.equal(run([usurp("polyq"), newInteger(5n)]), "['one' 5]");
  });
});

describe("name/u — what the registry answers", () => {
  it("makes a NEW function every time", () => {
    const r = fixture();
    const a = r.resolveUsurp("mixq") as Value;
    const b = r.resolveUsurp("mixq") as Value;
    assert.notEqual(a.data, b.data);
    assert.equal("mixq", a.fnName);
  });

  it("renders the wrapper's parameters reversed, by name", () => {
    assert.equal(run([usurp("mixq", true)]), "fn [[c:Any b:Any a:Any][][]]");
  });

  it("declines a name bound to a value, and a name nothing binds", () => {
    const r = fixture();
    r.pushDef("x", newInteger(5n));
    assert.equal("illegal_ref", r.resolveUsurp("x"));
    assert.equal(undefined, r.resolveUsurp("nosuch"));
  });

  it("raises illegal_ref for a value binding, naming it", () => {
    const r = fixture();
    r.pushDef("x", newInteger(5n));
    assert.throws(
      () => run([usurp("x")], r),
      (e: unknown) =>
        e instanceof BoruError &&
        "illegal_ref" === e.code &&
        /\/u requires a function word: x/.test(e.message),
    );
  });
});

describe("name/u — under a check pass", () => {
  function withAnalysis<T>(body: () => T): T {
    const saved = {
      strip: AnalysisImpl.stripToCarriers,
      carrier: AnalysisImpl.carrierResults,
    };
    installAnalysisImpl({
      stripToCarriers: (input: Value[]) => input,
      carrierResults: () => [newTypeLiteral(TAny)],
    });
    try {
      return body();
    } finally {
      installAnalysisImpl({
        stripToCarriers: saved.strip,
        carrierResults: saved.carrier,
      });
    }
  }

  it("records an unbound name as a diagnostic instead of throwing", () => {
    withAnalysis(() => {
      const r = fixture();
      r.check.begin();
      const out = new Engine(r).run([usurp("nosuch")]);
      assert.deepEqual(
        r.check.diagnostics.map((d) => d.code),
        ["undefined_word"],
      );
      assert.equal(true, out[0]!.undefined);
    });
  });

  it("records a value binding as illegal_ref and carries on", () => {
    withAnalysis(() => {
      const r = fixture();
      r.pushDef("x", newInteger(5n));
      r.check.begin();
      const out = new Engine(r).run([usurp("x"), newInteger(7n)]);
      assert.deepEqual(
        r.check.diagnostics.map((d) => d.code),
        ["illegal_ref"],
      );
      assert.equal("x", r.check.diagnostics[0]!.word);
      // The placeholder stands where the word stood; analysis went on to
      // the 7 after it.
      assert.equal(2, out.length);
    });
  });
});
