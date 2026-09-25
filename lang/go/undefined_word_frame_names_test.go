package lang

import "testing"

// TestCompiledUndefinedWordSuggestsFrameNames pins NUR146: the did-you-mean
// pool of a compiled undefined_word holds the frame's locals — a loop
// variable the interpreter's registry holds as a def — so the two lanes
// render the same help line (`did you mean \`i\`?` for the popped `k`).
func TestCompiledUndefinedWordSuggestsFrameNames(t *testing.T) {
	for _, src := range []string{
		"def k 5 for 2 [ if (k eq 5) [undef k] [] ] 9",
		"def kk 5 for 2 [ if (kk eq 5) [undef kk] [] ] 9",
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
		if !compiled {
			t.Errorf("%q: must compile", src)
		}
	}
}
