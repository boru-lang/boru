package lang

import "testing"

// TestNUR239NamelessFnValueIsFn pins NUR239's first half: a nameless fn
// value's return-contract error names its frame `<fn>` on both lanes, as the
// interpreter's fn-value frame does. (The second half — an applied value
// named after the binding it was called under — is still open.)
func TestNUR239NamelessFnValueIsFn(t *testing.T) {
	agreeOnBothLanes(t, `def Handler class {cb: Function} def h (make Handler {cb: (fn [[n:Integer][Integer][n 1]])}) each h.cb [1 2 3]`,
		"ERROR:<fn>: expected 1 return value(s), got 2")
}
