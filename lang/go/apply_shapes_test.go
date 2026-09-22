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

// TestApplyShapesZeroArgLeadIsLoud pins NUR176: a 0-ARG runtime lead under
// the one-arg window. The interpreter dispatches it with no args and leaves
// the argument as residual — so a fn-valued argument then APPLIES to the
// lead's result (`(k inc/v)` with k = `[] -> 7` answers 8), and a literal
// argument fails the frame's return count (type_error) — where the compiled
// window no-matches (signature_error). The bar this test holds is that the
// compiled lane is LOUD on every member of the family: it never answers a
// value of its own. The interpreter's answers are asserted too, so a moved
// oracle re-opens the record rather than passing silently.
func TestApplyShapesZeroArgLeadIsLoud(t *testing.T) {
	for _, tc := range []struct {
		src       string
		wantI     string // the interpreter's value, or "" for an error
		wantICode string
	}{
		{`def z fn [[] [Integer] [7]] end def inc fn [[n:Integer] [Integer] [n add 1]] end def h fn [[k:Function] [Integer] [(k inc/v)]] end h z/v`, "[8]", ""},
		{`def z fn [[] [Integer] [7]] end def h fn [[k:Function] [Integer] [(k 5)]] end h z/v`, "[]", "type_error"},
		{`def app fn [[g:Function] [Integer] [(g 3)]] end def h fn [[k:Function] [Integer] [(k ([] => [9]))]] end h app/v`, "[]", "type_error"},
	} {
		gotI, errI := mustNew(t).RunInterp(tc.src)
		if fmt.Sprint(gotI) != tc.wantI || codeOf(errI) != tc.wantICode {
			t.Errorf("%s: interpreter oracle moved: got %v err=[%s], want %s err=[%s] — re-derive NUR176", tc.src, gotI, codeOf(errI), tc.wantI, tc.wantICode)
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(tc.src)
		if noteCompileDefect(t, tc.src, gotC, errC) {
			continue // a loud decline satisfies the bar
		}
		if !compiled {
			t.Fatalf("%s: did not run compiled (%v)", tc.src, errC)
		}
		if codeOf(errC) != "signature_error" || len(gotC) != 0 {
			t.Errorf("%s: compiled got %v err=[%s], want a loud signature_error — a 0-arg lead must never answer a value of its own", tc.src, gotC, codeOf(errC))
		}
	}
}
