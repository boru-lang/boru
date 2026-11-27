// `name/v` — the reference read — and the function VALUE of a registered
// word: the arms the corpus cannot reach.
//
// core/spec/dispatch.tsv pins the rule (a `/v` word is the bound value and
// never a call; it is collected by a waiting word like any literal and is
// otherwise placed). Its fixture words all carry parameter names and
// declared returns, take no fallback signature, and are never run under a
// check pass — those shapes are built here.

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
import { TAny, TWord } from "./type.ts";
import {
  newFnDef,
  newInteger,
  newString,
  newTypeLiteral,
  newWord,
  Value,
  type WordInfo,
} from "./value.ts";

/** A word written `name/v`, as the parser hands it over. */
const ref = (name: string): Value =>
  new Value(TWord, { name, forceVal: true } satisfies WordInfo);

function fixture(): Registry {
  const r = new Registry();
  // No parameter names and no declared returns: the bare minimum a host
  // registers, which is what most of basic/ts and eng/ts's words are.
  r.registerNativeFunc({
    name: "idq",
    signatures: [
      { args: [TAny], barrierPos: 1, handler: (a: Value[]): Value[] => [a[0]!] },
    ],
  });
  // A NAME slot typed Word: the raw word is what fills it.
  r.registerNativeFunc({
    name: "nameq",
    signatures: [
      {
        args: [TWord],
        returns: [TAny],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [newString(a[0]!.asWord().name)],
      },
    ],
  });
  // Nothing but a FALLBACK signature — the zero-argument catch-all a word
  // carries so that a bare miss is a clean error.
  r.registerNativeFunc({
    name: "missq",
    signatures: [
      {
        args: [],
        fallback: true,
        barrierPos: 0,
        handler: (): Value[] => {
          throw new BoruError("signature_error", "missq: no arguments");
        },
      },
    ],
  });
  return r;
}

const run = (input: Value[], r: Registry = fixture()): string =>
  canon(new Engine(r).run(input));

describe("name/v — a registered word's function value", () => {
  it("renders an unnamed parameter with the empty name, and no returns", () => {
    // Go writes a parameter as NAME, then `:TYPE`; a parameter registered
    // without a name has the empty one. A signature that declares no
    // returns renders an empty returns list.
    assert.equal(run([ref("idq")]), "fn [[:Any][][]]");
  });

  it("carries the word's own dispatch table and the name it was read by", () => {
    const r = fixture();
    const v = r.resolveRef("idq")!;
    assert.equal("idq", v.fnName);
    assert.equal(r.lookup("idq"), v.asFnDef().native);
  });

  it("is undefined for a name nothing binds", () => {
    assert.equal(undefined, fixture().resolveRef("nosuch"));
  });

  it("is the SAME function at every read", () => {
    // The payload is the function's identity, so two references to one
    // word hold one payload — in two holders.
    const r = fixture();
    const a = r.resolveRef("idq")!;
    const b = r.resolveRef("idq")!;
    assert.notEqual(a, b);
    assert.equal(a.data, b.data);
  });

  it("rebuilds the word's function when the word gains a signature", () => {
    const r = fixture();
    const before = r.resolveRef("idq")!;
    r.registerNativeFunc({
      name: "idq",
      signatures: [
        {
          args: [TAny, TAny],
          barrierPos: 2,
          handler: (a: Value[]): Value[] => [a[0]!],
        },
      ],
    });
    const after = r.resolveRef("idq")!;
    assert.equal(1, before.asFnDef().sigs.length);
    assert.equal(2, after.asFnDef().sigs.length);
  });

  it("shares a def-bound function's payload and names the holder", () => {
    // The binding names its holder: the same function, under the name
    // the reference was read through — whatever name it carried before.
    const r = fixture();
    const f = newFnDef({ sigs: [{ params: [], returns: [], body: [] }] }, "f");
    r.pushDef("g", f);
    const g = r.resolveRef("g")!;
    assert.equal("g", g.fnName);
    assert.equal("f", f.fnName);
    assert.equal(f.data, g.data);
  });

  it("shadows a word's function stacked under a boru-bodied one", () => {
    // A name bound twice pools the overloads of both definitions — but a
    // native word's function is a dispatch table, not a body to pool.
    const r = fixture();
    const top = newFnDef({ sigs: [{ params: [], returns: [], body: [] }] });
    r.pushDef("h", r.resolveRef("idq")!);
    r.pushDef("h", top);
    assert.equal(top.data, r.resolveRef("h")!.data);
  });

  it("never takes a FALLBACK signature when the value is stepped", () => {
    // The word's only signature is its catch-all, which a bare `missq`
    // runs into its error. A value at rest is not a failed bare word, so
    // the catch-all is not its match — and with a 5 beside it, matching
    // nothing makes it a failed CALL.
    const r = fixture();
    assert.throws(
      () => new Engine(r).run([r.resolveRef("missq")!, newInteger(5n)]),
      (e: unknown) => e instanceof BoruError && "uncalled_function" === e.code,
    );
    // And with nothing beside it, it is simply data.
    const alone = r.resolveRef("missq")!;
    assert.deepEqual(new Engine(r).run([alone]), [alone]);
  });
});

describe("name/v — where the reference is read", () => {
  it("defers at a NAME slot when the name is unbound, so it is reported", () => {
    // A Word-typed slot keeps a plain word as data. A `/v` word is not a
    // name to capture: unbound, it must be stepped so the miss is the
    // name's own error rather than a word handed to the handler.
    assert.throws(
      () => run([newWord("nameq"), ref("nosuch")]),
      (e: unknown) =>
        e instanceof BoruError &&
        "undefined_word" === e.code &&
        /nosuch/.test(e.message),
    );
  });

  it("gives a NAME slot the plain word as before", () => {
    assert.equal(run([newWord("nameq"), newWord("anything")]), "'anything'");
  });
});

describe("name/v — under a check pass", () => {
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

  it("records an unbound reference as a diagnostic instead of throwing", () => {
    withAnalysis(() => {
      const r = fixture();
      r.check.begin();
      const out = new Engine(r).run([ref("nosuch")]);
      const undef = r.check.diagnostics.filter(
        (d) => "undefined_word" === d.code,
      );
      assert.equal(1, undef.length);
      assert.equal("nosuch", undef[0]!.word);
      assert.equal(1, out.length);
      assert.equal(true, out[0]!.undefined);
    });
  });

  it("counts the read as a USE of the binding", () => {
    withAnalysis(() => {
      const r = fixture();
      r.check.recordDef("x");
      r.pushDef("x", newInteger(7n));
      r.check.begin();
      new Engine(r).run([ref("x")]);
      r.check.emitUnusedDefDiagnostics();
      assert.equal(
        0,
        r.check.diagnostics.filter((d) => "unused_def" === d.code).length,
      );
    });
  });

  it("places a word's function value that matches nothing", () => {
    withAnalysis(() => {
      const r = fixture();
      r.check.begin();
      const missq = r.resolveRef("missq")!;
      const out = new Engine(r).run([missq, newInteger(5n)]);
      assert.equal(2, out.length);
      assert.equal(0, r.check.diagnostics.length);
    });
  });
});
