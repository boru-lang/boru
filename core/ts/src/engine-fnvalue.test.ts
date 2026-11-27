// A function VALUE standing in the program — the arms of the step loop the
// corpus cannot reach.
//
// core/spec/dispatch.tsv pins the rule itself (a function value at the
// pointer is a call, planned as its word would be; a named one that matches
// nothing with anything around it is `uncalled_function`; the paren re-step
// rule). Its `fn(` form builds exactly one shape, though: a NAMED function
// of one required parameter. What needs a hand-built value is tested here —
// a nameless literal, optional parameters, several overloads, the parser's
// dispatch-modifier marker, and a check pass.

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
import { TAny, TInteger, TScalar, TString } from "./type.ts";
import {
  newCloseParen,
  newDispatchMod,
  newEnd,
  newFnDef,
  newInteger,
  newList,
  newOpenParen,
  newString,
  newTypeLiteral,
  newWord,
  Value,
  type FnSig,
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
      {
        args: [TAny],
        returns: [TAny],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [a[0]!],
      },
    ],
  });
  return r;
}

const run = (input: Value[], r: Registry = fixture()): string =>
  canon(new Engine(r).run(input));

const errorCode = (input: Value[], r: Registry = fixture()): string => {
  try {
    new Engine(r).run(input);
  } catch (e) {
    if (e instanceof BoruError) return e.code;
    throw e;
  }
  return "";
};

/** `n → n + 1`, one required Integer parameter. */
const incSig: FnSig = {
  params: [{ name: "n", type: TInteger }],
  returns: [TInteger],
  body: [newWord("addq"), newWord("n"), newInteger(1n)],
};

/** A nameless literal — what a `fn […]` word hands back. */
const literal = (...sigs: FnSig[]): Value => newFnDef({ sigs });

/** The same function reached through a name. */
const named = (name: string, ...sigs: FnSig[]): Value =>
  newFnDef({ sigs }, name);

describe("a function value at the pointer — the literal and the named", () => {
  it("calls a nameless literal on the argument after it", () => {
    assert.equal(run([literal(incSig), newInteger(5n)]), "6");
  });

  it("leaves a nameless literal that matches nothing as DATA", () => {
    // A named function here is `uncalled_function` (core/spec pins it).
    // A literal has no name to report a failed call under: it is placed.
    const lit = literal(incSig);
    const out = new Engine(fixture()).run([newString("x"), lit]);
    assert.equal(2, out.length);
    assert.equal(lit, out[1]);
  });

  it("raises uncalled_function for a NAMED one, naming it", () => {
    assert.throws(
      () => run([newString("x"), named("inc", incSig)]),
      (e: unknown) =>
        e instanceof BoruError &&
        "uncalled_function" === e.code &&
        /call to 'inc' matched no signature/.test(e.message) &&
        /inc\/v/.test(e.message),
    );
  });

  it("calls a literal of NO parameters where it stands", () => {
    const zero: FnSig = { params: [], returns: [], body: [newInteger(42n)] };
    assert.equal(run([literal(zero)]), "42");
  });
});

describe("a function value at the pointer — optional parameters", () => {
  const opt: FnSig = {
    params: [{ name: "n", type: TInteger, optional: true }],
    returns: [],
    body: [newWord("n")],
  };

  it("holds a LITERAL to its full parameter count", () => {
    // Go matches a bare literal against its signatures as written, so the
    // optional parameter is not optional here and nothing is called — the
    // rule that keeps `def f (fn [[a:Integer ?] …])` binding the function
    // rather than its zero-argument result.
    const lit = literal(opt);
    assert.deepEqual(new Engine(fixture()).run([lit]), [lit]);
    assert.equal(run([literal(opt), newInteger(3n)]), "3");
  });

  it("lets the same function reached through a NAME omit it", () => {
    // …while a named function is the function its word calls, and the
    // word may omit the parameter: it is bound to its type's base value.
    assert.equal(run([named("f", opt)]), "0");
  });
});

describe("a function value at the pointer — a word in its window", () => {
  /** Two overloads over Any: one argument, and two. */
  const one: FnSig = {
    params: [{ name: "a", type: TAny }],
    returns: [],
    body: [newWord("a")],
  };
  const two: FnSig = {
    params: [
      { name: "a", type: TAny },
      { name: "b", type: TAny },
    ],
    returns: [],
    body: [newWord("a"), newWord("b")],
  };

  it("COMMITS to the overload that takes what it has when a word begins", () => {
    // The two-argument overload plans `5` and the word `idq`. `idq` is a
    // function word: beginning its own dispatch is a statement boundary,
    // and the waiting call commits to the overload of exactly the one
    // argument it has claimed. `idq 7` then runs on its own.
    assert.equal(
      run([literal(one, two), newInteger(5n), newWord("idq"), newInteger(7n)]),
      "5 7",
    );
  });

  it("commits to the MORE SPECIFIC overload, whichever was written first", () => {
    // `(Any)`, `(Integer)` and `(Any Any)`: the two-argument overload
    // plans `1` and the word `idq`; when `idq` begins, the claimed 1 goes
    // to the overload normal dispatch would give it — the Integer one —
    // not to the first one written.
    const anyOne: FnSig = {
      params: [{ name: "a", type: TAny }],
      returns: [],
      body: [newInteger(10n)],
    };
    const intOne: FnSig = {
      params: [{ name: "a", type: TInteger }],
      returns: [],
      body: [newInteger(20n)],
    };
    for (const sigs of [
      [anyOne, intOne, two],
      [intOne, anyOne, two],
    ]) {
      assert.equal(
        run([literal(...sigs), newInteger(1n), newWord("idq"), newInteger(7n)]),
        "20 7",
      );
      // a String is only the generic overload's
      assert.equal(
        run([literal(...sigs), newString("x"), newWord("idq"), newInteger(7n)]),
        "10 7",
      );
    }
  });

  it("orders overloads by the LATTICE, so a branch root is tried before Any", () => {
    // `Scalar` and `Any` are both one step from the top of their paths,
    // but Scalar sorts after Any in the lattice order — rank, then depth,
    // then name — so it is the more specific and is tried first, in
    // whichever order the three were written.
    const of = (type: typeof TAny, n: bigint): FnSig => ({
      params: [{ name: "a", type }],
      returns: [],
      body: [newInteger(n)],
    });
    const scalar = of(TScalar, 1n);
    const str = of(TString, 2n);
    const any = of(TAny, 3n);
    for (const sigs of [
      [scalar, str, any],
      [any, str, scalar],
      [str, any, scalar],
    ]) {
      // an Integer is a Scalar and no String
      assert.equal(run([literal(...sigs), newInteger(1n)]), "1");
      // a String goes to the narrowest of the three
      assert.equal(run([literal(...sigs), newString("s")]), "2");
      // a list is only Any's
      assert.equal(run([literal(...sigs), newList([newInteger(1n)])]), "3");
    }
  });

  it("is STRANDED when no overload takes what it has", () => {
    // With only the two-argument overload there is nothing to commit to.
    assert.equal(
      errorCode([literal(two), newInteger(5n), newWord("idq"), newInteger(7n)]),
      "signature_error",
    );
  });

  it("commits with an argument the plan took from the STACK", () => {
    // The three-argument overload plans `idq` and `7` forward and the 5
    // beneath as its third. Claimed when `idq` begins: nothing ahead of
    // the word, and that 5 — which the one-argument overload takes.
    const three: FnSig = {
      params: [
        { name: "a", type: TAny },
        { name: "b", type: TAny },
        { name: "c", type: TAny },
      ],
      returns: [],
      body: [newWord("a")],
    };
    const r = fixture();
    r.pushDef("f", literal(one, three));
    assert.equal(
      run([newInteger(5n), newWord("f"), newWord("idq"), newInteger(7n)], r),
      "5 7",
    );
  });

  it("is stranded when the word comes FIRST and nothing is claimed", () => {
    // `5 f idq 7`, two arguments wanted: both come from the forward side,
    // so the 5 beneath was never part of the plan and there is nothing to
    // commit with.
    const r = fixture();
    r.pushDef("f", literal(one, two));
    assert.equal(
      errorCode([newInteger(5n), newWord("f"), newWord("idq"), newInteger(7n)], r),
      "signature_error",
    );
  });

  it("skips an overload the claimed arguments do not fit", () => {
    // The one-argument overload wants an Integer and the claimed argument
    // is a String, so there is nothing to commit to after all.
    const oneInt: FnSig = {
      params: [{ name: "a", type: TInteger }],
      returns: [],
      body: [newWord("a")],
    };
    assert.equal(
      errorCode([
        literal(oneInt, two),
        newString("x"),
        newWord("idq"),
        newInteger(7n),
      ]),
      "signature_error",
    );
  });

  it("binds a word that merely steps to a VALUE as that value", () => {
    // `null` is a keyword the forward scan leaves as a word; stepping it
    // yields the atom, so the atom is what arrives and what is bound.
    assert.equal(run([literal(one), newWord("null")]), "null/q");
  });

  it("binds a word a NAME is bound to as the data it is", () => {
    // `h` is bound to the word `idq`. The scan resolves the name to its
    // value — a word, as data — and that settles nothing: it is an
    // argument, not a function word beginning a dispatch.
    const r = fixture();
    r.pushDef("h", newWord("idq"));
    assert.equal(run([literal(one), newWord("h")], r), "idq");
  });

  it("reports an unbound name in the window by name", () => {
    assert.equal(errorCode([literal(one), newWord("nosuch")]), "undefined_word");
  });
});

describe("a function value at the pointer — the dispatch-modifier marker", () => {
  // What the parser emits after a group for `( … )/v` and `( … )/q`.
  const mark = (): Value => newDispatchMod({ val: true, quote: false });

  it("is DATA when a modifier marker follows it", () => {
    // The marker says so: the value is placed, the marker is consumed, and
    // the 5 that would have been its argument is left alone.
    const inc = named("inc", incSig);
    const out = new Engine(fixture()).run([inc, mark(), newInteger(5n)]);
    assert.equal(2, out.length);
    assert.equal(inc, out[0]);
    assert.equal("5", canon([out[1]!]));
  });

  it("drops a marker that follows a value which is not a function", () => {
    // `( 1 addq 2 )/v`: the modifier has nothing to say about a 3.
    assert.equal(
      run([
        newOpenParen(),
        newWord("addq"),
        newInteger(1n),
        newInteger(2n),
        newCloseParen(),
        mark(),
      ]),
      "3",
    );
  });
});

describe("a function value at the pointer — in a call frame's body", () => {
  it("is a failed call at the END of a body, with nothing else around", () => {
    // core/go counts the frame's own cleanup marker among the tokens the
    // value could have been called on, so a named function standing last
    // in a body always "has something after it". The Lean model answers
    // the same. It reads as an accident of the walk — the marker is no
    // argument — and is REPORTED as such rather than pinned in core/spec;
    // until core/go changes, this port answers what core/go answers.
    const outer: FnSig = {
      params: [{ name: "n", type: TAny }],
      returns: [],
      body: [named("inc", incSig)],
    };
    assert.equal(
      errorCode([named("outer", outer), newInteger(5n)]),
      "uncalled_function",
    );
  });

  it("is data there once an `end` bounds it", () => {
    // An `end` IS one of the boundaries the walk stops at, so the value
    // has nothing after it and is the body's result.
    const inc = named("inc", incSig);
    const outer: FnSig = {
      params: [{ name: "n", type: TAny }],
      returns: [],
      body: [inc, newEnd()],
    };
    const out = new Engine(fixture()).run([
      named("outer", outer),
      newInteger(5n),
    ]);
    assert.deepEqual(out, [inc]);
  });

  it("is called on what follows it in the body, as anywhere else", () => {
    const outer: FnSig = {
      params: [{ name: "n", type: TAny }],
      returns: [],
      body: [named("inc", incSig), newWord("n")],
    };
    assert.equal(run([named("outer", outer), newInteger(5n)]), "6");
  });
});

describe("a function value at the pointer — under a check pass", () => {
  /** Install a fake analysis for one test: core/ts must not reach for eng/ts. */
  function withAnalysis<T>(body: () => T): T {
    const saved = {
      strip: AnalysisImpl.stripToCarriers,
      carrier: AnalysisImpl.carrierResults,
    };
    installAnalysisImpl({
      stripToCarriers: (input: Value[]) => input,
      carrierResults: () => [newTypeLiteral(TInteger)],
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

  it("models a matched call through the body's analysis", () => {
    withAnalysis(() => {
      const r = fixture();
      r.check.begin();
      // The declared return models the call, as it does for the word.
      const out = new Engine(r).run([named("inc", incSig), newInteger(5n)]);
      assert.equal(1, out.length);
      assert.ok(out[0]!.carrier);
      assert.ok(out[0]!.vType.equal(TInteger));
    });
  });

  it("places a value that matches nothing, and raises nothing", () => {
    withAnalysis(() => {
      const r = fixture();
      r.check.begin();
      // At run time this is `uncalled_function`. A check pass is lenient:
      // the value is placed and analysis continues past it.
      const out = new Engine(r).run([newString("x"), named("inc", incSig)]);
      assert.equal(2, out.length);
      assert.ok(out[1]!.isFnDef());
      assert.equal(0, r.check.diagnostics.length);
    });
  });

  it("still assumes an overload for the WORD, gathering from the stack", () => {
    withAnalysis(() => {
      // The word's miss is the other arm: a failed call, modelled by
      // assuming the overload over the operand beneath it and saying so.
      const r = fixture();
      r.pushDef("inc", literal(incSig));
      r.check.begin();
      const out = new Engine(r).run([newString("x"), newWord("inc")]);
      assert.equal(1, out.length);
      assert.ok(out[0]!.vType.equal(TInteger));
      assert.deepEqual(
        r.check.diagnostics.map((d) => d.code),
        ["no_signature"],
      );
    });
  });
});
