// This file translates tabnas/jsonic lexer-and-grammar failures into
// boru-voice BoruErrors (design/DIAGNOSTICS.0.md, phase 2) — the TS twin
// of parser/go/parse_error.go. The library's own
// rendering is disabled at the source (errMsgOptions / colorOff below
// kill the always-on ANSI palette, the `--internal:` suffix block, and
// the library docs link), and every JsonicError is rebuilt as an
// [boru/syntax_error] carrying the same position, a boru-phrased detail
// line, and a plain-language note — so parse failures render through the
// shared diagnostic renderer exactly like every other boru error, on
// every surface (CLI, REPL, LSP, wasm).

import { JsonicError } from '@tabnas/jsonic'

import { BoruError } from '@boru-lang/core'
import { normalizeReportedPos } from './nodes.ts'

// colorOff pins the library renderer's color off; the translated
// BoruError owns presentation (color is caller-resolved there).
// (Go: parseColorOff — grammar.ts inlines the same setting in its
// make() options; exported here for parity and reuse.)
export const colorOff = false

// errMsgOptions brands and quiets the library error format for the
// rare failure that escapes translation untyped.
// (Go: parseErrMsgOptions — grammar.ts inlines the same setting.)
export const errMsgOptions = { name: 'boru', suffix: false }

// translateParseError rebuilds a jsonic parse failure as a boru
// syntax_error at the same source position. Errors of any other type
// keep the historical `parse error: %w`-style wrapping.
export function translateParseError(e: unknown, src: string): BoruError {
  if (e instanceof BoruError) {
    return e
  }
  if (!(e instanceof JsonicError)) {
    const msg = e instanceof Error ? e.message : String(e)
    return new BoruError('syntax_error', 'parse error: ' + msg, '')
  }
  // The library assigns code/lineNumber/… dynamically (errdesc), so the
  // declared TabnasError type carries none of them — go via unknown.
  const te = e as unknown as JsonicErrorFields
  const tokenSrc = jsonicErrSrc(te)
  const [detail, note, help] = parseErrText(te)
  const pos = normalizeReportedPos(src, {
    row: te.lineNumber,
    col: te.columnNumber,
    src: tokenSrc,
  })
  return new BoruError('syntax_error', detail, tokenSrc, {
    row: pos.row,
    col: pos.col,
    src: tokenSrc,
    fullSource: src,
    notes: '' === note ? [] : [note],
    suggestions: '' === help ? [] : [{ message: help }],
  })
}

// The JsonicError fields the translation reads (assigned onto the
// error by the library's errdesc). The Go library exposes the offending
// token's source as te.Src and the detail line as te.Detail; the TS
// library exposes neither directly — the rendered texts ride on the
// txts() accessor and the token src must be recovered from the message
// template (see jsonicErrSrc).
interface JsonicErrorFields {
  code: string
  message: string
  lineNumber: number
  columnNumber: number
  details: Record<string, unknown>
  txts?: () => { msg?: string; hint?: string; site?: string }
}

// jsonicErrDetail returns the library's own detail line (Go: te.Detail)
// — the errinjected message template without the `[name/code]:` prefix.
function jsonicErrDetail(te: JsonicErrorFields): string {
  const msg = te.txts?.().msg
  if ('string' === typeof msg) {
    return msg
  }
  // Fallback: strip the `[boru/<code>]: ` prefix off the first line.
  // split() always returns at least one element, including for an empty
  // string; the assertion avoids inventing an unreachable fallback branch.
  const first = te.message.split('\n')[0]!
  return first.replace(/^\[[^\]]*\]:\s*/, '')
}

// The library's error message templates (defaults.js `error`), used to
// recover the injected {src} — the offending token's source text, Go's
// te.Src field, which the TS JsonicError does not expose directly.
const errTemplatePrefix: Record<string, string> = {
  unexpected: 'unexpected character(s): ',
  invalid_unicode: 'invalid unicode escape: ',
  invalid_ascii: 'invalid ascii escape: ',
  unprintable: 'unprintable character: ',
  unterminated_string: 'unterminated string: ',
  unterminated_comment: 'unterminated comment: ',
}

// jsonicErrSrc recovers the offending token's source text (Go: te.Src)
// from the detail line's known template prefix; codes whose template
// carries no {src} (end_of_source, unknown codes) yield ''.
function jsonicErrSrc(te: JsonicErrorFields): string {
  const prefix = errTemplatePrefix[te.code]
  if (undefined === prefix) {
    return ''
  }
  const detail = jsonicErrDetail(te)
  if (detail.startsWith(prefix)) {
    return detail.slice(prefix.length)
  }
  return ''
}

// parseErrText maps a tabnas error code to the boru-voice detail line,
// an explanatory note, and an actionable help suggestion (either may be
// empty). The code set is the library's errorMessages table; unknown
// codes keep the library detail with an honest parser-internal note.
export function parseErrText(te: JsonicErrorFields): [detail: string, note: string, help: string] {
  const q = (s: string): string => '`' + s + '`'
  const src = jsonicErrSrc(te)
  switch (te.code) {
    case 'unexpected':
      return [
        'unexpected ' + q(src) + ' — nothing valid can appear here',
        "boru's grammar allows no continuation with " + q(src) + ' at this position',
        'check for a missing bracket, quote, or value just before it',
      ]
    case 'empty_child':
      return [
        'empty list element: remove the leading/repeated comma (write `none` for an explicit empty value)',
        "a typed list's `:` child needs a value after the colon",
        'write the element type after the colon, e.g. `[:Integer]`',
      ]
    case 'arrow_no_body':
      return [
        '`=>` has no body',
        "a lambda's body follows the arrow: nothing does here",
        'write the body after the arrow, e.g. `x => [ x mul 2 ]`',
      ]
    case 'unterminated_string':
      return ['this string is never closed: ' + src, '', 'add the closing quote']
    case 'unterminated_comment':
      return ['this comment is never closed', '', 'close the comment before the end of the source']
    case 'end_of_source':
      return [
        'the source ends in the middle of an expression',
        'an unclosed `[` list, `{` map, `(` group, or string earlier is the usual cause',
        '',
      ]
    case 'invalid_unicode':
      return [
        'invalid unicode escape: ' + src,
        'the escape sequence ' + q(src) + ' does not encode a valid unicode code point',
        '',
      ]
    case 'invalid_ascii':
      return [
        'invalid ascii escape: ' + src,
        'the escape sequence ' + q(src) + ' does not encode a valid ASCII character',
        '',
      ]
    case 'unprintable':
      return [
        'unprintable character inside a string',
        'a raw control character (code point below 32) is not allowed in a string literal',
        'use an escape sequence instead',
      ]
  }
  // unknown_rule / internal / unknown / future codes: the failure is in
  // the parser, not the program — keep the library detail and say so.
  return [
    jsonicErrDetail(te),
    "this is a parser-internal failure — likely a bug in boru's grammar, not in your program",
    '',
  ]
}
