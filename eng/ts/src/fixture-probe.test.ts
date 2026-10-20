// Fixture guard probes — the TS twin of eng/go/specfix_probe_test.go:
// the fixture words' guard arms and less-travelled paths the shared
// corpus never reaches, driven through the same harness the spec
// runner uses. Where the Go probe needed Go-constructed bindings, the
// TS rows lean on the shapes the TS fixtures accept directly; rows
// shared with the Go probe pin cross-engine fixture behavior.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { BoruError, Engine, Registry, type Value } from './index.ts'
import { registerSpecWords, renderStack, tokenize } from './spec-fixture.ts'

function run(input: string): { ok: true; got: string } | { ok: false; err: string } {
  const r = new Registry()
  registerSpecWords(r)
  let values: Value[]
  try {
    values = tokenize(input)
  } catch (e) {
    return { ok: false, err: e instanceof Error ? e.message : String(e) }
  }
  try {
    return { ok: true, got: renderStack(new Engine(r).run(values)) }
  } catch (e) {
    if (e instanceof BoruError) return { ok: false, err: e.message }
    return { ok: false, err: e instanceof Error ? e.message : String(e) }
  }
}

describe('fixture guard probes', () => {
  const rows: Array<[input: string, want: string | { err: string }]> = [
    // def family guards (shared with the Go probe suite).
    ['def {a:Integer b:Integer} 3', { err: 'exactly one key' }],
    ['def {x:String} 3', { err: 'does not satisfy' }],
    ['def f fn [1 2]', { err: 'multiple of 3' }],
    ['fn [1 2]', { err: 'multiple of 3' }],

    // word / splice.
    ['word 5', '5'],

    // get on containers: hits, misses (the None TYPE literal, the Go
    // reference), string keys on lists, and get-on-None. (The 1-arg
    // refine remains on the fixture-parity backlog.)
    ['[10 20 30] get 1', '20'],
    ['[10 20 30] get 5', 'None'],
    ['{a:1} get a', '1'],
    ['{a:1} get b', 'None'],
    ['[1 2] get a', 'None'],
    ['{a:1} get b get 1', 'None'],

    // do's escape hatch: a body error surfaces as an Error VALUE.
    ['do [notaword_xyz]', 'error(undefined word: notaword_xyz)'],

    // refine Record guards (the Go reference messages) and the 1-arg
    // bare form: the base passes through; a non-type errors.
    ['refine Record []', { err: 'at least one field' }],
    ['refine Record [5]', { err: 'must be a pair' }],
    ['refine Integer', 'Integer'],
    ['refine 5', { err: 'must be a type' }],

    // fnsig arms.
    ['fnsig [1]', { err: 'multiple of 2' }],

    // enum over words and typed lists.
    ['enum [a b]', 'a tor b'],

    // typed-def constraint fallback: an unknown constraint name goes
    // through the atomic-value converter; literal-value constraints
    // accept exactly their value.
    ['def x:zzz 1', { err: 'does not satisfy' }],
    ['def x:5 5', ''],
    ['def x:5 6', { err: 'does not satisfy declared type 5' }],
    ['def x:true true', ''],
    ["def x:'s' 's'", ''],
    ['def x:2.5 2.5', ''],
    ['def x:none 1', { err: 'does not satisfy declared type none' }],
    ['def tt Integer def y:tt 3 y', '3'],
    ['def y:[:Integer] [1 2] y', '[1 2]'],
    // Modifier-suffixed constraint names walk the full modifier scan:
    // valid suffixes strip to the base word, /q quotes, doubled or
    // unknown letters raise, /t builds the type-bound sugar, and the
    // float specials bind by value.
    ['def x:f/v 1', { err: 'declared type word(f)' }],
    ['def x:f/2 1', { err: 'declared type word(f)' }],
    ['def x:f/q 1', { err: 'declared type f' }],
    ['def x:go/vq 1', { err: 'invalid word modifier /vq on "go"' }],
    ['def x:a/3f 1', { err: 'declared type word(a)' }],
    ['def x:w/u 1', { err: 'declared type word(w)' }],
    ['def x:w/t 1', { err: 'declared type sugar(type-bound [[w]])' }],
    ['def x:w/ff 1', { err: 'invalid word modifier /ff on "w"' }],
    ['def x:w/99 1', { err: 'declared type word(w)' }],
    ['def x:-inf -inf', ''],
    ['def x:inf inf', ''],
    ['def x:nan nan', { err: 'does not satisfy declared type nan' }],
  ]
  for (const [input, want] of rows) {
    it(JSON.stringify(input), () => {
      const res = run(input)
      if (typeof want === 'string') {
        assert.ok(res.ok, res.ok ? '' : res.err)
        assert.equal(res.got, want)
      } else {
        assert.ok(!res.ok, res.ok ? `expected error, got ${res.got}` : '')
        assert.match(res.err, new RegExp(want.err.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))
      }
    })
  }
})
