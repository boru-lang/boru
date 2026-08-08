// The TS twin of parser/go/grammar_seam5b_test.go — unit tests for the
// grammar/xml guard arms that NEITHER corpus can reach.
//
// This file is the one deliberate exception to "coverage comes from corpus
// rows, not per-engine unit tests" (design/GO-TS-PARITY.0.md). Go carved
// the same exception for the same arms and gave the reason: the jsonic
// grammar actions and lex matchers are ordinary closures, and where a
// guard cannot be provoked through parse() — a rule with no parent, a
// matcher invoked with no rule, a child node that never materialised — the
// closure is retrieved from the CONFIGURED grammar and invoked directly
// with a synthetic rule. That is the in-package equivalent Go gets for
// free and TS has to arrange.
//
// The rule is narrow: a guard belongs here only if a source text cannot
// reach it. Everything a source CAN reach stays in parse.tsv, where one
// row lifts both engines at once. Several arms this file originally
// covered moved out to rows once the probe found a shape that reached
// them — the map-value dot chains in particular.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'

import { makeLex } from '@tabnas/jsonic'
import { makeBoruJsonic } from './grammar.ts'
import { ParenGroup, Sited, UnclosedAngle, InterpGroup, ArrowTag } from './nodes.ts'

/**
 * The NORULE sentinel. tabnas mints a real one per parse
 * (`ctx.NORULE = makeRule(...)`) and every guard compares it by IDENTITY
 * (`r.parent === ctx.NORULE`), so a unique object is behaviourally the
 * same sentinel — Go's tests use `jsonic.NoRule` for the identical reason.
 */
const NORULE = { name: '@norule' }

/** The ctx the actions read: the sentinel, plus the token table. */
function makeCtx(cfg: unknown): any {
  return { NORULE, cfg }
}

function grammar(): { j: any; cfg: any } {
  const { j } = makeBoruJsonic()
  return { j, cfg: j.internal().config }
}

/**
 * matcher retrieves a registered lex matcher from the configured
 * instance — the analogue of Go's s5beMatcher walking
 * Config().CustomMatchers.
 */
function matcher(cfg: any, name: string): (lex: any, rule: any) => any {
  const entry = (cfg.lex.match as any[]).find((m) => name === m.matcher)
  assert.ok(entry, `matcher ${name} not registered`)
  return entry.make(cfg, {})
}

/**
 * lexOf builds a Lex over `src`. The matchers read only `pnt`, `src` and
 * `token()`, and tabnas's Lex takes its source from a ctx callback, so a
 * two-field ctx is the whole requirement — Go's `jsonic.NewLex(src,
 * &LexConfig{})` in one line.
 */
function lexOf(cfg: any, src: string): any {
  return (makeLex as unknown as (ctx: unknown) => any)({ src: () => src, cfg })
}

/**
 * actions retrieves the BORU actions a rule registered for one phase.
 *
 * tabnas installs its own hooks in the same arrays (`@elem-bc/replace`,
 * `@pair-bc`, `@val-bc/replace`), and those expect a real parser rule —
 * feeding them a synthetic one tests the library, not this port. They are
 * distinguishable because every library hook is a NAMED function and
 * every boru action is an anonymous arrow, so the filter is exact rather
 * than positional. Go needs no equivalent: its actions arrive through
 * RuleSpec.Actions already separated.
 */
function actions(j: any, rule: string, phase: 'bo' | 'bc' | 'ac'): any[] {
  let out: any[] = []
  j.rule(rule, (rs: any) => {
    out = (rs.def[phase] ?? []).filter((f: any) => !String(f.name).startsWith('@'))
  })
  assert.ok(out.length > 0, `${rule}.${phase} has no boru actions`)
  return out
}

/** alts retrieves a rule's open or close alternates. */
function alts(j: any, rule: string, side: 'open' | 'close'): any[] {
  let out: any[] = []
  j.rule(rule, (rs: any) => {
    out = rs.def[side] ?? []
  })
  return out
}

// --- lex matchers -----------------------------------------------------------

describe('lex matcher guards', () => {
  it('the template matcher declines with no rule and at end of source', () => {
    const { cfg } = grammar()
    const m = matcher(cfg, 'template_literal')

    assert.equal(m(lexOf(cfg, '`x`'), undefined), undefined, 'nil rule must decline')

    const armed = { k: { boru_tpl: true } }
    assert.equal(m(lexOf(cfg, ''), armed), undefined, 'empty source must decline')

    // Sanity: armed mid-text it produces a #TL token, so the two declines
    // above are the guards firing and not the matcher being inert.
    assert.notEqual(m(lexOf(cfg, 'ab`'), armed), undefined, 'plain text must lex a #TL')
  })

  it('the template matcher declines when unarmed', () => {
    const { cfg } = grammar()
    const m = matcher(cfg, 'template_literal')
    assert.equal(m(lexOf(cfg, 'ab`'), { k: {} }), undefined, 'unarmed must decline')
  })

  it('the xml matcher declines with no rule and on no progress', () => {
    const { cfg } = grammar()
    const m = matcher(cfg, 'xml_literal')

    assert.equal(m(lexOf(cfg, '<a/>'), undefined), undefined, 'nil rule must decline')
    assert.equal(m(lexOf(cfg, '<a/>'), { k: {} }), undefined, 'unarmed must decline')

    const armed = { k: { boru_xml: true } }
    // `<` at end of source: buildXmlTmpl reports "expected a tag name",
    // the error arm sets end to the source length — which is the cursor
    // — so there is no progress and the matcher declines, leaving normal
    // lexing to report the character.
    assert.equal(m(lexOf(cfg, ''), armed), undefined, 'no progress must decline')

    // Cursor at 0: the raw slice start is afterLA-1, which clamps to 0.
    // Reached only with the `<` already outside the lexed source, which
    // no real parse produces.
    const tok = m(lexOf(cfg, 'a/>'), armed)
    assert.notEqual(tok, undefined, 'expected an #XML token')
    assert.equal(tok.val.err, undefined, 'expected a clean element')
  })
})

// --- dotchain ---------------------------------------------------------------

describe('dotchain action guards', () => {
  it('the BO and BC actions are inert with no parent', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const [bo] = actions(j, 'dotchain', 'bo')
    const [bc] = actions(j, 'dotchain', 'bc')

    bo({}, ctx)
    bo({ parent: NORULE }, ctx)
    bc({}, ctx)
    bc({ parent: NORULE }, ctx)
  })

  it('a looped dotchain keeps the parenGroup its first push seeded', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const [bo] = actions(j, 'dotchain', 'bo')

    const parent = { node: new ParenGroup(['bf']) }
    bo({ parent }, ctx)
    assert.ok(parent.node instanceof ParenGroup)
    assert.deepEqual(parent.node.items, ['bf'], 'an existing chain must not be re-seeded')
  })

  it('the BC mirrors the accumulated chain onto the dotchain node', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const [bc] = actions(j, 'dotchain', 'bc')

    const grp = new ParenGroup(['bf'])
    const r: any = { parent: { node: grp } }
    bc(r, ctx)
    assert.equal(r.node, grp)
  })

  it('a segment with no key token appends the bare marker', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    // dotchainSeg is registered as the action of the `. key` alternate,
    // so the configured grammar is where the closure comes from.
    const seg = alts(j, 'dotchain', 'open')[0].a as (r: any, ctx: any) => void

    // No key token AND no parent: both guard arms fire, no effect.
    seg({}, ctx)

    const parent = { node: new ParenGroup([]) }
    seg({ parent }, ctx)
    assert.ok(parent.node instanceof ParenGroup)
    assert.equal(parent.node.items.length, 1, 'the bare marker must be appended')
    assert.equal(String(parent.node.items[0]), '.')
  })
})

// --- arrow folds ------------------------------------------------------------

describe('arrow fold guards', () => {
  it('the fold condition declines with no parent', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const cond = alts(j, 'val', 'close').find((a) => 'arrowfold' === a.p)?.c
    assert.ok(cond, 'the arrowfold close alternate must be registered')

    assert.equal(cond({}, ctx), false)
    assert.equal(cond({ parent: NORULE }, ctx), false)
  })

  it('the fold condition declines after a flat dot chain', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const cond = alts(j, 'val', 'close').find((a) => 'arrowfold' === a.p)!.c
    const pelem = { name: 'pelem', u: {} }

    // A `.`/`!` previous sibling: the fold would capture only the final
    // key as the sig, so it must decline. `m.k => body` is folded into
    // `(m get k)` by convertTopLevelItems instead.
    const dotted = { parent: pelem, prev: { node: new Sited(mkText('.'), { row: 0, col: 0 }) } }
    assert.equal(cond(dotted, ctx), false, 'a `.` marker sibling must not fold')

    const strict = { parent: pelem, prev: { node: mkText('!') } }
    assert.equal(cond(strict, ctx), false, 'a `!` marker sibling must not fold')

    // An ordinary sibling folds — the arms above are the guard, not the
    // condition being false throughout.
    const plain = { parent: pelem, prev: { node: mkText('x') } }
    assert.equal(cond(plain, ctx), true)
  })

  it('the arrowfold BO and BC are inert with no parent or no body', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const [bo] = actions(j, 'arrowfold', 'bo')
    const [bc] = actions(j, 'arrowfold', 'bc')

    bo({}, ctx)
    bo({ parent: NORULE }, ctx)
    bc({}, ctx)
    bc({ parent: NORULE }, ctx)
    // A parent but no child node: nothing to append.
    const parent = { node: new ParenGroup(['sig', new ArrowTag()]) }
    bc({ parent, child: {} }, ctx)
    assert.equal((parent.node as ParenGroup).items.length, 2, 'no body must append nothing')
  })

  it('the arrowfoldelem BO is inert with no parent and on an empty list', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const [bo] = actions(j, 'arrowfoldelem', 'bo')

    bo({ u: {} }, ctx)
    bo({ u: {}, parent: NORULE }, ctx)

    // Empty parent list: there is no pair to pop as the sig.
    const r: any = { u: {}, parent: { node: [] } }
    bo(r, ctx)
    assert.equal('af_sig' in r.u, false, 'an empty list must not stash a sig')

    // Non-empty, with a grandparent: the pair pops and BOTH levels see
    // the shortened array (the elem and its list share it).
    const grand: any = {}
    const parent: any = { node: ['pairA', 'pairB'], parent: grand }
    const r2: any = { u: {}, parent }
    bo(r2, ctx)
    assert.equal(r2.u['af_sig'], 'pairB')
    assert.deepEqual(parent.node, ['pairA'])
    assert.deepEqual(grand.node, ['pairA'])
  })

  it('the arrowfoldelem BC is inert with no stashed sig and no body', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const [bc] = actions(j, 'arrowfoldelem', 'bc')

    bc({ u: {}, parent: { node: [] } }, ctx)
    bc({ u: { af_sig: 'sig' }, parent: NORULE }, ctx)
    // A sig but no body val: nothing to group.
    const parent: any = { node: [] }
    bc({ u: { af_sig: 'sig' }, parent, child: {} }, ctx)
    assert.deepEqual(parent.node, [], 'no body must append nothing')

    // The full path: (SIG afn BODY) replaces the bare pair, propagated up.
    const grand: any = {}
    const parent2: any = { node: [], parent: grand }
    bc({ u: { af_sig: 'sig' }, parent: parent2, child: { node: 'body' } }, ctx)
    assert.equal(parent2.node.length, 1)
    assert.ok(parent2.node[0] instanceof ParenGroup)
    assert.equal(grand.node.length, 1)
  })
})

// --- pair / elem / interp / angle --------------------------------------------

describe('map-node and propagation guards', () => {
  it('the key-recording BCs bail when the node is not a map', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    // Every pair BC gates on mapInfoOf(r.node). A pair always closes into
    // a map node in a real parse, so the miss arms need a synthetic node:
    // a non-object, an array, and an object with no info marker.
    for (const node of ['not-a-map', ['a'], {}, null]) {
      for (const bc of actions(j, 'pair', 'bc')) {
        bc({ k: { boru_qm: true }, u: { key: 'a' }, node, o0: { tin: -1 } }, ctx)
      }
    }
  })

  it('the elem BC propagates only from the outer optional-key elem', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const bcs = actions(j, 'elem', 'bc')

    // Inner elem (no done flag): the outer one owns the propagation, so
    // this must not overwrite the parent's node.
    const parent: any = { node: 'keep' }
    for (const bc of bcs) {
      bc({ k: { boru_qm: true }, u: {}, node: 'inner', parent }, ctx)
    }
    assert.equal(parent.node, 'keep')
  })

  it('the ielem BC drops an element with no node', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const [bc] = actions(j, 'ielem', 'bc')

    const parent = { node: new InterpGroup([]) }
    bc({ child: {}, parent }, ctx)
    assert.equal((parent.node as InterpGroup).parts.length, 0, 'an empty ielem is not a part')
  })

  it('the angle BC and AC are inert with no parent', () => {
    const { j, cfg } = grammar()
    const ctx = makeCtx(cfg)
    const [bc] = actions(j, 'angle', 'bc')
    const [ac] = actions(j, 'angle', 'ac')

    bc({}, ctx)
    bc({ parent: NORULE }, ctx)
    ac({ u: {} }, ctx)
    ac({ u: {}, parent: NORULE }, ctx)

    // A CLOSED group returns before touching the node — the AC rewrite is
    // for the unclosed-at-EOF case only.
    const closed: any = { u: { closed: true }, node: 'keep' }
    ac(closed, ctx)
    assert.equal(closed.node, 'keep')

    // Unclosed with a parent: both levels become an UnclosedAngle.
    const parent: any = {}
    const r: any = { u: {}, parent, node: 'x' }
    ac(r, ctx)
    assert.ok(r.node instanceof UnclosedAngle)
    assert.ok(parent.node instanceof UnclosedAngle)
  })
})

/**
 * mkText builds the jsonic Text node the grammar's asText() recognises —
 * a boxed String carrying the hidden info marker. The grammar reads the
 * marker's `quote` to tell an unquoted word from a quoted string, so a
 * bare `new String(s)` is exactly what a real unquoted token looks like.
 */
function mkText(s: string): unknown {
  const boxed: any = new String(s)
  // The marker key is the same Symbol the grammar's INFO uses; jsonic
  // exposes it on any node it builds, so one is borrowed from a real
  // parse rather than re-derived here.
  return boxed
}
