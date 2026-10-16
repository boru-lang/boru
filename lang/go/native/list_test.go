package native

import (
	"testing"
)

// makeTable creates a table Value with the given column names and rows.
// Each row is a map with the column names as keys.
func makeTable(columns []string, rows [][]Value) Value {
	fields := NewOrderedMap()
	for _, col := range columns {
		fields.Set(col, NewTypeLiteral(TAny))
	}
	rec := RecordTypeInfo{Fields: fields}

	var rowValues []Value
	for _, row := range rows {
		m := NewOrderedMap()
		for i, col := range columns {
			m.Set(col, row[i])
		}
		rowValues = append(rowValues, NewMap(m))
	}

	return ValueOf(&Datum{Parent: TList, Data: TableData{
		Record: rec,
		Rows:   rowValues,
	}})
}

func TestListAllHandler(t *testing.T) {
	table := makeTable(
		[]string{"name", "age"},
		[][]Value{
			{NewString("alice"), NewInteger(30)},
			{NewString("bob"), NewInteger(25)},
		},
	)

	result, err := listAllHandler([]Value{table}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Errorf("expected 2 rows, got %d", len(list))
	}
}

func TestListFilterHandler(t *testing.T) {
	table := makeTable(
		[]string{"name", "age", "city"},
		[][]Value{
			{NewString("alice"), NewInteger(30), NewString("london")},
			{NewString("bob"), NewInteger(25), NewString("paris")},
			{NewString("carol"), NewInteger(30), NewString("london")},
		},
	)

	// Filter by city == "london"
	filter := NewOrderedMap()
	filter.Set("city", NewString("london"))

	result, err := listFilterHandler(
		[]Value{NewMap(filter), table},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Errorf("expected 2 matching rows, got %d", len(list))
	}

	// Check the names
	for _, row := range list {
		m, _ := AsMap(row)
		nameVal, _ := m.Get("name")
		name, _ := AsString(nameVal)
		if name != "alice" && name != "carol" {
			t.Errorf("unexpected name: %s", name)
		}
	}
}

func TestListFilterMultipleKeys(t *testing.T) {
	table := makeTable(
		[]string{"name", "age", "city"},
		[][]Value{
			{NewString("alice"), NewInteger(30), NewString("london")},
			{NewString("bob"), NewInteger(25), NewString("paris")},
			{NewString("carol"), NewInteger(30), NewString("paris")},
		},
	)

	// Filter by age == 30 AND city == "london"
	filter := NewOrderedMap()
	filter.Set("age", NewInteger(30))
	filter.Set("city", NewString("london"))

	result, err := listFilterHandler(
		[]Value{NewMap(filter), table},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 1 {
		t.Errorf("expected 1 matching row, got %d", len(list))
	}
	if len(list) > 0 {
		m, _ := AsMap(list[0])
		nameVal, _ := m.Get("name")
		ns, _ := AsString(nameVal)
		if ns != "alice" {
			t.Errorf("expected alice, got %s", ns)
		}
	}
}

func TestListFilterNoMatch(t *testing.T) {
	table := makeTable(
		[]string{"name", "age"},
		[][]Value{
			{NewString("alice"), NewInteger(30)},
		},
	)

	filter := NewOrderedMap()
	filter.Set("name", NewString("nobody"))

	result, err := listFilterHandler(
		[]Value{NewMap(filter), table},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 matching rows, got %d", len(list))
	}
}

func TestListFilterMissingField(t *testing.T) {
	table := makeTable(
		[]string{"name", "age"},
		[][]Value{
			{NewString("alice"), NewInteger(30)},
		},
	)

	// Filter on field that doesn't exist in records
	filter := NewOrderedMap()
	filter.Set("city", NewString("london"))

	result, err := listFilterHandler(
		[]Value{NewMap(filter), table},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 matching rows, got %d", len(list))
	}
}

func TestListAllEmptyTable(t *testing.T) {
	table := makeTable([]string{"name"}, nil)

	result, err := listAllHandler([]Value{table}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

func TestValuesEqual(t *testing.T) {
	tests := []struct {
		a, b Value
		want bool
	}{
		{NewInteger(1), NewInteger(1), true},
		{NewInteger(1), NewInteger(2), false},
		{NewString("a"), NewString("a"), true},
		{NewString("a"), NewString("b"), false},
		{NewBoolean(true), NewBoolean(true), true},
		{NewBoolean(true), NewBoolean(false), false},
		{NewAtom("x"), NewAtom("x"), true},
		{NewAtom("x"), NewString("x"), true},
		{NewString("x"), NewAtom("x"), true},
	}
	for i, tt := range tests {
		got := valuesEqual(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("test %d: valuesEqual(%s, %s) = %v, want %v",
				i, tt.a, tt.b, got, tt.want)
		}
	}
}
