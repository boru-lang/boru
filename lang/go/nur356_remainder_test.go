package lang

import (
	"fmt"
	"strings"
	"testing"
)

// nur356_remainder_test.go pins NUR356's remainder (2026-09-29): a no-match
// note renders every operand the interpreter's window holds as written, and
// a value-returning native with an observable effect is never run where the
// interpreter runs it later or never.

const nur356H = `def h fn [[] [Any] [3]] end `

// TestNUR356UserCallNoMatchRendersLiteral: a user call over a gradual
// argument whose contract refuses it raises the interpreter's no-match, and
// the window's pending literal renders as written — `[word(x)]`, not the
// `[1]` the compiled call assembled (NUR356 (a), CallWindows).
func TestNUR356UserCallNoMatchRendersLiteral(t *testing.T) {
	const x = `def x 1 end `
	const f = `def f fn [[a:List b:List][Any][a]] end `
	for _, tc := range []struct{ src, want string }{
		{nur356H + x + f + `f [x] (h)`, "ERROR:the arguments were [word(x)] (a List) and 3"},
		{nur356H + f + `f [1 add 2] (h)`, "ERROR:[1 word(add) 2] (a List)"},
		{nur356H + f + `f [[1 add 2] 5] (h)`, "ERROR:[[1 word(add) 2] 5]"},
		{nur356H + x + `def f fn [[a:Map b:List][Any][a]] end f {a:x} (h)`, "ERROR:{a:word(x)} (a Map)"},
		{nur356H + x + `def f fn [[a:Map b:List][Any][a]] end f {a:(x add 1)} (h)`, "ERROR:{a:paren([word(x) word(add) 1])}"},
		{nur356H + x + `def f fn [[b:List a:List][Any][a]] end f (h) [x]`, "ERROR:3 (an Integer) and [word(x)]"},
		{nur356H + x + f + `[x] (h) f`, "ERROR:3 (an Integer) and [word(x)]"},
		{nur356H + f + `def g fn [[y:Integer][Any][f [y 1 add 2] (h)]] end g 4`, "ERROR:[word(y) 1 word(add) 2]"},
		{nur356H + x + f + `def g fn [[][Any][f [x] (h)]] end g`, "ERROR:[word(x)] (a List)"},
		// The contract holds: the literal is taken and evaluated.
		{`def h fn [[] [Any] [[3]]] end def x 1 end ` + f + `f [x] (h)`, "[[1]]"},
		// A scalar literal renders the same either way.
		{nur356H + f + `f [1 2] (h)`, "ERROR:[1 2] (a List)"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR356SplitNoMatchRendersWordRead: a poly's no-match over a layout
// whose written operand the pass read from a data word renders the word
// where the interpreter's plan left it on its tape, so its report stops
// there — "the argument was 3", never "3 and 5" (NUR356 (a),
// PolySplit.Words). `fold (h) 0 l` used to be an internal_error compiled.
func TestNUR356SplitNoMatchRendersWordRead(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{nur356H + `def l 5 end each (h) l`, "ERROR:the argument was 3"},
		{nur356H + `def l "s" end each (h) l`, "ERROR:the argument was 3"},
		{nur356H + `def l 5 end 1 each (h) l`, "ERROR:the arguments were 3 (an Integer) and 1"},
		{nur356H + `def l 5 end fold (h) 0 l`, "ERROR:the arguments were 3 (an Integer) and 0"},
		{nur356H + `def l [1] end fold (h) 0 l`, "ERROR:the arguments were 3 (an Integer) and 0"},
		{nur356H + `def l 5 end fold (h) l 0`, "ERROR:the argument was 3"},
		{nur356H + `def l 5 end def m 6 end each (h) l m`, "ERROR:the argument was 3"},
		{nur356H + `def l "s" end def m {a:1} end each (h) l m`, "ERROR:the argument was 3"},
		{nur356H + `def l 5 end [each (h) l]`, "ERROR:the argument was 3"},
		{nur356H + `def g fn [[] [Any] [def l [print "p" 1] each (h) l]] end g`, "ERROR:the argument was 3"},
		{nur356H + `def g fn [[] [Any] [def l [1 2] each (h) l end]] end g`, "ERROR:the argument was 3"},
		{nur356H + `def g fn [[] [Any] [def l 5 each (h) l]] end g`, "ERROR:the argument was 3"},
		{nur356H + `def g fn [[n:Integer] [Any] [each (h) n]] end g 5`, "ERROR:the argument was 3"},
		{nur356H + `def g fn [[n:Integer] [Any] [def l n each (h) l]] end g 5`, "ERROR:the argument was 3"},
		{nur356H + `def l 5 end def g fn [[] [Any] [fold (h) 0 l]] end g`, "ERROR:the arguments were 3 (an Integer) and 0"},
		{`def k fn [[x:Any][Any][x]] end def h fn [[] [Any] [k/v]] end def l 5 end each (h) l`, "ERROR:the argument was fn k(Any)"},
		// The run's value fits: the poly dispatches the word's binding.
		{`def k fn [[x:Any][Any][x]] end def h fn [[] [Any] [k/v]] end def l [1 2] end each (h) l`, "[[1 2]]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR356EffectfulNativeDeclines: NEGATIVE — a native declaring
// core.CompileSideEffect (a random draw, the clock, an environment read, a
// flex container's in-place write) or a value-dependent raise (`div` by a
// computed zero) inside a literal a call matched at run time takes, or
// between an `if` arm and the pending literal it leaves, declines "(NUR356)"
// — each used to run early compiled. The interpreter's answer stands.
func TestNUR356EffectfulNativeDeclines(t *testing.T) {
	const rnd = `import "boru:rand" end `
	for _, tc := range []struct{ src, want string }{
		{rnd + nur356H + `each (h) [Rand.int 0 100]`, "ERROR:cannot call `each`"},
		{rnd + nur356H + `def g fn [[] [Any] [each (h) [Rand.int 0 100]]] end g`, "ERROR:cannot call `each`"},
		{`import "boru:time-util" end ` + nur356H + `each (h) [TimeUtil.now]`, "ERROR:cannot call `each`"},
		{`import "boru:io" end ` + nur356H + `each (h) [IO.env "HOME"]`, "ERROR:cannot call `each`"},
		{nur356H + `def fl (flex [1]) end each (h) [push 2 fl]`, "ERROR:cannot call `each`"},
		{nur356H + `def fl (flex [1]) end do [each (h) [push 2 fl]] end fl`, "[error(cannot call `each` — no signature matches the arguments) [1]]"},
		{nur356H + `def z fn [[] [Any] [0]] end each (h) [1 div (z)]`, "ERROR:cannot call `each`"},
		{nur356H + `def z fn [[] [Integer] [0]] end each (h) [1 mod (z)]`, "ERROR:cannot call `each`"},
		// Between an arm and its pending literal: the flex write runs
		// first on the interpreter, and the literal reads its effect.
		{`def fl (flex [1]) end def c true end if c [[size fl]] [0] end push 9 fl end 5`, "[[2] [1 9] 5]"},
		{`def fl (flex [1]) end def c true end if c [[fl get 0]] [0] end set 0 7 fl end 5`, "[[7] [7] 5]"},
	} {
		declinesWithInterpAnswer(t, tc.src, "(NUR356)", tc.want)
	}
}

// TestNUR356QuietGuardDefers: a poly the walks judged quiet though one of
// its word's overloads writes in place (`push` / `set` over a gradual
// container) compiles with the guard; a run that picks the flex write is a
// designed defer — loud — where the silent early write used to answer
// `[[1] …]` for the interpreter's `[[2] …]`.
func TestNUR356QuietGuardDefers(t *testing.T) {
	const mk = `def mk fn [[][Any][flex [1]]] end def fl (mk) end def c true end `
	for _, tc := range []struct{ src, want string }{
		{mk + `if c [[size fl]] [0] end push 9 fl end 5`, "[[2] [1 9] 5]"},
		{mk + `if c [[fl get 0]] [0] end set 0 7 fl end 5`, "[[7] [7] 5]"},
	} {
		r := runBothWithOutput(t, tc.src)
		if !r.compiled || r.errC == nil || !strings.Contains(r.errC.Error(), "NUR356") || !strings.Contains(r.errC.Error(), "internal_error") {
			t.Errorf("%s: want the NUR356 guard's defer, got compiled=%v %v %v", tc.src, r.compiled, r.gotC, r.errC)
		}
		if r.errI != nil || fmt.Sprint(r.gotI) != tc.want {
			t.Errorf("%s: interpreted %v %v, want %s", tc.src, r.gotI, r.errI, tc.want)
		}
	}
	// A plain list at the same site: the quiet overload runs, both lanes agree.
	agreeOnBothLanes(t, `def mk fn [[][Any][[1]]] end def fl (mk) end def c true end if c [[size fl]] [0] end push 9 fl end 5`, "[[1] [1 9] 5]")
}

// TestNUR356SureMatchCompiles: POSITIVE — a poly whose operands are
// constants and literals as written matches whatever the literals evaluate
// to, so the interpreter evaluates them right where the compiled code
// assembles them, and an effect inside runs on both lanes in the same order
// (kg's `make KgEntity {… attributes:(m set k v)}`, `flatten [(push …)]`).
func TestNUR356SureMatchCompiles(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`def mk fn [[][Any][flex {k:1}]] end def P class {x:{}} end def m (mk) end (make P {x:(m set k 2)}) dot x end m`, "[{k:2} {k:2}]"},
		{`def h fn [[] [Any] [[1 2]]] end def m fn [[] [Any] [{k:1}]] end def g fn [[] [Any] [flatten [(h) ((m) set k 2)]]] end g`, "[[1 2 {k:2}]]"},
		{`def mk fn [[][Any][flex [1]]] end def fl (mk) end def g fn [[] [Any] [flatten [(push 2 fl)]]] end g end fl`, "[[1 2] [1 2]]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}
