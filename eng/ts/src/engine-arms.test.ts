// Unit probes for engine.ts's remaining arms: syntax guards (unmatched
// parens, stray end), the zero-value table, forward-collection edge
// errors, sugar-marker misses, interp-string resolution inside parens,
// the async-handler refusal, and the fn-recursion induction hatch.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { BoruError, Engine, Registry, TSugar, Value, newInteger, newMark, newMove, newWord } from './index.ts'
import { newOpenParen } from '@boru-lang/core'
import { runCheckRow } from './check-fixture.ts'
import { registerSpecWords, renderStack, tokenize } from './spec-fixture.ts'

function run(input: string): string {
  const r = new Registry()
  registerSpecWords(r)
  return renderStack(new Engine(r).run(tokenize(input)))
}

function runErr(input: string): string {
  try {
    run(input)
  } catch (e) {
    if (e instanceof BoruError) return e.message
    throw e
  }
  return 'NO-ERROR'
}

describe('engine syntax guards', () => {
  it('an unmatched close paren refuses', () => {
    assert.match(runErr(') 1'), /unmatched '\)'/)
  })
  it('an unmatched open paren refuses', () => {
    assert.match(runErr('(1 2'), /unmatched opening parenthesis/)
  })
})

describe('word special forms', () => {
  it('none pushes the none value', () => {
    assert.equal(run('none'), 'none')
  })
  it('a programmatic Word(none) resolves in the engine itself', () => {
    // tokenize converts `none` at parse time, so the engine's own arm
    // needs a hand-built token.
    const r = new Registry()
    registerSpecWords(r)
    assert.equal(renderStack(new Engine(r).run([newWord('none')])), 'none')
  })
  // The pointer walks past an unmatched `(` — it is a scan boundary, as in
  // core/go — and the run refuses it when it ENDS with the paren still
  // unmatched. (This was titled "refuses in the scanner": the refusal is
  // the same, the moment is the end of the run.)
  it('a programmatic unmatched open paren is refused when the run ends', () => {
    const r = new Registry()
    registerSpecWords(r)
    assert.throws(
      () => new Engine(r).run([newOpenParen(), newInteger(1n)]),
      (e: unknown) => e instanceof BoruError && /unmatched '\('/.test(e.message),
    )
  })
})

describe('omitted optional params take base values (pinned to Go)', () => {
  it('every scalar family has a base', () => {
    assert.equal(run('def f fn [[s:String ?] [String] [s]] f'), "''")
    assert.equal(run('def f fn [[b:Boolean ?] [Boolean] [b]] f'), 'false')
    assert.equal(run('def f fn [[x:Float ?] [Float] [x]] f'), '0.0')
    assert.equal(run('def f fn [[n:Integer ?] [Integer] [n]] f'), '0')
  })
  it('container and atom bases mirror BaseValue', () => {
    // The base list rides the binding quote like any list arg.
    assert.equal(run('def f fn [[l:List ?] [Any] [l]] f'), '(quote [])')
    assert.equal(run('def f fn [[m:Map ?] [Any] [m]] f'), '{}')
    assert.equal(run('def f fn [[a:Atom ?] [Any] [a]] f'), '/q')
  })
})

describe('forward collection shapes (pinned to Go)', () => {
  it('a parked marker fires at the statement end', () => {
    assert.equal(run('5 addq 3 ;'), '8')
    assert.equal(run('addq 1 2'), '3')
  })
  it('an interp-string forward arg resolves during the scan', () => {
    assert.equal(run("concatq `v=${1}` 'y'"), "'yv=1'")
  })
  it("'end' with missing args fails the dispatch", () => {
    assert.match(runErr('addq 1 ;'), /no signature matches/)
  })
})

describe('typed-def constraint fallbacks (pinned to Go)', () => {
  it('literal constraints admit equal values and refuse others', () => {
    assert.equal(run('def x:5 5 x'), '5')
    assert.equal(run('def x:5.5 5.5 x'), '5.5')
    assert.equal(run('def x:-3 -3 x'), '-3')
    assert.equal(run('def x:true true x'), 'true')
    assert.equal(run('def x:none none x'), 'none')
    assert.match(runErr('def x:5 6 x'), /value 6 does not satisfy declared type 5/)
    assert.match(runErr('def x:zz 5 x'), /does not satisfy declared type word\(zz\)/)
  })
  it('the null constraint and value resolve to the atom', () => {
    assert.equal(run('def x:null null x'), 'null/q')
  })
  it('a def-resolved value satisfies its type constraint', () => {
    assert.equal(run('def y 7 def x:Integer y x'), '7')
  })
})

describe('data-eval shapes (pinned to Go)', () => {
  it('a multi-result paren map value collects into a list', () => {
    assert.equal(run('{a:(1 2)}'), '{a:[1 2]}')
  })
  it('non-xml list elements stringify inside xml interpolation', () => {
    assert.equal(run('<a>${[1 2]}</a>'), '<a>12</a>')
  })
})

describe('forward collection edges', () => {
  it('a mistyped forward arg fails the dispatch', () => {
    assert.match(runErr("addq 1 'x'"), /no signature matches/)
  })
  it("'end' before all forward args collected refuses", () => {
    assert.match(runErr('addq 1 ;'), /no signature matches/)
  })
})

describe('paren scan resolution', () => {
  it('interp strings resolve inside parens', () => {
    assert.equal(run("def x 5 (`v=${x}` concatq '!')"), "'v=5!'")
  })
})

describe('marker and token edge arms', () => {
  it('a sugar token with no payload steps over', () => {
    const r = new Registry()
    registerSpecWords(r)
    const out = new Engine(r).run([new Value(TSugar, null), newInteger(1n)])
    assert.equal(out.length, 2)
  })
  it('an orphan move with no mark drops silently', () => {
    const r = new Registry()
    registerSpecWords(r)
    const out = new Engine(r).run([newMove('m9'), newInteger(4n)])
    assert.equal(out.length, 1)
    assert.equal(out[0]!.data, 4n)
  })
  it('a second move to a consumed mark drops and forgets the id', () => {
    const r = new Registry()
    registerSpecWords(r)
    const out = new Engine(r).run([newMark('m1', []), newMove('m1'), newMove('m1'), newInteger(7n)])
    assert.equal(out[out.length - 1]!.data, 7n)
  })
  it('a recursive fn under the check pass takes the induction hatch', () => {
    // The declared return is the induction hypothesis; a fn that declares
    // none breaks the cycle with a dynamic Any.
    assert.equal(runCheckRow('def f fn [[n:Integer] [Integer] [f n]] f 1'), 'Integer')
    assert.equal(runCheckRow('def f fn [[n:Integer] [] [f n]] f 1'), 'dynamic(Any)')
  })
})

describe('async handler refusal', () => {
  it('a promise-returning handler is refused', () => {
    const r = new Registry()
    registerSpecWords(r)
    r.registerNativeFunc({
      name: 'zzasync',
      signatures: [{ args: [], handler: (() => Promise.resolve([])) as never }],
    })
    assert.throws(
      () => new Engine(r).run(tokenize('zzasync')),
      (e: unknown) => e instanceof BoruError && /async handlers are not supported/.test(e.message),
    )
  })
})
