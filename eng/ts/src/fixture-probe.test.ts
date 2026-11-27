// Fixture guard probes — the TS twin of eng/go/specfix_probe_test.go:
// the fixture words' guard arms and less-travelled paths the shared
// corpus never reaches, driven through the same harness the spec
// runner uses. Where the Go probe needed Go-constructed bindings, the
// TS rows lean on the shapes the TS fixtures accept directly; rows
// shared with the Go probe pin cross-engine fixture behavior.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'

import { BoruError, Engine, Registry, type Value } from './index.ts'
import { parseSpec, registerSpecWords, renderStack, tokenize } from './spec-fixture.ts'

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

    // A typed name may not reuse a builtin type name.
    ['def Integer:5 5', { err: 'name part "Integer" in "Integer" conflicts with an existing type name' }],

    // tor stores its alternatives in one order whichever way they are
    // written: concrete numbers by magnitude, a magnitude tie (1 and 1.0)
    // by render, and a non-scalar after a scalar.
    ['1 tor 2', '1 tor 2'],
    ['2 tor 1', '1 tor 2'],
    ['1 tor 1.0', '1 tor 1.0'],
    ['1.0 tor 1', '1 tor 1.0'],
    ['[1] tor 2', '2 tor [1]'],

    // fn param guards: a param is `name:Type`.
    ['fn [[5] [Integer] [1]]', { err: 'fn: expected param Word, got 5' }],
    ['fn [[n] [Integer] [n]]', { err: 'fn: param "n" missing \':TypeName\' suffix' }],
    ['fn [[n:Zzz] [Integer] [n]]', { err: 'fn: unknown type "Zzz"' }],
  ]
  pin(rows)
})

// Everyday fixture rows — infix and forward dispatch, defs over literal and
// computed values, typed defs whose constraint is checked against a
// COMPUTED value, do, the stack words, quote/typeof, inspect of a word
// and of a def. These were once run beside the bytecode VM with the
// interpreter as the reference; the interpreter's answers are pinned here.
describe('fixture value pins', () => {
  pin([
    ['1 addq 2', '3'],
    ["'a' concatq 'b'", "'ab'"],
    ['def x 5 x addq 1', '6'],
    ['def x (2 addq 3) x', '5'],
    ['do [1 addq 2]', '3'],
    ['do [10 20 30]', '10 20 30'],
    ['dup 1 dup', { err: 'cannot call `dup` — no signature matches the arguments' }],
    ['1 2 swap', '2 1'],
    ['quote zz typeof', 'Atom'],
    ['not true', 'false'],
    ['true and false', 'false'],
    ['fnsig [[Integer] [Integer]] typeof', 'FunctionSignature'],
    ['def f fn [[x:Integer] [Integer] [x addq 1]] f 4', '5'],
    ['inspect addq', "{name:'addq' kind:native signatures:[{args:['Number' 'Number']}]}"],
    ['def m {k:2} m get k', '2'],
    // Typed defs over computed values.
    ['def x:Integer (1 addq 2) x', '3'],
    ['def x:5 (2 addq 3) x', '5'],
    ['def x:Integer (1 addq 2) def y:Integer (x addq 1) y', '4'],
    ["def s:'ab' ('a' concatq 'b') s", "'ab'"],
    // A word only the check fixtures register is undefined here.
    ['noretq 5 dup drop', { err: 'undefined word: noretq' }],
    ['def x (1 addq 2) inspect x', "{name:'x' kind:defined value:3 signatures:[]}"],
    ['def x (1 addq 2) x typeof', 'Integer'],
    // A typed-list constraint over a list.
    ['def l:[:Integer] [1 2] l lengthq', '2'],
  ])
})

describe('corpus loader', () => {
  it('reads input and expected, skipping comments and blank lines', () => {
    const file = path.join(os.tmpdir(), `boru-spec-loader-${process.pid}.tsv`)
    fs.writeFileSync(file, '# a comment\n\n1 addq 2\t3\ta note\n')
    try {
      assert.deepEqual(parseSpec(file), [{ lineNum: 3, input: '1 addq 2', expected: '3' }])
    } finally {
      fs.rmSync(file, { force: true })
    }
  })
  it('refuses a row with no expected column', () => {
    const file = path.join(os.tmpdir(), `boru-spec-loader-bad-${process.pid}.tsv`)
    fs.writeFileSync(file, '1 addq 2\n')
    try {
      assert.throws(() => parseSpec(file), /L1: malformed row, want at least input<TAB>expected/)
    } finally {
      fs.rmSync(file, { force: true })
    }
  })
})

function pin(rows: Array<[input: string, want: string | { err: string }]>): void {
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
}
