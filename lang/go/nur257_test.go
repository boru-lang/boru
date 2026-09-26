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

// TestNUR257FoldedMemberBinderRead pins the interim rule NUR257 records. A
// map literal's member fn is a FOLDED constant, and its body is analysed at
// the end of the pass with no call-graph identity (NUR105's last position),
// so the rescue cannot ask whether a binder frame reaches it. For that
// position alone, a name some fn binds is not a finding: the member reached
// from the binder's frame compiles and answers as the interpreter does. A
// name no fn binds is still reported (NUR105's typo row), and every other
// anonymous position keeps its finding exactly as before.
func TestNUR257FoldedMemberBinderRead(t *testing.T) {
	// Reached from the binder's frame: a callback, a behaviour (main's #511
	// row, TestBehaveOverContainerMemberCompiles, alike).
	agreeOnBothLanes(t, `def m {c: ([x:Any] => [k])} end def h fn [[][List] [def k 7 [1] each m.c]] end h`, "[[7]]")
	agreeOnBothLanes(t, behaveTemp+`def m {c: (fn [[t:Temp][String][k]])} end def g fn [[][String] [def k 'K' canon (make Temp 5)]] end behave canon/q m.c end g`, "[K]")

	// A name no fn binds: the folded member's typo is still a finding, used
	// or not, and the program declines.
	for _, src := range []string{
		`def m {c: ([x:Any] => [nosuchw 1])} end 1`,
		`def m {c: ([x:Any] => [nosuchw 1])} end [1] each m.c`,
	} {
		if flagged, compiled := checkFlags(t, src, "nosuchw"); !flagged || compiled {
			t.Errorf("%s: the folded member's typo must be a finding (flagged=%v compiled=%v)", src, flagged, compiled)
		}
	}

	// Every other anonymous position is untouched: a callback run at the
	// root over a name only an unrelated fn binds is still a finding, both
	// written in place and held in a list (the interpreter raises), and so is
	// a bare lambda statement (parked as data, never run).
	for _, r := range []struct {
		src    string
		raises bool
	}{
		{`def g fn [[x:Integer][Integer][1]] end [1] each ([e:Any] => [x])`, true},
		{`def g fn [[x:Integer][Integer][1]] end def xs [([e:Any] => [x])] end [1] each xs.0`, true},
		{`def g fn [[x:Integer][Integer][1]] end ([e:Any] => [x]) 5`, false},
	} {
		if flagged, compiled := checkFlags(t, r.src, "x"); !flagged || compiled {
			t.Errorf("%s: an unfolded anonymous body keeps its finding (flagged=%v compiled=%v)", r.src, flagged, compiled)
		}
		if _, err := mustNew(t).RunInterp(r.src); (err != nil) != r.raises {
			t.Errorf("%s: interpreter error %v, want raises=%v", r.src, err, r.raises)
		}
	}
}
