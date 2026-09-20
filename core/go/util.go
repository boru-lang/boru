package core

import "fmt"

// Shared utility helpers consolidating duplicated patterns spread
// across the engine package. The goal is a single, well-tested
// surface for the half-dozen interactions every handler repeats —
// type-literal vs. concrete-value distinction, panic-safe map/list
// access, type-checked map-field lookups, and DefStacks resolution.
//
// Each helper is independently testable; the test file
// (util_test.go) targets 100% coverage. New duplicated patterns
// found in future code should land here, not be re-inlined.

// IsTypeLiteral reports whether v is a bare type literal — a Value
// that is a type-lattice node carrying no concrete payload and is
// not a CheckMode carrier. After the type/value merge a type literal
// IS its lattice node; it has Data == nil and Carrier == false.
//
// None is excluded: the unit type's only inhabitant doubles as the
// "absent value" sentinel throughout the codebase (handlers return
// NewTypeLiteral(TNone) for "no value found"). Treating it as a type
// literal would make consumers try to use it as a constraint;
// IsTypeLiteral(NewTypeLiteral(TNone)) returns false so the type-
// vs-value dispatch treats it as a value.
func IsTypeLiteral(v Value) bool {
	if v.Data != nil || v.Carrier {
		return false
	}
	return !IsNoneShape(v)
}

// IsConcrete reports whether v carries a real payload (not a type
// literal, not a Carrier). Mirrors the negation of IsTypeLiteral
// without the None exception — None values are concrete in the
// sense that they're real values flowing through the program.
//
// IsConcrete is NOT the negation of "Data == nil": a list or map
// carrier (NewCarrier(TList/TMap)) carries a ChildTypeInfo payload
// — Data != nil — yet is not concrete. Use IsConcrete when the
// intent is "I need a real value to read"; use IsBareTypeNode when
// the intent is "v is its own lattice node".
func IsConcrete(v Value) bool {
	return v.Data != nil && !v.Carrier
}

// DeepConcrete reports whether v is concrete ALL THE WAY DOWN: concrete
// itself, and — for a list or map — every element concrete too,
// recursively.
//
// IsConcrete is deliberately SHALLOW, and that is a trap for check-mode
// models. A map literal written at a call site is concrete even while a
// field inside it still holds a carrier: in `IO.read p {enc: e}` with a
// computed `e`, the options map passes IsConcrete but `enc` does not. The
// field accessors (MapFieldString and friends) report a carrier field as
// ABSENT, so a model that reads options to decide its result silently
// takes the default branch — that is how `IO.read p {enc: <computed>}`
// came to check as String while the run produced Bytes.
//
// So: gate on IsConcrete when the model merely needs a real value to
// exist; gate on DeepConcrete when the model ROUTES on the interior —
// reads fields, picks a branch per option, or hands the value to a
// validator that projects fields. Anything not provably concrete to the
// bottom must decline to the declared (widest) result instead.
//
// The recursion is depth-capped rather than cycle-tracked: a flex
// container can be self-referential, and the honest answer for a
// structure this deep is "cannot prove it", which is the safe direction.
func DeepConcrete(v Value) bool {
	return deepConcrete(v, 0)
}

// deepConcreteMaxDepth bounds DeepConcrete's walk. Options maps and the
// literals models route on are shallow; anything deeper declines.
const deepConcreteMaxDepth = 32

func deepConcrete(v Value, depth int) bool {
	if depth > deepConcreteMaxDepth || !IsConcrete(v) {
		return false
	}
	switch d := v.Data.(type) {
	case ListPayload:
		for _, e := range d.Elems {
			if !deepConcrete(e, depth+1) {
				return false
			}
		}
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !deepConcrete(mv, depth+1) {
				return false
			}
		}
	}
	return true
}

// DeepKnown is DeepConcrete widened by allConcreteArgs' rule: a BARE
// TYPE NODE carries no payload, but it is inert and self-evaluating, so
// the checked node IS the runtime operand. DeepConcrete rejects one
// (IsConcrete needs a payload), and rejecting it nested inside a literal
// throws away a decidable case — `{value: None}` is as statically known
// as `{value: 5}`, and a validator that demands a String declines both at
// run time.
//
// Use DeepKnown where the model routes on the interior of a literal a
// call site WRITES OUT; use DeepConcrete where the model needs a real
// payload to read (a string to parse, bytes to encode). Carriers and
// dynamic values are declined at every level either way — those are the
// shapes whose runtime value analysis does NOT hold.
func DeepKnown(v Value) bool {
	return deepKnown(v, 0)
}

func deepKnown(v Value, depth int) bool {
	if depth > deepConcreteMaxDepth || v.Dynamic {
		return false
	}
	if !IsConcrete(v) {
		// The one payload-less operand whose value is nonetheless known.
		return IsBareTypeNode(v)
	}
	switch d := v.Data.(type) {
	case ListPayload:
		for _, e := range d.Elems {
			if !deepKnown(e, depth+1) {
				return false
			}
		}
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !deepKnown(mv, depth+1) {
				return false
			}
		}
	}
	return true
}

// IsBareTypeNode reports whether v is a bare lattice node: it carries
// no payload (Data == nil) and is not a CheckMode carrier. It is the
// precise, intent-named replacement for the recurring
// `v.Data == nil && !v.Carrier` probe used wherever a value's OWN
// identity is treated as a type — naming (TypeNameOf / TypePathOf /
// ValueType), lattice-identity ordering (litVsConcreteOrder), and the
// "v IS its denoted type" shortcut (denotedType).
//
// Unlike IsTypeLiteral, this INCLUDES the None / Any / Never / Absent
// type literals: every such value's own node is its type, so naming
// and ordering must treat them as nodes. When the question is instead
// "may the caller use v as a type CONSTRAINT?" (None excluded, because
// handlers return NewTypeLiteral(TNone) for "no value found"), use
// IsTypeLiteral.
func IsBareTypeNode(v Value) bool {
	return v.Data == nil && !v.Carrier
}

// RequireConcreteList unwraps a list-typed Value into its ReadList,
// returning an error if the value is a type literal, a carrier, or
// otherwise lacks a concrete list payload. Use this at native-handler
// entry points that take a list arg via [TList] or [TAny]; it
// replaces the recurring `if args[i].Data == nil { return error }`
// preamble.
//
// The op string is included in the error so callers can identify
// their site without wrapping again.
func RequireConcreteList(v Value, op string) (ReadList, error) {
	if v.Data == nil {
		return ReadList{}, fmt.Errorf("%s: expected a concrete list, got type literal %s", op, v.Parent.String())
	}
	if v.Carrier {
		return ReadList{}, fmt.Errorf("%s: expected a concrete list, got carrier %s", op, v.Parent.String())
	}
	list, err := AsList(v)
	if err != nil || list.IsNil() {
		return ReadList{}, fmt.Errorf("%s: value is not a list (got %s)", op, v.Parent.String())
	}
	return list, nil
}

// RequireConcreteMap unwraps a map-typed Value into its ReadMap. As
// with RequireConcreteList, type literals, carriers, and map-subtypes
// (RecordTypeInfo, OptionsTypeInfo, ChildTypeInfo) that lack a
// concrete OrderedMap payload return an error.
func RequireConcreteMap(v Value, op string) (ReadMap, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("%s: expected a concrete map, got type literal %s", op, v.Parent.String())
	}
	if v.Carrier {
		return nil, fmt.Errorf("%s: expected a concrete map, got carrier %s", op, v.Parent.String())
	}
	m, err := AsMap(v)
	if err != nil || m == nil {
		return nil, fmt.Errorf("%s: value is not a concrete map (got %s)", op, v.Parent.String())
	}
	return m, nil
}

// IsFlexMap reports whether v is a concrete FlexMap — a mutable
// Node/Map/FlexMap value with a real MapPayload (type literals and
// carriers are excluded).
func IsFlexMap(v Value) bool {
	if v.Parent == nil || !v.Parent.Equal(TFlexMap) || !IsConcrete(v) {
		return false
	}
	_, ok := v.Data.(MapPayload)
	return ok
}

// IsFlexList reports whether v is a concrete FlexList — a mutable
// Node/List/FlexList value with a pointer-backed FlexListData store.
func IsFlexList(v Value) bool {
	if v.Parent == nil || !v.Parent.Equal(TFlexList) || !IsConcrete(v) {
		return false
	}
	_, ok := v.Data.(*FlexListData)
	return ok
}

// IsFlexXml reports whether v is a concrete FlexXml — a mutable
// Node/Xml/FlexXml value with a pointer-backed FlexXmlData store.
func IsFlexXml(v Value) bool {
	if v.Parent == nil || !v.Parent.Equal(TFlexXml) || !IsConcrete(v) {
		return false
	}
	_, ok := v.Data.(*FlexXmlData)
	return ok
}

// IsFlexNode reports whether v is a concrete flex node of any kind.
func IsFlexNode(v Value) bool {
	return IsFlexMap(v) || IsFlexList(v) || IsFlexXml(v)
}

// MapFieldString fetches a String-valued field from a ReadMap.
// Returns the string and true on hit; "" and false when the key is
// absent OR the value's type is not String. Replaces the
// `if v, ok := m.Get(k); ok && v.Parent.ConformsTo(TString) { s, _ := v.AsString(); … }`
// pattern that appeared in fileio.go, native_string_helpers.go, and
// native_type_make.go.
//
// Nil map → false (panic-safe).
func MapFieldString(m ReadMap, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m.Get(key)
	if !ok || !v.Parent.ConformsTo(TString) {
		return "", false
	}
	s, err := AsString(v)
	if err != nil {
		return "", false
	}
	return s, true
}

// MapFieldInteger fetches an Integer-valued field. Same shape as
// MapFieldString.
func MapFieldInteger(m ReadMap, key string) (int64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m.Get(key)
	if !ok || !v.Parent.ConformsTo(TInteger) || v.IsDepScalar() {
		return 0, false
	}
	n, err := AsInteger(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

// MapFieldBoolean fetches a Boolean-valued field.
func MapFieldBoolean(m ReadMap, key string) (bool, bool) {
	if m == nil {
		return false, false
	}
	v, ok := m.Get(key)
	if !ok || !v.Parent.ConformsTo(TBoolean) {
		return false, false
	}
	b, err := AsBoolean(v)
	if err != nil {
		return false, false
	}
	return b, true
}

// MapFieldFloat fetches a Float-valued field.
func MapFieldFloat(m ReadMap, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m.Get(key)
	if !ok || !v.Parent.ConformsTo(TFloat) || v.IsDepScalar() {
		return 0, false
	}
	f, err := AsFloat(v)
	if err != nil {
		return 0, false
	}
	return f, true
}

// --- Pos threading ------------------------------------------------------

// WithPos returns v with its Pos copied from src. Use when a handler
// constructs a new Value from an input — error reporting downstream
// then has the source location even though the new value is
// structurally unrelated to the input.
func WithPos(v, src Value) Value {
	v.pos = src.pos
	return v
}

func WithPosAt(v Value, p SrcPos) Value {
	if p.Row == 0 || v.Pos().Row != 0 {
		return v
	}
	v.pos = &p
	return v
}

// ReparentValue returns a copy of v with its Parent rebound to def.
// The single primitive every typed-def reparent path uses (predicate
// types, ObjectInstance dispatch on Person, FnUndef function-shape
// binding, refine-bare scalar subtypes). Payload, Pos, Quoted,
// Carrier, Eval are preserved unchanged.
//
// The discipline this codifies: NEVER mutate `Parent` on a Value
// that was returned by Unify when Unify could have swapped to a
// type-literal side. The refine-bare reparent originally got this
// wrong and stored the Foo type literal as the binding instead of
// the integer body. Using this helper makes the by-value copy
// explicit and the mistake unreachable.
//
// See `design/legacy/TYPE-CANONICALIZATION.10.ignore`.
func ReparentValue(v Value, def *Type) Value {
	v.Parent = def
	return v
}

// CanonicalType resolves t to its canonical lattice node — the
// pointer registered in TypeTable.byID for t.ID. If t is already
// canonical, has no ID, or no registry is in scope, returns t
// unchanged.
//
// Background: `type Type = Value`, so a "type literal" is a Value
// carrying the lattice node's fields by value. Code that grabs
// `&v` of such a stack-local Value gets a non-canonical pointer:
// it compares Equal via ID, but mutations to fields like Behavior
// (which `behave` writes through the canonical pointer) don't
// propagate to the orphan copies. Every site that needs *Type
// identity for a Value-shaped type literal — fn-sig parameter
// resolution, refine subtype minting, behave validation — routes
// through this helper so identity stays canonical at every hop.
//
// See `design/legacy/TYPE-CANONICALIZATION.10.ignore`.
func CanonicalType(r *Registry, t *Type) *Type {
	if t == nil || r == nil || t.ID == "" {
		return t
	}
	if canon := r.Types.LookupByID(t.ID); canon != nil {
		return canon
	}
	return t
}

// predicateSandbox holds the slice/map state that RunPredicate
// snapshots before invoking a predicate body. DefStacks is NOT
// included — CallBoru handles that itself. r.Check is preserved by
// reference (the entire CheckState struct is copied) so any
// per-call diagnostics or step counters set during the predicate
// don't leak.
type predicateSandbox struct {
	types    *TypeTable
	ctxStack []*StoreInstanceInfo
	check    *CheckState
}

func snapshotPredicateState(r *Registry) predicateSandbox {
	if r == nil {
		return predicateSandbox{}
	}
	return predicateSandbox{
		types:    r.Types.Clone(),
		ctxStack: r.Contexts.Snapshot(),
		check:    r.analysisSnapshot(),
	}
}

func restorePredicateState(r *Registry, s predicateSandbox) {
	if r == nil {
		return
	}
	r.Types = s.types
	r.Contexts.Restore(s.ctxStack)
	r.restoreAnalysisSnapshot(s.check)
}

// AsConcreteString unwraps a String-typed Value into its Go string,
// returning a clear error if the value is a DepScalar constraint
// payload rather than a concrete String. The lattice override makes
// `DepString.ConformsTo(TString)` true for sig-matching purposes, so
// any code path that sees a TString value and immediately calls
// `AsString` will hit a `DepString → "" + error` silent miscompile
// when the caller swallows the error. Use AsConcreteString in any
// path where the concrete payload is required (display, comparison,
// indexing, …); the error is loud and discoverable.
func (v Value) AsConcreteString() (string, error) {
	if v.IsDepScalar() {
		return "", fmt.Errorf("AsConcreteString: value is a dependent-type constraint (%s), not a concrete String", v.Parent.String())
	}
	// Handlers with dual [TString]+[TAtom] signatures (trim, upper,
	// lower, concat, …) call into AsConcreteString from either path;
	// historically they relied on the raw-string payload being
	// shared between strings and atoms. Post Step 5, atoms carry
	// AtomPayload; accept it here so the "textual content" semantic
	// of AsConcreteString is preserved for those handlers.
	if IsAtom(v) {
		return AsAtom(v)
	}
	return AsString(v)
}

// AsConcreteInteger — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteInteger() (int64, error) {
	if v.IsDepScalar() {
		return 0, fmt.Errorf("AsConcreteInteger: value is a dependent-type constraint (%s), not a concrete Integer", v.Parent.String())
	}
	return AsInteger(v)
}

// AsConcreteFloat — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteFloat() (float64, error) {
	if v.IsDepScalar() {
		return 0, fmt.Errorf("AsConcreteFloat: value is a dependent-type constraint (%s), not a concrete Float", v.Parent.String())
	}
	return AsFloat(v)
}

// AsConcreteBoolean — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteBoolean() (bool, error) {
	if v.IsDepScalar() {
		return false, fmt.Errorf("AsConcreteBoolean: value is a dependent-type constraint (%s), not a concrete Boolean", v.Parent.String())
	}
	return AsBoolean(v)
}

// AsConcreteAtom — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteAtom() (string, error) {
	if v.IsDepScalar() {
		return "", fmt.Errorf("AsConcreteAtom: value is a dependent-type constraint (%s), not a concrete Atom", v.Parent.String())
	}
	return AsAtom(v)
}

// FlattenDisjunctAlts returns the alternatives of a disjunct value
// or a single-element slice containing v if it isn't a disjunct.
// Replaces the
// `if v.IsDisjunct() { d, _ := v.AsDisjunct(); alts = append(alts,
// d.Alternatives...) } else { alts = append(alts, v) }` pattern in
// `tor`, `tand`, `tany`, and several unify branches.
//
// Inlines the type-assertion the IsDisjunct/AsDisjunct pair would
// otherwise duplicate so there's no unreachable error branch.
// Values whose Parent is a Disjunct (Type/Disjunct or a subtype such
// as Type/Disjunct/Enum) but whose payload isn't a real DisjunctInfo
// fall back to the single-element slice.
func FlattenDisjunctAlts(v Value) []Value {
	if d, ok := v.Data.(DisjunctInfo); ok && v.Parent.ConformsTo(TDisjunct) {
		return d.Alternatives
	}
	return []Value{v}
}
