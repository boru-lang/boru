/-
  Boru.Core.Theorems.ZeroArg — a function of no parameters.

  A call of a function that takes nothing needs nothing beside it. So a
  function VALUE of no parameters is called wherever it is stepped: at
  the top level, in a group, in a list literal, where a word hands it
  back. A word that is collecting takes the function, not its result.

  An ANONYMOUS function of no parameters is DATA where it stands. Only an
  application that was asked for calls it: the APPLICATION MARK, a
  one-shot signal put on an anonymous function whose every signature
  takes nothing, and spent by the step that meets it.

  That the two kinds differ is stated here as the kernel has it, not as
  the rule (NUR417; ADR-016 forbids an exception keyed on where a
  function came from), and the theorem that sets them side by side is
  marked.

  A NAME bound to either kind calls it; `/v` is the function, placed and
  not stepped.

  The theorems run the machine over the corpus fixture — `afn0q` builds
  the anonymous kind and `applyq` is the application — and are discharged
  by `native_decide` over the stated domains.
-/
import Boru.Core.Theorems.Counted

namespace Boru.Core.ZeroArg
open Boru.Core
open Boru.Core.Counted (w ws outWith out ints)

def pend (vs : List Value) : Value := .list vs true
def quot (vs : List Value) : Value := .list vs false
def one (v : Value) : String := canonValues [v]
def refused : String := "ERROR:signature_error"

def results : List Int := [42, -1, 0]

/-- A function of no parameters whose body is `body`: NAMED, as
    `fn( RTYPE [ body ] )` writes it, or ANONYMOUS, as `afn0q` builds it. -/
def named (body : List Value) : Value :=
  .fn "f" [.mk [] [TAny] 0 (.boru body [] false none []) false] 9201
def anon (body : List Value) : Value :=
  .fn "" [.mk [] [TAny] 0 (.boru body [] true none []) false] 9202

/-- `( afn0q [q body ] )`, as tokens: the anonymous function, built where
    it stands. -/
def built (body : List Value) : List Value := [.openParen, w "afn0q", quot body, .closeParen]

/-! ## Called where it is stepped -/

/-- A FUNCTION OF NO PARAMETERS IS CALLED WHERE IT IS STEPPED: alone, in a
    group, in a list literal — and what stands before or after it is not
    its argument. In a QUOTED list nothing is stepped, and it is a value
    there. -/
theorem a_function_of_no_parameters_is_called_where_it_is_stepped :
    (results.all fun n =>
      let f := named [.int n]
      out [f] == ints [n] &&
      out [.openParen, f, .closeParen] == ints [n] &&
      out [pend [f]] == one (quot [.int n]) &&
      out [f, .int 5] == ints [n, 5] &&
      out [.int 5, f] == ints [5, n] &&
      out [f, w "negq"] == ints [-n] &&
      out [quot [f]] == one (quot [f])) = true := by
  native_decide

/-- A WORD THAT IS COLLECTING TAKES THE FUNCTION, not its result: a typed
    slot refuses it, and a word that hands back what it took hands back a
    function — which is then stepped, and called, each copy. -/
theorem a_collecting_word_takes_the_function :
    (results.all fun n =>
      let f := named [.int n]
      out [w "negq", f] == refused &&
      out [w "dupq", f] == ints [n, n] &&
      out [w "bothq", .int 1, f] == ints [n] &&
      out [w "bothq", f, .int 1] == ints [1]) = true := by
  native_decide

/-- THE CALL IS A CALL LIKE ANY OTHER: its frame is its own — the body
    sees an empty stack and an empty args list — and its return is
    checked. -/
theorem the_call_is_a_call_like_any_other :
    (out [.int 7, .int 8, named [w "depthq"]] == ints [7, 8, 0] &&
     out [named [w "argsq"]] == one (quot []) &&
     out [named [.int 1, .int 2]] == "ERROR:type_error" &&
     out [named []] == "ERROR:type_error") = true := by
  native_decide

/-! ## The anonymous one, and the application -/

/-- AN ANONYMOUS FUNCTION OF NO PARAMETERS IS DATA WHERE IT STANDS: alone,
    in a group, among other values, and where a word hands it back — each
    copy. -/
theorem an_anonymous_one_is_data_where_it_stands :
    (results.all fun n =>
      let f := anon [.int n]
      out [f] == one f &&
      out (built [.int n]) == one f &&
      out [f, .int 5] == canonValues [f, .int 5] &&
      out [.int 5, f] == canonValues [.int 5, f] &&
      out [w "dupq", f] == canonValues [f, f] &&
      out ([w "bothq", .int 1] ++ built [.int n]) == one f) = true := by
  native_decide

/-- AN APPLICATION CALLS IT. `applyq` marks the function; the step that
    meets the mark makes the call — forward or from the stack — and what
    follows is not its argument. The call's frame is its own, and an
    error in the body is the call's. -/
theorem an_application_calls_it :
    (results.all fun n =>
      out ([w "applyq"] ++ built [.int n]) == ints [n] &&
      out (built [.int n] ++ [w "applyq"]) == ints [n] &&
      out ([w "applyq"] ++ built [.int n] ++ [.int 5]) == ints [n, 5] &&
      out ([.int 5, w "applyq"] ++ built [w "addq", .int n]) == refused &&
      out ([w "applyq"] ++ built [w "nosuch"]) == "ERROR:undefined_word" &&
      -- a function that answers a function, applied twice
      out ([w "applyq", .openParen, w "applyq"] ++ built [w "afn0q", quot [.int n]] ++ [.closeParen])
        == ints [n]) = true := by
  native_decide

/-- THE MARK IS SPENT BY THE CALL: what the call leaves is not applied
    again. A function that answers a function, applied ONCE, leaves the
    inner function — as data. -/
theorem the_mark_is_spent_by_the_call :
    (results.all fun n =>
      out ([w "applyq"] ++ built [w "afn0q", quot [.int n]]) == one (anon [.int n])) = true := by
  native_decide

/-- THE MARK IS ONLY FOR AN ANONYMOUS FUNCTION OF NO PARAMETERS. Any other
    value is handed back as it is: a literal, a list, an anonymous
    function of one parameter. That function is then a function value a
    word handed back, and is stepped as one: data with nothing beside it,
    called when an argument follows. -/
theorem the_mark_is_only_for_an_anonymous_function_of_no_parameters :
    (results.all fun n =>
      let takesOne := [.openParen, w "afn", w "x", quot [w "x"], .closeParen]
      out [w "applyq", .int n] == ints [n] &&
      out [w "applyq", quot [.int n]] == one (quot [.int n]) &&
      out ([w "applyq"] ++ takesOne) == out takesOne &&
      out ([w "applyq"] ++ takesOne ++ [.int n]) == ints [n]) = true := by
  native_decide

/-! ## Bound to a name -/

/-- A NAME CALLS EITHER KIND, each time it is stepped; and `/v` is the
    function, placed and not stepped — so not called, whichever kind. -/
theorem a_name_calls_either_kind :
    (results.all fun n =>
      [named [.int n], anon [.int n]].all fun f =>
        let d := Boru.Corespec.fixtureDefs.push "z" f
        let zv : Value := .word { name := "z", forceVal := true }
        outWith d [w "z"] == ints [n] &&
        outWith d [w "z", w "z"] == ints [n, n] &&
        outWith d [w "z", w "negq"] == ints [-n] &&
        outWith d [pend [w "z"]] == one (quot [.int n]) &&
        outWith d [zv] == one f &&
        outWith d [zv, .int 5] == canonValues [f, .int 5]) = true := by
  native_decide

/-- BESIDE AN OVERLOAD THAT TAKES ONE, the one of no parameters answers
    when there is nothing the other takes — nothing at all, or an
    argument the other refuses, which is then left where it was. -/
theorem beside_an_overload_that_takes_one :
    (results.all fun n =>
      let takesOne : Value :=
        .fn "z" [.mk [.mk "k" TInteger none false false false {}] [TAny] 1
          (.boru [w "k"] [] false none []) false] 9203
      let d := (Boru.Corespec.fixtureDefs.push "z" (named [.int n])).push "z" takesOne
      outWith d [w "z"] == ints [n] &&
      outWith d [w "z", .int 5] == ints [5] &&
      outWith d [.int 5, w "z"] == ints [5] &&
      outWith d [w "z", .str "a"] == canonValues [.int n, .str "a"]) = true := by
  native_decide

/-! ## As the kernel has it -/

/-- AS THE KERNEL HAS IT (NUR417): WHERE IT CAME FROM DECIDES. The same
    body, the same signature: stepped with nothing asked of it, the named
    function is called and the anonymous one is kept. Handed back twice by
    a word, the named one is called twice and the anonymous one is two
    functions. So an expression cannot hand on a named function of no
    parameters — the step that would deliver it spends it. -/
theorem nur417_where_it_came_from_decides :
    (results.all fun n =>
      out [named [.int n]] == ints [n] &&
      out [anon [.int n]] == one (anon [.int n]) &&
      out [.openParen, named [.int n], .closeParen] == ints [n] &&
      out [.openParen, anon [.int n], .closeParen] == one (anon [.int n]) &&
      (let dn := Boru.Corespec.fixtureDefs.push "z" (named [.int n])
       let da := Boru.Corespec.fixtureDefs.push "z" (anon [.int n])
       let zv : Value := .word { name := "z", forceVal := true }
       outWith dn [w "dupq", zv] == ints [n, n] &&
       outWith da [w "dupq", zv] == canonValues [anon [.int n], anon [.int n]])) = true := by
  native_decide

end Boru.Core.ZeroArg
