// Canon renders values as canonical boru source — the string a spec row's
// `expected` column is compared against. Ported from eng/go/canon.go +
// eng/go/value.go::FormatFloat so the TS engine's output matches the Go
// reference row-for-row.
//
// PARITY NOTE: this covers the value kinds the eng/spec corpus reaches
// through the current TS engine (none, type literals, scalars, atoms,
// lists, fn defs). Map / BigInteger / Reach / Flex / DepScalar branches
// are added by their owning port increments.
import { TAtom, TBoolean, TDispatchMod, TFloat, TInspect, TInteger, TList, TMap, TPathon, TString, TEmailon, TUrlon } from './type.ts'
import { Decimal } from './decimal.ts'
import { urlonHref, type UrlonInfo } from './value.ts'
import type { DispatchModInfo, FnDefInfo, XmlElement } from './value.ts'
import { ChildType, ErrorInfo, OptionsData, OrderedMap, Value, asReach, isReach } from './value.ts'

// canonXml renders an XML element, normalising an empty element to the
// self-closing form (<br></br> → <br/>).
// XML entity re-escaping for the canonical render — text and attribute
// values are stored DECODED (the parser runs unescapeXml), so rendering
// must re-escape. Mirrors eng/go/core_xml.go's xmlTextEscaper /
// xmlAttrEscaper exactly.
function escapeXmlText(s: string): string {
  return s.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
}

function escapeXmlAttr(s: string): string {
  return escapeXmlText(s).replaceAll('"', '&quot;')
}

export function canonXml(e: XmlElement): string {
  const attrs = e.attrs.map((a) => ` ${a.name}="${escapeXmlAttr(a.value)}"`).join('')
  if (e.children.length === 0) return `<${e.tag}${attrs}/>`
  const body = e.children
    .map((c) => (typeof c === 'string' ? escapeXmlText(c) : canonXml(c)))
    .join('')
  return `<${e.tag}${attrs}>${body}</${e.tag}>`
}

/**
 * canonString renders a string payload as parseable boru source. Plain
 * content uses single quotes; content with a single quote switches to
 * double quotes; content with both quote kinds, backslashes, or control
 * characters falls back to single quotes with backslash escapes.
 * Mirrors eng/go/canon.go::canonString.
 */
export function canonString(s: string): string {
  const hasSingle = s.includes("'")
  const hasEscape = /[\\\n\t\r]/.test(s)
  if (!hasSingle && !hasEscape) return `'${s}'`
  if (!s.includes('"') && !hasEscape) return `"${s}"`
  let b = "'"
  for (const r of s) {
    switch (r) {
      case '\\':
        b += '\\\\'
        break
      case "'":
        b += "\\'"
        break
      case '\n':
        b += '\\n'
        break
      case '\t':
        b += '\\t'
        break
      case '\r':
        b += '\\r'
        break
      default:
        b += r
    }
  }
  return b + "'"
}

/**
 * formatFloat renders a float with a guaranteed decimal point so it
 * stays visually distinct from Integer, using the shortest round-trip
 * representation. Mirrors eng/go/value.go::FormatFloat.
 */
export function formatFloat(f: number): string {
  if (Number.isNaN(f)) return 'nan'
  if (f === Infinity) return 'inf'
  if (f === -Infinity) return '-inf'
  // Negative zero is a distinct bit pattern (IEEE) and renders as -0.0.
  if (f === 0 && 1 / f === -Infinity) return '-0.0'

  if (f !== 0) {
    const a = Math.abs(f)
    if (a >= 1e21 || a < 1e-10) {
      return formatExponential(f)
    }
  }
  let s = toFixedShortest(f)
  if (!/[.eE]/.test(s)) s += '.0'
  return s
}

// toFixedShortest renders a float in plain decimal ('f') form with the
// shortest precision that round-trips — the equivalent of Go's
// strconv.FormatFloat(f, 'f', -1, 64). JS's String() uses 'g'-style
// formatting (switching to exponent for large/small values), so coerce
// any exponent form back to plain decimal for the everyday range that
// FormatFloat keeps non-scientific.
function toFixedShortest(f: number): string {
  const s = String(f)
  if (!/[eE]/.test(s)) return s
  // Expand the JS exponential form to plain decimal.
  return expandExponential(s)
}

// formatExponential renders the Go strconv 'e' form: a mantissa with a
// sign-and-at-least-two-digit exponent (e.g. 1e+21 → "1e+21").
function formatExponential(f: number): string {
  let s = f.toExponential()
  // JS gives "1e+21"; Go gives "1e+21" too. Normalise the exponent to a
  // leading sign (JS already includes it) — no zero-padding needed to
  // match Go's -1 precision 'e' form for the magnitudes the specs use.
  // Drop a trailing ".0"-style mantissa? Go keeps the shortest mantissa.
  s = s.replace(/e([+-])(\d)$/, 'e$10$2')
  return s
}

// expandExponential turns a JS exponential literal into plain decimal.
function expandExponential(s: string): string {
  const neg = s.startsWith('-')
  if (neg) s = s.slice(1)
  const m = /^(\d+)(?:\.(\d+))?[eE]([+-]?\d+)$/.exec(s)
  if (!m) return (neg ? '-' : '') + s
  const intPart = m[1]!
  const fracPart = m[2] ?? ''
  const exp = Number.parseInt(m[3]!, 10)
  const digits = intPart + fracPart
  const pointPos = intPart.length + exp
  // The decimal point always lands at or left of the digits here:
  // formatFloat routes |f| >= 1e21 through formatExponential, and below
  // that JS's String only goes exponential for |f| < 1e-6 (negative
  // exp). A positive pointPos would make the repeat throw — loudly.
  let out = '0.' + '0'.repeat(-pointPos) + digits
  // Trim a trailing fractional zero run produced by the expansion.
  if (out.includes('.')) out = out.replace(/0+$/, '').replace(/\.$/, '')
  return (neg ? '-' : '') + out
}

/**
 * formatBigInteger renders a BigInteger as the parseable literal `0d…`,
 * sign BEFORE the marker — Go's FormatBigInteger (core/go/value.go).
 * `-0d789`, not `0d-789`: the marker introduces the digits, so the sign
 * has to lead or the literal does not parse back.
 */
export function formatBigInteger(n: bigint): string {
  if (n < 0n) return '-0d' + (-n).toString()
  return '0d' + n.toString()
}

/**
 * formatBigDecimal renders a BigDecimal as the parseable literal `0d…`,
 * mirroring Go's FormatBigDecimal — apd's plain 'f' form with the sign
 * before the marker.
 *
 * Exact at every magnitude and scale since the payload became a Decimal
 * (decimal.ts): `0d1e400`, `0d1e-400` and `0d0.30` all round-trip, where
 * the previous binary64 payload rendered `0dInfinity`, `0d0` and `0d0.3`.
 */
export function formatBigDecimal(d: Decimal): string {
  const s = d.toString()
  return s.startsWith('-') ? '-0d' + s.slice(1) : '0d' + s
}

/** Canon renders a stack of values as canonical boru source. */
export function canon(stack: Value[]): string {
  return stack.map(canonValue).join(' ')
}

/** canonValue renders one value as canonical boru source. */
export function canonValue(v: Value): string {
  // The `none` value renders lowercase; a bare type literal (including
  // the `None` type) renders as its user-facing leaf name.
  if (v.isNone()) return 'none'
  if (v.data === null) {
    return v.vType.leaf()
  }
  // The `/r` / `/q` group modifier the parser emits AFTER a paren or
  // dotted-path group. Canon had no arm for it, so each engine fell through
  // to a DIFFERENT debug spelling — TS to `word(undefined)` (the word arm
  // reading a DispatchModInfo as a WordInfo) and Go to
  // `word()({false true})`. Neither is source, and they disagreed, which is
  // what the parity probe caught on `m.k/q`.
  if (v.vType.equal(TDispatchMod)) {
    return (v.data as DispatchModInfo).ref ? '/r' : '/q'
  }
  // A caught-error VALUE (the do escape hatch) — mirrors Go's render.
  if (v.data instanceof ErrorInfo) {
    return `error(${v.data.message})`
  }
  if (v.vType.matches(TInteger)) {
    return v.asInteger().toString()
  }
  if (v.vType.matches(TFloat)) {
    return formatFloat(v.asFloat())
  }
  if (v.vType.matches(TString)) {
    return canonString(v.asString())
  }
  if (v.vType.matches(TBoolean)) {
    return v.asBoolean() ? 'true' : 'false'
  }
  if (v.vType.equal(TAtom)) {
    return `${v.asAtom()}/q`
  }
  if (v.vType.equal(TPathon) && v.data !== null && typeof v.data === 'object' && 'segments' in v.data) {
    const p = v.data as { segments: string[]; abs: boolean }
    return (p.abs ? '/' : '') + p.segments.join('/')
  }
  if (v.vType.equal(TEmailon) && v.data !== null && typeof v.data === 'object' && 'user' in v.data) {
    const e = v.data as { user: string; host: string }
    return `${e.user}@${e.host}`
  }
  if (v.vType.equal(TUrlon) && v.data !== null && typeof v.data === 'object' && 'scheme' in v.data) {
    return urlonHref(v.data as UrlonInfo)
  }
  if (v.data instanceof ChildType) {
    const ct = v.data
    const open = v.isTypedMap() ? '{' : '['
    const close = v.isTypedMap() ? '}' : ']'
    // A typed MAP carries its concrete pairs in `entries`, not `elements`,
    // and canon rendered only the latter — so `{:Integer a:1}` came out
    // `{:Integer}` with the entries silently gone. Both are rendered here,
    // mirroring Value.toString and core/go/canon.go's ChildTypeInfo arm.
    const parts = [
      `:${canonTypeTag(ct.child)}`,
      ...ct.elements.map(canonValue),
      ...ct.entries.map((e) => `${e.key}:${canonChild(e.value)}`),
    ]
    return `${open}${parts.join(' ')}${close}`
  }
  if (v.vType.matches(TList) && Array.isArray(v.data)) {
    const body = `[${v.asList().map(canonChild).join(' ')}]`
    return v.quoted ? `(quote ${body})` : body
  }
  if (v.data instanceof OptionsData) {
    const m = v.data.map
    // D1: render in INSERTION order (design/FLEX-ATTRS.1.md §3), matching
    // Go's joinEntries which iterates Keys(). SortedKeys() stays for
    // order-insensitive equality only (coretype.ts valuesEqual).
    const parts = m.keys().map((k) => `${k}:${canonValue(m.get(k)!)}`)
    return `options{${parts.join(' ')}}`
  }
  if (v.vType.equal(TMap) && v.data instanceof OrderedMap) {
    const m = v.data
    const parts = m.keys().map((k) => `${k}:${canonChild(m.get(k)!)}`)
    return `{${parts.join(' ')}}`
  }
  // An inspection map renders in insertion order with bare word values
  // (e.g. `kind:native`), unlike a plain map.
  if (v.vType.equal(TInspect) && v.data instanceof OrderedMap) {
    const m = v.data
    const parts = m.keys().map((k) => {
      const val = m.get(k)!
      const rendered = val.isWord() ? val.asWord().name : canonValue(val)
      return `${k}:${rendered}`
    })
    return `{${parts.join(' ')}}`
  }
  if (v.isXml()) {
    return canonXml(v.data as XmlElement)
  }
  if (isReach(v)) {
    return canonReach(v)
  }
  if (v.isFnDef()) {
    return canonFnDef(v.asFnDef())
  }
  // Disjunct / Enum: alternatives joined by ' tor ' — the word form, so
  // the rendering re-reads as source (`tor` is commutative). Atoms render
  // bare, type literals as their leaf. A TOP-LEVEL union is left ungrouped
  // (it re-parses as-is); a union nested in a container is grouped by
  // canonChild so it does not fragment into extra tokens.
  if (v.isDisjunct()) {
    return v
      .asDisjunct()
      .alternatives.map((a) =>
        a.vType.equal(TAtom) && a.data !== null
          ? a.asAtom()
          : a.data === null
            ? a.vType.leaf()
            : canonValue(a),
      )
      .join(' tor ')
  }
  return v.toString()
}

// canonChild renders a value that sits INSIDE a composite canon (a map value,
// a list element, a record/childtype field). A union's `A tor B` renders with
// whitespace, so concatenated into a container it fragments into extra
// word-separated tokens (`{x:Integer tor String}` reads as broken map syntax)
// and breaks the source round-trip; grouping it in parens keeps the compound
// canon re-parseable (`{x:(Integer tor String)}`). A top-level union is left
// ungrouped by canonValue — it re-parses as-is and is a comparison ordering
// key. Mirrors eng/go/canon.go::canonChild.
function canonChild(v: Value): string {
  return v.isDisjunct() ? `(${canonValue(v)})` : canonValue(v)
}

// canonTypeTag renders a container's element-type tag in SOURCE form. The
// parser is type-name-opaque (ADR-012 rule 4), so the tag on an unevaluated
// literal is still a bare Word — `[:Integer]` holds word(Integer), not the
// Integer type node — and rendering it through the generic value path leaks
// the debug spelling `word(Integer)` into what is meant to be source.
// Mirrors core/go/canon.go canonTypeTag.
function canonTypeTag(v: Value): string {
  if (v.isWord()) return v.asWord().name
  return canonChild(v)
}

// canonFnDef renders a function value's discriminating canonical form.
// The TS FnDefInfo carries a single params/returns/body triple (the
// spec subset), so this renders that one signature. Mirrors the shape
// of eng/go/canon.go::canonFnDef for a single authored sig.
function canonFnDef(fd: FnDefInfo): string {
  const sigs = fd.sigs
    .map((s) => {
      const params = s.params.map((p) => `${p.name}:${p.type.toString()}`).join(' ')
      const returns = s.returns.map((r) => r.toString()).join(' ')
      const body = s.body.map(canonValue).join(' ')
      return `[${params}][${returns}][${body}]`
    })
    .join(' ')
  return `fn [${sigs}]`
}

// canonReach renders a Reach back to its dotted surface — m.a.b, m!.x,
// m.'k', m.(expr), (expr).k — the read-print round-trip, mirroring
// eng/go/canon.go::canonReach (words stay bare; a codequote-captured
// reach wraps so it round-trips).
function canonReachToken(v: Value): string {
  if (v.isWord()) return v.asWord().name
  if (v.isParenExpr() && Array.isArray(v.data)) return '(' + canonReachTokens(v.data as Value[]) + ')'
  if (isReach(v)) return canonReach(v)
  return canonValue(v)
}

function canonReachTokens(toks: Value[]): string {
  return toks.map(canonReachToken).join(' ')
}

function canonReach(v: Value): string {
  const info = asReach(v)
  if (info === undefined) return String(v)
  let b: string
  if (info.receiver.length === 0) b = '$'
  else if (info.receiver.length === 1) b = canonReachToken(info.receiver[0]!)
  else b = '(' + canonReachTokens(info.receiver) + ')'
  for (const seg of info.segments) {
    b += seg.getr ? '!.' : '.'
    if (seg.computed) b += '(' + canonReachTokens(seg.keyExpr ?? []) + ')'
    else b += canonReachToken(seg.keyLit!)
  }
  if (v.quoted) return '(codequote ' + b + ')'
  return b
}
