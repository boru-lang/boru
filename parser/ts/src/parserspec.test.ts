// The TS half of the parser-level parity corpus (parser/spec/*.tsv).
//
// parser/go/parserspec_test.go is the other half. The two runners share NO
// code — they read the same files and each decodes the escape notation and
// renders the value stream independently, exactly as core/spec's pair does.
// That independence is the point: shared scaffolding can hide the same bug
// from both engines (design/legacy/CORE-GO-TS-DEFECTS.0.ignore, blind spot 9), and a
// parser corpus with a shared reader would hide precisely the class of defect
// design/legacy/TS-PARITY-AUDIT.0.ignore found.
//
// Six files, six contracts (lex.tsv and data.tsv have their own strict
// runners in lexspec.test.ts and dataspec.test.ts because they render
// tokens and decoded data rather than parsed values):
//
//   parse.tsv      src -> expected. Both engines must produce `expected`.
//   divergent.tsv  src -> go, ts.   The LIVE parity-debt ledger: each row is a
//                                   real measured divergence. This runner
//                                   re-renders every row and must reproduce
//                                   the `ts` column exactly, so a fixed
//                                   divergence fails loudly and the row moves
//                                   to parse.tsv. The row count is pinned in
//                                   both runners.
//   lex.tsv        src, EOF status -> exact trivia-preserving token stream,
//                                   including UTF-8 byte offsets.
//   data.tsv       src -> decode.  The safe DATA-decode seam
//                                   (safeParseData + convertParsedNumber).
//   nesting.tsv    kind, depth -> outcome. Each runner independently builds
//                                   the source, checks OK/error code, and walks
//                                   every successful spine iteratively.
//   shape.tsv      case, src -> semantic shape. Positions, flags, modifier
//                                   payloads, nested values, and diagnostics
//                                   omitted by the canonical render.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  BoruError,
  TAtom,
  TInteger,
  TList,
  TMap,
  Value,
  asReach,
  asSugar,
  canon,
  canonValue,
  type WordInfo,
} from '@boru-lang/core'
import { parse } from './index.ts'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const SPEC_DIR = path.resolve(__dirname, '..', '..', 'spec')
const PARSE_SPEC_ROW_COUNT = 725
const DIVERGENT_SPEC_ROW_COUNT = 9
const NESTING_SPEC_ROW_COUNT = 18
const SHAPE_SPEC_ROW_COUNT = 28
// canon-fixpoint.tsv: ADR-015's shrink-only ledger of parse.tsv rows that do
// not yet reach their canon fixpoint (NUR072). The Go runner pins the same.
const CANON_FIXPOINT_ROW_COUNT = 0

interface SpecRow {
  line: number
  src: string
  cols: string[]
}

interface NestingSpecRow {
  line: number
  kind: string
  depth: number
  expected: string
}

interface ShapeSpecRow {
  line: number
  name: string
  src: string
  expected: string
}

/**
 * decodeSpecEscapes turns the corpus's \n, \t and \\ back into the bytes they
 * stand for, so one source can span a line of the file.
 */
function decodeSpecEscapes(s: string): string {
  let out = ''
  for (let i = 0; i < s.length; i++) {
    if ('\\' === s[i] && i + 1 < s.length) {
      const next = s[i + 1]
      if ('n' === next) {
        out += '\n'
        i++
        continue
      }
      if ('t' === next) {
        out += '\t'
        i++
        continue
      }
      if ('\\' === next) {
        out += '\\'
        i++
        continue
      }
    }
    out += s[i]
  }
  return out
}

/**
 * readSpec decodes one corpus file. A row is tab-separated; '#' at the start
 * of a line is a comment and blank lines are skipped.
 */
function readSpec(name: string): SpecRow[] {
  const text = fs.readFileSync(path.join(SPEC_DIR, name), 'utf8')
  const wantColumns = name === 'parse.tsv' || name === 'canon-fixpoint.tsv' ? 3 : 4
  const rows: SpecRow[] = []
  const seen = new Map<string, number>()
  const lines = text.split('\n')
  for (let n = 0; n < lines.length; n++) {
    const line = lines[n]!
    // A line is a COMMENT only when it starts with '#' AND carries no tab.
    // '#' is boru's own comment marker, so sources begin with it; treating
    // every '#' line as a comment silently drops those rows, which is the
    // failure this corpus exists to make impossible.
    if ('' === line || (line.startsWith('#') && !line.includes('\t'))) continue
    const parts = line.split('\t')
    assert.equal(
      parts.length,
      wantColumns,
      `${name}:${n + 1}: need exactly ${wantColumns} tab-separated columns`,
    )
    // EVERY column is escaped, not just the source: a render can itself
    // contain a newline (XML text spanning lines), which would otherwise split
    // one row across two lines and silently truncate it.
    const src = decodeSpecEscapes(parts[0]!)
    assert.equal(
      seen.has(src),
      false,
      `${name}:${n + 1}: duplicate source (first at line ${seen.get(src)}): ${JSON.stringify(src)}`,
    )
    seen.set(src, n + 1)
    const cols = parts.slice(1).map(decodeSpecEscapes)
    if (name === 'divergent.tsv') {
      assert.notEqual(cols[2]!.trim(), '', `${name}:${n + 1}: divergence justification must not be blank`)
      assert.notEqual(cols[0], cols[1], `${name}:${n + 1}: equal renders belong in parse.tsv`)
    }
    rows.push({
      line: n + 1,
      src,
      cols,
    })
  }
  const wantRows =
    name === 'parse.tsv'
      ? PARSE_SPEC_ROW_COUNT
      : name === 'canon-fixpoint.tsv'
        ? CANON_FIXPOINT_ROW_COUNT
        : DIVERGENT_SPEC_ROW_COUNT
  assert.equal(rows.length, wantRows, `${name}: exact row-count ratchet`)
  return rows
}

/**
 * Read the compact nesting contract. This is deliberately separate from
 * readSpec: nesting.tsv has exactly three columns and describes generated
 * sources, rather than storing source/render pairs.
 */
function readNestingSpec(): NestingSpecRow[] {
  const name = 'nesting.tsv'
  const text = fs.readFileSync(path.join(SPEC_DIR, name), 'utf8')
  const rows: NestingSpecRow[] = []
  const seen = new Map<string, number>()
  const lines = text.split('\n')
  for (let n = 0; n < lines.length; n++) {
    const line = lines[n]!
    if (line === '' || line.startsWith('#')) continue
    const parts = line.split('\t')
    assert.equal(
      parts.length,
      3,
      `${name}:${n + 1}: need exactly 3 tab-separated columns (kind, depth, expected outcome)`,
    )
    const depth = Number(parts[1])
    assert.ok(Number.isSafeInteger(depth) && depth > 0, `${name}:${n + 1}: depth must be a positive integer`)
    const expected = parts[2]!
    assert.ok(
      expected === 'OK' || expected === 'ERR evaluation_limit',
      `${name}:${n + 1}: expected outcome must be OK or ERR evaluation_limit`,
    )
    const kind = parts[0]!
    const key = `${kind}\u0000${depth}`
    assert.equal(
      seen.has(key),
      false,
      `${name}:${n + 1}: duplicate kind/depth ${JSON.stringify(kind)}/${depth} (first at line ${seen.get(key)})`,
    )
    seen.set(key, n + 1)
    rows.push({ line: n + 1, kind, depth, expected })
  }
  assert.equal(rows.length, NESTING_SPEC_ROW_COUNT, `${name}: exact row-count ratchet`)
  return rows
}

/**
 * Independently read the TS half of the semantic-shape oracle. It has a strict
 * three-column schema and unique case names; optional trailing columns would
 * make a malformed oracle too easy for one implementation to ignore.
 */
function readShapeSpec(): ShapeSpecRow[] {
  const name = 'shape.tsv'
  const text = fs.readFileSync(path.join(SPEC_DIR, name), 'utf8')
  const rows: ShapeSpecRow[] = []
  const seen = new Set<string>()
  const lines = text.split('\n')
  for (let n = 0; n < lines.length; n++) {
    const line = lines[n]!
    if (line === '' || line.startsWith('#')) continue
    const parts = line.split('\t')
    assert.equal(
      parts.length,
      3,
      `${name}:${n + 1}: need exactly 3 tab-separated columns (case, source, expected shape)`,
    )
    const caseName = parts[0]!
    assert.ok(
      caseName !== '' && caseName.trim() === caseName,
      `${name}:${n + 1}: case name must be non-empty with no surrounding whitespace`,
    )
    assert.ok(!seen.has(caseName), `${name}:${n + 1}: duplicate case name ${JSON.stringify(caseName)}`)
    seen.add(caseName)
    const expected = decodeSpecEscapes(parts[2]!)
    assert.notEqual(expected, '', `${name}:${n + 1}: expected shape must not be empty`)
    rows.push({
      line: n + 1,
      name: caseName,
      src: decodeSpecEscapes(parts[1]!),
      expected,
    })
  }
  assert.equal(rows.length, SHAPE_SPEC_ROW_COUNT, `${name}: exact row-count ratchet`)
  return rows
}

/** Independently expand one compact row into boru source. */
function nestingSource(kind: string, depth: number): string {
  switch (kind) {
    case 'list':
      return '['.repeat(depth) + '1' + ']'.repeat(depth)
    case 'map':
      return '{a:'.repeat(depth) + '1' + '}'.repeat(depth)
    case 'paren':
      return '('.repeat(depth) + '1' + ')'.repeat(depth)
    case 'typed-list':
      return '[:'.repeat(depth) + 'Integer' + ']'.repeat(depth)
    case 'typed-map':
      return '{:'.repeat(depth) + 'Integer' + '}'.repeat(depth)
    case 'mixed': {
      const opens: string[] = []
      const closes: string[] = []
      for (let i = 0; i < depth; i++) {
        switch (i % 3) {
          case 0:
            opens.push('[')
            closes.push(']')
            break
          case 1:
            opens.push('{a:')
            closes.push('}')
            break
          case 2:
            opens.push('(')
            closes.push(')')
            break
        }
      }
      closes.reverse()
      return opens.join('') + '1' + closes.join('')
    }
    default:
      throw new Error(`nesting.tsv: unknown kind ${JSON.stringify(kind)}`)
  }
}

/** Parse without canonically rendering the recursively deep result. */
function nestingOutcome(src: string): { values: Value[]; outcome: string } {
  try {
    return { values: parse(src), outcome: 'OK' }
  } catch (e) {
    return {
      values: [],
      outcome: e instanceof BoruError ? `ERR ${e.code}` : 'ERR unstructured',
    }
  }
}

/**
 * Iteratively validate every generated container. A parse that silently
 * truncates, flattens, or changes one node still returns OK, so acceptance
 * alone is not the deep-value contract.
 */
function assertNestingSpine(values: Value[], kind: string, depth: number): void {
  assert.equal(values.length, 1, `${kind} depth ${depth}: exactly one root value`)
  let current = values[0]!
  for (let level = 0; level < depth; level++) {
    const wantKind = kind === 'mixed' ? ['list', 'map', 'paren'][level % 3]! : kind
    switch (wantKind) {
      case 'list': {
        assert.ok(
          current.vType.equal(TList) && !current.isTypedList() && Array.isArray(current.data),
          `${kind} depth ${depth}: level ${level + 1} is not an untyped List`,
        )
        const items = current.asList()
        assert.equal(items.length, 1, `${kind} depth ${depth}: level ${level + 1} List arity`)
        current = items[0]!
        break
      }
      case 'map': {
        assert.ok(
          current.vType.equal(TMap) && !current.isTypedMap(),
          `${kind} depth ${depth}: level ${level + 1} is not an untyped Map`,
        )
        const entries = current.asMap()
        assert.deepEqual(entries.keys(), ['a'], `${kind} depth ${depth}: level ${level + 1} Map keys`)
        current = entries.get('a')!
        assert.ok(current !== undefined, `${kind} depth ${depth}: level ${level + 1} Map child`)
        break
      }
      case 'paren': {
        assert.ok(current.isParenExpr(), `${kind} depth ${depth}: level ${level + 1} is not a ParenExpr`)
        const items = current.asList()
        assert.equal(items.length, 1, `${kind} depth ${depth}: level ${level + 1} ParenExpr arity`)
        current = items[0]!
        break
      }
      case 'typed-list':
      case 'typed-map': {
        const rightKind = wantKind === 'typed-list' ? current.isTypedList() : current.isTypedMap()
        assert.ok(rightKind, `${kind} depth ${depth}: level ${level + 1} typed-container kind`)
        const child = current.asChildType()
        assert.equal(child.elements.length, 0, `${kind} depth ${depth}: level ${level + 1} typed elements`)
        assert.equal(child.entries.length, 0, `${kind} depth ${depth}: level ${level + 1} typed entries`)
        current = child.child
        break
      }
      default:
        assert.fail(`nesting.tsv: unknown kind ${JSON.stringify(wantKind)}`)
    }
  }

  if (kind === 'typed-list' || kind === 'typed-map') {
    assert.ok(current.isWord(), `${kind} depth ${depth}: terminal is not a Word`)
    assert.equal(current.asWord().name, 'Integer', `${kind} depth ${depth}: terminal word`)
    return
  }
  assert.ok(current.vType.equal(TInteger), `${kind} depth ${depth}: terminal is not an Integer`)
  assert.equal(current.asInteger(), 1n, `${kind} depth ${depth}: terminal integer`)
}

/**
 * renderSpec is the corpus's `expected` contract: the canon of each parsed
 * value, space-joined, or 'ERR ' and the first line of the error text.
 */
function renderSpec(src: string): string {
  try {
    return canon(parse(src))
  } catch (e) {
    const msg = e instanceof Error ? (e.message.split('\n')[0] ?? '') : String(e)
    return 'ERR ' + msg
  }
}

function shapeBit(value: boolean | undefined): string {
  return value === true ? '1' : '0'
}

function shapeQuote(value: string): string {
  return JSON.stringify(value) as string
}

function renderShapeStrings(items: string[]): string {
  return '[' + items.map(shapeQuote).join(',') + ']'
}

function renderShapeValues(items: Value[]): string {
  return '[' + items.map(renderShapeValue).join(',') + ']'
}

function renderShapeChildEntries(entries: { key: string; value: Value }[]): string {
  return (
    '[' + entries.map((entry) => shapeQuote(entry.key) + '=>' + renderShapeValue(entry.value)).join(',') + ']'
  )
}

function renderShapeOptionalValue(value: Value | undefined): string {
  return value === undefined ? 'none' : renderShapeValue(value)
}

/**
 * The TS-local semantic shape renderer. Canon remains the payload label, while
 * this layer exposes parser metadata canon intentionally omits and recursively
 * walks the structural payloads used by shape.tsv.
 */
function renderShapeValue(v: Value): string {
  const p = v.pos
  let out =
    'v(c=' +
    shapeQuote(canonValue(v)) +
    ',p=' +
    p.row +
    ':' +
    p.col +
    ':' +
    shapeQuote(p.src) +
    ',e=' +
    shapeBit(v.eval) +
    ',q=' +
    shapeBit(v.quoted)

  if (v.isWord()) {
    const word = v.asWord() as WordInfo & { forceVal?: boolean; forceUsurp?: boolean }
    out +=
      ',w={name=' +
      shapeQuote(word.name) +
      ',argc=' +
      (word.argCount ?? -1) +
      ',stack=' +
      shapeBit(word.forceStack) +
      ',forward=' +
      shapeBit(word.forceForward) +
      ',val=' +
      shapeBit(word.forceVal) +
      ',usurp=' +
      shapeBit(word.forceUsurp) +
      '}'
  } else if (v.vType.equal(TAtom)) {
    out += ',atom=' + shapeQuote(v.asAtom())
  }
  const sugar = asSugar(v)
  if (sugar !== undefined) {
    out +=
      ',sugar={kind=' +
      shapeQuote(sugar.kind) +
      ',n=' +
      (sugar.n ?? 0n).toString() +
      ',name=' +
      shapeQuote(sugar.name ?? '') +
      ',src=' +
      shapeQuote(sugar.src ?? '') +
      ',head=' +
      renderShapeOptionalValue(sugar.head) +
      ',headErr=' +
      shapeQuote(sugar.headErr ?? '') +
      ',items=' +
      renderShapeValues(sugar.items ?? []) +
      '}'
  }

  if (v.isTypedList() || v.isTypedMap()) {
    const child = v.asChildType()
    out +=
      ',typed={kind=' +
      shapeQuote(v.isTypedMap() ? 'map' : 'list') +
      ',child=' +
      renderShapeValue(child.child) +
      ',elements=' +
      renderShapeValues(child.elements) +
      ',entries=' +
      renderShapeChildEntries(child.entries) +
      '}'
  } else if (v.isParenExpr()) {
    out += ',paren=' + renderShapeValues(v.asList())
  } else {
    const reach = asReach(v)
    if (reach !== undefined) {
      const segments = reach.segments.map((segment) => {
        const op = segment.getr ? 'getr' : 'dot'
        return segment.computed
          ? op + '(expr=' + renderShapeValues(segment.keyExpr ?? []) + ')'
          : op + '(lit=' + renderShapeValue(segment.keyLit!) + ')'
      })
      out +=
        ',reach={eval=' +
        shapeBit(reach.eval) +
        ',recv=' +
        renderShapeValues(reach.receiver) +
        ',segs=[' +
        segments.join(',') +
        ']}'
    } else if (v.vType.equal(TList) && Array.isArray(v.data)) {
      out += ',list=' + renderShapeValues(v.asList())
    } else if (v.isMap()) {
      const entries = v.asMap()
      const rawComputed = entries.meta?.['ck']
      if (rawComputed !== undefined && !(rawComputed instanceof Set)) {
        throw new Error('shape renderer: map meta.ck is not a Set')
      }
      const computed = rawComputed instanceof Set ? [...rawComputed].map(String).sort() : []
      out +=
        ',map={implicit=' +
        shapeBit(entries.implicit) +
        ',computed=' +
        renderShapeStrings(computed) +
        ',entries=[' +
        entries
          .keys()
          .map((key) => shapeQuote(key) + '=>' + renderShapeValue(entries.get(key)!))
          .join(',') +
        ']}'
    }
  }

  return out + ')'
}

function renderShape(src: string): string {
  try {
    return 'values=' + renderShapeValues(parse(src))
  } catch (e) {
    if (!(e instanceof BoruError)) {
      const message = e instanceof Error ? e.message : String(e)
      return 'error(unstructured=' + shapeQuote(message) + ')'
    }
    return (
      'error(code=' +
      shapeQuote(e.code) +
      ',detail=' +
      shapeQuote(e.detail) +
      ',p=' +
      e.row +
      ':' +
      e.col +
      ':' +
      shapeQuote(e.src) +
      ',full=' +
      shapeQuote(e.fullSource) +
      ',hint=' +
      shapeQuote(e.hint) +
      ',notes=' +
      renderShapeStrings(e.notes) +
      ',help=' +
      renderShapeStrings(e.suggestions.map((suggestion) => suggestion.message)) +
      ')'
    )
  }
}

describe('parser spec — parse.tsv', () => {
  for (const r of readSpec('parse.tsv')) {
    it(`${r.line}: ${JSON.stringify(r.src)}`, () => {
      assert.equal(renderSpec(r.src), r.cols[0])
    })
  }
})

// The parity-debt ledger is LIVE rather than merely counted: this side
// re-renders every row's source and must reproduce the `ts` column exactly
// (the Go runner does the same against its `go` column). A divergence that
// gets fixed therefore fails here loudly — the row must then move to
// parse.tsv — and a row can never drift into recording behavior neither
// port has. readSpec validates the four-column/provenance form and the
// exact-count ratchet first, so debt cannot grow silently.
describe('parser spec — divergent.tsv (the live parity-debt ledger)', () => {
  for (const r of readSpec('divergent.tsv')) {
    it(`${r.line}: ${JSON.stringify(r.src)} still diverges as recorded`, () => {
      assert.equal(
        renderSpec(r.src),
        r.cols[1],
        'if the ports now agree here, move the row to parse.tsv',
      )
    })
  }
})

// ADR-015's fixpoint diagnostic (design/CANON-ROUNDTRIP.0.md §1, §4), the Go
// runner's TestParserCanonFixpoint over the same ledger: every parse.tsv row
// that parses must canon to text that, parsed again, canons the same — or be
// listed on canon-fixpoint.tsv with the record that closes it. A listed row
// that reaches its fixpoint fails until removed: the ledger only shrinks.
describe('parser spec — canon fixpoint (ADR-015)', () => {
  const ledger = new Map(readSpec('canon-fixpoint.tsv').map((r) => [r.src, r.cols[0]!]))
  const seen = new Set<string>()
  for (const r of readSpec('parse.tsv')) {
    const r1 = renderSpec(r.src)
    if (r1.startsWith('ERR')) continue
    if (ledger.has(r.src)) seen.add(r.src)
    it(`${r.line}: ${JSON.stringify(r.src)} reaches its fixpoint or is ledgered`, () => {
      const r2 = renderSpec(r1)
      if (ledger.has(r.src)) {
        assert.notEqual(r2, r1, `reaches its fixpoint now (${ledger.get(r.src)}): remove it from canon-fixpoint.tsv`)
      } else {
        assert.equal(r2, r1, 'not a fixpoint, and not on canon-fixpoint.tsv')
      }
    })
  }
  it('lists only parse.tsv rows that parse', () => {
    for (const src of ledger.keys()) assert.ok(seen.has(src), `canon-fixpoint.tsv lists ${JSON.stringify(src)}`)
  })
})

describe('parser spec — nesting.tsv', () => {
  for (const r of readNestingSpec()) {
    it(`${r.line}: ${r.kind} depth ${r.depth}`, () => {
      const src = nestingSource(r.kind, r.depth)
      const result = nestingOutcome(src)
      assert.equal(result.outcome, r.expected)
      if (result.outcome === 'OK') assertNestingSpine(result.values, r.kind, r.depth)
    })
  }
})

describe('parser spec — shape.tsv', () => {
  for (const r of readShapeSpec()) {
    it(`${r.line}: ${r.name}`, () => {
      assert.equal(renderShape(r.src), r.expected, JSON.stringify(r.src))
    })
  }
})
