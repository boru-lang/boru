package native

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
)

// Typed-container tag retention + write enforcement
// (design/TYPED-CONTAINER-TAG-RETENTION.0.md). A concrete {:T} map / [:T]
// list RETAINS its element constraint (Value.elem) while staying concrete,
// so a non-conforming write is a type_error at BOTH runtime and check, and
// a conforming write keeps the tag (so chained writes stay enforced).

func tcRun(t *testing.T, src string) ([]Value, error) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	toks, perr := parser.Parse(src)
	if perr != nil {
		t.Fatalf("parse %q: %v", src, perr)
	}
	return NewTop(r).Run(toks)
}

func tcCheck(t *testing.T, src string) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	r.Check.Mode = true
	toks, perr := parser.Parse(src)
	if perr != nil {
		t.Fatalf("parse %q: %v", src, perr)
	}
	if _, rerr := NewTop(r).Run(toks); rerr != nil {
		t.Fatalf("check run %q: %v", src, rerr)
	}
	return r
}

func tcHasTypeError(r *Registry, substr string) bool {
	for _, d := range r.Check.Diagnostics {
		if d.Code == "type_error" && strings.Contains(d.Detail, substr) {
			return true
		}
	}
	return false
}

// --- R1: construction retains the exact element constraint ---

func TestTypedContainerRetainsElem(t *testing.T) {
	cases := []struct {
		src  string
		want *Type
	}{
		{`def m:{:Integer} {a:1} m`, TInteger},
		{`def m:{:Boolean} {a:true} m`, TBoolean},
		{`def m:{:String} {a:"x"} m`, TString},
		{`def xs:[:Integer] [1 2 3] xs`, TInteger},
	}
	for _, tc := range cases {
		out, err := tcRun(t, tc.src)
		if err != nil {
			t.Fatalf("[%s] run: %v", tc.src, err)
		}
		v := out[len(out)-1]
		if !core.IsConcrete(&v) {
			t.Errorf("[%s] retained value is not concrete", tc.src)
		}
		c, ok := v.ElemConstraint()
		if !ok {
			t.Fatalf("[%s] no element constraint retained", tc.src)
		}
		if !c.Equal(tc.want) {
			t.Errorf("[%s] elem = %s, want %s", tc.src, c.Parent.String(), tc.want.String())
		}
	}
}

// An untyped container carries NO tag — the negative case.
func TestUntypedContainerNoElem(t *testing.T) {
	for _, src := range []string{`{a:1}`, `[1 2 3]`, `def m:{:Any} {a:1} m`} {
		out, err := tcRun(t, src)
		if err != nil {
			t.Fatalf("[%s] run: %v", src, err)
		}
		if _, ok := out[len(out)-1].ElemConstraint(); ok {
			t.Errorf("[%s] unexpectedly carries an element tag", src)
		}
	}
}

// --- R2: runtime write enforcement ---

func TestTypedMapWriteRejectRuntime(t *testing.T) {
	for _, src := range []string{
		`def m:{:Integer} {a:1} (m set b/q "wrong")`,
		`def m:{:Integer} {a:1} (m set "b" "wrong")`,
		`def m:{:Integer} {a:1} (m set b/q 2.5)`, // Float rejected — exact element type
		`def m:{:String} {a:"x"} (m set b/q 9)`,
	} {
		_, err := tcRun(t, src)
		if err == nil {
			t.Errorf("[%s] expected a runtime type_error, got none", src)
			continue
		}
		if !strings.Contains(err.Error(), "does not conform to element type") {
			t.Errorf("[%s] wrong error: %v", src, err)
		}
	}
}

func TestTypedListWriteRejectRuntime(t *testing.T) {
	_, err := tcRun(t, `def xs:[:Integer] [1 2 3] (xs set 0 "wrong")`)
	if err == nil || !strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Fatalf("expected [:Integer] write reject, got %v", err)
	}
}

func TestTypedContainerWriteConformingRuntime(t *testing.T) {
	// Conforming writes succeed AND the result keeps the tag.
	out, err := tcRun(t, `def m:{:Integer} {a:1} (m set b/q 2)`)
	if err != nil {
		t.Fatalf("conforming write: %v", err)
	}
	res := out[len(out)-1]
	c, ok := res.ElemConstraint()
	if !ok || !c.Equal(TInteger) {
		t.Errorf("conforming set result lost its {:Integer} tag (ok=%v)", ok)
	}
	out2, err2 := tcRun(t, `def xs:[:Integer] [1 2 3] (xs set 0 9)`)
	if err2 != nil {
		t.Fatalf("conforming list write: %v", err2)
	}
	if c2, ok2 := out2[len(out2)-1].ElemConstraint(); !ok2 || !c2.Equal(TInteger) {
		t.Errorf("conforming list set result lost its [:Integer] tag")
	}
}

// A chained write is enforced because the inner set's result keeps the tag.
func TestTypedMapChainedWriteRejectRuntime(t *testing.T) {
	_, err := tcRun(t, `def m:{:Integer} {a:1} ((m set b/q 2) set c/q "x")`)
	if err == nil || !strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Fatalf("expected chained write reject, got %v", err)
	}
}

// An untyped map is unchanged — heterogeneous writes are accepted.
func TestUntypedMapWriteAllowedRuntime(t *testing.T) {
	out, err := tcRun(t, `def m {a:1} (m set b/q "wrong")`)
	if err != nil {
		t.Fatalf("untyped write should succeed: %v", err)
	}
	if _, ok := out[len(out)-1].ElemConstraint(); ok {
		t.Errorf("untyped map write produced a tagged result")
	}
}

// --- R2: check-mode enforcement (the RuntimeMirror) ---

func TestTypedMapWriteRejectCheck(t *testing.T) {
	r := tcCheck(t, `def m:{:Integer} {a:1} (m set b/q "wrong")`)
	if !tcHasTypeError(r, "does not conform to element type Integer") {
		t.Errorf("expected a check type_error, got %+v", r.Check.Diagnostics)
	}
}

func TestTypedListWriteRejectCheck(t *testing.T) {
	r := tcCheck(t, `def xs:[:Integer] [1 2 3] (xs set 0 "wrong")`)
	if !tcHasTypeError(r, "does not conform to element type Integer") {
		t.Errorf("expected a check type_error for [:T], got %+v", r.Check.Diagnostics)
	}
}

// A non-conforming BODY value fails construction outright — the unify origin's
// error path (the tag is never minted on a rejected container).
func TestTypedContainerConstructionRejects(t *testing.T) {
	for _, src := range []string{
		`def m:{:Integer} {a:"s"}`,  // map element mismatch
		`def xs:[:Integer] [1 "s"]`, // list element mismatch
	} {
		_, err := tcRun(t, src)
		if err == nil || !strings.Contains(err.Error(), "does not unify with declared type") {
			t.Errorf("[%s] expected construction type error, got %v", src, err)
		}
	}
}

// A write INSIDE a fn body is enforced at runtime but the checker stays
// conservative (atUncaughtTopLevel is false) — no check diagnostic here.
func TestTypedContainerWriteBodyCheckConservative(t *testing.T) {
	r := tcCheck(t, `def probe fn [[m:{:Integer}] [Map] [(m set b/q "wrong")]] def t:{:Integer} {a:1} (probe t)`)
	if tcHasTypeError(r, "does not conform to element type") {
		t.Errorf("body write should be check-conservative, got %+v", r.Check.Diagnostics)
	}
}

// --- R3: setpath deep-write enforcement ---

// tcValue runs src and returns its final value (a helper for R3 setpath tests).
func tcValue(t *testing.T, src string) (Value, *Registry) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	out, rerr := tcRun(t, src)
	if rerr != nil {
		t.Fatalf("[%s] run: %v", src, rerr)
	}
	return out[len(out)-1], r
}

func TestSetpathTypedWriteEnforcement(t *testing.T) {
	m, r := tcValue(t, `def m:{:Integer} {a:1} m`)

	// Shallow non-conforming → runtime type_error.
	if _, err := setReachNative(m, []Value{NewString("b")}, NewString("wrong"), r); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("shallow non-conforming setpath: got %v, want conformance error", err)
	}
	// Shallow conforming → ok, result keeps the tag.
	out, err := setReachNative(m, []Value{NewString("b")}, NewInteger(9), r)
	if err != nil {
		t.Fatalf("shallow conforming setpath: %v", err)
	}
	if c, ok := out.ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("setpath result lost its {:Integer} tag")
	}

	// Nested {:{:Integer}}: a deep non-conforming write is rejected because the
	// inner container carries the recursively-retained tag.
	nm, nr := tcValue(t, `def m:{:{:Integer}} {a:{x:1}} m`)
	if _, err := setReachNative(nm, []Value{NewString("a"), NewString("x")}, NewString("wrong"), nr); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("nested non-conforming setpath: got %v, want conformance error", err)
	}
	// Nested conforming → ok.
	if _, err := setReachNative(nm, []Value{NewString("a"), NewString("x")}, NewInteger(42), nr); err != nil {
		t.Errorf("nested conforming setpath: %v", err)
	}
}

func TestSetpathTypedListWrite(t *testing.T) {
	xs, r := tcValue(t, `def xs:[:Integer] [1 2 3] xs`)
	if _, err := setReachNative(xs, []Value{NewInteger(0)}, NewString("wrong"), r); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("[:Integer] setpath reject: got %v", err)
	}
	out, err := setReachNative(xs, []Value{NewInteger(0)}, NewInteger(9), r)
	if err != nil {
		t.Fatalf("conforming list setpath: %v", err)
	}
	if c, ok := out.ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("list setpath result lost its [:Integer] tag")
	}
}

// An untyped container's setpath is unchanged.
func TestSetpathUntypedUnenforced(t *testing.T) {
	m, r := tcValue(t, `def m {a:1} m`)
	if _, err := setReachNative(m, []Value{NewString("b")}, NewString("wrong"), r); err != nil {
		t.Errorf("untyped setpath should succeed: %v", err)
	}
}

// setpathReturns flags a shallow non-conforming write in check mode; a deep path
// or a conforming write stays silent.
func TestSetpathReturnsCheck(t *testing.T) {
	mk := func() *Registry {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		r.Check.Mode = true
		return r
	}
	tm, _ := tcValue(t, `def m:{:Integer} {a:1} m`)

	// Shallow non-conforming (data, "path", newVal) → diagnostic.
	r := mk()
	setpathReturns([]Value{tm, NewString("b"), NewString("wrong")}, r)
	if !tcHasTypeError(r, "does not conform to element type Integer") {
		t.Errorf("shallow non-conforming setpathReturns: expected diagnostic, got %+v", r.Check.Diagnostics)
	}
	// Deep path → conservative (runtime-only).
	r = mk()
	setpathReturns([]Value{tm, NewString("a.x"), NewString("wrong")}, r)
	if tcHasTypeError(r, "does not conform") {
		t.Errorf("deep path should be check-conservative, got %+v", r.Check.Diagnostics)
	}
	// Conforming → silent.
	r = mk()
	setpathReturns([]Value{tm, NewString("b"), NewInteger(9)}, r)
	if tcHasTypeError(r, "does not conform") {
		t.Errorf("conforming setpathReturns should be silent, got %+v", r.Check.Diagnostics)
	}
}

// d2TypedContainerBound (Part B read narrowing) — its edge branches are not
// reachable through ordinary dispatch (the accessor callers gate on
// IsTypedMap/IsTypedList, and the disjunct-child read currently ends dynamic
// Any before reaching here), so pin them by direct call, the same discipline
// w3HandlerErr uses for dispatch-unreachable arms.
func TestD2TypedContainerBoundBranches(t *testing.T) {
	// Non-container → AsChildType errors → no bound.
	if _, ok := d2TypedContainerBound(NewInteger(5)); ok {
		t.Errorf("d2TypedContainerBound(Integer) should not produce a bound")
	}
	// Disjunct child → the bound preserves the disjunct verbatim.
	disj := core.NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TInteger)})
	b, ok := d2TypedContainerBound(core.NewTypedMap(disj))
	if !ok || !core.IsDisjunct(&b) {
		t.Errorf("disjunct-typed-map bound = (%v, %v), want a disjunct carrier", b.Parent, ok)
	}
	// Single-type child → a dynamic carrier. It binds child.Parent (the
	// element's SUPERTYPE — Integer→Number); the exact-type narrowing (R4) is
	// held back because it diverges the compiled/interpreted differential (see
	// d2TypedContainerBound). R2's write-check is exact regardless (it unifies
	// against the child value directly).
	b2, ok2 := d2TypedContainerBound(core.NewTypedMap(NewTypeLiteral(TInteger)))
	if !ok2 || core.IsConcrete(&b2) {
		t.Errorf("integer-typed-map bound = (%v, %v), want a dynamic carrier", b2.Parent, ok2)
	}
}

// The set residual retains the element tag, so a CHAINED / BOUND write is
// caught in check mode too (mirrors the runtime — Codex review #2).
func TestChainedWriteCheckMode(t *testing.T) {
	r := tcCheck(t, `def m:{:Integer} {a:1} ((m set b/q 2) set c/q "wrong")`)
	if !tcHasTypeError(r, "does not conform to element type Integer") {
		t.Errorf("chained check-mode write should be flagged: %+v", r.Check.Diagnostics)
	}
	r2 := tcCheck(t, `def m:{:Integer} {a:1} def m2 (m set b/q 2) (m2 set c/q "wrong")`)
	if !tcHasTypeError(r2, "does not conform to element type Integer") {
		t.Errorf("bound-then-write check should be flagged: %+v", r2.Check.Diagnostics)
	}
	// A conforming chained write stays silent.
	r3 := tcCheck(t, `def m:{:Integer} {a:1} ((m set b/q 2) set c/q 3)`)
	if tcHasTypeError(r3, "does not conform") {
		t.Errorf("conforming chained write should be silent: %+v", r3.Check.Diagnostics)
	}
}

// A conforming write is silent; an untyped map write is silent.
func TestTypedContainerWriteCheckNegatives(t *testing.T) {
	for _, src := range []string{
		`def m:{:Integer} {a:1} (m set b/q 2)`, // conforming
		`def m {a:1} (m set b/q "wrong")`,      // untyped
		// non-concrete written value (a gradual fn result) — can't be
		// statically judged, so the check stays silent (runtime re-checks).
		`def f fn [[] [Any] [1]] def m:{:Integer} {a:1} (m set b/q (f))`,
	} {
		r := tcCheck(t, src)
		if tcHasTypeError(r, "does not conform to element type") {
			t.Errorf("[%s] unexpected check type_error: %+v", src, r.Check.Diagnostics)
		}
	}
}
