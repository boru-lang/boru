package core

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

// mkInst builds a synthetic flat instance for deq-key tests. The
// Parent only feeds Parent.Equal / .ID, so any non-Node, non-Scalar
// type works as a stand-in class.
func mkInst(t *Type, kv ...any) Value {
	om := NewOrderedMap()
	for i := 0; i+1 < len(kv); i += 2 {
		om.Set(kv[i].(string), kv[i+1].(Value))
	}
	return NewClassInstance(t, ClassInstanceInfo{Fields: om})
}

// deqKeyBattery is the shared value battery: every DeqKey branch and
// every deq-equal shape the contract must survive. Order matters for
// TestDeqIndexMatchesLinearScan — unkeyed values sit both before and
// after their keyed deq-mates so the first-match merge walks both ways.
func deqKeyBattery() []Value {
	deep := NewInteger(1)
	for i := 0; i < deqKeyMaxDepth+8; i++ {
		deep = NewList([]Value{deep})
	}
	typed := func() Value { return NewTypedList(NewCarrier(TInteger)) }
	m := func(kv ...any) Value {
		om := NewOrderedMap()
		for i := 0; i+1 < len(kv); i += 2 {
			om.Set(kv[i].(string), kv[i+1].(Value))
		}
		return NewMap(om)
	}
	bigInt, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	return []Value{
		// numbers — int64, cross-leaf, signed zero, NaN, Inf, 2^53 collision
		NewInteger(0), NewInteger(1), NewFloat(1.0), NewFloat(0.0),
		NewFloat(math.Copysign(0, -1)), NewFloat(math.NaN()), NewFloat(math.NaN()),
		NewFloat(math.Inf(1)),
		NewInteger(1 << 53), NewInteger(1<<53 + 1), NewFloat(float64(uint64(1) << 53)),
		NewBigInteger(big.NewInt(1)), NewBigInteger(bigInt),
		NewValueRaw(TBigInteger, IntPayload{N: 1}), // accessor error → never equal
		// numeric type literals — NUR032: NOT deq-equal to 0 or to each
		// other; bucket by type identity
		NewTypeLiteral(TInteger), NewTypeLiteral(TFloat),
		// other scalar families + their literals
		NewString(""), NewString("a"), NewString("a,s:b"), NewBoolean(true),
		NewBoolean(false), NewAtom("a"), NewAtom("Integer"),
		NewTypeLiteral(TString), NewTypeLiteral(TNone),
		// DepScalar — equal iff parent+constraint match
		NewDepScalar(DepGT, NewInteger(10)), NewDepScalar(DepGT, NewInteger(10)),
		NewDepScalar(DepLT, NewInteger(10)),
		// semantic scalars (Micron family)
		NewPathon([]string{"a", "b"}, false), NewPathon([]string{"a", "b"}, false),
		NewPathon([]string{"a", "b"}, true),
		// lists — plain, cross-leaf elements, empty, nested, flex
		NewList([]Value{NewInteger(1)}), NewList([]Value{NewFloat(1.0)}),
		NewList([]Value{NewInteger(1), NewInteger(2)}), NewList(nil),
		NewList([]Value{NewString("a")}),
		NewFlexList([]Value{NewInteger(1)}),
		NewList([]Value{NewList([]Value{NewInteger(1)})}),
		// unkeyed list shapes — typed lists sandwiching a plain twin
		typed(), NewList([]Value{NewInteger(9)}), typed(), NewCarrier(TList),
		NewList([]Value{typed()}), // unkeyed child propagates
		deep,                      // recursion cap → unkeyed
		// maps — insertion order must not matter
		m("a", NewInteger(1), "b", NewInteger(2)), m("b", NewInteger(2), "a", NewInteger(1)),
		m(), m("a", NewInteger(1)), NewCarrier(TMap),
		m("k", typed()), // unkeyed child propagates
		// xml
		NewXmlElement("t", nil, nil), NewXmlElement("t", nil, nil),
		NewXmlElement("u", nil, nil),
		// flat instances — equal pair, differing field, unkeyed field
		// (before AND after its keyed twin), never-equal field, nil fields
		mkInst(TStore, "x", NewInteger(1)), mkInst(TStore, "x", NewInteger(1)),
		mkInst(TStore, "x", NewInteger(2)),
		mkInst(TStore, "k", typed()), mkInst(TStore, "x", NewInteger(3)),
		mkInst(TStore, "k", typed()),
		mkInst(TStore, "s", NewStoreValue(TStore, &StoreInstanceInfo{})),
		NewClassInstance(TStore, ClassInstanceInfo{}),
		// NUR031 deq-comparable handles (DeqUnkeyed): two empty stores are
		// deq-equal, an error, a timer.
		NewStoreValue(TStore, &StoreInstanceInfo{}),
		NewStoreValue(TStore, &StoreInstanceInfo{}),
		errVal("c", "m", nil), NewValueRaw(TIdeal, &TimeoutInfo{ID: "t"}),
		// the code/opaque values — still deq-equal to nothing
		NewWord("w"),
	}
}

// TestDeqKeyContract pins the one-directional contract: any deq-equal
// pair either shares a DeqKeyed key or has an unkeyed side, and a
// DeqNeverEqual value is deq-equal to nothing at all.
func TestDeqKeyContract(t *testing.T) {
	battery := deqKeyBattery()
	for i, a := range battery {
		for j, b := range battery {
			if !DeepEqual(a, b) {
				continue
			}
			ka, ca := DeqKey(a)
			kb, cb := DeqKey(b)
			if ca == DeqNeverEqual || cb == DeqNeverEqual {
				t.Fatalf("battery[%d] deq battery[%d] but a side is DeqNeverEqual (%v %v)", i, j, a, b)
			}
			if ca == DeqUnkeyed || cb == DeqUnkeyed {
				continue
			}
			if ka != kb {
				t.Fatalf("battery[%d] deq battery[%d] but keys differ: %q vs %q (%v %v)", i, j, ka, kb, a, b)
			}
		}
	}
}

// TestDeqIndexMatchesLinearScan pins DeqIndex against the semantics it
// replaces: FirstMatch must return exactly what a linear DeepEqual
// scan over the previously-added values returns.
func TestDeqIndexMatchesLinearScan(t *testing.T) {
	battery := deqKeyBattery()
	var idx DeqIndex
	for i, v := range battery {
		want := -1
		for j := 0; j < i; j++ {
			if DeepEqual(battery[j], v) {
				want = j
				break
			}
		}
		if got := idx.FirstMatch(v); got != want {
			t.Fatalf("FirstMatch(battery[%d]) = %d, linear scan says %d (%v)", i, got, want, v)
		}
		if got := idx.Add(v); got != i {
			t.Fatalf("Add(battery[%d]) returned position %d", i, got)
		}
	}
}

// TestDeqKeyBranches pins each branch's key shape directly.
func TestDeqKeyBranches(t *testing.T) {
	key := func(v Value) string {
		k, c := DeqKey(v)
		if c != DeqKeyed {
			t.Fatalf("expected DeqKeyed for %v, got class %d", v, c)
		}
		return k
	}
	class := func(v Value) DeqKeyClass {
		_, c := DeqKey(v)
		return c
	}

	if key(NewTypeLiteral(TNone)) != "N" {
		t.Fatalf("None key: %q", key(NewTypeLiteral(TNone)))
	}
	if k := key(NewDepScalar(DepGT, NewInteger(1))); !strings.HasPrefix(k, "D:") {
		t.Fatalf("DepScalar key: %q", k)
	}
	if key(NewInteger(1)) != "n:1" || key(NewFloat(1.0)) != "n:1" {
		t.Fatal("cross-leaf 1 and 1.0 must share n:1")
	}
	if key(NewFloat(0.0)) != key(NewFloat(math.Copysign(0, -1))) {
		t.Fatal("-0.0 must fold into the +0.0 bucket")
	}
	if key(NewFloat(math.NaN())) != "n:NaN" {
		t.Fatalf("NaN key: %q", key(NewFloat(math.NaN())))
	}
	if key(NewBigInteger(big.NewInt(7))) != "n:7" {
		t.Fatalf("big 7 key: %q", key(NewBigInteger(big.NewInt(7))))
	}
	if class(NewValueRaw(TBigInteger, IntPayload{N: 1})) != DeqNeverEqual {
		t.Fatal("accessor-error big must be DeqNeverEqual")
	}
	// NUR032: a bare numeric type literal buckets by type identity, NOT
	// as the value 0 — it is no longer deq-equal to 0, and Integer and
	// Float literals land in distinct buckets.
	intLit, fltLit := key(NewTypeLiteral(TInteger)), key(NewTypeLiteral(TFloat))
	if !strings.HasPrefix(intLit, "tlit:") || intLit == fltLit {
		t.Fatalf("numeric literals must bucket by type identity: %q vs %q", intLit, fltLit)
	}
	if intLit == key(NewInteger(0)) {
		t.Fatalf("Integer literal must not share 0's bucket: %q", intLit)
	}
	if key(NewString("a")) != "s:a" || key(NewBoolean(true)) != "b:true" || key(NewAtom("x")) != "a:x" {
		t.Fatal("scalar family key shapes")
	}
	// A bare String type literal buckets as a type literal (NUR032),
	// not as a concrete scalar; distinct from Integer's literal bucket.
	if k := key(NewTypeLiteral(TString)); !strings.HasPrefix(k, "tlit:") || k == key(NewTypeLiteral(TInteger)) {
		t.Fatalf("String literal key: %q", k)
	}
	p1 := NewPathon([]string{"a"}, false)
	p2 := NewPathon([]string{"b"}, false)
	if k := key(p1); !strings.HasPrefix(k, "c:") || k != key(p2) {
		t.Fatalf("Pathons must share their family bucket: %q vs %q", key(p1), key(p2))
	}
	if k := key(NewList([]Value{NewInteger(1), NewInteger(2)})); k != "[n:1,n:2]" {
		t.Fatalf("list key: %q", k)
	}
	if key(NewList(nil)) != "[]" {
		t.Fatalf("empty list key: %q", key(NewList(nil)))
	}
	om1 := NewOrderedMap()
	om1.Set("b", NewInteger(2))
	om1.Set("a", NewInteger(1))
	if k := key(NewMap(om1)); k != `{"a"=n:1,"b"=n:2}` {
		t.Fatalf("map key must sort by key: %q", k)
	}
	if key(NewXmlElement("t", nil, nil)) != "x:t" {
		t.Fatalf("xml key: %q", key(NewXmlElement("t", nil, nil)))
	}
	if k := key(mkInst(TStore, "x", NewInteger(1))); !strings.HasPrefix(k, "i:") {
		t.Fatalf("instance key: %q", k)
	}
	two := mkInst(TStore, "y", NewInteger(2), "x", NewInteger(1))
	if k := key(two); !strings.HasSuffix(k, `{"x"=n:1,"y"=n:2}`) {
		t.Fatalf("two-field instance key must sort fields: %q", k)
	}

	// NUR033: an EMPTY typed list/map value keys as its (empty) content,
	// deq-equal to a plain empty container — no longer DeqUnkeyed. So do
	// containers nesting one.
	typed := NewTypedList(NewCarrier(TInteger))
	if k := key(typed); k != "[]" {
		t.Fatalf("empty typed list must key as empty: %q", k)
	}
	if key(NewList([]Value{typed})) != "[[]]" {
		t.Fatalf("list nesting an empty typed list must key structurally")
	}
	// DeqUnkeyed now means a genuine type-level operand (a carrier, which
	// DeepEqual still compares by render) or a container whose child has
	// no sound key.
	mapWithStore := func() Value {
		om := NewOrderedMap()
		om.Set("s", NewStoreValue(TStore, &StoreInstanceInfo{}))
		return NewMap(om)
	}()
	for _, v := range []Value{
		NewCarrier(TList), NewCarrier(TMap),
		NewList([]Value{NewStoreValue(TStore, &StoreInstanceInfo{})}),
		mapWithStore, // a MAP whose value has no sound key propagates unkeyed
	} {
		if class(v) != DeqUnkeyed {
			t.Fatalf("expected DeqUnkeyed for %v", v)
		}
	}
	// DeqNeverEqual is now only instances with no readable fields — a
	// Store is DeqUnkeyed after NUR031, and the code values (Word,
	// Function) are keyed by canon since NUR031's second half (both
	// covered by TestNUR031DeqKeyClassification).
	for _, v := range []Value{
		NewClassInstance(TStore, ClassInstanceInfo{}),
	} {
		if class(v) != DeqNeverEqual {
			t.Fatalf("expected DeqNeverEqual for %v", v)
		}
	}
	// …and an instance whose field is a Word is keyed THROUGH that
	// field's key, no longer poisoned to never-equal by it.
	if class(mkInst(TStore, "fn", NewWord("f"))) != DeqKeyed {
		t.Fatal("an instance with a word field must be DeqKeyed")
	}
	// The poisoning itself still happens — an instance whose field is a
	// never-equal value propagates that up. The subject moved from a Word
	// to a field-less instance when NUR031 keyed the code values.
	if class(mkInst(TStore, "inner", NewClassInstance(TStore, ClassInstanceInfo{}))) != DeqNeverEqual {
		t.Fatal("a never-equal FIELD must make its instance never-equal")
	}
	// The terminal arm's own residents after NUR031: the internal shapes
	// no user names — a forward-collection marker, a move marker.
	for _, v := range []Value{
		NewForward(ForwardInfo{FuncName: "f"}),
		NewMove("x", "y"),
	} {
		if class(v) != DeqNeverEqual {
			t.Fatalf("an internal marker (%T) must reach the terminal arm", DataOf(&v))
		}
	}
	// An instance whose field is a (now deq-comparable) store is
	// DeqUnkeyed, propagated up from the field.
	if class(mkInst(TStore, "s", NewStoreValue(TStore, &StoreInstanceInfo{}))) != DeqUnkeyed {
		t.Fatal("instance with a store field must be DeqUnkeyed")
	}
}

// TestDeqKeyTerminatesOnCycles is the regression guard for the depth
// cap: a self-referential flex container must classify (as unkeyed)
// instead of recursing forever. The pairwise scans DeqKey feeds never
// fire for a lone value, matching the old nested-scan behaviour.
func TestDeqKeyTerminatesOnCycles(t *testing.T) {
	data := &FlexListData{}
	v := NewValueRaw(TFlexList, data)
	data.Elems = append(data.Elems, v)
	if _, c := DeqKey(v); c != DeqUnkeyed {
		t.Fatalf("cyclic flex list: expected DeqUnkeyed, got %d", c)
	}
}

// TestDeqIndexNeverEqualSkips pins the DeqNeverEqual fast path on both
// sides: a field-less instance neither matches anything nor is matched.
// (The subject used to be a Word — NUR031's second half made words
// value-like and keyed, leaving the field-less instance as the class's
// remaining resident.)
func TestDeqIndexNeverEqualSkips(t *testing.T) {
	var idx DeqIndex
	v := NewClassInstance(TStore, ClassInstanceInfo{})
	idx.Add(v)
	if got := idx.FirstMatch(v); got != -1 {
		t.Fatalf("field-less instance FirstMatch = %d, want -1", got)
	}
	// The positive twin: a Word now DOES match itself through the index.
	var widx DeqIndex
	w := NewWord("f")
	widx.Add(w)
	if got := widx.FirstMatch(w); got != 0 {
		t.Fatalf("word FirstMatch = %d, want 0", got)
	}
	if got := widx.FirstMatch(NewWord("g")); got != -1 {
		t.Fatalf("a different word must not match: got %d", got)
	}
}
