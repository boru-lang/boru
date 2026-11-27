// Unit probes for the matcher's literal-pattern arms, registry
// bookkeeping, and the float-canon exponent tails (the canon rows are
// pinned to the Go engine's CanonValue output).

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { Registry, type Value, newInteger, newString, newTypeLiteral } from './index.ts'
import { canonValue } from '@boru-lang/core'
import { matchValues } from '@boru-lang/core'
import { TInteger } from '@boru-lang/core'
import { newFloat } from '@boru-lang/core'

describe('literal-pattern matching arms', () => {
  const entry = (patterns: Map<number, Value>) => {
    const r = new Registry()
    r.registerNativeFunc({
      name: 'pw',
      signatures: [{ args: [TInteger], handler: (a) => [a[0]!], patterns }],
    })
    return r.lookup('pw')!
  }

  it('a type-literal pattern gates by type at stack positions', () => {
    const fn = entry(new Map([[0, newTypeLiteral(TInteger)]]))
    assert.equal(matchValues(fn, [newString('x')]), null)
    assert.notEqual(matchValues(fn, [newInteger(1n)]), null)
  })
})

describe('registry bookkeeping arms', () => {
  it('re-registering a word merges forward precedence', () => {
    const r = new Registry()
    r.registerNativeFunc({ name: 'w', signatures: [{ args: [], handler: () => [] }] })
    r.registerNativeFunc({
      name: 'w',
      forwardPrecedence: true,
      signatures: [{ args: [TInteger], handler: (a) => [a[0]!] }],
    })
    const entry = r.lookup('w')!
    assert.equal(entry.forwardPrecedence, true)
    assert.equal(entry.signatures.length, 2)
  })

  it('def-stack introspection reads through', () => {
    const r = new Registry()
    assert.equal(r.hasDef('d'), false)
    assert.equal(r.defStackDepth('d'), 0)
    r.pushDef('d', newInteger(1n))
    r.pushDef('d', newInteger(2n))
    assert.equal(r.hasDef('d'), true)
    assert.equal(r.defStackDepth('d'), 2)
    assert.deepEqual(r.defNames(), ['d'])
  })

  it('snapshot and restore round-trip defs and args', () => {
    const r = new Registry()
    r.pushDef('d', newInteger(1n))
    r.pushArgs([newInteger(9n)])
    const snap = r.snapshot()
    r.pushDef('d', newInteger(2n))
    r.restore(snap)
    assert.equal(r.defStackDepth('d'), 1)
  })
})

describe('float canon exponent tails (pinned to Go CanonValue)', () => {
  it('renders the strconv forms byte-identically', () => {
    assert.equal(canonValue(newFloat(1e21)), '1e+21')
    assert.equal(canonValue(newFloat(1.25e22)), '1.25e+22')
    assert.equal(canonValue(newFloat(1.5e-7)), '0.00000015')
    assert.equal(canonValue(newFloat(1e20)), '100000000000000000000.0')
  })
})
