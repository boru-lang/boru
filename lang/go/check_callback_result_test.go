package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The check pass types a fold's or scan's result from a FUNCTION body as it
// does from a quotation body (native_array.go analyseCallbackFn): the lambda
// is matched over the arm's inputs — top-down off the stack for a list,
// positionally for a map — and its body's accumulator is the result. Before
// the arm the result was a strict Any, so every later read of it failed
// no_signature although the program ran; the var construct's quotation
// bodies never had the problem, and their lambda rewrites must not either
// (design/IMMUTABLE-DEF.1.md §5 phase 1, kg's `groups`).
func TestCheckTypesFoldWithLambdaBody(t *testing.T) {
	clean := []struct{ src, want string }{
		{`def groups (fold ([a acc] => [acc set (a.k) a]) [{k:"x"}] {}) groups get "x"`, "[{k:'x'}]"},
		{`def xs (fold ([a acc] => [acc add a]) [1 2] 0) xs add 1`, "[4]"},
		{`def m {a:1 b:2} fold ([acc kv:KeyVal] => [acc add kv.v]) m 0 add 1`, "[4]"},
		{`def m {a:1 b:2} fold ([acc v] => [acc add v]) m 0 add 1`, "[4]"},
		{`def n (fold ([a acc] => [acc add a]) [1 2]) n add 1`, "[4]"},
		{`def ys (scan ([a acc] => [acc add a]) [1 2 3]) (ys get 2) add 1`, "[7]"},
		// A body the pass cannot analyse (a fn-typed carrier) leaves the
		// result gradual, never strict: the read after it still types.
		{`def f fn [[g:Function][Any][fold g/v [1 2] 0 add 1]] f ([a acc] => [acc add a])`, "[4]"},
	}
	for _, c := range clean {
		a := mustNew(t)
		res, err := a.Check(c.src)
		if err != nil {
			t.Fatalf("%s: check %v", c.src, err)
		}
		for _, d := range res.Diagnostics {
			if d.Severity == "error" {
				t.Errorf("%s: unexpected check error %s: %s", c.src, d.Code, d.Detail)
			}
		}
		got, rerr := mustNew(t).RunInterp(c.src)
		if rerr != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: ran %v / %v, want %s", c.src, got, rerr, c.want)
		}
	}
	// The negative pair: the lambda's own body still reports its misuse of
	// the accumulator, on the pass and at run time.
	bad := `fold ([a acc] => [acc keys]) [1] 0`
	res, err := mustNew(t).Check(bad)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if strings.Contains(d.Detail, "keys") {
			found = true // no_signature on the first round, partial_dispatch once the accumulator joined
		}
	}
	if !found {
		t.Errorf("%s: the body's misuse of the accumulator was not reported: %+v", bad, res.Diagnostics)
	}
	if _, rerr := mustNew(t).RunInterp(bad); rerr == nil {
		t.Errorf("%s: ran clean", bad)
	}
}
