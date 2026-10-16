package core

import (
	"errors"
	"strings"
	"testing"
)

// Wave 4b coverage: the IdealConverter capability walk and the concrete
// converters (convert_ideal.go), plus the Scalar/Micron family —
// construction, rendering, properties, comparison, check-mode mirror
// (micron.go) — and the Micron branches of InstallType (core_type.go).

// --- convert_ideal.go ----------------------------------------------------------

// The Ideal-conversion halves of the former convert_micron_wave4b
// suite — they pin eng convert_ideal machinery and stayed behind when
// the Micron content moved to basic.

func TestConvertIdealFallbacks(t *testing.T) {
	// A value whose parent chain carries no IdealConverter at all
	// (Integer) falls to the empty-map / empty-list terminal.
	m, err := ConvertIdealToMap(NewInteger(1))
	if err != nil {
		t.Fatalf("ConvertIdealToMap: %v", err)
	}
	mm, _ := AsMap(m)
	if mm.Len() != 0 {
		t.Errorf("integer → map = %v, want {}", m)
	}
	l, err := ConvertIdealToList(NewInteger(1))
	if err != nil {
		t.Fatalf("ConvertIdealToList: %v", err)
	}
	ll, _ := AsList(l)
	if ll.Len() != 0 {
		t.Errorf("integer → list = %v, want []", l)
	}

	// A plain Ideal-rooted value hits the Ideal root's {} / [] fallback.
	r := newTestRegistry(t)
	node := r.Types.MintType("W4bIdealThing", TIdeal)
	v := NewValueRaw(node, ExtensionPayload{Body: "x"})
	m, _ = ConvertIdealToMap(v)
	mm, _ = AsMap(m)
	if mm.Len() != 0 {
		t.Errorf("plain ideal → map = %v, want {}", m)
	}
	l, _ = ConvertIdealToList(v)
	ll, _ = AsList(l)
	if ll.Len() != 0 {
		t.Errorf("plain ideal → list = %v, want []", l)
	}
	// The base behavior's core methods delegate to the defaults.
	base := idealRootBehavior{}
	if !base.Match(NewValueRaw(node, ExtensionPayload{}), node) {
		t.Error("ideal behavior Match broken")
	}
	if base.Format(NewInteger(3)) == "" {
		t.Error("ideal behavior Format empty")
	}
	if !base.Equal(NewInteger(3), NewInteger(3)) {
		t.Error("ideal behavior Equal broken")
	}
}

func TestConvertErrorValue(t *testing.T) {
	plain := NewError(errors.New("boom"))
	m, err := ConvertIdealToMap(plain)
	if err != nil {
		t.Fatalf("error → map: %v", err)
	}
	mm, _ := AsMap(m)
	msg, ok := mm.Get("message")
	if !ok {
		t.Fatal("no message field")
	}
	if s, _ := AsString(msg); s != "boom" {
		t.Errorf("message = %q", s)
	}
	if _, hasCode := mm.Get("code"); hasCode {
		t.Error("plain Go error grew a code field")
	}

	// A BoruError contributes code and raise-payload data.
	data := NewOrderedMap()
	data.Set("extra", NewInteger(7))
	av := NewError(&BoruError{Code: "user_error", Detail: "bad", Data: data})
	m, _ = ConvertIdealToMap(av)
	mm, _ = AsMap(m)
	code, ok := mm.Get("code")
	if !ok {
		t.Fatal("no code field")
	}
	if c, _ := AsAtom(code); c != "user_error" {
		t.Errorf("code = %q", c)
	}
	if _, ok := mm.Get("extra"); !ok {
		t.Error("raise payload key lost")
	}

	l, _ := ConvertIdealToList(plain)
	ll, _ := AsList(l)
	if ll.Len() != 1 {
		t.Fatalf("error → list = %v", l)
	}
	if s, _ := AsString(ll.Get(0)); s != "boom" {
		t.Errorf("list message = %q", s)
	}
	// Direct negative: a non-error payload converts to the empty list.
	el, _ := errorConvertBehavior{}.ToList(NewInteger(1))
	ell, _ := AsList(el)
	if ell.Len() != 0 {
		t.Error("non-error → non-empty list")
	}
	if (errorConvertBehavior{}).Format(plain) == "" {
		t.Error("error Format empty")
	}
}

func TestConvertStoreValue(t *testing.T) {
	sv := NewStore(TStore)
	si, err := AsStore(sv)
	if err != nil {
		t.Fatalf("AsStore: %v", err)
	}
	si.Data["beta"] = NewInteger(2)
	si.Data["alpha"] = NewInteger(1)

	m, err := ConvertIdealToMap(sv)
	if err != nil {
		t.Fatalf("store → map: %v", err)
	}
	mm, _ := AsMap(m)
	if keys := mm.Keys(); len(keys) != 2 || keys[0] != "alpha" || keys[1] != "beta" {
		t.Errorf("store keys = %v, want sorted [alpha beta]", keys)
	}
	l, _ := ConvertIdealToList(sv)
	ll, _ := AsList(l)
	if ll.Len() != 2 {
		t.Errorf("store → list = %v", l)
	}
	if n, _ := AsInteger(ll.Get(0)); n != 1 {
		t.Errorf("first store value = %v, want 1 (alpha)", ll.Get(0))
	}
	// A non-store payload yields an empty entry map.
	if storeEntryMap(NewInteger(1)).Len() != 0 {
		t.Error("non-store produced entries")
	}
}

func TestConvertClassInstance(t *testing.T) {
	r := newTestRegistry(t)
	node := r.Types.MintType("W4bConvPt", TClass)
	fields := NewOrderedMap()
	fields.Set("x", NewInteger(1))
	fields.Set("y", NewInteger(2))
	inst := NewClassInstance(node, ClassInstanceInfo{Fields: fields})

	m, err := ConvertIdealToMap(inst)
	if err != nil {
		t.Fatalf("instance → map: %v", err)
	}
	mm, _ := AsMap(m)
	if mm.Len() != 2 {
		t.Errorf("instance map = %v", m)
	}
	l, _ := ConvertIdealToList(inst)
	ll, _ := AsList(l)
	if ll.Len() != 2 {
		t.Errorf("instance list = %v", l)
	}
	if n, _ := AsInteger(ll.Get(1)); n != 2 {
		t.Errorf("second field = %v", ll.Get(1))
	}
	// ClassFields exposes the flat field map; nil-safe.
	ci, _ := inst.Data().(ClassInstanceInfo)
	if ClassFields(&ci).Len() != 2 {
		t.Error("ClassFields lost fields")
	}
	if ClassFields(nil).Len() != 0 {
		t.Error("ClassFields(nil) not empty")
	}
	// Direct negatives: a non-instance converts to {} / [].
	em, _ := objectConvertBehavior{}.ToMap(NewInteger(1))
	emm, _ := AsMap(em)
	if emm.Len() != 0 {
		t.Error("non-instance → non-empty map")
	}
	el, _ := objectConvertBehavior{}.ToList(NewInteger(1))
	ell, _ := AsList(el)
	if ell.Len() != 0 {
		t.Error("non-instance → non-empty list")
	}
}

func TestConvertTableValue(t *testing.T) {
	rec := intStrRecord()
	row1 := NewOrderedMap()
	row1.Set("a", NewInteger(1))
	row1.Set("b", NewString("x"))
	row2 := NewOrderedMap()
	row2.Set("a", NewInteger(2))
	row2.Set("b", NewString("y"))
	tbl := NewValueRaw(TTable, TableData{
		Record: rec,
		Rows:   []Value{NewMap(row1), NewMap(row2)},
	})

	l, err := ConvertIdealToList(tbl)
	if err != nil {
		t.Fatalf("table → list: %v", err)
	}
	ll, _ := AsList(l)
	if ll.Len() != 2 {
		t.Errorf("rows = %v", l)
	}
	m, err := ConvertIdealToMap(tbl)
	if err != nil {
		t.Fatalf("table → map: %v", err)
	}
	mm, _ := AsMap(m)
	col, ok := mm.Get("a")
	if !ok {
		t.Fatal("columnar map missing column a")
	}
	cl, _ := AsList(col)
	if cl.Len() != 2 {
		t.Errorf("column a = %v", col)
	}
	// Non-table payloads degrade to [] / {}.
	el, _ := tableConvertBehavior{}.ToList(NewInteger(1))
	ell, _ := AsList(el)
	if ell.Len() != 0 {
		t.Error("non-table → rows")
	}
	em, _ := tableConvertBehavior{}.ToMap(NewInteger(1))
	emm, _ := AsMap(em)
	if emm.Len() != 0 {
		t.Error("non-table → columns")
	}
	if (tableConvertBehavior{}).Format(tbl) == "" {
		t.Error("table Format empty")
	}
}

func TestConvertReachValue(t *testing.T) {
	reach := NewReachFromKeys(NewWord("m"), []Value{NewString("a"), NewString("b")})
	m, err := ConvertIdealToMap(reach)
	if err != nil {
		t.Fatalf("reach → map: %v", err)
	}
	mm, _ := AsMap(m)
	if _, ok := mm.Get("receiver"); !ok {
		t.Error("no receiver field")
	}
	segs, ok := mm.Get("segments")
	if !ok {
		t.Fatal("no segments field")
	}
	sl, _ := AsList(segs)
	if sl.Len() != 2 {
		t.Errorf("segments = %v", segs)
	}
	seg0, _ := AsMap(sl.Get(0))
	op, _ := seg0.Get("op")
	if a, _ := AsAtom(op); a != "get" {
		t.Errorf("segment op = %v", op)
	}

	l, err := ConvertIdealToList(reach)
	if err != nil {
		t.Fatalf("reach → list: %v", err)
	}
	ll, _ := AsList(l)
	if ll.Len() != 2 {
		t.Errorf("reach keys = %v", l)
	}
	// Format renders the dotted surface.
	if got := reach.String(); !strings.Contains(got, "m.") {
		t.Errorf("reach render = %q", got)
	}
	// Getr + computed segments and a multi-token receiver.
	multi := NewReach(ReachInfo{
		Receiver: []Value{NewWord("f"), NewInteger(1)},
		Segments: []ReachSeg{
			{Getr: true, KeyLit: NewString("k")},
			{Computed: true, KeyExpr: []Value{NewWord("g")}},
		},
	})
	m, _ = ConvertIdealToMap(multi)
	mm, _ = AsMap(m)
	recv, _ := mm.Get("receiver")
	if !recv.Parent().ConformsTo(TList) {
		t.Errorf("multi-token receiver = %v, want a list", recv)
	}
	l, _ = ConvertIdealToList(multi)
	ll, _ = AsList(l)
	if ll.Len() != 2 || !ll.Get(1).Parent().ConformsTo(TList) {
		t.Errorf("computed key list = %v", l)
	}
	// Non-reach payloads degrade to {} / [].
	em, _ := reachConvertBehavior{}.ToMap(NewInteger(1))
	emm, _ := AsMap(em)
	if emm.Len() != 0 {
		t.Error("non-reach → map fields")
	}
	el, _ := reachConvertBehavior{}.ToList(NewInteger(1))
	ell, _ := AsList(el)
	if ell.Len() != 0 {
		t.Error("non-reach → list keys")
	}
}

// --- micron.go ------------------------------------------------------------------

// TestOrderedCompareCrossKindMicron pins the raising `cmp` wrapper's
// incomparable arm over cross-kind micron payloads (the family
// Comparer's cross-kind opt-out lives in basic; the raise is kernel).
// Payloads are built directly — the validators are content, the
// payloads and the ordering cascade are kernel.
func TestOrderedCompareCrossKindMicron(t *testing.T) {
	om := NewOrderedMap()
	om.Set("user", NewString("u"))
	om.Set("host", NewString("h.io"))
	emailon := NewValueRaw(TEmailon, MicronPayload{Fields: om})
	pathon := NewPathon([]string{"a"}, false)
	if _, err := orderedCompare("cmp", pathon, emailon); err == nil {
		t.Error("cross-kind micron cmp did not raise incomparable")
	}
}
