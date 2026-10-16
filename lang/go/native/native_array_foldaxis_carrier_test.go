package native

import "testing"

// rank2ElemCarrier's arms, driven directly: the carrier a foldaxis body sees
// is joined over EVERY row's elements — uniform rows give the plain strict
// type, mixed rows a Disjunct the body dispatch distributes over (the
// first-row predecessor baked an Integer overload over `[[1 2] ["a" "b"]]`,
// a measured compiled divergence) — and every shape the check pass cannot
// open (a non-concrete argument, a non-list row, no element at all) is the
// gradual Any carrier.
func TestRank2ElemCarrierArms(t *testing.T) {
	row := func(vs ...Value) Value { return NewList(vs) }
	uniform := rank2ElemCarrier(row(row(NewInteger(1), NewInteger(2)), row(NewInteger(3), NewInteger(4))))
	if IsDynamic(&uniform) || IsDisjunct(&uniform) || !ParentOf(&uniform).Equal(TInteger) {
		t.Fatalf("uniform rows must give the plain strict element type, got %v", uniform)
	}
	mixed := rank2ElemCarrier(row(row(NewInteger(1), NewInteger(2)), row(NewString("a"), NewString("b"))))
	if !IsDisjunct(&mixed) {
		t.Fatalf("mixed rows must join to a Disjunct (the second row's type must reach the body), got %v", mixed)
	}
	for name, data := range map[string]Value{
		"non-concrete": NewCarrier(TList),
		"empty":        row(),
		"no element":   row(row()),
		"non-list row": row(row(NewInteger(1)), NewInteger(2)),
	} {
		if c := rank2ElemCarrier(data); !IsDynamic(&c) {
			t.Fatalf("%s: a data argument the check pass cannot open must be the gradual Any carrier, got %v", name, c)
		}
	}
}

// The RESULT carrier follows the same join: over mixed rows foldaxisReturnsFn
// answers a list whose element is the Disjunct, never the first row's plain
// type — a consumer of the result is then analysed against every alternative
// (the first-row predecessor typed `(foldaxis … [[1 2] ["a" "b"]]) each [1 add]`
// as a list of Integer and the compiled consumer answered `[1 1]` for the
// interpreter's `['a1' 'b1']`). Uniform rows keep the plain type: the
// positive control.
func TestFoldaxisReturnsFnJoinsRowsIntoTheResult(t *testing.T) {
	r := w9Reg(t)
	row := func(vs ...Value) Value { return NewList(vs) }
	uniform := foldaxisReturnsFn([]Value{NewInteger(1), w9AddBody(), row(row(NewInteger(1), NewInteger(2)), row(NewInteger(3), NewInteger(4)))}, r)
	if len(uniform) != 1 || !DataListElemTypeFromValue(uniform[0]).Equal(TInteger) {
		t.Fatalf("uniform rows must answer a list of the plain element type, got %v", uniform)
	}
	mixed := foldaxisReturnsFn([]Value{NewInteger(1), w9AddBody(), row(row(NewInteger(1), NewInteger(2)), row(NewString("a"), NewString("b")))}, r)
	if len(mixed) != 1 || DataListElemTypeFromValue(mixed[0]).Equal(TInteger) {
		t.Fatalf("mixed rows must not answer a list of the first row's type, got %v", mixed)
	}

	// The empty-lane mirror over POSITION-LESS operands (a module wrapper's,
	// or these Go-built literals): the diagnostic still lands, at the call
	// site the checker exposes rather than at a zero position.
	base := len(r.Check.Diagnostics)
	r.Check.CurCallPos = SrcPos{Row: 3, Col: 9}
	out := foldaxisReturnsFn([]Value{NewInteger(1), w9AddBody(), row(NewList([]Value{}))}, r) // an EMPTY row, not a nil one: StaticListLen reads only a concrete non-nil payload
	if len(out) != 1 || !ParentOf(&out[0]).ConformsTo(TList) {
		t.Fatalf("the empty-lane arm must still answer the bare List carrier, got %v", out)
	}
	if n := len(r.Check.Diagnostics); n != base+1 || r.Check.Diagnostics[base].Code != "foldaxis_error" ||
		r.Check.Diagnostics[base].Row != 3 || r.Check.Diagnostics[base].Col != 9 {
		t.Fatalf("expected one foldaxis_error mirror at the exposed call site, got %+v", r.Check.Diagnostics[base:])
	}
	// And the mirror's own negatives: a data argument the check pass cannot
	// open (a carrier), or one with no row, is not exactly known — no error
	// text, nothing flagged.
	if d := staticEmptyLaneDetail([]Value{NewInteger(1), w9AddBody(), NewCarrier(TList)}); d != "" {
		t.Fatalf("a non-concrete data argument must not mirror, got %q", d)
	}
	if d := staticEmptyLaneDetail([]Value{NewInteger(1), w9AddBody(), NewList([]Value{})}); d != "" {
		t.Fatalf("an empty rank-2 list has no lane to raise on, got %q", d)
	}
}
