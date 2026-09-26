package lang

import (
	"fmt"
	"testing"
)

// TestApplyShapesParity pins S1b's apply shapes (2026-09-22): the leading
// one-arg fn-carrier window `(f x)` is recorded wherever the lead is a slot
// of the recording unit — inside a branch arm, a loop body, a `do` body, a
// callback lambda (each/fold) — and its argument may be a gradual value
// whose declared bound excludes Function (a lambda's `e:Integer` param, now
// narrowed from the callback's Any element carrier) or a fn VALUE that
// arrived inert (`g/v`, a lambda literal), which the lead binds to its
// Function param exactly as the interpreter's forward collection does
// (RecordDynApplyLead). Every shape runs on both lanes and must agree —
// values and error codes alike.
func TestApplyShapesParity(t *testing.T) {
	for _, src := range []string{
		// the lead inside a branch arm, taken and not taken
		`def app fn [[f:Function x:Integer b:Boolean] [Integer] [if b [(f x)] [0]]] end app ([n:Integer] => [n add 1]) 4 true`,
		`def app fn [[f:Function x:Integer b:Boolean] [Integer] [if b [(f x)] [0]]] end app ([n:Integer] => [n add 1]) 4 false`,
		// inside a loop body, accumulating through a loop-carried def
		`def app fn [[f:Function x:Integer] [Integer] [def r 0 end for [0 3] [def r (r add (f x))] end r]] end app ([n:Integer] => [n add 1]) 4`,
		// the island-era witness: a factory's captured param applied inside a do body
		`def mkg fn [[g:Function] [Function] [([v:Integer] => [(g v)])]] end def h (mkg ([n:Integer] => [n mul 8])) end do [(h 1)]`,
		// a callback lambda's typed param over an untyped collection
		`def sumf fn [[f:Function xs:List] [Integer] [0 fold ([a:Integer e:Integer] => [a add (f e)]) xs]] end sumf ([n:Integer] => [n mul 2]) [1 2 3]`,
		`def sumf fn [[f:Function xs:List] [Integer] [0 fold ([a:Integer e:Integer] => [a add (f e)]) xs]] end sumf ([n:Integer] => [n mul 2]) []`,
		`def mapf fn [[f:Function xs:List] [List] [each ([e:Integer] => [(f e)]) xs]] end mapf ([n:Integer] => [n mul 2]) [1 2 3]`,
		// a fn value at the argument position: a /v read, a lambda literal, a captured param's /v read
		`def inc fn [[n:Integer] [Integer] [n add 1]] end def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k inc/v)]] end h app/v`,
		`def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k ([n:Integer] => [n add 1]))]] end h app/v`,
		`def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function g:Function] [Integer] [(k g/v)]] end def inc fn [[n:Integer] [Integer] [n add 1]] end h app/v inc/v`,
		`def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k/v inc/v)]] end def inc fn [[n:Integer] [Integer] [n add 1]] end h app/v`,
		// the lead's Any-typed param takes the fn value too
		`def app fn [[g:Any] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k inc/v)]] end def inc fn [[n:Integer] [Integer] [n add 1]] end h app/v`,
		// the event seats like any computed result: an operand, a def-local
		`def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k inc/v) add 1]] end def inc fn [[n:Integer] [Integer] [n add 1]] end h app/v`,
		`def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [def r (k inc/v) r]] end def inc fn [[n:Integer] [Integer] [n add 1]] end h app/v`,
		// a chained apply over the lead
		`def twice fn [[f:Function x:Integer] [Integer] [(f (f x))]] end twice ([n:Integer] => [n mul 2]) 3`,
		// the runtime lead rejects the fn value: a no-match on both lanes
		`def app fn [[n:Integer] [Integer] [n add 3]] end def inc fn [[n:Integer] [Integer] [n add 1]] end def h fn [[k:Function] [Integer] [(k inc/v)]] end h app/v`,
		`def two fn [[a:Function b:Integer] [Integer] [(a b)]] end def inc fn [[n:Integer] [Integer] [n add 1]] end def h fn [[k:Function] [Integer] [(k inc/v)]] end h two/v`,
		// a quoted fn word at the argument position stays a quote: a no-match on both lanes
		`def inc fn [[n:Integer] [Integer] [n add 1]] end def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k (quote inc))]] end h app/v`,
	} {
		gotI, errI := mustNew(t).RunInterp(src)
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			t.Errorf("%s: did not compile: %v", src, errC)
			continue
		}
		if !compiled {
			t.Fatalf("%s: did not run compiled (%v)", src, errC)
		}
		if codeOf(errI) != codeOf(errC) || fmt.Sprint(gotI) != fmt.Sprint(gotC) {
			t.Errorf("%s:\n  interp   %v err=[%s]\n  compiled %v err=[%s]", src, gotI, codeOf(errI), gotC, codeOf(errC))
		}
	}
}

// TestApplyShapesBareFnWordArgDeclines pins the argument the leading window
// must keep out: a BARE fn word. `(k inc)` is not a value the lead collects
// — the interpreter's word dispatch of `inc` runs first (its no-match, or
// its 0-arg fire), and the lead's model would bind the fn instead. The
// compiled lane declines it (loudly, at the check) rather than answering.
func TestApplyShapesBareFnWordArgDeclines(t *testing.T) {
	for _, src := range []string{
		`def inc fn [[n:Integer] [Integer] [n add 1]] end def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k inc)]] end h app/v`,
		// a bare read of a Function-typed PARAM is a word dispatch too (NUR123)
		`def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function g:Function] [Integer] [(k g)]] end def inc fn [[n:Integer] [Integer] [n add 1]] end h app/v inc/v`,
	} {
		prog, _, _, cerr := mustNew(t).CompileCheck(src)
		if cerr == nil && prog != nil {
			t.Errorf("%s: compiled — a bare fn word at the lead's argument position must decline", src)
		}
	}
}

// TestApplyShapesZeroArgLeadResolves pins NUR176's close: a 0-arg runtime
// lead under the one-arg leading window `(k x)` — and under a trailing
// window `(1 2 c)` — fires over NOTHING on the interpreter, which then steps
// the tokens written after the lead on their own (a fn value dispatching
// over the result, a literal landing beside it) while the values written
// before it stay beneath. The window op used to raise the no-match a
// 1-arg window owes a fn that takes none (`signature_error`); it hands such
// a lead to the island in the window's written order now, a name-read lead
// marked applied so an anonymous 0-arg value dispatches as the word did.
func TestApplyShapesZeroArgLeadResolves(t *testing.T) {
	for _, src := range []string{
		`def z fn [[] [Integer] [7]] end def inc fn [[n:Integer] [Integer] [n add 1]] end def h fn [[k:Function] [Integer] [(k inc/v)]] end h z/v`,
		`def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Integer] [(k 5)]] end h z/v`,
		`def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k ([] => [9]))]] end h app/v`,
		`def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Integer Integer] [(k 5)]] end h z/v`,
		`def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Integer Integer Integer] [(1 2 k)]] end h z/v`,
		`def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Integer Integer] [(1 k 5)]] end h z/v`,
		`def z fn [[] [Integer] [7]] end def dbl fn [[n:Integer] [Integer] [n mul 2]] end def h fn [[k:Function] [Integer] [(k dbl/v)]] end h z/v`,
		`def app fn [[g:Function] [Integer Integer] [(g 3)]] end def h fn [[k:Function] [Integer Integer] [(k ([] => [9]))]] end h app/v`,
	} {
		requireSameVerdict(t, src)
	}
	for _, c := range []struct{ src, want string }{
		{`def z fn [[] [Integer] [7]] end def inc fn [[n:Integer] [Integer] [n add 1]] end def h fn [[k:Function] [Integer] [(k inc/v)]] end h z/v`, "[8]"},
		{`def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Integer Integer] [(k 5)]] end h z/v`, "[7 5]"},
		{`def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Integer Integer Integer] [(1 2 k)]] end h z/v`, "[1 2 7]"},
		{`def app fn [[g:Function] [Integer Integer] [(g 3)]] end def h fn [[k:Function] [Integer Integer] [(k ([] => [9]))]] end h app/v`, "[9 3]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
}
