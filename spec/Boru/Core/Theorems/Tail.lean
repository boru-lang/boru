/-
  Boru.Core.Theorems.Tail — a call that is the last thing a body does.

  THE SPECIFICATION NESTS. A call made in a body opens a frame inside the
  caller's frame, whatever follows it; the caller's frame closes when its
  body is done, and its return check sees what the inner call left. There
  is no other rule for a call in tail position, so there is nothing to
  state about one except that it is a call.

  The kernel does something else with such a call — it closes the
  caller's frame first and lets the callee's take its place, so a chain of
  them costs no tape — and that is an economy the language guarantees
  (REFERENCE.md, "Recursion and tail calls"), not a meaning. The contract
  is that no program can tell. These theorems state the nested answers the
  kernel's elision has to reproduce: both frames' return checks, each
  call's own arguments and locals, a chain that counts down, two functions
  that call each other.

  The last theorem is where the kernel does NOT reproduce it (NUR407): a
  callee that unbinds its own parameter reads the caller's binding of the
  name. The specification's answer is the nested one — the same whether
  the call is last or not.
-/
import Boru.Core.Theorems.Locals

namespace Boru.Core.Tail
open Boru.Core
open Boru.Core.Counted (w ws outWith ints)

/-- A boru-bodied function of one parameter `n`, typed, with one typed
    return. -/
def fnT (name : String) (ptype rtype : TypeId) (body : List Value) (ident : Nat) : Value :=
  .fn name [.mk [.mk "n" ptype none false false false {}] [rtype] 1 (.boru body [] false none []) false] ident

/-- The fixture, the outer bindings — oldest first — and the functions. -/
def withFns (outer : List (String × Value)) (fns : List (String × Value)) : Defs :=
  fns.foldl (fun d (k, v) => d.push k v) (outer.foldl (fun d (k, v) => d.push k v) Boru.Corespec.fixtureDefs)

/-- `f` whose body is the call `g n` and nothing else, over a `g` with the
    given body. -/
def chain (gBody : List Value) (gRet fRet : TypeId := TAny) : Defs :=
  withFns [] [("g", fnT "g" TAny gRet gBody 9301), ("f", fnT "f" TAny fRet [w "g", w "n"] 9302)]

/-- The same `g`, called by an `f` that does something AFTER the call —
    `bothq 0 ( g n )`, which answers what the group left. -/
def nested (gBody : List Value) (gRet fRet : TypeId := TAny) : Defs :=
  withFns [] [("g", fnT "g" TAny gRet gBody 9301),
    ("f", fnT "f" TAny fRet [w "bothq", .int 0, .openParen, w "g", w "n", .closeParen] 9302)]

def bodies : List (List Value) :=
  [[w "n"], [w "addq", w "n", .int 1], [w "n", w "n"], [], [w "argsq"], [w "depthq"], [w "boomq", .int 1],
   [w "def", w "y", w "n", .endMark, w "y"], [w "negq", w "n"]]

def args : List Value := [.int 5, .int 0, .str "a", .bool true]

/-- A CALL IN TAIL POSITION IS A CALL: over every body and argument here,
    `f` whose body ends with the call answers what `f` answers when the
    same call is NOT its last act. -/
theorem a_tail_call_is_a_call :
    (bodies.all fun b => args.all fun a =>
      outWith (chain b) [.int 8, w "f", a] == outWith (nested b) [.int 8, w "f", a]) = true := by
  native_decide

/-- BOTH RETURN CHECKS HOLD. The inner call's result must satisfy the
    inner function's declared return AND the outer one's: an Integer the
    callee may return is refused by a caller that declares a String, and a
    String the caller would accept is refused by a callee that declares an
    Integer. -/
theorem both_return_checks_hold :
    (outWith (chain [w "n"] TInteger TString) [w "f", .int 5] == "ERROR:type_error" &&
      outWith (chain [w "n"] TAny TInteger) [w "f", .str "x"] == "ERROR:type_error" &&
      outWith (chain [w "n"] TInteger TAny) [w "f", .str "x"] == "ERROR:type_error" &&
      outWith (chain [w "n"] TInteger TInteger) [w "f", .int 5] == ints [5] &&
      outWith (chain [w "n"] TAny TAny) [w "f", .str "x"] == canonValues [.str "x"]) = true := by
  native_decide

/-- NOTHING OF EITHER FRAME IS LEFT: after the chain both parameters are
    unbound — the outer `n` is read — and there is no args list. -/
theorem a_tail_chain_leaves_nothing :
    (args.all fun a =>
      let d := withFns [("n", .int 9)]
        [("g", fnT "g" TAny TAny [w "n"] 9301), ("f", fnT "f" TAny TAny [w "g", w "n"] 9302)]
      outWith d [w "f", a, w "n"] == canonValues [a, .int 9] &&
      outWith d [w "f", a, w "argsq"] == "ERROR:fixture_no_args") = true := by
  native_decide

/-- The countdown: `f n` calls `f (n - 1)` last, through a branch, until
    `n` is zero, and then answers `last`. -/
def countdown (pre last : List Value) : Value :=
  fnT "f" TInteger TAny
    (pre ++ [w "ifq", .list [w "n"] false,
      .list [.openParen, w "addq", w "n", .int (-1), .closeParen, ws "f"] false,
      .list last false]) 9303

/-- A FUNCTION THAT CALLS ITSELF LAST runs the chain to its end, and the
    end's answer is the whole call's: its own arguments, its own stack,
    its own local — at every depth here, with the caller's stack beneath
    untouched and every frame's local undone. -/
theorem a_self_tail_call_runs_to_its_end :
    ([0, 1, 2, 3, 10, 40].all fun (k : Nat) =>
      outWith (withFns [] [("f", countdown [] [.int 7])]) [.int 8, w "f", .int k] == ints [8, 7] &&
      outWith (withFns [] [("f", countdown [] [w "argsq"])]) [w "f", .int k]
        == canonValues [.list [.int 0] false] &&
      outWith (withFns [] [("f", countdown [] [w "depthq"])]) [.int 8, w "f", .int k] == ints [8, 0] &&
      outWith (withFns [("y", .int 9)] [("f", countdown [w "def", w "y", w "n", .endMark] [w "y"])])
        [w "f", .int k, w "y"] == ints [0, 9]) = true := by
  native_decide

/-- TWO FUNCTIONS THAT CALL EACH OTHER LAST: `ev` and `od` hand a count
    back and forth, and the answer is the one the chain ends in. -/
theorem a_mutual_tail_call_runs_to_its_end :
    ([0, 1, 2, 3, 4, 9].all fun (k : Nat) =>
      let step (other : String) (last : Bool) : List Value :=
        [w "ifq", .list [w "n"] false,
          .list [.openParen, w "addq", w "n", .int (-1), .closeParen, ws other] false,
          .list [.bool last] false]
      let d := withFns [] [("ev", fnT "ev" TInteger TAny (step "od" true) 9304),
        ("od", fnT "od" TInteger TAny (step "ev" false) 9305)]
      outWith d [w "ev", .int k] == canonValues [.bool (k % 2 == 0)]) = true := by
  native_decide

/-- WHERE THE KERNEL DIFFERS (NUR407). A callee that unbinds its own
    parameter reads the caller's binding of that name: the caller's frame
    is open, and its parameter is what lies beneath. That is so whether
    the call is the caller's last act or not — here, `5` both ways, over
    whatever is bound outside. core/go answers the outer binding, or
    `undefined_word`, when the call is last. -/
theorem a_callee_reads_its_callers_binding_last_or_not :
    ([[], [("n", Value.int 8)]].all fun outer =>
      let g := fnT "g" TAny TAny [w "undef", w "n", w "n"] 9306
      let last := withFns outer [("g", g), ("f", fnT "f" TAny TAny [w "g", .int 7] 9307)]
      let notLast := withFns outer [("g", g),
        ("f", fnT "f" TAny TAny [w "bothq", .int 0, .openParen, w "g", .int 7, .closeParen] 9307)]
      outWith last [w "f", .int 5] == ints [5] &&
      outWith notLast [w "f", .int 5] == ints [5]) = true := by
  native_decide

end Boru.Core.Tail
