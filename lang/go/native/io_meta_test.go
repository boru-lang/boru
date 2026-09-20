package native

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// io_meta_test.go — P2 word-layer and wrapper coverage for ownership and
// extended attributes: the stat owner/group + {xattr} surface, touch
// {owner group xattr}, the permissioned gates, and the mount bridge's
// optional metadata handlers.

func TestStatOwnerGroupFields(t *testing.T) {
	r, mem := ioFSReg(t)
	mem.SetIdentity(func() int { return 7 }, func() int { return 8 })
	if err := mem.WriteFile("o.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := statField(t, r, "o.txt")
	owner, _ := m.Get("owner")
	group, _ := m.Get("group")
	if owner.String() != "7" || group.String() != "8" {
		t.Errorf("owner/group = %v/%v, want 7/8", owner, group)
	}
}

func TestStatXattrOption(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("x.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Seed one text and one binary attribute THROUGH the host mapping.
	if err := mem.XattrSet("x.txt", boruToHostXattr("note"), []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := mem.XattrSet("x.txt", boruToHostXattr("blob"), []byte{0xFF, 0x01}); err != nil {
		t.Fatal(err)
	}
	// A foreign-namespace attribute must not surface.
	if capabilities.XattrNamespacePrefix != "" {
		if err := mem.XattrSet("x.txt", "security.other", []byte("z")); err != nil {
			t.Fatal(err)
		}
	}

	res := runBoru(t, r, []Value{
		NewWord("stat"), pathV("x.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("xattr", NewBoolean(true)) }),
	})
	rec, err := AsMap(res[0])
	if err != nil {
		t.Fatal(err)
	}
	xv, ok := rec.Get("xattr")
	if !ok {
		t.Fatal("stat {xattr:true} record has no xattr field")
	}
	xm, err := AsMap(xv)
	if err != nil {
		t.Fatal(err)
	}
	if note, _ := xm.Get("note"); note.String() != "'hi'" {
		t.Errorf("xattr note = %v", note)
	}
	if blob, _ := xm.Get("blob"); !strings.HasPrefix(blob.String(), "Bytes<") {
		t.Errorf("binary xattr rendered as %v, want Bytes", blob)
	}
	if _, foreign := xm.Get("security.other"); foreign {
		t.Error("foreign-namespace attribute leaked into the display")
	}
	// Without the option the field is absent.
	m := statField(t, r, "x.txt")
	if _, has := m.Get("xattr"); has {
		t.Error("xattr field present without {xattr:true}")
	}
}

func TestTouchOwnerGroupXattr(t *testing.T) {
	r, mem := ioFSReg(t)
	mem.SetIdentity(func() int { return 0 }, func() int { return 0 }) // root: chown allowed

	// {owner}/{group} re-own; an omitted id stays.
	runBoru(t, r, []Value{
		NewWord("touch"), pathV("t.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("owner", NewInteger(42)) }),
	})
	fi, _ := mem.Stat("t.txt", true)
	if fi.UID != 42 || fi.GID != 0 {
		t.Errorf("touch {owner:42} = %d/%d", fi.UID, fi.GID)
	}
	runBoru(t, r, []Value{
		NewWord("touch"), pathV("t.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("group", NewInteger(9)) }),
	})
	if fi, _ = mem.Stat("t.txt", true); fi.GID != 9 || fi.UID != 42 {
		t.Errorf("touch {group:9} = %d/%d", fi.UID, fi.GID)
	}

	// {xattr:{…}} sets attributes (String and Bytes payloads).
	runBoru(t, r, []Value{
		NewWord("touch"), pathV("t.txt"),
		wrapMap(func(om *OrderedMap) {
			xs := NewOrderedMap()
			xs.Set("note", NewString("v"))
			xs.Set("raw", NewBytesValue([]byte{1, 2}))
			om.Set("xattr", NewMap(xs))
		}),
	})
	if v, err := mem.XattrGet("t.txt", boruToHostXattr("note")); err != nil || string(v) != "v" {
		t.Errorf("touch xattr note = %q (%v)", v, err)
	}
	if v, err := mem.XattrGet("t.txt", boruToHostXattr("raw")); err != nil || len(v) != 2 {
		t.Errorf("touch xattr raw = %v (%v)", v, err)
	}

	// A non-scalar xattr value declines.
	if err := runBoruError(t, r, []Value{
		NewWord("touch"), pathV("t.txt"),
		wrapMap(func(om *OrderedMap) {
			xs := NewOrderedMap()
			xs.Set("bad", NewList([]Value{NewInteger(1)}))
			om.Set("xattr", NewMap(xs))
		}),
	}); err == nil || !strings.Contains(err.Error(), "String or Bytes") {
		t.Errorf("list-valued xattr: err = %v", err)
	}

	// Chown compile failure surfaces through touch (non-root, real id change).
	mem.SetIdentity(func() int { return 1000 }, func() int { return 1000 })
	if err := runBoruError(t, r, []Value{
		NewWord("touch"), pathV("t.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("owner", NewInteger(0)) }),
	}); err == nil {
		t.Error("non-root touch {owner} should decline")
	}
}

func TestStatXattrErrorBranches(t *testing.T) {
	// A backend declining xattrs turns {xattr:true} into a loud stat error
	// (support-absence must not read as attribute-absence).
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	SetHostFileOps(r, notInstalledFileOps{})
	if err := runBoruError(t, r, []Value{
		NewWord("stat"), pathV("x"),
		wrapMap(func(om *OrderedMap) { om.Set("xattr", NewBoolean(true)) }),
	}); err == nil {
		t.Error("stat {xattr} over a declining backend should error")
	}

	// XattrGet failing mid-attach (list succeeds, get declines) propagates.
	r2, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r2)
	mem := capabilities.NewMem()
	if err := mem.WriteFile("g.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mem.XattrSet("g.txt", boruToHostXattr("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	SetHostFileOps(r2, &failAtOps{MemFileOps: mem, failXattrGet: "g.txt"})
	if err := runBoruError(t, r2, []Value{
		NewWord("stat"), pathV("g.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("xattr", NewBoolean(true)) }),
	}); err == nil {
		t.Error("mid-attach XattrGet failure should surface")
	}
}

func TestMapMapOpt(t *testing.T) {
	if _, ok := mapMapOpt(NewTypeLiteral(TMap), "xattr"); ok {
		t.Error("type-literal opts should report absent")
	}
	if _, ok := mapMapOpt(NewOptionsType(NewOrderedMap()), "xattr"); ok {
		t.Error("options-typed opts should report absent")
	}
	if _, ok := mapMapOpt(wrapMap(func(om *OrderedMap) { om.Set("xattr", NewInteger(1)) }), "xattr"); ok {
		t.Error("a non-map value should report absent")
	}
	if _, ok := mapMapOpt(wrapMap(func(om *OrderedMap) {}), "xattr"); ok {
		t.Error("a missing key should report absent")
	}
	opts := wrapMap(func(om *OrderedMap) {
		xs := NewOrderedMap()
		xs.Set("k", NewString("v"))
		om.Set("xattr", NewMap(xs))
	})
	m, ok := mapMapOpt(opts, "xattr")
	if !ok || m == nil {
		t.Fatal("mapMapOpt missed a concrete sub-map")
	}
	if v, _ := m.Get("k"); v.String() != "'v'" {
		t.Errorf("sub-map value = %v", v)
	}
}

func TestHostXattrNameMapping(t *testing.T) {
	host := boruToHostXattr("note")
	if capabilities.XattrNamespacePrefix != "" && !strings.HasPrefix(host, capabilities.XattrNamespacePrefix) {
		t.Errorf("boruToHostXattr(%q) = %q, want prefixed", "note", host)
	}
	back, ours := hostToBoruXattr(host)
	if !ours || back != "note" {
		t.Errorf("round trip = %q/%v", back, ours)
	}
	if capabilities.XattrNamespacePrefix != "" {
		if _, ours := hostToBoruXattr("security.selinux"); ours {
			t.Error("foreign namespace claimed as ours")
		}
	}
}

func TestPermissionedFileOpsGatesMetadata(t *testing.T) {
	// Sandbox (default-deny) declines all five new ops; trusted passes
	// them through to the inner backend.
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	ops := HostFileOps(r)
	assertDenied := func(name string, err error) {
		t.Helper()
		var d *policy.Denied
		if !errors.As(err, &d) {
			t.Errorf("%s: expected *policy.Denied, got %v", name, err)
		}
	}
	assertDenied("chown", ops.Chown("/tmp/x", -1, -1, true))
	_, err = ops.XattrGet("/tmp/x", "a")
	assertDenied("xattr-get", err)
	assertDenied("xattr-set", ops.XattrSet("/tmp/x", "a", nil))
	_, err = ops.XattrList("/tmp/x")
	assertDenied("xattr-list", err)
	assertDenied("xattr-remove", ops.XattrRemove("/tmp/x", "a"))

	// Allow-all: the wrapper delegates (mem backend answers).
	mem := capabilities.NewMem()
	if err := mem.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrapped := NewPermissionedFileOps(mem, loadPolicy(t, "trusted"))
	if err := wrapped.XattrSet("f", "a", []byte("1")); err != nil {
		t.Fatalf("trusted xattr-set: %v", err)
	}
	if v, err := wrapped.XattrGet("f", "a"); err != nil || string(v) != "1" {
		t.Errorf("trusted xattr-get = %q (%v)", v, err)
	}
	if names, err := wrapped.XattrList("f"); err != nil || len(names) != 1 {
		t.Errorf("trusted xattr-list = %v (%v)", names, err)
	}
	if err := wrapped.XattrRemove("f", "a"); err != nil {
		t.Errorf("trusted xattr-remove: %v", err)
	}
	if err := wrapped.Chown("f", -1, -1, true); err != nil {
		t.Errorf("trusted chown no-op: %v", err)
	}
}

func TestMountMetadataBridge(t *testing.T) {
	// A mount WITHOUT the metadata handlers declines each op cleanly.
	bare := HostFileOps(mountFixture(t, `mount { read: (p:Pathon => ['x']) }`))
	if err := bare.Chown("f", 1, 1, true); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("unbridged chown: %v", err)
	}
	if _, err := bare.XattrGet("f", "a"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("unbridged xattr-get: %v", err)
	}
	if err := bare.XattrSet("f", "a", []byte("v")); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("unbridged xattr-set: %v", err)
	}
	if _, err := bare.XattrList("f"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("unbridged xattr-list: %v", err)
	}
	if err := bare.XattrRemove("f", "a"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("unbridged xattr-remove: %v", err)
	}

	// A mount WITH handlers: chown forwards ids; xattr-get maps the
	// String/none conventions; xattr-list collects names; the stat
	// handler's owner/group fields flow into FileInfo.
	rich := HostFileOps(mountFixture(t, `mount {
	  read: (p:Pathon => [none])
	  "chown": ([p:Pathon uid:Integer gid:Integer] => [uid])
	  "xattr-get": ([p:Pathon n:String] => [if (n eq "here") ["v"] [none]])
	  "xattr-set": ([p:Pathon n:String v:Any] => [n])
	  "xattr-list": (p:Pathon => [["b" "a"]])
	  "xattr-remove": ([p:Pathon n:String] => [n])
	  stat: (p:Pathon => [{name:"f" size:1 owner:12 group:34}])
	}`))
	if err := rich.Chown("f", 1, 2, true); err != nil {
		t.Errorf("bridged chown: %v", err)
	}
	if v, err := rich.XattrGet("f", "here"); err != nil || string(v) != "v" {
		t.Errorf("bridged xattr-get = %q (%v)", v, err)
	}
	bytesMount := HostFileOps(mountFixture(t, `mount {
	  read: (p:Pathon => [none])
	  "xattr-get": ([p:Pathon n:String] => [(convert Bytes "bb")])
	}`))
	if v, err := bytesMount.XattrGet("f", "a"); err != nil || string(v) != "bb" {
		t.Errorf("bridged Bytes xattr-get = %q (%v)", v, err)
	}
	if _, err := rich.XattrGet("f", "absent"); err == nil || !strings.Contains(err.Error(), "attribute not found") {
		t.Errorf("bridged absent attr: %v", err)
	}
	names, err := rich.XattrList("f")
	if err != nil || len(names) != 2 || names[0] != "a" {
		t.Errorf("bridged xattr-list = %v (%v) — want sorted [a b]", names, err)
	}
	if err := rich.XattrRemove("f", "a"); err != nil {
		t.Errorf("bridged xattr-remove: %v", err)
	}
	if err := rich.XattrSet("f", "a", []byte{0xFF}); err != nil { // Bytes payload arm
		t.Errorf("bridged xattr-set bytes: %v", err)
	}
	fi, err := rich.Stat("f", true)
	if err != nil || fi.UID != 12 || fi.GID != 34 {
		t.Errorf("bridged stat owner = %d/%d (%v), want 12/34", fi.UID, fi.GID, err)
	}

	// A stat handler OMITTING owner/group reports the -1 sentinel, and a
	// name-string list entry does too.
	plain := HostFileOps(mountFixture(t, `mount {
	  read: (p:Pathon => [none])
	  stat: (p:Pathon => [{name:"f"}])
	  list: (p:Pathon => [["child"]])
	}`))
	if fi, err := plain.Stat("f", true); err != nil || fi.UID != -1 || fi.GID != -1 {
		t.Errorf("ownerless stat = %d/%d (%v), want -1/-1", fi.UID, fi.GID, err)
	}
	infos, err := plain.ReadDir("d")
	if err != nil || len(infos) != 1 || infos[0].UID != -1 {
		t.Errorf("name-entry ReadDir owner = %+v (%v)", infos, err)
	}

	// An xattr-get handler returning a non-scalar is a shaped error; a
	// non-list xattr-list likewise.
	broken := HostFileOps(mountFixture(t, `mount {
	  read: (p:Pathon => [none])
	  "xattr-get": ([p:Pathon n:String] => [[1 2]])
	  "xattr-list": (p:Pathon => [7])
	}`))
	if _, err := broken.XattrGet("f", "a"); err == nil || !strings.Contains(err.Error(), "want String, Bytes, or none") {
		t.Errorf("mis-shaped xattr-get: %v", err)
	}
	if _, err := broken.XattrList("f"); err == nil || !strings.Contains(err.Error(), "want a list") {
		t.Errorf("mis-shaped xattr-list: %v", err)
	}
}

// TestMetaFailInjection covers the remaining word-layer error arms: an
// XattrList failure inside attachXattrs, and an XattrSet failure inside
// applyTouch — both against a host-installed failing backend (the mem
// context toggle must stay OFF so the wrapper is consulted).
func TestMetaFailInjection(t *testing.T) {
	newReg := func(f *failAtOps) *Registry {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		registerIOWords(r)
		mem := capabilities.NewMem()
		if err := mem.WriteFile("f.txt", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		f.MemFileOps = mem
		SetHostFileOps(r, f)
		return r
	}

	r := newReg(&failAtOps{failXattrList: "f.txt"})
	if err := runBoruError(t, r, []Value{
		NewWord("stat"), pathV("f.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("xattr", NewBoolean(true)) }),
	}); err == nil {
		t.Error("attachXattrs XattrList failure should surface")
	}

	r = newReg(&failAtOps{failXattrSet: "f.txt"})
	if err := runBoruError(t, r, []Value{
		NewWord("touch"), pathV("f.txt"),
		wrapMap(func(om *OrderedMap) {
			xs := NewOrderedMap()
			xs.Set("note", NewString("v"))
			om.Set("xattr", NewMap(xs))
		}),
	}); err == nil {
		t.Error("applyTouch XattrSet failure should surface")
	}
}

// TestStripXattrNS covers BOTH prefix regimes of the namespace core —
// the platform constant fixes one arm per build, so the core is tested
// directly with each.
func TestStripXattrNS(t *testing.T) {
	if name, ok := stripXattrNS("", "note"); !ok || name != "note" {
		t.Errorf("flat regime = %q/%v", name, ok)
	}
	if name, ok := stripXattrNS("user.", "user.note"); !ok || name != "note" {
		t.Errorf("prefixed regime = %q/%v", name, ok)
	}
	if _, ok := stripXattrNS("user.", "security.selinux"); ok {
		t.Error("foreign namespace claimed")
	}
}
