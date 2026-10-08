package modules

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// s7bTypeNative returns the named inner native from typeModuleNatives.
func s7bTypeNative(t *testing.T, name string) native.NativeFunc {
	t.Helper()
	for _, nf := range typeModuleNatives {
		if nf.Name == name {
			return nf
		}
	}
	t.Fatalf("type native %q not found", name)
	return native.NativeFunc{}
}

func s7bTypeHandler(t *testing.T, name string) native.Handler {
	t.Helper()
	gi, ok := s7bTypeNative(t, name).Signatures[0].Impl.(*core.GoImpl)
	if !ok {
		t.Fatalf("%s: Impl not *core.GoImpl", name)
	}
	return gi.Handler
}

// s7bFn builds a Function value from raw signatures.
func s7bFn(sigs ...native.FnSig) native.Value {
	return native.NewFunction(native.FnDefInfo{Name: "s7bfn", Signatures: sigs})
}

// TestS7B_TypeBodyArgErrors drives the typeBodyArg / AsConcreteAtom compile failure
// arms of the binary and unary type words, in BOTH dispatch orders so each
// arg position's guard fires.
func TestS7B_TypeBodyArgErrors(t *testing.T) {
	cases := []string{
		// exclude: args[0] (what) then args[1] (target).
		`Integer TypeUtil.exclude 5`,
		`5 TypeUtil.exclude Integer`,
		// extract: same two arms.
		`Integer TypeUtil.extract 5`,
		`5 TypeUtil.extract Integer`,
		// lca: both arms.
		`Integer TypeUtil.lca 5`,
		`5 TypeUtil.lca Integer`,
		// alts / nominal: unary non-type.
		`TypeUtil.alts 5`,
		`TypeUtil.nominal 5`,
		// omit: non-record/object target → fields==nil.
		`Integer TypeUtil.omit [x/q]`,
		// brand: args[0] tag not an atom, then args[1] base not a type.
		`Integer TypeUtil.brand 5`,
		`5 TypeUtil.brand userid/q`,
	}
	for _, expr := range cases {
		runTypeErr(t, expr, "")
	}
}

// TestS7B_TypeLCANoCommon drives lca's no-common-ancestor fallback: a
// degenerate root (Never) shares no lattice ancestor with Integer, so the
// walk finds nothing and falls back to Any.
func TestS7B_TypeLCANoCommon(t *testing.T) {
	got := runType(t, `Integer TypeUtil.lca Never`)
	if len(got) != 1 || got[0].String() != "Any" {
		t.Errorf("Integer TypeUtil.lca Never = %v, want Any", formatResults(got))
	}
}

// TestS7B_TypeFnIntrospectionEdges drives the introspection words' empty /
// untyped-shape arms directly (no boru surface can build a zero-sig or
// nil-param-type Function).
func TestS7B_TypeFnIntrospectionEdges(t *testing.T) {
	r := typeRegistry(t)

	// paramsof / arityof over a zero-signature Function.
	noSig := s7bFn()
	pOut, err := s7bTypeHandler(t, "paramsof")([]native.Value{noSig}, nil, nil, r)
	if err != nil || len(pOut) != 1 || pOut[0].String() != "[]" {
		t.Errorf("paramsof(no sigs) = %v (err %v), want []", pOut, err)
	}
	aOut, err := s7bTypeHandler(t, "arityof")([]native.Value{noSig}, nil, nil, r)
	if err != nil || len(aOut) != 1 || aOut[0].String() != "0" {
		t.Errorf("arityof(no sigs) = %v (err %v), want 0", aOut, err)
	}

	// paramsof over a sig with an untyped (nil-Type) param → Any element.
	nilParam := s7bFn(native.FnSig{Params: []native.FnParam{{Type: nil}}})
	nOut, err := s7bTypeHandler(t, "paramsof")([]native.Value{nilParam}, nil, nil, r)
	if err != nil || len(nOut) != 1 || nOut[0].String() != "[Any]" {
		t.Errorf("paramsof(nil-type param) = %v (err %v), want [Any]", nOut, err)
	}

	// returnsof over a sig with empty Returns → Any.
	noRet := s7bFn(native.FnSig{Params: []native.FnParam{{Type: native.TInteger}}, Returns: nil})
	rOut, err := s7bTypeHandler(t, "returnsof")([]native.Value{noRet}, nil, nil, r)
	if err != nil || len(rOut) != 1 || rOut[0].String() != "Any" {
		t.Errorf("returnsof(empty returns) = %v (err %v), want Any", rOut, err)
	}
}

// TestS7B_TypeReturnsofCheckFallback drives returnsof's check-mode ReturnsFn
// fallback: a non-function arg makes returnsofResult error, so the shape
// falls back to a bare Any carrier.
func TestS7B_TypeReturnsofCheckFallback(t *testing.T) {
	r := typeRegistry(t)
	rf := s7bTypeNative(t, "returnsof").Signatures[0].ReturnsFn
	if rf == nil {
		t.Fatal("returnsof has no ReturnsFn")
	}
	out := rf([]native.Value{native.NewInteger(5)}, r)
	if len(out) != 1 {
		t.Fatalf("ReturnsFn fallback: got %d values, want 1", len(out))
	}
	if !out[0].Parent.Equal(native.TAny) {
		t.Errorf("fallback carrier parent = %s, want Any", out[0].Parent.String())
	}
}
