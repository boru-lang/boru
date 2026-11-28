/-
  Boru.Core.Builtin — the kernel's own lattice, and the questions asked
  of it.

  `builtinDecls` is generated from core/go/typetable.go; this file fixes
  it as THE table and names the nodes the step loop and the unifier
  branch on.
-/
import Boru.Core.TypeTable
import Boru.Core.Value

namespace Boru.Core

/-- The builtin lattice. -/
def builtin : TypeTable := ⟨builtinDecls⟩

/-! ## Minted nodes

A type a program DEFINES is a new node of the lattice, minted under the
node its content sits at (core/go `TypeTable.MintType`). Its path is its
parent's, then the name it was defined under, then the number that tells
it from every other node minted under that name (`mintSep` between
them). No table holds it — the path alone says where it hangs. -/

/-- The path of a node minted under `parent`. -/
def mintedPath (parent : TypeId) (name : String) (ident : Nat) : TypeId :=
  parent ++ mintSep ++ name ++ mintSep ++ toString ident

/-- The parent of a minted path; `none` for a builtin one. -/
def mintedParent? (p : TypeId) : Option TypeId :=
  match (p.splitOn mintSep).reverse with
  | _ident :: _name :: parent@(_ :: _) => some (mintSep.intercalate parent.reverse)
  | _ => none

/-- The number a minted path ends in. -/
def mintedIdent (p : TypeId) : Nat :=
  ((p.splitOn mintSep).getLastD "").toNat?.getD 0

/-- `p` and its ancestors, nearest first: through the minted nodes, then
    up the builtin table. -/
def typeChain (p : TypeId) : List TypeId := go (p.length + 1) p
where
  go : Nat → TypeId → List TypeId
    | 0, p => [p]
    | fuel + 1, p =>
      match mintedParent? p with
      | some q => p :: go fuel q
      | none => builtin.chain p

/-- `t` conforms to `pattern`: `pattern` is `Any`, `t` itself, or one of
    its ancestors. -/
def conforms (t pattern : TypeId) : Bool := pattern == "Any" || (typeChain t).contains pattern

def isSubtype (t pattern : TypeId) : Bool := t != pattern && (typeChain t).contains pattern

/-- Depth of a node: the number of its ancestors. -/
def typeDepth (p : TypeId) : Nat := (typeChain p).length - 1

/-- The lowest common ancestor of two nodes, when they share one. -/
def typeLca? (a b : TypeId) : Option TypeId :=
  let cb := typeChain b
  (typeChain a).find? (cb.contains ·)

/-- A bare name resolved against the builtin lattice (ADR-012 rule 4). -/
def resolveTypeName? (name : String) : Option TypeId := builtin.resolveName? name

abbrev TAny : TypeId := "Any"
abbrev TNone : TypeId := "None"
abbrev TNever : TypeId := "Never"
abbrev TAbsent : TypeId := "Absent"
abbrev TNode : TypeId := "Node"
abbrev TList : TypeId := "Node/List"
abbrev TMap : TypeId := "Node/Map"
abbrev TAtom : TypeId := "Scalar/Atom"
abbrev TWord : TypeId := "Word"
abbrev TType : TypeId := "Type"
abbrev TFunction : TypeId := "Type/Function"
abbrev TInteger : TypeId := "Scalar/Number/Integer"
abbrev TString : TypeId := "Scalar/String"
abbrev TBoolean : TypeId := "Scalar/Boolean"

namespace Value

/-- `v` is a TYPE: a bare lattice node or a structural type body — a
    typed container, a disjunct, a function (core/go/core_boundedtype.go
    `TypeMembership`). -/
def isType (v : Value) : Bool :=
  match v.unquote with
  | typeLit _ | named _ _ | typedList _ _ | typedMap _ _ | disjunct _ | fn _ _ _ => true
  | _ => false

/-- `v` is a value of type `t` (the kernel's `Value.Is`). By default the
    value's node conforms to `t`. `Type` is the one builtin node with a
    membership of its own: every type is a `Type`, whatever branch of the
    lattice it sits on — `Integer` the type literal is a `Type`, though
    the Integer node is a Scalar. -/
def is (v : Value) (t : TypeId) : Bool :=
  if t == TType then v.isType || conforms v.typeOf t else conforms v.typeOf t

/-- `v` carries a real, readable payload. -/
def isConcrete (v : Value) : Bool := !v.isBareTypeNode

def isListShape : Value → Bool
  | list _ _ | typedList _ _ => true
  | quoted v => isListShape v
  | _ => false

def isMapShape : Value → Bool
  | map _ _ | typedMap _ _ => true
  | quoted v => isMapShape v
  | _ => false

end Value

end Boru.Core
