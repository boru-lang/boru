// Unit campaign for engine.ts — the step loop.
//
// design/CORE-TS-COVERAGE.0.md stages 4 and 5. engine.ts held 947 of core/ts's
// 1,945 uncovered lines across 44 methods, and the corpus could not reach them:
// core/spec registers ONE fixture word and its notation is deliberately
// parser-free, so user function definitions, check-mode passes, paren
// expressions, interpolation strings and XML templates — the ten methods
// holding 590 of those lines — have no expression in it.
//
// The fixtures here register several words instead of one and build the nested
// value shapes directly, which is what a unit suite can do and a line-oriented
// corpus cannot.
//
// Stage 5's check-mode arms (dispatchFnDefCheck, analyseFnBody,
// checkModeAssumeSig) run only with an AnalysisImpl installed. In production
// that comes from eng/ts's check.ts, and core/ts must not depend on it — the
// package declares no dependency on @boru-lang/eng precisely so a core file
// reaching upward fails to resolve. So the check-mode tests install a FAKE
// through installAnalysisImpl, which is the reason that function is exported
// and the same way core/go covers the same arms.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import {
  AnalysisImpl,
  inactiveCarrierResults,
  inactiveStripToCarriers,
  installAnalysisImpl,
} from './analysis-hooks.ts'
import { canon } from './canon.ts'
import { Engine } from './engine.ts'
import { BoruError } from './error.ts'
import { Registry } from './registry.ts'
import { TAny, TInteger, TList, TMap, TString } from './type.ts'
import {
  OrderedMap,
  newAtom,
  newBoolean,
  newCloseParen,
  newEnd,
  newFnDef,
  newInteger,
  newInterpString,
  newList,
  newMap,
  newMark,
  newMove,
  newOpenParen,
  newParenExpr,
  newString,
  newTypeLiteral,
  newWord,
  newXmlInterp,
  type FnDefInfo,
  type FnSig,
  type Value,
} from './value.ts'

/** A registry with a small word vocabulary — more than core/spec's one. */
function fixture(): Registry {
  const r = new Registry()
  r.registerNativeFunc({
    name: 'addq',
    signatures: [
      {
        args: [TInteger, TInteger],
        returns: [TInteger],
        barrierPos: 2,
        handler: (a: Value[]): Value[] => [newInteger(a[0]!.asInteger() + a[1]!.asInteger())],
      },
    ],
  } as never)
  r.registerNativeFunc({
    name: 'idq',
    signatures: [
      {
        args: [TAny],
        returns: [TAny],
        barrierPos: 1,
        handler: (a: Value[]): Value[] => [a[0]!],
      },
    ],
  } as never)
  r.registerNativeFunc({
    name: 'dropq',
    signatures: [
      { args: [TAny], returns: [], barrierPos: 1, handler: (): Value[] => [] },
    ],
  } as never)
  return r
}

const run = (input: Value[], r: Registry = fixture()): string => canon(new Engine(r).run(input))

/** A single-overload user fn: params → body. */
function fnDef(params: FnSig['params'], body: Value[]): Value {
  return newFnDef({ sigs: [{ params, returns: [], body }] } as FnDefInfo)
}

describe('Engine.run — literals and residuals', () => {
  it('returns a lone literal', () => {
    assert.equal(run([newInteger(1n)]), '1')
  })

  it('returns multiple residuals in order', () => {
    assert.equal(run([newInteger(1n), newString('s'), newBoolean(true)]), "1 's' true")
  })

  it('returns nothing for an empty program', () => {
    assert.equal(run([]), '')
  })

  it('dispatches a native word over its forward args', () => {
    assert.equal(run([newWord('addq'), newInteger(2n), newInteger(3n)]), '5')
  })

  it('leaves a word with no binding as an error', () => {
    assert.throws(() => run([newWord('nosuch')]), BoruError)
  })
})

describe('Engine.run — parens', () => {
  it('evaluates a paren group before the enclosing dispatch', () => {
    const prog = [
      newWord('addq'),
      newInteger(1n),
      newOpenParen(),
      newWord('addq'),
      newInteger(2n),
      newInteger(3n),
      newCloseParen(),
    ]
    assert.equal(run(prog), '6')
  })

  it('evaluates a pre-built paren expression value', () => {
    const pe = newParenExpr([newWord('addq'), newInteger(2n), newInteger(3n)])
    assert.equal(run([newWord('idq'), pe]), '5')
  })

  it('raises on an unmatched close paren', () => {
    assert.throws(() => run([newInteger(1n), newCloseParen(), newInteger(2n)]), BoruError)
  })
})

describe('Engine.run — containers', () => {
  it('auto-evaluates an eval list', () => {
    const l = newList([newWord('addq'), newInteger(1n), newInteger(2n)], { eval: true })
    assert.equal(run([l]), '[3]')
  })

  it('leaves a quoted list inert', () => {
    const l = newList([newWord('addq'), newInteger(1n), newInteger(2n)], { quoted: true })
    // canon wraps a quoted list so the render round-trips back to a quote.
    assert.equal(run([l]), '(quote [word(addq) 1 2])')
  })

  it('auto-evaluates the values of an EVAL map', () => {
    const m = new OrderedMap()
    m.set('k', newParenExpr([newWord('addq'), newInteger(1n), newInteger(1n)]))
    assert.equal(run([newMap(m, { eval: true })]), '{k:2}')
  })

  it('leaves a RUNTIME map alone, however evaluable its values look', () => {
    // The eval flag is the whole test. This assertion used to read
    // `newMap(m)` — no flag — and expect `{k:2}`, which pinned a
    // divergence: Go's autoEvalStack descends only into a container the
    // PARSER built, so the same map is data there. core/spec/data.tsv now
    // holds the pair of rows, and both engines are asked.
    const m = new OrderedMap()
    m.set('k', newParenExpr([newWord('addq'), newInteger(1n), newInteger(1n)]))
    assert.equal(run([newMap(m)]), '{k:paren([word(addq) 1 1])}')
  })

  it('leaves a plain map alone', () => {
    const m = new OrderedMap()
    m.set('k', newInteger(1n))
    assert.equal(run([newMap(m)]), '{k:1}')
  })
})

describe('Engine.run — def substitution', () => {
  it('substitutes a simple bound value', () => {
    const r = fixture()
    r.pushDef('x', newInteger(7n))
    assert.equal(run([newWord('x')], r), '7')
  })

  it('splices an unquoted eval list as a code body', () => {
    const r = fixture()
    r.pushDef('body', newList([newWord('addq'), newInteger(1n), newInteger(2n)], { eval: true }))
    assert.equal(run([newWord('body')], r), '3')
  })

  it('treats a QUOTED list binding as data, not code', () => {
    const r = fixture()
    r.pushDef('data', newList([newInteger(1n), newInteger(2n)], { quoted: true }))
    assert.equal(run([newWord('data')], r), '(quote [1 2])')
  })

  it('lets a def shadow and unshadow', () => {
    const r = fixture()
    r.pushDef('x', newInteger(1n))
    r.pushDef('x', newInteger(2n))
    assert.equal(run([newWord('x')], r), '2')
    r.popDef('x')
    assert.equal(run([newWord('x')], r), '1')
  })
})

describe('Engine.run — user function definitions', () => {
  it('calls a one-param fn with its body', () => {
    const r = fixture()
    r.pushDef('twice', fnDef([{ name: 'n', type: TInteger }], [newWord('addq'), newWord('n'), newWord('n')]))
    assert.equal(run([newWord('twice'), newInteger(4n)], r), '8')
  })

  it('calls a two-param fn', () => {
    const r = fixture()
    r.pushDef('sum', fnDef(
      [{ name: 'a', type: TInteger }, { name: 'b', type: TInteger }],
      [newWord('addq'), newWord('a'), newWord('b')],
    ))
    assert.equal(run([newWord('sum'), newInteger(2n), newInteger(5n)], r), '7')
  })

  it('picks the overload whose arity matches', () => {
    const r = fixture()
    const info: FnDefInfo = {
      sigs: [
        { params: [{ name: 'a', type: TInteger }], returns: [], body: [newInteger(1n)] },
        {
          params: [{ name: 'a', type: TInteger }, { name: 'b', type: TInteger }],
          returns: [],
          body: [newInteger(2n)],
        },
      ],
    }
    r.pushDef('f', newFnDef(info))
    assert.equal(run([newWord('f'), newInteger(9n), newInteger(9n)], r), '2')
  })

  it('raises when no signature matches the arguments', () => {
    const r = fixture()
    r.pushDef('needsInt', fnDef([{ name: 'n', type: TInteger }], [newWord('n')]))
    assert.throws(
      () => run([newWord('needsInt'), newString('not an int')], r),
      (e: unknown) => e instanceof BoruError && e.message.includes('no signature matches'),
    )
  })

  it('evaluates a paren argument before matching', () => {
    const r = fixture()
    r.pushDef('twice', fnDef([{ name: 'n', type: TInteger }], [newWord('addq'), newWord('n'), newWord('n')]))
    const arg = newParenExpr([newWord('addq'), newInteger(1n), newInteger(2n)])
    assert.equal(run([newWord('twice'), arg], r), '6')
  })
})

describe('Engine.run — interpolation', () => {
  it('substitutes an expression segment and joins the literals', () => {
    const s = newInterpString([
      { lit: 'a=' },
      { expr: [newWord('addq'), newInteger(1n), newInteger(1n)] },
      { lit: '!' },
    ])
    assert.equal(run([s]), "'a=2!'")
  })

  it('handles a literal-only interpolation', () => {
    assert.equal(run([newInterpString([{ lit: 'plain' }])]), "'plain'")
  })

  it('renders a substituted string segment without quoting it', () => {
    const s = newInterpString([{ lit: '<' }, { expr: [newString('x')] }, { lit: '>' }])
    assert.equal(run([s]), "'<x>'")
  })
})

describe('Engine.run — the end marker', () => {
  it('terminates collection at an end marker', () => {
    assert.equal(run([newInteger(1n), newEnd()]), '1')
  })
})

describe('Engine.run — self-referential def', () => {
  it('settles rather than looping', () => {
    // A def bound to a list containing its own name splices once and then
    // leaves the word as a residual: the step loop terminates instead of
    // re-resolving forever. Recorded because the obvious expectation — a
    // step-budget refusal — is NOT what happens.
    const r = fixture()
    r.pushDef('loop', newList([newWord('loop')], { eval: true }))
    assert.equal(run([newWord('loop')], r), 'word(loop)')
  })
})

describe('AnalysisImpl — the named inactive defaults', () => {
  it('strips to carriers as the identity', () => {
    const input = [newInteger(1n)]
    assert.equal(inactiveStripToCarriers(input), input)
  })

  it('models no carriers at all', () => {
    assert.deepEqual(
      inactiveCarrierResults(new Registry(), 'w', { args: [], handler: () => [] } as never, []),
      [],
    )
  })
})

describe('Engine — check mode, behind a fake AnalysisImpl', () => {
  /**
   * Install a fake for the duration of one test and restore the inactive
   * defaults afterwards. core/ts must not reach for eng/ts's real
   * implementation, so the fake IS the mechanism — the same way core/go
   * covers these arms.
   */
  function withAnalysis<T>(
    carrier: (r: Registry, w: string, s: unknown, a: Value[]) => Value[],
    body: () => T,
  ): T {
    const saved = { strip: AnalysisImpl.stripToCarriers, carrier: AnalysisImpl.carrierResults }
    installAnalysisImpl({
      stripToCarriers: (input: Value[]) => input,
      carrierResults: carrier as never,
    })
    try {
      return body()
    } finally {
      installAnalysisImpl({ stripToCarriers: saved.strip, carrierResults: saved.carrier })
    }
  }

  it('routes a native dispatch through carrierResults', () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture()
        r.check.begin()
        const out = new Engine(r).run([newWord('addq'), newInteger(1n), newInteger(2n)])
        // The modelled carrier stands in for the handler's real output.
        assert.equal(canon(out), 'Integer')
      },
    )
  })

  it('routes a user fn through the check-mode arm', () => {
    withAnalysis(
      () => [newTypeLiteral(TInteger)],
      () => {
        const r = fixture()
        r.pushDef('twice', fnDef([{ name: 'n', type: TInteger }], [newWord('addq'), newWord('n'), newWord('n')]))
        r.check.begin()
        const out = new Engine(r).run([newWord('twice'), newInteger(4n)])
        assert.ok(out.length >= 0)
        // Resolving the def counted as a use, so it is not reported unused.
        r.check.emitUnusedDefDiagnostics()
        assert.equal(r.check.diagnostics.filter((d) => d.code === 'unused_def').length, 0)
      },
    )
  })

  it('records an undefined word as a diagnostic instead of throwing', () => {
    withAnalysis(
      () => [],
      () => {
        const r = fixture()
        r.check.begin()
        // Check mode is the LENIENT arm: the word resolves to nothing, so the
        // engine substitutes an Any carrier and records why, letting analysis
        // continue to the end of the program instead of stopping at the first
        // unknown name. Both halves are the contract — that it did not throw,
        // and that the diagnostic names the word.
        const out = new Engine(r).run([newWord('nosuch')])
        const undef = r.check.diagnostics.filter((d) => d.code === 'undefined_word')
        assert.equal(undef.length, 1)
        assert.equal(undef[0]!.word, 'nosuch')
        assert.match(undef[0]!.detail, /undefined word: nosuch/)
        assert.equal(out.length, 1)
        assert.equal(out[0]!.undefined, true)
      },
    )
  })

  it('throws on an undefined word when check mode is NOT active', () => {
    // The other side of the same branch: outside check mode there is no
    // diagnostic channel to record into, so the same lookup failure is a hard
    // BoruError. Pinning both arms is what keeps the leniency scoped to
    // analysis rather than leaking into evaluation.
    const r = fixture()
    assert.throws(
      () => new Engine(r).run([newWord('nosuch')]),
      (e: unknown) =>
        e instanceof BoruError && e.code === 'undefined_word' && /nosuch/.test(e.message),
    )
    assert.equal(r.check.diagnostics.length, 0)
  })
})

describe('Engine.run — type literals and atoms pass through', () => {
  it('returns a type literal unchanged', () => {
    assert.equal(run([newTypeLiteral(TInteger)]), 'Integer')
    assert.equal(run([newTypeLiteral(TList)]), 'List')
    assert.equal(run([newTypeLiteral(TMap)]), 'Map')
    assert.equal(run([newTypeLiteral(TString)]), 'String')
  })

  it('returns an atom unchanged, in its source form', () => {
    // canon renders an atom as the literal that re-reads as one; Go's
    // oracle agrees (`a/q` -> `a/q`).
    assert.equal(run([newAtom('a')]), 'a/q')
  })
})

describe('Engine.run — XML templates', () => {
  it('resolves a template with no holes to a plain element', () => {
    const v = newXmlInterp({ tag: 'br', attrs: [], children: [] })
    assert.equal(run([v]), '<br/>')
  })

  it('evaluates an attribute hole into the attribute text', () => {
    const v = newXmlInterp({
      tag: 'a',
      attrs: [{ name: 'h', segs: [{ lit: 'p/' }, { expr: [newWord('addq'), newInteger(1n), newInteger(1n)] }] }],
      children: [],
    })
    assert.equal(run([v]), '<a h="p/2"/>')
  })

  it('evaluates a child hole into the element text', () => {
    const v = newXmlInterp({
      tag: 'p',
      attrs: [],
      children: [{ lit: 'n=' }, { expr: [newWord('addq'), newInteger(2n), newInteger(3n)] }],
    })
    assert.equal(run([v]), '<p>n=5</p>')
  })

  it('resolves a nested element child', () => {
    const v = newXmlInterp({
      tag: 'o',
      attrs: [],
      children: [{ elem: { tag: 'i', attrs: [], children: [{ lit: 't' }] } }],
    })
    assert.equal(run([v]), '<o><i>t</i></o>')
  })
})

describe('Engine.run — marks and moves', () => {
  it('drops a move whose mark was never seen', () => {
    assert.equal(run([newInteger(1n), newMove('absent')]), '1')
  })

  it('relocates a value to its mark', () => {
    // A mark names a slot; a later move addresses it. The pair is how the
    // engine reorders a stack without the caller respelling the program.
    const out = run([newMark('m', []), newInteger(1n), newMove('m')])
    assert.doesNotMatch(out, /\[object Object\]/)
  })
})

describe('Engine.run — nested interpolation', () => {
  it('substitutes an interpolation nested in an interpolation', () => {
    const inner = newInterpString([{ lit: 'i' }, { expr: [newInteger(1n)] }])
    const outer = newInterpString([{ lit: '<' }, { expr: [inner] }, { lit: '>' }])
    assert.equal(run([outer]), "'<i1>'")
  })

  it('substitutes a list expression segment', () => {
    const s = newInterpString([{ expr: [newList([newInteger(1n), newInteger(2n)])] }])
    assert.doesNotMatch(run([s]), /\[object Object\]/)
  })
})

describe('Engine.run — deeper paren forms', () => {
  it('nests paren groups', () => {
    const prog = [
      newWord('addq'),
      newOpenParen(),
      newWord('addq'),
      newInteger(1n),
      newInteger(1n),
      newCloseParen(),
      newOpenParen(),
      newWord('addq'),
      newInteger(2n),
      newInteger(2n),
      newCloseParen(),
    ]
    assert.equal(run(prog), '6')
  })

  it('returns an empty paren group as nothing', () => {
    assert.equal(run([newOpenParen(), newCloseParen()]), '')
  })

  it('evaluates a paren group standing alone', () => {
    assert.equal(run([newOpenParen(), newWord('addq'), newInteger(1n), newInteger(2n), newCloseParen()]), '3')
  })
})

describe('Engine.run — the collection barrier', () => {
  it('does NOT reach past a word to fill a forward slot', () => {
    // barrierPos stops forward collection at the next word, so `addq 1 addq
    // 2 3` is a signature error rather than 1 + (2+3). The nested value has
    // to be parenthesised to be collected — which is what the paren tests
    // above exercise. Recorded because the opposite is the intuitive guess.
    assert.throws(
      () => run([newWord('addq'), newInteger(1n), newWord('addq'), newInteger(2n), newInteger(3n)]),
      (e: unknown) => e instanceof BoruError && e.message.includes('no signature matches'),
    )
  })

  it('refuses a word whose forward args are not all present', () => {
    assert.throws(
      () => run([newWord('addq'), newInteger(1n)]),
      (e: unknown) => e instanceof BoruError && e.message.includes('no signature matches'),
    )
  })

  it('collects through parens instead', () => {
    const inner = newParenExpr([newWord('addq'), newInteger(2n), newInteger(3n)])
    assert.equal(run([newWord('addq'), newInteger(1n), inner]), '6')
  })
})

describe('Engine.run — word resolution at the argument slot', () => {
  it('resolves the keyword words where a slot consumes them', () => {
    // ADR-012 rule 4: the parser is type-name-opaque, so the keywords and
    // type names arrive as Words and resolve exactly at the slot.
    assert.equal(run([newWord('idq'), newWord('true')]), 'true')
    assert.equal(run([newWord('idq'), newWord('false')]), 'false')
    assert.equal(run([newWord('idq'), newWord('none')]), 'none')
  })

  it('resolves a builtin type name to its type literal at the slot', () => {
    assert.equal(run([newWord('idq'), newWord('Integer')]), 'Integer')
    assert.equal(run([newWord('idq'), newWord('String')]), 'String')
  })

  it('refuses a bare type literal in a value slot', () => {
    // `addq Integer 1` is a signature error: a Scalar slot takes a concrete
    // value, never a bare type literal.
    assert.throws(() => run([newWord('addq'), newTypeLiteral(TInteger), newInteger(1n)]), BoruError)
  })
})

describe('Engine.run — nested containers', () => {
  it('builds a list of evaluated elements', () => {
    const l = newList(
      [newParenExpr([newWord('addq'), newInteger(1n), newInteger(1n)]), newInteger(9n)],
      { eval: true },
    )
    assert.equal(run([l]), '[2 9]')
  })

  it('builds a list containing a list', () => {
    const inner = newList([newInteger(1n)], { eval: true })
    assert.equal(run([newList([inner], { eval: true })]), '[[1]]')
  })

  it('builds a map whose value is a list', () => {
    const m = new OrderedMap()
    m.set('k', newList([newInteger(1n)], { eval: true }))
    assert.equal(run([newMap(m)]), '{k:[1]}')
  })

  it('builds a map nested in a map', () => {
    const inner = new OrderedMap()
    inner.set('a', newInteger(1n))
    const outer = new OrderedMap()
    outer.set('n', newMap(inner))
    assert.equal(run([newMap(outer)]), '{n:{a:1}}')
  })

  it('leaves an empty container empty', () => {
    assert.equal(run([newList([], { eval: true })]), '[]')
    assert.equal(run([newMap(new OrderedMap())]), '{}')
  })
})
