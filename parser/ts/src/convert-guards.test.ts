// Direct-call tests for the converter arms no source text reaches — the
// TS twin of the convert-level tests in parser/go/parse_test.go and
// parser/go/grammar_seam5b_test.go (TestConvertTopLevelValueRawMap,
// TestParseWordDirectCoverage, TestS5bEConvertFloat64,
// TestS5bENumberValFallbacksAndOverflow, TestS5bEParseBigNumberEmptyDigits).
//
// Same narrow rule as guards.test.ts: a converter arm belongs here only if
// no source can reach it. Two structural reasons put arms in that class,
// and they are worth naming because they explain why the list is as long
// as it is:
//
//   - The grammar wraps every number token in a NumberVal (setupNumberSub),
//     so the RAW number and boolean arms of the value converters are for
//     nodes the boru grammar never produces. They stay because Go's
//     converters have them and the two are ported function-by-function.
//   - parseWord carries its own numeric classification for words that
//     LOOK numeric, but jsonic lexes anything actually numeric as a
//     number first — so the whole base-prefixed branch there is
//     shadowed by numberValToValue.
//
// The functions are reached by exporting them from convert.ts. That is
// module-internal, not package-public: parser/ts/src/index.ts exports
// `parse` and `SrcPos` and nothing else, so this is the same access Go
// gets from an in-package test.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { BoruError, TMap, TList } from '@boru-lang/core'

import {
  convertDataValueInner,
  convertInterpGroup,
  convertParsedNumber,
  convertTopLevelValueInner,
  floatToValue,
  groupModifier,
  isPlainDecimalInteger,
  metaStrSet,
  numberValToValue,
  parseBigNumber,
  parseWord,
  tryParseBigIntBase0,
  typeName,
} from './convert.ts'
import {
  ArrowTag,
  InterpGroup,
  NumberVal,
  ParseDepth,
  UnclosedAngle,
  UnclosedParen,
} from './nodes.ts'

const d = (): ParseDepth => new ParseDepth('')

/** Both value converters, so every arm below is checked in both contexts. */
const CONVERTERS: [string, (v: unknown, d: ParseDepth) => unknown][] = [
  ['top-level', convertTopLevelValueInner],
  ['data', convertDataValueInner],
]

describe('value-converter arms the grammar never produces', () => {
  for (const [where, convert] of CONVERTERS) {
    it(`${where}: a raw number becomes a numeric value`, () => {
      // setupNumberSub wraps every number token in a NumberVal, so a bare
      // JS number arrives only from a caller that built the node itself.
      const v: any = convert(1.5, d())
      assert.equal(v.toString(), '1.5')
    })

    it(`${where}: a raw boolean becomes a Boolean value`, () => {
      // Value.Lex is off (SafeMake), so `true` in source is a WORD — pinned
      // by the `true` row in parse.tsv. This arm is for a node the grammar
      // does not build.
      assert.equal(String(convert(true, d())), 'true')
      assert.equal(String(convert(false, d())), 'false')
    })

    it(`${where}: a raw map converts, with and without a typed child`, () => {
      const plain: any = convert({ x: 1 }, d())
      assert.ok(plain.vType.equal(TMap), 'a raw object is a Map')
      assert.ok((convert({ child$: 1 }, d()) as any).isTypedMap(), 'child$ marks a typed map')
    })

    it(`${where}: a raw array with a typed child becomes a typed list`, () => {
      const arr: any[] = [1]
      ;(arr as any)['child$'] = 1
      assert.ok((convert(arr, d()) as any).isTypedList())
      assert.ok((convert([1, 2], d()) as any).vType.equal(TList))
    })

    it(`${where}: a bare arrow tag is the lambda sugar marker`, () => {
      // A `=>` that did NOT fold — every folding position wraps it in a
      // ParenGroup, so the bare tag survives only where the fold declined
      // (pinned by the `{a: b => [b]}` and `m.k => [1]` rows), and in
      // DATA context not even there.
      assert.equal(String(convert(new ArrowTag(), d())), 'sugar(lambda)')
    })

    it(`${where}: an unsupported node type is refused`, () => {
      // Go's `default:` with %T. Neither engine can produce the node; the
      // arm exists so a future grammar change fails loudly instead of
      // silently dropping a value.
      assert.throws(() => convert(Symbol('x'), d()), /unsupported value type/)
    })
  }

  it('the DATA converter refuses an unclosed group', () => {
    // Only the data converter has these arms — the top-level one leaves
    // an unclosed group to parse(), which handles the root itself, so
    // asserting both would have been asserting a shape that does not
    // exist. The step-cap guard now fires before conversion for the
    // sources that produced these nodes, which is why no row reaches
    // them.
    assert.throws(
      () => convertDataValueInner(new UnclosedParen([]), d()),
      (e: any) => 'syntax_error' === e.code,
    )
    assert.throws(
      () => convertDataValueInner(new UnclosedAngle('Map'), d()),
      (e: any) => 'syntax_error' === e.code,
    )
  })

  it('a top-level implicit map is auto-evaluated', () => {
    // The `info.implicit && !mv.eval` arm. parse() applies the same rule to
    // the ROOT itself, which is the path a source takes, so this one is
    // only reachable for a nested implicit map node built directly.
    const node: any = { a: 1 }
    const v: any = convertTopLevelValueInner(withInfo(node, { implicit: true, meta: {} }), d())
    assert.ok(v.eval, 'an implicit map must be marked eval')
  })
})

describe('parseWord numeric classification', () => {
  it('refuses a word with no base name', () => {
    // `/r` — the modifier scan strips everything and leaves nothing.
    // Pinned as a row too; kept here because the row asserts the rendered
    // error and this asserts the throw site.
    assert.throws(() => parseWord('/r'), /empty word/)
  })

  it('parses base-prefixed integers that reached it as words', () => {
    // Shadowed in practice: jsonic lexes `0x10` as a NUMBER, so the source
    // path is numberValToValue (pinned by the `0x10` row). This branch is
    // parseWord's own copy of the same classification.
    assert.equal(String(parseWord('0x10')), '16')
    assert.equal(String(parseWord('0b1_0')), '2')
    assert.throws(() => parseWord('0x1__0'), (e: any) => 'syntax_error' === e.code)
    assert.throws(
      () => parseWord('0xFFFFFFFFFFFFFFFFF'),
      (e: any) => 'integer_overflow' === e.code,
    )
    // An invalid digit for the base falls through to the numeric-shape
    // classification below rather than erroring here.
    assert.throws(() => parseWord('0x1g'), (e: any) => e instanceof BoruError)
  })

  it('refuses a float-shaped word that overflows', () => {
    assert.throws(() => parseWord('1e400'), (e: any) => 'float_overflow' === e.code)
  })
})

describe('number conversion fallbacks', () => {
  it('a dotted source that fails re-parsing falls back to the lexed float', () => {
    // Go: TestS5bENumberValFallbacksAndOverflow. `1.2.3` is not a number
    // the exact-digit path can re-read, so the value jsonic already lexed
    // is the answer.
    assert.equal(String(numberValToValue(new NumberVal(7.5, '1.2.3', 1, 1))), '7.5')
  })

  it('a base-prefixed source out of int64 range is integer_overflow', () => {
    assert.throws(
      () => numberValToValue(new NumberVal(0, '0x10000000000000000', 1, 1)),
      (e: any) => 'integer_overflow' === e.code,
    )
  })

  it('a float source that overflows is float_overflow, NaN falls back', () => {
    assert.throws(
      () => numberValToValue(new NumberVal(0, '1.5e400', 1, 1)),
      (e: any) => 'float_overflow' === e.code,
    )
    // A source the exact re-read cannot parse keeps jsonic's own value —
    // the arm that makes the re-read an optimisation rather than a second
    // parser with its own failure modes.
    assert.equal(String(numberValToValue(new NumberVal(2.5, 'not-a-number', 1, 1))), '2.5')
  })

  it('floatToValue narrows an integral float to an Integer', () => {
    assert.equal(String(floatToValue(3)), '3')
    assert.equal(String(floatToValue(3.5)), '3.5')
    // Outside the exactly-representable int64 range it stays a Float.
    assert.equal(String(floatToValue(1e30)).includes('e'), true)
  })

  it('a big literal with no digits is refused', () => {
    // Go: TestS5bEParseBigNumberEmptyDigits.
    assert.throws(() => parseBigNumber('0d'), (e: any) => e instanceof BoruError)
    assert.throws(() => parseBigNumber('0i'), (e: any) => e instanceof BoruError)
  })

  it('tryParseBigIntBase0 handles signs, an empty body, and a bad body', () => {
    assert.equal(tryParseBigIntBase0('+0x10'), 16n)
    assert.equal(tryParseBigIntBase0('-0x10'), -16n)
    assert.equal(tryParseBigIntBase0('0x'), undefined)
    assert.equal(tryParseBigIntBase0('+'), undefined)
    assert.equal(tryParseBigIntBase0('0xzz'), undefined)
  })

  it('isPlainDecimalInteger rejects empty and sign-only sources', () => {
    assert.equal(isPlainDecimalInteger(''), false)
    assert.equal(isPlainDecimalInteger('-'), false)
    assert.equal(isPlainDecimalInteger('+'), false)
    assert.equal(isPlainDecimalInteger('12'), true)
    assert.equal(isPlainDecimalInteger('1.2'), false)
  })
})

describe('remaining converter guards', () => {
  it('an interp group with an unknown part type is refused', () => {
    // Go: TestS5bEConvertInterpGroupUnknownPart.
    assert.throws(() => convertInterpGroup(new InterpGroup([42]), d()), /unexpected interp part type/)
  })

  it('groupModifier declines everything that is not an ARGUMENT-SHAPE modifier', () => {
    assert.equal(groupModifier(42), null, 'not a text node')
    assert.equal(groupModifier(new String('plain')), null, 'no modifier at all')
    assert.equal(groupModifier(new String('a/ff')), null, 'an invalid modifier')
    // `/t` IS valid and IS a modifier, but it produces a type expression
    // rather than an argument shape, so there is no group prefix or
    // suffix to emit — the one arm reached with every flag off.
    assert.equal(groupModifier(new String('a/t')), null, 'the type-bound modifier')
  })

  it('convertParsedNumber decodes a NumberVal and declines anything else', () => {
    // The public seam Go exports as ConvertParsedNumber, for callers that
    // hold a parsed node rather than source text.
    assert.equal(String(convertParsedNumber(new NumberVal(1, '1', 1, 1))), '1')
    assert.equal(convertParsedNumber('1'), undefined)
  })

  it('metaStrSet accepts every shape the grammar might have recorded', () => {
    // Go types these channels as map[string]bool; the TS grammar may have
    // written a Set, an array, a Map, or a plain object, and the arms that
    // are not what the current grammar writes are unreachable from source.
    assert.deepEqual([...metaStrSet({ k: new Set(['a']) }, 'k')], ['a'])
    assert.deepEqual([...metaStrSet({ k: ['a'] }, 'k')], ['a'])
    assert.deepEqual([...metaStrSet({ k: new Map([['a', true], ['b', false]]) }, 'k')], ['a'])
    assert.deepEqual([...metaStrSet({ k: { a: true, b: false } }, 'k')], ['a'])
    assert.deepEqual([...metaStrSet({ k: 7 }, 'k')], [], 'an unusable shape is empty, not a throw')
    assert.deepEqual([...metaStrSet(undefined, 'k')], [])
  })

  it('typeName names each node kind for the unsupported-type message', () => {
    assert.equal(typeName(null), 'null')
    assert.equal(typeName(new InterpGroup([])), 'InterpGroup')
    assert.equal(typeName(Object.create(null)), 'object')
    assert.equal(typeName(7), 'number')
  })
})

/**
 * withInfo attaches the hidden node marker jsonic puts on its map nodes.
 * Non-enumerable, because the converters list a map's keys with
 * Object.keys and filter this one out by NAME — an enumerable copy would
 * show up as an extra entry.
 */
function withInfo(node: Record<string, unknown>, info: unknown): unknown {
  Object.defineProperty(node, '__info__', { value: info, writable: true })
  return node
}
