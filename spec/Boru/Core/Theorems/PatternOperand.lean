/-
  Boru.Core.Theorems.PatternOperand — the operand a pattern judges.

  A slot's pattern may read what a container HOLDS: a map shape (the map
  must hold these members), a typed list (every element is of this type).
  Such a pattern judges the value the callee receives. A pending list or
  map standing at the slot is not that value yet — `{a: (1 add 2)}` is a
  program for a map — so the dispatch EVALUATES it before the pattern
  reads it: where it stands, as the call would, and once. Every overload
  tried after that, and the call itself, see the value.

  The evaluation is part of the dispatch. An error it raises is the
  call's error, not "no signature"; what it bound stays bound, also when
  the pattern then declines the value and another overload — or none —
  takes the call.

  A slot that takes its operand RAW is exempt: its pattern judges what
  was written, and nothing is run.

  The theorems run the machine over the corpus fixture. `mpatq` takes a
  map that holds a:1 and `lpatq` a list of Integers, each on an ordinary
  slot; `rawmpatq` and `rawlpatq` carry the same patterns on slots that
  take a map, a list, unevaluated. Each hands back what it was handed.
  Discharged by `native_decide` over the stated domains.
-/
import Boru.Core.Theorems.Counted

namespace Boru.Core.PatternOperand
open Boru.Core
open Boru.Core.Counted (w ws outWith out ints)

def pend (vs : List Value) : Value := .list vs true
def quot (vs : List Value) : Value := .list vs false
def paren (vs : List Value) : Value := .parenExpr vs
def one (v : Value) : String := canonValues [v]
def sum (a b : Int) : List Value := [w "addq", .int a, .int b]

/-- `{a: v}` pending, and as data. -/
def mapA (v : Value) : Value := .map [("a", v)] true
def dataA (v : Value) : Value := .map [("a", v)] false

def refused : String := "ERROR:signature_error"

/-- Pairs whose sum is 1 — what the map pattern admits — and pairs whose
    sum is not. -/
def pairs : List (Int × Int) := [(0, 1), (1, 0), (3, -2), (1, 1), (0, 0), (5, -6), (-4, -4)]

/-- `bindq z 5 v`: bind `z`, and leave `v`. -/
def bindThen (v : Int) : List Value := [w "bindq", w "z", .int 5, .int v]

/-- The fixture with `f` installed from a written specification of two
    overloads: `first` and `second` are each a parameter list, and the
    bodies answer `'one'` and `'two'`. -/
def twoOverloads (first second : List Value) (call : List Value) : List Value :=
  let spec := quot [quot first, quot [.typeLit TAny], quot [.str "one"],
                    quot second, quot [.typeLit TAny], quot [.str "two"]]
  [.endMark, w "def", w "f", .openParen, w "fn", spec, .closeParen, .endMark] ++ call

/-! ## A pending operand is evaluated before its pattern reads it -/

/-- A PENDING MAP IS EVALUATED BEFORE THE PATTERN READS IT. The member is
    an expression; the pattern admits the map exactly when the expression
    evaluates to what the pattern names — and the word is handed the
    evaluated map. Forward or from the stack. -/
theorem a_pending_map_is_evaluated_before_its_pattern_reads_it :
    (pairs.all fun (a, b) =>
      let want := if a + b == 1 then one (dataA (.int 1)) else refused
      out [w "mpatq", mapA (paren (sum a b))] == want &&
      out [mapA (paren (sum a b)), w "mpatq"] == want &&
      out [mapA (paren (sum a b)), ws "mpatq"] == want) = true := by
  native_decide

/-- A PENDING LIST IS EVALUATED BEFORE THE PATTERN READS IT: the list is
    a program, and what it leaves are the elements the typed-list pattern
    judges — every one of them. -/
theorem a_pending_list_is_evaluated_before_its_pattern_reads_it :
    (pairs.all fun (a, b) =>
      out [w "lpatq", pend (sum a b)] == one (quot [.int (a + b)]) &&
      out [pend (sum a b), w "lpatq"] == one (quot [.int (a + b)]) &&
      out [w "lpatq", pend [w "dupq", .int a]] == one (quot [.int a, .int a]) &&
      out [w "lpatq", pend (sum a b ++ [.str "x"])] == refused) = true := by
  native_decide

/-- WHAT IS NOT PENDING IS JUDGED AS IT STANDS. A quoted map's member is
    the expression, not its value, and a quoted list's elements are its
    tokens: neither is run for the pattern, and each is declined unless it
    already is what the pattern names. -/
theorem what_is_not_pending_is_judged_as_it_stands :
    (pairs.all fun (a, b) =>
      out [w "mpatq", dataA (paren (sum a b))] == refused &&
      out [w "lpatq", quot (sum a b)] == refused &&
      out [w "mpatq", dataA (.int 1)] == one (dataA (.int 1)) &&
      out [w "lpatq", quot [.int a, .int b]] == one (quot [.int a, .int b])) = true := by
  native_decide

/-- A NAME BOUND TO A PENDING OPERAND is the same case: the binding is
    evaluated for the pattern, and the word is handed the value. -/
theorem a_bound_pending_operand_is_evaluated_too :
    (pairs.all fun (a, b) =>
      let want := if a + b == 1 then one (dataA (.int 1)) else refused
      let dm := Boru.Corespec.fixtureDefs.push "x" (mapA (paren (sum a b)))
      let dl := Boru.Corespec.fixtureDefs.push "x" (pend (sum a b))
      outWith dm [w "mpatq", w "x"] == want &&
      outWith dm [w "x", w "mpatq"] == want &&
      outWith dl [w "lpatq", w "x"] == one (quot [.int (a + b)])) = true := by
  native_decide

/-! ## The evaluation is the dispatch's -/

/-- AN ERROR THE EVALUATION RAISES IS THE CALL'S ERROR — not "no
    signature" — and the WHOLE operand is evaluated, not the member the
    pattern names. A signal it lets out with no loop to take it is the
    run's `flow_error`. -/
theorem the_evaluations_error_is_the_calls :
    (out [w "mpatq", mapA (paren [w "boomq", .int 1])] == "ERROR:fixture_boom" &&
     out [w "mpatq", mapA (paren [w "nosuch"])] == "ERROR:undefined_word" &&
     out [w "mpatq", .map [("a", .int 1), ("b", paren [w "nosuch"])] true] == "ERROR:undefined_word" &&
     out [w "lpatq", pend [w "nosuch"]] == "ERROR:undefined_word" &&
     out [w "mpatq", mapA (paren [w "breakq"])] == "ERROR:flow_error" &&
     out [w "lpatq", pend [w "breakq"]] == "ERROR:flow_error") = true := by
  native_decide

/-- A SIGNAL A LOOP CAN TAKE abandons the call where it stands: the loop
    answers exactly as it does when the signal is raised in the call's
    place — nothing after the call in the round runs, and the call hands
    nothing back. -/
theorem a_signal_from_the_operand_abandons_the_call :
    ([("breakq", "mpatq"), ("contq", "mpatq"), ("breakq", "lpatq"), ("contq", "lpatq")].all
      fun (signal, word) =>
        let operand := if word == "mpatq" then mapA (paren [w signal]) else pend [w signal]
        let loop (body : List Value) : List Value := [.int 9, w "forq", .int 2, quot body, .int 8]
        out (loop [.int 6, w word, operand, .int 7]) == out (loop [.int 6, w signal, .int 7])) = true := by
  native_decide

/-- WHAT THE EVALUATION DID STAYS DONE: a binding a member made outlives
    the call that had it evaluated. -/
theorem what_the_evaluation_did_stays_done :
    (out [w "mpatq", mapA (paren (bindThen 1)), w "z"] == canonValues [dataA (.int 1), .int 5] &&
     out [w "lpatq", pend (bindThen 1), w "z"] == canonValues [quot [.int 1], .int 5]) = true := by
  native_decide

/-! ## Across overloads -/

/-- THE VALUE STAYS FOR THE OVERLOADS THAT FOLLOW. The first overload's
    pattern has the map evaluated and declines it; the second judges the
    value that evaluation left, and takes the call — and the binding the
    member made is there, though the overload that had it made was
    refused. An overload with no pattern takes the same value. -/
theorem the_value_stays_for_the_overloads_that_follow :
    (let call := [w "f", mapA (paren (bindThen 7)), w "z"]
     out (twoOverloads [dataA (.int 1)] [dataA (.int 7)] call) == canonValues [.str "two", .int 5] &&
     out (twoOverloads [dataA (.int 1)] [.typeLit TMap] call) == canonValues [.str "two", .int 5] &&
     out (twoOverloads [dataA (.int 7)] [dataA (.int 1)] call) == canonValues [.str "one", .int 5]) = true := by
  native_decide

/-- AN OVERLOAD A TYPE RULES OUT NEVER READS ITS PATTERN. The patterned
    overload wants an Integer beside its map and is handed a String: it is
    not a candidate, so its pattern does not have the map evaluated — the
    call that takes the map does, as it evaluates any argument. The
    answer and the binding are the same either way; the theorem pins that
    the call goes to the overload the types choose. -/
theorem a_ruled_out_overload_does_not_read_its_pattern :
    (out (twoOverloads [dataA (.int 1), .typeLit TInteger] [.typeLit TMap, .typeLit TString]
        [w "f", mapA (paren (bindThen 1)), .str "x", w "z"])
      == canonValues [.str "two", .int 5]) = true := by
  native_decide

/-! ## A raw slot is exempt -/

/-- A RAW SLOT'S PATTERN JUDGES WHAT WAS WRITTEN. A map of literals reads
    the same either way. A member that is an expression is judged AS the
    expression at the map-raw slot — it is not 1 — where the ordinary
    slot evaluates it and admits it; and a list that is a program is
    judged as its tokens at the list-raw slot. -/
theorem a_raw_slot_judges_what_was_written :
    (pairs.all fun (a, b) =>
      out [w "rawmpatq", mapA (.int 1)] == one (dataA (.int 1)) &&
      out [w "rawmpatq", mapA (paren (sum a b))] == refused &&
      out [mapA (paren (sum a b)), w "rawmpatq"] == refused &&
      out [w "rawlpatq", pend [.int a, .int b]] == one (quot [.int a, .int b]) &&
      out [w "rawlpatq", pend (sum a b)] == refused &&
      out [pend (sum a b), w "rawlpatq"] == refused) = true := by
  native_decide

/-- AT A RAW SLOT NOTHING IS RUN: what stands beside the member the
    pattern names is handed over as it was written — where the ordinary
    slot's evaluation raises its error — and no binding is made. -/
theorem at_a_raw_slot_nothing_is_run :
    (out [w "rawmpatq", .map [("a", .int 1), ("b", paren [w "nosuch"])] true]
        == one (.map [("a", .int 1), ("b", paren [w "nosuch"])] false) &&
     out [w "mpatq", .map [("a", .int 1), ("b", paren [w "nosuch"])] true] == "ERROR:undefined_word" &&
     out [w "rawmpatq", mapA (paren [w "nosuch"])] == refused &&
     out [w "rawlpatq", pend [w "nosuch"]] == refused &&
     out [w "rawmpatq", .map [("a", .int 1), ("b", paren (bindThen 2))] true, w "z"]
        == "ERROR:undefined_word") = true := by
  native_decide

/-- AS THE KERNEL HAS IT (NUR413): `/u` drops the raw mode, and the
    wrapper — which carries the pattern — evaluates the operand for it and
    admits what the word itself refuses. -/
theorem nur413_usurped_the_raw_slots_pattern_evaluates :
    (let u (name : String) : Value := .word { name, forceUsurp := true }
     out [u "rawmpatq", mapA (paren (sum 0 1))] == one (dataA (.int 1)) &&
     out [w "rawmpatq", mapA (paren (sum 0 1))] == refused &&
     out [u "rawlpatq", pend (sum 0 1)] == one (quot [.int 1]) &&
     out [w "rawlpatq", pend (sum 0 1)] == refused) = true := by
  native_decide

end Boru.Core.PatternOperand
