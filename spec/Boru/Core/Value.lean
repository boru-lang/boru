/-
  Boru.Core.Value — what the kernel's tape holds.

  A boru program is a sequence of VALUES, and so is its result: the
  kernel has no separate syntax tree. A parser hands the step loop a
  `List Value`; the step loop hands back a `List Value`. Some values are
  data (numbers, strings, containers, types), some are words to dispatch,
  and a few are the markers the step loop itself reads (parens, the
  collection barrier, a parked forward).

  The kernel seals the payload set (core/go/payload.go): a value's
  payload is one of a closed list of shapes. This file is that list, for
  the part of the kernel the specification covers; `spec/COVERAGE.md`
  names what is not here yet.
-/
import Boru.Core.Lattice
import Boru.Core.Number

namespace Boru.Core

/-- A word occurrence: its name and the call-site modifiers written
    after it (`name/2s`). -/
structure WordInfo where
  name : String
  /-- `/N` — dispatch only overloads of exactly N arguments. -/
  argCount : Option Nat := none
  /-- `/s` — every argument comes from the value stack. -/
  forceStack : Bool := false
  /-- `/f` — every argument comes from the forward stack. -/
  forceForward : Bool := false
  /-- `/v` — the bound value itself, never a call. -/
  forceVal : Bool := false
  /-- `/u` — usurp: the bound function with its argument order reversed. -/
  forceUsurp : Bool := false
  deriving Repr, BEq, DecidableEq

/-- A parked forward collection: the word at `funcIndex` has chosen the
    overload `sigIdx` and waits for `expected` values to arrive from the
    forward stack. -/
structure ForwardInfo where
  funcName : String
  /-- Forward arguments still owed in total. -/
  expected : Nat
  /-- Forward arguments collected so far. -/
  collected : Nat := 0
  /-- Signature positions the plan filled from the value stack. -/
  stackArgs : Nat
  /-- Tape index of the waiting word. -/
  funcIndex : Nat
  deriving Repr, BEq, DecidableEq


/-- The RAW-CAPTURE modes of a parameter: four independent ways a slot
    takes its operand as it was WRITTEN rather than as it evaluates
    (core/go `FnSig.NoEvalArgs`, `NoEvalMapArgs`, `RawParens`,
    `FormArgs`). A slot with none of them is an ordinary one.

    * `list` — a pending LIST literal is handed over unevaluated;
    * `map` — a pending MAP literal is;
    * `paren` — a paren expression written after the word is captured as
      code, where an ordinary slot takes the value it evaluates to;
    * `form` — whatever is written after the word is captured as it
      stands: a word stays a word, bound or not, and a paren expression
      stays code. -/
structure Raw where
  list : Bool := false
  map : Bool := false
  paren : Bool := false
  form : Bool := false
  deriving BEq, Repr, Inhabited

mutual

/-- A value. -/
inductive Value where
  /-- `Scalar/Number/Integer` — a signed 64-bit integer. -/
  | int (n : Int)
  /-- `Scalar/Number/BigInteger`. -/
  | bigInt (n : Int)
  /-- `Scalar/Number/BigDecimal`. -/
  | bigDec (d : Decimal)
  /-- `Scalar/Number/Float`. -/
  | float (f : F64)
  /-- `Scalar/String`. -/
  | str (s : String)
  /-- `Scalar/Boolean`. -/
  | bool (b : Bool)
  /-- `Scalar/Atom` — a name as data. -/
  | atom (name : String)
  /-- The None VALUE: the sole inhabitant of the `None` type. -/
  | none
  /-- A bare lattice node used as a value: a type literal. Types are
      values (ADR-010). -/
  | typeLit (t : TypeId)
  /-- `Node/List`. `eval` is set on a list the PARSER wrote and nothing
      has consumed yet: such a list is a pending program. A list a word
      returned is data. -/
  | list (elems : List Value) (eval : Bool)
  /-- `Node/Map`, in insertion order. `eval` as for a list. -/
  | map (entries : List (String × Value)) (eval : Bool)
  /-- A typed list `[:T e…]`: a child constraint and retained elements. -/
  | typedList (child : Value) (elems : List Value)
  /-- A typed map `{:T k:v…}`. -/
  | typedMap (child : Value) (entries : List (String × Value))
  /-- `Type/Disjunct` — a union of alternatives. -/
  | disjunct (alts : List Value)
  /-- `Word/__PE` — a deferred paren expression held as ONE value (what a
      map value like `{a: (1 add 2)}` is). -/
  | parenExpr (items : List Value)
  /-- `Word` — a word to resolve and dispatch. -/
  | word (w : WordInfo)
  /-- `Word/__OP` — the open-paren marker. -/
  | openParen
  /-- `Word/__OP` carrying a frame — the open paren of a CALL FRAME. To
      everything that asks "is this an open paren" it is one; it is told
      apart only where a frame's own tail must find its own open. -/
  | frameOpen
       /- how many of the tokens after it are the call's UNNAMED
          arguments: resolved where the call was made, they enter the
          frame as stack data, and the step loop skips them -/
       (argSpan : Nat)
  /-- `Word/__CP` — the close-paren marker. -/
  | closeParen
  /-- `Word/__ED` — the collection barrier (`end` / `;`). -/
  | endMark
  /-- `Word/__DM` — a group dispatch modifier, `/v` or `/q`. -/
  | dispatchMod (val : Bool)
  /-- `Word/__FW` — a parked forward collection, holding the signature
      the plan chose: the collection is finished against THAT signature
      even if the word's binding changes while it waits. -/
  | forward (info : ForwardInfo) (sig : Sig)
  /-- `Type/Function` — a function value: its overloads, most specific
      first. One Function type (ADR-011). -/
  | fn (name : String) (sigs : List Sig)
       /- the function's IDENTITY: two function values are the same value
          exactly when they are the same function — one written or bound
          once — whatever their text (core/go/compare.go `sameFnIdentity`) -/
       (ident : Nat)
  /-- `Word/__IN/__DC` — a call frame's cleanup marker: the parameter
      names to unbind when the frame closes. -/
  | defCleanup (names : List String)
       /- whether the pending containers the body left are evaluated HERE,
          in the frame, before anything is unbound (core/go
          `ResidualEvalsInFrame`) -/
       (evalResidual : Bool)
       /- the frame's ENTRY SNAPSHOT: how many bindings every name held
          once the frame had bound its captures and parameters. Closing
          the frame unbinds every name down to it — what the body bound
          for itself ends with the call. `none` in a LEAF frame, whose
          body names no binding word: nothing is recorded, and nothing
          is undone -/
       (snapshot : Option (List (String × Nat)))
  /-- `Word/__RC` — a call frame's return check: the declared return
      types the frame's residual must satisfy. -/
  | returnCheck (returns : List TypeId)
       /- how many unnamed arguments the frame was entered with: that
          many values beneath the declared returns are let pass -/
       (unnamed : Nat)
       /- the declared returns' patterns, by position -/
       (patterns : List (Option Value))
  /-- `Word/__MK` — a MARK: the head of a loop's region on the tape. -/
  | mark (id : Nat)
  /-- `Word/__MV` — the MOVE that ends a loop's region. Stepped, it takes
      what the region left and either replays the body under a new mark or
      puts the loop's results in the region's place. It carries the loop:
      the whole of its state rides the tape. -/
  | move (to : Nat) (loop : Loop)
  /-- The same value, QUOTED: an occurrence the step loop treats as
      data. The kernel keeps this as a flag on the occurrence; a call
      frame sets it on a list bound to a named parameter, so the body's
      read of the parameter can never re-run it. -/
  | quoted (v : Value)

/-- WHAT A MOVE CARRIES. A LOOP ON THE TAPE (core/go `Loop`), in one of
    its two modes — or one of the two things a move does that are no
    loop.

    COUNTED: the index name, its current value, the exclusive bound and
    the step; the body replayed each round; the values the completed
    rounds left; and the depth of the index name's bindings when the loop
    began, the index included — the floor of the loop's own scope.

    CONDITIONAL: a condition region and a body region, run in turn; the
    values the completed body rounds left; and which of the two regions is
    on the tape now. -/
inductive Loop where
  | counted (index : String) (current stop step : Int) (body results : List Value) (depth : Nat)
  | cond (test body results : List Value) (inBody : Bool)
  /-- Not a loop at all: the `if` CONTINUATION. The region is a condition,
      and its last value picks the branch that takes the region's place. -/
  | branch (thenBranch elseBranch : List Value)
  /-- The plain mark and move, with no continuation: the move puts `body`
      — the tokens its mark carried — where the region stood, once. -/
  | replay (body : List Value)

/-- One parameter of a signature. -/
inductive Param where
  | mk (name : String) (type : TypeId)
       /- a structural pattern the argument must unify with -/
       (pattern : Option Value)
       /- `/q` — a word written here is captured as a name, not run -/
       (quote : Bool)
       /- a TYPE-ARG slot: admits a type, declines every concrete value -/
       (typeArg : Bool)
       /- OPTIONAL: when the function is installed, a signature without
          this parameter is added, whose body calls the function by name
          with the parameter's type's base value in its place -/
       (optional : Bool)
       /- how the slot takes its operand RAW, if it does -/
       (raw : Raw)

/-- What runs when a signature is selected. -/
inductive Impl where
  /-- A host handler, named in the registry's handler table. -/
  | native (key : String)
  /-- A boru body: tokens spliced onto the tape in a call frame. A
      CLOSURE also carries the bindings it captured where it was built;
      its frame installs them beneath its parameters. -/
  | boru (body : List Value) (captured : List (String × Value))
         /- an ANONYMOUS function — one built where it is written, with no
            name of its own (core/go `FnSig.Anonymous`). It differs from a
            named one in a single thing: how its body's residual is
            evaluated -/
         (anonymous : Bool)
         /- whether a call of it RECORDS the frame's entry state — the
            baseline a closure is measured against and the snapshot the
            close goes back to. An INSTALLED function is asked once, when
            it is made, against the bindings of that moment (`some`); a
            function value that was never installed is asked at every
            call (`none`). core/go: `buildFnBodyHandler` against
            `execFnDefSig` -/
         (frameState : Option Bool)
         /- the PATTERNS of the declared returns, by position: a return
            declared as more than a type — a union, a literal — is checked
            against the pattern as well as the type (core/go
            `FnSig.ReturnPatterns`). Empty when none is declared -/
         (returnPatterns : List (Option Value))
  /-- A usurp wrapper (`/u`): re-dispatch `orig` — the function as it was
      bound — with the collected arguments in reverse; `barrier` is the
      barrier of the signature this one wraps. -/
  | usurp (orig : Value) (barrier : Nat)

/-- One overload: parameters in signature order, declared returns, the
    forward barrier, and the implementation. -/
inductive Sig where
  | mk (params : List Param) (returns : List TypeId)
       /- how many leading parameters may be collected from the forward
          stack; the rest always come from the value stack -/
       (barrier : Nat)
       (impl : Impl)
       /- a FULL-STACK handler receives the whole resolved stack of its
          paren scope and returns its replacement -/
       (fullStack : Bool)

end

deriving instance BEq, Repr for Value, Loop, Param, Impl, Sig

instance : Inhabited Value := ⟨Value.none⟩

namespace Param
def name : Param → String | mk n _ _ _ _ _ _ => n
def type : Param → TypeId | mk _ t _ _ _ _ _ => t
def pattern : Param → Option Value | mk _ _ p _ _ _ _ => p
def quote : Param → Bool | mk _ _ _ q _ _ _ => q
def typeArg : Param → Bool | mk _ _ _ _ t _ _ => t
def optional : Param → Bool | mk _ _ _ _ _ o _ => o
def raw : Param → Raw | mk _ _ _ _ _ _ r => r
end Param

namespace Sig
def params : Sig → List Param | mk p _ _ _ _ => p
def returns : Sig → List TypeId | mk _ r _ _ _ => r
def barrier : Sig → Nat | mk _ _ b _ _ => b
def impl : Sig → Impl | mk _ _ _ i _ => i
def fullStack : Sig → Bool | mk _ _ _ _ f => f
def arity (s : Sig) : Nat := s.params.length
def param? (s : Sig) (i : Nat) : Option Param := s.params[i]?
end Sig

namespace Value

/-! ## Classification

The kernel never asks "is the payload nil": it asks a named question
(core/go/CLAUDE.md, "Payload-presence vs value-mode"). These are those
questions. -/

/-- `v` is its own lattice node — a bare type literal. -/
def isBareTypeNode : Value → Bool
  | typeLit _ => true
  | quoted v => isBareTypeNode v
  | _ => false

/-- `v` may be used as a type CONSTRAINT: a bare node other than `None`,
    which dispatch treats as a value. -/
def isTypeLiteral : Value → Bool
  | typeLit t => t != "None"
  | _ => false

def isWord : Value → Bool | word _ => true | _ => false
def isForward : Value → Bool | forward _ _ => true | _ => false
def isOpenParen : Value → Bool | openParen | frameOpen _ => true | _ => false
/-- The inert tokens an open paren carries past it: a call frame's unnamed
    arguments. -/
def openSpan : Value → Nat | frameOpen k => k | _ => 0
def isCloseParen : Value → Bool | closeParen => true | _ => false
def isEnd : Value → Bool | endMark => true | _ => false
def isFn : Value → Bool | fn _ _ _ => true | _ => false
/-- A loop's mark or move: transparent to the value stack, a boundary to
    a forward scan. -/
def isLoopToken : Value → Bool | mark _ | move _ _ => true | _ => false
def isQuoted : Value → Bool | quoted _ => true | _ => false

/-- The lattice node a value sits at — its `Parent` in the kernel. A bare
    type literal sits at the node it names. -/
def typeOf : Value → TypeId
  | int _ => "Scalar/Number/Integer"
  | bigInt _ => "Scalar/Number/BigInteger"
  | bigDec _ => "Scalar/Number/BigDecimal"
  | float _ => "Scalar/Number/Float"
  | str s => if s.isEmpty then "Scalar/String/EmptyString" else "Scalar/String/ProperString"
  | bool _ => "Scalar/Boolean"
  | atom _ => "Scalar/Atom"
  | none => "None"
  | typeLit t => t
  | list _ _ => "Node/List"
  | map _ _ => "Node/Map"
  | typedList _ _ => "Node/List"
  | typedMap _ _ => "Node/Map"
  | disjunct _ => "Type/Disjunct"
  | parenExpr _ => "Word/__PE"
  | word _ => "Word"
  | openParen => "Word/__OP"
  | frameOpen _ => "Word/__OP"
  | closeParen => "Word/__CP"
  | endMark => "Word/__ED"
  | dispatchMod _ => "Word/__DM"
  | forward _ _ => "Word/__FW"
  | fn _ _ _ => "Type/Function"
  | defCleanup _ _ _ => "Word/__IN/__DC"
  | returnCheck _ _ _ => "Word/__RC"
  | mark _ => "Word/__MK"
  | move _ _ => "Word/__MV"
  | quoted v => typeOf v

/-- The value under any quoting. -/
def unquote : Value → Value
  | quoted v => unquote v
  | v => v

/-- A marker the step loop reads structurally: it is never an operand. -/
def isStructural : Value → Bool
  | forward _ _ | defCleanup _ _ _ | returnCheck _ _ _ | mark _ | move _ _ => true
  | _ => false

/-- A pending container: one the parser wrote that nothing has evaluated. -/
def isPendingContainer : Value → Bool
  | list _ true => true
  | map _ true => true
  | _ => false

end Value

mutual
/-- A size measure: any fuel above it suffices for a structural descent. -/
def Value.size : Value → Nat
  | .list elems _ => 1 + sizeList elems
  | .map entries _ => 1 + sizeEntries entries
  | .typedList c elems => 1 + c.size + sizeList elems
  | .typedMap c entries => 1 + c.size + sizeEntries entries
  | .disjunct alts => 1 + sizeList alts
  | .parenExpr items => 1 + sizeList items
  | .quoted v => 1 + v.size
  | _ => 1
def sizeList : List Value → Nat
  | [] => 0
  | v :: vs => v.size + sizeList vs
def sizeEntries : List (String × Value) → Nat
  | [] => 0
  | (_, v) :: es => v.size + sizeEntries es
end

end Boru.Core
