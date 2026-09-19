package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestGradualArgParamGuard pins the fix for a real compile==interpret VIOLATION
// (found by the Stage-D-2/4 adversarial sweep): a gradual (Dynamic) value — a
// value of static type exactly Any, e.g. laundered through an `[Any]`-returning
// helper — matches a CONCRETE user-fn param OPTIMISTICALLY at check time (the
// gradual-Any allowance). The compiled OpCallUser used to enter the fn frame with
// NO param-type check, while the interpreter runtime-matches the concrete value:
// a List laundered into an `m:Map` param ran the body and returned `m size` = 3
// where the interpreter raises signature_error.
//
// The fix is a runtime param guard at CALL_USER entry (CompiledFn.Params, checked
// via v.Is — the compiled mirror of the RET return-check). So the call now
// COMPILES but raises the SAME signature_error at run time. Pinned with the clean
// `f (id …)` call form so the interpreter's error is the plain signature_error
// (not the forward-collection usage hint a bare `(r f)` produces).
func TestGradualArgParamGuard(t *testing.T) {
	laundered := []struct{ name, src string }{
		{"List laundered into m:Map", `def id fn [[x:Any] [Any] [x]] def f fn [[m:Map] [Integer] [m size]] f (id [10 20 30])`},
		{"Integer laundered into s:String", `def id fn [[x:Any] [Any] [x]] def f fn [[s:String] [Integer] [s size]] f (id 99)`},
		{"List laundered into m:Map via 0-arg [Any] fn", `def g fn [[] [Any] [[1 2 3]]] def f fn [[m:Map] [Integer] [m size]] f (g)`},
		// FnParam.Pattern constraints — the inline-disjunct / inline-predicate
		// residual the param-guard adversarial sweep flagged, closed by also
		// threading + checking ParamPatterns (OpenUnifyMap / Unify, the interpreter's
		// dispatch check).
		{"inline disjunct non-member laundered", `def id fn [[y:Any] [Any] [y]] def f fn [[x:(Integer tor String)] [Integer] [42]] def v (id true) (v f)`},
		{"inline predicate out-of-range laundered", `def id fn [[y:Any] [Any] [y]] def f fn [[b:(Integer gt 10)] [Integer] [99]] def v (id 5) (v f)`},
	}
	for _, c := range laundered {
		t.Run("guarded/"+c.name, func(t *testing.T) {
			a, _ := New()
			gotC, _, errC := a.RunCompiled(c.src)
			if noteCompileDefect(t, c.src, gotC, errC) {
				return
			}
			// The miscompile was: compiled RETURNED a value. Now it must ERROR.
			if errC == nil {
				t.Fatalf("compiled must raise (param guard), not return %v", gotC)
			}
			if !strings.Contains(fmt.Sprint(errC), "no signature matches") {
				t.Errorf("compiled error = %v, want a signature_error", errC)
			}
			// And the interpreter raises the SAME thing.
			b, _ := New()
			_, errI := b.RunInterp(c.src)
			if errI == nil || !strings.Contains(fmt.Sprint(errI), "no signature matches") {
				t.Errorf("interpreter error = %v, want a signature_error", errI)
			}
		})
	}

	// NEGATIVE: a directly-typed arg matches its concrete param for real, so it
	// must STILL compile natively to the correct value (the guard is a pass).
	typed := []struct{ name, src, want string }{
		{"Map literal -> m:Map", `def f fn [[m:Map] [Integer] [m size]] (f {a:1 b:2})`, "[2]"},
		{"Integer -> n:Integer", `def f fn [[n:Integer] [Integer] [n add 1]] (f 41)`, "[42]"},
		// A gradual arg whose runtime value DOES match the param compiles AND runs
		// correctly (the guard passes) — the Test.run-spec harness shape in miniature.
		{"gradual arg that matches at runtime", `def id fn [[x:Any] [Any] [x]] def f fn [[n:Integer] [Integer] [n add 1]] f (id 41)`, "[42]"},
		// Options-typed params: the guard must use sigTypeMatches, NOT v.Is — a
		// concrete map satisfies an Options slot (the codebase recommends
		// `opts:Options`). A v.Is guard OVER-RAISED here; these pin that it does not.
		{"Options param + map", `def f fn [[o:Options] [Integer] [o size]] (f {a:1 b:2})`, "[2]"},
		{"optional Options param", `def f fn [[a:Integer o?:Options] [Integer] [a]] (f 7 {a:1 b:2})`, "[7]"},
		{"Options laundered through id", `def id fn [[x:Any] [Any] [x]] def f fn [[o:Options] [Integer] [o size]] f (id {a:1 b:2})`, "[2]"},
		// Inline-Pattern MEMBERS must compile+run (the pattern check must not over-raise).
		{"inline disjunct member (Integer)", `def f fn [[x:(Integer tor String)] [Integer] [7]] (f 3)`, "[7]"},
		{"inline disjunct member (String)", `def f fn [[x:(Integer tor String)] [Integer] [7]] (f "hi")`, "[7]"},
		{"inline predicate in-range", `def f fn [[b:(Integer gt 10)] [Integer] [b]] (f 20)`, "[20]"},
	}
	for _, c := range typed {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("must compile and run, error: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
