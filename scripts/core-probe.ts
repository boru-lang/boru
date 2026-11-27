// The TS half of scripts/core-probe.sh — see core/go/corespec_test.go's
// TestCoreProbe for the Go half and the rationale.
//
// It re-implements the corpus's expression notation the same way the runner
// does. That duplication is deliberate and matches scripts/parity-probe.ts:
// importing the runner's copy would make the probe agree with the runner by
// construction, which is the one thing a probe must not do.

import * as fs from "node:fs";

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
  newCloseParen,
  newDispatchMod,
  newEnd,
  newFnDef,
  newFloat,
  newInteger,
  newList,
  newMap,
  newNone,
  newOpenParen,
  newParenExpr,
  newString,
  newTypedList,
  newTypedMap,
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
  TString,
  type BoruType,
  TWord,
  Value,
  type WordInfo,
} from "../core/ts/src/index.ts";

const TYPE_LITS: Record<string, BoruType> = {
  Integer: TInteger,
  String: TString,
  Boolean: TBoolean,
  List: TList,
  Map: TMap,
  Any: TAny,
  None: TNone,
};

function token(tok: string): Value {
  if (tok.length >= 2 && tok.startsWith("'") && tok.endsWith("'")) {
    return newString(tok.slice(1, -1));
  }
  if (/^-?\d+$/.test(tok)) return newInteger(BigInt(tok));
  const t = TYPE_LITS[tok];
  if (t !== undefined) return newTypeLiteral(t);
  if ("(" === tok) return newOpenParen();
  if (")" === tok) return newCloseParen();
  // The END marker. `end` is its word spelling and `;` its synonym
  // (REFERENCE.md:415); the corpus uses the punctuation so a row can still
  // name a WORD called end.
  if (";" === tok) return newEnd();
  // `name/N`, `name/s`, `name/f`, `name/Ns`, `name/Nf` — a modified word
  // (the same reading as core/ts/src/corespec.test.ts's token()).
  const mod = /^(.+)\/(\d*)([sf]?)$/.exec(tok);
  if (mod !== null && (mod[2] !== "" || mod[3] !== "")) {
    const info: WordInfo = { name: mod[1]! };
    if (mod[2] !== "") info.argCount = BigInt(mod[2]!);
    if (mod[3] === "s") info.forceStack = true;
    if (mod[3] === "f") info.forceForward = true;
    return new Value(TWord, info);
  }
  return newWord(tok);
}

function item(toks: string[], st: { i: number }): Value {
  const tok = toks[st.i]!;
  if ("[" === tok || "[q" === tok) {
    st.i++;
    const elems: Value[] = [];
    while (st.i < toks.length && "]" !== toks[st.i]) elems.push(item(toks, st));
    st.i++;
    return newList(elems, { eval: "[" === tok });
  }
  if ("fn(" === tok) {
    // fn( NAME PTYPE RTYPE [ body ] ) — one param, one return, a boru
    // body: the smallest shape that DISPATCHES. Written independently of
    // the Go runner's copy, per the two-runner rule.
    st.i++;
    const pname = toks[st.i]!;
    const ptype = TYPE_LITS[toks[st.i + 1]!];
    const rtype = TYPE_LITS[toks[st.i + 2]!];
    if (undefined === ptype || undefined === rtype) {
      throw new Error(`fn( ${pname}: not corpus-known type names`);
    }
    st.i += 3;
    const body = item(toks, st);
    st.i++; // past the ')'
    return newFnDef({
      sigs: [
        {
          params: [{ name: pname, type: ptype }],
          returns: [rtype],
          body: body.asList(),
        },
      ],
    });
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
      st.i++;
      m.set(key.slice(0, -1), item(toks, st));
    }
    st.i++;
    return newMap(m, { eval: "{" === tok });
  }
  st.i++;
  return token(tok);
}

function assemble(toks: string[]): Value[] {
  const st = { i: 0 };
  const out: Value[] = [];
  while (st.i < toks.length) out.push(item(toks, st));
  return out;
}

function fields(s: string): string[] {
  return s.split(" ").filter((f) => "" !== f);
}

/** The fixture vocabulary, kept in step with both runners' copies. */
function fixtureRegistry(): Registry {
  const r = new Registry();
  const int = (v: Value): bigint => v.asInteger();
  r.registerNativeFunc({
    name: "addq",
    signatures: [
      {
        args: [TInteger, TInteger],
        returns: [TInteger],
        barrierPos: 2,
        handler: (a: Value[]): Value[] => [newInteger(BigInt.asIntN(64, int(a[0]!) + int(a[1]!)))],
      },
    ],
  });
  r.registerNativeFunc({
    name: "negq",
    signatures: [
      {
        args: [TInteger],
        returns: [TInteger],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [newInteger(BigInt.asIntN(64, -int(a[0]!)))],
      },
    ],
  });
  r.registerNativeFunc({
    name: "pairq",
    signatures: [
      {
        args: [TInteger],
        returns: [TInteger, TInteger],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [
          newInteger(int(a[0]!)),
          newInteger(int(a[0]!)),
        ],
      },
    ],
  });
  r.registerNativeFunc({
    name: "sumq",
    signatures: [
      {
        args: [TInteger, TInteger],
        returns: [TInteger],
        barrierPos: 0,
        handler: (a: Value[]): Value[] => [newInteger(BigInt.asIntN(64, int(a[0]!) + int(a[1]!)))],
      },
    ],
  });
  r.registerNativeFunc({
    name: "boomq",
    signatures: [
      {
        args: [TAny],
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
        returns: [TAny, TAny],
        returnsFn: returnsIdentity(0, 0),
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [a[0]!, a[0]!],
      },
    ],
  });
  // __pa pops the per-call args frame, and undef removes a def binding.
  // NEITHER is a core word — basic/go registers both — but every boru fn
  // body's frame tail emits them (AppendFrameTail), so a bare core
  // registry cannot run a boru-bodied fn without them. That is why the
  // engine's fn-dispatch surface was unreachable from this corpus. They
  // are FIXTURES rather than a port: the mechanisms are core's own (the
  // args stack, the def table); only the words are basic's.
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
  // with undef's Atom slot it pins /q's CONDITIONAL coercion.
  r.registerNativeFunc({
    name: "qanyq",
    signatures: [
      {
        args: [TAny],
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
  // The PREDICATE half only: did the two values unify. Go's UnifyR also
  // returns the unified VALUE, and core/ts has no meet to return, so
  // asking for it would pin a port gap as a language contract.
  r.registerNativeFunc({
    name: "unifyq",
    signatures: [
      {
        args: [TAny, TAny],
        returns: [TBoolean],
        barrierPos: 2,
        handler: (a: Value[]): Value[] => [
          newBoolean(unifiesValue(a[0]!, a[1]!)),
        ],
      },
    ],
  });
  r.registerNativeFunc({
    name: "depthq",
    signatures: [
      {
        args: [],
        fullStack: true,
        barrierPos: 0,
        handler: (_a: Value[], _c: unknown, stack: Value[]): Value[] => [
          ...stack,
          newInteger(BigInt(stack.length)),
        ],
      },
    ],
  });
  return r;
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
      return canon([newBoolean("true" === arg)]);
    case "none":
      return canon([newNone()]);
    case "end":
      return canon([newEnd()]);
    case "closeparen":
      return canon([newCloseParen()]);
    case "dispatchmod":
      return canon([newDispatchMod({ ref: "r" === arg, quote: "q" === arg })]);
    case "typedlist": {
      const toks = fields(arg);
      return canon([newTypedList(token(toks[0]!), toks.slice(1).map(token))]);
    }
    case "typedmap": {
      const toks = fields(arg);
      return canon([
        newTypedMap(
          token(toks[0]!),
          toks.slice(1).map((t) => {
            const i = t.indexOf(":");
            return { key: t.slice(0, i), value: token(t.slice(i + 1)) };
          }),
        ),
      ]);
    }
    case "typelit":
      return canon([newTypeLiteral(TYPE_LITS[arg]!)]);
    case "list":
      return canon([newList(assemble(fields(arg)))]);
    case "run":
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
        return canon(new Engine(reg).run(assemble(toks)));
      } catch (e) {
        if (e instanceof BoruError) return `ERROR:${e.code}`;
        return `ERROR:non_boru:${(e as Error).message}`;
      }
  }
  return `ERROR:unknown expression kind ${JSON.stringify(kind)}`;
}

const file = process.argv[2];
if (undefined === file) {
  console.error("usage: core-probe.ts <file-of-expressions>");
  process.exit(1);
}
for (const line of fs
  .readFileSync(file, "utf8")
  .replace(/\n$/, "")
  .split("\n")) {
  if ("" === line) continue;
  let out: string;
  try {
    out = evalExpr(line);
  } catch (e) {
    out = `ERROR:runner:${(e as Error).message}`;
  }
  console.log(out);
}
