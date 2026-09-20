package native

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
)

// Typed FLEX nodes (design/TYPED-CONTAINER-TAG-RETENTION.0.md, flex layer).
// `flex` only toggles mutability — it never drops the {:T}/[:T] element
// contract. A typed flex node is constructed-validated, write-enforced (set /
// append / push / unshift / setpath), and mutability-preserving. This suite is
// the flex×typed MATRIX — the interaction grid whose absence let the original
// flex-{:T} panic ship (plain/flex × map/list × construct/read/write ×
// run/check). Enforcement lives on the interpreter path; the compiled path
// falls back to it (`markTypedContainerDefUncompilable`) — the CLI/census cover
// that surface.

// tfRunMode runs src on a fresh registry in run (check=false) or check mode and
// returns the raw error (nil on success). A recovered engine panic surfaces as
// an internal_error, so the no-panic grid asserts on that.
func tfRunMode(t *testing.T, src string, check bool) error {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	r.Check.Mode = check
	toks, perr := parser.Parse(src)
	if perr != nil {
		return perr
	}
	_, rerr := NewTop(r).Run(toks)
	return rerr
}

// --- The no-panic grid: nothing in the matrix may nil-deref (the bug that shipped) ---

func TestTypedFlexNoPanicGrid(t *testing.T) {
	shapes := []string{
		`{:Integer}`, `[:Integer]`, `{:{:Integer}}`, `[:[:Integer]]`,
		`{:(String tor Integer)}`, `{:Any}`,
	}
	// {construct, read, write} bodies, parameterised by the shape S and a flex
	// wrapper (%s for the body expr).
	forms := []string{
		`def m:%s %s m`,                // construct
		`def m:%s %s (m get a/q)`,      // read
		`def m:%s %s (m set a/q 1)`,    // write (map key / list index 'a' is fine to attempt)
		`def m:%s %s def f (flex m) f`, // re-flex
	}
	bodies := map[string]string{
		"plainMap": `{a:1}`, "flexMap": `(flex {a:1})`,
		"plainList": `[1 2 3]`, "flexList": `(flex [1 2 3])`,
	}
	for _, s := range shapes {
		for _, bodyName := range []string{"plainMap", "flexMap", "plainList", "flexList"} {
			body := bodies[bodyName]
			for _, form := range forms {
				src := strings.Replace(strings.Replace(form, "%s", s, 1), "%s", body, 1)
				for _, check := range []bool{false, true} {
					err := tfRunMode(t, src, check)
					if err != nil && (strings.Contains(err.Error(), "internal engine error") ||
						strings.Contains(err.Error(), "invalid memory address")) {
						t.Errorf("PANIC [check=%v] %q: %v", check, src, err)
					}
				}
			}
		}
	}
}

// --- Construction validation: conforming accepted, non-conforming rejected ---

func TestTypedFlexConstruction(t *testing.T) {
	ok := []string{
		`def m:{:Integer} (flex {a:1}) m`,
		`def xs:[:Integer] (flex [1 2 3]) xs`,
		`def m:{:{:Integer}} (flex {a:{x:1}}) m`,
	}
	for _, src := range ok {
		if err := tfRunMode(t, src, false); err != nil {
			t.Errorf("conforming typed flex should construct: %q: %v", src, err)
		}
	}
	bad := []string{
		`def m:{:Integer} (flex {a:"s"}) m`,
		`def xs:[:Integer] (flex [1 "s" 3]) xs`,
	}
	for _, src := range bad {
		err := tfRunMode(t, src, false)
		if err == nil || !strings.Contains(err.Error(), "does not unify with declared type") {
			t.Errorf("non-conforming typed flex should reject: %q: got %v", src, err)
		}
	}
}

// --- Write enforcement across the full flex mutation API ---

func TestTypedFlexWriteEnforcement(t *testing.T) {
	reject := []struct{ src, word string }{
		{`def m:{:Integer} (flex {a:1}) (m set b/q "x")`, "set"},
		{`def xs:[:Integer] (flex [1 2 3]) (xs set 0 "x")`, "set"},
		{`def xs:[:Integer] (flex [1 2 3]) (append "x" xs)`, "append"},
		{`def xs:[:Integer] (flex [1 2 3]) (push "x" xs)`, "push"},
		{`def xs:[:Integer] (flex [1 2 3]) (unshift "x" xs)`, "unshift"},
	}
	for _, tc := range reject {
		err := tfRunMode(t, tc.src, false)
		if err == nil || !strings.Contains(err.Error(), "does not conform to element type Integer") {
			t.Errorf("%s into typed flex should reject: %q: got %v", tc.word, tc.src, err)
		}
	}
	accept := []string{
		`def m:{:Integer} (flex {a:1}) (m set b/q 2)`,
		`def xs:[:Integer] (flex [1 2 3]) (append 9 xs)`,
		`def xs:[:Integer] (flex [1 2 3]) (push 9 xs)`,
	}
	for _, src := range accept {
		if err := tfRunMode(t, src, false); err != nil {
			t.Errorf("conforming write should succeed: %q: %v", src, err)
		}
	}
}

// --- FlexDeepCopy keeps the tag; untyped flex is unenforced ---

func TestTypedFlexTagThroughFlexDeepCopy(t *testing.T) {
	// flex an already-typed map — the tag rides through and still enforces.
	err := tfRunMode(t, `def t:{:Integer} {a:1} def f (flex t) (f set b/q "x") f`, false)
	if err == nil || !strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("flex of a typed map should keep enforcement: got %v", err)
	}
	// direct FlexDeepCopy preserves elem.
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	typed, _ := core.Unify(NewMap(om), core.NewTypedMap(NewTypeLiteral(TInteger)))
	flexed, ferr := core.FlexDeepCopy(typed)
	if ferr != nil {
		t.Fatalf("FlexDeepCopy: %v", ferr)
	}
	if c, okc := flexed.ElemConstraint(); !okc || !c.Equal(TInteger) {
		t.Errorf("FlexDeepCopy dropped the {:Integer} tag (ok=%v)", okc)
	}
	if !core.IsFlexMap(flexed) {
		t.Errorf("FlexDeepCopy result is not a FlexMap")
	}
}

func TestTypedFlexUntypedUnchanged(t *testing.T) {
	for _, src := range []string{
		`def m (flex {a:1}) (m set b/q "x") m`,
		`def xs (flex [1 2 3]) (append "x" xs) xs`,
	} {
		if err := tfRunMode(t, src, false); err != nil {
			t.Errorf("untyped flex write should succeed: %q: %v", src, err)
		}
	}
}

// --- Mutability is preserved: a typed flex is still flex (in-place, shared) ---

func TestTypedFlexPreservesMutability(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	typed, _ := core.Unify(NewMap(om), core.NewTypedMap(NewTypeLiteral(TInteger)))
	flexed, _ := core.FlexDeepCopy(typed)
	if !core.IsFlexMap(flexed) {
		t.Fatalf("typed flex is not a FlexMap")
	}
	// list twin
	lt, _ := core.Unify(NewList([]Value{NewInteger(1)}), core.NewTypedList(NewTypeLiteral(TInteger)))
	fl, _ := core.FlexDeepCopy(lt)
	if !core.IsFlexList(fl) {
		t.Errorf("typed list flex is not a FlexList")
	}
	if c, okc := fl.ElemConstraint(); !okc || !c.Equal(TInteger) {
		t.Errorf("typed flex list lost its tag")
	}
}

// --- Codex round-2: deep setpath / merge / flex check-mirror ---

func TestDeepSetpathEnforcement(t *testing.T) {
	// #1: setReachNative validates the value written at EVERY level against the
	// parent's element type, not just the leaf. (Direct call — the StructUtil
	// module resolver isn't wired into the bare test registry.)
	m, r := tcValue(t, `def m:{:Integer} {a:1} m`)
	// deep path into a scalar-element {:Integer} — the intermediate is a map,
	// which is not an Integer, so even a conforming leaf is rejected.
	if _, err := setReachNative(m, []Value{NewString("b"), NewString("x")}, NewInteger(9), r); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("deep setpath into scalar-element {:Integer} should reject: got %v", err)
	}
	// a nested-typed {:{:Integer}} allows a conforming deep write.
	nm, nr := tcValue(t, `def m:{:{:Integer}} {a:{x:1}} m`)
	if _, err := setReachNative(nm, []Value{NewString("a"), NewString("x")}, NewInteger(9), nr); err != nil {
		t.Errorf("conforming nested-typed deep setpath should succeed: %v", err)
	}
	// a non-conforming leaf into the nested-typed inner is rejected.
	if _, err := setReachNative(nm, []Value{NewString("a"), NewString("x")}, NewString("bad"), nr); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("bad leaf into {:{:Integer}} should reject: got %v", err)
	}
	// untyped is unchanged.
	um, ur := tcValue(t, `def m {a:1} m`)
	if _, err := setReachNative(um, []Value{NewString("b"), NewString("x")}, NewString("ok"), ur); err != nil {
		t.Errorf("untyped deep setpath should succeed: %v", err)
	}
}

func TestMergeTypedEnforcement(t *testing.T) {
	// #2: mergeHandler enforces the element type on a merge INTO a {:T} and
	// retains the tag. (Direct call — no module resolver needed.)
	bad := NewOrderedMap()
	bad.Set("b", NewString("bad"))
	m, r := tcValue(t, `def m:{:Integer} {a:1} m`)
	if _, err := mergeHandler([]Value{m, NewMap(bad)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type") {
		t.Errorf("merge of a bad value into {:Integer} should reject: got %v", err)
	}
	// conforming merge keeps the tag.
	good := NewOrderedMap()
	good.Set("b", NewInteger(2))
	m2, r2 := tcValue(t, `def m:{:Integer} {a:1} m`)
	out, merr := mergeHandler([]Value{m2, NewMap(good)}, nil, nil, r2)
	if merr != nil {
		t.Fatalf("conforming merge: %v", merr)
	}
	if c, ok := out[0].ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("merge result lost the {:Integer} tag")
	}
	// untyped merge unchanged (no tag).
	um, ur := tcValue(t, `def m {a:1} m`)
	uout, uerr := mergeHandler([]Value{um, NewMap(good)}, nil, nil, ur)
	if uerr != nil {
		t.Fatalf("untyped merge: %v", uerr)
	}
	if _, ok := uout[0].ElemConstraint(); ok {
		t.Errorf("untyped merge produced a tagged result")
	}

	// Index-merge into a TYPED LIST enforces the element type too (the
	// mergeListMap / mergeMapList paths + d2ReTagContainer's list arm).
	xs, xr := tcValue(t, `def xs:[:Integer] [1 2 3] xs`)
	badIdx := NewOrderedMap()
	badIdx.Set("0", NewString("bad"))
	if _, err := mergeListMapHandler([]Value{xs, NewMap(badIdx)}, nil, nil, xr); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("list-map index-merge of a bad value into [:Integer] should reject: got %v", err)
	}
	xs2, xr2 := tcValue(t, `def xs:[:Integer] [1 2 3] xs`)
	if _, err := mergeMapListHandler([]Value{NewMap(badIdx), xs2}, nil, nil, xr2); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("map-list index-merge of a bad value into [:Integer] should reject: got %v", err)
	}
	// d2ReTagContainer's default arm: a typed operand but a non-container
	// result is returned unchanged (defensive — a merge never yields a scalar).
	tm, tr := tcValue(t, `def m:{:Integer} {a:1} m`)
	if out, err := d2ReTagContainer(tr, tm, NewInteger(5), "merge"); err != nil || !out.Parent.Equal(TInteger) {
		t.Errorf("d2ReTagContainer over a scalar result = (%v, %v), want the value unchanged", out.Parent, err)
	}
}

func TestFlexWriteCheckMirror(t *testing.T) {
	// #3: flex writes are now flagged in CHECK mode (top-level), mirroring the
	// runtime — set / append / push / unshift.
	for _, src := range []string{
		`def m:{:Integer} (flex {a:1}) (m set b/q "bad")`,
		`def xs:[:Integer] (flex [1]) (append "bad" xs)`,
		`def xs:[:Integer] (flex [1]) (push "bad" xs)`,
		`def xs:[:Integer] (flex [1]) (unshift "bad" xs)`,
	} {
		r := tcCheck(t, src)
		if !tcHasTypeError(r, "does not conform to element type Integer") {
			t.Errorf("flex write should be flagged in check mode: %q: %+v", src, r.Check.Diagnostics)
		}
	}
	// conforming flex write is silent.
	r := tcCheck(t, `def m:{:Integer} (flex {a:1}) (m set b/q 2)`)
	if tcHasTypeError(r, "does not conform") {
		t.Errorf("conforming flex write should be check-silent: %+v", r.Check.Diagnostics)
	}
}

// --- Immutable list mutators enforce + retain the element tag (Codex review #3) ---

func TestImmutableListMutatorEnforcement(t *testing.T) {
	// push/unshift into an IMMUTABLE [:Integer] enforce (the copy-returning
	// grow must uphold the same invariant `set` does — was a soundness hole).
	for _, tc := range []struct{ src, word string }{
		{`def xs:[:Integer] [1 2 3] (push "bad" xs)`, "push"},
		{`def xs:[:Integer] [1 2 3] (unshift "bad" xs)`, "unshift"},
	} {
		if err := tfRunMode(t, tc.src, false); err == nil ||
			!strings.Contains(err.Error(), "does not conform to element type Integer") {
			t.Errorf("%s into [:Integer] should reject: got %v", tc.word, err)
		}
	}
	if err := tfRunMode(t, `def xs:[:Integer] [1 2 3] (push 4 xs)`, false); err != nil {
		t.Errorf("conforming push should succeed: %v", err)
	}
	// pop/shift retain the tag, so a bad grow on the RESULT is still caught.
	for _, src := range []string{
		`def xs:[:Integer] [1 2 3] def ys (pop xs) (push "bad" ys)`,
		`def xs:[:Integer] [1 2 3] def ys (shift xs) (unshift "bad" ys)`,
	} {
		if err := tfRunMode(t, src, false); err == nil ||
			!strings.Contains(err.Error(), "does not conform to element type Integer") {
			t.Errorf("pop/shift should retain the [:Integer] tag: %q: got %v", src, err)
		}
	}
	// Untyped lists are unchanged.
	if err := tfRunMode(t, `def xs [1 2 3] (push "x" xs)`, false); err != nil {
		t.Errorf("untyped push should succeed: %v", err)
	}
}

// --- Nested typed containers re-tag on write (the adversarial-verify hole) ---

func TestTypedContainerNestedRetag(t *testing.T) {
	// Writing an UNTYPED nested container into {:{:T}} / [:[:T]] re-tags it, so a
	// later write into the nested part is enforced. Covers flex + immutable +
	// setpath + the append-spread concat. (Construction tags nested recursively;
	// writes must too.)
	reject := []string{
		// flex nested
		`def m:{:{:Integer}} (flex {a:{x:1}}) (m set b/q (flex {y:2})) (m.b set z/q "bad")`,
		// immutable nested (the committed-R2 hole)
		`def m:{:{:Integer}} {a:{x:1}} def m2 (m set b/q {y:2}) def kid (m2 get b/q) (kid set z/q "bad")`,
		// nested typed list
		`def xs:[:[:Integer]] (flex [[1 2]]) (xs set 0 (flex [9])) def leaf (xs get 0) (leaf set 0 "bad")`,
		// append-spread must enforce each spread element
		`def xs:[:Integer] (flex [1]) (append ["a"] xs)`,
	}
	for _, src := range reject {
		err := tfRunMode(t, src, false)
		if err == nil || !strings.Contains(err.Error(), "does not conform to element type") {
			t.Errorf("nested/spread write should reject: %q: got %v", src, err)
		}
	}
	// A conforming nested write succeeds and the re-tag holds at depth.
	if err := tfRunMode(t, `def m:{:{:Integer}} (flex {a:{x:1}}) (m set b/q (flex {y:2})) (m.b set z/q 7)`, false); err != nil {
		t.Errorf("conforming nested write should succeed: %v", err)
	}
}

// --- node/flex round-trip keeps the tag (both directions only toggle mutability) ---

func TestTypedFlexNodeRoundTrip(t *testing.T) {
	// node (un-flex) a typed flex keeps {:T} → the immutable result still enforces.
	err := tfRunMode(t, `def m:{:Integer} (flex {a:1}) def p (node m) (p set b/q "x") p`, false)
	if err == nil || !strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("node of a typed flex should keep enforcement: got %v", err)
	}
	// direct NodeDeepCopy preserves elem, map and list.
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	tfm, _ := core.Unify(NewMap(om), core.NewTypedMap(NewTypeLiteral(TInteger)))
	fm, _ := core.FlexDeepCopy(tfm)
	nm, nerr := core.NodeDeepCopy(fm)
	if nerr != nil {
		t.Fatalf("NodeDeepCopy map: %v", nerr)
	}
	if c, ok := nm.ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("NodeDeepCopy dropped the {:Integer} map tag")
	}
	tfl, _ := core.Unify(NewList([]Value{NewInteger(1)}), core.NewTypedList(NewTypeLiteral(TInteger)))
	fl, _ := core.FlexDeepCopy(tfl)
	nl, lerr := core.NodeDeepCopy(fl)
	if lerr != nil {
		t.Fatalf("NodeDeepCopy list: %v", lerr)
	}
	if c, ok := nl.ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("NodeDeepCopy dropped the [:Integer] list tag")
	}
}

// --- Compile pass: a {:T} flex def declines (does not compile) ---

func TestTypedFlexDefUncompilable(t *testing.T) {
	// A {:T} constraint over a NON-concrete (flex) body validates + tags at
	// runtime only → the compile pass marks the program uncompilable, so it
	// does not compile (which enforces).
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	if _, cerr := seam5CompileCheck(r, `def m:{:Integer} (flex {a:1}) m`); cerr != nil {
		t.Fatalf("compile-check flex: %v", cerr)
	}
	if r.Check.Emit != nil && r.Check.Emit.Active() {
		t.Errorf("a {:T} def over a flex body must mark the program uncompilable")
	}

	// A CONCRETE typed body is validated statically and stays compilable (the
	// IsConcrete skip in markTypedContainerDefUncompilable).
	r2, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r2)
	if _, cerr := seam5CompileCheck(r2, `def m:{:Integer} {a:1} m`); cerr != nil {
		t.Fatalf("compile-check plain: %v", cerr)
	}
	if r2.Check.Emit == nil || !r2.Check.Emit.Active() {
		t.Errorf("a plain concrete typed def should stay compilable")
	}
}

// --- Check mode does not FALSE-REJECT a conforming typed flex (carrier path) ---

func TestTypedFlexCheckGradual(t *testing.T) {
	for _, src := range []string{
		`def m:{:Integer} (flex {a:1}) m`,
		`def xs:[:Integer] (flex [1 2 3]) xs`,
	} {
		if err := tfRunMode(t, src, true); err != nil {
			t.Errorf("check mode should accept typed flex gradually: %q: %v", src, err)
		}
	}
}
