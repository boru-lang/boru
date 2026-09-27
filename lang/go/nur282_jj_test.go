package lang

import "testing"

// TestNUR282DispatchResultsInARow pins NUR282's `j j`. A def-bound name
// over a factory's fn value dispatches at each read; the interpreter parks
// what a boru fn returns (fnReturnPark), so each result sits as data beside
// the next. The compiled residual declined the first result as a dynamic
// value that might apply to the entries above it. It is placed now,
// claimed on its apply (DynMethodSpec.Parks), and the VM defers on a callee
// whose result the interpreter might step on — a native (TestParksResult).
func TestNUR282DispatchResultsInARow(t *testing.T) {
	const mk = `def mk fn [[][Any][([] => [42])]] end def j (mk) end `
	for _, c := range []struct{ src, want string }{
		{mk + `j j`, "[42 42]"},
		{mk + `j ; j`, "[42 42]"},
		{mk + `j j j`, "[42 42 42]"},
		{mk + `def k (mk) end j k`, "[42 42]"},
		{`def mk fn [[][Any][([] => [([] => [5])])]] end def j (mk) end j j`, "[fn fn]"},
		{`def mk fn [[][Any][([] => [([x:Integer] => [x])])]] end def j (mk) end j j 7`, "[fn (Integer) fn (Integer) 7]"},
		{`def mk fn [[][Any][5]] end def j (mk) end j j`, "[5 5]"},
		{`def inc fn [[n:Integer][Integer][n add 1]] end def mk fn [[][Any][([] => [inc/v])]] end def j (mk) end j j 5`, "[fn inc(Integer) fn inc(Integer) 5]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
