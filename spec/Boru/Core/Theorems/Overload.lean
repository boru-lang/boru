/-
  Boru.Core.Theorems.Overload — which definition a call takes.

  A name may be defined several times, and a call of it dispatches to ALL
  its definitions, tried in ONE order whatever order they were made in
  (core/go/signature.go `CompareSignatures`, here `Sig.order`):

    1. more arguments first;
    2. at the first position whose declared types differ, the type that
       sorts GREATER in the total order of values first — a type before
       every type above it, a branch root before `Any`;
    3. definitions the order holds equal: the newest first.

  The first one that matches is the call.

  The theorems run the machine over a word `f` defined once per overload —
  each definition a function of its own whose body answers a tag — and are
  discharged by `native_decide` over every order of definition and every
  operand of a family.
-/
import Boru.Core.Engine
import Boru.Core.Canon
import Boru.Corespec.Fixture

namespace Boru.Core.Overload
open Boru.Core

def w (name : String) : Value := .word { name }

/-- One definition of `f`: its parameter types, and the tag its body
    answers. -/
abbrev Overload := List TypeId × Int

def sigOf (o : Overload) : Sig :=
  .mk (o.1.zipIdx.map fun (t, i) => .mk s!"p{i}" t none false false) [TAny] o.1.length
    (.boru [.int o.2] [] false) false

/-- The fixture with `f` defined once per overload, in the order given:
    the last is the newest. -/
def defsOf (os : List Overload) : Defs :=
  os.zipIdx.foldl (fun d (o, i) => d.push "f" (.fn "f" [sigOf o] (9000 + i)))
    Boru.Corespec.fixtureDefs

def out (defs : Defs) (tape : List Value) : String :=
  match run Boru.Corespec.fixtureNatives defs tape with
  | .ok vs => canonValues vs
  | .error e => "ERROR:" ++ e.code

/-- Every order a list can be written in. -/
def orders : List α → List (List α)
  | [] => [[]]
  | x :: xs => (orders xs).flatMap fun p =>
      (List.range (p.length + 1)).map fun i => p.take i ++ x :: p.drop i

/-- A chain of four types, each above the next: one overload for each. -/
def chain : List Overload :=
  [([TAny], 4), (["Scalar"], 3), (["Scalar/Number"], 2), ([TInteger], 1)]

/-- An operand, and the tag of the NARROWEST type of the chain it is one
    of: an Integer; a number that is no Integer; a scalar that is no
    number; a value that is no scalar. -/
def chainCases : List (Value × Int) :=
  [(.int 5, 1), (.bigInt 7, 2), (.str "a", 3), (.bool true, 3), (.list [] false, 4), (.none, 4)]

/-- THE NARROWEST DEFINITION TAKES THE CALL, WHATEVER ORDER THEY WERE MADE
    IN. Over all twenty-four orders of the chain's four definitions, every
    operand is answered by the definition for the narrowest type it is one
    of. -/
theorem narrowest_wins_in_every_order :
    ((orders chain).all fun os => chainCases.all fun (v, tag) =>
      out (defsOf os) [w "f", v] == canon (.int tag)) = true := by
  native_decide

/-- The same in STACK form: where the operand is collected from does not
    change which definition takes it. -/
theorem narrowest_wins_from_the_stack :
    ((orders chain).all fun os => chainCases.all fun (v, tag) =>
      out (defsOf os) [v, w "f"] == canon (.int tag)) = true := by
  native_decide

/-- THE DISPATCH ORDER IS ONE LIST. Whatever order the chain is defined
    in, the function the name dispatches to lists the same signatures in
    the same order: narrowest first. -/
theorem dispatch_order_is_canonical :
    ((orders chain).all fun os =>
      (defsOf os).lookup? "f" == some (chain.reverse.map sigOf)) = true := by
  native_decide

/-- A BRANCH ROOT IS BEFORE `Any`, though both are one step from the top
    of their paths; so is `None`. -/
theorem a_root_is_narrower_than_any :
    ([["Scalar"], [TList], [TMap], [TNone]].all fun ts =>
      Sig.order (sigOf (ts, 1)) (sigOf ([TAny], 2)) == .lt &&
      Sig.order (sigOf ([TAny], 2)) (sigOf (ts, 1)) == .gt) = true := by
  native_decide

/-- THE FIRST POSITION THAT DIFFERS DECIDES — not how many positions are
    narrow. `(Integer Any)` is tried before `(Any Integer)`, and before
    `(Any Any)`; a call both could take goes to it, in either order of
    definition, and a call it declines falls to the other. -/
theorem first_differing_position_decides :
    ((orders [([TInteger, TAny], 1), ([TAny, TInteger], 2)]).all fun os =>
      out (defsOf os) [w "f", .int 5, .int 6] == "1" &&
      out (defsOf os) [w "f", .str "a", .int 6] == "2" &&
      out (defsOf os) [w "f", .str "a", .str "b"] == "ERROR:signature_error") = true := by
  native_decide

/-- A later position decides only where the earlier ones are alike. -/
theorem a_later_position_breaks_the_tie :
    ((orders [([TAny, TInteger], 1), ([TAny, TAny], 2)]).all fun os =>
      out (defsOf os) [w "f", .int 5, .int 6] == "1" &&
      out (defsOf os) [w "f", .int 5, .str "b"] == "2") = true := by
  native_decide

/-- ARITY BEFORE TYPE. The two-argument definition is tried first however
    narrow the one-argument definition is: with two operands written it
    takes the call, and at a boundary — where only one can be collected —
    the call is the shorter one's. -/
theorem more_arguments_first :
    ((orders [([TAny, TAny], 2), ([TInteger], 1)]).all fun os =>
      out (defsOf os) [w "f", .int 5, .int 6] == "2" &&
      out (defsOf os) [w "f", .int 5, .endMark, .int 6] == "1 6") = true := by
  native_decide

/-- EQUAL IN THE ORDER: THE NEWEST DEFINITION FIRST. Two definitions with
    the same parameter types are both the function; the later one answers,
    and unbinding it uncovers the earlier. -/
theorem newest_of_equals_first :
    ([[TInteger], [TAny], [TAny, TInteger]].all fun ts =>
      let args := ts.map fun _ => Value.int 5
      out (defsOf [(ts, 1), (ts, 2)]) (w "f" :: args) == "2" &&
      out (defsOf [(ts, 2), (ts, 1)]) (w "f" :: args) == "1" &&
      out ((defsOf [(ts, 1), (ts, 2)]).pop "f") (w "f" :: args) == "1") = true := by
  native_decide

/-- THE ORDER IS ANTISYMMETRIC over the signatures of a family: of two
    different signatures one is before the other, and which does not
    depend on the side it is asked from. -/
theorem order_is_antisymmetric :
    (let types := [TAny, "Scalar", "Scalar/Number", TInteger, TString, TList, TNone]
     let sigs := (types.map fun t => sigOf ([t], 0)) ++
       (types.flatMap fun a => types.map fun b => sigOf ([a, b], 0))
     sigs.all fun a => sigs.all fun b =>
       Sig.order a b == (Sig.order b a).swap && ((Sig.order a b == .eq) == (a == b))) = true := by
  native_decide

end Boru.Core.Overload
