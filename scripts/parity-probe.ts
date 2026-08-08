// The TS half of scripts/parity-probe.sh: render each candidate source
// exactly as parser/spec's runner does, one line per input.
//
// Deliberately reuses renderSpec's contract (canon, or 'ERR ' and the first
// line of the error) rather than inventing a second one, so a probe result
// can be pasted straight into parse.tsv.
import * as fs from 'node:fs'
import { canon } from '../core/ts/src/index.ts'
import { parse } from '../parser/ts/src/index.ts'

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

function encode(s: string): string {
  return s.replaceAll('\\', '\\\\').replaceAll('\n', '\\n').replaceAll('\t', '\\t')
}

const file = process.argv[2]
if (undefined === file) {
  console.error('usage: parity-probe.ts <file-of-sources>')
  process.exit(1)
}

for (const line of fs.readFileSync(file, 'utf8').split('\n')) {
  if ('' === line) continue
  const src = decodeSpecEscapes(line)
  let out: string
  try {
    out = canon(parse(src))
  } catch (e) {
    const msg = e instanceof Error ? (e.message.split('\n')[0] ?? '') : String(e)
    out = 'ERR ' + msg
  }
  console.log(encode(out))
}
