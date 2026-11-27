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

/-- Every value unifies with itself — except the EMPTY disjunct, which
    has no alternative to match and so unifies with nothing, itself
    included. -/
theorem unifies_refl_except_empty_disjunct :
    (sample.all fun a => unifies a a == !(a == Value.disjunct [])) = true := by
  native_decide

/-- `Any` unifies with everything but `Never` and `Absent`. (The empty
    disjunct, which unifies with nothing else, does unify with `Any`:
    `Any` keeps the disjunct whole.) -/
theorem any_unifies :
    (sample.all fun a =>
      unifies (.typeLit "Any") a == !(a == Value.typeLit "Never" || a == Value.typeLit "Absent")) = true := by
  native_decide

/-- The degenerate roots unify only with themselves — `Never` with
    `Never`, `Absent` with `Absent` — and `None` also with `Any`. `Absent`
    is the one root `Any` does not admit: a slot typed `Any` still
    requires its value to be PRESENT. -/
theorem roots_unify_only_with_themselves :
    (sample.all fun a =>
      (unifies (.typeLit "Never") a == (a == Value.typeLit "Never"))
        && (unifies (.typeLit "Absent") a == (a == Value.typeLit "Absent"))
        && (unifies .none a == (a == Value.none || a == Value.typeLit "None" || a == Value.typeLit "Any"))
        && (unifies (.typeLit "None") a
              == (a == Value.none || a == Value.typeLit "None" || a == Value.typeLit "Any"))) = true := by
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

/-- THE MEET OF TWO DISJUNCTS IS NOT THEIR MEET. Unifying a disjunct with
    a disjunct returns the first alternative that matches — `Integer`
    for `Integer tor String` against itself — where the meet is the
    disjunct. The predicate is unaffected; only the value is. Pinned here
    because it is what the kernel does (NUR386), not because it is right. -/
theorem disjunct_meet_is_first_alternative :
    ((unify (.disjunct [.typeLit TInteger, .typeLit TString])
            (.disjunct [.typeLit TInteger, .typeLit TString])).map canon == some "Integer") = true := by
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
