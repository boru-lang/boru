package core

import "testing"

// A driven region's inputs and a call frame's unnamed arguments are INERT:
// resolved where the call was made, they enter as stack data and are never
// stepped. That holds wherever the loop or the call runs — at the top
// level, in a bare group, and in a group a waiting word evaluates as its
// argument. The last of those has a token loop of its own
// (evalParenGroupAt), and until it skipped the inert span an open paren
// carries, a Function among the inputs was CALLED on what the body wrote
// after it and a `/v` word among them was resolved (NUR397).
func TestInertInputsStayInertInAnEvaluatedGroup(t *testing.T) {
	for _, c := range []struct{ outside, inside, want string }{
		// a function element is data; the body's 5 stands beside it
		{
			"run eachq [q fn( n Integer Integer [ addq n 1 ] ) ] [q 5 ]",
			"run bothq 1 ( eachq [q fn( n Integer Integer [ addq n 1 ] ) ] [q 5 ] )",
			"[fn [[n:Integer][Integer][addq n 1]] 5]",
		},
		// every element of several, not only the first
		{
			"run eachq [q fn( n Any Any [ 9 ] ) 7 ] [q 'a' ]",
			"run bothq 1 ( eachq [q fn( n Any Any [ 9 ] ) 7 ] [q 'a' ] )",
			"[fn [[n:Any][Any][9]] 'a' 7 'a']",
		},
		// a `/v` word element is the word it was written as
		{
			"run eachq [q negq/v ] [q 4 ]",
			"run bothq 1 ( eachq [q negq/v ] [q 4 ] )",
			"[negq/v 4]",
		},
		// a body that fails beside a function input fails as itself: the
		// function is not what is refused
		{
			"run eachq [q fn( n Integer Integer [ n ] ) ] [q addq 1 ]",
			"run negq ( eachq [q fn( n Integer Integer [ n ] ) ] [q addq 1 ] )",
			"ERROR:signature_error",
		},
	} {
		if got := evalCoreSpec(t, coreSpecRegistry(t), c.outside); got != c.want {
			t.Errorf("%s\n   got  %s\n   want %s", c.outside, got, c.want)
		}
		if got := evalCoreSpec(t, coreSpecRegistry(t), c.inside); got != c.want {
			t.Errorf("%s\n   got  %s\n   want %s", c.inside, got, c.want)
		}
	}
}

// Skipping the span skips ONLY the inputs: the body after them is stepped
// as it always was, so an element the body does call is called, in a group
// as out of one.
func TestAnEvaluatedGroupStillStepsTheBody(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		{"run bothq 1 ( eachq [q 1 2 ] [q negq ] )", "[-1 -2]"},
		{"run eachq [q 1 2 ] [q negq ]", "[-1 -2]"},
		// the function standing in the BODY is the one that dispatches
		{"run bothq 1 ( eachq [q 5 ] [q fn( n Integer Integer [ addq n 1 ] ) ] )", "[6]"},
		{"run eachq [q 5 ] [q fn( n Integer Integer [ addq n 1 ] ) ]", "[6]"},
	} {
		if got := evalCoreSpec(t, coreSpecRegistry(t), c.expr); got != c.want {
			t.Errorf("%s\n   got  %s\n   want %s", c.expr, got, c.want)
		}
	}
}

// The same rule for a CALL FRAME: an unnamed argument is spliced at the
// frame's head and skipped by the open paren that carries its span. A
// function passed that way is data while the body runs — it is the frame's
// RESULT that is stepped again, where the call stood. The body here doubles
// what is on top of the stack: with the argument inert that is the body's
// own 5, and the function then takes the first of the two; had the argument
// been stepped in the frame it would have taken the 5 itself and the body
// would have doubled its answer.
func TestUnnamedArgumentsStayInertInAnEvaluatedGroup(t *testing.T) {
	build := func() *Registry {
		r := coreSpecRegistry(t)
		InstallFnDef(r, "keepq", FnDefInfo{Signatures: []Signature{{
			Params:     []FnParam{{Type: TAny}},
			Impl:       Boru([]Value{NewInteger(5), NewWordModified("pairq", -1, true, false)}),
			BarrierPos: 1,
		}}})
		return r
	}
	for _, c := range []struct{ expr, want string }{
		{"run keepq fn( n Integer Integer [ addq n 1 ] )", "6 5"},
		{"run ( keepq fn( n Integer Integer [ addq n 1 ] ) )", "6 5"},
		// inside a group a waiting word evaluates: the word takes the 1
		// and the first of the group's two results
		{"run bothq 1 ( keepq fn( n Integer Integer [ addq n 1 ] ) )", "6 5"},
	} {
		if got := evalCoreSpec(t, build(), c.expr); got != c.want {
			t.Errorf("%s\n   got  %s\n   want %s", c.expr, got, c.want)
		}
	}
}
