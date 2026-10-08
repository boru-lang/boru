package lang

import (
	"fmt"
	"testing"
)

// errAt is an error's primary position as row:col, or "" for none.
func errAt(err error) string {
	be, ok := err.(*BoruError)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d:%d", be.Row, be.Col)
}

// TestMain513RecordsClosedOnTheMergedTree pins main's #513 records the merged
// tree answers through this branch's earlier fixes, each with the
// interpreter's own answer — value, error and position alike:
//
//   - NUR220: a stored fn value's unit read a Function-typed param bare as
//     data (`[fn f]` for `[0]`). NUR279's decline hands a fn argument in a
//     slot the unit reads bare to the island.
//   - NUR221: a name def-bound to a `/q`-capturing member fn, read with
//     nothing after it, stayed data (`[0 fn h(Atom)]` for the interpreter's
//     `cannot call`). NUR207's root read dispatches it as the word it is.
//   - NUR224: a fn's return-count error pointed at the body's first token
//     compiled. NUR118 anchors a nested frame's contract error at the call.
func TestMain513RecordsClosedOnTheMergedTree(t *testing.T) {
	const g = `def g fn [[f:Function] [Any] [f]] end def mk fn [[] [Map] [{g: g/v}]] end def m (mk) end def z fn [[] [Integer] [0]] end `
	const h = `def h fn [[x:Atom/q] [Any] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end def z fn [[] [Integer] [0]] end `
	for _, c := range []struct{ src, want, code, at string }{
		{g + `def gg m.g gg z/v`, "[0]", "", ""},
		{g + `g z/v`, "[0]", "", ""},
		{h + `def r m.f z r`, "[]", "signature_error", "1:132"},
		{h + `def r m.f z`, "[0]", "", ""},
		{`def f fn [[][Integer][1 2]] end f`, "[]", "type_error", "1:33"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled: %v", c.src, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want || codeOf(errC) != c.code || errAt(errC) != c.at {
			t.Errorf("%q: compiled %v [%s] at %q, want %s [%s] at %q", c.src, gotC, codeOf(errC), errAt(errC), c.want, c.code, c.at)
		}
	}
	// Negative: the bare word before the Function-typed slot CALLS (NUR078),
	// so `m.g z` is the named no-match on both lanes, not a reference. (Its
	// caret is NUR219's open sibling: the landing's raise points at the
	// landing, the interpreter's at the value's own token.)
	gotC, _, errC, gotI, errI := runBothEngines(t, g+`m.g z`)
	if codeOf(errC) != "uncalled_function" || codeOf(errI) != codeOf(errC) || detailOf(errC) != detailOf(errI) || len(gotC) != 0 || len(gotI) != 0 {
		t.Errorf("m.g z: the named no-match on both lanes, got compiled %v %v, interp %v %v", gotC, errC, gotI, errI)
	}
}

// TestApplyKernelFrameAnchorsAtTheValue pins NUR274: a fn value applied
// through the Apply kernel anchors its contract error where the
// interpreter's return check does — at the value it re-stepped at the
// pointer. A member read hands the stored value on unmoved, so the caret is
// the value's own token (`h/v`, 1:69); a NAME read re-positions it to the
// read, which the op's named head carries (the param `g` inside `ap`, 1:125).
// The call anchor NUR118 gave every nested frame blamed the member read's
// op (1:95) instead, where main blamed the value.
func TestApplyKernelFrameAnchorsAtTheValue(t *testing.T) {
	const h = `def h fn [[x:Integer] [Integer] ['s']] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	for _, c := range []struct{ src, at string }{
		{h + `m.f 5`, "1:69"},
		{h + `5 m.f`, "1:69"},
		{h + `(m.f 5)`, "1:69"},
		{h + `m get 'f' 5`, "1:69"},
		{h + `def ap fn [[g:Function][Any][(g 5)]] end ap m.f`, "1:125"},
		{h + `def g h/v end g 5`, "1:109"},
		{h + `h/v apply 5`, "1:95"},
		// The landing's `/q` claim enters through the same kernel.
		{`def h fn [[x:Atom/q] [Integer] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end def z fn [[] [Integer] [0]] end m.f z`, "1:66"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled: %v", c.src, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if codeOf(errC) != "type_error" || errAt(errC) != c.at {
			t.Errorf("%q: the contract error at %s, got [%s] at %q", c.src, c.at, codeOf(errC), errAt(errC))
		}
	}
}

// TestRootDefBoundMemberFnContractAgrees pins NUR275's close: a member fn
// value DEF-BOUND at the root and applied by its NAME is the interpreter's
// word dispatch of the binding — `def g <fn value>` renames the value to g
// (installFnDef) and the read's return check sits at the read's token — so
// the compiled root's leading apply seats the binding as a named head (its
// name, the read's position; compiler Finalize over rootResidualReads) and
// the entered frame's contract takes the head's name (eng
// headNamedContract). Both lanes report `g: …` at the read, where the
// compiled lane named the value's own `h` at its `h/v` token.
func TestRootDefBoundMemberFnContractAgrees(t *testing.T) {
	const h = `def h fn [[x:Integer] [Integer] ['s']] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	const h2 = `def h fn [[x:Integer] [Integer Integer] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	const h1 = `def h fn [[x:Integer] [Integer] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	for _, c := range []struct{ src, code, at string }{
		{h + `def g m.f end g 5`, "type_error", "1:109"},
		{h + `def g (m.f) end g 5`, "type_error", "1:111"},
		{h2 + `def g m.f end g 5`, "type_error", ""},
		{h1 + `def g m.f end g 'x'`, "signature_error", ""},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || len(gotC) != 0 || len(gotI) != 0 || codeOf(errI) != c.code || codeOf(errC) != c.code {
			t.Errorf("%q: %s on both lanes, got compiled=%v %v %v, interp %v %v", c.src, c.code, compiled, gotC, errC, gotI, errI)
			continue
		}
		if detailOf(errC) != detailOf(errI) || errAt(errC) != errAt(errI) || (c.at != "" && errAt(errI) != c.at) {
			t.Errorf("%q: interp %q at %q, compiled %q at %q", c.src, detailOf(errI), errAt(errI), detailOf(errC), errAt(errC))
		}
	}
	// A matching window answers on both lanes, and a 0-arg value fires.
	agreeOnBothLanes(t, h1+`def g m.f end g 5`, "[5]")
	agreeOnBothLanes(t, `def g0 fn [[][Integer][7]] end def mk fn [[] [Map] [{f: g0/v}]] end def m (mk) end def g m.f end g 5`, "[7 5]")
}
