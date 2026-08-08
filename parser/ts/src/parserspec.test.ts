// The TS half of the parser-level parity corpus (parser/spec/*.tsv).
//
// parser/go/parserspec_test.go is the other half. The two runners share NO
// code — they read the same files and each decodes the escape notation and
// renders the value stream independently, exactly as core/spec's pair does.
// That independence is the point: shared scaffolding can hide the same bug
// from both engines (design/CORE-GO-TS-DEFECTS.0.md, blind spot 9), and a
// parser corpus with a shared reader would hide precisely the class of defect
// design/TS-PARITY-AUDIT.0.md found.
//
// Two files, two contracts:
//
//   parse.tsv      src -> expected. Both engines must produce `expected`.
//   divergent.tsv  src -> go, ts.   The parity debt. Each runner asserts its
//                                   OWN column, so a divergence stays pinned
//                                   instead of drifting, and fixing one means
//                                   deleting the row.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

import { canon } from '@boru-lang/core'
import { parse } from './index.ts'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const SPEC_DIR = path.resolve(__dirname, '..', '..', 'spec')

interface SpecRow {
  line: number
  src: string
  cols: string[]
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
  const rows: SpecRow[] = []
  const lines = text.split('\n')
  for (let n = 0; n < lines.length; n++) {
    const line = lines[n]!
    // A line is a COMMENT only when it starts with '#' AND carries no tab.
    // '#' is boru's own comment marker, so sources begin with it; treating
    // every '#' line as a comment silently drops those rows, which is the
    // failure this corpus exists to make impossible.
    if ('' === line || (line.startsWith('#') && !line.includes('\t'))) continue
    const parts = line.split('\t')
    assert.ok(parts.length >= 2, `${name}:${n + 1}: need at least 2 tab-separated columns`)
    // EVERY column is escaped, not just the source: a render can itself
    // contain a newline (XML text spanning lines), which would otherwise split
    // one row across two lines and silently truncate it.
    rows.push({
      line: n + 1,
      src: decodeSpecEscapes(parts[0]!),
      cols: parts.slice(1).map(decodeSpecEscapes),
    })
  }
  // divergent.tsv is the parity DEBT, so empty is the goal state, not a
  // broken corpus. Every other file must have rows — an empty one there
  // means the corpus is not being read.
  assert.ok(rows.length > 0 || name === 'divergent.tsv', `${name}: no rows`)
  return rows
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

describe('parser spec — parse.tsv', () => {
  for (const r of readSpec('parse.tsv')) {
    it(`${r.line}: ${JSON.stringify(r.src)}`, () => {
      assert.equal(renderSpec(r.src), r.cols[0])
    })
  }
})

// Pins the TS side of every recorded divergence, and fails if a row has
// stopped diverging — a fixed divergence must be MOVED to parse.tsv, not left
// here, or the file stops being an honest debt list.
describe('parser spec — divergent.tsv (parity debt)', () => {
  const divergentRows = readSpec('divergent.tsv')
  if (divergentRows.length === 0) {
    // The debt is paid. Kept as a live assertion rather than deleted: the
    // file is the ratchet, and a NEW divergence has to be added here
    // deliberately (with the justification the header demands) instead of
    // quietly landing as a changed expectation elsewhere.
    it('is empty — parser/go and parser/ts agree on every corpus row', () => {
      assert.equal(divergentRows.length, 0)
    })
  }
  for (const r of divergentRows) {
    it(`${r.line}: ${JSON.stringify(r.src)}`, () => {
      assert.ok(r.cols.length >= 2, 'need src, go, ts columns')
      const [wantGo, wantTs] = [r.cols[0]!, r.cols[1]!]
      assert.notEqual(wantGo, wantTs, 'go and ts columns are identical — move this row to parse.tsv')
      assert.equal(renderSpec(r.src), wantTs)
    })
  }
})
