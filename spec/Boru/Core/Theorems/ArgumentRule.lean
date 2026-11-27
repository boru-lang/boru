/-
  Boru.Core.Theorems.ArgumentRule — one rule, at every arity.

  A call binds its arguments in signature order: positions fill from the
  forward stack up to the barrier, then from the value stack, top first.
  A call form only chooses where the split falls — so every split of the
  same operands is the SAME call, and no arity is special.

  The theorems run the machine. `probe` words return the argument vector
  they were handed, as one list, so a result shows the binding itself.
  Each theorem quantifies over every choice of operands from a fixed set
  of distinct literals and is discharged by `native_decide`.
-/
import Boru.Core.Engine
import Boru.Core.Canon

namespace Boru.Core.ArgumentRule
open Boru.Core

/-- A probe word: `n` gradual parameters, forward-eligible up to
    `barrier`; it returns its argument vector as a list. -/
def probe (name : String) (n barrier : Nat) : String × List Value :=
  (name, [.fn name [.mk ((List.range n).map fun i => .mk s!"a{i}" TAny none false false)
    [TAny] barrier (.native "vec") false] 0])

def defs : Defs :=
  [probe "f1" 1 1, probe "f2" 2 2, probe "f3" 3 3, probe "f4" 4 4,
   -- barrier in the middle: slot 0 forward-eligible, the rest stack-only
   probe "g3" 3 1,
   -- stack form: nothing forward-eligible
   probe "s2" 2 0]

def natives : Natives := [("vec", .plain fun args d => pure ([.list args false], d))]

def w (name : String) : Value := .word { name }

/-- Run a tape; render the residual, or the error code. -/
def out (tape : List Value) : String :=
  match run natives defs tape with
  | .ok vs => canonValues vs
  | .error e => "ERROR:" ++ e.code

/-- Distinct literal operands of several types. -/
def operands : List Value :=
  [.int 1, .int 2, .int 3, .str "a", .bool true, .none, .list [.int 9] false]

/-- The binding `[a b …]` as it renders. -/
def bound (args : List Value) : String := canon (.list args false)

/-- ARITY 2. `f a b`, `b f a` and `b a f` are one call: slot 0 is `a`,
    slot 1 is `b`, wherever the word stands. -/
theorem arity2_splits :
    (operands.all fun a => operands.all fun b =>
      out [w "f2", a, b] == bound [a, b]
        && out [b, w "f2", a] == bound [a, b]
        && out [b, a, w "f2"] == bound [a, b]) = true := by
  native_decide

/-- ARITY 3. Four splits, one call. -/
theorem arity3_splits :
    (operands.all fun a => operands.all fun b => operands.all fun c =>
      out [w "f3", a, b, c] == bound [a, b, c]
        && out [c, w "f3", a, b] == bound [a, b, c]
        && out [c, b, w "f3", a] == bound [a, b, c]
        && out [c, b, a, w "f3"] == bound [a, b, c]) = true := by
  native_decide

/-- ARITY 4. Five splits, one call — nothing changes with arity. -/
theorem arity4_splits :
    ([Value.int 1, .int 2, .str "a", .bool true].all fun a =>
     [Value.int 1, .int 2, .str "a", .bool true].all fun b =>
     [Value.int 1, .int 2, .str "a", .bool true].all fun c =>
     [Value.int 1, .int 2, .str "a", .bool true].all fun d =>
      out [w "f4", a, b, c, d] == bound [a, b, c, d]
        && out [d, w "f4", a, b, c] == bound [a, b, c, d]
        && out [d, c, w "f4", a, b] == bound [a, b, c, d]
        && out [d, c, b, w "f4", a] == bound [a, b, c, d]
        && out [d, c, b, a, w "f4"] == bound [a, b, c, d]) = true := by
  native_decide

/-- There is no "swap form". `a f b` and `f a b` are DIFFERENT calls —
    different splits of the same operands — at arity two exactly as at
    any other: `a f b` binds `[b a]`, `f a b` binds `[a b]`. -/
theorem infix_is_not_forward :
    (operands.all fun a => operands.all fun b =>
      out [a, w "f2", b] == bound [b, a] && out [w "f2", a, b] == bound [a, b]) = true := by
  native_decide

/-- All operands on the value stack is FORTH order: the top of the stack
    is slot 0. -/
theorem stack_form_is_forth_order :
    (operands.all fun a => operands.all fun b => operands.all fun c =>
      out [a, b, c, w "f3"] == bound [c, b, a]) = true := by
  native_decide

/-- THE BARRIER. A signature takes at most `barrier` arguments forward;
    the rest come from the stack even when values are written after the
    word. `g3` (barrier 1) written `c b g3 a x` binds `[a b c]` and
    leaves `x` alone. -/
theorem barrier_bounds_forward :
    (operands.all fun a => operands.all fun b => operands.all fun c => operands.all fun x =>
      out [c, b, w "g3", a, x] == bound [a, b, c] ++ " " ++ canon x) = true := by
  native_decide

/-- A stack-only word never reads forward: what follows it is untouched. -/
theorem stack_only_ignores_forward :
    (operands.all fun a => operands.all fun b => operands.all fun x =>
      out [b, a, w "s2", x] == bound [a, b] ++ " " ++ canon x) = true := by
  native_decide

/-- Forward collection takes exactly the signature's arity: a value
    written past it is the next result, not an argument. -/
theorem forward_takes_arity :
    (operands.all fun a => operands.all fun b => operands.all fun x =>
      out [w "f2", a, b, x] == bound [a, b] ++ " " ++ canon x
        && out [w "f1", a, x] == bound [a] ++ " " ++ canon x) = true := by
  native_decide

/-- An under-supplied call is refused, never silently applied — the
    negative half of every theorem above. -/
theorem undersupplied_is_refused :
    (operands.all fun a =>
      out [w "f2", a] == "ERROR:signature_error"
        && out [w "f3", a, a] == "ERROR:signature_error"
        && out [a, w "s2"] == "ERROR:signature_error"
        && out [w "s2", a, a] == "ERROR:signature_error") = true := by
  native_decide

/-- `end` closes a collection: the word fires with what it holds, or is
    refused — it never reaches past the barrier for more. -/
theorem end_closes_collection :
    (operands.all fun a => operands.all fun b =>
      out [w "f2", a, .endMark, b] == "ERROR:signature_error"
        && out [b, w "f2", a, .endMark] == bound [a, b]
        && out [w "f2", a, b, .endMark] == bound [a, b]) = true := by
  native_decide

/-- A paren group is evaluated first and supplies its result as ONE
    forward argument; an EMPTY group supplies nothing and the call is
    refused for want of a value. -/
theorem group_is_one_argument :
    (operands.all fun a => operands.all fun b =>
      out [w "f2", .openParen, w "f1", a, .closeParen, b] == bound [.list [a] false, b]
        && out [w "f2", .openParen, .closeParen, b] == "ERROR:no_value_error") = true := by
  native_decide

/-- THE STRICT FORWARD BARRIER. Another function word ends a parked
    collection: a word that cannot fire with what it already holds is
    stranded, however the next word would have turned out. Grouping the
    inner call is what feeds the outer one. -/
theorem function_word_is_a_barrier :
    (operands.all fun a => operands.all fun b =>
      out [w "f2", a, w "f1", b] == "ERROR:signature_error"
        && out [w "f2", a, .openParen, w "f1", b, .closeParen] == bound [a, .list [b] false]) = true := by
  native_decide

/-! ## `/u` — the same call, reversed

`f/u` is the function `f` with its argument order reversed: a wrapper
whose every parameter is collectable forward, which re-dispatches the
original with what it collected, reversed. -/

def wu (name : String) : Value := .word { name, forceUsurp := true }

/-- USURP REVERSES THE BINDING, AT EVERY BARRIER. `f/u a b` hands the
    original `b a` — its slot i takes the operand written (n−1−i)-th —
    whether the original collects forward (`f2`) or from the stack (`s2`). -/
theorem usurp_reverses :
    (operands.all fun a => operands.all fun b =>
      out [wu "f1", a] == bound [a]
        && out [wu "f2", a, b] == bound [b, a]
        && out [wu "s2", a, b] == bound [b, a]) = true := by
  native_decide

/-- At arity 3 the same, with the barrier at the end (`f3`) and in the
    middle (`g3`). -/
theorem usurp_reverses_arity3 :
    (operands.all fun a => operands.all fun b => operands.all fun c =>
      out [wu "f3", a, b, c] == bound [c, b, a]
        && out [wu "g3", a, b, c] == bound [c, b, a]) = true := by
  native_decide

/-- THE WRAPPER IS AN ORDINARY FUNCTION, so the one rule splits ITS
    operands too: `f3/u a b c`, `c f3/u a b`, `c b f3/u a` and
    `c b a f3/u` are one call — and it is all-forward even over an
    original that is not (`g3`). -/
theorem usurp_splits :
    (operands.all fun a => operands.all fun b => operands.all fun c =>
      out [c, wu "f3", a, b] == bound [c, b, a]
        && out [c, b, wu "f3", a] == bound [c, b, a]
        && out [c, b, a, wu "f3"] == bound [c, b, a]
        && out [c, b, wu "g3", a] == bound [c, b, a]) = true := by
  native_decide

/-- Only a FUNCTION has an argument order to reverse: `/u` on a name bound
    to anything else is `illegal_ref`, and on a name bound to nothing
    `undefined_word` — with or without `/v` beside it. -/
theorem usurp_needs_a_function :
    let code (ds : Defs) (tape : List Value) : String :=
      match run natives ds tape with
      | .error e => e.code
      | .ok _ => "ok"
    ((operands.all fun v =>
        code (defs.push "x" v) [wu "x"] == "illegal_ref"
          && code (defs.push "x" v) [.word { name := "x", forceUsurp := true, forceVal := true }]
               == "illegal_ref")
      && code defs [wu "nosuch"] == "undefined_word") = true := by
  native_decide

end Boru.Core.ArgumentRule
