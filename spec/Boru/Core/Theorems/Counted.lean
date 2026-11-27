/-
  Boru.Core.Theorems.Counted — a loop that counts, and a loop that asks.

  The counted and the conditional loop are ONE mechanism: a mark, a body,
  and a move that takes what the body left and replays it — with no paren
  around a round, so the body runs on the stack the loop's word stood on.
  (A driven loop's round is a sealed region; `Theorems/Loop.lean`.)

    * a counted loop runs its body once for each index of its range, the
      index bound for the round, and its values are every round's, in order;
    * the index is the loop's own: an outer binding of its name is there
      again when the loop ends;
    * a round reads the caller's stack, and what it takes is gone for the
      next round;
    * the loop's values are STEPPED where the loop stood;
    * a conditional loop runs its body while its condition's LAST value is
      true — by the one truthiness rule, presence and not content — and a
      condition that leaves no value is an error.

  The theorems run the machine over the corpus fixture — `forq n body`,
  `rangeq start end step body`, `whileq cond body` — and are discharged by
  `native_decide` over the stated domains.
-/
import Boru.Core.Engine
import Boru.Core.Canon
import Boru.Corespec.Fixture

namespace Boru.Core.Counted
open Boru.Core

def w (name : String) : Value := .word { name }
def ws (name : String) : Value := .word { name, forceStack := true }

def outWith (defs : Defs) (tape : List Value) : String :=
  match run Boru.Corespec.fixtureNatives defs tape with
  | .ok vs => canonValues vs
  | .error e => "ERROR:" ++ e.code

def out (tape : List Value) : String := outWith Boru.Corespec.fixtureDefs tape

def ints (ns : List Int) : String := canonValues (ns.map Value.int)

def forq (n : Int) (body : List Value) : List Value := [w "forq", .int n, .list body false]

def rangeq (start stop step : Int) (body : List Value) : List Value :=
  [w "rangeq", .int start, .int stop, .int step, .list body false]

/-- The indices a range visits: from `start`, by `step`, while short of the
    exclusive bound `stop`. `fuel` bounds the walk. -/
def indices (start stop step : Int) : Nat → List Int
  | 0 => []
  | fuel + 1 =>
    if (step > 0 && start < stop) || (step < 0 && start > stop) then
      start :: indices (start + step) stop step fuel
    else []

/-- A COUNTED LOOP RUNS ITS BODY ONCE FOR EACH INDEX, the index bound as
    it runs: `forq n [i]` is 0 … n-1, and nothing for a count of zero or
    less. -/
theorem counts_each_index :
    ([-2, -1, 0, 1, 2, 3, 4, 5, 6].all fun (n : Int) =>
      out (forq n [w "i"]) == ints (indices 0 n 1 20)) = true := by
  native_decide

/-- ANY RANGE: from the start, by the step, short of the bound — up or
    down, and nothing when the step leads away from the bound. -/
theorem counts_any_range :
    ([-1, 0, 2, 5].all fun (start : Int) => [-3, 0, 3, 6].all fun (stop : Int) =>
      [-2, -1, 1, 2, 3].all fun (step : Int) =>
        out (rangeq start stop step [w "i"]) == ints (indices start stop step 20)) = true := by
  native_decide

/-- EVERY ROUND'S VALUES, IN ORDER. A body that leaves two values leaves
    them every round. -/
theorem rounds_values_in_order :
    ([0, 1, 2, 3, 4].all fun (n : Int) => [Value.int 7, .str "a", .bool true, .none].all fun v =>
      out (forq n [w "i", v]) ==
        canonValues ((indices 0 n 1 20).flatMap fun i => [.int i, v])) = true := by
  native_decide

/-- THE INDEX IS THE LOOP'S OWN. Whatever `i` was bound to before the
    loop, it is bound to that again after it; and with no outer binding
    the name is unbound again. -/
theorem index_is_scoped :
    ([0, 1, 2, 3].all fun (n : Int) => [Value.int 50, .str "a", .bool false].all fun v =>
      outWith (Boru.Corespec.fixtureDefs.push "i" v) (forq n [w "i"] ++ [w "i"]) ==
        canonValues ((indices 0 n 1 20).map Value.int ++ [v]) &&
      out (forq n [] ++ [w "i"]) == "ERROR:undefined_word") = true := by
  native_decide

/-- LOOPS NEST, each with its own index: the inner one shadows the outer
    for as long as it runs, and the outer is there again after it. -/
theorem inner_index_shadows :
    ([0, 1, 2, 3].all fun (n : Int) => [0, 1, 2].all fun (m : Int) =>
      out (forq n ([w "i"] ++ forq m [w "i"] ++ [w "i"])) ==
        ints ((indices 0 n 1 20).flatMap fun i => [i] ++ indices 0 m 1 20 ++ [i])) = true := by
  native_decide

/-- A ROUND RUNS ON THE CALLER'S STACK. There is no paren around a round:
    a stack word in the body takes what stands beneath the loop — and what
    one round takes, the next does not find. Over a stack of three, a loop
    of n rounds negates the top n, nearest first, and leaves the rest. -/
theorem rounds_share_the_callers_stack :
    ([0, 1, 2, 3].all fun (n : Nat) =>
      out ([Value.int 5, .int 6, .int 7] ++ forq n [ws "negq"]) ==
        ints (([5, 6, 7] : List Int).take (3 - n) ++ (([7, 6, 5] : List Int).take n).map (-·))) = true := by
  native_decide

/-- …and a fourth round, finding nothing left, fails as the word would. -/
theorem a_round_too_many_finds_nothing :
    (out ([Value.int 5, .int 6, .int 7] ++ forq 4 [ws "negq"]) == "ERROR:signature_error") = true := by
  native_decide

/-- THE LOOP'S VALUES ARE STEPPED WHERE IT STOOD: a word written after the
    loop takes them as it takes any values before it. Here `negq` takes the
    last index. -/
theorem values_are_stepped :
    ([1, 2, 3, 4].all fun (n : Int) =>
      out (forq n [w "i"] ++ [w "negq"]) == ints (indices 0 (n - 1) 1 20 ++ [-(n - 1)])) = true := by
  native_decide

/-- So a FUNCTION a round leaves — a value for as long as the loop runs —
    is called, when the loop ends, on what follows the loop. -/
theorem a_function_left_is_called :
    ([Value.int 9, .int (-4), .int 0].all fun v =>
      out (forq 1 [.word { name := "negq", forceVal := true }] ++ [v]) == out [w "negq", v]) = true := by
  native_decide

/-! ## The conditional loop -/

def whileq (test body : List Value) : List Value :=
  [w "whileq", .list test false, .list body false]

/-- `c` bound to each of `vs` in turn, the last on top: a body that pops
    it changes what the condition reads. -/
def stackOf (vs : List Value) : Defs :=
  vs.foldl (fun d v => d.push "c" v) Boru.Corespec.fixtureDefs

/-- Values the truthiness rule calls true, and values it calls false. -/
def trues : List Value :=
  [.bool true, .int 1, .int (-3), .str "a", .str "false", .list [.int 0] false,
   .map [("a", .none)] false, .typeLit TAny, .typeLit TString, .atom "null",
   .typedList (.typeLit TInteger) [], .disjunct [.int 1, .int 2]]
def falses : List Value :=
  [.bool false, .int 0, .str "", .none, .list [] false, .map [] false,
   .typeLit TNone, .typeLit TInteger, .typeLit TBoolean, .typeLit TList, .typeLit TMap,
   .disjunct []]

/-- THE ONE TRUTHINESS RULE: presence, not content. -/
theorem truth_is_presence :
    (trues.all coerceBoolean && falses.all (!coerceBoolean ·)) = true := by
  native_decide

/-- A CONDITIONAL LOOP RUNS WHILE ITS CONDITION IS TRUE. With `c` bound to
    a false value beneath k true ones, and a body that pops `c` and leaves
    a 7, the loop runs exactly k rounds — for every choice of the values. -/
theorem runs_while_true :
    (falses.all fun f => trues.all fun t => [0, 1, 2, 3].all fun (k : Nat) =>
      outWith (stackOf (f :: List.replicate k t))
        (whileq [w "c"] [w "undef", w "c", .int 7]) == ints (List.replicate k 7)) = true := by
  native_decide

/-- THE CONDITION'S LAST VALUE DECIDES, whatever it left before it. -/
theorem last_value_decides :
    (trues.all fun t => falses.all fun f =>
      -- a true value before a false one: no round
      out (whileq [t, f] [.int 7]) == "" &&
      -- a false value before the popped stack's top: the top decides
      outWith (stackOf [f, t]) (whileq [f, w "c"] [w "undef", w "c", .int 7]) == "7") = true := by
  native_decide

/-- A CONDITION THAT LEAVES NO VALUE IS AN ERROR — and only that: a
    condition that leaves a false value, of any kind, ends the loop. -/
theorem no_condition_value_is_an_error :
    (out (whileq [] [.int 7]) == "ERROR:runtime_error" &&
      falses.all fun f => out (whileq [f] [.int 7]) == "") = true := by
  native_decide

/-- WHAT FOLLOWS A LOOP FOLLOWS IT: a loop of no rounds leaves the program
    as if it had not been written. -/
theorem an_empty_loop_is_nothing :
    ([Value.int 9, .str "a"].all fun v =>
      out ([v] ++ forq 0 [.int 7] ++ [v]) == canonValues [v, v] &&
      out ([v] ++ whileq [.bool false] [.int 7] ++ [v]) == canonValues [v, v]) = true := by
  native_decide

end Boru.Core.Counted
