// Value is the engine's universal datum. It carries a runtime type
// (`vType`), an optional `data` payload, and a few flags. The Go
// engine puts these on a flat struct; in TS we use a class so we can
// hang accessor methods off it. PARITY NOTE: TS classes are reference
// types; the Go Value is a value type that handlers receive by copy.
// Mutation through copies is impossible in Go; in TS callers must be
// disciplined about not mutating shared Value instances. Constructors
// here always return fresh instances.
import {
  BoruType,
  TAny,
  TAtom,
  TBigDecimal,
  TBigInteger,
  TBoolean,
  TDisjunct,
  TEnum,
  TFnUndef,
  TFloat,
  TForward,
  TInspect,
  TInterpString,
  TFunction,
  TInteger,
  TList,
  TMap,
  TMark,
  TEmailon,
  TPathon,
  TUrlon,
  TMove,
  TError,
  TNone,
  TParenExpr,
  TCloseParen,
  TDispatchMod,
  TEnd,
  TOpenParen,
  TReach,
  TSplice,
  TString,
  TStringEmpty,
  TStringProper,
  TSugar,
  TType,
  TWord,
  TXml,
  TXmlInterp,
} from './type.ts'
import { Decimal } from './decimal.ts'
import {
  canonValue,
  canonXml,
  formatBigDecimal,
  formatBigInteger,
  formatFloat,
} from './canon.ts'

/** A reified word reference — produced by NewWord, dispatched by the engine. */
export interface WordInfo {
  name: string
  /** Optional argument-count modifier from /N suffix. Unused in spec subset. */
  argCount?: number
  forceStack?: boolean
  forceForward?: boolean
  /**
   * Optional type constraint on a `def NAME:CONTAINER` binding, where the
   * constraint is a container shape (`[:T]` typed list or `{k:T …}` record
   * shape) that the tokenizer attaches to the binding name token.
   */
  constraint?: Value
}

/** A typed parameter on a function definition. */
export interface FnParam {
  name: string
  type: BoruType
  /** Optional (`?`) params default to their type's base value when omitted. */
  optional?: boolean
}

/**
 * A typed parameter on a function definition.
 *
 * Forward dispatch deferral:
 *
 * When a function's match has uncollected forward args at dispatch
 * time, we replace the function word with a `ForwardMarker` Value.
 * The engine continues stepping; intervening function calls produce
 * literals that flow back to the marker via stepLiteral. Once
 * `collected.length === expectedForward`, the marker fires the
 * original handler and is itself replaced by the handler's result.
 * Mirrors the (insertForward, stepLiteral, ForwardInfo) trio in
 * borueng/go/engine.go.
 */
export interface ForwardMarker {
  funcName: string
  sig: import('./signature.ts').Signature
  expectedForward: number
  /** Forward args in collection order (== sig order from sig[0]). */
  collected: Value[]
  /** Stack args resolved at match time. Combined with `collected` to
   *  build the full sig-ordered arg list. Stored in sig order. */
  stackArgs: Value[]
}

/**
 * A user-defined function: typed params + return-type slots + a body
 * the engine runs when the fn is invoked. The spec subset uses a
 * single-overload shape; the full Go FnDefInfo carries multiple
 * overloads (Sigs[]) plus optional Patterns / NoEvalArgs.
 */
/** One authored signature of a function definition. */
export interface FnSig {
  params: FnParam[]
  returns: BoruType[]
  body: Value[]
}

export interface FnDefInfo {
  sigs: FnSig[]
}

export class Value {
  readonly vType: BoruType
  readonly data: unknown
  /**
   * For TList values: when true, the list contents will auto-evaluate
   * when the list is consumed as a word argument (or when it's
   * unconsumed at end of Run). Parser/tokenizer produces lists with
   * eval=true; quote produces lists with quoted=true to suppress.
   * Mirrors Go Value.Eval / Value.Quoted on TList.
   */
  readonly eval: boolean
  readonly quoted: boolean
  readonly carrier: boolean
  /**
   * Check-mode gradual modality. A `dynamic` carrier is a type-only
   * value whose type is statically unknown (Any) or arrived from an
   * unannotated word — it matches optimistically and its contagion
   * widens results to dynamic. Only meaningful when `carrier` is true.
   * Mirrors Go Value.Dynamic.
   */
  readonly dynamic: boolean
  /**
   * Check-mode marker: an undefined word kept as a lenient placeholder
   * so analysis can continue past a typo. Drained to an Any carrier at
   * end of run. Mirrors Go Value.Undefined.
   */
  undefined: boolean

  constructor(
    vType: BoruType,
    data: unknown,
    opts?: { eval?: boolean; quoted?: boolean; carrier?: boolean; dynamic?: boolean },
  ) {
    this.vType = vType
    this.data = data
    this.eval = opts?.eval ?? false
    this.quoted = opts?.quoted ?? false
    this.carrier = opts?.carrier ?? false
    this.dynamic = opts?.dynamic ?? false
    this.undefined = false
  }

  /** True iff this Value is the unique `none` value (None's sole inhabitant). */
  isNone(): boolean {
    return this.vType.equal(TNone) && this.data === NONE_SENTINEL
  }

  /** True iff this Value is a type literal (no concrete payload). */
  isTypeLiteral(): boolean {
    return this.data === null && !this.carrier && !this.vType.equal(TNone)
  }

  /** True iff this Value carries a concrete payload (not a literal, not a carrier). */
  isConcrete(): boolean {
    return this.data !== null && !this.carrier
  }

  isWord(): boolean {
    return this.vType.equal(TWord)
  }

  asInteger(): bigint {
    if (this.data === null) throw new Error('AsInteger: nil data')
    if (typeof this.data !== 'bigint') {
      throw new Error(`AsInteger: not an integer value (got ${typeof this.data})`)
    }
    return this.data
  }

  asFloat(): number {
    if (this.data === null) throw new Error('AsFloat: nil data')
    if (typeof this.data !== 'number') {
      throw new Error(`AsFloat: not a float value (got ${typeof this.data})`)
    }
    return this.data
  }

  /** Back-compat alias for asFloat. */
  asDecimal(): number {
    return this.asFloat()
  }

  asString(): string {
    if (this.data === null) throw new Error('AsString: nil data')
    if (typeof this.data !== 'string') {
      throw new Error(`AsString: not a string value (got ${typeof this.data})`)
    }
    return this.data
  }

  asBoolean(): boolean {
    if (this.data === null) throw new Error('AsBoolean: nil data')
    if (typeof this.data !== 'boolean') {
      throw new Error(`AsBoolean: not a boolean value (got ${typeof this.data})`)
    }
    return this.data
  }

  asWord(): WordInfo {
    if (this.data === null || typeof this.data !== 'object') {
      throw new Error('AsWord: not a word value')
    }
    return this.data as WordInfo
  }

  asAtom(): string {
    if (this.data === null) throw new Error('AsAtom: nil data')
    if (typeof this.data !== 'string') {
      throw new Error(`AsAtom: not an atom value (got ${typeof this.data})`)
    }
    return this.data
  }

  asList(): Value[] {
    if (this.data === null) throw new Error('AsList: nil data')
    if (!Array.isArray(this.data)) {
      throw new Error(`AsList: not a list value`)
    }
    return this.data as Value[]
  }

  isMap(): boolean {
    return this.vType.equal(TMap)
  }

  asMap(): OrderedMap {
    if (this.data instanceof OptionsData) return this.data.map
    if (this.data === null || !(this.data instanceof OrderedMap)) {
      throw new Error('AsMap: not a map value')
    }
    return this.data
  }

  isOptions(): boolean {
    return this.data instanceof OptionsData
  }

  /** True iff this is a typed list `[:T]` (ChildType payload, VType List). */
  isTypedList(): boolean {
    return this.data instanceof ChildType && this.vType.equal(TList)
  }

  /** True iff this is a typed map `{:T}` (ChildType payload, VType Map). */
  isTypedMap(): boolean {
    return this.data instanceof ChildType && this.vType.equal(TMap)
  }

  asChildType(): ChildType {
    if (!(this.data instanceof ChildType)) throw new Error('AsChildType: not a typed container')
    return this.data
  }

  /** True iff this is a Disjunct or Enum value (carries DisjunctInfo). */
  isDisjunct(): boolean {
    return this.vType.matches(TDisjunct) && this.data !== null
  }

  asDisjunct(): DisjunctInfo {
    if (this.data === null || typeof this.data !== 'object') {
      throw new Error('AsDisjunct: not a disjunct value')
    }
    return this.data as DisjunctInfo
  }

  asFnDef(): FnDefInfo {
    if (this.data === null) throw new Error('AsFnDef: nil data')
    if (typeof this.data !== 'object') {
      throw new Error(`AsFnDef: not a function value`)
    }
    return this.data as FnDefInfo
  }

  isFnDef(): boolean {
    return this.vType.matches(TFunction) && this.data !== null
  }

  isForward(): boolean {
    return this.vType.equal(TForward)
  }

  asForward(): ForwardMarker {
    if (this.data === null) throw new Error('AsForward: nil data')
    return this.data as ForwardMarker
  }

  isMark(): boolean {
    return this.vType.equal(TMark)
  }

  asMark(): MarkInfo {
    if (this.data === null) throw new Error('AsMark: nil data')
    return this.data as MarkInfo
  }

  isMove(): boolean {
    return this.vType.equal(TMove)
  }

  isInterpString(): boolean {
    return this.vType.equal(TInterpString)
  }

  isXml(): boolean {
    return this.vType.equal(TXml)
  }

  isXmlInterp(): boolean {
    return this.vType.equal(TXmlInterp)
  }

  asXmlTmpl(): XmlTmpl {
    return this.data as XmlTmpl
  }

  isParenExpr(): boolean {
    return this.vType.equal(TParenExpr)
  }

  asInterpSegments(): InterpSegment[] {
    if (!Array.isArray(this.data)) throw new Error('AsInterp: not an interp string')
    return this.data as InterpSegment[]
  }

  asMove(): MoveInfo {
    if (this.data === null) throw new Error('AsMove: nil data')
    return this.data as MoveInfo
  }

  /**
   * Stringify in the kernel's debug form — the SAME renders as Go's
   * Value.String (the stream-parity oracle compares these): words as
   * word(name), strings single-quoted, atoms bare, markers by their
   * structural spelling, paren-exprs as paren([...]).
   */
  toString(): string {
    if (this.isNone()) return 'none'
    if (this.data === null) {
      // A bare TYPE LITERAL renders by its LEAF name — `Integer`, not
      // `Scalar/Number/Integer`. Go is the reference here
      // (core.NewTypeLiteral(core.TInteger).String() is `Integer`) and
      // canonValue already drew the same line with vType.leaf(); only this
      // render used the full path.
      //
      // None is included deliberately, with no special case: only the none
      // VALUE renders lowercase, and isNone() above (data === NONE_SENTINEL)
      // has already taken it. A TNone arm here rendered the None ALTERNATIVE
      // of a union as `none` instead of `None`.
      return this.vType.leaf()
    }
    if (this.vType.equal(TSugar)) {
      return renderSugar(this.data as SugarInfo)
    }
    // Word-branch internal markers render by their structural spelling —
    // BEFORE the generic word arm, which would render `word(undefined)`
    // for payloads with no name. Mirrors the explicit arms in Go's
    // kernelFormatDefault.
    if (this.vType.equal(TOpenParen)) return '('
    if (this.vType.equal(TCloseParen)) return ')'
    if (this.vType.equal(TEnd)) return 'end'
    if (this.vType.equal(TParenExpr) && Array.isArray(this.data)) {
      return `paren([${(this.data as Value[]).map((v) => v.toString()).join(' ')}])`
    }
    if (this.vType.equal(TForward)) {
      const f = this.data as ForwardMarker
      return `forward(${f.funcName},${f.collected.length}/${f.expectedForward})`
    }
    if (this.vType.equal(TMark)) {
      return `mark(${(this.data as MarkInfo).id})`
    }
    if (this.vType.equal(TMove)) {
      return `move(${(this.data as MoveInfo).to},)`
    }
    if (this.vType.equal(TInterpString) && Array.isArray(this.data)) {
      // Source-form render — mirrors Go renderInterpParts byte for byte.
      return renderInterpSegments(this.data as InterpSegment[])
    }
    if (this.vType.equal(TXmlInterp)) {
      // Source-form skeleton render — mirrors Go renderXmlTmplSrc.
      return `interp-xml(${renderXmlTmplSrc(this.data as XmlTmpl)})`
    }
    if (this.data instanceof ChildType) {
      // Typed list/map (`[:T]` / `{:T}`), with any inline elements.
      const ct = this.data
      const open = this.vType.equal(TMap) ? '{' : '['
      const close = this.vType.equal(TMap) ? '}' : ']'
      const parts = [
        `:${ct.child.toString()}`,
        ...ct.elements.map((e) => e.toString()),
        ...ct.entries.map((e) => `${e.key}:${e.value.toString()}`),
      ]
      return `${open}${parts.join(' ')}${close}`
    }
    if (this.vType.equal(TXml) && this.data !== null) {
      return canonXml(this.data as XmlElement)
    }
    if (this.vType.matches(TWord)) {
      return `word(${(this.data as WordInfo).name})`
    }
    if (this.vType.matches(TString)) {
      return `'${this.data}'`
    }
    if (this.vType.matches(TInteger)) {
      return String(this.data)
    }
    // Arbitrary-precision literals render as the PARSEABLE `0d…` form, sign
    // before the marker — Go's FormatBigInteger / FormatBigDecimal
    // (core/go/value.go). Without these arms both fell through to
    // String(data) and dropped the prefix, so `0d123` rendered `123` — a
    // BigInteger became indistinguishable from an Integer, and the round
    // trip through the render was no longer parseable as the same value.
    // Eight rows of parser/spec/divergent.tsv were this one omission.
    if (this.vType.matches(TBigInteger)) {
      return formatBigInteger(this.data as bigint)
    }
    if (this.vType.matches(TBigDecimal)) {
      return formatBigDecimal(this.data as Decimal)
    }
    if (this.vType.matches(TFloat)) {
      return formatFloat(this.data as number)
    }
    if (this.vType.matches(TBoolean)) {
      return String(this.data)
    }
    if (this.vType.matches(TAtom)) {
      return String(this.data)
    }
    if (this.vType.matches(TList) && Array.isArray(this.data)) {
      const elems = (this.data as Value[]).map((v) => v.toString())
      return `[${elems.join(' ')}]`
    }
    if (this.data instanceof OrderedMap) {
      const m = this.data
      // D1: render in INSERTION order (design/FLEX-ATTRS.1.md §3), matching
      // Go's joinEntries (Keys()). sortedKeys() is retained only for
      // order-insensitive map equality (coretype.ts valuesEqual).
      const parts = m.keys().map((k) => `${k}:${m.get(k)!.toString()}`)
      return `{${parts.join(' ')}}`
    }
    // A disjunction renders as its alternatives joined by ` tor `, matching
    // Go's kernelFormatDefault (core/go/value.go:3660). Without this arm the
    // fallthrough below reached String(DisjunctInfo) and produced the literal
    // '[object Object]' — the defect the parser stream oracle was built to
    // catch and never ran to find (design/TS-PARITY-AUDIT.0.md).
    if (this.isDisjunct()) {
      return this.asDisjunct()
        .alternatives.map((alt) => alt.toString())
        .join(' tor ')
    }
    // Payload kinds whose debug render IS the canonical one. Go's
    // kernelFormatDefault and CanonValue agree on every one of these —
    // `a/b`, `/a/b`, `error(msg)`, `m.k/q` — and canonValue already
    // implemented them all while toString had no arm at all, so each of
    // these printed the literal '[object Object]'. Delegating keeps the two
    // renders from drifting apart again, which is how the disjunction arm
    // above came to be missing in the first place.
    if (this.canonRenders()) {
      return canonValue(this)
    }
    return String(this.data)
  }

  /**
   * True when canonValue has an arm that actually RECOGNISES this payload —
   * not merely when the vType says it should.
   *
   * The delegation above and canonValue's fallback (`return v.toString()`)
   * form a cycle, and the only thing that breaks it is agreement about which
   * values canonValue can render. Gating on vType alone did not agree:
   * canonValue's arms are guarded on payload SHAPE (`instanceof ErrorInfo`,
   * `'segments' in`, …), so a value whose type said Error but whose payload
   * was a bare string fell through every arm, hit the fallback, and came
   * straight back here — `new Value(TError, 'bad').toString()` overflowed the
   * stack rather than printing anything.
   *
   * So this predicate mirrors canon.ts's guards exactly. A payload that fails
   * it is malformed, and falls to `String(this.data)` — an unhelpful render
   * for an unrepresentable value, which is the correct outcome for a debug
   * stringifier and is what every other kind already does.
   */
  private canonRenders(): boolean {
    const d = this.data
    // Shape-tagged payloads carry their own proof; canonValue keys on the
    // class, not on the vType, and so does this.
    if (d instanceof ErrorInfo || d instanceof OptionsData) return true
    if (d === null || typeof d !== 'object') return false
    if (this.vType.equal(TPathon)) return 'segments' in d
    if (this.vType.equal(TEmailon)) return 'user' in d
    if (this.vType.equal(TUrlon)) return 'scheme' in d
    if (this.vType.equal(TReach)) {
      // canonReach walks receiver and segments; asReach admits any object, so
      // the arrays are what must exist before it is safe to enter.
      const r = d as Partial<ReachInfo>
      return Array.isArray(r.receiver) && Array.isArray(r.segments)
    }
    return false
  }
}

// ── Constructors ────────────────────────────────────────────────────────────

/**
 * Construct an integer value with VType = Scalar/Number/Integer.
 *
 * Earlier versions of this engine encoded the literal in the type
 * path (e.g. `Scalar/Number/Integer/42`) so that signature dispatch
 * could fire on specific numeric values via subtype matching. That
 * "value-tagged subtype" trick made `v.vType.equal(TInteger)` return
 * false for any concrete integer, bloated the type lattice per
 * literal payload, and forced consumers to use `matches` everywhere.
 * Specific-value dispatch now goes through `Signature.patterns`
 * instead — see PORT_OBSERVATIONS.5.md §1.1.
 *
 * BigInt is used because the Go engine carries int64 — TS's number
 * loses precision past 2^53.
 */
export function newInteger(n: bigint | number): Value {
  const big = typeof n === 'bigint' ? n : BigInt(n)
  return new Value(TInteger, big)
}

export function newFloat(f: number): Value {
  return new Value(TFloat, f)
}

/**
 * newBigInteger / newBigDecimal are the arbitrary-precision constructors,
 * the twins of Go's NewBigInteger / NewBigDecimal. core/ts declared both
 * TYPES from the start but neither constructor, so the only way to build
 * one was `new Value(TBigInteger, …)` at the use site — which is how
 * parser/ts came to have its own private pair, and how the render arms in
 * toString came to be missing (nothing in core ever produced one of these
 * values, so nothing in core ever rendered one).
 *
 * The BigDecimal payload is a Decimal (decimal.ts) — a scaled bigint, the
 * same model as Go's apd.Decimal. It was a binary64 until 2026-08-08,
 * which could not represent what Go represented: `0d1e400` overflowed to
 * Infinity, `0d1e-400` underflowed to zero, and `0d0.30` lost its scale.
 */
export function newBigInteger(n: bigint): Value {
  return new Value(TBigInteger, n)
}

export function newBigDecimal(d: Decimal): Value {
  return new Value(TBigDecimal, d)
}

/** Back-compat alias: the decimal scalar is named Float in the lattice. */
export const newDecimal = newFloat

/**
 * Construct a string value. The VType carries the concrete leaf so
 * `typeof` renders the precise name: a non-empty string is
 * Scalar/String/ProperString, an empty string is
 * Scalar/String/EmptyString. Mirrors eng/go/value.go::NewString.
 */
export function newString(s: string): Value {
  return new Value(s.length === 0 ? TStringEmpty : TStringProper, s)
}

export function newBoolean(b: boolean): Value {
  return new Value(TBoolean, b)
}

export function newAtom(name: string): Value {
  return new Value(TAtom, name)
}

export function newTypeLiteral(t: BoruType): Value {
  return new Value(t, null)
}

// The unique payload that marks the `none` value (None's sole
// inhabitant), distinguishing it from the `None` type literal (which
// has a null payload). Mirrors eng/go's noneSentinel.
const NONE_SENTINEL: unique symbol = Symbol('none')

/** Construct the unique `none` value. */
export function newNone(): Value {
  return new Value(TNone, NONE_SENTINEL)
}

/**
 * Error VALUE payload — the caught-error data the `do` escape hatch
 * surfaces (the Go twin is ErrorInfo in eng/go). Renders as
 * `error(<message>)` and types as Ideal/Error.
 */
export class ErrorInfo {
  readonly message: string
  constructor(message: string) {
    this.message = message
  }
}

export function newErrorValue(message: string): Value {
  return new Value(TError, new ErrorInfo(message))
}

export function isErrorValue(v: Value): boolean {
  return v.data instanceof ErrorInfo
}

/** XML element payload (static, post-interpolation). */
export interface XmlElement {
  tag: string
  attrs: { name: string; value: string }[]
  children: (string | XmlElement)[]
}

/** Construct an XML value (VType Node/Xml). */
export function newXml(elem: XmlElement): Value {
  return new Value(TXml, elem)
}

// XML interpolation template (pre-evaluation). Attribute values carry
// interp segments; children are literal text, a hole expression, or a
// nested template.
export type XmlAttrTmpl = { name: string; segs: InterpSegment[] }
export type XmlChildTmpl = { lit: string } | { expr: Value[] } | { elem: XmlTmpl }
export interface XmlTmpl {
  tag: string
  attrs: XmlAttrTmpl[]
  children: XmlChildTmpl[]
}

/** Construct an interpolated XML template value (VType Word/__XI). */
export function newXmlInterp(t: XmlTmpl): Value {
  return new Value(TXmlInterp, t)
}

/** Path payload: normalised segments + absolute flag. */
export interface PathonInfo {
  segments: string[]
  abs: boolean
}

/** Construct a path value (VType Scalar/Micron/Pathon). */
export function newPath(segments: string[], abs: boolean): Value {
  return new Value(TPathon, { segments, abs } satisfies PathonInfo)
}

/** Emailon payload: primary fields (address is derived at render). */
export interface EmailonInfo {
  user: string
  host: string
}

export function newEmailon(user: string, host: string): Value {
  return new Value(TEmailon, { user, host } satisfies EmailonInfo)
}

/** Urlon payload: primary fields (href is derived at render). */
export interface UrlonInfo {
  scheme: string
  host: string
  port?: number
  path?: string
  query?: string
  fragment?: string
}

export function newUrlon(info: UrlonInfo): Value {
  return new Value(TUrlon, info)
}

/** Canonical Urlon render: scheme://host[:port][path][?query][#fragment]. */
export function urlonHref(u: UrlonInfo): string {
  let out = `${u.scheme}://${u.host}`
  if (u.port !== undefined) out += `:${u.port}`
  if (u.path) out += u.path
  if (u.query) out += `?${u.query}`
  if (u.fragment) out += `#${u.fragment}`
  return out
}

export function newWord(name: string): Value {
  return new Value(TWord, { name } satisfies WordInfo)
}

/**
 * Construct a binding-name word carrying a container type constraint,
 * used by the spec tokenizer for `def NAME:[…]` / `def NAME:{…}` forms.
 */
export function newConstrainedWord(name: string, constraint: Value): Value {
  return new Value(TWord, { name, constraint } satisfies WordInfo)
}

/** Convenience for constructing an Any-typed carrier (test harness use). */
export function newAny(data: unknown): Value {
  return new Value(TAny, data)
}

/**
 * Construct a strict check-mode carrier: a type-only value of type `t`
 * (no concrete payload) that matches signatures as a value of that type
 * and propagates its type through dispatch. Mirrors Go NewCarrier.
 */
export function newCarrier(t: BoruType): Value {
  return new Value(t, null, { carrier: true })
}

/**
 * Construct a gradual (dynamic) carrier — a type-only value whose type
 * is statically unknown or arrived from an unannotated word. It matches
 * optimistically and its contagion widens results to dynamic. Mirrors
 * Go NewDynamicCarrier.
 */
export function newDynamicCarrier(t: BoruType): Value {
  return new Value(t, null, { carrier: true, dynamic: true })
}

/**
 * Construct a list value with VType = Node/List. Data is the array
 * of element Values. The `eval` option controls auto-evaluation when
 * the list is consumed as a word argument: parser/tokenizer-built
 * lists pass true; lists synthesised by handlers (e.g. quote, fn
 * params) pass false. Mirrors Go's NewList vs NewEvalList split.
 */
export function newList(elems: Value[], opts?: { eval?: boolean; quoted?: boolean }): Value {
  return new Value(TList, elems, { eval: opts?.eval, quoted: opts?.quoted })
}

/**
 * OrderedMap preserves key insertion order. Mirrors
 * eng/go/value.go::OrderedMap (the subset the spec reaches).
 */
export class OrderedMap {
  private readonly _keys: string[] = []
  private readonly vals = new Map<string, Value>()

  set(key: string, val: Value): void {
    if (!this.vals.has(key)) this._keys.push(key)
    this.vals.set(key, val)
  }

  get(key: string): Value | undefined {
    return this.vals.get(key)
  }

  has(key: string): boolean {
    return this.vals.has(key)
  }

  keys(): string[] {
    return [...this._keys]
  }

  /** Keys in sorted order — the canonical render order. */
  sortedKeys(): string[] {
    return [...this._keys].sort()
  }

  get size(): number {
    return this._keys.length
  }
}

/** Construct a map value with VType = Node/Map wrapping an OrderedMap. */
export function newMap(m: OrderedMap, opts?: { eval?: boolean; quoted?: boolean }): Value {
  // The flags are optional for the same reason newList's are: Go has both
  // NewMap and NewEvalMap, and the difference decides whether the step
  // loop auto-evaluates the map's values at all. A caller that omits them
  // gets the runtime form (no auto-eval), which is what NewMap builds.
  return new Value(TMap, m, { eval: opts?.eval, quoted: opts?.quoted })
}

/**
 * OptionsData marks an Options instance (an Ideal/Options value). It
 * has VType Map (so typeof reports Map) but renders as `options{…}`.
 */
export class OptionsData {
  readonly map: OrderedMap
  constructor(map: OrderedMap) {
    this.map = map
  }
}

/** Construct an Options instance (`make Options {…}`). */
export function newOptions(map: OrderedMap): Value {
  return new Value(TMap, new OptionsData(map))
}

/**
 * A class TYPE built by `class {…}` / `refine Base {…}`.
 * `name` is empty until a `def Name …` binding stamps it. `parentPath`
 * is the lattice path of the parent (the base `Class`, or another
 * class type's path like `Class/Bar`). `fields` maps field name →
 * type literal, with inherited fields first. Mirrors ClassTypeInfo in
 * eng/go.
 */
export class ClassTypeInfo {
  name: string
  readonly parentPath: string
  readonly fields: OrderedMap
  constructor(name: string, parentPath: string, fields: OrderedMap) {
    this.name = name
    this.parentPath = parentPath
    this.fields = fields
  }
  /** The full lattice path of this class type, once named. */
  get path(): string {
    return `${this.parentPath}/${this.name}`
  }
}

/** Construct a class-type value (a Type-branch value carrying ClassTypeInfo). */
export function newClassType(info: ClassTypeInfo): Value {
  return new Value(TType, info)
}

/**
 * A segment of an interpolated string: literal text, or an expression
 * (token list) to evaluate and stringify. Mirrors InterpPart in eng/go.
 */
export type InterpSegment = { lit: string } | { expr: Value[] }

/** Construct an interpolated-string value (VType Word/__IS). */
export function newInterpString(segments: InterpSegment[]): Value {
  return new Value(TInterpString, segments)
}

/**
 * Construct a paren-expression value (VType Word/__PE) holding the
 * tokens of a `( … )` group. In data context it evaluates and collapses
 * to a single residual value (unlike a list, which stays a list).
 */
export function newParenExpr(tokens: Value[]): Value {
  return new Value(TParenExpr, tokens)
}

/**
 * Construct an inspection map (VType Node/Map/Inspect). Renders in
 * insertion order with word values bare (e.g. `kind:native`), unlike a
 * plain map which sorts keys.
 */
export function newInspect(m: OrderedMap): Value {
  return new Value(TInspect, m)
}

/**
 * ChildType backs a typed list `[:T]` / typed map `{:T}` (and their
 * concrete-with-elements forms `[:T 1 2 3]`). Mirrors the
 * ChildTypeInfo payload in eng/go/value.go. `child` is the element/value
 * type constraint (a type literal); `elements` carries concrete
 * elements when present.
 */
export class ChildType {
  readonly child: Value
  readonly elements: Value[]
  /** Typed-MAP inline entries (`{:T a:1}`), in source order. */
  readonly entries: { key: string; value: Value }[]
  constructor(child: Value, elements: Value[] = [], entries: { key: string; value: Value }[] = []) {
    this.child = child
    this.elements = elements
    this.entries = entries
  }
}

/** Construct a typed list value (VType Node/List, ChildType payload). */
export function newTypedList(child: Value, elements: Value[] = []): Value {
  return new Value(TList, new ChildType(child, elements))
}

/** Construct a typed map value (VType Node/Map, ChildType payload). */
export function newTypedMap(child: Value, entries: { key: string; value: Value }[] = []): Value {
  return new Value(TMap, new ChildType(child, [], entries))
}

/**
 * DisjunctInfo holds the alternatives of a disjunction (union) type, or
 * the members of an enum. Mirrors eng/go/value.go::DisjunctInfo.
 */
export interface DisjunctInfo {
  alternatives: Value[]
}

/** Construct a Disjunct (union) value — VType Type/Disjunct. */
export function newDisjunct(alternatives: Value[]): Value {
  return new Value(TDisjunct, { alternatives } satisfies DisjunctInfo)
}

/** Construct a FunctionSignature (fnsig) value — VType Type/FunctionSignature. */
export function newFnUndef(sigs: Value[]): Value {
  return new Value(TFnUndef, sigs)
}

/** Construct an Enum value — VType Type/Disjunct/Enum (a Disjunct subtype). */
export function newEnum(alternatives: Value[]): Value {
  return new Value(TEnum, { alternatives } satisfies DisjunctInfo)
}

/** Clone a Value with `quoted=true` (used by `quote` for lists). */
export function withQuoted(v: Value): Value {
  return new Value(v.vType, v.data, { eval: v.eval, quoted: true, carrier: v.carrier })
}

/**
 * Construct a function-definition value. VType = Word/Function so
 * sigTypeMatches can route fn refs through TFunction sig slots, and
 * the engine's def-sub branch knows to dispatch instead of substitute.
 */
export function newFnDef(info: FnDefInfo): Value {
  return new Value(TFunction, info)
}

/**
 * Construct a forward-dispatch marker. The engine inserts these in
 * place of a function word whose match still needs forward args; as
 * those args resolve they're collected into `marker.collected` and
 * the marker fires once full.
 */
export function newForwardMarker(info: ForwardMarker): Value {
  return new Value(TForward, info)
}

/**
 * MarkInfo / MoveInfo: control-flow primitives. A handler emits a
 * Mark followed by a body sequence followed by a Move; when the
 * engine reaches the Move it walks back to the matching Mark,
 * splices the original body in place of [Mark .. body .. Move], and
 * sets the pointer to the start so the body re-runs. One-shot
 * replay only — the body is removed when the move fires.
 */
export interface MarkInfo {
  id: string
  body: Value[]
}

export interface MoveInfo {
  to: string
}

export function newMark(id: string, body: Value[]): Value {
  return new Value(TMark, { id, body } satisfies MarkInfo)
}

export function newMove(to: string): Value {
  return new Value(TMove, { to } satisfies MoveInfo)
}

/**
 * Reach — the `.` / `!.` access chain the parser folds into one value
 * (`m.a.b`); the engine lowers it to a get/getr token chain. Mirrors
 * Go ReachInfo / ReachSeg (eng/go/payload.go).
 */
export interface ReachSeg {
  getr: boolean
  computed: boolean
  keyLit?: Value
  keyExpr?: Value[]
}

export interface ReachInfo {
  receiver: Value[]
  segments: ReachSeg[]
  /** Evaluate-by-default (like list eval); quote/codequote suppress. */
  eval: boolean
}

export function newReach(info: ReachInfo): Value {
  return new Value(TReach, info)
}

export function isReach(v: Value): boolean {
  return v.vType.equal(TReach)
}

export function asReach(v: Value): ReachInfo | undefined {
  if (!isReach(v) || v.data === null || typeof v.data !== 'object') return undefined
  return v.data as ReachInfo
}

/**
 * Splice marker (`Word/__SP`): fires when stepped at the pointer,
 * replaced unevaluated by its payload. Mirrors Go NewSplice.
 */
export function newSplice(payload: Value): Value {
  return new Value(TSplice, { payload })
}

/**
 * Dispatch-mod marker (`Word/__DM`): `/r` (leave the function as
 * data) / `/q` (treat the result as data) group modifiers, emitted
 * AFTER the group. Mirrors Go DispatchModInfo.
 */
export interface DispatchModInfo {
  ref: boolean
  quote: boolean
}

export function newDispatchMod(info: DispatchModInfo): Value {
  return new Value(TDispatchMod, info)
}

/**
 * Typed tape-structure markers: the parser emits these for `(`, `)`
 * and `end` / `;` so the engine recognises them by lattice identity
 * (mirrors Go NewOpenParen/NewCloseParen/NewEnd). Each carries its
 * source spelling as a WordInfo payload; the dual predicates below
 * accept BOTH the typed marker and the legacy fixture spelling
 * (a plain Word named '(' / ')' / 'end') so the engine handles either
 * stream during the tokenizer→parser transition.
 */
export function newOpenParen(): Value {
  return new Value(TOpenParen, { name: '(' } satisfies WordInfo)
}

export function newCloseParen(): Value {
  return new Value(TCloseParen, { name: ')' } satisfies WordInfo)
}

export function newEnd(): Value {
  return new Value(TEnd, { name: 'end' } satisfies WordInfo)
}

function wordNamed(v: Value, name: string): boolean {
  return v.vType.equal(TWord) && (v.data as WordInfo | null)?.name === name
}

export function isOpenParen(v: Value): boolean {
  return v.vType.equal(TOpenParen) || wordNamed(v, '(')
}

export function isCloseParen(v: Value): boolean {
  return v.vType.equal(TCloseParen) || wordNamed(v, ')')
}

export function isEnd(v: Value): boolean {
  return v.vType.equal(TEnd) || wordNamed(v, 'end')
}

/**
 * Sugar markers — the parser-side half of ADR-012 rule 3 (2026-08-04
 * amendment): the parser emits STRUCTURE, never names. Each piece of
 * surface sugar that historically desugared to word tokens inside the
 * parser (`/u /s /f /N`, `X/t`, `+kind<…>` minilang literals, `=>`,
 * `Name<Args>`) parses to ONE kernel marker value (`Word/__SG`,
 * SugarInfo payload). When the marker is stepped, the engine lowers it
 * to word dispatches, resolving each word through the sugar-role table
 * the language layer binds at registration (Registry.bindSugarWord). A
 * registry with no binding for a stepped role fails loudly
 * (`sugar_unbound`). Mirrors eng/go/sugar.go.
 */
export type SugarKind =
  | 'usurp' // `/u` — take over the preceding group's dispatch
  | 'stack-args' // `/s` — force stack collection
  | 'forward-args' // `/f` — force forward collection
  | 'force-arity' // `/N` — filter signatures by arity N
  | 'mini' // `+kind<src>` minilang literal
  | 'lambda' // `=>` — the anonymous-fn constructor
  | 'angle' // `Name<Args>` — generic head or use-site
  | 'type-bound' // `X/t` — bounded-Type sugar
  | 'gen-head' // binding-only: the generic-def head word
  | 'gen-apply' // binding-only: the generic-apply word
  | 'gen-default' // `=` in a gen-param entry — the default-value word

/** The Word/__SG marker payload. One shape covers every kind. */
export interface SugarInfo {
  kind: SugarKind
  /** SugarForceArity: the arity. */
  n?: bigint
  /** mini: the minilang kind; angle: the receiver name. */
  name?: string
  /** mini: the raw literal source. */
  src?: string
  /** angle: the converted gen-params list (head form). */
  head?: Value
  /** angle: why the head form is unavailable (surfaced only if selected). */
  headErr?: string
  /** angle: the converted use-site type args; type-bound: the bound tokens. */
  items?: Value[]
}

export function newSugar(info: SugarInfo): Value {
  return new Value(TSugar, info)
}

export function isSugar(v: Value): boolean {
  return v.vType.equal(TSugar)
}

export function asSugar(v: Value): SugarInfo | undefined {
  if (!isSugar(v) || v.data === null || typeof v.data !== 'object') return undefined
  return v.data as SugarInfo
}


/**
 * Render interpolated-string segments in source form — mirrors Go
 * renderInterpParts byte for byte (`interp('a ' ${word(x)})`).
 */
export function renderInterpSegments(segments: InterpSegment[]): string {
  const parts: string[] = []
  for (const s of segments) {
    if ('expr' in s) {
      parts.push('${' + s.expr.map((t) => t.toString()).join(' ') + '}')
    } else {
      parts.push("'" + s.lit + "'")
    }
  }
  return 'interp(' + parts.join(' ') + ')'
}

/**
 * Render an XML template skeleton in source form — mirrors Go
 * renderXmlTmplSrc (`<tag attr="lit${expr}">text${hole}</tag>`).
 */
export function renderXmlTmplSrc(t: XmlTmpl): string {
  let b = '<' + t.tag
  for (const a of t.attrs) {
    b += ' ' + a.name + '="'
    for (const p of a.segs) {
      if ('expr' in p) {
        b += '${' + p.expr.map((x) => x.toString()).join(' ') + '}'
      } else {
        b += p.lit
      }
    }
    b += '"'
  }
  if (t.children.length === 0) return b + '/>'
  b += '>'
  for (const c of t.children) {
    if ('lit' in c) b += c.lit
    else if ('expr' in c) b += '${' + c.expr.map((x) => x.toString()).join(' ') + '}'
    else b += renderXmlTmplSrc(c.elem)
  }
  return b + '</' + t.tag + '>'
}

/** Render a marker exactly as the Go kernel's String arm does. */
export function renderSugar(info: SugarInfo): string {
  switch (info.kind) {
    case 'force-arity':
      return `sugar(${info.kind} ${info.n})`
    case 'mini':
      return `sugar(${info.kind} ${info.name} '${info.src}')`
    case 'angle':
      return `sugar(${info.kind} ${info.name} [${(info.items ?? []).map(String).join(' ')}])`
    case 'type-bound':
      return `sugar(${info.kind} [${(info.items ?? []).map(String).join(' ')}])`
  }
  return `sugar(${info.kind})`
}
