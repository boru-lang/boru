package native

import (
	"strings"
	"testing"
)

// Codex round-4 fixes (design/TYPED-CONTAINER-TAG-RETENTION.0.md):
//
//	P1a the args stack (args.N) + unnamed body pushes now bind the RETAGGED arg,
//	    so a body write through args.N enforces the {:T}/[:T] param contract
//	    identically to the named binding (and to the compiled path).
//	P1b remove-at, at, replicate, compress retain the source tag; insert-at
//	    enforces the inserted element AND retains the tag.
//	P2  the immutable list/map `set` check residual is a PROPER typed carrier
//	    when the receiver is tagged, so a read from `(xs set i v)` narrows.

// --- P1a: args.N write enforces the param contract ---

func TestParamArgsStackWriteEnforced(t *testing.T) {
	for _, src := range []string{
		`def p fn [[m:{:Integer}] [Map] [(args.0 set b/q "bad")]] (p {a:1})`,
		`def p fn [[xs:[:Integer]] [List] [(args.0 set 0 "bad")]] (p [1 2 3])`,
		// a flex argument reached through args.0 is tagged too
		`def p fn [[m:{:Integer}] [Map] [(args.0 set b/q "bad")]] (p (flex {a:1}))`,
	} {
		_, err := tcRun(t, src)
		if err == nil || !strings.Contains(err.Error(), "does not conform to element type") {
			t.Errorf("[%s] args.N write should enforce the param contract, got %v", src, err)
		}
	}
	// A conforming args.0 write succeeds; an untyped param arg is unenforced.
	if _, err := tcRun(t, `def p fn [[m:{:Integer}] [Map] [(args.0 set b/q 2)]] (p {a:1})`); err != nil {
		t.Errorf("conforming args.0 write should succeed: %v", err)
	}
	if _, err := tcRun(t, `def p fn [[m:Map] [Map] [(args.0 set b/q "ok")]] (p {a:1})`); err != nil {
		t.Errorf("untyped args.0 write should succeed: %v", err)
	}
}

// --- P1b: array copy/gather ops retain the tag; insert-at enforces + retains ---

func TestArrayCopyRetainsTagDirect(t *testing.T) {
	xs, r := tcValue(t, `def xs:[:Integer] [10 20 30] xs`)
	idx, _ := tcValue(t, `def i [0 2] i`)
	counts, _ := tcValue(t, `def c [1 1 1] c`)
	mask, _ := tcValue(t, `def m [true false true] m`)

	tagged := func(name string, out []Value, err error) {
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c, ok := out[0].ElemConstraint(); !ok || !c.Equal(TInteger) {
			t.Errorf("%s dropped the [:Integer] tag (ok=%v)", name, ok)
		}
	}

	out, err := atHandler([]Value{idx, xs}, nil, nil, r)
	tagged("at", out, err)
	out, err = removeAtHandler([]Value{NewInteger(0), xs}, nil, nil, r)
	tagged("remove-at", out, err)
	out, err = replicateHandler([]Value{counts, xs}, nil, nil, r)
	tagged("replicate", out, err)
	out, err = compressHandler([]Value{mask, xs}, nil, nil, r)
	tagged("compress", out, err)
	// insert-at with a conforming element retains the tag.
	out, err = insertAtHandler([]Value{NewInteger(0), NewInteger(99), xs}, nil, nil, r)
	tagged("insert-at", out, err)
}

// insert-at enforces the inserted element against the list's [:T] type.
func TestInsertAtEnforcesElement(t *testing.T) {
	xs, r := tcValue(t, `def xs:[:Integer] [1 2 3] xs`)
	if _, err := insertAtHandler([]Value{NewInteger(0), NewString("bad"), xs}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("insert-at should reject a non-Integer element, got %v", err)
	}
	// A conforming insert into an untyped list is unenforced (no tag).
	ux, ur := tcValue(t, `def u [1 2 3] u`)
	if _, err := insertAtHandler([]Value{NewInteger(0), NewString("ok"), ux}, nil, nil, ur); err != nil {
		t.Errorf("insert into an untyped list should succeed: %v", err)
	}
}

// --- P2: the immutable set check residual is a proper typed carrier ---

func TestSetResidualTypedCarrier(t *testing.T) {
	// childType extracts a carrier's ChildTypeInfo.Child (the read-narrowing
	// bound). NewCarrier(TList/TMap) sets Child=Any, so IsTypedList/IsTypedMap is
	// true even for a bare carrier — the DISTINCTION P2 makes is Child != Any.
	childType := func(v Value) Value {
		ci, err := AsChildType(v)
		if err != nil {
			t.Fatalf("residual has no ChildTypeInfo: %v", err)
		}
		return ci.Child
	}
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Check.Mode = true

	xs, _ := tcValue(t, `def xs:[:Integer] [1 2 3] xs`)
	lout := setListIndexReturns([]Value{NewInteger(0), NewInteger(9), xs}, r)
	// P2: the child narrows to the element type (read narrowing) ...
	if c := childType(lout[0]); !c.Equal(TInteger) {
		t.Errorf("typed-list set residual child = %v, want Integer", ParentOf(&c))
	}
	// ... AND the elem pointer survives (chained-write enforcement, round-3 #3).
	if e, ok := lout[0].ElemConstraint(); !ok || !e.Equal(TInteger) {
		t.Errorf("typed-list set residual lost its elem pointer (chained writes would escape)")
	}

	m, _ := tcValue(t, `def m:{:Integer} {a:1} m`)
	mout := setMapTypedReturns([]Value{NewString("b"), NewInteger(2), m}, r)
	if c := childType(mout[0]); !c.Equal(TInteger) {
		t.Errorf("typed-map set residual child = %v, want Integer", ParentOf(&c))
	}
	if e, ok := mout[0].ElemConstraint(); !ok || !e.Equal(TInteger) {
		t.Errorf("typed-map set residual lost its elem pointer")
	}

	// The negative: an untyped receiver keeps a bare Any-child carrier.
	u, _ := tcValue(t, `def u [1 2 3] u`)
	uout := setListIndexReturns([]Value{NewInteger(0), NewInteger(9), u}, r)
	if c := childType(uout[0]); !ParentOf(&c).Equal(TAny) {
		t.Errorf("untyped-list set residual child = %v, want Any", ParentOf(&c))
	}
	if _, ok := uout[0].ElemConstraint(); ok {
		t.Errorf("untyped-list set residual should carry no elem tag")
	}
}
