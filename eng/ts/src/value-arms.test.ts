// Unit probes for value.ts's remaining arms: the predicate table over
// every value mode (the paths the engine suites reach only from one
// side), constructor round-trips, and the accessor miss paths.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import {
  TInteger,
  TList,
  TMap,
  TNone,
  TString,
} from '@boru-lang/core'
import {
  ChildType,
  ClassTypeInfo,
  ErrorInfo,
  OrderedMap,
  Value,
  asReach,
  isErrorValue,
  isReach,
  newAny,
  newClassType,
  newConstrainedWord,
  newErrorValue,
  newFloat,
  newForwardMarker,
  newInteger,
  newList,
  newMap,
  newMark,
  newMove,
  newNone,
  newOptions,
  newReach,
  newSplice,
  newString,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
  newWord,
  renderSugar,
} from '@boru-lang/core'
import { tokenize } from './spec-fixture.ts'

describe('value predicate table', () => {
  const none = newNone()
  const lit = newTypeLiteral(TInteger)
  const int = newInteger(5)
  const carrier = new Value(TMap, null, { carrier: true })

  it('isTypeLiteral excludes none and carriers', () => {
    assert.equal(lit.isTypeLiteral(), true)
    assert.equal(none.isTypeLiteral(), false)
    assert.equal(carrier.isTypeLiteral(), false)
    assert.equal(int.isTypeLiteral(), false)
  })

  it('isConcrete tracks payload presence', () => {
    assert.equal(int.isConcrete(), true)
    assert.equal(lit.isConcrete(), false)
    assert.equal(carrier.isConcrete(), false)
  })

  it('container predicates over every construction', () => {
    const om = new OrderedMap()
    om.set('a', int)
    const m = newMap(om)
    const l = newList([int])
    const tl = newTypedList(newTypeLiteral(TInteger))
    const tm = newTypedMap(newTypeLiteral(TString))
    assert.equal(m.isMap(), true)
    assert.equal(l.isMap(), false)
    assert.equal(tl.isTypedList(), true)
    assert.equal(l.isTypedList(), false)
    assert.equal(tm.isTypedMap(), true)
    assert.equal(m.isTypedMap(), false)
    assert.equal(tl.data instanceof ChildType, true)
  })

  it('error values carry their message and type', () => {
    const e = newErrorValue('boom')
    assert.equal(isErrorValue(e), true)
    assert.equal(isErrorValue(int), false)
    assert.equal((e.data as ErrorInfo).message, 'boom')
  })

  it('reach values round-trip and miss safely', () => {
    const r = newReach({ segments: [] } as never)
    assert.equal(isReach(r), true)
    assert.equal(isReach(int), false)
    assert.notEqual(asReach(r), undefined)
    assert.equal(asReach(int), undefined)
  })

  it('splices and words render distinctly', () => {
    const sp = newSplice(int)
    const w = newWord('go')
    assert.notEqual(sp.toString(), '')
    assert.equal(w.isWord(), true)
    assert.equal(int.isWord(), false)
  })

  it('class-type info paths compose', () => {
    const fields = new OrderedMap()
    fields.set('x', newTypeLiteral(TInteger))
    const info = new ClassTypeInfo('P', 'Class', fields)
    assert.equal(info.path, 'Class/P')
  })

  it('none and the None literal stay distinct', () => {
    assert.equal(none.isNone(), true)
    assert.equal(newTypeLiteral(TNone).isNone(), false)
    assert.equal(newTypeLiteral(TList).vType.equal(TList), true)
  })
})

describe('accessor mismatch guards', () => {
  const int = newInteger(5n)
  const str = newString('s')
  it('every scalar accessor rejects a foreign payload', () => {
    assert.throws(() => str.asInteger(), /not an integer value/)
    assert.throws(() => int.asFloat(), /not a float value/)
    assert.throws(() => int.asDecimal(), /not a float value/)
    assert.throws(() => int.asString(), /not a string value/)
    assert.throws(() => int.asBoolean(), /not a boolean value/)
    assert.throws(() => int.asWord(), /not a word value/)
    assert.throws(() => int.asAtom(), /not an atom value/)
    assert.throws(() => int.asMap(), /not a map value/)
    assert.throws(() => int.asDisjunct(), /not a disjunct value/)
    assert.throws(() => int.asList(), /not a list value/)
    assert.equal(newFloat(0.5).asDecimal(), 0.5)
  })
})

describe('marker and container renders', () => {
  it('forward, mark, and move render their state', () => {
    const f = newForwardMarker({ funcName: 'f', collected: [], expectedForward: 2 } as never)
    assert.equal(String(f), 'forward(f,0/2)')
    assert.equal(String(newMark('m1', [])), 'mark(m1)')
    assert.equal(String(newMove('m1')), 'move(m1,)')
  })

  it('typed containers render child and inline content', () => {
    // The child type literal renders by its LEAF name, matching Go:
    // core.NewTypedListWithElements(core.NewTypeLiteral(core.TInteger), …)
    // .String() is `[:Integer 1]`. These assertions previously pinned the
    // full path, which is what let the divergence stand
    // (design/legacy/TS-PARITY-AUDIT.0.ignore).
    const tl = newTypedList(newTypeLiteral(TInteger), [newInteger(1n)])
    assert.equal(String(tl), '[:Integer 1]')
    const tm = newTypedMap(newTypeLiteral(TString), [{ key: 'k', value: newString('v') }])
    assert.match(String(tm), /^\{:String k:/)
  })

  it('sugar renders carry their operands', () => {
    assert.equal(renderSugar({ kind: 'force-arity', n: 2n }), 'sugar(force-arity 2)')
    assert.equal(renderSugar({ kind: 'mini', name: 'sql', src: 's' }), "sugar(mini sql 's')")
  })

  it('an xml value renders through canonXml', () => {
    const toks = tokenize("<a x='1'>hi</a>")
    assert.equal(String(toks[0]), '<a x="1">hi</a>')
  })

  it('options wrap a map and asFnDef rejects scalars', () => {
    const om = new OrderedMap()
    om.set('k', newInteger(1n))
    assert.equal(newOptions(om).isOptions(), true)
    assert.equal(newInteger(2n).isOptions(), false)
    assert.throws(() => newInteger(5n).asFnDef(), /not a function value/)
  })
})

describe('auxiliary constructors', () => {
  it('constrained words carry their constraint', () => {
    const c = newConstrainedWord('w', newTypeLiteral(TInteger))
    assert.equal(c.isWord(), true)
    assert.notEqual((c.data as { constraint?: Value }).constraint, undefined)
  })

  it('newAny and class types wrap their payloads', () => {
    assert.equal(newAny(null).vType.parts[0], 'Any')
    const fields = new OrderedMap()
    fields.set('x', newTypeLiteral(TInteger))
    const ct = newClassType(new ClassTypeInfo('P', 'Class', fields))
    assert.equal(ct.vType.equal(newTypeLiteral(TInteger).vType) === false, true)
    assert.equal((ct.data as ClassTypeInfo).path, 'Class/P')
  })
})
