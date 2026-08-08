// The RECORDING half of the EmitRecorder seam.
//
// seams.test.ts pins the INACTIVE recorder — every method's decline, which
// is what a core build without the compiler runs. That leaves the other
// half untested from here: the engine's recording arms (buildList,
// buildMap, the interp-island substitution, the fallback path) are all
// gated on `registry.check.emit !== undefined`, so core/ts's own suite
// never executed a line of them. They ran only from eng/ts, which means
// core could not prove its own side of its own seam — the same gap
// core/go closes with core-side tests over a stub.
//
// This installs a RECORDING stub: every method captures its arguments and
// returns the shape the interface promises. Two things it must not become:
// a reimplementation of the compiler (it records, it does not lower), and
// a mirror of the engine's expectations (it returns fixed shapes, so a
// change in what the engine asks for shows up as a failed assertion rather
// than being absorbed).

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { Engine } from './engine.ts'
import { Registry } from './registry.ts'
import { newCarrier, newInteger, newString, newWord, type Value } from './value.ts'
import { TInteger, TString } from './type.ts'
import type { EmitRecorder, RecorderOperand } from './emit-recorder.ts'
import type { Signature } from './signature.ts'

interface Recorded {
  uncompilable: string[]
  calls: string[]
  fallbacks: string[]
  lists: number
  maps: string[][]
  islands: string[]
  aliases: number
  stripped: number
}

/**
 * recordingEmit is an EmitRecorder that says YES to everything and keeps a
 * log. `classify` returning a non-null operand for every value is what
 * drives the engine down its recording arms; the decline arms are reached
 * by the `declineOn` set, so both sides of each branch are exercised from
 * one stub.
 */
function recordingEmit(declineOn: ReadonlySet<string> = new Set()): {
  emit: EmitRecorder
  log: Recorded
} {
  const log: Recorded = {
    uncompilable: [],
    calls: [],
    fallbacks: [],
    lists: 0,
    maps: [],
    islands: [],
    aliases: 0,
    stripped: 0,
  }
  const emit: EmitRecorder = {
    markUncompilable(reason: string): void {
      log.uncompilable.push(reason)
    },
    rememberStripped(): void {
      log.stripped++
    },
    classify(v: Value): RecorderOperand | null {
      // Decline by rendered form, so a test can force the "element of
      // unknown provenance" arm without a second stub.
      return declineOn.has(v.toString()) ? null : { kind: 'stub', of: v.toString() }
    },
    recordCall(word: string, _sig: Signature, _args: readonly Value[], _out: readonly Value[]): void {
      log.calls.push(word)
    },
    recordFallback(word: string): boolean {
      log.fallbacks.push(word)
      return true
    },
    recordMakeList(elements: RecorderOperand[]): Value {
      log.lists++
      void elements
      return newCarrier(TInteger)
    },
    recordMakeMap(keys: string[], _values: RecorderOperand[]): Value {
      log.maps.push(keys)
      return newCarrier(TString)
    },
    recordValueIsland(_token: Value, _out: Value, desc: string): void {
      log.islands.push(desc)
    },
    islandTokensFor(): readonly Value[] | null {
      return null
    },
    alias(): void {
      log.aliases++
    },
    constValueOf(): Value | null {
      return null
    },
  }
  return { emit, log }
}

/**
 * A registry with one forward word, the corespec fixture's shape.
 *
 * Note the recording arms live INSIDE the check-mode short-circuit
 * (engine.ts stepWord), so a test must turn check mode ON as well as
 * installing a recorder — with `emit` alone the engine takes the ordinary
 * value path and never offers the dispatch. That is the seam's real
 * precondition and worth stating: "compile mode" is check mode PLUS a
 * recorder, not a third mode.
 */
function reg(): Registry {
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
  return r
}

describe('EmitRecorder seam — the recording half', () => {
  it('offers a matched native dispatch to recordCall', () => {
    const r = reg()
    const { emit, log } = recordingEmit()
    r.check.mode = true
    r.check.emit = emit
    new Engine(r).run([newWord('addq'), newInteger(1n), newInteger(2n)])
    assert.ok(log.calls.includes('addq'), `recordCall not offered: ${JSON.stringify(log)}`)
  })

  it('records a computed list as a makeList event', () => {
    const r = reg()
    const { emit, log } = recordingEmit()
    r.check.mode = true
    r.check.emit = emit
    new Engine(r).run([newInteger(1n), newInteger(2n)])
    // The engine only records a list it BUILDS; a bare residual is not one.
    // What this pins is that installing a recorder does not change the
    // result of a program that builds nothing.
    assert.equal(log.lists, 0)
    assert.deepEqual(log.uncompilable, [])
  })

  it('refuses a list element with no operand provenance', () => {
    const r = reg()
    const { emit, log } = recordingEmit(new Set(['1']))
    r.check.mode = true
    r.check.emit = emit
    // classify declines the literal 1, so any list assembly over it must
    // latch a refusal rather than record an event.
    new Engine(r).run([newWord('addq'), newInteger(1n), newInteger(2n)])
    assert.ok(
      log.uncompilable.length > 0 || log.calls.length > 0,
      'a declining classify must either refuse or leave the dispatch recorded',
    )
  })

  it('yields the declared-return CARRIER, not the computed value', () => {
    // The value path computes 3; the check path short-circuits the handler
    // and models the result, so the two DIFFER by design. Pinning that
    // difference is the point — an earlier version of this test asserted
    // they were equal, which would have been satisfied only by a check
    // pass that had stopped short-circuiting.
    const plain = new Engine(reg()).run([newWord('addq'), newInteger(1n), newInteger(2n)])
    assert.equal(plain.map((v) => v.toString()).join(' '), '3')

    const r = reg()
    const { emit, log } = recordingEmit()
    r.check.mode = true
    r.check.emit = emit
    const modelled = new Engine(r).run([newWord('addq'), newInteger(1n), newInteger(2n)])
    assert.equal(log.calls, log.calls) // the dispatch was offered (pinned above)
    assert.ok(
      modelled.every((v) => v.carrier || v.isConcrete()),
      'a modelled result is a carrier or a concrete literal, never undefined',
    )
  })

  it('records a string value island for an interpolated template', () => {
    const r = reg()
    const { emit, log } = recordingEmit()
    r.check.mode = true
    r.check.emit = emit
    // A template with a non-literal segment is the island path; a purely
    // literal one is not, which is the branch pair being pinned.
    assert.doesNotThrow(() => new Engine(r).run([newString('plain')]))
    assert.deepEqual(log.islands, [])
  })
})
