package core

import (
	"runtime"
	"strings"
	"testing"
)

// ── fixtures ─────────────────────────────────────────────────────────

func freshFlexMap(t *testing.T) Value {
	t.Helper()
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	fv, err := FlexDeepCopy(NewMap(om))
	if err != nil {
		t.Fatalf("flex: %v", err)
	}
	return fv
}

func freshWeakMap() (Value, *WeakFlexMapData) {
	v := NewWeakFlexMap()
	return v, v.Data.(*WeakFlexMapData)
}

// ── classification ───────────────────────────────────────────────────

func TestClassifyWeakStrongScalars(t *testing.T) {
	for _, v := range []Value{NewInteger(7), NewString("s"), NewBoolean(true), NewAtom("a"), NewFloat(1.5)} {
		slot, refusal := classifyWeak(v)
		if refusal != nil {
			t.Fatalf("scalar %s refused: %+v", v.String(), refusal)
		}
		if slot.ref != nil {
			t.Fatalf("scalar %s stored weakly", v.String())
		}
	}
}

func TestClassifyWeakHandles(t *testing.T) {
	fm := freshFlexMap(t)
	fl := NewFlexList([]Value{NewInteger(1)})
	fx := NewFlexXml("a", nil, nil)
	wm := NewWeakFlexMap()
	wl := NewWeakFlexList()
	wx := NewWeakFlexXml("w")
	st := NewStore(TStore)
	oi := NewValueRaw(TMap, ClassInstanceInfo{Fields: NewOrderedMap()})
	ri := NewValueRaw(TMap, ResourceInstanceInfo{Fields: NewOrderedMap()})
	for _, v := range []Value{fm, fl, fx, wm, wl, wx, st, oi, ri} {
		slot, refusal := classifyWeak(v)
		if refusal != nil {
			t.Fatalf("handle %T refused: %+v", v.Data, refusal)
		}
		if slot.ref == nil {
			t.Fatalf("handle %T stored strongly", v.Data)
		}
		got, alive := slot.ref.revive()
		if !alive {
			t.Fatalf("handle %T dead while pinned", v.Data)
		}
		if got.Parent == nil || !got.Parent.Equal(v.Parent) {
			t.Fatalf("revived %T lost its Parent tag", v.Data)
		}
	}
	// Class/Resource instances with nil Fields are NOT weak-eligible.
	for _, v := range []Value{
		NewValueRaw(TMap, ClassInstanceInfo{}),
		NewValueRaw(TMap, ResourceInstanceInfo{}),
	} {
		if _, refusal := classifyWeak(v); refusal == nil {
			t.Fatalf("nil-Fields instance %T accepted", v.Data)
		}
	}
}

func TestClassifyWeakRefusals(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	fnVal := NewValueRaw(TFunction, &FnDefInfo{Name: "f"})
	errVal := NewValueRaw(TAny, ErrorInfo{Message: "boom"})
	record := NewRecordType(NewOrderedMap())
	cases := []struct {
		v    Value
		kind string
	}{
		{NewNone(), "none"},
		{NewTypeLiteral(TInteger), "a type literal"},
		{NewMap(om), "an immutable Map"},
		{NewList([]Value{NewInteger(1)}), "an immutable List"},
		{NewXmlElement("a", nil, nil), "an immutable Xml element"},
		{record, "a structural type body"},
		{fnVal, "a fn value"},
		{errVal, "an Error value"},
		{NewValueRaw(TWord, WordInfo{Name: "w"}), "a Word value"},
	}
	for _, c := range cases {
		_, refusal := classifyWeak(c.v)
		if refusal == nil {
			t.Fatalf("%s not refused", c.kind)
		}
		if refusal.Kind != c.kind {
			t.Fatalf("kind = %q, want %q", refusal.Kind, c.kind)
		}
		if refusal.Note == "" || refusal.Help == "" {
			t.Fatalf("%s refusal missing note/help", c.kind)
		}
	}
}

func TestWeakRefusalErrorRendering(t *testing.T) {
	om := NewOrderedMap()
	_, refusal := classifyWeak(NewMap(om))
	err := WeakRefusalError(nil, "set", "WeakFlexMap", refusal)
	msg := err.Error()
	for _, want := range []string{"weak_value_error", "cannot store an immutable Map in a WeakFlexMap", "= note:", "= help:"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error missing %q:\n%s", want, msg)
		}
	}
	if err2 := weakRefusalError(nil, "make", "WeakFlexList", refusal); err2 == nil {
		t.Fatal("internal spelling returned nil")
	}
}

// ── predicates / accessors ───────────────────────────────────────────

func TestWeakPredicatesAndAccessors(t *testing.T) {
	wm, _ := freshWeakMap()
	wl := NewWeakFlexList()
	wx := NewWeakFlexXml("t")
	if !IsWeakFlexMap(wm) || !IsWeakFlexList(wl) || !IsWeakFlexXml(wx) {
		t.Fatal("positive predicates failed")
	}
	for _, v := range []Value{wm, wl, wx} {
		if !IsWeakFlexNode(v) {
			t.Fatal("IsWeakFlexNode false for weak node")
		}
	}
	plain := NewMap(NewOrderedMap())
	if IsWeakFlexMap(plain) || IsWeakFlexList(plain) || IsWeakFlexXml(plain) || IsWeakFlexNode(plain) {
		t.Fatal("negative predicates failed on plain map")
	}
	// Bare literals: right Parent, nil payload.
	for _, lit := range []Value{NewTypeLiteral(TWeakFlexMap), NewTypeLiteral(TWeakFlexList), NewTypeLiteral(TWeakFlexXml)} {
		if IsWeakFlexNode(lit) {
			t.Fatal("literal classified as weak node")
		}
	}
	if _, err := AsWeakFlexMap(plain); err == nil {
		t.Fatal("AsWeakFlexMap accepted plain map")
	}
	if _, err := AsWeakFlexList(plain); err == nil {
		t.Fatal("AsWeakFlexList accepted plain map")
	}
	if _, err := AsWeakFlexXml(plain); err == nil {
		t.Fatal("AsWeakFlexXml accepted plain map")
	}
	if _, err := AsWeakFlexMap(wm); err != nil {
		t.Fatal(err)
	}
	if _, err := AsWeakFlexList(wl); err != nil {
		t.Fatal(err)
	}
	if _, err := AsWeakFlexXml(wx); err != nil {
		t.Fatal(err)
	}
}

// ── map operations ───────────────────────────────────────────────────

func TestWeakMapSetSnapshotAndPosition(t *testing.T) {
	_, wd := freshWeakMap()
	if r := wd.SetValue("b", NewInteger(2)); r != nil {
		t.Fatalf("refused: %+v", r)
	}
	if r := wd.SetValue("a", NewInteger(1)); r != nil {
		t.Fatalf("refused: %+v", r)
	}
	if r := wd.SetValue("b", NewInteger(9)); r != nil {
		t.Fatalf("re-set refused: %+v", r)
	}
	snap := wd.snapshot()
	if got := strings.Join(snap.Keys(), ","); got != "b,a" {
		t.Fatalf("keys = %s, want b,a (insertion order, re-set keeps position)", got)
	}
	if v, _ := snap.Get("b"); weakInt(t, v) != 9 {
		t.Fatal("re-set did not replace")
	}
	if r := wd.SetValue("x", NewMap(NewOrderedMap())); r == nil {
		t.Fatal("immutable map accepted")
	}
}

func weakInt(t *testing.T, v Value) int64 {
	t.Helper()
	n, err := AsInteger(v)
	if err != nil {
		t.Fatalf("not an integer: %v", err)
	}
	return n
}

// ── collection (the GC-dependent branches) ───────────────────────────

// storeCollectible stores a freshly-allocated flex handle whose only
// strong reference is this frame — gone once we return.
//
//go:noinline
func storeCollectible(wd *WeakFlexMapData) {
	om := NewOrderedMap()
	om.Set("x", NewInteger(1))
	fv, _ := FlexDeepCopy(NewMap(om))
	if r := wd.SetValue("dead", fv); r != nil {
		panic("refused in fixture")
	}
}

//go:noinline
func appendCollectible(wd *WeakFlexListData) {
	fv, _ := FlexDeepCopy(NewList([]Value{NewInteger(1)}))
	if r := wd.Append(fv); r != nil {
		panic("refused in fixture")
	}
}

//go:noinline
func appendCollectibleChild(wd *WeakFlexXmlData) {
	fv, _ := FlexDeepCopy(NewXmlElement("c", nil, nil))
	if r := wd.AppendChild(fv); r != nil {
		panic("refused in fixture")
	}
}

func TestWeakMapCollection(t *testing.T) {
	_, wd := freshWeakMap()
	live := freshFlexMap(t)
	if r := wd.SetValue("live", live); r != nil {
		t.Fatalf("refused: %+v", r)
	}
	storeCollectible(wd)
	runtime.GC()
	runtime.GC()
	snap := wd.snapshot()
	if _, ok := snap.Get("dead"); ok {
		t.Fatal("collectible entry survived GC")
	}
	if _, ok := snap.Get("live"); !ok {
		t.Fatal("pinned entry was collected")
	}
	runtime.KeepAlive(live)
}

func TestWeakListCollection(t *testing.T) {
	wl := NewWeakFlexList()
	wd := wl.Data.(*WeakFlexListData)
	if r := wd.Append(NewInteger(7)); r != nil {
		t.Fatalf("refused: %+v", r)
	}
	appendCollectible(wd)
	runtime.GC()
	runtime.GC()
	if n := wd.Len(); n != 1 {
		t.Fatalf("post-GC length = %d, want 1 (splice-compact)", n)
	}
	elems := wd.snapshotElems()
	if len(elems) != 1 || weakInt(t, elems[0]) != 7 {
		t.Fatalf("survivor order broken: %v", elems)
	}
}

func TestWeakXmlCollection(t *testing.T) {
	wx := NewWeakFlexXml("root")
	wd := wx.Data.(*WeakFlexXmlData)
	if r := wd.AppendChild(NewString("text")); r != nil {
		t.Fatalf("refused: %+v", r)
	}
	appendCollectibleChild(wd)
	runtime.GC()
	runtime.GC()
	cren := wd.snapshotCren()
	if len(cren) != 1 {
		t.Fatalf("post-GC children = %d, want 1", len(cren))
	}
	wd.SetAttr("k", NewString("v"))
	if v, ok := wd.Attr.Get("k"); !ok || v.String() != "'v'" {
		t.Fatalf("attr write failed: %v %v", v, ok)
	}
	// nil-Attr arm.
	bare := &WeakFlexXmlData{Tag: "b"}
	bare.SetAttr("a", NewInteger(1))
	if bare.Attr == nil {
		t.Fatal("SetAttr did not mint Attr")
	}
}

// ── list index / bounds interplay ────────────────────────────────────

func TestWeakListSetIndex(t *testing.T) {
	wd := NewWeakFlexList().Data.(*WeakFlexListData)
	if r := wd.Append(NewInteger(1)); r != nil {
		t.Fatal("append refused")
	}
	if r := wd.SetIndex(0, NewInteger(9)); r != nil {
		t.Fatal("set refused")
	}
	if elems := wd.snapshotElems(); weakInt(t, elems[0]) != 9 {
		t.Fatal("set did not replace")
	}
	if r := wd.SetIndex(0, NewList(nil)); r == nil {
		t.Fatal("immutable list accepted at index")
	}
}

// ── populate (make sources) ──────────────────────────────────────────

func TestPopulateWeakContainers(t *testing.T) {
	src := NewOrderedMap()
	src.Set("n", NewInteger(1))
	out, refusal := PopulateWeakFlexMap(src)
	if refusal != nil || !IsWeakFlexMap(out) {
		t.Fatalf("populate map: %+v", refusal)
	}
	bad := NewOrderedMap()
	bad.Set("m", NewMap(NewOrderedMap()))
	if _, refusal = PopulateWeakFlexMap(bad); refusal == nil {
		t.Fatal("populate accepted immutable map entry")
	}
	lo, refusal := PopulateWeakFlexList(ReadList{elems: []Value{NewInteger(1)}})
	if refusal != nil || !IsWeakFlexList(lo) {
		t.Fatalf("populate list: %+v", refusal)
	}
	if _, refusal = PopulateWeakFlexList(ReadList{elems: []Value{NewList(nil)}}); refusal == nil {
		t.Fatal("populate accepted immutable list element")
	}
	attr := NewOrderedMap()
	attr.Set("k", NewString("v"))
	xo, refusal := PopulateWeakFlexXml("t", attr, []Value{NewString("hi")})
	if refusal != nil || !IsWeakFlexXml(xo) {
		t.Fatalf("populate xml: %+v", refusal)
	}
	om := NewOrderedMap()
	if _, refusal = PopulateWeakFlexXml("t", nil, []Value{NewMap(om)}); refusal == nil {
		t.Fatal("populate xml accepted immutable map child")
	}
}

// ── clone / freshen ──────────────────────────────────────────────────

func TestCloneWeakData(t *testing.T) {
	_, wd := freshWeakMap()
	live := freshFlexMap(t)
	wd.SetValue("s", NewInteger(1))
	wd.SetValue("w", live)
	ident := func(v Value) Value { return v }
	nd := CloneWeakFlexMapData(wd, ident)
	snap := nd.snapshot()
	if snap.Len() != 2 {
		t.Fatalf("clone lost entries: %d", snap.Len())
	}
	wl := NewWeakFlexList().Data.(*WeakFlexListData)
	wl.Append(NewInteger(1))
	wl.Append(live)
	if got := CloneWeakFlexListData(wl, ident); len(got.snapshotElems()) != 2 {
		t.Fatal("list clone lost elements")
	}
	wx := NewWeakFlexXml("t").Data.(*WeakFlexXmlData)
	wx.SetAttr("a", NewInteger(1))
	wx.AppendChild(live)
	nx := CloneWeakFlexXmlData(wx, ident, func(m *OrderedMap) *OrderedMap { return m })
	if len(nx.snapshotCren()) != 1 || nx.Tag != "t" {
		t.Fatal("xml clone broken")
	}
	runtime.KeepAlive(live)
}

func TestCloneValueWeakArms(t *testing.T) {
	wm, wd := freshWeakMap()
	wd.SetValue("n", NewInteger(1))
	cl := CloneValue(wm)
	if !IsWeakFlexMap(cl) {
		t.Fatalf("clone changed kind: %s", cl.Parent.String())
	}
	if sameContainer(cl.Data, wm.Data) {
		t.Fatal("clone shares the container")
	}
	// Shared payload appears once (seen-cache) inside one clone call.
	pair := NewList([]Value{wm, wm})
	cp := CloneValue(pair)
	lst, _ := AsList(cp)
	if !sameContainer(lst.Get(0).Data, lst.Get(1).Data) {
		t.Fatal("seen-cache miss: one payload cloned twice")
	}
	wlv := NewWeakFlexList()
	wlv.Data.(*WeakFlexListData).Append(NewInteger(1))
	if !IsWeakFlexList(CloneValue(wlv)) {
		t.Fatal("list clone changed kind")
	}
	lp := NewList([]Value{wlv, wlv})
	lc, _ := AsList(CloneValue(lp))
	if !sameContainer(lc.Get(0).Data, lc.Get(1).Data) {
		t.Fatal("list seen-cache miss")
	}
	wxv := NewWeakFlexXml("t")
	if !IsWeakFlexXml(CloneValue(wxv)) {
		t.Fatal("xml clone changed kind")
	}
	xp := NewList([]Value{wxv, wxv})
	xc, _ := AsList(CloneValue(xp))
	if !sameContainer(xc.Get(0).Data, xc.Get(1).Data) {
		t.Fatal("xml seen-cache miss")
	}
}

func TestFreshenDefaultWeakArm(t *testing.T) {
	wm, wd := freshWeakMap()
	wd.SetValue("n", NewInteger(1))
	fr := FreshenDefault(wm)
	if !IsWeakFlexMap(fr) {
		t.Fatalf("freshen changed kind: %s", fr.Parent.String())
	}
	if sameContainer(fr.Data, wm.Data) {
		t.Fatal("freshen shared the container")
	}
	wl := NewWeakFlexList()
	wl.Data.(*WeakFlexListData).Append(NewInteger(1))
	if !IsWeakFlexList(FreshenDefault(wl)) {
		t.Fatal("list freshen changed kind")
	}
	wx := NewWeakFlexXml("t")
	wx.Data.(*WeakFlexXmlData).SetAttr("a", NewInteger(1))
	fx := FreshenDefault(wx)
	if !IsWeakFlexXml(fx) {
		t.Fatal("xml freshen changed kind")
	}
	if fx.Data.(*WeakFlexXmlData).Attr.Len() != 1 {
		t.Fatal("xml freshen lost attrs")
	}
	if m := freshenAttrMap(nil); m == nil || m.Len() != 0 {
		t.Fatal("freshenAttrMap nil arm")
	}
}

// ── seam arms: accessors / equality / shape / family ─────────────────

func TestWeakSeamArms(t *testing.T) {
	wm, wd := freshWeakMap()
	wd.SetValue("n", NewInteger(42))

	// AsMap snapshot arm.
	m, err := AsMap(wm)
	if err != nil || m.Len() != 1 {
		t.Fatalf("AsMap arm: %v", err)
	}
	// AsMutableMap refuses — the loud-failure contract.
	if _, err := AsMutableMap(wm); err == nil {
		t.Fatal("AsMutableMap accepted weak payload")
	}
	// AsList arm.
	wl := NewWeakFlexList()
	wl.Data.(*WeakFlexListData).Append(NewInteger(1))
	lst, err := AsList(wl)
	if err != nil || lst.Len() != 1 {
		t.Fatalf("AsList arm: %v", err)
	}
	// xmlParts arm.
	wx := NewWeakFlexXml("t")
	wx.Data.(*WeakFlexXmlData).AppendChild(NewString("hi"))
	tag, _, cren, ok := xmlParts(wx)
	if !ok || tag != "t" || len(cren) != 1 {
		t.Fatal("xmlParts weak arm")
	}

	// nodeFamily folds.
	if !nodeFamily(TWeakFlexMap).Equal(TMap) || !nodeFamily(TWeakFlexList).Equal(TList) || !nodeFamily(TWeakFlexXml).Equal(TFlexXml) {
		t.Fatal("nodeFamily weak folds")
	}
	// Shape families.
	if Shape(wm) != ShapeMap {
		t.Fatalf("weak map shape = %v", Shape(wm))
	}
	if Shape(wl) != ShapeList {
		t.Fatalf("weak list shape = %v", Shape(wl))
	}
	// sameContainer weak arms (match and mismatch).
	if !sameContainer(wm.Data, wm.Data) {
		t.Fatal("weak map not self-identical")
	}
	other, _ := freshWeakMap()
	if sameContainer(wm.Data, other.Data) {
		t.Fatal("distinct weak maps identical")
	}
	if sameContainer(wl.Data, NewWeakFlexList().Data) {
		t.Fatal("distinct weak lists identical")
	}
	if !sameContainer(wl.Data, wl.Data) {
		t.Fatal("weak list not self-identical")
	}
	wx2 := NewWeakFlexXml("u")
	if sameContainer(wx.Data, wx2.Data) {
		t.Fatal("distinct weak xml identical")
	}
	if !sameContainer(wx.Data, wx.Data) {
		t.Fatal("weak xml not self-identical")
	}

	// containsFlex / adoption / deep copies.
	if !containsFlex(wm) {
		t.Fatal("containsFlex misses weak")
	}
	ad, err := AdoptIntoFlex(wm)
	if err != nil || !sameContainer(ad.Data, wm.Data) {
		t.Fatal("AdoptIntoFlex did not share the weak handle")
	}
	nd, err := NodeDeepCopy(wm)
	if err != nil || !nd.Parent.Equal(TMap) {
		t.Fatalf("NodeDeepCopy: %v %s", err, nd.Parent.String())
	}
	fd, err := FlexDeepCopy(wm)
	if err != nil || !fd.Parent.Equal(TFlexMap) {
		t.Fatalf("FlexDeepCopy: %v %s", err, fd.Parent.String())
	}

	// canon arms.
	if c := CanonValue(wm); c != "(make WeakFlexMap {n:42})" {
		t.Fatalf("map canon = %s", c)
	}
	if c := CanonValue(wl); c != "(make WeakFlexList [1])" {
		t.Fatalf("list canon = %s", c)
	}
	if c := CanonValue(wx); !strings.HasPrefix(c, "(make WeakFlexXml ") {
		t.Fatalf("xml canon = %s", c)
	}
}

// ── make targets ─────────────────────────────────────────────────────

func TestMakeNodeHandlerWeakTargets(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	src := NewOrderedMap()
	src.Set("n", NewInteger(1))
	out, err := MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexMap), NewMap(src)}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsWeakFlexMap(out[0]) {
		t.Fatalf("make weak map: %v", err)
	}
	if _, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexMap), NewList(nil)}, nil, nil, r); err == nil {
		t.Fatal("make weak map accepted a list source")
	}
	bad := NewOrderedMap()
	bad.Set("m", NewMap(NewOrderedMap()))
	if _, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexMap), NewMap(bad)}, nil, nil, r); err == nil {
		t.Fatal("make weak map accepted refused entry")
	}
	out, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexList), NewList([]Value{NewInteger(1)})}, nil, nil, r)
	if err != nil || !IsWeakFlexList(out[0]) {
		t.Fatalf("make weak list: %v", err)
	}
	if _, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexList), NewMap(src)}, nil, nil, r); err == nil {
		t.Fatal("make weak list accepted a map source")
	}
	if _, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexList), NewList([]Value{NewList(nil)})}, nil, nil, r); err == nil {
		t.Fatal("make weak list accepted refused element")
	}
	xml := NewXmlElement("a", nil, []Value{NewString("hi")})
	out, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexXml), xml}, nil, nil, r)
	if err != nil || !IsWeakFlexXml(out[0]) {
		t.Fatalf("make weak xml: %v", err)
	}
	if _, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexXml), NewMap(src)}, nil, nil, r); err == nil {
		t.Fatal("make weak xml accepted a map source")
	}
	badXml := NewXmlElement("a", nil, []Value{NewMap(NewOrderedMap())})
	if _, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexXml), badXml}, nil, nil, r); err == nil {
		t.Fatal("make weak xml accepted refused child")
	}
}

// ── unify literal arms ───────────────────────────────────────────────

func TestUnifyWeakLiteralArms(t *testing.T) {
	wm, _ := freshWeakMap()
	lit := NewTypeLiteral(TWeakFlexMap)
	res, ok := Unify(lit, wm)
	if !ok || !IsWeakFlexMap(res) {
		t.Fatal("WeakFlexMap literal vs weak value failed")
	}
	if _, ok = Unify(lit, NewMap(NewOrderedMap())); ok {
		t.Fatal("WeakFlexMap literal accepted a plain map")
	}
	if res, ok = Unify(NewTypeLiteral(TFlexMap), wm); !ok || !IsWeakFlexMap(res) {
		t.Fatal("FlexMap literal must accept its subtree")
	}
	wl := NewWeakFlexList()
	if res, ok = Unify(NewTypeLiteral(TWeakFlexList), wl); !ok || !IsWeakFlexList(res) {
		t.Fatal("WeakFlexList literal vs weak value failed")
	}
	if _, ok = Unify(NewTypeLiteral(TWeakFlexList), NewList(nil)); ok {
		t.Fatal("WeakFlexList literal accepted a plain list")
	}
	if res, ok = Unify(NewTypeLiteral(TFlexList), wl); !ok || !IsWeakFlexList(res) {
		t.Fatal("FlexList literal must accept its subtree")
	}
	// Both-literal arm: two WeakFlexMap literals unify to the same node.
	res, ok = Unify(lit, NewTypeLiteral(TWeakFlexMap))
	if !ok || !denotedType(res).Equal(TWeakFlexMap) {
		t.Fatalf("both-literal arm: ok=%v res=%s", ok, res.String())
	}
}

// ── residual guard coverage (ADR-008) ────────────────────────────────

// TestMakeNodeHandlerWeakUnreadableSources drives the conforms-but-
// unreadable guards: values in the right FAMILY whose payload the
// reader cannot surface (a record body for maps, a bare typed-list
// carrier for lists, an interp skeleton for xml).
func TestMakeNodeHandlerWeakUnreadableSources(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	record := NewRecordType(NewOrderedMap())
	if _, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexMap), record}, nil, nil, r); err == nil {
		t.Fatal("record body accepted as WeakFlexMap source")
	}
	typedList := NewTypedList(NewTypeLiteral(TInteger))
	if _, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexList), typedList}, nil, nil, r); err == nil {
		t.Fatal("typed-list carrier accepted as WeakFlexList source")
	}
	interp := NewValueRaw(TXml, ParenExprPayload{})
	if _, err = MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexXml), interp}, nil, nil, r); err == nil {
		t.Fatal("non-element xml payload accepted as WeakFlexXml source")
	}
}

// TestWeakMapSetValueZeroStruct covers the nil-slots lazy-init arm.
func TestWeakMapSetValueZeroStruct(t *testing.T) {
	wd := &WeakFlexMapData{}
	if r := wd.SetValue("k", NewInteger(1)); r != nil {
		t.Fatalf("refused: %+v", r)
	}
	if wd.snapshot().Len() != 1 {
		t.Fatal("zero-struct set lost the entry")
	}
}

// ── Codex-round seams: exported classifier, compare views, make tag carry ──

// TestClassifyWeakRefusalExport pins the exported wrapper the word
// layer's check-time mirror consults (weakValueMirror).
func TestClassifyWeakRefusalExport(t *testing.T) {
	if r := ClassifyWeakRefusal(NewInteger(1)); r != nil {
		t.Fatalf("scalar refused: %+v", r)
	}
	om := NewOrderedMap()
	if r := ClassifyWeakRefusal(NewMap(om)); r == nil || r.Kind != "an immutable Map" {
		t.Fatalf("immutable map verdict = %+v", r)
	}
}

// TestCompareWeakContainers pins cmp as CONTENT ordering, mode-blind
// (Codex round: the canon fallback ordered every weak container before
// its plain sibling because of the `(make WeakFlex…)` prefix).
func TestCompareWeakContainers(t *testing.T) {
	om := NewOrderedMap()
	om.Set("n", NewInteger(42))
	weakMap, refusal := PopulateWeakFlexMap(om)
	if refusal != nil {
		t.Fatalf("populate map: %+v", refusal)
	}
	weakList, refusal := PopulateWeakFlexList(ReadList{elems: []Value{NewInteger(1), NewInteger(2)}})
	if refusal != nil {
		t.Fatalf("populate list: %+v", refusal)
	}
	plainMap := NewMap(om)
	plainList := NewList([]Value{NewInteger(1), NewInteger(2)})
	for _, tc := range []struct {
		name string
		a, b Value
		want int
	}{
		{"weak map vs same plain map", weakMap, plainMap, 0},
		{"weak list vs same plain list", weakList, plainList, 0},
		{"plain map vs weak map (flipped)", plainMap, weakMap, 0},
	} {
		got, err := CompareValues(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("%s: cmp = %d, %v (want %d)", tc.name, got, err, tc.want)
		}
	}
	// …and stays contentful: a differing entry still orders.
	om2 := NewOrderedMap()
	om2.Set("n", NewInteger(43))
	if got, err := CompareValues(weakMap, NewMap(om2)); err != nil || got != -1 {
		t.Fatalf("differing content cmp = %d, %v", got, err)
	}
	// Non-container inputs decline both views (the false arms).
	if _, ok := compareListView(NewInteger(1)); ok {
		t.Fatal("integer produced a list view")
	}
	if _, ok := compareMapView(NewInteger(1)); ok {
		t.Fatal("integer produced a map view")
	}
}

// TestMakeWeakCarriesElemConstraint pins the {:T}/[:T] carry through
// make for BOTH tag homes: the header elem (a typed-def-unified
// source) and a typed literal's ChildTypeInfo payload.
func TestMakeWeakCarriesElemConstraint(t *testing.T) {
	intLit := NewTypeLiteral(TInteger)

	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	headerMap := NewMap(om)
	headerMap.SetElemConstraint(intLit)
	literalMap := NewValueRaw(TMap, ChildTypeInfo{
		Child:   intLit,
		Entries: []ChildEntry{{Key: "a", Value: NewInteger(1)}},
	})
	headerList := NewList([]Value{NewInteger(1)})
	headerList.SetElemConstraint(intLit)
	literalList := NewValueRaw(TList, ChildTypeInfo{
		Child:    intLit,
		Elements: []Value{NewInteger(1)},
	})

	for _, tc := range []struct {
		name   string
		target *Type
		src    Value
	}{
		{"map header elem", TWeakFlexMap, headerMap},
		{"map typed literal", TWeakFlexMap, literalMap},
		{"list header elem", TWeakFlexList, headerList},
		{"list typed literal", TWeakFlexList, literalList},
	} {
		out, err := MakeNodeHandler([]Value{NewTypeLiteral(tc.target), tc.src}, nil, nil, nil)
		if err != nil || len(out) != 1 {
			t.Fatalf("%s: make failed: %v", tc.name, err)
		}
		elem, ok := out[0].ElemConstraint()
		if !ok || !elem.Equal(TInteger) {
			t.Fatalf("%s: constraint not carried (ok=%v)", tc.name, ok)
		}
	}
	// An untagged source stays untagged.
	plain := NewOrderedMap()
	out, err := MakeNodeHandler([]Value{NewTypeLiteral(TWeakFlexMap), NewMap(plain)}, nil, nil, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("plain make failed: %v", err)
	}
	if _, ok := out[0].ElemConstraint(); ok {
		t.Fatal("untagged source grew a constraint")
	}
}

// ── deletion (the `del` half of the storage column, NUR022) ──────────

func TestWeakMapDeleteKey(t *testing.T) {
	_, wd := freshWeakMap()
	for _, k := range []string{"a", "b", "c"} {
		if r := wd.SetValue(k, NewInteger(1)); r != nil {
			t.Fatalf("set %s refused: %+v", k, r)
		}
	}

	// Removing a middle key must drop it from the ORDER as well as the
	// slots — a stale key would resurface as a zero-value entry.
	wd.DeleteKey("b")
	snap := wd.snapshot()
	if got := strings.Join(snap.Keys(), ","); got != "a,c" {
		t.Fatalf("keys = %s, want a,c", got)
	}
	if _, ok := snap.Get("b"); ok {
		t.Fatal("deleted key still readable")
	}

	// Idempotent and total: deleting an absent key changes nothing.
	wd.DeleteKey("b")
	wd.DeleteKey("never")
	if got := strings.Join(wd.snapshot().Keys(), ","); got != "a,c" {
		t.Fatalf("after no-op deletes keys = %s, want a,c", got)
	}

	// A re-set after a delete appends at the END — the key is genuinely
	// gone, not merely hidden, so it does not reclaim its old position.
	if r := wd.SetValue("b", NewInteger(2)); r != nil {
		t.Fatalf("re-set refused: %+v", r)
	}
	if got := strings.Join(wd.snapshot().Keys(), ","); got != "a,c,b" {
		t.Fatalf("keys = %s, want a,c,b", got)
	}
}

func TestWeakXmlDeleteAttr(t *testing.T) {
	v := NewWeakFlexXml("a")
	wd := v.Data.(*WeakFlexXmlData)

	// No attribute map at all (the constructor allocates one, but a
	// zero-struct payload — the same shape TestWeakMapSetValueZeroStruct
	// drives — does not): deleting must be a no-op, not a nil write.
	var zero WeakFlexXmlData
	zero.DeleteAttr("gone")
	if zero.Attr != nil {
		t.Fatal("DeleteAttr on a nil Attr must not allocate")
	}

	wd.SetAttr("b", NewString("1"))
	wd.SetAttr("c", NewString("2"))
	wd.DeleteAttr("b")
	if _, ok := wd.Attr.Get("b"); ok {
		t.Fatal("deleted attribute still present")
	}
	if _, ok := wd.Attr.Get("c"); !ok {
		t.Fatal("delete removed the wrong attribute")
	}
	// Absent name is a no-op.
	wd.DeleteAttr("never")
	if wd.Attr.Len() != 1 {
		t.Fatalf("attr count = %d, want 1", wd.Attr.Len())
	}
}
