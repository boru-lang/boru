/-
  Boru.Core.Lattice — the type lattice.

  Every boru value carries a type, and types form a tree under a single top
  (`Any`): a child conforms to each of its ancestors, never the reverse.
  The kernel calls the relation `ConformsTo` (core/go/types.go); this file
  defines it over an arbitrary declaration table, so the properties proved
  in `Boru.Core.Theorems.Lattice` hold for the builtin table and for any
  table grown by minting.

  A node is NAMED BY ITS PATH (`Scalar/Number/Integer`). The path is the
  identity here; the kernel's wire id (`FixedID`) and total-order `Rank`
  ride along on the declaration.
-/

namespace Boru.Core

/-- A lattice node's identity: its full path, e.g. `Scalar/Number/Integer`. -/
abbrev TypeId := String

/-- One declared lattice node (core/go/typetable.go `builtinDecl`). -/
structure TypeDecl where
  /-- Full path; the last segment is the leaf name. -/
  path : TypeId
  /-- The parent node, `none` for a root. Structural roots (`Scalar`,
      `Node`, `Ideal`, `Word`, `Type`) declare `Any`; `Any`, `None`,
      `Never` and `Absent` are parentless. -/
  parent : Option TypeId
  /-- Wire-stable numeric identity, baked into serialised value ids. -/
  fixedId : Nat
  /-- The unified lattice rank: the total order every cross-type
      comparison falls back to. -/
  rank : Nat
  /-- Engine-internal node (a tape marker's type): not nameable by its
      bare leaf name. -/
  internal : Bool
  /-- An extra short name the path resolver accepts for the node. -/
  alias : Option String
  deriving Repr, BEq, DecidableEq

/-- The separator of a MINTED node's path: a character no name can hold.
    A node a program defines is not in any table; its path is its
    parent's, then the name it was defined under, then the number that
    tells it from every other node of that name — each after this
    separator. The name may itself hold slashes. -/
def mintSep : String := "\x01"

/-- The name a type literal renders as: a builtin's last path segment; a
    minted node's own name, whole. -/
def leafOf (path : TypeId) : String :=
  match (path.splitOn mintSep).reverse with
  | _ident :: name :: _ :: _ => name
  | _ => (path.splitOn "/").getLastD path

def TypeDecl.leaf (d : TypeDecl) : String := leafOf d.path

/-- A lattice: the declared nodes, parents before children. -/
structure TypeTable where
  decls : List TypeDecl
  deriving Repr

namespace TypeTable

def find? (tt : TypeTable) (p : TypeId) : Option TypeDecl :=
  tt.decls.find? (·.path == p)

def has (tt : TypeTable) (p : TypeId) : Bool := (tt.find? p).isSome

def parent? (tt : TypeTable) (p : TypeId) : Option TypeId :=
  (tt.find? p).bind (·.parent)

/-- `p` followed by its ancestors, nearest first. The walk is bounded by
    the table's size, so a (malformed) cyclic table still terminates. -/
def chainAux (tt : TypeTable) : Nat → TypeId → List TypeId
  | 0, p => [p]
  | fuel + 1, p =>
    match tt.parent? p with
    | some q => p :: chainAux tt fuel q
    | none => [p]

def chain (tt : TypeTable) (p : TypeId) : List TypeId :=
  chainAux tt tt.decls.length p

/-- Depth of a node: the number of its ancestors, a root being 0. -/
def depth (tt : TypeTable) (p : TypeId) : Nat := (tt.chain p).length - 1

/-- `t` conforms to `pattern`: every value of type `t` is also a value of
    type `pattern`. Everything conforms to `Any`; otherwise `pattern` must
    be `t` itself or one of its ancestors. Directional — a partial order,
    not a symmetric predicate. -/
def conformsTo (tt : TypeTable) (t pattern : TypeId) : Bool :=
  pattern == "Any" || (tt.chain t).contains pattern

/-- Strict conformance: `t` is below `pattern` and is not `pattern`. -/
def isSubtypeOf (tt : TypeTable) (t pattern : TypeId) : Bool :=
  t != pattern && (tt.chain t).contains pattern

/-- The lowest common ancestor of two nodes, when they share one. -/
def lca? (tt : TypeTable) (a b : TypeId) : Option TypeId :=
  let cb := tt.chain b
  (tt.chain a).find? (cb.contains ·)

/-- The kernel's short specificity measure (core/go/types.go
    `Specificity`): the count of nodes on the chain up to but excluding
    `Any` — except `Any` itself, which counts 1. -/
def specificity (tt : TypeTable) (p : TypeId) : Nat :=
  if p == "Any" then 1 else ((tt.chain p).filter (· != "Any")).length

/-- Bare-name resolution, arm one: the user-facing leaf-name table. Only
    non-internal nodes are nameable by their leaf. -/
def byName? (tt : TypeTable) (name : String) : Option TypeId :=
  (tt.decls.find? (fun d => !d.internal && d.leaf == name)).map (·.path)

/-- The path a short name expands to: a leaf name that names exactly one
    node (internal ones included), or a declared alias. -/
def expandShort? (tt : TypeTable) (short : String) : Option TypeId :=
  match tt.decls.find? (fun d => d.alias == some short) with
  | some d => some d.path
  | none =>
    match tt.decls.filter (fun d => d.leaf == short) with
    | [d] => some d.path
    | _ => none

/-- A path part may not open with a lowercase letter. -/
def partOk (part : String) : Bool :=
  match part.toList with
  | c :: _ => !(c.isAlpha && !c.isUpper)
  | [] => true

/-- Bare-name resolution, arm two: a slash path. The first part may be a
    short name; the expanded path must name a declared node — an undeclared
    path never mints one. -/
def byPath? (tt : TypeTable) (name : String) : Option TypeId :=
  let parts := name.splitOn "/"
  if parts.length < 2 || !parts.all partOk then none
  else
    let head := parts.headD ""
    let parts :=
      if tt.has head then parts
      else match tt.expandShort? head with
        | some full => full.splitOn "/" ++ parts.drop 1
        | none => parts
    let full := "/".intercalate parts
    if tt.has full then some full else none

/-- The canonical cascade's builtin arm (ADR-012 rule 4,
    core/go/resolve.go `ResolveBuiltinTypeName`): the leaf-name table
    first, then the path resolver. -/
def resolveName? (tt : TypeTable) (name : String) : Option TypeId :=
  match tt.byName? name with
  | some p => some p
  | none => tt.byPath? name

end TypeTable

end Boru.Core
