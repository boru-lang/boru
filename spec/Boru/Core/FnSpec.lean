/-
  Boru.Core.FnSpec — a function's specification, read and installed.

  A function is WRITTEN as a list of triples — parameters, returns, body,
  one triple per overload — and this file is what the kernel makes of it
  (core/go `fn_def.go`, `fn_params.go`, and the install in
  `core_helpers.go`).

  THE READER. A parameter list is read left to right. A word `name:Type`
  is a named parameter, `name?:Type` an optional one, `name:Type/q` one
  that captures a written word as a name; a bare type name is an unnamed
  parameter; an Integer or a String written there is an unnamed parameter
  that admits exactly that value; `|` sets the forward barrier where it
  stands; `?` makes the parameter before it optional. A return is a type,
  or MORE than a type — a union, a literal — and then the type is what
  the lattice can say of it and the rest is a PATTERN the returned value
  must unify with.

  THE INSTALL. Binding a function by name COMPILES it: every optional
  parameter becomes extra signatures that call the function by that name
  with the parameter's type's base value in its place; each body is asked
  then, against the bindings of that moment, whether its frames record
  their entry state; and an older definition of the name whose parameter
  types it repeats is taken off first — a redefinition REPLACES, where two
  definitions of different types stack.

  What is here is what the corpus notation reaches. The reader's other
  arms — a `{name: Type}` pair written by the parser, a parenthesised
  annotation that has to be RUN, a type a program defined, a dotted
  annotation, generics — are not, and an input that needs one is answered
  `unmodelled` rather than guessed at.
-/
import Boru.Core.Engine

namespace Boru.Core
namespace FnSpec

/-- The reader refuses with a reason; `unmodelled` marks an input outside
    what is specified here. -/
abbrev R := Except String

def unmodelled : R α := throw "unmodelled"

/-- THE WORD-NAME RULE (core/go `ValidateWordName`): not empty, not all
    `$`, not all `-`; the first character one of `a-z _ - $`, the rest
    `a-z 0-9 - _ $`. A parameter's name must meet it. -/
def validName (name : String) : Bool :=
  let cs := name.toList
  let lower (c : Char) : Bool := 'a' ≤ c && c ≤ 'z'
  let rest (c : Char) : Bool := lower c || ('0' ≤ c && c ≤ '9') || c == '-' || c == '_' || c == '$'
  match cs with
  | [] => false
  | first :: more =>
    !(cs.all (· == '$')) && !(cs.all (· == '-')) &&
    (lower first || first == '_' || first == '-' || first == '$') && more.all rest

/-- The parameter and return TYPE NAMES the single-word forms accept
    (core/go `lookupTypeNameInRegistry`): a builtin node's name. -/
def typeNamed (name : String) : R TypeId :=
  match resolveTypeName? name with
  | some t => pure t
  | none => throw s!"unknown type {name}"

/-- A name has the SHAPE of a type name when every `/`-part begins with a
    capital (core/go `looksLikeTypeName`): a misspelled type is an error,
    where a string that could never be one is a literal. -/
def looksLikeTypeName (name : String) : Bool :=
  !name.isEmpty && (name.splitOn "/").all fun part =>
    match part.toList with
    | c :: _ => c.isUpper
    | [] => false

/-- A NAME BOUND TO A TYPE (core/go `LookupDefType`, `ResolveDefType`).
    Bound to a bare type, the name is that type. Bound to any other type
    body — a union, a typed container, a function — it is `Any` with NO
    pattern: the binding is accepted as a declaration and enforces
    nothing (NUR411, as the kernel has it; the same union written in
    place is enforced). Bound to a value that is no type, or not bound,
    it names nothing. -/
def boundType (d : Defs) (name : String) : Option (TypeId × Option Value) :=
  match d.top? name with
  | some (.typeLit t) => some (t, none)
  | some (.disjunct _) | some (.typedList _ _) | some (.typedMap _ _) | some (.fn _ _ _) => some (TAny, none)
  | _ => none

/-- A NAME THAT IS A TYPE BINDING (core/go `Registry.LookupTypeName`): the
    slot's type is the binding's NODE. For an alias that is the node it
    adopted; for a type the program minted it is the minted node, which
    the slot carries whole — its path as the type, and the node itself
    where a pattern would stand (`Sig.named?`). -/
def typeBound (d : Defs) (name : String) : Option (TypeId × Option Value) :=
  match d.typeNode? name with
  | some (.typeLit t) => some (t, none)
  | some (.named path body) => some (path, some (.named path body))
  | _ => none

/-- WHAT A DECLARATION RESOLVES TO (core/go `ResolveSigType`): the slot's
    type, and a pattern where the declaration says more than a type.

    A type is itself. A NAME is the type it names — a builtin, or a name
    bound to a type. A scalar is its family, and the scalar the pattern. A
    union is `Any`, and the union the pattern: no node of the lattice
    names it, so the pattern is the whole contract. A list or a map is
    its family with itself as the pattern. -/
def resolveSigType (d : Defs) : Value → R (TypeId × Option Value)
  | .typeLit t => pure (t, none)
  | v@(.named path _) => pure (path, some v)
  | .word w =>
    match typeBound d w.name with
    | some r => pure r
    | none =>
      match resolveTypeName? w.name with
      | some t => pure (t, none)
      | none =>
        match boundType d w.name with
        | some r => pure r
        -- a word is always a NAME: one that names no type is an error
        | none => throw s!"unknown type {w.name}"
  | .str s =>
    match typeBound d s with
    | some r => pure r
    | none =>
      match resolveTypeName? s with
      | some t => pure (t, none)
      | none =>
        match boundType d s with
        | some r => pure r
        | none =>
          if looksLikeTypeName s then throw s!"unknown type {s}"
          else pure (TString, some (.str s))
  | .disjunct alts => pure (TAny, some (.disjunct alts))
  | .int n => pure (TInteger, some (.int n))
  | .bool b => pure (TBoolean, some (.bool b))
  | v@(.list _ _) => pure (TList, some v)
  | v@(.typedList _ _) => pure (TList, some (resolveInert d v))
  | v@(.typedMap _ _) => pure (TMap, some (resolveInert d v))
  | .map entries ev => pure (TMap, some (.map (entries.map fun (k, v) => (k, resolveWordsDeep v)) ev))
  | .none => pure (TAny, none)
  | _ => unmodelled

/-- One element of a parameter list as it is read, with the state the
    list carries: the parameters so far, newest last, and the barrier. -/
structure Params where
  params : List Param := []
  barrier : Option Nat := none

/-- Split `name:Type` at its first colon — a colon that is not the first
    character. -/
def splitColon (s : String) : Option (String × String) :=
  match s.splitOn ":" with
  | name :: rest@(_ :: _) => if name.isEmpty then none else some (name, ":".intercalate rest)
  | _ => none

def stripSuffix (s suffix : String) : Option String :=
  if s.endsWith suffix then some (String.ofList (s.toList.take (s.length - suffix.length))) else none

/-- READ A PARAMETER LIST (core/go `ParseFnParams`). -/
def parseParams (d : Defs) : List Value → Params → R Params
  | [], acc => pure acc
  | elem :: rest, acc =>
    let add (p : Param) : R Params := parseParams d rest { acc with params := acc.params ++ [p] }
    match elem with
    | .word w =>
      if w.name == "?" then
        -- the parameter BEFORE it is optional
        let ps := match acc.params.reverse with
          | .mk n t pat q ta _ raw :: before => (Param.mk n t pat q ta true raw :: before).reverse
          | [] => []
        parseParams d rest { acc with params := ps }
      else if w.name == "|" || w.name == "__SB" then
        parseParams d rest { acc with barrier := some acc.params.length }
      else
        match splitColon w.name with
        | some (rawName, rawType) =>
          let (name, optional) := match stripSuffix rawName "?" with
            | some n => (n, true)
            | none => (rawName, false)
          let (typeName, quote) := match stripSuffix rawType "/q" with
            | some t => (t, true)
            | none => (rawType, false)
          if !validName name then throw s!"invalid parameter name {name}"
          else
            -- a builtin's name, or a name the program bound as a TYPE
            match (resolveTypeName? typeName).map (·, none) <|> typeBound d typeName with
            | some (t, node) => add (.mk name t node quote false optional {})
            | none =>
              -- `name:kw/q`, where `kw` names no type: a KEYWORD slot — it
              -- admits exactly the written word `kw`
              if quote then add (.mk name TAtom (some (.atom typeName)) true false optional {})
              else throw s!"unknown type {typeName}"
        | none =>
          -- a bare type name: an unnamed parameter
          match (resolveTypeName? w.name).map (·, none) <|> typeBound d w.name with
          | some (t, node) => add (.mk "" t node false false false {})
          | none => throw s!"unknown type {w.name}"
    | .typeLit t => add (.mk "" t none false false false {})
    | .int n => add (.mk "" TInteger (some (.int n)) false false false {})
    | .bool b => add (.mk "" TBoolean (some (.bool b)) false false false {})
    | .str s => add (.mk "" TString (some (.str s)) false false false {})
    -- a map written here is an unnamed Map parameter with the map as its
    -- pattern (the parser's own `{name: Type}` pair is another thing, and
    -- is not reached)
    | v@(.map _ _) | v@(.typedMap _ _) => do
      let (t, pat) ← resolveSigType d v
      add (.mk "" t pat false false false {})
    | .atom _ => unmodelled
    | .parenExpr _ => unmodelled
    | _ => throw "invalid parameter"

/-- THE ELEMENTS OF A LIST-FAMILY VALUE (core/go `AsList`): a list's
    elements, and a TYPED list's retained ones. Every part of a
    specification that may be a list is read through this: `[: Integer 7 ]`
    as a body is the body `7`, and as a parameter list the one parameter
    `7`. -/
def listElems? : Value → Option (List Value)
  | .list elems _ => some elems
  | .typedList _ elems => some elems
  | .quoted v => listElems? v
  | _ => none

/-- A named return — `name:Type` — is its type (core/go
    `unwrapNamedReturn`). -/
def unwrapNamedReturn : Value → R Value
  | .word w =>
    match splitColon w.name with
    | some (name, type) =>
      if validName name then pure (.word { name := type }) else throw s!"invalid return name {name}"
    | none => pure (.word w)
  | v => pure v

/-- READ THE RETURNS (core/go `ParseFnReturns`): a list of declarations,
    or one declaration standing alone. -/
def parseReturns (d : Defs) (output : Value) : R (List TypeId × List (Option Value)) :=
  let slots : List Value := (listElems? output).getD [output]
  do
    let resolved ← slots.mapM fun slot => do
      let v ← unwrapNamedReturn slot
      match v with
      | .parenExpr _ => unmodelled
      | v => resolveSigType d v
    let pats := resolved.map (·.2)
    pure (resolved.map (·.1), if pats.all Option.isNone then [] else pats)

/-- READ A SPECIFICATION (core/go `ParseFnDef`): triples of parameters,
    returns and a body, one signature each. A part that is not a list
    stands for the list of itself. -/
def parseTriples (d : Defs) : List Value → R (List Sig)
  | input :: output :: body :: rest => do
    let inputElems := (listElems? input).getD [input]
    let ps ← parseParams d inputElems {}
    let (returns, pats) ← parseReturns d output
    let bodyElems := (listElems? body).getD [body]
    let sig := Sig.mk ps.params returns (ps.barrier.getD ps.params.length)
      (.boru bodyElems [] false none pats) false
    pure (sig :: (← parseTriples d rest))
  | _ => pure []

/-! ## The install -/

/-- A TYPE'S BASE VALUE (core/go `BaseValue`): what an omitted optional
    parameter is given. A type with none cannot be omitted. -/
def baseValue (t : TypeId) : Option Value :=
  if conforms t TInteger then some (.int 0)
  else if conforms t "Scalar/Number/Float" then some (.float (F64.zero false))
  else if conforms t "Scalar/Number" then some (.int 0)
  else if conforms t TString then some (.str "")
  else if conforms t TBoolean then some (.bool false)
  else if conforms t TList then some (.list [] false)
  else if conforms t TMap then some (.map [] false)
  else if conforms t TNone then some (.typeLit TNone)
  else if conforms t TAtom then some (.atom "")
  else none

/-- Every way of choosing which of `n` things to leave out, but for
    leaving none out — in the kernel's order: the choices counted in
    binary, the first thing the lowest bit. -/
def omissions (n : Nat) : List (List Bool) :=
  (List.range (2 ^ n - 1)).map fun i =>
    (List.range n).map fun bit => ((i + 1) / 2 ^ bit) % 2 == 1

/-- EXPAND THE OPTIONAL PARAMETERS (core/go `ExpandOptionalSigs`). The
    signature stays; and for every choice of optional parameters to leave
    out there is one more, of the remaining parameters, whose body CALLS
    THE FUNCTION BY NAME: each parameter left out is its type's base
    value, each one kept is read back — by its name, or, unnamed, from
    the args list by position. A choice whose base value the parameter's
    own pattern refuses, or whose type has no base value, adds nothing. -/
def expandOptional (name : String) (s : Sig) : List Sig :=
  match s with
  | .mk params returns barrier (.boru _ captured anonymous _ rpats) full =>
    let optIdx := (params.zipIdx.filter fun (p, _) => p.optional).map (·.2)
    let extra := (omissions optIdx.length).filterMap fun choice =>
      let omitted (i : Nat) : Bool :=
        ((optIdx.zip choice).any fun (j, out) => j == i && out)
      let kept := (params.zipIdx.filter fun (_, i) => !omitted i).map (·.1)
      -- the call's arguments, in the signature's order
      let rec args : List (Param × Nat) → Nat → Option (List Value)
        | [], _ => some []
        | (p, i) :: more, present =>
          if omitted i then
            match baseValue p.type with
            | some v =>
              -- a parameter of a NAMED type is given its type's base value
              -- unasked: the type's own rule is not consulted (NUR420)
              if (match p.pattern with
                  | some (.named _ _) => true
                  | some pat => unifies v pat
                  | none => true) then
                (args more present).map (v :: ·)
              else none
            | none => none
          else
            let here : List Value :=
              if !p.name.isEmpty then [.word { name := p.name }]
              else [.openParen, .word { name := "args" }, .atom (toString present),
                    .word { name := "dot" }, .closeParen]
            (args more (present + 1)).map (here ++ ·)
      match args params.zipIdx 0 with
      | some call =>
        some (Sig.mk (kept.map fun p => .mk p.name p.type p.pattern false false false {}) returns
          (min barrier kept.length)
          (.boru (.word { name := name } :: call) captured anonymous none rpats) full)
      | none => none
    s :: extra
  | s => [s]

/-- Two definitions OVERLAP when some signature of one has the parameter
    types of some signature of the other, position by position (core/go
    `FnDefsOverlap`). Names, returns and bodies do not enter into it. -/
def overlap (a b : List Sig) : Bool :=
  a.any fun sa => b.any fun sb =>
    sa.arity == sb.arity && (sa.params.zip sb.params).all fun (pa, pb) => pa.type == pb.type

/-- INSTALL A DEFINITION (core/go `InstallDef`). A value that is not a
    function is bound. A function is COMPILED under the name: an older
    definition of the name that it overlaps is taken off — unless it is a
    host word's, which is locked and stays — its optional
    parameters are expanded, and each body is asked — now — whether its
    frames record their entry state. A host function is bound under the
    name as it is. -/
def install (d : Defs) (name : String) : Value → Defs
  | .fn _ sigs ident =>
    let isBoru (s : Sig) : Bool := match s with | .mk _ _ _ (.boru _ _ _ _ _) _ => true | _ => false
    -- a redefinition REPLACES: the entries of the name it overlaps go —
    -- but for a HOST word's, whose signatures are locked (core/go
    -- `hasLockedSig`): a function defined over a host word of the same
    -- parameter types stacks on it, and unbinding it shows the word
    let locked (oldSigs : List Sig) : Bool :=
      oldSigs.any fun s => match s with | .mk _ _ _ (.native _) _ => true | _ => false
    let kept := (d.stack name).filter fun old =>
      match old with
      | .fn _ oldSigs _ => locked oldSigs || !(overlap oldSigs sigs)
      | _ => true
    let d := if kept.length == (d.stack name).length then d
      else kept.reverse.foldl (fun acc v => acc.push name v) (d.popTo name 0)
    let compiled := (sigs.flatMap fun s => if isBoru s then expandOptional name s else [s]).map fun s =>
      match s with
      | .mk ps rs b (.boru body captured anonymous _ rpats) full =>
        Sig.mk ps rs b (.boru body captured anonymous (some (bodyNeedsFrameState d body)) rpats) full
      | s => s
    d.push name (.fn name compiled ident)
  | v => d.push name v

end FnSpec
end Boru.Core
