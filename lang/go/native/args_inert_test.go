package native

import "testing"

// Pins the ARGUMENTS-ARE-INERT invariant (design/ARG-SEMANTICS-
// UNIFICATION.0.md): an argument never acts on its own — binding style
// (named vs unnamed) changes only how the body refers to it. Before the
// unification, an unnamed Function arg spliced onto the body frame was
// re-STEPPED and auto-dispatched against whatever sibling value happened
// to sit beside it (sibling-dependent firing), while the same argument
// bound to a name stayed data until referenced. The boru-source twins live
// in lang/spec/module-fnvalue-boundary.tsv; these tests pin the kernel
// mechanics the rows can't reach: the frame ArgSpan skip, the sub-engine
// start offset, and the flow-unwind that keeps the per-call stacks
// balanced when break/continue escape a live frame.

// defineFn defines a boru fn via source tokens and returns nothing; the
// fn dispatches by name afterwards.
func defineFn(t *testing.T, r *Registry, name string, fnBody Value) {
	t.Helper()
	_ = runBoru(t, r, []Value{NewWord("def"), NewWord(name), NewWord("fn"), fnBody, NewEnd()})
}

// unnamedFnBody builds `fn [[<types>] [<ret>] [body...]]` with UNNAMED
// params given as bare type words.
func unnamedFnBody(types []string, ret string, body ...Value) Value {
	params := make([]Value, len(types))
	for i, ty := range types {
		params[i] = NewWord(ty)
	}
	return NewList([]Value{
		NewList(params),
		NewList([]Value{NewWord(ret)}),
		NewList(body),
	})
}

// TestUnnamedFnArgInertViaValue builds the call with the resolved fn
// VALUE — the shape the boundary rows reach via export maps. An unnamed
// [Function Integer] param pair with an empty body used to auto-apply
// the fn to its sibling on frame placement (result 42); inert arguments
// return the untouched 14 with the unconsumed fn value trimmed.
func TestUnnamedFnArgInertViaValue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defineFn(t, r, "mkai2", typedFnBody("x", "Integer", "Integer",
		NewWord("x"), NewWord("mul"), NewInteger(3)))
	bound, ok := r.Defs.Top("mkai2")
	if !ok {
		t.Fatal("mkai2 not bound")
	}
	// Anonymous copy: a lone anonymous fn value is data by rule (the
	// export-map shape), so it survives the paren group as the argument.
	fd := *DataOf(&bound).(*FnDefInfo)
	fd.Name = ""
	fd.Anonymous = true
	fnVal := NewFunction(fd)
	defineFn(t, r, "useai2", unnamedFnBody([]string{"Function", "Integer"}, "Integer"))

	// The paren group resolves the fn value first so forward collection
	// takes it as the Function arg — the token twin of source `(mk/v)`.
	res := runBoru(t, r, []Value{NewWord("useai2"), NewOpenParen(), fnVal, NewCloseParen(), NewInteger(14), NewEnd()})
	if len(res) != 1 {
		t.Fatalf("residual = %v, want one value", res)
	}
	if n, _ := AsInteger(res[0]); n != 14 {
		t.Fatalf("unnamed fn arg interacted with its sibling: got %d, want 14 (42 means the argument auto-fired)", n)
	}

	// Negative twin: the SAME argument applied EXPLICITLY fires exactly
	// once — inertness must not break application.
	defineFn(t, r, "apai2", NewList([]Value{
		NewList([]Value{NewWord("Function"), NewWord("Integer")}),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewOpenParen(), NewWord("args"), NewWord("dot"), NewInteger(0), NewWord("args"), NewWord("dot"), NewInteger(1), NewCloseParen()}),
	}))
	res = runBoru(t, r, []Value{NewWord("apai2"), NewOpenParen(), fnVal, NewCloseParen(), NewInteger(14), NewEnd()})
	if n, _ := AsInteger(res[len(res)-1]); n != 42 {
		t.Fatalf("explicit apply through args.N = %d, want 42 (a single dispatch)", n)
	}
}

func TestLoneUnnamedFnArgStaysData(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defineFn(t, r, "mkai3", typedFnBody("x", "Integer", "Integer",
		NewWord("x"), NewWord("mul"), NewInteger(3)))
	bound, _ := r.Defs.Top("mkai3")
	fd := *DataOf(&bound).(*FnDefInfo)
	fd.Name = ""
	fd.Anonymous = true
	fnVal := NewFunction(fd)
	defineFn(t, r, "keepai", unnamedFnBody([]string{"Function"}, "Function"))

	res := runBoru(t, r, []Value{NewWord("keepai"), NewOpenParen(), fnVal, NewCloseParen(), NewEnd()})
	if len(res) != 1 {
		t.Fatalf("residual = %v, want the held fn", res)
	}
	if _, ok := DataOf(&res[0]).(*FnDefInfo); !ok {
		t.Fatalf("lone unnamed fn arg did not survive as data: got %s", ParentOf(&res[0]).String())
	}
}

func TestFlowUnwindKeepsPerCallStacksBalanced(t *testing.T) {
	// A break/continue escaping a spliced fn frame discards the frame's
	// cleanup tail; the flow resolvers must unwind it (fn_frame.go::
	// unwindLiveFrames) or the per-call Args list, FnBaseline, and param
	// bindings leak — observed as the CALLER's args.N reading the dead
	// callee's args in later iterations. Pin the balance directly.
	for _, tc := range []struct {
		name string
		flow string
	}{
		{"break", "break"},
		{"continue", "continue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			defineFn(t, r, "flw", typedFnBody("x", "Integer", "Any", NewWord(tc.flow)))
			res := runBoru(t, r, []Value{
				NewWord("def"), NewWord("accfw"), NewInteger(0), NewEnd(),
				NewWord("for"), NewInteger(4), NewList([]Value{
					NewOpenParen(), NewWord("flw"), NewInteger(9), NewCloseParen(),
					NewWord("def"), NewWord("accfw"), NewOpenParen(), NewWord("accfw"), NewWord("add"), NewInteger(1), NewCloseParen(),
				}), NewEnd(),
				NewWord("accfw"), NewEnd(),
			})
			// break terminates on iteration 1 (acc 0); continue skips every
			// increment (acc 0). Either way the increments after the flow
			// word never run.
			if n, _ := AsInteger(res[len(res)-1]); n != 0 {
				t.Fatalf("acc = %d, want 0", n)
			}
			// The leak pins: the escaped frame's args list and param binding
			// must be gone.
			if _, ok, _ := r.Args.Top(); ok {
				t.Error("per-call args list leaked past the flow-control unwind")
			}
			if r.Defs.Has("x") {
				t.Error("callee param binding leaked past the flow-control unwind")
			}
		})
	}
}
