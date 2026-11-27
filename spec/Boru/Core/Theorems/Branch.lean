/-
  Boru.Core.Theorems.Branch — the two moves that are no loops.

  A move ends a region that a mark began. Two of the four things a move
  does are loops (`Theorems/Counted.lean`); the other two are here.

  THE `if` CONTINUATION. The region is a condition. Its LAST value — by
  the one truthiness rule — picks one of two branches, and the branch
  takes the region's place: the condition's values are gone, and the
  branch is stepped where the `if` stood. A condition that leaves no
  value is an error. It is not a loop: a flow signal passes it.

  THE PLAIN MOVE. The mark carries a body; the body runs between the two,
  and the move puts the mark's body in the place of everything that run
  left — to run once more.

  The theorems run the machine over the corpus fixture —
  `ifq cond then else`, whose branches are paren groups as basic's `if`
  lays them out, and `replayq body` — and are discharged by
  `native_decide` over the stated domains.
-/
import Boru.Core.Theorems.Counted

namespace Boru.Core.Branch
open Boru.Core
open Boru.Core.Counted (w ws out outWith ints forq trues falses stackOf)

def ifq (test thenB elseB : List Value) : List Value :=
  [w "ifq", .list test false, .list thenB false, .list elseB false]

def replayq (body : List Value) : List Value := [w "replayq", .list body false]

/-- THE CONDITION PICKS ONE BRANCH, by the one truthiness rule: every true
    value takes the first, every false value the second. -/
theorem takes_one_branch :
    ((trues.all fun t => out (ifq [t] [.int 1] [.int 2]) == "1") &&
      (falses.all fun f => out (ifq [f] [.int 1] [.int 2]) == "2")) = true := by
  native_decide

/-- THE CONDITION'S LAST VALUE DECIDES, and the condition leaves nothing:
    whatever it left before its last value is gone with it. -/
theorem last_value_decides :
    (trues.all fun t => falses.all fun f =>
      out (ifq [f, t] [.int 1] [.int 2]) == "1" &&
      out (ifq [t, f] [.int 1] [.int 2]) == "2" &&
      out (ifq [.int 9, .int 8, t] [.int 1] [.int 2]) == "1") = true := by
  native_decide

/-- A CONDITION THAT LEAVES NO VALUE IS AN ERROR — and a branch that
    leaves none is not: an `if` may come to nothing. -/
theorem no_condition_value_is_an_error :
    (out (ifq [] [.int 1] [.int 2]) == "ERROR:runtime_error" &&
      out (ifq [.bool true] [] [.int 2]) == "" &&
      out (ifq [.bool false] [.int 1] []) == "") = true := by
  native_decide

/-- ONLY THE CHOSEN BRANCH RUNS: a branch that would fail is harmless when
    the condition picks the other. -/
theorem only_the_chosen_branch_runs :
    (out (ifq [.bool false] [w "boomq", .int 1] [.int 9]) == "9" &&
      out (ifq [.bool true] [.int 9] [w "boomq", .int 1]) == "9" &&
      out (ifq [.bool true] [w "boomq", .int 1] [.int 9]) == "ERROR:fixture_boom") = true := by
  native_decide

/-- THE BRANCH IS STEPPED WHERE THE `if` STOOD: its values are the `if`'s,
    and a word written after takes them as it takes any values. -/
theorem the_branch_stands_where_the_if_stood :
    ([3, -4, 0].all fun (n : Int) =>
      out (ifq [.bool true] [.int n] [.int 9] ++ [w "negq"]) == ints [-n] &&
      out ([.int 7] ++ ifq [.bool false] [.int 9] [.int n, .int n] ++ [.int 5]) == ints [7, n, n, 5]) = true := by
  native_decide

/-- `if`s NEST, in a condition and in a branch. -/
theorem ifs_nest :
    ([true, false].all fun a => [true, false].all fun b =>
      -- an `if` as the condition of another
      out (ifq (ifq [.bool a] [.bool b] [.bool (!b)]) [.int 1] [.int 2])
        == ints [if (if a then b else !b) then 1 else 2] &&
      -- an `if` as a branch
      out (ifq [.bool a] (ifq [.bool b] [.int 1] [.int 2]) [.int 3])
        == ints [if a then (if b then 1 else 2) else 3]) = true := by
  native_decide

/-- AN `if` IS NO LOOP. A signal raised in its branch, or in its
    condition, is taken by the loop around it: here the loop keeps the
    rounds before the index is first true, and breaks. -/
theorem an_if_is_no_loop :
    ([1, 2, 3, 4].all fun (n : Int) =>
      out (forq n (ifq [w "i"] [w "breakq"] [w "i"])) == "0" &&
      out (forq n (ifq [w "i", w "breakq"] [.int 1] [.int 2])) == "" &&
      -- and with no loop around it, the signal is an error
      out (ifq [.bool true] [w "breakq"] [.int 2]) == "ERROR:flow_error") = true := by
  native_decide

/-- THE PLAIN MOVE RUNS ITS BODY AGAIN, in the place of everything the
    first run left: a body of values leaves them once. -/
theorem a_replay_leaves_its_body_once :
    ([[], [Value.int 1], [.int 1, .str "a"], [.bool true, .none, .int 0]].all fun vs =>
      out (replayq vs) == canonValues vs) = true := by
  native_decide

/-- …and it did run twice: a body that reads a binding and removes it
    answers with the SECOND one it read. -/
theorem a_replay_runs_twice :
    ([Value.int 1, .str "a", .bool false].all fun first => [Value.int 2, .none].all fun second =>
      outWith (stackOf [second, first]) (replayq [w "c", w "undef", w "c"]) == canon second) = true := by
  native_decide

/-- A BRANCH IS A GROUP, a replayed body is not: a function a branch
    leaves alone is placed, as a group's lone function is; a function a
    replay leaves is called on what follows. -/
theorem a_branch_is_a_group :
    ([5, -2].all fun (n : Int) =>
      -- the fixture's `negq`, as a function value standing in a body
      let f : Value := (Boru.Corespec.fixtureDefs.top? "negq").getD .none
      out (replayq [f] ++ [.int n]) == ints [-n] &&
      out (ifq [.bool true] [f] [.int 9] ++ [.int n]) == canonValues [f, .int n]) = true := by
  native_decide

end Boru.Core.Branch
