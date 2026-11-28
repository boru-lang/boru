/-
  Boru.Core.Theorems.Raw — the raw-capture slots.

  An ordinary slot is handed the VALUE of what is written for it: a
  pending list or map is evaluated, a paren expression is run, a word is
  looked up or called. A signature may mark a slot to take its operand as
  it was WRITTEN instead, in four independent ways (`Raw`):

    list   a pending list is handed over unevaluated
    map    a pending map is
    paren  a paren expression written after the word is captured as code
    form   whatever is written after the word is captured as it stands

  This file states what each mode keeps, what it leaves alone, and that a
  mode belongs to one slot of the signature.

  Two things are stated as the kernel has them and not as the rule, and
  are marked: the usurp wrapper does not carry the modes with the
  parameters it reverses (NUR413), and a paren expression that ARRIVES at
  a waiting word is kept by the mode of the word's signature, not of the
  slot it arrives at (NUR414).

  The theorems run the machine over the corpus fixture — `rawlq`, `rawmq`,
  `rawpq`, `formq` each have the one slot, `raw2q` and `form2q` have the
  raw slot second of two — and are discharged by `native_decide` over the
  stated domains.
-/
import Boru.Core.Theorems.Counted

namespace Boru.Core.RawThm
open Boru.Core
open Boru.Core.Counted (w ws outWith out ints)

/-- A pending list, a quoted list, a paren expression. -/
def pend (vs : List Value) : Value := .list vs true
def quot (vs : List Value) : Value := .list vs false
def paren (vs : List Value) : Value := .parenExpr vs

def one (v : Value) : String := canonValues [v]

/-- `addq a b`, as tokens. -/
def sum (a b : Int) : List Value := [w "addq", .int a, .int b]

def pairs : List (Int × Int) := [(1, 2), (5, -6), (0, 9), (-4, -4)]

/-- Programs a raw slot must not run: a call, an unbound word, a word
    that would raise a signal, a call of a raw word, a nested literal. -/
def bodies : List (List Value) :=
  [ sum 1 2, [w "nosuch"], [w "breakq"], [w "rawlq", pend (sum 1 2)],
    [pend (sum 1 2), .map [("a", w "nosuch")] true], [], [.int 1, .int 2] ]

/-- The fixture with `x` bound to `v`. -/
def withX (v : Value) : Defs := Boru.Corespec.fixtureDefs.push "x" v

/-! ## The list mode -/

/-- A PENDING LIST AT A LIST-RAW SLOT IS HANDED OVER AS IT WAS WRITTEN:
    the result is the same tokens, quoted — whichever side of the word
    the list stands on, and whatever it holds. Nothing in it is run,
    looked up, or raised. -/
theorem a_list_is_handed_over_as_written :
    (bodies.all fun b =>
      out [w "rawlq", pend b] == one (quot b) &&
      out [pend b, w "rawlq"] == one (quot b) &&
      out [pend b, ws "rawlq"] == one (quot b)) = true := by
  native_decide

/-- AN ORDINARY SLOT EVALUATES THE SAME LIST. `dupq` hands back what it
    was handed, twice. -/
theorem an_ordinary_slot_evaluates_the_list :
    (pairs.all fun (a, b) =>
      out [w "dupq", pend (sum a b)] == canonValues [quot [.int (a + b)], quot [.int (a + b)]] &&
      out [w "rawlq", pend (sum a b)] == one (quot (sum a b))) = true := by
  native_decide

/-- WHAT IS HANDED BACK IS DATA. The list is no longer pending: a second
    call, an ordinary word, a list literal, a group and the end of the
    run all leave it as it is. -/
theorem what_is_handed_back_is_data :
    (pairs.all fun (a, b) =>
      let l := pend (sum a b)
      let q := quot (sum a b)
      out [w "rawlq", l, w "rawlq"] == one q &&
      out [w "rawlq", l, w "dupq"] == canonValues [q, q] &&
      out [pend [w "rawlq", l]] == one (quot [q]) &&
      out [w "rawlq", .openParen, w "rawlq", l, .closeParen] == one q) = true := by
  native_decide

/-- THE LIST MODE IS FOR A LIST. Any other operand is taken as an
    ordinary slot takes it: a literal is itself, a group and a paren
    expression are run, and a pending MAP is evaluated — its list members
    too. -/
theorem the_list_mode_is_for_a_list :
    (pairs.all fun (a, b) =>
      out [w "rawlq", .int a] == ints [a] &&
      out ([w "rawlq", .openParen] ++ sum a b ++ [.closeParen]) == ints [a + b] &&
      out [w "rawlq", paren (sum a b)] == ints [a + b] &&
      out [w "rawlq", .map [("a", paren (sum a b))] true] == one (.map [("a", .int (a + b))] false) &&
      out [w "rawlq", .map [("a", pend (sum a b))] true]
        == one (.map [("a", quot [.int (a + b)])] false)) = true := by
  native_decide

/-- A NAME BOUND TO A PENDING LIST DELIVERS IT PENDING, and the slot
    takes it as written — where an ordinary slot evaluates it. -/
theorem a_bound_pending_list_is_taken_as_written :
    (pairs.all fun (a, b) =>
      let d := withX (pend (sum a b))
      outWith d [w "rawlq", w "x"] == one (quot (sum a b)) &&
      outWith d [w "x", w "rawlq"] == one (quot (sum a b)) &&
      outWith d [w "dupq", w "x"] == canonValues [quot [.int (a + b)], quot [.int (a + b)]]) = true := by
  native_decide

/-! ## The map mode -/

/-- A PENDING MAP AT A MAP-RAW SLOT IS HANDED OVER AS IT WAS WRITTEN,
    member by member — and the two modes are SEPARATE: a list at the
    map-raw slot is evaluated, as a map at the list-raw slot is. -/
theorem a_map_is_handed_over_as_written :
    (pairs.all fun (a, b) =>
      let m := Value.map [("a", paren (sum a b)), ("b", w "nosuch")] true
      out [w "rawmq", m] == one (.map [("a", paren (sum a b)), ("b", w "nosuch")] false) &&
      out [m, w "rawmq"] == one (.map [("a", paren (sum a b)), ("b", w "nosuch")] false) &&
      out [w "rawmq", pend (sum a b)] == one (quot [.int (a + b)]) &&
      out [w "rawlq", .map [("a", paren (sum a b))] true] == one (.map [("a", .int (a + b))] false)) = true := by
  native_decide

/-! ## The paren mode -/

/-- A PAREN EXPRESSION WRITTEN AFTER THE WORD IS CAPTURED AS CODE: the
    capture is the expression, whatever it holds. -/
theorem a_paren_is_captured_as_code :
    (bodies.all fun b => out [w "rawpq", paren b] == one (quot [paren b])) = true := by
  native_decide

/-- ONLY FORWARD, AND ONLY A PAREN EXPRESSION. Before the word it has run
    by the time the word is reached; a GROUP on the tape is evaluated
    before any slot sees it; a literal and a bound name are their values,
    and a pending list is evaluated. -/
theorem the_paren_mode_is_for_a_forward_paren :
    (pairs.all fun (a, b) =>
      out [paren (sum a b), w "rawpq"] == one (quot [.int (a + b)]) &&
      out ([w "rawpq", .openParen] ++ sum a b ++ [.closeParen]) == one (quot [.int (a + b)]) &&
      out [w "rawpq", .int a] == one (quot [.int a]) &&
      outWith (withX (.int a)) [w "rawpq", w "x"] == one (quot [.int a]) &&
      out [w "rawpq", pend (sum a b)] == one (quot [quot [.int (a + b)]]) &&
      out [w "rawpq", w "nosuch"] == "ERROR:undefined_word") = true := by
  native_decide

/-! ## The form mode -/

/-- A FORM SLOT TAKES A WORD AS IT IS WRITTEN: unbound, bound to a value,
    a function word, the word itself — none is looked up or called, and a
    function word's operands are not the form's. -/
theorem a_form_is_the_word_as_written :
    (pairs.all fun (a, b) =>
      out [w "formq", w "nosuch"] == one (quot [w "nosuch"]) &&
      outWith (withX (.int a)) [w "formq", w "x"] == one (quot [w "x"]) &&
      out ([w "formq"] ++ sum a b) == canonValues [quot [w "addq"], .int a, .int b] &&
      out [w "formq", w "formq"] == one (quot [w "formq"]) &&
      out [w "formq", w "breakq"] == one (quot [w "breakq"]) &&
      out [w "formq", .word { name := "nosuch", argCount := some 2 }]
        == one (quot [.word { name := "nosuch", argCount := some 2 }])) = true := by
  native_decide

/-- A PAREN EXPRESSION AT A FORM SLOT IS CODE, and a literal is itself —
    but THE FORM MODE DOES NOT KEEP A LIST OR A MAP UNEVALUATED: the slot
    captures the pending literal, and the call evaluates what it captured.
    Taking one raw is the list or map mode's. -/
theorem a_form_slot_still_evaluates_a_pending_literal :
    (pairs.all fun (a, b) =>
      out [w "formq", paren (sum a b)] == one (quot [paren (sum a b)]) &&
      out [w "formq", .int a] == one (quot [.int a]) &&
      out [w "formq", pend (sum a b)] == one (quot [quot [.int (a + b)]]) &&
      out [w "formq", .map [("a", paren (sum a b))] true]
        == one (quot [.map [("a", .int (a + b))] false])) = true := by
  native_decide

/-- `/v` IS READ BEFORE THE FORM IS CAPTURED, as it is at a name-capturing
    slot: the capture is the VALUE the word is bound to, and an unbound
    `/v` word is an error even here. -/
theorem a_reference_is_read_before_the_form :
    (pairs.all fun (a, _) =>
      outWith (withX (.int a)) [w "formq", .word { name := "x", forceVal := true }] == one (quot [.int a]) &&
      out [w "formq", .word { name := "nosuch", forceVal := true }] == "ERROR:undefined_word") = true := by
  native_decide

/-- WITH NOTHING WRITTEN AFTER THE WORD the form slot is filled from the
    stack, like any slot — with a value: what stands there was stepped
    before the word was reached. And with nothing at all, the word has no
    argument. -/
theorem a_form_slot_is_filled_from_the_stack_too :
    (pairs.all fun (a, b) =>
      out [.int a, w "formq"] == one (quot [.int a]) &&
      out [paren (sum a b), w "formq"] == one (quot [.int (a + b)]) &&
      out [w "nosuch", w "formq"] == "ERROR:undefined_word" &&
      out [w "formq"] == "ERROR:signature_error" &&
      out [w "formq", .endMark] == "ERROR:signature_error") = true := by
  native_decide

/-! ## A mode is one slot's -/

/-- A MODE BELONGS TO ONE POSITION OF THE SIGNATURE, not to the word and
    not to a place on the tape. With the raw slot second of two: the
    first evaluates its list and looks its word up; the second keeps its
    own — whether it is filled forward or from the stack. -/
theorem a_mode_is_one_slots :
    (pairs.all fun (a, b) =>
      let l := pend (sum a b)
      let q := quot (sum a b)
      let v := quot [.int (a + b)]
      -- raw2q a b: `a` ordinary, `b` list-raw
      out [w "raw2q", l, l] == one (quot [v, q]) &&
      out [l, w "raw2q", l] == one (quot [v, q]) &&
      out [l, l, w "raw2q"] == one (quot [v, q]) &&
      -- form2q a b: `a` ordinary, `b` a form
      outWith (withX (.int a)) [w "form2q", w "x", w "x"] == one (quot [.int a, w "x"]) &&
      out [w "form2q", paren (sum a b), paren (sum a b)] == one (quot [.int (a + b), paren (sum a b)]) &&
      out [w "form2q", w "nosuch", .int 1] == "ERROR:undefined_word" &&
      -- forward fills the FIRST slot: the form slot then takes a value
      -- from the stack
      out [.int b, w "form2q", .int a] == one (quot [.int a, .int b])) = true := by
  native_decide

/-! ## As the kernel has it -/

/-- AS THE KERNEL HAS IT (NUR413): THE USURP WRAPPER DOES NOT CARRY THE
    MODES WITH THE PARAMETERS IT REVERSES. `f/u a b` is `f b a` — except
    that the wrapper drops the list and map modes, so the list or map is
    evaluated before the original is handed it; and keeps the paren and
    form modes at the positions they had in the ORIGINAL, which are other
    parameters' positions once the parameters are reversed. -/
theorem nur413_usurp_does_not_carry_the_modes :
    (pairs.all fun (a, b) =>
      let l := pend (sum a b)
      let u (name : String) : Value := .word { name, forceUsurp := true }
      -- one parameter: nothing to reverse, and the list is evaluated
      out [u "rawlq", l] == one (quot [.int (a + b)]) &&
      out [w "rawlq", l] == one (quot (sum a b)) &&
      out [u "rawmq", .map [("a", paren (sum a b))] true] == one (.map [("a", .int (a + b))] false) &&
      -- two: `raw2q/u l 1` is not `raw2q 1 l`
      out [u "raw2q", l, .int 1] == one (quot [.int 1, quot [.int (a + b)]]) &&
      out [w "raw2q", .int 1, l] == one (quot [.int 1, quot (sum a b)]) &&
      -- the form mode stays at the ORIGINAL's second position: the
      -- wrapper captures its second operand, which the original then
      -- evaluates at its first slot, and evaluates its first operand,
      -- which the original was to capture
      out [u "form2q", paren (sum a b), paren (sum b a)] == one (quot [.int (a + b), .int (a + b)]) &&
      out [w "form2q", paren (sum b a), paren (sum a b)] == one (quot [.int (a + b), paren (sum a b)]) &&
      -- one parameter: the position is the same, and the mode survives
      out [u "formq", w "nosuch"] == one (quot [w "nosuch"]) &&
      out [u "rawpq", paren (sum a b)] == one (quot [paren (sum a b)])) = true := by
  native_decide

/-- AS THE KERNEL HAS IT (NUR414): A PAREN EXPRESSION THAT ARRIVES IS
    KEPT BY THE SIGNATURE, NOT BY THE SLOT. Written at an ordinary slot it
    is evaluated — the plan asks the position. A NAME bound to the same
    expression, stepped while the word waits, arrives as a paren
    expression at the same ordinary slot and is captured as code, because
    another slot of the signature is a form slot. A signature with no
    such slot evaluates it. -/
theorem nur414_an_arriving_paren_is_kept_by_the_signature :
    (pairs.all fun (a, b) =>
      let p := paren (sum a b)
      out [w "form2q", p, .int 1] == one (quot [.int (a + b), .int 1]) &&
      outWith (withX p) [w "form2q", w "x", .int 1] == one (quot [p, .int 1]) &&
      outWith (withX p) [w "raw2q", w "x", .int 1] == one (quot [.int (a + b), .int 1]) &&
      -- at the raw-paren slot itself the name's expression is captured,
      -- as the rule has it
      outWith (withX p) [w "rawpq", w "x"] == one (quot [p])) = true := by
  native_decide

end Boru.Core.RawThm
