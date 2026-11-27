/-
  Boru.Core.Engine — the step loop.

  The kernel runs a program on ONE tape. A pointer walks it left to
  right; everything left of the pointer is resolved — it IS the value
  stack — and everything right of it is the forward stack. Stepping a
  token rewrites the tape in place:

    * a literal stays where it is and the pointer moves on;
    * a word is resolved: a bound value replaces it, a function word is
      matched (`planMatch`) and either runs at once over the stack or
      PARKS a forward marker and waits for its forward arguments to
      arrive — each one stepped like any other token first;
    * a paren group runs to completion and its results are re-stepped
      where the group stood;
    * `end`, a close paren, another function word, or the end of the
      program closes a parked collection.

  When the pointer runs off the end the tape is the result.

  This file is the kernel's interpreter (core/go/engine.go) for the
  mechanisms the specification covers, arm for arm. Every function takes
  `fuel` — the step budget — so the whole machine is total and reduces
  inside Lean's kernel.
-/
import Boru.Core.Match

namespace Boru.Core

/-- An error the kernel raises: a taxonomy code and a human detail. -/
structure BoruError where
  code : String
  detail : String := ""
  deriving Repr, BEq

/-- A host handler. A plain handler receives its arguments in signature
    order and the binding store; a FULL-STACK handler also receives the
    resolved stack of its paren scope and returns its replacement. -/
inductive Native where
  | plain (f : List Value → Defs → Except BoruError (List Value × Defs))
  | fullStack (f : List Value → List Value → Except BoruError (List Value))

abbrev Natives := List (String × Native)

/-- The machine state. -/
structure Engine where
  /-- The tape. -/
  tape : List Value
  /-- The pointer: the next token to step. -/
  ptr : Nat
  /-- The binding store. -/
  defs : Defs
  /-- Words that stood before a paren group which collapsed to nothing —
      the candidates to blame if one of them then fails to match. -/
  voidGroups : List String := []
  /-- This tape is a SEALED REGION: cell 0 is the region's own open
      paren, and its matching close ends the run. -/
  sealed : Bool := false
  /-- A one-shot seal: the function VALUE at this index has just had its
      forward arguments collected, so its re-step reads the stack only —
      the value twin of rewriting a word to `/s`. -/
  sealAt : Option Nat := none

/-! ## Tape edits -/

def splice (l : List α) (i count : Nat) (repl : List α) : List α :=
  l.take i ++ repl ++ l.drop (i + count)

def removeAt (l : List α) (i : Nat) : List α := splice l i 1 []
def insertAt (l : List α) (i : Nat) (v : α) : List α := splice l i 0 [v]
def setAt (l : List α) (i : Nat) (v : α) : List α := if i < l.length then splice l i 1 [v] else l

namespace Engine

def tok? (e : Engine) (i : Nat) : Option Value := e.tape[i]?

def err (code : String) (detail : String := "") : Except BoruError α := throw { code, detail }

/-- Out of step budget. -/
def outOfFuel : Except BoruError α := err "evaluation_limit" "step budget exhausted"

end Engine

/-- Rewrite a word so its re-dispatch reads the value stack only. -/
def forceStackWord (tape : List Value) (idx : Nat) : List Value :=
  match tape[idx]? with
  | some (.word w) => setAt tape idx (.word { name := w.name, argCount := w.argCount, forceStack := true })
  | _ => tape

/-- Lay collected forward arguments out for a stack-only re-dispatch: the
    `forwardArgs` cells nearest the word are reversed, so the first one
    collected ends on top — where the stack-reading matcher finds
    signature slot 0. -/
def rearrangeForForward (tape : List Value) (ptr stackArgs forwardArgs : Nat) : List Value :=
  let total := stackArgs + forwardArgs
  if total == 0 then tape
  else
    let idx := resolvedIndicesBefore tape ptr total
    if idx.length < total then tape
    else
      let fidx := idx.drop stackArgs
      let vals := (fidx.filterMap (tape[·]?)).reverse
      (fidx.zip vals).foldl (fun t (i, v) => setAt t i v) tape

/-- The matching close paren of the open at `openIdx`. -/
def findCloseAfter (tape : List Value) (openIdx : Nat) : Option Nat :=
  let rec go : List Value → Nat → Nat → Option Nat
    | [], _, _ => none
    | .openParen :: rest, i, depth => go rest (i + 1) (depth + 1)
    | .frameOpen :: rest, i, depth => go rest (i + 1) (depth + 1)
    | .closeParen :: rest, i, depth => if depth == 0 then some i else go rest (i + 1) (depth - 1)
    | _ :: rest, i, depth => go rest (i + 1) depth
  go (tape.drop (openIdx + 1)) (openIdx + 1) 0

/-- The nearest call-frame open before `i`. Frames collapse innermost
    first, so when a frame's cleanup marker is reached the nearest frame
    open is its own. -/
def frameOpenBefore (tape : List Value) : Nat → Option Nat
  | 0 => none
  | i + 1 =>
    match tape[i]? with
    | some .frameOpen => some i
    | _ => frameOpenBefore tape i

/-- The nearest open paren before `i`. -/
def openBefore (tape : List Value) : Nat → Option Nat
  | 0 => none
  | i + 1 =>
    match tape[i]? with
    | some .openParen | some .frameOpen => some i
    | _ => openBefore tape i

/-- Is a parked forward's NEXT slot a name-capturing (`/q`) one. -/
def pendingQuoteSlot (tape : List Value) (defs : Defs) (ptr : Nat) : Bool :=
  match pendingForwardIdx tape ptr with
  | some i =>
    match tape[i]? with
    | some (.forward f s) => f.collected < s.arity && s.isQuote f.collected
    | _ => false
  | none => false

/-- Is a parked forward still short of its signature's arity — still
    collecting. -/
def pendingCollecting (tape : List Value) (defs : Defs) (ptr : Nat) : Bool :=
  match pendingForwardIdx tape ptr with
  | some i =>
    match tape[i]? with
    | some (.forward f s) => f.collected < s.arity
    | _ => false
  | none => false

/-- A token no forward scan walks past. -/
def scanBoundary : Value → Bool
  | .forward _ _ | .defCleanup _ | .returnCheck _ | .endMark | .closeParen => true
  | _ => false

/-- Resolve a parked forward that will get no more forward arguments
    (`end`, a close paren, the end of the program, a value that does not
    fit): if some overload fires over the stack as it stands, re-dispatch
    the word stack-only. Failing that, when an OUTER collection is waiting
    for a value, the word and what it collected become one curry list for
    it; otherwise the word re-dispatches and fails on its own terms. -/
def curryOrStack (e : Engine) (funcIdx collected stackArgs : Nat) : Engine :=
  match e.tape[funcIdx]? with
  | some (.word w) =>
    let sigs? := e.defs.lookup? w.name
    -- the stack is read BEFORE the collected arguments are laid out
    let resolved := effectiveResolved e.tape funcIdx
    let tape :=
      match sigs? with
      | some sigs =>
        if hasForwardSigs sigs && stackArgs > 0 then rearrangeForForward e.tape funcIdx stackArgs collected
        else e.tape
      | none => e.tape
    let fires :=
      match sigs? with
      | some sigs => (matchStack sigs resolved { name := w.name, forceStack := true }).isSome
      | none => false
    if fires then { e with tape := forceStackWord tape funcIdx, ptr := funcIdx }
    else
      let start := funcIdx - collected
      if (pendingForwardIdx tape start).isSome then
        let elems := Value.word { name := w.name } :: (tape.drop start).take collected
        { e with tape := splice tape start (collected + 1) [.list elems false], ptr := start }
      else { e with tape := forceStackWord tape funcIdx, ptr := funcIdx }
  | _ => { e with ptr := funcIdx }

/-- A function word beginning its own dispatch closes the nearest parked
    forward — if that word can already fire with exactly what it has
    claimed. Returns the committed state, or `none`. -/
def commitBarrierForward (e : Engine) : Option Engine :=
  match pendingForwardIdx e.tape e.ptr with
  | none => none
  | some fwdIdx =>
    match e.tape[fwdIdx]? with
    | some (.forward f _) =>
      let claimed := f.collected + f.stackArgs
      if claimed == 0 || f.funcIndex < claimed then none
      else match e.tape[f.funcIndex]? with
        | some (.word w) =>
          match e.defs.lookup? w.name with
          | none => none
          | some sigs =>
            let resolved := ((e.tape.drop (f.funcIndex - claimed)).take claimed).filter
              (fun v => !(v.isForward || v.isOpenParen))
            if resolved.isEmpty then none
            else match matchStack sigs resolved { name := w.name, forceStack := true } with
              | some s =>
                if s.arity != resolved.length then none
                else
                  let tape := removeAt e.tape fwdIdx
                  let funcIdx := if fwdIdx < f.funcIndex then f.funcIndex - 1 else f.funcIndex
                  let tape := forceStackWord tape funcIdx
                  some { e with tape := rearrangeForForward tape funcIdx f.stackArgs f.collected, ptr := funcIdx }
              | none => none
        | _ => none
    | _ => none

/-- THE USURP WRAPPER (core/go/core_ref.go `usurpOver`): a function whose
    every signature is one of the original's with its parameters REVERSED
    and every one of them collectable forward, so `f/u a b c` is `f c b a`.
    A parameter keeps its type, pattern and `/q`; a type-arg slot is the
    original's to enforce, on the re-dispatch. The wrapper's handler
    re-dispatches the original as it was bound. -/
def usurpOver (name : String) (sigs : List Sig) (origIdent ident : Nat) : Value :=
  let orig := Value.fn name sigs origIdent
  let wrapped := sigs.map fun s =>
    Sig.mk (s.params.reverse.map fun p => Param.mk p.name p.type p.pattern p.quote false)
      s.returns s.arity (.usurp orig s.barrier) false
  .fn name (wrapped.foldl (fun acc s => Defs.insertSig s acc) []) ident

/-- Resolve the type-name words inside an inert TYPE SHAPE — a typed
    container's child, a disjunct's alternatives — where the value is
    consumed or left as a result (ADR-012 rule 4). A quoted value stays
    opaque. -/
def resolveInert : Value → Value
  | .typedList child elems => .typedList (resolveWordsDeep child) elems
  | .typedMap child entries => .typedMap (resolveWordsDeep child) entries
  | .disjunct alts => resolveWordsDeep (.disjunct alts)
  | v => v

/-- A frame's cleanup: unbind its names, newest first, and drop the
    baseline it was entered with. -/
def popNames (defs : Defs) (names : List String) : Defs :=
  (names.reverse.foldl Defs.pop defs).popBaseline

/-! ## Closures

A function BUILT while a call frame is open captures the bindings that
frame made (core/go/fn_capture.go). -/

mutual
/-- The words a body names, in order (`WalkBodyWords`): through lists,
    paren expressions and map values; a quoted value is data, and a
    function value keeps its own words to itself. -/
def bodyWords : Value → List String
  | .word w => [w.name]
  | .list elems _ => bodyWordsList elems
  | .parenExpr items => bodyWordsList items
  | .map entries _ => bodyWordsEntries entries
  | _ => []
def bodyWordsList : List Value → List String
  | [] => []
  | v :: vs => bodyWords v ++ bodyWordsList vs
def bodyWordsEntries : List (String × Value) → List String
  | [] => []
  | (_, v) :: es => bodyWords v ++ bodyWordsEntries es
end

/-- The words whose execution can bind a name in the current scope or
    construct an inner function (`frameStateWords`). -/
def frameStateWords : List String :=
  ["def", "undef", "fn", "afn", "do", "call", "eval", "var", "word", "module", "import", "export",
   "usurp", "behave"]

/-- A body that names none of them is a LEAF: its frame is entered with
    no baseline (`bodyNeedsFrameState`). -/
def bodyNeedsFrameState (body : List Value) : Bool :=
  (bodyWordsList body).any frameStateWords.contains

mutual
/-- The names a body binds for itself — `def NAME` at any depth outside a
    nested function (`CollectBodyLocalDefs`). They are the body's own
    locals, never captures. -/
def bodyLocals : List Value → List String
  | [] => []
  | .word w :: rest =>
    (match w.name, rest with
     | "def", .word n :: _ => [n.name]
     | _, _ => []) ++ bodyLocals rest
  | v :: rest => bodyLocalsIn v ++ bodyLocals rest
def bodyLocalsIn : Value → List String
  | .list elems _ => bodyLocals elems
  | .parenExpr items => bodyLocals items
  | _ => []
end

/-- THE CAPTURE RULE (`ComputeCaptures`). A word of the body is captured,
    with the value it is bound to, when it is bound and its binding was
    made INSIDE the enclosing call frame — deeper than that frame's
    baseline. The function's own parameters and locals are not; a name
    bound at the top level is not (it is read when the body runs); and
    with no baseline — at the top level, or in a leaf frame — nothing is.
    Sorted by name. -/
def computeCaptures (d : Defs) (params : List String) (body : List Value) : List (String × Value) :=
  match d.topBaseline? with
  | none => []
  | some baseline =>
    let locals := bodyLocals body
    let names := (bodyWordsList body).eraseDups.filter fun n =>
      !n.isEmpty && !params.contains n && !locals.contains n
    let captured := names.filterMap fun n =>
      match d.top? n with
      | some v =>
        let base := match baseline.lookup n with
          | some (.int k) => k.toNat
          | _ => 0
        if d.depth n > base then some (n, v) else none
      | none => none
    captured.mergeSort (fun a b => a.1 ≤ b.1)

mutual

/-- Step until the pointer leaves the tape. -/
def steps (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    if e.ptr ≥ e.tape.length then pure e
    else do
      let e ← stepToken nat fuel e
      steps nat fuel e

/-- Step the token at the pointer. -/
def stepToken (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    match e.tok? e.ptr with
    | none => pure e
    | some (.word w) => stepWord nat fuel e w
    | some (.forward _ _) => pure { e with ptr := e.ptr + 1 }
    | some .openParen | some .frameOpen => pure { e with ptr := e.ptr + 1 }
    | some .closeParen => stepCloseParen nat fuel e
    | some .endMark => stepEnd nat fuel e
    | some (.returnCheck _) => pure { e with ptr := e.ptr + 1 }
    | some (.defCleanup names) => do
      -- the frame's cleanup runs ONCE: stepped, the marker is spent and
      -- leaves the tape, so the frame's close finds nothing to run again
      -- (run twice, it unbound an outer binding a parameter had shadowed)
      let e ← evalFrameResidual nat fuel e e.ptr
      pure { e with defs := popNames e.defs names, tape := removeAt e.tape e.ptr }
    | some _ => stepLiteral nat fuel e

/-- Step a word: a bound value substitutes; a function word dispatches. -/
def stepWord (nat : Natives) : Nat → Engine → WordInfo → Except BoruError Engine
  | 0, _, _ => Engine.outOfFuel
  | fuel + 1, e, w =>
    if w.forceUsurp then
      -- `/u`: the bound FUNCTION with its argument order reversed. Only a
      -- function has a signature to reverse; the arity and `s`/`f`
      -- modifiers written beside `u` are not read.
      match e.defs.top? w.name with
      | none => Engine.err "undefined_word" w.name
      | some top =>
        if !top.isFn then Engine.err "illegal_ref" ("/u requires a function word: " ++ w.name)
        else
          -- the function as it is bound, and a wrapper that is a new
          -- function every time one is made
          let (origIdent, defs) := e.defs.refIdent w.name
          let (ident, defs) := defs.freshIdent
          let wrapper := usurpOver w.name ((e.defs.lookup? w.name).getD []) origIdent ident
          let e := { e with defs := defs }
          if w.forceVal then
            -- `/uv`: the wrapper as a value, placed and stepped over. It
            -- is NOT delivered to a word that is collecting, as a `/v`
            -- value is — as the kernel has it, not as it should be (NUR390)
            pure { e with tape := setAt e.tape e.ptr wrapper, ptr := e.ptr + 1 }
          else
            -- `/u` alone is a call: a statement boundary, as a bare
            -- function word is, and then the wrapper dispatched where
            -- it stands as any function value is
            match commitBarrierForward e with
            | some e' => pure e'
            | none =>
              if !w.name.startsWith "__" && (pendingForwardIdx e.tape e.ptr).isSome then
                Engine.err "signature_error" ("stranded forward before " ++ w.name)
              else stepLiteral nat fuel { e with tape := setAt e.tape e.ptr wrapper }
    else
    if w.forceVal then
      -- `/v`: the bound VALUE, never a call. A function binding yields
      -- the function the name dispatches to, as a value; it is collected
      -- by a waiting word like any literal, and otherwise simply placed.
      match e.defs.top? w.name with
      | none => Engine.err "undefined_word" w.name
      | some top =>
        let (v, defs) :=
          if top.isFn then (match e.defs.lookup? w.name with
            | some sigs =>
              let (ident, defs) := e.defs.refIdent w.name
              (Value.fn w.name sigs ident, defs)
            | none => (top, e.defs))
          else (top, e.defs)
        let e := { e with tape := setAt e.tape e.ptr v, defs := defs }
        let collecting := pendingCollecting e.tape e.defs e.ptr
        if collecting then stepLiteral nat fuel e else pure { e with ptr := e.ptr + 1 }
    else
    -- a parked forward that captures NAMES takes the word as it is written
    if pendingQuoteSlot e.tape e.defs e.ptr then stepLiteral nat fuel e
    else
      match e.defs.top? w.name with
      | some top =>
        if !top.isFn then stepLiteral nat fuel { e with tape := setAt e.tape e.ptr top }
        else dispatchWord nat fuel e w
      | none => dispatchWord nat fuel e w

def dispatchWord (nat : Natives) : Nat → Engine → WordInfo → Except BoruError Engine
  | 0, _, _ => Engine.outOfFuel
  | fuel + 1, e, w =>
    match e.defs.lookup? w.name with
    | none =>
      -- the word literals, then a builtin type name, else an error
      if w.name == "true" then pure { e with tape := setAt e.tape e.ptr (.bool true) }
      else if w.name == "false" then pure { e with tape := setAt e.tape e.ptr (.bool false) }
      else if w.name == "none" then pure { e with tape := setAt e.tape e.ptr .none }
      else if w.name == "null" then pure { e with tape := setAt e.tape e.ptr (.atom "null") }
      else match resolveTypeName? w.name with
        | some t => stepLiteral nat fuel { e with tape := setAt e.tape e.ptr (.typeLit t) }
        | none => Engine.err "undefined_word" w.name
    | some sigs =>
      -- THE STRICT FORWARD BARRIER: another function word closes a parked
      -- collection, which must commit with what it holds or is stranded
      match commitBarrierForward e with
      | some e' => pure e'
      | none =>
        if !w.name.startsWith "__" && (pendingForwardIdx e.tape e.ptr).isSome then
          Engine.err "signature_error" ("stranded forward before " ++ w.name)
        else do
          -- phase 1: the plan walk pre-evaluates the groups this call collects
          let e ←
            if (hasForwardSigs sigs && !w.forceStack) || w.forceForward then
              collectForward nat fuel e sigs w
            else pure e
          let resolved := effectiveResolved e.tape e.ptr
          let inside := (pendingForwardIdx e.tape e.ptr).isSome
          let plan :=
            match planMatch e.tape e.defs sigs w resolved e.ptr inside with
            | some p => some p
            | none =>
              if hasForwardSigs sigs && !w.forceStack then
                planMatch e.tape e.defs sigs { w with forceStack := true } resolved e.ptr inside
              else none
          match plan with
          | none =>
            if e.voidGroups.contains w.name then
              Engine.err "no_value_error" ("argument expression produced no value for " ++ w.name)
            else Engine.err "signature_error" ("no matching signature for " ++ w.name)
          | some p =>
            let fwdCount := (p.positions.filter (· > e.ptr)).length
            let stkCount := p.positions.length - fwdCount
            if fwdCount > 0 then
              -- park: the forward arguments are stepped, then collected
              match sigs[p.sigIdx]? with
              | some s =>
                let fwd : ForwardInfo :=
                  { funcName := w.name, expected := fwdCount, stackArgs := stkCount, funcIndex := e.ptr }
                pure { e with tape := insertAt e.tape (e.ptr + 1) (.forward fwd s), ptr := e.ptr + 2 }
              | none => Engine.err "internal_error" "plan names no signature"
            else
              match sigs[p.sigIdx]? with
              | some s => execMatch nat fuel e w.name s p.positions
              | none => Engine.err "internal_error" "plan names no signature"

/-- The phase-1 plan walk (core/go/collect_kernel.go `CollectForward`):
    over the union of the viable overloads' barriers, evaluate each paren
    group a still-viable overload would collect, pruning overloads a
    concrete forward value rules out. A function word, a boundary token,
    or a position no viable overload consumes stops the walk. -/
def collectForward (nat : Natives) : Nat → Engine → List Sig → WordInfo → Except BoruError Engine
  | 0, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, sigs, w =>
    let viable := sigs.filterMap fun s =>
      let b := s.forwardLimit w
      if b > 0 then some (s, b) else none
    collectWalk nat fuel e sigs viable 0 (e.ptr + 1)

def collectWalk (nat : Natives) :
    Nat → Engine → List Sig → List (Sig × Nat) → Nat → Nat → Except BoruError Engine
  | 0, _, _, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, sigs, viable, pos, scanIdx =>
    let maxBarrier := viable.foldl (fun m (_, b) => max m b) 0
    let consumes := viable.any (fun (_, b) => pos < b)
    let rawSlot (s : Sig) : Bool := s.isTypeArg pos
    let captures := sigs.any (fun s => s.isQuote pos || s.isTypeArg pos)
    let prunePatterns (v : Value) : List (Sig × Nat) :=
      viable.filter fun (s, b) =>
        !(pos < b && !rawSlot s && !s.isQuote pos && forwardPatternRejects s pos v)
    let pruneResolved (tok : Value) : List (Sig × Nat) :=
      match tok with
      | .word tw => (match e.defs.top? tw.name with | some top => prunePatterns top | none => viable)
      | v => prunePatterns v
    if pos ≥ maxBarrier then pure e
    else match e.tok? scanIdx with
      | none => pure e
      | some tok =>
        if scanBoundary tok then pure e
        else match tok with
          | .openParen | .frameOpen =>
            if !consumes then pure e
            else do
              let e ← evalParenGroupAt nat fuel e scanIdx
              let viable := match e.tok? scanIdx with
                | some res => (match res with
                    | .word tw => (match e.defs.top? tw.name with
                        | some top => viable.filter fun (s, b) =>
                            !(pos < b && !s.isTypeArg pos && !s.isQuote pos && forwardPatternRejects s pos top)
                        | none => viable)
                    | v => viable.filter fun (s, b) =>
                        !(pos < b && !s.isTypeArg pos && !s.isQuote pos && forwardPatternRejects s pos v))
                | none => viable
              collectWalk nat fuel e sigs viable (pos + 1) (scanIdx + 1)
          | .parenExpr items =>
            -- a paren expression becomes its marker span and is re-read
            collectWalk nat fuel
              { e with tape := splice e.tape scanIdx 1 (.openParen :: items ++ [.closeParen]) }
              sigs viable pos scanIdx
          | .word tw =>
            if !captures && (e.defs.lookup? tw.name).isSome && !tw.forceVal then pure e
            else collectWalk nat fuel e sigs (pruneResolved tok) (pos + 1) (scanIdx + 1)
          | v =>
            let viable :=
              if v.isConcrete then
                viable.filter fun (s, b) =>
                  !(pos < b && !rawSlot s
                    && ((s.ptype pos != TAny && !sigArgMatches s pos v) || forwardPatternRejects s pos v))
              else viable
            collectWalk nat fuel e sigs viable (pos + 1) (scanIdx + 1)

/-- Collapse, in place, the paren group opening at `scanIdx` — a group in
    another word's forward window. The pointer is restored afterwards: the
    group's results are left as resolved cells for that word's match. An
    unmatched open is not an error here; the walk simply runs out. -/
def evalParenGroupAt (nat : Natives) : Nat → Engine → Nat → Except BoruError Engine
  | 0, _, _ => Engine.outOfFuel
  | fuel + 1, e, scanIdx => do
    let saved := e.ptr
    let e ← groupLoop nat fuel { e with ptr := scanIdx + 1 } scanIdx 1
    pure { e with ptr := saved }

/-- The group opened at `scanIdx` ends at its own close — or, when it has
    none, where the structure around it ends: the tape, the close of the
    sealed region it stands in, or the cleanup marker of the call frame it
    stands in. In those three cases the open is UNMATCHED and is left as
    it is: a scan boundary. The enclosing structure's own close is never
    taken for the group's. -/
def groupLoop (nat : Natives) : Nat → Engine → Nat → Nat → Except BoruError Engine
  | 0, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, scanIdx, depth =>
    if depth == 0 then pure e
    else match e.tok? e.ptr with
      | none => pure e
      | some .openParen | some .frameOpen => groupLoop nat fuel { e with ptr := e.ptr + 1 } scanIdx (depth + 1)
      | some .closeParen =>
        if e.sealed && e.ptr + 1 == e.tape.length then pure e
        else do
          let e ← stepCloseParen nat fuel e
          groupLoop nat fuel e scanIdx (depth - 1)
      | some (.defCleanup _) =>
        if (frameOpenBefore e.tape e.ptr).any (· < scanIdx) then pure e
        else do
          let e ← stepToken nat fuel e
          groupLoop nat fuel e scanIdx depth
      | some _ => do
        let e ← stepToken nat fuel e
        groupLoop nat fuel e scanIdx depth

/-- Step a value that is not a word: it is collected by the nearest
    parked forward if it fits that forward's next slot, and closes the
    collection if it does not; with nothing parked it is simply resolved. -/
def stepLiteral (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    let valIdx := e.ptr
    match e.tok? valIdx with
    | none => pure e
    | some (.parenExpr items) =>
      -- a deferred paren expression becomes its marker span and is re-stepped
      pure { e with tape := splice e.tape valIdx 1 (.openParen :: items ++ [.closeParen]) }
    | some val =>
      match pendingForwardIdx e.tape e.ptr with
      | none =>
        match val with
        | .dispatchMod _ => pure { e with tape := removeAt e.tape valIdx }
        | .fn name sigs _ =>
          -- a function value at the pointer is a CALL, unless a `/v` or
          -- `/q` marker right after it says it is data
          match e.tok? (valIdx + 1) with
          | some (.dispatchMod _) =>
            pure { e with tape := removeAt e.tape (valIdx + 1), ptr := valIdx + 1, sealAt := none }
          | _ => dispatchFnValue nat fuel e name sigs
        | _ => pure { e with ptr := e.ptr + 1 }
      | some fwdIdx =>
        match e.tok? fwdIdx with
        | some (.forward f s) =>
            let next := f.collected
            let fits := sigArgMatches s next val
            -- a `/q` Atom slot takes a written word as its name
            let (val, fits) :=
              match val with
              | .word vw =>
                if !fits && s.isQuote next && conforms TAtom (s.ptype next) then (Value.atom vw.name, true)
                else (val, fits)
              | _ => (val, fits)
            if f.collected < f.expected && !fits then
              -- the value does not fit: the collection ends here. A
              -- function VALUE refused on its FIRST forward argument is
              -- sealed to the stack for its re-step, as a word is rewritten
              -- to `/s`: with nothing collected the tape is unchanged, and a
              -- full re-plan would be the same plan, refused again.
              let sealAt :=
                if f.collected == 0 then
                  (match e.tape[f.funcIndex]? with
                   | some (.fn _ _ _) => some f.funcIndex
                   | _ => e.sealAt)
                else e.sealAt
              let tape := removeAt e.tape fwdIdx
              let funcIdx := if fwdIdx < f.funcIndex then f.funcIndex - 1 else f.funcIndex
              pure (curryOrStack { e with tape := tape, sealAt := sealAt } funcIdx f.collected f.stackArgs)
            else
              -- collect: move the value to just before the waiting word
              let tape := removeAt e.tape valIdx
              let funcIdx := if valIdx < f.funcIndex then f.funcIndex - 1 else f.funcIndex
              let fwdIdx := if valIdx < fwdIdx then fwdIdx - 1 else fwdIdx
              let tape := insertAt tape funcIdx val
              let funcIdx := funcIdx + 1
              let fwdIdx := fwdIdx + 1
              let collected := f.collected + 1
              if collected ≥ f.expected then
                -- complete: drop the marker and re-dispatch stack-only
                let tape := removeAt tape fwdIdx
                let funcIdx := if fwdIdx < funcIdx then funcIdx - 1 else funcIdx
                -- a word is rewritten to `/s`; a function value is sealed
                let sealAt := match tape[funcIdx]? with
                  | some (.fn _ _ _) => some funcIdx
                  | _ => none
                let tape := forceStackWord tape funcIdx
                pure { e with tape := rearrangeForForward tape funcIdx f.stackArgs collected,
                              ptr := funcIdx, sealAt := sealAt }
              else
                let f' := { f with collected := collected, funcIndex := funcIdx }
                pure { e with tape := setAt tape fwdIdx (.forward f' s), ptr := fwdIdx + 1 }
        | _ => Engine.err "internal_error" "pending forward index is not a forward"

/-- A function VALUE at the pointer dispatches as its word would: the
    plan walk, the plan, then a parked forward or an immediate run. With no
    match, a named function that has anything to be called on is an
    `uncalled_function` error; with nothing at all around it, it is data. -/
def dispatchFnValue (nat : Natives) : Nat → Engine → String → List Sig → Except BoruError Engine
  | 0, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, name, sigs => do
    let sealed := e.sealAt == some e.ptr
    let e := { e with sealAt := none }
    let w : WordInfo := { name := name, forceStack := sealed }
    let e ←
      if hasForwardSigs sigs && !w.forceStack then collectForward nat fuel e sigs w else pure e
    let resolved := effectiveResolved e.tape e.ptr
    let inside := (pendingForwardIdx e.tape e.ptr).isSome
    let plan :=
      match planMatch e.tape e.defs sigs w resolved e.ptr inside with
      | some p => some p
      | none =>
        if hasForwardSigs sigs && !w.forceStack then
          planMatch e.tape e.defs sigs { w with forceStack := true } resolved e.ptr inside
        else none
    match plan with
    | none =>
      -- what follows, up to a boundary, could have been its arguments
      let upcoming := ((e.tape.drop (e.ptr + 1)).filter (!·.isForward)).takeWhile
        (fun v => !(v.isCloseParen || v.isEnd))
      -- an ANONYMOUS function is a value until something it can take
      -- stands beside it: unmatched, it is simply left where it is
      if !name.isEmpty && (!resolved.isEmpty || !upcoming.isEmpty) then
        Engine.err "uncalled_function" ("call to '" ++ name ++ "' matched no signature")
      else pure { e with ptr := e.ptr + 1 }
    | some p =>
      let fwdCount := (p.positions.filter (· > e.ptr)).length
      let stkCount := p.positions.length - fwdCount
      if fwdCount > 0 then
        match sigs[p.sigIdx]? with
        | some s =>
          let fwd : ForwardInfo :=
            { funcName := name, expected := fwdCount, stackArgs := stkCount, funcIndex := e.ptr }
          pure { e with tape := insertAt e.tape (e.ptr + 1) (.forward fwd s), ptr := e.ptr + 2 }
        | none => Engine.err "internal_error" "plan names no signature"
      else
        match sigs[p.sigIdx]? with
        | some s => execMatch nat fuel e name s p.positions
        | none => Engine.err "internal_error" "plan names no signature"

/-- `end` closes the nearest parked collection in its paren scope; with
    none parked it is simply dropped. -/
def stepEnd (_nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | _ + 1, e =>
    let e := { e with voidGroups := [] }
    let endIdx := e.ptr
    match pendingForwardIdx e.tape endIdx with
    | none => pure { e with tape := removeAt e.tape endIdx }
    | some fwdIdx =>
      match e.tok? fwdIdx with
      | some (.forward f _) =>
        let tape := removeAt (removeAt e.tape endIdx) fwdIdx
        let funcIdx := if fwdIdx < f.funcIndex then f.funcIndex - 1 else f.funcIndex
        pure (curryOrStack { e with tape := tape } funcIdx f.collected f.stackArgs)
      | _ => pure { e with tape := removeAt e.tape endIdx }

/-- A close paren collapses its group: collections still parked inside
    are resolved, a call frame's cleanup and return check run, both
    markers are removed, and the pointer returns to where the group
    opened — so the group's results are RE-STEPPED in the enclosing
    scope, where a waiting word may collect them. -/
def stepCloseParen (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    let closeIdx := e.ptr
    if e.sealed && closeIdx + 1 == e.tape.length then do
      -- THE REGION'S OWN CLOSE — the last token of a sealed tape. It pairs
      -- with the region's own open (cell 0), whatever stands between: an
      -- unmatched `(` among the tokens does not take it. Collections still
      -- parked are resolved, the pair is removed, and the residual is NOT
      -- re-stepped.
      let e ← regionForwards nat fuel e
      let tape := removeAt (removeAt e.tape (e.tape.length - 1)) 0
      pure { e with tape := tape, ptr := tape.length, sealed := false }
    else
    match openBefore e.tape closeIdx with
    | none => Engine.err "syntax_error" "unmatched closing parenthesis"
    | some openIdx => do
      let e ← closeForwards nat fuel e openIdx
      match findCloseAfter e.tape openIdx with
      | none => Engine.err "syntax_error" "unmatched closing parenthesis"
      | some closeIdx =>
        if e.sealed && openIdx == 0 then
          -- a close that reaches the region's open without being the
          -- region's own: a stray `)`
          Engine.err "syntax_error" "unmatched closing parenthesis"
        else
        -- an EMPTY group: remember who stood before it
        let e :=
          if closeIdx == openIdx + 1 then
            let start := scopeStart e.tape openIdx
            let names := ((e.tape.take openIdx).drop start).reverse.filterMap fun v =>
              match v with
              | .word vw => some vw.name
              | .forward f _ => some f.funcName
              | _ => none
            { e with voidGroups := e.voidGroups ++ names }
          else e
        let e ← closeFrame nat fuel e openIdx closeIdx
        match findCloseAfter e.tape openIdx with
        | none => Engine.err "syntax_error" "unmatched closing parenthesis"
        | some closeIdx =>
          let tape := removeAt (removeAt e.tape closeIdx) openIdx
          -- THE PAREN RE-STEP RULE: a group that collapses to ONE value,
          -- a function, PLACES it — the pointer steps past, and it is
          -- not called on what stands outside the group
          let park :=
            if closeIdx == openIdx + 2 then
              (match tape[openIdx]? with | some (.fn _ _ _) => 1 | _ => 0)
            else 0
          pure { e with tape := tape, ptr := openIdx + park }

/-- Resolve every collection still parked in a sealed region when its
    close is reached, stepping what each resolution leaves before it. -/
def regionForwards (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    let last := e.tape.length - 1
    match (List.range last).find? (fun i => i > 0 && (e.tape[i]?).any Value.isForward) with
    | none => pure e
    | some i =>
      match e.tok? i with
      | some (.forward f _) => do
        let tape := removeAt e.tape i
        let funcIdx := if i < f.funcIndex then f.funcIndex - 1 else f.funcIndex
        let e := curryOrStack { e with tape := tape } funcIdx f.collected f.stackArgs
        let e ← regionSteps nat fuel e
        regionForwards nat fuel e
      | _ => pure e

/-- Step until the pointer reaches the region's close (the last token). -/
def regionSteps (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    if e.ptr + 1 ≥ e.tape.length then pure e
    else match e.tok? e.ptr with
      | some .openParen | some .frameOpen => regionSteps nat fuel { e with ptr := e.ptr + 1 }
      | _ => do
        let e ← stepToken nat fuel e
        regionSteps nat fuel e

/-- Resolve every collection still parked inside the group at `openIdx`,
    stepping what each resolution leaves before the close. -/
def closeForwards (nat : Natives) : Nat → Engine → Nat → Except BoruError Engine
  | 0, _, _ => Engine.outOfFuel
  | fuel + 1, e, openIdx =>
    match findCloseAfter e.tape openIdx with
    | none => Engine.err "syntax_error" "unmatched closing parenthesis"
    | some closeIdx =>
      let inside := (List.range closeIdx).find? fun i => i > openIdx && (e.tape[i]?).any Value.isForward
      match inside with
      | none => pure e
      | some i =>
        match e.tok? i with
        | some (.forward f _) => do
          let tape := removeAt e.tape i
          let funcIdx := if i < f.funcIndex then f.funcIndex - 1 else f.funcIndex
          let e := curryOrStack { e with tape := tape } funcIdx f.collected f.stackArgs
          let e ← stepsToClose nat fuel e openIdx
          closeForwards nat fuel e openIdx
        | _ => pure e

/-- Step until the pointer reaches the group's close paren. -/
def stepsToClose (nat : Natives) : Nat → Engine → Nat → Except BoruError Engine
  | 0, _, _ => Engine.outOfFuel
  | fuel + 1, e, openIdx =>
    match findCloseAfter e.tape openIdx with
    | none => Engine.err "syntax_error" "unmatched closing parenthesis"
    | some closeIdx =>
      if e.ptr ≥ closeIdx then pure e
      else match e.tok? e.ptr with
        | some .openParen | some .frameOpen => stepsToClose nat fuel { e with ptr := e.ptr + 1 } openIdx
        | _ => do
          let e ← stepToken nat fuel e
          stepsToClose nat fuel e openIdx

/-- A call frame closing: run its cleanup marker, then its return check
    over the group's residual. -/
def closeFrame (nat : Natives) : Nat → Engine → Nat → Nat → Except BoruError Engine
  | 0, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, openIdx, closeIdx =>
    let cleanupAt := (List.range closeIdx).find? fun i =>
      i > openIdx && (match e.tape[i]? with | some (.defCleanup _) => true | _ => false)
    match cleanupAt with
    | some i =>
      match e.tok? i with
      | some (.defCleanup names) => do
        let e ← evalFrameResidual nat fuel e i
        closeFrame nat fuel { e with defs := popNames e.defs names, tape := removeAt e.tape i } openIdx (closeIdx - 1)
      | _ => pure e
    | none =>
      let checkAt := (List.range closeIdx).find? fun i =>
        i > openIdx && (match e.tape[i]? with | some (.returnCheck _) => true | _ => false)
      match checkAt with
      | some i =>
        match e.tok? i with
        | some (.returnCheck returns) =>
          let tape := removeAt e.tape i
          let results := (tape.drop (openIdx + 1)).take (closeIdx - 1 - (openIdx + 1))
          if results.length != returns.length then
            Engine.err "type_error" "return count"
          else if !((results.zip returns).all fun (v, t) => v.is t) then
            Engine.err "type_error" "return type"
          else pure { e with tape := tape }
        | _ => pure e
      | none => pure e

/-- Evaluate the pending containers a frame leaves, while the frame's
    bindings are still live. -/
def evalFrameResidual (nat : Natives) : Nat → Engine → Nat → Except BoruError Engine
  | 0, _, _ => Engine.outOfFuel
  | fuel + 1, e, markerIdx =>
    let lo := scopeStart e.tape markerIdx
    sweepRange nat fuel e lo markerIdx

/-- Evaluate every pending container in `[i, hi)`. -/
def sweepRange (nat : Natives) : Nat → Engine → Nat → Nat → Except BoruError Engine
  | 0, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, i, hi =>
    if i ≥ hi then pure e
    else match e.tok? i with
      | some v =>
        if v.isPendingContainer then do
          let (v', defs) ← autoEval nat fuel e.defs v
          sweepRange nat fuel { e with tape := setAt e.tape i v', defs := defs } (i + 1) hi
        else sweepRange nat fuel { e with tape := setAt e.tape i (resolveInert v) } (i + 1) hi
      | none => pure e

/-- Run a matched signature: evaluate its pending container arguments,
    call the handler, and splice the results where the arguments and the
    word stood. The pointer is left ON the first result, so results are
    re-stepped — which is how a result reaches an outer waiting word. -/
def execMatch (nat : Natives) :
    Nat → Engine → String → Sig → List Nat → Except BoruError Engine
  | 0, _, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, name, s, positions => do
    let n := s.arity
    let indices := if positions.isEmpty && n > 0 then resolvedIndicesBefore e.tape e.ptr n else positions
    let rawArgs := indices.filterMap (e.tape[·]?)
    -- a pending container argument is a program: run it first
    let (args, defs) ← evalArgs nat fuel e.defs rawArgs
    let e := { e with defs := defs }
    if s.fullStack then
      let base := scopeStart e.tape e.ptr
      let stack := (List.range e.ptr).filterMap fun i =>
        if i < base || indices.contains i then none
        else match e.tape[i]? with
          | some v => if v.isForward || v.isOpenParen then none else some v
          | none => none
      match s.impl with
      | .native key =>
        match nat.lookup key with
        | some (.fullStack f) => do
          let results ← f args stack
          pure { e with tape := splice e.tape base (e.ptr + 1 - base) results, ptr := base }
        | _ => Engine.err "internal_error" ("no full-stack handler for " ++ name)
      | .boru _ _ => Engine.err "internal_error" "a boru body is not a full-stack handler"
      | .usurp _ _ => Engine.err "internal_error" "a usurp wrapper is not a full-stack handler"
    else do
      let (results, defs) ←
        match s.impl with
        | .native key =>
          match nat.lookup key with
          | some (.plain f) => f args e.defs
          | _ => Engine.err "internal_error" ("no handler for " ++ name)
        | .usurp orig b =>
          -- the original, re-dispatched in a group of its own with the
          -- arguments reversed and laid out for ITS barrier: what it
          -- takes from the stack before it, what it takes forward after
          let k := args.length - min b args.length
          pure (.openParen :: args.take k ++ [orig] ++ (args.drop k).reverse ++ [.closeParen], e.defs)
        | .boru body captured =>
          -- a call frame: bind the named parameters, splice the body in
          -- a paren group closed by the cleanup marker and return check
          let names := s.params.map Param.name
          -- the baseline is recorded BEFORE the frame binds anything, so
          -- what it binds is what a function built in its body captures
          let defs := e.defs.pushBaseline (bodyNeedsFrameState body)
          -- a closure's captures go in first: a parameter of the same
          -- name shadows one
          let defs := captured.foldl (fun d (nm, v) => d.push nm v) defs
          -- a list argument is bound QUOTED: the body reads it as data
          let bind (a : Value) : Value := if a.isListShape && !a.isQuoted then .quoted a else a
          let defs := (names.zip args).foldl (fun d (nm, a) => d.push nm (bind a)) defs
          let tail := Value.defCleanup (captured.map (·.1) ++ names)
            :: (if s.returns.isEmpty then [] else [.returnCheck s.returns])
          pure (.frameOpen :: body ++ tail ++ [.closeParen], defs)
      let e := { e with defs := defs }
      if indices.length == n && n > 0 then
        let first := indices.foldl min e.ptr
        let kept := (List.range (e.ptr + 1 - first)).filterMap fun k =>
          let i := first + k
          if indices.contains i || i == e.ptr then none else e.tape[i]?
        pure { e with tape := splice e.tape first (e.ptr + 1 - first) (kept ++ results), ptr := first }
      else if n == 0 then
        pure { e with tape := splice e.tape e.ptr 1 results }
      else
        let argStart := e.ptr - n
        pure { e with tape := splice e.tape argStart (e.ptr + 1 - argStart) results, ptr := argStart }

def evalArgs (nat : Natives) :
    Nat → Defs → List Value → Except BoruError (List Value × Defs)
  | 0, _, _ => Engine.outOfFuel
  | _ + 1, defs, [] => pure ([], defs)
  | fuel + 1, defs, v :: vs => do
    let (v', defs) ← if v.isPendingContainer then autoEval nat fuel defs v else pure (v, defs)
    let (vs', defs) ← evalArgs nat fuel defs vs
    pure (resolveInert v' :: vs', defs)

/-- Evaluate a pending container. A list's elements run as one program
    and the list of its residuals is the value. A map evaluates member by
    member: each value is a program whose single residual is the member,
    several residuals a list, and NO residual drops the key. The result
    is data — its pending flag is spent. -/
def autoEval (nat : Natives) : Nat → Defs → Value → Except BoruError (Value × Defs)
  | 0, _, _ => Engine.outOfFuel
  | fuel + 1, defs, v =>
    match v with
    | .list elems true =>
      if elems.isEmpty then pure (.list [] false, defs)
      else do
        let (res, defs) ← runSealed nat fuel defs elems
        pure (.list res false, defs)
    | .map entries true => do
      let (es, defs) ← autoEvalEntries nat fuel defs entries
      pure (.map es false, defs)
    | v => pure (v, defs)

def autoEvalEntries (nat : Natives) :
    Nat → Defs → List (String × Value) → Except BoruError (List (String × Value) × Defs)
  | 0, _, _ => Engine.outOfFuel
  | _ + 1, defs, [] => pure ([], defs)
  | fuel + 1, defs, (k, v) :: rest => do
    -- a paren-expression member runs as a GROUP inside the region
    let program := match v with
      | .parenExpr items => .openParen :: items ++ [.closeParen]
      | v => [v]
    let (res, defs) ← runSealed nat fuel defs program
    let (rest', defs) ← autoEvalEntries nat fuel defs rest
    match res with
    | [] => pure (rest', defs)
    | [one] => pure ((k, one) :: rest', defs)
    | many => pure ((k, .list many false) :: rest', defs)

/-- Run tokens as a SEALED REGION (core/go/sealed.go): the tokens are
    stepped between a paren pair of the region's own, so they see nothing
    beneath them and a collection parked inside is closed where the
    region closes. The residual is collected without being re-stepped,
    and a pending container left in it is evaluated in turn.

    The pair is real: an unmatched open paren among the tokens takes the
    region's close for its own, and the region — never closed — fails. -/
def runSealed (nat : Natives) : Nat → Defs → List Value → Except BoruError (List Value × Defs)
  | 0, _, _ => Engine.outOfFuel
  | fuel + 1, defs, toks => do
    let e ← steps nat fuel
      { tape := .openParen :: toks ++ [.closeParen], ptr := 1, defs := defs, sealed := true }
    -- the region's move collects what is there — evaluating a pending
    -- container left in it, whose own error surfaces first — and only then
    -- is a region that never closed refused
    let unclosed := e.sealed
    let e ← sweepRange nat fuel e 0 e.tape.length
    if unclosed || e.tape.any Value.isOpenParen then
      Engine.err "syntax_error" "unmatched opening parenthesis"
    else pure (e.tape, e.defs)

/-- Run a program to its residual: step to the end, resolve what is
    still parked, refuse an unmatched open paren, then evaluate the
    pending containers left in the result. -/
def runProgram (nat : Natives) : Nat → Defs → List Value → Except BoruError (List Value × Defs)
  | 0, _, _ => Engine.outOfFuel
  | fuel + 1, defs, input => do
    let e ← steps nat fuel { tape := input, ptr := 0, defs := defs }
    let e ← resolveOrphans nat fuel e
    if e.tape.any Value.isOpenParen then Engine.err "syntax_error" "unmatched opening parenthesis"
    else do
      let e ← sweepRange nat fuel e 0 e.tape.length
      pure (e.tape, e.defs)

/-- The end of the program closes every collection still parked. -/
def resolveOrphans (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    match (List.range e.tape.length).find? (fun i => (e.tape[i]?).any Value.isForward) with
    | none => pure e
    | some i =>
      match e.tok? i with
      | some (.forward f _) => do
        let tape := removeAt e.tape i
        let funcIdx := if i < f.funcIndex then f.funcIndex - 1 else f.funcIndex
        let e := curryOrStack { e with tape := tape } funcIdx f.collected f.stackArgs
        let e ← orphanSteps nat fuel e
        resolveOrphans nat fuel e
      | _ => pure e

def orphanSteps (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    if e.ptr ≥ e.tape.length then pure e
    else match e.tok? e.ptr with
      | some .openParen | some .frameOpen => orphanSteps nat fuel { e with ptr := e.ptr + 1 }
      | _ => do
        let e ← stepToken nat fuel e
        orphanSteps nat fuel e

end

/-- The default step budget. -/
def defaultFuel : Nat := 100000

/-- Run a program over a binding store and a handler table. -/
def run (nat : Natives) (defs : Defs) (program : List Value) (fuel : Nat := defaultFuel) :
    Except BoruError (List Value) :=
  (runProgram nat fuel defs program).map (·.1)

end Boru.Core
