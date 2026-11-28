/-
  Boru.Core.Theorems.FnSpec — a function's specification, read and
  installed.

  THE READER turns a written list — parameters, returns, a body, a triple
  per overload — into signatures. Each element of a parameter list says
  one thing: a name and a type, a type alone, a value the slot admits
  exactly, where the forward barrier stands, that the parameter before it
  may be left out. A declared return is a type, or more than a type, and
  then what the lattice cannot say is a pattern the returned value must
  unify with.

  THE INSTALL is what binding a function by name does to it. Optional
  parameters become signatures, a redefinition of the same parameter
  types replaces, and every body is asked then whether its frames record.

  Three things are stated as the kernel has them, and marked: the reader
  takes fewer declarations where no name is written (NUR409); an optional
  parameter whose type has no base value is silently not optional
  (NUR410); and the value `none` in a declaration constrains nothing
  (NUR408).

  The first theorems read the reader's own output; the rest run the
  machine over the corpus fixture — `fn spec` answers the function as the
  reader leaves it, `def name value` installs — and all are discharged by
  `native_decide` over the stated domains.
-/
import Boru.Core.Theorems.Tail

namespace Boru.Core.FnSpecThm
open Boru.Core
open Boru.Core.Counted (w ws outWith ints)

/-- A written triple. -/
def triple (params returns body : List Value) : List Value :=
  [.list params false, .list returns false, .list body false]

/-- The signatures the reader makes of a specification, or nothing. -/
def read (spec : List Value) : List Sig :=
  match FnSpec.parseTriples Boru.Corespec.fixtureDefs spec with
  | .ok sigs => sigs
  | .error _ => []

def refused (spec : List Value) : Bool :=
  match FnSpec.parseTriples Boru.Corespec.fixtureDefs spec with
  | .ok _ => false
  | .error _ => true

/-- `; def name ( fn spec ) ;` -/
def defn (name : String) (spec : List Value) : List Value :=
  [.endMark, w "def", w name, .openParen, w "fn", .list spec false, .closeParen, .endMark]

def out (tape : List Value) : String := outWith Boru.Corespec.fixtureDefs tape

def names (s : Sig) : List String := s.params.map Param.name
def types (s : Sig) : List TypeId := s.params.map Param.type
def patterns (s : Sig) : List (Option String) := s.params.map fun p => p.pattern.map canon
def optionals (s : Sig) : List Bool := s.params.map Param.optional
def quotes (s : Sig) : List Bool := s.params.map Param.quote

/-! ## The reader -/

/-- A PARAMETER LIST IS READ ELEMENT BY ELEMENT. `name:Type` is a named
    parameter; `|` sets the barrier where it stands — with none, every
    parameter is forward; `name?:Type` and a `?` after a parameter both
    make it optional; `/q` after the type makes it capture a name. -/
theorem a_parameter_list_is_read_element_by_element :
    (let s := (read (triple [w "n:Integer", w "|", w "m?:String", w "k:Atom/q", w "?"] [] [])).headD
        (.mk [] [] 0 (.native "") false)
     names s == ["n", "m", "k"] &&
     types s == [TInteger, TString, TAtom] &&
     s.barrier == 1 &&
     optionals s == [false, true, true] &&
     quotes s == [false, false, true] &&
     -- with no `|`, all forward; a leading one, none
     ((read (triple [w "n:Integer", w "m:Integer"] [] [])).map Sig.barrier) == [2] &&
     ((read (triple [w "|", w "n:Integer", w "m:Integer"] [] [])).map Sig.barrier) == [0]) = true := by
  native_decide

/-- A BARE TYPE IS AN UNNAMED PARAMETER, AND A VALUE IS A PARAMETER THAT
    ADMITS EXACTLY IT: the value's family is the slot's type and the value
    its pattern. A keyword — `/q` after a name that is no type — is an
    Atom slot whose pattern is that name. -/
theorem a_type_or_a_value_alone_is_an_unnamed_parameter :
    (let s := (read (triple [.typeLit TInteger, w "Number", .int 7, .str "a", w "kw:on/q"] [] [])).headD
        (.mk [] [] 0 (.native "") false)
     names s == ["", "", "", "", "kw"] &&
     types s == [TInteger, "Scalar/Number", TInteger, TString, TAtom] &&
     patterns s == [none, none, some "7", some "'a'", some "on/q"]) = true := by
  native_decide

/-- A DECLARATION IS A TYPE AND, WHERE IT SAYS MORE, A PATTERN
    (`ResolveSigType`). A type is itself; a name the type it names; a
    scalar its family with itself as the pattern; a union `Any` with the
    union as the pattern; a string that could never name a type the
    string, and one shaped like a type name that names none an error. -/
theorem a_declaration_is_a_type_and_perhaps_a_pattern :
    (let r (v : Value) : Option (TypeId × Option String) :=
       match FnSpec.resolveSigType Boru.Corespec.fixtureDefs v with
       | .ok (t, p) => some (t, p.map canon)
       | .error _ => none
     r (.typeLit TString) == some (TString, none) &&
     r (w "Integer") == some (TInteger, none) &&
     r (.str "Integer") == some (TInteger, none) &&
     r (.int 7) == some (TInteger, some "7") &&
     r (.bool true) == some (TBoolean, some "true") &&
     r (.str "ok") == some (TString, some "'ok'") &&
     r (.disjunct [.typeLit TInteger, .typeLit TString]) == some (TAny, some "Integer tor String") &&
     r (.list [.int 1] false) == some (TList, some "[1]") &&
     r (.str "Nosuch") == none &&
     r (w "nosuch") == none) = true := by
  native_decide

/-- A PART THAT IS NOT A LIST STANDS FOR THE LIST OF ITSELF, several
    triples are several signatures, and a list that is no whole number of
    triples reads as the whole ones it holds. -/
theorem triples_and_their_short_forms :
    (canon (.fn "" (read [w "n:Integer", .typeLit TInteger, w "n"]) 0)
        == canon (.fn "" (read (triple [w "n:Integer"] [.typeLit TInteger] [w "n"])) 0) &&
      (read (triple [w "n:Integer"] [] [.int 1] ++ triple [w "n:String"] [] [.int 2])).length == 2 &&
      (read (triple [w "n:Integer"] [] [.int 1] ++ [.list [] false])).length == 1) = true := by
  native_decide

/-- A TYPED LIST IS READ AS THE LIST OF ITS RETAINED ELEMENTS, in every
    part of a specification that may be a list (core/go `AsList`): as the
    parameters, as the returns and as the body it reads exactly as the
    plain list of the same elements does — and with nothing retained it is
    the empty list. -/
theorem a_typed_list_is_the_list_of_its_elements :
    (let typed (elems : List Value) : Value := .typedList (.typeLit TInteger) elems
     let same (a b : List Value) : Bool :=
       canon (.fn "" (read a) 0) == canon (.fn "" (read b) 0) && (read a).length == 1
     same [typed [w "n:Integer"], .list [.typeLit TInteger] false, .list [w "n"] false]
          (triple [w "n:Integer"] [.typeLit TInteger] [w "n"]) &&
     same [.list [w "n:Integer"] false, typed [.typeLit TInteger], .list [w "n"] false]
          (triple [w "n:Integer"] [.typeLit TInteger] [w "n"]) &&
     same [.list [w "n:Integer"] false, .list [.typeLit TInteger] false, typed [w "n"]]
          (triple [w "n:Integer"] [.typeLit TInteger] [w "n"]) &&
     same [typed [], typed [], typed []] (triple [] [] [])) = true := by
  native_decide

/-- NUR409, AS THE KERNEL HAS IT — recorded, not the rule. WITHOUT A NAME
    THE READER TAKES LESS. A union, a list and `none` are declarations —
    `resolveSigType` reads each — and a parameter list refuses each where
    no name is written before it. -/
theorem an_unnamed_parameter_is_read_by_a_shorter_list :
    ([Value.disjunct [.typeLit TInteger, .typeLit TString], .list [.int 1] false, .none].all fun v =>
      (match FnSpec.resolveSigType Boru.Corespec.fixtureDefs v with | .ok _ => true | .error _ => false) &&
      refused (triple [v] [] [])) = true := by
  native_decide

/-- NUR408, AS THE KERNEL HAS IT — recorded, not the rule. THE VALUE
    `none` IN A DECLARATION CONSTRAINS NOTHING: it resolves to `Any` with
    no pattern, where every other scalar is its family and itself. -/
theorem the_value_none_constrains_nothing :
    ((match FnSpec.resolveSigType Boru.Corespec.fixtureDefs .none with
      | .ok (t, p) => t == TAny && p.isNone
      | .error _ => false) &&
     (match FnSpec.resolveSigType Boru.Corespec.fixtureDefs (.typeLit TNone) with
      | .ok (t, p) => t == TNone && p.isNone
      | .error _ => false)) = true := by
  native_decide

/-- NUR411, AS THE KERNEL HAS IT — recorded, not the rule. A NAME BOUND
    TO A TYPE BODY THAT IS NOT A BARE TYPE IS A WILDCARD. Bound to a bare
    type the name is that type; bound to a union — which constrains when
    it is written in place — it is `Any` with no pattern; bound to a
    value that is no type it names nothing. -/
theorem a_name_bound_to_a_union_constrains_nothing :
    (let union := Value.disjunct [.typeLit TInteger, .typeLit TString]
     let r (bound : Value) : Option (TypeId × Bool) :=
       match FnSpec.resolveSigType (Boru.Corespec.fixtureDefs.push "x" bound) (w "x") with
       | .ok (t, p) => some (t, p.isSome)
       | .error _ => none
     r (.typeLit TInteger) == some (TInteger, false) &&
     r union == some (TAny, false) &&
     r (.typedList (.typeLit TInteger) []) == some (TAny, false) &&
     r (.int 5) == none &&
     -- written in place, the union is the pattern
     (match FnSpec.resolveSigType Boru.Corespec.fixtureDefs union with
      | .ok (t, p) => t == TAny && p.isSome
      | .error _ => false)) = true := by
  native_decide

/-! ## What the signatures do -/

/-- A VALUE WRITTEN AS A PARAMETER ADMITS EXACTLY THAT VALUE. -/
theorem a_literal_parameter_admits_exactly_it :
    ([0, 7, -3].all fun (k : Int) =>
      let f := defn "f" (triple [.int k] [.typeLit TInteger] [])
      out (f ++ [w "f", .int k]) == ints [k] &&
      out (f ++ [w "f", .int (k + 1)]) == "ERROR:signature_error") = true := by
  native_decide

/-- A RETURN THAT IS MORE THAN A TYPE IS CHECKED AGAINST ITS PATTERN. A
    union's type is `Any`, which admits everything: the union is the
    check. A literal return admits exactly the literal. -/
theorem a_return_pattern_is_checked :
    (let union := defn "f" (triple [w "n:Any"] [.disjunct [.typeLit TInteger, .typeLit TString]] [w "n"])
     let lit := defn "f" (triple [w "n:Any"] [.int 7] [w "n"])
     out (union ++ [w "f", .int 5]) == ints [5] &&
     out (union ++ [w "f", .str "a"]) == canonValues [.str "a"] &&
     out (union ++ [w "f", .bool true]) == "ERROR:type_error" &&
     out (lit ++ [w "f", .int 7]) == ints [7] &&
     out (lit ++ [w "f", .int 8]) == "ERROR:type_error") = true := by
  native_decide

/-- SEVERAL RETURNS ARE CHECKED BY POSITION, and none declared is none
    checked. -/
theorem several_returns_by_position :
    (let two := defn "f" (triple [w "n:Integer"] [.typeLit TInteger, .typeLit TString] [w "n", .str "a"])
     let swapped := defn "f" (triple [w "n:Integer"] [.typeLit TInteger, .typeLit TString] [.str "a", w "n"])
     let none := defn "f" (triple [w "n:Integer"] [] [w "n", w "n", w "n"])
     out (two ++ [w "f", .int 5]) == canonValues [.int 5, .str "a"] &&
     out (swapped ++ [w "f", .int 5]) == "ERROR:type_error" &&
     out (none ++ [w "f", .int 5]) == ints [5, 5, 5]) = true := by
  native_decide

/-! ## The install -/

/-- The signatures a name dispatches over. -/
def sigsOf (tape : List Value) (name : String) : List Sig :=
  match runDefs Boru.Corespec.fixtureNatives Boru.Corespec.fixtureDefs tape with
  | some d => (d.lookup? name).getD []
  | none => []

/-- How many bindings a name holds after a program. -/
def depthOf (tape : List Value) (name : String) : Nat :=
  match runDefs Boru.Corespec.fixtureNatives Boru.Corespec.fixtureDefs tape with
  | some d => d.depth name
  | none => 0

/-- OPTIONAL PARAMETERS BECOME SIGNATURES AT THE INSTALL. The function as
    the reader leaves it has the one signature that was written. Installed,
    it has one for every way of leaving optional parameters out — two,
    four, eight — and a call that leaves one out is given its type's base
    value. -/
theorem optional_parameters_become_signatures :
    ([1, 2, 3].all fun k =>
      let params := (["n?:Integer", "m?:String", "k?:Boolean"].take k).map w
      (read (triple params [] [])).length == 1 &&
      (sigsOf (defn "f" (triple params [] [])) "f").length == 2 ^ k) &&
    (let f := defn "f" (triple [w "n?:Integer", w "m?:String"] [] [.list [w "n", w "m"] true])
     out (f ++ [w "f"]) == canonValues [.list [.int 0, .str ""] false] &&
     out (f ++ [w "f", .int 5]) == canonValues [.list [.int 5, .str ""] false] &&
     out (f ++ [w "f", .str "a"]) == canonValues [.list [.int 0, .str "a"] false] &&
     out (f ++ [w "f", .int 5, .str "a"]) == canonValues [.list [.int 5, .str "a"] false]) = true := by
  native_decide

/-- THE ADDED SIGNATURE CALLS THE FUNCTION BY THE NAME IT WAS INSTALLED
    UNDER: its body is the name and the arguments, the one left out as its
    base value. -/
theorem the_added_signature_calls_by_name :
    (["f", "g", "area"].all fun name =>
      out (defn name (triple [w "n?:Integer"] [.typeLit TInteger] [w "n"]) ++ [.word { name := name, forceVal := true }])
        == s!"fn [[n:Integer][Integer][n] [][Integer][{name} 0]]") = true := by
  native_decide

/-- NUR410, AS THE KERNEL HAS IT — recorded, not the rule. A TYPE WITH NO
    BASE VALUE MAKES `?` DO NOTHING: no signature is added, and the call
    that leaves the parameter out matches nothing. So does a literal the
    base value is not. -/
theorem an_optional_without_a_base_value_adds_nothing :
    ([w "n?:Any", w "n?:Function", w "n?:Scalar"].all fun p =>
      (sigsOf (defn "f" (triple [p] [] [w "n"])) "f").length == 1 &&
      out (defn "f" (triple [p] [] [w "n"]) ++ [w "f"]) == "ERROR:signature_error") = true := by
  native_decide

/-- A REDEFINITION REPLACES; OTHER PARAMETER TYPES STACK. Installed twice
    with the same parameter types — whatever the names, returns and body —
    the name holds one definition; with different types, two; and a value
    that is no function is simply bound over what is there. -/
theorem a_redefinition_replaces :
    (let a := defn "f" (triple [w "n:Integer"] [] [.int 1])
     let same := defn "f" (triple [w "m:Integer"] [.typeLit TInteger] [.int 2])
     let other := defn "f" (triple [w "n:String"] [] [.int 3])
     depthOf (a ++ same) "f" == 1 &&
     out (a ++ same ++ [w "f", .int 5]) == ints [2] &&
     depthOf (a ++ other) "f" == 2 &&
     out (a ++ other ++ [w "f", .int 5, w "f", .str "x"]) == ints [1, 3] &&
     depthOf (a ++ [w "def", w "f", .int 7, .endMark]) "f" == 2) = true := by
  native_decide

/-- A HOST WORD IS LOCKED: the install's replacement passes over it. A
    function defined over a host word with the word's own parameter types
    stacks on it — the name holds both, the function is tried first, and
    unbinding the function shows the word again. A second function of
    those types replaces the first, and the word is still beneath. -/
theorem a_host_word_survives_a_redefinition :
    (let over := defn "negq" (triple [w "n:Integer"] [.typeLit TInteger] [.int 7])
     let again := defn "negq" (triple [w "m:Integer"] [.typeLit TInteger] [.int 8])
     depthOf over "negq" == 2 &&
     out (over ++ [w "negq", .int 5]) == ints [7] &&
     out (over ++ [w "undef", w "negq", w "negq", .int 5]) == ints [-5] &&
     depthOf (over ++ again) "negq" == 2 &&
     out (over ++ again ++ [w "negq", .int 5, w "undef", w "negq", w "negq", .int 5]) == ints [8, -5]) = true := by
  native_decide

/-- A BODY IS ASKED AT THE INSTALL whether its frames record: the reader
    leaves the question open — a function value is asked at each call —
    and the install answers it for every signature it compiles. -/
theorem the_install_answers_the_frame_question :
    (let open? (s : Sig) : Bool := match s with
       | .mk _ _ _ (.boru _ _ _ fs _) _ => fs.isNone
       | _ => false
     let spec := triple [w "n?:Integer"] [] [w "def", w "y", w "n", .endMark, w "y"]
     (read spec).all open? &&
     (sigsOf (defn "f" spec) "f").all (fun s => !open? s) &&
     -- and the answer is the body's: this one binds
     out (defn "f" spec ++ [w "f", .int 5, w "y"]) == "ERROR:undefined_word") = true := by
  native_decide

end Boru.Core.FnSpecThm
