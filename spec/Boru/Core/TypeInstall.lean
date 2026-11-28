/-
  Boru.Core.TypeInstall — a type a program defines.

  A name that begins with a capital, bound to a type body, is a TYPE
  (core/go/core_type.go `InstallType`). What the body is decides what the
  name becomes:

    * a bare type — `Integer`, or a name already bound to a type — makes
      an ALIAS: the name ADOPTS that node. No new type exists; the name
      is the other type, everywhere;
    * a UNION mints a new node under `Type/Disjunct`, whose members are
      the values its alternatives admit;
    * a LITERAL — a number, a string, `none`, a list, a map — or a typed
      container mints a new node under the node the body itself sits at
      (`1` under Integer, `[: Integer ]` under List), whose members are
      the values that unify with the body. A literal is a SINGLETON type.

  A minted node is a bare type like a builtin: it has no payload, it sits
  in the lattice, and it carries the content that decides its members.
  Nothing is ever TAGGED with it — a member keeps its own type — so
  membership is never a walk up the lattice: it is the node's own rule
  (`memberMatch`, `unifyNamed` in `Boru.Core.Unify`).

  The installer's other arms — a class, a schema, a surface, a declared
  predicate, a refinement, a negation, a bounded scalar, a function
  shape — are not here: nothing the core's corpus can write reaches them.
-/
import Boru.Core.Match

namespace Boru.Core
namespace TypeInstall

/-- A refusal: the error's code, and what it says. `unmodelled` marks an
    input outside what is specified here. -/
abbrev R := Except (String × String)

/-- A type name begins with an ASCII capital (core/go
    `IsCapitalisedName`): the first BYTE, so a capital of another script
    is not one. -/
def isCapitalised (name : String) : Bool :=
  match name.toList with
  | c :: _ => 'A' ≤ c && c ≤ 'Z'
  | [] => false

/-! ## Name parts

Every part of every type's path is RESERVED: a new type may not be named
by one (core/go `ValidateTypeNameParts`, `Registry.IsKnownPart`). The
builtin table's parts are reserved from the start; a type a program
defines reserves its own name — each part of it, where it is written
with slashes — and the reservation is never given back:
unbinding the type does not release its name (NUR418). -/

/-- The reserved parts ride in the binding table, under a key no word can
    spell. -/
def partsKey : String := " type parts"

def builtinParts : List String := (builtinDecls.flatMap fun d => d.path.splitOn "/").eraseDups

def isKnownPart (d : Defs) (part : String) : Bool :=
  builtinParts.contains part || (d.stack partsKey).contains (.str part)

def registerPart (d : Defs) (part : String) : Defs :=
  if (d.stack partsKey).contains (.str part) then d
  else (partsKey, .str part :: d.stack partsKey) :: d.filter (fun (k, _) => k != partsKey)

/-! ## What may be a type's body -/

/-- A TYPE body (core/go `IsTypeBody`): a bare node, a union, a typed
    container, a function. -/
def isTypeBody : Value → Bool
  | .typeLit _ | .named _ _ | .disjunct _ | .typedList _ _ | .typedMap _ _ | .fn _ _ _ => true
  | _ => false

/-- A LITERAL body (core/go `IsLiteralTypeBody`): a scalar, `none`, a
    list or a map — the type whose members are the values that unify with
    it. -/
def isLiteralBody : Value → Bool
  | .none | .int _ | .bigInt _ | .bigDec _ | .float _ | .str _ | .bool _ | .atom _
  | .list _ _ | .map _ _ => true
  | _ => false

/-- THE NAME CHECKS every type binding passes (core/go
    `validateTypeName`), in this order: the name begins with a capital;
    unless the name is ALREADY a type — a redefinition — it is no reserved
    part — a name may be written with slashes, and then no part of it
    is; it is no function's name; and it is no value's. -/
def validateName (d : Defs) (name : String) : R Unit :=
  if !isCapitalised name then
    throw ("type_error", "type " ++ name ++ ": type names must start with a capital letter")
  else if !d.isType name && (name.splitOn "/").any (isKnownPart d) then
    throw ("type_error", "type: a part of " ++ name ++ " conflicts with an existing type name")
  else if (d.lookup? name).isSome then
    throw ("type_error", "type " ++ name ++ ": name clash — already a registered function")
  else if (d.top? name).isSome && !d.isType name then
    throw ("type_error", "type " ++ name ++ ": name clash — already a def'd value")
  else pure ()

/-- INSTALL A TYPE (core/go `InstallType`): the body is checked, then the
    name, then the name is bound — to the node it adopts, or to one minted
    for it — and reserved.

    A redefinition STACKS: the name's older binding is beneath the new
    one, and unbinding shows it again. A function, a slot or a value that
    took the older NODE keeps it. -/
def install (d : Defs) (name : String) (body : Value) : R Defs := do
  let body := body.unquote
  -- a typed container whose child is still an expression has it run
  -- first: not specified here
  match body with
  | .typedList (.parenExpr _) _ | .typedMap (.parenExpr _) _ => throw ("unmodelled", "a child type expression")
  | _ => pure ()
  if !(isTypeBody body || isLiteralBody body) then
    throw ("type_error", "type: body must be a type value or literal")
  validateName d name
  let d ← match body with
    | .fn _ _ _ =>
      -- a function is a type only as a declared predicate
      throw ("def_error", "def " ++ name ++ ": a capitalised name declares a type, and a function is a type only as a declared predicate")
    | .typeLit _ | .named _ _ =>
      -- ALIAS: the name adopts the node
      pure (d.push name (.typeBind body false))
    | _ =>
      -- a new node, under the node the body sits at
      let (ident, d) := d.freshIdent
      pure (d.push name (.typeBind (.named (mintedPath body.typeOf name ident) body) true))
  pure ((name.splitOn "/").foldl registerPart d)

end TypeInstall
end Boru.Core
