// Unit campaign for type.ts's constructor/lattice tails and signature.ts's
// scoring — design/legacy/CORE-TS-COVERAGE.0.ignore stage 2.
//
// newType's short-name expansion is the load-bearing one: "Integer" must
// expand to "Scalar/Number/Integer" here exactly as Go's NewType +
// ExpandShortName does, because every type literal in the port is built
// through it and the lattice comparisons downstream are path-prefix tests.
//
// builtinRank exists only to keep `tor` unions in the same tcmp order the Go
// kernel stores them in, so the cross-engine differential agrees — a type
// absent from the table must sort LAST rather than first, which is the arm a
// naive Map lookup gets wrong.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { signatureScore, sortSignatures, type Signature } from "./signature.ts";
import {
  BoruType,
  TAny,
  TInteger,
  TNumber,
  TString,
  builtinRank,
  newType,
} from "./type.ts";
import { newInteger, newTypeLiteral } from "./value.ts";

describe("newType", () => {
  it("expands a short leaf name to its full path", () => {
    assert.deepEqual(newType("Integer").parts, ["Scalar", "Number", "Integer"]);
    assert.deepEqual(newType("String").parts, ["Scalar", "String"]);
  });

  it("leaves an already-full path alone", () => {
    assert.deepEqual(newType("Scalar/Number/Integer").parts, [
      "Scalar",
      "Number",
      "Integer",
    ]);
  });

  it("leaves a root alone", () => {
    assert.deepEqual(newType("Any").parts, ["Any"]);
    assert.deepEqual(newType("Scalar").parts, ["Scalar"]);
  });

  it("expands the head and keeps the tail", () => {
    // A path whose HEAD is a short name expands, carrying its tail along.
    assert.deepEqual(newType("Integer/0").parts, [
      "Scalar",
      "Number",
      "Integer",
      "0",
    ]);
  });

  it("refuses a part that does not start uppercase", () => {
    assert.throws(
      () => newType("integer"),
      /must start with an uppercase letter/,
    );
    assert.throws(
      () => newType("Scalar/number"),
      /must start with an uppercase letter/,
    );
  });

  it("leaves an unknown capitalised name as a bare single part", () => {
    assert.deepEqual(newType("Nonesuch").parts, ["Nonesuch"]);
  });
});

describe("BoruType", () => {
  it("specificity counts path parts", () => {
    assert.equal(TAny.specificity(), 1);
    assert.equal(TInteger.specificity(), 3);
    assert.equal(TString.specificity(), 2);
  });

  it("leaf is the last part, toString the whole path", () => {
    assert.equal(TInteger.leaf(), "Integer");
    assert.equal(TInteger.toString(), "Scalar/Number/Integer");
    assert.equal(new BoruType([]).leaf(), "");
  });

  it("matches is Any-permissive, pathSubtype is strict", () => {
    assert.equal(TInteger.matches(TAny), true);
    assert.equal(TInteger.matches(TNumber), true);
    assert.equal(TNumber.matches(TInteger), false);
    assert.equal(TInteger.pathSubtype(TAny), false);
    assert.equal(TInteger.pathSubtype(TNumber), true);
  });

  it("equal compares the whole path", () => {
    assert.equal(TInteger.equal(newType("Integer")), true);
    assert.equal(TInteger.equal(TNumber), false);
  });
});

describe("builtinRank", () => {
  it("orders builtins parent-first", () => {
    assert.ok(
      builtinRank(TNumber) < builtinRank(TInteger),
      "Number precedes Integer",
    );
    assert.ok(
      builtinRank(TAny) < builtinRank(TInteger),
      "Any precedes Integer",
    );
  });

  it("sorts an unknown type LAST, not first", () => {
    // The arm a plain Map lookup gets wrong: absent must be +infinity, or a
    // user type would lead every union and the differential would disagree.
    assert.equal(builtinRank(newType("Nonesuch")), Number.MAX_SAFE_INTEGER);
    assert.ok(builtinRank(newType("Nonesuch")) > builtinRank(TInteger));
  });
});

function sig(
  args: BoruType[],
  patterns?: Map<number, ReturnType<typeof newInteger>>,
): Signature {
  const s = { args, returns: [] } as unknown as Signature;
  if (patterns) (s as { patterns?: unknown }).patterns = patterns;
  return s;
}

describe("signatureScore", () => {
  it("sums the specificity of the argument types", () => {
    assert.equal(signatureScore(sig([TInteger, TInteger])), 6);
    assert.equal(signatureScore(sig([TAny, TAny])), 2);
    assert.equal(signatureScore(sig([])), 0);
  });

  it("scores a specific signature above a permissive one", () => {
    assert.ok(
      signatureScore(sig([TInteger, TInteger])) >
        signatureScore(sig([TAny, TAny])),
    );
  });

  it("boosts a concrete-value pattern by 10", () => {
    const withPattern = sig([TInteger], new Map([[0, newInteger(1n)]]));
    assert.equal(signatureScore(withPattern), 13);
  });

  it("does not boost a pattern with no concrete payload", () => {
    const bare = sig([TInteger], new Map([[0, newTypeLiteral(TInteger)]]));
    assert.equal(signatureScore(bare), 3);
  });
});

describe("sortSignatures", () => {
  it("puts the most specific signature first", () => {
    const permissive = sig([TAny, TAny]);
    const specific = sig([TInteger, TInteger]);
    const sigs = [permissive, specific];
    sortSignatures(sigs);
    assert.equal(sigs[0], specific);
    assert.equal(sigs[1], permissive);
  });

  it("is stable for equal scores", () => {
    const a = sig([TInteger]);
    const b = sig([TInteger]);
    const sigs = [a, b];
    sortSignatures(sigs);
    assert.equal(sigs[0], a);
  });
});
