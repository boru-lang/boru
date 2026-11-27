/-
  Boru.Core.Number — the numeric payloads and their source forms.

  The kernel holds four numeric leaves: `Integer` (signed 64-bit),
  `Float` (IEEE 754 binary64), `BigInteger` and `BigDecimal` (arbitrary
  precision). The kernel does no arithmetic of its own — words do — so
  what it owns about a number is its IDENTITY and its CANONICAL SOURCE
  FORM, which must read back as the same value of the same type.

  Floats are modelled exactly, as their 64 bits, and both directions of
  the decimal conversion are done in integer arithmetic. Nothing here
  depends on a host floating-point implementation, so every definition
  reduces in the kernel and can be proved about.
-/

namespace Boru.Core

/-! ## Decimal digits -/

/-- Decimal digits of a natural, most significant first. -/
def natDigits (n : Nat) : String := toString n

def zeros (n : Nat) : String := String.ofList (List.replicate n '0')

/-! ## BigDecimal -/

/-- An Integer is a signed 64-bit integer: arithmetic that leaves the
    range WRAPS. (core/go holds one in an `int64`; the words that add
    Integers check the range themselves, and what does not — a counted
    loop's own index — wraps.) -/
def wrapInt64 (n : Int) : Int :=
  let m := n % (2 ^ 64 : Int)
  if m ≥ (2 ^ 63 : Int) then m - (2 ^ 64 : Int) else m

/-- An arbitrary-precision decimal: `(-1)^neg · coeff · 10^exp`. The
    exponent and the sign of zero are part of the IDENTITY, not noise:
    `0.00`, `0` and `-0.0` are three different values. -/
structure Decimal where
  neg : Bool
  coeff : Nat
  exp : Int
  deriving Repr, BEq, DecidableEq

namespace Decimal

/-- Plain positional text, never scientific: the coefficient's digits
    with the exponent applied as trailing zeros or as a decimal point. -/
def plain (d : Decimal) : String :=
  let ds := natDigits d.coeff
  if d.exp ≥ 0 then ds ++ zeros d.exp.toNat
  else
    let n := (-d.exp).toNat
    if ds.length > n then
      let cs := ds.toList
      String.ofList (cs.take (ds.length - n)) ++ "." ++ String.ofList (cs.drop (ds.length - n))
    else "0." ++ zeros (n - ds.length) ++ ds

/-- The canonical source form: the `0d` marker, with the sign BEFORE it. -/
def canon (d : Decimal) : String :=
  (if d.neg then "-0d" else "0d") ++ d.plain

end Decimal

/-- The canonical source form of a BigInteger: `0d123`, `-0d789`. -/
def canonBigInt (n : Int) : String :=
  if n < 0 then "-0d" ++ natDigits n.natAbs else "0d" ++ natDigits n.natAbs

/-! ## Reading a decimal literal

`[-]digits[.digits][e[+-]digits]`, as the corpus notation and the parser's
numeric literals spell it. -/

structure DecLit where
  neg : Bool
  coeff : Nat
  exp : Int
  deriving Repr

private def digitsVal (cs : List Char) : Option Nat :=
  if cs.isEmpty || !cs.all Char.isDigit then none
  else some (cs.foldl (fun acc c => acc * 10 + (c.toNat - '0'.toNat)) 0)

private def readInt (cs : List Char) : Option Int :=
  match cs with
  | '-' :: rest => (digitsVal rest).map (fun n => -(n : Int))
  | '+' :: rest => (digitsVal rest).map (fun n => (n : Int))
  | _ => (digitsVal cs).map (fun n => (n : Int))

def readDecLit (s : String) : Option DecLit :=
  let cs := s.toList
  let (neg, cs) := match cs with
    | '-' :: rest => (true, rest)
    | '+' :: rest => (false, rest)
    | _ => (false, cs)
  let (mant, ex) := match cs.span (fun c => c != 'e' && c != 'E') with
    | (m, []) => (m, ([] : List Char))
    | (m, _ :: e) => (m, e)
  let (ip, fp) := match mant.span (· != '.') with
    | (i, []) => (i, ([] : List Char))
    | (i, _ :: f) => (i, f)
  if ip.isEmpty && fp.isEmpty then none
  else if !(ip ++ fp).all Char.isDigit then none
  else
    let coeff := (ip ++ fp).foldl (fun acc c => acc * 10 + (c.toNat - '0'.toNat)) 0
    let e? : Option Int := if ex.isEmpty then (if s.toList.any (fun c => c == 'e' || c == 'E') then none else some 0) else readInt ex
    e?.map fun e => { neg, coeff, exp := e - (fp.length : Int) }

def Decimal.ofString? (s : String) : Option Decimal :=
  (readDecLit s).map fun l => { neg := l.neg, coeff := l.coeff, exp := l.exp }

/-! ## Float — IEEE 754 binary64, held as its bits -/

/-- A binary64 value, identified by its bit pattern. -/
structure F64 where
  bits : Nat
  deriving Repr, BEq, DecidableEq

namespace F64

def signBit (f : F64) : Bool := f.bits / 2 ^ 63 % 2 == 1
def biasedExp (f : F64) : Nat := f.bits / 2 ^ 52 % 2 ^ 11
def fraction (f : F64) : Nat := f.bits % 2 ^ 52

def isNaN (f : F64) : Bool := f.biasedExp == 2047 && f.fraction != 0
def isInf (f : F64) : Bool := f.biasedExp == 2047 && f.fraction == 0
def isZero (f : F64) : Bool := f.biasedExp == 0 && f.fraction == 0

def mk' (neg : Bool) (biased frac : Nat) : F64 :=
  ⟨(if neg then 2 ^ 63 else 0) + biased * 2 ^ 52 + frac⟩

def inf (neg : Bool) : F64 := mk' neg 2047 0
def zero (neg : Bool) : F64 := mk' neg 0 0

/-- The magnitude of a finite value as `m · 2^e`, `m` an integer. -/
def magnitude (f : F64) : Nat × Int :=
  if f.biasedExp == 0 then (f.fraction, -1074)
  else (2 ^ 52 + f.fraction, (f.biasedExp : Int) - 1075)

/-- `num / den` scaled by `2^(-e)`, as a floor and "is there a remainder,
    and how does it compare with one half". -/
private def scaledDiv (num den : Nat) (e : Int) : Nat × Nat × Nat :=
  -- value = num / den / 2^e
  let (n, d) := if e ≥ 0 then (num, den * 2 ^ e.toNat) else (num * 2 ^ (-e).toNat, den)
  (n / d, n % d, d)

/-- Round `num / den` (both positive) to the nearest binary64, ties to
    even. Exact: integer arithmetic only. -/
def ofRatio (neg : Bool) (num den : Nat) : F64 :=
  if num == 0 then zero neg
  else
    -- first estimate of the binary exponent of the 53-bit mantissa
    let e0 : Int := (num.log2 : Int) - (den.log2 : Int) - 52
    -- settle e so that 2^52 ≤ floor(v / 2^e) < 2^53, within the subnormal floor
    let fix (e : Int) : Int :=
      let (q, _, _) := scaledDiv num den e
      if q ≥ 2 ^ 53 then e + 1 else if q < 2 ^ 52 then e - 1 else e
    let e := fix (fix e0)
    let e := if e < -1074 then -1074 else e
    let (q, r, d) := scaledDiv num den e
    let q := if 2 * r > d || (2 * r == d && q % 2 == 1) then q + 1 else q
    let (q, e) := if q ≥ 2 ^ 53 then (q / 2, e + 1) else (q, e)
    if e > 971 then inf neg
    else if q < 2 ^ 52 then mk' neg 0 q
    else mk' neg (e + 1075).toNat (q - 2 ^ 52)

/-- The binary64 nearest a decimal literal. -/
def ofDecLit (l : DecLit) : F64 :=
  if l.exp ≥ 0 then ofRatio l.neg (l.coeff * 10 ^ l.exp.toNat) 1
  else ofRatio l.neg l.coeff (10 ^ (-l.exp).toNat)

def ofString? (s : String) : Option F64 :=
  match s with
  | "nan" => some ⟨2047 * 2 ^ 52 + 2 ^ 51⟩
  | "inf" | "+inf" => some (inf false)
  | "-inf" => some (inf true)
  | _ => (readDecLit s).map ofDecLit

/-- `a/b ≤ c/d`-style comparisons on positive rationals `n · 2^e`. -/
private def ratOfScaled (m : Nat) (e : Int) : Nat × Nat :=
  if e ≥ 0 then (m * 2 ^ e.toNat, 1) else (m, 2 ^ (-e).toNat)

/-- The shortest decimal `digits · 10^p` that reads back as `f` (finite,
    non-zero), preferring the candidate nearest `f`. Returns the digits
    with no trailing zeros and the exponent of the LAST digit. -/
def shortest (f : F64) : Nat × Int :=
  let (m, e) := f.magnitude
  -- v = m·2^e; the half-gaps to the neighbours, as rationals over 2^(e-2)
  let lowerBoundary := f.biasedExp > 1 && f.fraction == 0
  -- everything scaled by 4 so the bounds are integers times 2^(e-2)
  let v4 := 4 * m
  let lo4 := if lowerBoundary then 4 * m - 1 else 4 * m - 2
  let hi4 := 4 * m + 2
  let incl := m % 2 == 0
  let (vn, vd) := ratOfScaled v4 (e - 2)
  let (ln, ld) := ratOfScaled lo4 (e - 2)
  let (hn, hd) := ratOfScaled hi4 (e - 2)
  -- is the positive rational cn/cd inside the rounding interval?
  let inside (cn cd : Nat) : Bool :=
    let geLo := if incl then cn * ld ≥ ln * cd else cn * ld > ln * cd
    let leHi := if incl then cn * hd ≤ hn * cd else cn * hd < hn * cd
    geLo && leHi
  -- floor(log10 v): the largest k with 10^k ≤ v
  let k0 : Int := ((vn.log2 : Int) - (vd.log2 : Int)) * 30103 / 100000
  let le10 (k : Int) : Bool :=  -- 10^k ≤ v
    if k ≥ 0 then 10 ^ k.toNat * vd ≤ vn else vd ≤ vn * 10 ^ (-k).toNat
  let k := if le10 (k0 + 1) then k0 + 1 else if le10 k0 then k0 else k0 - 1
  -- try n = 1 .. 17 significant digits
  let rec go (fuel n : Nat) : Nat × Int :=
    match fuel with
    | 0 => (m, e)  -- unreachable: 17 digits always round-trip
    | fuel + 1 =>
      let p : Int := k - ((n : Int) - 1)
      -- d = floor(v / 10^p)
      let (qn, qd) := if p ≥ 0 then (vn, vd * 10 ^ p.toNat) else (vn * 10 ^ (-p).toNat, vd)
      let dDown := qn / qd
      let dUp := dDown + 1
      let asRat (d : Nat) : Nat × Nat := if p ≥ 0 then (d * 10 ^ p.toNat, 1) else (d, 10 ^ (-p).toNat)
      let okDown := dDown > 0 && (let (cn, cd) := asRat dDown; inside cn cd)
      let okUp := (let (cn, cd) := asRat dUp; inside cn cd)
      -- distance comparison: is dUp strictly closer to v than dDown?  (2·rem > qd)
      let rem := qn % qd
      let upCloser := 2 * rem > qd || (2 * rem == qd && dDown % 2 == 1)
      if okDown && okUp then (if upCloser then (dUp, p) else (dDown, p))
      else if okDown then (dDown, p)
      else if okUp then (dUp, p)
      else go fuel (n + 1)
  let (d, p) := go 17 1
  -- strip trailing zeros
  let rec strip (fuel d : Nat) (p : Int) : Nat × Int :=
    match fuel with
    | 0 => (d, p)
    | fuel + 1 => if d != 0 && d % 10 == 0 then strip fuel (d / 10) (p + 1) else (d, p)
  strip 400 d p

/-- Positional rendering of `digits · 10^p`. -/
private def positional (d : Nat) (p : Int) : String :=
  (Decimal.mk false d p).plain

/-- Exponent rendering `d.ddde±XX`, at least two exponent digits. -/
private def scientific (d : Nat) (p : Int) : String :=
  let ds := (natDigits d).toList
  let e10 : Int := p + ((ds.length : Int) - 1)
  let mant := match ds with
    | [c] => String.ofList [c]
    | c :: rest => String.ofList (c :: '.' :: rest)
    | [] => "0"
  let es := natDigits e10.natAbs
  mant ++ "e" ++ (if e10 < 0 then "-" else "+") ++ (if es.length < 2 then "0" ++ es else es)

/-- The canonical source form of a Float. A decimal point is always
    present, so the form reads back as a Float and never as an Integer;
    magnitudes outside `[1e-10, 1e21)` use an exponent. -/
def canon (f : F64) : String :=
  if f.isNaN then "nan"
  else if f.isInf then (if f.signBit then "-inf" else "inf")
  else if f.isZero then (if f.signBit then "-0.0" else "0.0")
  else
    let (d, p) := f.shortest
    let (m, e) := f.magnitude
    -- |f| ≥ 1e21  or  |f| < 1e-10
    let (vn, vd) := ratOfScaled m e
    let big := vn ≥ 10 ^ 21 * vd
    let small := vn * 10 ^ 10 < vd
    let body :=
      if big || small then scientific d p
      else
        let s := positional d p
        if s.toList.contains '.' then s else s ++ ".0"
    (if f.signBit then "-" else "") ++ body

end F64

end Boru.Core
