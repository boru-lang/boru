// Arbitrary-precision decimal — the TS twin of Go's apd.Decimal, for the
// BigDecimal payload.
//
// core/ts has NO dependencies and is meant to keep it that way (the module
// exists precisely so the TS core needs nothing), so this is a deliberately
// small implementation rather than a decimal library: a bigint `unscaled`
// and an integer `scale`, together denoting unscaled x 10^-scale. That is
// exactly apd's model, which is what makes byte-identical rendering
// achievable.
//
// It carries no arithmetic. Nothing in core/ts does arithmetic on a
// BigDecimal today — the parser builds one and canon renders it — and a
// half-implemented `add` would be worse than none. When BigDecimal maths
// arrives it goes here, against apd's semantics.
//
// This replaced a binary64 payload, which could not represent what Go
// represented: `0d1e400` overflowed to Infinity and rendered the
// unparseable `0dInfinity`, `0d1e-400` underflowed and rendered `0d0` —
// silently turning a nonzero value into zero — and `0d0.30` lost the
// trailing-zero scale. All three were live rows in
// parser/spec/divergent.tsv.

export class Decimal {
  /** The significand, sign included. */
  readonly unscaled: bigint
  /**
   * Digits after the decimal point. NEGATIVE means trailing zeros before
   * it — `1e2` is unscaled 1, scale -2. Preserved exactly as written, so
   * `0.30` (scale 2) does not collapse to `0.3` (scale 1); apd keeps the
   * same distinction and `Text('f')` shows it.
   */
  readonly scale: number

  constructor(unscaled: bigint, scale: number) {
    this.unscaled = unscaled
    this.scale = scale
  }

  /**
   * Renders the plain 'f' form at every magnitude — never scientific,
   * matching apd's `Text('f')`. Trailing zeros are KEPT: the scale is part
   * of the value's identity here, not noise to normalise away.
   */
  toString(): string {
    const neg = this.unscaled < 0n
    const digits = (neg ? -this.unscaled : this.unscaled).toString()
    const sign = neg ? '-' : ''
    if (this.scale <= 0) {
      // Whole, with -scale trailing zeros. A zero significand stays "0"
      // rather than growing a run of zeros after it.
      if (0n === this.unscaled) return sign + '0'
      return sign + digits + '0'.repeat(-this.scale)
    }
    if (digits.length > this.scale) {
      const cut = digits.length - this.scale
      return sign + digits.slice(0, cut) + '.' + digits.slice(cut)
    }
    // The point falls at or left of the leading digit: pad to reach it.
    return sign + '0.' + '0'.repeat(this.scale - digits.length) + digits
  }
}

/**
 * decimalFromString parses a decimal literal — optional sign, digits, an
 * optional fraction, an optional exponent — EXACTLY, with no binary64 in
 * the path. Returns undefined when the text is not a decimal literal, so
 * callers raise their own diagnostic rather than inheriting one.
 *
 * The `0d` marker is NOT accepted here: that is the parser's spelling of a
 * big literal, and this function is about the number.
 */
export function decimalFromString(src: string): Decimal | undefined {
  const m = /^([+-]?)(\d*)(?:\.(\d*))?(?:[eE]([+-]?\d+))?$/.exec(src)
  if (null === m) return undefined
  const sign = '-' === m[1] ? -1n : 1n
  const intPart = m[2] ?? ''
  const fracPart = m[3] ?? ''
  // A lone sign, a bare `.`, or an exponent with no significand is not a
  // number — the regex above admits those shapes, so reject them here.
  if ('' === intPart && '' === fracPart) return undefined
  const exp = undefined === m[4] ? 0 : Number.parseInt(m[4], 10)
  const digits = intPart + fracPart
  return new Decimal(sign * BigInt(digits), fracPart.length - exp)
}
