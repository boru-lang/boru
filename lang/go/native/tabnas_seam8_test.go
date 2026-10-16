package native

import (
	"strings"
	"testing"
)

// --- tabnasAnyToValue scalar + fallback branches ---

func TestW8TabnasAnyToValueIntAndDefault(t *testing.T) {
	// `int` case -> Integer.
	v := tabnasAnyToValue(int(5))
	if n, _ := AsInteger(v); n != 5 {
		t.Errorf("int(5) -> %v, want Integer 5", v)
	}
	// unrecognised shape -> string form (a float32 is not one of the
	// recognised cases).
	v = tabnasAnyToValue(float32(1.5))
	if s, _ := AsString(v); !strings.Contains(s, "1.5") {
		t.Errorf("float32 fallback -> %q, want a string containing 1.5", s)
	}
}

// --- tabnasXmlToValue non-string/non-map child (default arm) ---

func TestW8TabnasXmlValueOtherChild(t *testing.T) {
	m := map[string]any{
		"name":     "tag",
		"children": []any{42}, // neither string nor map[string]any
	}
	v := tabnasXmlToValue(m)
	// A well-formed element value is produced without panicking; the
	// numeric child was stringified.
	if !v.Parent().ConformsTo(TXml) {
		t.Errorf("expected an Xml element value, got %s", v.Parent())
	}
}

// --- tabnasXmlToValue non-element top level (the generic fallback) ---

// The `xml` kind is the only caller, and since tabnas/xml v0.7.1 a source
// that does not denote an element is a parse ERROR rather than a nil result,
// so the kind's own path no longer reaches this arm. It is covered directly
// instead of allowlisted: the arm is reachable as a function, and what it
// must do — hand a non-element top level to the GENERIC conversion rather
// than mint a malformed Xml element — is worth pinning. Before v0.7.1 the
// decoder returned `(nil, nil)` for `""`, `"hello"` and `"<!-- c -->"`, and
// those sources covered this arm through `parse xml`; a future decoder that
// reintroduces a non-element success would land here again.
func TestW8TabnasXmlValueNonElementTop(t *testing.T) {
	// A string top level: generic conversion, NOT an Xml element.
	v := tabnasXmlToValue("hello")
	if s, _ := AsString(v); s != "hello" || !v.Is(TString) {
		t.Errorf("non-map top level -> %s, want String hello", v.String())
	}
	if v.Is(TXml) {
		t.Errorf("non-map top level must not become an Xml element, got %s", v.String())
	}
	// The nil top level the pre-v0.7.1 decoder produced: None, not a panic.
	if got := tabnasXmlToValue(nil); !got.Is(TNone) {
		t.Errorf("nil top level -> %s, want None", got.String())
	}
}

// --- jsonRoundTrip marshal-error path ---

func TestW8JsonRoundTripMarshalError(t *testing.T) {
	// A channel cannot be marshalled by encoding/json.
	if _, err := jsonRoundTrip(make(chan int)); err == nil {
		t.Fatal("jsonRoundTrip of an unmarshalable value must error")
	}
}

// --- feed parse error (the j.Parse error arm inside the feed kind) ---

func TestW8TabnasFeedParseError(t *testing.T) {
	var feed TabnasKind
	for _, k := range TabnasKinds() {
		if k.Name == "feed" {
			feed = k
		}
	}
	if feed.Parse == nil {
		t.Fatal("feed kind not found")
	}
	if _, err := feed.Parse("<<<>>>", nil); err == nil {
		t.Fatal("feed.Parse of malformed input must error")
	}
}
