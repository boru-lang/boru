package native

import (
	"testing"
)

func TestValueStringTypes(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"integer", NewInteger(42), "42"},
		{"string", NewString("hello"), "'hello'"},
		{"empty_string", NewString(""), "''"},
		{"bool_true", NewBoolean(true), "true"},
		{"bool_false", NewBoolean(false), "false"},
		{"atom", NewAtom("foo"), "foo"},
		{"none_literal", NewTypeLiteral(TNone), "None"},
		{"number_literal", NewTypeLiteral(TNumber), "Number"},
		{"string_literal", NewTypeLiteral(TString), "String"},
		{"word", NewWord("upper"), "word(upper)"},
		{"open_paren", NewOpenParen(), "("},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.val.String()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValueStringListOrig(t *testing.T) {
	v := NewList([]Value{NewInteger(1), NewString("a")})
	got := v.String()
	if got != "[1 'a']" {
		t.Errorf("got %q, want %q", got, "[1 'a']")
	}
}

func TestValueStringMapOrig(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	m.Set("y", NewString("hi"))
	v := NewMap(m)
	got := v.String()
	if got != "{x:1 y:'hi'}" {
		t.Errorf("got %q, want %q", got, "{x:1 y:'hi'}")
	}
}

func TestValueStringTypedListOrig(t *testing.T) {
	v := NewTypedList(NewTypeLiteral(TString))
	got := v.String()
	if got != "[:String]" {
		t.Errorf("got %q, want %q", got, "[:String]")
	}
}

func TestValueStringTypedMapOrig(t *testing.T) {
	v := NewTypedMap(NewTypeLiteral(TNumber))
	got := v.String()
	if got != "{:Number}" {
		t.Errorf("got %q, want %q", got, "{:Number}")
	}
}

func TestValueStringRecordTypeOrig(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	v := NewRecordType(fields)
	got := v.String()
	if got != "record{x:Number y:String}" {
		t.Errorf("got %q, want %q", got, "record{x:Number y:String}")
	}
}

func TestValueStringTableTypeOrig(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TNumber))
	v := NewTableType(RecordTypeInfo{Fields: fields})
	got := v.String()
	if got != "table{a:Number}" {
		t.Errorf("got %q, want %q", got, "table{a:Number}")
	}
}

func TestValueStringDisjunctOrig(t *testing.T) {
	v := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	got := v.String()
	if got != "String tor None" {
		t.Errorf("got %q, want %q", got, "String tor None")
	}
}

func TestValueStringForward(t *testing.T) {
	v := NewForward(ForwardInfo{FuncName: "add", CollectedArgs: 1, ExpectedArgs: 2})
	got := v.String()
	if got != "forward(add,1/2)" {
		t.Errorf("got %q, want %q", got, "forward(add,1/2)")
	}
}

func TestNewFunction(t *testing.T) {
	v := NewFunction(FnDefInfo{})
	if !ParentOf(&v).Equal(TFunction) {
		t.Errorf("expected fndef type, got %s", ParentOf(&v))
	}
}

func TestNewDisjunct(t *testing.T) {
	v := NewDisjunct([]Value{NewTypeLiteral(TString)})
	if !IsDisjunct(&v) {
		t.Error("expected IsDisjunct to be true")
	}
	di, _ := AsDisjunct(v)
	if len(di.Alternatives) != 1 {
		t.Errorf("expected 1 alternative, got %d", len(di.Alternatives))
	}
}

func TestIsBoolean(t *testing.T) {
	if !IsBoolean(addrOf(NewBoolean(true))) {
		t.Error("expected true to be boolean")
	}
	if !IsBoolean(addrOf(NewBoolean(false))) {
		t.Error("expected false to be boolean")
	}
	if IsBoolean(addrOf(NewInteger(1))) {
		t.Error("integer should not be boolean")
	}
}

func TestAsTableType(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	v := NewTableType(RecordTypeInfo{Fields: fields})
	tt, _ := AsTableType(v)
	if tt.Record.Fields.Len() != 1 {
		t.Errorf("expected 1 field, got %d", tt.Record.Fields.Len())
	}
}

func TestAsChildType(t *testing.T) {
	v := NewTypedList(NewTypeLiteral(TString))
	ct, _ := AsChildType(v)
	// ct.Child is a String type literal; its denoted lattice node is
	// the value itself (a by-value copy of TString).
	if !(&ct.Child).Equal(TString) {
		t.Errorf("expected string child, got %s", ct.Child.String())
	}
}

func TestOrderedMapSortedKeys(t *testing.T) {
	m := NewOrderedMap()
	m.Set("c", NewInteger(3))
	m.Set("a", NewInteger(1))
	m.Set("b", NewInteger(2))

	keys := m.SortedKeys()
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("SortedKeys = %v, want [a b c]", keys)
	}
}
