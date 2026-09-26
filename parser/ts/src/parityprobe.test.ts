import { strict as assert } from 'node:assert'
import { execFileSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { describe, it } from 'node:test'
import { fileURLToPath } from 'node:url'

describe('standalone parity probe', () => {
  it('renders with the parser package\'s core identity', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'boru-parity-probe-'))
    try {
      const input = path.join(dir, 'sources.txt')
      fs.writeFileSync(input, '[a]:a\n{a:1}:A\n')
      const script = path.resolve(
        path.dirname(fileURLToPath(import.meta.url)),
        '..',
        '..',
        '..',
        'scripts',
        'parity-probe.ts',
      )
      const rendered = execFileSync(
        process.execPath,
        ['--experimental-strip-types', '--no-warnings', script, input],
        { encoding: 'utf8' },
      )
      assert.equal(rendered, '[:a [a]]\n[:A {a:1}]\n')
    } finally {
      fs.rmSync(dir, { recursive: true, force: true })
    }
  })
})
