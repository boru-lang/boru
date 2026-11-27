/-
  Boru.Corespec.Notation — the corpus's parser-free notation.

  core/spec rows are not boru source: the kernel has no parser, so the
  corpus writes VALUES in a tiny whitespace-separated notation each
  runner implements independently (core/spec/README.md). This is the
  specification's reading of it.
-/
import Boru.Corespec.Fixture
import Boru.Core.Canon

namespace Boru.Corespec
open Boru.Core

def fields (s : String) : List String := (s.splitOn " ").filter (· != "")

/-- The seven type names the notation spells as type literals. -/
def typeLit? : String → Option TypeId
  | "Integer" => some TInteger
  | "String" => some TString
  | "Boolean" => some TBoolean
  | "List" => some TList
  | "Map" => some TMap
  | "Any" => some TAny
  | "None" => some TNone
  | _ => none

def int? (s : String) : Option Int :=
  let cs := s.toList
  let (neg, ds) := match cs with
    | '-' :: rest => (true, rest)
    | '+' :: rest => (false, rest)
    | _ => (false, cs)
  if ds.isEmpty || !ds.all Char.isDigit then none
  else
    let n : Int := (ds.foldl (fun acc c => acc * 10 + (c.toNat - '0'.toNat)) 0 : Nat)
    let v := if neg then -n else n
    -- a signed 64-bit integer, or the token is a word
    if v < -(2 ^ 63 : Int) || v ≥ (2 ^ 63 : Int) then none else some v

/-- `name/N`, `name/s`, `name/f`, `name/Ns`, `name/Nf` and `name/v` — a
    word with call-site modifiers. A slash whose suffix is not a modifier
    (`Scalar/String`) leaves the token a plain word. -/
def modifiedWord? (tok : String) : Option WordInfo :=
  let cs := tok.toList
  let suffix := (cs.reverse.takeWhile (· != '/')).reverse
  let name := cs.take (cs.length - suffix.length - 1)
  if suffix.length == cs.length || name.isEmpty || suffix.isEmpty then none
  else
    let digits := suffix.takeWhile Char.isDigit
    let rest := suffix.drop digits.length
    let argCount : Option Nat :=
      if digits.isEmpty then none
      else some (digits.foldl (fun acc c => acc * 10 + (c.toNat - '0'.toNat)) 0)
    match rest with
    | ['v'] => if digits.isEmpty then some { name := String.ofList name, forceVal := true } else none
    | [] => some { name := String.ofList name, argCount }
    | ['s'] => some { name := String.ofList name, argCount, forceStack := true }
    | ['f'] => some { name := String.ofList name, argCount, forceForward := true }
    | _ => none

/-- One plain token. -/
def token (tok : String) : Value :=
  let cs := tok.toList
  if cs.length ≥ 2 && cs.head? == some '\'' && cs.getLast? == some '\'' then
    .str (String.ofList ((cs.drop 1).take (cs.length - 2)))
  else match int? tok with
    | some n => .int n
    | none =>
      match typeLit? tok with
      | some t => .typeLit t
      | none =>
        if tok == "(" then .openParen
        else if tok == ")" then .closeParen
        else if tok == ";" then .endMark
        else match modifiedWord? tok with
          | some w => .word w
          | none => .word { name := tok }

/-- A plain map is built by SETTING each key in turn: a repeated key
    keeps its first position and takes the last value. (A typed map's
    retained entries are a list and keep every one.) -/
def setEntries (entries : List (String × Value)) : List (String × Value) :=
  entries.foldl (fun acc (k, v) =>
    if acc.any (·.1 == k) then acc.map (fun (k', v') => if k' == k then (k', v) else (k', v'))
    else acc ++ [(k, v)]) []

mutual

/-- Read ONE item from the token list. -/
partial def item : List String → Except String (Value × List String)
  | [] => throw "unexpected end of expression"
  | "[" :: rest => do
    let (elems, rest) ← itemsUntil "]" rest
    pure (.list elems true, rest)
  | "[q" :: rest => do
    let (elems, rest) ← itemsUntil "]" rest
    pure (.list elems false, rest)
  | "p(" :: rest => do
    let (elems, rest) ← itemsUntil ")" rest
    pure (.parenExpr elems, rest)
  | "d(" :: rest => do
    let (elems, rest) ← itemsUntil ")" rest
    pure (.disjunct elems, rest)
  | "[:" :: rest => do
    let (child, rest) ← item rest
    let (elems, rest) ← itemsUntil "]" rest
    pure (.typedList child elems, rest)
  | "{:" :: rest => do
    let (child, rest) ← item rest
    let (entries, rest) ← entriesUntil rest
    pure (.typedMap child entries, rest)
  | "{" :: rest => do
    let (entries, rest) ← entriesUntil rest
    pure (.map (setEntries entries) true, rest)
  | "{q" :: rest => do
    let (entries, rest) ← entriesUntil rest
    pure (.map (setEntries entries) false, rest)
  | "fn(" :: pname :: ptype :: rtype :: rest => do
    match typeLit? ptype, typeLit? rtype with
    | some pt, some rt =>
      let (body, rest) ← item rest
      match body, rest with
      | .list elems _, ")" :: rest =>
        pure (.fn "__corespec_fn" [.mk [.mk pname pt none false false] [rt] 1 (.boru elems) false], rest)
      | _, _ => throw "fn( body must be a list closed by )"
    | _, _ => throw "fn( types must be corpus-known type names"
  | tok :: rest => pure (token tok, rest)

partial def itemsUntil (close : String) : List String → Except String (List Value × List String)
  | [] => throw s!"unclosed bracket, wanted {close}"
  | tok :: rest =>
    if tok == close then pure ([], rest)
    else do
      let (v, rest) ← item (tok :: rest)
      let (vs, rest) ← itemsUntil close rest
      pure (v :: vs, rest)

partial def entriesUntil : List String → Except String (List (String × Value) × List String)
  | [] => throw "unclosed {"
  | "}" :: rest => pure ([], rest)
  | key :: rest =>
    if !key.endsWith ":" then throw s!"map key {key} must end with ':'"
    else do
      let k := String.ofList (key.toList.take (key.length - 1))
      let (v, rest) ← item rest
      let (es, rest) ← entriesUntil rest
      pure ((k, v) :: es, rest)

end

partial def assemble (toks : List String) : Except String (List Value) :=
  match toks with
  | [] => pure []
  | _ => do
    let (v, rest) ← item toks
    let vs ← assemble rest
    pure (v :: vs)

/-- Leading `def NAME <item> ;` clauses bind names before the program. -/
partial def defClauses (defs : Defs) : List String → Except String (Defs × List String)
  | "def" :: name :: rest => do
    let (v, rest) ← item rest
    match rest with
    | ";" :: rest => defClauses (defs.push name v) rest
    | _ => throw s!"def clause for {name} must end with ';'"
  | toks => pure (defs, toks)

def renderResult : Except BoruError (List Value) → String
  | .ok vs => canonValues vs
  | .error e => "ERROR:" ++ e.code

/-- Evaluate one row's expression to the string its `expected` column
    is compared with. -/
def eval (expr : String) : Except String String :=
  let (kind, arg) := match expr.splitOn " " with
    | [] => ("", "")
    | k :: rest => (k, " ".intercalate rest)
  match kind with
  | "int" => (int? arg).elim (throw s!"int {arg}") (fun n => pure (canon (.int n)))
  | "bigint" =>
    let cs := arg.toList
    let (neg, ds) := match cs with | '-' :: r => (true, r) | _ => (false, cs)
    if ds.isEmpty || !ds.all Char.isDigit then throw s!"bigint {arg}"
    else
      let n : Int := (ds.foldl (fun acc c => acc * 10 + (c.toNat - '0'.toNat)) 0 : Nat)
      pure (canon (.bigInt (if neg then -n else n)))
  | "bigdec" => (Decimal.ofString? arg).elim (throw s!"bigdec {arg}") (fun d => pure (canon (.bigDec d)))
  | "float" => (F64.ofString? arg).elim (throw s!"float {arg}") (fun f => pure (canon (.float f)))
  | "str" => pure (canon (.str arg))
  | "bool" => pure (canon (.bool (arg == "true")))
  | "none" => pure (canon .none)
  | "end" => pure (canon .endMark)
  | "closeparen" => pure (canon .closeParen)
  | "dispatchmod" => pure (canon (.dispatchMod (arg == "v")))
  | "typelit" => (typeLit? arg).elim (throw s!"typelit {arg}") (fun t => pure (canon (.typeLit t)))
  | "typedlist" =>
    match fields arg with
    | child :: elems => pure (canon (.typedList (token child) (elems.map token)))
    | [] => throw "typedlist needs a child"
  | "typedmap" =>
    match fields arg with
    | child :: entries =>
      let es := entries.map fun tok =>
        match tok.splitOn ":" with
        | k :: rest => (k, token (":".intercalate rest))
        | [] => (tok, Value.none)
      pure (canon (.typedMap (token child) es))
    | [] => throw "typedmap needs a child"
  | "list" => do
    let vs ← assemble (fields arg)
    pure (canon (.list vs false))
  | "run" => do
    let (defs, toks) ← defClauses fixtureDefs (fields arg)
    let program ← assemble toks
    pure (renderResult (run fixtureNatives defs program))
  | k => throw s!"unknown expression kind {k}"

end Boru.Corespec
