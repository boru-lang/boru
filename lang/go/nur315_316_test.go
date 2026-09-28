package lang

import "testing"

// TestNUR315FlexWrittenThroughAParam pins NUR315. The check pass joins a
// flex container's element types over the writes it sees on the container,
// and a user fn it is passed to may write through its parameter's alias,
// which the body analysis (over a plain carrier) does not see: `poke fl drop
// def j (fl get 0) j`, where poke stores a fn at 0, read the stale Integer
// claim and answered `fn j` for the interpreter's 42. Passing a shaped flex
// container to a user fn poisons its claims (PoisonFlexShapes).
func TestNUR315FlexWrittenThroughAParam(t *testing.T) {
	const h = `def h fn [[] [Integer] [42]] `
	for _, c := range []struct{ src, want string }{
		{h + `def poke fn [[l:FlexList] [Any] [set 0 h/v l]] def fl (flex [1 2 3]) poke fl drop def j (fl get 0) j`, "[42]"},
		{h + `def pokem fn [[m:FlexMap] [Any] [set a/q h/v m]] def fm (flex {a: 1}) pokem fm drop def j (fm get "a") j`, "[42]"},
		// The write on the container's own name was always seen.
		{h + `def fl (flex [1 2 3]) set 0 h/v fl drop def j (fl get 0) j`, "[42]"},
		// A reader alone changes nothing the run can observe.
		{`def peek fn [[l:FlexList] [Any] [l get 0]] def fl (flex [1 2 3]) peek fl drop (fl get 0) add 1`, "[2]"},
		{`def poke fn [[l:FlexList] [Any] [set 0 "s" l]] def fl (flex [1 2 3]) poke fl drop (fl get 0) add "t"`, "[st]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR316GradualElementsStayGradual pins NUR316. `each`'s check-mode
// result typed its elements by the body value's type alone, dropping its
// gradual mark, and the closure compiler read a typed list's element the
// same way: a later `each [add 1]` committed a direct add over an element
// the run did not hold — a String ([[1]] for [['s1']]) or the None an
// out-of-range read gives ([[1]] for a signature_error). A gradual element
// stays gradual through the typed list (core.CarrierTypedListOf,
// check.ElementCarrierOf), so the body re-matches at run time.
func TestNUR316GradualElementsStayGradual(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def poke fn [[l:FlexList] [Any] [set 0 "s" l]] def fl (flex [1 2 3]) poke fl drop def ys ([0] each [drop (fl get 0)]) ys each [add 1]`, "[['s1']]"},
		{`def f fn [[xs:[:Integer]] [Any] [ def ys ([0] each [drop (xs get 0)]) ys each [add 1] ]] f [1 2 3]`, "[[2]]"},
		{`def f fn [[xs:[:Integer]] [Any] [ xs each [add 1] ]] f [1 2 3]`, "[[2 3 4]]"},
		{`[1 2 3] each [add 1] each [add 1]`, "[[3 4 5]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// An out-of-range read's None reaches add on the run: the interpreter's
	// signature_error, and the compiled poly re-match's loud defer.
	src := `def f fn [[xs:[:Integer]] [Any] [ def ys ([0] each [drop (xs get 5)]) ys each [add 1] ]] f [1 2 3]`
	gotC, _, errC := mustNew(t).RunCompiled(src)
	if !isBailDefect(errC) || len(gotC) != 0 {
		t.Errorf("%q: want the loud poly defer, got %v / %v", src, gotC, errC)
	}
}
