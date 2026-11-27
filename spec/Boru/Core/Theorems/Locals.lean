/-
  Boru.Core.Theorems.Locals — what a body binds, and what the close undoes.

  Binding is DYNAMIC: a name is bound when a word binds it and is read by
  whatever runs while it is bound. A call frame gives that a boundary.

  THE ENTRY SNAPSHOT. Once a frame has bound its captures and parameters
  it records how many bindings every name holds. Its close unbinds every
  name down to that record — so what the body bound for itself ends with
  the call, and a binding it shadowed is read again — and then takes the
  frame's own names off, one binding of each.

  The close only ever UNBINDS. A name the body unbound below its recorded
  depth stays unbound; a name it unbound and bound again holds the new
  value, at the old depth.

  Three things are stated as the kernel has them, and marked:

  * the close takes the frame's own names off by COUNT, so a body that
    unbound its own parameter costs the caller a binding (NUR406);
  * a frame records a snapshot only when its body NAMES a binding word, so
    a body that binds through any other word — the fixture's `bindq`
    stands for them — undoes nothing (NUR384);
  * and so, in such a frame, a binding made over the parameter's own name
    leaves the parameter bound after the call.

  Outside a function nothing is a boundary: a group, a loop's round, a
  region and a branch leave what they bound.

  The theorems run the machine over the corpus fixture — `def name value`
  binds at run time, `undef name` unbinds — and are discharged by
  `native_decide` over the stated domains.
-/
import Boru.Core.Theorems.Frame

namespace Boru.Core.Locals
open Boru.Core
open Boru.Core.Counted (w ws outWith ints forq)
open Boru.Core.Frame (fnv)

/-- `def name value ;` and `bindq name value ;` as tokens. -/
def bind (word name : String) (v : Value) : List Value := [w word, w name, v, .endMark]
def defv (name : String) (v : Value) : List Value := bind "def" name v
def undef (name : String) : List Value := [w "undef", w name]

/-- The fixture, the given outer bindings — oldest first — and `f`, a
    function of one parameter `n` with the given body. -/
def withOuter (outer : List (String × Value)) (body : List Value) : Defs :=
  (outer.foldl (fun d (k, v) => d.push k v) Boru.Corespec.fixtureDefs).push "f" (fnv "f" ["n"] body 9201)

def out (outer : List (String × Value)) (body tape : List Value) : String :=
  outWith (withOuter outer body) tape

/-- What a body binds and what was bound before it: distinct scalars. -/
def pairs : List (Value × Value) :=
  [(.int 3, .int 1), (.str "a", .int 2), (.bool true, .str "o"), (.none, .int 0)]

def vals (vs : List Value) : String := canonValues vs

/-! ## The entry snapshot -/

/-- WHAT A BODY BINDS ENDS WITH THE CALL. The body reads its own binding;
    after the call the name is unbound — or, bound before the call, holds
    what it held. -/
theorem a_local_ends_with_the_call :
    (pairs.all fun (v, o) =>
      out [] (defv "y" v ++ [w "y"]) [w "f", .int 5] == vals [v] &&
      out [] (defv "y" v ++ [w "y"]) [w "f", .int 5, w "y"] == "ERROR:undefined_word" &&
      out [("y", o)] (defv "y" v ++ [w "y"]) [w "f", .int 5, w "y"] == vals [v, o]) = true := by
  native_decide

/-- EVERY BINDING THE BODY MADE, AND NONE IT DID NOT. Bound once, twice or
    three times in the body, the name is back at the depth it had: with
    two bindings beneath, the call leaves exactly those two, in order. -/
theorem the_close_goes_back_to_the_entry_depth :
    (pairs.all fun (v, o) =>
      [1, 2, 3].all fun k =>
        let body := (List.replicate k (defv "y" v)).flatten ++ [w "y"]
        out [("y", .int 8), ("y", o)] body ([w "f", .int 5, w "y"] ++ undef "y" ++ [w "y"] ++ undef "y" ++ [w "y"])
          == "ERROR:undefined_word" &&
        out [("y", .int 8), ("y", o)] body ([w "f", .int 5, w "y"] ++ undef "y" ++ [w "y"])
          == vals [v, o, .int 8]) = true := by
  native_decide

/-- A LOCAL OVER THE PARAMETER, AND OVER THE FUNCTION'S OWN NAME, ends the
    same way: the caller's `n` is read after the call, and the function is
    there for the next one. -/
theorem a_local_over_the_frames_own_names :
    (pairs.all fun (v, o) =>
      out [("n", o)] (defv "n" v ++ [w "n"]) [w "f", .int 5, w "n"] == vals [v, o] &&
      out [] (defv "f" v ++ [w "f"]) [w "f", .int 5, w "f", .int 6] == vals [v, v]) = true := by
  native_decide

/-- BINDING IS DYNAMIC INSIDE THE FRAME: a function the body calls reads
    the body's local while the frame is open — and the local is gone when
    it has closed. A local of the CALLEE is gone in the caller. -/
theorem a_callee_reads_the_callers_local :
    (pairs.all fun (v, _) =>
      let g := fnv "g" ["m"] [w "y"] 9202
      let d := (Boru.Corespec.fixtureDefs.push "g" g).push "f" (fnv "f" ["n"] (defv "y" v ++ [w "g", w "n"]) 9203)
      let g2 := fnv "g" ["m"] (defv "z" v ++ [w "z"]) 9204
      let d2 := (Boru.Corespec.fixtureDefs.push "g" g2).push "f" (fnv "f" ["n"] [w "g", w "n", w "z"] 9205)
      outWith d [w "f", .int 5] == vals [v] &&
      outWith d [w "f", .int 5, w "y"] == "ERROR:undefined_word" &&
      outWith d2 [w "f", .int 5] == "ERROR:undefined_word") = true := by
  native_decide

/-- A CONTAINER THE BODY LEAVES IS EVALUATED BEFORE THE LOCAL GOES: the
    frame's residual is evaluated in the frame, and then the close
    unbinds. -/
theorem the_residual_is_evaluated_first :
    (pairs.all fun (v, _) =>
      out [] (defv "y" v ++ [.map [("a", w "y")] true]) [w "f", .int 5] == vals [.map [("a", v)] false] &&
      out [] (defv "y" v ++ [.list [w "y"] true]) [w "f", .int 5] == vals [.list [v] false] &&
      out [] (defv "y" v ++ [.map [("a", w "y")] true]) [w "f", .int 5, w "y"] == "ERROR:undefined_word") = true := by
  native_decide

/-- THE EXCEPTION: AN ANONYMOUS FUNCTION'S ONE BARE CONTAINER IS DEFERRED
    (core/go `ResidualEvalsInFrame`). A function built where it is
    written, whose body is a single list or map, leaves it unevaluated:
    it is evaluated by whatever consumes it, after the frame has closed,
    and reads the name as it is bound OUTSIDE. The same body in a named
    function, and an anonymous body that computes — a paren expression,
    or more than one token — is evaluated in the frame. -/
theorem an_anonymous_bare_container_is_deferred :
    (pairs.all fun (v, o) =>
      let anon (body : List Value) : List Value := [w "afn", w "n", .list body false, v]
      let d := Boru.Corespec.fixtureDefs.push "n" o
      outWith d (anon [.list [w "n"] true]) == vals [.list [o] false] &&
      outWith d (anon [.map [("a", w "n")] true]) == vals [.map [("a", o)] false] &&
      outWith Boru.Corespec.fixtureDefs (anon [.list [w "n"] true]) == "ERROR:undefined_word" &&
      -- a computing body is evaluated in the frame
      outWith d (anon [.openParen, .list [w "n"] true, .closeParen]) == vals [.list [v] false] &&
      outWith d (anon [.list [w "n"] true, .endMark]) == vals [.list [v] false] &&
      -- and so is a named function's bare list
      out [("n", o)] [.list [w "n"] true] [w "f", v] == vals [.list [v] false]) = true := by
  native_decide

/-- EACH CALL HAS ITS OWN. A recursive call binds the same name deeper;
    when it has closed, the outer call reads its own binding again:
    `f n = y + f (n - 1)` with `y` read AFTER the inner call. -/
theorem each_call_has_its_own_locals :
    ([0, 1, 2, 3, 4].all fun (n : Nat) =>
      let body := defv "y" (w "n") ++
        [w "ifq", .list [w "n"] false,
          .list [.openParen, w "addq", w "n", .int (-1), .closeParen, ws "f", w "y", ws "addq"] false,
          .list [.int 0] false]
      out [] body [w "f", .int n] == ints [(n * (n + 1) / 2 : Nat)]) = true := by
  native_decide

/-- A CLOSURE KEEPS A LOCAL. A function built in the body captures the
    body's binding, and reads it when the frame has closed — while the
    name itself is unbound again outside. -/
theorem a_closure_keeps_a_local :
    ([1, 2, 7].all fun (v : Int) =>
      let body := defv "y" (.int v) ++ [w "afn", w "m", .list [w "addq", w "m", w "y"] false]
      let call := [w "bothq", .int 0, .openParen, w "f", .int 5, .closeParen, .int 10]
      out [] body call == ints [10 + v] &&
      out [("y", .int 100)] body (call ++ [w "y"]) == ints [10 + v, 100]) = true := by
  native_decide

/-- A SIGNAL THAT LEAVES THE FRAME UNDOES ITS LOCALS: a break or a continue
    raised in the body closes the frame as its end would. -/
theorem a_signal_undoes_the_locals :
    (pairs.all fun (v, o) =>
      out [("y", o)] (defv "y" v ++ [w "breakq"]) (forq 2 [w "f", w "i"] ++ [w "y"]) == vals [o] &&
      out [("y", o)] (defv "y" v ++ [w "contq"]) (forq 2 [w "f", w "i"] ++ [w "y"]) == vals [o]) = true := by
  native_decide

/-! ## The close only unbinds -/

/-- A NAME THE BODY UNBOUND STAYS UNBOUND — the close goes down to the
    entry depth, never up to it. Unbound and bound again, the name holds
    the body's value at the caller's depth: the binding is REPLACED. -/
theorem the_close_undoes_nothing_below_entry :
    (pairs.all fun (v, o) =>
      out [("x", o)] (undef "x" ++ [w "n"]) [w "f", .int 5, w "x"] == "ERROR:undefined_word" &&
      out [("x", o)] (undef "x" ++ defv "x" v ++ [w "n"]) [w "f", .int 5, w "x"] == vals [.int 5, v] &&
      -- and one binding above the entry depth goes
      out [("x", o)] (undef "x" ++ defv "x" v ++ defv "x" (.int 9) ++ [w "n"]) [w "f", .int 5, w "x"]
        == vals [.int 5, v]) = true := by
  native_decide

/-- NUR406, AS THE KERNEL HAS IT — recorded, not the rule. THE CLOSE TAKES
    ONE BINDING OF EACH OF THE FRAME'S NAMES, whatever the name holds by
    then. A body that unbound its own parameter leaves the caller's
    binding of that name to be taken: bound once outside, the name is
    unbound after the call; bound twice, it holds the older value. A body
    that bound it again leaves the caller's alone. -/
theorem the_close_takes_one_binding_of_each_name :
    (pairs.all fun (v, o) =>
      out [("n", o)] (undef "n" ++ [.int 7]) [w "f", .int 5, w "n"] == "ERROR:undefined_word" &&
      out [("n", v), ("n", o)] (undef "n" ++ [.int 7]) [w "f", .int 5, w "n"] == vals [.int 7, v] &&
      -- inside, the caller's binding is what the name reads
      out [("n", o)] (undef "n" ++ [w "n"]) [w "f", .int 5] == vals [o] &&
      out [("n", o)] (undef "n" ++ defv "n" v ++ [w "n"]) [w "f", .int 5, w "n"] == vals [v, o]) = true := by
  native_decide

/-! ## The leaf frame -/

/-- NUR384'S RULE, AS THE KERNEL HAS IT — recorded, not the rule. A FRAME
    RECORDS A SNAPSHOT ONLY WHEN ITS BODY NAMES A BINDING WORD. `bindq`
    binds as `def` does and is not one: what a body binds through it alone
    stays bound after the call, over an outer binding too. Beside any
    word that is — an `undef` of an unbound name will do — the same
    binding ends with the call. -/
theorem a_leaf_frame_undoes_nothing :
    (pairs.all fun (v, o) =>
      out [] (bind "bindq" "y" v ++ [w "y"]) [w "f", .int 5, w "y"] == vals [v, v] &&
      out [("y", o)] (bind "bindq" "y" v ++ [w "y"]) [w "f", .int 5, w "y"] == vals [v, v] &&
      out [("y", o)] (bind "bindq" "y" v ++ undef "z" ++ [w "y"]) [w "f", .int 5, w "y"] == vals [v, o] &&
      out [("y", o)] (defv "y" v ++ [w "y"]) [w "f", .int 5, w "y"] == vals [v, o]) = true := by
  native_decide

/-- AND SO A PARAMETER CAN OUTLIVE ITS CALL (NUR384 again): bound over in
    a leaf frame, the parameter's name loses ONE binding at the close —
    the body's — and the argument is what the name reads afterwards. -/
theorem a_leaf_frames_parameter_can_outlive_the_call :
    (pairs.all fun (v, o) =>
      out [] (bind "bindq" "n" v ++ [w "n"]) [w "f", .int 5, w "n"] == vals [v, .int 5] &&
      out [("n", o)] (bind "bindq" "n" v ++ [w "n"]) [w "f", .int 5, w "n"] == vals [v, .int 5] &&
      out [("n", o)] (defv "n" v ++ [w "n"]) [w "f", .int 5, w "n"] == vals [v, o]) = true := by
  native_decide

/-- WHAT A LEAF CALLEE LEFT BOUND, A CALLER THAT RECORDS UNDOES: the
    binding is above the caller's entry depth, whoever made it. Through
    two leaf frames it reaches the top level. -/
theorem a_recording_caller_undoes_a_leaf_callees_binding :
    (pairs.all fun (v, _) =>
      let g := fnv "g" ["m"] (bind "bindq" "z" v ++ [w "z"]) 9206
      let leafF := (Boru.Corespec.fixtureDefs.push "g" g).push "f" (fnv "f" ["n"] [w "g", w "n"] 9207)
      let recF := (Boru.Corespec.fixtureDefs.push "g" g).push "f"
        (fnv "f" ["n"] (defv "w" (.int 1) ++ [w "g", w "n"]) 9208)
      outWith leafF [w "f", .int 5, w "z"] == vals [v, v] &&
      outWith recF [w "f", .int 5, w "z"] == "ERROR:undefined_word") = true := by
  native_decide

/-! ## Outside a function -/

/-- NOTHING BUT A FRAME IS A BOUNDARY. A group, a loop's round, a driven
    region, a call region and a branch leave what they bound: the name is
    read after each. -/
theorem outside_a_function_nothing_is_undone :
    (pairs.all fun (v, _) =>
      outWith Boru.Corespec.fixtureDefs ([.openParen] ++ defv "y" v ++ [.closeParen, w "y"]) == vals [v] &&
      outWith Boru.Corespec.fixtureDefs (forq 1 (defv "y" v) ++ [w "y"]) == vals [v] &&
      outWith Boru.Corespec.fixtureDefs
        [w "callq", v, .list [w "def", w "y"] false, w "y"] == vals [v] &&
      outWith Boru.Corespec.fixtureDefs
        [w "ifq", .list [.bool true] false, .list (defv "y" v) false, .list [] false, w "y"] == vals [v]) = true := by
  native_decide

end Boru.Core.Locals
