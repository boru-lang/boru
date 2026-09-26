package lang

import "testing"

// TestFnValueSeamAnchorsAtTheRead pins NUR122 (and NUR118's fn-value seam):
// a fn VALUE's failed dispatch or return contract, applied inside a user fn
// — the bare spelling `g x`, the paren window `(g 5)`, a returned lambda's
// `(g x)`, a lambda read from a map member — raises the same error at the
// same token on both lanes: the check pass's carrier for a bare fn-typed
// binding read carries the read's position, and the trailing apply seats it.
func TestFnValueSeamAnchorsAtTheRead(t *testing.T) {
	for _, src := range []string{
		"def ap fn [[g:Function][Any][(g 5)]]  ap ([x:Integer] => [x 1])",
		"def f fn [[g:Function x:Integer][Integer][g x]]  f (z:String => [z]) 5",
		"def f fn [[g:Function x:Integer][Integer][g x]]  f ([] => [42]) 5",
		"def app fn [[g:Function][Function][( fn [[x:Integer][Integer][(g x)]] )]]  def h (app (z:String => [z]))  (h 5)",
		"def m {f: ([x:Integer] => [x 1])}  m.f 5",
	} {
		gotC, _, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}
