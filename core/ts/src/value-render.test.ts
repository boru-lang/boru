// Unit campaign for Value.toString()'s type-node arms — the two the corpus
// never reaches on its own, both of which were WRONG until the parser stream
// oracle was finally run over eng/spec (design/legacy/TS-PARITY-AUDIT.0.ignore).
//
// toString() is not canonValue: it is the debug/stream render, and it is what
// parser/ts/src/streamdump.ts emits for the row-for-row differential against
// parser/go's TestStreamDump. Both engines must agree on it, so both arms
// below are pinned to core/go's kernelFormatDefault (core/go/value.go:3660).

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { TInteger, TNone, TString } from "./type.ts";
import {
  newDisjunct,
  newInteger,
  newNone,
  newTypeLiteral,
  newWord,
} from "./value.ts";

describe("Value.toString — disjunctions", () => {
  it('joins the alternatives with " tor "', () => {
    // core/go/value.go:3660 renders a disjunct as its alternatives joined by
    // " tor ". TS had no arm at all and fell through to String(this.data),
    // which printed the literal '[object Object]' for every union in a
    // parsed stream.
    const d = newDisjunct([newTypeLiteral(TInteger), newTypeLiteral(TString)]);
    assert.equal(d.toString(), "Integer tor String");
  });

  it("renders each alternative with its own toString", () => {
    const d = newDisjunct([newWord("M"), newTypeLiteral(TNone)]);
    assert.equal(d.toString(), "word(M) tor None");
  });

  it("renders a single-alternative disjunction as that alternative", () => {
    assert.equal(newDisjunct([newInteger(1)]).toString(), "1");
  });
});

describe("Value.toString — None is a value and a type", () => {
  it("renders the none VALUE lowercase", () => {
    assert.equal(newNone().toString(), "none");
  });

  it("renders the None TYPE LITERAL by its name", () => {
    // The distinction canonValue already drew and toString did not: a bare
    // type literal renders by name, so the None alternative of a union is
    // `None`, not `none`.
    assert.equal(newTypeLiteral(TNone).toString(), "None");
  });
});
