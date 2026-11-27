/-
  Boru.Core.Unify — unification: the meet of two values.

  Since a type is a value (ADR-010), "does this value have this type",
  "do these two shapes agree" and "is this the same value" are ONE
  question: do the two values unify, and to what. The kernel's answer is
  core/go/unify.go `unifyInner`; this is that function for the value
  shapes the specification holds.

  The result is the more specific of the two — a concrete value over the
  type that admits it, a narrower type over a wider one — or nothing when
  they are incompatible.

  The order of the arms is the rule: an earlier arm rules the pair before
  a later one sees it.
-/
import Boru.Core.Compare

namespace Boru.Core

/-- Word resolution ahead of unification (core/go/resolve.go
    `ResolveWordsDeep`): a bare word inside a shape is the keyword, type
    name or atom it spells. Only plain lists and maps, typed containers
    and disjunct alternatives are entered. -/
def resolveWordValue : Value → Value
  | .word w =>
    if w.name == "true" then .bool true
    else if w.name == "false" then .bool false
    else match resolveTypeName? w.name with
      | some t => .typeLit t
      | none => .atom w.name
  | v => v

mutual
def resolveWordsDeep : Value → Value
  | .word w => resolveWordValue (.word w)
  | .list elems _ => .list (resolveWordsList elems) false
  | .map entries _ => .map (resolveWordsEntries entries) false
  | .typedList child elems => .typedList (resolveWordsDeep child) (resolveWordsList elems)
  | .typedMap child entries => .typedMap (resolveWordsDeep child) (resolveWordsEntries entries)
  | .disjunct alts =>
    -- a disjunct is a SET: resolved, it is reduced to its canonical
    -- alternatives — repeats and absorbed alternatives dropped, the rest
    -- in the total order
    .disjunct (simplifyDisjunctAlts (resolveWordsList alts))
  | .quoted v => resolveWordsDeep v
  | v => v
def resolveWordsList : List Value → List Value
  | [] => []
  | v :: vs => resolveWordsDeep v :: resolveWordsList vs
def resolveWordsEntries : List (String × Value) → List (String × Value)
  | [] => []
  | (k, v) :: es => (k, resolveWordsDeep v) :: resolveWordsEntries es
end

namespace Value

/-- The degenerate roots and the top, as the unifier sees them. -/
def isNoneShape : Value → Bool
  | .none => true
  | .typeLit t => t == TNone
  | _ => false

def isNeverShape : Value → Bool | .typeLit t => t == TNever | _ => false
def isAbsentShape : Value → Bool | .typeLit t => t == TAbsent | _ => false
def isAnyShape : Value → Bool | .typeLit t => t == TAny | _ => false
def isDisjunct : Value → Bool | .disjunct _ => true | _ => false

/-- A plain (untyped) map with a payload. -/
def isPlainMap : Value → Bool | .map _ _ => true | _ => false

end Value

/-- `Any` and the degenerate roots fold a pair before any structural arm:
    a root unifies only with itself (None also with `Any`). -/
private def foldDegen (isRoot : Value → Bool) (ruling other : Value) : Option Value :=
  if !isRoot other then none
  else if other.isConcrete && !ruling.isConcrete then some other
  else some ruling

/-- Same type: equal values. Otherwise the narrower of two types, or the
    concrete value a type literal admits. -/
def unifySameOrSubtype (a b : Value) : Option Value :=
  let ta := a.typeOf
  let tb := b.typeOf
  if a.isBareTypeNode && b.isConcrete && conforms tb ta then some b
  else if b.isBareTypeNode && a.isConcrete && conforms ta tb then some a
  else if ta == tb then (if a == b then some a else none)
  else if isSubtype ta tb then some a
  else if isSubtype tb ta then some b
  else none

mutual

/-- The meet of two (word-resolved) values. `fuel` bounds the descent; any
    fuel above the combined size of the two values is enough. -/
def unifyInner : Nat → Value → Value → Option Value
  | 0, _, _ => none
  | fuel + 1, a, b =>
    -- a disjunct rules first, whichever side it is on
    match a, b with
    | .disjunct alts, _ => unifyDisjunct fuel alts b
    | _, .disjunct alts => unifyDisjunct fuel alts a
    | _, _ =>
      if a.isNeverShape then foldDegen Value.isNeverShape a b
      else if b.isNeverShape then foldDegen Value.isNeverShape b a
      else if a.isNoneShape then (if b.isAnyShape then some a else foldDegen Value.isNoneShape a b)
      else if b.isNoneShape then (if a.isAnyShape then some b else foldDegen Value.isNoneShape b a)
      else if a.isAbsentShape then foldDegen Value.isAbsentShape a b
      else if b.isAbsentShape then foldDegen Value.isAbsentShape b a
      else if a.isAnyShape then some b
      else if b.isAnyShape then some a
      else
        let aNode := a == .typeLit TNode
        let bNode := b == .typeLit TNode
        if aNode && bNode then some a
        else if aNode || bNode then
          let other := if bNode then a else b
          if other.isMapShape || other.isListShape then some other
          else match other with
            | .typeLit t => if conforms t TNode then some other else none
            | _ => none
        else
          let aListLit := a == .typeLit TList
          let bListLit := b == .typeLit TList
          let aMapLit := a == .typeLit TMap
          let bMapLit := b == .typeLit TMap
          if a.isListShape || b.isListShape || aListLit || bListLit then
            if aListLit then (if b.isListShape || bListLit then some b else none)
            else if bListLit then (if a.isListShape then some a else none)
            else match a, b with
              | .typedList ca _, .typedList cb _ =>
                (unifyInner fuel ca cb).map (fun c => .typedList c [])
              | .typedList c _, .list elems _ => (unifyAllWith fuel c elems).map (.list · false)
              | .list elems _, .typedList c _ => (unifyAllWith fuel c elems).map (.list · false)
              | .list ea _, .list eb _ => (unifyZip fuel ea eb).map (.list · false)
              | _, _ => none
          else if a.isMapShape || b.isMapShape || aMapLit || bMapLit then
            if aMapLit then (if b.isMapShape || bMapLit then some b else none)
            else if bMapLit then (if a.isMapShape then some a else none)
            else match a, b with
              | .typedMap ca _, .typedMap cb _ =>
                (unifyInner fuel ca cb).map (fun c => .typedMap c [])
              | .typedMap c _, .map entries _ => (unifyEntriesWith fuel c entries).map (.map · false)
              | .map entries _, .typedMap c _ => (unifyEntriesWith fuel c entries).map (.map · false)
              | .map ea _, .map eb _ => unifyMaps fuel ea eb
              | _, _ => none
          else unifySameOrSubtype a b

/-- A disjunct against a value: `Any` leaves the disjunct; otherwise the
    first alternative that unifies wins. A plain-map alternative against a
    plain map unifies OPENLY — the value may carry more keys. -/
def unifyDisjunct : Nat → List Value → Value → Option Value
  | 0, _, _ => none
  | fuel + 1, alts, val =>
    if val.isAnyShape then some (.disjunct alts)
    else unifyAlts fuel alts val

def unifyAlts : Nat → List Value → Value → Option Value
  | 0, _, _ => none
  | _ + 1, [], _ => none
  | fuel + 1, alt :: rest, val =>
    match alt, val with
    | .map pe _, .map ce _ =>
      if openUnifyEntries fuel pe ce then some val else unifyAlts fuel rest val
    | _, _ =>
      match unifyInner fuel alt val with
      | some u => some u
      | none => unifyAlts fuel rest val

/-- Open map unification: every key the pattern names must unify, a key
    the candidate lacks standing as `Absent`; extra candidate keys pass. -/
def openUnifyEntries : Nat → List (String × Value) → List (String × Value) → Bool
  | 0, _, _ => false
  | _ + 1, [], _ => true
  | fuel + 1, (k, pv) :: rest, cand =>
    let cv := (cand.lookup k).getD (.typeLit TAbsent)
    (unifyInner fuel (resolveWordsDeep pv) (resolveWordsDeep cv)).isSome
      && openUnifyEntries fuel rest cand

/-- Element-wise unification of two lists of equal length. -/
def unifyZip : Nat → List Value → List Value → Option (List Value)
  | 0, _, _ => none
  | _ + 1, [], [] => some []
  | fuel + 1, x :: xs, y :: ys =>
    match unifyInner fuel x y, unifyZip fuel xs ys with
    | some u, some us => some (u :: us)
    | _, _ => none
  | _ + 1, _, _ => none

/-- Every element against one child constraint. -/
def unifyAllWith : Nat → Value → List Value → Option (List Value)
  | 0, _, _ => none
  | _ + 1, _, [] => some []
  | fuel + 1, c, x :: xs =>
    match unifyInner fuel c x, unifyAllWith fuel c xs with
    | some u, some us => some (u :: us)
    | _, _ => none

def unifyEntriesWith : Nat → Value → List (String × Value) → Option (List (String × Value))
  | 0, _, _ => none
  | _ + 1, _, [] => some []
  | fuel + 1, c, (k, x) :: xs =>
    match unifyInner fuel c x, unifyEntriesWith fuel c xs with
    | some u, some us => some ((k, u) :: us)
    | _, _ => none

/-- Two plain maps: CLOSED. Every key of either must unify with the
    other's value for it, a missing key standing as `Absent`; a key whose
    meet is `Absent` is dropped. Key order follows the left map, then the
    right map's extra keys. -/
def unifyMaps : Nat → List (String × Value) → List (String × Value) → Option Value
  | 0, _, _ => none
  | fuel + 1, ea, eb =>
    match unifySide fuel ea eb, unifySide fuel (eb.filter (fun (k, _) => (ea.lookup k).isNone)) [] with
    | some l, some r => some (.map (l ++ r) false)
    | _, _ => none

def unifySide : Nat → List (String × Value) → List (String × Value) → Option (List (String × Value))
  | 0, _, _ => none
  | _ + 1, [], _ => some []
  | fuel + 1, (k, av) :: rest, other =>
    let bv := (other.lookup k).getD (.typeLit TAbsent)
    match unifyInner fuel av bv, unifySide fuel rest other with
    | some u, some us => if u.isAbsentShape then some us else some ((k, u) :: us)
    | _, _ => none

end

/-- Unification as the kernel's clients call it (`UnifyR`): resolve the
    words inside both shapes, then take the meet. -/
def unify (a b : Value) : Option Value :=
  let a := resolveWordsDeep a
  let b := resolveWordsDeep b
  unifyInner (2 * (a.size + b.size) + 8) a b

/-- The predicate half: do the two values unify at all. -/
def unifies (a b : Value) : Bool := (unify a b).isSome

end Boru.Core
