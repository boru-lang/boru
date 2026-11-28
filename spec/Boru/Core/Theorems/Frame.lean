/-
  Boru.Core.Theorems.Frame — a call's arguments: the frame's, and the list.

  A call of a boru-bodied function opens a FRAME. Its arguments reach the
  body three ways, and this file states each.

  A NAMED parameter binds its argument: the body reads it by name, and the
  binding ends with the frame.

  An UNNAMED parameter does not bind. Its argument enters the frame as a
  VALUE — the frame's first tokens, in signature order, already resolved
  where the call was made and never stepped again — so the body reads it
  from the stack. What the body leaves of them is not returned: beneath
  the declared returns, up to as many values as there are unnamed
  parameters are let pass and dropped.

  THE ARGS LIST. Every argument of the call, named or not, in signature
  order, is pushed as one list beside the frame and popped when the frame
  closes — by the close, by an error, by a flow signal that leaves it.

  The allowance is a COUNT (NUR404): nothing ties the values it drops to
  the arguments, so a body that consumed its argument has a value it
  computed dropped in the argument's place. The theorem that says so is
  marked as the kernel's recorded behaviour, not as the rule.

  `__pa` is that pop as a WORD. No frame has run it since the kernel's
  close took the work over, and it is still registered: called in a body
  it pops once more than the close expects (NUR405). The last theorem
  states what happens, marked the same way.

  The theorems run the machine over the corpus fixture — `argsq` reads
  the list, `depthq` counts the frame's stack — and are discharged by
  `native_decide` over the stated domains.
-/
import Boru.Core.Theorems.Counted

namespace Boru.Core.Frame
open Boru.Core
open Boru.Core.Counted (w ws outWith ints forq)

/-- A boru-bodied function of the named parameters — `""` is an unnamed
    one — all of them `Any`, every one collectable forward, one return. -/
def fnv (name : String) (params : List String) (body : List Value) (ident : Nat) : Value :=
  .fn name [.mk (params.map fun p => .mk p TAny none false false false {}) [TAny] params.length
    (.boru body [] false none []) false] ident

/-- The fixture with `f` bound. -/
def withF (params : List String) (body : List Value) : Defs :=
  Boru.Corespec.fixtureDefs.push "f" (fnv "f" params body 9101)

/-- The fixture with `g` and `f` bound, each of one named parameter. -/
def withGF (gBody fBody : List Value) : Defs :=
  (Boru.Corespec.fixtureDefs.push "g" (fnv "g" ["m"] gBody 9102)).push "f" (fnv "f" ["n"] fBody 9103)

def out (params : List String) (body tape : List Value) : String := outWith (withF params body) tape

def lists (vs : List Value) : String := canonValues [.list vs false]

/-- Arguments: distinct integers, so an order is visible. -/
def triples : List (Int × Int × Int) := [(1, 2, 3), (5, -6, 7), (0, 9, -4)]

/-! ## Unnamed parameters -/

/-- THE FRAME HOLDS ITS UNNAMED ARGUMENTS, AND NOTHING ELSE. Under one,
    two and three unnamed parameters the body's stack is that deep,
    whatever the caller's stack holds; a named parameter adds nothing to
    it. -/
theorem the_frame_holds_its_unnamed_arguments :
    (triples.all fun (a, b, c) =>
      out [""] [w "depthq"] [.int 8, .int 9, w "f", .int a] == ints [8, 9, 1] &&
      out ["", ""] [w "depthq"] [.int 8, .int 9, w "f", .int a, .int b] == ints [8, 9, 2] &&
      out ["", "", ""] [w "depthq"] [.int 8, w "f", .int a, .int b, .int c] == ints [8, 3] &&
      out ["x"] [w "depthq"] [.int 8, w "f", .int a] == ints [8, 0] &&
      out ["x", ""] [w "depthq"] [.int 8, w "f", .int a, .int b] == ints [8, 1]) = true := by
  native_decide

/-- THEY STAND IN SIGNATURE ORDER: the last parameter's argument is on
    top. So an empty body returns it, a stack word reads it, and the one
    argument rule decides which written value that is — written after the
    word, the last written; taken from the stack, the DEEPER one, since
    the top of the stack is the first argument; under `/u`, the first
    written. -/
theorem unnamed_arguments_stand_in_signature_order :
    (triples.all fun (a, b, _) =>
      out ["", ""] [] [w "f", .int a, .int b] == ints [b] &&
      out ["", ""] [ws "negq"] [w "f", .int a, .int b] == ints [-b] &&
      out ["", ""] [] [.int b, w "f", .int a] == ints [b] &&
      out ["", ""] [] [.int a, .int b, w "f"] == ints [a] &&
      out ["", ""] [] [.word { name := "f", forceUsurp := true }, .int a, .int b] == ints [a]) = true := by
  native_decide

/-- AN UNNAMED ARGUMENT IS INERT IN THE FRAME. It was resolved where the
    call was made and is not stepped again: a list whose elements would
    fail if they ran, and a function that would be called if it were met
    in a program, enter as the values they are. -/
theorem an_unnamed_argument_is_inert :
    (let boom := Value.list [w "boomq", .int 1] false
     let neg := (Boru.Corespec.fixtureDefs.top? "negq").getD .none
     out [""] [] [w "f", boom] == canonValues [boom] &&
     out [""] [] [w "f", neg, .int 5] == canonValues [neg, .int 5] &&
     out [""] [.int 7] [w "f", boom] == ints [7]) = true := by
  native_decide

/-- A NAMED PARAMETER BESIDE IT IS BOUND AS EVER: the body reads one
    argument by name and the other from the stack. -/
theorem named_and_unnamed_together :
    (triples.all fun (a, b, _) =>
      out ["x", ""] [w "x"] [w "f", .int a, .int b] == ints [a] &&
      out ["", "y"] [w "y"] [w "f", .int a, .int b] == ints [b] &&
      out ["", "y"] [w "addq", w "y"] [w "f", .int a, .int b] == ints [a + b] &&
      out ["x", ""] [] [w "f", .int a, .int b] == ints [b] &&
      out ["", "y"] [] [w "f", .int a, .int b] == ints [a]) = true := by
  native_decide

/-- THE DECLARED RETURNS ARE THE TOP. A body that leaves its own value
    above the arguments returns that value, and the arguments beneath it
    are dropped — one, two or three of them. One value more than the
    returns and the arguments account for is the error it is in any
    frame. -/
theorem the_declared_returns_are_the_top :
    (triples.all fun (a, b, c) =>
      out [""] [.int 7] [w "f", .int a] == ints [7] &&
      out ["", ""] [.int 7] [w "f", .int a, .int b] == ints [7] &&
      out ["", "", ""] [.int 7] [w "f", .int a, .int b, .int c] == ints [7] &&
      out [""] [.int 7, .int 8] [w "f", .int a] == "ERROR:type_error" &&
      out ["", ""] [.int 7, .int 8] [w "f", .int a, .int b] == "ERROR:type_error" &&
      out ["", "", ""] [.int 7, .int 8] [w "f", .int a, .int b, .int c] == "ERROR:type_error") = true := by
  native_decide

/-- NUR404, AS THE KERNEL HAS IT — recorded, not the rule. The allowance
    is a count: a body that CONSUMED its argument and left two values has
    the lower one dropped as if it were the argument, where the same body
    over a named parameter is refused. -/
theorem the_allowance_is_a_count :
    (triples.all fun (a, _, _) =>
      out [""] [ws "negq", .int 7] [w "f", .int a] == ints [7] &&
      out ["n"] [w "negq", w "n", .int 7] [w "f", .int a] == "ERROR:type_error") = true := by
  native_decide

/-! ## The args list -/

/-- THE ARGS LIST IS EVERY ARGUMENT, IN SIGNATURE ORDER — named, unnamed
    or both, however the call collected them: forward as written, from
    the stack top first, reversed under `/u`. -/
theorem the_args_list_is_every_argument :
    (triples.all fun (a, b, c) =>
      [["x", "y", "z"], ["", "", ""], ["x", "", "z"], ["", "y", ""]].all fun ps =>
        out ps [w "argsq"] [w "f", .int a, .int b, .int c] == lists [.int a, .int b, .int c] &&
        out ps [w "argsq"] [.int c, w "f", .int a, .int b] == lists [.int a, .int b, .int c] &&
        out ps [w "argsq"] [.int a, .int b, .int c, w "f"] == lists [.int c, .int b, .int a] &&
        out ps [w "argsq"] [.word { name := "f", forceUsurp := true }, .int a, .int b, .int c]
          == lists [.int c, .int b, .int a]) = true := by
  native_decide

/-- A LIST ARGUMENT IS IN THE LIST AS THE LIST. The quote that keeps a
    list from running is its named BINDING's — reading the name answers
    the quoted list — and the args list holds the argument itself. -/
theorem a_list_argument_is_itself :
    (let xs := Value.list [.int 1, .int 2] false
     out ["n"] [w "argsq"] [w "f", xs] == lists [xs] &&
     out ["n"] [w "n"] [w "f", xs] == canonValues [.quoted xs]) = true := by
  native_decide

/-- EACH CALL HAS ITS OWN, AND IT ENDS WITH THE CALL. A call made in a
    body reads its own arguments; when it returns the outer call's are
    the list again; after the outer call there is none — and none at the
    top level, nor in a group there. -/
theorem each_call_has_its_own :
    (triples.all fun (a, b, _) =>
      outWith (withGF [w "argsq"] [w "g", .int b]) [w "f", .int a] == lists [.int b] &&
      outWith (withGF [w "m"] [w "bothq", .openParen, w "g", .int b, .closeParen,
          .openParen, w "argsq", .closeParen]) [w "f", .int a] == lists [.int a] &&
      outWith (withGF [w "m"] [w "n"]) [w "f", .int a, w "argsq"] == "ERROR:fixture_no_args" &&
      out [] [] [w "argsq"] == "ERROR:fixture_no_args" &&
      out [] [] [.openParen, w "argsq", .closeParen] == "ERROR:fixture_no_args") = true := by
  native_decide

/-- A LOOP'S ROUND, A REGION AND A BRANCH ARE NOT CALLS: read inside
    them, the list is the enclosing function's. -/
theorem only_a_call_has_one :
    (triples.all fun (a, _, _) =>
      out ["n"] (forq 1 [w "argsq"]) [w "f", .int a] == lists [.int a] &&
      out ["n"] [w "ifq", .list [.bool true] false, .list [w "argsq"] false, .list [.int 0] false]
        [w "f", .int a] == lists [.int a] &&
      out ["n"] [w "replayq", .list [w "argsq"] false] [w "f", .int a] == lists [.int a] &&
      out ["n"] [w "eachq", .list [.int 1] false, .list [w "argsq"] false] [w "f", .int a]
        == lists [.int 1, .list [.int a] false]) = true := by
  native_decide

/-- A SIGNAL THAT LEAVES A FRAME TAKES ITS LIST. A break raised in a call
    made from a loop's round closes the call's frame: the loop ends, and
    no list is left behind. A frame the signal does NOT leave — the one
    the loop itself stands in — keeps its own. -/
theorem a_signal_takes_the_frames_list :
    (triples.all fun (a, _, _) =>
      out ["n"] [w "argsq", w "breakq"] (forq 2 [w "f", w "i"] ++ [w "argsq"]) == "ERROR:fixture_no_args" &&
      out [""] [w "breakq"] (forq 2 [w "f", w "i"] ++ [.int a]) == ints [a] &&
      outWith (withGF [w "breakq"] (forq 2 [w "g", w "i"] ++ [w "argsq"])) [w "f", .int a]
        == lists [.int a]) = true := by
  native_decide

/-! ## The retired frame-pop word -/

/-- NUR405, AS THE KERNEL HAS IT — recorded, not the rule. `__pa` IS WHAT
    THE CLOSE DOES, AND THE CLOSE DOES IT AGAIN. Called in
    a body it pops the frame's list — the named binding is untouched —
    and the frame's own close then pops the ENCLOSING call's: after an
    inner call whose body ran `__pa`, the outer call has no list. At the
    top level there is nothing to pop, and it is nothing. -/
theorem pa_pops_what_the_close_pops :
    (triples.all fun (a, b, _) =>
      out ["n"] [w "__pa", w "argsq"] [w "f", .int a] == "ERROR:fixture_no_args" &&
      out ["n"] [w "__pa", w "n"] [w "f", .int a] == ints [a] &&
      outWith (withGF [w "__pa", w "m"] [w "g", .int b, w "argsq"]) [w "f", .int a]
        == "ERROR:fixture_no_args" &&
      outWith (withGF [w "__pa", w "m"] [w "g", .int b]) [w "f", .int a, w "f", .int a] == ints [b, b] &&
      out [] [] [w "__pa", w "__pa", .int a] == ints [a]) = true := by
  native_decide

end Boru.Core.Frame
