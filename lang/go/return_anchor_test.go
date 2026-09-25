package lang

import "testing"

// TestReturnContractErrorAnchorsAtTheCall pins NUR118: a fn's return-contract
// error — the count, or a return value's type — anchors at the CALL word on
// both lanes (the interpreter's ReturnCheck marker; the VM's RET stamps at
// the frame's return address in the caller's debug table, and the recorded
// CALL_USER carries the word's position, not the first argument's). A
// runtime no-match over a gradual argument stays at the word too.
func TestReturnContractErrorAnchorsAtTheCall(t *testing.T) {
	for _, src := range []string{
		"def h fn [[][Integer][1 2]] end h",
		"def h fn [[][Integer][do [1 2]]] end h",
		"def h fn [[n:Integer][String][n add 1]] end h 1",
		`def h fn [[n:Integer][Integer][n]] end def m (flex {k:"x"}) h (m get "k")`,
		`def svc (service {}) end  add {} ([r:Map state:Any] => [1]) svc  def h fn [[][List][do [call {} svc call {} svc]]] end h`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
		if !compiled {
			t.Errorf("%q: must compile", src)
		}
	}
}
