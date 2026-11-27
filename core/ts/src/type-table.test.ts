// The builtin type table is DATA the Go kernel owns
// (core/go/typetable.go, `builtinDecls`). core/ts restates it by hand, and
// a hand-kept copy drifts: it was thirteen nodes short of Go's and carried
// no ranks at all, so `Module` was undefined_word here and a type literal
// there, and nothing could order two types the way Go does.
//
// The first test below reads the Go file and holds this copy to it row by
// row — the check spec/tools/gen-typetable.sh makes for the Lean model's
// table. The rest pin what is BUILT on the table: the rank and the type
// order, the code-point text order, and the path arm of name resolution.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";
import * as fs from "node:fs";
import * as path from "node:path";
import { fileURLToPath } from "node:url";

import {
  BoruType,
  TBoolean,
  TFloat,
  TInteger,
  TList,
  TMap,
  TNone,
  TNumber,
  TScalar,
  TString,
  TType,
  builtinTypeDecls,
  compareText,
  compareTypes,
  newType,
  resolveBuiltinTypeName,
  typeRank,
} from "./type.ts";
import {
  Value,
  isCloseParen,
  isEnd,
  isOpenParen,
  newCarrier,
  newCloseParen,
  newEnd,
  newOpenParen,
  newTypeLiteral,
} from "./value.ts";

const GO_TABLE = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "go",
  "typetable.go",
);

interface GoDecl {
  path: string;
  rank: number;
  alias?: string;
  internal?: boolean;
}

/** The rows of Go's `builtinDecls`, read the way gen-typetable.sh reads them. */
function goBuiltinDecls(): GoDecl[] {
  const lines = fs.readFileSync(GO_TABLE, "utf8").split("\n");
  const start = lines.findIndex((l) => l.startsWith("var builtinDecls = []builtinDecl{"));
  assert.ok(start >= 0, "core/go/typetable.go: builtinDecls not found");
  const out: GoDecl[] = [];
  for (let i = start + 1; !lines[i]!.startsWith("}"); i++) {
    const line = lines[i]!;
    const p = /\{Path: "([^"]*)"/.exec(line);
    if (p === null) continue;
    const rank = /Rank: ([0-9_]+)/.exec(line);
    assert.ok(rank !== null, `${p[1]}: no Rank`);
    const d: GoDecl = { path: p[1]!, rank: Number(rank[1]!.replaceAll("_", "")) };
    const alias = /Alias: "([^"]*)"/.exec(line);
    if (alias !== null) d.alias = alias[1]!;
    if (line.includes("IsInternal: true")) d.internal = true;
    out.push(d);
  }
  return out;
}

describe("the builtin type table", () => {
  it("is core/go's table, row for row", () => {
    const want = goBuiltinDecls();
    assert.ok(want.length > 70, "the Go table was not read");
    assert.deepEqual(
      builtinTypeDecls().map((d) => ({ ...d })),
      want,
    );
  });

  it("names every non-internal node by its leaf, and none of the internal ones", () => {
    for (const d of builtinTypeDecls()) {
      const leaf = d.path.split("/").at(-1)!;
      const got = resolveBuiltinTypeName(leaf);
      if (d.internal) assert.equal(got, undefined, leaf);
      else assert.equal(got?.toString(), d.path, leaf);
      // ...and every node, internal or not, by its full path.
      if (d.path.includes("/")) {
        assert.equal(resolveBuiltinTypeName(d.path)?.toString(), d.path);
      }
    }
  });
});

describe("resolveBuiltinTypeName — the path arm", () => {
  it("expands a short first part, as NewType does", () => {
    assert.equal(
      resolveBuiltinTypeName("Number/Integer")?.toString(),
      "Scalar/Number/Integer",
    );
    assert.equal(
      resolveBuiltinTypeName("FlexMap/WeakFlexMap")?.toString(),
      "Node/Map/FlexMap/WeakFlexMap",
    );
    // An alias expands too: `End` is the short name of `Word/__ED`.
    assert.equal(resolveBuiltinTypeName("Word/__ED")?.toString(), "Word/__ED");
  });

  it("admits only a path the table declares", () => {
    assert.equal(resolveBuiltinTypeName("Scalar/Nope"), undefined);
    assert.equal(resolveBuiltinTypeName("Integer/Foo"), undefined);
    assert.equal(resolveBuiltinTypeName("Nope"), undefined);
  });

  it("refuses a part that opens with a lowercase letter, in any script", () => {
    assert.equal(resolveBuiltinTypeName("Scalar/string"), undefined);
    assert.equal(resolveBuiltinTypeName("scalar/String"), undefined);
    assert.throws(() => newType("Scalar/étoile"), /uppercase/);
    // A digit, an underscore or an empty part is not a lowercase letter.
    assert.equal(newType("Word/__ED").toString(), "Word/__ED");
    assert.equal(newType("Scalar/").toString(), "Scalar/");
  });
});

describe("typeRank and compareTypes", () => {
  it("gives a declared type its own rank", () => {
    assert.equal(typeRank(TInteger), 20_310_000_000);
    assert.equal(typeRank(TNone), 12_000_000_000);
  });

  it("gives a minted type its nearest declared ancestor's rank", () => {
    assert.equal(typeRank(newType("Scalar/Number/Integer/Pos")), typeRank(TInteger));
    assert.equal(typeRank(new BoruType(["Scalar", "Mine", "Deep"])), typeRank(TScalar));
    assert.equal(typeRank(new BoruType(["Nowhere"])), 0);
  });

  it("orders types by rank, then depth, then leaf name", () => {
    assert.equal(compareTypes(TInteger, TInteger), 0);
    assert.equal(compareTypes(TInteger, TFloat), -1);
    assert.equal(compareTypes(TString, TBoolean), 1);
    assert.equal(compareTypes(TNumber, TInteger), -1);
    assert.equal(compareTypes(TMap, TList), 1);
    assert.equal(compareTypes(TNone, TType), -1);
    // Two minted types share their parent's rank: the shallower first,
    // then by name.
    const pos = newType("Scalar/Number/Integer/Pos");
    const neg = newType("Scalar/Number/Integer/Neg");
    assert.equal(compareTypes(TInteger, pos), -1);
    assert.equal(compareTypes(pos, TInteger), 1);
    assert.equal(compareTypes(neg, pos), -1);
    assert.equal(compareTypes(pos, neg), 1);
  });
});

describe("compareText", () => {
  it("orders by code point, which is UTF-8 byte order", () => {
    assert.equal(compareText("a", "b"), -1);
    assert.equal(compareText("b", "a"), 1);
    assert.equal(compareText("a", "a"), 0);
    assert.equal(compareText("a", "ab"), -1);
    assert.equal(compareText("ab", "a"), 1);
    assert.equal(compareText("B", "a"), -1);
  });

  it("puts an astral character after every BMP one", () => {
    // U+1F600 is the surrogate pair D83D DE00; as UTF-16 units D83D sorts
    // BEFORE U+FFFD, which is the order `<` gives and Go does not.
    assert.equal("\u{1F600}" < "�", true);
    assert.equal(compareText("\u{1F600}", "�"), 1);
    assert.equal(compareText("�", "\u{1F600}"), -1);
    // Two astral characters still order by code point.
    assert.equal(compareText("\u{1F600}", "\u{1F601}"), -1);
    assert.equal(compareText("\u{1F601}", "\u{1F600}"), 1);
  });
});

describe("a marker and the type literal of its type", () => {
  // `Word/__ED` the NAME is a type literal; the End marker shares its vType.
  // core/spec/dispatch.tsv pins what the step loop does with each; these
  // pin the predicate, and the case the corpus cannot build — a check-mode
  // carrier, which eng/ts's check pass strips a marker to and which must
  // still step as the marker it stands for.
  const TEnd = newType("Word/__ED");
  const TOpen = newType("Word/__OP");
  const TClose = newType("Word/__CP");

  it("the marker is one; the literal is not; the carrier is", () => {
    assert.equal(isEnd(newEnd()), true);
    assert.equal(isEnd(newTypeLiteral(TEnd)), false);
    assert.equal(isEnd(newCarrier(TEnd)), true);
    assert.equal(isOpenParen(newOpenParen()), true);
    assert.equal(isOpenParen(newTypeLiteral(TOpen)), false);
    assert.equal(isOpenParen(newCarrier(TOpen)), true);
    assert.equal(isCloseParen(newCloseParen()), true);
    assert.equal(isCloseParen(newTypeLiteral(TClose)), false);
    assert.equal(isCloseParen(newCarrier(TClose)), true);
  });

  it("holds for every marker predicate on Value", () => {
    const literalOf = (p: string): Value => newTypeLiteral(newType(p));
    assert.equal(literalOf("Word/__FW").isForward(), false);
    assert.equal(literalOf("Word/__MK").isMark(), false);
    assert.equal(literalOf("Word/__MV").isMove(), false);
    assert.equal(literalOf("Word/__IS").isInterpString(), false);
    assert.equal(literalOf("Word/__XI").isXmlInterp(), false);
    assert.equal(literalOf("Word/__MK").isMarker(newType("Word/__MK")), false);
    assert.equal(newCarrier(newType("Word/__MK")).isMark(), true);
  });
});
