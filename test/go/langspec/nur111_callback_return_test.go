package langspec

import "testing"

// TestCallbackReturnContractIsChecked pins NUR111: the checker holds a fn
// body to its OWN declared return regardless of the shape that reaches it.
//
// Before the fix only the first two shapes were flagged. The third — a fn
// VALUE handed to a higher-order word — never dispatches during the check
// pass, so the declaration was enforced by nobody statically while BOTH
// engines raised. The pass already analysed that body (an undefined word
// inside it was reported); what was missing was the return obligation.
//
// The fourth row is the deliberate widening the fix carries with it, and it
// is pinned HERE rather than in the corpus on purpose: the program runs
// fine (the fn is never called), so a spec row for it would register as a
// checker false positive in TestCheckAccuracyRatchet's model and there is
// no honest pin value for it there. The finding is still right — a body
// that can never satisfy its declaration is a static error wherever it sits
// — so it is stated as its own claim instead of hidden in a ratchet count.
func TestCallbackReturnContractIsChecked(t *testing.T) {
	t.Parallel()
	const decl = "def cbad fn [[n:Integer][Boolean][n]] end "

	cases := []struct {
		name  string
		src   string
		flags bool
	}{
		{"called directly", decl + "cbad 1", true},
		{"body as a code block", decl + "[1 2] each [cbad]", true},
		{"fn VALUE as a callback", decl + "[1 2] each cbad/v", true},
		{"never called at all", decl + "1", true},
		{
			"a conforming callback stays clean",
			"def cokr fn [[n:Integer][Boolean][n gt 0]] end [1 2] each cokr/v",
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkFlagsError(t, tc.src); got != tc.flags {
				t.Fatalf("checkFlagsError(%q) = %v, want %v", tc.src, got, tc.flags)
			}
		})
	}
}

// TestGeneralisedResidualStaysSilent pins the soundness boundary of that
// same obligation, from the other side: three CORRECT corpus rows the drain
// falsely rejected before the residual gate landed. Each one analyses to a
// residual the generalised pass did not derive — an operand stranded by an
// application it could not model, or a concrete param stand-in — and the
// checker must stay silent on all three.
func TestGeneralisedResidualStaysSilent(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, src string }{
		{
			"unapplied fn-typed param strands its literal",
			`def f fn x:Integer String [convert String x]  def h fn g:(fnsig Integer String) String [(g 5)]  h f/v`,
		},
		{
			"unapplied fnsig-typed param strands both literals",
			`def T (fnsig [[Integer Integer] [Integer]])  def f fn [[a:Integer b:Integer][Integer][add a b]]  def h fn g:T Integer [(g 2 3)]  h f/v`,
		},
		{
			"a Map param's concrete stand-in survives to the residual",
			`import "boru:emitlang"  def px fn [[value:Any opts:Map] [String] [(opts get 'p') add (emit value)]]  emit px {p:'X:'} {a:1}`,
		},
		{
			// Raised in review on #414: an unmodelled application can strand
			// ONLY carriers, and at exactly the declared arity, so neither
			// the concrete check nor the length check sees it. The stranded
			// CALLEE is what gives it away. Without that arm this reports
			// three type_errors here.
			"an unmodelled application strands only carriers at the declared arity",
			`def T (fnsig [[Integer Integer] [Boolean Boolean Boolean]])  def h fn [[g:T a:Integer b:Integer] [Boolean Boolean Boolean] [(g a b)]]  1`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if checkFlagsError(t, tc.src) {
				t.Fatalf("checker flags a correct program: %s", tc.src)
			}
		})
	}
}
