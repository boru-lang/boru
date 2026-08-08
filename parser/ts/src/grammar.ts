// The boru grammar on @tabnas/jsonic — the TS twin of
// parser/go/grammar.go (the REFERENCE; port function-by-function).
// makeBoruJsonic() builds the configured jsonic instance with every boru
// token, matcher, and rule extension installed (stages 1-2 of Go Parse),
// returning the instance plus the registered token ids.
//
// The tabnas jsonic engine exposes RuleSpec alternates and actions through
// accessor methods (open/close with ListMods, clearOpen, clearActions, bo,
// bc, ac, …) rather than the exported Open/Close/BO/BC slice fields the
// legacy jsonicjs parser used. These helpers re-create the field-assignment
// idioms the boru grammar is written against, so the rule definitions read
// the same as the Go source:
//
//	setOpen(rs, alts)      — replace the open alternates       (was rs.Open = alts)
//	prependOpen(rs, alts)  — prepend before existing opens     (was rs.Open = append(alts, rs.Open...))
//	rs.open([alt], {append:true}) — append one open alternate  (was rs.Open = append(rs.Open, alt))
//	setBO(rs, actions)     — replace the before-open actions   (was rs.BO = actions)
//
// (…and the Close / BC / AC equivalents.)

/* eslint-disable @typescript-eslint/no-explicit-any */

import { Jsonic } from '@tabnas/jsonic'
import { BoruError } from '@boru-lang/core'

import {
  type DeclGrammar,
  type DeclHooks,
  applyDeclEdits,
  fixedTokenOptions,
  loadDeclGrammar,
  resolveDeclTokens,
} from './declgrammar.ts'
import {
  AngleGroup,
  ArrowTag,
  InterpGroup,
  IexprGroup,
  MiniLitVal,
  NumberVal,
  ParenGroup,
  Sited,
  UnclosedAngle,
  UnclosedParen,
  type SrcPos,
} from './nodes.ts'
import { setupXmlMatcher, setupXmlGrammar } from './xml.ts'
// The escape processor and the word-modifier scan live in convert.ts (the
// parse.go twin), exported for this file — the Go originals sit in
// parse.go beside the other converters but are called from grammar.go's
// template-literal matcher and shorthand-key action. The import cycle
// (convert → grammar → convert) is benign: both functions are hoisted and
// only called at parse time.
import { processTemplateEscapes, scanWordModifier } from './convert.ts'

export interface ParserTokens {
  OP: number // (
  CP: number // )
  DT: number // .
  SC: number // ;
  QM: number // ?
  BG: number // !
  PI: number // |
  AR: number // => (lambda arrow → aliases the word `afn`)
  BT: number // ` (backtick)
  IS: number // ${ (interp start)
  TL: number // template literal segment
  LA: number // < (angle open — general-purpose token, D14)
  RA: number // > (angle close)
  ML: number // minilang literal: +name<delim>src<delim> (matcher-produced)
  XML: number // embedded XML literal <tag>…</tag> (matcher-produced)
}

// The info-marker property name for jsonic metadata nodes: boxed String
// text (quote info), map/list nodes (implicit flag + meta bag). We never
// change cfg.info.marker from its default, so the constant is safe to
// use everywhere (verified against @tabnas/parser defaults).
const INFO = '__info__'

// newText builds the TS analogue of Go's jsonic.Text{Str, Quote}: a boxed
// String carrying the quote kind on the hidden info marker ('' = unquoted
// word, "'"/'"' = quoted string, 'tl' = template-literal segment).
function newText(str: string, quote = ''): String {
  const sv = new String(str)
  Object.defineProperty(sv, INFO, { value: { quote }, writable: true })
  return sv
}

// asText reads a jsonic Text node — the TS analogue of Go's
// `n.(jsonic.Text)` type assertion. Only boxed Strings qualify (primitive
// strings are not Text nodes, exactly as Go's assertion fails on non-Text).
function asText(v: unknown): { str: string; quote: string } | undefined {
  if (v instanceof String) {
    const info = (v as any)[INFO] as { quote?: string } | undefined
    return { str: v.valueOf(), quote: info?.quote ?? '' }
  }
  return undefined
}

// mapInfoOf reads the hidden info marker of a jsonic map node — the TS
// analogue of Go's `r.Node.(jsonic.MapRef)` assertion. With info.map on,
// every map node carries `__info__ = { implicit, meta }`; the meta bag is
// a reference type, so Meta mutations need no MapRef reassignment dance.
function mapInfoOf(v: unknown): { implicit?: boolean; meta: Record<string, any> } | undefined {
  if (null === v || 'object' !== typeof v || Array.isArray(v) || v instanceof String) {
    return undefined
  }
  const info = (v as any)[INFO]
  if (info && 'object' === typeof info && info.meta) {
    return info as { implicit?: boolean; meta: Record<string, any> }
  }
  return undefined
}

function setOpen(rs: any, alts: any[]): void {
  rs.clearOpen()
  rs.open(alts, { append: true })
}

function setClose(rs: any, alts: any[]): void {
  rs.clearClose()
  rs.close(alts, { append: true })
}

function prependOpen(rs: any, alts: any[]): void {
  rs.open(alts) // prepend is the tabnas TS default
}

function prependClose(rs: any, alts: any[]): void {
  rs.close(alts) // prepend is the tabnas TS default
}

function setBO(rs: any, actions: any[]): void {
  rs.clearActions('bo')
  for (const a of actions) {
    rs.bo(a)
  }
}

function setBC(rs: any, actions: any[]): void {
  rs.clearActions('bc')
  for (const a of actions) {
    rs.bc(a)
  }
}

function setAC(rs: any, actions: any[]): void {
  rs.clearActions('ac')
  for (const a of actions) {
    rs.ac(a)
  }
}

// addMatcher re-creates jsonicjs's AddMatcher(name, priority, fn): it
// registers a priority-ordered custom lex matcher. The tabnas lexer
// interleaves lex.match entries by order against the built-in matchers
// using the SAME bands (match=1e6, fixed=2e6, space=3e6, …), so the boru
// matchers keep their exact relative ordering. Registration goes through
// options() — the config is rebuilt on every options() call, so direct
// config mutation would be discarded.
function addMatcher(
  j: any,
  name: string,
  priority: number,
  fn: (lex: any, rule: any) => any,
): void {
  j.options({ lex: { match: { [name]: { order: priority, make: () => fn } } } })
}

// makeBoruJsonic constructs the jsonic instance the boru parser runs on:
// the mapped SafeMake options (info/list/map/value plus the errmsg/color
// silencing from parse_error.go — the library's own error rendering is
// disabled at the source: parse failures are translated into boru
// syntax_errors, so the always-on ANSI palette, the `--internal:` suffix,
// and the library docs link must never reach a user), then the stage-1 lex
// setup and stage-2 grammar setup in the same order as Go Parse.
export function makeBoruJsonic(): { j: any; t: ParserTokens } {
  const j = Jsonic.make({
    // tabnas groups the output-shaping flags under info: text wraps
    // scalars in boxed Strings (quoted/unquoted distinction), list/map
    // attach the structural-metadata info marker to list and map nodes.
    // These were the top-level TextInfo/ListRef/MapRef flags in the
    // legacy jsonicjs.
    info: { text: true, list: true, map: true },
    // (list.property stays at its default of true; the TS options type
    // requires the full list object, unlike Go's pointer-partial one.)
    list: { property: true, pair: true, child: true },
    map: { child: true },
    value: { lex: false },
    // parseErrMsgOptions: brand and quiet the library error format for
    // the rare failure that escapes translation untyped.
    errmsg: { name: 'boru', suffix: false },
    // parseColorOff: pin the library renderer's color off; the translated
    // BoruError owns presentation.
    color: { active: false },
  })

  // Stage 1: Lex setup — register tokens and custom matchers, the
  // token table coming from the declarative grammar artifact.
  const g = loadDeclGrammar()
  const { t, tins } = setupBaseTokens(j, g)
  setupTemplateLiteralMatcher(j, t)
  setupBigNumberMatcher(j, t)
  setupMiniLitMatcher(j, t)
  setupXmlMatcher(j, t)

  // Stage 2: Grammar setup — the declarative edits first (they were
  // the head of the val amendments), then the remaining imperative
  // rule extensions, batch-migrating into grammar.json.
  applyDeclEdits(j, g, tins, valDeclHooks(), {
    prependOpen,
    prependClose,
    setOpen,
    setClose,
    newText: (str: string) => newText(str),
  })
  setupValRule(j, t)
  setupPairGrammar(j, t)
  setupParenGrammar(j, t)
  setupAngleGrammar(j, t)
  setupInterpGrammar(j, t)
  setupXmlGrammar(j, t)
  setupNumberSub(j)

  return { j, t }
}

// setupBaseTokens registers the fixed boru tokens and removes backtick from
// jsonic's string/multi chars so template strings are handled by custom rules.
export function setupBaseTokens(j: any, g: DeclGrammar): { t: ParserTokens; tins: Record<string, number> } {
  // Remove backtick from string chars so jsonic doesn't consume backtick
  // strings with the built-in string matcher. Template strings are handled
  // by custom tokens and rules below. (options() rather than config
  // mutation: the TS config is rebuilt on every options() call.) The
  // token table itself — fixed tokens with their source text, matcher-
  // produced tokens (#TL/#ML/#XML) by name — comes from the declarative
  // grammar artifact, the shared source with the Go twin.
  j.options({
    string: { chars: '\'"', multiChars: '' },
    fixed: { token: fixedTokenOptions(g) },
  })

  const tins = resolveDeclTokens(j, g)
  const t: ParserTokens = {
    OP: tins['OP']!, CP: tins['CP']!, DT: tins['DT']!, SC: tins['SC']!,
    QM: tins['QM']!, BG: tins['BG']!, PI: tins['PI']!, AR: tins['AR']!,
    BT: tins['BT']!, IS: tins['IS']!, TL: tins['TL']!, LA: tins['LA']!,
    RA: tins['RA']!, ML: tins['ML']!, XML: tins['XML']!,
  }
  return { t, tins }
}

// valDeclHooks binds the named behavior the declarative val edits
// reference — the twin of Go's valDeclHooks.
function valDeclHooks(): DeclHooks {
  return {
    actions: {
      minilangNode: (r: any) => {
        r.node = r.o0.val
      },
      arrowTag: (r: any) => {
        r.node = new ArrowTag()
      },
    },
    conds: {
      inPairValue: (r: any, ctx: any): boolean => {
        return r.parent && r.parent !== ctx.NORULE && 'pair' === r.parent.name
      },
    },
  }
}

// setupBigNumberMatcher registers a high-priority lex matcher that claims a
// whole `0d…` literal — `[+-]?0[dD][0-9_]+(\.[0-9_]+)?([eE][+-]?[0-9_]+)?` —
// as a single #BD token BEFORE jsonic's number matcher or the `.` (#DT)
// fixed token can split it (jsonic does not know the `0d` prefix, and would
// otherwise break `0d12.5` into `0d12` · `.` · `5`). The token carries a
// bigNumberVal; the val rule (setupValRule) turns it into r.node, and
// convertTopLevelValueInner / convertDataValue route it to bigNumberToValue.
// A trailing `.` is consumed ONLY when followed by a digit, so `0d5.foo`
// stays `0d5` + dot-access. Runs only outside template strings.
export function setupBigNumberMatcher(j: any, _t: ParserTokens): void {
  const isDigit = (c: string | undefined): boolean =>
    undefined !== c && ((c >= '0' && c <= '9') || '_' === c)
  addMatcher(j, 'big_number', 1000001, (lex: any, rule: any) => {
    if (rule && rule.k && rule.k['boru_tpl']) {
      return undefined // inside a template string: leave to the literal matcher
    }
    const cursor = lex.pnt
    const s: string = lex.src
    let si: number = cursor.sI
    const start = si
    if (si < s.length && ('+' === s[si] || '-' === s[si])) {
      si++ // optional sign, glued to the literal
    }
    // Require the 0d / 0D prefix.
    if (si + 1 >= s.length || '0' !== s[si] || ('d' !== s[si + 1] && 'D' !== s[si + 1])) {
      return undefined
    }
    si += 2
    // Require at least one integer digit.
    if (si >= s.length || !isDigit(s[si])) {
      return undefined
    }
    while (si < s.length && isDigit(s[si])) {
      si++
    }
    // Optional fraction — consume `.` only when a digit follows, so a
    // dot-access (`0d5.foo`) is left for the #DT token.
    if (si + 1 < s.length && '.' === s[si] && isDigit(s[si + 1])) {
      si++
      while (si < s.length && isDigit(s[si])) {
        si++
      }
    }
    // Optional exponent.
    if (si < s.length && ('e' === s[si] || 'E' === s[si])) {
      let j2 = si + 1
      if (j2 < s.length && ('+' === s[j2] || '-' === s[j2])) {
        j2++
      }
      if (j2 < s.length && isDigit(s[j2])) {
        si = j2
        while (si < s.length && isDigit(s[si])) {
          si++
        }
      }
    }
    const src = s.slice(start, si)
    // Emit the whole run as ONE text token. jsonic's val rule already
    // opens on text, so it flows to parseWord, which recognises the
    // `0d` prefix and builds the BigInteger/BigDecimal. The matcher's
    // only job is to claim the run so the embedded `.` isn't split off.
    const tkn = lex.token('#TX', src, src, cursor)
    cursor.sI = si
    cursor.cI += si - start // a 0d literal never spans newlines
    return tkn
  })
}

// setupMiniLitMatcher registers a high-priority lex matcher for the minilang
// shortcut `+name<delim>src<delim>` — terse sugar for `mini name 'src'`. The
// delimiter is the first character after the (lowercase) name; the SAME
// character closes. Backslash escapes the delimiter, itself, and whitespace
// (`\<delim>` → the delimiter, `\\` → `\`, `\<space>`/`\<tab>` → the space);
// every OTHER backslash is preserved literally, so regex sources keep their
// escapes raw (`+re/\d+/` → src `\d+`, no doubling).
//
// A literal is a SINGLE whitespace-free span — its source never contains
// unescaped whitespace. Within the span the first unescaped delimiter closes
// it (the closed form, which can abut syntax: `+re/x/).fst`). The closing
// delimiter is OPTIONAL when the source does not contain the delimiter char:
// with no delimiter before the first unescaped whitespace (or end of source),
// the span IS the source — the open form, `+email:alice@example.com` ≡
// `mini email 'alice@example.com'`. There is no scan-ahead past whitespace,
// so a stray delimiter char in a later token (the `:` in `{limit:2}` or
// `{limit: 2}`) can never capture the literal, and a literal never spans
// lines. A source that needs whitespace uses the quoted `mini` form or
// escapes (`+hb/de\ ad/`); hb/bb sources can group with `_` instead
// (`+hb/de_ad_be_ef/`).
//
// The match emits a single #ML token carrying a MiniLitVal; the val rule turns
// it into r.node and convertTopLevelValueInner expands it to the mini splice,
// so all the normal mini machinery (stack subject, a trailing opts Map, the
// expansion-time unknown-kind error, check mode) takes over unchanged. Runs
// only outside template strings; a `+` not followed by a name + non-space
// delimiter + a source char is left to normal lexing.
export function setupMiniLitMatcher(j: any, _t: ParserTokens): void {
  const isNameStart = (c: string | undefined): boolean =>
    undefined !== c && c >= 'a' && c <= 'z'
  const isNameChar = (c: string | undefined): boolean =>
    undefined !== c && ((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || '-' === c)
  const isSpace = (c: string | undefined): boolean =>
    ' ' === c || '\t' === c || '\n' === c || '\r' === c
  addMatcher(j, 'minilang_literal', 1000002, (lex: any, rule: any) => {
    if (rule && rule.k && rule.k['boru_tpl']) {
      return undefined // inside a template string: not a minilang literal
    }
    const cursor = lex.pnt
    const s: string = lex.src
    const start: number = cursor.sI
    let si = start
    if (si >= s.length || '+' !== s[si]) {
      return undefined
    }
    si++
    // Name: a lowercase kind atom. (A digit after `+` — e.g. `+0d5` — is
    // not a name start, so signed bignums fall through to big_number.)
    if (si >= s.length || !isNameStart(s[si])) {
      return undefined
    }
    const nameStart = si
    while (si < s.length && isNameChar(s[si])) {
      si++
    }
    const name = s.slice(nameStart, si)
    // Delimiter: the first char after the name. Must exist and not be
    // whitespace (a space means this was not a minilang literal).
    if (si >= s.length || isSpace(s[si])) {
      return undefined
    }
    const delim = s[si]
    si++
    // Single-span scan: a literal never contains unescaped whitespace.
    // The first unescaped delimiter closes it (so a closed literal can
    // abut syntax: `+re/x/).fst`, `+hb/de_ad/).typeof`); unescaped
    // whitespace before any delimiter ends the literal OPEN — no
    // scan-ahead, so a stray delimiter char in a later token (`{limit:2}`
    // or `{limit: 2}`) can never capture it. Escapes: `\<delim>`,
    // `\\`, `\<space>`, `\<tab>` produce the escaped char; every other
    // backslash is preserved raw.
    let b = ''
    let closed = false
    while (si < s.length) {
      const c = s[si]
      if (isSpace(c)) {
        break // open form: the literal ends at unescaped whitespace
      }
      if ('\\' === c && si + 1 < s.length) {
        const nxt = s[si + 1]
        if (nxt === delim || '\\' === nxt || ' ' === nxt || '\t' === nxt) {
          b += nxt // \<delim> → delim ; \\ → \ ; \<ws> → ws
          si += 2
          continue
        }
        b += '\\' // any other escape is preserved raw (e.g. \d)
        si++
        continue
      }
      if (c === delim) {
        closed = true
        si++
        break
      }
      b += c
      si++
    }
    if (!closed && 0 === b.length) {
      return undefined // an empty open source (`+name:` before whitespace) is not a literal
    }
    const tkn = lex.token('#ML', new MiniLitVal(name, b), s.slice(start, si), cursor)
    cursor.sI = si
    // The span can never contain a newline (the scan breaks at any
    // unescaped whitespace and only escape-consumes space/tab), so
    // the cursor stays on this row and advances one column per rune.
    cursor.cI += Array.from(s.slice(start, si)).length
    return tkn
  })
}

// setupTemplateLiteralMatcher registers a high-priority lex matcher that
// produces #TL tokens for literal text inside template strings. Active only
// when rule.k["boru_tpl"] is set (inside a backtick-opened interp rule).
export function setupTemplateLiteralMatcher(j: any, _t: ParserTokens): void {
  addMatcher(j, 'template_literal', 1000000, (lex: any, rule: any) => {
    if (!rule) {
      return undefined
    }
    if (!rule.k || !rule.k['boru_tpl']) {
      return undefined
    }
    const cursor = lex.pnt
    let si: number = cursor.sI
    const s: string = lex.src
    if (si >= s.length) {
      return undefined
    }
    // Don't match if at ` or ${ — let fixed token matcher handle those.
    if ('`' === s[si]) {
      return undefined
    }
    if ('$' === s[si] && si + 1 < s.length && '{' === s[si + 1]) {
      return undefined
    }
    // Scan forward collecting literal text until ` or ${ or end.
    const start = si
    while (si < s.length) {
      if ('`' === s[si]) {
        break
      }
      if ('$' === s[si] && si + 1 < s.length && '{' === s[si + 1]) {
        break
      }
      // Process escape sequences in template literals.
      if ('\\' === s[si] && si + 1 < s.length) {
        si += 2
        continue
      }
      si++
    }
    // si > start always: the entry guards returned on end-of-source,
    // backtick, and `${`, so the first iteration cannot break and
    // every arm advances si.
    let lit = s.slice(start, si)
    // Process escape sequences.
    lit = processTemplateEscapes(lit)
    const tkn = lex.token('#TL', lit, s.slice(start, si), cursor)
    cursor.sI = si
    // Update row/col tracking.
    for (const ch of s.slice(start, si)) {
      if ('\n' === ch) {
        cursor.rI++
        cursor.cI = 1
      } else {
        cursor.cI++
      }
    }
    return tkn
  })
}

// setupValRule extends the jsonic "val" rule with boru-specific alternates:
// parens, template strings, close-paren markers, semicolons, ?, !, |, and dots.
export function setupValRule(j: any, t: ParserTokens): void {
  // The value-open marker batch and the map-pair-value dot-chain
  // closes now live in the declarative grammar artifact
  // (grammar.json, applied by applyDeclEdits before this function
  // runs); their named hooks are valDeclHooks.

  // Dot-chain continuation. After a value closes, a following `.` (or
  // `!.`) means member access — `bf.n`, `a.b.c`, `a!.b`. In WORD
  // context (top level, lists, parens) the dot tokens are already
  // folded into `get`/`getr` calls by convertTopLevelItems, because
  // those contexts collect a flat item sequence. But a MAP VALUE is a
  // single `val`: once `bf` is parsed jsonic has no rule for the
  // trailing `.`, so `{n: bf.n}` errored with `unexpected character`.
  //
  // This Close alternate folds the receiver value plus the trailing
  // dot-chain into a parenGroup — exactly the shape the explicit
  // workaround `{n: (bf.n)}` produces — so the existing converter
  // paths (parenGroup → ParenExpr in data context, ( … ) markers in
  // word context) handle it uniformly with no converter change.
  // Prepended so it is tried before jsonic's default "there's more"
  // backtrack, which is what produced the error.
  // Gate strictly on map-pair-value position: the val whose Close sees
  // the dot is the value side of a `key:` pair exactly when its parent
  // rule is "pair" (in word context — top level, list elements, paren
  // contents — the parent is "elem"/"pelem" and the dot tokens are
  // already collected as a flat item sequence and folded into get/getr
  // by convertTopLevelItems; firing the dotchain there would
  // double-wrap, e.g. `1 foo.bar` → `1 ((foo get bar))`). Only the
  // map-value position lacks that flat-item path and needs this fold.

  // dotchain rule: collects the `.key` / `!.key` segments that follow a
  // value, folding the receiver (the parent val's already-parsed node)
  // and the segments into a parenGroup. Pushed from the val.Close
  // alternates above with the pointer backtracked onto the first `.` or
  // `!`. The resulting parenGroup is written back to the parent val so
  // downstream conversion treats `bf.n` exactly like the explicit
  // `(bf.n)`.
  //
  // jsonic alternates match at most two tokens, so the chain is
  // consumed one token at a time: a `.` or `!` marker, then the member
  // key, looping via R (replace-sibling) until a non-chain token ends
  // it. Each matched token is appended to the parent val's parenGroup.
  //
  // Each dotchain rule consumes exactly one `. key` (or `! . key`)
  // segment, then loops via R only when ANOTHER `.`/`!` immediately
  // follows. Crucially it must NOT treat a following bare key as a
  // chain segment — in a map, `{a: bf.n b: 9}` has `b` as the next
  // pair's key, not part of `bf.n` — so the loop is gated strictly on
  // a leading `.`/`!`, never on a key alone.
  j.rule('dotchain', (rs: any) => {
    setBO(rs, [
      (r: any, ctx: any) => {
        // On the first dotchain rule for this value, seed the
        // parent val's node as a parenGroup holding the receiver.
        // Subsequent looped dotchains see a parenGroup already.
        if (!r.parent || r.parent === ctx.NORULE) {
          return
        }
        if (r.parent.node instanceof ParenGroup) {
          return
        }
        let recv = r.parent.node
        if (recv instanceof Sited) {
          recv = recv.node
        }
        r.parent.node = new ParenGroup([recv])
      },
    ])
    // Mirror the accumulated chain (built into the PARENT val's node by the
    // segment actions) into THIS rule's own node. tabnas's val coalescer
    // (`@val-bc`, re-run after a close-pushed child completes) sets the
    // val's value from r.child.node — so the folded parenGroup must live on
    // the dotchain's own node, not only on the parent's, to survive. Each
    // looped dotchain (the val's current child) refreshes its node to the
    // running parenGroup; the final one carries the complete chain.
    setBC(rs, [
      (r: any, ctx: any) => {
        if (!r.parent || r.parent === ctx.NORULE) {
          return
        }
        if (r.parent.node instanceof ParenGroup) {
          r.node = r.parent.node
        }
      },
    ])
    setOpen(rs, [
      // `. key` — plain member access: append "." then the key.
      { s: [t.DT, '#KEY'], a: dotchainSeg('.') },
      // `!` — strict (getr) marker; the following `. key` is taken
      // by the next loop iteration. `!` only appears mid-chain
      // before a dot, so emitting it alone here is safe.
      { s: [t.BG], a: dotchainMarker('!') },
    ])
    setClose(rs, [
      // One segment per push: end and backtrack so the enclosing val
      // re-closes. A following `.`/`!` re-fires the val.Close dotchain
      // alt, pushing a FRESH dotchain that accumulates onto the same
      // parent parenGroup. (R-looping within this rule would leave the
      // val's r.child pointing at the first dotchain, whose node was
      // snapshotted at one segment — tabnas's val coalescer then drops
      // the rest of the chain. Re-pushing per segment keeps r.child on
      // the latest dotchain, which carries the full chain.)
      { b: 1 },
    ])
  })

  // Arrow fold. `SIG => BODY` groups as `(SIG afn BODY)` — the arrow
  // binds TIGHTER than any enclosing word's forward collection, so
  // `def double x:Integer => [x mul 2]` is by construction the same
  // program as `def double (x:Integer => [x mul 2])`. Without the
  // fold, the flat token run `SIG afn BODY` races the enclosing word:
  // def's Any slot claims the SIG literal at plan time, binding the
  // name to the sig map/list while the lambda evaporates as an
  // orphaned anonymous value (the parenless forms previously "worked"
  // exactly once by feeding that orphan).
  //
  // Mirrors the dotchain fold above: the val whose Close sees the
  // arrow pushes "arrowfold", which seeds the val's node as
  // parenGroup{SIG, afn}, consumes the arrow, parses ONE following
  // val as the body, and appends it. Gates:
  //
  //   - parent elem / pelem — word contexts (top-level items, list
  //     elements, paren contents), where the flat run is what races;
  //   - parent arrowfold — the BODY of an enclosing fold, so
  //     `x:Integer => y:Integer => [x add y]` curries
  //     right-associatively: (x ⇒ (y ⇒ body));
  //   - NOT a list-pair value (parent elem with U["pair"]) — `[k:v]`
  //     forms keep their flat shape;
  //   - NOT when the previous sibling is a `.`/`!` marker — a flat
  //     dot-chain (`m.k => body`) is folded into `(m get k)` by
  //     convertTopLevelItems, and folding here would capture only the
  //     final key as the sig.
  //
  // Everywhere else (map-pair values, template exprs, degenerate
  // leading arrows) the val.Open #AR alternate still produces the
  // bare `afn` word, exactly as before.
  const arrowFoldable = (r: any, ctx: any): boolean => {
    const p = r.parent
    if (!p || p === ctx.NORULE) {
      return false
    }
    switch (p.name) {
      case 'pelem':
      case 'arrowfold':
      case 'arrowfoldelem':
        // ok — paren contents and fold bodies (the latter make
        // `x => y => body` curry right-associatively).
        break
      case 'elem': {
        // A list-PAIR's value val must not fold (the sig is the
        // whole pair, which only exists on the elem — see the
        // elem-level fold below). Plain elems fold here.
        if (true === p.u['pair']) {
          return false
        }
        break
      }
      default:
        return false
    }
    if (r.prev && r.prev !== ctx.NORULE) {
      let prev = r.prev.node
      if (prev instanceof Sited) {
        prev = prev.node
      }
      const txt = asText(prev)
      if (txt && ('.' === txt.str || '!' === txt.str)) {
        return false
      }
    }
    return true
  }
  j.rule('val', (rs: any) => {
    prependClose(rs, [{ s: [t.AR], c: arrowFoldable, p: 'arrowfold', b: 1 }])
  })

  j.rule('arrowfold', (rs: any) => {
    setBO(rs, [
      (r: any, ctx: any) => {
        // Seed the parent val's node: the already-parsed value
        // becomes the sig half of (SIG afn BODY). deSite the
        // receiver — the parent val's BC re-sites the whole group.
        if (!r.parent || r.parent === ctx.NORULE) {
          return
        }
        let recv = r.parent.node
        if (recv instanceof Sited) {
          recv = recv.node
        }
        r.parent.node = new ParenGroup([recv, new ArrowTag()])
      },
    ])
    setBC(rs, [
      (r: any, ctx: any) => {
        // Append the body val to the parent's parenGroup.
        if (!r.parent || r.parent === ctx.NORULE) {
          return
        }
        if (undefined === r.child.node) {
          return
        }
        if (r.parent.node instanceof ParenGroup) {
          const grp = r.parent.node
          grp.items.push(r.child.node)
          r.parent.node = grp
          // Mirror the assembled group onto this rule's own node:
          // tabnas's val coalescer (`@val-bc`, re-run after this
          // close-pushed rule completes) sets the parent val's value
          // from r.child.node, so the (SIG afn BODY) group must live
          // here, not only on the parent. (Same reason as dotchain.)
          r.node = grp
        }
      },
    ])
    setOpen(rs, [
      // Consume the arrow, parse exactly one following val as the body.
      { s: [t.AR], p: 'val' },
    ])
    setClose(rs, [
      // Single shot — chaining is the BODY val's own fold (see the
      // arrowfold parent gate above). Backtrack so the enclosing
      // context consumes the next token.
      { b: 1 },
    ])
  })

  // Elem-level arrow fold. A bare pair OUTSIDE parens — `def double
  // x:Integer => [x mul 2]` at top level, or inside an explicit list —
  // parses via the elem rule's list-pair alternate, NOT via the val
  // dive: the value val sees the arrow first (its fold is gated off
  // above — the sig is the whole pair, not the value), and by the time
  // elem.Close examines the arrow, the elem's BC has already appended
  // the assembled one-entry pair map as the list node's LAST element.
  // So the elem-level fold pops that element, seeds (PAIR afn …), and
  // appends the folded group back. Gated on U["pair"] — a plain elem's
  // val folds at the val level, and a flat dot-chain that declined the
  // val fold must not be re-folded here.
  const elemPairArrow = (r: any): boolean => {
    return true === r.u['pair']
  }
  j.rule('elem', (rs: any) => {
    prependClose(rs, [
      {
        s: [t.AR],
        c: elemPairArrow,
        p: 'arrowfoldelem',
        b: 1,
        a: (r: any) => {
          // The elem's BC runs AGAIN when the elem finally
          // closes after the pushed fold returns; clear the
          // pair flag so that second pass doesn't rebuild a
          // pair from the fold rule's node (the first pass
          // already appended the real pair, which the fold's
          // BO pops as the sig).
          r.u['pair'] = false
        },
      },
    ])
  })

  j.rule('arrowfoldelem', (rs: any) => {
    setBO(rs, [
      (r: any, ctx: any) => {
        // Pop the just-appended pair map off the parent elem's
        // list node and hold it as the sig half. The elem and its
        // list share the array, so propagate the shortened node
        // to both. (Go type-switches ListRef vs []any; in TS both
        // arrive as arrays — in-place truncation keeps the hidden
        // info marker and the shared references consistent.)
        if (!r.parent || r.parent === ctx.NORULE) {
          return
        }
        const n = r.parent.node
        if (Array.isArray(n)) {
          if (0 === n.length) {
            return
          }
          r.u['af_sig'] = n[n.length - 1]
          n.length = n.length - 1
          r.parent.node = n
          if (r.parent.parent && r.parent.parent !== ctx.NORULE) {
            r.parent.parent.node = n
          }
        }
      },
    ])
    setBC(rs, [
      (r: any, ctx: any) => {
        // Append (SIG afn BODY) where the bare pair sat.
        const sig = r.u['af_sig']
        if (undefined === sig || !r.parent || r.parent === ctx.NORULE) {
          return
        }
        const body = r.child.node
        if (undefined === body) {
          return
        }
        const group = new ParenGroup([sig, new ArrowTag(), body])
        const n = r.parent.node
        if (Array.isArray(n)) {
          n.push(group)
          r.parent.node = n
          if (r.parent.parent && r.parent.parent !== ctx.NORULE) {
            r.parent.parent.node = n
          }
        }
      },
    ])
    setOpen(rs, [{ s: [t.AR], p: 'val' }])
    setClose(rs, [{ b: 1 }])
  })

  // Capture source position: wrap every value node with the row/col of its
  // opening token, so the converter can stamp eng.Value.Pos for precise
  // error reporting (mirrors aontu's addsite). This runs in the AFTER-CLOSE
  // (AC) phase, NOT before-close: the tabnas jsonic grammar installs a
  // `@val-ac` hook that re-resolves a primitive value from its token after
  // BC, which would otherwise overwrite the sited wrapper for every value
  // except the first. Running in AC — after `@val-ac` — makes the wrapper
  // the final node for every value. The Open alternates above have already
  // set r.node (including the marker Texts), so markers carry positions too.
  // The wrapper is unwrapped (deSite) at every node-type switch in
  // convert.ts and never leaks.
  j.rule('val', (rs: any) => {
    rs.ac((r: any) => {
      // TS-engine accommodation: tabnas's `@val-bc/replace` coalescer
      // stashes the value a plugin set in a val OPEN action as
      // r.u.openval, then re-resolves the matched token for any OBJECT
      // plugin value; its own `@val-ac` restore re-instates PRIMITIVES
      // only. boru's marker Texts are boxed Strings and the arrow tag is
      // a class instance — both objects — so they were dropped to
      // undefined. Restore exactly those marker shapes here (a stale
      // parent-seeded container never has these types, and a pushed
      // child's value still wins via the child gate). The Go engine's
      // coalescer keeps jsonic.Text plugin values, so this branch has no
      // Go counterpart.
      const ov = r.u['openval']
      if (
        null != ov &&
        undefined === r.child.node &&
        (ov instanceof String || ov instanceof ArrowTag)
      ) {
        r.node = ov
      }
      if (null === r.node || undefined === r.node) {
        return
      }
      if (r.node instanceof Sited) {
        return
      }
      let pos: SrcPos = { row: 0, col: 0 }
      const o0 = r.o0
      if (o0 && -1 !== o0.tin) {
        pos = { row: o0.rI, col: o0.cI, src: o0.src }
      }
      r.node = new Sited(r.node, pos)
    })
  })
}

// dotchainAppendTo appends node(s) to the parent val's parenGroup (the
// chain accumulator seeded in the dotchain BO). Shared tail of the
// dotchain Open actions.
function dotchainAppendTo(r: any, ctx: any, ...nodes: unknown[]): void {
  if (!r.parent || r.parent === ctx.NORULE) {
    return
  }
  if (r.parent.node instanceof ParenGroup) {
    r.parent.node.items.push(...nodes)
  }
}

// dotchainMarker returns an AltAction that appends a single marker Text
// (e.g. the "!" of a strict member access) to the chain.
function dotchainMarker(marker: string): (r: any, ctx: any) => void {
  return (r: any, ctx: any) => {
    dotchainAppendTo(r, ctx, newText(marker))
  }
}

// dotchainSeg returns an AltAction for a `<marker> key` segment: it
// appends the marker Text (the "." of member access, o0) followed by the
// member key token (o1), preserving the key's quoted/unquoted
// distinction so an unquoted key stays a word and a quoted key a string.
function dotchainSeg(marker: string): (r: any, ctx: any) => void {
  return (r: any, ctx: any) => {
    const tok = r.o1
    if (!tok || -1 === tok.tin) {
      dotchainAppendTo(r, ctx, newText(marker))
      return
    }
    let quote = ''
    if (tok.tin === ctx.cfg.t.ST) {
      quote = "'"
    }
    let str: string = tok.src
    if ('string' === typeof tok.val) {
      str = tok.val
    }
    dotchainAppendTo(r, ctx, newText(marker), newText(str, quote))
  }
}

// setupPairGrammar extends "pair" and "elem" rules for optional-field (?:)
// and computed-key ([key]:) syntax in both map and list contexts.
export function setupPairGrammar(j: any, t: ParserTokens): void {
  const TX = j.token('#TX')
  const ST = j.token('#ST')
  const CL = j.token('#CL')
  const CB = j.token('#CB')
  const OS = j.token('#OS')
  const CS = j.token('#CS')

  // Shared helper: extract key from pair-open token.
  const pairkey = (r: any): void => {
    const keyToken = r.o0
    let key: string
    if (keyToken.tin === ST || keyToken.tin === TX) {
      key = 'string' === typeof keyToken.val ? keyToken.val : ''
    } else {
      key = keyToken.src
    }
    r.u['key'] = key
  }

  // --- Optional field syntax: key?:value in pair context ---

  j.rule('pair', (rs: any) => {
    prependOpen(rs, [
      // Match KEY ? — save key via K (propagated), push to pair.
      {
        s: ['#KEY', t.QM],
        p: 'pair',
        k: { boru_qm: true },
        a: (r: any) => {
          pairkey(r)
          // Propagate key to inner pair via K.
          r.k['key'] = r.u['key']
        },
      },
      // Match : when boru_qm flag is set — copy key from K, proceed as pair value.
      {
        s: [CL],
        c: (r: any) => {
          return 'boru_qm' in r.k
        },
        p: 'val',
        u: { pair: true },
        a: (r: any) => {
          r.u['key'] = r.k['key']
        },
      },
      // Optional shorthand with NO value — `{foo?}`. The inner pair
      // pushed by the `KEY ?` alt sees the closing brace; backtrack so
      // the map closes, and record the key both as a shorthand (so
      // convertMapData synthesises `foo: foo`) and as optional (Meta
      // "qm"). Without this the empty pushed pair has no alt for the
      // closing brace and errors. (The legacy jsonicjs pair rule closed
      // an empty pushed pair on the brace implicitly; tabnas does not.)
      {
        s: [CB],
        c: (r: any) => {
          return 'boru_qm' in r.k
        },
        b: 1,
        a: (r: any) => {
          if ('key' in r.k) {
            const k = r.k['key']
            r.u['key'] = k
            if ('string' === typeof k) {
              r.u['boru_sh'] = k
            }
          }
        },
      },
    ])
  })

  // Record optional keys in map Meta via pair.BC callback.
  j.rule('pair', (rs: any) => {
    rs.bc((r: any) => {
      if (!('boru_qm' in r.k)) {
        return
      }
      const key = 'string' === typeof r.u['key'] ? r.u['key'] : ''
      if ('' === key) {
        return
      }
      // Store optional key in map Meta["qm"].
      const info = mapInfoOf(r.node)
      if (info) {
        const qmSet: Record<string, boolean> = info.meta['qm'] ?? {}
        qmSet[key] = true
        info.meta['qm'] = qmSet
      }
    })
  })

  // --- Computed key syntax: {[key]:value} in pair context ---

  j.rule('pair', (rs: any) => {
    prependOpen(rs, [
      // Step 1: match [ — set computed key flag, push to pair.
      { s: [OS], p: 'pair', k: { boru_ck: true } },
      // Step 2: match KEY ] when boru_ck — save key, push to pair.
      {
        s: ['#KEY', CS],
        c: (r: any) => {
          return 'boru_ck' in r.k
        },
        p: 'pair',
        a: (r: any) => {
          pairkey(r)
          r.k['key'] = r.u['key']
        },
      },
      // Step 3: match : when boru_ck and key is set — proceed as pair value.
      {
        s: [CL],
        c: (r: any) => {
          return 'boru_ck' in r.k && 'key' in r.k
        },
        p: 'val',
        u: { pair: true },
        a: (r: any) => {
          r.u['key'] = r.k['key']
        },
      },
    ])
  })

  // Record computed keys in map Meta via pair.BC callback.
  j.rule('pair', (rs: any) => {
    rs.bc((r: any) => {
      if (!('boru_ck' in r.k)) {
        return
      }
      try {
        const key = 'string' === typeof r.u['key'] ? r.u['key'] : ''
        if ('' === key) {
          return
        }
        // Store computed key in map Meta["ck"].
        const info = mapInfoOf(r.node)
        if (info) {
          const ckSet: Record<string, boolean> = info.meta['ck'] ?? {}
          ckSet[key] = true
          info.meta['ck'] = ckSet
        }
      } finally {
        // Clear the computed-key flag as this pair closes so it cannot
        // LEAK to the sibling pair. tabnas's K map is shared across the
        // sibling pairs of a map, so a lingering boru_ck made a later bare
        // key (`{[k]:1 aa:2}`) wrongly parse as computed — the pre-existing
        // leak the D1 order channel sits on (design/FLEX-ATTRS.1.md §2.6).
        delete r.k['boru_ck']
      }
    })
  })

  // --- Shorthand field syntax: {foo} ≡ {foo: foo}, {foo/r} ≡ {foo: foo/r} ---

  j.rule('pair', (rs: any) => {
    rs.open(
      [
        // A lone unquoted-text key with no following colon. Appended
        // LAST so it never shadows a real `key:value` pair: the base
        // KEY CL alt (and the prepended qm/ck alts) are tried first,
        // and only `{foo}` / `{foo/r}` — where no colon, `?`, or `[`
        // follows — fall through to here. The optional shorthand
        // `{foo?}` is handled by the qm path above (it consumes
        // `foo ?` and records Meta["qm"]); the value is synthesized in
        // convertMapData. No P/U["pair"]: nothing is pushed, so the
        // built-in pairval BC stays inert and the pair closes cleanly.
        {
          s: [TX],
          a: (r: any) => {
            if ('string' === typeof r.o0.val) {
              r.u['boru_sh'] = r.o0.val
            }
          },
        },
      ],
      { append: true },
    )
  })

  // Record shorthand keys in map Meta["sh"] via pair.BC callback.
  j.rule('pair', (rs: any) => {
    rs.bc((r: any) => {
      const raw = r.u['boru_sh']
      if ('string' !== typeof raw || '' === raw) {
        return
      }
      const info = mapInfoOf(r.node)
      if (info) {
        const shList: string[] = info.meta['sh'] ?? []
        shList.push(raw)
        info.meta['sh'] = shList
      }
    })
  })

  // Record quoted keys in map Meta["qk"] via pair.BC callback. A
  // quoted key (`{'a/b': 1}`) arrives as a string token (#ST) rather
  // than bare text (#TX); convertMapData can't tell them apart from the
  // plain string map afterwards, so it needs this flag to know the key
  // was an explicit literal — e.g. to allow a `/` in it that would be an
  // illegal word modifier on a bare key.
  j.rule('pair', (rs: any) => {
    rs.bc((r: any) => {
      if (r.o0.tin !== ST) {
        return
      }
      const key = r.o0.val
      if ('string' !== typeof key || '' === key) {
        return
      }
      const info = mapInfoOf(r.node)
      if (info) {
        const qkSet: Record<string, boolean> = info.meta['qk'] ?? {}
        qkSet[key] = true
        info.meta['qk'] = qkSet
      }
    })
  })

  // Record source KEY ORDER in map Meta["ko"] via pair.BC callback —
  // the D1 insertion-order channel (design/FLEX-ATTRS.1.md §3). The Go
  // engine hands convertMapData an UNORDERED Go map, so without this the
  // parser sorts keys for determinism and a `{b:2 a:1}` literal loses its
  // source order. BC fires once per pair in source order; each records its
  // FINAL union key (the same string convertMapData keys on): a shorthand
  // base name, an explicit/computed/optional key, else the bare pair key.
  // First-position/last-value dedup — a repeated key keeps its first slot
  // (matching OrderedMap.Set), and inner pushed pairs (the qm/ck
  // double-fire) collapse onto the same slot.
  j.rule('pair', (rs: any) => {
    rs.bc((r: any) => {
      let key = ''
      if (null != r.u['boru_sh']) {
        if ('string' === typeof r.u['boru_sh']) {
          key = wordBaseName(r.u['boru_sh'])
        }
      } else if (null != r.u['key']) {
        key = 'string' === typeof r.u['key'] ? r.u['key'] : ''
      } else if (r.o0 && -1 !== r.o0.tin) {
        // The explicit/computed/optional/quoted pairs all set
        // r.u["key"] above; a pair reaching here is a bare `k:v`,
        // whose key token renders verbatim via Src (its Val is a
        // non-string primitive — see the D1 coverage note).
        key = r.o0.src
      }
      if ('' === key) {
        return
      }
      // Sibling recording BCs (qk/sh/ck) gate on the map node the same
      // way; the pair always closes into a map node, so the miss arm is
      // unreachable and stays out of the guard.
      const info = mapInfoOf(r.node)
      if (info) {
        const ko: string[] = info.meta['ko'] ?? []
        for (const e of ko) {
          if (e === key) {
            return // already recorded — first-position dedup
          }
        }
        ko.push(key)
        info.meta['ko'] = ko
      }
    })
  })

  // --- Optional field syntax in list context: [x?:Integer] ---

  j.rule('elem', (rs: any) => {
    prependOpen(rs, [
      // Step 1: match KEY ? — save key, push to elem.
      {
        s: ['#KEY', t.QM],
        p: 'elem',
        k: { boru_qm: true },
        u: { done: true },
        a: (r: any) => {
          pairkey(r)
          r.k['key'] = r.u['key']
        },
      },
      // Step 2: match : when boru_qm is set — proceed as list pair.
      {
        s: [CL],
        c: (r: any) => {
          return 'boru_qm' in r.k
        },
        p: 'val',
        n: { pk: 1, dmap: 1 },
        u: { done: true, pair: true, list: true },
        a: (r: any) => {
          r.u['key'] = String(r.k['key']) + '?'
        },
      },
    ])
  })

  // Propagate the outer elem's updated node to grandparent (list rule).
  j.rule('elem', (rs: any) => {
    rs.bc((r: any, ctx: any) => {
      if (!('boru_qm' in r.k)) {
        return
      }
      // Only propagate from the OUTER elem (which has done=true).
      if (true !== r.u['done']) {
        return
      }
      if (r.parent && r.parent !== ctx.NORULE) {
        r.parent.node = r.node
      }
    })
  })

  // --- Lambda arrow closes an implicit pair-dive: (x:Integer => body) ---
  //
  // Inside a paren, `x:Integer` parses via val's pair-dive alternate
  // (pk > 0), and the dive's pair/map rules have no alternative for the
  // arrow token — pair.Close falls to the implicit-continuation
  // fallback, re-opens `pair` on `=>`, and errors with "unexpected
  // character". These two Close alternates end the dive instead
  // (backtracking the arrow), so the implicit one-entry map
  // {x:Integer} completes as the preceding value and the arrow
  // re-reads in the enclosing context, where val's #AR alternate emits
  // `afn`. ParseFnParams reads the Implicit map as a named param, so
  // `(x:Integer => [x mul 2])` ≡ `([x:Integer] => [x mul 2])`.
  //
  // The pk condition scopes both alternates to IMPLICIT pairs: a
  // pair-dive runs with pk > 0, the top-level implicit map runs with
  // pk ABSENT, and pairs of an explicit `{…}` map run with pk = 0
  // (set at the OB open) — so `{a: 1 => 2}` keeps erroring exactly as
  // before. The matched parse state was unconditionally fatal before
  // these alternates, so they are additive — nothing that parsed
  // previously changes shape.
  const arrowEndsDive = (r: any): boolean => {
    const pk = r.n['pk']
    return undefined === pk || pk > 0
  }
  j.rule('pair', (rs: any) => {
    prependClose(rs, [{ s: [t.AR], c: arrowEndsDive, b: 1 }])
  })
  j.rule('map', (rs: any) => {
    prependClose(rs, [{ s: [t.AR], c: arrowEndsDive, b: 1 }])
  })
}

// setupParenGrammar defines the "paren" and "pelem" rules that collect
// values between ( and ) into a parenGroup.
export function setupParenGrammar(j: any, t: ParserTokens): void {
  const CA = j.token('#CA')
  const ZZ = j.token('#ZZ')

  // Paren rule: collects values between ( and ) into a parenGroup.
  // Works like a simplified list rule but closes on ) instead of ].
  j.rule('paren', (rs: any) => {
    setBO(rs, [
      (r: any) => {
        r.node = []
      },
      // Increment dlist and dmap so val.Close won't create
      // implicit lists or maps inside paren groups.
      (r: any) => {
        r.n['dlist'] = (r.n['dlist'] ?? 0) + 1
        r.n['dmap'] = (r.n['dmap'] ?? 0) + 1
      },
    ])
    setBC(rs, [
      (r: any) => {
        if (Array.isArray(r.node)) {
          if (r.node.length > 0 && r.node[r.node.length - 1] === null) {
            // A trailing comma hole (`(1,)`, `(,)`): Go's grammar
            // derails the paren close here — same taxonomy and text.
            throw new BoruError('syntax_error', 'unmatched opening parenthesis', '(')
          }
          r.node = new ParenGroup(r.node)
        }
      },
    ])
    setAC(rs, [
      (r: any) => {
        if (true !== r.u['closed']) {
          if (r.node instanceof ParenGroup) {
            r.node = new UnclosedParen(r.node.items)
          }
        }
      },
    ])
    setOpen(rs, [
      // Empty parens: (). Match the ")" but BACKTRACK (b:1) so the
      // Close phase consumes it exactly once — like a non-empty paren,
      // whose pelem also backtracks the ")" for paren.Close. Consuming
      // it here instead would let the Close phase eat a SECOND ")",
      // stealing the closer of an enclosing paren when an empty paren
      // is the last element (e.g. `(())`, `(1 ())`).
      { s: [t.CP], b: 1 },
      // First element
      { p: 'pelem' },
    ])
    setClose(rs, [
      { s: [t.CP], u: { closed: true } },
      // End of source: auto-close (unclosed paren)
      { s: [ZZ] },
    ])
  })

  // Pelem (paren element): each item inside a paren group.
  // Pushes to val for each value, appends to parent paren's list.
  j.rule('pelem', (rs: any) => {
    setBC(rs, [
      (r: any, ctx: any) => {
        if (Array.isArray(r.node)) {
          // A comma with no value between (`(1,,2)`, `(,1)`, `(1,)`)
          // leaves the child empty: record a NULL HOLE so the paren
          // close and the converter raise the same taxonomy Go does —
          // interior/leading holes are empty list elements; a trailing
          // hole derails the paren close (Go parity).
          r.node.push(undefined !== r.child.node ? r.child.node : null)
          if (r.parent && r.parent !== ctx.NORULE) {
            r.parent.node = r.node
          }
        }
      },
    ])
    setOpen(rs, [{ p: 'val' }])
    setClose(rs, [
      // ) ends the paren group (backtrack so paren.Close consumes it)
      { s: [t.CP], b: 1 },
      // End of source inside paren: auto-close (unclosed paren)
      { s: [ZZ] },
      // Comma: next element
      { s: [CA], r: 'pelem' },
      // Space-separated: next element (backtrack to re-read token)
      { r: 'pelem', b: 1 },
    ])
  })
}

// setupInterpGrammar defines the "interp", "ielem", "iexpr", and "ieval"
// rules for template string interpolation (`hello ${name}`).
export function setupInterpGrammar(j: any, t: ParserTokens): void {
  const CA = j.token('#CA')
  const CB = j.token('#CB')
  const ZZ = j.token('#ZZ')

  // Interp rule: collects template string parts between backticks.
  // K["boru_tpl"] is set in BO so the custom matcher produces #TL tokens
  // for literal text segments. Parts are accumulated in node as an interpGroup.
  j.rule('interp', (rs: any) => {
    setBO(rs, [
      (r: any) => {
        r.node = new InterpGroup([])
        // Set K so the custom matcher knows we're inside a template.
        // K propagates to child rules.
        r.k['boru_tpl'] = true
      },
    ])
    setOpen(rs, [
      // Empty template: `` (immediate closing backtick)
      { s: [t.BT] },
      // First element: push to ielem.
      { p: 'ielem' },
    ])
    setClose(rs, [
      // Closing backtick ends the template.
      { s: [t.BT] },
      // End of source: unterminated template string (auto-close).
      { s: [ZZ] },
    ])
  })

  // Ielem rule: each element inside a template string.
  // Handles #TL (literal text) and #IS (interpolation start).
  // On close, appends its own node to the parent interp's interpGroup.
  j.rule('ielem', (rs: any) => {
    setBC(rs, [
      (r: any, ctx: any) => {
        // Collect the node value. For #TL, the action sets r.node directly.
        // For #IS→iexpr, the child rule sets r.node (iexprGroup) via push.
        let node = r.node
        if (undefined !== r.child.node) {
          node = r.child.node
        }
        if (undefined === node) {
          return
        }
        if (r.parent && r.parent !== ctx.NORULE) {
          if (r.parent.node instanceof InterpGroup) {
            r.parent.node.parts.push(node)
          }
        }
      },
    ])
    setOpen(rs, [
      // Literal text segment.
      {
        s: [t.TL],
        a: (r: any) => {
          // Store literal text as a Text with Quote="tl".
          if ('string' === typeof r.o0.val) {
            r.node = newText(r.o0.val, 'tl')
          }
        },
      },
      // Interpolation start ${ — push to iexpr to collect the expression.
      { s: [t.IS], p: 'iexpr' },
    ])
    setClose(rs, [
      // Closing backtick: backtrack so interp.Close can consume it.
      { s: [t.BT], b: 1 },
      // End of source.
      { s: [ZZ] },
      // Next element (another literal or interpolation).
      { r: 'ielem', b: 1 },
    ])
  })

  // Iexpr rule: collects expression values between ${ and }.
  // Pushes to val for each value, collects into a list.
  // Clears boru_tpl in BO so that expression content is parsed normally
  // (the custom matcher won't fire inside expressions).
  j.rule('iexpr', (rs: any) => {
    setBO(rs, [
      (r: any) => {
        r.node = []
        // Clear template mode so expressions parse normally.
        delete r.k['boru_tpl']
        // Increment dlist and dmap so val.Close won't create
        // implicit lists or maps inside interpolation expressions.
        r.n['dlist'] = (r.n['dlist'] ?? 0) + 1
        r.n['dmap'] = (r.n['dmap'] ?? 0) + 1
      },
    ])
    setBC(rs, [
      (r: any) => {
        // Wrap the collected values as an iexprGroup.
        if (Array.isArray(r.node)) {
          r.node = new IexprGroup(r.node)
        }
      },
    ])
    setOpen(rs, [
      // Empty expression: ${}
      { s: [CB] },
      // First expression value.
      { p: 'ieval' },
    ])
    setClose(rs, [
      // Closing brace } ends the expression.
      { s: [CB] },
      // End of source inside expression.
      { s: [ZZ] },
    ])
  })

  // Ieval rule: each value inside an interpolation expression.
  // Similar to pelem but closes on } instead of ).
  j.rule('ieval', (rs: any) => {
    setBC(rs, [
      (r: any, ctx: any) => {
        if (undefined !== r.child.node) {
          if (Array.isArray(r.node)) {
            r.node.push(r.child.node)
            if (r.parent && r.parent !== ctx.NORULE) {
              r.parent.node = r.node
            }
          }
        }
      },
    ])
    setOpen(rs, [{ p: 'val' }])
    setClose(rs, [
      // } ends the expression (backtrack so iexpr.Close consumes it).
      { s: [CB], b: 1 },
      // End of source.
      { s: [ZZ] },
      // Comma: next expression value.
      { s: [CA], r: 'ieval' },
      // Space-separated: next expression value.
      { r: 'ieval', b: 1 },
    ])
  })
}

// setupAngleGrammar defines the generics angle-bracket sugar rules
// (design/GENERICS.10.md Phase 6, decisions D14/D15): `Box<Integer>`.
//
// The consumer is CONTEXTUALLY GATED (D14): an angle group opens only
// when the value that just closed is a capitalised bare name — the
// type-name convention — via a val.Close alternate (the dotchain
// technique). A `<` in any other position matches no rule and is a
// syntax error in v1, only because no other consumer exists yet.
//
// The "angle"/"aelem" rule pair is modeled on "paren"/"pelem": items
// collect into a flat list (commas are optional separators, exactly
// like list/paren elements), and the BC fold replaces the receiver
// val's node with an angleGroup{Name, Items}. Desugaring to the
// canonical stream happens in the conversion walks (convert.ts), so no
// angle marker ever reaches the engine value stream — macros, quote,
// and Vm.parse see the canonical form (D15).
export function setupAngleGrammar(j: any, t: ParserTokens): void {
  const CA = j.token('#CA')
  const ZZ = j.token('#ZZ')

  // Gate: the just-closed val is a capitalised bare name.
  const angleGate = (r: any): boolean => {
    let n = r.node
    if (n instanceof Sited) {
      n = n.node
    }
    const txt = asText(n)
    if (!txt || '' !== txt.quote || '' === txt.str) {
      return false
    }
    const c = txt.str[0] as string
    return c >= 'A' && c <= 'Z'
  }
  j.rule('val', (rs: any) => {
    prependClose(rs, [{ s: [t.LA], c: angleGate, p: 'angle' }])
  })

  j.rule('angle', (rs: any) => {
    setBO(rs, [
      (r: any) => {
        r.node = []
      },
      // Suppress implicit list/map formation inside the group
      // (same bump the paren rule applies).
      (r: any) => {
        r.n['dlist'] = (r.n['dlist'] ?? 0) + 1
        r.n['dmap'] = (r.n['dmap'] ?? 0) + 1
      },
    ])
    setBC(rs, [
      (r: any, ctx: any) => {
        if (!r.parent || r.parent === ctx.NORULE) {
          return
        }
        let recv = r.parent.node
        let pos: SrcPos = { row: 0, col: 0 }
        if (recv instanceof Sited) {
          pos = recv.pos
          recv = recv.node
        }
        let name = ''
        const txt = asText(recv)
        if (txt) {
          name = txt.str
        }
        const arr: unknown[] = Array.isArray(r.node) ? r.node : []
        const grp = new Sited(new AngleGroup(name, arr), pos)
        r.parent.node = grp
        // Mirror onto this rule's own node: tabnas's val coalescer
        // (`@val-bc`, re-run after this close-pushed rule completes)
        // sets the parent val's value from r.child.node, so the
        // angleGroup must live here too (same reason as dotchain).
        r.node = grp
      },
    ])
    // The "closed" flag is set by the Close alternate, which runs
    // AFTER BC — so the unclosed-at-EOF rewrite happens in AC (the
    // unclosed-paren technique).
    setAC(rs, [
      (r: any, ctx: any) => {
        if (true === r.u['closed']) {
          return
        }
        if (!r.parent || r.parent === ctx.NORULE) {
          return
        }
        let name = ''
        if (r.node instanceof Sited && r.node.node instanceof AngleGroup) {
          name = r.node.node.name
        }
        // Set our own node (the val coalesces from r.child.node); also
        // keep the parent in sync for any direct readers.
        r.node = new UnclosedAngle(name)
        r.parent.node = new UnclosedAngle(name)
      },
    ])
    setOpen(rs, [
      // Empty group `Name<>`: backtrack so Close consumes the `>`
      // exactly once (the empty-paren technique).
      { s: [t.RA], b: 1 },
      { p: 'aelem' },
    ])
    setClose(rs, [
      { s: [t.RA], u: { closed: true } },
      // End of source: unclosed — flagged by the BC fold above.
      { s: [ZZ] },
    ])
  })

  // Aelem: each item inside an angle group (the pelem shape with `>`
  // as the boundary). Nested sugar works naturally: an inner val that
  // closes on another `<` re-enters the angle rule, and the two `>`s
  // close inner then outer (no C++-style `>>` shift problem — `>` is
  // a standalone token).
  j.rule('aelem', (rs: any) => {
    setBC(rs, [
      (r: any, ctx: any) => {
        if (undefined !== r.child.node) {
          if (Array.isArray(r.node)) {
            r.node.push(r.child.node)
            if (r.parent && r.parent !== ctx.NORULE) {
              r.parent.node = r.node
            }
          }
        }
      },
    ])
    setOpen(rs, [{ p: 'val' }])
    setClose(rs, [
      // > ends the group (backtrack so angle.Close consumes it).
      { s: [t.RA], b: 1 },
      // End of source inside the group: unclosed.
      { s: [ZZ] },
      // Comma: next item.
      { s: [CA], r: 'aelem' },
      // Space-separated: next item.
      { r: 'aelem', b: 1 },
    ])
  })
}

// setupNumberSub registers a token subscriber that wraps every number
// token in a NumberVal carrying the original source text. The
// source text lets the converter (1) distinguish "5" (integer) from
// "5.0" (decimal), and (2) parse plain decimal integers from their exact
// digits via BigInt rather than round-tripping through the number jsonic
// already produced — a float64 silently corrupts any integer above 2^53,
// so the source string is the only exact record.
// See numberValToValue and design/INTEGER-OVERFLOW-STRATEGY.5.md.
export function setupNumberSub(j: any): void {
  const NR = j.token('#NR')
  j.sub({
    lex: (tkn: any) => {
      if (tkn.tin === NR && 'number' === typeof tkn.val) {
        tkn.val = new NumberVal(tkn.val, tkn.src, tkn.rI, tkn.cI)
      }
    },
  })
}

// wordBaseName returns the base name of an unquoted word token, stripping
// a valid `/...` modifier suffix (foo/r → foo, foo → foo). Used to derive
// the key of a shorthand map entry, whose value keeps the full token.
//
// Go's original is a two-liner over the single scanWordModifier in
// parse.go. This port used to carry an inlined COPY of that scan, on the
// grounds that convert.ts is a separately owned file — but grammar.ts
// already imports processTemplateEscapes from it, so the cycle was
// already there and benign, and the copy had drifted: it bounded the
// digit run with parseInt's NaN check where convert.ts bounds it with
// INT64_MAX, so the two disagreed about `a/9223372036854775808`. Two
// copies of a validity scan is one more place for the engines to
// diverge, and its arms were reachable from neither corpus.
function wordBaseName(text: string): string {
  return scanWordModifier(text).base
}
