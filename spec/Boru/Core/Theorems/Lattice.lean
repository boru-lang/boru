/-
  Boru.Core.Theorems.Lattice — the type lattice is an order.

  Two kinds of statement live here, and the difference matters:

    * GENERAL theorems, proved for every table, by ordinary proof;
    * TABLE theorems, about the kernel's own builtin table. These are
      finite checks over every node, pair or triple of nodes and are
      discharged by `native_decide` — evaluated by Lean's compiler, not
      reduced in its kernel. They are as trustworthy as the compiler.
-/
import Boru.Core.Builtin

namespace Boru.Core

/-! ## General -/

/-- A node's chain starts with the node. -/
theorem TypeTable.mem_chainAux_self (tt : TypeTable) (fuel : Nat) (p : TypeId) :
    p ∈ tt.chainAux fuel p := by
  cases fuel with
  | zero => simp [TypeTable.chainAux]
  | succ n =>
    unfold TypeTable.chainAux
    cases tt.parent? p <;> simp

/-- Conformance is reflexive, in every table. -/
theorem TypeTable.conformsTo_refl (tt : TypeTable) (t : TypeId) : tt.conformsTo t t = true := by
  unfold TypeTable.conformsTo TypeTable.chain
  have h := tt.mem_chainAux_self tt.decls.length t
  simp [h]

/-- `Any` is the top, in every table: everything conforms to it. -/
theorem TypeTable.conformsTo_any (tt : TypeTable) (t : TypeId) : tt.conformsTo t "Any" = true := by
  simp [TypeTable.conformsTo]

/-- Nothing is a strict subtype of itself. -/
theorem TypeTable.isSubtypeOf_irrefl (tt : TypeTable) (t : TypeId) : tt.isSubtypeOf t t = false := by
  simp [TypeTable.isSubtypeOf]

/-! ## The builtin table -/

/-- The builtin node paths. -/
def builtinPaths : List TypeId := builtinDecls.map (·.path)

/-- Paths are unique: a path is an identity. -/
theorem builtin_paths_unique :
    (builtinPaths.all fun p => (builtinPaths.filter (· == p)).length == 1) = true := by
  native_decide

/-- Wire ids are unique. -/
theorem builtin_fixedIds_unique :
    (builtinDecls.all fun d => (builtinDecls.filter (·.fixedId == d.fixedId)).length == 1) = true := by
  native_decide

/-- Every declared parent is itself declared, and declared EARLIER — the
    table is a forest in topological order, so every chain is finite and
    the fuel-bounded walk in `chain` never truncates one. -/
theorem builtin_parents_first :
    ((List.range builtinDecls.length).all fun i =>
      match builtinDecls[i]? with
      | some d =>
        (match d.parent with
         | none => true
         | some p => ((builtinDecls.take i).any (·.path == p)))
      | none => true) = true := by
  native_decide

/-- Conformance is transitive on the builtin lattice. -/
theorem builtin_conforms_trans :
    (builtinPaths.all fun a => builtinPaths.all fun b => builtinPaths.all fun c =>
      !(conforms a b && conforms b c) || conforms a c) = true := by
  native_decide

/-- Conformance is antisymmetric on the builtin lattice: two nodes that
    conform to each other are the same node. With reflexivity and
    transitivity, conformance is a PARTIAL ORDER. -/
theorem builtin_conforms_antisymm :
    (builtinPaths.all fun a => builtinPaths.all fun b =>
      !(conforms a b && conforms b a) || a == b) = true := by
  native_decide

/-- A child always ranks strictly above its parent: the rank order
    refines the lattice order, which is what lets rank stand in for the
    lattice wherever a total order is needed. -/
theorem builtin_rank_above_parent :
    (builtinDecls.all fun d =>
      match d.parent with
      | none => true
      | some p => (match builtin.find? p with
          | some pd => pd.rank < d.rank
          | none => false)) = true := by
  native_decide

/-- Ranks are distinct: on the builtin lattice the type order needs no
    tie-break. -/
theorem builtin_ranks_unique :
    (builtinDecls.all fun d => (builtinDecls.filter (·.rank == d.rank)).length == 1) = true := by
  native_decide

/-- A non-internal leaf name names exactly one node, so bare-name
    resolution is a function. -/
theorem builtin_leaf_names_unique :
    ((builtinDecls.filter (!·.internal)).all fun d =>
      ((builtinDecls.filter (!·.internal)).filter (·.leaf == d.leaf)).length == 1) = true := by
  native_decide

/-- Bare-name resolution finds every public node by its leaf, and the
    path resolver finds every node by its full path. -/
theorem builtin_names_resolve :
    (builtinDecls.all fun d =>
      (d.internal || resolveTypeName? d.leaf == some d.path)
        && (!(d.path.splitOn "/").length ≥ 2 || builtin.byPath? d.path == some d.path)) = true := by
  native_decide

end Boru.Core
