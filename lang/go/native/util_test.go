package native

import (
	"strings"
	"testing"
)

// Tests for the shared utility helpers in util.go. Coverage target:
// 100% for util.go (every branch in every helper). Each helper is
// exercised in isolation — the goal is to pin behaviour at the
// helper boundary, not test the broader engine through them.

// --- IsTypeLiteral / IsConcrete ---

func TestIsTypeLiteral_BareTypeLiteral(t *testing.T) {
	if !IsTypeLiteral(NewTypeLiteral(TInteger)) {
		t.Errorf("Integer type literal: IsTypeLiteral=false, want true")
	}
}

func TestIsTypeLiteral_ConcreteValue(t *testing.T) {
	if IsTypeLiteral(NewInteger(42)) {
		t.Errorf("concrete Integer: IsTypeLiteral=true, want false")
	}
}

func TestIsTypeLiteral_Carrier(t *testing.T) {
	c := NewCarrier(TInteger)
	if IsTypeLiteral(c) {
		t.Errorf("carrier: IsTypeLiteral=true, want false (carriers are not literals)")
	}
}

func TestIsTypeLiteral_None(t *testing.T) {
	// None is the unit type AND its only inhabitant; treat it as a
	// value, not a type literal, so consumers don't try to "use" it
	// as a constraint.
	if IsTypeLiteral(NewTypeLiteral(TNone)) {
		t.Errorf("None: IsTypeLiteral=true, want false (None is a value, not a type)")
	}
}

func TestIsConcrete_ConcreteValue(t *testing.T) {
	if !IsConcrete(NewInteger(42)) {
		t.Errorf("concrete Integer: IsConcrete=false, want true")
	}
}

func TestIsConcrete_TypeLiteral(t *testing.T) {
	if IsConcrete(NewTypeLiteral(TInteger)) {
		t.Errorf("type literal: IsConcrete=true, want false")
	}
}

func TestIsConcrete_Carrier(t *testing.T) {
	if IsConcrete(NewCarrier(TInteger)) {
		t.Errorf("carrier: IsConcrete=true, want false")
	}
}

func TestIsConcrete_NoneIsConcrete(t *testing.T) {
	// Mirror image of IsTypeLiteral: None counts as concrete.
	if IsConcrete(NewTypeLiteral(TNone)) {
		// IsConcrete needs Data != nil, and None's Data IS nil.
		// So IsConcrete(None) should be FALSE — None is neither
		// a type literal (per IsTypeLiteral) nor concrete in the
		// "has a payload" sense. Document the corner.
		t.Errorf("None: IsConcrete=true, want false (None has nil Data)")
	}
}

// --- RequireConcreteList ---

func TestRequireConcreteList_Concrete(t *testing.T) {
	v := NewList([]Value{NewInteger(1), NewInteger(2)})
	list, err := RequireConcreteList(v, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if list.Len() != 2 {
		t.Errorf("got Len=%d, want 2", list.Len())
	}
}

func TestRequireConcreteList_TypeLiteral(t *testing.T) {
	_, err := RequireConcreteList(NewTypeLiteral(TList), "myop")
	if err == nil {
		t.Fatalf("expected error for type literal")
	}
	if !strings.Contains(err.Error(), "myop:") {
		t.Errorf("error %q does not include op name", err)
	}
	if !strings.Contains(err.Error(), "type literal") {
		t.Errorf("error %q does not say 'type literal'", err)
	}
}

func TestRequireConcreteList_Carrier(t *testing.T) {
	_, err := RequireConcreteList(NewCarrier(TList), "myop")
	if err == nil {
		t.Fatalf("expected error for carrier")
	}
	if !strings.Contains(err.Error(), "carrier") {
		t.Errorf("error %q does not mention carrier", err)
	}
}

func TestRequireConcreteList_NotAList(t *testing.T) {
	// A typed-list payload (ChildTypeInfo) has Parent=TList but
	// AsList returns IsNil. RequireConcreteList rejects it with
	// the "not a list" branch.
	v := NewTypedList(NewTypeLiteral(TInteger))
	_, err := RequireConcreteList(v, "myop")
	if err == nil {
		t.Fatalf("expected error for typed-list (no concrete payload)")
	}
	if !strings.Contains(err.Error(), "not a list") {
		t.Errorf("error %q does not say 'not a list'", err)
	}
}

// --- RequireConcreteMap ---

func TestRequireConcreteMap_Concrete(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	v := NewMap(om)
	m, err := RequireConcreteMap(v, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Len() != 1 {
		t.Errorf("got Len=%d, want 1", m.Len())
	}
}

func TestRequireConcreteMap_TypeLiteral(t *testing.T) {
	_, err := RequireConcreteMap(NewTypeLiteral(TMap), "myop")
	if err == nil {
		t.Fatalf("expected error for type literal")
	}
	if !strings.Contains(err.Error(), "type literal") {
		t.Errorf("error %q does not say 'type literal'", err)
	}
}

func TestRequireConcreteMap_Carrier(t *testing.T) {
	_, err := RequireConcreteMap(NewCarrier(TMap), "myop")
	if err == nil {
		t.Fatalf("expected error for carrier")
	}
	if !strings.Contains(err.Error(), "carrier") {
		t.Errorf("error %q does not mention carrier", err)
	}
}

func TestRequireConcreteMap_NotAMap(t *testing.T) {
	// RecordType has Parent=TMap but AsMap returns nil — its payload
	// is RecordTypeInfo, not *OrderedMap.
	v := NewRecordType(NewOrderedMap())
	_, err := RequireConcreteMap(v, "myop")
	if err == nil {
		t.Fatalf("expected error for record type (non-OrderedMap payload)")
	}
	if !strings.Contains(err.Error(), "not a concrete map") {
		t.Errorf("error %q does not say 'not a concrete map'", err)
	}
}

// --- MapField* helpers ---

func newOptsMap(pairs ...[2]any) ReadMap {
	om := NewOrderedMap()
	for _, p := range pairs {
		key := p[0].(string)
		switch v := p[1].(type) {
		case string:
			om.Set(key, NewString(v))
		case int:
			om.Set(key, NewInteger(int64(v)))
		case int64:
			om.Set(key, NewInteger(v))
		case bool:
			om.Set(key, NewBoolean(v))
		case float64:
			om.Set(key, NewFloat(v))
		case Value:
			om.Set(key, v)
		}
	}
	return om
}

// MapFieldString

func TestMapFieldString_Hit(t *testing.T) {
	m := newOptsMap([2]any{"key", "hello"})
	s, ok := MapFieldString(m, "key")
	if !ok || s != "hello" {
		t.Errorf("got (%q, %v), want (\"hello\", true)", s, ok)
	}
}

func TestMapFieldString_MissingKey(t *testing.T) {
	m := newOptsMap([2]any{"other", "x"})
	s, ok := MapFieldString(m, "key")
	if ok || s != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", s, ok)
	}
}

func TestMapFieldString_WrongType(t *testing.T) {
	m := newOptsMap([2]any{"key", 42})
	s, ok := MapFieldString(m, "key")
	if ok || s != "" {
		t.Errorf("Integer at String slot: got (%q, %v), want (\"\", false)", s, ok)
	}
}

func TestMapFieldString_NilMap(t *testing.T) {
	s, ok := MapFieldString(nil, "key")
	if ok || s != "" {
		t.Errorf("nil map: got (%q, %v), want (\"\", false)", s, ok)
	}
}

func TestMapFieldString_TypeLiteralValue(t *testing.T) {
	// A type literal at the slot — Parent matches String but
	// AsString returns ("", error) because Data is nil.
	m := newOptsMap([2]any{"key", NewTypeLiteral(TString)})
	s, ok := MapFieldString(m, "key")
	if ok || s != "" {
		t.Errorf("type literal at String slot: got (%q, %v), want (\"\", false)", s, ok)
	}
}

// MapFieldInteger

func TestMapFieldInteger_Hit(t *testing.T) {
	m := newOptsMap([2]any{"n", 42})
	n, ok := MapFieldInteger(m, "n")
	if !ok || n != 42 {
		t.Errorf("got (%d, %v), want (42, true)", n, ok)
	}
}

func TestMapFieldInteger_Missing(t *testing.T) {
	n, ok := MapFieldInteger(newOptsMap(), "n")
	if ok || n != 0 {
		t.Errorf("got (%d, %v), want (0, false)", n, ok)
	}
}

func TestMapFieldInteger_WrongType(t *testing.T) {
	m := newOptsMap([2]any{"n", "hello"})
	_, ok := MapFieldInteger(m, "n")
	if ok {
		t.Errorf("String at Integer slot returned ok=true")
	}
}

func TestMapFieldInteger_NilMap(t *testing.T) {
	n, ok := MapFieldInteger(nil, "n")
	if ok || n != 0 {
		t.Errorf("nil map: got (%d, %v), want (0, false)", n, ok)
	}
}

func TestMapFieldInteger_DepScalarRejected(t *testing.T) {
	// DepInteger.Parent.ConformsTo(TInteger)=true via lattice override —
	// the helper must reject DepScalar payloads explicitly so the
	// caller doesn't get a zero-value silent miscompile.
	m := newOptsMap([2]any{"n", NewDepScalar(DepGTE, NewInteger(10))})
	_, ok := MapFieldInteger(m, "n")
	if ok {
		t.Errorf("DepInteger at Integer slot returned ok=true")
	}
}

func TestMapFieldInteger_TypeLiteralValue(t *testing.T) {
	m := newOptsMap([2]any{"n", NewTypeLiteral(TInteger)})
	_, ok := MapFieldInteger(m, "n")
	if ok {
		t.Errorf("type literal at Integer slot returned ok=true")
	}
}

// MapFieldBoolean

func TestMapFieldBoolean_HitTrue(t *testing.T) {
	m := newOptsMap([2]any{"b", true})
	b, ok := MapFieldBoolean(m, "b")
	if !ok || !b {
		t.Errorf("got (%v, %v), want (true, true)", b, ok)
	}
}

func TestMapFieldBoolean_HitFalse(t *testing.T) {
	m := newOptsMap([2]any{"b", false})
	b, ok := MapFieldBoolean(m, "b")
	if !ok || b {
		t.Errorf("got (%v, %v), want (false, true)", b, ok)
	}
}

func TestMapFieldBoolean_Missing(t *testing.T) {
	_, ok := MapFieldBoolean(newOptsMap(), "b")
	if ok {
		t.Errorf("missing key returned ok=true")
	}
}

func TestMapFieldBoolean_WrongType(t *testing.T) {
	m := newOptsMap([2]any{"b", "true"})
	_, ok := MapFieldBoolean(m, "b")
	if ok {
		t.Errorf("String at Boolean slot returned ok=true")
	}
}

func TestMapFieldBoolean_NilMap(t *testing.T) {
	_, ok := MapFieldBoolean(nil, "b")
	if ok {
		t.Errorf("nil map returned ok=true")
	}
}

func TestMapFieldBoolean_TypeLiteralValue(t *testing.T) {
	m := newOptsMap([2]any{"b", NewTypeLiteral(TBoolean)})
	_, ok := MapFieldBoolean(m, "b")
	if ok {
		t.Errorf("type literal at Boolean slot returned ok=true")
	}
}

// MapFieldFloat

func TestMapFieldFloat_Hit(t *testing.T) {
	m := newOptsMap([2]any{"f", 1.5})
	f, ok := MapFieldFloat(m, "f")
	if !ok || f != 1.5 {
		t.Errorf("got (%v, %v), want (1.5, true)", f, ok)
	}
}

func TestMapFieldFloat_Missing(t *testing.T) {
	_, ok := MapFieldFloat(newOptsMap(), "f")
	if ok {
		t.Errorf("missing key returned ok=true")
	}
}

func TestMapFieldFloat_WrongType(t *testing.T) {
	m := newOptsMap([2]any{"f", 42})
	_, ok := MapFieldFloat(m, "f")
	if ok {
		t.Errorf("Integer at Float slot returned ok=true")
	}
}

func TestMapFieldFloat_NilMap(t *testing.T) {
	_, ok := MapFieldFloat(nil, "f")
	if ok {
		t.Errorf("nil map returned ok=true")
	}
}

func TestMapFieldFloat_DepScalarRejected(t *testing.T) {
	m := newOptsMap([2]any{"f", NewDepScalar(DepGTE, NewFloat(1.5))})
	_, ok := MapFieldFloat(m, "f")
	if ok {
		t.Errorf("DepFloat at Float slot returned ok=true")
	}
}

func TestMapFieldFloat_TypeLiteralValue(t *testing.T) {
	m := newOptsMap([2]any{"f", NewTypeLiteral(TFloat)})
	_, ok := MapFieldFloat(m, "f")
	if ok {
		t.Errorf("type literal at Float slot returned ok=true")
	}
}

// --- TopOfDefStack ---

func TestTopOfDefStack_Empty(t *testing.T) {
	r, _ := NewRegistry()
	v, ok := r.Defs.Top("nonexistent")
	if ok {
		t.Errorf("missing name returned ok=true (v=%v)", v)
	}
}

func TestTopOfDefStack_SingleEntry(t *testing.T) {
	r, _ := NewRegistry()
	r.Defs.Push("x", NewInteger(42))
	v, ok := r.Defs.Top("x")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	n, _ := AsInteger(v)
	if n != 42 {
		t.Errorf("got %d, want 42", n)
	}
}

func TestTopOfDefStack_StackedReturnsTop(t *testing.T) {
	r, _ := NewRegistry()
	r.Defs.Push("x", NewInteger(1))
	r.Defs.Push("x", NewInteger(2))
	r.Defs.Push("x", NewInteger(3))
	v, ok := r.Defs.Top("x")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	n, _ := AsInteger(v)
	if n != 3 {
		t.Errorf("got %d, want 3 (top of stack)", n)
	}
}

func TestTopOfDefStack_NilDefTable(t *testing.T) {
	var dt *DefTable
	v, ok := dt.Top("x")
	if ok {
		t.Errorf("nil DefTable: ok=true, v=%v", v)
	}
}

// --- ResolveTypedName ---

func TestResolveTypedName_FallbackToDefStacks(t *testing.T) {
	r, _ := NewRegistry()
	r.Defs.Push("X", NewString("from-defstacks"))
	v, ok := r.ResolveTypedName("X")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	s, _ := AsString(v)
	if s != "from-defstacks" {
		t.Errorf("got %q, want \"from-defstacks\"", s)
	}
}

func TestResolveTypedName_NotFound(t *testing.T) {
	r, _ := NewRegistry()
	_, ok := r.ResolveTypedName("nonexistent")
	if ok {
		t.Errorf("missing name returned ok=true")
	}
}

func TestResolveTypedName_NilRegistry(t *testing.T) {
	var r *Registry
	_, ok := r.ResolveTypedName("X")
	if ok {
		t.Errorf("nil registry returned ok=true")
	}
}

// --- AsConcreteX accessors ---

func TestAsConcreteString_Concrete(t *testing.T) {
	s, err := NewString("hi").AsConcreteString()
	if err != nil || s != "hi" {
		t.Errorf("got (%q, %v), want (\"hi\", nil)", s, err)
	}
}

func TestAsConcreteString_DepScalarRejected(t *testing.T) {
	v := NewDepScalar(DepLT, NewString("z"))
	_, err := v.AsConcreteString()
	if err == nil {
		t.Errorf("expected error for DepString")
	}
}

func TestAsConcreteInteger_Concrete(t *testing.T) {
	n, err := NewInteger(42).AsConcreteInteger()
	if err != nil || n != 42 {
		t.Errorf("got (%d, %v), want (42, nil)", n, err)
	}
}

func TestAsConcreteInteger_DepScalarRejected(t *testing.T) {
	v := NewDepScalar(DepGT, NewInteger(10))
	_, err := v.AsConcreteInteger()
	if err == nil {
		t.Errorf("expected error for DepInteger")
	}
}

func TestAsConcreteFloat_Concrete(t *testing.T) {
	f, err := NewFloat(1.5).AsConcreteFloat()
	if err != nil || f != 1.5 {
		t.Errorf("got (%v, %v), want (1.5, nil)", f, err)
	}
}

func TestAsConcreteFloat_DepScalarRejected(t *testing.T) {
	v := NewDepScalar(DepGTE, NewFloat(1.5))
	_, err := v.AsConcreteFloat()
	if err == nil {
		t.Errorf("expected error for DepFloat")
	}
}

func TestAsConcreteBoolean_Concrete(t *testing.T) {
	b, err := NewBoolean(true).AsConcreteBoolean()
	if err != nil || !b {
		t.Errorf("got (%v, %v), want (true, nil)", b, err)
	}
}

func TestAsConcreteBoolean_DepScalarRejected(t *testing.T) {
	v := NewDepScalar(DepGTE, NewBoolean(true))
	_, err := v.AsConcreteBoolean()
	if err == nil {
		t.Errorf("expected error for DepBoolean")
	}
}

func TestAsConcreteAtom_Concrete(t *testing.T) {
	a, err := NewAtom("foo").AsConcreteAtom()
	if err != nil || a != "foo" {
		t.Errorf("got (%q, %v), want (\"foo\", nil)", a, err)
	}
}

func TestAsConcreteAtom_DepScalarRejected(t *testing.T) {
	v := NewDepScalar(DepGTE, NewAtom("a"))
	_, err := v.AsConcreteAtom()
	if err == nil {
		t.Errorf("expected error for DepAtom")
	}
}

// --- ResolveTypedNameValue ---

func TestResolveTypedNameValue_NotAWord(t *testing.T) {
	r, _ := NewRegistry()
	v := NewInteger(42)
	out, name, ok := r.ResolveTypedNameValue(v)
	if !ok {
		t.Errorf("non-word value: ok=false, want true (no resolution attempted)")
	}
	if name != "" {
		t.Errorf("non-word value: name=%q, want empty", name)
	}
	n, _ := AsInteger(out)
	if n != 42 {
		t.Errorf("non-word value: out modified (got %d)", n)
	}
}

func TestResolveTypedNameValue_WordResolved(t *testing.T) {
	r, _ := NewRegistry()
	r.Defs.Push("Bbd", NewString("from-types"))
	out, name, ok := r.ResolveTypedNameValue(NewWord("Bbd"))
	if !ok || name != "Bbd" {
		t.Errorf("got (name=%q, ok=%v), want (\"Bbd\", true)", name, ok)
	}
	s, _ := AsString(out)
	if s != "from-types" {
		t.Errorf("resolved value = %q, want \"from-types\"", s)
	}
}

func TestResolveTypedNameValue_WordUnresolved(t *testing.T) {
	r, _ := NewRegistry()
	w := NewWord("Unknown")
	out, name, ok := r.ResolveTypedNameValue(w)
	if ok {
		t.Errorf("unresolved word: ok=true, want false")
	}
	if name != "Unknown" {
		t.Errorf("unresolved: name=%q, want \"Unknown\"", name)
	}
	// out should be the original Word (unchanged) so callers can
	// continue with it.
	if !IsWord(out) {
		t.Errorf("unresolved word: out.IsWord=false")
	}
}

// --- RunPredicate ---

func TestRunPredicate_NotAFn(t *testing.T) {
	r, _ := NewRegistry()
	_, _, err := r.RunPredicate(NewInteger(1), NewInteger(42))
	if err == nil {
		t.Fatalf("expected error for non-fn constraint")
	}
}

func TestRunPredicate_BadPayload(t *testing.T) {
	r, _ := NewRegistry()
	// Post Step 5g: payload is a sealed interface. A wrong-shape
	// payload (a StrPayload for a TFunction-typed Value) is the
	// closest we can express to "not a FnDefInfo" — the value
	// satisfies Payload but is the wrong variant. RunPredicate must
	// detect the mismatch at runtime.
	v := Value{Parent: TFunction, Data: StrPayload{S: "not a FnDefInfo"}}
	_, _, err := r.RunPredicate(v, NewInteger(42))
	if err == nil {
		t.Fatalf("expected error for invalid FnDef payload")
	}
}

// A predicate with no signature, or none that takes one value, admits
// nothing: membership is a one-value application, and no parameter count
// raises (NUR100).
func TestRunPredicate_ZeroArgPredicate(t *testing.T) {
	r, _ := NewRegistry()
	v := Value{Parent: TFunction, Data: FnDefInfo{}}
	if _, matched, err := r.RunPredicate(v, NewInteger(42)); err != nil || matched {
		t.Fatalf("a sig-less predicate admits nothing: %v %v", matched, err)
	}
}

func TestRunPredicate_MultiArgPredicate(t *testing.T) {
	r, _ := NewRegistry()
	v := Value{Parent: TFunction, Data: FnDefInfo{
		Signatures: []FnSig{{Params: []FnParam{{Type: TAny}, {Type: TAny}}, BarrierPos: -1}},
	}}
	if _, matched, err := r.RunPredicate(v, NewInteger(42)); err != nil || matched {
		t.Fatalf("a two-param predicate admits nothing: %v %v", matched, err)
	}
}

// Happy-path tests for RunPredicate live in lang/go/test/type_*_test.go
// (they need the full parse pipeline to construct predicates).
// Coverage at the unit level is satisfied by the four error-path
// tests above — every branch of RunPredicate's pre-CallBoru logic is
// exercised. The CallBoru → result-shape branches are reachable only
// via real predicate bodies, hence the integration-level coverage.

// --- FlattenDisjunctAlts ---

func TestFlattenDisjunctAlts_NotADisjunct(t *testing.T) {
	v := NewInteger(42)
	alts := FlattenDisjunctAlts(v)
	if len(alts) != 1 {
		t.Fatalf("got %d alts, want 1", len(alts))
	}
	n, _ := AsInteger(alts[0])
	if n != 42 {
		t.Errorf("got %d, want 42 (the original value)", n)
	}
}

func TestFlattenDisjunctAlts_Disjunct(t *testing.T) {
	v := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	alts := FlattenDisjunctAlts(v)
	if len(alts) != 2 {
		t.Fatalf("got %d alts, want 2", len(alts))
	}
	// alts[i] is a type literal; its denoted lattice node is &alts[i].
	a0, a1 := alts[0], alts[1]
	if !(&a0).Equal(TInteger) || !(&a1).Equal(TString) {
		t.Errorf("got types [%v, %v], want [Integer, String]", a0.String(), a1.String())
	}
}

func TestFlattenDisjunctAlts_TypeLiteral(t *testing.T) {
	// A bare type literal (e.g., Integer) — not a disjunct, so the
	// helper wraps it in a single-element slice.
	v := NewTypeLiteral(TInteger)
	alts := FlattenDisjunctAlts(v)
	if len(alts) != 1 {
		t.Fatalf("got %d alts, want 1", len(alts))
	}
	a0 := alts[0]
	if !(&a0).Equal(TInteger) {
		t.Errorf("got %v, want Integer literal", a0.String())
	}
}

func TestFlattenDisjunctAlts_DisjunctWithBadPayload(t *testing.T) {
	// Construct a Value that claims to be a disjunct but has a
	// payload that AsDisjunct can't unwrap. The helper should
	// fall back gracefully to a single-element slice rather than
	// returning nil or panicking.
	v := Value{Parent: TDisjunct, Data: StrPayload{S: "not a DisjunctInfo"}}
	alts := FlattenDisjunctAlts(v)
	if len(alts) != 1 {
		t.Fatalf("got %d alts, want 1 (graceful fallback)", len(alts))
	}
}
