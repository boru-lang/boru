package formatter

import "testing"

// TestFormatLeavesIrreducibleTriples pins the two shapes NUR088's collapse
// leaves as written: a bare-triple fn whose parameter run is not one safe
// parameter, and a spec list whose return slot is neither a list nor a
// word.
func TestFormatLeavesIrreducibleTriples(t *testing.T) {
	for _, keep := range []string{
		"def s fn a:Integer b:Integer [Integer] [a add b]\n",
		"def s fn [x:Integer {a:1} [x]]\n",
	} {
		if got := Format(keep); got != keep {
			t.Errorf("Format(%q) = %q, want it unchanged", keep, got)
		}
	}
}
