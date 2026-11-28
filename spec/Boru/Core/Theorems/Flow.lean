/-
  Boru.Core.Theorems.Flow — break and continue.

  A FLOW SIGNAL is not an error: a word raises it, and the run resolves
  it once the step that raised it is done. The loop that takes it is the
  NEAREST counted or conditional loop ahead on the tape.

    * BREAK ends that loop: the values its completed rounds left stand
      where it stood, the round in progress leaves nothing, and the index
      is unbound;
    * CONTINUE ends the round: what it left is dropped and the loop goes
      on to its next round, or ends, as it would have;
    * a group, a container literal, a call frame and a driven loop are no
      loops for a signal: it passes THROUGH them to the loop around them,
      and whatever it escapes is closed behind it;
    * with no loop to take it, the signal is an error.

  The theorems run the machine over the corpus fixture — `breakq` and
  `contq` raise the signals — and are discharged by `native_decide` over
  the stated domains. A body cannot test a condition with the fixture's
  words, so a round is made to break, continue or carry on by what `c` is
  bound to when the body reads it: a value, or one of the two words.
-/
import Boru.Core.Engine
import Boru.Core.Canon
import Boru.Corespec.Fixture

namespace Boru.Core.Flow
open Boru.Core

def w (name : String) : Value := .word { name }

def outWith (defs : Defs) (tape : List Value) : String :=
  match run Boru.Corespec.fixtureNatives defs tape with
  | .ok vs => canonValues vs
  | .error e => "ERROR:" ++ e.code

def out (tape : List Value) : String := outWith Boru.Corespec.fixtureDefs tape

def ints (ns : List Int) : String := canonValues (ns.map Value.int)

def forq (n : Int) (body : List Value) : List Value := [w "forq", .int n, .list body false]

def whileq (test body : List Value) : List Value :=
  [w "whileq", .list test false, .list body false]

def eachq (xs body : List Value) : List Value := [w "eachq", .list xs false, .list body false]

/-- What a round does when its body reads `c`. -/
inductive Step where
  | go | break | continue
  deriving BEq, Repr

/-- The fixture's function for a word, as a value a name can be bound to. -/
def fnOf (name : String) : Value := (Boru.Corespec.fixtureDefs.top? name).getD .none

def Step.value : Step → Value
  | .go => .int 0
  | .break => fnOf "breakq"
  | .continue => fnOf "contq"

/-- `c` bound for each round in turn — the first round's nearest the
    top, under one binding to discard — so that a body's `undef c c`
    uncovers this round's and then reads it. (The round's own binding is
    uncovered BEFORE it is read: a round that breaks or continues does not
    reach what stands after the read.) A name bound to a function calls
    it: a round whose `c` is `breakq` breaks. -/
def script (steps : List Step) : Defs :=
  (steps.reverse.foldl (fun d s => d.push "c" s.value) Boru.Corespec.fixtureDefs).push "c" .none

/-- Every script of the given length. -/
def scripts : Nat → List (List Step)
  | 0 => [[]]
  | n + 1 => (scripts n).flatMap fun s => [Step.go :: s, Step.break :: s, Step.continue :: s]

/-- The body: leave the index, act as this round's `c` says, then leave
    a 7. -/
def body : List Value := [w "i", w "undef", w "c", w "c", .int 7]

/-- What `forq n body` leaves under a script: each round that carries on
    leaves its index, the 0 it read and the 7; a round that continues
    leaves nothing; a round that breaks ends the loop and leaves nothing. -/
def expected : Nat → List Step → List Int
  | _, [] => []
  | i, .go :: rest => [(i : Int), 0, 7] ++ expected (i + 1) rest
  | i, .continue :: rest => expected (i + 1) rest
  | _, .break :: _ => []

/-- BREAK KEEPS THE COMPLETED ROUNDS, CONTINUE DROPS ITS OWN. Over every
    script of four rounds — each round carrying on, breaking or
    continuing — the loop leaves exactly what its completed rounds left. -/
theorem rounds_follow_the_script :
    ((scripts 4).all fun s => outWith (script s) (forq 4 body) == ints (expected 0 s)) = true := by
  native_decide

/-- A CONDITIONAL LOOP takes the signals the same way: under a condition
    that is always true, the loop runs until a round breaks. -/
theorem a_conditional_loop_takes_them_too :
    ((scripts 3).all fun s =>
      -- every script here is ended by a break, so the loop ends
      let s := s ++ [Step.break]
      let carried := ((s.takeWhile (· != .break)).filter (· == .go)).length
      outWith (script s) (whileq [.bool true] [w "undef", w "c", w "c", .int 7]) ==
        ints ((List.replicate carried [0, 7]).flatten)) = true := by
  native_decide

/-- THE NEAREST LOOP TAKES IT. A signal raised in an inner loop ends the
    inner loop, or its round; the outer loop runs on, and leaves its 7
    every round. -/
theorem the_nearest_loop_takes_it :
    ([0, 1, 2, 3].all fun (n : Int) => [1, 2, 3].all fun (m : Int) =>
      [w "breakq", w "contq"].all fun sig =>
        out (forq n (forq m [w "i", sig] ++ [.int 7])) == ints (List.replicate n.toNat 7)) = true := by
  native_decide

/-- What stands between a signal and the loop that takes it, in a round
    `i ‹site› 7`: nothing, a group, a container literal, a map member, a
    driven loop's region, a group a waiting word evaluates. -/
def sites (sig : Value) : List (List Value) :=
  [ [sig],
    [.openParen, sig, .closeParen],
    [.list [sig] true],
    [.map [("a", sig)] true],
    eachq [.int 1, .int 2] [sig],
    [w "bothq", .int 1, .openParen, .int 2, sig, .closeParen] ]

/-- A SIGNAL PASSES THROUGH whatever is not a counted or conditional loop:
    a break at any of the sites ends the loop with nothing kept, and a
    continue ends every round with nothing kept. -/
theorem a_signal_passes_through :
    ([w "breakq", w "contq"].all fun sig => (sites sig).all fun site =>
      [1, 2, 3].all fun (n : Int) => out (forq n ([w "i"] ++ site ++ [.int 7])) == "") = true := by
  native_decide

/-- …and through a CALL FRAME, which is closed behind it: the function's
    parameter does not stay bound over the caller's own binding of its
    name. -/
theorem an_escaped_frame_is_closed :
    ([Step.break, Step.continue].all fun s => [Value.int 50, .str "a", .bool true].all fun v =>
      let f : Value := .fn "f" [.mk [.mk "n" TAny none false false] [TAny] 1
        (.boru [w "n", if s == .break then w "breakq" else w "contq"] [] false none) false] 9001
      let defs := (Boru.Corespec.fixtureDefs.push "n" v).push "f" f
      outWith defs (forq 2 [w "f", w "i"] ++ [w "n"]) == canon v) = true := by
  native_decide

/-- BREAK UNBINDS THE INDEX, as the loop's own end does. -/
theorem break_unbinds_the_index :
    ([Value.int 50, .str "a"].all fun v =>
      outWith (Boru.Corespec.fixtureDefs.push "i" v) (forq 3 [w "breakq"] ++ [w "i"]) == canon v &&
      out (forq 3 [w "breakq"] ++ [w "i"]) == "ERROR:undefined_word") = true := by
  native_decide

/-- WITH NO LOOP TO TAKE IT, A SIGNAL IS AN ERROR — wherever it is raised:
    at the top, in a group, in a literal, in a driven loop, after a loop
    that has ended. -/
theorem no_loop_is_an_error :
    ([w "breakq", w "contq"].all fun sig =>
      ((sites sig).all fun site => out ([.int 1] ++ site ++ [.int 2]) == "ERROR:flow_error") &&
      out (forq 2 [w "i"] ++ [sig]) == "ERROR:flow_error") = true := by
  native_decide

end Boru.Core.Flow
