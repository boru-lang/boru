package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR217ShuffledFnDefIsTheWord pins NUR217's close. A fn laundered
// through a stack shuffle and def-bound (`def j ((mk) dup drop) end j`) was
// an internal error compiled: the def was taken for a multi-value region's
// first-value bind whose rest spills (SplitEventRegionBind), though the drop
// had consumed the rest, so the splice underflowed (`def j (1 dup drop) end
// j add 1` did too, over plain data), and a root read counted the drop as
// its consumer. The split now declines when a later event consumed the
// rest, the read's consumer is the one that takes ITS result, a push-tested
// read with no push is tested before its statement when it is the
// consumer's deepest operand, the def's multi-output source is promoted
// when every rest result is consumed, and the shuffled copy carries the
// factory's shape claim, so every position reads it as it reads `def j
// (mk)`. The other four witnesses already agreed; `do [l.0]` still declines
// at the fold's fence (the fold is not measured through a code body's
// result, and the non-folded twin of `def j (5 do [(mk)])` answers wrong —
// NUR286).
func TestNUR217ShuffledFnDefIsTheWord(t *testing.T) {
	const mk = `def mk fn [[][Any][([] => [42])]] end `
	const mk1 = `def mk fn [[][Any][([x:Integer] => [x add 1])]] end `
	for _, c := range []struct{ src, want string }{
		{mk + `def j ((mk) dup drop) end j`, "[42]"},
		{mk + `def j ((mk) dup drop) end j add 1`, "[43]"},
		{mk + `def j ((mk) dup drop) end j typeof`, "[Integer]"},
		{mk + `def j ((mk) dup drop) end [j]`, "[[42]]"},
		{mk + `def j ((mk) dup drop) end 1 j`, "[1 42]"},
		{mk + `def j ((mk) dup drop) end 3 j add 1`, "[3 43]"},
		{mk + `def j ((mk) dup drop) end do [j]`, "[42]"},
		{mk + `def j ((mk) dup drop) end [10] each [drop j]`, "[[42]]"},
		{mk + `def j ((mk) dup drop) end def g fn [[][Any][j]] end g`, "[42]"},
		{mk + `def j ((mk) dup drop) end j/v`, "[fn j]"},
		{mk + `def j (5 (mk) nip) end j`, "[42]"},
		{mk + `def j ((mk) 5 over drop drop) end j`, "[42]"},
		{mk1 + `def j ((mk) dup drop) end 5 j`, "[6]"},
		{mk1 + `def j ((mk) dup drop) end [10] each [j]`, "[[11]]"},
		{`def mk fn [[][Any][(7)]] end def j ((mk) dup drop) end [10] each [drop j]`, "[[7]]"},
		{`def j (1 dup drop) end j add 1`, "[2]"},
		{`def j (1 2 swap drop) end j add 1`, "[3]"},
		{`def j (1 2 swap drop) end [10] each [drop j]`, "[[2]]"},
		{`def j (1 2 over drop drop) end j`, "[1]"},
		{`def a (1 2 over) end a`, "[2 1 1]"},
		{`def s (flex {}) end s set f ([] => [42]) end def j (s get "f") end j`, "[{f:fn} 42]"},
		{`def mk fn [[][Function][([] => [42])]] end def mk2 fn [[][Any][(mk)]] end def j (mk2) end j`, "[42]"},
		{`def m {f: ([] => [42])} end def m (m set f ([] => [7])) end def j m.f end j`, "[7]"},
		{mk + `def c true end def j (if c [(mk)] [2]) end j`, "[42]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, `def l [([] => [42])] end def j (do [l.0]) end j`)
	if compiled || fmt.Sprint(gotI) != "[42]" || errI != nil || !strings.Contains(fmt.Sprint(errC), "folded to its parking lambda") {
		t.Errorf("the fold's fence still declines `do [l.0]`; got compiled=%v %v %v / %v %v", compiled, gotC, errC, gotI, errI)
	}
}

// TestNUR285FnDefReadInABody pins NUR285's close. A root def bound to a
// factory's fn and read inside a code body at the root, over values the
// body pushed beneath it (`do [10 j]`, `[1 2] each [j]`), was a slot push —
// `[10 fn j]` for the interpreter's 11 — because the body's deopt point
// needed a parent unit to bind its captures and the root is none: the root's
// defs are in the registry already, so the point stands, installing the
// captured fn under its name for the island (DeoptSpec.Install). Read in a
// fn body, the lookup that fed the frame's replay deferred on the fn
// binding (an internal error); the replay window's word reads take the
// data lookup. And a def whose value lives in a frame local kept the
// unrenamed copy there, so `def j (do [(mk)]) end j/v` read `fn` for `fn
// j`: the def's rename goes back to the local (GlobalBindSpec.WriteSlot).
func TestNUR285FnDefReadInABody(t *testing.T) {
	const mk = `def mk fn [[][Any][([x:Integer] => [x add 1])]] end def j (mk) end `
	for _, c := range []struct{ src, want string }{
		{mk + `do [10 j]`, "[11]"},
		{mk + `do [5 10 j]`, "[5 11]"},
		{mk + `do [10 j j]`, "[12]"},
		{mk + `[10] each [j]`, "[[11]]"},
		{mk + `[1 2] each [j]`, "[[2 3]]"},
		{mk + `[10] each [dup drop j]`, "[[11]]"},
		{mk + `def g fn [[][Any][10 j]] end g`, "[11]"},
		{mk + `def g fn [[y:Integer][Any][y j]] end g 10`, "[11]"},
		{mk + `def f ([] => [10 j]) end f`, "[11]"},
		{`def m {f: ([x:Integer] => [x mul 2])} end def j m.f end def g fn [[][Any][10 j]] end g`, "[20]"},
		{`def mk fn [[][Any][([] => [42])]] end def j (do [(mk)]) end j/v`, "[fn j]"},
		{`def mk fn [[][Any][([] => [42])]] end def j (do [(mk)]) end {a: j/v}`, "[{a:fn j}]"},
		{`def mk fn [[][Any][(7)]] end def j (mk) end do [10 j]`, "[10 7]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	gotC, _, errC, gotI, errI := runBothEngines(t, mk+`def g fn [[][Any][j]] end g`)
	if codeOf(errI) != "signature_error" || fmt.Sprint(gotC, errC) != fmt.Sprint(gotI, errI) {
		t.Errorf("a bare read with nothing beneath is the interpreter's no-match on both lanes; got %v %v / %v %v", gotC, errC, gotI, errI)
	}
}
