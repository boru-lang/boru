package lang

import "testing"

// checkFlags reports whether src's check pass names word in an
// undefined-word finding, and whether the program compiled.
func checkFlags(t *testing.T, src, word string) (flagged, compiled bool) {
	t.Helper()
	prog, _, diags, _ := mustNew(t).CompileCheck(src)
	for _, d := range diags.Diagnostics {
		if d.Code == "undefined_word" && d.Word == word {
			flagged = true
		}
	}
	return flagged, prog != nil
}

// TestNUR257AnonymousReaderIdentity pins NUR257's close. An anonymous fn
// value's body is analysed at the end of the pass (NUR105's drain), or
// compiled as a stored unit where it was written, outside every frame that
// runs it, so the dynamic-scope rescue had no reader to ask about. The value
// now has one — its body's position (core AnonFnBodies) — and the frames
// that run it reach it: one that runs it as a callback or applies it
// (AnalyseFnBody's entry), and one that handles a value of a type `behave`
// installed it on (the capability, BehaveReaders). A name only a fn binds is
// a finding exactly when no binder frame reaches the value, in every
// position — a list member, a map member, written in place.
func TestNUR257AnonymousReaderIdentity(t *testing.T) {
	// Reached from the binder's frame: no finding, and both lanes answer.
	for _, c := range []struct{ src, want string }{
		{`def m {c: ([x:Any] => [k])} end def h fn [[][List] [def k 7 [1] each m.c]] end h`, "[[7]]"},
		{`def m [([x:Any] => [k])] end def h fn [[][List] [def k 7 [1] each m.0]] end h`, "[[7]]"},
		{`def xs [([e:Any] => [k])] end def h fn [[][List] [def k 7 [1] each xs.0]] end h`, "[[7]]"},
		{behaveTemp + `def m {c: (fn [[t:Temp][String][k]])} end def g fn [[][String] [def k 'K' canon (make Temp 5)]] end behave canon/q m.c end g`, "[K]"},
		{behaveTemp + `def m [(fn [[t:Temp][String][k]])] end def g fn [[][String] [def k 'K' canon (make Temp 5)]] end behave canon/q m.0 end g`, "[K]"},
		{behaveTemp + `def g fn [[][String] [def k 'K' canon (make Temp 5)]] end behave canon/q (fn [[t:Temp][String][k]]) end g`, "[K]"},
	} {
		if flagged, _ := checkFlags(t, c.src, "k"); flagged {
			t.Errorf("%s: reached from k's binder, no finding", c.src)
		}
		agreeOnBothLanes(t, c.src, c.want)
	}

	// Reached from no binder frame: a finding, and the program declines — a
	// callback run at the root, a behaviour exercised there, a value never
	// run — whatever the value's position.
	for _, r := range []struct {
		src, word string
		raises    bool
	}{
		{`def g fn [[x:Integer][Integer][1]] end def m {c: ([e:Any] => [x])} end [1] each m.c`, "x", true},
		{`def g fn [[x:Integer][Integer][1]] end [1] each ([e:Any] => [x])`, "x", true},
		{`def g fn [[x:Integer][Integer][1]] end def xs [([e:Any] => [x])] end [1] each xs.0`, "x", true},
		{`def g fn [[x:Integer][Integer][1]] end def m {c: ([e:Any] => [x])} end 1`, "x", false},
		{`def g fn [[x:Integer][Integer][1]] end def m [([e:Any] => [x])] end 1`, "x", false},
		{behaveTemp + `def g fn [[][String] [def k 'K' 1]] end behave canon/q (fn [[t:Temp][String][k]]) end canon (make Temp 5)`, "k", false},
		{`def m {c: ([x:Any] => [nosuchw 1])} end [1] each m.c`, "nosuchw", true},
	} {
		if flagged, compiled := checkFlags(t, r.src, r.word); !flagged || compiled {
			t.Errorf("%s: no binder reaches the value: a finding and a decline (flagged=%v compiled=%v)", r.src, flagged, compiled)
		}
		if _, err := mustNew(t).RunInterp(r.src); (err != nil) != r.raises {
			t.Errorf("%s: interpreter error %v, want raises=%v", r.src, err, r.raises)
		}
	}
}
