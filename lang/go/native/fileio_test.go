package native

import "testing"

func TestNormalizeLineEndings(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no_change", "hello\nworld", "hello\nworld"},
		{"crlf_to_lf", "hello\r\nworld", "hello\nworld"},
		{"cr_to_lf", "hello\rworld", "hello\nworld"},
		{"mixed", "a\r\nb\rc\n", "a\nb\nc\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeLineEndings(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDenormalizeLineEndings(t *testing.T) {
	tests := []struct {
		name  string
		input string
		nl    string
		want  string
	}{
		{"crlf", "a\nb\n", "crlf", "a\r\nb\r\n"},
		{"lf_default", "a\nb", "lf", "a\nb"},
		{"unknown", "a\nb", "xyz", "a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := denormalizeLineEndings(tt.input, tt.nl)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyNL(t *testing.T) {
	tests := []struct {
		name    string
		content string
		nl      string
		want    string
	}{
		{"lf_normalizes", "a\r\nb", "lf", "a\nb"},
		{"crlf_normalizes", "a\r\nb\nc", "crlf", "a\r\nb\r\nc"},
		{"raw_preserves", "a\r\nb", "raw", "a\r\nb"},
		{"default_normalizes", "a\r\nb", "", "a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyNL(tt.content, tt.nl)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseFileOpts(t *testing.T) {
	// Default opts from non-map value
	enc, format, mode, nl, _, _ := parseFileOpts(NewInteger(0))
	if enc != "utf8" || format != "text" || mode != "write" || nl != "lf" {
		t.Errorf("defaults wrong: enc=%s fmt=%s mode=%s nl=%s", enc, format, mode, nl)
	}

	// Custom opts
	m := NewOrderedMap()
	m.Set("enc", NewString("binary"))
	m.Set("fmt", NewString("json"))
	m.Set("mode", NewString("append"))
	m.Set("nl", NewString("crlf"))
	enc, format, mode, nl, _, _ = parseFileOpts(NewMap(m))
	if enc != "binary" {
		t.Errorf("enc = %s, want binary", enc)
	}
	if format != "json" {
		t.Errorf("fmt = %s, want json", format)
	}
	if mode != "append" {
		t.Errorf("mode = %s, want append", mode)
	}
	if nl != "crlf" {
		t.Errorf("nl = %s, want crlf", nl)
	}
}

func TestParseFileOptsPartial(t *testing.T) {
	m := NewOrderedMap()
	m.Set("fmt", NewString("lines"))
	enc, format, mode, nl, _, _ := parseFileOpts(NewMap(m))
	if enc != "utf8" || format != "lines" || mode != "write" || nl != "lf" {
		t.Errorf("partial opts wrong: enc=%s fmt=%s mode=%s nl=%s", enc, format, mode, nl)
	}
}

func TestSortedMapKeys(t *testing.T) {
	m := map[string]any{"c": 3, "a": 1, "b": 2}
	keys := sortedMapKeys(m)
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("got %v, want [a b c]", keys)
	}
}

func TestJsonicToValue(t *testing.T) {
	// nil → none
	v, err := jsonicToValue(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !IsNoneShape(v) {
		t.Errorf("expected none, got %s", v.String())
	}

	// bool
	v, err = jsonicToValue(true)
	if err != nil {
		t.Fatal(err)
	}
	_as0, _ := AsBoolean(v)
	if !_as0 {
		t.Error("expected true")
	}

	// float64
	v, err = jsonicToValue(float64(42))
	if err != nil {
		t.Fatal(err)
	}
	_as1, _ := AsInteger(v)
	if _as1 != 42 {
		_as2, _ := AsInteger(v)
		t.Errorf("expected 42, got %d", _as2)
	}

	// string
	v, err = jsonicToValue("hello")
	if err != nil {
		t.Fatal(err)
	}
	_as3, _ := AsString(v)
	if _as3 != "hello" {
		_as4, _ := AsString(v)
		t.Errorf("expected hello, got %s", _as4)
	}

	// list
	v, err = jsonicToValue([]any{float64(1), "two"})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Parent().Equal(TList) {
		t.Errorf("expected list, got %s", v.Parent())
	}

	// map
	v, err = jsonicToValue(map[string]any{"x": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Parent().Equal(TMap) {
		t.Errorf("expected map, got %s", v.Parent())
	}

	// unsupported type
	_, err = jsonicToValue(complex(1, 2))
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestValueToJsonic(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"string", NewString("hi"), `"hi"`},
		{"int", NewInteger(42), "42"},
		{"bool_true", NewBoolean(true), "true"},
		{"bool_false", NewBoolean(false), "false"},
		{"none", NewTypeLiteral(TNone), "null"},
		{"atom", NewAtom("foo"), `"foo"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := valueToJsonic(tt.val)
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestValueToJsonicList(t *testing.T) {
	v := NewList([]Value{NewInteger(1), NewString("a")})
	got := valueToJsonic(v)
	if got != `[1,"a"]` {
		t.Errorf("got %s, want [1,\"a\"]", got)
	}
}

func TestValueToJsonicMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	v := NewMap(m)
	got := valueToJsonic(v)
	if got != `{"x":1}` {
		t.Errorf("got %s, want {\"x\":1}", got)
	}
}

func TestValueToJsonicEmptyList(t *testing.T) {
	// A typed list (ChildTypeInfo data) should produce "[]"
	v := NewTypedList(NewTypeLiteral(TString))
	got := valueToJsonic(v)
	if got != "[]" {
		t.Errorf("got %s, want []", got)
	}
}

func TestValueToJsonicEmptyMap(t *testing.T) {
	// A typed map (ChildTypeInfo data) should produce "{}"
	v := NewTypedMap(NewTypeLiteral(TString))
	got := valueToJsonic(v)
	if got != "{}" {
		t.Errorf("got %s, want {}", got)
	}
}
