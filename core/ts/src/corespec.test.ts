// The TS half of the core-level parity corpus (core/spec/*.tsv).
//
// core/go/corespec_test.go is the other half. The two runners share NO code —
// they read the same files and each implements the tiny expression notation
// independently. That independence is the point: shared scaffolding can hide
// the same bug from both engines, which is exactly how eng/ts's spec-fixture
// papered over unported mechanisms (design/legacy/CORE-GO-TS-DEFECTS.0.ignore, blind
// spot 9).
//
// This is a SPEC, not a differential. The `expected` column is written from
// the documented contract (REFERENCE.md, design/TYPES.10.md), so a row can
// legitimately fail on BOTH engines — the class of defect an agreement
// corpus is structurally blind to.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";
import * as fs from "node:fs";
import * as path from "node:path";
import { fileURLToPath } from "node:url";

import {
  BoruError,
  canon,
  Engine,
  decimalFromString,
  newBigDecimal,
  newBigInteger,
  newBoolean,
  newDisjunct,
  unifiesValue,
  unify,
  newDispatchMod,
  newEnd,
  newFnDef,
  newTypedList,
  newTypedMap,
  newCloseParen,
  newFloat,
  newInteger,
  newList,
  newMap,
  newNone,
  newOpenParen,
  newParenExpr,
  newString,
  newTypeLiteral,
  newWord,
  OrderedMap,
  Registry,
  returnsIdentity,
  TAtom,
  TAny,
  TBoolean,
  TInteger,
  TList,
  TMap,
  TNone,
  TNumber,
  TScalar,
  TString,
  TWord,
  Value,
  type BoruType,
  type WordInfo,
} from "./index.ts";

const SPEC_DIR = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "spec",
);

interface SpecRow {
  file: string;
  line: number;
  expr: string;
  expected: string;
  note: string;
}

function parseSpec(file: string): SpecRow[] {
  const rows: SpecRow[] = [];
  const text = fs.readFileSync(path.join(SPEC_DIR, file), "utf8");
  const lines = text.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!;
    if (line.trim() === "" || line.trim().startsWith("#")) continue;
    const parts = line.split("\t");
    assert.ok(
      parts.length >= 2,
      `${file}:${i + 1}: want >= 2 tab-separated columns`,
    );
    rows.push({
      file,
      line: i + 1,
      expr: parts[0]!,
      expected: parts[1]!,
      note: parts[2] ?? "",
    });
  }
  return rows;
}

/** Builtin type names the corpus may name. Kept short on purpose. */
const TYPE_LITS: Record<string, BoruType> = {
  Integer: TInteger,
  String: TString,
  Boolean: TBoolean,
  List: TList,
  Map: TMap,
  Any: TAny,
  None: TNone,
};

/** The two nodes between Any and Integer, which a `fn(` parameter may
 *  also declare, so a row can state a chain of three. */
const FN_TYPES: Record<string, BoruType> = {
  Number: TNumber,
  Scalar: TScalar,
};

/** One `run` token: decimal → Integer, '…' → String, type name → type
 *  literal, parens → the markers, anything else → Word. */
function token(tok: string): Value {
  if (tok.length >= 2 && tok.startsWith("'") && tok.endsWith("'")) {
    return newString(tok.slice(1, -1));
  }
  if (/^-?\d+$/.test(tok)) return newInteger(BigInt(tok));
  const t = TYPE_LITS[tok];
  if (t !== undefined) return newTypeLiteral(t);
  if (tok === "(") return newOpenParen();
  if (tok === ")") return newCloseParen();
  // The END marker. `end` is its word spelling and `;` its synonym
  // (REFERENCE.md:415); the corpus uses the punctuation so a row can still
  // name a WORD called end.
  if (tok === ";") return newEnd();
  // `name/N`, `name/s`, `name/f`, `name/Ns`, `name/Nf`: a word with
  // call-site modifiers, as the parser would hand it over. A slash whose
  // suffix is not a modifier (`Scalar/String`) leaves the token a plain
  // word. Written independently of the Go runner's copy.
  // The modifiers come in ONE written order — a forced arity N, then at
  // most one of `s` or `f`, then `u` (the bound function with its argument
  // order reversed), then `v` (the bound VALUE, never a call) — each
  // optional and at least one present.
  const mod = /^(.+)\/(\d*)([sf]?)(u?)(v?)$/.exec(tok);
  if (mod !== null && mod.slice(2).some((m) => m !== "")) {
    const info: WordInfo = { name: mod[1]! };
    if (mod[2] !== "") info.argCount = BigInt(mod[2]!);
    if (mod[3] === "s") info.forceStack = true;
    if (mod[3] === "f") info.forceForward = true;
    if (mod[4] === "u") info.forceUsurp = true;
    if (mod[5] === "v") info.forceVal = true;
    return new Value(TWord, info);
  }
  return newWord(tok);
}

function fields(s: string): string[] {
  return s.split(" ").filter((f) => f !== "");
}

/**
 * assemble turns the flat token list into VALUES, folding the bracket forms
 * the README describes into containers: `[ … ]` an eval list, `[q … ]` a
 * quoted one, `{ k: v … }` a map. Written independently of the Go runner's
 * copy — an asymmetry between the two shows up as a false divergence, which
 * is the failure mode this corpus exists to prevent.
 *
 * Recursive-descent over an index the callers share, because the forms nest
 * and a scan-for-the-matching-bracket pass would have to re-count depth at
 * every level.
 */
function assemble(toks: string[]): Value[] {
  const st = { i: 0 };
  const out: Value[] = [];
  while (st.i < toks.length) out.push(item(toks, st));
  return out;
}

/** One item: a container if it opens one, else a single token. */
function item(toks: string[], st: { i: number }): Value {
  const tok = toks[st.i]!;
  if ("[" === tok || "[q" === tok) {
    st.i++;
    const elems: Value[] = [];
    while (st.i < toks.length && "]" !== toks[st.i]) elems.push(item(toks, st));
    assert.equal(toks[st.i], "]", `unclosed ${tok} in ${toks.join(" ")}`);
    st.i++;
    return newList(elems, { eval: "[" === tok });
  }
  if ("fn(" === tok) {
    // fn( NAME PTYPE … RTYPE [ body ] ) — the parameters in signature
    // order, one return, a boru body. Written independently of the Go
    // runner's copy, per the two-runner rule.
    st.i++;
    const spec: string[] = [];
    while (st.i < toks.length && "[" !== toks[st.i] && "[q" !== toks[st.i]) {
      spec.push(toks[st.i++]!);
    }
    if (spec.length < 3 || 0 === spec.length % 2) {
      throw new Error(`fn( wants NAME PTYPE … RTYPE before its body: ${spec.join(" ")}`);
    }
    const fnType = (name: string): BoruType => {
      const t = FN_TYPES[name] ?? TYPE_LITS[name];
      if (undefined === t) throw new Error(`fn( ${name}: not a corpus-known type name`);
      return t;
    };
    const params: { name: string; type: BoruType }[] = [];
    for (let i = 0; i + 1 < spec.length; i += 2) {
      // `_` is the UNNAMED parameter: the empty name, whose argument is
      // the frame's own stack data rather than a binding.
      const pname = "_" === spec[i] ? "" : spec[i]!;
      params.push({ name: pname, type: fnType(spec[i + 1]!) });
    }
    const rtype = fnType(spec[spec.length - 1]!);
    const body = item(toks, st);
    st.i++; // past the ')'
    // NAMED, because that is what the form builds in the Go runner: it
    // installs the function under a scratch name and hands back the
    // installed value. The name is what makes a failed call of the value
    // an error (`uncalled_function`) rather than a function left as data,
    // so a nameless one here would be a different value, not a twin.
    return newFnDef(
      {
        sigs: [
          {
            params,
            returns: [rtype],
            body: body.asList(),
          },
        ],
      },
      "__corespec_fn",
    );
  }
  if ("d(" === tok) {
    // `d( … )` — a DISJUNCT of the listed items. The corpus could name
    // seven builtin type literals and nothing else, so the whole
    // type-CONSTRUCTOR surface — the part of unification that actually
    // differs between the ports — had no expression. Three forms open
    // it: this one and the two typed containers below. Written
    // independently of the Go runner's copy, per the two-runner rule.
    st.i++;
    const alts: Value[] = [];
    while (st.i < toks.length && ")" !== toks[st.i]) alts.push(item(toks, st));
    st.i++;
    return newDisjunct(alts);
  }
  if ("[:" === tok) {
    // `[: T e1 e2 … ]` — a typed list: child constraint, then any
    // retained elements. `[: T ]` alone is the bare carrier.
    st.i++;
    const child = item(toks, st);
    const elems: Value[] = [];
    while (st.i < toks.length && "]" !== toks[st.i]) elems.push(item(toks, st));
    st.i++;
    return newTypedList(child, elems);
  }
  if ("{:" === tok) {
    // `{: T k: v … }` — a typed map: child constraint, then any retained
    // entries.
    st.i++;
    const child = item(toks, st);
    const entries: Array<{ key: string; value: Value }> = [];
    while (st.i < toks.length && "}" !== toks[st.i]) {
      const key = toks[st.i]!;
      st.i++;
      entries.push({ key: key.slice(0, -1), value: item(toks, st) });
    }
    st.i++;
    return newTypedMap(child, entries);
  }
  if ("p(" === tok) {
    st.i++;
    const items: Value[] = [];
    while (st.i < toks.length && ")" !== toks[st.i]) items.push(item(toks, st));
    st.i++;
    return newParenExpr(items);
  }
  if ("{" === tok || "{q" === tok) {
    st.i++;
    const m = new OrderedMap();
    while (st.i < toks.length && "}" !== toks[st.i]) {
      const key = toks[st.i]!;
      assert.ok(
        key.endsWith(":"),
        `map key ${JSON.stringify(key)} must end with ':'`,
      );
      st.i++;
      assert.ok(st.i < toks.length, `map key ${key} has no value`);
      m.set(key.slice(0, -1), item(toks, st));
    }
    assert.equal(toks[st.i], "}", `unclosed { in ${toks.join(" ")}`);
    st.i++;
    return newMap(m, { eval: "{" === tok });
  }
  st.i++;
  return token(tok);
}

/** The fixture: a bare registry plus ONE word, so the registry, signature
 *  matching, dispatch and the step loop are exercised with no word library. */
function fixtureRegistry(): Registry {
  const r = new Registry();
  r.registerNativeFunc({
    name: "addq",
    signatures: [
      {
        args: [TInteger, TInteger],
        argNames: ["a", "b"],
        returns: [TInteger],
        barrierPos: 2,
        handler: (args: Value[]): Value[] => [
          newInteger(BigInt.asIntN(64, args[0]!.asInteger() + args[1]!.asInteger())),
        ],
      },
    ],
  });
  // One word reaches only the one dispatch shape. These four add the shapes
  // the step loop actually distinguishes — a STACK-form word, a MULTI-return
  // word, a gradual (Any) slot, and a handler that RAISES — so a corpus row
  // can exercise collection, residual layout, gradual matching and error
  // propagation rather than just forward addition. core/go/corespec_test.go
  // declares the same five independently; any asymmetry here shows up as a
  // false divergence, which is the failure mode this corpus exists to
  // prevent, so keep them in step.
  r.registerNativeFunc({
    name: "negq",
    signatures: [
      {
        args: [TInteger],
        argNames: ["a"],
        returns: [TInteger],
        barrierPos: 1,
        handler: (args: Value[]): Value[] => [
          newInteger(BigInt.asIntN(64, -args[0]!.asInteger())),
        ],
      },
    ],
  });
  r.registerNativeFunc({
    name: "pairq",
    signatures: [
      {
        args: [TInteger],
        argNames: ["a"],
        returns: [TInteger, TInteger],
        barrierPos: 1,
        handler: (args: Value[]): Value[] => [
          newInteger(args[0]!.asInteger()),
          newInteger(args[0]!.asInteger()),
        ],
      },
    ],
  });
  r.registerNativeFunc({
    name: "sumq",
    signatures: [
      {
        args: [TInteger, TInteger],
        argNames: ["a", "b"],
        returns: [TInteger],
        barrierPos: 0, // STACK form: both operands come off the stack
        handler: (args: Value[]): Value[] => [
          newInteger(BigInt.asIntN(64, args[0]!.asInteger() + args[1]!.asInteger())),
        ],
      },
    ],
  });
  r.registerNativeFunc({
    name: "boomq",
    signatures: [
      {
        args: [TAny],
        argNames: ["a"],
        returns: [TAny],
        barrierPos: 1,
        handler: (): Value[] => {
          throw new BoruError("fixture_boom", "the fixture word always raises");
        },
      },
    ],
  });
  // patq carries a concrete-scalar PATTERN on its only slot: it matches
  // the Integer 7 and nothing else, so a row can pin that a pattern
  // NARROWS the overload rather than merely typing it.
  r.registerNativeFunc({
    name: "patq",
    signatures: [
      {
        args: [TInteger],
        argNames: ["n"],
        returns: [TInteger],
        patterns: new Map([[0, newInteger(7n)]]),
        barrierPos: 1,
        handler: (): Value[] => [newInteger(1n)],
      },
    ],
  });
  // dupq returns its operand TWICE through the identity-returns
  // mechanism (returnsIdentity): the returns are the ARGS themselves
  // rather than fresh values, which is what lets a duplicating word keep
  // its operand's identity. Declared independently of the Go runner's
  // copy, per the two-runner rule.
  r.registerNativeFunc({
    name: "dupq",
    signatures: [
      {
        args: [TAny],
        argNames: ["a"],
        returns: [TAny, TAny],
        returnsFn: returnsIdentity(0, 0),
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [a[0]!, a[0]!],
      },
    ],
  });
  // __pa pops the per-call args frame, and undef removes a def binding.
  // NEITHER is a core word — basic/go registers both. Go's frame tail
  // emitted them until 2026-10-08 (its DefCleanup marker does the work
  // now) and no frame here ever ran them: they are in the vocabulary
  // because the corpus rows call them. FIXTURES rather than a port: the
  // mechanisms are core's own (the args stack, the def table); only the
  // words are basic's.
  r.registerNativeFunc({
    name: "__pa",
    signatures: [
      {
        args: [],
        returns: [],
        barrierPos: 0,
        handler: (
          _a: Value[],
          _c: unknown,
          _s: Value[],
          reg: Registry,
        ): Value[] => {
          reg.popArgs();
          return [];
        },
      },
    ],
  });
  // argsq reads the per-call args frame: the list a function's body sees
  // as its own arguments — every one of them, named or not, in signature
  // order. Outside a function there is none, and the fixture says so
  // rather than answer an empty list (basic's `args` word answers `[]`;
  // the fixture keeps the two cases apart).
  r.registerNativeFunc({
    name: "argsq",
    signatures: [
      {
        args: [],
        returns: [TList],
        barrierPos: 0,
        handler: (
          _a: Value[],
          _c: unknown,
          _s: Value[],
          reg: Registry,
        ): Value[] => {
          const top = reg.topArgs();
          if (undefined === top) {
            throw new BoruError("fixture_no_args", "argsq: not inside a function");
          }
          return [newList([...top])];
        },
      },
    ],
  });
  // undef's name slot is TAtom, as basic/go's own `undef` declares it:
  // /q capture is CONDITIONAL on the slot type, so an Any-typed one would
  // collect the raw Word rather than the Atom name (see the /q block in
  // core/spec/dispatch.tsv). Declared independently of the Go runner's
  // copy, per the two-runner rule.
  r.registerNativeFunc({
    name: "undef",
    signatures: [
      {
        args: [TAtom],
        argNames: ["n"],
        returns: [],
        quoteArgs: new Set([0]),
        barrierPos: 1,
        handler: (
          a: Value[],
          _c: unknown,
          _s: Value[],
          reg: Registry,
        ): Value[] => {
          reg.popDef(a[0]!.asAtom());
          return [];
        },
      },
    ],
  });
  // qanyq is the /q fixture whose name slot is ANY rather than Atom, and it
  // RETURNS what it captured so a row can read the capture back. Paired
  // with undef's Atom slot it pins /q's CONDITIONAL coercion: the Atom
  // slot takes the Word→Atom conversion, the Any slot — which the raw Word
  // already fills — does not. Written independently of the Go runner's
  // copy, per the two-runner rule.
  r.registerNativeFunc({
    name: "qanyq",
    signatures: [
      {
        args: [TAny],
        argNames: ["n"],
        returns: [TAny],
        quoteArgs: new Set([0]),
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [a[0]!],
      },
    ],
  });
  // bothq takes TWO gradual args from the forward side. One-arg words
  // cannot reach the collection loop's middle: a marker that parks and
  // then collects TWICE, and one that meets `end` with its slots still
  // unfilled, both need an arity of two.
  r.registerNativeFunc({
    name: "bothq",
    signatures: [
      {
        args: [TAny, TAny],
        argNames: ["a", "b"],
        returns: [TAny],
        barrierPos: 2,
        handler: (a: Value[]): Value[] => [a[1]!],
      },
    ],
  });
  // tyq's only slot is a TYPE-ARG slot: it admits a type (a bare literal
  // or a structural type body) and refuses every concrete value, which is
  // a different rule from "a slot whose declared type is Type". It returns
  // what it was given so a row can read the match back.
  r.registerNativeFunc({
    name: "tyq",
    signatures: [
      {
        args: [TAny],
        argNames: ["t"],
        returns: [TAny],
        typeArgs: new Set([0]),
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [a[0]!],
      },
    ],
  });
  // tpatq carries a TYPE-LITERAL pattern (rather than patq's concrete
  // scalar one) on a STACK slot — the shape that reaches the pattern
  // matcher's type arm, since a type pattern is enforced only on stack
  // positions.
  r.registerNativeFunc({
    name: "tpatq",
    signatures: [
      {
        args: [TAny],
        argNames: ["v"],
        returns: [TInteger],
        patterns: new Map([[0, newTypeLiteral(TInteger)]]),
        barrierPos: 0,
        handler: (a: Value[]): Value[] => [a[0]!],
      },
    ],
  });
  // unifyq is the door onto UNIFICATION — the one core mechanism this
  // corpus could not reach at all, because unify's clients (`is`, `case`,
  // typed defs, record shapes) are basic-layer words and a bare core
  // registry called it from nowhere.
  //
  // The PREDICATE half: did the two values unify. `meetq` below is the
  // VALUE half — the two were one word short while core/ts had a predicate
  // and no meet. Declared independently of the Go runner's copy, per the
  // two-runner rule.
  r.registerNativeFunc({
    name: "unifyq",
    signatures: [
      {
        args: [TAny, TAny],
        argNames: ["a", "b"],
        returns: [TBoolean],
        barrierPos: 2,
        handler: (
          a: Value[],
          _c: unknown,
          _s: Value[],
          reg: Registry,
        ): Value[] => [newBoolean(unifiesValue(a[0]!, a[1]!, reg))],
      },
    ],
  });
  // meetq is unifyq's VALUE half: the unified value itself — the meet of
  // the two — or the fixture error `fixture_no_meet` when they have none.
  // The predicate cannot tell `5 tor 6` from `5` as the answer to
  // `d( 5 6 )` against `Integer`; this can.
  r.registerNativeFunc({
    name: "meetq",
    signatures: [
      {
        args: [TAny, TAny],
        argNames: ["a", "b"],
        returns: [TAny],
        barrierPos: 2,
        handler: (
          a: Value[],
          _c: unknown,
          _s: Value[],
          reg: Registry,
        ): Value[] => {
          const meet = unify(a[0]!, a[1]!, reg);
          if (meet === undefined) {
            throw new BoruError("fixture_no_meet", "the two values do not unify");
          }
          return [meet];
        },
      },
    ],
  });
  // depthq is the FULL-STACK fixture: the handler receives the whole
  // resolved stack of the current paren scope and returns its complete
  // replacement, rather than N args and their replacement. Declared here
  // independently of the Go runner's copy — any asymmetry shows up as a
  // false divergence, which is the failure mode this corpus prevents.
  r.registerNativeFunc({
    name: "depthq",
    signatures: [
      {
        args: [],
        fullStack: true,
        barrierPos: 0,
        handler: (
          _a: Value[],
          _c: Map<string, Value> | null,
          stack: Value[],
        ): Value[] => [...stack, newInteger(BigInt(stack.length))],
      },
    ],
  });
  return r;
}

function render(vs: Value[]): string {
  return canon(vs);
}

function evalExpr(expr: string): string {
  const sp = expr.indexOf(" ");
  const kind = sp < 0 ? expr : expr.slice(0, sp);
  const arg = sp < 0 ? "" : expr.slice(sp + 1);
  switch (kind) {
    case "int":
      return canon([newInteger(BigInt(arg))]);
    case "bigint":
      return canon([newBigInteger(BigInt(arg))]);
    case "bigdec":
      return canon([newBigDecimal(decimalFromString(arg)!)]);
    case "float":
      return canon([newFloat(Number(arg))]);
    case "str":
      return canon([newString(arg)]);
    case "bool":
      return canon([newBoolean(arg === "true")]);
    case "none":
      return canon([newNone()]);
    case "end":
      return canon([newEnd()]);
    case "closeparen":
      return canon([newCloseParen()]);
    case "dispatchmod":
      return canon([newDispatchMod({ val: "v" === arg, quote: "q" === arg })]);
    case "typedlist": {
      const toks = fields(arg);
      return canon([newTypedList(token(toks[0]!), toks.slice(1).map(token))]);
    }
    case "typedmap": {
      const toks = fields(arg);
      const entries = toks.slice(1).map((t) => {
        const i = t.indexOf(":");
        return { key: t.slice(0, i), value: token(t.slice(i + 1)) };
      });
      return canon([newTypedMap(token(toks[0]!), entries)]);
    }
    case "typelit": {
      const t = TYPE_LITS[arg];
      assert.ok(
        t !== undefined,
        `typelit ${arg}: not a corpus-known type name`,
      );
      return canon([newTypeLiteral(t)]);
    }
    case "list":
      return canon([newList(assemble(fields(arg)))]);
    case "run": {
      try {
        // A leading `def NAME <item> ;` clause installs a def BINDING before
        // the program runs, which is the only way the corpus can reach the
        // engine's def-substitution paths — nothing in the fixture
        // vocabulary binds a name. `;` ends the clause; several may stack.
        let toks = fields(arg);
        const reg = fixtureRegistry();
        while (toks.length > 0 && "def" === toks[0]) {
          const st = { i: 2 };
          const bound = item(toks, st);
          if (";" !== toks[st.i])
            throw new Error(`def clause for ${toks[1]} must end with ';'`);
          reg.pushDef(toks[1]!, bound);
          toks = toks.slice(st.i + 1);
        }
        return render(new Engine(reg).run(assemble(toks)));
      } catch (e) {
        if (e instanceof BoruError) return `ERROR:${e.code}`;
        return `ERROR:non_boru:${(e as Error).message}`;
      }
    }
  }
  throw new Error(`unknown expression kind ${JSON.stringify(kind)}`);
}

// divergent.tsv is the parity DEBT and has a different column shape, so it
// is read by its own block below rather than as a two-column spec file.
const DIVERGENT = "divergent.tsv";

const files = fs
  .readdirSync(SPEC_DIR)
  .filter((f) => f.endsWith(".tsv") && DIVERGENT !== f)
  .sort();

assert.ok(
  files.length > 0,
  "core/spec has no .tsv files — the corpus is not being read",
);

describe("core/spec", () => {
  for (const file of files) {
    describe(file.replace(/\.tsv$/, ""), () => {
      for (const row of parseSpec(file)) {
        it(`L${row.line} ${row.expr}`, () => {
          assert.equal(evalExpr(row.expr), row.expected, row.note);
        });
      }
    });
  }
});

// Pins the TS side of every recorded divergence, and fails a row that has
// STOPPED diverging — a fixed divergence must move to the spec file it
// belongs in, not sit here looking like debt. The Go runner asserts the other
// column from the same file, so both suites stay green while the difference
// stays visible. Same discipline as parser/spec/divergent.tsv.
describe("core/spec — divergent.tsv (parity debt)", () => {
  const rows = parseSpec(DIVERGENT);
  if (0 === rows.length) {
    // The debt is paid. Kept as a live assertion rather than deleted: the
    // file is the ratchet, so a NEW divergence has to be added here
    // deliberately instead of quietly landing as a changed expectation
    // somewhere else.
    it("is empty — core/go and core/ts agree on every corpus expression", () => {
      assert.equal(rows.length, 0);
    });
  }
  for (const row of rows) {
    it(`L${row.line} ${row.expr}`, () => {
      // parseSpec puts column 2 in `expected` and column 3 in `note`; for
      // this file those are the go and ts columns.
      const [wantGo, wantTs] = [row.expected, row.note];
      assert.notEqual(
        wantGo,
        wantTs,
        "go and ts columns are identical — move this row out of divergent.tsv",
      );
      assert.equal(evalExpr(row.expr), wantTs);
    });
  }
});
