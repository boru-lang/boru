// The conversion layer of the boru TS parser — the port of
// parser/go/parse.go (stage 3 of Go Parse). parse(src) runs the
// configured jsonic instance (grammar.ts makeBoruJsonic) over src and
// converts the jsonic output into engine Values, throwing BoruError on
// syntax errors (jsonic failures are translated by errors.ts
// translateParseError). The functions below keep the Go twin's structure;
// the independent shared specs adjudicate behavior when either port drifts.
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
// reversed signature arg order; combine with /v to leave it as data. The
function newWordUsurp(name: string, forceVal: boolean): Value {
  return new Value(TWord, { name, forceUsurp: true, forceVal } satisfies WordInfo)
}

// newWordRef mirrors eng.NewWordRef: a word value marked with the /v
// modifier — resolve the name to its bound Function without invoking.
function newWordRef(name: string): Value {
  return new Value(TWord, { name, forceVal: true } satisfies WordInfo)
}

// newWordModified mirrors eng.NewWordModified: a word with explicit
// argument-shape modifiers. Go's -1 argCount sentinel maps to the TS
// optional-field convention (undefined = unset). The arity is a bigint, as
// exact as Go's int64 (NUR072: a JS number rounded `/N` above 2^53).
function newWordModified(
  name: string,
  argCount: bigint,
  forceStack: boolean,
  forceForward: boolean,
): Value {
  const info: WordInfo = { name, forceStack, forceForward }
  if (argCount >= 0n) info.argCount = argCount
  return new Value(TWord, info)
}

// newBigInteger / newBigDecimal mirror eng.NewBigInteger/NewBigDecimal.
// Both payloads preserve arbitrary precision: bigint for integers and the
// core Decimal scaled-bigint model for decimals.
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
export function errMessage(e: unknown): string {
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

// enterDepth/leaveDepth retain Go's balanced conversion accounting. The
// explicit prepareNestedConversions walk is the single depth-limit authority:
// it validates the complete graph before any direct converter can recurse, so
// a second limit branch here would be dead on every parser path.
function enterDepth(d: ParseDepth): void {
  d.cur++
}

function leaveDepth(d: ParseDepth): void {
  d.cur--
}

// withPos stamps a parser source position onto the fresh Value. This mirrors
// core.Value.SetPos on the Go side; synthesized runtime values retain the
// core default {row:0,col:0} location.
function withPos(v: Value, pos: SrcPos): Value {
  if (pos.row !== 0) {
    v.pos = { row: pos.row, col: pos.col, src: pos.src ?? '' }
  }
  return v
}

// A convertedNode is an internal trampoline cell. The tabnas rule engine is
// iterative, but the direct Go-shaped conversion port used to recurse once
// per list/map/paren level and could overflow V8's host stack well below the
// language's 10,000-level limit. prepareNestedConversions walks the parsed
// node graph with an explicit work stack and replaces each nested structural
// node with one of these already-converted cells. The ordinary converters
// then retain all of their source-order and context logic without recursively
// descending through host call frames.
// Source-module export only (like the direct converter seams): the guard
// tests assert the preparation walk actually installs these cells. It is
// not part of the package entry point.
export class ConvertedNode {
  value: Value
  parenItems: Value[] | undefined

  constructor(value: Value, parenItems?: Value[]) {
    this.value = value
    this.parenItems = parenItems
  }
}

// prepareNestedConversions must not THROW a descendant conversion error as
// soon as its bottom-up work stack reaches that descendant. Go converts in
// source/context order: an outer numeric receiver can therefore reject
// `1.e:` before the trailing `e:` map's empty value is inspected, while a
// typed-map child is inspected before its concrete entries. Retain failures
// as cells, just like successful ConvertedNodes, and rethrow only when the
// ordinary converter reaches that cell. This keeps the stack-safe traversal
// without changing Go's observable error precedence.
class FailedConvertedNode {
  error: unknown

  constructor(error: unknown) {
    this.error = error
  }
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
type ConversionContext = 'top' | 'data'

interface ConversionTask {
  node: unknown
  context: ConversionContext
  level: number
  replace?: (next: unknown) => void
  exit?: boolean
  root?: boolean
}

function interpolationExpressionError(err: unknown): Error {
  return Object.assign(new Error(`interpolation expression error: ${errMessage(err)}`), {
    cause: err,
  })
}

function nestingLimitError(src: string): BoruError {
  return new BoruError(
    'evaluation_limit',
    `source nesting exceeds the depth limit of ${MAX_PARSE_NESTING_DEPTH} — lists, maps, and parentheses are nested too deeply`,
    '',
    {
      fullSource: src,
      hint: 'flatten the structure or split it across definitions; nesting this deep is almost always generated, not intended',
    },
  )
}

// prepareNestedConversions is the stack-safe counterpart of Go's recursive
// conversion walk. Besides trampolining nested structures bottom-up, it
// tracks the SAME logical enterDepth calls the direct converters make. That
// matters at the boundary: a root paren has the implicit top-level item frame
// plus its own paren frame, while a root list/map has just its container
// frame. The limit therefore stays behaviorally identical to parser/go rather
// than becoming a merely lexical bracket count (which would also miscount
// bracket characters inside XML text).
export function prepareNestedConversions(root: unknown, d: ParseDepth): void {
  const rootLevel = root instanceof ParenGroup ? 1 : 0
  const work: ConversionTask[] = [{
    node: root,
    context: 'top',
    level: rootLevel,
    root: true,
  }]

  const pushValue = (
    node: unknown,
    context: ConversionContext,
    level: number,
    replace: (next: unknown) => void,
  ): void => {
    work.push({ node, context, level, replace })
  }

  while (work.length > 0) {
    const task = work.pop()!
    let node = task.node
    let replace = task.replace
    while (node instanceof Sited) {
      const site = node
      site.pos = d.normalizePos(site.pos)
      node = site.node
      if (node instanceof NumberVal && site.pos.row !== 0) {
        node.row = site.pos.row
        node.col = site.pos.col
      }
      replace = (next: unknown): void => {
        site.node = next
      }
    }

    const structural =
      Array.isArray(node) ||
      isMapNode(node) ||
      node instanceof ParenGroup ||
      node instanceof AngleGroup ||
      node instanceof InterpGroup
    if (!structural) {
      continue
    }

    if (task.exit) {
      if (!task.root) {
        try {
          let parenItems: Value[] | undefined
          let value: Value
          if (node instanceof ParenGroup) {
            parenItems = convertTopLevelItems(node.items, d)
            value = newParenExpr(parenItems)
          } else if (task.context === 'data') {
            value = convertDataValueInner(node, d)
          } else {
            value = convertTopLevelValueInner(node, d)
          }
          replace!(new ConvertedNode(value, parenItems))
        } catch (err) {
          // Defer the failure to the ordinary source/context-ordered walk.
          // Interpolation wrappers are added naturally when that walk crosses
          // each enclosing InterpGroup; wrapping here would either report a
          // later child too early or double-wrap a stored failure.
          replace!(new FailedConvertedNode(err))
        }
      }
      continue
    }

    let childLevel = task.level
    if (Array.isArray(node) || isMapNode(node) || node instanceof ParenGroup) {
      childLevel++
      if (childLevel > MAX_PARSE_NESTING_DEPTH) {
        // Defer the breach exactly like a failed descendant conversion: Go
        // raises the limit when its source-ordered recursive walk ENTERS
        // this container, so a source-earlier error elsewhere must still
        // win. The ordinary walk adds interpolation wrappers when it
        // crosses each enclosing InterpGroup; wrapping here would either
        // report this breach too early or double-wrap the stored failure.
        replace!(new FailedConvertedNode(nestingLimitError(d.src)))
        continue
      }
    }

    work.push({ ...task, node, replace, exit: true })

    if (Array.isArray(node)) {
      const arr = node as Record<string, unknown> & unknown[]
      const typed = arr['child$'] !== undefined
      for (let i = node.length - 1; i >= 0; i--) {
        const index = i
        pushValue(node[index], typed ? 'data' : 'top', childLevel, (next) => {
          node[index] = next
        })
      }
      if (typed) {
        pushValue(arr['child$'], 'data', childLevel, (next) => {
          arr['child$'] = next
        })
      }
      continue
    }

    if (isMapNode(node)) {
      const keys = mapKeys(node)
      for (let i = keys.length - 1; i >= 0; i--) {
        const key = keys[i]!
        pushValue(node[key], 'data', childLevel, (next) => {
          node[key] = next
        })
      }
      continue
    }

    if (node instanceof ParenGroup || node instanceof AngleGroup) {
      for (let i = node.items.length - 1; i >= 0; i--) {
        const index = i
        pushValue(node.items[index], 'top', childLevel, (next) => {
          node.items[index] = next
        })
      }
      continue
    }

    // Interpolation itself adds no depth, but each ${...} expression is
    // converted through convertTopLevelItems and therefore adds one logical
    // frame. Literal parts have no descendants.
    const interp = node as InterpGroup
    for (let p = interp.parts.length - 1; p >= 0; p--) {
      const raw = interp.parts[p]
      const [part] = deSite(raw)
      if (!(part instanceof IexprGroup)) {
        continue
      }
      const exprLevel = childLevel + 1
      if (exprLevel > MAX_PARSE_NESTING_DEPTH) {
        // Defer the breach exactly like a failed descendant conversion:
        // Go raises the limit when its source-ordered walk enters this
        // expression frame, so a source-earlier error elsewhere must
        // still win. convertInterpGroup adds this expression's own
        // wrapper when it reaches the cell; the enclosing `${...}`
        // frames wrap naturally after that.
        const cell = new FailedConvertedNode(nestingLimitError(d.src))
        if (raw instanceof Sited) {
          raw.node = cell
        } else {
          interp.parts[p] = cell
        }
        continue
      }
      for (let i = part.items.length - 1; i >= 0; i--) {
        const index = i
        pushValue(part.items[index], 'top', exprLevel, (next) => {
          part.items[index] = next
        })
      }
    }
  }
}


export function parse(src: string): Value[] {
  // Stages 1-2 (lex + grammar setup) live in grammar.ts; the instance is
  // built per parse, exactly as Go Parse constructs a fresh jsonic.
  const { j } = makeBoruJsonic()

  // Stage 3: Parse and convert to engine values. The library's own error
  // rendering is silenced at the source (grammar.ts options); failures
  // are translated into boru syntax_errors here.
  let result: unknown
  try {
    result = j.parse(src)
  } catch (e) {
    throw translateParseError(e, src)
  }

  if (result === null || result === undefined) {
    return []
  }

  // A single top-level scalar/paren/interp value arrives wrapped by the
  // val-rule BC; unwrap it so the cases below see a bare jsonic node, but
  // keep its position to stamp the single produced value. (Root containers
  // come from the list/map rule and are not sited; their elements carry
  // positions individually.)
  // One depth tracker for this parse. Besides logical nesting, it owns a
  // linear UTF-16-index → code-point-column table so normalizing every
  // sited node below remains O(n), even for a 10,000-node single line.
  const d = new ParseDepth(src)
  const [res, rawRootPos] = deSite(result)
  const rootPos = d.normalizePos(rawRootPos)
  if (res instanceof NumberVal && rootPos.row !== 0) {
    res.row = rootPos.row
    res.col = rootPos.col
  }

  // The rule engine is iterative, but the direct Go-shaped converter used
  // to recurse once per structural level and overflow V8's call stack well
  // before Go's 10,000-level contract. Prepare nested nodes bottom-up on an
  // explicit work stack, enforcing the same logical conversion depth.
  prepareNestedConversions(res, d)

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
    // The configured parser always enables info.map, so every root map has
    // this marker. Raw-map compatibility lives in the public value converter;
    // retaining a second fallback here would be an unreachable parse branch.
    const info = getInfo(res)!
    const implicit = !!info.implicit
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
    throw new BoruError('syntax_error', 'unmatched opening parenthesis', '(', {
      fullSource: src,
    })
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
export function convertTopLevelItems(items: unknown[], d: ParseDepth): Value[] {
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
        // auto-dispatch first), while the Word/__DM marker (/N /v /q)
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
          if (
            keyItem instanceof ParenGroup ||
            (keyItem instanceof ConvertedNode && keyItem.parenItems !== undefined)
          ) {
            // Computed key: m.(expr) — store the paren's tokens.
            seg.computed = true
            seg.keyExpr = keyItem instanceof ConvertedNode
              ? keyItem.parenItems
              : convertTopLevelItems(keyItem.items, d)
          } else {
            // Literal key (word → atom-via-get/q, string, number).
            const tmp: Value[] = []
            emitPrimary(tmp, keyItem, kpos, d)
            // emitPrimary appends exactly one value on success.
            seg.keyLit = reachSegmentName(keyItem, tmp[0]!, kpos)
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
      // before any auto-dispatch); the /v /q marker is emitted AFTER.
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
//	/v /q → suffix Word/__DM marker (leave the result inert/data)
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
  if (m.argCount >= 0n) {
    return {
      base: m.base,
      prefix: [newSugar({ kind: 'force-arity', n: m.argCount })],
      suffix: null,
    }
  }
  if (m.valFlag || m.quoteFlag) {
    return {
      base: m.base,
      prefix: null,
      suffix: [newDispatchMod({ val: m.valFlag, quote: m.quoteFlag })],
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
// Step 1, design/legacy/PAREN-REPRESENTATION.9.ignore), the same representation data
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
  if (v instanceof FailedConvertedNode) {
    throw v.error
  }
  if (v instanceof ConvertedNode) {
    return v.value
  }
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

  if (v instanceof UnclosedParen) {
    // An unclosed group in a member or operand position (`a.(`,
    // `quote . ( =`): the item loop's refusal, never the internal marker's
    // type name (NUR060).
    throw new BoruError('syntax_error', 'unmatched opening parenthesis', '(')
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
  return new BoruError('syntax_error', 'a number has no members to access with `.`', pos.src ?? '', {
    row: pos.row,
    col: pos.col,
    src: pos.src ?? '',
    hint: 'this looks like a malformed numeric literal (e.g. `1.2.3`) or `.`-access on a number — numbers have no fields or keys',
  })
}

// danglingDotError rejects a leading / receiverless `.` or `!.` — the
// prefix dot form (e.g. `.5`, `.name`, `!.x`). Member access must follow
// a value (`m.key`); a leading `.` is never valid. In particular `.5` is
// not a fraction — write `0.5`. The receiverless reach `$.name` is the
// supported way to write a detached accessor.
function danglingDotError(pos: SrcPos): BoruError {
  return new BoruError('syntax_error', '`.` member access has no receiver', pos.src ?? '', {
    row: pos.row,
    col: pos.col,
    src: pos.src ?? '',
    hint: 'a `.`/`!.` access must follow a value (e.g. `m.key`); a leading `.` is not valid — write `0.5` for a fraction, or `$.key` for a detached accessor',
  })
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
      om.implicit = true
    }
    // Extract metadata from the map node's Meta.
    const qmSet = metaStrSet(meta, 'qm') // optional keys (? syntax)
    const ckSet = metaStrSet(meta, 'ck') // computed keys ([key] syntax)
    const qkSet = metaStrSet(meta, 'qk') // quoted keys ({'k': v} syntax)
    const shList = metaStrList(meta, 'sh') // shorthand keys ({foo} / {foo/v} syntax)
    const ko = metaStrList(meta, 'ko') // source key order (D1: Meta["ko"] channel)

    // Word modifiers (`/v`, `/q`, `/f`, `/s`, `/N`) are legal only on
    // shorthand entries — `{foo/v}` ≡ `{foo: foo/v}` — where the token is
    // also the value, so the modifier qualifies that value. On a bare
    // `key: value` pair the modifier would only ever land on the KEY,
    // which is meaningless (a map key is a plain name). Reject it rather
    // than silently keeping `f/v` as a literal key. A quoted key
    // (`{'f/v': …}`) or computed key (`{[f/v]: …}`) is an explicit string
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

    // Shorthand entries: `{foo}` ≡ `{foo: foo}`, `{foo/v}` ≡ `{foo: foo/v}`,
    // `{foo?}` ≡ `{foo?: foo}`, `{foo/v?}` ≡ `{foo?: foo/v}`. The key is
    // always the base name (modifiers stay on the value only); the value
    // is the full word token. synth maps each synthesized base key to that
    // token. Two sources: explicit shorthand tokens (Meta["sh"]) and
    // optional keys (Meta["qm"]) that never received an explicit value.
    //
    // optBase collects the base name of every optional key so optionality
    // is looked up by base name below — qmSet is keyed by the raw token
    // (e.g. `f/v`), which no longer matches the synthesized base key.
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
      om.meta = { ck: ckSet }
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
  if (v instanceof FailedConvertedNode) {
    throw v.error
  }
  if (v instanceof ConvertedNode) {
    return v.value
  }
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
// and retained on the resulting Value's ChildType.entries; the runtime
// `is` validates each entry's value against child on demand.
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
    // Go stores BoruError.Error(), not just its first line, in HeadErr.
    // angleGenList's two refusals are structured BoruErrors with either a
    // located hint (bad parameter name) or just a location (missing value).
    const error = herr as BoruError
    let rendered = error.message + '\n  --> '
    rendered += error.row > 0 ? `${error.row}:${Math.max(error.col, 1)}` : 'source position unknown'
    if (error.hint !== '') rendered += `\n  = ${error.hint}`
    info.headErr = rendered
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
        {
          row: epos.row,
          col: epos.col,
          src: epos.src ?? '',
          hint: 'write def Name<T> / def Name<T extends Bound> / def Name<T = Default>',
        },
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
              { row: epos.row, col: epos.col, src: epos.src ?? '' },
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
// Source-module export only: a well-formed grammar order channel is already
// de-duplicated, while direct tests still pin this defensive converter seam.
export function orderedKeys(union: Set<string>, ko: string[]): string[] {
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
  argCount: bigint
  forceStack: boolean
  forceForward: boolean
  quoteFlag: boolean
  valFlag: boolean
  usurpFlag: boolean
  typeFlag: boolean
  valid: boolean
}

// scanWordModifier parses the optional `/...` modifier suffix of an
// unquoted word token: name/f (forceForward), name/s (forceStack),
// name/N (argCount), name/q (quote → Atom), name/v (val → bound value),
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
    argCount: -1n,
    forceStack: false,
    forceForward: false,
    quoteFlag: false,
    valFlag: false,
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

  // Scan modifier chars in any order: digits, 'f', 's', 'q', 'v', 'u',
  // 't'. Each letter appears at most once; f/s are mutually exclusive;
  // q is mutually exclusive with r and u; t (the type-bound sugar,
  // `Map/t` ≡ `(Type of [Map])`) combines with nothing — it produces a
  // type expression, not a word; digits run contiguously and form a
  // single argCount value.
  let valid = true
  let seenDigits = false
  let argCount = -1n
  let forceStack = false
  let forceForward = false
  let quoteFlag = false
  let valFlag = false
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
          argCount = n
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
      if (quoteFlag || valFlag || usurpFlag) {
        valid = false
      } else {
        quoteFlag = true
      }
    } else if (c === 'v') {
      if (valFlag || quoteFlag) {
        valid = false
      } else {
        valFlag = true
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
  if (typeFlag && (quoteFlag || valFlag || usurpFlag || forceStack || forceForward || argCount >= 0n)) {
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
    valFlag,
    usurpFlag,
    typeFlag,
    valid: true,
  }
}

// isModifierAlphabet reports whether every character of a candidate
// modifier suffix is drawn from the modifier alphabet (digits plus
// f s q v u t). It is the line between a BOTCHED MODIFIER (`foo/fs`,
// `foo/qv`, `foo/1f2` — all-alphabet but an invalid combination),
// which errors loudly, and a slash-bearing plain name (`foo/bar`,
// `add/x`, the builtin type paths `Scalar/Number/Integer` — the
// suffix contains a non-modifier character, uppercase included),
// which stays a single plain word exactly as before. Valid modifier
// combinations never reach this: scanWordModifier claims them first.
function isModifierAlphabet(s: string): boolean {
  for (let i = 0; i < s.length; i++) {
    const c = s[i]!
    if (c >= '0' && c <= '9') continue
    if (c === 'f' || c === 's' || c === 'q' || c === 'v' || c === 'u' || c === 't') continue
    return false
  }
  return true
}

// wordBaseName returns the base name of an unquoted word token, stripping
// a valid `/...` modifier suffix (foo/v → foo, foo → foo). Used to derive
// the key of a shorthand map entry, whose value keeps the full token.
function wordBaseName(text: string): string {
  return scanWordModifier(text).base
}

// parseWord interprets an unquoted text token as a boru word, handling
// the modifier syntax decoded by scanWordModifier. q produces an Atom and
// overrides the other modifiers; u emits a usurp-word and r emits a
// val-word, both of which short-circuit the rest.
// BARE_WORD_NAME mirrors core.ValidateWordName's character rule on the Go
// side (that rule is not ported to core/ts): first character [a-z_-$], the
// rest [a-z0-9_-$]. ValidateWordName additionally rejects the all-`$` and
// all-`-` names; those are not repeated here because they cannot reach this
// point — parseWord returns a WORD for both, and the only caller
// (reachSegmentName) has already returned by then. The shared `parser/spec`
// rows both ports run are what actually keep the two in step; this is a
// convenience, not a second source of truth.
const BARE_WORD_NAME = /^[a-z_$-][a-z0-9_$-]*$/

// reachSegmentName restores the dot-access lowering identity for a segment
// spelled as a BARE NAME. Dot access IS a `get`/`getr` chain
// (design/REACH.10.md is the single source of truth for the lowering), so
// `m.k` and `m get k/q` are meant to differ in spelling only. They did not:
// the reserved VALUE literals — `none`, `end`, `inf`, `-inf`, `nan` —
// resolve to their values in parseWord before the chain sees them, so the
// segment arrived as a marker or a Float and no `dot` signature matched
// (NUR066). The `/q` form never had the problem, because a modifier suffix
// returns an atom before the literal switch is reached, which is why
// `m get none/q` read the field all along.
//
// The rule is DERIVED rather than a list of names: a segment whose source
// token is an unquoted valid word name that parseWord turned into
// something OTHER than a Word is exactly a reserved literal, and as a
// field name it means the name. A new literal joins parseWord and this
// stays in step with no second edit.
//
// Non-names are untouched and keep their literal readings: `m.1` indexes,
// `m.'k'` is a string key, `m.(expr)` is computed (handled by the caller),
// and the punctuation-spelled markers fail the name rule — `;` among them,
// which is the reason the grammar hands the semicolon its own text instead
// of rewriting it to `end`. `m.;` therefore stays the error it was rather
// than silently reading a field called `end`.
function reachSegmentName(keyItem: unknown, key: Value, pos: SrcPos): Value {
  if (key.isWord()) {
    return key
  }
  const [node] = deSite(keyItem)
  const text = asText(node)
  if (text === undefined || text.quote !== '') {
    return key
  }
  if (!BARE_WORD_NAME.test(text.str)) {
    return key
  }
  return withPos(newWord(text.str), pos)
}

export function parseWord(text: string): Value {
  const m = scanWordModifier(text)
  const name = m.base

  if (name === '') {
    // A `/` modifier with nothing before it (`/s`, `/v`, `/2`): a modifier
    // follows the word or group it modifies. It used to leave the parser as
    // a plain `empty word` Error — the one parse failure that was no
    // syntax_error, in both ports (NUR060).
    throw new BoruError(
      'syntax_error',
      '`' + text + '` modifies nothing: a `/` modifier follows the word or group it modifies',
    )
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
        {
          hint: 'modifier letters stack in any order, each at most once; f|s are exclusive; q excludes v and u; t combines with nothing; digits form one contiguous run within int range',
        },
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
    if (hasUppercaseNumericPrefix(name)) {
      throw uppercaseNumericPrefixError(name)
    }
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
  // with /v: /u alone dispatches the wrapper, /uv leaves it as data.
  // Argument-shape modifiers don't apply (the wrapper supplies its own).
  if (m.usurpFlag) {
    return newWordUsurp(name, m.valFlag)
  }

  // /v emits a val-word that, when reached at the pointer, resolves the
  // name to its bound Function value without invoking. /v is legal only
  // for function words; a non-fn binding raises illegal_ref at run time
  // (the parser accepts the syntax — the binding kind isn't known until
  // resolution). Argument-shape modifiers don't apply because val
  // bypasses dispatch entirely; they're accepted syntactically but
  // ignored.
  if (m.valFlag) {
    return newWordRef(name)
  }

  if (m.forceStack || m.forceForward || m.argCount >= 0n) {
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
  //
  // `;` carries its OWN text rather than being rewritten to `end` in
  // the grammar (as `)` already does): the two spell the same value but
  // are not the same source, and a dot-path segment has to tell them
  // apart — `m.end` names a field, `m.;` is a stray terminator
  // (reachSegmentName, NUR066).
  switch (name) {
    case 'end':
    case ';':
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
    // No uppercase check here: both lexers CLAIM a base-prefixed run as #NR
    // whatever its magnitude, so an uppercase one is refused in
    // numberValToValue and can never reach this text path.
    if (name.includes('_') && !validUnderscores(name)) {
      throw new BoruError('syntax_error', 'misplaced `_` in numeric literal: ' + name, name, {
        hint: '`_` is a single digit-separator — use one between digits',
      })
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
    // setupDecimalUnderscoreMatcher gives this port Go's number/text token
    // boundaries for underscore-bearing decimal runs. Thus this fallback
    // can mirror Go directly: a text token can report overflow, but it is
    // never converted into a value here; actual numeric tokens flow through
    // numberValToValue.
    const stripped = stripUnderscores(name)
    const f = Number(stripped)
    if (f === Infinity || f === -Infinity) {
      throw floatLiteralOverflowError(name)
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
    {
      hint: 'the Float range is about ±1.8e308; write the `inf` literal for infinity',
    },
  )
}

// malformedNumberError reports a digit-led token that is not a valid
// number. Since no word may start with a digit, such a token can only be
// a botched numeric literal (`1e`, `0x1p4`) or a digit-first name (`2dup`
// — the stack words are now `dup2`/`swap2`/`drop2`/`over2`).
function malformedNumberError(src: string): BoruError {
  return new BoruError('syntax_error', 'invalid numeric literal: ' + src, src, {
    hint: 'a number is digits with an optional sign, base prefix (0x/0o/0b), `.`, exponent (`e`), and single `_` separators — and a name cannot start with a digit',
  })
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
    if (item instanceof FailedConvertedNode) {
      // A `${...}` expression frame the pre-pass could not convert (e.g.
      // a deferred nesting-limit breach). Rethrow with this expression's
      // own wrapper, exactly as the catch below adds it for a failure
      // raised during live conversion.
      throw interpolationExpressionError(item.error)
    }
    const t = asText(item)
    if (t !== undefined) {
      // Template literal segment (quote "tl").
      parts.push({ lit: t.str })
      continue
    }
    if (item instanceof IexprGroup) {
      if (0 === item.items.length) {
        // An empty hole (`${}`, `${ }`) holds no expression and contributes
        // nothing: a template whose holes are all empty is the plain string
        // it spells, as `abc` in backticks is (NUR060) — and as an XML
        // attribute's empty hole folds.
        continue
      }
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
        throw interpolationExpressionError(err)
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

// processTemplateEscapes processes escape sequences in template literal
// text. (Exported for the grammar layer — the Go twin lives in parse.go
// but is called from grammar.go's template-literal matcher.)
export function processTemplateEscapes(s: string): string {
  if (!s.includes('\\')) {
    return s
  }
  let buf = ''
  for (let i = 0; i < s.length; i++) {
    if (s[i] === '\\' && i + 1 < s.length) {
      const [text, used] = readStringEscape(s, i + 1)
      buf += text
      i += used
    } else {
      buf += s[i]!
    }
  }
  return buf
}

// readStringEscape decodes ONE escape sequence — the character(s) after a
// backslash at s[at] — returning the text it produces and how many bytes
// of s it consumed (always >= 1, counting the escape character itself).
//
// This is the single escape vocabulary for boru string literals
// (NUR026). Templates used to carry a hand-rolled six-case switch
// (`\n \t \r \\ ` $`) that kept everything else LITERAL, while quoted
// strings rode jsonic's native handling and got the full set — so
// `size "z\x41z"` was 3 and its template spelling 6. That was an
// implementation accident, not a design choice: the backtick was removed
// from jsonic's StringChars so templates could carry `${…}`
// interpolation, and the replacement escape handler was never brought to
// parity. A reader should not have to know which quoting form they are
// in to know what `\x41` means.
//
// The vocabulary matches what jsonic accepts for `"…"` / `'…'`, measured
// 2026-08-15:
//
//     \n \t \r \b \f \v   the control characters
//     \xNN                one byte, two hex digits
//     \uNNNN              one rune, four hex digits
//     anything else       the character itself, backslash DROPPED
//
// That last rule is the behaviour change to watch: `\z` is `z` and `\0`
// is `0`, in a template exactly as in a quoted string. The
// template-only spellings need no case of their own — a backslash
// before a backtick or a `$` falls into the default arm and yields the
// bare character, which is what they always meant.
//
// One asymmetry SURVIVES, deliberately and recorded: a MALFORMED \x /
// \u falls through to the default arm, so a template's `\xZZ` is `xZZ`,
// while jsonic RAISES on the same sequence in a quoted string. Matching
// that needs an error channel the call site does not have (a jsonic
// LexMatcher returning a token), which is the unified-lexer work
// NUR026's earlier verdict sketched and this one did not ask for.
function readStringEscape(s: string, at: number): [string, number] {
  const c = s[at]!
  switch (c) {
    case 'n':
      return ['\n', 1]
    case 't':
      return ['\t', 1]
    case 'r':
      return ['\r', 1]
    case 'b':
      return ['\b', 1]
    case 'f':
      return ['\f', 1]
    case 'v':
      return ['\v', 1]
    case 'x': {
      const v = parseHexEscape(s, at + 1, 2)
      return v === null ? [c, 1] : [String.fromCharCode(v), 3]
    }
    case 'u': {
      if ('{' === s[at + 1]) {
        // The braced form, `\u{1F600}`: 1-6 hex digits, any code point.
        const b = parseBracedEscape(s, at + 2)
        return b === null ? [c, 1] : [String.fromCodePoint(b[0]), b[1] + 3]
      }
      // A UTF-16 surrogate pair split across two escapes needs no pairing
      // here: the two code units concatenate into the one code point, as
      // Go's writeStringEscape pairs them explicitly.
      const v = parseHexEscape(s, at + 1, 4)
      return v === null ? [c, 1] : [String.fromCodePoint(v), 5]
    }
    default:
      // Unknown escape: the character itself, backslash dropped —
      // jsonic's rule for a quoted string, now the template's too.
      return [c, 1]
  }
}

// parseBracedEscape reads a braced code point, `{` already consumed: 1-6 hex
// digits at s[at:] and a closing `}`, at most U+10FFFF. Returns the value
// and the digit count, or null.
function parseBracedEscape(s: string, at: number): [number, number] | null {
  const end = s.indexOf('}', Math.min(at, s.length)) - at
  if (end < 1 || end > 6) {
    return null
  }
  const v = parseHexEscape(s, at, end)
  if (v === null || v > 0x10ffff) {
    return null
  }
  return [v, end]
}

// escapeFault reports a malformed `\x` / `\u` escape — the one definition a
// template's text and a quoted string's body both answer to (NUR026). at
// indexes the character after the backslash; stop is the form's closing
// delimiter, which a reported span never crosses. Returns jsonic's code
// (invalid_ascii / invalid_unicode) and the end of the offending span, which
// runs from the backslash; null for a well-formed or other escape.
export function escapeFault(s: string, at: number, stop: string): [string, number] | null {
  const span = (n: number): number => {
    for (let i = at; i < at - 1 + n; i++) {
      if (i >= s.length || s[i] === stop) {
        return i
      }
    }
    return at - 1 + n
  }
  switch (s[at]) {
    case 'x':
      if (parseHexEscape(s, at + 1, 2) === null) {
        return ['invalid_ascii', span(4)]
      }
      break
    case 'u':
      if ('{' === s[at + 1]) {
        if (parseBracedEscape(s, at + 2) === null) {
          // The span runs through the closing `}` when one comes before the
          // delimiter, else to the delimiter or the end.
          const rest = s.slice(at)
          const c = rest.indexOf('}')
          const d = rest.indexOf(stop)
          if (c >= 0 && (d < 0 || c < d)) {
            return ['invalid_unicode', at + c + 1]
          }
          return ['invalid_unicode', span(rest.length + 1)]
        }
      } else if (parseHexEscape(s, at + 1, 4) === null) {
        return ['invalid_unicode', span(6)]
      }
      break
  }
  return null
}

// parseHexEscape reads exactly n hex digits at s[at:] and returns their
// value, or null when the run is short or holds a non-hex digit so the
// caller can fall back to the literal-character reading.
function parseHexEscape(s: string, at: number, n: number): number | null {
  if (at + n > s.length) {
    return null
  }
  let v = 0
  for (let i = at; i < at + n; i++) {
    const c = s.charCodeAt(i)
    let d: number
    if (c >= 0x30 && c <= 0x39) {
      d = c - 0x30
    } else if (c >= 0x61 && c <= 0x66) {
      d = c - 0x61 + 10
    } else if (c >= 0x41 && c <= 0x46) {
      d = c - 0x41 + 10
    } else {
      return null
    }
    v = (v << 4) | d
  }
  return v
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
  if (hasUppercaseNumericPrefix(nv.src)) {
    throw uppercaseNumericPrefixError(nv.src, nv.row, nv.col)
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
// BigDecimal. A `.` or exponent makes it an exact scaled-bigint Decimal;
// otherwise it is a BigInteger (bigint, unbounded). Sign and `_` separators
// are handled like the int/float paths. Errors carry no source position
// (parseWord has none — mirrors the other parseWord numeric errors).
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
  return new BoruError('syntax_error', 'invalid 0d numeric literal: ' + src, src, {
    hint: 'a 0d literal is `0d` then digits, with an optional `.fraction`, exponent, and `_` separators',
  })
}

// isLiteralDigit reports whether c is an actual digit in base. Separators
// beside a prefix/exponent marker or a digit the literal's base cannot spell
// are not digit separators (`0x_1`, `1e_4`, `0b1_2`).
function isLiteralDigit(c: string, base: number): boolean {
  if (c >= '0' && c <= '9') {
    return c.charCodeAt(0) - 48 < base
  }
  return base === 16 && ((c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'))
}

// validUnderscores reports whether every `_` in src is a single separator
// flanked by actual digits for the literal's base. Decimal fractions and
// exponents use decimal digits; 0x/0o/0b bodies use their named base.
// `1_000` and `0xFF_FF` pass; `1e_4`, `0x_1`, `1__0`, and `1_` fail.
function validUnderscores(src: string): boolean {
  let s = src
  if (s.length > 0 && (s[0] === '-' || s[0] === '+')) {
    s = s.slice(1)
  }
  let base = 10
  if (s.length >= 2 && s[0] === '0') {
    switch (s[1]) {
      case 'x': case 'X':
        base = 16
        break
      case 'o': case 'O':
        base = 8
        break
      case 'b': case 'B':
        base = 2
        break
    }
  }
  for (let i = 0; i < src.length; i++) {
    if (src[i] !== '_') {
      continue
    }
    if (
      i === 0 ||
      i === src.length - 1 ||
      !isLiteralDigit(src[i - 1]!, base) ||
      !isLiteralDigit(src[i + 1]!, base)
    ) {
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

// hasUppercaseNumericPrefix reports whether src carries an UPPERCASE base or
// big-number prefix — `0X`, `0O`, `0B`, `0D` — after an optional sign.
//
// Boru's numeric syntax prefixes are lowercase only. Both lexers still CLAIM
// an uppercase run as a numeric token (declining would split the ports: Go's
// stock scanner reads `0XFF` as 255 while this port's reads it as text), so
// the refusal belongs here in the converter, where one diagnostic serves both
// ports and every context.
function hasUppercaseNumericPrefix(src: string): boolean {
  let s = src
  if (s.length > 0 && (s[0] === '-' || s[0] === '+')) {
    s = s.slice(1)
  }
  if (s.length < 3 || s[0] !== '0') {
    return false
  }
  const c = s[1]!
  return c === 'X' || c === 'O' || c === 'B' || c === 'D'
}

// uppercaseNumericPrefixError refuses an uppercase-prefixed numeric literal.
// Loud in EVERY context, data decode included: `0XFF` is a typo for `0xFF`
// far more often than it is text, and a silent string would be the kind of
// wrong answer this parser exists to refuse.
function uppercaseNumericPrefixError(src: string, row = 0, col = 0): BoruError {
  // Callers reach here only via hasUppercaseNumericPrefix, so the prefix
  // letter is always present — replace it unconditionally rather than
  // carrying an arm no input can take.
  const lower = src.replace(/[XOBD]/, (c) => c.toLowerCase())
  return new BoruError('syntax_error', 'numeric prefix must be lowercase: ' + src, src, {
    row,
    col,
    hint: 'write `' + lower + '` — boru\'s numeric prefixes are lowercase (`0x`, `0o`, `0b`, `0d`)',
  })
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
// source position when known (row/col 0 = unknown).
function integerLiteralOverflowError(src: string, row: number, col: number): BoruError {
  return new BoruError(
    'integer_overflow',
    'integer literal out of range: ' +
      src +
      ' exceeds the Integer range (-9223372036854775808..9223372036854775807)',
    src,
    {
      row,
      col,
      src,
      hint: 'this value exceeds the 64-bit signed Integer range; use a Float (e.g. add a decimal point) for an approximate magnitude',
    },
  )
}

// underscoreError reports a misused `_` digit-separator (leading, trailing,
// or repeated) in a numeric literal.
function underscoreError(nv: NumberVal): BoruError {
  return new BoruError('syntax_error', 'misplaced `_` in numeric literal: ' + nv.src, nv.src, {
    row: nv.row,
    col: nv.col,
    src: nv.src,
    hint: '`_` is a single digit-separator — use one between digits, e.g. `1_000` (not `1__0` or `1_`)',
  })
}

// leadingDotNumberError reports a `.`-leading numeric literal (`-.5`,
// `+.5`); a number needs a digit before the decimal point.
function leadingDotNumberError(nv: NumberVal): BoruError {
  return new BoruError(
    'syntax_error',
    'numeric literal has no digit before `.`: ' + nv.src,
    nv.src,
    {
      row: nv.row,
      col: nv.col,
      src: nv.src,
      hint: 'write a leading zero, e.g. `-0.5` instead of `-.5`',
    },
  )
}
