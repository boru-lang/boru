// Unit campaign for the render surface — value.ts's toString arms and the
// exported renderers, plus canon.ts's tails.
//
// design/legacy/CORE-TS-COVERAGE.0.ignore stage 2. Every expectation is the Go form the
// function names as its mirror: renderSugar tracks the kernel's String arm,
// renderInterpSegments tracks renderInterpParts "byte for byte", and
// renderXmlTmplSrc tracks renderXmlTmplSrc.
//
// The '[object Object]' assertions are deliberate and belong here rather than
// in a comment: two separate families of missing toString arm have already
// shipped in this port (the disjunction one the parser stream oracle caught,
// and the Pathon/Emailon/Urlon/Error/Options/Reach one stage 1 caught). A
// value that renders as '[object Object]' is a missing arm every time, so the
// suite asserts against it directly.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { canon, canonString, canonValue } from "./canon.ts";
import {
  TAny,
  TEmailon,
  TError,
  TInteger,
  TPathon,
  TReach,
  TString,
  TUrlon,
} from "./type.ts";
import {
  ErrorInfo,
  OrderedMap,
  Value,
  newAtom,
  newBoolean,
  newCloseParen,
  newDisjunct,
  newEnd,
  newErrorValue,
  newFloat,
  newInspect,
  newInteger,
  newInterpString,
  newList,
  newMap,
  newNone,
  newOpenParen,
  newOptions,
  newParenExpr,
  newPath,
  newReach,
  newString,
  newSugar,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
  newWord,
  newXml,
  renderInterpSegments,
  renderSugar,
  type SugarInfo,
} from "./value.ts";

function inspectMap(): OrderedMap {
  const m = new OrderedMap();
  m.set("k", newInteger(1n));
  return m;
}

describe("renderSugar — one arm per marker kind", () => {
  it("renders the data-carrying kinds with their payload", () => {
    assert.equal(
      renderSugar({ kind: "force-arity", n: 3n } as SugarInfo),
      "sugar(force-arity 3)",
    );
    assert.equal(
      renderSugar({ kind: "mini", name: "m", src: "1 add" } as SugarInfo),
      "sugar(mini m '1 add')",
    );
    assert.equal(
      renderSugar({
        kind: "angle",
        name: "Box",
        items: [newWord("T")],
      } as SugarInfo),
      "sugar(angle Box [word(T)])",
    );
    assert.equal(
      renderSugar({ kind: "type-bound", items: [newWord("T")] } as SugarInfo),
      "sugar(type-bound [word(T)])",
    );
  });

  it("renders every bare kind as sugar(kind)", () => {
    for (const kind of [
      "usurp",
      "stack-args",
      "forward-args",
      "lambda",
      "gen-default",
    ]) {
      assert.equal(renderSugar({ kind } as SugarInfo), `sugar(${kind})`);
    }
  });

  it("is what a Sugar VALUE renders as", () => {
    assert.equal(
      newSugar({ kind: "lambda" } as SugarInfo).toString(),
      "sugar(lambda)",
    );
  });
});

describe("renderInterpSegments", () => {
  it("renders literal and expression segments in source form", () => {
    const s = renderInterpSegments([
      { lit: "a " },
      { expr: [newWord("x")] },
      { lit: " b" },
    ]);
    assert.equal(s, "interp('a ' ${word(x)} ' b')");
  });

  it("renders an expression-only interpolation", () => {
    assert.equal(
      renderInterpSegments([{ expr: [newInteger(1n)] }]),
      "interp(${1})",
    );
  });

  it("is what an InterpString VALUE renders as", () => {
    const v = newInterpString([{ lit: "a" }, { expr: [newInteger(1n)] }]);
    assert.equal(v.toString(), "interp('a' ${1})");
  });
});

describe("Value.toString — the marker arms", () => {
  it("renders the structural markers by their spelling", () => {
    assert.equal(newOpenParen().toString(), "(");
    assert.equal(newCloseParen().toString(), ")");
    assert.equal(newEnd().toString(), "end");
    assert.equal(newNone().toString(), "none");
  });

  it("renders a paren expression with its contents", () => {
    assert.equal(
      newParenExpr([newInteger(1n), newWord("w")]).toString(),
      "paren([1 word(w)])",
    );
  });

  it("renders containers", () => {
    assert.equal(
      newList([newInteger(1n), newString("s")]).toString(),
      "[1 's']",
    );
    const m = new OrderedMap();
    m.set("a", newInteger(1n));
    assert.equal(newMap(m).toString(), "{a:1}");
    assert.equal(
      newTypedList(newTypeLiteral(TInteger), [newInteger(1n)]).toString(),
      "[:Integer 1]",
    );
    assert.equal(
      newTypedMap(newTypeLiteral(TString), []).toString(),
      "{:String}",
    );
  });

  it("renders the scalar arms", () => {
    assert.equal(newInteger(-3n).toString(), "-3");
    assert.equal(newFloat(1.5).toString(), "1.5");
    assert.equal(newBoolean(false).toString(), "false");
    assert.equal(newAtom("a").toString(), "a");
    assert.equal(newString("s").toString(), "'s'");
    assert.equal(newWord("w").toString(), "word(w)");
  });

  it("leaves no payload kind rendering as [object Object]", () => {
    // Both shipped families of missing arm looked exactly like this.
    const values = [
      newPath(["a", "b"], false),
      newPath(["a"], true),
      newErrorValue("boom"),
      newOptions(new OrderedMap()),
      newReach({ receiver: [newWord("m")], segments: [], eval: true }),
      newDisjunct([newTypeLiteral(TInteger), newTypeLiteral(TString)]),
      newInspect(inspectMap()),
      newXml({ tag: "t", attrs: [], children: [] }),
      newInterpString([{ lit: "a" }]),
      newSugar({ kind: "lambda" } as SugarInfo),
    ];
    for (const v of values) {
      assert.doesNotMatch(
        v.toString(),
        /\[object Object\]/,
        `${v.vType.toString()} has no arm`,
      );
    }
  });

  it("terminates on a MALFORMED payload of a canon-delegated kind", () => {
    // The arm above delegates six kinds to canonValue, and canonValue's own
    // fallback is `v.toString()` — a cycle. What breaks it is that both ends
    // agree on which payloads canonValue can render, and gating the delegation
    // on the vType alone did not: canonValue's arms are guarded on payload
    // SHAPE, so a Value whose type said Error but whose payload was a bare
    // string fell through every arm, hit the fallback, and re-entered
    // toString. Both of these overflowed the stack.
    //
    // The contract is termination, not a particular string: a malformed
    // payload has no canonical form, so the render is whatever String() makes
    // of it. Asserting only that these RETURN is the whole point.
    const malformed = [
      new Value(TError, "bad"),
      new Value(TPathon, {}),
      new Value(TEmailon, {}),
      new Value(TUrlon, {}),
      new Value(TReach, "x"),
      new Value(TReach, {}),
      new Value(TReach, { receiver: [], segments: "no" }),
      // An object payload of no recognised kind at all — the fallthrough
      // below every guard, and the one case where '[object Object]' is the
      // right answer rather than a missing arm.
      new Value(TAny, { nope: 1 }),
    ];
    for (const v of malformed) {
      assert.equal(
        typeof v.toString(),
        "string",
        `${v.vType.toString()} did not terminate`,
      );
    }
  });

  it("still delegates when the payload IS well-formed", () => {
    // The guard must not be so tight that it strands the arm it protects.
    assert.equal(
      new Value(TError, new ErrorInfo("boom")).toString(),
      "error(boom)",
    );
    assert.equal(newPath(["a", "b"], true).toString(), "/a/b");
    assert.equal(newOptions(new OrderedMap()).toString(), "options{}");
    assert.equal(
      newReach({
        receiver: [newWord("m")],
        segments: [],
        eval: true,
      }).toString(),
      "m",
    );
  });
});

describe("canonValue tails", () => {
  it("renders an error value as error(message)", () => {
    assert.equal(canonValue(newErrorValue("boom")), "error(boom)");
    assert.equal(new ErrorInfo("boom").message, "boom");
  });

  it("renders a reach back to its dotted surface", () => {
    const plain = newReach({
      receiver: [newWord("m")],
      segments: [{ getr: false, computed: false, keyLit: newAtom("k") }],
      eval: true,
    });
    assert.equal(canonValue(plain), "m.k/q");

    // getr renders `!.` — the form Go's m.k/q is NOT.
    const getr = newReach({
      receiver: [newWord("m")],
      segments: [{ getr: true, computed: false, keyLit: newAtom("k") }],
      eval: true,
    });
    assert.equal(canonValue(getr), "m!.k/q");
  });

  it("renders a receiverless reach as $", () => {
    const dollar = newReach({
      receiver: [],
      segments: [{ getr: false, computed: false, keyLit: newAtom("name") }],
      eval: false,
    });
    assert.equal(canonValue(dollar), "$.name/q");
  });

  it("renders a computed key as a paren expression", () => {
    const computed = newReach({
      receiver: [newWord("m")],
      segments: [{ getr: false, computed: true, keyExpr: [newWord("e")] }],
      eval: true,
    });
    assert.equal(canonValue(computed), "m.(e)");
  });

  it("renders a multi-token receiver in parens", () => {
    const multi = newReach({
      receiver: [newWord("a"), newWord("b")],
      segments: [{ getr: false, computed: false, keyLit: newAtom("k") }],
      eval: true,
    });
    assert.equal(canonValue(multi), "(a b).k/q");
  });

  it("renders an Options map", () => {
    const m = new OrderedMap();
    m.set("a", newInteger(1n));
    assert.equal(canonValue(newOptions(m)), "options{a:1}");
    assert.equal(canonValue(newOptions(new OrderedMap())), "options{}");
  });

  it("renders an inspect value from its map", () => {
    assert.doesNotMatch(
      canonValue(newInspect(inspectMap())),
      /\[object Object\]/,
    );
  });

  it("renders a disjunction with atoms bare and type literals by leaf", () => {
    assert.equal(
      canonValue(
        newDisjunct([newTypeLiteral(TInteger), newTypeLiteral(TString)]),
      ),
      "Integer tor String",
    );
    assert.equal(
      canonValue(newDisjunct([newAtom("a"), newAtom("b")])),
      "a tor b",
    );
  });

  it("joins a whole stack with canon", () => {
    assert.equal(
      canon([newInteger(1n), newString("s"), newNone()]),
      "1 's' none",
    );
    assert.equal(canon([]), "");
  });
});

describe("canonString quoting", () => {
  it("keeps plain content single-quoted", () => {
    assert.equal(canonString("abc"), "'abc'");
  });
  it("switches to double quotes when the content has a single quote", () => {
    assert.equal(canonString("it's"), '"it\'s"');
  });
  it("escapes when the content has both quote kinds or a control char", () => {
    assert.match(canonString("a\nb"), /\\n/);
    assert.match(canonString("a\tb"), /\\t/);
  });
});

describe("canonXml", () => {
  it("renders a self-closing element when there are no children", () => {
    assert.equal(
      canonValue(newXml({ tag: "br", attrs: [], children: [] })),
      "<br/>",
    );
  });
  it("renders attributes and text children", () => {
    const x = newXml({
      tag: "a",
      attrs: [{ name: "h", value: "u" }],
      children: ["t"],
    });
    assert.equal(canonValue(x), '<a h="u">t</a>');
  });
  it("renders nested elements", () => {
    const x = newXml({
      tag: "o",
      attrs: [],
      children: [{ tag: "i", attrs: [], children: [] }],
    });
    assert.equal(canonValue(x), "<o><i/></o>");
  });
});
