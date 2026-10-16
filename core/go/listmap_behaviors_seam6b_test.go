package core

// Seam-6b cluster B5: unit tests exercising previously-unreached blocks
// in coretype_list_map_behaviors.go (the List/Map format Behaviors and
// the resugarDotChain display helper). All calls are in-package and
// direct, per design/TEST-SEAMS.10.md. No global state is touched.

import (
	"errors"
	"strings"
	"testing"
)

// s6b5MatPayload is a test-only Payload that itself implements the
// Materializer interface (unlike MaterializerPayload, which WRAPS a
// Materializer). It drives listFormatBehavior.Format's direct
// `v.Data.(Materializer)` branch, which no production payload variant
// reaches.
type s6b5MatPayload struct {
	td  TableData
	err error
}

func (s6b5MatPayload) payloadMarker()                    {}
func (s6b5MatPayload) IsTypeContent(*Type) bool          { return false }
func (m s6b5MatPayload) Materialize() (TableData, error) { return m.td, m.err }
func (m s6b5MatPayload) SourceRecord() RecordTypeInfo    { return m.td.Record }

func TestS6b5ListFormatDirectMaterializer(t *testing.T) {
	b := listFormatBehavior{}

	// Error arm: Materialize fails → query(error:…).
	bad := NewValueRaw(TList, s6b5MatPayload{err: errors.New("s6b5-boom")})
	if got := b.Format(bad); got != "query(error:s6b5-boom)" {
		t.Errorf("Format(direct materializer, error) = %q", got)
	}

	// Success arm: renders via formatTableDataAsList.
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	ok := NewValueRaw(TList, s6b5MatPayload{td: TableData{
		Record: RecordTypeInfo{Fields: fields},
	}})
	got := b.Format(ok)
	if !strings.HasPrefix(got, "table{") {
		t.Errorf("Format(direct materializer, ok) = %q, want a table render", got)
	}
}

func TestS6b5ListFormatResugarsDotChain(t *testing.T) {
	// A quoted-code list containing a parser-grouped dot chain must
	// render back as recv.key via the resugar path in Format.
	lst := NewList([]Value{
		NewOpenParen(), NewWord("m"), NewWord("get"), NewWord("k"), NewCloseParen(),
	})
	if got := (listFormatBehavior{}).Format(lst); got != "[m.k]" {
		t.Errorf("Format(dot chain) = %q, want [m.k]", got)
	}
}

func TestS6b5ResugarDotChainShapes(t *testing.T) {
	open, close_ := NewOpenParen(), NewCloseParen()
	w := NewWord

	// Multi-segment chain mixing get and getr renders . and !. segments.
	elems := []Value{open, w("m"), w("get"), w("a"), w("getr"), w("b"), close_}
	s, end, ok := resugarDotChain(elems, 0)
	if !ok || s != "m.a!.b" || end != 6 {
		t.Errorf("resugarDotChain(get+getr) = %q, %d, %v; want m.a!.b, 6, true", s, end, ok)
	}

	// A non-get verb stops the walk with no segment consumed → no match.
	if _, _, ok := resugarDotChain([]Value{open, w("m"), w("add"), w("k"), close_}, 0); ok {
		t.Error("a non-get verb must not resugar")
	}

	// A computed (non-word) key leaves the group verbatim.
	if _, _, ok := resugarDotChain([]Value{open, w("m"), w("get"), NewInteger(5), close_}, 0); ok {
		t.Error("a non-word key must not resugar")
	}

	// A chain with no clean closing paren does not match.
	if _, _, ok := resugarDotChain([]Value{open, w("m"), w("get"), w("k")}, 0); ok {
		t.Error("a chain without a closing paren must not resugar")
	}

	// A word after the last key that is not a close paren does not match.
	if _, _, ok := resugarDotChain([]Value{open, w("m"), w("get"), w("k"), NewInteger(9)}, 0); ok {
		t.Error("a non-close tail must not resugar")
	}
}

func TestS6b5MapFormatNonMapPayload(t *testing.T) {
	// Parent says Map but the payload is not readable as one: the
	// Behavior renders the empty-map form instead of panicking.
	bad := MakeValue(TMap, ListPayload{})
	if got := (mapFormatBehavior{}).Format(bad); got != "{}" {
		t.Errorf("Format(non-map payload) = %q, want {}", got)
	}
}
