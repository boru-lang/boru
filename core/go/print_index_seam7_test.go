package core

// Seam-7 (cluster A4): in-package unit tests for previously-unreached
// blocks in print.go and indexcheck.go. Renderers are driven with crafted
// Values; the check-mode index helpers are called directly with
// DepScalar / concrete / malformed operands. Per design/TEST-SEAMS.10.md.

import (
	"errors"
	"strings"
	"testing"
)

// s7Mz is a test Materializer that can be stored directly as a payload
// (payloadMarker) or wrapped in a MaterializerPayload.
type s7Mz struct{ err error }

func (m s7Mz) Materialize() (TableData, error) {
	return TableData{Record: RecordTypeInfo{Fields: NewOrderedMap()}}, m.err
}
func (s7Mz) SourceRecord() RecordTypeInfo { return RecordTypeInfo{Fields: NewOrderedMap()} }
func (s7Mz) payloadMarker()               {}
func (s7Mz) IsTypeContent(*Type) bool     { return false }

// --- print.go -------------------------------------------------------------

func TestS7FormatForPrintMaterializer(t *testing.T) {
	boom := errors.New("boom")
	// MaterializerPayload, erroring.
	v := NewValueRaw(TList, MaterializerPayload{M: s7Mz{err: boom}})
	if got := FormatForPrint(v); !strings.Contains(got, "error") {
		t.Errorf("materializer-payload error render = %q", got)
	}
	// MaterializerPayload, succeeding -> formatTable.
	ok := NewValueRaw(TList, MaterializerPayload{M: s7Mz{}})
	if got := FormatForPrint(ok); got == "" {
		t.Error("materializer-payload ok render should not be empty")
	}
	// Direct Materializer payload, erroring.
	dv := NewValueRaw(TList, s7Mz{err: boom})
	if got := FormatForPrint(dv); !strings.Contains(got, "error") {
		t.Errorf("materializer error render = %q", got)
	}
	// Direct Materializer payload, succeeding.
	dok := NewValueRaw(TList, s7Mz{})
	if got := FormatForPrint(dok); got == "" {
		t.Error("materializer ok render should not be empty")
	}
}

func TestS7FormatForPrintOptions(t *testing.T) {
	opt := NewValueRaw(TMap, OptionsTypeInfo{Fields: NewOrderedMap()})
	if got := FormatForPrint(opt); got == "" {
		t.Error("options print should not be empty")
	}
}

func TestS7FormatMapJSONError(t *testing.T) {
	// AsMutableMap fails on a record payload -> "{}".
	rec := NewValueRaw(TMap, RecordTypeInfo{Fields: NewOrderedMap()})
	if got := formatMapJSON(rec); got != "{}" {
		t.Errorf("formatMapJSON on a non-mutable map = %q, want {}", got)
	}
}

func TestS7FormatValueJSONArms(t *testing.T) {
	if got := FormatValueJSON(NewTypeLiteral(TInteger)); got != `"Integer"` {
		t.Errorf("type-literal JSON = %q", got)
	}
	if got := FormatValueJSON(NewDepScalar(DepGT, NewInteger(10))); !strings.Contains(got, "gt") {
		t.Errorf("depscalar JSON = %q", got)
	}
	// A None-parent value that is not the none sentinel -> "null" case arm.
	if got := FormatValueJSON(Value{d: &Datum{Parent: TNone, Data: IntPayload{N: 1}}}); got != "null" {
		t.Errorf("None-parent JSON = %q, want null", got)
	}
	// A float takes the default arm.
	if got := FormatValueJSON(NewFloat(1.5)); got == "" {
		t.Error("float JSON default should not be empty")
	}
}

// --- indexcheck.go --------------------------------------------------------
