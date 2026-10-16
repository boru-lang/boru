package core

// Stage-5 coverage: print.go. Drives the print/printstr handlers against
// a captured Registry.Output and the remaining FormatForPrint /
// FormatValueJSON / formatMapJSON / formatListJSON arms with crafted
// values.

import (
	"errors"
	"testing"
)

// TestStage5FormatForPrintArms pins the scalar and container arms of
// FormatForPrint: none, type literals, errors, strings (no quotes),
// JSON-ish maps and lists, and the ValToString fallback.
func TestStage5FormatForPrintArms(t *testing.T) {
	if got := FormatForPrint(NewNone()); got != "none" {
		t.Errorf("none prints %q, want %q", got, "none")
	}
	if got := FormatForPrint(NewTypeLiteral(TInteger)); got != "Integer" {
		t.Errorf("type literal prints %q, want %q", got, "Integer")
	}
	if got := FormatForPrint(NewError(errors.New("boom"))); got != "boom" {
		t.Errorf("error prints %q, want %q", got, "boom")
	}
	if got := FormatForPrint(NewString("plain text")); got != "plain text" {
		t.Errorf("string prints %q, want unquoted %q", got, "plain text")
	}

	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	om.Set("b", NewString("x"))
	if got := FormatForPrint(NewMap(om)); got != `{"a": 1, "b": "x"}` {
		t.Errorf("map prints %q", got)
	}

	lst := NewList([]Value{NewInteger(1), NewString("x"), NewBoolean(true)})
	if got := FormatForPrint(lst); got != `[1, "x", true]` {
		t.Errorf("list prints %q", got)
	}

	// Fallback arm: integers go through ValToString.
	if got := FormatForPrint(NewInteger(7)); got != "7" {
		t.Errorf("integer prints %q, want %q", got, "7")
	}
}

// TestStage5FormatValueJSONArms pins the per-value JSON renderer arms that
// the seam-7 tests left unexercised: quoted strings, integers, boolean
// true, and the recursive map / list arms.
func TestStage5FormatValueJSONArms(t *testing.T) {
	if got := FormatValueJSON(NewString("x")); got != `"x"` {
		t.Errorf("string JSON = %q, want %q", got, `"x"`)
	}
	if got := FormatValueJSON(NewInteger(3)); got != "3" {
		t.Errorf("integer JSON = %q, want %q", got, "3")
	}
	if got := FormatValueJSON(NewBoolean(true)); got != "true" {
		t.Errorf("true JSON = %q, want %q", got, "true")
	}
	if got := FormatValueJSON(NewBoolean(false)); got != "false" {
		t.Errorf("false JSON = %q, want %q", got, "false")
	}

	inner := NewOrderedMap()
	inner.Set("k", NewNone())
	if got := FormatValueJSON(NewMap(inner)); got != `{"k": null}` {
		t.Errorf("nested map JSON = %q", got)
	}
	if got := FormatValueJSON(NewList([]Value{NewInteger(1), NewInteger(2)})); got != "[1, 2]" {
		t.Errorf("nested list JSON = %q", got)
	}
}
