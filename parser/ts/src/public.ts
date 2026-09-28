// Host-facing parser helpers shared by the CLI, data decoders, and
// formatter front ends. These are the TypeScript twins of parser/go's
// config.go, safemake.go, and lex.go seams.

import {
  Jsonic,
  JsonicError,
  makeLex,
  type Jsonic as JsonicParser,
  type Options,
} from '@tabnas/jsonic'
import { keyOrder } from '@tabnas/parser'

import { loadDeclGrammar } from './declgrammar.ts'
import {
  setupBaseTokens,
  setupBigNumberMatcher,
  setupDataDecimalMatcher,
  setupDecimalUnderscoreMatcher,
  setupMiniLitMatcher,
  setupNumberSub,
  setupStringEscapeMatcher,
  setupTemplateLiteralMatcher,
} from './grammar.ts'
import { setupXmlMatcher } from './xml.ts'
import { codePointColumnAt } from './nodes.ts'

/** One raw, trivia-preserving token from the Boru-configured lexer. */
export interface LexToken {
  name: string
  src: string
  /** UTF-8 byte offset into the original source, matching Go's SI. */
  si: number
}

/** A formatter-friendly token stream and whether lexing reached clean EOF. */
export interface LexTokensResult {
  tokens: LexToken[]
  ok: boolean
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  if (null === value || 'object' !== typeof value) {
    return false
  }
  const prototype = Object.getPrototypeOf(value)
  return null === prototype || Object.prototype === prototype
}

/**
 * Recursively removes Jsonic's boxed-text and structural-info nodes.
 * Non-Jsonic class instances are left intact rather than silently losing
 * their identity.
 */
export function plainify(value: unknown): unknown {
  if (value instanceof String) {
    return value.valueOf()
  }
  if (Array.isArray(value)) {
    return value.map(plainify)
  }
  if (isPlainRecord(value)) {
    return Object.fromEntries(
      Object.entries(value).map(([key, child]) => [key, plainify(child)]),
    )
  }
  return value
}

/** Construct a fresh plain Jsonic parser. JavaScript construction is synchronous. */
export function safeMake(options?: Options | string): JsonicParser {
  // Go's stock text scanner treats configured string quotes as unconditional
  // text boundaries. The TS dependency exposes that behavior through `ender`,
  // so append every configured quote character while retaining caller enders. String
  // modes (`json` / `jsonic`) are dependency-native special constructors and
  // deliberately pass through unchanged.
  if ('string' === typeof options) {
    return Jsonic.make(options)
  }
  const supplied = options?.ender
  const enders = 'string' === typeof supplied
    ? Array.from(supplied)
    : [...(supplied ?? [])]
  const quoteChars = false === options?.string?.lex
    ? ''
    : (options?.string?.chars ?? "'\"`")
  for (const quote of quoteChars) {
    if (!enders.includes(quote)) enders.push(quote)
  }
  return Jsonic.make({ ...options, ender: enders })
}

// The TS lexer reports columns as UTF-16 offsets; Go's public helper reports
// Unicode code-point columns. Preserve the original JsonicError instance and
// normalize only its public location fields before it crosses this wrapper.
// Exported from this source module for the same reason as the converter's
// direct-call seams: tests must pin defensive arms a real Jsonic parser cannot
// produce. It is deliberately not re-exported from the package entry point.
export function normalizePlainParseError(error: unknown, src: string): unknown {
  if (!(error instanceof JsonicError)) {
    return error
  }
  const reported = error as unknown as { lineNumber: number; columnNumber: number }
  if (
    !Number.isInteger(reported.lineNumber) ||
    reported.lineNumber <= 0 ||
    !Number.isInteger(reported.columnNumber) ||
    reported.columnNumber <= 0
  ) {
    return error
  }
  const line = src.split(/\r\n|\n|\r/)[reported.lineNumber - 1]
  if (line !== undefined) {
    reported.columnNumber = codePointColumnAt(line, reported.columnNumber - 1)
  }
  return error
}

function parsePlain(jsonic: JsonicParser, src: string): unknown {
  try {
    return jsonic.parse(src)
  } catch (error) {
    throw normalizePlainParseError(error, src)
  }
}

/**
 * Parse relaxed-JSON/Jsonic source with a fresh plain parser. Errors remain
 * native JsonicError instances; parseConfig owns its stable outer diagnostic.
 */
export function safeParse(src: string): unknown {
  return parsePlain(safeMake(), src)
}

const CONFIG_SYNTAX_MESSAGE = 'invalid options syntax'

function configValueCategory(value: unknown): 'list' | 'string' | 'scalar' {
  if (Array.isArray(value)) return 'list'
  if ('string' === typeof value) return 'string'
  return 'scalar'
}

/**
 * Parse data while retaining each numeric token's exact spelling, so
 * convertParsedNumber can preserve integer/float kind and full precision.
 */
export function safeParseData(src: string): unknown {
  // `map.ordered` records each map's true insertion order on the decoded
  // node, recoverable with `dataKeyOrder` below. A plain JS object
  // enumerates integer-like keys ascending whatever the source said, so
  // without this the seam cannot express what Go's OrderedMap preserves:
  // `{2:9, 1:8}` is keys 2,1. Object.keys() order is untouched, so nothing
  // that ignores the side-channel changes behaviour.
  const jsonic = safeMake({ map: { ordered: true } })
  // The data-mode matcher claims only complete numeric tokens and never
  // splits: digit-led prose (`1.x`, `1.2.3`, `1.2-beta`) stays on the stock
  // lenient text path, preserving the jsonic-superset contract.
  setupDataDecimalMatcher(jsonic)
  setupNumberSub(jsonic, src)
  return parsePlain(jsonic, src)
}

/**
 * The insertion order of a decoded map's keys, in source order — the TS
 * twin of reading `*jsonic.OrderedMap.Keys` on the Go side. Falls back to
 * `Object.keys` order for a node parsed without `map.ordered`.
 */
export function dataKeyOrder(node: unknown): string[] {
  return keyOrder(node)
}

/** Parse a host options blob, requiring a map at the top level. */
export function parseConfig(src: string): Record<string, unknown> {
  let parsed: unknown
  try {
    parsed = safeParse(src)
  } catch (error) {
    // SafeParse is deliberately dependency-native. ParseConfig is the
    // narrower host seam, so its outer message must not vary with the Go/JS
    // jsonic renderer; callers can still inspect the original through cause.
    throw new Error(CONFIG_SYNTAX_MESSAGE, { cause: error })
  }

  const plain = plainify(parsed)
  if (!isPlainRecord(plain)) {
    throw new Error(
      'options must be a map of key:value pairs, got ' + configValueCategory(parsed),
    )
  }
  return plain
}

/**
 * Tokenise with the same base tokens and custom matchers as the Boru parser,
 * retaining whitespace, newlines, and comments for formatter consumers.
 */
export function lexTokens(src: string): LexTokensResult {
  const jsonic = safeMake({})
  const { t } = setupBaseTokens(jsonic, loadDeclGrammar())
  setupTemplateLiteralMatcher(jsonic, t)
  setupStringEscapeMatcher(jsonic)
  setupBigNumberMatcher(jsonic, t)
  // The same Boru decimal-boundary shim is installed in both ports so raw
  // formatter token streams agree even where their stock scanners differ.
  setupDecimalUnderscoreMatcher(jsonic, t)
  setupMiniLitMatcher(jsonic, t)
  setupXmlMatcher(jsonic, t)

  // Lex accepts the parser context shape structurally at runtime. A bare
  // scan needs only source, resolved config, and an empty subscriber set.
  const lex = makeLex({ src: () => src, cfg: jsonic.config(), sub: {} } as never)
  const tokens: LexToken[] = []
  let byteOffset = 0

  for (;;) {
    // Context-only matchers deliberately decline when no grammar rule is
    // active, just as parser/go's flat Lex.Next loop does.
    const token = lex.next(undefined as never)
    if ('#ZZ' === token.name) {
      return { tokens, ok: true }
    }
    if ('#BD' === token.name) {
      return { tokens, ok: false }
    }
    tokens.push({
      name: token.name,
      src: token.src,
      // The trivia-preserving stream is contiguous, so advancing by each
      // raw token avoids repeatedly encoding the entire source prefix.
      si: byteOffset,
    })
    byteOffset += Buffer.byteLength(token.src, 'utf8')
  }
}
