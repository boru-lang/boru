// Embedded XML literals — the TS twin of parser/go/xml_literal.go
// (the REFERENCE; port function-by-function). The xml_literal lex
// matcher, when armed by the xml rule's BO (rule.k["boru_xml"]), scans
// the whole balanced `<tag …>…</tag>` element and emits ONE #XML token
// carrying an XmlElemVal (nodes.ts) — the built Node/Xml value, the
// unresolved template when ${…} holes are present, or the build failure.
// See design/XML-LITERAL.0.md.

/* eslint-disable @typescript-eslint/no-explicit-any */

import type { Value, XmlTmpl, XmlElement, InterpSegment, XmlChildTmpl } from '@boru-lang/core'
import { newXml, newXmlInterp } from '@boru-lang/core'
import { XmlElemVal } from './nodes.ts'
import type { ParserTokens } from './grammar.ts'
import { parse } from './convert.ts'

// --- local grammar helpers -------------------------------------------
//
// grammar.ts keeps its setOpen/setClose/setBO/addMatcher idiom helpers
// module-private; these are the same minimal re-creations of the Go
// field-assignment idioms, duplicated here so this file stays
// self-contained (grammar.ts is separately owned).

function setOpen(rs: any, alts: any[]): void {
  rs.clearOpen()
  rs.open(alts, { append: true })
}

function setClose(rs: any, alts: any[]): void {
  rs.clearClose()
  rs.close(alts, { append: true })
}

function setBO(rs: any, actions: any[]): void {
  rs.clearActions('bo')
  for (const a of actions) {
    rs.bo(a)
  }
}

function addMatcher(
  j: any,
  name: string,
  priority: number,
  fn: (lex: any, rule: any) => any,
): void {
  j.options({ lex: { match: { [name]: { order: priority, make: () => fn } } } })
}

// ---------------------------------------------------------------------

// setupXmlGrammar wires the embedded XML literal rule. The val.Open `<`
// alternate (setupValRule) pushes "xml"; this rule's BO arms the
// xml_literal matcher, whose single #XML token carries the whole
// element. Generics' angle sugar consumes `<` only as a val.Close
// suffix after a capitalised name, so the two never compete for `<`.
// See design/XML-LITERAL.0.md §3.
export function setupXmlGrammar(j: any, t: ParserTokens): void {
  j.rule('xml', (rs: any) => {
    setBO(rs, [
      (r: any) => {
        // Arm the matcher for the next token (the element body).
        r.k['boru_xml'] = true
      },
    ])
    setOpen(rs, [
      {
        s: [t.XML],
        a: (r: any) => {
          // Disarm immediately: the lookahead token the Close
          // alternate lexes must not scan a following sibling
          // `<…>` as part of this element.
          delete r.k['boru_xml']
          r.node = r.o0.val
        },
      },
    ])
    setClose(rs, [
      // The #XML token already consumed the entire element; close
      // without consuming the lookahead (the dotchain technique).
      { b: 1 },
    ])
  })
}

// setupXmlMatcher registers the lex matcher that, when armed by the xml
// rule's BO (rule.k["boru_xml"]), scans the whole balanced element that
// follows the already-consumed `<` and emits one #XML token carrying the
// built Node/Xml value. It runs ONLY when armed, so it never touches the
// generics `<` (which is lexed outside the xml rule) or any other `<`.
export function setupXmlMatcher(j: any, _t: ParserTokens): void {
  addMatcher(j, 'xml_literal', 1000003, (lex: any, rule: any) => {
    if (!rule) {
      return undefined
    }
    if (true !== rule.k?.['boru_xml']) {
      return undefined
    }
    const cursor = lex.pnt
    const s: string = lex.src
    const afterLA: number = cursor.sI // position just past the `<` the val.Open alt consumed
    const built = buildXmlTmpl(s, afterLA)
    const tmpl = built[0]
    let end = built[1]
    const err = built[2]
    if (undefined !== err) {
      // Absorb the rest of the source so leftover tokens don't
      // trigger a confusing jsonic error before conversion surfaces
      // this (more specific) one.
      end = s.length
    }
    if (end <= afterLA) {
      // No progress (`<` at end of source): decline and let normal
      // lexing report the unexpected character.
      return undefined
    }
    // No interpolation anywhere → freeze to a constant Node/Xml (the
    // Increment-1 path). Otherwise emit the skeleton; the engine
    // evaluates it to a Node/Xml at runtime. See evalXmlInterp.
    let v: Value | undefined
    let holes: XmlTmpl | undefined
    if (undefined === err && undefined !== tmpl) {
      const cv = freezeXmlTmpl(tmpl)
      if (undefined !== cv) {
        v = newXml(cv)
      } else {
        v = newXmlInterp(tmpl)
        holes = tmpl
      }
    }
    let la = afterLA - 1
    if (la < 0) {
      la = 0
    }
    const raw = s.slice(la, end)
    const tkn = lex.token('#XML', new XmlElemVal(v, err, holes), raw, cursor)
    for (let k = afterLA; k < end; k++) {
      if ('\n' === s[k]) {
        cursor.rI++
        cursor.cI = 1
      } else {
        cursor.cI++
      }
    }
    cursor.sI = end
    return tkn
  })
}

function isXmlNameStart(c: string): boolean {
  return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || '_' === c
}

function isXmlNameChar(c: string): boolean {
  return isXmlNameStart(c) || (c >= '0' && c <= '9') || '-' === c || '.' === c || ':' === c
}

function isXmlSpace(c: string): boolean {
  return ' ' === c || '\t' === c || '\n' === c || '\r' === c
}

// buildXmlTmpl parses one element from s starting at i (the index just
// after the element's opening `<`) and returns its skeleton (XmlTmpl), the
// index just past the element's final `>`, and any error. The skeleton
// captures literal text and `${expr}` holes uniformly; the matcher then
// either freezes a hole-free skeleton to a constant Node/Xml or emits it
// for runtime evaluation (see freezeXmlTmpl / evalXmlInterp). On a
// well-formedness error it returns a usable stop index (> i where possible)
// so the matcher can still advance the cursor and surface the error through
// conversion. When the very first character is not a tag name, it returns
// end == i so the matcher declines.
function buildXmlTmpl(s: string, i: number): [XmlTmpl | undefined, number, string | undefined] {
  const n = s.length
  if (i >= n || !isXmlNameStart(s[i]!)) {
    return [undefined, i, "xml: expected a tag name after '<'"]
  }
  const nameStart = i
  while (i < n && isXmlNameChar(s[i]!)) {
    i++
  }
  const t: XmlTmpl = { tag: s.slice(nameStart, i), attrs: [], children: [] }
  const seenAttr: Record<string, boolean> = {}

  // Opening-tag attributes, up to `>` or `/>`.
  for (;;) {
    while (i < n && isXmlSpace(s[i]!)) {
      i++
    }
    if (i >= n) {
      return [undefined, n, `xml: unterminated opening tag <${t.tag}>`]
    }
    if ('/' === s[i]) {
      if (i + 1 < n && '>' === s[i + 1]) {
        return [t, i + 2, undefined]
      }
      return [undefined, i + 1, `xml: expected '/>' to close <${t.tag}>`]
    }
    if ('>' === s[i]) {
      i++
      break
    }
    if (!isXmlNameStart(s[i]!)) {
      return [undefined, i + 1, `xml: invalid attribute name in <${t.tag}>`]
    }
    const anStart = i
    while (i < n && isXmlNameChar(s[i]!)) {
      i++
    }
    const aname = s.slice(anStart, i)
    if (true === seenAttr[aname]) {
      return [undefined, i, `xml: duplicate attribute "${aname}" in <${t.tag}>`]
    }
    seenAttr[aname] = true
    while (i < n && isXmlSpace(s[i]!)) {
      i++
    }
    // Default (valueless) attribute → empty string, like strict XML.
    let parts: InterpSegment[] = [{ lit: '' }]
    if (i < n && '=' === s[i]) {
      i++
      while (i < n && isXmlSpace(s[i]!)) {
        i++
      }
      if (i < n && '$' === s[i] && i + 1 < n && '{' === s[i + 1]) {
        // Bare interpolation as the whole value: `name=${expr}`.
        const [toks, end, err] = scanInterp(s, i)
        if (undefined !== err) {
          return [undefined, end, err]
        }
        parts = [{ expr: toks! }]
        i = end
      } else if (i < n && ('"' === s[i] || "'" === s[i])) {
        const [vparts, end, err] = scanAttrValue(s, i + 1, s[i]!, aname, t.tag)
        if (undefined !== err) {
          return [undefined, end, err]
        }
        parts = vparts!
        i = end
      } else {
        return [undefined, i, `xml: attribute "${aname}" in <${t.tag}> must have a quoted value`]
      }
    }
    t.attrs.push({ name: aname, segs: parts })
  }

  // Element content, up to the matching `</tag>`.
  let text = ''
  const flush = (): void => {
    if (text.length > 0) {
      t.children.push({ lit: unescapeXml(text) })
      text = ''
    }
  }
  for (;;) {
    if (i >= n) {
      return [undefined, n, `xml: unterminated element <${t.tag}>`]
    }
    // ${expr} interpolation hole in content (child position).
    if ('$' === s[i] && i + 1 < n && '{' === s[i + 1]) {
      flush()
      const [toks, end, err] = scanInterp(s, i)
      if (undefined !== err) {
        return [undefined, end, err]
      }
      t.children.push({ expr: toks! })
      i = end
      continue
    }
    if ('<' !== s[i]) {
      text += s[i]
      i++
      continue
    }
    // s[i] == '<'
    if (i + 1 < n && '/' === s[i + 1]) {
      flush()
      let jj = i + 2
      while (jj < n && isXmlSpace(s[jj]!)) {
        jj++
      }
      const cnStart = jj
      while (jj < n && isXmlNameChar(s[jj]!)) {
        jj++
      }
      const cname = s.slice(cnStart, jj)
      while (jj < n && isXmlSpace(s[jj]!)) {
        jj++
      }
      if (jj >= n || '>' !== s[jj]) {
        let stop = jj + 1
        if (stop > n) {
          stop = n
        }
        return [undefined, stop, `xml: malformed closing tag for <${t.tag}>`]
      }
      if (cname !== t.tag) {
        return [undefined, jj + 1, `xml: mismatched closing tag </${cname}> for <${t.tag}>`]
      }
      return [t, jj + 1, undefined]
    }
    if (i + 3 < n && '!' === s[i + 1] && '-' === s[i + 2] && '-' === s[i + 3]) {
      flush()
      let k = i + 4
      while (k + 2 < n && !('-' === s[k] && '-' === s[k + 1] && '>' === s[k + 2])) {
        k++
      }
      if (k + 2 >= n) {
        return [undefined, n, `xml: unterminated comment in <${t.tag}>`]
      }
      i = k + 3
      continue
    }
    // Child element.
    flush()
    const [child, ni, err] = buildXmlTmpl(s, i + 1)
    if (undefined !== err) {
      return [undefined, ni, err]
    }
    t.children.push({ elem: child! })
    i = ni
  }
}

// scanAttrValue parses a quoted attribute value beginning at i (just past
// the opening quote q) into InterpSegments — literal segments plus `${expr}`
// holes — returning the index just past the closing quote.
function scanAttrValue(
  s: string,
  i: number,
  q: string,
  aname: string,
  tag: string,
): [InterpSegment[] | undefined, number, string | undefined] {
  const n = s.length
  let parts: InterpSegment[] = []
  let lit = ''
  const flush = (): void => {
    if (lit.length > 0) {
      parts.push({ lit: unescapeXml(lit) })
      lit = ''
    }
  }
  while (i < n) {
    if (s[i] === q) {
      flush()
      if (0 === parts.length) {
        parts = [{ lit: '' }]
      }
      return [parts, i + 1, undefined]
    }
    if ('$' === s[i] && i + 1 < n && '{' === s[i + 1]) {
      flush()
      const [toks, end, err] = scanInterp(s, i)
      if (undefined !== err) {
        return [undefined, end, err]
      }
      parts.push({ expr: toks! })
      i = end
      continue
    }
    lit += s[i]
    i++
  }
  return [undefined, n, `xml: unterminated value for attribute "${aname}" in <${tag}>`]
}

// scanInterp consumes a `${ … }` interpolation beginning at i (where
// s[i:i+2] == "${"), returning the parsed expression tokens, the index
// just past the closing `}`, and any error. The matching brace is found by
// depth counting that skips over quoted strings (', ", `), so a `}` inside
// a string does not close the hole early.
function scanInterp(s: string, i: number): [Value[] | undefined, number, string | undefined] {
  const n = s.length
  let j = i + 2
  let depth = 1
  while (j < n) {
    switch (s[j]) {
      case "'":
      case '"':
      case '`': {
        const q = s[j]
        j++
        while (j < n) {
          if ('\\' === s[j] && j + 1 < n) {
            j += 2
            continue
          }
          if (s[j] === q) {
            j++
            break
          }
          j++
        }
        break
      }
      case '{':
        depth++
        j++
        break
      case '}':
        depth--
        if (0 === depth) {
          const inner = s.slice(i + 2, j).trim()
          let toks: Value[]
          try {
            toks = parse(inner)
          } catch (e) {
            return [
              undefined,
              j + 1,
              'xml: in ${...}: ' + (e instanceof Error ? e.message : String(e)),
            ]
          }
          return [toks, j + 1, undefined]
        }
        j++
        break
      default:
        j++
    }
  }
  return [undefined, n, 'xml: unterminated ${...} interpolation']
}

// freezeXmlTmpl returns a constant XmlElement for a skeleton with no
// `${expr}` holes anywhere, or undefined when any hole is present — in
// which case the matcher emits the skeleton for runtime evaluation. The
// frozen value is byte-identical to the Increment-1 constant build.
// (Go returns (eng.Value, bool) built via NewOrderedMap/NewString; the
// TS value layer's newXml takes the plain XmlElement shape directly, so
// this returns the element and the matcher wraps the top level.)
function freezeXmlTmpl(t: XmlTmpl): XmlElement | undefined {
  const attrs: { name: string; value: string }[] = []
  for (const a of t.attrs) {
    const s = staticParts(a.segs)
    if (undefined === s) {
      return undefined
    }
    attrs.push({ name: a.name, value: s })
  }
  const children: (string | XmlElement)[] = []
  for (const c of t.children as XmlChildTmpl[]) {
    if ('lit' in c) {
      children.push(c.lit)
    } else if ('elem' in c) {
      const cv = freezeXmlTmpl(c.elem)
      if (undefined === cv) {
        return undefined
      }
      children.push(cv)
    } else {
      // expr hole
      return undefined
    }
  }
  return { tag: t.tag, attrs, children }
}

// staticParts concatenates InterpSegments when none is an expression hole.
//
// An EMPTY hole — `${}` or `${   }` — is not a hole. Go's test is
// `p.Expr != nil`, and `Parse("")` yields a nil token slice, so an empty
// attribute interpolation folds to the empty string and the element
// freezes to a constant. That is an artifact of Go's nil/empty
// conflation rather than a designed semantics — the proof is that Go's
// CHILD path returns false for XmlCrenExpr unconditionally, so `<a>${}
// </a>` stays an interp there while `<a b=${}/>` does not. Mirrored
// anyway: parity with the shipped language is the contract, and
// "improving" one side is the divergence this apparatus exists to catch.
// Pinned by parser/spec/parse.tsv rows for both shapes.
function staticParts(parts: InterpSegment[]): string | undefined {
  let b = ''
  for (const p of parts) {
    if ('expr' in p && p.expr.length > 0) {
      return undefined
    }
    if ('lit' in p) {
      b += p.lit
    }
  }
  return b
}

// xmlUnescaper's entity table — checked in argument order at each `&`,
// single pass over the input (the TS re-creation of Go's
// strings.NewReplacer semantics: no re-scanning of replaced output, so
// `&amp;lt;` unescapes to `&lt;`, not `<`).
const xmlEntities: [string, string][] = [
  ['&lt;', '<'],
  ['&gt;', '>'],
  ['&quot;', '"'],
  ['&apos;', "'"],
  ['&amp;', '&'],
]

function unescapeXml(s: string): string {
  if (!s.includes('&')) {
    return s
  }
  let out = ''
  let i = 0
  scan: while (i < s.length) {
    if ('&' === s[i]) {
      for (const [ent, rep] of xmlEntities) {
        if (s.startsWith(ent, i)) {
          out += rep
          i += ent.length
          continue scan
        }
      }
    }
    out += s[i]
    i++
  }
  return out
}
