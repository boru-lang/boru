/-
  Boru.Core.Theorems.Splice — splice markers.

  A SPLICE MARKER wraps a payload. Stepped with nothing waiting for it,
  it FIRES: the payload takes its place, unevaluated — a plain list gives
  its elements, any other value itself — and what took its place is
  stepped from there, against the live stack. A marker whose payload is
  words is a macro.

  With a word waiting for an argument a marker does not fire: it is a
  value, collected if the slot takes it, and it is not its payload.

  A NAME bound to a marker fires where the name is stepped. Where a word
  is collecting, a name bound to a DATA splice — a payload with no word,
  paren expression or marker at its top level — is read as the group
  `(w)`: `f w` is `f (w)`, and the values fill as many slots as there
  are. A code-bearing splice is never grouped. Three kinds of slot take
  something else: a name-capturing slot and a form slot take the word, a
  raw-paren slot takes the group as code, and a binder takes the marker.

  One thing is stated as the kernel has it and not as the rule, and is
  marked: the binder is known by its NAME (NUR415).

  The theorems run the machine over the corpus fixture and are
  discharged by `native_decide` over the stated domains.
-/
import Boru.Core.Theorems.Counted

namespace Boru.Core.SpliceThm
open Boru.Core
open Boru.Core.Counted (w ws outWith out ints)

def pend (vs : List Value) : Value := .list vs true
def quot (vs : List Value) : Value := .list vs false
def paren (vs : List Value) : Value := .parenExpr vs
def one (v : Value) : String := canonValues [v]
def refused : String := "ERROR:signature_error"

def pairs : List (Int × Int) := [(1, 2), (5, -6), (0, 9), (-4, -4)]

/-- A marker of two Integers. -/
def two (a b : Int) : Value := .splice (quot [.int a, .int b])

/-- The fixture with `w` bound to a marker of `a b`, and `inc` to the
    macro `addq 1`. -/
def withW (a b : Int) : Defs :=
  (Boru.Corespec.fixtureDefs.push "w" (two a b)).push "inc" (.splice (quot [w "addq", .int 1]))

/-- `f ( w )`, as tokens. -/
def grouped (f : String) : List Value := [w f, .openParen, w "w", .closeParen]

/-! ## A marker fires where it is stepped -/

/-- A MARKER IS REPLACED BY ITS PAYLOAD: a list's elements, pending or
    quoted — the list only carries them — and any other value itself. A
    typed list is one value, an empty list nothing. -/
theorem a_marker_is_replaced_by_its_payload :
    (pairs.all fun (a, b) =>
      out [two a b] == ints [a, b] &&
      out [.splice (pend [.int a, .int b])] == ints [a, b] &&
      out [.splice (.int a)] == ints [a] &&
      out [.splice (.map [("a", .int a)] false)] == one (.map [("a", .int a)] false) &&
      out [.splice (.typedList (.typeLit TInteger) [.int a, .int b])]
        == one (.typedList (.typeLit TInteger) [.int a, .int b]) &&
      out [.splice (quot [])] == "" &&
      out [.int 9, .splice (quot []), .int 8] == ints [9, 8]) = true := by
  native_decide

/-- WHAT IS SPLICED IS STEPPED, AGAINST THE LIVE STACK: a spliced word
    takes what stands before the marker and what follows it; a marker
    among the elements fires in turn; a paren expression runs; an unbound
    word is an error where it is stepped. -/
theorem what_is_spliced_is_stepped :
    (pairs.all fun (a, b) =>
      out [.splice (quot [w "addq", .int a, .int b])] == ints [a + b] &&
      out [.int a, .splice (quot [w "addq", .int b])] == ints [a + b] &&
      out [.splice (quot [w "addq", .int b]), .int a] == ints [a + b] &&
      out [.splice (quot [two a b, .int 3])] == ints [a, b, 3] &&
      out [.splice (.splice (quot [.int a, .int b]))] == ints [a, b] &&
      out [.splice (paren [w "addq", .int a, .int b])] == ints [a + b] &&
      out [.splice (quot [w "nosuch"])] == "ERROR:undefined_word") = true := by
  native_decide

/-! ## A marker a word is collecting is a value -/

/-- WITH A WORD WAITING THE MARKER IS A VALUE. It is collected as it is,
    and fires wherever it is next stepped with nothing waiting — each
    copy a word hands back; handed to a word that keeps it, never. It is
    NOT its payload: a typed slot refuses it. -/
theorem a_collected_marker_is_a_value :
    (pairs.all fun (a, b) =>
      out [w "dupq", two a b] == ints [a, b, a, b] &&
      out [w "bothq", .int 9, two a b] == ints [a, b] &&
      out [w "bothq", two a b, .int 9] == ints [9] &&
      out [w "addq", two a b] == refused &&
      out [w "negq", .splice (.int a)] == refused &&
      -- before the word it has fired, and its values are on the stack
      out [two a b, w "addq"] == ints [a + b]) = true := by
  native_decide

/-! ## A name bound to a marker -/

/-- A NAME BOUND TO A MARKER FIRES WHERE IT IS STEPPED, each time; and a
    marker whose payload is words is a MACRO, run against the live stack
    each time. An arity or stack modifier changes nothing — there is no
    call — and `/u` wants a function. -/
theorem a_bound_marker_fires_each_time :
    (pairs.all fun (a, b) =>
      let d := withW a b
      outWith d [w "w"] == ints [a, b] &&
      outWith d [w "w", w "w"] == ints [a, b, a, b] &&
      outWith d [w "w", w "addq"] == ints [a + b] &&
      outWith d [.int a, w "inc", w "inc", w "inc"] == ints [a + 3] &&
      outWith d [.word { name := "w", argCount := some 2 }] == ints [a, b] &&
      outWith d [ws "w"] == ints [a, b] &&
      outWith d [.word { name := "w", forceUsurp := true }] == "ERROR:illegal_ref") = true := by
  native_decide

/-- `/v` IS THE MARKER, AS A VALUE. With nothing waiting it is placed
    and NOT fired — it stays on the stack as a marker; collected, it is
    a value like a written marker. -/
theorem a_reference_to_a_marker_is_the_marker :
    (pairs.all fun (a, b) =>
      let d := withW a b
      let wv : Value := .word { name := "w", forceVal := true }
      outWith d [wv] == one (two a b) &&
      outWith d [w "dupq", wv] == ints [a, b, a, b] &&
      outWith d [w "bothq", wv, .int 9] == ints [9] &&
      outWith d [w "addq", wv] == refused) = true := by
  native_decide

/-! ## `f w` is `f (w)` -/

/-- `f w` IS `f (w)` FOR A WORD BOUND TO A DATA SPLICE. Where a word is
    collecting an argument, the name answers exactly as the group does —
    whatever the word's arity, and whatever is left over. -/
theorem a_data_splice_word_is_its_group :
    (pairs.all fun (a, b) =>
      let d := withW a b
      ["addq", "dupq", "bothq", "unifyq", "negq", "rawlq", "boomq"].all fun f =>
        outWith d [w f, w "w"] == outWith d (grouped f) &&
        outWith d [w f, w "w", .int 9] == outWith d (grouped f ++ [.int 9])) = true := by
  native_decide

/-- So the values fill AS MANY SLOTS AS THERE ARE: two fill a two-slot
    word; a one-slot word takes the first, and the second is left after
    its result; a group that leaves nothing is no operand. -/
theorem the_values_fill_as_many_slots_as_there_are :
    (pairs.all fun (a, b) =>
      let d := withW a b
      outWith d [w "addq", w "w"] == ints [a + b] &&
      outWith d [w "dupq", w "w"] == ints [a, a, b] &&
      outWith d [w "bothq", w "w"] == ints [b] &&
      outWith (d.push "w" (.splice (quot [.int a]))) [w "addq", w "w", .int b] == ints [a + b] &&
      outWith (d.push "w" (.splice (quot [.int a]))) [.int b, w "addq", w "w"] == ints [a + b] &&
      outWith (d.push "w" (.splice (.int a))) [w "negq", w "w"] == ints [-a] &&
      outWith (d.push "w" (.splice (quot []))) [w "negq", w "w"] == "ERROR:no_value_error") = true := by
  native_decide

/-- A CODE-BEARING SPLICE IS NEVER GROUPED. A payload with a word, a paren
    expression or a marker at its top level is a macro: collected, the
    marker is a value, and a typed slot refuses it — where the group,
    written out, would have run it and handed over its result. -/
theorem a_code_splice_is_never_grouped :
    (pairs.all fun (a, _) =>
      let d := Boru.Corespec.fixtureDefs.push "x" (.int a)
      [quot [w "x"], paren [w "x"], quot [.splice (quot [.int a])]].all fun payload =>
        let d := d.push "w" (.splice payload)
        outWith d [w "negq", w "w"] == refused &&
        outWith d (grouped "negq") == ints [-a]) = true := by
  native_decide

/-! ## The slots that take something else -/

/-- A NAME-CAPTURING SLOT AND A FORM SLOT TAKE THE WORD; A RAW-PAREN SLOT
    TAKES THE GROUP AS CODE. `qanyq` hands the word back, and it then
    fires; `formq` and `rawpq` hand back what they captured in a list. -/
theorem the_capturing_slots_take_the_word_or_the_group :
    (pairs.all fun (a, b) =>
      let d := withW a b
      outWith d [w "qanyq", w "w"] == ints [a, b] &&
      outWith d [w "formq", w "w"] == one (quot [w "w"]) &&
      outWith d [w "rawpq", w "w"] == one (quot [paren [w "w"]]) &&
      outWith d [w "tyq", w "w"] == refused) = true := by
  native_decide

/-- A KEYWORD SLOT CAPTURES ONLY ITS KEYWORD. An overload whose slot
    admits the one word `kw` does not make a capture of every word
    written at its position: beside it, a word bound to a data splice is
    still the group, and its value takes the overload that fits it. A
    slot that captures ANY name takes the word. -/
theorem a_keyword_slot_captures_only_its_keyword :
    (pairs.all fun (a, b) =>
      let d := withW a b
      let f (first : String) (call : List Value) : List Value :=
        let spec := quot [quot [w first], quot [.typeLit TAny], quot [.str "name"],
                          quot [.typeLit TInteger], quot [.typeLit TAny], quot [.str "integer"]]
        [.endMark, w "def", w "f", .openParen, w "fn", spec, .closeParen, .endMark] ++ call
      let d1 := d.push "w" (.splice (quot [.int a]))
      outWith d1 (f "n:kw/q" [w "f", w "w"]) == one (.str "integer") &&
      outWith d1 (f "n:kw/q" [w "f", w "kw"]) == one (.str "name") &&
      outWith d1 (f "n:Atom/q" [w "f", w "w"]) == one (.str "name") &&
      -- and a function word that is not the keyword is still a boundary
      outWith d1 (f "n:kw/q" [w "f", w "negq", .int b]) == "ERROR:signature_error") = true := by
  native_decide

/-- A BINDER TAKES THE MARKER: `def y w` rebinds it, and `y` is the same
    splice under another name — where `def y ( w )` binds the group's
    value. -/
theorem a_binder_takes_the_marker :
    (pairs.all fun (a, b) =>
      let d := withW a b
      outWith d [.endMark, w "def", w "y", w "w", .endMark, w "y"] == ints [a, b] &&
      outWith d [.endMark, w "def", w "y", w "w", .endMark, w "addq", w "y"] == ints [a + b] &&
      outWith d [.endMark, w "def", w "y", .openParen, w "w", .closeParen, .endMark, w "y"] == ints [b, a]) = true := by
  native_decide

/-- AS THE KERNEL HAS IT (NUR415): THE BINDER IS KNOWN BY ITS NAME. The
    exemption is the word named `def`. Another word that binds — and
    `def`'s own function under another name — is not it: it is handed the
    group's first value, and the second is left on the stack. -/
theorem nur415_the_binder_is_known_by_its_name :
    (pairs.all fun (a, b) =>
      let d := withW a b
      outWith d [.endMark, w "bindq", w "y", w "w", .endMark, w "y"] == ints [b, a] &&
      -- `def mydef def/v`: the same function, called by another name
      outWith d [.endMark, w "def", w "mydef", .word { name := "def", forceVal := true }, .endMark,
                 w "mydef", w "y", w "w", .endMark, w "y"] == ints [b, a]) = true := by
  native_decide

/-! ## In a frame -/

/-- FRAME STATE SEES THROUGH A MARKER. A body that names only `w` names
    what `w`'s payload names: if that is a binding word, the frame
    records its entry state, and what the spliced word binds ends with
    the call — through as many markers as it takes. A payload whose word
    the frame analysis does not know leaves its binding in place. -/
theorem frame_state_sees_through_a_marker :
    (let spec := quot [quot [w "n:Any"], quot [.typeLit TAny], quot [w "w", .endMark, w "y"]]
     let call := [.endMark, w "def", w "f", .openParen, w "fn", spec, .closeParen, .endMark,
                  w "f", .int 0, w "y"]
     let withPayload (payload : List Value) : Defs :=
       Boru.Corespec.fixtureDefs.push "w" (.splice (quot payload))
     outWith (withPayload [w "def", w "y", .int 3]) call == "ERROR:undefined_word" &&
     outWith ((withPayload [w "v"]).push "v" (.splice (quot [w "def", w "y", .int 3]))) call
       == "ERROR:undefined_word" &&
     outWith (withPayload [w "bindq", w "y", .int 3]) call == ints [3, 3]) = true := by
  native_decide

end Boru.Core.SpliceThm
