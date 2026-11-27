// Spec-driven interpreter tests, mirroring borueng/go/spec_test.go. Reads the
// shared eng/spec/*.tsv corpus and runs each row through the TS interpreter.
// Fixtures + tokenizer + corpus loader live in ./spec-fixture.ts (shared with
// the cross-engine differential and the fixture probes).

import { describe, it } from "node:test"
import { strict as assert } from "node:assert"
import * as fs from "node:fs"
import * as path from "node:path"

import { BoruError, Engine, Registry, Value } from "./index.ts"
import { SPEC_DIR, parseSpec, registerSpecWords, renderStack, tokenize, type SpecRow } from "./spec-fixture.ts"

function runRow(row: SpecRow): { ok: true; got: string } | { ok: false; err: string } {
  const r = new Registry()
  registerSpecWords(r)

  let values: Value[]
  try {
    values = tokenize(row.input)
  } catch (e) {
    return { ok: false, err: `tokenize: ${(e as Error).message}` }
  }

  try {
    const result = new Engine(r).run(values)
    return { ok: true, got: renderStack(result) }
  } catch (e) {
    if (e instanceof BoruError) {
      return { ok: false, err: e.message }
    }
    return { ok: false, err: `unexpected: ${(e as Error).message}` }
  }
}

const specFiles = fs
  .readdirSync(SPEC_DIR)
  .filter((f) => f.endsWith('.tsv'))
  .sort()

describe('spec', () => {
  for (const file of specFiles) {
    const fullPath = path.join(SPEC_DIR, file)
    const rows = parseSpec(fullPath)
    describe(file.replace(/\.tsv$/, ''), () => {
      for (const row of rows) {
        const name = `L${row.lineNum} ${row.input}`
        it(name, () => {
          const result = runRow(row)
          if (row.expected.startsWith('ERROR:')) {
            const want = row.expected.slice('ERROR:'.length)
            assert.ok(!result.ok, `expected error containing ${JSON.stringify(want)}`)
            if (!result.ok && want !== '') {
              assert.ok(
                result.err.toLowerCase().includes(want.toLowerCase()),
                `error ${JSON.stringify(result.err)} does not contain ${JSON.stringify(want)}`,
              )
            }
            return
          }
          assert.ok(result.ok, result.ok ? '' : `unexpected error: ${result.err}`)
          if (result.ok) {
            assert.equal(result.got, row.expected)
          }
        })
      }
    })
  }
})
