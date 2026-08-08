// The TS half of scripts/core-probe.sh — see core/go/corespec_test.go's
// TestCoreProbe for the Go half and the rationale.
//
// It re-implements the corpus's expression notation the same way the runner
// does. That duplication is deliberate and matches scripts/parity-probe.ts:
// importing the runner's copy would make the probe agree with the runner by
// construction, which is the one thing a probe must not do.

import * as fs from 'node:fs'

import {
  BoruError,
  canon,
  Engine,
  decimalFromString,
  newBigDecimal,
  newBigInteger,
  newBoolean,
  newCloseParen,
  newDispatchMod,
  newEnd,
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
  TAny,
  TBoolean,
  TInteger,
  TList,
  TMap,
  TNone,
  TString,
  type BoruType,
  type Value,
} from '../core/ts/src/index.ts'

const TYPE_LITS: Record<string, BoruType> = {
  Integer: TInteger,
  String: TString,
  Boolean: TBoolean,
  List: TList,
  Map: TMap,
  Any: TAny,
  None: TNone,
}

function token(tok: string): Value {
  if (tok.length >= 2 && tok.startsWith("'") && tok.endsWith("'")) {
    return newString(tok.slice(1, -1))
  }
  if (/^-?\d+$/.test(tok)) return newInteger(BigInt(tok))
  const t = TYPE_LITS[tok]
  if (t !== undefined) return newTypeLiteral(t)
  if ('(' === tok) return newOpenParen()
  if (')' === tok) return newCloseParen()
  // The END marker. `end` is its word spelling and `;` its synonym
  // (REFERENCE.md:415); the corpus uses the punctuation so a row can still
  // name a WORD called end.
  if (';' === tok) return newEnd()
  return newWord(tok)
}

function item(toks: string[], st: { i: number }): Value {
  const tok = toks[st.i]!
  if ('[' === tok || '[q' === tok) {
    st.i++
    const elems: Value[] = []
    while (st.i < toks.length && ']' !== toks[st.i]) elems.push(item(toks, st))
    st.i++
    return newList(elems, { eval: '[' === tok })
  }
  if ('p(' === tok) {
    st.i++
    const items: Value[] = []
    while (st.i < toks.length && ')' !== toks[st.i]) items.push(item(toks, st))
    st.i++
    return newParenExpr(items)
  }
  if ('{' === tok || '{q' === tok) {
    st.i++
    const m = new OrderedMap()
    while (st.i < toks.length && '}' !== toks[st.i]) {
      const key = toks[st.i]!
      st.i++
      m.set(key.slice(0, -1), item(toks, st))
    }
    st.i++
    return newMap(m, { eval: '{' === tok })
  }
  st.i++
  return token(tok)
}

function assemble(toks: string[]): Value[] {
  const st = { i: 0 }
  const out: Value[] = []
  while (st.i < toks.length) out.push(item(toks, st))
  return out
}

function fields(s: string): string[] {
  return s.split(' ').filter((f) => '' !== f)
}

/** The fixture vocabulary, kept in step with both runners' copies. */
function fixtureRegistry(): Registry {
  const r = new Registry()
  const int = (v: Value): bigint => v.asInteger()
  r.registerNativeFunc({
    name: 'addq',
    signatures: [
      {
        args: [TInteger, TInteger],
        returns: [TInteger],
        barrierPos: 2,
        handler: (a: Value[]): Value[] => [newInteger(int(a[0]!) + int(a[1]!))],
      },
    ],
  })
  r.registerNativeFunc({
    name: 'negq',
    signatures: [
      {
        args: [TInteger],
        returns: [TInteger],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [newInteger(-int(a[0]!))],
      },
    ],
  })
  r.registerNativeFunc({
    name: 'pairq',
    signatures: [
      {
        args: [TInteger],
        returns: [TInteger, TInteger],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [newInteger(int(a[0]!)), newInteger(int(a[0]!))],
      },
    ],
  })
  r.registerNativeFunc({
    name: 'sumq',
    signatures: [
      {
        args: [TInteger, TInteger],
        returns: [TInteger],
        barrierPos: 0,
        handler: (a: Value[]): Value[] => [newInteger(int(a[0]!) + int(a[1]!))],
      },
    ],
  })
  r.registerNativeFunc({
    name: 'boomq',
    signatures: [
      {
        args: [TAny],
        returns: [TAny],
        barrierPos: 1,
        handler: (): Value[] => {
          throw new BoruError('fixture_boom', 'the fixture word always raises')
        },
      },
    ],
  })
  r.registerNativeFunc({
    name: 'depthq',
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
  })
  return r
}

function evalExpr(expr: string): string {
  const sp = expr.indexOf(' ')
  const kind = sp < 0 ? expr : expr.slice(0, sp)
  const arg = sp < 0 ? '' : expr.slice(sp + 1)
  switch (kind) {
    case 'int':
      return canon([newInteger(BigInt(arg))])
    case 'bigint':
      return canon([newBigInteger(BigInt(arg))])
    case 'bigdec':
      return canon([newBigDecimal(decimalFromString(arg)!)])
    case 'str':
      return canon([newString(arg)])
    case 'bool':
      return canon([newBoolean('true' === arg)])
    case 'none':
      return canon([newNone()])
    case 'end':
      return canon([newEnd()])
    case 'closeparen':
      return canon([newCloseParen()])
    case 'dispatchmod':
      return canon([newDispatchMod({ ref: 'r' === arg, quote: 'q' === arg })])
    case 'typedlist': {
      const toks = fields(arg)
      return canon([newTypedList(token(toks[0]!), toks.slice(1).map(token))])
    }
    case 'typedmap': {
      const toks = fields(arg)
      return canon([
        newTypedMap(
          token(toks[0]!),
          toks.slice(1).map((t) => {
            const i = t.indexOf(':')
            return { key: t.slice(0, i), value: token(t.slice(i + 1)) }
          }),
        ),
      ])
    }
    case 'typelit':
      return canon([newTypeLiteral(TYPE_LITS[arg]!)])
    case 'list':
      return canon([newList(assemble(fields(arg)))])
    case 'run':
      try {
        return canon(new Engine(fixtureRegistry()).run(assemble(fields(arg))))
      } catch (e) {
        if (e instanceof BoruError) return `ERROR:${e.code}`
        return `ERROR:non_boru:${(e as Error).message}`
      }
  }
  return `ERROR:unknown expression kind ${JSON.stringify(kind)}`
}

const file = process.argv[2]
if (undefined === file) {
  console.error('usage: core-probe.ts <file-of-expressions>')
  process.exit(1)
}
for (const line of fs.readFileSync(file, 'utf8').replace(/\n$/, '').split('\n')) {
  if ('' === line) continue
  let out: string
  try {
    out = evalExpr(line)
  } catch (e) {
    out = `ERROR:runner:${(e as Error).message}`
  }
  console.log(out)
}
