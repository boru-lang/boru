package core

import (
	"strings"
	"testing"
)

// Phase-5 return errors carry two secondary spans — where the offending
// value was produced and where the contract was declared — paired with
// the negatives: no declaration site → no span (never guess), and a
// value produced at the error position is not re-labelled.

func TestReturnTypeErrorSpans(t *testing.T) {
	src := "def f fn [[n:Integer] String [n]]\n42 f"
	pos := SrcPos{Row: 2, Col: 4, Src: "f"}
	decl := DeclSite{Pos: SrcPos{Row: 1, Col: 23, Src: "String"}}
	got := NewInteger(42)
	got.SetPos(SrcPos{Row: 2, Col: 1, Src: "42"})
	ae := BuildReturnTypeError(src, "f", 1, TString, got, pos, decl)
	if ae.Src != "f" {
		t.Errorf("the primary site is the call position's token, got %q", ae.Src)
	}
	if len(ae.Spans) != 2 {
		t.Fatalf("want 2 spans, got %+v", ae.Spans)
	}
	if ae.Spans[0].Label != "the returned value was produced here" || ae.Spans[0].Pos.Row != 2 {
		t.Errorf("value span = %+v", ae.Spans[0])
	}
	if !strings.Contains(ae.Spans[1].Label, "the declaration says `f` returns String") ||
		ae.Spans[1].Pos.Row != 1 {
		t.Errorf("decl span = %+v", ae.Spans[1])
	}
	rendered := ae.Error()
	if !strings.Contains(rendered, "------ the declaration says `f` returns String") {
		t.Errorf("declaration span must render with its excerpt:\n%s", rendered)
	}
}

func TestReturnTypeErrorSpanNegatives(t *testing.T) {
	// No declaration site: no decl span. The value carries a position, so
	// the produced-value span DOES attach — the attachment is decided by
	// the value's own position alone.
	pos := SrcPos{Row: 2, Col: 4}
	got := NewInteger(42)
	got.SetPos(SrcPos{Row: 2, Col: 4})
	ae := BuildReturnTypeError("", "f", 1, TString, got, pos, DeclSite{})
	if len(ae.Spans) != 1 || ae.Spans[0].Label != "the returned value was produced here" {
		t.Errorf("want exactly the value span, got %+v", ae.Spans)
	}
	// A position that names no token leaves the word as the primary site.
	if ae.Src != "f" {
		t.Errorf("a tokenless position keeps the word, got %q", ae.Src)
	}
	// A positionless value attaches no span at all (no location to point at).
	ae = BuildReturnTypeError("", "f", 1, TString, NewInteger(42), pos, DeclSite{})
	if len(ae.Spans) != 0 {
		t.Errorf("positionless value must attach no span, got %+v", ae.Spans)
	}
}

func TestReturnCountErrorDeclSpan(t *testing.T) {
	e := engWithTape(t, nil, 0)
	rc := ReturnCheckInfo{
		FuncName: "f",
		Pos:      SrcPos{Row: 2, Col: 4},
		Decl:     DeclSite{Pos: SrcPos{Row: 1, Col: 23}, Source: "src", File: "mod.boru"},
	}
	vals := []Value{NewInteger(7)}
	ae := e.returnCountError(rc, 2, 1, vals)
	if len(ae.Spans) != 1 {
		t.Fatalf("want the decl span, got %+v", ae.Spans)
	}
	sp := ae.Spans[0]
	if !strings.Contains(sp.Label, "expects 2 return value(s)") ||
		sp.Source != "src" || sp.File != "mod.boru" {
		t.Errorf("decl span = %+v", sp)
	}
	// The detail names the values it is about.
	if !strings.Contains(ae.Detail, "got 1 — [7]") {
		t.Errorf("detail must show the values, got %q", ae.Detail)
	}
	// Zero declaration → no span.
	rc.Decl = DeclSite{}
	if ae := e.returnCountError(rc, 2, 1, vals); len(ae.Spans) != 0 {
		t.Errorf("zero decl must attach nothing, got %+v", ae.Spans)
	}
}
