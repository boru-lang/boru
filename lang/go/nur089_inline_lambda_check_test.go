package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestInlineLambdaChecksLikeReference pins NUR089's close. An inline `=>`
// lambda and a named `/v` reference to the same function are the same
// function value, and the check must treat them alike. The pass bound an
// analysed body's params and captures with a raw push, where the run binds
// them through the frame install that makes a fn value's signatures
// dispatch-ready; an inline lambda's authored signature is not, so a body
// CALLING the bound name (`g x` inside a curried combinator) matched
// nothing and drew `no_signature: cannot call g` — for the lambda spelling
// only, since a named fn's signatures were compiled at its def. Both
// spellings now check clean and run to the same answer; an argument the run
// rejects is still reported, in both spellings, with its real type.
func TestInlineLambdaChecksLikeReference(t *testing.T) {
	const bb = `def bb f:Function => [g:Function => [x:Any => [f (g x)]]] `
	const refs = `def d fn n:Integer Integer [mul n 2] def e fn n:Integer Integer [add n 3] `
	const ss = `def ss f:Function => [g:Function => [x:Any => [((f x) (g x))]]] `
	for _, c := range []struct{ src, run, diag string }{
		{bb + `(((bb (n:Integer => [mul n 2])) (n:Integer => [add n 3])) 4)`, "[14]", ""},
		{refs + bb + `(((bb d/v) e/v) 4)`, "[14]", ""},
		{ss + `(((ss (a:Any => [b:Any => [a]])) (a:Any => [b:Any => [a]])) 5)`, "[5]", ""},
		{ss + `def kk a:Any => [b:Any => [a]] (((ss kk/v) kk/v) 5)`, "[5]", ""},
		// Negatives: the run raises on a String, and the check says so in
		// both spellings, naming the argument's real type.
		{bb + `(((bb (n:Integer => [mul n 2])) (n:Integer => [add n 3])) 'zz')`, "signature_error", "got (ProperString)"},
		{refs + bb + `(((bb d/v) e/v) 'zz')`, "signature_error", "got (ProperString)"},
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		cr, err := a.Check(c.src)
		if err != nil {
			t.Fatalf("%q: check: %v", c.src, err)
		}
		var diags []string
		named := false
		for _, d := range cr.Diagnostics {
			diags = append(diags, d.Code+": "+d.Detail)
			named = named || (d.Code == "no_signature" && strings.Contains(d.Detail, c.diag))
		}
		switch {
		case c.diag == "" && len(diags) != 0:
			t.Errorf("%q: must check clean, got %v", c.src, diags)
		case c.diag != "" && !named:
			t.Errorf("%q: want a no_signature naming %q, got %v", c.src, c.diag, diags)
		}
		got, err := mustNew(t).RunInterp(c.src)
		answer := fmt.Sprint(got)
		if err != nil {
			answer = codeOf(err)
		}
		if answer != c.run {
			t.Errorf("%q: interpreted %s, want %s", c.src, answer, c.run)
		}
	}
}
