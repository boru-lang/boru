// The TS half of the core-level parity corpus (core/spec/*.tsv).
//
// core/go/corespec_test.go is the other half. The two runners share NO code —
// they read the same files and each implements the tiny expression notation
// independently. That independence is the point: shared scaffolding can hide
// the same bug from both engines, which is exactly how eng/ts's spec-fixture
// papered over unported mechanisms (design/CORE-GO-TS-DEFECTS.0.md, blind
// spot 9).
//
// This is a SPEC, not a differential. The `expected` column is written from
// the documented contract (REFERENCE.md, design/TYPES.10.md), so a row can
// legitimately fail on BOTH engines — the class of defect an agreement
// corpus is structurally blind to.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  BoruError,
  canon,
  Engine,
  decimalFromString,
  newBigDecimal,
  newBigInteger,
  newBoolean,
  newDispatchMod,
  newEnd,
  newTypedList,
  newTypedMap,
  newCloseParen,
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
  TAny,
  TBoolean,
  TInteger,
  TList,
  TMap,
  TNone,
  TString,
  type BoruType,
  type Value,
} from './index.ts'

const SPEC_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..', 'spec')

interface SpecRow {
  file: string
  line: number
  expr: string
  expected: string
  note: string
}

function parseSpec(file: string): SpecRow[] {
  const rows: SpecRow[] = []
  const text = fs.readFileSync(path.join(SPEC_DIR, file), 'utf8')
  const lines = text.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!
    if (line.trim() === '' || line.trim().startsWith('#')) continue
    const parts = line.split('\t')
    assert.ok(parts.length >= 2, `${file}:${i + 1}: want >= 2 tab-separated columns`)
    rows.push({
      file,
      line: i + 1,
      expr: parts[0]!,
      expected: parts[1]!,
      note: parts[2] ?? '',
    })
  }
  return rows
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
}

/** One `run` token: decimal → Integer, '…' → String, type name → type
 *  literal, parens → the markers, anything else → Word. */
function token(tok: string): Value {
  if (tok.length >= 2 && tok.startsWith("'") && tok.endsWith("'")) {
    return newString(tok.slice(1, -1))
  }
  if (/^-?\d+$/.test(tok)) return newInteger(BigInt(tok))
  const t = TYPE_LITS[tok]
  if (t !== undefined) return newTypeLiteral(t)
  if (tok === '(') return newOpenParen()
  if (tok === ')') return newCloseParen()
  // The END marker. `end` is its word spelling and `;` its synonym
  // (REFERENCE.md:415); the corpus uses the punctuation so a row can still
  // name a WORD called end.
  if (tok === ';') return newEnd()
  return newWord(tok)
}

function fields(s: string): string[] {
  return s.split(' ').filter((f) => f !== '')
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
  const st = { i: 0 }
  const out: Value[] = []
  while (st.i < toks.length) out.push(item(toks, st))
  return out
}

/** One item: a container if it opens one, else a single token. */
function item(toks: string[], st: { i: number }): Value {
  const tok = toks[st.i]!
  if ('[' === tok || '[q' === tok) {
    st.i++
    const elems: Value[] = []
    while (st.i < toks.length && ']' !== toks[st.i]) elems.push(item(toks, st))
    assert.equal(toks[st.i], ']', `unclosed ${tok} in ${toks.join(' ')}`)
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
      assert.ok(key.endsWith(':'), `map key ${JSON.stringify(key)} must end with ':'`)
      st.i++
      assert.ok(st.i < toks.length, `map key ${key} has no value`)
      m.set(key.slice(0, -1), item(toks, st))
    }
    assert.equal(toks[st.i], '}', `unclosed { in ${toks.join(' ')}`)
    st.i++
    return newMap(m, { eval: '{' === tok })
  }
  st.i++
  return token(tok)
}

/** The fixture: a bare registry plus ONE word, so the registry, signature
 *  matching, dispatch and the step loop are exercised with no word library. */
function fixtureRegistry(): Registry {
  const r = new Registry()
  r.registerNativeFunc({
    name: 'addq',
    signatures: [
      {
        args: [TInteger, TInteger],
        returns: [TInteger],
        barrierPos: 2,
        handler: (args: Value[]): Value[] => [
          newInteger(args[0]!.asInteger() + args[1]!.asInteger()),
        ],
      },
    ],
  })
  // One word reaches only the one dispatch shape. These four add the shapes
  // the step loop actually distinguishes — a STACK-form word, a MULTI-return
  // word, a gradual (Any) slot, and a handler that RAISES — so a corpus row
  // can exercise collection, residual layout, gradual matching and error
  // propagation rather than just forward addition. core/go/corespec_test.go
  // declares the same five independently; any asymmetry here shows up as a
  // false divergence, which is the failure mode this corpus exists to
  // prevent, so keep them in step.
  r.registerNativeFunc({
    name: 'negq',
    signatures: [
      {
        args: [TInteger],
        returns: [TInteger],
        barrierPos: 1,
        handler: (args: Value[]): Value[] => [newInteger(-args[0]!.asInteger())],
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
        handler: (args: Value[]): Value[] => [
          newInteger(args[0]!.asInteger()),
          newInteger(args[0]!.asInteger()),
        ],
      },
    ],
  })
  r.registerNativeFunc({
    name: 'sumq',
    signatures: [
      {
        args: [TInteger, TInteger],
        returns: [TInteger],
        barrierPos: 0, // STACK form: both operands come off the stack
        handler: (args: Value[]): Value[] => [
          newInteger(args[0]!.asInteger() + args[1]!.asInteger()),
        ],
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
  // depthq is the FULL-STACK fixture: the handler receives the whole
  // resolved stack of the current paren scope and returns its complete
  // replacement, rather than N args and their replacement. Declared here
  // independently of the Go runner's copy — any asymmetry shows up as a
  // false divergence, which is the failure mode this corpus prevents.
  r.registerNativeFunc({
    name: 'depthq',
    signatures: [
      {
        args: [],
        fullStack: true,
        barrierPos: 0,
        handler: (_a: Value[], _c: Map<string, Value> | null, stack: Value[]): Value[] => [
          ...stack,
          newInteger(BigInt(stack.length)),
        ],
      },
    ],
  })
  return r
}

function render(vs: Value[]): string {
  return canon(vs)
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
      return canon([newBoolean(arg === 'true')])
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
      const entries = toks.slice(1).map((t) => {
        const i = t.indexOf(':')
        return { key: t.slice(0, i), value: token(t.slice(i + 1)) }
      })
      return canon([newTypedMap(token(toks[0]!), entries)])
    }
    case 'typelit': {
      const t = TYPE_LITS[arg]
      assert.ok(t !== undefined, `typelit ${arg}: not a corpus-known type name`)
      return canon([newTypeLiteral(t)])
    }
    case 'list':
      return canon([newList(assemble(fields(arg)))])
    case 'run': {
      try {
        return render(new Engine(fixtureRegistry()).run(assemble(fields(arg))))
      } catch (e) {
        if (e instanceof BoruError) return `ERROR:${e.code}`
        return `ERROR:non_boru:${(e as Error).message}`
      }
    }
  }
  throw new Error(`unknown expression kind ${JSON.stringify(kind)}`)
}

const files = fs
  .readdirSync(SPEC_DIR)
  .filter((f) => f.endsWith('.tsv'))
  .sort()

assert.ok(files.length > 0, 'core/spec has no .tsv files — the corpus is not being read')

describe('core/spec', () => {
  for (const file of files) {
    describe(file.replace(/\.tsv$/, ''), () => {
      for (const row of parseSpec(file)) {
        it(`L${row.line} ${row.expr}`, () => {
          assert.equal(evalExpr(row.expr), row.expected, row.note)
        })
      }
    })
  }
})
