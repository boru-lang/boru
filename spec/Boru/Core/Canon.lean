/-
  Boru.Core.Canon — the canonical source form of a value.

  `canon` is the kernel's rendering contract (core/go/canon.go
  `CanonValue`): the text a value is compared by, printed as, and — for
  data — read back from. Two rules shape every arm:

    * the form keeps the TYPE: a Float always shows a point, a BigInteger
      its `0d` marker, an atom its `/q`, a typed container its tag;
    * the form is SOURCE: a marker renders as the token that wrote it
      (`end`, `)`, `/q`), never as nothing.

  ADR-015 — canon always round-trips.
-/
import Boru.Core.Value

namespace Boru.Core

/-- A string's source form. Single quotes by default; double quotes when
    the text holds a single quote and nothing else needing an escape;
    backslash escapes otherwise. -/
def canonString (s : String) : String :=
  let cs := s.toList
  let hasSingle := cs.contains '\''
  let hasEscape := cs.any (fun c => c == '\\' || c == '\n' || c == '\t' || c == '\r')
  if !hasSingle && !hasEscape then "'" ++ s ++ "'"
  else if !cs.contains '"' && !hasEscape then "\"" ++ s ++ "\""
  else
    let esc (c : Char) : String :=
      if c == '\\' then "\\\\"
      else if c == '\'' then "\\'"
      else if c == '\n' then "\\n"
      else if c == '\t' then "\\t"
      else if c == '\r' then "\\r"
      else String.ofList [c]
    "'" ++ String.join (cs.map esc) ++ "'"

/-- A map key's source form: bare when it is letters, digits and
    `_ $ - @`; a quoted string otherwise. -/
def canonKey (k : String) : String :=
  if k.isEmpty then canonString k
  else if k.toList.all (fun c => c.isAlpha || c.isDigit || c == '_' || c == '$' || c == '-' || c == '@')
  then k else canonString k

/-- A word's call-site modifiers, in their one written order. -/
def canonWordModifiers (w : WordInfo) : String :=
  let n := match w.argCount with | some k => toString k | none => ""
  let fs := if w.forceForward then "f" else if w.forceStack then "s" else ""
  let u := if w.forceUsurp then "u" else ""
  let v := if w.forceVal then "v" else ""
  let all := n ++ fs ++ u ++ v
  if all.isEmpty then "" else "/" ++ all

def canonWord (w : WordInfo) : String := w.name ++ canonWordModifiers w

mutual

/-- The canonical source form of one value. -/
def canon : Value → String
  | .none => "none"
  | .closeParen => ")"
  | .openParen => "("
  | .frameOpen _ => "("
  | .dispatchMod val => if val then "/v" else "/q"
  | .endMark => "end"
  | .typeLit t => leafOf t
  -- a type a program defined renders as its name
  | .named path _ => leafOf path
  | .typeBind node => canon node
  | .bigInt n => canonBigInt n
  | .bigDec d => d.canon
  | .int n => toString n
  | .float f => f.canon
  | .str s => canonString s
  | .bool b => if b then "true" else "false"
  | .atom name => name ++ "/q"
  | .list elems _ => "[" ++ " ".intercalate (canonChildren elems) ++ "]"
  | .typedList child elems =>
    "[" ++ " ".intercalate ((":" ++ canonTag child) :: canonChildren elems) ++ "]"
  | .map entries _ => "{" ++ " ".intercalate (canonEntries entries) ++ "}"
  | .typedMap child entries =>
    "{" ++ " ".intercalate ((":" ++ canonTag child) :: canonEntries entries) ++ "}"
  | .word w => canonWord w
  | .parenExpr items => "(" ++ " ".intercalate (canonGroup items) ++ ")"
  -- A SPLICE MARKER has no source spelling. The kernel's canon has no arm
  -- for one and falls back to the engine's own value print — a debug
  -- spelling (NUR416) — with the payload printed the same way
  | .splice payload => "word()({" ++ debugValue payload ++ "})"
  | .disjunct alts => " tor ".intercalate (canonAlts alts)
  | .fn _ sigs _ => "fn [" ++ " ".intercalate (canonSigs sigs) ++ "]"
  | .forward info _ => "__FW(" ++ info.funcName ++ ")"
  | .defCleanup _ _ _ => "__DC"
  | .returnCheck _ _ _ => "__RC"
  | .mark _ => "__MK"
  | .move _ _ => "__MV"
  -- a quoted LIST reads back through `quote`; on anything else the
  -- quoting is invisible
  | .quoted (.list elems e) => "(quote " ++ canon (.list elems e) ++ ")"
  | .quoted (.typedList c elems) => "(quote " ++ canon (.typedList c elems) ++ ")"
  | .quoted v => canon v
  -- the application mark is spent where it is stepped, and never printed
  | .applied v => canon v

/-- A typed container's tag: an unresolved type name is still a bare word
    there (ADR-012 rule 4). -/
def canonTag : Value → String
  | .word w => canonWord w
  | .disjunct alts => "(" ++ " tor ".intercalate (canonAlts alts) ++ ")"
  | v => canon v

/-- The engine's own value print (core/go `Value.String`,
    `kernelFormatDefault`), for the one place canon falls back to it. It
    is not canon: a word prints as `word(name)`, without its modifiers; a
    string is put between single quotes as it is, nothing escaped; a
    paren expression is `paren([…])`; an atom is its bare name; and a
    list or map prints its members the same way. What it prints for a
    function is not stated — that falls to canon here, and is wrong. -/
def debugValue : Value → String
  | .word w => "word(" ++ w.name ++ ")"
  | .str s => "'" ++ s ++ "'"
  | .atom name => name
  | .parenExpr items => "paren([" ++ " ".intercalate (debugList items) ++ "])"
  | .list elems _ => "[" ++ " ".intercalate (debugList elems) ++ "]"
  | .map entries _ => "{" ++ " ".intercalate (debugEntries entries) ++ "}"
  | .quoted v => debugValue v
  | v => canon v

def debugList : List Value → List String
  | [] => []
  | v :: vs => debugValue v :: debugList vs

def debugEntries : List (String × Value) → List String
  | [] => []
  | (k, v) :: es => (canonKey k ++ ":" ++ debugValue v) :: debugEntries es

/-- Elements of a container: a disjunct element is parenthesised so it
    reads back as one element. -/
def canonChildren : List Value → List String
  | [] => []
  | .disjunct alts :: vs => ("(" ++ " tor ".intercalate (canonAlts alts) ++ ")") :: canonChildren vs
  | v :: vs => canon v :: canonChildren vs

/-- A plain token sequence (a fn body). -/
def canonSeq : List Value → List String
  | [] => []
  | v :: vs => canon v :: canonSeq vs

/-- The tokens of a paren expression. Inside a group the collection
    barrier is written `;` — its punctuation spelling — where on its own
    it renders as the word `end`. -/
def canonGroup : List Value → List String
  | [] => []
  | .endMark :: vs => ";" :: canonGroup vs
  | v :: vs => canon v :: canonGroup vs

def canonEntries : List (String × Value) → List String
  | [] => []
  | (k, .disjunct alts) :: es =>
    (canonKey k ++ ":(" ++ " tor ".intercalate (canonAlts alts) ++ ")") :: canonEntries es
  | (k, v) :: es => (canonKey k ++ ":" ++ canon v) :: canonEntries es

/-- A disjunct's alternatives: an atom shows its bare name, a type its
    leaf. -/
def canonAlts : List Value → List String
  | [] => []
  | .atom name :: vs => name :: canonAlts vs
  | v :: vs => canon v :: canonAlts vs

def canonSigs : List Sig → List String
  | [] => []
  | .mk params returns _ impl _ :: ss =>
    ("[" ++ " ".intercalate (canonParams params) ++ "]["
      ++ " ".intercalate (returns.map leafOf) ++ "]["
      ++ " ".intercalate (canonImpl impl) ++ "]") :: canonSigs ss

def canonParams : List Param → List String
  | [] => []
  -- an unnamed parameter has no name before the colon: `:Integer`
  | .mk name type _ _ _ _ _ :: ps =>
    (name ++ ":" ++ leafOf type) :: canonParams ps

def canonImpl : Impl → List String
  | .native _ => []
  | .usurp _ _ => []
  | .boru body _ _ _ _ => canonSeq body

end

/-- The canonical form of a result: its values, space-separated. -/
def canonValues (vs : List Value) : String := " ".intercalate (vs.map canon)

end Boru.Core
