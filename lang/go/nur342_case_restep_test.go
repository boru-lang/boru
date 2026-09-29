package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// requireCompiledOutputParity is requireCompiledParity that also compares
// everything the program wrote: the program must compile, and the compiled
// lane must answer, raise and print exactly what the interpreter does, in
// the interpreter's order.
func requireCompiledOutputParity(t *testing.T, src string) {
	t.Helper()
	var outC, outI bytes.Buffer
	c := mustNew(t)
	c.SetOutput(&outC)
	gotC, compiled, errC := c.RunCompiled(src)
	i := mustNew(t)
	i.SetOutput(&outI)
	gotI, errI := i.RunInterp(src)
	if !compiled || fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) || outC.String() != outI.String() {
		t.Errorf("%q:\n  compiled %v [%v] out=%q (compiled=%v)\n  interp   %v [%v] out=%q",
			src, gotC, errC, outC.String(), compiled, gotI, errI, outI.String())
	}
}

// TestNUR342ReSteppedBlockCompiles pins NUR342's compile: a case whose
// taken clause block is a bare FUNCTION word (or a splice binding) hands
// the word back, and the interpreter re-steps it AT THE CASE over the values
// beneath it and the tokens after it. Where the check pass can decide the
// clause the run takes — a known scalar scrutinee, and guards that are
// scalar literal matches or predicates over pure comparison words — the
// case is exactly that word written in its place: the pass hands the word
// back and re-steps it there, each call it makes recording on its own, and
// the case records nothing. These declined before (and the `g` rows were a
// silent wrong answer on main 2ad4620, [1 5] for [101]).
func TestNUR342ReSteppedBlockCompiles(t *testing.T) {
	for _, src := range []string{
		// The rows TestNUR332ReSteppedBlockDeclines pinned as declines.
		`1 2 case 7 [[lt 10] add]`,
		`case 7 [[lt 10] add] 1 2`,
		`1 case 7 [[lt 10] dup]`,
		`case 7 [[lt 10] if "z"] 1 2 3`,
		`def g fn [[][Integer][5] [x:Integer][Integer][x add 100]] end 1 case 7 [[lt 10] g "z"]`,
		`def g fn [[x:Integer][Integer][x add 100] [][Integer][5]] end 1 case 7 [[lt 10] g "z"]`,
		`1 2 case 7 [[lt 10] depth "z"]`,
		`def mk fn [[][Any][5]] end def k (mk) end 1 case 7 [[lt 10] k "z"]`,
		`def g word [add] 1 2 case 7 [[lt 10] g 0]`,
		`def g word [add] end 1 2 case 7 [[lt 10] g 0] end`,
		`1 if (1 eq 1) [case 7 [[lt 10] dup]] [0]`,
		// Written order and Forth order around a non-commutative word.
		`1 2 case 7 [[lt 10] sub]`,
		`case 7 [[lt 10] sub] 10 3`,
		`10 3 case 7 [[lt 10] sub]`,
		`10 3 7 case [[lt 10] sub "z"]`,
		// A later clause, the default, a value match, a string, a float, a
		// predicate over a named constant.
		`1 2 case 7 [[gt 10] "big" [lt 10] add]`,
		`1 2 case 20 [[lt 10] "a" [gt 15] add "d"]`,
		`1 2 case 7 [8 "no" 7 add "z"]`,
		`1 2 case "b" ["a" "x" "b" add "d"]`,
		`1 2 case 7.5 [[lt 10] add "z"]`,
		`1 2 case 7 [[lt 10 5] add "z"]`,
		`1 2 case 7 [[] add "z"]`,
		`def c 5 end 1 2 case 7 [[gt c] add "z"]`,
		`def n 7 end 1 2 case n [[lt 10] add "z"] n`,
		// The re-step's no-match raises the interpreter's error.
		`1 2 case 20 [[lt 10] "a" add]`,
		`case 7 [[lt 10] add] 1`,
		`1 case 7 [[lt 10] add]`,
		`1 2 (case 7 [[lt 10] add "z"])`,
		// Consumed, listed, bound, after a statement end.
		`(1 2 case 7 [[lt 10] add]) mul 3`,
		`[1 2 case 7 [[lt 10] add]]`,
		`def r (1 2 case 7 [[lt 10] add]) end r`,
		`1 2 case 7 [[lt 10] add] end 5`,
		`def g fn [[][Integer][5] [x:Integer][Integer][x add 100]] end 1 case 7 [[lt 10] g "z"] end 4`,
		`def g fn [[][Integer][5] [x:Integer][Integer][x add 100]] end case 7 [[lt 10] g "z"] 4`,
		// Inside an arm, a fn body, a loop body, a do.
		`if (1 eq 1) [1 2 case 7 [[lt 10] add]] [0]`,
		`1 if (1 eq 1) [2 case 7 [[lt 10] add]] [0]`,
		`def f fn [[x:Integer][Any][x x case 7 [[lt 10] add 0]]] end f 4`,
		`def f fn [[x:Integer y:Integer][Any][x y case 7 [[lt 10] sub 0]]] end f 10 3`,
		`def f fn [[x:Integer][Any][x case 7 [[lt 10] add 0]]] end f 1`,
		`def f fn [[n:Integer][Any][case 7 [[lt 10] add "z"] n n]] end f 5`,
		`def f fn [[Integer][Any][case 3 [[lt 10] dup "z"]]] end f 1`,
		`[1 2] each [10 case 7 [[lt 10] add 0]]`,
		`[1 2] each [case 7 [[lt 10] add 0]]`,
		`do [1 2 case 7 [[lt 10] add "z"]]`,
		`1 2 do [case 7 [[lt 10] add "z"]]`,
		// The taken block is no re-stepping word: the chain's sealed arm is
		// exact, and the re-stepping arm never runs.
		`1 2 case 20 [[lt 10] add "z"]`,
		`1 2 case 7 [[gt 10] add "small"]`,
	} {
		requireCompiledParity(t, src)
	}
}

// TestNUR342ReSteppedBlockInAFrame: a case bare in its fn frame — nothing
// beneath it in the frame, nothing after it in the body — meets exactly
// what the desugar's sealed arm meets, as a bare top-level case does: the
// run seals the frame at its bottom (Engine.FrameRoot). So a scrutinee only
// the run knows compiles there too, and the no-match raises the
// interpreter's error (the fn-body row TestNUR332ReSteppedBlockDeclines
// pinned as a decline).
func TestNUR342ReSteppedBlockInAFrame(t *testing.T) {
	for _, src := range []string{
		`def f fn [[x:Integer][Any][case x [[lt 10] dup "z"]]] end f 1`,
		`def f fn [[x:Integer][Any][case x [[lt 10] dup "z"]]] end f 20`,
		`def f fn [[x:Integer][Any][case x [[lt 10] dup "z"]]] end [(f 20)]`,
		`def f fn [[x:Integer][Any][case x [[lt 10] dup "z"] end]] end f 1`,
		`def f fn [[x:Integer][Any][case x [[lt 10] add "z"]]] end f 20`,
		`def f fn [[x:Integer][Any][case x [[lt 10] add "z"]]] end 1 2 f 1`,
		`def f fn [[x:Integer][Any][case x [[lt 10] 1 "z"]]] end f 1`,
		`def f ([x:Integer] => [case x [[lt 10] dup "z"]]) end f 1`,
		`def f ([x:Integer] => [case x [[lt 10] dup "z"]]) end f 20`,
		`[1 2] each ([x:Integer] => [case x [[lt 10] dup "z"]])`,
		`[20 30] each ([x:Integer] => [case x [[lt 10] dup "z"]])`,
		`def f fn [[x:Integer][Any][do [case x [[lt 10] dup "z"]]]] end f 1`,
	} {
		requireCompiledParity(t, src)
	}
}

// TestNUR342ReSteppedBlockEffectOrder: the re-stepped word runs where the
// case stood — after every effect before the case and before every effect
// after it — and a guard is decided only when it has no effect of its own.
func TestNUR342ReSteppedBlockEffectOrder(t *testing.T) {
	for _, src := range []string{
		`print "a" 1 2 case 7 [[lt 10] add] print "b"`,
		`1 2 case 7 [[lt 10] add] print`,
		`1 2 case (print "p" 7) [[lt 10] add "z"]`,
	} {
		requireCompiledOutputParity(t, src)
	}
}

// TestNUR337ParkedEffectfulMemberCompiles pins NUR337's remainder: a parked
// paren-bounded fn-value apply over a container member with an effect
// (`({a:(print 1)} lam/v) print 2`) declined — the interpreter leaves the
// container pending and its end-of-run sweep evaluates the member AFTER the
// next statement has run (2, then 1). The compiled program already leaves it
// to the same sweep (the `end print 2` twin compiled with parity); what
// declined it was the collection-hazard note (NUR121): the check pass lays a
// forward operand out beneath its word (rearrangeForForward), so `print 2`
// read as a stack collection reaching past the parked lam. A dispatch whose
// every operand was written after its word collected nothing a PARKED fn
// value beneath it could have taken — the word is a barrier — so such a
// value is no longer marked. A fn-typed CARRIER still is: it may be a fn
// word's read, which the run dispatches where it stands.
func TestNUR337ParkedEffectfulMemberCompiles(t *testing.T) {
	const lamInt = `def lam ([x:Integer] => [x]) end `
	for _, src := range []string{
		lamInt + `({a:(print 1)} lam/v) print 2`,
		lamInt + `({a:(print 1)} lam/v) print 2 print 3`,
		lamInt + `[({a:(print 1)} lam/v) print 2]`,
		lamInt + `([(print 1)] lam/v) print 2`,
		lamInt + `def g fn [[][Any][({a:(print 1)} lam/v) print 2]] end g`,
		// the twins that compiled before, unchanged
		lamInt + `({a:(print 1)} lam/v) end print 2`,
		lamInt + `({a:(print 1)} lam/v)`,
		lamInt + `({a:(print 1)} lam/v) 7`,
		lamInt + `({a:(1 add 2)} lam/v) print 2`,
		// the word's result does not land on the parked fn
		`def lam ([x:Integer] => [x add 100]) end ({a:1} lam/v) add 1 2`,
		`def lam ([x:Integer] => [x add 100]) end ({a:1} lam/v) print 2 5`,
		`def lam ([x:Integer] => [x add 100]) end ({a:1} lam/v) size "abc"`,
		`def lam ([x:String] => [x]) end ({a:1} lam/v) print "q" "r"`,
		`def g fn [[f:Function][Any][f/v print "ab"]] end g ([x:Integer] => [x mul 10])`,
	} {
		requireCompiledOutputParity(t, src)
	}
}

// TestNUR337CarrierLeadStaysMarked is the negative half: a fn-typed CARRIER
// beneath a forward-only dispatch keeps its collection-hazard mark. `f add
// 1 2` over f:Function steps f as a call at run time — `add` is a barrier,
// so f is called over nothing and raises — where the model cannot dispatch
// the carrier and would apply it to add's result (30). The program declines
// on the mark, never compiling that answer.
func TestNUR337CarrierLeadStaysMarked(t *testing.T) {
	for _, src := range []string{
		`def g fn [[f:Function][Any][f add 1 2]] end g ([x:Integer] => [x mul 10])`,
		`def g fn [[f:Function][Any][f size "ab"]] end g ([x:Integer] => [x mul 10])`,
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		gotI, errI := mustNew(t).RunInterp(src)
		if errI == nil || !strings.Contains(errI.Error(), "cannot call `f`") {
			t.Errorf("%q: interpreter %v [%v], want its no-match on f", src, gotI, errI)
		}
		if compiled && (fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI)) {
			t.Errorf("%q must decline or agree, but compiled %v [%v]", src, gotC, errC)
		}
		_, reason, _, _ := mustNew(t).CompileCheck(src)
		if !strings.Contains(reason, "NUR121") {
			t.Errorf("%q: want the NUR121 decline, got %q", src, reason)
		}
	}
}
