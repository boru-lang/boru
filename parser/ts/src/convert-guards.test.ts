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
// module-internal, not package-public: parser/ts/src/index.ts re-exports
// only the deliberate host-facing surface (`parse`, `convertParsedNumber`,
// the config/safe-construction/lexer seams from public.ts, and the
// `SrcPos`/`LexToken` types); the converter internals exercised here are
// NOT part of it, so this is the same access Go gets from an in-package
// test.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { BoruError, TMap, TList } from '@boru-lang/core'

import {
  ConvertedNode,
  convertDataValueInner,
  convertInterpGroup,
  convertParsedNumber,
  convertTopLevelItems,
  convertTopLevelValueInner,
  errMessage,
  floatToValue,
  groupModifier,
  isPlainDecimalInteger,
  metaStrSet,
  numberValToValue,
  orderedKeys,
  parse,
  parseBigNumber,
  parseWord,
  prepareNestedConversions,
  tryParseBigIntBase0,
  typeName,
} from './convert.ts'
import {
  ArrowTag,
  AngleGroup,
  InterpGroup,
  IexprGroup,
  NumberVal,
  ParenGroup,
  ParseDepth,
  Sited,
  UnclosedAngle,
  UnclosedParen,
  codePointColumnAt,
  normalizeReportedPos,
  normalizeSrcPos,
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

  it('the DATA converter preserves its direct ParenGroup seam', () => {
    // parse() trampolines nested parens before DATA conversion, but this
    // converter is also deliberately callable with Go's raw intermediate.
    assert.equal(
      String(convertDataValueInner(new ParenGroup([new String('x')]), d())),
      'paren([word(x)])',
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

  it('de-duplicates a defensive source-order channel', () => {
    assert.deepEqual(orderedKeys(new Set(['a', 'b', 'c']), ['b', 'b', 'a']), [
      'b',
      'a',
      'c',
    ])
  })
})

describe('Unicode source-position normalization', () => {
  it('converts exact UTF-16 indices and preserves unknown positions', () => {
    assert.equal(codePointColumnAt('🙂 x', -1), 1)
    assert.equal(codePointColumnAt('🙂 x', 99), 4)
    assert.equal(codePointColumnAt('🙂 x', 3), 3)
    assert.equal(codePointColumnAt('a\n🙂 x', 5), 3)
    assert.deepEqual(
      normalizeSrcPos('🙂 x', { row: 1, col: 4, src: 'x', si: 3 }),
      { row: 1, col: 3, src: 'x', si: 3 },
    )
    const unknown = { row: 0, col: 0, si: 0 }
    assert.equal(normalizeSrcPos('x', unknown), unknown)
    const withoutIndex = { row: 1, col: 2 }
    assert.equal(normalizeSrcPos('x', withoutIndex), withoutIndex)

    const depth = new ParseDepth('a\n🙂 x')
    assert.equal(depth.normalizePos({ row: 2, col: 4, si: 5 }).col, 3)
    assert.equal(depth.normalizePos(unknown), unknown)
    assert.equal(depth.normalizePos(withoutIndex), withoutIndex)

    // Grammar sites always carry token text; a direct Go-shaped site may not.
    // Pin withPos's honest empty-source fallback on that public converter seam.
    const [positioned]: any[] = convertTopLevelItems([
      new Sited(new String('x'), { row: 1, col: 1 }),
    ], d())
    assert.deepEqual(positioned.pos, { row: 1, col: 1, src: '' })
  })

  it('anchors reported columns to tokens across stock and custom matchers', () => {
    assert.equal(normalizeReportedPos('🙂 ]', { row: 1, col: 4, src: ']' }).col, 3)
    assert.equal(normalizeReportedPos('+re/🙂/ ]', { row: 1, col: 8, src: ']' }).col, 8)
    assert.equal(normalizeReportedPos('x 🙂 x', { row: 1, col: 1, src: 'x' }).col, 1)
    assert.equal(normalizeReportedPos('abc', { row: 1, col: 2, src: 'z' }).col, 2)
  })

  it('handles unknown rows and tokenless end positions honestly', () => {
    const unknown = { row: 0, col: 0, src: '' }
    assert.equal(normalizeReportedPos('x', unknown), unknown)
    const missingRow = { row: 3, col: 1, src: 'x' }
    assert.equal(normalizeReportedPos('x', missingRow), missingRow)
    const unknownColumn = { row: 1, col: 0, src: '' }
    assert.equal(normalizeReportedPos('x', unknownColumn), unknownColumn)
    assert.equal(normalizeReportedPos('x', { row: 1, col: 1 }).col, 1)
    assert.equal(normalizeReportedPos('🙂', { row: 1, col: 2, src: '' }).col, 2)
    assert.equal(normalizeReportedPos('🙂', { row: 1, col: 3, src: '' }).col, 2)
  })
})

describe('parseWord numeric classification', () => {
  it('refuses a word with no base name', () => {
    // `/v` — the modifier scan strips everything and leaves nothing.
    // Pinned as a row too; kept here because the row asserts the rendered
    // error and this asserts the throw site.
    assert.throws(() => parseWord('/v'), /`\/v` modifies nothing/)
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
  it('the stack-safe preparation walk replaces every unsited container slot', () => {
    // Real grammar children are normally Sited, in which case replacement
    // happens inside the wrapper. These raw carriers are the Go-compatible
    // direct seam and exercise each outer slot writer explicitly — and the
    // walk must actually INSTALL the converted cell in each slot, not just
    // survive the traversal.
    const typed: any[] = []
    ;(typed as any)['child$'] = new AngleGroup('Box', [new String('Integer')])
    prepareNestedConversions(typed, d())
    assert.ok((typed as any)['child$'] instanceof ConvertedNode)

    const map = { x: new ParenGroup([new String('x')]) }
    prepareNestedConversions(map, d())
    assert.ok(map.x instanceof ConvertedNode)
    assert.notEqual((map.x as unknown as ConvertedNode).parenItems, undefined)

    const iexpr = new IexprGroup([new ParenGroup([new String('x')])])
    const interp = new InterpGroup([iexpr])
    prepareNestedConversions(interp, d())
    assert.ok(iexpr.items[0] instanceof ConvertedNode)
  })

  it('counts an interpolation expression as a logical depth frame', () => {
    let nested: unknown = new InterpGroup([new IexprGroup([new String('x')])])
    for (let i = 0; i < 10000; i++) {
      nested = [nested]
    }
    // The pre-pass DEFERS the breach as a failure cell (Go raises the
    // limit when its source-ordered walk enters the frame, so an earlier
    // error elsewhere must be able to win); the ordinary converter then
    // rethrows it with the expression's own interpolation wrapper.
    const depth = d()
    prepareNestedConversions(nested, depth)
    assert.throws(
      () => convertTopLevelValueInner(nested, depth),
      (e: any) =>
        e instanceof Error &&
        e.message.startsWith('interpolation expression error: [boru/evaluation_limit]:') &&
        e.cause instanceof BoruError &&
        e.cause.code === 'evaluation_limit',
    )
  })

  it('a source-earlier conversion error beats a later depth-limit breach', () => {
    // Go converts in source order and reports the `1_` literal before it
    // ever enters the too-deep bracket run; the TS pre-pass must not let
    // its eager depth accounting preempt that. Pinned against parser/go,
    // which raises [boru/syntax_error] for this exact source.
    const src = '[1_ ' + '['.repeat(10001) + ']'.repeat(10001) + ']'
    assert.throws(
      () => parse(src),
      (e: any) =>
        e instanceof BoruError &&
        e.code === 'syntax_error' &&
        e.message.includes('misplaced `_`'),
    )
    // The breach itself still raises once it IS the first error in
    // source order.
    const deep = '['.repeat(10001) + ']'.repeat(10001)
    assert.throws(
      () => parse(deep),
      (e: any) => e instanceof BoruError && e.code === 'evaluation_limit',
    )
  })

  it('restores interpolation error wrappers around preconverted structures', () => {
    for (const src of ['`${[1,,2]}`', '`${{a/ss}}`', '`${(1,,2)}`']) {
      assert.throws(
        () => parse(src),
        (error: any) =>
          error instanceof Error &&
          !(error instanceof BoruError) &&
          error.message.startsWith('interpolation expression error: [boru/syntax_error]:') &&
          error.cause instanceof BoruError,
        src,
      )
    }

    // Each nested interpolation is a separate Go fmt.Errorf("...: %w")
    // frame. The explicit work stack must rebuild both frames and causes.
    assert.throws(
      () => parse('`${`${[1,,2]}`}`'),
      (error: any) =>
        error instanceof Error &&
        error.message.startsWith(
          'interpolation expression error: interpolation expression error: [boru/syntax_error]:',
        ) &&
        error.cause instanceof Error &&
        error.cause.cause instanceof BoruError,
    )
  })

  it('normalises unknown thrown values and unknown source positions', () => {
    assert.equal(errMessage(new Error('boom')), 'boom')
    assert.equal(errMessage('boom'), 'boom')

    assert.throws(
      () => convertTopLevelItems([
        new NumberVal(1, '1', 0, 0),
        new String('.'),
        new String('x'),
      ], d()),
      (e: any) => '' === e.src && '' === e.word,
    )
    assert.throws(
      () => convertTopLevelItems([new String('.')], d()),
      (e: any) => '' === e.src && '' === e.word,
    )
  })

  it('handles raw strings and positionless invalid generic heads', () => {
    assert.equal(String(convertDataValueInner('x', d())), 'word(x)')
    const quoted = new String('x') as String & Record<string, unknown>
    Object.defineProperty(quoted, '__info__', {
      value: { quote: "'" },
      writable: true,
    })
    assert.equal(convertDataValueInner(quoted, d()).data, 'x')

    // angleUseSite precomputes and stores generic-head errors. Raw direct
    // nodes carry UNKNOWN_POS, so diagnostic source defaults are empty.
    const lower: any = convertTopLevelValueInner(
      new AngleGroup('Box', [new String('lower')]),
      d(),
    )
    assert.match(lower.data.headErr, /capitalised name/)

    const missing: any = convertTopLevelValueInner(
      new AngleGroup('Box', [new String('T'), new String('extends')]),
      d(),
    )
    assert.match(missing.data.headErr, /needs a value/)
  })

  it('applies a standalone prefix modifier to a completed raw reach', () => {
    // Source modifiers are folded into the final key token by the grammar
    // (the shared `a.b /s` row owns that behavior). This direct intermediate
    // is the compatible seam for a caller that supplies `/s` separately.
    const values = convertTopLevelItems([
      new String('a'),
      new String('.'),
      new String('b'),
      new String('/s'),
    ], d())
    assert.deepEqual(values.map(String), ['sugar(stack-args)', 'a.b'])
  })

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
