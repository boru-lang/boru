/-
  Boru.Core.Theorems.UnifyOrder — unification and the total order, over
  a sample.

  These are finite checks over `sample` — one or more values of every
  shape the specification holds — discharged by `native_decide`. They
  state what holds, and, where the kernel's rule has an exception, they
  state the exception rather than hide it.
-/
import Boru.Core.Engine
import Boru.Core.Canon

namespace Boru.Core.UnifyOrder
open Boru.Core

def sample : List Value :=
  [ .int 0, .int 1, .int (-3), .str "", .str "a", .str "b", .bool true, .bool false, .none, .atom "x",
    .float (F64.ofInt 1), .bigInt 1,
    .typeLit "Any", .typeLit "None", .typeLit "Never", .typeLit "Absent",
    .typeLit TInteger, .typeLit "Scalar/Number", .typeLit "Scalar", .typeLit TString, .typeLit TBoolean,
    .typeLit TNode, .typeLit TList, .typeLit TMap, .typeLit "Word", .typeLit "Type",
    .list [] false, .list [.int 1] false, .list [.int 1, .int 2] false, .list [.typeLit TInteger] false,
    .list [.str "a"] false,
    .map [] false, .map [("a", .int 1)] false, .map [("a", .typeLit TInteger)] false,
    .map [("a", .int 1), ("b", .int 2)] false, .map [("b", .int 2), ("a", .int 1)] false,
    .typedList (.typeLit TInteger) [], .typedList (.typeLit TString) [], .typedList (.typeLit "Any") [],
    .typedMap (.typeLit TInteger) [], .typedMap (.typeLit TString) [],
    .disjunct [], .disjunct [.typeLit TInteger, .typeLit TString], .disjunct [.int 5, .int 6],
    .disjunct [.typeLit TMap, .typeLit TList], .disjunct [.map [("a", .int 1)] false] ]

/-! ## Unification -/

/-- Unification is SYMMETRIC as a predicate: which side a value is on
    never decides whether the two unify. -/
theorem unifies_symm :
    (sample.all fun a => sample.all fun b => unifies a b == unifies b a) = true := by
  native_decide

/-- The empty disjunct denotes the empty set: to the unifier it is
    `Never`. -/
def isNever (v : Value) : Bool := v == Value.typeLit "Never" || v == Value.disjunct []

/-- What a value's meet with itself is: the value — a disjunct in its
    reduced form, where no alternative is `Never` and one alternative is
    just that alternative. -/
def reduced : Value → Value
  | .disjunct [] => .typeLit "Never"
  | .disjunct [v] => v
  | v => v

/-- Unification is IDEMPOTENT: every value unifies with itself, and the
    meet is the value. -/
theorem unify_idem :
    (sample.all fun a => unify a a == some (reduced (resolveWordsDeep a))) = true := by
  native_decide

/-- `Any` unifies with everything but `Never` and `Absent`. -/
theorem any_unifies :
    (sample.all fun a =>
      unifies (.typeLit "Any") a == !(isNever a || a == Value.typeLit "Absent")) = true := by
  native_decide

/-- The degenerate roots unify only with themselves — `Never` with
    `Never`, `Absent` with `Absent` — and `None` also with `Any`. `Absent`
    is the one root `Any` does not admit: a slot typed `Any` still
    requires its value to be PRESENT. -/
theorem roots_unify_only_with_themselves :
    (sample.all fun a =>
      (unifies (.typeLit "Never") a == isNever a)
        && (unifies (.typeLit "Absent") a == (a == Value.typeLit "Absent"))
        && (unifies .none a == (a == Value.none || a == Value.typeLit "None" || a == Value.typeLit "Any"))
        && (unifies (.typeLit "None") a
              == (a == Value.none || a == Value.typeLit "None" || a == Value.typeLit "Any"))) = true := by
  native_decide

/-- THE EMPTY DISJUNCT IS `Never`: against every value it answers as
    `Never` does, on either side. -/
theorem empty_disjunct_is_never :
    (sample.all fun a =>
      unify (.disjunct []) a == unify (.typeLit "Never") a
        && unify a (.disjunct []) == unify a (.typeLit "Never")) = true := by
  native_decide

/-- A type literal unifies with a concrete scalar exactly when the
    scalar's type conforms to it. -/
theorem type_admits_conforming_scalars :
    ([Value.int 0, .int 1, .str "", .str "a", .bool true, .atom "x", .float (F64.ofInt 1), .bigInt 1].all fun v =>
     [TInteger, "Scalar/Number", "Scalar", TString, TBoolean, "Scalar/Atom", "Word", "Type", TNode].all fun t =>
      unifies (.typeLit t) v == conforms v.typeOf t) = true := by
  native_decide

/-- Plain maps unify CLOSED and regardless of key order: the same keys
    with unifying values, no key extra on either side. -/
theorem maps_unify_closed :
    (unifies (.map [("a", .int 1), ("b", .int 2)] false) (.map [("b", .int 2), ("a", .int 1)] false)
      && !unifies (.map [("a", .int 1)] false) (.map [("a", .int 1), ("b", .int 2)] false)
      && !unifies (.map [("a", .int 1), ("b", .int 2)] false) (.map [("a", .int 1)] false)) = true := by
  native_decide

/-- THE MEET OF TWO DISJUNCTS IS THE DISJUNCT OF THE PAIRWISE MEETS of
    their alternatives: a disjunct against itself is itself, the order
    the alternatives were written in decides nothing, one surviving meet
    is that value, and no pair unifying is no meet. -/
theorem disjunct_meet_is_pairwise :
    let meet (a b : List Value) := (unify (.disjunct a) (.disjunct b)).map canon
    (meet [.typeLit TInteger, .typeLit TString] [.typeLit TInteger, .typeLit TString] == some "Integer tor String"
      && meet [.typeLit TString, .typeLit TInteger] [.typeLit TInteger, .typeLit TString] == some "Integer tor String"
      && meet [.int 5, .int 6] [.int 5, .int 6] == some "5 tor 6"
      && meet [.int 5, .int 6] [.int 6, .int 7] == some "6"
      && meet [.typeLit TInteger, .typeLit TString] [.int 5, .int 6] == some "5 tor 6"
      && meet [.typeLit "Scalar/Number", .typeLit TString] [.typeLit TInteger, .typeLit "Scalar/Number"]
           == some "Number"
      && meet [.int 5, .int 6] [.int 7, .typeLit TString] == none) = true := by
  native_decide

/-- A TYPE AGAINST A DISJUNCT NARROWS EVERY ALTERNATIVE, not the first:
    `Integer` against `5 tor 6` is `5 tor 6`, `Scalar` against
    `Integer tor String` is the whole disjunct. A VALUE against a
    disjunct is that value. -/
theorem disjunct_meet_distributes :
    let meet (a : List Value) (b : Value) := (unify (.disjunct a) b).map canon
    (meet [.int 5, .int 6] (.typeLit TInteger) == some "5 tor 6"
      && (unify (.typeLit TInteger) (.disjunct [.int 5, .int 6])).map canon == some "5 tor 6"
      && meet [.typeLit TInteger, .typeLit TString] (.typeLit "Scalar") == some "Integer tor String"
      && meet [.typeLit TInteger, .typeLit TString] (.typeLit "Scalar/Number") == some "Integer"
      && meet [.list [.int 5] false, .list [.int 6] false] (.list [.typeLit TInteger] false) == some "[5] tor [6]"
      && meet [.typeLit TInteger, .typeLit TString] (.int 5) == some "5"
      && meet [.int 5, .int 6] (.int 6) == some "6"
      && meet [.int 5, .int 6] (.typeLit TString) == none) = true := by
  native_decide

/-- A map alternative is an open PATTERN. It admits a map VALUE carrying
    its keys and answers the value, extra keys and all; it narrows a map
    SHAPE carrying them, key by key; a map lacking a key it names is
    refused either way. -/
theorem map_alternative_against_value_and_shape :
    let m (es : List (String × Value)) : Value := .map es false
    let meet (a : List Value) (b : Value) := (unify (.disjunct a) b).map canon
    (meet [m [("a", .int 1)], m [("a", .int 2)]] (m [("a", .int 1), ("b", .int 2)]) == some "{a:1 b:2}"
      && meet [m [("a", .int 1)]] (m [("b", .int 2)]) == none
      && meet [m [("a", .int 1)], m [("a", .int 2)]] (m [("a", .typeLit TInteger)]) == some "{a:1} tor {a:2}"
      && meet [m [("a", .int 1)]] (m [("a", .typeLit TInteger), ("b", .typeLit TString)])
           == some "{a:1 b:String}"
      && meet [m [("a", .int 1)]] (m [("b", .typeLit TInteger)]) == none
      && meet [m [("a", .int 1)]] (m [("a", .typeLit TString)]) == none) = true := by
  native_decide

/-- Two map alternatives are two open PATTERNS: their meet is the pattern
    carrying the keys of either, whichever side each is on — where a
    plain map against a plain map unifies closed (`maps_unify_closed`). -/
theorem map_alternatives_meet_as_patterns :
    let m (es : List (String × Value)) : Value := .map es false
    let meet (a b : Value) := (unify (.disjunct [a]) (.disjunct [b])).map canon
    (meet (m [("a", .int 1)]) (m [("a", .int 1), ("b", .int 2)]) == some "{a:1 b:2}"
      && meet (m [("a", .int 1), ("b", .int 2)]) (m [("a", .int 1)]) == some "{a:1 b:2}"
      && meet (m [("a", .int 1)]) (m [("b", .int 2)]) == some "{a:1 b:2}"
      && meet (m [("a", .typeLit TInteger)]) (m [("a", .int 3)]) == some "{a:3}"
      && meet (m [("a", .int 1)]) (m [("a", .int 2)]) == none) = true := by
  native_decide

/-! ## `Type` -/

/-- EVERY TYPE IS A `Type`. `Type` is the one builtin node whose
    membership is not lattice ancestry: a type literal from any branch, a
    typed container, a disjunct and a function are all `Type`s, and no
    concrete value is. -/
theorem every_type_is_a_Type :
    (([Value.typeLit TInteger, .typeLit "Any", .typeLit "None", .typeLit TList, .typeLit "Word",
        .typedList (.typeLit TInteger) [], .typedMap (.typeLit TString) [],
        .disjunct [.typeLit TInteger, .typeLit TString], .fn "f" []].all (·.is TType))
      && ([Value.int 1, .str "a", .bool true, .none, .atom "x", .list [] false, .map [] false].all
            (fun v => !v.is TType))) = true := by
  native_decide

/-- …while ancestry alone decides every other slot: `Integer` the type
    literal sits on the Integer node, so it is an Integer by type — and
    is then DECLINED at a concrete-payload slot, because a type is not a
    value of itself. -/
theorem type_literal_is_not_its_own_value :
    ((Value.typeLit TInteger).is TInteger
      && rejectsTypeLiteral (.typeLit TInteger) TInteger
      && !rejectsTypeLiteral (.typeLit TInteger) TType
      && !rejectsTypeLiteral (.typeLit TInteger) TAny
      && !rejectsTypeLiteral (.int 1) TInteger) = true := by
  native_decide

/-! ## The total order -/

/-- The order is ANTISYMMETRIC in the comparison sense: swapping the
    operands swaps the answer. In particular it is total — every pair of
    values has an answer. -/
theorem cmp_swap :
    (sample.all fun a => sample.all fun b => cmp a b == (cmp b a).swap) = true := by
  native_decide

/-- The order is TRANSITIVE. -/
theorem cmp_trans :
    (sample.all fun a => sample.all fun b => sample.all fun c =>
      !(cmp a b == .lt && cmp b c == .lt) || cmp a c == .lt) = true := by
  native_decide

/-- Equality in the order is transitive too, so "equal in the order" is
    an equivalence — the classes are single values, plus the one
    deliberate identification below. -/
theorem cmp_eq_trans :
    (sample.all fun a => sample.all fun b => sample.all fun c =>
      !(cmp a b == .eq && cmp b c == .eq) || cmp a c == .eq) = true := by
  native_decide

/-- Numbers compare by MAGNITUDE across leaves: the Integer 1, the Float
    1.0 and the BigInteger 1 are equal in the order, though they are
    three different values with three different canonical forms. -/
theorem numbers_compare_by_magnitude :
    (cmp (.int 1) (.float (F64.ofInt 1)) == .eq
      && cmp (.int 1) (.bigInt 1) == .eq
      && cmp (.float (F64.ofInt 1)) (.bigInt 1) == .eq
      && canon (.int 1) != canon (.float (F64.ofInt 1))
      && canon (.int 1) != canon (.bigInt 1)) = true := by
  native_decide

/-- A bare type sorts before every concrete value of its family. -/
theorem type_before_its_values :
    (cmp (.typeLit TInteger) (.int (-3)) == .lt
      && cmp (.typeLit "Scalar/Number") (.int 0) == .lt
      && cmp (.typeLit TString) (.str "") == .lt
      && cmp (.typeLit TBoolean) (.bool false) == .lt
      && cmp (.typeLit "Scalar/Atom") (.atom "x") == .lt) = true := by
  native_decide

/-- Across families the lattice rank decides: every scalar before every
    node, whatever the values. -/
theorem scalars_before_nodes :
    ([Value.int 0, .str "b", .bool true, .atom "x"].all fun s =>
     [Value.list [] false, .list [.int 1] false, .map [] false].all fun n =>
      cmp s n == .lt) = true := by
  native_decide

end Boru.Core.UnifyOrder
