/-
  Boru.Corespec.Fixture — the fixture vocabulary of core/spec.

  A bare kernel registry has no words. The corpus's runners register a
  handful — each one the smallest word that reaches one dispatch shape —
  so the step loop, matching and dispatch are exercised with no word
  library. This is the same vocabulary core/go/corespec_test.go and
  core/ts/src/corespec.test.ts declare, written a third time on purpose:
  the runners share no code, so a bug in one cannot hide in another.
-/
import Boru.Core.Engine

namespace Boru.Corespec
open Boru.Core

private def p (name : String) (type : TypeId) (pattern : Option Value := none)
    (quote : Bool := false) (typeArg : Bool := false) : Param :=
  .mk name type pattern quote typeArg

private def word (name : String) (params : List Param) (returns : List TypeId) (barrier : Nat)
    (fullStack : Bool := false) : String × Value :=
  (name, .fn name [.mk params returns barrier (.native name) fullStack])

private def asInt : Value → Int
  | .int n => n
  | _ => 0

/-- An Integer is a signed 64-bit integer: the fixture arithmetic wraps,
    as the Go runner's `int64` does. -/
private def wrap64 (n : Int) : Int :=
  let m := n % (2 ^ 64 : Int)
  if m ≥ (2 ^ 63 : Int) then m - (2 ^ 64 : Int) else m

/-- The fixture words, as bindings. -/
def fixtureDefs : Defs :=
  [ word "addq" [p "a" TInteger, p "b" TInteger] [TInteger] 2,
    word "negq" [p "a" TInteger] [TInteger] 1,
    word "pairq" [p "a" TInteger] [TInteger, TInteger] 1,
    -- STACK form: both operands come off the stack
    word "sumq" [p "a" TInteger, p "b" TInteger] [TInteger] 0,
    word "boomq" [p "a" TAny] [TAny] 1,
    -- a concrete-scalar PATTERN: matches the Integer 7 and nothing else
    word "patq" [p "n" TInteger (pattern := some (.int 7))] [TInteger] 1,
    word "dupq" [p "a" TAny] [TAny, TAny] 1,
    word "__pa" [] [] 0,
    -- `/q` over an Atom slot: the written word is captured as a name
    word "undef" [p "n" TAtom (quote := true)] [] 1,
    -- `/q` over an Any slot: the raw word already fills the slot
    word "qanyq" [p "n" TAny (quote := true)] [TAny] 1,
    word "bothq" [p "a" TAny, p "b" TAny] [TAny] 2,
    -- a TYPE-ARG slot
    word "tyq" [p "t" TAny (typeArg := true)] [TAny] 1,
    -- a TYPE-LITERAL pattern on a stack slot
    word "tpatq" [p "v" TAny (pattern := some (.typeLit TInteger))] [TInteger] 0,
    word "unifyq" [p "a" TAny, p "b" TAny] [TBoolean] 2,
    -- unifyq's VALUE half: the meet itself, or `fixture_no_meet`
    word "meetq" [p "a" TAny, p "b" TAny] [TAny] 2,
    -- the FULL-STACK fixture
    word "depthq" [] [] 0 (fullStack := true)
  ].map fun (n, v) => (n, [v])

/-- The fixture handlers. -/
def fixtureNatives : Natives :=
  [ ("addq", .plain fun a d => pure ([.int (wrap64 (asInt (a.getD 0 .none) + asInt (a.getD 1 .none)))], d)),
    ("negq", .plain fun a d => pure ([.int (wrap64 (-(asInt (a.getD 0 .none))))], d)),
    ("pairq", .plain fun a d => pure ([a.getD 0 .none, a.getD 0 .none], d)),
    ("sumq", .plain fun a d => pure ([.int (wrap64 (asInt (a.getD 0 .none) + asInt (a.getD 1 .none)))], d)),
    ("boomq", .plain fun _ _ => throw { code := "fixture_boom", detail := "the fixture word always raises" }),
    ("patq", .plain fun _ d => pure ([.int 1], d)),
    ("dupq", .plain fun a d => pure ([a.getD 0 .none, a.getD 0 .none], d)),
    ("__pa", .plain fun _ d => pure ([], d)),
    ("undef", .plain fun a d =>
      match a.getD 0 .none with
      | .atom n => pure ([], d.pop n)
      | _ => throw { code := "type_error", detail := "undef: not an atom" }),
    ("qanyq", .plain fun a d => pure ([a.getD 0 .none], d)),
    ("bothq", .plain fun a d => pure ([a.getD 1 .none], d)),
    ("tyq", .plain fun a d => pure ([a.getD 0 .none], d)),
    ("tpatq", .plain fun a d => pure ([a.getD 0 .none], d)),
    ("unifyq", .plain fun a d => pure ([.bool (unifies (a.getD 0 .none) (a.getD 1 .none))], d)),
    ("meetq", .plain fun a d =>
      match unify (a.getD 0 .none) (a.getD 1 .none) with
      | some meet => pure ([meet], d)
      | none => throw { code := "fixture_no_meet", detail := "the two values do not unify" }),
    ("depthq", .fullStack fun _ stack => pure (stack ++ [.int stack.length]))
  ]

end Boru.Corespec
