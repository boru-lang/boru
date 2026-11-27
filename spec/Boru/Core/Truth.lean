/-
  Boru.Core.Truth — the one truthiness rule.

  Every place the kernel asks "is this value true?" — a conditional
  loop's condition, a branch — asks `coerceBoolean` (core/go
  core_helpers.go `CoerceBoolean`). It answers by PRESENCE, not by reading
  content:

    a Boolean is itself;          a number is true unless it is zero;
    none is false;                a list or map is true unless it is empty;
    a String is true unless it is empty — its characters are never read.

  Anything else is true unless it RENDERS as `false` or as nothing: a
  name written `false` that reached here as an atom or a word keeps its
  boolean reading. A union renders as its alternatives, so one of no
  alternatives is false, and so is one whose ONLY alternative renders as
  `false` or as nothing — the Boolean `false`, such an atom, such a union.

  A bare TYPE has no payload to be present: a Boolean, a number, a list or
  a map type is false, and so is `None`; any other type is true.
-/
import Boru.Core.Compare

namespace Boru.Core

/-- Does a value, standing as a union's only alternative, render as
    `false` or as nothing? `fuel` bounds the descent through nested
    unions. -/
def rendersFalse : Nat → Value → Bool
  | 0, _ => false
  | fuel + 1, v =>
    match v with
    | .bool b => !b
    | .atom name => name == "false" || name.isEmpty
    | .disjunct [] => true
    | .disjunct [alt] => rendersFalse fuel alt
    | .quoted inner => rendersFalse fuel inner
    | _ => false

/-- Is the value true? -/
def coerceBoolean (v : Value) : Bool :=
  match v.unquote with
  | .bool b => b
  | .int n => n != 0
  | .bigInt n => n != 0
  | .bigDec d => d.coeff != 0
  -- a float is zero when every bit but its sign is clear; NaN is not zero
  | .float f => f.bits % 2 ^ 63 != 0
  | .none => false
  | .str s => !s.isEmpty
  | .list elems _ => !elems.isEmpty
  | .map entries _ => !entries.isEmpty
  -- a typed container is a constraint with retained members, never
  -- "empty"
  | .typedList _ _ | .typedMap _ _ => true
  | .typeLit t =>
    !(conforms t TBoolean || conforms t "Scalar/Number" || t == TNone
      || nodeFamily t == TList || nodeFamily t == TMap)
  -- a union renders as its alternatives
  | .disjunct alts => !rendersFalse (v.size + 1) (.disjunct alts)
  | .atom name => name != "false" && !name.isEmpty
  | .word w => w.name != "false" && !w.name.isEmpty
  | _ => true

end Boru.Core
