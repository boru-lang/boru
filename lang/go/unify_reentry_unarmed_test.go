package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The registry of an armed unify (`is`, `unify`, a typed def, `make`)
// travels explicitly down the kernel's unify chain and NOWHERE else
// (core/go/unify.go, UnifyExplainR). In particular it does not leak into
// the ENGINE when the chain re-enters it: a predicate body run by the
// predicate's Unifier dispatches its own calls exactly as the same call
// dispatches at top level — unarmed. So a fn whose parameter pattern is a
// typed container with a predicate child (`xs:[:Pos]`, `m:{a:Pos}`) is
// refused for a concrete argument inside a predicate body precisely
// because it is refused at top level, and the outer `is` / `unify`
// therefore answers false / ~unify-fail.
//
// Before the registry was threaded explicitly, a package-global stack made
// that in-body dispatch armed BY ACCIDENT: the verdict of `[1 2] is Wrap`
// depended on whether some UnifyExplainR — on any goroutine — happened to
// be in flight, while the identical top-level call `(chk [1 2])` was
// refused. This pins the consistent rule on both engines.
func TestPredicateBodyDispatchIsUnarmedLikeTopLevel(t *testing.T) {
	const defs = "def Pos fnpred n:Integer [n gt 0]  " +
		"def chk fn [[xs:[:Pos]] [Boolean] [true]]  " +
		"def Wrap fnpred xs:List [(chk xs)]  "
	const mapDefs = "def Pos fnpred n:Integer [n gt 0]  " +
		"def chk fn [[m:{a:Pos}] [Boolean] [true]]  " +
		"def Wrap fnpred m:Map [chk m]  "
	rows := []struct{ src, want string }{
		{defs + "[1 2] is Wrap", "[false]"},
		{defs + "([1 2] unify Wrap)", "[~unify-fail false]"},
		{defs + "[1 -2] is Wrap", "[false]"},
		{defs + "[[1 2] [3]] is [:Wrap]", "[false]"},
		{mapDefs + "{a:5} is Wrap", "[false]"},
		{mapDefs + "{a:-5} is Wrap", "[false]"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || errC != nil || errI != nil {
			t.Fatalf("%q: compiled=%v errC=%v errI=%v", c.src, compiled, errC, errI)
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q = %v, want %s", c.src, gotC, c.want)
		}
	}
	// The top-level call the body makes is refused the same way — that
	// is the rule the in-body verdicts above follow.
	top := "def Pos fnpred n:Integer [n gt 0]  def chk fn [[xs:[:Pos]] [Boolean] [true]]  (chk [1 2])"
	_, _, errC, _, errI := runBothEngines(t, top)
	for _, err := range []error{errC, errI} {
		if err == nil || !strings.Contains(err.Error(), "no signature matches the arguments") {
			t.Fatalf("%q: top-level dispatch must be refused, got %v", top, err)
		}
	}
}
