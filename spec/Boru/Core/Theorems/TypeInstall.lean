/-
  Boru.Core.Theorems.TypeInstall — a type a program defines.

  A capitalised name bound to a type body is a TYPE. A bare type makes an
  ALIAS: the name adopts that node, and no new type exists. A union, a
  literal, a list or a map, a typed container MINTS a node — a bare type
  like a builtin, under the node its content sits at — whose members are
  decided by that content and by nothing else: no value is ever tagged
  with a minted node, so membership is never a walk up the lattice.

  Two questions are asked of a minted node, and both are answered by its
  content. IS THIS VALUE A MEMBER — what a slot of the type, and a return
  declared as it, ask — and WHAT IS THE MEET with this other value. A
  union answers both as its alternatives would. Every other type unifies
  a VALUE with its content, and settles a TYPE by the lattice.

  Six things the kernel does that the rule does not say are stated here
  as the kernel has them, and marked: NUR418 (a name is never released),
  NUR419 (a named type's pending operand is not evaluated), NUR420 (an
  omitted parameter of a named type is given a base value unasked),
  NUR421 (a singleton passes for a value), NUR422 (the meet of two named
  types depends on their order) and NUR424 (a word literal written after
  the word is not claimed by a slot of a named type).

  Discharged by `native_decide` over the stated domains: the installer
  and the unifier directly, and the machine over the corpus fixture —
  `typeq Name body` is the installer as a word.
-/
import Boru.Core.Theorems.Counted

namespace Boru.Core.TypeDef
open Boru.Core
open Boru.Core.Counted (w outWith out ints)
open Boru.Corespec (fixtureDefs)

def pend (vs : List Value) : Value := .list vs true
def quot (vs : List Value) : Value := .list vs false
def one (v : Value) : String := canonValues [v]
def refused : String := "ERROR:signature_error"

/-! ## The domains -/

/-- Bodies that mint a singleton or a structural type: a number, a
    string, a Boolean, a list, a map shape, a typed list. -/
def bodies : List Value :=
  [.int 1, .str "a", .bool true, quot [.int 1, .int 2],
   .map [("a", .typeLit TInteger)] false, .typedList (.typeLit TInteger) []]

/-- Unions, each in its canonical order. -/
def unions : List (List Value) :=
  [[.typeLit TInteger, .typeLit TString], [.int 1, .int 2], [.typeLit TNone, .typeLit TInteger]]

def allBodies : List Value := bodies ++ unions.map .disjunct

def values : List Value :=
  [.int 1, .int 2, .str "a", .str "", .bool true, .none, .atom "x", quot [.int 1, .int 2], quot [],
   quot [.str "x"], .map [("a", .int 1)] false, .map [("a", .int 1), ("b", .int 2)] false, .map [] false]

/-- Every builtin type a program can name. -/
def builtinTypes : List TypeId := (builtinDecls.filter (!·.internal)).map (·.path)

/-- The bindings after `name` is installed as a type over `body`. -/
def installed (name : String) (body : Value) : Defs :=
  match TypeInstall.install fixtureDefs name body with
  | .ok d => d
  | .error _ => []

/-- The node the name is bound to. -/
def nodeIn (d : Defs) (name : String) : Value := (d.typeNode? name).getD .none
def node (name : String) (body : Value) : Value := nodeIn (installed name body) name

/-- The code an installation is refused with; empty when it is not. -/
def refusal (d : Defs) (name : String) (body : Value) : String :=
  match TypeInstall.install d name body with
  | .ok _ => ""
  | .error (code, _) => code

/-- `; typeq Name body ;` and `; def f ( fn [params] [returns] [body] ) ;`,
    as tokens. -/
def typeq (name : String) (body : Value) : List Value := [.endMark, w "typeq", w name, body, .endMark]
def defF (param ret : String) : List Value :=
  [.endMark, w "def", w "f", .openParen, w "fn",
   quot [quot [w param], quot [w ret], quot [w "n"]], .closeParen, .endMark]

/-! ## An alias, and a mint -/

/-- AN ALIAS ADOPTS THE NODE. Bound to a bare type, the name IS that type:
    it reads as it, it is a type binding, and nothing was minted. Bound to
    a type a program minted, it is that same node. -/
theorem an_alias_adopts_the_node :
    ((builtinTypes.all fun t =>
        let d := installed "Foo" (.typeLit t)
        d.top? "Foo" == some (.typeLit t) && d.isType "Foo" &&
        d.freshIdent.1 == fixtureDefs.freshIdent.1) &&
     (allBodies.all fun b =>
        let d := installed "Foo" b
        match TypeInstall.install d "Bar" (nodeIn d "Foo") with
        | .ok d2 => nodeIn d2 "Bar" == nodeIn d "Foo" && d2.freshIdent.1 == d.freshIdent.1
        | .error _ => false)) = true := by
  native_decide

/-- ANY OTHER BODY MINTS A NODE UNDER ITS OWN TYPE. The node carries the
    body, renders as its name, and is a bare type — it has no payload. It
    sits under the node the body sits at; and NOTHING sits under it: no
    builtin, and no value's type. -/
theorem a_body_mints_a_node_under_its_own_type :
    (allBodies.all fun b =>
      match node "Foo" b with
      | .named path body =>
        body == b && mintedParent? path == some b.typeOf && canon (.named path body) == "Foo"
          && (Value.named path body).isBareTypeNode && !(Value.named path body).isConcrete
          && conforms path b.typeOf && conforms path path
          && (builtinTypes.all fun t => !conforms t path)
          && (values.all fun v => !conforms v.typeOf path)
      | _ => false) = true := by
  native_decide

/-- TWO TYPES OF ONE CONTENT ARE TWO TYPES: each installation mints a
    node of its own, neither is under the other, and they do not unify —
    while each unifies with itself. -/
theorem two_types_of_one_content_are_two_types :
    (bodies.all fun b =>
      match TypeInstall.install (installed "One" b) "Uno" b with
      | .ok d =>
        let a := nodeIn d "One"
        let c := nodeIn d "Uno"
        a != c && unify a c == none && unify c a == none && unify a a == some a
      | .error _ => false) = true := by
  native_decide

/-! ## Membership -/

/-- A SLOT ASKS THE CONTENT. A value is a member of a singleton or a
    structural type exactly when it unifies with the body; of a union,
    exactly when it unifies with the union written in place. -/
theorem membership_is_the_contents :
    ((bodies.all fun b => values.all fun v => memberMatch (node "Foo" b) v == unifies v b) &&
     (unions.all fun alts => values.all fun v =>
        memberMatch (node "Foo" (.disjunct alts)) v == unifies (.disjunct alts) v)) = true := by
  native_decide

/-- A TYPE IS NOT A MEMBER. No builtin type is a member of a minted type —
    an alternative is not a value of the union — but for `None`, which a
    union that names it takes as the value it stands for. The type ITSELF
    is taken: it is under itself in the lattice, the one thing that is. -/
theorem a_type_is_not_a_member :
    (allBodies.all fun b =>
      let n := node "Foo" b
      memberMatch n n &&
      builtinTypes.all fun t => t == TNone || !memberMatch n (.typeLit t)) = true := by
  native_decide

/-- THROUGH THE MACHINE: a function whose parameter is of the type takes a
    member and refuses what is none; and a function whose RETURN is of the
    type hands a member back and raises on what is none. -/
theorem a_slot_and_a_return_ask_the_type :
    (let foo : Value := .disjunct [.typeLit TInteger, .typeLit TString]
     let viaSlot (arg : Value) := out (typeq "Foo" foo ++ defF "n:Foo" "Any" ++ [w "f", arg])
     let viaReturn (arg : Value) := out (typeq "Foo" foo ++ defF "n:Any" "Foo" ++ [w "f", arg])
     [Value.int 5, .str "a", .int (-3), .str ""].all (fun v => viaSlot v == one v && viaReturn v == one v) &&
     [Value.bool true, .atom "x"].all (fun v => viaSlot v == refused && viaReturn v == "ERROR:type_error") &&
     -- the slot reads as the type's name, and so does the return
     out (typeq "Foo" foo ++ defF "n:Foo" "Foo" ++ [.word { name := "f", forceVal := true }])
       == "fn [[n:Foo][Foo][n]]") = true := by
  native_decide

/-! ## The meet -/

/-- THE MEET WITH A VALUE IS THE VALUE'S MEET WITH THE CONTENT — the value
    as that unification leaves it, never the type. From either side. -/
theorem the_meet_with_a_value_is_the_values :
    (bodies.all fun b => values.all fun v =>
      let n := node "Foo" b
      unify n v == unify v b && unify v n == unify v b) = true := by
  native_decide

/-- AGAINST A TYPE THE LATTICE ANSWERS. A singleton or a structural type
    sits under the node its content sits at: against that node, or any
    node above it, the named type is the narrower and is the meet; against
    any other builtin there is none. -/
theorem against_a_type_the_lattice_answers :
    (bodies.all fun b => builtinTypes.all fun t =>
      let n := node "Foo" b
      let want := if conforms b.typeOf t then some n else none
      unify n (.typeLit t) == want && unify (.typeLit t) n == want) = true := by
  native_decide

/-- A UNION ANSWERS AS ITS ALTERNATIVES. Against a value, a builtin type
    or a union written in place, the named union's meet is the meet of
    the union it was declared as — from either side. So the answer never
    carries the name; only the type against ITSELF is itself. -/
theorem a_union_answers_as_its_alternatives :
    (unions.all fun alts =>
      let n := node "Foo" (.disjunct alts)
      unify n n == some n &&
      (values ++ builtinTypes.map Value.typeLit ++ unions.map Value.disjunct).all fun x =>
        unify n x == unify (.disjunct alts) x && unify x n == unify (.disjunct alts) x) = true := by
  native_decide

/-! ## Dispatch order -/

/-- A signature of one parameter, of a builtin type or of a named one. -/
def slotSig (t : TypeId) (pat : Option Value) : Sig :=
  .mk [.mk "n" t pat false false false {}] [TAny] 1 (.boru [] [] false none []) false
def nodeSig (n : Value) : Sig := slotSig n.typeOf (some n)

/-- A NAMED TYPE IS TRIED BEFORE THE BUILTINS OF ITS BRANCH, and before
    `Any`: it is ranked in its branch's external band, above every builtin
    there. A union's node is in the Type branch's band, the highest: it is
    tried before every builtin. (`KeyVal` is the one builtin that sits in
    an external band itself, and is left out.) A literal pattern of the
    singleton's own family is still tried before it: a value sorts above
    every bare type of its family, a named one included. -/
theorem a_named_type_is_tried_before_the_builtins_of_its_branch :
    ((bodies.all fun b =>
        let n := node "Foo" b
        let branch := (b.typeOf.splitOn "/").headD ""
        (builtinTypes.all fun t =>
          !(t == TAny || (t.splitOn "/").headD "" == branch) || t == "Node/Map/KeyVal"
            || Sig.order (nodeSig n) (slotSig t none) == .lt)) &&
     ([1, 7, -3].all fun (k : Int) =>
        Sig.order (slotSig TInteger (some (.int 1))) (nodeSig (node "Foo" (.int k))) == .lt) &&
     (unions.all fun alts => builtinTypes.all fun t =>
        Sig.order (nodeSig (node "Foo" (.disjunct alts))) (slotSig t none) == .lt)) = true := by
  native_decide

/-- THROUGH THE MACHINE: of two overloads, one of the singleton and one of
    the builtin it sits under, the singleton's is tried first in either
    written order; what it refuses falls to the builtin's. -/
theorem the_overload_of_a_named_type_is_tried_first :
    (let overloads (first second : String × String) : List Value :=
       [.endMark, w "def", w "f", .openParen, w "fn",
        quot [quot [w first.1], quot [w "Any"], quot [.str first.2],
              quot [w second.1], quot [w "Any"], quot [.str second.2]], .closeParen, .endMark]
     [overloads ("n:One", "one") ("n:Integer", "int"), overloads ("n:Integer", "int") ("n:One", "one"),
      overloads ("n:One", "one") ("n:Any", "any"), overloads ("n:Any", "any") ("n:One", "one")].all fun defs =>
       out (typeq "One" (.int 1) ++ defs ++ [w "f", .int 1]) == one (.str "one") &&
       out (typeq "One" (.int 1) ++ defs ++ [w "f", .int 2]) != one (.str "one")) = true := by
  native_decide

/-! ## A name inside a shape, and stepped -/

/-- INSIDE A TYPE SHAPE A NAME IS THE CONTENT; STEPPED, IT IS THE TYPE. A
    typed container's child written as the name is the body the type was
    declared with; the name stepped — alone, or in a list literal — is the
    node, which renders as the name. -/
theorem inside_a_shape_a_name_is_the_content :
    (allBodies.all fun b =>
      let d := installed "Foo" b
      resolveInert d (.typedList (w "Foo") []) == .typedList b [] &&
      resolveInert d (.typedMap (w "Foo") []) == .typedMap b [] &&
      outWith d [w "Foo"] == "Foo" && outWith d [pend [w "Foo"]] == "[Foo]") = true := by
  native_decide

/-! ## What the installer refuses, and what a name holds -/

/-- WHAT THE INSTALLER REFUSES. A name that does not begin with a capital;
    a name that is a part of any builtin's path, alone or among the parts
    of a slashed name; a function's name; a value's name. A body that is
    a function (a `def_error`: it is a type only as a declared predicate),
    and a body that is neither a type nor a literal. The NAME is checked
    before the function arm, so a lowercase name over a function is the
    name's error. -/
theorem what_the_installer_refuses :
    (let f : Value := .fn "g" [slotSig TInteger none] 9301
     refusal fixtureDefs "foo" (.int 1) == "type_error" &&
     refusal fixtureDefs "" (.int 1) == "type_error" &&
     (TypeInstall.builtinParts.all fun part => refusal fixtureDefs part (.int 1) == "type_error") &&
     refusal fixtureDefs "Foo/Integer" (.int 1) == "type_error" &&
     refusal fixtureDefs "Number/Foo" (.int 1) == "type_error" &&
     refusal (fixtureDefs.push "Zed" f) "Zed" (.int 1) == "type_error" &&
     refusal (fixtureDefs.push "Zed" (.int 5)) "Zed" (.int 1) == "type_error" &&
     refusal fixtureDefs "Foo" f == "def_error" &&
     refusal fixtureDefs "foo" f == "type_error" &&
     ([Value.splice (.int 5), w "x", .parenExpr [.int 1], .openParen].all fun b =>
        refusal fixtureDefs "Foo" b == "type_error") &&
     -- and what it takes: every body of the domains, under a free name
     (allBodies ++ builtinTypes.map Value.typeLit).all fun b => refusal fixtureDefs "Foo" b == "") = true := by
  native_decide

/-- A REDEFINITION STACKS. The newest binding is the name's; the older
    node is beneath it, unchanged, and unbinding shows it again. -/
theorem a_redefinition_stacks :
    (match TypeInstall.install (installed "Foo" (.int 1)) "Foo" (.int 2) with
     | .ok d =>
       let newer := nodeIn d "Foo"
       let older := nodeIn (d.pop "Foo") "Foo"
       d.depth "Foo" == 2 && memberMatch newer (.int 2) && !memberMatch newer (.int 1)
         && memberMatch older (.int 1) && older == node "Foo" (.int 1)
     | .error _ => false) = true := by
  native_decide

/-- A SLOT KEEPS THE NODE IT WAS DECLARED WITH. A function whose parameter
    is of the type still takes the older type's member after the name is
    redefined, and after it is unbound — and still reads as the name. -/
theorem a_slot_keeps_the_node_it_was_declared_with :
    (let prog (tail : List Value) := out (typeq "Foo" (.int 1) ++ defF "n:Foo" "Any" ++ tail)
     prog (typeq "Foo" (.int 2) ++ [w "f", .int 1]) == ints [1] &&
     prog (typeq "Foo" (.int 2) ++ [w "f", .int 2]) == refused &&
     prog [w "undef", w "Foo", .endMark, w "f", .int 1] == ints [1] &&
     prog [w "undef", w "Foo", .endMark, .word { name := "f", forceVal := true }]
       == "fn [[n:Foo][Any][n]]") = true := by
  native_decide

/-! ## As the kernel has it -/

/-- AS THE KERNEL HAS IT (NUR418): UNBINDING DOES NOT RELEASE THE NAME.
    The name is unbound — it reads as nothing — and cannot be a type
    again: its reservation outlives the binding. Another name is free. -/
theorem nur418_unbinding_does_not_release_the_name :
    (allBodies.all fun b =>
      let d := (installed "Foo" b).pop "Foo"
      d.top? "Foo" == none && refusal d "Foo" b == "type_error" && refusal d "Bar" b == "") = true := by
  native_decide

/-- AS THE KERNEL HAS IT (NUR419): A NAMED TYPE'S PENDING OPERAND IS NOT
    EVALUATED. A typed list written as a slot's pattern has a pending
    operand evaluated before it judges it (`lpatq [ addq 1 2 ]` is the
    list `[3]`). The same typed list under a NAME is the slot's type, not
    its pattern: the operand's tokens are judged as they stand, and
    refused. -/
theorem nur419_a_named_types_pending_operand_is_not_evaluated :
    (let lst : Value := .typedList (.typeLit TInteger) []
     let sum : Value := pend [w "addq", .int 1, .int 2]
     let prog (arg : Value) := out (typeq "Lst" lst ++ defF "n:Lst" "Any" ++ [w "f", arg])
     prog (quot [.int 3]) == one (.quoted (quot [.int 3])) &&
     prog sum == refused &&
     out [w "lpatq", sum] == one (quot [.int 3])) = true := by
  native_decide

/-- AS THE KERNEL HAS IT (NUR420): AN OMITTED PARAMETER OF A NAMED TYPE IS
    GIVEN A BASE VALUE UNASKED. An optional parameter adds a signature
    without it, whose body calls the function with the parameter's type's
    base value. For a singleton under Integer that is 0 — which is no
    member — and the type's own rule is not consulted: the call the body
    makes can only come back to the same signature. -/
theorem nur420_an_omitted_parameter_of_a_named_type_is_given_a_base_value_unasked :
    ([1, 7, -3].all fun (k : Int) =>
      let n := node "One" (.int k)
      let s : Sig := .mk [.mk "n" n.typeOf (some n) false false true {}] [TAny] 1
        (.boru [w "n"] [] false none []) false
      let expanded := FnSpec.expandOptional "g" s
      expanded.length == 2 && !memberMatch n (.int 0) &&
      (match expanded[1]? with
       | some (Sig.mk [] _ _ (Impl.boru body _ _ _ _) _) => body == [w "g", .int 0]
       | _ => false)) = true := by
  native_decide

/-- AS THE KERNEL HAS IT (NUR421): A SINGLETON PASSES FOR A VALUE. A bare
    type is refused at a slot that reads a payload — `Integer` is not an
    Integer. A minted node that carries content is exempt from that
    refusal, and it sits under its content's type: so `One` is admitted
    at an Integer slot, and the word reads the payload it does not have —
    zero, whatever the singleton's literal. -/
theorem nur421_a_singleton_passes_for_a_value :
    ([1, 7, -3].all fun (k : Int) =>
      let n := node "One" (.int k)
      !rejectsTypeLiteral n TInteger && rejectsTypeLiteral (.typeLit TInteger) TInteger &&
      n.is TInteger &&
      out (typeq "One" (.int k) ++ [w "addq", w "One", .int 1]) == ints [1] &&
      out (typeq "One" (.int k) ++ [w "negq", w "One"]) == ints [0] &&
      out [w "addq", .typeLit TInteger, .int 1] == refused) = true := by
  native_decide

/-- AS THE KERNEL HAS IT (NUR422): THE MEET OF TWO NAMED TYPES DEPENDS ON
    THEIR ORDER — and on which of them is named. Two named types are
    settled by the LEFT one's rule. The union first, its alternatives are
    asked and Integer takes the singleton: the meet is the singleton. The
    singleton first, the lattice is asked, where no minted type is under
    another: no meet. And the same union written in place is a VALUE to
    the singleton's rule: the meet is the value 1, not the type. -/
theorem nur422_the_meet_of_two_named_types_depends_on_their_order :
    (let alts : List Value := [.typeLit TInteger, .typeLit TString]
     match TypeInstall.install (installed "Foo" (.disjunct alts)) "One" (.int 1) with
     | .ok d =>
       let foo := nodeIn d "Foo"
       let uno := nodeIn d "One"
       unify foo uno == some uno && unify uno foo == none &&
       unify (.disjunct alts) uno == some (.int 1) && unify uno (.disjunct alts) == some (.int 1)
     | .error _ => false) = true := by
  native_decide

/-- AS THE KERNEL HAS IT (NUR424): A WORD LITERAL WRITTEN AFTER THE WORD IS
    NOT CLAIMED BY A SLOT OF A NAMED TYPE. The plan reads `true` or
    `false` written after a word as "a Boolean will stand here" — a probe
    of that type with no payload. A slot of a named type asks for the
    VALUE, and to it such a probe is a type, which is no member. So the
    union of `'a'` and Boolean, of which every Boolean is a member,
    refuses `true` written after the word — and takes it from the stack,
    as a group's value, and as the Boolean itself where no word stands
    for it. -/
theorem nur424_a_word_literal_is_not_claimed_by_a_slot_of_a_named_type :
    (let foo : Value := .disjunct [.str "a", .typeLit TBoolean]
     let prog (tail : List Value) := out (typeq "Foo" foo ++ defF "n:Foo" "Any" ++ tail)
     ["true", "false"].all fun b =>
       let v : Value := .bool (b == "true")
       memberMatch (node "Foo" foo) v &&
       prog [w "f", w b] == refused &&
       prog [w b, w "f"] == one v &&
       prog [w "f", .openParen, w b, .closeParen] == one v &&
       prog [w "f", v] == one v) = true := by
  native_decide

end Boru.Core.TypeDef
