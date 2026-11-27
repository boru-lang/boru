/-
  Boru.Core.Theorems.Loop — a loop's body runs in a region of its own.

  A looping word hands the engine a driver, and the engine runs one
  sealed region per iteration: the inputs the driver names, then the body.
  Three things hold of every such region, whatever the loop:

    * the body's stack is its inputs — nothing beneath the loop's word is
      in its sight;
    * an input is data: it is never stepped, whatever it is;
    * what the region leaves is the driver's — it is never stepped again.

  The theorems run the machine over the corpus fixture's `eachq xs body`:
  one iteration per element, the element the region's one input, and
  everything a region leaves collected, in order, into the list the loop
  answers. Each quantifies over every choice of operands and is
  discharged by `native_decide`.
-/
import Boru.Core.Engine
import Boru.Core.Canon
import Boru.Corespec.Fixture

namespace Boru.Core.Loop
open Boru.Core

def w (name : String) : Value := .word { name }

def out (tape : List Value) : String :=
  match run Boru.Corespec.fixtureNatives Boru.Corespec.fixtureDefs tape with
  | .ok vs => canonValues vs
  | .error e => "ERROR:" ++ e.code

/-- `eachq xs body`, written forward. -/
def each (xs body : List Value) : List Value :=
  [w "eachq", .list xs false, .list body false]

def operands : List Value := [.int 1, .int 2, .str "a", .bool true, .none]

/-- Every list of up to two operands. -/
def lists : List (List Value) :=
  [[]] ++ operands.map (fun a => [a]) ++ operands.flatMap fun a => operands.map fun b => [a, b]

/-- AN EMPTY BODY LEAVES ITS INPUT: the loop answers the list it was given.
    A region's residual is everything in it — an input the body did not
    take included. -/
theorem empty_body_is_identity :
    (lists.all fun xs => out (each xs []) == canon (.list xs false)) = true := by
  native_decide

/-- NO ELEMENT, NO ITERATION: over an empty list the body never runs —
    not even one that would raise. -/
theorem empty_loop_runs_nothing :
    ([[w "boomq"], [w "nosuch"], [.closeParen], [w "negq"]].all fun body =>
      out (each [] body) == "[]") = true := by
  native_decide

/-- THE BODY'S STACK IS ITS INPUT. `depthq` counts what its scope holds:
    in a loop body that is the one element, whatever stands beneath the
    loop's word — and those values are there, untouched, afterwards. -/
theorem body_sees_only_its_input :
    (lists.all fun below => operands.all fun x =>
      out (below ++ each [x] [w "depthq"])
        == canonValues (below ++ [.list [x, .int 1] false])) = true := by
  native_decide

/-- AN INPUT IS DATA. A function, a word naming one, a bare word naming
    nothing: as a region's input each is carried, not stepped — the body
    `5` is left beside it. -/
theorem inputs_are_inert :
    (([w "negq", .word { name := "negq", forceVal := true }, w "nosuch", w "boomq"] : List Value).all
      fun x => out (each [x] [.int 5]) == canon (.list [x, .int 5] false)) = true := by
  native_decide

/-- WHAT A REGION LEAVES IS NOT STEPPED AGAIN. A body that leaves a
    FUNCTION leaves a value: it is collected, and neither it nor the list
    the loop answers is called with what follows the loop. -/
theorem residual_is_not_restepped :
    (operands.all fun x => operands.all fun after =>
      (match run Boru.Corespec.fixtureNatives Boru.Corespec.fixtureDefs
          (each [x] [.word { name := "negq", forceVal := true }] ++ [after]) with
       | .ok [.list [x', .fn _ _ _] _, after'] => x' == x && after' == after
       | _ => false)) = true := by
  native_decide

/-- THE BODY RUNS ONCE PER ELEMENT, IN ORDER, each on its own element:
    `dupq` leaves each element twice, and the loop's list holds them in
    the order the elements stood. -/
theorem one_iteration_per_element :
    (lists.all fun xs =>
      out (each xs [w "dupq"]) == canon (.list (xs.flatMap fun x => [x, x]) false)) = true := by
  native_decide

/-- A BODY'S ERROR IS THE LOOP'S: with an element to run on, a body that
    raises ends the loop with that error. -/
theorem body_error_ends_the_loop :
    (operands.all fun x =>
      out (each [x] [w "boomq"]) == "ERROR:fixture_boom"
        && out (each [x] [w "nosuch"]) == "ERROR:undefined_word") = true := by
  native_decide

/-- LOOPS NEST: an inner loop is a call like any other in the outer body,
    and its region is sealed again — its body sees one element, not the
    outer loop's. -/
theorem loops_nest :
    (operands.all fun x => operands.all fun y =>
      out (each [x] (each [y] [w "depthq"]))
        == canon (.list [x, .list [y, .int 1] false] false)) = true := by
  native_decide

/-! ## The call region

A native whose last act is running a body returns the run as a CALL REGION
(core/go `CallRegion`): a driven loop of ONE round. Its inputs are sealed
and inert like any region's — and its residual is not a driver's to keep
but the word's own results, stepped again where the word stood. The
fixture's `callq x body` is one, with `x` its one input. -/

/-- `callq x body`, written forward. -/
def call (x : Value) (body : List Value) : List Value := [w "callq", x, .list body false]

/-- THE BODY'S STACK IS ITS INPUT, as in any region: values beneath the
    word are out of its sight, and stand untouched. -/
theorem a_call_region_is_sealed :
    (operands.all fun below => operands.all fun x =>
      out ([below] ++ call x [w "depthq"]) == canonValues [below, x, .int 1]) = true := by
  native_decide

/-- THE RESIDUAL IS THE WORD'S RESULTS: everything the round left — the
    input the body did not take, then what the body left — stands where
    the word stood, and a word written after takes it. -/
theorem a_call_regions_residual_is_its_results :
    (operands.all fun x => operands.all fun v =>
      out (call x [v]) == canonValues [x, v] &&
      out (call (.int 5) [.int 6] ++ [w "sumq"]) == "11") = true := by
  native_decide

/-- …AND IT IS STEPPED AGAIN. A function the body leaves is a value in
    the region and is CALLED when the region's residual is stepped where
    the word stood — where the driven loop beside it hands the same
    residual to its driver, and nothing steps it. -/
theorem a_call_regions_residual_is_stepped :
    ([5, -2, 0].all fun (n : Int) =>
      let ref : Value := .word { name := "negq", forceVal := true }
      out (call (.int n) [ref]) == canonValues [.int (-n)] &&
      out (each [.int n] [ref]) != canonValues [.int (-n)]) = true := by
  native_decide

end Boru.Core.Loop
