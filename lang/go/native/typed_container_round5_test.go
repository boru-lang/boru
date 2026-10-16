package native

import (
	"strings"
	"testing"
)

// Codex round-5 fixes (design/TYPED-CONTAINER-TAG-RETENTION.0.md):
//
//	#1 `vals` projects a {:T} map's values to a [:T] list — retain the tag
//	   (runtime + a typed check residual). `keys` stays untagged (String keys).
//	#2 the list-concat `append` overload ([TList, TFlexList]) had no ReturnsFn,
//	   so check mode never preflighted its per-element enforcement — added.
//	#3 the setpath (dynamicContainerKind) residual is a dynamic TYPED carrier
//	   when the receiver is tagged (child + elem), like the set residuals.

// #1 — vals retains the tag: a write into (vals m) is enforced at runtime.
func TestValsRetainsTag(t *testing.T) {
	if _, err := tcRun(t, `def m:{:Integer} {a:1 b:2} def xs (vals m) (xs set 0 "bad")`); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("write into (vals m) should reject, got %v", err)
	}
	// The projection carries the element tag.
	out, err := tcRun(t, `def m:{:Integer} {a:1 b:2} def xs (vals m) xs`)
	if err != nil {
		t.Fatalf("vals: %v", err)
	}
	if c, ok := out[len(out)-1].ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("(vals {:Integer}) should be [:Integer] (ok=%v)", ok)
	}
	// Negatives: a conforming write passes; an untyped map's vals carries no tag;
	// keys is never tagged (keys are String, not the value type).
	if _, err := tcRun(t, `def m:{:Integer} {a:1} def xs (vals m) (xs set 0 99)`); err != nil {
		t.Errorf("conforming vals write should pass: %v", err)
	}
	uo, _ := tcRun(t, `def m {a:1 b:2} def xs (vals m) xs`)
	if _, ok := uo[len(uo)-1].ElemConstraint(); ok {
		t.Errorf("(vals {untyped}) should carry no tag")
	}
	ko, _ := tcRun(t, `def m:{:Integer} {a:1} def xs (keys m) xs`)
	if _, ok := ko[len(ko)-1].ElemConstraint(); ok {
		t.Errorf("(keys m) should carry no tag (keys are String)")
	}
}

// #2 — the list-concat append check mirror flags a provably-bad spread element.
func TestAppendListCheckMirror(t *testing.T) {
	r := tcCheck(t, `def f:[:Integer] (flex [1]) append [2 "bad"] f`)
	if !tcHasTypeError(r, "does not conform to element type Integer") {
		t.Errorf("list-append spread of a bad element should be flagged in check: %+v", r.Check.Diagnostics)
	}
	// A conforming spread stays silent.
	r2 := tcCheck(t, `def f:[:Integer] (flex [1]) append [2 3] f`)
	if tcHasTypeError(r2, "does not conform") {
		t.Errorf("conforming list-append should be silent: %+v", r2.Check.Diagnostics)
	}
}

// #2 (runtime) — the runtime enforcement was always present; pin it stays.
func TestAppendListRuntimeEnforcement(t *testing.T) {
	if _, err := tcRun(t, `def f:[:Integer] (flex [1]) append [2 "bad"] f`); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("list-append of a bad element should reject at runtime, got %v", err)
	}
}

// #3 — the setpath residual retains the tag: a write into the result is enforced.
func TestSetpathResidualRetainsTag(t *testing.T) {
	// setReachNative is the setpath core; its result must stay [:Integer].
	xs, r := tcValue(t, `def xs:[:Integer] [1 2 3] xs`)
	out, err := setReachNative(xs, []Value{NewInteger(0)}, NewInteger(9), r)
	if err != nil {
		t.Fatalf("setpath: %v", err)
	}
	if c, ok := out.ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("setpath result lost its [:Integer] tag")
	}
	// The dynamic typed residual (check mirror) carries the element in the child
	// payload AND the elem pointer — for BOTH a list and a map receiver.
	for _, src := range []string{`def xs:[:Integer] [1 2 3] xs`, `def m:{:Integer} {a:1} m`} {
		v, _ := tcValue(t, src)
		res := d2DynamicTypedResidual(v)
		if !IsDynamic(&res) {
			t.Errorf("[%s] setpath residual should stay dynamic", src)
		}
		if ci, cerr := AsChildType(res); cerr != nil || !ci.Child.Equal(TInteger) {
			t.Errorf("[%s] setpath residual child = %v (err %v), want Integer", src, res.Parent(), cerr)
		}
		if e, ok := res.ElemConstraint(); !ok || !e.Equal(TInteger) {
			t.Errorf("[%s] setpath residual lost its elem pointer", src)
		}
	}
	// An untyped receiver keeps the plain dynamic carrier (no tag).
	u, _ := tcValue(t, `def u [1 2 3] u`)
	if _, ok := addrOf(d2DynamicTypedResidual(u)).ElemConstraint(); ok {
		t.Errorf("untyped setpath residual should carry no elem tag")
	}
}
