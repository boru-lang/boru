package core

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached branches in unify_options.go. The family handler and its
// helpers (optionsDefault / optionsBaseType) are pure — driven by direct
// calls with crafted Options / Map / Disjunct values. Per
// design/TEST-SEAMS.10.md.

import "testing"

func w8opts(pairs ...Value) Value {
	m := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		key, _ := AsString(pairs[i])
		m.Set(key, pairs[i+1])
	}
	return NewOptionsType(m)
}

func TestW8UnifyOptionsPairSuccess(t *testing.T) {
	a := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	b := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	got, err := unifyOptionsFamily(a, Shape(a), b, Shape(b), nil)
	if err != nil {
		t.Fatalf("two compatible Options must unify: %v", err)
	}
	if !IsOptionsType(got) {
		t.Fatalf("result should be an Options type, got %v", got)
	}
}

func TestW8UnifyOptionsPairFieldBagError(t *testing.T) {
	// Same field-count, different key → unifyFieldBags reports the key
	// missing on the right (also covers unify_map.go's missing-key arm).
	a := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	b := w8opts(NewString("y"), NewTypeLiteral(TInteger))
	if _, err := unifyOptionsFamily(a, Shape(a), b, Shape(b), nil); err == nil {
		t.Fatal("Options with mismatched keys must fail to unify")
	}
}

func TestW8UnifyOptionsBareMapLiteral(t *testing.T) {
	// Options vs a bare Map type literal preserves the Options schema.
	opts := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	lit := NewTypeLiteral(TMap)
	got, err := unifyOptionsFamily(opts, Shape(opts), lit, Shape(lit), nil)
	if err != nil {
		t.Fatalf("Options vs Map literal: %v", err)
	}
	if !IsOptionsType(got) {
		t.Fatalf("expected Options schema preserved, got %v", got)
	}
}

func TestW8UnifyOptionsRejectsStructuralMap(t *testing.T) {
	// Options only unifies with a plain concrete Map, never a Record.
	opts := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	rf := NewOrderedMap()
	rf.Set("x", NewTypeLiteral(TInteger))
	rec := NewRecordType(rf)
	if !IsConcrete(rec) {
		t.Fatal("precondition: record type value must be concrete for this arm")
	}
	if _, err := unifyOptionsFamily(opts, Shape(opts), rec, Shape(rec), nil); err == nil {
		t.Fatal("Options must not unify with a Record")
	}
}

func TestW8UnifyOptionsConcreteOnRight(t *testing.T) {
	// sa==ShapeOptions branch with a concrete map on the right: the
	// present key unifies against its Options field constraint.
	opts := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	cm := NewOrderedMap()
	cm.Set("x", NewInteger(7))
	concrete := NewMap(cm)
	got, err := unifyOptionsFamily(opts, Shape(opts), concrete, Shape(concrete), nil)
	if err != nil {
		t.Fatalf("Options vs concrete map: %v", err)
	}
	m, _ := AsMap(got)
	if v, ok := m.Get("x"); !ok {
		t.Fatal("expected key x in result")
	} else if n, _ := AsInteger(v); n != 7 {
		t.Fatalf("expected x=7, got %d", n)
	}
}

// --- optionsDefault ---------------------------------------------------------

func TestW8OptionsDefaultDisjunctConcreteAlt(t *testing.T) {
	// No None alternative, but a concrete alternative → it is the default.
	d := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewInteger(42)})
	got, ok := optionsDefault(d)
	if !ok {
		t.Fatal("disjunct with a concrete alternative must yield a default")
	}
	if n, _ := AsInteger(got); n != 42 {
		t.Fatalf("expected default 42, got %d", n)
	}
}

func TestW8OptionsDefaultDisjunctNoConcrete(t *testing.T) {
	// No None and no concrete alternative → no default.
	d := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	if _, ok := optionsDefault(d); ok {
		t.Fatal("disjunct of bare type literals has no default")
	}
}

func TestW8OptionsDefaultNoneValue(t *testing.T) {
	// A bare None literal defaults to itself.
	got, ok := optionsDefault(NewTypeLiteral(TNone))
	if !ok {
		t.Fatal("None must be its own default")
	}
	if !IsNoneShape(got) {
		t.Fatalf("expected None default, got %v", got)
	}
}

// --- optionsBaseType --------------------------------------------------------

func TestW8OptionsBaseType(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		want *Type
	}{
		{"float", NewFloat(1.5), TFloat},
		{"string", NewString("s"), TString},
		{"boolean", NewBoolean(true), TBoolean},
		{"map", NewMap(NewOrderedMap()), TMap},
		{"list", NewList(nil), TList},
		{"none", NewNone(), TNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := optionsBaseType(c.v); !got.Equal(c.want) {
				t.Fatalf("optionsBaseType(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
	// Default arm: an Atom is not one of the well-known bases, so its
	// own Parent is returned unchanged.
	atom := NewAtom("a")
	if got := optionsBaseType(atom); !got.Equal(atom.Parent) {
		t.Fatalf("default arm should return the value's Parent, got %v", got)
	}
}

// TestW8FillConcreteOptionDefaults covers every branch of the
// materialize-concrete-defaults helper wired into execMatch.
func TestW8FillConcreteOptionDefaults(t *testing.T) {
	mapOf := func(pairs ...Value) Value {
		m := NewOrderedMap()
		for i := 0; i+1 < len(pairs); i += 2 {
			k, _ := AsString(pairs[i])
			m.Set(k, pairs[i+1])
		}
		return NewMap(m)
	}
	get := func(v Value, key string) (Value, bool) {
		m, err := AsMap(v)
		if err != nil || m == nil {
			return Value{}, false
		}
		return m.Get(key)
	}
	concreteOpts := w8opts(NewString("x"), NewInteger(1), NewString("y"), NewInteger(2))

	// 1. Non-Options pattern → returned unchanged.
	if out := FillConcreteOptionDefaults(NewInteger(0), mapOf(NewString("x"), NewInteger(9))); out.String() != mapOf(NewString("x"), NewInteger(9)).String() {
		t.Errorf("non-Options pattern should pass m through, got %s", out)
	}
	// 2. Options pattern with nil Fields → unchanged.
	if out := FillConcreteOptionDefaults(NewOptionsType(nil), mapOf(NewString("x"), NewInteger(9))); out.String() != mapOf(NewString("x"), NewInteger(9)).String() {
		t.Errorf("nil-Fields Options should pass m through, got %s", out)
	}
	// 3. m is not a plain map (AsMap nil) → unchanged.
	if out := FillConcreteOptionDefaults(concreteOpts, NewInteger(5)); out.String() != "5" {
		t.Errorf("non-map m should pass through, got %s", out)
	}
	// 4. One concrete default absent → materialized.
	out := FillConcreteOptionDefaults(concreteOpts, mapOf(NewString("x"), NewInteger(10)))
	if xv, ok := get(out, "x"); !ok {
		t.Error("x should remain present")
	} else if n, _ := AsInteger(xv); n != 10 {
		t.Errorf("x = %d, want 10 (caller value wins)", n)
	}
	if yv, ok := get(out, "y"); !ok {
		t.Error("y default should be materialized")
	} else if n, _ := AsInteger(yv); n != 2 {
		t.Errorf("y = %d, want 2 (concrete default)", n)
	}
	// 5. All absent → every default materialized (covers filled-init once
	// then reuse on the second fill).
	out = FillConcreteOptionDefaults(concreteOpts, mapOf())
	if xv, ok := get(out, "x"); !ok {
		t.Error("x default should be materialized")
	} else if n, _ := AsInteger(xv); n != 1 {
		t.Errorf("x = %d, want 1", n)
	}
	if _, ok := get(out, "y"); !ok {
		t.Error("y default should be materialized")
	}
	// 6. All present → returned unchanged (filled stays nil).
	full := mapOf(NewString("x"), NewInteger(10), NewString("y"), NewInteger(20))
	if out := FillConcreteOptionDefaults(concreteOpts, full); out.String() != full.String() {
		t.Errorf("all-present should pass through, got %s", out)
	}
	// 7. Optional-only (T tor None) field → NOT materialized (no concrete
	// default), and a required bare type-literal field → NOT materialized.
	optOpts := w8opts(
		NewString("a"), NewDisjunct([]Value{NewTypeLiteral(TBoolean), NewTypeLiteral(TNone)}),
		NewString("b"), NewTypeLiteral(TString),
	)
	out = FillConcreteOptionDefaults(optOpts, mapOf())
	if _, ok := get(out, "a"); ok {
		t.Error("None-union field must NOT be materialized")
	}
	if _, ok := get(out, "b"); ok {
		t.Error("required type-literal field must NOT be materialized")
	}
}

// TestW8FillDefaultsMutableIsolation (PR #284 P1): a mutable concrete
// default must be freshened per call — one caller's mutation must not
// leak into a later call or into the schema's stored default.
func TestW8FillDefaultsMutableIsolation(t *testing.T) {
	flexDefault := NewFlexMap(NewOrderedMap())
	schema := w8opts(NewString("m"), flexDefault)

	out1 := FillConcreteOptionDefaults(schema, NewMap(NewOrderedMap()))
	out2 := FillConcreteOptionDefaults(schema, NewMap(NewOrderedMap()))

	get := func(v Value, key string) Value {
		m, _ := AsMap(v)
		got, _ := m.Get(key)
		return got
	}
	// Mutate the flex default the FIRST call received (in-place, like `set`).
	m1, err := AsMutableMap(get(out1, "m"))
	if err != nil || m1 == nil {
		t.Fatalf("out1.m is not a concrete map: %v", err)
	}
	m1.Set("leaked", NewInteger(1))

	// The second call's default and the schema's stored default must be
	// unaffected (distinct copies).
	if m2, _ := AsMutableMap(get(out2, "m")); m2 == nil || m2.Len() != 0 {
		t.Errorf("out2.m leaked the mutation: len=%d, want 0", m2.Len())
	}
	if ms, _ := AsMutableMap(flexDefault); ms == nil || ms.Len() != 0 {
		t.Errorf("schema default leaked the mutation: len=%d, want 0", ms.Len())
	}
}

// TestW8FillDefaultsPreservesFlexMap (PR #284 P2): filling a default into
// a FlexMap argument must return a FlexMap, not a plain Map, so downstream
// `set` keeps in-place mutation.
func TestW8FillDefaultsPreservesFlexMap(t *testing.T) {
	schema := w8opts(NewString("x"), NewInteger(1))

	// Plain-Map input → plain Map result.
	plain := FillConcreteOptionDefaults(schema, NewMap(NewOrderedMap()))
	if plain.Parent.ConformsTo(TFlexMap) {
		t.Error("plain Map input must not become a FlexMap")
	}
	// FlexMap input → FlexMap result (flavor preserved through the fill).
	flexOut := FillConcreteOptionDefaults(schema, NewFlexMap(NewOrderedMap()))
	if !flexOut.Parent.ConformsTo(TFlexMap) {
		t.Errorf("FlexMap input lost its flavor: parent=%s", flexOut.Parent)
	}
	// The default was still materialized.
	if m, _ := AsMap(flexOut); m == nil {
		t.Fatal("flex result is not a concrete map")
	} else if xv, ok := m.Get("x"); !ok {
		t.Error("x default not materialized into the flex result")
	} else if n, _ := AsInteger(xv); n != 1 {
		t.Errorf("x = %d, want 1", n)
	}
}
