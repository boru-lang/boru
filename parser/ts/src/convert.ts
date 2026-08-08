// The conversion layer of the boru TS parser — the port of
// parser/go/parse.go (stage 3 of Go Parse). parse(src) runs the
// configured jsonic instance (grammar.ts makeBoruJsonic) over src and
// converts the jsonic output into engine Values, throwing BoruError on
// syntax errors (jsonic failures are translated by errors.ts
// translateParseError). The Go file is the REFERENCE: every function
// below ports its same-named Go twin, keeping structure and comments.
//
// Where Go type-switches on jsonic node types, this file discriminates
// on the shared intermediate classes in nodes.ts plus the TS jsonic
// info-node shapes (boxed String / array / plain object with a hidden
// `__info__` marker — see scratchpad api-mapping.md).

import {
  Value,
  OrderedMap,
  newWord,
  newInteger,
  newFloat,
  newString,
  newBoolean,
  newAtom,
  newNone,
  newList,
  newMap,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
  newDisjunct,
  newParenExpr,
  newInterpString,
  newReach,
  newDispatchMod,
  newSugar,
  newCloseParen,
  newEnd,
} from '@boru-lang/core'
import type { WordInfo, ReachSeg, ReachInfo, SugarInfo, InterpSegment } from '@boru-lang/core'
import { TWord, TNone, TAbsent, TBigInteger, TBigDecimal } from '@boru-lang/core'
import { Decimal, decimalFromString } from '@boru-lang/core'
import { BoruError } from '@boru-lang/core'
import {
  ParenGroup,
  UnclosedParen,
  AngleGroup,
  UnclosedAngle,
  InterpGroup,
  IexprGroup,
  MiniLitVal,
  NumberVal,
  ArrowTag,
  XmlElemVal,
  Sited,
  deSite,
  ParseDepth,
  MAX_PARSE_NESTING_DEPTH,
  UNKNOWN_POS,
} from './nodes.ts'
import type { SrcPos } from './nodes.ts'
import { makeBoruJsonic } from './grammar.ts'
import { translateParseError } from './errors.ts'

// ── jsonic info-node access ─────────────────────────────────────────────────

// The hidden metadata property jsonic stamps on info nodes (boxed String
// text markers, ListRef arrays, MapRef objects). See api-mapping.md — the
// name comes from ctx.cfg.info.marker, default '__info__'.
const INFO_MARKER = '__info__'

interface NodeInfo {
  implicit?: boolean
  meta?: Record<string, unknown>
  quote?: string
}

function getInfo(node: object): NodeInfo | undefined {
  return (node as Record<string, unknown>)[INFO_MARKER] as NodeInfo | undefined
}

// The TS analogue of Go's jsonic.Text{Str, Quote}: a boxed String with
// quote metadata. Primitive strings (which can appear where info.text is
// off for a path — api-mapping.md gotcha 1) read as unquoted text, the
// same default a zero-Quote Go Text carries.
interface TextNode {
  str: string
  quote: string
}

function asText(node: unknown): TextNode | undefined {
  if (node instanceof String) {
    const info = getInfo(node)
    return { str: String(node), quote: info?.quote ?? '' }
  }
  if (typeof node === 'string') return { str: node, quote: '' }
  return undefined
}

// makeText builds a boxed-String text marker (the TS spelling of a Go
// jsonic.Text literal like `jsonic.Text{Str: base}`).
function makeText(str: string, quote: string): String {
  const sv = new String(str)
  Object.defineProperty(sv, INFO_MARKER, { value: { quote }, writable: true })
  return sv
}

// isMapNode reports whether v is a jsonic map node (Go's jsonic.MapRef /
// raw map[string]any): a plain object — not an array, boxed String,
// engine Value, or one of the nodes.ts intermediate classes.
function isMapNode(v: unknown): v is Record<string, unknown> {
  if (v === null || typeof v !== 'object') return false
  if (Array.isArray(v) || v instanceof String) return false
  const proto = Object.getPrototypeOf(v)
  return proto === Object.prototype || proto === null
}

// mapKeys returns the own enumerable keys of a jsonic map node, minus the
// hidden info marker (defensively — jsonic defines it non-enumerable).
function mapKeys(m: Record<string, unknown>): string[] {
  return Object.keys(m).filter((k) => k !== INFO_MARKER)
}

function hasOwn(m: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(m, key)
}

// metaStrSet normalises a Meta channel that Go types as map[string]bool
// (qm / ck / qk) into a Set of member keys, accepting whichever shape the
// TS grammar recorded (Set, array, Map, or plain object).
export function metaStrSet(meta: Record<string, unknown> | undefined, key: string): Set<string> {
  const v = meta?.[key]
  if (!v) return new Set()
  if (v instanceof Set) return new Set([...v].map(String))
  if (Array.isArray(v)) return new Set(v.map(String))
  if (v instanceof Map) {
    return new Set([...v.entries()].filter(([, b]) => !!b).map(([k]) => String(k)))
  }
  if (typeof v === 'object') {
    const o = v as Record<string, unknown>
    return new Set(Object.keys(o).filter((k) => !!o[k]))
  }
  return new Set()
}

// metaStrList normalises a Meta channel that Go types as []string (sh / ko).
function metaStrList(meta: Record<string, unknown> | undefined, key: string): string[] {
  const v = meta?.[key]
  return Array.isArray(v) ? v.map(String) : []
}

// ── engine-value helpers the TS value layer does not export ─────────────────

// markEval is the TS spelling of Go's `mv.Eval = true` / NewEvalMap:
// Value.eval is readonly, so a fresh Value is minted with the same
// payload and the eval flag raised.
function markEval(v: Value): Value {
  return new Value(v.vType, v.data, { eval: true, quoted: v.quoted, carrier: v.carrier })
}

// newWordUsurp mirrors eng.NewWordUsurp: a word value marked with the /u
// modifier — resolve the name to its bound Function and wrap it with
// reversed signature arg order; combine with /r to leave it as data. The
// forceUsurp / forceRef flags ride as extra structural properties on
// WordInfo (the TS interface does not declare them yet — see deviations).
function newWordUsurp(name: string, ref: boolean): Value {
  return new Value(TWord, { name, forceUsurp: true, forceRef: ref } as WordInfo)
}

// newWordRef mirrors eng.NewWordRef: a word value marked with the /r
// modifier — resolve the name to its bound Function without invoking.
function newWordRef(name: string): Value {
  return new Value(TWord, { name, forceRef: true } as WordInfo)
}

// newWordModified mirrors eng.NewWordModified: a word with explicit
// argument-shape modifiers. Go's -1 argCount sentinel maps to the TS
// optional-field convention (undefined = unset).
function newWordModified(
  name: string,
  argCount: number,
  forceStack: boolean,
  forceForward: boolean,
): Value {
  const info: WordInfo = { name, forceStack, forceForward }
  if (argCount >= 0) info.argCount = argCount
  return new Value(TWord, info)
}

// newBigInteger / newBigDecimal mirror eng.NewBigInteger/NewBigDecimal.
// TS has no apd — the BigDecimal payload is a binary64 number (closest
// available; recorded in deviations). BigInteger keeps full precision
// via bigint.
function newBigInteger(n: bigint): Value {
  return new Value(TBigInteger, n)
}

function newBigDecimal(d: Decimal): Value {
  return new Value(TBigDecimal, d)
}

const INT64_MIN = -9223372036854775808n
const INT64_MAX = 9223372036854775807n

// errMessage extracts the message of an unknown thrown value (the TS
// spelling of Go's err.Error()).
function errMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

// typeName renders a node's type for the "unsupported value type"
// diagnostics (Go's %T; unreachable in practice).
export function typeName(v: unknown): string {
  if (v === null) return 'null'
  if (typeof v === 'object') return v.constructor?.name ?? 'object'
  return typeof v
}

// goQuote approximates Go's %q verb for the identifier-shaped keys the
// diagnostics quote.
function goQuote(s: string): string {
  return JSON.stringify(s)
}

// ── depth guard ─────────────────────────────────────────────────────────────

// enterDepth descends one container level, failing with a clean
// evaluation_limit error (never a stack overflow) once nesting passes
// MAX_PARSE_NESTING_DEPTH. Pair every enterDepth with a finally'd
// leaveDepth. Mirrors Go parseDepth.enter/leave (the constant and the
// per-parse tracker live in nodes.ts).
function enterDepth(d: ParseDepth): void {
  d.cur++
  // Provably unreachable backstop: checkSourceNesting refuses at the
  // TS-safe 500 bound before any converter can recurse this deep (the
  // Go twin's 10,000 guard is live because Go has no prescan).
  /* node:coverage ignore next 6 */
  if (d.cur > MAX_PARSE_NESTING_DEPTH) {
    throw new BoruError(
      'evaluation_limit',
      `source nesting exceeds the depth limit of ${MAX_PARSE_NESTING_DEPTH} — lists, maps, and parentheses are nested too deeply`,
    )
  }
}

function leaveDepth(d: ParseDepth): void {
  d.cur--
}

// withPos stamps a source position onto v. TS Values carry no pos field
// yet — position stamping lands later, so this is a no-op that returns
// the value unchanged (recorded in deviations). Kept so every stamping
// site mirrors the Go source line-for-line.
function withPos(v: Value, _pos: SrcPos): Value {
  return v
}

// ── Parse ───────────────────────────────────────────────────────────────────

/**
 * parse tokenizes the boru source string into an array of engine Values.
 * The input is treated as a top-level implicit list: jsonic parses the
 * entire source. The info.text option distinguishes quoted strings from
 * unquoted text (words).
 *
 * Custom tokens are registered for (, ), and . so that they are lexed as
 * separate tokens by jsonic (grammar.ts). This replaces the earlier
 * preprocessParens approach and string-based dot expansion, making the
 * parser cleaner.
 *
 * Context rules:
 *   - Top level: unquoted text → words, quoted text → strings.
 *   - Inside maps (including implicit): all text → scalar data.
 *   - Inside lists at the top level: unquoted text → words (quotation).
 *   - Inside lists inside maps: all text → scalar data.
 *
 * Mirrors Go Parse. (Go's BeginIDMintScope has no TS counterpart — the
 * TS value layer mints no IDs; recorded in deviations.)
 */
/**
 * TS-specific nesting bound, enforced by a LINEAR pre-parse scan.
 * The Go parser's 10,000-level guard lives in the conversion walk,
 * but in TS the tabnas rule engine itself recurses per nesting level
 * and overflows the JS call stack near ~900 levels — before any
 * converter-side counter could fire — surfacing an uncontrolled
 * RangeError instead of the promised evaluation_limit. The scan
 * refuses well under the measured overflow point; the converters'
 * ParseDepth guard stays as the backstop.
 */
const TS_MAX_PARSE_NESTING = 500

/**
 * checkSourceNesting scans src once, tracking bracket/paren nesting
 * outside string/template literals and comments, and raises the same
 * evaluation_limit error the Go depth guard produces when the source
 * nests past the TS-safe bound.
 */
function checkSourceNesting(src: string): void {
  let depth = 0
  let quote: string | null = null
  for (let i = 0; i < src.length; i++) {
    const c = src[i]!
    if (quote !== null) {
      if (c === '\\') {
        i++
      } else if (c === quote) {
        quote = null
      }
      continue
    }
    if (c === "'" || c === '"' || c === '`') {
      quote = c
      continue
    }
    if (c === '#') {
      while (i < src.length && src[i] !== '\n') i++
      continue
    }
    if (c === '[' || c === '{' || c === '(') {
      depth++
      if (depth > TS_MAX_PARSE_NESTING) {
        throw new BoruError(
          'evaluation_limit',
          `source nesting exceeds the depth limit of ${TS_MAX_PARSE_NESTING} — lists, maps, and parentheses are nested too deeply`,
          '',
        )
      }
    } else if (c === ']' || c === '}' || c === ')') {
      if (depth > 0) depth--
    }
  }
}

/**
 * watchRuleSteps counts the rule iterations one parse performs, so parse()
 * can tell "the engine finished" from "the engine gave up".
 *
 * WHY THIS EXISTS. The tabnas TS rule engine bounds its main loop at
 * `2 * ruleCount * srcLength * 2 * maxmul` iterations and, on reaching
 * that bound, simply STOPS — the trailing-token check then sees #ZZ, so
 * nothing is thrown and the partial root is returned. For boru that turns
 * a malformed program into a silent wrong answer: `[Map<]` parsed to an
 * EMPTY value stream and `Map<]` to a bare `word(Map)`, where Go's engine
 * reports "unexpected `]`". The shapes that reach it are groups a
 * container terminator cannot close — `[Map<]`, `{a: (1}`, `[1 (2]` —
 * because the TS val rule's implicit-null alternate matches the `]`,
 * backtracks, and the enclosing elem re-pushes forever.
 *
 * The cap is what the two ENGINES do differently, not what the two boru
 * ports do, so this cannot be fixed by porting a rule more faithfully. It
 * is caught rather than papered over: an error whose text differs from
 * Go's is a recorded divergence (parser/spec/divergent.tsv), while
 * silently accepting a program Go rejects is a correctness bug.
 *
 * The test is EQUALITY against the library's own formula, not `>=`. The
 * loop exits with kI === cap, so the last subscriber call saw cap - 1. If
 * the library ever changes the formula this guard stops firing rather
 * than firing early — the fail-open direction. A valid parse uses O(token
 * count) steps (~11 for `[1 2]` against a cap of ~1000), so landing on
 * exactly cap - 1 by accident is not a practical concern.
 */
function watchRuleSteps(j: any): { exhausted: (src: string) => boolean } {
  let last = -1
  j.sub({
    rule: (_rule: unknown, ctx: any) => {
      last = ctx.kI
    },
  })
  return {
    exhausted: (src: string): boolean => {
      // An EMPTY source short-circuits before the loop runs, leaving the
      // step count at -1 and the cap at 0 — which would satisfy the
      // equality below by coincidence. It is the one input where "no
      // steps" means success rather than exhaustion.
      const cap = ruleStepCap(j, src)
      return cap > 0 && last === cap - 1
    },
  }
}

/** ruleStepCap mirrors the tabnas parser's own maximum-iteration bound. */
function ruleStepCap(j: any, src: string): number {
  const internal = j.internal()
  const ruleCount = Object.keys(internal.parser.rsm).length
  return 2 * ruleCount * src.length * 2 * internal.config.rule.maxmul
}

export function parse(src: string): Value[] {
  // A linear nesting pre-check guards the recursive rule engine (see
  // checkSourceNesting) before any parsing work happens.
  checkSourceNesting(src)
  // Stages 1-2 (lex + grammar setup) live in grammar.ts; the instance is
  // built per parse, exactly as Go Parse constructs a fresh jsonic.
  const { j } = makeBoruJsonic()

  // Stage 3: Parse and convert to engine values. The library's own error
  // rendering is silenced at the source (grammar.ts options); failures
  // are translated into boru syntax_errors here.
  const steps = watchRuleSteps(j)
  let result: unknown
  try {
    result = j.parse(src)
  } catch (e) {
    throw translateParseError(e, src)
  }
  // The TS rule engine gave up rather than parsed: see ruleStepCap.
  if (steps.exhausted(src)) {
    throw new BoruError(
      'syntax_error',
      'the parser could not make progress here — a group is left open by a `]`, `}` or end of input that cannot close it',
      '',
    )
  }

  if (result === null || result === undefined) {
    return []
  }

  // A single top-level scalar/paren/interp value arrives wrapped by the
  // val-rule BC; unwrap it so the cases below see a bare jsonic node, but
  // keep its position to stamp the single produced value. (Root containers
  // come from the list/map rule and are not sited; their elements carry
  // positions individually.)
  const [res, rootPos] = deSite(result)

  // One depth tracker for this parse, threaded through the recursive
  // converters so pathologically deep nesting fails with a clean
  // evaluation_limit error instead of overflowing the stack — enforced
  // inside the single conversion walk, not as a separate pre-pass.
  const d = new ParseDepth(src)

  // With info.list and info.map enabled, jsonic returns list/map carriers
  // for all lists and maps (array / object with a hidden __info__).
  // info.implicit distinguishes implicit structures from explicit ones.
  if (Array.isArray(res)) {
    if ((res as Record<string, unknown> & unknown[])['child$'] !== undefined) {
      return [convertTypedList(res, d)]
    }
    if (!getInfo(res)?.implicit) {
      // Explicit list [...]  — a single list value (quotation).
      return [convertWordList(res, d)]
    }
    // Implicit list — top-level stack values.
    return convertTopLevel(res, d)
  }
  if (isMapNode(res)) {
    const info = getInfo(res)
    // A map node without an info marker is Go's raw map[string]any case
    // (pair syntax): implicit, no meta.
    const implicit = info ? !!info.implicit : true
    const meta = info?.meta
    if (hasMapChild(res)) {
      return [convertTypedMap(res, d, meta)]
    }
    let mv = convertMapData(res, implicit, d, meta)
    // Top-level implicit maps (e.g. entire input is "a:x") must be
    // auto-evaluated so expressions in values resolve.
    if (implicit && !mv.eval) {
      mv = markEval(mv)
    }
    return [mv]
  }
  if (res instanceof UnclosedParen) {
    throw new BoruError('syntax_error', 'unmatched opening parenthesis', '(')
  }
  if (res instanceof ParenGroup) {
    // Single paren group at top level: expand to paren markers.
    return convertTopLevelItems([res], d)
  }
  if (res instanceof InterpGroup) {
    // Single template string at top level.
    return [withPos(convertInterpGroup(res, d), rootPos)]
  }

  // Single top-level scalar/word: stamp the position the root deSite
  // recovered (convertTopLevelValue sees a bare node, so it cannot).
  return [withPos(convertTopLevelValue(res, d), rootPos)]
}

// isToken checks if item is an unquoted text marker matching the given string.
// Quoted text (e.g. "." or "!") has quote != '' and is handled as a string
// by convertTopLevelValue, so it never reaches the token checks.
function isToken(item: unknown, tok: string): boolean {
  const [node] = deSite(item)
  const text = asText(node)
  return text !== undefined && text.str === tok && text.quote === ''
}

// convertTopLevelItems converts a list of jsonic items in word context,
// handling parenthesis markers and token sequences.
//
// Dotted access is sugar for `get`/`getr` and is GROUPED so it binds to
// its immediate receiver: a chain `recv.k1.k2` (or `recv!.k1`) wraps as a
// single paren group `( recv get k1 get k2 … )`. Grouping isolates the
// access from surrounding forward-collection, so `size m.x` means
// `size (m.x)`, not `(size m).x`. `get`/`getr` left-compose on the stack,
// so one flat group covers any chain length. The receiver and each key may
// themselves be paren groups, which covers `(expr).k` and the computed-key
// form `m.(expr)`.
//
//   - "." → get      "!" "." → getr
//
// A lone "." / "!." with no receiver still emits the bare word (and errors
// at runtime, as before). All other items convert to engine values
// directly.
function convertTopLevelItems(items: unknown[], d: ParseDepth): Value[] {
  enterDepth(d)
  try {
    // Strip sited wrappers once into a bare-node slice plus a parallel
    // position slice. The token helpers (isToken/startsDot/isChainReceiver)
    // then see bare jsonic nodes, while the emitted operator-marker words
    // (get/getr) and primaries are still stamped from poss[i].
    const poss: SrcPos[] = new Array<SrcPos>(items.length).fill(UNKNOWN_POS)
    if (anySited(items)) {
      const bare: unknown[] = new Array(items.length)
      for (let i = 0; i < items.length; i++) {
        const [node, pos] = deSite(items[i])
        bare[i] = node
        poss[i] = pos
      }
      items = bare
    }

    const values: Value[] = []
    for (let i = 0; i < items.length; i++) {
      // Minilang literal `+name<delim>src<delim>`: emit the three tokens
      // `mini name 'src'` INLINE in word context (top level, list elements,
      // paren contents) — byte-for-byte the same value stream as if the
      // user had typed `mini name 'src'`. This keeps full parity with the
      // explicit form (a trailing opts Map collected by mini's 3-arg sig, a
      // stack subject, the expansion-time unknown-kind error, and check
      // mode) instead of wrapping in an outer splice (which would
      // double-splice mini's own splice and trip check mode at the bare top
      // level). Data-context map values fall back to the splice form in
      // convertTopLevelValueInner.
      const ml = items[i]
      if (ml instanceof MiniLitVal) {
        values.push(
          withPos(newSugar({ kind: 'mini', name: ml.name, src: ml.src }), poss[i]!),
        )
        continue
      }
      // Dotted-access chain: a receiver primary followed by one or more
      // `.key` / `!.key` segments → one group `( recv get k … )`.
      if (isChainReceiver(items[i]) && i + 1 < items.length && startsDot(items, i + 1)) {
        // A reach receiver must be something `get` can index into (a
        // Map, List, Store, Object, module namespace, …). A number has no
        // members, so a numeric literal before a `.` can never be a
        // valid reach — `1.2.3` (a malformed numeric literal), `1 . 2`,
        // or `5 . foo`. Reject it here with a clear message rather than
        // the runtime "no matching signature for get".
        if (isNumberLiteral(items[i])) {
          throw numberReceiverError(poss[i]!)
        }
        // Build the access group into a temp slice so a trailing
        // `/`-modifier can be placed correctly: a WORD modifier
        // (usurp / stack-args / forward-args) is emitted BEFORE the
        // group so it forward-collects the result (the result must not
        // auto-dispatch first), while the Word/__DM marker (/N /r /q)
        // is emitted AFTER for execFnDefLiteral to peek.
        // Build a first-class Reach node (design/REACH.10.md Phase B):
        // receiver tokens + per-segment {op, literal-or-computed key}.
        const recv: Value[] = []
        emitPrimary(recv, items[i], poss[i]!, d)
        const segs: ReachSeg[] = []
        let j = i + 1
        let pfx: Value[] | null = null
        let sfx: Value[] | null = null
        chain: while (j < items.length) {
          let getr: boolean
          let keyItem: unknown
          let kpos: SrcPos
          if (isToken(items[j], '!') && j + 2 < items.length && isToken(items[j + 1], '.')) {
            getr = true
            keyItem = items[j + 2]
            kpos = poss[j + 2]!
            j += 3
          } else if (isToken(items[j], '.') && j + 1 < items.length) {
            getr = false
            keyItem = items[j + 1]
            kpos = poss[j + 1]!
            j += 2
          } else {
            break chain
          }
          // A `/`-modifier on the final key applies to the whole reach.
          const gm = groupModifier(keyItem)
          if (gm !== null && gm.base !== '') {
            keyItem = makeText(gm.base, '')
            pfx = gm.prefix
            sfx = gm.suffix
          }
          const seg: ReachSeg = { getr, computed: false }
          if (keyItem instanceof ParenGroup) {
            // Computed key: m.(expr) — store the paren's tokens.
            seg.computed = true
            seg.keyExpr = convertTopLevelItems(keyItem.items, d)
          } else {
            // Literal key (word → atom-via-get/q, string, number).
            const tmp: Value[] = []
            emitPrimary(tmp, keyItem, kpos, d)
            // emitPrimary appends exactly one value on success.
            seg.keyLit = tmp[0]!
          }
          segs.push(seg)
        }
        // A `$` receiver marks a RECEIVERLESS reach — a detached lens
        // (`$.name`), not a `$ get name` chain. It is inert (eval=false):
        // it evaluates to itself (a Reach value) so it can be passed as a
        // reusable accessor (`people each $.name`) and applied/rebound to a
        // receiver later. `$` is reserved as a user word (see
        // ValidateWordName), so this never shadows a real binding.
        const reachInfo: ReachInfo = { receiver: recv, segments: segs, eval: true }
        if (isDollarReceiver(recv)) {
          reachInfo.receiver = []
          reachInfo.eval = false
        }
        const pe = withPos(newReach(reachInfo), poss[i]!)
        // A standalone `/mod` token immediately after the path also
        // applies to the result (`(a.b)/mod`).
        if (pfx === null && sfx === null && j < items.length) {
          const gm2 = groupModifier(items[j])
          if (gm2 !== null && gm2.base === '') {
            pfx = gm2.prefix
            sfx = gm2.suffix
            j++
          }
        }
        for (const p of pfx ?? []) {
          values.push(withPos(p, poss[i]!))
        }
        values.push(pe)
        for (const s of sfx ?? []) {
          values.push(withPos(s, poss[i]!))
        }
        i = j - 1
        continue
      }

      // A "!." or "." with no receiver before it is the leading / prefix
      // dot form (e.g. `.5`, `.name`, `!.x`). It has nothing to access,
      // so reject it at parse time with a clear message instead of
      // emitting a receiverless get/getr that fails later as an obscure
      // signature error. The receiverless reach `$.name` is unaffected:
      // it carries an explicit `$` receiver and is built in the chain
      // branch above.
      if (isToken(items[i], '!') && i + 1 < items.length && isToken(items[i + 1], '.')) {
        throw danglingDotError(poss[i]!)
      }
      if (isToken(items[i], '.')) {
        throw danglingDotError(poss[i]!)
      }

      // Unclosed paren: error at parse time.
      if (items[i] instanceof UnclosedParen) {
        throw new BoruError('syntax_error', 'unmatched opening parenthesis', '(')
      }

      // A standalone `/mod` token right after a primary (e.g. `(expr)/s`)
      // applies the modifier to that primary's result. Word modifiers are
      // emitted BEFORE the primary (so they forward-collect the result
      // before any auto-dispatch); the /r /q marker is emitted AFTER.
      if (i + 1 < items.length) {
        const gm = groupModifier(items[i + 1])
        if (gm !== null && gm.base === '') {
          for (const p of gm.prefix ?? []) {
            values.push(withPos(p, poss[i]!))
          }
          emitPrimary(values, items[i], poss[i]!, d)
          for (const s of gm.suffix ?? []) {
            values.push(withPos(s, poss[i]!))
          }
          i++
          continue
        }
      }

      // A value, or a paren group expanded to ( … ) markers.
      emitPrimary(values, items[i], poss[i]!, d)
    }
    return values
  } finally {
    leaveDepth(d)
  }
}

// anySited reports whether any item is a sited wrapper, so the common
// data-context path (where items are never sited) skips re-allocation.
function anySited(items: unknown[]): boolean {
  return items.some((it) => it instanceof Sited)
}

// startsDot reports whether items[j] begins a dot-access segment: a "."
// token, or a "!" immediately followed by ".".
function startsDot(items: unknown[], j: number): boolean {
  if (isToken(items[j], '.')) {
    return true
  }
  return isToken(items[j], '!') && j + 1 < items.length && isToken(items[j + 1], '.')
}

// isDollarReceiver reports whether a dot-chain receiver is the lone reserved
// `$` sentinel — the marker for a receiverless reach (`$.name` → a lens). The
// receiver emits as a single Word("$") (see emitPrimary / convertTopLevelValue).
function isDollarReceiver(recv: Value[]): boolean {
  if (recv.length !== 1 || !recv[0]!.isWord()) {
    return false
  }
  return recv[0]!.asWord().name === '$'
}

// isChainReceiver reports whether an item can be the receiver of a dot
// chain — anything except the "." / "!" operator markers and an unclosed
// paren (i.e. a real value, word, or paren group).
function isChainReceiver(item: unknown): boolean {
  if (item instanceof UnclosedParen) {
    return false
  }
  return !isToken(item, '.') && !isToken(item, '!')
}

// groupModifier decodes a `/`-modifier on a word token that should apply to
// a parenthesised / dotted-path RESULT. It returns the base text (empty for
// a standalone `/mod` token following the group) plus tokens to place around
// the group:
//
//	/u  → prefix `usurp`            /s  → prefix `stack-args`
//	/f  → prefix `forward-args`     /N  → prefix `force-arity N`
//	/r /q → suffix Word/__DM marker (leave the result inert/data)
//
// Word modifiers are PREFIXED (emitted before the group) so they
// forward-collect the result before it can auto-dispatch; the marker is
// SUFFIXED for execFnDefLiteral to peek. So `a.b/u` parses as
// `usurp (a get b)` and `a.b/2` as `force-arity 2 (a get b)`. null for
// a plain word (no modifier). Prefix/suffix keep Go's nil-vs-present
// distinction (null = the Go nil slice).
export function groupModifier(
  item: unknown,
): { base: string; prefix: Value[] | null; suffix: Value[] | null } | null {
  const [node] = deSite(item)
  const t = asText(node)
  if (t === undefined || t.quote !== '') {
    return null
  }
  const m = scanWordModifier(t.str)
  if (!m.valid) {
    return null
  }
  if (m.usurpFlag) {
    return { base: m.base, prefix: [newSugar({ kind: 'usurp' })], suffix: null }
  }
  if (m.forceStack) {
    return { base: m.base, prefix: [newSugar({ kind: 'stack-args' })], suffix: null }
  }
  if (m.forceForward) {
    return { base: m.base, prefix: [newSugar({ kind: 'forward-args' })], suffix: null }
  }
  if (m.argCount >= 0) {
    return {
      base: m.base,
      prefix: [newSugar({ kind: 'force-arity', n: BigInt(m.argCount) })],
      suffix: null,
    }
  }
  if (m.refFlag || m.quoteFlag) {
    return {
      base: m.base,
      prefix: null,
      suffix: [newDispatchMod({ ref: m.refFlag, quote: m.quoteFlag })],
    }
  }
  return null
}

// emitPrimary appends the converted form of a single primary item — a
// value, or a parenthesised group as a nested ParenExpr value — to dst. Used
// for the receiver, the keys of a dot chain, and ordinary top-level items.
// pos is the source position of item (caller has already deSited it).
//
// Word-context paren groups become a single ParenExpr value (paren-nesting
// Step 1, design/PAREN-REPRESENTATION.9.md), the same representation data
// context already uses. The engine evaluates it via evalParenExprResults
// (Step 2 at the pointer, Step 3 in a forward window).
function emitPrimary(dst: Value[], item: unknown, pos: SrcPos, d: ParseDepth): void {
  if (item instanceof ParenGroup) {
    const inner = convertTopLevelItems(item.items, d)
    dst.push(withPos(newParenExpr(inner), pos))
    return
  }
  const v = convertTopLevelValue(item, d)
  // convertTopLevelValue deSites internally, but item arrived bare from
  // the caller's normalization, so re-stamp from the caller's pos.
  dst.push(withPos(v, pos))
}

// convertTopLevel converts a top-level implicit list from jsonic into
// an array of engine Values using word context.
function convertTopLevel(items: unknown[], d: ParseDepth): Value[] {
  return convertTopLevelItems(items, d)
}

// convertTopLevelValue converts a single value in word context.
// Unquoted text → word, quoted text → string. The value is stamped with
// the source position captured by the val-rule BC (if any).
function convertTopLevelValue(v: unknown, d: ParseDepth): Value {
  const [node, pos] = deSite(v)
  return withPos(convertTopLevelValueInner(node, d), pos)
}

export function convertTopLevelValueInner(v: unknown, d: ParseDepth): Value {
  if (v instanceof ArrowTag) {
    // `=>` — the lambda sugar marker; the engine lowers it to the
    // role-bound constructor word (ADR-012 rule 3 amendment).
    return newSugar({ kind: 'lambda' })
  }
  const text = asText(v)
  if (text !== undefined) {
    if (text.quote === '') {
      return parseWord(text.str)
    }
    return newString(text.str)
  }

  if (v instanceof InterpGroup) {
    return convertInterpGroup(v, d)
  }

  if (v instanceof MiniLitVal) {
    // `+name<delim>src<delim>` desugars to the splice `mini name 'src'`
    // (the `word` mechanism): one value that re-steps as the standard
    // mini call, so a stack subject, a trailing opts Map, the
    // unknown-kind error, and check mode all behave exactly as if the
    // user had typed `mini name 'src'`.
    return newSugar({ kind: 'mini', name: v.name, src: v.src })
  }

  if (v instanceof XmlElemVal) {
    // Embedded XML literal `<tag>…</tag>`: the matcher already built
    // the immutable Node/Xml value; surface any build error here.
    if (v.err !== undefined) {
      throw new Error(v.err)
    }
    return v.v!
  }

  if (v instanceof NumberVal) {
    return numberValToValue(v)
  }

  if (typeof v === 'number') {
    return floatToValue(v)
  }

  if (isMapNode(v)) {
    const info = getInfo(v)
    if (info !== undefined) {
      // Go's jsonic.MapRef case.
      if (hasMapChild(v)) {
        return convertTypedMap(v, d, info.meta)
      }
      let mv = convertMapData(v, !!info.implicit, d, info.meta)
      // In word context (top level), implicit maps from pair syntax
      // (e.g. a:x) must be auto-evaluated so expressions resolve.
      if (info.implicit && !mv.eval) {
        mv = markEval(mv)
      }
      return mv
    }
    // Raw map from list.pair syntax (e.g., [x:number] produces
    // {x: Text("number")} inside the list) — Go's map[string]any case.
    if (hasMapChild(v)) {
      return convertTypedMap(v, d)
    }
    const mv = convertMapData(v, true, d)
    // In word context (top level), implicit maps from pair syntax
    // must be auto-evaluated so expressions resolve.
    return markEval(mv)
  }

  if (Array.isArray(v)) {
    if ((v as Record<string, unknown> & unknown[])['child$'] !== undefined) {
      return convertTypedList(v, d)
    }
    return convertWordList(v, d)
  }

  if (v instanceof AngleGroup) {
    // Use-site sugar: `Box<Integer>` → the canonical paren span
    // `( Box of [Integer] )` (a ParenExpr, exactly what the
    // canonical source produces in word context).
    return angleUseSite(v, d)
  }

  if (v instanceof UnclosedAngle) {
    throw unclosedAngleError(v)
  }

  if (typeof v === 'boolean') {
    return newBoolean(v)
  }

  if (v === null || v === undefined) {
    // An untyped-nil element is an EMPTY list slot — `[1,,2]`, `[,1]`,
    // a leading/repeated comma. (An explicit `null` arrives as the
    // jsonic Text token "null", handled above, so this case only fires
    // for a genuinely empty element.) A repeated or leading comma is a
    // typo, not a value — reject it rather than fabricating a `null`.
    throw emptyElementError()
  }

  throw new Error(`unsupported value type ${typeName(v)}`)
}

// emptyElementError is raised when a list literal contains an empty
// element produced by a leading or repeated comma (`[1,,2]`, `[,1]`).
// boru has no implicit "hole" value; commas are optional separators, so a
// missing element is a typo. Use `none` for an explicit empty value.
function emptyElementError(): BoruError {
  return new BoruError(
    'syntax_error',
    'empty list element: remove the leading/repeated comma (write `none` for an explicit empty value)',
  )
}

// isNumberLiteral reports whether a (deSited) jsonic item is a numeric
// literal: an integer arrives from jsonic as a plain number, a decimal as a
// NumberVal (dot-bearing source wrapped by setupNumberSub).
function isNumberLiteral(item: unknown): boolean {
  return typeof item === 'number' || item instanceof NumberVal
}

// numberReceiverError rejects a `.`-access whose receiver is a numeric
// literal. `get`'s receiver is always a container (Map / List / Store /
// Object / module namespace / …) — a number has no members — so `1.2.3` (a
// malformed numeric literal), `1 . 2`, or `5 . foo` can never be a valid
// reach. Caught at parse time instead of surfacing the runtime
// "no matching signature for get".
function numberReceiverError(pos: SrcPos): BoruError {
  return new BoruError('syntax_error', 'a number has no members to access with `.`', pos.src ?? '')
}

// danglingDotError rejects a leading / receiverless `.` or `!.` — the
// prefix dot form (e.g. `.5`, `.name`, `!.x`). Member access must follow
// a value (`m.key`); a leading `.` is never valid. In particular `.5` is
// not a fraction — write `0.5`. The receiverless reach `$.name` is the
// supported way to write a detached accessor.
function danglingDotError(pos: SrcPos): BoruError {
  return new BoruError('syntax_error', '`.` member access has no receiver', pos.src ?? '')
}

// convertWordList converts a list in word context (top-level list).
// The resulting list is marked for auto-evaluation: its contents will
// be executed at the end of Run unless quoted or consumed by a word.
function convertWordList(items: unknown[], d: ParseDepth): Value {
  return newList(convertTopLevelItems(items, d), { eval: true })
}

// convertMapData converts a map in data context. All text values are
// scalar data regardless of quoting. When implicit is true the
// resulting OrderedMap is marked as coming from pair syntax (e.g.,
// [x:Integer] rather than {x:Integer}).
// Explicit maps are marked for auto-evaluation (eval=true).
// The optional meta parameter receives the map node's Meta for optional
// field detection.
function convertMapData(
  m: Record<string, unknown>,
  implicit: boolean,
  d: ParseDepth,
  meta?: Record<string, unknown>,
): Value {
  enterDepth(d)
  try {
    const om = new OrderedMap()
    if (implicit) {
      // The TS OrderedMap declares no Implicit field — the flag rides as
      // an expando property (recorded in deviations).
      ;(om as unknown as Record<string, unknown>)['implicit'] = true
    }
    // Extract metadata from the map node's Meta.
    const qmSet = metaStrSet(meta, 'qm') // optional keys (? syntax)
    const ckSet = metaStrSet(meta, 'ck') // computed keys ([key] syntax)
    const qkSet = metaStrSet(meta, 'qk') // quoted keys ({'k': v} syntax)
    const shList = metaStrList(meta, 'sh') // shorthand keys ({foo} / {foo/r} syntax)
    const ko = metaStrList(meta, 'ko') // source key order (D1: Meta["ko"] channel)

    // Word modifiers (`/r`, `/q`, `/f`, `/s`, `/N`) are legal only on
    // shorthand entries — `{foo/r}` ≡ `{foo: foo/r}` — where the token is
    // also the value, so the modifier qualifies that value. On a bare
    // `key: value` pair the modifier would only ever land on the KEY,
    // which is meaningless (a map key is a plain name). Reject it rather
    // than silently keeping `f/r` as a literal key. A quoted key
    // (`{'f/r': …}`) or computed key (`{[f/r]: …}`) is an explicit string
    // literal — the slash is data, not a modifier — so those are exempt.
    // Bare explicit keys (with and without an optional `?`) arrive in m;
    // shorthand (sh) and value-less optional (qm) tokens are handled below
    // where the modifier legitimately rides along on the synthesized value.
    for (const key of mapKeys(m)) {
      if (qkSet.has(key) || ckSet.has(key)) {
        continue
      }
      if (scanWordModifier(key).valid) {
        throw new BoruError(
          'illegal_key',
          `word modifier not allowed on map key ${goQuote(key)} (modifiers qualify values, not keys; quote the key as '${key}' for a literal slash)`,
        )
      }
    }

    // Shorthand entries: `{foo}` ≡ `{foo: foo}`, `{foo/r}` ≡ `{foo: foo/r}`,
    // `{foo?}` ≡ `{foo?: foo}`, `{foo/r?}` ≡ `{foo?: foo/r}`. The key is
    // always the base name (modifiers stay on the value only); the value
    // is the full word token. synth maps each synthesized base key to that
    // token. Two sources: explicit shorthand tokens (Meta["sh"]) and
    // optional keys (Meta["qm"]) that never received an explicit value.
    //
    // optBase collects the base name of every optional key so optionality
    // is looked up by base name below — qmSet is keyed by the raw token
    // (e.g. `f/r`), which no longer matches the synthesized base key.
    const synth = new Map<string, string>()
    const optBase = new Set<string>()
    for (const raw of shList) {
      synth.set(wordBaseName(raw), raw)
    }
    for (const key of qmSet) {
      const base = wordBaseName(key.endsWith('?') ? key.slice(0, -1) : key)
      optBase.add(base)
      if (!hasOwn(m, key)) {
        synth.set(base, key)
      }
    }

    // Iterate the sorted union of value-map keys and synthesized keys.
    const union = new Set<string>(mapKeys(m))
    for (const k of synth.keys()) {
      union.add(k)
    }

    for (const key of orderedKeys(union, ko)) {
      let child: Value
      if (hasOwn(m, key)) {
        child = convertDataValue(m[key], d)
      } else {
        child = parseWord(synth.get(key)!)
      }
      // Optional field: `?:T` means "None or absent, universally" —
      // desugared to `disjunct(T, None, Absent)`. The Absent
      // alternative carries the "may be missing" half of the rule
      // (the map unifier synthesises Absent when a key is absent);
      // the None alternative carries the "may be explicitly None"
      // half. No separate metadata needed — the optionality lives
      // entirely in the type. For an EXPLICIT record/map (`{a?:T}`), route
      // through SimplifyDisjunctAlts — the same boundary user-facing `tor`
      // unions pass through — so `{a?:T}` and the explicit
      // `{a:(T tor None tor Absent)}` reduce to the SAME canonically-ordered
      // value (otherwise they build order-divergent disjuncts and record
      // equality compares false). An IMPLICIT map is the fn-param list
      // (`[a?:T]`), whose optional-param expansion reads the base type off
      // the FIRST alternative — keep the parser order there.
      let optional = qmSet.has(key) || optBase.has(key)
      let realKey = key
      if (key.endsWith('?')) {
        realKey = key.slice(0, -1)
        optional = true
      }
      if (optional) {
        let alts = [child, newTypeLiteral(TNone), newTypeLiteral(TAbsent)]
        // An unresolved type-name child (a Word, post-ADR-012) cannot
        // be reasoned about here; the engine's resolve prepass
        // re-simplifies the disjunct after resolving the name, which
        // restores the `{a?:T}` ≡ `{a:(T tor None tor Absent)}`
        // canonical equality at every consumption boundary.
        if (!implicit && !child.isWord()) {
          alts = simplifyDisjunctAlts(alts)
        }
        child = newDisjunct(alts)
      }
      om.set(realKey, child)
    }
    // Propagate computed keys to the map's meta for autoEvalMap. (The TS
    // OrderedMap declares no Meta field — an expando carries it; the ck
    // channel is a Set here where Go stores map[string]bool.)
    if (ckSet.size > 0) {
      ;(om as unknown as Record<string, unknown>)['meta'] = { ck: ckSet }
    }
    // Explicit maps (from {...} syntax) are marked for auto-evaluation.
    // Implicit maps (from pair syntax [x:Integer]) are structural and not evaluated.
    if (!implicit) {
      return markEval(newMap(om))
    }
    return newMap(om)
  } finally {
    leaveDepth(d)
  }
}

// simplifyDisjunctAlts is the parser-local stand-in for Go's
// eng.SimplifyDisjunctAlts, exercised here on exactly one shape: the
// optional-field triple [child, None, Absent] where child is NOT a Word.
// For every parser-reachable child the full simplifier's drop/dedupe
// passes are no-ops (a parser child is never a bare type literal, never
// Never, and never a strict subtype of None/Absent), so only the final
// canonical tcmp SORT is observable: None (rank 12e9) before Absent
// (14e9) before everything the parser can produce (Scalar 2e10+ /
// Node / Ideal / Word bands), with the type-literal-first rule breaking
// the None-vs-`none` tie. This local sort reproduces that order without
// the kernel's lattice machinery (see deviations).
function simplifyDisjunctAlts(alts: Value[]): Value[] {
  const rank = (v: Value): number => {
    if (v.vType.equal(TNone)) return v.data === null ? 0 : 1
    if (v.vType.equal(TAbsent)) return 2
    return 3
  }
  return [...alts].sort((a, b) => rank(a) - rank(b))
}

// convertDataValue converts a value in data context (inside maps).
// Quoted text → strings, unquoted text → words (executable). The value is
// stamped with the source position captured by the val-rule BC (if any).
function convertDataValue(v: unknown, d: ParseDepth): Value {
  const [node, pos] = deSite(v)
  return withPos(convertDataValueInner(node, d), pos)
}

export function convertDataValueInner(v: unknown, d: ParseDepth): Value {
  if (v instanceof ArrowTag) {
    // `=>` in data context — the same lambda sugar marker.
    return newSugar({ kind: 'lambda' })
  }
  const text = asText(v)
  if (text !== undefined) {
    if (text.quote !== '') {
      // Quoted text (e.g. "hello") → string
      return newString(text.str)
    }
    // Unquoted text → word (same as top-level word context).
    // This allows map values like {r:rv} to evaluate rv.
    return parseWord(text.str)
  }

  if (v instanceof InterpGroup) {
    return convertInterpGroup(v, d)
  }

  if (v instanceof MiniLitVal) {
    // `+name<delim>src<delim>` desugars to the splice `mini name 'src'`
    // (the `word` mechanism): one value that re-steps as the standard
    // mini call, so a stack subject, a trailing opts Map, the
    // unknown-kind error, and check mode all behave exactly as if the
    // user had typed `mini name 'src'`.
    return newSugar({ kind: 'mini', name: v.name, src: v.src })
  }

  if (v instanceof XmlElemVal) {
    // Embedded XML literal in data context (a map value): same
    // immutable Node/Xml value the matcher built.
    if (v.err !== undefined) {
      throw new Error(v.err)
    }
    return v.v!
  }

  if (v instanceof NumberVal) {
    return numberValToValue(v)
  }

  if (typeof v === 'number') {
    return floatToValue(v)
  }

  if (isMapNode(v)) {
    const info = getInfo(v)
    if (info !== undefined) {
      // Go's jsonic.MapRef case.
      if (hasMapChild(v)) {
        return convertTypedMap(v, d, info.meta)
      }
      return convertMapData(v, !!info.implicit, d, info.meta)
    }
    // Go's raw map[string]any case.
    if (hasMapChild(v)) {
      return convertTypedMap(v, d)
    }
    return convertMapData(v, true, d)
  }

  if (Array.isArray(v)) {
    if ((v as Record<string, unknown> & unknown[])['child$'] !== undefined) {
      return convertTypedList(v, d)
    }
    return convertDataList(v, d)
  }

  if (v instanceof UnclosedParen) {
    throw new BoruError('syntax_error', 'unmatched opening parenthesis', '(')
  }

  if (v instanceof ParenGroup) {
    // Paren group in data context: convert items in word context
    // and wrap as a ParenExpr for inline evaluation by autoEvalMap.
    return newParenExpr(convertTopLevelItems(v.items, d))
  }

  if (v instanceof AngleGroup) {
    // Use-site sugar in data context — `{x: Box<Integer>}`, the
    // `[:Box<Integer>]` child constraint, `b:Box<Integer>` typed
    // annotations — becomes the same ParenExpr the canonical
    // `(Box of [Integer])` produces here.
    return angleUseSite(v, d)
  }

  if (v instanceof UnclosedAngle) {
    throw unclosedAngleError(v)
  }

  if (typeof v === 'boolean') {
    return newBoolean(v)
  }

  if (v === null || v === undefined) {
    // An untyped-nil element is an EMPTY list slot — `[1,,2]`, `[,1]`,
    // a leading/repeated comma. (An explicit `null` arrives as the
    // jsonic Text token "null", handled above, so this case only fires
    // for a genuinely empty element.) A repeated or leading comma is a
    // typo, not a value — reject it rather than fabricating a `null`.
    throw emptyElementError()
  }

  throw new Error(`unsupported value type ${typeName(v)}`)
}

// convertTypedList converts a list node with a child$ into a typed list value.
// The child value is converted in data context (type names resolve to type literals).
//
// When the list carries concrete elements alongside the child constraint
// (`[v0 :T v1]`), each element is converted in data context and
// retained on the resulting Value's ChildType.elements; the
// runtime `is` validates them against child on demand.
function convertTypedList(arr: unknown[], d: ParseDepth): Value {
  enterDepth(d)
  try {
    const childVal = convertDataValue((arr as Record<string, unknown> & unknown[])['child$'], d)
    if (arr.length === 0) {
      return newTypedList(childVal)
    }
    const elems: Value[] = []
    for (let i = 0; i < arr.length; i++) {
      elems.push(convertDataValue(arr[i], d))
    }
    return newTypedList(childVal, elems)
  } finally {
    leaveDepth(d)
  }
}

// hasMapChild reports whether a jsonic map contains the "child$" key
// set by the map.child option (bare colon syntax {:value}).
function hasMapChild(m: Record<string, unknown>): boolean {
  return hasOwn(m, 'child$')
}

// convertTypedMap converts a map with a "child$" key into a typed
// map value. The child value is converted in data context (type
// names resolve to type literals).
//
// When the source carries concrete entries alongside the child
// constraint (`{k:v :T}`), each entry is converted in data context
// and retained on the resulting Value's ChildType (as an `entries`
// expando — the TS ChildType declares no entries field; see
// deviations); the runtime `is` validates each entry's value against
// child on demand.
function convertTypedMap(
  m: Record<string, unknown>,
  d: ParseDepth,
  meta?: Record<string, unknown>,
): Value {
  enterDepth(d)
  try {
    const childVal = convertDataValue(m['child$'], d)
    // Collect non-`child$` entries as concrete values, in SOURCE order
    // (D1 — design/FLEX-ATTRS.1.md §3). The `child$` constraint is not a
    // pair, so the order channel never records it; drop it from the union
    // before ordering.
    const ko = metaStrList(meta, 'ko')
    const union = new Set<string>(mapKeys(m).filter((k) => k !== 'child$'))
    if (union.size === 0) {
      return newTypedMap(childVal)
    }
    const entries: { key: string; value: Value }[] = []
    for (const k of orderedKeys(union, ko)) {
      entries.push({ key: k, value: convertDataValue(m[k], d) })
    }
    return newTypedMap(childVal, entries)
  } finally {
    leaveDepth(d)
  }
}

// angleUseSite converts an angle group to ONE structural sugar marker
// (ADR-012 amendment) carrying both precomputed forms: the use-site
// type args (items) and the generic-def head params (head, or headErr
// when the items are not a valid param list). The engine picks the
// form at dispatch. Each item converts in word context; nested sugar
// (`Box<Pair<A, B>>`) recurses through the AngleGroup case.
function angleUseSite(ag: AngleGroup, d: ParseDepth): Value {
  const args: Value[] = []
  for (const it of ag.items) {
    args.push(convertTopLevelValue(it, d))
  }
  // Both forms are precomputed; the ENGINE picks (ADR-012 rule 3
  // amendment): a marker arriving at a binder's /q name slot lowers
  // to the generic-def head (`Name gen [params]`), anywhere else to
  // the use-site apply. A head form whose items are not a valid
  // param list carries the error text instead — it surfaces only if
  // the head form is actually selected.
  const info: SugarInfo = { kind: 'angle', name: ag.name, items: args }
  try {
    info.head = angleGenList(ag.items, UNKNOWN_POS, d)
  } catch (herr) {
    info.headErr = errMessage(herr)
  }
  return newSugar(info)
}

// angleGenList desugars an angle group's items to the canonical
// generic-def head parameter list (the marker's precomputed head,
// used only if the engine selects the head form):
//
//	T                 → Word(T)
//	T extends C       → ParenExpr[ Word(T) Word(extends) C ]
//	T = D             → ParenExpr[ Word(T) sugar(gen-default) D ]
//
// `extends` rides as the user-written word itself; `=` is not a legal
// word name, so it rides as the gen-default sugar marker.
//
// Items arrive as a flat val sequence (commas are optional separators,
// like list elements), so entries are recognised by the grammar above:
// a capitalised name, optionally followed by an `extends`/`=` operator
// and its operand.
function angleGenList(items: unknown[], pos: SrcPos, d: ParseDepth): Value {
  const entries: Value[] = []
  let i = 0
  while (i < items.length) {
    const [node, epos] = deSite(items[i])
    const txt = asText(node)
    if (
      txt === undefined ||
      txt.quote !== '' ||
      txt.str === '' ||
      !(txt.str[0]! >= 'A' && txt.str[0]! <= 'Z')
    ) {
      throw new BoruError(
        'syntax_error',
        'generic parameter must be a capitalised name',
        epos.src ?? '',
      )
    }
    const name = txt.str
    if (i + 1 < items.length) {
      const [op] = deSite(items[i + 1])
      const opTxt = asText(op)
      if (opTxt !== undefined && opTxt.quote === '') {
        let kw = ''
        if (opTxt.str === 'extends') {
          kw = 'extends'
        } else if (opTxt.str === '=') {
          kw = 'default'
        }
        if (kw !== '') {
          if (i + 2 >= items.length) {
            throw new BoruError(
              'syntax_error',
              `generic parameter ${name}: ${opTxt.str} needs a value`,
              epos.src ?? '',
            )
          }
          // (In Go this conversion carries a covergate note: angleUseSite's
          // items loop converts every raw item with the same converter
          // before angleGenList runs, so a failing operand errors there
          // first.)
          const operand = convertTopLevelValue(items[i + 2], d)
          let opVal = newWord(opTxt.str)
          if (kw === 'default') {
            // `=` is not a legal word name — it rides as
            // the gen-default sugar marker; `extends` is
            // the user-written word itself.
            opVal = newSugar({ kind: 'gen-default' })
          }
          entries.push(withPos(newParenExpr([newWord(name), opVal, operand]), epos))
          i += 3
          continue
        }
      }
    }
    entries.push(withPos(newWord(name), epos))
    i++
  }
  return withPos(newList(entries, { eval: true }), pos)
}

// unclosedAngleError is raised for an angle group auto-closed at EOF.
function unclosedAngleError(ua: UnclosedAngle): BoruError {
  return new BoruError('syntax_error', 'unclosed angle bracket: ' + ua.name + '<… has no matching `>`')
}

// convertDataList converts a list in data context (inside maps).
// Lists use word context and are marked for auto-evaluation.
function convertDataList(items: unknown[], d: ParseDepth): Value {
  return newList(convertTopLevelItems(items, d), { eval: true })
}

// resolveTextValue converts a bare text string into the appropriate
// boru value — boolean, float special, or atom.
// Unquoted text is never a string; only quoted text produces strings.
export function resolveTextValue(text: string): Value {
  if (text === 'true') {
    return newBoolean(true)
  }
  if (text === 'false') {
    return newBoolean(false)
  }
  switch (text) {
    case 'inf':
      return newFloat(Infinity)
    case '-inf':
      return newFloat(-Infinity)
    case 'nan':
      return newFloat(NaN)
  }
  // Type names are not resolved (ADR-012 rule 4 — parseWord has the
  // same rule); a capitalised name is data here, an Atom.
  return newAtom(text)
}

// orderedKeys returns the keys of a key set in SOURCE order — the D1
// insertion-order default (design/FLEX-ATTRS.1.md §3). `ko` is the source
// key order captured by the grammar's `Meta["ko"]` channel
// (grammar.ts): keys present in `ko` are emitted in that order (first-
// position/last-value — a repeated literal key keeps its first slot,
// matching OrderedMap.set), then any remaining union keys the channel did
// not cover are appended in sorted order.
//
// The sorted tail is the determinism fallback for map values that reach
// this path WITHOUT an order channel — where Go's unordered map made
// source order unrecoverable, sorted was the only stable choice; the TS
// port keeps the same rule so both parsers emit identical key orders.
// When `ko` covers every key (the common literal case) the tail is empty
// and output is pure source order.
function orderedKeys(union: Set<string>, ko: string[]): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const k of ko) {
    if (seen.has(k)) {
      continue
    }
    if (union.has(k)) {
      out.push(k)
      seen.add(k)
    }
  }
  const rest: string[] = []
  for (const k of union) {
    if (!seen.has(k)) {
      rest.push(k)
    }
  }
  // One library sort call, ascending — mirrors Go's sort.Strings choice
  // (and its ADR-008 determinism rationale).
  rest.sort()
  return out.concat(rest)
}

// ── word-modifier scanning ──────────────────────────────────────────────────

// The decoded `/...` modifier suffix of an unquoted word token. Mirrors
// scanWordModifier's Go multi-return as one record.
interface WordMod {
  base: string
  argCount: number
  forceStack: boolean
  forceForward: boolean
  quoteFlag: boolean
  refFlag: boolean
  usurpFlag: boolean
  typeFlag: boolean
  valid: boolean
}

// scanWordModifier parses the optional `/...` modifier suffix of an
// unquoted word token: name/f (forceForward), name/s (forceStack),
// name/N (argCount), name/q (quote → Atom), name/r (ref → bound value),
// name/u (usurp → reversed-sig wrapper), and combinations like name/1f,
// name/qs, name/f2, name/ur. Modifiers stack in any order; f and s are
// mutually exclusive; q is mutually exclusive with both r and u (an atom
// has no binding to reference or usurp); u may combine with r (name/ur);
// the argCount digits form a single number. When the token has no `/` or a
// malformed modifier suffix, valid is false, base is the whole text, and
// every flag is at its zero value (argCount -1).
export function scanWordModifier(text: string): WordMod {
  const invalid = (): WordMod => ({
    base: text,
    argCount: -1,
    forceStack: false,
    forceForward: false,
    quoteFlag: false,
    refFlag: false,
    usurpFlag: false,
    typeFlag: false,
    valid: false,
  })

  const idx = text.lastIndexOf('/')
  if (idx < 0 || idx >= text.length - 1) {
    return invalid()
  }
  const mod = text.slice(idx + 1)
  const baseName = text.slice(0, idx)

  // Scan modifier chars in any order: digits, 'f', 's', 'q', 'r', 'u',
  // 't'. Each letter appears at most once; f/s are mutually exclusive;
  // q is mutually exclusive with r and u; t (the type-bound sugar,
  // `Map/t` ≡ `(Type of [Map])`) combines with nothing — it produces a
  // type expression, not a word; digits run contiguously and form a
  // single argCount value.
  let valid = true
  let seenDigits = false
  let argCount = -1
  let forceStack = false
  let forceForward = false
  let quoteFlag = false
  let refFlag = false
  let usurpFlag = false
  let typeFlag = false
  let i = 0
  while (i < mod.length) {
    const c = mod[i]!
    if (c >= '0' && c <= '9') {
      if (seenDigits) {
        valid = false
      } else {
        let j = i
        while (j < mod.length && mod[j]! >= '0' && mod[j]! <= '9') {
          j++
        }
        // Go bounds the run with strconv.Atoi's int (64-bit) range; the
        // same bound keeps the valid/invalid split identical.
        const n = BigInt(mod.slice(i, j))
        if (n > INT64_MAX) {
          valid = false
        } else {
          argCount = Number(n)
          seenDigits = true
        }
        i = j
        if (!valid) {
          break
        }
        continue
      }
    } else if (c === 'f') {
      if (forceForward || forceStack) {
        valid = false
      } else {
        forceForward = true
      }
    } else if (c === 's') {
      if (forceForward || forceStack) {
        valid = false
      } else {
        forceStack = true
      }
    } else if (c === 'q') {
      if (quoteFlag || refFlag || usurpFlag) {
        valid = false
      } else {
        quoteFlag = true
      }
    } else if (c === 'r') {
      if (refFlag || quoteFlag) {
        valid = false
      } else {
        refFlag = true
      }
    } else if (c === 'u') {
      if (usurpFlag || quoteFlag) {
        valid = false
      } else {
        usurpFlag = true
      }
    } else if (c === 't') {
      if (typeFlag) {
        valid = false
      } else {
        typeFlag = true
      }
    } else {
      valid = false
    }
    if (!valid) {
      break
    }
    i++
  }

  // /t combines with nothing — any companion flag invalidates, in
  // either order.
  if (typeFlag && (quoteFlag || refFlag || usurpFlag || forceStack || forceForward || argCount >= 0)) {
    valid = false
  }
  if (!valid) {
    // Unrecognized / malformed modifier — treat entire token as plain word.
    // parseWord upgrades the all-modifier-alphabet subset of this case
    // (a botched modifier like foo/fs, never a legitimate name) to a
    // loud parse error — see isModifierAlphabet (NUR027).
    return invalid()
  }
  return {
    base: baseName,
    argCount,
    forceStack,
    forceForward,
    quoteFlag,
    refFlag,
    usurpFlag,
    typeFlag,
    valid: true,
  }
}

// isModifierAlphabet reports whether every character of a candidate
// modifier suffix is drawn from the modifier alphabet (digits plus
// f s q r u t). It is the line between a BOTCHED MODIFIER (`foo/fs`,
// `foo/qr`, `foo/1f2` — all-alphabet but an invalid combination),
// which errors loudly, and a slash-bearing plain name (`foo/bar`,
// `add/x`, the builtin type paths `Scalar/Number/Integer` — the
// suffix contains a non-modifier character, uppercase included),
// which stays a single plain word exactly as before. Valid modifier
// combinations never reach this: scanWordModifier claims them first.
function isModifierAlphabet(s: string): boolean {
  for (let i = 0; i < s.length; i++) {
    const c = s[i]!
    if (c >= '0' && c <= '9') continue
    if (c === 'f' || c === 's' || c === 'q' || c === 'r' || c === 'u' || c === 't') continue
    return false
  }
  return true
}

// wordBaseName returns the base name of an unquoted word token, stripping
// a valid `/...` modifier suffix (foo/r → foo, foo → foo). Used to derive
// the key of a shorthand map entry, whose value keeps the full token.
function wordBaseName(text: string): string {
  return scanWordModifier(text).base
}

// parseWord interprets an unquoted text token as a boru word, handling
// the modifier syntax decoded by scanWordModifier. q produces an Atom and
// overrides the other modifiers; u emits a usurp-word and r emits a
// ref-word, both of which short-circuit the rest.
export function parseWord(text: string): Value {
  const m = scanWordModifier(text)
  const name = m.base

  if (name === '') {
    throw new Error('empty word')
  }

  // An invalid modifier combination spelled entirely from the modifier
  // alphabet is a PARSE ERROR (NUR027) — previously the whole token
  // silently became a slash-bearing word that surfaced later as an
  // obscure undefined_word. A suffix containing any non-modifier
  // character keeps the historical plain-word reading (slash-bearing
  // names, builtin type paths).
  if (!m.valid) {
    const idx = text.lastIndexOf('/')
    if (idx >= 0 && idx < text.length - 1 && isModifierAlphabet(text.slice(idx + 1))) {
      throw new BoruError(
        'syntax_error',
        `invalid word modifier /${text.slice(idx + 1)} on ${goQuote(text.slice(0, idx))}`,
        text,
      )
    }
  }

  // `X/t` — the type-bound sugar: desugars to the paren group
  // `(Type of [X/q])`, the bounded-Type application (≡ `Type<X>`).
  // X rides as an ATOM inside the argument list: it survives list
  // auto-eval untouched, so `of` sees the NAME and can bind a named
  // structural type (a disjunct, a record) to its MINTED lattice
  // node — a word would resolve to the body first. Resolution
  // happens entirely inside `of`; a non-type X fails there with
  // of's error — the parser owns no semantics.
  if (m.typeFlag) {
    const bound = newList([newAtom(name)])
    return newSugar({ kind: 'type-bound', items: [bound] })
  }

  // `0d…` arbitrary-precision literals (BigInteger / BigDecimal). The
  // lexer matcher claims the whole run as one text token (so an embedded
  // `.` isn't split off); here we build the value. Checked early — these
  // never take modifiers and must not fall through to word/number paths.
  if (isBigNumberLiteral(name)) {
    return parseBigNumber(name)
  }

  // /q produces an Atom: equivalent to (quote name). Other modifiers
  // in the same suffix are accepted but ignored — the atom is data,
  // not a function call.
  if (m.quoteFlag) {
    return newAtom(name)
  }

  // /u emits a usurp-word that resolves the name to its bound Function
  // value and wraps it with reversed signature arg order. Legal only for
  // function words (illegal_ref at run time otherwise). It may combine
  // with /r: /u alone dispatches the wrapper, /ur leaves it as data.
  // Argument-shape modifiers don't apply (the wrapper supplies its own).
  if (m.usurpFlag) {
    return newWordUsurp(name, m.refFlag)
  }

  // /r emits a ref-word that, when reached at the pointer, resolves the
  // name to its bound Function value without invoking. /r is legal only
  // for function words; a non-fn binding raises illegal_ref at run time
  // (the parser accepts the syntax — the binding kind isn't known until
  // resolution). Argument-shape modifiers don't apply because ref
  // bypasses dispatch entirely; they're accepted syntactically but
  // ignored.
  if (m.refFlag) {
    return newWordRef(name)
  }

  if (m.forceStack || m.forceForward || m.argCount >= 0) {
    return newWordModified(name, m.argCount, m.forceStack, m.forceForward)
  }

  // `none` is the unique inhabitant of None — a value, not a type.
  // `null` is the JSON-null atom (handled in data context via the
  // null/undefined case; here in word context bare `null` falls through
  // as a Word for engine-side resolution).
  if (name === 'none') {
    return newNone()
  }

  // IEEE-754 special-value literals, following the boru reserved-literal
  // convention (lowercase, parser-emitted, like true / false / none):
  // `inf` / `-inf` / `nan` produce the corresponding Float. They render
  // back to these same tokens (see FormatFloat), so print∘parse is
  // identity. `inf negate` is the long form for -inf.
  switch (name) {
    case 'inf':
      return newFloat(Infinity)
    case '-inf':
      return newFloat(-Infinity)
    case 'nan':
      return newFloat(NaN)
  }

  // Reserved tape-syntax tokens emit typed marker values so the
  // engine recognises them by Parent identity (parens, end / ';').
  // These would otherwise become plain Word values that the engine
  // would have to name-dispatch in stepWord.
  switch (name) {
    case 'end':
      return newEnd()
    case ')':
      return newCloseParen()
  }

  // Type names are NOT resolved here — the parser is type-name-opaque
  // (ADR-012 rule 4): a capitalised name is an ordinary Word in every
  // context, and the engine's canonical cascade (eng/go/resolve.go)
  // resolves it at consumption, exactly as user-defined type names
  // have always resolved. Quotation meaning is consumption-time.

  // A base-prefixed integer token whose magnitude jsonic could not lex
  // as a number (>= 2^63) arrives here as bare text. Parse it exactly:
  // `-0x8000000000000000` (= int64 min) succeeds, while a truly
  // out-of-range magnitude becomes a clean [boru/integer_overflow]
  // instead of an opaque undefined_word. (In-range base-prefixed
  // literals never reach this path — jsonic lexes them as numbers.)
  if (isBasePrefixedInteger(name)) {
    if (name.includes('_') && !validUnderscores(name)) {
      throw new BoruError('syntax_error', 'misplaced `_` in numeric literal: ' + name, name)
    }
    const n = tryParseBigIntBase0(stripUnderscores(name))
    if (n !== undefined) {
      // Go's ParseInt reports overflow as a range error; bigint is
      // unbounded, so the int64 range check plays that role.
      if (n < INT64_MIN || n > INT64_MAX) {
        throw integerLiteralOverflowError(name, 0, 0)
      }
      return newInteger(n)
    }
    // Other parse errors (e.g. an invalid digit) fall through to the
    // numeric-shape classification below.
  }

  // A digit-led token reached here only because jsonic could not lex it
  // as a number — and no word may start with a digit (see
  // ValidateWordName), so it can only be a malformed or out-of-range
  // numeric literal. Classify it for a clear diagnostic instead of an
  // opaque "undefined word": a value the float parser recognises but
  // overflows binary64 to infinity; anything else is malformed (e.g.
  // `1e`, `2dup` — the renamed-away stack words are now `dup2` etc.).
  if (isDigitLed(name)) {
    // The two jsonic ports disagree about which underscore-bearing tokens
    // are NUMBERS: the Go port lexes `1_` and `1_e5`, so Go classifies them
    // in numberValToValue; the TS package declines both and drops them
    // here. So this fallback has to reproduce what Go's lexer + classifier
    // jointly decide, and the ORDER is what makes it agree:
    //
    //   1e400_  overflow first  — Go's fallback reports float_overflow,
    //                             not a separator error, for a trailing `_`
    //                             on an out-of-range literal
    //   1_      misplaced `_`   — invalid placement, and the token is
    //                             otherwise a number
    //   1_+     malformed       — invalid placement, but stripping the `_`
    //                             still leaves a non-number
    //   1_e5    the VALUE       — `_` between two alnums is legal
    //                             (validUnderscores is byte-identical on
    //                             both sides), and Go returns 100000 here
    //
    // Only underscore-bearing tokens are valued: everything else that
    // reaches this fallback is a token Go's lexer also declined.
    const stripped = stripUnderscores(name)
    const f = Number(stripped)
    if (f === Infinity || f === -Infinity) {
      throw floatLiteralOverflowError(name)
    }
    if (name.includes('_')) {
      if (!validUnderscores(name)) {
        if (Number.isNaN(f)) {
          throw malformedNumberError(name)
        }
        throw new BoruError(
          'syntax_error',
          'misplaced `_` in numeric literal: ' + name,
          name,
        )
      }
      // Only a plain DECIMAL literal is valued. Number() also accepts the
      // base prefixes (`0x1` -> 1), which Go's ParseFloat does not, so
      // without this shape gate `0_x1` — invalid on both engines — came
      // back as 1 here while Go reported a malformed literal.
      if (!Number.isNaN(f) && /^[+-]?\d+(\.\d+)?([eE][+-]?\d+)?$/.test(stripped)) {
        return floatToValue(f)
      }
    }
    throw malformedNumberError(name)
  }

  return newWord(name)
}

// isDigitLed reports whether s starts (after an optional sign) with a
// decimal digit. Because no word may begin with a digit
// (ValidateWordName), a digit-led token is always a numeric literal —
// never an identifier.
function isDigitLed(s: string): boolean {
  if (s.length > 0 && (s[0] === '-' || s[0] === '+')) {
    s = s.slice(1)
  }
  return s.length > 0 && s[0]! >= '0' && s[0]! <= '9'
}

// (Go's isRangeError has no TS twin: BigInt is unbounded and Number
// saturates to ±Infinity, so overflow is detected by explicit range
// checks / infinity probes at each numeric call site instead.)

// floatLiteralOverflowError reports a floating-point literal whose
// magnitude overflows binary64 to ±infinity (e.g. 1e309).
function floatLiteralOverflowError(src: string): BoruError {
  return new BoruError(
    'float_overflow',
    'floating-point literal out of range: ' + src + ' overflows to infinity',
    src,
  )
}

// malformedNumberError reports a digit-led token that is not a valid
// number. Since no word may start with a digit, such a token can only be
// a botched numeric literal (`1e`, `0x1p4`) or a digit-first name (`2dup`
// — the stack words are now `dup2`/`swap2`/`drop2`/`over2`).
function malformedNumberError(src: string): BoruError {
  return new BoruError('syntax_error', 'invalid numeric literal: ' + src, src)
}

// convertInterpGroup converts an InterpGroup (produced by the interp/ielem/iexpr
// jsonic rules) into an engine InterpString value, or a plain string if there
// are no expression parts.
export function convertInterpGroup(grp: InterpGroup, d: ParseDepth): Value {
  if (grp.parts.length === 0) {
    return newString('')
  }
  const parts: InterpSegment[] = []
  let hasExpr = false
  for (const raw of grp.parts) {
    const [item] = deSite(raw) // interp parts are not normally sited; guard anyway
    const t = asText(item)
    if (t !== undefined) {
      // Template literal segment (quote "tl").
      parts.push({ lit: t.str })
      continue
    }
    if (item instanceof IexprGroup) {
      hasExpr = true
      let exprVals: Value[]
      try {
        exprVals = convertTopLevelItems(item.items, d)
      } catch (err) {
        // Go wraps with `fmt.Errorf("…: %w", err)`, which renders the
        // INNER error's own text — `[boru/float_overflow]: …` — after
        // the prefix, and leaves the top-level error an ordinary wrapped
        // one. An earlier version re-raised a BoruError carrying the
        // inner CODE with the prefix moved onto the detail, which
        // rendered the two halves in the opposite order:
        //
        //   go: interpolation expression error: [boru/float_overflow]: …
        //   ts: [boru/float_overflow]: interpolation expression error: …
        //
        // Matching Go means the thrown error must render like Go's, so
        // it is a plain Error over the inner error's rendered text. The
        // taxonomy survives on `cause`, which is what errors.As unwraps
        // to on the Go side — a caller that needs the code still has it,
        // and the rendered text is identical.
        throw Object.assign(new Error(`interpolation expression error: ${errMessage(err)}`), {
          cause: err,
        })
      }
      parts.push({ expr: exprVals })
      continue
    }
    throw new Error(`unexpected interp part type ${typeName(item)}`)
  }
  if (!hasExpr) {
    // No interpolations — just concatenate literals into a plain string.
    let buf = ''
    for (const p of parts) {
      if ('lit' in p) {
        buf += p.lit
      }
    }
    return newString(buf)
  }
  return newInterpString(parts)
}

// processTemplateEscapes processes escape sequences in template literal text.
// (Exported for the grammar layer — the Go twin lives in parse.go but is
// called from grammar.go's template-literal matcher.)
export function processTemplateEscapes(s: string): string {
  if (!s.includes('\\')) {
    return s
  }
  let buf = ''
  for (let i = 0; i < s.length; i++) {
    if (s[i] === '\\' && i + 1 < s.length) {
      const next = s[i + 1]!
      switch (next) {
        case 'n':
          buf += '\n'
          break
        case 't':
          buf += '\t'
          break
        case 'r':
          buf += '\r'
          break
        case '\\':
          buf += '\\'
          break
        case '`':
          buf += '`'
          break
        case '$':
          buf += '$'
          break
        default:
          // Unknown escape: keep as-is.
          buf += '\\' + next
      }
      i++ // skip the escaped char
    } else {
      buf += s[i]!
    }
  }
  return buf
}

// (Go's numberVal struct is the NumberVal class in nodes.ts: a number
// wrapped with its source text and source position so we can (1)
// distinguish integer literals ("5") from decimal literals ("5.0"),
// (2) parse integers from their exact digits, and (3) locate an
// out-of-range integer literal in the source. Injected by the jsonic
// Sub callback for every number token — setupNumberSub in grammar.ts.)

// floatToValue converts a jsonic number to the appropriate boru numeric value.
// Whole numbers become integers; fractional values become decimals.
export function floatToValue(f: number): Value {
  // Mirrors Go `f == float64(int64(f))`: integral and inside the int64
  // range (both bounds exactly representable in binary64).
  if (Number.isFinite(f) && f === Math.trunc(f) && f >= -9223372036854775808 && f < 9223372036854775808) {
    return newInteger(BigInt(f))
  }
  return newFloat(f)
}

// numberValToValue converts a NumberVal (number + source) to the
// appropriate boru numeric value.
//
//   - A source containing "." is always a Float (even whole-valued, e.g.
//     5.0), using the number jsonic produced.
//   - A *plain decimal integer* source (optional leading '-', then digits
//     and '_' separators only — no base prefix, no exponent) is parsed
//     from its exact digits. This is the fix for WAT Exhibit K: routing
//     through binary64 silently corrupts any integer above 2^53 (e.g.
//     9007199254740993 → ...992) and turns near-int64-max literals into
//     Floats. An out-of-int64-range literal now raises
//     [boru/integer_overflow] instead of silently degrading.
//   - Everything else (scientific notation like 1e3, or a base prefix
//     like 0x10 / 0o17 / 0b101) keeps the existing number-derived path,
//     which already matches jsonic's own base interpretation.
//
// See design/INTEGER-OVERFLOW-STRATEGY.5.md.
export function numberValToValue(nv: NumberVal): Value {
  // `_` is a single digit-separator only: it must sit between two
  // digits (no leading, trailing, or repeated underscores). `1__0` and
  // `1_` are rejected.
  if (nv.src.includes('_') && !validUnderscores(nv.src)) {
    throw underscoreError(nv)
  }
  if (nv.src.includes('.')) {
    // A leading `.` (with or without a sign) is not a valid number —
    // `.5`, `-.5`, `+.5` are rejected; write `0.5` / `-0.5`. The bare
    // `.5` is caught earlier as a stray `.` token; the signed forms
    // reach here because jsonic lexes them as one number token.
    if (isLeadingDotFloat(nv.src)) {
      throw leadingDotNumberError(nv)
    }
    // Parse the Float from its exact source digits for a guaranteed
    // correctly-rounded (round-ties-to-even) decimal→binary64
    // conversion, rather than trusting the number jsonic produced.
    // Number() is IEEE-correct; fall back to jsonic's value
    // only if the (jsonic-validated) token somehow fails to reparse.
    const f = Number(stripUnderscores(nv.src))
    // A literal whose magnitude overflows binary64 is REJECTED, not
    // silently folded to ±inf — Go raises float_overflow here via
    // strconv's ErrRange (parse.go floatLiteralOverflowError), and
    // `1e400` evaluating to `inf` in one engine and refusing in the other
    // is a disagreement about what the language accepts, not a rendering
    // difference. The digit-led fallback below already had this check;
    // this path never reached it because jsonic lexes `1e400` as a
    // perfectly good number whose value happens to be Infinity.
    if (f === Infinity || f === -Infinity) {
      throw floatLiteralOverflowError(nv.src)
    }
    if (!Number.isNaN(f)) {
      return newFloat(f)
    }
    return newFloat(nv.val)
  }
  if (isPlainDecimalInteger(nv.src)) {
    const n = tryParseBigIntBase0(stripUnderscores(nv.src))
    if (n === undefined || n < INT64_MIN || n > INT64_MAX) {
      throw integerLiteralOverflowError(nv.src, nv.row, nv.col)
    }
    return newInteger(n)
  }
  if (isBasePrefixedInteger(nv.src)) {
    // The 0x / 0o / 0b prefix is auto-detected (Go's ParseInt base 0);
    // parsing the exact digits keeps full precision above 2^53 and
    // range-checks like the decimal path (out of int64 range →
    // integer_overflow).
    const n = tryParseBigIntBase0(stripUnderscores(nv.src))
    if (n === undefined || n < INT64_MIN || n > INT64_MAX) {
      throw integerLiteralOverflowError(nv.src, nv.row, nv.col)
    }
    return newInteger(n)
  }
  // The scientific-notation tail (`1e3`, `1e400`). Same overflow refusal as
  // the fractional branch above, and it has to live HERE rather than at the
  // top of this function: an oversized PLAIN INTEGER (`999…`, 300 digits)
  // also reads as Infinity through Number(), but Go raises integer_overflow
  // for it, not float_overflow, and the integer branches above already
  // classify it correctly.
  if (nv.val === Infinity || nv.val === -Infinity) {
    throw floatLiteralOverflowError(nv.src)
  }
  return floatToValue(nv.val)
}

// tryParseBigIntBase0 parses an integer literal with an optional sign and
// an optional 0x / 0o / 0b base prefix (Go's strconv.ParseInt base-0
// contract, minus the bit-size bound — callers range-check). undefined
// for invalid digits.
export function tryParseBigIntBase0(s: string): bigint | undefined {
  let sign = 1n
  let body = s
  if (body.startsWith('-')) {
    sign = -1n
    body = body.slice(1)
  } else if (body.startsWith('+')) {
    body = body.slice(1)
  }
  if (body === '') {
    return undefined
  }
  try {
    return sign * BigInt(body)
  } catch {
    return undefined
  }
}

// convertParsedNumber converts a jsonic parse-result element that the
// number-Sub wrapped in a NumberVal (see setupNumberSub) into its boru
// numeric Value, preserving the int/float distinction carried by the
// source text. Returns undefined for any non-NumberVal input so the
// caller falls through to its default conversion. This is the single
// public seam data-decode paths use (Go's ConvertParsedNumber).
export function convertParsedNumber(v: unknown): Value | undefined {
  if (v instanceof NumberVal) {
    return numberValToValue(v)
  }
  return undefined
}

// isBigNumberLiteral reports whether src is a `0d`/`0D`-prefixed literal
// (optional leading sign). The lexer matcher claims these as one text
// token; parseWord routes them here.
function isBigNumberLiteral(src: string): boolean {
  let s = src
  if (s.length > 0 && (s[0] === '-' || s[0] === '+')) {
    s = s.slice(1)
  }
  return s.length >= 3 && s[0] === '0' && (s[1] === 'd' || s[1] === 'D')
}

// parseBigNumber converts a `0d…` literal string to a BigInteger or
// BigDecimal. A `.` or exponent makes it a BigDecimal (binary64-parsed
// here — TS has no apd; see deviations); otherwise a BigInteger (bigint,
// unbounded). Sign and `_` separators are handled like the int/float
// paths. Errors carry no source position (parseWord has none — mirrors
// the other parseWord numeric errors).
export function parseBigNumber(src: string): Value {
  if (src.includes('_') && !validUnderscores(src)) {
    throw bigLiteralError(src)
  }
  let body = src
  let sign = ''
  if (body.length > 0 && (body[0] === '-' || body[0] === '+')) {
    sign = body[0]!
    body = body.slice(1)
  }
  const digits = stripUnderscores(body.slice(2)) // body[0:2] is the 0d / 0D prefix
  if (digits === '') {
    throw bigLiteralError(src)
  }
  const text = sign + digits
  if (/[.eE]/.test(digits)) {
    // Parsed from the exact source digits — never through binary64, which
    // is what used to turn `0d1e400` into Infinity and `0d1e-400` into 0.
    const d = decimalFromString(text)
    if (undefined === d) {
      throw bigLiteralError(src)
    }
    return newBigDecimal(d)
  }
  // Go's big.Int SetString(text, 10): sign plus decimal digits only.
  if (!/^[0-9]+$/.test(digits)) {
    throw bigLiteralError(src)
  }
  const n = BigInt(digits)
  return newBigInteger(sign === '-' ? -n : n)
}

function bigLiteralError(src: string): BoruError {
  return new BoruError('syntax_error', 'invalid 0d numeric literal: ' + src, src)
}

// isAlnum reports whether c is an ASCII letter or digit (the chars that
// may flank a `_` digit-separator: decimal/hex digits and the x/o/b/e of
// a prefix or exponent).
function isAlnum(c: string): boolean {
  return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// validUnderscores reports whether every `_` in src is a single separator
// flanked by alphanumeric characters — no leading, trailing, or repeated
// underscores. `1_000` and `0xFF_FF` pass; `1__0`, `1_`, `_1` fail.
function validUnderscores(src: string): boolean {
  for (let i = 0; i < src.length; i++) {
    if (src[i] !== '_') {
      continue
    }
    if (i === 0 || i === src.length - 1 || !isAlnum(src[i - 1]!) || !isAlnum(src[i + 1]!)) {
      return false
    }
  }
  return true
}

// isLeadingDotFloat reports whether src is a `.`-leading number (after an
// optional sign): `.5`, `-.5`, `+.5`. These need a digit before the dot.
function isLeadingDotFloat(src: string): boolean {
  let s = src
  if (s.length > 0 && (s[0] === '-' || s[0] === '+')) {
    s = s.slice(1)
  }
  return s.length > 0 && s[0] === '.'
}

// isPlainDecimalInteger reports whether src is a base-10 integer literal:
// an optional leading sign ('-' or '+') followed by one or more decimal
// digits, with '_' permitted as a digit separator. It deliberately
// rejects base prefixes (0x/0o/0b — handled by isBasePrefixedInteger) and
// exponents (1e3). Leading zeros stay decimal (jsonic treats 010 as 10,
// not octal), which the base-10 parse also does.
export function isPlainDecimalInteger(src: string): boolean {
  if (src === '') {
    return false
  }
  let i = 0
  if (src[0] === '-' || src[0] === '+') {
    i = 1
  }
  if (i === src.length) {
    return false
  }
  let sawDigit = false
  for (; i < src.length; i++) {
    const c = src[i]!
    if (c >= '0' && c <= '9') {
      sawDigit = true
    } else if (c === '_') {
      // separator; allowed between digits
    } else {
      return false
    }
  }
  return sawDigit
}

// isBasePrefixedInteger reports whether src is a hex / octal / binary
// integer literal — an optional sign then a 0x / 0o / 0b prefix. These are
// parsed exactly (bigint) rather than via binary64, so values above 2^53
// keep full precision instead of silently rounding.
function isBasePrefixedInteger(src: string): boolean {
  let s = src
  if (s.length > 0 && (s[0] === '-' || s[0] === '+')) {
    s = s.slice(1)
  }
  if (s.length < 3 || s[0] !== '0') {
    return false
  }
  const c = s[1]!
  return c === 'x' || c === 'X' || c === 'o' || c === 'O' || c === 'b' || c === 'B'
}

// stripUnderscores removes '_' digit separators so the exact-digit
// parsers accept a source jsonic lexed with separators (e.g. 1_000).
function stripUnderscores(src: string): string {
  if (!src.includes('_')) {
    return src
  }
  return src.replaceAll('_', '')
}

// integerLiteralOverflowError reports an integer literal (decimal or
// base-prefixed) that does not fit in int64, located at the literal's
// source position when known (row/col 0 = unknown; the TS BoruError
// carries no position fields yet — the detail line is the parity unit).
function integerLiteralOverflowError(src: string, _row: number, _col: number): BoruError {
  return new BoruError(
    'integer_overflow',
    'integer literal out of range: ' +
      src +
      ' exceeds the Integer range (-9223372036854775808..9223372036854775807)',
    src,
  )
}

// underscoreError reports a misused `_` digit-separator (leading, trailing,
// or repeated) in a numeric literal.
function underscoreError(nv: NumberVal): BoruError {
  return new BoruError('syntax_error', 'misplaced `_` in numeric literal: ' + nv.src, nv.src)
}

// leadingDotNumberError reports a `.`-leading numeric literal (`-.5`,
// `+.5`); a number needs a digit before the decimal point.
function leadingDotNumberError(nv: NumberVal): BoruError {
  return new BoruError(
    'syntax_error',
    'numeric literal has no digit before `.`: ' + nv.src,
    nv.src,
  )
}
