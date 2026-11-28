/-
  Boru.Core.Theorems.Closure — a function keeps the frame it was built in.

  A function built while a call frame is open CAPTURES the bindings that
  frame made, and carries them: called after the frame is gone, it still
  reads them — and reads them in preference to anything bound since. A
  name bound at the top level is not captured; it is read when the body
  runs.

  The theorems run the machine over the corpus fixture: `afn n [q body ]`
  builds an anonymous one-parameter function where it stands, and `bothq`
  answers its second argument — which, being a function, is then called
  with what follows. Each theorem quantifies over every choice of
  operands and is discharged by `native_decide`.
-/
import Boru.Core.Engine
import Boru.Core.Canon
import Boru.Corespec.Fixture

namespace Boru.Core.Closure
open Boru.Core

def w (name : String) : Value := .word { name }

/-- `outer x` builds, in its own frame, the function `n ↦ body`. -/
def outerOver (body : List Value) : Value :=
  .fn "outer" [.mk [.mk "x" TAny none false false] [TAny] 1
    (.boru [w "afn", w "n", .list body false] [] false none) false] 9001

/-- The fixture, `outer`, and whatever globals a theorem binds. -/
def defsWith (body : List Value) (globals : List (String × Value)) : Defs :=
  globals.foldl (fun d (k, v) => d.push k v) (Boru.Corespec.fixtureDefs.push "outer" (outerOver body))

def out (defs : Defs) (tape : List Value) : String :=
  match run Boru.Corespec.fixtureNatives defs tape with
  | .ok vs => canonValues vs
  | .error e => "ERROR:" ++ e.code

/-- Scalar operands: what a binding holds and what a call is given. -/
def operands : List Value := [.int 1, .int 2, .str "a", .bool true, .none]

/-- The closure `outer v` made, called with `a` once `outer`'s frame is gone. -/
def callLater (v a : Value) : List Value :=
  [w "bothq", .int 0, .openParen, w "outer", v, .closeParen, a]

/-- A CLOSURE READS WHAT ITS FRAME BOUND. `outer v` returns `n ↦ x`; called
    after `outer` has returned, it answers `v` — the `x` of the frame it
    was built in, though that frame is gone. -/
theorem closure_keeps_its_frame :
    (operands.all fun v => operands.all fun a =>
      out (defsWith [w "x"] []) (callLater v a) == canon v) = true := by
  native_decide

/-- THE CAPTURE OUTRANKS A LATER BINDING. A global `x` bound when the
    closure is called does not reach its body: the captured `x` does. -/
theorem capture_outranks_global :
    (operands.all fun v => operands.all fun a => operands.all fun g =>
      out (defsWith [w "x"] [("x", g)]) (callLater v a) == canon v) = true := by
  native_decide

/-- THE PARAMETER IS THE CALL'S, not the frame's: `n ↦ n` answers its own
    argument, whatever the frame or the globals hold under that name. -/
theorem parameter_is_not_captured :
    (operands.all fun v => operands.all fun a => operands.all fun g =>
      out (defsWith [w "n"] [("n", g)]) (callLater v a) == canon a) = true := by
  native_decide

/-- A CALL LEAVES THE OUTER BINDINGS AS IT FOUND THEM. The closure's frame
    binds its capture and its parameter over any global of the same name,
    and unbinds exactly those: afterwards both names read the globals. -/
theorem call_restores_shadowed_bindings :
    (operands.all fun v => operands.all fun a => operands.all fun g =>
      out (defsWith [w "x"] [("x", g), ("n", g)]) (callLater v a ++ [.endMark, w "x", w "n"])
        == canonValues [v, g, g]) = true := by
  native_decide

/-- A NAME BOUND AT THE TOP LEVEL IS NOT CAPTURED. A function built outside
    any frame captures nothing: its body reads `x` when it runs. -/
theorem top_level_is_read_when_run :
    (operands.all fun a => operands.all fun g =>
      out (defsWith [] [("x", g)])
          [w "bothq", .int 0, .openParen, w "afn", w "n", .list [w "x"] false, .closeParen, a]
        == canon g) = true := by
  native_decide

/-- CAPTURE NESTS. `outer v` returns `n ↦ (m ↦ x)`: the inner function is
    built in the middle one's frame, long after `outer` returned, and still
    answers `outer`'s `x` — the middle function captured it for it. -/
theorem capture_nests :
    (operands.all fun v => operands.all fun a => operands.all fun b =>
      out (defsWith [w "afn", w "m", .list [w "x"] false] [])
          [w "bothq", .int 0, .openParen, w "bothq", .int 0, .openParen, w "outer", v, .closeParen, a,
           .closeParen, b]
        == canon v) = true := by
  native_decide

/-- A CLOSURE IS A NEW FUNCTION EACH TIME ONE IS BUILT: two calls of
    `outer` with the same argument return two functions, alike in text
    and distinct in identity. -/
theorem each_closure_is_its_own :
    (operands.all fun v =>
      out (defsWith [w "x"] [])
          [w "unifyq", .openParen, w "outer", v, .closeParen, .openParen, w "outer", v, .closeParen]
        == "false") = true := by
  native_decide

end Boru.Core.Closure
