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

/-- `t` conforms to `pattern` in the builtin lattice. -/
def conforms (t pattern : TypeId) : Bool := builtin.conformsTo t pattern

def isSubtype (t pattern : TypeId) : Bool := builtin.isSubtypeOf t pattern

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

/-- `v` is a value of type `t`: the kernel's `Value.Is` under the default
    behaviour — the value's node conforms to `t`. -/
def is (v : Value) (t : TypeId) : Bool := conforms v.typeOf t

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
