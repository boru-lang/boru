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
import Boru.Core.Truth

namespace Boru.Core

/-- An error the kernel raises: a taxonomy code and a human detail. -/
structure BoruError where
  code : String
  detail : String := ""
  deriving Repr, BEq

/-- THE DRIVER OF A LOOP (core/go/loop.go `LoopDriver`): what a looping
    word hands the engine instead of a result. The engine runs one sealed
    region per iteration and gives each region's residual back; the driver
    says what an iteration is and what the loop answers. Its state is the
    values it has gathered so far. -/
structure Driver where
  /-- Iteration `it` (0-based) of a call with these arguments: the INERT
      inputs its region opens with — the body's whole stack, never
      stepped — and the body's tokens; or nothing, when no iteration is
      left. -/
  next : List Value → Nat → Except BoruError (Option (List Value × List Value))
  /-- Take what iteration `it`'s region left, bottom to top. -/
  collect : List Value → Nat → List Value → List Value → Except BoruError (List Value)
  /-- The loop's results. -/
  finish : List Value → List Value → Except BoruError (List Value)

/-- A host handler. A plain handler receives its arguments in signature
    order and the binding store; a FULL-STACK handler also receives the
    resolved stack of its paren scope and returns its replacement; a
    DRIVEN one runs a loop. -/
inductive Native where
  | plain (f : List Value → Defs → Except BoruError (List Value × Defs))
  | fullStack (f : List Value → List Value → Except BoruError (List Value))
  | driven (d : Driver)

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
    | .frameOpen _ :: rest, i, depth => go rest (i + 1) (depth + 1)
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
    | some (.frameOpen _) => some i
    | _ => frameOpenBefore tape i

/-- The nearest open paren before `i`. -/
def openBefore (tape : List Value) : Nat → Option Nat
  | 0 => none
  | i + 1 =>
    match tape[i]? with
    | some .openParen | some (.frameOpen _) => some i
    | _ => openBefore tape i

/-- Is a parked forward's NEXT slot a name-capturing (`/q`) one. -/
def pendingQuoteSlot (tape : List Value) (defs : Defs) (ptr : Nat) : Bool :=
  match pendingForwardIdx tape ptr with
  | some i =>
    match tape[i]? with
    | some (.forward f s) => f.collected < s.arity && s.isQuote f.collected
    | _ => false
  | none => false

/-- Is a parked forward's NEXT slot a FORM slot: the word about to be
    stepped is collected as it is written — not run, not replaced by what
    it is bound to, not turned into a name (core/go
    `hasPendingForwardFormArg`). -/
def pendingFormSlot (tape : List Value) (ptr : Nat) : Bool :=
  match pendingForwardIdx tape ptr with
  | some i =>
    match tape[i]? with
    | some (.forward f s) => f.collected < s.arity && (s.raw f.collected).form
    | _ => false
  | none => false

/-- Does the nearest parked forward capture a paren expression RAW. The
    question is asked of the forward's SIGNATURE, not of its next slot:
    a signature with a raw-paren or form slot anywhere takes a paren
    expression that arrives at the pointer as it stands (core/go
    `pendingForwardWantsRawParen`). The plan walk has already evaluated
    the paren expressions written at the signature's ordinary slots, so
    the two questions differ only for an overload whose raw slot another
    overload of the word kept raw. -/
def pendingWantsRawParen (tape : List Value) (ptr : Nat) : Bool :=
  match pendingForwardIdx tape ptr with
  | some i =>
    match tape[i]? with
    | some (.forward _ s) => s.wantsRawParen
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
  | .forward _ _ | .defCleanup _ _ _ | .returnCheck _ _ _ | .endMark | .closeParen
  | .mark _ | .move _ _ => true
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

/-- A word DEFINED IN BORU answers a failed plan itself (core/go
    `fnFallbackSig`): the function a name dispatches to carries a
    zero-argument fallback whenever one of its overloads has a body, the
    plan hands that back in place of "no match", and the call is refused
    there — so the STACK-ONLY RETRY a native word gets, and a function
    literal standing at the pointer gets, never runs for it. A forced
    arity (`/N`) rules the fallback out and the retry back in. NUR398
    records the difference as a defect of the kernel's. -/
def answersItself (sigs : List Sig) (w : WordInfo) : Bool :=
  w.argCount.isNone && sigs.any fun s =>
    match s.impl with
    | .boru (_ :: _) _ _ _ _ => true
    | _ => false

/-- ARITY WIDENING (core/go `fnValueWouldWiden`). A function VALUE whose
    collection has just completed is re-stepped reading the stack only —
    unless one of its WIDER overloads could take the token that follows
    it, in any position it may collect forward: then the call is planned
    again in full, and the arguments already collected fall to the stack
    side of the wider overload.

    What the next token would contribute decides. A group is an
    expression whose value is not known: it is taken to widen. A `/v`
    word is a function; a name bound to a value is that value; `true`,
    `false` and `none` are themselves; a literal is itself. A boundary, a
    function word and an unbound name prove nothing. -/
def wouldWiden (tape : List Value) (defs : Defs) (sigs : List Sig) (completed idx : Nat) : Bool :=
  let wider := sigs.filter fun s => s.arity > completed && s.barrier != 0
  let claims (probe : Value) : Bool :=
    wider.any fun s => (List.range (min s.barrier s.arity)).any fun slot => sigArgMatches s slot probe
  if wider.isEmpty then false
  else match tape[idx]? with
    | none | some .endMark | some .closeParen | some (.forward _ _) | some (.dispatchMod _) => false
    | some .openParen | some (.frameOpen _) | some (.parenExpr _) => true
    | some (.word w) =>
      if w.forceVal then claims (.fn "" [] 0)
      else match defs.top? w.name with
        | some top => if top.isFn then false else claims top
        | none =>
          if w.name == "true" || w.name == "false" then claims (.bool (w.name == "true"))
          else if w.name == "none" then claims .none
          else false
    | some v => claims v

/-- THE USURP WRAPPER (core/go/core_ref.go `usurpOver`): a function whose
    every signature is one of the original's with its parameters REVERSED
    and every one of them collectable forward, so `f/u a b c` is `f c b a`.
    A parameter keeps its type, pattern and `/q`; a type-arg slot is the
    original's to enforce, on the re-dispatch. The wrapper's handler
    re-dispatches the original as it was bound. -/
def usurpOver (name : String) (sigs : List Sig) (origIdent ident : Nat) : Value :=
  let orig := Value.fn name sigs origIdent
  let wrapped := sigs.map fun s =>
    -- the raw-capture modes are NOT reversed with the parameters: the
    -- wrapper drops the two no-evaluation modes — so it evaluates a list
    -- or map the original would have taken raw — and keeps the paren and
    -- form modes at the positions they had in the ORIGINAL, which are
    -- other parameters' positions now (NUR413, as the kernel has it)
    Sig.mk (s.params.reverse.zipIdx.map fun (p, i) =>
        let stale := ((s.params[i]?).map Param.raw).getD {}
        Param.mk p.name p.type p.pattern p.quote false p.optional
          { paren := stale.paren, form := stale.form })
      s.returns s.arity (.usurp orig s.barrier) false
  .fn name (wrapped.foldl (fun acc s => Defs.insertSig s acc) []) ident

/-- Resolve the type-name words inside an inert TYPE SHAPE — a typed
    container's child, a disjunct's alternatives — where the value is
    consumed or left as a result (ADR-012 rule 4). A quoted value stays
    opaque. A name a program bound to a type is resolved to that type's
    declared CONTENT, not to its node: `[: Foo ]`, where `Foo` is a union,
    is a list of the union's alternatives. -/
def resolveInert (d : Defs) : Value → Value
  | .typedList child elems => .typedList (resolveWordsDeepWith d.typeBody? child) elems
  | .typedMap child entries => .typedMap (resolveWordsDeepWith d.typeBody? child) entries
  | .disjunct alts => resolveWordsDeepWith d.typeBody? (.disjunct alts)
  | v => v

/-- A FRAME'S CLEANUP (core/go `stepDefCleanup`). What the body bound for
    itself goes first: every name is unbound down to the frame's entry
    snapshot — in a frame that took one. Then the args list and the
    baseline the frame was entered with, and last the frame's own names,
    newest first: ONE binding of each, whatever the body did to them. -/
def popNames (defs : Defs) (names : List String) (snapshot : Option (List (String × Nat))) : Defs :=
  let defs := match snapshot with
    | some snap => defs.truncateTo snap
    | none => defs
  (names.reverse.foldl Defs.pop defs).popArgs.popBaseline

/-- Close the call frames a discarded stretch of tape leaves open: each
    frame's cleanup marker still ahead in it unbinds what the frame bound,
    innermost first, as its own tail would have (core/go/fn_frame.go
    `unwindLiveFrames`). -/
def unwindFrames (defs : Defs) (ahead : List Value) : Defs :=
  ahead.foldl (fun d v => match v with | .defCleanup names _ snap => popNames d names snap | _ => d) defs

/-- RESOLVE A FLOW SIGNAL ON THIS TAPE (core/go/engine.go
    `handleLoopBreak`, `handleLoopContinue`). The loop that takes it is
    the NEAREST counted or conditional loop whose move stands ahead of the
    pointer, before `limit`, with its mark still on the tape. Whatever
    stands between — the rest of the round, a group, a call frame — is
    discarded, and the frames in it are closed.

    BREAK ends the loop: the values its COMPLETED rounds left stand where
    it stood, and the index is unbound. The round in progress leaves
    nothing — not even what it took from the stack beneath the loop.

    CONTINUE ends the round: what it left is dropped, and the pointer
    stands on the move, which starts the next round, or ends the loop, as
    it would have.

    `none` when no loop on this tape takes the signal: it stays raised
    for the run that holds this one. -/
def handleFlow (e : Engine) (limit : Nat) : Option Engine :=
  match e.defs.flow? with
  | none => none
  | some isBreak =>
    let markOf (to : Nat) (upto : Nat) : Option Nat :=
      (List.range upto).find? fun j => match e.tape[j]? with | some (.mark id) => id == to | _ => false
    -- an `if`'s move and a replay's are no loops: the signal passes them
    let taker := (List.range (min limit e.tape.length)).find? fun i =>
      i ≥ e.ptr && (match e.tape[i]? with
        | some (.move to (.counted ..)) | some (.move to (.cond ..)) => (markOf to i).isSome
        | _ => false)
    match taker with
    | none => none
    | some moveIdx =>
      match e.tape[moveIdx]? with
      | some (.move to loop) =>
        let markIdx := (markOf to moveIdx).getD 0
        let defs := (unwindFrames e.defs ((e.tape.drop e.ptr).take (moveIdx - e.ptr))).takeFlow
        if isBreak then
          let (defs, results) := match loop with
            | .counted index _ _ _ _ results depth => (defs.popTo index (depth - 1), results)
            | .cond _ _ results _ => (defs, results)
            | .branch _ _ | .replay _ => (defs, [])
          some { e with tape := splice e.tape markIdx (moveIdx - markIdx + 1) results,
                        ptr := markIdx, defs := defs }
        else
          some { e with tape := splice e.tape (markIdx + 1) (moveIdx - markIdx - 1) [],
                        ptr := markIdx + 1, defs := defs }
      | _ => none

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

/-- WHAT A SPLICE MARKER CONTRIBUTES (core/go `SpliceExpand`): a plain
    list gives its top-level elements — pending or quoted, it is only a
    carrier here — and any other value gives itself. Nothing is evaluated:
    what is spliced is stepped afterwards, where the marker stood. -/
def spliceExpand : Value → List Value
  | .list elems _ => elems
  | .quoted (.list elems _) => elems
  | v => [v]

/-- A DATA splice (core/go `spliceIsData`): what it contributes is plain
    values at the top level — no word, no paren expression, no marker. It
    gives the same values wherever it stands, so a word bound to one may
    be read as the group `(w)`. A CODE-bearing splice is a macro that runs
    against the live stack, and is never grouped. -/
def spliceIsData (payload : Value) : Bool :=
  (spliceExpand payload).all fun v => match v with
    | .word _ | .parenExpr _ | .splice _ => false
    | _ => true

/-- Is `name` bound to a data splice. -/
def boundToDataSplice (d : Defs) (name : String) : Bool :=
  match d.top? name with
  | some (.splice payload) => spliceIsData payload
  | _ => false

/-- The words whose execution can bind a name in the current scope or
    construct an inner function (`frameStateWords`). -/
def frameStateWords : List String :=
  ["def", "undef", "fn", "afn", "do", "call", "eval", "var", "word", "module", "import", "export",
   "usurp", "behave"]

/-- A COMPUTING body (core/go `BodyEvalsResidual`): more than one token,
    or one paren expression. A body of one bare token is not one. -/
def bodyEvalsResidual (body : List Value) : Bool :=
  body.length > 1 || (match body with | [.parenExpr _] => true | _ => false)

/-- A name STANDS FOR a binding word when it is bound to that word's own
    function (core/go `fnValueIsBindingWord`): `def mydef def/v` makes
    `mydef` one. The function is known by the word's name, or — reached
    through another reference, under another name — by BEING the function
    a binding word is bound to: a host function of the same identity. -/
def standsForBindingWord (d : Defs) (name : String) : Bool :=
  match d.top? name with
  | some (.fn fname sigs ident) =>
    frameStateWords.contains fname ||
    (!sigs.isEmpty && sigs.all (fun s => match s with | .mk _ _ _ (.native _) _ => true | _ => false) &&
      frameStateWords.any fun fw =>
        match d.top? fw with
        | some (.fn _ _ i) => i == ident
        | _ => false)
  | _ => false

/-- A body NEEDS FRAME STATE when it names a binding word, or a name that
    stands for one in the bindings it is asked against. A body that names
    none is a LEAF: its frame is entered with no baseline and records no
    snapshot (`bodyNeedsFrameState`). -/
def bodyNeedsFrameState (d : Defs) (body : List Value) : Bool :=
  -- A body word can HIDE a binding word: a name bound to a splice marker
  -- splices its payload into THIS frame, so the payload's words are the
  -- body's too — through as many markers as it takes, each name once
  let rec go (fuel : Nat) (seen : List String) (names : List String) : Bool :=
    match fuel with
    | 0 => false
    | fuel + 1 =>
      match names with
      | [] => false
      | n :: rest =>
        if frameStateWords.contains n || standsForBindingWord d n then true
        else if seen.contains n then go fuel seen rest
        else match d.top? n with
          | some (.splice payload) =>
            go fuel (n :: seen) (bodyWordsList (spliceExpand payload) ++ rest)
          | _ => go fuel seen rest
  let names := bodyWordsList body
  go (names.length + d.foldl (fun acc (_, vs) => acc + sizeList vs) 0 + 1) [] names

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
      -- a flow signal raised during the step: the nearest loop ahead on
      -- this tape takes it; with none, the run ends here with the signal
      -- still raised, for the run that holds this one
      if e.defs.flowing then
        match handleFlow e e.tape.length with
        | some e' => steps nat fuel e'
        | none => pure e
      else steps nat fuel e

/-- Step the token at the pointer. -/
def stepToken (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    match e.tok? e.ptr with
    | none => pure e
    | some (.word w) => stepWord nat fuel e w
    | some (.forward _ _) => pure { e with ptr := e.ptr + 1 }
    -- an open paren is stepped PAST, and past the inert arguments a call
    -- frame's carries (core/go `stepPastOpenParen`)
    | some .openParen => pure { e with ptr := e.ptr + 1 }
    | some (.frameOpen span) => pure { e with ptr := e.ptr + 1 + span }
    | some .closeParen => stepCloseParen nat fuel e
    | some .endMark => stepEnd nat fuel e
    | some (.returnCheck _ _ _) => pure { e with ptr := e.ptr + 1 }
    | some (.mark _) => pure { e with ptr := e.ptr + 1 }
    | some (.move to loop) => stepMove nat fuel e to loop
    | some (.defCleanup names evalResidual snap) => do
      -- the frame's cleanup runs ONCE: stepped, the marker is spent and
      -- leaves the tape, so the frame's close finds nothing to run again
      -- (run twice, it unbound an outer binding a parameter had shadowed)
      let e ← if evalResidual then evalFrameResidual nat fuel e e.ptr else pure e
      -- a signal a residual literal let out: the frame is still open, and
      -- the resolution closes it — the marker is left unspent for that
      if e.defs.flowing then pure e
      else pure { e with defs := popNames e.defs names snap, tape := removeAt e.tape e.ptr }
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
    -- and one whose next slot is a FORM slot takes the word itself
    else if pendingFormSlot e.tape e.ptr then stepLiteral nat fuel e
    else
      match e.defs.top? w.name with
      | some top =>
        if !top.isFn then
          -- `f w` IS `f (w)` for a word bound to a DATA splice that a word
          -- is waiting to collect: the word becomes the group `(w)`, so
          -- the payload's values arrive from a segment of their own — as
          -- many as there are. A BINDER is exempt: `def y w` rebinds the
          -- marker, and `y` is the splice under another name (the kernel
          -- knows the binder by its name, `def`: `bindsReferent`)
          let grouped := boundToDataSplice e.defs w.name &&
            (match pendingForwardIdx e.tape e.ptr with
             | some i => (match e.tape[i]? with
                | some (.forward f _) => f.funcName != "def"
                | _ => false)
             | none => false)
          if grouped then stepLiteral nat fuel { e with tape := setAt e.tape e.ptr (.parenExpr [.word w]) }
          else stepLiteral nat fuel { e with tape := setAt e.tape e.ptr top }
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
          -- a signal a group let out: the word is abandoned where it stands
          if e.defs.flowing then pure e else
          -- the plan — after the pending operands its patterns must judge
          -- have been evaluated, where they stand
          let (e, plan) ← planEvaluating nat fuel e sigs w
            (hasForwardSigs sigs && !w.forceStack && !answersItself sigs w)
          -- a signal an operand let out: the word is abandoned where it stands
          if e.defs.flowing then pure e else
          match plan with
          | none =>
            if e.voidGroups.contains w.name then
              -- the word named `def` has the error under its own code
              -- (core/go `voidArgErrorFor`: the kernel knows that one name)
              Engine.err (if w.name == "def" then "def_error" else "no_value_error")
                ("argument expression produced no value for " ++ w.name)
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

/-- THE PLAN OF A DISPATCH (core/go `matchForDispatch`). The signatures
    are matched as the plan-level matcher matches them — and a pending
    list or map that a candidate's pattern must judge is EVALUATED first,
    where it stands on the tape, as the call would evaluate it (NUR235): a
    pattern reads the value the callee receives, not the tokens it was
    written as. The evaluation happens once; every later candidate, and
    the call itself, see the value.

    With no match, a word that may is planned again reading the stack
    only (`retry`), over the tape as the first attempt left it.

    An error the evaluation raises is the dispatch's. A flow signal it
    lets out abandons the dispatch: the state comes back with the signal
    raised and no plan. -/
def planEvaluating (nat : Natives) :
    Nat → Engine → List Sig → WordInfo → Bool → Except BoruError (Engine × Option Plan)
  | 0, _, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, sigs, w, retry => do
    let e ← settleOperands nat fuel e sigs w
    if e.defs.flowing then pure (e, none) else
    let inside := (pendingForwardIdx e.tape e.ptr).isSome
    match planMatch e.tape e.defs sigs w (effectiveResolved e.tape e.ptr) e.ptr inside with
    | some p => pure (e, some p)
    | none =>
      if !retry then pure (e, none)
      else do
        let ws := { w with forceStack := true }
        let e ← settleOperands nat fuel e sigs ws
        if e.defs.flowing then pure (e, none) else
        pure (e, planMatch e.tape e.defs sigs ws (effectiveResolved e.tape e.ptr) e.ptr inside)

/-- Evaluate, one at a time and in the order the matcher meets them, the
    pending operands a candidate's pattern must wait for. Each is replaced
    on the tape by its value, which is no longer pending — so the walk
    ends. -/
def settleOperands (nat : Natives) : Nat → Engine → List Sig → WordInfo → Except BoruError Engine
  | 0, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, sigs, w =>
    let inside := (pendingForwardIdx e.tape e.ptr).isSome
    match planNeed e.tape e.defs sigs w (effectiveResolved e.tape e.ptr) e.ptr inside with
    | none => pure e
    | some (idx, val) => do
      let (v, defs) ← autoEval nat fuel e.defs val
      if defs.flowing then pure { e with defs := defs }
      else settleOperands nat fuel { e with tape := setAt e.tape idx v, defs := defs } sigs w

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
    collectWalk nat fuel (w.name == "def") e sigs viable 0 (e.ptr + 1)

def collectWalk (nat : Natives) :
    Nat → Bool → Engine → List Sig → List (Sig × Nat) → Nat → Nat → Except BoruError Engine
  | 0, _, _, _, _, _, _ => Engine.outOfFuel
  | fuel + 1, binder, e, sigs, viable, pos, scanIdx =>
    let maxBarrier := viable.foldl (fun m (_, b) => max m b) 0
    let consumes := viable.any (fun (_, b) => pos < b)
    let rawSlot (s : Sig) : Bool := s.isRawSlot pos
    -- does any overload capture THIS token structurally at this position
    -- (core/go `capturesForwardToken`): a keyword slot captures only its
    -- keyword, so a word that is not it keeps its ordinary treatment —
    -- the boundary a function word is, the group a data splice becomes
    let captures (tok : Value) : Bool := sigs.any (fun s => s.capturesToken pos tok)
    -- a paren expression is left as code when ANY overload of the word
    -- captures this position raw (core/go `rawParenForward`,
    -- `rawFormForward`): the walk does not know yet which one will run
    let keepsParen := sigs.any (fun s => s.keepsParen pos)
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
          | .openParen | .frameOpen _ =>
            if !consumes then pure e
            else do
              let e ← evalParenGroupAt nat fuel e scanIdx
              if e.defs.flowing then pure e else
              let viable := match e.tok? scanIdx with
                | some res => (match res with
                    | .word tw => (match e.defs.top? tw.name with
                        | some top => viable.filter fun (s, b) =>
                            !(pos < b && !s.isRawSlot pos && !s.isQuote pos && forwardPatternRejects s pos top)
                        | none => viable)
                    | v => viable.filter fun (s, b) =>
                        !(pos < b && !s.isRawSlot pos && !s.isQuote pos && forwardPatternRejects s pos v))
                | none => viable
              collectWalk nat fuel binder e sigs viable (pos + 1) (scanIdx + 1)
          | .parenExpr items =>
            -- at a raw-capture position it stays code, for the overload
            -- that captures it
            if keepsParen then collectWalk nat fuel binder e sigs viable (pos + 1) (scanIdx + 1)
            else
            -- a paren expression becomes its marker span and is re-read
            collectWalk nat fuel binder
              { e with tape := splice e.tape scanIdx 1 (.openParen :: items ++ [.closeParen]) }
              sigs viable pos scanIdx
          | .word tw =>
            -- `f w` IS `f (w)` for a word bound to a DATA splice: at a
            -- position a still-viable overload collects, and no overload
            -- captures structurally, the word is rewritten to the group
            -- and read again — so the group is evaluated here, and what
            -- it leaves fills as many slots as it has values. A binder's
            -- operand is exempt (`def y w` rebinds the marker)
            if consumes && !binder && !captures tok && boundToDataSplice e.defs tw.name then
              collectWalk nat fuel binder
                { e with tape := setAt e.tape scanIdx (.parenExpr [tok]) } sigs viable pos scanIdx
            else
            if !captures tok && (e.defs.lookup? tw.name).isSome && !tw.forceVal then pure e
            else collectWalk nat fuel binder e sigs (pruneResolved tok) (pos + 1) (scanIdx + 1)
          | v =>
            let viable :=
              if v.isConcrete then
                viable.filter fun (s, b) =>
                  !(pos < b && !rawSlot s
                    && ((s.ptype pos != TAny && !sigArgMatches s pos v) || forwardPatternRejects s pos v))
              else viable
            collectWalk nat fuel binder e sigs viable (pos + 1) (scanIdx + 1)

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
      | some .openParen => groupLoop nat fuel { e with ptr := e.ptr + 1 } scanIdx (depth + 1)
      | some (.frameOpen span) => groupLoop nat fuel { e with ptr := e.ptr + 1 + span } scanIdx (depth + 1)
      | some .closeParen =>
        if e.sealed && e.ptr + 1 == e.tape.length then pure e
        else do
          let e ← stepCloseParen nat fuel e
          groupFlow nat fuel e scanIdx (depth - 1)
      | some (.defCleanup _ _ _) =>
        if (frameOpenBefore e.tape e.ptr).any (· < scanIdx) then pure e
        else do
          let e ← stepToken nat fuel e
          groupFlow nat fuel e scanIdx depth
      | some _ => do
        let e ← stepToken nat fuel e
        groupFlow nat fuel e scanIdx depth

/-- After a step inside the group: carry on — unless the step raised a
    FLOW SIGNAL. A loop the group itself holds is the nearest one and takes
    it, and the group goes on from where that leaves the tape. Otherwise a
    paren is no loop boundary: the group is abandoned WHOLE — the frames
    its run opened are closed, its tokens leave the tape — and the signal
    stays raised for the run that holds the word collecting it. -/
def groupFlow (nat : Natives) : Nat → Engine → Nat → Nat → Except BoruError Engine
  | 0, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, scanIdx, depth =>
    if !e.defs.flowing then groupLoop nat fuel e scanIdx depth
    else
      let groupEnd := (findCloseAfter e.tape scanIdx).getD e.tape.length
      match handleFlow e groupEnd with
      | some e' =>
        -- the resolution rewrote the group's tokens: read the depth back
        let inside := (e'.tape.drop scanIdx).take (e'.ptr - scanIdx)
        let depth := (inside.filter Value.isOpenParen).length - (inside.filter Value.isCloseParen).length
        groupLoop nat fuel e' scanIdx depth
      | none =>
        let defs := unwindFrames e.defs ((e.tape.drop e.ptr).take (groupEnd - e.ptr))
        pure { e with tape := splice e.tape scanIdx (groupEnd + 1 - scanIdx) [], defs := defs }

/-- Step a value that is not a word: it is collected by the nearest
    parked forward if it fits that forward's next slot, and closes the
    collection if it does not; with nothing parked it is simply resolved. -/
def stepLiteral (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    let valIdx := e.ptr
    match e.tok? valIdx with
    | none => pure e
    | some (.applied inner) =>
      -- THE APPLICATION MARK is spent by the step that meets it (core/go
      -- `takeAppliedMark`), whatever follows: on a function with nothing
      -- waiting for it, it is the request to call; anywhere else the
      -- value is stepped again as itself
      let e := { e with tape := setAt e.tape valIdx inner }
      match inner, pendingForwardIdx e.tape e.ptr with
      | .fn name sigs _, none =>
        match e.tok? (valIdx + 1) with
        | some (.dispatchMod _) =>
          pure { e with tape := removeAt e.tape (valIdx + 1), ptr := valIdx + 1, sealAt := none }
        | _ => dispatchFnValue nat fuel e name sigs true
      | _, _ => pure e
    | some val =>
      -- a paren expression becomes its marker span and is re-stepped —
      -- unless a parked forward that captures parens raw is waiting: then
      -- it is a value like any other, and is collected as it stands
      let expands := match val with
        | .parenExpr _ => !pendingWantsRawParen e.tape e.ptr
        | _ => false
      if expands then
        match val with
        | .parenExpr items =>
          pure { e with tape := splice e.tape valIdx 1 (.openParen :: items ++ [.closeParen]) }
        | _ => pure e
      else
      match pendingForwardIdx e.tape e.ptr with
      | none =>
        match val with
        -- A SPLICE MARKER with nothing waiting for it FIRES: its payload
        -- takes its place, unevaluated, and is stepped from there against
        -- the live stack. (With a word waiting it is a value like any
        -- other, collected below if the slot takes it.)
        | .splice payload => pure { e with tape := splice e.tape valIdx 1 (spliceExpand payload) }
        | .dispatchMod _ => pure { e with tape := removeAt e.tape valIdx }
        | .fn name sigs _ =>
          -- a function value at the pointer is a CALL, unless a `/v` or
          -- `/q` marker right after it says it is data
          match e.tok? (valIdx + 1) with
          | some (.dispatchMod _) =>
            pure { e with tape := removeAt e.tape (valIdx + 1), ptr := valIdx + 1, sealAt := none }
          | _ => dispatchFnValue nat fuel e name sigs false
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
                -- a word is rewritten to `/s`; a function value is sealed,
                -- unless a wider overload could take what follows it
                let sealAt := match tape[funcIdx]? with
                  | some (.fn _ sigs _) =>
                    if wouldWiden tape e.defs sigs (collected + f.stackArgs) (funcIdx + 1) then none
                    else some funcIdx
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
def dispatchFnValue (nat : Natives) :
    Nat → Engine → String → List Sig → Bool → Except BoruError Engine
  | 0, _, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, name, sigs, applied => do
    let sealed := e.sealAt == some e.ptr
    let e := { e with sealAt := none }
    let w : WordInfo := { name := name, forceStack := sealed }
    let e ←
      if hasForwardSigs sigs && !w.forceStack then collectForward nat fuel e sigs w else pure e
    if e.defs.flowing then pure e else
    let (e, plan) ← planEvaluating nat fuel e sigs w (hasForwardSigs sigs && !w.forceStack)
    if e.defs.flowing then pure e else
    let resolved := effectiveResolved e.tape e.ptr
    match plan with
    | none =>
      -- what follows, up to a boundary, could have been its arguments
      let upcoming := ((e.tape.drop (e.ptr + 1)).filter (!·.isForward)).takeWhile
        (fun v => !(v.isCloseParen || v.isEnd || v.isLoopToken))
      -- an ANONYMOUS function is a value until something it can take
      -- stands beside it: unmatched, it is simply left where it is
      if !name.isEmpty && (!resolved.isEmpty || !upcoming.isEmpty) then
        Engine.err "uncalled_function" ("call to '" ++ name ++ "' matched no signature")
      else pure { e with ptr := e.ptr + 1 }
    | some p =>
      -- AN ANONYMOUS FUNCTION THAT TAKES NOTHING IS DATA WHERE IT STANDS
      -- (core/go `execFnDefLiteral`'s gate): a call of it would need
      -- nothing beside it, so standing anywhere would be a call — and the
      -- value could not be bound, stored or handed on. Only an
      -- APPLICATION that was asked for calls it: the mark `applied`
      -- carries. A function that is not anonymous is called (NUR417, as
      -- the kernel has it: the two differ by where they came from)
      let anonymous := sigs.any fun s => match s.impl with
        | .boru _ _ true _ _ => true
        | _ => false
      if anonymous && !applied && p.positions.isEmpty then pure { e with ptr := e.ptr + 1 } else
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

/-- THE MOVE (core/go/engine.go `stepMove`, `stepMoveCont`,
    `stepMoveWhile`). A loop's round is `mark body… move` on the tape, with
    no paren around it: the body runs on the stack the loop's word stood
    on. Stepped, the move finds its mark — the nearest behind it — and
    takes what stands between them as the round's values.

    A COUNTED loop keeps them, advances the index, and while the index is
    short of its bound replays the body under a new mark, the index
    rebound; when it is not, the index is unbound and every round's values
    stand where the loop stood — and are stepped from there, as any
    word's results are.

    A CONDITIONAL loop alternates two regions. The condition region's LAST
    value decides: true, and the body region replaces it; false, and the
    loop's values replace it; no value at all is an error. The body
    region's values are kept, and the condition replaces it.

    Two moves are no loops. An `if`'s region is a condition: its last
    value picks the branch that takes the region's place. A plain move
    replays its mark's body once.

    A move whose mark is gone is dropped where it stands. -/
def stepMove (nat : Natives) : Nat → Engine → Nat → Loop → Except BoruError Engine
  | 0, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, to, loop =>
    let moveIdx := e.ptr
    let isMark (i : Nat) : Bool := match e.tape[i]? with | some (.mark id) => id == to | _ => false
    match (List.range moveIdx).reverse.find? isMark with
    | none => pure { e with tape := removeAt e.tape moveIdx }
    | some markIdx =>
      let span := moveIdx - markIdx + 1
      let between := (e.tape.drop (markIdx + 1)).take (moveIdx - markIdx - 1)
      -- a round under a fresh mark, in the old round's place
      let replay (defs : Defs) (region : List Value) (next : Loop) : Engine :=
        let (id, defs) := defs.freshMark
        { e with tape := splice e.tape markIdx span (.mark id :: region ++ [.move id next]),
                 ptr := markIdx, defs := defs }
      -- the loop's values, in the loop's place, stepped from there
      let finish (defs : Defs) (results : List Value) : Engine :=
        { e with tape := splice e.tape markIdx span results, ptr := markIdx, defs := defs }
      match loop with
      | .counted index current stop step body results depth => do
        let (region, defs) ← collectRegion nat fuel e.defs between
        -- a signal a literal in the round let out is THIS loop's: the
        -- collection is undone and the pointer stays on the move, where
        -- the run finds the loop and breaks or continues it
        if defs.flowing then pure { e with defs := defs } else
        let results := results ++ region
        -- the index is an Integer: an advance past the range WRAPS, and
        -- the wrapped index is short of the bound again (NUR402, as the
        -- kernel has it)
        let current := wrapInt64 (current + step)
        if (step > 0 && current < stop) || (step < 0 && current > stop) then
          -- the round's own bindings of the index name end with it; then
          -- the index is rebound
          let defs := ((defs.popTo index depth).pop index).push index (.int current)
          pure (replay defs body (.counted index current stop step body results depth))
        else
          pure (finish (defs.popTo index (depth - 1)) results)
      | .cond test body results true => do
        let (region, defs) ← collectRegion nat fuel e.defs between
        if defs.flowing then pure { e with defs := defs } else
        pure (replay defs test (.cond test body (results ++ region) false))
      | .cond test body results false =>
        match between.getLast? with
        | none => Engine.err "runtime_error" "while: condition produced no value"
        | some v =>
          if coerceBoolean v then pure (replay e.defs body (.cond test body results true))
          else pure (finish e.defs results)
      -- the `if` continuation: the chosen branch takes the region's place
      -- — the condition's values with it — and is stepped from there
      | .branch thenBranch elseBranch =>
        match between.getLast? with
        | none => Engine.err "runtime_error" "if: condition produced no value"
        | some v => pure (finish e.defs (if coerceBoolean v then thenBranch else elseBranch))
      -- the plain move: the mark's body, once more, in the region's place
      | .replay body => pure (finish e.defs body)

/-- What a round left, as the loop keeps it: a pending container among
    the values is evaluated HERE, while the round's bindings are live —
    the round is the literal's frame — and everything else is kept as it
    stands. -/
def collectRegion (nat : Natives) :
    Nat → Defs → List Value → Except BoruError (List Value × Defs)
  | 0, _, _ => Engine.outOfFuel
  | _ + 1, defs, [] => pure ([], defs)
  | fuel + 1, defs, v :: vs => do
    let (v', defs) ← if v.isPendingContainer then autoEval nat fuel defs v else pure (v, defs)
    if defs.flowing then pure ([], defs) else do
    let (vs', defs) ← collectRegion nat fuel defs vs
    pure (v' :: vs', defs)

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
      if e.defs.flowing then pure e else
      let tape := removeAt (removeAt e.tape (e.tape.length - 1)) 0
      pure { e with tape := tape, ptr := tape.length, sealed := false }
    else
    match openBefore e.tape closeIdx with
    | none => Engine.err "syntax_error" "unmatched closing parenthesis"
    | some openIdx => do
      let e ← closeForwards nat fuel e openIdx
      if e.defs.flowing then pure e else
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
        if e.defs.flowing then pure e else
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
        if e.defs.flowing then pure e else regionForwards nat fuel e
      | _ => pure e

/-- Step until the pointer reaches the region's close (the last token). -/
def regionSteps (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    if e.ptr + 1 ≥ e.tape.length then pure e
    else match e.tok? e.ptr with
      | some .openParen => regionSteps nat fuel { e with ptr := e.ptr + 1 }
      | some (.frameOpen span) => regionSteps nat fuel { e with ptr := e.ptr + 1 + span }
      | _ => do
        let e ← stepToken nat fuel e
        if e.defs.flowing then pure e else regionSteps nat fuel e

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
          if e.defs.flowing then pure e else closeForwards nat fuel e openIdx
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
        | some .openParen => stepsToClose nat fuel { e with ptr := e.ptr + 1 } openIdx
        | some (.frameOpen span) => stepsToClose nat fuel { e with ptr := e.ptr + 1 + span } openIdx
        | _ => do
          let e ← stepToken nat fuel e
          if e.defs.flowing then pure e else stepsToClose nat fuel e openIdx

/-- A call frame closing: run its cleanup marker, then its return check
    over the group's residual. -/
def closeFrame (nat : Natives) : Nat → Engine → Nat → Nat → Except BoruError Engine
  | 0, _, _, _ => Engine.outOfFuel
  | fuel + 1, e, openIdx, closeIdx =>
    let cleanupAt := (List.range closeIdx).find? fun i =>
      i > openIdx && (match e.tape[i]? with | some (.defCleanup _ _ _) => true | _ => false)
    match cleanupAt with
    | some i =>
      match e.tok? i with
      | some (.defCleanup names evalResidual snap) => do
        let e ← if evalResidual then evalFrameResidual nat fuel e i else pure e
        if e.defs.flowing then pure e else
        closeFrame nat fuel { e with defs := popNames e.defs names snap, tape := removeAt e.tape i } openIdx (closeIdx - 1)
      | _ => pure e
    | none =>
      let checkAt := (List.range closeIdx).find? fun i =>
        i > openIdx && (match e.tape[i]? with | some (.returnCheck _ _ _) => true | _ => false)
      match checkAt with
      | some i =>
        match e.tok? i with
        | some (.returnCheck returns unnamed patterns) =>
          let tape := removeAt e.tape i
          let results := (tape.drop (openIdx + 1)).take (closeIdx - 1 - (openIdx + 1))
          -- THE DECLARED RETURNS ARE THE TOP of what the frame left. Beneath
          -- them, up to as many values as the frame had unnamed arguments
          -- are let pass and DISCARDED, from the bottom — whatever they are
          -- (NUR404, as the kernel has it: a value the body computed goes
          -- the same way as an argument it left untouched)
          let extra := results.length - returns.length
          if results.length < returns.length || extra > unnamed then
            Engine.err "type_error" "return count"
          -- a return declared as a NAMED type asks the type whether the
          -- value is a member, as a slot of that type does
          else if !(((results.drop extra).zip returns).zipIdx.all fun ((v, t), k) =>
              match patterns[k]? with
              | some (some (.named path body)) => memberMatch (.named path body) v
              | _ => v.is t) then
            Engine.err "type_error" "return type"
          -- a return declared as more than a type is checked against its
          -- pattern too: the type of a union is Any, and admits everything
          else if !((((results.drop extra).zip patterns).all fun (v, pat) =>
              match pat with
              | some (.named _ _) => true
              | some p => unifies p v
              | none => true)) then
            Engine.err "type_error" "return pattern"
          else pure { e with tape := splice tape (openIdx + 1) extra [] }
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
          -- a signal the literal let out: it has no value, and the sweep
          -- stops for the run to resolve it
          if defs.flowing then pure { e with defs := defs } else
          sweepRange nat fuel { e with tape := setAt e.tape i v', defs := defs } (i + 1) hi
        else sweepRange nat fuel { e with tape := setAt e.tape i (resolveInert e.defs v) } (i + 1) hi
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
    let (args, defs) ← evalArgs nat fuel e.defs (rawArgs.zipIdx.map fun (v, i) => (v, s.raw i))
    let e := { e with defs := defs }
    -- a signal an argument literal let out: the call is abandoned
    if e.defs.flowing then pure e else
    if s.fullStack then
      let base := scopeStart e.tape e.ptr
      let stack := (List.range e.ptr).filterMap fun i =>
        if i < base || indices.contains i then none
        else match e.tape[i]? with
          | some v => if v.isForward || v.isOpenParen || v.isLoopToken then none else some v
          | none => none
      match s.impl with
      | .native key =>
        match nat.lookup key with
        | some (.fullStack f) => do
          -- the replacement stands for EVERYTHING from the scope's start
          -- to the word: a loop's mark standing in that stretch goes with
          -- it, and the loop with the mark (NUR400, as the kernel has it)
          let results ← f args stack
          pure { e with tape := splice e.tape base (e.ptr + 1 - base) results, ptr := base }
        | _ => Engine.err "internal_error" ("no full-stack handler for " ++ name)
      | .boru _ _ _ _ _ => Engine.err "internal_error" "a boru body is not a full-stack handler"
      | .usurp _ _ => Engine.err "internal_error" "a usurp wrapper is not a full-stack handler"
    else do
      let (results, defs) ←
        match s.impl with
        | .native key =>
          match nat.lookup key with
          | some (.plain f) => f args e.defs
          -- a looping word: its results are the driver's, once every
          -- iteration has run
          | some (.driven d) => driveLoop nat fuel e.defs d args 0 []
          | _ => Engine.err "internal_error" ("no handler for " ++ name)
        | .usurp orig b =>
          -- the original, re-dispatched in a group of its own with the
          -- arguments reversed and laid out for ITS barrier: what it
          -- takes from the stack before it, what it takes forward after
          let k := args.length - min b args.length
          pure (.openParen :: args.take k ++ [orig] ++ (args.drop k).reverse ++ [.closeParen], e.defs)
        | .boru body captured anonymous frameState returnPatterns =>
          -- a call frame: bind the named parameters, splice the body in
          -- a paren group closed by the cleanup marker and return check
          -- a NAMED parameter binds its argument; an UNNAMED one's argument
          -- is spliced at the head of the frame, in signature order, as
          -- stack data the step loop skips
          let pairs := s.params.zip args
          let named := pairs.filter fun (p, _) => !p.name.isEmpty
          let unnamed := (pairs.filter fun (p, _) => p.name.isEmpty).map (·.2)
          let names := named.map fun (p, _) => p.name
          -- the baseline is recorded BEFORE the frame binds anything, so
          -- what it binds is what a function built in its body captures;
          -- and the call's arguments, all of them, go on the args stack
          -- WHEN the frame is asked whether it records its entry state: an
          -- installed function was asked when it was made; a function
          -- value is asked now, against the bindings of the call —
          -- before it binds anything of its own
          let needsState := frameState.getD (bodyNeedsFrameState e.defs body)
          let defs := (e.defs.pushBaseline needsState).pushArgs args
          -- a closure's captures go in first: a parameter of the same
          -- name shadows one
          let defs := captured.foldl (fun d (nm, v) => d.push nm v) defs
          -- a list argument is bound QUOTED: the body reads it as data
          let bind (a : Value) : Value := if a.isListShape && !a.isQuoted then .quoted a else a
          let defs := named.foldl (fun d (p, a) => d.push p.name (bind a)) defs
          -- THE ENTRY SNAPSHOT, taken once the captures and parameters
          -- are bound: what the body binds above it ends with the frame.
          -- A leaf frame takes none (the same test as its baseline)
          let snap := if needsState then some defs.depths else none
          -- THE RESIDUAL RULE (core/go `ResidualEvalsInFrame`): the pending
          -- containers a body leaves are evaluated in the frame, while its
          -- bindings are live — unless the function is ANONYMOUS and its
          -- body is one bare token: that container leaves the frame as it
          -- is, and is evaluated by whatever consumes it, where the
          -- frame's names are no longer bound
          let evalResidual := !anonymous || bodyEvalsResidual body
          let tail := Value.defCleanup (captured.map (·.1) ++ names) evalResidual snap
            :: (if s.returns.isEmpty then [] else [.returnCheck s.returns unnamed.length returnPatterns])
          pure (.frameOpen unnamed.length :: unnamed ++ body ++ tail ++ [.closeParen], defs)
      let e := { e with defs := defs }
      -- a driven loop a signal passed THROUGH is abandoned where its word
      -- stands: the loop that takes the signal discards the stretch
      let driven := match s.impl with
        | .native key => (match nat.lookup key with | some (.driven _) => true | _ => false)
        | _ => false
      if driven && e.defs.flowing then pure e else
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
    Nat → Defs → List (Value × Raw) → Except BoruError (List Value × Defs)
  | 0, _, _ => Engine.outOfFuel
  | _ + 1, defs, [] => pure ([], defs)
  | fuel + 1, defs, (v, raw) :: vs => do
    -- THE NO-EVALUATION SLOTS (core/go `NoEvalArgs`, `NoEvalMapArgs`): a
    -- pending list at a slot that takes lists raw, or a pending map at
    -- one that takes maps raw, is handed over as it was written — no
    -- longer pending, so nothing evaluates it later either. The two modes
    -- are separate: a map at a list-raw slot is evaluated, and a list at
    -- a map-raw slot is
    let kept : Option Value := match v with
      | .list elems true => if raw.list then some (.list elems false) else none
      | .map entries true => if raw.map then some (.map entries false) else none
      | _ => none
    let (v', defs) ← match kept with
      | some k => pure (k, defs)
      | none => if v.isPendingContainer then autoEval nat fuel defs v else pure (v, defs)
    if defs.flowing then pure ([], defs) else do
    -- a slot with either mode leaves an inert type shape unresolved too,
    -- whatever the value is. The shape is resolved HERE, in the
    -- argument's own turn: a later argument that rebinds a type the
    -- shape names does not change what this one resolved to
    let here := if raw.list || raw.map then v' else resolveInert defs v'
    let (vs', defs) ← evalArgs nat fuel defs vs
    pure (here :: vs', defs)

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
    -- a signal a member let out: the literal has no value
    if defs.flowing then pure ([], defs) else do
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
    -- a signal no loop in the region took: a literal is not a loop. The
    -- region is abandoned whole — the frames open in it are closed — and
    -- has no value; the run that holds it resolves the signal
    if e.defs.flowing then pure ([], unwindFrames e.defs (e.tape.drop e.ptr)) else
    let unclosed := e.sealed
    let e ← sweepRange nat fuel e 0 e.tape.length
    if e.defs.flowing then pure ([], e.defs) else
    if unclosed || e.tape.any Value.isOpenParen then
      Engine.err "syntax_error" "unmatched opening parenthesis"
    else pure (e.tape, e.defs)

/-- A DRIVEN LOOP (core/go/loop.go `StartLoop`, `stepMoveDriven`). Each
    iteration is a sealed region of its own: the inputs the driver names,
    inert, and then the body, stepped where the loop's word stands. The
    region's residual — everything it left, an input the body did not
    take included — goes to the driver and is never stepped again. When
    the driver has no iteration left, what it finishes with stands in the
    loop's place and is stepped as any handler's results are. -/
def driveLoop (nat : Natives) :
    Nat → Defs → Driver → List Value → Nat → List Value → Except BoruError (List Value × Defs)
  | 0, _, _, _, _, _ => Engine.outOfFuel
  | fuel + 1, defs, d, args, it, acc => do
    match ← d.next args it with
    | none =>
      let results ← d.finish args acc
      pure (results, defs)
    | some (inputs, body) =>
      let (residual, defs) ← runRegion nat fuel defs inputs body
      -- a signal passes THROUGH a driven loop to the counted or
      -- conditional loop around it: the driven loop is abandoned
      if defs.flowing then pure ([], defs) else
      let acc ← d.collect args it acc residual
      driveLoop nat fuel defs d args (it + 1) acc

/-- One iteration's region: the body sees its inputs and nothing beneath
    them, and steps from AFTER them — an input is data, whatever it is. The
    region's close resolves what is still parked in it, and its collection
    evaluates a pending container left there. Unlike a container literal's
    region, a loop's is not refused for an open paren left unmatched: the
    marker is collected with everything else. -/
def runRegion (nat : Natives) :
    Nat → Defs → List Value → List Value → Except BoruError (List Value × Defs)
  | 0, _, _, _ => Engine.outOfFuel
  | fuel + 1, defs, inputs, body => do
    let e ← steps nat fuel
      { tape := .openParen :: inputs ++ body ++ [.closeParen], ptr := 1 + inputs.length,
        defs := defs, sealed := true }
    if e.defs.flowing then pure ([], unwindFrames e.defs (e.tape.drop e.ptr)) else
    let e ← sweepRange nat fuel e 0 e.tape.length
    pure (e.tape, e.defs)

/-- Run a program to its residual: step to the end, resolve what is
    still parked, refuse an unmatched open paren, then evaluate the
    pending containers left in the result. -/
def runProgram (nat : Natives) : Nat → Defs → List Value → Except BoruError (List Value × Defs)
  | 0, _, _ => Engine.outOfFuel
  | fuel + 1, defs, input => do
    let e ← steps nat fuel { tape := input, ptr := 0, defs := defs }
    -- a signal no loop took, wherever it was raised: there is no run
    -- left to hand it to
    let outsideLoop (d : Defs) : Except BoruError (List Value × Defs) :=
      Engine.err "flow_error" ((if d.flow? == some true then "break" else "continue") ++ " outside loop")
    if e.defs.flowing then outsideLoop e.defs else do
    let e ← resolveOrphans nat fuel e
    if e.defs.flowing then outsideLoop e.defs
    else if e.tape.any Value.isOpenParen then Engine.err "syntax_error" "unmatched opening parenthesis"
    else do
      -- a mark or a move still on the tape belongs to no loop any more
      let e := { e with tape := e.tape.filter (!·.isLoopToken) }
      let e ← sweepRange nat fuel e 0 e.tape.length
      if e.defs.flowing then outsideLoop e.defs else
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
        if e.defs.flowing then pure e else resolveOrphans nat fuel e
      | _ => pure e

def orphanSteps (nat : Natives) : Nat → Engine → Except BoruError Engine
  | 0, _ => Engine.outOfFuel
  | fuel + 1, e =>
    if e.ptr ≥ e.tape.length then pure e
    else match e.tok? e.ptr with
      | some .openParen => orphanSteps nat fuel { e with ptr := e.ptr + 1 }
      | some (.frameOpen span) => orphanSteps nat fuel { e with ptr := e.ptr + 1 + span }
      | _ => do
        let e ← stepToken nat fuel e
        if e.defs.flowing then pure e else orphanSteps nat fuel e

end

/-- The default step budget. -/
def defaultFuel : Nat := 100000

/-- Run a program over a binding store and a handler table. -/
def run (nat : Natives) (defs : Defs) (program : List Value) (fuel : Nat := defaultFuel) :
    Except BoruError (List Value) :=
  (runProgram nat fuel defs program).map (·.1)

/-- The bindings a program leaves, when it ends without an error. -/
def runDefs (nat : Natives) (defs : Defs) (program : List Value) (fuel : Nat := defaultFuel) :
    Option Defs :=
  match runProgram nat fuel defs program with
  | .ok (_, d) => some d
  | .error _ => none

end Boru.Core
