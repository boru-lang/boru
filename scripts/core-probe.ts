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
  unify,
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
  newSplice,
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
  TNumber,
  TScalar,
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

/** The two nodes between Any and Integer, which a `fn(` parameter may
 *  also declare, so a row can state a chain of three. */
const FN_TYPES: Record<string, BoruType> = {
  Number: TNumber,
  Scalar: TScalar,
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
    // NAMED, as the runner's copy is: the Go form installs the function
    // under a scratch name, and the name is what makes a failed call of
    // the value an error rather than a function left as data.
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
  if ("s(" === tok) {
    // `s( item )` — a SPLICE MARKER (`__SP`) wrapping one item: stepped with
    // nothing waiting for it, it is replaced by its payload — a plain
    // list's elements, any other value itself.
    st.i++;
    const payload = item(toks, st);
    if (")" !== toks[st.i]) throw new Error("s( wants one item and a closing )");
    st.i++;
    return newSplice(payload);
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
        argNames: ["a", "b"],
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
        argNames: ["a"],
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
        argNames: ["a"],
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
        argNames: ["a", "b"],
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
  // def binds a name while the program RUNS — `def name value` — where a
  // row's leading `def NAME item ;` clauses bind before it starts. It is
  // what lets a function's body make a definition of its own, which the
  // frame's close is then to undo. bindq does the same under a name the
  // Go kernel's frame analysis does not list as a binding word, so a body
  // that binds only through it is a frame with nothing recorded to undo.
  // Both are the def table's push; only the names differ.
  for (const binder of ["def", "bindq"]) {
    r.registerNativeFunc({
      name: binder,
      signatures: [
        {
          args: [TAtom, TAny],
          argNames: ["n", "v"],
          returns: [],
          quoteArgs: new Set([0]),
          barrierPos: 2,
          handler: (
            a: Value[],
            _c: unknown,
            _s: Value[],
            reg: Registry,
          ): Value[] => {
            reg.pushDef(a[0]!.asAtom(), a[1]!);
            return [];
          },
        },
      ],
    });
  }
  // qanyq is the /q fixture whose name slot is ANY rather than Atom, and it
  // RETURNS what it captured so a row can read the capture back. Paired
  // with undef's Atom slot it pins /q's CONDITIONAL coercion.
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
  // rawlq's slot takes a pending LIST unevaluated (noEvalArgs) and hands it
  // back; rawmq's takes a pending MAP (noEvalMapArgs, a separate field);
  // raw2q puts the list slot SECOND of two and returns both arguments in
  // signature order, so a row can tell which position the mode belongs to.
  // The kernel's two other raw-capture modes — a paren expression captured
  // as code, a form captured as written — are signature fields core/ts does
  // not have: their fixtures (rawpq, formq, form2q) are not registered
  // here, and their rows are the divergence ledger's.
  r.registerNativeFunc({
    name: "rawlq",
    signatures: [
      {
        args: [TAny],
        argNames: ["x"],
        returns: [TAny],
        noEvalArgs: new Set([0]),
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [a[0]!],
      },
    ],
  });
  r.registerNativeFunc({
    name: "rawmq",
    signatures: [
      {
        args: [TAny],
        argNames: ["x"],
        returns: [TAny],
        noEvalMapArgs: new Set([0]),
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [a[0]!],
      },
    ],
  });
  r.registerNativeFunc({
    name: "raw2q",
    signatures: [
      {
        args: [TAny, TAny],
        argNames: ["a", "b"],
        returns: [TList],
        noEvalArgs: new Set([1]),
        barrierPos: 2,
        handler: (a: Value[]): Value[] => [newList([a[0]!, a[1]!])],
      },
    ],
  });
  // spliceq makes a SPLICE MARKER of its argument, as basic's `word` does.
  // Handed back, the marker is at the pointer with nothing waiting for it,
  // and fires: `spliceq [q 1 2 ]` leaves 1 and 2.
  r.registerNativeFunc({
    name: "spliceq",
    signatures: [
      {
        args: [TAny],
        argNames: ["v"],
        returns: [TAny],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [newSplice(a[0]!)],
      },
    ],
  });
  // (mpatq, rawmpatq, lpatq and rawlpatq — a pattern that reads a
  // container's CONTENTS, a map shape or a typed list, and the evaluation
  // of the pending operand it judges — are not registered here: core/ts's
  // patterns compare a payload by identity, so they are scalar patterns
  // only, and a container pattern would admit nothing. Their rows are the
  // divergence ledger's.)
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
  // VALUE half.
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
  // meetq: the unified value itself, or `fixture_no_meet`.
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
