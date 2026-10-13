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
	e := engWithTape(t, nil, 0)
	e.SetSource("def f fn [[n:Integer] String [n]]\n42 f")
	rc := ReturnCheckInfo{
		FuncName: "f",
		Returns:  []*Type{TString},
		Pos:      SrcPos{Row: 2, Col: 4, Src: "f"},
		Decl:     DeclSite{Pos: SrcPos{Row: 1, Col: 23, Src: "String"}},
	}
	got := NewInteger(42)
	got.SetPos(SrcPos{Row: 2, Col: 1, Src: "42"})
	ae := e.returnTypeError(rc, 1, TString, got)
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
	e := engWithTape(t, nil, 0)
	// No declaration site: no decl span. The value carries a position, so
	// the produced-value span DOES attach — the attachment is decided by
	// the value's own position alone (engine-independent), which is what
	// keeps the compiled and interpreted span sets identical even though
	// the primary caret sits at different places in the two engines
	// (design/DIAGNOSTICS.0.md phase 7).
	rc := ReturnCheckInfo{FuncName: "f", Pos: SrcPos{Row: 2, Col: 4}}
	got := NewInteger(42)
	got.SetPos(SrcPos{Row: 2, Col: 4})
	ae := e.returnTypeError(rc, 1, TString, got)
	if len(ae.Spans) != 1 || ae.Spans[0].Label != "the returned value was produced here" {
		t.Errorf("want exactly the value span, got %+v", ae.Spans)
	}
	// A positionless value attaches no span at all (no location to point at).
	ae = e.returnTypeError(rc, 1, TString, NewInteger(42))
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
