package test

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestTypeDeclInstalledUnderTwoNames pins that installing a declaration
// under a type name never writes the declaration: a `gen` schema or a
// surface bound first to a lower-case name and then installed as two types
// is two types. The installer completed the shared declaration in place, so
// the second install renamed the first type's schema (`Box of [Integer]`
// rendered as Bag's) and both surfaces shared one conformance set (a type
// exposing one conformed to the other).
func TestTypeDeclInstalledUnderTwoNames(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def b (gen [T] class {x:T}) end def Box b end def Bag b end [(Box of [Integer]) (pathof (Box of [Integer]))]`,
			`[object<Box of [Integer]>{x:Integer} [Ideal Class Box Box of [Integer]]]`},
		{`def s (surface {area: (fnsig [[Self] [Float]])}) end def Shape s end def Other s end ` +
			`def Circle class {r:Float} end def area fn [[c:Circle][Float][1.0]] end ` +
			`Circle exposes Shape def c (make Circle {r: 1.0}) [(c is Shape) (c is Other)]`,
			`[true false]`},
	} {
		res, err := runNativeSteps(t, nil, []string{c.src})
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if got := core.Canon(res); got != c.want {
			t.Errorf("%s\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}
