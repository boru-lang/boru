package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// TestMiniSpHandler drives the sp handler over crafted subjects, covering
// every result projection (node-set element, text node, string / boolean /
// number scalar), the Map and List subject builders, and the three error
// arms (non-string src, non-container subject, malformed XPath).
func TestMiniSpHandler(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	opts := native.NewMap(native.NewOrderedMap())

	// Subject {a:1 b:{c:2}}: a scalar entry and a nested-map entry.
	inner := native.NewOrderedMap()
	inner.Set("c", native.NewInteger(2))
	om := native.NewOrderedMap()
	om.Set("a", native.NewInteger(1))
	om.Set("b", native.NewMap(inner))
	doc := native.NewMap(om)

	first := func(src string) native.Value {
		t.Helper()
		out, herr := miniSpHandler([]native.Value{native.NewString(src), opts, doc}, nil, nil, r)
		if herr != nil {
			t.Fatalf("sp %q: %v", src, herr)
		}
		lst, lerr := native.AsList(out[0])
		if lerr != nil || lst.Len() == 0 {
			t.Fatalf("sp %q: no result (%v)", src, out)
		}
		return lst.Get(0)
	}

	// A matched element comes back as its source value.
	if v, _ := addrOf(first("//a")).AsConcreteInteger(); v != 1 {
		t.Errorf("//a = %v, want 1", v)
	}
	// A text() node comes back as a String.
	if s, _ := addrOf(first("//a/text()")).AsConcreteString(); s != "1" {
		t.Errorf("//a/text() = %q, want \"1\"", s)
	}
	// A string() result is a String.
	if s, _ := addrOf(first("string(/a)")).AsConcreteString(); s != "1" {
		t.Errorf("string(/a) = %q, want \"1\"", s)
	}
	// A boolean result is a Boolean.
	if b, _ := addrOf(first("1 > 0")).AsConcreteBoolean(); !b {
		t.Errorf("1 > 0 not true")
	}
	// An integral number is an Integer, a fractional one a Float.
	if v, _ := addrOf(first("count(//a)")).AsConcreteInteger(); v != 1 {
		t.Errorf("count(//a) = %v, want 1", v)
	}
	if f, _ := addrOf(first("1 div 4")).AsConcreteFloat(); f != 0.25 {
		t.Errorf("1 div 4 = %v, want 0.25", f)
	}

	// A List subject builds `item` elements.
	lst := native.NewList([]native.Value{native.NewInteger(10), native.NewInteger(20)})
	out, herr := miniSpHandler([]native.Value{native.NewString("//item"), opts, lst}, nil, nil, r)
	if herr != nil {
		t.Fatalf("list subject: %v", herr)
	}
	if rl, _ := native.AsList(out[0]); rl.Len() != 2 {
		t.Errorf("//item over a 2-element list = %v", out)
	}

	// Error arms.
	if _, e := miniSpHandler([]native.Value{native.NewTypeLiteral(native.TString), opts, doc}, nil, nil, r); e == nil {
		t.Error("non-string src should error")
	}
	if _, e := miniSpHandler([]native.Value{native.NewString("/a"), opts, native.NewTypeLiteral(native.TMap)}, nil, nil, r); e == nil {
		t.Error("non-container subject should error")
	}
	if _, e := miniSpHandler([]native.Value{native.NewString("//["), opts, doc}, nil, nil, r); e == nil {
		t.Error("malformed XPath should error")
	}
}
