package core

import "testing"

// at is a positioned token for the reader tests.
func at(v Value, row, col int) Value {
	v.SetPos(SrcPos{Row: row, Col: col})
	return v
}

// anonFn is an anonymous fn value whose one signature's body is body.
func anonFn(body ...Value) FnDefInfo {
	return FnDefInfo{Signatures: []Signature{{Impl: Boru(body)}}}
}

// TestBodyEndSpansNestedTokens pins an anonymous body's span: the last
// position its tokens cover, inside nested lists, groups and map values; a
// nil map adds nothing, and neither does a positionless token.
func TestBodyEndSpansNestedTokens(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", at(NewInteger(1), 4, 2))
	body := []Value{
		at(NewWord("k"), 1, 5),
		NewList([]Value{at(NewWord("x"), 2, 1)}),
		NewParenExpr([]Value{at(NewWord("y"), 3, 7)}),
		NewMap(om),
		{Parent: TMap, Data: MapPayload{}},
		NewWord("synthetic"),
	}
	if end := bodyEnd(body); end.Row != 4 || end.Col != 2 {
		t.Errorf("the map value inside is the last covered, got %+v", end)
	}
	if !posAfter(SrcPos{Row: 2, Col: 1}, SrcPos{Row: 1, Col: 9}) || !posAfter(SrcPos{Row: 1, Col: 3}, SrcPos{Row: 1, Col: 2}) || posAfter(SrcPos{Row: 1, Col: 2}, SrcPos{Row: 1, Col: 2}) {
		t.Error("source order is row first, then column; a position is not after itself")
	}
	if AnonFnReader(SrcPos{Row: 3, Col: 14}) != "fn value at 3:14" {
		t.Errorf("identity name, got %q", AnonFnReader(SrcPos{Row: 3, Col: 14}))
	}
}

// TestAnonReaderIdentity pins NUR257's reader: an anonymous value's body is
// registered at queueing, a named fn's and an unpositioned body are not; the
// named fn that runs the body reaches it, an analysis outside every named fn
// records nothing; and a read inside the body is rescued exactly when a
// binder of the name reaches the value — the innermost body when they nest.
func TestAnonReaderIdentity(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	t.Cleanup(done)
	c := r.Check
	inner := at(NewWord("k"), 1, 20)
	outer := anonFn(at(NewWord("dup"), 1, 10), NewList([]Value{inner}))
	c.NoteAnonFnBody(outer)
	c.NoteAnonFnBody(anonFn(inner))
	c.NoteAnonFnBody(FnDefInfo{Name: "named", Signatures: outer.Signatures})
	c.NoteAnonFnBody(anonFn())
	c.NoteAnonFnBody(anonFn(NewWord("synthetic")))
	if len(c.AnonFnBodies) != 2 {
		t.Fatalf("two anonymous bodies register, got %v", c.AnonFnBodies)
	}
	// g binds k and runs the inner value; the root runs the outer.
	c.FnNameStack = []string{"g"}
	c.RecordFnBinder("k")
	c.NoteAnonDispatch(inner.Pos())
	c.NoteAnonDispatch(SrcPos{Row: 9, Col: 9})
	c.FnNameStack = nil
	c.NoteAnonDispatch(SrcPos{Row: 1, Col: 10})
	if !c.callReaches("g", AnonFnReader(inner.Pos())) || len(c.FnCallGraph["g"]) != 1 {
		t.Errorf("g reaches the inner value alone, got %v", c.FnCallGraph)
	}
	if !c.AnonScopeReachable("k", inner.Pos()) {
		t.Error("the read inside the inner body is reached from g's frame")
	}
	if c.AnonScopeReachable("k", SrcPos{Row: 1, Col: 12}) {
		t.Error("the outer body alone is reached from no binder")
	}
	if c.AnonScopeReachable("k", SrcPos{Row: 5, Col: 1}) || c.AnonScopeReachable("j", inner.Pos()) {
		t.Error("a read outside every anonymous body, or of a name no fn binds, is no reference")
	}
}

// TestBehaveReaders pins the capability half: `behave` notes the fn it
// installs — by name, or by each positioned body's identity — and a named fn
// that handles a value of the target, or the target type itself, reaches it.
// Nothing is noted outside a check pass or for no target, and a dispatch
// outside every named fn, or over another type, records no edge.
func TestBehaveReaders(t *testing.T) {
	r := newTestRegistry(t)
	c := r.Check
	temp := r.Types.MintMemberType("Temp", TInteger, func(Value) bool { return true })
	c.NoteBehaveReader(temp, anonFn(at(NewWord("k"), 2, 3)))
	if len(c.BehaveReaders) != 0 {
		t.Fatal("outside a check pass nothing is noted")
	}
	done := c.Begin()
	t.Cleanup(done)
	c.NoteBehaveReader(nil, anonFn(at(NewWord("k"), 2, 3)))
	c.NoteBehaveReader(temp, anonFn(at(NewWord("k"), 2, 3)))
	c.NoteBehaveReader(temp, FnDefInfo{Name: "canonical"})
	c.NoteBehaveReader(temp, anonFn(NewWord("synthetic")))
	if len(c.BehaveReaders) != 2 {
		t.Fatalf("an anonymous body and a named fn, got %+v", c.BehaveReaders)
	}
	v := NewInteger(5)
	v.Parent = temp
	c.NoteBehaveDispatch([]Value{v})
	if len(c.FnCallGraph) != 0 {
		t.Error("a dispatch outside every named fn records no edge")
	}
	c.FnNameStack = []string{"g"}
	c.NoteBehaveDispatch([]Value{NewString("s"), {}})
	if len(c.FnCallGraph) != 0 {
		t.Error("a dispatch over another type records no edge")
	}
	c.NoteBehaveDispatch([]Value{v})
	if !c.callReaches("g", "canonical") || !c.callReaches("g", AnonFnReader(SrcPos{Row: 2, Col: 3})) {
		t.Errorf("g handles a Temp: it reaches both, got %v", c.FnCallGraph)
	}
	c.FnCallGraph = nil
	c.NoteBehaveDispatch([]Value{*temp})
	if !c.callReaches("g", "canonical") {
		t.Error("the type itself — a constructor's operand — reaches the capability")
	}
	c.FnNameStack, c.BehaveReaders = nil, nil
	c.NoteBehaveDispatch([]Value{v})
}

// TestFnMemberReads pins the member tag behave's check half sees through: a
// get-family read's one result is tagged with the fn value a concrete list
// index or map key resolves; any other read, operand or member tags nothing.
func TestFnMemberReads(t *testing.T) {
	r := newTestRegistry(t)
	c := r.Check
	fnv := NewFunction(anonFn(at(NewWord("k"), 1, 1)))
	om := NewOrderedMap()
	om.Set("c", fnv)
	om.Set("n", NewInteger(1))
	mp, lst := NewMap(om), NewList([]Value{NewInteger(0), fnv})
	out := func() []Value { return []Value{withID(NewCarrier(TAny))} }
	for _, tc := range []struct {
		name string
		word string
		args []Value
		want bool
	}{
		{"a map's fn member by atom", "dot", []Value{NewAtom("c"), mp}, true},
		{"a map's fn member by string", "getr", []Value{NewString("c"), mp}, true},
		{"a list's fn element", "get", []Value{NewInteger(1), lst}, true},
		{"a list index out of range", "get", []Value{NewInteger(5), lst}, false},
		{"a list's data element", "get", []Value{NewInteger(0), lst}, false},
		{"a map's data member", "dot", []Value{NewAtom("n"), mp}, false},
		{"a carrier key", "dot", []Value{NewCarrier(TAtom), mp}, false},
		{"a carrier container", "dot", []Value{NewAtom("c"), NewCarrier(TMap)}, false},
		{"a nil map", "dot", []Value{NewAtom("c"), {Parent: TMap, Data: MapPayload{}}}, false},
		{"an Integer key into a map", "dot", []Value{NewInteger(1), mp}, false},
		{"another word", "first", []Value{NewAtom("c"), mp}, false},
	} {
		c.FnMemberReads = nil
		res := out()
		c.NoteFnMemberRead(tc.word, tc.args, res)
		if _, ok := c.FnMemberRead(res[0].ID); ok != tc.want {
			t.Errorf("%s: tagged=%v, want %v", tc.name, ok, tc.want)
		}
	}
	c.NoteFnMemberRead("dot", []Value{NewAtom("c"), mp}, nil)
	c.NoteFnMemberRead("dot", []Value{NewAtom("c"), mp}, []Value{{Parent: TAny, Carrier: true}})
	if len(c.FnMemberReads) != 0 {
		t.Error("no result, or a result with no identity, is not tagged")
	}
}

// TestRescueAnonymousReader pins the forward-reference rescue's anonymous
// arm: an undefined-word finding inside an anonymous value's body, with no
// named reader, is rescued when a binder of the name reaches the value, and
// kept when none does.
func TestRescueAnonymousReader(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	t.Cleanup(done)
	c := r.Check
	c.NoteAnonFnBody(anonFn(at(NewWord("k"), 1, 20)))
	c.NoteAnonFnBody(anonFn(at(NewWord("k"), 2, 20)))
	c.FnNameStack = []string{"g"}
	c.RecordFnBinder("k")
	c.NoteAnonDispatch(SrcPos{Row: 1, Col: 20})
	c.FnNameStack = nil
	c.Diagnostics = []CheckDiagnostic{
		{Code: "undefined_word", Word: "k", FnBody: true, Row: 1, Col: 20},
		{Code: "undefined_word", Word: "k", FnBody: true, Row: 2, Col: 20},
	}
	r.RescueForwardRefDiagnostics()
	if len(c.Diagnostics) != 1 || c.Diagnostics[0].Row != 2 {
		t.Errorf("the value g runs is rescued, the one nothing runs is not; got %+v", c.Diagnostics)
	}
}
