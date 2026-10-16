package native

import (
	"strings"
	"testing"
)

func TestValueStringWord(t *testing.T) {
	v := NewWord("test")
	if !strings.Contains(v.String(), "test") {
		t.Errorf("expected word(test), got %s", v.String())
	}
}

func TestValueStringOpenParen(t *testing.T) {
	v := NewOpenParen()
	if !strings.Contains(v.String(), "(") {
		t.Errorf("expected word containing (, got %s", v.String())
	}
}

func TestValueStringFloatCov(t *testing.T) {
	v := NewFloat(3.14)
	if v.String() != "3.14" {
		t.Errorf("expected 3.14, got %s", v.String())
	}
}

func TestValueStringIntegerCov(t *testing.T) {
	v := NewInteger(42)
	if v.String() != "42" {
		t.Errorf("expected 42, got %s", v.String())
	}
}

func TestValueStringBoolTrue(t *testing.T) {
	v := NewBoolean(true)
	if v.String() != "true" {
		t.Errorf("expected true, got %s", v.String())
	}
}

func TestValueStringBoolFalse(t *testing.T) {
	v := NewBoolean(false)
	if v.String() != "false" {
		t.Errorf("expected false, got %s", v.String())
	}
}

func TestValueStringStringCov(t *testing.T) {
	v := NewString("hello")
	if v.String() != "'hello'" {
		t.Errorf("expected 'hello', got %s", v.String())
	}
}

func TestValueStringAtomCov(t *testing.T) {
	v := NewAtom("myatom")
	if v.String() != "myatom" {
		t.Errorf("expected myatom, got %s", v.String())
	}
}

func TestValueStringTypeLiteralCov(t *testing.T) {
	v := NewTypeLiteral(TNumber)
	s := v.String()
	if s == "" {
		t.Error("expected non-empty type literal string")
	}
}

func TestValueStringTableDataCov(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	row := NewOrderedMap()
	row.Set("x", NewInteger(1))
	td := TableData{
		Record: RecordTypeInfo{Fields: fields},
		Rows:   []Value{NewMap(row)},
	}
	v := MakeValue(TList, td)
	s := v.String()
	if !strings.HasPrefix(s, "table{") {
		t.Errorf("expected table{...}, got %s", s)
	}
}

func TestValueAsNumberCov(t *testing.T) {
	v := NewInteger(42)
	_as0, _ := AsNumber(v)
	if _as0 != 42.0 {
		_as1, _ := AsNumber(v)
		t.Errorf("expected 42.0, got %f", _as1)
	}
	v = NewFloat(3.14)
	_as2, _ := AsNumber(v)
	if _as2 != 3.14 {
		_as3, _ := AsNumber(v)
		t.Errorf("expected 3.14, got %f", _as3)
	}
}

func TestValueAsTableTypeCov(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	tti := TableTypeInfo{Record: RecordTypeInfo{Fields: fields}}
	v := MakeValue(TList, tti)
	tt, _ := AsTableType(v)
	if tt.Record.Fields.Len() != 1 {
		t.Errorf("expected 1 field, got %d", tt.Record.Fields.Len())
	}
}

func TestValueAsListCov(t *testing.T) {
	v := NewList([]Value{NewInteger(1), NewInteger(2)})
	_lst, _ := AsList(v)
	list := _lst.Slice()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}

	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	row := NewOrderedMap()
	row.Set("x", NewInteger(1))
	td := TableData{
		Record: RecordTypeInfo{Fields: fields},
		Rows:   []Value{NewMap(row)},
	}
	v = MakeValue(TList, td)
	_lst2, _ := AsList(v)
	list = _lst2.Slice()
	if len(list) != 1 {
		t.Fatalf("expected 1 row, got %d", len(list))
	}
}
