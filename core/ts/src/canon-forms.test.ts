// Unit campaign for the canonical renderer's source-form arms the core
// suite never built a value for: template strings and XML templates
// (NUR302), the sugar markers (NUR059), group modifiers in a sequence and
// the capitalised-token comma (NUR072, NUR304) — the eng corpus reaches
// them through the parser, which this package cannot import — and the named
// type the Stage 2 flip binds (Value.typeName, Value.named). Mirrors
// core/go/canon.go.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { canon, canonValue } from "./canon.ts";
import { isValueOfType } from "./coretype.ts";
import { TInteger, TType } from "./type.ts";
import {
  newAtom,
  newDisjunct,
  newInteger,
  newInterpString,
  newList,
  newMap,
  newSugar,
  newTypeLiteral,
  newWord,
  newXmlInterp,
  OrderedMap,
} from "./value.ts";

describe("canon template strings", () => {
  it("renders literal text and holes as backtick source, escaping the template's specials", () => {
    const v = newInterpString([{ lit: "a\\b`c${\n\t\r$z" }, { expr: [newInteger(1)] }, { lit: "!" }]);
    assert.equal(canonValue(v), "`a\\\\b\\`c\\${\\n\\t\\r$z${1}!`");
  });
  it("leaves a lone `$` (no `{` after it) unescaped", () => {
    assert.equal(canonValue(newInterpString([{ lit: "$5" }])), "`$5`");
  });
});

describe("canon XML templates", () => {
  it("renders attributes, text, holes and nested elements, escaping text and attribute values", () => {
    const v = newXmlInterp({
      tag: "p",
      attrs: [{ name: "id", segs: [{ lit: 'x"&y' }, { expr: [newInteger(2)] }] }],
      children: [{ lit: "a<b>" }, { expr: [newWord("w")] }, { elem: { tag: "br", attrs: [], children: [] } }],
    });
    assert.equal(canonValue(v), '<p id="x&quot;&amp;y${2}">a&lt;b&gt;${w}<br/></p>');
  });
  it("self-closes an element with no children", () => {
    assert.equal(canonValue(newXmlInterp({ tag: "hr", attrs: [], children: [] })), "<hr/>");
  });
});

describe("canon sugar markers", () => {
  it("spells each marker kind with a source form", () => {
    assert.equal(canonValue(newSugar({ kind: "angle", name: "Box", items: [newTypeLiteral(TInteger)] })), "Box<Integer>");
    assert.equal(canonValue(newSugar({ kind: "angle", name: "Box" })), "Box<>");
    assert.equal(canonValue(newSugar({ kind: "lambda" })), "=>");
    assert.equal(canonValue(newSugar({ kind: "mini", name: "re", src: "a'b c\td\\e\\" })), "+re'a\\'b\\ c\\\td\\e\\\\'");
    assert.equal(canonValue(newSugar({ kind: "type-bound", items: [newList([newAtom("w")])] })), "w/t");
  });
  it("falls back off the source form for a marker no source can produce", () => {
    const bad = [
      newSugar({ kind: "mini", name: "Bad", src: "x" }), // not a minilang kind name
      newSugar({ kind: "mini", name: "re", src: "a\nb" }), // a line break no literal holds
      newSugar({ kind: "type-bound", items: [] }), // no bound tokens
      newSugar({ kind: "type-bound", items: [newList([newInteger(1)])] }), // a bound that is not a name
    ];
    for (const v of bad) {
      const got = canonValue(v);
      assert.notEqual(got, "");
      assert.ok(!got.startsWith("+") && !got.endsWith("/t"), `fell back for ${got}`);
    }
  });
});

describe("canon sequences", () => {
  it("writes a group modifier after the value it modifies", () => {
    assert.equal(canon([newSugar({ kind: "usurp" }), newWord("x")]), "x /u");
    assert.equal(canon([newSugar({ kind: "stack-args" }), newWord("x")]), "x /s");
    assert.equal(canon([newSugar({ kind: "forward-args" }), newWord("x")]), "x /f");
    assert.equal(canon([newSugar({ kind: "force-arity", n: 2n }), newWord("x")]), "x /2");
  });
  it("leaves a trailing modifier, with nothing after it, as itself", () => {
    assert.equal(canon([newWord("x"), newSugar({ kind: "lambda" })]), "x =>");
  });
  it("separates a capitalised token from a following `<` with a comma, and nothing else", () => {
    const xml = newXmlInterp({ tag: "a", attrs: [], children: [] });
    assert.equal(canon([newWord("Box"), xml]), "Box, <a/>");
    assert.equal(canon([newWord("box"), xml]), "box <a/>");
  });
});

describe("named types (Value.typeName)", () => {
  it("renders a named type as its name, in canon and toString", () => {
    const n = newInteger(5).named("N");
    assert.equal(n.toString(), "N");
    assert.equal(canonValue(n), "N");
    // the unnamed body renders as itself
    assert.equal(newInteger(5).toString(), "5");
  });
  it("copies the body, keeps its position, and never auto-evaluates", () => {
    const body = newList([newInteger(1)], { eval: true });
    body.pos = { row: 3, col: 4, src: "x" };
    const n = body.named("P");
    assert.equal(n.typeName, "P");
    assert.equal(n.data, body.data);
    assert.ok(n.vType.equal(body.vType));
    assert.deepEqual(n.pos, body.pos);
    assert.equal(n.eval, false);
    assert.equal(body.typeName, undefined, "the original is not renamed");
  });
  it("renders a named disjunct inside a map as its bare name, an unnamed one grouped", () => {
    const color = newDisjunct([newAtom("a"), newAtom("b")]);
    const m = new OrderedMap();
    m.set("c", color.named("Color"));
    m.set("d", color);
    assert.equal(canonValue(newMap(m)), "{c:Color d:(a tor b)}");
  });
  it("is a Type whatever its body, where the unnamed body is not", () => {
    const typeT = newTypeLiteral(TType);
    assert.equal(isValueOfType(newInteger(5).named("N"), typeT), true);
    assert.equal(isValueOfType(newInteger(5), typeT), false);
  });
});
