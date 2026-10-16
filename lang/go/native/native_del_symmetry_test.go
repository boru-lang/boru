package native

import (
	"sort"
	"strings"
	"testing"
)

// NUR022: `del` covered a fraction of `set`'s containers — Map and
// FlexMap out of eleven — so most receivers failed dispatch with an
// opaque no_signature rather than either working or saying why not.
//
// The gate below is the record's actual resolution: it does not merely
// check the containers that exist today, it asserts the two words carry
// the SAME container set, so the next container added to `set` cannot
// silently reopen the gap.

// delSetContainers extracts the receiver type name of every signature of
// a word, deduplicated. The receiver is the LAST arg for both words:
// `set` is [key value container], `del` is [key container].
func delSetContainers(t *testing.T, name string) []string {
	t.Helper()
	r := w9Reg(t)
	fn := r.Lookup(name)
	if fn == nil {
		t.Fatalf("%s is not registered", name)
	}
	seen := map[string]bool{}
	for _, sig := range fn.Signatures {
		if len(sig.Args) == 0 {
			t.Fatalf("%s: a zero-arg signature has no receiver", name)
		}
		seen[sig.Args[len(sig.Args)-1].String()] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestDelCoversEverySetContainer is the NUR022 gate. It fails in BOTH
// directions: a container `set` gains and `del` does not is the original
// defect returning, and a container `del` carries that `set` does not is
// equally wrong — `del` removes a slot only some other word could have
// written, which is a gap in the opposite direction.
func TestDelCoversEverySetContainer(t *testing.T) {
	setC := delSetContainers(t, "set")
	delC := delSetContainers(t, "del")

	inDel := map[string]bool{}
	for _, c := range delC {
		inDel[c] = true
	}
	for _, c := range setC {
		if !inDel[c] {
			t.Errorf("`set` dispatches over %s but `del` does not (NUR022).\n"+
				"Add a del signature for it: either a real removal, or a "+
				"registered refusal that names why — an absent signature "+
				"raises an opaque dispatch failure instead.", c)
		}
	}

	inSet := map[string]bool{}
	for _, c := range setC {
		inSet[c] = true
	}
	for _, c := range delC {
		if !inSet[c] {
			t.Errorf("`del` dispatches over %s but `set` does not — the column "+
				"rule is symmetric, so a removable slot must be a writable one.", c)
		}
	}
	t.Logf("set and del both dispatch over %d containers: %s",
		len(setC), strings.Join(setC, " "))
}

// TestDelKeyShapesMatchSet pins the finer half of the symmetry: for each
// container, the KEY types del accepts must match set's. Covering the
// same container with only the String form (and not the Atom form) would
// pass the container gate while `m del a/q` still failed.
func TestDelKeyShapesMatchSet(t *testing.T) {
	r := w9Reg(t)
	shapes := func(name string) map[string]bool {
		fn := r.Lookup(name)
		if fn == nil {
			t.Fatalf("%s is not registered", name)
		}
		out := map[string]bool{}
		for _, sig := range fn.Signatures {
			// key type + receiver type, ignoring set's middle value slot.
			out[sig.Args[0].String()+" -> "+sig.Args[len(sig.Args)-1].String()] = true
		}
		return out
	}
	setS, delS := shapes("set"), shapes("del")
	for k := range setS {
		if !delS[k] {
			t.Errorf("set accepts %q but del does not (NUR022)", k)
		}
	}
	for k := range delS {
		if !setS[k] {
			t.Errorf("del accepts %q but set does not (NUR022)", k)
		}
	}
}

// TestDelRefusalsAreSpecific pins that every container del REFUSES says
// why in its own words. A refusal that fell back to a generic dispatch
// error would satisfy the gate above while leaving the user exactly
// where the record found them.
func TestDelRefusalsAreSpecific(t *testing.T) {
	r := w9Reg(t)

	// Class: sealed. Named class, then the nameless fallback.
	ct := &ClassTypeInfo{Name: "Point", Fields: NewOrderedMap()}
	inst := NewClassInstance(TClass, ClassInstanceInfo{TypeRef: ct, Fields: NewOrderedMap()})
	_, err := delClassInstanceHandler([]Value{NewString("x"), inst}, nil, nil, r)
	w9wantErr(t, err, "sealed")
	if got := delClassDetail(nil); !strings.Contains(got, "Class fields are sealed") {
		t.Errorf("delClassDetail fallback = %q, want the bare Class name", got)
	}

	// Micron: immutable, mirroring set's message.
	_, err = delMicronHandler([]Value{NewString("kind"), NewTypeLiteral(TMicron)}, nil, nil, r)
	w9wantErr(t, err, "immutable")
	if got := delMicronDetail(nil); !strings.Contains(got, "Micron values are immutable") {
		t.Errorf("delMicronDetail fallback = %q, want the bare Micron name", got)
	}

	// Lists: the refusal must name the words that DO remove an element,
	// or it is just a wall.
	_, err = delListHandler([]Value{NewInteger(0), w9List(NewInteger(1))}, nil, nil, r)
	w9wantErr(t, err, "cannot remove an element")
	for _, want := range []string{"pop", "shift", "ArrayUtil.remove-at"} {
		if !strings.Contains(delListHint, want) {
			t.Errorf("the list refusal hint does not name %q: %q", want, delListHint)
		}
	}
	if got := delListDetail(nil); !strings.Contains(got, "a List by index") {
		t.Errorf("delListDetail fallback = %q, want the bare List name", got)
	}
}

// TestDelRemovalRejects drives the removal handlers' wrong-receiver arms,
// matching the shape of TestW9SetContainerRejects for the set side.
func TestDelRemovalRejects(t *testing.T) {
	r := w9Reg(t)

	_, err := delStoreHandler([]Value{NewString("k"), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "expected a Store")

	_, err = delWeakFlexMapHandler([]Value{NewString("k"), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "expected a WeakFlexMap")

	_, err = delFlexXmlHandler([]Value{NewString("a"), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "expected a FlexXml")

	_, err = delWeakFlexXmlHandler([]Value{NewString("a"), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "expected a WeakFlexXml")

	// A FlexXml element with no attribute map at all: deleting must be a
	// no-op, not a nil-map write.
	fx := w9Top(t, "flex <a/>")
	out, xerr := delFlexXmlHandler([]Value{NewString("gone"), fx}, nil, nil, r)
	if xerr != nil {
		t.Fatalf("del on an attribute-less FlexXml: %v", xerr)
	}
	if len(out) != 1 {
		t.Fatalf("del must return the receiver for chaining, got %v", out)
	}
}

// TestDelCheckModeMirrors covers the ReturnsFn twins: the guaranteed-error
// mirrors must fire for the refusing containers, and the removal twins
// must widen (never narrow) what they forget.
func TestDelCheckModeMirrors(t *testing.T) {
	r := w9Reg(t)

	// Arity guards: a short arg list must not panic and must not mirror.
	if got := delClassInstanceReturns(nil, r); len(got) != 0 {
		t.Errorf("delClassInstanceReturns(nil) = %v, want empty", got)
	}
	if got := delMicronReturns(nil, r); len(got) != 0 {
		t.Errorf("delMicronReturns(nil) = %v, want empty", got)
	}
	if got := delListReturns(nil, r); len(got) != 0 {
		t.Errorf("delListReturns(nil) = %v, want empty", got)
	}
	if got := delStoreReturnsFn(nil, r); got != nil {
		t.Errorf("delStoreReturnsFn(nil) = %v, want nil", got)
	}
	if got := delStoreReturnsFn([]Value{NewString("k")}, nil); got != nil {
		t.Errorf("delStoreReturnsFn(nil registry) = %v, want nil", got)
	}

	// The WeakFlexMap twin returns a fresh carrier on a short list and the
	// receiver otherwise (the in-place contract).
	short := delWeakFlexMapReturns(nil, r)
	if len(short) != 1 || !IsCarrier(&short[0]) {
		t.Errorf("delWeakFlexMapReturns(nil) = %v, want one carrier", short)
	}
	wm := w9Top(t, "make WeakFlexMap {a:1}")
	pass := delWeakFlexMapReturns([]Value{NewString("a"), wm}, r)
	if len(pass) != 1 {
		t.Fatalf("delWeakFlexMapReturns = %v, want the receiver", pass)
	}

	// The Store twin widens to dynamic Any — the inverse of set's record.
	// Widening, not deleting, because the shape model is join-only
	// monotone: a narrower residual would claim the key still reads.
	sv := w9Top(t, "context")
	r.Check.Mode = true
	defer func() { r.Check.Mode = false }()
	if got := delStoreReturnsFn([]Value{NewString("k"), sv}, r); got != nil {
		t.Errorf("delStoreReturnsFn returns no stack value, got %v", got)
	}
	if rec, ok := r.Check.LookupContextType("k"); ok && !ParentOf(&rec).Equal(TAny) {
		t.Errorf("after del, key k narrows to %s; it must widen to Any", ParentOf(&rec))
	}
}
