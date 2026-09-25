package formatter

import "testing"

// NUR088: one single-parameter signature has six valid spellings, all
// building a canon-identical value; the formatter collapses every one of
// them to the one-pair form, and leaves the irreducible shapes alone.
func TestFormatCollapsesEverySingleParamFnSpelling(t *testing.T) {
	const want = "def s fn x:Integer Integer [mul 2 x]\n"
	for _, in := range []string{
		"def s fn x:Integer Integer [mul 2 x]\n",
		"def s fn x:Integer [Integer] [mul 2 x]\n",
		"def s fn [x:Integer Integer [mul 2 x]]\n",
		"def s fn [[x:Integer] Integer [mul 2 x]]\n",
		"def s fn [x:Integer [Integer] [mul 2 x]]\n",
		"def s fn [[x:Integer] [Integer] [mul 2 x]]\n",
	} {
		if got := Format(in); got != want {
			t.Errorf("Format(%q)\n  got:  %q\n  want: %q", in, got, want)
		}
	}
	// The unnamed-parameter twin collapses the same way.
	if got, want := Format("def s fn [Integer [Integer] [add 1]]\n"), "def s fn Integer Integer [add 1]\n"; got != want {
		t.Errorf("unnamed param: got %q, want %q", got, want)
	}
	// Irreducible shapes pass through: two params, a bare List param, a
	// multi-overload spec list.
	for _, keep := range []string{
		"def s fn [[a:Integer b:Integer] [Integer] [a add b]]\n",
		"def s fn [[List] [Integer] [size]]\n",
		"def s fn [[[a:Integer] [Integer] [a]] [[b:String] [String] [b]]]\n",
	} {
		if got := Format(keep); got != keep {
			t.Errorf("Format(%q) = %q, want it unchanged", keep, got)
		}
	}
}
