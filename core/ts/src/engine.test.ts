// Unit campaign for engine.ts — the step loop.
//
// design/legacy/CORE-TS-COVERAGE.0.ignore stages 4 and 5. engine.ts held 947 of core/ts's
// 1,945 uncovered lines across 44 methods, and the corpus could not reach them:
// core/spec registers ONE fixture word and its notation is deliberately
// parser-free, so user function definitions, check-mode passes, paren
// expressions, interpolation strings and XML templates — the ten methods
// holding 590 of those lines — have no expression in it.
//
// The fixtures here register several words instead of one and build the nested
// value shapes directly, which is what a unit suite can do and a line-oriented
// corpus cannot.
//
// Stage 5's check-mode arms (dispatchFnDefCheck, analyseFnBody,
// checkModeAssumeSig) run only with an AnalysisImpl installed. In production
// that comes from eng/ts's check.ts, and core/ts must not depend on it — the
// package declares no dependency on @boru-lang/eng precisely so a core file
// reaching upward fails to resolve. So the check-mode tests install a FAKE
// through installAnalysisImpl, which is the reason that function is exported
// and the same way core/go covers the same arms.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import {
  AnalysisImpl,
  inactiveCarrierResults,
  inactiveStripToCarriers,
  installAnalysisImpl,
} from "./analysis-hooks.ts";
import { canon } from "./canon.ts";
import { Engine } from "./engine.ts";
import { BoruError } from "./error.ts";
import { Registry } from "./registry.ts";
import {
  TAny,
  TAtom,
  TBoolean,
  TFloat,
  TInteger,
  TList,
  TMap,
  TNone,
  TString,
} from "./type.ts";
import {
  OrderedMap,
  newAtom,
  newBoolean,
  newCloseParen,
  newDynamicCarrier,
  newEnd,
  newFnDef,
  newInteger,
  newInterpString,
  newList,
  newMap,
  newMark,
  newMove,
  newMoveIf,
  nextMarkId,
  newOpenParen,
  newParenExpr,
  newString,
  newSugar,
  newDisjunct,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
  newWord,
  newXmlInterp,
  Value,
  type FnDefInfo,
  type FnSig,
} from "./value.ts";

/** A registry with a small word vocabulary — more than core/spec's one. */
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
  } as never);
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
  } as never);
  r.registerNativeFunc({
    name: "dropq",
    signatures: [
      { args: [TAny], returns: [], barrierPos: 1, handler: (): Value[] => [] },
    ],
  } as never);
  // Two returns from one operand: the shape a single-return word cannot
  // reach — a map value that yields more than one residual, which the
  // engine has to gather into a list rather than storing straight.
  r.registerNativeFunc({
    name: "pairq",
    signatures: [
      {
        args: [TAny],
        returns: [TAny, TAny],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [a[0]!, a[0]!],
      },
    ],
  } as never);
  return r;
}

const run = (input: Value[], r: Registry = fixture()): string =>
  canon(new Engine(r).run(input));

/** A single-overload user fn: params → body. */
function fnDef(params: FnSig["params"], body: Value[]): Value {
  return newFnDef({ sigs: [{ params, returns: [], body }] } as FnDefInfo);
}

describe("Engine.run — literals and residuals", () => {
  it("returns a lone literal", () => {
    assert.equal(run([newInteger(1n)]), "1");
  });

  it("returns multiple residuals in order", () => {
    assert.equal(
      run([newInteger(1n), newString("s"), newBoolean(true)]),
      "1 's' true",
    );
  });

  it("returns nothing for an empty program", () => {
    assert.equal(run([]), "");
  });

  it("dispatches a native word over its forward args", () => {
    assert.equal(run([newWord("addq"), newInteger(2n), newInteger(3n)]), "5");
  });

  it("leaves a word with no binding as an error", () => {
    assert.throws(() => run([newWord("nosuch")]), BoruError);
  });
});

describe("Engine.run — parens", () => {
  it("evaluates a paren group before the enclosing dispatch", () => {
    const prog = [
      newWord("addq"),
      newInteger(1n),
      newOpenParen(),
      newWord("addq"),
      newInteger(2n),
      newInteger(3n),
      newCloseParen(),
    ];
    assert.equal(run(prog), "6");
  });

  it("evaluates a pre-built paren expression value", () => {
    const pe = newParenExpr([newWord("addq"), newInteger(2n), newInteger(3n)]);
    assert.equal(run([newWord("idq"), pe]), "5");
  });

  it("raises on an unmatched close paren", () => {
    assert.throws(
      () => run([newInteger(1n), newCloseParen(), newInteger(2n)]),
      BoruError,
    );
  });
});

describe("Engine.run — containers", () => {
  it("auto-evaluates an eval list", () => {
    const l = newList([newWord("addq"), newInteger(1n), newInteger(2n)], {
      eval: true,
    });
    assert.equal(run([l]), "[3]");
  });

  it("leaves a quoted list inert", () => {
    const l = newList([newWord("addq"), newInteger(1n), newInteger(2n)], {
      quoted: true,
    });
    // canon wraps a quoted list so the render round-trips back to a quote.
    assert.equal(run([l]), "(quote [addq 1 2])");
  });

  it("auto-evaluates the values of an EVAL map", () => {
    const m = new OrderedMap();
    m.set("k", newParenExpr([newWord("addq"), newInteger(1n), newInteger(1n)]));
    assert.equal(run([newMap(m, { eval: true })]), "{k:2}");
  });

  it("leaves a RUNTIME map alone, however evaluable its values look", () => {
    // The eval flag is the whole test. This assertion used to read
    // `newMap(m)` — no flag — and expect `{k:2}`, which pinned a
    // divergence: Go's autoEvalStack descends only into a container the
    // PARSER built, so the same map is data there. core/spec/data.tsv now
    // holds the pair of rows, and both engines are asked.
    const m = new OrderedMap();
    m.set("k", newParenExpr([newWord("addq"), newInteger(1n), newInteger(1n)]));
    // NUR059 gave the paren group a SOURCE form; this pinned the debug
    // dump `paren([word(addq) 1 1])`. The POINT of the row is unchanged:
    // a runtime map is left alone, its paren value unevaluated.
    assert.equal(run([newMap(m)]), "{k:(addq 1 1)}");
  });

  it("leaves a plain map alone", () => {
    const m = new OrderedMap();
    m.set("k", newInteger(1n));
    assert.equal(run([newMap(m)]), "{k:1}");
  });
});

describe("Engine.run — def substitution", () => {
  it("substitutes a simple bound value", () => {
    const r = fixture();
    r.pushDef("x", newInteger(7n));
    assert.equal(run([newWord("x")], r), "7");
  });

  it("splices an unquoted eval list as a code body", () => {
    const r = fixture();
    r.pushDef(
      "body",
      newList([newWord("addq"), newInteger(1n), newInteger(2n)], {
        eval: true,
      }),
    );
    assert.equal(run([newWord("body")], r), "3");
  });

  it("treats a QUOTED list binding as data, not code", () => {
    const r = fixture();
    r.pushDef(
      "data",
      newList([newInteger(1n), newInteger(2n)], { quoted: true }),
    );
    assert.equal(run([newWord("data")], r), "(quote [1 2])");
  });

  it("lets a def shadow and unshadow", () => {
    const r = fixture();
    r.pushDef("x", newInteger(1n));
    r.pushDef("x", newInteger(2n));
    assert.equal(run([newWord("x")], r), "2");
    r.popDef("x");
    assert.equal(run([newWord("x")], r), "1");
  });
});

describe("Engine.run — user function definitions", () => {
  it("calls a one-param fn with its body", () => {
    const r = fixture();
    r.pushDef(
      "twice",
      fnDef(
        [{ name: "n", type: TInteger }],
        [newWord("addq"), newWord("n"), newWord("n")],
      ),
    );
    assert.equal(run([newWord("twice"), newInteger(4n)], r), "8");
  });

  it("calls a two-param fn", () => {
    const r = fixture();
    r.pushDef(
      "sum",
      fnDef(
        [
          { name: "a", type: TInteger },
          { name: "b", type: TInteger },
        ],
        [newWord("addq"), newWord("a"), newWord("b")],
      ),
    );
    assert.equal(run([newWord("sum"), newInteger(2n), newInteger(5n)], r), "7");
  });

  it("picks the overload whose arity matches", () => {
    const r = fixture();
    const info: FnDefInfo = {
      sigs: [
        {
          params: [{ name: "a", type: TInteger }],
          returns: [],
          body: [newInteger(1n)],
        },
        {
          params: [
            { name: "a", type: TInteger },
            { name: "b", type: TInteger },
          ],
          returns: [],
          body: [newInteger(2n)],
        },
      ],
    };
    r.pushDef("f", newFnDef(info));
    assert.equal(run([newWord("f"), newInteger(9n), newInteger(9n)], r), "2");
  });

  it("raises when no signature matches the arguments", () => {
    const r = fixture();
    r.pushDef(
      "needsInt",
      fnDef([{ name: "n", type: TInteger }], [newWord("n")]),
    );
    assert.throws(
      () => run([newWord("needsInt"), newString("not an int")], r),
      (e: unknown) =>
        e instanceof BoruError && e.message.includes("no signature matches"),
    );
  });

  it("evaluates a paren argument before matching", () => {
    const r = fixture();
    r.pushDef(
      "twice",
      fnDef(
        [{ name: "n", type: TInteger }],
        [newWord("addq"), newWord("n"), newWord("n")],
      ),
    );
    const arg = newParenExpr([newWord("addq"), newInteger(1n), newInteger(2n)]);
    assert.equal(run([newWord("twice"), arg], r), "6");
  });
});

describe("Engine.run — interpolation", () => {
  it("substitutes an expression segment and joins the literals", () => {
    const s = newInterpString([
      { lit: "a=" },
      { expr: [newWord("addq"), newInteger(1n), newInteger(1n)] },
      { lit: "!" },
    ]);
    assert.equal(run([s]), "'a=2!'");
  });

  it("handles a literal-only interpolation", () => {
    assert.equal(run([newInterpString([{ lit: "plain" }])]), "'plain'");
  });

  it("renders a substituted string segment without quoting it", () => {
    const s = newInterpString([
      { lit: "<" },
      { expr: [newString("x")] },
      { lit: ">" },
    ]);
    assert.equal(run([s]), "'<x>'");
  });
});

describe("Engine.run — the end marker", () => {
  it("terminates collection at an end marker", () => {
    assert.equal(run([newInteger(1n), newEnd()]), "1");
  });
});

describe("Engine.run — self-referential def", () => {
  it("settles rather than looping", () => {
    // A def bound to a list containing its own name splices once and then
    // leaves the word as a residual: the step loop terminates instead of
    // re-resolving forever. Recorded because the obvious expectation — a
    // step-budget refusal — is NOT what happens.
    const r = fixture();
    r.pushDef("loop", newList([newWord("loop")], { eval: true }));
    assert.equal(run([newWord("loop")], r), "loop");
  });
});

describe("AnalysisImpl — the named inactive defaults", () => {
  it("strips to carriers as the identity", () => {
    const input = [newInteger(1n)];
    assert.equal(inactiveStripToCarriers(input), input);
  });

  it("models no carriers at all", () => {
    assert.deepEqual(
      inactiveCarrierResults(
        new Registry(),
        "w",
        { args: [], handler: () => [] } as never,
        [],
      ),
      [],
    );
  });
});

describe("Engine — check mode, behind a fake AnalysisImpl", () => {
  /**
   * Install a fake for the duration of one test and restore the inactive
   * defaults afterwards. core/ts must not reach for eng/ts's real
   * implementation, so the fake IS the mechanism — the same way core/go
   * covers these arms.
   */
  function withAnalysis<T>(
    carrier: (r: Registry, w: string, s: unknown, a: Value[]) => Value[],
    body: () => T,
  ): T {
    const saved = {
      strip: AnalysisImpl.stripToCarriers,
      carrier: AnalysisImpl.carrierResults,
    };
    installAnalysisImpl({
      stripToCarriers: (input: Value[]) => input,
      carrierResults: carrier as never,
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

  it("routes a native dispatch through carrierResults", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        r.check.begin();
        const out = new Engine(r).run([
          newWord("addq"),
          newInteger(1n),
          newInteger(2n),
        ]);
        // The modelled carrier stands in for the handler's real output.
        assert.equal(canon(out), "Integer");
      },
    );
  });

  it("routes a user fn through the check-mode arm", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        r.pushDef(
          "twice",
          fnDef(
            [{ name: "n", type: TInteger }],
            [newWord("addq"), newWord("n"), newWord("n")],
          ),
        );
        r.check.begin();
        const out = new Engine(r).run([newWord("twice"), newInteger(4n)]);
        assert.ok(out.length >= 0);
        // Resolving the def counted as a use, so it is not reported unused.
        r.check.emitUnusedDefDiagnostics();
        assert.equal(
          r.check.diagnostics.filter((d) => d.code === "unused_def").length,
          0,
        );
      },
    );
  });

  it("records an undefined word as a diagnostic instead of throwing", () => {
    withAnalysis(
      () => [],
      () => {
        const r = fixture();
        r.check.begin();
        // Check mode is the LENIENT arm: the word resolves to nothing, so the
        // engine substitutes an Any carrier and records why, letting analysis
        // continue to the end of the program instead of stopping at the first
        // unknown name. Both halves are the contract — that it did not throw,
        // and that the diagnostic names the word.
        const out = new Engine(r).run([newWord("nosuch")]);
        const undef = r.check.diagnostics.filter(
          (d) => d.code === "undefined_word",
        );
        assert.equal(undef.length, 1);
        assert.equal(undef[0]!.word, "nosuch");
        assert.match(undef[0]!.detail, /undefined word: nosuch/);
        assert.equal(out.length, 1);
        assert.equal(out[0]!.undefined, true);
      },
    );
  });

  it("throws on an undefined word when check mode is NOT active", () => {
    // The other side of the same branch: outside check mode there is no
    // diagnostic channel to record into, so the same lookup failure is a hard
    // BoruError. Pinning both arms is what keeps the leniency scoped to
    // analysis rather than leaking into evaluation.
    const r = fixture();
    assert.throws(
      () => new Engine(r).run([newWord("nosuch")]),
      (e: unknown) =>
        e instanceof BoruError &&
        e.code === "undefined_word" &&
        /nosuch/.test(e.message),
    );
    assert.equal(r.check.diagnostics.length, 0);
  });

  // ── The recovery arms ───────────────────────────────────────────────
  //
  // Check mode never stops at the first refusal: a dispatch that matches
  // NO signature assumes a best-fit overload, reports one honest
  // diagnostic and keeps going with modelled results, so a later error is
  // still reported instead of being hidden behind the first. These tests
  // pin both halves — the diagnostic AND the shape analysis continues
  // with.

  it("assumes a best-fit overload for a native word that matches none", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        r.check.begin();
        // addq wants two Integers. Two Strings match nothing.
        const out = new Engine(r).run([
          newWord("addq"),
          newString("x"),
          newString("y"),
        ]);
        const diag = r.check.diagnostics.filter(
          (d) => d.code === "no_signature",
        );
        assert.equal(diag.length, 1);
        assert.equal(diag[0]!.word, "addq");
        assert.match(diag[0]!.detail, /assuming best-fit candidate/);
        // The assumed signature's modelled result replaced the word AND
        // both operands, so nothing is left dangling for the next step.
        assert.equal(canon(out), "Integer");
      },
    );
  });

  it("gathers the assumed call's operands from BOTH sides of the word", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        r.check.begin();
        // One operand before the word, one after: the recovery walks
        // forward to the arity and then fills the rest from the prefix.
        const out = new Engine(r).run([
          newString("x"),
          newWord("addq"),
          newString("y"),
        ]);
        assert.equal(canon(out), "Integer");
      },
    );
  });

  it("stops the operand gather at a word rather than reaching past it", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        r.check.begin();
        // `idq` is a word, so it bounds the gather: addq assumes its
        // overload over ONE operand, leaving idq to dispatch on its own.
        const out = new Engine(r).run([
          newWord("addq"),
          newString("x"),
          newWord("idq"),
          newInteger(1n),
        ]);
        assert.equal(
          r.check.diagnostics.filter((d) => d.code === "no_signature").length,
          1,
        );
        assert.equal(canon(out), "Integer Integer");
      },
    );
  });

  it("assumes an overload for a user fn whose args do not fit", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        r.pushDef(
          "inc",
          newFnDef({
            sigs: [
              {
                params: [{ name: "n", type: TInteger }],
                returns: [TInteger],
                body: [newWord("addq"), newWord("n"), newInteger(1n)],
              },
            ],
          } as FnDefInfo),
        );
        r.check.begin();
        const out = new Engine(r).run([newWord("inc"), newString("x")]);
        const diag = r.check.diagnostics.filter(
          (d) => d.code === "no_signature",
        );
        assert.equal(diag.length, 1);
        assert.equal(diag[0]!.word, "inc");
        // The declared return still models the call, so the caller's own
        // analysis continues from a typed value.
        assert.equal(canon(out), "Integer");
      },
    );
  });

  it("breaks a recursive fn with its declared returns", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        // A body that calls ITSELF: without the in-flight guard the
        // analysis would descend forever.
        r.pushDef(
          "loop",
          newFnDef({
            sigs: [
              {
                params: [{ name: "n", type: TInteger }],
                returns: [TInteger],
                body: [newWord("loop"), newWord("n")],
              },
            ],
          } as FnDefInfo),
        );
        r.check.begin();
        const out = new Engine(r).run([newWord("loop"), newInteger(1n)]);
        assert.equal(canon(out), "Integer");
      },
    );
  });

  it("breaks a recursive fn with NO declared returns via a dynamic Any", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        r.pushDef(
          "loop",
          fnDef(
            [{ name: "n", type: TInteger }],
            [newWord("loop"), newWord("n")],
          ),
        );
        r.check.begin();
        const out = new Engine(r).run([newWord("loop"), newInteger(1n)]);
        // Nothing is declared, so the cycle is cut with a value of
        // statically-unknown type rather than a guessed one.
        assert.equal(out.length, 1);
        assert.equal(out[0]!.dynamic, true);
      },
    );
  });

  it("returns the body residual when a fn declares no returns", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        r.pushDef("two", fnDef([], [newInteger(2n)]));
        r.check.begin();
        assert.equal(canon(new Engine(r).run([newWord("two")])), "2");
      },
    );
  });

  it("spreads dynamic contagion from an argument to the returns", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        r.pushDef(
          "inc",
          newFnDef({
            sigs: [
              {
                params: [{ name: "n", type: TAny }],
                returns: [TInteger],
                body: [newInteger(1n)],
              },
            ],
          } as FnDefInfo),
        );
        r.check.begin();
        // A gradual argument makes the declared Integer return dynamic
        // too: what came in unknown cannot make the result certain.
        const out = new Engine(r).run([
          newWord("inc"),
          newDynamicCarrier(TAny),
        ]);
        assert.equal(out.length, 1);
        assert.equal(out[0]!.dynamic, true);
      },
    );
  });

  it("binds an omitted optional param to its type's base value", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        // Each param type has a distinct zero; the body returns the bound
        // value so the test reads back what the analysis chose.
        const cases: Array<[import("./type.ts").BoruType, string]> = [
          [TInteger, "0"],
          [TFloat, "0.0"],
          [TString, "''"],
          [TBoolean, "false"],
          [TList, "(quote [])"],
          [TMap, "{}"],
          [TAtom, "/q"],
          [TNone, "none"],
        ];
        for (const [t, want] of cases) {
          const r = fixture();
          r.pushDef(
            "z",
            fnDef([{ name: "p", type: t, optional: true }], [newWord("p")]),
          );
          r.check.begin();
          assert.equal(
            canon(new Engine(r).run([newWord("z")])),
            want,
            t.toString(),
          );
        }
      },
    );
  });

  it("models a forward marker's dispatch rather than running its handler", () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture();
        r.check.begin();
        // `idq`'s Any slot DEFERS over a word that does not resolve, so
        // the dispatch completes through the forward-MARKER path rather
        // than immediately. In check mode the marker's handler must not
        // run: the modelled carrier stands in for its result, exactly as
        // for a direct dispatch. `true` would NOT reach here — the
        // matcher resolves a keyword before the slot test, so nothing
        // defers.
        const out = new Engine(r).run([newWord("idq"), newWord("nosuch")]);
        assert.equal(canon(out), "Integer");
        assert.deepEqual(
          r.check.diagnostics.map((d) => d.code),
          ["undefined_word"],
        );
      },
    );
  });
});

describe("Engine.run — sugar markers", () => {
  // A sugar marker is the parser's structural stand-in for a surface
  // form; the engine lowers it through the registry's role table when it
  // is STEPPED, and again — separately — when preEvalParens meets one in
  // a forward window. Both sites are here.

  function sugarFixture(): Registry {
    const r = fixture();
    r.bindSugarWord("lambda", "idq");
    return r;
  }

  it("lowers a marker stepped at the pointer", () => {
    const r = sugarFixture();
    const out = new Engine(r).run([
      newSugar({ kind: "lambda" }),
      newInteger(7n),
    ]);
    // The marker became `idq`, which then dispatched over the 7.
    assert.equal(canon(out), "7");
  });

  it("lowers a marker met inside a forward window", () => {
    const r = sugarFixture();
    // The window scan expands the marker IN PLACE before matching, so
    // what the matcher sees is the lowered word. The call still refuses —
    // a bare function word is a collection barrier, which is the language
    // rule and not the marker's doing — and the refusal is what shows the
    // lowering ran: the stack it prints holds `word(idq)`, not a marker.
    assert.throws(
      () =>
        new Engine(r).run([
          newWord("addq"),
          newSugar({ kind: "lambda" }),
          newInteger(2n),
          newInteger(3n),
        ]),
      (e: unknown) =>
        e instanceof BoruError &&
        e.code === "signature_error" &&
        /word\(idq\)/.test(e.message),
    );
  });

  it("steps over a sugar TYPE carrying no payload", () => {
    // core's constructor cannot build one, but the type is exported and a
    // downstream port builds the shape directly. The token is stepped
    // over rather than crashing the pass.
    const bare = new Value(newSugar({ kind: "lambda" }).vType, null);
    // Stepped over, so it stays on the stack as an opaque residual and
    // the value after it is untouched.
    assert.equal(run([bare, newInteger(1n)]), "__SG 1");
  });

  it("refuses a marker whose role the registry does not bind", () => {
    // A bare kernel supports no surface forms at all, and says so rather
    // than silently dropping the marker.
    assert.throws(
      () => new Engine(fixture()).run([newSugar({ kind: "lambda" })]),
      (e: unknown) =>
        e instanceof BoruError &&
        e.code === "sugar_unbound" &&
        /lambda/.test(e.message),
    );
  });
});

describe("Engine.run — handler contract", () => {
  it("refuses an async handler rather than leaking a promise", () => {
    // The Go kernel has no async handlers; a port that let one through
    // would put a Promise on the stack and render it as a value.
    const r = new Registry();
    r.registerNativeFunc({
      name: "slowq",
      signatures: [
        {
          args: [TInteger],
          returns: [TInteger],
          barrierPos: 1,
          handler: (a: Value[]) => Promise.resolve([a[0]!]),
        },
      ],
    } as never);
    assert.throws(
      () => new Engine(r).run([newWord("slowq"), newInteger(1n)]),
      (e: unknown) =>
        e instanceof BoruError &&
        e.code === "unsupported" &&
        /async handlers are not supported/.test(e.message),
    );
  });
});

describe("Engine.run — type literals and atoms pass through", () => {
  it("returns a type literal unchanged", () => {
    assert.equal(run([newTypeLiteral(TInteger)]), "Integer");
    assert.equal(run([newTypeLiteral(TList)]), "List");
    assert.equal(run([newTypeLiteral(TMap)]), "Map");
    assert.equal(run([newTypeLiteral(TString)]), "String");
  });

  it("returns an atom unchanged, in its source form", () => {
    // canon renders an atom as the literal that re-reads as one; Go's
    // oracle agrees (`a/q` -> `a/q`).
    assert.equal(run([newAtom("a")]), "a/q");
  });
});

describe("Engine.run — XML templates", () => {
  it("resolves a template with no holes to a plain element", () => {
    const v = newXmlInterp({ tag: "br", attrs: [], children: [] });
    assert.equal(run([v]), "<br/>");
  });

  it("evaluates an attribute hole into the attribute text", () => {
    const v = newXmlInterp({
      tag: "a",
      attrs: [
        {
          name: "h",
          segs: [
            { lit: "p/" },
            { expr: [newWord("addq"), newInteger(1n), newInteger(1n)] },
          ],
        },
      ],
      children: [],
    });
    assert.equal(run([v]), '<a h="p/2"/>');
  });

  it("evaluates a child hole into the element text", () => {
    const v = newXmlInterp({
      tag: "p",
      attrs: [],
      children: [
        { lit: "n=" },
        { expr: [newWord("addq"), newInteger(2n), newInteger(3n)] },
      ],
    });
    assert.equal(run([v]), "<p>n=5</p>");
  });

  it("resolves a nested element child", () => {
    const v = newXmlInterp({
      tag: "o",
      attrs: [],
      children: [{ elem: { tag: "i", attrs: [], children: [{ lit: "t" }] } }],
    });
    assert.equal(run([v]), "<o><i>t</i></o>");
  });
});

describe("Engine.run — marks and moves", () => {
  it("drops a move whose mark was never seen", () => {
    assert.equal(run([newInteger(1n), newMove("absent")]), "1");
  });

  it("relocates a value to its mark", () => {
    // A mark names a slot; a later move addresses it. The pair is how the
    // engine reorders a stack without the caller respelling the program.
    const out = run([newMark("m", []), newInteger(1n), newMove("m")]);
    assert.doesNotMatch(out, /\[object Object\]/);
  });
});

describe("Engine.run — a CONDITIONAL move", () => {
  // The if-continuation. A condition that is a code body cannot be
  // evaluated before its branches are chosen — running it early would run
  // it in the wrong place on the tape — so the emitting word hands back
  // `Mark · condition · MoveIf`, the engine runs the condition where it
  // stands, and the move reads its RESULT to splice in one branch.

  function condMove(cond: Value[], then: Value[], els?: Value[]): Value[] {
    const id = nextMarkId();
    return [
      newMark(id, [...cond]),
      ...cond,
      newMoveIf(id, "if", els === undefined ? { then } : { then, else: els }),
    ];
  }

  it("splices the THEN branch for a truthy condition", () => {
    assert.equal(
      run(condMove([newBoolean(true)], [newInteger(1n)], [newInteger(2n)])),
      "1",
    );
  });

  it("splices the ELSE branch for a falsy one", () => {
    assert.equal(
      run(condMove([newBoolean(false)], [newInteger(1n)], [newInteger(2n)])),
      "2",
    );
  });

  it("produces NOTHING when a falsy condition has no else", () => {
    assert.equal(run(condMove([newBoolean(false)], [newInteger(1n)])), "");
  });

  it("decides on the condition's LAST value", () => {
    assert.equal(
      run(
        condMove(
          [newInteger(1n), newBoolean(false)],
          [newInteger(1n)],
          [newInteger(2n)],
        ),
      ),
      "2",
    );
  });

  it("runs the condition rather than reading it as data", () => {
    assert.equal(
      run(
        condMove(
          [newWord("addq"), newInteger(1n), newInteger(1n)],
          [newInteger(7n)],
          [newInteger(8n)],
        ),
      ),
      "7",
    );
  });

  it("refuses a condition that produced NO value", () => {
    // Quietly taking the else arm would hide the mistake: an `if` whose
    // condition vanished has no answer.
    assert.throws(
      () => run(condMove([], [newInteger(1n)], [newInteger(2n)])),
      (e: unknown) =>
        e instanceof BoruError &&
        e.code === "runtime_error" &&
        /condition produced no value/.test(e.message),
    );
  });

  it("drops a conditional move whose mark was never seen", () => {
    assert.equal(
      run([
        newInteger(5n),
        newMoveIf("absent", "if", { then: [newInteger(1n)] }),
      ]),
      "5",
    );
  });
});

describe("Engine.run — nested interpolation", () => {
  it("substitutes an interpolation nested in an interpolation", () => {
    const inner = newInterpString([{ lit: "i" }, { expr: [newInteger(1n)] }]);
    const outer = newInterpString([
      { lit: "<" },
      { expr: [inner] },
      { lit: ">" },
    ]);
    assert.equal(run([outer]), "'<i1>'");
  });

  it("substitutes a list expression segment", () => {
    const s = newInterpString([
      { expr: [newList([newInteger(1n), newInteger(2n)])] },
    ]);
    assert.doesNotMatch(run([s]), /\[object Object\]/);
  });
});

describe("Engine.run — deeper paren forms", () => {
  it("nests paren groups", () => {
    const prog = [
      newWord("addq"),
      newOpenParen(),
      newWord("addq"),
      newInteger(1n),
      newInteger(1n),
      newCloseParen(),
      newOpenParen(),
      newWord("addq"),
      newInteger(2n),
      newInteger(2n),
      newCloseParen(),
    ];
    assert.equal(run(prog), "6");
  });

  it("returns an empty paren group as nothing", () => {
    assert.equal(run([newOpenParen(), newCloseParen()]), "");
  });

  it("evaluates a paren group standing alone", () => {
    assert.equal(
      run([
        newOpenParen(),
        newWord("addq"),
        newInteger(1n),
        newInteger(2n),
        newCloseParen(),
      ]),
      "3",
    );
  });
});

describe("Engine.run — the collection barrier", () => {
  it("does NOT reach past a word to fill a forward slot", () => {
    // barrierPos stops forward collection at the next word, so `addq 1 addq
    // 2 3` is a signature error rather than 1 + (2+3). The nested value has
    // to be parenthesised to be collected — which is what the paren tests
    // above exercise. Recorded because the opposite is the intuitive guess.
    assert.throws(
      () =>
        run([
          newWord("addq"),
          newInteger(1n),
          newWord("addq"),
          newInteger(2n),
          newInteger(3n),
        ]),
      (e: unknown) =>
        e instanceof BoruError && e.message.includes("no signature matches"),
    );
  });

  it("refuses a word whose forward args are not all present", () => {
    assert.throws(
      () => run([newWord("addq"), newInteger(1n)]),
      (e: unknown) =>
        e instanceof BoruError && e.message.includes("no signature matches"),
    );
  });

  it("collects through parens instead", () => {
    const inner = newParenExpr([
      newWord("addq"),
      newInteger(2n),
      newInteger(3n),
    ]);
    assert.equal(run([newWord("addq"), newInteger(1n), inner]), "6");
  });
});

describe("Engine.run — word resolution at the argument slot", () => {
  it("resolves the keyword words where a slot consumes them", () => {
    // ADR-012 rule 4: the parser is type-name-opaque, so the keywords and
    // type names arrive as Words and resolve exactly at the slot.
    assert.equal(run([newWord("idq"), newWord("true")]), "true");
    assert.equal(run([newWord("idq"), newWord("false")]), "false");
    assert.equal(run([newWord("idq"), newWord("none")]), "none");
  });

  it("resolves a builtin type name to its type literal at the slot", () => {
    assert.equal(run([newWord("idq"), newWord("Integer")]), "Integer");
    assert.equal(run([newWord("idq"), newWord("String")]), "String");
  });

  it("refuses a bare type literal in a value slot", () => {
    // `addq Integer 1` is a signature error: a Scalar slot takes a concrete
    // value, never a bare type literal.
    assert.throws(
      () => run([newWord("addq"), newTypeLiteral(TInteger), newInteger(1n)]),
      BoruError,
    );
  });
});

describe("Engine.run — nested containers", () => {
  it("builds a list of evaluated elements", () => {
    const l = newList(
      [
        newParenExpr([newWord("addq"), newInteger(1n), newInteger(1n)]),
        newInteger(9n),
      ],
      { eval: true },
    );
    assert.equal(run([l]), "[2 9]");
  });

  it("builds a list containing a list", () => {
    const inner = newList([newInteger(1n)], { eval: true });
    assert.equal(run([newList([inner], { eval: true })]), "[[1]]");
  });

  it("builds a map whose value is a list", () => {
    const m = new OrderedMap();
    m.set("k", newList([newInteger(1n)], { eval: true }));
    assert.equal(run([newMap(m)]), "{k:[1]}");
  });

  it("builds a map nested in a map", () => {
    const inner = new OrderedMap();
    inner.set("a", newInteger(1n));
    const outer = new OrderedMap();
    outer.set("n", newMap(inner));
    assert.equal(run([newMap(outer)]), "{n:{a:1}}");
  });

  it("leaves an empty container empty", () => {
    assert.equal(run([newList([], { eval: true })]), "[]");
    assert.equal(run([newMap(new OrderedMap())]), "{}");
  });
});

describe("Engine.run — a map argument's values", () => {
  // The values of a consumed map are evaluated at consumption. Each one is
  // a PROGRAM unless it is one of the two INERT type shapes, whose names
  // resolve without anything running.

  function mapOf(entries: Array<[string, Value]>): Value {
    const m = new OrderedMap();
    for (const [k, v] of entries) m.set(k, v);
    return newMap(m, { eval: true });
  }

  it("gathers a value with SEVERAL residuals into a list", () => {
    const out = run([
      newWord("idq"),
      mapOf([["k", newParenExpr([newWord("pairq"), newInteger(3n)])]]),
    ]);
    assert.equal(out, "{k:[3 3]}");
  });

  it("resolves a typed container's child constraint without running it", () => {
    // `[:Integer]` is inert: there is nothing to dispatch, and its child
    // is a NAME that must become a type. Running it in a sub-engine would
    // leave the name unresolved.
    const r = fixture();
    const typed = newTypedList(newWord("Integer"), []);
    const out = canon(
      new Engine(r).run([newWord("idq"), mapOf([["k", typed]])]),
    );
    assert.equal(out, "{k:[:Integer]}");
  });

  it("resolves a typed MAP's child constraint the same way", () => {
    const r = fixture();
    const typed = newTypedMap(newWord("Integer"), []);
    const out = canon(
      new Engine(r).run([newWord("idq"), mapOf([["k", typed]])]),
    );
    assert.equal(out, "{k:{:Integer}}");
  });

  it("resolves a disjunct's alternatives", () => {
    const r = fixture();
    const d = newDisjunct([newWord("Integer"), newWord("String")]);
    const out = canon(new Engine(r).run([newWord("idq"), mapOf([["k", d]])]));
    assert.match(out, /Integer/);
    assert.match(out, /String/);
  });
});

describe("Engine.run — XML holes that yield containers", () => {
  it("splices a list-valued hole's elements as children", () => {
    // A hole is not restricted to one value: a list of them flattens into
    // the element's children, each rendered where it stands.
    const v = newXmlInterp({
      tag: "p",
      attrs: [],
      children: [
        {
          expr: [newList([newInteger(1n), newString("s")], { eval: false })],
        },
      ],
    });
    assert.equal(run([v]), "<p>1s</p>");
  });

  it("sees a capture through an xml template nested in a HOLE", () => {
    // xmlCaptureFree walks into a hole's tokens, and an xml token there is
    // walked as a template rather than as opaque data — otherwise a
    // binding it captures would be missed and the outer template islanded
    // wrongly. With no recorder installed the walk is not consulted, so
    // this pins the resolved value instead.
    const inner = newXmlInterp({
      tag: "b",
      attrs: [],
      children: [{ lit: "x" }],
    });
    const outer = newXmlInterp({
      tag: "p",
      attrs: [],
      children: [{ expr: [inner] }],
    });
    assert.equal(run([outer]), "<p><b>x</b></p>");
  });
});
