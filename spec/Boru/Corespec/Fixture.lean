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
  (name, .fn name [.mk params returns barrier (.native name) fullStack] 0)

private def asInt : Value → Int
  | .int n => n
  | _ => 0

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
    -- the DRIVEN-LOOP fixture: an iteration per element, the element the
    -- region's one inert input, everything a region leaves collected
    word "eachq" [p "xs" TList, p "body" TList] [TList] 2,
    -- the COUNTED and CONDITIONAL loop fixtures: a mark, a body, and a
    -- move that replays it — on the caller's own stack
    word "forq" [p "n" TInteger, p "body" TList] [] 2,
    word "rangeq" [p "start" TInteger, p "end" TInteger, p "step" TInteger, p "body" TList] [] 4,
    word "whileq" [p "cond" TList, p "body" TList] [] 2,
    -- the `if` continuation, and the plain mark and move
    word "ifq" [p "cond" TList, p "then" TList, p "else" TList] [] 3,
    word "replayq" [p "body" TList] [] 1,
    -- the FLOW SIGNALS: break and continue
    word "breakq" [] [] 0,
    word "contq" [] [] 0,
    -- the CLOSURE fixture: build an anonymous one-parameter function at
    -- run time, where the capture analysis runs
    word "afn" [p "param" TAtom (quote := true), p "body" TList] ["Type/Function"] 2,
    -- the FULL-STACK fixture
    word "depthq" [] [] 0 (fullStack := true)
  ].zipIdx.map fun ((n, v), i) =>
    -- each fixture word is one function, with an identity of its own
    (n, [match v with
         | .fn name sigs _ => .fn name sigs (i + 1)
         | v => v])

/-- The elements a fixture reads from a List-typed argument: a plain
    list's, or the ones a typed list retains. A typed list that retains
    none is a type, not a sequence: it fills a List slot and has nothing
    to read. -/
private def elemsOf (word : String) (v : Value) : Except BoruError (List Value) :=
  match v.unquote with
  | .list xs _ => pure xs
  | .typedList _ (x :: xs) => pure (x :: xs)
  | _ => throw { code := "fixture_not_a_list", detail := word ++ ": no elements to read" }

/-- A counted loop's first round (basic's `RunForLoop`): an empty range
    runs nothing, a zero step is refused; otherwise the index is bound as
    `i` and the body stands between a mark and the move that counts. -/
private def counted (word : String) (start stop step : Int) (body : Value) (d : Defs) :
    Except BoruError (List Value × Defs) := do
  let toks ← elemsOf word body
  if step == 0 then
    throw { code := "fixture_zero_step", detail := word ++ ": the step cannot be zero" }
  else if (step > 0 && start ≥ stop) || (step < 0 && start ≤ stop) then pure ([], d)
  else
    let d := d.push "i" (.int start)
    let (id, d) := d.freshMark
    pure (.mark id :: toks ++ [.move id (.counted "i" start stop step toks [] (d.depth "i"))], d)

/-- The fixture handlers. -/
def fixtureNatives : Natives :=
  [ ("addq", .plain fun a d => pure ([.int (wrapInt64 (asInt (a.getD 0 .none) + asInt (a.getD 1 .none)))], d)),
    ("negq", .plain fun a d => pure ([.int (wrapInt64 (-(asInt (a.getD 0 .none))))], d)),
    ("pairq", .plain fun a d => pure ([a.getD 0 .none, a.getD 0 .none], d)),
    ("sumq", .plain fun a d => pure ([.int (wrapInt64 (asInt (a.getD 0 .none) + asInt (a.getD 1 .none)))], d)),
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
    ("eachq", .driven {
      next := fun a it => do
        let xs ← elemsOf "eachq" (a.getD 0 .none)
        -- a body with elements to read is its tokens; anything else is
        -- the one token it is
        let body := match elemsOf "eachq" (a.getD 1 .none) with
          | .ok toks => toks
          | .error _ => [a.getD 1 .none]
        pure ((xs[it]?).map fun x => ([x], body))
      collect := fun _ _ acc residual => pure (acc ++ residual)
      finish := fun _ acc => pure [.list acc false] }),
    ("forq", .plain fun a d => do
      counted "forq" 0 (asInt (a.getD 0 .none)) 1 (a.getD 1 .none) d),
    ("rangeq", .plain fun a d => do
      counted "rangeq" (asInt (a.getD 0 .none)) (asInt (a.getD 1 .none)) (asInt (a.getD 2 .none))
        (a.getD 3 .none) d),
    ("whileq", .plain fun a d => do
      let test ← elemsOf "whileq" (a.getD 0 .none)
      let body ← elemsOf "whileq" (a.getD 1 .none)
      let (id, d) := d.freshMark
      pure (.mark id :: test ++ [.move id (.cond test body [] false)], d)),
    ("ifq", .plain fun a d => do
      let test ← elemsOf "ifq" (a.getD 0 .none)
      let thenB ← elemsOf "ifq" (a.getD 1 .none)
      let elseB ← elemsOf "ifq" (a.getD 2 .none)
      -- each branch a paren group of its own, as basic's `if` lays it out
      let group (b : List Value) : List Value := .openParen :: b ++ [.closeParen]
      let (id, d) := d.freshMark
      pure (.mark id :: test ++ [.move id (.branch (group thenB) (group elseB))], d)),
    ("replayq", .plain fun a d => do
      let body ← elemsOf "replayq" (a.getD 0 .none)
      let (id, d) := d.freshMark
      pure (.mark id :: body ++ [.move id (.replay body)], d)),
    ("breakq", .plain fun _ d => pure ([], d.raiseFlow true)),
    ("contq", .plain fun _ d => pure ([], d.raiseFlow false)),
    ("afn", .plain fun a d =>
      match a.getD 0 .none with
      | .atom name => do
        let body ← elemsOf "afn" (a.getD 1 .none)
        -- a new function each time one is built, capturing what the
        -- enclosing frame has bound
        let (ident, d) := d.freshIdent
        let sig := Sig.mk [.mk name TAny none false false] [TAny] 1
          (.boru body (computeCaptures d [name] body)) false
        pure ([.fn "" [sig] ident], d)
      | _ => throw { code := "type_error", detail := "afn: the parameter is a name" }),
    ("depthq", .fullStack fun _ stack => pure (stack ++ [.int stack.length]))
  ]

end Boru.Corespec
