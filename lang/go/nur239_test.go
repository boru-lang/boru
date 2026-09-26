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

// TestNUR239BindingNameLabelsTheFrame pins NUR239's binding half: a fn value
// read under a binding and applied over a paren window its only overloads
// do not take fires over nothing (NUR176), and its frame is named by the
// binding it was read under — the interpreter's word dispatch — not by the
// def that made the value. The VM's island dispatches that word over a
// frame binding of it, so a contract error reads `k:` on both lanes, for a
// return type and a return count alike; a value that answers keeps its
// result beside the window.
func TestNUR239BindingNameLabelsTheFrame(t *testing.T) {
	const h = `def h fn [[k:Function] [Integer Integer] [(k 5)]] end h z/v`
	for _, r := range []struct{ src, want string }{
		{`def z fn [[] [Integer] ['s']] end ` + h, "ERROR:k: return value 1: expected Integer, got ProperString"},
		{`def z fn [[] [Integer] ['s' 1]] end ` + h, "ERROR:k: expected 1 return value(s), got 2"},
		{`def z fn [[] [Integer] [7]] end ` + h, "[7 5]"},
		{`def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Any] [(1 2 k)]] end h z/v`, "ERROR:h: expected 1 return value(s), got 3"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
