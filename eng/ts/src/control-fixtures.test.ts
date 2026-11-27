// Interpreter pins over a hand-built fixture set: control words whose
// handlers run their bodies in a sub-engine (`if`, `for`), a fn builder
// and a def binder, a higher-order `applyq`, and a few scalar words.
// Every row is run on the interpreter and its residual (rendered via
// canon) is pinned to an explicit value; an error row pins the BoruError
// code.
//
// These rows used to be run twice — on the interpreter and through the
// TypeScript bytecode VM — with the two residuals compared. The VM is
// gone (design/COMPILER-LANE-REMOVAL.0.md §4); the interpreter's answers,
// which were the reference then, are the assertion now. The fixtures are
// local and value-mode only, and the tokenizer is hand-rolled, so the
// rows reach the engine with token shapes the parser does not produce
// (bare `(`/`)` word tokens, a ParenExpr as a map value).
import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { canon } from '@boru-lang/core'
import {
  BoruError,
  Engine,
  type FnParam,
  type FnSig,
  type Handler,
  type NativeFunc,
  Registry,
  TAny,
  TAtom,
  TFunction,
  TInteger,
  TList,
  TNumber,
  TString,
  Value,
  coerceBoolean,
  newBoolean,
  newFloat,
  newFnDef,
  newInteger,
  newList,
  newMap,
  newParenExpr,
  newString,
  newWord,
  OrderedMap,
  typeNameTable,
} from './index.ts'
import { registerSpecWords, tokenize as specTokenize } from './spec-fixture.ts'

const typeTable = typeNameTable()

function toFloat(v: Value): number {
  return v.vType.matches(TInteger) ? Number(v.asInteger()) : v.asFloat()
}

// numH: numeric binary op — integer when both args are integers, else
// float.
function numH(intOp: (a: bigint, b: bigint) => bigint, fltOp: (a: number, b: number) => number): Handler {
  return (args) => {
    const a = args[0]!
    const b = args[1]!
    if (a.vType.matches(TInteger) && b.vType.matches(TInteger)) {
      return [newInteger(intOp(a.asInteger(), b.asInteger()))]
    }
    return [newFloat(fltOp(toFloat(a), toFloat(b)))]
  }
}

function registerFixtures(r: Registry): void {
  const reg = (fn: NativeFunc): void => r.registerNativeFunc(fn)
  const numberPair = [TNumber, TNumber]
  reg({
    name: 'addq',
    forwardPrecedence: true,
    signatures: [{ args: numberPair, handler: numH((a, b) => a + b, (a, b) => a + b) }],
  })
  reg({
    name: 'subq',
    forwardPrecedence: true,
    signatures: [{ args: numberPair, handler: numH((a, b) => b - a, (a, b) => b - a) }],
  })
  reg({
    name: 'mulq',
    forwardPrecedence: true,
    signatures: [{ args: numberPair, handler: numH((a, b) => a * b, (a, b) => a * b) }],
  })
  reg({
    name: 'negq',
    signatures: [
      {
        args: [TNumber],
        barrierPos: 1,
        handler: (args) => {
          const v = args[0]!
          return v.vType.matches(TInteger) ? [newInteger(-v.asInteger())] : [newFloat(-v.asFloat())]
        },
      },
    ],
  })
  reg({
    name: 'concatq',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TString, TString],
        handler: (args) => [newString(args[0]!.asString() + args[1]!.asString())],
      },
    ],
  })
  reg({
    name: 'lengthq',
    forwardPrecedence: true,
    signatures: [{ args: [TList], handler: (args) => [newInteger(BigInt(args[0]!.asList().length))] }],
  })
  // dupq returns TWO results.
  reg({
    name: 'dupq',
    forwardPrecedence: true,
    signatures: [{ args: [TAny], handler: (args) => [args[0]!, args[0]!] }],
  })
  // gtq: a Boolean comparison (a > b).
  reg({
    name: 'gtq',
    forwardPrecedence: true,
    signatures: [
      { args: [TNumber, TNumber], handler: (args) => [newBoolean(toFloat(args[0]!) > toFloat(args[1]!))] },
    ],
  })
  // if: the handler runs the chosen arm in a sub-engine (like `do`). The
  // 3-arg form takes both arms; the 2-arg form has no else and leaves
  // nothing when the condition is false.
  reg({
    name: 'if',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TAny, TList, TList],
        noEvalArgs: new Set([1, 2]),
        handler: (args, _ctx, _stk, r) =>
          coerceBoolean(args[0]!)
            ? new Engine(r).run([...args[1]!.asList()])
            : new Engine(r).run([...args[2]!.asList()]),
      },
      {
        args: [TAny, TList],
        noEvalArgs: new Set([1]),
        handler: (args, _ctx, _stk, r) =>
          coerceBoolean(args[0]!) ? new Engine(r).run([...args[1]!.asList()]) : [],
      },
    ],
  })
  // fn: builds a FnDefInfo from [params][returns][body] triples (params
  // are `name:Type` words). `def` then binds it and a call runs the body.
  reg({
    name: 'fn',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TList],
        noEvalArgs: new Set([0]),
        handler: (args) => {
          const elems = args[0]!.asList()
          const sigs: FnSig[] = []
          for (let i = 0; i + 2 < elems.length; i += 3) {
            const params: FnParam[] = elems[i]!.asList().map((p) => {
              const nm = p.isWord() ? p.asWord().name : ''
              const colon = nm.indexOf(':')
              const name = colon > 0 ? nm.slice(0, colon) : nm
              const t = (colon > 0 ? typeTable.get(nm.slice(colon + 1)) : undefined) ?? TAny
              return { name, type: t }
            })
            const returns = elems[i + 1]!.asList().map((rt) => rt.vType)
            sigs.push({ params, returns, body: elems[i + 2]!.asList() })
          }
          return [newFnDef({ sigs })]
        },
      },
    ],
  })
  // for: counted loop, count form `for N [body]` (iterator `i` bound
  // 0..N-1). The handler iterates and ACCUMULATES each iteration's body
  // residual, so the loop is variadic.
  reg({
    name: 'for',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TInteger, TList],
        noEvalArgs: new Set([1]),
        handler: (args, _ctx, _stk, r) => {
          const count = args[0]!.asInteger()
          const body = args[1]!.asList()
          const out: Value[] = []
          for (let i = 0n; i < count; i++) {
            r.pushDef('i', newInteger(i))
            out.push(...new Engine(r).run([...body]))
            r.popDef('i')
          }
          return out
        },
      },
    ],
  })
  // def: name binding. A bound computed value (`def a (addq 1 2)`) is
  // one value; referencing it twice reads the same binding.
  reg({
    name: 'def',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TAtom, TAny],
        quoteArgs: new Set([0]),
        handler: (args, _ctx, _stk, r) => {
          const a = args[0]!
          const name = a.isWord() ? a.asWord().name : a.asAtom()
          r.pushDef(name, args[1]!)
          return []
        },
      },
    ],
  })
  // trapq: a word that always raises.
  reg({
    name: 'trapq',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TAny],
        handler: () => {
          throw new BoruError('trap_error', 'trapq: deliberate runtime trap', 'trapq')
        },
      },
    ],
  })
  // tripleq: n*3.
  reg({
    name: 'tripleq',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TNumber],
        handler: (args) => {
          const v = args[0]!
          return v.vType.matches(TInteger) ? [newInteger(v.asInteger() * 3n)] : [newFloat(v.asFloat() * 3)]
        },
      },
    ],
  })
  // dynq: identity over Any.
  reg({
    name: 'dynq',
    forwardPrecedence: true,
    signatures: [{ args: [TAny], handler: (args) => [args[0]!] }],
  })
  // polyq: two overloads selected by operand type — Integer -> 'int',
  // String -> 'str'.
  reg({
    name: 'polyq',
    forwardPrecedence: true,
    signatures: [
      { args: [TInteger], handler: () => [newString('int')] },
      { args: [TString], handler: () => [newString('str')] },
    ],
  })
  // applyq: a higher-order word that APPLIES a unary fn VALUE to an arg.
  // The fn flows in as a runtime operand (a closure); a body that reads an
  // enclosing def resolves it when the body runs.
  reg({
    name: 'applyq',
    forwardPrecedence: true,
    signatures: [
      {
        args: [TFunction, TAny],
        handler: (args, _ctx, _stk, r) => {
          const sig = args[0]!.asFnDef().sigs[0]!
          const name = sig.params[0]!.name
          r.pushDef(name, args[1]!)
          try {
            return new Engine(r).run([...sig.body])
          } finally {
            r.popDef(name)
          }
        },
      },
    ],
  })
}

// Minimal tokenizer: words, ints, floats, single/double strings, lists,
// and `( )` paren groups (emitted as bare word tokens the engine's paren
// handling consumes).
function tokenize(src: string): Value[] {
  let i = 0
  const readList = (): Value[] => {
    const out: Value[] = []
    for (;;) {
      while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
      if (i >= src.length) throw new Error('unterminated list')
      if (src[i] === ']') {
        i++
        return out
      }
      out.push(readOne())
    }
  }
  // A `( … )` group as a map value becomes a ParenExpr (collapses to a
  // single residual when the map deep-evaluates). Flat parens only.
  const readParenExpr = (): Value => {
    i++ // consume '('
    const toks: Value[] = []
    for (;;) {
      while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
      if (src[i] === ')') {
        i++
        break
      }
      toks.push(readOne())
    }
    return newParenExpr(toks)
  }
  const readMap = (): Value => {
    const om = new OrderedMap()
    for (;;) {
      while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
      if (src[i] === '}') {
        i++
        break
      }
      let k = i
      while (k < src.length && src[k] !== ':') k++
      const key = src.slice(i, k)
      i = k + 1 // consume ':'
      while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
      om.set(key, src[i] === '(' ? readParenExpr() : readOne())
    }
    // An EVAL map, which is what the parser builds for a `{…}` literal.
    // core/ts's autoEvalStack now gates on that flag exactly as Go's does —
    // only a container the PARSER built auto-evaluates — so a flagless map
    // from this hand-rolled tokenizer stopped resolving its computed
    // values, which is the whole job the flag has.
    return newMap(om, { eval: true })
  }
  const readOne = (): Value => {
    while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
    const c = src[i]!
    if (c === "'" || c === '"') {
      let j = i + 1
      while (j < src.length && src[j] !== c) j++
      const s = newString(src.slice(i + 1, j))
      i = j + 1
      return s
    }
    if (c === '[') {
      i++
      return newList(readList(), { eval: true })
    }
    if (c === '{') {
      i++
      return readMap()
    }
    if (c === '(' || c === ')') {
      i++
      return newWord(c)
    }
    let j = i
    while (j < src.length && !' \t[](){}'.includes(src[j]!)) j++
    const tok = src.slice(i, j)
    i = j
    if (tok === 'true' || tok === 'false') return newBoolean(tok === 'true')
    if (/^-?\d+$/.test(tok)) return newInteger(BigInt(tok))
    if (/^-?\d+\.\d+$/.test(tok)) return newFloat(Number.parseFloat(tok))
    const t = typeTable.get(tok)
    if (t !== undefined) return new Value(t, null)
    return newWord(tok)
  }
  const out: Value[] = []
  while (i < src.length) {
    while (i < src.length && (src[i] === ' ' || src[i] === '\t')) i++
    if (i >= src.length) break
    out.push(readOne())
  }
  return out
}

function freshRegistry(): Registry {
  const r = new Registry()
  registerFixtures(r)
  return r
}

// Run `fn`, capturing either its rendered residual or the code of a
// BoruError it throws.
type Outcome = { residual: string } | { code: string }
function outcome(fn: () => Value[]): Outcome {
  try {
    return { residual: canon(fn()) }
  } catch (e) {
    if (e instanceof BoruError) return { code: e.code }
    throw e
  }
}

// A row: input source and the interpreter's residual, rendered via canon.
const ROWS: Array<[input: string, want: string]> = [
  ['addq 1 2', '3'],
  ['subq 5 2', '-3'], // forward form reads backwards: args[1] - args[0]
  ['mulq 2 3', '6'],
  ['addq 1 2.0', '3.0'],
  ['negq 5', '-5'],
  ["concatq 'a' 'b'", "'ab'"],
  ['lengthq [1 2 3]', '3'],
  ['5', '5'],
  ["'hi'", "'hi'"],
  ['5 6', '5 6'],
  ['addq 1 2 mulq 3', '9'],
  ['addq 1 (mulq 2 3)', '7'],
  ["concatq 'x' (concatq 'y' 'z')", "'xyz'"],
  ['addq 1 (addq 2 (addq 3 4))', '10'], // deep right-nesting
  ['mulq (addq 1 2) (subq 10 5)', '-15'], // two computed operands in one call
  ['subq 10 3', '-7'], // non-commutative arg order
  ['def x 5 addq x 1', '6'], // def of a literal
  ['def a (addq 1 2) mulq a a', '9'], // multiref of a computed value
  ['def a (addq 1 2) def b (mulq a 10) subq b a', '-27'], // chained shared bindings
  ['if true [1] [2]', '1'], // literal cond, true arm
  ['if false [1] [2]', '2'], // literal cond, false arm
  ['if (gtq 5 3) [addq 1 2] [mulq 2 2]', '3'], // computed cond
  ['if (gtq 1 9) [addq 1 2] [mulq 2 2]', '4'], // computed cond, false arm
  ["if true [1] ['x']", '1'], // divergent arm types
  ['if (gtq 5 3) [if true [10] [20]] [30]', '10'], // nested if in an arm
  ['addq 100 (if (gtq 5 3) [1] [2])', '101'], // branch result feeds a call
  ['for 3 [i]', '0 1 2'], // counted loop, iterator accumulates
  ['for 3 [addq i 10]', '10 11 12'], // body computes per-iteration result
  ['for (lengthq [1 2 3]) [i]', '0 1 2'], // computed count
  ['for 0 [i]', ''], // zero iterations -> empty residual
  ['def n 3 for n [i]', '0 1 2'], // def-bound count
  ['def inc fn [[n:Integer] [Integer] [addq n 1]] inc 5', '6'], // user fn
  ['def double fn [[n:Number] [Number] [mulq n 2]] double (addq 1 2)', '6'], // fn with computed arg
  ['def add2 fn [[x:Integer y:Integer] [Integer] [addq x y]] add2 3 4', '7'], // two-param fn
  ['def id fn [[n:Integer] [Integer] [n]] id 7', '7'], // fn returning its param
  ['def inc fn [[n:Integer] [Integer] [addq n 1]] addq (inc 1) (inc 2)', '5'], // called twice
  ['[addq 1 2]', '[3]'], // computed list residual
  ['[addq 1 2 mulq 3 4]', '[3 12]'], // multi-element computed list
  ['[1 2 3]', '[1 2 3]'], // constant list
  ['{a:(addq 1 2)}', '{a:3}'], // computed map
  ['{a:(addq 1 2) b:(mulq 2 3)}', '{a:3 b:6}'], // multi-entry computed map
  ['[[addq 1 2] 9]', '[[3] 9]'], // nested computed list
  ['tripleq 5', '15'],
  ['tripleq 2.5', '7.5'],
  ['addq 1 (tripleq 5)', '16'],
  ['tripleq (addq 1 2)', '9'],
  ['addq (tripleq 2) (tripleq 3)', '15'],
  ['def t (tripleq 4) addq t t', '24'],
  ['polyq (dynq 5)', "'int'"], // the Integer overload
  ["polyq (dynq 'x')", "'str'"], // the String overload
  ['addq (dynq 1) 2', '3'],
  ['dynq 7', '7'],
  ['applyq (fn [[n:Integer] [Integer] [addq n 1]]) 5', '6'], // fn value applied
  ['applyq (fn [[n:Integer] [Integer] [mulq n n]]) 4', '16'], // multi-word fn body
  ['dupq 5', '5 5'], // multi-result call
  ['lengthq [addq 1 2]', '1'], // computed-list arg auto-evaluated then consumed
  ['if false [42]', ''], // 2-arg if (no else), condition false
  ['for 2 [dupq 1]', '1 1 1 1'], // loop body leaves two values per iteration
  ['for 2 [i] addq 1 2', '0 1 3'], // loop followed by a call
  ['def k 100 applyq (fn [[n:Integer] [Integer] [addq n k]]) 5', '105'], // fn body reads an enclosing def
]

// Error rows: the BoruError code the interpreter raises.
const ERROR_ROWS: Array<[input: string, code: string]> = [
  ['trapq 5', 'trap_error'],
  ['addq 1 2 trapq', 'trap_error'], // the call before it runs first
  ['if true [trapq 5] [9]', 'trap_error'], // raised inside the taken arm
  // A fn body that leaves more than its declared return fails the call
  // frame's RETURN CHECK, as it does in core/go. This row sat among the
  // value rows as `5 5` while core/ts ran no check at all — a per-engine
  // pin of the gap, which core/spec/dispatch.tsv ("the call FRAME") now
  // holds both engines to.
  ['def f fn [[n:Integer] [Integer] [dupq n]] f 5', 'type_error'],
]

describe('interpreter pins (hand-built control fixtures)', () => {
  for (const [input, want] of ROWS) {
    it(`${input} -> ${want === '' ? '(empty)' : want}`, () => {
      assert.deepEqual(outcome(() => new Engine(freshRegistry()).run(tokenize(input))), { residual: want })
    })
  }

  for (const [input, code] of ERROR_ROWS) {
    it(`${input} raises ${code}`, () => {
      assert.deepEqual(outcome(() => new Engine(freshRegistry()).run(tokenize(input))), { code })
    })
  }

  it('a def under an untaken trap arm is bound exactly once', () => {
    const r = freshRegistry()
    assert.equal(canon(new Engine(r).run(tokenize('def x 1 if false [trapq 0] [x]'))), '1')
    assert.equal(r.defStackDepth('x'), 1)
  })

  // The spec registry's `def` validates a `:Constraint`; a violation raises
  // at run time.
  it('a def type-constraint violation raises type_error (spec fixtures)', () => {
    const r = new Registry()
    registerSpecWords(r)
    assert.deepEqual(
      outcome(() => new Engine(r).run(specTokenize("def x:Integer 'wrong'"))),
      { code: 'type_error' },
    )
  })
})
