// Build and pack the TypeScript core/parser packages, install only those
// tarballs into a clean consumer, and import them with plain Node. This catches
// raw-.ts entry points (Node will not strip types inside node_modules), missing
// runtime assets such as grammar.json, and undeclared package dependencies.

import { execFileSync } from 'node:child_process'
import {
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const temp = mkdtempSync(join(tmpdir(), 'boru-parser-package-'))

function run(command, args, cwd, options = {}) {
  return execFileSync(command, args, {
    cwd,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
    ...options,
  })
}

function pack(relativeDir) {
  const cwd = join(ROOT, relativeDir)
  const output = run(
    'npm',
    ['pack', '--silent', '--json', '--pack-destination', temp],
    cwd,
  )
  // npm 11 and earlier print an array of manifests; npm 12 prints an object
  // keyed by package name. One package is packed here, so take its only
  // manifest either way.
  const parsed = JSON.parse(output)
  const result = Array.isArray(parsed) ? parsed[0] : Object.values(parsed)[0]
  if (!result || typeof result.filename !== 'string' || !Array.isArray(result.files)) {
    throw new Error(`npm pack returned an invalid manifest for ${relativeDir}`)
  }
  return result
}

function assertPacked(result, required, forbidden) {
  const paths = new Set(result.files.map((file) => file.path))
  for (const path of required) {
    if (!paths.has(path)) {
      throw new Error(`${result.name}: packed tarball is missing ${path}`)
    }
  }
  for (const pattern of forbidden) {
    const found = [...paths].find((path) => pattern.test(path))
    if (found) {
      throw new Error(`${result.name}: packed tarball unexpectedly contains ${found}`)
    }
  }
}

try {
  const core = pack('core/ts')
  const parser = pack('parser/ts')

  assertPacked(
    core,
    ['dist/index.js', 'dist/index.d.ts'],
    [/\.test\.[cm]?[jt]s$/, /^dist\/.*(?<!\.d)\.ts$/],
  )
  assertPacked(
    parser,
    ['dist/index.js', 'dist/index.d.ts', 'dist/grammar.json'],
    [/\.test\.[cm]?[jt]s$/, /streamdump/, /^dist\/.*(?<!\.d)\.ts$/],
  )

  writeFileSync(
    join(temp, 'package.json'),
    JSON.stringify({ name: 'boru-parser-package-smoke', private: true, type: 'module' }),
  )
  run(
    'npm',
    [
      'install',
      '--ignore-scripts',
      '--no-audit',
      '--no-fund',
      `./${core.filename}`,
      `./${parser.filename}`,
    ],
    temp,
  )

  const smokeSource = `
    import { canon } from '@boru-lang/core'
    import {
      convertParsedNumber,
      lexTokens,
      parse,
      safeParseData,
    } from '@boru-lang/parser'

    const result = {
      parse: canon(parse('Box<(Integer)>')),
      number: canon([convertParsedNumber(safeParseData('1.e2'))]),
      lex: lexTokens('é🙂 1'),
    }
    process.stdout.write(JSON.stringify(result))
  `
  const actual = JSON.parse(run(process.execPath, ['--input-type=module', '--eval', smokeSource], temp))
  const expected = {
    // NUR059 gave the angle sugar a SOURCE form; this used to pin the
    // debug dump `sugar(angle Box [paren([word(Integer)])])`.
    parse: 'Box<(Integer)>',
    number: '100.0',
    lex: {
      tokens: [
        { name: '#TX', src: 'é🙂', si: 0 },
        { name: '#SP', src: ' ', si: 6 },
        { name: '#NR', src: '1', si: 7 },
      ],
      ok: true,
    },
  }
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(`clean package import mismatch:\nwant ${JSON.stringify(expected)}\n got ${JSON.stringify(actual)}`)
  }

  const parserPackage = JSON.parse(
    readFileSync(join(temp, 'node_modules', '@boru-lang', 'parser', 'package.json'), 'utf8'),
  )
  if (parserPackage.exports?.['.']?.import !== './dist/index.js') {
    throw new Error('installed parser package does not export dist/index.js')
  }

  writeFileSync(
    join(temp, 'consumer.ts'),
    `
      import { type Value } from '@boru-lang/core'
      import { lexTokens, parse, type LexTokensResult } from '@boru-lang/parser'

      const values: Value[] = parse('1 add 2')
      const tokens: LexTokensResult = lexTokens('1 add 2')
      void [values, tokens]
    `,
  )
  writeFileSync(
    join(temp, 'tsconfig.json'),
    JSON.stringify({
      compilerOptions: {
        target: 'ES2023',
        module: 'NodeNext',
        moduleResolution: 'NodeNext',
        strict: true,
        noEmit: true,
        skipLibCheck: false,
      },
      files: ['consumer.ts'],
    }),
  )
  run(
    process.execPath,
    [join(ROOT, 'parser', 'ts', 'node_modules', 'typescript', 'bin', 'tsc'), '-p', 'tsconfig.json'],
    temp,
  )

  console.log(
    `parser package smoke: PASS (core ${core.entryCount} files, parser ${parser.entryCount} files)`,
  )
} finally {
  rmSync(temp, { recursive: true, force: true })
}
