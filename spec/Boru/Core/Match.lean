/-
  Boru.Core.Match — which signature a call takes, and where its
  arguments come from.

  THE RULE. A call binds its arguments in signature order: matching fills
  positions from the FORWARD stack — the tokens written after the word,
  in written order — up to that signature's barrier, then fills every
  remaining position from the VALUE stack in reverse, top of stack first.
  That is the whole rule, at every arity; a call form only chooses where
  the split falls.

  This file is the kernel's plan-level matcher (core/go/collect_plan.go
  `PlanMatch`, the per-candidate scan `CollectCandidateScan` in
  core/go/collect_kernel.go, and the stack-form probe `MatchSignature` in
  core/go/signature.go). All of it is PURE: it reads the tape and the
  binding store and decides; it evaluates nothing.
-/
import Boru.Core.Unify

namespace Boru.Core

/-! ## The binding store

One store holds every binding (ADR: one binding store): a name maps to a
STACK of values, the top being the active binding. A function word is a
name whose stack holds function values. -/

abbrev Defs := List (String × List Value)

namespace Defs

def stack (d : Defs) (name : String) : List Value := (d.lookup name).getD []

/-- The active binding of a name. -/
def top? (d : Defs) (name : String) : Option Value := (d.stack name).head?

def push (d : Defs) (name : String) (v : Value) : Defs :=
  if (d.lookup name).isSome then
    d.map (fun (k, s) => if k == name then (k, v :: s) else (k, s))
  else (name, [v]) :: d

/-- Drop the active binding; a name never bound is left alone. -/
def pop (d : Defs) (name : String) : Defs :=
  d.map (fun (k, s) => if k == name then (k, s.drop 1) else (k, s))

/-- The identity a REFERENCE to the name carries (core/go/registry.go
    `aggregateIdent`): the one function bound to it. A name with several
    function bindings has no identity of its own — a reference to it is
    built from all of them, and is a new function each time it is read,
    so such a word is not equal to itself: as the kernel has it, not as it
    should be (NUR391). -/
def soleIdent? (d : Defs) (name : String) : Option Nat :=
  match (d.stack name).filterMap (fun v => match v with | .fn _ _ i => some i | _ => none) with
  | [i] => some i
  | _ => none

/-- The identity counter rides in the binding table, under a name no
    program can write, so every run threads it wherever it threads the
    bindings. -/
def identKey : String := " ident"

/-- A fresh function identity, and the table that will not hand it out
    again. -/
def freshIdent (d : Defs) : Nat × Defs :=
  let n := match d.stack identKey with
    | .int k :: _ => k.toNat
    | _ => 1000000
  (n, (identKey, [.int (n + 1)]) :: d.filter (fun (k, _) => k != identKey))

/-- The identity of a reference to `name`: its one function's, or a
    fresh one. -/
def refIdent (d : Defs) (name : String) : Nat × Defs :=
  match d.soleIdent? name with
  | some i => (i, d)
  | none => d.freshIdent

/-- Insert a signature keeping the dispatch order: more arguments first,
    equal arity in definition order (core/go/signature.go
    `CompareSignatures`; the slot-specificity tie-break is not modelled). -/
def insertSig (s : Sig) : List Sig → List Sig
  | [] => [s]
  | t :: ts => if s.arity > t.arity then s :: t :: ts else t :: insertSig s ts

/-- The function a name dispatches to: the overloads of every function
    value on the name's stack, newest binding first, in dispatch order. -/
def lookup? (d : Defs) (name : String) : Option (List Sig) :=
  let fns := (d.stack name).filterMap (fun v => match v with | .fn _ sigs _ => some sigs | _ => none)
  if fns.isEmpty then none
  else some (fns.flatten.foldl (fun acc s => insertSig s acc) [])

end Defs

/-! ## Signature slots -/

namespace Sig

def ptype (s : Sig) (i : Nat) : TypeId := ((s.param? i).map Param.type).getD TAny
def isQuote (s : Sig) (i : Nat) : Bool := ((s.param? i).map Param.quote).getD false
def isTypeArg (s : Sig) (i : Nat) : Bool := ((s.param? i).map Param.typeArg).getD false
def pattern? (s : Sig) (i : Nat) : Option Value := (s.param? i).bind Param.pattern

/-- How many leading parameters THIS call may take from the forward
    stack: the declared barrier, unless the call site overrides it
    (`/s` — none, `/f` — all). -/
def forwardLimit (s : Sig) (w : WordInfo) : Nat :=
  if w.forceStack then 0 else if w.forceForward then s.arity else s.barrier

end Sig

/-- Does any overload collect from the forward stack at all. -/
def hasForwardSigs (sigs : List Sig) : Bool := sigs.any (·.barrier > 0)

/-! ## Slot admission -/

/-- A TYPE-ARG slot admits a type — a bare node or a structural type body,
    a function value among them (a function is its own shape) — and
    declines every concrete value. -/
def sigTypeMatchesAsType (v : Value) (t : TypeId) : Bool :=
  match v.unquote with
  | .typeLit n => conforms n t
  | .typedList _ _ | .typedMap _ _ | .disjunct _ | .fn _ _ _ => conforms v.typeOf t
  | _ => false

/-- Does `v` fit slot `i` of `s` by type. -/
def sigArgMatches (s : Sig) (i : Nat) (v : Value) : Bool :=
  if s.isTypeArg i then sigTypeMatchesAsType v (s.ptype i) else v.is (s.ptype i)

/-- A bare type literal is declined at a concrete-payload slot: `Integer`
    the type is not an Integer. `Any` and `Type` slots take it. -/
def rejectsTypeLiteral (v : Value) (expected : TypeId) : Bool :=
  v.isTypeLiteral && expected != TAny && expected != TType

/-! ## Patterns

A parameter may carry a structural pattern the argument must unify with.
A concrete pattern narrows the overload wherever the argument comes from;
a type-literal pattern is enforced only at stack positions. -/

/-- Would a concrete forward value at `pos` definitely fail the slot's
    pattern. -/
def forwardPatternRejects (s : Sig) (pos : Nat) (v : Value) : Bool :=
  match s.pattern? pos with
  | none => false
  | some pat =>
    if pat.isPlainMap && v.isPlainMap then false
    else if !pat.isConcrete then false
    else !unifies v pat

/-- Check every patterned slot against the value the plan put there.
    `positions` are tape indices in signature order; the first `fwd` are
    forward positions. -/
def patternsOk (tape : List Value) (defs : Defs) (s : Sig) (positions : List Nat) (fwd : Nat) : Bool :=
  (List.range s.arity).all fun idx =>
    match s.pattern? idx, positions[idx]? with
    | some pat, some p =>
      let isForward := idx < fwd
      let val := tape.getD p .none
      -- a forward word is patterned on its binding
      let val := match val with
        | .word w => if isForward then (defs.top? w.name).getD val else val
        | _ => val
      if pat.isPlainMap && val.isPlainMap then
        isForward || (match pat, val with
          | .map pe _, .map ce _ => openUnifyEntries (2 * (pat.size + val.size) + 8) pe ce
          | _, _ => true)
      else if isForward && !pat.isConcrete then true
      else unifies val pat
    | _, _ => true

/-! ## The value stack a word sees

The tape left of the pointer, back to the enclosing open paren, is the
value stack — minus what a parked forward in the same scope has already
claimed (its word, its collected arguments, its claimed stack slots). -/

/-- Index just past the nearest open paren before `i`; 0 when none. -/
def scopeStart (tape : List Value) : Nat → Nat
  | 0 => 0
  | i + 1 =>
    match tape[i]? with
    | some .openParen | some .frameOpen => i + 1
    | _ => scopeStart tape i

/-- The nearest parked forward before `i` in the same paren scope. -/
def pendingForwardIdx (tape : List Value) : Nat → Option Nat
  | 0 => none
  | i + 1 =>
    match tape[i]? with
    | some .openParen | some .frameOpen => none
    | some (.forward _ _) => some i
    | _ => pendingForwardIdx tape i

/-- Every parked forward before `i` in the same paren scope. -/
def scopeForwards (tape : List Value) : Nat → List ForwardInfo
  | 0 => []
  | i + 1 =>
    match tape[i]? with
    | some .openParen | some .frameOpen => []
    | some (.forward f _) => f :: scopeForwards tape i
    | _ => scopeForwards tape i

/-- The tape cells a parked forward has claimed. -/
def ForwardInfo.claimed (f : ForwardInfo) : List Nat :=
  let lo := f.funcIndex - f.collected - f.stackArgs
  f.funcIndex :: (List.range (f.funcIndex - lo)).map (· + lo)

/-- The value stack visible to a word at `hi`. -/
def effectiveResolved (tape : List Value) (hi : Nat) : List Value :=
  let start := scopeStart tape hi
  let excl := (scopeForwards tape hi).flatMap ForwardInfo.claimed
  (List.range hi).filterMap fun i =>
    if i < start || excl.contains i then none
    else match tape[i]? with
      | some v => if v.isForward || v.isOpenParen then none else some v
      | none => none

/-- Tape indices of the top `n` resolved cells before `i`, deepest first.
    Unlike `effectiveResolved` this does NOT subtract what a parked
    forward claimed — the kernel computes the two separately
    (core/go/engine.go `resolvedIndicesBeforeInto`). -/
def resolvedIdxBack (tape : List Value) : Nat → Nat → List Nat
  | 0, _ => []
  | _, 0 => []
  | i + 1, n + 1 =>
    match tape[i]? with
    | some .openParen | some .frameOpen => []
    | some (.forward _ _) => resolvedIdxBack tape i (n + 1)
    | _ => i :: resolvedIdxBack tape i n

def resolvedIndicesBefore (tape : List Value) (ptr n : Nat) : List Nat :=
  (resolvedIdxBack tape ptr n).reverse

/-! ## The per-candidate scan

Once per candidate signature, over THAT signature's forward limit:
classify the tokens after the word, claiming one per parameter until a
boundary. It never evaluates. -/

/-- Does the token at the scan head fill forward slot `fwd` of `s`. -/
def scanAdmits (defs : Defs) (s : Sig) (fwd : Nat) (tok : Value) : Bool :=
  let expected := s.ptype fwd
  -- the `Any` escape is for a gradual VALUE slot: a type-arg slot declared
  -- `Any` still takes only a type, and the plan must not claim a literal
  -- the arrival will decline
  let literal (v : Value) : Bool :=
    (sigArgMatches s fwd v || (expected == TAny && !s.isTypeArg fwd))
      && !(!s.isTypeArg fwd && rejectsTypeLiteral v expected)
  -- a word that is not claimed through its binding
  let bareWord (w : WordInfo) : Bool :=
    if (defs.lookup? w.name).isSome then false                    -- a function word is a boundary
    else if w.name == "true" || w.name == "false" then
      sigArgMatches s fwd (.bool true) || expected == TAny
    else match resolveTypeName? w.name with
      | some t =>
        sigArgMatches s fwd (.typeLit t)
          && !(!s.isTypeArg fwd && rejectsTypeLiteral (.typeLit t) expected)
      | none => sigArgMatches s fwd (.atom w.name) || expected == TAny
  match tok with
  | .forward _ _ | .endMark | .closeParen | .openParen | .frameOpen | .defCleanup _ | .returnCheck _ => false
  | .word w =>
    if s.isQuote fwd then conforms TAtom expected                  -- `/q`: the name is the operand
    else match defs.top? w.name with
      | some top =>
        if top.isBareTypeNode && !s.isTypeArg fwd && rejectsTypeLiteral top expected then false
        else if (sigArgMatches s fwd top && (!top.isFn || w.forceVal)) || expected == TAny then true
        else if !top.isFn then false
        else bareWord w
      | none => bareWord w
  | v => literal v

/-- Forward positions claimed by `s`, in signature order. -/
def candidateScan (tape : List Value) (defs : Defs) (s : Sig) (limit : Nat) :
    Nat → Nat → Nat → List Nat
  | 0, _, _ => []
  | fuel + 1, fwd, scanIdx =>
    if fwd ≥ limit then []
    else match tape[scanIdx]? with
      | none => []
      | some tok =>
        if scanAdmits defs s fwd tok then scanIdx :: candidateScan tape defs s limit fuel (fwd + 1) (scanIdx + 1)
        else []

/-! ## The plan -/

/-- A chosen overload: its index among the function's signatures and the
    tape index of each argument, in signature order. A position past the
    word is a forward argument still to arrive. -/
structure Plan where
  sigIdx : Nat
  positions : List Nat
  deriving Repr, BEq

/-- Try one signature at the word at `ptr`. -/
def planSig (tape : List Value) (defs : Defs) (w : WordInfo) (resolved : List Value)
    (ptr : Nat) (insideForward : Bool) (s : Sig) : Option (List Nat) :=
  let n := s.arity
  let fwdPos := candidateScan tape defs s (s.forwardLimit w) (tape.length + 1) 0 (ptr + 1)
  let fwd := fwdPos.length
  let resolvedIdx := resolvedIndicesBefore tape ptr (min resolved.length n)
  let rtop (j : Nat) : Option Value := resolved[resolved.length - 1 - j]?
  let itop (j : Nat) : Option Nat := resolvedIdx[resolvedIdx.length - 1 - j]?
  let remaining := n - fwd
  if fwd == n then
    if patternsOk tape defs s fwdPos fwd then some fwdPos else none
  else if insideForward && fwd > 0 then
    -- inside another word's pending collection: forward first, the rest
    -- only if the stack already satisfies it
    if resolved.length ≥ remaining
        && (List.range remaining).all (fun j => (rtop j).any (sigArgMatches s (fwd + j))) then
      let positions := fwdPos ++ (List.range remaining).filterMap itop
      if positions.length == n && patternsOk tape defs s positions fwd then some positions else none
    else none
  else if w.forceForward then none
  else if resolved.length < remaining then none
  else
    let ok := (List.range remaining).all fun j =>
      match rtop j with
      | some v =>
        sigArgMatches s (fwd + j) v
          && !(!s.isTypeArg (fwd + j) && rejectsTypeLiteral v (s.ptype (fwd + j)))
      | none => false
    if !ok then none
    else
      let positions := fwdPos ++ (List.range remaining).filterMap itop
      if positions.length == n && patternsOk tape defs s positions fwd then some positions else none

/-- The plan-level matcher: signatures are tried in dispatch order and
    the first match wins — except that when the next token is a WORD, an
    overload capturing a name (`/q` at slot 0) is preferred over one that
    would take a value. A zero-argument overload is the last resort. -/
def planMatch (tape : List Value) (defs : Defs) (sigs : List Sig) (w : WordInfo)
    (resolved : List Value) (ptr : Nat) (insideForward : Bool) : Option Plan :=
  let preferWordSig := (tape[ptr + 1]?).any Value.isWord
  let candidates := (List.range sigs.length).filterMap fun i =>
    match sigs[i]? with
    | some s =>
      if s.arity == 0 then none
      else if w.argCount.any (· != s.arity) then none
      else (planSig tape defs w resolved ptr insideForward s).map fun ps =>
        (Plan.mk i ps, preferWordSig && s.isQuote 0)
    | none => none
  let preferred := candidates.find? (fun (_, pref) => pref || !preferWordSig)
  match preferred, candidates.head? with
  | some (p, _), _ => some p
  | none, some (p, _) => some p
  | none, none =>
    -- the zero-argument fallback
    ((List.range sigs.length).find? fun i =>
      (sigs[i]?).any (fun s => s.arity == 0 && !(w.argCount.any (· != 0)))).map (Plan.mk · [])

/-! ## The stack-form probe

`matchStack` asks whether some overload fires over the values just below
a word and nothing else. It reads the values BOTTOM-first: the deepest of
the top `n` is slot 0. The step loop uses it to decide whether a parked
forward can be resolved; the dispatch itself is always `planMatch`. -/

def positionalMatch (s : Sig) (values : List Value) : Bool :=
  (List.range s.arity).all fun i =>
    match values[i]? with
    | none => false
    | some v =>
      let t := s.ptype i
      if s.isQuote i && v.isWord then conforms TAtom t
      else
        sigArgMatches s i v
          && !(!s.isTypeArg i && v.isBareTypeNode
                && (conforms t TMap || conforms t TList || rejectsTypeLiteral v t))

def matchStack (sigs : List Sig) (stack : List Value) (w : WordInfo) : Option Sig :=
  sigs.find? fun s =>
    let n := s.arity
    if w.argCount.any (· != n) then false
    else if stack.length < n then false
    else
      let top := stack.drop (stack.length - n)
      positionalMatch s top
        && (List.range n).all fun idx =>
          match s.pattern? idx, top[idx]? with
          | some pat, some v =>
            (match pat, v with
              | .map pe _, .map ce _ => openUnifyEntries (2 * (pat.size + v.size) + 8) pe ce
              | _, _ => unifies v pat)
          | _, _ => true

end Boru.Core
