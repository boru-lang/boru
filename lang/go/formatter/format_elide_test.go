package formatter

import "testing"

// TestElideFnBracketsBranches exercises the decision branches of the
// single-param bracket-elision pass: which fns are rewritten to the
// bracketless triple form and which keep the bracketed spec-list form.
func TestElideFnBracketsBranches(t *testing.T) {
	cases := []struct{ name, in, want string }{
		// Multi-overload (six inner lists) is not a single triple → kept.
		{
			"multi overload kept",
			"def f fn [[a:Integer] [Integer] [a] [b:String] [String] [b]]\n",
			"def f fn [[a:Integer] [Integer] [a] [b:String] [String] [b]]\n",
		},
		// A bare single-param run inside the wrapper is one of the six
		// spellings of the same signature (NUR088) → collapsed like the rest.
		{
			"bare param in the wrapper collapses",
			"def f fn [x [Integer] [x]]\n",
			"def f fn x Integer [x]\n",
		},
		// A single `{…}` map param unbrackets to the triple form.
		{
			"map param unbrackets",
			"def f fn [[{x:Integer}] [Integer] [x]]\n",
			"def f fn {x:Integer} Integer [x]\n",
		},
		// A single value-pattern (literal) param unbrackets.
		{
			"value pattern param unbrackets",
			"def f fn [[0] [String] ['zero']]\n",
			"def f fn 0 String ['zero']\n",
		},
		// A paren-typed param stays bracketed (unsafe to unbracket).
		{
			"paren-typed param kept",
			"def f fn [[x:(Integer)] [Integer] [x]]\n",
			"def f fn [[x:(Integer)] [Integer] [x]]\n",
		},
		// A single param but a multi-type return keeps the return bracketed.
		{
			"multi-type return kept bracketed",
			"def f fn [[x:Integer] [Integer String] [x]]\n",
			"def f fn x:Integer [Integer String] [x]\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Format(c.in); got != c.want {
				t.Errorf("Format(%q)\n  got:  %q\n  want: %q", c.in, got, c.want)
			}
			// Every rewrite is a fixed point.
			if got := Format(c.want); got != c.want {
				t.Errorf("not idempotent: Format(%q) = %q", c.want, got)
			}
		})
	}
}
