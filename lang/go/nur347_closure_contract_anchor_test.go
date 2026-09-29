package lang

import (
	"strings"
	"testing"
)

// anchoredOnBothLanes is agreeOnBothLanes for an error the test pins by its
// frame name and its primary position: both lanes raise byte-identically,
// the report names `name:` and points at `at` ("source position unknown"
// for none).
func anchoredOnBothLanes(t *testing.T, src, name, at string) {
	t.Helper()
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled {
		t.Errorf("%s: must compile, got %v", src, errC)
		return
	}
	if errC == nil || errI == nil || errC.Error() != errI.Error() {
		t.Errorf("%s:\n  compiled %v / %v\n  interp   %v / %v", src, gotC, errC, gotI, errI)
		return
	}
	msg := errI.Error()
	if !strings.Contains(msg, "]: "+name+": ") {
		t.Errorf("%s: want the frame named %q, got %v", src, name, msg)
	}
	if !strings.Contains(msg, "--> "+at+"\n") && !strings.HasSuffix(msg, "--> "+at) {
		t.Errorf("%s: want the report at %s, got %v", src, at, msg)
	}
}

// TestNUR347ClosureContractAnchor pins NUR347's close: a fn VALUE's return
// contract error names the interpreter's frame and points where the
// interpreter's frame points.
//
//   - The frame of a nameless VERBOSE `fn` value is `<fn>` (a `=>` lambda's
//     is unnamed): a callback body unit's and a factory's returned closure's
//     report said “ (closureFrameName).
//   - A nameless value answers at its OWN position: a `fn` literal's token
//     (the returned closure's push carried the enclosing paren, 1:36 for
//     1:37), a word's result stamp where the literal had none (`m.f` over a
//     stored lambda — the VM's member read now stamps a positionless fn
//     result as execMatch does, stampFnResultPos), and nothing for a lambda
//     that had none — which no apply op then stamps (AnchorFinal).
//   - A value its binding names answers at the name, the word that calls it.
//   - The fn-VALUE seam (walk's hooks: CallBoru) is unlabelled and answers at
//     the calling word.
func TestNUR347ClosureContractAnchor(t *testing.T) {
	const mkFn = `def mk fn [[k:Integer] [Function] [(fn [[a:Integer][String] [a add k]])]] end `
	const mkLam = `def mk fn [[k:Integer] [Function] [([a:Integer] => [a k])]] end `
	for _, c := range []struct{ src, name, at string }{
		// The register's rows.
		{`def m {f: ([x:Integer] => [x x])} each m.f [1]`, "", "1:40"},
		{`def m {f: ([x:Integer] => [x x])} fold m.f [1] 0`, "", "1:40"},
		{`each (fn [[x:Integer][Integer][x x]]) [1]`, "<fn>", "1:7"},
		{mkFn + `((mk 1) 5)`, "<fn>", "1:37"},
		{mkLam + `((mk 1) 5)`, "", "source position unknown"},
		// Variants: the word read, a fold, a nested body, a list, a map member.
		{`def m {f: ([x:Integer] => [x x])} each (m get "f") [1]`, "", "1:43"},
		{`def m {f: ([x:Integer] => [x x])} [1] each m.f`, "", "1:44"},
		{`def f fn [[] [Any] [def m {f: ([x:Integer] => [x x])} each m.f [1]]] end f`, "", "1:60"},
		{`each (fn [[x:Integer][String][x]]) [1]`, "<fn>", "1:7"},
		{`fold (fn [[x:Integer y:Integer][Integer][x y]]) [1] 0`, "<fn>", "1:7"},
		{`def f fn [[] [Any] [each (fn [[x:Integer][Integer][x x]]) [1]]] end f`, "<fn>", "1:27"},
		{mkFn + `each (mk 1) [5]`, "<fn>", "1:37"},
		{mkFn + `def f fn [[] [Any] [((mk 1) 5)]] end f`, "<fn>", "1:37"},
		{`def mk fn [[k:Integer] [Function] [(fn [[a:Integer][Integer] [a k]])]] end ((mk 1) 5)`, "<fn>", "1:37"},
		{mkLam + `each (mk 1) [5]`, "", "source position unknown"},
		{mkLam + `def f fn [[] [Any] [((mk 1) 5)]] end f`, "", "source position unknown"},
		{mkLam + `[((mk 1) 5)]`, "", "source position unknown"},
		{mkLam + `if true [((mk 1) 5)] [0]`, "", "source position unknown"},
		{mkLam + `def m {f: (mk 1)} each m.f [5]`, "", "1:88"},
		{mkFn + `def m {f: (mk 1)} (m.f 5)`, "<fn>", "1:37"},
		// Negative: `apply` hands the value back stamped, so the run answers
		// at the `apply` — the stamp is the word's, not every op's.
		{mkLam + `5 (mk 1) apply`, "", "1:74"},
		// A named value answers at the name that calls it.
		{mkFn + `def h (mk 1) end h 5`, "h", "1:96"},
		{mkFn + `def h (mk 1) end (h 5)`, "h", "1:97"},
		{mkFn + `def h (mk 1) end [(h 5)]`, "h", "1:98"},
		{mkLam + `def h (mk 1) end h 5`, "h", "1:82"},
		{mkFn + `def f fn [[] [Any] [def h (mk 1) h 5]] end f`, "h", "1:112"},
		{mkFn + `def f fn [[g:Function] [Any] [g 5]] end f (mk 1)`, "g", "1:109"},
		{mkLam + `def f fn [[g:Function] [Any] [g 5]] end f (mk 1)`, "g", "1:95"},
		// A word's result keeps its stamp only until a binding takes it.
		{`import "boru:fn-util"  def two x:Integer => [x x] end def h (FnUtil.compose two/v two/v) end (h 5)`, "FnUtil.compose", "1:95"},
		{`import "boru:fn-util"  def two x:Integer => [x x] end def f fn [[] [Any] [def h (FnUtil.compose two/v two/v) (h 5)]] end f`, "FnUtil.compose", "1:111"},
		{`import "boru:fn-util"  def two x:Integer => [x x] end def m {h: (FnUtil.compose two/v two/v)} (m.h 5)`, "FnUtil.compose", "1:66"},
		{`import "boru:fn-util"  def two x:Integer => [x x] end each (FnUtil.compose two/v two/v) [5]`, "FnUtil.compose", "1:61"},
		// The fn-VALUE seam: unlabelled, at the calling word.
		{`def cb fn [[m:Any][Integer][m.path "s"]]  walk {mode:"breadth"} {a:1} cb/v`, "", "1:43"},
		{`walk {mode:"breadth"} {a:1} (fn [[m:Any][Integer][m.path "s"]])`, "", "1:1"},
		{`def f fn [[] [Any] [walk {mode:"breadth"} {a:1} (fn [[m:Any][Integer][m.path "s"]])]] end f`, "", "1:21"},
	} {
		anchoredOnBothLanes(t, c.src, c.name, c.at)
	}
	// A nameless verbose value that matches no signature is still DATA — the
	// frame name is the contract's only; it never makes the value a callee.
	for _, c := range []struct{ src, want string }{
		{`each (fn [[x:Integer][Integer][x]]) ['a']`, "[[fn (Integer)]]"},
		{mkFn + `((mk 1) 'z')`, "[fn (Integer) z]"},
		{mkFn + `each (mk 1) ['z']`, "[[fn (Integer)]]"},
		// A conforming value runs clean.
		{`def mk fn [[k:Integer] [Function] [(fn [[a:Integer][Integer] [a add k]])]] end ((mk 1) 5)`, "[6]"},
		{mkLam + `[1 2] each (mk 3)`, "ERROR:expected 1 return value(s), got 2"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
