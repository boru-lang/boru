// The TS half of the base-layer parity corpus (basic/spec/*.tsv).
//
// basic/go/basicspec_test.go is the other half. The two runners share NO
// code — they read the same files and each builds the registry and renders
// the residual independently, exactly as core/spec's and parser/spec's
// pairs do. That independence is the point: shared scaffolding can hide
// the same bug from both engines (design/CORE-GO-TS-DEFECTS.0.md, blind
// spot 9).
//
// This is a SPEC, not a differential: the `expected` column is the
// documented contract, so a row can legitimately fail on BOTH engines.

import { describe, it } from 'node:test'
import { strict as assert } from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  BoruError,
  canon,
  Engine,
  newInteger,
  newList,
  newTypeLiteral,
  TInteger,
  TList,
  newString,
  newWord,
  Registry,
  type Value,
} from '@boru-lang/core'
import { stackNatives } from './native-stack.ts'
import { controlNatives } from './native-control.ts'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const SPEC_DIR = path.resolve(__dirname, '..', '..', 'spec')

interface SpecRow {
  file: string
  line: number
  prog: string
  expected: string
  note: string
}

function readSpec(file: string): SpecRow[] {
  const text = fs.readFileSync(path.join(SPEC_DIR, file), 'utf8')
  const rows: SpecRow[] = []
  const lines = text.split('\n')
  for (let n = 0; n < lines.length; n++) {
    const line = lines[n]!
    // A line is a COMMENT only when it starts with '#' AND carries no tab —
    // '#' is boru's own comment marker, so a program may begin with one.
    if ('' === line.trim() || (line.startsWith('#') && !line.includes('\t'))) continue
    const parts = line.split('\t')
    assert.ok(parts.length >= 2, `${file}:${n + 1}: need at least 2 tab-separated columns`)
    rows.push({
      file,
      line: n + 1,
      prog: parts[0]!,
      expected: parts[1]!,
      note: parts[2] ?? '',
    })
  }
  return rows
}

/**
 * token builds one program token: a decimal is an Integer, '…' is a
 * String, anything else is a Word. Deliberately tiny — the corpus is about
 * the WORDS, so the token notation stays out of the way.
 */
function token(tok: string): Value {
  if (tok.startsWith("'") && tok.endsWith("'") && tok.length >= 2) {
    return newString(tok.slice(1, -1))
  }
  // A capitalised builtin name is that type's LITERAL, so a row can pass a
  // bare type where a word expects a concrete value — the refusal path is
  // otherwise unreachable from the corpus.
  if ('List' === tok) return newTypeLiteral(TList)
  if ('Integer' === tok) return newTypeLiteral(TInteger)
  if (/^-?\d+$/.test(tok)) return newInteger(BigInt(tok))
  return newWord(tok)
}

/**
 * program turns a row's token list into values, assembling bracketed runs
 * into LIST values. The corpus notation has no parser behind it, so
 * `do [ 1 2 ]` needs the brackets to become one list Value rather than
 * three tokens. Eval=true mirrors what the parser produces for a `[…]`
 * literal. Written independently of the Go runner's copy, per the
 * two-runner rule.
 */
function program(toks: string[]): Value[] {
  const out: Value[] = []
  for (let i = 0; i < toks.length; i++) {
    if ('[' !== toks[i]) {
      out.push(token(toks[i]!))
      continue
    }
    let depth = 1
    let j = i + 1
    for (; j < toks.length && depth > 0; j++) {
      if ('[' === toks[j]) depth++
      if (']' === toks[j]) depth--
    }
    assert.equal(depth, 0, `unbalanced [ in program ${toks.join(' ')}`)
    out.push(newList(program(toks.slice(i + 1, j - 1)), { eval: true }))
    i = j - 1
  }
  return out
}

/** Registers the ported vocabulary, and nothing else. */
function specRegistry(): Registry {
  const r = new Registry()
  for (const nf of stackNatives) r.registerNativeFunc(nf)
  for (const nf of controlNatives) r.registerNativeFunc(nf)
  return r
}

function runProgram(prog: string): string {
  const toks = program(prog.split(/\s+/).filter((s) => '' !== s))
  try {
    return canon(new Engine(specRegistry()).run(toks))
  } catch (e) {
    if (e instanceof BoruError) return `ERROR:${e.code}`
    return `ERROR:non_boru:${(e as Error).message}`
  }
}

// Every .tsv in basic/spec, discovered rather than listed — a new corpus
// file must not need a runner edit to be read, which is how a file gets
// silently skipped on one engine.
for (const file of fs.readdirSync(SPEC_DIR).filter((f) => f.endsWith('.tsv')).sort()) {
  describe(`basic spec — ${file}`, () => {
    const rows = readSpec(file)
    assert.ok(rows.length > 0, `${file}: no rows — the corpus is not being read`)
    for (const r of rows) {
      it(`${r.line}: ${r.prog}`, () => {
        assert.equal(runProgram(r.prog), r.expected, r.note)
      })
    }
  })
}
