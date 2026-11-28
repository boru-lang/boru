/-
  Boru.Core.Compare — the one total order.

  Every pair of values is ordered, whatever their types
  (core/go/compare.go `CompareValues`). The answer comes from a cascade:

    1. the nearest common ancestor of the two types that owns a FAMILY
       comparison decides — numbers by magnitude across leaves, strings
       and atoms by text, booleans false-first, and any two scalars of
       different families by lattice rank;
    2. with no family comparison, the lattice RANK of the two types
       decides (then depth, then name);
    3. two values of one type compare by size, then structurally — lists
       element by element, maps by sorted keys then values, anything else
       by canonical text.

  One rule runs through every family: a bare TYPE sorts before any
  concrete value of that family, and two bare types sort by rank.
-/
import Boru.Core.Canon
import Boru.Core.Builtin

namespace Boru.Core

/-! ## Ordering types -/

/-- The rank of a node: its own, or the nearest ancestor's that has one.
    A MINTED node has the rank of its branch's EXTERNAL BAND (core/go
    `externalBandFor`): one rank for every type a program defines under
    `Scalar`, one under `Node`, and so on — above every builtin of the
    branch. -/
def typeRank (t : TypeId) : Nat :=
  let inherited := ((typeChain t).filterMap fun p => (builtin.find? p).bind fun d =>
    if d.rank != 0 then some d.rank else none).headD 0
  if (mintedParent? t).isNone then inherited
  else match (t.splitOn "/").headD "" with
    | "Scalar" => 21000000000
    | "Node" => 31000000000
    | "Ideal" => 41000000000
    | "Word" => 51000000000
    | "Type" => 61000000000
    | _ => inherited

/-- Rank, then depth, then leaf name — and two minted nodes of one name
    in the order they were minted. -/
def compareTypes (a b : TypeId) : Ordering :=
  if a == b then .eq
  else match compare (typeRank a) (typeRank b) with
    | .eq =>
      match compare (typeDepth a) (typeDepth b) with
      | .eq =>
        (match compare (leafOf a) (leafOf b) with
         | .eq => compare (mintedIdent a) (mintedIdent b)
         | o => o)
      | o => o
    | o => o

/-- The flexible container types order as their base family. -/
def nodeFamily (t : TypeId) : TypeId :=
  if t == "Node/Map/FlexMap" || t == "Node/Map/FlexMap/WeakFlexMap" then TMap
  else if t == "Node/List/FlexList" || t == "Node/List/FlexList/WeakFlexList" then TList
  else t

/-! ## Exact numeric comparison -/

/-- A rational as numerator over positive denominator. -/
abbrev Rat' := Int × Nat

def ratCmp (a b : Rat') : Ordering := compare (a.1 * (b.2 : Int)) (b.1 * (a.2 : Int))

def intRat (n : Int) : Rat' := (n, 1)

def Decimal.toRat (d : Decimal) : Rat' :=
  let sgn : Int := if d.neg then -1 else 1
  if d.exp ≥ 0 then (sgn * ((d.coeff * 10 ^ d.exp.toNat : Nat) : Int), 1)
  else (sgn * (d.coeff : Int), 10 ^ (-d.exp).toNat)

/-- The exact value of a finite float. -/
def F64.toRat (f : F64) : Rat' :=
  let (m, e) := f.magnitude
  let sgn : Int := if f.signBit then -1 else 1
  if e ≥ 0 then (sgn * ((m * 2 ^ e.toNat : Nat) : Int), 1) else (sgn * (m : Int), 2 ^ (-e).toNat)

/-- The float nearest an integer — how a mixed Integer/Float pair is read. -/
def F64.ofInt (n : Int) : F64 := F64.ofRatio (n < 0) n.natAbs 1

/-- Floats by magnitude: NaN above everything and equal to itself. -/
def F64.cmp (a b : F64) : Ordering :=
  if a.isNaN && b.isNaN then .eq
  else if a.isNaN then .gt
  else if b.isNaN then .lt
  else
    let key (f : F64) : Option Rat' := if f.isInf then none else some f.toRat
    match key a, key b with
    | some x, some y => ratCmp x y
    | none, none => compare (if a.signBit then 0 else 1) (if b.signBit then 0 else 1)
    | none, some _ => if a.signBit then .lt else .gt
    | some _, none => if b.signBit then .gt else .lt

/-- Negative zero sorts just below positive zero. -/
def signedZeroOrder (a b : Value) : Ordering :=
  let negZero : Value → Bool
    | .float f => f.isZero && f.signBit
    | _ => false
  match negZero a, negZero b with
  | true, false => .lt
  | false, true => .gt
  | _, _ => .eq

/-- Numbers across leaves, by magnitude. `1` and `1.0` are EQUAL here: the
    one deliberate value-level equivalence in the order. -/
def compareNumbers (a b : Value) : Ordering :=
  let isBig : Value → Bool
    | .bigInt _ | .bigDec _ => true
    | _ => false
  let rat? : Value → Option Rat'
    | .int n => some (intRat n)
    | .bigInt n => some (intRat n)
    | .bigDec d => some d.toRat
    | .float f => if f.isNaN || f.isInf then none else some f.toRat
    | _ => none
  let nonFinite : Value → Option Ordering
    | .float f => if f.isNaN then some .gt else if f.isInf then some (if f.signBit then .lt else .gt) else none
    | _ => none
  if isBig a || isBig b then
    match nonFinite a, nonFinite b with
    | some o, _ => o
    | none, some o => o.swap
    | none, none =>
      match rat? a, rat? b with
      | some x, some y => (match ratCmp x y with | .eq => signedZeroOrder a b | o => o)
      | _, _ => .eq
  else match a, b with
    | .int x, .int y => compare x y
    | _, _ =>
      let asFloat : Value → F64
        | .int n => F64.ofInt n
        | .float f => f
        | _ => F64.zero false
      match F64.cmp (asFloat a) (asFloat b) with
      | .eq => signedZeroOrder a b
      | o => o

/-- The family that owns the comparison of two types, if any: the first
    of these on the walk up from their common ancestor. -/
def comparerFamily? (ta tb : TypeId) : Option TypeId :=
  match typeLca? ta tb with
  | none => none
  | some l =>
    (typeChain l).find? fun p =>
      p == "Scalar/Number" || p == "Scalar/String" || p == "Scalar/Boolean" || p == "Scalar/Atom"
        || p == "Scalar" || p == "Word"

/-- The size a container is first ordered by. -/
def sizeOf' : Value → Nat
  | .list elems _ => elems.length
  | .typedList _ elems => elems.length
  | .map entries _ => entries.length
  | .typedMap _ entries => entries.length
  | _ => 0

/-- Insertion sort of strings: map keys compare in sorted order. -/
def sortStrings (xs : List String) : List String :=
  xs.foldl (fun acc x =>
    let (lo, hi) := acc.span (· < x)
    lo ++ x :: hi) []

mutual

/-- The total order. `fuel` bounds the structural descent. -/
def compareValues : Nat → Value → Value → Ordering
  | 0, _, _ => .eq
  | fuel + 1, a, b =>
    let a := a.unquote
    let b := b.unquote
    let ta := if a.isConcrete then nodeFamily a.typeOf else a.typeOf
    let tb := if b.isConcrete then nodeFamily b.typeOf else b.typeOf
    -- a bare type before any concrete value of the family; two by rank
    let prologue : Option Ordering :=
      match a.isBareTypeNode, b.isBareTypeNode with
      | true, false => some .lt
      | false, true => some .gt
      | true, true => some (compareTypes a.typeOf b.typeOf)
      | false, false => none
    match comparerFamily? ta tb with
    | some "Scalar" =>
      -- two scalars of different families: by rank
      compareTypes a.typeOf b.typeOf
    | some fam =>
      match prologue with
      | some o => o
      | none =>
        if fam == "Scalar/Number" then compareNumbers a b
        else match a, b with
          | .str x, .str y => compare x y
          | .bool x, .bool y => compare (if x then 1 else 0) (if y then 1 else 0)
          | .atom x, .atom y => compare x y
          | _, _ => compare (canon a) (canon b)
    | none =>
      match compareTypes ta tb with
      | .eq =>
        match compare (sizeOf' a) (sizeOf' b) with
        | .eq =>
          match a, b with
          | .list ea _, .list eb _ => compareLists fuel ea eb
          | .map ea _, .map eb _ =>
            let ka := sortStrings (ea.map (·.1))
            let kb := sortStrings (eb.map (·.1))
            match compare ka kb with
            | .eq => compareByKeys fuel ka ea eb
            | o => o
          | _, _ => compare (canon a) (canon b)
        | o => o
      | o => o

def compareLists : Nat → List Value → List Value → Ordering
  | 0, _, _ => .eq
  | _ + 1, [], [] => .eq
  | _ + 1, [], _ => .lt
  | _ + 1, _, [] => .gt
  | fuel + 1, x :: xs, y :: ys =>
    match compareValues fuel x y with
    | .eq => compareLists fuel xs ys
    | o => o

def compareByKeys : Nat → List String → List (String × Value) → List (String × Value) → Ordering
  | 0, _, _, _ => .eq
  | _ + 1, [], _, _ => .eq
  | fuel + 1, k :: ks, ea, eb =>
    match compareValues fuel ((ea.lookup k).getD .none) ((eb.lookup k).getD .none) with
    | .eq => compareByKeys fuel ks ea eb
    | o => o

end

/-- `a` against `b` in the total order. -/
def cmp (a b : Value) : Ordering := compareValues (2 * (a.size + b.size) + 8) a b

/-! ## A disjunct's canonical alternatives

A disjunct is reduced (core/go/core_helpers.go `SimplifyDisjunctAlts`):
`Never` alternatives go; a repeat of an earlier alternative goes; an
alternative absorbed by a strictly wider bare type goes; what is left is
ordered by the total order, ties by canonical text. -/

/-- Two leaves are the same value: equal — and for two FUNCTIONS, the same
    function. A function is equal to itself and to nothing else, however
    alike two of them read (core/go/compare.go `sameFnIdentity`). -/
def sameValue : Value → Value → Bool
  | .fn _ _ i, .fn _ _ j => i == j
  | a, b => a == b

mutual
/-- Value equality as a disjunct's alternatives are deduplicated by
    (core/go/equal.go `ValuesEqual`): structural, and for maps
    independent of insertion order. -/
def valuesEqual : Nat → Value → Value → Bool
  | 0, _, _ => false
  | fuel + 1, a, b =>
    match a.unquote, b.unquote with
    | .list ea _, .list eb _ => listsEqual fuel ea eb
    | .map ea _, .map eb _ => ea.length == eb.length && entriesIn fuel ea eb
    | x, y => sameValue x y
def listsEqual : Nat → List Value → List Value → Bool
  | 0, _, _ => false
  | _ + 1, [], [] => true
  | fuel + 1, x :: xs, y :: ys => valuesEqual fuel x y && listsEqual fuel xs ys
  | _ + 1, _, _ => false
def entriesIn : Nat → List (String × Value) → List (String × Value) → Bool
  | 0, _, _ => false
  | _ + 1, [], _ => true
  | fuel + 1, (k, v) :: rest, other =>
    (match other.lookup k with
     | some w => valuesEqual fuel v w
     | none => false) && entriesIn fuel rest other
end

def simplifyDisjunctAlts (alts : List Value) : List Value :=
  let live := alts.filter (fun v => v.typeOf != TNever)
  let kept := (List.range live.length).filterMap fun i =>
    match live[i]? with
    | none => none
    | some cand =>
      let ct := cand.typeOf
      let repeated := (live.take i).any fun o =>
        o.typeOf == ct && valuesEqual (2 * (o.size + cand.size) + 8) o cand
      let absorbed := (List.range live.length).any fun j =>
        j != i && (match live[j]? with
          | some other =>
            let ot := other.typeOf
            ct != ot && conforms ct ot && other.isBareTypeNode
          | none => false)
      if repeated || absorbed then none else some cand
  -- a stable insertion sort under "less in the total order, ties by text"
  let less (x y : Value) : Bool :=
    match cmp x y with
    | .lt => true
    | .gt => false
    | .eq => canon x < canon y
  kept.foldl (fun acc x =>
    let (lo, hi) := acc.span (fun y => !less x y)
    lo ++ x :: hi) []

end Boru.Core
