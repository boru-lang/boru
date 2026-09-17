package lang_test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// This suite stresses the boru signature matcher with USER functions
// (`def f fn [...]`) carrying pathological argument signatures, focusing on
// the structure-first, lazy forward-argument resolution
// (design/legacy/LAZY-ARG-RESOLUTION.10.ignore). It complements lazy_arg_test.go (which
// exercises the real `import` native) by driving the same dispatch machinery
// through user-defined overloads, patterns, refinements, barriers, recursion
// and call-site modifiers.
//
// Findings these tests pin down (all verified against the pre-change binary
// to be either unchanged, or the one intended behavioural difference):
//   - Dispatch SELECTION is unchanged by lazy resolution across every
//     pathological overload set tried — only the EVALUATION TIMING of an
//     unclaimed trailing paren differs.
//   - A forward paren that no viable overload consumes is left raw and runs
//     AFTER the word (the N1 fix), so it observes state the word mutated.
//   - A forward paren that a viable overload DOES consume is evaluated
//     exactly once (no double-eval) and binds as that argument.
//   - All three call forms (forward / swap / prefix) select the same overload.

func mustRun(t *testing.T, src string) []interface{} {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.RunInterp(src)
	if err != nil {
		t.Fatalf("Run(%q) errored: %v", src, err)
	}
	return res
}

func mustErr(t *testing.T, src, wantSubstr string) {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.RunInterp(src)
	if err == nil {
		t.Fatalf("Run(%q) succeeded, want error containing %q", src, wantSubstr)
	}
	if wantSubstr != "" && !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("Run(%q) error %q does not contain %q", src, err.Error(), wantSubstr)
	}
}

func eq(t *testing.T, got, want []interface{}) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %#v (len %d), want %#v (len %d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("value %d: got %#v, want %#v (full: %#v)", i, got[i], want[i], got)
		}
	}
}

// --- Value-level dispatch table: selection must be exactly as expected ---

func TestSigMatch_Pathological(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []interface{}
	}{
		{
			// Type-discriminated overload, no parens.
			"type_overload_int",
			"def f fn [[Integer][String][\"I\"] [String][String][\"S\"]]\nf 5 end",
			[]interface{}{"I"},
		},
		{
			"type_overload_str",
			"def f fn [[Integer][String][\"I\"] [String][String][\"S\"]]\nf \"x\" end",
			[]interface{}{"S"},
		},
		{
			// Three-way overload, each arm selected in turn.
			"three_way",
			"def f fn [[a:Integer][String][\"I\"] [a:String][String][\"S\"] [a:Integer b:Integer][String][\"II\"]]\n(f 1) (f \"x\") (f 1 2) end",
			[]interface{}{"I", "S", "II"},
		},
		{
			// Ambiguity: two equally-arity overloads; the more specific
			// sig[0] (Integer over Any) wins, deterministically.
			"ambiguity_specificity",
			"def f fn [[a:Any b:Integer][String][\"AI\"] [a:Integer b:Any][String][\"IA\"]]\nf 1 2 end",
			[]interface{}{"IA"},
		},
		{
			// LAZY, pruned: leading String prunes the [Integer Integer]
			// overload, so the trailing paren is consumed by no viable
			// overload — it is left raw and runs as a separate statement.
			// Result: f returns "x"; the paren's 7 lands beside it.
			"lazy_pruned_paren_raw",
			"def g fn [[][Integer][7]]\ndef f fn [[a:Integer b:Integer][Integer][a add b] [s:String][String][s]]\nf \"x\" (g) end",
			[]interface{}{"x", int64(7)},
		},
		{
			// LAZY, claimed: leading String keeps the [String Integer]
			// overload viable, so the trailing paren IS a claimed argument
			// and the arity-2 overload is selected.
			"lazy_claimed_paren",
			"def g fn [[][Integer][7]]\ndef f fn [[a:String][String][a] [a:String b:Integer][String][\"TWO\"]]\nf \"x\" (g) end",
			[]interface{}{"TWO"},
		},
		{
			// Value-dependent dispatch: a LEADING paren must be evaluated
			// to learn its type, then it selects the Integer overload.
			"leading_paren_value_dep",
			"def f fn [[Integer][String][\"I\"] [String][String][\"S\"]]\nf (add 2 3) end",
			[]interface{}{"I"},
		},
		{
			// Pattern dispatch on a literal value, fed by a paren that must
			// be evaluated for the pattern to be tested.
			"pattern_paren",
			"def f fn [[0][String][\"zero\"] [n:Integer][String][\"nonzero\"]]\n(f (sub 5 5)) (f (sub 5 4)) end",
			[]interface{}{"zero", "nonzero"},
		},
		{
			// Recursion through a paren argument (claimed at each step).
			"recursion_paren",
			"def sum fn [[0][Integer][0] [n:Integer][Integer][n add (sum (n sub 1))]]\nsum 5 end",
			[]interface{}{int64(15)},
		},
		{
			// Refinement-typed param + a trailing paren. The String value
			// selects the [String] arm; the paren runs separately.
			"refine_param_paren",
			"def Big refine (Integer gt 10)\ndef f fn [[n:Big][Integer][n] [s:String][String][s]]\nf \"x\" (add 1 2) end",
			[]interface{}{"x", int64(3)},
		},
		{
			// All three call forms select the same (arity-2) overload.
			"cross_form_consistency",
			"def f fn [[a:Integer b:Integer][String][\"II\"] [a:Integer][String][\"I\"]]\n(f 4 5) (4 f 5) (4 5 f) end",
			[]interface{}{"II", "II", "II"},
		},
		{
			// /s forces stack collection; /f forces forward — both reach
			// the arity-2 overload from the respective argument sources.
			"force_modifiers",
			"def f fn [[a:Integer b:Integer][Integer][a sub b]]\n(2 3 f/s) (f/f 10 3) end",
			[]interface{}{int64(1), int64(7)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eq(t, mustRun(t, tc.src), tc.want)
		})
	}
}

// --- The one intended behavioural change: unclaimed paren runs AFTER the
// word, so it observes state the word mutated (N1 generalised). ---

func TestSigMatch_LazyOrderingObservesPostWordState(t *testing.T) {
	// f's arity-2 overload raises MaxForwardArgs to 2 (so the former eager
	// scan would have pre-evaluated the paren BEFORE f). The leading String
	// prunes that overload, so under lazy resolution the paren runs AFTER
	// f's [String] body sets x = 2, reading the post-word value.
	res := mustRun(t,
		"context set x 1 end\n"+
			"def f fn [[a:Integer b:Integer][Integer][99] [a:String][String][context set x 2 end a]]\n"+
			"f \"x\" (context dot x) end")
	// Stack: f returned "x"; the trailing paren read x AFTER f mutated it.
	eq(t, res, []interface{}{"x", int64(2)})
}

// --- A claimed paren argument is evaluated exactly once (no double-eval). ---

func TestSigMatch_ClaimedParenEvaluatedExactlyOnce(t *testing.T) {
	res := mustRun(t,
		"context set n 0 end\n"+
			"def bump fn [[][Integer][context set n (add (context dot n) 1) end (context dot n)]]\n"+
			"def f fn [[a:Any b:Any][Any][b]]\n"+
			"f \"z\" (bump) drop end\n"+
			"context dot n end")
	// bump ran once: counter is 1.
	eq(t, res, []interface{}{int64(1)})
}

// --- Pathological footguns the matcher exposes (stable, by-design). ---

func TestSigMatch_BarrierStackOnlyArgFootgun(t *testing.T) {
	// With a barrier `[a | b]`, b is stack-only. The infix form supplies b
	// from the stack and works; the all-forward form leaves b starved and
	// fails — a genuine footgun worth pinning.
	def := "def sl fn [[a:Integer | b:Integer][Integer][b sub a]]\n"
	eq(t, mustRun(t, def+"10 sl 3 end"), []interface{}{int64(7)})
	mustErr(t, def+"sl 3 10 end", "no signature matches")
}

func TestSigMatch_PrunedToNoViableOverloadErrors(t *testing.T) {
	// Leading String prunes the only ([Integer Integer]) overload; the
	// trailing paren is left raw and there is no matching signature — the
	// failure is loud (identical to the pre-change behaviour).
	mustErr(t,
		"def f fn [[a:Integer b:Integer][String][\"II\"]]\nf \"x\" (add 1 2) end",
		"no signature matches")
}
