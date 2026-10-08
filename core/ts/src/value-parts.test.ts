// Unit campaign for value.ts's accessors, the XML-template renderer and
// ClassTypeInfo, plus coretype.ts's unification arms.
// design/legacy/CORE-TS-COVERAGE.0.ignore stage 4 tail.
//
// The `as*` accessors are the port's narrowing boundary: each throws rather
// than returning a wrong-shaped payload, and the throwing arm is what a suite
// driven only by well-formed programs never reaches.
//
// The map arm is worth stating precisely, because coretype's own comment
// describes unifiesValue's EXACT-key-set rule and it is easy to assume that
// is what isValueOfType does. It is not: isValueOfType routes maps through the
// record-shape/open-subset path at every depth, so {a:1 b:2} DOES satisfy
// {a:1}, nested or not. These tests pin the observed contract rather than the
// one the internal helper's comment describes.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { isValueOfType } from "./coretype.ts";
import { TInteger, TString } from "./type.ts";
import {
  ClassTypeInfo,
  OrderedMap,
  newAtom,
  newDisjunct,
  newFloat,
  newForwardMarker,
  newInteger,
  newList,
  newMap,
  newMark,
  newMove,
  newString,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
  newXmlInterp,
  renderXmlTmplSrc,
  type XmlTmpl,
} from "./value.ts";

const mapOf = (pairs: Array<[string, ReturnType<typeof newInteger>]>) => {
  const m = new OrderedMap();
  for (const [k, v] of pairs) m.set(k, v);
  return newMap(m);
};

describe("renderXmlTmplSrc", () => {
  it("renders a self-closing tag with no attributes", () => {
    assert.equal(
      renderXmlTmplSrc({ tag: "br", attrs: [], children: [] }),
      "<br/>",
    );
  });

  it("renders a literal attribute", () => {
    const t: XmlTmpl = {
      tag: "a",
      attrs: [{ name: "h", segs: [{ lit: "u" }] }],
      children: [],
    };
    assert.equal(renderXmlTmplSrc(t), '<a h="u"/>');
  });

  it("renders an interpolated attribute hole", () => {
    const t: XmlTmpl = {
      tag: "a",
      attrs: [{ name: "h", segs: [{ lit: "p/" }, { expr: [newInteger(1n)] }] }],
      children: [],
    };
    assert.equal(renderXmlTmplSrc(t), '<a h="p/${1}"/>');
  });

  it("renders literal and interpolated children", () => {
    const t: XmlTmpl = {
      tag: "p",
      attrs: [],
      children: [{ lit: "a" }, { expr: [newInteger(2n)] }, { lit: "b" }],
    };
    assert.equal(renderXmlTmplSrc(t), "<p>a${2}b</p>");
  });

  it("renders a nested element child", () => {
    const t: XmlTmpl = {
      tag: "o",
      attrs: [],
      children: [{ elem: { tag: "i", attrs: [], children: [] } }],
    };
    assert.equal(renderXmlTmplSrc(t), "<o><i/></o>");
  });

  it("is what an XmlInterp VALUE renders as, inside its wrapper", () => {
    const v = newXmlInterp({ tag: "br", attrs: [], children: [] });
    assert.equal(v.toString(), "interp-xml(<br/>)");
  });
});

describe("ClassTypeInfo", () => {
  it("joins the parent path and name into a full lattice path", () => {
    const c = new ClassTypeInfo("Point", "Ideal/Class", new OrderedMap());
    assert.equal(c.path, "Ideal/Class/Point");
    assert.equal(c.name, "Point");
    assert.equal(c.parentPath, "Ideal/Class");
  });

  it("tracks a rename through the getter", () => {
    const c = new ClassTypeInfo("A", "Ideal/Class", new OrderedMap());
    c.name = "B";
    assert.equal(c.path, "Ideal/Class/B");
  });
});

describe("Value accessors — the throwing arms", () => {
  it("each as* refuses a wrong-shaped payload", () => {
    const i = newInteger(1n);
    const s = newString("s");
    assert.throws(() => s.asInteger(), /not an integer/);
    assert.throws(() => i.asString(), /not a string/);
    assert.throws(() => i.asFloat(), /not a float/);
    assert.throws(() => i.asBoolean(), /not a boolean/);
    assert.throws(() => i.asList(), /not a list/);
    assert.throws(() => i.asMap(), /not a map/);
    assert.throws(() => i.asAtom(), /not an atom/);
  });

  it("each as* refuses a nil payload", () => {
    const bare = newTypeLiteral(TInteger);
    assert.throws(() => bare.asInteger(), /nil data/);
    assert.throws(() => bare.asString(), /nil data/);
    assert.throws(() => bare.asFloat(), /nil data/);
    assert.throws(() => bare.asBoolean(), /nil data/);
    assert.throws(() => bare.asList(), /nil data/);
    assert.throws(() => bare.asAtom(), /nil data/);
  });

  it("reads the marker payloads back", () => {
    assert.equal(newMark("m1", [newInteger(1n)]).asMark().id, "m1");
    assert.equal(newMove("m1").asMove().to, "m1");
    const f = newForwardMarker({
      funcName: "f",
      collected: [],
      expectedForward: 2,
      stackArgs: [],
    } as never);
    assert.equal(f.asForward().funcName, "f");
  });

  it("asDecimal is the asFloat alias", () => {
    assert.equal(newFloat(1.5).asDecimal(), 1.5);
    assert.equal(newFloat(1.5).asDecimal(), newFloat(1.5).asFloat());
  });
});

describe("coretype unification — the container arms", () => {
  it("satisfies a map type by open subset, at any depth", () => {
    const t = mapOf([["a", newInteger(1n)]]);
    assert.equal(isValueOfType(mapOf([["a", newInteger(1n)]]), t), true);
    // A SUPERSET satisfies it — the extra key is ignored.
    assert.equal(
      isValueOfType(
        mapOf([
          ["a", newInteger(1n)],
          ["b", newInteger(2n)],
        ]),
        t,
      ),
      true,
    );
    // A differing value for a named key does not.
    assert.equal(isValueOfType(mapOf([["a", newInteger(9n)]]), t), false);

    // The same open rule applies one level down.
    const nest = (inner: ReturnType<typeof mapOf>) => {
      const o = new OrderedMap();
      o.set("n", inner);
      return newMap(o);
    };
    const nestedType = nest(mapOf([["a", newInteger(1n)]]));
    assert.equal(
      isValueOfType(
        nest(
          mapOf([
            ["a", newInteger(1n)],
            ["b", newInteger(2n)],
          ]),
        ),
        nestedType,
      ),
      true,
    );
  });

  it("unifies lists element-wise and by length", () => {
    const t = newList([newInteger(1n), newInteger(2n)]);
    assert.equal(
      isValueOfType(newList([newInteger(1n), newInteger(2n)]), t),
      true,
    );
    assert.equal(isValueOfType(newList([newInteger(1n)]), t), false);
    assert.equal(
      isValueOfType(newList([newInteger(1n), newInteger(9n)]), t),
      false,
    );
  });

  it("refuses to unify a list with a non-list", () => {
    assert.equal(
      isValueOfType(newInteger(1n), newList([newInteger(1n)])),
      false,
    );
    assert.equal(
      isValueOfType(newList([newInteger(1n)]), newInteger(1n)),
      false,
    );
  });

  it("checks a typed map value-wise", () => {
    const t = newTypedMap(newTypeLiteral(TInteger), []);
    assert.equal(isValueOfType(mapOf([["a", newInteger(1n)]]), t), true);
    const bad = new OrderedMap();
    bad.set("a", newString("x"));
    assert.equal(isValueOfType(newMap(bad), t), false);
  });

  it("admits a bare type literal on either side", () => {
    assert.equal(isValueOfType(newInteger(1n), newTypeLiteral(TInteger)), true);
    assert.equal(
      isValueOfType(
        newTypedList(newTypeLiteral(TInteger), [newInteger(3n)]),
        newList([newTypeLiteral(TInteger)]),
      ),
      true,
    );
  });

  it("compares scalar leaves by type and payload", () => {
    assert.equal(isValueOfType(newAtom("a"), newAtom("a")), true);
    assert.equal(isValueOfType(newAtom("a"), newAtom("b")), false);
    assert.equal(isValueOfType(newInteger(1n), newInteger(1n)), true);
    assert.equal(isValueOfType(newInteger(1n), newInteger(2n)), false);
    assert.equal(isValueOfType(newString("s"), newInteger(1n)), false);
  });

  it("admits any alternative of a disjunction, and rejects the rest", () => {
    const d = newDisjunct([newInteger(1n), newString("s")]);
    assert.equal(isValueOfType(newInteger(1n), d), true);
    assert.equal(isValueOfType(newString("s"), d), true);
    assert.equal(isValueOfType(newInteger(9n), d), false);
  });

  it("admits a value under a disjunction of type literals", () => {
    const d = newDisjunct([newTypeLiteral(TInteger), newTypeLiteral(TString)]);
    assert.equal(isValueOfType(newInteger(5n), d), true);
    assert.equal(isValueOfType(newString("x"), d), true);
  });
});
